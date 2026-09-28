<script setup lang="ts">
import { computed, ref } from 'vue'

export interface TrendPoint {
  id: string
  time: number // timestamp ms
  latencyMs?: number
  durationMs?: number
  status?: string // 'completed' | 'partial_failed' | 'failed' | 'unknown'
  outcome?: string // 'matched' | 'unlocked' | 'reachable' | 'timed_out' etc.
  loss?: number
  summary?: string
  isLatest?: boolean
}

const props = withDefaults(
  defineProps<{
    points: TrendPoint[]
    type?: 'latency' | 'service'
    height?: number
    window?: '24h' | '48h' | '7d' | 'all'
    showAxis?: boolean
  }>(),
  {
    type: 'latency',
    height: 36,
    window: 'all',
    showAxis: false,
  }
)

const activeWindow = ref<'24h' | '48h' | '7d' | 'all'>(props.window)
const hoveredIndex = ref<number | null>(null)
const tooltipX = ref(0)
const tooltipY = ref(0)
const svgContainerRef = ref<HTMLDivElement | null>(null)

// Filter points by selected window
const filteredPoints = computed(() => {
  if (!props.points || props.points.length === 0) return []
  const sorted = [...props.points].sort((a, b) => a.time - b.time)
  const now = Date.now()

  let cutoff = 0
  if (activeWindow.value === '24h') cutoff = now - 24 * 3600 * 1000
  else if (activeWindow.value === '48h') cutoff = now - 48 * 3600 * 1000
  else if (activeWindow.value === '7d') cutoff = now - 7 * 24 * 3600 * 1000

  const filtered = cutoff > 0 ? sorted.filter(p => p.time >= cutoff) : sorted
  return filtered.length > 0 ? filtered : sorted
})

// Metrics for rendering
const svgWidth = 220
const padX = 14
const padY = 6

const maxLatency = computed(() => {
  const vals = filteredPoints.value.map(p => p.latencyMs ?? p.durationMs ?? 0).filter(v => v > 0)
  if (vals.length === 0) return 500
  return Math.max(100, Math.ceil(Math.max(...vals) * 1.25))
})

function getX(index: number, total: number): number {
  if (total <= 1) return svgWidth / 2
  return padX + (index / (total - 1)) * (svgWidth - padX * 2)
}

function getY(point: TrendPoint): number {
  const val = point.latencyMs ?? point.durationMs ?? 0
  const isFailed = point.status === 'failed' || point.outcome === 'timed_out' || (point.loss ?? 0) >= 100
  if (isFailed || val <= 0) return props.height - padY
  const ratio = Math.min(1, Math.max(0, val / maxLatency.value))
  return (props.height - padY) - ratio * (props.height - padY * 2)
}

function pointColor(point: TrendPoint): string {
  const isFailed = point.status === 'failed' || point.outcome === 'timed_out' || (point.loss ?? 0) >= 100
  if (isFailed) return '#e05252' // Red
  const val = point.latencyMs ?? point.durationMs ?? 0
  if (val > 400 || (point.loss ?? 0) > 0) return '#d9822b' // Yellow/Orange
  return '#15803d' // Green
}

const pathD = computed(() => {
  const pts = filteredPoints.value
  if (pts.length < 2) return ''
  let d = ''
  pts.forEach((pt, i) => {
    const x = getX(i, pts.length)
    const y = getY(pt)
    if (i === 0) d += `M ${x.toFixed(1)},${y.toFixed(1)}`
    else d += ` L ${x.toFixed(1)},${y.toFixed(1)}`
  })
  return d
})

const areaD = computed(() => {
  const pts = filteredPoints.value
  if (pts.length < 2) return ''
  const firstX = getX(0, pts.length)
  const lastX = getX(pts.length - 1, pts.length)
  const bottomY = props.height - padY
  return `${pathD.value} L ${lastX.toFixed(1)},${bottomY} L ${firstX.toFixed(1)},${bottomY} Z`
})

