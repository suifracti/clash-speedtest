<script setup lang="ts">
import {computed,onBeforeUnmount,ref,watch} from 'vue'
import {api,type ClashCompareResult,type ClashNodeStatus} from '../../api'
import {manualLatencyPoint} from '../../presentation'
import {date,key,type NodeOption,type LatencyTest} from '../../domain'
import {inspectorMedian as median,inspectorSamples as samples} from '../../inspectorLatency'
import {clashSampleVerdict} from '../../clashComparison'
import {latencyRounds,latencyHistorySplit,readLatencyHistoryWithConfig,roundSiteDetails,type LatencySeries,type LatencyConfigChange,type RoundSiteDetail} from '../../latencyHistory'
import {hourBuckets,hourConclusion,hourTitle,slowestHours} from '../../historyPulse'
import {hideHoverTip,showHoverTip} from '../../hoverTip'
import LatencyChart from './LatencyChart.vue'
const props=defineProps<{series:LatencySeries[];node?:NodeOption;focus?:{id:string;test:LatencyTest}}>()
const emit=defineEmits<{(e:'round-change',series:LatencySeries[]):void;(e:'return-latest'):void}>()
const hours=ref(168),saved=ref<LatencySeries[]|null>(null),changes=ref<LatencyConfigChange[]>([]),loading=ref(false),error=ref(''),selectedId=ref(''),expandedId=ref(''),expanded=ref(false)
const clashStatus=ref<ClashNodeStatus|null>(null),clashStatusLoading=ref(false),clashStatusError=ref(''),clashComparison=ref<ClashCompareResult|null>(null),clashCompareLoading=ref(false),clashCompareError=ref('')
watch(()=>props.focus,focus=>selectedId.value=focus?.id||'',{immediate:true})
let generation=0
let clashGeneration=0
watch(()=>props.node?`${key(props.node)}\u0000${props.node.node_key}`:'',async()=>{
 const token=++clashGeneration,node=props.node
 clashStatus.value=null;clashComparison.value=null;clashStatusError.value='';clashCompareError.value='';clashCompareLoading.value=false
 if(!node){clashStatusLoading.value=false;return}
 clashStatusLoading.value=true
 try{const result=await api.clashNodeStatus(node);if(token===clashGeneration)clashStatus.value=result}
 catch(e){if(token===clashGeneration)clashStatusError.value=e instanceof Error?e.message:String(e)}
 finally{if(token===clashGeneration)clashStatusLoading.value=false}
},{immediate:true,flush:'sync'})
async function compareWithClash(){
 const node=props.node,token=clashGeneration
 if(!node||!clashStatus.value?.loaded||clashCompareLoading.value)return
 clashComparison.value=null;clashCompareError.value='';clashCompareLoading.value=true
 try{const result=await api.compareClashNode(node);if(token===clashGeneration)clashComparison.value=result}
 catch(e){if(token===clashGeneration)clashCompareError.value=e instanceof Error?e.message:String(e)}
 finally{if(token===clashGeneration)clashCompareLoading.value=false}
}
// Read the widest range once and filter for the selector locally, so switching
// 24 hours / 7 days / 30 days never hits the history API again.
watch([()=>props.node&&key(props.node),()=>props.series.flatMap(s=>s.points.map(p=>p.id)).join('|')],async()=>{if(!props.node)return;const token=++generation;loading.value=true;error.value='';try{const result=await readLatencyHistoryWithConfig(props.node,720);if(token===generation){saved.value=result.series;changes.value=result.changes}}catch(e){if(token===generation)error.value=e instanceof Error?e.message:String(e)}finally{if(token===generation)loading.value=false}},{immediate:true})
const historySeries=computed(()=>{const since=Date.now()-hours.value*3600000;return (saved.value||props.series).map(s=>({...s,points:s.points.filter(p=>Date.parse(p.time)>=since)}))})
const visibleChanges=computed(()=>changes.value.filter(c=>Date.parse(c.time)>=Date.now()-hours.value*3600000))
const combined=computed(()=>props.series.map(s=>({...s,points:[...new Map([...(saved.value?.find(v=>v.id===s.id)?.points||[]),...s.points.flatMap(p=>p.latencyTests?.length?p.latencyTests.map(t=>({...manualLatencyPoint(t,s.id),...(p.oldConfig?{oldConfig:true}:{})})):[p]),...(props.focus?[{...manualLatencyPoint(props.focus.test,s.id),id:props.focus.id,source:props.focus.test.source==='monitor'?'monitor' as const:'manual' as const}]:[])].map(p=>[p.id,p])).values()]})))
const rounds=computed(()=>latencyRounds(combined.value)),latest=computed(()=>rounds.value.at(-1)),current=computed(()=>rounds.value.find(r=>r.id===selectedId.value)||latest.value)
const currentSeries=computed(()=>current.value?.series||props.series.map(s=>({...s,points:[]})))
watch(currentSeries,value=>emit('round-change',value),{immediate:true})
const colors:Record<string,string>={cloudflare:'#378ADD',google:'#1D9E75',github:'#7F77DD',apple:'#D85A30'}
const data=computed(()=>currentSeries.value.map(s=>{const list=s.points[0]?samples(s.points[0],s.id):[],values=list.flatMap(p=>p.value===null?[]:[p.value]);return {...s,color:colors[s.id],values,stable:median(list.slice(1).flatMap(p=>p.value===null?[]:[p.value])),first:list[0]?.value??null,responded:values.length,total:list.length}}))
const historical=computed(()=>!!current.value&&current.value.id!==latest.value?.id)
const historySplit=computed(()=>latencyHistorySplit(historySeries.value)),stats=computed(()=>historySplit.value.all),currentStats=computed(()=>historySplit.value.current),configChanged=computed(()=>changes.value.length>0||stats.value.total>currentStats.value.total),listed=computed(()=>stats.value.rounds.slice().reverse().slice(0,expanded.value?undefined:10))
const siteDetails=computed(()=>new Map(stats.value.rounds.map(r=>[r.id,roundSiteDetails(r)])))
const hourReady=computed(()=>stats.value.total>=20)
function sitesFor(round:{id:string}){return siteDetails.value.get(round.id)||[]}
function toggleRound(id:string){selectedId.value=id;expandedId.value=expandedId.value===id?'':id}
function siteToneText(tone:RoundSiteDetail['tone']){return tone==='good'?'全部响应':tone==='warn'?'部分响应':tone==='bad'?'无响应':'未测'}
// Saved errors start with the probe URL and the node's own address; the tooltip
// only needs the cause, so drop that prefix and keep the head and tail readable.
function shortReason(reason:string){
 const at=reason.lastIndexOf('connect error: '),tail=(at>=0?reason.slice(at+'connect error: '.length):reason).replace(/\s+/g,' ').trim()
 return tail.length>72?tail.slice(0,44)+' … '+tail.slice(-24):tail
}
function showSiteTip(event:MouseEvent,time:string,site:RoundSiteDetail){showHoverTip(event,`${date(time)} · ${site.name} ${siteToneText(site.tone)} · ${site.responses}/${site.total} 次成功${site.reason?' · '+shortReason(site.reason):''}`)}
onBeforeUnmount(hideHoverTip)
const hourData=computed(()=>hourBuckets(stats.value.rounds)),bandNote=computed(()=>hourConclusion(stats.value.rounds)),slowest=computed(()=>slowestHours(hourData.value))
const heatRange=computed(()=>{const values=hourData.value.flatMap(b=>b.median===null?[]:[b.median]);return {min:Math.min(...values),max:Math.max(...values),count:values.length}})
function heat(value:number|null){
 if(value===null)return 'var(--hd-empty-bg)'
 const {min,max,count}=heatRange.value
 if(!count||max<=min)return 'color-mix(in srgb,var(--hd-accent) 40%,var(--hd-empty-bg))'
 const t=(value-min)/(max-min)
 return `color-mix(in srgb,var(--hd-accent) ${Math.round(18+t*52)}%,var(--hd-empty-bg))`
}
const comparisonHost=computed(()=>{try{return new URL(clashComparison.value?.target||'').hostname}catch{return ''}})
const comparisonVerdict=computed(()=>{
 const r=clashComparison.value
 if(!r)return {text:'',tone:'empty'}
 return clashSampleVerdict(r.cst.map(s=>s.success),r.clash.map(s=>s.success))
})
const pct=(v:number)=>Math.min(100,Math.max(0,v/1500*100))+'%'
const ms=(v:number|null|undefined)=>v==null?'—':Math.round(v).toLocaleString()+' ms'
</script>
<template>
 <section class="ins-latency" aria-label="延迟统计与合并走势">
  <div v-if="historical" class="round-banner">正在看 {{date(current?.time)}} <button class="hd-link" @click="selectedId='';emit('return-latest')">回到最新</button></div>
  <header><strong>本次 · {{current?date(current.time):'暂无采样'}}</strong><small>每站 {{Math.max(0,...data.map(s=>s.total))}} 次</small></header>
  <div class="latency-metrics">
   <div><span>稳定延迟</span><strong title="各站去掉首次采样后的中位数，再取四站中位数">{{ms(current?.stable)}}</strong></div>
   <div><span>首次连接</span><strong class="first" title="各站第 1 次采样的中位数，包含建立连接">{{ms(current?.first)}}</strong></div>
   <div><span>响应</span><strong :class="current?.responses===current?.total&&current?.total?'responded':''">{{current?.total?current.responses+'/'+current.total:'—'}}</strong></div>
  </div>
  <LatencyChart :series="currentSeries"/>
  <div v-for="s in data" :key="s.id" class="hd-site latency-range" :title="s.total?`${s.responded}/${s.total} 次响应 · 首次连接 ${ms(s.first)}`:'未测'">
   <span class="hd-site-name">{{s.name}}</span><div class="range-track"><template v-if="s.values.length"><i :style="{left:pct(Math.min(...s.values)),right:pct(1500-Math.max(...s.values)),background:s.color}"/><b v-if="s.stable!==null" :style="{left:pct(s.stable),background:s.color}"/></template></div><span class="hd-site-value" :title="s.context">{{ms(s.stable)}}</span>
  </div>
  <div class="range-scale"><div class="range-axis"><span v-for="v in [0,500,1000,1500]" :key="v" :style="{left:pct(v)}">{{v}}{{v===1500?' ms':''}}</span></div></div>
  <section class="clash-comparison" aria-label="和 Clash 对照">
   <div class="clash-comparison-head"><span :title="clashStatus?.endpoint">Clash 内核{{clashStatusLoading?'连接中':clashStatus?.connected?'已连接':'未连接'}}</span><button class="hd-btn" :disabled="clashStatusLoading||!clashStatus?.loaded||clashCompareLoading" :title="clashStatus&&!clashStatus.loaded?'Clash 当前没有加载这个节点':undefined" @click="compareWithClash">{{clashCompareLoading?'正在对照…':'和 Clash 对照'}}</button></div>
   <p v-if="clashCompareError||clashStatusError" class="clash-status warn" role="alert">{{clashCompareError||clashStatusError}}</p>
   <div v-if="clashComparison" class="clash-comparison-body" aria-live="polite">
    <table class="clash-result-table"><thead><tr><th></th><th>第 1 次</th><th>第 2 次</th></tr></thead><tbody><tr v-for="side in ['cst','clash'] as const" :key="side"><th>{{side==='cst'?'CST':'Clash'}}</th><td v-for="(sample,index) in clashComparison[side]" :key="index" :class="sample.success?'good':'bad'" :title="sample.error">{{sample.success?ms(sample.latency_ms):sample.error||'失败'}}</td></tr></tbody></table>
    <p class="clash-verdict" :class="comparisonVerdict.tone">{{comparisonVerdict.text}}</p>
    <small class="clash-result-meta" :title="clashComparison.target">{{date(clashComparison.finished_at)}} 测 · {{comparisonHost}}</small>
   </div>
  </section>
  <section class="latency-history" aria-label="延迟历史">
   <header><strong>历史</strong><nav aria-label="延迟历史范围"><button v-for="[h,label] in [[24,'24 小时'],[168,'7 天'],[720,'30 天']]" :key="h" class="hd-link" :aria-pressed="hours===h" @click="hours=Number(h);expanded=false">{{label}}</button></nav></header>
   <p v-if="loading" class="history-note">读取历史中…</p><p v-if="error" class="bad small">历史读取失败：{{error}}</p>
   <p v-if="stats.total===1" class="history-note single-round">这段时间只测过 1 轮，多测几次才能看出稳定性</p>
   <template v-else-if="stats.total>=2">
    <div class="history-metrics"><div><span>响应率</span><strong>{{stats.responseRate===null?'—':stats.responseRate.toFixed(0)+'%'}} <small>（{{stats.responses}}/{{stats.samples}} 次）</small></strong><small>{{stats.total}} 轮中 {{stats.failedRounds}} 轮有失败</small></div><div><span>稳定延迟中位数</span><strong>{{ms(stats.median)}}</strong></div><div title="慢的时候比快的时候多多少（P90−P10）"><span>波动幅度</span><strong v-if="stats.volatility!==null">{{ms(stats.volatility)}}</strong><small v-else class="few-rounds">轮数太少，暂不计算</small></div><div><span>最近一次失败</span><strong class="last-failure">{{stats.lastFailure?date(stats.lastFailure.time)+' · '+stats.lastFailure.detail:'无失败记录'}}</strong></div></div>
    <p v-if="configChanged" class="config-note">{{currentStats.total?`当前配置：${currentStats.total} 轮 · 响应率 ${currentStats.responseRate===null?'—':currentStats.responseRate.toFixed(0)+'%'}`:'当前配置还没测过'}}</p>
    <p class="history-note">共 {{stats.total}} 轮</p><LatencyChart :series="historySeries" :changes="visibleChanges" history/>
    <section class="hour-panel" aria-label="时段分布">
     <header><strong>时段分布</strong><small>本地时间 · 每小时一格</small></header>
     <template v-if="hourReady && hourData.some(b=>b.rounds)">
      <div class="hour-strip" role="img" aria-label="最近范围内各小时的稳定延迟与失败率"><i v-for="b in hourData" :key="b.hour" :class="{empty:b.median===null,failed:(b.failureRate||0)>0}" :style="{background:b.median===null?'':heat(b.median)}" :title="hourTitle(b)"/></div>
      <div class="hour-ticks" aria-hidden="true"><span v-for="h in [0,3,6,9,12,15,18,21]" :key="h" :style="{gridColumn:h+1}">{{h}}</span></div>
     </template>
     <p v-else-if="!hourReady" class="history-note">满 20 轮后显示时段分布（当前 {{stats.total}} 轮）</p>
     <p v-else class="history-note">这个范围没有可用的轮次。</p>
     <p v-if="hourReady&&slowest" class="hour-slowest">{{slowest}}</p>
     <p v-if="hourReady&&bandNote" class="hour-note">{{bandNote}}</p>
     <p v-if="hourReady" class="history-note">颜色越深越慢，红边表示有失败；浅灰格是轮次不足 2 轮，不写数字；悬停看几点、稳定延迟、失败率和轮数；旧配置轮次也计入。</p>
    </section>
   </template>
   <div v-if="listed.length" class="round-list"><div class="round-columns"><span>时间</span><span>四站</span><span>稳定延迟</span><span>首次连接</span></div><div v-for="r in listed" :key="r.id" class="round-item"><button class="round-row" :aria-label="'查看轮次 '+date(r.time)" :aria-pressed="r.id===current?.id" :aria-expanded="expandedId===r.id" @click="toggleRound(r.id)"><time><span v-if="r.oldConfig||r.id===current?.id" class="round-tags"><small v-if="r.oldConfig" class="old-config">旧配置</small><small v-if="r.id===current?.id" class="current-round">当前</small></span>{{date(r.time)}}</time><span class="round-sites" aria-hidden="true"><i v-for="site in sitesFor(r)" :key="site.id" class="round-site" :class="site.tone" @mouseenter.stop="showSiteTip($event,r.time,site)" @mouseleave="hideHoverTip"/></span><span>{{ms(r.stable)}}</span><span>{{ms(r.first)}}</span></button><div v-if="expandedId===r.id" class="round-detail"><div v-for="site in sitesFor(r)" :key="site.id" class="round-detail-row"><span class="round-detail-site"><i :class="site.tone"/>{{site.name}}</span><span>稳定 {{ms(site.stable)}}</span><span>首次 {{ms(site.first)}}</span><span>响应 {{site.responses}}/{{site.total}}</span></div></div></div></div>
   <p v-else-if="!loading&&!error" class="history-note">这个范围没有延迟记录。</p><button v-if="stats.total>10" class="hd-link history-more" @click="expanded=!expanded">{{expanded?'收起':'查看更多轮次（'+(stats.total-10)+'）'}}</button>
  </section>
 </section>
