import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import WorkbenchDownloadPanel from '../WorkbenchDownloadPanel.vue'
import type { MonitorNodeOption, WorkbenchDownloadAttempt } from '../../../types'

const apiMocks = vi.hoisted(() => ({
  startWorkbenchDownloadTest: vi.fn(),
  fetchWorkbenchDownloadHistory: vi.fn(),
  fetchWorkbenchDownloadAttempt: vi.fn(),
  cancelWorkbenchDownloadTest: vi.fn(),
  retrySaveWorkbenchDownloadTest: vi.fn(),
  subscribeEvents: vi.fn(() => () => {}),
}))
vi.mock('../../../api/bridge', () => apiMocks)

const node: MonitorNodeOption = {
  profileId: 'profile-a', profileName: '订阅 A', nodeKey: 'node-a', nodeIdentityKey: 'identity-a',
  configRevisionKey: 'revision-a', displayName: '下载节点', type: 'http', countryCode: '', countryFlag: '',
}

function attempt(overrides: Partial<WorkbenchDownloadAttempt> = {}): WorkbenchDownloadAttempt {
  return {
    attempt_id: 'download-a', request_id: 'request-a', profile_id: 'profile-a', node_key: 'node-a',
    node_identity_key: 'identity-a', config_revision_key: 'revision-a', display_name: '下载节点', node_type: 'http',
    source: 'workbench_manual_download', requested_at: '2026-09-23T09:00:00Z', started_at: '2026-09-23T09:00:00Z',
    execution_state: 'running', persistence_state: 'not_started',
    rule: { rule_version: 1, target_url: 'https://speed.cloudflare.com/__down?bytes=1001', method: 'GET', maximum_bytes: 1000, maximum_duration_ns: 5_000_000_000, sample_every_bytes: 256 * 1024, sample_every_ns: 100_000_000 },
    ...overrides,
  }
}

let wrapper: VueWrapper | null = null
beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.fetchWorkbenchDownloadHistory.mockResolvedValue({ attempts: [], since: '', until: '', has_more: false, complete: true })
  apiMocks.startWorkbenchDownloadTest.mockResolvedValue(attempt())
  apiMocks.fetchWorkbenchDownloadAttempt.mockResolvedValue(attempt())
  apiMocks.cancelWorkbenchDownloadTest.mockResolvedValue(attempt({ execution_state: 'cancelling' }))
  apiMocks.retrySaveWorkbenchDownloadTest.mockResolvedValue(attempt({ execution_state: 'completed', persistence_state: 'saved' }))
})
afterEach(() => { wrapper?.unmount(); wrapper = null })

