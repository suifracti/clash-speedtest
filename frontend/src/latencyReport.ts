import {inspectorMedian} from './inspectorLatency'
import {activeLatencyTargets,latencyTargetId} from './latencyTargets'
import type {BatchItem,LatencyBatch,LatencyTest} from './domain'

export type LatencyReportStatus='usable'|'slow'|'unstable'|'no-response'|'skipped'
export type LatencyReportIssueKind='slow'|'unstable'|'no-response'|'suspected'

export interface LatencyReportSite {
 id:string
 name:string
 firstMs:number|null
 stableMs:number|null
 attemptedSamples:number
 responseSamples:number
}

export interface LatencyReportRow {
 key:string
 item:BatchItem
 nodeName:string
 airportName:string
 nodeType:string
 status:LatencyReportStatus
 executed:boolean
 attemptedSamples:number
 responseSamples:number
 firstMedianMs:number|null
 stableMs:number|null
 sites:LatencyReportSite[]
 skipReason?:string
 suspected:boolean
 suspectedReasons:string[]
}

export interface LatencyReportCounts {
 total:number
 executed:number
 usable:number
 slow:number
 unstable:number
 noResponse:number
 suspected:number
 skipped:number
}

export interface LatencyReportSummary {
 requestedAt:string
 finishedAt?:string
 durationMs:number|null
 nodeCount:number
 airportNames:string[]
 targetId?:string
 timeoutSeconds:number|null
 sampleCountPerSite:number|null
 siteCount:number
 firstMedianMs:number|null
 stableMedianMs:number|null
}

export interface LatencyReportIssue {
 kind:LatencyReportIssueKind
 reason:string
 detail:string
 item:BatchItem
 row:LatencyReportRow
}

export interface LatencyReportGroup {
 kind:LatencyReportIssueKind|'skipped'
 reason:string
 rows:LatencyReportRow[]
 issues:LatencyReportIssue[]
}

export interface LatencyReport {
 rows:LatencyReportRow[]
 counts:LatencyReportCounts
 summary:LatencyReportSummary
 fastest5:LatencyReportRow[]
 groups:LatencyReportGroup[]
 issues:LatencyReportIssue[]
 usableItems:BatchItem[]
}

export interface LatencyReportComparison {
 comparableNodes:number
 newlyFailed:number
 recovered:number
 stableMedianDeltaMs:number|null
 latestPreviousAt?:string
 reason?:string
}

type BatchConditions=LatencyBatch&{timeout_seconds?:number}
type ItemNodeType=BatchItem&{node_type?:string}

const targets=activeLatencyTargets.map(t=>({id:t.id,name:t.name}))
const identity=(item:Pick<BatchItem,'profile_id'|'node_identity_key'|'config_revision_key'>)=>`${item.profile_id}\u0000${item.node_identity_key}\u0000${item.config_revision_key}`
const finite=(value:unknown):value is number=>typeof value==='number'&&Number.isFinite(value)
const usableType=(value:string)=>!!value&&value!=='unknown'&&value!=='unverified'

function targetId(raw:string|undefined,batchTarget:string|undefined):string|undefined{
 const normalized=raw?.trim().toLowerCase()
 const direct=targets.find(t=>t.id===normalized)?.id
 if(direct)return direct
 if(raw){const matched=latencyTargetId(raw);if(matched&&targets.some(t=>t.id===matched))return matched}
 if(!raw&&batchTarget&&batchTarget!=='all'){
  const targetDirect=targets.find(t=>t.id===batchTarget.toLowerCase())?.id
  if(targetDirect)return targetDirect
  const matched=latencyTargetId(batchTarget)
  if(matched&&targets.some(t=>t.id===matched))return matched
 }
 return undefined
}

function sampleOrder(a:LatencyTest['samples'][number],b:LatencyTest['samples'][number]){
 return a.seq-b.seq||Date.parse(a.timestamp)-Date.parse(b.timestamp)
}

function skipReason(item:BatchItem):string{
 const extended=item as ItemNodeType&{not_executed_reason?:string}
 return extended.not_executed_reason||item.error_message||item.persistence_error||item.result?.error_message||'未取得采样数据'
}

