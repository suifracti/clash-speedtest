<script setup lang="ts">
import {computed,nextTick,onMounted,ref,watch} from 'vue'
import {useWorkspace} from '../workspace'
import {isNotice,scope,type NodeOption,type Page} from '../domain'
import {nodeDisplayName,nodeRegionLabel} from '../nodePresentation'
import {airportNodeCounts} from '../airportViewing'
import Modal from './Modal.vue'
import Icon from './Icon.vue'
const props=defineProps<{pages:{id:Page;label:string;icon:string;keywords?:string[]}[]}>()
const emit=defineEmits<{close:[];navigate:[page:Page]}>()
const w=useWorkspace(),query=ref(''),active=ref(0),input=ref<HTMLInputElement>(),list=ref<HTMLElement>(),nodeLimit=ref(40)
type Item={id:string;group:string;label:string;meta:string;icon:string;keepOpen?:boolean;run:()=>void}
const norm=(s:string)=>s.normalize('NFKC').toLocaleLowerCase().replace(/\s+/g,'')
const terms=computed(()=>query.value.trim().split(/\s+/u).map(norm).filter(Boolean))
function matches(text:string){const normalized=norm(text);return terms.value.every(word=>normalized.includes(word))}
// 节点只“查看”：切到工作台并在检查器打开，不勾选、不改筛选与冻结范围。
function viewNode(n:NodeOption){emit('navigate','home');w.viewRequest.value=n}
// Index all loaded accounts independently of the workbench's browsing filters.
const searchNodes=computed(()=>w.nodes.value.filter(n=>!isNotice(n)).map(n=>{
 const label=nodeDisplayName(n),airport=w.nodeAirportName(n),account=w.nodeSubscriptionName(n),region=nodeRegionLabel(n.country_code||'')
 return {node:n,label,meta:[airport,account,region,n.type].filter(Boolean).join(' · '),text:norm([n.display_name,label,airport,account,region,n.country_code||'',n.type].join(' '))}
}))
const nodeMatches=computed(()=>terms.value.length?searchNodes.value.filter(row=>terms.value.every(word=>row.text.includes(word))):[])
const counts=computed(()=>airportNodeCounts(w.airports.value,w.nodes.value))
function showMore(){const firstNew=items.value.length-1;nodeLimit.value+=40;void nextTick(()=>{active.value=firstNew;reveal()})}
const items=computed<Item[]>(()=>{
 const out:Item[]=[]
 for(const p of props.pages)if(matches([p.label,...(p.keywords||[])].join(' ')))out.push({id:'page:'+p.id,group:'页面',label:p.label,meta:w.page.value===p.id?'当前页面':'',icon:p.icon,run:()=>emit('navigate',p.id)})
 if(w.queue.value.length&&matches('检测队列 进度'))out.push({id:'queue',group:'页面',label:'检测队列',meta:`已处理 ${w.done.value}/${w.queue.value.length}`,icon:'select',run:()=>{w.queueOpen.value=true}})
 for(const a of w.airports.value)if(matches(a.name))out.push({id:'airport:'+a.id,group:'机场',label:a.name,meta:`${counts.value[a.id]||0} 个节点 · 在工作台只看此机场`,icon:'layers',run:()=>{if(w.airportFilter.value!==a.id)w.viewAirport(a.id);emit('navigate','home')}})
 for(const row of nodeMatches.value.slice(0,nodeLimit.value))out.push({id:'node:'+JSON.stringify(scope(row.node)),group:'节点',label:row.label,meta:row.meta,icon:'globe',run:()=>viewNode(row.node)})
 if(nodeMatches.value.length>nodeLimit.value)out.push({id:'nodes:more',group:'节点',label:'显示更多节点',meta:`已显示 ${nodeLimit.value} / ${nodeMatches.value.length} · 可继续输入缩小范围`,icon:'arrowRight',keepOpen:true,run:showMore})
 return out
})
const groups=computed(()=>{const g:{name:string;rows:{item:Item;index:number}[]}[]=[];items.value.forEach((item,index)=>{let x=g.find(v=>v.name===item.group);if(!x)g.push(x={name:item.group,rows:[]});x.rows.push({item,index})});return g})
watch(query,()=>{active.value=0;nodeLimit.value=40},{flush:'sync'})
watch(items,rows=>{active.value=Math.max(0,Math.min(active.value,rows.length-1))})
function reveal(){void nextTick(()=>list.value?.querySelector('[aria-selected=true]')?.scrollIntoView?.({block:'nearest'}))}
function keydown(e:KeyboardEvent){
 if(e.isComposing)return
 const n=items.value.length
 if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();if(n)active.value=(active.value+(e.key==='ArrowDown'?1:n-1))%n;reveal()}
 else if(e.key==='Enter'){e.preventDefault();choose(active.value)}
}
function choose(i:number){const item=items.value[i];if(!item)return;if(!item.keepOpen)emit('close');item.run()}
onMounted(()=>input.value?.focus())
</script>
<template>
  <Modal title="快速查找" variant="cst-palette" @close="emit('close')">
    <div class="cst-palette-search"><Icon name="search"/><input ref="input" v-model="query" type="search" role="combobox" aria-expanded="true" aria-controls="cst-palette-list" :aria-activedescendant="items[active]?'cst-pal-'+active:undefined" aria-label="搜索页面、机场或节点" placeholder="页面、机场、账户或节点…" autocomplete="off" spellcheck="false" @keydown="keydown"><button type="button" class="cst-palette-close" aria-label="关闭查找" @click="emit('close')"><Icon name="close"/></button></div>
    <div id="cst-palette-list" ref="list" class="cst-palette-list" role="listbox" aria-label="跳转结果">
      <p v-if="!items.length" class="cst-palette-empty">没有匹配的页面、机场或节点</p>
      <section v-for="g in groups" :key="g.name" role="group" :aria-label="g.name">
        <h3>{{g.name}}<span v-if="g.name==='节点'"> · {{nodeMatches.length}} 个匹配</span></h3>
        <div v-for="r in g.rows" :id="'cst-pal-'+r.index" :key="r.item.id" class="cst-palette-row" role="option" :aria-selected="r.index===active" @mousemove="active=r.index" @click="choose(r.index)">
          <Icon :name="r.item.icon"/><span class="cst-palette-copy"><span class="cst-palette-label">{{r.item.label}}</span><small v-if="r.item.meta">{{r.item.meta}}</small></span>
        </div>
      </section>
      <p v-if="!query" class="cst-palette-hint">查找所有已载入的机场与账户。可用空格组合地区、账户和节点名称；打开节点只查看，不会勾选或改变筛选。</p>
    </div>
    <footer class="cst-palette-keys"><span><kbd>↑</kbd><kbd>↓</kbd> 选择</span><span><kbd>↩</kbd> 打开</span><span><kbd>esc</kbd> 关闭</span></footer>
  </Modal>
</template>
