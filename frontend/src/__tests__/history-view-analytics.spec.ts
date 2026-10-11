import {afterEach,beforeEach,it,expect,vi} from 'vitest'
import {mount,flushPromises,type VueWrapper} from '@vue/test-utils'
import HistoryView from '../components/HistoryView.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {api} from '../api'
import type {MonitorSample} from '../domain'
vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async original=>{const a=await original<typeof import('../api')>();return {...a,api:{...a.api,batches:vi.fn(),monitorFacets:vi.fn(),monitorSamples:vi.fn()}}})
let w:Workspace,wrapper:VueWrapper
const sample=(id:string,hours=1):MonitorSample=>({sample_id:id,profile_id:'deleted-profile',node_key:'old-key',node_identity_key:'old-identity',config_revision_key:'old-revision',display_name_snapshot:'Deleted Node',target:'site',probe_type:'rtt',success:true,latency:20e6,ttfb:0,sampling_tier:'regular',run_id:'one-round',timestamp:new Date(Date.now()-hours*3600000).toISOString()})
const facets={nodes:[{profile_id:'deleted-profile',node_identity_key:'old-identity',node_key:'old-key',display_name:'Deleted Node'}],profiles:['deleted-profile'],targets:['site'],truncated:true}
function deferred<T>(){let resolve!:(v:T)=>void;const promise=new Promise<T>(r=>resolve=r);return {promise,resolve}}
async function monitor(){wrapper=mount(HistoryView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises();await wrapper.findAll('.project-tabs button')[2]!.trigger('click');await flushPromises()}
function button(label:string){return wrapper.findAll('button').find(b=>b.text()===label)!}
beforeEach(()=>{w=createWorkspace();vi.mocked(api.batches).mockResolvedValue([]);vi.mocked(api.monitorFacets).mockResolvedValue(facets);vi.mocked(api.monitorSamples).mockResolvedValue({items:[sample('new'),sample('older',3)],has_more:true,next_cursor:'cursor-1'});HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()})
afterEach(()=>{wrapper?.unmount();w.dispose();document.body.innerHTML='';vi.resetAllMocks()})
it('shows loaded statistics, true time spacing, focus/hover freeze and raw JSON without inventing a current node',async()=>{
 await monitor()
 expect(wrapper.find('.history-analytics').text()).toContain('成功 2 / 2')
 expect(wrapper.text()).toContain('仍有缺页')
 expect(wrapper.text()).toContain('筛选项已截断')
 expect(wrapper.text()).toContain('不提供同条件排名')
 const points=wrapper.findAll('.history-point');expect(points).toHaveLength(2)
 expect(Number(points[1]!.attributes('cx'))-Number(points[0]!.attributes('cx'))).toBeGreaterThan(100)
 await points[0]!.trigger('focus');expect(wrapper.find('.history-tooltip').text()).toContain('older')
 await points[0]!.trigger('click');await points[1]!.trigger('mouseenter');expect(wrapper.find('.history-tooltip').text()).toContain('older')
 await button('原始 JSON / 轮次').trigger('click');await flushPromises();expect(document.querySelector('dialog pre')?.textContent).toContain('one-round')
 expect(w.inspectNode.value).toBeNull()
})
it('retains loaded data after an append error, retries same cursor and reports completeness only on success',async()=>{
 await monitor();vi.mocked(api.monitorSamples).mockRejectedValueOnce(new Error('page offline'))
 await button('读取更早样本').trigger('click');await flushPromises()
 expect(wrapper.find('.history-analytics').text()).toContain('成功 2 / 2');expect(wrapper.text()).toContain('page offline')
 vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[sample('earliest',4)],has_more:false,next_cursor:''})
 await button('读取更早样本').trigger('click');await flushPromises()
 expect(api.monitorSamples).toHaveBeenLastCalledWith(expect.objectContaining({cursor:'cursor-1'}))
 expect(wrapper.find('.history-analytics').text()).toContain('成功 3 / 3');expect(wrapper.text()).toContain('窗口分页已读完')
})
it('discards late samples and facets after changing scope or tab',async()=>{
 const late=deferred<Awaited<ReturnType<typeof api.monitorSamples>>>();vi.mocked(api.monitorSamples).mockReturnValueOnce(late.promise)
 await monitor();w.hours.value=168;await flushPromises()
 late.resolve({items:[sample('stale')],has_more:false,next_cursor:''});await flushPromises()
 expect(wrapper.find('.history-analytics').text()).not.toContain('stale')
 const lateFacets=deferred<typeof facets>();vi.mocked(api.monitorFacets).mockReturnValueOnce(lateFacets.promise)
 await button('刷新').trigger('click');await wrapper.findAll('.project-tabs button')[0]!.trigger('click')
 lateFacets.resolve({...facets,targets:['stale-target']});await flushPromises()
 expect(wrapper.find('.history-analytics').exists()).toBe(false)
})
it('uses historical profile plus identity when selecting a removed node',async()=>{
 await monitor();await wrapper.find('select[aria-label="监测历史节点"]').setValue('old-identity');await flushPromises()
 expect(api.monitorSamples).toHaveBeenLastCalledWith(expect.objectContaining({profile_id:'deleted-profile',node_identity_key:'old-identity'}))
 expect(wrapper.find('.history-analytics').text()).toContain('Deleted Node')
})
it('reports a stalled cursor without falsely declaring complete history',async()=>{
 await monitor();vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[],has_more:true,next_cursor:'cursor-1'})
 await button('读取更早样本').trigger('click');await flushPromises()
 expect(wrapper.text()).toContain('游标');expect(wrapper.text()).not.toContain('窗口分页已读完')
})
it('refreshes facets for a new time window and ignores older window facets without accepting older results',async()=>{
 const old=deferred<typeof facets>();vi.mocked(api.monitorFacets).mockReturnValueOnce(old.promise)
 await monitor();w.hours.value=168;await flushPromises()
 old.resolve({...facets,targets:['old-window-target']});await flushPromises()
 expect(wrapper.text()).not.toContain('old-window-target')
 expect(api.monitorFacets).toHaveBeenCalledTimes(2)
 expect(wrapper.find('.history-analytics').text()).toContain('成功 2 / 2')
})
it('keeps no records and cancelled-only samples untested',async()=>{
 vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[],has_more:false,next_cursor:''});await monitor()
 expect(wrapper.find('.history-analytics').text()).toContain('未测')
 vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[{...sample('cancel'),success:false,error_class:'cancelled'}],has_more:false,next_cursor:''})
 await button('刷新').trigger('click');await flushPromises()
 expect(wrapper.find('.history-analytics').text()).toContain('成功 0 / 0');expect(wrapper.find('.history-analytics').text()).toContain('排除 1')
})
it('isolates duplicate identities in different historical profiles',async()=>{
 vi.mocked(api.monitorFacets).mockResolvedValueOnce({...facets,nodes:[...facets.nodes,{...facets.nodes[0]!,profile_id:'second-profile'}],profiles:['deleted-profile','second-profile']})
 await monitor();await wrapper.find('select[aria-label="监测历史节点"]').setValue(JSON.stringify(['second-profile','old-identity','old-key']));await flushPromises()
 expect(api.monitorSamples).toHaveBeenLastCalledWith(expect.objectContaining({profile_id:'second-profile',node_identity_key:'old-identity'}))
 expect(wrapper.find('.history-analytics').text()).not.toContain('成功 2 / 2')
})
it('lets the user cancel a pending read and ignores its later success',async()=>{
 const pending=deferred<Awaited<ReturnType<typeof api.monitorSamples>>>();vi.mocked(api.monitorSamples).mockReturnValueOnce(pending.promise)
 await monitor();await button('取消读取').trigger('click');await flushPromises()
 pending.resolve({items:[{...sample('late'),display_name_snapshot:'late cancelled read'}],has_more:false,next_cursor:''});await flushPromises()
 expect(wrapper.text()).not.toContain('late cancelled read');expect(wrapper.text()).toContain('读取已取消')
 expect(wrapper.text()).not.toContain('窗口分页已读完')
})
