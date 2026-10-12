import {api} from './api'
import {downloadSpeed,key,median,scope,type NodeScope,type LatencyTest,type MonitorSample,type NodeOption,type Attempt} from './domain'
import {latencyTargets,latencyTargetId} from './latencyTargets'
import {servicePassed} from './sharedResults'
export interface QualityGroup {key:string;source:string;method:string;target:string;condition:string;comparable:boolean;total:number;success:number;excluded:number;p50:number|null;p95:number|null;jitter:number|null;successRate:number|null;lastAt:string;insufficient:boolean}
export type QualityReadKind='latency'|'monitor'|'download'|'service'
export interface QualityReadStatus {complete:boolean;error?:string}
export interface QualityEvidence {tests:LatencyTest[];monitor:MonitorSample[];downloads:Attempt[];services:Attempt[];reads:Record<QualityReadKind,QualityReadStatus>;complete:boolean;since:string;until:string}
// Actual observation time precedes outer persistence time and round request time.
// Parse offsets numerically; lexical order is not chronological across time zones.
export function attemptObservedAt(a:Attempt){return [a.result?.finished_at,a.finished_at,a.started_at,a.requested_at].find(v=>v&&!v.startsWith('0001')&&Number.isFinite(Date.parse(v)))||''}
function newestAttempts(records:Attempt[]){return [...records].sort((a,b)=>(Date.parse(attemptObservedAt(b))||0)-(Date.parse(attemptObservedAt(a))||0)||b.attempt_id.localeCompare(a.attempt_id))}
export function latestServices(records:Attempt[]){const seen=new Set<string>();return newestAttempts(records).filter(a=>{const id=a.service_id||a.rule.service_id;if(!id||seen.has(id))return false;seen.add(id);return true})}
export function downloadEvidence(records:Attempt[]){const ordered=newestAttempts(records);return {latest:ordered[0],valid:ordered.find(a=>downloadSpeed(a)!==null)}}
export function latencyReadComplete(e:QualityEvidence,g?:QualityGroup){return !!g&&e.reads[g.source==='monitor'?'monitor':'latency'].complete}
export function qualityMatchesGate(e:QualityEvidence|undefined,g:QualityGroup|undefined,gate:string,until:string){
 if(gate==='all')return true;if(!e)return false
 if(gate==='download')return e.reads.download.complete&&!!downloadEvidence(e.downloads).valid
 if(gate==='service')return e.reads.service.complete&&latestServices(e.services).some(a=>{
  const at=attemptObservedAt(a),id=a.service_id||a.rule.service_id
  // Undated or conflicting simultaneous conclusions cannot prove current success.
  return !!at&&servicePassed(a)&&e.services.filter(v=>(v.service_id||v.rule.service_id)===id&&Date.parse(attemptObservedAt(v))===Date.parse(at)).every(servicePassed)
 })
 if(!latencyReadComplete(e,g))return false
 return gate==='samples'?g!.success>=5:gate==='response'?g!.success>=5&&(g!.successRate||0)>=90:gate==='fresh'?Date.parse(until)-Date.parse(g!.lastAt)>=0&&Date.parse(until)-Date.parse(g!.lastAt)<=86400000:false
}
const observationTime=(raw:string)=>{const at=Date.parse(raw);return raw&&!raw.startsWith('0001')&&Number.isFinite(at)?at:Number.NEGATIVE_INFINITY}
const excludedError=(error='')=>/cancel(?:led|ed)|context canceled|未执行|not.executed|local.physical|local.path|budget.exhausted|取消|物理出口未确认/i.test(error)
function latencyConditions(t:LatencyTest){const p=t.network_path;return [t.method_version??'unknown',t.node_type||'unknown',p?.method||'unknown',p?.interface||'unknown',p?.dns_mode||'unknown',p?.socket_bind_verified??'unknown',p?.dns_bind_verified??'unknown',p?.address_family||'unknown',p?.address_source||'unknown']}
const qualityGroupKey=(source:string,method:string,url:string,condition:unknown)=>JSON.stringify([source,method,url,condition])
export function qualityGroups(tests:LatencyTest[],monitor:MonitorSample[],target:string):QualityGroup[]{
 const groups=new Map<string,{g:QualityGroup;values:number[]}>(),seen=new Set<string>()
 function add(id:string,source:string,method:string,url:string,condition:unknown,comparable:boolean,timestamp:string,success:boolean,value:number,error=''){
  if(seen.has(id)||latencyTargetId(url)!==target)return;seen.add(id)
  const conditionText=JSON.stringify(condition),k=qualityGroupKey(source,method,url,condition),entry=groups.get(k)||{g:{key:k,source,method,target:url,condition:conditionText,comparable,total:0,success:0,excluded:0,p50:null,p95:null,jitter:null,successRate:null,lastAt:'',insufficient:true},values:[]};groups.set(k,entry)
  if(!success&&excludedError(error)){entry.g.excluded++;return}
  const g=entry.g;g.total++;if(!g.lastAt||observationTime(timestamp)>observationTime(g.lastAt))g.lastAt=timestamp
  if(success&&Number.isFinite(value)&&value>=0){g.success++;entry.values.push(value)}
 }
 for(const t of tests){const p=t.network_path;const condition=latencyConditions(t);const comparable=!!t.method&&t.method_version!==undefined&&!!p?.method&&!!p.interface&&!!p.dns_mode&&p.socket_bind_verified===true&&p.dns_bind_verified===true&&!!p.address_family&&p.address_family!=='unknown'&&!!p.address_source&&p.address_source!=='unknown'
  for(const s of t.samples||[])add(t.attempt_id+':'+s.seq,t.source||'workbench',t.method||'旧方法未记录',s.target||t.target||'',condition,comparable,s.timestamp,s.success,s.latency_ms,s.error)
 }
 for(const s of monitor)if(s.probe_type==='rtt')add('monitor:'+s.sample_id,'monitor','旧监测 RTT（条件未记录）',s.target,['legacy-unverified'],false,s.timestamp,s.success,s.latency/1e6,s.error_class+' '+(s.error_detail||''))
 for(const {g,values} of groups.values()){values.sort((a,b)=>a-b);g.p50=median(values);g.p95=values.length?values[Math.ceil(values.length*.95)-1]:null;const mean=values.length?values.reduce((a,b)=>a+b,0)/values.length:0;g.jitter=values.length>1?Math.sqrt(values.reduce((sum,v)=>sum+(v-mean)**2,0)/values.length):null;g.successRate=g.total?g.success/g.total*100:null;g.insufficient=g.total<5||g.success<5}
 return [...groups.values()].map(e=>e.g).filter(g=>g.total||g.excluded).sort((a,b)=>observationTime(b.lastAt)-observationTime(a.lastAt)||a.key.localeCompare(b.key))
}
interface Readers {latency:typeof api.latencyHistory;monitor:typeof api.monitorSamples;attempts:typeof api.attemptHistory}
// Bounded local database reads only. Never dispatches a measurement. A cap or
// malformed/stalled cursor makes evidence incomplete; it never implies success.
export async function readQuality(node:NodeOption,since:string,until:string,readers:Readers={latency:api.latencyHistory,monitor:api.monitorSamples,attempts:api.attemptHistory}):Promise<QualityEvidence>{
 const evidence:QualityEvidence={tests:[],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since,until}
 async function read(kind:QualityReadKind,operation:()=>Promise<void>){try{await operation()}catch(error){evidence.reads[kind]={complete:false,error:error instanceof Error?error.message:String(error)}}}
 function incomplete(kind:QualityReadKind,reason:string){evidence.reads[kind]={complete:false,error:reason}}
 await read('latency',async()=>{let before:Record<string,unknown>={},previous=''
  for(let page=0;page<50;page++){
   const r=await readers.latency(node,{since,until,limit:100,...before});const tests=r.tests||[];evidence.tests.push(...tests)
   if(!r.has_more){if(r.complete===false)incomplete('latency','接口报告历史不完整');break}
   const last=tests.at(-1),cursor=last?.finished_at+'|'+last?.attempt_id
   if(!last?.finished_at||!last.attempt_id||cursor===previous||page===49){incomplete('latency','历史游标缺失、停滞或达到读取上限');break}previous=cursor;before={before_finished_at:last.finished_at,before_attempt_id:last.attempt_id}
  }
 })
 await read('monitor',async()=>{let cursor='',previous='';for(let page=0;page<50;page++){
  const r=await readers.monitor({...scope(node),since,until,probe_type:'rtt',limit:100,order_desc:true,...(cursor?{cursor}:{})});evidence.monitor.push(...(r.items||[]));if(!r.has_more)break
  if(!r.next_cursor||r.next_cursor===previous||page===49){incomplete('monitor','历史游标缺失、停滞或达到读取上限');break}previous=r.next_cursor;cursor=r.next_cursor
 }})
 for(const kind of ['download','service'] as const)await read(kind,async()=>{let before:Record<string,unknown>={},previous='';const records=kind==='download'?evidence.downloads:evidence.services
  for(let page=0;page<50;page++){
   const r=await readers.attempts(kind,node,{since,until,limit:100,...before});const list=r.attempts||[];records.push(...list)
   if(!r.has_more){if(r.complete===false)incomplete(kind,'接口报告历史不完整');break}
   const last=list.at(-1),at=last?.finished_at||last?.started_at||last?.requested_at,c=at+'|'+last?.attempt_id
   if(!at||!last?.attempt_id||c===previous||page===49){incomplete(kind,'历史游标缺失、停滞或达到读取上限');break}previous=c;before={before_at:at,before_attempt_id:last.attempt_id}
  }
 })
 // Aggregate is only a summary. Filtering/ranking consults the relevant stream.
 evidence.complete=Object.values(evidence.reads).every(status=>status.complete)
 return evidence
}


