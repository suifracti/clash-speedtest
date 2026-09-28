import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { MonitorJob, WorkbenchLatencyTest } from '../../../types'

const mocks = vi.hoisted(() => ({
  fetchNodes: vi.fn(),
  fetchHistory: vi.fn(),
  fetchHistories: vi.fn(),
  fetchDownloadHistory: vi.fn(),
  fetchServiceHistory: vi.fn(),
  fetchTest: vi.fn(),
  fetchBatches: vi.fn(),
  fetchJobs: vi.fn(),
  fetchAirports: vi.fn(),
  saveMaintenance: vi.fn(),
  startBatch: vi.fn(),
  startDownload: vi.fn(),
  cancelBatch: vi.fn(),
  retryItem: vi.fn(),
  subscribe: vi.fn(),
  eventCallback: null as null | ((type: string, payload: unknown) => void),
}))

vi.mock('../../../api/monitor', () => ({
  fetchMonitorNodeOptions: mocks.fetchNodes,
  fetchMonitorJobs: mocks.fetchJobs,
}))

vi.mock('../../../api/bridge', () => ({
  fetchAirports: mocks.fetchAirports,
  saveAirportMaintenance: mocks.saveMaintenance,
  fetchWorkbenchLatencyHistory: mocks.fetchHistory,
  fetchWorkbenchLatencyHistories: mocks.fetchHistories,
  fetchWorkbenchDownloadHistory: mocks.fetchDownloadHistory,
  fetchWorkbenchPublicServiceHistory: mocks.fetchServiceHistory,
  fetchWorkbenchLatencyTest: mocks.fetchTest,
  fetchWorkbenchLatencyBatches: mocks.fetchBatches,
  startWorkbenchLatencyBatch: mocks.startBatch,
  startWorkbenchDownloadTest: mocks.startDownload,
  cancelWorkbenchLatencyBatch: mocks.cancelBatch,
  retryWorkbenchLatencyBatchItem: mocks.retryItem,
  subscribeEvents: mocks.subscribe,
}))

import LatencyWorkbench from '../LatencyWorkbench.vue'

