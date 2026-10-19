import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {mount,type VueWrapper} from '@vue/test-utils'
import TestPlanModal from '../components/TestPlanModal.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {api} from '../api'
import type {Attempt,NodeOption,ServiceRule} from '../domain'

vi.hoisted(()=>Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{},removeItem:()=>{},clear:()=>{}}}))
vi.mock('../api',async importOriginal=>{
 const actual=await importOriginal<typeof import('../api')>()
 return {...actual,api:{...actual.api,createRound:vi.fn(async()=>undefined),finishRound:vi.fn(async()=>undefined),measurementRounds:vi.fn(async()=>[]),startAttempt:vi.fn()}}
})

const node:NodeOption={profile_id:'profile-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'机场 A',display_name:'香港 01',type:'trojan',country_code:'HK',country_flag:''}
const rule=(service_id:string,result_kind:string,name=service_id):ServiceRule=>({service_id,result_kind,name,category:'fixture',description:'fixture',success_criterion:'fixture',timeout_seconds:10})
const rules=[
 rule('cloudflare_204','connectivity','Cloudflare 204'),
 rule('netflix_unlock','unlock','Netflix'),
 rule('antigravity','model','Antigravity'),
 rule('exit_ipv4','exit_profile','IPv4 出口'),
 rule('exit_ipv6','exit_profile','IPv6 出口'),
 rule('ping0_ipv4','ip_quality','Ping0 IPv4'),
 rule('ping0_ipv6','ip_quality','Ping0 IPv6'),
 rule('ippure_ip_quality','ip_quality','IPPure'),
 rule('ipok_optional','connectivity','IPOK optional'),
 rule('quality_optional','ip_quality','Quality optional')
]
const modalStub={template:'<div class="modal-stub"><slot/><slot name="footer"/></div>'}
let w:Workspace,wrapper:VueWrapper|undefined

function setupWorkspace(){
 w=createWorkspace()
 w.airports.value=[{id:'airport-a',name:'机场 A',url_display:'',node_count:1,has_cache:true,subscriptions:[{id:'profile-a',airport_id:'airport-a',name:'默认',url_display:'',url_configured:true,node_count:1,has_cache:true}]}]
 w.nodes.value=[node]
 w.setAirports(['airport-a'])
 w.catalog.value=[...rules]
 w.serviceIds.value=['cloudflare_204']
}

beforeEach(()=>{
 window.localStorage.clear();window.sessionStorage.clear();vi.clearAllMocks()
 HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()
 setupWorkspace()
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML=''})

it('defaults to every available IP reputation rule without changing normal service selection',()=>{
 w.project.value='service';w.catalog.value=rules.filter(item=>item.service_id!=='exit_ipv6');w.serviceIds.value=['cloudflare_204','exit_ipv4','ippure_ip_quality']
 w.openPlan([node])
 expect(w.plan.value?.projects).toEqual(['service'])
 expect(w.plan.value?.serviceIds).toEqual(['cloudflare_204'])
 expect(w.plan.value?.includeReputation).toBe(true)
 expect(w.plan.value?.reputationServiceIds).toEqual(['exit_ipv4','ping0_ipv4','ping0_ipv6','ippure_ip_quality','quality_optional'])
 w.openReputationPlan([node])
 expect(w.plan.value?.projects).toEqual([])
 expect(w.plan.value?.includeReputation).toBe(true)
 expect(w.plan.value?.reputationServiceIds).toEqual(['exit_ipv4','ping0_ipv4','ping0_ipv6','ippure_ip_quality','quality_optional'])
 expect(w.plan.value?.serviceIds).toEqual(['cloudflare_204'])
 expect(w.serviceIds.value).toEqual(['cloudflare_204'])
})

it('keeps the IP reputation checkbox and its filtered picker independent from normal services',async()=>{
 w.project.value='service';w.openPlan([node])
 wrapper=mount(TestPlanModal,{global:{provide:{[workspaceKey as symbol]:w},stubs:{Modal:modalStub}}})
 const reputationToggle=wrapper.findAll('.plan-projects label').find(label=>label.text().includes('IP信誉/纯净度'))
 expect(reputationToggle).toBeDefined()
 expect((reputationToggle!.find('input').element as HTMLInputElement).checked).toBe(true)
 await reputationToggle!.find('input').setValue(false)
 expect(w.plan.value?.includeReputation).toBe(false)
 await reputationToggle!.find('input').setValue(true)
 const pickers=wrapper.findAll('.service-picker')
 expect(pickers).toHaveLength(2)
 const regularNames=pickers[0].findAll('.service-picker-list strong').map(item=>item.text())
 const reputationNames=pickers[1].findAll('.service-picker-list strong').map(item=>item.text())
 expect(regularNames).toEqual(['Cloudflare 204','Netflix','Antigravity'])
 expect(reputationNames).toEqual(['IPv4 出口','IPv6 出口','Ping0 IPv4','Ping0 IPv6','IPPure','Quality optional'])
 expect(pickers[1].findAll('.service-picker-list input').every(input=>(input.element as HTMLInputElement).checked)).toBe(true)
 for(const input of pickers[1].findAll('.service-picker-list input'))await input.setValue(false)
 const ping0=pickers[1].findAll('.service-picker-list label').find(label=>label.text().includes('Ping0'))!
 await ping0.find('input').setValue(true)
 expect(w.plan.value?.reputationServiceIds).toEqual(['ping0_ipv4'])
 expect(w.plan.value?.serviceIds).toEqual(['cloudflare_204'])
 const netflix=pickers[0].findAll('.service-picker-list label').find(label=>label.text().includes('Netflix'))!
 await netflix.find('input').setValue(true)
 expect(w.plan.value?.serviceIds).toEqual(['cloudflare_204','netflix_unlock'])
 expect(w.plan.value?.reputationServiceIds).toEqual(['ping0_ipv4'])
 expect(wrapper.get('.plan-budget').text()).toContain('3 次计划采样')
 expect(w.plan.value?.reputationServiceIds).not.toContain('antigravity')
})

it('runs an IP-only plan with deduplicated reputation services and ignores unchecked normal service IDs',async()=>{
 const completed=(kind:string,body:any):Attempt=>({...node,attempt_id:String(body.request_id),request_id:String(body.request_id),service_id:String(body.service_id),requested_at:'2026-10-18T10:00:00Z',execution_state:'completed',persistence_state:'saved',rule:{service_id:String(body.service_id),target_url:''},result:{outcome:'profiled',bytes_read:0,finished_at:'2026-10-18T10:00:01Z'}})
 vi.mocked(api.startAttempt).mockImplementation(async(kind,body:any)=>completed(kind,body))
 w.plan.value={nodes:[node],projects:[],target:'all',samples:6,timeout:5,concurrency:8,downloadMiB:10,downloadSeconds:10,serviceIds:['cloudflare_204'],repeats:1,includeReputation:true,reputationServiceIds:['exit_ipv4','ping0_ip_quality','exit_ipv4','netflix_unlock']}
 const frozen=w.plan.value
 await w.runPlan()
 expect(api.startAttempt).toHaveBeenCalledTimes(2)
 expect(vi.mocked(api.startAttempt).mock.calls.map(([,body])=>(body as {service_id:string}).service_id)).toEqual(['exit_ipv4','ping0_ip_quality'])
 expect(w.queue.value.map(item=>item.serviceId)).toEqual(['exit_ipv4','ping0_ip_quality'])
 expect(frozen.projects).toEqual([])
 expect(frozen.serviceIds).toEqual(['cloudflare_204'])
 expect(frozen.reputationServiceIds).toEqual(['exit_ipv4','ping0_ip_quality','exit_ipv4','netflix_unlock'])
 expect(api.createRound).toHaveBeenCalledWith(expect.objectContaining({items:expect.arrayContaining([
  expect.objectContaining({project:'service',service_id:'exit_ipv4'}),
  expect.objectContaining({project:'service',service_id:'ping0_ip_quality'})
 ])}))
})
