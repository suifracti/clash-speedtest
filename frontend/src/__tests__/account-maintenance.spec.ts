import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import AirportsView from '../components/AirportsView.vue'
import {api,request} from '../api'
import {createWorkspace,workspaceKey} from '../workspace'
import type {Airport} from '../domain'

vi.mock('../api',async importOriginal=>({...await importOriginal<typeof import('../api')>(),request:vi.fn()}))
let w:ReturnType<typeof createWorkspace>,view:VueWrapper
const month=()=>`${new Date().getFullYear()}-${String(new Date().getMonth()+1).padStart(2,'0')}`
const airport=():Airport=>({id:'airport-a',name:'机场甲',url_display:'example.invalid',node_count:2,has_cache:true,subscriptions:[{id:'sub-a',airport_id:'airport-a',name:'账户甲',url_display:'example.invalid',url_configured:true,node_count:2,has_cache:true}],maintenance:{refresh_hours:5,next_refresh:'2026-12-01T10:00:00Z',last_result:'定时刷新完成',links:[{label:'月度活动',url:'https://example.invalid/activity',monthly_day:1,done_month:'2025-01'},{label:'公告',url:'https://example.invalid/news',monthly_day:0}]}})
function button(text:string){const found=[...document.querySelectorAll<HTMLButtonElement>('button')].find(b=>b.textContent?.trim()===text);expect(found,`button ${text}`).toBeTruthy();return found!}
async function click(text:string){button(text).click();await flushPromises()}
function submit(){document.querySelector('dialog form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}))}
beforeEach(()=>{
 HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')};HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
 vi.mocked(request).mockReset();vi.spyOn(api,'refreshJobs').mockResolvedValue([])
 w=createWorkspace();w.airports.value=[airport()];vi.spyOn(w,'refreshNodes').mockResolvedValue();vi.spyOn(w,'refreshJobs').mockResolvedValue();vi.spyOn(w,'notify');vi.spyOn(w,'fail')
 w.jobs.value=[{id:'affected',name:'账户甲定点观察',profile_id:'sub-a'},{id:'other',name:'另一个账户任务',profile_id:'sub-b'}] as never
 view=mount(AirportsView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w},stubs:{UsageDays:true,SubscriptionChanges:true}}})
})
afterEach(()=>{view.unmount();w.dispose();vi.restoreAllMocks();vi.useRealTimers();document.body.innerHTML='';localStorage.clear()})

