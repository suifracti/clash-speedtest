<script setup lang="ts">
import {computed,nextTick,onBeforeUnmount,onMounted,onUpdated,ref,useAttrs,useId,watch} from 'vue'
import Icon from './Icon.vue'

defineOptions({inheritAttrs:false})
type Value=string|number
type Choice={value:Value;label:string;disabled:boolean;index:number}
const props=withDefaults(defineProps<{modelValue?:Value;modelModifiers?:{number?:boolean};disabled?:boolean}>(),{disabled:false})
const emit=defineEmits<{'update:modelValue':[value:Value];change:[event:Event]}>()
const attrs=useAttrs(),uid=useId(),root=ref<HTMLDivElement>(),trigger=ref<HTMLButtonElement>(),native=ref<HTMLSelectElement>(),menu=ref<HTMLDivElement>()
const choices=ref<Choice[]>([]),open=ref(false),active=ref(-1),inheritedDisabled=ref(false),label=ref(''),popoverSupported=ref(false),menuStyle=ref<Record<string,string>>({})
const disabled=computed(()=>props.disabled||inheritedDisabled.value)
const selected=computed(()=>choices.value.findIndex(o=>String(o.value)===String(props.modelValue??'')))
const selectedLabel=computed(()=>choices.value[selected.value]?.label||'请选择')
const buttonAttrs=computed(()=>Object.fromEntries(Object.entries(attrs).filter(([key])=>!['class','style','name','form','required','value','onChange'].includes(key))))
let fieldsObserver:MutationObserver|undefined,optionsObserver:MutationObserver|undefined,search='',searchTimer:ReturnType<typeof setTimeout>|undefined

