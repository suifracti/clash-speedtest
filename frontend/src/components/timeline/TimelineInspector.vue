<script setup lang="ts">
/**
 * Sample inspector.
 *
 * The panel is driven *only* by the selected raw sample. Nothing here is reverse-engineered from
 * aggregated statistics, so what is displayed is always exactly what was persisted.
 */
import { computed } from 'vue'
import { useTimelineStore } from '../../stores/timeline'
import { sampleSemantic } from '../../utils/timeline/encoding'
import { shortenKey } from '../../utils/timeline/lanes'
import { formatUtc, formatWallClock, hostTzOffsetMinutes } from '../../utils/timeline/time'

const store = useTimelineStore()

const sample = computed(() => store.selectedSample)

const tzOffsetMinutes = computed(() => (store.useUtc ? 0 : hostTzOffsetMinutes()))
const tzLabel = computed(() => (store.useUtc ? 'UTC' : '本地'))

const semantic = computed(() => (sample.value ? sampleSemantic(sample.value) : null))

const toneClass = computed(() => {
  switch (semantic.value?.tone) {
    case 'success':
      return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/30'
    case 'danger':
      return 'bg-red-500/10 text-red-600 dark:text-red-400 border-red-500/30'
    case 'warning':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/30'
    default:
      return 'bg-card-subtle text-content-muted border-border'
  }
})

const localTime = computed(() =>
  sample.value ? formatWallClock(sample.value.timestampMs, tzOffsetMinutes.value, true) : '—'
)
const utcTime = computed(() => (sample.value ? formatUtc(sample.value.timestampMs, true) : '—'))

const latencyText = computed(() =>
  sample.value && sample.value.success ? `${sample.value.latencyMs.toFixed(1)} ms` : '—'
)
const ttfbText = computed(() =>
  sample.value && sample.value.ttfbMs > 0 ? `${sample.value.ttfbMs.toFixed(1)} ms` : '—'
)

function stepCandidate(direction: 1 | -1): void {
  store.cycleCandidate(direction)
}
</script>

