import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import AirportsView from '../components/AirportsView.vue'
import {api,request} from '../api'
import {createWorkspace,workspaceKey} from '../workspace'
import type {Airport,Subscription} from '../domain'

vi.mock('../api',async importOriginal=>({...await importOriginal<typeof import('../api')>(),request:vi.fn()}))
let w:ReturnType<typeof createWorkspace>,view:VueWrapper
const account=():Subscription=>({id:'sub-a',airport_id:'airport-a',name:'账户甲',note:'',url_display:'https://example.invalid/•••',url_configured:true,node_count:2,has_cache:true})
const airport=():Airport=>({id:'airport-a',name:'机场甲',note:'仅属于机场的备注',url_display:'https://example.invalid/•••',node_count:2,has_cache:true,subscriptions:[account()]})
function button(text:string){const found=[...document.querySelectorAll<HTMLButtonElement>('button')].find(b=>b.textContent?.trim()===text);expect(found,`button ${text}`).toBeTruthy();return found!}
async function click(text:string){button(text).click();await flushPromises()}
async function input(name:string,value:string){const el=document.querySelector<HTMLInputElement|HTMLTextAreaElement>(`dialog [name="${name}"]`)!;expect(el,`input ${name}`).toBeTruthy();el.value=value;el.dispatchEvent(new Event('input',{bubbles:true}));await flushPromises()}
function submit(){document.querySelector('dialog form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}))}
beforeEach(()=>{
 HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')};HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
 vi.mocked(request).mockReset();vi.spyOn(api,'refreshJobs').mockResolvedValue([])
 w=createWorkspace();w.airports.value=[airport()];vi.spyOn(w,'refreshNodes').mockResolvedValue();vi.spyOn(w,'notify');vi.spyOn(w,'fail')
 view=mount(AirportsView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w},stubs:{UsageDays:true,SubscriptionChanges:true}}})
})
afterEach(()=>{view.unmount();w.dispose();vi.restoreAllMocks();document.body.innerHTML='';localStorage.clear()})

