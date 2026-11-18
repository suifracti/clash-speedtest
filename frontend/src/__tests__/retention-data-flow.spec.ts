import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import SettingsView from '../components/SettingsView.vue'
import DataView from '../components/DataView.vue'
import {api,backup,request} from '../api'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import type {Settings} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{},removeItem:()=>{}}})})
vi.mock('../api',async original=>({...await original<typeof import('../api')>(),backup:vi.fn(),request:vi.fn(),api:{settings:vi.fn(),saveSettings:vi.fn(),storage:vi.fn(),setup:vi.fn()}}))
const settings:Settings={preferred_browser:'default',monitor_retention_policy:'30d',monitor_retention_custom_days:0,monitor_storage_warning_bytes:2**30,monitor_storage_hard_bytes:2**31,monitor_budget_max_concurrent:4,monitor_budget_response_bytes:65536}
const usage={total_bytes:3*2**20,database_bytes:2*2**20,wal_bytes:2**20,shared_memory_bytes:0,warning_bytes:2**30,hard_bytes:2**31,warning:false,protected:false}
const cutoff='2026-10-01T02:00:00Z'
const preview={policy:'30d',cutoff,samples_to_delete:1200,runs_to_delete:2,storage:usage}
const button=(label:string)=>[...document.querySelectorAll<HTMLButtonElement>('button')].find(el=>el.textContent?.trim()===label)!
async function click(label:string){button(label).click();await flushPromises()}
let workspace:Workspace,wrapper:VueWrapper
async function start(component:any=SettingsView){workspace=createWorkspace();workspace.setup.value={state:'ready',initialized:true,data_root:'/synthetic/data',profile_dir:'/synthetic/profiles',history_dir:'/synthetic/history',settings_file:'/synthetic/settings.json',migration:{state:'ready',target_history_dir:'/synthetic/history'}};wrapper=mount(component,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:workspace},stubs:{Icon:true}}});await flushPromises();if(component===SettingsView)await click('历史容量与保留')}
beforeEach(()=>{vi.resetAllMocks();vi.mocked(api.settings).mockResolvedValue({...settings});vi.mocked(api.storage).mockResolvedValue({...usage});HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()})
afterEach(()=>{wrapper?.unmount();workspace?.dispose();document.body.innerHTML=''})

