import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import MonitorCreateModal from '../components/MonitorCreateModal.vue'
import TrendChart from '../components/TrendChart.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import {key,type Attempt,type LatencyTest,type MonitorJob,type MonitorSample,type NodeOption,type Plan} from '../domain'
import {nodeDisplayName} from '../nodePresentation'
vi.hoisted(()=>{const items=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>items.get(k)||null,setItem:(k:string,v:string)=>items.set(k,v),removeItem:(k:string)=>items.delete(k),clear:()=>items.clear()}})})
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,api:{...actual.api,monitorSamples:vi.fn(async()=>({items:[] as MonitorSample[],has_more:false,next_cursor:''})),latencySummaries:vi.fn(async()=>[]),attemptHistory:vi.fn(async()=>({attempts:[],has_more:false,complete:true})),jobs:vi.fn(async()=>[]),createJob:vi.fn(),jobAction:vi.fn(async()=>{}),startLatency:vi.fn(),startAttempt:vi.fn(),attempt:vi.fn(),attemptAction:vi.fn()}}})
const a:NodeOption={profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'机场 A',display_name:'香港 01',type:'trojan',country_code:'HK',country_flag:''}
const b:NodeOption={profile_id:'sub-b',node_key:'node-b',node_identity_key:'identity-b',config_revision_key:'revision-b',profile_name:'机场 B',display_name:'新加坡 01',type:'ss',country_code:'SG',country_flag:''}
let wrappers:VueWrapper[]=[]
function workspace(){const w=createWorkspace();w.airports.value=[{id:'airport-a',name:'机场 A',node_count:1,has_cache:true,url_display:'',subscriptions:[{id:'sub-a',airport_id:'airport-a',name:'默认',url_display:'',url_configured:true,node_count:1,has_cache:true}]},{id:'airport-b',name:'机场 B',node_count:1,has_cache:true,url_display:'',subscriptions:[{id:'sub-b',airport_id:'airport-b',name:'默认',url_display:'',url_configured:true,node_count:1,has_cache:true}]}];w.nodes.value=[a,b];w.setAirports(['airport-a','airport-b']);w.catalog.value=[{service_id:'netflix_unlock',name:'Netflix',category:'影音',description:'',result_kind:'unlock',success_criterion:'',timeout_seconds:15}];w.serviceIds.value=['netflix_unlock'];return w}
function attach(component:any,w:ReturnType<typeof workspace>){const wrapper=mount(component,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});wrappers.push(wrapper);return wrapper}
beforeEach(()=>{window.localStorage.clear();vi.clearAllMocks();HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})})
afterEach(()=>{wrappers.forEach(w=>w.unmount());wrappers=[];document.body.innerHTML='';vi.useRealTimers();vi.unstubAllGlobals()})
describe('shared node selection',()=>{
  it('uses uniform node labels without changing subscription names or identity',async()=>{
    const samples=[
      ['🇦🇷 [IPLC-移动优化 -阿根廷] ✨ 5x','阿根廷 · IPLC · 移动优化 · 5×'],
      ['🇨🇦加拿大01|流媒体解锁|AI|移动优化','加拿大 · 01 · 流媒体解锁 · AI · 移动优化'],
      ['🇿🇦ZA²_1|5.3MB/s|0%|GPT⁺|YT-CN','南非 · 01（²） · 5.3MB/s · 0% · GPT⁺ · YT-CN'],
      ['加拿大-优化2','加拿大 · 02 · 优化'],
      ['CA-01 专线 2x','加拿大 · 01 · 专线 · 2×'],
      ['线路🇨🇦入口','未识别地区 · 线路🇨🇦入口'],
      ['🇨🇦','加拿大'],
      ['hy2台湾01','台湾 · 01 · HY2'],
      ['🇭🇰【亚洲】香港01丨V6【1x】','香港 · 01 · V6 · 1×'],
      ['🇨🇳台湾专线01|BGP|住宅IP','台湾 · 01 · 专线 · BGP · 住宅IP'],
      ['加拿大-优化2-GPT','加拿大 · 02 · 优化 · GPT'],
      ['🇻🇳VN²_3|3.1MB/s|YT-US','越南 · 03（²） · 3.1MB/s · YT-US'],
    ]
    for(const [raw,expected] of samples)expect(nodeDisplayName({display_name:raw})).toBe(expected)
    const w=workspace(),node={...a,country_code:'CA',display_name:samples[1][0]},originalKey=key(node)
    w.nodes.value=[node]
    vi.mocked(api.latencySummaries).mockResolvedValueOnce([{tests:[],has_more:false,complete:true}])
    const wrapper=attach(HomeView,w)
    expect(wrapper.find('.country-code').text()).toBe('CA')
    expect(wrapper.find('.node-name strong').text()).toBe('加拿大 · 01 · 流媒体解锁 · AI · 移动优化')
    expect(wrapper.find('.node-name-button').attributes('title')).toContain('🇨🇦加拿大01|流媒体解锁|AI|移动优化')
    await wrapper.find('.node-name-button').trigger('click');await flushPromises()
    expect(wrapper.find('.inline-panel-header strong').text()).toBe('加拿大 · 01 · 流媒体解锁 · AI · 移动优化 · 延迟检测')
    expect(wrapper.find('.multi-node-chart').exists()).toBe(true)
    expect(wrapper.find('.multi-node-chart h3').text()).toBe('加拿大 · 01 · 流媒体解锁 · AI · 移动优化 · 完整走势')
    expect(w.nodes.value[0].display_name).toBe('🇨🇦加拿大01|流媒体解锁|AI|移动优化')
    expect(key(w.nodes.value[0])).toBe(originalKey)
    w.nodes.value=[{...node,country_code:'VN',display_name:'🇻🇳VN²_3|3.1MB/s|YT-US'}]
    w.search.value='越南';expect(w.filteredNodes.value).toHaveLength(1)
    w.search.value='VN²';expect(w.filteredNodes.value).toHaveLength(1)
    w.dispose()
  })
  it('offers optional following controls and remembers only that preference',async()=>{
    const w=workspace(),wrapper=attach(HomeView,w)
    expect(wrapper.get('input[aria-label="跟随滚动"]').element).toHaveProperty('checked',false)
    await wrapper.get('input[aria-label="跟随滚动"]').setValue(true)
    expect(wrapper.get('.home-controls').classes()).toContain('following')
    expect(window.localStorage.getItem('speedtest-follow-controls')).toBe('true')
    wrapper.unmount();wrappers=wrappers.filter(item=>item!==wrapper)
    const reopened=attach(HomeView,w)
    expect(reopened.get('input[aria-label="跟随滚动"]').element).toHaveProperty('checked',true)
    await reopened.get('input[aria-label="跟随滚动"]').setValue(false)
    expect(reopened.get('.home-controls').classes()).not.toContain('following')
    expect(window.localStorage.getItem('speedtest-follow-controls')).toBe('false')
    w.dispose()
  })
  it('separates explicit subscription notices while retaining real region nodes, unknown routes and saved history',()=>{
    const w=workspace(),names=['电报群https://t.me/feiniaoyunjichang','防失联页https://github.com/feiniaoyun','每次使用前更新订阅到最新，用不了，换个客户端，重新导入','有超过20多个节点，不够请官网使用文档，下载最新的客户端','剩余流量：200 GB','❗如果只显示此节点|0.1x','❗是客户端太旧导致|0.1x','❗更新一下客户端|0.1x','❗请到网站使用教程|0.1x','请重新复制导入','[69云]✨永久:::69yun69.com','[重要提示]若节点超时，请到网站，使用69代理软件即可！EjZ','订阅地址失效'],notices=names.map((display_name,i)=>({...a,node_key:'notice-'+i,node_identity_key:'notice-'+i,display_name,country_code:'OTHER'})),regional={...a,display_name:'香港 02 · 3倍率 · 剩余流量 100 GB · 有效期至 10 月'},unknown={...a,node_key:'unknown-route',node_identity_key:'unknown-route',display_name:'海外中转 2 · 3倍率',country_code:'OTHER'}
    notices[4].country_code='UK'
    w.nodes.value=[regional,unknown,...notices]
    const saved={...notices[0],attempt_id:'old-notice-history',node_type:'trojan',requested_at:'2026-10-01T00:00:00Z',finished_at:'2026-10-01T00:00:01Z',status:'completed',latency_ms:20,total_samples:1,success_samples:1,failure_samples:0,samples:[],persistence_state:'saved'} as LatencyTest
    w.mergeLatency(saved)
    expect(w.notices.value.map(n=>n.display_name)).toEqual(names)
    expect(w.proxyNodes.value).toEqual([regional,unknown])
    expect(w.nodes.value).toHaveLength(15)
    expect(w.latency.value[key(notices[0])]).toEqual([saved])
    w.dispose()
  })
  it('groups region sorting by airport and subscription, uses natural node numbers and puts unknown regions last',()=>{
    const w=workspace(),a10={...a,display_name:'香港 10'},a2={...a,node_key:'a2',node_identity_key:'a2',display_name:'香港 2'},aSecond={...a,node_key:'a-second',node_identity_key:'a-second',profile_id:'sub-a-2',display_name:'香港 1'},b2={...b,display_name:'香港 2',country_code:'HK'},japan={...b,node_key:'japan',node_identity_key:'japan',display_name:'日本 01',country_code:'JP'},unknown={...a,node_key:'unknown',node_identity_key:'unknown',display_name:'海外中转',country_code:'OTHER'}
    w.airports.value[0].subscriptions!.push({id:'sub-a-2',airport_id:'airport-a',name:'备用',url_display:'',url_configured:true,node_count:1,has_cache:true})
    w.nodes.value=[unknown,japan,b2,a10,aSecond,a2]
    expect(w.filteredNodes.value.map(n=>n.node_key)).toEqual(['a-second','a2','node-a','node-b','japan','unknown'])
    expect(w.proxyNodes.value.filter(n=>n.display_name==='香港 2')).toHaveLength(2)
    w.sort.value='airport';w.priorityAirportId.value='airport-b'
    expect(w.filteredNodes.value.slice(0,2).map(n=>n.node_key)).toEqual(['node-b','japan'])
    w.sort.value='name'
    expect(w.filteredNodes.value.findIndex(n=>n.node_key==='a2')).toBeLessThan(w.filteredNodes.value.findIndex(n=>n.node_key==='node-a'))
    w.nodes.value=[a2,b2,unknown]
    const attempt=(node:NodeOption,id:string,second:number,result?:Attempt['result'],service_id?:string):Attempt=>({...node,attempt_id:id,request_id:id,service_id,requested_at:`2026-10-01T00:00:0${second}Z`,execution_state:result?'completed':'queued',persistence_state:result?'saved':'pending',rule:{target_url:''},result})
    const download=(bytes_read:number):NonNullable<Attempt['result']>=>({outcome:'byte_limit',bytes_read,duration_ns:2000000000,finished_at:'2026-10-01T00:00:01Z'})
    w.mergeAttempt('download',attempt(a2,'download-slow',1,download(10485760)))
    w.mergeAttempt('download',attempt(b2,'download-fast',1,download(20971520)))
    w.sort.value='latency';w.project.value='download'
    expect(w.sort.value).toBe('download');expect(w.filteredNodes.value.map(n=>n.node_key)).toEqual(['node-b','a2','unknown'])
    w.mergeAttempt('download',attempt(b2,'download-new-failure',2,{outcome:'transfer_interrupted',bytes_read:20971520,duration_ns:1000000000,finished_at:'2026-10-01T00:00:03Z'}))
    expect(w.filteredNodes.value.map(n=>n.node_key)).toEqual(['a2','node-b','unknown'])
    w.catalog.value=[...w.catalog.value,{service_id:'youtube',name:'YouTube',category:'服务',description:'',result_kind:'availability',success_criterion:'',timeout_seconds:15}];w.displayServiceIds.value=['netflix_unlock','youtube']
    const service=(outcome:string):NonNullable<Attempt['result']>=>({outcome,bytes_read:0,finished_at:'2026-10-01T00:00:01Z'})
    w.mergeAttempt('service',attempt(a2,'service-old-good',1,service('unlocked'),'netflix_unlock'))
    w.mergeAttempt('service',attempt(a2,'service-new-failure',2,service('transport_error'),'netflix_unlock'))
    w.mergeAttempt('service',attempt(b2,'service-good',1,service('unlocked'),'netflix_unlock'))
    w.mergeAttempt('service',attempt(b2,'service-other-pending',2,undefined,'youtube'))
    w.mergeAttempt('service',attempt(unknown,'service-unknown-old-good',1,service('unlocked'),'netflix_unlock'))
    w.mergeAttempt('service',attempt(unknown,'service-unknown-pending',2,undefined,'netflix_unlock'))
    w.project.value='service'
    expect(w.sort.value).toBe('service');expect(w.filteredNodes.value.map(n=>n.node_key)).toEqual(['node-b','a2','unknown'])
    w.project.value='combined';expect(w.sort.value).toBe('service')
    w.sort.value='download';w.project.value='latency';expect(w.sort.value).toBe('latency')
    for(const sort of ['region','airport','name']){w.sort.value=sort;w.project.value=w.project.value==='service'?'download':'service';expect(w.sort.value).toBe(sort)}
    w.dispose()
  })
  it('filters one airport without losing cross-airport selections or changing the frozen test and monitoring scopes',async()=>{
    const w=workspace();w.toggleNode(a);w.toggleNode(b);w.subscription.value='sub-b';w.airportFilter.value='airport-a'
    expect(w.subscription.value).toBe('all')
    expect(w.filteredNodes.value).toEqual([a])
    expect(w.selectedNodes.value).toEqual([a,b])
    w.openPlan();expect(w.plan.value?.nodes).toEqual([a,b]);expect(w.plan.value?.target).toBe('all')
    expect(w.monitorGroups(w.selectedNodes.value,'监测',60,5,'latency','light').map(g=>[g.profile_id,g.node_keys])).toEqual([['sub-a',['node-a']],['sub-b',['node-b']]])
    w.openPlan(w.filteredNodes.value);expect(w.plan.value?.nodes).toEqual([a])
    w.region.value='HK';w.airportFilter.value='all'
    expect(w.region.value).toBe('HK')
    w.airportFilter.value='airport-b'
    expect(w.region.value).toBe('all');expect(w.filteredNodes.value).toEqual([b])
    w.region.value='SG';w.airportFilter.value='all'
    expect(w.region.value).toBe('SG');expect(w.filteredNodes.value).toEqual([b])
    w.priorityAirportId.value='airport-a';w.setAirports(['airport-b']);await flushPromises()
    expect(w.airportFilter.value).toBe('all');expect(w.priorityAirportId.value).toBe('all');expect(w.selectedNodes.value).toEqual([b])
    w.dispose()
  })
  it('keeps valid nonempty display sites and the primary site synchronized without changing test targets',()=>{
    const w=workspace();expect(w.displayTargets.value).toEqual(['cloudflare'])
    w.displayTargets.value=['github','google','github','invalid']
    expect(w.displayTargets.value).toEqual(['github','google']);expect(w.displayTarget.value).toBe('github')
    w.displayTarget.value='google'
    expect(w.displayTargets.value).toEqual(['google','github'])
    w.displayTargets.value=[];expect(w.displayTargets.value).toEqual(['cloudflare']);expect(w.displayTarget.value).toBe('cloudflare')
    w.displayTarget.value='apple';expect(w.displayTargets.value).toEqual(['apple'])
    const test=(node:NodeOption,id:string,second:number,target:string,latency:number,success=true):LatencyTest=>{
      const time=`2026-10-01T00:00:0${second}Z`
      return {...node,attempt_id:id,node_type:node.type,requested_at:time,finished_at:time,status:success?'completed':'failed',latency_ms:latency,total_samples:1,success_samples:success?1:0,failure_samples:success?0:1,samples:[{seq:1,target,timestamp:time,latency_ms:latency,success,error:success?undefined:'timeout'}],persistence_state:'saved'}
    }
    w.mergeLatency(test(a,'a-google-old',1,'https://www.gstatic.com/generate_204',90))
    w.mergeLatency(test(b,'b-google-old',2,'https://www.gstatic.com/generate_204',60))
    w.mergeLatency(test(a,'a-cloudflare-new',4,'https://speed.cloudflare.com',20))
    w.mergeLatency(test(b,'b-cloudflare-new',4,'https://speed.cloudflare.com',40))
    expect(w.latest(a)?.attempt_id).toBe('a-cloudflare-new')
    expect(w.latestForTarget(a,'google')?.attempt_id).toBe('a-google-old')
    expect(w.latestForTarget(a,'apple')).toBeUndefined()
    w.displayTarget.value='google';w.sort.value='latency'
    expect(w.filteredNodes.value.map(n=>n.node_key)).toEqual(['node-b','node-a'])
    w.mergeLatency(test(a,'a-google-failed',3,'https://www.gstatic.com/generate_204',0,false))
    expect(w.latest(a)?.attempt_id).toBe('a-cloudflare-new')
    expect(w.latestForTarget(a,'google')?.attempt_id).toBe('a-google-failed')
    expect(w.latestForTarget(a,'google')?.samples[0].success).toBe(false)
    expect(w.filteredNodes.value.map(n=>n.node_key)).toEqual(['node-b','node-a'])
    w.openPlan([a]);expect(w.plan.value?.target).toBe('all')
    w.dispose()
  })
  it('extends the same node row to full history and comparison without replacing the list or changing test selection',async()=>{
    vi.useFakeTimers()
    const w=workspace(),at=new Date().toISOString(),more=Array.from({length:23},(_,i)=>({...b,node_key:'extra-'+i,node_identity_key:'identity-extra-'+i,display_name:'Extra '+i}));w.nodes.value=[a,b,...more];w.project.value='download';w.toggleNode(b)
    w.downloads.value[key(a)]=[{...a,attempt_id:'download-a',request_id:'req-a',requested_at:at,finished_at:at,execution_state:'byte_limit',persistence_state:'saved',rule:{target_url:''},result:{outcome:'byte_limit',bytes_read:20971520,duration_ns:2000000000,finished_at:at}}]
    const wrapper=attach(HomeView,w);await flushPromises()
    await wrapper.find('.node-row .health-cell.has-record').trigger('click')
    const panel=()=>wrapper.find('.node-row .node-inline-results')
    await panel().findAll('button').find(b=>b.text()==='完整走势图')!.trigger('click');await flushPromises()
    expect(wrapper.findAll('.node-row')).toHaveLength(20)
    expect(panel().findAll('.overview-series')).toHaveLength(1)
    expect(panel().find('h3').text()).toContain('香港 · 01')
    await panel().findAll('.inline-history-tools button').find(b=>b.text().startsWith('已选节点'))!.trigger('click');await flushPromises()
    expect(panel().findAll('.overview-series').map(s=>s.attributes('aria-label'))).toEqual(['香港 · 01 · 机场 A · 默认','新加坡 · 01 · 机场 B · 默认'])
    await panel().findAll('.inline-history-tools button').find(b=>b.text().startsWith('当前筛选'))!.trigger('click');await flushPromises()
    expect(panel().findAll('.overview-series')).toHaveLength(25)
    expect(vi.mocked(api.attemptHistory).mock.calls.map(call=>call[1].node_identity_key)).toContain('identity-extra-22')
    await panel().trigger('mouseleave');await vi.advanceTimersByTimeAsync(150)
    expect(panel().exists()).toBe(true)
    expect(w.selectedKeys.value).toEqual([key(b)])
    expect(w.inspectNode.value).toBe(null)
    await panel().findAll('button').find(b=>b.text()==='本轮数据')!.trigger('click')
    expect(panel().find('.inline-result-columns').text()).toContain('10.00 MiB/s')
    await panel().findAll('button').find(b=>b.text()==='收起')!.trigger('click')
    expect(panel().exists()).toBe(false)
    await wrapper.findAll('.node-name-button')[1].trigger('click');await flushPromises()
    expect(wrapper.find('.node-inline-results .inline-full-history').exists()).toBe(true)
    expect(wrapper.find('.node-inline-results .chart-empty').text()).toContain('暂无已读取的检测记录')
    expect(wrapper.find('.node-inline-results .inline-panel-header .view-switch button').attributes('disabled')).toBeDefined()
    expect(w.inspectNode.value).toBe(null)
    const nodeButton=wrapper.findAll('.node-name-button')[1]
    expect(nodeButton.attributes('aria-expanded')).toBe('true')
    await nodeButton.trigger('click');await flushPromises()
    expect(wrapper.find('.node-inline-results').exists()).toBe(false)
    await nodeButton.trigger('click');await flushPromises()
    await wrapper.find('.node-inline-results').trigger('keydown',{key:'Escape'});await flushPromises()
    expect(wrapper.find('.node-inline-results').exists()).toBe(false)
    expect(document.activeElement).toBe(nodeButton.element)
  })
  it('keeps per-service health history and exact round data inside the node expansion',async()=>{
    const w=workspace(),at=new Date().toISOString();w.project.value='service'
    w.catalog.value.push({...w.catalog.value[0],service_id:'youtube',name:'YouTube'});w.serviceIds.value=['netflix_unlock','youtube']
    w.services.value[key(a)]=['netflix_unlock','youtube'].map((id,i)=>({...a,attempt_id:'service-'+id,request_id:'round:r:'+id,service_id:id,display_name:a.display_name,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{name:i?'YouTube':'Netflix',target_url:''},result:{outcome:i?'transport_error':'unlocked',bytes_read:0,finished_at:at,error_message:i?'connection refused':undefined}}))
    const wrapper=attach(HomeView,w);await flushPromises();await wrapper.find('.node-name-button').trigger('click');await flushPromises()
    const panel=wrapper.find('.node-inline-results');await panel.find('.inline-service-history>summary').trigger('click')
    expect(panel.findAll('.inline-service-row')).toHaveLength(2)
    expect(panel.find('.inline-service-row').text()).toContain('Netflix')
    await panel.findAll('.inline-service-row')[1].find('.health-cell.has-record').trigger('mouseenter')
    expect(panel.find('.measurement-results').text()).toContain('YouTube')
    expect(panel.find('.measurement-results').text()).toContain('connection refused')
    expect(panel.find('.measurement-results').text()).not.toContain('Netflix')
    expect((panel.find('select').element as HTMLSelectElement).value).toBe('youtube')
    expect(w.serviceIds.value).toEqual(['netflix_unlock','youtube'])
    expect(w.inspectNode.value).toBe(null)
  })
  it('opens data directly from the corresponding status bar and closes it on leaving',async()=>{
    vi.useFakeTimers()
    const w=workspace(),at=new Date().toISOString();w.jobs.value=[{id:'job-a',profile_id:'sub-a',node_keys:['node-a'],state:'running'} as MonitorJob]
    vi.mocked(api.monitorSamples).mockResolvedValue({items:[{...a,sample_id:'monitor-a',timestamp:at,latency:400000000,success:true,display_name_snapshot:a.display_name,target:'https://cp.cloudflare.com/generate_204',probe_type:'rtt',ttfb:0,sampling_tier:'regular'} as MonitorSample],has_more:false,next_cursor:''})
    w.downloads.value[key(a)]=[{...a,attempt_id:'download-a',request_id:'req-a',requested_at:at,finished_at:at,execution_state:'byte_limit',persistence_state:'saved',rule:{target_url:''},result:{outcome:'byte_limit',bytes_read:20971520,duration_ns:2000000000,finished_at:at}}]
    w.catalog.value.push({service_id:'youtube',name:'YouTube',category:'影音',description:'',result_kind:'unlock',success_criterion:'',timeout_seconds:15})
    w.displayServiceIds.value=['netflix_unlock','youtube']
    w.services.value[key(a)]=[{...a,attempt_id:'netflix-a',request_id:'netflix-round',service_id:'netflix_unlock',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{name:'Netflix',target_url:''},result:{outcome:'matched',bytes_read:0,finished_at:at}},{...a,attempt_id:'youtube-a',request_id:'youtube-round',service_id:'youtube',requested_at:at,finished_at:at,execution_state:'failed',persistence_state:'saved',rule:{name:'YouTube',target_url:''},result:{outcome:'connection_failed',bytes_read:0,finished_at:at}}]
    w.project.value='combined';const wrapper=attach(HomeView,w);await flushPromises()
    expect(wrapper.find('.node-row .combined-values').text()).toContain('下载速度10.0 MiB/s')
    expect(wrapper.find('.node-row .combined-values').text()).toContain('Netflix通过')
    expect(wrapper.find('.node-row .combined-values').text()).toContain('YouTube连接失败')
    await wrapper.find('.node-row').trigger('mouseenter')
    expect(wrapper.find('.node-inline-results').exists()).toBe(false)
    const bars=wrapper.find('.node-row').findAll('.health-bars')
    await bars[0].find('.health-cell.has-record').trigger('mouseenter')
    expect(wrapper.findComponent(TrendChart).props('points')[0].value).toBe(400)
    expect(wrapper.find('.node-inline-results').text()).toContain('400 ms')
    await bars[1].find('.health-cell.has-record').trigger('mouseenter')
    expect(wrapper.findComponent(TrendChart).props('unit')).toBe('MiB/s')
    expect(wrapper.findComponent(TrendChart).props('points')[0]).toMatchObject({id:'download-a',value:10,tone:'good'})
    expect(wrapper.find('.node-inline-results').text()).toContain('10.00 MiB/s')
    await bars[2].find('.health-cell.has-record').trigger('mouseenter')
    expect(wrapper.findComponent(TrendChart).props('points')).toEqual([expect.objectContaining({id:'netflix-a',value:1,tone:'good'})])
    expect(wrapper.find('.measurement-results').text()).toContain('Netflix')
    await bars[3].find('.health-cell.has-record').trigger('mouseenter')
    expect(wrapper.findComponent(TrendChart).props('points')).toEqual([expect.objectContaining({id:'youtube-a',value:0,tone:'bad'})])
    expect(wrapper.find('.measurement-results').text()).toContain('YouTube')
    expect(wrapper.find('.measurement-results').text()).not.toContain('Netflix')
    expect(w.serviceIds.value).toEqual(['netflix_unlock'])
    await bars[1].trigger('mouseleave');await vi.advanceTimersByTimeAsync(150)
    expect(wrapper.find('.node-inline-results').exists()).toBe(false)
    vi.mocked(api.monitorSamples).mockResolvedValue({items:[],has_more:false,next_cursor:''})
  })
  it('makes both individual and filtered-range test entry points use the displayed project',async()=>{
    const w=workspace(),wrapper=attach(HomeView,w);await flushPromises()
    await wrapper.findAll('.row-run')[1].trigger('click')
    expect(w.plan.value?.projects).toEqual(['latency']);expect(w.plan.value?.nodes.map(n=>n.node_key)).toEqual(['node-b'])
    w.plan.value=null;await wrapper.findAll('.project-tabs button').find(b=>b.text()==='下载测速')!.trigger('click')
    expect(wrapper.find('.scope-run').text()).toContain('测速')
    await wrapper.find('.scope-run').trigger('click')
    expect((w.plan.value as Plan|null)?.projects).toEqual(['download']);expect((w.plan.value as Plan|null)?.nodes.map(n=>n.node_key)).toEqual(['node-a','node-b'])
  })
  it('keeps the selected node when changing project and sends that exact node to download and service',async()=>{const w=workspace(),wrapper=attach(HomeView,w);await flushPromises();w.toggleNode(b);await flushPromises();await wrapper.findAll('.project-tabs button').find(b=>b.text()==='下载测速')!.trigger('click');await wrapper.find('.selection-actions .primary').trigger('click');expect((w.plan.value as Plan|null)?.projects).toEqual(['download']);expect((w.plan.value as Plan|null)?.nodes.map(n=>n.node_key)).toEqual(['node-b']);w.plan.value=null;await wrapper.findAll('.project-tabs button').find(b=>b.text()==='服务可用性')!.trigger('click');await wrapper.find('.selection-actions .primary').trigger('click');expect((w.plan.value as Plan|null)?.projects).toEqual(['service']);expect((w.plan.value as Plan|null)?.nodes.map(n=>n.node_key)).toEqual(['node-b']);expect((w.plan.value as Plan|null)?.serviceIds).toEqual(['netflix_unlock'])})
  it('cancel multi-select clears the selection and its actions',async()=>{const w=workspace(),wrapper=attach(HomeView,w);w.toggleNode(a);w.toggleNode(b);await flushPromises();expect(wrapper.find('.selection-bar').text()).toContain('已选 2 个节点');await wrapper.findAll('.toolbar-filters button').find(b=>b.text()==='取消多选')!.trigger('click');expect(w.selectedKeys.value).toEqual([]);expect(wrapper.find('.selection-bar').exists()).toBe(false);expect(wrapper.findAll('.node-check input')).toHaveLength(0)})
  it('preserves selection through a search but removes nodes when their airport is deselected',async()=>{const w=workspace();w.toggleNode(a);w.toggleNode(b);w.search.value='香港';await flushPromises();expect(w.selectedNodes.value.map(n=>n.node_key)).toEqual(['node-a','node-b']);w.setAirports(['airport-a']);await flushPromises();expect(w.selectedNodes.value.map(n=>n.node_key)).toEqual(['node-a']);expect(w.selectedKeys.value).toEqual([key(a)])})
  it('creates separate monitor jobs for selected subscriptions and resumes a partial failure without duplicates',async()=>{const w=workspace();w.monitorNodes.value=[a,b];const jobA={id:'job-a',profile_id:'sub-a'} as MonitorJob,jobB={id:'job-b',profile_id:'sub-b'} as MonitorJob;vi.mocked(api.createJob).mockResolvedValueOnce(jobA).mockRejectedValueOnce(new Error('temporary write failure')).mockResolvedValueOnce(jobB);attach(MonitorCreateModal,w);const button=()=>Array.from(document.querySelectorAll('dialog .modal-footer button')).find(b=>b.textContent?.includes('创建')||b.textContent?.includes('继续未完成')) as HTMLButtonElement;button().click();await flushPromises();expect(api.createJob).toHaveBeenNthCalledWith(1,expect.objectContaining({profile_id:'sub-a',node_keys:['node-a'],node_contexts:[{node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a'}]}));expect(api.createJob).toHaveBeenNthCalledWith(2,expect.objectContaining({profile_id:'sub-b',node_keys:['node-b']}));expect(document.body.textContent).toContain('已成功创建的任务保留');button().click();await flushPromises();expect(api.createJob).toHaveBeenCalledTimes(3);expect(api.createJob).toHaveBeenNthCalledWith(3,expect.objectContaining({profile_id:'sub-b',node_keys:['node-b']}));expect(vi.mocked(api.jobAction).mock.calls.filter(c=>c[1]==='start').map(c=>c[0])).toEqual(['job-a','job-b'])})
  it('cancels an attempt whose start response arrives after cancel and does not start the next node',async()=>{vi.useFakeTimers();const w=workspace();let resolveStart!:(a:Attempt)=>void;vi.mocked(api.startAttempt).mockImplementationOnce(()=>new Promise(resolve=>resolveStart=resolve));const cancelled={...a,attempt_id:'attempt-a',request_id:'req-a',display_name:a.display_name,requested_at:new Date().toISOString(),execution_state:'cancelled',persistence_state:'saved',rule:{target_url:''}} as Attempt;vi.mocked(api.attemptAction).mockResolvedValue(cancelled);vi.mocked(api.attempt).mockResolvedValue(cancelled);w.plan.value={nodes:[a,b],projects:['download'],target:'all',samples:6,timeout:5,concurrency:8,downloadMiB:20,downloadSeconds:10,serviceIds:[],repeats:1};const running=w.runPlan();await flushPromises();await w.cancelTests();resolveStart({...cancelled,execution_state:'running'});await vi.runAllTimersAsync();await running;expect(api.startAttempt).toHaveBeenCalledTimes(1);expect(api.startAttempt).toHaveBeenCalledWith('download',expect.objectContaining({profile_id:'sub-a',node_key:'node-a',maximum_bytes:20971520}));expect(api.attemptAction).toHaveBeenCalledWith('download','attempt-a',expect.objectContaining({profile_id:'sub-a',node_key:'node-a'}),'cancel');expect(w.queue.value.map(q=>q.state)).toEqual(['cancelled','cancelled'])})
})
