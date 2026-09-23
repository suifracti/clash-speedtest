import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkbenchLatencyBatch, WorkbenchLatencyBatchRequest, WorkbenchLatencyTest } from '../../../types'

const mocks = vi.hoisted(() => ({
  fetchNodes: vi.fn(),
  fetchHistory: vi.fn(),
  fetchBatches: vi.fn(),
  startBatch: vi.fn(),
  cancelBatch: vi.fn(),
  retryItem: vi.fn(),
  subscribe: vi.fn(),
  eventCallback: null as null | ((type: string, payload: unknown) => void),
}))

vi.mock('../../../api/monitor', () => ({ fetchMonitorNodeOptions: mocks.fetchNodes }))
vi.mock('../../../api/bridge', () => ({
  fetchWorkbenchLatencyHistory: mocks.fetchHistory,
  fetchWorkbenchLatencyBatches: mocks.fetchBatches,
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
    method: 'http_get_via_proxy_first_byte', method_version: 1, target: 'https://probe.example/__down?bytes=1', unit: 'ms',
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
    mocks.fetchBatches.mockResolvedValue([])
    mocks.subscribe.mockImplementation((callback: (type: string, payload: unknown) => void) => { mocks.eventCallback = callback; return vi.fn() })
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
    await wrapper.findAll('button').find((button) => button.text().includes('测试所选节点'))!.trigger('click')
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
    mocks.retryItem.mockResolvedValue({ ...failedBatch, state: 'completed_with_issues', items: failedBatch.items!.map((item) => item.item_id === 'item-0' ? { ...item, persistence_state: 'saved', persistence_error: '' } : item) })
    await wrapper.findAll('button').find((button) => button.text().includes('重试保存'))!.trigger('click')
    await flushPromises()
    expect(mocks.retryItem).toHaveBeenCalledWith('batch-active', 'item-0')
    wrapper.unmount()
  })
})
