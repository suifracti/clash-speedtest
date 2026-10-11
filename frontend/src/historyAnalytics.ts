import type {MonitorSample,LatencyTest} from './domain'
export interface HistoryContext {since:string;until:string;hasMore:boolean;error?:string;loading?:boolean;profile?:string;identity?:string;target?:string}
export interface HistoryPoint {id:string;timestamp:string;at:number;profile:string;identity:string;revision:string;name:string;target:string;probe:string;tier:string;condition:string;comparable:boolean;round:string;success:boolean;value:number|null;excluded:boolean;error:string;raw:unknown;group:string}
export interface HistoryGroup {key:string;name:string;profile:string;identity:string;revision:string;target:string;probe:string;tier:string;condition:string;comparable:boolean;count:number;eligible:number;success:number;timeout:number;failed:number;excluded:number;invalidTiming:number;p50:number|null;p95:number|null;successRate:number|null;rounds:number;unknownRounds:number;days:number;firstAt:string;lastAt:string}
const known=(v:unknown)=>v!==undefined&&v!==null&&v!==''&&v!=='unknown'
function excluded(error:string){return /cancel(?:led|ed)|context canceled|not.executed|local.path|local.physical|config.error|invalid.request|budget.exhausted|rate.limit|\bhttp(?:[ _-]?status)?[ :_-]+429\b|取消|未执行|物理出口未确认/i.test(error)}
function point(s:MonitorSample):HistoryPoint{
 const metadata=s.metadata as Record<string,unknown>|undefined,error=[s.error_class,s.error_detail,metadata?.execution_status,metadata?.http_status===429?'HTTP status 429':''].filter(Boolean).join(' ')
 const value=s.probe_type==='rtt'?s.latency/1e6:s.probe_type==='ttfb'?s.ttfb/1e6:null
 return {id:'monitor:'+s.sample_id,timestamp:s.timestamp,at:Date.parse(s.timestamp),profile:s.profile_id,identity:s.node_identity_key||'',revision:s.config_revision_key||'',name:s.display_name_snapshot,target:s.target,probe:s.probe_type,tier:s.sampling_tier||'unknown',condition:'monitor: method/version/address-family/source/physical-egress unknown',comparable:false,round:s.run_id||'',success:s.success,value:value!==null&&Number.isFinite(value)&&value>=0?value:null,excluded:excluded(error),error,raw:s,group:''}
}
function latencyPoints(t:LatencyTest):HistoryPoint[]{
 const p=t.network_path,condition=JSON.stringify([t.source||'unknown',t.method||'unknown',t.method_version??'unknown',t.node_type||'unknown',p?.method||'unknown',p?.interface||'unknown',p?.interface_index??'unknown',p?.dns_mode||'unknown',p?.dns_bind_verified??'unknown',p?.socket_bind_verified??'unknown',p?.address_family||'unknown',p?.address_source||'unknown'])
 const comparable=[t.source,t.method,t.method_version,p?.method,p?.interface,p?.dns_mode,p?.address_family,p?.address_source].every(known)&&p?.dns_bind_verified===true&&p.socket_bind_verified===true
 return (t.samples||[]).map(s=>({id:'latency:'+t.attempt_id+':'+s.seq,timestamp:s.timestamp,at:Date.parse(s.timestamp),profile:t.profile_id,identity:t.node_identity_key||'',revision:t.config_revision_key||'',name:t.display_name,target:s.target||t.target||'',probe:'latency',tier:'manual',condition,comparable,round:t.attempt_id,success:s.success,value:Number.isFinite(s.latency_ms)&&s.latency_ms>=0?s.latency_ms:null,excluded:excluded((s.error||'')+' '+(t.status==='cancelled'||t.status==='not_executed'?t.status:'')),error:s.error||'',raw:{attempt:t,sample:s},group:''}))
}
// Statistics describe loaded evidence only. They never dispatch probes or infer a path from a current node.
export function analyzeHistory(samples:MonitorSample[],context:HistoryContext,tests:LatencyTest[]=[]){
 const all=[...samples.map(point),...tests.flatMap(latencyPoints)],seen=new Set<string>(),start=Date.parse(context.since),end=Date.parse(context.until)
 const points=all.filter(p=>{if(seen.has(p.id))return false;seen.add(p.id);return !p.timestamp.startsWith('0001')&&Number.isFinite(p.at)&&p.at>=start&&p.at<=end&&(!context.profile||p.profile===context.profile)&&(!context.identity||p.identity===context.identity)&&(!context.target||p.target===context.target)}).sort((a,b)=>a.at-b.at)
 const groups=new Map<string,HistoryGroup>(),values=new Map<string,number[]>(),rounds=new Map<string,Set<string>>(),days=new Map<string,Set<string>>()
 for(const p of points){
  // Missing identity is isolated by recorded raw node key, never joined to a current identity.
  const raw=p.raw as MonitorSample|{attempt:LatencyTest},nodeKey='attempt' in raw?raw.attempt.node_key:raw.node_key
  p.group=JSON.stringify([p.profile,p.identity||['unknown',nodeKey],p.revision,p.target,p.probe,p.tier,p.condition])
  let g=groups.get(p.group)
  if(!g){g={key:p.group,name:p.name,profile:p.profile,identity:p.identity,revision:p.revision,target:p.target,probe:p.probe,tier:p.tier,condition:p.condition,comparable:p.comparable&&!!p.identity&&!!p.revision,count:0,eligible:0,success:0,timeout:0,failed:0,excluded:0,invalidTiming:0,p50:null,p95:null,successRate:null,rounds:0,unknownRounds:0,days:0,firstAt:p.timestamp,lastAt:p.timestamp};groups.set(p.group,g);values.set(p.group,[]);rounds.set(p.group,new Set());days.set(p.group,new Set())}
  g.count++;g.lastAt=p.timestamp;days.get(p.group)!.add(new Date(p.at).toISOString().slice(0,10));if(p.round)rounds.get(p.group)!.add(p.round);else g.unknownRounds++
  if(p.excluded){g.excluded++;continue}
  g.eligible++;if(p.success){g.success++;if(p.value!==null)values.get(p.group)!.push(p.value);else g.invalidTiming++}else if(/timeout|timed.out|deadline.exceeded/i.test(p.error))g.timeout++;else g.failed++
 }
 for(const g of groups.values()){
  const v=values.get(g.key)!.sort((a,b)=>a-b),n=v.length
  g.p50=n?(n%2?v[Math.floor(n/2)]!:(v[n/2-1]!+v[n/2]!)/2):null;g.p95=n?v[Math.ceil(n*.95)-1]!:null
  g.successRate=g.eligible?g.success/g.eligible*100:null;g.rounds=rounds.get(g.key)!.size;g.days=days.get(g.key)!.size
 }
 return {groups:[...groups.values()],points,loaded:all.length,inWindow:points.length,discarded:all.length-points.length,complete:!context.hasMore&&!context.error&&!context.loading}
}
