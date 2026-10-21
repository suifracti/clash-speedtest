import {api} from './api'
import {activeLatencyTargets} from './latencyTargets'
import {manualLatencyPoint,monitorLatencyPoints,type TrendPoint} from './presentation'
import {inspectorMedian,inspectorSamples,stableLatency} from './inspectorLatency'
import {scope,type NodeOption,type LatencyTest,type MonitorSample} from './domain'
export interface LatencySeries {id:string;name:string;points:TrendPoint[];context:string}
export function latencyRounds(series:LatencySeries[]){
 const rounds=new Map<string,{id:string;time:string}>()
 for(const s of series)for(const p of s.points)if(!p.id.startsWith('planned:')&&inspectorSamples(p,s.id).length)rounds.set(p.id,{id:p.id,time:p.time})
 return [...rounds.values()].map(r=>{
  const selected=series.map(s=>({...s,points:s.points.filter(p=>p.id===r.id)})),samples=selected.flatMap(s=>s.points.flatMap(p=>inspectorSamples(p,s.id)))
  const failedSites=selected.flatMap(s=>{const list=s.points.flatMap(p=>inspectorSamples(p,s.id)),failures=list.filter(p=>p.value===null).length;return failures?[{name:s.name,failures,total:list.length}]:[]})
  return {...r,series:selected,stable:stableLatency(selected),first:inspectorMedian(selected.flatMap(s=>{const v=s.points[0]?inspectorSamples(s.points[0],s.id)[0]?.value:null;return v==null?[]:[v]})),responses:samples.filter(s=>s.value!==null).length,total:samples.length,failedSites,failureSummary:failedSites.map(s=>s.failures===s.total?s.name+' 全失败':s.name+' '+s.failures+' 次无响应').join(' · ')}
 }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
export function latencyHistoryStats(series:LatencySeries[]){
 const rounds=latencyRounds(series),values=rounds.flatMap(r=>r.stable===null?[]:[r.stable]).sort((a,b)=>a-b)
 const percentile=(p:number)=>{const x=(values.length-1)*p,i=Math.floor(x);return values[i]+(values[Math.ceil(x)]-values[i])*(x-i)}
 const responses=rounds.reduce((n,r)=>n+r.responses,0),samples=rounds.reduce((n,r)=>n+r.total,0),failed=rounds.filter(r=>r.responses<r.total),last=failed.at(-1)
 return {rounds,responses,samples,total:rounds.length,failedRounds:failed.length,responseRate:samples?responses/samples*100:null,median:inspectorMedian(values),volatility:rounds.length>=5&&values.length?percentile(.9)-percentile(.1):null,lastFailure:last?{time:last.time,detail:last.failedSites.map(s=>s.name+' '+s.failures+' 次无响应').join(' · ')}:null}
}
// Read saved records only, following the existing per-node history cursors.
export async function readLatencyHistory(node:NodeOption,hours:number):Promise<LatencySeries[]>{
 const until=new Date().toISOString(),since=new Date(Date.parse(until)-hours*3600000).toISOString()
 const [tests,monitor]=await Promise.all([(async()=>{const all:LatencyTest[]=[];let before={};for(;;){const page=await api.latencyHistory(node,{since,until,limit:100,...before});all.push(...(page.tests||[]));if(!page.has_more)break;const last=page.tests!.at(-1)!;before={before_finished_at:last.finished_at,before_attempt_id:last.attempt_id}}return all})(),(async()=>{const all:MonitorSample[]=[];let cursor:string|undefined;for(;;){const page=await api.monitorSamples({...scope(node),since,until,probe_type:'rtt',limit:100,order_desc:true,cursor});all.push(...(page.items||[]));if(!page.has_more)break;cursor=page.next_cursor}return all})()])
 return activeLatencyTargets.map(t=>({...t,context:'',points:[...tests.map(test=>manualLatencyPoint(test,t.id)),...monitorLatencyPoints(monitor,t.id)].sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))}))
}
