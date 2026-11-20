<script setup lang="ts">
import {computed,defineAsyncComponent,nextTick,onMounted,onBeforeUnmount,provide,ref,watch} from 'vue'
import {useLocalNotifications} from './localNotifications'
import {api} from './api'
import {createWorkspace,terminal,workspaceKey} from './workspace'
import type {Page} from './domain'
import Icon from './components/Icon.vue'
import Modal from './components/Modal.vue'
import HomeView from './components/HomeView.vue'
import {latencyReportText} from './latencyReport'
import HoverTip from './components/HoverTip.vue'
const RecommendationView=defineAsyncComponent(()=>import('./components/RecommendationView.vue'))
const AirportsView=defineAsyncComponent(()=>import('./components/AirportsView.vue'))
const MonitorView=defineAsyncComponent(()=>import('./components/MonitorView.vue'))
const HistoryView=defineAsyncComponent(()=>import('./components/HistoryView.vue'))
const SettingsView=defineAsyncComponent(()=>import('./components/SettingsView.vue'))
const DataView=defineAsyncComponent(()=>import('./components/DataView.vue'))
const TestPlanModal=defineAsyncComponent(()=>import('./components/TestPlanModal.vue'))
const MonitorCreateModal=defineAsyncComponent(()=>import('./components/MonitorCreateModal.vue'))
const NodeDetailModal=defineAsyncComponent(()=>import('./components/NodeDetailModal.vue'))
const QueueModal=defineAsyncComponent(()=>import('./components/QueueModal.vue'))
const AutoPickReportModal=defineAsyncComponent(()=>import('./components/AutoPickReportModal.vue'))
const CommandPalette=defineAsyncComponent(()=>import('./components/CommandPalette.vue'))
import {date,projectNames} from './domain'
import {applyTheme,savedThemePreference,type ThemePreference} from './theme'
const w=createWorkspace();provide(workspaceKey,w);useLocalNotifications(w)
const windowHidden=ref(document.hidden)
function visibilityChanged(){windowHidden.value=document.hidden}
const password=ref(''),loginError=ref(''),loggingIn=ref(false)
const themePref=ref<ThemePreference>(savedThemePreference())
const themeOrder:ThemePreference[]=['light','dark','system']
const themeMeta=computed(()=>({light:{label:'浅色',icon:'sun'},dark:{label:'深色',icon:'moon'},system:{label:'自动',icon:'settings'}}[themePref.value]))
function onThemeChanged(e:Event){const detail=(e as CustomEvent<{preference?:ThemePreference}>).detail;if(detail?.preference)themePref.value=detail.preference}
async function cycleTheme(){const next=themeOrder[(themeOrder.indexOf(themePref.value)+1)%themeOrder.length];await applyTheme(next)}
const primaryPages:{id:Page;label:string;icon:string}[]=[{id:'home',label:'线路工作台',icon:'home'},{id:'recommend',label:'现在用哪个',icon:'pulse'}]
const managePages:{id:Page;label:string;icon:string}[]=[{id:'airports',label:'机场与账户',icon:'layers'},{id:'monitor',label:'持续监测',icon:'monitor'},{id:'history',label:'检测记录',icon:'history'}]
const pages=[...primaryPages,...managePages]
const navLabels:Record<string,string>={home:'线路',recommend:'为你选线',airports:'订阅',monitor:'监测',history:'记录'}
const shortcutLabel=/Mac|iPhone|iPad/.test(navigator.platform)?'⌘ K':'Ctrl K'
const utilities:{id:Page;label:string;icon:string}[]=[{id:'settings',label:'偏好设置',icon:'settings'},{id:'data',label:'数据与备份',icon:'data'}]
const pageKeywords:Record<Page,string[]>={home:['线路','节点','测速'],recommend:['为你选线','推荐'],airports:['订阅','账户'],monitor:['监测','自动精选','定时检测'],history:['记录','报告','历史'],settings:['偏好','设置'],data:['数据','备份','恢复']}
const palettePages=[...pages,...utilities].map(p=>({...p,keywords:pageKeywords[p.id]}))
const palette=ref(false)
const accessReady=computed(()=>!w.authRequired.value&&!loggingIn.value)
const workspaceReady=computed(()=>accessReady.value&&w.booted.value&&w.nodesReady.value&&w.setup.value?.state==='ready')
const dataReady=computed(()=>accessReady.value&&w.booted.value&&!!w.setup.value&&(w.setup.value.state!=='ready'||w.nodesReady.value))
const startupBusy=computed(()=>w.loading.value||!w.booted.value&&!w.error.value)
const runningJobs=computed(()=>w.jobs.value.filter(j=>j.state==='running').length)
const header=ref<HTMLElement>(),headerHeight=ref(0)
let headerObserver:ResizeObserver|undefined
const recentReport=computed(()=>w.batches.value.find(batch=>terminal(batch.state)&&!!batch.items?.length))
const recentReportLabel=computed(()=>recentReport.value?latencyReportText(recentReport.value):'')

