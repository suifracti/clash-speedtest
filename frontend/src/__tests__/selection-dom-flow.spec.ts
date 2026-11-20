import {afterEach,beforeEach,describe,expect,it,onTestFinished,vi} from 'vitest'
import {DOMWrapper,flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import {key,type LatencyTest,type MonitorSample,type NodeOption,type Plan} from '../domain'
import {nodeDisplayName} from '../nodePresentation'
import {activeChartTip} from '../chartTip'

vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),clear:()=>items.clear()}})})
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,api:{...actual.api,latencyHistoryScopes:vi.fn(async()=>[]),batches:vi.fn(async()=>[]),revisions:vi.fn(async()=>[]),refreshJobs:vi.fn(async()=>[]),latencyHistory:vi.fn(async()=>({tests:[],has_more:false,complete:true})),measurementRounds:vi.fn(async()=>[]),createRound:vi.fn(async()=>undefined),finishRound:vi.fn(async()=>undefined),latencySummaries:vi.fn(async(nodes:NodeOption[])=>nodes.map(()=>({tests:[],complete:true,has_more:false}))),attemptHistory:vi.fn(async()=>({attempts:[],complete:true,has_more:false})),monitorSamples:vi.fn(async()=>({items:[],has_more:false,next_cursor:''})),startLatency:vi.fn(),startAttempt:vi.fn(),createJob:vi.fn()}}})
vi.mock('../usageDaily',async importOriginal=>({...await importOriginal<typeof import('../usageDaily')>(),accountDailyUsage:vi.fn(async()=>({account:undefined,report:undefined,days:[]}))}))

const a:NodeOption={profile_id:'sub-a-one',node_key:'shared-key',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A / One',display_name:'Node A One',type:'trojan',country_code:'HK',country_flag:''}
const aTwo:NodeOption={...a,profile_id:'sub-a-two',node_key:'node-two',node_identity_key:'identity-a-two',config_revision_key:'revision-a-two',profile_name:'Airport A / Two',display_name:'Node A Two',country_code:'JP'}
const b:NodeOption={...a,profile_id:'sub-b',node_identity_key:'identity-b',config_revision_key:'revision-b',profile_name:'Airport B',display_name:'Node B',country_code:'US'}
let wrapper:VueWrapper|undefined,w:ReturnType<typeof createWorkspace>|undefined

function attach(nodes:NodeOption[]=[a,aTwo,b],selected:string[]=[]){
  w=createWorkspace();w.project.value='latency';w.airports.value=[{id:'airport-a',name:'Airport A',node_count:2,has_cache:true,url_display:'',subscriptions:['sub-a-one','sub-a-two'].map((id,i)=>({id,airport_id:'airport-a',name:i?'Two':'One',url_display:'',url_configured:true,node_count:1,has_cache:true}))},{id:'airport-b',name:'Airport B',node_count:1,has_cache:true,url_display:'',subscriptions:[{id:'sub-b',airport_id:'airport-b',name:'Default',url_display:'',url_configured:true,node_count:1,has_cache:true}]}]
  w.nodes.value=nodes;w.selectedAirportIds.value=selected;w.catalog.value=[{service_id:'netflix_unlock',name:'Netflix',category:'Video',description:'',result_kind:'unlock',success_criterion:'',timeout_seconds:15}];w.serviceIds.value=['netflix_unlock']
  wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});return {w,wrapper}
}
async function button(container:DOMWrapper<Element>|VueWrapper,selector:string,label:string){const found=container.findAll(selector).find(b=>b.text()===label);expect(found,`button ${label}`).toBeDefined();await found!.trigger('click')}
function dialog(){return new DOMWrapper(document.querySelector('dialog')!)}
function airportInput(name:string){const label=Array.from(document.querySelectorAll('.airport-pick-list label')).find(e=>e.querySelector('strong')?.textContent===name)!;return new DOMWrapper(label.querySelector('input')!)}
function nodeInput(view:VueWrapper,name:string){return view.get(`.hd-tr input[aria-label="选择 ${name}"]`)}
const rows='.hd-tr:not(.hd-th)'
function cells(view:VueWrapper){return view.findAll(`${rows} .hd-num:not(.hd-history-cell), ${rows} .response-stop strong`).map(m=>m.text())}

