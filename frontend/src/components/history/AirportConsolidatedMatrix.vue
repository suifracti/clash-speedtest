<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'
import type { RunSummary, RunComparison } from '../../types'

const store = useWorkbenchStore()

const runs = ref<RunSummary[]>([])
const selectedRunId = ref<string>('')
const compareBaseId = ref<string>('')
const compareTargetId = ref<string>('')
const comparisonResult = ref<RunComparison | null>(null)
const isComparing = ref(false)

onMounted(async () => {
  await loadRuns()
})

async function loadRuns() {
  try {
    runs.value = await api.fetchHistory()
    if (runs.value.length > 0 && !selectedRunId.value) {
      selectedRunId.value = runs.value[0].id
    }
    if (runs.value.length >= 2) {
      compareTargetId.value = runs.value[0].id
      compareBaseId.value = runs.value[1].id
    }
  } catch (e) {
    console.error('Failed to load history runs:', e)
  }
}

async function handleViewRun(runId: string) {
  try {
    const run = await api.fetchHistoryRun(runId)
    store.selectedRun = run
    // Populate results into workbench for review
    store.resultsMap = {}
    for (const r of run.results) {
      store.resultsMap[r.proxy_name] = r
    }
    store.isHistoryModalOpen = false
  } catch (e) {
    console.error('Failed to view run:', e)
  }
}

async function handleDeleteRun(runId: string) {
  if (confirm('确定删除该测试记录吗？')) {
    try {
      await api.deleteHistoryRun(runId)
      await loadRuns()
    } catch (e) {
      console.error('Failed to delete run:', e)
    }
  }
}

async function handleCompare() {
  if (!compareBaseId.value || !compareTargetId.value) return
  isComparing.value = true
  try {
    comparisonResult.value = await api.compareRuns(compareBaseId.value, compareTargetId.value)
  } catch (e) {
    console.error('Failed to compare runs:', e)
  } finally {
    isComparing.value = false
  }
}

function openOfflineReport() {
  window.open('/api/report/html', '_blank')
}
</script>

