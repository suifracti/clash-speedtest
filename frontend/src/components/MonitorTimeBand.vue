<script setup lang="ts">
import {computed,onBeforeUnmount,ref,watch} from 'vue'
import {api} from '../api'
import type {MonitorJob,MonitorSample} from '../domain'
// 离散时间带：48 个半小时段；斜纹始终表示尚未完整读取，不推断没有采样。
const props=defineProps<{job:MonitorJob}>()
const SLOTS=48,HOURS=24
const samples=ref<MonitorSample[]>([]),hasMore=ref(false),error=ref(''),loading=ref(false),until=ref(Date.now()),hasRead=ref(false),coverageStart=ref<number|null>(null)
const signature=computed(()=>JSON.stringify([props.job.id,props.job.profile_id,(props.job.nodes||[]).map(n=>[n.node_key,n.node_identity_key,n.config_revision_key]).sort()]))
let generation=0
async function load(){
 const token=++generation,end=Date.now(),job=props.job
 const scopes=new Set((job.nodes||[]).filter(n=>n.node_identity_key&&n.config_revision_key&&n.node_key).map(n=>JSON.stringify([job.profile_id,n.node_key,n.node_identity_key,n.config_revision_key])))
 loading.value=true;error.value=''
 try{
  const r=await api.monitorSamples({since:new Date(end-HOURS*3600000).toISOString(),until:new Date(end).toISOString(),profile_id:job.profile_id,limit:500,order_desc:true})
  if(token!==generation)return
  const raw=r.items||[],times=raw.map(s=>Date.parse(s.timestamp)).filter(Number.isFinite)
  // 用原始页的时间边界确定覆盖范围，不能用筛选后的节点样本估计分页边界。
  coverageStart.value=r.has_more?(times.length?Math.min(...times):end):end-HOURS*3600000
  samples.value=raw.filter(s=>scopes.has(JSON.stringify([s.profile_id,s.node_key,s.node_identity_key,s.config_revision_key])))
  hasMore.value=r.has_more;until.value=end;hasRead.value=true
 }catch(e){if(token===generation)error.value=e instanceof Error?e.message:String(e)}
 finally{if(token===generation)loading.value=false}
}
watch(()=>[signature.value,props.job.updated_at],(next,previous)=>{
 if(!previous||next[0]!==previous[0]){samples.value=[];hasMore.value=false;hasRead.value=false;coverageStart.value=null;until.value=Date.now()}
 void load()
},{immediate:true})
onBeforeUnmount(()=>generation++)
const slots=computed(()=>{
 const start=until.value-HOURS*3600000,size=HOURS*3600000/SLOTS
 return Array.from({length:SLOTS},(_,i)=>{
  const a=start+i*size,b=a+size,inSlot=samples.value.filter(s=>{const t=Date.parse(s.timestamp);return t>=a&&t<b&&s.error_class!=='not_executed'})
  const ok=inSlot.filter(s=>s.success).length,label=new Date(a).toLocaleTimeString('zh-CN',{timeZone:'Asia/Shanghai',hour:'2-digit',minute:'2-digit'})
  // 跨越分页边界的整格也不完整；保留已读计数，但不着成功/失败色。
  if(coverageStart.value===null||(hasMore.value&&a<=coverageStart.value))return {tone:'unread',title:`${label} · 未完整读取${inSlot.length?` · 已读 ${ok}/${inSlot.length} 次有响应`:''}`}
  if(!inSlot.length)return {tone:'none',title:`${label} · 没有已执行采样`}
  return {tone:ok===inSlot.length?'good':ok?'warn':'bad',title:`${label} · ${ok}/${inSlot.length} 次有响应`}
 })
})
const last=computed(()=>samples.value.length?new Date(Math.max(...samples.value.map(s=>Date.parse(s.timestamp)))).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}):'')
const readTime=computed(()=>new Date(until.value).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}))
const note=computed(()=>{
 if(error.value)return `采样读取失败：${error.value}${hasRead.value?`；保留上次成功读取 ${readTime.value} 的时间带`:''}`
 if(loading.value&&!hasRead.value)return '正在读取采样…'
 const count=samples.value.length?`最近采样 ${last.value} · 已读 ${samples.value.length} 个样本`:hasMore.value?'已读页内没有匹配样本，较早时段未完整读取':'最近 24 小时没有匹配的已保存样本'
 return `${count}${hasMore.value?'；只读该账户最新一页，斜纹时段未完整读取':''}${loading.value?' · 刷新中…':''}`
})
</script>
<template>
  <div class="cst-band">
    <div class="cst-band-strip" role="img" :aria-label="`最近 24 小时采样时间带，${samples.length} 个已读样本`"><span v-for="(s,i) in slots" :key="i" :class="s.tone" :title="s.title"/></div>
    <div class="cst-band-axis"><span>读取前 24 小时</span><span>12 小时前</span><span :title="hasRead?'读取截至 '+readTime:undefined">{{hasRead?'读取时刻':'尚未读取'}}</span></div>
    <small class="cst-band-note" :class="{bad:!!error}">{{note}}</small>
    <small class="cst-band-note">按任务冻结配置匹配；可含其他任务的同配置样本。斜纹为未读完整，空白为没有已执行采样。</small>
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
