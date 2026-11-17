import {expect,it} from 'vitest'
import {recommend} from '../recommendations'
import {key,type Attempt,type LatencyTest,type NodeOption,type ServiceRule} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
it('ranks continuous download ahead of an initial burst followed by a timeout stall',()=>{
 const nodes=['burst','continuous'].map(id=>({profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',display_name:id,profile_name:'P',type:'ss',country_code:'',country_flag:''}) satisfies NodeOption)
 const at='2026-11-18T00:00:00Z',mib=1048576
 const data=Object.fromEntries(nodes.map((node,i)=>{
  const download:Attempt={...node,attempt_id:node.node_key,request_id:node.node_key,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://download.example/file',method:'GET',rule_version:3,maximum_bytes:20*mib,maximum_duration_ns:15e9},result:{outcome:'time_limit',finished_at:at,bytes_read:(i?15:10)*mib,phase_timings_ns:{response_body:15e9},samples:i?[
   {elapsed_ns:5e9,cumulative_bytes:5*mib},{elapsed_ns:10e9,cumulative_bytes:10*mib},{elapsed_ns:15e9,cumulative_bytes:15*mib},
  ]:[{elapsed_ns:1e9,cumulative_bytes:1*mib},{elapsed_ns:2e9,cumulative_bytes:10*mib}]}}
  return [key(node),{tests:[],monitor:[],downloads:[download],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at}]
 })) as Record<string,QualityEvidence>
 const result=recommend(nodes,data,[]).download
 expect(result.map(c=>c.node.node_key)).toEqual(['continuous','burst'])
 expect(result[0].score).toBeCloseTo(1)
 expect(result[1].score).toBeCloseTo(10/15)
})
it('offers three evidenced AI candidates and drops unread or newly failed evidence',()=>{
 const nodes=Array.from({length:4},(_,i)=>({profile_id:'p',node_key:'n'+i,node_identity_key:'i'+i,config_revision_key:'r',display_name:'Node '+i,profile_name:'P',type:'ss',country_code:'',country_flag:''}) satisfies NodeOption)
 const at='2026-10-20T10:00:00Z',catalog=[{service_id:'ai',category:'AI 服务',name:'AI',result_kind:'account_availability'}] as ServiceRule[]
 const data=Object.fromEntries(nodes.map(n=>[key(n),{tests:[],monitor:[],downloads:[],services:[{...n,attempt_id:'a',request_id:'request-a',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',service_id:'ai',rule:{name:'AI',target_url:'https://ai.example'},result:{outcome:'matched',finished_at:at,bytes_read:0}} as Attempt],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at}])) as Record<string,QualityEvidence>
 const first=recommend(nodes,data,catalog);expect(first.ai).toHaveLength(3);expect(first.ai[0].basis).toContain('AI：规则通过');expect(first.ai[0].count).toBe(1);expect(first.ai[0].at).toBe(at);expect(first.download).toEqual([])
 for(const e of Object.values(data))e.reads.service.complete=false
 expect(recommend(nodes,data,catalog).ai).toEqual([])
 data[key(nodes[0])].reads.service.complete=true;data[key(nodes[0])].services.unshift({...data[key(nodes[0])].services[0],attempt_id:'new',finished_at:'2026-10-20T11:00:00Z',result:{outcome:'region_blocked',bytes_read:0,finished_at:'2026-10-20T11:00:00Z'}})
 expect(recommend(nodes,data,catalog).ai).toEqual([])
})

it('recommends same-condition daily and download candidates on non-Darwin and ranks full downloads alongside partial ones under the same rule',()=>{
 const nodes=Array.from({length:4},(_,i)=>({profile_id:'p',node_key:'n'+i,node_identity_key:'i'+i,config_revision_key:'r',display_name:'Node '+i,profile_name:'P',type:'hysteria2',country_code:'',country_flag:''}) satisfies NodeOption)
 const at='2026-10-20T10:00:00Z',target='https://speed.cloudflare.com/__down?bytes=1',dlUrl='https://speed.cloudflare.com/__down?bytes=10485760'
 const makeLatency=(n:NodeOption,ms:number):LatencyTest=>({...n,attempt_id:'lat-'+n.node_key,display_name:n.display_name,node_type:n.type,source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,target,requested_at:at,finished_at:at,status:'completed',latency_ms:ms,total_samples:6,success_samples:6,failure_samples:0,persistence_state:'saved',samples:Array.from({length:6},(_,seq)=>({seq:seq+1,target,timestamp:at,latency_ms:ms,success:true}))})
 const makeDownload=(n:NodeOption,mib:number,outcome:'byte_limit'|'time_limit'):Attempt=>({...n,attempt_id:'dl-'+n.node_key,request_id:'req-'+n.node_key,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',node_type:n.type,rule:{target_url:dlUrl,method:'GET',rule_version:1,maximum_bytes:10485760,maximum_duration_ns:10000000000,network_path_method:'legacy_default_binding_unverified'},result:{outcome,target_url:dlUrl,bytes_read:Math.round(mib*1048576),duration_ns:1000000000,phase_timings_ns:{response_body:1000000000},finished_at:at,network_path:{method:'legacy_default_binding_unverified',dns_bind_verified:false,socket_bind_verified:false,tcp_bindings:0,udp_bindings:0}}})
 const speeds:[number,'byte_limit'|'time_limit'][]=[[2.6,'byte_limit'],[2.5,'byte_limit'],[1.3,'time_limit'],[0.5,'time_limit']]
 const latencies=[180,138,158,210]
 const data=Object.fromEntries(nodes.map((n,i)=>[key(n),{tests:[makeLatency(n,latencies[i])],monitor:[],downloads:[makeDownload(n,speeds[i][0],speeds[i][1])],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at}])) as Record<string,QualityEvidence>
 const result=recommend(nodes,data,[])
 expect(result.daily.map(c=>c.node.display_name)).toEqual(['Node 1','Node 2','Node 0'])
 expect(result.daily[0].basis).toContain('Cloudflare · 6/6 次响应 · 中位 138 ms')
 expect(result.download.map(c=>c.node.display_name)).toEqual(['Node 0','Node 1','Node 2'])
 expect(result.download[0].basis).toContain('2.60 MiB/s · 完整下载')
 expect(result.download[2].basis).toContain('1.30 MiB/s · 部分下载')
})

it('labels only the configuration scopes contributing to the chosen daily condition',()=>{
 const node:NodeOption={profile_id:'p',node_key:'current',node_identity_key:'identity',config_revision_key:'current',display_name:'Node',profile_name:'P',type:'ss',country_code:'',country_flag:''}
 const at='2026-11-18T00:00:00Z',oldAt='2026-11-17T00:00:00Z',target='https://speed.cloudflare.com/__down?bytes=1'
 const current:LatencyTest={...node,attempt_id:'current',node_type:'ss',requested_at:at,finished_at:at,status:'completed',method:'http_ttfb',method_version:2,total_samples:6,success_samples:6,failure_samples:0,latency_ms:20,persistence_state:'saved',samples:Array.from({length:6},(_,seq)=>({seq,target,timestamp:at,latency_ms:20,success:true}))}
 const old:LatencyTest={...current,node_key:'older',config_revision_key:'older',attempt_id:'older',method_version:1,samples:current.samples.map(sample=>({...sample,timestamp:oldAt}))}
 const e:QualityEvidence={tests:[old,current],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:oldAt,until:at}
 const get=()=>recommend([node],{[key(node)]:e},[]).daily[0]
 expect(get()).toMatchObject({configuration:'current',inspectionScope:{node_key:'current',config_revision_key:'current'}})
 old.method_version=2
 expect(get()).toMatchObject({configuration:'mixed',inspectionScope:{node_key:'current',config_revision_key:'current'}})
 e.tests=[old]
 expect(get()).toMatchObject({configuration:'historical',inspectionScope:{node_key:'older',config_revision_key:'older'}})
})
it('shows mixed service configurations and never fills missing historical identity with the current one',()=>{
 const node:NodeOption={profile_id:'p',node_key:'current',node_identity_key:'identity',config_revision_key:'current',display_name:'Node',profile_name:'P',type:'ss',country_code:'',country_flag:''}
 const at='2026-11-18T00:00:00Z',oldAt='2026-11-17T00:00:00Z'
 const catalog=[{service_id:'ai-a',name:'A',category:'AI'},{service_id:'ai-b',name:'B',category:'AI'}] as ServiceRule[]
 const current:Attempt={...node,attempt_id:'current',request_id:'current',display_name:'Node',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',service_id:'ai-a',rule:{target_url:'https://example.invalid',name:'A'},result:{outcome:'matched',bytes_read:0,finished_at:at}}
 const old:Attempt={...current,node_key:'older',node_identity_key:'older-identity',config_revision_key:'older',attempt_id:'older',service_id:'ai-b',finished_at:oldAt,result:{...current.result!,finished_at:oldAt}}
 const e:QualityEvidence={tests:[],monitor:[],downloads:[],services:[old,current],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:oldAt,until:at}
 const get=()=>recommend([node],{[key(node)]:e},catalog).ai[0]
 expect(get()).toMatchObject({configuration:'mixed',inspectionScope:{node_identity_key:'identity',config_revision_key:'current'}})
 e.services=[old];old.node_identity_key=''
 expect(get()).toMatchObject({configuration:'unknown',inspectionScope:{node_identity_key:'',node_key:'older',config_revision_key:'older'}})
})
