import {expect,it,vi} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import AirportsView from '../components/AirportsView.vue'
import DataView from '../components/DataView.vue'
import UiSelect from '../components/UiSelect.vue'
import {api} from '../api'
import {createWorkspace,workspaceKey} from '../workspace'

it('associates a Verge subscription with the chosen existing account and keeps it for later sync',async()=>{
 HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
 HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
 const list=vi.spyOn(api,'vergeSubscriptions').mockResolvedValue([{uid:'remote-a',name:'外部订阅',updated_at:'2026-11-02T10:00:00Z',linked:false,url_changed:false}])
 const sync=vi.spyOn(api,'importVergeSubscription').mockResolvedValue({id:'sub-a',airport_id:'airport-a',node_count:3} as never)
 const jobs=vi.spyOn(api,'refreshJobs').mockResolvedValue([])
 const w=createWorkspace();w.airports.value=[{id:'airport-a',name:'机场A',subscriptions:[{id:'sub-a',airport_id:'airport-a',name:'账户A'}]}] as never
 const refresh=vi.spyOn(w,'refreshNodes').mockResolvedValue(undefined)
 const wrapper=mount(AirportsView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w},stubs:{UsageDays:true,SubscriptionChanges:true}}})
 try{
  await wrapper.findAll('button').find(button=>button.text()==='从 Clash Verge 导入／同步')!.trigger('click')
  await flushPromises()
  const button=()=>Array.from(document.body.querySelectorAll<HTMLButtonElement>('button')).find(b=>b.textContent?.trim()==='导入'||b.textContent?.trim()==='同步')!
  expect(button().disabled).toBe(true)
  wrapper.findComponent(UiSelect).vm.$emit('update:modelValue','airport-a:sub-a');await flushPromises()
  button().click();await flushPromises()
  expect(sync).toHaveBeenLastCalledWith({uid:'remote-a',airport_id:'airport-a',subscription_id:'sub-a'})
  expect(document.body.textContent).toContain('已同步 3 个节点')
  expect(wrapper.findComponent(UiSelect).props('disabled')).toBe(true)
  const nameInput=document.body.querySelector<HTMLInputElement>('input.verge-name-input')!
  nameInput.value='自定义同步名称';nameInput.dispatchEvent(new Event('input',{bubbles:true}));await flushPromises()
  button().click();await flushPromises()
  expect(sync).toHaveBeenCalledTimes(2)
  expect(sync).toHaveBeenLastCalledWith({uid:'remote-a',airport_id:'airport-a',subscription_id:'sub-a',name:'自定义同步名称'})
  expect(document.querySelector('.verge-name strong')?.textContent).toBe('外部订阅')
  expect(refresh).toHaveBeenCalled()
 }finally{wrapper.unmount();w.dispose();list.mockRestore();sync.mockRestore();jobs.mockRestore();refresh.mockRestore()}
})

it('keeps the binding and reports import success when only the subsequent list reload fails',async()=>{
 HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
 HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
 const list=vi.spyOn(api,'vergeSubscriptions').mockResolvedValue([{uid:'remote-reload',name:'源订阅',updated_at:'2026-11-18T01:00:00Z',linked:false,url_changed:false}])
 const sync=vi.spyOn(api,'importVergeSubscription').mockResolvedValue({id:'sub-new',airport_id:'airport-new',name:'目标账户',node_count:4} as never)
 const jobs=vi.spyOn(api,'refreshJobs').mockResolvedValue([])
 const w=createWorkspace();w.airports.value=[]
 const refresh=vi.spyOn(w,'refreshNodes').mockRejectedValue(new Error('synthetic reload unavailable'))
 const notify=vi.spyOn(w,'notify')
 const wrapper=mount(AirportsView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w},stubs:{UsageDays:true,SubscriptionChanges:true}}})
 try{
  await wrapper.findAll('button').find(b=>b.text()==='从 Clash Verge 导入／同步')!.trigger('click');await flushPromises()
  Array.from(document.body.querySelectorAll<HTMLButtonElement>('button')).find(b=>b.textContent?.trim()==='导入')!.click();await flushPromises()
  expect(document.querySelector('[role=alert]')?.textContent).toContain('已导入，但列表刷新失败')
  expect(document.body.textContent).toContain('已同步 4 个节点')
  expect(notify).toHaveBeenCalledWith('已导入「目标账户」的 4 个节点')
  expect(wrapper.findComponent(UiSelect).props('modelValue')).toBe('airport-new:sub-new')
  expect(wrapper.findComponent(UiSelect).props('disabled')).toBe(true)
  expect(document.querySelector('.verge-name strong')?.textContent).toBe('源订阅')
 }finally{wrapper.unmount();w.dispose();list.mockRestore();sync.mockRestore();jobs.mockRestore();refresh.mockRestore();notify.mockRestore()}
})

