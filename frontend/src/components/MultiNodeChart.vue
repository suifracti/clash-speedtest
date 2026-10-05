<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {date} from '../domain'
import type {NodeSeries,TrendPoint} from '../presentation'
type ChartSeries=NodeSeries&{nodeId?:string;targetId?:string;targetName?:string}
const props=defineProps<{series:ChartSeries[];unit:string;title?:string;foldLegend?:boolean}>()
const hidden=ref<string[]>([]),search=ref(''),hovered=ref<{series:ChartSeries;point:TrendPoint}|null>(null)
const colors=['#4063eb','#1b987b','#b45ad4','#cc793b','#d25278','#4699bf','#72882e','#8063a6']
const shown=computed(()=>props.series.filter(s=>!hidden.value.includes(s.id)))
const multiTarget=computed(()=>new Set(props.series.flatMap(s=>s.targetId?[s.targetId]:[])).size>1)
const comparisonItem=computed(()=>props.unit==='状态'?'服务':'站点')
const nodeCount=computed(()=>new Set(props.series.map(s=>s.nodeId||s.id)).size)
const legend=computed(()=>props.series.filter(s=>(s.name+' '+s.airport+' '+(s.targetName||'')).toLowerCase().includes(search.value.trim().toLowerCase())))
const allPoints=computed(()=>props.series.flatMap(s=>s.points).filter(p=>Number.isFinite(Date.parse(p.time))&&p.tone!=='empty'))
const minTime=computed(()=>Math.min(...allPoints.value.map(p=>Date.parse(p.time))))
const maxTime=computed(()=>Math.max(...allPoints.value.map(p=>Date.parse(p.time))))
const maxValue=computed(()=>props.unit==='状态'?1:Math.max(10,...allPoints.value.flatMap(p=>p.value===null?[]:[p.value]))*1.1)
const measured=computed(()=>props.series.filter(s=>s.points.some(p=>p.tone!=='empty')).length)
function color(id:string){return colors[props.series.findIndex(s=>s.id===id)%colors.length]}
function x(time:string){return maxTime.value===minTime.value?495:44+(Date.parse(time)-minTime.value)/(maxTime.value-minTime.value)*902}
function y(value:number){return 250-value/maxValue.value*215}
function path(series:ChartSeries){let line='',connected=false;for(const p of series.points){if(!Number.isFinite(Date.parse(p.time))||p.tone==='empty'||p.tone==='bad'||p.value===null){connected=false;continue}line+=`${connected?'L':'M'}${x(p.time)},${y(p.value)} `;connected=true}return line}
function toggle(id:string){hidden.value=hidden.value.includes(id)?hidden.value.filter(k=>k!==id):[...hidden.value,id]}
function inspect(e:PointerEvent){const box=(e.currentTarget as SVGElement).getBoundingClientRect(),px=(e.clientX-box.left)/box.width*960,py=(e.clientY-box.top)/box.height*300;let distance=Infinity,result:typeof hovered.value=null;for(const s of shown.value)for(const point of s.points){if(point.tone==='empty'||!Number.isFinite(Date.parse(point.time)))continue;const d=Math.hypot(x(point.time)-px,(point.value===null||point.tone==='bad'?266:y(point.value))-py);if(d<distance){distance=d;result={series:s,point}}}hovered.value=distance<55?result:null}
function valueText(p:TrendPoint){return p.tone==='bad'?'失败':p.tone==='empty'?'暂无记录':props.unit==='状态'?p.tone==='good'?'通过':'受限':p.value===null?'—':p.value.toFixed(props.unit==='MiB/s'?2:0)+' '+props.unit}
function latest(s:ChartSeries){const p=s.points[s.points.length-1];return p?valueText(p):'暂无记录'}
function seriesLabel(s:ChartSeries){return [s.name,s.airport,s.targetName].filter(Boolean).join(' · ')}
watch(()=>props.series.map(s=>s.id).sort().join('|'),()=>{hidden.value=hidden.value.filter(id=>props.series.some(s=>s.id===id));hovered.value=null})
watch(shown,list=>{
  const current=hovered.value;if(!current)return
  const series=list.find(s=>s.id===current.series.id),point=series?.points.find(p=>p.id===current.point.id)
  hovered.value=series&&point&&point.tone!=='empty'&&Number.isFinite(Date.parse(point.time))?{series,point}:null
})
</script>
<template>
  <section class="multi-node-chart" aria-label="节点历史图表">
    <header><div><h3>{{title||'节点历史走势'}}</h3><p v-if="multiTarget">{{nodeCount}} 个节点 · {{series.length}} 条{{comparisonItem}}曲线 · {{measured}} 条有记录 · 当前显示 {{shown.length}} 条</p><p v-else>{{series.length}} 个节点 · {{measured}} 个有记录 · 当前显示 {{shown.length}} 个节点</p></div><div v-if="series.length>1" class="button-group"><button class="button ghost" @click="hidden=[]">显示全部</button><button class="button ghost" @click="hidden=series.map(s=>s.id)">清空曲线</button></div></header>
    <div v-if="!allPoints.length" class="chart-empty">当前范围暂无已读取的检测记录</div>
    <svg v-else viewBox="0 0 960 300" class="overview-svg" role="img" :aria-label="title||'节点历史走势图'" @pointermove="inspect" @pointerleave="hovered=null" @click="inspect">
      <text x="3" y="18">{{unit}}</text><g v-for="fraction in (unit==='状态'?[0,.5,1]:[0,.25,.5,.75,1])" :key="fraction"><line x1="44" x2="946" :y1="y(maxValue*fraction)" :y2="y(maxValue*fraction)"/><text x="37" :y="y(maxValue*fraction)+4" text-anchor="end">{{unit==='状态'?fraction===1?'通过':fraction===0?'失败':'受限':(maxValue*fraction).toFixed(unit==='MiB/s'?1:0)}}</text></g>
      <g v-for="s in shown" :key="s.id" class="overview-series" :class="{dimmed:hovered&&hovered.series.id!==s.id}" :aria-label="seriesLabel(s)" :style="{color:color(s.id)}"><path :d="path(s)" class="overview-line"/><template v-for="p in s.points" :key="p.id"><circle v-if="p.value!==null&&p.tone!=='bad'&&p.tone!=='empty'" :cx="x(p.time)" :cy="y(p.value)" r="2.3"/><path v-else-if="p.tone==='bad'" :d="`M${x(p.time)-3} 263l6 6m-6 0l6-6`" class="overview-failure"/></template></g>
      <line v-if="hovered" :x1="x(hovered.point.time)" :x2="x(hovered.point.time)" y1="30" y2="271" class="overview-guide"/>
      <text x="44" y="293">{{date(new Date(minTime).toISOString())}}</text><text x="946" y="293" text-anchor="end">{{date(new Date(maxTime).toISOString())}}</text>
    </svg>
    <div class="overview-readout"><template v-if="hovered"><strong>{{hovered.series.name}}<template v-if="hovered.series.targetName"> · {{hovered.series.targetName}}</template></strong><span>{{hovered.series.airport}} · {{hovered.point.description}}</span><b>{{valueText(hovered.point)}}</b></template><span v-else>{{series.length===1?'悬停图中记录查看数值；上方可加入其它节点对比。':multiTarget?'各'+comparisonItem+'共用真实时间轴；没有记录的'+comparisonItem+'标为未测。':'所有节点共用真实时间轴；可在下方挑选要比较的曲线。'}}</span></div>
    <details v-if="series.length>1||!foldLegend" class="chart-comparison-picker" :open="multiTarget||!foldLegend"><summary>{{multiTarget?'挑选节点与'+comparisonItem+'曲线':'挑选对比节点'}} · 当前显示 {{shown.length}} {{multiTarget?'条':'个'}}</summary>
    <input v-model="search" type="search" :aria-label="multiTarget?'搜索图表节点、机场或'+comparisonItem:'搜索图表节点'" :placeholder="multiTarget?'搜索节点、机场或'+comparisonItem:'搜索要对比的节点或机场'">
    <div class="overview-legend"><label v-for="s in legend" :key="s.id"><input type="checkbox" :checked="!hidden.includes(s.id)" :aria-label="'图表显示 '+seriesLabel(s)" @change="toggle(s.id)"><i :style="{background:color(s.id)}"/><span><strong>{{s.name}}<template v-if="s.targetName"> · {{s.targetName}}</template></strong><small>{{s.airport}}</small></span><b>{{s.points.some(point=>point.tone!=='empty')?latest(s):multiTarget?'未测':'暂无记录'}}</b></label></div>
    </details>
  </section>
</template>
<style scoped>
.overview-readout strong{color:var(--ink)}
.overview-readout>span{min-width:0;overflow-wrap:anywhere}
.overview-legend strong{line-height:1.5}
@media(max-width:640px){.overview-legend{grid-template-columns:minmax(0,1fr)}.overview-readout{gap:5px 10px}.overview-readout>span{flex-basis:100%}.overview-legend label{min-height:42px}}
</style>