function failureReason(row:LatencyReportRow):string{
 const samples=row.item.result?.samples||[]
 const counts=new Map<string,number>()
 for(const sample of samples){
  if(sample.success)continue
  const value=(sample.error||'').toLowerCase()
  const reason=/dns|resolve|解析/.test(value)?'DNS 解析失败':/tls|handshake|certificate|证书/.test(value)?'TLS 握手失败':/timeout|timed out|deadline|超时/.test(value)?'连接超时':/refus|reset|dial|connect|连接/.test(value)?'连接失败':/http|status|状态/.test(value)?'HTTP 状态异常':'无响应'
  counts.set(reason,(counts.get(reason)||0)+1)
 }
 const fallback=(row.item.error_message||row.item.persistence_error||row.item.result?.error_message||'').toLowerCase()
 if(!counts.size&&fallback){
  const reason=/dns|resolve|解析/.test(fallback)?'DNS 解析失败':/tls|handshake|certificate|证书/.test(fallback)?'TLS 握手失败':/timeout|timed out|deadline|超时/.test(fallback)?'连接超时':/refus|reset|dial|connect|连接/.test(fallback)?'连接失败':/http|status|状态/.test(fallback)?'HTTP 状态异常':'无响应'
  counts.set(reason,1)
 }
 return [...counts].sort((a,b)=>b[1]-a[1])[0]?.[0]||'无响应'
}

function makeRow(item:BatchItem,airportNames:Record<string,string>,batchTarget?:string):LatencyReportRow{
 const result=item.result
 const samples=(result?.samples||[]).slice().sort(sampleOrder)
 const bySite=new Map<string,LatencyTest['samples']>()
 for(const target of targets)bySite.set(target.id,[])
 for(const sample of samples){
  const id=targetId(sample.target||result?.target,batchTarget)
  if(id)bySite.get(id)?.push(sample)
 }
 const sites:LatencyReportSite[]=targets.map(target=>{
  const list=bySite.get(target.id)||[]
  const first=list[0]
  const stableValues=list.slice(1).filter(s=>s.success&&finite(s.latency_ms)).map(s=>s.latency_ms)
  return {id:target.id,name:target.name,firstMs:first?.success&&finite(first.latency_ms)?first.latency_ms:null,stableMs:inspectorMedian(stableValues),attemptedSamples:list.length,responseSamples:list.filter(s=>s.success).length}
 })
 const sampleSuccesses=samples.filter(s=>s.success).length
 const attemptedSamples=Math.max(samples.length,result?.total_samples||0)
 const responseSamples=Math.max(sampleSuccesses,result?.success_samples||0)
 const executed=attemptedSamples>0
 const firstValues=sites.map(s=>s.firstMs).filter(finite)
 const stableValues=sites.map(s=>s.stableMs).filter(finite)
 const firstMedianMs=inspectorMedian(firstValues)
 const stableMs=inspectorMedian(stableValues)
 const status:LatencyReportStatus=!executed?'skipped':responseSamples===0?'no-response':responseSamples*6<attemptedSamples*5?'unstable':stableMs!==null&&stableMs>1500?'slow':'usable'
 const dynamicType=(item as ItemNodeType).node_type||result?.node_type||''
 return {
  key:identity(item),item,nodeName:item.display_name||result?.display_name||item.node_key,
  airportName:airportNames[item.profile_id]||item.profile_id,nodeType:dynamicType||'未知协议',
  status,executed,attemptedSamples,responseSamples,firstMedianMs,stableMs,sites,
  skipReason:status==='skipped'?skipReason(item):undefined,suspected:false,suspectedReasons:[]
 }
}

function markSuspected(rows:LatencyReportRow[]):void{
 const executed=rows.filter(r=>r.executed)
 for(const row of executed){
  const values=row.sites.map(s=>s.stableMs)
  if(values.length===4&&values.every((v):v is number=>finite(v)&&v>2000)){
   const mean=values.reduce((sum,value)=>sum+value,0)/values.length
   const spread=(Math.max(...values)-Math.min(...values))/mean
   if(mean>0&&spread<0.05){
    const meanRounded=Math.round(mean)
    const spreadPercent=(spread*100).toFixed(1)
    row.suspectedReasons.push(`四站都约 ${meanRounded} ms，站间差异 ${spreadPercent}%`)
   }
  }
 }

 const protocolGroups=new Map<string,LatencyReportRow[]>()
 for(const row of executed){
  if(!usableType(row.nodeType))continue
  const groupKey=`${row.airportName.toLowerCase()}\u0000${row.nodeType.toLowerCase()}`
  protocolGroups.set(groupKey,[...(protocolGroups.get(groupKey)||[]),row])
 }
 for(const group of protocolGroups.values()){
  if(!group.length||!group.every(row=>row.status==='no-response'))continue
  const airport=group[0].airportName.toLowerCase(),protocol=group[0].nodeType.toLowerCase()
  const otherProtocolResponds=executed.some(row=>row.airportName.toLowerCase()===airport&&row.nodeType.toLowerCase()!==protocol&&row.responseSamples>0)
  if(otherProtocolResponds)for(const row of group)row.suspectedReasons.push('同机场同协议的已执行节点均无响应，其他协议有响应')
 }
 for(const row of executed)row.suspected=row.suspectedReasons.length>0
}

