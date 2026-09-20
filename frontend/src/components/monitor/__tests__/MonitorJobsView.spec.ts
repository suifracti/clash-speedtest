import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

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
import type { MonitorJob, MonitorNodeOption } from '../../../types'
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

function job(state: MonitorJob['state'] = 'stopped'): MonitorJob {
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
})
