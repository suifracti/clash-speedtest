<script setup lang="ts">
/**
 * Observation summary strip.
 *
 * Product rule this component enforces: a single sample is a single fact, never a verdict. The
 * blocks below are therefore explicitly separated into "current sample" (one raw observation) and
 * "evidence window" (a bounded aggregate), and the wording avoids stability verdicts such as
 * "stable" or "persistent failure" that a single snapshot cannot support.
 *
 * These numbers are a secondary layer. They never replace the raw timeline.
 */
import { computed } from 'vue'
import { useTimelineStore } from '../../stores/timeline'
import { OUTCOME_LABELS, errorClassMeta, sampleSemantic } from '../../utils/timeline/encoding'
import { formatWallClock, hostTzOffsetMinutes } from '../../utils/timeline/time'
import type { DerivedStats } from '../../types'

const store = useTimelineStore()

const tzOffsetMinutes = computed(() => (store.useUtc ? 0 : hostTzOffsetMinutes()))

const current = computed(() => store.latestSample)

const currentSemantic = computed(() => (current.value ? sampleSemantic(current.value) : null))

const statsScopeLabel = computed(() => {
  if (store.samplingTier) return `来源：${store.samplingTier}`
  return '常规观测：regular / focus / sparse；已排除 diagnostic 与 legacy_unknown'
})

function includedTierLabel(stats: DerivedStats | null): string {
  return stats?.includedSamplingTiers.length ? stats.includedSamplingTiers.join('、') : '无'
}

function observedRangeLabel(stats: DerivedStats | null): string {
  if (!stats?.firstSampleAtMs || !stats.lastSampleAtMs) return '无样本时间范围'
  return `${new Date(stats.firstSampleAtMs).toLocaleString()} → ${new Date(stats.lastSampleAtMs).toLocaleString()}`
}

const rangeWindowLabel = computed(() => {
  const start = formatWallClock(store.domainStartMs, tzOffsetMinutes.value)
  const end = formatWallClock(store.domainEndMs, tzOffsetMinutes.value)
  return `${start} → ${end}`
})

function pct(value: number): string {
  return `${(value * 100).toFixed(1)}%`
}

function ms(value: number | null): string {
  return value === null ? '—' : `${value} ms`
}

function errorTotal(stats: DerivedStats | null): number {
  return stats ? stats.failureCount : 0
}

