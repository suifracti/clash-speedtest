import {expect,it,vi} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import AirportsView from '../components/AirportsView.vue'
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
  button().click();await flushPromises()
  expect(sync).toHaveBeenCalledTimes(2)
  expect(sync).toHaveBeenLastCalledWith({uid:'remote-a',airport_id:'airport-a',subscription_id:'sub-a'})
  expect(refresh).toHaveBeenCalled()
 }finally{wrapper.unmount();w.dispose();list.mockRestore();sync.mockRestore();jobs.mockRestore();refresh.mockRestore()}
})
