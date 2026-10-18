import {mount} from '@vue/test-utils'
import {expect,it} from 'vitest'
import SiteMetrics from './SiteMetrics.vue'
import {multiSiteQuality,type QualityEvidence} from '../nodeQuality'
import {targets,type NodeScope,type LatencyTest} from '../domain'

const node:NodeScope={profile_id:'p',node_key:'n',node_identity_key:'identity',config_revision_key:'rev'}
const at='2026-10-18T08:00:00Z',url=targets[0].urls[0]
function fixture():QualityEvidence{
 const test:LatencyTest={...node,attempt_id:'one',display_name:'合成',node_type:'ss',requested_at:at,finished_at:at,status:'completed',latency_ms:30,total_samples:6,success_samples:5,failure_samples:1,persistence_state:'saved',method:'http_get_via_proxy_first_byte',method_version:1,network_path:{method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',socket_bind_verified:true,dns_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:'ipv4',address_source:'A'},samples:Array.from({length:6},(_,i)=>({seq:i,target:url,timestamp:at,latency_ms:20+i*10,success:i!==5,error:i===5?'HTTP 429':''}))}
 return {tests:[test],monitor:[],downloads:[],services:[],complete:true,reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},since:'2026-10-18T00:00:00Z',until:'2026-10-18T10:00:00Z'}
}
it('shows every selected site with explicit response/sample/P50/P95 without opening details',async()=>{
 const e=fixture(),w=mount(SiteMetrics,{props:{summaries:multiSiteQuality(node,e),targetIds:targets.map(t=>t.id),tests:e.tests}})
 expect(w.findAll('.site-metric')).toHaveLength(6)
 const cf=w.get('td[data-site="cloudflare"]');expect(cf.text()).toContain('40 ms');expect(w.get('.site-metrics-table').text()).toContain('响应 / 样本83% · 5/6');expect(w.text()).toContain('P95 样本不足');expect(w.text()).toContain('429 限流');expect(w.get('td[data-site="google"]').text()).toContain('未测')
 await w.findAll('button[aria-label="查看 Cloudflare 原始历史"]')[0].trigger('click');expect(w.emitted('history')).toEqual([['cloudflare']]);w.unmount()
})
it('keeps failed reads distinct from unmeasured and does not label an HTTP rejection as business success',()=>{
 const e=fixture();e.tests[0].samples=e.tests[0].samples.map(s=>({...s,success:true,error:'HTTP 403'}))
 const w=mount(SiteMetrics,{props:{summaries:multiSiteQuality(node,e),targetIds:['cloudflare'],tests:e.tests}})
 expect(w.text()).toContain('403 拒绝');expect(w.text()).toContain('HTTP 首字节（非业务验证）');w.unmount()
 const failed=mount(SiteMetrics,{props:{targetIds:['cloudflare'],error:'read failed'}});expect(failed.text()).toContain('读取失败');expect(failed.text()).not.toContain('未测');failed.unmount()
})
