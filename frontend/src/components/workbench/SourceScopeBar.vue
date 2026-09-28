<script setup lang="ts">
import { computed } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'
import AppIcon from '../common/AppIcon.vue'

/**
 * `activeView` is owned by App.vue and switched here so the entry point sits in the same
 * header the user already uses, instead of hiding the timeline behind a modal.
 */
defineProps<{ activeView: 'workbench' | 'airports' | 'monitor-jobs' | 'timeline' }>()
const emit = defineEmits<{ (e: 'update:activeView', value: 'workbench' | 'airports' | 'monitor-jobs' | 'timeline'): void }>()

const store = useWorkbenchStore()

const isTokenValid = computed(() => store.tokenStatus.has_token)

async function handleLogin() {
  await api.startOAuthLogin()
}
</script>

<template>
  <header class="prototype-topbar">
    <div class="prototype-brand">
      <strong><i aria-hidden="true"><AppIcon name="pulse" /></i> SpeedTest</strong>
    </div>

    <nav class="prototype-nav" aria-label="主导航">
      <button type="button" :aria-current="activeView === 'workbench' ? 'page' : undefined" @click="emit('update:activeView', 'workbench')"><AppIcon name="dashboard" /><span>节点工作台</span></button>
      <button type="button" :aria-current="activeView === 'airports' ? 'page' : undefined" @click="emit('update:activeView', 'airports')"><AppIcon name="subscription" /><span>机场订阅</span></button>
      <button type="button" :aria-current="activeView === 'monitor-jobs' ? 'page' : undefined" @click="emit('update:activeView', 'monitor-jobs')"><AppIcon name="monitor" /><span>持续监测</span></button>
      <button type="button" :aria-current="activeView === 'timeline' ? 'page' : undefined" @click="emit('update:activeView', 'timeline')"><AppIcon name="history" /><span>历史记录</span></button>
    </nav>

    <div class="prototype-tools">
      <button
        type="button"
        class="tool-button data-root-button"
        :title="store.profileSetup?.data_root || '查看数据根与初始化状态'"
        aria-label="数据与备份"
        @click="store.isProfileSetupOpen = true"
      >
        <AppIcon name="backup" />
      </button>
      <span v-if="isTokenValid" class="data-status" :title="'已绑定凭据（' + store.tokenStatus.source + '）'" aria-label="凭据有效">●</span>
      <button v-else type="button" class="tool-button" @click="handleLogin">绑定凭据</button>
      <button type="button" class="tool-button icon-button" title="设置" aria-label="打开设置" @click="store.isSettingsModalOpen = true"><AppIcon name="settings" /></button>
    </div>
  </header>
</template>

<style scoped>
.prototype-topbar{display:grid;grid-template-columns:1fr auto 1fr;align-items:center;gap:24px;min-height:88px;padding:16px 32px;background:transparent}
.prototype-brand{flex:none}
.prototype-brand strong{display:inline-flex;align-items:center;gap:9px;font-size:17px;letter-spacing:-.4px;padding:9px 13px;background:var(--card-bg);border:1px solid var(--border);border-radius:12px;box-shadow:var(--shadow-sm)}
.prototype-brand i{display:grid;place-items:center;width:25px;height:25px;color:var(--primary);font-style:normal}
.prototype-nav{display:flex;align-items:center;gap:4px;padding:5px;background:var(--card-bg);border:1px solid var(--border);border-radius:28px;box-shadow:var(--shadow-nav)}
.prototype-nav button{position:relative;display:flex;align-items:center;gap:8px;padding:9px 15px;white-space:nowrap;color:var(--text-secondary);font-size:13px;border:1px solid transparent;border-radius:22px;background:transparent}
.prototype-nav button i{font-style:normal;font-size:16px}
.prototype-nav button:hover{color:var(--text-main);background:var(--card-subtle)}
.prototype-nav button[aria-current=page]{color:var(--primary);font-weight:650;background:var(--primary-subtle);border-color:var(--primary)}
.prototype-tools{display:flex;align-items:center;justify-content:flex-end;gap:10px}
.tool-button{display:flex;align-items:center;justify-content:center;white-space:nowrap;min-width:38px;height:38px;padding:8px;border:1px solid var(--border);border-radius:50%;background:var(--card-bg);color:var(--text-secondary);font-size:12px;box-shadow:var(--shadow-sm)}
.tool-button:hover{color:var(--primary);border-color:var(--primary)}
.data-status{font-size:11px;color:var(--success);white-space:nowrap}
@media(max-width:1050px){
.prototype-topbar{grid-template-columns:1fr auto;gap:16px;padding:18px}
.prototype-nav{grid-column:1/-1;grid-row:2;justify-self:center;max-width:100%;min-height:44px;overflow-x:auto}
.prototype-nav button{flex:1;justify-content:center;padding:0 12px}
}
@media(max-width:560px){
.prototype-topbar{gap:8px;padding:10px 12px 0}
.prototype-tools{gap:5px}
.prototype-brand strong{font-size:15px}
.prototype-brand i{width:25px;height:25px}
.data-status{display:none}
.tool-button{font-size:11px;padding:7px}
.prototype-nav{gap:0}
.prototype-nav button{font-size:12px;padding:0 8px}
.prototype-nav button i{display:none}
}
</style>
