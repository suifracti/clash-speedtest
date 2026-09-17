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
