import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {DOMWrapper,flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import {key,type MonitorSample,type NodeOption,type Plan} from '../domain'

vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),clear:()=>items.clear()}})})
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,api:{...actual.api,refreshJobs:vi.fn(async()=>[]),measurementRounds:vi.fn(async()=>[]),createRound:vi.fn(async()=>undefined),finishRound:vi.fn(async()=>undefined),latencySummaries:vi.fn(async(nodes:NodeOption[])=>nodes.map(()=>({tests:[],complete:true,has_more:false}))),attemptHistory:vi.fn(async()=>({attempts:[],complete:true,has_more:false})),monitorSamples:vi.fn(async()=>({items:[],has_more:false,next_cursor:''})),startLatency:vi.fn(),startAttempt:vi.fn(),createJob:vi.fn()}}})

const a:NodeOption={profile_id:'sub-a-one',node_key:'shared-key',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A / One',display_name:'Node A One',type:'trojan',country_code:'HK',country_flag:''}
const aTwo:NodeOption={...a,profile_id:'sub-a-two',node_key:'node-two',node_identity_key:'identity-a-two',config_revision_key:'revision-a-two',profile_name:'Airport A / Two',display_name:'Node A Two',country_code:'JP'}
const b:NodeOption={...a,profile_id:'sub-b',node_identity_key:'identity-b',config_revision_key:'revision-b',profile_name:'Airport B',display_name:'Node B',country_code:'US'}
let wrapper:VueWrapper|undefined,w:ReturnType<typeof createWorkspace>|undefined

function attach(nodes:NodeOption[]=[a,aTwo,b],selected:string[]=[]){
  w=createWorkspace();w.airports.value=[{id:'airport-a',name:'Airport A',node_count:2,has_cache:true,url_display:'',subscriptions:['sub-a-one','sub-a-two'].map((id,i)=>({id,airport_id:'airport-a',name:i?'Two':'One',url_display:'',url_configured:true,node_count:1,has_cache:true}))},{id:'airport-b',name:'Airport B',node_count:1,has_cache:true,url_display:'',subscriptions:[{id:'sub-b',airport_id:'airport-b',name:'Default',url_display:'',url_configured:true,node_count:1,has_cache:true}]}]
  w.nodes.value=nodes;w.selectedAirportIds.value=selected;w.catalog.value=[{service_id:'netflix_unlock',name:'Netflix',category:'Video',description:'',result_kind:'unlock',success_criterion:'',timeout_seconds:15}];w.serviceIds.value=['netflix_unlock']
  wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});return {w,wrapper}
}
async function button(container:DOMWrapper<Element>|VueWrapper,selector:string,label:string){const found=container.findAll(selector).find(b=>b.text()===label);expect(found,`button ${label}`).toBeDefined();await found!.trigger('click')}
function dialog(){return new DOMWrapper(document.querySelector('dialog')!)}
function airportInput(name:string){const label=Array.from(document.querySelectorAll('.airport-pick-list label')).find(e=>e.querySelector('strong')?.textContent===name)!;return new DOMWrapper(label.querySelector('input')!)}
function nodeInput(view:VueWrapper,name:string){return view.get(`.node-check input[aria-label="选择 ${name}"]`)}

beforeEach(()=>{window.localStorage.clear();window.localStorage.setItem('speedtest-home-view','detailed');window.sessionStorage.clear();vi.clearAllMocks();HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}});vi.stubGlobal('fetch',vi.fn().mockRejectedValue(new Error('Unexpected network request in isolated DOM selection test')))})
afterEach(()=>{expect(fetch).not.toHaveBeenCalled();wrapper?.unmount();w?.dispose();wrapper=undefined;w=undefined;document.body.innerHTML='';vi.unstubAllGlobals()})

