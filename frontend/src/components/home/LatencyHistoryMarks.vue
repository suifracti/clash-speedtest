<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,shallowRef} from 'vue'
import {date} from '../../domain'
import {latencyPointLabel,type TrendPoint} from '../../presentation'

const props=defineProps<{points:TrendPoint[];label:string;target:string;selected?:TrendPoint}>()
const emit=defineEmits<{select:[point:TrendPoint]}>()
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
const tip=shallowRef<{point:TrendPoint;reading:string;x:number;y:number;below:boolean}|null>(null)
function showTip(e:MouseEvent|FocusEvent,point:TrendPoint){
 const el=e.currentTarget as HTMLElement|null
 if(!el)return
 const rect=el.getBoundingClientRect()
 const vw=typeof window!=='undefined'?window.innerWidth:1280
 const x=Math.max(115,Math.min(vw-115,rect.left+rect.width/2))
 const below=rect.top<90
 const reading=props.target==='all'?'整轮历史（点击回看）':latencyPointLabel(point,props.target)
 tip.value={point,reading,x,y:below?rect.bottom:rect.top,below}
}
function hideTip(){tip.value=null}
onMounted(()=>window.addEventListener('scroll',hideTip,true))
onBeforeUnmount(()=>window.removeEventListener('scroll',hideTip,true))
</script>

<template>
 <span v-if="points.length" class="response-history" role="group" :aria-label="label" @mouseleave="hideTip">
  <button v-for="(point,index) in points" :key="point.source+':'+point.id" type="button"
   :aria-pressed="index===selectedIndex" :tabindex="index===(selectedIndex<0?points.length-1:selectedIndex)?0:-1"
   :class="[point.tone,{monitor:point.source==='monitor'}]" :title="labelFor(point)" :aria-label="label+' · '+labelFor(point)"
   @mouseenter="showTip($event,point)" @focus="showTip($event,point)" @blur="hideTip"
   @keydown.left.stop.prevent="move($event,true)" @keydown.right.stop.prevent="move($event,false)" @click.stop="hideTip();emit('select',point)"/>
  <Teleport to="body">
   <div v-if="tip" class="cst-chart-tip" :class="{below:tip.below}" role="tooltip" :style="{left:tip.x+'px',top:tip.y+'px'}">
    <div class="cst-chart-tip-head"><span>{{date(tip.point.time)}}</span><span>{{tip.point.source==='monitor'?'持续监测':'手动检测'}}</span></div>
    <strong class="cst-chart-tip-val" :class="{unknown:tip.point.tone==='empty'}">{{tip.reading}}</strong>
   </div>
  </Teleport>
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
