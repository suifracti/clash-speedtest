import { describe, expect, it } from 'vitest'
import { buildLanes } from '../lanes'
import {
  hitTestMarks,
  laneBand,
  laneIndexAtY,
  markIndexAtOrAfter,
  pickNearestMark,
  projectSamples,
  stepMarkInLane,
  type TimelineLayout,
  type TimelineProjection,
} from '../marks'
import { viewportForRange, type TimelineViewport } from '../viewport'
import { BASE_TS, makeSample, makeSeries } from './fixtures'

const MIN = 60_000
const HOUR = 60 * MIN

const LAYOUT: TimelineLayout = { plotTopPx: 0, laneHeightPx: 40, laneGapPx: 4 }

function project(
  samples: Parameters<typeof buildLanes>[0],
  viewport: TimelineViewport,
  layout: TimelineLayout = LAYOUT
): TimelineProjection {
  return projectSamples({
    lanes: buildLanes(samples),
    viewport,
    layout,
    latencyScaleMs: 200,
    maxStemHeightPx: 30,
  })
}

function oneHourViewport(widthPx = 600): TimelineViewport {
  return viewportForRange(BASE_TS, BASE_TS + HOUR, widthPx)
}

describe('sample projection', () => {
  it('produces exactly one mark per in-viewport sample (no aggregation)', () => {
    const samples = makeSeries(50)
    const projection = project(samples, oneHourViewport())

    expect(projection.marks).toHaveLength(50)
    expect(projection.totalCount).toBe(50)
    expect(projection.visibleCount).toBe(50)

    const ids = new Set(projection.marks.map((m) => m.sample.sampleId))
    expect(ids.size).toBe(50)
  })

  it('maps a timestamp onto the expected pixel column', () => {
    const samples = [makeSample({ timestampMs: BASE_TS + HOUR / 2, sampleId: 'mid' })]
    const projection = project(samples, oneHourViewport(600))
    expect(projection.marks[0].x).toBeCloseTo(300, 6)
    expect(projection.marks[0].column).toBe(300)
  })

  it('culls samples outside the viewport from geometry but keeps them in the loaded total', () => {
    const inView = makeSeries(3, { sampleId: 'in' }, BASE_TS + MIN, MIN)
    const before = makeSeries(4, { sampleId: 'before' }, BASE_TS - 10 * HOUR, MIN)
    const after = makeSeries(4, { sampleId: 'after' }, BASE_TS + 10 * HOUR, MIN)
    const all = [...before, ...inView, ...after]

    const projection = project(all, oneHourViewport())

    expect(projection.totalCount).toBe(11)
    expect(projection.visibleCount).toBe(3)
    expect(projection.marks).toHaveLength(3)
  })

  it('keeps marks in their own lane', () => {
    const samples = [
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'rtt', sampleId: 'a1' }),
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'ttfb', sampleId: 'a2' }),
    ]
    const projection = project(samples, oneHourViewport())
    const laneIndexes = projection.marks.map((m) => m.laneIndex).sort()
    expect(laneIndexes).toEqual([0, 1])
  })

  it('assigns a fixed failure height instead of implying a latency value', () => {
    const samples = [
      makeSample({ success: false, errorClass: 'timeout', latencyMs: 0, sampleId: 'f1' }),
    ]
    const projection = project(samples, oneHourViewport())
    expect(projection.marks[0].semantic.outcome).toBe('transport_failure')
    expect(projection.marks[0].stemHeight).toBe(8)
  })

  it('scales a successful mark height with latency', () => {
    const slow = project([makeSample({ latencyMs: 400, sampleId: 's1' })], oneHourViewport())
    const fast = project([makeSample({ latencyMs: 10, sampleId: 's2' })], oneHourViewport())
    expect(slow.marks[0].stemHeight).toBeGreaterThan(fast.marks[0].stemHeight)
  })
})

