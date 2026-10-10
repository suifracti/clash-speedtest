import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import AirportsView from '../components/AirportsView.vue'
import type {Airport} from '../domain'

const mocks=vi.hoisted(()=>({
  api:{refreshJobs:vi.fn(async()=>[]),startRefresh:vi.fn(),refreshJob:vi.fn(),cancelRefresh:vi.fn()},
  workspace:{airports:{value:[] as Airport[]},maintenanceAirportId:{value:''},page:{value:'airports'},subscription:{value:'all'},setAirports:vi.fn(),refreshNodes:vi.fn(async()=>{}),refreshJobs:vi.fn(async()=>{}),notify:vi.fn(),fail:vi.fn()},
  request:vi.fn(async()=>undefined),
}))
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,request:mocks.request,desktop:()=>false,api:{...actual.api,...mocks.api}}})
vi.mock('../workspace',()=>({useWorkspace:()=>mocks.workspace}))

let wrapper:VueWrapper|undefined
const airports:Airport[]=[{id:'airport-a',name:'A',node_count:2,has_cache:true,url_display:'',subscriptions:[
  {id:'account-a1',airport_id:'airport-a',name:'A1',url_display:'',url_configured:true,node_count:1,has_cache:true},
  {id:'account-a2',airport_id:'airport-a',name:'A2',url_display:'',url_configured:true,node_count:1,has_cache:true},
]}]
const job=(id:string,state:string,items:any[],counts={completed:0,success:0,failed:0,cancelled:0,not_executed:0})=>({id,state,created_at:'2026-10-11T00:00:00Z',items,...counts})
beforeEach(()=>{
  mocks.workspace.airports.value=airports
  mocks.workspace.maintenanceAirportId.value=''
  mocks.workspace.page.value='airports'
  mocks.workspace.subscription.value='all'
  mocks.api.refreshJobs.mockResolvedValue([])
  mocks.api.startRefresh.mockReset()
  mocks.api.refreshJob.mockReset()
  mocks.api.cancelRefresh.mockReset()
  mocks.request.mockReset()
  HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
  HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;document.body.innerHTML='';vi.useRealTimers();vi.clearAllMocks()})

describe('AirportsView manual refresh jobs',()=>{
  it('starts a selected subscription job and exposes progress and cancellation',async()=>{
    const running=job('refresh-running','running',[{airport_id:'airport-a',subscription_id:'account-a2',airport_name:'A',subscription_name:'A2',state:'fetching',stages:[{state:'fetching',at:'2026-10-11T00:00:01Z'}]}])
    mocks.api.startRefresh.mockResolvedValue(running)
    mocks.api.cancelRefresh.mockResolvedValue({...running,state:'cancelled',cancel_requested:true})
    wrapper=mount(AirportsView)
    await flushPromises()
    await wrapper.findAll('button[aria-label="刷新订阅"]')[1]!.trigger('click')
    await flushPromises()

    expect(mocks.api.startRefresh).toHaveBeenCalledWith(expect.objectContaining({selections:[{airport_id:'airport-a',subscription_id:'account-a2'}]}))
    const dialog=document.body.querySelector('dialog.modal')
    expect(dialog?.textContent).toContain('刷新进度')
    expect(dialog?.textContent).toContain('抓取')
    const cancel=dialog?.querySelector('button[aria-label="取消刷新任务"]')
    expect(cancel).toBeTruthy()
    cancel?.dispatchEvent(new MouseEvent('click',{bubbles:true}))
    await flushPromises()
    expect(mocks.api.cancelRefresh).toHaveBeenCalledWith('refresh-running')
  })

  it('retries only the failed items from the visible refresh job',async()=>{
    const failed=job('refresh-failed','finished',[{airport_id:'airport-a',subscription_id:'account-a2',airport_name:'A',subscription_name:'A2',state:'failed',error_code:'http_500',error_message:'server error'}],{completed:1,success:0,failed:1,cancelled:0,not_executed:0})
    mocks.api.startRefresh.mockResolvedValueOnce(failed).mockResolvedValueOnce(job('retry-running','running',[]))
    wrapper=mount(AirportsView)
    await flushPromises()
    await wrapper.findAll('button[aria-label="刷新订阅"]')[1]!.trigger('click')
    await flushPromises()
    const retry=Array.from(document.body.querySelectorAll('button')).find(button=>button.textContent?.includes('只重试失败项'))
    expect(retry).toBeTruthy()
    retry?.dispatchEvent(new MouseEvent('click',{bubbles:true}))
    await flushPromises()
    expect(mocks.api.startRefresh).toHaveBeenNthCalledWith(2,expect.objectContaining({retry_of:'refresh-failed'}))
  })

  it('keeps a dismissed running progress modal closed across polling and provides a reopen entry',async()=>{
    vi.useFakeTimers()
    const running=job('refresh-running','running',[{airport_id:'airport-a',subscription_id:'account-a1',airport_name:'A',subscription_name:'A1',state:'fetching',stages:[{state:'fetching',at:'2026-10-11T00:00:01Z'}]}])
    mocks.api.startRefresh.mockResolvedValue(running)
    mocks.api.refreshJob.mockResolvedValue({...running,items:[{...running.items[0],state:'parsing',stages:[{state:'fetching',at:'2026-10-11T00:00:01Z'},{state:'parsing',at:'2026-10-11T00:00:02Z'}]}]})
    wrapper=mount(AirportsView)
    await flushPromises()
    await wrapper.findAll('button[aria-label="刷新订阅"]')[0]!.trigger('click')
    await flushPromises()
    expect(mocks.api.startRefresh).toHaveBeenCalledTimes(1)
    expect(document.body.querySelector('dialog.modal')).toBeTruthy()

    document.body.querySelector<HTMLButtonElement>('dialog.modal button[aria-label="关闭"]')?.click()
    await flushPromises()
    expect(document.body.querySelector('dialog.modal')).toBeNull()

    const refreshButton=wrapper.find<HTMLButtonElement>('button[aria-label="刷新订阅"]')
    expect(refreshButton.element.disabled).toBe(true)
    await refreshButton.trigger('click')
    await flushPromises()
    expect(mocks.api.startRefresh).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(700)
    await flushPromises()
    expect(document.body.querySelector('dialog.modal')).toBeNull()
    const reopen=wrapper.findAll('button').find(button=>button.text().includes('查看刷新进度'))
    expect(reopen).toBeTruthy()
    await reopen?.trigger('click')
    await flushPromises()
    expect(document.body.querySelector('dialog.modal')?.textContent).toContain('解析中')
  })
})
