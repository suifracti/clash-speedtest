<script setup lang="ts">
import { computed } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import type { NodeResult } from '../../types'

const store = useWorkbenchStore()

const results = computed(() => store.filteredResults)

// Find max speed in current batch for percentile normalization
const maxSpeedInBatch = computed(() => {
  let max = 1
  for (const r of results.value) {
    if (r.download_speed_mbps > max) max = r.download_speed_mbps
  }
  return max
})

function getSpeedPercent(speed: number): number {
  if (!speed || maxSpeedInBatch.value <= 0) return 0
  return Math.min(100, Math.round((speed / maxSpeedInBatch.value) * 100))
}

function onRowHover(node: NodeResult) {
  store.focusedNode = node
}

async function handleRetest(nodeName: string) {
  await store.retestNode(nodeName)
}
</script>

<template>
  <div class="flex-1 overflow-auto bg-canvas">
    <table class="w-full border-collapse text-left text-xs select-none">
      <!-- Sticky Table Header -->
      <thead class="sticky top-0 bg-card border-b border-border z-10 text-content-secondary font-medium">
        <tr>
          <th class="py-2.5 px-4 w-12 text-center">序号</th>
          <th class="py-2.5 px-4 min-w-[200px]">节点名称</th>
          <th class="py-2.5 px-3 w-24">协议/地区</th>
          <th class="py-2.5 px-3 w-36">延迟 & Jitter</th>
          <th class="py-2.5 px-4 w-52">下载吞吐 (相对参照)</th>
          <th class="py-2.5 px-4 w-44">Google TTFB</th>
          <th class="py-2.5 px-4 w-48">分诊结论 & 采样轨迹</th>
          <th class="py-2.5 px-4 w-20 text-center">操作</th>
        </tr>
      </thead>

      <!-- Table Body -->
      <tbody class="divide-y divide-border/60">
        <tr
          v-for="(r, idx) in results"
          :key="r.proxy_name"
          @mouseenter="onRowHover(r)"
          @click="onRowHover(r)"
          class="transition-colors hover:bg-card-hover cursor-pointer"
          :class="store.focusedNode?.proxy_name === r.proxy_name ? 'bg-card-hover border-l-2 border-l-brand' : ''"
        >
          <!-- 1. Index -->
          <td class="py-2.5 px-4 text-center font-mono text-content-muted text-[11px]">
            {{ idx + 1 }}
          </td>

          <!-- 2. Node Name & Flags -->
          <td class="py-2.5 px-4 font-medium text-content-main">
            <div class="flex items-center gap-2">
              <span class="text-base leading-none">{{ r.country_flag || '🌐' }}</span>
              <span class="truncate max-w-xs font-mono" :title="r.proxy_name">{{ r.proxy_name }}</span>
            </div>
          </td>

          <!-- 3. Protocol & Country -->
          <td class="py-2.5 px-3 text-content-secondary font-mono">
            <div class="flex flex-col">
              <span class="uppercase text-[11px] font-semibold text-content-main">{{ r.proxy_type }}</span>
              <span class="text-[10px] text-content-muted">{{ r.country_code || 'UNK' }}</span>
            </div>
          </td>

          <!-- 4. Latency & Jitter with discrete glyph dots -->
          <td class="py-2.5 px-3 font-mono">
            <div class="flex flex-col gap-1">
              <div class="flex items-center gap-1.5">
                <span
                  class="font-bold text-xs"
                  :class="r.latency_ms <= 0 ? 'text-zinc-500' : r.latency_ms < 150 ? 'text-emerald-500' : r.latency_ms < 300 ? 'text-amber-500' : 'text-orange-500'"
                >
                  {{ r.latency_ms > 0 ? `${r.latency_ms} ms` : '超时' }}
                </span>
                <span v-if="r.jitter_ms > 0" class="text-[10px] text-content-muted">
                  ±{{ r.jitter_ms }}ms
                </span>
              </div>

              <!-- Discrete 6-sample shape glyph ribbon (Shapes, not color alone!) -->
              <div v-if="r.latency_samples && r.latency_samples.length > 0" class="flex items-center gap-1">
                <span
                  v-for="s in r.latency_samples"
                  :key="s.seq"
                  class="inline-flex items-center justify-center text-[9px] font-bold"
                  :title="`样本 #${s.seq}: ${s.success ? s.latency_ms + 'ms' : s.error || '失败'}`"
                >
                  <!-- Success = Solid circle glyph -->
                  <span v-if="s.success" class="text-emerald-500">●</span>
                  <!-- Failure = Cross symbol glyph -->
                  <span v-else class="text-red-500">✕</span>
                </span>
              </div>
            </div>
          </td>

          <!-- 5. Throughput Bar with Percentile Reference -->
          <td class="py-2.5 px-4">
            <div class="flex flex-col gap-1">
              <div class="flex items-center justify-between font-mono text-xs">
                <span class="font-bold text-content-main">
                  {{ r.download_speed_mbps.toFixed(2) }} <span class="text-[10px] font-normal text-content-muted">MB/s</span>
                </span>
                <span class="text-[10px] text-content-muted">
                  {{ getSpeedPercent(r.download_speed_mbps) }}%
                </span>
              </div>
              <!-- Horizontal Bar -->
              <div class="w-full h-1.5 rounded-full bg-border overflow-hidden">
                <div
                  class="h-full rounded-full transition-all duration-300"
                  :class="r.download_speed_mbps >= 20 ? 'bg-emerald-500' : r.download_speed_mbps >= 5 ? 'bg-blue-500' : 'bg-amber-500'"
                  :style="{ width: `${getSpeedPercent(r.download_speed_mbps)}%` }"
                ></div>
              </div>
            </div>
          </td>

          <!-- 6. Google TTFB Latency -->
          <td class="py-2.5 px-4 font-mono text-xs">
            <div v-if="r.google_ttfb_ms > 0" class="flex items-center gap-1.5">
              <span
                class="font-bold"
                :class="r.google_ttfb_ms < 600 ? 'text-emerald-500' : r.google_ttfb_ms < 1500 ? 'text-amber-500' : 'text-red-500'"
              >
                {{ r.google_ttfb_ms }} ms
              </span>
              <span class="text-[10px] text-content-muted px-1 rounded bg-card-subtle border border-border">
                {{ r.stability?.latency_grade || 'TTFB' }}
              </span>
            </div>
            <span v-else class="text-content-muted text-[11px]">—</span>
          </td>

          <!-- 7. Triage Badge & Raw Evidence Micro-Trajectory (e.g. 5/8 -> 0/6) -->
          <td class="py-2.5 px-4">
            <div class="flex flex-col gap-1 items-start">
              <!-- Conclusion Badge -->
              <span
                class="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium"
                :class="store.nodeAuditMap[r.proxy_name]?.badgeClass || 'bg-card-subtle text-content-muted border border-border'"
              >
                {{ store.nodeAuditMap[r.proxy_name]?.badgeLabel || '待评估' }}
              </span>

              <!-- Raw Evidence Micro Trajectory -->
              <div v-if="store.nodeAuditMap[r.proxy_name]?.historyTrail" class="flex items-center gap-1 font-mono text-[10px] text-content-muted">
                <span>轨迹:</span>
                <span class="text-content-secondary font-medium">
                  {{ store.nodeAuditMap[r.proxy_name]?.historyTrail }}
                </span>
              </div>
            </div>
          </td>

          <!-- 8. Action: Single Retest -->
          <td class="py-2.5 px-4 text-center">
            <button
              @click.stop="handleRetest(r.proxy_name)"
              class="px-2 py-1 rounded text-xs border border-border hover:border-brand hover:text-brand bg-card-subtle transition-colors"
              title="单独复测该节点"
            >
              复测
            </button>
          </td>
        </tr>

        <!-- Empty State -->
        <tr v-if="results.length === 0">
          <td colspan="8" class="py-16 text-center text-content-muted">
            <div class="flex flex-col items-center justify-center gap-2">
              <span class="text-3xl">📡</span>
              <span class="text-sm font-medium">暂无符合条件的测速结果</span>
              <span class="text-xs text-content-secondary">点击顶部“开始测试”启动节点遥测与分诊</span>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
