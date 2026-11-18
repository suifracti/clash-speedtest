import {afterEach,beforeEach,expect,it,onTestFinished,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import App from '../App.vue'
import {api,events} from '../api'
import {createWorkspace,type Workspace} from '../workspace'
import type {LatencyBatch,NodeOption} from '../domain'

vi.mock('../workspace',async original=>{const actual=await original<typeof import('../workspace')>();return {...actual,createWorkspace:vi.fn(actual.createWorkspace)}})
vi.mock('../api',async original=>{const actual=await original<typeof import('../api')>();return {...actual,desktop:()=>false,events:vi.fn(()=>vi.fn()),api:{...actual.api,auth:vi.fn(),login:vi.fn(),setup:vi.fn(),airports:vi.fn(),nodes:vi.fn(),catalog:vi.fn(),jobs:vi.fn(),settings:vi.fn(),token:vi.fn(),batches:vi.fn(),subscriptionUsageState:vi.fn()}}})
const ready={state:'ready',initialized:true,data_root:'/synthetic',profile_dir:'/synthetic/profiles',history_dir:'/synthetic/history',settings_file:'/synthetic/settings.json',migration:{state:'isolated',target_history_dir:'/synthetic/history'}}
let w:Workspace,wrapper:VueWrapper|undefined
const stubs={HomeView:{template:'<section data-testid="home">线路列表</section>'},DataView:{template:'<section data-testid="data">数据与备份</section>'},TestPlanModal:{template:'<section data-testid="plan">检测草稿</section>'},CommandPalette:{template:'<section data-testid="palette">查找</section>'},AirportsView:true,MonitorView:true,RecommendationView:true,HistoryView:true,SettingsView:true,MonitorCreateModal:true,QueueModal:true,AutoPickReportModal:true,NodeDetailModal:true,HoverTip:true}
function attach(){w=createWorkspace();vi.mocked(createWorkspace).mockReturnValueOnce(w);wrapper=mount(App,{attachTo:document.body,global:{stubs}});return wrapper}
beforeEach(()=>{
 localStorage.clear();sessionStorage.clear();vi.clearAllMocks();HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')};HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
 vi.mocked(api.auth).mockResolvedValue({auth_required:false,authenticated:true});vi.mocked(api.setup).mockResolvedValue({...ready});vi.mocked(api.airports).mockResolvedValue([]);vi.mocked(api.nodes).mockResolvedValue([]);vi.mocked(api.catalog).mockResolvedValue([]);vi.mocked(api.jobs).mockResolvedValue([]);vi.mocked(api.settings).mockResolvedValue({} as never);vi.mocked(api.token).mockResolvedValue({has_token:false} as never);vi.mocked(api.batches).mockResolvedValue([]);vi.mocked(api.subscriptionUsageState).mockResolvedValue({state:'idle'} as never)
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w?.dispose();document.body.innerHTML=''})
it('keeps the first workspace loading until the node inventory is known',async()=>{
 let finish!:(nodes:never[])=>void;vi.mocked(api.nodes).mockImplementationOnce(()=>new Promise(resolve=>finish=resolve));const app=attach();await flushPromises()
 expect(app.find('[data-testid=home]').exists()).toBe(false);expect(app.find('[data-testid=data]').exists()).toBe(false);expect(app.text()).toContain('正在读取机场与记录')
 finish([]);await flushPromises();expect(app.find('[data-testid=home]').exists()).toBe(true)
})
it('offers connection retry after initial failure without displaying unknown storage as a data page',async()=>{
 vi.mocked(api.auth).mockRejectedValueOnce(new Error('synthetic connection failure'));const app=attach();await flushPromises()
 expect(app.find('[data-testid=data]').exists()).toBe(false);expect(app.text()).toContain('暂时无法打开工作台');expect(app.text()).toContain('synthetic connection failure')
 await app.findAll('button').find(b=>b.text()==='重试连接')!.trigger('click');await flushPromises();expect(app.find('[data-testid=home]').exists()).toBe(true);expect(api.auth).toHaveBeenCalledTimes(2)
})
it('does not treat an unreadable initial inventory as a genuinely empty workspace',async()=>{
 vi.mocked(api.nodes).mockRejectedValueOnce(new Error('synthetic node inventory unavailable'));const app=attach();await flushPromises()
 expect(app.find('[data-testid=home]').exists()).toBe(false);expect(app.text()).toContain('synthetic node inventory unavailable');expect(app.get('button[aria-label="数据与备份"]').attributes('disabled')).toBeDefined()
 await app.findAll('button').find(b=>b.text()==='重试连接')!.trigger('click');await flushPromises();expect(app.find('[data-testid=home]').exists()).toBe(true)
})
it('hides old operation windows during authentication and keeps the accepted task data',async()=>{
 const app=attach();await flushPromises();w.openPlan([]);w.queue.value=[{id:'accepted',node:{},project:'download',state:'completed',repeat:1} as never];await app.get('button.cst-jump').trigger('click');await flushPromises();expect(app.find('[data-testid=palette]').exists()).toBe(true)
 window.dispatchEvent(new Event('speedtest:auth-required'));await flushPromises()
 expect(app.find('[data-testid=plan]').exists()).toBe(false);expect(app.find('[data-testid=palette]').exists()).toBe(false);expect(app.get('button[aria-label="机场与账户"]').attributes('disabled')).toBeDefined()
 expect(document.querySelector('dialog[open]')?.textContent).toContain('访问此工作台');expect(w.queue.value[0].id).toBe('accepted');expect(api.login).not.toHaveBeenCalled()
})
it('keeps initialization accessible while preventing navigation to unavailable workspace pages',async()=>{
 vi.mocked(api.setup).mockResolvedValueOnce({...ready,state:'needs_choice',initialized:false});const app=attach();await flushPromises()
 expect(app.find('[data-testid=data]').exists()).toBe(true);expect(app.get('button[aria-label="线路工作台"]').attributes('disabled')).toBeDefined();expect(api.airports).not.toHaveBeenCalled();expect(events).toHaveBeenCalledOnce()
})
it('keeps login visible and prevents duplicate submission until the workspace has loaded',async()=>{
 vi.mocked(api.auth).mockResolvedValueOnce({auth_required:true,authenticated:false});vi.mocked(api.login).mockResolvedValue(undefined)
 let finish!:(nodes:never[])=>void;vi.mocked(api.nodes).mockImplementationOnce(()=>new Promise(resolve=>finish=resolve));const app=attach();await flushPromises()
 const input=document.querySelector<HTMLInputElement>('input[type=password]')!;input.value='synthetic-password';input.dispatchEvent(new Event('input'))
 const form=document.querySelector('dialog form')!;form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));await flushPromises();form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));await flushPromises()
 expect(api.login).toHaveBeenCalledTimes(1);expect(api.login).toHaveBeenCalledWith('synthetic-password');expect(document.querySelector('dialog[open]')?.textContent).toContain('正在验证并读取工作台');expect(input.disabled).toBe(true);expect(app.find('[data-testid=home]').exists()).toBe(false)
 finish([]);await flushPromises();expect(document.querySelector('dialog[open]')).toBe(null);expect(app.find('[data-testid=home]').exists()).toBe(true)
})