function buildGroups(rows:LatencyReportRow[]):{groups:LatencyReportGroup[];issues:LatencyReportIssue[]}{
 const groupsByKey=new Map<string,LatencyReportGroup>()
 const issues:LatencyReportIssue[]=[]
 const add=(row:LatencyReportRow,kind:LatencyReportIssueKind,reason:string,detail:string)=>{
  const issue:LatencyReportIssue={kind,reason,detail,item:row.item,row}
  issues.push(issue)
  const key=`${kind}\u0000${reason}`
  let group=groupsByKey.get(key)
  if(!group){group={kind,reason,rows:[],issues:[]};groupsByKey.set(key,group)}
  if(!group.rows.some(existing=>existing.key===row.key))group.rows.push(row)
  group.issues.push(issue)
 }
 for(const row of rows){
  if(row.status==='slow')add(row,'slow','偏慢',`四站稳定中位 ${Math.round(row.stableMs!)} ms（阈值 1,500 ms）`)
  if(row.status==='unstable')add(row,'unstable','不稳定',`响应 ${row.responseSamples}/${row.attemptedSamples}，少于 5/6`)
  if(row.status==='no-response')add(row,'no-response',failureReason(row),'已执行采样，但四站均未取得响应')
  if(row.suspected)add(row,'suspected','疑似检测问题',row.suspectedReasons.join('；'))
 }
 const skipped=rows.filter(row=>row.status==='skipped')
 if(skipped.length)groupsByKey.set('skipped\u0000未执行',{kind:'skipped',reason:'未执行',rows:skipped,issues:[]})
 const order:Record<LatencyReportGroup['kind'],number>={'no-response':0,suspected:1,unstable:2,slow:3,skipped:4}
 return {groups:[...groupsByKey.values()].sort((a,b)=>order[a.kind]-order[b.kind]||a.reason.localeCompare(b.reason,'zh-CN')),issues}
}

function latestTimestamp(items:BatchItem[]):string|undefined{
 const values=items.map(item=>item.result?.finished_at||'').filter(Boolean).sort((a,b)=>Date.parse(b)-Date.parse(a))
 return values[0]
}

export function buildLatencyReport(batch:LatencyBatch,airportNames:Record<string,string>={}):LatencyReport{
 const rows=(batch.items||[]).map(item=>makeRow(item,airportNames,batch.target_id))
 markSuspected(rows)
 const statusOrder:Record<LatencyReportStatus,number>={usable:0,slow:1,unstable:2,'no-response':3,skipped:4}
 rows.sort((a,b)=>statusOrder[a.status]-statusOrder[b.status]||(a.stableMs??Infinity)-(b.stableMs??Infinity)||a.nodeName.localeCompare(b.nodeName,'zh-CN'))
 const counts:LatencyReportCounts={
  total:rows.length,executed:rows.filter(row=>row.executed).length,
  usable:rows.filter(row=>row.status==='usable').length,
  slow:rows.filter(row=>row.status==='slow').length,
  unstable:rows.filter(row=>row.status==='unstable').length,
  noResponse:rows.filter(row=>row.status==='no-response').length,
  suspected:rows.filter(row=>row.suspected).length,
  skipped:rows.filter(row=>row.status==='skipped').length
 }
 const finishedAt=latestTimestamp(batch.items||[])
 const requestedAt=batch.requested_at
 const start=Date.parse(requestedAt),finish=finishedAt?Date.parse(finishedAt):NaN
 const durationMs=Number.isFinite(start)&&Number.isFinite(finish)&&finish>=start?finish-start:null
 const conditions=batch as BatchConditions
 const airportList=[...new Set(rows.map(row=>row.airportName).filter(Boolean))]
 const grouped=buildGroups(rows)
 return {
  rows,counts,
  summary:{requestedAt,finishedAt,durationMs,nodeCount:rows.length,airportNames:airportList,targetId:batch.target_id,
   timeoutSeconds:finite(conditions.timeout_seconds)&&conditions.timeout_seconds>0?conditions.timeout_seconds:null,
   sampleCountPerSite:observedSampleCount(batch)||null,
   siteCount:targets.length,
   firstMedianMs:inspectorMedian(rows.flatMap(row=>row.firstMedianMs===null?[]:[row.firstMedianMs])),
   stableMedianMs:inspectorMedian(rows.flatMap(row=>row.stableMs===null?[]:[row.stableMs]))},
  fastest5:rows.filter(row=>row.stableMs!==null).slice().sort((a,b)=>a.stableMs!-b.stableMs!||a.nodeName.localeCompare(b.nodeName,'zh-CN')).slice(0,5),
  groups:grouped.groups,issues:grouped.issues,
  usableItems:rows.filter(row=>row.status==='usable').map(row=>row.item)
 }
}

