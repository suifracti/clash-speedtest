import {mount,flushPromises} from '@vue/test-utils'
import {afterEach,expect,it,vi} from 'vitest'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import type {Attempt,NodeOption} from '../domain'
import {regionBadgeClass} from '../nodePresentation'
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
 expect(w.serviceIds.value).toEqual(['chatgpt_unlock','netflix_unlock','abema_unlock']);expect(wrapper.findAll('.hd-svc-cell')).toHaveLength(3)
 expect(wrapper.findAll('[role="columnheader"]').slice(-3).map(x=>x.text())).toEqual(['AI','流媒体','日本'])
 await wrapper.get('.hd-services summary').trigger('click')
 const japan=wrapper.findAll('.unlock-group-options label').find(x=>x.text().includes('日本'))!
 await japan.get('input').setValue(false);await flushPromises();expect(w.serviceIds.value).toEqual(['chatgpt_unlock','netflix_unlock'])
 await japan.get('input').setValue(true);await flushPromises()
 expect(w.serviceIds.value).toEqual(['chatgpt_unlock','netflix_unlock','abema_unlock'])
 await wrapper.get('.hd-run').trigger('click');expect(w.plan.value?.serviceIds).toEqual(w.serviceIds.value);w.plan.value=null
 const reopened=createWorkspace();expect(reopened.serviceIds.value).toEqual(w.serviceIds.value);reopened.dispose()
 await wrapper.get('.hd-node-open').trigger('click');await flushPromises()
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
it('saves and toggles custom unlock groups alongside default groups in UnlockPicker',async()=>{
 window.localStorage.clear();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
 const node:NodeOption={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',profile_name:'fixture',display_name:'fixture',type:'anytls',country_code:'TW',country_flag:''}
 w=createWorkspace();w.project.value='service';w.nodes.value=[node];w.airports.value=[{id:'a',name:'fixture',has_cache:true,url_display:'',node_count:1,subscriptions:[{id:'p',airport_id:'a',name:'account',url_display:'',url_configured:false,node_count:1,has_cache:true}]}];w.setAirports(['a']);w.catalog.value=['chatgpt_unlock','netflix_unlock','abema_unlock'].map((id,i)=>({service_id:id,name:['ChatGPT','Netflix','ABEMA'][i],category:'fixture',description:'',result_kind:'unlock',success_criterion:'fixture',timeout_seconds:10}));
 wrapper=mount(HomeView,{global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 expect(wrapper.get('.hd-proto-tag').classes()).toContain('proto-anytls')
 expect(wrapper.get('.hd-flag').classes()).toContain('region-tw')
 expect(wrapper.get('.hd-node').classes()).toContain('region-tw')
 expect(new Set(['HK','TW','JP','SG','KR','US','UK'].map(c=>regionBadgeClass(c))).size).toBe(7)
 await wrapper.get('.hd-services summary').trigger('click')
 w.serviceIds.value=['chatgpt_unlock','abema_unlock'];await flushPromises()
 await wrapper.get('input[aria-label="自定义分组名称"]').setValue('我的常测')
 await wrapper.findAll('button').find(b=>b.text()==='保存分组')!.trigger('click');await flushPromises()
 const customLabel=wrapper.findAll('.unlock-group-options label').find(x=>x.text().includes('我的常测'))!
 expect(customLabel).toBeDefined()
 expect(customLabel.text()).toContain('2 项')
 await customLabel.get('input').setValue(false);await flushPromises()
 expect(w.serviceIds.value).toEqual([])
 await customLabel.get('input').setValue(true);await flushPromises()
 expect(w.serviceIds.value).toEqual(['chatgpt_unlock','abema_unlock'])
 await customLabel.get('button[aria-label="删除自定义分组 我的常测"]').trigger('click');await flushPromises()
 expect(wrapper.findAll('.unlock-group-options label').find(x=>x.text().includes('我的常测'))).toBeUndefined()
 // Built-in groups (e.g. AI) can also be edited, deleted, and restored.
 await wrapper.get('button[aria-label="编辑分组 AI"]').trigger('click');await flushPromises()
 w.serviceIds.value=['chatgpt_unlock','netflix_unlock'];await flushPromises()
 await wrapper.get('input[aria-label="自定义分组名称"]').setValue('主力常用')
 await wrapper.findAll('button').find(b=>b.text()==='更新分组')!.trigger('click');await flushPromises()
 const renamedLabel=wrapper.findAll('.unlock-group-options label').find(x=>x.text().includes('主力常用'))!
 expect(renamedLabel.text()).toContain('2 项')
 await wrapper.get('button[aria-label="删除分组 流媒体"]').trigger('click');await flushPromises()
 expect(wrapper.findAll('.unlock-group-options label').find(x=>x.text().includes('流媒体'))).toBeUndefined()
 await wrapper.findAll('button').find(b=>b.text()==='恢复默认分组')!.trigger('click');await flushPromises()
 expect(wrapper.findAll('.unlock-group-options label').some(x=>x.text().includes('AI'))).toBe(true)
 expect(wrapper.findAll('.unlock-group-options label').some(x=>x.text().includes('流媒体'))).toBe(true)
 vi.unstubAllGlobals()
})

it('hides optional legacy reference services (ChatGPT/Claude/Spotify/YouTube) by default in full catalog until toggled',async()=>{
 window.localStorage.clear();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
 w=createWorkspace();w.project.value='service'
 w.catalog.value=['chatgpt_unlock','claude_unlock','gemini_unlock','antigravity','netflix_unlock','youtube_premium','spotify_unlock','tiktok_unlock'].map(id=>({service_id:id,name:id,category:'fixture',description:'',result_kind:'unlock',success_criterion:'',timeout_seconds:10}))
 wrapper=mount(HomeView,{global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 await wrapper.get('.hd-services summary').trigger('click')
 expect(wrapper.findAll('.unlock-custom .service-picker-grid strong').map(x=>x.text())).toEqual(['gemini_unlock','antigravity','netflix_unlock','tiktok_unlock'])
 await wrapper.findAll('button').find(b=>b.text().includes('显示传统参考项 (4)'))!.trigger('click');await flushPromises()
 expect(wrapper.findAll('.unlock-custom .service-picker-grid strong')).toHaveLength(8)
 vi.unstubAllGlobals()
})
it('filters by quick region chips (including multi-select) and selects or clears matching nodes',async()=>{
 window.localStorage.clear();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
 const hk:NodeOption={profile_id:'p',node_key:'n-hk',node_identity_key:'i-hk',config_revision_key:'r',profile_name:'fixture',display_name:'HK 01 优化',type:'anytls',country_code:'HK',country_flag:''}
 const jp:NodeOption={profile_id:'p',node_key:'n-jp',node_identity_key:'i-jp',config_revision_key:'r',profile_name:'fixture',display_name:'JP 02 HY2',type:'vless',country_code:'JP',country_flag:''}
 const us:NodeOption={profile_id:'p',node_key:'n-us',node_identity_key:'i-us',config_revision_key:'r',profile_name:'fixture',display_name:'US 03 AI',type:'vless',country_code:'US',country_flag:''}
 w=createWorkspace();w.project.value='latency';w.nodes.value=[hk,jp,us];w.airports.value=[{id:'a',name:'fixture',has_cache:true,url_display:'',node_count:3,subscriptions:[{id:'p',airport_id:'a',name:'fixture',url_display:'',url_configured:false,node_count:3,has_cache:true}]}];w.setAirports(['a'])
 wrapper=mount(HomeView,{global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 expect(wrapper.find('select[aria-label="地区"]').exists()).toBe(false)
 expect(wrapper.get('.hd-region-strip').text()).toContain('全部3')
 expect(wrapper.get('.hd-source-label').text()).toBe('fixture')
 expect(wrapper.get('.hd-node-region').text()).toBe('香港')
 expect(wrapper.findAll('.hd-node-part.num')[0].text()).toBe('01')
 expect(wrapper.findAll('.hd-node-part.line')[0].text()).toBe('优化')
 await wrapper.findAll('.hd-node-part.line')[0].trigger('mouseenter');await flushPromises()
 expect(wrapper.get('.hd-attr-popover').text()).toContain('路由优化')
 await wrapper.findAll('.hd-node-part.line')[0].trigger('mouseleave');await flushPromises()
 expect(wrapper.find('.hd-attr-popover').exists()).toBe(false)
 await wrapper.findAll('.hd-region-chip').find(b=>b.text().includes('日本'))!.trigger('click');await flushPromises()
 expect(w.region.value).toBe('JP')
 expect(wrapper.findAll('.hd-node')).toHaveLength(1)
 await wrapper.findAll('.hd-region-chip').find(b=>b.text().includes('香港'))!.trigger('click');await flushPromises()
 expect(w.region.value).toBe('JP,HK')
 expect(wrapper.findAll('.hd-node')).toHaveLength(2)
 await wrapper.findAll('.hd-region-chip').find(b=>b.text().includes('日本'))!.trigger('click');await flushPromises()
 expect(w.region.value).toBe('HK')
 expect(wrapper.findAll('.hd-node')).toHaveLength(1)
 await wrapper.get('.hd-region-chip.exclude-toggle').trigger('click');await flushPromises()
 expect(w.excludeRegion.value).toBe(true)
 expect(wrapper.findAll('.hd-node')).toHaveLength(2)
 expect(wrapper.get('.hd-region-strip').text()).toContain('排除 香港')
 await wrapper.findAll('.hd-region-chip').find(b=>b.text().includes('日本'))!.trigger('click');await flushPromises()
 expect(w.excludeRegion.value).toBe(true)
 expect(w.region.value).toBe('HK,JP')
 expect(wrapper.findAll('.hd-node')).toHaveLength(1)
 await wrapper.get('.hd-region-chip.all').trigger('click');await flushPromises()
 expect(w.excludeRegion.value).toBe(false)
 expect(w.region.value).toBe('all')
 expect(wrapper.findAll('.hd-node')).toHaveLength(3)
 await wrapper.get('.hd-region-chip.exclude-toggle').trigger('click');await flushPromises()
 await wrapper.findAll('.hd-region-chip').find(b=>b.text().includes('香港'))!.trigger('click');await flushPromises()
 expect(w.excludeRegion.value).toBe(true)
 expect(w.region.value).toBe('HK')
 expect(wrapper.findAll('.hd-node')).toHaveLength(2)
 await wrapper.get('.hd-quick-select').trigger('click');await flushPromises()
 expect(w.selectedKeys.value).toHaveLength(2)
 await wrapper.get('.hd-quick-select.clear').trigger('click');await flushPromises()
 expect(w.selectedKeys.value).toHaveLength(0)
 vi.unstubAllGlobals()
})
