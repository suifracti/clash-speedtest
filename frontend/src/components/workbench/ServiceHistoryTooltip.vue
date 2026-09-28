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
    nodeKey?: string
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

const emit = defineEmits<{
  (e: 'keep-open'): void
  (e: 'request-close'): void
  (e: 'retest', payload: { nodeKey: string; serviceId: string; repeatCount: number }): void
}>()

const tooltipRef = ref<HTMLElement | null>(null)
const adjustedX = ref(0)
const adjustedY = ref(0)
const hoveredPtIndex = ref<number | null>(null)
const inspectedRunId = ref<string | null>(null)
const copiedNotice = ref('')

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

// Currently selected attempt to view (defaults to latest, user can click past runs to inspect)
const inspectedAttempt = computed(() => {
  if (!inspectedRunId.value) return latest.value
  return sortedHistory.value.find(a => (a.attempt_id || a.request_id) === inspectedRunId.value) || latest.value
})

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

function outcomeColor(outcome?: string): string {
  const tone = outcomeTone(outcome)
  if (tone === 'good') return '#10b981'
  if (tone === 'bad') return '#ef4444'
  if (tone === 'limited') return '#f59e0b'
  return '#94a3b8'
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
      return `真实模型响应成功 (${res.details.checked_model})`
    }
    return '真实模型响应成功'
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
const svgChartWidth = 330
const svgChartHeight = 64
const svgPadX = 14
const svgPadY = 10

const sparklinePoints = computed(() => {
  const reversed = [...sortedHistory.value].reverse()
  const valid = reversed.filter((a) => a.result?.duration_ms && a.result.duration_ms > 0)
  if (valid.length < 2) return []
  const max = Math.max(...valid.map((a) => a.result!.duration_ms!))
  const min = Math.min(...valid.map((a) => a.result!.duration_ms!))
  const range = max - min || 1
  return valid.map((a, idx) => {
    const dur = a.result!.duration_ms!
    const normalizedY = (svgChartHeight - svgPadY) - ((dur - min) / range) * (svgChartHeight - svgPadY * 2)
    const normalizedX = svgPadX + (idx / (valid.length - 1)) * (svgChartWidth - svgPadX * 2)
    const tone = outcomeTone(a.result?.outcome)
    return {
      id: a.attempt_id || a.request_id || String(idx),
      x: normalizedX,
      y: normalizedY,
      duration: dur,
      tone,
      color: outcomeColor(a.result?.outcome),
      outcome: a.result?.outcome,
      httpStatus: a.result?.http_status,
      timeRelative: formatRelativeTime(a.result?.finished_at),
      timeAbsolute: formatTime(a.result?.finished_at),
      ip: a.result?.details?.ip || '',
      summary: cleanResultSummary(a),
    }
  })
})

const sparklineSvgPath = computed(() => {
  if (sparklinePoints.value.length < 2) return ''
  return sparklinePoints.value.map((pt, i) => `${i === 0 ? 'M' : 'L'} ${pt.x.toFixed(1)} ${pt.y.toFixed(1)}`).join(' ')
})

const sparklineAreaPath = computed(() => {
  if (sparklinePoints.value.length < 2) return ''
  const first = sparklinePoints.value[0]
  const last = sparklinePoints.value[sparklinePoints.value.length - 1]
  const linePart = sparklinePoints.value.map((pt, i) => `${i === 0 ? 'M' : 'L'} ${pt.x.toFixed(1)} ${pt.y.toFixed(1)}`).join(' ')
  return `${linePart} L ${last.x.toFixed(1)} ${svgChartHeight} L ${first.x.toFixed(1)} ${svgChartHeight} Z`
})

const chartStats = computed(() => {
  if (sparklinePoints.value.length < 2) return null
  const durs = sparklinePoints.value.map(p => p.duration)
  const min = Math.min(...durs)
  const max = Math.max(...durs)
  const avg = Math.round(durs.reduce((s, v) => s + v, 0) / durs.length)
  return { min, max, avg }
})

