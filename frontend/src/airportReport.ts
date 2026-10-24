import {key,type NodeOption} from './domain'
import type {QualityEvidence} from './nodeQuality'
import {shanghaiDay} from './usageDaily'
import {activeLatencyTargets} from './latencyTargets'
import {manualLatencyPoint,monitorLatencyPoints} from './presentation'
import {latencyHistoryStats} from './latencyHistory'
export function airportReport(nodes:NodeOption[],data:Record<string,QualityEvidence>){
 const rounds=new Map<string,{responses:number;samples:number}>(),failures=new Map<string,number>(),days=new Map<string,number[]>(),seen=new Set<string>()
 let incomplete=false
 function sample(id:string,round:string,at:string,success:boolean,ms:number,error=''){
  if(seen.has(id)||/cancel|not_executed|取消|未执行/i.test(error))return;seen.add(id)
  const count=rounds.get(round)||{responses:0,samples:0};count.samples++;if(success)count.responses++;rounds.set(round,count)
  if(success&&Number.isFinite(ms)){const day=shanghaiDay(new Date(at)),values=days.get(day)||[];values.push(ms);days.set(day,values)}
  else if(!success){const kind=/timeout|deadline|超时/i.test(error)?'超时':/dns|resolve|域名/i.test(error)?'DNS':/tls|certificate|证书/i.test(error)?'TLS':/refused|reset|connect|连接/i.test(error)?'连接失败':/HTTP/i.test(error)?'HTTP':'其他';failures.set(kind,(failures.get(kind)||0)+1)}
 }
 for(const node of nodes){const e=data[key(node)];if(!e){incomplete=true;continue};incomplete ||= !e.reads.latency.complete||!e.reads.monitor.complete
  for(const t of e.tests)for(const s of t.samples||[])sample(t.attempt_id+':'+s.seq+':'+s.target,t.attempt_id,s.timestamp,s.success,s.latency_ms,s.error)
  for(const s of e.monitor)if(s.probe_type==='rtt')sample(s.sample_id,key(node)+':'+(s.run_id||s.sample_id),s.timestamp,s.success,s.latency/1e6,s.error_class)
 }
 const list=[...rounds.values()],responses=list.reduce((n,r)=>n+r.responses,0),samples=list.reduce((n,r)=>n+r.samples,0),total=list.length
 return {total,responses,samples,rate:samples?responses/samples*100:null,failedRounds:list.filter(r=>r.responses<r.samples).length,incomplete,failures:[...failures].sort((a,b)=>b[1]-a[1]),trend:[...days].sort(([a],[b])=>a.localeCompare(b)).map(([day,values])=>({id:day,time:day+'T12:00:00+08:00',value:values.reduce((a,b)=>a+b,0)/values.length,tone:'good' as const,description:`${day} · ${values.length} 次响应样本`}))}
}

export function filterAirportNodes(nodes:NodeOption[],region:string,line:string,protocol:string){
 return nodes.filter(n=>(region==='all'||n.country_code===region)&&(line==='all'||n.display_name.includes(line))&&(protocol==='all'||n.type===protocol))
}
export function airportNodeStats(evidence?:QualityEvidence){
 return latencyHistoryStats(activeLatencyTargets.map(target=>({...target,context:'',points:[
  ...(evidence?.tests||[]).map(test=>manualLatencyPoint(test,target.id)),
  ...monitorLatencyPoints(evidence?.monitor||[],target.id),
 ]})))
}
