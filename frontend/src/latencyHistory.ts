import {api} from './api'
import {activeLatencyTargets} from './latencyTargets'
import {manualLatencyPoint,monitorLatencyPoints,type TrendPoint} from './presentation'
import {inspectorMedian,inspectorSamples,stableLatency} from './inspectorLatency'
import {date,scope,type NodeOption,type LatencyTest,type MonitorSample,type NodeScope} from './domain'
export interface LatencySeries {id:string;name:string;points:TrendPoint[];context:string}
export interface LatencyConfigChange {time:string;label:string;serverChanged?:boolean}
export interface FailedSite {name:string;failures:number;total:number}
// Four identical "site failed" clauses read worse than one sentence. Only a
// round where every site failed outright gets the merged wording; partial
// failures still name each site.
export function failureText(failedSites:FailedSite[],responses:number,style:'summary'|'detail'){
 if(responses===0&&failedSites.length>=4)return style==='summary'?'四站全失败':'四站全部无响应'
 return failedSites.map(s=>style==='summary'&&s.failures===s.total?s.name+' 全失败':s.name+' '+s.failures+' 次无响应').join(' · ')
}
export function latencyRounds(series:LatencySeries[]){
 const rounds=new Map<string,{id:string;time:string}>()
 for(const s of series)for(const p of s.points)if(!p.id.startsWith('planned:')&&inspectorSamples(p,s.id).length)rounds.set(p.id,{id:p.id,time:p.time})
 return [...rounds.values()].map(r=>{
  const selected=series.map(s=>({...s,points:s.points.filter(p=>p.id===r.id)})),samples=selected.flatMap(s=>s.points.flatMap(p=>inspectorSamples(p,s.id)))
  const failedSites:FailedSite[]=selected.flatMap(s=>{const list=s.points.flatMap(p=>inspectorSamples(p,s.id)),failures=list.filter(p=>p.value===null).length;return failures?[{name:s.name,failures,total:list.length}]:[]})
  const responses=samples.filter(s=>s.value!==null).length
  return {...r,series:selected,stable:stableLatency(selected),first:inspectorMedian(selected.flatMap(s=>{const v=s.points[0]?inspectorSamples(s.points[0],s.id)[0]?.value:null;return v==null?[]:[v]})),responses,total:samples.length,failedSites,failureSummary:failureText(failedSites,responses,'summary'),failureDetail:failureText(failedSites,responses,'detail'),oldConfig:selected.some(s=>s.points.some(p=>p.oldConfig))}
 }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
export function latencyHistoryStats(series:LatencySeries[]){
 const rounds=latencyRounds(series),values=rounds.flatMap(r=>r.stable===null?[]:[r.stable]).sort((a,b)=>a-b)
 const percentile=(p:number)=>{const x=(values.length-1)*p,i=Math.floor(x);return values[i]+(values[Math.ceil(x)]-values[i])*(x-i)}
 const responses=rounds.reduce((n,r)=>n+r.responses,0),samples=rounds.reduce((n,r)=>n+r.total,0),failed=rounds.filter(r=>r.responses<r.total),last=failed.at(-1)
 return {rounds,responses,samples,total:rounds.length,failedRounds:failed.length,responseRate:samples?responses/samples*100:null,median:inspectorMedian(values),volatility:rounds.length>=5&&values.length?percentile(.9)-percentile(.1):null,lastFailure:last?{time:last.time,detail:last.failureDetail}:null}
}
export type LatencyHistoryStats=ReturnType<typeof latencyHistoryStats>
// Current-config numbers never inherit old-config rounds: an old config may be a
// different machine, so it can only describe the past, not how the node is now.
export function latencyHistorySplit(series:LatencySeries[]){
 const pick=(keep:(point:TrendPoint)=>boolean)=>series.map(s=>({...s,points:s.points.filter(keep)}))
 const all=latencyHistoryStats(series),current=latencyHistoryStats(pick(p=>!p.oldConfig)),old=latencyHistoryStats(pick(p=>!!p.oldConfig))
 return {all,current,old,allOld:all.total>0&&current.total===0}
}
export interface LatencyHistorySplit {all:LatencyHistoryStats;current:LatencyHistoryStats;old:LatencyHistoryStats;allOld:boolean}
// Node keys embed the sanitized server host between the type and port segments.
function serverTagFromKey(nodeKey:string){
 const parts=(nodeKey||'').split('_')
 return parts.length>3?parts.slice(2,-2).join('_'):''
}
// Batch items keep the exact scope every saved test ran with, so an identity that
// a subscription update replaced is still discoverable by the node's display name.
let historicalScopesPromise:Promise<Map<string,NodeScope[]>>|null=null
function historyKey(profileID:string,displayName:string){return profileID+'\u0000'+displayName}
async function loadHistoricalScopes(){
 const batches=await api.batches(),details=await Promise.all(batches.map(batch=>api.batch(batch.batch_id).catch(()=>null))),map=new Map<string,NodeScope[]>()
 for(const batch of details)for(const item of batch?.items||[]){
  if(!item.profile_id||!item.display_name)continue
  const key=historyKey(item.profile_id,item.display_name),list=map.get(key)||[]
  if(!list.some(s=>s.node_identity_key===item.node_identity_key))list.push({profile_id:item.profile_id,node_key:item.node_key,node_identity_key:item.node_identity_key,config_revision_key:item.config_revision_key})
  map.set(key,list)
 }
 return map
}
function historicalScopes(node:NodeOption){
 if(!historicalScopesPromise)historicalScopesPromise=loadHistoricalScopes().catch(()=>new Map())
 return historicalScopesPromise.then(map=>map.get(historyKey(node.profile_id,node.display_name))||[])
}
async function revisionsFor(node:NodeOption){try{return await api.revisions(node)}catch{return []}}
async function readScope(node:NodeOption,since:string,until:string){
 const [tests,monitor]=await Promise.all([(async()=>{const all:LatencyTest[]=[];let before={};for(;;){const page=await api.latencyHistory(node,{since,until,limit:100,...before});all.push(...(page.tests||[]));if(!page.has_more)break;const last=page.tests!.at(-1)!;before={before_finished_at:last.finished_at,before_attempt_id:last.attempt_id}}return all})(),(async()=>{const all:MonitorSample[]=[];let cursor:string|undefined;for(;;){const page=await api.monitorSamples({...scope(node),since,until,probe_type:'rtt',limit:100,order_desc:true,cursor});all.push(...(page.items||[]));if(!page.has_more)break;cursor=page.next_cursor}return all})()])
 return {tests,monitor}
}
// Read every saved revision of the same subscription node, including identities the
// subscription replaced, and keep the current revision as the reference for tagging.
export async function readLatencyHistoryWithConfig(node:NodeOption,hours:number):Promise<{series:LatencySeries[];changes:LatencyConfigChange[]}>{
 const until=new Date().toISOString(),since=new Date(Date.parse(until)-hours*3600000).toISOString()
 const saved=await historicalScopes(node)
 const identities=new Map<string,NodeOption>()
 for(const candidate of [node,...saved.map(s=>({...node,...s}))])identities.set(candidate.node_identity_key,candidate)
 const identityList=[...identities.values()],revisionLists=await Promise.all(identityList.map(identity=>revisionsFor(identity)))
 const scopes=new Map<string,NodeOption>(),observedAt=new Map<string,string>()
 for(const candidate of [node,...saved.map(s=>({...node,...s}))])scopes.set(candidate.config_revision_key,candidate)
 identityList.forEach((identity,index)=>revisionLists[index].forEach(revision=>{scopes.set(revision.config_revision_key,{...identity,node_key:revision.node_key,config_revision_key:revision.config_revision_key});if(revision.last_observed_at)observedAt.set(revision.config_revision_key,revision.last_observed_at)}))
 const loaded=await Promise.all([...scopes].map(async([revision,item])=>({old:revision!==node.config_revision_key,...await readScope(item,since,until)})))
 const series=activeLatencyTargets.map(target=>({...target,context:'',points:loaded.flatMap(batch=>[...batch.tests.map(test=>({...manualLatencyPoint(test,target.id),...(batch.old?{oldConfig:true}:{})})),...monitorLatencyPoints(batch.monitor,target.id).map(point=>batch.old?{...point,oldConfig:true}:point)]).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))}))
 const currentServer=serverTagFromKey(node.node_key)
 const changes:LatencyConfigChange[]=[...observedAt].filter(([revision])=>revision!==node.config_revision_key).map(([revision,time])=>{
  const oldServer=serverTagFromKey(scopes.get(revision)?.node_key||''),serverChanged=!!currentServer&&!!oldServer&&oldServer!==currentServer
  return {time,serverChanged,label:date(time)+' 订阅更新了这个节点的配置'+(serverChanged?'；服务器地址也变了，旧记录可能不是同一台机器':'')}
 }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
 return {series,changes}
}
export async function readLatencyHistory(node:NodeOption,hours:number):Promise<LatencySeries[]>{return (await readLatencyHistoryWithConfig(node,hours)).series}
