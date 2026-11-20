import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import MeasurementRoundModal from '../components/MeasurementRoundModal.vue'
import HistoryView from '../components/HistoryView.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {api,saveFile} from '../api'
import {buildManualRoundCSV} from '../manualRoundReport'
import type {Attempt,LatencyBatch,ManualRoundSummary,MeasurementRoundItem,NodeOption} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async original=>{
 const actual=await original<typeof import('../api')>()
 return {
  ...actual,
  saveFile:vi.fn(async()=>true),
  api:{
   ...actual.api,
   manualRounds:vi.fn(async()=>({rounds:[]})),
   manualRound:vi.fn(),
   batches:vi.fn(async()=>[]),
   batch:vi.fn(),
   exportClash:vi.fn(async(nodes)=>({yaml_content:'proxies: []',node_count:nodes.length})),
  },
 }
})

const at='2026-11-21T02:00:00Z'
const jpNode:NodeOption={profile_id:'sub-a',node_key:'jp-1',node_identity_key:'id-jp-1',config_revision_key:'rev-1',profile_name:'机场A',display_name:'🇯🇵 日本 01 专线',type:'Hysteria2',country_code:'JP',country_flag:''}
const sgNode:NodeOption={profile_id:'sub-a',node_key:'sg-1',node_identity_key:'id-sg-1',config_revision_key:'rev-1',profile_name:'机场A',display_name:'🇸🇬 新加坡 01 高速',type:'Vless',country_code:'SG',country_flag:''}
const airport={id:'air-a',name:'机场A',node_count:2,has_cache:true,url_display:'',subscriptions:[{id:'sub-a',airport_id:'air-a',name:'机场A',url_display:'',url_configured:true,node_count:2,has_cache:true}]}

let w:Workspace,wrapper:VueWrapper|undefined
beforeEach(()=>{
 vi.clearAllMocks()
 HTMLDialogElement.prototype.showModal=vi.fn()
 HTMLDialogElement.prototype.close=vi.fn()
 w=createWorkspace()
 w.airports.value=[airport]
 w.nodes.value=[jpNode,sgNode]
 w.catalog.value=[
  {service_id:'gemini_unlock',name:'Google Gemini',category:'AI',result_kind:'unlock'} as any,
  {service_id:'ippure_ip_quality',name:'IPPure 信誉',category:'IP 风险与纯净度',result_kind:'ip_quality'} as any,
 ]
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML=''})

