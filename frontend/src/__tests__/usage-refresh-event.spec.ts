import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises} from '@vue/test-utils'
import {api,events} from '../api'
import {createWorkspace} from '../workspace'

vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),clear:()=>items.clear()}})})
let onEvent:(type:string,payload:any)=>void
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,events:vi.fn((callback:any)=>{onEvent=callback;return vi.fn()}),api:{...actual.api,measurementRounds:vi.fn(async()=>[]),createRound:vi.fn(async()=>undefined),finishRound:vi.fn(async()=>undefined),auth:vi.fn(async()=>({auth_required:false,authenticated:true})),setup:vi.fn(async()=>({state:'ready'})),airports:vi.fn(),nodes:vi.fn(),catalog:vi.fn(async()=>[]),jobs:vi.fn(async()=>[]),settings:vi.fn(async()=>({})),token:vi.fn(async()=>({has_token:false})),batches:vi.fn(async()=>[]),batch:vi.fn(),startLatency:vi.fn(),cancelBatch:vi.fn(),startAttempt:vi.fn(),attempt:vi.fn(),attemptAction:vi.fn(),attemptHistory:vi.fn(async()=>({attempts:[],has_more:false,complete:true})),subscriptionUsageState:vi.fn(async()=>({state:'completed'}))}}})
const workspaces:ReturnType<typeof createWorkspace>[]=[]
async function workspace(){const w=createWorkspace();workspaces.push(w);await w.boot();return w}
const airport=(lastFailed:boolean)=>({id:'airport-a',name:'Airport A',subscriptions:[{id:'sub-a',last_attempt_at:'2026-10-24T16:45:58.856141+08:00',last_attempt_failed:lastFailed}]})
beforeEach(()=>{vi.useFakeTimers();vi.resetAllMocks();window.localStorage.clear();window.sessionStorage.clear();vi.mocked(events).mockImplementation((callback)=>{onEvent=callback;return vi.fn()});vi.mocked(api.auth).mockResolvedValue({auth_required:false,authenticated:true});vi.mocked(api.setup).mockResolvedValue({state:'ready'} as any);vi.mocked(api.airports).mockResolvedValue([airport(false)] as any);vi.mocked(api.nodes).mockResolvedValue([]);vi.mocked(api.catalog).mockResolvedValue([]);vi.mocked(api.jobs).mockResolvedValue([]);vi.mocked(api.settings).mockResolvedValue({} as any);vi.mocked(api.token).mockResolvedValue({has_token:false} as any);vi.mocked(api.batches).mockResolvedValue([]);vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[],has_more:false,complete:true});vi.mocked(api.subscriptionUsageState).mockResolvedValue({state:'completed'} as any)})
afterEach(()=>{workspaces.splice(0).forEach(w=>w.dispose());vi.useRealTimers()})

describe('background usage refresh',()=>{
  it('re-reads airports and drops cached daily buckets when a scheduled run finishes',async()=>{
    const w=await workspace()
    const reads=vi.mocked(api.airports).mock.calls.length,version=w.usageRefreshVersion.value
    vi.mocked(api.airports).mockResolvedValue([airport(true)] as any)
    onEvent('subscription_usage_updated',{state:'completed_with_issues',message:'已尝试 3 个订阅，2 个更新或保存异常；旧快照保留'})
    await flushPromises()
    expect(vi.mocked(api.airports).mock.calls.length).toBe(reads+1)
    expect(w.usageRefreshVersion.value).toBe(version+1)
    expect(w.airports.value[0].subscriptions?.[0]?.last_attempt_failed).toBe(true)
  })
  it('keeps the page usable when the re-read fails',async()=>{
    const w=await workspace()
    vi.mocked(api.airports).mockRejectedValueOnce(new Error('temporary read failure'))
    onEvent('subscription_usage_updated',{state:'completed'})
    await flushPromises()
    expect(w.airports.value).toHaveLength(1)
    expect(w.error.value).toBe('')
  })
  it('waits for a run that started before the window opened',async()=>{
    vi.mocked(api.subscriptionUsageState).mockResolvedValue({state:'running'} as any)
    const w=createWorkspace();workspaces.push(w)
    const booting=w.boot();await flushPromises()
    const version=w.usageRefreshVersion.value,reads=vi.mocked(api.airports).mock.calls.length
    vi.mocked(api.subscriptionUsageState).mockResolvedValue({state:'completed_with_issues'} as any)
    vi.mocked(api.airports).mockResolvedValue([airport(true)] as any)
    await vi.advanceTimersByTimeAsync(2100);await flushPromises();await booting
    expect(vi.mocked(api.airports).mock.calls.length).toBe(reads+1)
    expect(w.usageRefreshVersion.value).toBe(version+1)
    expect(w.airports.value[0].subscriptions?.[0]?.last_attempt_failed).toBe(true)
  })
})