describe('WorkbenchDownloadPanel', () => {
  it('freezes the stable node selection and cancels the actual active attempt', async () => {
    wrapper = mount(WorkbenchDownloadPanel, { props: { node } })
    await flushPromises()
    await wrapper.get('button.download-primary').trigger('click')
    await flushPromises()

    expect(apiMocks.startWorkbenchDownloadTest).toHaveBeenCalledWith(expect.objectContaining({
      profile_id: 'profile-a', node_key: 'node-a', node_identity_key: 'identity-a', config_revision_key: 'revision-a',
      maximum_bytes: 20 * 1024 * 1024, timeout_seconds: 10,
    }))
    expect(wrapper.text()).toContain('下载中')
    await wrapper.get('button.download-button').trigger('click')
    await flushPromises()
    expect(apiMocks.cancelWorkbenchDownloadTest).toHaveBeenCalledWith('download-a', expect.objectContaining({
      profile_id: 'profile-a', node_identity_key: 'identity-a', config_revision_key: 'revision-a',
    }))
    expect(wrapper.text()).toContain('正在取消')
  })

  it('retries a staged save using the same attempt without downloading again', async () => {
    const result = { outcome: 'byte_limit', bytes_read: 1000, started_at: '2026-09-23T09:00:00Z', finished_at: '2026-09-23T09:00:01Z', duration_ns: 1_000_000_000, samples: [{ elapsed_ns: 1_000_000_000, interval_ns: 1_000_000_000, delta_bytes: 1000, cumulative_bytes: 1000, speed_mbps: 0.008 }] }
    apiMocks.startWorkbenchDownloadTest.mockResolvedValue(attempt({
      execution_state: 'completed', persistence_state: 'failed', persistence_error: '暂存结果可重试',
      result,
    }))
    apiMocks.retrySaveWorkbenchDownloadTest.mockResolvedValue(attempt({ execution_state: 'completed', persistence_state: 'saved', result }))
    wrapper = mount(WorkbenchDownloadPanel, { props: { node } })
    await flushPromises()
    await wrapper.get('button.download-primary').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('重试保存（不重新下载）')
    const retry = wrapper.findAll('button').find((button) => button.text().includes('重试保存'))
    expect(retry).toBeDefined()
    await retry!.trigger('click')
    await flushPromises()
    expect(apiMocks.retrySaveWorkbenchDownloadTest).toHaveBeenCalledWith('download-a', expect.objectContaining({ node_identity_key: 'identity-a' }))
    expect(apiMocks.startWorkbenchDownloadTest).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('已保存')
    expect(wrapper.text()).toContain('0.01 Mbps')
    await wrapper.findAll('button').find((button) => button.text().includes('查看节点详情'))!.trigger('click')
    expect(wrapper.emitted('open-node-detail')?.[0]?.[0]).toMatchObject({
      profileId: 'profile-a', nodeIdentityKey: 'identity-a', configRevisionKey: 'revision-a',
      origin: { kind: 'workbench_download_attempt', attemptId: 'download-a' },
    })
  })

  it('after remount selects a recovered staged result and retries its original save without measuring again', async () => {
    const result = { outcome: 'byte_limit', bytes_read: 1000, started_at: '2026-09-23T09:00:00Z', finished_at: '2026-09-23T09:00:01Z', duration_ns: 1_000_000_000, samples: [{ elapsed_ns: 1_000_000_000, interval_ns: 1_000_000_000, delta_bytes: 1000, cumulative_bytes: 1000 }] }
    const recovered = attempt({ execution_state: 'completed', persistence_state: 'failed', persistence_error: '启动后暂存结果可重试', result })
    apiMocks.fetchWorkbenchDownloadHistory.mockResolvedValue({ attempts: [recovered], since: '', until: '', has_more: false, complete: true })
    apiMocks.fetchWorkbenchDownloadAttempt.mockResolvedValue(recovered)
    apiMocks.retrySaveWorkbenchDownloadTest.mockResolvedValue(attempt({ execution_state: 'completed', persistence_state: 'saved', result }))

    wrapper = mount(WorkbenchDownloadPanel, { props: { node } })
    await flushPromises()
    wrapper.unmount()
    wrapper = null

    wrapper = mount(WorkbenchDownloadPanel, { props: { node } })
    await flushPromises()
    const savedAttempt = wrapper.findAll('button').find((button) => button.text().includes('选择') && button.text().includes('download-a'))
    expect(savedAttempt).toBeDefined()
    await savedAttempt!.trigger('click')
    await flushPromises()
    expect(apiMocks.fetchWorkbenchDownloadAttempt).toHaveBeenCalledWith('download-a', expect.objectContaining({
      profile_id: 'profile-a', node_key: 'node-a', node_identity_key: 'identity-a', config_revision_key: 'revision-a',
    }))
    const retry = wrapper.findAll('button').find((button) => button.text().includes('重试保存（不重新下载）'))
    expect(retry).toBeDefined()
    await retry!.trigger('click')
    await flushPromises()

    expect(apiMocks.retrySaveWorkbenchDownloadTest).toHaveBeenCalledWith('download-a', expect.objectContaining({
      profile_id: 'profile-a', node_identity_key: 'identity-a', config_revision_key: 'revision-a',
    }))
    expect(apiMocks.startWorkbenchDownloadTest).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('已保存')
  })

  it('ignores a late start response after the selected node changes', async () => {
    let resolveStart!: (value: WorkbenchDownloadAttempt) => void
    apiMocks.startWorkbenchDownloadTest.mockImplementationOnce(() => new Promise((resolve) => { resolveStart = resolve }))
    wrapper = mount(WorkbenchDownloadPanel, { props: { node } })
    await flushPromises()
    await wrapper.get('button.download-primary').trigger('click')
    const otherNode = { ...node, nodeKey: 'node-b', nodeIdentityKey: 'identity-b', configRevisionKey: 'revision-b', displayName: '另一节点' }
    await wrapper.setProps({ node: otherNode })
    await flushPromises()
    resolveStart(attempt())
    await flushPromises()

    expect(apiMocks.startWorkbenchDownloadTest).toHaveBeenCalledWith(expect.objectContaining({ node_key: 'node-a', node_identity_key: 'identity-a', config_revision_key: 'revision-a' }))
    expect(wrapper.text()).toContain('另一节点')
    expect(wrapper.text()).not.toContain('结果属于 下载节点')
  })
})
