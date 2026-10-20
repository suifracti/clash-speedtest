import {expect,it,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import AirportSidebar from '../components/AirportSidebar.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
vi.mock('../api',async original=>({...await original<typeof import('../api')>(),api:{batch:vi.fn()}}))
it('searches and collapses airports, shows batch failures and handles read failure',async()=>{
 Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})
 const w=createWorkspace();w.airports.value=[{id:'a',name:'Airport A',node_count:2,has_cache:true,url_display:'',subscriptions:[{id:'p',airport_id:'a',name:'P',node_count:2,has_cache:true,url_display:'',url_configured:true}]}]
 const batch={batch_id:'b',request_id:'r',state:'completed',requested_at:'2026-10-20T10:00:00Z',item_count:2}
 vi.mocked(api.batch).mockResolvedValue({...batch,items:[{profile_id:'p',result:{total_samples:1,success_samples:0}},{profile_id:'p',result:{total_samples:1,success_samples:1}}]} as any);w.batches.value=[batch]
 const wrapper=mount(AirportSidebar,{global:{provide:{[workspaceKey as symbol]:w}}});await flushPromises();expect(wrapper.find('.airport-health.bad').exists()).toBe(true)
 await wrapper.get('input').setValue('missing');expect(wrapper.text()).toContain('没有匹配机场');await wrapper.get('input').setValue('Airport');expect(wrapper.text()).toContain('Airport A');await wrapper.get('.airport-heading').trigger('click');expect(wrapper.find('input').exists()).toBe(false)
 await wrapper.get('.airport-heading').trigger('click');vi.mocked(api.batch).mockRejectedValue(new Error('read failed'));w.batches.value=[{...batch,batch_id:'unread'}];await flushPromises();expect(wrapper.find('.airport-health.empty').exists()).toBe(true);wrapper.unmount();w.dispose()
})
