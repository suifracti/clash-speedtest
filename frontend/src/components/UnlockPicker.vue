<script setup lang="ts">
import {computed,ref} from 'vue'
import {useWorkspace} from '../workspace'
import type {ServiceRule} from '../domain'
import {unlockGroups,unlockGroup,unlockSupported} from '../unlock'
const props=defineProps<{modelValue:string[];catalog:ServiceRule[]}>(),emit=defineEmits<{'update:modelValue':[value:string[]]}>(),search=ref(''),customName=ref(''),customOpen=ref(false),editingGroupId=ref('')
const w=useWorkspace()
const customKey='cst.custom-unlock-groups',groupsKey='cst.unlock-groups.v1'
interface EditableUnlockGroup{id:string;name:string;ids:string[];custom?:boolean}
const selectable=computed(()=>props.catalog.filter(s=>!s.superseded_by&&unlockGroup(s.service_id)!=='legacy'))
function defaultPresetGroups():EditableUnlockGroup[]{
 return unlockGroups.map(g=>({id:g.id,name:g.name,ids:g.ids.filter(id=>id!=='antigravity'&&unlockSupported(id)),custom:false}))
}
function loadGroups():EditableUnlockGroup[]{
 try{
  const raw=window.localStorage.getItem(groupsKey)
  if(raw){
   const parsed=JSON.parse(raw)
   if(Array.isArray(parsed))return parsed.filter((g):g is EditableUnlockGroup=>!!g&&typeof g.id==='string'&&typeof g.name==='string'&&Array.isArray(g.ids))
  }
  const legacyRaw=window.localStorage.getItem(customKey),legacy=legacyRaw?JSON.parse(legacyRaw):[]
  const extras=Array.isArray(legacy)?legacy.filter((g):g is EditableUnlockGroup=>!!g&&typeof g.id==='string'&&typeof g.name==='string'&&Array.isArray(g.ids)).map(g=>({...g,custom:true})):[]
  return [...defaultPresetGroups(),...extras]
 }catch{return defaultPresetGroups()}
}
const groups=ref<EditableUnlockGroup[]>(loadGroups())
function persistGroups(){
 try{
  window.localStorage.setItem(groupsKey,JSON.stringify(groups.value))
  window.localStorage.setItem(customKey,JSON.stringify(groups.value.filter(g=>g.custom)))
 }catch{}
}
const visible=computed(()=>selectable.value.filter(s=>!search.value||(s.name+' '+s.description).toLowerCase().includes(search.value.trim().toLowerCase())))
function toggle(id:string){emit('update:modelValue',props.modelValue.includes(id)?props.modelValue.filter(s=>s!==id):[...props.modelValue,id])}
function groupIds(id:string){
 const g=groups.value.find(item=>item.id===id)
 if(!g)return []
 return g.ids.filter(cid=>selectable.value.some(s=>s.service_id===cid&&unlockSupported(s.service_id)))
}
function toggleGroup(id:string,checked:boolean){const ids=groupIds(id);emit('update:modelValue',checked?[...new Set([...props.modelValue,...ids])]:props.modelValue.filter(s=>!ids.includes(s)))}
function selectedSupportedIds(){return props.modelValue.filter(id=>selectable.value.some(s=>s.service_id===id&&unlockSupported(s.service_id)))}
function startEditGroup(id:string){
 const target=groups.value.find(g=>g.id===id);if(!target)return
 editingGroupId.value=id;customName.value=target.name;customOpen.value=true
 emit('update:modelValue',groupIds(id))
}
function cancelEditGroup(){editingGroupId.value='';customName.value=''}
function saveCustomGroup(){
 const ids=selectedSupportedIds();if(!ids.length)return
 if(editingGroupId.value){
  const target=groups.value.find(g=>g.id===editingGroupId.value)
  if(target){
   const name=(customName.value||'').trim()||target.name
   target.name=name;target.ids=[...ids]
   editingGroupId.value='';customName.value='';persistGroups();return
  }
 }
 const name=(customName.value||'').trim()||'自定义'
 const existing=groups.value.find(g=>g.name===name)
 if(existing)existing.ids=[...ids]
 else groups.value.push({id:`custom-${Date.now().toString(36)}`,name,ids:[...ids],custom:true})
 customName.value='';persistGroups()
}
function removeGroup(id:string){
 groups.value=groups.value.filter(g=>g.id!==id)
 if(editingGroupId.value===id)cancelEditGroup()
 persistGroups()
}
function resetDefaultGroups(){
 const customOnly=groups.value.filter(g=>g.custom)
 groups.value=[...defaultPresetGroups(),...customOnly]
 cancelEditGroup();persistGroups()
}
const defaultsCustomized=computed(()=>{
 const defs=defaultPresetGroups()
 return defs.some(d=>{const cur=groups.value.find(g=>g.id===d.id);return !cur||cur.name!==d.name||cur.ids.join(',')!==d.ids.join(',')})
})
</script>
<template><div class="service-picker unlock-picker">
 <div class="unlock-group-options"><label v-for="g in groups" :key="g.id" :class="{'is-editing':editingGroupId===g.id}"><input type="checkbox" :checked="groupIds(g.id).length>0&&groupIds(g.id).every(id=>modelValue.includes(id))" :indeterminate="groupIds(g.id).some(id=>modelValue.includes(id))&&!groupIds(g.id).every(id=>modelValue.includes(id))" @change="toggleGroup(g.id,($event.target as HTMLInputElement).checked)"><strong>{{g.name}}</strong><small>{{groupIds(g.id).length}} 项</small><span class="unlock-group-actions"><button type="button" class="text-button unlock-group-edit" :aria-label="`编辑分组 ${g.name}`" title="编辑此分组名称与包含项目" @click.prevent.stop="startEditGroup(g.id)">编辑</button><button type="button" class="text-button unlock-group-remove" :aria-label="g.custom?`删除自定义分组 ${g.name}`:`删除分组 ${g.name}`" title="删除此分组" @click.prevent.stop="removeGroup(g.id)">×</button></span></label><p v-if="!groups.length" class="muted small">暂无预设分组，请在下方展开自定义勾选后保存分组，或<button type="button" class="text-button" @click="resetDefaultGroups">恢复默认分组</button>。</p></div>
 <div class="unlock-group-meta"><label class="unlock-region-toggle"><input v-model="w.skipByApplicableRegions.value" type="checkbox">按适用地区跳过</label><button v-if="defaultsCustomized" type="button" class="text-button" @click="resetDefaultGroups">恢复默认分组</button></div>
 <details class="unlock-custom" :open="customOpen" @toggle="customOpen=($event.target as HTMLDetailsElement).open"><summary>展开自定义 <small>已选 {{modelValue.length}} 项<template v-if="editingGroupId"> · 正在编辑分组</template></small></summary><div class="unlock-custom-bar"><input v-model="search" type="search" aria-label="搜索检测服务" placeholder="搜索解锁项目"><div class="unlock-custom-save"><input v-model="customName" type="text" aria-label="自定义分组名称" :placeholder="editingGroupId?'修改分组名称':'保存为分组（默认：自定义）'"><button type="button" class="button" :disabled="!selectedSupportedIds().length" @click="saveCustomGroup">{{editingGroupId?'更新分组':'保存分组'}}</button><button v-if="editingGroupId" type="button" class="button ghost" @click="cancelEditGroup">取消</button></div></div><div class="service-picker-list"><label v-for="s in visible" :key="s.service_id" :class="{checked:modelValue.includes(s.service_id)}" :title="s.description"><input type="checkbox" :checked="modelValue.includes(s.service_id)" :disabled="!unlockSupported(s.service_id)" @change="toggle(s.service_id)"><span><strong>{{s.name}}{{!unlockSupported(s.service_id)?' · 暂不支持':''}}</strong><small>{{s.description}}</small></span></label></div></details>
