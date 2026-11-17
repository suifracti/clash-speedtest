import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import NodeDetailModal from '../components/NodeDetailModal.vue'
import TrendChart from '../components/TrendChart.vue'
import {api} from '../api'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {serviceAvailability} from '../sharedResults'
import type {Attempt,NodeOption,LatencyTest} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,measurementRounds:vi.fn(async()=>[]),revisions:vi.fn(async()=>[]),attemptHistory:vi.fn(),previousAttempt:vi.fn(),latencyHistory:vi.fn(),monitorSamples:vi.fn()}}
})

const node:NodeOption={profile_id:'profile',node_key:'node',node_identity_key:'identity',config_revision_key:'revision',profile_name:'Airport',display_name:'Node',type:'ss',country_code:'',country_flag:''}
const time='2026-10-10T12:00:00Z'
function attempt(id:string,outcome:string,options:{execution_state?:string;http_status?:number}={}):Attempt{
  return {...node,attempt_id:id,request_id:'request-'+id,service_id:'service-a',requested_at:time,finished_at:time,execution_state:options.execution_state||'completed',persistence_state:'saved',rule:{service_id:'service-a',name:'Service A',target_url:'https://service.example'},result:{outcome,http_status:options.http_status,bytes_read:0,finished_at:time}}
}

let wrapper:VueWrapper|undefined,w:Workspace
beforeEach(()=>{
  vi.resetAllMocks()
  HTMLDialogElement.prototype.showModal=vi.fn()
  HTMLDialogElement.prototype.close=vi.fn()
  vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
  vi.mocked(api.revisions).mockResolvedValue([])
  w=createWorkspace();w.project.value='service';w.serviceIds.value=['service-a']
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML='';vi.unstubAllGlobals()})

it('uses confirmed service availability for each manual-history trend point',async()=>{
  const attempts=[
    attempt('pass','matched'),
    attempt('business-reject','region_blocked'),
    attempt('rate-limited','http_rejected',{http_status:429}),
    attempt('profiled','profiled'),
    attempt('transport-error','transport_error'),
    attempt('not-executed','unknown',{execution_state:'not_executed'}),
  ]
  vi.mocked(api.attemptHistory).mockResolvedValue({attempts,has_more:false,complete:true})
  wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
  await flushPromises()

  expect(attempts.map(a=>serviceAvailability(a))).toEqual([1,0,null,null,null,null])
  const points=wrapper.findComponent(TrendChart).props('points')
  const byId=Object.fromEntries(points.map(point=>[point.id,point]))
  expect(Object.fromEntries(Object.entries(byId).map(([id,point])=>[id,point.value]))).toEqual({
    pass:1,'business-reject':0,'rate-limited':null,profiled:null,'transport-error':null,'not-executed':null,
  })
  expect(byId['business-reject'].tone).toBe('bad')
  expect(byId['rate-limited'].tone).toBe('warn')
  expect(byId['transport-error'].tone).toBe('warn')
  expect(byId['not-executed'].tone).toBe('empty')
})

it('shows the previous service result and reports a comparison read failure',async()=>{
  vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[attempt('current','matched')],has_more:false,complete:true})
  vi.mocked(api.previousAttempt).mockResolvedValue(attempt('old','region_blocked'))
  wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
  const detail=()=>Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(b=>b.textContent==='详情')!
  detail().click();await flushPromises();await flushPromises();expect(document.querySelector('[aria-label="和上次比"]')?.textContent).toContain('→ 探针规则通过')
  detail().click();await flushPromises();vi.mocked(api.previousAttempt).mockRejectedValue(new Error('history unavailable'))
  detail().click();await flushPromises();await flushPromises();expect(document.querySelector('[aria-label="和上次比"]')?.textContent).toContain('对比读取失败：history unavailable')
})

it('shows the download speed change for the same conditions',async()=>{
  w.project.value='download'
  const make=(id:string,size:number)=>({...attempt(id,'byte_limit'),trigger_type:'manual',round_id:'recorded-round',node_type:'ss',rule:{target_url:'https://download.example',method:'GET',rule_version:1,maximum_bytes:10485760,maximum_duration_ns:10000000000,network_path_method:'physical_socket_v1',physical_interface:'en0',dns_mode:'physical'},result:{outcome:'byte_limit',bytes_read:size,duration_ns:1000000000,phase_timings_ns:{response_body:1000000000},finished_at:time,network_path:{method:'physical_socket_v1',interface:'en0',dns_mode:'physical',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:'ipv4',address_source:'dns'}}})
  vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[make('now',2097152)],has_more:false,complete:true});vi.mocked(api.previousAttempt).mockResolvedValue(make('old',1048576))
  wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
  Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(b=>b.textContent==='详情')!.click();await flushPromises()
  expect(document.querySelector('[aria-label="和上次比"]')?.textContent).toContain('增加 1.00 MiB/s')
  expect(document.querySelector('.selected-record-meta')?.textContent).toContain('手动 · 轮次 recorded-round')
  expect(document.querySelector('.measurement-detail')?.textContent).not.toContain('来源未知')
})

