import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

vi.mock('../../../api/bridge', () => ({ fetchSettings: vi.fn(), saveSettings: vi.fn() }))
vi.mock('../../../api/monitor', () => ({ fetchMonitorBudgetStatus: vi.fn() }))

import { fetchSettings, saveSettings } from '../../../api/bridge'
import { fetchMonitorBudgetStatus } from '../../../api/monitor'
import MonitorBudgetPanel from '../MonitorBudgetPanel.vue'

let wrapper: VueWrapper | null = null

beforeEach(() => {
  vi.mocked(fetchSettings).mockResolvedValue({ preferred_browser: 'edge', monitor_retention_policy: 'keep_all', monitor_retention_custom_days: 0 })
  vi.mocked(saveSettings).mockResolvedValue(undefined)
  vi.mocked(fetchMonitorBudgetStatus).mockResolvedValue({
    limits: { max_concurrent: 4, daily_requests: 20000, daily_bytes: 32 * 1024 * 1024, response_bytes: 256 * 1024 },
    usage: { utc_day: '2026-09-23', requests_used: 7, bytes_used: 512 },
    reset_at: '2026-09-24T00:00:00Z', active_requests: 0, blocked_code: 'requests_exhausted', blocked_reason: '今日额度已用尽',
  })
})

afterEach(() => { wrapper?.unmount(); wrapper = null; vi.clearAllMocks() })

describe('MonitorBudgetPanel', () => {
  it('shows durable usage and only saves an explicit valid change without starting a job', async () => {
    wrapper = mount(MonitorBudgetPanel)
    await flushPromises()
    expect(wrapper.text()).toContain('7 / 20000')
    expect(wrapper.text()).toContain('2026-09-24T00:00:00Z')
    expect(wrapper.text()).toContain('未采集不是节点失败')
    expect(saveSettings).not.toHaveBeenCalled()
    const inputs = wrapper.findAll('input[type="number"]')
    await inputs[1]!.setValue('0')
    expect(wrapper.find('button').attributes('disabled')).toBeDefined()
    await inputs[1]!.setValue('30000')
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(saveSettings).toHaveBeenCalledWith(expect.objectContaining({
      preferred_browser: 'edge', monitor_retention_policy: 'keep_all', monitor_budget_daily_requests: 30000,
    }))
    expect(wrapper.find('[role="status"]').text()).toContain('不会立即启动或触发探测')
  })
})
