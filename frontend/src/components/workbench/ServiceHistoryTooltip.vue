<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { WorkbenchPublicServiceAttempt } from '../../types'
import { serviceDetailLabel, serviceOutcomeLabel } from '../../utils/serviceOutcome'

const props = withDefaults(
  defineProps<{
    visible: boolean
    x: number
    y: number
    serviceName: string
    serviceId?: string
    nodeName: string
    nodeFlag?: string
    countryCode?: string
    history?: WorkbenchPublicServiceAttempt[]
    isTesting?: boolean
    ruleEvidence?: string
    teleportDisabled?: boolean
  }>(),
  {
    visible: false,
    history: () => [],
    nodeFlag: '🌐',
    countryCode: '',
    isTesting: false,
    ruleEvidence: '',
    teleportDisabled: false,
  },
)

const tooltipRef = ref<HTMLElement | null>(null)
const adjustedX = ref(0)
const adjustedY = ref(0)

// Sort history chronologically (newest first for comparison display)
const sortedHistory = computed(() => {
  const list = [...(props.history || [])].filter(
    (a) => a.result && a.result.outcome !== 'cancelled',
  )
  return list.sort((a, b) => {
    const timeA = Date.parse(a.result?.finished_at || a.finished_at || a.requested_at || '') || 0
    const timeB = Date.parse(b.result?.finished_at || b.finished_at || b.requested_at || '') || 0
    return timeB - timeA // Newest first
  })
})

const latest = computed(() => sortedHistory.value[0] || null)
const previous = computed(() => sortedHistory.value[1] || null)

// Track unique exit IPs across history runs
const distinctExitIPs = computed(() => {
  const set = new Set<string>()
  for (const a of sortedHistory.value) {
    const ip = a.result?.details?.ip
    if (ip && ip.trim()) set.add(ip.trim())
  }
  return [...set]
})

// Flapping / Drift analysis
const outcomeChanges = computed(() => {
  let count = 0
  for (let i = 1; i < sortedHistory.value.length; i++) {
    if (sortedHistory.value[i].result?.outcome !== sortedHistory.value[i - 1].result?.outcome) {
      count++
    }
  }
  return count
})

const isIPDrifting = computed(() => distinctExitIPs.value.length > 1)
const isStatusFlapping = computed(() => outcomeChanges.value > 0)
const hasDriftOrFlap = computed(() => isIPDrifting.value || isStatusFlapping.value)

// Pass rate calculation
const totalRuns = computed(() => sortedHistory.value.length)
const passedRuns = computed(
  () =>
    sortedHistory.value.filter((a) =>
      ['matched', 'reachable', 'profiled', 'unlocked'].includes(a.result?.outcome || ''),
    ).length,
)
const passRate = computed(() =>
  totalRuns.value > 0 ? Math.round((passedRuns.value / totalRuns.value) * 100) : 0,
)

// Latency comparison between latest and previous
const latencyDelta = computed(() => {
  if (!latest.value?.result?.duration_ms || !previous.value?.result?.duration_ms) return null
  return latest.value.result.duration_ms - previous.value.result.duration_ms
})

function outcomeTone(outcome?: string) {
  if (!outcome) return 'unknown'
  if (['matched', 'unlocked', 'profiled', 'reachable'].includes(outcome)) return 'good'
  if (['transport_error', 'timed_out'].includes(outcome)) return 'bad'
  return 'limited'
}

function formatRelativeTime(dateStr?: string) {
  if (!dateStr) return ''
  const t = Date.parse(dateStr)
  if (isNaN(t)) return ''
  const diffSec = Math.floor((Date.now() - t) / 1000)
  if (diffSec < 60) return '刚刚'
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)} 分钟前`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)} 小时前`
  return `${Math.floor(diffSec / 86400)} 天前`
}

function formatTime(dateStr?: string) {
  if (!dateStr) return ''
  const t = new Date(dateStr)
  if (isNaN(t.getTime())) return ''
  return `${t.getMonth() + 1}/${t.getDate()} ${t.getHours().toString().padStart(2, '0')}:${t.getMinutes().toString().padStart(2, '0')}`
}

