import type { WorkbenchLatencyTest } from '../../types'

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
