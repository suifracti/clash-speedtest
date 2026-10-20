import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import SubscriptionOverview from '../components/SubscriptionOverview.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import type {Airport,NodeOption} from '../domain'

const refreshMocks=vi.hoisted(()=>({refreshJobs:vi.fn(async():Promise<any[]>=>[]),startRefresh:vi.fn(),refreshJob:vi.fn(),cancelRefresh:vi.fn()}))
vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,...refreshMocks}}
})

let wrapper:VueWrapper|undefined
const airport:Airport={
  id:'airport-a',name:'Airport A',url_display:'',node_count:2,has_cache:true,website_url:'https://official.example',
  maintenance:{refresh_hours:0,links:[{label:'TG',url:'https://t.me/airport_channel',monthly_day:0}]},
  subscriptions:[
    {id:'account-a1',airport_id:'airport-a',name:'Account A1',url_display:'',url_configured:true,node_count:1,has_cache:true,usage:{upload:1024,download:0,total:4096,expire:2000000000}},
    {id:'account-a2',airport_id:'airport-a',name:'Account A2',url_display:'',url_configured:true,node_count:1,has_cache:true,usage:{upload:2048,download:1024,total:8192,expire:1}},
  ],
}
const airportB:Airport={
  id:'airport-b',name:'Airport B',url_display:'',node_count:1,has_cache:true,website_url:'',
  subscriptions:[{id:'account-b1',airport_id:'airport-b',name:'Account B1',url_display:'',url_configured:true,node_count:1,has_cache:true}],
}
const notice=(subscriptionId:string,id:string,text:string):NodeOption=>({profile_id:subscriptionId,node_key:id,node_identity_key:id,config_revision_key:'rev-'+id,profile_name:'Airport A',display_name:`公告：${text}`,type:'',country_code:'',country_flag:''})

beforeEach(()=>{
  window.localStorage.clear()
  window.localStorage.setItem('speedtest-home-view','detailed')
  HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
  HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
  refreshMocks.refreshJobs.mockReset().mockResolvedValue([])
  refreshMocks.startRefresh.mockReset()
  refreshMocks.refreshJob.mockReset()
  refreshMocks.cancelRefresh.mockReset()
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;vi.clearAllMocks()})

describe('HomeView subscription account integration',()=>{
  it('shows one row per subscription and refreshes only the row account',async()=>{
    const workspace=createWorkspace();workspace.airports.value=[airport,airportB];workspace.selectedAirportIds.value=['airport-a','airport-b'];workspace.airportFilter.value='airport-a'
    refreshMocks.startRefresh.mockResolvedValue({id:'one',state:'finished',created_at:'2026-10-20T00:00:00Z',items:[],success:0})
    wrapper=mount(HomeView,{global:{provide:{[workspaceKey as symbol]:workspace}}});await flushPromises()
    expect(wrapper.find('.hd-orbit').exists()).toBe(false)
    expect(wrapper.get('.subscription-airport-heading').text()).toContain('Airport A')
    const rows=wrapper.findAll('.subscription-airport-row');expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('Account A1');expect(rows[1].text()).toContain('Account A2')
    await rows[1].get('button').trigger('click');await flushPromises()
    expect(refreshMocks.startRefresh).toHaveBeenLastCalledWith(expect.objectContaining({selections:[{airport_id:'airport-a',subscription_id:'account-a2'}]}))
    workspace.viewAirport('all');await flushPromises()
    expect(wrapper.findAll('.subscription-airport-row')).toHaveLength(2)
    expect(wrapper.get('.airport-summary-name').text()).toContain('2 个订阅')
    expect(workspace.airportFilter.value).toBe('all');workspace.dispose()
  })
})
