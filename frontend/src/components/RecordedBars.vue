<script setup lang="ts">
import {date} from '../domain'
import {unlockResult} from '../unlock'
import type {TrendPoint} from '../presentation'
const props=defineProps<{points:TrendPoint[];label:string;serviceId:string}>()
const value=(point:TrendPoint)=>{const attempt=point.attempts?.find(a=>(a.service_id||a.rule.service_id)===props.serviceId);const state=unlockResult(attempt).state;return state==='available'?1:state==='unavailable'?0:state==='partial'?2:state==='verification'?3:null}
const conclusion=(point:TrendPoint)=>{const result=unlockResult(point.attempts?.find(a=>(a.service_id||a.rule.service_id)===props.serviceId));return result.state==='available'?'可用':result.label}
</script>
<template>
 <span class="recorded-bars" :aria-label="label"><span v-for="point in points" :key="point.id" class="recorded-mark" :class="value(point)===1?'good':value(point)===0?'bad':value(point)===2||value(point)===3?'partial':'unknown'" :title="date(point.time)+' · '+conclusion(point)+' · '+point.description"/></span>
</template>
<style scoped>
.recorded-bars{display:flex;align-items:center;justify-content:flex-end;gap:3px;flex-wrap:wrap;min-width:0;max-width:177px}.recorded-mark{display:block;flex:0 0 12px;width:12px;height:12px;border-radius:2px;background:#89919c}.recorded-mark.good{background:#16864c}.recorded-mark.bad{background:#ce3d43}.recorded-mark.partial{background:#bd811c}
</style>
