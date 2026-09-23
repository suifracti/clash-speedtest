import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import NodeDetailView from '../NodeDetailView.vue'
import type { NodeDetailRequest } from '../../../types'

const bridgeMocks = vi.hoisted(() => ({
  fetchNodeHistoryRevisions: vi.fn(),
  fetchWorkbenchLatencyHistory: vi.fn(),
  fetchWorkbenchLatencyTest: vi.fn(),
  fetchWorkbenchLatencyBatch: vi.fn(),
  fetchWorkbenchPublicServiceHistory: vi.fn(),
  fetchWorkbenchPublicServiceAttempt: vi.fn(),
  startWorkbenchLatencyBatch: vi.fn(),
  runWorkbenchLatencyTest: vi.fn(),
  startWorkbenchPublicServiceTest: vi.fn(),
}))
const monitorMocks = vi.hoisted(() => ({
  fetchMonitorNodeOptions: vi.fn(),
  fetchMonitorStats: vi.fn(),
  queryMonitorSamplesCursor: vi.fn(),
  triggerMonitorJob: vi.fn(),
}))

vi.mock('../../../api/bridge', () => bridgeMocks)
vi.mock('../../../api/monitor', () => monitorMocks)

const baseScope: NodeDetailRequest = {
  profileId: 'profile-a', profileName: '订阅 A', nodeKey: 'node-a', nodeIdentityKey: 'identity-a',
  configRevisionKey: 'rev-a', displayName: '同名节点', nodeType: 'http',
}

function monitorSample(sampleId: string, revision = 'rev-a') {
  return {
    sampleId, runId: `run-${sampleId}`, samplingTier: 'regular' as const, triggerType: 'scheduled' as const,
    samplingStrategyVersion: 1, nodeKey: 'node-a', nodeIdentityKey: 'identity-a', configRevisionKey: revision,
    profileId: 'profile-a', displayNameSnapshot: '同名节点', probeType: 'rtt', target: 'https://probe.example/204',
    timestampMs: Date.parse('2026-09-23T09:00:00Z'), timestampIso: '2026-09-23T09:00:00Z',
    success: true, latencyMs: 35, ttfbMs: 0, errorClass: 'none',
  }
}

function stats() {
  return {
    sampleCount: 1, includedSamplingTiers: ['regular'], regularObservationOnly: true,
    successCount: 1, failureCount: 0, successRate: 1, latencyMinMs: 35, latencyP50Ms: 35,
    latencyP95Ms: 35, latencyMaxMs: 35, ttfbP50Ms: null, ttfbP95Ms: null,
    errorBreakdown: {}, firstSampleAtMs: Date.parse('2026-09-23T09:00:00Z'), lastSampleAtMs: Date.parse('2026-09-23T09:00:00Z'),
  }
}

function latencyTest(attemptId: string) {
  return {
    attempt_id: attemptId, profile_id: 'profile-a', node_key: 'node-a', node_identity_key: 'identity-a',
    config_revision_key: 'rev-a', display_name: '同名节点', node_type: 'http', test_project: 'latency_stability',
    source: 'workbench_manual', method: 'http_status', method_version: 1, target: 'https://probe.example/204', unit: 'ms',
    requested_at: '2026-09-23T09:00:00Z', started_at: '2026-09-23T09:00:00Z', finished_at: '2026-09-23T09:00:01Z',
    status: 'completed' as const, latency_ms: 35, jitter_ms: 2, packet_loss: 0, total_samples: 1,
    success_samples: 1, failure_samples: 0, samples: [{ seq: 1, timestamp: '2026-09-23T09:00:01Z', latency_ms: 35, success: true }],
    persistence_state: 'saved' as const,
  }
}

function publicServiceAttempt(attemptId: string, revision = 'rev-a', serviceId = 'github_api_root') {
  return {
    attempt_id: attemptId, request_id: `request-${attemptId}`, profile_id: 'profile-a', node_key: 'node-a',
    node_identity_key: 'identity-a', config_revision_key: revision, display_name: '同名节点', node_type: 'http',
    source: 'workbench_public_service', service_id: serviceId,
    rule: { service_id: serviceId, name: 'GitHub 公共 API 根端点', rule_version: 1, target_url: 'https://api.github.com/', method: 'GET', success_criterion: 'HTTP 200 and root index JSON', redirect_policy: 'do_not_follow', timeout_seconds: 10, maximum_body_bytes: 65536 },
    requested_at: '2026-09-23T09:00:00Z', started_at: '2026-09-23T09:00:00Z', finished_at: '2026-09-23T09:00:01Z',
    execution_state: 'completed', persistence_state: 'saved',
    result: { outcome: 'matched', http_status: 200, bytes_read: 128, started_at: '2026-09-23T09:00:00Z', finished_at: '2026-09-23T09:00:01Z', duration_ms: 1000 },
  }
}

