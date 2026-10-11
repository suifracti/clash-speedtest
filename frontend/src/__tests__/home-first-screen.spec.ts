import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import type {Airport,NodeOption,MonitorSample} from '../domain'

vi.hoisted(()=>{
  const items=new Map<string,string>()
  Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(key:string)=>items.get(key)||null,setItem:(key:string,value:string)=>items.set(key,value),removeItem:(key:string)=>items.delete(key),clear:()=>items.clear()}})
})

vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,
    refreshJobs:vi.fn(async()=>[]),refreshJob:vi.fn(),startRefresh:vi.fn(),cancelRefresh:vi.fn(),
    latencyHistory:vi.fn(async()=>({tests:[],has_more:false,complete:true})),
    measurementRounds:vi.fn(async()=>[]),monitorSamples:vi.fn(async()=>({items:[] as MonitorSample[],has_more:false,next_cursor:''})),
    latencySummaries:vi.fn(async()=>[]),attemptHistory:vi.fn(async()=>({attempts:[],has_more:false,complete:true})),
    startLatency:vi.fn(),startAttempt:vi.fn(),createRound:vi.fn(),finishRound:vi.fn(),createJob:vi.fn(),jobAction:vi.fn()
  }}
})

const profileIds=Array.from({length:8},(_,i)=>`subscription-${i+1}`)
function airports():Airport[]{
  return Array.from({length:4},(_,airportIndex)=>({
    id:`airport-${airportIndex+1}`,name:`合成机场 ${airportIndex+1}`,url_display:'',node_count:64,has_cache:true,
    website_url:'https://synthetic.invalid',backup_url:'',
    subscriptions:Array.from({length:2},(_,subscriptionIndex)=>{
      const index=airportIndex*2+subscriptionIndex
      return {id:profileIds[index],airport_id:`airport-${airportIndex+1}`,name:`合成账户 ${index+1}`,url_display:'',url_configured:true,node_count:32,has_cache:true,
        usage:{upload:1024**3,download:2*1024**3,total:10*1024**3,expire:Date.UTC(2027,0,1)/1000}}
    })
  }))
}
function nodes():NodeOption[]{
  return Array.from({length:256},(_,index)=>({
    profile_id:profileIds[index%profileIds.length],node_key:`synthetic-node-${index+1}`,node_identity_key:`synthetic-identity-${index+1}`,config_revision_key:'synthetic-revision',
    profile_name:`合成账户 ${index%profileIds.length+1}`,display_name:`香港合成节点 ${String(index+1).padStart(3,'0')}`,type:index%2?'trojan':'ss',country_code:'HK',country_flag:''
  }))
}

const wrappers:VueWrapper[]=[]
const workspaces:ReturnType<typeof createWorkspace>[]=[]
function attachHome(){
  const workspace=createWorkspace();workspaces.push(workspace)
  workspace.airports.value=airports();workspace.nodes.value=nodes();workspace.setAirports(workspace.airports.value.map(airport=>airport.id))
  const wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:workspace}}});wrappers.push(wrapper)
  return {workspace,wrapper}
}

beforeEach(()=>{
  window.localStorage.clear();window.localStorage.setItem('speedtest-home-view','detailed');window.sessionStorage.clear();vi.clearAllMocks()
  vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
})
afterEach(()=>{
  wrappers.forEach(wrapper=>wrapper.unmount());wrappers.length=0
  workspaces.forEach(workspace=>workspace.dispose());workspaces.length=0
  document.body.innerHTML='';vi.unstubAllGlobals()
})

describe('synthetic 256-node home first screen',()=>{
  it('keeps account details discoverable without changing the airport and node scope',async()=>{
    const {workspace,wrapper}=attachHome();await flushPromises()
    const disclosure=wrapper.get('details.home-subscription-disclosure')
    const disclosureElement=disclosure.element as HTMLDetailsElement
    expect(disclosureElement.open).toBe(false)
    expect(disclosure.get('summary').text()).toContain('订阅账户')
    expect(wrapper.get('.orbit-button').text()).toContain('4 个机场')
    expect(wrapper.get('.node-list-context').text()).toContain('256 个节点')

    const selectedAirports=[...workspace.selectedAirportIds.value!]
    await disclosure.get('summary').trigger('click')
    expect(disclosureElement.open).toBe(true)
    const accountPicker=wrapper.get('select[aria-label="首页用量订阅"]')
    const accountSelect=accountPicker.element as HTMLSelectElement
    expect(accountSelect.options).toHaveLength(8)
    await accountPicker.setValue(accountSelect.options[5].value)
    await flushPromises()

    expect(wrapper.get('.subscription-overview-account').text()).toContain('合成账户 6')
    expect(workspace.selectedAirportIds.value).toEqual(selectedAirports)
    expect(workspace.filteredNodes.value).toHaveLength(256)
    expect(workspace.subscription.value).toBe('all')
    expect(api.startRefresh).not.toHaveBeenCalled()
    expect(api.startLatency).not.toHaveBeenCalled()
    expect(api.startAttempt).not.toHaveBeenCalled()

    await disclosure.get('summary').trigger('click')
    expect(disclosureElement.open).toBe(false)
  })
})