describe('node selection through DOM controls',()=>{
  it('retains stopped monitor history in metrics, bars and node trends without mixing sites or revisions',async()=>{
    const sample:MonitorSample={...a,sample_id:'monitor-ok',run_id:'run-ok',display_name_snapshot:a.display_name,timestamp:'2026-10-03T06:30:00Z',probe_type:'rtt',target:'https://cp.cloudflare.com/generate_204',success:true,latency:80000000,ttfb:0,sampling_tier:'regular'}
    vi.mocked(api.monitorSamples).mockImplementation(async q=>({items:q.node_identity_key==='identity-a'&&q.config_revision_key==='revision-a'?[{...sample,sample_id:'monitor-fail',run_id:'run-fail',timestamp:'2026-10-03T06:35:00Z',success:false,error_class:'timeout'},sample]:[],has_more:false,next_cursor:''}))
    const {w,wrapper}=attach([a],['airport-a']);w.displayTargets.value=['cloudflare','google'];await flushPromises()
    expect(w.jobs.value).toEqual([])
    expect(api.monitorSamples).toHaveBeenCalledWith(expect.objectContaining({profile_id:'sub-a-one',node_identity_key:'identity-a',config_revision_key:'revision-a',probe_type:'rtt',target:undefined,limit:480}))
    const metrics=wrapper.findAll('.node-target-metrics strong')
    expect(metrics.map(m=>m.text())).toEqual(['失败','未测'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(2)
    expect(wrapper.text()).toContain('含持续监测 · 各站独立记录')
    await wrapper.get('.node-name').trigger('click');await flushPromises()
    expect(wrapper.get('.inline-full-history').text()).toContain('持续监测 · Cloudflare')
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    w.latency.value={[key(a)]:[{...a,attempt_id:'new-manual',node_type:'trojan',requested_at:'2026-10-03T07:00:00Z',finished_at:'2026-10-03T07:00:01Z',status:'completed',latency_ms:55,total_samples:1,success_samples:1,failure_samples:0,persistence_state:'saved',samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-03T07:00:01Z',latency_ms:55,success:true}]}]}
    await flushPromises();expect(wrapper.findAll('.node-target-metrics strong').map(m=>m.text())).toEqual(['55 ms','未测'])
    w.nodes.value=[{...a,config_revision_key:'revision-new'}];await flushPromises()
    expect(api.monitorSamples).toHaveBeenLastCalledWith(expect.objectContaining({config_revision_key:'revision-new'}))
    expect(wrapper.findAll('.node-target-metrics strong').map(m=>m.text())).toEqual(['未测','未测'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(0)
  })
  it('shows stopped HTTP monitoring in service and combined views without claiming catalog checks passed',async()=>{
    const google:MonitorSample={...a,sample_id:'http-google-1',run_id:'http-run-1',display_name_snapshot:a.display_name,timestamp:'2026-10-03T06:30:00Z',probe_type:'service_google',target:'https://www.google.com/generate_204',success:true,latency:80000000,ttfb:0,sampling_tier:'regular'}
    const github:MonitorSample={...google,sample_id:'http-github-1',probe_type:'service_github',target:'https://api.github.com'}
    vi.mocked(api.monitorSamples).mockImplementation(async q=>({items:q.config_revision_key!=='revision-a'?[]:q.probe_type==='service_google'?[{...google,sample_id:'http-google-2',run_id:'http-run-2',timestamp:'2026-10-03T06:35:00Z',success:false,error_class:'timeout',error_detail:'network timeout'},google]:q.probe_type==='service_github'?[github]:[],has_more:false,next_cursor:''}))
    const {w,wrapper}=attach([a],['airport-a']);w.catalog.value=[{service_id:'google_204',name:'Google 204 连通性',category:'基础网络',description:'',result_kind:'connectivity',success_criterion:'exact 204',timeout_seconds:10}];w.displayServiceIds.value=['google_204'];w.project.value='service';await flushPromises()
    expect(w.jobs.value).toEqual([])
    expect(wrapper.findAll('.monitor-http-metrics strong').map(m=>m.text())).toEqual(['失败','HTTP 可达'])
    expect(wrapper.findAll('.node-target-metrics strong').map(m=>m.text())).toEqual(['未测','失败','HTTP 可达'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(3)
    const googleBars=wrapper.get('.health-bars[aria-label="Google 基础 HTTP历史状态，悬停查看本轮数据"]')
    await googleBars.findAll('.has-record').at(-1)!.trigger('click');await flushPromises()
    expect(wrapper.get('.measurement-results').text()).toContain('network timeout')
    expect(wrapper.get('.measurement-results').text()).toContain('响应 2xx／3xx')
    expect(wrapper.get('.measurement-results').text()).not.toContain('没有匹配的结果')
    await button(wrapper,'.inline-panel-header button','完整走势图');await flushPromises()
    expect(wrapper.get('.inline-full-history').text()).toContain('持续监测 · Google 基础 HTTP')
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    expect(wrapper.find('select[aria-label="走势对比服务"]').exists()).toBe(false)
    await button(wrapper,'.inline-panel-header button','收起');w.project.value='combined';await flushPromises()
    expect(wrapper.findAll('.monitor-http-metrics strong').map(m=>m.text())).toEqual(['失败','HTTP 可达'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(3)
    vi.mocked(api.monitorSamples).mockRejectedValue(new Error('HTTP history unavailable'));w.hours.value=6;await flushPromises()
    expect(wrapper.findAll('.monitor-http-metrics strong').map(m=>m.text())).toEqual(['未测','未测'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(0)
    expect(wrapper.get('[role=alert]').text()).toContain('监测 HTTP 记录读取失败')
    vi.mocked(api.monitorSamples).mockResolvedValue({items:[],has_more:false,next_cursor:''})
    w.nodes.value=[{...a,config_revision_key:'revision-new'}];await flushPromises()
    expect(wrapper.findAll('.monitor-http-metrics strong')).toHaveLength(0)
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(0)
  })
  it('uses the airport picker, subscription filter and node checkboxes as one scope for every project and monitor entry',async()=>{
    const {w,wrapper}=attach();await flushPromises()
    await wrapper.get('.orbit-button').trigger('click')
    await airportInput('Airport A').setValue(true);await airportInput('Airport B').setValue(true)
    expect(wrapper.get('.orbit-button strong').text()).toContain('2 个机场')
    await button(dialog(),'.modal-footer button','完成')
    expect(document.querySelector('dialog')).toBeNull()
    await button(wrapper,'.toolbar-filters button','多选节点')
    await nodeInput(wrapper,'日本 · Node A Two').setValue(true);await nodeInput(wrapper,'美国 · Node B').setValue(true)
    await wrapper.get('select[aria-label="订阅筛选"]').setValue('sub-a-two')
    expect(wrapper.findAll('.node-row')).toHaveLength(1)
    expect(wrapper.get('.node-name strong').text()).toBe('日本 · Node A Two')
    expect(wrapper.get('.selection-info strong').text()).toBe('已选 2 个节点')

    for(const [label,projects] of [['延迟检测',['latency']],['下载测速',['download']],['服务可用性',['service']],['综合',['latency','download','service']]] as const){
      await button(wrapper,'.project-tabs button',label)
      expect(nodeInput(wrapper,'日本 · Node A Two').element).toHaveProperty('checked',true)
      await wrapper.get('.selection-actions .primary').trigger('click')
      expect(w.plan.value?.projects).toEqual(projects)
      expect(w.plan.value?.nodes).toEqual([aTwo,b])
      w.plan.value=null
      await wrapper.get('.scope-run').trigger('click')
      expect((w.plan.value as Plan|null)?.nodes).toEqual([aTwo,b])
      expect((w.plan.value as Plan|null)?.projects).toEqual(projects)
      w.plan.value=null
      await button(wrapper,'.selection-actions button','持续监测延迟')
      expect(w.monitorNodes.value).toEqual([aTwo,b])
      w.monitorNodes.value=null
    }

    await wrapper.get('.orbit-button').trigger('click');await airportInput('Airport A').setValue(false)
    await button(dialog(),'.modal-footer button','完成')
    expect(w.subscription.value).toBe('all')
    expect(wrapper.find('select[aria-label="订阅筛选"]').exists()).toBe(false)
    expect(wrapper.findAll('.node-row')).toHaveLength(1)
    expect(wrapper.get('.selection-info strong').text()).toBe('已选 1 个节点')
    await wrapper.get('.selection-actions .primary').trigger('click');expect(w.plan.value?.nodes).toEqual([b]);w.plan.value=null
    await button(wrapper,'.selection-actions button','持续监测延迟');expect(w.monitorNodes.value).toEqual([b]);w.monitorNodes.value=null
    await nodeInput(wrapper,'美国 · Node B').setValue(false)
    expect(wrapper.get('.selection-info strong').text()).toBe('已选 0 个节点')
    expect(wrapper.get('.selection-actions .primary').attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('.selection-actions button')[1].attributes('disabled')).toBeDefined()
  })

  it('selects and cancels page or filtered ranges without removing a selected node in another subscription',async()=>{
    const many=Array.from({length:25},(_,i)=>({...a,node_key:`node-${i+1}`,node_identity_key:`identity-${i+1}`,display_name:`Node ${String(i+1).padStart(2,'0')}`}))
    const {w,wrapper}=attach([...many,b],['airport-a','airport-b']);await flushPromises()
    await button(wrapper,'.toolbar-filters button','多选节点')
    await wrapper.get('select[aria-label="订阅筛选"]').setValue('sub-b');await nodeInput(wrapper,'美国 · Node B').setValue(true)
    await wrapper.get('select[aria-label="订阅筛选"]').setValue('sub-a-one')
    await wrapper.get('input[aria-label="选择本页全部节点"]').setValue(true)
    expect(w.selectedNodes.value).toEqual([...many.slice(0,20),b])
    await wrapper.get('button[aria-label="下一页"]').trigger('click')
    expect(wrapper.findAll('.node-row')).toHaveLength(5)
    await wrapper.get('input[aria-label="选择本页全部节点"]').setValue(true)
    expect(w.selectedNodes.value).toEqual([...many,b])
    await wrapper.get('input[aria-label="选择本页全部节点"]').setValue(false)
    expect(w.selectedNodes.value).toEqual([...many.slice(0,20),b])
    await button(wrapper,'.selection-info button','全选筛选范围')
    expect(w.selectedNodes.value).toEqual([...many,b])
    await button(wrapper,'.selection-info button','取消范围选择')
    expect(w.selectedNodes.value).toEqual([b])
    expect(wrapper.get('.selection-info strong').text()).toBe('已选 1 个节点')
    await wrapper.get('select[aria-label="订阅筛选"]').setValue('sub-b')
    expect(nodeInput(wrapper,'美国 · Node B').element).toHaveProperty('checked',true)
    await button(wrapper,'.selection-info button','清空已选')
    expect(w.selectedNodes.value).toEqual([])
    expect(nodeInput(wrapper,'美国 · Node B').element).toHaveProperty('checked',false)
  })
})
