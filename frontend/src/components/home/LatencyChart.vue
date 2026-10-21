<script setup lang="ts">
import {computed} from 'vue'
import {date} from '../../domain'
import {inspectorMedian as median,inspectorSamples as samples} from '../../inspectorLatency'
import {latencyRounds,type LatencySeries,type LatencyConfigChange} from '../../latencyHistory'
const props=defineProps<{series:LatencySeries[];history?:boolean;changes?:LatencyConfigChange[]}>()
const colors:Record<string,string>={cloudflare:'#378ADD',google:'#1D9E75',github:'#7F77DD',apple:'#D85A30'}
const daily=computed(()=>props.history&&latencyRounds(props.series).length>30)
const plotted=computed(()=>props.series.map(s=>{
 const rounds=s.points.filter(p=>!p.id.startsWith('planned:')).map(p=>({...p,actual:samples(p,s.id)})).filter(p=>p.actual.length)
 let dots=props.history?rounds.map(p=>({id:p.id,time:p.time,value:median(p.actual.slice(1).flatMap(s=>s.value===null?[]:[s.value])),position:Date.parse(p.time),failed:p.actual.every(s=>s.value===null),reason:p.actual.find(s=>s.value===null)?.error||'没有稳定延迟采样'})):(rounds.at(-1)?.actual||[]).map((p,i)=>({id:p.id,time:p.time,value:p.value,position:i,failed:p.value===null,reason:p.error||p.note||'没有响应'}))
 if(daily.value){const days=new Map<string,typeof dots>();for(const p of dots){const day=new Intl.DateTimeFormat('en-CA',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit',day:'2-digit'}).format(new Date(p.time));days.set(day,[...(days.get(day)||[]),p])}dots=[...days].map(([day,list])=>({id:day,time:day+'T12:00:00+08:00',position:Date.parse(day+'T12:00:00+08:00'),value:median(list.flatMap(p=>p.value===null?[]:[p.value])),failed:list.some(p=>p.failed),reason:day+' 有失败记录'}))}
 return {...s,color:colors[s.id]||'#7F77DD',dots}
}))
const hasData=computed(()=>plotted.value.some(s=>s.dots.length))
const maximum=computed(()=>Math.max(1500,...plotted.value.flatMap(s=>s.dots.map(p=>p.value||0))))
const changePositions=computed(()=>props.history?(props.changes||[]).map(c=>Date.parse(c.time)).filter(Number.isFinite):[])
const bounds=computed(()=>{const times=[...plotted.value.flatMap(s=>s.dots.map(p=>p.position)),...changePositions.value];return [Math.min(...times),Math.max(...times)]})
const x=(v:number)=>34+(bounds.value[1]===bounds.value[0]?155:(v-bounds.value[0])*310/(bounds.value[1]-bounds.value[0]))
const configLines=computed(()=>props.history?(props.changes||[]).map(c=>({...c,position:Date.parse(c.time)})).filter(c=>Number.isFinite(c.position)&&c.position>=bounds.value[0]&&c.position<=bounds.value[1]).map(c=>({...c,x:x(c.position)})):[])
const y=(v:number)=>8+94*(1-v/maximum.value)
function path(dots:typeof plotted.value[number]['dots']){let started=false;return dots.map(p=>{if(p.value===null){started=false;return ''}const cmd=started?'L':'M';started=true;return `${cmd}${x(p.position)},${y(p.value)}`}).join(' ')}
const ticks=computed(()=>props.history?[...new Set(bounds.value)].map(position=>({position,label:date(new Date(position).toISOString())})):Array.from({length:Math.max(0,...plotted.value.map(s=>s.dots.length))},(_,i)=>({position:i,label:i===0?'第1次':String(i+1)})).filter((_,i,list)=>list.length<=8||i===0||i===list.length-1))
const ms=(v:number)=>Math.round(v)+' ms'
</script>
<template><div v-if="hasData"><svg class="latency-combined" viewBox="0 0 360 120" preserveAspectRatio="none" role="img" :aria-label="history?'四站历史稳定延迟随时间走势':'四站本轮实际采样走势'"><rect x="34" :y="y(300)" width="310" :height="y(0)-y(300)" fill="#EAF3DE" opacity=".6"/><rect x="34" :y="y(800)" width="310" :height="y(300)-y(800)" fill="#FAEEDA" opacity=".6"/><rect x="34" y="8" width="310" :height="y(800)-8" fill="#FAECE7" opacity=".6"/><text v-for="v in [0,500,1000,maximum]" :key="v" x="29" :y="y(v)+3" text-anchor="end">{{Math.round(v)}}</text><text v-for="(t,i) in ticks" :key="t.position" :x="x(t.position)" y="117" :text-anchor="history?(i?'end':'start'):'middle'">{{t.label}}</text><g v-for="c in configLines" :key="c.time"><line class="config-change" :x1="c.x" :x2="c.x" y1="8" y2="102"><title>{{c.label}}</title></line></g><g v-for="s in plotted" :key="s.id"><path :d="path(s.dots)" fill="none" :stroke="s.color" stroke-width="1.8" stroke-linejoin="round"/><g v-for="p in s.dots" :key="p.id"><title>{{s.name}} · {{date(p.time)}} · {{p.value===null?'失败：'+p.reason:ms(p.value)}}</title><circle v-if="p.value!==null" :cx="x(p.position)" :cy="y(p.value)" r="3" :fill="s.color"/><text v-if="p.failed" :x="x(p.position)" y="102" class="failed" text-anchor="middle">×</text></g></g></svg><div class="latency-legend"><span v-for="s in plotted" :key="s.id"><i :style="{background:s.color}"/>{{s.name}}</span></div><small v-if="daily" class="daily-note">超过 30 轮，按天显示稳定延迟中位数</small></div></template>
<style scoped>
.latency-combined{display:block;width:100%;height:120px;overflow:visible}.latency-combined text{fill:var(--hd-muted);font-size:9px}.latency-combined .failed{fill:var(--red);font-size:13px;font-weight:700}.latency-combined .config-change{stroke:var(--hd-muted);stroke-width:1;stroke-dasharray:3 3;opacity:.65;vector-effect:non-scaling-stroke}.latency-legend{display:flex;justify-content:space-between;gap:6px;font-size:10px;color:var(--hd-muted);margin:4px 0 12px}.latency-legend span{display:flex;align-items:center;gap:4px}.latency-legend i{width:10px;height:3px}.daily-note{display:block;color:var(--hd-muted);margin-bottom:8px}
</style>
