import type { WorkbenchLatencyTest } from '../../types'

export type LatencyWindowMode = `${number}h` | `${number}d`

export interface LatencyWindow {
  mode: LatencyWindowMode
  since: string
  until: string
}

export function freezeLatencyWindow(mode: LatencyWindowMode, asOf = new Date()): LatencyWindow {
  const untilMs = asOf.getTime()
  if (!Number.isFinite(untilMs)) throw new Error('invalid latency observation as-of')
  const match = /^(\d+(?:\.\d+)?)(h|d)$/.exec(mode)
  const amount = match ? Number(match[1]) : NaN
  const durationMs = amount * (match?.[2] === 'd' ? 24 : 1) * 3600000
  if (!Number.isFinite(durationMs) || durationMs < 1 || !Number.isFinite(new Date(untilMs - durationMs).getTime())) {
    throw new Error('请输入有效的正数时间范围')
  }
  return {
    mode,
    since: new Date(untilMs - durationMs).toISOString(),
    until: new Date(untilMs).toISOString(),
  }
}

export interface LatencyScope {
  profileId: string
  nodeKey: string
}

export interface LatencyAttemptScope extends LatencyScope {
  attemptId: string
}

export function sameLatencyScope(left: LatencyScope, right: LatencyScope): boolean {
  return left.profileId !== '' && left.nodeKey !== '' &&
    left.profileId === right.profileId && left.nodeKey === right.nodeKey
}

export function testMatchesLatencyScope(test: Pick<WorkbenchLatencyTest, 'profile_id' | 'node_key'>, scope: LatencyScope): boolean {
  return test.profile_id === scope.profileId && test.node_key === scope.nodeKey
}

export function acceptsLatencyScopeResponse(
  requestId: number,
  latestRequestId: number,
  requestedScope: LatencyScope,
  currentScope: LatencyScope,
): boolean {
  return requestId === latestRequestId && sameLatencyScope(requestedScope, currentScope)
}

export function acceptsLatencyTestResult(
  requestId: number,
  latestRequestId: number,
  requestedScope: LatencyScope,
  currentScope: LatencyScope,
  result: Pick<WorkbenchLatencyTest, 'profile_id' | 'node_key' | 'attempt_id'>,
): boolean {
  return acceptsLatencyScopeResponse(requestId, latestRequestId, requestedScope, currentScope) &&
    result.attempt_id.trim() !== '' && testMatchesLatencyScope(result, requestedScope)
}

export function acceptsLatencyDetailResponse(
  requestId: number,
  latestRequestId: number,
  requested: LatencyAttemptScope,
  currentScope: LatencyScope,
  currentAttemptId: string,
  result: Pick<WorkbenchLatencyTest, 'profile_id' | 'node_key' | 'attempt_id'>,
): boolean {
  return requestId === latestRequestId &&
    sameLatencyScope(requested, currentScope) &&
    currentAttemptId === requested.attemptId &&
    result.attempt_id === requested.attemptId &&
    testMatchesLatencyScope(result, requested)
}
