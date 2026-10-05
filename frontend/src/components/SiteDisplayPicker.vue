<script setup lang="ts">
import {computed,nextTick,onBeforeUnmount,onMounted,ref,useId} from 'vue'
import {targets} from '../domain'
import Icon from './Icon.vue'

const props=withDefaults(defineProps<{modelValue:string[];choices?:{id:string;name:string}[];label?:string;itemLabel?:string;defaultId?:string}>(),{label:'显示站点',itemLabel:'站点',defaultId:'cloudflare'})
const emit=defineEmits<{'update:modelValue':[value:string[]]}>()
const root=ref<HTMLElement>(),trigger=ref<HTMLButtonElement>(),menu=ref<HTMLElement>(),open=ref(false),search=ref(''),uid=useId(),menuStyle=ref<Record<string,string>>({})
const choices=computed(()=>props.choices??targets)
const defaultChoice=computed(()=>choices.value.find(choice=>choice.id===props.defaultId)||choices.value[0])
const selected=computed(()=>{const valid=choices.value.filter(choice=>props.modelValue.includes(choice.id)).map(choice=>choice.id);return valid.length?valid:defaultChoice.value?[defaultChoice.value.id]:[]})
const filtered=computed(()=>choices.value.filter(choice=>choice.name.toLowerCase().includes(search.value.trim().toLowerCase())))
const summary=computed(()=>selected.value.length===1?choices.value.find(choice=>choice.id===selected.value[0])!.name:selected.value.length?`${selected.value.length} 个${props.itemLabel}`:`暂无${props.itemLabel}`)
function selectAll(){if(!filtered.value.length)return;emit('update:modelValue',[...new Set([...selected.value,...filtered.value.map(choice=>choice.id)])])}
function choose(id:string){if(selected.value.includes(id)){if(selected.value.length===1)return;emit('update:modelValue',selected.value.filter(value=>value!==id))}else emit('update:modelValue',[...selected.value,id])}
function close(restore=false){open.value=false;search.value='';if(restore)trigger.value?.focus()}
function position(){if(!open.value||!trigger.value)return;const box=trigger.value.getBoundingClientRect(),width=Math.min(300,window.innerWidth-24),height=Math.min(menu.value?.offsetHeight||360,window.innerHeight-24);const below=window.innerHeight-box.bottom-16,above=box.top-16,top=below>=height||below>=above?box.bottom+6:Math.max(12,box.top-height-6);menuStyle.value={width:`${width}px`,left:`${Math.max(12,Math.min(box.left,window.innerWidth-width-12))}px`,top:`${top}px`,maxHeight:`${Math.max(100,window.innerHeight-top-12)}px`}}
function buttons(){return Array.from(menu.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')||[])}
async function show(last=false){if(!choices.value.length)return;open.value=true;await nextTick();position();const list=buttons(),index=last?list.length-1:filtered.value.findIndex(choice=>selected.value.includes(choice.id));list[Math.max(0,index)]?.focus()}
function triggerKey(e:KeyboardEvent){if(['ArrowDown','ArrowUp'].includes(e.key)){e.preventDefault();void show(e.key==='ArrowUp')}else if(e.key==='Escape'&&open.value){e.preventDefault();close(true)}}
function menuKey(e:KeyboardEvent){if(e.key==='Escape'){e.preventDefault();e.stopPropagation();close(true);return}if(!['ArrowDown','ArrowUp','Home','End'].includes(e.key)||(e.target instanceof HTMLInputElement&&['Home','End'].includes(e.key)))return;e.preventDefault();const list=buttons(),index=list.indexOf(document.activeElement as HTMLButtonElement);const next=e.key==='Home'?0:e.key==='End'?list.length-1:index<0?(e.key==='ArrowDown'?0:list.length-1):(index+(e.key==='ArrowDown'?1:-1)+list.length)%list.length;list[next]?.focus()}
function outside(e:Event){if(open.value&&!root.value?.contains(e.target as Node))close()}
onMounted(()=>{document.addEventListener('pointerdown',outside,true);document.addEventListener('focusin',outside,true);window.addEventListener('resize',position);window.addEventListener('scroll',position,true)})
onBeforeUnmount(()=>{document.removeEventListener('pointerdown',outside,true);document.removeEventListener('focusin',outside,true);window.removeEventListener('resize',position);window.removeEventListener('scroll',position,true)})
</script>

<template>
  <div ref="root" class="site-display-picker">
    <span :id="uid+'-label'" class="site-display-label">{{label}}</span>
    <button ref="trigger" type="button" class="site-display-trigger" :disabled="!choices.length" :aria-label="label+'：'+summary" aria-haspopup="menu" :aria-expanded="open" :aria-controls="uid+'-menu'" @click="open?close():show()" @keydown="triggerKey"><span>{{summary}}</span><Icon name="down"/></button>
    <div v-if="open" :id="uid+'-menu'" ref="menu" class="site-display-menu" role="menu" :aria-labelledby="uid+'-label'" :style="menuStyle" @keydown="menuKey">
      <p class="site-display-help">选择要一起查看的{{itemLabel}}，至少保留一个</p>
      <input v-if="choices.length>8" v-model="search" type="search" class="site-display-search" :aria-label="'搜索'+label" :placeholder="'搜索'+itemLabel+'名称'">
      <div class="site-display-options" role="group" :aria-label="label">
        <button v-for="choice in filtered" :key="choice.id" type="button" role="menuitemcheckbox" :aria-checked="selected.includes(choice.id)" :aria-disabled="selected.length===1&&selected[0]===choice.id||undefined" :class="{checked:selected.includes(choice.id)}" @click="choose(choice.id)"><span class="site-display-check"><Icon v-if="selected.includes(choice.id)" name="check"/></span><span>{{choice.name}}</span></button>
        <p v-if="!filtered.length" class="site-display-help">暂无匹配的{{itemLabel}}</p>
      </div>
      <div class="site-display-actions">
        <button type="button" role="menuitem" :disabled="!filtered.length" @click="selectAll">{{search.trim()?'全选筛选项':'全选'}}</button>
        <button type="button" role="menuitem" :disabled="!defaultChoice" @click="defaultChoice&&emit('update:modelValue',[defaultChoice.id])">还原 {{defaultChoice?.name}}</button>
        <button type="button" role="menuitem" class="site-display-done" @click="close(true)">完成</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.site-display-picker{display:inline-flex;align-items:center;gap:6px;min-width:0;position:relative}
.site-display-label{font-size:12px;color:var(--muted);white-space:nowrap}
.site-display-trigger{display:flex;align-items:center;gap:12px;min-width:0;max-width:220px;min-height:30px;padding:5px 8px;border:1px solid transparent;border-radius:7px;background:transparent;color:var(--ink);font-size:12px;white-space:nowrap}
.site-display-trigger>span{overflow:hidden;text-overflow:ellipsis}
.site-display-trigger .icon{width:13px;height:13px;color:var(--muted)}
.site-display-trigger:hover,.site-display-trigger[aria-expanded=true]{background:var(--surface);border-color:var(--line)}
.site-display-menu{position:fixed;z-index:10000;display:flex;flex-direction:column;padding:7px;border:1px solid var(--line);border-radius:9px;background:var(--surface);color:var(--ink);box-shadow:var(--shadow);overflow:auto;overscroll-behavior:contain;text-align:left}
.site-display-help{padding:5px 8px 8px;font-size:11px;color:var(--muted);line-height:1.6}
.site-display-help,.site-display-search,.site-display-actions{flex:none}
.site-display-search{width:100%;margin:0 0 6px;font-size:12px}
.site-display-options{min-height:0;max-height:320px;overflow:auto;overscroll-behavior:contain}
.site-display-options>button{display:flex;align-items:center;gap:10px;width:100%;min-height:35px;padding:8px;border:0;border-radius:5px;background:transparent;text-align:left;font-size:12px}
.site-display-options>button>span:last-child{min-width:0;overflow-wrap:anywhere}
.site-display-options>button:hover,.site-display-options>button:focus-visible{background:var(--blue-soft)}
.site-display-options>button.checked{color:var(--blue)}
.site-display-check{display:grid;place-items:center;width:15px;height:15px;border:1px solid var(--gray);border-radius:3px;flex:none}
.checked .site-display-check{border-color:var(--blue);background:var(--blue-soft)}
.site-display-check .icon{width:13px;height:13px}
.site-display-actions{display:flex;gap:4px;align-items:center;flex-wrap:wrap;border-top:1px solid var(--line);padding-top:8px;margin-top:6px}
.site-display-actions button{padding:5px 7px;border:0;border-radius:5px;background:transparent;font-size:11px;color:var(--blue)}
.site-display-actions button:hover{background:var(--blue-soft)}
.site-display-actions .site-display-done{margin-left:auto;background:var(--blue);color:var(--surface)}
@media(max-width:640px){.site-display-label{font-size:11px}.site-display-trigger{gap:7px;font-size:11px}}
</style>
