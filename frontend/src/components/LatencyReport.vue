<script setup lang="ts">
import {computed,nextTick,ref,watch,onMounted} from 'vue'
import LatencyReportImage from './LatencyReportImage.vue'
import ReportMs from './ReportMs.vue'
import {saveReportImage} from '../reportImage'
import {nodeDisplayName} from '../nodePresentation'
import {stateLabel,type BatchItem,type LatencyBatch} from '../domain'
import {partialLatencyError} from '../inspectorLatency'
import {partialLatencyLabel} from '../latencyTargets'
import {buildLatencyReport,compareLatencyReports,latencyAirportSummary,latencyHttpNoticeDetails,latencySlowThresholdNote,type LatencyReportGroup,type LatencyReportRow,type LatencyReportSite,type LatencyReportStatus} from '../latencyReport'

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
const detailSubset=ref<{label:string;keys:Set<string>}|null>(null)
const detailSearch=ref('')
const detailPage=ref(1)
const detailPageSize=15
const expandedGroups=ref(new Set<number>())
const inspectedKey=ref<string|null>(null)
const inspectedPool=ref<string[]>([])

function toggleGroup(index:number){const next=new Set(expandedGroups.value);next.has(index)?next.delete(index):next.add(index);expandedGroups.value=next}
function openDetails(status:'all'|LatencyReportStatus='all'){inspectedKey.value=null;detailSubset.value=null;detailStatus.value=status;detailPage.value=1;reportView.value='details'}
function openSubset(label:string,keys:string[]){
 if(!keys.length)return
 inspectedKey.value=null
 detailStatus.value='all'
 detailSearch.value=''
 detailSubset.value={label,keys:new Set(keys)}
 detailPage.value=1
 reportView.value='details'
}
function openAirportDetails(airport:string){
 const keys=report.value.rows.filter(r=>r.airportName===airport).map(r=>r.key)
 openSubset(`机场 · ${airport}`,keys)
}
function openSuspectedDetails(){
 const keys=report.value.rows.filter(r=>r.suspected).map(r=>r.key)
 openSubset('疑似检测问题',keys)
}
function openNodeDetail(row:LatencyReportRow,pool?:LatencyReportRow[]){
 inspectedKey.value=row.key
 inspectedPool.value=(pool&&pool.length?pool:filteredRows.value.length?filteredRows.value:report.value.rows).map(r=>r.key)
}
function closeNodeDetail(){inspectedKey.value=null}
function clearDetailFilters(){detailStatus.value='all';detailSubset.value=null;detailSearch.value=''}
watch(()=>props.batch.batch_id,()=>{inspectedKey.value=null;reportView.value='overview';clearDetailFilters();detailPage.value=1})
watch([detailStatus,detailSearch,detailSubset],()=>{detailPage.value=1})

const inspectedRow=computed(()=>inspectedKey.value?report.value.rows.find(r=>r.key===inspectedKey.value)||null:null)
const inspectedIndex=computed(()=>inspectedKey.value?inspectedPool.value.indexOf(inspectedKey.value):-1)
function stepInspected(delta:number){
 const nextKey=inspectedPool.value[inspectedIndex.value+delta]
 if(nextKey)inspectedKey.value=nextKey
}

const airportSummary=computed(()=>latencyAirportSummary(report.value))
const httpNoticeItems=computed(()=>latencyHttpNoticeDetails(report.value))
const comparison=computed(()=>compareLatencyReports(props.batch,report.value,props.previous))
const usablePercent=computed(()=>report.value.counts.total?Math.round(report.value.counts.usable/report.value.counts.total*1000)/10:0)
const usableTone=computed(()=>!report.value.counts.total?'empty':usablePercent.value>=80?'good':usablePercent.value>=50?'warn':'bad')
const stateTone=computed(()=>props.batch.state==='completed'?'good':props.batch.state.includes('fail')||props.batch.state.includes('issue')?'warn':'empty')
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

const resultSegments=computed(()=>[...countCards.value,{key:'skipped' as LatencyReportStatus,label:'未执行',value:report.value.counts.skipped,tone:'empty'}].filter(c=>c.value>0))

function formatTime(value?:string){
 if(!value)return '时间未知'
 const at=new Date(value)
 if(!Number.isFinite(at.getTime()))return '时间未知'
 return new Intl.DateTimeFormat('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hourCycle:'h23'}).format(at)
}
function formatHms(value?:string){
 if(!value)return ''
 const at=new Date(value)
 if(!Number.isFinite(at.getTime()))return ''
 return new Intl.DateTimeFormat('zh-CN',{timeZone:'Asia/Shanghai',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}).format(at)
}
function formatDuration(value:number|null){
 if(value===null||!Number.isFinite(value))return '未记录'
 const seconds=Math.max(0,Math.round(value/1000)),minutes=Math.floor(seconds/60),remainder=seconds%60
 return minutes?`${minutes} 分 ${remainder} 秒`:`${seconds} 秒`
}
function formatMs(value:number|null|undefined){return value===null||value===undefined||!Number.isFinite(value)?'—':`${Math.round(value)} ms`}
function formatSignedMs(value:number){const rounded=Math.round(value);return `${rounded>0?'+':''}${rounded} ms`}
function statusLabel(row:LatencyReportRow){
 return row.status==='partial'?partialLatencyLabel(row.partialSites):({usable:'能用',slow:'偏慢',unstable:'不稳定',partial:'部分站点不通','no-response':'无响应',skipped:'未执行'} as const)[row.status]
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
  if(detailSubset.value&&!detailSubset.value.keys.has(row.key))return false
  if(detailStatus.value!=='all'&&row.status!==detailStatus.value)return false
  if(!q)return true
  return (nodeDisplayName(row.item)+' '+row.airportName+' '+row.nodeType+' '+statusLabel(row)).toLowerCase().includes(q)
 })
})
const detailPageCount=computed(()=>Math.max(1,Math.ceil(filteredRows.value.length/detailPageSize)))
watch(detailPageCount,count=>{if(detailPage.value>count)detailPage.value=count})
const pagedRows=computed(()=>filteredRows.value.slice((detailPage.value-1)*detailPageSize,detailPage.value*detailPageSize))
function shortSiteName(name:string){return name==='Cloudflare'?'CF':name}
function siteChipClass(site:LatencyReportSite){
 if(!site.attemptedSamples)return 'empty'
 if(site.responseSamples===0)return 'bad'
 if(site.failCount>0)return 'warn'
 if(site.httpCount>0)return 'http-refused'
 if(site.stableMs!==null&&site.stableMs>1500)return 'slow'
 return ''
}
function siteChipTitle(site:LatencyReportSite){
 const parts=[`${site.name}: ${site.responseSamples}/${site.attemptedSamples} 响应`]
 if(site.stableMs!==null)parts.push(`稳定 ${Math.round(site.stableMs)} ms`)
 if(site.httpCodes.length)parts.push(`HTTP ${site.httpCodes.join('/')}（${site.httpCount}/${site.attemptedSamples} 次，已连通但被拒绝/限流）`)
 if(site.failCount)parts.push(`失败 ${site.failCount} 次（${site.failReasons.join(' / ')}）`)
 return parts.join(' · ')
}
function rowFailSummary(row:LatencyReportRow){
 const parts=row.sites.filter(s=>s.failCount>0).map(s=>`${shortSiteName(s.name)} ${s.failCount}次${s.failReasons[0]||'失败'}`)
 if(parts.length)return parts.join(' · ')
 return partialLatencyError(row.item.result)||row.suspectedReasons.join('；')||row.skipReason||''
}
function inspectedPeakMs(row:LatencyReportRow){
 return Math.max(1,...row.sites.flatMap(s=>s.samples.map(sp=>sp.latencyMs??0)))
}
function siteMaxMs(site:LatencyReportSite,fallback:number){
 return Math.max(1,fallback,...site.samples.map(sp=>sp.latencyMs??0))
}
function inspectedAlerts(row:LatencyReportRow){
 const alerts:Array<{tone:'warn'|'bad'|'info';title:string;detail:string}>=[]
 for(const note of row.httpNotes){
  const refused=['403','429'].includes(note.code)
  alerts.push({
   tone:'warn',
   title:`${note.siteName} 返回 HTTP ${note.code}（${note.count}/${note.total} 次）`,
   detail:refused?'代理已成功建立连接并测得响应时延，但目标站点拒绝了本次请求（常见于共享出口 IP 触发限流或风控）。':'代理已连通目标站点，但返回了非常规 HTTP 状态码。'
  })
 }
 for(const site of row.sites){
  if(!site.failCount)continue
  const failedSeqs=site.samples.map((sp,idx)=>sp.success?0:idx+1).filter(Boolean)
  alerts.push({
   tone:site.responseSamples===0?'bad':'warn',
   title:`${site.name} 失败 ${site.failCount}/${site.attemptedSamples} 次（${site.failReasons.join(' / ')}）`,
   detail:`发生在第 ${failedSeqs.join('、')} 次采样`
  })
 }
 if(row.skipReason){
  alerts.push({tone:'info',title:'未执行采样',detail:row.skipReason})
 }
 for(const reason of row.suspectedReasons){
  alerts.push({tone:'info',title:'疑似检测异常',detail:reason})
 }
 return alerts
}
</script>

