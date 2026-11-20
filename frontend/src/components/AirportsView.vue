<script setup lang="ts">
import SubscriptionChanges from './SubscriptionChanges.vue'
import AirportReport from './AirportReport.vue'
import ClashVergeImport from './ClashVergeImport.vue'
import UiSelect from './UiSelect.vue'
import {computed,onBeforeUnmount,onMounted,ref,watch} from 'vue'
import {api,desktop,request,type SubscriptionRefreshItem,type SubscriptionRefreshJob} from '../api'
import {useWorkspace} from '../workspace'
import {bytes,date,isNotice,subscriptionUsed,type Airport,type Subscription} from '../domain'
import Modal from './Modal.vue'
import Icon from './Icon.vue'
import SubscriptionUsageView from './SubscriptionUsageView.vue'
import {airportAccounts,noticesForAccount,parseNoticeUsage,maintenanceNoticeLinks} from '../airportViewing'
import {readUsage,usageRefreshError,shanghaiDay,shiftDay,type UsageSnapshot} from '../usageDaily'
import UsageDays from './UsageDays.vue'
const section=ref('manage'),vergeOpen=ref(false)
const noticeBalances=ref<Record<string,number|undefined>>({})
interface SourceDraft {kind:'airport'|'subscription';airportId?:string;airportName?:string;subId?:string;name:string;url:string;website_url:string;backup_url:string;note:string;needsSource:boolean;sourceReady:boolean}
const w=useWorkspace(),busy=ref(''),search=ref(''),form=ref<SourceDraft|null>(null),formError=ref(''),sourceLoading=ref(false),remove=ref<{airport:Airport;sub?:Subscription}|null>(null),maintenance=ref<{airport:Airport;settings:NonNullable<Airport['maintenance']>}|null>(null),refreshJob=ref<SubscriptionRefreshJob|null>(null),refreshProgressVisible=ref(false),refreshLoading=ref(false)
const requestSettings=ref<{airport:Airport;sub:Subscription;userAgent:string}|null>(null),requestError=ref('')
const maintenanceError=ref(''),maintenanceErrorAirport=ref(''),maintenanceNow=ref(new Date()),removeError=ref(''),removeLoading=ref(false),removeImpactError=ref('')
const maintenanceMonth=computed(()=>`${maintenanceNow.value.getFullYear()}-${String(maintenanceNow.value.getMonth()+1).padStart(2,'0')}`)
const removeProfiles=computed(()=>new Set(remove.value?(remove.value.sub?[remove.value.sub.id]:[remove.value.airport.id,...(remove.value.airport.subscriptions||[]).map(s=>s.id)]):[]))
const affectedJobs=computed(()=>remove.value?w.jobs.value.filter(job=>removeProfiles.value.has(job.profile_id)):[])
const formTitle=computed(()=>form.value?.kind==='airport'?(form.value.airportId?'编辑机场资料':'添加机场'):(form.value?.subId?'编辑订阅账户':'添加订阅账户'))
const visible=computed(()=>w.airports.value.filter(a=>(a.name+' '+(a.note||'')).includes(search.value)))
const viewingAirport=ref('')
const focusedAirport=computed(()=>visible.value.find(a=>a.id===viewingAirport.value)||visible.value[0])
const allSubscriptions=computed(()=>w.airports.value.flatMap(a=>a.subscriptions||[]))
const accountNotices=computed(()=>w.nodes.value.filter(isNotice))
const totalAccounts=computed(()=>airportAccounts(w.airports.value).length)
const totalNodes=computed(()=>w.airports.value.reduce((n,x)=>n+(x.node_count||0),0))
const knownQuotaStats=computed(()=>{
  let sum=0,count=0
  for(const a of w.airports.value){
    for(const s of a.subscriptions||[]){
      const rem=remaining(s)??noticeUsage(a,s).remaining??null
      if(rem!==null){sum+=rem;count++}
    }
  }
  return {sum:count>0?sum:null,count}
})
const expiringSoonCount=computed(()=>allSubscriptions.value.filter(s=>s.usage?.expire&&expireDays(s.usage.expire)<=7).length)
const safeLink=(v?:string)=>/^https?:\/\//i.test(v||'')?v:undefined
let refreshTimer:ReturnType<typeof setTimeout>|undefined,maintenanceTimer:ReturnType<typeof setInterval>|undefined,disposed=false,editGeneration=0,removeGeneration=0
function closeForm(){if(busy.value==='form')return;editGeneration++;form.value=null;sourceLoading.value=false}
async function loadFormSource(){
 const f=form.value;if(!f?.airportId||!f.needsSource||sourceLoading.value)return
 const generation=++editGeneration;sourceLoading.value=true;f.sourceReady=false;formError.value=''
 try{
  const result=await request<{url:string}|string>(f.subId?`/api/airports/${f.airportId}/subscriptions/${f.subId}/url`:`/api/airports/${f.airportId}/url`,'GET',undefined,f.subId?'GetSubscriptionURL':'GetAirportURL',f.subId?[f.airportId,f.subId]:[f.airportId])
  if(disposed||generation!==editGeneration||form.value!==f)return
  f.url=typeof result==='string'?result:result.url;f.sourceReady=true
 }catch(e){if(!disposed&&generation===editGeneration)formError.value=e instanceof Error?e.message:String(e)}
 finally{if(generation===editGeneration)sourceLoading.value=false}
}
function edit(airport?:Airport,sub?:Subscription){
 if(busy.value)return
 editGeneration++;formError.value='';sourceLoading.value=false
 if(!airport){form.value={kind:'airport',name:'',url:'',website_url:'',backup_url:'',note:'',needsSource:true,sourceReady:true};return}
 const needsSource=!!sub||!airport.subscriptions?.length&&!!airport.url_display
 form.value={kind:sub?'subscription':'airport',airportId:airport.id,airportName:airport.name,subId:sub?.id,name:sub?sub.name:airport.name,url:'',website_url:airport.website_url||'',backup_url:airport.backup_url||'',note:sub?(sub.note||''):(airport.note||''),needsSource,sourceReady:!needsSource}
 if(needsSource)void loadFormSource()
}
function addSub(airport:Airport){if(busy.value)return;editGeneration++;sourceLoading.value=false;formError.value='';form.value={kind:'subscription',airportId:airport.id,airportName:airport.name,name:'',url:'',website_url:'',backup_url:'',note:'',needsSource:true,sourceReady:true}}
async function save(){
 const draft=form.value;if(!draft||busy.value||!draft.sourceReady)return
 const f={...draft};formError.value=''
 if(!f.name.trim()||f.needsSource&&!f.url.trim()){formError.value=f.needsSource?'请填写名称和订阅地址或配置文件路径':'请填写机场名称';return}
 busy.value='form'
 try{
  if(f.kind==='airport'){
   const body={name:f.name,url:f.needsSource?f.url:'',website_url:f.website_url,backup_url:f.backup_url,note:f.note}
   const result=await request<Airport>(f.airportId?`/api/airports/${f.airportId}`:'/api/airports',f.airportId?'PUT':'POST',body,f.airportId?'UpdateFrontendAirport':'CreateFrontendAirport',f.airportId?[f.airportId,body]:[body])
   viewingAirport.value=result?.id||f.airportId||'';search.value=''
  }else await request(`/api/airports/${f.airportId}/subscriptions${f.subId?'/'+f.subId:''}`,f.subId?'PUT':'POST',{name:f.name,url:f.url,note:f.note},f.subId?'UpdateSubscription':'AddSubscription',f.subId?[f.airportId,f.subId,f.name,f.url,f.note]:[f.airportId,f.name,f.url,f.note])
  editGeneration++;form.value=null;w.notify(f.kind==='airport'&&f.airportId?'机场资料已保存':'订阅已保存')
  try{await w.refreshNodes()}catch(e){w.fail(`已保存，但列表刷新失败：${e instanceof Error?e.message:String(e)}。可重新加载列表，无需重复保存。`)}
 }catch(e){formError.value=e instanceof Error?e.message:String(e)}
 finally{busy.value=''}
}
function openRequestSettings(airport:Airport,sub:Subscription){if(busy.value)return;requestError.value='';requestSettings.value={airport,sub,userAgent:sub.user_agent||''}}
async function saveRequestSettings(){
 const draft=requestSettings.value;if(!draft||busy.value)return
 const {airport,sub}=draft,userAgent=draft.userAgent.trim();busy.value='request-settings';requestError.value=''
 try{
  const result=await request<Subscription>(`/api/airports/${airport.id}/subscriptions/${sub.id}/user-agent`,'PUT',{user_agent:userAgent},'SetSubscriptionUserAgent',[airport.id,sub.id,userAgent])
  const current=w.airports.value.find(a=>a.id===airport.id)?.subscriptions?.find(s=>s.id===sub.id)
  if(current)current.user_agent=result.user_agent||''
  requestSettings.value=null;w.notify('请求设置已保存，下次获取订阅时生效')
 }catch(e){requestError.value=e instanceof Error?e.message:String(e)}
 finally{busy.value=''}
}
const refreshState=(state:string)=>({queued:'排队中',fetching:'抓取中',parsing:'解析中',saving:'保存中',succeeded:'成功',failed:'失败',cancelled:'已取消',not_executed:'未执行',interrupted:'中断'} as Record<string,string>)[state]||state
const failedSnapshots=ref<UsageSnapshot[]>([])
watch(()=>JSON.stringify([refreshProgressVisible.value,refreshJob.value?.id,refreshJob.value?.items.filter(item=>item.state==='failed').map(item=>[item.airport_id,item.subscription_id,item.finished_at])]),async(_,__,onCleanup)=>{
  failedSnapshots.value=[]
  if(!refreshProgressVisible.value||!refreshJob.value?.items.some(item=>item.state==='failed'))return
  let current=true;onCleanup(()=>{current=false})
  const today=shanghaiDay()
  try{const report=await readUsage({from:shiftDay(today,-13),until:today,period:'day'},0);if(current)failedSnapshots.value=report.accounts}
  catch{/* Keep the task's existing message if the saved snapshot is unavailable. */}
},{immediate:true})
function refreshItemError(item:SubscriptionRefreshItem){
  if(item.state!=='failed')return item.error_message
  const snapshot=failedSnapshots.value.find(s=>s.airport_id===item.airport_id&&s.subscription_id===item.subscription_id&&s.status==='refresh_failed'&&s.raw_error)
  if(!snapshot)return item.error_message
  const at=Date.parse(snapshot.captured_at)
  if(item.started_at&&at<Date.parse(item.started_at)||item.finished_at&&at>Date.parse(item.finished_at))return item.error_message
  return usageRefreshError(snapshot)
}
function mergeRefresh(job:SubscriptionRefreshJob,openProgress=false){refreshJob.value=job;if(openProgress)refreshProgressVisible.value=true;if(job.state==='running')scheduleRefreshPoll(job.id);else clearTimeout(refreshTimer)}
function scheduleRefreshPoll(id:string){clearTimeout(refreshTimer);refreshTimer=setTimeout(()=>void pollRefresh(id),700)}
async function pollRefresh(id:string){if(disposed)return;try{const job=await api.refreshJob(id);if(disposed)return;mergeRefresh(job);if(job.state!=='running'){await w.refreshNodes();if(job.failed===0)w.notify('订阅已刷新，选择与节点范围已同步')}}catch(e){if(!disposed)w.fail(e)}}
// 打开页面只读本地缓存与刷新记录，不自动发起订阅刷新，也不自动弹进度框。
async function loadRefreshJobs(){try{const jobs=await api.refreshJobs();if(disposed)return;const current=jobs.find(job=>job.state==='running')||jobs.find(job=>job.failed>0);if(current)mergeRefresh(current,false)}catch(e){if(!disposed)w.fail(e)}}
function refreshRequestID(){return globalThis.crypto?.randomUUID?.()||`refresh-${Date.now()}-${Math.random().toString(16).slice(2)}`}
async function refresh(a:Airport,s?:Subscription){if(refreshLoading.value||refreshJob.value?.state==='running')return;busy.value=s?.id||a.id;refreshLoading.value=true;try{const accounts=airportAccounts([a]),selected=s?accounts.find(entry=>entry.subscription.id===s.id):undefined,requestBody=s?{request_id:refreshRequestID(),selections:[{airport_id:a.id,subscription_id:s.id}]}:{request_id:refreshRequestID(),selections:accounts.map(entry=>({airport_id:a.id,subscription_id:entry.subscription.id}))};if(s&&!selected)throw new Error('订阅已变化，请重新载入机场列表');mergeRefresh(await api.startRefresh(requestBody),true);if(refreshJob.value?.state!=='running'){await w.refreshNodes();w.notify('订阅刷新任务已完成')}}catch(e){w.fail(e)}finally{busy.value='';refreshLoading.value=false}}
const hm=(value?:string)=>!value||value.startsWith('0001')?'':new Date(value).toLocaleTimeString('zh-CN',{timeZone:'Asia/Shanghai',hour:'2-digit',minute:'2-digit',hour12:false})
async function refreshUsage(a:Airport,s:Subscription){
  busy.value=s.id
  try{
    const result=await request<{observation:{status:string;remaining?:number};recently_refreshed?:boolean}>(`/api/airports/${a.id}/subscriptions/${s.id}/usage/refresh`,'POST',undefined,'RefreshSubscriptionUsage',[a.id,s.id])
    noticeBalances.value[s.id]=result.observation.remaining
    await w.refreshNodes()
    if(!result.recently_refreshed)w.notify(result.observation.status==='ok'?'账户用量已刷新':result.observation.status==='partial'?'已读取部分用量':'机场未返回用量')
  }catch{
    // 失败不弹窗：保留上一次快照，行内只显示“上次刷新 HH:MM”。
    try{await w.refreshNodes()}catch{}
  }finally{busy.value=''}
}

