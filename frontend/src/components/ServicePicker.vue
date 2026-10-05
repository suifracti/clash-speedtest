<script setup lang="ts">
import UiSelect from './UiSelect.vue'
import {computed,ref} from 'vue'
import type {ServiceRule} from '../domain'
import Icon from './Icon.vue'
const props=defineProps<{modelValue:string[];catalog:ServiceRule[]}>(),emit=defineEmits<{ 'update:modelValue':[value:string[]] }>(),search=ref(''),category=ref('')
const categories=computed(()=>[...new Set(props.catalog.map(s=>s.category).filter(Boolean))])
const visible=computed(()=>props.catalog.filter(s=>(!category.value||s.category===category.value)&&(!search.value||(s.name+' '+s.description).toLowerCase().includes(search.value.trim().toLowerCase()))))
function toggle(id:string){emit('update:modelValue',props.modelValue.includes(id)?props.modelValue.filter(s=>s!==id):[...props.modelValue,id])}
function selectVisible(){emit('update:modelValue',[...new Set([...props.modelValue,...visible.value.map(s=>s.service_id)])])}
function clearVisible(){const ids=new Set(visible.value.map(s=>s.service_id));emit('update:modelValue',props.modelValue.filter(id=>!ids.has(id)))}
</script>
<template><div class="service-picker"><div class="service-picker-tools"><label class="search-field"><Icon name="search"/><input v-model="search" type="search" aria-label="搜索检测服务" placeholder="搜索服务"></label><UiSelect v-model="category" aria-label="服务分类"><option value="">全部分类</option><option v-for="c in categories" :key="c" :value="c">{{c}}</option></UiSelect><button type="button" class="text-button" :disabled="!visible.length" @click="selectVisible">全选筛选项</button><button type="button" class="text-button" :disabled="!visible.length" @click="clearVisible">取消筛选项</button><span class="small muted">已选 {{modelValue.length}} 项</span></div><div class="service-picker-list"><label v-for="s in visible" :key="s.service_id" :class="{checked:modelValue.includes(s.service_id)}"><input type="checkbox" :checked="modelValue.includes(s.service_id)" @change="toggle(s.service_id)"><span><strong>{{s.name}}</strong><small>{{s.description}}</small></span></label><p v-if="!visible.length" class="muted">没有匹配的服务。</p></div></div></template>
