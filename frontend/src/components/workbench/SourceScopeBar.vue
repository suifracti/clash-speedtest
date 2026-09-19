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

async function onAirportChange(e: Event) {
  const target = e.target as HTMLSelectElement
  store.selectedAirportId = target.value
  await store.loadAirportNodes(target.value)
}

function toggleMetric(metric: string) {
  const idx = store.testConfig.metrics.indexOf(metric)
  if (idx >= 0) {
    if (store.testConfig.metrics.length > 1) {
      store.testConfig.metrics.splice(idx, 1)
    }
  } else {
    store.testConfig.metrics.push(metric)
  }
}

async function handleLogin() {
  await api.startOAuthLogin()
}
</script>

<template>
  <header class="bg-card border-b border-border px-6 py-3.5 flex flex-wrap items-center justify-between gap-4 select-none">
    <!-- Left: Brand & Source Selection -->
    <div class="flex items-center gap-4">
      <div class="flex items-center gap-2.5">
        <div class="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center text-white font-bold text-sm shadow-sm">
          ⚡
        </div>
        <div>
          <h1 class="text-sm font-semibold tracking-tight leading-tight">Clash SpeedTest Pro</h1>
          <p class="text-xs text-content-muted">节点测速与分诊决策工作台</p>
        </div>
      </div>

      <div class="h-6 w-px bg-border mx-1"></div>

      <!-- Airport Picker -->
      <div class="flex items-center gap-2">
        <label class="text-xs font-medium text-content-secondary">测速源:</label>
        <select
          :value="store.selectedAirportId"
          @change="onAirportChange"
          class="bg-card-subtle text-content-main text-xs rounded-md border border-border px-3 py-1.5 focus:outline-none focus:border-brand font-medium"
        >
          <option v-for="ap in store.airports" :key="ap.id" :value="ap.id">
            {{ ap.name }} ({{ ap.node_count }} 节点)
          </option>
        </select>
        <button
          @click="store.isAirportModalOpen = true"
          class="text-xs text-content-secondary hover:text-brand px-2 py-1 rounded border border-border hover:border-brand transition-colors"
          title="管理订阅与机场源"
        >
          管理机场
        </button>
      </div>
    </div>

    <!-- Center: Metrics Toggles -->
    <div class="flex items-center gap-1.5 bg-card-subtle p-1 rounded-lg border border-border text-xs">
      <button
        @click="toggleMetric('latency')"
        :class="store.testConfig.metrics.includes('latency') ? 'bg-card text-brand shadow-sm font-medium' : 'text-content-secondary hover:text-content-main'"
        class="px-2.5 py-1 rounded transition-all flex items-center gap-1"
      >
        <span>📶</span> 延迟
      </button>
      <button
        @click="toggleMetric('download')"
        :class="store.testConfig.metrics.includes('download') ? 'bg-card text-brand shadow-sm font-medium' : 'text-content-secondary hover:text-content-main'"
        class="px-2.5 py-1 rounded transition-all flex items-center gap-1"
      >
        <span>📥</span> 下载吞吐
      </button>
      <button
        @click="toggleMetric('antigravity')"
        :class="store.testConfig.metrics.includes('antigravity') ? 'bg-card text-brand shadow-sm font-medium' : 'text-content-secondary hover:text-content-main'"
        class="px-2.5 py-1 rounded transition-all flex items-center gap-1"
      >
        <span>🌐</span> Antigravity
      </button>
    </div>

    <!-- Right: View Switch, Token & Trigger Actions -->
    <div class="flex items-center gap-3">
      <!-- View switch: Workbench (batch triage) vs Monitor Timeline (raw samples) -->
      <div class="flex items-center gap-1 bg-card-subtle p-1 rounded-lg border border-border text-xs">
        <button
          @click="emit('update:activeView', 'workbench')"
          :class="
            activeView === 'workbench'
              ? 'bg-card text-brand shadow-sm font-medium'
              : 'text-content-secondary hover:text-content-main'
          "
          class="px-2.5 py-1 rounded transition-all flex items-center gap-1"
          title="节点测速与分诊工作台"
        >
          <span>🧪</span> 工作台
        </button>
        <button
          @click="emit('update:activeView', 'monitor-jobs')"
          :class="
            activeView === 'monitor-jobs'
              ? 'bg-card text-brand shadow-sm font-medium'
              : 'text-content-secondary hover:text-content-main'
          "
          class="px-2.5 py-1 rounded transition-all flex items-center gap-1"
          title="创建并管理后台监控任务"
        >
          <span>🛰️</span> 监控任务
        </button>
        <button
          @click="emit('update:activeView', 'timeline')"
          :class="
            activeView === 'timeline'
              ? 'bg-card text-brand shadow-sm font-medium'
              : 'text-content-secondary hover:text-content-main'
          "
          class="px-2.5 py-1 rounded transition-all flex items-center gap-1"
          title="监控稳定性时间轴：逐条原始样本，可缩放与点选"
        >
          <span>📈</span> 稳定性时间轴
        </button>
      </div>

      <!-- Token Status Indicator -->
      <div class="flex items-center gap-2">
        <span
          v-if="isTokenValid"
          class="inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/25"
          :title="'已绑定凭据 (' + store.tokenStatus.source + ')'"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
          凭据有效
        </span>
        <button
          v-else
          @click="handleLogin"
          class="text-xs px-2.5 py-1 rounded-full bg-amber-500/10 text-amber-400 border border-amber-500/25 hover:bg-amber-500/20 transition-colors flex items-center gap-1"
        >
          <span>🔑</span> 绑定凭据
        </button>
      </div>

      <!-- History button -->
      <button
        @click="store.isHistoryModalOpen = true"
        class="text-xs text-content-secondary hover:text-content-main px-3 py-1.5 rounded-md border border-border hover:border-border-subtle bg-card transition-colors flex items-center gap-1.5"
      >
        <span>📊</span> 历史矩阵
      </button>

      <!-- Settings button -->
      <button
        @click="store.isSettingsModalOpen = true"
        class="text-xs text-content-secondary hover:text-content-main p-1.5 rounded-md border border-border hover:border-border-subtle bg-card transition-colors"
        title="设置与首选项"
      >
        ⚙️
      </button>

      <!-- Start/Stop Primary Button -->
      <button
        v-if="!store.testStatus.is_running"
        @click="store.startBatch()"
        class="bg-blue-600 hover:bg-blue-500 active:bg-blue-700 text-white text-xs font-semibold px-4 py-2 rounded-lg shadow-sm transition-all flex items-center gap-1.5"
      >
        <span>🚀</span> 开始测试
      </button>
      <button
        v-else
        @click="store.stop()"
        class="bg-red-600 hover:bg-red-500 active:bg-red-700 text-white text-xs font-semibold px-4 py-2 rounded-lg shadow-sm transition-all flex items-center gap-1.5 animate-pulse"
      >
        <span>⏹</span> 停止测速
      </button>
    </div>
  </header>
</template>
