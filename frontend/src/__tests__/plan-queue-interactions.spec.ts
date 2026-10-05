import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import TestPlanModal from '../components/TestPlanModal.vue'
import QueueModal from '../components/QueueModal.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {api} from '../api'
import {key,type Attempt,type NodeOption} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,airports:vi.fn(),nodes:vi.fn(),startLatency:vi.fn(),startAttempt:vi.fn(),attemptAction:vi.fn()}}
})
const node:NodeOption={profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A',display_name:'Node A',type:'ss',country_code:'SG',country_flag:''}
const airport={id:'airport-a',name:'Airport A',node_count:1,has_cache:true,url_display:'',subscriptions:[{id:'sub-a',airport_id:'airport-a',name:'Default',url_display:'',url_configured:true,node_count:1,has_cache:true}]}
let wrapper:VueWrapper|undefined,w:Workspace
function attach(component:any){wrapper=mount(component,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});return wrapper}
function button(text:string){const found=Array.from(document.querySelectorAll('dialog button')).find(b=>b.textContent?.includes(text)) as HTMLButtonElement|undefined;expect(found).toBeDefined();return found!}
beforeEach(()=>{
  vi.resetAllMocks();window.sessionStorage.clear()
  HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()
  w=createWorkspace();w.airports.value=[airport];w.nodes.value=[node];w.setAirports(['airport-a'])
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML=''})

it('keeps the configuration dialog open when a node changed after selecting the test scope',async()=>{
  w.openPlan([node]);attach(TestPlanModal)
  vi.mocked(api.airports).mockResolvedValue([airport]);vi.mocked(api.nodes).mockResolvedValue([{...node,config_revision_key:'revision-new'}])
  await w.refreshNodes();await flushPromises()
  button('开始检测').click();await flushPromises()
  expect(document.querySelector('[role="alert"]')?.textContent).toContain('部分节点配置已变化，请重新选择')
  expect(w.plan.value?.nodes[0].config_revision_key).toBe('revision-a')
  expect(w.queueOpen.value).toBe(false)
  expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
  button('返回调整').click();await flushPromises();expect(w.plan.value).toBe(null)
})

it('retries a failed service save from its queue row and retains it after a rejected retry',async()=>{
  const failed:Attempt={...node,attempt_id:'service-a',request_id:'request-a',service_id:'cloudflare_204',requested_at:'2026-10-02T12:00:00Z',execution_state:'completed',persistence_state:'failed',persistence_error:'disk temporarily unavailable',rule:{target_url:'https://cp.cloudflare.com/generate_204'},result:{outcome:'matched',bytes_read:0,http_status:204,finished_at:'2026-10-02T12:00:01Z'}}
  w.queue.value=[{id:failed.request_id,node,project:'service',serviceId:failed.service_id,state:'completed',attempt:failed,error:failed.persistence_error}]
  w.mergeAttempt('service',failed);attach(QueueModal)
  vi.mocked(api.attemptAction).mockRejectedValueOnce(new Error('save retry unavailable')).mockResolvedValueOnce({...failed,persistence_state:'saved',persistence_error:undefined})
  button('重试保存').click();await flushPromises()
  expect(w.error.value).toBe('save retry unavailable')
  expect(button('重试保存').disabled).toBe(false)
  expect(w.queue.value[0].attempt?.persistence_state).toBe('failed')
  button('重试保存').click();await flushPromises()
  expect(api.attemptAction).toHaveBeenLastCalledWith('service','service-a',expect.objectContaining({profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a'}),'retry-save','cloudflare_204')
  expect(w.queue.value[0].attempt?.persistence_state).toBe('saved')
  expect(w.services.value[key(node)][0].persistence_state).toBe('saved')
  expect(document.body.textContent).not.toContain('重试保存')
  expect(document.body.textContent).not.toContain('disk temporarily unavailable')
})
