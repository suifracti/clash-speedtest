import type {
  Airport,
  NodeItem,
  BatchTestRequest,
  SingleTestRequest,
  TestStatus,
  RunSummary,
  TestRun,
  AirportHistory,
  RunComparison,
  TokenStatus,
  AppSettings,
  NodeResult,
  ControllerConfig,
  ControllerGroup,
  ControllerStatus,
  SwitchPolicy,
  SwitchEvent,
  WorkbenchLatencyTestRequest,
  WorkbenchLatencyHistoryQuery,
  WorkbenchLatencyHistoryDetailQuery,
  NodeHistoryRevision,
  WorkbenchLatencyHistoryResult,
  WorkbenchLatencyTest,
  WorkbenchLatencyBatchRequest,
  WorkbenchLatencyBatch,
  WorkbenchPublicServiceAttempt,
  WorkbenchPublicServiceHistoryQuery,
  WorkbenchPublicServiceHistoryResult,
  WorkbenchPublicServiceRule,
  WorkbenchPublicServiceTestRequest,
  WorkbenchDownloadAttempt,
  WorkbenchDownloadHistoryQuery,
  WorkbenchDownloadHistoryResult,
  WorkbenchDownloadTestRequest,
  ProfileSetup,
  ProfileSource,
} from '../types'

declare global {
  interface Window {
    go?: {
      desktop?: {
        App?: Record<string, (...args: any[]) => Promise<any>>
      }
    }
    runtime?: {
      EventsOn: (eventName: string, callback: (payload: any) => void) => () => void
    }
  }
}

export function isWails(): boolean {
  return typeof window !== 'undefined' && !!window.go?.desktop?.App
}

const API_BASE = ''

// --- Event Subscription ---

export function subscribeEvents(onEvent: (type: string, payload: any) => void): () => void {
  if (isWails() && window.runtime?.EventsOn) {
    const events = [
      'test_started',
      'test_stopped',
      'test_completed',
      'node_progress',
      'single_test_started',
      'single_node_progress',
      'single_test_completed',
      'workbench_latency_test_completed',
      'workbench_latency_test_persistence_updated',
      'workbench_download_progress',
      'workbench_download_attempt_updated',
      'antigravity_token_updated',
      'antigravity_login_failed',
      'controller_status_changed',
      'controller_node_switched',
      'controller_switch_failed',
      'controller_policy_updated',
    ]
    const unsubs = events.map((evt) =>
      window.runtime!.EventsOn(evt, (payload) => onEvent(evt, payload))
    )
    return () => {
      unsubs.forEach((u) => u())
    }
  }

  // Fallback to HTTP SSE
  const source = new EventSource(`${API_BASE}/api/events`)
  source.onmessage = (e) => {
    try {
      const data = JSON.parse(e.data)
      if (data && data.type) {
        onEvent(data.type, data.payload)
      }
    } catch {}
  }
  return () => {
    source.close()
  }
}

// --- API Methods ---

