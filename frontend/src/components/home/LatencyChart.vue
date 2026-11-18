<script setup lang="ts">
import {computed,onBeforeUnmount,ref,watch} from 'vue'
import {date} from '../../domain'
import {inspectorSamples as samples} from '../../inspectorLatency'
import {latencyMedianPoints,type LatencySeries,type LatencyConfigChange,type RoundSiteDetail} from '../../latencyHistory'
import {latencyScale,latencyTick,type LatencyScale} from '../../latencyScale'
import {hideHoverTip,showHoverTip} from '../../hoverTip'
const props=defineProps<{series:LatencySeries[];history?:boolean;changes?:LatencyConfigChange[];scale?:LatencyScale}>()
const holder=ref<HTMLElement>(),width=ref(360)
watch(holder,(el,_,onCleanup)=>{
 if(!el||typeof ResizeObserver==='undefined')return
 const observer=new ResizeObserver(([entry])=>{if(entry.contentRect.width>0)width.value=Math.max(240,entry.contentRect.width)})
 observer.observe(el);onCleanup(()=>observer.disconnect())
},{flush:'post'})
onBeforeUnmount(hideHoverTip)
const colors:Record<string,string>={cloudflare:'#378ADD',google:'#1D9E75',github:'#7F77DD',apple:'#D85A30'}
interface Dot {id:string;time:string;value:number|null;position:number;missing?:'failed'|'insufficient';reason:string;details?:RoundSiteDetail[]}
const plotted=computed(()=>{
 if(props.history){
  const points=latencyMedianPoints(props.series)
  return [{id:'median',name:'四站中位数',color:'#378ADD',dots:points.map<Dot>(p=>({id:p.id,time:p.time,value:p.value,position:Date.parse(p.time),missing:p.value===null?(p.details.some(d=>d.responses>0)?'insufficient':'failed'):undefined,reason:'',details:p.details}))}]
 }
 return props.series.map(s=>{
  const rounds=s.points.filter(p=>!p.id.startsWith('planned:')).map(p=>({...p,actual:samples(p,s.id)})).filter(p=>p.actual.length)
  const dots=rounds.at(-1)?.actual.map<Dot>((p,i)=>({id:p.id,time:p.time,value:p.value,position:i,missing:p.value===null?'failed':undefined,reason:p.error||p.note||''}))||[]
  return {...s,color:colors[s.id]||'#7F77DD',dots}
 })
})
const hasData=computed(()=>plotted.value.some(s=>s.dots.length))
const hasValues=computed(()=>plotted.value.some(s=>s.dots.some(p=>p.value!==null)))
const scale=computed(()=>props.scale||latencyScale(plotted.value.flatMap(s=>s.dots.map(p=>p.value))))
const left=computed(()=>Math.max(34,...scale.value.ticks.map(v=>latencyTick(v).length*6+9)))
const plotWidth=computed(()=>width.value-left.value-16)
const changePositions=computed(()=>props.history?(props.changes||[]).map(c=>Date.parse(c.time)).filter(Number.isFinite):[])
const bounds=computed(()=>{const times=[...plotted.value.flatMap(s=>s.dots.map(p=>p.position)),...changePositions.value];return times.length?[Math.min(...times),Math.max(...times)]:[0,0]})
const x=(v:number)=>left.value+(bounds.value[1]===bounds.value[0]?plotWidth.value/2:(v-bounds.value[0])*plotWidth.value/(bounds.value[1]-bounds.value[0]))
const configLines=computed(()=>props.history?(props.changes||[]).map(c=>({...c,position:Date.parse(c.time)})).filter(c=>Number.isFinite(c.position)&&c.position>=bounds.value[0]&&c.position<=bounds.value[1]).map(c=>({...c,x:x(c.position)})):[])
const y=(v:number)=>8+104*(1-v/scale.value.maximum)
function path(dots:Dot[]){let started=false;return dots.map(p=>{if(p.value===null){started=false;return ''}const cmd=started?'L':'M';started=true;return `${cmd}${x(p.position)},${y(p.value)}`}).join(' ')}
const ticks=computed(()=>props.history?[...new Set(bounds.value)].map(position=>({position,label:date(new Date(position).toISOString())})):Array.from({length:Math.max(0,...plotted.value.map(s=>s.dots.length))},(_,i)=>({position:i,label:i===0?'第1次':String(i+1)})).filter((_,i,list)=>list.length<=8||i===0||i===list.length-1))
const columns=computed(()=>{
 const groups=new Map<number,{name:string;point:Dot}[]>()
 for(const s of plotted.value)for(const point of s.dots){const group=groups.get(point.position)||[];group.push({name:s.name,point});groups.set(point.position,group)}
 return [...groups].sort(([a],[b])=>a-b).map(([position,items])=>({position,items,failed:items.filter(i=>i.point.missing==='failed').length,insufficient:items.filter(i=>i.point.missing==='insufficient').length}))
})
const hasFailed=computed(()=>columns.value.some(c=>c.failed))
const hasInsufficient=computed(()=>columns.value.some(c=>c.insufficient))
const hasMissing=computed(()=>hasFailed.value||hasInsufficient.value)
const chartHeight=computed(()=>hasMissing.value?174:142)
function hitBounds(index:number){
 const list=columns.value,position=x(list[index].position)
 const start=index?(x(list[index-1].position)+position)/2:left.value-8
 const end=index<list.length-1?(position+x(list[index+1].position))/2:width.value-8
 return {x:start,width:end-start}
}
const ms=(v:number)=>Math.round(v)+' ms'
function tip(items:{name:string;point:Dot}[]){
 if(props.history)return items.map(({point})=>date(point.time)+'\n'+(point.details||[]).map(d=>`${d.name} · ${d.responses}/${d.total} 响应 · ${d.stable!==null?ms(d.stable):!d.total?'未测':d.responses?'样本不足':'无响应'}${d.first!==null?' · 首次 '+ms(d.first):''}${d.reason?' · '+d.reason:''}`).join('\n')).join('\n')
 return `第 ${items[0].point.position+1} 次采样\n`+items.map(({name,point})=>`${name} · ${date(point.time)} · ${point.value===null?'无响应'+(point.reason?'：'+point.reason:''):ms(point.value)+(point.reason?' · '+point.reason:'')}`).join('\n')
}
</script>
<template>
 <div v-if="hasData" ref="holder" class="latency-chart">
  <div class="chart-scale"><span>延迟 · ms</span><span>{{hasValues?'自动量程 0–'+latencyTick(scale.maximum):'暂无可绘制的延迟值'}}</span></div>
  <svg class="latency-combined" :viewBox="`0 0 ${width} ${chartHeight}`" :style="{height:chartHeight+'px'}" role="img" :aria-label="history?'四站稳定延迟中位数随时间走势':'四站本轮实际采样走势'" @mouseleave="hideHoverTip">
   <g v-if="hasValues" class="latency-grid">
    <g v-for="v in scale.ticks" :key="v"><line :x1="left" :x2="width-16" :y1="y(v)" :y2="y(v)"/><text class="latency-axis-label" :x="left-6" :y="y(v)+3" text-anchor="end">{{latencyTick(v)}}</text></g>
   </g>
   <text v-for="(t,i) in ticks" :key="t.position" :x="x(t.position)" :y="chartHeight-9" :text-anchor="history?(ticks.length===1?'middle':i?'end':'start'):'middle'">{{t.label}}</text>
   <g v-for="c in configLines" :key="c.time"><line class="config-change" :x1="c.x" :x2="c.x" y1="8" y2="112"><title>{{c.label}}</title></line><text :x="Math.min(c.x+3,width-80)" y="16">配置已变化</text></g>
   <g v-for="s in plotted" :key="s.id"><path :d="path(s.dots)" fill="none" :stroke="s.color" stroke-width="1.8" stroke-linejoin="round"/><circle v-for="p in s.dots.filter(p=>p.value!==null)" :key="p.id" :cx="x(p.position)" :cy="y(p.value!)" r="3" :fill="s.color" pointer-events="none"/></g>
   <g v-if="hasMissing" class="missing-lane"><line :x1="left" :x2="width-16" y1="127" y2="127"/><text :x="left-6" y="145" text-anchor="end">未得值</text><g v-for="c in columns" :key="c.position"><text v-if="c.failed" class="failed" :x="x(c.position)" y="145" text-anchor="middle">×{{c.failed>1?c.failed:''}}</text><text v-else-if="c.insufficient" class="insufficient" :x="x(c.position)" y="145" text-anchor="middle">○</text></g></g>
   <rect v-for="(c,i) in columns" :key="c.position" class="hit" v-bind="hitBounds(i)" y="0" :height="chartHeight-20" fill="transparent" @mouseenter="showHoverTip($event,tip(c.items))" @mousemove="showHoverTip($event,tip(c.items))" @click="showHoverTip($event,tip(c.items))"><title>{{tip(c.items)}}</title></rect>
  </svg>
  <div class="latency-legend"><span v-for="s in plotted" :key="s.id"><i :style="{background:s.color}"/>{{s.name}}</span></div>
  <div v-if="hasMissing" class="missing-key"><span v-if="hasFailed">× 无响应{{!history?'（数字为站点数）':''}}</span><span v-if="hasInsufficient">○ 样本不足，未算出稳定延迟</span></div>
 </div>
