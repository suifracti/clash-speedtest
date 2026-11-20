import {
 bytes,
 downloadReading,
 downloadTone,
 isReputationRule,
 key,
 stateLabel,
 type ManualRoundSummary,
 type MeasurementRoundItem,
 type NodeOption,
 type Project,
 type QueueItem,
 type ServiceRule,
} from './domain'
import {attemptStatus,serviceConclusion} from './measurementRounds'
import {nodeDisplayName,nodeDisplayParts} from './nodePresentation'
import {buildRetestPlanDraft,classifyQueueIssue,type QueueFailureCategory} from './queueReport'
import {servicePassed} from './sharedResults'

export type ManualRoundRowStatus='good'|'warn'|'bad'|'empty'

export interface ManualRoundRowAnalysis {
 item:MeasurementRoundItem
 index:number
 nodeName:string
 region:string
 projectKey:string
 projectLabel:string
 status:ManualRoundRowStatus
 saveFailed:boolean
 resultLabel:string
 evidenceText:string
 issueCategory?:QueueFailureCategory|'latency_fail'|'latency_partial'
 issueLabel?:string
 issueTone?:'warn'|'bad'
 issueReason?:string
}

export interface ManualRoundIssueGroup {
 category:string
 label:string
 tone:'warn'|'bad'
 count:number
 rows:ManualRoundRowAnalysis[]
 sampleReason:string
}

export interface ManualRoundSummaryStats {
 total:number
 good:number
 warn:number
 bad:number
 empty:number
 saveFailed:number
 issueGroups:ManualRoundIssueGroup[]
 projects:{key:string;label:string;count:number}[]
 regions:string[]
}

export interface ManualRoundRetestDraft {
 nodes:NodeOption[]
 missingNodeCount:number
 projects:Exclude<Project,'combined'>[]
 serviceIds:string[]
 includeReputation:boolean
 reputationServiceIds:string[]
}

function rawItemName(item:MeasurementRoundItem){
 return item.display_name||item.service?.display_name||item.download?.display_name||item.latency?.display_name||''
}

function asQueueItem(item:MeasurementRoundItem,index:number):QueueItem{
 const rawName=rawItemName(item)
 const node:NodeOption={
  profile_id:item.profile_id,
  node_key:item.node_key,
  node_identity_key:item.node_identity_key,
  config_revision_key:item.config_revision_key,
  display_name:rawName||'历史节点',
  profile_name:'',
  type:item.service?.node_type||item.download?.node_type||item.latency?.node_type||'',
  country_code:'',
  country_flag:'',
 }
 return {
  id:item.request_id||`round-item-${index}`,
  node,
  project:item.project,
  serviceId:item.service_id||item.service?.service_id,
  state:item.execution_state,
  attempt:item.service||item.download,
  error:item.execution_error||item.not_executed_reason||item.service?.result?.error_message||item.download?.result?.error_message,
 }
}

export function manualRoundItemResultText(item:MeasurementRoundItem):string{
 if(item.service)return serviceConclusion(item.service)
 if(item.download)return attemptStatus(item.download,true)
 if(item.latency?.total_samples)return `${item.latency.success_samples}/${item.latency.total_samples} 次响应`
 return item.execution_state==='not_executed'?'未执行':'—'
}

export function manualRoundItemEvidence(item:MeasurementRoundItem):string{
 const result=item.service?.result||item.download?.result
 return [
  result?.http_status?`HTTP ${result.http_status}`:'',
  result?.error_message||result?.summary||item.execution_error||item.latency?.error_message||item.not_executed_reason,
 ].filter(Boolean).join(' · ')
}

export function manualRoundItemProjectLabel(item:MeasurementRoundItem,catalog:ServiceRule[]):string{
 if(item.project==='latency')return '延迟检测'
 if(item.project==='download')return '下载测速'
 return item.service?.rule.name||catalog.find(r=>r.service_id===item.service_id)?.name||item.service_id||'服务检测'
}

