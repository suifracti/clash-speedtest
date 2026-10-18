import {mount} from '@vue/test-utils'
import {expect,it,vi} from 'vitest'
import NodeActivity from './NodeActivity.vue'
let visibility:(value:boolean)=>void
vi.mock('../visibleArea',()=>({observeVisibleArea:(_:Element,callback:(value:boolean)=>void)=>{visibility=callback;callback(true);return ()=>{}}}))
it('starts only for visible active work and stops at terminal state',async()=>{
 vi.useFakeTimers()
 const w=mount(NodeActivity)
 expect(w.get('.node-activity').isVisible()).toBe(false)
 await w.setProps({state:'queued',label:'等待检测'});expect(w.classes()).not.toContain('active')
 await w.setProps({state:'running',label:'检测中'});expect(w.classes()).toContain('active')
 visibility(false);await w.vm.$nextTick();expect(w.classes()).not.toContain('active')
 visibility(true);await w.setProps({state:'completed',label:'检测已结束'});expect(w.classes()).not.toContain('active');expect(w.classes()).toContain('arrived')
 await vi.advanceTimersByTimeAsync(700);expect(w.classes()).not.toContain('arrived');expect(w.get('.node-activity').isVisible()).toBe(false)
 w.unmount();vi.useRealTimers()
})

it('does not show a permanent completed label when opening saved results',()=>{const w=mount(NodeActivity,{props:{state:'completed',label:'检测已结束'}});expect(w.get('.node-activity').isVisible()).toBe(false);w.unmount()})