async function cancelRefresh(){const job=refreshJob.value;if(!job||job.state!=='running')return;clearTimeout(refreshTimer);try{mergeRefresh(await api.cancelRefresh(job.id))}catch(e){w.fail(e)}}
async function retryRefresh(){const job=refreshJob.value;if(!job||job.failed===0||refreshLoading.value)return;refreshLoading.value=true;try{mergeRefresh(await api.startRefresh({request_id:refreshRequestID(),retry_of:job.id}),true)}catch(e){w.fail(e)}finally{refreshLoading.value=false}}
function closeRemove(){if(busy.value==='delete')return;removeGeneration++;remove.value=null;removeLoading.value=false}
async function openRemove(airport:Airport,sub?:Subscription){
 if(busy.value)return
 const generation=++removeGeneration;remove.value={airport,sub};removeError.value='';removeImpactError.value='';removeLoading.value=true
 try{await w.refreshJobs()}catch(e){if(generation===removeGeneration)removeImpactError.value=`未能读取关联监测：${e instanceof Error?e.message:String(e)}。任务不会随来源一起删除。`}
 finally{if(generation===removeGeneration)removeLoading.value=false}
}
async function deleteItem(){
 const item=remove.value;if(!item||busy.value||removeLoading.value)return
 const profiles=new Set(removeProfiles.value);busy.value='delete';removeError.value=''
 try{
  await request(item.sub?`/api/airports/${item.airport.id}/subscriptions/${item.sub.id}`:`/api/airports/${item.airport.id}`,'DELETE',undefined,item.sub?'DeleteSubscription':'DeleteAirport',item.sub?[item.airport.id,item.sub.id]:[item.airport.id])
  if(item.sub){
   const current=w.airports.value.find(a=>a.id===item.airport.id)
   if(current){current.subscriptions=(current.subscriptions||[]).filter(s=>s.id!==item.sub!.id);current.node_count=current.subscriptions.reduce((sum,s)=>sum+s.node_count,0);current.has_cache=current.subscriptions.some(s=>s.has_cache);current.url_display=current.subscriptions[0]?.url_display||''}
  }else{w.airports.value=w.airports.value.filter(a=>a.id!==item.airport.id);if(w.selectedAirportIds.value)w.selectedAirportIds.value=w.selectedAirportIds.value.filter(id=>id!==item.airport.id)}
  w.nodes.value=w.nodes.value.filter(node=>!profiles.has(node.profile_id))
  if(profiles.has(w.subscription.value))w.subscription.value='all'
  removeGeneration++;remove.value=null;w.notify(`${item.sub?'账户':'机场'}已删除，历史记录保留`)
  const refreshed=await Promise.allSettled([w.refreshNodes(),w.refreshJobs()])
  const failed=refreshed.flatMap((r,i)=>r.status==='rejected'?[`${i===0?'账户与节点':'监测任务'}：${r.reason instanceof Error?r.reason.message:String(r.reason)}`]:[])
  if(failed.length)w.fail(`已删除，但部分列表刷新失败：${failed.join('；')}。无需重复删除。`)
 }catch(e){removeError.value=e instanceof Error?e.message:String(e)}
 finally{busy.value=''}
}
type Maintenance=NonNullable<Airport['maintenance']>
function maintenanceCopy(a:Airport):Maintenance{return JSON.parse(JSON.stringify({...a.maintenance,refresh_hours:a.maintenance?.refresh_hours||0,links:a.maintenance?.links||[]}))}
function openMaintenance(a:Airport){if(busy.value)return;maintenanceError.value='';maintenanceErrorAirport.value=a.id;maintenance.value={airport:a,settings:maintenanceCopy(a)}}
function reminderDue(link:Maintenance['links'][number]){return link.monthly_day>0&&maintenanceNow.value.getDate()>=link.monthly_day&&link.done_month!==maintenanceMonth.value}
function maintenanceTime(value?:string){return value&&!value.startsWith('0001')?date(value):'尚未安排'}
function discoveredLinks(a:Airport){return maintenanceNoticeLinks(a,accountNotices.value)}
async function persistMaintenance(a:Airport,value:Maintenance,message:string){
 if(busy.value)return
 const settings:Maintenance=JSON.parse(JSON.stringify(value));busy.value='maintenance';maintenanceError.value='';maintenanceErrorAirport.value=a.id
 try{
  await request(`/api/airports/${a.id}/maintenance`,'PUT',settings,'SaveAirportMaintenance',[a.id,settings])
  const current=w.airports.value.find(item=>item.id===a.id)
  if(current){const old=current.maintenance;current.maintenance={...old,refresh_hours:settings.refresh_hours,links:settings.links,next_refresh:old?.refresh_hours===settings.refresh_hours?old?.next_refresh:undefined}}
  maintenance.value=null;w.notify(message)
  try{await w.refreshNodes()}catch(e){w.fail(`已保存，但列表刷新失败：${e instanceof Error?e.message:String(e)}。无需重复保存。`)}
 }catch(e){maintenanceError.value=e instanceof Error?e.message:String(e)}
 finally{busy.value=''}
}
async function saveMaintenance(){const m=maintenance.value;if(m)await persistMaintenance(m.airport,m.settings,'维护设置已保存')}
function toggleReminder(a:Airport,index:number){const next=maintenanceCopy(a),link=next.links[index];if(!link||busy.value)return;link.done_month=link.done_month===maintenanceMonth.value?'':maintenanceMonth.value;void persistMaintenance(a,next,link.done_month?'已标记本月完成':'已撤销本月完成')}
function addDiscoveredLink(a:Airport,link:{label:string;url:string}){const next=maintenanceCopy(a);if(busy.value||next.links.length>=20)return;next.links.push({...link,monthly_day:0});void persistMaintenance(a,next,'已加入常用入口')}
function used(s:Subscription){return subscriptionUsed(s.usage)}
function remaining(s:Subscription){const amount=used(s);return s.usage?.total&&amount!==null?Math.max(0,s.usage.total-amount):null}
function remainingPercent(s:Subscription){const rem=remaining(s);return rem!==null&&s.usage?.total?Math.min(100,Math.max(0,rem/s.usage.total*100)):null}
// 订阅头没有用量时，退回到节点列表里的公告文字（标明来源，不当作订阅返回值）。
function noticeUsage(a:Airport,s:Subscription){const parsed=parseNoticeUsage(noticesForAccount(accountNotices.value,airportAccounts([a]).find(x=>x.subscription.id===s.id)));return noticeBalances.value[s.id]===undefined?parsed:{...parsed,remaining:noticeBalances.value[s.id]}}
function expireDays(t:number){return Math.ceil((t*1000-Date.now())/86400000)}
function expireLabel(t:number){const d=expireDays(t);return d<0?'已过期':d===0?'今天到期':`还剩 ${d} 天`}
function expireTone(t:number){const d=expireDays(t);return d<0?'bad':d<=7?'warn':''}
function browse(a:Airport,s?:Subscription){w.setAirports([a.id]);if(s)w.subscription.value=s.id;w.page.value='home'}
onMounted(()=>{void loadRefreshJobs();maintenanceTimer=setInterval(()=>{maintenanceNow.value=new Date()},60_000)})
onBeforeUnmount(()=>{disposed=true;editGeneration++;removeGeneration++;clearTimeout(refreshTimer);clearInterval(maintenanceTimer)})
</script>
<template><section class="page-view airports-view">
<div class="page-header airports-top-bar">
  <div class="airports-title-group">
    <h1>机场与账户</h1>
    <p>{{w.airports.value.length}} 个机场 · {{totalAccounts}} 个独立账户 · {{totalNodes}} 条缓存配置</p>
  </div>
  <div class="airports-header-right">
    <div class="subscription-view-tabs" role="group" aria-label="机场订阅视图">
      <button class="button" :class="{active:section==='manage'}" :aria-pressed="section==='manage'" @click="section='manage'">账户台账</button>
      <button class="button" :class="{active:section==='usage'}" :aria-pressed="section==='usage'" @click="section='usage'">用量明细</button>
      <button class="button" :class="{active:section==='report'}" :aria-pressed="section==='report'" @click="section='report'">体检</button>
    </div>
    <div v-if="section==='manage'" class="button-group page-header-actions">
      <button v-if="refreshJob&&!refreshProgressVisible" class="button ghost" @click="refreshProgressVisible=true"><Icon name="refresh"/>查看刷新进度</button>
      <button class="button" :disabled="!!busy" @click="vergeOpen=true">从 Clash Verge 导入／同步</button>
      <button class="button primary" :disabled="!!busy" @click="edit()"><Icon name="plus"/>添加机场</button>
    </div>
  </div>
