import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type {
  Airport,
  NodeItem,
  NodeResult,
  AuditEvidence,
  TestStatus,
  TestConfig,
  TokenStatus,
  TriageCategory,
  RunSummary,
  TestRun,
  ProfileSetup,
} from '../types'
import * as api from '../api/bridge'

export const useWorkbenchStore = defineStore('workbench', () => {
  // State
  const airports = ref<Airport[]>([])
  const selectedAirportId = ref<string>('')
  const nodes = ref<NodeItem[]>([])
  const selectedNodeNames = ref<string[]>([])
  const resultsMap = ref<Record<string, NodeResult>>({})
  const nodeAuditMap = ref<Record<string, AuditEvidence>>({})
  const retestHistoryMap = ref<Record<string, { passed: number; total: number }[]>>({})
  const focusedNode = ref<NodeResult | null>(null)
  const activeTriageFilter = ref<TriageCategory | 'all'>('all')

  const testStatus = ref<TestStatus>({
    is_running: false,
    current_index: 0,
    total_nodes: 0,
    percent: 0,
  })

  const testConfig = ref<TestConfig>({
    metrics: ['latency', 'download', 'antigravity'],
    concurrent: 4,
    timeout_sec: 5,
    download_size: 50 * 1024 * 1024,
    upload_size: 20 * 1024 * 1024,
    server_url: 'https://speed.cloudflare.com',
    rounds: 1,
  })

  const tokenStatus = ref<TokenStatus>({
    has_token: false,
    source: '',
    preview: '',
  })

  const historyRuns = ref<RunSummary[]>([])
  const selectedRun = ref<TestRun | null>(null)
  const isHistoryModalOpen = ref(false)
  const isAirportModalOpen = ref(false)
  const isSettingsModalOpen = ref(false)
  const isDiffModalOpen = ref(false)
  const profileSetup = ref<ProfileSetup | null>(null)
  const isProfileSetupOpen = ref(false)

  // Getters
  const selectedAirport = computed(() =>
    airports.value.find((a) => a.id === selectedAirportId.value) || null
  )

  const triageCounts = computed(() => {
    const counts: Record<TriageCategory, number> = {
      stable: 0,
      single_pass: 0,
      unverified: 0,
      flapping: 0,
      blocked: 0,
      failed: 0,
    }
    for (const name of Object.keys(resultsMap.value)) {
      const audit = nodeAuditMap.value[name]
      if (audit) {
        counts[audit.category] = (counts[audit.category] || 0) + 1
      }
    }
    return counts
  })

  const filteredResults = computed(() => {
    const list: NodeResult[] = Object.values(resultsMap.value)
    if (activeTriageFilter.value === 'all') {
      return list
    }
    return list.filter((r) => {
      const audit = nodeAuditMap.value[r.proxy_name]
      return audit && audit.category === activeTriageFilter.value
    })
  })

  // Evaluates node results conforming strictly to the user's credibility guidelines:
  function evaluateNodeAudit(r: NodeResult, isRetest = false): AuditEvidence {
    const stab = r.stability
    const passed = stab ? stab.success_probes : r.latency_ms > 0 ? 1 : 0
    const total = stab ? stab.total_probes : 1

    // Maintain retest history trail: e.g. "5/8 -> 0/6"
    if (!retestHistoryMap.value[r.proxy_name]) {
      retestHistoryMap.value[r.proxy_name] = []
    }
    if (isRetest) {
      retestHistoryMap.value[r.proxy_name].push({ passed, total })
    } else if (retestHistoryMap.value[r.proxy_name].length === 0) {
      retestHistoryMap.value[r.proxy_name].push({ passed, total })
    }

    const historyRounds = retestHistoryMap.value[r.proxy_name]
    const trailStr = historyRounds.map((h) => `${h.passed}/${h.total}`).join(' → ')
    const numRounds = historyRounds.length

    // 1. Explicit Geo-Blocking check
    const isBlocked =
      r.antigravity_status === 'blocked' ||
      (r.antigravity_detail &&
        (r.antigravity_detail.includes('User location is not supported') ||
          r.antigravity_detail.includes('FAILED_PRECONDITION') ||
          r.antigravity_detail.includes('地区不支持')))

    if (isBlocked) {
      return {
        category: 'blocked',
        badgeLabel: '地区明确阻断',
        badgeClass: 'bg-red-500/15 text-red-500 border border-red-500/30',
        reason: '服务商返回明确地区封锁错误 (FAILED_PRECONDITION)',
        historyTrail: trailStr,
        roundsPassed: passed,
        roundsTotal: total,
        requiresRetest: false,
        isGeoBlocked: true,
        isConfirmedFlapping: false,
      }
    }

    // 2. Failure: 0 passes
    if (passed === 0) {
      if (numRounds > 1 && historyRounds.every((h) => h.passed === 0)) {
        return {
          category: 'failed',
          badgeLabel: '当前持续失败',
          badgeClass: 'bg-zinc-500/15 text-zinc-400 border border-zinc-500/30',
          reason: `连续 ${numRounds} 轮探针 0 成功，出口完全不可达`,
          historyTrail: trailStr,
          roundsPassed: passed,
          roundsTotal: total,
          requiresRetest: true,
          isGeoBlocked: false,
          isConfirmedFlapping: false,
        }
      }
      return {
        category: 'failed',
        badgeLabel: '当前不可用',
        badgeClass: 'bg-zinc-500/15 text-zinc-400 border border-zinc-500/30',
        reason: '本轮采样全败，属于临时失败或断线',
        historyTrail: trailStr,
        roundsPassed: passed,
        roundsTotal: total,
        requiresRetest: true,
        isGeoBlocked: false,
        isConfirmedFlapping: false,
      }
    }

    // 3. Multi-probe partial success: check IP drift vs persistent instability
    if (passed < total) {
      const exitIPs = stab?.exit_ips || []
      const hasIPDrift = exitIPs.length > 1
      if (hasIPDrift) {
        return {
          category: 'flapping',
          badgeLabel: '确认多出口漂移',
          badgeClass: 'bg-amber-500/15 text-amber-500 border border-amber-500/30',
          reason: `观测到 ${exitIPs.length} 个不同出口 IP 轮换，存在会话漂移`,
          historyTrail: trailStr,
          roundsPassed: passed,
          roundsTotal: total,
          requiresRetest: true,
          isGeoBlocked: false,
          isConfirmedFlapping: true,
        }
      }
      return {
        category: 'unverified',
        badgeLabel: '持续不稳定 (待复核)',
        badgeClass: 'bg-orange-500/15 text-orange-400 border border-orange-500/30',
        reason: `部分探针失败 (${passed}/${total})，需进一步复测`,
        historyTrail: trailStr,
        roundsPassed: passed,
        roundsTotal: total,
        requiresRetest: true,
        isGeoBlocked: false,
        isConfirmedFlapping: false,
      }
    }

    // 4. All passes: passed === total
    if (numRounds > 1 && historyRounds.every((h) => h.passed === h.total)) {
      return {
        category: 'stable',
        badgeLabel: '稳定可用',
        badgeClass: 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30',
        reason: `连续 ${numRounds} 轮复测全通，出口与延迟稳定`,
        historyTrail: trailStr,
        roundsPassed: passed,
        roundsTotal: total,
        requiresRetest: false,
        isGeoBlocked: false,
        isConfirmedFlapping: false,
      }
    }

    return {
      category: 'single_pass',
      badgeLabel: '初测全通 (待复核)',
      badgeClass: 'bg-blue-500/15 text-blue-400 border border-blue-500/30',
      reason: '单轮采样全通，尚需复测验证稳定性',
      historyTrail: trailStr,
      roundsPassed: passed,
      roundsTotal: total,
      requiresRetest: false,
      isGeoBlocked: false,
      isConfirmedFlapping: false,
    }
  }

  // Actions
  async function loadProfileSetup() {
    try {
      profileSetup.value = await api.fetchProfileSetup()
      if (
        profileSetup.value.state !== 'ready' ||
        ['pending', 'conflict', 'invalid'].includes(profileSetup.value.migration?.state)
      ) {
        isProfileSetupOpen.value = true
      }
    } catch (e) {
      console.error('Failed to load profile setup:', e)
    }
  }

  async function loadAirports() {
    if (profileSetup.value && profileSetup.value.state !== 'ready') {
      return
    }
    try {
      airports.value = await api.fetchAirports()
      if (airports.value.length > 0 && !selectedAirportId.value) {
        selectedAirportId.value = airports.value[0].id
        await loadAirportNodes(airports.value[0].id)
      }
    } catch (e) {
      console.error('Failed to load airports:', e)
    }
  }

  async function loadAirportNodes(airportId: string) {
    try {
      nodes.value = await api.fetchAirportNodes(airportId)
      selectedNodeNames.value = nodes.value.map((n) => n.name)
    } catch (e) {
      console.error('Failed to load airport nodes:', e)
    }
  }

  async function loadTokenStatus() {
    try {
      tokenStatus.value = await api.fetchTokenStatus()
    } catch (e) {
      console.error('Failed to load token status:', e)
    }
  }

  async function loadHistory() {
    try {
      historyRuns.value = await api.fetchHistory()
    } catch (e) {
      console.error('Failed to load history:', e)
    }
  }

  function handleEvent(type: string, payload: any) {
    if (type === 'test_started') {
      testStatus.value = {
        is_running: true,
        current_index: 0,
        total_nodes: payload.total_nodes || 0,
        percent: 0,
        airport_name: payload.airport_name,
      }
      resultsMap.value = {}
      nodeAuditMap.value = {}
      retestHistoryMap.value = {}
    } else if (type === 'test_stopped') {
      testStatus.value.is_running = false
    } else if (type === 'test_completed') {
      testStatus.value.is_running = false
      testStatus.value.percent = 100
      loadHistory()
    } else if (type === 'node_progress') {
      testStatus.value.current_index = payload.current_index
      testStatus.value.total_nodes = payload.total
      testStatus.value.percent = payload.percent
      if (payload.result) {
        const r = payload.result as NodeResult
        resultsMap.value[r.proxy_name] = r
        nodeAuditMap.value[r.proxy_name] = evaluateNodeAudit(r, false)
        if (!focusedNode.value) {
          focusedNode.value = r
        }
      }
    } else if (type === 'single_node_progress' || type === 'single_test_completed') {
      if (payload.result) {
        const r = payload.result as NodeResult
        resultsMap.value[r.proxy_name] = r
        nodeAuditMap.value[r.proxy_name] = evaluateNodeAudit(r, true)
        focusedNode.value = r
      }
    } else if (type === 'antigravity_token_updated') {
      loadTokenStatus()
    }
  }

  async function startBatch() {
    if (!selectedAirportId.value) return
    await api.startBatchTest({
      airport_id: selectedAirportId.value,
      node_names: selectedNodeNames.value,
      config: testConfig.value,
    })
  }

  async function retestNode(nodeName: string) {
    if (!selectedAirportId.value) return
    await api.startSingleTest({
      airport_id: selectedAirportId.value,
      node_name: nodeName,
      config: testConfig.value,
    })
  }

  async function retestCategory(category: TriageCategory) {
    const targetNames = Object.values(resultsMap.value)
      .filter((r) => nodeAuditMap.value[r.proxy_name]?.category === category)
      .map((r) => r.proxy_name)

    for (const name of targetNames) {
      await retestNode(name)
    }
  }

  async function stop() {
    await api.stopTest()
  }

  return {
    airports,
    selectedAirportId,
    selectedAirport,
    nodes,
    selectedNodeNames,
    resultsMap,
    nodeAuditMap,
    retestHistoryMap,
    focusedNode,
    activeTriageFilter,
    triageCounts,
    filteredResults,
    testStatus,
    testConfig,
    tokenStatus,
    historyRuns,
    selectedRun,
    isHistoryModalOpen,
    isAirportModalOpen,
    profileSetup,
    isProfileSetupOpen,
    isSettingsModalOpen,
    isDiffModalOpen,
    loadAirports,
    loadProfileSetup,
    loadAirportNodes,
    loadTokenStatus,
    loadHistory,
    handleEvent,
    startBatch,
    retestNode,
    retestCategory,
    stop,
  }
})
