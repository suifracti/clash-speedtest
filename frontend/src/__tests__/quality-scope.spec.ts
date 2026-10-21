import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import {key,targets,type Attempt,type LatencyBatch,type LatencyTest,type NetworkPath,type NodeOption,type NodeScope} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
import s5 from './fixtures/s5-saved-quality.json'

const readQualityMock=vi.hoisted(()=>vi.fn())
const localStorageItems=vi.hoisted(()=>new Map<string,string>())
vi.mock('../nodeQuality',async importOriginal=>{const actual=await importOriginal<typeof import('../nodeQuality')>();return {...actual,readQuality:readQualityMock}})
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,api:{...actual.api,refreshJobs:vi.fn(async()=>[]),latencyHistory:vi.fn(async()=>({tests:[],has_more:false,complete:true})),measurementRounds:vi.fn(async()=>[]),createRound:vi.fn(async()=>undefined),finishRound:vi.fn(async()=>undefined),monitorSamples:vi.fn(async()=>({items:[],has_more:false,next_cursor:''})),latencySummaries:vi.fn(async()=>[]),attemptHistory:vi.fn(async()=>({attempts:[],has_more:false,complete:true})),jobs:vi.fn(async()=>[]),createJob:vi.fn(),jobAction:vi.fn(async()=>{}),startLatency:vi.fn(),batch:vi.fn(),startAttempt:vi.fn(),attempt:vi.fn(),attemptAction:vi.fn()}}})
vi.hoisted(()=>Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>localStorageItems.get(k)||null,setItem:(k:string,v:string)=>localStorageItems.set(k,v),removeItem:(k:string)=>localStorageItems.delete(k),clear:()=>localStorageItems.clear()}}))

const stale:NodeOption={profile_id:'sub-a',node_key:'stale',node_identity_key:'stale-identity',config_revision_key:'stale-revision',profile_name:'机场 A',display_name:'旧范围节点',type:'trojan',country_code:'HK',country_flag:''}
const fresh=Array.from({length:21},(_,i):NodeOption=>({profile_id:'sub-b',node_key:`b-${i+1}`,node_identity_key:`b-identity-${i+1}`,config_revision_key:`b-revision-${i+1}`,profile_name:'机场 B',display_name:`香港 ${String(i+1).padStart(2,'0')}`,type:'trojan',country_code:'HK',country_flag:''}))
const at='2026-10-10T00:00:00Z',target='https://speed.cloudflare.com/__down?bytes=1'
let wrapper:VueWrapper|undefined,w:ReturnType<typeof createWorkspace>|undefined

function evidence(node:NodeOption,value:number,complete=true):QualityEvidence{
 const path:NetworkPath={method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:'IPv4',address_source:'A'}
 const test:LatencyTest={...node,attempt_id:`latency-${node.node_key}`,display_name:node.display_name,node_type:node.type,source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,network_path:path,target,requested_at:at,finished_at:at,status:'completed',latency_ms:value,total_samples:5,success_samples:5,failure_samples:0,persistence_state:'saved',samples:Array.from({length:5},(_,i)=>({seq:i+1,target,timestamp:at,latency_ms:value+i,success:true}))}
 const reads={latency:{complete},monitor:{complete:true},download:{complete:true},service:{complete:true}}
 return {tests:[test],monitor:[],downloads:[],services:[],reads,complete:Object.values(reads).every(read=>read.complete),since:'2026-10-09T00:00:00Z',until:at}
}

beforeEach(()=>{localStorageItems.clear();localStorageItems.set('speedtest-home-view','quick');window.sessionStorage.clear();vi.clearAllMocks();HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})})
afterEach(()=>{wrapper?.unmount();w?.dispose();wrapper=undefined;w=undefined;document.body.innerHTML='';vi.unstubAllGlobals();vi.useRealTimers()})

