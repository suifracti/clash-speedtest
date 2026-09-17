<script setup lang="ts">
/**
 * Raw-sample timeline canvas.
 *
 * Every loaded sample is projected to its own mark. Colliding marks are never merged: the pointer
 * hit test returns the full collision group and the store lets the user step through it, so a
 * sample can always be reached individually regardless of zoom level.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useTimelineStore } from '../../stores/timeline'
import { computeLatencyScale, sampleSemantic, OUTCOME_LABELS } from '../../utils/timeline/encoding'
import { hitTestMarks, laneBand, projectSamples, type TimelineLayout } from '../../utils/timeline/marks'
import { renderTimeline } from '../../utils/timeline/render'
import { resolveTimelineTheme, type TimelineTheme } from '../../utils/timeline/theme'
import { buildTimeTicks, formatWallClock, hostTzOffsetMinutes } from '../../utils/timeline/time'

const store = useTimelineStore()

const PLOT_TOP_PX = 8
const AXIS_HEIGHT_PX = 24
const LANE_GAP_PX = 4
const MIN_LANE_HEIGHT_PX = 26
const MAX_LANE_HEIGHT_PX = 64
const LANE_LABEL_WIDTH_PX = 208

const canvasRef = ref<HTMLCanvasElement | null>(null)
const plotRef = ref<HTMLDivElement | null>(null)
const size = ref({ width: 800, height: 360 })
const hoverSampleId = ref<string | null>(null)
const hoverLaneIndex = ref<number | null>(null)
const hoverPoint = ref<{ x: number; y: number } | null>(null)
const theme = ref<TimelineTheme>(resolveTimelineTheme(null))
let dragState: { startX: number; startViewportStart: number } | null = null
let rafHandle: number | null = null
let resizeObserver: ResizeObserver | null = null

const tzOffsetMinutes = computed(() => (store.useUtc ? 0 : hostTzOffsetMinutes()))

const layout = computed<TimelineLayout>(() => {
  const laneCount = Math.max(1, store.lanes.length)
  const available = Math.max(0, size.value.height - AXIS_HEIGHT_PX - PLOT_TOP_PX)
  const raw = Math.floor(available / laneCount) - LANE_GAP_PX
  return {
    plotTopPx: PLOT_TOP_PX,
    laneHeightPx: Math.min(MAX_LANE_HEIGHT_PX, Math.max(MIN_LANE_HEIGHT_PX, raw)),
    laneGapPx: LANE_GAP_PX,
  }
})

const latencyScaleMs = computed(() => computeLatencyScale(store.samples))

const projection = computed(() =>
  projectSamples({
    lanes: store.lanes,
    viewport: store.viewport,
    layout: layout.value,
    latencyScaleMs: latencyScaleMs.value,
    maxStemHeightPx: Math.max(8, layout.value.laneHeightPx - 12),
  })
)

const ticks = computed(() =>
  buildTimeTicks(store.viewport, { tzOffsetMinutes: tzOffsetMinutes.value })
)

const hoveredSample = computed(() =>
  hoverSampleId.value ? store.samples.find((s) => s.sampleId === hoverSampleId.value) ?? null : null
)

const hoverTooltip = computed(() => {
  const sample = hoveredSample.value
  if (!sample) return null
  const local = formatWallClock(sample.timestampMs, tzOffsetMinutes.value, true)
  const tzLabel = store.useUtc ? 'UTC' : '本地'
  const semantic = sampleSemantic(sample)
  return {
    title: `${sample.displayNameSnapshot || sample.nodeIdentityKey} · ${sample.probeType}`,
    time: `${local} ${tzLabel}`,
    target: sample.target,
    outcome: OUTCOME_LABELS[semantic.outcome],
    latency: sample.success ? `${sample.latencyMs.toFixed(0)} ms` : '—',
    errorClass: semantic.errorClass,
    colliding:
      projection.value.marks.find((m) => m.sample.sampleId === sample.sampleId)?.coincidentCount ?? 1,
  }
})

function scheduleDraw(): void {
  if (rafHandle !== null) return
  rafHandle = requestAnimationFrame(() => {
    rafHandle = null
    draw()
  })
}

function draw(): void {
  const canvas = canvasRef.value
  if (!canvas) return
  const { width, height } = size.value
  if (width <= 0 || height <= 0) return

  const dpr = window.devicePixelRatio || 1
  const pixelWidth = Math.max(1, Math.floor(width * dpr))
  const pixelHeight = Math.max(1, Math.floor(height * dpr))
  if (canvas.width !== pixelWidth) canvas.width = pixelWidth
  if (canvas.height !== pixelHeight) canvas.height = pixelHeight
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`

  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

  renderTimeline({
    ctx,
    widthPx: width,
    heightPx: height,
    axisHeightPx: AXIS_HEIGHT_PX,
    projection: projection.value,
    viewport: store.viewport,
    ticks: ticks.value,
    boundaries: store.revisionBoundaries,
    selectedSampleId: store.selectedSampleId,
    hoverSampleId: hoverSampleId.value,
    theme: theme.value,
    hoverLaneIndex: hoverLaneIndex.value,
  })
}

function measure(): void {
  const el = plotRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  size.value = { width: Math.max(1, Math.floor(rect.width)), height: Math.max(1, Math.floor(rect.height)) }
  store.setWidth(size.value.width)
  theme.value = resolveTimelineTheme(el)
  scheduleDraw()
}

function pointerX(event: MouseEvent): number {
  const canvas = canvasRef.value
  if (!canvas) return 0
  return event.clientX - canvas.getBoundingClientRect().left
}

function pointerY(event: MouseEvent): number {
  const canvas = canvasRef.value
  if (!canvas) return 0
  return event.clientY - canvas.getBoundingClientRect().top
}

function updateHover(event: MouseEvent): void {
  const x = pointerX(event)
  const y = pointerY(event)
  const hits = hitTestMarks(projection.value, x, y)
  if (hits.length === 0) {
    hoverSampleId.value = null
    hoverLaneIndex.value = null
    hoverPoint.value = null
    return
  }
  const mark = projection.value.marks[hits[0]]
  hoverSampleId.value = mark.sample.sampleId
  hoverLaneIndex.value = mark.laneIndex
  hoverPoint.value = { x: mark.x, y: y }
}

function onMouseMove(event: MouseEvent): void {
  if (dragState) {
    const x = pointerX(event)
    const dx = x - dragState.startX
    const msPerPx = (store.viewport.endMs - store.viewport.startMs) / Math.max(1, store.viewport.widthPx)
    const startMs = dragState.startViewportStart - dx * msPerPx
    store.panToRange(startMs, startMs + (store.viewport.endMs - store.viewport.startMs))
    store.maybeLoadOlder()
    return
  }
  updateHover(event)
}

function onMouseLeave(): void {
  hoverSampleId.value = null
  hoverLaneIndex.value = null
  hoverPoint.value = null
}

function onMouseDown(event: MouseEvent): void {
  if (event.button !== 0) return
  dragState = { startX: pointerX(event), startViewportStart: store.viewport.startMs }
  ;(event.target as HTMLElement).focus?.()
}

function onMouseUp(): void {
  dragState = null
}

function onClick(event: MouseEvent): void {
  const hits = hitTestMarks(projection.value, pointerX(event), pointerY(event))
  if (hits.length === 0) {
    store.clearSelection()
    return
  }
  const ids = hits.map((i) => projection.value.marks[i].sample.sampleId)
  store.selectSample(ids[0], ids)
}

function onWheel(event: WheelEvent): void {
  event.preventDefault()
  const x = pointerX(event)
  const horizontal = Math.abs(event.deltaX) > Math.abs(event.deltaY)

  if (event.shiftKey || horizontal) {
    store.pan(event.deltaX !== 0 ? event.deltaX : event.deltaY)
  } else {
    const factor = event.deltaY > 0 ? 1.15 : 1 / 1.15
    store.zoom(factor, x)
  }
  store.maybeLoadOlder()
}

function onKeyDown(event: KeyboardEvent): void {
  switch (event.key) {
    case 'ArrowLeft':
      event.preventDefault()
      store.selectRelativeInLane(-1)
      break
    case 'ArrowRight':
      event.preventDefault()
      store.selectRelativeInLane(1)
      break
    case 'ArrowUp':
      event.preventDefault()
      store.selectAdjacentLane(-1)
      break
    case 'ArrowDown':
      event.preventDefault()
      store.selectAdjacentLane(1)
      break
    case 'Home':
      event.preventDefault()
      store.resetToLatest()
      break
    case 'End':
      event.preventDefault()
      store.resetToLatest()
      break
    case '+':
    case '=':
      event.preventDefault()
      store.zoom(1 / 1.3, store.viewport.widthPx / 2)
      break
    case '-':
    case '_':
      event.preventDefault()
      store.zoom(1.3, store.viewport.widthPx / 2)
      break
    case '0':
      event.preventDefault()
      store.resetToLatest()
      break
    case 'Escape':
      event.preventDefault()
      store.clearSelection()
      break
    default:
      break
  }
}

function laneLabelTop(index: number): number {
  return laneBand(layout.value, index).top
}

/** Selects the first sample of a lane so lane-level review is keyboard reachable. */
function selectLane(index: number): void {
  const lane = store.lanes[index]
  if (lane && lane.samples.length > 0) store.selectSample(lane.samples[0].sampleId)
}

