import {mount} from '@vue/test-utils'
import {expect,it} from 'vitest'
import type {Attempt} from '../domain'
import NodeExitSummary from './NodeExitSummary.vue'

const modalStub={template:'<div class="modal-stub"><slot/></div>'}
const attempt=(serviceId:string,finishedAt:string,outcome:string,details?:Record<string,unknown>,summary?:string,error?:string):Attempt=>({
 profile_id:'profile',node_key:'node',node_identity_key:'identity',config_revision_key:'revision',display_name:'node',attempt_id:serviceId+finishedAt,request_id:'round:'+serviceId,service_id:serviceId,requested_at:finishedAt,finished_at:finishedAt,execution_state:'completed',persistence_state:'saved',rule:{service_id:serviceId,target_url:'https://example.test/profile'},result:{outcome,bytes_read:0,finished_at:finishedAt,details,summary,error_message:error}
})

it('accepts no attempt prop and reports that the profile has not been queried',()=>{
 const wrapper=mount(NodeExitSummary,{global:{stubs:{Modal:modalStub}}});expect(wrapper.get('.node-exit-summary').text()).toContain('未查询');expect(wrapper.get('.node-exit-open').text()).toContain('查看全部来源（0）');wrapper.unmount()
})

it('shows separate family rows and keeps long summaries and failure evidence in the modal',async()=>{
 const ipv4Old=attempt('ipok_ipv4','2026-10-17T10:00:00Z','profiled',{ip:'198.51.100.7',ip_family:'IPv4',country_code:'US',ip_type:'住宅'},'旧画像')
 const ipv4Latest=attempt('ipok_ipv4','2026-10-17T11:00:00Z','unknown',undefined,'IPOK 这是一段不应进入列表的失败摘要','IPOK 请求超时')
 const ipv6=attempt('ipok_ipv6','2026-10-17T10:30:00Z','profiled',{ip:'2001:db8::7',ip_family:'IPv6',country_code:'SG',location:'Singapore',ip_type:'机房 IDC'},'第三方风险分 20 · 机房 IDC')
 const wrapper=mount(NodeExitSummary,{props:{attempts:[ipv4Old,ipv4Latest,ipv6]},global:{stubs:{Modal:modalStub}}})
 const summary=wrapper.get('.node-exit-summary'),lines=wrapper.findAll('.node-exit-summary-record')
 expect(summary.element.tagName).not.toBe('BUTTON');expect(lines).toHaveLength(2)
 expect(lines[0].get('.node-exit-summary-main').text()).toContain('IPv4 · 地区未知 · 类型未知')
 expect(lines[0].get('.node-exit-summary-source').text()).toContain('IPOK');expect(lines[0].get('.node-exit-summary-source').text()).toContain('未确认')
 expect(lines[1].get('.node-exit-summary-main').text()).toContain('IPv6 · SG · 机房 IDC')
 expect(summary.text()).not.toContain('IPOK 这是一段不应进入列表的失败摘要');expect(summary.text()).not.toContain('第三方风险分 20 · 机房 IDC')
 expect(summary.text()).not.toContain('US');expect(summary.text()).not.toContain('住宅');expect(summary.text()).not.toContain('纯净')
 await wrapper.get('.node-exit-open').trigger('click')
 const modal=wrapper.get('.modal-stub').text();expect(modal).toContain('IPOK 请求超时');expect(modal).toContain('IPOK 这是一段不应进入列表的失败摘要');expect(modal).toContain('第三方风险分 20 · 机房 IDC');expect(modal).toContain('2001:db8::7');expect(modal).toContain('2026-10-17T11:00:00Z');expect(modal).toContain('原始服务记录')
 wrapper.unmount()
})

it('uses the query timestamp as the exit observation fallback and reads string booleans',async()=>{
 const queried=attempt('exit_ipv4','2026-10-17T12:00:00Z','profiled',{ip:'198.51.100.8',ip_family:'IPv4',query_at:'2026-10-17T11:59:00Z',is_idc:'true'},'IPv4 出口 198.51.100.8')
 const residential=attempt('ippure_ip_quality','2026-10-17T12:01:00Z','profiled',{ip:'2001:db8::8',is_residential:'false'},'第三方欺诈分 10 · 非住宅')
 const wrapper=mount(NodeExitSummary,{props:{attempts:[queried,residential]},global:{stubs:{Modal:modalStub}}})
 await wrapper.get('.node-exit-open').trigger('click')
 const modal=wrapper.get('.modal-stub').text()
 expect(modal).toContain('机房 IDC');expect(modal).toContain('非住宅');expect(modal).toContain('查询 2026-10-17T11:59:00Z · 出口观察 2026-10-17T11:59:00Z');expect(modal).not.toContain('出口观察 未提供')
 wrapper.unmount()
})
