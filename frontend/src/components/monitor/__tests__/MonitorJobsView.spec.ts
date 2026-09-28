import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

vi.mock('../MonitorRetentionPanel.vue', () => ({ default: { template: '<div />' } }))
vi.mock('../MonitorBudgetPanel.vue', () => ({ default: { template: '<div />' } }))

vi.mock('../../../api/monitor', () => ({
  fetchMonitorNodeOptions: vi.fn(),
  fetchMonitorJobs: vi.fn(),
  fetchMonitorRuns: vi.fn(),
  createMonitorJob: vi.fn(),
  updateMonitorJobSamplingTier: vi.fn(),
  updateMonitorJobResumeOnLaunch: vi.fn(),
  deleteMonitorJob: vi.fn(),
  controlMonitorJob: vi.fn(),
  triggerMonitorJob: vi.fn(),
}))

import {
  controlMonitorJob,
  createMonitorJob,
  updateMonitorJobSamplingTier,
  updateMonitorJobResumeOnLaunch,
  fetchMonitorJobs,
  fetchMonitorNodeOptions,
  fetchMonitorRuns,
  triggerMonitorJob,
} from '../../../api/monitor'
import type { MonitorJob, MonitorJobPrefill, MonitorNodeOption, MonitorRun } from '../../../types'
import MonitorJobsView from '../MonitorJobsView.vue'
import UiSelect from '../../common/UiSelect.vue'