</div>
<ClashVergeImport v-if="vergeOpen" @close="vergeOpen=false"/>
<SubscriptionUsageView v-if="section==='usage'"/>
<AirportReport v-else-if="section==='report'" :initial-airport-id="focusedAirport?.id" @airport-change="viewingAirport=$event;search=''"/>
<template v-else>
<div v-if="w.airports.value.length" class="account-register-summary" aria-label="账户台账概览">
  <div class="summary-stat-card">
    <small>接入来源规模</small>
    <strong>{{w.airports.value.length}} 个机场 · {{totalAccounts}} 个账户</strong>
    <span>共缓存 {{totalNodes}} 条可用节点配置</span>
  </div>
  <div class="summary-stat-card accent">
    <small>有效套餐剩余总额</small>
    <strong>{{knownQuotaStats.sum!==null?bytes(knownQuotaStats.sum):'未知'}}</strong>
    <span>{{knownQuotaStats.count}} 个账户已返回流量余量 · 各账户独立使用</span>
  </div>
  <div class="summary-stat-card" :class="{warn:expiringSoonCount>0}">
    <small>续费与到期状态</small>
    <strong>{{expiringSoonCount?`${expiringSoonCount} 个近期到期`:'全部正常'}}</strong>
    <span>{{expiringSoonCount?`${expiringSoonCount} 个账户已过期或将在 7 天内到期`:'已知到期日中，没有近期到期账户'}}</span>
  </div>
