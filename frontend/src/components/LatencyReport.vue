<script setup lang="ts">
import {computed,nextTick,ref,watch,onMounted} from 'vue'
import LatencyReportImage from './LatencyReportImage.vue'
import ReportMs from './ReportMs.vue'
import {saveReportImage} from '../reportImage'
import {nodeDisplayName} from '../nodePresentation'
import {stateLabel,type BatchItem,type LatencyBatch} from '../domain'
import {partialLatencyError} from '../inspectorLatency'
import {partialLatencyLabel} from '../latencyTargets'
import {buildLatencyReport,compareLatencyReports,latencyAirportSummary,latencyHttpNotices,latencySlowThresholdNote,type LatencyReportGroup,type LatencyReportRow,type LatencyReportStatus} from '../latencyReport'

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
const imageBusy=ref(false),imageMessage=ref(''),imageError=ref(false),imageRoot=ref<HTMLElement>()
async function exportImage(){
 imageBusy.value=true;imageMessage.value='';imageError.value=false
 try{
  await nextTick()
  const saved=await saveReportImage(imageRoot.value!.firstElementChild as HTMLElement,'CST-report-'+props.batch.requested_at.replace(/[^0-9]/g,'').slice(0,14)+'.png')
  imageMessage.value=saved?'图片已保存':'已取消保存'
 }catch(error){imageError.value=true;imageMessage.value='导出图片失败：'+(error instanceof Error?error.message:String(error))}
 finally{imageBusy.value=false}
}
const report=computed(()=>buildLatencyReport(props.batch,props.airportNames))
const reportTitle=ref<HTMLElement>()
onMounted(()=>nextTick(()=>reportTitle.value?.focus({preventScroll:true})))
const reportView=ref<'overview'|'details'>('overview')
const detailStatus=ref<'all'|LatencyReportStatus>('all')
const detailSearch=ref('')
const detailPage=ref(1)
const detailPageSize=15
const expandedGroups=ref(new Set<number>())
function toggleGroup(index:number){const next=new Set(expandedGroups.value);next.has(index)?next.delete(index):next.add(index);expandedGroups.value=next}
function openDetails(status:'all'|LatencyReportStatus='all'){detailStatus.value=status;detailPage.value=1;reportView.value='details'}
watch(()=>props.batch.batch_id,()=>{reportView.value='overview';detailStatus.value='all';detailSearch.value='';detailPage.value=1})
watch([detailStatus,detailSearch],()=>{detailPage.value=1})
const airportSummary=computed(()=>latencyAirportSummary(report.value))
const httpNotices=computed(()=>latencyHttpNotices(report.value))
const comparison=computed(()=>compareLatencyReports(props.batch,report.value,props.previous))
const issueItems=computed(()=>{
 const unique=new Map<string,BatchItem>()
 for(const issue of report.value.issues)unique.set(issue.row.key,issue.item)
 return [...unique.values()]
})
const summaryLine=computed(()=>{
 const s=report.value.summary
 const places=s.airportNames.length===1?s.airportNames[0]:s.airportNames.length?`${s.airportNames.length} 个机场`:'机场未知'
 const plan=s.targetId==='all'?`多站各 ${s.sampleCountPerSite??'—'} 次`:s.sampleCountPerSite?`每站 ${s.sampleCountPerSite} 次`:'采样次数未知'
 return `${formatTime(s.requestedAt)} · ${places} ${s.nodeCount} 个节点 · ${plan} · 用时 ${formatDuration(s.durationMs)}`
})
const comparisonLine=computed(()=>{
 const c=comparison.value
 if(c.reason)return `没有可比记录：${c.reason}`
 const delta=c.stableMedianDeltaMs===null?'稳定中位差暂无数据':`稳定中位差 ${formatSignedMs(c.stableMedianDeltaMs)}`
 return `和上次同条件（按节点取最近一轮，${c.comparableNodes} 个可比节点）比：新增失败 ${c.newlyFailed} · 恢复 ${c.recovered} · ${delta}`
})

