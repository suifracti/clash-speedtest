import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import NodeDetailModal from '../components/NodeDetailModal.vue'
import HistoryView from '../components/HistoryView.vue'
import MonitorView from '../components/MonitorView.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {api,request} from '../api'
import type {Attempt,LatencyBatch,NodeOption,MonitorSample,MonitorJob} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,request:vi.fn(),api:{...actual.api,revisions:vi.fn(),attemptHistory:vi.fn(),attemptAction:vi.fn(),batches:vi.fn(),batch:vi.fn(),retryBatch:vi.fn(),latencyHistory:vi.fn(),monitorSamples:vi.fn(),monitorFacets:vi.fn(),jobs:vi.fn(),budget:vi.fn(),storage:vi.fn()}}})
const node:NodeOption={profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A',display_name:'Node A',type:'ss',country_code:'SG',country_flag:''}
const failed:Attempt={...node,attempt_id:'service-a',request_id:'request-a',service_id:'cloudflare_204',requested_at:'2026-10-02T12:00:00Z',execution_state:'completed',persistence_state:'failed',persistence_error:'save failed',rule:{name:'Cloudflare service',target_url:'https://cp.cloudflare.com/generate_204'},result:{outcome:'matched',bytes_read:0,http_status:204,finished_at:'2026-10-02T12:00:01Z'}}
let wrapper:VueWrapper|undefined,w:Workspace
beforeEach(()=>{
  vi.resetAllMocks();window.sessionStorage.clear()
  HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()
  vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
  vi.mocked(api.revisions).mockResolvedValue([])
  vi.mocked(api.attemptHistory).mockImplementation(async kind=>({attempts:kind==='service'?[failed]:[],has_more:false,complete:true}))
  w=createWorkspace();w.project.value='service';w.serviceIds.value=['cloudflare_204']
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML='';vi.unstubAllGlobals()})

it('shows recorded sample totals and Chinese terminal states without inventing a missing per-site count',async()=>{
  const summaries:LatencyBatch[]=[{batch_id:'batch-one-sample',request_id:'one-sample',target_id:'cloudflare',state:'completed_with_issues',requested_at:'2026-10-02T12:39:39Z',item_count:1},{batch_id:'batch-interrupted',request_id:'interrupted',target_id:'cloudflare',state:'interrupted_with_save_failures',requested_at:'2026-10-02T12:00:00Z',item_count:1}]
  vi.mocked(api.batches).mockResolvedValue(summaries)
  vi.mocked(api.batch).mockResolvedValue({...summaries[0],items:[{...node,item_id:'item-one-sample',batch_id:'batch-one-sample',execution_state:'completed',persistence_state:'saved',result:{...node,attempt_id:'attempt-one-sample',node_type:'ss',requested_at:'2026-10-02T12:39:39Z',finished_at:'2026-10-02T12:39:40Z',status:'completed',latency_ms:594,total_samples:1,success_samples:1,failure_samples:0,persistence_state:'saved',samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-02T12:39:40Z',latency_ms:594,success:true}]}}]})
  wrapper=mount(HistoryView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
  await flushPromises()
  const rows=wrapper.findAll('.history-batch-row')
  expect(rows[0].text()).not.toContain('每站')
  expect(rows[0].text()).toContain('完成 · 部分检测异常')
  expect(rows[1].text()).toContain('已中断 · 部分保存失败')
  await rows[0].trigger('click');await flushPromises()
  expect(document.querySelector('dialog')?.textContent).toContain('实际采样 1 次 · 成功 1 次')
  expect(document.querySelector('dialog')?.textContent).not.toContain('6 次')
})

it.each(['saved','rejected'])('ignores a late %s save response after changing the displayed project',async outcome=>{
  let resolveRetry!:(a:Attempt)=>void,rejectRetry!:(e:Error)=>void
  vi.mocked(api.attemptAction).mockImplementationOnce(()=>new Promise((resolve,reject)=>{resolveRetry=resolve;rejectRetry=reject}))
  wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
  await flushPromises()
  ;(Array.from(document.querySelectorAll('dialog button')).find(b=>b.textContent==='详情') as HTMLButtonElement).click();await flushPromises()
  ;(Array.from(document.querySelectorAll('dialog button')).find(b=>b.textContent==='重试保存') as HTMLButtonElement).click();await flushPromises()
  expect(api.attemptAction).toHaveBeenCalledWith('service','service-a',expect.objectContaining({profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a'}),'retry-save','cloudflare_204')
  ;(Array.from(document.querySelectorAll('dialog .project-tabs button')).find(b=>b.textContent==='下载') as HTMLButtonElement).click();await flushPromises()
  if(outcome==='saved')resolveRetry({...failed,persistence_state:'saved',persistence_error:undefined})
  else rejectRetry(new Error('old service save failed'))
  await flushPromises()
  expect(document.querySelector('dialog .project-tabs button[aria-selected="true"]')?.textContent).toBe('下载')
  expect(document.querySelector('.measurement-detail')).toBe(null)
  expect(document.querySelector('[role="alert"]')).toBe(null)
  expect(document.body.textContent).not.toContain('Cloudflare service')
})

function deferred<T>(){let resolve!:(value:T)=>void,reject!:(reason:Error)=>void;const promise=new Promise<T>((yes,no)=>{resolve=yes;reject=no});return {promise,resolve,reject}}
const monitorSample:MonitorSample={...node,sample_id:'sample-http',run_id:'run-http',display_name_snapshot:node.display_name,timestamp:'2026-10-04T00:00:00Z',target:'https://api.github.com',probe_type:'service_github',success:false,error_class:'conn_error',error_detail:'Get https://api.github.com: EOF',latency:0,ttfb:0,sampling_tier:'regular'}
function bodyButton(label:string){return Array.from(document.querySelectorAll('button')).find(b=>b.textContent?.trim()===label) as HTMLButtonElement}

it('opens monitoring-node details on saved monitor data, keeps manual history distinct and discards a late probe response',async()=>{
  w.page.value='monitor';w.project.value='service';w.hours.value=168
  const oldProbe=deferred<{items:MonitorSample[];has_more:boolean;next_cursor:string}>()
  vi.mocked(api.monitorSamples).mockImplementation(q=>q.probe_type==='service_google'?oldProbe.promise:Promise.resolve({items:[monitorSample],has_more:false,next_cursor:''}))
  wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
  expect(document.body.textContent).toContain('读取记录中')
  const select=document.querySelector('select[aria-label="节点监测服务"]') as HTMLSelectElement
  select.value='monitor_github_http';select.dispatchEvent(new Event('change',{bubbles:true}));await flushPromises()
  expect(document.body.textContent).toContain('Get https://api.github.com: EOF')
  expect(document.body.textContent).toContain('2xx／3xx')
  expect(document.body.textContent).not.toContain('当前范围没有记录')
  oldProbe.resolve({items:[{...monitorSample,sample_id:'late-google',probe_type:'service_google',target:'https://www.google.com/generate_204',error_detail:'old google error'}],has_more:false,next_cursor:''});await flushPromises()
  expect(document.body.textContent).not.toContain('old google error')
  expect(document.querySelector('.table-scroll tbody')?.textContent).toContain('https://api.github.com')
  bodyButton('手动检测').click();await flushPromises()
  expect(document.body.textContent).toContain('Cloudflare service')
  expect(document.body.textContent).not.toContain('Get https://api.github.com: EOF')
  expect(document.body.textContent).not.toContain('基础 HTTP：')
})

it('clears old filtered samples and pagination on a failed new filter, resets a node belonging to another subscription and retries',async()=>{
  vi.mocked(api.batches).mockResolvedValue([])
  vi.mocked(api.monitorFacets).mockResolvedValue({profiles:['sub-a','sub-b'],nodes:[{...node}],targets:[monitorSample.target]})
  vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[monitorSample],has_more:true,next_cursor:'old-cursor'})
  wrapper=mount(HistoryView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
  bodyButton('持续监测').click();await flushPromises()
  expect(document.querySelector('tbody')?.textContent).toContain('EOF')
  const selects=wrapper.findAll('.history-filter-row select')
  vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[monitorSample],has_more:true,next_cursor:'old-cursor'})
  await selects[2].setValue(node.node_identity_key);await flushPromises()
  vi.mocked(api.monitorSamples).mockRejectedValueOnce(new Error('new subscription unavailable'))
  await selects[1].setValue('sub-b');await flushPromises()
  expect((selects[2].element as HTMLSelectElement).value).toBe('')
  expect(document.querySelector('tbody')?.textContent).not.toContain('EOF')
  expect(document.body.textContent).not.toContain('读取更早样本')
  expect(document.body.textContent).not.toContain('这个范围没有监测样本')
  expect(document.querySelector('[role="alert"]')?.textContent).toContain('new subscription unavailable')
  vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[],has_more:false,next_cursor:''})
  bodyButton('刷新').click();await flushPromises()
  expect(document.querySelector('[role="alert"]')).toBe(null)
  expect(document.body.textContent).toContain('这个范围没有监测样本')
})

it('ignores an old facets response after leaving monitoring history',async()=>{
  vi.mocked(api.batches).mockResolvedValue([]);const facets=deferred<Awaited<ReturnType<typeof api.monitorFacets>>>()
  vi.mocked(api.monitorFacets).mockReturnValueOnce(facets.promise)
  wrapper=mount(HistoryView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
  bodyButton('持续监测').click();await flushPromises();bodyButton('批量检测').click();await flushPromises()
  facets.resolve({profiles:[],nodes:[],targets:[]});await flushPromises()
  expect(api.monitorSamples).not.toHaveBeenCalled()
  expect(document.querySelector('.project-tabs [aria-selected="true"]')?.textContent).toBe('批量检测')
})

it('does not relabel a late task run as another task and distinguishes a read failure from no runs',async()=>{
  const jobs=[{id:'job-a',name:'Task A',profile_id:'sub-a',state:'stopped',node_keys:[],nodes:[],interval_seconds:300,profile_name:'Test subscription',probe_set:'service',sampling_tier:'regular',timeout_seconds:5,resume_on_launch:false,updated_at:''},{id:'job-b',name:'Task B',profile_id:'sub-b',state:'stopped',node_keys:[],nodes:[],interval_seconds:300,profile_name:'Test subscription',probe_set:'service',sampling_tier:'regular',timeout_seconds:5,resume_on_launch:false,updated_at:''}] as MonitorJob[]
  w.jobs.value=jobs;vi.mocked(api.jobs).mockResolvedValue(jobs);vi.mocked(api.budget).mockResolvedValue({});vi.mocked(api.storage).mockResolvedValue({})
  const oldRead=deferred<Record<string,any>[]>(),newRead=deferred<Record<string,any>[]>()
  vi.mocked(request).mockImplementation(path=>path.includes('job-a')?oldRead.promise:newRead.promise)
  wrapper=mount(MonitorView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
  await wrapper.findAll('.monitor-card').at(0)!.findAll('button').find(b=>b.text()==='运行记录')!.trigger('click');await flushPromises()
  expect(document.body.textContent).toContain('读取运行记录中')
  expect(document.body.textContent).not.toContain('还没有运行记录')
  ;(document.querySelector('dialog button[aria-label="关闭"]') as HTMLButtonElement).click();await flushPromises()
  await wrapper.findAll('.monitor-card').at(1)!.findAll('button').find(b=>b.text()==='运行记录')!.trigger('click');await flushPromises()
  newRead.resolve([{run_id:'run-b',started_at:'2026-10-04T00:00:00Z',status:'completed',success_nodes:2,total_nodes:2,failed_nodes:0}]);await flushPromises()
  oldRead.resolve([{run_id:'run-a',started_at:'2026-10-03T00:00:00Z',status:'completed',success_nodes:99,total_nodes:99,failed_nodes:0}]);await flushPromises()
  expect(document.querySelector('dialog h2')?.textContent).toBe('Task B · 运行记录')
  expect(document.querySelector('dialog tbody')?.textContent).toContain('2 / 2')
  expect(document.querySelector('dialog tbody')?.textContent).not.toContain('99')
  ;(document.querySelector('dialog button[aria-label="关闭"]') as HTMLButtonElement).click();await flushPromises()
  vi.mocked(request).mockRejectedValueOnce(new Error('runs unavailable'))
  await wrapper.findAll('.monitor-card').at(1)!.findAll('button').find(b=>b.text()==='运行记录')!.trigger('click');await flushPromises()
  expect(document.querySelector('dialog [role="alert"]')?.textContent).toContain('runs unavailable')
  expect(document.querySelector('dialog')?.textContent).not.toContain('还没有运行记录')
  vi.mocked(request).mockResolvedValueOnce([]);bodyButton('重新读取').click();await flushPromises()
  expect(document.querySelector('dialog [role="alert"]')).toBe(null)
  expect(document.querySelector('dialog')?.textContent).toContain('还没有运行记录')
})

it('does not attach another node configuration list when revision reads arrive out of order',async()=>{
  const old=deferred<Awaited<ReturnType<typeof api.revisions>>>(),other={...node,node_key:'node-b',node_identity_key:'identity-b',config_revision_key:'revision-b',display_name:'Node B'}
  vi.mocked(api.revisions).mockImplementation(n=>n.node_identity_key===node.node_identity_key?old.promise:Promise.resolve([{config_revision_key:'revision-b',node_key:'node-b',display_name:'Node B',last_observed_at:''},{config_revision_key:'revision-b-old',node_key:'node-b-old',display_name:'Node B old',last_observed_at:''}]))
  wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
  await wrapper.setProps({node:other});await flushPromises()
  old.resolve([{config_revision_key:'revision-a-old',node_key:'node-a-old',display_name:'Node A old',last_observed_at:''}]);await flushPromises()
  expect(document.querySelector('dialog h2')?.textContent).toContain('Node B')
  const options=Array.from(document.querySelectorAll('dialog select option')).map(o=>(o as HTMLOptionElement).value)
  expect(options).toContain('revision-b-old');expect(options).not.toContain('revision-a-old')
})

it('keeps the last selected batch and does not reopen a closed record after save completes',async()=>{
  const first:LatencyBatch={batch_id:'batch-a',request_id:'a',target_id:'cloudflare',state:'completed',requested_at:'2026-10-04T00:00:00Z',item_count:1}
  const second:LatencyBatch={...first,batch_id:'batch-b',request_id:'b',items:[{...node,item_id:'item-b',batch_id:'batch-b',execution_state:'completed',persistence_state:'failed',persistence_error:'save unavailable'}]}
  const old=deferred<LatencyBatch>(),saved=deferred<LatencyBatch>()
  vi.mocked(api.batches).mockResolvedValue([first,second]);vi.mocked(api.batch).mockImplementation(id=>id==='batch-a'?old.promise:Promise.resolve(second));vi.mocked(api.retryBatch).mockReturnValueOnce(saved.promise)
  wrapper=mount(HistoryView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
  const rows=wrapper.findAll('.history-batch-row');await rows[0].trigger('click');await rows[1].trigger('click');await flushPromises()
  old.resolve({...first,items:[{...node,item_id:'item-a',batch_id:'batch-a',display_name:'Wrong old node',execution_state:'completed',persistence_state:'saved'}]});await flushPromises()
  expect(document.querySelector('dialog')?.textContent).toContain('save unavailable')
  expect(document.querySelector('dialog')?.textContent).not.toContain('Wrong old node')
  bodyButton('重试保存').click();await flushPromises()
  ;(document.querySelector('dialog button[aria-label="关闭"]') as HTMLButtonElement).click();await flushPromises()
  saved.resolve({...second,state:'completed'});await flushPromises()
  expect(document.querySelector('dialog')).toBe(null)
})