function setupDefaults(): void {
  monitorMocks.fetchMonitorNodeOptions.mockResolvedValue([{
    profileId: 'profile-a', profileName: '订阅 A', nodeKey: 'node-a', nodeIdentityKey: 'identity-a',
    configRevisionKey: 'rev-a', displayName: '同名节点', type: 'http', countryCode: '', countryFlag: '',
  }])
  monitorMocks.fetchMonitorStats.mockResolvedValue(stats())
  monitorMocks.queryMonitorSamplesCursor.mockImplementation(async (query: { configRevisionKey?: string }) => ({
    items: [monitorSample(`sample-${query.configRevisionKey || 'all'}`, query.configRevisionKey)], nextCursor: '', hasMore: false, limit: 50,
  }))
  bridgeMocks.fetchNodeHistoryRevisions.mockResolvedValue([
    { config_revision_key: 'rev-a', node_key: 'node-a', display_name: '同名节点', last_observed_at: '2026-09-23T09:00:00Z' },
    { config_revision_key: 'rev-b', node_key: 'node-a-rev-b', display_name: '同名节点', last_observed_at: '2026-09-22T09:00:00Z' },
  ])
  bridgeMocks.fetchWorkbenchLatencyHistory.mockResolvedValue({ tests: [latencyTest('attempt-a')], since: '', until: '', as_of: '', has_more: false, complete: true })
  bridgeMocks.fetchWorkbenchLatencyTest.mockResolvedValue(latencyTest('attempt-origin'))
  bridgeMocks.fetchWorkbenchPublicServiceHistory.mockResolvedValue({ attempts: [], since: '', until: '', has_more: false, complete: true })
  bridgeMocks.fetchWorkbenchPublicServiceAttempt.mockResolvedValue(publicServiceAttempt('service-origin'))
  bridgeMocks.fetchWorkbenchLatencyBatch.mockResolvedValue({ batch_id: 'batch-a', request_id: 'req', test_project: 'latency_stability', timeout_seconds: 5, requested_at: '', state: 'completed', item_count: 1, items: [] })
}

let wrapper: VueWrapper | null = null

beforeEach(() => { vi.clearAllMocks(); setupDefaults() })
afterEach(() => { wrapper?.unmount(); wrapper = null })

