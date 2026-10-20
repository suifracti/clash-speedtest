import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api,type SubscriptionRefreshJob} from '../api'
import type {Airport,NodeOption,MonitorSample} from '../domain'

vi.hoisted(()=>{
  const items=new Map<string,string>()
  Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(key:string)=>items.get(key)||null,setItem:(key:string,value:string)=>items.set(key,value),removeItem:(key:string)=>items.delete(key),clear:()=>items.clear()}})
})

vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,
    refreshJobs:vi.fn(async()=>[]),refreshJob:vi.fn(),startRefresh:vi.fn(),cancelRefresh:vi.fn(),
    latencyHistory:vi.fn(async()=>({tests:[],has_more:false,complete:true})),
    measurementRounds:vi.fn(async()=>[]),monitorSamples:vi.fn(async()=>({items:[] as MonitorSample[],has_more:false,next_cursor:''})),
    latencySummaries:vi.fn(async()=>[]),attemptHistory:vi.fn(async()=>({attempts:[],has_more:false,complete:true})),
    startLatency:vi.fn(),startAttempt:vi.fn(),createRound:vi.fn(),finishRound:vi.fn(),createJob:vi.fn(),jobAction:vi.fn()
  }}
})

const profileIds=Array.from({length:8},(_,i)=>`subscription-${i+1}`)
function airports():Airport[]{
  return Array.from({length:4},(_,airportIndex)=>({
    id:`airport-${airportIndex+1}`,name:`合成机场 ${airportIndex+1}`,url_display:'',node_count:64,has_cache:true,
    website_url:'https://synthetic.invalid',backup_url:'',
    subscriptions:Array.from({length:2},(_,subscriptionIndex)=>{
      const index=airportIndex*2+subscriptionIndex
      return {id:profileIds[index],airport_id:`airport-${airportIndex+1}`,name:`合成账户 ${index+1}`,url_display:'',url_configured:true,node_count:32,has_cache:true,
        usage:{upload:1024**3,download:2*1024**3,total:10*1024**3,expire:Date.UTC(2027,0,1)/1000}}
    })
  }))
}
function nodes():NodeOption[]{
  return Array.from({length:256},(_,index)=>({
    profile_id:profileIds[index%profileIds.length],node_key:`synthetic-node-${index+1}`,node_identity_key:`synthetic-identity-${index+1}`,config_revision_key:'synthetic-revision',
    profile_name:`合成账户 ${index%profileIds.length+1}`,display_name:`香港合成节点 ${String(index+1).padStart(3,'0')}`,type:index%2?'trojan':'ss',country_code:'HK',country_flag:''
  }))
}
function noticeNode(profileId=profileIds[0]):NodeOption{
  const index=profileIds.indexOf(profileId)
  return {profile_id:profileId,node_key:`synthetic-notice-${index+1}`,node_identity_key:`synthetic-notice-identity-${index+1}`,config_revision_key:'synthetic-revision',profile_name:`合成账户 ${index+1}`,display_name:'公告：合成维护通知',type:'',country_code:'XX',country_flag:''}
}
function refreshJob(itemState:'failed'|'interrupted'|'succeeded'='failed',createdAt='2026-10-11T10:00:00Z'):SubscriptionRefreshJob{
  const failed=itemState==='failed'||itemState==='interrupted'
  return {id:`synthetic-refresh-${itemState}-${createdAt}`,state:'finished',created_at:createdAt,finished_at:'2026-10-11T10:00:01Z',completed:1,success:itemState==='succeeded'?1:0,failed:failed?1:0,cancelled:0,not_executed:0,cancel_requested:false,items:[{
    airport_id:'airport-1',subscription_id:profileIds[0],airport_name:'合成机场 1',subscription_name:'合成账户 1',source_fingerprint:'synthetic-source',source_version:1,state:itemState,error_code:failed?'synthetic_failure':undefined,error_message:failed?'synthetic refresh failure':undefined,duration_ms:100,node_count:0,stages:[{state:itemState,at:'2026-10-11T10:00:01Z'}]
  }]}
}
function runningRefreshJob():SubscriptionRefreshJob{
  const job=refreshJob('succeeded')
  job.state='running';job.completed=1;job.success=1
  job.items.push({...job.items[0]!,subscription_id:profileIds[1]!,subscription_name:'合成账户 2',state:'running',error_code:undefined,error_message:undefined})
  return job
}

