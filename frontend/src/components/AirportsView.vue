<script setup lang="ts">
import UiSelect from './UiSelect.vue'
import {computed,onBeforeUnmount,onMounted,ref} from 'vue'
import {api,desktop,request,type SubscriptionRefreshJob} from '../api'
import {useWorkspace} from '../workspace'
import {bytes,date,type Airport,type Subscription} from '../domain'
import Modal from './Modal.vue'
import Icon from './Icon.vue'
import SubscriptionUsageView from './SubscriptionUsageView.vue'
import {airportAccounts,noticesForAccount,parseNoticeUsage} from '../airportViewing'
import UsageDays from './UsageDays.vue'
const section=ref('manage')
const w=useWorkspace(),busy=ref(''),search=ref(''),form=ref<{kind:'airport'|'subscription';airportId?:string;subId?:string;name:string;url:string;website_url:string;backup_url:string;note:string}|null>(null),formError=ref(''),remove=ref<{airport:Airport;sub?:Subscription}|null>(null),maintenance=ref<{airport:Airport;settings:NonNullable<Airport['maintenance']>}|null>(null),refreshJob=ref<SubscriptionRefreshJob|null>(null),refreshProgressVisible=ref(false),refreshLoading=ref(false)
const visible=computed(()=>w.airports.value.filter(a=>(a.name+' '+(a.note||'')).includes(search.value)))
const safeLink=(v?:string)=>/^https?:\/\//i.test(v||'')?v:undefined
let refreshTimer:ReturnType<typeof setTimeout>|undefined,disposed=false
async function edit(airport?:Airport,sub?:Subscription){formError.value='';if(!airport){form.value={kind:'airport',name:'',url:'',website_url:'',backup_url:'',note:''};return}busy.value=sub?.id||airport.id;try{const url=await request<{url:string}|string>(sub?`/api/airports/${airport.id}/subscriptions/${sub.id}/url`:`/api/airports/${airport.id}/url`,'GET',undefined,sub?'GetSubscriptionURL':'GetAirportURL',sub?[airport.id,sub.id]:[airport.id]);form.value={kind:sub?'subscription':'airport',airportId:airport.id,subId:sub?.id,name:sub?.name||airport.name,url:typeof url==='string'?url:url.url,website_url:airport.website_url||'',backup_url:airport.backup_url||'',note:sub?.note||airport.note||''}}catch(e){w.fail(e)}finally{busy.value=''}}
function addSub(airport:Airport){formError.value='';form.value={kind:'subscription',airportId:airport.id,name:'',url:'',website_url:'',backup_url:'',note:''}}
async function save(){const f=form.value;if(!f)return;formError.value='';if(!f.name.trim()||!f.url.trim()){formError.value='请填写名称和订阅地址或配置文件路径';return}busy.value='form';try{if(f.kind==='airport'){const body={name:f.name,url:f.url,website_url:f.website_url,backup_url:f.backup_url,note:f.note};await request(f.airportId?`/api/airports/${f.airportId}`:'/api/airports',f.airportId?'PUT':'POST',body,f.airportId?'UpdateFrontendAirport':'CreateFrontendAirport',f.airportId?[f.airportId,body]:[body])}else await request(`/api/airports/${f.airportId}/subscriptions${f.subId?'/'+f.subId:''}`,f.subId?'PUT':'POST',{name:f.name,url:f.url,note:f.note},f.subId?'UpdateSubscription':'AddSubscription',f.subId?[f.airportId,f.subId,f.name,f.url,f.note]:[f.airportId,f.name,f.url,f.note]);form.value=null;await w.refreshNodes();w.notify('订阅已保存')}catch(e){formError.value=e instanceof Error?e.message:String(e)}finally{busy.value=''}}
const refreshState=(state:string)=>({queued:'排队中',fetching:'抓取中',parsing:'解析中',saving:'保存中',succeeded:'成功',failed:'失败',cancelled:'已取消',not_executed:'未执行',interrupted:'中断'} as Record<string,string>)[state]||state
function mergeRefresh(job:SubscriptionRefreshJob,openProgress=false){refreshJob.value=job;if(openProgress)refreshProgressVisible.value=true;if(job.state==='running')scheduleRefreshPoll(job.id);else clearTimeout(refreshTimer)}
function scheduleRefreshPoll(id:string){clearTimeout(refreshTimer);refreshTimer=setTimeout(()=>void pollRefresh(id),700)}
async function pollRefresh(id:string){if(disposed)return;try{const job=await api.refreshJob(id);if(disposed)return;mergeRefresh(job);if(job.state!=='running'){await w.refreshNodes();w.notify(job.failed?`刷新结束：${job.success} 成功，${job.failed} 失败`:'订阅已刷新，选择与节点范围已同步')}}catch(e){if(!disposed)w.fail(e)}}
async function loadRefreshJobs(){try{const jobs=await api.refreshJobs();if(disposed)return;const current=jobs.find(job=>job.state==='running')||jobs.find(job=>job.failed>0);if(current)mergeRefresh(current,true)}catch(e){if(!disposed)w.fail(e)}}
function refreshRequestID(){return globalThis.crypto?.randomUUID?.()||`refresh-${Date.now()}-${Math.random().toString(16).slice(2)}`}
async function refresh(a:Airport,s?:Subscription){if(refreshLoading.value||refreshJob.value?.state==='running')return;busy.value=s?.id||a.id;refreshLoading.value=true;try{const accounts=airportAccounts([a]),selected=s?accounts.find(entry=>entry.subscription.id===s.id):undefined,requestBody=s?{request_id:refreshRequestID(),selections:[{airport_id:a.id,subscription_id:s.id}]}:{request_id:refreshRequestID(),selections:accounts.map(entry=>({airport_id:a.id,subscription_id:entry.subscription.id}))};if(s&&!selected)throw new Error('订阅已变化，请重新载入机场列表');mergeRefresh(await api.startRefresh(requestBody),true);if(refreshJob.value?.state!=='running'){await w.refreshNodes();w.notify('订阅刷新任务已完成')}}catch(e){w.fail(e)}finally{busy.value='';refreshLoading.value=false}}
async function cancelRefresh(){const job=refreshJob.value;if(!job||job.state!=='running')return;clearTimeout(refreshTimer);try{mergeRefresh(await api.cancelRefresh(job.id))}catch(e){w.fail(e)}}
async function retryRefresh(){const job=refreshJob.value;if(!job||job.failed===0||refreshLoading.value)return;refreshLoading.value=true;try{mergeRefresh(await api.startRefresh({request_id:refreshRequestID(),retry_of:job.id}),true)}catch(e){w.fail(e)}finally{refreshLoading.value=false}}
async function deleteItem(){const item=remove.value;if(!item)return;busy.value='delete';try{await request(item.sub?`/api/airports/${item.airport.id}/subscriptions/${item.sub.id}`:`/api/airports/${item.airport.id}`,'DELETE',undefined,item.sub?'DeleteSubscription':'DeleteAirport',item.sub?[item.airport.id,item.sub.id]:[item.airport.id]);remove.value=null;await w.refreshNodes();await w.refreshJobs();w.notify('订阅已删除')}catch(e){w.fail(e)}finally{busy.value=''}}
function openMaintenance(a:Airport){maintenance.value={airport:a,settings:JSON.parse(JSON.stringify(a.maintenance||{refresh_hours:0,links:[]}))}}
async function saveMaintenance(){const m=maintenance.value;if(!m)return;busy.value='maintenance';try{await request(`/api/airports/${m.airport.id}/maintenance`,'PUT',m.settings,'SaveAirportMaintenance',[m.airport.id,m.settings]);await w.refreshNodes();maintenance.value=null;w.notify('维护设置已保存')}catch(e){w.fail(e)}finally{busy.value=''}}
function remaining(s:Subscription){return s.usage?.total?Math.max(0,s.usage.total-s.usage.upload-s.usage.download):null}
// 订阅头没有用量时，退回到节点列表里的公告文字（标明来源，不当作订阅返回值）。
function noticeUsage(a:Airport,s:Subscription){return parseNoticeUsage(noticesForAccount(w.notices.value,airportAccounts([a]).find(x=>x.subscription.id===s.id)))}
function expireDays(t:number){return Math.ceil((t*1000-Date.now())/86400000)}
function expireLabel(t:number){const d=expireDays(t);return d<0?'已过期':d===0?'今天到期':`还剩 ${d} 天`}
function expireTone(t:number){const d=expireDays(t);return d<0?'bad':d<=7?'warn':''}
function browse(a:Airport,s?:Subscription){w.setAirports([a.id]);if(s)w.subscription.value=s.id;w.page.value='home'}
onMounted(()=>void loadRefreshJobs())
onBeforeUnmount(()=>{disposed=true;clearTimeout(refreshTimer)})
</script>
<template><section class="page-view airports-view"><div class="page-header"><div><h1>机场与账户</h1><p>每个订阅账户一行：套餐用量、到期、每日用量和更新状态。这里是机场账户计数，不是本工具的探测流量。</p></div><button v-if="section==='manage'&&refreshJob&&!refreshProgressVisible" class="button ghost" @click="refreshProgressVisible=true"><Icon name="refresh"/>查看刷新进度</button><button v-if="section==='manage'" class="button primary" @click="edit()"><Icon name="plus"/>添加机场</button></div><div class="subscription-view-tabs" role="group" aria-label="机场订阅视图"><button class="button" :class="{active:section==='manage'}" :aria-pressed="section==='manage'" @click="section='manage'">账户台账</button><button class="button" :class="{active:section==='usage'}" :aria-pressed="section==='usage'" @click="section='usage'">用量明细</button></div><SubscriptionUsageView v-if="section==='usage'"/><template v-else><div class="page-toolbar"><label class="search-field"><Icon name="search"/><input v-model="search" placeholder="搜索机场" aria-label="搜索机场"></label><span class="muted small">{{w.airports.value.length}} 个机场 · {{w.airports.value.reduce((n,x)=>n+(x.subscriptions?.length||1),0)}} 个账户</span></div>
<div class="cst-ledger">
<article v-for="a in visible" :key="a.id" class="airport-card cst-ledger-airport">
  <header class="cst-ledger-airport-head">
    <span class="cst-ledger-logo" aria-hidden="true">{{a.name.slice(0,1)}}</span>
    <div class="cst-ledger-title"><h2>{{a.name}}</h2><small>{{a.subscriptions?.length||1}} 个订阅 · {{a.node_count}} 条配置 · {{a.maintenance?.refresh_hours?`每 ${a.maintenance.refresh_hours} 小时自动更新`:'手动更新'}}<template v-if="safeLink(a.website_url)"> · <a :href="safeLink(a.website_url)" target="_blank" rel="noopener noreferrer">官网 ↗</a></template><template v-if="safeLink(a.backup_url)"> · <a :href="safeLink(a.backup_url)" target="_blank" rel="noopener noreferrer">备用入口 ↗</a></template><template v-for="link in (a.maintenance?.links||[]).filter(l=>safeLink(l.url))" :key="link.url"> · <a :href="safeLink(link.url)" target="_blank" rel="noopener noreferrer">{{link.label||'入口'}} ↗</a></template></small></div>
    <div class="button-group"><button class="button" :disabled="!!busy||refreshLoading||refreshJob?.state==='running'" @click="refresh(a)"><Icon name="refresh"/>{{busy===a.id||refreshJob?.state==='running'?'更新中…':'全部更新'}}</button><button class="button ghost" @click="openMaintenance(a)">更新与维护</button><button class="button ghost icon-button" :disabled="!!busy" aria-label="编辑机场" @click="edit(a)"><Icon name="edit"/></button><button class="button ghost icon-button" aria-label="删除机场" @click="remove={airport:a}"><Icon name="trash"/></button></div>
  </header>
  <p v-if="a.note" class="airport-note">{{a.note}}</p>
  <div v-if="a.subscriptions?.length" class="cst-ledger-cols" aria-hidden="true"><span>账户</span><span>套餐用量</span><span>到期</span><span>每日用量 · 近 14 天</span><span/></div>
  <div v-for="s in a.subscriptions||[]" :key="s.id" class="cst-ledger-row">
    <div class="cst-ledger-account">
      <strong>{{s.name}}</strong>
      <small><span :class="s.status==='error'?'bad':''">{{s.status==='error'?'更新失败':s.has_cache?'已缓存':'尚未更新'}}</span> · {{s.node_count}} 条配置 · {{date(s.updated_at)}}</small>
      <code :title="s.url_display">{{s.url_display}}</code>
    </div>
    <div class="cst-ledger-quota">
      <template v-if="s.usage"><span><strong>{{bytes(s.usage.upload+s.usage.download)}}</strong><template v-if="s.usage.total"> / {{bytes(s.usage.total)}}</template></span><div v-if="s.usage.total" class="cst-meter" role="meter" aria-label="套餐已用比例" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="Math.round(Math.min(100,(s.usage.upload+s.usage.download)/s.usage.total*100))"><i :class="{warn:(s.usage.upload+s.usage.download)/s.usage.total>.85}" :style="{width:`${Math.min(100,(s.usage.upload+s.usage.download)/s.usage.total*100)}%`}"/></div><small v-if="s.usage.total">剩余 {{bytes(remaining(s)||0)}}</small><small v-else>机场未返回总量</small></template>
      <template v-else-if="noticeUsage(a,s).remaining!=null||noticeUsage(a,s).used!=null"><span><strong>{{noticeUsage(a,s).remaining!=null?'剩余 '+bytes(noticeUsage(a,s).remaining):'已用 '+bytes(noticeUsage(a,s).used)}}</strong><template v-if="noticeUsage(a,s).total"> / {{bytes(noticeUsage(a,s).total)}}</template></span><small>公告识别 · 订阅未返回用量头</small></template>
      <small v-else class="muted">机场未返回套餐用量</small>
    </div>
    <div class="cst-ledger-expire"><template v-if="s.usage?.expire"><strong :class="expireTone(s.usage.expire)">{{expireLabel(s.usage.expire)}}</strong><small>{{new Date(s.usage.expire*1000).toLocaleDateString('zh-CN')}}</small></template><template v-else-if="noticeUsage(a,s).expireText"><strong>{{noticeUsage(a,s).expireText}}</strong><small>公告识别</small></template><small v-else class="muted">未提供</small></div>
    <UsageDays :airport-id="a.id" :subscription-id="s.id" :days="14" chart/>
    <div class="cst-ledger-actions">
      <button class="button" :disabled="!s.has_cache" title="在工作台把这个账户设为测量来源并查看节点" @click="browse(a,s)">选为测量来源</button>
      <button class="button ghost icon-button" :disabled="!!busy||refreshLoading||refreshJob?.state==='running'" aria-label="刷新订阅" title="更新订阅：重新拉取节点，并记录机场返回的用量" @click="refresh(a,s)"><Icon name="refresh"/></button>
      <button class="button ghost icon-button" :disabled="!!busy" aria-label="编辑订阅" @click="edit(a,s)"><Icon name="edit"/></button>
      <button class="button ghost icon-button" aria-label="删除订阅" @click="remove={airport:a,sub:s}"><Icon name="trash"/></button>
    </div>
  </div>
  <footer class="cst-ledger-foot"><button class="text-button" @click="addSub(a)"><Icon name="plus"/>添加订阅</button></footer>
  <div v-if="!a.subscriptions?.length" class="compact-row"><span>{{a.url_display}}</span><button class="button" :disabled="!a.has_cache" @click="browse(a)">选为测量来源</button></div>