const taskNode:NodeOption={profile_id:'synthetic-account',node_key:'synthetic-node',node_identity_key:'synthetic-identity',config_revision_key:'synthetic-revision',profile_name:'示例机场',display_name:'示例线路',type:'ss',country_code:'HK',country_flag:''}
const recentBatch:LatencyBatch={batch_id:'synthetic-batch',request_id:'synthetic-request',state:'completed',requested_at:'2026-11-18T01:00:00Z',item_count:1,items:[{...taskNode,item_id:'synthetic-item',batch_id:'synthetic-batch',execution_state:'completed',persistence_state:'saved'}]}

it('opens the current queue and the latency report through distinct actions after a mixed task',async()=>{
 const app=attach();await flushPromises()
 w.batches.value=[recentBatch]
 w.queue.value=[
  {id:'latency',node:taskNode,project:'latency',state:'completed',batchId:recentBatch.batch_id,itemId:'synthetic-item'},
  {id:'download',node:taskNode,project:'download',state:'completed',attempt:{...taskNode,request_id:'download',attempt_id:'download',execution_state:'completed',persistence_state:'failed',requested_at:recentBatch.requested_at,rule:{target_url:'https://example.invalid/'}}}
 ]
 await flushPromises()
 const task=app.get('button.cst-task')
 expect(task.text()).toContain('1 项未保存')
 await task.trigger('click')
 expect(w.queueOpen.value).toBe(true)
 expect(w.reportSelection.value).toBeNull()
 w.queueOpen.value=false
 await app.get('button[aria-label^="查看最近延迟报告"]').trigger('click')
 expect(w.reportSelection.value?.batch.batch_id).toBe(recentBatch.batch_id)
 w.queueOpen.value=false
 await task.trigger('click')
 expect(w.reportSelection.value).toBeNull()
})

it('keeps an unconfirmed start distinct even if an older batch reference remains',async()=>{
 const app=attach();await flushPromises()
 w.activeBatch.value=recentBatch
 w.pendingStart.value={kind:'latency',requestId:'unconfirmed'}
 w.queue.value=[{id:'pending',node:taskNode,project:'latency',state:'running'}]
 await flushPromises()
 expect(app.get('button.cst-task').text()).toContain('正在确认启动状态')
 expect(app.find('.cst-task-bar').exists()).toBe(false)
 w.pendingStart.value=null
 w.activeBatch.value={...recentBatch,state:'saving'}
 await flushPromises()
 expect(app.get('button.cst-task').text()).toContain('正在保存结果')
 expect(app.find('.cst-task-cancel').exists()).toBe(false)
})

it('returns to the workbench reading position from the navigation and from in-page links',async()=>{
 const scrollTo=vi.spyOn(window,'scrollTo').mockImplementation(()=>{})
 vi.stubGlobal('scrollY',480)
 onTestFinished(()=>{scrollTo.mockRestore();vi.unstubAllGlobals()})
 const app=attach();await flushPromises()
 await app.get('button[aria-label="现在用哪个"]').trigger('click');await flushPromises()
 expect(scrollTo).toHaveBeenLastCalledWith(expect.objectContaining({top:0}))
 vi.stubGlobal('scrollY',125)
 await app.get('button[aria-label="线路工作台"]').trigger('click');await flushPromises()
 expect(scrollTo).toHaveBeenLastCalledWith(expect.objectContaining({top:480}))
 vi.stubGlobal('scrollY',320)
 w.page.value='history';await flushPromises()
 expect(scrollTo).toHaveBeenLastCalledWith(expect.objectContaining({top:0}))
 w.page.value='home';await flushPromises()
 expect(scrollTo).toHaveBeenLastCalledWith(expect.objectContaining({top:320}))
 w.page.value='history';await flushPromises();scrollTo.mockClear()
 w.page.value='home';w.viewRequest.value=taskNode;await flushPromises()
 expect(scrollTo).not.toHaveBeenCalled()
})
