<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref} from 'vue'
import {nodeDisplayName} from '../nodePresentation'
import {api,request} from '../api'
import {useWorkspace} from '../workspace'
import {bytes,date,stateLabel,type MonitorJob,type NodeOption} from '../domain'
import Icon from './Icon.vue'
import MonitorTimeBand from './MonitorTimeBand.vue'
import PeriodicSamplingPanel from './PeriodicSamplingPanel.vue'
import Modal from './Modal.vue'

type ReadState={value:Record<string,any>|null;readAt:string;error:string}
const initialReadState=():ReadState=>({value:null,readAt:'',error:''})
const w=useWorkspace()
const busyJobs=ref(new Set<string>()),actionErrors=ref<Record<string,string>>({}),actionWarnings=ref<Record<string,string>>({})
const deleteErrors=ref<Record<string,string>>({})
const remove=ref<MonitorJob|null>(null),budgetState=ref<ReadState>(initialReadState()),storageState=ref<ReadState>(initialReadState())
const details=ref<MonitorJob|null>(null),runs=ref<Record<string,any>[]>([]),runsLoading=ref(false),runsError=ref('')
const jobsReadAt=ref(''),jobsError=ref(''),jobsLoading=ref(false),refreshing=ref(false)
let timer:ReturnType<typeof setInterval>|undefined,reloadGeneration=0,budgetGeneration=0,storageGeneration=0,runsGeneration=0,mounted=false,latestReload:Promise<boolean>|null=null

const active=computed(()=>w.jobs.value.filter(job=>job.state==='running').length)
const jobSummary=computed(()=>{
  if(jobsReadAt.value)return `${active.value} 个任务运行中 · ${w.jobs.value.length} 个任务${jobsLoading.value?' · 正在刷新':''}`
  if(w.jobs.value.length)return `${active.value} 个任务运行中 · ${w.jobs.value.length} 个任务（本页尚无成功刷新）`
  if(jobsError.value)return '监测任务状态尚未确认'
  return jobsLoading.value?'正在读取监测任务…':'监测任务状态尚未读取'
})
const resourceWarning=computed(()=>{
  const budgetReason=budgetState.value.value?.blocked_reason
  if(budgetReason)return budgetState.value.error?`预算状态上次成功读取于 ${budgetState.value.readAt}：${budgetReason}`:budgetReason
  if(storageState.value.value?.protected){
    const reason='历史容量达到保护阈值，新的监测采样已受限。'
    return storageState.value.error?`存储保护状态上次成功读取于 ${storageState.value.readAt}：${reason}`:reason
  }
  return ''
})

function errorMessage(error:unknown){return error instanceof Error?error.message:String(error)}
function resourceMessage(state:ReadState,label:string){
  if(state.error)return `${label}${state.value?'刷新失败；显示上次成功读取于 '+state.readAt:'读取失败；目前无可用读数'}：${state.error}`
  if(state.readAt)return state.value?`最近成功读取于 ${state.readAt}`:`已读取于 ${state.readAt}，接口未提供可用读数`
  return `${label}尚未读取`
}
function formatBackendBytes(value:unknown){return typeof value==='number'&&Number.isFinite(value)?bytes(value):'未知'}
function isBusy(id:string){return busyJobs.value.has(id)}
function clearActionError(id:string){const next={...actionErrors.value};delete next[id];actionErrors.value=next}
function setActionError(id:string,message:string){actionErrors.value={...actionErrors.value,[id]:message}}
function clearActionWarning(id:string){const next={...actionWarnings.value};delete next[id];actionWarnings.value=next}
function setActionWarning(id:string,message:string){actionWarnings.value={...actionWarnings.value,[id]:message}}
function clearDeleteError(id:string){const next={...deleteErrors.value};delete next[id];deleteErrors.value=next}
function setDeleteError(id:string,message:string){deleteErrors.value={...deleteErrors.value,[id]:message}}

async function reload(){
  const generation=++reloadGeneration
  const budgetRead=++budgetGeneration,storageRead=++storageGeneration
  refreshing.value=true;jobsLoading.value=true
  Promise.resolve().then(()=>api.budget()).then(value=>{
    if(!mounted||budgetRead!==budgetGeneration)return
    budgetState.value={value:value??null,readAt:new Date().toLocaleTimeString(),error:''}
  },error=>{
    if(!mounted||budgetRead!==budgetGeneration)return
    budgetState.value={...budgetState.value,error:errorMessage(error)}
  })
  Promise.resolve().then(()=>api.storage()).then(value=>{
    if(!mounted||storageRead!==storageGeneration)return
    storageState.value={value:value??null,readAt:new Date().toLocaleTimeString(),error:''}
  },error=>{
    if(!mounted||storageRead!==storageGeneration)return
    storageState.value={...storageState.value,error:errorMessage(error)}
  })
  const response=Promise.resolve().then(()=>api.jobs()).then(value=>{
    if(!mounted||generation!==reloadGeneration)return false
    w.jobs.value=value||[];jobsReadAt.value=new Date().toLocaleTimeString();jobsError.value='';actionWarnings.value={}
    jobsLoading.value=false;refreshing.value=false
    return true
  },error=>{
    if(!mounted||generation!==reloadGeneration)return false
    jobsError.value=errorMessage(error)
    jobsLoading.value=false;refreshing.value=false
    return true
  })
  let current!:Promise<boolean>
  current=response.then(async applied=>{
    if(applied||!mounted)return applied
    const latest=latestReload
    return latest&&latest!==current?latest:false
  })
  latestReload=current
  return current
}

