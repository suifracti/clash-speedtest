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
 w=createWorkspace();w.project.value='service';w.nodes.value=[node];w.airports.value=[{id:'a',name:'fixture',has_cache:true,url_display:'',node_count:1,subscriptions:[{id:'p',airport_id:'a',name:'account',url_display:'',url_configured:false,node_count:1,has_cache:true}]}];w.setAirports(['a']);w.catalog.value=Array.from({length:82},(_,i)=>({service_id:'svc'+i,name:'服务'+i,category:i%2?'网页':'连接',description:'',result_kind:'connectivity',success_criterion:'fixture',timeout_seconds:10}));
 const saved:Attempt={...node,attempt_id:'saved',request_id:'saved',requested_at:new Date().toISOString(),service_id:'svc20',execution_state:'completed',persistence_state:'saved',rule:{service_id:'svc20',target_url:'https://example.test'},result:{outcome:'matched',bytes_read:0,finished_at:new Date().toISOString()}}
 vi.mocked(api.attemptHistory).mockImplementation(async kind=>({attempts:kind==='service'?[saved]:[],has_more:false,complete:true}))
 wrapper=mount(HomeView,{global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 expect(w.serviceIds.value).toEqual([]);expect(wrapper.findAll('.observatory-service-cell')).toHaveLength(0)
 expect(wrapper.find('[aria-label="服务显示模式"]').exists()).toBe(false)
 await wrapper.get('.service-picker-disclosure summary').trigger('click')
 await wrapper.get('input[aria-label="搜索检测服务"]').setValue('服务80')
 await wrapper.get('.service-picker-list input').setValue(true);await flushPromises()
 expect(w.serviceIds.value).toEqual(['svc80']);expect(w.displayServiceIds.value).toEqual(['svc80'])
 expect(wrapper.get('.quality-heading-cols').text()).toBe('服务80');expect(wrapper.findAll('.observatory-service-cell')).toHaveLength(1)
 expect(wrapper.get('.observatory-service-cell').text()).toContain('未测')
 await wrapper.get('.scope-run').trigger('click');expect(w.plan.value?.serviceIds).toEqual(['svc80']);w.plan.value=null
 w.project.value='combined';await flushPromises();expect(wrapper.get('.overview-services').text()).toContain('服务80');expect(wrapper.text()).not.toContain('精选服务')
 const reopened=createWorkspace();expect(reopened.serviceIds.value).toEqual(['svc80']);reopened.catalog.value=w.catalog.value;expect(reopened.displayServiceIds.value).toEqual(['svc80']);reopened.dispose()
 await wrapper.get('.service-picker-list input').setValue(false);await flushPromises();expect(w.serviceIds.value).toEqual([])
 const emptyReopened=createWorkspace();emptyReopened.catalog.value=w.catalog.value;expect(emptyReopened.serviceIds.value).toEqual([]);emptyReopened.dispose()
 w.project.value='service';w.serviceIds.value=w.catalog.value.slice(0,14).map(s=>s.service_id);await flushPromises()
 expect(wrapper.findAll('.observatory-service-cell')).toHaveLength(12);expect(wrapper.get('[aria-label="服务列分页"]').text()).toContain('1–12 / 14')
 await wrapper.get('button[aria-label="下一组服务"]').trigger('click');expect(wrapper.findAll('.observatory-service-cell')).toHaveLength(2)
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