const countCards=computed<{key:LatencyReportStatus;label:string;value:number;tone:string}[]>(()=>[
 {key:'usable',label:'能用',value:report.value.counts.usable,tone:'good'},
 {key:'slow',label:'偏慢',value:report.value.counts.slow,tone:'warn'},
 {key:'unstable',label:'不稳定',value:report.value.counts.unstable,tone:'warn'},
 {key:'no-response',label:'无响应',value:report.value.counts.noResponse,tone:'bad'},
 {key:'partial',label:'部分站点不通',value:report.value.counts.partial,tone:'warn'}
])

const resultSegments=computed(()=>[...countCards.value,{key:'skipped',label:'未执行',value:report.value.counts.skipped,tone:'empty'}].filter(c=>c.value>0))

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
 return row.status==='partial'?partialLatencyLabel(row.partialSites):({usable:'能用',slow:'偏慢',unstable:'不稳定',partial:'部分站点不通', 'no-response':'无响应',skipped:'未执行'} as const)[row.status]
}
function issueDetail(group:LatencyReportGroup,row:LatencyReportRow){
 return group.issues.find(issue=>issue.row.key===row.key)?.detail||row.skipReason||row.suspectedReasons.join('；')
}
function sharedIssueDetail(group:LatencyReportGroup){
 if(group.kind==='slow')return latencySlowThresholdNote
 const first=group.rows[0]&&issueDetail(group,group.rows[0])
 return first&&group.rows.every(row=>issueDetail(group,row)===first)?first:''
}
function uniqueItems(rows:LatencyReportRow[]){
 const unique=new Map<string,BatchItem>()
 for(const row of rows)unique.set(row.key,row.item)
 return [...unique.values()]
}
const detailFilters=computed(()=>[
 {key:'all' as const,label:'全部',value:report.value.rows.length},
 ...countCards.value.filter(c=>c.value>0),
 ...(report.value.counts.skipped?[{key:'skipped' as const,label:'未执行',value:report.value.counts.skipped,tone:'empty'}]:[])
])
const filteredRows=computed(()=>{
 const q=detailSearch.value.trim().toLowerCase()
 return report.value.rows.filter(row=>{
  if(detailStatus.value!=='all'&&row.status!==detailStatus.value)return false
  if(!q)return true
  return (nodeDisplayName(row.item)+' '+row.airportName+' '+row.nodeType+' '+statusLabel(row)).toLowerCase().includes(q)
 })
})
const detailPageCount=computed(()=>Math.max(1,Math.ceil(filteredRows.value.length/detailPageSize)))
watch(detailPageCount,count=>{if(detailPage.value>count)detailPage.value=count})
const pagedRows=computed(()=>filteredRows.value.slice((detailPage.value-1)*detailPageSize,detailPage.value*detailPageSize))
function shortSiteName(name:string){return name==='Cloudflare'?'CF':name}
</script>

