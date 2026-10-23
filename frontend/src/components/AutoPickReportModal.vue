<script setup lang="ts">
import {computed,onMounted,ref} from 'vue'
import {api} from '../api'
import {date} from '../domain'
import {reportStatusLabels,groupServices,type AutoPickNode,type AutoPickReport} from '../autoPick'
import {reportPurposeText,reportServiceText} from '../autoPickReportText'
import Modal from './Modal.vue'

const props=defineProps<{reportId:string}>()
const emit=defineEmits<{close:[]}>()
const report=ref<AutoPickReport|null>(null),loading=ref(true),error=ref('')
const nodes=computed(()=>report.value?.nodes||[])
const tone=computed(()=>report.value?.kept_previous?'warn':report.value?.status==='selected'?'good':'')
const requiredText=computed(()=>{
  const required=report.value?.rules?.required_services
  if(!required)return ''
  return Object.entries(required).map(([group,ids])=>`${group}（必需 ${ids.map(serviceLabel).join('、')}）`).join(' + ')
})
const serviceLabels:Record<string,string>={}
for(const group of Object.values(groupServices))for(const service of group)serviceLabels[service.id]=service.label
function serviceLabel(id:string){return serviceLabels[id]||id}
onMounted(async()=>{
  try{report.value=await api.autoPickReport(props.reportId)}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}
})
function purposeText(node:AutoPickNode){return reportPurposeText(node,report.value?.rules)}
</script>

<template>
<Modal title="自动精选报告" wide @close="emit('close')">
  <p v-if="loading" class="muted" role="status">正在读取报告…</p>
  <p v-else-if="error" class="inline-error" role="alert">{{error}}</p>
  <template v-else-if="report">
    <div class="auto-report-head">
      <span class="badge" :class="tone">{{reportStatusLabels[report.status]||report.status}}</span>
      <strong>{{report.summary}}</strong>
    </div>
    <p class="muted small">{{date(report.created_at)}} · {{report.trigger==='manual'?'手动运行':'定时运行'}} · 范围 {{report.scope.airport_names?.join('、')||report.scope.airport_ids.join('、')}}<template v-if="report.scope.region_filter"> · 地区“{{report.scope.region_filter }}”</template></p>
    <ul class="auto-report-rules">
      <li>响应率 ≥ {{report.rules.min_response_rate}}%</li>
      <li>稳定延迟（去掉首次采样后的中位数）≤ {{report.rules.max_latency_ms}} ms</li>
      <li>检测项目：延迟<template v-if="requiredText"> + {{requiredText}}</template><template v-else-if="report.rules.groups?.length"> + {{report.rules.groups.join('、')}} 组</template></li>
      <li>测量 {{report.measured}} 个 · 延迟合格 {{report.latency_passed}} 个 · 选中 {{report.selected_count}} 个</li>
    </ul>
    <div v-for="note in report.notes||[]" :key="note" class="auto-report-note" :class="{'inline-warning':report.kept_previous}">{{note}}</div>
    <table class="auto-report-table">
      <thead><tr><th>节点</th><th>延迟</th><th>响应率</th><th>分组</th><th>说明</th></tr></thead>
      <tbody>
        <tr v-for="node in nodes" :key="node.profile_id+node.node_key">
          <td><strong>{{node.display_name}}</strong><small>{{node.airport_name}}</small></td>
          <td>{{node.latency_ms?node.latency_ms+' ms':'—'}}</td>
          <td>{{node.samples?node.response_rate.toFixed(0)+'%':'—'}}<small v-if="node.samples">{{node.success_samples}}/{{node.samples}}</small></td>
          <td>{{node.purposes?.join('、')||'—'}}</td>
          <td class="auto-report-reason">
            <span v-if="node.excluded_reason">{{node.excluded_reason}}</span>
            <template v-else-if="node.services&&Object.keys(node.services).length">
              <span class="auto-reason-line">{{reportServiceText(node.services)}}</span>
              <span v-if="purposeText(node)" class="auto-reason-line auto-reason-gap">{{purposeText(node)}}</span>
            </template>
            <span v-else class="muted">通过延迟规则</span>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-if="report.subscription_path" class="muted small">订阅文件：{{report.subscription_path}}</p>
  </template>
  <template #footer><button class="button" @click="emit('close')">关闭</button></template>
</Modal>
</template>

<style scoped>
.auto-report-head{display:flex;align-items:center;gap:10px;margin-bottom:6px}
.auto-report-rules{margin:10px 0;padding-left:18px;font-size:13px;color:var(--cst-muted)}
.auto-report-note{padding:8px 10px;border-radius:8px;background:var(--cst-panel);font-size:13px;margin:6px 0}
.auto-report-table{width:100%;border-collapse:collapse;font-size:13px;margin-top:10px}
.auto-report-table th{text-align:left;color:var(--cst-muted);font-weight:500;border-bottom:1px solid var(--cst-line);padding:6px 8px}
.auto-report-table td{border-bottom:1px solid var(--cst-line);padding:6px 8px;vertical-align:top}
.auto-report-table small{display:block;color:var(--cst-muted);font-size:11px}
.auto-report-reason{max-width:340px;word-break:break-word}
.auto-reason-line{display:block}
.auto-reason-gap{margin-top:3px;color:var(--cst-muted)}
</style>
