<script setup lang="ts">
import {computed} from 'vue'
import {date} from '../../domain'
import {latencyTargetId} from '../../latencyTargets'
import type {SamplePoint,TrendPoint} from '../../presentation'
const props=defineProps<{series:{id:string;name:string;points:TrendPoint[];context:string}[]}>()
const colors=['#378ADD','#1D9E75','#7F77DD','#D85A30','#BB8C24','#427C91']
const fixed:Record<string,string>={cloudflare:colors[0],google:colors[1],github:colors[2],apple:colors[3]}
function median(values:number[]){const a=values.slice().sort((a,b)=>a-b),i=Math.floor(a.length/2);return a.length?a.length%2?a[i]:(a[i-1]+a[i])/2:null}
function samples(point:TrendPoint,id:string):SamplePoint[]{return (point.sampleGroups?.filter(g=>latencyTargetId(g.target)===id).flatMap(g=>g.samples)||point.samples||[]).filter(s=>!s.executionState&&!(s.tone==='empty'&&s.value===null))}
const data=computed(()=>props.series.map((s,i)=>{
 const rounds=s.points.filter(p=>!p.id.startsWith('planned:')).map(p=>({...p,actual:samples(p,s.id)})).filter(p=>p.actual.length)
 const latest=rounds.at(-1),values=(latest?.actual||[]).flatMap(p=>p.value===null?[]:[p.value])
 return {...s,color:fixed[s.id]||colors[i%colors.length],rounds,latest,values,median:median(values),stable:median((latest?.actual.slice(1)||[]).flatMap(p=>p.value===null?[]:[p.value])),first:latest?.actual[0]?.value??null}
}))
const rounds=computed(()=>[...new Map(data.value.flatMap(s=>s.rounds.map(p=>[p.id,p.time] as const))).entries()].sort((a,b)=>Date.parse(a[1])-Date.parse(b[1])))
const stable=computed(()=>median(data.value.flatMap(s=>s.stable===null?[]:[s.stable])))
const first=computed(()=>median(data.value.flatMap(s=>s.first===null?[]:[s.first])))
const total=computed(()=>data.value.reduce((n,s)=>n+(s.latest?.actual.length||0),0))
const responses=computed(()=>data.value.reduce((n,s)=>n+s.values.length,0))
const latestTime=computed(()=>rounds.value.at(-1)?.[1])
const single=computed(()=>rounds.value.length<=1)
const count=computed(()=>Math.max(1,...data.value.map(s=>s.latest?.actual.length||0)))
const plotted=computed(()=>data.value.map(s=>({...s,dots:single.value?(s.latest?.actual||[]).map((p,i)=>({id:p.id,time:p.time,value:p.value,position:i,failed:p.value===null,reason:p.error||p.note||'没有响应'})):s.rounds.map(p=>({id:p.id,time:p.time,value:median(p.actual.slice(1).flatMap(s=>s.value===null?[]:[s.value])),position:Date.parse(p.time),failed:p.actual.some(s=>s.value===null),reason:p.actual.find(s=>s.value===null)?.error||'没有稳定延迟采样'}))})))
const maximum=computed(()=>Math.max(1500,...plotted.value.flatMap(s=>s.dots.map(p=>p.value||0))))
const timeBounds=computed(()=>{const times=plotted.value.flatMap(s=>s.dots.map(p=>p.position));return [Math.min(...times),Math.max(...times)]})
const x=(position:number)=>single.value?34+(count.value===1?155:position*310/(count.value-1)):34+(position-timeBounds.value[0])*310/Math.max(1,timeBounds.value[1]-timeBounds.value[0])
const y=(value:number)=>8+94*(1-value/maximum.value)
function path(dots:typeof plotted.value[number]['dots']){let started=false;return dots.map(p=>{if(p.value===null){started=false;return ''}const cmd=started?'L':'M';started=true;return `${cmd}${x(p.position)},${y(p.value)}`}).join(' ')}
const ticks=computed(()=>single.value?Array.from({length:count.value},(_,i)=>({position:i,label:i===0?'第1次':i===count.value-1?`第${i+1}次`:String(i+1)})).filter((_,i)=>count.value<=8||i===0||i===count.value-1):[...new Set(timeBounds.value)].map(position=>({position,label:date(new Date(position).toISOString())})))
const pct=(v:number)=>Math.min(100,Math.max(0,v/1500*100))+'%'
const ms=(v:number|null)=>v===null?'—':Math.round(v).toLocaleString()+' ms'
</script>
<template>
 <section class="ins-latency" aria-label="延迟统计与合并走势">
  <header><strong>延迟</strong><small>{{rounds.length?rounds.length+' 轮 · '+date(latestTime):'暂无采样'}}</small></header>
  <div class="latency-metrics">
   <div><span>稳定延迟</span><strong :title="'各站去掉首次采样后的中位数，再取四站中位数'">{{ms(stable)}}</strong></div>
   <div><span>首次连接</span><strong class="first" title="各站第 1 次采样的中位数，包含建立连接">{{ms(first)}}</strong></div>
   <div><span>响应</span><strong :class="responses===total&&total?'responded':''">{{total?responses+'/'+total:'—'}}</strong></div>
  </div>
  <template v-if="total">
   <svg class="latency-combined" viewBox="0 0 360 120" preserveAspectRatio="none" role="img" :aria-label="single?'四站本轮实际采样走势':'四站稳定延迟随时间走势'">
    <rect x="34" :y="y(300)" width="310" :height="y(0)-y(300)" fill="#EAF3DE" opacity=".6"/>
    <rect x="34" :y="y(800)" width="310" :height="y(300)-y(800)" fill="#FAEEDA" opacity=".6"/>
    <rect x="34" y="8" width="310" :height="y(800)-8" fill="#FAECE7" opacity=".6"/>
    <text v-for="v in [0,500,1000,maximum]" :key="v" x="29" :y="y(v)+3" text-anchor="end">{{Math.round(v)}}</text>
    <text v-for="t in ticks" :key="t.position" :x="x(t.position)" y="117"  :text-anchor="single?'middle':t.position===ticks[0]?.position?'start':'end'">{{t.label}}</text>
    <g v-for="s in plotted" :key="s.id">
     <path :d="path(s.dots)" fill="none" :stroke="s.color" stroke-width="1.8" stroke-linejoin="round"/>
     <g v-for="p in s.dots" :key="p.id"><title>{{s.name}} · {{date(p.time)}} · {{p.value===null?'失败：'+p.reason:ms(p.value)}}</title>
      <circle v-if="p.value!==null" :cx="x(p.position)" :cy="y(p.value)" r="3" :fill="s.color"/>
      <text v-if="p.failed" :x="x(p.position)" y="102" class="failed" text-anchor="middle">×</text>
     </g>
    </g>
   </svg>
   <div class="latency-legend"><span v-for="s in data" :key="s.id"><i :style="{background:s.color}"/>{{s.name}}</span></div>
  </template>
  <div v-for="s in data" :key="s.id" class="hd-site latency-range" :title="s.context">
   <span class="hd-site-name">{{s.name}}</span>
   <div class="range-track"><template v-if="s.values.length"><i :style="{left:pct(Math.min(...s.values)),right:pct(1500-Math.max(...s.values)),background:s.color}"/><b :style="{left:pct(s.median!),background:s.color}"/></template></div>
   <span class="hd-site-value" :title="s.context">{{ms(s.median)}}</span>
  </div>
  <div class="range-scale"><span>0</span><span>500</span><span>1000</span><span>1500 ms</span></div>
 </section>
