import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import * as api from '../../../api/bridge'
import * as monitorApi from '../../../api/monitor'
import LatencyWorkbench from '../LatencyWorkbench.vue'

const nodes = [
  { profileId: 'profile-a', profileName: '订阅 A', nodeKey: 'node-a', nodeIdentityKey: 'identity-a', configRevisionKey: 'rev-a', displayName: '香港极速 01', type: 'hysteria2', countryCode: 'HK', countryFlag: '🇭🇰' },
]

vi.mock('../../../api/monitor', () => ({
  fetchMonitorNodeOptions: vi.fn(),
  fetchMonitorJobs: vi.fn().mockResolvedValue([]),
}))

vi.mock('../../../api/bridge', () => ({
  fetchWorkbenchLatencyHistory: vi.fn(),
  fetchWorkbenchLatencyHistories: vi.fn().mockResolvedValue({}),
  fetchWorkbenchLatencyBatches: vi.fn().mockResolvedValue([]),
  fetchWorkbenchLatencyBatch: vi.fn(),
  startWorkbenchLatencyBatch: vi.fn(),
  cancelWorkbenchLatencyBatch: vi.fn(),
  retryWorkbenchLatencyBatchItem: vi.fn(),
  fetchWorkbenchDownloadHistory: vi.fn().mockResolvedValue([]),
  fetchWorkbenchPublicServiceHistory: vi.fn().mockResolvedValue([]),
  startWorkbenchPublicServiceTest: vi.fn(),
  fetchWorkbenchPublicServiceAttempt: vi.fn(),
  fetchActiveMonitorJobs: vi.fn().mockResolvedValue([]),
  fetchPublicServiceCatalog: vi.fn().mockResolvedValue([]),
  listWorkbenchPublicServiceCatalog: vi.fn().mockResolvedValue([
    { service_id: 'antigravity', name: 'Google Antigravity', category: 'AI' },
    { service_id: 'chatgpt_web', name: 'OpenAI ChatGPT', category: 'AI' },
    { service_id: 'youtube_premium', name: 'YouTube Premium', category: '流媒体' },
    { service_id: 'netflix_unlock', name: 'Netflix Unlock', category: '流媒体' },
  ]),
  fetchNoticeOverrides: vi.fn().mockResolvedValue({}),
  saveNoticeOverride: vi.fn(),
  saveProfileEntrypoints: vi.fn(),
  checkAuthStatus: vi.fn().mockResolvedValue({ authenticated: true, required: false }),
  exportNodesClashConfig: vi.fn(),
  subscribeEvents: vi.fn(() => () => {}),
}))

describe('LatencyWorkbench Service Synchronization & Pill Inspection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(monitorApi.fetchMonitorNodeOptions).mockResolvedValue(nodes)
    vi.mocked(api.fetchWorkbenchLatencyHistory).mockResolvedValue({
      tests: [],
      since: '2026-09-23T00:00:00Z',
      until: '2026-09-24T00:00:00Z',
      as_of: '2026-09-23T12:00:00Z',
      has_more: false,
      complete: true,
    })
  })

  it('renders synchronized service pills and handles test trigger', async () => {
    vi.mocked(api.startWorkbenchPublicServiceTest).mockResolvedValue({
      attempt_id: 'att-123',
      request_id: 'req-123',
      profile_id: 'profile-a',
      node_key: 'node-a',
      node_identity_key: 'identity-a',
      config_revision_key: 'rev-a',
      service_id: 'antigravity',
      execution_state: 'completed',
      persistence_state: 'saved',
      requested_at: '2026-09-24T00:00:00Z',
      result: {
        attempt_id: 'att-123',
        service_id: 'antigravity',
        outcome: 'matched',
        duration_ms: 128,
        http_status: 200,
        finished_at: '2026-09-24T00:00:01Z',
      },
    } as any)

    vi.mocked(api.fetchWorkbenchPublicServiceAttempt).mockResolvedValue({
      attempt_id: 'att-123',
      request_id: 'req-123',
      profile_id: 'profile-a',
      node_key: 'node-a',
      node_identity_key: 'identity-a',
      config_revision_key: 'rev-a',
      service_id: 'antigravity',
      execution_state: 'completed',
      persistence_state: 'saved',
      requested_at: '2026-09-24T00:00:00Z',
      result: {
        attempt_id: 'att-123',
        service_id: 'antigravity',
        outcome: 'matched',
        duration_ms: 128,
        http_status: 200,
        finished_at: '2026-09-24T00:00:01Z',
      },
    } as any)

    const wrapper = mount(LatencyWorkbench, {
      global: {
        stubs: {
          UiSelect: true,
          LatencySamplePlot: true,
          MultiSiteLatencyTrend: true,
          ExportClashModal: true,
          TestPlanDialog: true,
          ServiceComparison: true,
          DownloadComparison: true,
        },
      },
    })
    await flushPromises()

    // Service pills should be rendered on the node row
    const pills = wrapper.findAll('.compact-unlock-pill')
    expect(pills.length).toBeGreaterThanOrEqual(4)

    // First pill is 反重力
    expect(pills[0].text()).toContain('反重力')

    // Clicking an untested pill triggers quick service test
    await pills[0].trigger('click')
    await flushPromises()

    expect(api.startWorkbenchPublicServiceTest).toHaveBeenCalledWith(expect.objectContaining({
      node_key: 'node-a',
      service_id: 'antigravity',
    }))
  })

  it('groups pills and provides +N 项 dropdown when more than 4 services are selected', async () => {
    const wrapper = mount(LatencyWorkbench, {
      global: {
        stubs: {
          UiSelect: true,
          LatencySamplePlot: true,
          MultiSiteLatencyTrend: true,
          ExportClashModal: true,
          TestPlanDialog: true,
          ServiceComparison: true,
          DownloadComparison: true,
        },
      },
    })
    await flushPromises()

    // Switch to compact mode and set 5 services
    ;(wrapper.vm as any).allNodesExpanded = false
    ;(wrapper.vm as any).selectedServiceIds = ['antigravity', 'chatgpt_web', 'youtube_premium', 'netflix_unlock', 'disney_plus']
    await flushPromises()

    // Should find the "+2 项 ▾" button (5 total - 3 visible = 2 remaining)
    const moreBtn = wrapper.find('.more-pills-btn')
    expect(moreBtn.exists()).toBe(true)
    expect(moreBtn.text()).toContain('+2 项')

    // Click "+2 项 ▾" to open popover
    expect(wrapper.find('.more-pills-popover').exists()).toBe(false)
    await moreBtn.trigger('click')
    expect(wrapper.find('.more-pills-popover').exists()).toBe(true)

    // Popover shows all 5 items
    const popoverPills = wrapper.findAll('.more-pills-popover .popover-pill')
    expect(popoverPills).toHaveLength(5)
  })
})
