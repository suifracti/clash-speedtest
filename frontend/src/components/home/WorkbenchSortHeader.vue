<script setup lang="ts">
import {computed,inject} from 'vue'
import {homeModelKey} from './homeModel'
const props=defineProps<{column:string;label:string}>()
const m=inject(homeModelKey)!
const active=computed(()=>m.headerSort.value===props.column)
const dir=computed(()=>m.headerSortDirection(props.column))
</script>

<template>
 <span role="columnheader" :aria-sort="active&&m.headerSortReady.value?dir:'none'">
  <button type="button" class="hd-column-sort" :class="{ 'history-sort': column==='responseRate'||column==='volatility' }"
   :aria-pressed="active" :title="active?'恢复原来的排序':'按'+label+(dir==='descending'?'从高到低':'从低到高')+'排列，未测置后'"
   @click="m.sortHeader(column)">{{label}}<span v-if="active" aria-hidden="true">{{m.headerSortReady.value?(dir==='descending'?'↓':'↑'):'…'}}</span></button>
 </span>
</template>
