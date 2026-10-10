import {api} from './api'
import {downloadSpeed,median,scope,type LatencyTest,type MonitorSample,type NodeOption,type Attempt} from './domain'
import {latencyTargetId} from './latencyTargets'
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
export function qualityGroups(tests:LatencyTest[],monitor:MonitorSample[],target:string):QualityGroup[]{
 const groups=new Map<string,{g:QualityGroup;values:number[]}>(),seen=new Set<string>()
 function add(id:string,source:string,method:string,url:string,condition:unknown,comparable:boolean,timestamp:string,success:boolean,value:number,error=''){
  if(seen.has(id)||latencyTargetId(url)!==target)return;seen.add(id)
  const conditionText=JSON.stringify(condition),k=JSON.stringify([source,method,url,condition]),entry=groups.get(k)||{g:{key:k,source,method,target:url,condition:conditionText,comparable,total:0,success:0,excluded:0,p50:null,p95:null,jitter:null,successRate:null,lastAt:'',insufficient:true},values:[]};groups.set(k,entry)
  if(!success&&excludedError(error)){entry.g.excluded++;return}
  const g=entry.g;g.total++;if(!g.lastAt||observationTime(timestamp)>observationTime(g.lastAt))g.lastAt=timestamp
  if(success&&Number.isFinite(value)&&value>=0){g.success++;entry.values.push(value)}
 }
 for(const t of tests){const p=t.network_path;const condition=[t.method_version??'unknown',t.node_type||'unknown',p?.method||'unknown',p?.interface||'unknown',p?.dns_mode||'unknown',p?.socket_bind_verified??'unknown',p?.dns_bind_verified??'unknown',p?.address_family||'unknown',p?.address_source||'unknown'];const comparable=!!t.method&&t.method_version!==undefined&&!!p?.method&&!!p.interface&&!!p.dns_mode&&p.socket_bind_verified===true&&p.dns_bind_verified===true&&!!p.address_family&&p.address_family!=='unknown'&&!!p.address_source&&p.address_source!=='unknown'
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
