<script setup lang="ts">
import {computed,onBeforeUnmount,ref,watch} from 'vue'
import {api} from '../api'
import type {MonitorJob,MonitorSample} from '../domain'
// 离散时间带：最近 24 小时分成 48 段。空白=这段时间没有采样；斜纹=这段没有读到（只读最近一页）。
const props=defineProps<{job:MonitorJob}>()
const SLOTS=48,HOURS=24
const samples=ref<MonitorSample[]>([]),hasMore=ref(false),error=ref(''),loading=ref(false),until=ref(Date.now())
let generation=0
async function load(){
 const token=++generation;loading.value=true;error.value='';until.value=Date.now()
 try{
  const r=await api.monitorSamples({since:new Date(until.value-HOURS*3600000).toISOString(),until:new Date(until.value).toISOString(),profile_id:props.job.profile_id,limit:500,order_desc:true})
  if(token!==generation)return
  const ids=new Set((props.job.nodes||[]).map(n=>n.node_identity_key).filter(Boolean)),keys=new Set(props.job.node_keys||[])
  samples.value=(r.items||[]).filter(s=>ids.has(s.node_identity_key)||keys.has(s.node_key));hasMore.value=r.has_more
 }catch(e){if(token===generation)error.value=e instanceof Error?e.message:String(e)}
 finally{if(token===generation)loading.value=false}
}
watch(()=>[props.job.id,props.job.updated_at],()=>void load(),{immediate:true})
onBeforeUnmount(()=>generation++)
const slots=computed(()=>{
 const start=until.value-HOURS*3600000,size=HOURS*3600000/SLOTS
 // 只读了最近一页时，早于最早一条已读样本的时段算“未读取”，不当作没有采样。
 const oldest=hasMore.value&&samples.value.length?Math.min(...samples.value.map(s=>Date.parse(s.timestamp))):-Infinity
 return Array.from({length:SLOTS},(_,i)=>{
  const a=start+i*size,b=a+size,inSlot=samples.value.filter(s=>{const t=Date.parse(s.timestamp);return t>=a&&t<b&&s.error_class!=='not_executed'})
  const ok=inSlot.filter(s=>s.success).length,label=new Date(a).toLocaleTimeString('zh-CN',{timeZone:'Asia/Shanghai',hour:'2-digit',minute:'2-digit'})
  if(b<=oldest)return {tone:'unread',title:`${label} · 未读取`}
  if(!inSlot.length)return {tone:'none',title:`${label} · 没有采样`}
  return {tone:ok===inSlot.length?'good':ok?'warn':'bad',title:`${label} · ${ok}/${inSlot.length} 次有响应`}
 })
})
const last=computed(()=>samples.value.length?new Date(Math.max(...samples.value.map(s=>Date.parse(s.timestamp)))).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}):'')
</script>
<template>
  <div class="cst-band">
    <div class="cst-band-strip" role="img" :aria-label="`最近 24 小时采样时间带，${samples.length} 个已读样本`"><span v-for="(s,i) in slots" :key="i" :class="s.tone" :title="s.title"/></div>
    <div class="cst-band-axis"><span>24 小时前</span><span>12 小时前</span><span>现在</span></div>
    <small class="cst-band-note" :class="{bad:!!error}">{{error?'采样读取失败：'+error:loading&&!samples.length?'正在读取采样…':samples.length?`最近采样 ${last} · 已读 ${samples.length} 个样本${hasMore?'（只读最近一页，更早时段标为未读取）':''}`:'最近 24 小时没有采样'}}</small>
  </div>
</template>
<style>
.cst-band{display:flex;flex-direction:column;gap:3px;margin:8px 0}
.cst-band-strip{display:grid;grid-template-columns:repeat(48,1fr);gap:2px;height:18px}
.cst-band-strip span{border-radius:2px;background:transparent;box-shadow:inset 0 0 0 1px var(--cst-line)}
.cst-band-strip .good{background:var(--cst-good);box-shadow:none}
.cst-band-strip .warn{background:var(--cst-warn);box-shadow:none}
.cst-band-strip .bad{background:var(--cst-bad);box-shadow:none}
.cst-band-strip .unread{background:repeating-linear-gradient(-45deg,var(--cst-line) 0 2px,transparent 2px 4px)}
.cst-band-axis{display:flex;justify-content:space-between;font-size:10px;color:var(--cst-subtle)}
.cst-band-note{font-size:11px;color:var(--cst-muted)}
.cst-band-note.bad{color:var(--cst-bad)}
</style>