export type SiteQualityMetric='p50'|'p95'|'rate'
export type SiteQualityReason='unknown_identity'|'invalid_window'|'unmeasured'|'incomplete_read'|'unknown_observation_time'|'unknown_conditions'|'insufficient_samples'|'insufficient_p95_samples'|'stale'|'ambiguous_latest_group'|'invalid_samples'|'latest_unobserved_attempt'|'conflicting_observations'
export interface SiteQualityObservations {failed:number;cancelled:number;notExecuted:number;sourceLimited:number}
export interface SiteQualitySummary {
 targetId:string;nodeId:string;since:string;until:string;groups:QualityGroup[];group?:QualityGroup
 total:number;success:number;excluded:number;p50:number|null;p95:number|null;successRate:number|null;lastAt:string
 state:'unmeasured'|'responded'|'partial'|'failed'|'cancelled'|'not_executed'|'source_limited'
 observations:SiteQualityObservations;attemptStates:{cancelled:number;notExecuted:number};unplacedSamples:number;invalidSamples:number;conflictingSamples:number
 readComplete:boolean;readError?:string;gates:{samples:boolean;response:boolean;fresh:boolean}
 rankable:Record<SiteQualityMetric,boolean>;reasons:SiteQualityReason[]
}
const cancelledError=(error='')=>/cancel(?:led|ed)|context canceled|取消/i.test(error)
const sourceLimitedError=(error='')=>/\bHTTP\s*429\b|\brate[ _-]?limit(?:ed|ing)?\b|too many requests|限流/i.test(error)
function knownQualityConditions(g:QualityGroup){
 const c=JSON.parse(g.condition) as unknown[]
 const known=(v:unknown)=>typeof v==='string'&&!!v&&!['unknown','unverified'].includes(v.toLowerCase())
 // Reuse the group key; never substitute a route/method/family to make it rankable.
 return g.comparable&&known(g.source)&&known(g.method)&&latencyTargetId(g.target)!==undefined&&/^https?:\/\//.test(g.target)&&
  Number.isInteger(c[0])&&(c[0] as number)>0&&[c[1],c[2],c[3],c[4],c[7],c[8]].every(known)&&c[5]===true&&c[6]===true
}

