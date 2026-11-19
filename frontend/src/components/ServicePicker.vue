<script setup lang="ts">
import UiSelect from './UiSelect.vue'
import {computed,ref} from 'vue'
import type {ServiceRule} from '../domain'
import Icon from './Icon.vue'
const props=defineProps<{modelValue:string[];catalog:ServiceRule[]}>(),emit=defineEmits<{ 'update:modelValue':[value:string[]] }>(),search=ref(''),category=ref('')
const selectable=computed(()=>props.catalog.filter(s=>!s.superseded_by&&!s.service_id.startsWith('ipok_')))
function groupNameFor(s:ServiceRule){
 const id=s.service_id.toLowerCase()
 if(id==='google_region'||id==='cloudflare_challenge')return '地区与人机验证'
 if(s.result_kind==='exit_profile'||['cloudflare_trace','exit_ipv4','exit_ipv6'].includes(id))return '出口地址与机房'
 if(s.result_kind==='ip_quality'||id.startsWith('ping0_')||id.startsWith('ippure_'))return 'IP 风险与纯净度'
 return s.category||'其他检测'
}
const groupRank=(name:string)=>({'出口地址与机房':0,'IP 风险与纯净度':1,'地区与人机验证':2} as Record<string,number>)[name]??9
const categories=computed(()=>[...new Set(selectable.value.map(groupNameFor))].sort((a,b)=>groupRank(a)-groupRank(b)))
const visible=computed(()=>selectable.value.filter(s=>(!category.value||groupNameFor(s)===category.value)&&(!search.value||(s.name+' '+s.description+' '+groupNameFor(s)).toLowerCase().includes(search.value.trim().toLowerCase()))))
const groupedVisible=computed(()=>{
 const map=new Map<string,ServiceRule[]>()
 for(const s of visible.value){
  const name=groupNameFor(s)
  if(!map.has(name))map.set(name,[])
  map.get(name)!.push(s)
 }
 return [...map.entries()].sort((a,b)=>groupRank(a[0])-groupRank(b[0])).map(([name,items])=>({name,items}))
})
function toggle(id:string){emit('update:modelValue',props.modelValue.includes(id)?props.modelValue.filter(s=>s!==id):[...props.modelValue,id])}
function toggleCategory(items:ServiceRule[]){
 const ids=items.map(s=>s.service_id),allChecked=ids.length>0&&ids.every(id=>props.modelValue.includes(id))
 emit('update:modelValue',allChecked?props.modelValue.filter(id=>!ids.includes(id)):[...new Set([...props.modelValue,...ids])])
}
function selectVisible(){emit('update:modelValue',[...new Set([...props.modelValue,...visible.value.filter(s=>s.service_id!=='antigravity').map(s=>s.service_id)])])}
function clearVisible(){const ids=new Set(visible.value.map(s=>s.service_id));emit('update:modelValue',props.modelValue.filter(id=>!ids.has(id)))}
</script>
<template>
  <div class="service-picker">
    <div class="service-picker-tools">
      <label class="search-field"><Icon name="search"/><input v-model="search" type="search" aria-label="搜索检测服务" placeholder="搜索服务"></label>
      <UiSelect v-model="category" aria-label="服务分类"><option value="">全部分类</option><option v-for="c in categories" :key="c" :value="c">{{c}}</option></UiSelect>
      <button type="button" class="text-button" :disabled="!visible.length" @click="selectVisible">全选筛选项</button>
      <button type="button" class="text-button" :disabled="!visible.length" @click="clearVisible">取消筛选项</button>
      <span class="small muted">已选 {{modelValue.length}} 项</span>
    </div>
    <div class="service-picker-list is-grouped">
      <section v-for="group in groupedVisible" :key="group.name" class="service-picker-group">
        <header class="service-picker-group-head">
          <h4>{{group.name}} <small>{{group.items.filter(s=>modelValue.includes(s.service_id)).length}}/{{group.items.length}}</small></h4>
          <button type="button" class="text-button" @click="toggleCategory(group.items)">{{group.items.every(s=>modelValue.includes(s.service_id))?'取消本组':'全选本组'}}</button>
        </header>
        <div class="service-picker-grid">
          <label v-for="s in group.items" :key="s.service_id" :class="{checked:modelValue.includes(s.service_id)}" :title="s.description">
            <input type="checkbox" :checked="modelValue.includes(s.service_id)" @change="toggle(s.service_id)">
            <span><strong>{{s.name}}</strong><small>{{s.description}}</small></span>
          </label>
        </div>
      </section>
      <p v-if="!visible.length" class="muted">没有匹配的服务。</p>
    </div>
  </div>
</template>
<style scoped>
.service-picker-list.is-grouped{display:flex;flex-direction:column;gap:14px;max-height:320px;overflow:auto;padding:2px 4px}
.service-picker-group{display:grid;gap:8px}
.service-picker-group-head{display:flex;align-items:center;justify-content:space-between;gap:8px;padding-bottom:4px;border-bottom:1px solid var(--cst-line,var(--line))}
.service-picker-group-head h4{margin:0;font-size:12px;font-weight:700;color:var(--cst-text,var(--ink));display:inline-flex;align-items:center;gap:6px}
.service-picker-group-head small{font-weight:500;color:var(--cst-muted,var(--muted));font-size:11px}
.service-picker-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:8px}
.service-picker-grid>label{display:flex;align-items:flex-start;gap:9px;min-width:0;padding:10px 12px;border:1px solid var(--cst-line,var(--line));border-radius:10px;background:var(--cst-panel,var(--surface));cursor:pointer;transition:border-color .14s,background .14s}
.service-picker-grid>label:hover{border-color:var(--cst-accent,var(--blue))}
.service-picker-grid>label.checked{background:var(--cst-accent-soft,var(--blue-soft));border-color:color-mix(in srgb,var(--cst-accent,var(--blue)) 38%,var(--cst-line,var(--line)))}
.service-picker-grid>label>input{width:15px;height:15px;margin-top:2px;flex:none}
.service-picker-grid>label>span{min-width:0;display:grid;gap:3px}
.service-picker-grid>label strong{font-size:12.5px;line-height:1.35;color:var(--cst-text,var(--ink))}
.service-picker-grid>label small{font-size:11px;line-height:1.45;color:var(--cst-muted,var(--muted));display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
</style>
