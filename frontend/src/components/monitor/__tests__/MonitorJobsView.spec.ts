import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

vi.mock('../MonitorRetentionPanel.vue', () => ({ default: { template: '<div />' } }))
vi.mock('../MonitorBudgetPanel.vue', () => ({ default: { template: '<div />' } }))

vi.mock('../../../api/monitor', () => ({
  fetchMonitorNodeOptions: vi.fn(),
  fetchMonitorJobs: vi.fn(),
  fetchMonitorRuns: vi.fn(),
  createMonitorJob: vi.fn(),
  controlMonitorJob: vi.fn(),
}))

import {
  controlMonitorJob,
  createMonitorJob,
  fetchMonitorJobs,
  fetchMonitorNodeOptions,
  fetchMonitorRuns,
} from '../../../api/monitor'
import type { MonitorJob, MonitorJobPrefill, MonitorNodeOption } from '../../../types'
import MonitorJobsView from '../MonitorJobsView.vue'

const mockedOptions = vi.mocked(fetchMonitorNodeOptions)
const mockedJobs = vi.mocked(fetchMonitorJobs)
const mockedRuns = vi.mocked(fetchMonitorRuns)
const mockedCreate = vi.mocked(createMonitorJob)
const mockedControl = vi.mocked(controlMonitorJob)

const option: MonitorNodeOption = {
  profileId: 'profile-1',
  profileName: 'Test subscription',
  nodeKey: 'nk-real',
  nodeIdentityKey: 'nid-real',
  configRevisionKey: 'rev-real',
  displayName: 'Real node',
  type: 'ss',
  countryCode: 'JP',
  countryFlag: '🇯🇵',
}

const prefill: MonitorJobPrefill = {
  profileId: option.profileId,
  nodeKeys: [option.nodeKey],
  nodeContexts: [{
    node_key: option.nodeKey,
    node_identity_key: option.nodeIdentityKey,
    config_revision_key: option.configRevisionKey,
  }],
}

function job(state: MonitorJob['state'] = 'stopped', blockedReason = ''): MonitorJob {
  return {
    id: 'job-1',
    name: 'UI monitor',
    profileId: 'profile-1',
    profileName: 'Test subscription',
    nodeKeys: [option.nodeKey],
    nodes: [{
      nodeKey: option.nodeKey,
      nodeIdentityKey: option.nodeIdentityKey,
      configRevisionKey: option.configRevisionKey,
      displayName: option.displayName,
      type: option.type,
    }],
    probeSet: 'light',
    intervalSeconds: 30,
    timeoutSeconds: 5,
    state,
    blockedReason,
    createdAt: '2026-09-19T00:00:00Z',
    updatedAt: '2026-09-19T00:00:00Z',
  }
}

let wrapper: VueWrapper | null = null

beforeEach(() => {
  mockedOptions.mockResolvedValue([option])
  mockedJobs.mockResolvedValue([job()])
  mockedRuns.mockResolvedValue([])
  mockedCreate.mockResolvedValue(job())
  mockedControl.mockResolvedValue()
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('MonitorJobsView', () => {
  it('creates from a stable node key and never starts as a side effect', async () => {
    wrapper = mount(MonitorJobsView)
    await flushPromises()

    expect(wrapper.text()).toContain('创建（不会自动启动）')
    await wrapper.find('input[type="checkbox"]').setValue(true)
    const createButton = wrapper.findAll('button').find((button) => button.text().includes('创建（不会自动启动）'))
    expect(createButton).toBeDefined()
    await createButton!.trigger('click')
    await flushPromises()

    expect(mockedCreate).toHaveBeenCalledWith(expect.objectContaining({
      profile_id: 'profile-1',
      node_keys: ['nk-real'],
      interval_seconds: 60,
      timeout_seconds: 10,
    }))
    expect(mockedControl).not.toHaveBeenCalled()
  })

  it('uses the explicit start action and re-reads the backend job state', async () => {
    let current = job('stopped')
    mockedJobs.mockImplementation(async () => [current])
    mockedControl.mockImplementation(async () => {
      current = job('running')
    })

    wrapper = mount(MonitorJobsView)
    await flushPromises()
    const startButton = wrapper.findAll('button').find((button) => button.text().trim() === '启动')
    expect(startButton).toBeDefined()
    await startButton!.trigger('click')
    await flushPromises()

    expect(mockedControl).toHaveBeenCalledWith('job-1', 'start')
    expect(wrapper.text()).toContain('运行中')
  })

  it('shows an explicit blocked reason without offering an automatic recovery action', async () => {
    mockedJobs.mockResolvedValue([job('blocked', '节点配置 revision 已变化，需要重新确认')])

    wrapper = mount(MonitorJobsView)
    await flushPromises()

    expect(wrapper.text()).toContain('已阻塞')
    expect(wrapper.text()).toContain('节点配置 revision 已变化，需要重新确认')
    expect(wrapper.findAll('button').some((button) => ['启动', '暂停', '恢复'].includes(button.text().trim()))).toBe(false)
  })

  it('applies Workbench prefill, supports cancel without creating, and sends the stable context on confirm', async () => {
    mockedJobs.mockResolvedValue([])
    wrapper = mount(MonitorJobsView, { props: { prefill } })
    await flushPromises()

    expect(wrapper.text()).toContain('工作台已选择')
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(true)

    const cancel = wrapper.findAll('button').find((button) => button.text().includes('取消本次预填'))
    expect(cancel).toBeDefined()
    await cancel!.trigger('click')
    expect(mockedCreate).not.toHaveBeenCalled()
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)

    wrapper.unmount()
    wrapper = mount(MonitorJobsView, { props: { prefill } })
    await flushPromises()
    const confirm = wrapper.findAll('button').find((button) => button.text().includes('确认创建（不会自动启动）'))
    expect(confirm).toBeDefined()
    await confirm!.trigger('click')
    await flushPromises()
    expect(mockedCreate).toHaveBeenCalledWith(expect.objectContaining({
      node_keys: ['nk-real'],
      node_contexts: [{
        node_key: 'nk-real',
        node_identity_key: 'nid-real',
        config_revision_key: 'rev-real',
      }],
    }))
    expect(mockedControl).not.toHaveBeenCalled()
  })
})
