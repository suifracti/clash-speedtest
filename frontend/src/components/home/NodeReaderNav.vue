<script setup lang="ts">
defineProps<{index:number;count:number;selectedCount:number;previous?:string;next?:string}>()
defineEmits<{previous:[];next:[];compare:[]}>()
</script>
<template>
  <nav class="hd-reader-nav" aria-label="连续查看节点">
    <span title="打开详情时的筛选范围与顺序">本次浏览 <b aria-live="polite">{{index+1}} / {{count}}</b></span>
    <button class="hd-btn" type="button" aria-label="查看上一条节点" :title="previous" :disabled="index<=0" @click="$emit('previous')">‹ 上一条</button>
    <button class="hd-btn" type="button" aria-label="查看下一条节点" :title="next" :disabled="index<0||index>=count-1" @click="$emit('next')">下一条 ›</button>
    <button v-if="selectedCount" class="hd-btn hd-reader-compare" type="button" :disabled="selectedCount<2" title="至少选择两个节点后比较" @click="$emit('compare')">比较已选 {{selectedCount}}</button>
  </nav>
</template>
