import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import NodeDetailModal from '../components/NodeDetailModal.vue'
import TrendChart from '../components/TrendChart.vue'
import {api} from '../api'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {serviceAvailability} from '../sharedResults'
import type {Attempt,NodeOption} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,measurementRounds:vi.fn(async()=>[]),revisions:vi.fn(async()=>[]),attemptHistory:vi.fn()}}
})

const node:NodeOption={profile_id:'profile',node_key:'node',node_identity_key:'identity',config_revision_key:'revision',profile_name:'Airport',display_name:'Node',type:'ss',country_code:'',country_flag:''}
const time='2026-10-10T12:00:00Z'
function attempt(id:string,outcome:string,options:{execution_state?:string;http_status?:number}={}):Attempt{
  return {...node,attempt_id:id,request_id:'request-'+id,service_id:'service-a',requested_at:time,finished_at:time,execution_state:options.execution_state||'completed',persistence_state:'saved',rule:{service_id:'service-a',name:'Service A',target_url:'https://service.example'},result:{outcome,http_status:options.http_status,bytes_read:0,finished_at:time}}
}

let wrapper:VueWrapper|undefined,w:Workspace
beforeEach(()=>{
  vi.resetAllMocks()
  HTMLDialogElement.prototype.showModal=vi.fn()
  HTMLDialogElement.prototype.close=vi.fn()
  vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})
  vi.mocked(api.revisions).mockResolvedValue([])
  w=createWorkspace();w.project.value='service';w.serviceIds.value=['service-a']
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML='';vi.unstubAllGlobals()})

it('uses confirmed service availability for each manual-history trend point',async()=>{
  const attempts=[
    attempt('pass','matched'),
    attempt('business-reject','region_blocked'),
    attempt('rate-limited','http_rejected',{http_status:429}),
    attempt('profiled','profiled'),
    attempt('transport-error','transport_error'),
    attempt('not-executed','unknown',{execution_state:'not_executed'}),
  ]
  vi.mocked(api.attemptHistory).mockResolvedValue({attempts,has_more:false,complete:true})
  wrapper=mount(NodeDetailModal,{props:{node},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
  await flushPromises()

  expect(attempts.map(a=>serviceAvailability(a))).toEqual([1,0,null,null,null,null])
  const points=wrapper.findComponent(TrendChart).props('points')
  const byId=Object.fromEntries(points.map(point=>[point.id,point]))
  expect(Object.fromEntries(Object.entries(byId).map(([id,point])=>[id,point.value]))).toEqual({
    pass:1,'business-reject':0,'rate-limited':null,profiled:null,'transport-error':null,'not-executed':null,
  })
  expect(byId['business-reject'].tone).toBe('bad')
  expect(byId['rate-limited'].tone).toBe('warn')
  expect(byId['transport-error'].tone).toBe('warn')
  expect(byId['not-executed'].tone).toBe('empty')
})