function cleanResultSummary(attempt?: WorkbenchPublicServiceAttempt | null): string {
  if (!attempt?.result) return ''
  const res = attempt.result
  if (res.outcome === 'matched') {
    if (res.details?.checked_model) {
      return `真实模型响应 (${res.details.checked_model})`
    }
    return '检测通过，真实服务响应正常'
  }
  if (res.outcome === 'unlocked') return '完整版权片目已解锁'
  if (res.outcome === 'originals_only') return '仅可访问自制剧内容'
  if (res.outcome === 'region_blocked') return '服务明确限制该地区访问 (403)'
  if (res.outcome === 'challenge') return '遇到 Cloudflare 验证盾阻拦'
  if (res.outcome === 'rate_limited') return '请求频次或配额受限'
  if (res.outcome === 'timed_out') return '网络请求超时，未能收到响应'
  return res.summary || res.error_message || serviceOutcomeLabel(res.outcome)
}

// Sparkline points for recent runs (oldest to newest for visual flow)
const sparklinePoints = computed(() => {
  const reversed = [...sortedHistory.value].reverse()
  const valid = reversed.filter((a) => a.result?.duration_ms && a.result.duration_ms > 0)
  if (valid.length < 2) return []
  const max = Math.max(...valid.map((a) => a.result!.duration_ms!))
  const min = Math.min(...valid.map((a) => a.result!.duration_ms!))
  const range = max - min || 1
  return valid.map((a, idx) => {
    const dur = a.result!.duration_ms!
    const normalizedY = 28 - ((dur - min) / range) * 20
    const normalizedX = (idx / (valid.length - 1)) * 120 + 10
    const tone = outcomeTone(a.result?.outcome)
    return {
      x: normalizedX,
      y: normalizedY,
      duration: dur,
      tone,
    }
  })
})

const sparklineSvgPath = computed(() => {
  if (sparklinePoints.value.length < 2) return ''
  return sparklinePoints.value.map((pt, i) => `${i === 0 ? 'M' : 'L'} ${pt.x.toFixed(1)} ${pt.y.toFixed(1)}`).join(' ')
})

// Viewport boundary adjustment
watch(
  () => [props.visible, props.x, props.y],
  async () => {
    if (!props.visible) return
    await nextTick()
    if (!tooltipRef.value) return

    const el = tooltipRef.value
    const rect = el.getBoundingClientRect()
    const winWidth = window.innerWidth
    const winHeight = window.innerHeight

    let targetX = props.x + 12
    let targetY = props.y + 12

    // Prevent right overflow
    if (targetX + rect.width > winWidth - 16) {
      targetX = props.x - rect.width - 12
    }
    // Prevent left overflow
    if (targetX < 12) {
      targetX = 12
    }

    // Prevent bottom overflow
    if (targetY + rect.height > winHeight - 16) {
      targetY = props.y - rect.height - 12
    }
    // Prevent top overflow
    if (targetY < 12) {
      targetY = 12
    }

    adjustedX.value = targetX
    adjustedY.value = targetY
  },
  { immediate: true },
)
</script>

