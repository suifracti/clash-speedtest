import {key,type NodeOption} from './domain'
import type {QualityEvidence} from './nodeQuality'
import {shanghaiDay} from './usageDaily'
export function airportReport(nodes:NodeOption[],data:Record<string,QualityEvidence>){
 const rounds=new Map<string,boolean>(),failures=new Map<string,number>(),days=new Map<string,number[]>(),seen=new Set<string>()
 let incomplete=false
 function sample(id:string,round:string,at:string,success:boolean,ms:number,error=''){
  if(seen.has(id)||/cancel|not_executed|取消|未执行/i.test(error))return;seen.add(id)
  rounds.set(round,!!rounds.get(round)||success)
  if(success&&Number.isFinite(ms)){const day=shanghaiDay(new Date(at)),values=days.get(day)||[];values.push(ms);days.set(day,values)}
  else if(!success){const kind=/timeout|deadline|超时/i.test(error)?'超时':/dns|resolve|域名/i.test(error)?'DNS':/tls|certificate|证书/i.test(error)?'TLS':/refused|reset|connect|连接/i.test(error)?'连接失败':/HTTP/i.test(error)?'HTTP':'其他';failures.set(kind,(failures.get(kind)||0)+1)}
 }
 for(const node of nodes){const e=data[key(node)];if(!e){incomplete=true;continue};incomplete ||= !e.reads.latency.complete||!e.reads.monitor.complete
  for(const t of e.tests)for(const s of t.samples||[])sample(t.attempt_id+':'+s.seq+':'+s.target,t.attempt_id,s.timestamp,s.success,s.latency_ms,s.error)
  for(const s of e.monitor)if(s.probe_type==='rtt')sample(s.sample_id,key(node)+':'+(s.run_id||s.sample_id),s.timestamp,s.success,s.latency/1e6,s.error_class)
 }
 const available=[...rounds.values()].filter(Boolean).length,total=rounds.size
 return {available,total,rate:total?available/total*100:null,incomplete,failures:[...failures].sort((a,b)=>b[1]-a[1]),trend:[...days].sort(([a],[b])=>a.localeCompare(b)).map(([day,values])=>({id:day,time:day+'T12:00:00+08:00',value:values.reduce((a,b)=>a+b,0)/values.length,tone:'good' as const,description:`${day} · ${values.length} 次响应样本`}))}
}