function attachSavedSnapshot(){
 vi.useFakeTimers({toFake:['Date']});vi.setSystemTime(new Date('2026-10-10T00:00:00Z'))
 w=createWorkspace();w.project.value='latency';w.airports.value=[{id:'airport-b',name:'机场 B',url_display:'',node_count:1,has_cache:true,subscriptions:[{id:'sub-b',airport_id:'airport-b',name:'默认',url_display:'',url_configured:true,node_count:1,has_cache:true}]}];w.nodes.value=[fresh[0]];w.setAirports(['airport-b'])
 wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
}
function latency(){return wrapper!.get('.hd-latency').text()}
function windowStats(){return wrapper!.findAll('.hd-latency .hd-site-value').map(e=>e.attributes('title')||'').filter(text=>text.includes('次响应')||text.includes('读取失败')).join(' ')}
function emptyEvidence(node:NodeOption){const e=evidence(node,40);e.tests=[];return e}

it('refreshes the completed saved round without a manual click, but not while the frozen plan is running',async()=>{
 let saved=false
 readQualityMock.mockImplementation(async(node:NodeOption,_since:string,until:string)=>saved&&Date.parse(until)>=Date.parse('2026-10-10T00:01:00Z')?evidence(node,980):emptyEvidence(node))
 attachSavedSnapshot();await flushPromises();expect(latency()).toContain('暂无采样');expect(windowStats()).toBe('')
 const snapshot={fixture:{services:[]}};w!.sortSnapshots.value=snapshot
 w!.running.value=true;vi.setSystemTime(new Date('2026-10-10T00:01:00Z'));saved=true
 const test={...evidence(fresh[0],980).tests[0],finished_at:'2026-10-10T00:01:00Z'}
 w!.mergeLatency(test);await flushPromises()
 expect(windowStats()).not.toContain('983 ms')
 w!.running.value=false;await flushPromises()
 expect(windowStats()).toContain('983 ms')
 expect(windowStats()).not.toContain('P95')
 expect(w!.sortSnapshots.value).toEqual(snapshot)
 expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
})

it('refreshes after real runPlan polling and raw measurement-round readback before busy clears',async()=>{
 const {readQuality}=await vi.importActual<typeof import('../nodeQuality')>('../nodeQuality')
 const node=fresh[0],finished='2026-10-10T00:02:19.000Z'
 // Anonymous six-site sample shape/timings from the saved native SG01 run.
 const saved:LatencyTest={...evidence(node,0).tests[0],attempt_id:'native-six-site',method_version:2,finished_at:finished,total_samples:6,success_samples:6,failure_samples:0,network_path:{...evidence(node,0).tests[0].network_path!,address_source:'physical_ipv4_dns_multi_v2'},samples:targets.map((site,i)=>({seq:i+1,target:site.urls[0],timestamp:finished,success:true,latency_ms:[5118,7557,5795,9009,8443,5232][i]}))}
 const old:LatencyTest={...evidence(node,0).tests[0],attempt_id:'old-seven-failures',status:'failed',total_samples:7,success_samples:0,failure_samples:7,samples:Array.from({length:7},(_,i)=>({seq:i+1,target,timestamp:at,success:false,latency_ms:0}))}
 let available=false,requestId=''
 readQualityMock.mockImplementation((n:NodeOption,since:string,until:string)=>readQuality(n,since,until,{
  latency:async()=>({tests:available&&Date.parse(until)>=Date.parse(finished)?[saved,old]:[old],has_more:false,complete:true}),
  monitor:async()=>({items:[],has_more:false,next_cursor:''}),attempts:async()=>({attempts:[],has_more:false,complete:true})
 }))
 const batch=(state:string):LatencyBatch=>({batch_id:'native-batch',request_id:requestId,state,requested_at:at,item_count:1,items:[{...node,item_id:'native-item',batch_id:'native-batch',display_name:node.display_name,execution_state:state==='running'?'running':'completed',persistence_state:state==='running'?'pending':'saved',result:state==='running'?undefined:saved}]})
 vi.mocked(api.startLatency).mockImplementationOnce(async(body:any)=>{requestId=body.request_id;return batch('running')})
 vi.mocked(api.batch).mockImplementationOnce(async()=>{available=true;return batch('completed')})
 // Batch Get/emit uses WorkbenchLatencyTestDTO (state inside result). The
 // separate ListMeasurementRoundNodes API returns history.LatencyTest without it.
 const {persistence_state:_,...rawResult}=saved
 vi.mocked(api.measurementRounds).mockImplementation(async()=>available?[{round_id:'native-round',trigger_type:'manual',started_at:at,state:'finished',items:[{...node,request_id:requestId,project:'latency',execution_state:'completed',persistence_state:'saved',latency:rawResult as LatencyTest}]}]:[])
 attachSavedSnapshot();await flushPromises()
 expect(windowStats()).toContain('0/7 次响应')
 const previousReads=readQualityMock.mock.calls.length
 vi.useFakeTimers({toFake:['Date','setTimeout','clearTimeout']})
 w!.plan.value={nodes:[node],projects:['latency'],target:'all',samples:1,timeout:10,concurrency:1,downloadMiB:1,downloadSeconds:1,serviceIds:[],repeats:1}
 const run=w!.runPlan();await flushPromises();expect(w!.busy.value).toBe(true)
 vi.setSystemTime(new Date(finished));await vi.advanceTimersByTimeAsync(1200);await run;await flushPromises()
 expect(w!.busy.value).toBe(false)
 expect(api.batch).toHaveBeenCalledTimes(1);expect(api.measurementRounds).toHaveBeenCalled()
 expect(windowStats()).toContain('1/1 次响应')
 expect(windowStats()).not.toContain('稳定中位') // 每站仅一次，不能把首次连接当稳定值
 expect(windowStats()).not.toContain('P95')
 expect(readQualityMock.mock.calls.length).toBeGreaterThan(previousReads)
 expect(Date.parse(readQualityMock.mock.calls.at(-1)![2])).toBeGreaterThanOrEqual(Date.parse(finished))
 expect(w!.latest(node)?.persistence_state).toBe('saved')
 expect(api.startLatency).toHaveBeenCalledTimes(1);expect(api.startAttempt).not.toHaveBeenCalled()
})

