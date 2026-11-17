import {afterEach,beforeEach,describe,expect,it} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import CommandPalette from '../components/CommandPalette.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {key,type NodeOption} from '../domain'

let mounted:{unmount:()=>void}|undefined
let work:Workspace|undefined
afterEach(()=>{mounted?.unmount();mounted=undefined;work?.dispose();work=undefined;document.body.innerHTML=''})
beforeEach(()=>{HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')};HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}})
const node=(i:number,name:string,cc='JP'):NodeOption=>({profile_id:'p1',node_key:`k${i}`,node_identity_key:`id${i}`,config_revision_key:'r',profile_name:'合成账户',display_name:name,type:'ss',country_code:cc,country_flag:''} as NodeOption)
const pages=[{id:'home' as const,label:'线路工作台',icon:'home'},{id:'history' as const,label:'检测记录',icon:'history'},{id:'airports' as const,label:'机场与账户',icon:'layers',keywords:['订阅']}]
async function search(text:string){const input=document.querySelector<HTMLInputElement>('.cst-palette-search input')!;input.value=text;input.dispatchEvent(new Event('input'));await flushPromises();return input}
function option(text:string){return [...document.querySelectorAll<HTMLElement>('[role=option]')].find(row=>row.textContent?.includes(text))}

describe('command palette (synthetic data)',()=>{
  function setup(){
    const w=createWorkspace();work=w
    w.airports.value=[{id:'a1',name:'合成机场',url_display:'',node_count:2,has_cache:true,subscriptions:[{id:'p1',airport_id:'a1',name:'合成账户',url_display:'',url_configured:true,node_count:2,has_cache:true}]}] as never
    w.selectedAirportIds.value=['a1']
    w.nodes.value=[node(1,'日本 合成 01'),node(2,'香港 合成 02','HK')]
    const wrapper=mount(CommandPalette,{props:{pages},global:{provide:{[workspaceKey as symbol]:w}},attachTo:document.body})
    mounted=wrapper
    return {w,wrapper}
  }
  it('views a node without selecting it or changing filters',async()=>{
    const {w}=setup()
    const input=document.body.querySelector<HTMLInputElement>('.cst-palette-search input')!
    expect(document.activeElement).toBe(input)
    input.value='日本';input.dispatchEvent(new Event('input'))
    await flushPromises()
    const rows=[...document.body.querySelectorAll('.cst-palette-row')].map(r=>r.textContent)
    expect(rows.some(t=>t?.includes('合成 01'))).toBe(true)
    expect(rows.some(t=>t?.includes('香港'))).toBe(false)
    input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter'}))
    expect(w.viewRequest.value?.node_key).toBe('k1')
    expect(w.selectedKeys.value).toEqual([])
    expect(w.airportFilter.value).toBe('all')
  })
  it('lists pages first and filters airports by name',async()=>{
    const {w,wrapper}=setup()
    const input=document.body.querySelector<HTMLInputElement>('.cst-palette-search input')!
    input.value='合成机场';input.dispatchEvent(new Event('input'))
    await flushPromises()
    input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter'}))
    expect(w.airportFilter.value).toBe('a1')
    expect(wrapper.emitted('navigate')?.[0]).toEqual(['home'])
  })
  it('finds nodes outside the workbench scope without changing selections, filters or an existing plan',async()=>{
    const {w}=setup(),outside={...node(3,'新加坡 外部 03','SG'),profile_id:'p3',profile_name:'外部账户'}
    w.airports.value.push({id:'a2',name:'另一个机场',url_display:'',node_count:1,has_cache:true,subscriptions:[{id:'p3',airport_id:'a2',name:'外部账户',url_display:'',url_configured:true,node_count:1,has_cache:true}]})
    w.nodes.value.push(outside,{...node(99,'公告：新加坡订阅说明','SG'),profile_id:'p3'});w.selectedKeys.value=[key(w.nodes.value[0])];w.region.value='JP';w.search.value='日常';w.openPlan([w.nodes.value[0]])
    const before=JSON.stringify(w.plan.value),input=await search('新加坡')
    expect(option('外部 03')?.textContent).toContain('外部账户')
    expect(document.querySelectorAll('[role=option]')).toHaveLength(1)
    input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter'}))
    expect(w.viewRequest.value).toEqual(outside);expect(w.selectedAirportIds.value).toEqual(['a1'])
    expect(w.selectedKeys.value).toEqual([key(w.nodes.value[0])]);expect(w.region.value).toBe('JP');expect(w.search.value).toBe('日常');expect(JSON.stringify(w.plan.value)).toBe(before)
  })
  it('distinguishes same-name nodes by account and accepts search words in either order',async()=>{
    const {w}=setup()
    w.airports.value[0].subscriptions!.push({id:'p2',airport_id:'a1',name:'备用套餐',url_display:'',url_configured:true,node_count:1,has_cache:true})
    const second={...node(3,'香港 合成 02','HK'),profile_id:'p2',profile_name:'合成机场 / 备用套餐'};w.nodes.value.push(second)
    await search('香港')
    const rows=[...document.querySelectorAll('[role=option]')]
    expect(rows).toHaveLength(2);expect(rows[0].textContent).toContain('合成账户');expect(rows[1].textContent).toContain('备用套餐')
    const input=await search('备用 香港');expect(document.querySelectorAll('[role=option]')).toHaveLength(1)
    input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter'}));expect(w.viewRequest.value).toEqual(second)
  })
  it('finds pages by the names shown in the main navigation',async()=>{
    const {wrapper}=setup(),input=await search('订阅')
    expect(option('机场与账户')).toBeDefined();input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter'}))
    expect(wrapper.emitted('navigate')?.[0]).toEqual(['airports'])
  })
  it('lets the user reach matches beyond the first forty without closing the search',async()=>{
    const {w,wrapper}=setup();w.nodes.value=Array.from({length:47},(_,i)=>node(i+1,`日本 合成 ${String(i+1).padStart(2,'0')}`))
    await search('日本')
    expect(document.querySelector('.cst-palette-list')?.textContent).toContain('47')
    const more=option('显示更多节点');expect(more).toBeDefined();more!.click();await flushPromises()
    expect(wrapper.emitted('close')).toBeUndefined();expect(document.querySelectorAll('[role=option]')).toHaveLength(47)
    option('47')!.click();expect(w.viewRequest.value?.node_key).toBe('k47')
  })
  it('keeps keyboard selection valid when the loaded node list shrinks',async()=>{
    const {w}=setup();w.nodes.value=Array.from({length:3},(_,i)=>node(i+1,`日本 合成 ${i+1}`));const input=await search('日本')
    input.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowUp'}));await flushPromises()
    w.nodes.value=[w.nodes.value[0]];await flushPromises();input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter'}))
    expect(w.viewRequest.value?.node_key).toBe('k1')
  })
})

describe('airport quick switch (synthetic data)',()=>{
  it('adds an airport outside the measurement scope before filtering to it',()=>{
    const w=createWorkspace()
    w.airports.value=[{id:'a1',name:'甲',url_display:'',node_count:1,has_cache:true,subscriptions:[{id:'p1',airport_id:'a1',name:'默认',url_display:'',url_configured:true,node_count:1,has_cache:true}]},{id:'a2',name:'乙',url_display:'',node_count:1,has_cache:true,subscriptions:[{id:'p2',airport_id:'a2',name:'默认',url_display:'',url_configured:true,node_count:1,has_cache:true}]}] as never
    w.nodes.value=[node(1,'甲 01'),{...node(2,'乙 01'),profile_id:'p2'}]
    w.selectedAirportIds.value=['a1']
    expect(w.proxyNodes.value).toHaveLength(1)
    w.viewAirport('a2')
    expect(w.selectedAirportIds.value).toEqual(['a1','a2'])
    expect(w.airportFilter.value).toBe('a2')
    expect(w.filteredNodes.value.map(n=>n.display_name)).toEqual(['乙 01'])
    w.viewAirport('all')
    expect(w.airportFilter.value).toBe('all')
    expect(w.filteredNodes.value).toHaveLength(2)
  })
})
