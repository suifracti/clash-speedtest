import {mount,flushPromises} from '@vue/test-utils'
import {afterEach,expect,it,vi} from 'vitest'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import type {Attempt,NodeOption} from '../domain'
vi.mock('../api',async original=>{const actual=await original<typeof import('../api')>();return {...actual,api:{...actual.api,refreshJobs:vi.fn(async()=>[]),measurementRounds:vi.fn(async()=>[]),latencyHistory:vi.fn(async()=>({tests:[],complete:true,has_more:false})),latencySummaries:vi.fn(async()=>[]),monitorSamples:vi.fn(async()=>({items:[],has_more:false})),attemptHistory:vi.fn(async()=>({attempts:[],complete:true,has_more:false})),startAttempt:vi.fn(),startLatency:vi.fn()}}})
let wrapper:ReturnType<typeof mount>|undefined,w:ReturnType<typeof createWorkspace>|undefined
afterEach(()=>{wrapper?.unmount();w?.dispose();window.localStorage.clear()})
it('uses one remembered selection for columns, overview and detection without starting requests',async()=>{
 window.localStorage.clear();vi.clearAllMocks();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
 const node:NodeOption={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',profile_name:'fixture',display_name:'fixture',type:'ss',country_code:'SG',country_flag:''}
 w=createWorkspace();w.project.value='service';w.nodes.value=[node];w.airports.value=[{id:'a',name:'fixture',has_cache:true,url_display:'',node_count:1,subscriptions:[{id:'p',airport_id:'a',name:'account',url_display:'',url_configured:false,node_count:1,has_cache:true}]}];w.setAirports(['a']);w.catalog.value=['chatgpt_unlock','netflix_unlock','abema_unlock'].map((id,i)=>({service_id:id,name:['ChatGPT','Netflix','ABEMA'][i],category:'fixture',description:'',result_kind:'unlock',success_criterion:'fixture',timeout_seconds:10}));
 const saved:Attempt={...node,attempt_id:'saved',request_id:'saved',requested_at:new Date().toISOString(),service_id:'svc20',execution_state:'completed',persistence_state:'saved',rule:{service_id:'svc20',target_url:'https://example.test'},result:{outcome:'matched',bytes_read:0,finished_at:new Date().toISOString()}}
 vi.mocked(api.attemptHistory).mockImplementation(async kind=>({attempts:kind==='service'?[saved]:[],has_more:false,complete:true}))
 wrapper=mount(HomeView,{global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 expect(w.serviceIds.value).toEqual(['chatgpt_unlock','netflix_unlock']);expect(wrapper.findAll('.hd-svc-cell')).toHaveLength(3)
 expect(wrapper.findAll('[role="columnheader"]').slice(-3).map(x=>x.text())).toEqual(['AI','流媒体','日本'])
 await wrapper.get('.hd-services summary').trigger('click')
 const japan=wrapper.findAll('.unlock-group-options label').find(x=>x.text().includes('日本'))!
 await japan.get('input').setValue(true);await flushPromises()
 expect(w.serviceIds.value).toEqual(['chatgpt_unlock','netflix_unlock','abema_unlock'])
 await wrapper.get('.hd-run').trigger('click');expect(w.plan.value?.serviceIds).toEqual(w.serviceIds.value);w.plan.value=null
 const reopened=createWorkspace();expect(reopened.serviceIds.value).toEqual(w.serviceIds.value);reopened.dispose()
 expect(wrapper.get('.unlock-group.legacy summary').text()).toContain('其他（旧检测项）')
 expect(wrapper.get('.unlock-group.legacy').attributes('open')).toBeUndefined()
 expect(api.startAttempt).not.toHaveBeenCalled();expect(api.startLatency).not.toHaveBeenCalled();vi.unstubAllGlobals()
})
it('does not preserve an automatic three-service recommendation as a user choice',()=>{
 window.localStorage.clear();window.localStorage.setItem('speedtest.display-services',JSON.stringify(['chatgpt_web','netflix_unlock','youtube_premium']))
 w=createWorkspace();expect(w.serviceIds.value).toEqual([]);expect(w.displayServiceIds.value).toEqual([])
})
it('does not promote old display-only choices into detection defaults',()=>{
 window.localStorage.clear();window.localStorage.setItem('speedtest.display-services',JSON.stringify(['cloudflare_204','grok_web']))
 w=createWorkspace();expect(w.serviceIds.value).toEqual([])
})
