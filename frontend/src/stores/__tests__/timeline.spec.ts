import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('../../api/monitor', () => ({
  queryMonitorSamplesCursor: vi.fn(),
  fetchMonitorStats: vi.fn(),
  fetchMonitorFacets: vi.fn(),
}))

import { fetchMonitorFacets, fetchMonitorStats, queryMonitorSamplesCursor } from '../../api/monitor'
import { useTimelineStore } from '../timeline'
import type { DerivedStats, MonitorSample, MonitorSampleFacets } from '../../types'

const mockedCursor = vi.mocked(queryMonitorSamplesCursor)
const mockedStats = vi.mocked(fetchMonitorStats)
const mockedFacets = vi.mocked(fetchMonitorFacets)

const NOW = Date.parse('2026-09-17T12:00:00.000Z')
const MIN = 60_000
const HOUR = 60 * MIN

function sample(id: string, timestampMs: number, overrides: Partial<MonitorSample> = {}): MonitorSample {
  return {
    sampleId: id,
    runId: 'run_1',
    nodeKey: 'nk_jp01',
    nodeIdentityKey: 'nid_jp01',
    configRevisionKey: 'rev_1',
    profileId: 'prof_1',
    displayNameSnapshot: 'JP01',
    probeType: 'rtt',
    target: 'https://cp.cloudflare.com/generate_204',
    timestampMs,
    timestampIso: new Date(timestampMs).toISOString(),
    success: true,
    latencyMs: 40,
    ttfbMs: 30,
    errorClass: 'none',
    ...overrides,
  }
}

function page(items: MonitorSample[], nextCursor = '', hasMore = false) {
  return { items, nextCursor, hasMore, limit: 500 }
}

function emptyStats(): DerivedStats {
  return {
    sampleCount: 0,
    successCount: 0,
    failureCount: 0,
    successRate: 0,
    latencyMinMs: null,
    latencyP50Ms: null,
    latencyP95Ms: null,
    latencyMaxMs: null,
    ttfbP50Ms: null,
    ttfbP95Ms: null,
    errorBreakdown: {},
    firstSampleAtMs: null,
    lastSampleAtMs: null,
  }
}

function emptyFacets(): MonitorSampleFacets {
  return {
    nodes: [],
    profiles: [],
    probeTypes: [],
    targets: [],
    windowSinceMs: NOW - 30 * 24 * HOUR,
    windowUntilMs: NOW,
    truncated: false,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.useFakeTimers()
  vi.setSystemTime(new Date(NOW))
  mockedCursor.mockReset()
  mockedStats.mockReset()
  mockedFacets.mockReset()
  mockedStats.mockResolvedValue(emptyStats())
  mockedFacets.mockResolvedValue(emptyFacets())
  mockedCursor.mockResolvedValue(page([]))
})

afterEach(() => {
  vi.useRealTimers()
})

describe('initial load', () => {
  it('requests the newest page without a cursor', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('a', NOW - 2 * MIN), sample('b', NOW - MIN)]))

    await store.reload()

    expect(mockedCursor).toHaveBeenCalledTimes(1)
    const filter = mockedCursor.mock.calls[0][0]
    expect(filter.cursor).toBeUndefined()
    expect(filter.orderDesc).toBe(true)
    expect(store.loadState).toBe('ready')
    expect(store.samples.map((s) => s.sampleId)).toEqual(['a', 'b'])
  })

  it('exposes a ready-but-empty state instead of fabricating placeholder data', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([]))

    await store.reload()

    expect(store.loadState).toBe('ready')
    expect(store.samples).toHaveLength(0)
    expect(store.isEmpty).toBe(true)
  })

  it('surfaces an API error without inventing samples', async () => {
    const store = useTimelineStore()
    mockedCursor.mockRejectedValue(new Error('boom'))

    await store.reload()

    expect(store.loadState).toBe('error')
    expect(store.loadError).toBe('boom')
    expect(store.samples).toHaveLength(0)
  })
})

