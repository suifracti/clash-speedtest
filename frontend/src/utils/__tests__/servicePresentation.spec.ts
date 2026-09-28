import { expect, it } from 'vitest'
import { summarizeService, serviceGroup } from '../servicePresentation'
import type { WorkbenchPublicServiceAttempt, WorkbenchPublicServiceRule } from '../../types'

function attempt(id: string, outcome: string, ip?: string, version = 4, model = 'example-model') {
  return { attempt_id: id, rule: { rule_version: version }, result: { outcome, model, finished_at: `2026-09-27T00:00:0${id}Z`, details: ip ? { ip } : {} } } as WorkbenchPublicServiceAttempt
}
it('counts repeated results and only observed adjacent exit changes, excluding older criteria', () => {
  const first = attempt('1', 'matched', '203.0.113.1')
  const summary = summarizeService([attempt('0', 'matched', '203.0.113.9', 3), first, first,
    attempt('2', 'matched', '203.0.113.2'), attempt('3', 'region_blocked'), attempt('4', 'matched', '203.0.113.3')])
  expect(summary).toMatchObject({ total: 4, passed: 3, rate: 75, changes: 2, exitChanges: 1, exitPairs: 1 })
})
it('does not compare model switches as network instability and groups additional AI services', () => {
  expect(summarizeService([attempt('1', 'matched'), attempt('2', 'rate_limited', undefined, 4, 'another-model')])).toMatchObject({ total: 1, rate: 0, changes: 0 })
  expect(serviceGroup({ service_id: 'claude_web', category: 'AI 与平台' } as WorkbenchPublicServiceRule)).toBe('AI 助手')
  expect(serviceGroup({ service_id: 'lemino_web', category: '日本服务' } as WorkbenchPublicServiceRule)).toBe('视频与音乐')
})
it('keeps failures before model discovery in the availability denominator', () => {
  const unavailable = attempt('2', 'transport_error', undefined, 4, '')
  expect(summarizeService([attempt('1', 'matched'), unavailable])).toMatchObject({ total: 2, passed: 1, rate: 50, changes: 1, latest: unavailable })
})