function batchConditions(batch:LatencyBatch):{target:string;timeout:number;sampleCount:number}|undefined{
 const extended=batch as BatchConditions
 const target=batch.target_id?.trim()
 const timeout=extended.timeout_seconds,sampleCount=observedSampleCount(batch)
 if(!target||target==='unknown'||target==='unverified'||!finite(timeout)||timeout<=0||!finite(sampleCount)||sampleCount<=0)return undefined
 return {target,timeout,sampleCount}
}

function sameConditions(a:LatencyBatch,b:LatencyBatch):boolean{
 const current=batchConditions(a),previous=batchConditions(b)
 return !!current&&!!previous&&current.target===previous.target&&current.timeout===previous.timeout&&current.sampleCount===previous.sampleCount
}

export function compareLatencyReports(batch:LatencyBatch,current:LatencyReport,previous:LatencyBatch[]=[]):LatencyReportComparison{
 const empty:LatencyReportComparison={comparableNodes:0,newlyFailed:0,recovered:0,stableMedianDeltaMs:null}
 const currentAt=Date.parse(batch.requested_at)
 if(!Number.isFinite(currentAt))return {...empty,reason:'本轮时间未知，暂不比较'}
 if(!batchConditions(batch))return {...empty,reason:'目标、超时或采样次数未完整记录，暂不比较'}
 const candidates=previous.filter(candidate=>candidate.batch_id!==batch.batch_id&&Date.parse(candidate.requested_at)<currentAt&&sameConditions(batch,candidate))
  .slice().sort((a,b)=>Date.parse(b.requested_at)-Date.parse(a.requested_at))
 if(!candidates.length)return {...empty,reason:previous.length?'没有更早的同条件批次':'没有更早的同条件记录'}
 const candidateReports=candidates.map(candidate=>({batch:candidate,report:buildLatencyReport(candidate)}))
 let newlyFailed=0,recovered=0,latestPreviousAt:string|undefined
 const currentStable:number[]=[],previousStable:number[]=[]
 let comparableNodes=0
 for(const row of current.rows){
  if(!row.executed)continue
  let oldRow:LatencyReportRow|undefined,oldBatch:LatencyBatch|undefined
  for(const candidate of candidateReports){
   const found=candidate.report.rows.find(old=>old.key===row.key&&old.executed)
   if(found){oldRow=found;oldBatch=candidate.batch;break}
  }
  if(!oldRow||!oldBatch)continue
  comparableNodes++
  if(!latestPreviousAt||Date.parse(oldBatch.requested_at)>Date.parse(latestPreviousAt))latestPreviousAt=oldBatch.requested_at
  const previousFailed=oldRow.status==='no-response'
  const currentFailed=row.status==='no-response'
  if(oldRow.responseSamples>0&&currentFailed)newlyFailed++
  if(previousFailed&&row.responseSamples>0)recovered++
  if(row.stableMs!==null&&oldRow.stableMs!==null){currentStable.push(row.stableMs);previousStable.push(oldRow.stableMs)}
 }
 if(!comparableNodes)return {...empty,reason:'没有同一节点、同一配置的已执行记录可比'}
 const currentMedian=inspectorMedian(currentStable),previousMedian=inspectorMedian(previousStable)
 return {comparableNodes,newlyFailed,recovered,stableMedianDeltaMs:currentMedian===null||previousMedian===null?null:currentMedian-previousMedian,latestPreviousAt}
}

export function latencyReportText(batch:LatencyBatch){const c=buildLatencyReport(batch).counts;return `${c.usable} 能用 · ${c.slow} 偏慢 · ${c.unstable} 不稳定 · ${c.noResponse} 无响应${c.skipped?` · ${c.skipped} 未执行`:''}`}

function observedSampleCount(batch:LatencyBatch){if(batch.sample_count)return batch.sample_count;return Math.max(0,...(batch.items||[]).flatMap(i=>{const counts=new Map<string,number>();for(const s of i.result?.samples||[])counts.set(s.target||'',1+(counts.get(s.target||'')||0));return [...counts.values()]}))}
