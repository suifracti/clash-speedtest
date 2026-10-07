import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {mount,flushPromises,type VueWrapper} from '@vue/test-utils'
import HealthBars from '../components/HealthBars.vue'
import type {TrendPoint} from '../presentation'
let wrappers:VueWrapper[]=[],drivers:Array<{emit:(el:Element,visible:boolean)=>void;targets:Set<Element>;disconnect:ReturnType<typeof vi.fn>}> = []
beforeEach(()=>{drivers=[];vi.stubGlobal('IntersectionObserver',class{
 targets=new Set<Element>();disconnect=vi.fn(()=>this.targets.clear())
 constructor(private cb:(entries:any[])=>void){drivers.push(this)}
 observe(el:Element){this.targets.add(el)}unobserve(el:Element){this.targets.delete(el)}
 emit(el:Element,visible:boolean){this.cb([{target:el,isIntersecting:visible}])}
});vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})})
afterEach(()=>{wrappers.forEach(w=>w.unmount());wrappers=[];document.body.innerHTML='';vi.unstubAllGlobals()})
const point:TrendPoint={id:'round',time:'2026-10-01T00:00:00Z',value:80,tone:'good',description:'80 ms · 定时',trigger:'scheduled',samples:[{id:'sample',time:'2026-10-01T00:00:00Z',value:80,tone:'good',label:'采样',description:'80 ms'}]}
function attach(){const w=mount(HealthBars,{attachTo:document.body,props:{points:[point],label:'Cloudflare',defer:true}});wrappers.push(w);return w}
describe('visible-area health bars',()=>{
 it('omits offscreen cells, preserves geometry, and renders updated records on re-entry',async()=>{
  const w=attach(),el=w.element;expect(w.findAll('button')).toHaveLength(0);expect(w.find('[aria-hidden="true"]').exists()).toBe(true)
  drivers[0].emit(el,true);await flushPromises();expect(w.findAll('button')).toHaveLength(16)
  await w.find('.has-record').trigger('mouseenter');expect(w.emitted('inspect')?.[0]?.[0]).toMatchObject({id:'round',value:80})
  drivers[0].emit(el,false);await flushPromises();expect(w.findAll('button')).toHaveLength(0)
  await w.setProps({points:[{...point,value:120,description:'120 ms · 手动',trigger:'manual'}]});drivers[0].emit(el,true);await flushPromises()
  await w.find('.has-record').trigger('mouseenter');expect(w.emitted('inspect')?.at(-1)?.[0]).toMatchObject({id:'round',value:120,trigger:'manual'})
  expect(w.find('.has-record').attributes('aria-label')).toContain('120 ms');await w.find('.has-record').trigger('click');expect(w.emitted('open')?.[0]?.[0]).toMatchObject({value:120})
 })
 it('shares one observer and releases targets after repeated visibility changes and unmount',async()=>{
  const a=attach(),b=attach();expect(drivers).toHaveLength(1);expect(drivers[0].targets.size).toBe(2)
  for(let i=0;i<30;i++){drivers[0].emit(a.element,true);await flushPromises();drivers[0].emit(a.element,false);await flushPromises();expect(a.findAll('button')).toHaveLength(0)}
  a.unmount();expect(drivers[0].targets.size).toBe(1);b.unmount();expect(drivers[0].targets.size).toBe(0);expect(drivers[0].disconnect).toHaveBeenCalledOnce();wrappers=[]
 })
 it('mounts recorded buttons on keyboard entry and retains them while focus remains inside',async()=>{
  const w=attach();expect(w.attributes('tabindex')).toBe('0');(w.element as HTMLElement).focus();await flushPromises()
  expect(document.activeElement?.classList.contains('has-record')).toBe(true);expect(w.emitted('inspect')?.[0]?.[0]).toMatchObject({id:'round',value:80})
  drivers[0].emit(w.element,false);await flushPromises();expect(w.findAll('button')).toHaveLength(16)
  const outside=document.createElement('button');document.body.appendChild(outside);outside.focus();await flushPromises();expect(w.findAll('button')).toHaveLength(0);expect(w.emitted('leave')?.length).toBeGreaterThan(0)
 })
})
