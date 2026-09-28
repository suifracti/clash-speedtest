import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkbenchLatencyBatch, WorkbenchLatencyBatchRequest, WorkbenchLatencyTest } from '../../../types'

const mocks = vi.hoisted(() => ({
  fetchNodes: vi.fn(),
  fetchHistory: vi.fn(),
  fetchHistories: vi.fn(),
  fetchBatches: vi.fn(),
  fetchBatch: vi.fn(),
  startBatch: vi.fn(),
  cancelBatch: vi.fn(),
  retryItem: vi.fn(),
  subscribe: vi.fn(),
  eventCallback: null as null | ((type: string, payload: unknown) => void),
}))

vi.mock('../../../api/monitor', () => ({
  fetchMonitorNodeOptions: mocks.fetchNodes,
  fetchMonitorJobs: vi.fn().mockResolvedValue([]),
}))
vi.mock('../../../api/bridge', () => ({
  fetchWorkbenchLatencyHistory: mocks.fetchHistory,
  fetchWorkbenchLatencyHistories: mocks.fetchHistories,
  fetchWorkbenchLatencyBatches: mocks.fetchBatches,
  fetchWorkbenchLatencyBatch: mocks.fetchBatch,
  startWorkbenchLatencyBatch: mocks.startBatch,
  cancelWorkbenchLatencyBatch: mocks.cancelBatch,
  retryWorkbenchLatencyBatchItem: mocks.retryItem,
  subscribeEvents: mocks.subscribe,
}))

import LatencyWorkbench from '../LatencyWorkbench.vue'

const nodes = [
  { profileId: 'profile-a', profileName: '订阅 A', nodeKey: 'node-a', nodeIdentityKey: 'identity-a', configRevisionKey: 'rev-a', displayName: '同名节点', type: 'http', countryCode: 'US', countryFlag: '🇺🇸' },
  { profileId: 'profile-b', profileName: '订阅 B', nodeKey: 'node-b', nodeIdentityKey: 'identity-b', configRevisionKey: 'rev-b', displayName: '同名节点', type: 'http', countryCode: 'JP', countryFlag: '🇯🇵' },
]

function emptyHistory() { return { tests: [], since: '2026-09-23T00:00:00Z', until: '2026-09-24T00:00:00Z', as_of: '2026-09-23T12:00:00Z', has_more: false, complete: true } }

function attempt(): WorkbenchLatencyTest {
  return {
    attempt_id: 'attempt-a', profile_id: 'profile-a', node_key: 'node-a', node_identity_key: 'identity-a', config_revision_key: 'rev-a',
    display_name: '同名节点', node_type: 'http', test_project: 'latency_stability', source: 'workbench_batch_latency',
    method: 'http_get_via_proxy_first_byte', method_version: 1, target: 'https://speed.cloudflare.com/__down?bytes=1', unit: 'ms',
    requested_at: '2026-09-23T12:00:00Z', started_at: '2026-09-23T12:00:01Z', finished_at: '2026-09-23T12:00:02Z',
    status: 'partial_failed', latency_ms: 42, jitter_ms: 3, packet_loss: 50, total_samples: 2, success_samples: 1, failure_samples: 1,
    samples: [{ seq: 1, timestamp: '2026-09-23T12:00:01Z', latency_ms: 42, success: true }, { seq: 2, timestamp: '2026-09-23T12:00:02Z', latency_ms: 0, success: false, error: 'timeout' }],
    persistence_state: 'failed', persistence_error: 'injected save failure',
  }
}