<template>
 <section class="latency-report" aria-label="延迟检测报告">
  <template v-if="inspectedRow">
   <div class="report-node-detail" aria-label="本轮节点采样详情">
    <header class="report-detail-nav">
     <button type="button" class="button report-back-btn" @click="closeNodeDetail">← 返回报告</button>
     <div class="report-detail-nav-right">
      <div v-if="inspectedPool.length>1" class="report-step-group">
       <button type="button" class="button ghost small" :disabled="inspectedIndex<=0" @click="stepInspected(-1)">上一条</button>
       <span class="report-step-pos">{{inspectedIndex+1}} / {{inspectedPool.length}}</span>
       <button type="button" class="button ghost small" :disabled="inspectedIndex<0||inspectedIndex>=inspectedPool.length-1" @click="stepInspected(1)">下一条</button>
      </div>
      <button type="button" class="button small" @click="emit('retry',[inspectedRow.item])">重测此节点</button>
      <button type="button" class="button ghost small" @click="emit('inspect',inspectedRow.item)">去工作台看历史 ↗</button>
     </div>
    </header>

    <section class="report-node-hero">
     <div class="report-node-hero-head">
      <div>
       <h3>{{nodeDisplayName(inspectedRow.item)}}</h3>
       <p>{{inspectedRow.airportName}}<template v-if="inspectedRow.nodeType!=='未知协议'"> · {{inspectedRow.nodeType}}</template> · 本轮 {{formatTime(props.batch.requested_at)}}</p>
      </div>
      <span class="latency-report-status" :class="inspectedRow.status">{{statusLabel(inspectedRow)}}</span>
     </div>
     <div class="report-node-kpis">
      <div><small>本轮响应</small><strong>{{inspectedRow.responseSamples}} / {{inspectedRow.attemptedSamples}}</strong></div>
      <div><small>首次中位（含握手）</small><strong><ReportMs :value="inspectedRow.firstMedianMs"/></strong></div>
      <div><small>稳定中位（第 2–6 次）</small><strong><ReportMs :value="inspectedRow.stableMs"/></strong></div>
      <div><small>异常 / 限流</small><strong :class="inspectedAlerts(inspectedRow).length?'warn':'good'">{{inspectedAlerts(inspectedRow).length?`${inspectedAlerts(inspectedRow).length} 项需留意`:'全部直通'}}</strong></div>
     </div>
    </section>

    <section v-if="inspectedAlerts(inspectedRow).length" class="report-node-alerts" aria-label="本轮错误与异常说明">
     <article v-for="(alert,idx) in inspectedAlerts(inspectedRow)" :key="idx" class="report-node-alert" :class="alert.tone">
      <strong>{{alert.title}}</strong>
      <small>{{alert.detail}}</small>
     </article>
    </section>

    <section class="report-site-cards" aria-label="四站逐次采样明细">
     <article v-for="site in inspectedRow.sites" :key="site.id" class="report-site-card" :class="siteChipClass(site)">
      <header class="report-site-card-head">
       <div class="report-site-card-title">
        <h4>{{site.name}}</h4>
        <span class="report-site-count" :class="site.responseSamples===0?'bad':site.failCount>0?'warn':'good'">{{site.responseSamples}}/{{site.attemptedSamples}} 响应</span>
        <span v-for="code in site.httpCodes" :key="code" class="report-site-http-badge">HTTP {{code}} ×{{site.httpCount}}</span>
       </div>
       <span class="report-site-medians">首次 {{formatMs(site.firstMs)}} · 稳定 {{formatMs(site.stableMs)}}</span>
      </header>
      <ol v-if="site.samples.length" class="report-attempt-list">
       <li v-for="(sample,idx) in site.samples" :key="sample.seq+'-'+idx" :class="{failed:!sample.success,'http-refused':!!sample.httpStatus}">
        <span class="attempt-seq">#{{idx+1}}<em v-if="idx===0">首次</em></span>
        <time class="attempt-time">{{formatHms(sample.timestamp)||'—'}}</time>
        <div class="attempt-bar-wrap">
         <span v-if="sample.latencyMs!==null" class="attempt-bar"><i :class="sample.httpStatus?'warn':sample.latencyMs>1500?'warn':'good'" :style="{width:Math.max(8,Math.min(100,sample.latencyMs/siteMaxMs(site,inspectedPeakMs(inspectedRow))*100))+'%'}"/></span>
         <span v-else class="attempt-fail-pill">{{sample.reason||'失败'}}</span>
        </div>
        <strong class="attempt-ms" :class="sample.latencyMs===null?'bad':sample.httpStatus||sample.latencyMs>1500?'warn':'good'">{{sample.latencyMs!==null?`${Math.round(sample.latencyMs)} ms`:'—'}}</strong>
        <span class="attempt-note" :title="sample.error||''">
         <b v-if="sample.httpStatus" class="attempt-http-chip">HTTP {{sample.httpStatus}}</b>
         <template v-else-if="!sample.success">{{sample.error||sample.reason||'请求失败'}}</template>
         <template v-else>正常</template>
        </span>
       </li>
      </ol>
      <p v-else class="latency-report-empty">本轮未采集该站点。</p>
     </article>
    </section>
   </div>
  </template>

  <template v-else>
  <header class="latency-report-head">
   <div class="latency-report-head-main">
    <div class="latency-report-title-row">
     <h2 ref="reportTitle" tabindex="-1" autofocus>{{props.monitor?'持续监测概览':'本轮检测概览'}}</h2>
     <span class="latency-report-state" :class="stateTone"><i aria-hidden="true"/>{{stateLabel(props.batch.state)}}</span>
    </div>
    <p class="latency-report-meta">{{summaryLine}}</p>
   </div>
   <nav class="report-tabs" role="tablist" aria-label="延迟报告视图">
    <button type="button" class="report-tab" role="tab" :aria-selected="reportView==='overview'" @click="reportView='overview'">结果概览</button>
    <button type="button" class="report-tab" role="tab" :aria-selected="reportView==='details'" @click="reportView='details'">逐个明细 <small>{{report.rows.length}}</small></button>
   </nav>
  </header>

  <div v-show="reportView==='overview'" class="report-pane">
  <section class="latency-report-overview" aria-label="本轮结果分布">
   <button type="button" class="latency-report-verdict" :disabled="!report.counts.usable" :title="report.counts.usable?`查看能用的 ${report.counts.usable} 个节点`:undefined" @click="report.counts.usable&&openDetails('usable')">
    <span class="verdict-label">能用的节点 <b v-if="report.counts.total" class="verdict-pct" :class="usableTone">{{usablePercent}}%</b></span>
    <strong>{{report.counts.usable}}<small> / {{report.counts.total}}</small></strong>
    <p>点击下方任意分类或柱段可直达明细</p>
   </button>
   <div class="latency-report-distribution">
    <div v-if="report.counts.total" class="latency-report-distribution-bar" role="img" :aria-label="resultSegments.map(c=>c.label+' '+c.value+' 个').join('，')">
     <button v-for="segment in resultSegments" :key="segment.key" type="button" class="report-bar-seg" :class="segment.tone" :style="{width:segment.value/report.counts.total*100+'%'}" :title="`查看 ${segment.label}（${segment.value} 个）`" @click="openDetails(segment.key)"/>
    </div>
    <div class="latency-report-counts" aria-label="检测结果计数">
     <button v-for="card in countCards" :key="card.key" type="button" class="latency-report-count" :class="[card.tone,{clickable:card.value>0}]" :disabled="card.value===0" :title="card.value>0?`查看 ${card.label} 的 ${card.value} 个节点`:undefined" @click="openDetails(card.key)"><i aria-hidden="true"/><small>{{card.label}}</small><strong>{{card.value}}</strong></button>
    </div>
   </div>
  </section>

  <div v-if="report.counts.suspected||report.counts.skipped" class="latency-report-notes">
   <button v-if="report.counts.suspected" type="button" class="latency-report-note clickable" @click="openSuspectedDetails">疑似检测问题 {{report.counts.suspected}} 项（与上方分类重叠）<span>查看 ›</span></button>
   <button v-if="report.counts.skipped" type="button" class="latency-report-note clickable" @click="openDetails('skipped')">未执行 {{report.counts.skipped}} 项（不计入无响应）<span>查看 ›</span></button>
  </div>

  <div class="latency-report-compare">
   <template v-if="comparison.reason"><span>{{comparisonLine}}</span></template>
   <template v-else>
    <span>和上次同条件（{{comparison.comparableNodes}} 个可比节点）比：</span>
    <button v-if="comparison.newlyFailed>0" type="button" class="report-inline-chip bad" @click="openSubset('较上次新增失败',comparison.newlyFailedKeys||[])">新增失败 {{comparison.newlyFailed}} ›</button>
    <span v-else class="report-inline-muted">新增失败 0</span>
    <span class="report-dot">·</span>
    <button v-if="comparison.recovered>0" type="button" class="report-inline-chip good" @click="openSubset('较上次已恢复',comparison.recoveredKeys||[])">恢复 {{comparison.recovered}} ›</button>
    <span v-else class="report-inline-muted">恢复 0</span>
    <span class="report-dot">·</span>
    <span>{{comparison.stableMedianDeltaMs===null?'稳定中位差暂无数据':`稳定中位差 ${formatSignedMs(comparison.stableMedianDeltaMs)}`}}</span>
   </template>
  </div>

  <button v-for="notice in httpNoticeItems" :key="notice.text" type="button" class="latency-report-http" @click="openSubset(notice.label,notice.nodeKeys)">
   <span>{{notice.text}}</span>
   <span class="report-http-action">查看这 {{notice.nodeKeys.length}} 个节点 ›</span>
  </button>

  <section v-if="airportSummary.length>1" class="airport-summary">
   <header><h3>按机场汇总</h3><small>点击任意机场查看该机场明细</small></header>
   <div class="airport-summary-scroll">
    <table>
     <thead><tr><th>机场</th><th>节点数</th><th>能用</th><th>能用率</th><th>偏慢/不稳定</th><th>部分不通</th><th>无响应</th></tr></thead>
     <tbody>
      <tr v-for="a in airportSummary" :key="a.airport" class="clickable-row" @click="openAirportDetails(a.airport)">
       <td><strong>{{a.airport}}</strong></td><td>{{a.total}}</td><td class="good">{{a.usable}}</td><td>{{a.total?Math.round(a.usable/a.total*100):0}}%</td><td>{{a.slow}}</td><td>{{a.partial}}</td><td :class="{bad:a.noResponse>0}">{{a.noResponse}}</td>
      </tr>
     </tbody>
    </table>
   </div>
  </section>

  <div class="latency-report-columns">
   <section class="latency-report-panel">
    <header><div><h3>最快 5 个</h3><small>按已测站点稳定中位排序 · 点行查本轮逐次采样</small></div></header>
    <ol v-if="report.fastest5.length" class="latency-report-fastest">
     <li v-for="(row,index) in report.fastest5" :key="row.key" class="report-clickable-item" @click="openNodeDetail(row,report.fastest5)">
      <span class="latency-report-rank" :class="index===0?'top1':index<3?'top3':''">{{index+1}}</span>
      <div class="latency-report-node"><button type="button" class="text-button" @click.stop="openNodeDetail(row,report.fastest5)">{{nodeDisplayName(row.item)}}</button><small>{{row.airportName}}<template v-if="row.nodeType!=='未知协议'"> · {{row.nodeType}}</template></small></div>
      <strong class="report-fast-ms"><ReportMs :value="row.stableMs"/></strong>
      <span class="report-row-arrow" aria-hidden="true">›</span>
     </li>
    </ol>
    <p v-else class="latency-report-empty">没有可计算的稳定延迟结果，需有首次之后的响应采样。</p>
   </section>

   <section class="latency-report-panel latency-report-problems">
    <header><div><h3>问题按原因</h3><small>点标题筛选整组，点节点看本轮逐次报错详情</small></div></header>
    <div v-if="report.groups.length" class="latency-report-groups">
     <article v-for="(group,index) in report.groups" :key="group.kind+'-'+group.reason+'-'+index" class="latency-report-group" :class="group.kind">
      <header>
       <div class="report-group-title">
        <h4><button type="button" class="report-group-drill" :title="`在明细中查看这 ${group.rows.length} 个节点`" @click="openSubset(group.reason,group.rows.map(r=>r.key))">{{group.reason}} <span>· {{group.rows.length}} 个 ›</span></button></h4>
        <small v-if="sharedIssueDetail(group)&&sharedIssueDetail(group)!==group.reason">{{sharedIssueDetail(group)}}</small>
        <small v-if="group.kind==='skipped'">未执行项目单列，不计为无响应</small>
       </div>
       <button v-if="group.rows.length&&group.kind!=='skipped'" type="button" class="button latency-report-retry" @click="emit('retry',uniqueItems(group.rows))">重测</button>
      </header>
      <p v-if="group.kind==='slow'&&props.entryNotice" class="latency-report-group-note">{{props.entryNotice}}</p>
      <ul>
       <li v-for="row in expandedGroups.has(index)||group.rows.length<=8?group.rows:group.rows.slice(0,6)" :key="row.key" class="report-clickable-item" @click="openNodeDetail(row,group.rows)">
        <span class="report-group-node"><button type="button" class="text-button" @click.stop="openNodeDetail(row,group.rows)">{{nodeDisplayName(row.item)}}</button><span v-if="row.airportName" class="report-airport">{{row.airportName}}</span></span>
        <span class="report-group-tail"><small v-if="group.kind==='slow'||!sharedIssueDetail(group)&&issueDetail(group,row)!==group.reason">{{issueDetail(group,row)}}</small><span class="report-row-arrow" aria-hidden="true">›</span></span>
       </li>
      </ul>
      <button v-if="group.rows.length>8" type="button" class="text-button report-expand-btn" @click="toggleGroup(index)">{{expandedGroups.has(index)?'收起':'再看 '+(group.rows.length-6)+' 个'}}</button>
     </article>
    </div>
    <p v-else class="latency-report-empty">本轮没有需要处理的问题。</p>
   </section>
  </div>
  </div>

  <div v-show="reportView==='details'" class="report-pane latency-report-details">
   <div class="report-detail-toolbar">
    <div class="report-detail-filters" role="group" aria-label="按结果筛选">
     <button v-for="f in detailFilters" :key="f.key" type="button" class="report-filter-chip" :aria-pressed="!detailSubset&&detailStatus===f.key" @click="detailSubset=null;detailStatus=f.key">{{f.label}}<small>{{f.value}}</small></button>
     <button v-if="detailSubset" type="button" class="report-subset-pill" @click="detailSubset=null">范围：{{detailSubset.label}}（{{detailSubset.keys.size}}）<span>×</span></button>
    </div>
    <div class="report-detail-search">
     <input v-model="detailSearch" type="search" placeholder="搜节点、机场、协议" aria-label="筛选节点明细">
     <button v-if="detailStatus!=='all'||detailSearch||detailSubset" type="button" class="button ghost small" @click="clearDetailFilters">重置</button>
    </div>
   </div>
   <div class="latency-report-table-wrap">
    <table>
     <thead><tr><th>节点</th><th>结果</th><th class="number">首次中位</th><th class="number">稳定中位</th><th>各站稳定延迟</th><th>说明</th></tr></thead>
     <tbody>
      <tr v-for="row in pagedRows" :key="row.key" class="clickable-row" @click="openNodeDetail(row,filteredRows)">
       <td><button type="button" class="text-button" @click.stop="openNodeDetail(row,filteredRows)">{{nodeDisplayName(row.item)}}</button><small>{{row.airportName}}<template v-if="row.nodeType!=='未知协议'"> · {{row.nodeType}}</template></small></td>
       <td><span class="latency-report-status" :class="row.status">{{statusLabel(row)}}</span><small v-if="row.suspected" class="latency-report-suspected">疑似检测问题</small></td>
       <td class="number"><ReportMs :value="row.firstMedianMs"/></td><td class="number"><ReportMs :value="row.stableMs"/></td>
       <td>
        <div class="report-sites-grid">
         <span v-for="site in row.sites" :key="site.id" class="report-site-chip" :class="siteChipClass(site)" :title="siteChipTitle(site)">
          <small>{{shortSiteName(site.name)}}</small>
          <b>{{site.responseSamples===0&&site.attemptedSamples>0?(site.failReasons[0]||'失败'):formatMs(site.stableMs)}}</b>
          <em v-if="site.httpCodes.length" class="site-chip-http">{{site.httpCodes[0]}}</em>
          <em v-else-if="site.failCount>0&&site.responseSamples>0" class="site-chip-fail">{{site.responseSamples}}/{{site.attemptedSamples}}</em>
         </span>
        </div>
       </td>
       <td>
        <div class="report-detail-note-cell">
         <div class="report-note-stack">
          <span v-for="note in row.httpNotes" :key="note.siteId+note.code" class="report-http-pill">{{shortSiteName(note.siteName)}} {{note.code}} ({{note.count}}/{{note.total}})</span>
          <small v-if="rowFailSummary(row)">{{rowFailSummary(row)}}</small>
          <small v-else-if="!row.httpNotes.length">—</small>
         </div>
         <span class="report-row-arrow" aria-hidden="true">›</span>
        </div>
       </td>
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
  </template>

  <p v-if="imageMessage" :role="imageError?'alert':'status'">{{imageMessage}}</p>
  <Teleport to="body"><div v-if="imageBusy" ref="imageRoot" class="report-image-stage" aria-hidden="true"><LatencyReportImage :report="report" :summary="summaryLine" :comparison="comparisonLine" :monitor="monitor"/></div></Teleport>
 </section>
