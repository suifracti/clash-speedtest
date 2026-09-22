import type { WorkbenchLatencyTest } from '../../types'

export type LatencyWindowMode = '4h' | '24h'

export interface LatencyWindow {
  mode: LatencyWindowMode
  since: string
  until: string
}

export function freezeLatencyWindow(mode: LatencyWindowMode, asOf = new Date()): LatencyWindow {
  const untilMs = asOf.getTime()
  if (!Number.isFinite(untilMs)) throw new Error('invalid latency observation as-of')
  const durationMs = mode === '4h' ? 4 * 60 * 60 * 1000 : 24 * 60 * 60 * 1000
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