</div>
<div class="account-register">
<aside class="account-register-index" aria-label="机场索引">
<div class="page-toolbar">
  <label class="search-field"><Icon name="search"/><input v-model="search" placeholder="搜索机场名称或备注…" aria-label="搜索机场"></label>
  <span class="muted small">{{visible.length}} / {{w.airports.value.length}} 个机场</span>
</div>
<nav aria-label="查看机场账户"><button v-for="a in visible" :key="a.id" :aria-pressed="focusedAirport?.id===a.id" @click="viewingAirport=a.id"><span class="account-index-initial" aria-hidden="true">{{a.name.slice(0,1)}}</span><span><strong>{{a.name}}</strong><small>{{airportAccounts([a]).length}} 个账户 · {{a.node_count}} 条配置<template v-if="a.maintenance?.links?.some(reminderDue)"> · 有待办</template></small></span><Icon name="chevron"/></button></nav>
</aside>
<div class="cst-ledger">
<article v-for="a in focusedAirport?[focusedAirport]:[]" :key="a.id" class="airport-card cst-ledger-airport">
  <header class="cst-ledger-airport-head">
    <span class="cst-ledger-logo" aria-hidden="true">{{a.name.slice(0,1)}}</span>
    <div class="cst-ledger-title"><h2>{{a.name}}</h2><small>{{airportAccounts([a]).length}} 个订阅 · {{a.node_count}} 条配置 · {{a.maintenance?.refresh_hours?`每 ${a.maintenance.refresh_hours} 小时独立更新`:'未设独立更新计划'}}<template v-if="safeLink(a.website_url)"> · <a :href="safeLink(a.website_url)" target="_blank" rel="noopener noreferrer">官网 ↗</a></template><template v-if="safeLink(a.backup_url)"> · <a :href="safeLink(a.backup_url)" target="_blank" rel="noopener noreferrer">备用入口 ↗</a></template></small></div>
    <div class="button-group"><button class="button" :disabled="!!busy||refreshLoading||refreshJob?.state==='running'||!airportAccounts([a]).length" @click="refresh(a)"><Icon name="refresh"/>{{busy===a.id||refreshJob?.state==='running'?'更新中…':'全部更新'}}</button><button class="button ghost" :disabled="!!busy" @click="openMaintenance(a)">更新与维护</button><button class="button ghost icon-button" :disabled="!!busy" aria-label="编辑机场" @click="edit(a)"><Icon name="edit"/></button><button class="button ghost icon-button" :disabled="!!busy" aria-label="删除机场" @click="openRemove(a)"><Icon name="trash"/></button></div>
  </header>
  <p v-if="a.note" class="airport-note">{{a.note}}</p>
  <section v-if="a.maintenance?.links?.length||a.maintenance?.refresh_hours||a.maintenance?.last_result||discoveredLinks(a).length" class="airport-daily" aria-label="日常维护">
   <header><h3>日常维护</h3><div class="maintenance-schedule"><span v-if="a.maintenance?.refresh_hours">下次更新 {{maintenanceTime(a.maintenance.next_refresh)}}</span><small v-if="a.maintenance?.last_result">{{a.maintenance.last_result}}<template v-if="a.maintenance.last_attempt&&!a.maintenance.last_attempt.startsWith('0001')"> · {{date(a.maintenance.last_attempt)}}</template></small></div></header>
   <div v-for="(link,index) in a.maintenance?.links||[]" :key="index" class="maintenance-reminder" :class="{due:reminderDue(link)}">
    <a v-if="safeLink(link.url)" :href="safeLink(link.url)" target="_blank" rel="noopener noreferrer">{{link.label}} ↗</a><strong v-else>{{link.label}}</strong>
    <span :class="{warn:reminderDue(link),good:link.monthly_day>0&&link.done_month===maintenanceMonth}">{{!link.monthly_day?'常用入口':link.done_month===maintenanceMonth?'本月已完成':reminderDue(link)?'本月待处理':`每月 ${link.monthly_day} 日`}}</span>
    <button v-if="link.monthly_day" class="text-button" :disabled="!!busy" @click="toggleReminder(a,index)">{{link.done_month===maintenanceMonth?'撤销完成':'标记完成'}}</button>
   </div>
   <div v-for="link in discoveredLinks(a)" :key="link.url" class="maintenance-discovered"><div><small>订阅发现</small><a :href="link.url" target="_blank" rel="noopener noreferrer">{{link.label}} ↗</a></div><button class="text-button" :disabled="!!busy||(a.maintenance?.links?.length||0)>=20" @click="addDiscoveredLink(a,link)">加入常用入口</button></div>
   <p v-if="maintenanceError&&maintenanceErrorAirport===a.id&&!maintenance" class="inline-error" role="alert">{{maintenanceError}}</p>
   <p v-if="busy==='maintenance'&&maintenanceErrorAirport===a.id&&!maintenance" class="small muted" role="status">正在保存…</p>
  </section>
  <div v-if="a.subscriptions?.length" class="cst-ledger-cols" aria-hidden="true"><span>账户</span><span>套餐用量</span><span>到期</span><span>每日用量 · 近 14 天</span><span class="cst-col-actions">操作</span></div>
  <div v-for="s in a.subscriptions||[]" :key="s.id" class="cst-ledger-row">
    <div class="cst-ledger-account">
      <strong>{{s.name}}</strong>
      <small><span v-if="s.last_attempt_failed" class="muted">上次刷新 {{hm(s.last_attempt_at)}}</span><span v-else :class="s.status==='error'?'bad':''">{{s.status==='error'?'更新失败':s.has_cache?'已缓存':'尚未更新'}}</span> · {{s.node_count}} 条配置 · {{date(s.updated_at)}}</small>
      <div class="cst-ledger-account-meta"><code :title="s.url_display">{{s.url_display}}</code><SubscriptionChanges :airport-id="a.id" :subscription="s"/></div>
    </div>
    <div class="cst-ledger-quota"><small class="account-field-label">套餐用量</small>
      <template v-if="s.usage&&(used(s)!==null||noticeUsage(a,s).remaining==null&&noticeUsage(a,s).used==null)"><template v-if="s.usage.total&&used(s)!==null"><span class="quota-main-row">剩余 <strong>{{bytes(remaining(s)!)}}</strong> <em class="quota-pct-pill" :class="{warn:(remainingPercent(s)??100)<15}">{{remainingPercent(s)!.toFixed(1)}}% 可用</em></span><div class="cst-meter" role="meter" aria-label="套餐剩余比例" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="Math.round(remainingPercent(s)!)"><i :class="{warn:(remainingPercent(s)??100)<15}" :style="{width:`${remainingPercent(s)!}%`}"/></div><small>已用 {{bytes(used(s)!)}} / {{bytes(s.usage.total)}}</small></template><template v-else><span><strong>{{used(s)===null?'已用未知':bytes(used(s)!)}}</strong><template v-if="s.usage.total"> / {{bytes(s.usage.total)}}</template></span><small v-if="s.usage.total">{{remaining(s)===null?'剩余未知':'剩余 '+bytes(remaining(s)!)}}</small><small v-else>机场未返回总量</small></template></template>
      <template v-else-if="noticeUsage(a,s).remaining!=null||noticeUsage(a,s).used!=null"><span><strong>{{noticeUsage(a,s).remaining!=null?'剩余 '+bytes(noticeUsage(a,s).remaining):'已用 '+bytes(noticeUsage(a,s).used)}}</strong><template v-if="noticeUsage(a,s).total"> / {{bytes(noticeUsage(a,s).total)}}</template></span><small>公告识别 · 订阅未返回用量头</small></template>
      <small v-else class="muted">机场未返回套餐用量</small>
    </div>
    <div class="cst-ledger-expire"><small class="account-field-label">到期时间</small><template v-if="s.usage?.expire"><strong :class="expireTone(s.usage.expire)">{{expireLabel(s.usage.expire)}}</strong><small>{{new Date(s.usage.expire*1000).toLocaleDateString('zh-CN')}}</small></template><template v-else-if="noticeUsage(a,s).expireText"><strong>{{noticeUsage(a,s).expireText}}</strong><small>公告识别</small></template><template v-else-if="s.usage?.total!=null||noticeUsage(a,s).total!==undefined"><strong>未提供到期日</strong><small>有效期未知</small></template><small v-else class="muted">未提供</small></div>
    <UsageDays :airport-id="a.id" :subscription-id="s.id" :days="14" :refresh-version="w.usageRefreshVersion.value" chart @account="noticeBalances[s.id]=$event?.remaining"/>
    <div class="cst-ledger-actions">
      <button class="button" :disabled="!s.has_cache" title="在工作台把这个账户设为测量来源并查看节点" @click="browse(a,s)">选为测量来源</button>
      <button class="button ghost" :disabled="!!busy" aria-label="刷新用量" @click="refreshUsage(a,s)">刷新用量</button>
      <button class="button ghost" :disabled="!!busy" @click="openRequestSettings(a,s)">请求设置</button>
      <button class="button ghost icon-button" :disabled="!!busy||refreshLoading||refreshJob?.state==='running'" aria-label="更新节点" title="更新节点与订阅用量" @click="refresh(a,s)"><Icon name="refresh"/></button>
      <button class="button ghost icon-button" :disabled="!!busy" aria-label="编辑订阅" @click="edit(a,s)"><Icon name="edit"/></button>
      <button class="button ghost icon-button" :disabled="!!busy" aria-label="删除订阅" @click="openRemove(a,s)"><Icon name="trash"/></button>
    </div>
  </div>
  <footer class="cst-ledger-foot"><button class="text-button" :disabled="!!busy" @click="addSub(a)"><Icon name="plus"/>添加订阅</button></footer>
  <div v-if="!a.subscriptions?.length" class="compact-row"><span>{{a.url_display}}</span><button class="button" :disabled="!a.has_cache" @click="browse(a)">选为测量来源</button></div>
