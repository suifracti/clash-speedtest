import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import {api,request,saveFile} from '../api'
import {createWorkspace,workspaceKey} from '../workspace'
import MonitorTransfer from '../components/MonitorTransfer.vue'
import type {MonitorCreate,MonitorJob,NodeOption} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{},removeItem:()=>{}}})})
vi.mock('../api',()=>({api:{nodes:vi.fn(),jobs:vi.fn(),createJob:vi.fn()},request:vi.fn(),saveFile:vi.fn(),events:vi.fn(),mapLimited:vi.fn()}))
const node:NodeOption={profile_id:'account-a',profile_name:'示例机场 / 日常账户',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',display_name:'示例线路',type:'ss',country_code:'HK',country_flag:''}
const config=(name='日常观察'):MonitorCreate=>({name,profile_id:node.profile_id,node_keys:[node.node_key],node_contexts:[{node_key:node.node_key,node_identity_key:node.node_identity_key,config_revision_key:node.config_revision_key}],probe_set:'latency_four_v1',sampling_tier:'regular',interval_seconds:300,timeout_seconds:10})
const job=(name='日常观察'):MonitorJob=>({...config(name),id:name,profile_name:node.profile_name,nodes:[node],state:'stopped',resume_on_launch:false,updated_at:'2026-11-18T00:00:00Z'})
const file=(jobs:unknown[]=[config()])=>JSON.stringify({format:'speedtest-monitor',version:1,jobs})
function deferred<T>(){let resolve!:(value:T)=>void,reject!:(error:unknown)=>void;const promise=new Promise<T>((yes,no)=>{resolve=yes;reject=no});return {promise,resolve,reject}}
const button=(label:string)=>[...document.querySelectorAll<HTMLButtonElement>('button')].find(el=>el.textContent?.includes(label))!
async function click(label:string){button(label).click();await flushPromises()}
async function choose(text=file(),size=text.length){const input=document.querySelector<HTMLInputElement>('input[type=file]')!;Object.defineProperty(input,'files',{configurable:true,value:[{name:'monitor-config.json',size,text:async()=>text}]});input.dispatchEvent(new Event('change',{bubbles:true}));await flushPromises()}
let wrapper:VueWrapper,workspace:ReturnType<typeof createWorkspace>
async function start(){workspace=createWorkspace();wrapper=mount(MonitorTransfer,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:workspace},stubs:{Icon:true}}});await click('任务配置')}
beforeEach(()=>{vi.resetAllMocks();vi.mocked(api.nodes).mockResolvedValue([node]);vi.mocked(api.jobs).mockResolvedValue([job()]);vi.mocked(api.createJob).mockImplementation(async body=>job(body.name));vi.mocked(saveFile).mockResolvedValue(true);HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()})
afterEach(()=>{wrapper?.unmount();document.body.innerHTML=''})