</template>
<style scoped>
.latency-chart{margin-bottom:12px}.chart-scale{display:flex;justify-content:space-between;gap:10px;color:var(--hd-muted);font-size:10px;margin:8px 0 3px}.latency-combined{display:block;width:100%;overflow:visible}.latency-combined text{fill:var(--hd-muted);font-size:10px}.latency-grid line,.missing-lane line{stroke:var(--hd-line);stroke-width:1;vector-effect:non-scaling-stroke}.missing-lane line{stroke-dasharray:2 3}.latency-combined .failed{fill:var(--red);font-size:13px;font-weight:700}.latency-combined .insufficient{fill:var(--hd-muted);font-size:13px;font-weight:700}.latency-combined .hit{cursor:crosshair}.latency-combined .config-change{stroke:var(--hd-muted);stroke-width:1;stroke-dasharray:3 3;opacity:.65;vector-effect:non-scaling-stroke}.latency-legend{display:flex;flex-wrap:wrap;justify-content:space-between;gap:6px;font-size:10px;color:var(--hd-muted);margin-top:3px}.latency-legend span{display:flex;align-items:center;gap:4px}.latency-legend i{width:10px;height:3px}.missing-key{display:flex;flex-wrap:wrap;gap:4px 12px;font-size:10px;color:var(--hd-muted);margin-top:5px}
</style>