/** Pure per-node, per-site evidence. Pass a fixed, fully bounded read window.
 * Latest current-method condition wins, even if unrankable; older/legacy groups
 * remain visible in groups. No cross-site score or hidden fallback to nicer data.
 */
export function multiSiteQuality(node:NodeScope,e:QualityEvidence):SiteQualitySummary[]{
 const nodeId=key(node),identityKnown=!!node.profile_id&&!!node.node_identity_key&&!!node.config_revision_key
 const start=observationTime(e.since),end=observationTime(e.until),windowValid=Number.isFinite(start)&&Number.isFinite(end)&&start<=end
 const tests=e.tests.filter(t=>key(t)===nodeId),monitor=e.monitor.filter(s=>key(s)===nodeId)
 const fingerprints=new Map<string,string>(),conflicts=new Set<string>()
 function remember(id:string,payload:unknown){
  const fingerprint=JSON.stringify(payload),previous=fingerprints.get(id)
  if(previous!==undefined&&previous!==fingerprint)conflicts.add(id)
  else fingerprints.set(id,fingerprint)
 }
 // Identical repeated pages are duplicates. Contradictory copies of one saved
 // observation are not two samples and must not select a winner by input order.
 for(const t of tests)for(const sample of t.samples||[]){
  remember('latency:'+t.attempt_id+':'+sample.seq,[qualityGroupKey(t.source||'workbench',t.method||'旧方法未记录',sample.target||t.target||'',latencyConditions(t)),observationTime(sample.timestamp),sample.success,sample.latency_ms,sample.error||''])
 }
 for(const sample of monitor)if(sample.probe_type==='rtt')remember('monitor:'+sample.sample_id,[sample.target,observationTime(sample.timestamp),sample.success,sample.latency,sample.error_class||'',sample.error_detail||''])
 return latencyTargets.map(target=>{
  let unplacedSamples=0,invalidSamples=0
  const conflictingIDs=new Set<string>()
  const inWindow=(timestamp:string)=>{const at=observationTime(timestamp);if(!Number.isFinite(at)){unplacedSamples++;return false}return windowValid&&at>=start&&at<=end}
  const scopedTests=tests.map(t=>({...t,samples:(t.samples||[]).filter(s=>{
   if(latencyTargetId(s.target||t.target||'')!==target.id||!inWindow(s.timestamp))return false
   const id='latency:'+t.attempt_id+':'+s.seq
   if(conflicts.has(id)){conflictingIDs.add(id);return false}
   if(s.success&&(!Number.isFinite(s.latency_ms)||s.latency_ms<0)){invalidSamples++;return false}
   return true
  })}))
  const scopedMonitor=monitor.filter(s=>{
   if(s.probe_type!=='rtt'||latencyTargetId(s.target)!==target.id||!inWindow(s.timestamp))return false
   const id='monitor:'+s.sample_id
   if(conflicts.has(id)){conflictingIDs.add(id);return false}
   if(s.success&&(!Number.isFinite(s.latency)||s.latency<0)){invalidSamples++;return false}
   return true
  })
  const groups=qualityGroups(scopedTests,scopedMonitor,target.id)
  const details=new Map<string,{at:number;observations:SiteQualityObservations}>(),seen=new Set<string>(),partialGroups=new Set<string>()
  const byKey=new Map(groups.map(g=>[g.key,g]))
  function observe(group:QualityGroup|undefined,id:string,timestamp:string,success:boolean,error=''){
   if(!group||seen.has(id))return;seen.add(id)
   const d=details.get(group.key)||{at:Number.NEGATIVE_INFINITY,observations:{failed:0,cancelled:0,notExecuted:0,sourceLimited:0}}
   d.at=Math.max(d.at,observationTime(timestamp))
   if(sourceLimitedError(error))d.observations.sourceLimited++
   if(!success){if(cancelledError(error))d.observations.cancelled++;else if(excludedError(error))d.observations.notExecuted++;else d.observations.failed++}
   details.set(group.key,d)
  }
  for(const t of scopedTests)for(const s of t.samples){
   const group=byKey.get(qualityGroupKey(t.source||'workbench',t.method||'旧方法未记录',s.target||t.target||'',latencyConditions(t)))
   observe(group,t.attempt_id+':'+s.seq,s.timestamp,s.success,s.error)
   if(group&&t.status==='cancelled')partialGroups.add(group.key)
  }
  for(const s of scopedMonitor){
   const group=byKey.get(qualityGroupKey('monitor','旧监测 RTT（条件未记录）',s.target,['legacy-unverified']))
   observe(group,'monitor:'+s.sample_id,s.timestamp,s.success,s.error_class+' '+(s.error_detail||''))
  }
  // Excluded observations choose the latest condition, but do not refresh its
  // lastAt/response denominator. Prefer current measurements over legacy RTT.
  groups.sort((a,b)=>(details.get(b.key)?.at??Number.NEGATIVE_INFINITY)-(details.get(a.key)?.at??Number.NEGATIVE_INFINITY)||a.key.localeCompare(b.key))
  const currentGroups=groups.filter(g=>g.source!=='monitor'),candidates=currentGroups.length?currentGroups:groups
  const observed=candidates[0]
  const ambiguous=!!observed&&candidates.some(g=>g.key!==observed.key&&details.get(g.key)!.at===details.get(observed.key)!.at)
  let latestEmpty:{test:LatencyTest;at:number}|undefined
  const attemptStates={cancelled:0,notExecuted:0},attemptsSeen=new Set<string>()
  for(const t of tests){
   if((latencyTargetId(t.target)!==target.id&&!(t.samples||[]).some(s=>latencyTargetId(s.target)===target.id))||attemptsSeen.has(t.attempt_id))continue
   const at=[t.finished_at,t.requested_at].map(observationTime).find(Number.isFinite)??Number.NEGATIVE_INFINITY
   if(!windowValid||at<start||at>end||!Number.isFinite(at))continue
   attemptsSeen.add(t.attempt_id)
   if(t.status==='cancelled')attemptStates.cancelled++;else if(t.status==='not_executed')attemptStates.notExecuted++
   const hasSiteSample=(t.samples||[]).some(s=>latencyTargetId(s.target||t.target||'')===target.id)
   if(!hasSiteSample&&(!latestEmpty||at>latestEmpty.at))latestEmpty={test:t,at}
  }
  // A newer attempt with no observation cannot inherit the old path's metrics.
  // Keep old groups as history, without inventing a zero or a failed sample.
  const unobservedLatest=!!latestEmpty&&(!observed||latestEmpty.at>=details.get(observed.key)!.at)
  const group=unobservedLatest?undefined:observed
  const observations=group?details.get(group.key)!.observations:{failed:0,cancelled:0,notExecuted:0,sourceLimited:0}
  const readComplete=group?latencyReadComplete(e,group):unobservedLatest?e.reads.latency.complete:e.reads.latency.complete&&e.reads.monitor.complete
  const readError=group?e.reads[group.source==='monitor'?'monitor':'latency'].error:unobservedLatest?e.reads.latency.error:e.reads.latency.error||e.reads.monitor.error
  const gates={samples:qualityMatchesGate(e,group,'samples',e.until),response:qualityMatchesGate(e,group,'response',e.until),fresh:qualityMatchesGate(e,group,'fresh',e.until)}
  const reasons:SiteQualityReason[]=[]
  if(!identityKnown)reasons.push('unknown_identity')
  if(!windowValid)reasons.push('invalid_window')
  if(!group)reasons.push('unmeasured')
  if(unobservedLatest)reasons.push('latest_unobserved_attempt')
  if(!readComplete)reasons.push('incomplete_read')
  if(unplacedSamples)reasons.push('unknown_observation_time')
  if(invalidSamples)reasons.push('invalid_samples')
  // Keep conflicts across saved streams diagnostic, but admit the selected
  // stream independently, just as latencyReadComplete does for current HTTP.
  if([...conflictingIDs].some(id=>id.startsWith(group?.source==='monitor'?'monitor:':'latency:')))reasons.push('conflicting_observations')
  if(ambiguous)reasons.push('ambiguous_latest_group')
  if(group&&!knownQualityConditions(group))reasons.push('unknown_conditions')
  if(group&&!gates.samples)reasons.push('insufficient_samples')
  if(group&&group.success<20)reasons.push('insufficient_p95_samples')
  if(group&&!gates.fresh)reasons.push('stale')
  const baseRankable=reasons.every(reason=>reason==='insufficient_p95_samples')&&!!group&&group.p50!==null
  const state:SiteQualitySummary['state']=!group?(unobservedLatest&&latestEmpty?.test.status==='cancelled'?'cancelled':unobservedLatest&&latestEmpty?.test.status==='not_executed'?'not_executed':'unmeasured'):
   group.success>0?(observations.failed||group.excluded||partialGroups.has(group.key)?'partial':observations.sourceLimited===group.total?'source_limited':'responded'):
   observations.sourceLimited?'source_limited':group.total?'failed':observations.cancelled?'cancelled':observations.notExecuted?'not_executed':'unmeasured'
  return {targetId:target.id,nodeId,since:e.since,until:e.until,groups,group,total:group?.total||0,success:group?.success||0,excluded:group?.excluded||0,
   p50:group?.p50??null,p95:group?.p95??null,successRate:group?.successRate??null,lastAt:group?.lastAt||'',state,observations,attemptStates,unplacedSamples,invalidSamples,conflictingSamples:conflictingIDs.size,
   readComplete,readError,gates,reasons,rankable:{p50:baseRankable,p95:baseRankable&&group!.success>=20&&group!.p95!==null,rate:baseRankable&&group!.successRate!==null}}
 })
}