const wrappers:VueWrapper[]=[]
const workspaces:ReturnType<typeof createWorkspace>[]=[]
function attachHome(options:{airports?:Airport[];nodes?:NodeOption[]}={}){
  const workspace=createWorkspace();workspaces.push(workspace)
  workspace.airports.value=options.airports||airports();workspace.nodes.value=options.nodes||nodes();workspace.setAirports(workspace.airports.value.map(airport=>airport.id))
  const wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:workspace}}});wrappers.push(wrapper)
  return {workspace,wrapper}
}

beforeEach(()=>{
  window.localStorage.clear();window.localStorage.setItem('speedtest-home-view','detailed');window.sessionStorage.clear();vi.clearAllMocks()
  vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
})
afterEach(()=>{
  wrappers.forEach(wrapper=>wrapper.unmount());wrappers.length=0
  workspaces.forEach(workspace=>workspace.dispose());workspaces.length=0
  document.body.innerHTML='';vi.unstubAllGlobals()
})

describe('synthetic 256-node home first screen',()=>{
  it('keeps current account notices, expiry, and refresh status visible beside the airport orbit',async()=>{
    const syntheticAirports=airports()
    syntheticAirports[0]!.subscriptions![0]!.usage!.expire=Math.floor(Date.now()/1000)-60
    vi.mocked(api.refreshJobs).mockResolvedValueOnce([refreshJob()])
    const {wrapper}=attachHome({airports:syntheticAirports,nodes:[...nodes(),noticeNode()]});await flushPromises()
    const overview=wrapper.get('.subscription-overview')
    expect(wrapper.find('details.home-subscription-disclosure').exists()).toBe(false)
    expect(overview.text()).toContain('公告 1')
    expect(overview.text()).toContain('已过期')
    expect(overview.text()).toContain('刷新失败')
    expect(overview.text()).toContain('synthetic refresh failure')
    await wrapper.get('.hd-orbit').trigger('click');await wrapper.get('.hd-orbit').trigger('click');await flushPromises()
    const accountSelect=wrapper.get('select[aria-label="首页用量订阅"]')
    await accountSelect.setValue((accountSelect.element as HTMLSelectElement).options[1]!.value)
    await flushPromises()
    expect(wrapper.get('.subscription-account').text()).toContain('合成账户 6')
    expect(overview.text()).toContain('刷新异常 1 项')
    expect(wrapper.get('.subscription-issues').text()).toContain('合成机场 1 · 合成账户 1')
    expect(api.startRefresh).not.toHaveBeenCalled()
  })

  it('keeps refresh status unknown while the job read is pending or fails',async()=>{
    let resolveJobs!:(jobs:SubscriptionRefreshJob[])=>void
    vi.mocked(api.refreshJobs).mockImplementationOnce(()=>new Promise(resolve=>{resolveJobs=resolve}))
    const pending=attachHome().wrapper
    expect(pending.get('.status-read').text()).toContain('刷新状态读取中')
    resolveJobs([]);await flushPromises()
    expect(pending.find('.status-read').exists()).toBe(false)

    vi.mocked(api.refreshJobs).mockRejectedValueOnce(new Error('synthetic job read failed'))
    const failed=attachHome().wrapper;await flushPromises()
    expect(failed.get('.status-read').text()).toContain('刷新状态未知')
    expect(failed.get('.subscription-message').text()).toContain('synthetic job read failed')
  })

  it('shows interrupted refreshes and replaces them with a newer success',async()=>{
    vi.mocked(api.refreshJobs).mockResolvedValueOnce([refreshJob('interrupted')])
    const interrupted=attachHome().wrapper;await flushPromises()
    expect(interrupted.get('.subscription-actions .badge').text()).toBe('刷新中断')
    expect(interrupted.get('.subscription-issues summary').text()).toBe('刷新异常 1 项')

    vi.mocked(api.refreshJobs).mockResolvedValueOnce([refreshJob('succeeded','2026-10-11T12:00:00Z'),refreshJob('interrupted')])
    const recovered=attachHome().wrapper;await flushPromises()
    expect(recovered.get('.subscription-actions .badge').text()).toBe('刷新成功')
    expect(recovered.find('.subscription-issues').exists()).toBe(false)
  })

  it('shows current account success and whole job progress together',async()=>{
    vi.mocked(api.refreshJobs).mockResolvedValueOnce([runningRefreshJob()])
    const {wrapper}=attachHome();await flushPromises()

    expect(wrapper.get('.subscription-actions .badge').text()).toBe('刷新成功 · 其他订阅刷新中 1/2')
    expect(api.startRefresh).not.toHaveBeenCalled()
  })

  it('uses subscription read state instead of unrelated workspace loading or errors',async()=>{
    vi.mocked(api.refreshJobs).mockResolvedValueOnce([])
    const {workspace,wrapper}=attachHome()
    workspace.loading.value=true
    workspace.error.value='synthetic unrelated settings failure'
    await flushPromises()
    expect(wrapper.find('.status-read').exists()).toBe(false)
    expect(wrapper.find('.subscription-message').exists()).toBe(false)
  })

  it('keeps the status strip visible without changing airport and node measurement scope',async()=>{
    const {workspace,wrapper}=attachHome();await flushPromises()
    expect(wrapper.find('details.home-subscription-disclosure').exists()).toBe(false)
    expect(wrapper.find('.subscription-overview').exists()).toBe(true)
    expect(wrapper.get('.hd-orbit').text()).toContain('合成机场 1')
    expect(wrapper.get('.hd-foot').text()).toContain('256 个节点')

    const selectedAirports=[...workspace.selectedAirportIds.value!]
    const accountPicker=wrapper.get('select[aria-label="首页用量订阅"]')
    const accountSelect=accountPicker.element as HTMLSelectElement
    expect(accountSelect.options).toHaveLength(2)
    await wrapper.get('.hd-orbit').trigger('click');await wrapper.get('.hd-orbit').trigger('click');await flushPromises()
    const nextAccountPicker=wrapper.get('select[aria-label="首页用量订阅"]')
    const nextAccountSelect=nextAccountPicker.element as HTMLSelectElement
    await nextAccountPicker.setValue(nextAccountSelect.options[1]!.value)
    await flushPromises()

    expect(wrapper.get('.subscription-account').text()).toContain('合成账户 6')
    expect(workspace.selectedAirportIds.value).toEqual(selectedAirports)
    expect(workspace.filteredNodes.value).toHaveLength(256)
    expect(workspace.subscription.value).toBe('all')
    expect(api.startRefresh).not.toHaveBeenCalled()
    expect(api.startLatency).not.toHaveBeenCalled()
    expect(api.startAttempt).not.toHaveBeenCalled()
  })

  it('shows failures from other workspace airports in the visible status strip',async()=>{
    const otherAirportFailure=refreshJob()
    otherAirportFailure.items[0]={...otherAirportFailure.items[0]!,airport_id:'airport-4',subscription_id:profileIds[6]!,airport_name:'合成机场 4',subscription_name:'合成账户 7'}
    vi.mocked(api.refreshJobs).mockResolvedValueOnce([otherAirportFailure])
    vi.mocked(api.startRefresh).mockResolvedValueOnce({...refreshJob('succeeded'),id:'retry-job',state:'finished'})
    const {wrapper}=attachHome();await flushPromises()

    const issue=wrapper.get('.subscription-issues')
    expect(issue.text()).toContain('刷新异常 1 项')
    await issue.get('summary').trigger('click')
    expect(issue.text()).toContain('合成机场 4 · 合成账户 7')
    expect(issue.text()).toContain('synthetic refresh failure')
    const retry=issue.findAll('button').find(button=>button.text().includes('重试此任务失败项'))
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    expect(api.startRefresh).toHaveBeenCalledWith(expect.objectContaining({retry_of:otherAirportFailure.id}))
  })
})

describe('airport filter keeps the header and source strip on that airport (synthetic data)',()=>{
  it('switches the source strip when the sidebar filter changes and follows the orbit while filtered',async()=>{
    const {workspace,wrapper}=attachHome();await flushPromises()
    expect(wrapper.get('.hd-orbit-text strong').text()).toBe('合成机场 1')
    workspace.viewAirport('airport-3');await flushPromises()
    expect(wrapper.get('.hd-orbit-text strong').text()).toBe('合成机场 3')
    expect(wrapper.get('.hd-title p').text()).toContain('合成机场 3 · 32 个节点')
    expect(wrapper.get('.hd-title p').text()).toContain('看全部 4 个机场')
    await wrapper.get('.hd-orbit').trigger('click');await flushPromises()
    expect(workspace.airportFilter.value).toBe('airport-4')
    expect(wrapper.get('.hd-orbit-text strong').text()).toBe('合成机场 4')
    await wrapper.findAll('.hd-title button').find(b=>b.text().includes('看全部'))!.trigger('click');await flushPromises()
    expect(workspace.airportFilter.value).toBe('all')
    expect(wrapper.get('.hd-title p').text()).toContain('128 个节点')
  })
})
