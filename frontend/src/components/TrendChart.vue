<script setup lang="ts">
import {computed,onMounted,onBeforeUnmount,ref} from 'vue'
import type {TrendPoint} from '../presentation'
import {date} from '../domain'
import MultiNodeChart from './MultiNodeChart.vue'
const props=defineProps<{points:TrendPoint[];unit:string;label:string;series?:{id:string;name:string;points:TrendPoint[]}[]}>();const holder=ref<HTMLElement>(),width=ref(400),hover=ref<number|null>(null);let observer:ResizeObserver|undefined
const comparisonSeries=computed(()=>props.series?.map(series=>({...series,name:props.label,airport:'',nodeId:'current',targetId:series.id,targetName:series.name}))||[])
onMounted(()=>{observer=new ResizeObserver(([entry])=>width.value=Math.max(240,entry.contentRect.width));if(holder.value)observer.observe(holder.value)});onBeforeUnmount(()=>observer?.disconnect())
const valid=computed(()=>props.points.filter(p=>p.value!==null&&p.tone!=='empty'&&p.tone!=='bad'))
const max=computed(()=>props.unit==='通过'?1:Math.max(10,...valid.value.map(p=>p.value||0))*1.15)
const timestamps=computed(()=>props.points.map(p=>Date.parse(p.time)))
const timeMin=computed(()=>Math.min(...timestamps.value)),timeMax=computed(()=>Math.max(...timestamps.value))
function x(i:number){return timestamps.value.length<2?width.value/2:38+(timestamps.value[i]-timeMin.value)/Math.max(1,timeMax.value-timeMin.value)*(width.value-54)}
function y(value:number){return 153-value/max.value*123}
const path=computed(()=>{let d='',connected=false,condition:string|undefined;props.points.forEach((p,i)=>{if(p.value===null||p.tone==='empty'||p.tone==='bad'){connected=false;return}if(condition!==p.conditionKey)connected=false;condition=p.conditionKey;d+=`${connected?'L':'M'}${x(i).toFixed(1)},${y(p.value).toFixed(1)} `;connected=true});return d})
const ticks=computed(()=>[0,.5,1].map(t=>({value:max.value*t,y:y(max.value*t)})))
function over(e:PointerEvent){const box=(e.currentTarget as SVGElement).getBoundingClientRect();const cursor=e.clientX-box.left;let nearest=0;for(let i=1;i<props.points.length;i++)if(Math.abs(x(i)-cursor)<Math.abs(x(nearest)-cursor))nearest=i;hover.value=props.points.length?nearest:null}
const hovered=computed(()=>hover.value===null?null:props.points[hover.value])
</script>
<template><div ref="holder" class="trend-holder"><MultiNodeChart v-if="comparisonSeries.length>1" :series="comparisonSeries" :unit="unit==='通过'?'状态':unit" :title="label"/><div v-else-if="!points.length" class="chart-empty">这个范围还没有检测记录</div><template v-else><svg :viewBox="`0 0 ${width} 188`" class="trend-svg" role="img" :aria-label="label" @pointermove="over" @pointerleave="hover=null" @click="over"><text x="2" y="15">{{unit==='通过'?'本轮通过比例':unit}}</text><g v-for="tick in ticks" :key="tick.value"><line x1="38" :x2="width-16" :y1="tick.y" :y2="tick.y"/><text x="30" :y="tick.y+4" text-anchor="end">{{unit==='通过'?Math.round(tick.value*100)+'%':tick.value.toFixed(max<10?1:0)}}</text></g><path class="trend-line" :d="path"/><template v-for="(point,i) in points" :key="point.id"><circle v-if="point.value!==null&&point.tone!=='bad'&&point.tone!=='empty'" :cx="x(i)" :cy="y(point.value)" r="2.8" class="trend-point" :class="point.tone"/><path v-else-if="point.tone==='bad'" :d="`M${x(i)-3} 161l6 6m-6 0l6-6`" class="failure-mark"/></template><line v-if="hover!==null" :x1="x(hover)" :x2="x(hover)" y1="22" y2="167" class="hover-guide"/><text x="38" y="185">{{date(points[0]?.time)}}</text><text v-if="width>320&&points.length>1" :x="width-16" y="185" text-anchor="end">{{date(points[points.length-1]?.time)}}</text></svg><div class="chart-readout">{{hovered?.description||'悬浮查看记录；下方红叉表示失败'}}<strong v-if="hovered?.value!==null&&hovered?.value!==undefined&&hovered?.tone!=='bad'">{{unit==='通过'?Math.round(hovered.value*100)+'%':hovered.value.toFixed(unit==='MiB/s'?2:0)+' '+unit}}</strong></div></template></div></template>