// Comparisons: Current (hovered or latest), Previous (hovered - 1), Prior (hovered - 2)
const comparisonData = computed(() => {
  const pts = filteredPoints.value
  if (pts.length === 0) return null
  const idx = hoveredIndex.value !== null ? hoveredIndex.value : pts.length - 1
  const current = pts[idx]
  const prev = idx > 0 ? pts[idx - 1] : null
  const prior = idx > 1 ? pts[idx - 2] : null

  const curVal = current.latencyMs ?? current.durationMs ?? 0
  const prevVal = prev ? (prev.latencyMs ?? prev.durationMs ?? 0) : null
  const diffFromPrev = prevVal !== null && curVal > 0 && prevVal > 0 ? curVal - prevVal : null

  // Summary over window
  const validVals = pts.map(p => p.latencyMs ?? p.durationMs ?? 0).filter(v => v > 0)
  const avgVal = validVals.length > 0 ? Math.round(validVals.reduce((a, b) => a + b, 0) / validVals.length) : 0
  const passCount = pts.filter(p => p.status !== 'failed' && p.outcome !== 'timed_out' && (p.loss ?? 0) < 100).length
  const passRate = Math.round((passCount / pts.length) * 100)

  return {
    index: idx,
    total: pts.length,
    current,
    prev,
    prior,
    curVal,
    prevVal,
    diffFromPrev,
    avgVal,
    passRate,
  }
})

function formatTimeRel(timestamp: number): string {
  if (!timestamp) return ''
  const diffSec = Math.floor((Date.now() - timestamp) / 1000)
  if (diffSec < 60) return '刚刚'
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}分钟前`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}小时前`
  return `${Math.floor(diffSec / 86400)}天前`
}

function formatExactTime(timestamp: number): string {
  if (!timestamp) return ''
  const d = new Date(timestamp)
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function handleMouseMove(e: MouseEvent) {
  if (!svgContainerRef.value || filteredPoints.value.length === 0) return
  const rect = svgContainerRef.value.getBoundingClientRect()
  const mouseX = e.clientX - rect.left
  const total = filteredPoints.value.length

  let closestIdx = 0
  let minDist = Infinity
  for (let i = 0; i < total; i++) {
    const px = getX(i, total)
    const dist = Math.abs(px - mouseX)
    if (dist < minDist) {
      minDist = dist
      closestIdx = i
    }
  }

  hoveredIndex.value = closestIdx
  tooltipX.value = e.clientX
  tooltipY.value = e.clientY
}

function handleMouseLeave() {
  hoveredIndex.value = null
}
</script>

<template>
  <div
    ref="svgContainerRef"
    class="interactive-sparkline-wrapper"
    @mousemove="handleMouseMove"
    @mouseleave="handleMouseLeave"
  >
    <div v-if="filteredPoints.length === 0" class="sparkline-empty">
      <span class="empty-dot"></span>
      <span class="empty-text">无历史记录</span>
    </div>

    <template v-else>
      <svg
        :viewBox="`0 0 ${svgWidth} ${height}`"
        class="sparkline-svg"
        preserveAspectRatio="none"
      >
        <defs>
          <linearGradient id="sparkAreaGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#15803d" stop-opacity="0.18" />
            <stop offset="100%" stop-color="#15803d" stop-opacity="0.01" />
          </linearGradient>
        </defs>

        <!-- Area fill under line -->
        <path
          v-if="areaD"
          :d="areaD"
          fill="url(#sparkAreaGrad)"
        />

        <!-- Connected line -->
        <path
          v-if="pathD"
          :d="pathD"
          fill="none"
          stroke="#15803d"
          stroke-width="1.8"
          stroke-linecap="round"
          stroke-linejoin="round"
          opacity="0.85"
        />

        <!-- Hover vertical cursor line -->
        <line
          v-if="hoveredIndex !== null"
          :x1="getX(hoveredIndex, filteredPoints.length)"
          :x2="getX(hoveredIndex, filteredPoints.length)"
          y1="2"
          :y2="height - 4"
          stroke="#94a3b8"
          stroke-width="1.2"
          stroke-dasharray="2,2"
        />

        <!-- Data points -->
        <g
          v-for="(point, idx) in filteredPoints"
          :key="point.id || idx"
          class="point-group"
        >
          <!-- Outer halo for hovered or latest point -->
          <circle
            v-if="hoveredIndex === idx || (hoveredIndex === null && idx === filteredPoints.length - 1)"
            :cx="getX(idx, filteredPoints.length)"
            :cy="getY(point)"
            r="5"
            :fill="pointColor(point)"
            opacity="0.25"
          />
          <!-- Core dot -->
          <circle
            :cx="getX(idx, filteredPoints.length)"
            :cy="getY(point)"
            :r="hoveredIndex === idx || idx === filteredPoints.length - 1 ? 3.5 : 2.5"
            :fill="pointColor(point)"
            stroke="#ffffff"
            stroke-width="1.2"
          />
        </g>
      </svg>

      <!-- Hover Tooltip Floating Card -->
      <teleport to="body">
        <div
          v-if="hoveredIndex !== null && comparisonData"
          class="trend-hover-popover"
          :style="{
            left: `${tooltipX + 16}px`,
            top: `${tooltipY - 30}px`,
          }"
        >
          <div class="popover-header">
            <span class="header-badge">
              {{ hoveredIndex === filteredPoints.length - 1 ? '🌟 本次 (最新)' : `第 ${hoveredIndex + 1}/${filteredPoints.length} 次测试` }}
            </span>
            <span class="header-time">{{ formatExactTime(comparisonData.current.time) }} ({{ formatTimeRel(comparisonData.current.time) }})</span>
          </div>

          <div class="popover-body">
            <!-- Current Attempt Stat -->
            <div class="stat-row highlight">
              <span class="stat-label">本次耗时:</span>
              <strong class="stat-val" :style="{ color: pointColor(comparisonData.current) }">
                {{ comparisonData.curVal > 0 ? `${comparisonData.curVal} ms` : '测试失败 / 超时' }}
              </strong>
              <span v-if="comparisonData.current.loss !== undefined" class="loss-badge" :class="{ 'has-loss': comparisonData.current.loss > 0 }">
                丢包 {{ Math.round(comparisonData.current.loss) }}%
              </span>
            </div>

            <!-- Comparison with Previous -->
            <div v-if="comparisonData.prev" class="stat-row compare-row">
              <span class="stat-label">上次 ({{ formatTimeRel(comparisonData.prev.time) }}):</span>
              <span class="stat-prev">{{ comparisonData.prevVal ?? 0 }} ms</span>
              <span
                v-if="comparisonData.diffFromPrev !== null"
                class="diff-badge"
                :class="comparisonData.diffFromPrev <= 0 ? 'faster' : 'slower'"
              >
                {{ comparisonData.diffFromPrev <= 0 ? `较上次快 ${Math.abs(comparisonData.diffFromPrev)}ms` : `较上次慢 +${comparisonData.diffFromPrev}ms` }}
              </span>
            </div>

            <!-- Prior Attempt (上上次) -->
            <div v-if="comparisonData.prior" class="stat-row prior-row">
              <span class="stat-label">上上次 ({{ formatTimeRel(comparisonData.prior.time) }}):</span>
              <span class="stat-prev">{{ comparisonData.prior.latencyMs ?? comparisonData.prior.durationMs ?? 0 }} ms</span>
            </div>

            <!-- Aggregate Context -->
            <div class="popover-footer">
              <span>均值: <b>{{ comparisonData.avgVal }}ms</b></span>
              <span>通过率: <b>{{ comparisonData.passRate }}%</b></span>
              <span>记录数: <b>{{ comparisonData.total }}次</b></span>
            </div>
          </div>
        </div>
      </teleport>
    </template>
  </div>
