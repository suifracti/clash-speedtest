import { describe, expect, it } from 'vitest'
import { buildWorkbenchLatencyHistoryQuery } from '../bridge'

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
})
