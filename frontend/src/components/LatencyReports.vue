<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {useWorkspace} from '../workspace'
import {date} from '../domain'
import {readLatencyReports,type SavedLatencyReport} from '../latencyReportData'
import {buildLatencyReport,latencyReportText} from '../latencyReport'
const w=useWorkspace(),rows=ref<SavedLatencyReport[]>([]),loading=ref(true),error=ref('')
onMounted(async()=>{try{rows.value=await readLatencyReports(w.nodes.value)}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}})
function open(r:SavedLatencyReport){w.openLatencyReport(r.batch,r.monitor,rows.value.filter(v=>v.monitor===r.monitor).map(v=>v.batch))}
</script>
<template><div class="report-list"><p v-if="loading" class="muted">读取报告中…</p><p v-else-if="error" class="bad">{{error}}</p><p v-else-if="!rows.length" class="empty-state">还没有延迟检测报告。</p><button v-for="r in rows" :key="r.batch.batch_id" class="report-row" @click="open(r)"><time>{{date(r.batch.requested_at)}}</time><span>{{latencyReportText(r.batch)}}</span><small v-if="r.monitor" class="badge">监测</small><i v-if="buildLatencyReport(r.batch).issues.length" title="有需要注意的结果"/><span class="muted">查看报告 ›</span></button></div></template>
<style scoped>
.report-list{display:grid;gap:8px}.report-row{display:flex;align-items:center;gap:16px;width:100%;text-align:left;padding:14px 16px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel);color:var(--cst-text)}.report-row time{color:var(--cst-muted);font-size:12px}.report-row>span:first-of-type{flex:1}.report-row i{width:7px;height:7px;border-radius:50%;background:var(--cst-warn)}
</style>
