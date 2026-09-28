<script setup lang="ts">
import { serviceOutcomeLabel } from "../../utils/serviceOutcome"
import { computed, ref } from 'vue'

export type LatencySampleItem = {
  seq?: number
  timestamp?: string
  latency_ms: number
  success: boolean
  error?: string
}

export type ThroughputSampleItem = {
  elapsed_ns: number
  interval_ns?: number
  delta_bytes: number
  cumulative_bytes: number
  speed_mbps?: number
}

export type ServiceSampleItem = {
  seq?: number
  name?: string
  timestamp?: string
  duration_ms: number
  http_status?: number
  outcome: string
  error?: string
}

const props = withDefaults(
  defineProps<{
    mode?: 'latency' | 'throughput' | 'service'
    latencySamples?: LatencySampleItem[]
    throughputSamples?: ThroughputSampleItem[]
    serviceSamples?: ServiceSampleItem[]
    height?: number
    compact?: boolean
  }>(),
  {
    mode: 'latency',
    latencySamples: () => [],
    throughputSamples: () => [],
    serviceSamples: () => [],
    height: 72,
    compact: false,
  }
)

const emit = defineEmits<{
  (e: 'select-sample', sample: any, index: number): void
}>()

const activeIndex = ref<number | null>(null)
const pinnedIndex = ref<number | null>(null)
const svgRef = ref<SVGSVGElement | null>(null)

const padLeft = 44
const padRight = 16
const padTop = 14
const padBottom = 22

// --- Latency Calculations ---
const validLatencySamples = computed(() => props.latencySamples || [])
const latencyMax = computed(() => {
  const values = validLatencySamples.value.map((s) => s.latency_ms || 0)
  const max = Math.max(...values, 0)
  return Math.max(100, Math.ceil((max * 1.15) / 10) * 10)
})

// --- Throughput Calculations ---
const validThroughputSamples = computed(() => props.throughputSamples || [])
const speedMax = computed(() => {
  const values = validThroughputSamples.value.map((s) => s.speed_mbps || 0)
  const max = Math.max(...values, 0)
  return Math.max(10, Math.ceil((max * 1.2) / 5) * 5)
})

// --- Service Calculations ---
const validServiceSamples = computed(() => props.serviceSamples || [])
const serviceDurationMax = computed(() => {
  const values = validServiceSamples.value.map((s) => s.duration_ms || 0)
  const max = Math.max(...values, 0)
  return Math.max(200, Math.ceil((max * 1.2) / 50) * 50)
})

const totalCount = computed(() => {
  if (props.mode === 'latency') return validLatencySamples.value.length
  if (props.mode === 'throughput') return validThroughputSamples.value.length
  return validServiceSamples.value.length
})

function sampleX(index: number, width: number): number {
  const count = totalCount.value
  if (count <= 1) return padLeft + (width - padLeft - padRight) / 2
  const span = width - padLeft - padRight
  return padLeft + (index / (count - 1)) * span
}

function latencyY(sample: LatencySampleItem, height: number): number {
  const plotH = height - padTop - padBottom
  const val = Math.max(0, sample.latency_ms)
  return padTop + plotH - (val / latencyMax.value) * plotH
}

function speedY(sample: ThroughputSampleItem, height: number): number {
  const plotH = height - padTop - padBottom
  const val = Math.max(0, sample.speed_mbps || 0)
  return padTop + plotH - (val / speedMax.value) * plotH
}

function serviceY(sample: ServiceSampleItem, height: number): number {
  const plotH = height - padTop - padBottom
  const val = Math.max(0, sample.duration_ms || 0)
  return padTop + plotH - (val / serviceDurationMax.value) * plotH
}

function pointsPath(width: number, height: number): string {
  if (props.mode === 'latency') {
    const list = validLatencySamples.value
    if (list.length < 2) return ''
    return list.map((s, i) => `${sampleX(i, width)},${latencyY(s, height)}`).join(' ')
  }
  if (props.mode === 'throughput') {
    const list = validThroughputSamples.value
    if (list.length < 2) return ''
    return list.map((s, i) => `${sampleX(i, width)},${speedY(s, height)}`).join(' ')
  }
  const list = validServiceSamples.value
  if (list.length < 2) return ''
  return list.map((s, i) => `${sampleX(i, width)},${serviceY(s, height)}`).join(' ')
}

const currentHoverIndex = computed(() => {
  if (pinnedIndex.value !== null) return pinnedIndex.value
  if (activeIndex.value !== null) return activeIndex.value
  return null
})

