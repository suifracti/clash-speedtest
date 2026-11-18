<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref} from 'vue'
import {api} from '../api'
import {bytes,date,stateLabel,type ManualRoundDetail,type ManualRoundSummary,type MeasurementRoundItem} from '../domain'
import {attemptStatus,manualRoundCount,manualRoundLabel,manualRoundState,serviceConclusion} from '../measurementRounds'
import {nodeDisplayName,nodeSourceIndex} from '../nodePresentation'
import {useWorkspace} from '../workspace'
import Modal from './Modal.vue'

const props=defineProps<{summary:ManualRoundSummary}>()
const emit=defineEmits<{close:[]}>()
const w=useWorkspace(),detail=ref<ManualRoundDetail>(),loading=ref(false),error=ref('')
const offset=ref(0),requestedOffset=ref(0),visited=ref<number[]>([])
const sources=computed(()=>nodeSourceIndex(w.airports.value))
let mounted=true
async function read(next:number){
 if(loading.value)return
 loading.value=true;error.value='';requestedOffset.value=next
 try{
  const page=await api.manualRound(props.summary.round_id,next)
  if(mounted){
   detail.value=page;offset.value=next
   if(!visited.value.includes(next))visited.value.push(next)
  }
 }catch(e){if(mounted)error.value=e instanceof Error?e.message:String(e)}
 finally{if(mounted)loading.value=false}
}
onMounted(()=>void read(0))
onBeforeUnmount(()=>{mounted=false})

function nodeName(item:MeasurementRoundItem){
 const name=item.display_name||item.service?.display_name||item.download?.display_name||item.latency?.display_name
 return name?nodeDisplayName({display_name:name}):'历史节点'
}
function accountName(item:MeasurementRoundItem){
 const source=sources.value.get(item.profile_id)
 return source?`${source.airportName} · ${source.subscriptionName}`:`历史账户 ${item.profile_id}`
}
function projectName(item:MeasurementRoundItem){
 if(item.project==='latency')return '延迟检测'
 if(item.project==='download')return '下载测速'
 return item.service?.rule.name||w.catalog.value.find(r=>r.service_id===item.service_id)?.name||item.service_id||'服务检测'
}
function resultText(item:MeasurementRoundItem){
 if(item.service)return serviceConclusion(item.service)
 if(item.download)return attemptStatus(item.download,true)
 if(item.latency?.total_samples)return `${item.latency.success_samples}/${item.latency.total_samples} 次响应`
 return item.execution_state==='not_executed'?'未执行':'—'
}
function evidence(item:MeasurementRoundItem){
 const result=item.service?.result||item.download?.result
 return [result?.http_status?`HTTP ${result.http_status}`:'',result?.error_message||result?.summary||item.execution_error||item.latency?.error_message||item.not_executed_reason].filter(Boolean).join(' · ')
}
</script>

<template>
 <Modal :title="manualRoundLabel(summary)" wide @close="emit('close')">
  <p class="muted small">{{date(summary.started_at)}} · {{summary.node_count}} 个节点 · {{manualRoundCount(summary)}} 项 · {{manualRoundState(detail?.state||summary.state)}}</p>
  <p v-if="loading" role="status" class="muted small">正在读取轮次…</p>
  <p v-if="error" role="alert" class="inline-warning">轮次读取失败：{{error}} <button class="text-button" @click="read(requestedOffset)">重新读取</button></p>
  <template v-if="detail">
   <p v-if="!detail.items.length" class="muted">此页没有检测项目。</p>
   <div v-else class="manual-round-table">
    <table aria-label="轮次检测结果">
     <thead><tr><th>节点与项目</th><th>执行</th><th>检测结果</th><th>保存</th></tr></thead>
     <tbody>
      <tr v-for="(item,index) in detail.items" :key="offset+index">
       <td><strong>{{projectName(item)}}</strong><small :title="`配置 ${item.config_revision_key}`">{{nodeName(item)}} · {{accountName(item)}}</small></td>
       <td>{{stateLabel(item.execution_state)}}</td>
       <td>{{resultText(item)}}
        <small v-if="item.download?.result&&item.download.result.bytes_read>0" :title="item.download.result.bytes_read+' 字节'">已读 {{bytes(item.download.result.bytes_read)}}</small>
        <small v-if="evidence(item)">{{evidence(item)}}</small>
       </td>
       <td>{{stateLabel(item.persistence_state)}}<small v-if="item.service?.persistence_error||item.download?.persistence_error">{{item.service?.persistence_error||item.download?.persistence_error}}</small></td>
      </tr>
     </tbody>
    </table>
   </div>
   <div v-if="detail.next_offset||offset" class="manual-round-pages">
    <span>{{offset+1}}–{{offset+detail.items.length}} / {{manualRoundCount(summary)}} 项</span>
    <button class="button" :disabled="loading||!offset" @click="read(visited[visited.indexOf(offset)-1]||0)">上一页</button>
    <button class="button" :disabled="loading||!detail.next_offset" @click="read(detail.next_offset!)">下一页</button>
   </div>
  </template>
 </Modal>
</template>

<style scoped>
.manual-round-table {overflow-x:auto;margin-top:16px}
.manual-round-table table {border-collapse:collapse;width:100%;min-width:540px;text-align:left;font-size:13px;table-layout:fixed}
.manual-round-table th,.manual-round-table td {padding:11px 8px;border-bottom:1px solid var(--cst-line);vertical-align:top;overflow-wrap:anywhere}
.manual-round-table th {color:var(--cst-muted);font-weight:600}
.manual-round-table th:nth-child(1) {width:30%}
.manual-round-table th:nth-child(2),.manual-round-table th:nth-child(4) {width:14%}
.manual-round-table small {display:block;color:var(--cst-muted);margin-top:4px;line-height:1.5}
.manual-round-pages {display:flex;align-items:center;justify-content:flex-end;gap:8px;margin-top:12px}
.manual-round-pages span {font-size:12px;color:var(--cst-muted)}
</style>
