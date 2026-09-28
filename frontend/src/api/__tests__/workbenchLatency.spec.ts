import { describe, expect, it, vi } from 'vitest'
import { buildWorkbenchLatencyHistoryQuery, fetchWorkbenchLatencyHistories, subscribeEvents } from '../bridge'

describe('Workbench latency history request contract', () => {
  it('sends the frozen UTC half-open bounds before the attempt limit', () => {
    const params = new URLSearchParams(buildWorkbenchLatencyHistoryQuery({
      profile_id: 'profile-a',
      node_key: 'node-shared',
      node_identity_key: 'identity-a',
      config_revision_key: 'rev-a',
      since: '2026-09-22T08:00:00.000Z',
      until: '2026-09-22T12:00:00.000Z',
      limit: 20,
      before_finished_at: '2026-09-22T11:00:00.000Z',
      before_attempt_id: 'attempt-a',
    }))

    expect(params.get('profile_id')).toBe('profile-a')
    expect(params.get('node_key')).toBe('node-shared')
    expect(params.get('node_identity_key')).toBe('identity-a')
    expect(params.get('config_revision_key')).toBe('rev-a')
    expect(params.get('since')).toBe('2026-09-22T08:00:00.000Z')
    expect(params.get('until')).toBe('2026-09-22T12:00:00.000Z')
    expect(params.get('limit')).toBe('20')
    expect(params.get('before_finished_at')).toBe('2026-09-22T11:00:00.000Z')
    expect(params.get('before_attempt_id')).toBe('attempt-a')
  })

  it('reads multiple stable node scopes in one Web request', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => [{ tests: [] }, { tests: [] }] })
    vi.stubGlobal('fetch', fetchMock)
    try {
      const since = '2026-09-22T08:00:00.000Z'
      const until = '2026-09-22T12:00:00.000Z'
      const queries = ['profile-a', 'profile-b'].map((profile_id) => ({
        profile_id, node_key: 'same-name', node_identity_key: `identity-${profile_id}`, config_revision_key: 'rev-1', since, until, limit: 100,
      }))
      expect(await fetchWorkbenchLatencyHistories(queries)).toHaveLength(2)
      expect(fetchMock).toHaveBeenCalledTimes(1)
      const [url, options] = fetchMock.mock.calls[0]
      expect(url).toBe('/api/workbench/latency-tests/query')
      expect(JSON.parse(options.body)).toMatchObject({ since, until, limit: 100, nodes: [
        { profile_id: 'profile-a', node_key: 'same-name', node_identity_key: 'identity-profile-a' },
        { profile_id: 'profile-b', node_key: 'same-name', node_identity_key: 'identity-profile-b' },
      ] })
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('shares one local event connection across page subscribers', () => {
    const sources: Array<{ onmessage: ((event: { data: string }) => void) | null; close: ReturnType<typeof vi.fn> }> = []
    class FakeEventSource {
      onmessage: ((event: { data: string }) => void) | null = null
      close = vi.fn()
      constructor() { sources.push(this) }
    }
    vi.stubGlobal('EventSource', FakeEventSource)
    const first = vi.fn()
    const second = vi.fn()
    const stopFirst = subscribeEvents(first)
    const stopSecond = subscribeEvents(second)
    try {
      expect(sources).toHaveLength(1)
      sources[0].onmessage?.({ data: JSON.stringify({ type: 'workbench_latency_test_completed', payload: { attempt_id: 'fake-1' } }) })
      expect(first).toHaveBeenCalledTimes(1)
      expect(second).toHaveBeenCalledTimes(1)
      stopFirst()
      expect(sources[0].close).not.toHaveBeenCalled()
      stopSecond()
      expect(sources[0].close).toHaveBeenCalledTimes(1)
    } finally {
      stopFirst()
      stopSecond()
      vi.unstubAllGlobals()
    }
  })
})
