<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref} from 'vue'
import {useWorkspace} from '../workspace'
import {date,type ManualRoundSummary} from '../domain'
import {manualRoundCount,manualRoundLabel,manualRoundState} from '../measurementRounds'
import {api} from '../api'
import {readLatencyReports,type SavedLatencyReport} from '../latencyReportData'
import {buildLatencyReport,latencyReportText} from '../latencyReport'
import type {AutoPickReportMeta} from '../autoPick'
const props=withDefaults(defineProps<{manualRounds?:ManualRoundSummary[];manualLoading?:boolean;manualError?:boolean}>(),{manualRounds:()=>[]})
const emit=defineEmits<{'open-round':[round:ManualRoundSummary]}>()
const w=useWorkspace(),rows=ref<SavedLatencyReport[]>([]),autoPickRows=ref<AutoPickReportMeta[]>([])
type Source='latency'|'auto'
const reading=ref<Record<Source,boolean>>({latency:false,auto:false}),errors=ref<Record<Source,string>>({latency:'',auto:''})
let mounted=true
async function load(source:Source){
 if(reading.value[source])return
 reading.value[source]=true;errors.value[source]=''
 try{
  if(source==='latency'){const result=await readLatencyReports(w.nodes.value);if(mounted){rows.value=result.reports;errors.value.latency=result.errors.join('；')}}
  else{const picked=await api.autoPickReports(30);if(mounted)autoPickRows.value=picked||[]}
 }catch(e){if(mounted)errors.value[source]=e instanceof Error?e.message:String(e)}
 finally{if(mounted)reading.value[source]=false}
}
onMounted(()=>{void load('latency');void load('auto')})
onBeforeUnmount(()=>{mounted=false})
const loading=computed(()=>reading.value.latency||reading.value.auto||props.manualLoading)
const hasError=computed(()=>!!errors.value.latency||!!errors.value.auto||props.manualError)
const reports=computed(()=>[
 ...rows.value.map(row=>({id:(row.monitor?'monitor:':'manual:')+row.batch.batch_id,at:row.batch.requested_at,kind:row.monitor?'监测报告':'手动检测',summary:latencyReportText(row.batch),issue:buildLatencyReport(row.batch).issues.length>0,row,auto:null,round:null})),
 ...autoPickRows.value.map(auto=>({id:'auto:'+auto.report_id,at:auto.created_at,kind:'自动精选',summary:auto.summary,issue:auto.kept_previous||auto.status!=='selected',row:null,auto,round:null})),
 ...props.manualRounds.map(round=>({id:'round:'+round.round_id,at:round.started_at,kind:manualRoundLabel(round),summary:`${round.node_count} 个节点 · ${manualRoundCount(round)} 项 · ${manualRoundState(round.state)}`,issue:round.state==='interrupted',row:null,auto:null,round}))
].sort((a,b)=>(Date.parse(b.at)||0)-(Date.parse(a.at)||0)))
const days=computed(()=>{
 const groups:{day:string;rows:typeof reports.value}[]=[]
 for(const report of reports.value){const at=new Date(report.at),day=Number.isFinite(at.getTime())?new Intl.DateTimeFormat('zh-CN',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit',day:'2-digit'}).format(at):'日期未知';let group=groups.find(g=>g.day===day);if(!group)groups.push(group={day,rows:[]});group.rows.push(report)}
 return groups
})
function open(r:SavedLatencyReport){w.openLatencyReport(r.batch,r.monitor,rows.value.filter(v=>v.monitor===r.monitor).map(v=>v.batch))}
function openReport(report:(typeof reports.value)[number]){
 if(report.row)open(report.row)
 else if(report.auto)w.openAutoPickReport(report.auto.report_id)
 else if(report.round)emit('open-round',report.round)
}
function time(at:string){const d=new Date(at);return Number.isFinite(d.getTime())?d.toLocaleTimeString('zh-CN',{timeZone:'Asia/Shanghai',hour:'2-digit',minute:'2-digit',hour12:false}):'—'}
</script>
<template>
 <div class="report-list">
  <div class="report-list-intro"><p>检测与精选报告按时间排列</p><small>{{reports.length}} 份已读取{{hasError?' · 部分来源未读取':''}}{{loading?' · 读取中…':''}}</small></div>
  <p v-for="source in (['latency','auto'] as const)" v-show="errors[source]" :key="source" class="inline-warning" role="alert">{{source==='latency'?'延迟与监测报告':'自动精选报告'}}读取失败：{{errors[source]}} <button class="text-button" :disabled="reading[source]" @click="load(source)">{{source==='latency'?'重试延迟与监测':'重试自动精选'}}</button></p>
  <section v-for="group in days" :key="group.day" class="report-day">
   <header><h3>{{group.day}}</h3><small>{{group.rows.length}} 份</small></header>
   <div class="report-day-rows">
    <button v-for="r in group.rows" :key="r.id" class="report-row" @click="openReport(r)">
     <time :datetime="r.at" :title="date(r.at)">{{time(r.at)}}</time>
     <span class="report-summary"><small>{{r.kind}}</small><strong>{{r.summary}}</strong></span>
     <span v-if="r.issue" class="report-attention">有待留意</span><span class="report-open" aria-hidden="true">查看 ›</span>
    </button>
   </div>
  </section>
  <p v-if="loading&&!reports.length" class="muted" role="status">读取报告中…</p>
  <p v-if="!loading&&!hasError&&!reports.length" class="empty-state">还没有已保存的检测或精选报告。</p>
 </div>
</template>
<style scoped>
.report-list{display:grid;gap:18px}.report-list-intro{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px 18px;border:1px solid var(--cst-line-strong);border-radius:13px;background:var(--cst-panel);box-shadow:var(--cst-shadow)}.report-list-intro p{margin:0;font-size:13.5px;font-weight:600}.report-list-intro small{font-size:11.5px;color:var(--cst-muted);padding:2px 9px;border-radius:999px;background:var(--cst-panel-2);border:1px solid var(--cst-line)}
.report-list>.inline-warning{margin:0}.report-day{display:grid;grid-template-columns:120px minmax(0,1fr);gap:18px;align-items:start}.report-day>header{padding:10px 12px;border-radius:11px;background:var(--cst-panel);border:1px solid var(--cst-line);position:sticky;top:calc(var(--cst-nav-height,58px) + 16px)}.report-day h3{font-size:13.5px;margin:0 0 3px;font-weight:700}.report-day header small{font-size:11px;color:var(--cst-muted)}.report-day-rows{display:grid;gap:9px;border-top:0}
.report-row{display:flex;align-items:center;gap:16px;width:100%;text-align:left;padding:14px 18px;border:1px solid var(--cst-line-strong);border-radius:13px;background:var(--cst-panel);box-shadow:var(--cst-shadow);color:var(--cst-text);cursor:pointer;transition:border-color .14s,transform .12s}.report-row:hover{border-color:var(--cst-accent);background:color-mix(in srgb,var(--cst-accent) 4%,var(--cst-panel))}.report-row time{color:var(--cst-accent);font-size:12.5px;font-weight:700;padding:3px 9px;border-radius:8px;background:color-mix(in srgb,var(--cst-accent) 10%,var(--cst-panel-2));flex:none;font-variant-numeric:tabular-nums}.report-summary{display:grid;gap:4px;flex:1;min-width:0}.report-summary small{font-size:11px;font-weight:600;color:var(--cst-muted)}.report-summary strong{font-size:14px;font-weight:600;line-height:1.55}.report-attention{font-size:11px;font-weight:600;padding:2px 8px;border-radius:999px;background:var(--cst-warn-bg);color:var(--cst-warn);border:1px solid var(--cst-warn-line);white-space:nowrap}.report-open{font-size:12px;font-weight:600;padding:4px 10px;border-radius:8px;background:var(--cst-panel-2);border:1px solid var(--cst-line);color:var(--cst-text);white-space:nowrap}.report-row:hover .report-open{color:var(--cst-accent);border-color:var(--cst-accent)}
@media(max-width:700px){.report-day{grid-template-columns:minmax(0,1fr);gap:10px}.report-day>header{display:flex;align-items:baseline;justify-content:space-between;gap:12px;position:static}.report-day h3{margin:0}.report-row{gap:12px;flex-wrap:wrap}.report-summary{min-width:65%}.report-open{margin-left:auto}}
</style>
