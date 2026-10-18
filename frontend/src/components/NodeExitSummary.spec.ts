import {mount} from '@vue/test-utils'
import {expect,it,vi} from 'vitest'
import type {Attempt} from '../domain'
import NodeExitSummary from './NodeExitSummary.vue'
const modalStub={template:'<div class="modal-stub"><slot/></div>'}
const attempt=(serviceId:string,finishedAt:string,outcome:string,details?:Record<string,unknown>,summary?:string,error?:string):Attempt=>({
 profile_id:'profile',node_key:'node',node_identity_key:'identity',config_revision_key:'revision',display_name:'node',attempt_id:serviceId+finishedAt,request_id:'round:'+serviceId,service_id:serviceId,requested_at:finishedAt,finished_at:finishedAt,execution_state:'completed',persistence_state:'saved',rule:{service_id:serviceId,target_url:'https://example.test/profile'},result:{outcome,bytes_read:0,finished_at:finishedAt,details,summary,error_message:error}
})
it('accepts no attempt prop and reports that the profile has not been queried',()=>{
 const wrapper=mount(NodeExitSummary,{global:{stubs:{Modal:modalStub}}});expect(wrapper.get('.node-exit-summary').text()).toContain('未查询');expect(wrapper.get('.node-exit-open').text()).toContain('来源详情（0）');wrapper.unmount()
})
it('shows separate family rows and keeps long summaries and failure evidence in the modal',async()=>{
 const ipv4Old=attempt('ipok_ipv4','2026-10-17T10:00:00Z','profiled',{ip:'198.51.100.7',ip_family:'IPv4',country_code:'US',ip_type:'住宅'},'旧画像')
 const ipv4Latest=attempt('ipok_ipv4','2026-10-17T11:00:00Z','unknown',undefined,'IPOK 这是一段不应进入列表的失败摘要','IPOK 请求超时')
 const ipv6=attempt('ipok_ipv6','2026-10-17T10:30:00Z','profiled',{ip:'2001:db8::7',ip_family:'IPv6',country_code:'SG',location:'Singapore',ip_type:'机房 IDC'},'第三方风险分 20 · 机房 IDC')
 const wrapper=mount(NodeExitSummary,{props:{attempts:[ipv4Old,ipv4Latest,ipv6]},global:{stubs:{Modal:modalStub}}})
 const summary=wrapper.get('.node-exit-summary'),lines=wrapper.findAll('.node-exit-summary-record').sort((a,b)=>Number(b.text().includes('IPOK 请求超时'))-Number(a.text().includes('IPOK 请求超时')))
 expect(summary.element.tagName).not.toBe('BUTTON');expect(lines).toHaveLength(2)
 expect(lines[0].find('.node-exit-summary-main').exists()).toBe(false)
 expect(lines[0].get('.node-exit-summary-source').text()).toContain('IPOK');expect(lines[0].get('.node-exit-summary-source').text()).toContain('查询未完成：IPOK 请求超时')
 expect(lines[1].get('.node-exit-summary-main').text()).toContain('IPv6 · SG · 机房 IDC')
 expect(summary.text()).not.toContain('IPOK 这是一段不应进入列表的失败摘要');expect(summary.text()).not.toContain('第三方风险分 20 · 机房 IDC')
 expect(summary.text()).not.toContain('US');expect(summary.text()).not.toContain('住宅');expect(summary.text()).not.toContain('纯净')
 await wrapper.get('.node-exit-open').trigger('click')
 const modal=wrapper.get('.modal-stub').text();expect(modal).toContain('IPOK 请求超时');expect(modal).toContain('IPOK 这是一段不应进入列表的失败摘要');expect(modal).toContain('第三方风险分 20 · 机房 IDC');expect(modal).toContain('2001:db8::7');expect(modal).toContain('2026-10-17T11:00:00Z');expect(modal).toContain('原始服务记录');wrapper.unmount()
})
it('uses the query timestamp as the exit observation fallback and reads string booleans',async()=>{
 const queried=attempt('exit_ipv4','2026-10-17T12:00:00Z','profiled',{ip:'198.51.100.8',ip_family:'IPv4',query_at:'2026-10-17T11:59:00Z',is_idc:'true'},'IPv4 出口 198.51.100.8')
 const residential=attempt('ippure_ip_quality','2026-10-17T12:01:00Z','profiled',{ip:'2001:db8::8',is_residential:'false'},'第三方欺诈分 10 · 非住宅')
 const wrapper=mount(NodeExitSummary,{props:{attempts:[queried,residential]},global:{stubs:{Modal:modalStub}}});await wrapper.get('.node-exit-open').trigger('click')
 const modal=wrapper.get('.modal-stub').text();expect(modal).toContain('机房 IDC');expect(modal).toContain('非住宅');expect(modal).toContain('查询时间2026-10-17T11:59:00Z');expect(modal).toContain('出口观察时间2026-10-17T11:59:00Z');expect(modal).not.toContain('未提供');wrapper.unmount()
})
it('shows vendor risk and matching address details with their original sources',()=>{
 const exit=attempt('exit_ipv4','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.40',ip_family:'IPv4',country:'新加坡'})
 const ping0=attempt('ping0_ip_quality','2026-10-18T10:01:00Z','profiled',{ip:'198.51.100.40',ip_family:'IPv4',country:'新加坡',asn:'AS16509',organization:'Amazon.com, Inc.',ip_type:'机房 IDC',risk_score:25})
 const ippure=attempt('ippure_ip_quality','2026-10-18T10:02:00Z','profiled',{ip:'198.51.100.40',ip_family:'IPv4',fraud_score:0})
 const wrapper=mount(NodeExitSummary,{props:{attempts:[exit,ping0,ippure]},global:{stubs:{Modal:modalStub}}});const rows=wrapper.findAll('.node-exit-summary-record')
 expect(rows).toHaveLength(1);expect(rows[0].get('.node-exit-summary-main').text()).toContain('IPv4 · 新加坡 · 机房 IDC');expect(rows[0].text()).toContain('Ping0');expect(rows[0].text()).toContain('风险分 25');expect(rows[0].text()).not.toContain('Amazon.com, Inc.');expect(rows[0].text()).toContain('IPPure');expect(rows[0].text()).toContain('风险分 0');wrapper.unmount()
})
it('keeps different IPs and address families independent and labels exit-only results',()=>{
 const exit=attempt('exit_ipv4','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.41',ip_family:'IPv4',country:'美国',ip_type:'住宅'})
 const ping0=attempt('ping0_ip_quality','2026-10-18T10:01:00Z','profiled',{ip:'198.51.100.42',ip_family:'IPv4',country:'新加坡',ip_type:'机房 IDC',risk_score:25})
 const ipv6=attempt('ippure_ip_quality','2026-10-18T10:02:00Z','profiled',{ip:'2001:db8::42',ip_family:'IPv6',risk_score:0})
 const wrapper=mount(NodeExitSummary,{props:{attempts:[exit,ping0,ipv6]},global:{stubs:{Modal:modalStub}}});const rows=wrapper.findAll('.node-exit-summary-record');expect(rows).toHaveLength(3)
 const exitRow=rows.find(row=>row.text().includes('198.51.100.41'))!,pingRow=rows.find(row=>row.text().includes('198.51.100.42'))!
 expect(exitRow.text()).toContain('已取得地址，尚无信誉结果');expect(exitRow.text()).toContain('美国 · 住宅');expect(exitRow.text()).not.toContain('风险分 25');expect(pingRow.text()).toContain('新加坡 · 机房 IDC');expect(pingRow.text()).toContain('风险分 25');expect(pingRow.text()).not.toContain('美国');expect(rows.filter(row=>row.text().includes('IPv6'))).toHaveLength(1);expect(rows.find(row=>row.text().includes('IPv6'))!.text()).toContain('风险分 0');wrapper.unmount()
})
it('does not show an older success as current after the latest rule fails',async()=>{
 const old=attempt('ping0_ip_quality','2026-10-18T09:00:00Z','profiled',{ip:'198.51.100.50',ip_family:'IPv4',country:'新加坡',risk_score:18}),latest=attempt('ping0_ip_quality','2026-10-18T10:00:00Z','unknown',undefined,undefined,'Ping0 请求超时')
 const wrapper=mount(NodeExitSummary,{props:{attempts:[old,latest]},global:{stubs:{Modal:modalStub}}}),row=wrapper.get('.node-exit-summary-record');expect(row.text()).toContain('Ping0 请求超时');expect(row.text()).not.toContain('风险分 18');expect(row.text()).not.toContain('198.51.100.50');await wrapper.get('.node-exit-open').trigger('click')
 const modal=wrapper.get('.modal-stub').text();expect(modal).toContain('历史有效画像');expect(modal).toContain('风险18');expect(modal).toContain('198.51.100.50');expect(modal).toContain('Ping0 请求超时');wrapper.unmount()
})
it('labels IPPure challenges and omits fields with no supplied value in details',async()=>{
 const challenge=attempt('ippure_ip_quality','2026-10-18T10:00:00Z','challenge',undefined,undefined,'challenge page blocked');const wrapper=mount(NodeExitSummary,{props:{attempts:[challenge]},global:{stubs:{Modal:modalStub}}});expect(wrapper.get('.node-exit-summary-record').text()).toContain('验证页拦截');await wrapper.get('.node-exit-open').trigger('click')
 const modal=wrapper.get('.modal-stub').text();expect(modal).toContain('状态验证页拦截，未取得风险结果');expect(modal).not.toContain('未提供');expect(modal).toContain('challenge page blocked');wrapper.unmount()
})
// User correction: a failed IPPure result must stay as visible as Ping0.
it('keeps IPPure challenge visible beside Ping0 without borrowing the IPv6 score for IPv4',()=>{
 const ipv4=attempt('exit_ipv4','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.60',ip_family:'IPv4'})
 const ipv6=attempt('ping0_ip_quality','2026-10-18T10:01:00Z','profiled',{ip:'2001:db8::60',ip_family:'IPv6',country:'新加坡',ip_type:'机房 IDC',risk_score:25})
 const challenge=attempt('ippure_ip_quality','2026-10-18T10:02:00Z','challenge',undefined,undefined,'challenge page blocked')
 const wrapper=mount(NodeExitSummary,{props:{attempts:[ipv4,ipv6,challenge],compact:true},global:{stubs:{Modal:modalStub}}})
 const addresses=wrapper.findAll('.exit-ledger-address');expect(addresses).toHaveLength(2);expect(addresses[0].text()).toContain('IPv4');expect(addresses[0].text()).toContain('地区未返回');expect(addresses[0].text()).not.toContain('25');expect(addresses[0].text()).not.toContain('新加坡')
 const ping=wrapper.get('[data-provider="ping0_ip_quality"]'),pure=wrapper.get('[data-provider="ippure_ip_quality"]');expect(ping.text().replace(/\s/g,'')).toContain('25/100');expect(ping.text()).toContain('IPv6');expect(pure.text()).toContain('验证页拦截');expect(pure.text()).toContain('未取得风险分');expect(pure.text()).not.toContain('25');expect(pure.find('.node-exit-risk-scale').exists()).toBe(false);expect(wrapper.text()).not.toContain('198.51.100.60');wrapper.unmount()
})
it('preserves both zero scores with source-specific scales and keeps raw details',async()=>{
 const ping=attempt('ping0_ip_quality','2026-10-18T10:00:00Z','profiled',{ip:'2001:db8::61',risk_score:0,risk_tier:'vendor-low'}),pure=attempt('ippure_ip_quality','2026-10-18T10:01:00Z','profiled',{ip:'198.51.100.61',fraud_score:0,risk_scale_max:'100',risk_direction:'lower_is_better'})
 const wrapper=mount(NodeExitSummary,{props:{attempts:[ping,pure],compact:true},global:{stubs:{Modal:modalStub}}})
 for(const id of ['ping0_ip_quality','ippure_ip_quality']){const row=wrapper.get(`[data-provider="${id}"]`);expect(row.text().replace(/\s/g,'')).toContain('0/100');expect(row.get('.node-exit-risk-scale').attributes('style')).toContain('0%');expect(row.text()).toContain('越低越好')}
 await wrapper.get('.node-exit-open').trigger('click');expect(wrapper.get('.modal-stub').text()).toContain('风险等级vendor-low');expect(wrapper.get('.modal-stub').text()).toContain('风险0/100');wrapper.unmount()
})
it('does not invent an IPPure scale without the provider metadata',()=>{
 const pure=attempt('ippure_ip_quality','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.70',fraud_score:0});const wrapper=mount(NodeExitSummary,{props:{compact:true,attempts:[pure]}}),row=wrapper.get('[data-provider="ippure_ip_quality"]');expect(row.text()).toContain('风险分 0');expect(row.text()).toContain('刻度未声明');expect(row.text()).not.toContain('/100');expect(row.find('.node-exit-risk-scale').exists()).toBe(false);wrapper.unmount()
})
it('does not hide an explicit failed IPv4 lookup behind a successful reputation query',()=>{
 const failed=attempt('exit_ipv4','2026-10-18T11:00:00Z','timed_out',undefined,undefined,'timeout'),ping=attempt('ping0_ip_quality','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.70',risk_score:25,country:'SG'})
 const wrapper=mount(NodeExitSummary,{props:{compact:true,attempts:[failed,ping]}});expect(wrapper.get('[data-family="IPv4"]').text()).toContain('查询超时');expect(wrapper.get('[data-family="IPv4"]').text()).not.toContain('SG');wrapper.unmount()
})
it('opens saved details without issuing IP queries and retains both unqueried providers',async()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch)
 const wrapper=mount(NodeExitSummary,{props:{compact:true},global:{stubs:{Modal:modalStub}}})
 try{expect(wrapper.get('[data-provider="ping0_ip_quality"]').text()).toContain('未查询');expect(wrapper.get('[data-provider="ippure_ip_quality"]').text()).toContain('未查询');await wrapper.get('.node-exit-open').trigger('click');expect(fetch).not.toHaveBeenCalled()}finally{wrapper.unmount();vi.unstubAllGlobals()}
})
