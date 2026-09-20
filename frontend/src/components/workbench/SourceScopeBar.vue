<script setup lang="ts">
import { computed } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'

/**
 * `activeView` is owned by App.vue and switched here so the entry point sits in the same
 * header the user already uses, instead of hiding the timeline behind a modal.
 */
defineProps<{ activeView: 'workbench' | 'monitor-jobs' | 'timeline' }>()
const emit = defineEmits<{ (e: 'update:activeView', value: 'workbench' | 'monitor-jobs' | 'timeline'): void }>()

const store = useWorkbenchStore()

const isTokenValid = computed(() => store.tokenStatus.has_token)

async function handleLogin() {
  await api.startOAuthLogin()
}
</script>

<template>
  <header class="prototype-topbar">
    <div class="prototype-brand">
      <strong>Clash SpeedTest</strong>
      <span>节点事实与持续观察</span>
    </div>

    <nav class="prototype-nav" aria-label="主导航">
      <button type="button" :aria-current="activeView === 'workbench' ? 'page' : undefined" @click="emit('update:activeView', 'workbench')">节点工作台</button>
      <button type="button" :aria-current="activeView === 'monitor-jobs' ? 'page' : undefined" @click="emit('update:activeView', 'monitor-jobs')">持续监测</button>
      <button type="button" :aria-current="activeView === 'timeline' ? 'page' : undefined" @click="emit('update:activeView', 'timeline')">历史记录</button>
    </nav>

    <div class="prototype-tools">
      <button type="button" class="tool-button" @click="store.isAirportModalOpen = true">管理订阅</button>
      <span v-if="isTokenValid" class="data-status" :title="'已绑定凭据（' + store.tokenStatus.source + '）'">● 凭据有效</span>
      <button v-else type="button" class="tool-button" @click="handleLogin">绑定凭据</button>
      <button type="button" class="tool-button" @click="store.isHistoryModalOpen = true">旧历史矩阵</button>
      <button type="button" class="tool-button icon-button" aria-label="打开设置" @click="store.isSettingsModalOpen = true">设置</button>
    </div>
  </header>
</template>

<style scoped>
.prototype-topbar { display: flex; align-items: stretch; gap: 28px; min-height: 66px; padding: 0 34px; background: var(--card-bg); border-bottom: 1px solid var(--border); }
.prototype-brand { display: flex; flex: 0 0 206px; flex-direction: column; justify-content: center; gap: 2px; }
.prototype-brand strong { font-size: 16px; letter-spacing: .02em; }
.prototype-brand span { color: var(--text-secondary); font-size: 11px; }
.prototype-nav { display: flex; align-items: stretch; gap: 18px; }
.prototype-nav button { padding: 0 2px; border: 0; border-bottom: 3px solid transparent; background: transparent; color: var(--text-secondary); font-size: 13px; }
.prototype-nav button[aria-current="page"] { border-bottom-color: var(--primary); color: var(--text-main); font-weight: 700; }
.prototype-tools { display: flex; align-items: center; gap: 8px; margin-left: auto; }
.tool-button { min-height: 31px; padding: 5px 9px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); color: var(--text-secondary); font-size: 11px; }
.tool-button:hover { border-color: var(--border-subtle); color: var(--primary); }
.icon-button { min-width: 42px; }
.data-status { color: var(--success); font-size: 11px; font-weight: 700; }
@media (max-width: 860px) {
  .prototype-topbar { flex-wrap: wrap; gap: 0 20px; padding: 14px 20px 0; }
  .prototype-brand { flex-basis: 170px; padding-bottom: 13px; }
  .prototype-tools { margin-left: auto; padding-bottom: 13px; }
  .prototype-nav { order: 3; width: 100%; height: 41px; }
  .prototype-nav button { flex: 1; }
}
@media (max-width: 560px) {
  .prototype-topbar { padding-left: 14px; padding-right: 14px; }
  .prototype-brand { flex-basis: 150px; }
  .prototype-tools .tool-button:nth-child(1), .prototype-tools .tool-button:nth-child(3) { display: none; }
}
</style>