const mockedOptions = vi.mocked(fetchMonitorNodeOptions)
const mockedJobs = vi.mocked(fetchMonitorJobs)
const mockedRuns = vi.mocked(fetchMonitorRuns)
const mockedCreate = vi.mocked(createMonitorJob)
const mockedUpdateTier = vi.mocked(updateMonitorJobSamplingTier)
const mockedUpdateRecovery = vi.mocked(updateMonitorJobResumeOnLaunch)
const mockedControl = vi.mocked(controlMonitorJob)
const mockedTrigger = vi.mocked(triggerMonitorJob)

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
    samplingTier: 'regular',
    intervalSeconds: 30,
    timeoutSeconds: 5,
    state,
    runtimeState: state,
    resumeOnLaunch: state === 'blocked',
    desiredState: state === 'blocked' ? 'running' : state,
    recoveryState: state === 'blocked' ? 'blocked' : state === 'running' ? 'active' : state === 'paused' ? 'paused' : 'stopped',
    recoveryReason: blockedReason,
    intentPersistenceError: '',
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
  mockedUpdateTier.mockResolvedValue()
  mockedUpdateRecovery.mockResolvedValue()
  mockedControl.mockResolvedValue()
  mockedTrigger.mockResolvedValue({
    runId: 'diagnostic-1', jobId: 'job-1', samplingTier: 'diagnostic', triggerType: 'manual',
    samplingStrategyVersion: 1, scheduledAt: '2026-09-23T00:00:00Z', startedAt: '2026-09-23T00:00:00Z',
    status: 'completed', totalNodes: 1, successNodes: 1, failedNodes: 0,
  } satisfies MonitorRun)
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('MonitorJobsView', () => {
  it('keeps both configurations when one airport source groups subscriptions with the same endpoint', async () => {
    mockedOptions.mockResolvedValue([
      { ...option, profileId: 'first', profileName: '飞鸟云 · 默认订阅', nodeKey: 'first-node', nodeIdentityKey: 'shared-endpoint', configRevisionKey: 'credential-a' },
      { ...option, profileId: 'second', profileName: '飞鸟云 · 2', nodeKey: 'second-node', nodeIdentityKey: 'shared-endpoint', configRevisionKey: 'credential-b' },
    ])
    wrapper = mount(MonitorJobsView)
    await flushPromises()
    expect(wrapper.findAll('.node-picker input[type="checkbox"]')).toHaveLength(2)
    for (const input of wrapper.findAll('.node-picker input[type="checkbox"]')) await input.setValue(true)
    await wrapper.findAll('button').find(button => button.text() === '创建（不会自动启动）')!.trigger('click')
    await flushPromises()
    expect(mockedCreate.mock.calls.map(([request]) => request.profile_id).sort()).toEqual(['first', 'second'])
    expect(mockedControl).not.toHaveBeenCalled()
  })

  it('filters notices and regions, then creates separate stopped jobs across subscriptions and probes', async () => {
    mockedOptions.mockResolvedValue([option, { ...option, profileId: 'profile-2', profileName: 'Second airport', countryCode: 'US', displayName: 'US node' }, { ...option, nodeKey: 'notice', displayName: '官网 https://example.com' }])
    wrapper = mount(MonitorJobsView)
    await flushPromises()
    expect(wrapper.findAll('.node-picker input')).toHaveLength(1)
    const second = wrapper.findAll('.profile-multiselect input').find(input => (input.element as HTMLInputElement).value === 'profile-1')!
    await second.setValue(true)
    const regionSelect = wrapper.findAllComponents(UiSelect).find(component => component.props('ariaLabel') === '筛选地区')!
    regionSelect.vm.$emit('update:modelValue', 'US')
    await flushPromises()
    expect(wrapper.findAll('.node-picker input')).toHaveLength(1)
    await wrapper.find('.node-picker input').setValue(true)
    regionSelect.vm.$emit('update:modelValue', 'JP')
    await flushPromises()
    await wrapper.find('.node-picker input').setValue(true)
    await wrapper.find('.extra-probes input[value="service"]').setValue(true)
    await wrapper.findAll('button').find(button => button.text() === '创建（不会自动启动）')!.trigger('click')
    await flushPromises()
    const requests = mockedCreate.mock.calls.map(([request]) => [request.profile_id, request.probe_set, request.node_keys]).sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)))
    expect(requests).toEqual([
      ['profile-1', 'light', ['nk-real']], ['profile-1', 'service', ['nk-real']],
      ['profile-2', 'light', ['nk-real']], ['profile-2', 'service', ['nk-real']],
    ])
    expect(mockedControl).not.toHaveBeenCalled()
  })
  it('saves startup recovery permission without starting or stopping the current task', async () => {
    wrapper = mount(MonitorJobsView)
    await flushPromises()

    const recoveryLabel = wrapper.findAll('label').find((label) => label.text().includes('应用启动时恢复'))
    expect(recoveryLabel).toBeDefined()
    await recoveryLabel!.find('input[type="checkbox"]').setValue(true)
    await flushPromises()

    expect(mockedUpdateRecovery).toHaveBeenCalledWith('job-1', true)
    expect(mockedControl).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('此设置不会启动或停止当前任务')
  })

  it('creates from a stable node key and never starts as a side effect', async () => {
    wrapper = mount(MonitorJobsView)
    await flushPromises()

    expect(wrapper.text()).toContain('创建（不会自动启动）')
    await wrapper.find('.node-picker input[type="checkbox"]').setValue(true)
    const createButton = wrapper.findAll('button').find((button) => button.text().includes('创建（不会自动启动）'))
    expect(createButton).toBeDefined()
    await createButton!.trigger('click')
    await flushPromises()

    expect(mockedCreate).toHaveBeenCalledWith(expect.objectContaining({
      profile_id: 'profile-1',
      node_keys: ['nk-real'],
      interval_seconds: 60,
      timeout_seconds: 10,
      sampling_tier: 'regular',
    }))
    expect(mockedControl).not.toHaveBeenCalled()
  })

  it('suggests two minutes for a new focus task and persists the selected tier', async () => {
    wrapper = mount(MonitorJobsView)
    await flushPromises()
    await wrapper.find('.node-picker input[type="checkbox"]').setValue(true)

    wrapper.findAllComponents(UiSelect).find(component => component.props('ariaLabel') === '选择周期采样层级')!.vm.$emit('update:modelValue', 'focus')
    await flushPromises()

    const createButton = wrapper.findAll('button').find((button) => button.text().includes('创建（不会自动启动）'))
    await createButton!.trigger('click')
    await flushPromises()
    expect(mockedCreate).toHaveBeenCalledWith(expect.objectContaining({ sampling_tier: 'focus', interval_seconds: 120 }))
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

  it('saves a paused job tier without resuming or starting it', async () => {
    mockedJobs.mockResolvedValue([job('paused')])
    wrapper = mount(MonitorJobsView)
    await flushPromises()

    wrapper.findAllComponents(UiSelect).find(component => component.props('ariaLabel') === 'UI monitor 的周期采样层级')!.vm.$emit('update:modelValue', 'sparse')
    await flushPromises()
    const saveButton = wrapper.findAll('button').find((button) => button.text().trim() === '保存层级')
    expect(saveButton).toBeDefined()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(mockedUpdateTier).toHaveBeenCalledWith('job-1', 'sparse')
    expect(mockedControl).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('任务仍为已暂停')
    expect(wrapper.text()).toContain('已暂停')
  })

  it('runs a paused task diagnostic once without resuming or starting its schedule', async () => {
    mockedJobs.mockResolvedValue([job('paused')])
    wrapper = mount(MonitorJobsView)
    await flushPromises()
    const diagnostic = wrapper.findAll('button').find((button) => button.text().includes('手动诊断（一次）'))
    expect(diagnostic).toBeDefined()
    await diagnostic!.trigger('click')
    await flushPromises()
    expect(mockedTrigger).toHaveBeenCalledWith('job-1')
    expect(mockedControl).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('diagnostic 手动诊断')
  })

  it('shows an explicit blocked reason and requires a user start to retry', async () => {
    mockedJobs.mockResolvedValue([job('blocked', '节点配置 revision 已变化，需要重新确认')])

    wrapper = mount(MonitorJobsView)
    await flushPromises()

    expect(wrapper.text()).toContain('已阻塞')
    expect(wrapper.text()).toContain('节点配置 revision 已变化，需要重新确认')
    const retry = wrapper.findAll('button').find((button) => button.text().trim() === '重新检查并启动')
    expect(retry).toBeDefined()
    await retry!.trigger('click')
    await flushPromises()
    expect(mockedControl).toHaveBeenCalledWith('job-1', 'start')
  })

  it('applies Workbench prefill, supports cancel without creating, and sends the stable context on confirm', async () => {
    mockedJobs.mockResolvedValue([])
    wrapper = mount(MonitorJobsView, { props: { prefill } })
    await flushPromises()

    expect(wrapper.text()).toContain('工作台已选择')
    expect((wrapper.find('.node-picker input[type="checkbox"]').element as HTMLInputElement).checked).toBe(true)

    const cancel = wrapper.findAll('button').find((button) => button.text().includes('取消本次预填'))
    expect(cancel).toBeDefined()
    await cancel!.trigger('click')
    expect(mockedCreate).not.toHaveBeenCalled()
    expect((wrapper.find('.node-picker input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)

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