</article>
</div><div v-if="!visible.length" class="empty-state"><Icon name="layers"/><h3>{{search?'没有匹配的机场':'添加你的第一个机场'}}</h3><p>支持订阅地址或本地 Clash / Mihomo 配置。</p><button class="button primary" :disabled="!!busy" @click="edit()">添加机场</button></div>
</div>
</template>
<Modal v-if="form" :title="formTitle" :closable="busy!=='form'" @close="closeForm"><form @submit.prevent="save">
 <p class="source-context">{{form.kind==='subscription'?`所属机场 · ${form.airportName}`:form.airportId?'机场名称、入口与备注':'填入第一个订阅，创建机场及其默认账户。之后可添加更多账户。'}}</p>
 <p v-if="sourceLoading" role="status" class="muted">正在读取这个账户的订阅来源…</p>
 <fieldset class="form-stack account-fields" :disabled="busy==='form'||!form.sourceReady">
  <label>{{form.kind==='airport'?'机场名称':'账户名称'}}<input v-model="form.name" name="name" autofocus required maxlength="100"></label>
  <label v-if="form.needsSource">订阅地址或配置路径<input v-model="form.url" name="url" required autocomplete="off" spellcheck="false" placeholder="https://… 或本地文件路径"></label>
  <template v-if="form.kind==='airport'"><label>官网地址<input v-model="form.website_url" name="website_url" placeholder="可选"></label><label>备用入口<input v-model="form.backup_url" name="backup_url" placeholder="可选"></label></template>
  <label>{{form.kind==='airport'?'机场备注':'账户备注'}}<textarea v-model="form.note" name="note" rows="3" placeholder="可选"/></label>
 </fieldset>
 <p v-if="formError" class="inline-error" role="alert">{{formError}}</p>
 <div class="form-actions"><button v-if="!form.sourceReady&&!sourceLoading" type="button" class="button" @click="loadFormSource">重试读取</button><button type="button" class="button" :disabled="busy==='form'" @click="closeForm">取消</button><button class="button primary" type="submit" :disabled="!!busy||!form.sourceReady">{{busy==='form'?'保存中…':form.kind==='airport'&&form.airportId?'保存资料':'保存订阅'}}</button></div>
