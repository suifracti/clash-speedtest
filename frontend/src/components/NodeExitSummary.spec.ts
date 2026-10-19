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
 const ipv4Old=attempt('ping0_ipv4','2026-10-17T10:00:00Z','profiled',{ip:'198.51.100.7',ip_family:'IPv4',country_code:'US',ip_type:'住宅'},'旧画像')
 const ipv4Latest=attempt('ping0_ipv4','2026-10-17T11:00:00Z','unknown',undefined,'Ping0 这是一段不应进入列表的失败摘要','Ping0 请求超时')
 const ipv6=attempt('ping0_ipv6','2026-10-17T10:30:00Z','profiled',{ip:'2001:db8::7',ip_family:'IPv6',country_code:'SG',location:'Singapore',ip_type:'机房 IDC'},'第三方风险分 20 · 机房 IDC')
 const wrapper=mount(NodeExitSummary,{props:{attempts:[ipv4Old,ipv4Latest,ipv6]},global:{stubs:{Modal:modalStub}}})
 const summary=wrapper.get('.node-exit-summary'),lines=wrapper.findAll('.node-exit-summary-record').sort((a,b)=>Number(b.text().includes('Ping0 请求超时'))-Number(a.text().includes('Ping0 请求超时')))
 expect(summary.element.tagName).not.toBe('BUTTON');expect(lines).toHaveLength(2)
 expect(lines[0].find('.node-exit-summary-main').exists()).toBe(false)
 expect(lines[0].get('.node-exit-summary-source').text()).toContain('Ping0');expect(lines[0].get('.node-exit-summary-source').text()).toContain('查询未完成：Ping0 请求超时')
 expect(lines[1].get('.node-exit-summary-main').text()).toContain('IPv6 · SG · 机房 IDC')
 expect(summary.text()).not.toContain('Ping0 这是一段不应进入列表的失败摘要');expect(summary.text()).not.toContain('第三方风险分 20 · 机房 IDC')
 expect(summary.text()).not.toContain('US');expect(summary.text()).not.toContain('住宅');expect(summary.text()).not.toContain('纯净')
 await wrapper.get('.node-exit-open').trigger('click')
 const modal=wrapper.get('.modal-stub').text();expect(modal).toContain('Ping0 请求超时');expect(modal).toContain('Ping0 这是一段不应进入列表的失败摘要');expect(modal).toContain('第三方风险分 20 · 机房 IDC');expect(modal).toContain('2001:db8::7');expect(modal).toContain('2026-10-17T11:00:00Z');expect(modal).toContain('原始服务记录');wrapper.unmount()
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
 expect(wrapper.find('.exit-ledger-address').exists()).toBe(false)
 const ping=wrapper.get('[data-provider="ping0_ip_quality"]'),pure=wrapper.get('[data-provider="ippure_ip_quality"]');expect(ping.get('.node-exit-compact-risk').text()).toBe('25');expect(ping.text()).toContain('IPv6');expect(pure.text()).toContain('验证页拦截');expect(pure.attributes('title')).toContain('未取得风险分');expect(pure.text()).not.toContain('25');expect(pure.find('.node-exit-risk-scale').exists()).toBe(false);expect(wrapper.text()).not.toContain('198.51.100.60');wrapper.unmount()
})
it('preserves both zero scores with source-specific scales and keeps raw details',async()=>{
 const ping=attempt('ping0_ip_quality','2026-10-18T10:00:00Z','profiled',{ip:'2001:db8::61',risk_score:0,risk_tier:'vendor-low'}),pure=attempt('ippure_ip_quality','2026-10-18T10:01:00Z','profiled',{ip:'198.51.100.61',fraud_score:0,risk_scale_max:'100',risk_direction:'lower_is_better'})
 const wrapper=mount(NodeExitSummary,{props:{attempts:[ping,pure],compact:true},global:{stubs:{Modal:modalStub}}})
 for(const id of ['ping0_ip_quality','ippure_ip_quality']){const row=wrapper.get(`[data-provider="${id}"]`);expect(row.get('.node-exit-compact-risk').text()).toBe('0');expect(row.get('.node-exit-risk-scale').attributes('style')).toContain('0%');expect(row.get('.node-exit-risk-scale').attributes('aria-label')).toContain('越低越好')}
 await wrapper.get('[data-provider="ping0_ip_quality"]').trigger('click');expect(wrapper.get('.modal-stub').text()).toContain('风险等级vendor-low');expect(wrapper.get('.modal-stub').text()).toContain('风险0/100');wrapper.unmount()
})
it('does not invent an IPPure scale without the provider metadata',()=>{
 const pure=attempt('ippure_ip_quality','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.70',fraud_score:0});const wrapper=mount(NodeExitSummary,{props:{compact:true,attempts:[pure]}}),row=wrapper.get('[data-provider="ippure_ip_quality"]');expect(row.text()).toContain('0');expect(row.attributes('title')).toContain('刻度未声明');expect(row.text()).not.toContain('/100');expect(row.find('.node-exit-risk-scale').exists()).toBe(false);wrapper.unmount()
})
it('keeps failed IPv4 lookup evidence available in details after hiding compact address rows',async()=>{
 const failed=attempt('exit_ipv4','2026-10-18T11:00:00Z','timed_out',undefined,undefined,'timeout'),ping=attempt('ping0_ip_quality','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.70',risk_score:25,country:'SG'})
 const wrapper=mount(NodeExitSummary,{props:{compact:true,attempts:[failed,ping]},global:{stubs:{Modal:modalStub}}});expect(wrapper.find('.exit-ledger-address').exists()).toBe(false);await wrapper.get('.node-exit-provider').trigger('click');const failedRow=wrapper.findAll('.node-exit-evidence').find(row=>row.text().includes('timeout'))!;expect(failedRow.text()).toContain('查询未完成：timeout');expect(failedRow.text()).not.toContain('SG');wrapper.unmount()
})
it('opens unqueried saved details without issuing IP queries',async()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch)
 const wrapper=mount(NodeExitSummary,{props:{compact:true},global:{stubs:{Modal:modalStub}}})
 try{expect(wrapper.text()).toContain('出口 / 信誉未查询');await wrapper.get('.node-exit-compact').trigger('click');expect(wrapper.get('.modal-stub').text()).toContain('未查询');expect(fetch).not.toHaveBeenCalled()}finally{wrapper.unmount();vi.unstubAllGlobals()}
})

