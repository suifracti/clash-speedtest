<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {api} from '../api'
import {useWorkspace} from '../workspace'
import {airportNodeCounts} from '../airportViewing'
import {date,type LatencyBatch} from '../domain'
const emit=defineEmits<{select:[id:string]}>(),w=useWorkspace(),search=ref(''),collapsed=ref(false),batches=ref<LatencyBatch[]>([])
const counts=computed(()=>airportNodeCounts(w.airports.value,w.nodes.value)),total=computed(()=>Object.values(counts.value).reduce((a,b)=>a+b,0))
const visible=computed(()=>w.airports.value.filter(a=>a.name.toLowerCase().includes(search.value.trim().toLowerCase())))
const inScope=computed(()=>new Set(w.selectedAirportIds.value||[]))
const excluded=computed(()=>w.airports.value.some(a=>!inScope.value.has(a.id)))
const allChecked=computed(()=>w.airports.value.length>0&&w.airports.value.every(a=>inScope.value.has(a.id)))
const someChecked=computed(()=>w.airports.value.some(a=>inScope.value.has(a.id))&&!allChecked.value)
function toggleAirportScope(id:string){const cur=w.selectedAirportIds.value||[];w.setAirports(cur.includes(id)?cur.filter(x=>x!==id):[...cur,id])}
function toggleAllScope(checked:boolean){w.setAirports(checked?w.airports.value.map(a=>a.id):[])}
const cache=new Map<string,LatencyBatch>()
watch(()=>w.batches.value,async(rows,_,onCleanup)=>{
 let cancelled=false;onCleanup(()=>cancelled=true)
 try{
  const read:LatencyBatch[]=[],covered=new Set<string>()
  for(const row of rows){
   if(!row.state.startsWith('completed'))continue
   const batch=row.items?row:cache.get(row.batch_id)||await api.batch(row.batch_id)
   if(cancelled)return
   cache.set(batch.batch_id,batch);read.push(batch)
   for(const a of w.airports.value)if(batch.items?.some(i=>(a.subscriptions?.map(s=>s.id)||[a.id]).includes(i.profile_id)))covered.add(a.id)
   if(covered.size===w.airports.value.length)break
  }
  batches.value=read
 }catch{if(!cancelled)batches.value=[]}
},{immediate:true})
const health=computed(()=>Object.fromEntries(w.airports.value.map(a=>{
 const profiles=a.subscriptions?.map(s=>s.id)||[a.id],batch=batches.value.find(b=>b.items?.some(i=>profiles.includes(i.profile_id)))
 const items=batch?.items?.filter(i=>profiles.includes(i.profile_id)&&i.result&&i.result.total_samples>0)||[],failed=items.filter(i=>i.result!.success_samples===0).length
 return [a.id,{tone:!items.length?'empty':failed/items.length>=.5?'bad':failed?'warn':'good',label:!items.length?'暂无延迟批次结果':`最近延迟批次 ${date(batch!.requested_at)}：${failed}/${items.length} 个已检测节点无响应`}]
})))
</script>
<template><section v-if="w.airports.value.length" class="cst-side-airports" aria-label="机场快捷切换">
 <button class="airport-heading" :aria-expanded="!collapsed" @click="collapsed=!collapsed"><span>机场</span><small>{{collapsed?'展开':'收起'}}</small></button>
 <template v-if="!collapsed"><input v-model="search" class="airport-search" placeholder="搜索机场" aria-label="搜索侧栏机场">
 <div class="sidebar-airport-row">
  <button type="button" class="sidebar-airport-pick" :aria-pressed="!excluded&&w.airportFilter.value==='all'" @click="emit('select','all')"><span>全部机场</span><small>{{total}}</small></button>
  <input type="checkbox" class="sidebar-scope-check" aria-label="纳入全部机场" title="纳入或取消全部机场" :checked="allChecked" :indeterminate="someChecked" @change="toggleAllScope(($event.target as HTMLInputElement).checked)">
 </div>
 <div v-for="a in visible" :key="a.id" class="sidebar-airport-row" :class="{excluded:!inScope.has(a.id)}">
  <button type="button" class="sidebar-airport-pick" :title="`${a.name} · ${health[a.id].label}`" :aria-pressed="w.airportFilter.value===a.id" @click="emit('select',a.id)"><i class="airport-health" :class="health[a.id].tone" :aria-label="health[a.id].label"/><span>{{a.name}}</span><small>{{counts[a.id]||0}}</small></button>
  <input type="checkbox" class="sidebar-scope-check" :aria-label="`纳入范围 ${a.name}`" :title="`勾选将 ${a.name} 纳入全部机场范围`" :checked="inScope.has(a.id)" @change="toggleAirportScope(a.id)">
 </div>
 <p v-if="!visible.length" class="muted small">没有匹配机场</p></template>
</section></template>
<style scoped>
.airport-search{width:100%;height:34px;min-height:34px;padding:4px 10px;font-size:12.5px;margin:4px 0 8px;border-radius:var(--cst-radius-sm,9px);border:1px solid var(--cst-line);background:var(--cst-panel-2);color:var(--cst-text);transition:border-color .15s,background .15s,box-shadow .15s}
.airport-search::placeholder{color:var(--cst-subtle)}
.airport-search:focus{outline:none;border-color:var(--cst-accent);background:var(--cst-panel);box-shadow:0 0 0 3px color-mix(in srgb,var(--cst-accent) 13%,transparent)}
.airport-heading{font-size:10.5px;font-weight:700;text-transform:uppercase;letter-spacing:.04em;color:var(--cst-subtle)!important;height:24px!important}
.airport-heading small{background:transparent!important;padding:0!important;font-weight:500;color:var(--cst-subtle)!important}
.airport-health{width:6px;height:6px;flex:0 0 6px;border-radius:50%;background:var(--cst-subtle)}
.airport-health.good{background:var(--cst-good)}.airport-health.warn{background:var(--cst-warn)}.airport-health.bad{background:var(--cst-bad)}
</style>