<template>
 <section class="latency-report" aria-label="延迟检测报告">
  <header class="latency-report-head">
   <div>
    <p class="latency-report-kicker">{{props.monitor?'持续监测':'本轮检测'}}</p>
    <h2 ref="reportTitle" tabindex="-1" autofocus>{{props.monitor?'持续监测报告':'延迟检测报告'}}</h2>
    <p class="latency-report-meta">{{summaryLine}}</p>
   </div>
   <span class="latency-report-state">{{stateLabel(props.batch.state)}}</span>
  </header>

  <nav class="report-tabs" role="tablist" aria-label="延迟报告视图">
   <button type="button" class="report-tab" role="tab" :aria-selected="reportView==='overview'" @click="reportView='overview'">结果概览</button>
   <button type="button" class="report-tab" role="tab" :aria-selected="reportView==='details'" @click="reportView='details'">逐个明细 <small>{{report.rows.length}}</small></button>
  </nav>

  <div v-show="reportView==='overview'" class="report-pane">
  <section class="latency-report-overview" aria-label="本轮结果分布">
   <div class="latency-report-verdict"><span>能用的节点</span><strong>{{report.counts.usable}}<small> / {{report.counts.total}}</small></strong><p>未执行与疑似检测问题单独说明</p></div>
   <div class="latency-report-distribution">
    <div v-if="report.counts.total" class="latency-report-distribution-bar" role="img" :aria-label="resultSegments.map(c=>c.label+' '+c.value+' 个').join('，')"><span v-for="segment in resultSegments" :key="segment.key" :class="segment.tone" :style="{width:segment.value/report.counts.total*100+'%'}" :title="segment.label+' '+segment.value+' 个'"/></div>
    <div class="latency-report-counts" aria-label="检测结果计数">
     <button v-for="card in countCards" :key="card.key" type="button" class="latency-report-count" :class="[card.tone,{clickable:card.value>0}]" :disabled="card.value===0" :title="card.value>0?`查看 ${card.label} 的 ${card.value} 个节点`:undefined" @click="openDetails(card.key)"><small>{{card.label}}</small><strong>{{card.value}}</strong></button>
    </div>
   </div>
  </section>
  <p v-if="report.counts.suspected" class="latency-report-note">疑似检测问题 {{report.counts.suspected}} 项（与上方分类重叠）</p>
  <p v-if="report.counts.skipped" class="latency-report-note">
   未执行 {{report.counts.skipped}} 项（不计入无响应）
  </p>

  <p class="latency-report-compare">{{comparisonLine}}</p>
  <p v-for="notice in httpNotices" :key="notice" class="latency-report-http">{{notice}}</p>

  <section v-if="airportSummary.length>1" class="airport-summary"><h3>按机场汇总</h3><div class="airport-summary-scroll"><table><thead><tr><th>机场</th><th>节点数</th><th>能用</th><th>能用率</th><th>偏慢/不稳定</th><th>部分不通</th><th>无响应</th></tr></thead><tbody><tr v-for="a in airportSummary" :key="a.airport"><td>{{a.airport}}</td><td>{{a.total}}</td><td>{{a.usable}}</td><td>{{a.total?Math.round(a.usable/a.total*100):0}}%</td><td>{{a.slow}}</td><td>{{a.partial}}</td><td :class="{bad:a.noResponse>0}">{{a.noResponse}}</td></tr></tbody></table></div></section>
  <div class="latency-report-columns">
   <section class="latency-report-panel">
    <header><div><h3>最快 5 个</h3><small>按已测站点的稳定中位值排序</small></div></header>
    <ol v-if="report.fastest5.length" class="latency-report-fastest">
     <li v-for="(row,index) in report.fastest5" :key="row.key">
      <span class="latency-report-rank">{{index+1}}</span>
      <div class="latency-report-node"><button class="text-button" @click="emit('inspect',row.item)">{{nodeDisplayName(row.item)}}</button><small>{{row.airportName}}<template v-if="row.nodeType!=='未知协议'"> · {{row.nodeType}}</template></small></div>
      <strong><ReportMs :value="row.stableMs"/></strong>
     </li>
    </ol>
    <p v-else class="latency-report-empty">没有可计算的稳定延迟结果，需有首次之后的响应采样。</p>
   </section>

   <section class="latency-report-panel latency-report-problems">
    <header><div><h3>问题按原因</h3><small>节点可打开明细，问题可直接重测</small></div></header>
    <div v-if="report.groups.length" class="latency-report-groups">
     <article v-for="(group,index) in report.groups" :key="group.kind+'-'+group.reason+'-'+index" class="latency-report-group" :class="group.kind">
      <header>
       <div><h4>{{group.reason}} <span>· {{group.rows.length}} 个</span></h4><small v-if="sharedIssueDetail(group)&&sharedIssueDetail(group)!==group.reason">{{sharedIssueDetail(group)}}</small><small v-if="group.kind==='skipped'">未执行项目单列，不计为无响应</small></div>
       <button v-if="group.rows.length&&group.kind!=='skipped'" class="button ghost latency-report-retry" @click="emit('retry',uniqueItems(group.rows))">重测</button>
      </header>
      <p v-if="group.kind==='slow'&&props.entryNotice" class="latency-report-group-note">{{props.entryNotice}}</p>
      <ul>
       <li v-for="row in expandedGroups.has(index)||group.rows.length<=8?group.rows:group.rows.slice(0,6)" :key="row.key">
        <span class="report-group-node"><button class="text-button" @click="emit('inspect',row.item)">{{nodeDisplayName(row.item)}}</button><span v-if="row.airportName" class="report-airport">{{row.airportName}}</span></span>
        <small v-if="group.kind==='slow'||!sharedIssueDetail(group)&&issueDetail(group,row)!==group.reason">{{issueDetail(group,row)}}</small>
       </li>
      </ul><button v-if="group.rows.length>8" class="text-button" @click="toggleGroup(index)">{{expandedGroups.has(index)?'收起':'再看 '+(group.rows.length-6)+' 个'}}</button>
     </article>
    </div>
    <p v-else class="latency-report-empty">本轮没有需要处理的问题。</p>
   </section>
  </div>
  </div>

  <div v-show="reportView==='details'" class="report-pane latency-report-details">
   <div class="report-detail-toolbar">
    <div class="report-detail-filters" role="group" aria-label="按结果筛选">
     <button v-for="f in detailFilters" :key="f.key" type="button" class="report-filter-chip" :aria-pressed="detailStatus===f.key" @click="detailStatus=f.key">{{f.label}}<small>{{f.value}}</small></button>
    </div>
    <div class="report-detail-search">
     <input v-model="detailSearch" type="search" placeholder="搜节点、机场、协议" aria-label="筛选节点明细">
     <button v-if="detailStatus!=='all'||detailSearch" type="button" class="button ghost small" @click="detailStatus='all';detailSearch=''">重置</button>
    </div>
   </div>
   <div class="latency-report-table-wrap">
    <table>
     <thead><tr><th>节点</th><th>结果</th><th class="number">首次中位</th><th class="number">稳定中位</th><th>各站稳定延迟</th><th>说明</th></tr></thead>
     <tbody>
      <tr v-for="row in pagedRows" :key="row.key">
       <td><button class="text-button" @click="emit('inspect',row.item)">{{nodeDisplayName(row.item)}}</button><small>{{row.airportName}}<template v-if="row.nodeType!=='未知协议'"> · {{row.nodeType}}</template></small></td>
       <td><span class="latency-report-status" :class="row.status">{{statusLabel(row)}}</span><small v-if="row.suspected" class="latency-report-suspected">疑似检测问题</small></td>
       <td class="number"><ReportMs :value="row.firstMedianMs"/></td><td class="number"><ReportMs :value="row.stableMs"/></td>
       <td><div class="report-sites-grid"><span v-for="site in row.sites" :key="site.id" class="report-site-chip" :class="{bad:site.stableMs===null,slow:site.stableMs!==null&&site.stableMs>1500}" :title="`${site.name}: ${formatMs(site.stableMs)}`"><small>{{shortSiteName(site.name)}}</small><b>{{formatMs(site.stableMs)}}</b></span></div></td>
       <td><small>{{partialLatencyError(row.item.result)||row.suspectedReasons.join('；')||row.skipReason||'—'}}</small></td>
      </tr>
      <tr v-if="!pagedRows.length"><td colspan="6" class="report-table-empty">没有符合当前筛选条件的节点</td></tr>
     </tbody>
    </table>
   </div>
   <div v-if="detailPageCount>1" class="report-pagination">
    <span>共 {{filteredRows.length}} 项 · 第 {{detailPage}} / {{detailPageCount}} 页</span>
    <div class="report-pagination-btns"><button type="button" class="button ghost small" :disabled="detailPage<=1" @click="detailPage--">上一页</button><button type="button" class="button ghost small" :disabled="detailPage>=detailPageCount" @click="detailPage++">下一页</button></div>
   </div>
  </div>

  <footer class="latency-report-actions">
   <button class="button" :disabled="imageBusy" @click="exportImage">{{imageBusy?'正在导出…':'导出图片'}}</button>
   <button v-if="issueItems.length" class="button" @click="emit('retry',issueItems)">重测问题（{{issueItems.length}}）</button>
   <button class="button primary" :disabled="!report.usableItems.length" @click="emit('export',report.usableItems)">导出能用的 {{report.usableItems.length}} 个</button>
  </footer>

  <p v-if="imageMessage" :role="imageError?'alert':'status'">{{imageMessage}}</p>
  <Teleport to="body"><div v-if="imageBusy" ref="imageRoot" class="report-image-stage" aria-hidden="true"><LatencyReportImage :report="report" :summary="summaryLine" :comparison="comparisonLine" :monitor="monitor"/></div></Teleport>
 </section>
