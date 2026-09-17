import { describe, expect, it } from 'vitest'
import {
  buildLanes,
  detectRevisionBoundaries,
  isLegacyBackfilledSample,
  laneKeyFor,
  shortenKey,
} from '../lanes'
import { BASE_TS, makeSample } from './fixtures'

const MIN = 60_000

describe('lane construction', () => {
  it('separates lanes by node identity, probe type and target', () => {
    const samples = [
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'rtt', target: 'https://a.example/1' }),
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'ttfb', target: 'https://a.example/1' }),
      makeSample({ nodeIdentityKey: 'nid_a', probeType: 'rtt', target: 'https://a.example/2' }),
      makeSample({ nodeIdentityKey: 'nid_b', probeType: 'rtt', target: 'https://a.example/1' }),
    ]

    const lanes = buildLanes(samples)
    expect(lanes).toHaveLength(4)
    for (const lane of lanes) {
      expect(lane.samples).toHaveLength(1)
    }
  })

  it('keeps a node-level outage and a single-service outage in different lanes', () => {
    // Same node, same instant: the transport probe fails while the AI-service probe is blocked.
    const samples = [
      makeSample({
        nodeIdentityKey: 'nid_a',
        probeType: 'rtt',
        target: 'https://cp.cloudflare.com/generate_204',
        success: false,
        errorClass: 'timeout',
        sampleId: 'a1',
      }),
      makeSample({
        nodeIdentityKey: 'nid_a',
        probeType: 'ai_check',
        target: 'https://ai.example/v1',
        success: false,
        errorClass: 'blocked',
        sampleId: 'a2',
      }),
    ]

    const lanes = buildLanes(samples)
    expect(lanes).toHaveLength(2)
    const probeTypes = lanes.map((l) => l.probeType).sort()
    expect(probeTypes).toEqual(['ai_check', 'rtt'])
  })

  it('sorts samples inside a lane ascending by time', () => {
    const samples = [
      makeSample({ timestampMs: BASE_TS + 3 * MIN, sampleId: 'c' }),
      makeSample({ timestampMs: BASE_TS + 1 * MIN, sampleId: 'a' }),
      makeSample({ timestampMs: BASE_TS + 2 * MIN, sampleId: 'b' }),
    ]
    const [lane] = buildLanes(samples)
    expect(lane.samples.map((s) => s.sampleId)).toEqual(['a', 'b', 'c'])
  })

  it('orders lanes deterministically regardless of input order', () => {
    const a = makeSample({ nodeIdentityKey: 'nid_a', displayNameSnapshot: 'Alpha', sampleId: 'x1' })
    const b = makeSample({ nodeIdentityKey: 'nid_b', displayNameSnapshot: 'Beta', sampleId: 'x2' })

    const forward = buildLanes([a, b]).map((l) => l.displayName)
    const reverse = buildLanes([b, a]).map((l) => l.displayName)
    expect(forward).toEqual(reverse)
    expect(forward).toEqual(['Alpha', 'Beta'])
  })

  it('builds a stable lane key', () => {
    const s = makeSample({ nodeIdentityKey: 'nid_a', probeType: 'rtt', target: 'https://t/x' })
    expect(laneKeyFor(s)).toBe('nid_a\u0000rtt\u0000https://t/x')
  })

  it('falls back to the legacy node key when no identity is present', () => {
    const s = makeSample({ nodeIdentityKey: '', nodeKey: 'nk_legacy' })
    expect(laneKeyFor(s)).toContain('nk_legacy')
  })
})

