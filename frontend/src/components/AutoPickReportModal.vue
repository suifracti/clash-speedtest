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
const tone=computed(()=>report.value?.kept_previous?'warn':report.value?.status==='selected'?'good':'empty')
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
function rateTone(node:AutoPickNode){
  if(!node.samples)return 'empty'
  return node.response_rate>=90?'good':node.response_rate>=60?'warn':'bad'
}
</script>

<template>
<Modal title="自动精选报告" wide @close="emit('close')">
  <p v-if="loading" class="muted" role="status">正在读取报告…</p>
  <p v-else-if="error" class="inline-error" role="alert">{{error}}</p>
  <template v-else-if="report">
    <section class="auto-hero">
      <div class="auto-hero-top">
        <div class="auto-hero-copy">
          <div class="auto-report-head">
            <span class="auto-status-pill" :class="tone"><i aria-hidden="true"/>{{reportStatusLabels[report.status]||report.status}}</span>
            <strong>{{report.summary}}</strong>
          </div>
          <p class="auto-hero-meta">{{date(report.created_at)}} · {{report.trigger==='manual'?'手动运行':'定时运行'}} · 范围 {{report.scope.airport_names?.join('、')||report.scope.airport_ids.join('、')}}<template v-if="report.scope.region_filter"> · 地区“{{report.scope.region_filter }}”</template></p>
        </div>
        <div class="auto-kpi-row">
          <div class="auto-kpi"><small>已测量</small><b>{{report.measured}}</b></div>
          <div class="auto-kpi"><small>延迟合格</small><b>{{report.latency_passed}}</b></div>
          <div class="auto-kpi good"><small>已选中</small><b>{{report.selected_count}}</b></div>
        </div>
      </div>
      <div class="auto-rule-chips" aria-label="精选筛选规则">
        <span>响应率 ≥ {{report.rules.min_response_rate}}%</span>
        <span>稳定延迟 ≤ {{report.rules.max_latency_ms}} ms</span>
        <span>项目：延迟<template v-if="requiredText"> + {{requiredText}}</template><template v-else-if="report.rules.groups?.length"> + {{report.rules.groups.join('、')}} 组</template></span>
      </div>
    </section>

    <div v-for="note in report.notes||[]" :key="note" class="auto-report-note" :class="{'inline-warning':report.kept_previous}">{{note}}</div>

    <div class="auto-table-wrap">
      <table class="auto-report-table">
        <thead><tr><th>节点</th><th>延迟</th><th>响应率</th><th>分组</th><th>说明</th></tr></thead>
        <tbody>
          <tr v-for="node in nodes" :key="node.profile_id+node.node_key" :class="{excluded:!!node.excluded_reason}">
            <td><strong>{{node.display_name}}</strong><small>{{node.airport_name}}</small></td>
            <td class="auto-num">{{node.latency_ms?node.latency_ms+' ms':'—'}}</td>
            <td class="auto-num"><span class="auto-rate-chip" :class="rateTone(node)">{{node.samples?node.response_rate.toFixed(0)+'%':'—'}}</span><small v-if="node.samples">{{node.success_samples}}/{{node.samples}}</small></td>
            <td><span v-if="node.purposes?.length" class="auto-purpose-tags"><b v-for="p in node.purposes" :key="p">{{p}}</b></span><span v-else class="muted">—</span></td>
            <td class="auto-report-reason">
              <span v-if="node.excluded_reason" class="auto-excluded-text">{{node.excluded_reason}}</span>
              <template v-else>
                <span v-if="node.services&&Object.keys(node.services).length" class="auto-reason-line">{{reportServiceText(node.services)}}</span>
                <span v-else class="muted">通过延迟规则</span>
                <span v-if="purposeText(node)" class="auto-reason-line auto-reason-gap">{{purposeText(node)}}</span>
              </template>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="report.subscription_path" class="auto-sub-path">订阅文件：<code>{{report.subscription_path}}</code></p>
  </template>
  <template #footer><button class="button primary" @click="emit('close')">关闭</button></template>
</Modal>
</template>