</template>

<style scoped>
.interactive-sparkline-wrapper {
  position: relative;
  display: flex;
  align-items: center;
  width: 100%;
  max-width: 220px;
  cursor: crosshair;
}

.sparkline-svg {
  width: 100%;
  height: 100%;
  overflow: visible;
}

.sparkline-empty {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: var(--text-muted, #94a3b8);
  padding: 4px 6px;
}

.empty-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--border, #cbd5e1);
}

/* Hover Tooltip Popover */
.trend-hover-popover {
  position: fixed;
  z-index: 9999;
  pointer-events: none;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.12), 0 8px 10px -6px rgba(0, 0, 0, 0.08);
  padding: 10px 12px;
  min-width: 210px;
  font-family: inherit;
  font-size: 12px;
  line-height: 1.4;
  color: #1e293b;
  animation: fadeIn 0.12s ease-out;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: translateY(0); }
}

.popover-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 7px;
  padding-bottom: 5px;
  border-bottom: 1px solid #f1f5f9;
}

.header-badge {
  font-weight: 700;
  color: #0f172a;
  font-size: 12px;
}

.header-time {
  font-size: 11px;
  color: #64748b;
}

.popover-body {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.stat-row {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
}

.stat-row.highlight {
  font-size: 13px;
  margin-bottom: 2px;
}

.stat-label {
  color: #64748b;
}

.stat-val {
  font-weight: 700;
}

.stat-prev {
  color: #334155;
  font-weight: 600;
}

.diff-badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  font-weight: 600;
  margin-left: auto;
}

.diff-badge.faster {
  background: #dcfce7;
  color: #15803d;
}

.diff-badge.slower {
  background: #fef3c7;
  color: #b45309;
}

.loss-badge {
  font-size: 10px;
  color: #64748b;
  margin-left: auto;
}

.loss-badge.has-loss {
  color: #e05252;
  font-weight: 600;
}

.popover-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px dashed #e2e8f0;
  font-size: 10.5px;
  color: #64748b;
}

.popover-footer b {
  color: #0f172a;
}
</style>
