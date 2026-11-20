<script setup lang="ts">
import {nodeDisplayName} from '../nodePresentation'
import {computed,ref,watch,nextTick,onMounted,onBeforeUnmount} from 'vue'
import {api,saveFile} from '../api'
import {readLatencyBatches} from '../latencyReportData'
import {bytes,key,median,targets,type BatchItem,type LatencyBatch,type LatencyBatchProgressItem,type LatencyTest} from '../domain'
import {latencyTargetId} from '../latencyTargets'
import LatencyReport from './LatencyReport.vue'
import {latencySampleState} from '../presentation'
import {downloadNoDataReason} from '../measurementRounds'
import {entryCountry,mainlandNotice} from '../entryPath'
import {buildRetestPlanDraft,classifyQueueIssue,summarizeQueueFailures,type QueueFailureCategory} from '../queueReport'
import {useWorkspace} from '../workspace'
import {projectNames,isReputationRule,stateLabel,outcomeLabel,outcomeTone,downloadReading,downloadTone,downloadOutcome,downloadSourceName,type QueueItem} from '../domain'
import Modal from './Modal.vue'
const w=useWorkspace(),retrying=ref(''),refreshingToken=ref(false),tokenNotice=ref(''),progressTitle=ref<HTMLElement>(),nowMs=ref(Date.now())
let elapsedTimer:ReturnType<typeof setInterval>|undefined
onMounted(()=>{
 nextTick(()=>progressTitle.value?.focus({preventScroll:true}))
 elapsedTimer=setInterval(()=>{if(compactLatency.value&&w.busy.value)nowMs.value=Date.now()},1000)
})
onBeforeUnmount(()=>{if(elapsedTimer)clearInterval(elapsedTimer)})
const report=computed(()=>w.reportSelection.value),previous=ref<LatencyBatch[]>([])
const airportNames=computed(()=>Object.fromEntries(w.nodes.value.map(n=>[n.profile_id,w.nodeAirportName(n)])))
// 只有本机在境外、且本轮确实有大陆入口的节点时，才在“偏慢”里解释绕路。
const entryNotice=computed(()=>{
 const local=w.localPosition.value?.country_code
 if(!local||local==='CN')return ''
 for(const item of report.value?.batch.items||[]){
  const node=w.nodes.value.find(n=>key(n)===key(item))
  const notice=mainlandNotice(entryCountry(node?w.entryFor(node):undefined),local)
  if(notice)return notice
 }
 return ''
})
watch(report,async selected=>{previous.value=selected?.previous||[];if(selected&&!selected.monitor&&!selected.previous.length){try{const all=await readLatencyBatches();if(report.value===selected)previous.value=all}catch(e){w.fail(e)}}},{immediate:true})
function currentNodes(items:BatchItem[]){return items.flatMap(i=>{const n=w.nodes.value.find(n=>key(n)===key(i)&&n.node_key===i.node_key);return n?[n]:[]})}
function retryReport(items:BatchItem[]){const nodes=currentNodes(items);if(!nodes.length){w.notify('这些历史节点的配置已变化，请重新选择');return}w.openPlan(nodes);if(w.plan.value){w.plan.value.projects=['latency'];w.plan.value.includeReputation=false;w.plan.value.target=report.value?.batch.target_id||'all';w.plan.value.samples=report.value?.batch.sample_count||6;w.plan.value.timeout=report.value?.batch.timeout_seconds||5}w.queueOpen.value=false}
async function exportReport(items:BatchItem[]){try{const nodes=currentNodes(items);if(nodes.length!==items.length){w.notify(`${items.length-nodes.length} 个报告节点的配置已变化或已移除，本次未导出。请在工作台明确选择当前节点后导出。`);return}const result=await api.exportClash(nodes);await saveFile(result.yaml_content,'cst-usable.yaml','application/x-yaml')}catch(e){w.fail(e)}}
async function inspectReport(item:BatchItem){
 if(!item.result?.samples?.length){w.notify('这项未执行，没有采样轮次');return}
 const n=currentNodes([item])[0]
 if(!n){
  const end=Date.parse(item.result.finished_at||report.value?.batch.requested_at||''),target=report.value?.batch.target_id
  w.inspectNode.value={...item,profile_name:airportNames.value[item.profile_id]||'历史账户',type:item.result.node_type||'',country_code:'',country_flag:'',historyProject:'latency',historySource:report.value?.monitor?'monitor':'manual',historyTargetId:targets.some(t=>t.id===target)?target:undefined,historyUntil:Number.isFinite(end)?new Date(end+1000).toISOString():undefined}
  w.queueOpen.value=false;return
 }
 w.latencyReportFocus.value={nodeKey:key(n),id:report.value?.monitor?report.value.batch.batch_id:item.result.attempt_id,test:item.result}
 w.queueOpen.value=false;w.project.value='latency';const airport=w.nodeAirportId(n);if(w.airportFilter.value!==airport)w.viewAirport(airport);w.search.value='';w.region.value='all';w.excludeRegion.value=false;w.subscription.value='all';w.downloadFullOnly.value=false;w.page.value='home';await nextTick();w.viewRequest.value=n
}
const batchItems=computed(()=>new Map(w.batches.value.flatMap(b=>(b.items||[]).map(i=>[i.item_id,i] as const))))
const counts=computed(()=>{
 const rows=w.queue.value,unknown=w.pendingStart.value?rows.filter(q=>q.id===w.pendingStart.value?.requestId||w.pendingStart.value?.kind==='latency'&&q.project==='latency'&&q.state==='running'&&!q.batchId).length:0
 return {queued:rows.filter(q=>q.state==='queued').length,running:Math.max(0,rows.filter(q=>['running','cancelling'].includes(q.state)).length-unknown),unknown,notRun:rows.filter(q=>['not_executed','cancelled'].includes(q.state)||resultStatus(q)?.notExecuted).length,saveFailed:rows.filter(saveFailed).length,saving:rows.filter(q=>persistenceState(q)==='saving').length}
})
function downloadBytes(q:QueueItem){return q.attempt?.result?.bytes_read??q.downloadBytesRead}
const downloadTransfer=computed(()=>{
 const rows=w.queue.value.filter(q=>q.project==='download')
 const active=rows.find(q=>['running','cancelling'].includes(q.state))
 return {visible:rows.length>0,active,activeBytes:active?downloadBytes(active):undefined,
   total:rows.reduce((sum,q)=>sum+(downloadBytes(q)??0),0),recovered:rows.some(q=>q.recovered)}
})
const isAntigravity=(q:QueueItem)=>q.project==='service'&&(q.serviceId||q.attempt?.service_id)==='antigravity'
const modelConfirmed=(q:QueueItem)=>q.attempt?.result?.outcome==='matched'&&q.attempt.result.details?.antigravity_progress==='真实模型已回答'
const antigravityAuthFailed=computed(()=>w.queue.value.some(q=>isAntigravity(q)&&['auth_failed','credentials_required'].includes(q.attempt?.result?.outcome||'')))
async function discoverLocalToken(){
 if(refreshingToken.value)return
 refreshingToken.value=true;tokenNotice.value=''
 try{
  const status=await api.discoverToken()
  w.token.value=status
  tokenNotice.value=status.has_token?`已刷新并绑定（${status.source||'本地检测'}），可重新发起检测`:'未检测到可用凭据，请在设置中添加'
 }catch(e){
  tokenNotice.value=e instanceof Error?e.message:String(e)
 }finally{
  refreshingToken.value=false
 }
}
const antigravitySummary=computed(()=>{
 const rows=w.queue.value.filter(isAntigravity);if(!rows.length)return ''
 const processed=rows.filter(q=>['completed','failed','not_executed','cancelled','interrupted'].includes(q.state))
 const confirmed=processed.filter(modelConfirmed).length,refused=processed.filter(q=>q.attempt?.result?.outcome==='region_blocked').length
 const flapping=processed.filter(q=>q.attempt?.result?.outcome==='flapping').length
 return `Antigravity：已处理 ${processed.length}/${rows.length} · 确认模型可用 ${confirmed}${flapping?` · 偶发送中/漂移 ${flapping}`:''} · 地区拒绝 ${refused} · 未确认 ${processed.length-confirmed-refused-flapping}`
})
function resultStatus(q:QueueItem):{label:string;tone:string;notExecuted?:boolean}|null{
 if(q.project==='service'&&['completed','failed'].includes(q.state)){
  const result=q.attempt?.result
  if(!result)return {label:'已结束 · 无结果',tone:''}
  const outcome=result.outcome
  if(isAntigravity(q)&&outcome==='matched')return {label:modelConfirmed(q)?'确认模型可用':'未确认模型可用',tone:modelConfirmed(q)?'good':'warn'}
  if(isAntigravity(q)&&outcome==='flapping')return {label:'偶发送中 / 漂移',tone:'warn'}
  if(isAntigravity(q)&&outcome==='region_blocked')return {label:'地区拒绝',tone:'warn'}
  if((q.serviceId||q.attempt?.service_id)==='cloudflare_challenge'){
   if(outcome==='profiled')return {label:'未触发 CF 盾',tone:'good'}
   if(outcome==='verification_required')return {label:'部分弹 CF 盾',tone:'warn'}
   if(outcome==='challenge')return {label:'易弹 CF 验证',tone:'bad'}
  }
  if(['not_executed','cancelled','user_cancelled','interrupted'].includes(outcome))return {label:stateLabel(outcome==='user_cancelled'?'cancelled':outcome),tone:'',notExecuted:true}
  if(['timed_out','transport_error','rate_limited'].includes(outcome))return {label:outcomeLabel(outcome)+' · 未确认',tone:'warn'}
  return {label:outcomeLabel(outcome),tone:outcomeTone(outcome)}
 }
 if(q.project==='download'&&!['queued','running','cancelling'].includes(q.state)){
  const a=q.attempt
  const result=a?.result
  if(!a||!result)return {label:'已结束 · 无结果',tone:''}
  const outcome=result.outcome
  if(result.error_message?.includes('备用源'))return {label:result.error_message,tone:'warn'}
  if(['not_executed','cancelled','user_cancelled','interrupted'].includes(outcome)){
   const read=result.bytes_read>0?` · 已读 ${bytes(result.bytes_read)}`:''
   return {label:stateLabel(outcome==='user_cancelled'?'cancelled':outcome)+read,tone:'',notExecuted:true}
  }
  if(downloadNoDataReason(a))return {label:downloadNoDataReason(a),tone:'warn'}
  if(downloadOutcome(a)==='response_too_short')return {label:outcomeLabel('response_too_short'),tone:'bad'}
  const reading=downloadReading(a)
  const end=a.rule.mode==='adaptive'&&reading?.label==='未稳定均速'?` · ${outcomeLabel(result.end_reason)}`:''
  if(reading)return {label:`${downloadSourceName(a)} · ${reading.label} ${reading.value.toFixed(2)} MiB/s · ${(result.bytes_read/1048576).toFixed(1)} MiB${end}`,tone:downloadTone(a,false)}
  if(outcome==='source_rate_limited'||result.http_status===429)return {label:'下载源限流',tone:'warn'}
  if(result.bytes_read>0)return {label:`${(result.bytes_read/1048576).toFixed(2)} MB · 中断`,tone:'warn'}
  if(['timed_out','transport_error','connection_failed'].includes(outcome))return {label:outcomeLabel(outcome)+' · 未确认',tone:'warn'}
  return {label:outcomeLabel(outcome),tone:outcomeTone(outcome)}
 }
 if(q.project!=='latency'||!['completed','failed'].includes(q.state))return null
 const item=batchItems.value.get(q.itemId||'')
 const result=item?.result
 if(!result||!result.total_samples)return {label:q.state==='completed'?'已结束 · 无采样':stateLabel(q.state),tone:q.state==='failed'?'bad':''}
 if(result.samples?.length&&result.samples.every(sample=>latencySampleState(sample).executionState==='not_executed'))return {label:'未执行 · 条件不足',tone:'',notExecuted:true}
 const ok=result.success_samples,total=result.total_samples
 return {label:`${ok===total?'已响应':ok?'部分响应':'无响应'} · ${ok}/${total}`,tone:ok===total?'good':ok?'warn':'bad'}
}
// Show recorded timing facts without guessing which proxy handshake failed.
function servicePhaseNote(q:QueueItem){
 if(q.project!=='service'||!['completed','failed'].includes(q.state)||!q.attempt?.result)return ''
 const r=q.attempt.result,d=r.details
 if(!d||!isAntigravity(q)&&!['timed_out','transport_error','unknown'].includes(r.outcome))return ''
 const ms=d.proxy_connect_ms,written=d.http_requests_written
 if((typeof ms!=='string'&&typeof ms!=='number')||String(ms).trim()===''||(typeof written!=='string'&&typeof written!=='number')||String(written).trim()==='')return ''
 const duration=Number(ms)
 if(!Number.isFinite(duration)||duration<0||!isAntigravity(q)&&Number(written)!==0)return ''
 if(isAntigravity(q)){
  const pending=Number(d.proxy_connect_pending||0),finished=Number(d.proxy_connect_attempts||0)-pending
  return `代理建连累计约 ${(duration/1000).toFixed(2)} 秒 · ${d.proxy_connect_attempts===undefined?'次数未记录':`已结束 ${finished} 次${pending?` · 另 ${pending} 次未结束`:''}`} · ${d.exit_observation?'含 Cloudflare 出口观察与 Google 请求':'各请求累计，非单次 Google 耗时'}`
 }
 return `代理建连累计约 ${(duration/1000).toFixed(2)} 秒 · 未记录完整 HTTP 请求写出，服务结论未确认`
}
function antigravityEvidenceLine(q:QueueItem){
 const r=q.attempt?.result;if(!r)return ''
 if((q.serviceId||q.attempt?.service_id)==='cloudflare_challenge'){
  const d=r.details||{},parts:string[]=[]
  if(d.cf_challenge_detail)parts.push(String(d.cf_challenge_detail))
  if(d.cf_colo)parts.push(`CF 机房 ${d.cf_colo}`)
  let path:{interface?:string;socket_bind_verified?:boolean}={};try{path=JSON.parse(String(d.network_path||'{}'))}catch{}
  if(path.socket_bind_verified&&path.interface)parts.push(`${path.interface} 绕过 TUN`)
  return parts.join(' · ')
 }
 if(!isAntigravity(q))return ''
 const d=r.details||{},parts:string[]=[]
 if(d.checked_model)parts.push(`模型 ${d.checked_model}${d.models_listed?`（可选 ${d.models_listed}/${d.all_models_count||d.models_listed}）`:''}`)
 if(d.reply_preview)parts.push(`实测回答“${d.reply_preview}”`)
 if(d.google_ttfb_ms)parts.push(`Google 响应 ${d.google_ttfb_ms}ms`)
 if(d.google_region)parts.push(`Google 判定 ${d.google_region}`)
 if(d.country_code)parts.push(`CF 观察 ${d.country_code}${d.cloudflare_colo?` (${d.cloudflare_colo})`:''}${d.ip?` ${d.ip}`:''}`)
 if(Number(d.sample_planned)>1)parts.push(`抽样 ${d.sample_success||0}/${d.sample_total||d.sample_planned}${d.blocked_ips?` · 受阻出口 ${d.blocked_ips}`:d.ip_drift_detected==='true'?' · 存在出口漂移':''}`)
 let path:{interface?:string;socket_bind_verified?:boolean}={};try{path=JSON.parse(String(d.network_path||'{}'))}catch{}
 if(path.socket_bind_verified&&path.interface)parts.push(`${path.interface} 绕过 TUN`)
 return parts.join(' · ')
}
function antigravityDetails(q:QueueItem){
 const r=q.attempt?.result;if(!isAntigravity(q)||!r)return []
 const d=r.details||{},steps:Record<string,string>={loadCodeAssist:'账号项目查询',fetchAvailableModels:'模型目录查询','streamGenerateContent?alt=sse':'模型生成',model_response:'模型回答确认',fetchUserInfo:'Google 地区识别',drift_validation:'多轮出口漂移'}
 const phases:Record<string,string>={proxy_connect:'节点代理建连',tls_handshake:'目标 TLS 握手',request_write:'请求写出',response_headers:'等待响应头',response_read:'读取响应体'}
 let path:{address_family?:string;interface?:string}={};try{path=JSON.parse(String(d.network_path||'{}'))}catch{}
 const latencyRange=d.google_min_ttfb_ms&&d.google_max_ttfb_ms?`（区间 ${d.google_min_ttfb_ms}–${d.google_max_ttfb_ms} ms）`:''
 return [`${d.antigravity_progress||'模型进度未记录'}${d.checked_model?` · 模型：${d.checked_model}`:''}`,...(d.reply_preview?[`模型实测回答：“${d.reply_preview}”`]:[]),...(d.google_ttfb_ms?[`Google API 响应：${d.google_ttfb_ms} ms${latencyRange}`]:[]),...(d.blocked_ips?[`受阻出口 IP：${d.blocked_ips}${d.blocked_regions?`（${d.blocked_regions}）`:''}`]:[]),...(d.models_listed?[`可选轻量模型：${d.models_listed} / 总模型 ${d.all_models_count||d.models_listed}`]:[]),...(d.available_flash_models?[`候选 Flash 模型：${d.available_flash_models}`]:[]),...(d.all_model_ids?[`账号返回全部模型：${d.all_model_ids}`]:[]),`Google 内部地区判定：${d.google_region||'未返回'}`,`账号／模型阶段：${steps[r.failure_phase||'']||r.failure_phase||'已结束'}`,`HTTP 状态：${r.http_status??'未收到'}`,`请求失败阶段：${phases[String(d.request_failure_phase)]||d.request_failure_phase||'未记录'}`,`超时来源：${d.timeout_origin==='checker_deadline'?'检查总时限':d.timeout_origin==='transport'?'传输超时':'未记录'}`,`HTTP 请求写出累计：${d.http_requests_written??'未记录'}（含出口观察）`,`地址族：${path.address_family||'未记录'} · 网卡：${path.interface||'未记录'}`,`Cloudflare 独立出口观察：${d.country_code?`${d.country_code}${d.cloudflare_colo?` (${d.cloudflare_colo})`:''}${d.ip?` · ${d.ip}`:''}（不代表 Google 出口）`:d.exit_observation_state||'未记录'}`]
}
// 执行与保存是两个独立属性：先看有没有跑完，再看结果有没有落库。
function latencyItem(q:QueueItem){return q.project==='latency'?batchItems.value.get(q.itemId||''):undefined}
function unknownStart(q:QueueItem){const p=w.pendingStart.value;return !!p&&(q.id===p.requestId||p.kind==='latency'&&q.project==='latency'&&q.state==='running'&&!q.batchId)}
function saveFailed(q:QueueItem){return q.attempt?.persistence_state==='failed'||!!q.error?.startsWith('保存失败')||latencyItem(q)?.persistence_state==='failed'}
function persistenceState(q:QueueItem){return q.project==='latency'?latencyItem(q)?.persistence_state:q.attempt?.persistence_state}
function persistenceError(q:QueueItem){return latencyItem(q)?.persistence_error||q.attempt?.persistence_error||(q.error?.startsWith('保存失败')?q.error:'')}
function saveState(q:QueueItem):{label:string;tone:string}{
 if(saveFailed(q))return {label:'保存失败',tone:'bad'}
 const state=persistenceState(q)
 if(state==='saving')return {label:'保存中',tone:'warn'}
 if(state==='saved')return {label:'已保存',tone:''}
 if(['queued','running','cancelling'].includes(q.state))return {label:'尚未结束',tone:'empty'}
 return {label:'—',tone:'empty'}
}
function errorText(q:QueueItem){const error=q.error;return (error&&!error.startsWith('保存失败')&&error!==persistenceError(q)?error:'')||latencyItem(q)?.error_message||latencyItem(q)?.result?.samples?.find(s=>!s.success)?.error||q.attempt?.result?.error_message||''}
function shortError(raw:string){
 if(!raw)return ''
 if(raw.includes('备用源')||/[\u4e00-\u9fff]/.test(raw))return raw
 if(/timeout|deadline|timed_out/i.test(raw))return '超时'
 if(/refused/i.test(raw))return '连接被拒绝'
 if(/reset/i.test(raw))return '连接被重置'
 if(/closed|EOF/i.test(raw))return '连接被关闭'
 if(/NXDOMAIN|no such host|DNS/i.test(raw))return '域名解析失败'
 if(/cancel/i.test(raw))return '已取消'
 return /[a-z]{3}/i.test(raw)?'连接失败':raw
}
function queueErrorLabel(q:QueueItem){
 const raw=errorText(q)
 if(q.project==='service'&&!['timed_out','transport_error','connection_failed'].includes(q.attempt?.result?.outcome||''))return raw
 const phase=q.attempt?.result?.failure_phase
 const text=q.project==='download'?downloadNoDataReason(q.attempt)||shortError(raw):shortError(raw)
 if(q.project==='service'&&phase&&!['timeout','http_status'].includes(phase)&&text&&!text.includes(phase))return `${text}（阶段：${phase}）`
 return text
}
type QueuePillFilter='all'|'good'|'issue'|'running'|'queued'|'notRun'
const queueView=ref<'focus'|'all'>('focus'),queueFilter=ref<QueuePillFilter>('all'),reasonFilter=ref<QueueFailureCategory|null>(null),queuePage=ref(1),pageSize=50
const failureSummary=computed(()=>summarizeQueueFailures(w.queue.value,w.catalog.value))
const isIssueItem=(q:QueueItem)=>(['completed','failed'].includes(q.state)&&resultStatus(q)?.tone!=='good'&&!resultStatus(q)?.notExecuted)||saveFailed(q)||!!q.error
const issueCount=computed(()=>w.queue.value.filter(isIssueItem).length)
const attentionQueue=computed(()=>w.queue.value.filter(q=>!['queued','completed'].includes(q.state)||unknownStart(q)||saveFailed(q)||persistenceState(q)==='saving'||!!q.error||q.state==='completed'&&resultStatus(q)?.tone!=='good'))
function matchesPill(q:QueueItem,filter:QueuePillFilter){
 if(filter==='all')return true
 if(filter==='good')return q.state==='completed'&&resultStatus(q)?.tone==='good'
 if(filter==='issue')return isIssueItem(q)
 if(filter==='running')return ['running','cancelling'].includes(q.state)&&!unknownStart(q)
 if(filter==='queued')return q.state==='queued'
 if(filter==='notRun')return ['not_executed','cancelled'].includes(q.state)||!!resultStatus(q)?.notExecuted
 return true
}
function togglePillFilter(next:QueuePillFilter){
 reasonFilter.value=null
 if(queueFilter.value===next&&queueView.value==='all'){queueFilter.value='all';return}
 queueView.value='all'
 queueFilter.value=next
}
function toggleReasonFilter(category:QueueFailureCategory){
 if(reasonFilter.value===category){reasonFilter.value=null;return}
 queueView.value='all'
 queueFilter.value='all'
 reasonFilter.value=category
}
function retestFailureItems(items:QueueItem[],label?:string){
 if(w.busy.value)return
 const draft=buildRetestPlanDraft(items,w.nodes.value,w.catalog.value)
 if(!draft.nodes.length){
  w.notify('这些异常节点的配置已变化或已移除，请在工作台重新选择')
  return
 }
 if(!draft.projects.length&&!draft.includeReputation){
  w.notify('未能识别可重测的项目，请手动配置检测计划')
  return
 }
 w.openPlan(draft.nodes)
 if(w.plan.value){
  w.plan.value.projects=draft.projects
  w.plan.value.serviceIds=draft.serviceIds
  w.plan.value.includeReputation=draft.includeReputation
  w.plan.value.reputationServiceIds=draft.reputationServiceIds
 }
 w.queueOpen.value=false
 if(draft.missingNodeCount>0){
  w.notify(`已为 ${draft.nodes.length} 个当前节点生成${label?`「${label}」`:'异常'}重测草稿（跳过 ${draft.missingNodeCount} 个配置已变节点）`)
 }
}
function queueProjectLabel(q:QueueItem){
 const sid=q.serviceId||q.attempt?.service_id;if(q.project==='service'&&sid){
  const rule=w.catalog.value.find(s=>s.service_id===sid)
  if(isReputationRule({service_id:sid,result_kind:rule?.result_kind}))return '出口与信誉'
 }
 return projectNames[q.project]
}
function cleanProfileName(raw?:string){
 if(!raw)return ''
 const parts=raw.split(/\s*[·/]\s*/).map(s=>s.trim()).filter(Boolean)
 return [...new Set(parts)].join(' · ')
}
const filteredQueue=computed(()=>{
 if(reasonFilter.value){
  return w.queue.value.filter(q=>classifyQueueIssue(q)?.category===reasonFilter.value)
 }
 const base=queueFilter.value!=='all'?w.queue.value:queueView.value==='all'?w.queue.value:attentionQueue.value
 return base.filter(q=>matchesPill(q,queueFilter.value))
})
const pageCount=computed(()=>Math.max(1,Math.ceil(filteredQueue.value.length/pageSize)))
const visibleQueue=computed(()=>filteredQueue.value.slice((queuePage.value-1)*pageSize,queuePage.value*pageSize))
watch([queueView,queueFilter,reasonFilter],()=>queuePage.value=1)
watch(failureSummary,summary=>{if(reasonFilter.value&&!summary.groups.some(g=>g.category===reasonFilter.value))reasonFilter.value=null})
watch(pageCount,count=>queuePage.value=Math.min(queuePage.value,count))
const succeeded=computed(()=>w.queue.value.filter(q=>q.state==='completed'&&resultStatus(q)?.tone==='good').length)
const compactLatency=computed(()=>!!w.activeBatch.value&&['queued','running','cancelling','saving'].includes(w.activeBatch.value.state)||w.running.value&&w.queue.value.length>0&&w.queue.value.every(q=>q.project==='latency'))
const progress=computed(()=>w.activeBatchProgress.value?.batch_id===w.activeBatch.value?.batch_id&&w.activeBatchProgress.value||{requested_at:w.activeBatch.value?.requested_at||'',total:w.activeBatch.value?.item_count||w.queue.value.length,processed:0,running:0,queued:0,all_success:0,partial_response:0,no_response:0,save_failed:0,not_executed:0,running_items:[] as LatencyBatchProgressItem[],recent_items:[] as LatencyBatchProgressItem[]})
const compactFilter=ref<'all'|'good'|'warn'|'bad'|'skipped'>('all')
const expandedLiveId=ref<string|null>(null)
const liveDetailCache=ref<Record<string,{loading:boolean;error?:string;test?:LatencyTest}>>({})
watch(()=>w.activeBatch.value?.batch_id,()=>{compactFilter.value='all';expandedLiveId.value=null;liveDetailCache.value={}})
function toggleCompactFilter(f:'good'|'warn'|'bad'|'skipped'){compactFilter.value=compactFilter.value===f?'all':f}
function liveItemCategory(item:LatencyBatchProgressItem):'good'|'warn'|'bad'|'skipped'{
 if(['not_executed','cancelled'].includes(item.execution_state))return 'skipped'
 if(item.total_samples>0&&item.success_samples===item.total_samples)return 'good'
 if(item.success_samples>0)return 'warn'
 return 'bad'
}
const filteredRecentItems=computed(()=>(progress.value.recent_items||[]).filter(item=>compactFilter.value==='all'||liveItemCategory(item)===compactFilter.value))
function compactFilterLabel(f:typeof compactFilter.value){
 return ({all:'全部已完成',good:'全部响应',warn:'部分响应',bad:'无响应',skipped:'未执行 / 取消'} as const)[f]
}
function findCachedLiveTest(item:LatencyBatchProgressItem):LatencyTest|undefined{
 const fromBatch=batchItems.value.get(item.item_id)?.result
 if(fromBatch)return fromBatch
 if(item.node_key&&item.node_identity_key&&item.config_revision_key){
  const list=w.latency.value[key({profile_id:item.profile_id,node_key:item.node_key,node_identity_key:item.node_identity_key,config_revision_key:item.config_revision_key})]||[]
  if(item.attempt_id){const matched=list.find(t=>t.attempt_id===item.attempt_id);if(matched)return matched}
 }
 return undefined
}
async function toggleLiveItemDetail(item:LatencyBatchProgressItem){
 if(expandedLiveId.value===item.item_id){expandedLiveId.value=null;return}
 expandedLiveId.value=item.item_id
 const cached=liveDetailCache.value[item.item_id]
 if(cached?.test||cached?.loading)return
 const local=findCachedLiveTest(item)
 if(local){liveDetailCache.value={...liveDetailCache.value,[item.item_id]:{loading:false,test:local}};return}
 if(!item.node_key||!item.node_identity_key||!item.config_revision_key)return
 liveDetailCache.value={...liveDetailCache.value,[item.item_id]:{loading:true}}
 try{
  const res=await api.latencyHistory({profile_id:item.profile_id,node_key:item.node_key,node_identity_key:item.node_identity_key,config_revision_key:item.config_revision_key},{limit:4})
  const found=(item.attempt_id?res.tests?.find(t=>t.attempt_id===item.attempt_id):undefined)||res.tests?.[0]
  liveDetailCache.value={...liveDetailCache.value,[item.item_id]:{loading:false,test:found,error:found?undefined:'暂未读取到逐站明细，结束后可在完整报告查看'}}
 }catch(e){
  liveDetailCache.value={...liveDetailCache.value,[item.item_id]:{loading:false,error:e instanceof Error?e.message:'读取失败'}}
 }
}
function liveSiteBreakdown(test:LatencyTest){
 return targets.map(t=>{
  const samples=(test.samples||[]).filter(s=>latencyTargetId(s.target||test.target||'')===t.id)
  const okSamples=samples.filter(s=>s.success&&Number.isFinite(s.latency_ms))
  const stableVals=(samples.slice(1).filter(s=>s.success&&Number.isFinite(s.latency_ms)).map(s=>s.latency_ms))
  const ms=median(stableVals.length?stableVals:okSamples.map(s=>s.latency_ms))
  const httpCode=samples.map(s=>s.error?.match(/HTTP (\d+)/)?.[1]).find(Boolean)||''
  const failErr=samples.find(s=>!s.success&&s.error)?.error||''
  const tone=!samples.length?'empty':okSamples.length===0?'bad':okSamples.length<samples.length||httpCode||(ms!==null&&ms>=1200)?'warn':'good'
  return {id:t.id,name:t.name,shortName:t.id==='cloudflare'?'CF':t.name,ok:okSamples.length,total:samples.length,ms,httpCode,failErr,tone}
 })
}
const readyReportBatch=computed(()=>{
 const active=w.activeBatch.value
 if(active&&['completed','completed_with_issues','completed_with_save_failures','cancelled','cancelled_with_issues','cancelled_with_save_failures','interrupted','interrupted_with_issues','interrupted_with_save_failures','failed'].includes(active.state)){
  const full=w.batches.value.find(b=>b.batch_id===active.batch_id&&b.items?.length)
  if(full)return full
 }
 if(active&&w.latestReportBatch.value?.batch_id===active.batch_id)return w.latestReportBatch.value
 return null
})
const compactPercent=computed(()=>progress.value.total?Math.min(100,Math.round(progress.value.processed/progress.value.total*100)):0)
const compactTiming=computed(()=>{
 const startedRaw=progress.value.requested_at||w.activeBatch.value?.requested_at
 const started=startedRaw?Date.parse(startedRaw):NaN
 if(!Number.isFinite(started))return {elapsed:'',eta:''}
 const sec=Math.max(0,Math.floor((nowMs.value-started)/1000))
 const fmt=(s:number)=>s>=60?`${Math.floor(s/60)} 分 ${s%60} 秒`:`${s} 秒`
 const remaining=Math.max(0,progress.value.total-progress.value.processed)
 const eta=progress.value.processed>=2&&remaining>0&&sec>0?`约剩 ${fmt(Math.max(1,Math.round(sec/progress.value.processed*remaining)))}`:''
 return {elapsed:`已用 ${fmt(sec)}`,eta}
})
function liveItemTone(item:LatencyBatchProgressItem){
 if(['not_executed','cancelled'].includes(item.execution_state))return 'empty'
 if(item.total_samples>0&&item.success_samples===item.total_samples)return 'good'
 if(item.success_samples>0)return 'warn'
 return 'bad'
}
function liveItemLabel(item:LatencyBatchProgressItem){
 if(['not_executed','cancelled'].includes(item.execution_state))return stateLabel(item.execution_state)
 if(item.total_samples>0&&item.success_samples===item.total_samples)return item.latency_ms?`${item.latency_ms} ms`:'全部响应'
 if(item.success_samples>0)return `${item.success_samples}/${item.total_samples}${item.latency_ms?` · ${item.latency_ms} ms`:''}`
 return '无响应'
}
const savingOnly=computed(()=>compactLatency.value?w.activeBatch.value?.state==='saving':counts.value.saving>0&&!counts.value.running&&!counts.value.queued)
const canCancel=computed(()=>w.busy.value&&(!!w.pendingStart.value||!savingOnly.value))
const headline=computed(()=>w.pendingStart.value?'正在确认启动状态':w.busy.value?(w.cancelRequested.value?'正在取消，已完成结果保留':savingOnly.value?'正在保存结果':compactLatency.value&&progress.value.total>0&&progress.value.processed>=progress.value.total?'正在整理本轮报告':'检测进行中'):w.done.value<w.queue.value.length?'检测已停止，部分项目没有运行完':counts.value.saveFailed?'检测已结束，部分结果未保存':'检测已结束')
const retryable=(q:QueueItem)=>!!(q.batchId&&q.itemId||q.project!=='latency'&&q.attempt?.result)
async function retry(q:QueueItem){retrying.value=q.id;try{if(q.batchId&&q.itemId)w.absorbBatch(await api.retryBatch(q.batchId,q.itemId));else if(q.attempt&&q.project!=='latency'){const a=await api.attemptAction(q.project,q.attempt.attempt_id,q.node,'retry-save',q.attempt.service_id||q.serviceId);q.attempt=a;q.error=a.persistence_error;w.mergeAttempt(q.project,a)}}catch(e){w.fail(e)}finally{retrying.value=''}}
</script>
<template><Modal :title="report?'延迟检测报告':'检测进度'" wide variant="cst-queue" @close="w.queueOpen.value=false;w.reportSelection.value=null">
<LatencyReport v-if="report" :batch="report.batch" :previous="previous" :airport-names="airportNames" :monitor="report.monitor" :entry-notice="entryNotice" @retry="retryReport" @export="exportReport" @inspect="inspectReport"/>
<template v-else>
  <div class="queue-summary cst-queue-head">
    <div><strong ref="progressTitle" class="queue-title" tabindex="-1" autofocus>{{headline}}</strong><span v-if="!w.pendingStart.value">{{compactLatency?`已处理 ${progress.processed} / ${progress.total} 个节点`:w.queue.value.length?`已处理 ${w.done.value} / ${w.queue.value.length} 项`:'后台任务'}}</span></div>
    <div class="cst-queue-actions"><button v-if="readyReportBatch" class="button primary" @click="w.openLatencyReport(readyReportBatch)">查看本轮报告</button><button class="button ghost" @click="w.syncActive()">刷新进度</button><button v-if="w.pendingStart.value?.body" class="button" :disabled="w.running.value||w.retryingStart.value" @click="w.retryPending()">{{w.retryingStart.value?'正在确认…':'确认启动状态'}}</button><button v-if="canCancel" class="button danger-outline" :disabled="w.cancelRequested.value" @click="w.cancelTests()">{{w.cancelRequested.value?'正在取消…':'取消检测'}}</button></div>
  </div>
  <p v-if="w.pendingStart.value" class="queue-notice muted">后台还没确认这次启动；确认前不显示进度，也不会重复启动。</p>
  <div v-else class="progress-track" role="progressbar" aria-label="已处理检测项目" :aria-valuenow="compactLatency?progress.processed:w.done.value" :aria-valuemax="compactLatency?progress.total:w.queue.value.length" aria-valuemin="0"><i :style="{width:compactLatency?(progress.total?`${progress.processed/progress.total*100}%`:'0%'):(w.queue.value.length?`${w.done.value/w.queue.value.length*100}%`:'0%')}"/></div>
  <p v-if="w.error.value" class="inline-error" role="alert">{{w.error.value}}</p>
  <dl v-if="downloadTransfer.visible" class="queue-download-transfer" aria-label="下载测速读量">
    <div v-if="downloadTransfer.active" class="queue-transfer-node">
      <dt>正在测速</dt>
      <dd>{{nodeDisplayName(downloadTransfer.active.node)}}<small v-if="downloadTransfer.active.node.profile_name">{{downloadTransfer.active.node.profile_name}}</small></dd>
    </div>
    <div v-if="downloadTransfer.active">
      <dt>当前已读</dt>
      <dd :title="downloadTransfer.activeBytes===undefined?undefined:downloadTransfer.activeBytes+' 字节'">{{downloadTransfer.activeBytes===undefined?'等待下载数据':bytes(downloadTransfer.activeBytes)}}</dd>
    </div>
    <div>
      <dt>{{downloadTransfer.recovered?'已恢复测速读取':'本轮下载正文'}}</dt>
      <dd :title="downloadTransfer.total+' 字节'">{{bytes(downloadTransfer.total)}}</dd>
    </div>
  </dl>
  <template v-if="compactLatency">
    <div v-if="!w.pendingStart.value" class="compact-live-meta">
      <strong class="compact-live-pct">{{compactPercent}}%</strong>
      <span v-if="progress.running" class="compact-live-pill running"><i class="cst-queue-dot running"/>正在测 {{progress.running}} 个</span>
      <span v-if="progress.queued" class="compact-live-pill">排队 {{progress.queued}} 个</span>
      <span v-if="compactTiming.elapsed" class="compact-live-time">{{compactTiming.elapsed}}<template v-if="compactTiming.eta"> · {{compactTiming.eta}}</template></span>
    </div>
    <div v-if="!w.pendingStart.value" class="compact-progress-counts" role="group" aria-label="按响应状态筛选已完成节点">
      <button type="button" class="compact-count-btn good" :class="{active:compactFilter==='good'}" :aria-pressed="compactFilter==='good'" :disabled="!progress.all_success&&compactFilter!=='good'" @click="toggleCompactFilter('good')">全部响应 <strong>{{progress.all_success}}</strong></button>
      <button type="button" class="compact-count-btn warn" :class="{active:compactFilter==='warn'}" :aria-pressed="compactFilter==='warn'" :disabled="!progress.partial_response&&compactFilter!=='warn'" @click="toggleCompactFilter('warn')">部分响应 <strong>{{progress.partial_response}}</strong></button>
      <button type="button" class="compact-count-btn bad" :class="{active:compactFilter==='bad'}" :aria-pressed="compactFilter==='bad'" :disabled="!progress.no_response&&compactFilter!=='bad'" @click="toggleCompactFilter('bad')">无响应 <strong>{{progress.no_response}}</strong></button>
      <button v-if="progress.not_executed" type="button" class="compact-count-btn" :class="{active:compactFilter==='skipped'}" :aria-pressed="compactFilter==='skipped'" @click="toggleCompactFilter('skipped')">未执行/取消 <strong>{{progress.not_executed}}</strong></button>
      <button v-if="compactFilter!=='all'" type="button" class="compact-count-reset" @click="compactFilter='all'">显示全部</button>
    </div>
    <p v-if="progress.save_failed" class="inline-warning" role="alert">保存失败 {{progress.save_failed}} 项；结束后可在报告重试保存。</p>
    <section v-if="!w.pendingStart.value&&progress.running_items?.length" class="compact-live-section" aria-label="正在检测的节点">
      <header class="compact-live-head"><strong>正在检测</strong><small>并发 {{progress.running_items.length}} 个节点</small></header>
      <div class="compact-running-grid">
        <span v-for="item in progress.running_items" :key="item.item_id" class="compact-running-chip" :title="item.display_name">
          <i class="spinner" aria-hidden="true"/>
          <span>{{nodeDisplayName({display_name:item.display_name})}}</span>
          <small v-if="item.node_type">{{item.node_type}}</small>
        </span>
      </div>
    </section>
    <section v-if="!w.pendingStart.value&&progress.recent_items?.length" class="compact-live-section" aria-label="最近完成节点">
      <header class="compact-live-head"><strong>{{compactFilter==='all'?'最近完成':`${compactFilterLabel(compactFilter)}（${filteredRecentItems.length}）`}}</strong><small>{{progress.processed>progress.recent_items.length?`已加载最新 ${progress.recent_items.length} 条`:`共 ${filteredRecentItems.length} 条`}} · 点击节点展开四站明细</small></header>
      <div v-if="filteredRecentItems.length" class="compact-recent-list">
        <div v-for="item in filteredRecentItems" :key="item.item_id" class="compact-recent-card" :class="{expanded:expandedLiveId===item.item_id}">
          <button type="button" class="compact-recent-row" :aria-expanded="expandedLiveId===item.item_id" @click="toggleLiveItemDetail(item)">
            <div class="compact-recent-node">
              <strong>{{nodeDisplayName({display_name:item.display_name})}}</strong>
              <small v-if="item.error_message" class="compact-recent-err">{{shortError(item.error_message)}}</small>
              <small v-else-if="item.total_samples">{{item.success_samples}}/{{item.total_samples}} 次采样</small>
            </div>
            <span class="compact-recent-tail">
              <span class="badge" :class="liveItemTone(item)">{{liveItemLabel(item)}}</span>
              <span class="compact-recent-chev" aria-hidden="true">{{expandedLiveId===item.item_id?'▾':'›'}}</span>
            </span>
          </button>
          <div v-if="expandedLiveId===item.item_id" class="compact-recent-detail">
            <p v-if="liveDetailCache[item.item_id]?.loading" class="muted small">正在读取四站采样明细…</p>
            <template v-else-if="liveDetailCache[item.item_id]?.test">
              <div class="compact-site-grid">
                <span v-for="s in liveSiteBreakdown(liveDetailCache[item.item_id].test!)" :key="s.id" class="compact-site-chip" :class="s.tone" :title="s.failErr||`${s.name}: ${s.ok}/${s.total}`">
                  <b>{{s.shortName}}</b>
                  <span>{{s.ms!==null?`${Math.round(s.ms)} ms`:s.total>0?'超时':'未测'}}</span>
                  <small v-if="s.total">{{s.ok}}/{{s.total}}</small>
                  <em v-if="s.httpCode">HTTP {{s.httpCode}}</em>
                </span>
              </div>
              <p v-if="item.error_message" class="compact-detail-err">错误明细：{{item.error_message}}</p>
            </template>
            <template v-else>
              <p class="muted small">{{liveDetailCache[item.item_id]?.error||(item.total_samples?`本节点共采样 ${item.total_samples} 次，成功 ${item.success_samples} 次`:'暂无逐站明细')}}</p>
              <p v-if="item.error_message" class="compact-detail-err">错误明细：{{item.error_message}}</p>
            </template>
          </div>
        </div>
      </div>
      <p v-else class="queue-empty">当前已加载的 {{progress.recent_items.length}} 条记录中没有「{{compactFilterLabel(compactFilter)}}」项，全批结束后可在完整报告查看全部。</p>
    </section>
    <p class="queue-note">点击上方状态卡可筛选已完成节点，点击任意节点可展开四站明细；全部结束后自动打开完整延迟报告。</p>
  </template>
  <template v-else>
    <div class="queue-counts" role="group" aria-label="按检测状态快速筛选">
      <button type="button" class="queue-pill" :class="{active:queueView==='all'&&queueFilter==='all'&&!reasonFilter}" :aria-pressed="queueView==='all'&&queueFilter==='all'&&!reasonFilter" @click="reasonFilter=null;queueView='all';queueFilter='all'">已处理 {{w.done.value}} / {{w.queue.value.length}}</button>
      <button type="button" class="queue-pill good" :class="{active:queueFilter==='good'&&!reasonFilter}" :aria-pressed="queueFilter==='good'&&!reasonFilter" @click="togglePillFilter('good')">已成功 {{succeeded}}</button>
      <button v-if="issueCount" type="button" class="queue-pill warn" :class="{active:queueFilter==='issue'&&!reasonFilter}" :aria-pressed="queueFilter==='issue'&&!reasonFilter" @click="togglePillFilter('issue')">需留意 / 异常 {{issueCount}}</button>
      <button v-if="counts.running" type="button" class="queue-pill running" :class="{active:queueFilter==='running'&&!reasonFilter}" :aria-pressed="queueFilter==='running'&&!reasonFilter" @click="togglePillFilter('running')">执行 {{counts.running}}</button>
      <button v-if="counts.queued" type="button" class="queue-pill" :class="{active:queueFilter==='queued'&&!reasonFilter}" :aria-pressed="queueFilter==='queued'&&!reasonFilter" @click="togglePillFilter('queued')">排队 {{counts.queued}}</button>
      <button v-if="counts.notRun" type="button" class="queue-pill warn" :class="{active:queueFilter==='notRun'&&!reasonFilter}" :aria-pressed="queueFilter==='notRun'&&!reasonFilter" @click="togglePillFilter('notRun')">未运行/取消 {{counts.notRun}}</button>
      <span v-if="counts.unknown" class="queue-pill warn">启动未知 {{counts.unknown}}</span>
      <span v-if="counts.saving" class="queue-pill">保存中 {{counts.saving}}</span>
      <span v-if="counts.saveFailed" class="queue-pill bad">保存失败 {{counts.saveFailed}}</span>
    </div>
    <p v-if="antigravitySummary" class="queue-counts antigravity-summary" role="status">{{antigravitySummary}}</p>
    <p v-if="antigravityAuthFailed" class="queue-counts antigravity-token-bar"><button class="button ghost small" :disabled="refreshingToken" @click="discoverLocalToken">{{refreshingToken?'正在刷新凭据…':'重新检测 / 刷新本地凭据'}}</button><span v-if="tokenNotice" class="small muted">{{tokenNotice}}</span></p>
    <section v-if="failureSummary.groups.length" class="queue-issue-report" aria-label="本轮异常按原因汇总">
      <header class="queue-issue-head">
        <div>
          <strong>异常按原因汇总</strong>
          <small>共 {{failureSummary.totalItems}} 项异常 · 涉及 {{failureSummary.totalNodes}} 个节点 · 点原因可筛选明细</small>
        </div>
        <button type="button" class="button small" :disabled="w.busy.value" :title="w.busy.value?'当前检测进行中，结束后可生成重测计划':'打开检测配置草稿，仅包含异常节点与项目（确认后才会开始）'" @click="retestFailureItems(failureSummary.groups.flatMap(g=>g.items))">一键重测异常（{{failureSummary.totalNodes}} 节点）</button>
      </header>
      <div class="queue-issue-groups">
        <article v-for="group in failureSummary.groups" :key="group.category" class="queue-issue-group" :class="[group.tone,{active:reasonFilter===group.category}]">
          <button type="button" class="queue-issue-main" :aria-pressed="reasonFilter===group.category" @click="toggleReasonFilter(group.category)">
            <div class="queue-issue-title">
              <span class="queue-issue-dot" :class="group.tone" aria-hidden="true"/>
              <strong>{{group.label}}</strong>
              <span class="queue-issue-meta">{{group.items.length}} 项 · {{group.nodeCount}} 节点</span>
            </div>
            <p v-if="group.serviceLabels.length" class="queue-issue-services">{{group.serviceLabels.slice(0,4).join('、')}}{{group.serviceLabels.length>4?` 等 ${group.serviceLabels.length} 项`:''}}</p>
            <small v-if="group.sampleReason" class="queue-issue-sample">{{group.sampleReason}}</small>
          </button>
          <button type="button" class="button ghost small queue-issue-retest" :disabled="w.busy.value" :title="w.busy.value?'检测进行中，结束后可重测':'仅针对此类异常生成重测计划草稿'" @click="retestFailureItems(group.items,group.label)">重测此类（{{group.items.length}}）</button>
        </article>
      </div>
    </section>
    <div v-if="!w.queue.value.length&&w.activeBatch.value" class="muted">恢复的后台批次包含 {{w.activeBatch.value.item_count}} 个节点；结果可在检测历史查看。</div>
    <div v-if="w.queue.value.length" class="queue-list-tools"><div class="queue-views" role="group" aria-label="检测项目范围"><button :aria-pressed="queueView==='focus'&&queueFilter==='all'&&!reasonFilter" @click="reasonFilter=null;queueFilter='all';queueView='focus'">进行中与需留意 <span>{{attentionQueue.length}}</span></button><button :aria-pressed="queueView==='all'&&queueFilter==='all'&&!reasonFilter" @click="reasonFilter=null;queueFilter='all';queueView='all'">全部项目 <span>{{w.queue.value.length}}</span></button></div><small>{{reasonFilter?`已按原因筛选：${failureSummary.groups.find(g=>g.category===reasonFilter)?.label||''}`:'点击上方状态或异常原因可直接筛选明细'}}<button v-if="reasonFilter" type="button" class="text-button small queue-clear-reason" @click="reasonFilter=null">清除筛选</button></small></div>
    <div v-if="visibleQueue.length" class="cst-queue-cols" aria-hidden="true"><span>节点与项目</span><span>执行</span><span>检测结果</span><span>保存</span></div>
    <div v-for="q in visibleQueue" :key="q.id" class="queue-row cst-queue-row">
      <div class="cst-queue-node"><strong>{{nodeDisplayName(q.node)}}</strong><small>{{cleanProfileName(q.node.profile_name)}} · {{queueProjectLabel(q)}}<template v-if="q.serviceId"> · {{w.catalog.value.find(s=>s.service_id===q.serviceId)?.name||q.serviceId}}</template></small><p v-if="errorText(q)" class="bad small" :title="errorText(q)">{{queueErrorLabel(q)}}</p><p v-if="antigravityEvidenceLine(q)" class="muted small">{{antigravityEvidenceLine(q)}}</p><p v-if="servicePhaseNote(q)" class="muted small">{{servicePhaseNote(q)}}</p><details v-if="antigravityDetails(q).length" class="small muted"><summary>执行详情</summary><p v-for="detail in antigravityDetails(q)" :key="detail">{{detail}}</p></details></div>
      <span class="cst-queue-exec" :class="{warn:unknownStart(q)}"><i class="cst-queue-dot" :class="unknownStart(q)?'warn':['queued'].includes(q.state)?'empty':['running','cancelling'].includes(q.state)?'running':'done'"/>{{unknownStart(q)?'启动未知':stateLabel(q.state)}}</span>
      <div class="queue-state"><span v-if="resultStatus(q)&&!unknownStart(q)" class="badge" :class="resultStatus(q)!.tone">{{resultStatus(q)!.label}}</span><span v-else-if="!unknownStart(q)&&!['queued','running','cancelling'].includes(q.state)" class="badge" :class="q.state==='completed'?'good':q.state==='failed'?'bad':''">{{stateLabel(q.state)}}</span><span v-else class="cst-queue-pending">{{unknownStart(q)?'待确认':'等待结果'}}</span></div>
      <div class="cst-queue-save" :class="saveState(q).tone"><span>{{saveState(q).label}}</span><small v-if="saveFailed(q)&&persistenceError(q)" class="queue-save-error">{{persistenceError(q)}}</small><button v-if="saveFailed(q)&&retryable(q)" class="text-button" :disabled="retrying===q.id" @click="retry(q)">重试保存</button><small v-else-if="saveFailed(q)&&!retryable(q)" class="muted">原结果无法恢复</small></div>
    </div>
    <p v-if="w.queue.value.length&&!visibleQueue.length" class="queue-empty">目前没有进行中或需留意的项目。排队项和已完成结果可在“全部项目”核对。</p>
    <div v-if="pageCount>1" class="queue-pagination"><span>{{filteredQueue.length}} 项 · 每页 {{pageSize}} 项</span><button class="button ghost" :disabled="queuePage<=1" @click="queuePage--">上一页</button><span>{{queuePage}} / {{pageCount}}</span><button class="button ghost" :disabled="queuePage>=pageCount" @click="queuePage++">下一页</button></div>
    <p class="queue-note">关闭此窗口后，可从顶部任务栏继续查看。</p>
  </template>
