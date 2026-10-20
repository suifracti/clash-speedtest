import {mount,flushPromises} from '@vue/test-utils'
import {ref} from 'vue'
import {expect,it,vi} from 'vitest'
import AirportReport from '../components/AirportReport.vue'
import RecommendationView from '../components/RecommendationView.vue'
import {workspaceKey} from '../workspace'
import {readQualityBatch} from '../qualityBatch'
import {key,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
vi.mock('../qualityBatch',()=>({readQualityBatch:vi.fn(async()=>({}))}))
vi.mock('../api',()=>({api:{settings:vi.fn(async()=>({}))}}))
vi.mock('../usageDaily',async original=>({...await original<typeof import('../usageDaily')>(),readUsage:vi.fn(async()=>({accounts:[],intervals:[]}))}))
const nodes=['a','b'].map(profile_id=>({profile_id,node_identity_key:profile_id,config_revision_key:'r',display_name:'Node'}) as NodeOption)
const w={hours:ref(24),project:ref('combined'),inspectNode:ref<NodeOption|null>(null),page:ref('recommend'),airports:ref([{id:'a',name:'A'},{id:'b',name:'B'}]),nodes:ref(nodes),catalog:ref<unknown[]>([]),nodeAirportId:(n:NodeOption)=>n.profile_id,nodeAirportName:()=>'',nodeSubscriptionName:()=>''}
const global={provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}
it('reads only the selected airport and reads again when it changes',async()=>{
 vi.mocked(readQualityBatch).mockClear();const wrapper=mount(AirportReport,{global});await flushPromises()
 expect(readQualityBatch).toHaveBeenLastCalledWith([nodes[0]],expect.any(String),expect.any(String))
 expect(wrapper.text()).toContain('这个机场近 30 天没有检测记录');expect(wrapper.text()).not.toContain('0 / 0');expect(wrapper.find('.report-metrics').exists()).toBe(false)
 await wrapper.get('select').setValue('b');await flushPromises()
 expect(readQualityBatch).toHaveBeenCalledTimes(2);expect(readQualityBatch).toHaveBeenLastCalledWith([nodes[1]],expect.any(String),expect.any(String));wrapper.unmount()
})
it('reads seven days for recommendations and labels that window',async()=>{
 vi.mocked(readQualityBatch).mockClear();const wrapper=mount(RecommendationView,{global});await flushPromises()
 const [selected,since,until]=vi.mocked(readQualityBatch).mock.calls[0]!
 expect(selected).toEqual(nodes);expect(Date.parse(until)-Date.parse(since)).toBe(7*86400000);expect(wrapper.text()).toContain('近 7 天');wrapper.unmount()
})
it('uses one empty card, and a single line for a missing purpose beside a real candidate',async()=>{
 let wrapper=mount(RecommendationView,{global});await flushPromises();expect(wrapper.findAll('.recommendation-empty')).toHaveLength(1);expect(wrapper.text()).not.toContain('导出 Clash');await wrapper.get('.recommendation-empty button').trigger('click');expect(w.page.value).toBe('home');wrapper.unmount()
 w.catalog.value=[{service_id:'ai',category:'AI',name:'AI',result_kind:'account_availability'}]
 vi.mocked(readQualityBatch).mockResolvedValueOnce({[key(nodes[0])]:{tests:[],monitor:[],downloads:[],services:[{service_id:'ai',finished_at:'2026-10-19T00:00:00Z',node_key:'old',config_revision_key:'old-revision',execution_state:'completed',rule:{name:'AI'},result:{outcome:'matched'}}],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true} as unknown as QualityEvidence})
 wrapper=mount(RecommendationView,{global});await flushPromises();expect(wrapper.findAll('.recommendation-missing')).toHaveLength(3);expect(wrapper.findAll('.recommendation-missing button')).toHaveLength(0);expect(wrapper.text()).toContain('近 7 天没有流媒体站点通过的记录');await wrapper.get('article button').trigger('click');expect(w.inspectNode.value?.config_revision_key).toBe('old-revision');expect(w.hours.value).toBe(168);expect(w.project.value).toBe('service');wrapper.unmount()
})
