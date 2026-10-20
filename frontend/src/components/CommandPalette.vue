<script setup lang="ts">
import {computed,nextTick,onMounted,ref,watch} from 'vue'
import {useWorkspace} from '../workspace'
import type {NodeOption,Page} from '../domain'
import {nodeDisplayName,nodeRegionLabel} from '../nodePresentation'
import {airportNodeCounts} from '../airportViewing'
import Modal from './Modal.vue'
import Icon from './Icon.vue'
const props=defineProps<{pages:{id:Page;label:string;icon:string}[]}>()
const emit=defineEmits<{close:[];navigate:[page:Page]}>()
const w=useWorkspace(),query=ref(''),active=ref(0),input=ref<HTMLInputElement>(),list=ref<HTMLElement>()
type Item={id:string;group:string;label:string;meta:string;icon:string;run:()=>void}
const norm=(s:string)=>s.toLowerCase().replace(/\s+/g,'')
function matches(text:string,q:string){return !q||norm(text).includes(q)}
// 节点只“查看”：切到工作台并在检查器打开，不勾选、不改筛选与冻结范围。
function viewNode(n:NodeOption){emit('navigate','home');w.viewRequest.value=n}
const items=computed<Item[]>(()=>{
 const q=norm(query.value),out:Item[]=[]
 for(const p of props.pages)if(matches(p.label,q))out.push({id:'page:'+p.id,group:'页面',label:p.label,meta:w.page.value===p.id?'当前页面':'',icon:p.icon,run:()=>emit('navigate',p.id)})
 if(w.queue.value.length&&matches('检测队列 进度',q))out.push({id:'queue',group:'页面',label:'检测队列',meta:`已处理 ${w.done.value}/${w.queue.value.length}`,icon:'select',run:()=>{w.queueOpen.value=true}})
 const counts=airportNodeCounts(w.airports.value)
 for(const a of w.airports.value)if(matches(a.name,q))out.push({id:'airport:'+a.id,group:'机场',label:a.name,meta:`${counts[a.id]||0} 个节点 · 在工作台只看此机场`,icon:'layers',run:()=>{if(w.airportFilter.value!==a.id)w.viewAirport(a.id);emit('navigate','home')}})
 if(q){
  let count=0
  for(const n of w.proxyNodes.value){
   const name=nodeDisplayName(n),text=`${n.display_name} ${name} ${w.nodeAirportName(n)} ${nodeRegionLabel(n.country_code||'')} ${n.country_code||''}`
   if(!matches(text,q))continue
   out.push({id:'node:'+n.profile_id+n.node_key,group:'节点',label:name,meta:[w.nodeAirportName(n),nodeRegionLabel(n.country_code||''),n.type].filter(Boolean).join(' · '),icon:'globe',run:()=>viewNode(n)})
   if(++count>=40)break
  }
 }
 return out
})
const groups=computed(()=>{const g:{name:string;rows:{item:Item;index:number}[]}[]=[];items.value.forEach((item,index)=>{let x=g.find(v=>v.name===item.group);if(!x)g.push(x={name:item.group,rows:[]});x.rows.push({item,index})});return g})
watch(query,()=>active.value=0)
function reveal(){void nextTick(()=>list.value?.querySelector('[aria-selected=true]')?.scrollIntoView?.({block:'nearest'}))}
function keydown(e:KeyboardEvent){
 if(e.isComposing)return
 const n=items.value.length
 if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();if(n)active.value=(active.value+(e.key==='ArrowDown'?1:n-1))%n;reveal()}
 else if(e.key==='Enter'){e.preventDefault();choose(active.value)}
}
function choose(i:number){const item=items.value[i];if(!item)return;emit('close');item.run()}
onMounted(()=>input.value?.focus())
</script>
<template>
  <Modal title="快速跳转" variant="cst-palette" @close="emit('close')">
    <div class="cst-palette-search"><Icon name="search"/><input ref="input" v-model="query" type="search" role="combobox" aria-expanded="true" aria-controls="cst-palette-list" :aria-activedescendant="items[active]?'cst-pal-'+active:undefined" aria-label="搜索页面、机场或节点" placeholder="搜索页面、机场或节点名称…" autocomplete="off" spellcheck="false" @keydown="keydown"></div>
    <div id="cst-palette-list" ref="list" class="cst-palette-list" role="listbox" aria-label="跳转结果">
      <p v-if="!items.length" class="cst-palette-empty">没有匹配的页面、机场或节点</p>
      <section v-for="g in groups" :key="g.name" role="group" :aria-label="g.name">
        <h3>{{g.name}}</h3>
        <div v-for="r in g.rows" :id="'cst-pal-'+r.index" :key="r.item.id" class="cst-palette-row" role="option" :aria-selected="r.index===active" @mousemove="active=r.index" @click="choose(r.index)">
          <Icon :name="r.item.icon"/><span class="cst-palette-label">{{r.item.label}}</span><small>{{r.item.meta}}</small>
        </div>
      </section>
      <p v-if="!query" class="cst-palette-hint">输入节点名称、地区或机场即可查找节点；选中节点只在检查器里查看，不会勾选。</p>
    </div>
    <footer class="cst-palette-keys"><span><kbd>↑</kbd><kbd>↓</kbd> 选择</span><span><kbd>↩</kbd> 打开</span><span><kbd>esc</kbd> 关闭</span></footer>
  </Modal>
</template>
