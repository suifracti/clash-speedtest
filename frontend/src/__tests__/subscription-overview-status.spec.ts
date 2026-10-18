import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import SubscriptionOverview from '../components/SubscriptionOverview.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import type {Airport} from '../domain'

const refreshMocks=vi.hoisted(()=>({refreshJobs:vi.fn(async():Promise<any[]>=>[]),startRefresh:vi.fn(),refreshJob:vi.fn(),cancelRefresh:vi.fn()}))
vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,...refreshMocks}}
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
  refreshMocks.refreshJobs.mockReset().mockResolvedValue([])
  refreshMocks.startRefresh.mockReset().mockResolvedValue({id:'scope-job',state:'finished',created_at:'2026-10-12T00:00:00Z',completed:0,success:0,failed:0,cancelled:0,not_executed:0,cancel_requested:false,items:[]})
  refreshMocks.refreshJob.mockReset()
  refreshMocks.cancelRefresh.mockReset()
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.clearAllMocks()})

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

  it('sends selected and full workspace refresh scopes without changing measurement scope',async()=>{
    const workspace=createWorkspace()
    workspace.airports.value=[airportA,airportB]
    workspace.selectedAirportIds.value=['airport-a']
    workspace.subscription.value='all'
    wrapper=mount(SubscriptionOverview,{props:{airports:[airportA],notices:[]},global:{provide:{[workspaceKey as symbol]:workspace}}})
    await flushPromises()

    expect(refreshMocks.startRefresh).not.toHaveBeenCalled()
    const accountSelect=wrapper!.get('select[aria-label="首页用量订阅"]')
    await accountSelect.setValue(JSON.stringify(['airport-a','account-a2']))
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