describe('individually addressable samples', () => {
  it('keeps multiple samples at the identical timestamp separately addressable', () => {
    const samples = [
      makeSample({ timestampMs: BASE_TS + MIN, sampleId: 'same_1', probeType: 'probe_1' }),
      makeSample({ timestampMs: BASE_TS + MIN, sampleId: 'same_2', probeType: 'probe_2' }),
      makeSample({ timestampMs: BASE_TS + MIN, sampleId: 'same_3', probeType: 'probe_3' }),
    ]

    // Force them into one lane so the collision is real.
    const lane = buildLanes(samples.map((s) => ({ ...s, probeType: 'rtt', target: 'https://t/x' })))
    const projection = projectSamples({
      lanes: lane,
      viewport: oneHourViewport(),
      layout: LAYOUT,
      latencyScaleMs: 200,
      maxStemHeightPx: 30,
    })

    expect(projection.marks).toHaveLength(3)
    const columns = new Set(projection.marks.map((m) => m.column))
    expect(columns.size).toBe(1)

    for (const mark of projection.marks) {
      expect(mark.coincidentCount).toBe(3)
    }
    expect(projection.marks.map((m) => m.coincidentIndex).sort()).toEqual([0, 1, 2])
  })

  it('returns every colliding sample from a hit test, not just one', () => {
    const base = makeSample({ timestampMs: BASE_TS + MIN, probeType: 'rtt', target: 'https://t/x' })
    const samples = [
      { ...base, sampleId: 'h1' },
      { ...base, sampleId: 'h2' },
      { ...base, sampleId: 'h3' },
    ]
    const projection = project(samples, oneHourViewport())
    const x = projection.marks[0].x
    const hits = hitTestMarks(projection, x, 10)

    expect(hits).toHaveLength(3)
    const hitIds = hits.map((i) => projection.marks[i].sample.sampleId).sort()
    expect(hitIds).toEqual(['h1', 'h2', 'h3'])
  })

  it('finds a mark with a small pointer tolerance (nearest-hit)', () => {
    const samples = [makeSample({ timestampMs: BASE_TS + MIN, sampleId: 'n1' })]
    const projection = project(samples, oneHourViewport())
    const x = projection.marks[0].x
    expect(pickNearestMark(projection, x + 1, 10)).not.toBeNull()
    expect(pickNearestMark(projection, x - 2, 10)).not.toBeNull()
  })

  it('does not return marks from a different lane', () => {
    const samples = [
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'rtt', sampleId: 'l1' }),
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'ttfb', sampleId: 'l2' }),
    ]
    const projection = project(samples, oneHourViewport())
    const lane0Mark = projection.marks.find((m) => m.laneIndex === 0)!
    // y inside lane 1 must not surface lane 0's mark.
    const hits = hitTestMarks(projection, lane0Mark.x, LAYOUT.laneHeightPx + LAYOUT.laneGapPx + 5)
    for (const index of hits) {
      expect(projection.marks[index].laneIndex).toBe(1)
    }
  })

  it('isolates multi-lane hits when multiple lanes share the exact same timestamp', () => {
    // 3 distinct lanes with samples at the exact same timestamp.
    const sharedTs = BASE_TS + 15 * MIN
    const samples = [
      makeSample({ displayNameSnapshot: 'Lane 0', nodeIdentityKey: 'nid_0', probeType: 'rtt', target: 'https://t/1', sampleId: 'lane0_s', timestampMs: sharedTs }),
      makeSample({ displayNameSnapshot: 'Lane 1', nodeIdentityKey: 'nid_1', probeType: 'rtt', target: 'https://t/1', sampleId: 'lane1_s', timestampMs: sharedTs }),
      makeSample({ displayNameSnapshot: 'Lane 2', nodeIdentityKey: 'nid_2', probeType: 'rtt', target: 'https://t/1', sampleId: 'lane2_s', timestampMs: sharedTs }),
    ]
    const projection = project(samples, oneHourViewport())
    expect(projection.lanes).toHaveLength(3)

    // All 3 marks share the exact same X coordinate.
    const sharedX = projection.marks[0].x
    expect(projection.marks[1].x).toBe(sharedX)
    expect(projection.marks[2].x).toBe(sharedX)

    // Lane 0 hit-test: pointer Y within Lane 0.
    const y0 = LAYOUT.laneHeightPx / 2
    const hits0 = hitTestMarks(projection, sharedX, y0)
    expect(hits0).toHaveLength(1)
    expect(projection.marks[hits0[0]].sample.sampleId).toBe('lane0_s')
    expect(projection.marks[hits0[0]].laneIndex).toBe(0)

    // Lane 1 hit-test: pointer Y within Lane 1.
    const y1 = LAYOUT.laneHeightPx + LAYOUT.laneGapPx + LAYOUT.laneHeightPx / 2
    const hits1 = hitTestMarks(projection, sharedX, y1)
    expect(hits1).toHaveLength(1)
    expect(projection.marks[hits1[0]].sample.sampleId).toBe('lane1_s')
    expect(projection.marks[hits1[0]].laneIndex).toBe(1)

    // Lane 2 hit-test: pointer Y within Lane 2.
    const y2 = (LAYOUT.laneHeightPx + LAYOUT.laneGapPx) * 2 + LAYOUT.laneHeightPx / 2
    const hits2 = hitTestMarks(projection, sharedX, y2)
    expect(hits2).toHaveLength(1)
    expect(projection.marks[hits2[0]].sample.sampleId).toBe('lane2_s')
    expect(projection.marks[hits2[0]].laneIndex).toBe(2)
  })

  it('deterministically tie-breaks multiple samples in the same lane with the exact same timestamp', () => {
    // 3 samples in the same lane with the identical timestamp and X, provided in arbitrary ID order.
    const sharedTs = BASE_TS + 20 * MIN
    const samples = [
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'rtt', sampleId: 'sample_z', timestampMs: sharedTs }),
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'rtt', sampleId: 'sample_a', timestampMs: sharedTs }),
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'rtt', sampleId: 'sample_m', timestampMs: sharedTs }),
    ]
    const projection = project(samples, oneHourViewport())
    expect(projection.lanes).toHaveLength(1)

    const x = projection.marks[0].x
    const y = LAYOUT.laneHeightPx / 2

    // All 3 samples must be returned without aggregation or dropping.
    const hits = hitTestMarks(projection, x, y)
    expect(hits).toHaveLength(3)

    // Order must be deterministic: identical distance, identical timestamp -> tie-break by sampleId ascending.
    const hitIds = hits.map((i) => projection.marks[i].sample.sampleId)
    expect(hitIds).toEqual(['sample_a', 'sample_m', 'sample_z'])

    // pickNearestMark must deterministically return the top tie-break winner (sample_a).
    const nearest = pickNearestMark(projection, x, y)
    expect(nearest).not.toBeNull()
    expect(projection.marks[nearest!].sample.sampleId).toBe('sample_a')
  })

  it('returns nothing for a pointer outside any lane band', () => {
    const projection = project(makeSeries(3), oneHourViewport())
    expect(hitTestMarks(projection, 100, 5000)).toEqual([])
  })

  it('keeps every mark reachable through the column index', () => {
    // 5000 samples over one hour in a single lane → heavy pixel collision.
    const samples = makeSeries(5000, { sampleId: 'dense' }, BASE_TS, 720)
    const projection = project(samples, oneHourViewport(600))

    expect(projection.totalCount).toBe(5000)
    expect(projection.marks).toHaveLength(5000)

    let indexed = 0
    for (const indices of projection.columnIndex.values()) indexed += indices.length
    expect(indexed).toBe(5000)

    const unique = new Set<number>()
    for (const indices of projection.columnIndex.values()) {
      for (const i of indices) unique.add(i)
    }
    expect(unique.size).toBe(5000)
  })

  it('keeps a dense set individually addressable through hit testing', () => {
    const samples = makeSeries(200, { sampleId: 'hit' }, BASE_TS, 300)
    const projection = project(samples, oneHourViewport(600))

    // Pick the busiest column and confirm every sample in it is returned.
    let busiestColumn = -1
    let busiestCount = 0
    for (const [key, indices] of projection.columnIndex) {
      if (indices.length > busiestCount) {
        busiestCount = indices.length
        busiestColumn = Number(key.split('|')[1])
      }
    }

    expect(busiestCount).toBeGreaterThan(1)
    const hits = hitTestMarks(projection, busiestColumn, 10)
    expect(hits.length).toBeGreaterThanOrEqual(busiestCount)
  })
})

