<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref,watch} from 'vue'
import {api,type SubscriptionRefreshJob,type SubscriptionRefreshRequest} from '../api'
import {bytes,date,type Airport,type NodeOption} from '../domain'
import {airportAccounts,noticesForAccount,safeAirportLink,parseNoticeUsage,type AirportAccount} from '../airportViewing'
import {useWorkspace} from '../workspace'
import UiSelect from './UiSelect.vue'
import Modal from './Modal.vue'
import UsageDays from './UsageDays.vue'

const props=defineProps<{airports:Airport[];notices:NodeOption[]}>()
type SubscriptionDisclosureSummary={refreshState:'loading'|'ready'|'error';noticeCount:number;expiredAccountCount:number;unknownExpiryAccountCount:number;failedItemCount:number;interruptedItemCount:number;runningItemCount:number;latestRefreshIssueAccount?:string;latestRefreshIssueState?:'failed'|'interrupted'}
const emit=defineEmits<{summary:[value:SubscriptionDisclosureSummary];viewed:[airportId:string]}>()
const workspace=useWorkspace()
const viewedAirportId=ref('')
const viewedAirport=computed(()=>props.airports.find(airport=>airport.id===viewedAirportId.value)||props.airports[0])
const viewedAirportName=computed(()=>viewedAirport.value?.name||'')
const accounts=computed(()=>viewedAirport.value?airportAccounts([viewedAirport.value]):[])
const selectedKey=ref('')
const rememberedAccountByAirport=ref<Record<string,string>>({})
const jobs=ref<SubscriptionRefreshJob[]>([])
const activeJob=ref<SubscriptionRefreshJob|null>(null)
const loading=ref(false)
const jobsReadState=ref<'loading'|'ready'|'error'>('loading')
const noticeOpen=ref(false)
const scopeOpen=ref(false)
const scopeTouched=ref(false)
const selectedScopeKeys=ref<string[]>([])
const error=ref('')

