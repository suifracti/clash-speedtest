import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises} from '@vue/test-utils'
import {api,events} from '../api'
import {createWorkspace} from '../workspace'
import type {Attempt,LatencyBatch,LatencyTest,NodeOption,Plan} from '../domain'

vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),clear:()=>items.clear()}})})
let onEvent:(type:string,payload:any)=>void
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,events:vi.fn((callback:any)=>{onEvent=callback;return vi.fn()}),api:{...actual.api,measurementRounds:vi.fn(async()=>[]),createRound:vi.fn(async()=>undefined),finishRound:vi.fn(async()=>undefined),auth:vi.fn(async()=>({auth_required:false,authenticated:true})),setup:vi.fn(async()=>({state:'ready'})),airports:vi.fn(),nodes:vi.fn(),catalog:vi.fn(async()=>[]),jobs:vi.fn(async()=>[]),settings:vi.fn(async()=>({})),token:vi.fn(async()=>({has_token:false})),batches:vi.fn(async()=>[]),batch:vi.fn(),batchProgress:vi.fn(),startLatency:vi.fn(),cancelBatch:vi.fn(),startAttempt:vi.fn(),attempt:vi.fn(),attemptAction:vi.fn(),attemptHistory:vi.fn(async()=>({attempts:[],has_more:false,complete:true}))}}})
const node:NodeOption={profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A',display_name:'Node A',type:'http',country_code:'HK',country_flag:''}
const plan:Plan={nodes:[node],projects:['download'],target:'all',samples:6,timeout:5,concurrency:8,downloadMiB:1,downloadSeconds:2,serviceIds:[],repeats:1}
function attempt(state='running'):Attempt{return {...node,attempt_id:'attempt-a',request_id:'request-a',requested_at:'2026-10-02T12:00:00Z',execution_state:state,persistence_state:state==='running'?'not_started':'saved',rule:{target_url:'https://probe.example'},result:state==='running'?undefined:{outcome:'byte_limit',bytes_read:1048576,duration_ns:1000000000,finished_at:'2026-10-02T12:00:01Z'}}}
function batch(state='running',requestId='latency-request'):LatencyBatch{const finished=state!=='running',persistence=state==='saving'?'saving':finished?'saved':'not_started';return {batch_id:'batch-a',request_id:requestId,state,requested_at:'2026-10-02T12:00:00Z',item_count:1,items:[{...node,item_id:'item-a',batch_id:'batch-a',execution_state:finished?'completed':'running',persistence_state:persistence,result:finished?{...node,attempt_id:'latency-a',display_name:node.display_name,node_type:node.type,requested_at:'2026-10-02T12:00:00Z',finished_at:'2026-10-02T12:00:01Z',status:'completed',latency_ms:42,total_samples:1,success_samples:1,failure_samples:0,samples:[],persistence_state:persistence}:undefined}]}}
const workspaces:ReturnType<typeof createWorkspace>[]=[]
async function workspace(){const w=createWorkspace();workspaces.push(w);await w.boot();return w}
beforeEach(()=>{vi.useFakeTimers();vi.resetAllMocks();vi.mocked(api.measurementRounds).mockResolvedValue([]);vi.mocked(api.createRound).mockResolvedValue(undefined);vi.mocked(api.finishRound).mockResolvedValue(undefined);window.localStorage.clear();window.sessionStorage.clear();vi.mocked(events).mockImplementation((callback)=>{onEvent=callback;return vi.fn()});vi.mocked(api.auth).mockResolvedValue({auth_required:false,authenticated:true});vi.mocked(api.setup).mockResolvedValue({state:'ready'} as any);vi.mocked(api.airports).mockResolvedValue([{id:'airport-a',name:'Airport A',subscriptions:[{id:'sub-a'}]}] as any);vi.mocked(api.nodes).mockResolvedValue([node]);vi.mocked(api.catalog).mockResolvedValue([]);vi.mocked(api.jobs).mockResolvedValue([]);vi.mocked(api.settings).mockResolvedValue({} as any);vi.mocked(api.token).mockResolvedValue({has_token:false} as any);vi.mocked(api.batches).mockResolvedValue([]);vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[],has_more:false,complete:true})})
afterEach(()=>{workspaces.splice(0).forEach(w=>w.dispose());vi.useRealTimers()})

