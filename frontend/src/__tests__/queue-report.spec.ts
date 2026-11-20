import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import QueueModal from '../components/QueueModal.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {api} from '../api'
import {buildRetestPlanDraft,classifyQueueIssue,summarizeQueueFailures} from '../queueReport'
import type {Attempt,NodeOption,QueueItem,ServiceRule} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async importOriginal=>{
 const actual=await importOriginal<typeof import('../api')>()
 return {...actual,api:{...actual.api,startAttempt:vi.fn(),startLatency:vi.fn(),attemptAction:vi.fn()}}
})

const mkNode=(id:string,name:string):NodeOption=>({
 profile_id:'sub-a',node_key:id,node_identity_key:'id-'+id,config_revision_key:'rev-1',
 profile_name:'Airport A',display_name:name,type:'Vless',country_code:'SG',country_flag:'',
})

const catalog:ServiceRule[]=[
 {service_id:'antigravity',name:'Google Antigravity',category:'AI',result_kind:'account_availability',description:'',success_criterion:'',timeout_seconds:15},
 {service_id:'gemini_unlock',name:'Google Gemini',category:'AI',result_kind:'unlock',description:'',success_criterion:'',timeout_seconds:15},
 {service_id:'ippure_ip_quality',name:'IPPure IP 纯净度',category:'IP 信誉',result_kind:'ip_quality',description:'',success_criterion:'',timeout_seconds:15},
 {service_id:'cloudflare_challenge',name:'Cloudflare 人机验证',category:'IP 信誉',result_kind:'ip_quality',description:'',success_criterion:'',timeout_seconds:15},
]

const at='2026-11-20T12:00:00Z'
function mkAttempt(node:NodeOption,serviceId:string,outcome:string,extra:Partial<NonNullable<Attempt['result']>>={},persist:'saved'|'failed'='saved'):Attempt{
 return {
  ...node,
  attempt_id:`att-${node.node_key}-${serviceId}-${outcome}`,
  request_id:`req-${node.node_key}-${serviceId}`,
  service_id:serviceId,
  requested_at:at,
  execution_state:'completed',
  persistence_state:persist,
  persistence_error:persist==='failed'?'disk busy':undefined,
  rule:{service_id:serviceId,name:catalog.find(c=>c.service_id===serviceId)?.name||serviceId,target_url:'https://example.invalid'},
  result:{outcome,bytes_read:0,finished_at:at,...extra},
 }
}

let wrapper:VueWrapper|undefined,w:Workspace
beforeEach(()=>{
 vi.resetAllMocks()
 HTMLDialogElement.prototype.showModal=vi.fn()
 HTMLDialogElement.prototype.close=vi.fn()
 w=createWorkspace()
 w.catalog.value=catalog
})
afterEach(()=>{
 wrapper?.unmount()
 wrapper=undefined
 w.dispose()
 document.body.innerHTML=''
})