it('keeps unqueried compact nodes to one entry instead of empty family and provider cards',()=>{
 const wrapper=mount(NodeExitSummary,{props:{compact:true}})
 expect(wrapper.text()).toContain('出口 / 信誉未查询')
 expect(wrapper.findAll('.exit-ledger-address')).toHaveLength(0)
 expect(wrapper.findAll('.node-exit-provider')).toHaveLength(0)
 wrapper.unmount()
})

it('shows compact Ping0 gauges independently for both families, including zero from saved v1 records',()=>{
 const v4=attempt('ping0_ipv4','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.80',risk_score:0,country:'Singapore'}),v6=attempt('ping0_ipv6','2026-10-18T10:00:00Z','profiled',{ip:'2001:db8::80',risk_score:65,country:'Japan'})
 const w=mount(NodeExitSummary,{props:{compact:true,attempts:[v4,v6]}})
 expect(w.get('[data-provider="ping0_ipv4"] .node-exit-compact-risk').text()).toBe('0')
 expect(w.get('[data-provider="ping0_ipv6"] .node-exit-risk-scale').attributes('style')).toContain('65%')
 expect(w.find('.exit-ledger-address').exists()).toBe(false);expect(w.get('[data-provider="ping0_ipv4"]').text()).toContain('IPv4');expect(w.get('[data-provider="ping0_ipv4"]').text()).not.toContain('65')
 expect(w.text()).not.toContain('↗');expect(w.find('.node-exit-provider time').exists()).toBe(false);expect(w.find('.node-exit-provider-caption').exists()).toBe(false);w.unmount()
})

