import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

vi.mock('../../../api/bridge', () => ({ fetchSettings: vi.fn(), saveSettings: vi.fn() }))
vi.mock('../../../api/monitor', () => ({
  fetchMonitorStorageUsage: vi.fn(),
  previewMonitorRetention: vi.fn(),
  applyMonitorRetention: vi.fn(),
}))

import { fetchSettings, saveSettings } from '../../../api/bridge'
import { applyMonitorRetention, fetchMonitorStorageUsage, previewMonitorRetention } from '../../../api/monitor'
import MonitorRetentionPanel from '../MonitorRetentionPanel.vue'

const storage = { database_bytes: 1024, wal_bytes: 4096, shared_memory_bytes: 0, total_bytes: 5120, warning_bytes: 1024, hard_bytes: 4096, warning: true, protected: true }
const cutoff = '2026-06-01T00:00:00Z'
let wrapper: VueWrapper | null = null

beforeEach(() => {
  vi.mocked(fetchSettings).mockResolvedValue({ preferred_browser: 'default', monitor_retention_policy: 'keep_all', monitor_retention_custom_days: 0 })
  vi.mocked(saveSettings).mockResolvedValue(undefined)
  vi.mocked(fetchMonitorStorageUsage).mockResolvedValue(storage)
  vi.mocked(previewMonitorRetention).mockResolvedValue({ policy: '90d', cutoff, samples_to_delete: 7, runs_to_delete: 2, storage })
  vi.mocked(applyMonitorRetention).mockResolvedValue({ policy: '90d', cutoff, samples_deleted: 7, runs_deleted: 2, duration_ms: 1, partial: false })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('MonitorRetentionPanel', () => {
  it('keeps keep_all until explicit preference, preview and separate deletion confirmation', async () => {
    wrapper = mount(MonitorRetentionPanel)
    await flushPromises()
    expect(wrapper.text()).toContain('当前保留偏好：keep_all')
    expect(wrapper.text()).toContain('WAL')
    expect(wrapper.text()).toContain('容量保护已生效')
    expect(wrapper.findAll('button').find(b => b.text().includes('预览'))!.attributes('disabled')).toBeDefined()
    expect(applyMonitorRetention).not.toHaveBeenCalled()

    await wrapper.find('select').setValue('90d')
    await wrapper.findAll('button').find(b => b.text().includes('保存偏好'))!.trigger('click')
    await flushPromises()
    expect(saveSettings).toHaveBeenCalledWith(expect.objectContaining({ monitor_retention_policy: '90d' }))
    expect(applyMonitorRetention).not.toHaveBeenCalled()

    await wrapper.findAll('button').find(b => b.text().includes('预览'))!.trigger('click')
    await flushPromises()
    expect(previewMonitorRetention).toHaveBeenCalledWith({ policy: '90d' })
    expect(wrapper.text()).toContain('Monitor 样本 7 条')
    expect(applyMonitorRetention).not.toHaveBeenCalled()
    const execute = wrapper.findAll('button').find(b => b.text().includes('确认删除'))!
    expect(execute.attributes('disabled')).toBeDefined()
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await execute.trigger('click')
    await flushPromises()
    expect(applyMonitorRetention).toHaveBeenCalledWith({ policy: '90d', cutoff_time: cutoff })
    expect(wrapper.text()).toContain('已删除 7 条')
  })

  it('reports partial deletion as partial, never success', async () => {
    vi.mocked(fetchSettings).mockResolvedValue({ preferred_browser: 'default', monitor_retention_policy: '90d', monitor_retention_custom_days: 0 })
    vi.mocked(applyMonitorRetention).mockRejectedValue(new Error('部分删除：已删除 4 条样本；执行中断'))
    wrapper = mount(MonitorRetentionPanel)
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text().includes('预览'))!.trigger('click')
    await flushPromises()
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.findAll('button').find(b => b.text().includes('确认删除'))!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('部分删除')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  })
})
