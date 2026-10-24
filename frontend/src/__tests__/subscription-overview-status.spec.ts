import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import SubscriptionOverview from '../components/SubscriptionOverview.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import type {Airport} from '../domain'

const refreshMocks=vi.hoisted(()=>({refreshJobs:vi.fn(async():Promise<any[]>=>[]),startRefresh:vi.fn(),refreshJob:vi.fn(),cancelRefresh:vi.fn()}))
const usageMocks=vi.hoisted(()=>({accountDailyUsage:vi.fn()}))
vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,...refreshMocks}}
})
vi.mock('../usageDaily',async importOriginal=>{
  const actual=await importOriginal<typeof import('../usageDaily')>()
  return {...actual,accountDailyUsage:usageMocks.accountDailyUsage}
})

let wrapper:VueWrapper|undefined
const airportA:Airport={
  id:'airport-a',name:'Airport A',url_display:'',node_count:2,has_cache:true,website_url:'https://official.example',
  subscriptions:[
    {id:'account-a1',airport_id:'airport-a',name:'Account A1',url_display:'source A1',url_configured:true,node_count:1,has_cache:true,usage:{upload:1024,download:0,total:4096,expire:2000000000}},
    {id:'account-a2',airport_id:'airport-a',name:'Account A2',url_display:'source A2',url_configured:true,node_count:1,has_cache:true,usage:{upload:2048,download:1024,total:8192,expire:1}},
  ],
}
const airportB:Airport={
  id:'airport-b',name:'Airport B',url_display:'',node_count:1,has_cache:true,
  subscriptions:[{id:'account-b1',airport_id:'airport-b',name:'Account B1',url_display:'source B1',url_configured:true,node_count:1,has_cache:true}],
}

beforeEach(()=>{
  window.localStorage.clear()
  usageMocks.accountDailyUsage.mockReset().mockResolvedValue({account:{account_key:'a',airport_id:'airport-a',subscription_id:'account-a1',airport_name:'Airport A',subscription_name:'Account A1',captured_at:'2026-10-20T00:00:00Z',status:'ok',counter_kind:'notice_remaining',upload:null,download:null,source:'subscription'},report:undefined,days:[
    {date:'2026-10-20',label:'今天',bytes:1024,estimated:false,recorded:true},
    {date:'2026-10-19',label:'昨天',bytes:2048,estimated:false,recorded:true},
    {date:'2026-10-18',label:'前天',bytes:null,estimated:false,recorded:false},
  ]})
  refreshMocks.refreshJobs.mockReset().mockResolvedValue([])
  refreshMocks.startRefresh.mockReset().mockResolvedValue({id:'scope-job',state:'finished',created_at:'2026-10-12T00:00:00Z',completed:0,success:0,failed:0,cancelled:0,not_executed:0,cancel_requested:false,items:[]})
  refreshMocks.refreshJob.mockReset()
  refreshMocks.cancelRefresh.mockReset()
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.useRealTimers();vi.clearAllMocks()})