it('refreshes a later successful save without treating a failed save as saved evidence',async()=>{
 let saved=false
 readQualityMock.mockImplementation(async(node:NodeOption)=>{const e=emptyEvidence(node);if(saved)e.downloads=[attempt];return e})
 const attempt:Attempt={...fresh[0],attempt_id:'new-download',request_id:'new-download',requested_at:'2026-10-10T00:01:00Z',finished_at:'2026-10-10T00:01:01Z',execution_state:'failed',persistence_state:'failed',rule:{method:'GET',target_url:'https://example.test/'},result:{outcome:'failed',bytes_read:0,error_message:'EOF',finished_at:'2026-10-10T00:01:01Z'}}
 attachSavedSnapshot();w!.project.value='download';await flushPromises()
 w!.mergeAttempt('download',attempt);await flushPromises()
 expect(wrapper!.get('.hd-download').text()).toContain('EOF')
 vi.setSystemTime(new Date('2026-10-10T00:01:02Z'));saved=true
 w!.mergeAttempt('download',{...attempt,persistence_state:'saved'});await flushPromises()
 expect(wrapper!.get('.hd-download').text()).toContain('EOF')
 expect(wrapper!.find('.download-success').exists()).toBe(false)
 expect(api.startAttempt).not.toHaveBeenCalled()
})

it('reads the invalidated snapshot on returning home after a background save, without restarting a test',async()=>{
 let saved=false
 readQualityMock.mockImplementation(async(node:NodeOption)=>saved?evidence(node,50):emptyEvidence(node))
 attachSavedSnapshot();await flushPromises();w!.page.value='history';await flushPromises()
 vi.setSystemTime(new Date('2026-10-10T00:01:00Z'));saved=true;w!.mergeLatency(evidence(fresh[0],50).tests[0]);await flushPromises()
 w!.page.value='home';await flushPromises()
 expect(windowStats()).toContain('53 ms')
 expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
})

it('shows a refresh read error instead of reusing an old successful value after a new saved result',async()=>{
 readQualityMock.mockImplementation(async(node:NodeOption)=>evidence(node,40))
 attachSavedSnapshot();await flushPromises();expect(windowStats()).toContain('43 ms')
 readQualityMock.mockRejectedValue(new Error('history unavailable'))
 vi.setSystemTime(new Date('2026-10-10T00:01:00Z'));w!.mergeLatency(evidence(fresh[0],100).tests[0]);await flushPromises()
 expect(windowStats()).toContain('读取失败')
 expect(windowStats()).not.toContain('43 ms')
 expect(api.startLatency).not.toHaveBeenCalled()
})

