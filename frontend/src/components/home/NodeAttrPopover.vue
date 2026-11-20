<script setup lang="ts">
import {shallowRef} from 'vue'
export interface AttrTipState {term:string;short:string;detail:string;x:number;y:number}
const tip=shallowRef<AttrTipState|null>(null)
function show(next:AttrTipState){tip.value=next}
function clear(){if(tip.value)tip.value=null}
defineExpose({show,clear})
</script>
<template>
  <div v-if="tip" class="hd-attr-popover" role="tooltip" :style="{left:tip.x+'px',top:tip.y+'px'}"><strong>{{tip.term}} · {{tip.short}}</strong><small>{{tip.detail}}</small></div>
</template>
