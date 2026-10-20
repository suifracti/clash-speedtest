<script setup lang="ts">
import {date} from '../domain'
import type {TrendPoint} from '../presentation'
defineProps<{points:TrendPoint[];label:string}>()
const conclusion=(point:TrendPoint)=>point.value===1?'可用':point.value===0?'不可用':'无法判断'
</script>
<template>
 <span class="recorded-bars" :aria-label="label"><span v-for="point in points" :key="point.id" class="recorded-mark" :class="point.value===1?'good':point.value===0?'bad':'unknown'" :title="date(point.time)+' · '+conclusion(point)+' · '+point.description"/></span>
</template>
<style scoped>
.recorded-bars{display:flex;align-items:center;justify-content:flex-end;gap:3px;flex-wrap:wrap;min-width:0;max-width:177px}.recorded-mark{display:block;flex:0 0 12px;width:12px;height:12px;border-radius:2px;background:#89919c}.recorded-mark.good{background:#16864c}.recorded-mark.bad{background:#ce3d43}
</style>