const task=computed(()=>{
 const rows=w.queue.value
 const batch=w.activeBatch.value,batchProgress=w.activeBatchProgress.value
 const matchingProgress=batchProgress?.batch_id===batch?.batch_id?batchProgress:null
 const rowSaveFailures=rows.filter(q=>q.attempt?.persistence_state==='failed'||q.error?.startsWith('保存失败')).length
 const unsaved=batch&&!terminal(batch.state)?Math.max(rowSaveFailures,matchingProgress?.save_failed||0):rowSaveFailures
 const errors=rows.filter(q=>q.state==='failed'&&q.attempt?.persistence_state!=='failed'&&!q.error?.startsWith('保存失败')).length
 const notRun=rows.filter(q=>['cancelled','not_executed','interrupted'].includes(q.state)).length
 const attention=[unsaved?`${unsaved} 项未保存`:'',errors?`${errors} 项出错`:'',notRun?`${notRun} 项取消或未完成`:''].filter(Boolean).join(' · ')
 if(w.pendingStart.value)return {state:'pending',title:'正在确认启动状态',detail:'等待后台回应',progress:null,attention,canCancel:true}
 const total=rows.length||matchingProgress?.total||0,done=rows.length?w.done.value:matchingProgress?.processed||0
 const detail=total?`已处理 ${done}/${total}`:''
 if(w.busy.value){
  const executing=rows.filter(q=>!terminal(q.state))
  const saving=batch?.state==='saving'||rows.some(q=>q.attempt?.persistence_state==='saving')
  const savingOnly=batch?.state==='saving'?!rows.some(q=>q.state==='queued'):saving&&!executing.length
  const projects=[...new Set(executing.map(q=>projectNames[q.project]))]
  let title=projects.length===1?`正在${projects[0]}`:'检测进行中'
  if(w.cancelRequested.value)title='正在取消检测'
  else if(savingOnly)title='正在保存结果'
  return {state:'running',title,detail,progress:total?Math.min(1,done/total):null,attention,canCancel:!savingOnly}
 }
 if(!rows.length)return null
 return {state:attention||done<total?'attention':'idle',title:done<total?'检测已停止，部分项目待确认':'本次检测已结束',detail,progress:null,attention,canCancel:false}
})
const connection=computed(()=>!accessReady.value?{tone:'warn',label:'需要访问密码'}:w.connected.value?{tone:'good',label:'本地服务已连接'}:w.loading.value?{tone:'warn',label:'正在连接本地服务'}:{tone:'bad',label:'本地服务未连接'})
function paletteKey(e:KeyboardEvent){if(e.isComposing||!palette.value&&document.querySelector('dialog[open]'))return;if((e.metaKey||e.ctrlKey)&&!e.altKey&&e.key.toLowerCase()==='k'&&workspaceReady.value){e.preventDefault();palette.value=!palette.value}}
let timer:ReturnType<typeof setInterval>|undefined
let workbenchScroll=0
let pendingScroll:{page:Page;top:number;hasNodeTarget:boolean}|null=null
watch(w.page,(page,previous)=>{
 if(previous==='home'&&workspaceReady.value)workbenchScroll=window.scrollY
 pendingScroll={page,top:page==='home'?workbenchScroll:0,hasNodeTarget:!!w.viewRequest.value}
},{flush:'sync'})
watch(w.viewRequest,node=>{
 if(node&&pendingScroll?.page==='home')pendingScroll.hasNodeTarget=true
},{flush:'sync'})
watch(w.page,async()=>{
 const position=pendingScroll
 // v-show applies after the page patch. Before that, the browser clamps the scroll to the hidden list's height.
 await nextTick()
 if(!position||pendingScroll!==position)return
 pendingScroll=null
 if(dataReady.value&&(position.page!=='home'||!position.hasNodeTarget))window.scrollTo({top:position.top,behavior:'instant'})
},{flush:'post'})
function navigate(page:Page){
 if(page==='data'?!dataReady.value:!workspaceReady.value)return
 if(w.page.value===page)window.scrollTo({top:0,behavior:'instant'})
 else w.page.value=page
}
function measureHeader(){headerHeight.value=header.value?.getBoundingClientRect().height||0}
function authExpired(){palette.value=false;workbenchScroll=0;pendingScroll=null;w.authRequired.value=true;w.dispose()}
async function login(){if(loggingIn.value)return;loggingIn.value=true;loginError.value='';try{await api.login(password.value);password.value='';w.authRequired.value=false;await w.boot()}catch(e){loginError.value=e instanceof Error?e.message:String(e)}finally{loggingIn.value=false}}
onMounted(()=>{
 measureHeader()
 if(header.value&&typeof ResizeObserver!=='undefined'){
  headerObserver=new ResizeObserver(measureHeader)
  headerObserver.observe(header.value)
 }
 document.addEventListener('visibilitychange',visibilityChanged)
 window.addEventListener('keydown',paletteKey)
 window.addEventListener('cst:theme',onThemeChanged)
 window.addEventListener('speedtest:auth-required',authExpired)
 void w.boot()
 timer=setInterval(()=>{if(w.busy.value&&!w.running.value&&document.visibilityState==='visible')void w.syncActive()},2500)
})
onBeforeUnmount(()=>{headerObserver?.disconnect();document.removeEventListener('visibilitychange',visibilityChanged);window.removeEventListener('keydown',paletteKey);window.removeEventListener('cst:theme',onThemeChanged);w.dispose();clearInterval(timer);window.removeEventListener('speedtest:auth-required',authExpired)})
</script>
<template>
  <div class="cst-shell observatory-app" :class="{'window-hidden':windowHidden}" :style="headerHeight?{'--cst-nav-height':headerHeight+'px'}:undefined">
    <header ref="header" class="cst-chrome">
      <div class="cst-masthead">
      <a class="cst-brand" href="#" @click.prevent="navigate('home')"><span class="cst-brand-mark"><Icon name="pulse"/></span><span class="cst-brand-copy"><strong>CST</strong><small>线路检测</small></span></a>
      <nav class="cst-nav" aria-label="主导航">
        <button v-for="p in pages" :key="p.id" type="button" :disabled="!workspaceReady" :title="p.label" :aria-label="p.label" :aria-current="workspaceReady&&w.page.value===p.id?'page':undefined" @click="navigate(p.id)">
          <Icon :name="p.icon"/><span>{{navLabels[p.id]}}</span>
          <small v-if="p.id==='monitor'&&runningJobs" :aria-label="runningJobs+' 个监测任务运行中'">{{runningJobs}}</small>
        </button>
      </nav>
      <div class="cst-header-tools">
        <button v-if="workspaceReady" class="cst-jump" type="button" aria-keyshortcuts="Control+K Meta+K" title="搜索页面、机场和节点" @click="palette=true"><Icon name="search"/><span>快速查找</span><kbd>{{shortcutLabel}}</kbd></button>
        <nav class="cst-utilities" aria-label="工具"><button v-for="u in utilities" :key="u.id" type="button" :disabled="u.id==='data'?!dataReady:!workspaceReady" :aria-label="u.label" :title="u.label" :aria-current="dataReady&&w.page.value===u.id?'page':undefined" @click="navigate(u.id)"><Icon :name="u.icon"/><span>{{u.id==='settings'?'偏好':'数据'}}</span></button></nav>
        <button type="button" class="cst-theme-cycle" :aria-label="`切换外观（当前：${themeMeta.label}）`" :title="`切换外观（当前：${themeMeta.label}）`" @click="cycleTheme"><Icon :name="themeMeta.icon"/><span>{{themeMeta.label}}</span></button>
      </div>
      </div>
      <div class="cst-status-strip">
        <div class="cst-connection" role="status" :class="connection.tone" :title="connection.label" :aria-label="connection.label"><span class="cst-dot"/><small>{{connection.label}}</small></div>
        <div v-if="workspaceReady" class="cst-task-tools">
        <button v-if="task" class="cst-task" type="button" :class="task.state" :aria-label="'检测任务：'+task.title+(task.detail?'，'+task.detail:'')+(task.attention?'，'+task.attention:'')" @click="w.openQueue()">
          <span class="cst-task-title"><span v-if="task.state==='running'||task.state==='pending'" class="spinner"/>{{task.title}}</span>
          <span v-if="task.detail" class="cst-task-detail">{{task.detail}}</span>
          <span v-if="task.progress!==null" class="cst-task-bar" aria-hidden="true"><i :style="{width:Math.round(task.progress*100)+'%'}"/></span>
          <span v-if="task.attention" class="cst-task-attention">{{task.attention}}</span>
          <Icon name="arrowRight"/>
        </button>
        <button v-if="task?.canCancel" class="cst-task-cancel" type="button" @click="w.cancelTests()">{{w.cancelRequested.value?'正在取消…':'取消检测'}}</button>
        <button v-if="recentReport" class="cst-recent-report" :class="{'with-task':task}" type="button" :aria-label="`查看最近延迟报告，${date(recentReport.requested_at)}，${recentReportLabel}`" @click="w.openLatencyReport(recentReport)">
          <Icon name="history"/><span>延迟报告</span><small>{{date(recentReport.requested_at)}}</small>
        </button>
        </div>
      </div>
    </header>
    <main class="cst-main workspace-body">
      <section v-if="!accessReady" class="cst-startup"><p class="cst-startup-caption">访问工作台</p><h1>验证后继续使用</h1><p>输入此服务的访问密码，继续查看机场与检测记录。</p></section>
      <section v-else-if="!w.booted.value||!w.setup.value||w.setup.value.state==='ready'&&!w.nodesReady.value" class="cst-startup" :aria-busy="startupBusy">
        <p class="cst-startup-caption"><span v-if="startupBusy" class="spinner"/>{{startupBusy?'打开工作台':'连接中断'}}</p>
        <h1>{{startupBusy?(w.setup.value?'正在读取机场与记录…':'正在连接本地服务…'):'暂时无法打开工作台'}}</h1>
        <p v-if="startupBusy" role="status">读取完成后显示你的线路、账户和最近检测。</p>
        <template v-else><p>暂时未能读全工作台数据。确认本地服务仍在运行后，可以重新连接。</p><p class="cst-startup-error" role="alert">{{w.error.value}}</p><button class="button primary" @click="w.boot()"><Icon name="refresh"/>重试连接</button></template>
      </section>
      <template v-else>
        <div v-if="w.error.value" class="app-error" role="alert"><span>{{w.error.value}}</span><div class="button-group"><button class="text-button" :disabled="w.loading.value" @click="w.boot()">{{w.loading.value?'正在读取…':'重新读取'}}</button><button class="button ghost icon-button" aria-label="关闭错误提示" @click="w.error.value=''"><Icon name="close"/></button></div></div>
        <HomeView v-if="w.setup.value?.state==='ready'" v-show="w.page.value==='home'"/>
        <AirportsView v-if="w.page.value==='airports'&&w.setup.value?.state==='ready'"/>
        <MonitorView v-if="w.page.value==='monitor'&&w.setup.value?.state==='ready'"/>
        <RecommendationView v-if="w.page.value==='recommend'&&w.setup.value?.state==='ready'"/>
        <HistoryView v-if="w.page.value==='history'&&w.setup.value?.state==='ready'"/>
        <SettingsView v-if="w.page.value==='settings'&&w.setup.value?.state==='ready'"/>
        <DataView v-if="w.page.value==='data'||w.setup.value?.state!=='ready'"/>
      </template>
    </main>
    <Transition name="toast"><div v-if="w.toast.value" class="toast-message" role="status"><Icon name="check"/>{{w.toast.value}}</div></Transition>
    <template v-if="workspaceReady">
    <CommandPalette v-if="palette" :pages="palettePages" @close="palette=false" @navigate="navigate"/>
    <TestPlanModal v-if="w.plan.value"/>
    <MonitorCreateModal v-if="w.monitorNodes.value"/>
    <QueueModal v-if="w.queueOpen.value"/>
    <AutoPickReportModal v-if="w.autoPickReportId.value" :key="w.autoPickReportId.value" :report-id="w.autoPickReportId.value" @close="w.autoPickReportId.value=''"/>
    <NodeDetailModal v-if="w.inspectNode.value" :key="w.inspectNode.value.profile_id+w.inspectNode.value.node_key" :node="w.inspectNode.value" :initial-service-id="w.inspectNode.value.historyServiceId" :initial-target-id="w.inspectNode.value.historyTargetId" :initial-project="w.inspectNode.value.historyProject" :initial-source="w.inspectNode.value.historySource" :initial-until="w.inspectNode.value.historyUntil" @close="w.inspectNode.value=null"/>
    </template>
    <Modal v-if="w.authRequired.value||loggingIn" title="访问此工作台" :closable="false"><form @submit.prevent="login"><p class="muted">输入此服务的访问密码。</p><label class="form-stack">访问密码<input v-model="password" type="password" :disabled="loggingIn" autofocus autocomplete="current-password" required></label><p v-if="loginError" class="inline-error" role="alert">{{loginError}}</p><p v-if="loggingIn" role="status">正在验证并读取工作台…</p><div class="form-actions"><button class="button primary" :disabled="loggingIn" type="submit">{{loggingIn?'正在登录…':'登录'}}</button></div></form></Modal>
    <HoverTip/>
  </div>
</template>
