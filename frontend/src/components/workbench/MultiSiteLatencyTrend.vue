<script setup lang="ts">
import { computed, ref } from 'vue'
import type { WorkbenchLatencyTest } from '../../types'
import { latencyTargets } from '../../utils/latencyTargets'
import { computeNodeHealthReport } from './nodeHealth'

const props = defineProps<{ tests: WorkbenchLatencyTest[]; since: string; until: string }>()
const selected = ref('all')
const hoveredAttempt = ref('')
const pinned = ref(false)
const sampleIndex = ref(-1)
const anchor = ref({ left: '8%', top: '45px', transform: 'translateX(0)' })
function placeReadout(time: number, median: number | null, svg: SVGSVGElement) {
  const box = svg.getBoundingClientRect()
  const px = x(time) / 900 * box.width
  const py = (median === null ? 153 : y(median)) / 196 * box.height
  anchor.value = { left: `${px}px`, top: `${box.top - svg.parentElement!.getBoundingClientRect().top + py}px`, transform: px > box.width / 2 ? 'translate(calc(-100% - 12px), 8px)' : 'translate(12px, 8px)' }
}
function focusPoint(event: FocusEvent, point: {id: string; time: number; median: number | null}) {
  hoveredAttempt.value = point.id
  placeReadout(point.time, point.median, (event.target as SVGElement).ownerSVGElement!)
}
const colors = ['#237a69', '#366ed4', '#9862bf', '#b47518', '#cc5267', '#008799']
const series = computed(() => latencyTargets.map((target, index) => ({ ...target, color: colors[index], points: props.tests.flatMap(test => {
  const samples = test.samples.filter(sample => (sample.target || test.target || latencyTargets[0]!.url) === target.url && Date.parse(sample.timestamp) >= Date.parse(props.since) && Date.parse(sample.timestamp) < Date.parse(props.until))
  if (!samples.length) return []
  const good = samples.filter(sample => sample.success).map(sample => sample.latency_ms).sort((a, b) => a - b)
  const middle = Math.floor(good.length / 2)
  const median = good.length ? good.length % 2 ? good[middle]! : (good[middle - 1]! + good[middle]!) / 2 : null
  const time = Math.max(...samples.map(sample => Date.parse(sample.timestamp)))
  return [{ id: test.attempt_id, time, median, failed: samples.length - good.length, label: `${target.label} · ${new Date(time).toLocaleString()} · ${median === null ? '失败' : `中位 ${Math.round(median)} ms`} · 成功 ${good.length}/${samples.length}${samples.some(sample => sample.error) ? ` · ${[...new Set(samples.filter(sample => sample.error).map(sample => sample.error))].join('；')}` : ''}` }]
}).sort((a, b) => a.time - b.time) })))
const visible = computed(() => series.value.filter(site => selected.value === 'all' || site.value === selected.value))
const hoveredPoints = computed(() => visible.value.flatMap(site => site.points.filter(point => point.id === hoveredAttempt.value).map(point => ({ ...point, siteLabel: site.label, color: site.color }))))
const siteHealth = computed(() => latencyTargets.filter(site => selected.value === 'all' || selected.value === site.value).map(site => ({ ...site, health: computeNodeHealthReport(props.tests.flatMap(test => test.samples.filter(sample => (sample.target || test.target || latencyTargets[0]!.url) === site.url && Date.parse(sample.timestamp) >= Date.parse(props.since) && Date.parse(sample.timestamp) < Date.parse(props.until)))) })))
const hoveredSamples = computed(() => (props.tests.find(test => test.attempt_id === hoveredAttempt.value)?.samples || []).filter(sample => Date.parse(sample.timestamp) >= Date.parse(props.since) && Date.parse(sample.timestamp) < Date.parse(props.until) && (selected.value === 'all' || sample.target === latencyTargets.find(site => site.value === selected.value)?.url)))
const currentSample = computed(() => hoveredSamples.value[sampleIndex.value])
function hover(event: MouseEvent) {
  if (pinned.value) return
  const bounds = (event.currentTarget as SVGSVGElement).getBoundingClientRect()
  const position = (event.clientX - bounds.left) / Math.max(1, bounds.width) * 900
  const vertical = (event.clientY - bounds.top) / Math.max(1, bounds.height) * 196
  const points = visible.value.flatMap(site => site.points)
  const distance = (point: typeof points[number]) => Math.hypot(x(point.time) - position, (point.median === null ? 153 : y(point.median)) - vertical)
  const nearest = points.reduce<typeof points[number] | undefined>((best, point) => !best || distance(point) < distance(best) ? point : best, undefined)
  if (hoveredAttempt.value !== nearest?.id) sampleIndex.value = -1
  hoveredAttempt.value = nearest?.id || ''
  if (nearest) placeReadout(nearest.time, nearest.median, event.currentTarget as SVGSVGElement)
}
function leave() { if (!pinned.value) { hoveredAttempt.value = ''; sampleIndex.value = -1 } }
function cycleSample(event: WheelEvent) {
  if (!event.shiftKey || !hoveredSamples.value.length) return
  event.preventDefault()
  sampleIndex.value = (sampleIndex.value + (event.deltaY >= 0 ? 1 : -1) + hoveredSamples.value.length) % hoveredSamples.value.length
}
const maximum = computed(() => Math.max(100, Math.ceil(Math.max(0, ...visible.value.flatMap(site => site.points.map(point => point.median || 0))) / 100) * 100))
function x(time: number) { return 55 + Math.max(0, Math.min(1, (time - Date.parse(props.since)) / Math.max(1, Date.parse(props.until) - Date.parse(props.since)))) * 815 }
function y(value: number) { return 128 - value / maximum.value * 108 }
function path(points: typeof series.value[number]['points']) {
  let connected = false
  return points.map(point => {
    if (point.median === null) { connected = false; return '' }
    const segment = `${connected ? 'L' : 'M'}${x(point.time)},${y(point.median)}`
    connected = true
    return segment
  }).join(' ')
}
</script>