it('groups non-latency failures by cause, skips save-only successes, and builds exact separated retest drafts',()=>{
 const n1=mkNode('sg-1','新加坡 01'),n2=mkNode('sg-2','新加坡 02'),n3=mkNode('uk-1','英国 01')
 const items:QueueItem[]=[
  {id:'q1',node:n1,project:'service',serviceId:'antigravity',state:'completed',attempt:mkAttempt(n1,'antigravity','timed_out',{failure_phase:'loadCodeAssist',error_message:'请求超过设定超时'})},
  {id:'q2',node:n2,project:'service',serviceId:'ippure_ip_quality',state:'completed',attempt:mkAttempt(n2,'ippure_ip_quality','timed_out',{error_message:'连接超时'})},
  {id:'q3',node:n3,project:'service',serviceId:'antigravity',state:'completed',attempt:mkAttempt(n3,'antigravity','region_blocked',{error_message:'Google 返回 CN 不支持'})},
  {id:'q4',node:n2,project:'service',serviceId:'cloudflare_challenge',state:'completed',attempt:mkAttempt(n2,'cloudflare_challenge','challenge',{error_message:'3/3 站点弹盾'})},
  // Succeeded execution with save failure must NOT appear as a network retest failure.
  {id:'q5',node:n1,project:'service',serviceId:'gemini_unlock',state:'completed',attempt:mkAttempt(n1,'gemini_unlock','unlocked',{},'failed'),error:'disk busy'},
  // Cancelled item is not classified as a probe failure.
  {id:'q6',node:n3,project:'service',serviceId:'gemini_unlock',state:'cancelled'},
 ]

 expect(classifyQueueIssue(items[4]!)).toBeNull()
 expect(classifyQueueIssue(items[5]!)).toBeNull()

 const summary=summarizeQueueFailures(items,catalog)
 expect(summary.totalItems).toBe(4)
 expect(summary.totalNodes).toBe(3)
 expect(summary.groups.map(g=>[g.category,g.items.length,g.nodeCount])).toEqual([
  ['timeout',2,2],
  ['region_blocked',1,1],
  ['challenge',1,1],
 ])
 expect(summary.groups[0]!.sampleReason).toContain('账号项目查询')

 const timeoutDraft=buildRetestPlanDraft(summary.groups[0]!.items,[n1,n2,n3],catalog)
 expect(timeoutDraft).toMatchObject({
  missingNodeCount:0,
  projects:['service'],
  serviceIds:['antigravity'],
  includeReputation:true,
  reputationServiceIds:['ippure_ip_quality'],
 })
 expect(timeoutDraft.nodes.map(n=>n.node_key)).toEqual(['sg-1','sg-2'])

 // Reputation-only group keeps projects=[] and includeReputation=true.
 const cfDraft=buildRetestPlanDraft(summary.groups[2]!.items,[n1,n2,n3],catalog)
 expect(cfDraft).toMatchObject({
  projects:[],
  serviceIds:[],
  includeReputation:true,
  reputationServiceIds:['cloudflare_challenge'],
 })
})

it('filters queue rows by failure category and opens pre-populated TestPlan draft without auto-running',async()=>{
 const n1=mkNode('sg-1','新加坡 01'),n2=mkNode('sg-2','新加坡 02')
 w.nodes.value=[n1,n2]
 w.queueOpen.value=true
 w.queue.value=[
  {id:'q1',node:n1,project:'service',serviceId:'antigravity',state:'completed',attempt:mkAttempt(n1,'antigravity','timed_out',{failure_phase:'loadCodeAssist',error_message:'连接超时：请求超过设定超时'})},
  {id:'q2',node:n2,project:'service',serviceId:'ippure_ip_quality',state:'completed',attempt:mkAttempt(n2,'ippure_ip_quality','transport_error',{error_message:'dial tcp refused'})},
  {id:'q3',node:n1,project:'service',serviceId:'gemini_unlock',state:'completed',attempt:mkAttempt(n1,'gemini_unlock','unlocked')},
 ]
 wrapper=mount(QueueModal,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 await flushPromises()

 const report=document.querySelector('.queue-issue-report')
 expect(report).not.toBeNull()
 expect(report!.textContent).toContain('连接 / 请求超时')
 expect(report!.textContent).toContain('连接 / TLS 失败')
 // Informative Chinese error message with failure_phase is preserved verbatim.
 expect(document.querySelector('.queue-row p.bad')?.textContent).toContain('连接超时：请求超过设定超时（阶段：loadCodeAssist）')

 // Clicking the timeout group filters the rows down to only the 1 timeout row.
 const groupBtns=Array.from(document.querySelectorAll('.queue-issue-main')) as HTMLButtonElement[]
 groupBtns.find(b=>b.textContent?.includes('连接 / 请求超时'))!.click()
 await flushPromises()
 expect(document.querySelectorAll('.queue-row')).toHaveLength(1)
 expect(document.querySelector('.queue-row')?.textContent).toContain('Google Antigravity')

 // Clicking per-category retest opens TestPlanModal draft for ONLY that category without starting tests.
 const retestBtns=Array.from(document.querySelectorAll('.queue-issue-retest')) as HTMLButtonElement[]
 retestBtns[0]!.click()
 await flushPromises()
 expect(w.queueOpen.value).toBe(false)
 expect(w.plan.value).not.toBeNull()
 expect(w.plan.value!.nodes.map(n=>n.node_key)).toEqual(['sg-1'])
 expect(w.plan.value!.projects).toEqual(['service'])
 expect(w.plan.value!.serviceIds).toEqual(['antigravity'])
 expect(w.plan.value!.includeReputation).toBe(false)
 expect(w.plan.value!.reputationServiceIds).toEqual([])
 expect(api.startAttempt).not.toHaveBeenCalled()
})