describe('cursor pagination', () => {
  it('walks older pages strictly through next_cursor', async () => {
    const store = useTimelineStore()
    mockedCursor
      .mockResolvedValueOnce(page([sample('n1', NOW - MIN)], 'CUR_1', true))
      .mockResolvedValueOnce(page([sample('n2', NOW - 2 * MIN)], 'CUR_2', true))

    await store.reload()
    await store.loadOlder()

    expect(mockedCursor).toHaveBeenCalledTimes(2)
    expect(mockedCursor.mock.calls[1][0].cursor).toBe('CUR_1')
    expect(store.nextCursor).toBe('CUR_2')
    expect(store.samples.map((s) => s.sampleId)).toEqual(['n2', 'n1'])
  })

  it('never fabricates a previous cursor', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - MIN)], 'CUR_1', true))
    await store.reload()

    // Every request must either omit the cursor (newest) or carry the server-issued one.
    for (const call of mockedCursor.mock.calls) {
      expect(call[0].cursor === undefined || call[0].cursor === 'CUR_1').toBe(true)
    }
  })

  it('does not request older pages once the server reports no more', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - MIN)], '', false))

    await store.reload()
    await store.loadOlder()

    expect(mockedCursor).toHaveBeenCalledTimes(1)
  })

  it('does not duplicate samples that reappear across pages', async () => {
    const store = useTimelineStore()
    mockedCursor
      .mockResolvedValueOnce(page([sample('n1', NOW - MIN), sample('n2', NOW - 2 * MIN)], 'CUR_1', true))
      .mockResolvedValueOnce(page([sample('n2', NOW - 2 * MIN), sample('n3', NOW - 3 * MIN)], '', false))

    await store.reload()
    await store.loadOlder()

    expect(store.samples.map((s) => s.sampleId)).toEqual(['n3', 'n2', 'n1'])
  })

  it('reports partial loading while older pages remain', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - MIN)], 'CUR_1', true))

    await store.reload()

    expect(store.isPartiallyLoaded).toBe(true)
  })
})

describe('filter changes', () => {
  it('discards the cursor and restarts from the newest page', async () => {
    const store = useTimelineStore()
    mockedCursor
      .mockResolvedValueOnce(page([sample('n1', NOW - MIN)], 'CUR_1', true))
      .mockResolvedValueOnce(page([sample('n0', NOW - 2 * MIN)], '', false))

    await store.reload()
    await store.loadOlder()
    expect(store.nextCursor).toBe('')

    mockedCursor.mockClear()
    mockedCursor.mockResolvedValueOnce(page([sample('p1', NOW - MIN, { probeType: 'ttfb' })], 'CUR_2', true))

    await store.setProbeTypeFilter('ttfb')

    expect(mockedCursor).toHaveBeenCalledTimes(1)
    expect(mockedCursor.mock.calls[0][0].cursor).toBeUndefined()
    expect(mockedCursor.mock.calls[0][0].probeType).toBe('ttfb')
    expect(store.samples.map((s) => s.sampleId)).toEqual(['p1'])
    expect(store.nextCursor).toBe('CUR_2')
  })

  it('resets the cursor when the time range changes', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - MIN)], 'CUR_1', true))

    await store.reload()
    mockedCursor.mockClear()

    await store.setRange('7d')

    expect(mockedCursor).toHaveBeenCalledTimes(1)
    expect(mockedCursor.mock.calls[0][0].cursor).toBeUndefined()
    expect(store.rangeKey).toBe('7d')
  })

  it('resets the cursor when the node changes', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - MIN)], 'CUR_1', true))

    await store.reload()
    mockedCursor.mockClear()

    await store.setNodeFilter('nid_other')

    expect(mockedCursor).toHaveBeenCalledTimes(1)
    expect(mockedCursor.mock.calls[0][0].cursor).toBeUndefined()
    expect(mockedCursor.mock.calls[0][0].nodeIdentityKey).toBe('nid_other')
  })

  it('does not reload when a filter is set to its current value', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - MIN)]))
    await store.reload()
    mockedCursor.mockClear()

    await store.setProbeTypeFilter('')

    expect(mockedCursor).not.toHaveBeenCalled()
  })

  it('scopes the query to the active range window', async () => {
    const store = useTimelineStore()
    await store.reload()

    const filter = mockedCursor.mock.calls[0][0]
    expect(filter.sinceMs).toBe(NOW - 24 * HOUR)
    expect(filter.untilMs).toBe(NOW)
  })
})