<template>
  <Teleport to="body" :disabled="teleportDisabled">
    <div
      v-if="visible"
      ref="tooltipRef"
      class="service-history-tooltip"
      :style="{
        left: `${adjustedX}px`,
        top: `${adjustedY}px`,
      }"
      role="tooltip"
    >
      <!-- Header -->
      <div class="tooltip-header">
        <div class="tooltip-title-line">
          <span class="tooltip-flag">{{ nodeFlag }}</span>
          <strong class="tooltip-node-name" :title="nodeName">{{ nodeName }}</strong>
          <span v-if="countryCode" class="tooltip-country-tag">{{ countryCode }}</span>
        </div>
        <div class="tooltip-service-line">
          <span class="tooltip-service-name">{{ serviceName }}</span>
          <span v-if="ruleEvidence" class="tooltip-evidence-tag">{{ ruleEvidence }}</span>
        </div>
      </div>

      <!-- Testing State -->
      <div v-if="isTesting" class="tooltip-testing-state">
        <span class="tooltip-spinner"></span>
        <span>正在为此节点执行探测…</span>
      </div>

      <!-- Untested State -->
      <div v-else-if="!latest" class="tooltip-untested-state">
        <div class="untested-icon">⚪</div>
        <div class="untested-text">
          <strong>尚未检测此项服务</strong>
          <small>点击胶囊可立即发起单项检测</small>
        </div>
      </div>

      <!-- Tested Content -->
      <template v-else>
        <!-- Flapping / Drift Alert Banner -->
        <div v-if="hasDriftOrFlap" class="tooltip-drift-banner">
          <div v-if="isIPDrifting" class="drift-alert-item ip-drift">
            <span class="drift-icon">⇄</span>
            <div class="drift-content">
              <strong>检测到落地 IP 漂移 (发现 {{ distinctExitIPs.length }} 个出口)</strong>
              <div class="drift-ips">
                <code v-for="ip in distinctExitIPs" :key="ip">{{ ip }}</code>
              </div>
            </div>
          </div>
          <div v-if="isStatusFlapping" class="drift-alert-item status-flap">
            <span class="drift-icon">⚠️</span>
            <div class="drift-content">
              <strong>状态发生震荡波动 (变化 {{ outcomeChanges }} 次)</strong>
              <small>通过率 {{ passRate }}% ({{ passedRuns }}/{{ totalRuns }} 次通过)</small>
            </div>
          </div>
        </div>

        <div v-else-if="totalRuns >= 2" class="tooltip-stable-banner">
          <span class="stable-icon">✔</span>
          <span>历史表现稳定，出口 IP 与服务状态均无漂移</span>
        </div>

        <!-- Latest Test Result Card -->
        <div class="tooltip-latest-card" :class="outcomeTone(latest.result?.outcome)">
          <div class="latest-card-header">
            <div class="latest-badge-group">
              <span class="latest-tone-dot"></span>
              <strong class="latest-outcome-label">
                {{ serviceOutcomeLabel(latest.result?.outcome) }}
              </strong>
            </div>
            <div class="latest-metrics">
              <span v-if="latest.result?.duration_ms" class="latest-duration">
                ⏱️ {{ latest.result.duration_ms }} ms
              </span>
              <span v-if="latest.result?.http_status" class="latest-status-code">
                HTTP {{ latest.result.http_status }}
              </span>
            </div>
          </div>

          <div class="latest-summary-line">
            {{ cleanResultSummary(latest) }}
          </div>

          <div class="latest-meta-line">
            <span class="latest-time">{{ formatRelativeTime(latest.result?.finished_at) }}</span>
            <span v-if="latest.result?.details?.ip" class="latest-ip">
              出口: {{ latest.result.details.ip }}
              <template v-if="latest.result.details.loc">({{ latest.result.details.loc }})</template>
            </span>
          </div>

          <!-- Latency Delta from Previous -->
          <div v-if="previous && latencyDelta !== null" class="latest-delta-line">
            <span class="delta-label">较上次测试:</span>
            <strong
              class="delta-value"
              :class="{ faster: latencyDelta < -50, slower: latencyDelta > 50 }"
            >
              {{ latencyDelta > 0 ? `+${latencyDelta} ms` : `${latencyDelta} ms` }}
              <span v-if="latencyDelta < -50">⚡ 变快</span>
              <span v-else-if="latencyDelta > 50">🐢 变慢</span>
              <span v-else>≈ 相近</span>
            </strong>
            <span class="delta-prev-time">({{ formatRelativeTime(previous.result?.finished_at) }})</span>
          </div>
        </div>

        <!-- Historical Sparkline (if >= 2 points) -->
        <div v-if="sparklinePoints.length >= 2" class="tooltip-sparkline-box">
          <div class="sparkline-header">
            <span>延迟走势 (近 {{ sparklinePoints.length }} 次)</span>
            <small>{{ sparklinePoints[0].duration }}ms → {{ sparklinePoints[sparklinePoints.length - 1].duration }}ms</small>
          </div>
          <svg class="sparkline-svg" viewBox="0 0 140 32">
            <path :d="sparklineSvgPath" class="sparkline-line" />
            <circle
              v-for="(pt, i) in sparklinePoints"
              :key="i"
              :cx="pt.x"
              :cy="pt.y"
              r="3.5"
              :class="['sparkline-point', pt.tone]"
            />
          </svg>
        </div>

        <!-- History Runs List (Newest to Oldest) -->
        <div v-if="sortedHistory.length > 1" class="tooltip-history-list">
          <div class="history-list-title">
            <span>测试历史记录 ({{ sortedHistory.length }} 次采样)</span>
            <span class="history-pass-stat">通过率: {{ passRate }}%</span>
          </div>
          <div class="history-runs">
            <div
              v-for="(run, idx) in sortedHistory.slice(0, 4)"
              :key="run.attempt_id || idx"
              class="history-run-item"
              :class="{ is_latest: idx === 0 }"
            >
              <div class="run-index-badge">
                {{ idx === 0 ? '本次' : idx === 1 ? '上次' : `${idx + 1}次前` }}
              </div>
              <div class="run-status" :class="outcomeTone(run.result?.outcome)">
                <span class="run-dot"></span>
                <span>{{ serviceOutcomeLabel(run.result?.outcome) }}</span>
              </div>
              <div class="run-dur">
                {{ run.result?.duration_ms ? `${run.result.duration_ms}ms` : '—' }}
              </div>
              <div class="run-ip" :title="run.result?.details?.ip || ''">
                {{ run.result?.details?.ip || '—' }}
              </div>
              <div class="run-time">
                {{ formatRelativeTime(run.result?.finished_at) }}
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- Footer Hint -->
      <div class="tooltip-footer">
        <span>💡 点击胶囊可打开完整报文诊断与快速复测</span>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.service-history-tooltip {
  position: fixed;
  z-index: 100000;
  pointer-events: none;
  width: 320px;
  max-width: calc(100vw - 24px);
  background: var(--card-bg, #ffffff);
  border: 1px solid var(--border-focus, #cbd5e1);
  border-radius: 12px;
  box-shadow: 0 20px 35px -8px rgba(0, 0, 0, 0.22), 0 8px 16px -6px rgba(0, 0, 0, 0.12);
  padding: 12px 14px;
  font-family: inherit;
  font-size: 12px;
  color: var(--text-primary, #0f172a);
  animation: tooltipFadeIn 0.12s cubic-bezier(0.16, 1, 0.3, 1);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

:global(.dark) .service-history-tooltip {
  background: #181d28;
  border-color: #334155;
  color: #f1f5f9;
  box-shadow: 0 20px 35px -8px rgba(0, 0, 0, 0.65), 0 8px 16px -6px rgba(0, 0, 0, 0.45);
}

@keyframes tooltipFadeIn {
  from {
    opacity: 0;
    transform: translateY(4px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

/* Header */
.tooltip-header {
  display: flex;
  flex-direction: column;
  gap: 3px;
  border-bottom: 1px solid var(--border, #e2e8f0);
  padding-bottom: 8px;
}

:global(.dark) .tooltip-header {
  border-color: #2e384d;
}

.tooltip-title-line {
  display: flex;
  align-items: center;
  gap: 6px;
}

.tooltip-flag {
  font-size: 15px;
  line-height: 1;
}

.tooltip-node-name {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #0f172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:global(.dark) .tooltip-node-name {
  color: #f8fafc;
}

.tooltip-country-tag {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--card-subtle, #f1f5f9);
  color: var(--text-secondary, #64748b);
  font-weight: 600;
}

:global(.dark) .tooltip-country-tag {
  background: #242f44;
  color: #94a3b8;
}

.tooltip-service-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.tooltip-service-name {
  font-size: 11px;
  font-weight: 600;
  color: #2563eb;
}

:global(.dark) .tooltip-service-name {
  color: #60a5fa;
}

.tooltip-evidence-tag {
  font-size: 10px;
  color: var(--text-secondary, #64748b);
}

/* State Boxes */
.tooltip-testing-state {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px;
  background: #eff6ff;
  border-radius: 8px;
  color: #2563eb;
  font-weight: 600;
}

:global(.dark) .tooltip-testing-state {
  background: #1e293b;
  color: #60a5fa;
}

.tooltip-spinner {
  width: 14px;
  height: 14px;
  border: 2px solid #93c5fd;
  border-top-color: #2563eb;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.tooltip-untested-state {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  background: var(--card-subtle, #f8fafc);
  border-radius: 8px;
}

:global(.dark) .tooltip-untested-state {
  background: #1e293b;
}

.untested-icon {
  font-size: 18px;
}

.untested-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.untested-text strong {
  font-size: 12px;
  color: var(--text-primary, #1e293b);
}

:global(.dark) .untested-text strong {
  color: #f1f5f9;
}

.untested-text small {
  font-size: 10px;
  color: var(--text-secondary, #64748b);
}

/* Flapping & Drift Banner */
.tooltip-drift-banner {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 10px;
  border-radius: 8px;
  background: #fffbeb;
  border: 1px solid #fde68a;
  color: #92400e;
}

:global(.dark) .tooltip-drift-banner {
  background: #2d2417;
  border-color: #78350f;
  color: #fcd34d;
}

.drift-alert-item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
}

.drift-icon {
  font-size: 13px;
  font-weight: 700;
  line-height: 1.2;
}

.drift-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
}

.drift-content strong {
  font-size: 11px;
  line-height: 1.3;
}

.drift-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 2px;
}

.drift-ips code {
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 4px;
  background: #fef3c7;
  border: 1px solid #fde68a;
  color: #78350f;
  font-family: ui-monospace, SFMono-Regular, monospace;
}

:global(.dark) .drift-ips code {
  background: #451a03;
  border-color: #92400e;
  color: #fef3c7;
}

.tooltip-stable-banner {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  border-radius: 6px;
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  color: #166534;
  font-size: 10px;
}

:global(.dark) .tooltip-stable-banner {
  background: #0f2e1b;
  border-color: #14532d;
  color: #86efac;
}

.stable-icon {
  font-weight: 700;
}

/* Latest Result Card */
.tooltip-latest-card {
  padding: 10px;
  border-radius: 8px;
  background: var(--card-subtle, #f8fafc);
  border: 1px solid var(--border, #e2e8f0);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

:global(.dark) .tooltip-latest-card {
  background: #1f2736;
  border-color: #334155;
}

.tooltip-latest-card.good {
  border-left: 3px solid #10b981;
}

.tooltip-latest-card.bad {
  border-left: 3px solid #ef4444;
}

.tooltip-latest-card.limited {
  border-left: 3px solid #f59e0b;
}

.latest-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.latest-badge-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.latest-tone-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
}

.good .latest-tone-dot {
  background: #10b981;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.4);
}

.bad .latest-tone-dot {
  background: #ef4444;
  box-shadow: 0 0 6px rgba(239, 68, 68, 0.4);
}

.limited .latest-tone-dot {
  background: #f59e0b;
  box-shadow: 0 0 6px rgba(245, 158, 11, 0.4);
}

.latest-outcome-label {
  font-size: 12px;
  font-weight: 700;
}

.good .latest-outcome-label {
  color: #059669;
}

.bad .latest-outcome-label {
  color: #dc2626;
}

.limited .latest-outcome-label {
  color: #d97706;
}

:global(.dark) .good .latest-outcome-label {
  color: #34d399;
}

:global(.dark) .bad .latest-outcome-label {
  color: #f87171;
}

:global(.dark) .limited .latest-outcome-label {
  color: #fbbf24;
}

.latest-metrics {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
}

.latest-duration {
  font-weight: 700;
  color: #2563eb;
}

:global(.dark) .latest-duration {
  color: #60a5fa;
}

.latest-status-code {
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 4px;
  background: rgba(0, 0, 0, 0.05);
  font-family: ui-monospace, SFMono-Regular, monospace;
}

:global(.dark) .latest-status-code {
  background: rgba(255, 255, 255, 0.1);
}

.latest-summary-line {
  font-size: 11px;
  color: var(--text-secondary, #475569);
  line-height: 1.4;
}

:global(.dark) .latest-summary-line {
  color: #cbd5e1;
}

.latest-meta-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 10px;
  color: #64748b;
  padding-top: 4px;
  border-top: 1px dashed rgba(0, 0, 0, 0.06);
}

:global(.dark) .latest-meta-line {
  border-color: rgba(255, 255, 255, 0.08);
  color: #94a3b8;
}

.latest-delta-line {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  background: rgba(0, 0, 0, 0.03);
  padding: 4px 6px;
  border-radius: 5px;
}

:global(.dark) .latest-delta-line {
  background: rgba(255, 255, 255, 0.04);
}

.delta-label {
  color: #64748b;
  font-size: 10px;
}

.delta-value {
  font-weight: 700;
}

.delta-value.faster {
  color: #10b981;
}

.delta-value.slower {
  color: #ef4444;
}

.delta-prev-time {
  font-size: 10px;
  color: #94a3b8;
}

/* Sparkline Box */
.tooltip-sparkline-box {
  display: flex;
  flex-direction: column;
  gap: 4px;
  background: var(--card-subtle, #f8fafc);
  padding: 6px 10px;
  border-radius: 8px;
}

:global(.dark) .tooltip-sparkline-box {
  background: #1f2736;
}

.sparkline-header {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: #64748b;
}

.sparkline-svg {
  width: 100%;
  height: 28px;
  overflow: visible;
}

.sparkline-line {
  fill: none;
  stroke: #3b82f6;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.sparkline-point.good {
  fill: #10b981;
}

.sparkline-point.bad {
  fill: #ef4444;
}

.sparkline-point.limited {
  fill: #f59e0b;
}

/* History Runs List */
.tooltip-history-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.history-list-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 10px;
  font-weight: 700;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.history-pass-stat {
  font-weight: 600;
  color: #10b981;
}

.history-runs {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.history-run-item {
  display: grid;
  grid-template-columns: 38px 1fr 48px 1fr 44px;
  align-items: center;
  gap: 4px;
  padding: 3px 6px;
  border-radius: 5px;
  background: var(--card-subtle, #f8fafc);
  font-size: 10px;
}

:global(.dark) .history-run-item {
  background: #1f2736;
}

.history-run-item.is_latest {
  background: #eff6ff;
  border: 1px solid #bfdbfe;
}

:global(.dark) .history-run-item.is_latest {
  background: #1e3a8a33;
  border-color: #1d4ed8;
}

.run-index-badge {
  font-size: 9px;
  font-weight: 700;
  color: #64748b;
}

.run-status {
  display: flex;
  align-items: center;
  gap: 4px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.run-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  flex-shrink: 0;
}

.run-status.good {
  color: #059669;
}
.run-status.good .run-dot {
  background: #10b981;
}

.run-status.bad {
  color: #dc2626;
}
.run-status.bad .run-dot {
  background: #ef4444;
}

.run-status.limited {
  color: #d97706;
}
.run-status.limited .run-dot {
  background: #f59e0b;
}

.run-dur {
  font-weight: 700;
  text-align: right;
  font-family: ui-monospace, SFMono-Regular, monospace;
}

.run-ip {
  color: #64748b;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, monospace;
}

.run-time {
  color: #94a3b8;
  text-align: right;
  white-space: nowrap;
}

/* Footer */
.tooltip-footer {
  border-top: 1px dashed var(--border, #e2e8f0);
  padding-top: 6px;
  font-size: 10px;
  color: var(--text-secondary, #94a3b8);
  text-align: center;
}

:global(.dark) .tooltip-footer {
  border-color: #2e384d;
}
</style>