it('opens the requested service evidence without changing the workbench service selection',async()=>{
 const evidence={...attempt('recommended','matched'),service_id:'service-b',rule:{service_id:'service-b',name:'Service B',target_url:'https://example.invalid'}}
 vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[evidence],has_more:false,complete:true})
 wrapper=mount(NodeDetailModal,{props:{node,initialServiceId:'service-b'},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 await flushPromises()
 expect(api.attemptHistory).toHaveBeenLastCalledWith('service',node,expect.objectContaining({service_id:'service-b'}))
 expect(w.serviceIds.value).toEqual(['service-a'])
 expect(wrapper.findComponent(TrendChart).props('points').map(point=>point.id)).toEqual(['recommended'])
})

it('opens the recommended Cloudflare evidence while preserving the workbench display target',async()=>{
 w.project.value='latency';w.displayTarget.value='google'
 const test:LatencyTest={...node,attempt_id:'recommended-latency',display_name:'Node',node_type:'ss',requested_at:time,finished_at:time,status:'completed',latency_ms:50,total_samples:2,success_samples:2,failure_samples:0,persistence_state:'saved',samples:[{seq:0,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:time,latency_ms:50,success:true},{seq:1,target:'https://www.gstatic.com/generate_204',timestamp:time,latency_ms:400,success:true}]}
 vi.mocked(api.latencyHistory).mockResolvedValue({tests:[test],has_more:false,complete:true})
 wrapper=mount(NodeDetailModal,{props:{node,initialTargetId:'cloudflare'},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 await flushPromises()
 expect(wrapper.findComponent(TrendChart).props('points')[0].value).toBe(50)
 expect(w.displayTarget.value).toBe('google')
})

it('labels an archived revision truthfully and uses only the live configuration for a new plan',async()=>{
 const old={...node,node_key:'older-key',config_revision_key:'older-revision'}
 w.nodes.value=[node]
 vi.mocked(api.revisions).mockResolvedValue([{...old,last_observed_at:time},{...node,last_observed_at:time}])
 vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[{...attempt('old','matched'),...old}],has_more:false,complete:true})
 wrapper=mount(NodeDetailModal,{props:{node:old},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 expect(document.querySelector('[role="status"]')?.textContent).toContain('正在看历史配置')
 const selected=Array.from(document.querySelectorAll<HTMLSelectElement>('select')).find(select=>select.value==='older-revision')!
 expect(selected.selectedOptions[0].textContent).toContain('历史配置')
 const run=Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(button=>button.textContent?.includes('检测当前配置'))!
 run.click();await flushPromises()
 expect(w.plan.value?.nodes).toEqual([node])
})
it('retains archive reading but cannot start detection for a configuration absent from current nodes',async()=>{
 vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[attempt('archive','matched')],has_more:false,complete:true})
 wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 expect(document.querySelector('[role="status"]')?.textContent).toContain('此配置已不在当前节点清单')
 const run=Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(button=>button.textContent?.includes('检测当前配置'))!
 expect(run.disabled).toBe(true)
 expect(wrapper.findComponent(TrendChart).props('points')).toHaveLength(1)
})

