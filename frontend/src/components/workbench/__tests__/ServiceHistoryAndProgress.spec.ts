import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ServiceHistoryTooltip from '../ServiceHistoryTooltip.vue'
import ServiceComparison from '../ServiceComparison.vue'
import type { WorkbenchPublicServiceAttempt, WorkbenchPublicServiceRule } from '../../../types'

describe('ServiceHistoryTooltip & Service Comparison Progress', () => {
  const dummyRule: WorkbenchPublicServiceRule = {
    service_id: 'antigravity',
    name: 'Google Antigravity',
    category: 'AI 服务',
    rule_version: 2,
    result_kind: 'account_availability',
    success_criterion: '必须收到真实模型回答',
    target_url: 'https://example.com/api',
    method: 'GET',
    redirect_policy: 'follow',
    timeout_seconds: 10,
    maximum_body_bytes: 1024,
  }

  const dummyAttempt1: WorkbenchPublicServiceAttempt = {
    attempt_id: 'att-1',
    request_id: 'req-1',
    profile_id: 'prof-1',
    node_key: 'node-hk',
    node_identity_key: 'id-hk',
    config_revision_key: 'rev-1',
    node_type: 'ss',
    source: 'workbench',
    service_id: 'antigravity',
    display_name: '香港01节点',
    execution_state: 'completed',
    persistence_state: 'saved',
    requested_at: '2026-09-28T10:00:00Z',
    finished_at: '2026-09-28T10:00:02Z',
    rule: dummyRule,
    result: {
      outcome: 'matched',
      duration_ms: 1200,
      http_status: 200,
      started_at: '2026-09-28T10:00:00Z',
      finished_at: '2026-09-28T10:00:02Z',
      bytes_read: 4096,
      summary: '真实模型响应成功 (gemini-pro)',
      details: {
        ip: '104.28.19.2',
        loc: 'HK',
        checked_model: 'gemini-pro',
      },
    },
  }

  const dummyAttempt2: WorkbenchPublicServiceAttempt = {
    attempt_id: 'att-2',
    request_id: 'req-2',
    profile_id: 'prof-1',
    node_key: 'node-hk',
    node_identity_key: 'id-hk',
    config_revision_key: 'rev-1',
    node_type: 'ss',
    source: 'workbench',
    service_id: 'antigravity',
    display_name: '香港01节点',
    execution_state: 'completed',
    persistence_state: 'saved',
    requested_at: '2026-09-28T11:00:00Z',
    finished_at: '2026-09-28T11:00:04Z',
    rule: dummyRule,
    result: {
      outcome: 'matched',
      duration_ms: 4328,
      http_status: 200,
      started_at: '2026-09-28T11:00:00Z',
      finished_at: '2026-09-28T11:00:04Z',
      bytes_read: 8192,
      summary: '真实模型响应成功 (gemini-pro)',
      details: {
        ip: '172.67.142.8', // Different IP -> IP Drift!
        loc: 'HK',
        checked_model: 'gemini-pro',
      },
    },
  }

  it('renders rich hover tooltip with history comparison, delta latency, and IP drift detection', async () => {
    const wrapper = mount(ServiceHistoryTooltip, {
      props: {
        visible: true,
        x: 100,
        y: 200,
        serviceName: 'Google Antigravity',
        serviceId: 'antigravity',
        nodeName: '香港01节点',
        nodeFlag: '🇭🇰',
        countryCode: 'HK',
        history: [dummyAttempt1, dummyAttempt2],
        teleportDisabled: true,
      },
    })

    // Assert header
    expect(wrapper.text()).toContain('香港01节点')
    expect(wrapper.text()).toContain('Google Antigravity')

    // Assert latest result (att-2 is newer with 4328ms)
    expect(wrapper.text()).toContain('4328 ms')
    expect(wrapper.text()).toContain('HTTP 200')

    // Assert delta from previous (+3128 ms slower)
    expect(wrapper.text()).toContain('+3128 ms')
    expect(wrapper.text()).toContain('变慢')

    // Assert IP drift detected because dummyAttempt1 has 104.28.19.2 and dummyAttempt2 has 172.67.142.8
    expect(wrapper.text()).toContain('检测到落地 IP 漂移')
    expect(wrapper.text()).toContain('104.28.19.2')
    expect(wrapper.text()).toContain('172.67.142.8')

    // Assert history list contains past runs
    expect(wrapper.text()).toContain('测试历史记录 (2 次采样)')
    expect(wrapper.text()).toContain('本次')
    expect(wrapper.text()).toContain('上次')

    // Assert retest action buttons inside tooltip
    expect(wrapper.text()).toContain('⚡ 测1次')
    expect(wrapper.text()).toContain('⚡ 连测3次')

    // Trigger retest 3 times
    const retest3Btn = wrapper.findAll('button').find(b => b.text().includes('连测3次'))
    expect(retest3Btn).toBeDefined()
    await retest3Btn!.trigger('click')
    expect(wrapper.emitted('retest')).toBeTruthy()
    expect(wrapper.emitted('retest')![0][0]).toEqual({
      nodeKey: '',
      serviceId: 'antigravity',
      repeatCount: 3,
    })
  })

  it('opens centered modal inspector in ServiceComparison with drift panel, sparkline, and repeat count', async () => {
    const rows = [
      {
        key: 'hk-1',
        node: {
          profileId: 'prof-1',
          profileName: '默认订阅',
          nodeKey: 'hk-1',
          nodeIdentityKey: 'id-hk',
          configRevisionKey: 'rev-1',
          displayName: '香港01节点',
          countryCode: 'HK',
          countryFlag: '🇭🇰',
          type: 'vless',
        },
      },
    ]

    const wrapper = mount(ServiceComparison, {
      props: {
        rows,
        services: [{ value: 'antigravity', label: 'Google Antigravity' }],
        records: {
          'hk-1': [dummyAttempt1, dummyAttempt2],
        },
        selected: [],
        states: {},
        partial: {},
      },
    })

    // Verify row has drift badge
    expect(wrapper.get('.node-drift-badge').text()).toContain('⇄ 漂移')
    expect(wrapper.get('.pill-flapping-tag').text()).toContain('⇄')

    // Verify inline sparkline exists inside overview service pill
    expect(wrapper.find('.pill-inline-sparkline').exists()).toBe(true)

    // Verify batch test emits repeat count (default 3)
    const runAllBtn = wrapper.findAll('.tool-btn.primary').find(b => b.text().includes('⚡ 检测全部'))
    expect(runAllBtn).toBeDefined()
    await runAllBtn!.trigger('click')
    expect(wrapper.emitted('run')).toBeTruthy()
    expect(wrapper.emitted('run')![0]).toEqual([['hk-1'], 3])

    // Click the pill to open centered inspector
    await wrapper.get('.overview-service-pill').trigger('click')

    // Inspector modal backdrop and dialog should exist
    expect(wrapper.find('.service-inspector-modal-backdrop').exists()).toBe(true)
    const inspector = wrapper.get('.service-inspector')
    expect(inspector.text()).toContain('Google Antigravity')
    expect(inspector.text()).toContain('漂移检测分析')
    expect(inspector.text()).toContain('检出 2 个不同落地出口 IP')
    expect(inspector.text()).toContain('完整测试历史记录 (2 次)')
    expect(inspector.text()).toContain('重新检测此项')
  })
})
