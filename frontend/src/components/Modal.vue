<script setup lang="ts">
import {onMounted,onBeforeUnmount,ref,useId} from 'vue'
import Icon from './Icon.vue'
const props=withDefaults(defineProps<{title:string;wide?:boolean;closable?:boolean}>(),{closable:true});const emit=defineEmits<{close:[]}>();const dialog=ref<HTMLDialogElement>();const titleId=useId();let previous:HTMLElement|null=null
onMounted(()=>{previous=document.activeElement as HTMLElement;dialog.value?.showModal()});onBeforeUnmount(()=>{dialog.value?.close();if(previous?.isConnected)previous.focus()})
</script>
<template><Teleport to="body"><dialog ref="dialog" class="modal" :class="{wide}" :aria-labelledby="titleId" @cancel.prevent="()=>{if(props.closable)emit('close')}" @click="e=>{if(e.target===dialog&&props.closable)emit('close')}"><header class="modal-header"><h2 :id="titleId">{{title}}</h2><button v-if="closable" class="button ghost icon-button" type="button" aria-label="关闭" @click="emit('close')"><Icon name="close"/></button></header><div class="modal-body"><slot/></div><footer v-if="$slots.footer" class="modal-footer"><slot name="footer"/></footer></dialog></Teleport></template>
