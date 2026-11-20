<script setup lang="ts">
import {hoverTip,hoverTipHost} from '../hoverTip'
</script>
<template>
 <Teleport :to="hoverTipHost||'body'"><div v-if="hoverTip.visible" class="global-hover-tip" role="tooltip" :style="{left:hoverTip.x+'px',top:hoverTip.y+'px'}">{{hoverTip.text}}</div></Teleport>
</template>
<style scoped>
.global-hover-tip{position:fixed;z-index:10000;max-width:348px;padding:8px 11px;border-radius:10px;border:1px solid var(--cst-line-strong,#d1d9e6);border-top:2px solid transparent;background-image:linear-gradient(var(--cst-panel-solid,#ffffff),var(--cst-panel-solid,#ffffff)),var(--cst-aurora-ribbon,linear-gradient(90deg,#6366f1,#38bdf8,#a855f7));background-origin:border-box;background-clip:padding-box,border-box;color:var(--cst-text,#0f172a);font-size:11.5px;font-weight:500;line-height:1.55;white-space:pre-line;pointer-events:none;box-shadow:var(--cst-shadow-pop,0 12px 32px -8px rgba(15,23,42,.18))}
</style>
