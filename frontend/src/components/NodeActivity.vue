<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref,watch} from 'vue'
import {observeVisibleArea} from '../visibleArea'
const props=defineProps<{state?:string;label?:string}>()
const root=ref<HTMLElement>(),visible=ref(false),arrived=ref(false)
let release:(()=>void)|undefined,timer:ReturnType<typeof setTimeout>|undefined
const active=computed(()=>['running','cancelling','saving'].includes(props.state||''))
watch(()=>props.state,(next,old)=>{if(old&&['running','saving','cancelling'].includes(old)&&next&&!['running','saving','cancelling','queued'].includes(next)){arrived.value=true;clearTimeout(timer);timer=setTimeout(()=>arrived.value=false,650)}})
onMounted(()=>{if(root.value)release=observeVisibleArea(root.value,v=>visible.value=v)})
onBeforeUnmount(()=>{release?.();clearTimeout(timer)})
</script>
<template><div v-show="state" ref="root" class="node-activity" :class="{active:active&&visible,arrived}" :data-state="state"><span aria-hidden="true"/><small>{{label}}</small></div></template>
