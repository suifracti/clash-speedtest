<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref,watch} from 'vue'
import {api,type SubscriptionRefreshJob,type SubscriptionRefreshRequest} from '../api'
import {bytes,type Airport,type NodeOption} from '../domain'
import {airportAccounts,noticesForAccount,safeAirportLink,type AirportAccount} from '../airportViewing'
import {useWorkspace} from '../workspace'
import UiSelect from './UiSelect.vue'
import Modal from './Modal.vue'

const props=defineProps<{airports:Airport[];notices:NodeOption[]}>()
const workspace=useWorkspace(),accounts=computed(()=>airportAccounts(props.airports)),selectedKey=ref(''),jobs=ref<SubscriptionRefreshJob[]>([]),activeJob=ref<SubscriptionRefreshJob|null>(null),loading=ref(false),noticeOpen=ref(false),error=ref('')
const selected=computed(()=>accounts.value.find(account=>account.key===selectedKey.value)),accountNotices=computed(()=>noticesForAccount(props.notices,selected.value)),usage=computed(()=>selected.value?.subscription.usage),used=computed(()=>usage.value?(usage.value.upload||0)+(usage.value.download||0):null),remaining=computed(()=>usage.value?.total===undefined?null:Math.max(0,usage.value.total-(used.value||0)))
const links=computed(()=>{
  const airport=selected.value?.airport
  if(!airport)return []
  const values=[{label:'官网',url:airport.website_url},{label:'备用入口',url:airport.backup_url},...(airport.maintenance?.links||[]).map(link=>({label:link.label,url:link.url}))]
  return values.map(link=>({...link,url:safeAirportLink(link.url)})).filter(link=>!!link.label&&!!link.url)
})
const visibleJob=computed(()=>{
  const account=selected.value
  if(!account)return null
  const jobMatches=(job:SubscriptionRefreshJob)=>job.items.find(item=>item.airport_id===account.airport.id&&item.subscription_id===account.subscription.id)
  if(activeJob.value&&jobMatches(activeJob.value))return activeJob.value
  return [...jobs.value].sort((a,b)=>Date.parse(b.created_at)-Date.parse(a.created_at)).find(jobMatches)||null
})
const visibleItem=computed(()=>{
  const account=selected.value,job=visibleJob.value
  return account&&job?job.items.find(item=>item.airport_id===account.airport.id&&item.subscription_id===account.subscription.id)||null:null
})
const expireLabel=computed(()=>{
  const expire=usage.value?.expire
  if(!expire)return ''
  const at=new Date(expire*1000)
  if(!Number.isFinite(at.getTime()))return ''
  return at.getTime()<=Date.now()?`已过期 · ${at.toLocaleDateString('zh-CN')}`:`到期 ${at.toLocaleDateString('zh-CN')}`
})
const jobRunning=(job:SubscriptionRefreshJob|null)=>job?.state==='running'
let pollTimer:ReturnType<typeof setTimeout>|undefined,disposed=false
function mergeJob(job:SubscriptionRefreshJob){jobs.value=[job,...jobs.value.filter(existing=>existing.id!==job.id)];if(activeJob.value?.id===job.id||job.state==='running')activeJob.value=job}
function schedulePoll(id:string){clearTimeout(pollTimer);pollTimer=setTimeout(()=>void poll(id),700)}
async function poll(id:string){if(disposed)return;try{const job=await api.refreshJob(id);if(disposed)return;mergeJob(job);if(jobRunning(job))schedulePoll(id);else{activeJob.value=job;if(job.success)await workspace.refreshNodes()}}catch(e){if(!disposed)error.value=e instanceof Error?e.message:String(e)}}
async function loadJobs(){try{const loaded=await api.refreshJobs();if(disposed)return;jobs.value=loaded||[];const running=jobs.value.find(jobRunning);if(running){activeJob.value=running;schedulePoll(running.id)}}catch(e){if(!disposed)error.value=e instanceof Error?e.message:String(e)}}
function requestID(){return globalThis.crypto?.randomUUID?.()||`refresh-${Date.now()}-${Math.random().toString(16).slice(2)}`}
async function startCurrent(){const account=selected.value;if(!account||loading.value||jobRunning(activeJob.value))return;loading.value=true;error.value='';try{const body:SubscriptionRefreshRequest={request_id:requestID(),selections:[{airport_id:account.airport.id,subscription_id:account.subscription.id}]};const job=await api.startRefresh(body);mergeJob(job);if(jobRunning(job))schedulePoll(job.id);else if(job.success)await workspace.refreshNodes()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}}
async function cancelCurrent(){const job=visibleJob.value;if(!job||!jobRunning(job))return;try{mergeJob(await api.cancelRefresh(job.id))}catch(e){error.value=e instanceof Error?e.message:String(e)}}
async function retryFailed(){const job=visibleJob.value;if(!job||!visibleItem.value||visibleItem.value.state!=='failed'||loading.value)return;loading.value=true;error.value='';try{const next=await api.startRefresh({request_id:requestID(),retry_of:job.id});mergeJob(next);if(jobRunning(next))schedulePoll(next.id)}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}}
watch(accounts,value=>{if(!value.some(account=>account.key===selectedKey.value))selectedKey.value=value[0]?.key||''},{immediate:true})
onMounted(()=>void loadJobs())
onBeforeUnmount(()=>{disposed=true;clearTimeout(pollTimer)})
</script>