<template>
  <div
    v-if="store.isHistoryModalOpen"
    class="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none"
  >
    <div class="bg-card border border-border rounded-xl shadow-2xl w-full max-w-4xl max-h-[85vh] flex flex-col overflow-hidden text-xs">
      <!-- Modal Header -->
      <div class="p-4 border-b border-border flex items-center justify-between">
        <h2 class="text-sm font-bold text-content-main flex items-center gap-2">
          <span>📊</span> 历史测速归档与批次对比
        </h2>
        <div class="flex items-center gap-2">
          <button
            @click="openOfflineReport"
            class="px-2.5 py-1 rounded bg-card-subtle hover:bg-card border border-border text-content-secondary"
            title="在独立标签页打开静态 HTML 报告"
          >
            打开 HTML 完整报告
          </button>
          <button @click="store.isHistoryModalOpen = false" class="text-content-muted hover:text-content-main text-lg font-mono">
            ✕
          </button>
        </div>
      </div>

      <!-- Modal Body -->
      <div class="p-4 flex-1 overflow-y-auto flex flex-col gap-6">
        <!-- Runs List -->
        <div class="flex flex-col gap-2">
          <span class="font-semibold text-content-secondary text-[11px] uppercase tracking-wider">
            历史批次记录 ({{ runs.length }})
          </span>

          <div class="border border-border rounded-lg overflow-hidden divide-y divide-border">
            <div
              v-for="run in runs"
              :key="run.id"
              class="p-3 bg-card hover:bg-card-hover flex items-center justify-between gap-4 transition-colors"
            >
              <div class="flex flex-col gap-0.5">
                <div class="flex items-center gap-2">
                  <span class="font-bold text-content-main">{{ run.airport_name }}</span>
                  <span class="text-content-muted font-mono text-[11px]">{{ new Date(run.created_at).toLocaleString() }}</span>
                </div>
                <div class="flex items-center gap-2 text-content-secondary font-mono text-[11px]">
                  <span>通过: {{ run.passed_nodes }} / {{ run.total_nodes }}</span>
                  <span>•</span>
                  <span>指标: {{ run.metrics.join(', ') }}</span>
                </div>
              </div>

              <!-- Actions -->
              <div class="flex items-center gap-2">
                <button
                  @click="handleViewRun(run.id)"
                  class="px-3 py-1 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium"
                >
                  载入查看
                </button>
                <button
                  @click="handleDeleteRun(run.id)"
                  class="px-2 py-1 rounded border border-red-500/30 text-red-400 hover:bg-red-500/10"
                >
                  删除
                </button>
              </div>
            </div>

            <div v-if="runs.length === 0" class="p-8 text-center text-content-muted">
              暂无历史测速记录
            </div>
          </div>
        </div>

        <!-- Run Diff Comparator Section -->
        <div v-if="runs.length >= 2" class="flex flex-col gap-3 pt-4 border-t border-border">
          <span class="font-semibold text-content-secondary text-[11px] uppercase tracking-wider">
            批次差异对比 (Run Diff)
          </span>

          <div class="bg-card-subtle p-3 rounded-lg border border-border flex items-center gap-3">
            <div class="flex items-center gap-2">
              <span class="text-content-muted">基准记录:</span>
              <select
                v-model="compareBaseId"
                class="bg-card text-content-main border border-border rounded px-2 py-1 focus:outline-none focus:border-brand"
              >
                <option v-for="r in runs" :key="r.id" :value="r.id">
                  {{ r.airport_name }} - {{ new Date(r.created_at).toLocaleTimeString() }}
                </option>
              </select>
            </div>

            <span class="text-content-muted">VS</span>

            <div class="flex items-center gap-2">
              <span class="text-content-muted">对比记录:</span>
              <select
                v-model="compareTargetId"
                class="bg-card text-content-main border border-border rounded px-2 py-1 focus:outline-none focus:border-brand"
              >
                <option v-for="r in runs" :key="r.id" :value="r.id">
                  {{ r.airport_name }} - {{ new Date(r.created_at).toLocaleTimeString() }}
                </option>
              </select>
            </div>

            <button
              @click="handleCompare"
              :disabled="isComparing"
              class="px-3 py-1 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium disabled:opacity-50 ml-auto"
            >
              {{ isComparing ? '计算中...' : '生成 Diff' }}
            </button>
          </div>

          <!-- Comparison Result Table -->
          <div v-if="comparisonResult" class="flex flex-col gap-2">
            <div class="flex items-center gap-4 text-xs font-medium">
              <span class="text-emerald-400">改善: {{ comparisonResult.summary.improved_count }}</span>
              <span class="text-red-400">劣化: {{ comparisonResult.summary.degraded_count }}</span>
              <span class="text-content-muted">未变化: {{ comparisonResult.summary.unchanged_count }}</span>
              <span class="text-blue-400">新增: {{ comparisonResult.summary.new_count }}</span>
              <span class="text-zinc-500">移除: {{ comparisonResult.summary.removed_count }}</span>
            </div>

            <div class="border border-border rounded-lg overflow-x-auto max-h-60">
              <table class="w-full text-left text-xs">
                <thead class="bg-card sticky top-0 border-b border-border">
                  <tr>
                    <th class="p-2">节点</th>
                    <th class="p-2">延迟变化</th>
                    <th class="p-2">速度变化</th>
                    <th class="p-2">状态</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border">
                  <tr v-for="d in comparisonResult.node_diffs" :key="d.proxy_name">
                    <td class="p-2 font-mono">{{ d.proxy_name }}</td>
                    <td class="p-2 font-mono">
                      {{ d.base_latency_ms }}ms → {{ d.target_latency_ms }}ms
                      <span :class="d.latency_delta_ms < 0 ? 'text-emerald-400' : 'text-red-400'">
                        ({{ d.latency_delta_ms }}ms)
                      </span>
                    </td>
                    <td class="p-2 font-mono">
                      {{ d.base_speed_mbps.toFixed(1) }} → {{ d.target_speed_mbps.toFixed(1) }} MB/s
                    </td>
                    <td class="p-2">
                      <span
                        class="px-1.5 py-0.5 rounded text-[10px]"
                        :class="d.status === 'improved' ? 'bg-emerald-500/20 text-emerald-400' : d.status === 'degraded' ? 'bg-red-500/20 text-red-400' : 'bg-card-subtle text-content-muted'"
                      >
                        {{ d.status }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