</template>
<style scoped>
.ins-latency header{display:flex;align-items:baseline;justify-content:space-between;gap:8px}.ins-latency header strong{font-size:13px}.ins-latency header small{font-size:10px;color:var(--hd-muted)}.latency-metrics{display:grid;grid-template-columns:1fr 1fr .8fr;gap:10px;margin:12px 0 4px}.latency-metrics span{display:block;font-size:11px;color:var(--hd-muted);margin-bottom:4px}.latency-metrics strong{font-size:23px;line-height:1.25;font-weight:600;white-space:nowrap;letter-spacing:-.4px;font-variant-numeric:tabular-nums}.latency-metrics .first{color:#9a640d}.latency-metrics .responded{color:var(--green)}.latency-combined{display:block;width:100%;height:120px;overflow:visible}.latency-combined text{fill:var(--hd-muted);font-size:9px}.latency-combined .failed{fill:var(--red);font-size:13px;font-weight:700}.latency-legend{display:flex;justify-content:space-between;gap:6px;font-size:10px;color:var(--hd-muted);margin:4px 0 12px}.latency-legend span{display:flex;align-items:center;gap:4px}.latency-legend i{width:10px;height:3px}.latency-range.hd-site{display:grid;grid-template-columns:70px minmax(0,1fr) 62px;gap:8px;height:28px;margin:0}.latency-range .hd-site-name{font-size:12px;font-weight:500}.latency-range .hd-site-value{font-size:12px;text-align:right;font-weight:500;white-space:nowrap}.range-track{height:10px;position:relative;border-radius:5px;background:var(--hd-soft)}.range-track i{position:absolute;top:3px;height:4px;opacity:.35;border-radius:2px}.range-track b{position:absolute;top:0;width:10px;height:10px;transform:translateX(-50%);border-radius:50%}.range-scale{margin:3px 70px 0 78px;display:flex;justify-content:space-between;font-size:9px;color:var(--hd-muted);white-space:nowrap}
</style>