it('replaces a previous service success with the newly saved failure and rejects a late old snapshot',async()=>{
 let latest=false,resolveOld!:(value:QualityEvidence)=>void
 const service:Attempt={...fresh[0],attempt_id:'old-service',request_id:'old-service',service_id:'cloudflare_204',requested_at:at,execution_state:'completed',persistence_state:'saved',rule:{service_id:'cloudflare_204',target_url:'https://cp.cloudflare.com/generate_204'},result:{outcome:'matched',bytes_read:0,finished_at:at}}
 const old=emptyEvidence(fresh[0]);old.services=[service]
 const pending=new Promise<QualityEvidence>(resolve=>resolveOld=resolve)
 readQualityMock.mockImplementation(async()=>latest?{...emptyEvidence(fresh[0]),services:[{...service,attempt_id:'new-service',requested_at:'2026-10-10T00:01:00Z',execution_state:'failed',result:{outcome:'failed',bytes_read:0,finished_at:'2026-10-10T00:01:01Z'}}]}:pending)
 attachSavedSnapshot();w!.catalog.value=[{service_id:'cloudflare_204',name:'Cloudflare 204',category:'基础网络',description:'',result_kind:'connectivity',success_criterion:'exact 204',timeout_seconds:10}];w!.displayServiceIds.value=['cloudflare_204'];w!.project.value='service';await flushPromises()
 latest=true;vi.setSystemTime(new Date('2026-10-10T00:01:02Z'))
 w!.mergeAttempt('service',{...service,attempt_id:'new-service',requested_at:'2026-10-10T00:01:00Z',execution_state:'failed',result:{outcome:'failed',bytes_read:0,finished_at:'2026-10-10T00:01:01Z'}});await flushPromises()
 expect(wrapper!.get('.hd-service-row .unlock-state').text()).toContain('无法判断')
 resolveOld(old);await flushPromises()
 expect(wrapper!.get('.hd-service-row .unlock-state').text()).toContain('无法判断')
 expect(wrapper!.get('.hd-service-row .unlock-state').text()).not.toContain('可用')
 expect(api.startAttempt).not.toHaveBeenCalled()
})

it('automatically reads both saved S5 nodes through the real projection with a new shared cutoff and unchanged sample gates',async()=>{
 const {readQuality}=await vi.importActual<typeof import('../nodeQuality')>('../nodeQuality')
 const nodes=s5.nodes as NodeOption[],tests=s5.tests as LatencyTest[],downloads=s5.download as unknown as Attempt[],services=s5.public_service as unknown as Attempt[]
 let available=false
 const inWindow=(list:Attempt[],node:NodeScope,q:any)=>available?list.filter(a=>key(a)===key(node)&&Date.parse(a.finished_at!)<=Date.parse(q.until)):[]
 readQualityMock.mockImplementation((node:NodeOption,since:string,until:string)=>readQuality(node,since,until,{
  latency:async(n,q)=>({tests:available?tests.filter(t=>key(t)===key(n)&&Date.parse(t.finished_at)<=Date.parse(String(q!.until))):[],has_more:false,complete:true}),
  monitor:async()=>({items:[],has_more:false,next_cursor:''}),
  attempts:async(kind,n,q)=>({attempts:inWindow(kind==='download'?downloads:services,n,q),has_more:false,complete:true})
 }))
 vi.useFakeTimers({toFake:['Date']});vi.setSystemTime(new Date('2026-10-12T14:54:00Z'))
 w=createWorkspace();w.project.value='combined';w.catalog.value=[...new Set(services.map(a=>a.service_id!))].map(id=>({service_id:id,name:id,category:'fixture',description:'',result_kind:'connectivity',success_criterion:'fixture',timeout_seconds:10}));w.displayServiceIds.value=w.catalog.value.map(s=>s.service_id);w.airports.value=nodes.map(n=>({id:n.profile_id,name:n.profile_name,url_display:'',node_count:1,has_cache:true,subscriptions:[{id:n.profile_id,airport_id:n.profile_id,name:'S5',url_display:'',url_configured:false,node_count:1,has_cache:true}]}));w.nodes.value=nodes;w.setAirports(nodes.map(n=>n.profile_id))
 wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
 const perNode=async()=>{const out:string[]=[];for(const row of wrapper!.findAll('.hd-tr:not(.hd-th)')){await row.trigger('click');await flushPromises();out.push(windowStats()+' '+(wrapper!.find('.hd-saved-download').exists()?wrapper!.get('.hd-saved-download').attributes('title'):''))}return out.join(' ')}
 expect(windowStats()).toBe('')
 w.running.value=true;available=true
 for(const t of tests)w.mergeLatency(t)
 for(const a of downloads)w.mergeAttempt('download',a)
 for(const a of services)w.mergeAttempt('service',a)
 await flushPromises();expect(windowStats()).not.toContain('ms')
 vi.setSystemTime(new Date('2026-10-12T14:55:11Z'));w.running.value=false;await flushPromises()
 const saved=await perNode()
 expect(saved).not.toContain('稳定中位')
 expect(saved).toContain('1/1 次响应')
 expect(saved).not.toContain('P95')
 expect(saved).toContain('1.55 MiB/s')
 expect(saved).toContain('1.91 MiB/s')
 expect(wrapper.findAll('.hd-tr:not(.hd-th)')).toHaveLength(2)
 expect(new Set(readQualityMock.mock.calls.slice(-2).map(c=>c[2]))).toEqual(new Set(['2026-10-12T14:55:11.000Z']))
 expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
})