describe('keyboard stepping', () => {
  it('steps forward and backward through a lane in time order', () => {
    const samples = makeSeries(4, { sampleId: 'k' }, BASE_TS, MIN)
    const projection = project(samples, oneHourViewport())
    const ordered = projection.marks
      .map((m, i) => ({ m, i }))
      .sort((a, b) => a.m.sample.timestampMs - b.m.sample.timestampMs)
      .map((e) => e.i)

    const next = stepMarkInLane(projection, ordered[0], 1)
    expect(next).toBe(ordered[1])

    const prev = stepMarkInLane(projection, ordered[1], -1)
    expect(prev).toBe(ordered[0])
  })

  it('returns null at the lane boundaries', () => {
    const samples = makeSeries(2, { sampleId: 'b' }, BASE_TS, MIN)
    const projection = project(samples, oneHourViewport())
    const ordered = projection.marks
      .map((m, i) => ({ m, i }))
      .sort((a, b) => a.m.sample.timestampMs - b.m.sample.timestampMs)
      .map((e) => e.i)

    expect(stepMarkInLane(projection, ordered[0], -1)).toBeNull()
    expect(stepMarkInLane(projection, ordered[ordered.length - 1], 1)).toBeNull()
  })

  it('never crosses into another lane', () => {
    const samples = [
      makeSample({ probeType: 'rtt', timestampMs: BASE_TS, sampleId: 'x1' }),
      makeSample({ probeType: 'ttfb', timestampMs: BASE_TS + MIN, sampleId: 'x2' }),
    ]
    const projection = project(samples, oneHourViewport())
    const lane0 = projection.marks.findIndex((m) => m.laneIndex === 0)
    expect(stepMarkInLane(projection, lane0, 1)).toBeNull()
  })
})

