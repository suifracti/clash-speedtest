/**
 * Monitor history API surface for the timeline.
 *
 * This module is the ONLY place that knows about the raw wire shape. In particular it owns the
 * nanosecond → millisecond conversion for `latency` / `ttfb`, because the Go backend serializes
 * `time.Duration` as an integer count of nanoseconds. Everywhere else in the app works in
 * milliseconds on a normalized `MonitorSample`.
 *
 * The timeline consumes the already-approved `QueryMonitorSamplesCursor` / `GetMonitorStats`
 * read models. It never re-implements history querying and never mutates retention, scheduler or
 * monitor lifecycle state.
 */

import { isWails } from './bridge'
import type {
	DerivedStats,
	FacetNode,
	MonitorSample,
	MonitorSampleFacets,
	MonitorJob,
	MonitorJobCreateRequest,
	MonitorJobNode,
	MonitorNodeOption,
	MonitorRun,
	RawDerivedStatsWire,
	RawFacetNodeWire,
	RawMonitorJobNodeWire,
	RawMonitorJobWire,
	RawMonitorSampleFacetsWire,
	RawMonitorSampleWire,
	RawMonitorNodeOptionWire,
	RawMonitorRunWire,
	MonitorRetentionRequest,
	MonitorRetentionPreview,
	MonitorRetentionResult,
	MonitorStorageUsage,
	MonitorBudgetStatus,
	MonitorSamplingTier,
	RawSampleCursorPageWire,
	SampleCursorPage,
} from '../types'

const API_BASE = ''

const NS_PER_MS = 1e6

/** Converts a Go `time.Duration` serialized as nanoseconds into milliseconds. */
export function durationNsToMs(ns: unknown): number {
  if (typeof ns !== 'number' || !Number.isFinite(ns)) return 0
  return ns / NS_PER_MS
}

function parseIsoMs(iso: string | null | undefined): number | null {
  if (!iso) return null
  const ms = Date.parse(iso)
  return Number.isFinite(ms) ? ms : null
}

export function normalizeMonitorSample(wire: RawMonitorSampleWire): MonitorSample {
  return {
    sampleId: wire.sample_id,
    runId: wire.run_id,
    samplingTier: wire.sampling_tier ?? 'legacy_unknown',
    triggerType: wire.trigger_type ?? 'legacy_unknown',
    samplingStrategyVersion: wire.sampling_strategy_version ?? 0,
    nodeKey: wire.node_key ?? '',
    nodeIdentityKey: wire.node_identity_key ?? '',
    configRevisionKey: wire.config_revision_key ?? '',
    profileId: wire.profile_id ?? '',
    displayNameSnapshot: wire.display_name_snapshot ?? '',
    probeType: wire.probe_type ?? '',
    target: wire.target ?? '',
    timestampMs: parseIsoMs(wire.timestamp) ?? 0,
    timestampIso: wire.timestamp ?? '',
    success: !!wire.success,
    latencyMs: durationNsToMs(wire.latency),
    ttfbMs: durationNsToMs(wire.ttfb),
    errorClass: wire.error_class ?? '',
    errorDetail: wire.error_detail,
    exitIp: wire.exit_ip,
    exitRegion: wire.exit_region,
    metadata: wire.metadata,
  }
}

export function normalizeCursorPage(wire: RawSampleCursorPageWire): SampleCursorPage {
  const items = Array.isArray(wire.items) ? wire.items : []
  return {
    items: items.map(normalizeMonitorSample),
    nextCursor: wire.next_cursor ?? '',
    hasMore: !!wire.has_more,
    limit: wire.limit ?? 0,
  }
}

export function normalizeDerivedStats(wire: RawDerivedStatsWire): DerivedStats {
  return {
    sampleCount: wire.sample_count ?? 0,
    includedSamplingTiers: Array.isArray(wire.included_sampling_tiers) ? wire.included_sampling_tiers : [],
    regularObservationOnly: !!wire.regular_observation_only,
    successCount: wire.success_count ?? 0,
    failureCount: wire.failure_count ?? 0,
    successRate: typeof wire.success_rate === 'number' ? wire.success_rate : 0,
    latencyMinMs: wire.latency_min_ms ?? null,
    latencyP50Ms: wire.latency_p50_ms ?? null,
    latencyP95Ms: wire.latency_p95_ms ?? null,
    latencyMaxMs: wire.latency_max_ms ?? null,
    ttfbP50Ms: wire.ttfb_p50_ms ?? null,
    ttfbP95Ms: wire.ttfb_p95_ms ?? null,
    errorBreakdown: wire.error_breakdown ?? {},
    firstSampleAtMs: parseIsoMs(wire.first_sample_at),
    lastSampleAtMs: parseIsoMs(wire.last_sample_at),
  }
}