it('restores scheduled status and month completion without changing other links or starting a refresh',async()=>{
 expect(view.text()).toContain('下次更新');expect(view.text()).toContain('定时刷新完成');expect(view.text()).toContain('本月待处理')
 await click('标记完成')
 const sent=vi.mocked(request).mock.calls[0]
 expect(sent[0]).toBe('/api/airports/airport-a/maintenance');expect(sent[3]).toBe('SaveAirportMaintenance')
 expect(sent[2]).toMatchObject({refresh_hours:5,links:[{label:'月度活动',done_month:month()},{label:'公告',monthly_day:0}]})
 expect(view.text()).toContain('本月已完成');expect(api.refreshJobs).toHaveBeenCalledTimes(1)
 await click('撤销完成');expect(vi.mocked(request).mock.calls[1][2]).toMatchObject({links:[{done_month:''},{label:'公告'}]});expect(view.text()).toContain('本月待处理')
})
it('makes a completed monthly task due again after the calendar month changes without any request',async()=>{
 view.unmount();vi.useFakeTimers();vi.setSystemTime(new Date(2026,10,18,12))
 w.airports.value[0].maintenance!.links[0].done_month='2026-11'
 view=mount(AirportsView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w},stubs:{UsageDays:true,SubscriptionChanges:true}}})
 await flushPromises();expect(view.text()).toContain('本月已完成')
 vi.setSystemTime(new Date(2026,11,1,12));await vi.advanceTimersByTimeAsync(60_000)
 expect(view.text()).toContain('本月待处理');expect(request).not.toHaveBeenCalled()
})
it('locks maintenance drafts and closes only after a committed save, keeping inline errors retryable',async()=>{
 await click('更新与维护');expect(document.querySelector('dialog')?.textContent).toContain('机场甲')
 expect(document.querySelector('dialog [role=combobox]')?.textContent).toContain('5')
 let reject!:(e:Error)=>void;vi.mocked(request).mockImplementationOnce(()=>new Promise((_,r)=>{reject=r}))
 submit();await flushPromises();submit();await flushPromises()
 expect(request).toHaveBeenCalledTimes(1);expect(document.querySelector('dialog input')?.matches(':disabled')).toBe(true)
 expect(document.querySelector('dialog [aria-label=关闭]')).toBeNull();expect(button('取消').disabled).toBe(true)
 reject(new Error('synthetic write denied'));await flushPromises()
 expect(document.querySelector('dialog [role=alert]')?.textContent).toContain('synthetic write denied')
 vi.mocked(request).mockResolvedValueOnce(undefined);submit();await flushPromises();expect(document.querySelector('dialog')).toBeNull()
})
it('does not leave an already-saved maintenance form open when the list reload fails',async()=>{
 await click('更新与维护');vi.mocked(w.refreshNodes).mockRejectedValueOnce(new Error('synthetic reload failure'))
 submit();await flushPromises()
 expect(document.querySelector('dialog')).toBeNull();expect(w.notify).toHaveBeenCalledWith('维护设置已保存')
 expect(String(vi.mocked(w.fail).mock.calls[0]?.[0])).toContain('已保存，但列表刷新失败')
})
it('discovers only this airport notice links and adds one without fetching it',async()=>{
 w.nodes.value=[{profile_id:'sub-a',display_name:'官网：https://example.invalid/home',country_code:'OTHER'},{profile_id:'sub-a',display_name:'公告：https://example.invalid/news',country_code:'OTHER'},{profile_id:'sub-a',display_name:'提示：https://user:secret@example.invalid/private',country_code:'OTHER'},{profile_id:'sub-b',display_name:'官网：https://example.invalid/other',country_code:'OTHER'}] as never
 await flushPromises();expect(view.text()).toContain('订阅发现');expect(view.text()).not.toContain('example.invalid/other')
 expect(view.findAll('.maintenance-discovered')).toHaveLength(1)
 await click('加入常用入口')
 expect(request).toHaveBeenCalledTimes(1);expect(vi.mocked(request).mock.calls[0][2]).toMatchObject({links:[{label:'月度活动'},{label:'公告'},{url:'https://example.invalid/home',monthly_day:0}]})
})
it('shows the exact deletion target and affected tasks before making a destructive request',async()=>{
 await view.get('[aria-label="删除订阅"]').trigger('click');await flushPromises()
 const text=document.querySelector('dialog')?.textContent||''
 expect(text).toContain('机场甲');expect(text).toContain('账户甲');expect(text).toContain('账户甲定点观察');expect(text).not.toContain('另一个账户任务')
 expect(text).toContain('历史记录保留');expect(w.refreshJobs).toHaveBeenCalledOnce();expect(request).not.toHaveBeenCalled()
})
it('keeps deletion failure inside the dialog and blocks cancel and duplicates while deleting',async()=>{
 await view.get('[aria-label="删除订阅"]').trigger('click');await flushPromises()
 let reject!:(e:Error)=>void;vi.mocked(request).mockImplementationOnce(()=>new Promise((_,r)=>{reject=r}))
 await click('删除账户');button('删除中…').click();await flushPromises()
 expect(request).toHaveBeenCalledTimes(1);expect(button('保留').disabled).toBe(true);expect(document.querySelector('dialog [aria-label=关闭]')).toBeNull()
 reject(new Error('synthetic delete denied'));await flushPromises()
 expect(document.querySelector('dialog [role=alert]')?.textContent).toContain('synthetic delete denied');expect(w.airports.value[0].subscriptions).toHaveLength(1)
})
it('keeps a committed removal visible and reloads tasks even if the nodes refresh fails',async()=>{
 w.subscription.value='sub-a';await view.get('[aria-label="删除订阅"]').trigger('click');await flushPromises()
 vi.mocked(w.refreshNodes).mockRejectedValueOnce(new Error('nodes unavailable'))
 await click('删除账户')
 expect(request).toHaveBeenCalledWith('/api/airports/airport-a/subscriptions/sub-a','DELETE',undefined,'DeleteSubscription',['airport-a','sub-a'])
 expect(document.querySelector('dialog')).toBeNull();expect(w.airports.value[0].subscriptions).toHaveLength(0);expect(w.subscription.value).toBe('all')
 expect(view.text()).toContain('0 个独立账户');expect(button('全部更新').disabled).toBe(true)
 expect(w.refreshJobs).toHaveBeenCalledTimes(2);expect(w.notify).toHaveBeenCalledWith('账户已删除，历史记录保留')
 expect(String(vi.mocked(w.fail).mock.calls[0]?.[0])).toContain('已删除，但部分列表刷新失败')
})
it('distinguishes unavailable task information from having no affected tasks',async()=>{
 vi.mocked(w.refreshJobs).mockRejectedValueOnce(new Error('jobs unavailable'))
 await view.get('[aria-label="删除机场"]').trigger('click');await flushPromises()
 expect(document.querySelector('dialog')?.textContent).toContain('未能读取关联监测');expect(document.querySelector('dialog')?.textContent).not.toContain('没有关联监测')
 expect(button('删除机场').disabled).toBe(false)
})
