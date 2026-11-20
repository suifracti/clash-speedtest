<script setup lang="ts">
import {computed} from 'vue'
import {date,type NodeOption} from '../../domain'
import {latencyPointLabel,type TrendPoint} from '../../presentation'
import {hideChartTip} from '../../chartTip'

const props=defineProps<{points:TrendPoint[];label:string;target:string;selected?:TrendPoint;node?:NodeOption}>()
const emit=defineEmits<{select:[point:TrendPoint,target:string,node?:NodeOption]}>()
const selectedIndex=computed(()=>props.points.findIndex(p=>p.id===props.selected?.id&&p.source===props.selected?.source))
function labelFor(point:TrendPoint){
 const reading=props.target==='all'?'整轮':latencyPointLabel(point,props.target)
 return `${date(point.time)} · ${point.source==='monitor'?'持续监测':'手动检测'} · ${reading}`
}
function move(event:KeyboardEvent,previous:boolean){
 const button=event.currentTarget as HTMLButtonElement
 const sibling=(previous?button.previousElementSibling:button.nextElementSibling) as HTMLButtonElement|null
 sibling?.focus()
}
</script>

<template>
 <span v-if="points.length" class="response-history" role="group" :aria-label="label">
  <button v-for="(point,index) in points" :key="point.source+':'+point.id" type="button"
   :aria-pressed="index===selectedIndex" :tabindex="index===(selectedIndex<0?points.length-1:selectedIndex)?0:-1"
   :class="[point.tone,{monitor:point.source==='monitor'}]" :aria-label="label+' · '+labelFor(point)"
   @keydown.left.stop.prevent="move($event,true)" @keydown.right.stop.prevent="move($event,false)" @click.stop="hideChartTip();emit('select',point,target,node)"/>
 </span>
</template>

<style scoped>
.response-history{display:flex;gap:2px;width:100%;align-items:center}
.response-history button{position:relative;padding:0;flex:1 1 0;min-width:0;max-width:14px;height:18px;border:0;border-radius:2px;background:none;cursor:pointer}
.response-history button::before{content:"";position:absolute;inset:3px 1px;border:1px solid transparent;border-radius:2px;background:var(--hd-good)}
.response-history button.bad::before{background:var(--hd-bad)}
.response-history button.warn::before{background:var(--hd-warn)}
.response-history button.empty::before{background:var(--hd-panel-2);border-color:var(--hd-line)}
.response-history button.monitor::before{border-color:var(--hd-text);border-style:dotted}
.response-history button:hover,.response-history button:focus-visible,.response-history button[aria-pressed=true]{outline:2px solid var(--hd-accent);outline-offset:1px}
</style>