const selected=computed(()=>accounts.value.find(account=>account.key===selectedKey.value))
const accountNotices=computed(()=>noticesForAccount(props.notices,selected.value))
const usage=computed(()=>selected.value?.subscription.usage)
const parsedNoticeUsage=computed(()=>parseNoticeUsage(accountNotices.value))
const used=computed(()=>usage.value?usage.value.upload+usage.value.download:parsedNoticeUsage.value.used??null)
const total=computed(()=>{
  const usageTotal=usage.value?.total
  if(typeof usageTotal==='number'&&Number.isFinite(usageTotal))return usageTotal
  if(typeof parsedNoticeUsage.value.total==='number'&&Number.isFinite(parsedNoticeUsage.value.total))return parsedNoticeUsage.value.total
  return undefined
})
const remaining=computed(()=>{
  if(total.value!==undefined&&used.value!==null&&(usage.value?.total!==undefined||parsedNoticeUsage.value.total!==undefined))return Math.max(0,total.value-used.value)
  if(parsedNoticeUsage.value.remaining!==undefined)return parsedNoticeUsage.value.remaining
  return null
})
const usedAmount=computed<number|null>(()=>{
  if(used.value!==null&&Number.isFinite(used.value))return Math.max(0,used.value)
  if(total.value!==undefined&&Number.isFinite(total.value)&&remaining.value!==null&&Number.isFinite(remaining.value))return Math.max(0,total.value-remaining.value)
  return null
})
const usageRatio=computed<number|null>(()=>{
  if(total.value===undefined||!Number.isFinite(total.value)||total.value<=0||usedAmount.value===null||!Number.isFinite(usedAmount.value))return null
  return Math.min(1,Math.max(0,usedAmount.value/total.value))
})
const remainingRatio=computed<number|null>(()=>{
  if(total.value===undefined||!Number.isFinite(total.value)||total.value<=0||remaining.value===null||!Number.isFinite(remaining.value))return null
  return Math.min(1,Math.max(0,remaining.value/total.value))
})
const expireLabel=computed(()=>{
  const expire=usage.value?.expire
  if(typeof expire==='number'&&Number.isFinite(expire)&&expire>0){
    const at=new Date(expire*1000)
    if(Number.isFinite(at.getTime()))return at.getTime()<=Date.now()?`已过期 ${at.toLocaleDateString('zh-CN')}`:`到期 ${at.toLocaleDateString('zh-CN')}`
  }
  if(parsedNoticeUsage.value.expireText)return parsedNoticeUsage.value.isPerpetual?'长期有效':`到期 ${parsedNoticeUsage.value.expireText}`
  return '到期未知'
})
const sourceLabel=computed(()=>{
  if(usage.value)return '订阅返回'
  if(parsedNoticeUsage.value.remaining!==undefined||parsedNoticeUsage.value.used!==undefined||parsedNoticeUsage.value.total!==undefined||parsedNoticeUsage.value.expireText)return '公告识别'
  const account=selected.value
  return account&&account.subscription.id===account.airport.id&&!account.airport.subscriptions?.length?'机场默认订阅':'额度来源未知'
})
const updatedAtLabel=computed(()=>{
  const updated=selected.value?.subscription.updated_at||selected.value?.airport.updated_at
  return updated?date(updated):'更新时间未知'
})
const officialLink=computed(()=>safeAirportLink(selected.value?.airport.website_url))
const extraLinks=computed(()=>{
  const airport=selected.value?.airport
  if(!airport)return []
  const values=[{label:'备用入口',url:airport.backup_url},...(airport.maintenance?.links||[]).map(link=>({label:link.label,url:link.url}))]
  return values.map(link=>({...link,url:safeAirportLink(link.url)})).filter((link,index,all)=>!!link.label&&!!link.url&&link.url!==officialLink.value&&all.findIndex(other=>safeAirportLink(other.url)===link.url)===index)
})
const workspaceAccounts=computed(()=>airportAccounts(workspace.airports.value))
const selectedScopeAccounts=computed(()=>workspaceAccounts.value.filter(account=>selectedScopeKeys.value.includes(account.key)))
const latestRefreshByAccount=computed(()=>{
  const latestFor=(accountList:AirportAccount[])=>accountList.map(account=>{
    let latest:{job:SubscriptionRefreshJob;item:SubscriptionRefreshJob['items'][number]}|undefined
    for(const job of jobs.value){
      const item=job.items.find(candidate=>candidate.airport_id===account.airport.id&&candidate.subscription_id===account.subscription.id)
      if(item&&(!latest||Date.parse(job.created_at)>Date.parse(latest.job.created_at)))latest={job,item}
    }
    return {account,...latest}
  })
  return latestFor(airportAccounts(props.airports))
})
const workspaceRefreshIssues=computed(()=>workspaceAccounts.value.flatMap(account=>{
  let latest:{job:SubscriptionRefreshJob;item:SubscriptionRefreshJob['items'][number]}|undefined
  for(const job of jobs.value){
    const item=job.items.find(candidate=>candidate.airport_id===account.airport.id&&candidate.subscription_id===account.subscription.id)
    if(item&&(!latest||Date.parse(job.created_at)>Date.parse(latest.job.created_at)))latest={job,item}
  }
  return latest&&(latest.item.state==='failed'||latest.item.state==='interrupted')?[{account,...latest}]:[]
}).sort((a,b)=>Date.parse(b.job.created_at)-Date.parse(a.job.created_at)))
const workspaceRefreshIssueJobs=computed(()=>[...new Map(workspaceRefreshIssues.value.map(issue=>[issue.job.id,issue.job])).values()])
const disclosureSummary=computed<SubscriptionDisclosureSummary>(()=>{
  const expiry=airportAccounts(props.airports).map(account=>account.subscription.usage?.expire).filter((value):value is number=>typeof value==='number'&&Number.isFinite(value)&&value>0)
  const latest=latestRefreshByAccount.value
  const failed=latest.filter(entry=>entry.item?.state==='failed')
  const interrupted=latest.filter(entry=>entry.item?.state==='interrupted')
  const issues=latest.filter(entry=>entry.item?.state==='failed'||entry.item?.state==='interrupted').sort((a,b)=>Date.parse(b.job!.created_at)-Date.parse(a.job!.created_at))
  const runningStates=['queued','running','fetching','parsing','saving']
  const running=latest.filter(entry=>!!entry.item&&runningStates.includes(entry.item.state))
  const knownExpiry=(account:AirportAccount)=>{
    const expire=account.subscription.usage?.expire
    return typeof expire==='number'&&Number.isFinite(expire)&&expire>0||!!parseNoticeUsage(noticesForAccount(props.notices,account)).expireText
  }
  return {
    refreshState:error.value||jobsReadState.value==='error'?'error':jobsReadState.value,
    noticeCount:props.notices.length,
    expiredAccountCount:expiry.filter(value=>value*1000<=Date.now()).length,
    unknownExpiryAccountCount:airportAccounts(props.airports).filter(account=>!knownExpiry(account)).length,
    failedItemCount:failed.length,
    interruptedItemCount:interrupted.length,
    runningItemCount:running.length,
    latestRefreshIssueAccount:issues[0]?`${issues[0].account.airport.name} · ${issues[0].account.subscription.name}`:undefined,
    latestRefreshIssueState:issues[0]?.item?.state==='interrupted'?'interrupted':issues[0]?'failed':undefined,
  }
})
const jobRunning=(job:SubscriptionRefreshJob|null)=>job?.state==='running'
const visibleJob=computed(()=>{
  const account=selected.value
  if(!account)return null
  const matches=(job:SubscriptionRefreshJob)=>job.items.find(item=>item.airport_id===account.airport.id&&item.subscription_id===account.subscription.id)
  if(activeJob.value&&matches(activeJob.value))return activeJob.value
  return [...jobs.value].sort((a,b)=>Date.parse(b.created_at)-Date.parse(a.created_at)).find(matches)||null
})
const visibleItem=computed(()=>{
  const account=selected.value,job=visibleJob.value
  return account&&job?job.items.find(item=>item.airport_id===account.airport.id&&item.subscription_id===account.subscription.id)||null:null
})
const itemStatusLabel=(state?:string)=>state?({queued:'等待刷新',running:'刷新中',fetching:'抓取中',parsing:'解析中',saving:'保存中',succeeded:'刷新成功',failed:'刷新失败',interrupted:'刷新中断',not_executed:'未执行',cancelled:'已取消'} as Record<string,string>)[state]||state:''
const statusLabel=computed(()=>{
  const state=visibleItem.value?.state
  const current=itemStatusLabel(state)
  if(jobRunning(activeJob.value)){
    const completed=Math.min(activeJob.value!.completed,activeJob.value!.items.length),totalItems=activeJob.value!.items.length
    const progress=`${completed}/${totalItems}`
    if(!current||['queued','running','fetching','parsing','saving'].includes(state||''))return `刷新中 ${progress}`
    return `${current} · 其他订阅刷新中 ${progress}`
  }
  return current
})
const statusTone=computed(()=>jobRunning(activeJob.value)?'warn':['failed','interrupted'].includes(visibleItem.value?.state||'')?'bad':visibleItem.value?.state==='succeeded'?'good':statusLabel.value?'warn':'')
const refreshDisabled=computed(()=>loading.value||jobRunning(activeJob.value))
const cancelJob=computed(()=>jobRunning(activeJob.value)?activeJob.value:jobRunning(visibleJob.value)?visibleJob.value:null)