function classifyRowStatus(item:MeasurementRoundItem):ManualRoundRowStatus{
 if(['cancelled','user_cancelled','not_executed','skipped_config','pending','not_started','queued','running','cancelling'].includes(item.execution_state))return 'empty'
 if(item.service){
  const outcome=item.service.result?.outcome||''
  if(['cancelled','user_cancelled','not_executed','interrupted'].includes(outcome))return 'empty'
  if(servicePassed(item.service))return 'good'
  if(['profiled','reachable'].includes(outcome))return outcome==='profiled'?'good':'warn'
  if(['transport_error','connection_failed','timed_out','banned','local_path_unverified'].includes(outcome))return 'bad'
  return 'warn'
 }
 if(item.download){
  const outcome=item.download.result?.outcome||''
  if(['cancelled','user_cancelled','not_executed','interrupted'].includes(outcome))return 'empty'
  const tone=downloadTone(item.download,false)
  if(tone==='good')return 'good'
  if(tone==='warn')return 'warn'
  return item.download.result?'bad':'empty'
 }
 if(item.latency?.total_samples){
  if(item.latency.success_samples===item.latency.total_samples)return 'good'
  return item.latency.success_samples>0?'warn':'bad'
 }
 if(['failed','interrupted'].includes(item.execution_state))return 'bad'
 return 'empty'
}

export function analyzeManualRoundItems(items:MeasurementRoundItem[],catalog:ServiceRule[]):ManualRoundRowAnalysis[]{
 return items.map((item,index)=>{
  const rawName=rawItemName(item)
  const nodeName=rawName?nodeDisplayName({display_name:rawName}):'历史节点'
  const region=rawName?(nodeDisplayParts({display_name:rawName}).region||'未识别地区'):'未识别地区'
  const projectKey=item.project==='service'?(item.service_id||item.service?.service_id||'service'):item.project
  const projectLabel=manualRoundItemProjectLabel(item,catalog)
  const status=classifyRowStatus(item)
  const saveFailed=item.persistence_state==='failed'
  const resultLabel=manualRoundItemResultText(item)
  const evidenceText=manualRoundItemEvidence(item)
  let issueCategory:ManualRoundRowAnalysis['issueCategory']
  let issueLabel:string|undefined
  let issueTone:'warn'|'bad'|undefined
  let issueReason:string|undefined

  if(status==='warn'||status==='bad'){
   if(item.project==='latency'){
    const failedAll=!item.latency?.success_samples
    issueCategory=failedAll?'latency_fail':'latency_partial'
    issueLabel=failedAll?'延迟无响应':'延迟部分丢包'
    issueTone=failedAll?'bad':'warn'
    issueReason=evidenceText||resultLabel
   }else{
    const classified=classifyQueueIssue(asQueueItem(item,index))
    if(classified){
     issueCategory=classified.category
     issueLabel=classified.label
     issueTone=classified.tone
     issueReason=classified.reason
    }
   }
  }

  return {
   item,
   index,
   nodeName,
   region,
   projectKey,
   projectLabel,
   status,
   saveFailed,
   resultLabel,
   evidenceText,
   issueCategory,
   issueLabel,
   issueTone,
   issueReason,
  }
 })
}

export function summarizeManualRound(rows:ManualRoundRowAnalysis[]):ManualRoundSummaryStats{
 let good=0,warn=0,bad=0,empty=0,saveFailed=0
 const projectCounts=new Map<string,{label:string;count:number}>()
 const regionSet=new Set<string>()
 const issues=new Map<string,{label:string;tone:'warn'|'bad';rows:ManualRoundRowAnalysis[];sampleReason:string}>()

 for(const row of rows){
  if(row.status==='good')good++
  else if(row.status==='warn')warn++
  else if(row.status==='bad')bad++
  else empty++
  if(row.saveFailed)saveFailed++

  const proj=projectCounts.get(row.projectKey)
  if(proj)proj.count++
  else projectCounts.set(row.projectKey,{label:row.projectLabel,count:1})
  if(row.region)regionSet.add(row.region)

  if(row.issueCategory&&row.issueLabel&&row.issueTone){
   let bucket=issues.get(row.issueCategory)
   if(!bucket){
    bucket={label:row.issueLabel,tone:row.issueTone,rows:[],sampleReason:row.issueReason||''}
    issues.set(row.issueCategory,bucket)
   }
   bucket.rows.push(row)
   if(!bucket.sampleReason&&row.issueReason)bucket.sampleReason=row.issueReason
  }
 }

 const issueGroups:ManualRoundIssueGroup[]=[...issues.entries()]
  .sort((a,b)=>b[1].rows.length-a[1].rows.length||a[0].localeCompare(b[0]))
  .map(([category,bucket])=>({
   category,
   label:bucket.label,
   tone:bucket.tone,
   count:bucket.rows.length,
   rows:bucket.rows,
   sampleReason:bucket.sampleReason,
  }))

 const projects=[...projectCounts.entries()]
  .map(([projectKey,v])=>({key:projectKey,label:v.label,count:v.count}))
  .sort((a,b)=>b.count-a.count||a.label.localeCompare(b.label,'zh-CN'))

 const regions=[...regionSet].sort((a,b)=>Number(a==='未识别地区')-Number(b==='未识别地区')||a.localeCompare(b,'zh-CN'))

 return {total:rows.length,good,warn,bad,empty,saveFailed,issueGroups,projects,regions}
}

