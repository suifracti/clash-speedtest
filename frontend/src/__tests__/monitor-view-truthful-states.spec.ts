import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import {nextTick} from 'vue'
import MonitorView from '../components/MonitorView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api,request} from '../api'
import type {MonitorJob,NodeOption} from '../domain'

vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),removeItem:(k:string)=>items.delete(k),clear:()=>items.clear()}})})
vi.mock('../api',()=>({api:{jobs:vi.fn(),budget:vi.fn(),storage:vi.fn(),jobAction:vi.fn(),deleteJob:vi.fn()},request:vi.fn(),events:vi.fn(),mapLimited:vi.fn(),saveFile:vi.fn()}))

const node:NodeOption={profile_id:'profile-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A',display_name:'Node A',type:'ss',country_code:'HK',country_flag:''}
const job=(state='running'):MonitorJob=>({id:'job-a',name:'Job A',profile_id:'profile-a',profile_name:'Airport A',nodes:[node],node_keys:['node-a'],probe_set:'latency_six_v1',sampling_tier:'normal',interval_seconds:900,timeout_seconds:8,state,resume_on_launch:false,updated_at:'2026-10-11T02:00:00Z'})
const run={run_id:'run-a',started_at:'2026-10-11T02:00:00Z',status:'completed',success_nodes:1,total_nodes:1,failed_nodes:0}

function deferred<T>(){let resolve!:(value:T)=>void, reject!:(reason?:unknown)=>void;const promise=new Promise<T>((yes,no)=>{resolve=yes;reject=no});return {promise,resolve,reject}}
function button(label:string){return [...document.body.querySelectorAll('button')].find(element=>element.textContent?.includes(label))}
function setViewport(width:number,height:number){Object.defineProperty(window,'innerWidth',{configurable:true,value:width});Object.defineProperty(window,'innerHeight',{configurable:true,value:height})}

let cleanups:Array<()=>void>=[]
function mountMonitorView(initialJobs:MonitorJob[]=[]){
  const workspace=createWorkspace();workspace.jobs.value=initialJobs;workspace.page.value='monitor'
  const wrapper=mount(MonitorView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:workspace},stubs:{Icon:true,PeriodicSamplingPanel:{template:'<article class="periodic-panel-stub" aria-label="原有定时监测面板"></article>'}}}})
  let mounted=true
  const unmount=()=>{if(mounted){wrapper.unmount();mounted=false}}
  cleanups.push(unmount)
  return {wrapper,workspace,unmount}
}

beforeEach(()=>{
  window.localStorage.clear();window.sessionStorage.clear();document.documentElement.dataset.theme='light';setViewport(1440,900)
  vi.resetAllMocks()
  vi.mocked(api.jobs).mockResolvedValue([])
  vi.mocked(api.budget).mockResolvedValue({usage:{requests_used:0,bytes_used:0},blocked_reason:''})
  vi.mocked(api.storage).mockResolvedValue({total_bytes:0,protected:false})
  vi.mocked(api.jobAction).mockResolvedValue(undefined)
  vi.mocked(api.deleteJob).mockResolvedValue(undefined)
  vi.mocked(request).mockResolvedValue([])
  HTMLDialogElement.prototype.showModal=vi.fn()
  HTMLDialogElement.prototype.close=vi.fn()
})
afterEach(()=>{cleanups.forEach(cleanup=>cleanup());cleanups=[];document.body.innerHTML='';document.documentElement.dataset.theme='light';setViewport(1440,900)})

