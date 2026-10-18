import {mount} from '@vue/test-utils'
import {it,expect} from 'vitest'
import QualityMetrics from './QualityMetrics.vue'
import {date,type LatencyTest} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
it('shows absent and insufficient evidence without assigning a score or treating profile as business availability',()=>{
 const wrapper=mount(QualityMetrics,{props:{evidence:{tests:[],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'',until:''},target:'cloudflare',catalog:[]}});expect(wrapper.text()).toContain('未测');expect(wrapper.text()).not.toContain('%');expect(wrapper.text()).not.toContain('评分')
})

it('keeps legacy latency statistics visible and labels missing route evidence as incomparable',()=>{
 const test:LatencyTest={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',attempt_id:'legacy',display_name:'n',node_type:'vless',target:'https://speed.cloudflare.com/__down?bytes=1',requested_at:'2026-10-09T00:00:00Z',finished_at:'2026-10-09T00:01:00Z',status:'completed',latency_ms:25,total_samples:5,success_samples:5,failure_samples:0,persistence_state:'saved',method:'http_get_via_proxy_first_byte',method_version:1,samples:Array.from({length:5},(_,i)=>({seq:i+1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-09T00:01:00Z',latency_ms:25+i,success:true}))}
 const evidence:QualityEvidence={tests:[test],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'2026-10-09T00:00:00Z',until:'2026-10-09T01:00:00Z'}
 const wrapper=mount(QualityMetrics,{props:{evidence,target:'cloudflare',catalog:[]}})
 expect(wrapper.text()).toContain('P50 27 ms');expect(wrapper.text()).toContain('出口条件证据不足');expect(wrapper.text()).not.toContain('读取失败');expect(wrapper.text()).not.toContain('失败')
})

import {oldService,newService,oldDownload,newDownload} from '../__tests__/fixtures/quality-evidence'
it('filters visible services and emits the selected service id',async()=>{
 const other={...oldService,attempt_id:'youtube',service_id:'youtube_unlock',rule:{...oldService.rule,service_id:'youtube_unlock',name:'YouTube'},result:{...oldService.result!,outcome:'unlocked'}}
 const wrapper=mount(QualityMetrics,{props:{evidence:{tests:[],monitor:[],downloads:[],services:[oldService,other],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'',until:''},target:'cloudflare',catalog:[],visibleServiceIds:['netflix_unlock']}})
 expect(wrapper.get('.quality-services-list').text()).toContain('netflix_unlock');expect(wrapper.get('.quality-services-list').text()).not.toContain('youtube_unlock')
 await wrapper.get('.service-pill-item').trigger('click');expect(wrapper.emitted('select-service')).toEqual([['netflix_unlock']])
 await wrapper.setProps({visibleServiceIds:[]});expect(wrapper.get('.quality-services-list').text()).not.toContain('netflix_unlock');wrapper.unmount()
})
it('keeps the home service summary compact and leaves comparison details available',async()=>{
 const services=Array.from({length:6},(_,i)=>({...oldService,attempt_id:'service-'+i,service_id:'service-'+i,rule:{...oldService.rule,service_id:'service-'+i,name:'Service '+i},result:{...oldService.result!,finished_at:`2026-10-09T10:0${i}:00Z`}}))
 const evidence:QualityEvidence={tests:[],monitor:[],downloads:[],services,reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'',until:''}
 const wrapper=mount(QualityMetrics,{props:{evidence,target:'cloudflare',catalog:[],project:'service'}});expect(wrapper.findAll('.service-pill-item')).toHaveLength(3)
 await wrapper.setProps({comparisonField:'service'});expect(wrapper.findAll('.service-pill-item')).toHaveLength(6);wrapper.unmount()
})
it('uses response and tail labels without claiming timeout or packet loss',()=>{
 const test:LatencyTest={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',attempt_id:'no-response',display_name:'n',node_type:'vless',target:'https://speed.cloudflare.com/__down?bytes=1',requested_at:'2026-10-09T00:00:00Z',finished_at:'2026-10-09T00:01:00Z',status:'completed',latency_ms:0,total_samples:2,success_samples:0,failure_samples:2,persistence_state:'saved',method:'http_get_via_proxy_first_byte',method_version:1,samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-09T00:01:00Z',latency_ms:0,success:false,error:'connection refused'},{seq:2,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-09T00:01:01Z',latency_ms:0,success:false,error:'connection refused'}]}
 const evidence:QualityEvidence={tests:[test],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'2026-10-09T00:00:00Z',until:'2026-10-09T01:00:00Z'}
 const wrapper=mount(QualityMetrics,{props:{evidence,target:'cloudflare',catalog:[]}}),text=wrapper.get('.quality-latency').text()
 expect(text).toContain('未获得响应');expect(text).toContain('尾部');expect(text).not.toContain('全数超时');expect(text).not.toContain('高丢包');expect(text).not.toContain('最慢')
 test.samples[0]={...test.samples[0],success:true,latency_ms:10};test.samples.push({...test.samples[1],seq:3,timestamp:'2026-10-09T00:01:02Z'},{...test.samples[1],seq:4,timestamp:'2026-10-09T00:01:03Z'});test.total_samples=4;test.success_samples=1;test.failure_samples=3
 const partial=mount(QualityMetrics,{props:{evidence:{...evidence,tests:[test]},target:'cloudflare',catalog:[]}});expect(partial.get('.quality-latency').text()).toContain('低响应率');partial.unmount();wrapper.unmount()
})
it('shows the latest same-rule failure by observation time, not the first old success or later save time',()=>{
 const wrapper=mount(QualityMetrics,{props:{evidence:{tests:[],monitor:[],downloads:[],services:[oldService,newService],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'',until:''},target:'cloudflare',catalog:[]}})
 expect(wrapper.get('.quality-services').text()).toContain('HTTP 拒绝');expect(wrapper.get('.quality-services').text()).not.toContain('已解锁')
})
it('distinguishes the newest failed download from the previous valid speed and their different limits',()=>{
 const wrapper=mount(QualityMetrics,{props:{expanded:true,evidence:{tests:[],monitor:[],downloads:[oldDownload,newDownload],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'',until:''},target:'cloudflare',catalog:[]}})
 expect(wrapper.get('.quality-download').text()).toContain('最近检测：超时');expect(wrapper.get('.quality-download').text()).toContain('历史有效速度：2.00 MiB/s')
 expect(wrapper.get('.quality-download').text()).toContain(date(newDownload.result!.finished_at));expect(wrapper.get('.quality-download').text()).toContain(date(oldDownload.result!.finished_at))
 expect(wrapper.get('.quality-download').attributes('title')).toContain('10 MiB');expect(wrapper.get('.quality-download').attributes('title')).toContain('5 MiB')
})
it('does not label latency incomplete when only download history is incomplete',()=>{
 const wrapper=mount(QualityMetrics,{props:{evidence:{tests:[],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:false,error:'missing download page'},service:{complete:true}},complete:false,since:'',until:''},target:'cloudflare',catalog:[],expanded:true}})
 expect(wrapper.get('.quality-latency').text()).not.toContain('读取不完整');expect(wrapper.get('.quality-download').text()).toContain('下载读取不完整');expect(wrapper.get('.quality-services').text()).not.toContain('读取不完整');expect(wrapper.get('.quality-evidence-details').text()).toContain('missing download page')
})

