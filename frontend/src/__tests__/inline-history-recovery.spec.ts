import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import InlineHistory from '../components/InlineHistory.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import {key,type Attempt,type HistoryPage,type LatencyTest,type MonitorSample,type NodeOption} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,api:{...actual.api,latencySummaries:vi.fn(),monitorSamples:vi.fn(),attemptHistory:vi.fn()}}})

const node:NodeOption={profile_id:'p',node_key:'n',node_identity_key:'identity',config_revision_key:'revision',profile_name:'Airport',display_name:'Node',type:'ss',country_code:'SG',country_flag:''}
let wrapper:VueWrapper|undefined
beforeEach(()=>{vi.resetAllMocks();window.sessionStorage.clear()})
afterEach(()=>{wrapper?.unmount();wrapper=undefined})
function deferred<T>(){let resolve!:(value:T)=>void,reject!:(reason:unknown)=>void;const promise=new Promise<T>((yes,no)=>{resolve=yes;reject=no});return {promise,resolve,reject}}
function savedAttempt(n:NodeOption,kind:'download'|'service',id='saved-attempt'):Attempt{return {...n,attempt_id:id,request_id:id,service_id:kind==='service'?'service-a':undefined,display_name:n.display_name,requested_at:'2026-10-02T00:00:00Z',finished_at:'2026-10-02T00:00:01Z',execution_state:'completed',persistence_state:'saved',rule:{name:kind==='service'?'Service A':undefined,target_url:'https://history.example.test'},result:{outcome:kind==='download'?'byte_limit':'unlocked',bytes_read:kind==='download'?2097152:0,duration_ns:1000000000,finished_at:'2026-10-02T00:00:01Z'}}}
function selectServices(w:ReturnType<typeof createWorkspace>){w.catalog.value=[{service_id:'service-a',name:'Service A',category:'test',description:'',result_kind:'unlock',success_criterion:'',timeout_seconds:15}];w.serviceIds.value=['service-a']}