describe('execution recovery',()=>{
  it('links early download progress to the original request and reconciles it with the final cancelled result',async()=>{
    const w=await workspace();w.plan.value={...plan}
    let admit!:(value:Attempt)=>void
    vi.mocked(api.startAttempt).mockImplementation(()=>new Promise(resolve=>{admit=resolve}))
    const running=w.runPlan();await flushPromises()
    const row=w.queue.value[0]
    const accepted={...attempt(),request_id:row.id,live_bytes_read:512}
    const sample={request_id:row.id,attempt_id:accepted.attempt_id}
    onEvent('workbench_download_progress',{...sample,request_id:'another-request',bytes_read:524288})
    expect(row.downloadBytesRead).toBeUndefined()
    onEvent('workbench_download_progress',{...sample,bytes_read:1024})
    admit(accepted);await flushPromises()
    expect(row.downloadBytesRead).toBe(1024)
    onEvent('workbench_download_progress',{...sample,attempt_id:'another-attempt',bytes_read:524288})
    expect(row.downloadBytesRead).toBe(1024)
    vi.mocked(api.attempt).mockResolvedValue({...accepted,live_bytes_read:2048})
    w.page.value='history';await w.syncActive()
    expect(row.downloadBytesRead).toBe(2048)
    const cancelled={...attempt('user_cancelled'),request_id:row.id,result:{...attempt('completed').result!,outcome:'user_cancelled',bytes_read:3072}}
    vi.mocked(api.attemptAction).mockResolvedValue(cancelled)
    await w.cancelTests()
    onEvent('workbench_download_progress',{...sample,bytes_read:524288})
    await vi.advanceTimersByTimeAsync(1000);await running
    expect(row.downloadBytesRead).toBe(2048)
    expect(row.attempt?.result?.bytes_read).toBe(3072)
    expect(row.state).toBe('user_cancelled')
  })

  it('polls only counters while latency is running and reads the full batch once finished',async()=>{
    const w=await workspace();w.plan.value={...plan,projects:['latency']}
    vi.mocked(api.startLatency).mockImplementation(async(body:any)=>batch('running',body.request_id))
    const counts={batch_id:'batch-a',state:'running',total:1,processed:1,all_success:1,partial_response:0,no_response:0,save_failed:0,not_executed:0}
    vi.mocked(api.batchProgress).mockResolvedValue(counts)
    const running=w.runPlan();await flushPromises();await vi.advanceTimersByTimeAsync(1200)
    expect(api.batchProgress).toHaveBeenCalledWith('batch-a');expect(api.batch).not.toHaveBeenCalled();expect(w.latest(node)).toBeUndefined();expect(w.done.value).toBe(w.activeBatchProgress.value?.processed)
    const completed=batch('completed',w.activeBatch.value!.request_id)
    onEvent('workbench_latency_test_completed',completed.items![0].result)
    expect(w.latest(node)).toBeUndefined()
    vi.mocked(api.batchProgress).mockResolvedValue({...counts,state:'completed',processed:1,all_success:1});vi.mocked(api.batch).mockResolvedValue(completed)
    await vi.advanceTimersByTimeAsync(1200);await running
    expect(api.batch).toHaveBeenCalledTimes(1);expect(w.latest(node)?.latency_ms).toBe(42);expect(w.reportSelection.value?.batch.batch_id).toBe('batch-a')
  })

  it('keeps an accepted attempt through a temporary read failure and uses its terminal event to release the queue',async()=>{
    const w=await workspace();w.plan.value={...plan};vi.mocked(api.startAttempt).mockImplementation(async(_kind,body:any)=>({...attempt(),request_id:body.request_id}));vi.mocked(api.attempt).mockRejectedValueOnce(new Error('temporary read failure'))
    const running=w.runPlan();await flushPromises();await vi.advanceTimersByTimeAsync(1000)
    const accepted=w.queue.value[0].attempt!;onEvent('workbench_download_attempt_updated',{...attempt('completed'),request_id:accepted.request_id})
    expect(w.queue.value[0].state).toBe('completed');expect(w.activeAttempt.value?.attempt.execution_state).toBe('completed')
    await vi.advanceTimersByTimeAsync(1000);await running;expect(w.busy.value).toBe(false)
  })
  it('recovers an accepted service attempt after a workspace reload using its saved identity',async()=>{
    const first=await workspace();first.plan.value={...plan,projects:['service'],serviceIds:['google_204']};vi.mocked(api.startAttempt).mockImplementation(async(_kind,body:any)=>({...attempt(),request_id:body.request_id,service_id:'google_204'}));vi.mocked(api.attempt).mockResolvedValue(attempt())
    const running=first.runPlan();await flushPromises();const accepted=first.queue.value[0].attempt!;expect(JSON.parse(window.sessionStorage.getItem('speedtest.active-work')!).serviceId).toBe('google_204');first.dispose()
    vi.mocked(api.attempt).mockResolvedValue({...accepted,execution_state:'running'})
    const restored=await workspace();expect(restored.activeAttempt.value?.kind).toBe('service');expect(restored.activeAttempt.value?.attempt.attempt_id).toBe(accepted.attempt_id);expect(restored.busy.value).toBe(true);expect(vi.mocked(api.attempt).mock.calls[0][2]).toMatchObject({service_id:'google_204'})
    expect(restored.queue.value[0].node.display_name).toBe(node.display_name)
    onEvent('workbench_public_service_attempt_updated',{...accepted,execution_state:'completed',persistence_state:'saved'});await vi.advanceTimersByTimeAsync(1000);await running;expect(restored.busy.value).toBe(false)
  })
  it('absorbs results when the initial latency response is already terminal and attributes a rejected start to its queue item',async()=>{
    const w=await workspace();w.plan.value={...plan,projects:['latency']}
    const result={...node,attempt_id:'latency-a',requested_at:'2026-10-02T12:00:00Z',finished_at:'2026-10-02T12:00:01Z',status:'completed',persistence_state:'saved',samples:[]} as unknown as LatencyTest
    vi.mocked(api.startLatency).mockImplementation(async(body:any)=>({batch_id:'batch-a',request_id:body.request_id,state:'completed',requested_at:result.requested_at,item_count:1,items:[{...node,item_id:'item-a',batch_id:'batch-a',execution_state:'completed',persistence_state:'saved',result}]}))
    await w.runPlan();expect(w.latest(node)?.attempt_id).toBe('latency-a');expect(w.queue.value[0].state).toBe('completed');expect(w.busy.value).toBe(false)
    w.plan.value={...plan,projects:['latency']};vi.mocked(api.startLatency).mockRejectedValueOnce(Object.assign(new Error('invalid selections'),{status:400}));await w.runPlan()
    expect(w.queue.value[0].state).toBe('not_executed');expect(w.queue.value[0].error).toBe('invalid selections');expect(w.busy.value).toBe(false)
  })
  it('keeps an unknown start visible and only looks up its original request without resubmission',async()=>{
    const w=await workspace(),oldBatch={batch_id:'old-batch',request_id:'old-request',state:'completed',requested_at:'2026-10-02T11:00:00Z',item_count:0};w.activeBatch.value=oldBatch;w.plan.value={...plan};vi.mocked(api.startAttempt).mockRejectedValueOnce(new TypeError('network unavailable'))
    await w.runPlan();expect(w.busy.value).toBe(true);expect(w.pendingStart.value?.kind).toBe('download')
    onEvent('workbench_latency_batch_updated',oldBatch);expect(w.pendingStart.value?.kind).toBe('download')
    const original=vi.mocked(api.startAttempt).mock.calls[0][1] as Record<string,unknown>,saved=JSON.parse(window.sessionStorage.getItem('speedtest.active-work')!)
    expect(saved.node).toEqual({profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a'});expect(saved.body).toMatchObject({request_id:original.request_id,maximum_bytes:1048576,timeout_seconds:2})
    await w.cancelTests();expect(w.error.value).toContain('无法确认取消');expect(w.busy.value).toBe(true)
    await w.retryPending();expect(api.startAttempt).toHaveBeenCalledTimes(1);expect(w.pendingStart.value?.requestId).toBe(original.request_id)
    vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[{...attempt('completed'),request_id:String(original.request_id)}],has_more:false,complete:true});await w.retryPending()
    expect(api.startAttempt).toHaveBeenCalledTimes(1);expect(w.pendingStart.value).toBe(null);expect(w.queue.value[0].state).toBe('completed');expect(w.busy.value).toBe(false);expect(window.sessionStorage.getItem('speedtest.active-work')).toBe(null)
  })
  it('keeps a latency batch recoverable while its measured results are still saving',async()=>{
    const first=await workspace();first.plan.value={...plan,projects:['latency']};vi.mocked(api.startLatency).mockImplementation(async(body:any)=>batch('saving',body.request_id))
    const running=first.runPlan();await flushPromises();expect(first.busy.value).toBe(true)
    const accepted=first.activeBatch.value!;expect(window.sessionStorage.getItem('speedtest.active-work')).not.toBe(null);first.dispose();vi.mocked(api.batchProgress).mockResolvedValue({batch_id:'batch-a',state:'saving',total:1,processed:1,all_success:1,partial_response:0,no_response:0,save_failed:0,not_executed:0})
    const restored=await workspace();expect(restored.activeBatch.value?.state).toBe('saving');expect(restored.busy.value).toBe(true)
    vi.mocked(api.batchProgress).mockResolvedValue({batch_id:'batch-a',state:'completed',total:1,processed:1,all_success:1,partial_response:0,no_response:0,save_failed:0,not_executed:0});vi.mocked(api.batch).mockResolvedValue(batch('completed',accepted.request_id));await vi.advanceTimersByTimeAsync(1200);await running
    expect(restored.busy.value).toBe(false);expect(restored.latest(node)?.latency_ms).toBe(42);expect(window.sessionStorage.getItem('speedtest.active-work')).toBe(null)
  })
  it('does not let a disposed plan rejection erase the replacement workspace task',async()=>{
    const other={...node,node_key:'node-b',node_identity_key:'identity-b',display_name:'Node B'};vi.mocked(api.nodes).mockResolvedValue([node,other])
    const first=await workspace();first.plan.value={...plan,nodes:[node,other]};let rejectOld!:(e:unknown)=>void,oldRequest=''
    vi.mocked(api.startAttempt).mockImplementationOnce(async(_kind,body:any)=>{oldRequest=body.request_id;return new Promise((_resolve,reject)=>rejectOld=reject)})
    const oldRunning=first.runPlan();await flushPromises();first.dispose()
    vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[{...attempt('completed'),request_id:oldRequest}],has_more:false,complete:true})
    const replacement=await workspace();expect(replacement.busy.value).toBe(false);expect(replacement.activeAttempt.value?.attempt.attempt_id).toBe('attempt-a');expect(replacement.downloads.value[Object.keys(replacement.downloads.value)[0]][0].result?.bytes_read).toBe(1048576);replacement.plan.value={...plan,nodes:[other]}
    vi.mocked(api.startAttempt).mockImplementationOnce(async(_kind,body:any)=>({...attempt(),...other,attempt_id:'attempt-b',request_id:body.request_id}))
    const newRunning=replacement.runPlan();await flushPromises();const newRequest=replacement.activeAttempt.value!.attempt.request_id
    rejectOld(Object.assign(new Error('late rejected start'),{status:400}));await flushPromises();await oldRunning
    expect(JSON.parse(window.sessionStorage.getItem('speedtest.active-work')!).requestId).toBe(newRequest);expect(replacement.activeAttempt.value?.attempt.attempt_id).toBe('attempt-b');expect(api.startAttempt).toHaveBeenCalledTimes(2)
    replacement.dispose();await vi.advanceTimersByTimeAsync(1000);await newRunning
  })
  it('does not resume the old node queue when the same workspace boots again after authentication',async()=>{
    const other={...node,node_key:'node-b',node_identity_key:'identity-b',display_name:'Node B'};vi.mocked(api.nodes).mockResolvedValue([node,other])
    const w=await workspace();w.plan.value={...plan,nodes:[node,other]};let resolveOld!:(a:Attempt)=>void,oldRequest=''
    vi.mocked(api.startAttempt).mockImplementationOnce(async(_kind,body:any)=>{oldRequest=body.request_id;return new Promise(resolve=>resolveOld=resolve)}).mockImplementation(async(_kind,body:any)=>({...attempt('completed'),...other,attempt_id:'unexpected-next-node',request_id:body.request_id}))
    const running=w.runPlan();await flushPromises();w.dispose();vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[{...attempt('completed'),request_id:oldRequest}],has_more:false,complete:true});await w.boot()
    resolveOld({...attempt(),request_id:oldRequest});await flushPromises();await running
    expect(api.startAttempt).toHaveBeenCalledTimes(1);expect(w.busy.value).toBe(false);expect(w.queue.value[1].state).toBe('not_executed')
  })
  it('preserves a service save failure and later successful retry when an older saving read arrives',async()=>{
    const w=await workspace();w.plan.value={...plan,projects:['service'],serviceIds:['google_204']};let finishRead!:(a:Attempt)=>void
    vi.mocked(api.startAttempt).mockImplementation(async(_kind,body:any)=>({...attempt('completed'),request_id:body.request_id,service_id:'google_204',persistence_state:'saving'}));vi.mocked(api.attempt).mockImplementationOnce(()=>new Promise(resolve=>finishRead=resolve))
    const running=w.runPlan();await flushPromises();expect(w.busy.value).toBe(true);expect(window.sessionStorage.getItem('speedtest.active-work')).not.toBe(null);await vi.advanceTimersByTimeAsync(1000)
    const saving=w.activeAttempt.value!.attempt,failed={...saving,persistence_state:'failed',persistence_error:'history write failed'}
    onEvent('workbench_public_service_attempt_updated',failed);finishRead(saving);await flushPromises()
    expect(w.activeAttempt.value?.attempt.persistence_state).toBe('failed');expect(w.queue.value[0].error).toBe('history write failed');await running;expect(w.busy.value).toBe(false);expect(w.services.value[Object.keys(w.services.value)[0]][0].result?.bytes_read).toBe(1048576)
    const saved={...saving,persistence_state:'saved'};onEvent('workbench_public_service_attempt_updated',saved);onEvent('workbench_public_service_attempt_updated',failed);onEvent('workbench_public_service_attempt_updated',saving)
    expect(w.activeAttempt.value?.attempt.persistence_state).toBe('saved');expect(w.queue.value[0].error).toBeUndefined();expect(w.busy.value).toBe(false);expect(window.sessionStorage.getItem('speedtest.active-work')).toBe(null)
  })
  it('connects to backend events while setup is uninitialized without loading ready-only workspace data',async()=>{
    vi.mocked(api.setup).mockResolvedValueOnce({state:'needs_choice'} as any)
    vi.mocked(events).mockImplementationOnce((callback,onConnection)=>{onEvent=callback;onConnection(true);return vi.fn()})
    const w=await workspace()
    expect(w.page.value).toBe('data');expect(w.connected.value).toBe(true);expect(api.airports).not.toHaveBeenCalled()
    onEvent('subscription_usage_updated',{});await flushPromises();expect(api.airports).not.toHaveBeenCalled()
    w.dispose();expect(w.connected.value).toBe(false)
  })
  it('coalesces repeated startup reads while the first setup request is pending',async()=>{
    let finishSetup!:(value:any)=>void
    vi.mocked(api.setup).mockImplementationOnce(()=>new Promise(resolve=>finishSetup=resolve))
    const w=createWorkspace();workspaces.push(w);const first=w.boot();await flushPromises();const second=w.boot();await flushPromises()
    expect(api.auth).toHaveBeenCalledTimes(1);expect(api.setup).toHaveBeenCalledTimes(1)
    finishSetup({state:'ready'});await Promise.all([first,second]);expect(events).toHaveBeenCalledTimes(1)
  })
  it('does not continue a disposed startup after its authentication response arrives',async()=>{
    let finishAuth!:(value:any)=>void
    vi.mocked(api.auth).mockImplementationOnce(()=>new Promise(resolve=>finishAuth=resolve))
    const w=createWorkspace();workspaces.push(w);const first=w.boot();w.dispose()
    finishAuth({auth_required:false,authenticated:true});await first
    expect(api.setup).not.toHaveBeenCalled();expect(events).not.toHaveBeenCalled();expect(w.setup.value).toBe(null)
  })
  it('does not let a late setup response replace the workspace reopened after authentication',async()=>{
    let finishSetup!:(value:any)=>void
    vi.mocked(api.setup).mockImplementationOnce(()=>new Promise(resolve=>finishSetup=resolve))
    const w=createWorkspace();workspaces.push(w);const first=w.boot();await flushPromises();w.dispose();await w.boot()
    finishSetup({state:'needs_choice'});await first
    expect(w.setup.value?.state).toBe('ready');expect(w.page.value).toBe('home');expect(events).toHaveBeenCalledTimes(1)
  })
  it('keeps newly loaded nodes and services when earlier startup reads finish late',async()=>{
    let finishNodes!:(value:NodeOption[])=>void,finishCatalog!:(value:any[])=>void
    vi.mocked(api.nodes).mockImplementationOnce(()=>new Promise(resolve=>finishNodes=resolve))
    vi.mocked(api.catalog).mockImplementationOnce(()=>new Promise(resolve=>finishCatalog=resolve))
    const w=createWorkspace();workspaces.push(w);const first=w.boot();await flushPromises();w.dispose()
    const replacement={...node,node_key:'replacement',node_identity_key:'new-identity',config_revision_key:'new-revision'}
    vi.mocked(api.nodes).mockResolvedValue([replacement]);vi.mocked(api.catalog).mockResolvedValue([{service_id:'current-service'}] as any);await w.boot()
    finishNodes([node]);finishCatalog([{service_id:'old-service'}]);await first
    expect(w.nodes.value).toEqual([replacement]);expect(w.catalog.value.map(s=>s.service_id)).toEqual(['current-service'])
  })
  it('does not restore a stale completed batch after a new workspace session has opened',async()=>{
    const w=await workspace();w.pendingStart.value={kind:'latency',requestId:'old-request'}
    vi.mocked(api.batches).mockResolvedValueOnce([batch('completed','old-request')]);let finishBatch!:(value:LatencyBatch)=>void
    vi.mocked(api.batch).mockImplementationOnce(()=>new Promise(resolve=>finishBatch=resolve));const recovery=w.retryPending();await flushPromises()
    w.dispose();w.pendingStart.value=null;await w.boot();finishBatch(batch('completed','old-request'));await recovery
    expect(w.batches.value).toEqual([]);expect(w.latest(node)).toBeUndefined();expect(w.pendingStart.value).toBe(null)
  })
})