// 切到指定机场（工作台按机场筛选时同步过来），记住每个机场上次看的订阅。
function showAirport(id:string){
  const target=props.airports.find(airport=>airport.id===id)
  if(!target||viewedAirport.value?.id===id&&viewedAirportId.value===id)return
  if(viewedAirport.value&&accounts.value.some(account=>account.key===selectedKey.value))rememberedAccountByAirport.value[viewedAirport.value.id]=selectedKey.value
  viewedAirportId.value=target.id
  const nextAccounts=airportAccounts([target]),remembered=rememberedAccountByAirport.value[target.id]
  selectedKey.value=nextAccounts.some(account=>account.key===remembered)?remembered:nextAccounts[0]?.key||''
}
function cycleAirport(){
  const airportList=props.airports
  if(!airportList.length){viewedAirportId.value='';selectedKey.value='';return}
  const currentIndex=airportList.findIndex(airport=>airport.id===viewedAirport.value?.id)
  const nextAirport=airportList[(currentIndex+1)%airportList.length]
  showAirport(nextAirport.id)
  emit('viewed',nextAirport.id)
}
function defaultScopeKeys(){
  const propAirportIds=new Set(props.airports.map(airport=>airport.id))
  return workspaceAccounts.value.filter(account=>propAirportIds.has(account.airport.id)).map(account=>account.key)
}
function syncDefaultScope(){if(!scopeTouched.value)selectedScopeKeys.value=defaultScopeKeys()}
function setScopeSelected(key:string,checked:boolean){
  scopeTouched.value=true
  selectedScopeKeys.value=checked?[...new Set([...selectedScopeKeys.value,key])]:selectedScopeKeys.value.filter(value=>value!==key)
}
function mergeJob(job:SubscriptionRefreshJob){
  jobs.value=[job,...jobs.value.filter(existing=>existing.id!==job.id)]
  if(activeJob.value?.id===job.id||job.state==='running')activeJob.value=job
}
let pollTimer:ReturnType<typeof setTimeout>|undefined,disposed=false
function schedulePoll(id:string){clearTimeout(pollTimer);pollTimer=setTimeout(()=>void poll(id),700)}
async function poll(id:string){
  if(disposed)return
  try{
    const job=await api.refreshJob(id)
    if(disposed)return
    mergeJob(job)
    if(jobRunning(job))schedulePoll(id)
    else{activeJob.value=job;if(job.success)await workspace.refreshNodes()}
  }catch(e){if(!disposed)error.value=e instanceof Error?e.message:String(e)}
}
async function loadJobs(){
  jobsReadState.value='loading'
  try{
    const loaded=await api.refreshJobs()
    if(disposed)return
    jobs.value=loaded||[]
    jobsReadState.value='ready'
    const running=jobs.value.find(jobRunning)
    if(running){activeJob.value=running;schedulePoll(running.id)}
  }catch(e){if(!disposed){error.value=e instanceof Error?e.message:String(e);jobsReadState.value='error'}}
}
function requestID(){return globalThis.crypto?.randomUUID?.()||`refresh-${Date.now()}-${Math.random().toString(16).slice(2)}`}
async function startSelections(accountsToRefresh:AirportAccount[]){
  if(!accountsToRefresh.length||refreshDisabled.value)return
  loading.value=true;error.value=''
  try{
    const selections=accountsToRefresh.map(account=>({airport_id:account.airport.id,subscription_id:account.subscription.id}))
    const body:SubscriptionRefreshRequest={request_id:requestID(),selections}
    const job=await api.startRefresh(body)
    mergeJob(job)
    if(jobRunning(job))schedulePoll(job.id)
    else if(job.success)await workspace.refreshNodes()
  }catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}
}
async function startCurrent(){if(selected.value)await startSelections([selected.value])}
async function startChosenScope(){await startSelections(selectedScopeAccounts.value);scopeOpen.value=false}
async function startAll(){await startSelections(workspaceAccounts.value)}
async function cancelCurrent(){
  const job=cancelJob.value
  if(!job)return
  try{mergeJob(await api.cancelRefresh(job.id))}catch(e){error.value=e instanceof Error?e.message:String(e)}
}
async function retryFailed(){
  const job=visibleJob.value
  if(!job||visibleItem.value?.state!=='failed'||loading.value)return
  await retryJob(job)
}
async function retryJob(job:SubscriptionRefreshJob){
  if(job.failed===0||loading.value||jobRunning(activeJob.value))return
  loading.value=true;error.value=''
  try{
    const next=await api.startRefresh({request_id:requestID(),retry_of:job.id})
    mergeJob(next)
    if(jobRunning(next))schedulePoll(next.id)
  }catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}
}
watch(accounts,value=>{
  const airport=viewedAirport.value
  if(!airport){selectedKey.value='';return}
  const remembered=rememberedAccountByAirport.value[airport.id]
  if(!value.some(account=>account.key===selectedKey.value))selectedKey.value=value.some(account=>account.key===remembered)?remembered:value[0]?.key||''
},{immediate:true})
watch(selectedKey,value=>{
  const airport=viewedAirport.value
  if(airport&&accounts.value.some(account=>account.key===value))rememberedAccountByAirport.value[airport.id]=value
})
watch(()=>props.airports,syncDefaultScope,{immediate:true,deep:true})
watch(()=>workspace.airports.value,syncDefaultScope,{deep:true})
watch(disclosureSummary,value=>emit('summary',value),{immediate:true})
onMounted(()=>void loadJobs())
onBeforeUnmount(()=>{disposed=true;clearTimeout(pollTimer)})
defineExpose({cycleAirport,showAirport,viewedAirportName,usageRatio,viewedAccount:selected})
</script>

