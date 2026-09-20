import { describe, expect, it } from 'vitest'
import {
  durationNsToMs,
  normalizeCursorPage,
  normalizeDerivedStats,
  normalizeFacets,
  normalizeMonitorJob,
  normalizeMonitorNodeOption,
  normalizeMonitorRun,
  normalizeMonitorSample,
} from '../monitor'
import type {
  RawMonitorJobWire,
  RawMonitorNodeOptionWire,
  RawMonitorRunWire,
  RawDerivedStatsWire,
  RawMonitorSampleFacetsWire,
  RawMonitorSampleWire,
  RawSampleCursorPageWire,
} from '../../types'

describe('Go wire JSON -> Frontend decoder DTO contract', () => {
  it('keeps monitor-job durations in explicit seconds and never expects Go duration nanoseconds', () => {
    const option: RawMonitorNodeOptionWire = {
      profile_id: 'profile-1',
      profile_name: 'Stable subscription',
      node_key: 'nk-stable',
      node_identity_key: 'nid-stable',
      config_revision_key: 'rev-stable',
      display_name: 'Stable node',
      type: 'ss',
      country_code: 'JP',
      country_flag: '🇯🇵',
    }
    const job: RawMonitorJobWire = {
      id: 'job-1',
      name: 'Nightly monitor',
      profile_id: 'profile-1',
      profile_name: 'Stable subscription',
      node_keys: ['nk-stable'],
      nodes: [{
        node_key: 'nk-stable',
        node_identity_key: 'nid-stable',
        config_revision_key: 'rev-stable',
        display_name: 'Stable node',
        type: 'ss',
      }],
      probe_set: 'light',
      interval_seconds: 30,
      timeout_seconds: 5,
      state: 'stopped',
      created_at: '2026-09-19T00:00:00Z',
      updated_at: '2026-09-19T00:00:00Z',
    }
    const run: RawMonitorRunWire = {
      run_id: 'run-1',
      job_id: 'job-1',
      scheduled_at: '2026-09-19T00:00:00Z',
      started_at: '2026-09-19T00:00:00Z',
      status: 'completed',
      total_nodes: 1,
      success_nodes: 1,
      failed_nodes: 0,
    }

    expect(normalizeMonitorNodeOption(option).nodeKey).toBe('nk-stable')
    expect(normalizeMonitorJob(job).intervalSeconds).toBe(30)
    expect(normalizeMonitorJob(job).timeoutSeconds).toBe(5)
    expect(normalizeMonitorJob(job).nodes[0].nodeIdentityKey).toBe('nid-stable')
    expect(normalizeMonitorRun(run).successNodes).toBe(1)
  })

  it('correctly decodes Go time.Duration integer nanoseconds to floating milliseconds', () => {
    // Go time.Duration is serialized as integer nanoseconds: 42.5ms = 42,500,000ns
    expect(durationNsToMs(42_500_000)).toBe(42.5)
    expect(durationNsToMs(1_000_000)).toBe(1.0)
    expect(durationNsToMs(500_000)).toBe(0.5)
    expect(durationNsToMs(0)).toBe(0)
    expect(durationNsToMs(null)).toBe(0)
    expect(durationNsToMs(undefined)).toBe(0)
    expect(durationNsToMs('invalid')).toBe(0)
  })

  it('decodes a full RawMonitorSampleWire matching Go json.Marshal output', () => {
    // Exact shape produced by Go's json.Marshal(monitor.MonitorSample)
    const wireJson: RawMonitorSampleWire = {
      sample_id: 's_wire_001',
      run_id: 'r_wire_01',
      node_key: 'nk_wire_tokyo',
      node_identity_key: 'nid_wire_tokyo',
      config_revision_key: 'rev_hash_abc123',
      profile_id: 'prof_asia',
      display_name_snapshot: 'Tokyo Edge 01',
      probe_type: 'rtt',
      target: 'https://cp.cloudflare.com/generate_204',
      timestamp: '2026-09-17T12:34:56.789Z',
      success: true,
      latency: 28_400_000, // 28.4ms in nanoseconds
      ttfb: 14_200_000,    // 14.2ms in nanoseconds
      error_class: 'none',
      error_detail: '',
      exit_ip: '203.0.113.195',
      exit_region: 'JP',
      metadata: { datacenter: 'nrt-01' },
    }

    const decoded = normalizeMonitorSample(wireJson)

    expect(decoded.sampleId).toBe('s_wire_001')
    expect(decoded.runId).toBe('r_wire_01')
    expect(decoded.nodeKey).toBe('nk_wire_tokyo')
    expect(decoded.nodeIdentityKey).toBe('nid_wire_tokyo')
    expect(decoded.configRevisionKey).toBe('rev_hash_abc123')
    expect(decoded.profileId).toBe('prof_asia')
    expect(decoded.displayNameSnapshot).toBe('Tokyo Edge 01')
    expect(decoded.probeType).toBe('rtt')
    expect(decoded.target).toBe('https://cp.cloudflare.com/generate_204')
    expect(decoded.timestampIso).toBe('2026-09-17T12:34:56.789Z')
    expect(decoded.timestampMs).toBe(Date.parse('2026-09-17T12:34:56.789Z'))
    expect(decoded.success).toBe(true)
    expect(decoded.latencyMs).toBe(28.4)
    expect(decoded.ttfbMs).toBe(14.2)
    expect(decoded.errorClass).toBe('none')
    expect(decoded.errorDetail).toBe('')
    expect(decoded.exitIp).toBe('203.0.113.195')
    expect(decoded.exitRegion).toBe('JP')
    expect(decoded.metadata).toEqual({ datacenter: 'nrt-01' })
  })

  it('decodes a failure sample with transport error detail', () => {
    const wireJson: RawMonitorSampleWire = {
      sample_id: 's_wire_fail',
      run_id: 'r_wire_02',
      node_key: 'nk_fail',
      node_identity_key: 'nid_fail',
      config_revision_key: 'rev_1',
      profile_id: 'prof_1',
      display_name_snapshot: 'Failing Node',
      probe_type: 'rtt',
      target: 'https://cp.cloudflare.com/generate_204',
      timestamp: '2026-09-17T12:35:00.000Z',
      success: false,
      latency: 5_000_000_000, // 5s timeout in nanoseconds
      ttfb: 0,
      error_class: 'timeout',
      error_detail: 'i/o timeout after 5000ms',
    }

    const decoded = normalizeMonitorSample(wireJson)
    expect(decoded.success).toBe(false)
    expect(decoded.latencyMs).toBe(5000)
    expect(decoded.ttfbMs).toBe(0)
    expect(decoded.errorClass).toBe('timeout')
    expect(decoded.errorDetail).toBe('i/o timeout after 5000ms')
  })

  it('decodes a RawSampleCursorPageWire from the cursor endpoint', () => {
    const wirePage: RawSampleCursorPageWire = {
      items: [
        {
          sample_id: 'p1',
          run_id: 'r1',
          node_key: 'nk_p1',
          node_identity_key: 'nid_p1',
          config_revision_key: 'rev_1',
          profile_id: 'prof_1',
          display_name_snapshot: 'Node P1',
          probe_type: 'rtt',
          target: 'https://cp.cloudflare.com/generate_204',
          timestamp: '2026-09-17T12:00:00Z',
          success: true,
          latency: 20_000_000,
          ttfb: 0,
          error_class: 'none',
        },
      ],
      next_cursor: 'eyJ2IjoxLCJ0IjoxNzU4MTAwODAwMDAwMDAwMDAsImlkIjoicDEiLCJkaXIiOiJkZXNjIn0=',
      has_more: true,
      limit: 500,
    }

    const decoded = normalizeCursorPage(wirePage)
    expect(decoded.items).toHaveLength(1)
    expect(decoded.items[0].sampleId).toBe('p1')
    expect(decoded.items[0].latencyMs).toBe(20)
    expect(decoded.nextCursor).toBe(wirePage.next_cursor)
    expect(decoded.hasMore).toBe(true)
    expect(decoded.limit).toBe(500)
  })

  it('decodes RawDerivedStatsWire matching GetDerivedStats SQL response', () => {
    const wireStats: RawDerivedStatsWire = {
      sample_count: 240,
      success_count: 236,
      failure_count: 4,
      success_rate: 0.9833,
      latency_min_ms: 18.2,
      latency_p50_ms: 24.5,
      latency_p95_ms: 45.1,
      latency_max_ms: 120.0,
      ttfb_p50_ms: 31.0,
      ttfb_p95_ms: 55.4,
      error_breakdown: { timeout: 3, blocked: 1 },
      first_sample_at: '2026-09-16T12:00:00Z',
      last_sample_at: '2026-09-17T12:00:00Z',
      observed_since: '2026-09-16T00:00:00Z',
      observed_until: '2026-09-17T12:00:00Z',
    }

    const decoded = normalizeDerivedStats(wireStats)
    expect(decoded.sampleCount).toBe(240)
    expect(decoded.successCount).toBe(236)
    expect(decoded.failureCount).toBe(4)
    expect(decoded.successRate).toBeCloseTo(0.9833, 4)
    expect(decoded.latencyMinMs).toBe(18.2)
    expect(decoded.latencyP50Ms).toBe(24.5)
    expect(decoded.latencyP95Ms).toBe(45.1)
    expect(decoded.latencyMaxMs).toBe(120.0)
    expect(decoded.ttfbP50Ms).toBe(31.0)
    expect(decoded.ttfbP95Ms).toBe(55.4)
    expect(decoded.errorBreakdown).toEqual({ timeout: 3, blocked: 1 })
    expect(decoded.firstSampleAtMs).toBe(Date.parse('2026-09-16T12:00:00Z'))
    expect(decoded.lastSampleAtMs).toBe(Date.parse('2026-09-17T12:00:00Z'))
  })

  it('decodes RawMonitorSampleFacetsWire matching GetMonitorSampleFacets endpoint', () => {
    const wireFacets: RawMonitorSampleFacetsWire = {
      nodes: [
        {
          node_identity_key: 'nid_01',
          node_key: 'nk_01',
          display_name: 'Tokyo 01',
          profile_id: 'prof_main',
          sample_count: 150,
        },
      ],
      profiles: ['prof_main'],
      probe_types: ['rtt', 'ttfb'],
      targets: ['https://cp.cloudflare.com/generate_204'],
      window_since: '2026-09-10T12:00:00Z',
      window_until: '2026-09-17T12:00:00Z',
      truncated: false,
    }

    const decoded = normalizeFacets(wireFacets)
    expect(decoded.nodes).toHaveLength(1)
    expect(decoded.nodes[0].nodeIdentityKey).toBe('nid_01')
    expect(decoded.nodes[0].displayName).toBe('Tokyo 01')
    expect(decoded.nodes[0].sampleCount).toBe(150)
    expect(decoded.profiles).toEqual(['prof_main'])
    expect(decoded.probeTypes).toEqual(['rtt', 'ttfb'])
    expect(decoded.targets).toEqual(['https://cp.cloudflare.com/generate_204'])
    expect(decoded.windowSinceMs).toBe(Date.parse('2026-09-10T12:00:00Z'))
    expect(decoded.windowUntilMs).toBe(Date.parse('2026-09-17T12:00:00Z'))
    expect(decoded.truncated).toBe(false)
  })
})