<template>
  <aside class="w-[320px] shrink-0 border-l border-border bg-card flex flex-col overflow-hidden select-none">
    <div class="flex items-center justify-between px-3 py-2 border-b border-border">
      <span class="text-[11px] font-semibold text-content-secondary">样本证据 (Sample Inspector)</span>
      <button
        v-if="sample"
        @click="store.clearSelection()"
        class="text-[11px] text-content-muted hover:text-content-main px-1.5 rounded"
        title="清除选择（Esc）"
      >
        ✕
      </button>
    </div>

    <div v-if="!sample" class="flex-1 flex flex-col items-center justify-center text-center px-5 gap-1.5">
      <span class="text-xl">🎯</span>
      <span class="text-[11px] font-medium text-content-secondary">点选时间轴上的任意样本</span>
      <span class="text-[10px] text-content-muted leading-relaxed">
        也可以把焦点放到时间轴后用 ← → 在样本间移动，↑ ↓ 切换 lane。此面板只展示真实存在的原始样本。
      </span>
    </div>

    <div v-else class="flex-1 overflow-y-auto px-3 py-3 flex flex-col gap-3 text-[11px]">
      <!-- Collision stepper: several raw samples can share one pixel column -->
      <div
        v-if="store.candidates.length > 1"
        class="flex items-center justify-between gap-2 px-2 py-1.5 rounded border border-amber-500/30 bg-amber-500/10"
      >
        <span class="text-amber-700 dark:text-amber-300">
          该时刻共 {{ store.candidates.length }} 条样本（{{ store.candidateIndex + 1 }}/{{ store.candidates.length }}）
        </span>
        <span class="flex items-center gap-1">
          <button
            @click="stepCandidate(-1)"
            class="w-5 h-5 rounded border border-amber-500/40 hover:bg-amber-500/20 leading-none"
            title="上一条"
          >
            ‹
          </button>
          <button
            @click="stepCandidate(1)"
            class="w-5 h-5 rounded border border-amber-500/40 hover:bg-amber-500/20 leading-none"
            title="下一条"
          >
            ›
          </button>
        </span>
      </div>

      <!-- Outcome -->
      <div class="flex items-center justify-between gap-2">
        <span class="px-2 py-0.5 rounded border text-[11px] font-medium" :class="toneClass">
          {{ semantic?.label }}
        </span>
        <span class="font-mono text-content-secondary">{{ semantic?.errorClass }}</span>
      </div>
      <div class="rounded border border-border bg-card-subtle px-2 py-1 text-[10px] text-content-secondary">
        采样来源：{{ sample.samplingTier }} · {{ sample.triggerType === 'manual' ? '手动触发' : sample.triggerType === 'scheduled' ? '周期触发' : '旧触发未知' }} · 策略 v{{ sample.samplingStrategyVersion }}
      </div>
      <p v-if="semantic && semantic.outcome !== 'success'" class="text-content-muted leading-relaxed -mt-1.5">
        {{ semantic.detail }}
      </p>

      <!-- Timestamp -->
      <div class="flex flex-col gap-1">
        <span class="text-[10px] font-semibold text-content-muted uppercase tracking-wider">时间戳</span>
        <div class="bg-card-subtle rounded border border-border px-2 py-1.5 font-mono flex flex-col gap-0.5">
          <div class="flex items-center justify-between">
            <span class="text-content-muted">{{ tzLabel }}</span>
            <span class="text-content-main font-semibold">{{ localTime }}</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-content-muted">UTC</span>
            <span class="text-content-secondary">{{ utcTime }}</span>
          </div>
        </div>
      </div>

      <!-- Measurements -->
      <div class="flex flex-col gap-1">
        <span class="text-[10px] font-semibold text-content-muted uppercase tracking-wider">测量值</span>
        <div class="bg-card-subtle rounded border border-border px-2 py-1.5 font-mono flex flex-col gap-1">
          <div class="flex items-center justify-between">
            <span class="text-content-muted">Latency</span>
            <span class="text-content-main font-semibold">{{ latencyText }}</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-content-muted">TTFB</span>
            <span class="text-content-main">{{ ttfbText }}</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-content-muted">Success</span>
            <span :class="sample.success ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">
              {{ sample.success ? 'true' : 'false' }}
            </span>
          </div>
          <div v-if="sample.errorDetail" class="flex flex-col gap-0.5 pt-1 border-t border-border">
            <span class="text-content-muted">ErrorDetail</span>
            <span class="text-content-secondary break-all">{{ sample.errorDetail }}</span>
          </div>
        </div>
      </div>

      <!-- Target & probe -->
      <div class="flex flex-col gap-1">
        <span class="text-[10px] font-semibold text-content-muted uppercase tracking-wider">探针与目标</span>
        <div class="bg-card-subtle rounded border border-border px-2 py-1.5 font-mono flex flex-col gap-1">
          <div class="flex items-center justify-between gap-2">
            <span class="text-content-muted shrink-0">ProbeType</span>
            <span class="text-content-main truncate">{{ sample.probeType || '—' }}</span>
          </div>
          <div class="flex items-start justify-between gap-2">
            <span class="text-content-muted shrink-0">Target</span>
            <span class="text-content-main text-right break-all">{{ sample.target || '—' }}</span>
          </div>
          <div class="flex items-center justify-between gap-2">
            <span class="text-content-muted shrink-0">Profile</span>
            <span class="text-content-secondary truncate">{{ sample.profileId || '—' }}</span>
          </div>
        </div>
      </div>

      <!-- Egress -->
      <div class="flex flex-col gap-1">
        <span class="text-[10px] font-semibold text-content-muted uppercase tracking-wider">出口</span>
        <div class="bg-card-subtle rounded border border-border px-2 py-1.5 font-mono flex flex-col gap-1">
          <div class="flex items-center justify-between gap-2">
            <span class="text-content-muted shrink-0">ExitIP</span>
            <span class="text-content-main truncate">{{ sample.exitIp || '—' }}</span>
          </div>
          <div class="flex items-center justify-between gap-2">
            <span class="text-content-muted shrink-0">Region</span>
            <span class="text-content-secondary truncate">{{ sample.exitRegion || '—' }}</span>
          </div>
        </div>
      </div>

      <!-- Advanced evidence -->
      <details class="flex flex-col gap-1">
        <summary class="text-[10px] font-semibold text-content-muted uppercase tracking-wider cursor-pointer">
          高级 / 证据标识
        </summary>
        <div class="mt-1 bg-card-subtle rounded border border-border px-2 py-1.5 font-mono flex flex-col gap-1">
          <div class="flex items-start justify-between gap-2">
            <span class="text-content-muted shrink-0">DisplayName</span>
            <span class="text-content-main text-right break-all">{{ sample.displayNameSnapshot || '—' }}</span>
          </div>
          <div class="flex items-start justify-between gap-2">
            <span class="text-content-muted shrink-0">NodeIdentityKey</span>
            <span class="text-content-main text-right break-all" :title="sample.nodeIdentityKey">
              {{ shortenKey(sample.nodeIdentityKey, 10, 6) }}
            </span>
          </div>
          <div class="flex items-start justify-between gap-2">
            <span class="text-content-muted shrink-0">LegacyNodeKey</span>
            <span class="text-content-secondary text-right break-all" :title="sample.nodeKey">
              {{ shortenKey(sample.nodeKey, 10, 6) }}
            </span>
          </div>
          <div class="flex items-start justify-between gap-2">
            <span class="text-content-muted shrink-0">ConfigRevision</span>
            <span class="text-content-main text-right break-all" :title="sample.configRevisionKey">
              {{ shortenKey(sample.configRevisionKey, 10, 6) }}
            </span>
          </div>
          <div class="flex items-start justify-between gap-2">
            <span class="text-content-muted shrink-0">RunID</span>
            <span class="text-content-secondary text-right break-all">{{ sample.runId }}</span>
          </div>
          <div class="flex items-start justify-between gap-2">
            <span class="text-content-muted shrink-0">SampleID</span>
            <span class="text-content-secondary text-right break-all">{{ sample.sampleId }}</span>
          </div>
        </div>
      </details>
    </div>
  </aside>
</template>