it('loads and sorts the full new node scope, then ignores a late result from the previous scope',async()=>{
 let resolveStale!:(value:QualityEvidence)=>void
 const staleResult=new Promise<QualityEvidence>(resolve=>resolveStale=resolve)
 readQualityMock.mockImplementation((node:NodeOption)=>node.node_identity_key==='stale-identity'?staleResult:Promise.resolve(evidence(node,Number(node.node_key.slice(2))*10)))
 w=createWorkspace();w.project.value='latency';w.airports.value=[{id:'airport-a',name:'机场 A',url_display:'',node_count:1,has_cache:true,subscriptions:[{id:'sub-a',airport_id:'airport-a',name:'默认',url_display:'',url_configured:true,node_count:1,has_cache:true}]},{id:'airport-b',name:'机场 B',url_display:'',node_count:fresh.length,has_cache:true,subscriptions:[{id:'sub-b',airport_id:'airport-b',name:'默认',url_display:'',url_configured:true,node_count:fresh.length,has_cache:true}]}];w.nodes.value=[stale,...fresh];w.setAirports(['airport-a'])
 wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 await wrapper.get('select[aria-label="质量证据筛选"]').setValue('samples')
 await wrapper.get('select[aria-label="质量分组排序"]').setValue('p50')
 w.setAirports(['airport-b'])
 await vi.waitFor(()=>expect(new Set(readQualityMock.mock.calls.filter(call=>call[0].profile_id==='sub-b').map(call=>call[0].node_identity_key)).size).toBe(fresh.length))
 await flushPromises()
 expect(readQualityMock.mock.calls.filter(call=>call[0].profile_id==='sub-b').every(call=>call[2]===readQualityMock.mock.calls.find(old=>old[0].node_identity_key==='stale-identity')?.[2])).toBe(true)
 expect(wrapper.get('[role="status"]').text()).toContain(`已读固定范围 ${fresh.length} 个节点`)
 expect(wrapper.get('[role="status"]').text()).toContain(`当前符合 ${fresh.length} 个`)
 expect(wrapper.findAll('.hd-node strong').map(node=>node.text()).slice(0,3)).toEqual(['香港 · 01','香港 · 02','香港 · 03'])
 resolveStale(evidence(stale,1,false));await flushPromises()
 expect(wrapper.get('[role="status"]').text()).toContain('存在缺页/读取错误 0 个')
 expect(wrapper.get('[role="status"]').text()).toContain(`已读固定范围 ${fresh.length} 个节点`)
 expect(wrapper.findAll('.hd-node strong').map(node=>node.text()).slice(0,3)).toEqual(['香港 · 01','香港 · 02','香港 · 03'])
 expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
})