<template>
  <section class="subscription-overview" aria-label="首页订阅账户概览">
    <header class="subscription-overview-header"><div><h2>订阅账户</h2><p>独立查看用量、到期时间和刷新结果，不改变机场与节点选择。</p></div><label v-if="accounts.length">首页用量订阅<UiSelect v-model="selectedKey" aria-label="首页用量订阅"><option v-for="entry in accounts" :key="entry.key" :value="entry.key">{{entry.airport.name}} · {{entry.subscription.name}}</option></UiSelect></label></header>
    <p v-if="!selected" class="muted">先在机场选择中选择要查看的机场。</p>
    <template v-else>
      <div class="subscription-overview-grid">
        <div class="subscription-overview-account"><strong>{{selected.airport.name}} · {{selected.subscription.name}}</strong><span v-if="used!==null">已用 {{bytes(used)}}<template v-if="usage?.total!==undefined"> / {{bytes(usage.total)}} · 剩余 {{bytes(remaining||0)}}</template></span><span v-else>暂无用量记录</span><span v-if="expireLabel">{{expireLabel}}</span><div v-if="usage?.total!==undefined" class="usage-track"><i :style="{width:`${Math.min(100,(used||0)/usage.total*100)}%`}"/></div></div>
        <div class="subscription-overview-links"><strong>机场入口</strong><div><a v-for="link in links" :key="link.label+link.url" :href="link.url" target="_blank" rel="noopener noreferrer">{{link.label}} ↗</a><span v-if="!links.length" class="muted">未配置</span></div></div>
        <div class="subscription-overview-refresh"><button class="button primary" :disabled="loading||jobRunning(activeJob)" @click="startCurrent">{{loading?'提交中…':'刷新当前订阅'}}</button><span v-if="visibleItem" class="badge" :class="visibleItem.state==='succeeded'?'good':visibleItem.state==='failed'?'bad':'warn'">{{visibleItem.state==='succeeded'?'刷新成功':visibleItem.state==='failed'?'刷新失败':visibleItem.state==='not_executed'?'未执行':visibleItem.state==='cancelled'?'已取消':'刷新中'}}</span><p v-if="visibleItem?.error_message" class="inline-error">{{visibleItem.error_message}}</p><div v-if="visibleItem?.stages?.length" class="small muted">{{visibleItem.stages.map(stage=>({queued:'排队',fetching:'抓取',parsing:'解析',saving:'保存',succeeded:'完成',failed:'失败',not_executed:'未执行',cancelled:'已取消'} as Record<string,string>)[stage.state]||stage.state).join(' → ')}}</div><button v-if="visibleItem?.state==='failed'" class="text-button" :disabled="loading" @click="retryFailed">只重试失败项</button><button v-if="visibleJob&&jobRunning(visibleJob)" class="text-button" @click="cancelCurrent">取消刷新</button><small v-if="visibleJob?.persistence_error" class="inline-error">{{visibleJob.persistence_error}}</small></div>
      </div>
      <div class="subscription-overview-notices"><button v-if="accountNotices.length" class="text-button" @click="noticeOpen=true">公告 {{accountNotices.length}}</button><span v-else class="muted small">此账户没有识别到公告</span><span v-if="error" class="inline-error" role="alert">{{error}}</span></div>
      <Modal v-if="noticeOpen" title="订阅账户公告" @close="noticeOpen=false"><div v-for="notice in accountNotices" :key="notice.profile_id+'|'+notice.node_identity_key+'|'+notice.config_revision_key" class="notice-row"><small>{{selected.airport.name}} · {{selected.subscription.name}}</small><p>{{notice.display_name}}</p></div></Modal>
    </template>
  </section>
</template>

<style scoped>
.subscription-overview{display:grid;gap:14px;margin:18px 0;padding:18px;border:1px solid var(--line,#d9dee8);border-radius:16px;background:var(--surface,#fff)}
.subscription-overview-header{display:flex;align-items:center;justify-content:space-between;gap:16px}.subscription-overview-header h2{margin:0}.subscription-overview-header p{margin:5px 0 0;color:var(--muted,#687386);font-size:.9rem}.subscription-overview-header label{display:grid;gap:5px;min-width:min(280px,45vw)}
.subscription-overview-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.subscription-overview-grid>div{display:grid;align-content:start;gap:9px;padding:12px;border-radius:12px;background:var(--surface-muted,#f5f7fa)}.subscription-overview-links>div{display:flex;flex-wrap:wrap;gap:12px}.subscription-overview-refresh{justify-items:start}.subscription-overview-notices{display:flex;align-items:center;gap:12px}.usage-track{height:5px;overflow:hidden;border-radius:5px;background:#e4e8ef}.usage-track i{display:block;height:100%;background:var(--accent,#5476d1)}
@media(max-width:760px){.subscription-overview-header{align-items:stretch;flex-direction:column}.subscription-overview-grid{grid-template-columns:1fr}.subscription-overview-header label{min-width:0}}
</style>
