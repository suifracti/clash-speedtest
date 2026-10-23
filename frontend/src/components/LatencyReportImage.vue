<script setup lang="ts">
import type {LatencyReport} from '../latencyReport'
import {nodeDisplayName} from '../nodePresentation'
import ReportMs from './ReportMs.vue'
import {reportImageText as safe} from '../reportImage'
defineProps<{report:LatencyReport;summary:string;comparison:string;monitor:boolean}>()
declare const __CST_VERSION__:string
const version=__CST_VERSION__
const status=(value:string)=>({usable:'能用',slow:'偏慢',unstable:'不稳定','no-response':'无响应',skipped:'未执行'}[value]||'未记录')
</script>
<template>
 <article class="report-image" aria-label="待导出的检测报告">
  <header><p class="eyebrow">CST · {{monitor?'持续监测':'延迟检测'}}报告</p><h1>{{safe(report.summary.airportNames.join(' · ')||'机场未知')}}</h1><p>{{safe(summary)}}</p></header>
  <div class="image-counts">
   <div class="good"><small>能用</small><strong>{{report.counts.usable}}</strong></div>
   <div class="warn"><small>偏慢/不稳定</small><strong>{{report.counts.slow+report.counts.unstable}}</strong></div>
   <div class="bad"><small>无响应</small><strong>{{report.counts.noResponse}}</strong></div>
   <div><small>疑似检测问题</small><strong>{{report.counts.suspected}}</strong><small v-if="report.counts.suspected">与其他结果重叠</small></div>
  </div>
  <p class="image-compare">{{safe(comparison)}}</p>
  <div class="image-highlights">
   <section><h2>最快 5 个 <small>稳定延迟</small></h2><table><tbody><tr v-for="(row,i) in report.fastest5" :key="row.key"><td>{{i+1}}. {{safe(nodeDisplayName(row.item))}}</td><td class="number"><ReportMs :value="row.stableMs"/></td></tr></tbody></table><p v-if="!report.fastest5.length">没有四站齐全的稳定延迟结果</p></section>
   <section><h2>问题分组</h2><div v-for="(group,i) in report.groups" :key="i" class="image-problem"><b>{{group.reason}} · {{group.rows.length}}</b><p>{{group.rows.map(row=>safe(nodeDisplayName(row.item))).join('、')}}</p></div><p v-if="!report.groups.length">本轮没有需要处理的问题</p></section>
  </div>
  <h2>节点结果 <small>{{report.rows.length}} 个节点</small></h2>
  <table class="image-nodes"><thead><tr><th>节点</th><th>结果</th><th class="number">稳定延迟</th><th class="number">首次连接</th><th class="number">响应</th></tr></thead><tbody>
   <tr v-for="row in report.rows" :key="row.key"><td>{{safe(nodeDisplayName(row.item))}}<small>{{safe(row.airportName)}} · {{row.nodeType}}</small></td><td :class="row.status==='usable'?'good':row.status==='no-response'?'bad':'warn'">{{status(row.status)}}<small v-if="row.suspected">疑似检测问题</small></td><td class="number"><ReportMs :value="row.stableMs"/></td><td class="number"><ReportMs :value="row.firstMedianMs"/></td><td class="number">{{row.responseSamples}}/{{row.attemptedSamples}}</td></tr>
  </tbody></table>
  <footer><strong>CST</strong><span>{{version}}</span><span>稳定延迟不含首次采样 · 仅包含检测摘要</span></footer>
 </article>
</template>
<style scoped>
.report-image{box-sizing:border-box;width:1000px;padding:32px;background:#fff;color:#202b3b;font:14px/1.55 -apple-system,BlinkMacSystemFont,"PingFang SC",sans-serif;text-align:left}
.report-image *{box-sizing:border-box}.report-image p,.report-image h1,.report-image h2{margin:0}.report-image header{border-top:5px solid #1669e8;padding:18px 0 22px}.report-image .eyebrow{font-size:12px;letter-spacing:1px;color:#1669e8}.report-image h1{font-size:28px;margin:4px 0}.report-image header>p:last-child{color:#64748b}.report-image h2{font-size:16px;margin:16px 0 8px}.report-image h2 small{font-weight:400;color:#64748b;font-size:12px;margin-left:8px}
.image-counts{display:flex;margin:0 -5px}.image-counts>div{width:25%;margin:0 5px;padding:12px 16px;background:#edf2f8;border-radius:8px}.image-counts strong{display:block;font-size:30px;line-height:1.3}.image-counts small{display:block}.image-counts .good{background:#e5f4eb}.image-counts .warn{background:#fff2dc}.image-counts .bad{background:#fce8e8}.report-image .good{color:#167545}.report-image .warn{color:#9b610a}.report-image .bad{color:#b53030}.image-compare{padding:14px 0;border-bottom:1px solid #dce3ed}.image-highlights{display:flex;margin:0 -10px 18px}.image-highlights section{width:50%;padding:0 10px}.image-problem{padding:7px 0;border-bottom:1px solid #e7ecf3}.image-problem p{color:#53637a;font-size:12px;overflow-wrap:anywhere}
.report-image table{width:100%;border-collapse:collapse;table-layout:fixed}.report-image th,.report-image td{padding:9px 12px;border-bottom:1px solid #e1e7ef;vertical-align:top;overflow-wrap:anywhere}.report-image th{background:#edf2f8;font-weight:600;color:#53637a}.image-highlights td:first-child{width:76%}.image-nodes th:first-child{width:42%}.image-nodes th:nth-child(2){width:16%}.image-nodes tbody tr:nth-child(even){background:#f7f9fc}.report-image td small{display:block;font-size:11px;color:#738097}/* html2canvas 用 canvas 画字，不认 tabular-nums；这里保留等宽会让浏览器排版比字形宽，撑大数字与 ms 的间距 */
.report-image .number{text-align:right;white-space:nowrap}.report-image :deep(.report-ms){font-variant-numeric:normal}.report-image footer{display:flex;align-items:center;border-top:2px solid #1669e8;margin-top:24px;padding-top:14px;color:#718097;font-size:12px}.report-image footer strong{font-size:19px;color:#1669e8;margin-right:12px}.report-image footer span:last-child{margin-left:auto}
</style>
