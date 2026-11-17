import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import App from '../App.vue'
import {api,events} from '../api'
import {createWorkspace,type Workspace} from '../workspace'

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
