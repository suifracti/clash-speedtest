<script setup lang="ts">
import {onMounted,onBeforeUnmount,provide,ref} from 'vue'
import {api} from './api'
import {createWorkspace,workspaceKey} from './workspace'
import type {Page} from './domain'
import Icon from './components/Icon.vue'
import Modal from './components/Modal.vue'
import HomeView from './components/HomeView.vue'
import AirportsView from './components/AirportsView.vue'
import MonitorView from './components/MonitorView.vue'
import HistoryView from './components/HistoryView.vue'
import SettingsView from './components/SettingsView.vue'
import DataView from './components/DataView.vue'
import TestPlanModal from './components/TestPlanModal.vue'
import MonitorCreateModal from './components/MonitorCreateModal.vue'
import NodeDetailModal from './components/NodeDetailModal.vue'
import QueueModal from './components/QueueModal.vue'
const w=createWorkspace();provide(workspaceKey,w)
const password=ref(''),loginError=ref(''),loggingIn=ref(false)
const pages:{id:Page;label:string;icon:string}[]=[{id:'home',label:'节点首页',icon:'home'},{id:'airports',label:'机场订阅',icon:'layers'},{id:'monitor',label:'持续监测',icon:'monitor'},{id:'history',label:'检测历史',icon:'history'}]
let timer:ReturnType<typeof setInterval>|undefined
function navigate(page:Page){w.page.value=page;window.scrollTo({top:0,behavior:'instant'})}
function authExpired(){w.authRequired.value=true;w.dispose()}
async function login(){loggingIn.value=true;loginError.value='';try{await api.login(password.value);password.value='';w.authRequired.value=false;await w.boot()}catch(e){loginError.value=e instanceof Error?e.message:String(e)}finally{loggingIn.value=false}}
onMounted(()=>{void w.boot();window.addEventListener('speedtest:auth-required',authExpired);timer=setInterval(()=>{if(w.busy.value&&!w.running.value&&document.visibilityState==='visible')void w.syncActive()},2500)})
onBeforeUnmount(()=>{w.dispose();clearInterval(timer);window.removeEventListener('speedtest:auth-required',authExpired)})
</script>
<template>
  <div class="app-shell">
    <div class="app-content">
      <header class="workspace-header">
        <div class="workspace-header-inner">
          <a class="brand" href="#" @click.prevent="navigate('home')">
            <span class="brand-mark"><Icon name="pulse"/></span>
            <div class="brand-copy"><strong>SpeedTest</strong><span>网络检测工作台</span></div>
          </a>
          <nav class="workspace-nav" aria-label="主导航">
            <button v-for="p in pages" :key="p.id" type="button" :aria-current="w.page.value===p.id?'page':undefined" @click="navigate(p.id)">
              <Icon :name="p.icon"/><span>{{p.label}}</span>
              <small v-if="p.id==='monitor'&&w.jobs.value.some(j=>j.state==='running')">{{w.jobs.value.filter(j=>j.state==='running').length}}</small>
            </button>
          </nav>
          <div class="workspace-tools">
            <nav class="workspace-utilities" aria-label="工具">
              <button type="button" aria-label="偏好设置" title="偏好设置" :aria-current="w.page.value==='settings'?'page':undefined" @click="navigate('settings')"><Icon name="settings"/><span>偏好设置</span></button>
              <button type="button" aria-label="数据与备份" title="数据与备份" :aria-current="w.page.value==='data'?'page':undefined" @click="navigate('data')"><Icon name="data"/><span>数据与备份</span></button>
            </nav>
            <div class="connection-state" role="status" :title="w.connected.value?'实时连接正常':w.loading.value?'正在连接…':'实时连接断开'" :aria-label="w.connected.value?'实时连接正常':w.loading.value?'正在连接…':'实时连接断开'"><span class="status-dot" :class="{connected:w.connected.value}"/></div>
          </div>
        </div>
      </header>
      <main class="workspace-body">
        <div v-if="w.error.value" class="app-error" role="alert"><span>{{w.error.value}}</span><div class="button-group"><button class="text-button" @click="w.boot()">重新读取</button><button class="button ghost icon-button" aria-label="关闭错误提示" @click="w.error.value=''"><Icon name="close"/></button></div></div>
        <div v-if="w.loading.value&&!w.setup.value" class="app-loading"><span class="spinner"/><p>正在读取你的机场与记录…</p></div>
        <template v-else-if="!w.authRequired.value">
          <HomeView v-if="w.setup.value?.state==='ready'" v-show="w.page.value==='home'"/>
          <AirportsView v-if="w.page.value==='airports'&&w.setup.value?.state==='ready'"/>
          <MonitorView v-if="w.page.value==='monitor'&&w.setup.value?.state==='ready'"/>
          <HistoryView v-if="w.page.value==='history'&&w.setup.value?.state==='ready'"/>
          <SettingsView v-if="w.page.value==='settings'&&w.setup.value?.state==='ready'"/>
          <DataView v-if="w.page.value==='data'||w.setup.value?.state!=='ready'"/>
        </template>
        <button v-if="w.busy.value&&w.page.value!=='home'" class="running-pill" @click="w.queueOpen.value=true"><span class="spinner"/>检测进行中 · 查看进度</button>
      </main>
    </div>
    <Transition name="toast"><div v-if="w.toast.value" class="toast-message" role="status"><Icon name="check"/>{{w.toast.value}}</div></Transition>
    <TestPlanModal v-if="w.plan.value"/>
    <MonitorCreateModal v-if="w.monitorNodes.value"/>
    <QueueModal v-if="w.queueOpen.value"/>
    <NodeDetailModal v-if="w.inspectNode.value" :key="w.inspectNode.value.profile_id+w.inspectNode.value.node_key" :node="w.inspectNode.value" @close="w.inspectNode.value=null"/>
    <Modal v-if="w.authRequired.value" title="访问此工作台" :closable="false"><form @submit.prevent="login"><p class="muted">输入此服务的访问密码。</p><label class="form-stack">访问密码<input v-model="password" type="password" autofocus autocomplete="current-password" required></label><p v-if="loginError" class="inline-error" role="alert">{{loginError}}</p><div class="form-actions"><button class="button primary" :disabled="loggingIn" type="submit">{{loggingIn?'正在登录…':'登录'}}</button></div></form></Modal>
  </div>
</template>