<style scoped>
.auto-hero{display:grid;gap:12px;padding:16px 18px;border:1px solid var(--cst-line);border-top:2.5px solid transparent;border-radius:14px;background-image:linear-gradient(var(--cst-panel-solid),var(--cst-panel-solid)),var(--cst-aurora-ribbon);background-origin:border-box;background-clip:padding-box,border-box;box-shadow:var(--cst-shadow-xs);margin-bottom:12px}
.auto-hero-top{display:flex;align-items:flex-start;justify-content:space-between;gap:14px;flex-wrap:wrap}
.auto-hero-copy{display:grid;gap:6px;min-width:0;flex:1 1 280px}
.auto-report-head{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.auto-report-head strong{font-size:15px;font-weight:700;color:var(--cst-text)}
.auto-status-pill{display:inline-flex;align-items:center;gap:6px;padding:2px 10px;border-radius:999px;font-size:11.5px;font-weight:700;border:1px solid var(--cst-line);background:var(--cst-panel-2);color:var(--cst-muted)}
.auto-status-pill i{width:6px;height:6px;border-radius:50%;background:currentColor}
.auto-status-pill.good{color:var(--cst-good);background:var(--cst-good-bg);border-color:color-mix(in srgb,var(--cst-good) 28%,transparent)}
.auto-status-pill.warn{color:var(--cst-warn);background:var(--cst-warn-bg);border-color:color-mix(in srgb,var(--cst-warn) 32%,transparent)}
.auto-hero-meta{margin:0;font-size:12px;color:var(--cst-muted);line-height:1.5}
.auto-kpi-row{display:flex;gap:8px;flex-wrap:wrap}
.auto-kpi{display:grid;gap:2px;min-width:68px;padding:7px 11px;border-radius:10px;border:1px solid var(--cst-line);background:var(--cst-panel-2);text-align:center}
.auto-kpi small{font-size:10.5px;color:var(--cst-muted)}
.auto-kpi b{font-size:18px;font-weight:750;font-variant-numeric:tabular-nums;color:var(--cst-text)}
.auto-kpi.good{background:var(--cst-good-bg);border-color:color-mix(in srgb,var(--cst-good) 28%,transparent)}
.auto-kpi.good b{color:var(--cst-good)}
.auto-rule-chips{display:flex;flex-wrap:wrap;gap:6px;padding-top:10px;border-top:1px solid var(--cst-line)}
.auto-rule-chips span{display:inline-flex;align-items:center;padding:3px 9px;border-radius:999px;background:var(--cst-panel-2);border:1px solid var(--cst-line);font-size:11.5px;color:var(--cst-muted)}
.auto-report-note{padding:9px 12px;border-radius:10px;border:1px solid var(--cst-line);background:var(--cst-panel-2);font-size:12.5px;margin:8px 0}
.auto-table-wrap{overflow:auto;border:1px solid var(--cst-line);border-radius:12px;background:var(--cst-panel);margin-top:10px}
.auto-report-table{width:100%;border-collapse:collapse;font-size:12.5px}
.auto-report-table th{text-align:left;color:var(--cst-muted);font-weight:650;font-size:11.5px;background:var(--cst-panel-2);border-bottom:1px solid var(--cst-line);padding:9px 12px;white-space:nowrap}
.auto-report-table td{border-bottom:1px solid var(--cst-line);padding:9px 12px;vertical-align:top}
.auto-report-table tbody tr:last-child td{border-bottom:0}
.auto-report-table tbody tr:hover{background:color-mix(in srgb,var(--cst-accent) 4%,var(--cst-panel))}
.auto-report-table tbody tr.excluded{opacity:.76}
.auto-report-table small{display:block;color:var(--cst-muted);font-size:11px;margin-top:2px}
.auto-num{white-space:nowrap;font-variant-numeric:tabular-nums;font-weight:600}
.auto-rate-chip{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;font-size:11.5px;font-weight:700}
.auto-rate-chip.good{color:var(--cst-good);background:var(--cst-good-bg)}
.auto-rate-chip.warn{color:var(--cst-warn);background:var(--cst-warn-bg)}
.auto-rate-chip.bad{color:var(--cst-bad);background:var(--cst-bad-bg)}
.auto-purpose-tags{display:flex;flex-wrap:wrap;gap:4px}
.auto-purpose-tags b{padding:1px 7px;border-radius:999px;background:var(--cst-accent-soft);color:var(--cst-accent);font-size:11px;font-weight:650;white-space:nowrap}
.auto-report-reason{max-width:340px;word-break:break-word;line-height:1.5}
.auto-excluded-text{color:var(--cst-warn);font-weight:550}
.auto-reason-line{display:block}
.auto-reason-gap{margin-top:3px;color:var(--cst-muted);font-size:11.5px}
.auto-sub-path{margin:10px 0 0;font-size:11.5px;color:var(--cst-muted)}
.auto-sub-path code{font-family:var(--cst-mono,ui-monospace,monospace);padding:1px 6px;border-radius:5px;background:var(--cst-panel-2);border:1px solid var(--cst-line);color:var(--cst-text)}
</style>