it('summarizes, filters, exports CSV/Clash, and builds a non-auto-start retest draft from MeasurementRoundModal',async()=>{
 const summary:ManualRoundSummary={round_id:'round-unlock-1',started_at:at,state:'finished',node_count:2,latency_count:0,download_count:0,service_count:3,service_ids:['gemini_unlock','ippure_ip_quality']}
 const passedGemini:Attempt={...jpNode,attempt_id:'a1',request_id:'r1',service_id:'gemini_unlock',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{service_id:'gemini_unlock',name:'Google Gemini',target_url:'https://gemini.google.com'},result:{outcome:'unlocked',bytes_read:100,finished_at:at}}
 const failedGemini:Attempt={...sgNode,attempt_id:'a2',request_id:'r2',service_id:'gemini_unlock',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{service_id:'gemini_unlock',name:'Google Gemini',target_url:'https://gemini.google.com'},result:{outcome:'timed_out',bytes_read:0,finished_at:at,error_message:'context deadline exceeded'}}
 const failedRep:Attempt={...sgNode,attempt_id:'a3',request_id:'r3',service_id:'ippure_ip_quality',requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{service_id:'ippure_ip_quality',name:'IPPure 信誉',target_url:'https://ippure.example'},result:{outcome:'timed_out',bytes_read:0,finished_at:at,error_message:'request timeout'}}
 const items:MeasurementRoundItem[]=[
  {...jpNode,request_id:'r1',project:'service',service_id:'gemini_unlock',execution_state:'completed',persistence_state:'saved',service:passedGemini},
  {...sgNode,request_id:'r2',project:'service',service_id:'gemini_unlock',execution_state:'completed',persistence_state:'saved',service:failedGemini},
  {...sgNode,request_id:'r3',project:'service',service_id:'ippure_ip_quality',execution_state:'completed',persistence_state:'saved',service:failedRep},
 ]
 vi.mocked(api.manualRound).mockResolvedValue({round_id:summary.round_id,started_at:at,trigger_type:'manual',state:'finished',items})

 wrapper=mount(MeasurementRoundModal,{props:{summary},attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 await flushPromises()

 // Summary cards and issue cause chip render
 expect(document.querySelectorAll('dialog tbody tr')).toHaveLength(3)
 const cards=Array.from(document.querySelectorAll('.mr-stat-card'))
 expect(cards.map(c=>c.textContent)).toEqual(expect.arrayContaining(['全部项目3','通过 / 可用1','异常 / 失败2']))

 // Filter by issue cause chip
 const issueChip=document.querySelector('.mr-issue-chip') as HTMLButtonElement
 expect(issueChip.textContent).toContain('连接 / 请求超时')
 issueChip.click();await flushPromises()
 expect(document.querySelectorAll('dialog tbody tr')).toHaveLength(2)

 // Reset filter
 ;(Array.from(document.querySelectorAll('dialog button')).find(b=>b.textContent?.includes('重置筛选')) as HTMLButtonElement).click()
 await flushPromises()
 expect(document.querySelectorAll('dialog tbody tr')).toHaveLength(3)

 // Filter by region
 const regionSelect=document.querySelector('select[aria-label="按地区筛选"]') as HTMLSelectElement
 expect(Array.from(regionSelect.options).map(o=>o.textContent)).toEqual(expect.arrayContaining(['日本','新加坡']))
 regionSelect.value='日本';regionSelect.dispatchEvent(new Event('change'));await flushPromises()
 expect(document.querySelectorAll('dialog tbody tr')).toHaveLength(1)

 // Reset region to test exports & retest
 regionSelect.value='all';regionSelect.dispatchEvent(new Event('change'));await flushPromises()

 // Export CSV
 ;(Array.from(document.querySelectorAll('dialog button')).find(b=>b.textContent?.includes('导出 CSV')) as HTMLButtonElement).click()
 await flushPromises()
 expect(saveFile).toHaveBeenCalledWith(expect.stringContaining('\uFEFF节点,地区,账户,检测项目'),expect.stringContaining('.csv'),'text/csv;charset=utf-8')

 // Export passed Clash nodes (only jpNode passed without bad items)
 ;(Array.from(document.querySelectorAll('dialog button')).find(b=>b.textContent?.includes('导出通过节点 (1)')) as HTMLButtonElement).click()
 await flushPromises()
 expect(api.exportClash).toHaveBeenCalledWith([expect.objectContaining({node_key:'jp-1'})],expect.stringContaining('通过节点'))

 // Retest issues pre-fills TestPlanModal draft cleanly split into service + reputation without running
 ;(Array.from(document.querySelectorAll('dialog button')).find(b=>b.textContent?.includes('重测异常')) as HTMLButtonElement).click()
 await flushPromises()
 expect(w.plan.value?.nodes.map(n=>n.node_key)).toEqual(['sg-1'])
 expect(w.plan.value?.projects).toEqual(['service'])
 expect(w.plan.value?.serviceIds).toEqual(['gemini_unlock'])
 expect(w.plan.value?.includeReputation).toBe(true)
 expect(w.plan.value?.reputationServiceIds).toEqual(['ippure_ip_quality'])
 expect(w.running.value).toBe(false)
})

it('opens the full LatencyReport directly from HistoryView batch detail pane',async()=>{
 const batch:LatencyBatch={
  batch_id:'b-1',request_id:'req-1',state:'completed',requested_at:at,target_id:'all',sample_count:6,item_count:1,
  items:[{...jpNode,item_id:'i-1',batch_id:'b-1',execution_state:'completed',persistence_state:'saved'}],
 }
 vi.mocked(api.batches).mockResolvedValue([batch])
 vi.mocked(api.batch).mockResolvedValue(batch)
 wrapper=mount(HistoryView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 await flushPromises()
 await wrapper.get('.history-batch-row').trigger('click')
 await flushPromises()
 const fullBtn=wrapper.findAll('button').find(b=>b.text()==='打开完整报告')
 expect(fullBtn).toBeDefined()
 await fullBtn!.trigger('click')
 expect(w.reportSelection.value?.batch.batch_id).toBe('b-1')
})

it('escapes commas and quotes cleanly in buildManualRoundCSV',()=>{
 const summary:ManualRoundSummary={round_id:'r',started_at:at,state:'finished',node_count:1,latency_count:0,download_count:0,service_count:1}
 const csv=buildManualRoundCSV(summary,[{
  item:{...jpNode,request_id:'r1',project:'service',service_id:'gemini_unlock',execution_state:'completed',persistence_state:'saved'},
  index:0,nodeName:'日本 · 01, "测试"',region:'日本',projectKey:'gemini_unlock',projectLabel:'Google Gemini',
  status:'good',saveFailed:false,resultLabel:'解锁规则通过',evidenceText:'HTTP 200',
 }],()=>'机场A')
 expect(csv.startsWith('\uFEFF')).toBe(true)
 expect(csv).toContain('"日本 · 01, ""测试"""')
})