function normalizeFacetNode(wire: RawFacetNodeWire): FacetNode {
  return {
    nodeIdentityKey: wire.node_identity_key ?? '',
    nodeKey: wire.node_key ?? '',
    displayName: wire.display_name ?? '',
    profileId: wire.profile_id ?? '',
    sampleCount: wire.sample_count ?? 0,
  }
}

export function normalizeFacets(wire: RawMonitorSampleFacetsWire): MonitorSampleFacets {
  return {
    nodes: (Array.isArray(wire.nodes) ? wire.nodes : []).map(normalizeFacetNode),
    profiles: Array.isArray(wire.profiles) ? wire.profiles : [],
    probeTypes: Array.isArray(wire.probe_types) ? wire.probe_types : [],
    targets: Array.isArray(wire.targets) ? wire.targets : [],
    windowSinceMs: parseIsoMs(wire.window_since) ?? 0,
    windowUntilMs: parseIsoMs(wire.window_until) ?? 0,
    truncated: !!wire.truncated,
  }
}

/** Filter shared by the cursor and stats read models. */
export interface MonitorQueryFilter {
  nodeIdentityKey?: string
  legacyNodeKey?: string
  nodeKey?: string
  profileId?: string
  probeType?: string
  target?: string
  samplingTier?: 'regular' | 'focus' | 'sparse' | 'diagnostic' | 'legacy_unknown'
  regularObservationOnly?: boolean
  sinceMs?: number | null
  untilMs?: number | null
}

export interface MonitorCursorQuery extends MonitorQueryFilter {
  limit?: number
  orderDesc?: boolean
  cursor?: string
}

function isoOrUndefined(ms: number | null | undefined): string | undefined {
  if (ms === null || ms === undefined || !Number.isFinite(ms)) return undefined
  return new Date(ms).toISOString()
}

/** Builds the Wails/JSON argument for the Go `monitor.CursorFilter` struct. */
function toGoCursorFilter(query: MonitorCursorQuery): Record<string, unknown> {
  const filter: Record<string, unknown> = {}
  if (query.nodeIdentityKey) filter.node_identity_key = query.nodeIdentityKey
  if (query.legacyNodeKey) filter.legacy_node_key = query.legacyNodeKey
  if (query.nodeKey) filter.node_key = query.nodeKey
  if (query.profileId) filter.profile_id = query.profileId
  if (query.probeType) filter.probe_type = query.probeType
  if (query.target) filter.target = query.target
  if (query.samplingTier) filter.sampling_tier = query.samplingTier
  if (query.regularObservationOnly) filter.regular_observation_only = true
  if (query.limit !== undefined) filter.limit = query.limit
  filter.order_desc = query.orderDesc !== false
  if (query.cursor) filter.cursor = query.cursor

  const since = isoOrUndefined(query.sinceMs)
  if (since) filter.since = since
  const until = isoOrUndefined(query.untilMs)
  if (until) filter.until = until

  return filter
}

/** Builds the HTTP query string for the cursor endpoint. */
export function buildCursorQueryString(query: MonitorCursorQuery): string {
  const params = new URLSearchParams()
  if (query.nodeIdentityKey) params.set('node_identity_key', query.nodeIdentityKey)
  if (query.legacyNodeKey) params.set('legacy_node_key', query.legacyNodeKey)
  if (query.nodeKey) params.set('node_key', query.nodeKey)
  if (query.profileId) params.set('profile_id', query.profileId)
  if (query.probeType) params.set('probe_type', query.probeType)
  if (query.target) params.set('target', query.target)
  if (query.samplingTier) params.set('sampling_tier', query.samplingTier)
  if (query.regularObservationOnly) params.set('regular_observation_only', 'true')
  if (query.limit !== undefined) params.set('limit', String(query.limit))
  params.set('order_desc', query.orderDesc === false ? 'false' : 'true')
  if (query.cursor) params.set('cursor', query.cursor)

  const since = isoOrUndefined(query.sinceMs)
  if (since) params.set('since', since)
  const until = isoOrUndefined(query.untilMs)
  if (until) params.set('until', until)

  return params.toString()
}

