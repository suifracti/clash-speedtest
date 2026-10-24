<script setup lang="ts">
import {hoverTip} from '../hoverTip'
</script>
<template>
 <Teleport to="body"><div v-if="hoverTip.visible" class="global-hover-tip" role="tooltip" :style="{left:hoverTip.x+'px',top:hoverTip.y+'px'}">{{hoverTip.text}}</div></Teleport>
</template>
<style scoped>
.global-hover-tip{position:fixed;z-index:10000;max-width:340px;padding:6px 8px;border-radius:5px;background:#1f2328;color:#fff;font-size:11px;line-height:1.55;white-space:pre-line;pointer-events:none;box-shadow:0 4px 14px rgba(0,0,0,.22)}
</style>
