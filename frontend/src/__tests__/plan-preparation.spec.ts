import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import TestPlanModal from '../components/TestPlanModal.vue'
import {api} from '../api'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {key,type NodeOption} from '../domain'
import type {EntryResult} from '../entryPath'

vi.hoisted(()=>Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}}))
vi.mock('../api',async importOriginal=>{
 const actual=await importOriginal<typeof import('../api')>()
 return {...actual,api:{...actual.api,entries:vi.fn(),createRound:vi.fn(),finishRound:vi.fn(),measurementRounds:vi.fn(),startLatency:vi.fn(),startAttempt:vi.fn()}}
})
const node:NodeOption={profile_id:'account-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A / Account A',display_name:'Node A',type:'ss',country_code:'JP',country_flag:''}
let w:Workspace,wrapper:VueWrapper|undefined
function deferred(){let resolve!:(v:EntryResult)=>void;const promise=new Promise<EntryResult>(r=>{resolve=r});return {promise,resolve}}
function attach(){wrapper=mount(TestPlanModal,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})}
function button(text:string){const found=Array.from(document.querySelectorAll('dialog button')).find(el=>el.textContent?.includes(text)) as HTMLButtonElement|undefined;expect(found).toBeDefined();return found!}
beforeEach(()=>{
 vi.resetAllMocks();window.sessionStorage.clear()
 HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()
 vi.mocked(api.createRound).mockResolvedValue({});vi.mocked(api.finishRound).mockResolvedValue({});vi.mocked(api.measurementRounds).mockResolvedValue([])
 vi.mocked(api.entries).mockResolvedValue({nodes:[]})
 vi.mocked(api.startLatency).mockImplementation(async(body:any)=>({batch_id:body.request_id,request_id:body.request_id,state:'completed',requested_at:'2026-11-18T00:00:00Z',item_count:1,items:[]}) as any)
 vi.mocked(api.startAttempt).mockImplementation(async(_kind,body:any)=>({...node,...body,attempt_id:body.request_id,requested_at:'2026-11-18T00:00:00Z',execution_state:'completed',persistence_state:'saved',rule:{service_id:body.service_id},result:{outcome:'profiled',bytes_read:0,finished_at:'2026-11-18T00:00:01Z'}}))
 w=createWorkspace();w.airports.value=[{id:'airport-a',name:'Airport A',node_count:1,has_cache:true,url_display:'',subscriptions:[{id:'account-a',airport_id:'airport-a',name:'Account A',node_count:1,has_cache:true,url_display:'',url_configured:true}]}];w.nodes.value=[{...node}];w.setAirports(['airport-a'])
 w.catalog.value=[{service_id:'exit_ipv4',name:'IPv4 出口',category:'出口',result_kind:'exit_profile',description:'',success_criterion:'',timeout_seconds:15}]
 w.project.value='latency';w.openPlan([node])
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML=''})

it('cancels preparation from the dialog without dispatching a late test or clearing earlier results',async()=>{
 const pending=deferred();vi.mocked(api.entries).mockReturnValueOnce(pending.promise)
 w.queue.value=[{id:'previous-result',node,project:'service',state:'completed'}];attach()
 button('开始检测').click();await flushPromises()
 expect(document.querySelector('[role=status]')?.textContent).toContain('正在准备检测')
 expect((document.querySelector('fieldset.plan-editor') as HTMLFieldSetElement)?.disabled).toBe(true)
 button('取消准备').click();await flushPromises();pending.resolve({nodes:[]});await flushPromises()
 expect(w.plan.value).toBe(null);expect(w.queueOpen.value).toBe(false)
 expect(w.queue.value.map(q=>q.id)).toEqual(['previous-result'])
 expect(api.createRound).not.toHaveBeenCalled();expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
})

it('freezes parameters, full scopes and regional eligibility before waiting for entry resolution',async()=>{
 const pending=deferred();vi.mocked(api.entries).mockReturnValueOnce(pending.promise)
 w.plan.value!.projects.push('service');w.plan.value!.serviceIds=['chatgpt_unlock','abema_unlock'];w.plan.value!.samples=3
 const draft=w.plan.value!,run=w.runPlan()
 draft.samples=20;draft.repeats=3;draft.serviceSamples=4;draft.target='google';draft.nodes[0].display_name='Changed label';draft.serviceIds.push('antigravity')
 w.services.value[key(node)]=[{...node,service_id:'cloudflare_trace',rule:{service_id:'cloudflare_trace'},result:{outcome:'profiled',details:{country_code:'US'}}} as any]
 pending.resolve({nodes:[]});await run
 expect(api.startLatency).toHaveBeenCalledOnce()
 expect(api.startLatency).toHaveBeenCalledWith(expect.objectContaining({target_id:'all',sample_count:3,selections:[expect.objectContaining({profile_id:node.profile_id,node_key:node.node_key,node_identity_key:node.node_identity_key,config_revision_key:node.config_revision_key,display_name:'Node A',node_type:'ss'})]}))
 expect(vi.mocked(api.startAttempt).mock.calls.map(([,body]:any)=>body.service_id)).toEqual(['chatgpt_unlock','abema_unlock','exit_ipv4'])
 expect(vi.mocked(api.startAttempt).mock.calls.every(([,body]:any)=>body.drift_samples===1)).toBe(true)
})