function topErrors(stats: DerivedStats | null): { label: string; count: number }[] {
  if (!stats) return []
  return Object.entries(stats.errorBreakdown)
    .filter(([cls]) => cls !== 'none')
    .map(([cls, count]) => ({ label: errorClassMeta(cls).label, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 4)
}
</script>

<template>
  <div class="bg-card border-b border-border px-3 py-2 flex items-stretch gap-2 overflow-x-auto text-[11px]">
    <!-- Current sample: exactly one raw observation -->
    <div class="shrink-0 w-[210px] rounded border border-border bg-card-subtle px-2.5 py-1.5 flex flex-col gap-1">
      <div class="flex items-center justify-between">
        <span class="text-[10px] font-semibold text-content-muted uppercase tracking-wider">当前样本</span>
        <span class="text-[10px] text-content-muted">单点事实</span>
      </div>
      <template v-if="current">
        <div class="flex items-center justify-between gap-2">
          <span
            class="px-1.5 py-0.5 rounded border text-[10px] font-medium"
            :class="
              currentSemantic?.tone === 'success'
                ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/30'
                : currentSemantic?.tone === 'danger'
                  ? 'bg-red-500/10 text-red-600 dark:text-red-400 border-red-500/30'
                  : currentSemantic?.tone === 'warning'
                    ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/30'
                    : 'bg-card text-content-muted border-border'
            "
          >
            {{ currentSemantic ? OUTCOME_LABELS[currentSemantic.outcome] : '—' }}
          </span>
          <span class="font-mono text-content-main">{{ current.success ? `${current.latencyMs.toFixed(0)} ms` : '—' }}</span>
        </div>
        <div class="font-mono text-[10px] text-content-muted truncate">
          {{ formatWallClock(current.timestampMs, tzOffsetMinutes, true) }}
        </div>
      </template>
      <span v-else class="text-[10px] text-content-muted">当前区间内没有样本</span>
    </div>

    <!-- Active-range evidence -->
    <div class="shrink-0 w-[250px] rounded border border-border px-2.5 py-1.5 flex flex-col gap-1">
      <div class="flex items-center justify-between gap-2">
        <span class="text-[10px] font-semibold text-content-muted uppercase tracking-wider">当前区间证据</span>
        <span class="text-[10px] text-content-muted font-mono truncate">{{ rangeWindowLabel }}</span>
      </div>
      <template v-if="store.rangeStats">
        <div class="text-[10px] leading-snug text-content-muted">{{ statsScopeLabel }}</div>
        <div class="grid grid-cols-2 gap-x-2 gap-y-0.5 font-mono">
          <span class="text-content-muted">样本</span>
          <span class="text-right text-content-main">{{ store.rangeStats.sampleCount }}</span>
          <span class="text-content-muted">成功率</span>
          <span class="text-right text-content-main">{{ pct(store.rangeStats.successRate) }}</span>
          <span class="text-content-muted">P50 / P95</span>
          <span class="text-right text-content-main">
            {{ ms(store.rangeStats.latencyP50Ms) }} / {{ ms(store.rangeStats.latencyP95Ms) }}
          </span>
          <span class="text-content-muted">失败数</span>
          <span class="text-right" :class="errorTotal(store.rangeStats) > 0 ? 'text-red-600 dark:text-red-400' : 'text-content-main'">
            {{ errorTotal(store.rangeStats) }}
          </span>
        </div>
        <div class="text-[10px] leading-snug text-content-muted">
          纳入层级：{{ includedTierLabel(store.rangeStats) }} · 样本加权结果，不代表时间可用率或公平节点排名
        </div>
        <div class="text-[10px] leading-snug text-content-muted">实际样本范围：{{ observedRangeLabel(store.rangeStats) }}</div>
        <div v-if="topErrors(store.rangeStats).length > 0" class="flex flex-wrap gap-1 pt-0.5">
          <span
            v-for="e in topErrors(store.rangeStats)"
            :key="e.label"
            class="px-1 py-0.5 rounded bg-card-subtle border border-border text-[10px] text-content-secondary"
          >
            {{ e.label }} {{ e.count }}
          </span>
        </div>
      </template>
      <span v-else-if="store.statsLoading" class="text-[10px] text-content-muted">统计计算中…</span>
      <span v-else class="text-[10px] text-content-muted">无可用统计</span>
    </div>

    <!-- 24h evidence -->
    <div class="shrink-0 w-[190px] rounded border border-border px-2.5 py-1.5 flex flex-col gap-1">
      <div class="flex items-center justify-between">
        <span class="text-[10px] font-semibold text-content-muted uppercase tracking-wider">24 小时证据</span>
      </div>
      <div v-if="store.evidence24h" class="grid grid-cols-2 gap-x-2 gap-y-0.5 font-mono">
        <span class="text-content-muted">样本</span>
        <span class="text-right text-content-main">{{ store.evidence24h.sampleCount }}</span>
        <span class="text-content-muted">成功率</span>
        <span class="text-right text-content-main">{{ pct(store.evidence24h.successRate) }}</span>
        <span class="text-content-muted">P95</span>
        <span class="text-right text-content-main">{{ ms(store.evidence24h.latencyP95Ms) }}</span>
      </div>
      <span v-else class="text-[10px] text-content-muted">—</span>
    </div>

    <!-- 7d evidence -->
    <div class="shrink-0 w-[190px] rounded border border-border px-2.5 py-1.5 flex flex-col gap-1">
      <div class="flex items-center justify-between">
        <span class="text-[10px] font-semibold text-content-muted uppercase tracking-wider">7 天证据</span>
      </div>
      <div v-if="store.evidence7d" class="grid grid-cols-2 gap-x-2 gap-y-0.5 font-mono">
        <span class="text-content-muted">样本</span>
        <span class="text-right text-content-main">{{ store.evidence7d.sampleCount }}</span>
        <span class="text-content-muted">成功率</span>
        <span class="text-right text-content-main">{{ pct(store.evidence7d.successRate) }}</span>
        <span class="text-content-muted">P95</span>
        <span class="text-right text-content-main">{{ ms(store.evidence7d.latencyP95Ms) }}</span>
      </div>
      <span v-else class="text-[10px] text-content-muted">—</span>
    </div>

    <div v-if="store.statsError" class="shrink-0 self-center text-[10px] text-amber-600 dark:text-amber-400">
      统计不可用：{{ store.statsError }}
    </div>
  </div>
</template>
