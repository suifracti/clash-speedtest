import {expect,it,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import AirportReport from '../components/AirportReport.vue'
import UiSelect from '../components/UiSelect.vue'
import {api} from '../api'
import {createWorkspace,workspaceKey} from '../workspace'
import {readQualityBatch} from '../qualityBatch'
vi.mock('../qualityBatch',()=>({readQualityBatch:vi.fn()}))
vi.mock('../usageDaily',async original=>({...await original<typeof import('../usageDaily')>(),readUsage:vi.fn(async()=>({accounts:[],intervals:[]}))}))
import {airportReport} from '../airportReport'
import {key,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
it('summarizes observed node rounds and leaves missing history unknown',()=>{
 const node={profile_id:'p',node_identity_key:'i',config_revision_key:'r',node_key:'n'} as NodeOption,at='2026-10-20T10:00:00Z'
 const evidence={tests:[{attempt_id:'good',samples:[{seq:1,target:'cloudflare',timestamp:at,success:true,latency_ms:10}]},{attempt_id:'bad',samples:[{seq:1,target:'cloudflare',timestamp:at,success:false,latency_ms:0,error:'timeout'}]}],monitor:[],reads:{latency:{complete:true},monitor:{complete:true}}} as unknown as QualityEvidence
 const report=airportReport([node],{[key(node)]:evidence});expect(report.rate).toBe(50);expect(report.total).toBe(2);expect(report.failures).toEqual([['超时',1]]);expect(report.trend[0].value).toBe(10)
 const missing=airportReport([node],{});expect(missing.rate).toBeNull();expect(missing.incomplete).toBe(true);expect(missing.trend).toEqual([])
})

it('filters the report and ranking together, sorts rows and opens the inspector',async()=>{
 const make=(id:string,country:string,name:string)=>({profile_id:'p',node_identity_key:id,config_revision_key:'r',node_key:id,country_code:country,display_name:name,type:'Hysteria2'} as NodeOption)
 const nodes=[make('fast','JP','日本专线01'),make('slow','JP','日本专线02'),make('other','SG','新加坡高速01')],at='2026-10-20T10:00:00Z'
 const evidence=Object.fromEntries(nodes.map((n,i)=>[key(n),{tests:[{...n,attempt_id:n.node_key,requested_at:at,finished_at:at,total_samples:3,success_samples:3,samples:[1000,200+i*200,200+i*200].map((ms,j)=>({seq:j+1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,success:true,latency_ms:ms}))}],monitor:[],reads:{latency:{complete:true},monitor:{complete:true}}}])) as unknown as Record<string,QualityEvidence>
 const slow=evidence[key(nodes[1]!)].tests[0]!;evidence[key(nodes[1]!)].tests=[0,1,2].map(i=>({...slow,attempt_id:'slow-'+i}))
 const settings=vi.spyOn(api,'settings').mockResolvedValue({} as never);vi.mocked(readQualityBatch).mockResolvedValue(evidence)
 const w=createWorkspace();w.airports.value=[{id:'a',name:'机场',subscriptions:[{id:'p',airport_id:'a'}]}] as never;w.nodes.value=nodes
 const wrapper=mount(AirportReport,{global:{provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}});await flushPromises()
 const selects=wrapper.get('.report-filters').findAllComponents(UiSelect)
 selects[0]!.vm.$emit('update:modelValue','JP');selects[1]!.vm.$emit('update:modelValue','专线');selects[2]!.vm.$emit('update:modelValue','Hysteria2');await flushPromises()
 expect(wrapper.get('.report-filters').text()).toContain('2 / 3 个节点')
 expect(wrapper.get('.report-metrics').text()).toContain('12/12 次');expect(wrapper.findAll('.rank-node').map(r=>r.text())).toEqual(['日本 · 02 · 专线','日本 · 01 · 专线'])
 expect(wrapper.get('th[aria-sort="descending"]').text()).toContain('响应率');expect(wrapper.get('thead').text()).toContain('轮数');expect(wrapper.findAll('.rank-table-wrap tbody tr')[0]!.findAll('td')[1]!.classes()).toContain('good');expect(wrapper.findAll('.rank-table-wrap tbody tr')[0]!.findAll('td')[2]!.text()).toBe('3');expect(wrapper.findAll('.rank-table-wrap tbody tr')[0]!.findAll('td')[4]!.text()).toBe('—')
 expect(wrapper.findAll('.rank-low')).toHaveLength(1);expect(wrapper.findAll('.rank-low')[0]!.text()).toBe('轮数少')
 expect(wrapper.findAll('.beat-cell.good')).toHaveLength(1);expect(readQualityBatch).toHaveBeenCalledTimes(1)
 await wrapper.findAll('th button').find(b=>b.text().startsWith('稳定延迟'))!.trigger('click')
 expect(wrapper.get('th[aria-sort="ascending"]').text()).toContain('稳定延迟');expect(wrapper.findAll('.rank-node')[0]!.text()).toContain('02')
 await wrapper.findAll('th button').find(b=>b.text().startsWith('稳定延迟'))!.trigger('click');expect(wrapper.findAll('.rank-node')[0]!.text()).toContain('02')
 await wrapper.findAll('.rank-node')[0]!.trigger('click');expect(w.viewRequest.value?.node_key).toBe('slow');expect(w.page.value).toBe('home')
 wrapper.unmount();w.dispose();settings.mockRestore()
})