describe('inline history read recovery',()=>{
  it('distinguishes a failed history read from no records and retries without changing selection',async()=>{
    vi.mocked(api.latencySummaries).mockResolvedValueOnce([{tests:[],has_more:false,complete:false}])
    const w=createWorkspace();w.selectedKeys.value=[key(node)]
    wrapper=mount(InlineHistory,{props:{node,nodes:[node],project:'latency'},global:{provide:{[workspaceKey as symbol]:w}}})
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.find('[role="alert"]').text()).toContain('历史读取失败')
    expect(wrapper.text()).not.toContain('当前范围暂无已读取的检测记录')

    const test:LatencyTest={...node,attempt_id:'saved-round',display_name:node.display_name,node_type:'ss',requested_at:'2026-10-02T00:00:00Z',finished_at:'2026-10-02T00:00:01Z',status:'completed',latency_ms:75,total_samples:2,success_samples:2,failure_samples:0,persistence_state:'saved',samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-02T00:00:01Z',latency_ms:75,success:true},{seq:2,target:'https://www.gstatic.com/generate_204',timestamp:'2026-10-02T00:00:01Z',latency_ms:125,success:true}]}
    vi.mocked(api.latencySummaries).mockResolvedValueOnce([{tests:[test],has_more:false,complete:true}])
    await wrapper.find('button[aria-label="重新读取历史"]').trigger('click');await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('.overview-series circle').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('当前范围暂无已读取的检测记录')
    await wrapper.setProps({targetIds:['cloudflare','google','apple']})
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(2)
    const legends=wrapper.findAll('.overview-legend label')
    expect(legends.find(label=>label.text().includes('Cloudflare'))?.text()).toContain('75 ms')
    expect(legends.find(label=>label.text().includes('Google'))?.text()).toContain('125 ms')
    expect(legends.find(label=>label.text().includes('Apple'))?.text()).toContain('未测')
    await wrapper.setProps({targetIds:['google']})
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    expect(wrapper.find('.inline-history-tools').text()).toContain('Google')
    expect(w.selectedKeys.value).toEqual([key(node)])
    w.dispose()
  })
  it('keeps an available monitoring curve while a comparison read fails and recovers in place',async()=>{
    const w=createWorkspace(),other={...node,node_key:'n-b',node_identity_key:'identity-b',display_name:'Node B'}
    wrapper=mount(InlineHistory,{props:{node,nodes:[node,other],project:'latency',source:'monitor',anchorPoints:[{id:'anchor',time:'2026-10-02T00:00:00Z',value:70,tone:'good',description:'70 ms',source:'monitor'}]},global:{provide:{[workspaceKey as symbol]:w}}})
    await flushPromises()
    vi.mocked(api.monitorSamples).mockRejectedValueOnce(new Error('monitor history unavailable'))
    await wrapper.findAll('.inline-history-tools .view-switch button')[2].trigger('click');await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('monitor history unavailable')
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)

    const sample:MonitorSample={...other,sample_id:'monitor-b',run_id:'run-b',display_name_snapshot:other.display_name,timestamp:'2026-10-02T00:01:00Z',target:'https://cp.cloudflare.com/generate_204',probe_type:'rtt',success:true,latency:90000000,ttfb:0,sampling_tier:'regular'}
    vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[sample],has_more:false,next_cursor:''})
    await wrapper.find('button[aria-label="重新读取历史"]').trigger('click');await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(2)
    expect(w.selectedKeys.value).toEqual([])
    w.dispose()
  })
  it.each(['download','service'] as const)('recovers %s history from a failed read and disables retry while pending',async project=>{
    const w=createWorkspace();selectServices(w);w.selectedKeys.value=[key(node)]
    vi.mocked(api.attemptHistory).mockRejectedValueOnce(new Error(project+' history unavailable'))
    wrapper=mount(InlineHistory,{props:{node,nodes:[node],project},global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain(project+' history unavailable')
    expect(wrapper.find('.overview-svg').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('当前范围暂无已读取的检测记录')

    const retry=deferred<HistoryPage<Attempt>>();vi.mocked(api.attemptHistory).mockReturnValueOnce(retry.promise)
    await wrapper.find('button[aria-label="重新读取历史"]').trigger('click')
    expect(wrapper.find('button[aria-label="重新读取历史"]').attributes('disabled')).toBeDefined()
    retry.resolve({attempts:[savedAttempt(node,project)],has_more:false,complete:true});await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    expect(wrapper.find('.overview-svg text').text()).toBe(project==='download'?'MiB/s':'状态')
    if(project==='service'){expect(wrapper.find('.inline-service-row').text()).toContain('已解锁');expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(1)}
    expect(w.selectedKeys.value).toEqual([key(node)]);expect(w.serviceIds.value).toEqual(['service-a'])
    w.dispose()
  })
  it('does not show a late download failure over a successfully loaded service project',async()=>{
    const w=createWorkspace();selectServices(w);const oldRead=deferred<HistoryPage<Attempt>>()
    vi.mocked(api.attemptHistory).mockImplementation(kind=>kind==='download'?oldRead.promise:Promise.resolve({attempts:[savedAttempt(node,'service')],has_more:false,complete:true}))
    wrapper=mount(InlineHistory,{props:{node,nodes:[node],project:'download'},global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
    await wrapper.setProps({project:'service'});await flushPromises()
    oldRead.reject(new Error('late download failure'));await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('.overview-svg text').text()).toBe('状态')
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    expect(wrapper.find('.inline-service-row').text()).toContain('已解锁')
    w.dispose()
  })
  it('does not replace the current node curve when an earlier node read arrives late',async()=>{
    const w=createWorkspace(),other={...node,node_key:'n-b',node_identity_key:'identity-b',display_name:'Node B'},oldRead=deferred<HistoryPage<Attempt>>()
    vi.mocked(api.attemptHistory).mockImplementation((_kind,n)=>key(n)===key(node)?oldRead.promise:Promise.resolve({attempts:[savedAttempt(other,'download','other-round')],has_more:false,complete:true}))
    wrapper=mount(InlineHistory,{props:{node,nodes:[node,other],project:'download'},global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
    await wrapper.setProps({node:other});await flushPromises()
    oldRead.resolve({attempts:[savedAttempt(node,'download','old-round')],has_more:false,complete:true});await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('h3').text()).toContain('Node B')
    expect(wrapper.findAll('.overview-series').map(s=>s.attributes('aria-label'))).toEqual(['新加坡 · Node B · Airport'])
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    w.dispose()
  })
  it('clears an old service hover when switching services while its history request is pending',async()=>{
    const w=createWorkspace();selectServices(w);w.catalog.value.push({...w.catalog.value[0],service_id:'service-b',name:'Service B'},{...w.catalog.value[0],service_id:'service-c',name:'Service C'})
    const first=savedAttempt(node,'service','service-round-a'),second={...savedAttempt(node,'service','service-round-b'),service_id:'service-b',rule:{name:'Service B',target_url:'https://history.example.test'},result:{outcome:'transport_error',bytes_read:0,finished_at:'2026-10-02T00:00:02Z',error_message:'connection refused'}}
    w.services.value[key(node)]=[first,second]
    const history=deferred<HistoryPage<Attempt>>();vi.mocked(api.attemptHistory).mockReturnValueOnce(history.promise)
    wrapper=mount(InlineHistory,{props:{node,nodes:[node],project:'service',serviceIds:['service-a','service-b','service-c']},global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises()
    const legends=wrapper.findAll('.overview-legend label')
    expect(legends.find(label=>label.text().includes('Service A'))?.text()).toContain('通过')
    expect(legends.find(label=>label.text().includes('Service B'))?.text()).toContain('失败')
    expect(legends.find(label=>label.text().includes('Service C'))?.text()).toContain('未测')
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    expect(wrapper.findAll('.overview-failure')).toHaveLength(1)
    const emptyRow=wrapper.findAll('.inline-service-row').find(row=>row.text().includes('Service C'))!
    await emptyRow.find('.health-cell').trigger('click')
    expect(wrapper.find('.measurement-results').exists()).toBe(false)
    const svg=wrapper.find('.overview-svg').element as SVGElement
    svg.getBoundingClientRect=()=>({left:0,top:0,right:960,bottom:300,width:960,height:300,x:0,y:0,toJSON(){return {}}})
    const move=new Event('pointermove',{bubbles:true});Object.assign(move,{clientX:495,clientY:35});svg.dispatchEvent(move);await flushPromises()
    expect(wrapper.find('.overview-readout').text()).toContain('已解锁')

    await wrapper.find('.inline-history-tools select').setValue('service-b')
    history.resolve({attempts:[first,second],has_more:false,complete:true});await flushPromises()
    expect(wrapper.findAll('.overview-failure')).toHaveLength(1)
    expect(wrapper.find('.overview-readout').text()).not.toContain('已解锁')
    expect(wrapper.find('.overview-guide').exists()).toBe(false)
    expect(w.serviceIds.value).toEqual(['service-a'])
    w.dispose()
  })
  it.each(['range','probe'] as const)('does not present a cached monitoring comparison under a changed %s when the new read fails',async change=>{
    const w=createWorkspace(),other={...node,node_key:'n-b',node_identity_key:'identity-b',display_name:'Node B'}
    const sample:MonitorSample={...other,sample_id:'old-http',run_id:'old-round',display_name_snapshot:other.display_name,timestamp:new Date().toISOString(),target:'https://www.google.com/generate_204',probe_type:'service_google',success:true,latency:90000000,ttfb:0,sampling_tier:'regular'}
    vi.mocked(api.monitorSamples).mockResolvedValueOnce({items:[sample],has_more:false,next_cursor:''})
    wrapper=mount(InlineHistory,{props:{node,nodes:[node,other],project:'service',source:'monitor',monitorServiceId:'monitor_google_http',anchorPoints:[]},global:{provide:{[workspaceKey as symbol]:w}}})
    await flushPromises();await wrapper.findAll('.inline-history-tools .view-switch button')[2].trigger('click');await flushPromises()
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    vi.mocked(api.monitorSamples).mockRejectedValueOnce(new Error('new context unavailable'))
    if(change==='range')w.hours.value=6
    else await wrapper.setProps({monitorServiceId:'monitor_github_http'})
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('new context unavailable')
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(0)
    expect(wrapper.find('.overview-svg').exists()).toBe(false)
    w.dispose()
  })

})