function makeBatch(requestID: string, state = 'queued'): WorkbenchLatencyBatch {
  return {
    batch_id: 'batch-active', request_id: requestID, test_project: 'latency_stability', timeout_seconds: 5,
    requested_at: '2026-09-23T12:00:00Z', state, item_count: 2,
    items: nodes.map((node, ordinal) => ({
      item_id: `item-${ordinal}`, batch_id: 'batch-active', ordinal, profile_id: node.profileId, node_key: node.nodeKey,
      node_identity_key: node.nodeIdentityKey, config_revision_key: node.configRevisionKey, display_name: node.displayName, node_type: node.type,
      execution_state: state === 'queued' ? 'queued' : state === 'saving' ? 'completed' : 'running', persistence_state: state === 'saving' ? 'saving' : 'pending', requested_at: '2026-09-23T12:00:00Z',
    })),
  }
}

describe('Workbench latency batch UI contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.fetchNodes.mockResolvedValue(nodes)
    mocks.fetchHistory.mockResolvedValue(emptyHistory())
    mocks.fetchHistories.mockImplementation((queries: unknown[]) => Promise.all(queries.map((query) => mocks.fetchHistory(query))))
    mocks.fetchBatches.mockResolvedValue([])
    mocks.fetchBatch.mockResolvedValue(null)
    mocks.subscribe.mockImplementation((callback: (type: string, payload: unknown) => void) => { mocks.eventCallback = callback; return vi.fn() })
  })

  it('selects nodes at the top and submits the integrated latency suite', async () => {
    mocks.startBatch.mockImplementation(() => new Promise(() => {}))
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()
    await wrapper.get('.top-node-picker-actions button').trigger('click')
    expect(wrapper.findAll('.top-node-grid input')).toHaveLength(2)
    await wrapper.findAll('.top-node-picker-actions button')[1]!.trigger('click')
    expect(wrapper.findAll('.node-row[aria-selected="true"]')).toHaveLength(2)
    await wrapper.findAll('.top-node-grid input')[1]!.setValue(false)
    expect(wrapper.findAll('.node-row[aria-selected="true"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('综合检测 · 6 个站点')
    expect(mocks.fetchHistory).toHaveBeenLastCalledWith(expect.objectContaining({ target_id: 'all' }))
    await wrapper.findAll('button').find(button => button.text().includes('测试 同名节点'))!.trigger('click')
    expect(mocks.startBatch).toHaveBeenCalledWith(expect.objectContaining({ target_id: 'all', selections: [expect.objectContaining({ node_key: 'node-a' })] }))
    wrapper.unmount()
  })

  it('shows the actual checked node before testing even when another row is focused', async () => {
    const choices = [
      { ...nodes[0], profileId: 'profile-a', nodeKey: 'sg-09', nodeIdentityKey: 'id-09', displayName: '新加坡09aws' },
      { ...nodes[1], profileId: 'profile-a', nodeKey: 'sg-12', nodeIdentityKey: 'id-12', displayName: '新加坡12aws' },
    ]
    mocks.fetchNodes.mockResolvedValue(choices)
    mocks.startBatch.mockImplementation(() => new Promise(() => {}))
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()

    const rows = wrapper.findAll('.node-row')
    const row09 = rows.find((row) => row.text().includes('新加坡09aws'))!
    const row12 = rows.find((row) => row.text().includes('新加坡12aws'))!
    await row09.get('input[type="checkbox"]').setValue(true)
    await row12.get('.node-evidence').trigger('click')
    expect(row09.attributes('aria-selected')).toBe('true')
    expect(row12.attributes('aria-selected')).toBe('false')
    expect(wrapper.get('.selection-count').text()).toContain('新加坡09aws')
    expect(wrapper.findAll('button').find((button) => button.text().includes('测试 新加坡09aws'))).toBeTruthy()

    await row09.get('input[type="checkbox"]').setValue(false)
    await row12.get('input[type="checkbox"]').setValue(true)
    await wrapper.findAll('button').find((button) => button.text().includes('测试 新加坡12aws'))!.trigger('click')
    expect(mocks.startBatch.mock.calls[0][0].selections).toMatchObject([{ profile_id: 'profile-a', node_key: 'sg-12', node_identity_key: 'id-12' }])
    wrapper.unmount()
  })

  it('selects multiple subscriptions and measures an exact duplicate configuration only once', async () => {
    const choices = [
      { ...nodes[0], profileId: 'profile-a', profileName: '机场甲', nodeKey: 'a-shared', nodeIdentityKey: 'shared-endpoint', configRevisionKey: 'shared-config', displayName: '甲 · 共享节点' },
      { ...nodes[0], profileId: 'profile-b', profileName: '机场乙', nodeKey: 'b-shared', nodeIdentityKey: 'shared-endpoint', configRevisionKey: 'shared-config', displayName: '乙 · 共享节点' },
      { ...nodes[1], profileId: 'profile-b', profileName: '机场乙', nodeKey: 'b-own', nodeIdentityKey: 'b-endpoint', configRevisionKey: 'b-config', displayName: '乙 · 独立节点' },
      { ...nodes[1], profileId: 'profile-c', profileName: '机场丙', nodeKey: 'c-own', nodeIdentityKey: 'c-endpoint', configRevisionKey: 'c-config', displayName: '丙 · 独立节点' },
    ]
    mocks.fetchNodes.mockResolvedValue(choices)
    mocks.startBatch.mockImplementation(() => new Promise(() => {}))
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()

    const picker = wrapper.get('details.profile-scope-picker')
    ;(picker.element as HTMLDetailsElement).open = true
    await picker.trigger('toggle')
    const profileChecks = wrapper.findAll('.profile-scope-option input[type="checkbox"]')
    expect(profileChecks).toHaveLength(3)
    await wrapper.findAll('.profile-scope-option').find(option => option.text().includes('机场丙'))!.get('input[type="checkbox"]').setValue(false)
    await flushPromises()
    expect(wrapper.get('.profile-scope-head').text()).toContain('2 个订阅')

    for (const row of wrapper.findAll('.node-row')) await row.get('input[type="checkbox"]').setValue(true)
    expect(wrapper.get('.selection-summary').text()).toContain('实测 2 个配置')
    await wrapper.get('.project-bar-actions .prototype-button.primary').trigger('click')

    const request = mocks.startBatch.mock.calls[0][0] as WorkbenchLatencyBatchRequest
    expect(request.selections).toHaveLength(2)
    expect(request.selections).toEqual(expect.arrayContaining([
      expect.objectContaining({ node_identity_key: 'shared-endpoint', config_revision_key: 'shared-config' }),
      expect.objectContaining({ profile_id: 'profile-b', node_key: 'b-own' }),
    ]))
    wrapper.unmount()
  })

  it('shows one airport line by default and allows its other subscription configuration to be tested separately', async () => {
    mocks.fetchNodes.mockResolvedValue([
      { ...nodes[0], profileId: 'default', profileName: '飞鸟云 · 默认订阅', nodeKey: 'first', nodeIdentityKey: 'same-endpoint', configRevisionKey: 'credential-a' },
      { ...nodes[0], profileId: 'second', profileName: '飞鸟云 · 2', nodeKey: 'second', nodeIdentityKey: 'same-endpoint', configRevisionKey: 'credential-b' },
    ])
    mocks.startBatch.mockImplementation(() => new Promise(() => {}))
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()

    expect(wrapper.findAll('.node-row')).toHaveLength(1)
    expect(wrapper.get('.node-meta').text()).toContain('飞鸟云')
    expect(wrapper.get('.node-meta').text()).not.toContain('默认订阅')
    await wrapper.get('.alternate-config-toggle').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.node-row')).toHaveLength(2)
    expect(wrapper.text()).toContain('飞鸟云 / 2')
    for (const row of wrapper.findAll('.node-row')) await row.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('.project-bar-actions .prototype-button.primary').trigger('click')
    expect((mocks.startBatch.mock.calls[0][0] as WorkbenchLatencyBatchRequest).selections).toHaveLength(2)
    wrapper.unmount()
  })

  it('shows the latest saved batch node and its new sample first after reopening', async () => {
    const choices = [
      { ...nodes[0], nodeKey: 'sg-12', nodeIdentityKey: 'id-12', displayName: '新加坡12aws' },
      { ...nodes[0], nodeKey: 'sg-13', nodeIdentityKey: 'id-13', displayName: '新加坡13aws' },
    ]
    const timestamp = new Date(Date.now() - 1000).toISOString()
    const savedTest = (node: typeof choices[number], latency: number): WorkbenchLatencyTest => ({
      ...attempt(), attempt_id: `attempt-${node.nodeKey}`, profile_id: node.profileId, node_key: node.nodeKey,
      node_identity_key: node.nodeIdentityKey, config_revision_key: node.configRevisionKey, display_name: node.displayName,
      finished_at: timestamp, status: 'completed', latency_ms: latency, total_samples: 1, success_samples: 1, failure_samples: 0,
      persistence_state: 'saved', persistence_error: '', samples: [{ seq: 1, timestamp, latency_ms: latency, success: true }],
    })
    mocks.fetchNodes.mockResolvedValue(choices)
    mocks.fetchHistory.mockImplementation((query: { node_key: string }) => ({
      ...emptyHistory(), tests: [savedTest(choices.find((node) => node.nodeKey === query.node_key)!, query.node_key === 'sg-12' ? 225 : 107)],
    }))
    mocks.fetchBatches.mockResolvedValue([{ ...makeBatch('latest', 'completed'), batch_id: 'latest-batch', items: undefined, item_count: 1 }])
    mocks.fetchBatch.mockResolvedValue({
      ...makeBatch('latest', 'completed'), batch_id: 'latest-batch', item_count: 1,
      items: [{ ...makeBatch('latest', 'completed').items![0], batch_id: 'latest-batch', node_key: 'sg-12', node_identity_key: 'id-12',
        display_name: '新加坡12aws', execution_state: 'completed', persistence_state: 'saved', result: savedTest(choices[0], 225) }],
    })
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()

    expect(mocks.fetchBatch).toHaveBeenCalledWith('latest-batch')
    wrapper.findAllComponents({ name: 'UiSelect' }).find((select) => select.attributes('aria-label') === '节点排序')!.vm.$emit('update:modelValue', 'recent')
    await flushPromises()
    const rows = wrapper.findAll('.node-row')
    expect(rows.map((row) => row.get('.node-name').text())).toEqual(['新加坡12aws', '新加坡13aws'])
    expect(rows[0].get('.row-readout-value').text()).toContain('225')
    expect(rows[0].findComponent({ name: 'LatencySamplePlot' }).props('samples')).toEqual(expect.arrayContaining([
      expect.objectContaining({ timestamp, latency_ms: 225 }),
    ]))
    expect(wrapper.get('.recent-batch-order-note').text()).toContain('最近一次测速的节点排在前面')
    wrapper.unmount()
  })

  it('updates the tested node chart immediately when its batch finishes', async () => {
    const choices = [
      { ...nodes[0], nodeKey: 'sg-12', nodeIdentityKey: 'id-12', displayName: '新加坡12aws' },
      { ...nodes[0], nodeKey: 'sg-13', nodeIdentityKey: 'id-13', displayName: '新加坡13aws' },
    ]
    const oldTime = new Date(Date.now() - 60000).toISOString()
    const newTime = new Date(Date.now() - 1000).toISOString()
    const savedTest = (node: typeof choices[number], latency: number, timestamp: string): WorkbenchLatencyTest => ({
      ...attempt(), attempt_id: `attempt-${node.nodeKey}-${timestamp}`, profile_id: node.profileId, node_key: node.nodeKey,
      node_identity_key: node.nodeIdentityKey, config_revision_key: node.configRevisionKey, display_name: node.displayName,
      finished_at: timestamp, status: 'completed', latency_ms: latency, total_samples: 1, success_samples: 1, failure_samples: 0,
      persistence_state: 'saved', persistence_error: '', samples: [{ seq: 1, timestamp, latency_ms: latency, success: true }],
    })
    mocks.fetchNodes.mockResolvedValue(choices)
    mocks.fetchHistory.mockImplementation((query: { node_key: string }) => ({
      ...emptyHistory(), tests: [savedTest(choices.find((node) => node.nodeKey === query.node_key)!, query.node_key === 'sg-12' ? 185 : 107, oldTime)],
    }))
    mocks.startBatch.mockImplementation((request: WorkbenchLatencyBatchRequest) => Promise.resolve({
      ...makeBatch(request.request_id), item_count: 1,
      items: [{ ...makeBatch(request.request_id).items![0], node_key: 'sg-12', node_identity_key: 'id-12', display_name: '新加坡12aws' }],
    }))
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()
    expect(wrapper.findAll('.node-row')[0].get('.node-name').text()).toBe('新加坡13aws')

    await wrapper.findAll('.node-row').find((row) => row.text().includes('新加坡12aws'))!.get('input[type="checkbox"]').setValue(true)
    await wrapper.findAll('button').find((button) => button.text().includes('测试 新加坡12aws'))!.trigger('click')
    await flushPromises()
    const request = mocks.startBatch.mock.calls[0][0] as WorkbenchLatencyBatchRequest
    const completed = savedTest(choices[0], 225, newTime)
    mocks.eventCallback?.('workbench_latency_batch_updated', {
      ...makeBatch(request.request_id, 'completed'), item_count: 1,
      items: [{ ...makeBatch(request.request_id, 'completed').items![0], node_key: 'sg-12', node_identity_key: 'id-12',
        display_name: '新加坡12aws', execution_state: 'completed', persistence_state: 'saved', result: completed }],
    })
    await flushPromises()

    const firstRow = wrapper.findAll('.node-row').find((row) => row.get('.node-name').text() === '新加坡12aws')!
    expect(firstRow.get('.node-name').text()).toBe('新加坡12aws')
    expect(firstRow.get('.row-readout-value').text()).toContain('225')
    expect(firstRow.findComponent({ name: 'LatencySamplePlot' }).props('samples')).toEqual(expect.arrayContaining([
      expect.objectContaining({ timestamp: newTime, latency_ms: 225 }),
    ]))
    wrapper.unmount()
  })

  it('freezes selected identities and keeps cancel, item save failure, retry, and late-batch events scoped', async () => {
    let resolveStart!: (batch: WorkbenchLatencyBatch) => void
    mocks.startBatch.mockImplementation((_request: WorkbenchLatencyBatchRequest) => new Promise<WorkbenchLatencyBatch>((resolve) => { resolveStart = resolve }))
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(2)
    await checkboxes[0].setValue(true)
    await checkboxes[1].setValue(true)
    await wrapper.findAll('button').find((button) => button.text().includes('测试所选 2 个节点'))!.trigger('click')
    await flushPromises()

    const request = mocks.startBatch.mock.calls[0][0] as WorkbenchLatencyBatchRequest
    expect(request.selections.map(({ profile_id, node_key, node_identity_key, config_revision_key }) => [profile_id, node_key, node_identity_key, config_revision_key])).toEqual([
      ['profile-a', 'node-a', 'identity-a', 'rev-a'], ['profile-b', 'node-b', 'identity-b', 'rev-b'],
    ])
    await checkboxes[0].setValue(false)
    await checkboxes[1].setValue(false)
    resolveStart(makeBatch(request.request_id))
    await flushPromises()
    expect(wrapper.text()).toContain('identity-a')
    expect(wrapper.text()).toContain('identity-b')

    const runningBatch = makeBatch(request.request_id, 'running')
    mocks.eventCallback?.('workbench_latency_batch_updated', runningBatch)
    await flushPromises()
    mocks.cancelBatch.mockResolvedValue({ ...runningBatch, state: 'cancelling' })
    const cancelButton = wrapper.findAll('button').find((button) => button.text().includes('取消本批次'))
    expect(cancelButton).toBeTruthy()
    await cancelButton!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('正在取消')

    const savingBatch = makeBatch(request.request_id, 'saving')
    mocks.eventCallback?.('workbench_latency_batch_updated', savingBatch)
    await flushPromises()
    expect(wrapper.text()).toContain('测量已结束，结果保存中')
    expect(wrapper.find('.batch-detail-heading').text()).toContain('0 个结果已保存')
    expect(wrapper.find('.batch-detail-heading').text()).toContain('保存中 2')
    expect(wrapper.find('progress').attributes('value')).toBe('2')
    expect(wrapper.findAll('button').find((button) => button.text().includes('结果保存中'))?.attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('button').some((button) => button.text().includes('取消本批次'))).toBe(false)
    expect(mocks.startBatch).toHaveBeenCalledTimes(1)

    mocks.eventCallback?.('workbench_latency_batch_updated', { ...makeBatch('old-request', 'running'), batch_id: 'batch-old' })
    await flushPromises()
    expect(wrapper.text()).toContain('batch-active')
    expect(wrapper.text()).not.toContain('batch-old')

    const failedBatch = makeBatch(request.request_id, 'completed_with_save_failures')
    failedBatch.items![0] = { ...failedBatch.items![0], execution_state: 'completed', persistence_state: 'failed', attempt_id: 'attempt-a', persistence_error: 'injected save failure', result: attempt() }
    failedBatch.items![1] = { ...failedBatch.items![1], execution_state: 'completed', persistence_state: 'saved', attempt_id: 'attempt-b' }
    mocks.eventCallback?.('workbench_latency_batch_updated', failedBatch)
    await flushPromises()
    expect(wrapper.text()).toContain('保存失败')
    expect(wrapper.text()).toContain('http_get_via_proxy_first_byte')
    await wrapper.findAll('.batch-filters button').find(button => button.text().includes('需关注'))!.trigger('click')
    expect(wrapper.findAll('.batch-item-row')).toHaveLength(1)
    expect(wrapper.find('.batch-item-row').text()).toContain('订阅 A')
    mocks.retryItem.mockResolvedValue({ ...failedBatch, state: 'completed_with_issues', items: failedBatch.items!.map((item) => item.item_id === 'item-0' ? { ...item, persistence_state: 'saved', persistence_error: '' } : item) })
    await wrapper.findAll('button').find((button) => button.text().includes('重试保存'))!.trigger('click')
    await flushPromises()
    expect(mocks.retryItem).toHaveBeenCalledWith('batch-active', 'item-0')
    wrapper.unmount()
  })

  it('opens composite test modal and validates options', async () => {
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[0].setValue(true)
    await flushPromises()

    const compositeBtn = wrapper.findAll('button').find((button) => button.text().includes('自选组合测试'))
    expect(compositeBtn).toBeTruthy()
    await compositeBtn!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('选择要测试的项目与参数')
    expect(wrapper.text()).toContain('延迟与稳定性探测')
    expect(wrapper.text()).toContain('下载速度测量')
    expect(wrapper.text()).toContain('公共服务即时连通性')

    await wrapper.get('.close-button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('选择要测试的项目与参数')

    wrapper.unmount()
  })

  it('displays batch action button and sequential testing hint when multiple nodes are selected in throughput and service projects', async () => {
    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true, WorkbenchDownloadPanel: true, WorkbenchPublicServicePanel: true } } })
    await flushPromises()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[0].setValue(true)
    await checkboxes[1].setValue(true)
    await flushPromises()

    // Switch to throughput tab
    const tabs = wrapper.findAll('.workspace-modes button')
    const throughputTab = tabs.find((b) => b.text().includes('下载测速'))
    expect(throughputTab).toBeTruthy()
    await throughputTab!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('排队测速所选节点 (2 个)')
    expect(wrapper.text()).toContain('逐个下载，避免带宽争抢')

    // Switch to service tab
    const serviceTab = tabs.find((b) => b.text().includes('服务检测'))
    expect(serviceTab).toBeTruthy()
    await serviceTab!.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('.selection-summary')[1]!.text()).toContain('已选 2 条线路')
    await wrapper.findAll('.selection-summary')[1]!.trigger('click')
    expect(wrapper.find('.workbench-sticky-wrapper').exists()).toBe(false)
    expect(wrapper.find('.category-tabs-group').exists()).toBe(false)
    await wrapper.get('input[aria-label="搜索待测节点"]').setValue('订阅 A')
    expect(wrapper.findAll('.node-choices label')).toHaveLength(1)
    expect(wrapper.findAll('.overview-row')).toHaveLength(0)
    expect(wrapper.find('.compare-footer').text()).toContain('当前范围 1 / 2 条线路')

    wrapper.unmount()
  })

  it('filters out non-proxy announcement nodes by default and isolates them in dedicated notices page', async () => {
    const mixedNodes = [
      { profileId: 'profile-a', profileName: '飞鸟云', nodeKey: 'node-real', nodeIdentityKey: 'identity-real', configRevisionKey: 'rev-a', displayName: '🇭🇰 香港 01', type: 'trojan', countryCode: 'HK', countryFlag: '🇭🇰' },
      { profileId: 'profile-a', profileName: '飞鸟云', nodeKey: 'node-tg', nodeIdentityKey: 'identity-tg', configRevisionKey: 'rev-a', displayName: '电报群https://t.me/feiniaoyunjichang', type: 'trojan', countryCode: 'OTHER', countryFlag: '' },
      { profileId: 'profile-a', profileName: '飞鸟云', nodeKey: 'node-backup', nodeIdentityKey: 'identity-backup', configRevisionKey: 'rev-a', displayName: '防失联页https://github.com/feiniaoyun', type: 'trojan', countryCode: 'OTHER', countryFlag: '' },
    ]
    mocks.fetchNodes.mockResolvedValueOnce(mixedNodes)

    const wrapper = mount(LatencyWorkbench, { global: { stubs: { UiSelect: true, LatencySamplePlot: true } } })
    await flushPromises()

    // By default, only the 1 real proxy node is shown
    expect(wrapper.text()).toContain('代理节点 (1)')
    expect(wrapper.text()).toContain('公告与信息 (2)')
    expect(wrapper.text()).toContain('🇭🇰 香港 01')
    expect(wrapper.text()).not.toContain('电报群https://t.me/feiniaoyunjichang')
    expect(wrapper.text()).not.toContain('防失联页https://github.com/feiniaoyun')
    expect(wrapper.text()).toContain('2 个公告条目已隔离')

    // Click category tab "公告与信息"
    const noticeTab = wrapper.findAll('button.category-tab').find((b) => b.text().includes('公告与信息'))
    expect(noticeTab).toBeTruthy()
    await noticeTab!.trigger('click')
    await flushPromises()

    // Now in the notices page
    expect(wrapper.text()).toContain('订阅公告与信息')
    expect(wrapper.text()).toContain('电报群https://t.me/feiniaoyunjichang')
    expect(wrapper.text()).toContain('防失联页https://github.com/feiniaoyun')
    expect(wrapper.text()).toContain('打开电报群/频道')
    expect(wrapper.text()).toContain('打开 GitHub / 防失联页')
    expect(wrapper.text()).not.toContain('🇭🇰 香港 01')

    // Switch back to "代理节点"
    const proxyTab = wrapper.findAll('button.category-tab').find((b) => b.text().includes('代理节点'))
    await proxyTab!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('🇭🇰 香港 01')
    expect(wrapper.text()).not.toContain('电报群https://t.me/feiniaoyunjichang')

    wrapper.unmount()
  })
})
