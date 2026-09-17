/**
 * Render-evidence harness for the monitor timeline.
 *
 * Purpose: let a reviewer *see* the visual grammar without needing a live 24/7 monitor history.
 * It drives the real production modules — `lanes`, `marks`, `encoding`, `time`, `theme`, `render` —
 * with deterministic synthetic samples, and never touches the app bundle (this file lives outside
 * `src/`, so `vite build` does not pick it up).
 *
 * It is deliberately NOT a substitute for real history: every sample below is generated, and the
 * page is watermarked as such. The point is to verify the glyph semantics and the
 * "one glyph per raw sample" invariant at a glance, which the unit tests assert numerically.
 *
 * How to view it:
 *
 *   cd frontend && npm run dev
 *   # then open http://localhost:5173/evidence/
 *
 * Screenshot it headlessly with:
 *
 *   chrome --headless=new --disable-gpu --hide-scrollbars \
 *     --force-device-scale-factor=1.6 --virtual-time-budget=9000 \
 *     --window-size=1330,940 --screenshot=evidence/timeline-render-evidence.png \
 *     http://localhost:5173/evidence/
 *
 * Note: this file lives outside `src/`, so `tsconfig.json` does not type-check it and
 * `vite build` does not bundle it. It is a development tool, not shipped code.
 */

import type { MonitorSample } from '../src/types'
import { computeLatencyScale } from '../src/utils/timeline/encoding'
import { buildLanes, detectRevisionBoundaries } from '../src/utils/timeline/lanes'
import { projectSamples, laneBand, type TimelineLayout } from '../src/utils/timeline/marks'
import { renderTimeline } from '../src/utils/timeline/render'
import { resolveTimelineTheme } from '../src/utils/timeline/theme'
import { buildTimeTicks, hostTzOffsetMinutes, MS_HOUR, MS_MINUTE } from '../src/utils/timeline/time'
import { viewportForRange } from '../src/utils/timeline/viewport'

const AXIS_HEIGHT_PX = 24
const PLOT_TOP_PX = 8
const LANE_HEIGHT_PX = 56
const LANE_GAP_PX = 4