</template></Modal></template>

<style scoped>
.queue-title:focus{outline:none}
.cst-queue-head{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;flex-wrap:wrap;margin-bottom:14px}
.cst-queue-head>div:first-child{display:grid;gap:7px}
.cst-queue-head strong{font-size:20px;line-height:1.4;color:var(--cst-text)}
.cst-queue-head span{color:var(--cst-muted);font-size:13px}
.cst-queue-actions{display:flex;gap:6px;flex-wrap:wrap}
.queue-download-transfer{display:flex;align-items:baseline;flex-wrap:wrap;gap:10px 28px;margin:8px 0 13px;padding:12px 0;border-block:1px solid var(--cst-line)}
.queue-download-transfer>div{display:flex;align-items:baseline;flex-wrap:wrap;gap:4px 9px;min-width:0}
.queue-download-transfer dt,.queue-download-transfer small{font-size:12px;color:var(--cst-muted)}
.queue-download-transfer dd{margin:0;font-size:13px;font-weight:600;color:var(--cst-text);font-variant-numeric:tabular-nums;overflow-wrap:anywhere}
.queue-transfer-node small{margin-inline-start:8px;font-weight:400}
.progress-track{height:7px;border-radius:999px;margin:8px 0 14px;background:var(--cst-aurora-track);overflow:hidden}
.progress-track i{background:var(--cst-aurora-bar);box-shadow:var(--cst-aurora-glow);border-radius:999px;transition:width .35s cubic-bezier(.22,1,.36,1)}
.queue-counts{display:flex;flex-wrap:wrap;gap:7px;margin:8px 0}
.queue-counts>.queue-pill{display:inline-flex;align-items:center;padding:5px 12px;border:1px solid var(--cst-line-strong);border-radius:999px;background:var(--cst-panel-2);font-size:12px;font-weight:550;color:var(--cst-muted);font-variant-numeric:tabular-nums;transition:all .14s ease}
button.queue-pill{cursor:pointer}
button.queue-pill:hover{border-color:var(--cst-accent);color:var(--cst-text);background:var(--cst-panel)}
.queue-pill.active{border-color:var(--cst-accent)!important;box-shadow:0 0 0 2px color-mix(in srgb,var(--cst-accent) 22%,transparent);color:var(--cst-accent)!important;background:var(--cst-panel)!important;font-weight:650}
.queue-counts>.queue-pill.good{color:var(--cst-good);border-color:color-mix(in srgb,var(--cst-good) 32%,var(--cst-line));background:color-mix(in srgb,var(--cst-good) 8%,var(--cst-panel))}
.queue-counts>.queue-pill.running{color:var(--cst-accent);border-color:color-mix(in srgb,var(--cst-accent) 32%,var(--cst-line));background:color-mix(in srgb,var(--cst-accent) 8%,var(--cst-panel))}
.queue-counts>.queue-pill.warn{color:var(--cst-warn);border-color:var(--cst-warn-line);background:var(--cst-warn-bg)}
.queue-counts>.queue-pill.bad{color:var(--cst-bad);border-color:color-mix(in srgb,var(--cst-bad) 32%,var(--cst-line));background:color-mix(in srgb,var(--cst-bad) 8%,var(--cst-panel))}
.queue-counts.antigravity-summary{display:block;padding:8px 12px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-2);font-size:12px}
.queue-issue-report{margin:12px 0 4px;padding:12px 14px;border:1px solid color-mix(in srgb,var(--cst-warn) 30%,var(--cst-line));border-radius:12px;background:color-mix(in srgb,var(--cst-warn-bg) 42%,var(--cst-panel));display:grid;gap:10px}
.queue-issue-head{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}
.queue-issue-head>div{display:grid;gap:2px}
.queue-issue-head strong{font-size:13px;font-weight:700;color:var(--cst-text)}
.queue-issue-head small{font-size:11.5px;color:var(--cst-muted)}
.queue-issue-groups{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:8px}
.queue-issue-group{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:9px 11px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel);transition:border-color .14s,box-shadow .14s}
.queue-issue-group.active{border-color:var(--cst-accent);box-shadow:0 0 0 2px color-mix(in srgb,var(--cst-accent) 18%,transparent)}
.queue-issue-group.bad{border-left:3px solid var(--cst-bad)}
.queue-issue-group.warn{border-left:3px solid var(--cst-warn)}
.queue-issue-main{flex:1;min-width:0;border:0;padding:0;background:transparent;text-align:left;display:grid;gap:3px;cursor:pointer;color:inherit}
.queue-issue-title{display:flex;align-items:center;gap:6px;flex-wrap:wrap}
.queue-issue-title strong{font-size:12.5px;font-weight:650;color:var(--cst-text)}
.queue-issue-dot{width:6px;height:6px;border-radius:50%;flex:none}
.queue-issue-dot.warn{background:var(--cst-warn)}.queue-issue-dot.bad{background:var(--cst-bad)}
.queue-issue-meta{font-size:11px;color:var(--cst-muted);font-variant-numeric:tabular-nums}
.queue-issue-services{margin:0;font-size:11.5px;color:var(--cst-text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.queue-issue-sample{font-size:11px;color:var(--cst-muted);line-height:1.45;overflow-wrap:anywhere;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.queue-issue-retest{flex:none;white-space:nowrap}
.queue-clear-reason{margin-left:8px}
.compact-live-meta{display:flex;align-items:center;flex-wrap:wrap;gap:8px 12px;margin:2px 0 10px;font-size:12px;color:var(--cst-muted);font-variant-numeric:tabular-nums}
.compact-live-pct{font-size:15px;font-weight:750;color:var(--cst-accent)}
.compact-live-pill{display:inline-flex;align-items:center;gap:5px;padding:2px 9px;border-radius:999px;border:1px solid var(--cst-line);background:var(--cst-panel-2);font-size:11.5px;font-weight:600;color:var(--cst-text)}
.compact-live-pill.running{border-color:color-mix(in srgb,var(--cst-accent) 32%,var(--cst-line));background:var(--cst-accent-soft);color:var(--cst-accent)}
.compact-live-time{margin-left:auto;font-size:11.5px}
.compact-progress-counts{display:flex;flex-wrap:wrap;align-items:center;gap:10px;padding:4px 0 14px;border-bottom:1px solid var(--cst-line)}
.compact-count-btn{color:var(--cst-muted);font-size:12px;display:inline-flex;align-items:baseline;gap:8px;padding:8px 12px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-2);cursor:pointer;transition:border-color .14s,background .14s,transform .14s,box-shadow .14s}
.compact-count-btn:hover:not(:disabled){border-color:var(--cst-accent);background:var(--cst-panel);transform:translateY(-1px);box-shadow:var(--cst-shadow-xs)}
.compact-count-btn:disabled{opacity:.52;cursor:default}
.compact-count-btn.active{border-color:var(--cst-accent);background:var(--cst-panel);box-shadow:0 0 0 2px color-mix(in srgb,var(--cst-accent) 22%,transparent);color:var(--cst-text);font-weight:650}
.compact-count-btn strong{color:var(--cst-text);font-size:20px;font-variant-numeric:tabular-nums}
.compact-count-btn.good strong{color:var(--cst-good)}
.compact-count-btn.warn strong{color:var(--cst-warn)}
.compact-count-btn.bad strong{color:var(--cst-bad)}
.compact-count-reset{margin-left:auto;padding:5px 10px;border-radius:999px;border:1px solid var(--cst-line);background:var(--cst-panel);color:var(--cst-accent);font-size:11.5px;font-weight:600;cursor:pointer}
.compact-count-reset:hover{border-color:var(--cst-accent);background:var(--cst-accent-soft)}
.compact-live-section{margin-top:12px;display:grid;gap:8px}
.compact-live-head{display:flex;align-items:baseline;justify-content:space-between;gap:8px;flex-wrap:wrap}
.compact-live-head strong{font-size:12.5px;font-weight:700;color:var(--cst-text)}
.compact-live-head small{font-size:11px;color:var(--cst-muted)}
.compact-running-grid{display:flex;flex-wrap:wrap;gap:6px}
.compact-running-chip{display:inline-flex;align-items:center;gap:6px;max-width:100%;padding:4px 10px;border-radius:999px;border:1px solid color-mix(in srgb,var(--cst-accent) 28%,var(--cst-line));background:color-mix(in srgb,var(--cst-accent) 7%,var(--cst-panel));font-size:12px;font-weight:550;color:var(--cst-text)}
.compact-running-chip .spinner{width:10px;height:10px;flex:none}
.compact-running-chip>span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:220px}
.compact-running-chip small{font-size:10.5px;color:var(--cst-muted);flex:none}
.compact-recent-list{display:grid;gap:6px;max-height:270px;overflow:auto;padding-right:2px}
.compact-recent-card{border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-2);overflow:hidden;transition:border-color .14s,background .14s}
.compact-recent-card:hover,.compact-recent-card.expanded{border-color:color-mix(in srgb,var(--cst-accent) 36%,var(--cst-line));background:var(--cst-panel)}
.compact-recent-row{width:100%;border:0;background:transparent;display:flex;align-items:center;justify-content:space-between;gap:10px;padding:8px 11px;text-align:left;cursor:pointer;color:inherit}
.compact-recent-node{min-width:0;display:grid;gap:2px}
.compact-recent-node strong{font-size:12.5px;font-weight:600;color:var(--cst-text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.compact-recent-node small{font-size:11px;color:var(--cst-muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.compact-recent-err{color:var(--cst-warn)!important}
.compact-recent-tail{display:inline-flex;align-items:center;gap:7px;flex:none}
.compact-recent-row .badge{flex:none;white-space:nowrap;font-variant-numeric:tabular-nums}
.compact-recent-chev{color:var(--cst-subtle);font-size:13px;line-height:1}
.compact-recent-detail{padding:8px 11px 10px;border-top:1px solid color-mix(in srgb,var(--cst-line) 75%,transparent);background:color-mix(in srgb,var(--cst-panel-2) 60%,var(--cst-panel));display:grid;gap:6px}
.compact-site-grid{display:flex;flex-wrap:wrap;gap:6px}
.compact-site-chip{display:inline-flex;align-items:center;gap:5px;padding:3px 8px;border-radius:7px;border:1px solid var(--cst-line);background:var(--cst-panel);font-size:11px;font-variant-numeric:tabular-nums}
.compact-site-chip b{font-weight:650;color:var(--cst-muted)}
.compact-site-chip span{font-weight:700;color:var(--cst-text)}
.compact-site-chip small{color:var(--cst-muted);font-size:10px}
.compact-site-chip em{font-style:normal;padding:0 5px;border-radius:999px;background:var(--cst-warn-bg);color:var(--cst-warn);font-size:10px;font-weight:700}
.compact-site-chip.good span{color:var(--cst-good)}
.compact-site-chip.warn{border-color:color-mix(in srgb,var(--cst-warn) 34%,var(--cst-line));background:var(--cst-warn-bg)}.compact-site-chip.warn span{color:var(--cst-warn)}
.compact-site-chip.bad{border-color:color-mix(in srgb,var(--cst-bad) 30%,var(--cst-line));background:var(--cst-bad-bg)}.compact-site-chip.bad span{color:var(--cst-bad)}
.compact-detail-err{margin:0;font-size:11px;color:var(--cst-warn);line-height:1.45;overflow-wrap:anywhere}
.queue-list-tools{display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap;margin:16px 0 10px}
.queue-list-tools>small,.queue-note,.queue-empty{font-size:12px;color:var(--cst-muted);line-height:1.7}
.queue-note{margin:16px 0 0}.queue-empty{padding:22px 16px;border:1px dashed var(--cst-line);border-radius:10px;background:var(--cst-panel-2);text-align:center}
.queue-notice{font-size:13px;line-height:1.7;margin:14px 0}
.queue-views{display:flex;gap:18px;flex-wrap:wrap}
.queue-views button{border:0;background:none;border-bottom:2px solid transparent;padding:8px 0;font-size:12px;color:var(--cst-muted);cursor:pointer}
.queue-views button[aria-pressed=true]{border-color:var(--cst-accent);color:var(--cst-text);font-weight:600}
.queue-views span{margin-left:6px;font-variant-numeric:tabular-nums;color:var(--cst-muted)}
.cst-queue-cols,.cst-queue-row{display:grid;grid-template-columns:minmax(0,1.5fr) 84px minmax(110px,1fr) minmax(90px,.8fr);gap:14px;align-items:start}
.cst-queue-cols{padding:6px 14px 8px;font-size:11px;color:var(--cst-muted)}
.cst-queue-row{padding:12px 14px;border:1px solid var(--cst-line);margin:0 0 8px;background:var(--cst-panel);border-radius:10px}
.cst-queue-node{min-width:0}.cst-queue-node strong{display:block;font-size:13px;overflow-wrap:anywhere}
.cst-queue-node small{color:var(--cst-muted)}.cst-queue-node p{margin:4px 0 0}
.cst-queue-exec{display:flex;align-items:center;gap:6px;font-size:12px}
.cst-queue-exec.warn{color:var(--cst-warn)}
.cst-queue-dot{width:6px;height:6px;border-radius:50%;background:var(--cst-muted);flex:none;margin-top:1px}
.cst-queue-dot.empty{background:transparent;border:1px solid var(--cst-muted)}
.cst-queue-dot.running{background:var(--cst-accent)}.cst-queue-dot.warn{background:var(--cst-warn)}
.queue-state{display:block;min-width:0;text-align:left}.queue-state .badge{white-space:normal;line-height:1.6}
.cst-queue-pending{font-size:12px;color:var(--cst-muted)}
.cst-queue-save{display:flex;flex-direction:column;gap:4px;font-size:12px;min-width:0}
.cst-queue-save.bad>span{color:var(--cst-bad);font-weight:600}
.cst-queue-save.warn>span{color:var(--cst-warn)}.cst-queue-save.empty>span{color:var(--cst-muted)}
.cst-queue-save .text-button{justify-content:flex-start;text-align:left}
.queue-save-error{overflow-wrap:anywhere;line-height:1.5}
.queue-pagination{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-top:18px;font-size:12px;color:var(--cst-muted)}
.queue-pagination>span:first-child{margin-right:auto}
@media(max-width:640px){
 .cst-queue-cols{display:none}.cst-queue-row{grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:10px 16px}
 .cst-queue-node{grid-column:1/-1}.cst-queue-exec{grid-column:1/-1}.queue-state::before,.cst-queue-save::before{display:block;font-size:10px;color:var(--cst-muted);margin-bottom:4px}
 .queue-state::before{content:'检测结果'}.cst-queue-save::before{content:'保存'}
 .cst-queue-head strong{font-size:18px}.cst-queue-head{gap:12px}
}
</style>