describe('background refresh', () => {
  it('deduplicates by sample id and counts only genuinely new samples', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - 2 * MIN), sample('n2', NOW - MIN)]))
    await store.reload()

    mockedCursor.mockResolvedValueOnce(
      page([sample('n3', NOW), sample('n2', NOW - MIN), sample('n1', NOW - 2 * MIN)])
    )

    const added = await store.refreshNewest()

    expect(added).toBe(1)
    expect(store.samples.map((s) => s.sampleId)).toEqual(['n1', 'n2', 'n3'])
    const ids = store.samples.map((s) => s.sampleId)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('reports zero new samples when the head has not moved', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - MIN)]))
    await store.reload()

    expect(await store.refreshNewest()).toBe(0)
    expect(store.newSampleCount).toBe(0)
  })

  it('does not move the viewport while the user is browsing history', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - 2 * HOUR)]))
    await store.reload()

    // Zoom into a historical window, away from the live edge.
    store.panToRange(NOW - 8 * HOUR, NOW - 4 * HOUR)
    expect(store.followLive).toBe(false)
    const viewportBefore = { ...store.viewport }

    mockedCursor.mockResolvedValueOnce(page([sample('n9', NOW), sample('n1', NOW - 2 * HOUR)]))
    await store.refreshNewest()

    expect(store.viewport.startMs).toBe(viewportBefore.startMs)
    expect(store.viewport.endMs).toBe(viewportBefore.endMs)
    expect(store.newSampleCount).toBe(1)
    expect(store.followLive).toBe(false)
  })

  it('lets the user jump back to the newest sample and clears the pending counter', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - 2 * HOUR)]))
    await store.reload()
    store.panToRange(NOW - 8 * HOUR, NOW - 4 * HOUR)

    mockedCursor.mockResolvedValueOnce(page([sample('n9', NOW), sample('n1', NOW - 2 * HOUR)]))
    await store.refreshNewest()
    expect(store.newSampleCount).toBe(1)

    store.resetToLatest()

    expect(store.newSampleCount).toBe(0)
    expect(store.followLive).toBe(true)
    expect(store.viewport.endMs).toBe(NOW)
  })

  it('keeps the viewport pinned to the live edge when already following it', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - 2 * HOUR)]))
    await store.reload()

    // Sit at the live edge with a zoomed-in window.
    store.panToRange(NOW - 2 * HOUR, NOW)
    expect(store.followLive).toBe(true)
    const spanBefore = store.viewport.endMs - store.viewport.startMs

    mockedCursor.mockResolvedValueOnce(page([sample('n9', NOW - MIN), sample('n1', NOW - 2 * HOUR)]))
    await store.refreshNewest()

    // The zoom level must survive the refresh, the newest sample must land inside the
    // viewport, and nothing is queued behind a "new samples" badge.
    expect(store.viewport.endMs - store.viewport.startMs).toBeCloseTo(spanBefore, 0)
    const newest = store.newestLoadedMs!
    expect(newest).toBeGreaterThanOrEqual(store.viewport.startMs)
    expect(newest).toBeLessThanOrEqual(store.viewport.endMs)
    expect(store.followLive).toBe(true)
    expect(store.newSampleCount).toBe(0)
  })

  it('ignores a refresh response that belongs to a superseded filter', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - MIN)]))
    await store.reload()

    // Start a refresh, then change the filter before it resolves.
    let resolveRefresh: (v: ReturnType<typeof page>) => void = () => {}
    mockedCursor.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRefresh = resolve as (v: ReturnType<typeof page>) => void
        })
    )
    const refreshPromise = store.refreshNewest()

    mockedCursor.mockResolvedValueOnce(page([sample('p1', NOW - MIN, { probeType: 'ttfb' })]))
    await store.setProbeTypeFilter('ttfb')

    resolveRefresh(page([sample('stale', NOW)]))
    await refreshPromise

    expect(store.samples.map((s) => s.sampleId)).toEqual(['p1'])
  })

  it('does not destroy the loaded view when a refresh fails', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(page([sample('n1', NOW - MIN)]))
    await store.reload()

    mockedCursor.mockRejectedValueOnce(new Error('network down'))
    const added = await store.refreshNewest()

    expect(added).toBe(0)
    expect(store.samples).toHaveLength(1)
    expect(store.loadState).toBe('ready')
  })
})

describe('viewport interactions', () => {
  it('clamps panning to the selected domain', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - MIN)]))
    await store.reload()

    store.pan(-1e9)

    expect(store.viewport.startMs).toBe(store.domainStartMs)
  })

  it('zooms around an anchor without leaving the domain', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - MIN)]))
    await store.reload()

    store.panToRange(NOW - 12 * HOUR, NOW)
    store.zoom(0.5, 500)

    expect(store.viewport.endMs - store.viewport.startMs).toBeCloseTo(6 * HOUR, 0)
    expect(store.viewport.startMs).toBeGreaterThanOrEqual(store.domainStartMs)
    expect(store.viewport.endMs).toBeLessThanOrEqual(store.domainEndMs)
  })

  it('requests older data only when the left edge is reached', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValueOnce(
      page([sample('new', NOW - MIN), sample('old', NOW - 60 * MIN)], 'CUR_1', true)
    )
    await store.reload()
    mockedCursor.mockClear()
    mockedCursor.mockResolvedValueOnce(page([sample('older', NOW - 120 * MIN)], '', false))

    // Well to the right of the oldest loaded sample → nothing to fetch.
    store.panToRange(NOW - 10 * MIN, NOW - 5 * MIN)
    store.maybeLoadOlder()
    expect(mockedCursor).not.toHaveBeenCalled()

    // Reaching the oldest loaded sample → fetch the next page through the cursor.
    store.panToRange(NOW - 61 * MIN, NOW - 60 * MIN)
    store.maybeLoadOlder()
    await Promise.resolve()

    expect(mockedCursor).toHaveBeenCalledTimes(1)
    expect(mockedCursor.mock.calls[0][0].cursor).toBe('CUR_1')
  })
})