it('counts category entries within the selected subscription', async () => {
  mocks.fetchNodes.mockResolvedValue([
    ...mockNodes,
    { ...mockNodes[0], nodeKey: 'notice', displayName: '官网 https://example.com' },
    { ...mockNodes[0], profileId: 'other', nodeKey: 'other', displayName: 'Other node' },
  ])
  const wrapper = mount(LatencyWorkbench, { props: { initialProfileId: 'prof-1' }, global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
  await flushPromises()
  expect(wrapper.text()).toContain('2 个缓存条目')
  expect(wrapper.text()).toContain('代理节点 (1)')
  expect(wrapper.text()).toContain('公告与信息 (1)')
  mocks.fetchAirports.mockResolvedValue([
    { id: 'wrong-owner', name: 'Other', subscriptions: [{ id: 'other' }] },
    { id: 'owner', name: 'Airport', subscriptions: [{ id: 'prof-1' }], maintenance: { refresh_hours: 24, links: [{ label: 'Existing', url: 'https://example.org', monthly_day: 1 }] } },
  ])
  mocks.saveMaintenance.mockResolvedValue(undefined)
  await wrapper.findAll('button').find(button => button.text().includes('公告与信息 (1)'))!.trigger('click')
  await wrapper.findAll('button').find(button => button.text().includes('加入机场管理'))!.trigger('click')
  await flushPromises()
  expect(mocks.saveMaintenance).toHaveBeenCalledWith('owner', { refresh_hours: 24, links: [
    { label: 'Existing', url: 'https://example.org', monthly_day: 1 },
    { label: '官网', url: 'https://example.com', monthly_day: 0 },
  ] })
  await wrapper.setProps({ initialProfileId: 'other' })
  await flushPromises()
  expect(wrapper.text()).toContain('1 个缓存条目')
  expect(wrapper.text()).toContain('公告与信息 (0)')
  wrapper.unmount()
})

const mockNodes = [
  {
    profileId: 'prof-1',
    profileName: '测试订阅',
    nodeKey: 'node-hk',
    nodeIdentityKey: 'id-hk',
    configRevisionKey: 'rev-1',
    displayName: '香港 01 | 专线',
    type: 'ss',
    countryCode: 'HK',
    countryFlag: '🇭🇰',
  },
]

function makeHistoryWithSamples(): { tests: WorkbenchLatencyTest[]; since: string; until: string; as_of: string; has_more: boolean; complete: true } {
  const baseTime = Date.now() - 60000
  const samples = [
    { seq: 1, timestamp: new Date(baseTime).toISOString(), latency_ms: 310, success: true },
    { seq: 2, timestamp: new Date(baseTime + 1000).toISOString(), latency_ms: 315, success: true },
    { seq: 3, timestamp: new Date(baseTime + 2000).toISOString(), latency_ms: 320, success: true },
    { seq: 4, timestamp: new Date(baseTime + 3000).toISOString(), latency_ms: 318, success: true },
    { seq: 5, timestamp: new Date(baseTime + 4000).toISOString(), latency_ms: 322, success: true },
    { seq: 6, timestamp: new Date(baseTime + 5000).toISOString(), latency_ms: 312, success: true },
  ]
  const test: WorkbenchLatencyTest = {
    attempt_id: 'att-1',
    profile_id: 'prof-1',
    node_key: 'node-hk',
    node_identity_key: 'id-hk',
    config_revision_key: 'rev-1',
    display_name: '香港 01 | 专线',
    node_type: 'ss',
    test_project: 'latency_stability',
    source: 'workbench_batch_latency',
    method: 'http_get_via_proxy_first_byte',
    method_version: 1,
    target: 'https://probe.example/__down?bytes=1',
    unit: 'ms',
    requested_at: new Date(baseTime).toISOString(),
    started_at: new Date(baseTime).toISOString(),
    finished_at: new Date(baseTime + 6000).toISOString(),
    status: 'completed',
    latency_ms: 318,
    jitter_ms: 5,
    packet_loss: 0,
    total_samples: 6,
    success_samples: 6,
    failure_samples: 0,
    samples,
    persistence_state: 'saved',
  }
  return {
    tests: [test],
    since: new Date(baseTime - 4 * 3600 * 1000).toISOString(),
    until: new Date(baseTime + 60000).toISOString(),
    as_of: new Date(baseTime + 60000).toISOString(),
    has_more: false,
    complete: true,
  }
}

describe('LatencyWorkbench Health & Redesign Features', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.fetchNodes.mockResolvedValue(mockNodes)
    mocks.fetchHistory.mockResolvedValue(makeHistoryWithSamples())
    mocks.fetchHistories.mockImplementation((queries: unknown[]) => Promise.all(queries.map((query) => mocks.fetchHistory(query))))
    mocks.fetchTest.mockImplementation(() => Promise.resolve(makeHistoryWithSamples().tests[0]))
    mocks.fetchBatches.mockResolvedValue([])
    mocks.fetchDownloadHistory.mockResolvedValue({ attempts: [], has_more: false, complete: true })
    mocks.fetchServiceHistory.mockResolvedValue({ attempts: [], has_more: false, complete: true })
    mocks.fetchJobs.mockResolvedValue([])
    mocks.subscribe.mockImplementation((callback: (type: string, payload: unknown) => void) => {
      mocks.eventCallback = callback
      return vi.fn()
    })
  })

  it('keeps both nodes pending until one scoped batch response arrives', async () => {
    const secondNode = { ...mockNodes[0], nodeKey: 'node-us', nodeIdentityKey: 'id-us', displayName: '美国 01' }
    mocks.fetchNodes.mockResolvedValue([mockNodes[0], secondNode])
    let finishBatch!: (value: ReturnType<typeof makeHistoryWithSamples>[]) => void
    mocks.fetchHistories.mockImplementation(() => new Promise((resolve) => { finishBatch = resolve }))
    const wrapper = mount(LatencyWorkbench, {
      global: { stubs: { UiSelect: true, LatencySamplePlot: true } },
    })
    await flushPromises()
    expect(wrapper.findAll('.history-loading-cell')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('没有历史')

    expect(mocks.fetchHistories).toHaveBeenCalledTimes(1)
    const queries = mocks.fetchHistories.mock.calls[0][0]
    expect(queries.map((query: { node_key: string }) => query.node_key)).toEqual(['node-hk', 'node-us'])
    expect(new Set(queries.map((query: { until: string }) => query.until)).size).toBe(1)
    finishBatch([makeHistoryWithSamples(), { ...makeHistoryWithSamples(), tests: [] }])
    await flushPromises()
    const rows = wrapper.findAll('.node-row')
    const first = rows.find((row) => row.text().includes('香港 01 | 专线'))!
    const second = rows.find((row) => row.text().includes('美国 01'))!
    expect(first.find('.row-health-overview').exists()).toBe(true)
    expect(first.find('.history-loading-cell').exists()).toBe(false)
    expect(second.find('.history-loading-cell').exists()).toBe(false)
    expect(second.text()).toContain('没有历史')
    wrapper.unmount()
  })

  it('renders absolute date labels in history-axis and does not output relative -3h or -5d', async () => {
    const wrapper = mount(LatencyWorkbench, {
      global: { stubs: { UiSelect: true, LatencySamplePlot: true } },
    })
    await flushPromises()

    const axis = wrapper.find('.history-axis')
    expect(axis.exists()).toBe(true)
    const text = axis.text()
    // Must end with 现在
    expect(text).toContain('现在')
    // Should NOT have old relative markers
    expect(text).not.toContain('−3h')
    expect(text).not.toContain('−5d')
    expect(text).not.toContain('最近 4 小时')
  })

  it('provides low-friction notice move via ⋯ dropdown and allows undoing via Toast', async () => {
    const wrapper = mount(LatencyWorkbench, {
      global: { stubs: { UiSelect: true, LatencySamplePlot: true } },
    })
    await flushPromises()

    // Find the ⋯ more action button
    const moreBtn = wrapper.find('.row-action-btn.icon-only')
    expect(moreBtn.exists()).toBe(true)
    expect(moreBtn.text()).toBe('⋯')

    // Popover is hidden initially
    expect(wrapper.find('.row-menu-popover').exists()).toBe(false)

    // Open popover
    await moreBtn.trigger('click')
    const popover = wrapper.find('.row-menu-popover')
    expect(popover.exists()).toBe(true)
    expect(popover.text()).toContain('移至公告与信息')

    // Click move to notice
    const moveBtn = popover.find('.row-menu-item')
    await moveBtn.trigger('click')
    await flushPromises()

    // Undo toast should appear
    const toast = wrapper.find('.workbench-toast')
    expect(toast.exists()).toBe(true)
    expect(toast.text()).toContain('已将「香港 01 | 专线」移至“公告与信息”')
    expect(toast.text()).toContain('撤销')

    // Click undo
    const undoBtn = toast.find('.toast-undo-btn')
    await undoBtn.trigger('click')
    await flushPromises()

    // Toast disappears
    expect(wrapper.find('.workbench-toast').exists()).toBe(false)
  })

  it('shows ● 监测中 tag when node is in an active monitor job, and removes noisy monitor placeholder text', async () => {
    const activeJobs: Partial<MonitorJob>[] = [
      {
        id: 'job-1',
        name: '亚太专线监测',
        profileId: 'prof-1',
        state: 'running',
        runtimeState: 'running',
        nodes: [{ nodeKey: 'node-hk', nodeIdentityKey: 'id-hk', configRevisionKey: 'rev-1', displayName: '香港 01', type: 'ss' }],
      },
    ]
    mocks.fetchJobs.mockResolvedValue(activeJobs)

    const wrapper = mount(LatencyWorkbench, {
      global: { stubs: { UiSelect: true, LatencySamplePlot: true } },
    })
    await flushPromises()

    // Check monitoring badge
    const badge = wrapper.find('.badge-monitoring')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain('● 监测中')

    // Ensure the old noisy text is completely gone
    expect(wrapper.text()).not.toContain('持续监测状态请在“持续监测”查看')
  })

  it('shows a readable health overview next to the chart without opening details', async () => {
    const wrapper = mount(LatencyWorkbench, {
      global: { stubs: { UiSelect: true, LatencySamplePlot: true } },
    })
    await flushPromises()

    const readout = wrapper.find('.row-readout')
    expect(readout.exists()).toBe(true)

    // Upper observation
    expect(readout.find('.readout-observation').exists()).toBe(true)
    expect(readout.text()).toContain('本次平均')
    expect(readout.text()).toContain('316')
    expect(readout.text()).toContain('ms')

    const healthSummary = wrapper.get('.row-health-overview')
    const healthText = healthSummary.text().replace(/\s+/g, '')
    expect(healthSummary.text()).toContain('样本不足')
    expect(healthText).toContain('连通100.0%')
    expect(wrapper.find('.row-health-expanded').exists()).toBe(true)
    expect(healthSummary.get('[role="tooltip"]').text()).toContain('仅凭这些记录不能判断长期是否稳定')
    expect(healthText).toContain('平时延迟310–322ms')
    expect(healthText).toContain('较慢时322ms')
  })

  it('restores the selected raw sample in the left readout while hovering the plot', async () => {
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()
    expect(wrapper.get('.row-readout').text()).toContain('本次平均')
    wrapper.getComponent({ name: 'LatencySamplePlot' }).vm.$emit('hover', 0)
    await flushPromises()
    expect(wrapper.get('.row-readout').text()).toContain('悬停 · 第 1/6 次')
    expect(wrapper.get('.row-readout-value').text()).toContain('310')
    wrapper.getComponent({ name: 'LatencySamplePlot' }).vm.$emit('hover', 5)
    await flushPromises()
    expect(wrapper.get('.row-readout-value').text()).toContain('312')
    wrapper.unmount()
  })

  it('uses scoped saved histories in speed comparisons and service reports without starting tests', async () => {
    const at = new Date(Date.now() - 30000).toISOString()
    mocks.fetchDownloadHistory.mockResolvedValue({
      attempts: [{
        attempt_id: 'download-1', profile_id: 'prof-1', node_key: 'node-hk', node_identity_key: 'id-hk', config_revision_key: 'rev-1',
        requested_at: at, finished_at: at, execution_state: 'completed', persistence_state: 'saved',
        result: { outcome: 'byte_limit', bytes_read: 1048576, duration_ns: 1000000000, finished_at: at, samples: [] },
      }], has_more: false, complete: true,
    })
    mocks.fetchServiceHistory.mockResolvedValue({
      attempts: [{
        attempt_id: 'service-1', profile_id: 'prof-1', node_key: 'node-hk', node_identity_key: 'id-hk', config_revision_key: 'rev-1',
        service_id: 'cloudflare_204', requested_at: at, finished_at: at, execution_state: 'completed', persistence_state: 'saved',
        rule: { name: 'Cloudflare 204', success_criterion: 'HTTP 204' },
        result: { outcome: 'matched', duration_ms: 180, finished_at: at },
      }], has_more: false, complete: true,
    })
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true, MetricHistoryPlot: true, WorkbenchDownloadPanel: true, WorkbenchPublicServicePanel: true } } })
    await flushPromises()

    await wrapper.findAll('.workspace-modes button')[1].trigger('click')
    await flushPromises()
    expect(mocks.fetchDownloadHistory).toHaveBeenCalledWith(expect.objectContaining({ profile_id: 'prof-1', node_identity_key: 'id-hk', config_revision_key: 'rev-1' }))
    expect(wrapper.get('.speed-value').text()).toContain('8.4 Mbps')
    expect(wrapper.get('.download-measure-meta').text()).toContain('1.0 MiB')
    await wrapper.get('.download-expand').trigger('click')
    expect(wrapper.get('.download-process').text()).toContain('本次没有有效过程样本')

    await wrapper.findAll('.workspace-modes button')[2].trigger('click')
    await flushPromises()
    expect(mocks.fetchServiceHistory).toHaveBeenCalledWith(expect.objectContaining({ profile_id: 'prof-1', node_identity_key: 'id-hk', config_revision_key: 'rev-1' }))
    expect(mocks.fetchServiceHistory.mock.calls.at(-1)?.[0].service_id).toBeUndefined()
    expect(wrapper.get('.service-results').text()).toContain('检测通过')
    expect(wrapper.get('.service-score').text()).toContain('100%')
    await wrapper.get('.status-history button').trigger('click')
    expect(wrapper.get('.service-inspector').text()).toContain('180 ms')
    expect(wrapper.findAll('.status-history button')).toHaveLength(1)
    expect(mocks.startBatch).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('requires traffic confirmation and cancellation never starts a download', async () => {
    HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
    const wrapper = mount(LatencyWorkbench, { attachTo: document.body, global: { stubs: { UiSelect: true, LatencySamplePlot: true, WorkbenchDownloadPanel: true, WorkbenchPublicServicePanel: true } } })
    await flushPromises()
    await wrapper.findAll('.workspace-modes button')[1].trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === '全选当前 1 个')!.trigger('click')
    await wrapper.findAll('button').find(b => b.text() === '测速所选节点')!.trigger('click')
    await flushPromises()
    expect(document.querySelector('dialog')?.textContent).toContain('20 MiB')
    expect(mocks.startDownload).not.toHaveBeenCalled()
    const cancel = [...document.querySelectorAll<HTMLButtonElement>('dialog button')].find(b => b.textContent === '返回调整')!
    cancel.click()
    await flushPromises()
    expect(document.querySelector('dialog')).toBeNull()
    expect(mocks.startDownload).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('groups regions by default and filters the visible selection to one region', async () => {
    mocks.fetchNodes.mockResolvedValue([
      { ...mockNodes[0], nodeKey: 'sg1', displayName: '新加坡1', countryCode: 'SG' },
      { ...mockNodes[0], nodeKey: 'us1', displayName: '美国1', countryCode: 'US' },
      { ...mockNodes[0], nodeKey: 'sg2', displayName: '新加坡2', countryCode: 'SG' },
    ])
    mocks.fetchHistory.mockResolvedValue({ ...makeHistoryWithSamples(), tests: [] })
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()
    expect(wrapper.findAll('.node-name').map((row) => row.text())).toEqual(['美国1', '新加坡1', '新加坡2'])
    wrapper.findAllComponents({ name: 'UiSelect' }).find((select) => select.attributes('aria-label') === '筛选地区')!.vm.$emit('update:modelValue', 'SG')
    await flushPromises()
    expect(wrapper.findAll('.node-name').map((row) => row.text())).toEqual(['新加坡1', '新加坡2'])
    expect(wrapper.text()).toContain('全选当前 2 个')
    wrapper.unmount()
  })

  it('shows per-round comparisons and the health details inline without opening each node', async () => {
    const history = makeHistoryWithSamples()
    const base = Date.now() - 180000
    const makeRound = (id: string, offset: number, latencies: number[]): WorkbenchLatencyTest => ({
      ...history.tests[0],
      attempt_id: id,
      requested_at: new Date(base + offset).toISOString(),
      started_at: new Date(base + offset).toISOString(),
      finished_at: new Date(base + offset + 3000).toISOString(),
      samples: latencies.map((latency_ms, index) => ({ seq: index + 1, timestamp: new Date(base + offset + index * 1000).toISOString(), latency_ms, success: true })),
    })
    mocks.fetchHistory.mockResolvedValue({
      ...history,
      tests: [
        makeRound('new', 120000, [250, 350]),
        makeRound('previous', 60000, [200, 200]),
        makeRound('old', 0, [100, 100]),
      ],
      since: new Date(base - 60000).toISOString(),
      until: new Date(base + 180000).toISOString(),
      as_of: new Date(base + 180000).toISOString(),
    })
    const wrapper = mount(LatencyWorkbench, {
      global: { stubs: { UiSelect: true, LatencySamplePlot: true } },
    })
    await flushPromises()
    const health = wrapper.get('.row-health-overview')
    expect(wrapper.get('.row-readout').text()).toContain('本次平均')
    expect(wrapper.get('.row-readout-value').text()).toContain('300')
    expect(health.text()).toContain('比上次慢 100 ms（50%）')
    expect(health.text()).toContain('比此前平均慢 150 ms（100%）')
    expect(health.text()).toContain('比此前最慢一轮慢 100 ms（50%）')
    expect(wrapper.find('.row-health-expanded').exists()).toBe(true)
    expect(health.text()).toContain('最快—最慢 100—350 ms')
    expect(health.find('.row-health-distribution').exists()).toBe(true)
    expect(health.text()).toContain('比上次慢 100 ms（50%）')
    wrapper.unmount()
  })

  it('supports modal tabs between 📈 走势, 🩺 健康分析 and 📋 节点配置, rendering 6-grid metrics and histogram', async () => {
    const wrapper = mount(LatencyWorkbench, {
      global: { stubs: { UiSelect: true, LatencySamplePlot: true, IntraTestSamplePlot: true } },
    })
    await flushPromises()

    // Click 📊 健康与走势 → from footer
    const healthOpenBtn = wrapper.find('.row-health-detail')
    expect(healthOpenBtn).toBeTruthy()
    await healthOpenBtn!.trigger('click')
    await flushPromises()

    // Modal is open
    const modal = wrapper.find('.prototype-history-modal')
    expect(modal.exists()).toBe(true)

    // Tab buttons exist (3 tabs: chart, health, config)
    const tabs = modal.findAll('.modal-tab-btn')
    expect(tabs).toHaveLength(3)
    expect(tabs[1].classes()).toContain('active')

    // Switch to health tab
    await tabs[1].trigger('click')
    expect(tabs[1].classes()).toContain('active')
    expect(modal.text()).toContain('健康概览')

    // 6-grid cards in human friendly Chinese
    const gridCards = modal.find('.health-grid-cards')
    expect(gridCards.exists()).toBe(true)
    expect(gridCards.text()).toContain('平时延迟区间')
    expect(modal.find('.health-more').text()).toContain('平时大概延迟')
    expect(gridCards.text()).toContain('较慢时的延迟')
    expect(gridCards.text()).toContain('连接通畅率')
    expect(gridCards.text()).toContain('100.0%')

    // Histogram section
    const histogram = modal.find('.health-histogram-section')
    expect(histogram.exists()).toBe(true)
    expect(histogram.text()).toContain('延迟通常落在哪')

    // Switch to config tab
    await tabs[2].trigger('click')
    expect(tabs[2].classes()).toContain('active')
    expect(modal.find('.modal-config-tab-content').isVisible()).toBe(true)
    expect(modal.text()).toContain('节点基本信息与订阅配置')

    // Can switch back to chart tab
    await tabs[0].trigger('click')
    expect(tabs[0].classes()).toContain('active')
    expect(modal.find('.modal-chart-tab-content').isVisible()).toBe(true)
  })
})