it.each(['manual','monitor'] as const)('opens the report time window and explicit %s source without changing workbench settings',async source=>{
 const until='2026-10-01T08:01:01.000Z'
 w.page.value=source==='manual'?'monitor':'history'
 vi.mocked(api.latencyHistory).mockResolvedValue({tests:[],has_more:false,complete:true})
 vi.mocked(api.monitorSamples).mockResolvedValue({items:[],has_more:false,next_cursor:''})
 wrapper=mount(NodeDetailModal,{props:{node,initialProject:'latency',initialSource:source,initialTargetId:'google',initialUntil:until},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 if(source==='manual'){expect(api.latencyHistory).toHaveBeenCalledWith(expect.objectContaining({profile_id:node.profile_id,config_revision_key:node.config_revision_key}),expect.objectContaining({until}));expect(api.monitorSamples).not.toHaveBeenCalled()}
 else{expect(api.monitorSamples).toHaveBeenCalledWith(expect.objectContaining({profile_id:node.profile_id,config_revision_key:node.config_revision_key,probe_type:'rtt',until}));expect(api.latencyHistory).not.toHaveBeenCalled()}
 expect(document.body.textContent).toContain('指定时段');expect(w.project.value).toBe('service');expect(w.serviceIds.value).toEqual(['service-a'])
 Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find(b=>b.textContent?.trim()==='刷新')!.click();await flushPromises()
 if(source==='manual')expect(api.latencyHistory).toHaveBeenLastCalledWith(expect.any(Object),expect.objectContaining({until}))
 else expect(api.monitorSamples).toHaveBeenLastCalledWith(expect.objectContaining({until}))
})


it('honors an explicit all-services history scope without inheriting the workbench service filter',async()=>{
 vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[],has_more:false,complete:true})
 wrapper=mount(NodeDetailModal,{props:{node,initialProject:'service',initialServiceId:'',initialSource:'manual',initialUntil:time},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 const query=vi.mocked(api.attemptHistory).mock.calls[0]
 expect(query[0]).toBe('service');expect(query[1]).toEqual(expect.objectContaining(node));expect(query[2]).toEqual(expect.objectContaining({until:time}));expect(query[2]).not.toHaveProperty('service_id')
 expect((document.querySelector('select[aria-label="历史服务项目"]') as HTMLSelectElement).value).toBe('');expect(w.serviceIds.value).toEqual(['service-a'])
})
it('offers the only saved older revision when the current configuration has no saved history',async()=>{
 const older={...node,node_key:'older-key',config_revision_key:'older-only',last_observed_at:time}
 w.nodes.value=[node]
 vi.mocked(api.revisions).mockResolvedValue([older])
 vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[],has_more:false,complete:true})
 wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 const select=Array.from(document.querySelectorAll<HTMLSelectElement>('dialog select')).find(select=>Array.from(select.options).some(option=>option.value==='older-only'))
 expect(select).toBeDefined()
 select!.value='older-only';select!.dispatchEvent(new Event('change',{bubbles:true}));await flushPromises()
 expect(api.attemptHistory).toHaveBeenLastCalledWith('service',expect.objectContaining({profile_id:node.profile_id,node_key:'older-key',node_identity_key:node.node_identity_key,config_revision_key:'older-only'}),expect.any(Object))
 expect(document.body.textContent).toContain('正在看历史配置')
})

it('reports revision discovery failure and can retry without discarding available records',async()=>{
 vi.mocked(api.revisions).mockRejectedValueOnce(new Error('revision list unavailable')).mockResolvedValueOnce([{...node,node_key:'older-key',config_revision_key:'older-only',last_observed_at:time}])
 vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[attempt('current','matched')],has_more:false,complete:true})
 wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 expect(document.body.textContent).toContain('revision list unavailable')
 expect(wrapper.findComponent(TrendChart).props('points')).toHaveLength(1)
 Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find(b=>b.textContent?.trim()==='重试读取配置')!.click();await flushPromises()
 expect(document.body.textContent).not.toContain('revision list unavailable')
 expect(Array.from(document.querySelectorAll<HTMLOptionElement>('dialog option')).some(option=>option.value==='older-only')).toBe(true)
 expect(api.attemptHistory).toHaveBeenCalledTimes(1)
})


it('keeps a removed service label selectable and does not turn a history read failure into an empty history',async()=>{
 vi.mocked(api.attemptHistory).mockRejectedValueOnce(new Error('saved records unavailable')).mockResolvedValueOnce({attempts:[attempt('old-service','matched')],has_more:false,complete:true})
 wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 expect(document.body.textContent).toContain('saved records unavailable')
 expect(document.body.textContent).not.toContain('这个范围还没有检测记录')
 expect(document.body.textContent).not.toContain('当前范围没有记录')
 const select=document.querySelector<HTMLSelectElement>('select[aria-label="历史服务项目"]')!
 expect(select.selectedOptions[0].textContent).toContain('service-a（历史项目）')
 Array.from(document.querySelectorAll<HTMLButtonElement>('dialog button')).find(b=>b.textContent?.trim()==='刷新')!.click();await flushPromises()
 expect(select.selectedOptions[0].textContent).toContain('Service A（历史项目）')
 expect(wrapper.findComponent(TrendChart).props('points')).toHaveLength(1)
})
