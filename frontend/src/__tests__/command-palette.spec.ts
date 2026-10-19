import {afterEach,beforeEach,describe,expect,it} from 'vitest'
import {flushPromises,mount} from '@vue/test-utils'
import CommandPalette from '../components/CommandPalette.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import type {NodeOption} from '../domain'

let mounted:{unmount:()=>void}|undefined
afterEach(()=>{mounted?.unmount();mounted=undefined;document.body.innerHTML=''})
beforeEach(()=>{HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')};HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}})
const node=(i:number,name:string,cc='JP'):NodeOption=>({profile_id:'p1',node_key:`k${i}`,node_identity_key:`id${i}`,config_revision_key:'r',profile_name:'合成账户',display_name:name,type:'ss',country_code:cc,country_flag:''} as NodeOption)
const pages=[{id:'home' as const,label:'线路工作台',icon:'home'},{id:'history' as const,label:'检测记录',icon:'history'}]

describe('command palette (synthetic data)',()=>{
  function setup(){
    const w=createWorkspace()
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
})
