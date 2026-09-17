<script setup lang="ts">
import { computed, ref } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'

const store = useWorkbenchStore()
const isExporting = ref(false)
const exportSuccess = ref(false)

const currentDisplayCount = computed(() => store.filteredResults.length)
const totalBatchCount = computed(() => Object.keys(store.resultsMap).length)

async function handleExportClash() {
  if (!store.selectedAirportId) return
  isExporting.value = true
  try {
    const nodeNames = store.filteredResults.map((r) => r.proxy_name)
    const yaml = await api.exportClashYAML(store.selectedAirportId, nodeNames)

    // Trigger download in browser or save
    const blob = new Blob([yaml], { type: 'application/x-yaml' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `clash-nodes-${new Date().toISOString().slice(0, 10)}.yaml`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)

    exportSuccess.value = true
    setTimeout(() => {
      exportSuccess.value = false
    }, 2000)
  } catch (e) {
    console.error('Export Clash YAML failed:', e)
  } finally {
    isExporting.value = false
  }
}
</script>

<template>
  <footer class="bg-card border-t border-border px-6 py-3 flex items-center justify-between gap-4 text-xs select-none">
    <!-- Left: Triage Staging Summary -->
    <div class="flex items-center gap-3">
      <span class="text-content-secondary">
        当前展示: <strong class="text-content-main font-mono">{{ currentDisplayCount }}</strong> / {{ totalBatchCount }} 个测试结果
      </span>
      <span v-if="store.activeTriageFilter !== 'all'" class="text-content-muted text-[11px]">
        (已启用 {{ store.activeTriageFilter }} 分诊过滤)
      </span>
    </div>

    <!-- Right: Export & Staging Actions -->
    <div class="flex items-center gap-2">
      <span v-if="exportSuccess" class="text-emerald-400 font-medium flex items-center gap-1">
        <span>✔</span> 导出成功!
      </span>

      <button
        @click="handleExportClash"
        :disabled="isExporting || currentDisplayCount === 0"
        class="bg-card-subtle hover:bg-card text-content-main border border-border px-3 py-1.5 rounded-md font-medium transition-colors disabled:opacity-50 flex items-center gap-1.5"
      >
        <span>📄</span> 导出选中节点为 Clash 配置
      </button>
    </div>
  </footer>
</template>