it('rechecks configuration after preparation and leaves the plan available for correction',async()=>{
 const pending=deferred();vi.mocked(api.entries).mockReturnValueOnce(pending.promise)
 const run=w.runPlan();w.nodes.value=[{...node,config_revision_key:'revision-new'}];pending.resolve({nodes:[]})
 await expect(run).rejects.toThrow('部分节点配置已变化，请重新选择')
 expect(w.plan.value?.nodes[0].config_revision_key).toBe('revision-a');expect(w.preparingPlan.value).toBe(false)
 expect(api.createRound).not.toHaveBeenCalled();expect(api.startLatency).not.toHaveBeenCalled()
})

it('ignores the earlier preparation when a new plan is opened and started',async()=>{
 const first=deferred(),second=deferred();vi.mocked(api.entries).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
 const oldRun=w.runPlan();w.openPlan([node]);w.plan.value!.samples=10;const next=w.plan.value,newRun=w.runPlan()
 first.resolve({nodes:[]});await oldRun
 expect(w.plan.value).toBe(next);expect(w.preparingPlan.value).toBe(true);expect(api.createRound).not.toHaveBeenCalled()
 second.resolve({nodes:[]});await newRun
 expect(api.startLatency).toHaveBeenCalledOnce();expect(api.startLatency).toHaveBeenCalledWith(expect.objectContaining({sample_count:10}))
})

it('accepts only one start while preparation is pending',async()=>{
 const pending=deferred();vi.mocked(api.entries).mockReturnValue(pending.promise)
 const run=w.runPlan();await w.runPlan();expect(api.entries).toHaveBeenCalledOnce()
 pending.resolve({nodes:[]});await run;expect(api.startLatency).toHaveBeenCalledOnce()
})

it('does not dispatch after workspace disposal during preparation',async()=>{
 const pending=deferred();vi.mocked(api.entries).mockReturnValueOnce(pending.promise)
 const run=w.runPlan();w.dispose();pending.resolve({nodes:[]});await run
 expect(api.createRound).not.toHaveBeenCalled();expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
})

it('retains entry errors while allowing the explicitly selected measurements to run',async()=>{
 vi.mocked(api.entries).mockRejectedValueOnce(new Error('Entry lookup unavailable'))
 await w.runPlan()
 expect(w.entryErrors.value[key(node)]).toBe('Entry lookup unavailable')
 expect(api.startLatency).toHaveBeenCalledOnce();expect(api.startAttempt).toHaveBeenCalledOnce()
})

it('allows repeated IP observations in an IP-only plan and includes them in the budget',async()=>{
 w.openReputationPlan([node]);attach()
 const label=Array.from(document.querySelectorAll('dialog label')).find(el=>el.textContent?.includes('每服务采样次数'))
 expect(label).toBeDefined()
 const select=label!.querySelector('select')!;select.value='4';select.dispatchEvent(new Event('change',{bubbles:true}));await flushPromises()
 expect(w.plan.value?.serviceSamples).toBe(4);expect(document.querySelector('.plan-budget')?.textContent).toContain('4 次计划观察')
 expect(document.querySelector('.plan-budget')?.textContent).not.toContain('延迟采样');expect(document.querySelector('.plan-budget')?.textContent).not.toContain('Netflix')
 button('开始检测').click();await flushPromises()
 expect(api.startAttempt).toHaveBeenCalledWith('service',expect.objectContaining({service_id:'exit_ipv4',drift_samples:4}))
 expect(api.startLatency).not.toHaveBeenCalled()
})

it('shows regional exclusions and does not start an empty service plan',async()=>{
 w.plan.value!.projects=['service'];w.plan.value!.includeReputation=false;w.plan.value!.serviceIds=['abema_unlock']
 w.services.value[key(node)]=[{...node,service_id:'cloudflare_trace',rule:{service_id:'cloudflare_trace'},result:{outcome:'profiled',details:{country_code:'US'}}} as any]
 attach()
 expect(document.querySelector('.plan-skipped')?.textContent).toContain('地区规则跳过 1 个节点/服务组合')
 expect(document.querySelector('.plan-skipped')?.textContent).toContain('Airport A / Account A')
 button('开始检测').click();await flushPromises()
 expect(document.querySelector('[role=alert]')?.textContent).toContain('所选服务都被适用地区规则跳过')
 expect(w.plan.value).not.toBe(null);expect(api.entries).not.toHaveBeenCalled();expect(api.createRound).not.toHaveBeenCalled()
})

it('can explicitly test a current node outside the workbench viewing scope without changing that scope',async()=>{
 const outside={...node,profile_id:'account-b',node_key:'node-b',node_identity_key:'identity-b',config_revision_key:'revision-b',profile_name:'Airport B / Account B'}
 w.airports.value.push({id:'airport-b',name:'Airport B',node_count:1,has_cache:true,url_display:'',subscriptions:[{id:'account-b',airport_id:'airport-b',name:'Account B',node_count:1,has_cache:true,url_display:'',url_configured:true}]})
 w.nodes.value.push(outside);w.selectedKeys.value=[key(node)];w.openPlan([outside]);w.plan.value!.includeReputation=false
 await w.runPlan()
 expect(api.startLatency).toHaveBeenCalledWith(expect.objectContaining({selections:[expect.objectContaining({profile_id:'account-b',node_key:'node-b',node_identity_key:'identity-b',config_revision_key:'revision-b'})]}))
 expect(w.selectedAirportIds.value).toEqual(['airport-a']);expect(w.selectedKeys.value).toEqual([key(node)])
})
