import {expect,it} from 'vitest'
import {latencyComparisonGroup,serviceComparisonGroup} from '../sharedResults'
import type {Attempt} from '../domain'
it('keeps legacy monitor and verified workbench conditions out of one latency ranking',()=>{
 const old=latencyComparisonGroup({id:'old',time:'',value:1,tone:'good',source:'monitor',conditionKey:'cp-rtt-unknown',description:'旧监测方法未记录'})
 const current=latencyComparisonGroup({id:'new',time:'',value:2,tone:'good',source:'manual',conditionKey:'physical-v2',description:'http_get_via_proxy_first_byte / v2 en1'})
 expect(old.key).not.toBe(current.key);expect(old.valid&&current.valid).toBe(true)
 expect(latencyComparisonGroup({id:'none',time:'',value:null,tone:'bad',description:'失败'}).valid).toBe(false)
})
it('separates service rule matches measured through recorded physical and legacy routes',()=>{
 const base:Attempt={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',attempt_id:'a',request_id:'q',display_name:'n',requested_at:'2026-10-09T00:00:00Z',persistence_state:'saved',service_id:'cloudflare_204',execution_state:'completed',result:{outcome:'matched',bytes_read:0,finished_at:'2026-10-09T00:00:01Z',details:{execution_status:'executed'}},rule:{method:'GET',rule_version:1,target_url:'https://cp.cloudflare.com/generate_204'}}
 const physical={...base,result:{...base.result!,details:{...base.result!.details,network_path:JSON.stringify({method:'physical_socket_v2',interface:'en1',dns_mode:'physical_interface_dns_v1',socket_bind_verified:true})}}}
 expect(serviceComparisonGroup([base],['cloudflare_204']).key).not.toBe(serviceComparisonGroup([physical],['cloudflare_204']).key)
})
