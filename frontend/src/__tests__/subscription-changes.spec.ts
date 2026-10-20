import {expect,it,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import SubscriptionChanges from '../components/SubscriptionChanges.vue'
import {request} from '../api'
vi.mock('../api',()=>({request:vi.fn()}))
it('opens the node change list and reports a read failure',async()=>{
 HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()
 const changes={added:['New'],removed:['Offline'],renamed:[{from:'Before',to:'After'}]}
 vi.mocked(request).mockResolvedValue(changes)
 const wrapper=mount(SubscriptionChanges,{props:{airportId:'a',subscription:{id:'s',airport_id:'a',name:'Account',url_display:'',url_configured:true,node_count:2,has_cache:true,node_changes:changes}},attachTo:document.body})
 expect(wrapper.text()).toContain('新增 1 / 下线 1 / 改名 1');await wrapper.get('button').trigger('click');await flushPromises();expect(document.querySelector('dialog')?.textContent).toContain('Before → After')
 document.querySelector<HTMLButtonElement>('button[aria-label="关闭"]')!.click();await flushPromises();vi.mocked(request).mockRejectedValue(new Error('read failed'));await wrapper.get('button').trigger('click');await flushPromises();expect(document.querySelector('[role="alert"]')?.textContent).toBe('read failed');wrapper.unmount();document.body.innerHTML=''
})