function buildStatsQueryString(query: MonitorQueryFilter): string {
  const params = new URLSearchParams()
  if (query.nodeIdentityKey) params.set('node_identity_key', query.nodeIdentityKey)
  if (query.legacyNodeKey) params.set('legacy_node_key', query.legacyNodeKey)
  if (query.nodeKey) params.set('node_key', query.nodeKey)
  if (query.profileId) params.set('profile_id', query.profileId)
  if (query.probeType) params.set('probe_type', query.probeType)
  if (query.target) params.set('target', query.target)
  if (query.samplingTier) params.set('sampling_tier', query.samplingTier)
  if (query.regularObservationOnly) params.set('regular_observation_only', 'true')

  const since = isoOrUndefined(query.sinceMs)
  if (since) params.set('since', since)
  const until = isoOrUndefined(query.untilMs)
  if (until) params.set('until', until)

  return params.toString()
}

function toGoStatsQuery(query: MonitorQueryFilter): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  if (query.nodeIdentityKey) out.node_identity_key = query.nodeIdentityKey
  if (query.legacyNodeKey) out.legacy_node_key = query.legacyNodeKey
  if (query.nodeKey) out.node_key = query.nodeKey
  if (query.profileId) out.profile_id = query.profileId
  if (query.probeType) out.probe_type = query.probeType
  if (query.target) out.target = query.target
  if (query.samplingTier) out.sampling_tier = query.samplingTier
  if (query.regularObservationOnly) out.regular_observation_only = true

  const since = isoOrUndefined(query.sinceMs)
  if (since) out.since = since
  const until = isoOrUndefined(query.untilMs)
  if (until) out.until = until

  return out
}

async function readError(res: Response): Promise<Error> {
  let message = `${res.status} ${res.statusText}`
  try {
    const text = await res.text()
    if (text) {
      try {
        const parsed = JSON.parse(text)
        if (parsed && typeof parsed.error === 'string') message = parsed.error
        else message = text
      } catch {
        message = text
      }
    }
  } catch {
    // Keep the status line as the message.
  }
  return new Error(message)
}

export function normalizeMonitorNodeOption(wire: RawMonitorNodeOptionWire): MonitorNodeOption {
	return {
		profileId: wire.profile_id ?? '',
		profileName: wire.profile_name ?? '',
		nodeKey: wire.node_key ?? '',
		nodeIdentityKey: wire.node_identity_key ?? '',
		configRevisionKey: wire.config_revision_key ?? '',
		displayName: wire.display_name ?? '',
		type: wire.type ?? '',
		countryCode: wire.country_code ?? '',
		countryFlag: wire.country_flag ?? '',
	}
}

function normalizeMonitorJobNode(wire: RawMonitorJobNodeWire): MonitorJobNode {
	return {
		nodeKey: wire.node_key ?? '',
		nodeIdentityKey: wire.node_identity_key ?? '',
		configRevisionKey: wire.config_revision_key ?? '',
		displayName: wire.display_name ?? '',
		type: wire.type ?? '',
	}
}

export function normalizeMonitorJob(wire: RawMonitorJobWire): MonitorJob {
	return {
		id: wire.id ?? '',
		name: wire.name ?? '',
		profileId: wire.profile_id ?? '',
		profileName: wire.profile_name ?? '',
		nodeKeys: Array.isArray(wire.node_keys) ? wire.node_keys : [],
		nodes: (Array.isArray(wire.nodes) ? wire.nodes : []).map(normalizeMonitorJobNode),
		probeSet: wire.probe_set,
		samplingTier: wire.sampling_tier ?? 'regular',
		intervalSeconds: Number.isFinite(wire.interval_seconds) ? wire.interval_seconds : 0,
		timeoutSeconds: Number.isFinite(wire.timeout_seconds) ? wire.timeout_seconds : 0,
		state: wire.state,
		blockedReason: wire.blocked_reason ?? '',
		persistenceState: wire.persistence_state === 'degraded' ? 'degraded' : 'healthy',
		persistenceError: wire.persistence_error ?? '',
		storageState: wire.storage_state === 'storage_protected' ? 'storage_protected' : 'ok',
		storageReason: wire.storage_reason ?? '',
		budgetState: wire.budget_state ?? '',
		budgetReason: wire.budget_reason ?? '',
		skippedRounds: wire.skipped_rounds ?? 0,
		resourceSkippedRounds: wire.resource_skipped_rounds ?? 0,
		createdAt: wire.created_at ?? '',
		updatedAt: wire.updated_at ?? '',
	}
}