/** Negative order means a ranks before b. Null is deliberately NOT a sort tie:
 * keep the existing order/show reasons when conditions cannot be compared.
 */
export function compareSiteQuality(a:SiteQualitySummary,b:SiteQualitySummary,metric:SiteQualityMetric):{comparable:boolean;order:number|null;reasons:string[]}{
 const reasons:string[]=[]
 if(a.targetId!==b.targetId)reasons.push('target_mismatch')
 if(observationTime(a.since)!==observationTime(b.since)||observationTime(a.until)!==observationTime(b.until))reasons.push('window_mismatch')
 if(!a.group||!b.group||a.group.key!==b.group.key)reasons.push('condition_mismatch')
 for(const [side,summary] of [['a',a],['b',b]] as const)if(!summary.rankable[metric])reasons.push(...summary.reasons.filter(reason=>metric==='p95'||reason!=='insufficient_p95_samples').map(reason=>side+':'+reason))
 if(reasons.length||!a.rankable[metric]||!b.rankable[metric])return {comparable:false,order:null,reasons}
 const av=metric==='rate'?a.successRate!:a[metric]!,bv=metric==='rate'?b.successRate!:b[metric]!
 return {comparable:true,order:Math.sign(metric==='rate'?bv-av:av-bv),reasons:[]}
}
