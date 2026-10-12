import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {mount,flushPromises,type VueWrapper} from '@vue/test-utils'
import SettingsView from '../components/SettingsView.vue'
import DataView from '../components/DataView.vue'
import {api,backup,selectProfileSourceDirectory,request} from '../api'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {applyTheme} from '../theme'
import type {Settings} from '../domain'
vi.hoisted(()=>Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}}))
vi.mock('../api',async original=>{const actual=await original<typeof import('../api')>();return {...actual,desktop:()=>true,backup:vi.fn(),selectProfileSourceDirectory:vi.fn(),request:vi.fn(),api:{...actual.api,settings:vi.fn(),saveSettings:vi.fn(),setToken:vi.fn(),token:vi.fn(),oauth:vi.fn(),inspect:vi.fn(),importSource:vi.fn(),migrate:vi.fn(),initialize:vi.fn()}}})
vi.mock('../theme',()=>({applyTheme:vi.fn().mockResolvedValue(undefined)}))
const settings={preferred_browser:'auto',monitor_retention_policy:'30d',monitor_retention_custom_days:30,monitor_storage_warning_bytes:1073741824,monitor_storage_hard_bytes:2147483648,monitor_budget_max_concurrent:4,monitor_budget_response_bytes:65536} as Settings
let w:Workspace,wrapper:VueWrapper|undefined
function attach(component:any){wrapper=mount(component,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});return wrapper}
function button(label:string){const el=Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(b=>b.textContent?.trim()===label);expect(el,`button ${label}`).toBeDefined();return el!}
async function click(label:string){button(label).click();await flushPromises()}
beforeEach(()=>{vi.resetAllMocks();HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn();w=createWorkspace();vi.spyOn(w,'boot').mockResolvedValue(undefined);vi.mocked(api.settings).mockResolvedValue({...settings});w.setup.value={state:'ready',initialized:true,data_root:'/synthetic/data',profile_dir:'/synthetic/data/profiles',history_dir:'/synthetic/data/history',settings_file:'/synthetic/data/settings.json',sources:[],migration:{state:'pending',target_history_dir:'/synthetic/data/history',source_history_dir:'/synthetic/legacy/history'}}})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML=''})
it('invalidates edited source and imports only after confirmation',async()=>{
  const view=attach(DataView);vi.mocked(api.inspect).mockResolvedValue({path:'/synthetic/a',label:'Source A',available:true,profile_count:2,cache_count:1});await view.get('input').setValue('/synthetic/a');await click('检查来源');await view.get('input').setValue('/synthetic/b');expect(document.body.textContent).not.toContain('Source A');expect(document.body.textContent).not.toContain('导入这个来源');vi.mocked(api.inspect).mockResolvedValue({path:'/synthetic/b',available:true,profile_count:3,cache_count:0});await click('检查来源');await click('导入这个来源');expect(api.importSource).not.toHaveBeenCalled();await click('取消');expect(api.importSource).not.toHaveBeenCalled();expect(document.querySelector('[role=status]')?.textContent).toContain('取消');await click('导入这个来源');await click('确认导入');expect(api.importSource).toHaveBeenCalledWith('/synthetic/b');expect(w.boot).toHaveBeenCalledOnce()
})
it('preserves source on picker cancellation and shows unknown counts',async()=>{
  const view=attach(DataView);await view.get('input').setValue('/synthetic/source');vi.mocked(api.inspect).mockResolvedValue({path:'/synthetic/source',available:true,profile_count:null,cache_count:0});await click('检查来源');vi.mocked(selectProfileSourceDirectory).mockResolvedValue('');await click('浏览目录');expect((view.get('input').element as HTMLInputElement).value).toBe('/synthetic/source');expect(document.body.textContent).toContain('未知 个机场');expect(document.querySelector('[role=status]')?.textContent).toContain('取消')
})
it('reports backup cancellation, failure and success and blocks duplicate export',async()=>{
  attach(DataView);let finish!:(v:boolean)=>void;vi.mocked(backup).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}));await click('导出备份');expect(button('导出备份').disabled).toBe(true);button('导出备份').click();expect(backup).toHaveBeenCalledOnce();finish(false);await flushPromises();expect(document.querySelector('[role=status]')?.textContent).toContain('取消');vi.mocked(backup).mockRejectedValueOnce(new Error('export denied'));await click('导出备份');expect(document.querySelector('[role=alert]')?.textContent).toContain('export denied');vi.mocked(backup).mockResolvedValueOnce(true);await click('导出备份');expect(document.querySelector('[role=alert]')).toBeNull();expect(document.querySelector('[role=status]')?.textContent).toContain('已导出')
})
it('confirms migration, protects busy dialog and retries inline failure',async()=>{
  attach(DataView);await click('迁移旧版历史与设置');expect(api.migrate).not.toHaveBeenCalled();await click('取消');await click('迁移旧版历史与设置');let reject!:(e:Error)=>void;vi.mocked(api.migrate).mockImplementationOnce(()=>new Promise((_,r)=>{reject=r}));await click('确认迁移');expect(button('取消').disabled).toBe(true);expect(document.querySelector('dialog button[aria-label="关闭"]')).toBeNull();reject(new Error('migration denied'));await flushPromises();expect(document.querySelector('dialog [role=alert]')?.textContent).toContain('migration denied');vi.mocked(api.migrate).mockResolvedValueOnce(undefined);await click('确认迁移');expect(document.querySelector('dialog')).toBeNull();expect(w.boot).toHaveBeenCalledOnce()
})
it('distinguishes unknown/loading/failed setup without undefined migration',async()=>{
  w.setup.value=null;attach(DataView);expect(document.body.textContent).not.toContain('待初始化');expect(document.body.textContent).not.toContain('undefined');expect(document.body.textContent).not.toContain('创建空白存储');w.loading.value=true;await flushPromises();expect(document.querySelector('[role=status]')?.textContent).toContain('加载');w.loading.value=false;w.error.value='setup unavailable';await flushPromises();expect(document.querySelector('[role=alert]')?.textContent).toContain('setup unavailable')
})
it('removes stale inspection when recheck fails',async()=>{
  const view=attach(DataView);await view.get('input').setValue('/synthetic/source');vi.mocked(api.inspect).mockResolvedValueOnce({path:'/synthetic/source',available:true});await click('检查来源');vi.mocked(api.inspect).mockRejectedValueOnce(new Error('source unreadable'));await click('检查来源');expect(document.body.textContent).not.toContain('导入这个来源');expect(document.querySelector('[role=alert]')?.textContent).toContain('source unreadable')
})
it('freezes settings during saving and keeps edits for retry after failure',async()=>{
  const view=attach(SettingsView);await flushPromises();await view.get('input').setValue(8);let reject!:(e:Error)=>void;vi.mocked(api.saveSettings).mockImplementationOnce(()=>new Promise((_,r)=>{reject=r}));await click('保存设置');expect((view.get('input').element as HTMLInputElement).matches(':disabled')).toBe(true);reject(new Error('disk unavailable'));await flushPromises();expect(document.querySelector('[role=alert]')?.textContent).toContain('disk unavailable');expect((view.get('input').element as HTMLInputElement).value).toBe('8');vi.mocked(api.saveSettings).mockResolvedValueOnce(undefined);await click('保存设置');expect(w.settings.value?.monitor_budget_max_concurrent).toBe(8);expect(document.querySelector('[role=status]')?.textContent).toContain('设置已保存')
})
it('retries settings load without reporting unknown token as unbound',async()=>{
  vi.mocked(api.settings).mockRejectedValueOnce(new Error('settings unavailable'));attach(SettingsView);await flushPromises();expect(document.body.textContent).not.toContain('尚未绑定');await click('重试加载');expect(document.querySelector('[role=alert]')).toBeNull();expect(document.querySelector('input')).not.toBeNull()
})
it('validates numeric budgets before saving',async()=>{
  const view=attach(SettingsView);await flushPromises();await view.get('input').setValue(0);await click('保存设置');expect(api.saveSettings).not.toHaveBeenCalled();expect(document.querySelector('[role=alert]')).not.toBeNull()
})
it('retention failure is retryable inside its dialog and uses the previewed policy',async()=>{
  attach(SettingsView);await flushPromises();vi.mocked(request).mockResolvedValueOnce({policy:'30d',cutoff:'2026-09-01',samples_to_delete:12,runs_to_delete:2});await click('预览旧采样清理');vi.mocked(request).mockRejectedValueOnce(new Error('cleanup denied'));await click('执行清理');expect(document.querySelector('dialog [role=alert]')?.textContent).toContain('cleanup denied');vi.mocked(request).mockResolvedValueOnce({samples_deleted:12});await click('执行清理');expect(request).toHaveBeenLastCalledWith('/api/monitor/retention','POST',{policy:'30d',custom_days:30,cutoff_time:'2026-09-01'},'ApplyRetention',[{policy:'30d',custom_days:30,cutoff_time:'2026-09-01'}]);expect(document.querySelector('dialog')).toBeNull();expect(document.querySelector('[role=status]')?.textContent).toContain('12')
})
it('preserves folder, initialization and candidate-source actions',async()=>{
  w.setup.value!.state='needs_choice';w.setup.value!.sources=[{path:'/synthetic/candidate',label:'Candidate',available:true,profile_count:2,cache_count:1}];attach(DataView);await click('打开数据目录');expect(request).toHaveBeenCalledWith('/api/data/open-folder','POST',undefined,'OpenDataFolder');expect(document.querySelector('.data-view [role=status]')?.textContent).toContain('打开');await click('创建空白存储');expect(api.initialize).toHaveBeenCalledOnce();expect(w.boot).toHaveBeenCalledOnce();vi.mocked(api.inspect).mockResolvedValue({path:'/synthetic/candidate',available:true});await click('检查');expect(api.inspect).toHaveBeenCalledWith('/synthetic/candidate');expect((document.querySelector('input') as HTMLInputElement).value).toBe('/synthetic/candidate')
})
it('retains login, token refresh and manual token save with busy/error/cancel feedback',async()=>{
  attach(SettingsView);await flushPromises();await click('浏览器登录');expect(api.oauth).toHaveBeenCalledOnce();expect(document.querySelector('.preferences-view [role=status]')?.textContent).toContain('登录');vi.mocked(api.token).mockResolvedValue({has_token:false,source:'none',preview:''});await click('刷新状态');expect(document.body.textContent).toContain('尚未绑定');await click('手动输入 Token');const input=document.querySelector('textarea')!;input.value='synthetic-test-token';input.dispatchEvent(new Event('input',{bubbles:true}));await flushPromises();vi.mocked(api.setToken).mockRejectedValueOnce(new Error('token denied'));await click('保存凭据');expect(document.querySelector('dialog [role=alert]')?.textContent).toContain('token denied');await click('取消');expect(document.querySelector('dialog')).toBeNull();await click('手动输入 Token');expect(document.querySelector('textarea')?.value).toBe('');input.value='';await click('取消');await click('手动输入 Token');const next=document.querySelector('textarea')!;next.value='synthetic-test-token';next.dispatchEvent(new Event('input',{bubbles:true}));await flushPromises();vi.mocked(api.token).mockResolvedValue({has_token:true,source:'manual',preview:'***'});await click('保存凭据');expect(api.setToken).toHaveBeenLastCalledWith('synthetic-test-token');expect(document.querySelector('dialog')).toBeNull();expect(document.body.textContent).toContain('已绑定')
})
it('preserves retention custom-day and keep-all dependent controls',async()=>{
  const view=attach(SettingsView);await flushPromises();const select=view.findAll('select')[1];await select.setValue('custom');expect(view.findAll('input')).toHaveLength(5);await view.findAll('input')[4].setValue(0);await click('预览旧采样清理');expect(request).not.toHaveBeenCalled();await select.setValue('keep_all');expect(view.findAll('input')).toHaveLength(4);expect(document.body.textContent).not.toContain('预览旧采样清理')
})
it('invalidates a committed import even if the subsequent workspace refresh has an error',async()=>{
  const view=attach(DataView);await view.get('input').setValue('/synthetic/source');vi.mocked(api.inspect).mockResolvedValue({path:'/synthetic/source',available:true});await click('检查来源');await click('导入这个来源');vi.mocked(w.boot).mockImplementationOnce(async()=>{w.error.value='catalog unavailable'});await click('确认导入');expect(document.body.textContent).not.toContain('导入这个来源');expect(document.querySelector('.data-view [role=status]')?.textContent).toContain('已导入');expect(document.querySelector('[role=alert]')?.textContent).toContain('catalog unavailable')
})

it('keeps theme selection aligned with the document after a native theme failure',async()=>{
  document.documentElement.dataset.theme='light';attach(SettingsView);await flushPromises();vi.mocked(applyTheme).mockImplementationOnce(async value=>{document.documentElement.dataset.theme=value;throw new Error('native theme denied')});await click('深色');expect(button('深色').getAttribute('aria-pressed')).toBe('true');expect(document.querySelector('[role=alert]')?.textContent).toContain('native theme denied')
})
it.each(['import','migration'])('restores focus to a stable control when %s removes its trigger',async kind=>{
  const view=attach(DataView)
  if(kind==='import'){await view.get('input').setValue('/synthetic/source');vi.mocked(api.inspect).mockResolvedValue({path:'/synthetic/source',available:true});await click('检查来源')}
  const opener=kind==='import'?'导入这个来源':'迁移旧版历史与设置';button(opener).focus();await click(opener)
  vi.mocked(w.boot).mockImplementationOnce(async()=>{w.setup.value!.migration.state='ready'})
  await click(kind==='import'?'确认导入':'确认迁移');expect(button('刷新状态')).toBe(document.activeElement)
})