describe('SubscriptionOverview status strip',()=>{
  it('does not expose or draw a usage ratio without a real total',async()=>{
    const noTotal:Airport={...airportB,subscriptions:[{id:'account-b1',airport_id:'airport-b',name:'Account B1',url_display:'',url_configured:true,node_count:1,has_cache:true,usage:{upload:1024,download:2048}}]}
    const workspace=createWorkspace()
    workspace.airports.value=[noTotal]
    wrapper=mount(SubscriptionOverview,{props:{airports:[noTotal],notices:[]},global:{provide:{[workspaceKey as symbol]:workspace}}})
    await flushPromises()

    expect(wrapper!.find('.usage-track').exists()).toBe(false)
    expect((wrapper!.vm as unknown as {usageRatio:number|null}).usageRatio).toBeNull()
  })

  it('groups the compact remaining quota with its meter, links the bar to availability and includes recent daily usage',async()=>{
    const workspace=createWorkspace()
    const single={...airportA,subscriptions:airportA.subscriptions!.slice(0,1)}
    workspace.airports.value=[single]
    wrapper=mount(SubscriptionOverview,{props:{airports:[single],notices:[]},global:{provide:{[workspaceKey as symbol]:workspace}}})
    await flushPromises()

    const meter=wrapper!.get('[role="meter"]')
    expect(wrapper!.get('.subscription-remaining-value').text()).toBe('3.0 KiB')
    expect(wrapper!.get('.subscription-remaining-percent').text()).toBe('75.00% 可用')
    expect(meter.attributes('aria-label')).toBe('套餐剩余比例')
    expect(meter.attributes('aria-valuenow')).toBe('75')
    expect(meter.get('i').attributes('style')).toContain('width: 75%')
    const readout=wrapper!.get('.subscription-quota')
    expect(readout.get('.subscription-quota-used').text()).toBe('已使用1.0 KiB')
    expect(readout.get('.subscription-quota-total').text()).toBe('总量4.0 KiB')
    expect(readout.findAll('.subscription-quota-legend>div')).toHaveLength(2)
    const daily=wrapper!.get('.subscription-main').get('.subscription-daily')
    expect(daily.text()).toContain('近 3 日用量')
    expect(daily.findAll('.cst-usage-day')).toHaveLength(3)
    expect(daily.findAll('.cst-usage-day-track i')).toHaveLength(3)
    expect(usageMocks.accountDailyUsage).toHaveBeenCalledWith('airport-a','account-a1',3)
    expect((wrapper!.vm as unknown as {usageRatio:number|null}).usageRatio).toBe(0.25)
  })

  it('keeps incomplete provider counters unknown instead of drawing a false quota',async()=>{
    const partial:Airport={...airportB,subscriptions:[{...airportB.subscriptions![0],usage:{upload:null,download:2048,total:4096} as any}]}
    const workspace=createWorkspace();workspace.airports.value=[partial]
    wrapper=mount(SubscriptionOverview,{props:{airports:[partial],notices:[]},global:{provide:{[workspaceKey as symbol]:workspace}}})
    await flushPromises()
    expect(wrapper.get('.subscription-remaining-value').text()).toBe('未知')
    expect(wrapper.get('.subscription-quota-used').text()).toBe('已使用未知')
    expect(wrapper.get('.subscription-quota-total').text()).toBe('总量4.0 KiB')
    expect(wrapper.find('[role="meter"]').exists()).toBe(false)
  })

  it('refreshes only the viewed airport with the airport refresh button',async()=>{
    const workspace=createWorkspace();workspace.airports.value=[airportA,airportB]
    wrapper=mount(SubscriptionOverview,{props:{airports:[airportA,airportB],notices:[],airportId:'airport-a'},global:{provide:{[workspaceKey as symbol]:workspace}}})
    await flushPromises()
    const buttons=wrapper.findAll('.subscription-actions .button')
    expect(wrapper.findAll('.subscription-airport-row')).toHaveLength(2)
    expect(wrapper.find('select[aria-label="首页用量订阅"]').exists()).toBe(false)
    expect(buttons.slice(0,2).map(b=>b.text())).toEqual(['刷新此机场','选择范围'])
    await buttons[0]!.trigger('click');await flushPromises()
    expect(refreshMocks.startRefresh).toHaveBeenLastCalledWith(expect.objectContaining({selections:[
      {airport_id:'airport-a',subscription_id:'account-a1'},
      {airport_id:'airport-a',subscription_id:'account-a2'},
    ]}))
  })

  it('keeps a failed refresh in the row as a gray last-refresh note and reuses cache inside thirty minutes',async()=>{
    vi.useFakeTimers({toFake:['Date','setInterval','clearInterval']})
    const recent=new Date(Date.now()-5*60*1000).toISOString()
    const single={...airportA,subscriptions:[{...airportA.subscriptions![0],last_attempt_at:recent,last_attempt_failed:true},airportA.subscriptions![1]!]}
    const workspace=createWorkspace()
    workspace.airports.value=[single]
    wrapper=mount(SubscriptionOverview,{props:{airports:[single],notices:[],airportId:'all'},global:{provide:{[workspaceKey as symbol]:workspace}}})
    await flushPromises()

    expect(wrapper!.get('.airport-refresh-note').text()).toContain('上次刷新')
    expect(wrapper!.get('.subscription-airport-row').text()).toContain('缓存')
    const refreshAll=wrapper!.findAll('button').find(button=>button.text().includes('全部刷新'))!
    expect(refreshAll.attributes('disabled')).toBeDefined()
    expect(wrapper!.text()).toContain('30 分钟内已刷新')
    await refreshAll.trigger('click')
    await flushPromises()
    expect(refreshMocks.startRefresh).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(25*60*1000);await flushPromises()
    expect(refreshAll.attributes('disabled')).toBeUndefined()
    expect(wrapper!.text()).not.toContain('30 分钟内已刷新')
  })

  it('sends selected and full workspace refresh scopes without changing measurement scope',async()=>{
    const workspace=createWorkspace()
    workspace.airports.value=[airportA,airportB]
    workspace.selectedAirportIds.value=['airport-a']
    workspace.subscription.value='all'
    wrapper=mount(SubscriptionOverview,{props:{airports:[airportA],notices:[]},global:{provide:{[workspaceKey as symbol]:workspace}}})
    await flushPromises()

    expect(refreshMocks.startRefresh).not.toHaveBeenCalled()
    expect(workspace.selectedAirportIds.value).toEqual(['airport-a'])
    expect(workspace.subscription.value).toBe('all')

    await wrapper!.get('button[aria-label="选择刷新范围"]').trigger('click')
    await wrapper!.get('input[aria-label="Airport A · Account A2"]').setValue(false)
    await wrapper!.get('input[aria-label="Airport B · Account B1"]').setValue(true)
    const chooseRefresh=wrapper!.findAll('button').find(button=>button.text().includes('刷新所选'))
    expect(chooseRefresh).toBeTruthy()
    await chooseRefresh!.trigger('click')
    await flushPromises()
    expect(refreshMocks.startRefresh).toHaveBeenLastCalledWith(expect.objectContaining({selections:[
      {airport_id:'airport-a',subscription_id:'account-a1'},
      {airport_id:'airport-b',subscription_id:'account-b1'},
    ]}))
    expect(workspace.selectedAirportIds.value).toEqual(['airport-a'])
    expect(workspace.subscription.value).toBe('all')

    await wrapper!.setProps({airportId:'all'})
    const refreshAll=wrapper!.findAll('button').find(button=>button.text().includes('全部刷新'))
    expect(refreshAll).toBeTruthy()
    await refreshAll!.trigger('click')
    await flushPromises()
    expect(refreshMocks.startRefresh).toHaveBeenLastCalledWith(expect.objectContaining({selections:[
      {airport_id:'airport-a',subscription_id:'account-a1'},
      {airport_id:'airport-a',subscription_id:'account-a2'},
      {airport_id:'airport-b',subscription_id:'account-b1'},
    ]}))
    expect(workspace.selectedAirportIds.value).toEqual(['airport-a'])
    expect(workspace.subscription.value).toBe('all')
  })
})