<template>
  <section class="subscription-overview" aria-label="首页订阅账户状态">
    <template v-if="selected">
      <div class="subscription-main">
        <div class="subscription-identity">
          <div class="subscription-icon" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m12 3 9 4.5-9 4.5-9-4.5L12 3Z"/><path d="m3 12 9 4.5 9-4.5"/><path d="m3 16.5 9 4.5 9-4.5"/></svg>
          </div>
          <div class="subscription-account">
            <UiSelect v-if="accounts.length>1" v-model="selectedKey" aria-label="首页用量订阅">
              <option v-for="entry in accounts" :key="entry.key" :value="entry.key">{{entry.subscription.name}}</option>
            </UiSelect>
            <strong v-else>{{selected.subscription.name}}</strong>
            <small>{{sourceLabel}} · {{updatedAtLabel}}</small>
            <div class="subscription-facts">
              <span class="subscription-expiry">{{expireLabel}}</span>
              <a v-if="officialLink" :href="officialLink" target="_blank" rel="noopener noreferrer">官网 ↗</a>
              <details v-if="extraLinks.length" class="subscription-extra-links">
                <summary>更多入口 {{extraLinks.length}}</summary>
                <div><a v-for="link in extraLinks" :key="link.label+link.url" :href="link.url" target="_blank" rel="noopener noreferrer">{{link.label}} ↗</a></div>
              </details>
              <button v-if="accountNotices.length" class="text-button" @click="noticeOpen=true">公告 {{accountNotices.length}} ›</button>
            </div>
          </div>
        </div>

        <div class="subscription-remaining" aria-label="剩余可用流量">
          <strong class="subscription-remaining-value">{{remaining===null?'未知':bytes(remaining)}}</strong>
          <span v-if="remainingRatio!==null" class="subscription-remaining-percent">{{(remainingRatio*100).toFixed(2)}}% 可用</span>
        </div>

        <div class="subscription-quota">
          <div v-if="remainingRatio!==null" class="usage-track" role="meter" aria-label="套餐剩余比例" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="Math.round(remainingRatio*100)">
            <i :style="{width:`${remainingRatio*100}%`}"/>
          </div>
          <div class="subscription-quota-line">已用 {{usedAmount===null?'未知':bytes(usedAmount)}} / 总量 {{total===undefined?'未知':bytes(total)}}</div>
        </div>

        <div class="subscription-daily">
          <h3>近 3 日用量</h3>
          <UsageDays :airport-id="selected.airport.id" :subscription-id="selected.subscription.id" :days="3" :refresh-version="workspace.usageRefreshVersion.value" summary/>
        </div>
      </div>

      <div class="subscription-actions">
        <button class="button primary" :disabled="refreshDisabled" @click="startCurrent">{{loading?'提交中…':'刷新当前订阅'}}</button>
        <div class="refresh-scope">
          <button class="button ghost" type="button" aria-label="选择刷新范围" :aria-expanded="scopeOpen" @click="scopeOpen=!scopeOpen">选择范围</button>
          <div v-if="scopeOpen" class="refresh-scope-panel" role="group" aria-label="刷新订阅范围">
            <label v-for="entry in workspaceAccounts" :key="entry.key">
              <input type="checkbox" :aria-label="`${entry.airport.name} · ${entry.subscription.name}`" :checked="selectedScopeKeys.includes(entry.key)" @change="setScopeSelected(entry.key,($event.target as HTMLInputElement).checked)">
              <span>{{entry.airport.name}} · {{entry.subscription.name}}</span>
            </label>
            <button class="button primary" :disabled="refreshDisabled||!selectedScopeAccounts.length" @click="startChosenScope">刷新所选 {{selectedScopeAccounts.length}}</button>
          </div>
        </div>
        <button class="button ghost" :disabled="refreshDisabled||!workspaceAccounts.length" @click="startAll">全部刷新</button>
        <span v-if="statusLabel" class="badge" :class="statusTone">{{statusLabel}}</span>
        <button v-if="visibleItem?.state==='failed'" class="text-button" :disabled="loading" @click="retryFailed">重试失败项</button>
        <button v-if="cancelJob" class="text-button danger" @click="cancelCurrent">取消刷新</button>
        <div v-if="visibleItem?.error_message||visibleJob?.persistence_error||error||jobsReadState==='error'" class="subscription-message" role="alert" :title="visibleItem?.error_message||visibleJob?.persistence_error||error||'刷新状态读取失败'">
          {{visibleItem?.error_message||visibleJob?.persistence_error||error||'刷新状态读取失败'}}
        </div>
        <details v-if="workspaceRefreshIssues.length" class="subscription-issues">
          <summary>刷新异常 {{workspaceRefreshIssues.length}} 项</summary>
          <p v-for="issue in workspaceRefreshIssues" :key="issue.account.key">
            <strong>{{issue.account.airport.name}} · {{issue.account.subscription.name}}</strong>
            <span>{{issue.item.state==='failed'?'刷新失败':'刷新中断'}}</span>
            <small v-if="issue.item.error_message">{{issue.item.error_message}}</small>
          </p>
          <div class="issue-retries">
            <template v-for="job in workspaceRefreshIssueJobs" :key="job.id">
              <button v-if="job.failed>0&&!(visibleJob?.id===job.id&&visibleItem?.state==='failed')" class="text-button" :disabled="refreshDisabled" @click="retryJob(job)">重试此任务失败项（{{job.failed}}）</button>
            </template>
          </div>
        </details>
        <span v-if="jobsReadState==='loading'" class="status-read muted">刷新状态读取中…</span>
        <span v-else-if="jobsReadState==='error'" class="status-read inline-error" role="alert">刷新状态未知</span>
      </div>
    </template>
    <span v-else class="muted">选择机场查看订阅</span>

    <Modal v-if="noticeOpen&&selected" title="订阅公告" @close="noticeOpen=false">
      <div v-for="notice in accountNotices" :key="notice.profile_id+'|'+notice.node_identity_key+'|'+notice.config_revision_key" class="notice-row">
        <small>{{selected.airport.name}} · {{selected.subscription.name}}</small>
        <p>{{notice.display_name}}</p>
      </div>
    </Modal>
  </section>
