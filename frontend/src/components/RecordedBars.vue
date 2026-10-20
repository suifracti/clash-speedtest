<script setup lang="ts">
import {date} from '../domain'
import type {TrendPoint} from '../presentation'
const props=defineProps<{points:TrendPoint[];label:string;serviceId:string}>()
const value=(point:TrendPoint)=>point.attempts?.some(a=>(a.service_id||a.rule.service_id)===props.serviceId&&['challenge','suspected_challenge'].includes(a.result?.outcome||''))?null:point.value
const conclusion=(point:TrendPoint)=>value(point)===1?'可用':value(point)===0?'不可用':'无法判断'
</script>
<template>
 <span class="recorded-bars" :aria-label="label"><span v-for="point in points" :key="point.id" class="recorded-mark" :class="value(point)===1?'good':value(point)===0?'bad':'unknown'" :title="date(point.time)+' · '+conclusion(point)+' · '+point.description"/></span>
</template>
<style scoped>
.recorded-bars{display:flex;align-items:center;justify-content:flex-end;gap:3px;flex-wrap:wrap;min-width:0;max-width:177px}.recorded-mark{display:block;flex:0 0 12px;width:12px;height:12px;border-radius:2px;background:#89919c}.recorded-mark.good{background:#16864c}.recorded-mark.bad{background:#ce3d43}
</style>