function zoomIn(): void {
  store.zoom(1 / 1.3, store.viewport.widthPx / 2)
}
function zoomOut(): void {
  store.zoom(1.3, store.viewport.widthPx / 2)
}

watch(
  () => [projection.value, store.selectedSampleId, hoverSampleId.value, hoverLaneIndex.value, store.viewport],
  scheduleDraw,
  { deep: false }
)

watch(() => store.samples.length, () => {
  theme.value = resolveTimelineTheme(plotRef.value)
  scheduleDraw()
})

onMounted(() => {
  const el = plotRef.value
  if (el && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => measure())
    resizeObserver.observe(el)
  }
  measure()

  const canvas = canvasRef.value
  canvas?.addEventListener('wheel', onWheel, { passive: false })
  window.addEventListener('mouseup', onMouseUp)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  canvasRef.value?.removeEventListener('wheel', onWheel)
  window.removeEventListener('mouseup', onMouseUp)
  if (rafHandle !== null) cancelAnimationFrame(rafHandle)
})

defineExpose({ zoomIn, zoomOut })
</script>

<template>
  <div class="flex-1 flex flex-col min-h-0 bg-card">
    <!-- Toolbar -->
    <div class="flex items-center justify-between gap-3 px-3 py-1.5 border-b border-border text-[11px]">
      <div class="flex items-center gap-3 text-content-muted font-mono">
        <span>
          已加载 <span class="text-content-main font-semibold">{{ store.samples.length }}</span> 条原始样本
        </span>
        <span v-if="store.refreshGapIncomplete" class="text-amber-600 dark:text-amber-400 font-medium" data-testid="catch-up-indicator">
          历史追赶中 · history catch-up in progress
        </span>
        <span v-if="store.isPartiallyLoaded" class="text-amber-600 dark:text-amber-400">
          仅加载部分历史（还有更旧数据）
        </span>
        <span v-if="projection.visibleCount !== store.samples.length">
          视口内绘制 {{ projection.visibleCount }} 条几何
        </span>
        <span v-if="store.lanes.length > 0">
          {{ store.lanes.length }} 条 lane
        </span>
      </div>

      <div class="flex items-center gap-1.5">
        <button
          v-if="store.newSampleCount > 0"
          @click="store.resetToLatest()"
          class="px-2 py-0.5 rounded-full bg-blue-600 text-white font-medium hover:bg-blue-500 transition-colors"
          :title="`有 ${store.newSampleCount} 条新样本，回到最新`"
        >
          {{ store.newSampleCount }} 条新样本 · 回到最新
        </button>
        <button
          @click="store.resetToLatest()"
          class="px-2 py-0.5 rounded border border-border hover:border-brand hover:text-brand transition-colors"
          title="回到最新（快捷键 Home / 0）"
        >
          回到最新
        </button>
        <button
          @click="zoomIn"
          class="w-6 h-6 rounded border border-border hover:border-brand hover:text-brand transition-colors leading-none"
          title="放大（+）"
        >
          +
        </button>
        <button
          @click="zoomOut"
          class="w-6 h-6 rounded border border-border hover:border-brand hover:text-brand transition-colors leading-none"
          title="缩小（-）"
        >
          −
        </button>
      </div>
    </div>

    <!-- Plot area -->
    <div class="flex-1 flex min-h-0">
      <!-- Lane labels -->
      <div
        class="relative border-r border-border bg-card overflow-hidden"
        :style="{ width: `${LANE_LABEL_WIDTH_PX}px` }"
      >
        <button
          v-for="(lane, index) in store.lanes"
          :key="lane.key"
          @click="selectLane(index)"
          class="absolute left-0 right-0 px-2 text-left hover:bg-card transition-colors"
          :style="{ top: `${laneLabelTop(index)}px`, height: `${layout.laneHeightPx}px` }"
          :title="`${lane.displayName} · ${lane.probeType} · ${lane.target}`"
        >
          <span class="block text-[11px] font-medium text-content-main truncate leading-tight pt-0.5">
            {{ lane.displayName }}
          </span>
          <span class="block text-[10px] text-content-muted truncate leading-tight">
            {{ lane.probeType }}
          </span>
          <span class="block text-[10px] text-content-muted truncate leading-tight">
            {{ lane.target.replace(/^https?:\/\//, '') }}
          </span>
        </button>
      </div>

      <!-- Canvas -->
      <div ref="plotRef" class="relative flex-1 min-w-0">
        <canvas
          ref="canvasRef"
          tabindex="0"
          role="application"
          :aria-label="`监控时间轴，共 ${store.samples.length} 条原始样本。使用左右方向键在样本间移动，上下方向键切换 lane。`"
          class="block outline-none focus-visible:ring-1 focus-visible:ring-brand"
          @mousemove="onMouseMove"
          @mouseleave="onMouseLeave"
          @mousedown="onMouseDown"
          @click="onClick"
          @keydown="onKeyDown"
        />

        <!-- Hover tooltip: supplementary to the inspector, never the only evidence path -->
        <div
          v-if="hoverTooltip && hoverPoint"
          class="pointer-events-none absolute z-10 px-2 py-1.5 rounded border border-border bg-card shadow-sm text-[10px] font-mono leading-snug"
          :style="{
            left: `${Math.min(hoverPoint.x + 12, Math.max(0, size.width - 190))}px`,
            top: `${Math.max(0, hoverPoint.y - 52)}px`,
          }"
        >
          <div class="text-content-main font-semibold truncate max-w-[180px]">{{ hoverTooltip.title }}</div>
          <div class="text-content-muted">{{ hoverTooltip.time }}</div>
          <div class="text-content-secondary">延迟 {{ hoverTooltip.latency }} · {{ hoverTooltip.errorClass }}</div>
          <div v-if="hoverTooltip.colliding > 1" class="text-amber-600 dark:text-amber-400">
            该像素列有 {{ hoverTooltip.colliding }} 条样本，点击后可逐条切换
          </div>
        </div>

        <!-- Screen-reader announcement of the current selection -->
        <div class="sr-only" aria-live="polite">
          <template v-if="store.selectedSample">
            已选择样本 {{ store.selectedSample.sampleId }}，
            {{ formatWallClock(store.selectedSample.timestampMs, tzOffsetMinutes, true) }}，
            延迟 {{ store.selectedSample.latencyMs.toFixed(0) }} 毫秒，
            结果 {{ store.selectedSample.success ? '成功' : '失败' }}，
            错误类型 {{ store.selectedSample.errorClass }}。
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