describe('selection', () => {
  it('selects a raw sample and exposes it for the inspector', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - 2 * MIN), sample('n2', NOW - MIN)]))
    await store.reload()

    store.selectSample('n2')

    expect(store.selectedSample?.sampleId).toBe('n2')
    expect(store.selectedSample?.latencyMs).toBe(40)
  })

  it('keeps colliding samples individually selectable', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(
      page([sample('c1', NOW - MIN), sample('c2', NOW - MIN), sample('c3', NOW - MIN)])
    )
    await store.reload()

    store.selectSample('c2', ['c1', 'c2', 'c3'])
    expect(store.candidates).toHaveLength(3)
    expect(store.candidateIndex).toBe(1)

    store.cycleCandidate(1)
    expect(store.selectedSample?.sampleId).toBe('c3')

    store.cycleCandidate(1)
    expect(store.selectedSample?.sampleId).toBe('c1')
  })

  it('steps to the previous and next sample in the lane', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(
      page([sample('s1', NOW - 3 * MIN), sample('s2', NOW - 2 * MIN), sample('s3', NOW - MIN)])
    )
    await store.reload()

    store.selectSample('s2')
    store.selectRelativeInLane(1)
    expect(store.selectedSample?.sampleId).toBe('s3')

    store.selectRelativeInLane(-1)
    expect(store.selectedSample?.sampleId).toBe('s2')
  })

  it('stops at the lane boundary rather than jumping lanes', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('s1', NOW - 2 * MIN), sample('s2', NOW - MIN)]))
    await store.reload()

    store.selectSample('s2')
    store.selectRelativeInLane(1)

    expect(store.selectedSample?.sampleId).toBe('s2')
  })

  it('clears selection state', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('s1', NOW - MIN)]))
    await store.reload()
    store.selectSample('s1')

    store.clearSelection()

    expect(store.selectedSample).toBeNull()
    expect(store.candidates).toHaveLength(0)
  })
})

describe('lanes and revision boundaries', () => {
  it('keeps probe and target dimensions in separate lanes', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(
      page([
        sample('a1', NOW - 2 * MIN, { probeType: 'rtt' }),
        sample('a2', NOW - MIN, { probeType: 'ttfb' }),
      ])
    )
    await store.reload()

    expect(store.lanes).toHaveLength(2)
  })

  it('detects a config revision boundary from raw samples', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(
      page([
        sample('r1', NOW - 2 * MIN, { configRevisionKey: 'rev_1' }),
        sample('r2', NOW - MIN, { configRevisionKey: 'rev_2' }),
      ])
    )
    await store.reload()

    expect(store.revisionBoundaries).toHaveLength(1)
    expect(store.revisionBoundaries[0].fromRevision).toBe('rev_1')
    expect(store.revisionBoundaries[0].toRevision).toBe('rev_2')
  })

  it('flags legacy backfilled history without altering it', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(
      page([sample('l1', NOW - MIN, { nodeKey: 'nk_legacy', nodeIdentityKey: 'nk_legacy' })])
    )
    await store.reload()

    expect(store.hasLegacySamples).toBe(true)
    expect(store.samples[0].nodeIdentityKey).toBe('nk_legacy')
  })
})

describe('evidence statistics', () => {
  it('fetches the active range plus fixed 24h and 7d evidence windows', async () => {
    const store = useTimelineStore()
    await store.reload()

    const windows = mockedStats.mock.calls.map((c) => [c[0].sinceMs, c[0].untilMs])
    expect(windows).toHaveLength(3)
    expect(windows).toContainEqual([NOW - 24 * HOUR, NOW])
    expect(windows).toContainEqual([NOW - 7 * 24 * HOUR, NOW])
  })

  it('keeps the stats failure separate from the sample load state', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - MIN)]))
    mockedStats.mockRejectedValue(new Error('stats unavailable'))

    await store.reload()

    expect(store.loadState).toBe('ready')
    expect(store.samples).toHaveLength(1)
    expect(store.statsError).toBe('stats unavailable')
  })
})

describe('auto refresh lifecycle', () => {
  it('polls on the configured interval and stops cleanly', async () => {
    const store = useTimelineStore()
    mockedCursor.mockResolvedValue(page([sample('n1', NOW - MIN)]))
    await store.reload()
    mockedCursor.mockClear()

    store.startAutoRefresh()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mockedCursor).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(15_000)
    expect(mockedCursor).toHaveBeenCalledTimes(2)

    store.stopAutoRefresh()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(mockedCursor).toHaveBeenCalledTimes(2)
  })
})