</template>
<style scoped>
.ins-latency header{display:flex;align-items:baseline;justify-content:space-between;gap:8px}.ins-latency header strong{font-size:13px}.ins-latency header small{font-size:10px;color:var(--hd-muted)}.latency-metrics{display:grid;grid-template-columns:1fr 1fr .8fr;gap:10px;margin:12px 0 4px}.latency-metrics span{display:block;font-size:11px;color:var(--hd-muted);margin-bottom:4px}.latency-metrics strong{font-size:23px;line-height:1.25;font-weight:600;white-space:nowrap;letter-spacing:-.4px;font-variant-numeric:tabular-nums}.latency-metrics .first{color:#9a640d}.latency-metrics .responded{color:var(--green)}.latency-range.hd-site{display:grid;grid-template-columns:70px minmax(0,1fr) 62px;gap:8px;height:28px;margin:0}.latency-range .hd-site-name{font-size:12px;font-weight:500}.latency-range .hd-site-value{font-size:12px;text-align:right;font-weight:500;white-space:nowrap}.range-track{height:10px;position:relative;border-radius:5px;background:var(--hd-soft)}.range-track i{position:absolute;top:3px;height:4px;opacity:.35;border-radius:2px}.range-track b{position:absolute;top:0;width:10px;height:10px;transform:translateX(-50%);border-radius:50%}.range-scale{display:grid;grid-template-columns:70px minmax(0,1fr) 62px;gap:8px;margin-top:3px;font-size:9px;color:var(--hd-muted);white-space:nowrap}.range-axis{grid-column:2;position:relative;height:12px}.range-axis span{position:absolute;transform:translateX(-50%)}
.clash-comparison{margin:12px 0;padding:8px 10px;border:1px solid var(--hd-line);border-radius:7px;background:var(--hd-soft)}.clash-comparison-head{display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:11px;color:var(--hd-muted)}.clash-comparison-head .hd-btn{font-size:12px;padding:5px 10px}.clash-status{font-size:10px;margin:6px 0;color:var(--hd-warn)}.clash-comparison-body{margin-top:6px}.clash-result-table{width:100%;table-layout:fixed;border-collapse:collapse;font-size:12px;font-variant-numeric:tabular-nums}.clash-result-table th,.clash-result-table td{padding:4px;text-align:right;overflow-wrap:anywhere}.clash-result-table th{font-weight:500;font-size:10px;color:var(--hd-muted)}.clash-result-table th:first-child{text-align:left;width:40px}.clash-result-table .good,.clash-verdict.good{color:var(--hd-good)}.clash-result-table .bad,.clash-verdict.bad{color:var(--hd-bad)}.clash-verdict{font-size:11px;margin:5px 0}.clash-verdict.warn{color:var(--hd-warn)}.clash-verdict.muted,.clash-result-meta{color:var(--hd-muted)}.clash-result-meta{font-size:10px}
.latency-history{margin-top:18px;padding-top:16px;border-top:1px solid var(--hd-line)}.latency-history nav{display:flex;gap:3px}.latency-history nav button{padding:3px 7px;font-size:11px;border-radius:5px}.latency-history nav button[aria-pressed=true]{background:var(--hd-soft);color:var(--hd-accent);font-weight:600}.history-metrics{display:grid;grid-template-columns:1fr 1fr;gap:14px 12px;padding:14px 0 4px}.history-metrics span,.history-metrics small{display:block;font-size:10px;color:var(--hd-muted)}.history-metrics strong{display:block;margin:4px 0;font-size:18px;font-weight:600;font-variant-numeric:tabular-nums}.history-metrics .last-failure{font-size:12px;line-height:1.5}.history-metrics strong small{display:inline;font-size:10px;font-weight:400}.few-rounds{margin-top:7px;line-height:1.5}.single-round{padding:12px 0;line-height:1.6}.current-round{display:inline-block;padding:1px 3px;margin-right:3px;border-radius:3px;background:var(--hd-soft);color:var(--hd-accent);font-size:9px}.old-config{display:inline-block;padding:1px 3px;margin-right:3px;border-radius:3px;background:var(--hd-soft);color:var(--hd-muted);font-size:9px;font-weight:600}.round-tags{display:flex;gap:2px;margin-bottom:1px}.round-row time{white-space:nowrap}.round-row .warn{color:var(--hd-warn)}.history-note{font-size:10px;color:var(--hd-muted);margin:8px 0}.config-note{font-size:10px;color:var(--hd-muted);margin:0 0 6px}.round-list{font-size:10px}.round-item{border-top:1px solid var(--hd-line)}.round-columns,.round-row{display:grid;grid-template-columns:1.1fr .6fr .82fr .82fr;gap:6px;align-items:center}.round-columns{padding:6px;color:var(--hd-muted)}.round-row{width:100%;padding:8px 6px;border:0;color:var(--hd-text);background:transparent;text-align:left;font-variant-numeric:tabular-nums;cursor:pointer}.round-row:hover,.round-row[aria-pressed=true]{background:var(--hd-soft);color:var(--hd-accent)}.round-sites{display:flex;gap:3px;align-items:center}.round-site{display:block;width:9px;height:9px;border-radius:2px;background:var(--hd-empty-bg)}.round-site.good{background:var(--hd-good)}.round-site.warn{background:var(--hd-warn)}.round-site.bad{background:var(--hd-bad)}.round-detail{padding:2px 6px 8px;background:color-mix(in srgb,var(--hd-soft) 55%,transparent)}.round-detail-row{display:grid;grid-template-columns:1.15fr .9fr .9fr .55fr;gap:6px;align-items:center;padding:4px 0;font-size:10px;color:var(--hd-muted);font-variant-numeric:tabular-nums}.round-detail-site{display:flex;align-items:center;gap:5px;color:var(--hd-text)}.round-detail-site i{display:block;width:7px;height:7px;border-radius:2px;background:var(--hd-empty-bg)}.round-detail-site i.good{background:var(--hd-good)}.round-detail-site i.warn{background:var(--hd-warn)}.round-detail-site i.bad{background:var(--hd-bad)}.history-more{margin-top:8px;font-size:11px}.round-banner{display:flex;align-items:center;justify-content:space-between;gap:6px;font-size:11px;color:var(--hd-accent);background:var(--hd-soft);padding:8px;margin-bottom:12px;border-radius:6px}
.hour-panel{margin-top:16px;padding-top:12px;border-top:1px dashed var(--hd-line)}.hour-panel header{display:flex;align-items:baseline;justify-content:space-between;gap:8px}.hour-panel header strong{font-size:12px}.hour-panel header small{font-size:9px;color:var(--hd-muted)}.hour-strip{display:grid;grid-template-columns:repeat(24,minmax(0,1fr));gap:2px;height:18px;margin:10px 0 0}.hour-strip i{display:block;min-width:0;border:1px solid transparent;border-radius:3px;background:var(--hd-empty-bg);box-sizing:border-box}.hour-strip i.failed{border-color:var(--hd-bad)}.hour-ticks{display:grid;grid-template-columns:repeat(24,minmax(0,1fr));gap:2px;margin-top:3px;font-size:8px;line-height:1.2;color:var(--hd-muted);font-variant-numeric:tabular-nums}.hour-ticks span{white-space:nowrap}.hour-slowest{margin:8px 0 0;font-size:10px;line-height:1.5;color:var(--hd-text);font-variant-numeric:tabular-nums}.hour-note{margin:4px 0 0;font-size:10px;line-height:1.6;color:var(--hd-accent);font-weight:600}
</style>