<template>
  <div class="multi-site-trend" @mouseleave="leave">
    <div class="multi-site-legend" aria-label="趋势图站点筛选">
      <button type="button" :aria-pressed="selected === 'all'" @click="selected = 'all'">全部站点</button>
      <button v-for="site in series" :key="site.value" type="button" :aria-pressed="selected === site.value" @click="selected = selected === site.value ? 'all' : site.value"><i :style="{ background: site.color }" />{{ site.label }}</button>
      <span>悬浮读数 · 点击固定 · Shift＋滚轮查看样本 · ×＝该轮有失败</span>
    </div>
    <svg viewBox="0 0 900 196" preserveAspectRatio="none" role="img" aria-label="六站延迟趋势，失败显示在独立失败区，不计为零延迟" @mousemove="hover" @click="hover($event); pinned = !pinned" @wheel="cycleSample" @keydown.esc="pinned = false; leave()">
      <g v-for="fraction in [0, 0.5, 1]" :key="fraction">
        <line x1="55" x2="870" :y1="y(maximum * fraction)" :y2="y(maximum * fraction)" stroke="#e1e6ed" />
        <text x="48" :y="y(maximum * fraction) + 4" text-anchor="end">{{ Math.round(maximum * fraction) }}</text>
      </g>
      <text x="8" y="13">ms</text><text x="8" y="159">失败</text>
      <rect x="55" y="143" width="815" height="25" rx="4" fill="#fbf0ee" />
      <line v-if="hoveredPoints.length" :x1="x(hoveredPoints[0]!.time)" :x2="x(hoveredPoints[0]!.time)" y1="20" y2="168" stroke="#859394" stroke-dasharray="3 3" />
      <g v-for="(site, index) in visible" :key="site.value" :stroke="site.color" :fill="site.color">
        <path :d="path(site.points)" fill="none" stroke-width="1.8" />
        <g v-for="point in site.points" :key="point.id">
          <circle v-if="point.median !== null" :cx="x(point.time)" :cy="y(point.median)" r="3.5" :fill="point.failed ? 'white' : site.color" tabindex="0" @focus="focusPoint($event, point)"><title>{{ point.label }}</title></circle>
          <path v-if="point.failed" :d="`M${x(point.time)-3},${147+index*3}l6,6m0,-6l-6,6`" stroke-width="1.5" tabindex="0"><title>{{ point.label }}</title></path>
        </g>
      </g>
      <text v-if="!visible.some(site => site.points.length)" x="460" y="82" text-anchor="middle">暂无记录；开始检测后显示各站结果</text>
      <text x="55" y="189">{{ new Date(since).toLocaleString() }}</text><text x="870" y="189" text-anchor="end">{{ new Date(until).toLocaleString() }}</text>
    </svg>
    <div v-if="hoveredPoints.length" class="trend-readout" :class="{ pinned }" :style="anchor" role="status">
      <strong>{{ pinned ? '已固定读数' : '本轮各站读数' }}</strong>
      <button v-if="pinned" type="button" @click="pinned = false; leave()">取消固定</button>
      <small>{{ new Date(hoveredPoints[0]!.time).toLocaleString() }}</small>
      <div v-for="point in hoveredPoints" :key="point.label" :style="{ borderLeftColor: point.color }" class="tooltip-site">
        <b>{{ point.siteLabel }}</b><span>{{ point.median === null ? '无成功样本' : `中位 ${Math.round(point.median)} ms` }}</span><strong :class="{ 'has-failure': point.failed }">{{ point.failed ? `失败 ${point.failed} 次` : '全部成功' }}</strong>
        <details v-if="point.failed"><summary>错误详情</summary>{{ point.label }}</details>
      </div>
      <p v-if="currentSample">样本 {{ sampleIndex + 1 }}/{{ hoveredSamples.length }} · {{ latencyTargets.find(site => site.url === currentSample?.target)?.label || 'Cloudflare' }} · {{ new Date(currentSample.timestamp).toLocaleString() }} · {{ currentSample.success ? `${currentSample.latency_ms} ms` : `失败：${currentSample.error || '请求失败'}` }}</p>
    </div>
    <details class="advanced-sites-details">
      <summary class="advanced-sites-summary">
        <span>📊 展开分站进阶直方图与各区间统计</span>
      </summary>
      <section class="sites-history" aria-label="分站历史统计">
        <header><strong>{{ selected === 'all' ? '六站历史统计' : `${latencyTargets.find(site => site.value === selected)?.label} 历史统计` }}</strong><small>跟随上方站点选择 · 当前时间范围 · 延迟按站独立统计</small></header>
        <div class="history-table-wrap"><table>
          <thead><tr><th>站点</th><th>成功 / 请求</th><th>成功率</th><th>常见延迟</th><th>较慢时 P95</th><th>延迟区间次数</th></tr></thead>
          <tbody><tr v-for="site in siteHealth" :key="site.value"><th>{{ site.label }}</th>
            <template v-if="site.health"><td>{{ site.health.successCount }} / {{ site.health.sampleCount }}<small v-if="site.health.failCount" class="has-failure">失败 {{ site.health.failCount }} 次</small></td><td>{{ site.health.successRateText }}</td><td>{{ site.health.normalRangeText }}</td><td>{{ site.health.p95 === null ? '—' : `${site.health.p95} ms` }}</td><td><div class="site-bins"><span v-for="bin in site.health.histogramBins" :key="bin.label" :title="`${bin.label}：${bin.count} 次（${bin.percentage}%）`">{{ bin.label }}<b>{{ bin.count }} 次</b><i :style="{ width: `${bin.percentage}%` }" /></span><small v-if="!site.health.histogramBins.length">无成功样本</small></div></td></template>
            <td v-else colspan="5">当前范围未检测</td>
          </tr></tbody>
        </table></div>
      </section>
    </details>
  </div>
