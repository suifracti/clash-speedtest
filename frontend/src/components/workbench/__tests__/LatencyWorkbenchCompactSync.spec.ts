import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import * as api from '../../../api/bridge'
import * as monitorApi from '../../../api/monitor'
import LatencyWorkbench from '../LatencyWorkbench.vue'

const nodes = [
  { profileId: 'profile-a', profileName: '订阅 A', nodeKey: 'node-a', nodeIdentityKey: 'identity-a', configRevisionKey: 'rev-a', displayName: '节点 A', type: 'hysteria2', countryCode: 'US', countryFlag: '🇺🇸' },
  { profileId: 'profile-b', profileName: '订阅 B', nodeKey: 'node-b', nodeIdentityKey: 'identity-b', configRevisionKey: 'rev-b', displayName: '节点 B', type: 'vmess', countryCode: 'JP', countryFlag: '🇯🇵' },
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
  fetchActiveMonitorJobs: vi.fn().mockResolvedValue([]),
  fetchPublicServiceCatalog: vi.fn().mockResolvedValue([]),
  fetchNoticeOverrides: vi.fn().mockResolvedValue({}),
  saveNoticeOverride: vi.fn(),
  saveProfileEntrypoints: vi.fn(),
  checkAuthStatus: vi.fn().mockResolvedValue({ authenticated: true, required: false }),
  exportNodesClashConfig: vi.fn(),
  subscribeEvents: vi.fn(() => () => {}),
}))

describe('LatencyWorkbench Compact Sync & Expand/Collapse', () => {
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

  it('renders global toggle button and allows synchronous expanding and collapsing of all nodes', async () => {
    const wrapper = mount(LatencyWorkbench, {
      global: {
        stubs: {
          UiSelect: true,
          LatencySamplePlot: true,
          MultiSiteLatencyTrend: true,
          ExportClashModal: true,
          TestPlanDialog: true,
        },
      },
    })
    await flushPromises()

    const rows = wrapper.findAll('.node-row')
    expect(rows).toHaveLength(2)

    // Find the global expand toggle button in selection-actions
    const globalToggle = wrapper.find('.global-expand-toggle-btn')
    expect(globalToggle.exists()).toBe(true)

    // Find the per-row toggle buttons
    const rowToggles = wrapper.findAll('.toggle-expand-btn')
    expect(rowToggles.length).toBeGreaterThanOrEqual(2)

    // Initially in test env it starts expanded; clicking row toggle on Node 1 collapses all nodes
    await rowToggles[0].trigger('click')
    await flushPromises()

    // Now all rows should be in compact mode!
    const compactRows = wrapper.findAll('.node-row.compact-mode-row')
    expect(compactRows).toHaveLength(2)
    expect(globalToggle.text()).toContain('全部展开走势')

    // Every node row's button now shows "展开走势 ▼"
    const togglesAfterCollapse = wrapper.findAll('.toggle-expand-btn')
    togglesAfterCollapse.forEach((btn) => {
      expect(btn.text()).toContain('展开走势 ▼')
    })

    // Click "展开走势 ▼" on Node 2: all nodes must expand synchronously!
    await togglesAfterCollapse[1].trigger('click')
    await flushPromises()

    // No rows are compact anymore, all are expanded!
    expect(wrapper.findAll('.node-row.compact-mode-row')).toHaveLength(0)
    expect(globalToggle.text()).toContain('全部收起走势')

    const togglesAfterExpand = wrapper.findAll('.toggle-expand-btn')
    togglesAfterExpand.forEach((btn) => {
      expect(btn.text()).toContain('收起走势 ▲')
    })

    // Click global toggle to collapse again
    await globalToggle.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('.node-row.compact-mode-row')).toHaveLength(2)
    expect(globalToggle.text()).toContain('全部展开走势')
  })
})