function onMouseMove(event: MouseEvent) {
  if (!svgRef.value || totalCount.value === 0) return
  const rect = svgRef.value.getBoundingClientRect()
  const mouseX = event.clientX - rect.left
  const viewWidth = rect.width
  let bestIdx = 0
  let minDist = Number.POSITIVE_INFINITY

  for (let i = 0; i < totalCount.value; i++) {
    const x = sampleX(i, viewWidth)
    const dist = Math.abs(x - mouseX)
    if (dist < minDist) {
      minDist = dist
      bestIdx = i
    }
  }

  activeIndex.value = bestIdx
  emitSample(bestIdx)
}

function onMouseLeave() {
  activeIndex.value = null
}

function onClick(event: MouseEvent) {
  if (activeIndex.value !== null) {
    if (pinnedIndex.value === activeIndex.value) {
      pinnedIndex.value = null
    } else {
      pinnedIndex.value = activeIndex.value
    }
    emitSample(activeIndex.value)
  }
}

function emitSample(index: number) {
  if (props.mode === 'latency') {
    emit('select-sample', validLatencySamples.value[index], index)
  } else if (props.mode === 'throughput') {
    emit('select-sample', validThroughputSamples.value[index], index)
  } else {
    emit('select-sample', validServiceSamples.value[index], index)
  }
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MiB`
}

function formatTime(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
</script>

<template>
  <div class="intra-sample-plot" :class="{ 'intra-sample-plot--compact': compact }">
    <div v-if="!compact" class="plot-meta-bar">
      <span class="meta-label">
        <template v-if="mode === 'latency'">本次测试内结果 ({{ totalCount }} 样本波动)</template>
        <template v-else-if="mode === 'throughput'">下载速率曲线 ({{ totalCount }} 阶段过程)</template>
        <template v-else>服务请求耗时 ({{ totalCount }} 次响应)</template>
      </span>
      <span class="meta-tip">可左右移动悬浮查看每个样本；点击固定</span>
    </div>

    <div class="svg-container" :style="{ height: `${height}px` }">
      <svg
        ref="svgRef"
        class="plot-svg"
        viewBox="0 0 600 100"
        preserveAspectRatio="none"
        @mousemove="onMouseMove"
        @mouseleave="onMouseLeave"
        @click="onClick"
      >
        <!-- Horizontal Grid Lines -->
        <line x1="44" x2="584" y1="14" y2="14" stroke="currentColor" stroke-opacity="0.1" />
        <line x1="44" x2="584" y1="52" y2="52" stroke="currentColor" stroke-opacity="0.08" stroke-dasharray="2 2" />
        <line x1="44" x2="584" y1="78" y2="78" stroke="currentColor" stroke-opacity="0.2" />

        <!-- Y Axis Labels -->
        <text x="38" y="18" text-anchor="end" class="axis-text">
          <template v-if="mode === 'latency'">{{ latencyMax }}ms</template>
          <template v-else-if="mode === 'throughput'">{{ speedMax }}M</template>
          <template v-else>{{ serviceDurationMax }}ms</template>
        </text>
        <text x="38" y="82" text-anchor="end" class="axis-text">0</text>

        <!-- Connecting Polyline -->
        <polyline
          v-if="totalCount > 1"
          :points="pointsPath(600, 100)"
          fill="none"
          :stroke="mode === 'throughput' ? 'var(--success)' : 'var(--primary)'"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        />

        <!-- Active Hover Vertical Indicator -->
        <line
          v-if="currentHoverIndex !== null"
          :x1="sampleX(currentHoverIndex, 600)"
          :x2="sampleX(currentHoverIndex, 600)"
          y1="14"
          y2="78"
          stroke="var(--primary)"
          stroke-width="1.5"
          stroke-dasharray="3 2"
          stroke-opacity="0.8"
        />

        <!-- Latency Mode Points -->
        <template v-if="mode === 'latency'">
          <g v-for="(sample, idx) in validLatencySamples" :key="idx">
            <circle
              v-if="sample.success"
              :cx="sampleX(idx, 600)"
              :cy="latencyY(sample, 100)"
              :r="currentHoverIndex === idx ? 4.5 : 2.5"
              :fill="currentHoverIndex === idx ? 'var(--primary-hover)' : 'var(--primary)'"
              :stroke="pinnedIndex === idx ? 'var(--text-main)' : 'var(--card-bg)'"
              :stroke-width="currentHoverIndex === idx ? 1.5 : 1"
            >
              <title>#{{ sample.seq || idx + 1 }} · {{ sample.latency_ms }} ms · {{ formatTime(sample.timestamp) }}</title>
            </circle>
            <!-- Error / Timeout Cross -->
            <g
              v-else
              :transform="`translate(${sampleX(idx, 600)}, ${latencyY(sample, 100)})`"
              stroke="var(--danger)"
              stroke-width="1.6"
              stroke-linecap="round"
            >
              <title>#{{ sample.seq || idx + 1 }} · 失败 ({{ sample.error || '超时' }})</title>
              <line x1="-3" y1="-3" x2="3" y2="3" />
              <line x1="3" y1="-3" x2="-3" y2="3" />
            </g>
          </g>
        </template>

        <!-- Throughput Mode Points -->
        <template v-else-if="mode === 'throughput'">
          <g v-for="(sample, idx) in validThroughputSamples" :key="idx">
            <circle
              :cx="sampleX(idx, 600)"
              :cy="speedY(sample, 100)"
              :r="currentHoverIndex === idx ? 4.5 : 2.5"
              fill="var(--success)"
              :stroke="pinnedIndex === idx ? 'var(--text-main)' : 'var(--card-bg)'"
              :stroke-width="currentHoverIndex === idx ? 1.5 : 1"
            >
              <title>{{ (sample.elapsed_ns / 1e9).toFixed(1) }}s · {{ sample.speed_mbps?.toFixed(1) }} Mbps · 累计 {{ formatBytes(sample.cumulative_bytes) }}</title>
            </circle>
          </g>
        </template>

        <!-- Service Mode Points -->
        <template v-else>
          <g v-for="(sample, idx) in validServiceSamples" :key="idx">
            <circle
              :cx="sampleX(idx, 600)"
              :cy="serviceY(sample, 100)"
              :r="currentHoverIndex === idx ? 4.5 : 3"
              :fill="sample.outcome === 'matched' ? 'var(--success)' : 'var(--danger)'"
              :stroke="pinnedIndex === idx ? 'var(--text-main)' : 'var(--card-bg)'"
              :stroke-width="currentHoverIndex === idx ? 1.5 : 1"
            >
              <title>#{{ idx + 1 }} · {{ sample.duration_ms }} ms · HTTP {{ sample.http_status || '—' }}</title>
            </circle>
          </g>
        </template>
      </svg>

      <!-- Active Floating Readout Pill -->
      <div
        v-if="currentHoverIndex !== null"
        class="sample-readout-pill"
        :class="{ pinned: pinnedIndex !== null }"
      >
        <template v-if="mode === 'latency' && validLatencySamples[currentHoverIndex]">
          <span class="pill-seq">#{{ validLatencySamples[currentHoverIndex].seq || currentHoverIndex + 1 }}</span>
          <strong :class="validLatencySamples[currentHoverIndex].success ? 'text-success' : 'text-danger'">
            {{ validLatencySamples[currentHoverIndex].success ? `${Math.round(validLatencySamples[currentHoverIndex].latency_ms)} ms` : '失败' }}
          </strong>
          <span v-if="validLatencySamples[currentHoverIndex].timestamp" class="pill-time">{{ formatTime(validLatencySamples[currentHoverIndex].timestamp) }}</span>
          <span v-if="validLatencySamples[currentHoverIndex].error" class="pill-error text-danger">{{ validLatencySamples[currentHoverIndex].error }}</span>
        </template>

        <template v-else-if="mode === 'throughput' && validThroughputSamples[currentHoverIndex]">
          <span class="pill-seq">{{ (validThroughputSamples[currentHoverIndex].elapsed_ns / 1e9).toFixed(1) }}s</span>
          <strong class="text-success">{{ (validThroughputSamples[currentHoverIndex].speed_mbps || 0).toFixed(1) }} Mbps</strong>
          <span class="pill-time">累计 {{ formatBytes(validThroughputSamples[currentHoverIndex].cumulative_bytes) }}</span>
        </template>

        <template v-else-if="mode === 'service' && validServiceSamples[currentHoverIndex]">
          <span class="pill-seq">#{{ currentHoverIndex + 1 }}</span>
          <strong :class="validServiceSamples[currentHoverIndex].outcome === 'matched' ? 'text-success' : 'text-danger'">
            {{ validServiceSamples[currentHoverIndex].duration_ms }} ms
          </strong>
          <span class="pill-time">{{ serviceOutcomeLabel(validServiceSamples[currentHoverIndex].outcome) }} · HTTP {{ validServiceSamples[currentHoverIndex].http_status || '无' }}</span>
        </template>

        <em v-if="pinnedIndex !== null" class="pill-pinned">已固定</em>
      </div>
    </div>

    <!-- Bottom X-axis Badges / Sequence Pills -->
    <div class="sample-pills-row">
      <template v-if="mode === 'latency'">
        <button
          v-for="(s, idx) in validLatencySamples"
          :key="idx"
          type="button"
          class="sample-pill"
          :class="{
            active: currentHoverIndex === idx,
            fail: !s.success,
          }"
          @mouseenter="activeIndex = idx; emitSample(idx)"
          @mouseleave="activeIndex = null"
          @click="pinnedIndex = (pinnedIndex === idx ? null : idx); emitSample(idx)"
        >
          <span class="sample-pill__seq">#{{ s.seq || idx + 1 }}</span>
          <span class="sample-pill__val">{{ s.success ? `${Math.round(s.latency_ms)}ms` : '失败' }}</span>
        </button>
      </template>

      <template v-else-if="mode === 'throughput'">
        <button
          v-for="(s, idx) in validThroughputSamples"
          :key="idx"
          type="button"
          class="sample-pill"
          :class="{ active: currentHoverIndex === idx }"
          @mouseenter="activeIndex = idx; emitSample(idx)"
          @mouseleave="activeIndex = null"
          @click="pinnedIndex = (pinnedIndex === idx ? null : idx); emitSample(idx)"
        >
          <span class="sample-pill__seq">{{ (s.elapsed_ns / 1e9).toFixed(1) }}s</span>
          <span class="sample-pill__val">{{ s.speed_mbps?.toFixed(1) }}M</span>
        </button>
      </template>

      <template v-else>
        <button
          v-for="(s, idx) in validServiceSamples"
          :key="idx"
          type="button"
          class="sample-pill"
          :class="{
            active: currentHoverIndex === idx,
            fail: s.outcome !== 'matched',
          }"
          @mouseenter="activeIndex = idx; emitSample(idx)"
          @mouseleave="activeIndex = null"
          @click="pinnedIndex = (pinnedIndex === idx ? null : idx); emitSample(idx)"
        >
          <span class="sample-pill__seq">#{{ idx + 1 }}</span>
          <span class="sample-pill__val">{{ s.duration_ms }}ms</span>
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.intra-sample-plot {
  display: flex;
  flex-direction: column;
  gap: 6px;
  background: var(--card-subtle);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px 12px;
  color: var(--text-main);
  transition: all .15s ease;
}

.intra-sample-plot--compact {
  padding: 5px 8px;
  gap: 4px;
}

.plot-meta-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
}
.meta-label {
  font-weight: 700;
  color: var(--text-main);
}
.meta-tip {
  font-size: 10px;
  color: var(--text-muted);
}

.svg-container {
  position: relative;
  width: 100%;
}
.plot-svg {
  width: 100%;
  height: 100%;
  display: block;
  overflow: visible;
  cursor: crosshair;
}

.axis-text {
  font-size: 9px;
  fill: var(--text-muted);
  user-select: none;
}

.sample-readout-pill {
  position: absolute;
  top: 2px;
  right: 6px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 8px;
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  font-size: 11px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  pointer-events: none;
  z-index: 5;
}
.sample-readout-pill.pinned {
  border-color: var(--primary);
}
.pill-seq {
  font-weight: 700;
  color: var(--primary);
}
.pill-time {
  color: var(--text-secondary);
  font-size: 10px;
}
.pill-error {
  font-size: 10px;
}
.pill-pinned {
  font-style: normal;
  font-size: 9px;
  color: var(--primary);
  font-weight: 700;
}

.sample-pills-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  margin-top: 2px;
}
.sample-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--card-bg);
  color: var(--text-secondary);
  font-size: 10px;
  line-height: 1.2;
  cursor: pointer;
  transition: all .15s ease;
}
.sample-pill:hover,
.sample-pill.active {
  border-color: var(--primary);
  background: var(--primary-subtle);
  color: var(--text-main);
  transform: translateY(-1px);
}
.sample-pill.fail {
  border-color: var(--danger);
  color: var(--danger);
}
.sample-pill__seq {
  font-weight: 600;
  color: var(--primary);
}
.sample-pill.fail .sample-pill__seq {
  color: var(--danger);
}
.sample-pill__val {
  font-weight: 500;
}

.text-success { color: var(--success); }
.text-danger { color: var(--danger); }
</style>
