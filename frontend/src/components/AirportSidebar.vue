<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {api} from '../api'
import {useWorkspace} from '../workspace'
import {airportNodeCounts} from '../airportViewing'
import {date,type LatencyBatch} from '../domain'
const emit=defineEmits<{select:[id:string]}>(),w=useWorkspace(),search=ref(''),collapsed=ref(false),batches=ref<LatencyBatch[]>([])
const counts=computed(()=>airportNodeCounts(w.airports.value)),total=computed(()=>Object.values(counts.value).reduce((a,b)=>a+b,0))
const visible=computed(()=>w.airports.value.filter(a=>a.name.toLowerCase().includes(search.value.trim().toLowerCase())))
const cache=new Map<string,LatencyBatch>()
watch(()=>w.batches.value,async(rows,_,onCleanup)=>{
 let cancelled=false;onCleanup(()=>cancelled=true)
 try{
  const read:LatencyBatch[]=[],covered=new Set<string>()
  for(const row of rows){
   if(row.state!=='completed')continue
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
 <button type="button" :aria-pressed="w.airportFilter.value==='all'" @click="emit('select','all')"><span>全部机场</span><small>{{total}}</small></button>
 <button v-for="a in visible" :key="a.id" type="button" :title="health[a.id].label" :aria-pressed="w.airportFilter.value===a.id" @click="emit('select',a.id)"><i class="airport-health" :class="health[a.id].tone" :aria-label="health[a.id].label"/><span>{{a.name}}</span><small>{{counts[a.id]||0}}</small></button>
 <p v-if="!visible.length" class="muted small">没有匹配机场</p></template>
</section></template>
<style scoped>
.airport-search{width:100%;height:26px;min-height:26px;padding:3px 8px;font-size:11px;margin:2px 0 4px}
.airport-heading{font-weight:600;color:var(--cst-muted)!important}
.airport-health{width:6px;height:6px;flex:0 0 6px;border-radius:50%;background:var(--cst-muted)}
.airport-health.good{background:var(--cst-good)}.airport-health.warn{background:var(--cst-warn)}.airport-health.bad{background:var(--cst-bad)}
</style>
