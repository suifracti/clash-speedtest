<script setup lang="ts">
import {computed,onMounted,onBeforeUnmount,provide,ref} from 'vue'
import {api} from './api'
import {createWorkspace,workspaceKey} from './workspace'
import type {Page} from './domain'
import Icon from './components/Icon.vue'
import Modal from './components/Modal.vue'
import HomeView from './components/HomeView.vue'
import RecommendationView from './components/RecommendationView.vue'
import AirportsView from './components/AirportsView.vue'
import MonitorView from './components/MonitorView.vue'
import HistoryView from './components/HistoryView.vue'
import SettingsView from './components/SettingsView.vue'
import DataView from './components/DataView.vue'
import TestPlanModal from './components/TestPlanModal.vue'
import MonitorCreateModal from './components/MonitorCreateModal.vue'
import NodeDetailModal from './components/NodeDetailModal.vue'
import QueueModal from './components/QueueModal.vue'
import CommandPalette from './components/CommandPalette.vue'
import {projectNames} from './domain'
import AirportSidebar from './components/AirportSidebar.vue'
const w=createWorkspace();provide(workspaceKey,w)
const windowHidden=ref(document.hidden)
function visibilityChanged(){windowHidden.value=document.hidden}
const password=ref(''),loginError=ref(''),loggingIn=ref(false)
const pages:{id:Page;label:string;icon:string}[]=[{id:'home',label:'线路工作台',icon:'home'},{id:'recommend',label:'现在用哪个',icon:'pulse'},{id:'airports',label:'机场与账户',icon:'layers'},{id:'history',label:'检测记录',icon:'history'},{id:'monitor',label:'持续监测',icon:'monitor'}]
const utilities:{id:Page;label:string;icon:string}[]=[{id:'settings',label:'偏好设置',icon:'settings'},{id:'data',label:'数据与备份',icon:'data'}]
const palette=ref(false)
const runningJobs=computed(()=>w.jobs.value.filter(j=>j.state==='running').length)
// 侧栏任务区只回答三件事：在做什么、已处理多少、有没有需要注意的。不写“成功”，未确认启动时不画进度。
const task=computed(()=>{
 const rows=w.queue.value,total=rows.length,done=w.done.value
 const unsaved=rows.filter(q=>q.attempt?.persistence_state==='failed'||q.error?.startsWith('保存失败')).length
 const errors=rows.filter(q=>q.state==='failed'&&!q.error?.startsWith('保存失败')).length
 const kinds=[...new Set(rows.filter(q=>!['completed','failed','cancelled','not_executed','interrupted'].includes(q.state)).map(q=>projectNames[q.project]))].join('、')
 const attention=[unsaved?`${unsaved} 项未保存`:'',errors?`${errors} 项出错`:''].filter(Boolean).join(' · ')
 if(w.pendingStart.value&&!w.activeBatch.value&&!w.activeAttempt.value)return {state:'pending',title:'正在确认启动状态',detail:'等待后台回应，不会重复启动',progress:null,attention}
 if(w.busy.value)return {state:'running',title:kinds?`正在${kinds}`:'检测进行中',detail:total?`已处理 ${done}/${total}`:'',progress:total?done/total:null,attention}
 if(total)return {state:attention||done<total?'attention':'idle',title:done<total?'检测已停止，部分项目没有运行完':attention?'检测已结束，部分结果需注意':'上一轮检测已结束',detail:`已处理 ${done}/${total}`,progress:null,attention}
 return {state:'empty',title:'没有进行中的检测',detail:'',progress:null,attention:''}
})
const connection=computed(()=>w.connected.value?{tone:'good',label:'本地服务已连接'}:w.loading.value?{tone:'warn',label:'正在连接本地服务'}:{tone:'bad',label:'本地服务未连接'})
function paletteKey(e:KeyboardEvent){if((e.metaKey||e.ctrlKey)&&!e.altKey&&e.key.toLowerCase()==='k'&&w.setup.value?.state==='ready'&&!w.authRequired.value){e.preventDefault();palette.value=!palette.value}}
let timer:ReturnType<typeof setInterval>|undefined
function navigate(page:Page){w.page.value=page;window.scrollTo({top:0,behavior:'instant'})}
function authExpired(){w.authRequired.value=true;w.dispose()}
async function login(){loggingIn.value=true;loginError.value='';try{await api.login(password.value);password.value='';w.authRequired.value=false;await w.boot()}catch(e){loginError.value=e instanceof Error?e.message:String(e)}finally{loggingIn.value=false}}
onMounted(()=>{document.addEventListener('visibilitychange',visibilityChanged);window.addEventListener('keydown',paletteKey);void w.boot();window.addEventListener('speedtest:auth-required',authExpired);timer=setInterval(()=>{if(w.busy.value&&!w.running.value&&document.visibilityState==='visible')void w.syncActive()},2500)})
onBeforeUnmount(()=>{document.removeEventListener('visibilitychange',visibilityChanged);window.removeEventListener('keydown',paletteKey);w.dispose();clearInterval(timer);window.removeEventListener('speedtest:auth-required',authExpired)})
</script>
<template>
  <div class="cst-shell observatory-app" :class="{'window-hidden':windowHidden}">
    <aside class="cst-side" aria-label="侧栏">
      <a class="cst-brand" href="#" @click.prevent="navigate('home')"><span class="cst-brand-mark"><Icon name="pulse"/></span><span class="cst-brand-copy"><strong>CST<span class="brand-period">.</span></strong><small>线路观测台</small></span></a>
      <button v-if="w.setup.value?.state==='ready'" class="cst-jump" type="button" aria-keyshortcuts="Meta+K" title="快速跳转（⌘K）" @click="palette=true"><Icon name="search"/><span>快速跳转</span><kbd>⌘K</kbd></button>
      <nav class="cst-nav" aria-label="主导航">
        <button v-for="p in pages" :key="p.id" type="button" :title="p.label" :aria-current="w.page.value===p.id?'page':undefined" @click="navigate(p.id)">
          <Icon :name="p.icon"/><span>{{p.label}}</span>
          <small v-if="p.id==='monitor'&&runningJobs" :aria-label="runningJobs+' 个监测任务运行中'">{{runningJobs}}</small>
        </button>
      </nav>
      <AirportSidebar @select="id=>{w.viewAirport(id);navigate('home')}"/>
      <div class="cst-side-foot">
        <button class="cst-task" type="button" :class="task.state" :disabled="!w.queue.value.length&&!w.busy.value" :aria-label="'检测任务：'+task.title+(task.detail?'，'+task.detail:'')+(task.attention?'，'+task.attention:'')" @click="w.queueOpen.value=true">
          <span class="cst-task-title"><span v-if="task.state==='running'||task.state==='pending'" class="spinner"/>{{task.title}}</span>
          <span v-if="task.detail" class="cst-task-detail">{{task.detail}}</span>
          <span v-if="task.progress!==null" class="cst-task-bar"><i :style="{width:Math.round(task.progress*100)+'%'}"/></span>
          <span v-if="task.attention" class="cst-task-attention">{{task.attention}}</span>
        </button>
        <nav class="cst-utilities" aria-label="工具">
          <button v-for="u in utilities" :key="u.id" type="button" :aria-label="u.label" :title="u.label" :aria-current="w.page.value===u.id?'page':undefined" @click="navigate(u.id)"><Icon :name="u.icon"/><span>{{u.label}}</span></button>
        </nav>
        <div class="cst-connection" role="status" :class="connection.tone" :title="connection.label" :aria-label="connection.label"><span class="cst-dot"/><small>{{connection.label}}</small></div>
      </div>
    </aside>
    <main class="cst-main workspace-body">
      <div v-if="w.error.value" class="app-error" role="alert"><span>{{w.error.value}}</span><div class="button-group"><button class="text-button" @click="w.boot()">重新读取</button><button class="button ghost icon-button" aria-label="关闭错误提示" @click="w.error.value=''"><Icon name="close"/></button></div></div>
      <div v-if="w.loading.value&&!w.setup.value" class="app-loading"><span class="spinner"/><p>正在读取你的机场与记录…</p></div>
      <template v-else-if="!w.authRequired.value">
        <HomeView v-if="w.setup.value?.state==='ready'" v-show="w.page.value==='home'"/>
        <AirportsView v-if="w.page.value==='airports'&&w.setup.value?.state==='ready'"/>
        <MonitorView v-if="w.page.value==='monitor'&&w.setup.value?.state==='ready'"/>
        <RecommendationView v-if="w.page.value==='recommend'&&w.setup.value?.state==='ready'"/>
        <HistoryView v-if="w.page.value==='history'&&w.setup.value?.state==='ready'"/>
        <SettingsView v-if="w.page.value==='settings'&&w.setup.value?.state==='ready'"/>
        <DataView v-if="w.page.value==='data'||w.setup.value?.state!=='ready'"/>
      </template>
      <button v-if="w.busy.value" class="running-pill cst-mobile-task" @click="w.queueOpen.value=true"><span class="spinner"/>检测进行中 · 查看进度</button>
    </main>
    <Transition name="toast"><div v-if="w.toast.value" class="toast-message" role="status"><Icon name="check"/>{{w.toast.value}}</div></Transition>
    <CommandPalette v-if="palette" :pages="[...pages,...utilities]" @close="palette=false" @navigate="navigate"/>
    <TestPlanModal v-if="w.plan.value"/>
    <MonitorCreateModal v-if="w.monitorNodes.value"/>
    <QueueModal v-if="w.queueOpen.value"/>
    <NodeDetailModal v-if="w.inspectNode.value" :key="w.inspectNode.value.profile_id+w.inspectNode.value.node_key" :node="w.inspectNode.value" @close="w.inspectNode.value=null"/>
    <Modal v-if="w.authRequired.value" title="访问此工作台" :closable="false"><form @submit.prevent="login"><p class="muted">输入此服务的访问密码。</p><label class="form-stack">访问密码<input v-model="password" type="password" autofocus autocomplete="current-password" required></label><p v-if="loginError" class="inline-error" role="alert">{{loginError}}</p><div class="form-actions"><button class="button primary" :disabled="loggingIn" type="submit">{{loggingIn?'正在登录…':'登录'}}</button></div></form></Modal>
  </div>
</template>
