<script setup lang="ts">
import {nextTick,onMounted,onBeforeUnmount,ref,useId} from 'vue'
import Icon from './Icon.vue'
const props=withDefaults(defineProps<{title:string;wide?:boolean;closable?:boolean;variant?:string}>(),{closable:true});const emit=defineEmits<{close:[]}>();const dialog=ref<HTMLDialogElement>();const titleId=useId();let previous:HTMLElement|null=null
const focusable='a[href],button:not([disabled]),input:not([disabled]):not([type=hidden]),select:not([disabled]),textarea:not([disabled]),summary,[tabindex]:not([tabindex="-1"])'
function visible(el:HTMLElement){return !el.closest('[hidden],[inert]')&&el.getClientRects().length>0}
function tabbables(){return Array.from(dialog.value?.querySelectorAll<HTMLElement>(focusable)||[]).filter(visible)}
// WAI-ARIA dialog：打开后焦点进入正文（优先 autofocus），Tab 在弹窗内循环，Esc 关闭，关闭后回到触发处。
function initialFocus(){
 const d=dialog.value;if(!d||d.contains(document.activeElement)&&document.activeElement!==d.querySelector('.modal-header button'))return
 const target=d.querySelector<HTMLElement>('[autofocus]')||tabbables().find(el=>!el.closest('.modal-header'))||tabbables()[0]
 target?.focus({preventScroll:true})
}
function trap(e:KeyboardEvent){
 if(e.key!=='Tab')return
 const list=tabbables();if(!list.length)return
 const first=list[0],last=list[list.length-1],current=document.activeElement
 if(e.shiftKey&&(current===first||!dialog.value?.contains(current))){e.preventDefault();last.focus()}
 else if(!e.shiftKey&&current===last){e.preventDefault();first.focus()}
}
onMounted(()=>{previous=document.activeElement as HTMLElement;dialog.value?.showModal();void nextTick(initialFocus)});onBeforeUnmount(()=>{dialog.value?.close();if(previous?.isConnected)previous.focus({preventScroll:true})})
</script>
<template><Teleport to="body"><dialog ref="dialog" class="modal" :class="[{wide},variant]" :aria-labelledby="titleId" @keydown="trap" @cancel.prevent="()=>{if(props.closable)emit('close')}" @click="e=>{if(e.target===dialog&&props.closable)emit('close')}"><header class="modal-header"><h2 :id="titleId">{{title}}</h2><button v-if="closable" class="button ghost icon-button" type="button" aria-label="关闭" @click="emit('close')"><Icon name="close"/></button></header><div class="modal-body"><slot/></div><footer v-if="$slots.footer" class="modal-footer"><slot name="footer"/></footer></dialog></Teleport></template>
