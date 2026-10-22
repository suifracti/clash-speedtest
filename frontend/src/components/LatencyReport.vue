<script setup lang="ts">
import {computed} from 'vue'
import {nodeDisplayName} from '../nodePresentation'
import type {BatchItem,LatencyBatch} from '../domain'
import {buildLatencyReport,compareLatencyReports,type LatencyReportGroup,type LatencyReportRow} from '../latencyReport'

const props=withDefaults(defineProps<{
 batch:LatencyBatch
 previous?:LatencyBatch[]
 airportNames?:Record<string,string>
 monitor?:boolean
 entryNotice?:string
}>(),{previous:()=>[],airportNames:()=>({}),monitor:false,entryNotice:''})
const emit=defineEmits<{
 (event:'retry',items:BatchItem[]):void
 (event:'export',items:BatchItem[]):void
 (event:'inspect',item:BatchItem):void
}>()
const report=computed(()=>buildLatencyReport(props.batch,props.airportNames))
const comparison=computed(()=>compareLatencyReports(props.batch,report.value,props.previous))
const issueItems=computed(()=>{
 const unique=new Map<string,BatchItem>()
 for(const issue of report.value.issues)unique.set(issue.row.key,issue.item)
 return [...unique.values()]
})
const summaryLine=computed(()=>{
 const s=report.value.summary
 const places=s.airportNames.length===1?s.airportNames[0]:s.airportNames.length?`${s.airportNames.length} 个机场`:'机场未知'
 const plan=s.targetId==='all'?`四站各 ${s.sampleCountPerSite??'—'} 次`:s.sampleCountPerSite?`每站 ${s.sampleCountPerSite} 次`:'采样次数未知'
 return `${formatTime(s.requestedAt)} · ${places} ${s.nodeCount} 个节点 · ${plan} · 用时 ${formatDuration(s.durationMs)}`
})
const comparisonLine=computed(()=>{
 const c=comparison.value
 if(c.reason)return `没有可比记录：${c.reason}`
 const delta=c.stableMedianDeltaMs===null?'稳定中位差暂无数据':`稳定中位差 ${formatSignedMs(c.stableMedianDeltaMs)}`
 return `和上次同条件（按节点取最近一轮，${c.comparableNodes} 个可比节点）比：新增失败 ${c.newlyFailed} · 恢复 ${c.recovered} · ${delta}`
})

const countCards=computed(()=>[
 {key:'usable',label:'能用',value:report.value.counts.usable,tone:'good'},
 {key:'slow',label:'偏慢 >1.5s',value:report.value.counts.slow,tone:'warn'},
 {key:'no-response',label:'无响应',value:report.value.counts.noResponse,tone:'bad'},
 {key:'suspected',label:'疑似检测问题',value:report.value.counts.suspected,tone:'suspected'}
])

