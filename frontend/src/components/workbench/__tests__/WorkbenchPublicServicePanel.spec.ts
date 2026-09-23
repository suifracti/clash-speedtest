import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import WorkbenchPublicServicePanel from '../WorkbenchPublicServicePanel.vue'
import type { MonitorNodeOption, WorkbenchPublicServiceAttempt, WorkbenchPublicServiceRule } from '../../../types'

const apiMocks = vi.hoisted(() => ({
  listWorkbenchPublicServiceCatalog: vi.fn(),
  startWorkbenchPublicServiceTest: vi.fn(),
  fetchWorkbenchPublicServiceAttempt: vi.fn(),
  cancelWorkbenchPublicServiceTest: vi.fn(),
  retrySaveWorkbenchPublicServiceTest: vi.fn(),
}))

vi.mock('../../../api/bridge', () => apiMocks)

const rule: WorkbenchPublicServiceRule = {
  service_id: 'cloudflare_204', name: 'Cloudflare 204 连通性', rule_version: 1,
  target_url: 'https://cp.cloudflare.com/generate_204', method: 'GET',
  success_criterion: 'HTTP status exactly 204', redirect_policy: 'do_not_follow',
  timeout_seconds: 10, maximum_body_bytes: 65536,
}

const node: MonitorNodeOption = {
  profileId: 'profile-a', profileName: '订阅 A', nodeKey: 'node-a', nodeIdentityKey: 'identity-a',
  configRevisionKey: 'revision-a', displayName: '同名节点', type: 'http', countryCode: '', countryFlag: '',
}

function attempt(overrides: Partial<WorkbenchPublicServiceAttempt> = {}): WorkbenchPublicServiceAttempt {
  return {
    attempt_id: 'attempt-a', request_id: 'request-a', profile_id: 'profile-a', node_key: 'node-a',
    node_identity_key: 'identity-a', config_revision_key: 'revision-a', display_name: '同名节点', node_type: 'http',
    source: 'workbench_public_service', service_id: rule.service_id, rule, requested_at: '2026-09-23T09:00:00Z',
    started_at: '2026-09-23T09:00:00Z', execution_state: 'running', persistence_state: 'not_started',
    ...overrides,
  }
}

let wrapper: VueWrapper | null = null

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listWorkbenchPublicServiceCatalog.mockResolvedValue([rule])
  apiMocks.startWorkbenchPublicServiceTest.mockResolvedValue(attempt())
  apiMocks.fetchWorkbenchPublicServiceAttempt.mockResolvedValue(attempt())
  apiMocks.cancelWorkbenchPublicServiceTest.mockResolvedValue(attempt({ execution_state: 'cancelling' }))
  apiMocks.retrySaveWorkbenchPublicServiceTest.mockResolvedValue(attempt({ execution_state: 'completed', persistence_state: 'saved', result: { outcome: 'matched', http_status: 204, bytes_read: 0, started_at: '2026-09-23T09:00:00Z', finished_at: '2026-09-23T09:00:01Z', duration_ms: 1000 } }))
})

afterEach(() => { wrapper?.unmount(); wrapper = null })

describe('WorkbenchPublicServicePanel', () => {
  it('freezes the selected node identity and cancels the actual active attempt', async () => {
    wrapper = mount(WorkbenchPublicServicePanel, { props: { node } })
    await flushPromises()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(apiMocks.startWorkbenchPublicServiceTest).toHaveBeenCalledWith(expect.objectContaining({
      profile_id: 'profile-a', node_key: 'node-a', node_identity_key: 'identity-a',
      config_revision_key: 'revision-a', service_id: 'cloudflare_204', timeout_seconds: 10,
    }))
    expect(wrapper.text()).toContain('检测中')
    await wrapper.get('button.public-service-button').trigger('click')
    await flushPromises()
    expect(apiMocks.cancelWorkbenchPublicServiceTest).toHaveBeenCalledWith('attempt-a', expect.objectContaining({
      profile_id: 'profile-a', node_identity_key: 'identity-a', config_revision_key: 'revision-a', service_id: 'cloudflare_204',
    }))
    expect(wrapper.text()).toContain('正在取消')
  })

  it('offers a same-attempt save retry without re-running the request', async () => {
    apiMocks.startWorkbenchPublicServiceTest.mockResolvedValue(attempt({
      execution_state: 'failed', persistence_state: 'failed', persistence_error: '保存失败',
      result: { outcome: 'http_rejected', http_status: 403, bytes_read: 0, started_at: '2026-09-23T09:00:00Z', finished_at: '2026-09-23T09:00:01Z', duration_ms: 1000, failure_phase: 'http_status', error_message: '服务返回 HTTP 403' },
    }))
    wrapper = mount(WorkbenchPublicServicePanel, { props: { node } })
    await flushPromises()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('重试保存（不重新检测）')
    const retry = wrapper.findAll('button').find((button) => button.text().includes('重试保存'))
    expect(retry).toBeDefined()
    await retry!.trigger('click')
    await flushPromises()
    expect(apiMocks.retrySaveWorkbenchPublicServiceTest).toHaveBeenCalledWith('attempt-a', expect.objectContaining({ service_id: 'cloudflare_204' }))
    expect(apiMocks.startWorkbenchPublicServiceTest).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('已保存')
  })

  it('shows an unstaged-result failure as terminal and does not offer a save retry without a result', async () => {
    apiMocks.startWorkbenchPublicServiceTest.mockResolvedValue(attempt({
      execution_state: 'completed', persistence_state: 'failed', persistence_error: '测量已结束，但结果未能暂存；该结果无法重试保存，请重新执行检测',
    }))
    wrapper = mount(WorkbenchPublicServicePanel, { props: { node } })
    await flushPromises()
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('结果未能暂存；需重新检测')
    expect(wrapper.text()).toContain('测量结果未能暂存')
    expect(wrapper.text()).not.toContain('重试保存（不重新检测）')
  })
})
