import {isReputationRule,key,outcomeLabel,type NodeOption,type Project,type QueueItem,type ServiceRule} from './domain'
import {downloadNoDataReason} from './measurementRounds'
import {downloadReading,downloadTone} from './domain'

export type QueueFailureCategory='timeout'|'transport'|'region_blocked'|'challenge'|'auth_quota'|'http_or_rule'|'other'

export interface QueueFailureClassification {
 category:QueueFailureCategory
 label:string
 tone:'warn'|'bad'
 reason:string
}

export interface QueueFailureGroup {
 category:QueueFailureCategory
 label:string
 tone:'warn'|'bad'
 items:QueueItem[]
 nodeCount:number
 serviceLabels:string[]
 sampleReason:string
}

export interface QueueFailureSummary {
 totalItems:number
 totalNodes:number
 groups:QueueFailureGroup[]
}

export interface RetestPlanDraft {
 nodes:NodeOption[]
 missingNodeCount:number
 projects:Exclude<Project,'combined'>[]
 serviceIds:string[]
 includeReputation:boolean
 reputationServiceIds:string[]
}

const categoryMeta:Record<QueueFailureCategory,{label:string;tone:'warn'|'bad';order:number}>={
 timeout:{label:'连接 / 请求超时',tone:'warn',order:1},
 transport:{label:'连接 / TLS 失败',tone:'bad',order:2},
 region_blocked:{label:'地区受限 / 送中',tone:'warn',order:3},
 challenge:{label:'触发验证 / 弹盾',tone:'bad',order:4},
 auth_quota:{label:'凭据失效 / 额度限流',tone:'warn',order:5},
 http_or_rule:{label:'HTTP 拒绝 / 规则未通过',tone:'warn',order:6},
 other:{label:'其他异常 / 待确认',tone:'warn',order:7},
}

const phaseLabels:Record<string,string>={
 loadCodeAssist:'账号项目查询',
 fetchAvailableModels:'模型目录查询',
 'streamGenerateContent?alt=sse':'模型生成',
 model_response:'模型回答确认',
 fetchUserInfo:'Google 地区识别',
 drift_validation:'多轮出口漂移',
 proxy_connect:'代理建连',
 proxy_handshake:'代理握手',
 tls_handshake:'TLS 握手',
 response_headers:'等待响应头',
 response_body:'读取响应体',
}

function formatReason(raw:string,phase?:string,fallback=''):string{
 const clean=raw.trim()
 const phaseText=phase&&!['timeout','http_status'].includes(phase)?(phaseLabels[phase]||phase):''
 const base=clean||fallback
 if(!base)return phaseText?`阶段：${phaseText}`:''
 if(phaseText&&!base.includes(phaseText)&&!base.includes(phase||''))return `${base}（阶段：${phaseText}）`
 return base
}

function isServiceGood(q:QueueItem):boolean{
 const r=q.attempt?.result
 if(!r)return false
 const sid=q.serviceId||q.attempt?.service_id||''
 if(sid==='antigravity')return r.outcome==='matched'&&r.details?.antigravity_progress==='真实模型已回答'
 if(sid==='cloudflare_challenge')return r.outcome==='profiled'
 return ['matched','unlocked','profiled'].includes(r.outcome)
}

function isDownloadGood(q:QueueItem):boolean{
 const a=q.attempt
 if(!a?.result)return false
 if(downloadNoDataReason(a))return false
 return !!downloadReading(a)&&downloadTone(a,false)==='good'
}

