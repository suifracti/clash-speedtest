import {expect,it} from 'vitest'
import {recommend} from '../recommendations'
import {key,type Attempt,type NodeOption,type ServiceRule} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
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
