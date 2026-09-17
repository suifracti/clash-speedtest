export interface Airport {
  id: string
  name: string
  url: string
  updated_at?: string
  node_count: number
  has_cache: boolean
}

export interface NodeItem {
  name: string
  type: string
  country_code: string
  country_flag: string
}

export interface LatencySample {
  seq: number
  timestamp: string
  latency_ms: number
  success: boolean
  error?: string
}

export interface Ping0Info {
  ip: string
  location: string
  country: string
  asn: string
  org: string
  is_idc: boolean
  risk_score: number
}

export interface IPPureInfo {
  ip: string
  asn: number
  as_org: string
  country: string
  country_code: string
  city: string
  fraud_score: number
  is_residential: boolean
  is_broadcast: boolean
}

export interface IPInfo {
  ip: string
  asn: string
  isp: string
  ip_type: string
  origin_type: string
  risk_score: number
  fraud_score: number
  location: string
  country: string
  country_code: string
  ping0?: Ping0Info
  ippure?: IPPureInfo
}

export interface StabilityInfo {
  total_probes: number
  success_probes: number
  stability_rate: number
  flapping: boolean
  exit_ips: string[]
  blocked_ips?: string[]
  flap_reason?: string
  google_ttfb_ms: number
  google_min_ttfb_ms: number
  google_max_ttfb_ms: number
  latency_grade?: string
}

export interface NodeResult {
  proxy_name: string
  proxy_type: string
  server?: string
  port?: number
  country_code: string
  country_flag: string
  latency_ms: number
  jitter_ms: number
  packet_loss: number
  latency_samples?: LatencySample[]
  timeout_count?: number
  download_speed_mbps: number
  upload_speed_mbps: number
  download_error?: string
  upload_error?: string
  antigravity_status: string
  antigravity_detail: string
  google_ttfb_ms: number
  exit_country: string
  exit_country_code: string
  ip_info?: IPInfo
  stability?: StabilityInfo
}

export type TriageCategory =
  | 'stable'
  | 'single_pass'
  | 'unverified'
  | 'flapping'
  | 'blocked'
  | 'failed'

export interface AuditEvidence {
  category: TriageCategory
  badgeLabel: string
  badgeClass: string
  reason: string
  historyTrail: string
  roundsPassed: number
  roundsTotal: number
  requiresRetest: boolean
  isGeoBlocked: boolean
  isConfirmedFlapping: boolean
}

export interface TestConfig {
  metrics: string[]
  concurrent: number
  timeout_sec: number
  download_size: number
  upload_size: number
  server_url: string
  rounds: number
}

export interface BatchTestRequest {
  airport_id: string
  node_names: string[]
  config: TestConfig
}

export interface SingleTestRequest {
  airport_id: string
  node_name: string
  config: TestConfig
}

export interface TestStatus {
  is_running: boolean
  current_node?: string
  current_step?: string
  current_index: number
  total_nodes: number
  percent: number
  airport_name?: string
  started_at?: string
}

export interface TokenStatus {
  has_token: boolean
  source: string
  preview: string
}

export interface RunSummary {
  id: string
  airport_id: string
  airport_name: string
  created_at: string
  total_nodes: number
  passed_nodes: number
  metrics: string[]
}

export interface TestRun {
  id: string
  airport_id: string
  airport_name: string
  created_at: string
  metrics: string[]
  total_nodes: number
  passed_nodes: number
  results: NodeResult[]
}

export interface AirportTimelineMoment {
  run_id: string
  created_at: string
  total_nodes: number
  passed_nodes: number
}

export interface AirportNodePoint {
  run_id: string
  created_at: string
  latency_ms: number
  jitter_ms: number
  packet_loss: number
  download_speed_mbps: number
  upload_speed_mbps: number
  antigravity_status: string
  antigravity_detail: string
  google_ttfb_ms?: number
  exit_country: string
  exit_country_code: string
  ip_info?: IPInfo
  stability?: StabilityInfo
}

export interface AirportNodeHistory {
  proxy_name: string
  country_code: string
  country_flag: string
  proxy_type: string
  server?: string
  port?: number
  points: AirportNodePoint[]
  latest: AirportNodePoint
  earliest: AirportNodePoint
  min_latency_ms: number
  max_latency_ms: number
  avg_latency_ms: number
  latency_change_ms: number
  latency_change_pct: number
  avg_google_ttfb_ms?: number
  latest_google_ttfb_ms?: number
  max_speed_mbps: number
  avg_speed_mbps: number
  latest_speed_mbps: number
  speed_change_mbps: number
  available_count: number
  total_tests: number
  available_rate: number
  unique_ips: string[]
}

export interface AirportHistory {
  airport_id: string
  airport_name: string
  total_runs: number
  earliest_time: string
  latest_time: string
  distinct_node_count: number
  moments: AirportTimelineMoment[]
  nodes: AirportNodeHistory[]
  runs: RunSummary[]
}

export interface NodeDiff {
  proxy_name: string
  country_code: string
  country_flag: string
  base_latency_ms: number
  target_latency_ms: number
  latency_delta_ms: number
  base_speed_mbps: number
  target_speed_mbps: number
  speed_delta_mbps: number
  base_antigravity: string
  target_antigravity: string
  antigravity_changed: boolean
  base_ip?: string
  target_ip?: string
  base_risk_score?: number
  target_risk_score?: number
  base_fraud_score?: number
  target_fraud_score?: number
  base_origin_type?: string
  target_origin_type?: string
  base_flapping?: boolean
  target_flapping?: boolean
  status: 'improved' | 'degraded' | 'unchanged' | 'new' | 'removed'
}

