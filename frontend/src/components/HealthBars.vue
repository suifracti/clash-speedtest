<script setup lang="ts">
import {computed,nextTick,onBeforeUnmount,onMounted,ref} from 'vue'
import {triggerLabel} from '../measurementRounds'
import {observeVisibleArea} from '../visibleArea'
import type {TrendPoint} from '../presentation'
const props=defineProps<{points:TrendPoint[];label:string;aggregate?:boolean;defer?:boolean}>(),emit=defineEmits<{open:[point:TrendPoint];inspect:[point:TrendPoint];leave:[]}>()
const holder=ref<HTMLElement>(),capacity=ref(16),inView=ref(!props.defer||typeof IntersectionObserver==='undefined'),focused=ref(false)
const renderCells=computed(()=>!props.defer||inView.value||focused.value)
let observer:ResizeObserver|undefined,unobserve:(()=>void)|undefined
onMounted(()=>{if(holder.value&&props.defer)unobserve=observeVisibleArea(holder.value,visible=>inView.value=visible);if(typeof ResizeObserver!=='undefined'){observer=new ResizeObserver(([entry])=>capacity.value=Math.max(16,Math.min(80,Math.floor(entry.contentRect.width/8))));if(holder.value)observer.observe(holder.value)}})
onBeforeUnmount(()=>{observer?.disconnect();unobserve?.()})
const visible=computed(()=>{const last=props.points.slice(-capacity.value);return [...Array.from({length:capacity.value-last.length},()=>null),...last]})
function recorded(point:TrendPoint|null):point is TrendPoint{return !!point}
function layers(point:TrendPoint|null){
  if(point?.tone==='empty')return [{id:point.id,tone:'empty' as const,weight:1,description:point.description}]
  if(props.aggregate&&point?.samples&&point.samples.length>12)return (['good','warn','bad','empty'] as const).flatMap(tone=>{const count=point.samples!.filter(s=>s.tone===tone).length;return count?[{id:tone,tone,weight:count,description:count+' 项'+({good:'通过',warn:'受限',bad:'失败',empty:'未完成'}[tone])}]:[]})
  return point?.samples?.length?point.samples.map(s=>({...s,weight:1})):[{id:point?.id||'empty',tone:point?.tone||'empty',weight:1,description:point?.description||'没有记录'}]
}
const columns=computed(()=>visible.value.map((point,i)=>({point,i,layers:layers(point)})))
function focusIn(e:FocusEvent){focused.value=true;if(e.target===holder.value)void nextTick(()=>{if(document.activeElement===holder.value)holder.value?.querySelector<HTMLButtonElement>('button.has-record')?.focus({preventScroll:true})})}
function blur(e:FocusEvent){if(!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node|null)){focused.value=false;emit('leave')}}
</script>
<template><div ref="holder" class="health-bars" :tabindex="defer&&!renderCells?0:undefined" role="group" :aria-label="label+'历史状态，悬停查看本轮数据'" @mouseleave="emit('leave')" @focusout="blur" @focusin="focusIn"><template v-if="renderCells"><button v-for="{point,i,layers:segments} in columns" :key="point?.id||i" type="button" class="health-cell" :class="{'has-record':recorded(point),[`trigger-${point?.trigger||'unknown'}`]:recorded(point)}" :title="recorded(point)?triggerLabel(point.trigger)+' · '+point.description:undefined" :tabindex="recorded(point)?0:-1" :aria-disabled="!recorded(point)||undefined" :aria-label="recorded(point)?label+' · '+point.description:point?label+' · 暂无检测记录':'没有记录'" @mouseenter="recorded(point)?emit('inspect',point):emit('leave')" @focus="recorded(point)&&emit('inspect',point)" @click="recorded(point)&&emit('open',point)"><span class="health-column"><i v-for="sample in segments" :key="sample.id" class="health-segment" :class="sample.tone" :style="{flex:sample.weight}" :title="sample.description"/></span></button></template><span v-else class="health-cell health-placeholder" aria-hidden="true"/></div></template>
<style scoped>
.health-placeholder{pointer-events:none}

.health-cell.trigger-manual .health-column{outline:1px solid var(--blue);outline-offset:-1px}
.health-cell.trigger-scheduled .health-column{outline:1px dashed var(--ink);outline-offset:-1px}
</style>