async function action(job:MonitorJob,type:string,body?:unknown){
  if(isBusy(job.id))return
  busyJobs.value.add(job.id);clearActionError(job.id);clearActionWarning(job.id)
  try{
    try{await api.jobAction(job.id,type,body)}
    catch(error){
      if(!mounted)return
      setActionError(job.id,errorMessage(error))
      await reload()
      return
    }
    if(!mounted)return
    await reload()
    if(!mounted)return
    if(jobsError.value)setActionWarning(job.id,`操作请求已返回，但任务状态未能重新读取：${jobsError.value}`)
    else if(type==='trigger')w.notify('已请求执行一次检测；结果请查看运行记录')
    else w.notify('任务状态已按后端列表重新读取')
  }finally{
    if(mounted)busyJobs.value.delete(job.id)
  }
}

async function deleteJob(){
  const target=remove.value
  if(!target||isBusy(target.id))return
  busyJobs.value.add(target.id);clearDeleteError(target.id)
  try{
    await api.deleteJob(target.id)
    if(!mounted)return
    if(remove.value?.id===target.id)remove.value=null
    await reload()
    if(!mounted)return
    w.notify(jobsError.value?'删除请求已返回；任务列表刷新失败，请刷新确认。':'任务已删除，原始历史保留')
  }catch(error){
    if(!mounted)return
    setDeleteError(target.id,errorMessage(error))
  }finally{
    if(mounted)busyJobs.value.delete(target.id)
  }
}

function closeRuns(){runsGeneration++;details.value=null;runs.value=[];runsLoading.value=false;runsError.value=''}
async function show(job:MonitorJob){
  const token=++runsGeneration
  details.value=job;runs.value=[];runsLoading.value=true;runsError.value=''
  try{
    const result=await request<Record<string,any>[]>(`/api/monitor/runs?job_id=${encodeURIComponent(job.id)}&limit=20`,'GET',undefined,'QueryMonitorRuns',[job.id,20])
    if(mounted&&token===runsGeneration)runs.value=result||[]
  }catch(error){
    if(mounted&&token===runsGeneration)runsError.value=errorMessage(error)
  }finally{
    if(mounted&&token===runsGeneration)runsLoading.value=false
  }
}
function cancelDelete(){if(remove.value&&!isBusy(remove.value.id))remove.value=null}
function openDelete(job:MonitorJob){clearDeleteError(job.id);remove.value=job}
function nodeOption(job:MonitorJob,node:NodeOption):NodeOption{return {...node,profile_id:job.profile_id,profile_name:job.profile_name,country_code:'',country_flag:''}}

onMounted(()=>{
  mounted=true
  void reload()
  timer=setInterval(()=>{if(w.page.value==='monitor'&&document.visibilityState==='visible')void reload()},15000)
})
onBeforeUnmount(()=>{mounted=false;reloadGeneration++;runsGeneration++;clearInterval(timer)})
</script>