describe('monitor configuration transfer',()=>{
  it('inspects without mutation, shows unmatched tasks, and imports only the explicitly selected valid rows',async()=>{
    await start();await choose(file([config(),{...config('来源已移除'),profile_id:'missing-account'},config('稍后再加')]))
    expect(api.createJob).not.toHaveBeenCalled()
    expect(document.querySelectorAll('.transfer-row')).toHaveLength(3)
    expect(document.body.textContent).toContain('无法匹配')
    const inputs=document.querySelectorAll<HTMLInputElement>('.transfer-task input');expect(inputs[1].disabled).toBe(true);inputs[2].click();await flushPromises()
    await click('新增所选 1 项')
    expect(api.createJob).toHaveBeenCalledOnce();expect(api.createJob).toHaveBeenCalledWith(config())
    expect(workspace.jobs.value).toEqual([job()]);expect(wrapper.emitted('imported')).toHaveLength(1)
    expect(document.body.textContent).toContain('已新增 · 停止')
    expect(document.body.textContent).toContain('本次已新增 1 项任务，均为停止状态')
  })

  it('rechecks the complete scope before writing and never silently drops newly changed nodes',async()=>{
    await start();await choose();vi.mocked(api.nodes).mockResolvedValueOnce([{...node,config_revision_key:'new-revision'}])
    await click('新增所选 1 项')
    expect(api.createJob).not.toHaveBeenCalled();expect(document.body.textContent).toContain('订阅缓存已变化');expect(document.body.textContent).toContain('节点配置已变化')
    expect(button('新增所选 0 项').disabled).toBe(true)
  })

  it('keeps confirmed successes after a later failure and retries only the remaining selection',async()=>{
    await start();await choose(file([config('第一项'),config('第二项')]))
    vi.mocked(api.createJob).mockResolvedValueOnce(job('第一项')).mockRejectedValueOnce(new Error('配置写入失败'))
    await click('新增所选 2 项')
    expect(workspace.jobs.value.map(row=>row.name)).toEqual(['第一项']);expect(document.body.textContent).toContain('本次已新增 1 项');expect(document.body.textContent).toContain('配置写入失败')
    expect(document.querySelector<HTMLInputElement>('.transfer-task input')!.disabled).toBe(true)
    await click('新增所选 1 项')
    expect(vi.mocked(api.createJob).mock.calls.map(([body])=>body.name)).toEqual(['第一项','第二项','第二项'])
    expect(workspace.jobs.value.map(row=>row.name)).toEqual(['第一项','第二项'])
    expect(wrapper.emitted('imported')).toHaveLength(2)
    expect(button('完成')).toBeTruthy()
  })

  it('locks file, selection, closing and duplicate submission while creating',async()=>{
    await start();await choose();const pending=deferred<MonitorJob>();vi.mocked(api.createJob).mockReturnValueOnce(pending.promise)
    button('新增所选 1 项').click();await flushPromises()
    expect(button('返回').disabled).toBe(true);expect(button('选择配置文件').disabled).toBe(true)
    expect(document.querySelector<HTMLInputElement>('.transfer-task input')!.disabled).toBe(true)
    expect(document.querySelector('[aria-label="关闭"]')).toBeNull()
    button('正在新增任务').click();await flushPromises();expect(api.createJob).toHaveBeenCalledOnce()
    pending.reject(new Error('写入失败'));await flushPromises()
    expect(button('新增所选 1 项').disabled).toBe(false);expect(document.querySelector('[role=alert]')?.textContent).toContain('写入失败')
  })

  it('exports a fresh read through the shared save dialog, distinguishes cancellation and preserves a pending import',async()=>{
    await start();await choose();workspace.jobs.value=[job('本地旧列表')]
    vi.mocked(api.jobs).mockResolvedValueOnce([job('后端当前任务')]);vi.mocked(saveFile).mockResolvedValueOnce(false)
    await click('导出当前任务')
    expect(JSON.parse(vi.mocked(saveFile).mock.calls[0][0]).jobs[0].name).toBe('后端当前任务')
    expect(vi.mocked(saveFile).mock.calls[0].slice(1)).toEqual(['monitor-config.json','application/json'])
    expect(document.body.textContent).toContain('已取消导出');expect(document.querySelectorAll('.transfer-row')).toHaveLength(1)
    vi.mocked(api.jobs).mockRejectedValueOnce(new Error('任务读取失败'));await click('导出当前任务');expect(saveFile).toHaveBeenCalledOnce();expect(document.body.textContent).toContain('任务读取失败')
  })

  it('keeps failed file and source reads separate from a valid empty result and allows selecting the file again',async()=>{
    await start();await choose('bad json');expect(document.body.textContent).toContain('无法读取 JSON');expect(api.nodes).not.toHaveBeenCalled()
    await choose(file(),1024*1024+1);expect(document.body.textContent).toContain('1 MiB');expect(api.nodes).not.toHaveBeenCalled()
    vi.mocked(api.nodes).mockRejectedValueOnce(new Error('缓存读取失败'));await choose();expect(document.body.textContent).toContain('缓存读取失败');expect(document.querySelectorAll('.transfer-row')).toHaveLength(0)
    await choose();expect(document.querySelectorAll('.transfer-row')).toHaveLength(1);expect(api.createJob).not.toHaveBeenCalled()
  })

  it('restores the monitor directory action and links to full data backups',async()=>{
    await start();await click('打开监测数据目录')
    expect(request).toHaveBeenCalledOnce();expect(request).toHaveBeenCalledWith('/api/data/open-folder?kind=monitor','POST',undefined,'OpenMonitorDataFolder')
    expect(document.body.textContent).toContain('数据库运行期间请勿直接编辑')
    await click('完整数据与备份');expect(workspace.page.value).toBe('data');expect(document.querySelector('dialog')).toBeNull()
  })
})
