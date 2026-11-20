<script setup lang="ts">
import {computed,nextTick,ref} from 'vue'
import {useWorkspace} from '../workspace'
import type {ServiceRule} from '../domain'
import {unlockGroup,unlockSupported} from '../unlock'
import {defaultPresetGroups,editableUnlockGroups,optionalUnlockIds,persistUnlockGroups,resetDefaultUnlockGroups,syncUnlockGroupsFromStorage} from '../unlockGroupStore'
const props=defineProps<{modelValue:string[];catalog:ServiceRule[]}>(),emit=defineEmits<{'update:modelValue':[value:string[]]}>(),search=ref(''),customName=ref(''),customOpen=ref(false),showOptional=ref(false),editingGroupId=ref(''),nameInput=ref<HTMLInputElement|null>(null)
const w=useWorkspace()
syncUnlockGroupsFromStorage()
const selectable=computed(()=>props.catalog.filter(s=>!s.superseded_by&&unlockGroup(s.service_id)!=='legacy'))
const groups=editableUnlockGroups
const persistGroups=persistUnlockGroups
const categoryNames:Record<string,string>={ai:'AI 服务',streaming:'流媒体',japan:'日本地区'}
const optionalCount=computed(()=>selectable.value.filter(s=>optionalUnlockIds.has(s.service_id)).length)
const editingOptional=computed(()=>!!groups.value.find(g=>g.id===editingGroupId.value)?.ids.some(id=>optionalUnlockIds.has(id)))
const visible=computed(()=>{
 const q=search.value.trim().toLowerCase()
 const hasCoreInGroup=(gid:string)=>selectable.value.some(s=>unlockGroup(s.service_id)===gid&&!optionalUnlockIds.has(s.service_id))
 return selectable.value.filter(s=>{
  const isOpt=optionalUnlockIds.has(s.service_id)
  if(isOpt&&!showOptional.value&&!q&&!props.modelValue.includes(s.service_id)&&!editingOptional.value&&hasCoreInGroup(unlockGroup(s.service_id)))return false
  return !q||(s.name+' '+s.description+' '+(categoryNames[unlockGroup(s.service_id)]||'')).toLowerCase().includes(q)
 })
})
const groupedVisible=computed(()=>{
 const map=new Map<string,{id:string;name:string;items:ServiceRule[]}>()
 for(const s of visible.value){
  const gid=unlockGroup(s.service_id)
  if(!map.has(gid))map.set(gid,{id:gid,name:categoryNames[gid]||s.category||'其他解锁',items:[]})
  map.get(gid)!.items.push(s)
 }
 return [...map.values()]
})
function toggle(id:string){emit('update:modelValue',props.modelValue.includes(id)?props.modelValue.filter(s=>s!==id):[...props.modelValue,id])}
function toggleCategory(items:ServiceRule[]){
 const ids=items.filter(s=>unlockSupported(s.service_id)).map(s=>s.service_id)
 const allChecked=ids.length>0&&ids.every(id=>props.modelValue.includes(id))
 emit('update:modelValue',allChecked?props.modelValue.filter(id=>!ids.includes(id)):[...new Set([...props.modelValue,...ids])])
}
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
 void nextTick(()=>nameInput.value?.focus())
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
 resetDefaultUnlockGroups()
 cancelEditGroup()
}
const editingGroup=computed(()=>groups.value.find(g=>g.id===editingGroupId.value))
const defaultsCustomized=computed(()=>{
 const defs=defaultPresetGroups()
 return defs.some(d=>{const cur=groups.value.find(g=>g.id===d.id);return !cur||cur.name!==d.name||cur.ids.join(',')!==d.ids.join(',')})
})
</script>
<template>
  <div class="service-picker unlock-picker">
    <div class="unlock-group-options">
      <label v-for="g in groups" :key="g.id" :class="{'is-editing':editingGroupId===g.id,'is-checked':groupIds(g.id).length>0&&groupIds(g.id).every(id=>modelValue.includes(id))}">
        <span class="unlock-group-main">
          <input type="checkbox" :checked="groupIds(g.id).length>0&&groupIds(g.id).every(id=>modelValue.includes(id))" :indeterminate="groupIds(g.id).some(id=>modelValue.includes(id))&&!groupIds(g.id).every(id=>modelValue.includes(id))" @change="toggleGroup(g.id,($event.target as HTMLInputElement).checked)">
          <strong>{{g.name}}</strong>
          <small>{{groupIds(g.id).length}} 项</small>
        </span>
        <span class="unlock-group-actions">
          <button type="button" class="text-button unlock-group-edit" :aria-label="`编辑分组 ${g.name}`" title="编辑此分组名称与包含项目" @click.prevent.stop="startEditGroup(g.id)">编辑</button>
          <button type="button" class="text-button unlock-group-remove" :aria-label="g.custom?`删除自定义分组 ${g.name}`:`删除分组 ${g.name}`" title="删除此分组" @click.prevent.stop="removeGroup(g.id)">×</button>
        </span>
      </label>
      <p v-if="!groups.length" class="muted small">暂无预设分组，请在下方勾选项目后保存分组，或<button type="button" class="text-button" @click="resetDefaultGroups">恢复默认分组</button>。</p>
    </div>
    <div class="unlock-group-meta">
      <label class="unlock-region-toggle"><input v-model="w.skipByApplicableRegions.value" type="checkbox">按适用地区跳过</label>
      <div class="unlock-meta-actions">
        <button v-if="optionalCount" type="button" class="text-button" @click="showOptional=!showOptional;if(showOptional)customOpen=true">{{showOptional?'隐藏传统参考项':`显示传统参考项 (${optionalCount})`}}</button>
        <button v-if="defaultsCustomized" type="button" class="text-button" @click="resetDefaultGroups">恢复默认分组</button>
        <button type="button" class="text-button" @click="customOpen=!customOpen">{{customOpen?'收起分类明细':'按分类自定义选择'}}</button>
      </div>
    </div>
    <details class="unlock-custom" :open="customOpen" @toggle="customOpen=($event.target as HTMLDetailsElement).open">
      <summary>展开自定义 <small>已选 {{modelValue.length}} 项<template v-if="editingGroup"> · 正在编辑「{{editingGroup.name}}」</template></small></summary>
      <div v-if="editingGroup" class="unlock-edit-banner" role="status">
        <span>正在编辑分组 <strong>{{editingGroup.name}}</strong>：在下方分类勾选需要的项目（可包含 Antigravity），改名后点击「更新分组」。</span>
      </div>
      <div class="unlock-custom-bar">
        <input v-model="search" type="search" aria-label="搜索检测服务" placeholder="搜索解锁项目或分类">
        <div class="unlock-custom-save">
          <input ref="nameInput" v-model="customName" type="text" aria-label="自定义分组名称" :placeholder="editingGroupId?'修改分组名称':'保存为分组（默认：自定义）'">
          <button type="button" class="button primary-soft" :disabled="!selectedSupportedIds().length" @click="saveCustomGroup">{{editingGroupId?'更新分组':'保存分组'}}</button>
          <button v-if="editingGroupId" type="button" class="button ghost" @click="cancelEditGroup">取消</button>
        </div>
      </div>
      <div class="service-picker-list is-grouped">
        <section v-for="group in groupedVisible" :key="group.id" class="service-picker-group">
          <header class="service-picker-group-head">
            <h4>{{group.name}} <small>{{group.items.filter(s=>modelValue.includes(s.service_id)).length}}/{{group.items.length}}</small></h4>
            <button type="button" class="text-button" @click="toggleCategory(group.items)">{{group.items.filter(s=>unlockSupported(s.service_id)).every(s=>modelValue.includes(s.service_id))?'取消本组':'全选本组'}}</button>
          </header>
          <div class="service-picker-grid">
            <label v-for="s in group.items" :key="s.service_id" :class="{checked:modelValue.includes(s.service_id)}" :title="s.description">
              <input type="checkbox" :checked="modelValue.includes(s.service_id)" :disabled="!unlockSupported(s.service_id)" @change="toggle(s.service_id)">
              <span><strong>{{s.name}}{{!unlockSupported(s.service_id)?' · 暂不支持':''}}</strong><small>{{s.description}}</small></span>
            </label>
          </div>
        </section>
        <p v-if="!visible.length" class="muted small">没有匹配的解锁项目。</p>
      </div>
    </details>
  </div>
