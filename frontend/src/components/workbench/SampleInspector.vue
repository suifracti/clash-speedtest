<script setup lang="ts">
import { computed } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'

const store = useWorkbenchStore()

const node = computed(() => store.focusedNode)
const audit = computed(() => (node.value ? store.nodeAuditMap[node.value.proxy_name] : null))

async function handleRetest() {
  if (node.value) {
    await store.retestNode(node.value.proxy_name)
  }
}
</script>

<template>
  <aside class="w-80 border-l border-border bg-card p-4 flex flex-col gap-4 overflow-y-auto select-none">
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-border pb-3">
      <div class="flex items-center gap-2">
        <span class="text-xs font-semibold text-content-secondary">样本检查器 (Inspector)</span>
      </div>
      <button
        v-if="node"
        @click="handleRetest"
        class="text-xs px-2.5 py-1 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors"
      >
        立即复测
      </button>
    </div>

    <!-- Content when a node is selected/hovered -->
    <div v-if="node" class="flex flex-col gap-4 text-xs">
      <!-- Node Identity Card -->
      <div class="bg-card-subtle p-3 rounded-lg border border-border flex flex-col gap-1.5">
        <div class="flex items-center gap-2">
          <span class="text-xl">{{ node.country_flag || '🌐' }}</span>
          <span class="font-bold text-content-main font-mono truncate" :title="node.proxy_name">
            {{ node.proxy_name }}
          </span>
        </div>
        <div class="flex items-center gap-2 text-content-muted font-mono text-[11px]">
          <span>{{ node.proxy_type.toUpperCase() }}</span>
          <span>•</span>
          <span>{{ node.country_code || '未知地区' }}</span>
          <span v-if="node.server">• {{ node.server }}:{{ node.port }}</span>
        </div>
      </div>

      <!-- Triage Verdict & Evidence -->
      <div class="flex flex-col gap-2">
        <span class="text-[11px] font-semibold text-content-muted uppercase tracking-wider">分诊决策与可信度</span>
        <div class="bg-card-subtle p-3 rounded-lg border border-border flex flex-col gap-2">
          <div class="flex items-center justify-between">
            <span
              class="px-2 py-0.5 rounded text-[11px] font-medium"
              :class="audit?.badgeClass || 'bg-card text-content-muted border border-border'"
            >
              {{ audit?.badgeLabel || '待评估' }}
            </span>
            <span class="font-mono text-content-secondary font-bold">
              {{ audit?.historyTrail || '—' }}
            </span>
          </div>
          <p class="text-content-secondary text-[11px] leading-relaxed">
            {{ audit?.reason || '尚未收集足够样本' }}
          </p>
        </div>
      </div>

      <!-- Google TTFB & Stability Probes -->
      <div class="flex flex-col gap-2">
        <span class="text-[11px] font-semibold text-content-muted uppercase tracking-wider">Google 延迟与漂移探针</span>
        <div class="bg-card-subtle p-3 rounded-lg border border-border flex flex-col gap-2 font-mono">
          <div class="flex items-center justify-between">
            <span class="text-content-muted">API TTFB 均值:</span>
            <span class="font-bold text-content-main">
              {{ node.google_ttfb_ms > 0 ? `${node.google_ttfb_ms} ms` : '—' }}
            </span>
          </div>
          <div v-if="node.stability?.google_min_ttfb_ms" class="flex items-center justify-between text-[11px] text-content-muted">
            <span>最小 / 最大 TTFB:</span>
            <span>{{ node.stability.google_min_ttfb_ms }}ms / {{ node.stability.google_max_ttfb_ms }}ms</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-content-muted">探针成功率:</span>
            <span class="font-bold text-emerald-400">
              {{ node.stability ? `${node.stability.stability_rate.toFixed(1)}%` : '—' }}
            </span>
          </div>
          <div v-if="node.stability?.exit_ips && node.stability.exit_ips.length > 0" class="flex flex-col gap-1 pt-1 border-t border-border">
            <span class="text-[11px] text-content-muted">观测到的出口 IP ({{ node.stability.exit_ips.length }} 个):</span>
            <div class="flex flex-wrap gap-1">
              <span
                v-for="ip in node.stability.exit_ips"
                :key="ip"
                class="px-1.5 py-0.5 rounded bg-card text-[10px] text-content-secondary border border-border"
              >
                {{ ip }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- IP Attributes & Risk Dual Probe -->
      <div v-if="node.ip_info" class="flex flex-col gap-2">
        <span class="text-[11px] font-semibold text-content-muted uppercase tracking-wider">IP 属性与风控纯净度</span>
        <div class="bg-card-subtle p-3 rounded-lg border border-border flex flex-col gap-2 font-mono">
          <div class="flex items-center justify-between">
            <span class="text-content-muted">出口 IP:</span>
            <span class="font-bold text-content-main">{{ node.ip_info.ip }}</span>
          </div>
          <div class="flex items-center justify-between text-[11px]">
            <span class="text-content-muted">运营商 / ISP:</span>
            <span class="text-content-secondary truncate max-w-[140px]" :title="node.ip_info.isp">
              {{ node.ip_info.isp || '—' }}
            </span>
          </div>
          <div class="flex items-center justify-between text-[11px]">
            <span class="text-content-muted">IP 类型:</span>
            <span class="text-content-main">{{ node.ip_info.ip_type }} • {{ node.ip_info.origin_type }}</span>
          </div>
          <div class="grid grid-cols-2 gap-2 pt-1 border-t border-border">
            <div class="bg-card p-2 rounded border border-border flex flex-col">
              <span class="text-[10px] text-content-muted">ping0 风控分</span>
              <span
                class="text-sm font-bold"
                :class="node.ip_info.risk_score > 50 ? 'text-red-400' : 'text-emerald-400'"
              >
                {{ node.ip_info.risk_score }}
              </span>
            </div>
            <div class="bg-card p-2 rounded border border-border flex flex-col">
              <span class="text-[10px] text-content-muted">IPPure 欺诈分</span>
              <span
                class="text-sm font-bold"
                :class="node.ip_info.fraud_score > 50 ? 'text-red-400' : 'text-emerald-400'"
              >
                {{ node.ip_info.fraud_score }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-else class="flex-1 flex flex-col items-center justify-center text-content-muted text-center p-4">
      <span class="text-2xl mb-2">🔍</span>
      <span class="font-medium text-xs">悬停或点击任意节点</span>
      <span class="text-[11px] text-content-muted mt-1">即可在此处下钻查看 TTFB、风控与多轮证据</span>
    </div>
  </aside>
</template>