it('edits provider metadata without revealing or resubmitting an account source',async()=>{
 await view.get('[aria-label="编辑机场"]').trigger('click');await flushPromises()
 expect(request).not.toHaveBeenCalled();expect(document.querySelector('dialog [name=url]')).toBeNull()
 await input('name','机场新名');submit();await flushPromises()
 expect(request).toHaveBeenCalledWith('/api/airports/airport-a','PUT',{name:'机场新名',url:'',website_url:'',backup_url:'',note:'仅属于机场的备注'},'UpdateFrontendAirport',['airport-a',{name:'机场新名',url:'',website_url:'',backup_url:'',note:'仅属于机场的备注'}])
 expect(document.querySelector('dialog')).toBeNull();expect(w.notify).toHaveBeenCalledWith('机场资料已保存')
})
it('keeps an empty account note separate from its provider and preserves a failed draft',async()=>{
 vi.mocked(request).mockResolvedValueOnce({url:'https://example.invalid/sub?token=synthetic'})
 await view.get('[aria-label="编辑订阅"]').trigger('click');await flushPromises()
 expect((document.querySelector('dialog [name=note]') as HTMLTextAreaElement).value).toBe('')
 expect(document.querySelector('dialog')?.textContent).toContain('机场甲')
 vi.mocked(request).mockRejectedValueOnce(new Error('synthetic save denied'))
 await input('name','账户新名');submit();await flushPromises()
 expect(document.querySelector('dialog [role=alert]')?.textContent).toContain('synthetic save denied')
 expect((document.querySelector('dialog [name=name]') as HTMLInputElement).value).toBe('账户新名')
 vi.mocked(request).mockResolvedValueOnce({...account(),name:'账户新名'});submit();await flushPromises()
 expect(request).toHaveBeenLastCalledWith('/api/airports/airport-a/subscriptions/sub-a','PUT',{name:'账户新名',url:'https://example.invalid/sub?token=synthetic',note:''},'UpdateSubscription',['airport-a','sub-a','账户新名','https://example.invalid/sub?token=synthetic',''])
 expect(document.querySelector('dialog')).toBeNull()
})
it('locks the submitted draft and rejects duplicate submit while saving',async()=>{
 await view.get('[aria-label="编辑机场"]').trigger('click');await flushPromises()
 let finish!:(v:unknown)=>void;vi.mocked(request).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
 submit();await flushPromises();submit();await flushPromises()
 expect(request).toHaveBeenCalledTimes(1);expect(document.querySelector('dialog [name=name]')?.matches(':disabled')).toBe(true)
 expect(button('取消').disabled).toBe(true);expect(document.querySelector('dialog [aria-label=关闭]')).toBeNull()
 finish(airport());await flushPromises();expect(document.querySelector('dialog')).toBeNull()
})
it('allows cancelling a source read and ignores its late answer after opening another draft',async()=>{
 let finish!:(v:unknown)=>void;vi.mocked(request).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
 await view.get('[aria-label="编辑订阅"]').trigger('click');await flushPromises()
 expect(document.querySelector('dialog [role=status]')?.textContent).toContain('读取')
 await click('取消');await click('添加机场');await input('name','新机场草稿')
 finish({url:'https://example.invalid/late'});await flushPromises()
 expect((document.querySelector('dialog [name=name]') as HTMLInputElement).value).toBe('新机场草稿')
 expect((document.querySelector('dialog [name=url]') as HTMLInputElement).value).toBe('')
})
it('reports committed save separately from a failed list reload',async()=>{
 await view.get('[aria-label="编辑机场"]').trigger('click');await flushPromises()
 vi.mocked(request).mockResolvedValueOnce(airport());vi.mocked(w.refreshNodes).mockRejectedValueOnce(new Error('synthetic list unavailable'))
 submit();await flushPromises()
 expect(document.querySelector('dialog')).toBeNull();expect(w.notify).toHaveBeenCalledWith('机场资料已保存')
 expect(String(vi.mocked(w.fail).mock.calls[0]?.[0])).toContain('已保存，但列表刷新失败')
 expect(request).toHaveBeenCalledTimes(1)
})
it('shows the newly created provider even when the ledger was filtered',async()=>{
 await view.get('[aria-label="搜索机场"]').setValue('机场甲');await click('添加机场')
 await input('name','机场乙');await input('url','https://example.invalid/new')
 const created={...airport(),id:'airport-b',name:'机场乙'}
 vi.mocked(request).mockResolvedValueOnce(created);vi.mocked(w.refreshNodes).mockImplementationOnce(async()=>{w.airports.value.push(created)})
 submit();await flushPromises()
 expect(view.get('.cst-ledger-title h2').text()).toBe('机场乙');expect((view.get('[aria-label="搜索机场"]').element as HTMLInputElement).value).toBe('')
})
it('saves and clears one account User-Agent without fetching its source or refreshing nodes',async()=>{
 await click('请求设置');await input('user_agent','FixtureClient/1.0')
 vi.mocked(request).mockResolvedValueOnce({...account(),user_agent:'FixtureClient/1.0'})
 submit();await flushPromises()
 expect(request).toHaveBeenLastCalledWith('/api/airports/airport-a/subscriptions/sub-a/user-agent','PUT',{user_agent:'FixtureClient/1.0'},'SetSubscriptionUserAgent',['airport-a','sub-a','FixtureClient/1.0'])
 expect(w.airports.value[0].subscriptions![0].user_agent).toBe('FixtureClient/1.0');expect(w.refreshNodes).not.toHaveBeenCalled()
 await click('请求设置');await input('user_agent','');vi.mocked(request).mockResolvedValueOnce({...account(),user_agent:''});submit();await flushPromises()
 expect(request).toHaveBeenCalledTimes(2);expect(w.airports.value[0].subscriptions![0].user_agent).toBe('');expect(w.refreshNodes).not.toHaveBeenCalled()
})
it('preserves legacy source editing while allowing metadata edits for an empty provider',async()=>{
 w.airports.value=[{...airport(),subscriptions:[]}];await flushPromises()
 vi.mocked(request).mockResolvedValueOnce({url:'https://example.invalid/legacy'})
 await view.get('[aria-label="编辑机场"]').trigger('click');await flushPromises()
 expect((document.querySelector('dialog [name=url]') as HTMLInputElement).value).toBe('https://example.invalid/legacy')
 expect(request).toHaveBeenCalledWith('/api/airports/airport-a/url','GET',undefined,'GetAirportURL',['airport-a'])
 await click('取消');vi.mocked(request).mockClear()
 w.airports.value=[{...airport(),subscriptions:[],url_display:''}];await flushPromises()
 await view.get('[aria-label="编辑机场"]').trigger('click');await flushPromises()
 expect(request).not.toHaveBeenCalled();expect(document.querySelector('dialog [name=url]')).toBeNull()
 expect(document.querySelector('dialog [name=name]')?.matches(':disabled')).toBe(false)
})
