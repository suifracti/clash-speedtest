import {mount,flushPromises} from '@vue/test-utils'
import {ref} from 'vue'
import {expect,it,vi} from 'vitest'
import AirportReport from '../components/AirportReport.vue'
import RecommendationView from '../components/RecommendationView.vue'
import {workspaceKey} from '../workspace'
import {readQualityBatch} from '../qualityBatch'
import type {NodeOption} from '../domain'
vi.mock('../qualityBatch',()=>({readQualityBatch:vi.fn(async()=>({}))}))
vi.mock('../api',()=>({api:{settings:vi.fn(async()=>({}))}}))
vi.mock('../usageDaily',async original=>({...await original<typeof import('../usageDaily')>(),readUsage:vi.fn(async()=>({accounts:[],intervals:[]}))}))
const nodes=['a','b'].map(profile_id=>({profile_id,node_identity_key:profile_id,config_revision_key:'r',display_name:'Node'}) as NodeOption)
const w={airports:ref([{id:'a',name:'A'},{id:'b',name:'B'}]),nodes:ref(nodes),catalog:ref([]),nodeAirportId:(n:NodeOption)=>n.profile_id}
const global={provide:{[workspaceKey as symbol]:w},stubs:{TrendChart:true}}
it('reads only the selected airport and reads again when it changes',async()=>{
 vi.mocked(readQualityBatch).mockClear();const wrapper=mount(AirportReport,{global});await flushPromises()
 expect(readQualityBatch).toHaveBeenLastCalledWith([nodes[0]],expect.any(String),expect.any(String))
 await wrapper.get('select').setValue('b');await flushPromises()
 expect(readQualityBatch).toHaveBeenCalledTimes(2);expect(readQualityBatch).toHaveBeenLastCalledWith([nodes[1]],expect.any(String),expect.any(String));wrapper.unmount()
})
it('reads seven days for recommendations and labels that window',async()=>{
 vi.mocked(readQualityBatch).mockClear();const wrapper=mount(RecommendationView,{global});await flushPromises()
 const [selected,since,until]=vi.mocked(readQualityBatch).mock.calls[0]!
 expect(selected).toEqual(nodes);expect(Date.parse(until)-Date.parse(since)).toBe(7*86400000);expect(wrapper.text()).toContain('近 7 天');wrapper.unmount()
})
