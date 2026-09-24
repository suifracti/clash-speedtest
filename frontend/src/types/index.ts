export interface Airport {
  id: string
  name: string
  url_display: string
  url_configured: boolean
  updated_at?: string
  node_count: number
  has_cache: boolean
}

export interface ProfileSource {
  path: string
  label: string
  available: boolean
  profile_count: number
  cache_count: number
  missing?: string[]
  possible_test_data: boolean
  error?: string
}

export interface DataMigration {
  state: 'ready' | 'pending' | 'conflict' | 'invalid' | 'isolated'
  source_history_dir?: string
  target_history_dir: string
  source_settings_file?: string
  target_settings_file: string
  source_has_sqlite: boolean
  source_json_count: number
  source_has_settings: boolean
  target_has_sqlite: boolean
  target_json_count: number
  target_has_settings: boolean
  error?: string
}

export interface ProfileSetup {
  state: 'ready' | 'needs_choice' | 'error'
  initialized: boolean
  data_root: string
  profile_dir: string
  history_dir: string
  settings_file: string
  unfinished_staging?: string[]
  lock_present: boolean
  error?: string
  sources?: ProfileSource[]
  migration: DataMigration
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

export interface WorkbenchLatencyTestRequest {
  profile_id: string
  node_key: string
  test_project: 'latency_stability'
  timeout_seconds: number
}

export interface WorkbenchLatencyBatchSelection {
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  display_name: string
  node_type: string
}

export interface WorkbenchLatencyBatchRequest {
  request_id: string
  test_project: 'latency_stability'
  timeout_seconds: number
  selections: WorkbenchLatencyBatchSelection[]
}

export type WorkbenchLatencyBatchExecutionState = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'skipped_config' | 'not_executed' | 'interrupted'
export type WorkbenchLatencyBatchPersistenceState = 'pending' | 'saving' | 'saved' | 'failed' | 'not_applicable'

export interface WorkbenchLatencyBatchItem {
  item_id: string
  batch_id: string
  ordinal: number
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  display_name: string
  node_type: string
  execution_state: WorkbenchLatencyBatchExecutionState
  persistence_state: WorkbenchLatencyBatchPersistenceState
  attempt_id?: string
  requested_at: string
  started_at?: string
  finished_at?: string
  error_message?: string
  persistence_error?: string
  result?: WorkbenchLatencyTest
}

export interface WorkbenchLatencyBatch {
  batch_id: string
  request_id: string
  test_project: 'latency_stability'
  timeout_seconds: number
  requested_at: string
  state: string
  item_count: number
  items?: WorkbenchLatencyBatchItem[]
}

export interface WorkbenchLatencyHistoryQuery {
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  since: string
  until: string
  limit?: number
  before_finished_at?: string
  before_attempt_id?: string
}

export interface WorkbenchLatencyHistoryDetailQuery {
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  attempt_id: string
  since?: string
  until?: string
}

export interface WorkbenchLatencyHistoryResult {
  tests: WorkbenchLatencyTest[]
  since: string
  until: string
  as_of: string
  has_more: boolean
  complete: boolean
}

export interface NodeHistoryRevision {
  config_revision_key: string
  node_key: string
  display_name: string
  last_observed_at: string
}

export type NodeDetailOrigin =
  | { kind: 'monitor_sample'; sampleId: string; observedAt?: string; samplingTier?: MonitorRunSamplingTier }
  | { kind: 'workbench_attempt'; attemptId: string; observedAt?: string; snapshot?: WorkbenchLatencyTest }
  | { kind: 'workbench_batch_item'; batchId: string; itemId: string; attemptId?: string; observedAt?: string; snapshot?: WorkbenchLatencyTest }
  | { kind: 'public_service_attempt'; attemptId: string; serviceId: string; observedAt?: string; snapshot?: WorkbenchPublicServiceAttempt }
  | { kind: 'workbench_download_attempt'; attemptId: string; observedAt?: string; snapshot?: WorkbenchDownloadAttempt }

export interface NodeDetailRequest {
  profileId: string
  profileName?: string
  nodeKey: string
  nodeIdentityKey: string
  configRevisionKey: string
  displayName: string
  nodeType?: string
  origin?: NodeDetailOrigin
}

export interface WorkbenchLatencySample {
  seq: number
  timestamp: string
  latency_ms: number
  success: boolean
  error?: string
}

export type WorkbenchLatencyStatus = 'completed' | 'partial_failed' | 'failed'

export interface WorkbenchLatencyTest {
  attempt_id: string
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  display_name: string
  node_type: string
  test_project: 'latency_stability'
  source?: string
  method?: string
  method_version?: number
  target?: string
  unit?: string
  requested_at: string
  started_at: string
  finished_at: string
  status: WorkbenchLatencyStatus
  latency_ms: number
  jitter_ms: number
  packet_loss: number
  total_samples: number
  success_samples: number
  failure_samples: number
  error_message?: string
  samples: WorkbenchLatencySample[]
  persistence_state: 'saving' | 'saved' | 'failed'
  persistence_error?: string
}

export interface WorkbenchPublicServiceRule {
  service_id: string
  name: string
  rule_version: number
  target_url: string
  method: string
  success_criterion: string
  redirect_policy: string
  timeout_seconds: number
  maximum_body_bytes: number
  accept?: string
  api_version_header?: string
}

export interface WorkbenchPublicServiceTestRequest {
  request_id: string
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  service_id: string
  timeout_seconds?: number
}

export interface WorkbenchPublicServiceMeasurement {
  outcome: string
  http_status?: number
  bytes_read: number
  started_at: string
  finished_at: string
  duration_ms: number
  failure_phase?: string
  error_message?: string
}

export interface WorkbenchPublicServiceAttempt {
  attempt_id: string
  request_id: string
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  display_name: string
  node_type: string
  source: string
  service_id: string
  rule: WorkbenchPublicServiceRule
  requested_at: string
  started_at?: string
  finished_at?: string
  execution_state: string
  persistence_state: string
  persistence_error?: string
  result?: WorkbenchPublicServiceMeasurement
}

export interface WorkbenchPublicServiceHistoryQuery {
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  service_id?: string
  since?: string
  until?: string
  limit?: number
  before_at?: string
  before_attempt_id?: string
}

export interface WorkbenchSaveRetryRequest {
  domain: 'public_service' | 'download'
  attempt_id: string
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  service_id?: string
}

export interface WorkbenchPublicServiceHistoryResult {
  attempts: WorkbenchPublicServiceAttempt[]
  since?: string
  until?: string
  has_more: boolean
  complete: boolean
}

export interface WorkbenchDownloadTestRequest {
  request_id: string
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  maximum_bytes?: number
  timeout_seconds?: number
}

export interface WorkbenchDownloadRule {
  rule_version: number
  target_url: string
  method: string
  maximum_bytes: number
  maximum_duration_ns: number
  sample_every_bytes: number
  sample_every_ns: number
}

export interface WorkbenchDownloadSample {
  elapsed_ns: number
  interval_ns: number
  delta_bytes: number
  cumulative_bytes: number
  speed_mbps?: number
}

export interface WorkbenchDownloadMeasurement {
  outcome: string
  http_status?: number
  bytes_read: number
  started_at: string
  finished_at: string
  duration_ns: number
  failure_phase?: string
  error_message?: string
  samples: WorkbenchDownloadSample[]
}

export interface WorkbenchDownloadAttempt {
  attempt_id: string
  request_id: string
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  display_name: string
  node_type: string
  source: string
  requested_at: string
  started_at?: string
  finished_at?: string
  execution_state: string
  persistence_state: string
  persistence_error?: string
  rule: WorkbenchDownloadRule
  result?: WorkbenchDownloadMeasurement
}

export interface WorkbenchDownloadHistoryQuery {
  profile_id: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  since?: string
  until?: string
  limit?: number
  before_at?: string
  before_attempt_id?: string
}

export interface WorkbenchDownloadHistoryResult {
  attempts: WorkbenchDownloadAttempt[]
  since?: string
  until?: string
  has_more: boolean
  complete: boolean
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
  monitor_retention_policy?: MonitorRetentionPolicy
  monitor_retention_custom_days?: number
  monitor_storage_warning_bytes?: number
  monitor_storage_hard_bytes?: number
  monitor_budget_max_concurrent?: number
  monitor_budget_daily_requests?: number
  monitor_budget_daily_bytes?: number
  monitor_budget_response_bytes?: number
}

export interface MonitorBudgetStatus {
  limits: { max_concurrent: number; daily_requests: number; daily_bytes: number; response_bytes: number }
  usage: { utc_day: string; requests_used: number; bytes_used: number }
  reset_at: string
  active_requests: number
  blocked_code?: string
  blocked_reason?: string
}

export type MonitorRetentionPolicy = 'keep_all' | '30d' | '90d' | '180d' | 'custom'

export interface MonitorRetentionRequest {
  policy: MonitorRetentionPolicy
  custom_days?: number
  cutoff_time?: string
}

export interface MonitorStorageUsage {
  database_bytes: number
  wal_bytes: number
  shared_memory_bytes: number
  total_bytes: number
  warning_bytes: number
  hard_bytes: number
  warning: boolean
  protected: boolean
}

export interface MonitorRetentionPreview {
  policy: MonitorRetentionPolicy
  cutoff: string
  samples_to_delete: number
  runs_to_delete: number
  storage: MonitorStorageUsage
}

export interface MonitorRetentionResult {
  policy: MonitorRetentionPolicy
  cutoff: string
  samples_deleted: number
  runs_deleted: number
  duration_ms: number
  partial: boolean
  error_message?: string
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
  sampling_tier?: MonitorRunSamplingTier
  trigger_type?: MonitorSamplingTrigger
  sampling_strategy_version?: number
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
  samplingTier: MonitorRunSamplingTier
  triggerType: MonitorSamplingTrigger
  samplingStrategyVersion: number
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
  included_sampling_tiers?: MonitorRunSamplingTier[] | null
  regular_observation_only?: boolean
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
  includedSamplingTiers: MonitorRunSamplingTier[]
  regularObservationOnly: boolean
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

// --- 24/7 Monitor: job management ---

/** Credential-free node choice resolved from an actual cached subscription. */
export interface RawMonitorNodeOptionWire {
  profile_id: string
  profile_name: string
  node_key: string
  node_identity_key: string
  config_revision_key: string
  display_name: string
  type: string
  country_code: string
  country_flag: string
}

export interface MonitorNodeOption {
  profileId: string
  profileName: string
  nodeKey: string
  nodeIdentityKey: string
  configRevisionKey: string
  displayName: string
  type: string
  countryCode: string
  countryFlag: string
}

/** Stable node context carried from Workbench into an explicit Monitor create. */
export interface MonitorNodeSelectionContext {
  node_key: string
  node_identity_key: string
  config_revision_key: string
}

export interface MonitorJobPrefill {
  profileId: string
  nodeKeys: string[]
  nodeContexts: MonitorNodeSelectionContext[]
}

export interface MonitorJobCreateRequest {
  name: string
  profile_id: string
  node_keys: string[]
  probe_set: 'light' | 'service' | 'heavy'
  sampling_tier: MonitorSamplingTier
  /** Seconds on the UI/API boundary; never nanoseconds. */
  interval_seconds: number
  /** Seconds on the UI/API boundary; never nanoseconds. */
  timeout_seconds: number
  /** Optional Workbench snapshot checked against the current canonical cache. */
  node_contexts?: MonitorNodeSelectionContext[]
}

export interface MonitorJobNode {
  nodeKey: string
  nodeIdentityKey: string
  configRevisionKey: string
  displayName: string
  type: string
}

export interface RawMonitorJobNodeWire {
  node_key: string
  node_identity_key: string
  config_revision_key: string
  display_name: string
  type: string
}

export interface RawMonitorJobWire {
  id: string
  name: string
  profile_id: string
  profile_name: string
  node_keys: string[]
  nodes: RawMonitorJobNodeWire[] | null
  probe_set: 'light' | 'service' | 'heavy'
  sampling_tier?: MonitorSamplingTier
  interval_seconds: number
  timeout_seconds: number
  state: MonitorJobState
  runtime_state?: MonitorJobState
  resume_on_launch?: boolean
  desired_state?: MonitorJobState
  recovery_state?: MonitorRecoveryState
  recovery_reason?: string
  intent_persistence_error?: string
  blocked_reason?: string
  persistence_state?: 'healthy' | 'degraded'
  persistence_error?: string
  storage_state?: 'ok' | 'storage_protected'
  storage_reason?: string
  budget_state?: string
  budget_reason?: string
  skipped_rounds?: number
  resource_skipped_rounds?: number
  created_at: string
  updated_at: string
}

export type MonitorJobState = 'stopped' | 'running' | 'paused' | 'blocked'
export type MonitorRecoveryState = 'disabled' | 'stopped' | 'paused' | 'active' | 'restoring' | 'restored' | 'blocked'

export interface MonitorJob {
  id: string
  name: string
  profileId: string
  profileName: string
  nodeKeys: string[]
  nodes: MonitorJobNode[]
  probeSet: 'light' | 'service' | 'heavy'
  samplingTier: MonitorSamplingTier
  intervalSeconds: number
  timeoutSeconds: number
  state: MonitorJobState
  runtimeState: MonitorJobState
  resumeOnLaunch: boolean
  desiredState: MonitorJobState
  recoveryState: MonitorRecoveryState
  recoveryReason: string
  intentPersistenceError: string
  blockedReason: string
  persistenceState?: 'healthy' | 'degraded'
  persistenceError?: string
  storageState?: 'ok' | 'storage_protected'
  storageReason?: string
  budgetState?: string
  budgetReason?: string
  skippedRounds?: number
  resourceSkippedRounds?: number
  createdAt: string
  updatedAt: string
}

export type MonitorRunStatus = 'running' | 'completed' | 'partial_failed' | 'failed' | 'skipped' | 'resource_limited' | 'interrupted' | 'persistence_failed'

export type MonitorSamplingTier = 'regular' | 'focus' | 'sparse'
export type MonitorRunSamplingTier = MonitorSamplingTier | 'diagnostic' | 'legacy_unknown'
export type MonitorSamplingTrigger = 'scheduled' | 'manual' | 'legacy_unknown'

export interface MonitorRun {
  runId: string
  jobId: string
  samplingTier: MonitorRunSamplingTier
  triggerType: MonitorSamplingTrigger
  samplingStrategyVersion: number
  scheduledAt: string
  startedAt: string
  finishedAt?: string
  status: MonitorRunStatus
  totalNodes: number
  successNodes: number
  failedNodes: number
  errorMessage?: string
}

export interface RawMonitorRunWire {
  run_id: string
  job_id: string
  sampling_tier?: MonitorRunSamplingTier
  trigger_type?: MonitorSamplingTrigger
  sampling_strategy_version?: number
  scheduled_at: string
  started_at: string
  finished_at?: string
  status: MonitorRunStatus
  total_nodes: number
  success_nodes: number
  failed_nodes: number
  error_message?: string
}

