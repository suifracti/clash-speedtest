<script setup lang="ts">
import {nodeDisplayName} from '../nodePresentation'
import {computed,ref} from 'vue'
import {api} from '../api'
import {latencySampleState} from '../presentation'
import {downloadNoDataReason} from '../measurementRounds'
import {useWorkspace} from '../workspace'
import {projectNames,stateLabel,outcomeLabel,outcomeTone,downloadSpeed,downloadTone,type QueueItem} from '../domain'
import Modal from './Modal.vue'
const w=useWorkspace(),retrying=ref('')
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
const retryable=(q:QueueItem)=>!!(q.batchId&&q.itemId||q.project!=='latency'&&q.attempt?.result)
async function retry(q:QueueItem){retrying.value=q.id;try{if(q.batchId&&q.itemId)w.absorbBatch(await api.retryBatch(q.batchId,q.itemId));else if(q.attempt&&q.project!=='latency'){const a=await api.attemptAction(q.project,q.attempt.attempt_id,q.node,'retry-save',q.attempt.service_id||q.serviceId);q.attempt=a;q.error=a.persistence_error;w.mergeAttempt(q.project,a)}}catch(e){w.fail(e)}finally{retrying.value=''}}
</script>
<template><Modal title="检测进度" wide @close="w.queueOpen.value=false"><div class="queue-summary"><strong>{{w.queue.value.length?w.done.value+'/'+w.queue.value.length:'后台任务'}}</strong><span>{{w.pendingStart.value?'启动尚待确认':w.busy.value?'检测进行中':'检测已结束'}}</span><button class="button ghost" @click="w.syncActive()">刷新进度</button><button v-if="w.pendingStart.value?.body" class="button" :disabled="w.running.value||w.retryingStart.value" @click="w.retryPending()">{{w.retryingStart.value?'正在确认…':'确认启动状态'}}</button><button v-if="w.busy.value" class="button danger-outline" :disabled="w.cancelRequested.value" @click="w.cancelTests()">{{w.cancelRequested.value?'正在取消…':'取消检测'}}</button></div><p class="queue-counts" role="status">终态 {{w.done.value}} / {{w.queue.value.length}} · 排队 {{counts.queued}} · 执行 {{counts.running}} · 未运行/取消 {{counts.notRun}} · 启动未知 {{counts.unknown}} · 保存失败 {{counts.saveFailed}}</p><div class="progress-track"><i :style="{width:w.queue.value.length?`${w.done.value/w.queue.value.length*100}%`:'0%'}"/></div><div v-if="!w.queue.value.length&&w.activeBatch.value" class="muted">恢复的后台批次包含 {{w.activeBatch.value.item_count}} 个节点；结果可在检测历史查看。</div><div v-for="q in w.queue.value" :key="q.id" class="queue-row"><div><strong>{{nodeDisplayName(q.node)}}</strong><small>{{q.node.profile_name}} · {{projectNames[q.project]}}<template v-if="q.serviceId"> · {{w.catalog.value.find(s=>s.service_id===q.serviceId)?.name||q.serviceId}}</template></small><p v-if="q.error" class="bad small">{{q.project==='download'&&!q.error.startsWith('保存失败')?downloadNoDataReason(q.attempt)||q.error:q.error}}</p><p v-if="servicePhaseNote(q)" class="muted small">{{servicePhaseNote(q)}}</p></div><div class="queue-state"><span class="badge" :class="resultStatus(q)?.tone??(q.state==='completed'?'good':q.state==='failed'?'bad':'')">{{w.pendingStart.value&&(q.id===w.pendingStart.value.requestId||w.pendingStart.value.kind==='latency'&&q.project==='latency'&&q.state==='running'&&!q.batchId)?'启动未知':resultStatus(q)?.label||stateLabel(q.state)}}</span><button v-if="(q.attempt?.persistence_state==='failed'||q.error?.startsWith('保存失败'))&&retryable(q)" class="text-button" :disabled="retrying===q.id" @click="retry(q)">重试保存</button><small v-else-if="(q.attempt?.persistence_state==='failed'||q.error?.startsWith('保存失败'))&&!retryable(q)" class="muted">原结果无法恢复</small></div></div></Modal></template>