function sync(){
  if(!native.value)return
  const next=Array.from(native.value.options).map((o,index)=>({index,label:o.label,value:(o as HTMLOptionElement&{_value?:Value})._value??o.value,disabled:o.disabled||(o.parentElement instanceof HTMLOptGroupElement&&o.parentElement.disabled)}))
  if(next.length!==choices.value.length||next.some((o,i)=>{const before=choices.value[i];return !before||o.value!==before.value||o.label!==before.label||o.disabled!==before.disabled}))choices.value=next
  native.value.value=String(props.modelValue??'')
  inheritedDisabled.value=native.value.matches(':disabled')&&!props.disabled
  const labels=trigger.value?.labels
  if(labels?.length){label.value=Array.from(labels).map(l=>{const copy=l.cloneNode(true) as HTMLElement;copy.querySelectorAll('.ui-select').forEach(el=>el.remove());return copy.textContent?.trim()||''}).filter(Boolean).join(' ')}
  if(disabled.value&&open.value)close(false)
  if(open.value&&(!choices.value[active.value]||choices.value[active.value].disabled))active.value=firstEnabled()
}
function firstEnabled(last=false){if(last){for(let i=choices.value.length-1;i>=0;i--)if(!choices.value[i].disabled)return i;return -1}return choices.value.findIndex(o=>!o.disabled)}
function position(){
  if(!open.value||!trigger.value||!menu.value)return
  const r=trigger.value.getBoundingClientRect(),margin=8,gap=5,vw=window.innerWidth,vh=window.innerHeight
  const below=Math.max(0,vh-r.bottom-margin-gap),above=Math.max(0,r.top-margin-gap),wanted=Math.min(328,choices.value.length*36+12)
  const upwards=below<wanted&&above>below,available=upwards?above:below,height=Math.min(wanted,available),width=Math.min(Math.max(r.width,160),Math.max(0,vw-margin*2))
  menuStyle.value={left:`${Math.max(margin,Math.min(r.left,vw-width-margin))}px`,top:`${Math.max(margin,upwards?r.top-height-gap:r.bottom+gap)}px`,width:`${width}px`,maxHeight:`${Math.max(0,height)}px`}
}
function reveal(){void nextTick(()=>menu.value?.querySelector<HTMLElement>(`[data-index="${active.value}"]`)?.scrollIntoView?.({block:'nearest'}))}
async function show(){
  sync();if(disabled.value||!choices.value.some(o=>!o.disabled))return
  active.value=selected.value>=0&&!choices.value[selected.value].disabled?selected.value:firstEnabled()
  open.value=true;await nextTick();position()
  // Popover's top layer stays above native dialogs without teleporting outside their focus scope.
  try{menu.value?.showPopover?.()}catch{popoverSupported.value=false}
  reveal()
}
function close(focus=true){
  if(!open.value)return
  try{menu.value?.hidePopover?.()}catch{}
  open.value=false;search='';clearTimeout(searchTimer)
  if(focus&&!disabled.value)trigger.value?.focus({preventScroll:true})
}
function toggle(){if(open.value)close();else void show()}
function nativeChange(event:Event){
  if(disabled.value||native.value?.matches(':disabled'))return
  const option=native.value?.options[native.value.selectedIndex]
  if(!option)return
  let value=(option as HTMLOptionElement&{_value?:Value})._value??option.value
  if(props.modelModifiers?.number&&typeof value==='string'){const number=parseFloat(value);if(!Number.isNaN(number))value=number}
  emit('update:modelValue',value);emit('change',event)
}
function choose(index:number){
  sync();if(disabled.value||!native.value||!choices.value[index]||choices.value[index].disabled)return
  native.value.selectedIndex=index
  native.value.dispatchEvent(new Event('change',{bubbles:true}));close()
}
function move(step:number){
  let next=active.value+step
  while(next>=0&&next<choices.value.length){if(!choices.value[next].disabled){active.value=next;reveal();return}next+=step}
}
async function keydown(event:KeyboardEvent){
  if(disabled.value)return
  if(event.key==='Tab'){close(false);return}
  if(event.key==='Escape'){if(open.value){event.preventDefault();event.stopPropagation();close()}return}
  if(['ArrowDown','ArrowUp','Home','End'].includes(event.key)){
    event.preventDefault();const wasOpen=open.value;if(!wasOpen)await show()
    if(event.key==='Home'||event.key==='End'){active.value=firstEnabled(event.key==='End');reveal()}
    else if(wasOpen)move(event.key==='ArrowDown'?1:-1)
    return
  }
  if(event.key==='Enter'||(event.key===' '&&!search)){event.preventDefault();if(open.value)choose(active.value);else await show();return}
  if(event.key.length===1&&!event.ctrlKey&&!event.metaKey&&!event.altKey){
    event.preventDefault();if(!open.value)await show();search+=event.key.toLocaleLowerCase();clearTimeout(searchTimer);searchTimer=setTimeout(()=>{search=''},650)
    const match=choices.value.findIndex(o=>!o.disabled&&o.label.toLocaleLowerCase().startsWith(search));if(match>=0){active.value=match;reveal()}
  }
}
function outside(event:PointerEvent){if(open.value&&!root.value?.contains(event.target as Node))close(false)}
function onFocus(event:FocusEvent){if(open.value&&!root.value?.contains(event.target as Node))close(false)}
onMounted(()=>{
  popoverSupported.value=typeof menu.value?.showPopover==='function';sync();optionsObserver=new MutationObserver(sync);if(native.value)optionsObserver.observe(native.value,{childList:true,subtree:true,characterData:true,attributes:true,attributeFilter:['disabled','label','value']})
  fieldsObserver=new MutationObserver(sync);let parent=root.value?.parentElement;while(parent){if(parent instanceof HTMLFieldSetElement)fieldsObserver.observe(parent,{attributes:true,attributeFilter:['disabled']});parent=parent.parentElement}
  document.addEventListener('pointerdown',outside,true);document.addEventListener('focusin',onFocus,true);window.addEventListener('resize',position);window.addEventListener('scroll',position,true)
})
onUpdated(sync)
watch(()=>props.disabled,()=>{void nextTick(sync)})
onBeforeUnmount(()=>{close(false);clearTimeout(searchTimer);fieldsObserver?.disconnect();optionsObserver?.disconnect();document.removeEventListener('pointerdown',outside,true);document.removeEventListener('focusin',onFocus,true);window.removeEventListener('resize',position);window.removeEventListener('scroll',position,true)})
</script>

<template>
  <div ref="root" class="ui-select" :class="attrs.class" :style="attrs.style as any">
    <button ref="trigger" v-bind="buttonAttrs" type="button" class="ui-select-trigger" role="combobox" :disabled="disabled" :aria-label="(attrs['aria-label'] as string)||label||undefined" aria-haspopup="listbox" :aria-expanded="open" :aria-controls="uid+'-list'" :aria-activedescendant="open&&active>=0?uid+'-option-'+active:undefined" @click="toggle" @keydown="keydown"><span>{{selectedLabel}}</span><Icon name="down"/></button>
    <select ref="native" class="ui-select-native" tabindex="-1" aria-hidden="true" :aria-label="attrs['aria-label'] as string" :name="attrs.name as string" :form="attrs.form as string" :required="!!attrs.required" :disabled="props.disabled" :value="modelValue" @change="nativeChange" @focus="trigger?.focus()"><slot/></select>
    <div :id="uid+'-list'" ref="menu" v-show="open" class="ui-select-menu" :class="{fallback:!popoverSupported}" :popover="popoverSupported?'manual':undefined" role="listbox" :aria-label="(attrs['aria-label'] as string)||label||undefined" :style="menuStyle" @pointerdown.prevent @click.stop.prevent>
      <div v-for="option in choices" :id="uid+'-option-'+option.index" :key="option.index" class="ui-select-option" :class="{active:active===option.index,selected:selected===option.index,disabled:option.disabled}" :data-index="option.index" role="option" :aria-selected="selected===option.index" :aria-disabled="option.disabled||undefined" @pointermove="()=>{if(!option.disabled)active=option.index}" @click="choose(option.index)"><span>{{option.label}}</span><Icon v-if="selected===option.index" name="check"/></div>
    </div>
  </div>