describe('history capacity and retention',()=>{
  it('restores actual file occupancy and labels failed later reads as stale',async()=>{
    await start();expect(api.storage).toHaveBeenCalledOnce()
    expect(document.querySelector('.storage-ledger')?.textContent).toContain('3.0 MiB')
    expect(document.querySelector('.storage-ledger')?.textContent).toContain('1.0 MiB')
    expect(document.querySelector('.storage-ledger')?.textContent).toContain('0 B')
    vi.mocked(api.storage).mockRejectedValueOnce(new Error('读取容量失败'));await click('刷新容量')
    expect(document.querySelector('.storage-ledger')?.textContent).toContain('3.0 MiB')
    expect(document.body.textContent).toContain('上次成功读取')
    expect(document.body.textContent).toContain('不是磁盘剩余空间')
  })

  it('does not render absent storage numbers or a failed first read as zero or unprotected',async()=>{
    vi.mocked(api.storage).mockRejectedValueOnce(new Error('磁盘状态读取失败'));await start()
    expect(document.querySelector('.storage-ledger')?.textContent).toContain('尚无可用容量读数')
    expect(document.querySelector('.storage-ledger')?.textContent).not.toContain('0 B')
    vi.mocked(api.storage).mockResolvedValueOnce({total_bytes:0} as any);await click('刷新容量')
    expect(document.querySelector('.storage-ledger')?.textContent).toContain('0 B')
    expect(document.querySelector('.storage-ledger')?.textContent).toContain('保护状态未提供')
  })

  it('preserves partial deletion counts and requires a refreshed preview of the same cutoff before retrying',async()=>{
    await start();vi.mocked(request).mockResolvedValueOnce(preview);await click('预览旧采样清理')
    vi.mocked(request).mockRejectedValueOnce(Object.assign(new Error('disk full'),{details:{partial:true,samples_deleted:1000,runs_deleted:0,cutoff,policy:'30d'}}))
    await click('执行清理')
    expect(document.querySelector('dialog')?.textContent).toContain('部分清理')
    expect(document.querySelector('dialog')?.textContent).toContain('1,000 条采样')
    expect(button('执行清理')).toBeUndefined()
    expect(document.querySelector('dialog')?.textContent).not.toContain('记录保持不变')
    vi.mocked(request).mockResolvedValueOnce({...preview,samples_to_delete:200});await click('重新预览剩余记录')
    expect(request).toHaveBeenLastCalledWith('/api/monitor/retention/preview','POST',{policy:'30d',custom_days:0,cutoff_time:cutoff},'PreviewMonitorRetention',[{policy:'30d',custom_days:0,cutoff_time:cutoff}])
    expect(document.querySelector('dialog')?.textContent).toContain('200')
    expect(vi.mocked(request).mock.calls.filter(([path])=>path==='/api/monitor/retention')).toHaveLength(1)
    vi.mocked(request).mockResolvedValueOnce({policy:'30d',cutoff,samples_deleted:200,runs_deleted:2,partial:false});await click('执行清理')
    expect(document.querySelector('dialog')).toBeNull();expect(document.body.textContent).toContain('200 条采样、2 条运行记录')
  })

  it('does not claim unchanged history after an unconfirmed execution error',async()=>{
    await start();vi.mocked(request).mockResolvedValueOnce(preview);await click('预览旧采样清理')
    vi.mocked(request).mockRejectedValueOnce(new Error('连接中断'));await click('执行清理')
    expect(document.querySelector('dialog')?.textContent).toContain('未能确认清理结果')
    expect(button('重新预览剩余记录')).toBeTruthy();await click('返回')
    expect(document.querySelector('.operation-status')?.textContent).not.toContain('保持不变')
  })

  it('keeps a successful cleanup separate from the subsequent capacity read failure',async()=>{
    await start();vi.mocked(request).mockResolvedValueOnce(preview);await click('预览旧采样清理')
    vi.mocked(api.storage).mockRejectedValueOnce(new Error('capacity offline'))
    vi.mocked(request).mockResolvedValueOnce({policy:'30d',cutoff,samples_deleted:1200,runs_deleted:2,partial:false});await click('执行清理')
    expect(document.querySelector('dialog')).toBeNull()
    expect(document.querySelector('.operation-status')?.textContent).toContain('1,200 条采样、2 条运行记录')
    expect(document.querySelector('.storage-ledger')?.textContent).toContain('capacity offline')
  })

  it('refuses incomplete preview counts and disables execution for a known empty range',async()=>{
    await start();vi.mocked(request).mockResolvedValueOnce({policy:'30d',cutoff});await click('预览旧采样清理')
    expect(button('执行清理')).toBeUndefined();expect(document.body.textContent).toContain('清理预览不完整')
    vi.mocked(request).mockResolvedValueOnce({...preview,samples_to_delete:0,runs_to_delete:0});await click('预览旧采样清理')
    expect(button('执行清理').disabled).toBe(true)
    expect(document.querySelector('dialog')?.textContent).toContain('没有需要清理的监测记录')
  })

  it('keeps saving preferences independent from preview or deletion',async()=>{
    await start();await wrapper.get('.settings-control-grid select').setValue('90d');await click('保存设置')
    expect(api.saveSettings).toHaveBeenCalledWith(expect.objectContaining({monitor_retention_policy:'90d'}))
    expect(request).not.toHaveBeenCalled();expect(document.body.textContent).toContain('保存设置不会删除历史')
  })
})

describe('backup result continuity',()=>{
  it('shows successful export separately from metadata refresh failure and never re-exports on a read retry',async()=>{
    await start(DataView);vi.mocked(backup).mockResolvedValueOnce(true);vi.mocked(api.setup).mockRejectedValueOnce(new Error('setup offline'))
    await click('导出备份')
    expect(document.querySelector('.operation-status')?.textContent).toContain('备份已导出')
    expect(document.querySelector('[role=alert]')?.textContent).toContain('备份已导出，但状态刷新失败')
    vi.spyOn(workspace,'boot').mockImplementationOnce(async()=>{workspace.setup.value={...workspace.setup.value!,last_backup_at:'2026-11-18T00:00:00Z'};workspace.error.value=''})
    await click('刷新状态')
    expect(document.querySelector('.operation-status')?.textContent).toContain('状态已刷新')
    expect(document.querySelector('.path-list time')?.getAttribute('datetime')).toBe('2026-11-18T00:00:00Z')
    expect(backup).toHaveBeenCalledOnce()
  })
})
