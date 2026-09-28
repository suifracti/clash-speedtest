import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi } from 'vitest'
import MonitorTransfer from '../MonitorTransfer.vue'
import { createMonitorJob, fetchMonitorNodeOptions } from '../../../api/monitor'
vi.mock('../../../api/monitor', () => ({ createMonitorJob: vi.fn(), fetchMonitorJobs: vi.fn(), fetchMonitorNodeOptions: vi.fn() }))
vi.mock('../../../api/bridge', () => ({ openDataFolder: vi.fn() }))

describe('Monitor config import', () => {
  it('previews matching identities without creating; confirms stopped-only requests and rejects stale configuration', async () => {
    vi.mocked(fetchMonitorNodeOptions).mockResolvedValue([{ profileId: 'p', profileName: 'P', nodeKey: 'n', nodeIdentityKey: 'identity', configRevisionKey: 'rev', displayName: 'Node', type: 'ss', countryCode: 'JP', countryFlag: '' }])
    vi.mocked(createMonitorJob).mockResolvedValue({} as never)
    const wrapper = mount(MonitorTransfer)
    const job = { name: 'Imported', profile_id: 'p', node_keys: ['n'], node_contexts: [{ node_key: 'n', node_identity_key: 'identity', config_revision_key: 'rev' }], probe_set: 'light', sampling_tier: 'regular', interval_seconds: 60, timeout_seconds: 10, state: 'running', resume_on_launch: true }
    const read = async () => {
      Object.defineProperty(wrapper.get('input[type=file]').element, 'files', { configurable: true, value: [{ size: 800, text: async () => JSON.stringify({ format: 'speedtest-monitor', version: 1, jobs: [job] }) }] })
      await wrapper.get('input[type=file]').trigger('change'); await flushPromises()
    }
    await read()
    expect(wrapper.text()).toContain('待导入 1 项')
    expect(createMonitorJob).not.toHaveBeenCalled()
    await wrapper.findAll('button').find(button => button.text().includes('确认新增任务'))!.trigger('click'); await flushPromises()
    expect(createMonitorJob).toHaveBeenCalledWith({ name: 'Imported', profile_id: 'p', node_keys: ['n'], node_contexts: [{ node_key: 'n', node_identity_key: 'identity', config_revision_key: 'rev' }], probe_set: 'light', sampling_tier: 'regular', interval_seconds: 60, timeout_seconds: 10 })
    job.node_contexts[0].config_revision_key = 'old'
    await read()
    expect(wrapper.text()).toContain('订阅或节点配置不匹配')
    expect(wrapper.text()).not.toContain('确认新增任务')
    wrapper.unmount()
  })
})
