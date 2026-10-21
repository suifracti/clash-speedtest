<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {manualLatencyPoint} from '../../presentation'
import {date,key,type NodeOption,type LatencyTest} from '../../domain'
import {inspectorMedian as median,inspectorSamples as samples} from '../../inspectorLatency'
import {latencyRounds,latencyHistoryStats,readLatencyHistory,type LatencySeries} from '../../latencyHistory'
import LatencyChart from './LatencyChart.vue'
const props=defineProps<{series:LatencySeries[];node?:NodeOption;focus?:{id:string;test:LatencyTest}}>()
const emit=defineEmits<{(e:'round-change',series:LatencySeries[]):void;(e:'return-latest'):void}>()
const hours=ref(168),saved=ref<LatencySeries[]|null>(null),loading=ref(false),error=ref(''),selectedId=ref(''),expanded=ref(false)
watch(()=>props.focus,focus=>selectedId.value=focus?.id||'',{immediate:true})
let generation=0
watch([()=>props.node&&key(props.node),hours,()=>props.series.flatMap(s=>s.points.map(p=>p.id)).join('|')],async()=>{if(!props.node)return;const token=++generation;loading.value=true;error.value='';try{const result=await readLatencyHistory(props.node,hours.value);if(token===generation)saved.value=result}catch(e){if(token===generation)error.value=e instanceof Error?e.message:String(e)}finally{if(token===generation)loading.value=false}},{immediate:true})
const historySeries=computed(()=>{const since=Date.now()-hours.value*3600000;return (saved.value||props.series).map(s=>({...s,points:s.points.filter(p=>Date.parse(p.time)>=since)}))})
const combined=computed(()=>props.series.map(s=>({...s,points:[...new Map([...(saved.value?.find(v=>v.id===s.id)?.points||[]),...s.points.flatMap(p=>p.latencyTests?.length?p.latencyTests.map(t=>manualLatencyPoint(t,s.id)):[p]),...(props.focus?[{...manualLatencyPoint(props.focus.test,s.id),id:props.focus.id,source:props.focus.test.source==='monitor'?'monitor' as const:'manual' as const}]:[])].map(p=>[p.id,p])).values()]})))
const rounds=computed(()=>latencyRounds(combined.value)),latest=computed(()=>rounds.value.at(-1)),current=computed(()=>rounds.value.find(r=>r.id===selectedId.value)||latest.value)
const currentSeries=computed(()=>current.value?.series||props.series.map(s=>({...s,points:[]})))
watch(currentSeries,value=>emit('round-change',value),{immediate:true})
const colors:Record<string,string>={cloudflare:'#378ADD',google:'#1D9E75',github:'#7F77DD',apple:'#D85A30'}
const data=computed(()=>currentSeries.value.map(s=>{const list=s.points[0]?samples(s.points[0],s.id):[],values=list.flatMap(p=>p.value===null?[]:[p.value]);return {...s,color:colors[s.id],values,stable:median(list.slice(1).flatMap(p=>p.value===null?[]:[p.value])),first:list[0]?.value??null,responded:values.length,total:list.length}}))
const historical=computed(()=>!!current.value&&current.value.id!==latest.value?.id)
const stats=computed(()=>latencyHistoryStats(historySeries.value)),listed=computed(()=>stats.value.rounds.slice().reverse().slice(0,expanded.value?undefined:10))
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
  <section class="latency-history" aria-label="延迟历史">
   <header><strong>历史</strong><nav aria-label="延迟历史范围"><button v-for="[h,label] in [[24,'24 小时'],[168,'7 天'],[720,'30 天']]" :key="h" class="hd-link" :aria-pressed="hours===h" @click="hours=Number(h);expanded=false">{{label}}</button></nav></header>
   <p v-if="loading" class="history-note">读取历史中…</p><p v-if="error" class="bad small">历史读取失败：{{error}}</p>
   <p v-if="stats.total===1" class="history-note single-round">这段时间只测过 1 轮，多测几次才能看出稳定性</p>
   <template v-else-if="stats.total>=2">
    <div class="history-metrics"><div><span>响应率</span><strong>{{stats.responseRate===null?'—':stats.responseRate.toFixed(0)+'%'}} <small>（{{stats.responses}}/{{stats.samples}} 次）</small></strong><small>{{stats.total}} 轮中 {{stats.failedRounds}} 轮有失败</small></div><div><span>稳定延迟中位数</span><strong>{{ms(stats.median)}}</strong></div><div title="慢的时候比快的时候多多少（P90−P10）"><span>波动幅度</span><strong v-if="stats.volatility!==null">{{ms(stats.volatility)}}</strong><small v-else class="few-rounds">轮数太少，暂不计算</small></div><div><span>最近一次失败</span><strong class="last-failure">{{stats.lastFailure?date(stats.lastFailure.time)+' · '+stats.lastFailure.detail:'无失败记录'}}</strong></div></div>
    <p class="history-note">共 {{stats.total}} 轮</p><LatencyChart :series="historySeries" history/>
   </template>
   <div v-if="listed.length" class="round-list"><div class="round-columns"><span>时间</span><span>稳定延迟</span><span>首次连接</span><span>响应</span></div><button v-for="r in listed" :key="r.id" class="round-row" :aria-label="'查看轮次 '+date(r.time)" :aria-pressed="r.id===current?.id" @click="selectedId=r.id"><time><small v-if="r.id===current?.id" class="current-round">当前</small>{{date(r.time)}}</time><span>{{ms(r.stable)}}</span><span>{{ms(r.first)}}</span><span :class="r.responses<r.total?'warn':''">{{r.responses}}/{{r.total}}<template v-if="r.failureSummary"> · {{r.failureSummary}}</template></span></button></div>
   <p v-else-if="!loading&&!error" class="history-note">这个范围没有延迟记录。</p><button v-if="stats.total>10" class="hd-link history-more" @click="expanded=!expanded">{{expanded?'收起':'查看更多轮次（'+(stats.total-10)+'）'}}</button>
  </section>
 </section>
