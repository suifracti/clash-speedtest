import {mount} from '@vue/test-utils'
import {it,expect} from 'vitest'
import QualityMetrics from './QualityMetrics.vue'
import type {LatencyTest} from '../domain'
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
it('shows the latest same-rule failure by observation time, not the first old success or later save time',()=>{
 const wrapper=mount(QualityMetrics,{props:{evidence:{tests:[],monitor:[],downloads:[],services:[oldService,newService],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'',until:''},target:'cloudflare',catalog:[]}})
 expect(wrapper.get('.quality-services').text()).toContain('HTTP 拒绝');expect(wrapper.get('.quality-services').text()).not.toContain('已解锁')
})
it('distinguishes the newest failed download from the previous valid speed and their different limits',()=>{
 const wrapper=mount(QualityMetrics,{props:{expanded:true,evidence:{tests:[],monitor:[],downloads:[oldDownload,newDownload],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'',until:''},target:'cloudflare',catalog:[]}})
 expect(wrapper.get('.quality-download').text()).toContain('最近检测：超时');expect(wrapper.get('.quality-download').text()).toContain('历史有效速度：2.00 MiB/s')
 expect(wrapper.get('.quality-download').text()).toContain('10/09 19:00');expect(wrapper.get('.quality-download').text()).toContain('10/09 18:00')
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
