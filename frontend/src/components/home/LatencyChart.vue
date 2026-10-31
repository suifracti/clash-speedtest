<script setup lang="ts">
import {computed} from 'vue'
import {date} from '../../domain'
import {inspectorMedian as median,inspectorSamples as samples} from '../../inspectorLatency'
import {latencyMedianPoints,type LatencySeries,type LatencyConfigChange,type RoundSiteDetail} from '../../latencyHistory'
import {hideHoverTip,showHoverTip} from '../../hoverTip'
const props=defineProps<{series:LatencySeries[];history?:boolean;changes?:LatencyConfigChange[]}>()
const colors:Record<string,string>={cloudflare:'#378ADD',google:'#1D9E75',github:'#7F77DD',apple:'#D85A30'}
interface Dot {id:string;time:string;value:number|null;position:number;failed:boolean;reason:string;details?:RoundSiteDetail[]}
const plotted=computed(()=>{
 if(props.history){
  const points=latencyMedianPoints(props.series)
  return [{id:'median',name:'四站中位数',color:'#378ADD',dots:points.map<Dot>(p=>({id:p.id,time:p.time,value:p.value,position:Date.parse(p.time),failed:p.value===null,reason:p.details.filter(d=>d.tone==='warn'||d.tone==='bad').map(d=>d.name+' '+(d.reason||'无响应')).join('；')||'四站都没有稳定延迟',details:p.details}))}]
 }
 return props.series.map(s=>{
  const rounds=s.points.filter(p=>!p.id.startsWith('planned:')).map(p=>({...p,actual:samples(p,s.id)})).filter(p=>p.actual.length)
  const dots=rounds.at(-1)?.actual.map<Dot>((p,i)=>({id:p.id,time:p.time,value:p.value,position:i,failed:p.value===null,reason:p.error||p.note||'没有响应'}))||[]
  return {...s,color:colors[s.id]||'#7F77DD',dots}
 })
})
const hasData=computed(()=>plotted.value.some(s=>s.dots.length))
const maximum=computed(()=>Math.max(1500,...plotted.value.flatMap(s=>s.dots.map(p=>p.value||0))))
const changePositions=computed(()=>props.history?(props.changes||[]).map(c=>Date.parse(c.time)).filter(Number.isFinite):[])
const bounds=computed(()=>{const times=[...plotted.value.flatMap(s=>s.dots.map(p=>p.position)),...changePositions.value];return [Math.min(...times),Math.max(...times)]})
const x=(v:number)=>34+(bounds.value[1]===bounds.value[0]?155:(v-bounds.value[0])*310/(bounds.value[1]-bounds.value[0]))
const configLines=computed(()=>props.history?(props.changes||[]).map(c=>({...c,position:Date.parse(c.time)})).filter(c=>Number.isFinite(c.position)&&c.position>=bounds.value[0]&&c.position<=bounds.value[1]).map(c=>({...c,x:x(c.position)})):[])
const y=(v:number)=>8+94*(1-v/maximum.value)
function path(dots:Dot[]){let started=false;return dots.map(p=>{if(p.value===null){started=false;return ''}const cmd=started?'L':'M';started=true;return `${cmd}${x(p.position)},${y(p.value)}`}).join(' ')}
const ticks=computed(()=>props.history?[...new Set(bounds.value)].map(position=>({position,label:date(new Date(position).toISOString())})):Array.from({length:Math.max(0,...plotted.value.map(s=>s.dots.length))},(_,i)=>({position:i,label:i===0?'第1次':String(i+1)})).filter((_,i,list)=>list.length<=8||i===0||i===list.length-1))
const ms=(v:number)=>Math.round(v)+' ms'
function tip(name:string,point:Dot){
 if(point.details?.length)return date(point.time)+'\n'+point.details.map(d=>`${d.name} ${d.stable===null?'—':ms(d.stable)}${d.tone==='good'?'':d.tone==='warn'?'（部分失败）':d.tone==='bad'?'（无响应）':'（未测）'}`).join('\n')
 return name+' · '+date(point.time)+' · '+(point.value===null?'失败：'+point.reason:ms(point.value))
}
</script>
<template><div v-if="hasData"><svg class="latency-combined" viewBox="0 0 360 120" preserveAspectRatio="none" role="img" :aria-label="history?'四站稳定延迟中位数随时间走势':'四站本轮实际采样走势'" @mouseleave="hideHoverTip"><rect x="34" :y="y(300)" width="310" :height="y(0)-y(300)" fill="#EAF3DE" opacity=".6"/><rect x="34" :y="y(800)" width="310" :height="y(300)-y(800)" fill="#FAEEDA" opacity=".6"/><rect x="34" y="8" width="310" :height="y(800)-8" fill="#FAECE7" opacity=".6"/><text v-for="v in [0,500,1000,maximum]" :key="v" x="29" :y="y(v)+3" text-anchor="end">{{Math.round(v)}}</text><text v-for="(t,i) in ticks" :key="t.position" :x="x(t.position)" y="117" :text-anchor="history?(i?'end':'start'):'middle'">{{t.label}}</text><g v-for="c in configLines" :key="c.time"><line class="config-change" :x1="c.x" :x2="c.x" y1="8" y2="102"><title>{{c.label}}</title></line><text :x="Math.min(c.x+3,280)" y="16">配置已变化</text></g><g v-for="s in plotted" :key="s.id"><path :d="path(s.dots)" fill="none" :stroke="s.color" stroke-width="1.8" stroke-linejoin="round"/><g v-for="p in s.dots" :key="p.id"><circle class="hit" :cx="x(p.position)" :cy="p.value===null?102:y(p.value)" r="8" fill="transparent" @mouseenter="showHoverTip($event,tip(s.name,p))"/><circle v-if="p.value!==null" :cx="x(p.position)" :cy="y(p.value)" r="3" :fill="s.color" pointer-events="none"/><text v-if="p.failed" :x="x(p.position)" y="102" class="failed" text-anchor="middle" pointer-events="none">×</text></g></g></svg><div class="latency-legend"><template v-if="history"><span><i style="background:#378ADD"/>四站中位数</span><span class="legend-failed">× 失败</span></template><template v-else><span v-for="s in plotted" :key="s.id"><i :style="{background:s.color}"/>{{s.name}}</span></template></div></div></template>
<style scoped>
.latency-combined{display:block;width:100%;height:120px;overflow:visible}.latency-combined text{fill:var(--hd-muted);font-size:9px}.latency-combined .failed{fill:var(--red);font-size:13px;font-weight:700}.latency-combined .hit{cursor:crosshair}.latency-combined .config-change{stroke:var(--hd-muted);stroke-width:1;stroke-dasharray:3 3;opacity:.65;vector-effect:non-scaling-stroke}.latency-legend{display:flex;justify-content:space-between;gap:6px;font-size:10px;color:var(--hd-muted);margin:4px 0 12px}.latency-legend span{display:flex;align-items:center;gap:4px}.latency-legend i{width:10px;height:3px}.latency-legend .legend-failed{color:var(--red)}
</style>