it('imports a Verge subscription directly from initial setup in DataView and boots workspace',async()=>{
 HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
 HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
 const list=vi.spyOn(api,'vergeSubscriptions').mockResolvedValue([{uid:'remote-init',name:'初始订阅',updated_at:'2026-11-15T03:00:00Z',linked:false,url_changed:false}])
 const sync=vi.spyOn(api,'importVergeSubscription').mockResolvedValue({id:'sub-init',airport_id:'airport-init',node_count:5} as never)
 const w=createWorkspace();w.setup.value={state:'needs_choice',initialized:false,data_root:'/tmp/cst',profile_dir:'/tmp/cst/profiles',history_dir:'/tmp/cst/history',settings_file:'/tmp/cst/settings.json',sources:[]} as never
 const boot=vi.spyOn(w,'boot').mockImplementation(async()=>{w.setup.value={...w.setup.value!,state:'ready',initialized:true}})
 const wrapper=mount(DataView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 try{
  await wrapper.findAll('button').find(button=>button.text()==='从 Clash Verge 导入／同步')!.trigger('click')
  await flushPromises()
  const button=()=>Array.from(document.body.querySelectorAll<HTMLButtonElement>('button')).find(b=>b.textContent?.trim()==='导入'||b.textContent?.trim()==='同步')!
  expect(button().disabled).toBe(false)
  button().click();await flushPromises()
  expect(sync).toHaveBeenCalledWith({uid:'remote-init'})
  expect(boot).toHaveBeenCalledOnce()
  expect(document.body.textContent).toContain('已同步 5 个节点')
 }finally{wrapper.unmount();w.dispose();list.mockRestore();sync.mockRestore();boot.mockRestore()}
})

it('shows quota usage, node count, and duplicate tag to distinguish same-name Verge subscriptions',async()=>{
 HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
 HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
 const list=vi.spyOn(api,'vergeSubscriptions').mockResolvedValue([
  {uid:'dup-1',name:'同名机场',updated_at:'2026-10-23T12:31:00Z',node_count:42,usage:{upload:0,download:589*1073741824,total:600*1073741824},linked:false,url_changed:false},
  {uid:'dup-2',name:'同名机场',updated_at:'2026-11-19T04:40:00Z',node_count:68,usage:{upload:0,download:162*1073741824,total:600*1073741824},linked:false,url_changed:false}
 ])
 const w=createWorkspace();w.airports.value=[{id:'a1',name:'同名机场',node_count:42,has_cache:true,url_display:'',subscriptions:[{id:'s1',airport_id:'a1',name:'同名机场',node_count:42,has_cache:true,url_display:'',url_configured:true,usage:{upload:0,download:589*1073741824,total:600*1073741824}}]}] as never
 const wrapper=mount(AirportsView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w},stubs:{UsageDays:true,SubscriptionChanges:true}}})
 try{
  await wrapper.findAll('button').find(b=>b.text()==='从 Clash Verge 导入／同步')!.trigger('click')
  await flushPromises()
  const text=document.body.textContent||''
  expect(text).toContain('同名订阅')
  expect(text).toContain('42 个节点')
  expect(text).toContain('68 个节点')
  expect(text).toContain('589.00 GiB / 600.00 GiB')
  expect(text).toContain('162.00 GiB / 600.00 GiB')
  expect(document.querySelectorAll('.verge-usage-meter')).toHaveLength(2)
 }finally{wrapper.unmount();w.dispose();list.mockRestore()}
})