describe('lane geometry', () => {
  it('computes non-overlapping bands', () => {
    const first = laneBand(LAYOUT, 0)
    const second = laneBand(LAYOUT, 1)
    expect(first.top).toBe(0)
    expect(first.bottom).toBe(40)
    expect(second.top).toBe(44)
    expect(second.bottom).toBe(84)
  })

  it('maps a y coordinate back to its lane index', () => {
    expect(laneIndexAtY(LAYOUT, 10, 3)).toBe(0)
    expect(laneIndexAtY(LAYOUT, 50, 3)).toBe(1)
    expect(laneIndexAtY(LAYOUT, 5000, 3)).toBeNull()
    expect(laneIndexAtY(LAYOUT, -5, 3)).toBeNull()
  })
})

describe('revision boundary anchoring', () => {
  it('finds the first mark at or after a boundary instant', () => {
    const samples = makeSeries(4, { sampleId: 'rb' }, BASE_TS, MIN)
    const projection = project(samples, oneHourViewport())
    const index = markIndexAtOrAfter(projection, projection.lanes[0].key, BASE_TS + 2 * MIN + 1000)
    expect(index).not.toBeNull()
    expect(projection.marks[index!].sample.timestampMs).toBe(BASE_TS + 3 * MIN)
  })

  it('returns null when no mark follows the instant', () => {
    const samples = makeSeries(2, { sampleId: 'rn' }, BASE_TS, MIN)
    const projection = project(samples, oneHourViewport())
    expect(markIndexAtOrAfter(projection, projection.lanes[0].key, BASE_TS + HOUR)).toBeNull()
  })
})