// Copy helper with visual feedback
async function copyText(text: string, label: string) {
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    copiedNotice.value = `${label}已复制`
    setTimeout(() => {
      copiedNotice.value = ''
    }, 2000)
  } catch {
    copiedNotice.value = '复制失败'
  }
}

function copyAllDiagnostics() {
  const item = inspectedAttempt.value
  if (!item) return
  const report = [
    `【节点】: ${props.nodeName} (${props.countryCode})`,
    `【服务】: ${props.serviceName} (${item.service_id})`,
    `【时间】: ${item.result?.finished_at || '—'}`,
    `【状态】: ${serviceOutcomeLabel(item.result?.outcome)} (HTTP ${item.result?.http_status ?? '—'})`,
    `【耗时】: ${item.result?.duration_ms ?? '—'} ms`,
    `【出口 IP】: ${item.result?.details?.ip || '—'}`,
    `【所有检出出口】: ${distinctExitIPs.value.join(', ') || '—'}`,
    `【通过率】: ${passRate.value}% (波动 ${outcomeChanges.value} 次)`,
    `【测试摘要】: ${cleanResultSummary(item)}`,
    `【目标地址】: ${item.rule?.target_url || '—'}`,
    item.result?.error_message ? `【错误报文】: ${item.result.error_message}` : '',
  ].filter(Boolean).join('\n')
  copyText(report, '完整诊断')
}

function triggerRetest(repeatCount: number) {
  emit('retest', {
    nodeKey: props.nodeKey || '',
    serviceId: props.serviceId || inspectedAttempt.value?.service_id || '',
    repeatCount,
  })
}