export function classifyQueueIssue(q:QueueItem):QueueFailureClassification|null{
 if(q.project==='latency')return null
 if(['queued','running','cancelling','not_executed','cancelled','user_cancelled','interrupted'].includes(q.state))return null
 const r=q.attempt?.result
 const outcome=r?.outcome||''
 if(['not_executed','cancelled','user_cancelled','interrupted'].includes(outcome))return null
 if(q.project==='service'&&isServiceGood(q))return null
 if(q.project==='download'&&isDownloadGood(q))return null
 const persistErr=q.attempt?.persistence_error||''
 const rawErr=(q.error&&!q.error.startsWith('保存失败')&&q.error!==persistErr?q.error:'')||r?.error_message||r?.summary||''
 const dlNoData=q.project==='download'?downloadNoDataReason(q.attempt):''
 const phase=r?.failure_phase||r?.phase
 const httpStatus=r?.http_status

 let category:QueueFailureCategory='other'
 if(outcome==='timed_out'||/超时/.test(dlNoData)||/timeout|deadline|timed_out|超时/i.test(rawErr)){
  category='timeout'
 }else if(['transport_error','connection_failed','local_path_unverified'].includes(outcome)||/refused|reset|EOF|closed|DNS|NXDOMAIN|no such host|tls|certificate|handshake|连接失败|连接被|读取失败|物理出口/i.test(rawErr)){
  category='transport'
 }else if(['region_blocked','region_limited','flapping','originals_only'].includes(outcome)){
  category='region_blocked'
 }else if(['challenge','verification_required','suspected_challenge'].includes(outcome)){
  category='challenge'
 }else if(['auth_failed','credentials_required','permission_denied','setup_required','rate_limited','source_rate_limited'].includes(outcome)||httpStatus===429||httpStatus===401){
  category='auth_quota'
 }else if(['http_rejected','banned','service_rejected','criteria_mismatch','response_too_short','time_limit','reachable'].includes(outcome)){
  category='http_or_rule'
 }
 const meta=categoryMeta[category]
 const fallback=dlNoData||(outcome?outcomeLabel(outcome):'未取得有效结果')
 const reason=dlNoData||formatReason(rawErr,phase,fallback)
 return {category,label:meta.label,tone:meta.tone,reason}
}

export function summarizeQueueFailures(queue:QueueItem[],catalog:ServiceRule[]):QueueFailureSummary{
 const byCategory=new Map<QueueFailureCategory,{items:QueueItem[];nodes:Set<string>;services:Set<string>;sampleReason:string}>()
 const allNodes=new Set<string>()
 const catalogNames=new Map(catalog.map(rule=>[rule.service_id,rule.name]))
 let totalItems=0

 for(const q of queue){
  const classified=classifyQueueIssue(q)
  if(!classified)continue
  totalItems++
  const nodeKey=key(q.node)
  allNodes.add(nodeKey)
  let bucket=byCategory.get(classified.category)
  if(!bucket){
   bucket={items:[],nodes:new Set(),services:new Set(),sampleReason:''}
   byCategory.set(classified.category,bucket)
  }
  bucket.items.push(q)
  bucket.nodes.add(nodeKey)
  if(q.project==='download'){
   bucket.services.add('下载测速')
  }else{
   const sid=q.serviceId||q.attempt?.service_id||''
   if(sid)bucket.services.add(catalogNames.get(sid)||q.attempt?.rule?.name||sid)
  }
  if(!bucket.sampleReason&&classified.reason)bucket.sampleReason=classified.reason
 }

 const groups:QueueFailureGroup[]=[...byCategory.entries()]
  .sort(([a,va],[b,vb])=>vb.items.length-va.items.length||categoryMeta[a].order-categoryMeta[b].order)
  .map(([category,bucket])=>({
   category,
   label:categoryMeta[category].label,
   tone:categoryMeta[category].tone,
   items:bucket.items,
   nodeCount:bucket.nodes.size,
   serviceLabels:[...bucket.services],
   sampleReason:bucket.sampleReason,
  }))

 return {totalItems,totalNodes:allNodes.size,groups}
}

export function buildRetestPlanDraft(items:QueueItem[],currentNodes:NodeOption[],catalog:ServiceRule[]):RetestPlanDraft{
 const nodeMap=new Map<string,NodeOption>()
 const seenItemNodes=new Set<string>()
 let missingNodeCount=0
 const serviceIds=new Set<string>()
 const reputationServiceIds=new Set<string>()
 let hasDownload=false

 for(const item of items){
  const k=key(item.node)
  if(!seenItemNodes.has(k)){
   seenItemNodes.add(k)
   const current=currentNodes.find(n=>key(n)===k&&n.node_key===item.node.node_key)
   if(current)nodeMap.set(k,current)
   else missingNodeCount++
  }
  if(item.project==='download'){
   hasDownload=true
  }else if(item.project==='service'){
   const sid=item.serviceId||item.attempt?.service_id||''
   if(!sid)continue
   const rule=catalog.find(r=>r.service_id===sid)
   if(isReputationRule(rule||{service_id:sid}))reputationServiceIds.add(sid)
   else serviceIds.add(sid)
  }
 }

 const projects:Exclude<Project,'combined'>[]=[]
 if(hasDownload)projects.push('download')
 if(serviceIds.size>0)projects.push('service')

 return {
  nodes:[...nodeMap.values()],
  missingNodeCount,
  projects,
  serviceIds:[...serviceIds],
  includeReputation:reputationServiceIds.size>0,
  reputationServiceIds:[...reputationServiceIds],
 }
}