describe('NodeDetailView', () => {
  it('queries by profile, stable identity and exact revision while showing independent Monitor and Workbench facts', async () => {
    wrapper = mount(NodeDetailView, { props: { scope: baseScope } })
    await flushPromises()

    expect(monitorMocks.queryMonitorSamplesCursor).toHaveBeenCalledWith(expect.objectContaining({ profileId: 'profile-a', nodeIdentityKey: 'identity-a', configRevisionKey: 'rev-a', regularObservationOnly: true }))
    expect(monitorMocks.fetchMonitorStats).toHaveBeenCalledWith(expect.objectContaining({ profileId: 'profile-a', nodeIdentityKey: 'identity-a', configRevisionKey: 'rev-a', regularObservationOnly: true }))
    expect(bridgeMocks.fetchWorkbenchLatencyHistory).toHaveBeenCalledWith(expect.objectContaining({ profile_id: 'profile-a', node_identity_key: 'identity-a', config_revision_key: 'rev-a', node_key: 'node-a' }))
    expect(wrapper.text()).toContain('1 个样本')
    expect(wrapper.text()).toContain('attempt-a')
    expect(wrapper.text()).toContain('常规观测')
    const rawDetails = wrapper.findAll('details').find((item) => item.text().includes('查看样本图与 1 条原始样本'))
    expect(rawDetails).toBeDefined()
    await rawDetails!.find('summary').trigger('click')
    expect(rawDetails!.find('svg[aria-label="延迟原始样本图，可用方向键浏览，回车固定，Esc 取消固定"]').exists()).toBe(true)
    expect(bridgeMocks.startWorkbenchLatencyBatch).not.toHaveBeenCalled()
    expect(bridgeMocks.runWorkbenchLatencyTest).not.toHaveBeenCalled()
    expect(monitorMocks.triggerMonitorJob).not.toHaveBeenCalled()

    bridgeMocks.fetchWorkbenchLatencyHistory.mockImplementation(async (query: { config_revision_key: string; node_key: string }) => ({
      tests: [{ ...latencyTest('attempt-b'), config_revision_key: query.config_revision_key, node_key: query.node_key }],
      since: '', until: '', as_of: '', has_more: false, complete: true,
    }))
    await wrapper.get('select[aria-label="配置 revision"]').setValue('rev-b')
    await flushPromises()
    expect(bridgeMocks.fetchWorkbenchLatencyHistory).toHaveBeenLastCalledWith(expect.objectContaining({ node_identity_key: 'identity-a', config_revision_key: 'rev-b', node_key: 'node-a-rev-b' }))
    expect(monitorMocks.fetchMonitorStats).toHaveBeenLastCalledWith(expect.objectContaining({ nodeIdentityKey: 'identity-a', configRevisionKey: 'rev-b' }))
    expect(wrapper.text()).toContain('attempt-b')
  })

  it('keeps deleted-profile history readable and never reports failed Monitor reads as an empty history', async () => {
    monitorMocks.fetchMonitorNodeOptions.mockResolvedValue([])
    monitorMocks.queryMonitorSamplesCursor.mockRejectedValue(new Error('database unavailable'))
    bridgeMocks.fetchNodeHistoryRevisions.mockRejectedValue(new Error('revision database unavailable'))
    wrapper = mount(NodeDetailView, { props: { scope: baseScope } })
    await flushPromises()

    expect(wrapper.text()).toContain('历史节点 · 当前订阅缓存中不可用')
    expect(wrapper.text()).toContain('Monitor raw 读取失败：database unavailable')
    expect(wrapper.text()).toContain('revision 列表读取失败：revision database unavailable')
    expect(wrapper.text()).not.toContain('所选身份、revision、来源和窗口内没有 raw 样本')
    expect(wrapper.text()).toContain('attempt-a')
  })

  it('renders a node with no history when revision reads return null without starting measurements', async () => {
    monitorMocks.fetchMonitorNodeOptions.mockResolvedValue([])
    monitorMocks.fetchMonitorStats.mockResolvedValue({ ...stats(), sampleCount: 0, successCount: 0, failureCount: 0, successRate: 0, includedSamplingTiers: [], firstSampleAtMs: 0, lastSampleAtMs: 0 })
    monitorMocks.queryMonitorSamplesCursor.mockResolvedValue({ items: [], nextCursor: '', hasMore: false, limit: 50 })
    bridgeMocks.fetchNodeHistoryRevisions.mockResolvedValue(null)
    bridgeMocks.fetchWorkbenchLatencyHistory.mockResolvedValue({ tests: [], since: '', until: '', as_of: '', has_more: false, complete: true })
    const scope = { ...baseScope, nodeKey: 'new-node', configRevisionKey: 'entry-revision', displayName: '新节点' }
    wrapper = mount(NodeDetailView, { props: { scope } })
    await flushPromises()

    const revisionSelect = wrapper.get('select[aria-label="配置 revision"]')
    expect(revisionSelect.find('option[value="entry-revision"]').exists()).toBe(true)
    expect(revisionSelect.text()).toContain('当前/入口版本')
    expect(wrapper.text()).toContain('所选身份、revision、来源和窗口内没有 raw 样本')
    expect(wrapper.text()).toContain('此身份、revision 和请求窗口内没有已保存 attempt')
    expect(wrapper.text()).toContain('实际样本 无样本')
    expect(wrapper.text()).not.toContain('1970')
    expect(wrapper.text()).not.toContain('revision 列表读取失败')
    expect(bridgeMocks.fetchWorkbenchLatencyHistory).toHaveBeenCalledWith(expect.objectContaining({ profile_id: 'profile-a', node_identity_key: 'identity-a', config_revision_key: 'entry-revision' }))
    expect(monitorMocks.queryMonitorSamplesCursor).toHaveBeenCalledWith(expect.objectContaining({ profileId: 'profile-a', nodeIdentityKey: 'identity-a', configRevisionKey: 'entry-revision' }))
    expect(bridgeMocks.startWorkbenchLatencyBatch).not.toHaveBeenCalled()
    expect(bridgeMocks.runWorkbenchLatencyTest).not.toHaveBeenCalled()
    expect(monitorMocks.triggerMonitorJob).not.toHaveBeenCalled()
  })

  it('ignores an older source response after the user changes the source filter', async () => {
    let releaseOld!: (page: { items: ReturnType<typeof monitorSample>[]; nextCursor: string; hasMore: boolean; limit: number }) => void
    monitorMocks.queryMonitorSamplesCursor.mockImplementationOnce(() => new Promise((resolve) => { releaseOld = resolve }))
    monitorMocks.queryMonitorSamplesCursor.mockImplementationOnce(async () => ({ items: [monitorSample('diagnostic-current')], nextCursor: '', hasMore: false, limit: 50 }))
    wrapper = mount(NodeDetailView, { props: { scope: baseScope } })
    await wrapper.get('select[aria-label="Monitor 来源"]').setValue('diagnostic')
    await flushPromises()
    releaseOld({ items: [monitorSample('stale-regular')], nextCursor: '', hasMore: false, limit: 50 })
    await flushPromises()

    expect(wrapper.text()).toContain('diagnostic-current')
    expect(wrapper.text()).not.toContain('stale-regular')
    expect(monitorMocks.queryMonitorSamplesCursor).toHaveBeenLastCalledWith(expect.objectContaining({ samplingTier: 'diagnostic' }))
  })

  it('does not let a late response from the previous revision replace the selected revision', async () => {
    let releaseOld!: (page: { items: ReturnType<typeof monitorSample>[]; nextCursor: string; hasMore: boolean; limit: number }) => void
    monitorMocks.queryMonitorSamplesCursor.mockImplementationOnce(() => new Promise((resolve) => { releaseOld = resolve }))
    monitorMocks.queryMonitorSamplesCursor.mockImplementationOnce(async () => ({ items: [monitorSample('current-rev-b', 'rev-b')], nextCursor: '', hasMore: false, limit: 50 }))
    wrapper = mount(NodeDetailView, { props: { scope: baseScope } })
    await flushPromises()
    await wrapper.get('select[aria-label="配置 revision"]').setValue('rev-b')
    await flushPromises()
    releaseOld({ items: [monitorSample('stale-rev-a', 'rev-a')], nextCursor: '', hasMore: false, limit: 50 })
    await flushPromises()

    expect(wrapper.text()).toContain('current-rev-b')
    expect(wrapper.text()).not.toContain('stale-rev-a')
    expect(monitorMocks.queryMonitorSamplesCursor).toHaveBeenLastCalledWith(expect.objectContaining({ configRevisionKey: 'rev-b' }))
  })

  it('loads the service-origin attempt and filters read-only history by identity, revision, and service', async () => {
    const attempt = publicServiceAttempt('service-origin')
    bridgeMocks.fetchWorkbenchPublicServiceHistory.mockResolvedValue({ attempts: [attempt], since: '', until: '', has_more: false, complete: true })
    bridgeMocks.fetchWorkbenchPublicServiceAttempt.mockResolvedValue(attempt)
    const scope: NodeDetailRequest = {
      ...baseScope,
      origin: { kind: 'public_service_attempt', attemptId: attempt.attempt_id, serviceId: attempt.service_id, observedAt: attempt.result.finished_at, snapshot: attempt },
    }
    wrapper = mount(NodeDetailView, { props: { scope } })
    await flushPromises()

    expect(bridgeMocks.fetchWorkbenchPublicServiceHistory).toHaveBeenCalledWith(expect.objectContaining({
      profile_id: 'profile-a', node_key: 'node-a', node_identity_key: 'identity-a', config_revision_key: 'rev-a', service_id: 'github_api_root',
    }))
    expect(bridgeMocks.fetchWorkbenchPublicServiceAttempt).toHaveBeenCalledWith('service-origin', expect.objectContaining({
      profile_id: 'profile-a', node_identity_key: 'identity-a', config_revision_key: 'rev-a', service_id: 'github_api_root',
    }))
    expect(wrapper.text()).toContain('GitHub 公共 API 根端点')
    expect(wrapper.text()).toContain('符合判据')
    expect(wrapper.text()).toContain('attempt service-origin')
    expect(bridgeMocks.startWorkbenchPublicServiceTest).not.toHaveBeenCalled()
  })

  it('ignores a late service-history page after the revision changes', async () => {
    let releaseOld!: (page: { attempts: ReturnType<typeof publicServiceAttempt>[]; since: string; until: string; has_more: boolean; complete: boolean }) => void
    bridgeMocks.fetchWorkbenchPublicServiceHistory.mockImplementationOnce(() => new Promise((resolve) => { releaseOld = resolve }))
    bridgeMocks.fetchWorkbenchPublicServiceHistory.mockResolvedValueOnce({ attempts: [{ ...publicServiceAttempt('service-rev-b', 'rev-b'), node_key: 'node-a-rev-b' }], since: '', until: '', has_more: false, complete: true })
    wrapper = mount(NodeDetailView, { props: { scope: baseScope } })
    await flushPromises()
    await wrapper.get('select[aria-label="配置 revision"]').setValue('rev-b')
    await flushPromises()
    releaseOld({ attempts: [publicServiceAttempt('stale-service-rev-a')], since: '', until: '', has_more: false, complete: true })
    await flushPromises()

    expect(wrapper.text()).toContain('service-rev-b')
    expect(wrapper.text()).not.toContain('stale-service-rev-a')
    expect(bridgeMocks.fetchWorkbenchPublicServiceHistory).toHaveBeenLastCalledWith(expect.objectContaining({ config_revision_key: 'rev-b' }))
  })
})