</template>

<style scoped>
.ui-select{display:inline-flex;position:relative;min-width:95px;width:max-content;max-width:100%;vertical-align:middle}
.ui-select-trigger{display:flex;align-items:center;justify-content:space-between;gap:15px;width:100%;min-width:0;min-height:36px;padding:9px 11px;border:1px solid var(--line);border-radius:7px;background:var(--surface);color:var(--ink);text-align:left;font:inherit;line-height:1.4;transition:border-color .12s,background .12s}
.ui-select-trigger>span{white-space:nowrap;overflow:hidden;text-overflow:ellipsis;min-width:0}.ui-select-trigger>.icon{width:14px;height:14px;color:var(--muted)}
.ui-select-trigger:hover:enabled{border-color:var(--gray);background:var(--surface-soft)}.ui-select-trigger[aria-expanded=true],.ui-select-trigger:focus-visible{border-color:var(--blue);outline:0;box-shadow:0 0 0 3px color-mix(in srgb,var(--blue) 13%,transparent)}
.ui-select-native{position:absolute!important;width:1px!important;height:1px!important;min-width:0!important;padding:0!important;margin:0!important;border:0!important;opacity:0;pointer-events:none;clip-path:inset(50%)}
.ui-select-menu{position:fixed;inset:auto;margin:0;padding:5px;border:1px solid var(--line);border-radius:9px;background:var(--surface);color:var(--ink);box-shadow:var(--shadow);overflow-y:auto;overscroll-behavior:contain;z-index:10000;font-size:13px;text-align:left;line-height:1.5}
.ui-select-menu:not(:popover-open){display:none}.ui-select-menu:popover-open{display:block}
.ui-select-menu.fallback{display:block}
.ui-select-option{display:flex;align-items:center;justify-content:space-between;gap:15px;min-height:36px;border-radius:5px;padding:8px 10px;cursor:pointer}.ui-select-option>span{overflow-wrap:anywhere}.ui-select-option>.icon{width:15px;height:15px;color:var(--blue)}.ui-select-option.active:not(.disabled){background:var(--blue-soft)}.ui-select-option.selected{color:var(--blue);font-weight:500}.ui-select-option.disabled{opacity:.45;cursor:not-allowed}
:global(.form-grid label>.ui-select),:global(label.form-stack>.ui-select){width:100%}
:global(.context-controls .ui-select-trigger){font-size:12px;min-height:30px;padding:5px 8px;background:transparent;border-color:transparent}
:global(.toolbar-filters .ui-select){max-width:190px}:global(.toolbar-filters .ui-select-trigger){font-size:12px;padding:8px 9px}
:global(.detail-filters .ui-select){max-width:200px}:global(.detail-filters .ui-select-trigger){font-size:12px;padding:6px 8px}
:global(.history-filter-row .ui-select){max-width:230px}:global(.history-filter-row .ui-select-trigger){font-size:12px}
:global(.service-picker-tools .ui-select){max-width:150px}:global(.inline-history-tools .ui-select){max-width:230px}:global(.inline-history-tools .ui-select-trigger),:global(.service-result-tools .ui-select-trigger),:global(.overview-toolbar .ui-select-trigger){font-size:11px}
@media(max-width:1100px){:global(.toolbar-filters .ui-select){max-width:150px}}
@media(max-width:900px){:global(.toolbar-filters .ui-select){max-width:140px}}
@media(max-width:640px){:global(.toolbar-filters .ui-select){min-width:0;flex:1;max-width:calc(33% - 36px)}:global(.toolbar-filters .ui-select-trigger){font-size:11px;gap:6px;padding:8px 5px}:global(.history-filter-row .ui-select){max-width:170px;width:100%}}
@media(forced-colors:active){.ui-select-trigger,.ui-select-menu{border-color:ButtonText}.ui-select-option.active:not(.disabled){outline:1px solid Highlight}.ui-select-option.selected{color:Highlight}}
@media(prefers-reduced-motion:reduce){.ui-select-trigger{transition:none}}
</style>