</form></Modal>
<Modal v-if="requestSettings" title="请求设置" :closable="busy!=='request-settings'" @close="requestSettings=null"><form @submit.prevent="saveRequestSettings">
 <p class="source-context">{{requestSettings.airport.name}} · {{requestSettings.sub.name}}</p>
 <fieldset class="form-stack account-fields" :disabled="busy==='request-settings'"><label>客户端标识（User-Agent）<input v-model="requestSettings.userAgent" name="user_agent" autofocus autocomplete="off" spellcheck="false" placeholder="留空使用默认值"></label><p class="muted small">部分订阅服务会根据客户端标识返回不同配置。仅影响这个账户下次获取订阅；保存不会立即更新节点或用量。</p></fieldset>
 <p v-if="requestError" class="inline-error" role="alert">{{requestError}}</p><div class="form-actions"><button type="button" class="button" :disabled="!!busy" @click="requestSettings=null">取消</button><button type="submit" class="button primary" :disabled="!!busy">{{busy==='request-settings'?'保存中…':'保存设置'}}</button></div>
</form></Modal>
<Modal v-if="remove" :title="remove.sub?'删除订阅账户':'删除机场及其账户'" :closable="busy!=='delete'" @close="closeRemove">
 <p class="removal-target">{{remove.airport.name}}<template v-if="remove.sub"> · <strong>{{remove.sub.name}}</strong></template></p>
 <p>{{remove.sub?'移除这个账户的订阅来源和节点缓存。':`移除这个机场及其 ${remove.airport.subscriptions?.length||0} 个账户的来源和节点缓存。`}}历史记录保留。</p>
 <p v-if="removeLoading" class="muted" role="status">正在读取关联监测…</p><p v-else-if="removeImpactError" class="inline-error" role="alert">{{removeImpactError}}</p>
 <div v-else class="removal-impact"><h3>{{affectedJobs.length?`${affectedJobs.length} 项监测使用这个来源`:'没有关联监测任务'}}</h3><ul v-if="affectedJobs.length"><li v-for="job in affectedJobs" :key="job.id">{{job.name}}</li></ul><p v-if="affectedJobs.length" class="small muted">任务不会一并删除，也不会立即取消正在进行的检测。后续启动或恢复需要有效来源，可在“持续监测”中管理。</p></div>
 <p v-if="removeError" class="inline-error" role="alert">{{removeError}}</p>
 <template #footer><button class="button" :disabled="busy==='delete'" @click="closeRemove">保留</button><button class="button danger-fill" :disabled="!!busy||removeLoading" @click="deleteItem">{{busy==='delete'?'删除中…':remove.sub?'删除账户':'删除机场'}}</button></template>
</Modal>
<Modal v-if="maintenance" title="更新与维护" wide :closable="busy!=='maintenance'" @close="maintenance=null"><form id="airport-maintenance-editor" @submit.prevent="saveMaintenance">
 <p class="source-context">{{maintenance.airport.name}}</p>
 <fieldset class="account-fields maintenance-fields" :disabled="busy==='maintenance'">
  <section class="maintenance-policy"><label>机场独立更新计划<UiSelect v-model.number="maintenance.settings.refresh_hours"><option :value="0">关闭</option><option v-for="h in [1,6,12,24,72,168]" :key="h" :value="h">每 {{h}} 小时</option><option v-if="maintenance.settings.refresh_hours&&![1,6,12,24,72,168].includes(maintenance.settings.refresh_hours)" :value="maintenance.settings.refresh_hours">每 {{maintenance.settings.refresh_hours}} 小时（已设置）</option></UiSelect></label><p class="small muted">程序运行时更新这个机场的全部账户，保存后按计划执行。全局自动更新另在“偏好设置”中管理。</p></section>
  <header class="maintenance-editor-heading"><div><h3>常用入口与月度待办</h3><p class="small muted">提醒日填 1–28；填 0 只保存入口。到期后在台账中提醒，可标记本月完成。</p></div><small class="muted">{{maintenance.settings.links.length}} / 20</small></header>
  <div v-for="(link,i) in maintenance.settings.links" :key="i" class="maintenance-editor-row">
   <label>入口名称<input v-model="link.label" aria-label="入口名称" required maxlength="100" placeholder="例如：月度活动"></label><label>入口地址<input v-model="link.url" aria-label="入口地址" type="url" required maxlength="2048" placeholder="https://…"></label>
   <label class="maintenance-day">每月提醒日<input v-model.number="link.monthly_day" type="number" min="0" max="28" required aria-label="每月提醒日"></label><button type="button" class="button ghost" aria-label="移除入口" @click="maintenance.settings.links.splice(i,1)">移除</button>
  </div>
  <p v-if="!maintenance.settings.links.length" class="small muted">还没有常用入口，可以添加公告、活动或续费页面。</p>
  <button type="button" class="button" :disabled="maintenance.settings.links.length>=20" @click="maintenance.settings.links.push({label:'',url:'',monthly_day:0})"><Icon name="plus"/>添加入口</button>
 </fieldset>
 <p v-if="maintenanceError" class="inline-error" role="alert">{{maintenanceError}}</p>