</template>
<style scoped>
.ins-latency header{display:flex;align-items:baseline;justify-content:space-between;gap:8px}.ins-latency header strong{font-size:13px}.ins-latency header small{font-size:10px;color:var(--hd-muted)}.latency-metrics{display:grid;grid-template-columns:1fr 1fr .8fr;gap:10px;margin:12px 0 4px}.latency-metrics span{display:block;font-size:11px;color:var(--hd-muted);margin-bottom:4px}.latency-metrics strong{font-size:23px;line-height:1.25;font-weight:600;white-space:nowrap;letter-spacing:-.4px;font-variant-numeric:tabular-nums}.latency-metrics .first{color:#9a640d}.latency-metrics .responded{color:var(--green)}.latency-range.hd-site{display:grid;grid-template-columns:70px minmax(0,1fr) 62px;gap:8px;height:28px;margin:0}.latency-range .hd-site-name{font-size:12px;font-weight:500}.latency-range .hd-site-value{font-size:12px;text-align:right;font-weight:500;white-space:nowrap}.range-track{height:10px;position:relative;border-radius:5px;background:var(--hd-soft)}.range-track i{position:absolute;top:3px;height:4px;opacity:.35;border-radius:2px}.range-track b{position:absolute;top:0;width:10px;height:10px;transform:translateX(-50%);border-radius:50%}.range-scale{display:grid;grid-template-columns:70px minmax(0,1fr) 62px;gap:8px;margin-top:3px;font-size:9px;color:var(--hd-muted);white-space:nowrap}.range-axis{grid-column:2;position:relative;height:12px}.range-axis span{position:absolute;transform:translateX(-50%)}
.latency-history{margin-top:18px;padding-top:16px;border-top:1px solid var(--hd-line)}.latency-history nav{display:flex;gap:3px}.latency-history nav button{padding:3px 7px;font-size:11px;border-radius:5px}.latency-history nav button[aria-pressed=true]{background:var(--hd-soft);color:var(--hd-accent);font-weight:600}.history-metrics{display:grid;grid-template-columns:1fr 1fr;gap:14px 12px;padding:14px 0 4px}.history-metrics span,.history-metrics small{display:block;font-size:10px;color:var(--hd-muted)}.history-metrics strong{display:block;margin:4px 0;font-size:18px;font-weight:600;font-variant-numeric:tabular-nums}.history-metrics .last-failure{font-size:12px;line-height:1.5}.history-metrics strong small{display:inline;font-size:10px;font-weight:400}.few-rounds{margin-top:7px;line-height:1.5}.single-round{padding:12px 0;line-height:1.6}.current-round{display:inline-block;padding:1px 3px;margin-right:3px;border-radius:3px;background:var(--hd-soft);color:var(--hd-accent);font-size:9px}.round-row .warn{color:var(--hd-warn)}.history-note{font-size:10px;color:var(--hd-muted);margin:8px 0}.round-list{font-size:10px}.round-columns,.round-row{display:grid;grid-template-columns:1.15fr .85fr .85fr 1.5fr;gap:6px;align-items:center}.round-columns{padding:6px;color:var(--hd-muted)}.round-row{width:100%;padding:8px 6px;border:0;border-top:1px solid var(--hd-line);color:var(--hd-text);background:transparent;text-align:left;font-variant-numeric:tabular-nums;cursor:pointer}.round-row:hover,.round-row[aria-pressed=true]{background:var(--hd-soft);color:var(--hd-accent)}.history-more{margin-top:8px;font-size:11px}.round-banner{display:flex;align-items:center;justify-content:space-between;gap:6px;font-size:11px;color:var(--hd-accent);background:var(--hd-soft);padding:8px;margin-bottom:12px;border-radius:6px}
</style>