const comparisonEvidence=():QualityEvidence=>({tests:[{attempt_id:'compare-latency',target:'https://speed.cloudflare.com/__down?bytes=1',source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,node_type:'vless',network_path:{method:'physical',interface:'en1',dns_mode:'bound',socket_bind_verified:true},status:'completed',samples:[...Array.from({length:20},(_,i)=>({seq:i,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-09T11:00:00Z',latency_ms:(i+1)*10,success:true})),{seq:20,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-09T11:00:01Z',latency_ms:0,success:false,error:'timeout'}]} as LatencyTest],monitor:[],downloads:[oldDownload,newDownload],services:[oldService,newService],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'2026-10-09T00:00:00Z',until:'2026-10-09T12:00:00Z'})
it('projects comparison distribution and response into separate aligned rows without dropping statistics',async()=>{
 const e=comparisonEvidence(),original=JSON.stringify(e),ui=mount(QualityMetrics,{props:{evidence:e,target:'cloudflare',catalog:[],comparisonField:'distribution'} as any})
 expect(ui.text()).toContain('P50 105 ms');expect(ui.text()).toContain('P95 190 ms');expect(ui.text()).not.toContain('最近检测');expect(ui.text()).not.toContain('响应成功')
 await ui.setProps({comparisonField:'response'} as any);expect(ui.text()).toContain('响应成功 95%');expect(ui.text()).toContain('20 / 21 样本');expect(ui.text()).not.toContain('P50');expect(JSON.stringify(e)).toBe(original)
 e.reads.latency.complete=false;await ui.setProps({evidence:{...e}});expect(ui.text()).toContain('不可比较');ui.unmount()
})
it('shows both actual failed-download and old-speed conditions in comparison, with original evidence still expandable',async()=>{
 const ui=mount(QualityMetrics,{props:{evidence:comparisonEvidence(),target:'cloudflare',catalog:[],comparisonField:'download'} as any})
 expect(ui.text()).toContain('最近检测：超时');expect(ui.text()).toContain('历史有效速度：2.00 MiB/s');expect(ui.text()).toContain('10 MiB');expect(ui.text()).toContain('5 MiB');expect(ui.text()).not.toContain('P50')
 await ui.setProps({comparisonField:'service'} as any);expect(ui.text()).toContain('HTTP 拒绝');expect(ui.text()).not.toContain('已解锁');expect(ui.text()).not.toContain('P50')
 await ui.setProps({comparisonField:'details'} as any);expect(ui.get('details summary').text()).toContain('分组依据');expect(ui.text()).toContain('波动');expect(ui.text()).toContain('20/21');ui.unmount()
})

it('keeps every latest service conclusion reachable from a comparison cell',()=>{
 const e=comparisonEvidence();e.services.push({...oldService,service_id:'rule-second',attempt_id:'second'}, {...newService,service_id:'rule-third',attempt_id:'third'})
 const ui=mount(QualityMetrics,{props:{evidence:e,target:'cloudflare',catalog:[],comparisonField:'service'}})
 expect(ui.text()).toContain('rule-third');expect(ui.text()).toContain('rule-second');expect(ui.text()).toContain('HTTP 拒绝');ui.unmount()
})

it('keeps a usable single observation distinct from missing conditions or incomplete history',async()=>{
 const e=comparisonEvidence(),test=e.tests[0]
 test.samples=test.samples.slice(0,1)
 test.network_path={method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',socket_bind_verified:true,dns_bind_verified:true,address_family:'ipv4',address_source:'physical_ipv4_dns',tcp_bindings:1,udp_bindings:0}
 const ui=mount(QualityMetrics,{props:{evidence:e,target:'cloudflare',catalog:[]}})
 expect(ui.get('.quality-latency').text()).toContain('P50 10 ms')
 expect(ui.get('.quality-latency').text()).toContain('稳定性未确认')
 expect(ui.get('.quality-latency').text()).not.toContain('不可比较')
 expect(ui.get('.quality-latency').text()).toContain('P95 样本不足')
 test.network_path.dns_bind_verified=false
 await ui.setProps({evidence:{...e}})
 expect(ui.get('.quality-latency').text()).toContain('不可比较')
 expect(ui.get('.quality-latency').text()).toContain('出口条件证据不足')
 test.network_path.dns_bind_verified=true;e.reads.latency.complete=false
 await ui.setProps({evidence:{...e}})
 expect(ui.get('.quality-latency').text()).toContain('读取不完整 · 不可比较')
 ui.unmount()
})

it('scopes visible metrics to project tabs, keeping latency in latency, download in download, service in service, and combined for all three',async()=>{
 const e=comparisonEvidence()
 const ui=mount(QualityMetrics,{props:{evidence:e,target:'cloudflare',catalog:[],project:'latency'}})
 expect(ui.find('.quality-latency').exists()).toBe(true)
 expect(ui.find('.quality-download').exists()).toBe(false)
 expect(ui.find('.quality-services').exists()).toBe(false)
 expect(ui.classes()).toContain('single-project')
 expect(ui.classes()).toContain('project-latency')

 await ui.setProps({project:'download'})
 expect(ui.find('.quality-latency').exists()).toBe(false)
 expect(ui.find('.quality-download').exists()).toBe(true)
 expect(ui.find('.quality-services').exists()).toBe(false)
 expect(ui.classes()).toContain('single-project')
 expect(ui.classes()).toContain('project-download')

 await ui.setProps({project:'service'})
 expect(ui.find('.quality-latency').exists()).toBe(false)
 expect(ui.find('.quality-download').exists()).toBe(false)
 expect(ui.find('.quality-services').exists()).toBe(true)
 expect(ui.classes()).toContain('single-project')
 expect(ui.classes()).toContain('project-service')

 await ui.setProps({project:'combined'})
 expect(ui.find('.quality-latency').exists()).toBe(true)
 expect(ui.find('.quality-download').exists()).toBe(true)
 expect(ui.find('.quality-services').exists()).toBe(true)
 expect(ui.classes()).not.toContain('single-project')
 expect(ui.classes()).toContain('project-combined')
 ui.unmount()
})
it('shows old ambiguous flapping and incomplete passes as unconfirmed in the quality list',()=>{
 for(const [outcome,details] of [['flapping',{sample_total:'2',sample_success:'1',sample_failure:'1',observed_outcomes:'matched, transport_error'}],['unlocked',{sample_total:'4',sample_success:'3',sample_unknown:'1'}]] as const){
  const a={...oldService,result:{...oldService.result!,outcome,details}},raw=JSON.stringify(a)
  const ui=mount(QualityMetrics,{props:{evidence:{tests:[],monitor:[],downloads:[],services:[a],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'',until:''},target:'cloudflare',catalog:[]}})
  expect(ui.get('.quality-services-list').text()).toContain('未确认');expect(ui.get('.quality-services-list').find('.good').exists()).toBe(false);expect(JSON.stringify(a)).toBe(raw);ui.unmount()
 }
})
