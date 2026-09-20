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
  WorkbenchLatencyTest,
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

export async function fetchAirports(): Promise<Airport[]> {
  if (isWails()) {
    return window.go!.desktop!.App!.ListAirports()
  }
  const res = await fetch(`${API_BASE}/api/airports`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
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

export async function fetchWorkbenchLatencyHistory(query: WorkbenchLatencyHistoryQuery): Promise<WorkbenchLatencyTest[]> {
  if (isWails()) {
    return window.go!.desktop!.App!.ListWorkbenchLatencyTests(query)
  }
  const params = new URLSearchParams({
    profile_id: query.profile_id,
    node_key: query.node_key,
  })
  if (query.limit) params.set('limit', String(query.limit))
  const res = await fetch(`${API_BASE}/api/workbench/latency-tests?${params.toString()}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function fetchWorkbenchLatencyTest(attemptID: string): Promise<WorkbenchLatencyTest> {
  if (isWails()) {
    return window.go!.desktop!.App!.GetWorkbenchLatencyTest(attemptID)
  }
  const res = await fetch(`${API_BASE}/api/workbench/latency-tests/${encodeURIComponent(attemptID)}`)
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

