<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref} from 'vue'
import type {TrendPoint} from '../presentation'
const props=defineProps<{points:TrendPoint[];label:string;aggregate?:boolean}>(),emit=defineEmits<{open:[point:TrendPoint];inspect:[point:TrendPoint];leave:[]}>()
const holder=ref<HTMLElement>(),capacity=ref(16);let observer:ResizeObserver|undefined
onMounted(()=>{if(typeof ResizeObserver==='undefined')return;observer=new ResizeObserver(([entry])=>capacity.value=Math.max(16,Math.min(80,Math.floor(entry.contentRect.width/8))));if(holder.value)observer.observe(holder.value)})
onBeforeUnmount(()=>observer?.disconnect())
const visible=computed(()=>{const last=props.points.slice(-capacity.value);return [...Array.from({length:capacity.value-last.length},()=>null),...last]})
function recorded(point:TrendPoint|null):point is TrendPoint{return !!point&&point.tone!=='empty'}
function layers(point:TrendPoint|null){
  if(point?.tone==='empty')return [{id:point.id,tone:'empty' as const,weight:1,description:'暂无检测记录'}]
  if(props.aggregate&&point?.samples&&point.samples.length>12)return (['good','warn','bad','empty'] as const).flatMap(tone=>{const count=point.samples!.filter(s=>s.tone===tone).length;return count?[{id:tone,tone,weight:count,description:count+' 项'+({good:'通过',warn:'受限',bad:'失败',empty:'未完成'}[tone])}]:[]})
  return point?.samples?.length?point.samples.map(s=>({...s,weight:1})):[{id:point?.id||'empty',tone:point?.tone||'empty',weight:1,description:point?.description||'没有记录'}]
}
function blur(e:FocusEvent){if(!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node|null))emit('leave')}
</script>
<template><div ref="holder" class="health-bars" role="group" :aria-label="label+'历史状态，悬停查看本轮数据'" @mouseleave="emit('leave')" @focusout="blur"><button v-for="(point,i) in visible" :key="point?.id||i" type="button" class="health-cell" :class="{'has-record':recorded(point)}" :tabindex="recorded(point)?0:-1" :aria-disabled="!recorded(point)||undefined" :aria-label="recorded(point)?label+' · '+point.description:point?label+' · 暂无检测记录':'没有记录'" @mouseenter="recorded(point)?emit('inspect',point):emit('leave')" @focus="recorded(point)&&emit('inspect',point)" @click="recorded(point)&&emit('open',point)"><span class="health-column"><i v-for="sample in layers(point)" :key="sample.id" class="health-segment" :class="sample.tone" :style="{flex:sample.weight}" :title="sample.description"/></span></button></div></template>
