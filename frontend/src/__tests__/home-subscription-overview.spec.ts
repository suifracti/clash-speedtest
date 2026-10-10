import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
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
  it('mounts the account overview and scopes quota, failure, and announcements to its selected account',async()=>{
    const workspace=createWorkspace()
    workspace.airports.value=[airport]
    workspace.nodes.value=[notice('account-a1','notice-a1','A1 only'),notice('account-a2','notice-a2','A2 only')]
    workspace.selectedAirportIds.value=['airport-a']
    workspace.subscription.value='all'
    refreshMocks.refreshJobs.mockResolvedValue([{id:'prior-job',state:'finished',created_at:'2026-10-10T00:00:00Z',completed:1,success:0,failed:1,cancelled:0,not_executed:0,cancel_requested:false,items:[{airport_id:'airport-a',subscription_id:'account-a2',airport_name:'Airport A',subscription_name:'Account A2',source_fingerprint:'fingerprint',source_version:1,state:'failed',error_code:'http_429',error_message:'订阅源限流；旧缓存保留',duration_ms:1,node_count:1,stages:[]}]}])
    wrapper=mount(HomeView,{global:{provide:{[workspaceKey as symbol]:workspace},stubs:{QualityMetrics:true,HealthBars:true,ServicePicker:true,TrendChart:true,RoundResults:true,MeasurementResults:true,InlineHistory:true,SiteDisplayPicker:true}}})
    await flushPromises()

    const accountSelect=wrapper.find('select[aria-label="首页用量订阅"]')
    expect(accountSelect.exists()).toBe(true)
    expect(Array.from((accountSelect.element as HTMLSelectElement).options).map(option=>option.value)).toEqual([JSON.stringify(['airport-a','account-a1']),JSON.stringify(['airport-a','account-a2'])])
    await accountSelect.setValue(JSON.stringify(['airport-a','account-a2']))
    expect(wrapper.text()).toContain('Airport A · Account A2')
    expect(wrapper.text()).toContain('3.0 KiB')
    expect(wrapper.text()).toContain('已过期')
    expect(wrapper.text()).toContain('订阅源限流；旧缓存保留')
    expect(workspace.selectedAirportIds.value).toEqual(['airport-a'])
    expect(workspace.subscription.value).toBe('all')

    expect(wrapper.find('a[href="https://official.example/"]').exists()).toBe(true)
    expect(wrapper.find('a[href="https://t.me/airport_channel"]').exists()).toBe(true)

    refreshMocks.startRefresh.mockResolvedValue({id:'selected-job',state:'finished',created_at:'2026-10-11T00:00:00Z',completed:1,success:0,failed:1,cancelled:0,not_executed:0,items:[{airport_id:'airport-a',subscription_id:'account-a2',state:'failed',error_message:'A2 refresh result'}]})
    const refreshCurrent=wrapper.findAll('button').find(button=>button.text().trim()==='刷新当前订阅')
    expect(refreshCurrent).toBeTruthy()
    await refreshCurrent!.trigger('click')
    await flushPromises()
    expect(refreshMocks.startRefresh).toHaveBeenCalledWith(expect.objectContaining({selections:[{airport_id:'airport-a',subscription_id:'account-a2'}]}))
    expect(wrapper.text()).toContain('A2 refresh result')
    await accountSelect.setValue(JSON.stringify(['airport-a','account-a1']))
    expect(wrapper.text()).not.toContain('A2 refresh result')
    await accountSelect.setValue(JSON.stringify(['airport-a','account-a2']))

    const noticeButton=wrapper.findAll('button').find(button=>button.text().trim()==='公告 1')
    expect(noticeButton).toBeTruthy()
    await noticeButton!.trigger('click')
    await flushPromises()
    const dialog=document.body.querySelector('dialog.modal')
    expect(dialog?.textContent).toContain('A2 only')
    expect(dialog?.textContent).not.toContain('A1 only')
  })
})