</template>
<style scoped>
.unlock-group-meta{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-top:10px}
.unlock-meta-actions{display:inline-flex;align-items:center;gap:12px;font-size:12px}
.unlock-region-toggle{display:flex;align-items:center;gap:7px;font-size:12px}
.unlock-region-toggle input{width:15px;height:15px}
.unlock-group-options{display:flex;gap:8px;flex-wrap:wrap}
.unlock-group-options label{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:8px 12px;border:1px solid var(--cst-line,var(--line));border-radius:10px;background:var(--cst-panel,var(--surface));flex:0 1 auto;min-width:176px;cursor:pointer;transition:border-color .14s,background .14s}
.unlock-group-options label:hover,.unlock-group-options label.is-editing{border-color:var(--cst-accent,var(--blue))}
.unlock-group-options label.is-checked{background:var(--cst-accent-soft,var(--blue-soft));border-color:color-mix(in srgb,var(--cst-accent,var(--blue)) 35%,var(--cst-line,var(--line)))}
.unlock-group-main{display:inline-flex;align-items:center;gap:7px;min-width:0}
.unlock-group-main input{width:15px;height:15px;flex:none}
.unlock-group-main strong{white-space:nowrap;font-size:13px;font-weight:650;color:var(--cst-text,var(--ink))}
.unlock-group-main small,.unlock-custom small{color:var(--cst-muted,var(--muted));font-size:11px;white-space:nowrap}
.unlock-group-actions{display:inline-flex;align-items:center;gap:4px;flex:none;padding-left:6px;border-left:1px solid var(--cst-line,var(--line))}
.unlock-group-edit{padding:2px 5px;font-size:11px;color:var(--cst-accent,var(--blue));white-space:nowrap}
.unlock-group-remove{padding:0 4px;font-size:14px;line-height:1;color:var(--cst-muted,var(--muted))}
.unlock-group-remove:hover{color:var(--cst-bad,var(--red))}
.unlock-custom{margin-top:10px}
.unlock-custom summary{font-size:12px;cursor:pointer;color:var(--cst-muted,var(--muted))}
.unlock-edit-banner{margin:10px 0 0;padding:8px 12px;border-radius:8px;background:var(--cst-accent-soft,var(--blue-soft));border:1px solid color-mix(in srgb,var(--cst-accent,var(--blue)) 30%,var(--cst-line,var(--line)));font-size:12px;color:var(--cst-text,var(--ink));line-height:1.5}
.unlock-custom-bar{display:flex;gap:10px;flex-wrap:wrap;align-items:center;margin:10px 0 12px}
.unlock-custom-bar>input{flex:1 1 170px;min-width:0}
.unlock-custom-save{display:flex;gap:8px;align-items:center;flex:1 1 250px;min-width:0}
.unlock-custom-save input{flex:1;min-width:0}
.service-picker-list.is-grouped{display:flex;flex-direction:column;gap:14px;max-height:300px;overflow:auto;padding:2px 4px}
.service-picker-group{display:grid;gap:8px}
.service-picker-group-head{display:flex;align-items:center;justify-content:space-between;gap:8px;padding-bottom:4px;border-bottom:1px solid var(--cst-line,var(--line))}
.service-picker-group-head h4{margin:0;font-size:12px;font-weight:700;color:var(--cst-text,var(--ink));display:inline-flex;align-items:center;gap:6px}
.service-picker-group-head small{font-weight:500;color:var(--cst-muted,var(--muted));font-size:11px}
.service-picker-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(210px,1fr));gap:8px}
.service-picker-grid>label{display:flex;align-items:flex-start;gap:9px;min-width:0;padding:9px 11px;border:1px solid var(--cst-line,var(--line));border-radius:10px;background:var(--cst-panel,var(--surface));cursor:pointer;transition:border-color .14s,background .14s}
.service-picker-grid>label:hover{border-color:var(--cst-accent,var(--blue))}
.service-picker-grid>label.checked{background:var(--cst-accent-soft,var(--blue-soft));border-color:color-mix(in srgb,var(--cst-accent,var(--blue)) 38%,var(--cst-line,var(--line)))}
.service-picker-grid>label>input{width:15px;height:15px;margin-top:2px;flex:none}
.service-picker-grid>label>span{min-width:0;display:grid;gap:3px}
.service-picker-grid>label strong{font-size:12.5px;line-height:1.35;color:var(--cst-text,var(--ink))}
.service-picker-grid>label small{font-size:11px;line-height:1.45;color:var(--cst-muted,var(--muted));display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
</style>