/** Deterministic PRNG so the evidence image is reproducible. */
function mulberry32(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

interface LaneSpec {
  nodeIdentityKey: string
  nodeKey: string
  displayName: string
  probeType: string
  target: string
  configRevisionKey: string
}

function makeSample(
  lane: LaneSpec,
  id: string,
  timestampMs: number,
  outcome: { success: boolean; errorClass: string; latencyMs: number }
): MonitorSample {
  return {
    sampleId: id,
    runId: 'run_evidence',
    nodeKey: lane.nodeKey,
    nodeIdentityKey: lane.nodeIdentityKey,
    configRevisionKey: lane.configRevisionKey,
    profileId: 'prof_main',
    displayNameSnapshot: lane.displayName,
    probeType: lane.probeType,
    target: lane.target,
    timestampMs,
    timestampIso: new Date(timestampMs).toISOString(),
    success: outcome.success,
    latencyMs: outcome.latencyMs,
    ttfbMs: outcome.latencyMs * 0.7,
    errorClass: outcome.errorClass,
  }
}

function renderScenario(
  host: HTMLElement,
  title: string,
  subtitle: string,
  samples: MonitorSample[],
  startMs: number,
  endMs: number,
  widthPx: number,
  selectedSampleId: string | null = null
): void {
  const lanes = buildLanes(samples)
  const layout: TimelineLayout = {
    plotTopPx: PLOT_TOP_PX,
    laneHeightPx: LANE_HEIGHT_PX,
    laneGapPx: LANE_GAP_PX,
  }
  const heightPx = PLOT_TOP_PX + Math.max(1, lanes.length) * (LANE_HEIGHT_PX + LANE_GAP_PX) + AXIS_HEIGHT_PX
  const viewport = viewportForRange(startMs, endMs, widthPx)
  const projection = projectSamples({
    lanes,
    viewport,
    layout,
    latencyScaleMs: computeLatencyScale(samples),
    maxStemHeightPx: LANE_HEIGHT_PX - 12,
  })

  const section = document.createElement('section')
  section.className = 'scenario'

  const caption = document.createElement('div')
  caption.className = 'caption'
  caption.innerHTML =
    `<span class="title">${title}</span>` +
    `<span>${subtitle}</span>` +
    `<span class="num">已加载 ${samples.length} 条原始样本 · 生成图元 ${projection.totalCount} 条 · 视口内绘制 ${projection.visibleCount} 条</span>` +
    `<span class="num">lane ${lanes.length}</span>`
  section.appendChild(caption)

  const card = document.createElement('div')
  card.className = 'card'

  // Lane label column. `renderTimeline` only draws the plot; the app renders these labels in
  // the Vue component, so the harness reproduces them to keep the evidence readable.
  const labelCol = document.createElement('div')
  labelCol.className = 'lane-labels'
  labelCol.style.height = `${heightPx}px`
  lanes.forEach((lane, index) => {
    const band = laneBand(layout, index)
    const label = document.createElement('div')
    label.className = 'lane-label'
    label.style.top = `${band.top}px`
    label.style.height = `${layout.laneHeightPx}px`
    label.innerHTML =
      `<b>${lane.displayName}</b>` +
      `<span>${lane.probeType}</span>` +
      `<span>${lane.target.replace(/^https?:\/\//, '')}</span>`
    labelCol.appendChild(label)
  })
  card.appendChild(labelCol)

  const canvas = document.createElement('canvas')
  const dpr = window.devicePixelRatio || 1
  canvas.width = Math.floor(widthPx * dpr)
  canvas.height = Math.floor(heightPx * dpr)
  canvas.style.width = `${widthPx}px`
  canvas.style.height = `${heightPx}px`
  card.appendChild(canvas)
  section.appendChild(card)
  host.appendChild(section)

  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

  renderTimeline({
    ctx,
    widthPx,
    heightPx,
    axisHeightPx: AXIS_HEIGHT_PX,
    projection,
    viewport,
    ticks: buildTimeTicks(viewport, { tzOffsetMinutes: hostTzOffsetMinutes() }),
    boundaries: detectRevisionBoundaries(samples),
    selectedSampleId,
    hoverSampleId: null,
    theme: resolveTimelineTheme(document.body),
    hoverLaneIndex: null,
  })
}

const host = document.getElementById('scenarios')!
const NOW = Date.UTC(2026, 8, 17, 12, 0, 0)
const WIDTH = 1080

// ---------------------------------------------------------------------------
// Scenario A: three lanes over 6 hours, showing the outcome families apart.
// The Google lane is *service-blocked* while its transport stays healthy — the
// exact case that must never be rendered as "the node is down".
// ---------------------------------------------------------------------------
{
  const jp01: LaneSpec = {
    nodeIdentityKey: 'nid_jp01',
    nodeKey: 'nk_jp01',
    displayName: 'JP01 东京',
    probeType: 'Connectivity',
    target: 'https://cp.cloudflare.com/generate_204',
    configRevisionKey: 'rev_a1',
  }
  const google: LaneSpec = {
    ...jp01,
    probeType: 'Google',
    target: 'https://www.google.com/generate_204',
  }
  const ai: LaneSpec = {
    ...jp01,
    probeType: 'AI',
    target: 'https://api.openai.com/v1/models',
  }

  const rnd = mulberry32(20260917)
  const samples: MonitorSample[] = []
  const start = NOW - 6 * MS_HOUR

  for (let i = 0; i < 360; i += 1) {
    // One sample per minute over the 6 hour window.
    const t = start + i * MS_MINUTE
    // Connectivity: healthy, with a transport outage window (timeouts).
    const outage = i >= 120 && i < 132
    samples.push(
      makeSample(jp01, `a-c-${i}`, t, {
        success: !outage,
        errorClass: outage ? 'timeout' : 'none',
        latencyMs: outage ? 0 : 38 + rnd() * 30,
      })
    )
    // Google: transport fine, but every request is region-blocked.
    const blocked = i >= 60 && i < 200
    samples.push(
      makeSample(google, `a-g-${i}`, t, {
        success: !blocked,
        errorClass: blocked ? 'blocked' : 'none',
        latencyMs: blocked ? 0 : 120 + rnd() * 60,
      })
    )
    // AI: mostly healthy, one unclassified failure to exercise the hollow glyph.
    const unknown = i === 250 || i === 251
    samples.push(
      makeSample(ai, `a-ai-${i}`, t, {
        success: !unknown,
        errorClass: unknown ? 'weird_proxy_error' : 'none',
        latencyMs: unknown ? 0 : 180 + rnd() * 90,
      })
    )
  }

  renderScenario(
    host,
    'A · 混合结果与 lane 分离（6 小时）',
    'JP01 的 Connectivity 在 02:00–02:12 出现传输超时；同节点 Google 在 01:00–03:20 被地区阻断（服务层失败，传输仍健康）；AI 出现 2 条未归类失败。',
    samples,
    NOW - 6 * MS_HOUR,
    NOW,
    WIDTH
  )
}

// ---------------------------------------------------------------------------
// Scenario B: pixel collision. 3000 raw samples in one lane inside a 20 minute
// window at 1080px. Marks overlap heavily but none is dropped: totalCount stays
// 3000 and visibleCount stays 3000.
// ---------------------------------------------------------------------------
{
  const lane: LaneSpec = {
    nodeIdentityKey: 'nid_sg02',
    nodeKey: 'nk_sg02',
    displayName: 'SG02 新加坡',
    probeType: 'Heavy',
    target: 'https://speed.cloudflare.com/__down?bytes=10000000',
    configRevisionKey: 'rev_b1',
  }

  const rnd = mulberry32(777)
  const samples: MonitorSample[] = []
  const span = 20 * MS_MINUTE
  const start = NOW - span

  for (let i = 0; i < 3000; i += 1) {
    const t = start + Math.floor((i / 3000) * span)
    const roll = rnd()
    const failed = roll > 0.94
    const serviceFailed = roll > 0.90 && roll <= 0.94
    samples.push(
      makeSample(lane, `b-${i}`, t, {
        success: !failed && !serviceFailed,
        errorClass: failed ? 'conn_refused' : serviceFailed ? 'http_status_error' : 'none',
        latencyMs: failed || serviceFailed ? 0 : 60 + rnd() * 200,
      })
    )
  }

  renderScenario(
    host,
    'B · 像素碰撞下的逐条可寻址（20 分钟 / 3000 条）',
    '1080px 宽内 3000 条样本必然重叠。压缩的是像素，不是事实：图元数仍等于样本数，碰撞组可通过点击逐条切换。',
    samples,
    NOW - span,
    NOW,
    WIDTH
  )
}

// ---------------------------------------------------------------------------
// Scenario C: config revision boundary. Same node identity, revision changes
// mid-window, plus a selection guide to show the inspector anchor.
// ---------------------------------------------------------------------------
{
  const before: LaneSpec = {
    nodeIdentityKey: 'nid_hk03',
    nodeKey: 'nk_hk03',
    displayName: 'HK03 香港',
    probeType: 'Service',
    target: 'https://www.youtube.com/generate_204',
    configRevisionKey: 'rev_c1_before',
  }
  const after: LaneSpec = { ...before, configRevisionKey: 'rev_c1_after' }

  const rnd = mulberry32(31337)
  const samples: MonitorSample[] = []
  const span = 2 * MS_HOUR
  const start = NOW - span
  const cutover = start + span / 2

  for (let i = 0; i < 240; i += 1) {
    const t = start + i * (30 * MS_MINUTE / 60)
    const lane = t < cutover ? before : after
    const worse = t >= cutover && rnd() > 0.7
    samples.push(
      makeSample(lane, `c-${i}`, t, {
        success: !worse,
        errorClass: worse ? 'tls_error' : 'none',
        latencyMs: worse ? 0 : (t < cutover ? 45 : 95) + rnd() * 25,
      })
    )
  }

  renderScenario(
    host,
    'C · 配置修订边界（2 小时）',
    '同一 NodeIdentityKey 下 ConfigRevisionKey 在中点切换，时间轴留下虚线边界与三角标记；选中样本显示选择参考线（Inspector 的锚点）。',
    samples,
    NOW - span,
    NOW,
    WIDTH,
    samples[180].sampleId
  )
}

// Expose the projection counts so the screenshot can be cross-checked.
;(window as unknown as { __evidenceReady: boolean }).__evidenceReady = true
