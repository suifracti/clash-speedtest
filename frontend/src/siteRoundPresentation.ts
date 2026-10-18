import {key,type LatencyTest} from './domain'
import {latencyTargetId} from './latencyTargets'
import {multiSiteQuality,type SiteQualitySummary,type QualityEvidence} from './nodeQuality'
import {manualLatencyPoint} from './presentation'

// Display one saved attempt at a time. Window aggregates remain untouched for sorting.
export function siteRoundHistory(summary:SiteQualitySummary|undefined,tests:LatencyTest[]=[]){
 if(!summary)return []
 const seen=new Set<string>(),start=Date.parse(summary.since),end=Date.parse(summary.until)
 return tests.filter(t=>{
  if(key(t)!==summary.nodeId||seen.has(t.attempt_id))return false
  const samples=(t.samples||[]).filter(s=>latencyTargetId(s.target||t.target||'')===summary.targetId)
  if(!samples.length&&latencyTargetId(t.target)!==summary.targetId&&!t.target?.startsWith('multi://'))return false
  const at=Date.parse(t.finished_at||t.requested_at)
  if(!(at>=start&&at<=end))return false
  seen.add(t.attempt_id);return true
 }).sort((a,b)=>Date.parse(b.finished_at||b.requested_at)-Date.parse(a.finished_at||a.requested_at)||b.attempt_id.localeCompare(a.attempt_id)).map(test=>{
  const evidence:QualityEvidence={tests:[test],monitor:[],downloads:[],services:[],since:summary.since,until:summary.until,complete:summary.readComplete,reads:{latency:{complete:summary.readComplete,error:summary.readError},monitor:{complete:true},download:{complete:true},service:{complete:true}}}
  return {test,summary:multiSiteQuality(test,evidence).find(s=>s.targetId===summary.targetId)!,point:manualLatencyPoint(test,summary.targetId)}
 })
}
