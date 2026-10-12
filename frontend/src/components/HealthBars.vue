<script setup lang="ts">
import {computed,nextTick,onBeforeUnmount,onMounted,ref} from 'vue'
import {latencyTargetId} from '../latencyTargets'
import {attemptTone} from '../measurementRounds'
import {triggerLabel} from '../measurementRounds'
import {observeVisibleArea} from '../visibleArea'
import type {TrendPoint} from '../presentation'
const props=defineProps<{points:TrendPoint[];label:string;aggregate?:boolean;defer?:boolean;sites?:{id:string;name:string;match:string}[];serviceIds?:string[]}>(),emit=defineEmits<{open:[point:TrendPoint];inspect:[point:TrendPoint];leave:[]}>()
const holder=ref<HTMLElement>(),capacity=ref(16),inView=ref(!props.defer||typeof IntersectionObserver==='undefined'),focused=ref(false)
const renderCells=computed(()=>!props.defer||inView.value||focused.value)
let observer:ResizeObserver|undefined,unobserve:(()=>void)|undefined
onMounted(()=>{if(holder.value&&props.defer)unobserve=observeVisibleArea(holder.value,visible=>inView.value=visible);if(typeof ResizeObserver!=='undefined'){observer=new ResizeObserver(([entry])=>capacity.value=Math.max(16,Math.min(80,Math.floor(entry.contentRect.width/8))));if(holder.value)observer.observe(holder.value)}})
onBeforeUnmount(()=>{observer?.disconnect();unobserve?.()})
function move(e:KeyboardEvent){if(!['ArrowLeft','ArrowRight','Home','End'].includes(e.key))return;const buttons=Array.from(holder.value?.querySelectorAll<HTMLButtonElement>('button.has-record')||[]),index=buttons.indexOf(e.target as HTMLButtonElement);if(index<0)return;e.preventDefault();buttons[e.key==='Home'?0:e.key==='End'?buttons.length-1:Math.max(0,Math.min(buttons.length-1,index+(e.key==='ArrowLeft'?-1:1)))]?.focus({preventScroll:true})}
const visible=computed(()=>{const last=props.points.slice(-capacity.value);return [...Array.from({length:capacity.value-last.length},()=>null),...last]})
function recorded(point:TrendPoint|null):point is TrendPoint{return !!point}
function layers(point:TrendPoint|null){
  if(props.sites?.length){const known=props.sites.map(site=>{const samples=point?.sampleGroups?.filter(g=>latencyTargetId(g.target)===site.id).flatMap(g=>g.samples)||[];const tone=!samples.length||samples.every(s=>s.tone==='empty')?'empty':samples.every(s=>s.tone==='good')?'good':samples.every(s=>s.tone==='bad')?'bad':'warn';return {id:site.id,tone,weight:1,description:site.name+' · '+samples.length+' 个子样本'}});const unknown=point?.sampleGroups?.filter(g=>!latencyTargetId(g.target))||[];return [...known,...unknown.map(g=>({id:g.id,tone:g.samples.every(s=>s.tone==='empty')?'empty':g.samples.every(s=>s.tone==='good')?'good':g.samples.every(s=>s.tone==='bad')?'bad':'warn',weight:1,description:'旧/未知目标 · '+g.target}))]}
  if(props.serviceIds?.length)return props.serviceIds.map(id=>{const a=point?.attempts?.find(a=>a.service_id===id);return {id,tone:a?attemptTone(a):'empty',weight:1,description:a?.rule.name||id}})

  if(point?.tone==='empty')return [{id:point.id,tone:'empty' as const,weight:1,description:point.description}]
  if(props.aggregate&&point?.samples&&point.samples.length>12)return (['good','warn','bad','empty'] as const).flatMap(tone=>{const count=point.samples!.filter(s=>s.tone===tone).length;return count?[{id:tone,tone,weight:count,description:count+' 项'+({good:'通过',warn:'受限',bad:'失败',empty:'未完成'}[tone])}]:[]})
  return point?.samples?.length?point.samples.map(s=>({...s,weight:1})):[{id:point?.id||'empty',tone:point?.tone||'empty',weight:1,description:point?.description||'没有记录'}]
}
const columns=computed(()=>visible.value.map((point,i)=>({point,i,layers:layers(point)})))
function focusIn(e:FocusEvent){focused.value=true;if(e.target===holder.value)void nextTick(()=>{if(document.activeElement===holder.value)holder.value?.querySelector<HTMLButtonElement>('button.has-record')?.focus({preventScroll:true})})}
function blur(e:FocusEvent){if(!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node|null)){focused.value=false;emit('leave')}}
</script>
<template><div ref="holder" class="health-bars" :tabindex="defer&&!renderCells?0:undefined" role="group" :aria-label="label+'历史状态，悬停查看本轮数据'" @keydown="move" @mouseleave="emit('leave')" @focusout="blur" @focusin="focusIn"><template v-if="renderCells"><button v-for="{point,i,layers:segments} in columns" :key="point?.id||i" type="button" class="health-cell" :class="{'has-record':recorded(point),[`trigger-${point?.trigger||'unknown'}`]:recorded(point)}" :title="recorded(point)?triggerLabel(point.trigger)+' · '+point.description:undefined" :tabindex="recorded(point)?0:-1" :aria-disabled="!recorded(point)||undefined" :aria-label="recorded(point)?label+' · '+point.description:point?label+' · 暂无检测记录':'没有记录'" @mouseenter="recorded(point)?emit('inspect',point):emit('leave')" @focus="recorded(point)&&emit('inspect',point)" @click="($event.currentTarget as HTMLElement).focus({preventScroll:true});recorded(point)&&emit('open',point)"><span class="health-column"><i v-for="sample in segments" :key="sample.id" class="health-segment" :class="sample.tone" :style="{flex:sample.weight}" :title="sample.description"/></span></button></template><span v-else class="health-cell health-placeholder" aria-hidden="true"/></div></template>
<style scoped>
.health-placeholder{pointer-events:none}

.health-cell.trigger-manual .health-column{border-bottom:2px solid var(--blue)}
.health-cell.trigger-scheduled .health-column{border-bottom:2px dashed var(--ink)}
</style>