// Viewport boundary adjustment
watch(
  () => [props.visible, props.x, props.y],
  async () => {
    if (!props.visible) {
      inspectedRunId.value = null
      hoveredPtIndex.value = null
      return
    }
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
      @mouseenter="emit('keep-open')"
      @mouseleave="emit('request-close')"
    >
      <!-- Header -->
      <div class="tooltip-header">
        <div class="header-left">
          <div class="tooltip-title-line">
            <span class="tooltip-flag">{{ nodeFlag }}</span>
            <strong class="tooltip-node-name" :title="nodeName">{{ nodeName }}</strong>
            <span v-if="countryCode" class="tooltip-country-tag">{{ countryCode }}</span>
          </div>
          <div class="tooltip-service-line">
            <span class="tooltip-service-name">{{ serviceName }}</span>
            <span v-if="ruleEvidence" class="tooltip-evidence-tag">{{ ruleEvidence }}</span>
            <span v-if="latest?.rule.rule_version" class="tooltip-rule-ver">v{{ latest.rule.rule_version }}</span>
          </div>
        </div>

        <div class="header-actions">
          <button
            type="button"
            class="header-retest-btn"
            :disabled="isTesting"
            title="立即为当前服务执行 1 次快速探测"
            @click.stop="triggerRetest(1)"
          >
            ⚡ 测1次
          </button>
          <button
            type="button"
            class="header-retest-btn primary"
            :disabled="isTesting"
            title="连续探测 3 次以深入识别多出口 IP 漂移与状态震荡"
            @click.stop="triggerRetest(3)"
          >
            ⚡ 连测3次
          </button>
          <button
            type="button"
            class="header-close-btn"
            title="关闭卡片"
            @click="emit('request-close')"
          >
            ✕
          </button>
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
          <small>点击上方“⚡ 测1次”或“连测3次”即可立即验证可用性</small>
        </div>
      </div>

      <!-- Tested Content -->
      <template v-else>
        <!-- Flapping / Drift Alert Banner -->
        <div v-if="hasDriftOrFlap" class="tooltip-drift-banner">
          <div v-if="isIPDrifting" class="drift-alert-item ip-drift">
            <span class="drift-icon">⇄</span>
            <div class="drift-content">
              <div class="drift-title-row">
                <strong>检测到落地 IP 漂移 (发现 {{ distinctExitIPs.length }} 个出口)</strong>
                <span class="drift-badge">会话轮换</span>
              </div>
              <div class="drift-ips">
                <span
                  v-for="ip in distinctExitIPs"
                  :key="ip"
                  class="drift-ip-pill"
                  :title="`点击复制出口 IP: ${ip}`"
                  @click.stop="copyText(ip, '出口IP')"
                >
                  <code>{{ ip }}</code>
                  <span class="copy-hint">📋</span>
                </span>
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

        <!-- Latest Test Result Card (Active Inspected Attempt) -->
        <div class="tooltip-latest-card" :class="outcomeTone(inspectedAttempt?.result?.outcome)">
          <div class="latest-card-header">
            <div class="latest-badge-group">
              <span class="latest-tone-dot"></span>
              <strong class="latest-outcome-label">
                {{ serviceOutcomeLabel(inspectedAttempt?.result?.outcome) }}
              </strong>
              <span v-if="inspectedRunId && inspectedRunId !== (latest?.attempt_id || latest?.request_id)" class="history-view-tag">
                (查看历史样本)
              </span>
            </div>
            <div class="latest-metrics">
              <span v-if="inspectedAttempt?.result?.duration_ms" class="latest-duration">
                ⏱️ {{ inspectedAttempt.result.duration_ms }} ms
              </span>
              <span v-if="inspectedAttempt?.result?.http_status" class="latest-status-code">
                HTTP {{ inspectedAttempt.result.http_status }}
              </span>
            </div>
          </div>

          <div class="latest-summary-line">
            {{ cleanResultSummary(inspectedAttempt) }}
          </div>

          <!-- Deep Diagnostic Info Bar -->
          <div class="latest-meta-line">
            <span class="latest-time">{{ formatRelativeTime(inspectedAttempt?.result?.finished_at) }}</span>
            <span v-if="inspectedAttempt?.result?.details?.ip" class="latest-ip" @click.stop="copyText(inspectedAttempt.result.details.ip, 'IP')">
              出口: <code>{{ inspectedAttempt.result.details.ip }}</code>
              <span v-if="inspectedAttempt.result.details.loc">({{ inspectedAttempt.result.details.loc }})</span>
              <span class="ip-copy-icon" title="点击复制 IP">📋</span>
            </span>
            <span v-if="inspectedAttempt?.result?.bytes_read" class="latest-bytes">
              包体: {{ (inspectedAttempt.result.bytes_read / 1024).toFixed(1) }} KiB
            </span>
          </div>

          <!-- Latency Delta from Previous -->
          <div v-if="previous && latencyDelta !== null && (!inspectedRunId || inspectedRunId === (latest?.attempt_id || latest?.request_id))" class="latest-delta-line">
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

          <!-- Error or diagnostic detail snippet -->
          <div v-if="inspectedAttempt?.result?.error_message" class="latest-error-box">
            <div class="error-box-title">异常诊断信息:</div>
            <code>{{ inspectedAttempt.result.error_message }}</code>
          </div>
        </div>

        <!-- Historical Sparkline (Enhanced Interactive Trend Chart) -->
        <div v-if="sparklinePoints.length >= 2" class="tooltip-sparkline-box">
          <div class="sparkline-header">
            <div class="sparkline-title-group">
              <span class="sparkline-icon">📈</span>
              <strong>延迟走势 (近 {{ sparklinePoints.length }} 次)</strong>
            </div>
            <div v-if="chartStats" class="sparkline-stat-tags">
              <span>极值: {{ chartStats.min }}ms ~ {{ chartStats.max }}ms</span>
              <span>均值: {{ chartStats.avg }}ms</span>
            </div>
          </div>

          <div class="sparkline-canvas-wrap">
            <svg class="sparkline-svg" :viewBox="`0 0 ${svgChartWidth} ${svgChartHeight}`">
              <defs>
                <linearGradient id="serviceTrendGrad" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" stop-color="#3b82f6" stop-opacity="0.32" />
                  <stop offset="100%" stop-color="#3b82f6" stop-opacity="0.01" />
                </linearGradient>
              </defs>

              <!-- Reference Gridlines -->
              <line :x1="svgPadX" :y1="svgPadY" :x2="svgChartWidth - svgPadX" :y2="svgPadY" class="sparkline-grid" />
              <line :x1="svgPadX" :y1="svgChartHeight / 2" :x2="svgChartWidth - svgPadX" :y2="svgChartHeight / 2" class="sparkline-grid dashed" />
              <line :x1="svgPadX" :y1="svgChartHeight - svgPadY" :x2="svgChartWidth - svgPadX" :y2="svgChartHeight - svgPadY" class="sparkline-grid" />

              <!-- Area Fill -->
              <path :d="sparklineAreaPath" fill="url(#serviceTrendGrad)" />

              <!-- Trend Polyline -->
              <path :d="sparklineSvgPath" class="sparkline-line" />

              <!-- Interactive Points -->
              <g
                v-for="(pt, i) in sparklinePoints"
                :key="i"
                class="sparkline-point-group"
                @mouseenter="hoveredPtIndex = i"
                @mouseleave="hoveredPtIndex = null"
              >
                <!-- Outer Hover Glow -->
                <circle
                  :cx="pt.x"
                  :cy="pt.y"
                  :r="hoveredPtIndex === i ? 7 : 4.5"
                  :fill="pt.color"
                  class="sparkline-outer-circle"
                  :class="{ active: hoveredPtIndex === i }"
                />
                <!-- Inner Dot -->
                <circle
                  :cx="pt.x"
                  :cy="pt.y"
                  r="2.5"
                  fill="#ffffff"
                  class="sparkline-inner-dot"
                />
                <!-- Transparent Hotspot -->
                <circle
                  :cx="pt.x"
                  :cy="pt.y"
                  r="14"
                  fill="transparent"
                  class="sparkline-hotspot"
                />
              </g>
            </svg>

            <!-- Floating Point Hover Popover -->
            <div
              v-if="hoveredPtIndex !== null && sparklinePoints[hoveredPtIndex]"
              class="point-hover-badge"
              :style="{
                left: `${Math.min(svgChartWidth - 90, Math.max(10, sparklinePoints[hoveredPtIndex].x - 45))}px`,
                top: `${Math.max(2, sparklinePoints[hoveredPtIndex].y - 34)}px`,
              }"
            >
              <strong>{{ sparklinePoints[hoveredPtIndex].duration }} ms</strong>
              <span>· HTTP {{ sparklinePoints[hoveredPtIndex].httpStatus ?? '—' }}</span>
              <small>({{ sparklinePoints[hoveredPtIndex].timeRelative }})</small>
            </div>
          </div>
        </div>

        <!-- History Runs List (Newest to Oldest) -->
        <div v-if="sortedHistory.length > 1" class="tooltip-history-list">
          <div class="history-list-title">
            <span>测试历史记录 ({{ sortedHistory.length }} 次采样)</span>
            <span class="history-pass-stat">通过率: {{ passRate }}%</span>
          </div>
          <div class="history-runs">
            <div
              v-for="(run, idx) in sortedHistory.slice(0, 6)"
              :key="run.attempt_id || idx"
              class="history-run-item"
              :class="{
                is_latest: idx === 0,
                is_selected: (run.attempt_id || run.request_id) === (inspectedAttempt?.attempt_id || inspectedAttempt?.request_id),
              }"
              title="点击可在上方预览该次采样的报文与出口诊断"
              @click="inspectedRunId = (run.attempt_id || run.request_id || null)"
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

        <!-- Target Rule & Technical Criterion Info -->
        <details v-if="latest?.rule" class="tooltip-tech-details">
          <summary>查看探测目标与成功判定准则</summary>
          <div class="tech-content">
            <div v-if="latest.rule.target_url" class="tech-row">
              <span class="tech-label">目标地址:</span>
              <code class="tech-code" :title="latest.rule.target_url">{{ latest.rule.target_url }}</code>
              <button type="button" class="tech-copy" @click.stop="copyText(latest.rule.target_url, '地址')">📋</button>
            </div>
            <div v-if="latest.rule.method" class="tech-row">
              <span class="tech-label">请求方法:</span>
              <span>{{ latest.rule.method }}</span>
            </div>
            <div v-if="latest.rule.success_criterion" class="tech-row">
              <span class="tech-label">通过准则:</span>
              <span>{{ latest.rule.success_criterion }}</span>
            </div>
          </div>
        </details>
      </template>

      <!-- Footer Action & Copy Bar -->
      <div class="tooltip-footer">
        <div class="footer-status-msg">
          <span v-if="copiedNotice" class="copied-badge">✅ {{ copiedNotice }}</span>
          <span v-else class="footer-tip">✨ 悬浮即查完整报文、走势与诊断 · 无需点击跳转</span>
        </div>
        <div class="footer-btns">
          <button
            type="button"
            class="footer-copy-btn"
            title="复制完整诊断报文至剪贴板"
            @click.stop="copyAllDiagnostics"
          >
            📋 复制诊断
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.service-history-tooltip {
  position: fixed;
  z-index: 10050;
  width: 390px;
  max-width: calc(100vw - 24px);
  max-height: min(85vh, 680px);
  overflow-y: auto;
  overflow-x: hidden;
  background: var(--bg-card, #ffffff);
  border: 1px solid var(--border-color, #e2e8f0);
  border-radius: 12px;
  box-shadow: 0 16px 38px -6px rgba(0, 0, 0, 0.22), 0 0 0 1px rgba(0, 0, 0, 0.05);
  font-family: inherit;
  font-size: 13px;
  line-height: 1.45;
  color: var(--text-main, #0f172a);
  pointer-events: auto;
  user-select: text;
  animation: tooltipFadeIn 0.16s cubic-bezier(0.16, 1, 0.3, 1);
}

/* Hover bridge: ensures mouse transitions seamlessly between trigger pill and tooltip */
.service-history-tooltip::before {
  content: '';
  position: absolute;
  top: -14px;
  left: 0;
  right: 0;
  height: 16px;
  background: transparent;
  pointer-events: auto;
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

:global(.dark) .service-history-tooltip {
  background: #182030;
  border-color: #2b3952;
  box-shadow: 0 18px 42px -6px rgba(0, 0, 0, 0.65), 0 0 0 1px rgba(255, 255, 255, 0.08);
  color: #f1f5f9;
}

/* Custom Slim Scrollbar */
.service-history-tooltip::-webkit-scrollbar {
  width: 5px;
}
.service-history-tooltip::-webkit-scrollbar-thumb {
  background: rgba(148, 163, 184, 0.35);
  border-radius: 4px;
}

/* Header */
.tooltip-header {
  padding: 10px 14px;
  background: var(--bg-hover, #f8fafc);
  border-bottom: 1px solid var(--border-color, #e2e8f0);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
:global(.dark) .tooltip-header {
  background: #1e293b;
  border-bottom-color: #2b3952;
}

.header-left {
  flex: 1;
  min-width: 0;
}

.tooltip-title-line {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 2px;
}

.tooltip-flag {
  font-size: 14px;
  flex-shrink: 0;
}

.tooltip-node-name {
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--text-main, #0f172a);
}
:global(.dark) .tooltip-node-name {
  color: #f8fafc;
}

.tooltip-country-tag {
  font-size: 10px;
  padding: 1px 4px;
  background: #e2e8f0;
  color: #475569;
  border-radius: 4px;
  font-weight: 600;
  flex-shrink: 0;
}
:global(.dark) .tooltip-country-tag {
  background: #334155;
  color: #94a3b8;
}

.tooltip-service-line {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.tooltip-service-name {
  font-weight: 600;
  color: #2563eb;
}
:global(.dark) .tooltip-service-name {
  color: #60a5fa;
}

.tooltip-evidence-tag {
  font-size: 10px;
  padding: 1px 5px;
  background: #dbeafe;
  color: #1e40af;
  border-radius: 4px;
}
:global(.dark) .tooltip-evidence-tag {
  background: #1e3a8a;
  color: #bfdbfe;
}

.tooltip-rule-ver {
  font-size: 10px;
  color: #94a3b8;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
}

.header-retest-btn {
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 600;
  border-radius: 6px;
  border: 1px solid #cbd5e1;
  background: #ffffff;
  color: #334155;
  cursor: pointer;
  transition: all 0.15s;
}
.header-retest-btn:hover:not(:disabled) {
  background: #f1f5f9;
  border-color: #94a3b8;
}
.header-retest-btn.primary {
  background: #2563eb;
  color: #ffffff;
  border-color: #1d4ed8;
}
.header-retest-btn.primary:hover:not(:disabled) {
  background: #1d4ed8;
}
.header-retest-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
:global(.dark) .header-retest-btn {
  background: #334155;
  color: #e2e8f0;
  border-color: #475569;
}
:global(.dark) .header-retest-btn:hover:not(:disabled) {
  background: #475569;
}
:global(.dark) .header-retest-btn.primary {
  background: #3b82f6;
  border-color: #2563eb;
  color: #ffffff;
}

.header-close-btn {
  border: none;
  background: transparent;
  color: #94a3b8;
  font-size: 13px;
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
}
.header-close-btn:hover {
  background: rgba(148, 163, 184, 0.2);
  color: #334155;
}
:global(.dark) .header-close-btn:hover {
  color: #f1f5f9;
}

/* Testing State */
.tooltip-testing-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  font-size: 13px;
  color: #3b82f6;
}
.tooltip-spinner {
  width: 14px;
  height: 14px;
  border: 2px solid #93c5fd;
  border-top-color: #2563eb;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Untested State */
.tooltip-untested-state {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 18px 16px;
  color: #64748b;
}
.untested-icon {
  font-size: 20px;
}
.untested-text strong {
  display: block;
  font-size: 13px;
  color: var(--text-main, #334155);
}
.untested-text small {
  display: block;
  font-size: 11px;
  margin-top: 2px;
  color: #94a3b8;
}

/* Flapping & IP Drift Banner */
.tooltip-drift-banner {
  margin: 10px 14px 4px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.drift-alert-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 8px;
  font-size: 12px;
}
.drift-alert-item.ip-drift {
  background: #fffbeb;
  border: 1px solid #fde68a;
  color: #92400e;
}
:global(.dark) .drift-alert-item.ip-drift {
  background: rgba(245, 158, 11, 0.12);
  border-color: rgba(245, 158, 11, 0.28);
  color: #fde68a;
}
.drift-alert-item.status-flap {
  background: #fef2f2;
  border: 1px solid #fecaca;
  color: #991b1b;
}
:global(.dark) .drift-alert-item.status-flap {
  background: rgba(239, 68, 68, 0.12);
  border-color: rgba(239, 68, 68, 0.28);
  color: #fca5a5;
}

.drift-icon {
  font-size: 14px;
  line-height: 1.2;
  flex-shrink: 0;
}
.drift-content {
  flex: 1;
  min-width: 0;
}
.drift-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}
.drift-badge {
  font-size: 10px;
  padding: 1px 4px;
  background: #fef3c7;
  color: #b45309;
  border-radius: 4px;
  font-weight: 600;
}
:global(.dark) .drift-badge {
  background: #78350f;
  color: #fef3c7;
}

.drift-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 5px;
}
.drift-ip-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 6px;
  background: #ffffff;
  border: 1px solid #fde68a;
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
  transition: all 0.15s;
}
.drift-ip-pill:hover {
  border-color: #f59e0b;
  background: #fef3c7;
}
:global(.dark) .drift-ip-pill {
  background: #1e293b;
  border-color: rgba(245, 158, 11, 0.35);
}
.copy-hint {
  font-size: 10px;
  opacity: 0.6;
}

.tooltip-stable-banner {
  margin: 8px 14px 2px;
  padding: 5px 10px;
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  border-radius: 6px;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: #166534;
}
:global(.dark) .tooltip-stable-banner {
  background: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.25);
  color: #86efac;
}

/* Latest Result Card */
.tooltip-latest-card {
  margin: 10px 14px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid var(--border-color, #e2e8f0);
  background: var(--bg-hover, #f8fafc);
}
:global(.dark) .tooltip-latest-card {
  background: #1e293b;
  border-color: #2b3952;
}

.tooltip-latest-card.good {
  border-left: 4px solid #10b981;
}
.tooltip-latest-card.bad {
  border-left: 4px solid #ef4444;
}
.tooltip-latest-card.limited {
  border-left: 4px solid #f59e0b;
}

.latest-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
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
.tooltip-latest-card.good .latest-tone-dot { background: #10b981; }
.tooltip-latest-card.bad .latest-tone-dot { background: #ef4444; }
.tooltip-latest-card.limited .latest-tone-dot { background: #f59e0b; }

.latest-outcome-label {
  font-size: 13px;
  font-weight: 700;
}
.history-view-tag {
  font-size: 11px;
  color: #3b82f6;
  font-weight: normal;
}

.latest-metrics {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}
.latest-duration {
  font-weight: 700;
  color: #2563eb;
}
:global(.dark) .latest-duration { color: #60a5fa; }

.latest-status-code {
  font-size: 11px;
  padding: 1px 4px;
  background: rgba(148, 163, 184, 0.15);
  border-radius: 4px;
  color: #475569;
}
:global(.dark) .latest-status-code { color: #cbd5e1; }

.latest-summary-line {
  font-size: 12px;
  color: #334155;
  margin-bottom: 6px;
  line-height: 1.4;
}
:global(.dark) .latest-summary-line { color: #e2e8f0; }

.latest-meta-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 11px;
  color: #64748b;
  border-top: 1px dashed rgba(148, 163, 184, 0.25);
  padding-top: 6px;
}
:global(.dark) .latest-meta-line { color: #94a3b8; }

.latest-ip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
}
.latest-ip:hover {
  color: #2563eb;
}
.ip-copy-icon {
  font-size: 10px;
  opacity: 0.7;
}

.latest-delta-line {
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px dashed rgba(148, 163, 184, 0.25);
  font-size: 11px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.delta-value.faster { color: #10b981; }
.delta-value.slower { color: #ef4444; }
.delta-prev-time { color: #94a3b8; }

.latest-error-box {
  margin-top: 6px;
  padding: 6px 8px;
  background: rgba(239, 68, 68, 0.08);
  border-radius: 6px;
  font-size: 11px;
  color: #dc2626;
  word-break: break-all;
}
:global(.dark) .latest-error-box {
  background: rgba(239, 68, 68, 0.15);
  color: #fca5a5;
}
.error-box-title {
  font-weight: 600;
  margin-bottom: 2px;
}

/* Sparkline Trend Chart */
.tooltip-sparkline-box {
  margin: 10px 14px;
  padding: 10px 12px;
  background: var(--bg-hover, #f8fafc);
  border: 1px solid var(--border-color, #e2e8f0);
  border-radius: 8px;
}
:global(.dark) .tooltip-sparkline-box {
  background: #1e293b;
  border-color: #2b3952;
}

.sparkline-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
  font-size: 12px;
}
.sparkline-title-group {
  display: flex;
  align-items: center;
  gap: 5px;
}
.sparkline-stat-tags {
  display: flex;
  gap: 6px;
  font-size: 10px;
  color: #64748b;
}
:global(.dark) .sparkline-stat-tags { color: #94a3b8; }

.sparkline-canvas-wrap {
  position: relative;
  width: 100%;
}

.sparkline-svg {
  width: 100%;
  height: 64px;
  overflow: visible;
}

.sparkline-grid {
  stroke: rgba(148, 163, 184, 0.2);
  stroke-width: 1;
}
.sparkline-grid.dashed {
  stroke-dasharray: 3 3;
}

.sparkline-line {
  fill: none;
  stroke: #3b82f6;
  stroke-width: 2.2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.sparkline-point-group {
  cursor: pointer;
}
.sparkline-outer-circle {
  transition: r 0.15s cubic-bezier(0.16, 1, 0.3, 1);
}
.sparkline-outer-circle.active {
  stroke: #ffffff;
  stroke-width: 2;
}

.point-hover-badge {
  position: absolute;
  z-index: 10;
  padding: 3px 7px;
  background: #0f172a;
  color: #ffffff;
  border-radius: 5px;
  font-size: 10px;
  white-space: nowrap;
  pointer-events: none;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
  display: flex;
  align-items: center;
  gap: 4px;
}
:global(.dark) .point-hover-badge {
  background: #334155;
  color: #f8fafc;
}

/* History List Table */
.tooltip-history-list {
  margin: 10px 14px;
}
.history-list-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
  margin-bottom: 6px;
}
:global(.dark) .history-list-title { color: #94a3b8; }
.history-pass-stat { color: #10b981; font-weight: 700; }

.history-runs {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.history-run-item {
  display: grid;
  grid-template-columns: 36px 1fr 50px 85px 65px;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border-radius: 6px;
  font-size: 11px;
  background: var(--bg-hover, #f8fafc);
  border: 1px solid transparent;
  cursor: pointer;
  transition: all 0.15s;
}
.history-run-item:hover {
  background: #f1f5f9;
  border-color: #cbd5e1;
}
.history-run-item.is_selected {
  background: #eff6ff;
  border-color: #93c5fd;
}
:global(.dark) .history-run-item {
  background: #1e293b;
}
:global(.dark) .history-run-item:hover {
  background: #283548;
  border-color: #3b4d66;
}
:global(.dark) .history-run-item.is_selected {
  background: rgba(59, 130, 246, 0.18);
  border-color: #3b82f6;
}

.run-index-badge {
  font-size: 10px;
  font-weight: 600;
  color: #64748b;
}
.run-status {
  display: flex;
  align-items: center;
  gap: 5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.run-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
  background: #94a3b8;
}
.run-status.good { color: #15803d; }
.run-status.good .run-dot { background: #10b981; }
.run-status.bad { color: #b91c1c; }
.run-status.bad .run-dot { background: #ef4444; }
.run-status.limited { color: #b45309; }
.run-status.limited .run-dot { background: #f59e0b; }

.run-dur {
  font-weight: 700;
  text-align: right;
  color: #334155;
}
:global(.dark) .run-dur { color: #cbd5e1; }

.run-ip {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: #64748b;
}
.run-time {
  text-align: right;
  color: #94a3b8;
}

/* Technical Details */
.tooltip-tech-details {
  margin: 8px 14px 4px;
  font-size: 11px;
  color: #64748b;
}
.tooltip-tech-details summary {
  cursor: pointer;
  user-select: none;
  color: #2563eb;
  margin-bottom: 4px;
}
:global(.dark) .tooltip-tech-details summary { color: #60a5fa; }

.tech-content {
  padding: 6px 8px;
  background: var(--bg-hover, #f8fafc);
  border-radius: 6px;
  border: 1px solid var(--border-color, #e2e8f0);
  display: flex;
  flex-direction: column;
  gap: 4px;
}
:global(.dark) .tech-content {
  background: #1e293b;
  border-color: #2b3952;
}
.tech-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.tech-label {
  color: #94a3b8;
  flex-shrink: 0;
}
.tech-code {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background: rgba(148, 163, 184, 0.15);
  padding: 1px 4px;
  border-radius: 4px;
  font-size: 10px;
}
.tech-copy {
  border: none;
  background: transparent;
  cursor: pointer;
  padding: 0 3px;
  font-size: 10px;
}

/* Footer Action & Copy Bar */
.tooltip-footer {
  padding: 8px 14px;
  background: var(--bg-hover, #f8fafc);
  border-top: 1px solid var(--border-color, #e2e8f0);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
}
:global(.dark) .tooltip-footer {
  background: #1e293b;
  border-top-color: #2b3952;
}

.footer-tip {
  color: #64748b;
}
:global(.dark) .footer-tip { color: #94a3b8; }

.copied-badge {
  color: #10b981;
  font-weight: 700;
}

.footer-copy-btn {
  padding: 3px 8px;
  font-size: 11px;
  background: #ffffff;
  border: 1px solid #cbd5e1;
  border-radius: 5px;
  cursor: pointer;
  color: #334155;
  transition: all 0.15s;
  flex-shrink: 0;
}
.footer-copy-btn:hover {
  background: #f1f5f9;
  border-color: #94a3b8;
}
:global(.dark) .footer-copy-btn {
  background: #334155;
  border-color: #475569;
  color: #f1f5f9;
}
:global(.dark) .footer-copy-btn:hover {
  background: #475569;
}
</style>
