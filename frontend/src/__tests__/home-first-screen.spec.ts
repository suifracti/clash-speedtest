import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api,type SubscriptionRefreshJob} from '../api'
import {key} from '../domain'
import type {Airport,LatencyTest,NodeOption,MonitorSample} from '../domain'

vi.hoisted(()=>{
  const items=new Map<string,string>()
  Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(key:string)=>items.get(key)||null,setItem:(key:string,value:string)=>items.set(key,value),removeItem:(key:string)=>items.delete(key),clear:()=>items.clear()}})
})

vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,
    refreshJobs:vi.fn(async()=>[]),refreshJob:vi.fn(),startRefresh:vi.fn(),cancelRefresh:vi.fn(),
    latencyHistoryScopes:vi.fn(async()=>[]),latencyHistory:vi.fn(async()=>({tests:[],has_more:false,complete:true})),
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
  workspace.airportFilter.value=workspace.airports.value[0]?.id||'all'
  const wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:workspace}}});wrappers.push(wrapper)
  return {workspace,wrapper}
}

beforeEach(()=>{
  window.localStorage.clear();window.localStorage.setItem('speedtest-home-view','detailed');window.sessionStorage.clear();vi.clearAllMocks()
  vi.mocked(api.latencyHistoryScopes).mockResolvedValue([])
  vi.mocked(api.latencyHistory).mockResolvedValue({tests:[],has_more:false,complete:true})
  vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
})
afterEach(()=>{
  wrappers.forEach(wrapper=>wrapper.unmount());wrappers.length=0
  workspaces.forEach(workspace=>workspace.dispose());workspaces.length=0
  document.body.innerHTML='';vi.unstubAllGlobals()
})

it('reads seven-day history for visible pages and reuses it when returning to a read page',async()=>{
  const {workspace,wrapper}=attachHome();await flushPromises()
  const reads=vi.mocked(api.latencyHistoryScopes)
  expect(reads).toHaveBeenCalledTimes(20)
  await wrapper.get('[aria-label="下一页"]').trigger('click');await flushPromises()
  expect(reads).toHaveBeenCalledTimes(40)
  expect(new Set(reads.mock.calls.map(([node])=>node.node_key)).size).toBe(40)
  expect(new Set(reads.mock.calls.map(([,window])=>window.until)).size).toBe(1)
  await wrapper.get('[aria-label="上一页"]').trigger('click');await flushPromises()
  expect(reads).toHaveBeenCalledTimes(40)
  workspace.page.value='history';workspace.running.value=true;await flushPromises()
  workspace.running.value=false;await flushPromises()
  expect(reads).toHaveBeenCalledTimes(40)
  workspace.page.value='home';await flushPromises()
  expect(reads).toHaveBeenCalledTimes(60)
})

it('stops starting reads for an abandoned page while keeping one shared request limit',async()=>{
  const release:(()=>void)[]=[]
  const reads=vi.mocked(api.latencyHistoryScopes)
  reads.mockImplementation(()=>release.length<6?new Promise(resolve=>{release.push(()=>resolve([]))}):Promise.resolve([]))
  const {wrapper}=attachHome();await flushPromises()
  expect(reads).toHaveBeenCalledTimes(6)
  await wrapper.get('[aria-label="下一页"]').trigger('click');await flushPromises()
  expect(reads).toHaveBeenCalledTimes(6)
  for(const finish of release)finish()
  await flushPromises()
  const pageKeys=wrapper.findAll('.hd-row-scroll [data-node-key]').map(row=>row.attributes('data-node-key'))
  expect(reads).toHaveBeenCalledTimes(26)
  expect(reads.mock.calls.slice(6).map(([node])=>key(node))).toEqual(pageKeys)
})