</article>
</div><div v-if="!visible.length" class="empty-state"><Icon name="layers"/><h3>{{search?'没有匹配的机场':'添加你的第一个机场'}}</h3><p>支持订阅地址或本地 Clash / Mihomo 配置。</p><button class="button primary" @click="edit()">添加机场</button></div>
</template>
<Modal v-if="form" :title="`${form.airportId&&form.kind==='airport'||form.subId?'编辑':'添加'}${form.kind==='airport'?'机场':'订阅'}`" @close="()=>{if(!busy)form=null}"><form @submit.prevent="save"><div class="form-stack"><label>名称<input v-model="form.name" autofocus required maxlength="100"></label><label>订阅地址或配置路径<input v-model="form.url" required autocomplete="off" spellcheck="false" placeholder="https://… 或本地文件路径"></label><template v-if="form.kind==='airport'"><label>官网地址<input v-model="form.website_url" placeholder="可选"></label><label>备用入口<input v-model="form.backup_url" placeholder="可选"></label></template><label>备注<textarea v-model="form.note" rows="3"/></label></div><p v-if="formError" class="inline-error" role="alert">{{formError}}</p><div class="form-actions"><button type="button" class="button" :disabled="!!busy" @click="form=null">取消</button><button class="button primary" type="submit" :disabled="!!busy">{{busy==='form'?'保存中…':'保存订阅'}}</button></div></form></Modal>
<Modal v-if="remove" title="删除订阅来源" @close="remove=null"><p>删除「{{remove.sub?.name||remove.airport.name}}」{{remove.sub?'这个订阅':'及其下的订阅'}}。已有检测历史保留，关联监测可能因来源缺失而停止。</p><template #footer><button class="button" @click="remove=null">返回</button><button class="button danger-fill" :disabled="!!busy" @click="deleteItem">删除</button></template></Modal>
<Modal v-if="maintenance" title="更新与维护" wide @close="maintenance=null"><div class="form-grid"><label>自动更新订阅<UiSelect v-model.number="maintenance.settings.refresh_hours"><option :value="0">关闭</option><option v-for="h in [1,6,12,24,72,168]" :key="h" :value="h">每 {{h}} 小时</option></UiSelect></label></div><h3>快捷入口与每月提醒</h3><div v-for="(link,i) in maintenance.settings.links" :key="i" class="maintenance-link"><input v-model="link.label" aria-label="入口名称" placeholder="入口名称"><input v-model="link.url" aria-label="入口地址" placeholder="https://…"><label>每月<input v-model.number="link.monthly_day" type="number" min="0" max="28" aria-label="每月提醒日"></label><button class="button ghost icon-button" aria-label="移除入口" @click="maintenance.settings.links.splice(i,1)"><Icon name="close"/></button></div><p class="small muted">提醒日填 1–28；填 0 时只保留快捷入口。</p><button class="button" @click="maintenance.settings.links.push({label:'',url:'',monthly_day:0})"><Icon name="plus"/>添加入口</button><template #footer><button class="button" @click="maintenance=null">返回</button><button class="button primary" :disabled="!!busy" @click="saveMaintenance">保存</button></template></Modal>
<Modal v-if="refreshJob&&refreshProgressVisible" title="订阅刷新进度" wide @close="refreshProgressVisible=false"><p class="muted">{{refreshJob.completed}} / {{refreshJob.items.length}} 项已完成 · {{refreshJob.success}} 成功 · {{refreshJob.failed}} 失败 · {{refreshJob.not_executed}} 未执行</p><p v-if="refreshJob.persistence_error" class="inline-error" role="alert">{{refreshJob.persistence_error}}</p><div class="refresh-progress-list"><article v-for="item in refreshJob.items" :key="item.airport_id+'|'+item.subscription_id"><strong>{{item.airport_name}} · {{item.subscription_name}}</strong><span class="badge" :class="item.state==='succeeded'?'good':item.state==='failed'?'bad':item.state==='not_executed'?'':'warn'">{{refreshState(item.state)}}</span><small v-if="item.stages?.length">{{item.stages.map(stage=>refreshState(stage.state)).join(' → ')}}</small><p v-if="item.error_message" class="inline-error">{{item.error_message}}</p><small v-if="item.duration_ms">{{item.duration_ms}} ms · {{item.node_count}} 条配置</small></article></div><template #footer><button v-if="refreshJob.failed>0" class="button" :disabled="refreshLoading||refreshJob.state==='running'" @click="retryRefresh">只重试失败项</button><button v-if="refreshJob.state==='running'" class="button danger-fill" aria-label="取消刷新任务" @click="cancelRefresh">取消刷新</button><button class="button primary" @click="refreshProgressVisible=false">关闭</button></template></Modal></section></template>
<style src="./secondary-observatory.css" scoped></style>
<style scoped>
.cst-ledger{display:flex;flex-direction:column;gap:14px}
.cst-ledger-airport{padding:0;overflow:hidden}
.cst-ledger-airport-head{display:flex;align-items:center;gap:12px;padding:14px 16px;border-bottom:1px solid var(--cst-line);flex-wrap:wrap}
.cst-ledger-logo{width:34px;height:34px;border-radius:9px;display:grid;place-items:center;background:var(--cst-accent-soft);color:var(--cst-accent);font-weight:700;flex:none}
.cst-ledger-title{flex:1;min-width:200px}
.cst-ledger-title h2{margin:0;font-size:15px}
.cst-ledger-title small{color:var(--cst-muted)}
.cst-ledger-airport .airport-note{margin:0;padding:8px 16px;color:var(--cst-muted);font-size:12px;border-bottom:1px solid var(--cst-line)}
.cst-ledger-cols,.cst-ledger-row{display:grid;grid-template-columns:minmax(180px,1.3fr) minmax(150px,1fr) 96px minmax(200px,1.3fr) auto;gap:16px;align-items:center;padding:0 16px}
.cst-ledger-cols{padding-top:8px;padding-bottom:6px;font-size:11px;font-weight:600;color:var(--cst-muted);background:var(--cst-panel-2);border-bottom:1px solid var(--cst-line)}
.cst-ledger-row{padding-top:12px;padding-bottom:12px;border-bottom:1px solid var(--cst-line)}
.cst-ledger-account{display:flex;flex-direction:column;gap:2px;min-width:0}
.cst-ledger-account small{color:var(--cst-muted)}
.cst-ledger-account small .bad{color:var(--cst-bad);font-weight:500}
.cst-ledger-account code{font:11px var(--cst-mono);color:var(--cst-subtle);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.cst-ledger-quota,.cst-ledger-expire{display:flex;flex-direction:column;gap:4px;font-variant-numeric:tabular-nums;min-width:0}
.cst-ledger-quota small,.cst-ledger-expire small{color:var(--cst-muted);font-size:11px}
.cst-ledger-expire strong.warn{color:var(--cst-warn)}
.cst-ledger-expire strong.bad{color:var(--cst-bad)}
.cst-meter{height:5px;border-radius:3px;background:var(--cst-line);overflow:hidden}
.cst-meter i{display:block;height:100%;background:var(--cst-accent)}
.cst-meter i.warn{background:var(--cst-warn)}
.cst-ledger-actions{display:flex;gap:4px;justify-content:flex-end}
.cst-ledger-foot{padding:8px 16px}
@media(max-width:1100px){.cst-ledger-cols{display:none}.cst-ledger-row{grid-template-columns:1fr 1fr;row-gap:10px}.cst-ledger-account{grid-column:1/-1}.cst-ledger-actions{grid-column:1/-1;justify-content:flex-start}}
@media(max-width:560px){.cst-ledger-row{grid-template-columns:1fr}}
</style>