it('keeps address-only records accessible without the two compact address lines',async()=>{
 const exit=attempt('exit_ipv4','2026-10-18T10:00:00Z','profiled',{ip:'198.51.100.90',country:'Singapore'})
 const wrapper=mount(NodeExitSummary,{props:{compact:true,attempts:[exit]},global:{stubs:{Modal:modalStub}}})
 expect(wrapper.find('.exit-ledger-address').exists()).toBe(false)
 await wrapper.get('.node-exit-compact').trigger('click')
 expect(wrapper.get('.modal-stub').text()).toContain('198.51.100.90');wrapper.unmount()
})

it('shows the saved TLS failure stage without turning it into a missing IP score',async()=>{
 const ping=attempt('ping0_ip_quality','2026-10-19T06:59:52Z','transport_error',{tls_handshake_failures:'1',http_requests_written:'0',tls_handshake_error:'对端或节点代理过早关闭连接 (EOF)'},undefined,'节点代理或 HTTP 传输失败: EOF')
 const wrapper=mount(NodeExitSummary,{props:{compact:true,attempts:[ping]},global:{stubs:{Modal:modalStub}}})
 expect(wrapper.get('.node-exit-provider-status').text()).toBe('TLS 连接失败')
 await wrapper.get('.node-exit-provider').trigger('click')
 expect(wrapper.get('.modal-stub').text()).toContain('TLS 握手失败：对端或节点代理过早关闭连接 (EOF)');wrapper.unmount()
})

it('shows separate Ping0 families, hides IPOK and removes the unused IPPure IPv6 note',()=>{
 const v4=attempt('ping0_ipv4','2026-10-19T07:00:00Z','profiled',{ip:'198.51.100.1',risk_score:28}),v6=attempt('ping0_ipv6','2026-10-19T07:00:00Z','profiled',{ip:'2001:db8::1',risk_score:21}),pure=attempt('ippure_ip_quality','2026-10-19T07:00:00Z','profiled',{ip:'198.51.100.1',fraud_score:61,risk_scale_max:100,risk_direction:'lower_is_better'}),retired=attempt('ipok_ipv4','2026-10-19T07:00:00Z','profiled',{ip:'198.51.100.1',risk_score:65})
 const legacy=attempt('ping0_ip_quality','2026-10-18T07:00:00Z','profiled',{ip:'2001:db8::1',risk_score:80})
 const wrapper=mount(NodeExitSummary,{props:{compact:true,attempts:[v4,v6,pure,retired,legacy]}})
 expect(wrapper.get('[data-provider="ping0_ipv4"]').text()).toContain('28')
 expect(wrapper.get('[data-provider="ping0_ipv6"]').text()).toContain('21')
 expect(wrapper.text()).not.toContain('IPOK');expect(wrapper.find('[data-provider="ping0_ip_quality"]').exists()).toBe(false)
 expect(wrapper.find('[data-provider="ippure_ipv6"]').exists()).toBe(false)
 wrapper.unmount()
})

it('groups IPPure risk, independent Cloudflare coefficient and ASN traffic without mixing scores',()=>{
 const pure=attempt('ippure_ip_quality','2026-10-19T07:00:00Z','profiled',{ip:'198.51.100.1',fraud_score:0,risk_scale_max:100,risk_direction:'lower_is_better',cf_risk_score:61,cf_ip_family:'IPv6',cf_status:'已取得',asn_traffic_status:'已取得',asn_traffic_asn:'AS15169',asn_human_percent:'5.78',asn_bot_percent:'94.22'})
 const w=mount(NodeExitSummary,{props:{compact:true,attempts:[pure]}}),group=w.get('.ippure-group')
 expect(group.get('[data-provider="ippure_ip_quality"] .node-exit-compact-risk').text()).toBe('0')
 expect(group.get('[data-metric="cf"] .node-exit-compact-risk').text()).toBe('61')
 expect(group.get('[data-metric="asn-traffic"]').text()).toContain('人类 5.78%')
 expect(group.get('[data-metric="asn-traffic"]').text()).toContain('机器 94.22%')
 expect(group.get('[data-metric="asn-traffic"]').attributes('title')).toContain('整个ASN')
 w.unmount()
})
