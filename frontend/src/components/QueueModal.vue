<script setup lang="ts">
import {nodeDisplayName} from '../nodePresentation'
import {computed,ref,watch,nextTick,onMounted} from 'vue'
import {api,saveFile} from '../api'
import {readLatencyBatches} from '../latencyReportData'
import {key,targets,type BatchItem,type LatencyBatch} from '../domain'
import LatencyReport from './LatencyReport.vue'
import {latencySampleState} from '../presentation'
import {downloadNoDataReason} from '../measurementRounds'
import {entryCountry,mainlandNotice} from '../entryPath'
import {useWorkspace} from '../workspace'
import {projectNames,stateLabel,outcomeLabel,outcomeTone,downloadReading,downloadTone,downloadOutcome,downloadSourceName,type QueueItem} from '../domain'
import Modal from './Modal.vue'
const w=useWorkspace(),retrying=ref(''),progressTitle=ref<HTMLElement>()
onMounted(()=>nextTick(()=>progressTitle.value?.focus({preventScroll:true})))
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
const isAntigravity=(q:QueueItem)=>q.project==='service'&&(q.serviceId||q.attempt?.service_id)==='antigravity'
const modelConfirmed=(q:QueueItem)=>q.attempt?.result?.outcome==='matched'&&q.attempt.result.details?.antigravity_progress==='真实模型已回答'
const antigravitySummary=computed(()=>{
 const rows=w.queue.value.filter(isAntigravity);if(!rows.length)return ''
 const processed=rows.filter(q=>['completed','failed','not_executed','cancelled','interrupted'].includes(q.state))
 const confirmed=processed.filter(modelConfirmed).length,refused=processed.filter(q=>q.attempt?.result?.outcome==='region_blocked').length
 return `Antigravity：已处理 ${processed.length}/${rows.length} · 确认模型可用 ${confirmed} · 地区拒绝 ${refused} · 未确认 ${processed.length-confirmed-refused}`
})
function resultStatus(q:QueueItem):{label:string;tone:string;notExecuted?:boolean}|null{
 if(q.project==='service'&&['completed','failed'].includes(q.state)){
  const result=q.attempt?.result
  if(!result)return {label:'已结束 · 无结果',tone:''}
  const outcome=result.outcome
  if(isAntigravity(q)&&outcome==='matched')return {label:modelConfirmed(q)?'确认模型可用':'未确认模型可用',tone:modelConfirmed(q)?'good':'warn'}
  if(isAntigravity(q)&&outcome==='region_blocked')return {label:'地区拒绝',tone:'warn'}
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
  if(['not_executed','cancelled','user_cancelled','interrupted'].includes(outcome))return {label:stateLabel(outcome==='user_cancelled'?'cancelled':outcome),tone:'',notExecuted:true}
  if(downloadNoDataReason(a))return {label:downloadNoDataReason(a),tone:'warn'}
  if(downloadOutcome(a)==='response_too_short')return {label:outcomeLabel('response_too_short'),tone:'bad'}
  const reading=downloadReading(a)
  if(reading)return {label:`${downloadSourceName(a)} · ${reading.label} ${reading.value.toFixed(2)} MiB/s · ${(result.bytes_read/1048576).toFixed(1)} MiB`,tone:downloadTone(a,false)}
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
function antigravityDetails(q:QueueItem){
 const r=q.attempt?.result;if(!isAntigravity(q)||!r)return []
 const d=r.details||{},steps:Record<string,string>={loadCodeAssist:'账号项目查询',fetchAvailableModels:'模型目录查询','streamGenerateContent?alt=sse':'模型生成',model_response:'模型回答确认'}
 const phases:Record<string,string>={proxy_connect:'节点代理建连',tls_handshake:'目标 TLS 握手',request_write:'请求写出',response_headers:'等待响应头',response_read:'读取响应体'}
 let path:{address_family?:string;interface?:string}={};try{path=JSON.parse(String(d.network_path||'{}'))}catch{}
 return [`${d.antigravity_progress||'模型进度未记录'}`,`账号／模型阶段：${steps[r.failure_phase||'']||r.failure_phase||'已结束'}`,`HTTP 状态：${r.http_status??'未收到'}`,`请求失败阶段：${phases[String(d.request_failure_phase)]||d.request_failure_phase||'未记录'}`,`超时来源：${d.timeout_origin==='checker_deadline'?'检查总时限':d.timeout_origin==='transport'?'传输超时':'未记录'}`,`HTTP 请求写出累计：${d.http_requests_written??'未记录'}（含出口观察）`,`地址族：${path.address_family||'未记录'} · 网卡：${path.interface||'未记录'}`,`Cloudflare 独立出口观察：${d.country_code?`${d.country_code}（不代表 Google 出口）`:d.exit_observation_state||'未记录'}`]
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
 if(raw.includes('备用源'))return raw
 if(/timeout|deadline|timed_out/i.test(raw))return '超时'
 if(/refused/i.test(raw))return '连接被拒绝'
 if(/reset/i.test(raw))return '连接被重置'
 if(/closed|EOF/i.test(raw))return '连接被关闭'
 if(/NXDOMAIN|no such host|DNS/i.test(raw))return '域名解析失败'
 if(/cancel/i.test(raw))return '已取消'
 return raw&&/[a-z]{3}/i.test(raw)?'连接失败':raw
}
function queueErrorLabel(q:QueueItem){
 const raw=errorText(q)
 if(q.project==='service'&&!['timed_out','transport_error','connection_failed'].includes(q.attempt?.result?.outcome||''))return raw
 return q.project==='download'?downloadNoDataReason(q.attempt)||shortError(raw):shortError(raw)
}
const queueView=ref<'focus'|'all'>('focus'),queuePage=ref(1),pageSize=50
const attentionQueue=computed(()=>w.queue.value.filter(q=>!['queued','completed'].includes(q.state)||unknownStart(q)||saveFailed(q)||persistenceState(q)==='saving'||!!q.error||q.state==='completed'&&resultStatus(q)?.tone!=='good'))
const filteredQueue=computed(()=>queueView.value==='all'?w.queue.value:attentionQueue.value)
const pageCount=computed(()=>Math.max(1,Math.ceil(filteredQueue.value.length/pageSize)))
const visibleQueue=computed(()=>filteredQueue.value.slice((queuePage.value-1)*pageSize,queuePage.value*pageSize))
watch(queueView,()=>queuePage.value=1)
watch(pageCount,count=>queuePage.value=Math.min(queuePage.value,count))
const succeeded=computed(()=>w.queue.value.filter(q=>q.state==='completed'&&resultStatus(q)?.tone==='good').length)
const compactLatency=computed(()=>!!w.activeBatch.value&&['queued','running','cancelling','saving'].includes(w.activeBatch.value.state)||w.running.value&&w.queue.value.length>0&&w.queue.value.every(q=>q.project==='latency'))
const progress=computed(()=>w.activeBatchProgress.value?.batch_id===w.activeBatch.value?.batch_id&&w.activeBatchProgress.value||{total:w.activeBatch.value?.item_count||w.queue.value.length,processed:0,all_success:0,partial_response:0,no_response:0,save_failed:0,not_executed:0})
const savingOnly=computed(()=>compactLatency.value?w.activeBatch.value?.state==='saving':counts.value.saving>0&&!counts.value.running&&!counts.value.queued)
const canCancel=computed(()=>w.busy.value&&(!!w.pendingStart.value||!savingOnly.value))
const headline=computed(()=>w.pendingStart.value?'正在确认启动状态':w.busy.value?(w.cancelRequested.value?'正在取消，已完成结果保留':savingOnly.value?'正在保存结果':'检测进行中'):w.done.value<w.queue.value.length?'检测已停止，部分项目没有运行完':counts.value.saveFailed?'检测已结束，部分结果未保存':'检测已结束')
const retryable=(q:QueueItem)=>!!(q.batchId&&q.itemId||q.project!=='latency'&&q.attempt?.result)
async function retry(q:QueueItem){retrying.value=q.id;try{if(q.batchId&&q.itemId)w.absorbBatch(await api.retryBatch(q.batchId,q.itemId));else if(q.attempt&&q.project!=='latency'){const a=await api.attemptAction(q.project,q.attempt.attempt_id,q.node,'retry-save',q.attempt.service_id||q.serviceId);q.attempt=a;q.error=a.persistence_error;w.mergeAttempt(q.project,a)}}catch(e){w.fail(e)}finally{retrying.value=''}}
</script>
<template><Modal :title="report?'延迟检测报告':'检测进度'" wide variant="cst-queue" @close="w.queueOpen.value=false;w.reportSelection.value=null">
<LatencyReport v-if="report" :batch="report.batch" :previous="previous" :airport-names="airportNames" :monitor="report.monitor" :entry-notice="entryNotice" @retry="retryReport" @export="exportReport" @inspect="inspectReport"/>
<template v-else>
  <div class="queue-summary cst-queue-head">
    <div><strong ref="progressTitle" class="queue-title" tabindex="-1" autofocus>{{headline}}</strong><span v-if="!w.pendingStart.value">{{compactLatency?`已处理 ${progress.processed} / ${progress.total} 个节点`:w.queue.value.length?`已处理 ${w.done.value} / ${w.queue.value.length} 项`:'后台任务'}}</span></div>
    <div class="cst-queue-actions"><button class="button ghost" @click="w.syncActive()">刷新进度</button><button v-if="w.pendingStart.value?.body" class="button" :disabled="w.running.value||w.retryingStart.value" @click="w.retryPending()">{{w.retryingStart.value?'正在确认…':'确认启动状态'}}</button><button v-if="canCancel" class="button danger-outline" :disabled="w.cancelRequested.value" @click="w.cancelTests()">{{w.cancelRequested.value?'正在取消…':'取消检测'}}</button></div>
  </div>
  <p v-if="w.pendingStart.value" class="queue-notice muted">后台还没确认这次启动；确认前不显示进度，也不会重复启动。</p>
  <div v-else class="progress-track" role="progressbar" aria-label="已处理检测项目" :aria-valuenow="compactLatency?progress.processed:w.done.value" :aria-valuemax="compactLatency?progress.total:w.queue.value.length" aria-valuemin="0"><i :style="{width:compactLatency?(progress.total?`${progress.processed/progress.total*100}%`:'0%'):(w.queue.value.length?`${w.done.value/w.queue.value.length*100}%`:'0%')}"/></div>
  <p v-if="w.error.value" class="inline-error" role="alert">{{w.error.value}}</p>
  <template v-if="compactLatency">
    <div v-if="!w.pendingStart.value" class="compact-progress-counts"><span>全部响应 <strong>{{progress.all_success}}</strong></span><span>部分响应 <strong>{{progress.partial_response}}</strong></span><span>无响应 <strong>{{progress.no_response}}</strong></span><span v-if="progress.not_executed">未执行/取消 <strong>{{progress.not_executed}}</strong></span></div>
    <p v-if="progress.save_failed" class="inline-warning" role="alert">保存失败 {{progress.save_failed}} 项；结束后可在报告重试保存。</p>
    <p class="queue-note">检测中显示整批进度，结束后提供逐节点报告。关闭此窗口不影响检测。</p>
  </template>
  <template v-else>
    <p class="queue-counts" role="status">已处理 {{w.done.value}} / {{w.queue.value.length}} · 已成功 {{succeeded}} · 排队 {{counts.queued}} · 执行 {{counts.running}} · 未运行/取消 {{counts.notRun}} · 启动未知 {{counts.unknown}} · 保存中 {{counts.saving}} · 保存失败 {{counts.saveFailed}}</p>
    <p v-if="antigravitySummary" class="queue-counts antigravity-summary" role="status">{{antigravitySummary}}</p>
    <div v-if="!w.queue.value.length&&w.activeBatch.value" class="muted">恢复的后台批次包含 {{w.activeBatch.value.item_count}} 个节点；结果可在检测历史查看。</div>
    <div v-if="w.queue.value.length" class="queue-list-tools"><div class="queue-views" role="group" aria-label="检测项目范围"><button :aria-pressed="queueView==='focus'" @click="queueView='focus'">进行中与需留意 <span>{{attentionQueue.length}}</span></button><button :aria-pressed="queueView==='all'" @click="queueView='all'">全部项目 <span>{{w.queue.value.length}}</span></button></div><small>检测结果与保存状态分别列出</small></div>
    <div v-if="visibleQueue.length" class="cst-queue-cols" aria-hidden="true"><span>节点与项目</span><span>执行</span><span>检测结果</span><span>保存</span></div>
    <div v-for="q in visibleQueue" :key="q.id" class="queue-row cst-queue-row">
      <div class="cst-queue-node"><strong>{{nodeDisplayName(q.node)}}</strong><small>{{q.node.profile_name}} · {{projectNames[q.project]}}<template v-if="q.serviceId"> · {{w.catalog.value.find(s=>s.service_id===q.serviceId)?.name||q.serviceId}}</template></small><p v-if="errorText(q)" class="bad small" :title="errorText(q)">{{queueErrorLabel(q)}}</p><p v-if="servicePhaseNote(q)" class="muted small">{{servicePhaseNote(q)}}</p><details v-if="antigravityDetails(q).length" class="small muted"><summary>执行详情</summary><p v-for="detail in antigravityDetails(q)" :key="detail">{{detail}}</p></details></div>
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
.progress-track{height:4px;border-radius:0;margin:8px 0 16px;background:var(--cst-line)}
.progress-track i{background:var(--cst-accent);border-radius:0}
.compact-progress-counts{display:flex;flex-wrap:wrap;gap:22px 36px;padding:16px 0 24px;border-bottom:1px solid var(--cst-line)}
.compact-progress-counts span{color:var(--cst-muted);font-size:12px;display:flex;align-items:baseline;gap:10px}
.compact-progress-counts strong{color:var(--cst-text);font-size:24px;font-variant-numeric:tabular-nums}
.queue-list-tools{display:flex;justify-content:space-between;align-items:center;gap:10px;flex-wrap:wrap;margin:20px 0 8px}
.queue-list-tools>small,.queue-note,.queue-empty{font-size:12px;color:var(--cst-muted);line-height:1.7}
.queue-note{margin:20px 0 0}.queue-empty{padding:26px 0;border-top:1px solid var(--cst-line)}
.queue-notice{font-size:13px;line-height:1.7;margin:14px 0}
.queue-views{display:flex;gap:18px;flex-wrap:wrap}
.queue-views button{border:0;background:none;border-bottom:2px solid transparent;padding:8px 0;font-size:12px;color:var(--cst-muted);cursor:pointer}
.queue-views button[aria-pressed=true]{border-color:var(--cst-accent);color:var(--cst-text);font-weight:600}
.queue-views span{margin-left:6px;font-variant-numeric:tabular-nums;color:var(--cst-muted)}
.cst-queue-cols,.cst-queue-row{display:grid;grid-template-columns:minmax(0,1.5fr) 84px minmax(110px,1fr) minmax(90px,.8fr);gap:16px;align-items:start}
.cst-queue-cols{padding:10px 0;border-bottom:1px solid var(--cst-line);font-size:11px;color:var(--cst-muted)}
.cst-queue-row{padding:15px 0;border-bottom:1px solid var(--cst-line);margin:0;background:none;border-radius:0}
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
