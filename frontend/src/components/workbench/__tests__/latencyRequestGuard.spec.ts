import { describe, expect, it } from 'vitest'
import {
  acceptsLatencyDetailResponse,
  acceptsLatencyScopeResponse,
  freezeLatencyWindow,
  type LatencyAttemptScope,
  type LatencyScope,
} from '../latencyRequestGuard'

const scopeA: LatencyScope = { profileId: 'profile-a', nodeKey: 'node-a' }
const scopeB: LatencyScope = { profileId: 'profile-b', nodeKey: 'node-b' }

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

describe('latency request ownership', () => {
  it('freezes rolling UTC since/until from one as-of', () => {
    const asOf = new Date('2026-09-22T12:34:56.789Z')

    expect(freezeLatencyWindow('4h', asOf)).toEqual({
      mode: '4h',
      since: '2026-09-22T08:34:56.789Z',
      until: '2026-09-22T12:34:56.789Z',
    })
    expect(freezeLatencyWindow('24h', asOf)).toEqual({
      mode: '24h',
      since: '2026-09-21T12:34:56.789Z',
      until: '2026-09-22T12:34:56.789Z',
    })
  })

  it('rejects a late same-node response after a window request supersedes it', () => {
    expect(acceptsLatencyScopeResponse(1, 2, scopeA, scopeA)).toBe(false)
  })

  it('rejects a late A history response after the user has moved to B', async () => {
    let latestRequestID = 0
    const requestA = ++latestRequestID
    const responseA = deferred<string>()
    const requestB = ++latestRequestID
    const responseB = deferred<string>()
    const applied: string[] = []

    const apply = async (requestID: number, requestedScope: LatencyScope, response: Promise<string>) => {
      const value = await response
      if (acceptsLatencyScopeResponse(requestID, latestRequestID, requestedScope, requestID === requestB ? scopeB : scopeB)) {
        applied.push(value)
      }
    }

    const b = apply(requestB, scopeB, responseB.promise)
    const a = apply(requestA, scopeA, responseA.promise)
    responseB.resolve('B history')
    await b
    responseA.resolve('A late history')
    await a

    expect(applied).toEqual(['B history'])
  })

  it('keeps the newest detail selection when responses return in reverse order', async () => {
    let latestRequestID = 0
    const requestedA: LatencyAttemptScope = { ...scopeA, attemptId: 'attempt-a' }
    const requestedB: LatencyAttemptScope = { ...scopeA, attemptId: 'attempt-b' }
    const requestA = ++latestRequestID
    const responseA = deferred<{ profile_id: string; node_key: string; attempt_id: string }>()
    const requestB = ++latestRequestID
    const responseB = deferred<{ profile_id: string; node_key: string; attempt_id: string }>()
    let selected = ''

    const apply = async (requestID: number, requested: LatencyAttemptScope, response: typeof responseA.promise) => {
      const value = await response
      if (acceptsLatencyDetailResponse(requestID, latestRequestID, requested, scopeA, 'attempt-b', value)) {
        selected = value.attempt_id
      }
    }

    const b = apply(requestB, requestedB, responseB.promise)
    const a = apply(requestA, requestedA, responseA.promise)
    responseB.resolve({ profile_id: 'profile-a', node_key: 'node-a', attempt_id: 'attempt-b' })
    await b
    responseA.resolve({ profile_id: 'profile-a', node_key: 'node-a', attempt_id: 'attempt-a' })
    await a

    expect(selected).toBe('attempt-b')
  })
})
