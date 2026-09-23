import type { MonitorSample } from '../../../types'

const BASE_TS = Date.parse('2026-09-17T10:00:00.000Z')

let seq = 0

export function resetSampleSeq(): void {
  seq = 0
}

/** Builds a normalized raw sample with sensible defaults. */
export function makeSample(overrides: Partial<MonitorSample> = {}): MonitorSample {
  seq += 1
  const timestampMs = overrides.timestampMs ?? BASE_TS
  return {
    sampleId: `s_${String(seq).padStart(4, '0')}`,
    runId: 'run_1',
    samplingTier: 'regular',
    triggerType: 'scheduled',
    samplingStrategyVersion: 1,
    nodeKey: 'nk_jp01',
    nodeIdentityKey: 'nid_jp01',
    configRevisionKey: 'rev_1',
    profileId: 'prof_1',
    displayNameSnapshot: 'JP01',
    probeType: 'rtt',
    target: 'https://cp.cloudflare.com/generate_204',
    timestampMs,
    timestampIso: new Date(timestampMs).toISOString(),
    success: true,
    latencyMs: 42,
    ttfbMs: 30,
    errorClass: 'none',
    ...overrides,
  }
}

/** Builds `count` samples one minute apart inside a single lane. */
export function makeSeries(
  count: number,
  overrides: Partial<MonitorSample> = {},
  startMs: number = BASE_TS,
  stepMs = 60_000
): MonitorSample[] {
  const out: MonitorSample[] = []
  for (let i = 0; i < count; i += 1) {
    out.push(
      makeSample({
        ...overrides,
        sampleId: `${overrides.sampleId ?? 'ser'}_${String(i).padStart(5, '0')}`,
        timestampMs: startMs + i * stepMs,
      })
    )
  }
  return out
}

export { BASE_TS }
