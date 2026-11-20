import {expect,it} from 'vitest'
import {candidateDiff,isRecommendation,recommend,recommendationService,recommendationServices} from '../recommendations'
import {key,type Attempt,type LatencyTest,type NodeOption,type ServiceRule} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
it('ranks continuous download ahead of an initial burst followed by a timeout stall',()=>{
 const nodes=['burst','continuous'].map(id=>({profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',display_name:id,profile_name:'P',type:'ss',country_code:'JP',country_flag:''}) satisfies NodeOption)
 const at='2026-11-18T00:00:00Z',mib=1048576
 const data=Object.fromEntries(nodes.map((node,i)=>{
  const download:Attempt={...node,attempt_id:node.node_key,request_id:node.node_key,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://download.example/file',method:'GET',rule_version:3,maximum_bytes:20*mib,maximum_duration_ns:15e9},result:{outcome:'time_limit',finished_at:at,bytes_read:(i?15:10)*mib,phase_timings_ns:{response_body:15e9},samples:i?[
   {elapsed_ns:5e9,cumulative_bytes:5*mib},{elapsed_ns:10e9,cumulative_bytes:10*mib},{elapsed_ns:15e9,cumulative_bytes:15*mib},
  ]:[{elapsed_ns:1e9,cumulative_bytes:1*mib},{elapsed_ns:2e9,cumulative_bytes:10*mib}]}}
  return [key(node),{tests:[],monitor:[],downloads:[download],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at}]
 })) as Record<string,QualityEvidence>
 const result=recommend(nodes,data,[]).download
 expect(result.map(c=>c.node.node_key)).toEqual(['continuous'])
 expect(isRecommendation(result[0])).toBe(false)
 expect(result[0].score).toBeCloseTo(1)
})
it('offers independent AI picks in each region and drops unread or newly failed evidence',()=>{
 const nodes=Array.from({length:4},(_,i)=>({profile_id:'p',node_key:'n'+i,node_identity_key:'i'+i,config_revision_key:'r',display_name:'Node '+i,profile_name:'P',type:'ss',country_code:i<2?'JP':'SG',country_flag:''}) satisfies NodeOption)
 const at='2026-10-20T10:00:00Z',catalog=[{service_id:'ai',category:'AI 服务',name:'AI',result_kind:'account_availability'}] as ServiceRule[]
 const data=Object.fromEntries(nodes.map(n=>[key(n),{tests:[],monitor:[],downloads:[],services:[{...n,attempt_id:'a',request_id:'request-a',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',service_id:'ai',rule:{name:'AI',target_url:'https://ai.example'},result:{outcome:'matched',finished_at:at,bytes_read:0}} as Attempt],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at}])) as Record<string,QualityEvidence>
 const first=recommend(nodes,data,catalog);expect(first.ai.map(c=>c.region)).toEqual(['JP','JP','SG','SG']);expect(first.ai[0].basis).toContain('AI · 规则通过');expect(first.ai[0].count).toBe(1);expect(first.ai[0].at).toBe(at);expect(first.download).toEqual([])
 for(const e of Object.values(data))e.reads.service.complete=false
 expect(recommend(nodes,data,catalog).ai).toEqual([])
 data[key(nodes[0])].reads.service.complete=true;data[key(nodes[0])].services.unshift({...data[key(nodes[0])].services[0],attempt_id:'new',finished_at:'2026-10-20T11:00:00Z',result:{outcome:'region_blocked',bytes_read:0,finished_at:'2026-10-20T11:00:00Z'}})
 expect(recommend(nodes,data,catalog).ai).toEqual([])
})

it('recommends two same-condition candidates on non-Darwin and uses full downloads for picks',()=>{
 const nodes=Array.from({length:4},(_,i)=>({profile_id:'p',node_key:'n'+i,node_identity_key:'i'+i,config_revision_key:'r',display_name:'Node '+i,profile_name:'P',type:'hysteria2',country_code:'JP',country_flag:''}) satisfies NodeOption)
 const at='2026-10-20T10:00:00Z',target='https://speed.cloudflare.com/__down?bytes=1',dlUrl='https://speed.cloudflare.com/__down?bytes=10485760'
 const makeLatency=(n:NodeOption,ms:number):LatencyTest=>({...n,attempt_id:'lat-'+n.node_key,display_name:n.display_name,node_type:n.type,source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,target,requested_at:at,finished_at:at,status:'completed',latency_ms:ms,total_samples:6,success_samples:6,failure_samples:0,persistence_state:'saved',samples:Array.from({length:6},(_,seq)=>({seq:seq+1,target,timestamp:at,latency_ms:ms,success:true}))})
 const makeDownload=(n:NodeOption,mib:number,outcome:'byte_limit'|'time_limit'):Attempt=>({...n,attempt_id:'dl-'+n.node_key,request_id:'req-'+n.node_key,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',node_type:n.type,rule:{target_url:dlUrl,method:'GET',rule_version:1,maximum_bytes:10485760,maximum_duration_ns:10000000000,network_path_method:'legacy_default_binding_unverified'},result:{outcome,target_url:dlUrl,bytes_read:Math.round(mib*1048576),duration_ns:1000000000,phase_timings_ns:{response_body:1000000000},finished_at:at,network_path:{method:'legacy_default_binding_unverified',dns_bind_verified:false,socket_bind_verified:false,tcp_bindings:0,udp_bindings:0}}})
 const speeds:[number,'byte_limit'|'time_limit'][]=[[2.6,'byte_limit'],[2.5,'byte_limit'],[1.3,'time_limit'],[0.5,'time_limit']]
 const latencies=[180,138,158,210]
 const data=Object.fromEntries(nodes.map((n,i)=>[key(n),{tests:[makeLatency(n,latencies[i])],monitor:[],downloads:[makeDownload(n,speeds[i][0],speeds[i][1])],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at}])) as Record<string,QualityEvidence>
 const result=recommend(nodes,data,[])
 expect(result.daily.map(c=>c.node.display_name)).toEqual(['Node 1','Node 2'])
 expect(result.daily[0].basis).toContain('Cloudflare · 6/6 次响应 · 中位 138 ms')
 expect(result.download.map(c=>c.node.display_name)).toEqual(['Node 0','Node 1'])
 expect(result.download[0].basis).toContain('短测均速 2.60 MiB/s')
 data[key(nodes[0])].downloads=[]
 data[key(nodes[1])].downloads=[]
 const reference=recommend(nodes,data,[]).download
 expect(reference.map(c=>c.node.display_name)).toEqual(['Node 2'])
 expect(isRecommendation(reference[0])).toBe(false)

 const fastWithLoss=makeLatency(nodes[0],20)
 fastWithLoss.samples[0].success=false
 const variable=makeLatency(nodes[2],138)
 variable.samples.forEach((sample,idx)=>{sample.latency_ms+=idx<3?-15:15})
 data[key(nodes[0])].tests=[fastWithLoss]
 data[key(nodes[2])].tests=[variable]
 expect(recommend(nodes,data,[]).daily.map(c=>c.node.display_name)).toEqual(['Node 1','Node 2'])
})

it('chooses distinct stable lines per region and keeps a fast unstable average as reference',()=>{
 const at='2026-11-18T00:00:00Z',mib=2**20
 function line(id:string,region:string,profile='P',identity=id):NodeOption{
  return {profile_id:profile,node_key:id,node_identity_key:identity,config_revision_key:'r',display_name:id,profile_name:profile,type:'ss',country_code:region,country_flag:''}
 }
 const rows=[
  {node:line('first','HK'),speed:42,stable:true},
  {node:line('alias','HK','P2','first'),speed:41,stable:true},
  {node:line('backup','HK'),speed:38,stable:true},
  {node:line('volatile','HK'),speed:82,stable:false},
  {node:line('reference','JP'),speed:70,stable:false},
  {node:line('unidentified','ZZ'),speed:95,stable:true},
 ]
 const data:Record<string,QualityEvidence>={}
 for(const {node,speed,stable} of rows){
  const download:Attempt={...node,attempt_id:node.node_key,request_id:node.node_key,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',node_type:node.type,
   rule:{target_url:'https://proof.ovh.net/files/1Gb.dat',method:'GET',mode:'adaptive',rule_version:5,maximum_duration_ns:15e9},
   result:{outcome:stable?'completed':'time_limit',end_reason:stable?'speed_stable':'time_limit',finished_at:at,bytes_read:speed*5*mib,
    stable_speed_mbps:stable?speed*8*mib/1e6:undefined,phase_timings_ns:{response_body:5e9}}}
  data[key(node)]={tests:[],monitor:[],downloads:[download],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at}
 }
 const result=recommend(rows.map(r=>r.node),data,[]).download
 expect(result.map(c=>[c.region,c.node.node_key,isRecommendation(c)])).toEqual([['HK','first',true],['HK','backup',true],['JP','reference',false]])
 expect(result[2].basis).toContain('未稳定均速 70.00 MiB/s · 达到时间上限')
})

it('labels only the configuration scopes contributing to the chosen daily condition',()=>{
 const node:NodeOption={profile_id:'p',node_key:'current',node_identity_key:'identity',config_revision_key:'current',display_name:'Node',profile_name:'P',type:'ss',country_code:'JP',country_flag:''}
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
it('uses only the selected service and never fills a missing historical identity',()=>{
 const node:NodeOption={profile_id:'p',node_key:'current',node_identity_key:'identity',config_revision_key:'current',display_name:'Node',profile_name:'P',type:'ss',country_code:'JP',country_flag:''}
 const at='2026-11-18T00:00:00Z',oldAt='2026-11-17T00:00:00Z'
 const catalog=[{service_id:'ai-a',name:'A',category:'AI'},{service_id:'ai-b',name:'B',category:'AI'}] as ServiceRule[]
 const current:Attempt={...node,attempt_id:'current',request_id:'current',display_name:'Node',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',service_id:'ai-a',rule:{target_url:'https://example.invalid',name:'A'},result:{outcome:'matched',bytes_read:0,finished_at:at}}
 const old:Attempt={...current,node_key:'older',node_identity_key:'older-identity',config_revision_key:'older',attempt_id:'older',service_id:'ai-b',finished_at:oldAt,result:{...current.result!,finished_at:oldAt}}
 const e:QualityEvidence={tests:[],monitor:[],downloads:[],services:[old,current],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:oldAt,until:at}
 const get=()=>recommend([node],{[key(node)]:e},catalog).ai[0]
 expect(get()).toMatchObject({configuration:'current',inspectionServiceId:'ai-a',inspectionScope:{node_identity_key:'identity',config_revision_key:'current'}})
 old.node_identity_key=''
 expect(recommend([node],{[key(node)]:e},catalog,{ai:'ai-b'}).ai[0]).toMatchObject({
  configuration:'unknown',inspectionServiceId:'ai-b',inspectionScope:{node_identity_key:'',node_key:'older',config_revision_key:'older'},
 })
})

it('uses four-site daily evidence and disqualifies a fast node with a completely blocked site',()=>{
 const at='2026-11-20T06:00:00Z'
 const urls={
  cloudflare:'https://speed.cloudflare.com/__down?bytes=1',
  google:'https://www.gstatic.com/generate_204',
  github:'https://api.github.com/zen',
  apple:'https://captive.apple.com/hotspot-detect.html',
 }
 const mkNode=(id:string,region='JP'):NodeOption=>({profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',display_name:id,profile_name:'P',type:'hysteria2',country_code:region,country_flag:''})
 const mkSite=(n:NodeOption,id:keyof typeof urls,ms:number,ok=true):LatencyTest=>({
  ...n,attempt_id:`lat-${n.node_key}-${id}`,display_name:n.display_name,node_type:n.type,source:'workbench',method:'http_get_via_proxy_first_byte',method_version:2,target:urls[id],requested_at:at,finished_at:at,status:'completed',latency_ms:ok?ms:0,total_samples:6,success_samples:ok?6:0,failure_samples:ok?0:6,persistence_state:'saved',
  samples:Array.from({length:6},(_,seq)=>({seq:seq+1,target:urls[id],timestamp:at,latency_ms:ok?ms:0,success:ok,error:ok?'':'timeout'})),
 })
 const brokenFast=mkNode('broken-fast','JP')
 const healthy1=mkNode('healthy-1','JP')
 const healthy2=mkNode('healthy-2','JP')
 const singleSiteFast=mkNode('single-cf','JP')
 const aloneBroken=mkNode('alone-broken','SG')
 const emptyReads={latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}}
 const data:Record<string,QualityEvidence>={
  [key(brokenFast)]:{tests:[mkSite(brokenFast,'cloudflare',25,true),mkSite(brokenFast,'google',0,false),mkSite(brokenFast,'github',30,true),mkSite(brokenFast,'apple',28,true)],monitor:[],downloads:[],services:[],reads:emptyReads,complete:true,since:at,until:at},
  [key(healthy1)]:{tests:[mkSite(healthy1,'cloudflare',130,true),mkSite(healthy1,'google',140,true),mkSite(healthy1,'github',150,true),mkSite(healthy1,'apple',135,true)],monitor:[],downloads:[],services:[],reads:emptyReads,complete:true,since:at,until:at},
  [key(healthy2)]:{tests:[mkSite(healthy2,'cloudflare',160,true),mkSite(healthy2,'google',170,true),mkSite(healthy2,'github',180,true),mkSite(healthy2,'apple',165,true)],monitor:[],downloads:[],services:[],reads:emptyReads,complete:true,since:at,until:at},
  [key(singleSiteFast)]:{tests:[mkSite(singleSiteFast,'cloudflare',60,true)],monitor:[],downloads:[],services:[],reads:emptyReads,complete:true,since:at,until:at},
  [key(aloneBroken)]:{tests:[mkSite(aloneBroken,'cloudflare',40,true),mkSite(aloneBroken,'google',0,false),mkSite(aloneBroken,'github',45,true),mkSite(aloneBroken,'apple',42,true)],monitor:[],downloads:[],services:[],reads:emptyReads,complete:true,since:at,until:at},
 }
 const daily=recommend([brokenFast,healthy1,healthy2,singleSiteFast,aloneBroken],data,[]).daily
 const jp=daily.filter(c=>c.region==='JP')
 const sg=daily.filter(c=>c.region==='SG')
 expect(jp.map(c=>c.node.node_key)).toEqual(['healthy-1','healthy-2'])
 expect(jp[0]!.basis).toContain('四站综合（4/4 站）')
 expect(jp[0]!.inspectionTargetId).toBeUndefined()
 expect(candidateDiff(jp[0]!,jp[1]!,'daily')).toBe('中位延迟比首选高 30 ms')
 expect(sg).toHaveLength(1)
 expect(isRecommendation(sg[0]!)).toBe(false)
 expect(sg[0]!.blockedSites).toEqual(['Google'])
 expect(sg[0]!.basis).toContain('部分站点不通（Google）')
})

it('defaults AI to gemini_unlock, exposes Japan unlock services like DLsite, and attaches saved CF challenge hints',()=>{
 const at='2026-11-20T06:00:00Z'
 const node:NodeOption={profile_id:'p',node_key:'jp1',node_identity_key:'jp1',config_revision_key:'r1',display_name:'JP 1',profile_name:'P',type:'ss',country_code:'JP',country_flag:''}
 const catalog=[
  {service_id:'chatgpt_unlock',name:'ChatGPT',category:'AI',result_kind:'unlock'},
  {service_id:'gemini_unlock',name:'Google Gemini',category:'AI',result_kind:'unlock'},
  {service_id:'netflix_unlock',name:'Netflix',category:'流媒体',result_kind:'unlock'},
  {service_id:'dlsite_unlock',name:'DLsite',category:'日本',result_kind:'unlock'},
 ] as ServiceRule[]
 const aiOptions=recommendationServices(catalog,'ai')
 expect(recommendationService(aiOptions,'ai')?.service_id).toBe('gemini_unlock')
 const streamOptions=recommendationServices(catalog,'streaming')
 expect(streamOptions.map(r=>r.service_id)).toEqual(['netflix_unlock','dlsite_unlock'])

 const mkAttempt=(serviceId:string,outcome:string,rev='r1',detail=''):Attempt=>({
  ...node,config_revision_key:rev,attempt_id:`att-${serviceId}-${rev}`,request_id:`req-${serviceId}`,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',service_id:serviceId,
  rule:{service_id:serviceId,name:serviceId,target_url:'https://example.invalid'},
  result:{outcome,bytes_read:0,finished_at:at,details:detail?{cf_challenge_detail:detail}:{}},
 })
 const e:QualityEvidence={
  tests:[],monitor:[],downloads:[],
  services:[
   mkAttempt('dlsite_unlock','unlocked','r1'),
   mkAttempt('cloudflare_challenge','verification_required','r1','ChatGPT: 直通(200) · Claude: 弹盾(403)'),
  ],
  reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:at,until:at,
 }
 const picks=recommend([node],{[key(node)]:e},catalog,{streaming:'dlsite_unlock'}).streaming
 expect(picks).toHaveLength(1)
 expect(picks[0]!.basis).toContain('DLsite · 规则通过')
 expect(picks[0]!.cfChallenge).toMatchObject({tone:'warn',label:'部分弹 CF 盾'})
 // Mismatched revision CF evidence must not be attached as current.
 e.services=[mkAttempt('dlsite_unlock','unlocked','r1'),mkAttempt('cloudflare_challenge','challenge','old-rev','all 403')]
 expect(recommend([node],{[key(node)]:e},catalog,{streaming:'dlsite_unlock'}).streaming[0]!.cfChallenge).toBeUndefined()
})

it('attaches multi-source reputation consensus, divergence and single-source fallback hints on recommendations',()=>{
 const at='2026-11-20T12:00:00Z'
 const mkNode=(id:string,region:string):NodeOption=>({profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r1',display_name:id,profile_name:'P',type:'hysteria2',country_code:region,country_flag:''})
 const catalog=[{service_id:'gemini_unlock',name:'Google Gemini',category:'AI',result_kind:'unlock'}] as ServiceRule[]
 const mkAttempt=(n:NodeOption,serviceId:string,outcome:string,details:Record<string,string>={}):Attempt=>({
  ...n,attempt_id:`att-${n.node_key}-${serviceId}`,request_id:`req-${n.node_key}-${serviceId}`,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',service_id:serviceId,
  rule:{service_id:serviceId,name:serviceId,target_url:'https://example.invalid'},
  result:{outcome,bytes_read:0,finished_at:at,details},
 })
 const divNode=mkNode('sg-div','SG'),fbNode=mkNode('jp-fb','JP'),cleanNode=mkNode('uk-clean','GB')
 const reads={latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}}
 const data:Record<string,QualityEvidence>={
  [key(divNode)]:{tests:[],monitor:[],downloads:[],services:[mkAttempt(divNode,'gemini_unlock','unlocked'),mkAttempt(divNode,'ipquery_ip_quality','profiled',{risk_score:'0'}),mkAttempt(divNode,'iplogs_ip_quality','profiled',{risk_score:'40'})],reads,complete:true,since:at,until:at},
  [key(fbNode)]:{tests:[],monitor:[],downloads:[],services:[mkAttempt(fbNode,'gemini_unlock','unlocked'),mkAttempt(fbNode,'ippure_ip_quality','timed_out'),mkAttempt(fbNode,'ping0_ipv4','profiled',{risk_score:'15'})],reads,complete:true,since:at,until:at},
  [key(cleanNode)]:{tests:[],monitor:[],downloads:[],services:[mkAttempt(cleanNode,'gemini_unlock','unlocked'),mkAttempt(cleanNode,'ippure_ip_quality','profiled',{risk_score:'12',risk_scale_max:'100',risk_direction:'lower_is_better'}),mkAttempt(cleanNode,'ping0_ipv4','profiled',{risk_score:'18'})],reads,complete:true,since:at,until:at},
 }
 const picks=recommend([divNode,fbNode,cleanNode],data,catalog,{ai:'gemini_unlock'}).ai
 expect(picks.find(p=>p.region==='SG')?.purity).toMatchObject({divergent:true,tone:'warn',badgeLabel:'2站分歧 · 均80',sourceCount:2})
 expect(picks.find(p=>p.region==='JP')?.purity).toMatchObject({singleFallback:true,tone:'warn',badgeLabel:'单源参考 · 纯净 85',sourceCount:1})
 expect(picks.find(p=>p.region==='GB')?.purity).toMatchObject({divergent:false,singleFallback:false,tone:'good',badgeLabel:'2站共识 · 纯净 85',sourceCount:2})
})
