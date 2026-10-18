import {mount} from '@vue/test-utils'
import {expect,it,vi} from 'vitest'
import SiteMetrics from './SiteMetrics.vue'
import {multiSiteQuality,type QualityEvidence} from '../nodeQuality'
import {targets,type NodeScope,type LatencyTest} from '../domain'
vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
const node:NodeScope={profile_id:'p',node_key:'n',node_identity_key:'identity',config_revision_key:'rev'}
const at='2026-10-18T08:00:00Z',url=targets[0].urls[0]
function fixture():QualityEvidence{
 const test:LatencyTest={...node,attempt_id:'one',display_name:'合成',node_type:'ss',requested_at:at,finished_at:at,status:'completed',latency_ms:30,total_samples:6,success_samples:5,failure_samples:1,persistence_state:'saved',method:'http_get_via_proxy_first_byte',method_version:1,network_path:{method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',socket_bind_verified:true,dns_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:'ipv4',address_source:'A'},samples:Array.from({length:6},(_,i)=>({seq:i,target:url,timestamp:at,latency_ms:20+i*10,success:i!==5,error:i===5?'HTTP 429':''}))}
 return {tests:[test],monitor:[],downloads:[],services:[],complete:true,reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},since:'2026-10-18T00:00:00Z',until:'2026-10-18T10:00:00Z'}
}
it('shows the newest failed round, clearly separates older success, and opens the local history chart from the value',async()=>{
 const e=fixture();e.tests[0].samples=e.tests[0].samples.map(s=>({...s,success:true,error:undefined}))
 const later='2026-10-18T09:00:00Z';e.tests.unshift({...e.tests[0],attempt_id:'new-failed',finished_at:later,requested_at:later,status:'failed',samples:e.tests[0].samples.map(s=>({...s,timestamp:later,success:false,latency_ms:0,error:'context deadline exceeded'}))})
 const summaries=multiSiteQuality(node,e);expect(summaries[0].success).toBe(6);expect(summaries[0].total).toBe(12)
 const w=mount(SiteMetrics,{props:{summaries,targetIds:targets.map(t=>t.id),tests:e.tests}})
 const cf=w.get('[data-site="cloudflare"]');expect(cf.get('.site-latest').text()).toBe('无响应');expect(cf.text()).toContain('本次 0 / 6 次响应');expect(cf.text()).toContain('上次 45 ms');expect(cf.text()).toContain('请求超时');expect(w.get('[data-site="google"]').text()).toContain('未测')
 await cf.get('.site-latest').trigger('click');expect(w.find('[aria-label="Cloudflare 延迟历史对比"]').exists()).toBe(true);expect(w.findAll('.site-round-list>button')).toHaveLength(2);expect(w.find('.trend-svg').exists()).toBe(true)
 await w.findAll('.site-round-list>button')[1].trigger('click');expect(w.find('.round-results').exists()).toBe(true)
 await w.get('.site-history-panel>.text-button').trigger('click');expect(w.emitted('history')).toEqual([['cloudflare']]);w.unmount()
})
it('keeps failed reads and HTTP rejection distinct from unmeasured and business pass',()=>{
 const e=fixture();e.tests[0].samples=e.tests[0].samples.map(s=>({...s,success:true,error:'HTTP 403'}))
 const w=mount(SiteMetrics,{props:{summaries:multiSiteQuality(node,e),targetIds:['cloudflare'],tests:e.tests}})
 expect(w.text()).toContain('403 拒绝');expect(w.text()).toContain('HTTP 响应不等于业务通过');w.unmount()
 const failed=mount(SiteMetrics,{props:{targetIds:['cloudflare'],error:'read failed'}});expect(failed.text()).toContain('读取失败');expect(failed.text()).not.toContain('未测');failed.unmount()
})
it('does not borrow the prior configuration or family when the latest round has no responses',()=>{
 const e=fixture(),latest={...e.tests[0],attempt_id:'changed',network_path:{...e.tests[0].network_path!,address_family:'ipv6'},finished_at:'2026-10-18T09:00:00Z',samples:e.tests[0].samples.map(s=>({...s,success:false,latency_ms:0,timestamp:'2026-10-18T09:00:00Z',error:'timeout'}))};e.tests.unshift(latest)
 const w=mount(SiteMetrics,{props:{summaries:multiSiteQuality(node,e),targetIds:['cloudflare'],tests:e.tests}});expect(w.get('.site-latest').text()).toBe('无响应');expect(w.get('.site-response').text()).toBe('本次 0 / 6 次响应');w.unmount()
})