<template>
  <section class="page-view monitor-view">
    <div class="page-header">
      <div>
        <h1>持续监测</h1>
        <p>{{jobSummary}}</p>
      </div>
      <div class="button-group">
        <button class="button ghost" :aria-busy="refreshing" @click="void reload()"><Icon name="refresh"/>{{refreshing?'刷新中…':'刷新'}}</button>
        <button class="button primary" @click="w.page.value='home';w.multiSelect.value=true"><Icon name="plus"/>选节点创建</button>
      </div>
    </div>
    <p v-if="jobsError" class="inline-warning monitor-read-error" role="alert">
      任务列表刷新失败{{jobsReadAt?`；显示上次成功读取于 ${jobsReadAt}`:'；尚未确认是否存在任务'}}：{{jobsError}}
    </p>
    <div class="monitor-sampling-panel"><PeriodicSamplingPanel/></div>
    <div class="monitor-resource monitor-resource-ledger" :aria-busy="refreshing" aria-live="polite">
      <div class="monitor-resource-values">
        <div class="monitor-resource-item">
          <span v-if="budgetState.value">本工具探测消耗 · 今日请求 {{budgetState.value.usage?.requests_used??'未知'}} · 已读取 {{formatBackendBytes(budgetState.value.usage?.bytes_used)}}</span>
          <span v-else-if="budgetState.readAt">请求预算读取成功，但接口未提供可用读数</span>
          <span v-else>请求预算尚未读取</span>
          <small :class="{'inline-error':!!budgetState.error}" :role="budgetState.error?'alert':undefined">{{resourceMessage(budgetState,'预算')}}</small>
        </div>
        <div class="monitor-resource-item">
          <span v-if="storageState.value">历史占用 {{formatBackendBytes(storageState.value.total_bytes)}}</span>
          <span v-else-if="storageState.readAt">历史占用读取成功，但接口未提供可用读数</span>
          <span v-else>历史占用尚未读取</span>
          <small :class="{'inline-error':!!storageState.error}" :role="storageState.error?'alert':undefined">{{resourceMessage(storageState,'存储')}}</small>
        </div>
      </div>
      <button class="text-button" @click="w.page.value='settings'">调整监测与保存策略</button>
    </div>
    <p v-if="resourceWarning" class="inline-warning">{{resourceWarning}}</p>

    <div v-if="!jobsReadAt&&!w.jobs.value.length" class="loading-state" :class="{'inline-warning':!!jobsError}" :role="jobsError?'alert':'status'">
      {{jobsError?'任务列表尚未读取，无法确认是否有任务。':'正在读取监测任务…'}}
    </div>
    <div v-else-if="!w.jobs.value.length" class="empty-state">
      <Icon name="monitor"/>
      <h3>{{jobsError&&jobsReadAt?'上次成功读取时没有监测任务':'让常用节点持续留下记录'}}</h3>
      <p v-if="jobsError&&jobsReadAt">刷新失败，任务列表可能已变化。请刷新确认后再操作。</p>
      <p v-else>在首页多选节点，再点“加入持续监测”。跨订阅选择会自动拆成任务。</p>
      <button class="button primary" @click="w.page.value='home';w.multiSelect.value=true">选择节点</button>
    </div>
    <div v-else class="monitor-cards">
      <div class="monitor-cards-heading"><div><h2>监测任务</h2></div><span class="monitor-task-count">{{w.jobs.value.length}} 项</span></div>
      <article v-for="job in w.jobs.value" :key="job.id" class="monitor-card" :aria-busy="isBusy(job.id)">
        <header>
          <div><h2>{{job.name}}</h2><small>{{job.profile_name}} · {{job.scope_kind==='current_patrol'?'当前全量巡检 · 延迟任务':'冻结范围任务 · 不随订阅自动改范围'}}</small></div>
          <span class="badge" :class="job.state==='running'&&!jobsError?'good':job.state==='blocked'?'warn':''" :title="jobsError?'任务列表刷新失败，这是上次成功读取时的状态':''">{{job.state==='running'?(jobsError?'上次读取时运行中':'监测中'):stateLabel(job.state)}}</span>
        </header>
        <div class="monitor-spec">
          <span>{{job.node_keys?.length||0}} 个节点</span>
          <span>每 {{job.interval_seconds>=60?`${job.interval_seconds/60} 分钟`:`${job.interval_seconds} 秒`}}</span>
          <span>{{job.probe_set==='latency_four_v1'?'四站延迟':job.probe_set==='latency_six_v1'?'旧六站延迟':job.probe_set==='light'?'旧单站延迟':job.probe_set==='service'?'基础服务':'深度诊断'}}</span>
          <span>{{job.sampling_tier==='focus'?'重点':job.sampling_tier==='sparse'?'稀疏':'常规'}}采样</span>
        </div>
        <MonitorTimeBand :job="job"/>
        <p v-if="job.blocked_reason||job.recovery_reason||job.persistence_error||job.storage_reason||job.budget_reason" class="inline-warning">
          {{job.blocked_reason||job.persistence_error||job.storage_reason||job.budget_reason||job.recovery_reason}}
        </p>
        <details v-if="job.unresolved_nodes?.length" class="monitor-nodes">
          <summary>完整缺失／不一致清单 · {{job.unresolved_node_count}} 个（整项未启动）</summary>
          <div v-for="node in job.unresolved_nodes" :key="node.node_identity_key+':'+node.node_key" class="compact-row">
            <strong>{{node.display_name}}</strong><small>{{node.reason}}</small>
            <details><summary>冻结稳定引用</summary><code>{{node.node_key}} · {{node.node_identity_key}} · {{node.config_revision_key}}</code></details>
          </div>
        </details>
        <details class="monitor-nodes">
          <summary>查看 {{job.nodes?.length||0}} 个节点</summary>
          <button v-for="node in job.nodes" :key="node.node_key" class="compact-row text-button" @click="w.inspectNode.value=nodeOption(job,node)">
            {{nodeDisplayName(node)}}<Icon name="chevron"/>
          </button>
        </details>
        <p v-if="actionErrors[job.id]" class="inline-error" role="alert">操作失败：{{actionErrors[job.id]}}</p>
        <p v-if="actionWarnings[job.id]" class="inline-warning" role="alert">操作未确认：{{actionWarnings[job.id]}}</p>
        <footer>
          <div class="button-group">
            <button v-if="job.state==='running'" class="button" :disabled="isBusy(job.id)" :aria-busy="isBusy(job.id)" @click="action(job,'pause')">
              <Icon name="pause"/>{{isBusy(job.id)?'正在暂停…':'暂停'}}
            </button>
            <button v-else class="button primary" :disabled="isBusy(job.id)||job.state==='blocked'" :aria-busy="isBusy(job.id)" @click="action(job,job.state==='paused'?'resume':'start')">
              <Icon name="play"/>{{isBusy(job.id)?job.state==='paused'?'正在继续…':'正在启动…':job.state==='paused'?'继续':'启动'}}
            </button>
            <button v-if="job.state==='running'||job.state==='paused'" class="button ghost" :disabled="isBusy(job.id)" :aria-busy="isBusy(job.id)" @click="action(job,'stop')">
              {{isBusy(job.id)?'正在停止…':'停止'}}
            </button>
            <button class="button ghost" :disabled="isBusy(job.id)||!['running','paused'].includes(job.state)" :aria-busy="isBusy(job.id)" @click="action(job,'trigger')">
              {{isBusy(job.id)?'正在请求检测…':'检测一次'}}
            </button>
            <button class="button ghost" @click="show(job)">运行记录</button>
            <button class="button ghost icon-button" aria-label="删除监测任务" :disabled="isBusy(job.id)" @click="openDelete(job)"><Icon name="trash"/></button>
          </div>
          <label class="check-field small">
            <input type="checkbox" :checked="job.resume_on_launch" :disabled="isBusy(job.id)" @change="action(job,'resume-on-launch',{enabled:($event.target as HTMLInputElement).checked})">
            启动应用时继续
          </label>
        </footer>
      </article>
    </div>

    <Modal v-if="remove" title="删除监测任务" :closable="!isBusy(remove.id)" @close="cancelDelete">
      <p>删除「{{remove.name}}」并停止后续监测，已经保存的原始记录保留。</p>
      <p v-if="deleteErrors[remove.id]" class="inline-error" role="alert">删除失败：{{deleteErrors[remove.id]}}</p>
      <template #footer>
        <button class="button" :disabled="isBusy(remove.id)" @click="cancelDelete">返回</button>
        <button class="button danger-fill" :disabled="isBusy(remove.id)" :aria-busy="isBusy(remove.id)" @click="deleteJob">{{isBusy(remove.id)?'正在删除…':'删除任务'}}</button>
      </template>
    </Modal>
    <Modal v-if="details" :title="details.name+' · 运行记录'" wide @close="closeRuns">
      <div class="table-scroll">
        <table>
          <thead><tr><th>时间</th><th>状态</th><th>至少一站成功的节点</th><th>无成功证据的节点</th></tr></thead>
          <tbody>
            <tr v-for="run in runs" :key="run.run_id">
              <td>{{date(run.started_at)}}</td><td>{{stateLabel(run.status)}}<small v-if="run.error_message">{{run.error_message}}</small></td>
              <td>{{run.success_nodes}} / {{run.total_nodes}}</td><td>{{run.failed_nodes}}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="runsLoading" class="muted">读取运行记录中…</p>
      <p v-else-if="runsError" class="inline-error" role="alert">运行记录读取失败：{{runsError}} <button class="text-button" @click="show(details!)">重新读取</button></p>
      <p v-else-if="!runs.length" class="muted">还没有运行记录。</p>
    </Modal>
  </section>
</template>

<style scoped>
.monitor-resource-values{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(340px,100%),1fr));gap:6px 18px;flex:1;min-width:0}
.monitor-resource-item{display:flex;align-items:baseline;gap:5px 10px;flex-wrap:wrap;min-width:0}
.monitor-resource-item small{overflow-wrap:anywhere}
.monitor-resource-item .inline-error{color:var(--red)}
.monitor-read-error{margin:-14px 0 18px;overflow-wrap:anywhere}
.monitor-card footer .button-group{max-width:100%}
.monitor-card footer .button{min-height:36px}
@media(max-width:800px){
  .monitor-resource{align-items:flex-start}
  .monitor-resource-values{grid-template-columns:minmax(0,1fr)}
  .monitor-card footer{align-items:flex-start}
  .monitor-card footer .button-group{width:100%}
  .monitor-card footer .button{min-height:40px}
  .monitor-card footer .check-field{width:100%}
}
</style>
<style src="./secondary-observatory.css" scoped></style>