</div></template>
<style scoped>.unlock-group-meta{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-top:12px}.unlock-region-toggle{display:flex;align-items:center;gap:7px;font-size:12px}.unlock-region-toggle input{width:15px;height:15px}.unlock-group-options{display:flex;gap:8px;flex-wrap:wrap}.unlock-group-options label{display:flex;align-items:center;gap:8px;padding:9px 13px;border:1px solid var(--line);border-radius:10px;background:var(--surface);flex:1 1 160px;min-width:0;cursor:pointer;transition:border-color .14s,background .14s}.unlock-group-options label:hover,.unlock-group-options label.is-editing{border-color:var(--blue)}.unlock-group-options strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12.5px}.unlock-group-options small,.unlock-custom small{color:var(--muted);font-size:11px;white-space:nowrap}.unlock-group-options input{width:15px;height:15px;flex:none}.unlock-group-actions{display:inline-flex;align-items:center;gap:4px;margin-left:auto;flex:none}.unlock-group-edit{padding:0 4px;font-size:11px;color:var(--blue)}.unlock-group-remove{padding:0 4px;font-size:14px;line-height:1;color:var(--muted)}.unlock-custom{margin-top:12px}.unlock-custom summary{font-size:12px;cursor:pointer}.unlock-custom-bar{display:flex;gap:10px;flex-wrap:wrap;align-items:center;margin:12px 0}.unlock-custom-bar>input{flex:1 1 180px;min-width:0}.unlock-custom-save{display:flex;gap:8px;align-items:center;flex:1 1 240px;min-width:0}.unlock-custom-save input{flex:1;min-width:0}</style>
