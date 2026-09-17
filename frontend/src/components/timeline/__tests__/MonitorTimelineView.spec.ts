import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('../../../api/monitor', () => ({
  queryMonitorSamplesCursor: vi.fn(),
  fetchMonitorStats: vi.fn(),
  fetchMonitorFacets: vi.fn(),
}))

import { fetchMonitorFacets, fetchMonitorStats, queryMonitorSamplesCursor } from '../../../api/monitor'
import { useTimelineStore } from '../../../stores/timeline'
import type { DerivedStats, MonitorSample, MonitorSampleFacets } from '../../../types'
import MonitorTimelineView from '../MonitorTimelineView.vue'

const mockedCursor = vi.mocked(queryMonitorSamplesCursor)
const mockedStats = vi.mocked(fetchMonitorStats)
const mockedFacets = vi.mocked(fetchMonitorFacets)

/**
 * Child components are stubbed so these tests assert the container's *state machine* only.
 * The canvas, projection and rendering are covered by the utils specs.
 */
const stubs = {
  TimelineFilterBar: { template: '<div data-testid="filter-bar" />' },
  TimelineEvidenceBar: { template: '<div data-testid="evidence-bar" />' },
  TimelineInspector: { template: '<div data-testid="inspector" />' },
  SampleTimeline: { template: '<div data-testid="timeline" />' },
}

const NOW = Date.parse('2026-09-17T12:00:00.000Z')

let wrapper: VueWrapper | null = null

function sample(id: string, overrides: Partial<MonitorSample> = {}): MonitorSample {
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
    timestampMs: NOW - 60_000,
    timestampIso: new Date(NOW - 60_000).toISOString(),
    success: true,
    latencyMs: 40,
    ttfbMs: 30,
    errorClass: 'none',
    ...overrides,
  }
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

function facetsWithNodes(count: number): MonitorSampleFacets {
  return {
    nodes: Array.from({ length: count }, (_, i) => ({
      nodeIdentityKey: `nid_${i}`,
      nodeKey: `nk_${i}`,
      displayName: `Node ${i}`,
      profileId: 'prof_1',
      sampleCount: 10,
    })),
    profiles: ['prof_1'],
    probeTypes: ['rtt'],
    targets: ['https://cp.cloudflare.com/generate_204'],
    windowSinceMs: NOW - 86_400_000,
    windowUntilMs: NOW,
    truncated: false,
  }
}