</template>

<style scoped>
.multi-site-trend { margin: 12px 0; position:relative; }
.trend-readout { position:absolute; z-index:5; width:min(420px, 80%); max-height:360px; overflow:auto; pointer-events:auto; padding:12px; border:1px solid var(--border); border-radius:8px; background:var(--card-bg); box-shadow:var(--shadow-sm); font-size:12px; overflow-wrap:anywhere; }
.trend-readout>small { display:block; margin:6px 0; color:var(--text-secondary); }
.tooltip-site { display:grid; grid-template-columns:1fr 1fr auto; gap:6px; align-items:center; }
.tooltip-site details { grid-column:1/-1; font-size:11px; }.tooltip-site summary { cursor:pointer; }
.has-failure { color:var(--danger, #b74437); }
.advanced-sites-details { margin-top: 14px; border-top: 1px dashed var(--border); padding-top: 10px; }
.advanced-sites-summary { font-size: 11px; color: var(--text-secondary); cursor: pointer; user-select: none; padding: 4px 8px; border-radius: 6px; width: fit-content; transition: background 0.15s; }
.advanced-sites-summary:hover { background: var(--card-subtle); color: var(--primary); }
.sites-history { border-top:1px solid var(--border); margin-top:10px; padding-top:10px; font-size:12px; }
.sites-history header { display:flex; gap:12px; align-items:center; margin-bottom:10px; }.sites-history small { color:var(--text-secondary); }
.history-table-wrap { overflow-x:auto; }.sites-history table { width:100%; border-collapse:collapse; text-align:left; }
.sites-history td,.sites-history th { padding:9px 8px; border-bottom:1px solid var(--border); }.sites-history td small { display:block; }
.site-bins { display:flex; gap:8px; min-width:300px; }.site-bins span { flex:1; position:relative; padding-bottom:5px; font-size:10px; }.site-bins b { display:block; font-weight:500; }.site-bins i { position:absolute; bottom:0; left:0; height:3px; background:var(--primary); }
.trend-readout.pinned { pointer-events:auto; }
.trend-readout div { border-left:3px solid; padding:4px 8px; margin-top:4px; }
.trend-readout button { float:right; color:var(--primary); }
.multi-site-legend { display:flex; flex-wrap:wrap; gap:6px; align-items:center; font-size:11px; }
.multi-site-legend button { border:1px solid #dce4ea; border-radius:6px; background:white; padding:4px 8px; cursor:pointer; }
.multi-site-legend button[aria-pressed=true] { background:#edf4f6; border-color:#64978f; }
.multi-site-legend i { display:inline-block; width:8px; height:8px; border-radius:50%; margin-right:5px; }
.multi-site-legend span { color:#687b8c; margin-left:auto; }
svg { width:100%; min-height:145px; max-height:220px; display:block; }
svg text { fill:#687b8c; font-size:11px; }
</style>