beforeEach(()=>{window.localStorage.clear();window.sessionStorage.clear();vi.clearAllMocks();HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}});vi.stubGlobal('fetch',vi.fn().mockRejectedValue(new Error('Unexpected network request in isolated DOM selection test')))})
afterEach(()=>{expect(vi.mocked(fetch).mock.calls.filter(([url,options])=>!((String(url)==='/api/clash/node-status'&&options?.method==='POST')||(String(url)==='/api/clash/usage'&&options?.method==='GET')))).toEqual([]);wrapper?.unmount();w?.dispose();wrapper=undefined;w=undefined;document.body.innerHTML='';vi.unstubAllGlobals()})

describe('node selection through DOM controls',()=>{
  it('keeps reading outside the airport scope without adding an invisible selection or expanding that scope',async()=>{
    const {w,wrapper}=attach([a,aTwo,b],['airport-a','airport-b']);await flushPromises()
    await wrapper.get('.hd-node-open').trigger('click');await flushPromises()
    w.setAirports(['airport-b']);await flushPromises()
    expect(wrapper.get('.hd-inspector h2').text()).toBe(nodeDisplayName(a))
    expect(wrapper.get('input[aria-label="选择这个节点"]').attributes('disabled')).toBeDefined()
    await wrapper.get('input[aria-label="选择这个节点"]').trigger('change');await flushPromises()
    expect(w.selectedKeys.value).toEqual([]);expect(w.selectedAirportIds.value).toEqual(['airport-b'])
    await wrapper.get('.hd-ins-actions .primary').trigger('click');await flushPromises()
    expect(w.plan.value?.nodes).toEqual([a]);expect(w.selectedAirportIds.value).toEqual(['airport-b'])
    expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
  })

  it('selects nodes while reading, compares that selection, and configures only the explicitly viewed node',async()=>{
    vi.stubGlobal('matchMedia',vi.fn(()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()})))
    const {w,wrapper}=attach([a,aTwo,b],['airport-a','airport-b']);await flushPromises()
    ;(wrapper.get('.hd-node-open').element as HTMLElement).focus()
    await wrapper.get('.hd-node-open').trigger('click');await flushPromises()
    await dialog().get('input[aria-label="选择这个节点"]').setValue(true)
    await dialog().get('button[aria-label="查看下一条节点"]').trigger('click');await flushPromises()
    await dialog().get('input[aria-label="选择这个节点"]').setValue(true);await flushPromises()
    expect(w.selectedKeys.value).toEqual([key(a),key(aTwo)])
    await button(dialog(),'.hd-reader-nav button','比较已选 2');await flushPromises()
    expect(dialog().find('.hd-compare').exists()).toBe(true)
    expect(dialog().get('.modal-header h2').text()).toBe('比较所选节点')
    await dialog().get('.modal-header button[aria-label="关闭"]').trigger('click');await flushPromises()
    expect(dialog().get('.hd-inspector h2').text()).toBe(nodeDisplayName(aTwo))
    expect(wrapper.get('.hd-tr.focus .hd-node-open').text()).toBe(nodeDisplayName(aTwo))
    expect(dialog().get('input[aria-label="选择这个节点"]').element).toHaveProperty('checked',true)
    await dialog().get('.hd-ins-actions .primary').trigger('click');await flushPromises()
    expect(w.plan.value?.nodes).toEqual([aTwo]);expect(w.selectedKeys.value).toEqual([key(a),key(aTwo)])
    expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
  })

  it('opens exit details from cached entry data and resolves only after an explicit action',async()=>{
    const entries=vi.spyOn(api,'entries').mockResolvedValue({nodes:[]});onTestFinished(()=>entries.mockRestore())
    const {wrapper}=attach([a],['airport-a']);await flushPromises()
    await button(wrapper,'.hd-seg button','出口与信誉');await wrapper.get('.hd-node-open').trigger('click');await flushPromises()
    expect(entries).toHaveBeenCalled();expect(entries.mock.calls.every(([,refresh])=>refresh===false)).toBe(true)
    expect(wrapper.get('.hd-entry-read').text()).toContain('入口尚未解析')
    await wrapper.get('.hd-entry-read button').trigger('click');await flushPromises()
    expect(entries).toHaveBeenLastCalledWith([a],true)
    expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
  })

  it('keeps the open node identity when filters hide it or its current configuration changes',async()=>{
    vi.stubGlobal('matchMedia',vi.fn(()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()})))
    const {w,wrapper}=attach([a,aTwo,b],['airport-a','airport-b']);w.selectedKeys.value=[key(a)];await flushPromises()
    await wrapper.findAll('.hd-node-open').find(button=>button.text()===nodeDisplayName(b))!.trigger('click');await flushPromises()
    w.search.value='Node A One';await flushPromises()
    expect(dialog().get('.hd-inspector h2').text()).toBe(nodeDisplayName(b))
    expect(dialog().text()).toContain('当前筛选之外')
    w.nodes.value=[a,aTwo,{...b,node_key:'replacement-key',display_name:'Changed B'}];await flushPromises()
    expect(dialog().get('.hd-inspector h2').text()).toBe(nodeDisplayName(b))
    expect(dialog().text()).toContain('配置已变化')
    expect(dialog().get('.hd-ins-actions .primary').attributes('disabled')).toBeDefined()
    expect(w.selectedKeys.value).toEqual([key(a)])
    expect(w.plan.value).toBeNull();expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
  })

  it('reads adjacent nodes across pages in the opening order without changing the selected detection scope',async()=>{
    vi.stubGlobal('matchMedia',vi.fn(()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()})))
    window.localStorage.setItem('speedtest-page-size','10')
    const nodes=Array.from({length:11},(_,i)=>({...a,node_key:'node-'+i,node_identity_key:'identity-'+i,display_name:'Node '+String(i+1).padStart(2,'0')}))
    const {w,wrapper}=attach(nodes,['airport-a']);w.selectedKeys.value=[key(nodes[0]!)];await flushPromises()
    await wrapper.findAll('.hd-node-open').at(-1)!.trigger('click');await flushPromises()
    expect(dialog().get('.hd-inspector h2').text()).toBe(nodeDisplayName(nodes[9]!))
    w.nodes.value=[{...nodes[0]!,node_key:'new',node_identity_key:'new',display_name:'Node 00'},...nodes];await flushPromises()
    await dialog().get('button[aria-label="查看下一条节点"]').trigger('click');await flushPromises()
    expect(dialog().get('.hd-inspector h2').text()).toBe(nodeDisplayName(nodes[10]!))
    expect(dialog().get('.hd-reader-nav').text()).toContain('11 / 11')
    expect(dialog().get('button[aria-label="查看下一条节点"]').attributes('disabled')).toBeDefined()
    await dialog().get('button[aria-label="查看上一条节点"]').trigger('click');await flushPromises()
    expect(dialog().get('.hd-inspector h2').text()).toBe(nodeDisplayName(nodes[9]!))
    expect(w.selectedKeys.value).toEqual([key(nodes[0]!)])
    expect(w.plan.value).toBeNull();expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
    await dialog().get('.modal-header button[aria-label="关闭"]').trigger('click');await flushPromises()
    expect((document.activeElement as HTMLElement).getAttribute('aria-label')).toContain('查看 '+nodeDisplayName(nodes[9]!))
  })

  it('browses rows with arrow keys without opening a sheet or changing the selected detection scope',async()=>{
    vi.stubGlobal('matchMedia',vi.fn(()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()})))
    const {w,wrapper}=attach([a,aTwo,b],['airport-a','airport-b']);w.selectedKeys.value=[key(b)];await flushPromises()
    const first=wrapper.findAll('.hd-node-open')[0]!,second=wrapper.findAll('.hd-node-open')[1]!
    ;(first.element as HTMLElement).focus();await first.trigger('keydown',{key:'ArrowDown'});await flushPromises()
    expect(document.querySelector('dialog')).toBeNull()
    expect(document.activeElement).toBe(second.element)
    expect(w.selectedKeys.value).toEqual([key(b)])
    expect(w.plan.value).toBeNull();expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
    await second.trigger('click');await flushPromises()
    expect(document.querySelector('dialog')?.textContent).toContain(aTwo.display_name)
  })

  it('leaves selection inputs to their native keys and keeps row browsing within the current page',async()=>{
    const {w,wrapper}=attach([a,aTwo,b],['airport-a','airport-b']);await flushPromises()
    const check=wrapper.get('.hd-tr:not(.hd-th) input[type=checkbox]')
    ;(check.element as HTMLElement).focus();await check.trigger('keydown',{key:'ArrowDown'});await flushPromises()
    expect(wrapper.find('.hd-inspector').exists()).toBe(false)
    expect(document.activeElement).toBe(check.element)
    const first=wrapper.findAll('.hd-node-open')[0]!,last=wrapper.findAll('.hd-node-open').at(-1)!
    ;(first.element as HTMLElement).focus();await first.trigger('keydown',{key:'End'});await flushPromises()
    expect(document.activeElement).toBe(last.element)
    await last.trigger('keydown',{key:'Home'});await flushPromises()
    expect(document.activeElement).toBe(first.element)
    expect(w.selectedKeys.value).toEqual([])
  })

  it('opens the Clash active node after clearing the workbench scope and keeps unqueried exit compact',async()=>{
    const usage=vi.spyOn(api,'clashUsage').mockResolvedValue([{name:b.display_name,bytes:100,chains:['AI','Airport B']}])
    const {w,wrapper}=attach([a,aTwo,b],['airport-a']);w.search.value='missing';w.region.value='JP';w.airportFilter.value='airport-a';w.subscription.value='sub-a-one';w.project.value='combined'
    await flushPromises();await wrapper.get('button.hd-clash-usage').trigger('click');await flushPromises()
    expect([w.search.value,w.region.value,w.airportFilter.value,w.subscription.value]).toEqual(['','all','all','all'])
    expect(wrapper.get('.hd-inspector h2').text()).toContain(b.display_name)
    expect(wrapper.get('.exit-unqueried').text()).toBe('出口与信誉：未查询 查询')
    expect(wrapper.find('.exit-ipkind').exists()).toBe(false);expect(wrapper.find('.landing-sources').exists()).toBe(false)
    w.services.value[key(b)]=[{...b,attempt_id:'purity',service_id:'ippure_ip_quality',persistence_state:'saved',execution_state:'completed',rule:{service_id:'ippure_ip_quality'},result:{outcome:'profiled',finished_at:'2026-10-24T10:00:00Z',details:{risk_score:'18',risk_scale_max:'100',risk_direction:'lower_is_better',ip_type:'机房 IDC',origin_type:'原生 IP',country_code:'SG'}}} as never]
    await flushPromises();expect(wrapper.get('.exit-purity').text()).toContain('纯净度 82 · 机房 IP · 原生 SG')
    await wrapper.findAll('[role="columnheader"] button').find(button=>button.text().startsWith('出口画像'))!.trigger('click');await flushPromises()
    expect(wrapper.findAll('.hd-tr:not(.hd-th) .hd-node')[0]!.text()).toContain(b.display_name)
    expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled();usage.mockRestore()
  })
  it('retains stopped monitor history in metrics, bars and node trends without mixing sites or revisions',async()=>{
    const sample:MonitorSample={...a,sample_id:'monitor-ok',run_id:'run-ok',display_name_snapshot:a.display_name,timestamp:'2026-10-03T06:30:00Z',probe_type:'rtt',target:'https://cp.cloudflare.com/generate_204',success:true,latency:80000000,ttfb:0,sampling_tier:'regular'}
    vi.mocked(api.monitorSamples).mockImplementation(async q=>({items:q.node_identity_key==='identity-a'&&q.config_revision_key==='revision-a'?[{...sample,sample_id:'monitor-fail',run_id:'run-fail',timestamp:'2026-10-03T06:35:00Z',success:false,error_class:'timeout'},sample]:[],has_more:false,next_cursor:''}))
    const {w,wrapper}=attach([a],['airport-a']);w.displayTargets.value=['cloudflare','google'];await flushPromises()
    expect(w.jobs.value).toEqual([])
    expect(api.monitorSamples).toHaveBeenCalledWith(expect.objectContaining({profile_id:'sub-a-one',node_identity_key:'identity-a',config_revision_key:'revision-a',probe_type:'rtt',target:undefined,limit:480}))
    expect(cells(wrapper)).toEqual(['—','超时','未测'])
    expect(wrapper.find('.latency-combined').exists()).toBe(false)
    await wrapper.get('.hd-node').trigger('click');await flushPromises()
    expect(wrapper.findAll('.hd-latency .health-cell')).toHaveLength(0)
    expect(wrapper.find('.latency-spark').exists()).toBe(false)
    expect(wrapper.get('.latency-combined').attributes('aria-label')).toContain('本轮实际采样')
    expect(wrapper.findAll('.latency-combined .failed')).toHaveLength(1)
    w.latency.value={[key(a)]:[{...a,attempt_id:'new-manual',node_type:'trojan',requested_at:'2026-10-03T07:00:00Z',finished_at:'2026-10-03T07:00:01Z',status:'completed',latency_ms:55,total_samples:1,success_samples:1,failure_samples:0,persistence_state:'saved',samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-03T07:00:01Z',latency_ms:55,success:true}]}]}
    await flushPromises();expect(cells(wrapper)).toEqual(['—','55 ms','未测'])
    w.nodes.value=[{...a,config_revision_key:'revision-new'}];await flushPromises()
    expect(api.monitorSamples).toHaveBeenLastCalledWith(expect.objectContaining({config_revision_key:'revision-new'}))
    expect(cells(wrapper)).toEqual(['—','未测','未测'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(0)
  })
  it('replays a station or all stations from the selected historical round and returns to the latest',async()=>{
    const {w,wrapper}=attach([a],['airport-a']);w.displayTargets.value=['cloudflare','google'];await flushPromises()
    const at='2026-11-19T01:10:00Z',cloudflare='https://speed.cloudflare.com/__down?bytes=1',google='https://www.gstatic.com/generate_204'
    const latency:LatencyTest={...a,node_type:a.type,attempt_id:'saved-test',requested_at:at,finished_at:at,latency_ms:70,status:'completed',total_samples:4,success_samples:3,failure_samples:1,persistence_state:'saved',samples:[
      {seq:2,target:cloudflare,timestamp:at,latency_ms:0,success:false,error:'timeout'},
      {seq:1,target:cloudflare,timestamp:at,latency_ms:70,success:true},
      {seq:3,target:google,timestamp:at,latency_ms:100,success:true},
      {seq:4,target:google,timestamp:at,latency_ms:35,success:true},
    ]}
    const earlier='2026-11-19T00:50:00Z',single='2026-11-19T00:30:00Z'
    const replayRound=(id:string,time:string,readings:[string,number][])=>{
      const samples=readings.map(([target,latency_ms],index)=>({seq:index+1,target,latency_ms,timestamp:time,success:true}))
      const test={...latency,attempt_id:id+'-test',requested_at:time,finished_at:time,total_samples:samples.length,success_samples:samples.length,failure_samples:0,samples}
      return {round_id:id,trigger_type:'manual' as const,state:'finished',started_at:time,items:[
        {...a,request_id:id,project:'latency' as const,execution_state:'completed',persistence_state:'saved',latency:test},
      ]}
    }
    w.measurementRounds.value[key(a)]=[replayRound('cloudflare-only',single,[[cloudflare,19]]),replayRound('older',earlier,[[cloudflare,22],[google,32]]),{round_id:'saved-round',trigger_type:'manual',state:'finished',started_at:at,items:[
      {...a,request_id:'req',project:'latency',execution_state:'completed',persistence_state:'saved',latency},
    ]}]
    await flushPromises()
    const station=wrapper.findAll('.response-stop')
    expect(station).toHaveLength(2)
    expect(station[0].findAll('.response-vertical i').map(bar=>bar.attributes('title'))).toEqual(['第 1 次 · 70 ms','第 2 次 · 超时'])
    expect(station[1].findAll('.response-vertical i').map(bar=>bar.attributes('title'))).toEqual(['第 1 次 · 100 ms','第 2 次 · 35 ms'])
    expect(station[0].findAll('.response-history button')).toHaveLength(3)
    const values=()=>wrapper.findAll('.response-stop strong').map(s=>s.text())
    await station[0].findAll('.response-history button')[1].trigger('mouseenter')
    await station[0].findAll('.response-history button')[1].trigger('focus')
    expect(activeChartTip.value).toBeNull()
    await station[0].findAll('.response-history button')[1].trigger('click');await flushPromises()
    expect(values()).toEqual(['22 ms','35 ms'])
    expect(station[0].get('.response-round small').text()).toBe('回看 1/1')
    expect(station[0].findAll('.response-history button')[1].attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('.hd-inspector').exists()).toBe(false)
    const whole=wrapper.findAll('.hd-round-history .response-history button')
    expect(whole).toHaveLength(3)
    await whole[1].trigger('click');await flushPromises()
    expect(values()).toEqual(['22 ms','32 ms'])
    expect(whole[1].attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('.hd-inspector').exists()).toBe(false)
    await whole[0].trigger('click');await flushPromises()
    expect(values()).toEqual(['19 ms','未测'])
    expect(wrapper.get('.response-missing').text()).toBe('该轮未测')
    await wrapper.get('.hd-round-review button').trigger('click');await flushPromises()
    expect(values()).toEqual(['70 ms','35 ms'])
    expect(wrapper.find('.hd-round-review').exists()).toBe(false)
    await station[0].get('.response-round').trigger('click');await flushPromises()
    expect(wrapper.find('.round-summary').text()).toContain('已读 2 个站点')
    expect(wrapper.findAll('.round-result strong').map(row=>row.text())).toEqual(['70 ms','超时','100 ms','35 ms'])
    await station[0].findAll('.response-history button')[1].trigger('click');await flushPromises()
    expect(wrapper.findAll('.round-result strong').map(row=>row.text())).toEqual(['22 ms','32 ms'])
    w.project.value='combined';await flushPromises()
    expect(wrapper.findAll('.hd-ov-cell')).toHaveLength(4)
    expect(wrapper.get('.hd-ov-cell').text()).toContain('2/2 站响应')
  })
  it('shows stopped HTTP monitoring in service and combined views without claiming catalog checks passed',async()=>{
    const google:MonitorSample={...a,sample_id:'http-google-1',run_id:'http-run-1',display_name_snapshot:a.display_name,timestamp:'2026-10-03T06:30:00Z',probe_type:'service_google',target:'https://www.google.com/generate_204',success:true,latency:80000000,ttfb:0,sampling_tier:'regular'}
    const github:MonitorSample={...google,sample_id:'http-github-1',probe_type:'service_github',target:'https://api.github.com'}
    vi.mocked(api.monitorSamples).mockImplementation(async q=>({items:q.config_revision_key!=='revision-a'?[]:q.probe_type==='service_google'?[{...google,sample_id:'http-google-2',run_id:'http-run-2',timestamp:'2026-10-03T06:35:00Z',success:false,error_class:'timeout',error_detail:'network timeout'},google]:q.probe_type==='service_github'?[github]:[],has_more:false,next_cursor:''}))
    const {w,wrapper}=attach([a],['airport-a']);w.catalog.value=[{service_id:'google_204',name:'Google 204 连通性',category:'基础网络',description:'',result_kind:'connectivity',success_criterion:'exact 204',timeout_seconds:10}];w.displayServiceIds.value=['google_204'];w.project.value='service';await flushPromises()
    await wrapper.get('.hd-node').trigger('click');await flushPromises()
    expect(w.jobs.value).toEqual([])
    expect(wrapper.findAll('.hd-http .hd-site-value').map(m=>m.text())).toEqual(['失败','HTTP 可达'])
    expect(wrapper.findAll('.hd-svc-cell').map(m=>m.text())).toEqual(['未测','未测','—'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(3)
    const googleBars=wrapper.get('.health-bars[aria-label="Google 基础 HTTP历史状态，悬停查看本轮数据"]')
    await googleBars.findAll('.has-record').at(-1)!.trigger('click');await flushPromises()
    expect(wrapper.get('.measurement-results').text()).toContain('network timeout')
    expect(wrapper.get('.measurement-results').text()).toContain('响应 2xx／3xx')
    expect(wrapper.get('.measurement-results').text()).not.toContain('没有匹配的结果')
    await button(wrapper,'.hd-ins-detail header button','看完整走势');await flushPromises()
    expect(wrapper.get('.inline-full-history').text()).toContain('持续监测 · Google 基础 HTTP')
    expect(wrapper.findAll('.overview-series circle')).toHaveLength(1)
    expect(wrapper.find('select[aria-label="走势对比服务"]').exists()).toBe(false)
    await button(wrapper,'.hd-ins-detail header button','收起');w.project.value='combined';await flushPromises()
    expect(wrapper.findAll('.hd-http .hd-site-value').map(m=>m.text())).toEqual(['失败','HTTP 可达'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(3)
    vi.mocked(api.monitorSamples).mockRejectedValue(new Error('HTTP history unavailable'));w.hours.value=6;await flushPromises()
    expect(wrapper.findAll('.hd-http .hd-site-value').map(m=>m.text())).toEqual(['未测','未测'])
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(0)
    expect(wrapper.findAll('[role=alert]').some(alert=>alert.text().includes('监测 HTTP 记录读取失败'))).toBe(true)
    vi.mocked(api.monitorSamples).mockResolvedValue({items:[],has_more:false,next_cursor:''})
    w.nodes.value=[{...a,config_revision_key:'revision-new'}];await flushPromises()
    expect(wrapper.get('.hd-reader-notice').text()).toContain('配置已变化')
    await wrapper.get('.hd-node-open').trigger('click');await flushPromises()
    expect(wrapper.find('.hd-http').exists()).toBe(false)
    expect(wrapper.findAll('.health-cell.has-record')).toHaveLength(0)
  })
  it('uses the airport picker, subscription filter and node checkboxes as one scope for every project and monitor entry',async()=>{
    const {w,wrapper}=attach();await flushPromises()
    await wrapper.get('.hd-title .hd-link').trigger('click')
    await airportInput('Airport A').setValue(true);await airportInput('Airport B').setValue(true)
    expect(w.selectedAirportIds.value).toHaveLength(2)
    await button(dialog(),'.modal-footer button','完成')
    expect(document.querySelector('dialog')).toBeNull()
    await nodeInput(wrapper,'日本 · Node A Two').setValue(true);await nodeInput(wrapper,'美国 · Node B').setValue(true)
    await wrapper.get('select[aria-label="订阅筛选"]').setValue('sub-a-two')
    expect(wrapper.findAll(rows)).toHaveLength(1)
    expect(wrapper.get('.hd-node strong').text()).toBe('日本 · Node A Two')
    expect(wrapper.get('.hd-selection strong').text()).toBe('已选 2 个节点')

    for(const [label,projects] of [['延迟检测',['latency']],['下载测速',['download']],['解锁检测',['service']],['综合',['latency','download','service']]] as const){
      await button(wrapper,'.hd-seg button',label)
      expect(nodeInput(wrapper,'日本 · Node A Two').element).toHaveProperty('checked',true)
      await wrapper.get('.hd-selection .primary').trigger('click')
      expect(w.plan.value?.projects).toEqual(projects)
      expect(w.plan.value?.nodes).toEqual([aTwo,b])
      w.plan.value=null
      await wrapper.get('.hd-run').trigger('click')
      expect((w.plan.value as Plan|null)?.nodes).toEqual([aTwo,b])
      expect((w.plan.value as Plan|null)?.projects).toEqual(projects)
      w.plan.value=null
      await button(wrapper,'.hd-selection button','持续监测延迟')
      expect(w.monitorNodes.value).toEqual([aTwo,b])
      w.monitorNodes.value=null
    }

    await wrapper.get('.hd-title .hd-link').trigger('click');await airportInput('Airport A').setValue(false)
    await button(dialog(),'.modal-footer button','完成')
    expect(w.subscription.value).toBe('all')
    expect(wrapper.find('select[aria-label="订阅筛选"]').exists()).toBe(false)
    expect(wrapper.findAll(rows)).toHaveLength(1)
    expect(wrapper.get('.hd-selection strong').text()).toBe('已选 1 个节点')
    await wrapper.get('.hd-selection .primary').trigger('click');expect(w.plan.value?.nodes).toEqual([b]);w.plan.value=null
    await button(wrapper,'.hd-selection button','持续监测延迟');expect(w.monitorNodes.value).toEqual([b]);w.monitorNodes.value=null
    await nodeInput(wrapper,'美国 · Node B').setValue(false)
    expect(w.selectedNodes.value).toEqual([])
    expect(wrapper.find('.hd-selection').exists()).toBe(false)
  })

  it('selects and cancels page or filtered ranges without removing a selected node in another subscription',async()=>{
    const many=Array.from({length:25},(_,i)=>({...a,node_key:`node-${i+1}`,node_identity_key:`identity-${i+1}`,display_name:`Node ${String(i+1).padStart(2,'0')}`}))
    const {w,wrapper}=attach([...many,b],['airport-a','airport-b']);await flushPromises()
    await wrapper.get('select[aria-label="订阅筛选"]').setValue('sub-b');await nodeInput(wrapper,'美国 · Node B').setValue(true)
    await wrapper.get('select[aria-label="订阅筛选"]').setValue('sub-a-one')
    await wrapper.get('input[aria-label="选择本页全部节点"]').setValue(true)
    expect(w.selectedNodes.value).toEqual([...many.slice(0,20),b])
    await wrapper.get('button[aria-label="下一页"]').trigger('click')
    expect(wrapper.findAll(rows)).toHaveLength(5)
    await wrapper.get('input[aria-label="选择本页全部节点"]').setValue(true)
    expect(w.selectedNodes.value).toEqual([...many,b])
    await wrapper.get('input[aria-label="选择本页全部节点"]').setValue(false)
    expect(w.selectedNodes.value).toEqual([...many.slice(0,20),b])
    await button(wrapper,'.hd-selection button','全选筛选范围')
    expect(w.selectedNodes.value).toEqual([...many,b])
    await button(wrapper,'.hd-selection button','取消范围选择')
    expect(w.selectedNodes.value).toEqual([b])
    expect(wrapper.get('.hd-selection strong').text()).toBe('已选 1 个节点')
    await wrapper.get('select[aria-label="订阅筛选"]').setValue('sub-b')
    expect(nodeInput(wrapper,'美国 · Node B').element).toHaveProperty('checked',true)
    await button(wrapper,'.hd-selection button','清空已选')
    expect(w.selectedNodes.value).toEqual([])
    expect(nodeInput(wrapper,'美国 · Node B').element).toHaveProperty('checked',false)
  })
})