</form><template #footer><button type="button" class="button" :disabled="!!busy" @click="maintenance=null">取消</button><button type="submit" form="airport-maintenance-editor" class="button primary" :disabled="!!busy">{{busy==='maintenance'?'保存中…':'保存设置'}}</button></template></Modal>
<Modal v-if="refreshJob&&refreshProgressVisible" title="订阅刷新进度" wide @close="refreshProgressVisible=false"><p class="muted">{{refreshJob.completed}} / {{refreshJob.items.length}} 项已完成 · {{refreshJob.success}} 成功 · {{refreshJob.failed}} 失败 · {{refreshJob.not_executed}} 未执行</p><p v-if="refreshJob.persistence_error" class="inline-error" role="alert">{{refreshJob.persistence_error}}</p><div class="refresh-progress-list"><article v-for="item in refreshJob.items" :key="item.airport_id+'|'+item.subscription_id"><strong>{{item.subscription_name&&item.subscription_name!==item.airport_name?`${item.airport_name} · ${item.subscription_name}`:item.airport_name}}</strong><span class="badge" :class="item.state==='succeeded'?'good':item.state==='failed'?'bad':item.state==='not_executed'?'':'warn'">{{refreshState(item.state)}}</span><small v-if="item.stages?.length">{{item.stages.map(stage=>refreshState(stage.state)).join(' → ')}}</small><p v-if="refreshItemError(item)" class="inline-error">{{refreshItemError(item)}}</p><small v-if="item.duration_ms">{{item.duration_ms}} ms · {{item.node_count}} 条配置</small></article></div><template #footer><button v-if="refreshJob.failed>0" class="button" :disabled="refreshLoading||refreshJob.state==='running'" @click="retryRefresh">只重试失败项</button><button v-if="refreshJob.state==='running'" class="button danger-fill" aria-label="取消刷新任务" @click="cancelRefresh">取消刷新</button><button class="button primary" @click="refreshProgressVisible=false">关闭</button></template></Modal></section></template>
<style src="./secondary-observatory.css" scoped></style>
<style scoped>
.account-fields{border:0;padding:0;margin:0;min-width:0}.source-context{margin:0 0 20px;color:var(--cst-muted);font-size:13px;line-height:1.7}
.airport-daily{padding:20px 24px;border-bottom:1px solid var(--cst-line);background:var(--cst-panel)}
.airport-daily>header{display:flex;align-items:baseline;justify-content:space-between;gap:14px;flex-wrap:wrap;margin-bottom:12px}.airport-daily h3,.maintenance-editor-heading h3,.removal-impact h3{font-size:13px;font-weight:650;margin:0}
.maintenance-schedule{display:flex;gap:12px;flex-wrap:wrap;font-size:12px;color:var(--cst-muted)}
.maintenance-reminder{display:grid;grid-template-columns:minmax(0,1fr) auto auto;align-items:center;gap:12px;padding:11px 0;border-top:1px solid var(--cst-line);font-size:12px}.maintenance-reminder>a{font-weight:550;overflow-wrap:anywhere}.maintenance-reminder>span{color:var(--cst-muted)}.maintenance-reminder.due{border-left:2px solid var(--cst-warn);padding-left:12px}
.maintenance-discovered{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:10px 0;font-size:12px}.maintenance-discovered>div{display:flex;gap:10px;min-width:0}.maintenance-discovered small{color:var(--cst-muted);flex-shrink:0}.maintenance-discovered a{overflow-wrap:anywhere}
.maintenance-fields{display:grid;gap:18px}.maintenance-policy{display:grid;grid-template-columns:minmax(160px,.8fr) minmax(0,1.2fr);gap:28px;align-items:center;padding-bottom:20px;border-bottom:1px solid var(--cst-line)}.maintenance-policy label{display:grid;gap:8px;font-size:13px}.maintenance-policy p{line-height:1.8;margin:0}
.maintenance-editor-heading{display:flex;align-items:baseline;justify-content:space-between;gap:14px}.maintenance-editor-heading p{margin:8px 0 0;line-height:1.7}
.maintenance-editor-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1.6fr) 88px auto;align-items:end;gap:12px;padding-bottom:18px;border-bottom:1px solid var(--cst-line)}.maintenance-editor-row label{display:grid;gap:8px;min-width:0;font-size:12px}.maintenance-fields>.button{justify-self:start}
.removal-target{font-size:17px;line-height:1.7;margin:0 0 16px}.removal-impact{margin-top:20px;border-top:1px solid var(--cst-line);padding-top:16px}.removal-impact ul{padding-left:20px;font-size:13px}.removal-impact p{line-height:1.8}
@media(max-width:720px){.maintenance-policy{grid-template-columns:minmax(0,1fr);gap:14px}.maintenance-editor-row{grid-template-columns:minmax(0,1fr) 88px}.maintenance-editor-row>label:nth-child(1),.maintenance-editor-row>label:nth-child(2){grid-column:1/-1}.maintenance-editor-row>.maintenance-day{display:flex;align-items:center;gap:12px}.maintenance-day input{width:80px}.maintenance-editor-row>.button{justify-self:end}.airport-daily{padding:16px}.maintenance-reminder{gap:8px}.maintenance-discovered>div{flex-direction:column;gap:3px}}
.airports-top-bar{align-items:center;margin-bottom:20px;gap:16px}
.airports-top-bar h1{font-size:22px;letter-spacing:-.02em}
.airports-top-bar p{font-size:12px}
.airports-header-right{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-left:auto}
.airports-top-bar .subscription-view-tabs{margin-bottom:0}
.account-register-summary{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px;margin-bottom:18px;padding:0;border:0}
.summary-stat-card{display:flex;flex-direction:column;gap:4px;padding:15px 18px;border:1px solid var(--cst-line);border-radius:var(--cst-radius);background:var(--cst-panel);box-shadow:var(--cst-shadow-xs)}
.summary-stat-card small{font-size:11.5px;font-weight:600;color:var(--cst-muted)}
.summary-stat-card strong{font-size:20px;font-weight:750;color:var(--cst-text);font-variant-numeric:tabular-nums;letter-spacing:-.02em;margin:0}
.summary-stat-card span{font-size:11.5px;color:var(--cst-muted)}
.summary-stat-card.accent{background:var(--cst-aurora-surface),var(--cst-panel);border-color:color-mix(in srgb,var(--cst-accent) 28%,var(--cst-line))}
.summary-stat-card.accent strong{color:var(--cst-accent)}
.summary-stat-card.warn{background:var(--cst-warn-bg);border-color:color-mix(in srgb,var(--cst-warn) 35%,var(--cst-line))}
.summary-stat-card.warn strong{color:var(--cst-warn)}
.account-register-summary .summary-stat-card strong{margin-left:0}
.account-register-summary small{font-size:11px}
.account-register{display:grid;grid-template-columns:240px minmax(0,1fr);gap:20px;align-items:start;margin-top:4px}
.account-register-index{position:sticky;top:calc(var(--cst-nav-height,56px) + 16px);min-width:0;padding:14px;border:1px solid var(--cst-line);border-radius:var(--cst-radius);background:var(--cst-panel);box-shadow:var(--cst-shadow-xs)}
.account-register-index .page-toolbar{display:grid;gap:12px;margin:0 0 16px;padding:0}
.account-register-index .search-field{width:100%;min-width:0;box-sizing:border-box}
.account-register-index nav{display:grid;gap:4px}
.account-register-index nav>button{display:flex;align-items:center;gap:10px;width:100%;padding:10px 12px;border:1px solid transparent;background:transparent;text-align:left;border-radius:11px;color:var(--cst-muted);transition:all .14s}
.account-register-index nav>button[aria-pressed=true]{border-color:color-mix(in srgb,var(--cst-accent) 32%,var(--cst-line));background:color-mix(in srgb,var(--cst-accent) 8%,var(--cst-panel-2));color:var(--cst-text);box-shadow:var(--cst-shadow)}
.account-register-index nav>button:hover{background:var(--cst-panel)}
.account-index-initial{display:grid;place-items:center;width:30px;height:30px;border-radius:9px;background:var(--cst-aurora-surface);border:1px solid color-mix(in srgb,var(--cst-accent) 24%,transparent);color:var(--cst-accent);font-size:13px;font-weight:750;flex:none}
.account-register-index nav strong{display:block;font-size:13px;font-weight:600;overflow-wrap:anywhere}
.account-register-index nav small{display:block;font-size:11px;color:var(--cst-muted);margin-top:4px}
.account-register-index nav .icon{margin-left:auto;width:12px;flex:none}
.cst-ledger{min-width:0}
.cst-ledger-airport{padding:0;border:1px solid var(--cst-line);border-radius:var(--cst-radius);background:var(--cst-panel);box-shadow:var(--cst-shadow-xs);overflow:hidden}
.cst-ledger-airport-head{display:flex;align-items:center;gap:14px;padding:20px 24px;border-bottom:1px solid var(--cst-line);flex-wrap:wrap;background:linear-gradient(180deg,color-mix(in srgb,var(--cst-panel-2) 70%,var(--cst-panel)),var(--cst-panel))}
.cst-ledger-logo{width:44px;height:44px;display:grid;place-items:center;border-radius:12px;background:var(--cst-aurora-primary);box-shadow:var(--cst-aurora-btn-shadow);color:#ffffff;font-size:18px;font-weight:800;flex:none}
.cst-ledger-title{flex:1;min-width:200px;display:grid;gap:6px}
.cst-ledger-title h2{margin:0;font-size:22px;font-weight:600;letter-spacing:-.02em}
.cst-ledger-title small{color:var(--cst-muted);font-size:12px;line-height:1.8}
.cst-ledger-airport-head>.button-group{margin-left:auto;flex-wrap:wrap}
.cst-ledger-airport .airport-note{margin:0;padding:12px 24px;color:var(--cst-muted);font-size:12px;border-bottom:1px solid var(--cst-line)}
.cst-ledger-cols{display:none}
.cst-ledger-row{display:grid;grid-template-columns:minmax(160px,1fr) minmax(110px,.7fr) minmax(210px,1.35fr);gap:16px 20px;align-items:start;margin:14px 18px;padding:18px 20px;border:1px solid var(--cst-line);border-radius:13px;background:var(--cst-panel-2)}
.cst-ledger-account{grid-column:1/-1;display:grid;gap:6px;min-width:0}
.cst-ledger-account strong{font-size:16px;font-weight:600;color:var(--cst-text)}
.cst-ledger-account small{color:var(--cst-muted);font-size:12px}
.cst-ledger-account-meta{display:flex;align-items:center;flex-wrap:wrap;gap:6px 10px;min-width:0}
.cst-ledger-account-meta :deep(button.text-button){display:inline-flex;align-items:center;padding:2px 9px;border-radius:999px;border:1px solid var(--cst-line);background:var(--cst-panel);color:var(--cst-muted);font-size:11px;font-weight:550;justify-content:flex-start}
.cst-ledger-account-meta :deep(button.text-button:hover){border-color:var(--cst-accent);color:var(--cst-accent)}
.cst-ledger-account code{font:11px/1.5 var(--cst-mono);color:var(--cst-muted);max-width:100%;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;background:var(--cst-panel);border:1px solid var(--cst-line);border-radius:6px;padding:2px 8px}
.cst-ledger-quota,.cst-ledger-expire{display:flex;flex-direction:column;gap:8px;font-variant-numeric:tabular-nums;min-width:0}
.cst-ledger-quota strong,.cst-ledger-expire strong{font-size:15px;font-weight:600;color:var(--cst-text)}
.cst-ledger-quota small,.cst-ledger-expire small{color:var(--cst-muted);font-size:12px}
.cst-ledger-expire strong.warn{color:var(--cst-warn)}.cst-ledger-expire strong.bad{color:var(--cst-bad)}
.account-field-label{margin-bottom:2px}
.quota-main-row{display:flex;align-items:baseline;flex-wrap:wrap;gap:6px;font-size:12.5px;color:var(--cst-muted)}.quota-main-row strong{font-size:15.5px;font-weight:750;color:var(--cst-text)}.quota-pct-pill{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;background:var(--cst-accent-soft);color:var(--cst-accent);font-style:normal;font-size:10.5px;font-weight:700}.quota-pct-pill.warn{background:var(--cst-warn-bg);color:var(--cst-warn)}
.cst-meter{height:7px;border-radius:999px;background:var(--cst-aurora-track);overflow:hidden;margin:3px 0}
.cst-meter i{display:block;height:100%;border-radius:999px;background:var(--cst-aurora-bar);box-shadow:var(--cst-aurora-glow);transition:width .4s cubic-bezier(.22,1,.36,1)}.cst-meter i.warn{background:var(--cst-aurora-warn);box-shadow:none}

.cst-ledger-actions{display:flex;align-items:center;gap:8px;flex-wrap:wrap;grid-column:1/-1;padding-top:2px}
.cst-ledger-foot{padding:16px 24px}
@media(min-width:1450px){.cst-ledger-row{grid-template-columns:minmax(230px,1.1fr) minmax(150px,.8fr) minmax(110px,.65fr) minmax(240px,1.4fr)}.cst-ledger-account{grid-column:auto}.cst-ledger-actions{justify-content:flex-end}}
@media(max-width:1100px){.account-register-summary{grid-template-columns:minmax(0,1fr)}.account-register{grid-template-columns:200px minmax(0,1fr);gap:16px}.cst-ledger-row{grid-template-columns:1fr 1fr}.cst-ledger-row>:deep(.cst-usage-days){grid-column:1/-1}.cst-ledger-row>.cst-usage-days{grid-column:1/-1}}
@media(max-width:760px){.account-register{grid-template-columns:minmax(0,1fr);gap:16px}.account-register-index{position:static}.account-register-index nav{display:flex;overflow:auto;gap:8px}.account-register-index nav>button{min-width:180px}.account-register-index .page-toolbar{grid-template-columns:minmax(0,1fr) auto;align-items:center}.cst-ledger-airport-head,.cst-ledger-row{padding:18px}.airports-header-right{margin-left:0}.cst-ledger-account code{white-space:normal;overflow-wrap:anywhere}}
</style>
