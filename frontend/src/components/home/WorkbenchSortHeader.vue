<script setup lang="ts">
import {computed,inject} from 'vue'
import {homeModelKey} from './homeModel'
const props=defineProps<{column:string;label:string}>()
const m=inject(homeModelKey)!
const active=computed(()=>m.headerSort.value===props.column)
</script>

<template>
 <span role="columnheader" :aria-sort="active&&m.headerSortReady.value?'ascending':'none'">
  <button type="button" class="hd-column-sort" :class="{ 'history-sort': column==='responseRate'||column==='volatility' }"
   :aria-pressed="active" :title="active?'再点恢复原来的顺序':'按'+label+'正序排列，未测放最后'"
   @click="m.sortHeader(column)">{{label}}<span v-if="active" aria-hidden="true">{{m.headerSortReady.value?'↑':'…'}}</span></button>
 </span>
</template>
