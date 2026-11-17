import {api} from './api'
import {activeLatencyTargets} from './latencyTargets'
import {manualLatencyPoint,monitorLatencyPoints,type TrendPoint} from './presentation'
import {inspectorMedian,inspectorSamples,stableLatency} from './inspectorLatency'
import {date,key,scope,type NodeOption,type LatencyTest,type MonitorSample} from './domain'
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
export type LatencyRound = ReturnType<typeof latencyRounds>[number]
export type RoundSiteTone = 'good' | 'warn' | 'bad' | 'empty'
export interface RoundSiteDetail {id:string;name:string;stable:number|null;first:number|null;responses:number;total:number;tone:RoundSiteTone;reason:string}
// One row per site inside a saved round: stable latency drops the first
// sample, first connection is that first sample, and the tone is based on
// how many of the saved samples actually responded.
export function roundSiteDetails(round: LatencyRound): RoundSiteDetail[] {
 return round.series.map(s => {
  const point = s.points.find(p => p.id === round.id)
  const list = point ? inspectorSamples(point, s.id) : []
  const values = list.flatMap(p => p.value === null ? [] : [p.value])
  const failed = list.find(p => p.value === null)
  const total = list.length, responses = values.length
  return {
   id: s.id,
   name: s.name,
   stable: inspectorMedian(list.slice(1).flatMap(p => p.value === null ? [] : [p.value])),
   first: list[0]?.value ?? null,
   responses,
   total,
   tone: !total ? 'empty' : !responses ? 'bad' : responses === total ? 'good' : 'warn',
   reason: !total ? '没有采样记录' : failed?.error || failed?.note || '',
  }
 })
}
export interface LatencyMedianPoint {id:string;time:string;value:number|null;details:RoundSiteDetail[]}
// The history chart keeps one point per round: the median of the four sites'
// stable latencies. A round where no site produced a stable value is a failure.
export function latencyMedianPoints(series: LatencySeries[]): LatencyMedianPoint[] {
 return latencyRounds(series).map(round => ({id: round.id, time: round.time, value: round.stable, details: roundSiteDetails(round)}))
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
async function readScope(node:NodeOption,since:string,until:string){
 const [tests,monitor]=await Promise.all([(async()=>{const all:LatencyTest[]=[];let before={};for(;;){const page=await api.latencyHistory(node,{since,until,limit:100,...before});all.push(...(page.tests||[]));if(!page.has_more)break;const last=page.tests!.at(-1)!;before={before_finished_at:last.finished_at,before_attempt_id:last.attempt_id}}return all})(),(async()=>{const all:MonitorSample[]=[];let cursor:string|undefined;for(;;){const page=await api.monitorSamples({...scope(node),since,until,probe_type:'rtt',limit:100,order_desc:true,cursor});all.push(...(page.items||[]));if(!page.has_more)break;cursor=page.next_cursor}return all})()])
 return {tests,monitor}
}
// Read every saved revision of the same subscription node, including identities the
// subscription replaced, and keep the current revision as the reference for tagging.
export async function readLatencyHistoryWithConfig(node:NodeOption,hours:number):Promise<{series:LatencySeries[];changes:LatencyConfigChange[]}>{
 const until=new Date().toISOString(),since=new Date(Date.parse(until)-hours*3600000).toISOString()
 const saved=await api.latencyHistoryScopes(node,{since,until})
 const scopeKey=(n:NodeOption)=>key(n)+'\u0000'+n.node_key
 const scopes=new Map<string,NodeOption>()
 for(const candidate of [node,...saved.map(s=>({...node,...s}))])scopes.set(scopeKey(candidate),candidate)
 const loaded=await Promise.all([...scopes].map(async([id,item])=>({old:id!==scopeKey(node),...await readScope(item,since,until)})))
 const series=activeLatencyTargets.map(target=>({...target,context:'',points:loaded.flatMap(batch=>[...batch.tests.map(test=>({...manualLatencyPoint(test,target.id),...(batch.old?{oldConfig:true}:{})})),...monitorLatencyPoints(batch.monitor,target.id).map(point=>batch.old?{...point,oldConfig:true}:point)]).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))}))
 const rounds=loaded.flatMap(batch=>batch.tests.map(test=>({scope:key(test)+'\u0000'+test.node_key,time:test.finished_at||test.requested_at}))).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
 const changes:LatencyConfigChange[]=[]
 for(let i=1;i<rounds.length;i++)if(rounds[i].scope!==rounds[i-1].scope)changes.push({time:rounds[i].time,label:'配置已变化'})
 return {series,changes}
}
export async function readLatencyHistory(node:NodeOption,hours:number):Promise<LatencySeries[]>{return (await readLatencyHistoryWithConfig(node,hours)).series}