export interface ComparisonSummary {
  total_compared: number
  improved_count: number
  degraded_count: number
  unchanged_count: number
  new_count: number
  removed_count: number
}

export interface RunComparison {
  base_run_id: string
  target_run_id: string
  base_time: string
  target_time: string
  node_diffs: NodeDiff[]
  summary: ComparisonSummary
}

export interface AppSettings {
  preferred_browser: string
}

export interface ControllerConfig {
  endpoint: string
  secret?: string
  mode: 'external' | 'standalone'
  allow_remote?: boolean
  allow_insecure_plaintext_remote?: boolean
}

export interface ControllerGroup {
  name: string
  type: string
  now: string
  all: string[]
}

export type OrchestratorMode = 'monitor_only' | 'recommend' | 'auto'
export type PolicyPurpose = 'general' | 'ai'

export interface ControllerStatus {
  connected: boolean
  endpoint: string
  core_version?: string
  core_type?: string
  current_group?: string
  current_node?: string
  locked_node?: string
  mode: OrchestratorMode
  has_secret: boolean
  available_groups?: string[]
}

export interface SwitchPolicy {
  purpose?: PolicyPurpose
  mode: OrchestratorMode
  target_group: string
  target_group_type?: string
  candidate_nodes?: string[]
  interval: number
  max_consecutive_failures: number
  min_improvement_rtt: number
  min_improvement_ratio: number
  cooldown_duration: number
  hysteresis_buffer: number
  locked_node?: string
  rollback_on_failure: boolean
  max_sample_age: number
  min_sample_count: number
  min_observation_window: number
  verification_grace_period: number
  verification_probe_count: number
  verification_failure_threshold: number
}

export interface SwitchEvent {
  id: string
  timestamp: string
  target_group: string
  from_node: string
  to_node: string
  reason: string
  trigger_type: 'failure_failover' | 'latency_improvement' | 'manual_override' | 'rollback'
  status: 'success' | 'rolled_back' | 'failed'
  old_metric?: number
  new_metric?: number
}

// --- 24/7 Monitor: raw sample history ---

/**
 * Raw wire shape of one immutable monitor sample.
 *
 * IMPORTANT: the Go backend serializes `latency` and `ttfb` as `time.Duration`, i.e. plain
 * integer NANOSECONDS. Never read these fields directly in UI code — go through
 * `normalizeMonitorSample()` so the rest of the app only ever sees milliseconds.
 */
export interface RawMonitorSampleWire {
  sample_id: string
  run_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  profile_id: string
  display_name_snapshot: string
  probe_type: string
  target: string
  timestamp: string
  success: boolean
  latency: number
  ttfb: number
  error_class: string
  error_detail?: string
  exit_ip?: string
  exit_region?: string
  metadata?: Record<string, unknown>
}

/** Normalized raw sample: durations converted to milliseconds, timestamp to epoch ms. */
export interface MonitorSample {
  sampleId: string
  runId: string
  nodeKey: string
  nodeIdentityKey: string
  configRevisionKey: string
  profileId: string
  displayNameSnapshot: string
  probeType: string
  target: string
  /** Epoch milliseconds. */
  timestampMs: number
  /** ISO 8601 as returned by the backend, kept verbatim for evidence display. */
  timestampIso: string
  success: boolean
  /** Milliseconds. */
  latencyMs: number
  /** Milliseconds. */
  ttfbMs: number
  errorClass: string
  errorDetail?: string
  exitIp?: string
  exitRegion?: string
  metadata?: Record<string, unknown>
}

export interface RawSampleCursorPageWire {
  items: RawMonitorSampleWire[] | null
  next_cursor?: string
  has_more: boolean
  limit: number
}

export interface SampleCursorPage {
  items: MonitorSample[]
  nextCursor: string
  hasMore: boolean
  limit: number
}

export interface RawDerivedStatsWire {
  sample_count: number
  success_count: number
  failure_count: number
  success_rate: number
  latency_min_ms: number | null
  latency_p50_ms: number | null
  latency_p95_ms: number | null
  latency_max_ms: number | null
  ttfb_p50_ms: number | null
  ttfb_p95_ms: number | null
  error_breakdown: Record<string, number> | null
  first_sample_at: string | null
  last_sample_at: string | null
  observed_since: string | null
  observed_until: string | null
  node_identity_key?: string
  node_key?: string
  probe_type?: string
}

export interface DerivedStats {
  sampleCount: number
  successCount: number
  failureCount: number
  successRate: number
  latencyMinMs: number | null
  latencyP50Ms: number | null
  latencyP95Ms: number | null
  latencyMaxMs: number | null
  ttfbP50Ms: number | null
  ttfbP95Ms: number | null
  errorBreakdown: Record<string, number>
  firstSampleAtMs: number | null
  lastSampleAtMs: number | null
}

export interface RawFacetNodeWire {
  node_identity_key: string
  node_key: string
  display_name: string
  profile_id: string
  sample_count: number
}

export interface RawMonitorSampleFacetsWire {
  nodes: RawFacetNodeWire[] | null
  profiles: string[] | null
  probe_types: string[] | null
  targets: string[] | null
  window_since: string
  window_until: string
  truncated: boolean
}

export interface FacetNode {
  nodeIdentityKey: string
  nodeKey: string
  displayName: string
  profileId: string
  sampleCount: number
}

export interface MonitorSampleFacets {
  nodes: FacetNode[]
  profiles: string[]
  probeTypes: string[]
  targets: string[]
  windowSinceMs: number
  windowUntilMs: number
  truncated: boolean
}

