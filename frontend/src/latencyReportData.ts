import {api,request,mapLimited} from './api'
import {key,type LatencyBatch,type LatencyTest,type MonitorSample,type NodeOption,type BatchItem} from './domain'
export interface MonitorReportRun {run_id:string;started_at:string;scheduled_at:string;finished_at?:string;status:string;total_nodes:number;job_id:string}
export interface SavedLatencyReport {batch:LatencyBatch;monitor:boolean}
export async function readLatencyBatches(){const list=await api.batches();return mapLimited(list||[],6,async b=>b.items?b:api.batch(b.batch_id))}
export function monitorLatencyBatch(run:MonitorReportRun,samples:MonitorSample[],nodes:NodeOption[]):LatencyBatch {
 const grouped=new Map<string,MonitorSample[]>()
 for(const s of samples.filter(s=>s.probe_type==='rtt')){const k=key(s),list=grouped.get(k)||[];list.push(s);grouped.set(k,list)}
 const items:BatchItem[]=[...grouped.values()].map(list=>{
  list.sort((a,b)=>Date.parse(a.timestamp)-Date.parse(b.timestamp));const first=list[0]!,n=nodes.find(n=>key(n)===key(first)),good=list.filter(s=>s.success)
  const scope={profile_id:first.profile_id,node_key:first.node_key,node_identity_key:first.node_identity_key,config_revision_key:first.config_revision_key}
  const result:LatencyTest={...scope,attempt_id:run.run_id+':'+key(first),display_name:first.display_name_snapshot,node_type:n?.type||'未知协议',source:'monitor',method:'monitor_rtt',method_version:1,requested_at:run.started_at,finished_at:run.finished_at||list.at(-1)!.timestamp,status:good.length?'completed':'failed',latency_ms:good.length?good.reduce((v,s)=>v+s.latency/1e6,0)/good.length:0,total_samples:list.length,success_samples:good.length,failure_samples:list.length-good.length,persistence_state:'saved',samples:list.map((s,i)=>({seq:i+1,target:s.target,timestamp:s.timestamp,latency_ms:s.latency/1e6,success:s.success,error:s.error_detail||s.error_class}))}
  return {...scope,item_id:result.attempt_id,batch_id:run.run_id,display_name:result.display_name,execution_state:'completed',persistence_state:'saved',result}
 })
 return {batch_id:run.run_id,request_id:run.run_id,target_id:'monitor-rtt',state:run.status,requested_at:run.started_at||run.scheduled_at,item_count:items.length,items}
}
export async function readLatencyReports(nodes:NodeOption[]):Promise<SavedLatencyReport[]>{
 const [batches,runs]=await Promise.all([readLatencyBatches(),request<MonitorReportRun[]>('/api/monitor/runs?limit=50','GET',undefined,'QueryMonitorRuns',['',50])])
 const monitoring=await mapLimited(runs||[],6,async run=>{
  const samples:MonitorSample[]=[]
  for(let offset=0;;offset+=1000){const page=await request<MonitorSample[]>(`/api/monitor/samples?run_id=${encodeURIComponent(run.run_id)}&limit=1000&offset=${offset}`,'GET',undefined,'QueryMonitorSamples',[{RunID:run.run_id,Limit:1000,Offset:offset}]);samples.push(...(page||[]));if(!page||page.length<1000)break}
  return {batch:monitorLatencyBatch(run,samples,nodes),monitor:true}
 })
 return [...batches.map(batch=>({batch,monitor:false})),...monitoring.filter(r=>r.batch.items?.length)].sort((a,b)=>Date.parse(b.batch.requested_at)-Date.parse(a.batch.requested_at))
}