async function mountView(): Promise<VueWrapper> {
  wrapper = mount(MonitorTimelineView, { global: { stubs } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  setActivePinia(createPinia())
  mockedCursor.mockResolvedValue({ items: [], nextCursor: '', hasMore: false, limit: 500 })
  mockedStats.mockResolvedValue(emptyStats())
  mockedFacets.mockResolvedValue(facetsWithNodes(0))
})

afterEach(() => {
  // Unmounting is required: the container starts a polling interval on mount.
  wrapper?.unmount()
  wrapper = null
})

describe('MonitorTimelineView states', () => {
  it('shows a real loading state before the first page resolves', async () => {
    mockedCursor.mockReturnValue(new Promise(() => {}))
    const w = await mountView()
    expect(w.text()).toContain('正在通过游标读取最新一页原始样本')
    expect(w.find('[data-testid="timeline"]').exists()).toBe(false)
  })

  it('never substitutes mock samples while loading', async () => {
    mockedCursor.mockReturnValue(new Promise(() => {}))
    const w = await mountView()
    expect(w.text()).toContain('不会用任何示例数据填充时间轴')
    expect(w.findAll('canvas')).toHaveLength(0)
  })

  it('shows the API error message and a retry action', async () => {
    mockedCursor.mockRejectedValue(new Error('database is locked'))
    const w = await mountView()
    expect(w.text()).toContain('读取监控样本失败')
    expect(w.text()).toContain('database is locked')
    expect(w.text()).toContain('重试')
  })

  it('distinguishes "no samples in this range" from an empty history', async () => {
    mockedFacets.mockResolvedValue(facetsWithNodes(2))
    const w = await mountView()
    expect(w.text()).toContain('所选时间范围内没有样本')
    expect(w.text()).not.toContain('监控历史中还没有任何样本')
  })

  it('reports an empty history when storage holds nothing at all', async () => {
    mockedFacets.mockResolvedValue(facetsWithNodes(0))
    const w = await mountView()
    expect(w.text()).toContain('监控历史中还没有任何样本')
  })

  it('reports "filter has no match" when narrowing filters are active', async () => {
    mockedFacets.mockResolvedValue(facetsWithNodes(3))
    const store = useTimelineStore()
    store.profileId = 'prof_absent'
    const w = await mountView()
    expect(w.text()).toContain('当前筛选条件在该时间范围内没有匹配样本')
    expect(w.text()).not.toContain('所选时间范围内没有样本')
  })

  it('renders the timeline as soon as one raw sample is loaded', async () => {
    mockedCursor.mockResolvedValue({
      items: [sample('s1')],
      nextCursor: '',
      hasMore: false,
      limit: 500,
    })
    const w = await mountView()
    expect(w.find('[data-testid="timeline"]').exists()).toBe(true)
    expect(w.text()).not.toContain('读取监控样本失败')
  })

  it('always keeps the inspector mounted so evidence has a stable entry point', async () => {
    mockedCursor.mockRejectedValue(new Error('boom'))
    const w = await mountView()
    expect(w.find('[data-testid="inspector"]').exists()).toBe(true)
  })

  it('surfaces a legacy-history notice only when bridged samples are present', async () => {
    mockedCursor.mockResolvedValue({
      items: [sample('s1', { nodeKey: 'nk_legacy', nodeIdentityKey: 'nk_legacy' })],
      nextCursor: '',
      hasMore: false,
      limit: 500,
    })
    const withLegacy = await mountView()
    expect(withLegacy.text()).toContain('迁移桥接的历史样本')

    wrapper?.unmount()
    mockedCursor.mockResolvedValue({
      items: [sample('s2')],
      nextCursor: '',
      hasMore: false,
      limit: 500,
    })
    const withoutLegacy = await mountView()
    expect(withoutLegacy.text()).not.toContain('迁移桥接的历史样本')
  })

  it('states the partial-load boundary and offers an explicit load-more', async () => {
    mockedCursor.mockResolvedValue({
      items: [sample('s1')],
      nextCursor: 'cursor_1',
      hasMore: true,
      limit: 500,
    })
    const w = await mountView()
    expect(w.text()).toContain('左侧仍有更旧的历史未加载')
    expect(w.text()).toContain('加载更旧样本')
  })

  it('does not show the partial-load bar when everything in range is loaded', async () => {
    mockedCursor.mockResolvedValue({
      items: [sample('s1')],
      nextCursor: '',
      hasMore: false,
      limit: 500,
    })
    const w = await mountView()
    expect(w.text()).not.toContain('加载更旧样本')
  })

  it('stops polling when the view is unmounted', async () => {
    const clearSpy = vi.spyOn(globalThis, 'clearInterval')
    await mountView()
    wrapper?.unmount()
    wrapper = null
    expect(clearSpy).toHaveBeenCalled()
    clearSpy.mockRestore()
  })

  it('clicks a specific lane on canvas and asserts Inspector displays the SampleID of that lane', async () => {
    const now = Date.now()
    const sampleL0 = sample('sample_lane0', {
      timestampMs: now - 60_000,
      timestampIso: new Date(now - 60_000).toISOString(),
      probeType: 'rtt',
      target: 'https://cp.cloudflare.com/generate_204',
      displayNameSnapshot: 'Node Alpha RTT',
    })
    const sampleL1 = sample('sample_lane1', {
      timestampMs: now - 60_000,
      timestampIso: new Date(now - 60_000).toISOString(),
      probeType: 'ttfb',
      target: 'https://cp.cloudflare.com/generate_204',
      displayNameSnapshot: 'Node Alpha TTFB',
    })

    mockedCursor.mockResolvedValue({
      items: [sampleL0, sampleL1],
      nextCursor: '',
      hasMore: false,
      limit: 500,
    })

    // Mount view WITHOUT stubbing SampleTimeline and TimelineInspector
    wrapper = mount(MonitorTimelineView, {
      global: {
        stubs: {
          TimelineFilterBar: { template: '<div data-testid="filter-bar" />' },
          TimelineEvidenceBar: { template: '<div data-testid="evidence-bar" />' },
        },
      },
    })
    await flushPromises()

    const store = useTimelineStore()
    expect(store.lanes).toHaveLength(2)

    // Lane buttons are rendered in the lane sidebar for every lane
    const laneButtons = wrapper.findAll('.relative.border-r button')
    expect(laneButtons).toHaveLength(2)

    // Click Lane 1 button
    await laneButtons[1].trigger('click')
    await flushPromises()

    // Assert that sample from Lane 1 is selected
    expect(store.selectedSample?.sampleId).toBe('sample_lane1')
    expect(store.selectedLaneKey).toBe(store.lanes[1].key)

    // Inspector must render and display sample_lane1
    const inspector = wrapper.find('aside')
    expect(inspector.exists()).toBe(true)
    expect(inspector.text()).toContain('sample_lane1')
    expect(inspector.text()).not.toContain('sample_lane0')

    // Click Lane 0 button
    await laneButtons[0].trigger('click')
    await flushPromises()

    // Assert that sample from Lane 0 is selected
    expect(store.selectedSample?.sampleId).toBe('sample_lane0')
    expect(store.selectedLaneKey).toBe(store.lanes[0].key)
    expect(inspector.text()).toContain('sample_lane0')
    expect(inspector.text()).not.toContain('sample_lane1')
  })
})