describe('ConfigRevision boundary detection', () => {
  it('finds a single boundary at the first sample under the new revision', () => {
    const samples = [
      makeSample({ timestampMs: BASE_TS + 0 * MIN, configRevisionKey: 'rev_1', sampleId: 'r1' }),
      makeSample({ timestampMs: BASE_TS + 1 * MIN, configRevisionKey: 'rev_1', sampleId: 'r2' }),
      makeSample({ timestampMs: BASE_TS + 2 * MIN, configRevisionKey: 'rev_2', sampleId: 'r3' }),
      makeSample({ timestampMs: BASE_TS + 3 * MIN, configRevisionKey: 'rev_2', sampleId: 'r4' }),
    ]

    const boundaries = detectRevisionBoundaries(samples)
    expect(boundaries).toHaveLength(1)
    expect(boundaries[0].atMs).toBe(BASE_TS + 2 * MIN)
    expect(boundaries[0].fromRevision).toBe('rev_1')
    expect(boundaries[0].toRevision).toBe('rev_2')
    expect(boundaries[0].nodeIdentityKey).toBe('nid_jp01')
  })

  it('reports no boundary when the revision is constant', () => {
    const samples = Array.from({ length: 5 }, (_, i) =>
      makeSample({ timestampMs: BASE_TS + i * MIN, configRevisionKey: 'rev_1', sampleId: `c${i}` })
    )
    expect(detectRevisionBoundaries(samples)).toEqual([])
  })

  it('ignores empty revision keys so legacy rows cannot fabricate a boundary', () => {
    const samples = [
      makeSample({ timestampMs: BASE_TS, configRevisionKey: 'rev_1', sampleId: 'e1' }),
      makeSample({ timestampMs: BASE_TS + MIN, configRevisionKey: '', sampleId: 'e2' }),
      makeSample({ timestampMs: BASE_TS + 2 * MIN, configRevisionKey: 'rev_1', sampleId: 'e3' }),
    ]
    expect(detectRevisionBoundaries(samples)).toEqual([])
  })

  it('does not emit a boundary when several probes share the same revision', () => {
    const samples = [
      makeSample({ timestampMs: BASE_TS, probeType: 'rtt', configRevisionKey: 'rev_1', sampleId: 'p1' }),
      makeSample({ timestampMs: BASE_TS, probeType: 'ttfb', configRevisionKey: 'rev_1', sampleId: 'p2' }),
      makeSample({ timestampMs: BASE_TS + MIN, probeType: 'rtt', configRevisionKey: 'rev_1', sampleId: 'p3' }),
      makeSample({ timestampMs: BASE_TS + MIN, probeType: 'ttfb', configRevisionKey: 'rev_1', sampleId: 'p4' }),
    ]
    expect(detectRevisionBoundaries(samples)).toEqual([])
  })

  it('resolves a mixed instant deterministically by majority', () => {
    const build = (order: string[]) =>
      detectRevisionBoundaries(
        order.map((rev, i) =>
          makeSample({
            timestampMs: BASE_TS + (i < 3 ? 0 : MIN),
            configRevisionKey: rev,
            probeType: `probe_${i}`,
            sampleId: `m${i}`,
          })
        )
      )

    // Instant 0 has two rev_2 and one rev_1 → rev_2 wins.
    const a = build(['rev_1', 'rev_2', 'rev_2', 'rev_2'])
    const b = build(['rev_2', 'rev_1', 'rev_2', 'rev_2'])
    expect(a).toEqual(b)
  })

  it('tracks boundaries independently per node', () => {
    const samples = [
      makeSample({ nodeIdentityKey: 'nid_a', timestampMs: BASE_TS, configRevisionKey: 'rev_1', sampleId: 'a1' }),
      makeSample({ nodeIdentityKey: 'nid_a', timestampMs: BASE_TS + MIN, configRevisionKey: 'rev_2', sampleId: 'a2' }),
      makeSample({ nodeIdentityKey: 'nid_b', timestampMs: BASE_TS, configRevisionKey: 'rev_9', sampleId: 'b1' }),
      makeSample({ nodeIdentityKey: 'nid_b', timestampMs: BASE_TS + MIN, configRevisionKey: 'rev_9', sampleId: 'b2' }),
    ]

    const boundaries = detectRevisionBoundaries(samples)
    expect(boundaries).toHaveLength(1)
    expect(boundaries[0].nodeIdentityKey).toBe('nid_a')
  })

  it('returns boundaries in ascending time order', () => {
    const samples = [
      makeSample({ timestampMs: BASE_TS, configRevisionKey: 'rev_1', sampleId: 'z1' }),
      makeSample({ timestampMs: BASE_TS + MIN, configRevisionKey: 'rev_2', sampleId: 'z2' }),
      makeSample({ timestampMs: BASE_TS + 2 * MIN, configRevisionKey: 'rev_3', sampleId: 'z3' }),
    ]
    const boundaries = detectRevisionBoundaries(samples)
    expect(boundaries.map((b) => b.toRevision)).toEqual(['rev_2', 'rev_3'])
  })
})

describe('key display helpers', () => {
  it('shortens a long opaque key but keeps both ends', () => {
    const long = 'abcdef1234567890abcdef1234567890'
    const short = shortenKey(long)
    expect(short.startsWith('abcdef')).toBe(true)
    expect(short.endsWith('7890')).toBe(true)
    expect(short).toContain('…')
  })

  it('leaves a short key untouched', () => {
    expect(shortenKey('rev_1')).toBe('rev_1')
    expect(shortenKey('')).toBe('—')
  })

  it('detects PR#3 backfilled legacy rows', () => {
    expect(isLegacyBackfilledSample(makeSample({ nodeKey: 'nk_x', nodeIdentityKey: 'nk_x' }))).toBe(true)
    expect(isLegacyBackfilledSample(makeSample({ nodeKey: 'nk_x', nodeIdentityKey: 'nid_x' }))).toBe(false)
  })
})