export function normalizeMonitorRun(wire: RawMonitorRunWire): MonitorRun {
	return {
		runId: wire.run_id ?? '',
		jobId: wire.job_id ?? '',
		samplingTier: wire.sampling_tier ?? 'legacy_unknown',
		triggerType: wire.trigger_type ?? 'legacy_unknown',
		samplingStrategyVersion: wire.sampling_strategy_version ?? 0,
		scheduledAt: wire.scheduled_at ?? '',
		startedAt: wire.started_at ?? '',
		finishedAt: wire.finished_at,
		status: wire.status,
		totalNodes: wire.total_nodes ?? 0,
		successNodes: wire.success_nodes ?? 0,
		failedNodes: wire.failed_nodes ?? 0,
		errorMessage: wire.error_message,
	}
}

/** Reads the safe node choices resolved from the current subscription caches. */
export async function fetchMonitorNodeOptions(): Promise<MonitorNodeOption[]> {
	if (isWails()) {
		const raw = await window.go!.desktop!.App!.ListMonitorNodeOptions()
		return (Array.isArray(raw) ? raw : []).map((item) => normalizeMonitorNodeOption(item as RawMonitorNodeOptionWire))
	}

	const res = await fetch(`${API_BASE}/api/monitor/nodes`)
	if (!res.ok) throw await readError(res)
	const raw = (await res.json()) as RawMonitorNodeOptionWire[]
	return (Array.isArray(raw) ? raw : []).map(normalizeMonitorNodeOption)
}

/** Lists persisted monitor definitions and current runtime status without exposing credentials. */
export async function fetchMonitorJobs(): Promise<MonitorJob[]> {
	if (isWails()) {
		const raw = await window.go!.desktop!.App!.ListMonitorJobs()
		return (Array.isArray(raw) ? raw : []).map((item) => normalizeMonitorJob(item as RawMonitorJobWire))
	}

	const res = await fetch(`${API_BASE}/api/monitor/jobs`)
	if (!res.ok) throw await readError(res)
	const raw = (await res.json()) as RawMonitorJobWire[]
	return (Array.isArray(raw) ? raw : []).map(normalizeMonitorJob)
}

/** Creates a stopped job. The UI must issue an explicit start action afterwards. */
export async function createMonitorJob(req: MonitorJobCreateRequest): Promise<MonitorJob> {
	if (isWails()) {
		const raw = await window.go!.desktop!.App!.CreateMonitorJob(req)
		return normalizeMonitorJob(raw as RawMonitorJobWire)
	}

	const res = await fetch(`${API_BASE}/api/monitor/jobs`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(req),
	})
	if (!res.ok) throw await readError(res)
	return normalizeMonitorJob((await res.json()) as RawMonitorJobWire)
}

/** Updates only a job's periodic sampling tier; lifecycle state is unchanged. */
export async function updateMonitorJobSamplingTier(jobId: string, tier: MonitorSamplingTier): Promise<void> {
	if (isWails()) {
		await window.go!.desktop!.App!.UpdateMonitorJobSamplingTier(jobId, tier)
		return
	}

	const res = await fetch(`${API_BASE}/api/monitor/jobs/${encodeURIComponent(jobId)}/sampling-tier`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ sampling_tier: tier }),
	})
	if (!res.ok) throw await readError(res)
}

export async function fetchMonitorStorageUsage(): Promise<MonitorStorageUsage> {
	if (isWails()) return window.go!.desktop!.App!.GetMonitorStorageUsage()
	const res = await fetch(`${API_BASE}/api/monitor/storage`)
	if (!res.ok) throw await readError(res)
	return res.json()
}

export async function fetchMonitorBudgetStatus(): Promise<MonitorBudgetStatus> {
	if (isWails()) return window.go!.desktop!.App!.GetMonitorBudgetStatus()
	const res = await fetch(`${API_BASE}/api/monitor/budget`)
	if (!res.ok) throw await readError(res)
	return res.json()
}

export async function previewMonitorRetention(req: MonitorRetentionRequest): Promise<MonitorRetentionPreview> {
	if (isWails()) return window.go!.desktop!.App!.PreviewMonitorRetention(req)
	const res = await fetch(`${API_BASE}/api/monitor/retention/preview`, {
		method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(req),
	})
	if (!res.ok) throw await readError(res)
	return res.json()
}

export async function applyMonitorRetention(req: MonitorRetentionRequest): Promise<MonitorRetentionResult> {
	if (isWails()) return window.go!.desktop!.App!.ApplyRetention(req)
	const res = await fetch(`${API_BASE}/api/monitor/retention`, {
		method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(req),
	})
	if (!res.ok) {
		const body = await res.json().catch(() => null) as { error?: string; partial?: boolean; samples_deleted?: number; runs_deleted?: number } | null
		if (body?.partial) throw new Error(`部分删除：已删除 ${body.samples_deleted ?? 0} 条样本、${body.runs_deleted ?? 0} 条 run；${body.error ?? '执行中断'}`)
		throw new Error(body?.error || `retention failed: ${res.status}`)
	}
	return res.json()
}