function formatTime(value?:string){
 if(!value)return '时间未知'
 const at=new Date(value)
 if(!Number.isFinite(at.getTime()))return '时间未知'
 return new Intl.DateTimeFormat('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hourCycle:'h23'}).format(at)
}
function formatDuration(value:number|null){
 if(value===null||!Number.isFinite(value))return '未记录'
 const seconds=Math.max(0,Math.round(value/1000)),minutes=Math.floor(seconds/60),remainder=seconds%60
 return minutes?`${minutes} 分 ${remainder} 秒`:`${seconds} 秒`
}
function formatMs(value:number|null|undefined){return value===null||value===undefined||!Number.isFinite(value)?'—':`${Math.round(value)} ms`}
function formatSignedMs(value:number){const rounded=Math.round(value);return `${rounded>0?'+':''}${rounded} ms`}
function statusLabel(row:LatencyReportRow){
 return ({usable:'能用',slow:'偏慢', 'no-response':'无响应',skipped:'未执行'} as const)[row.status]
}
function issueDetail(group:LatencyReportGroup,row:LatencyReportRow){
 return group.issues.find(issue=>issue.row.key===row.key)?.detail||row.skipReason||row.suspectedReasons.join('；')
}
function uniqueItems(rows:LatencyReportRow[]){
 const unique=new Map<string,BatchItem>()
 for(const row of rows)unique.set(row.key,row.item)
 return [...unique.values()]
}
function siteLine(row:LatencyReportRow){return row.sites.map(site=>`${site.name} ${formatMs(site.stableMs)}`).join(' · ')}
</script>

<template>
 <section class="latency-report" aria-label="延迟检测报告">
  <header class="latency-report-head">
   <div>
    <p class="latency-report-kicker">{{props.monitor?'持续监测':'本轮检测'}}</p>
    <h2>{{props.monitor?'持续监测报告':'延迟检测报告'}}</h2>
    <p class="latency-report-meta">{{summaryLine}}</p>
   </div>
   <span class="latency-report-state">{{props.batch.state==='completed'?'已完成':props.batch.state==='cancelling'?'正在取消':props.batch.state==='cancelled'?'已取消':props.batch.state==='failed'?'有项目失败':'检测结束'}}</span>
  </header>

  <div class="latency-report-counts" aria-label="检测结果计数">
   <article v-for="card in countCards" :key="card.key" class="latency-report-count" :class="card.tone">
    <small>{{card.label}}</small><strong>{{card.value}}</strong>
    <span v-if="card.key==='suspected'">与其他结果重叠</span>
   </article>
  </div>
  <p v-if="report.counts.skipped" class="latency-report-note">
   未执行 {{report.counts.skipped}} 项（不计入无响应）
  </p>

  <p class="latency-report-compare">{{comparisonLine}}</p>

  <div class="latency-report-columns">
   <section class="latency-report-panel">
    <header><div><h3>最快 5 个</h3><small>按四站稳定中位延迟排序</small></div></header>
    <ol v-if="report.fastest5.length" class="latency-report-fastest">
     <li v-for="(row,index) in report.fastest5" :key="row.key">
      <span class="latency-report-rank">{{index+1}}</span>
      <div class="latency-report-node"><button class="text-button" @click="emit('inspect',row.item)">{{nodeDisplayName(row.item)}}</button><small>{{row.airportName}}<template v-if="row.nodeType!=='未知协议'"> · {{row.nodeType}}</template></small></div>
      <strong>{{formatMs(row.stableMs)}}</strong>
     </li>
    </ol>
    <p v-else class="latency-report-empty">没有四站齐全的稳定延迟结果。</p>
   </section>

   <section class="latency-report-panel latency-report-problems">
    <header><div><h3>问题按原因</h3><small>节点可打开明细，问题可直接重测</small></div></header>
    <div v-if="report.groups.length" class="latency-report-groups">
     <article v-for="(group,index) in report.groups" :key="group.kind+'-'+group.reason+'-'+index" class="latency-report-group" :class="group.kind">
      <header>
       <div><h4>{{group.reason}} <span>· {{group.rows.length}} 个</span></h4><small v-if="group.kind==='skipped'">未执行项目单列，不计为无响应</small></div>
       <button v-if="group.rows.length&&group.kind!=='skipped'" class="button ghost latency-report-retry" @click="emit('retry',uniqueItems(group.rows))">重测</button>
      </header>
      <p v-if="group.kind==='slow'&&props.entryNotice" class="latency-report-group-note">{{props.entryNotice}}</p>
      <ul>
       <li v-for="row in group.rows" :key="row.key">
        <button class="text-button" @click="emit('inspect',row.item)">{{nodeDisplayName(row.item)}}</button>
        <small>{{issueDetail(group,row)}}</small>
       </li>
      </ul>
     </article>
    </div>
    <p v-else class="latency-report-empty">本轮没有需要处理的问题。</p>
   </section>
  </div>

  <footer class="latency-report-actions">
   <button v-if="issueItems.length" class="button" @click="emit('retry',issueItems)">重测问题（{{issueItems.length}}）</button>
   <button class="button primary" :disabled="!report.usableItems.length" @click="emit('export',report.usableItems)">导出能用的 {{report.usableItems.length}} 个</button>
  </footer>

  <details class="latency-report-details">
   <summary>查看逐个明细（{{report.rows.length}}）</summary>
   <div class="latency-report-table-wrap">
    <table>
     <thead><tr><th>节点</th><th>结果</th><th>首次四站中位</th><th>稳定四站中位</th><th>各站稳定值</th><th>说明</th></tr></thead>
     <tbody>
      <tr v-for="row in report.rows" :key="row.key">
       <td><button class="text-button" @click="emit('inspect',row.item)">{{nodeDisplayName(row.item)}}</button><small>{{row.airportName}} · {{row.nodeType}}</small></td>
       <td><span class="latency-report-status" :class="row.status">{{statusLabel(row)}}</span><small v-if="row.suspected" class="latency-report-suspected">疑似检测问题</small></td>
       <td>{{formatMs(row.firstMedianMs)}}</td><td>{{formatMs(row.stableMs)}}</td><td>{{siteLine(row)}}</td>
       <td><small>{{row.suspectedReasons.join('；')||row.skipReason||'—'}}</small></td>
      </tr>
     </tbody>
    </table>
   </div>
  </details>
 </section>
</template>

<style scoped>
.latency-report{display:flex;flex-direction:column;gap:14px;color:var(--cst-text)}
.latency-report-head{display:flex;align-items:flex-start;justify-content:space-between;gap:14px}
.latency-report-kicker{color:var(--cst-muted);font-size:11px;line-height:1.3}
.latency-report-head h2{margin:3px 0 4px;font-size:19px}
.latency-report-meta{color:var(--cst-muted);font-size:12px;line-height:1.45}
.latency-report-state{flex:none;border:1px solid var(--cst-line);border-radius:999px;padding:4px 9px;color:var(--cst-muted);font-size:11px}
.latency-report-counts{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px}
.latency-report-count{display:flex;min-height:70px;flex-direction:column;justify-content:center;gap:2px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel);padding:9px 12px}
.latency-report-count small,.latency-report-count span{color:var(--cst-muted);font-size:11px}
.latency-report-count strong{font-size:23px;line-height:1.1;font-variant-numeric:tabular-nums}
.latency-report-count.good{background:color-mix(in srgb,var(--cst-good) 10%,var(--cst-panel))}.latency-report-count.warn{background:color-mix(in srgb,var(--cst-warn) 12%,var(--cst-panel))}.latency-report-count.bad{background:color-mix(in srgb,var(--cst-bad) 10%,var(--cst-panel))}.latency-report-count.good strong{color:var(--cst-good)}.latency-report-count.warn strong{color:var(--cst-warn)}.latency-report-count.bad strong{color:var(--cst-bad)}.latency-report-count.suspected strong{color:var(--cst-accent)}
.latency-report-note{color:var(--cst-muted);font-size:11px;margin-top:-8px}
.latency-report-compare{border:1px solid var(--cst-line);border-radius:8px;background:var(--cst-panel);padding:9px 11px;font-size:12px;line-height:1.5}
.latency-report-columns{display:grid;grid-template-columns:minmax(0,.9fr) minmax(0,1.1fr);gap:10px;align-items:start}
.latency-report-panel{min-width:0;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel);padding:11px}
.latency-report-panel>header,.latency-report-group>header{display:flex;align-items:center;justify-content:space-between;gap:10px}
.latency-report-panel h3{font-size:13px}.latency-report-panel header small{color:var(--cst-muted);font-size:10px}
.latency-report-fastest{list-style:none;padding:0;margin:8px 0 0}
.latency-report-fastest li{display:flex;align-items:center;gap:9px;padding:7px 0;border-top:1px solid var(--cst-line)}
.latency-report-rank{display:grid;place-items:center;width:22px;height:22px;flex:none;border-radius:7px;background:var(--cst-hover);color:var(--cst-muted);font-size:11px}
.latency-report-node{display:flex;min-width:0;flex:1;flex-direction:column;align-items:flex-start}
.latency-report-node .text-button,.latency-report-group li .text-button,.latency-report-table-wrap .text-button{max-width:100%;white-space:normal;text-align:left}
.latency-report-node small{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--cst-muted)}
.latency-report-fastest li>strong{flex:none;font-size:12px;font-variant-numeric:tabular-nums}
.latency-report-empty{padding:12px 0 3px;color:var(--cst-muted);font-size:12px}
.latency-report-groups{display:flex;flex-direction:column;gap:7px;margin-top:8px}
.latency-report-group{border:1px solid var(--cst-line);border-radius:8px;padding:8px 9px}
.latency-report-group>header h4{font-size:12px;font-weight:600}.latency-report-group>header h4 span{color:var(--cst-muted);font-weight:400}
.latency-report-group>header small{display:block;margin-top:2px;color:var(--cst-muted)}.latency-report-group-note{margin:7px 0 0;padding:6px 8px;border-radius:6px;background:#fff6e5;border:1px solid #e3c184;color:#805211;font-size:10px;line-height:1.55}.latency-report-group ul{display:flex;flex-direction:column;gap:4px;list-style:none;padding:0;margin:7px 0 0}
.latency-report-group li{display:flex;align-items:baseline;justify-content:space-between;gap:8px;min-width:0}
.latency-report-group li small{min-width:0;color:var(--cst-muted);font-size:10px;text-align:right}
.latency-report-group.suspected{border-color:color-mix(in srgb,var(--cst-accent) 35%,var(--cst-line))}
.latency-report-group.skipped{background:color-mix(in srgb,var(--cst-panel) 85%,var(--cst-hover))}
.latency-report-retry{min-height:28px;padding:3px 8px;font-size:11px}
.latency-report-actions{display:flex;justify-content:flex-end;gap:8px;flex-wrap:wrap}
.latency-report-details{border-top:1px solid var(--cst-line);padding-top:10px}
.latency-report-details summary{cursor:pointer;color:var(--cst-muted);font-size:12px}
.latency-report-table-wrap{margin-top:9px;overflow:auto;border:1px solid var(--cst-line);border-radius:8px}
.latency-report-table-wrap table{width:100%;min-width:720px;border-collapse:collapse;font-size:11px}
.latency-report-table-wrap th,.latency-report-table-wrap td{padding:7px 8px;border-bottom:1px solid var(--cst-line);text-align:left;vertical-align:top}
.latency-report-table-wrap th{position:sticky;top:0;background:var(--cst-panel);color:var(--cst-muted);font-weight:500;white-space:nowrap}
.latency-report-table-wrap td:first-child{min-width:135px}
.latency-report-table-wrap td:first-child small,.latency-report-table-wrap td:nth-child(2) small,.latency-report-table-wrap td:last-child small{display:block;max-width:220px;margin-top:2px;color:var(--cst-muted);line-height:1.4}
.latency-report-status{white-space:nowrap}.latency-report-status.usable{color:var(--cst-good)}.latency-report-status.slow{color:var(--cst-warn)}.latency-report-status.no-response{color:var(--cst-bad)}.latency-report-status.incomplete,.latency-report-status.skipped{color:var(--cst-muted)}
.latency-report-suspected{color:var(--cst-accent)!important}
@media(max-width:760px){.latency-report-counts{grid-template-columns:repeat(2,minmax(0,1fr))}.latency-report-columns{grid-template-columns:minmax(0,1fr)}.latency-report-head{gap:8px}.latency-report-meta{max-width:58vw}.latency-report-group li{align-items:flex-start;flex-direction:column;gap:1px}.latency-report-group li small{text-align:left}.latency-report-actions{justify-content:stretch}.latency-report-actions .button{flex:1}}
</style>
