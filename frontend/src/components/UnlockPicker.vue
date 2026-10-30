<script setup lang="ts">
import {computed,ref} from 'vue'
import {useWorkspace} from '../workspace'
import type {ServiceRule} from '../domain'
import {unlockGroups,unlockGroup,unlockSupported} from '../unlock'
const props=defineProps<{modelValue:string[];catalog:ServiceRule[]}>(),emit=defineEmits<{'update:modelValue':[value:string[]]}>(),search=ref('')
const w=useWorkspace()
const selectable=computed(()=>props.catalog.filter(s=>!s.superseded_by&&unlockGroup(s.service_id)!=='legacy'))
const visible=computed(()=>selectable.value.filter(s=>!search.value||(s.name+' '+s.description).toLowerCase().includes(search.value.trim().toLowerCase())))
function toggle(id:string){emit('update:modelValue',props.modelValue.includes(id)?props.modelValue.filter(s=>s!==id):[...props.modelValue,id])}
function groupIds(id:string){return selectable.value.filter(s=>s.service_id!=='antigravity'&&unlockGroup(s.service_id)===id&&unlockSupported(s.service_id)).map(s=>s.service_id)}
function toggleGroup(id:string,checked:boolean){const ids=groupIds(id);emit('update:modelValue',checked?[...new Set([...props.modelValue,...ids])]:props.modelValue.filter(s=>!ids.includes(s)))}
</script>
<template><div class="service-picker unlock-picker">
 <div class="unlock-group-options"><label v-for="g in unlockGroups" :key="g.id"><input type="checkbox" :checked="groupIds(g.id).length>0&&groupIds(g.id).every(id=>modelValue.includes(id))" :indeterminate="groupIds(g.id).some(id=>modelValue.includes(id))&&!groupIds(g.id).every(id=>modelValue.includes(id))" @change="toggleGroup(g.id,($event.target as HTMLInputElement).checked)"><strong>{{g.name}}</strong><small>{{groupIds(g.id).length}} 项</small></label></div>
 <label class="unlock-region-toggle"><input v-model="w.skipByApplicableRegions.value" type="checkbox">按适用地区跳过</label>
 <details class="unlock-custom"><summary>展开自定义 <small>已选 {{modelValue.length}} 项</small></summary><input v-model="search" type="search" aria-label="搜索检测服务" placeholder="搜索解锁项目"><div class="service-picker-list"><label v-for="s in visible" :key="s.service_id" :class="{checked:modelValue.includes(s.service_id)}"><input type="checkbox" :checked="modelValue.includes(s.service_id)" :disabled="!unlockSupported(s.service_id)" @change="toggle(s.service_id)"><span><strong>{{s.name}}{{!unlockSupported(s.service_id)?' · 暂不支持':''}}</strong><small>{{s.description}}</small></span></label></div></details>
</div></template>
<style scoped>.unlock-region-toggle{display:flex;align-items:center;gap:7px;margin-top:12px;font-size:12px}.unlock-region-toggle input{width:15px;height:15px}.unlock-group-options{display:flex;gap:10px;flex-wrap:wrap}.unlock-group-options label{display:flex;align-items:center;gap:8px;padding:10px 18px;border:1px solid var(--line);border-radius:8px;flex:1}.unlock-group-options small,.unlock-custom small{color:var(--muted);font-size:11px}.unlock-group-options input{width:16px;height:16px}.unlock-custom{margin-top:12px}.unlock-custom summary{font-size:12px;cursor:pointer}.unlock-custom>input{margin:12px 0}</style>
