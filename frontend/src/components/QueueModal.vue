<script setup lang="ts">
import {nodeDisplayName} from '../nodePresentation'
import {computed,ref,watch,nextTick} from 'vue'
import {api,saveFile} from '../api'
import {readLatencyBatches} from '../latencyReportData'
import {key,type BatchItem,type LatencyBatch} from '../domain'
import LatencyReport from './LatencyReport.vue'
import {latencySampleState} from '../presentation'
import {downloadNoDataReason} from '../measurementRounds'
import {entryCountry,mainlandNotice} from '../entryPath'
import {useWorkspace} from '../workspace'
import {projectNames,stateLabel,outcomeLabel,outcomeTone,downloadSpeed,downloadTone,type QueueItem} from '../domain'
import Modal from './Modal.vue'
const w=useWorkspace(),retrying=ref('')
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
function currentNodes(items:BatchItem[]){return items.flatMap(i=>{const n=w.nodes.value.find(n=>key(n)===key(i));return n?[n]:[]})}
function retryReport(items:BatchItem[]){const nodes=currentNodes(items);if(!nodes.length){w.notify('这些历史节点的配置已变化，请重新选择');return}w.openPlan(nodes);if(w.plan.value){w.plan.value.projects=['latency'];w.plan.value.includeReputation=false;w.plan.value.target=report.value?.batch.target_id||'all';w.plan.value.samples=report.value?.batch.sample_count||6;w.plan.value.timeout=report.value?.batch.timeout_seconds||5}w.queueOpen.value=false}
async function exportReport(items:BatchItem[]){try{const nodes=currentNodes(items);if(!nodes.length){w.notify('这些历史节点已不在当前订阅中');return}const result=await api.exportClash(nodes);await saveFile(result.yaml_content,'cst-usable.yaml','application/x-yaml')}catch(e){w.fail(e)}}
async function inspectReport(item:BatchItem){
 const n=currentNodes([item])[0];if(!n){w.notify('这个历史节点已不在当前订阅中');return}if(!item.result?.samples?.length){w.notify('这项未执行，没有采样轮次');return}
 w.latencyReportFocus.value={nodeKey:key(n),id:report.value?.monitor?report.value.batch.batch_id:item.result.attempt_id,test:item.result}
 w.queueOpen.value=false;w.project.value='latency';const airport=w.nodeAirportId(n);if(w.airportFilter.value!==airport)w.viewAirport(airport);w.search.value='';w.region.value='all';w.excludeRegion.value=false;w.subscription.value='all';w.downloadFullOnly.value=false;w.page.value='home';await nextTick();w.viewRequest.value=n
}
const counts=computed(()=>{
 const rows=w.queue.value,unknown=w.pendingStart.value?rows.filter(q=>q.id===w.pendingStart.value?.requestId||w.pendingStart.value?.kind==='latency'&&q.project==='latency'&&q.state==='running'&&!q.batchId).length:0
 return {queued:rows.filter(q=>q.state==='queued').length,running:Math.max(0,rows.filter(q=>['running','cancelling'].includes(q.state)).length-unknown),unknown,notRun:rows.filter(q=>['not_executed','cancelled'].includes(q.state)||resultStatus(q)?.notExecuted).length,saveFailed:rows.filter(q=>q.attempt?.persistence_state==='failed'||q.error?.startsWith('保存失败')).length}
})
function resultStatus(q:QueueItem):{label:string;tone:string;notExecuted?:boolean}|null{
 if(q.project==='service'&&['completed','failed'].includes(q.state)){
  if(q.attempt?.persistence_state==='failed')return {label:'保存失败',tone:'bad'}
  const result=q.attempt?.result
  if(!result)return {label:'已结束 · 无结果',tone:''}
  const outcome=result.outcome
  if(['not_executed','cancelled','user_cancelled','interrupted'].includes(outcome))return {label:stateLabel(outcome==='user_cancelled'?'cancelled':outcome),tone:'',notExecuted:true}
  if(['timed_out','transport_error','rate_limited'].includes(outcome))return {label:outcomeLabel(outcome)+' · 未确认',tone:'warn'}
  return {label:outcomeLabel(outcome),tone:outcomeTone(outcome)}
 }
 if(q.project==='download'&&['completed','failed'].includes(q.state)){
  const a=q.attempt
  if(a?.persistence_state==='failed')return {label:'保存失败',tone:'bad'}
  const result=a?.result
  if(!a||!result)return {label:'已结束 · 无结果',tone:''}
  const outcome=result.outcome
  if(['not_executed','cancelled','user_cancelled','interrupted'].includes(outcome))return {label:stateLabel(outcome==='user_cancelled'?'cancelled':outcome),tone:'',notExecuted:true}
  if(downloadNoDataReason(a))return {label:downloadNoDataReason(a),tone:'warn'}
  const speed=downloadSpeed(a)
  if(speed!==null)return {label:`${speed.toFixed(2)} MB/s · ${(result.bytes_read/1048576).toFixed(1)}MB`,tone:downloadTone(a)}
  if(outcome==='source_rate_limited'||result.http_status===429)return {label:'下载源限流',tone:'warn'}
  if(result.bytes_read>0)return {label:`${(result.bytes_read/1048576).toFixed(2)} MB · 中断`,tone:'warn'}
  if(['timed_out','transport_error','connection_failed'].includes(outcome))return {label:outcomeLabel(outcome)+' · 未确认',tone:'warn'}
  return {label:outcomeLabel(outcome),tone:outcomeTone(outcome)}
 }
 if(q.project!=='latency'||!['completed','failed'].includes(q.state))return null
 const item=w.batches.value.find(b=>b.batch_id===q.batchId)?.items?.find(i=>i.item_id===q.itemId)
 if(item?.persistence_state==='failed')return {label:'保存失败',tone:'bad'}
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
 if(!['timed_out','transport_error','unknown'].includes(r.outcome)||!d)return ''
 const ms=d.proxy_connect_ms,written=d.http_requests_written
 if((typeof ms!=='string'&&typeof ms!=='number')||String(ms).trim()===''||(typeof written!=='string'&&typeof written!=='number')||String(written).trim()==='')return ''
 const duration=Number(ms)
 if(!Number.isFinite(duration)||duration<0||Number(written)!==0)return ''
 return `代理连接耗时约 ${(duration/1000).toFixed(2)} 秒 · 未记录完整 HTTP 请求写出，服务结论未确认`
}
// 执行与保存是两个独立属性：先看有没有跑完，再看结果有没有落库。
function latencyItem(q:QueueItem){return q.project==='latency'?w.batches.value.find(b=>b.batch_id===q.batchId)?.items?.find(i=>i.item_id===q.itemId):undefined}
function unknownStart(q:QueueItem){const p=w.pendingStart.value;return !!p&&(q.id===p.requestId||p.kind==='latency'&&q.project==='latency'&&q.state==='running'&&!q.batchId)}
function saveFailed(q:QueueItem){return q.attempt?.persistence_state==='failed'||!!q.error?.startsWith('保存失败')||latencyItem(q)?.persistence_state==='failed'}
function saveState(q:QueueItem):{label:string;tone:string}{
 if(saveFailed(q))return {label:'保存失败',tone:'bad'}
 const state=q.project==='latency'?latencyItem(q)?.persistence_state:q.attempt?.persistence_state
 if(state==='saving')return {label:'保存中',tone:'warn'}
 if(state==='saved')return {label:'已保存',tone:''}
 if(['queued','running','cancelling'].includes(q.state))return {label:'尚未结束',tone:'empty'}
 return {label:'—',tone:'empty'}
}
const finished=computed(()=>!w.busy.value&&!w.pendingStart.value&&w.done.value===w.queue.value.length)
const headline=computed(()=>w.pendingStart.value?'正在确认启动状态':w.busy.value?'检测进行中':w.done.value<w.queue.value.length?'检测已停止，部分项目没有运行完':counts.value.saveFailed?'检测已结束，部分结果未保存':'检测已结束')
const retryable=(q:QueueItem)=>!!(q.batchId&&q.itemId||q.project!=='latency'&&q.attempt?.result)
async function retry(q:QueueItem){retrying.value=q.id;try{if(q.batchId&&q.itemId)w.absorbBatch(await api.retryBatch(q.batchId,q.itemId));else if(q.attempt&&q.project!=='latency'){const a=await api.attemptAction(q.project,q.attempt.attempt_id,q.node,'retry-save',q.attempt.service_id||q.serviceId);q.attempt=a;q.error=a.persistence_error;w.mergeAttempt(q.project,a)}}catch(e){w.fail(e)}finally{retrying.value=''}}
</script>
<template><Modal :title="report?'延迟检测报告':'检测进度'" wide variant="cst-queue" @close="w.queueOpen.value=false;w.reportSelection.value=null">
<LatencyReport v-if="report" :batch="report.batch" :previous="previous" :airport-names="airportNames" :monitor="report.monitor" :entry-notice="entryNotice" @retry="retryReport" @export="exportReport" @inspect="inspectReport"/>
<template v-else>
  <div class="queue-summary cst-queue-head">
    <div><strong>{{w.queue.value.length?`已处理 ${w.done.value}/${w.queue.value.length}`:'后台任务'}}</strong><span :class="{warn:!!w.pendingStart.value||finished&&counts.saveFailed>0}">{{headline}}</span></div>
    <div class="cst-queue-actions"><button class="button ghost" @click="w.syncActive()">刷新进度</button><button v-if="w.pendingStart.value?.body" class="button" :disabled="w.running.value||w.retryingStart.value" @click="w.retryPending()">{{w.retryingStart.value?'正在确认…':'确认启动状态'}}</button><button v-if="w.busy.value" class="button danger-outline" :disabled="w.cancelRequested.value" @click="w.cancelTests()">{{w.cancelRequested.value?'正在取消…':'取消检测'}}</button></div>
  </div>
  <p class="queue-counts" role="status">已处理 {{w.done.value}} / {{w.queue.value.length}} · 排队 {{counts.queued}} · 执行 {{counts.running}} · 未运行/取消 {{counts.notRun}} · 启动未知 {{counts.unknown}} · 保存失败 {{counts.saveFailed}}</p>
  <div v-if="!w.pendingStart.value" class="progress-track"><i :style="{width:w.queue.value.length?`${w.done.value/w.queue.value.length*100}%`:'0%'}"/></div>
  <p v-else class="muted small">后台还没确认这次启动；确认前不显示进度，也不会重复启动。</p>
  <div v-if="!w.queue.value.length&&w.activeBatch.value" class="muted">恢复的后台批次包含 {{w.activeBatch.value.item_count}} 个节点；结果可在检测历史查看。</div>
  <div v-if="w.queue.value.length" class="cst-queue-cols" aria-hidden="true"><span>节点与项目</span><span>执行</span><span>结果</span><span>保存</span></div>
  <div v-for="q in w.queue.value" :key="q.id" class="queue-row cst-queue-row">
    <div class="cst-queue-node"><strong>{{nodeDisplayName(q.node)}}</strong><small>{{q.node.profile_name}} · {{projectNames[q.project]}}<template v-if="q.serviceId"> · {{w.catalog.value.find(s=>s.service_id===q.serviceId)?.name||q.serviceId}}</template></small><p v-if="q.error" class="bad small">{{q.project==='download'&&!q.error.startsWith('保存失败')?downloadNoDataReason(q.attempt)||q.error:q.error}}</p><p v-if="servicePhaseNote(q)" class="muted small">{{servicePhaseNote(q)}}</p></div>
    <span class="cst-queue-exec" :class="{warn:unknownStart(q)}"><i class="cst-queue-dot" :class="unknownStart(q)?'warn':['queued'].includes(q.state)?'empty':['running','cancelling'].includes(q.state)?'running':'done'"/>{{unknownStart(q)?'启动未知':stateLabel(q.state)}}</span>
    <div class="queue-state"><span v-if="resultStatus(q)&&!unknownStart(q)" class="badge" :class="resultStatus(q)!.tone">{{resultStatus(q)!.label}}</span><span v-else-if="!unknownStart(q)&&!['queued','running','cancelling'].includes(q.state)" class="badge" :class="q.state==='completed'?'good':q.state==='failed'?'bad':''">{{stateLabel(q.state)}}</span><span v-else class="cst-queue-pending">{{unknownStart(q)?'待确认':'等待结果'}}</span></div>
    <div class="cst-queue-save" :class="saveState(q).tone"><span>{{saveState(q).label}}</span><button v-if="saveFailed(q)&&retryable(q)" class="text-button" :disabled="retrying===q.id" @click="retry(q)">重试保存</button><small v-else-if="saveFailed(q)&&!retryable(q)" class="muted">原结果无法恢复</small></div>
  </div>
</template></Modal></template>