it('waits for the full seven-day sort and filter range before applying the known results',async()=>{
  const fixture=nodes().filter(n=>n.profile_id===profileIds[0]).slice(0,25)
  const best=fixture.at(-1)!,unreadable=fixture[2]!
  const at=new Date(Date.now()-60_000).toISOString()
  const saved:LatencyTest={...best,attempt_id:'later-page-good',node_type:best.type,requested_at:at,finished_at:at,status:'completed',latency_ms:34,total_samples:2,success_samples:2,failure_samples:0,persistence_state:'saved',samples:[1,2].map(seq=>({seq,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,latency_ms:34,success:true}))}
  vi.mocked(api.latencyHistory).mockImplementation(async n=>({tests:n.node_key===best.node_key?[saved]:[],has_more:false,complete:true}))
  let release!:()=>void
  vi.mocked(api.latencyHistoryScopes).mockImplementation(n=>{
    if(n.node_key===best.node_key)return new Promise(resolve=>{release=()=>resolve([])})
    if(n.node_key===unreadable.node_key)return Promise.reject(new Error('synthetic history unavailable'))
    return Promise.resolve([])
  })
  const {workspace,wrapper}=attachHome({nodes:fixture});workspace.project.value='latency';await flushPromises()
  const first=wrapper.get('.hd-row-scroll [role="row"] .hd-node-open').text()
  const sort=wrapper.findAll('.history-sort').find(button=>button.text().includes('7 天响应率'))!
  await sort.trigger('click');await flushPromises()
  expect(wrapper.text()).toContain('正在读取近 7 天记录')
  expect(sort.element.parentElement?.getAttribute('aria-sort')).toBe('none')
  expect(wrapper.get('.hd-row-scroll [role="row"] .hd-node-open').text()).toBe(first)
  await wrapper.get('select[aria-label="质量证据筛选"]').setValue('response');await flushPromises()
  expect(wrapper.get('.hd-run').attributes('disabled')).toBeDefined()
  workspace.toggleNode(fixture[0]!);await flushPromises()
  expect(wrapper.get('.hd-run').attributes('disabled')).toBeUndefined()
  workspace.cancelSelection();await flushPromises()
  release();await flushPromises()
  expect(vi.mocked(api.latencyHistoryScopes)).toHaveBeenCalledTimes(25)
  expect(sort.element.parentElement?.getAttribute('aria-sort')).toBe('descending')
  expect(wrapper.findAll('.hd-row-scroll [data-node-key]').map(row=>row.attributes('data-node-key'))).toEqual([key(best)])
  expect(wrapper.text()).toContain('1 个节点读取失败')
  expect(wrapper.get('.hd-run').attributes('disabled')).toBeUndefined()
})

it('sorts a station across pages, restores the previous order, and exposes page size above the list',async()=>{
  const fixture=nodes().filter(n=>n.profile_id===profileIds[0]).slice(0,25)
  const first=fixture[0]!,last=fixture.at(-1)!,at=new Date(Date.now()-60_000).toISOString(),target='https://speed.cloudflare.com/__down?bytes=1'
  const test=(node:NodeOption,value:number):LatencyTest=>({...node,node_type:node.type,attempt_id:'site-'+node.node_key,requested_at:at,finished_at:at,status:'completed',latency_ms:value,total_samples:2,success_samples:2,failure_samples:0,persistence_state:'saved',samples:[1,2].map(seq=>({seq,target,timestamp:at,latency_ms:value,success:true}))})
  vi.mocked(api.latencyHistory).mockImplementation(async n=>({tests:n.node_key===last.node_key?[test(last,22)]:n.node_key===first.node_key?[test(first,95)]:[],has_more:false,complete:true}))
  const {workspace,wrapper}=attachHome({nodes:fixture});workspace.project.value='latency';await flushPromises()
  const rows=()=>wrapper.findAll('.hd-row-scroll [data-node-key]').map(r=>r.attributes('data-node-key'))
  const original=rows(),button=wrapper.findAll('.hd-column-sort').find(b=>b.text()==='Cloudflare')!
  expect(original).not.toContain(key(last))
  await button.trigger('click');await flushPromises()
  expect(rows().slice(0,2)).toEqual([key(last),key(first)])
  expect(button.element.parentElement?.getAttribute('aria-sort')).toBe('ascending')
  await button.trigger('click');await flushPromises()
  expect(rows()).toEqual(original)
  expect(button.element.parentElement?.getAttribute('aria-sort')).toBe('none')
  const top=wrapper.get('nav[aria-label="节点列表分页"]')
  await top.get('select[aria-label="每页显示节点数"]').setValue('10');await flushPromises()
  expect(rows()).toHaveLength(10)
  expect(top.text()).toContain('第 1–10 条，共 25 条')
  expect(window.localStorage.getItem('speedtest-page-size')).toBe('10')
  await top.get('[aria-label="下一页"]').trigger('click');await flushPromises()
  expect(top.text()).toContain('第 11–20 条，共 25 条')
})