</template>

<style scoped>
.subscription-overview{position:relative;display:grid;gap:6px;min-width:0;min-height:120px;padding:8px 14px;border:1px solid var(--line);border-radius:14px;background:var(--surface);color:var(--ink);font-variant-numeric:tabular-nums;box-shadow:0 2px 8px rgba(27,49,79,.035)}
.subscription-main{display:grid;grid-template-columns:minmax(175px,1fr) minmax(200px,1.25fr) minmax(190px,1.15fr) minmax(120px,.75fr);align-items:center;gap:12px;min-width:0}
.subscription-identity{display:flex;align-items:center;gap:8px;min-width:0}.subscription-icon{display:grid;place-items:center;flex:none;width:32px;height:32px;border-radius:9px;background:var(--blue-soft);color:var(--blue)}.subscription-icon svg{width:21px;height:21px}
.subscription-account{display:grid;gap:1px;min-width:0}.subscription-account>strong,.subscription-account>small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.subscription-account>strong{font-size:14px;font-weight:650}.subscription-account>small{font-size:10px;line-height:1.25;color:var(--muted)}.subscription-account .ui-select{max-width:100%}.subscription-account .ui-select-trigger{min-height:26px;padding:2px 6px;font-size:14px;font-weight:650}
.subscription-facts{display:flex;align-items:center;flex-wrap:wrap;gap:1px 7px;color:var(--muted);font-size:10px;line-height:1.2}.subscription-facts a,.subscription-facts button{white-space:nowrap}.subscription-expiry{white-space:nowrap}.subscription-extra-links{position:relative}.subscription-extra-links summary{cursor:pointer;white-space:nowrap}.subscription-extra-links>div{position:absolute;top:calc(100% + 4px);left:0;z-index:10;display:grid;gap:7px;min-width:130px;padding:9px;border:1px solid var(--line);border-radius:8px;background:var(--surface);box-shadow:var(--shadow)}.subscription-facts .text-button{font-size:10px}
.subscription-remaining{display:flex;align-items:center;gap:7px;min-width:0;white-space:nowrap}.subscription-remaining-value{max-width:100%;overflow:hidden;text-overflow:ellipsis;color:var(--blue);font-size:26px;font-weight:700;line-height:1;letter-spacing:-.035em}.subscription-remaining-percent{display:inline-flex;align-items:center;flex:none;min-height:21px;padding:2px 7px;border-radius:7px;background:var(--blue-soft);color:var(--blue);font-size:10px;font-weight:600;white-space:nowrap}
.subscription-quota{display:grid;gap:4px;min-width:0}.subscription-quota-line{min-width:0;overflow:hidden;color:var(--muted);font-size:11px;line-height:1.2;white-space:nowrap;text-overflow:ellipsis}.usage-track{height:9px;overflow:hidden;border-radius:999px;background:var(--blue-soft);box-shadow:inset 0 0 0 1px color-mix(in srgb,var(--blue) 7%,transparent)}.usage-track i{display:block;height:100%;border-radius:inherit;background:var(--blue);transition:width .35s ease}
.subscription-daily{display:flex;flex-direction:column;justify-content:center;gap:3px;min-width:0;padding-left:10px;border-left:1px solid var(--line)}.subscription-daily h3{margin:0;color:var(--muted);font-size:11px;line-height:1.2;font-weight:600}.subscription-daily :deep(.cst-usage-summary){height:40px;grid-template-rows:12px minmax(0,1fr) 12px;gap:3px;padding-bottom:2px}.subscription-daily :deep(.cst-usage-day-value),.subscription-daily :deep(.cst-usage-day-label){font-size:9px}.subscription-daily :deep(.cst-usage-day-track){width:13px}.subscription-daily :deep(.cst-usage-caption){font-size:9px;line-height:10px}
.subscription-actions{display:flex;align-items:center;flex-wrap:wrap;gap:6px 8px;min-height:30px;font-size:11px}.subscription-actions .button{min-height:28px;padding:4px 10px;font-size:12px}.subscription-actions .badge{padding:4px 7px;font-size:11px}.subscription-actions .text-button{font-size:11px}.subscription-message{max-width:220px;overflow:hidden;color:var(--red);font-size:10px;line-height:1.3;white-space:nowrap;text-overflow:ellipsis}.subscription-issues{position:relative;font-size:11px}.subscription-issues summary{width:max-content;max-width:100%;color:var(--red);cursor:pointer}.subscription-issues p{display:flex;flex-wrap:wrap;gap:4px 8px;margin:6px 0 0;line-height:1.5}.subscription-issues p small{flex:1 0 100%;color:var(--muted)}.issue-retries{display:flex;flex-wrap:wrap;gap:8px;margin-top:7px}.issue-retries button{font-size:12px}.status-read{font-size:11px}.refresh-scope{position:relative}.refresh-scope-panel{position:absolute;top:calc(100% + 5px);left:0;z-index:20;display:grid;gap:8px;width:min(310px,calc(100vw - 28px));max-height:250px;overflow:auto;padding:10px;border:1px solid var(--line);border-radius:9px;background:var(--surface);box-shadow:var(--shadow)}.refresh-scope-panel label{display:flex;align-items:center;gap:8px;font-size:12px;line-height:1.4}.refresh-scope-panel label span{overflow-wrap:anywhere}.refresh-scope-panel>.button{justify-self:start}.notice-row{padding:8px 0;border-bottom:1px solid var(--line)}.notice-row small{color:var(--muted)}.notice-row p{margin-top:4px;overflow-wrap:anywhere}
@media(max-width:1200px){.subscription-main{grid-template-columns:minmax(155px,1fr) minmax(180px,1.2fr) minmax(170px,1.1fr) minmax(105px,.65fr);gap:8px}.subscription-overview{padding-right:12px;padding-left:12px}.subscription-remaining{gap:5px}.subscription-remaining-value{font-size:19px}.subscription-daily{padding-left:7px}}
@media(max-width:980px){.subscription-main{grid-template-columns:minmax(0,1.1fr) minmax(0,1fr);row-gap:8px}.subscription-identity,.subscription-daily{grid-column:1/-1}.subscription-daily{padding:5px 0 0;border-top:1px solid var(--line);border-left:0}.subscription-daily :deep(.cst-usage-summary){max-width:220px}}
@media(max-width:760px){.subscription-overview{gap:8px;padding:10px}.subscription-main{grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:8px}.subscription-identity,.subscription-daily{grid-column:1/-1}.subscription-remaining{flex-wrap:wrap}.subscription-remaining-value{font-size:24px}.subscription-actions{gap:6px}.subscription-message{max-width:100%}.refresh-scope-panel{left:0;right:auto}}
</style>