describe('MonitorView truthful read state and async interactions',()=>{
  it('shows unknown budget and storage when the first reads fail instead of displaying zero',async()=>{
    vi.mocked(api.budget).mockRejectedValueOnce(new Error('budget offline'))
    vi.mocked(api.storage).mockRejectedValueOnce(new Error('storage offline'))
    mountMonitorView();await flushPromises()
    const text=document.body.textContent||''
    expect(text).toContain('请求预算尚未读取')
    expect(text).toContain('预算读取失败')
    expect(text).toContain('历史占用尚未读取')
    expect(text).toContain('存储读取失败')
    expect(text).not.toContain('今日请求 0')
    expect(text).not.toContain('历史占用 0 B')
  })

  it('labels the last successful budget as stale after a later partial failure while showing real zero storage',async()=>{
    vi.mocked(api.budget).mockResolvedValueOnce({usage:{requests_used:17,bytes_used:2048}})
    vi.mocked(api.storage).mockResolvedValueOnce({total_bytes:1024,protected:false})
    mountMonitorView();await flushPromises()
    vi.mocked(api.budget).mockRejectedValueOnce(new Error('budget timeout'))
    vi.mocked(api.storage).mockResolvedValueOnce({total_bytes:0,protected:false})
    button('刷新')!.click();await flushPromises()
    const text=document.body.textContent||''
    expect(text).toContain('今日请求 17')
    expect(text).toContain('预算刷新失败')
    expect(text).toContain('显示上次成功读取于')
    expect(text).toContain('历史占用 0 B')
    expect(text).not.toContain('存储刷新失败')
  })

  it('shows a zero request count only when the backend supplied zero and keeps missing counters unknown',async()=>{
    vi.mocked(api.budget).mockResolvedValueOnce({usage:{requests_used:0},blocked_reason:'daily request budget reached'})
    mountMonitorView();await flushPromises()
    const text=document.body.textContent||''
    expect(text).toContain('今日请求 0')
    expect(text).toContain('已读取 未知')
    expect(text).toContain('daily request budget reached')
  })

  it('keeps the newest jobs and resource read when overlapping requests resolve out of order',async()=>{
    const firstJobs=deferred<MonitorJob[]>(),secondJobs=deferred<MonitorJob[]>()
    const firstBudget=deferred<Record<string,any>>()
    vi.mocked(api.jobs).mockReturnValueOnce(firstJobs.promise).mockReturnValueOnce(secondJobs.promise)
    vi.mocked(api.budget).mockReturnValueOnce(firstBudget.promise).mockResolvedValueOnce({usage:{requests_used:25,bytes_used:0}})
    const {workspace}=mountMonitorView();button('刷新')!.click();await flushPromises()
    secondJobs.resolve([job('paused')]);await flushPromises()
    firstJobs.resolve([job('running')]);firstBudget.resolve({usage:{requests_used:10,bytes_used:0}});await flushPromises()
    expect(workspace.jobs.value.map(item=>item.state)).toEqual(['paused'])
    expect(document.body.textContent).toContain('今日请求 25')
    expect(document.body.textContent).not.toContain('今日请求 10')
  })

  it('waits for the newest refresh before confirming an action state',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')])
    const {workspace}=mountMonitorView();await flushPromises()
    const actionRefresh=deferred<MonitorJob[]>(),newerRefresh=deferred<MonitorJob[]>()
    vi.mocked(api.jobs).mockReturnValueOnce(actionRefresh.promise).mockReturnValueOnce(newerRefresh.promise)
    button('暂停')!.click();await flushPromises()
    button('刷新')!.click();await flushPromises()
    actionRefresh.resolve([job('running')]);await flushPromises()
    expect(workspace.jobs.value[0].state).toBe('running')
    expect(workspace.toast.value).toBe('')
    expect(button('正在暂停')?.disabled).toBe(true)
    newerRefresh.resolve([job('paused')]);await flushPromises()
    expect(workspace.jobs.value[0].state).toBe('paused')
    expect(workspace.toast.value).toContain('任务状态已按后端列表重新读取')
  })

  it('ignores a jobs response that arrives after the component unmounts',async()=>{
    const pending=deferred<MonitorJob[]>()
    vi.mocked(api.jobs).mockReturnValueOnce(pending.promise)
    const {workspace,unmount}=mountMonitorView();await nextTick();unmount()
    pending.resolve([job()]);await flushPromises()
    expect(workspace.jobs.value).toEqual([])
  })

  it('keeps the displayed job state unchanged until a successful backend reread and reports busy state',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')]).mockResolvedValueOnce([job('paused')])
    const {workspace}=mountMonitorView();await flushPromises()
    const pending=deferred<void>();vi.mocked(api.jobAction).mockReturnValueOnce(pending.promise)
    button('暂停')!.click();await nextTick()
    expect(button('暂停')!.disabled).toBe(true)
    expect(button('暂停')!.textContent).toContain('正在暂停')
    expect(workspace.jobs.value[0].state).toBe('running')
    pending.resolve();await flushPromises()
    expect(workspace.jobs.value[0].state).toBe('paused')
    expect(document.body.textContent).toContain('已暂停')
    expect(button('继续')).toBeDefined()
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')])
    button('继续')!.click();await flushPromises()
    expect(api.jobAction).toHaveBeenLastCalledWith('job-a','resume',undefined)
    expect(workspace.jobs.value[0].state).toBe('running')
  })

  it('shows stopped state from the backend reread',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')]).mockResolvedValueOnce([job('stopped')])
    const {workspace}=mountMonitorView();await flushPromises()
    button('停止')!.click();await flushPromises()
    expect(api.jobAction).toHaveBeenCalledWith('job-a','stop',undefined)
    expect(workspace.jobs.value[0].state).toBe('stopped')
    expect(document.body.textContent).toContain('未运行')
    expect(button('停止')).toBeUndefined()
    expect(button('启动')).toBeDefined()
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')])
    button('启动')!.click();await flushPromises()
    expect(api.jobAction).toHaveBeenLastCalledWith('job-a','start',undefined)
    expect(workspace.jobs.value[0].state).toBe('running')
  })

  it('reports a single-trigger request without claiming a monitor result',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')]).mockResolvedValueOnce([job('running')])
    const {workspace}=mountMonitorView();await flushPromises()
    button('检测一次')!.click();await flushPromises()
    expect(workspace.toast.value).toContain('已请求执行一次检测')
    expect(workspace.toast.value).not.toContain('稳定')
  })

  it('writes startup-resume selection through the API and reflects only the reread value',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')]).mockResolvedValueOnce([{...job('running'),resume_on_launch:true}])
    mountMonitorView();await flushPromises()
    const checkbox=document.querySelector<HTMLInputElement>('input[type="checkbox"]')!
    checkbox.checked=true;checkbox.dispatchEvent(new Event('change',{bubbles:true}));await flushPromises()
    expect(api.jobAction).toHaveBeenCalledWith('job-a','resume-on-launch',{enabled:true})
    expect(document.querySelector<HTMLInputElement>('input[type="checkbox"]')?.checked).toBe(true)
  })

  it('keeps invalid frozen scope visible and prevents starting a blocked job',async()=>{
    const blocked={...job('blocked'),nodes:[],node_keys:[],unresolved_node_count:1,unresolved_nodes:[{node_key:'missing-node',node_identity_key:'identity-x',config_revision_key:'revision-x',display_name:'Missing Node',reason:'配置已变化'}]}
    vi.mocked(api.jobs).mockResolvedValueOnce([blocked])
    mountMonitorView();await flushPromises()
    expect(document.body.textContent).toContain('整项未启动')
    expect(document.body.textContent).toContain('Missing Node')
    expect(button('启动')?.disabled).toBe(true)
  })

  it('shows action failures on the job and lets the user retry',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')]).mockResolvedValueOnce([job('running')]).mockResolvedValueOnce([job('paused')])
    mountMonitorView();await flushPromises()
    vi.mocked(api.jobAction).mockRejectedValueOnce(new Error('pause unavailable')).mockResolvedValueOnce(undefined)
    button('暂停')!.click();await flushPromises()
    expect(document.querySelector('[role="alert"]')?.textContent||'').toContain('pause unavailable')
    expect(button('暂停')!.disabled).toBe(false)
    button('暂停')!.click();await flushPromises()
    expect(document.body.textContent).toContain('已暂停')
    expect(document.querySelector('[role="alert"]')).toBeNull()
  })

  it('lets the user cancel delete without calling the API and exposes preserved history in confirmation',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job()])
    mountMonitorView();await flushPromises()
    document.querySelector<HTMLButtonElement>('button[aria-label="删除监测任务"]')!.click();await nextTick()
    expect(document.querySelector('dialog')?.textContent).toContain('已经保存的原始记录保留')
    button('返回')!.click();await nextTick()
    expect(document.querySelector('dialog')).toBeNull()
    expect(api.deleteJob).not.toHaveBeenCalled()
  })

  it('shows delete busy feedback and calls only the job-delete API without deleting run history',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job()]).mockResolvedValueOnce([])
    const retainedHistory=[run]
    vi.mocked(request).mockImplementation(async(path,method='GET')=>{
      if(path.startsWith('/api/monitor/runs')&&method==='GET')return retainedHistory.slice() as any
      if(path.startsWith('/api/monitor/runs')&&method==='DELETE')retainedHistory.splice(0)
      return [] as any
    })
    const {workspace}=mountMonitorView();await flushPromises()
    button('运行记录')!.click();await flushPromises()
    expect(document.querySelector('dialog')?.textContent).toContain('完成')
    document.querySelector<HTMLButtonElement>('dialog button[aria-label="关闭"]')!.click();await nextTick()
    document.querySelector<HTMLButtonElement>('button[aria-label="删除监测任务"]')!.click();await nextTick()
    const pending=deferred<void>();vi.mocked(api.deleteJob).mockReturnValueOnce(pending.promise)
    button('删除任务')!.click();await nextTick()
    expect(button('正在删除')!.disabled).toBe(true)
    expect(button('返回')!.disabled).toBe(true)
    pending.resolve();await flushPromises()
    expect(api.deleteJob).toHaveBeenCalledWith('job-a')
    expect(workspace.toast.value).toContain('原始历史保留')
    expect(retainedHistory).toEqual([run])
    expect(request).toHaveBeenCalledTimes(1)
  })

  it('does not reopen run history when its request finishes after the dialog is closed',async()=>{
    vi.mocked(api.jobs).mockResolvedValueOnce([job()])
    mountMonitorView();await flushPromises()
    const pending=deferred<typeof run[]>();vi.mocked(request).mockReturnValueOnce(pending.promise)
    button('运行记录')!.click();await nextTick()
    document.querySelector<HTMLButtonElement>('dialog button[aria-label="关闭"]')!.click();await nextTick()
    pending.resolve([run]);await flushPromises()
    expect(document.querySelector('dialog')).toBeNull()
  })

  it.each([{width:1440,height:900,theme:'light'},{width:1440,height:900,theme:'dark'},{width:800,height:900,theme:'light'},{width:800,height:900,theme:'dark'}])('keeps monitor controls present at $width px in $theme mode',async({width,height,theme})=>{
    setViewport(width,height);document.documentElement.dataset.theme=theme
    vi.mocked(api.jobs).mockResolvedValueOnce([job('running')])
    mountMonitorView();await flushPromises()
    expect(document.querySelector('button[aria-label="删除监测任务"]')).not.toBeNull()
    expect(button('暂停')?.disabled).toBe(false)
    expect(button('检测一次')?.disabled).toBe(false)
    expect(document.querySelector<HTMLInputElement>('input[type="checkbox"]')?.disabled).toBe(false)
    expect(document.querySelector('.monitor-card footer .button-group')?.children.length).toBeGreaterThan(4)
    expect(document.documentElement.dataset.theme).toBe(theme)
  })
})
