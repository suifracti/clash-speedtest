import { describe, expect, it } from 'vitest'
import { buildLanes } from '../lanes'
import { projectSamples, type TimelineLayout } from '../marks'
import { renderTimeline, type RenderParams } from '../render'
import { DEFAULT_TIMELINE_THEME } from '../theme'
import { buildTimeTicks } from '../time'
import { viewportForRange } from '../viewport'
import { BASE_TS, makeSample, makeSeries } from './fixtures'
import type { MonitorSample } from '../../../types'

const MIN = 60_000
const HOUR = 60 * MIN
const LAYOUT: TimelineLayout = { plotTopPx: 0, laneHeightPx: 40, laneGapPx: 4 }
const WIDTH = 600
const HEIGHT = 120
const AXIS = 22

interface FakeContext {
  arcs: number
  strokes: number
  fills: number
  fillTexts: string[]
  dashSegments: number
  ctx: CanvasRenderingContext2D
}

/** Minimal recording 2D context: enough surface to exercise the renderer deterministically. */
function createFakeContext(): FakeContext {
  const record: FakeContext = {
    arcs: 0,
    strokes: 0,
    fills: 0,
    fillTexts: [],
    dashSegments: 0,
    ctx: null as unknown as CanvasRenderingContext2D,
  }

  const noop = () => {}
  const target: Record<string, unknown> = {
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    font: '',
    textBaseline: '',
    globalAlpha: 1,
    clearRect: noop,
    fillRect: noop,
    beginPath: noop,
    closePath: noop,
    moveTo: noop,
    lineTo: noop,
    save: noop,
    restore: noop,
    stroke: () => {
      record.strokes += 1
    },
    fill: () => {
      record.fills += 1
    },
    arc: () => {
      record.arcs += 1
    },
    setLineDash: (segments: number[]) => {
      record.dashSegments += segments.length
    },
    measureText: (text: string) => ({ width: text.length * 5 }),
    fillText: (text: string) => {
      record.fillTexts.push(text)
    },
  }

  record.ctx = target as unknown as CanvasRenderingContext2D
  return record
}

function render(samples: MonitorSample[], selectedSampleId: string | null = null): FakeContext {
  const viewport = viewportForRange(BASE_TS, BASE_TS + HOUR, WIDTH)
  const projection = projectSamples({
    lanes: buildLanes(samples),
    viewport,
    layout: LAYOUT,
    latencyScaleMs: 200,
    maxStemHeightPx: 30,
  })

  const fake = createFakeContext()
  const params: RenderParams = {
    ctx: fake.ctx,
    widthPx: WIDTH,
    heightPx: HEIGHT,
    axisHeightPx: AXIS,
    projection,
    viewport,
    ticks: buildTimeTicks(viewport, { tzOffsetMinutes: 480 }),
    boundaries: [],
    selectedSampleId,
    hoverSampleId: null,
    theme: DEFAULT_TIMELINE_THEME,
    hoverLaneIndex: null,
  }

  renderTimeline(params)
  return { ...fake, arcs: fake.arcs }
}

/** Every glyph except the transport-failure cross emits exactly one arc. */
function expectedArcCount(samples: MonitorSample[]): number {
  return samples.filter((s) => !(s.success === false && s.errorClass === 'timeout')).length
}

describe('timeline rendering', () => {
  it('draws one glyph per sample with no aggregation', () => {
    const samples = makeSeries(40, { sampleId: 'r' }, BASE_TS, 60_000)
    const fake = render(samples)

    expect(samples).toHaveLength(40)
    expect(fake.arcs).toBe(expectedArcCount(samples))
  })

  it('keeps drawing every sample even when they collide into a few pixel columns', () => {
    // 3000 samples across 600px → ~5 samples per column.
    const samples = makeSeries(3000, { sampleId: 'dense' }, BASE_TS, 1000)
    const fake = render(samples)

    expect(fake.arcs).toBe(expectedArcCount(samples))
  })

  it('renders a mix of success and failure glyphs without dropping any', () => {
    const samples = [
      makeSample({ sampleId: 'ok1', success: true, errorClass: 'none' }),
      makeSample({ sampleId: 'ok2', success: true, errorClass: 'none' }),
      makeSample({ sampleId: 'to1', success: false, errorClass: 'timeout' }),
      makeSample({ sampleId: 'bl1', success: false, errorClass: 'blocked' }),
      makeSample({ sampleId: 'un1', success: false, errorClass: 'weird_future_class' }),
    ]
    const fake = render(samples)

    // timeout draws a cross (no arc); the other four each draw one arc.
    expect(fake.arcs).toBe(4)
  })

  it('draws axis tick labels', () => {
    const fake = render(makeSeries(10, { sampleId: 'ax' }, BASE_TS, 60_000))
    expect(fake.fillTexts.length).toBeGreaterThan(0)
  })

  it('adds a selection guide for the selected sample only', () => {
    const samples = makeSeries(5, { sampleId: 'sel' }, BASE_TS, 60_000)
    const without = render(samples, null)
    const withSelection = render(samples, samples[2].sampleId)

    expect(withSelection.strokes).toBeGreaterThan(without.strokes)
    expect(withSelection.arcs).toBeGreaterThan(without.arcs)
  })

  it('does not throw on an empty dataset', () => {
    expect(() => render([])).not.toThrow()
  })

  it('renders only in-viewport geometry while the dataset stays complete', () => {
    const inView = makeSeries(5, { sampleId: 'v' }, BASE_TS + MIN, MIN)
    const outside = makeSeries(50, { sampleId: 'o' }, BASE_TS - 100 * HOUR, MIN)
    const fake = render([...inView, ...outside])

    expect(fake.arcs).toBe(5)
  })
})