describe('synthetic 256-node home first screen',()=>{
  it('keeps current account notices, expiry, and refresh status visible beside the airport orbit',async()=>{
    const syntheticAirports=airports()
    syntheticAirports[0]!.subscriptions![0]!.usage!.expire=Math.floor(Date.now()/1000)-60
    vi.mocked(api.refreshJobs).mockResolvedValueOnce([refreshJob()])
    const {workspace,wrapper}=attachHome({airports:syntheticAirports,nodes:[...nodes(),noticeNode()]});await flushPromises()
    const overview=wrapper.get('.subscription-overview')
    expect(wrapper.find('details.home-subscription-disclosure').exists()).toBe(false)
    expect(wrapper.findAll('.subscription-airport-row')).toHaveLength(2)
    expect(overview.text()).toContain('缓存')
    expect(overview.text()).toContain('synthetic refresh failure')
    workspace.viewAirport('airport-3');await flushPromises()
    expect(wrapper.get('.subscription-airport-heading').text()).toContain('合成机场 3')
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
    // 失败只留灰色“上次刷新 HH:MM”，不再用红色“刷新失败”占据行内状态。
    expect(interrupted.get('.airport-refresh-note').text()).toContain('上次刷新')
    expect(interrupted.get('.subscription-airport-row').text()).toContain('缓存')
    expect(interrupted.get('.subscription-issues summary').text()).toBe('刷新异常 1 项')

    vi.mocked(api.refreshJobs).mockResolvedValueOnce([refreshJob('succeeded','2026-10-11T12:00:00Z'),refreshJob('interrupted')])
    const recovered=attachHome().wrapper;await flushPromises()
    expect(recovered.get('.subscription-airport-row').text()).toContain('正常')
    expect(recovered.find('.subscription-issues').exists()).toBe(false)
  })

  it('shows current account success and whole job progress together',async()=>{
    vi.mocked(api.refreshJobs).mockResolvedValueOnce([runningRefreshJob()])
    const {wrapper}=attachHome();await flushPromises()

    expect(wrapper.get('.subscription-actions button').attributes('disabled')).toBeDefined()
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

  it('keeps the status strip visible while cycling the sidebar airport filter',async()=>{
    const {workspace,wrapper}=attachHome();await flushPromises()
    expect(wrapper.find('details.home-subscription-disclosure').exists()).toBe(false)
    expect(wrapper.find('.subscription-overview').exists()).toBe(true)
    expect(wrapper.get('.subscription-airport-heading').text()).toContain('合成机场 1')
    expect(wrapper.get('.hd-foot').text()).toContain('64 个节点')

    const selectedAirports=[...workspace.selectedAirportIds.value!]
    workspace.viewAirport('airport-3');await flushPromises()
    expect(wrapper.get('.subscription-airport-heading').text()).toContain('合成机场 3')
    expect(workspace.selectedAirportIds.value).toEqual(selectedAirports)
    expect(workspace.filteredNodes.value).toHaveLength(64)
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
  it('changes the viewed airport directly from the sidebar without widening the chosen scope',async()=>{
    const {workspace,wrapper}=attachHome()
    const selectedAirports=['airport-1','airport-3']
    workspace.setAirports(selectedAirports)
    await flushPromises()
    const selectedNode=workspace.filteredNodes.value[0]!
    workspace.toggleNode(selectedNode)
    expect(wrapper.find('select[aria-label="查看机场"]').exists()).toBe(false)
    const pickButton=(label:string)=>wrapper.findAll('.sidebar-airport-pick').find(b=>b.text().includes(label))!
    expect((wrapper.get('input[aria-label="纳入范围 合成机场 1"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('input[aria-label="纳入范围 合成机场 2"]').element as HTMLInputElement).checked).toBe(false)
    await pickButton('合成机场 3').trigger('click')
    expect(workspace.filteredNodes.value).toHaveLength(64)
    expect(workspace.selectedAirportIds.value).toEqual(selectedAirports)
    await wrapper.findAll('.hd-title button').find(button=>button.text()==='看全部 2 个机场')!.trigger('click')
    expect(workspace.selectedAirportIds.value).toEqual(selectedAirports)
    expect(workspace.filteredNodes.value).toHaveLength(128)
    expect(workspace.selectedNodes.value).toEqual([selectedNode])
    await wrapper.get('input[aria-label="纳入范围 合成机场 2"]').setValue(true)
    expect(workspace.selectedAirportIds.value).toEqual(['airport-1','airport-3','airport-2'])
    expect(api.startLatency).not.toHaveBeenCalled()
    expect(api.startRefresh).not.toHaveBeenCalled()
  })

  it('shows airport summary rows when switching to all airports',async()=>{
    const {workspace,wrapper}=attachHome();workspace.viewAirport('airport-2');await flushPromises()
    workspace.viewAirport('all');await flushPromises()
    expect(wrapper.find('.hd-orbit').exists()).toBe(false)
    expect(wrapper.findAll('.subscription-airport-row')).toHaveLength(4)
    expect(wrapper.find('select[aria-label="首页用量订阅"]').exists()).toBe(false)
    expect(wrapper.findAll('.subscription-actions .button').map(b=>b.text())).toEqual(['全部刷新'])
    expect(workspace.airportFilter.value).toBe('all')
    await wrapper.get('.subscription-airport-row button').trigger('click');await flushPromises()
    expect(workspace.airportFilter.value).toBe('airport-1')
  })

  it('switches the source strip when the sidebar filter changes and follows the orbit while filtered',async()=>{
    const {workspace,wrapper}=attachHome();await flushPromises()
    expect(wrapper.get('.subscription-airport-heading strong').text()).toBe('合成机场 1')
    workspace.viewAirport('airport-3');await flushPromises()
    expect(wrapper.get('.subscription-airport-heading strong').text()).toBe('合成机场 3')
    // These two accounts contain different identities, so both inventories count.
    expect(workspace.filteredNodes.value).toHaveLength(64)
    expect(wrapper.get('.hd-title p').text()).toContain('合成机场 3 · 64 个节点')
    expect(wrapper.get('.hd-title p').text()).toContain('看全部 4 个机场')
    workspace.viewAirport('airport-4');await flushPromises()
    expect(workspace.airportFilter.value).toBe('airport-4')
    expect(wrapper.get('.subscription-airport-heading strong').text()).toBe('合成机场 4')
    await wrapper.findAll('.hd-title button').find(b=>b.text().includes('看全部'))!.trigger('click');await flushPromises()
    expect(workspace.airportFilter.value).toBe('all')
    expect(workspace.filteredNodes.value).toHaveLength(256)
    expect(wrapper.get('.hd-title p').text()).toContain('256 个节点')
  })
})

it('clears all node filters and distinguishes an airport with no nodes',async()=>{
 const {wrapper,workspace}=attachHome();workspace.search.value='missing';workspace.region.value='US';workspace.excludeRegion.value=true;workspace.subscription.value='missing';workspace.downloadFullOnly.value=true
 await flushPromises();expect(wrapper.get('.hd-empty').text()).toContain('节点被筛选条件隐藏了');await wrapper.get('.hd-empty button').trigger('click');expect(workspace.search.value).toBe('');expect(workspace.region.value).toBe('all');expect(workspace.excludeRegion.value).toBe(false);expect(workspace.subscription.value).toBe('all');expect(workspace.downloadFullOnly.value).toBe(false)
 workspace.nodes.value=[];await flushPromises();expect(wrapper.get('.hd-empty').text()).toContain('这个机场还没有节点');await wrapper.get('.hd-empty button').trigger('click');expect(workspace.page.value).toBe('airports')
})

it('offers adding a subscription directly when the initialized store has no airports',async()=>{
 const {wrapper,workspace}=attachHome({airports:[],nodes:[]});await flushPromises()
 expect(wrapper.get('.hd-empty').text()).toContain('先添加一份机场订阅');expect(wrapper.get('.hd-empty button').text()).toBe('添加机场')
 await wrapper.get('.hd-empty button').trigger('click');expect(workspace.page.value).toBe('airports');expect(wrapper.find('.airport-pick-list').exists()).toBe(false)
})