export function buildManualRoundRetestDraft(
 rows:ManualRoundRowAnalysis[],
 currentNodes:NodeOption[],
 catalog:ServiceRule[],
):ManualRoundRetestDraft{
 const issueRows=rows.filter(r=>r.status==='warn'||r.status==='bad')
 const nonLatencyQueue=issueRows.filter(r=>r.item.project!=='latency').map(r=>asQueueItem(r.item,r.index))
 const base=buildRetestPlanDraft(nonLatencyQueue,currentNodes,catalog)

 const nodeMap=new Map<string,NodeOption>(base.nodes.map(n=>[key(n),n]))
 const seenKeys=new Set<string>(nonLatencyQueue.map(q=>key(q.node)))
 let missingNodeCount=base.missingNodeCount
 let hasLatency=false

 for(const row of issueRows){
  if(row.item.project!=='latency')continue
  hasLatency=true
  const k=key(row.item)
  if(!seenKeys.has(k)){
   seenKeys.add(k)
   const current=currentNodes.find(n=>key(n)===k&&n.node_key===row.item.node_key)
   if(current)nodeMap.set(k,current)
   else missingNodeCount++
  }
 }

 const projects:Exclude<Project,'combined'>[]=[...(hasLatency?(['latency'] as const):[]),...base.projects]
 return {
  nodes:[...nodeMap.values()],
  missingNodeCount,
  projects,
  serviceIds:base.serviceIds,
  includeReputation:base.includeReputation,
  reputationServiceIds:base.reputationServiceIds,
 }
}

export function collectPassedRoundNodes(
 rows:ManualRoundRowAnalysis[],
 currentNodes:NodeOption[],
):{nodes:NodeOption[];missingCount:number}{
 const passedByKey=new Map<string,MeasurementRoundItem>()
 const failedKeys=new Set<string>()
 for(const row of rows){
  const k=key(row.item)
  if(row.status==='bad')failedKeys.add(k)
  else if(row.status==='good'&&!passedByKey.has(k))passedByKey.set(k,row.item)
 }
 const nodes:NodeOption[]=[]
 let missingCount=0
 for(const [k,item] of passedByKey.entries()){
  if(failedKeys.has(k))continue
  const current=currentNodes.find(n=>key(n)===k&&n.node_key===item.node_key)
  if(current)nodes.push(current)
  else missingCount++
 }
 return {nodes,missingCount}
}

function csvCell(value:string|number|null|undefined):string{
 const raw=value===null||value===undefined?'':String(value)
 return /[",\r\n]/.test(raw)?`"${raw.replace(/"/g,'""')}"`:raw
}

export function buildManualRoundCSV(
 _summary:ManualRoundSummary,
 rows:ManualRoundRowAnalysis[],
 accountLabelFor:(item:MeasurementRoundItem)=>string,
):string{
 const headers=['节点','地区','账户','检测项目','执行状态','检测结果','速度/已读流量','异常分类','详细说明','保存状态']
 const lines=[headers.map(csvCell).join(',')]
 for(const row of rows){
  const dl=row.item.download
  const reading=dl?downloadReading(dl):null
  const transfer=dl?.result
   ?[reading?`${reading.value.toFixed(2)} MiB/s`:'',dl.result.bytes_read>0?bytes(dl.result.bytes_read):''].filter(Boolean).join(' / ')
   :''
  lines.push([
   row.nodeName,
   row.region,
   accountLabelFor(row.item),
   row.projectLabel,
   stateLabel(row.item.execution_state),
   row.resultLabel,
   transfer,
   row.issueLabel||'',
   row.evidenceText,
   stateLabel(row.item.persistence_state),
  ].map(csvCell).join(','))
 }
 return '\uFEFF'+lines.join('\r\n')
}

export function isReputationServiceID(id:string,catalog:ServiceRule[]):boolean{
 const rule=catalog.find(r=>r.service_id===id)
 return isReputationRule(rule||{service_id:id})
}