</template>

<style scoped>
.report-image-stage{position:fixed;left:-12000px;top:0;width:1000px;pointer-events:none}
.latency-report{display:flex;flex-direction:column;gap:14px;color:var(--cst-text)}
.latency-report-head{display:flex;align-items:flex-start;justify-content:space-between;gap:14px}
.latency-report-kicker{color:var(--cst-muted);font-size:11px;line-height:1.3;margin:0}
.latency-report-head h2{margin:3px 0 4px;font-size:19px}
.latency-report-meta{color:var(--cst-muted);font-size:12px;line-height:1.45;margin:0}
.latency-report-state{flex:none;border:1px solid var(--cst-line-strong);border-radius:999px;padding:4px 10px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:11px}
.report-tabs{display:flex;gap:6px;flex-wrap:wrap;padding-bottom:2px;border-bottom:1px solid var(--cst-line)}
.report-tab{display:inline-flex;align-items:center;gap:6px;padding:7px 14px;margin-bottom:-1px;border:1px solid transparent;border-bottom:2px solid transparent;border-radius:9px 9px 0 0;background:transparent;color:var(--cst-muted);font-size:13px;font-weight:550;cursor:pointer}
.report-tab small{padding:1px 6px;border-radius:999px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:11px;font-variant-numeric:tabular-nums}
.report-tab:hover{color:var(--cst-text)}
.report-tab[aria-selected="true"]{color:var(--cst-accent);border-bottom-color:var(--cst-accent);background:var(--cst-panel-2);font-weight:650}
.report-tab[aria-selected="true"] small{background:var(--cst-accent-soft);color:var(--cst-accent)}
.report-pane{display:flex;flex-direction:column;gap:12px}
.latency-report-overview{display:grid;grid-template-columns:195px minmax(0,1fr);gap:24px;padding:16px 18px;border:1px solid var(--cst-line-strong);border-radius:14px;background:var(--cst-panel-2);align-items:center}
.latency-report-verdict>span{font-size:12px;color:var(--cst-muted)}.latency-report-verdict>strong{display:block;margin:4px 0;font-size:38px;font-weight:600;line-height:1.1;font-variant-numeric:tabular-nums}.latency-report-verdict strong small{font-size:18px;color:var(--cst-muted);font-weight:400}.latency-report-verdict p{font-size:11px;color:var(--cst-muted);margin:6px 0 0}
.latency-report-distribution-bar{display:flex;gap:3px;height:10px;margin-bottom:14px;border-radius:999px;overflow:hidden;background:var(--cst-panel)}.latency-report-distribution-bar span.good{background:var(--cst-good)}.latency-report-distribution-bar span.warn{background:var(--cst-warn)}.latency-report-distribution-bar span.bad{background:var(--cst-bad)}.latency-report-distribution-bar span.empty{background:var(--cst-line-strong)}
.latency-report-counts{display:flex;gap:8px;flex-wrap:wrap}.latency-report-count{display:inline-flex;align-items:baseline;gap:7px;min-width:68px;padding:7px 11px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel);color:inherit;text-align:left}.latency-report-count.clickable{cursor:pointer}.latency-report-count.clickable:hover{border-color:var(--cst-accent);background:var(--cst-accent-soft)}.latency-report-count:disabled{opacity:.7;cursor:default}.latency-report-count small{color:var(--cst-muted);font-size:11px}.latency-report-count strong{font-size:16px;line-height:1.1;font-weight:600;font-variant-numeric:tabular-nums}.latency-report-count.good strong{color:var(--cst-good)}.latency-report-count.bad strong{color:var(--cst-bad)}.latency-report-count.warn strong{color:var(--cst-warn)}
.latency-report-note{color:var(--cst-muted);font-size:11px;margin:0}
.latency-report-compare{margin:0;padding:8px 12px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:12px;line-height:1.55}
.latency-report-http{margin:0;padding:8px 12px;border:1px solid var(--cst-warn-line);border-radius:10px;background:var(--cst-warn-bg);color:var(--cst-warn);font-size:12px;line-height:1.55}
.latency-report-columns{display:grid;grid-template-columns:minmax(220px,.85fr) minmax(0,1.15fr);gap:14px;align-items:start}
.latency-report-panel{min-width:0;border:1px solid var(--cst-line);border-radius:14px;background:var(--cst-panel);padding:14px 16px}
.latency-report-panel>header,.latency-report-group>header{display:flex;align-items:center;justify-content:space-between;gap:10px}
.latency-report-panel h3{font-size:14px;margin:0 0 2px}.latency-report-panel header small{color:var(--cst-muted);font-size:11px}
.latency-report-fastest{list-style:none;padding:0;margin:10px 0 0}
.latency-report-fastest li{display:flex;align-items:center;gap:10px;padding:10px 0;border-top:1px solid var(--cst-line)}
.latency-report-fastest li:first-child{border-top:0;padding-top:4px}
.latency-report-rank{display:inline-flex;align-items:center;justify-content:center;width:22px;height:22px;flex:none;border-radius:999px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:11px;font-weight:600;font-variant-numeric:tabular-nums}
.latency-report-node{display:flex;min-width:0;flex:1;flex-direction:column;align-items:flex-start;gap:2px}
.latency-report-node .text-button,.latency-report-group li .text-button,.latency-report-table-wrap .text-button{max-width:100%;white-space:normal;text-align:left}
.latency-report-node small{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--cst-muted);font-size:11px}
.latency-report-fastest li>strong{flex:none;font-size:13px;font-variant-numeric:tabular-nums}
.latency-report-empty{padding:12px 0 4px;color:var(--cst-muted);font-size:12px;margin:0}
.latency-report-groups{display:grid;grid-template-columns:minmax(0,1fr);gap:10px;margin-top:10px}
.latency-report-group{border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-2);padding:11px 13px}
.latency-report-group>header h4{margin:0;font-size:12.5px;font-weight:600}.latency-report-group>header h4 span{color:var(--cst-muted);font-weight:400}
.latency-report-group>header small{display:block;margin-top:2px;color:var(--cst-muted);font-size:11px}.latency-report-group-note{margin:7px 0 0;padding:6px 8px;border-radius:6px;background:var(--cst-warn-bg);border:1px solid var(--cst-warn-line);color:var(--cst-warn);font-size:11px;line-height:1.55}.latency-report-group ul{display:flex;flex-direction:column;gap:5px;list-style:none;padding:0;margin:8px 0 0}
.latency-report-group li{display:flex;align-items:baseline;justify-content:space-between;gap:8px;min-width:0;padding-top:5px;border-top:1px dashed var(--cst-line)}
.latency-report-group li:first-child{border-top:0;padding-top:0}
.report-group-node{display:inline-flex;align-items:baseline;gap:6px;flex-wrap:wrap;min-width:0}
.latency-report-group li small{min-width:0;color:var(--cst-muted);font-size:11px;text-align:right;white-space:nowrap}
.latency-report-group.suspected{border-color:color-mix(in srgb,var(--cst-accent) 35%,var(--cst-line))}
.latency-report-group.skipped{background:color-mix(in srgb,var(--cst-panel) 85%,var(--cst-hover))}
.latency-report-retry{min-height:26px;padding:3px 9px;font-size:11px;flex:none}
.latency-report-actions{display:flex;justify-content:flex-end;gap:8px;flex-wrap:wrap;padding-top:4px}
.report-detail-toolbar{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}
.report-detail-filters{display:flex;gap:6px;flex-wrap:wrap}
.report-filter-chip{display:inline-flex;align-items:center;gap:5px;padding:5px 10px;border:1px solid var(--cst-line);border-radius:999px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:12px;cursor:pointer}
.report-filter-chip small{font-size:11px;opacity:.8;font-variant-numeric:tabular-nums}
.report-filter-chip[aria-pressed="true"]{background:var(--cst-accent-soft);border-color:var(--cst-accent);color:var(--cst-accent);font-weight:600}
.report-detail-search{display:flex;align-items:center;gap:6px}
.report-detail-search input{width:190px;max-width:100%;padding:5px 10px;border:1px solid var(--cst-line);border-radius:8px;background:var(--cst-panel);color:var(--cst-text);font-size:12px}
.latency-report-table-wrap{overflow:auto;border:1px solid var(--cst-line);border-radius:12px;background:var(--cst-panel)}
.latency-report-table-wrap table{width:100%;min-width:700px;border-collapse:collapse;font-size:12px}
.latency-report-table-wrap th,.latency-report-table-wrap td{padding:9px 10px;border-bottom:1px solid var(--cst-line);text-align:left;vertical-align:top}
.latency-report-table-wrap tbody tr:last-child td{border-bottom:0}
.latency-report-table-wrap tbody tr:hover{background:var(--cst-panel-2)}
.latency-report-table-wrap th{position:sticky;top:0;background:var(--cst-panel-2);color:var(--cst-muted);font-weight:550;font-size:11px;white-space:nowrap;z-index:1}
.latency-report-table-wrap .number{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}
.latency-report-table-wrap td:first-child{min-width:165px}
.latency-report-table-wrap td:first-child small,.latency-report-table-wrap td:nth-child(2) small,.latency-report-table-wrap td:last-child small{display:block;max-width:220px;margin-top:3px;color:var(--cst-muted);font-size:11px;line-height:1.4}
.latency-report-status{display:inline-flex;align-items:center;padding:2px 8px;border-radius:999px;font-size:11px;font-weight:600;white-space:nowrap;border:1px solid var(--cst-line);background:var(--cst-panel-2)}
.latency-report-status.usable{color:var(--cst-good);background:color-mix(in srgb,var(--cst-good) 10%,var(--cst-panel));border-color:color-mix(in srgb,var(--cst-good) 28%,var(--cst-line))}
.latency-report-status.slow,.latency-report-status.unstable,.latency-report-status.partial{color:var(--cst-warn);background:var(--cst-warn-bg);border-color:var(--cst-warn-line)}
.latency-report-status.partial{white-space:normal}
.latency-report-status.no-response{color:var(--cst-bad);background:color-mix(in srgb,var(--cst-bad) 10%,var(--cst-panel));border-color:color-mix(in srgb,var(--cst-bad) 28%,var(--cst-line))}
.latency-report-status.incomplete,.latency-report-status.skipped{color:var(--cst-muted)}
.latency-report-note.partial{color:var(--cst-warn)}
.latency-report-suspected{color:var(--cst-accent)!important}
.report-sites-grid{display:flex;flex-wrap:wrap;gap:4px;max-width:290px}
.report-site-chip{display:inline-flex;align-items:center;gap:4px;padding:2px 6px;border-radius:6px;background:var(--cst-panel-2);border:1px solid var(--cst-line);font-size:11px;font-variant-numeric:tabular-nums}
.report-site-chip small{color:var(--cst-muted);font-size:10px}
.report-site-chip b{font-weight:550;color:var(--cst-text)}
.report-site-chip.slow{border-color:var(--cst-warn-line);background:var(--cst-warn-bg)}.report-site-chip.slow b{color:var(--cst-warn)}
.report-site-chip.bad{border-color:color-mix(in srgb,var(--cst-bad) 28%,var(--cst-line));background:color-mix(in srgb,var(--cst-bad) 8%,var(--cst-panel))}.report-site-chip.bad b{color:var(--cst-bad)}
.report-table-empty{text-align:center;color:var(--cst-muted);padding:20px!important}
.report-pagination{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap;font-size:12px;color:var(--cst-muted)}
.report-pagination-btns{display:flex;gap:6px}
@media(max-width:760px){.latency-report-overview{grid-template-columns:minmax(0,1fr);gap:14px}.latency-report-verdict>strong{font-size:32px}.latency-report-columns{grid-template-columns:minmax(0,1fr)}.latency-report-head{gap:8px}.latency-report-meta{max-width:58vw}.latency-report-group li{align-items:flex-start;flex-direction:column;gap:2px}.latency-report-group li small{text-align:left}.latency-report-actions{justify-content:stretch}.latency-report-actions .button{flex:1}}
.latency-report-problems{overflow:visible;max-height:none}.latency-report-groups,.latency-report-group,.latency-report-group ul{max-height:none;overflow:visible}.report-airport{display:inline-flex;padding:1px 6px;border-radius:999px;background:var(--cst-panel);border:1px solid var(--cst-line);font-size:10px;color:var(--cst-muted)}.airport-summary{border:1px solid var(--cst-line);border-radius:14px;background:var(--cst-panel);padding:12px 14px}.airport-summary-scroll{overflow:auto}.airport-summary h3{font-size:13px;margin:0 0 8px}.airport-summary table{min-width:540px;width:100%;border-collapse:collapse;font-size:12px}.airport-summary th,.airport-summary td{padding:7px;border-bottom:1px solid var(--cst-line);text-align:left}.airport-summary tbody tr:last-child td{border-bottom:0}
.latency-report-head h2:focus{outline:none}.latency-report-group{min-width:0}.latency-report-group li>span{min-width:0}
</style>