</template>

<style scoped>
.report-image-stage{position:fixed;left:-12000px;top:0;width:1000px;pointer-events:none}
.latency-report{display:flex;flex-direction:column;gap:14px;color:var(--cst-text)}
.latency-report-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}
.latency-report-head-main{display:flex;flex-direction:column;gap:4px;min-width:0}
.latency-report-title-row{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.latency-report-head h2{margin:0;font-size:15.5px;font-weight:750;letter-spacing:-.015em;color:var(--cst-text)}
.latency-report-head h2:focus{outline:none}
.latency-report-meta{color:var(--cst-muted);font-size:12px;line-height:1.45;margin:0}
.latency-report-state{display:inline-flex;align-items:center;gap:6px;flex:none;border:1px solid var(--cst-line-strong);border-radius:999px;padding:2.5px 10px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:11px;font-weight:650}
.latency-report-state i{width:6px;height:6px;border-radius:50%;background:currentColor;flex:none}
.latency-report-state.good{color:var(--cst-good);background:var(--cst-good-bg);border-color:color-mix(in srgb,var(--cst-good) 28%,transparent)}
.latency-report-state.warn{color:var(--cst-warn);background:var(--cst-warn-bg);border-color:color-mix(in srgb,var(--cst-warn) 32%,transparent)}

.report-tabs{display:inline-flex;align-items:center;gap:3px;padding:3px;border:1px solid var(--cst-line);border-radius:11px;background:var(--cst-panel-2);flex:none}
.report-tab{display:inline-flex;align-items:center;gap:6px;height:30px;padding:0 13px;border:1px solid transparent;border-radius:8px;background:transparent;color:var(--cst-muted);font-size:12.5px;font-weight:550;cursor:pointer;transition:background .14s,color .14s,border-color .14s,box-shadow .14s}
.report-tab small{padding:1px 6px;border-radius:999px;background:var(--cst-panel);color:var(--cst-muted);font-size:10.5px;font-weight:700;font-variant-numeric:tabular-nums}
.report-tab:hover{color:var(--cst-text)}
.report-tab[aria-selected="true"]{color:var(--cst-accent);background:var(--cst-panel-solid);border-color:color-mix(in srgb,var(--cst-accent) 26%,var(--cst-line));box-shadow:var(--cst-shadow-xs);font-weight:650}
.report-tab[aria-selected="true"] small{background:var(--cst-accent-soft);color:var(--cst-accent)}

.report-pane{display:flex;flex-direction:column;gap:12px}
.latency-report-overview{display:grid;grid-template-columns:208px minmax(0,1fr);gap:22px;padding:16px 18px;border:1px solid color-mix(in srgb,var(--cst-accent) 22%,var(--cst-line));border-radius:14px;background:var(--cst-aurora-surface),var(--cst-panel);box-shadow:var(--cst-shadow-xs);align-items:center}
.latency-report-verdict{display:flex;flex-direction:column;align-items:flex-start;padding:4px 8px 4px 2px;border:0;border-radius:10px;background:transparent;color:inherit;text-align:left;cursor:pointer;transition:background .14s}
.latency-report-verdict:hover:not(:disabled){background:color-mix(in srgb,var(--cst-accent) 6%,transparent)}
.latency-report-verdict:disabled{cursor:default}
.verdict-label{display:inline-flex;align-items:center;gap:7px;font-size:12px;color:var(--cst-muted);font-weight:550}
.verdict-pct{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;font-size:10.5px;font-weight:750;font-variant-numeric:tabular-nums}
.verdict-pct.good{background:var(--cst-good-bg);color:var(--cst-good)}
.verdict-pct.warn{background:var(--cst-warn-bg);color:var(--cst-warn)}
.verdict-pct.bad{background:var(--cst-bad-bg);color:var(--cst-bad)}
.latency-report-verdict>strong{display:block;margin:4px 0 2px;font-size:38px;font-weight:750;line-height:1.05;letter-spacing:-.03em;font-variant-numeric:tabular-nums;color:var(--cst-text)}
.latency-report-verdict strong small{font-size:17px;color:var(--cst-muted);font-weight:500}
.latency-report-verdict p{font-size:11px;color:var(--cst-muted);margin:5px 0 0}

.latency-report-distribution-bar{display:flex;gap:3px;height:11px;margin-bottom:13px;border-radius:999px;overflow:hidden;background:var(--cst-aurora-track);padding:1px}
.report-bar-seg{border:0;padding:0;height:100%;border-radius:999px;cursor:pointer;transition:filter .14s,transform .14s}
.report-bar-seg:hover{filter:brightness(1.1);transform:scaleY(1.18)}
.report-bar-seg.good{background:var(--cst-aurora-good);box-shadow:0 0 10px rgba(16,185,129,.3)}
.report-bar-seg.warn{background:var(--cst-aurora-warn)}
.report-bar-seg.bad{background:linear-gradient(90deg,#e11d48 0%,#fb7185 100%)}
.report-bar-seg.empty{background:var(--cst-line-strong)}

.latency-report-counts{display:flex;gap:8px;flex-wrap:wrap}
.latency-report-count{display:inline-flex;align-items:center;gap:6px;min-width:74px;padding:7px 11px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-solid);color:inherit;text-align:left;box-shadow:var(--cst-shadow-xs);transition:border-color .14s,background .14s,transform .14s,box-shadow .14s}
.latency-report-count i{width:6px;height:6px;border-radius:50%;flex:none;background:var(--cst-subtle)}
.latency-report-count.good i{background:var(--cst-good)}
.latency-report-count.warn i{background:var(--cst-warn)}
.latency-report-count.bad i{background:var(--cst-bad)}
.latency-report-count.clickable{cursor:pointer}
.latency-report-count.clickable:hover{border-color:var(--cst-accent);background:var(--cst-accent-soft);transform:translateY(-1px);box-shadow:var(--cst-shadow)}
.latency-report-count:disabled{opacity:.55;cursor:default;box-shadow:none}
.latency-report-count small{color:var(--cst-muted);font-size:11.5px;font-weight:550}
.latency-report-count strong{margin-left:auto;font-size:15px;line-height:1.1;font-weight:750;font-variant-numeric:tabular-nums}
.latency-report-count.good strong{color:var(--cst-good)}
.latency-report-count.bad strong{color:var(--cst-bad)}
.latency-report-count.warn strong{color:var(--cst-warn)}

.latency-report-notes{display:flex;flex-wrap:wrap;gap:8px}
.latency-report-note{display:inline-flex;align-items:center;gap:8px;padding:5px 11px;border:1px solid var(--cst-line);border-radius:999px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:11.5px;cursor:pointer;transition:border-color .14s,color .14s,background .14s}
.latency-report-note span{color:var(--cst-accent);font-weight:650}
.latency-report-note:hover{border-color:var(--cst-accent);background:var(--cst-accent-soft);color:var(--cst-text)}

.latency-report-compare{display:flex;align-items:center;flex-wrap:wrap;gap:6px;margin:0;padding:9px 13px;border:1px solid var(--cst-line);border-radius:11px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:12px;line-height:1.5}
.report-inline-chip{display:inline-flex;align-items:center;gap:3px;padding:1.5px 8px;border-radius:999px;border:1px solid transparent;font-size:11.5px;font-weight:650;cursor:pointer;transition:filter .14s}
.report-inline-chip.bad{background:var(--cst-bad-bg);color:var(--cst-bad);border-color:color-mix(in srgb,var(--cst-bad) 30%,transparent)}
.report-inline-chip.good{background:var(--cst-good-bg);color:var(--cst-good);border-color:color-mix(in srgb,var(--cst-good) 30%,transparent)}
.report-inline-chip:hover{filter:brightness(.94)}
.report-inline-muted{color:var(--cst-muted)}.report-dot{opacity:.5}

.latency-report-http{display:flex;align-items:center;justify-content:space-between;gap:12px;width:100%;margin:0;padding:9px 13px;border:1px solid color-mix(in srgb,var(--cst-warn) 34%,var(--cst-line));border-radius:11px;background:var(--cst-warn-bg);color:var(--cst-warn);font-size:12px;line-height:1.5;text-align:left;cursor:pointer;transition:border-color .14s,filter .14s,transform .14s}
.latency-report-http:hover{border-color:var(--cst-warn);filter:brightness(.98);transform:translateY(-1px)}
.report-http-action{flex:none;font-weight:700;white-space:nowrap;padding:2px 9px;border-radius:999px;background:color-mix(in srgb,var(--cst-warn) 14%,var(--cst-panel-solid))}

.latency-report-columns{display:grid;grid-template-columns:minmax(240px,.88fr) minmax(0,1.12fr);gap:14px;align-items:start}
.latency-report-panel{min-width:0;border:1px solid var(--cst-line);border-radius:14px;background:var(--cst-panel);padding:14px 16px;box-shadow:var(--cst-shadow-xs)}
.latency-report-panel>header,.latency-report-group>header{display:flex;align-items:center;justify-content:space-between;gap:10px}
.latency-report-panel h3{font-size:14px;font-weight:700;margin:0 0 2px}
.latency-report-panel header small{color:var(--cst-muted);font-size:11px}

.latency-report-fastest{list-style:none;padding:0;margin:10px 0 0;display:flex;flex-direction:column;gap:4px}
.latency-report-fastest li{display:flex;align-items:center;gap:10px;padding:8px 10px;border:1px solid transparent;border-radius:10px;background:var(--cst-panel-2);cursor:pointer;transition:border-color .14s,background .14s,transform .14s}
.latency-report-fastest li:hover{border-color:color-mix(in srgb,var(--cst-accent) 34%,var(--cst-line));background:color-mix(in srgb,var(--cst-accent) 6%,var(--cst-panel-solid));transform:translateX(2px)}
.latency-report-rank{display:inline-flex;align-items:center;justify-content:center;width:22px;height:22px;flex:none;border-radius:999px;background:var(--cst-panel-solid);border:1px solid var(--cst-line);color:var(--cst-muted);font-size:11px;font-weight:750;font-variant-numeric:tabular-nums}
.latency-report-rank.top1{background:var(--cst-aurora-primary);border-color:transparent;color:#fff;box-shadow:0 3px 10px -2px rgba(79,110,247,.5)}
.latency-report-rank.top3{background:var(--cst-accent-soft);border-color:color-mix(in srgb,var(--cst-accent) 26%,transparent);color:var(--cst-accent)}
.latency-report-node{display:flex;min-width:0;flex:1;flex-direction:column;align-items:flex-start;gap:2px}
.latency-report-node .text-button,.latency-report-group li .text-button,.latency-report-table-wrap .text-button{max-width:100%;white-space:normal;text-align:left;font-weight:600;color:var(--cst-text)}
.latency-report-fastest li:hover .text-button,.latency-report-group li:hover .text-button,.latency-report-table-wrap tbody tr.clickable-row:hover .text-button{color:var(--cst-accent)}
.latency-report-node small{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--cst-muted);font-size:11px}
.report-fast-ms{flex:none;padding:2px 8px;border-radius:999px;background:var(--cst-good-bg);color:var(--cst-good);font-size:12.5px;font-weight:700;font-variant-numeric:tabular-nums}
.report-row-arrow{flex:none;color:var(--cst-subtle);font-size:14px;line-height:1;transition:transform .14s,color .14s}
.report-clickable-item:hover .report-row-arrow,.clickable-row:hover .report-row-arrow{color:var(--cst-accent);transform:translateX(2px)}

.latency-report-empty{padding:12px 0 4px;color:var(--cst-muted);font-size:12px;margin:0}
.latency-report-groups{display:grid;grid-template-columns:minmax(0,1fr);gap:10px;margin-top:10px}
.latency-report-group{border:1px solid var(--cst-line);border-radius:11px;background:var(--cst-panel-2);padding:11px 12px;min-width:0}
.report-group-title{min-width:0}
.latency-report-group>header h4{margin:0;font-size:13px;font-weight:700}
.report-group-drill{display:inline-flex;align-items:center;gap:4px;padding:0;border:0;background:transparent;color:var(--cst-text);font:inherit;cursor:pointer;text-align:left}
.report-group-drill:hover{color:var(--cst-accent)}
.report-group-drill span{color:var(--cst-muted);font-weight:500;font-size:12px}
.report-group-drill:hover span{color:var(--cst-accent)}
.latency-report-group>header small{display:block;margin-top:2px;color:var(--cst-muted);font-size:11px}
.latency-report-group-note{margin:7px 0 0;padding:6px 8px;border-radius:7px;background:var(--cst-warn-bg);border:1px solid color-mix(in srgb,var(--cst-warn) 32%,transparent);color:var(--cst-warn);font-size:11px;line-height:1.55}
.latency-report-group ul{display:flex;flex-direction:column;gap:4px;list-style:none;padding:0;margin:8px 0 0}
.latency-report-group li{display:flex;align-items:center;justify-content:space-between;gap:8px;min-width:0;padding:6px 8px;border:1px solid transparent;border-radius:8px;background:var(--cst-panel-solid);cursor:pointer;transition:border-color .14s,background .14s}
.latency-report-group li:hover{border-color:color-mix(in srgb,var(--cst-accent) 34%,var(--cst-line));background:color-mix(in srgb,var(--cst-accent) 5%,var(--cst-panel-solid))}
.report-group-node{display:inline-flex;align-items:center;gap:6px;flex-wrap:wrap;min-width:0}
.report-group-tail{display:inline-flex;align-items:center;gap:6px;flex:none}
.latency-report-group li small{min-width:0;color:var(--cst-muted);font-size:11px;text-align:right;white-space:nowrap}
.latency-report-group.suspected{border-color:color-mix(in srgb,var(--cst-accent) 35%,var(--cst-line))}
.latency-report-group.skipped{background:color-mix(in srgb,var(--cst-panel) 85%,var(--cst-hover))}
.latency-report-retry{min-height:27px;padding:3px 10px;font-size:11.5px;flex:none;border-radius:8px}
.report-expand-btn{margin-top:7px;font-size:12px;font-weight:600}

.latency-report-actions{display:flex;justify-content:flex-end;gap:8px;flex-wrap:wrap;padding-top:6px;border-top:1px solid var(--cst-line)}
.report-detail-toolbar{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}
.report-detail-filters{display:flex;gap:6px;flex-wrap:wrap;align-items:center}
.report-filter-chip{display:inline-flex;align-items:center;gap:5px;padding:5px 11px;border:1px solid var(--cst-line);border-radius:999px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:12px;font-weight:550;cursor:pointer;transition:border-color .12s,background .12s,color .12s}
.report-filter-chip:hover{border-color:var(--cst-accent);color:var(--cst-text)}
.report-filter-chip small{font-size:11px;opacity:.85;font-variant-numeric:tabular-nums}
.report-filter-chip[aria-pressed="true"]{background:var(--cst-accent-soft);border-color:var(--cst-accent);color:var(--cst-accent);font-weight:650}
.report-subset-pill{display:inline-flex;align-items:center;gap:6px;padding:5px 11px;border:1px solid color-mix(in srgb,var(--cst-accent) 42%,transparent);border-radius:999px;background:var(--cst-aurora-surface),var(--cst-panel);color:var(--cst-accent);font-size:12px;font-weight:650;cursor:pointer}
.report-subset-pill span{font-size:14px;line-height:1}
.report-detail-search{display:flex;align-items:center;gap:6px}
.report-detail-search input{width:200px;max-width:100%;height:34px;padding:5px 11px;border:1px solid var(--cst-line);border-radius:9px;background:var(--cst-panel-2);color:var(--cst-text);font-size:12.5px}
.report-detail-search input:focus{outline:none;border-color:var(--cst-accent);background:var(--cst-panel);box-shadow:0 0 0 3px color-mix(in srgb,var(--cst-accent) 13%,transparent)}

.latency-report-table-wrap{overflow:auto;border:1px solid var(--cst-line);border-radius:12px;background:var(--cst-panel);box-shadow:var(--cst-shadow-xs)}
.latency-report-table-wrap table{width:100%;min-width:720px;border-collapse:collapse;font-size:12px}
.latency-report-table-wrap th,.latency-report-table-wrap td{padding:9px 11px;border-bottom:1px solid var(--cst-line);text-align:left;vertical-align:middle}
.latency-report-table-wrap tbody tr:last-child td{border-bottom:0}
.latency-report-table-wrap tbody tr.clickable-row{cursor:pointer;transition:background .12s}
.latency-report-table-wrap tbody tr.clickable-row:hover{background:color-mix(in srgb,var(--cst-accent) 5%,var(--cst-panel-2))}
.latency-report-table-wrap th{position:sticky;top:0;background:var(--cst-panel-2);color:var(--cst-muted);font-weight:600;font-size:11px;white-space:nowrap;z-index:1}
.latency-report-table-wrap .number{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}
.latency-report-table-wrap td:first-child{min-width:165px}
.latency-report-table-wrap td:first-child small,.latency-report-table-wrap td:nth-child(2) small{display:block;max-width:220px;margin-top:3px;color:var(--cst-muted);font-size:11px;line-height:1.4}
.report-detail-note-cell{display:flex;align-items:center;justify-content:space-between;gap:8px}
.report-note-stack{display:flex;flex-wrap:wrap;align-items:center;gap:4px 6px;min-width:0}
.report-note-stack small{color:var(--cst-muted);font-size:11px;line-height:1.4}
.report-http-pill{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;background:var(--cst-warn-bg);border:1px solid color-mix(in srgb,var(--cst-warn) 34%,transparent);color:var(--cst-warn);font-size:10.5px;font-weight:700;white-space:nowrap;font-variant-numeric:tabular-nums}
.latency-report-status{display:inline-flex;align-items:center;padding:2px 8px;border-radius:999px;font-size:11px;font-weight:650;white-space:nowrap;border:1px solid var(--cst-line);background:var(--cst-panel-2)}
.latency-report-status.usable{color:var(--cst-good);background:var(--cst-good-bg);border-color:color-mix(in srgb,var(--cst-good) 28%,transparent)}
.latency-report-status.slow,.latency-report-status.unstable,.latency-report-status.partial{color:var(--cst-warn);background:var(--cst-warn-bg);border-color:color-mix(in srgb,var(--cst-warn) 32%,transparent)}
.latency-report-status.partial{white-space:normal}
.latency-report-status.no-response{color:var(--cst-bad);background:var(--cst-bad-bg);border-color:color-mix(in srgb,var(--cst-bad) 28%,transparent)}
.latency-report-status.incomplete,.latency-report-status.skipped{color:var(--cst-muted)}
.latency-report-suspected{color:var(--cst-accent)!important}
.report-sites-grid{display:flex;flex-wrap:wrap;gap:4px;max-width:310px}
.report-site-chip{display:inline-flex;align-items:center;gap:4px;padding:2px 7px;border-radius:7px;background:var(--cst-panel-2);border:1px solid var(--cst-line);font-size:11px;font-variant-numeric:tabular-nums}
.report-site-chip small{color:var(--cst-muted);font-size:10px}
.report-site-chip b{font-weight:600;color:var(--cst-text)}
.report-site-chip em{font-style:normal;font-size:9.5px;font-weight:750;padding:0 4px;border-radius:999px;line-height:1.45}
.report-site-chip .site-chip-http{background:color-mix(in srgb,var(--cst-warn) 18%,var(--cst-panel-solid));color:var(--cst-warn)}
.report-site-chip .site-chip-fail{background:color-mix(in srgb,var(--cst-warn) 18%,var(--cst-panel-solid));color:var(--cst-warn)}
.report-site-chip.http-refused{border-color:color-mix(in srgb,var(--cst-warn) 40%,var(--cst-line));background:var(--cst-warn-bg)}
.report-site-chip.slow,.report-site-chip.warn{border-color:color-mix(in srgb,var(--cst-warn) 38%,transparent);background:var(--cst-warn-bg)}
.report-site-chip.slow b,.report-site-chip.warn b{color:var(--cst-warn)}
.report-site-chip.bad{border-color:color-mix(in srgb,var(--cst-bad) 32%,transparent);background:var(--cst-bad-bg)}
.report-site-chip.bad b{color:var(--cst-bad)}
.report-table-empty{text-align:center;color:var(--cst-muted);padding:20px!important}
.report-pagination{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap;font-size:12px;color:var(--cst-muted)}
.report-pagination-btns{display:flex;gap:6px}

/* 报告内嵌逐次采样详情视图 */
.report-node-detail{display:flex;flex-direction:column;gap:14px}
.report-detail-nav{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}
.report-back-btn{min-height:32px;padding:4px 12px;font-size:12.5px;font-weight:650}
.report-detail-nav-right{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.report-step-group{display:inline-flex;align-items:center;gap:4px;padding:2px 6px;border:1px solid var(--cst-line);border-radius:9px;background:var(--cst-panel-2)}
.report-step-pos{font-size:11.5px;color:var(--cst-muted);font-variant-numeric:tabular-nums;padding:0 4px}
.report-node-hero{display:flex;flex-direction:column;gap:12px;padding:15px 17px;border:1px solid color-mix(in srgb,var(--cst-accent) 22%,var(--cst-line));border-radius:14px;background:var(--cst-aurora-surface),var(--cst-panel);box-shadow:var(--cst-shadow-xs)}
.report-node-hero-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}
.report-node-hero-head h3{margin:0 0 3px;font-size:16px;font-weight:750;color:var(--cst-text)}
.report-node-hero-head p{margin:0;font-size:12px;color:var(--cst-muted)}
.report-node-kpis{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}
.report-node-kpis>div{padding:9px 11px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-solid);display:flex;flex-direction:column;gap:3px}
.report-node-kpis small{font-size:11px;color:var(--cst-muted)}
.report-node-kpis strong{font-size:15px;font-weight:750;font-variant-numeric:tabular-nums;color:var(--cst-text)}
.report-node-kpis strong.good{color:var(--cst-good)}.report-node-kpis strong.warn{color:var(--cst-warn)}
.report-node-alerts{display:grid;gap:8px}
.report-node-alert{display:flex;flex-direction:column;gap:2px;padding:9px 12px;border-radius:10px;border:1px solid var(--cst-line);background:var(--cst-panel-2);font-size:12px}
.report-node-alert.warn{background:var(--cst-warn-bg);border-color:color-mix(in srgb,var(--cst-warn) 34%,transparent);color:var(--cst-warn)}
.report-node-alert.bad{background:var(--cst-bad-bg);border-color:color-mix(in srgb,var(--cst-bad) 32%,transparent);color:var(--cst-bad)}
.report-node-alert.info{background:var(--cst-accent-soft);border-color:color-mix(in srgb,var(--cst-accent) 28%,transparent);color:var(--cst-accent)}
.report-node-alert strong{font-weight:700;font-size:12px}
.report-node-alert small{opacity:.9;font-size:11.5px;line-height:1.45;color:var(--cst-text)}
.report-site-cards{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}
.report-site-card{border:1px solid var(--cst-line);border-radius:12px;background:var(--cst-panel);padding:12px 14px;display:flex;flex-direction:column;gap:9px;box-shadow:var(--cst-shadow-xs)}
.report-site-card.bad{border-color:color-mix(in srgb,var(--cst-bad) 30%,var(--cst-line))}
.report-site-card.warn,.report-site-card.http-refused{border-color:color-mix(in srgb,var(--cst-warn) 34%,var(--cst-line))}
.report-site-card-head{display:flex;align-items:center;justify-content:space-between;gap:8px;flex-wrap:wrap;padding-bottom:8px;border-bottom:1px solid var(--cst-line)}
.report-site-card-title{display:inline-flex;align-items:center;gap:6px;flex-wrap:wrap}
.report-site-card-title h4{margin:0;font-size:13.5px;font-weight:750}
.report-site-count{padding:1px 7px;border-radius:999px;font-size:10.5px;font-weight:700;font-variant-numeric:tabular-nums}
.report-site-count.good{background:var(--cst-good-bg);color:var(--cst-good)}
.report-site-count.warn{background:var(--cst-warn-bg);color:var(--cst-warn)}
.report-site-count.bad{background:var(--cst-bad-bg);color:var(--cst-bad)}
.report-site-http-badge{padding:1px 7px;border-radius:999px;background:var(--cst-warn-bg);border:1px solid color-mix(in srgb,var(--cst-warn) 35%,transparent);color:var(--cst-warn);font-size:10.5px;font-weight:750}
.report-site-medians{font-size:11px;color:var(--cst-muted);font-variant-numeric:tabular-nums}
.report-attempt-list{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:5px}
.report-attempt-list li{display:grid;grid-template-columns:54px 56px minmax(64px,1fr) 56px minmax(68px,.9fr);align-items:center;gap:6px;padding:5px 7px;border-radius:8px;background:var(--cst-panel-2);font-size:11.5px;font-variant-numeric:tabular-nums}
.report-attempt-list li.failed{background:color-mix(in srgb,var(--cst-bad) 6%,var(--cst-panel-2))}
.report-attempt-list li.http-refused{background:color-mix(in srgb,var(--cst-warn) 7%,var(--cst-panel-2))}
.attempt-seq{color:var(--cst-muted);font-weight:650;white-space:nowrap}
.attempt-seq em{font-style:normal;margin-left:3px;padding:0 4px;border-radius:4px;background:var(--cst-panel-solid);border:1px solid var(--cst-line);font-size:9.5px;font-weight:600}
.attempt-time{color:var(--cst-subtle);font-size:10.5px}
.attempt-bar-wrap{display:flex;align-items:center;min-width:0}
.attempt-bar{display:block;width:100%;height:6px;border-radius:999px;overflow:hidden;background:var(--cst-aurora-track)}
.attempt-bar i{display:block;height:100%;border-radius:inherit}
.attempt-bar i.good{background:var(--cst-aurora-bar)}
.attempt-bar i.warn{background:var(--cst-aurora-warn)}
.attempt-fail-pill{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;background:var(--cst-bad-bg);color:var(--cst-bad);font-size:10px;font-weight:700;white-space:nowrap}
.attempt-ms{text-align:right;font-weight:700;white-space:nowrap}
.attempt-ms.good{color:var(--cst-text)}.attempt-ms.warn{color:var(--cst-warn)}.attempt-ms.bad{color:var(--cst-bad)}
.attempt-note{text-align:right;color:var(--cst-muted);font-size:10.5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.attempt-http-chip{display:inline-flex;padding:1px 6px;border-radius:999px;background:var(--cst-warn-bg);color:var(--cst-warn);font-size:10px;font-weight:750}

.latency-report-problems{overflow:visible;max-height:none}
.latency-report-groups,.latency-report-group,.latency-report-group ul{max-height:none;overflow:visible}
.report-airport{display:inline-flex;padding:1px 7px;border-radius:999px;background:var(--cst-panel-2);border:1px solid var(--cst-line);font-size:10px;color:var(--cst-muted)}
.airport-summary{border:1px solid var(--cst-line);border-radius:14px;background:var(--cst-panel);padding:12px 14px;box-shadow:var(--cst-shadow-xs)}
.airport-summary header{display:flex;align-items:baseline;justify-content:space-between;gap:8px;margin:0 0 8px}
.airport-summary h3{font-size:13.5px;font-weight:700;margin:0}
.airport-summary header small{font-size:11px;color:var(--cst-muted)}
.airport-summary-scroll{overflow:auto}
.airport-summary table{min-width:540px;width:100%;border-collapse:collapse;font-size:12px}
.airport-summary th,.airport-summary td{padding:7px 8px;border-bottom:1px solid var(--cst-line);text-align:left;font-variant-numeric:tabular-nums}
.airport-summary tbody tr:last-child td{border-bottom:0}
.airport-summary tbody tr.clickable-row{cursor:pointer;transition:background .12s}
.airport-summary tbody tr.clickable-row:hover{background:var(--cst-accent-soft)}
.airport-summary td.good{color:var(--cst-good);font-weight:650}
.airport-summary td.bad{color:var(--cst-bad);font-weight:650}

@media(max-width:760px){
 .latency-report-overview{grid-template-columns:minmax(0,1fr);gap:14px}
 .latency-report-verdict>strong{font-size:32px}
 .latency-report-columns,.report-site-cards{grid-template-columns:minmax(0,1fr)}
 .report-node-kpis{grid-template-columns:repeat(2,minmax(0,1fr))}
 .latency-report-head{gap:8px}
 .latency-report-group li{align-items:flex-start;flex-direction:column;gap:3px}
 .report-group-tail{width:100%;justify-content:space-between}
 .latency-report-actions{justify-content:stretch}
 .latency-report-actions .button{flex:1}
}
</style>