export async function fetchMonitorRuns(jobId: string, limit = 5): Promise<MonitorRun[]> {
	if (isWails()) {
		const raw = await window.go!.desktop!.App!.QueryMonitorRuns(jobId, limit)
		return (Array.isArray(raw) ? raw : []).map((item) => normalizeMonitorRun(item as RawMonitorRunWire))
	}

	const params = new URLSearchParams({ job_id: jobId, limit: String(limit) })
	const res = await fetch(`${API_BASE}/api/monitor/runs?${params.toString()}`)
	if (!res.ok) throw await readError(res)
	const raw = (await res.json()) as RawMonitorRunWire[]
	return (Array.isArray(raw) ? raw : []).map(normalizeMonitorRun)
}

export type MonitorJobAction = 'start' | 'pause' | 'resume' | 'stop'

/** Applies one idempotent scheduler lifecycle action, then the caller re-reads actual state. */
export async function controlMonitorJob(jobId: string, action: MonitorJobAction): Promise<void> {
	if (isWails()) {
		const app = window.go!.desktop!.App!
		if (action === 'start') return app.StartMonitorJob(jobId)
		if (action === 'pause') return app.PauseMonitorJob(jobId)
		if (action === 'resume') return app.ResumeMonitorJob(jobId)
		return app.StopMonitorJob(jobId)
	}

	const res = await fetch(`${API_BASE}/api/monitor/jobs/${encodeURIComponent(jobId)}/${action}`, {
		method: 'POST',
	})
	if (!res.ok) throw await readError(res)
}

/** Runs one explicit diagnostic round without changing the periodic scheduler state. */
export async function triggerMonitorJob(jobId: string): Promise<MonitorRun> {
	if (isWails()) {
		const raw = await window.go!.desktop!.App!.TriggerMonitorJob(jobId)
		return normalizeMonitorRun(raw as RawMonitorRunWire)
	}
	const res = await fetch(`${API_BASE}/api/monitor/jobs/${encodeURIComponent(jobId)}/trigger`, { method: 'POST' })
	if (!res.ok) throw await readError(res)
	return normalizeMonitorRun((await res.json()) as RawMonitorRunWire)
}

/** Reads one page of raw samples via the single-direction keyset cursor. */
export async function queryMonitorSamplesCursor(
  query: MonitorCursorQuery
): Promise<SampleCursorPage> {
  if (isWails()) {
    const raw = await window.go!.desktop!.App!.QueryMonitorSamplesCursor(toGoCursorFilter(query))
    return normalizeCursorPage(raw as RawSampleCursorPageWire)
  }

  const res = await fetch(`${API_BASE}/api/monitor/samples/cursor?${buildCursorQueryString(query)}`)
  if (!res.ok) throw await readError(res)
  return normalizeCursorPage((await res.json()) as RawSampleCursorPageWire)
}

/** Derives observation-window statistics over raw samples. */
export async function fetchMonitorStats(query: MonitorQueryFilter): Promise<DerivedStats> {
  if (isWails()) {
    const raw = await window.go!.desktop!.App!.GetMonitorStats(toGoStatsQuery(query))
    return normalizeDerivedStats(raw as RawDerivedStatsWire)
  }

  const res = await fetch(`${API_BASE}/api/monitor/stats?${buildStatsQueryString(query)}`)
  if (!res.ok) throw await readError(res)
  return normalizeDerivedStats((await res.json()) as RawDerivedStatsWire)
}

/** Lists the filter dimensions that actually exist in raw samples. */
export async function fetchMonitorFacets(
  sinceMs?: number | null,
  untilMs?: number | null
): Promise<MonitorSampleFacets> {
  const since = isoOrUndefined(sinceMs)
  const until = isoOrUndefined(untilMs)

  if (isWails()) {
    const raw = await window.go!.desktop!.App!.GetMonitorSampleFacets(since ?? null, until ?? null)
    return normalizeFacets(raw as RawMonitorSampleFacetsWire)
  }

  const params = new URLSearchParams()
  if (since) params.set('since', since)
  if (until) params.set('until', until)

  const res = await fetch(`${API_BASE}/api/monitor/facets?${params.toString()}`)
  if (!res.ok) throw await readError(res)
  return normalizeFacets((await res.json()) as RawMonitorSampleFacetsWire)
}