export async function fetchProfileSetup(): Promise<ProfileSetup> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetProfileSetup()
  }
  const res = await fetch(`${API_BASE}/api/profile/setup`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function migrateLegacyData(): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.MigrateLegacyData()
  }
  const res = await fetch(`${API_BASE}/api/data/migration`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function inspectProfileSource(path: string): Promise<ProfileSource> {
  if (isWails()) {
    return window.go!.desktop!.App!.InspectProfileSource(path)
  }
  const res = await fetch(`${API_BASE}/api/profile/source/inspect`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function initializeEmptyProfileStore(): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.InitializeEmptyProfileStore()
  }
  const res = await fetch(`${API_BASE}/api/profile/setup/empty`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function importProfileSource(path: string): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.ImportProfileSource(path)
  }
  const res = await fetch(`${API_BASE}/api/profile/import`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function discardProfileImport(): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.DiscardProfileImport()
  }
  const res = await fetch(`${API_BASE}/api/profile/import/discard`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function fetchAirports(): Promise<Airport[]> {
  if (isWails()) {
    return window.go!.desktop!.App!.ListAirports()
  }
  const res = await fetch(`${API_BASE}/api/airports`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

// The complete source is read only for an explicit management action such as
// opening an existing subscription in the edit form. It is not kept in the
// ordinary airport store/list DTO.
export async function getAirportURL(id: string): Promise<string> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetAirportURL(id)
  }
  const res = await fetch(`${API_BASE}/api/airports/${id}/url`)
  if (!res.ok) throw new Error(await res.text())
  const payload = await res.json() as { url?: unknown }
  if (typeof payload.url !== 'string') throw new Error('订阅链接读取失败')
  return payload.url
}

export async function createAirport(name: string, url: string): Promise<Airport> {
  if (isWails()) {
    return window.go!.desktop!.App!.CreateAirport(name, url)
  }
  const res = await fetch(`${API_BASE}/api/airports`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, url }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function updateAirport(id: string, name: string, url: string): Promise<Airport> {
  if (isWails()) {
    return window.go!.desktop!.App!.UpdateAirport(id, name, url)
  }
  const res = await fetch(`${API_BASE}/api/airports/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, url }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function deleteAirport(id: string): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.DeleteAirport(id)
  }
  const res = await fetch(`${API_BASE}/api/airports/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(await res.text())
}

export async function refreshAirport(id: string): Promise<Airport> {
  if (isWails()) {
    return window.go!.desktop!.App!.RefreshAirport(id)
  }
  const res = await fetch(`${API_BASE}/api/airports/${id}/refresh`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchAirportNodes(airportId: string): Promise<NodeItem[]> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetAirportNodes(airportId)
  }
  const res = await fetch(`${API_BASE}/api/airports/${airportId}/nodes`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function startBatchTest(req: BatchTestRequest): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.StartBatch(req)
  }
  const res = await fetch(`${API_BASE}/api/test/batch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function startSingleTest(req: SingleTestRequest): Promise<NodeResult> {
  if (isWails()) {
    return window.go!.desktop!.App!.TestSingle(req)
  }
  const res = await fetch(`${API_BASE}/api/test/single`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

// --- Stable-identity workbench latency path ---

export async function runWorkbenchLatencyTest(req: WorkbenchLatencyTestRequest): Promise<WorkbenchLatencyTest> {
  if (isWails()) {
    return window.go!.desktop!.App!.RunWorkbenchLatencyTest(req)
  }
  const res = await fetch(`${API_BASE}/api/workbench/latency-tests`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export function buildWorkbenchLatencyHistoryQuery(query: WorkbenchLatencyHistoryQuery): string {
  const params = new URLSearchParams({
    profile_id: query.profile_id,
    node_key: query.node_key,
    node_identity_key: query.node_identity_key,
    config_revision_key: query.config_revision_key,
    since: query.since,
    until: query.until,
  })
  if (query.limit) params.set('limit', String(query.limit))
  if (query.before_finished_at && query.before_attempt_id) {
    params.set('before_finished_at', query.before_finished_at)
    params.set('before_attempt_id', query.before_attempt_id)
  }
  return params.toString()
}

export async function fetchWorkbenchLatencyHistory(query: WorkbenchLatencyHistoryQuery): Promise<WorkbenchLatencyHistoryResult> {
  if (isWails()) {
    return window.go!.desktop!.App!.ListWorkbenchLatencyTests(query)
  }
  const res = await fetch(`${API_BASE}/api/workbench/latency-tests?${buildWorkbenchLatencyHistoryQuery(query)}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchWorkbenchLatencyTest(query: WorkbenchLatencyHistoryDetailQuery): Promise<WorkbenchLatencyTest> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetWorkbenchLatencyTest(query)
  }
  const params = new URLSearchParams({ profile_id: query.profile_id, node_key: query.node_key, node_identity_key: query.node_identity_key, config_revision_key: query.config_revision_key })
  if (query.since) params.set('since', query.since)
  if (query.until) params.set('until', query.until)
  const res = await fetch(`${API_BASE}/api/workbench/latency-tests/${encodeURIComponent(query.attempt_id)}?${params.toString()}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchNodeHistoryRevisions(profileId: string, nodeIdentityKey: string): Promise<NodeHistoryRevision[]> {
  if (isWails()) return window.go!.desktop!.App!.ListNodeHistoryRevisions(profileId, nodeIdentityKey)
  const params = new URLSearchParams({ profile_id: profileId, node_identity_key: nodeIdentityKey })
  const res = await fetch(`${API_BASE}/api/history/node-revisions?${params.toString()}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function startWorkbenchLatencyBatch(req: WorkbenchLatencyBatchRequest): Promise<WorkbenchLatencyBatch> {
  if (isWails()) return window.go!.desktop!.App!.StartWorkbenchLatencyBatch(req)
  const res = await fetch(`${API_BASE}/api/workbench/latency-batches`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(req) })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchWorkbenchLatencyBatches(limit = 20): Promise<WorkbenchLatencyBatch[]> {
  if (isWails()) return window.go!.desktop!.App!.ListWorkbenchLatencyBatches(limit)
  const res = await fetch(`${API_BASE}/api/workbench/latency-batches?limit=${limit}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchWorkbenchLatencyBatch(batchID: string): Promise<WorkbenchLatencyBatch> {
  if (isWails()) return window.go!.desktop!.App!.GetWorkbenchLatencyBatch(batchID)
  const res = await fetch(`${API_BASE}/api/workbench/latency-batches/${encodeURIComponent(batchID)}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function cancelWorkbenchLatencyBatch(batchID: string): Promise<WorkbenchLatencyBatch> {
  if (isWails()) return window.go!.desktop!.App!.CancelWorkbenchLatencyBatch(batchID)
  const res = await fetch(`${API_BASE}/api/workbench/latency-batches/${encodeURIComponent(batchID)}/cancel`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function retryWorkbenchLatencyBatchItem(batchID: string, itemID: string): Promise<WorkbenchLatencyBatch> {
  if (isWails()) return window.go!.desktop!.App!.RetryWorkbenchLatencyBatchItem(batchID, itemID)
  const res = await fetch(`${API_BASE}/api/workbench/latency-batches/${encodeURIComponent(batchID)}/items/${encodeURIComponent(itemID)}/retry-save`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function listWorkbenchPublicServiceCatalog(): Promise<WorkbenchPublicServiceRule[]> {
  if (isWails()) return window.go!.desktop!.App!.ListWorkbenchPublicServiceCatalog()
  const res = await fetch(`${API_BASE}/api/workbench/public-service-catalog`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function startWorkbenchPublicServiceTest(req: WorkbenchPublicServiceTestRequest): Promise<WorkbenchPublicServiceAttempt> {
  if (isWails()) return window.go!.desktop!.App!.StartWorkbenchPublicServiceTest(req)
  const res = await fetch(`${API_BASE}/api/workbench/public-service-tests`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(req),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export function buildWorkbenchPublicServiceHistoryQuery(query: WorkbenchPublicServiceHistoryQuery): string {
  const params = new URLSearchParams({
    profile_id: query.profile_id,
    node_key: query.node_key,
    node_identity_key: query.node_identity_key,
    config_revision_key: query.config_revision_key,
  })
  if (query.service_id) params.set('service_id', query.service_id)
  if (query.since) params.set('since', query.since)
  if (query.until) params.set('until', query.until)
  if (query.limit) params.set('limit', String(query.limit))
  if (query.before_at && query.before_attempt_id) {
    params.set('before_at', query.before_at)
    params.set('before_attempt_id', query.before_attempt_id)
  }
  return params.toString()
}

export async function fetchWorkbenchPublicServiceHistory(query: WorkbenchPublicServiceHistoryQuery): Promise<WorkbenchPublicServiceHistoryResult> {
  if (isWails()) return window.go!.desktop!.App!.ListWorkbenchPublicServiceTests(query)
  const res = await fetch(`${API_BASE}/api/workbench/public-service-tests?${buildWorkbenchPublicServiceHistoryQuery(query)}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchWorkbenchPublicServiceAttempt(attemptID: string, query: WorkbenchPublicServiceHistoryQuery): Promise<WorkbenchPublicServiceAttempt> {
  if (isWails()) return window.go!.desktop!.App!.GetWorkbenchPublicServiceAttempt(attemptID, query)
  const res = await fetch(`${API_BASE}/api/workbench/public-service-tests/${encodeURIComponent(attemptID)}?${buildWorkbenchPublicServiceHistoryQuery(query)}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function cancelWorkbenchPublicServiceTest(attemptID: string, query: WorkbenchPublicServiceHistoryQuery): Promise<WorkbenchPublicServiceAttempt> {
  if (isWails()) return window.go!.desktop!.App!.CancelWorkbenchPublicServiceTest(attemptID, query)
  const res = await fetch(`${API_BASE}/api/workbench/public-service-tests/${encodeURIComponent(attemptID)}/cancel?${buildWorkbenchPublicServiceHistoryQuery(query)}`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function retrySaveWorkbenchPublicServiceTest(attemptID: string, query: WorkbenchPublicServiceHistoryQuery): Promise<WorkbenchPublicServiceAttempt> {
  if (isWails()) return window.go!.desktop!.App!.RetrySaveWorkbenchPublicServiceTest(attemptID, query)
  const res = await fetch(`${API_BASE}/api/workbench/public-service-tests/${encodeURIComponent(attemptID)}/retry-save?${buildWorkbenchPublicServiceHistoryQuery(query)}`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function startWorkbenchDownloadTest(req: WorkbenchDownloadTestRequest): Promise<WorkbenchDownloadAttempt> {
  if (isWails()) return window.go!.desktop!.App!.StartWorkbenchDownloadTest(req)
  const res = await fetch(`${API_BASE}/api/workbench/download-tests`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(req) })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export function buildWorkbenchDownloadHistoryQuery(query: WorkbenchDownloadHistoryQuery): string {
  const params = new URLSearchParams({ profile_id: query.profile_id, node_key: query.node_key, node_identity_key: query.node_identity_key, config_revision_key: query.config_revision_key })
  if (query.since) params.set('since', query.since)
  if (query.until) params.set('until', query.until)
  if (query.limit) params.set('limit', String(query.limit))
  if (query.before_at && query.before_attempt_id) { params.set('before_at', query.before_at); params.set('before_attempt_id', query.before_attempt_id) }
  return params.toString()
}

export async function fetchWorkbenchDownloadHistory(query: WorkbenchDownloadHistoryQuery): Promise<WorkbenchDownloadHistoryResult> {
  if (isWails()) return window.go!.desktop!.App!.ListWorkbenchDownloadTests(query)
  const res = await fetch(`${API_BASE}/api/workbench/download-tests?${buildWorkbenchDownloadHistoryQuery(query)}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchWorkbenchDownloadAttempt(attemptID: string, query: WorkbenchDownloadHistoryQuery): Promise<WorkbenchDownloadAttempt> {
  if (isWails()) return window.go!.desktop!.App!.GetWorkbenchDownloadAttempt(attemptID, query)
  const res = await fetch(`${API_BASE}/api/workbench/download-tests/${encodeURIComponent(attemptID)}?${buildWorkbenchDownloadHistoryQuery(query)}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function cancelWorkbenchDownloadTest(attemptID: string, query: WorkbenchDownloadHistoryQuery): Promise<WorkbenchDownloadAttempt> {
  if (isWails()) return window.go!.desktop!.App!.CancelWorkbenchDownloadTest(attemptID, query)
  const res = await fetch(`${API_BASE}/api/workbench/download-tests/${encodeURIComponent(attemptID)}/cancel?${buildWorkbenchDownloadHistoryQuery(query)}`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function retrySaveWorkbenchDownloadTest(attemptID: string, query: WorkbenchDownloadHistoryQuery): Promise<WorkbenchDownloadAttempt> {
  if (isWails()) return window.go!.desktop!.App!.RetrySaveWorkbenchDownloadTest(attemptID, query)
  const res = await fetch(`${API_BASE}/api/workbench/download-tests/${encodeURIComponent(attemptID)}/retry-save?${buildWorkbenchDownloadHistoryQuery(query)}`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function stopTest(): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.StopTest()
  }
  const res = await fetch(`${API_BASE}/api/test/stop`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function fetchTestStatus(): Promise<TestStatus> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetStatus()
  }
  const res = await fetch(`${API_BASE}/api/test/status`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchHistory(): Promise<RunSummary[]> {
  if (isWails()) {
    return window.go!.desktop!.App!.ListHistory()
  }
  const res = await fetch(`${API_BASE}/api/history`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchHistoryRun(id: string): Promise<TestRun> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetHistory(id)
  }
  const res = await fetch(`${API_BASE}/api/history/${id}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function deleteHistoryRun(id: string): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.DeleteHistory(id)
  }
  const res = await fetch(`${API_BASE}/api/history/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(await res.text())
}

export async function fetchAirportTimeline(airportId: string): Promise<AirportHistory> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetAirportTimeline(airportId)
  }
  const res = await fetch(`${API_BASE}/api/history/airport?airport_id=${encodeURIComponent(airportId)}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function compareRuns(baseId: string, targetId: string): Promise<RunComparison> {
  if (isWails()) {
    return window.go!.desktop!.App!.CompareRuns(baseId, targetId)
  }
  const res = await fetch(
    `${API_BASE}/api/history/compare?base_id=${encodeURIComponent(baseId)}&target_id=${encodeURIComponent(targetId)}`
  )
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchTokenStatus(): Promise<TokenStatus> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetTokenStatus()
  }
  const res = await fetch(`${API_BASE}/api/antigravity/status`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function setToken(token: string): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.SetToken(token)
  }
  const res = await fetch(`${API_BASE}/api/antigravity/token`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function startOAuthLogin(): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.StartOAuthLogin()
  }
  const res = await fetch(`${API_BASE}/api/antigravity/login`, { method: 'POST' })
  if (!res.ok) throw new Error(await res.text())
}

export async function exportClashYAML(airportId: string, nodeNames: string[]): Promise<string> {
  if (isWails()) {
    return window.go!.desktop!.App!.ExportClashConfig(airportId, nodeNames)
  }
  const res = await fetch(`${API_BASE}/api/export/clash`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ airport_id: airportId, node_names: nodeNames }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.text()
}

export async function exportClashYAMLFromResults(runId: string, nodeNames: string[]): Promise<string> {
  if (isWails()) {
    return window.go!.desktop!.App!.ExportClashConfigFromResults(runId, nodeNames)
  }
  const res = await fetch(`${API_BASE}/api/export/clash-run`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ run_id: runId, node_names: nodeNames }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.text()
}

export async function fetchSettings(): Promise<AppSettings> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetSettings()
  }
  const res = await fetch(`${API_BASE}/api/settings`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function saveSettings(settings: AppSettings): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.SaveSettings(settings)
  }
  const res = await fetch(`${API_BASE}/api/settings`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function openExternalURL(url: string): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.OpenURL(url)
  }
  window.open(url, '_blank')
}

// --- Controller & Smart Orchestrator Bridge ---

export async function fetchControllerStatus(): Promise<ControllerStatus> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetControllerStatus()
  }
  const res = await fetch(`${API_BASE}/api/controller/status`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function configureController(cfg: ControllerConfig): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.ConfigureController(cfg)
  }
  const res = await fetch(`${API_BASE}/api/controller/config`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function fetchControllerGroups(): Promise<ControllerGroup[]> {
  if (isWails()) {
    return window.go!.desktop!.App!.ListControllerGroups()
  }
  const res = await fetch(`${API_BASE}/api/controller/groups`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function selectControllerNode(group: string, node: string): Promise<void> {
  if (isWails()) {
    return window.go!.desktop!.App!.SelectControllerNode(group, node)
  }
  const res = await fetch(`${API_BASE}/api/controller/select`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ group, node }),
  })
  if (!res.ok) throw new Error(await res.text())
}

export async function fetchSwitchPolicy(): Promise<SwitchPolicy> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetSwitchPolicy()
  }
  const res = await fetch(`${API_BASE}/api/controller/policy`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function updateSwitchPolicy(policy: SwitchPolicy): Promise<SwitchPolicy> {
  if (isWails()) {
    await window.go!.desktop!.App!.UpdateSwitchPolicy(policy)
    return policy
  }
  const res = await fetch(`${API_BASE}/api/controller/policy`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(policy),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchSwitchAuditTrail(): Promise<SwitchEvent[]> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetSwitchAuditTrail()
  }
  const res = await fetch(`${API_BASE}/api/controller/audit`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

