<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { MonitorNodeOption, WorkbenchPublicServiceAttempt, WorkbenchPublicServiceRule } from '../../types'
import { serviceDetailLabel, serviceOutcomeLabel } from '../../utils/serviceOutcome'
import { serviceEvidence, serviceTitle, summarizeService } from '../../utils/servicePresentation'
import UiSelect from '../common/UiSelect.vue'
import InteractiveTrendSparkline, { type TrendPoint } from './InteractiveTrendSparkline.vue'
import ServiceHistoryTooltip from './ServiceHistoryTooltip.vue'

const props = defineProps<{
  rows: { key: string; node: MonitorNodeOption }[]
  services: { value: string | number; label: string }[]
  catalog?: WorkbenchPublicServiceRule[]
  records: Record<string, WorkbenchPublicServiceAttempt[]>
  selected: string[]
  states: Record<string, string>
  partial: Record<string, { hasMore: boolean; complete: boolean }>
  running?: boolean
}>()

const emit = defineEmits<{
  (e: 'toggle', key: string): void
  (e: 'service', id: string): void
  (e: 'detail', node: MonitorNodeOption): void
  (e: 'run', keys?: string[]): void
  (e: 'select-all', keys: string[]): void
  (e: 'clear-selection'): void
  (e: 'back-to-picker'): void
  (e: 'back-to-latency'): void
}>()

const isTestingEnv = typeof process !== 'undefined' && (process.env?.NODE_ENV === 'test' || Boolean(process.env?.VITEST))

function readPersistedStorage<T>(key: string, fallback: T): T {
  try {
    if (typeof localStorage !== 'undefined' && !isTestingEnv) {
      const v = localStorage.getItem(key)
      if (v !== null) return v as unknown as T
    }
  } catch {}
  return fallback
}

function writePersistedStorage(key: string, value: string): void {
  try {
    if (typeof localStorage !== 'undefined' && !isTestingEnv) {
      localStorage.setItem(key, value)
    }
  } catch {}
}

const focus = ref(readPersistedStorage('speedtest.service-comparison-focus', ''))
watch(focus, (v) => { if (v) writePersistedStorage('speedtest.service-comparison-focus', v) })

const showAllNodes = ref(readPersistedStorage<string>('speedtest.service-comparison-show-all', 'false') === 'true')
watch(showAllNodes, (v) => writePersistedStorage('speedtest.service-comparison-show-all', String(v)))

const inspected = ref<WorkbenchPublicServiceAttempt | null>(null)
const inspectedNodeKey = ref('')
const viewMode = ref<'all' | 'single'>(readPersistedStorage<'all' | 'single'>('speedtest.service-comparison-view-mode', 'all'))
watch(viewMode, (v) => writePersistedStorage('speedtest.service-comparison-view-mode', v))

const sortBy = ref<'region' | 'pass_rate' | 'status' | 'recent' | 'changes' | 'name'>(
  readPersistedStorage<'region' | 'pass_rate' | 'status' | 'recent' | 'changes' | 'name'>('speedtest.service-comparison-sort', 'region')
)
watch(sortBy, (v) => writePersistedStorage('speedtest.service-comparison-sort', v))

const selectedTimeWindow = ref<'24h' | '48h' | '7d' | 'all'>('24h')

function nodeServiceTrendPoints(key: string): TrendPoint[] {
  const list = props.records[key] || []
  const filtered = viewMode.value === 'single' && focus.value
    ? list.filter(a => a.service_id === focus.value)
    : list
  return filtered
    .filter(a => a.result && a.result.outcome !== 'cancelled')
    .map(a => ({
      id: a.attempt_id,
      time: Date.parse(a.result?.finished_at || a.finished_at || a.requested_at),
      durationMs: a.result?.duration_ms,
      status: ['matched', 'reachable', 'profiled', 'unlocked'].includes(a.result?.outcome || '') ? 'completed' : 'failed',
      outcome: a.result?.outcome,
      summary: cleanSummary(a.result, a.rule),
    }))
    .sort((a, b) => a.time - b.time)
}

const sortOptions = [
  { value: 'region', label: '按地区' },
  { value: 'status', label: '状态优先 (可用在前)' },
  { value: 'pass_rate', label: '通过率优先' },
  { value: 'recent', label: '最新测试优先' },
  { value: 'changes', label: '结果变化优先' },
  { value: 'name', label: '按节点名称' },
]

const choices = computed(() => {
  const map = new Map<string, string>()
  for (const s of props.services) {
    map.set(String(s.value), s.label)
  }
  for (const list of Object.values(props.records)) {
    for (const item of list) {
      if (item.service_id && !map.has(item.service_id)) {
        const matched = props.catalog?.find(r => r.service_id === item.service_id)
        map.set(item.service_id, matched?.name || item.service_id)
      }
    }
  }
  return [...map.entries()].map(([value, label]) => {
    const matchedRule = props.catalog?.find(r => r.service_id === value)
    return { value, label: matchedRule?.name || label, evidence: serviceEvidence(matchedRule) }
  })
})

watch(choices, list => {
  if (!list.some(s => s.value === focus.value)) {
    focus.value = list[0]?.value || ''
  }
}, { immediate: true })

const rule = computed(() => props.catalog?.find(r => r.service_id === focus.value))

const viewChoice = computed({
  get: () => (viewMode.value === 'all' ? 'all' : focus.value),
  set: (value: string | number) => {
    if (value === 'all') {
      viewMode.value = 'all'
      return
    }
    focus.value = String(value)
    viewMode.value = 'single'
    emit('service', focus.value)
  },
})

function outcomeRank(outcome?: string): number {
  if (!outcome) return 0
  switch (outcome) {
    case 'matched':
    case 'unlocked':
      return 6
    case 'profiled':
    case 'reachable':
      return 5
    case 'challenge':
    case 'redirect':
      return 3
    case 'rate_limited':
    case 'permission_denied':
    case 'auth_failed':
    case 'setup_required':
      return 2
    case 'region_blocked':
    case 'transport_error':
    case 'timed_out':
    case 'criteria_mismatch':
      return 1
    default:
      return 1
  }
}

const rows = computed(() =>
  props.rows
    .map(row => ({
      ...row,
      report: summarizeService((props.records[row.key] || []).filter(a => a.service_id === focus.value)),
    }))
    .filter(row => showAllNodes.value || row.report.total > 0),
)

const sortedRows = computed(() => {
  const list = [...rows.value]
  switch (sortBy.value) {
    case 'pass_rate':
      return list.sort((a, b) => {
        if (a.report.total === 0 && b.report.total > 0) return 1
        if (a.report.total > 0 && b.report.total === 0) return -1
        if (b.report.rate !== a.report.rate) return (b.report.rate ?? 0) - (a.report.rate ?? 0)
        return b.report.passed - a.report.passed
      })
    case 'status':
      return list.sort((a, b) => {
        const rankA = outcomeRank(a.report.latest?.result?.outcome)
        const rankB = outcomeRank(b.report.latest?.result?.outcome)
        if (rankB !== rankA) return rankB - rankA
        return (b.report.rate ?? 0) - (a.report.rate ?? 0)
      })
    case 'recent':
      return list.sort((a, b) => {
        const timeA = a.report.latest
          ? Date.parse(a.report.latest.result?.finished_at || a.report.latest.finished_at || a.report.latest.requested_at)
          : 0
        const timeB = b.report.latest
          ? Date.parse(b.report.latest.result?.finished_at || b.report.latest.finished_at || b.report.latest.requested_at)
          : 0
        return timeB - timeA
      })
    case 'changes':
      return list.sort((a, b) => {
        if (b.report.changes !== a.report.changes) return b.report.changes - a.report.changes
        return b.report.total - a.report.total
      })
    case 'name':
      return list.sort((a, b) => a.node.displayName.localeCompare(b.node.displayName, 'zh-CN'))
    case 'region':
    default:
      return list.sort((a, b) => {
        const regA = a.node.countryCode || 'OTHER'
        const regB = b.node.countryCode || 'OTHER'
        if (regA !== regB) return regA.localeCompare(regB)
        return a.node.displayName.localeCompare(b.node.displayName, 'zh-CN')
      })
  }
})

const overviewRows = computed(() =>
  props.rows
    .map(row => ({
      ...row,
      reports: choices.value.map(service => ({
        service,
        report: summarizeService((props.records[row.key] || []).filter(a => a.service_id === service.value)),
      })),
    }))
    .filter(row => showAllNodes.value || row.reports.some(item => item.report.total > 0)),
)

const sortedOverviewRows = computed(() => {
  const list = [...overviewRows.value]
  switch (sortBy.value) {
    case 'pass_rate':
      return list.sort((a, b) => {
        const totalA = a.reports.reduce((sum, item) => sum + item.report.total, 0)
        const totalB = b.reports.reduce((sum, item) => sum + item.report.total, 0)
        if (totalA === 0 && totalB > 0) return 1
        if (totalA > 0 && totalB === 0) return -1
        const passA = a.reports.reduce((sum, item) => sum + item.report.passed, 0)
        const passB = b.reports.reduce((sum, item) => sum + item.report.passed, 0)
        const rateA = totalA > 0 ? passA / totalA : 0
        const rateB = totalB > 0 ? passB / totalB : 0
        if (rateB !== rateA) return rateB - rateA
        return passB - passA
      })
    case 'status':
      return list.sort((a, b) => {
        const maxRankA = Math.max(0, ...a.reports.map(r => outcomeRank(r.report.latest?.result?.outcome)))
        const maxRankB = Math.max(0, ...b.reports.map(r => outcomeRank(r.report.latest?.result?.outcome)))
        return maxRankB - maxRankA
      })
    case 'recent':
      return list.sort((a, b) => {
        const latestTimeA = Math.max(
          0,
          ...a.reports.map(r =>
            r.report.latest
              ? Date.parse(r.report.latest.result?.finished_at || r.report.latest.finished_at || r.report.latest.requested_at)
              : 0,
          ),
        )
        const latestTimeB = Math.max(
          0,
          ...b.reports.map(r =>
            r.report.latest
              ? Date.parse(r.report.latest.result?.finished_at || r.report.latest.finished_at || r.report.latest.requested_at)
              : 0,
          ),
        )
        return latestTimeB - latestTimeA
      })
    case 'changes':
      return list.sort((a, b) => {
        const chA = a.reports.reduce((sum, item) => sum + item.report.changes, 0)
        const chB = b.reports.reduce((sum, item) => sum + item.report.changes, 0)
        return chB - chA
      })
    case 'name':
      return list.sort((a, b) => a.node.displayName.localeCompare(b.node.displayName, 'zh-CN'))
    case 'region':
    default:
      return list.sort((a, b) => {
        const regA = a.node.countryCode || 'OTHER'
        const regB = b.node.countryCode || 'OTHER'
        if (regA !== regB) return regA.localeCompare(regB)
        return a.node.displayName.localeCompare(b.node.displayName, 'zh-CN')
      })
  }
})

const overviewMeasured = computed(() =>
  overviewRows.value.filter(row => row.reports.some(item => item.report.total > 0)).length,
)
const measured = computed(() => rows.value.filter(r => r.report.total > 0))
const changed = computed(() => measured.value.filter(r => r.report.changes > 0).length)

// Outdated rule detection
const outdatedRows = computed(() => {
  if (!rule.value) return []
  return rows.value.filter(
    r => r.report.total > 0 && r.report.latest?.rule.rule_version !== rule.value?.rule_version,
  )
})
const outdatedCount = computed(() => outdatedRows.value.length)
const outdatedVersion = computed(() => outdatedRows.value[0]?.report.latest?.rule.rule_version ?? '旧')

function retestOutdated() {
  const keys = outdatedRows.value.map(r => r.key)
  emit('run', keys)
}

const selectedRegion = ref(readPersistedStorage('speedtest.service-comparison-region', '全部地区'))
watch(selectedRegion, (v) => writePersistedStorage('speedtest.service-comparison-region', v))

const searchKeyword = ref('')
const statusFilter = ref<'all' | 'good' | 'bad' | 'flapping' | 'untested'>(
  readPersistedStorage<'all' | 'good' | 'bad' | 'flapping' | 'untested'>('speedtest.service-comparison-status', 'all')
)
watch(statusFilter, (v) => writePersistedStorage('speedtest.service-comparison-status', v))

const statusFilterOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'good', label: '仅可用 (通过)' },
  { value: 'bad', label: '仅异常 / 受限' },
  { value: 'flapping', label: '仅漂移/波动 (⇄)' },
  { value: 'untested', label: '仅未检测' },
]

function hasFlappingOrDrift(report?: ReturnType<typeof summarizeService>): boolean {
  if (!report || !report.samples || report.samples.length < 2) return false
  if (report.exitChanges > 0 || report.changes > 0) return true
  const ips = new Set<string>()
  for (const s of report.samples) {
    const ip = s.result?.details?.ip
    if (ip && ip.trim()) ips.add(ip.trim())
  }
  return ips.size > 1
}

function nodeHasDrift(row: { reports?: { report: ReturnType<typeof summarizeService> }[]; report?: ReturnType<typeof summarizeService> }): boolean {
  if (row.reports) {
    return row.reports.some(item => hasFlappingOrDrift(item.report))
  }
  if (row.report) {
    return hasFlappingOrDrift(row.report)
  }
  return false
}

const totalFlappingCount = computed(() => {
  if (viewMode.value === 'all') {
    return overviewRows.value.filter(r => nodeHasDrift(r)).length
  }
  return rows.value.filter(r => nodeHasDrift(r)).length
})

const inspectedHistory = computed(() => {
  if (!inspected.value) return []
  let matchKey = inspectedNodeKey.value
  if (!matchKey) {
    const matchEntry = Object.entries(props.records).find(([, attempts]) =>
      attempts.some(a => a.attempt_id === inspected.value?.attempt_id),
    )
    if (matchEntry) matchKey = matchEntry[0]
  }
  const attempts = props.records[matchKey] || []
  return attempts
    .filter(a => a.service_id === inspected.value?.service_id)
    .sort((a, b) => {
      const timeA = Date.parse(a.result?.finished_at || a.finished_at || a.requested_at || '') || 0
      const timeB = Date.parse(b.result?.finished_at || b.finished_at || b.requested_at || '') || 0
      return timeB - timeA
    })
})

const inspectedDriftInfo = computed(() => {
  const history = inspectedHistory.value
  const set = new Set<string>()
  let changes = 0
  for (let i = 0; i < history.length; i++) {
    const ip = history[i].result?.details?.ip
    if (ip && ip.trim()) set.add(ip.trim())
    if (i > 0 && history[i].result?.outcome !== history[i - 1].result?.outcome) {
      changes++
    }
  }
  const distinctIPs = [...set]
  const passed = history.filter(a => ['matched', 'unlocked', 'profiled', 'reachable'].includes(a.result?.outcome || '')).length
  const passRate = history.length ? Math.round((passed / history.length) * 100) : 0
  return {
    distinctIPs,
    changes,
    passRate,
    hasDrift: distinctIPs.length > 1 || changes > 0,
  }
})

function handleRetestInspected() {
  if (inspectedNodeKey.value) {
    emit('run', [inspectedNodeKey.value])
  } else if (inspected.value) {
    const matchEntry = Object.entries(props.records).find(([, attempts]) =>
      attempts.some(a => a.attempt_id === inspected.value?.attempt_id),
    )
    if (matchEntry) {
      emit('run', [matchEntry[0]])
    }
  }
}

// Hover Tooltip state & helpers
const tooltipState = ref({
  visible: false,
  x: 0,
  y: 0,
  serviceName: '',
  serviceId: '',
  nodeName: '',
  nodeFlag: '🌐',
  countryCode: '',
  history: [] as WorkbenchPublicServiceAttempt[],
  ruleEvidence: '',
})

let tooltipTimer: any = null

function showTooltip(x: number, y: number, data: Omit<typeof tooltipState.value, 'visible' | 'x' | 'y'>) {
  clearTimeout(tooltipTimer)
  tooltipState.value = {
    ...data,
    x,
    y,
    visible: true,
  }
}

function hideTooltip() {
  clearTimeout(tooltipTimer)
  tooltipTimer = setTimeout(() => {
    tooltipState.value.visible = false
  }, 120)
}

function handleOverviewPillEnter(
  event: MouseEvent,
  row: { key: string; node: MonitorNodeOption },
  item: { service: { value: string | number; label: string; evidence?: string }; report: ReturnType<typeof summarizeService> },
) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const history = (props.records[row.key] || []).filter(a => a.service_id === item.service.value)
  showTooltip(rect.left + rect.width / 2, rect.top, {
    serviceName: item.service.label,
    serviceId: String(item.service.value),
    nodeName: row.node.displayName,
    nodeFlag: row.node.countryFlag || '🌐',
    countryCode: row.node.countryCode || '',
    history,
    ruleEvidence: item.service.evidence || '',
  })
}

function handleSingleCardEnter(
  event: MouseEvent,
  row: { key: string; node: MonitorNodeOption; report: ReturnType<typeof summarizeService> },
) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const history = (props.records[row.key] || []).filter(a => a.service_id === focus.value)
  showTooltip(rect.left + rect.width / 2, rect.top, {
    serviceName: rule.value ? serviceTitle(rule.value) : '公共服务',
    serviceId: focus.value,
    nodeName: row.node.displayName,
    nodeFlag: row.node.countryFlag || '🌐',
    countryCode: row.node.countryCode || '',
    history,
    ruleEvidence: serviceEvidence(rule.value),
  })
}

const regionOptions = computed(() => {
  const set = new Set<string>()
  for (const r of props.rows) {
    if (r.node.countryCode && r.node.countryCode !== 'OTHER') set.add(r.node.countryCode)
  }
  const sorted = [...set].sort()
  return [{ value: '全部地区', label: '全部地区' }, ...sorted.map(c => ({ value: c, label: c }))]
})

const filteredRows = computed(() => {
  return sortedRows.value.filter(row => {
    if (selectedRegion.value !== '全部地区' && row.node.countryCode !== selectedRegion.value) {
      return false
    }
    if (searchKeyword.value.trim()) {
      const q = searchKeyword.value.trim().toLowerCase()
      const name = (row.node.displayName || '').toLowerCase()
      const reg = (row.node.countryCode || '').toLowerCase()
      const profile = (row.node.profileName || '').toLowerCase()
      if (!name.includes(q) && !reg.includes(q) && !profile.includes(q)) {
        return false
      }
    }
    if (statusFilter.value !== 'all') {
      const isGood = ['matched', 'unlocked', 'reachable', 'profiled'].includes(row.report.latest?.result?.outcome || '')
      if (statusFilter.value === 'good' && !isGood) return false
      if (statusFilter.value === 'bad' && (isGood || row.report.total === 0)) return false
      if (statusFilter.value === 'flapping' && !hasFlappingOrDrift(row.report)) return false
      if (statusFilter.value === 'untested' && row.report.total > 0) return false
    }
    return true
  })
})

const filteredOverviewRows = computed(() => {
  return sortedOverviewRows.value.filter(row => {
    if (selectedRegion.value !== '全部地区' && row.node.countryCode !== selectedRegion.value) {
      return false
    }
    if (searchKeyword.value.trim()) {
      const q = searchKeyword.value.trim().toLowerCase()
      const name = (row.node.displayName || '').toLowerCase()
      const reg = (row.node.countryCode || '').toLowerCase()
      const profile = (row.node.profileName || '').toLowerCase()
      if (!name.includes(q) && !reg.includes(q) && !profile.includes(q)) {
        return false
      }
    }
    if (statusFilter.value !== 'all') {
      if (statusFilter.value === 'flapping' && !nodeHasDrift(row)) return false
      if (statusFilter.value === 'untested' && row.reports.some(item => item.report.total > 0)) return false
      if (statusFilter.value === 'good' && !row.reports.some(item => ['matched', 'unlocked', 'reachable', 'profiled'].includes(item.report.latest?.result?.outcome || ''))) return false
      if (statusFilter.value === 'bad' && !row.reports.some(item => item.report.total > 0 && !['matched', 'unlocked', 'reachable', 'profiled'].includes(item.report.latest?.result?.outcome || ''))) return false
    }
    return true
  })
})

// Current selection helpers
const currentKeys = computed(() =>
  (viewMode.value === 'all' ? filteredOverviewRows.value : filteredRows.value).map(r => r.key),
)
const isAllSelected = computed(
  () => currentKeys.value.length > 0 && currentKeys.value.every(k => props.selected.includes(k)),
)

function toggleSelectAllCurrent() {
  if (isAllSelected.value) {
    emit('clear-selection')
  } else {
    emit('select-all', currentKeys.value)
  }
}

function handleRunBatch() {
  const keys = props.selected.length > 0 ? props.selected : currentKeys.value
  emit('run', keys)
}

function tone(a?: WorkbenchPublicServiceAttempt) {
  const outcome = a?.result?.outcome
  return !outcome
    ? 'unknown'
    : ['matched', 'profiled', 'reachable', 'unlocked'].includes(outcome)
      ? 'good'
      : ['transport_error', 'timed_out'].includes(outcome)
        ? 'bad'
        : 'limited'
}

function label(a?: WorkbenchPublicServiceAttempt) {
  if (a?.service_id === 'antigravity' && a.result?.outcome === 'matched') return '模型已回答'
  if (a?.rule.result_kind === 'exit_profile' && a.result?.outcome === 'profiled') return '出口已获取'
  return serviceOutcomeLabel(a?.result?.outcome)
}

function cleanSummary(result?: WorkbenchPublicServiceAttempt['result'], rule?: WorkbenchPublicServiceRule): string {
  if (!result) return '尚无检测记录'
  if (result.outcome === 'matched') {
    if (result.details?.checked_model) {
      return `真实模型响应成功 (${result.details.checked_model})`
    }
    return '检测通过，真实服务响应正常'
  }
  if (result.outcome === 'unlocked') {
    return '完整版权片目已解锁'
  }
  if (result.outcome === 'originals_only') {
    return '仅可访问自制剧内容'
  }
  if (result.outcome === 'region_blocked') {
    return '服务明确限制该地区访问 (403)'
  }
  if (result.outcome === 'challenge') {
    return '遇到 Cloudflare 验证盾阻拦'
  }
  if (result.outcome === 'rate_limited') {
    return '请求频次或配额受限'
  }
  if (result.outcome === 'credentials_required') {
    return '请先在系统设置中绑定 Google 账号凭据'
  }
  if (result.outcome === 'timed_out') {
    return '网络请求超时，未能收到响应'
  }
  if (result.error_message?.includes('读取上限') || result.summary?.includes('读取上限')) {
    return '旧规则响应截断，请点击右侧“⚡ 检测”以新规则重测'
  }
  return result.summary || result.error_message || serviceOutcomeLabel(result.outcome)
}

function stampRelative(a: WorkbenchPublicServiceAttempt): string {
  const t = Date.parse(a.result?.finished_at || a.finished_at || a.requested_at)
  if (!t || isNaN(t)) return ''
  const diffSec = Math.floor((Date.now() - t) / 1000)
  if (diffSec < 60) return '刚刚'
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}分钟前`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}小时前`
  return `${Math.floor(diffSec / 86400)}天前`
}

function inconclusive(report: ReturnType<typeof summarizeService>): boolean {
  return (
    report.total > 0 &&
    report.passed === 0 &&
    report.samples.every(a =>
      [
        'unknown',
        'timed_out',
        'transport_error',
        'rate_limited',
        'credentials_required',
        'setup_required',
        'auth_failed',
        'permission_denied',
      ].includes(a.result?.outcome || ''),
    )
  )
}

function stamp(a: WorkbenchPublicServiceAttempt) {
  return new Date(a.result?.finished_at || a.finished_at || a.requested_at).toLocaleString()
}
</script>

<template>
  <div class="service-results">
    <!-- Top Return & Navigation Bar -->
    <div class="service-nav-bar">
      <div class="service-nav-left">
        <button
          type="button"
          class="nav-back-btn primary"
          title="点击返回节点延迟与工作台概览"
          @click="emit('back-to-latency')"
        >
          <span class="back-arrow">←</span>
          <span>返回延迟工作台</span>
        </button>
        <span v-if="viewMode === 'single'" class="nav-crumb-separator">/</span>
        <button
          v-if="viewMode === 'single'"
          type="button"
          class="nav-back-btn subtle"
          title="返回全部服务概览列表"
          @click="viewChoice = 'all'"
        >
          <span>↩ 返回全部服务概览</span>
        </button>
      </div>
      <div class="service-nav-right">
        <span class="service-count-chip">已选 {{ selected.length }} / {{ currentKeys.length }} 节点</span>
      </div>
    </div>

    <div class="results-heading">
      <div v-if="viewMode !== 'all'" class="heading-title-box">
        <div class="heading-badge-line">
          <span class="service-rule-badge">{{ rule?.category || '公网服务' }}</span>
          <h3>{{ rule ? serviceTitle(rule) : '先在上方选择服务' }}</h3>
        </div>
        <p>
          {{
            `${serviceEvidence(rule)} · ${
              rule?.service_id === 'antigravity'
                ? '只有收到真实模型内容才标记通过；账号、配额、地区问题清晰展示。'
                : '通过率仅衡量当前服务判据。'
            }`
          }}
        </p>
      </div>
      <div class="results-tools">
        <div class="search-input-group">
          <span class="search-icon" aria-hidden="true">🔍</span>
          <input
            v-model="searchKeyword"
            type="search"
            placeholder="搜索节点 / 地区…"
            class="service-search-input"
          >
          <button v-if="searchKeyword" type="button" class="search-clear-btn" aria-label="清空搜索" @click="searchKeyword = ''">×</button>
        </div>
        <div class="tool-select-group">
          <span class="tool-label">地区</span>
          <UiSelect v-model="selectedRegion" aria-label="筛选地区" variant="compact" :options="regionOptions" />
        </div>
        <div v-if="viewMode !== 'all'" class="tool-select-group">
          <span class="tool-label">状态</span>
          <UiSelect v-model="statusFilter" aria-label="筛选状态" variant="compact" :options="statusFilterOptions" />
        </div>
        <div class="tool-select-group">
          <span class="tool-label">查看</span>
          <UiSelect v-model="viewChoice" aria-label="查看服务结果" variant="compact" :options="[{ value: 'all', label: '全部服务概览' }, ...choices]" />
        </div>
        <div class="tool-select-group">
          <span class="tool-label">排序</span>
          <UiSelect v-model="sortBy" aria-label="排序服务结果" variant="compact" :options="sortOptions" />
        </div>
        <label class="custom-toggle-label" :class="{ active: showAllNodes }" title="开启后显示未进行过该服务测试的节点">
          <input v-model="showAllNodes" type="checkbox">
          <span>含未测</span>
        </label>
        <div class="batch-action-group">
          <button type="button" class="tool-btn" @click="toggleSelectAllCurrent">
            {{ isAllSelected ? '取消全选' : `全选 (${currentKeys.length})` }}
          </button>
          <button type="button" class="tool-btn primary" :disabled="running || !currentKeys.length" @click="handleRunBatch">
            {{ running ? '检测中…' : selected.length ? `⚡ 检测已选 (${selected.length})` : `⚡ 检测全部 (${currentKeys.length})` }}
          </button>
        </div>
      </div>
    </div>

    <!-- Outdated rule notice banner -->
    <div v-if="viewMode !== 'all' && outdatedCount > 0" class="outdated-banner">
      <div class="outdated-info">
        <div class="outdated-tag-line">
          <span class="outdated-tag">⚠️ 历史规则缓存</span>
          <strong>发现 {{ outdatedCount }} 个节点的历史结果基于旧版规则 (v{{ outdatedVersion }})</strong>
        </div>
        <p>旧版规则因响应体读取上限（64 KiB）导致模型无法完整识别。当前新版规则已解除读取限制，建议重新检测以获得最新真实结果。</p>
      </div>
      <button type="button" class="outdated-retest-btn" :disabled="running" @click="retestOutdated">
        {{ running ? '检测中…' : `⚡ 重新检测这 ${outdatedCount} 个节点` }}
      </button>
    </div>

    <template v-if="viewMode === 'all'">
      <div v-if="overviewMeasured" class="overview-status">
        <span>已测节点：<b>{{ overviewMeasured }}</b> 个</span>
        <button
          v-if="totalFlappingCount > 0"
          type="button"
          class="stat-pill flapping-btn"
          :class="{ active: statusFilter === 'flapping' }"
          title="点击切换：仅查看检出多出口 IP 漂移或状态波动的节点"
          @click="statusFilter = statusFilter === 'flapping' ? 'all' : 'flapping'"
        >
          ⇄ 漂移节点 <b>{{ totalFlappingCount }}</b>
        </button>
        <small>未测节点点击“⚡ 检测”即可一键验证可用性。</small>
      </div>
      <div v-if="filteredOverviewRows.length" class="service-overview">
        <div v-for="row in filteredOverviewRows" :key="row.key" class="overview-row overview-row-v5">
          <label class="overview-node-label">
            <input type="checkbox" :checked="selected.includes(row.key)" :aria-label="`选择 ${row.node.displayName}`" @change="emit('toggle', row.key)">
            <span class="overview-flag">{{ row.node.countryFlag || '🌐' }}</span>
            <div class="overview-node-text">
              <div class="node-title-line">
                <strong :title="row.node.displayName">{{ row.node.displayName }}</strong>
                <span v-if="nodeHasDrift(row)" class="node-drift-badge" title="该节点在部分服务中检测到多出口 IP 漂移或状态波动">⇄ 漂移</span>
              </div>
              <small>{{ row.node.countryCode || 'OTHER' }} · {{ row.node.type || '节点' }}</small>
            </div>
          </label>
          <div class="overview-services-v5">
            <span
              v-for="item in row.reports"
              :key="item.service.value"
              :class="{ 'status-history': item.report.total > 0 }"
              class="inline-flex"
            >
              <button
                type="button"
                :class="['overview-service-pill', item.report.total ? tone(item.report.latest) : 'untested', { 'has-drift': hasFlappingOrDrift(item.report) }]"
                :title="`${item.service.label}：${item.report.total ? label(item.report.latest) : '未检测'} · 点击深入查看`"
                @mouseenter="handleOverviewPillEnter($event, row, item)"
                @mouseleave="hideTooltip"
                @click="inspected = item.report.latest || null; inspectedNodeKey = row.key"
              >
                <span class="pill-dot"></span>
                <span class="svc-name">{{ item.service.label.split(' ')[0] }}</span>
                <strong class="svc-status">{{ item.report.total ? label(item.report.latest) : '未测' }}</strong>
                <small v-if="item.report.latest?.result?.duration_ms" class="svc-dur">{{ item.report.latest.result.duration_ms }}ms</small>
                <span v-if="hasFlappingOrDrift(item.report)" class="pill-flapping-tag" title="检出多出口漂移或状态波动">⇄</span>
                <span v-if="item.report.total > 0" class="service-score" style="display:none">{{ Math.round((item.report.passed / item.report.total) * 100) }}%</span>
              </button>
            </span>
            <button type="button" class="node-quick-test-btn" :disabled="running" title="针对此节点立即执行检测" @click.stop="emit('run', [row.key])">
              ⚡ 检测
            </button>
          </div>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="result-summary-v5">
        <div class="summary-stat-group">
          <span class="stat-pill">已测节点 <b>{{ measured.length }}</b> / {{ rows.length }}</span>
          <span v-if="changed > 0" class="stat-pill warn">波动节点 <b>{{ changed }}</b></span>
          <button
            v-if="totalFlappingCount > 0"
            type="button"
            class="stat-pill flapping-btn"
            :class="{ active: statusFilter === 'flapping' }"
            title="点击切换：仅查看检出多出口 IP 漂移或状态波动的节点"
            @click="statusFilter = statusFilter === 'flapping' ? 'all' : 'flapping'"
          >
            ⇄ 漂移节点 <b>{{ totalFlappingCount }}</b>
          </button>
        </div>
      </div>

      <div class="service-node-grid-v5">
        <article
          v-for="row in filteredRows"
          :key="row.key"
          class="service-node-v5"
          :class="{ selected: selected.includes(row.key), 'is-outdated': row.report.latest?.rule.rule_version !== rule?.rule_version && row.report.total > 0 }"
        >
          <header class="card-header-v5">
            <label class="card-node-info">
              <input type="checkbox" :checked="selected.includes(row.key)" :aria-label="`选择 ${row.node.displayName}`" @change="emit('toggle', row.key)">
              <span class="card-flag">{{ row.node.countryFlag || '🌐' }}</span>
              <div class="card-title-group">
                <strong class="card-node-name" :title="row.node.displayName">{{ row.node.displayName }}</strong>
                <div class="card-tags-line">
                  <span v-if="row.node.countryCode" class="card-tag region">{{ row.node.countryCode }}</span>
                  <span class="card-tag protocol">{{ row.node.type || '节点' }}</span>
                  <span v-if="hasFlappingOrDrift(row.report)" class="card-tag flapping" title="检出多出口漂移或状态震荡">⇄ 漂移</span>
                </div>
              </div>
            </label>
            <div class="card-actions-v5">
              <span
                class="outcome-badge-v5"
                :class="tone(row.report.latest)"
                @mouseenter="handleSingleCardEnter($event, row)"
                @mouseleave="hideTooltip"
              >
                <span class="badge-dot"></span>
                {{ row.report.total ? label(row.report.latest) : states[row.key] === 'loading' ? '读取中…' : '未检测' }}
              </span>
              <button
                type="button"
                class="card-retest-btn-v5"
                :disabled="running"
                title="针对此节点立即执行本项检测"
                @click.stop="emit('run', [row.key])"
              >
                ⚡ 检测
              </button>
            </div>
          </header>

          <!-- Card Body -->
          <div v-if="row.report.total" class="card-body-v5">
            <!-- Interactive Trend Sparkline -->
            <div class="card-sparkline-row">
              <span class="sparkline-title">历史走势 (悬浮看对比):</span>
              <InteractiveTrendSparkline
                :points="nodeServiceTrendPoints(row.key)"
                type="service"
                :window="selectedTimeWindow"
                :height="30"
              />
            </div>
            <div class="card-evidence-line">
              <span v-if="row.report.latest?.result?.details?.checked_model" class="pill-model" title="已验证可用模型">
                🤖 {{ row.report.latest.result.details.checked_model }}
              </span>
              <span v-if="row.report.latest?.result?.duration_ms" class="pill-duration">
                ⏱️ {{ row.report.latest.result.duration_ms }} ms
              </span>
              <span v-if="row.report.latest?.rule.rule_version !== rule?.rule_version" class="pill-outdated" title="旧版规则限制了读取体，以新版规则重测可获取完整模型">
                ⚠️ 旧规则 (v{{ row.report.latest?.rule.rule_version }})
              </span>
              <span class="evidence-desc" :title="cleanSummary(row.report.latest?.result, rule)">
                {{ cleanSummary(row.report.latest?.result, rule) }}
              </span>
            </div>

            <div class="card-meta-line">
              <span class="meta-stat">
                通过率 <b>{{ row.report.rate }}%</b> <small>({{ row.report.passed }}/{{ row.report.total }}次)</small>
              </span>
              <span v-if="row.report.changes > 0" class="meta-fluctuation">
                ⚠️ 波动 {{ row.report.changes }} 次
              </span>
              <span class="meta-time">{{ stampRelative(row.report.latest!) }}</span>
              <button type="button" class="btn-evidence-detail" @click="inspected = row.report.latest || null; inspectedNodeKey = row.key">
                详情 →
              </button>
            </div>
          </div>

          <!-- Untested State -->
          <div v-else class="card-untested-v5">
            <span>尚未检测本项服务</span>
            <button type="button" class="btn-quick-run-inline" :disabled="running" @click.stop="emit('run', [row.key])">
              立即探测
            </button>
          </div>
        </article>
      </div>
    </template>

    <p v-if="!(viewMode === 'all' ? filteredOverviewRows.length : filteredRows.length)" class="results-empty">
      {{ selected.length ? `已选 ${selected.length} 条线路，等待开始检测；结果会在这里出现。` : '选择好服务和节点，即可开始检测。' }}
    </p>

    <!-- Centered Modal Inspector with Full History and Drift Analysis -->
    <div v-if="inspected" class="service-inspector-modal-backdrop" @click.self="inspected = null">
      <section class="service-inspector" role="dialog" aria-modal="true" aria-label="服务检测记录">
        <button class="close-inspector" aria-label="关闭检测记录" @click="inspected = null">×</button>
        <div class="inspector-header">
          <div class="inspector-badge-row">
            <span class="inspector-service-badge">{{ inspected.rule?.category || '公网服务' }}</span>
            <span class="inspector-time-tag">检测依据 · {{ stamp(inspected) }}</span>
          </div>
          <h3>{{ inspected.display_name }} · {{ inspected.rule.name }}</h3>
          <div class="inspector-status-row">
            <strong class="outcome" :class="tone(inspected)">{{ label(inspected) }}</strong>
            <button
              v-if="inspectedNodeKey"
              type="button"
              class="inspector-retest-btn"
              :disabled="running"
              @click="handleRetestInspected"
            >
              ⚡ 重新检测此项
            </button>
          </div>
        </div>

        <!-- Flapping / Drift Analysis in Inspector -->
        <div v-if="inspectedDriftInfo.hasDrift" class="inspector-drift-card">
          <div class="drift-header">
            <span class="drift-title-icon">⇄</span>
            <strong>漂移检测分析</strong>
          </div>
          <div v-if="inspectedDriftInfo.distinctIPs.length > 1" class="drift-row">
            <span>检出 {{ inspectedDriftInfo.distinctIPs.length }} 个不同落地出口 IP：</span>
            <div class="drift-ip-tags">
              <code v-for="ip in inspectedDriftInfo.distinctIPs" :key="ip">{{ ip }}</code>
            </div>
          </div>
          <div v-if="inspectedDriftInfo.changes > 0" class="drift-row">
            <span>历史状态发生 {{ inspectedDriftInfo.changes }} 次跳变震荡 (可用率: {{ inspectedDriftInfo.passRate }}%)</span>
          </div>
        </div>

        <p class="inspector-summary-text">{{ inspected.result?.summary || inspected.result?.error_message }}</p>

        <!-- Key Metrics Bar -->
        <div class="inspector-metrics-bar">
          <div class="metric-item">
            <span class="metric-lbl">HTTP 状态</span>
            <strong class="metric-val">{{ inspected.result?.http_status ?? '无响应' }}</strong>
          </div>
          <div class="metric-item">
            <span class="metric-lbl">响应耗时</span>
            <strong class="metric-val">{{ inspected.result?.duration_ms ?? '—' }} ms</strong>
          </div>
          <div class="metric-item">
            <span class="metric-lbl">读取数据量</span>
            <strong class="metric-val">{{ inspected.result?.bytes_read ? `${(inspected.result.bytes_read / 1024).toFixed(1)} KiB` : '—' }}</strong>
          </div>
          <div class="metric-item">
            <span class="metric-lbl">历史总测试</span>
            <strong class="metric-val">{{ inspectedHistory.length }} 次</strong>
          </div>
        </div>

        <dl>
          <div v-for="(value, key) in inspected.result?.details" :key="key">
            <dt>{{ serviceDetailLabel(String(key)) }}</dt>
            <dd>{{ value || '未提供' }}</dd>
          </div>
        </dl>

        <!-- Full History Timeline -->
        <div v-if="inspectedHistory.length > 1" class="inspector-history-section">
          <h4>完整测试历史记录 ({{ inspectedHistory.length }} 次)</h4>
          <div class="inspector-history-table">
            <div
              v-for="(hist, idx) in inspectedHistory"
              :key="hist.attempt_id || idx"
              class="history-row-item"
              :class="{ 'is-current': hist.attempt_id === inspected.attempt_id }"
            >
              <div class="hist-time">{{ stamp(hist) }}</div>
              <div class="hist-badge">
                <span class="hist-dot" :class="tone(hist)"></span>
                <span :class="tone(hist)">{{ label(hist) }}</span>
              </div>
              <div class="hist-dur">{{ hist.result?.duration_ms ? `${hist.result.duration_ms} ms` : '—' }}</div>
              <div class="hist-http">HTTP {{ hist.result?.http_status ?? '—' }}</div>
              <div class="hist-ip">{{ hist.result?.details?.ip || '—' }}</div>
              <div class="hist-summary" :title="hist.result?.summary || hist.result?.error_message || ''">
                {{ hist.result?.summary || hist.result?.error_message || '—' }}
              </div>
            </div>
          </div>
        </div>

        <details>
          <summary>技术信息与保存状态</summary>
          <p>HTTP {{ inspected.result?.http_status ?? '无响应' }} · {{ inspected.result?.duration_ms ?? '—' }} ms · {{ inspected.persistence_state === 'saved' ? '已保存' : '尚未保存' }}</p>
          <p>{{ inspected.rule.success_criterion }}</p>
          <p>{{ inspected.persistence_error }}</p>
        </details>
      </section>
    </div>

    <!-- Hover Tooltip Popover -->
    <ServiceHistoryTooltip
      :visible="tooltipState.visible"
      :x="tooltipState.x"
      :y="tooltipState.y"
      :service-name="tooltipState.serviceName"
      :service-id="tooltipState.serviceId"
      :node-name="tooltipState.nodeName"
      :node-flag="tooltipState.nodeFlag"
      :country-code="tooltipState.countryCode"
      :history="tooltipState.history"
      :rule-evidence="tooltipState.ruleEvidence"
    />
  </div>
</template>

<style scoped>
.service-results {
  padding: 16px 20px;
}
.service-nav-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border);
}
.service-nav-left {
  display: flex;
  align-items: center;
  gap: 10px;
}
.nav-back-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border-radius: 7px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
}
.nav-back-btn.primary {
  background: var(--card-subtle);
  border: 1px solid var(--border);
  color: var(--primary);
}
.nav-back-btn.primary:hover {
  background: var(--primary);
  border-color: var(--primary);
  color: white;
  transform: translateX(-2px);
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.25);
}
.nav-back-btn.subtle {
  background: transparent;
  border: 1px dashed var(--border);
  color: var(--text-secondary);
}
.nav-back-btn.subtle:hover {
  border-color: var(--primary);
  color: var(--primary);
}
.nav-crumb-separator {
  color: var(--text-muted);
  font-size: 13px;
}
.service-count-chip {
  font-size: 11px;
  padding: 3px 9px;
  border-radius: 999px;
  background: var(--card-subtle);
  color: var(--text-secondary);
  border: 1px solid var(--border);
}
.heading-badge-line {
  display: flex;
  align-items: center;
  gap: 8px;
}
.service-rule-badge {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  background: rgba(99, 102, 241, 0.12);
  color: var(--primary);
  font-weight: 700;
}
.results-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.results-heading > div > span {
  font-size: 11px;
  letter-spacing: 1px;
  color: var(--primary);
  font-weight: 750;
}
.results-heading h3 {
  font-size: 24px;
  margin: 6px 0;
  font-weight: 750;
}
.results-heading p {
  font-size: 12px;
  color: var(--text-secondary);
  max-width: 620px;
  line-height: 1.6;
}
.results-tools {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-left: auto;
}
.search-input-group {
  position: relative;
  display: inline-flex;
  align-items: center;
}
.search-input-group .search-icon {
  position: absolute;
  left: 9px;
  font-size: 12px;
  pointer-events: none;
  opacity: 0.6;
}
.search-input-group .service-search-input {
  padding: 6px 26px 6px 28px;
  font-size: 12px;
  border-radius: 7px;
  border: 1px solid var(--border);
  background: var(--card-subtle);
  color: var(--text-main);
  width: 170px;
  transition: all 0.15s ease;
}
.search-input-group .service-search-input:focus {
  outline: none;
  border-color: var(--primary);
  background: var(--card-bg);
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2);
}
.search-clear-btn {
  position: absolute;
  right: 6px;
  width: 16px;
  height: 16px;
  border: none;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
}
.search-clear-btn:hover {
  color: var(--text-main);
}
.tool-select-group {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.tool-label {
  font-size: 11.5px;
  color: var(--text-secondary);
  font-weight: 550;
}
.custom-toggle-label {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 8px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--card-subtle);
  font-size: 11.5px;
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  transition: all 0.15s ease;
}
.custom-toggle-label:hover {
  background: var(--card-bg);
  border-color: var(--border-focus);
}
.custom-toggle-label.active {
  background: rgba(99, 102, 241, 0.1);
  border-color: var(--primary);
  color: var(--primary);
  font-weight: 600;
}
.batch-action-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tool-btn {
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--card-bg);
  color: var(--text-primary);
  cursor: pointer;
  transition: all 0.15s ease;
}
.tool-btn:hover:not(:disabled) {
  border-color: var(--primary);
  background: var(--primary-subtle);
}
.tool-btn.primary {
  background: var(--primary);
  border-color: var(--primary);
  color: white;
}
.tool-btn.primary:hover:not(:disabled) {
  opacity: 0.9;
}
.tool-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

/* Outdated rule notice banner */
.outdated-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 18px;
  margin: 0 0 16px;
  background: #fff8e6;
  border: 1px solid #ffd591;
  border-radius: 10px;
}
:root.dark .outdated-banner {
  background: #2b2111;
  border-color: #594214;
}
.outdated-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.outdated-tag-line {
  display: flex;
  align-items: center;
  gap: 8px;
}
.outdated-tag {
  font-size: 11px;
  padding: 2px 6px;
  background: #ffe58f;
  color: #874d00;
  font-weight: 700;
  border-radius: 4px;
}
:root.dark .outdated-tag {
  background: #594214;
  color: #ffd591;
}
.outdated-banner strong {
  font-size: 13px;
  color: var(--text-primary);
}
.outdated-banner p {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0;
  line-height: 1.5;
}
.outdated-retest-btn {
  padding: 8px 16px;
  font-size: 12px;
  font-weight: 700;
  border-radius: 7px;
  background: #fa8c16;
  color: white;
  border: none;
  cursor: pointer;
  white-space: nowrap;
  flex-shrink: 0;
  transition: opacity 0.15s ease;
}
.outdated-retest-btn:hover:not(:disabled) {
  opacity: 0.9;
}
.outdated-retest-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.service-search-input {
  padding: 5px 10px;
  font-size: 12px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--card-bg);
  color: var(--text-primary);
  width: 170px;
  transition: all 0.15s ease;
}
.service-search-input:focus {
  outline: none;
  border-color: var(--primary);
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2);
}

.result-summary-v5 {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 6px 0 12px;
}
.summary-stat-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
.stat-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 9px;
  border-radius: 6px;
  background: var(--card-subtle);
  border: 1px solid var(--border);
  font-size: 11.5px;
  color: var(--text-secondary);
}
.stat-pill b {
  color: var(--text-primary);
}
.stat-pill.warn {
  color: #f59e0b;
  border-color: rgba(245, 158, 11, 0.3);
  background: rgba(245, 158, 11, 0.08);
}
.stat-pill.warn b {
  color: #f59e0b;
}

.service-node-grid-v5 {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 10px;
}

.service-node-v5 {
  border: 1px solid var(--border);
  border-radius: 9px;
  padding: 10px 14px;
  background: var(--card-bg);
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.service-node-v5:hover {
  border-color: rgba(99, 102, 241, 0.4);
}
.service-node-v5.selected {
  border-color: var(--primary);
  box-shadow: inset 3px 0 var(--primary);
  background: rgba(99, 102, 241, 0.03);
}
.service-node-v5.is-outdated {
  border-left: 3px solid #fa8c16;
}

.card-header-v5 {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
}
.card-node-info {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  cursor: pointer;
  flex: 1;
}
.card-flag {
  font-size: 15px;
  line-height: 1;
}
.card-title-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.card-node-name {
  font-size: 13px;
  font-weight: 650;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
}
.card-tags-line {
  display: flex;
  align-items: center;
  gap: 4px;
}
.card-tag {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  font-weight: 550;
  line-height: 1.2;
}
.card-tag.region {
  background: rgba(14, 165, 233, 0.12);
  color: #0284c7;
}
.card-tag.protocol {
  background: rgba(99, 102, 241, 0.1);
  color: #6366f1;
}
:root.dark .card-tag.region {
  color: #38bdf8;
}

.card-actions-v5 {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.outcome-badge-v5 {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  font-weight: 600;
  padding: 3px 8px;
  border-radius: 999px;
  white-space: nowrap;
}
.outcome-badge-v5 .badge-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.outcome-badge-v5.good {
  background: rgba(16, 185, 129, 0.12);
  color: #10b981;
}
.outcome-badge-v5.good .badge-dot {
  background: #10b981;
  box-shadow: 0 0 4px #10b981;
}
.outcome-badge-v5.warn,
.outcome-badge-v5.limited {
  background: rgba(245, 158, 11, 0.12);
  color: #f59e0b;
}
.outcome-badge-v5.warn .badge-dot,
.outcome-badge-v5.limited .badge-dot {
  background: #f59e0b;
}
.outcome-badge-v5.bad {
  background: rgba(239, 68, 68, 0.12);
  color: #ef4444;
}
.outcome-badge-v5.bad .badge-dot {
  background: #ef4444;
}
.outcome-badge-v5.unknown {
  background: var(--card-subtle);
  color: var(--text-secondary);
}
.outcome-badge-v5.unknown .badge-dot {
  background: var(--text-muted);
}

.card-retest-btn-v5 {
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 600;
  border-radius: 5px;
  border: 1px solid var(--border);
  background: var(--card-subtle);
  color: var(--text-primary);
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.card-retest-btn-v5:hover:not(:disabled) {
  border-color: var(--primary);
  background: var(--primary);
  color: white;
}
.card-retest-btn-v5:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.card-body-v5 {
  display: flex;
  flex-direction: column;
  gap: 5px;
  border-top: 1px dashed var(--border);
  padding-top: 6px;
}
.card-evidence-line {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pill-model {
  display: inline-flex;
  align-items: center;
  font-size: 10.5px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(99, 102, 241, 0.12);
  color: #6366f1;
  flex-shrink: 0;
}
.pill-duration {
  font-size: 10.5px;
  font-family: monospace;
  color: var(--text-secondary);
  flex-shrink: 0;
}
.pill-outdated {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  background: #ffe58f;
  color: #874d00;
  flex-shrink: 0;
}
:root.dark .pill-outdated {
  background: #594214;
  color: #ffd591;
}
.evidence-desc {
  font-size: 11px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-meta-line {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 10.5px;
  color: var(--text-secondary);
}
.meta-stat b {
  color: var(--text-primary);
}
.meta-fluctuation {
  color: #f59e0b;
  font-weight: 600;
}
.meta-time {
  margin-left: auto;
}
.btn-evidence-detail {
  background: none;
  border: none;
  color: var(--primary);
  font-size: 10.5px;
  cursor: pointer;
  padding: 0;
}
.btn-evidence-detail:hover {
  text-decoration: underline;
}

.card-untested-v5 {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 6px;
  border-top: 1px dashed var(--border);
  font-size: 11px;
  color: var(--text-secondary);
}
.btn-quick-run-inline {
  background: none;
  border: 1px dashed var(--border);
  border-radius: 4px;
  padding: 2px 7px;
  font-size: 10.5px;
  color: var(--primary);
  cursor: pointer;
  transition: all 0.15s ease;
}
.btn-quick-run-inline:hover:not(:disabled) {
  background: var(--primary-subtle);
  border-color: var(--primary);
}

/* Modal Dialog Backdrop & Centered Inspector */
.service-inspector-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 99999;
  background: rgba(15, 23, 42, 0.6);
  backdrop-filter: blur(5px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  animation: fadeInBackdrop 0.15s ease-out;
}

@keyframes fadeInBackdrop {
  from { opacity: 0; }
  to { opacity: 1; }
}

.service-inspector {
  position: relative;
  width: 740px;
  max-width: 95vw;
  max-height: 88vh;
  overflow-y: auto;
  padding: 24px;
  background: var(--card-bg, #ffffff);
  border: 1px solid var(--border-focus, #3b82f6);
  border-radius: 14px;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.35), 0 0 0 1px rgba(0, 0, 0, 0.05);
  animation: modalSlideUp 0.18s cubic-bezier(0.16, 1, 0.3, 1);
}

:global(.dark) .service-inspector {
  background: #18202f;
  border-color: #3b82f6;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.7);
}

@keyframes modalSlideUp {
  from { opacity: 0; transform: translateY(16px) scale(0.98); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}

.inspector-header {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 12px;
}
.inspector-badge-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.inspector-service-badge {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 4px;
  background: rgba(59, 130, 246, 0.12);
  color: #2563eb;
}
:global(.dark) .inspector-service-badge {
  background: rgba(96, 165, 250, 0.2);
  color: #93c5fd;
}
.inspector-time-tag {
  font-size: 11px;
  color: var(--text-secondary);
}
.service-inspector h3 {
  font-size: 19px;
  font-weight: 750;
  margin: 4px 0;
  color: var(--text-primary);
}
.inspector-status-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 4px;
}
.inspector-retest-btn {
  padding: 4px 12px;
  font-size: 12px;
  font-weight: 600;
  border-radius: 6px;
  border: 1px solid var(--primary);
  background: var(--primary);
  color: #ffffff;
  cursor: pointer;
  transition: all 0.15s ease;
}
.inspector-retest-btn:hover:not(:disabled) {
  opacity: 0.9;
  transform: translateY(-1px);
}
.close-inspector {
  position: absolute;
  right: 18px;
  top: 16px;
  font-size: 26px;
  line-height: 1;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-secondary);
  border-radius: 6px;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.close-inspector:hover {
  background: rgba(0, 0, 0, 0.06);
  color: var(--text-primary);
}
:global(.dark) .close-inspector:hover {
  background: rgba(255, 255, 255, 0.1);
}

/* Flapping & Drift Card in Inspector */
.inspector-drift-card {
  margin: 12px 0;
  padding: 12px 14px;
  border-radius: 8px;
  background: #fffbeb;
  border: 1px solid #fde68a;
  color: #92400e;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
:global(.dark) .inspector-drift-card {
  background: #2b2214;
  border-color: #78350f;
  color: #fde68a;
}
.drift-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}
.drift-title-icon {
  font-weight: 800;
  font-size: 14px;
}
.drift-row {
  font-size: 12px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}
.drift-ip-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.drift-ip-tags code {
  padding: 2px 6px;
  border-radius: 4px;
  background: #fef3c7;
  border: 1px solid #fde68a;
  font-family: monospace;
  font-size: 11px;
}
:global(.dark) .drift-ip-tags code {
  background: #451a03;
  border-color: #92400e;
}

.inspector-summary-text {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 10px 0;
  line-height: 1.6;
}

/* Metrics Bar */
.inspector-metrics-bar {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  margin: 14px 0;
  padding: 10px;
  background: var(--card-subtle);
  border-radius: 8px;
}
.metric-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.metric-lbl {
  font-size: 10.5px;
  color: var(--text-secondary);
}
.metric-val {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary);
}

.service-inspector dl {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px;
  margin: 15px 0;
}
.service-inspector dl div {
  padding: 10px;
  background: var(--card-subtle);
  border-radius: 7px;
}
.service-inspector dt {
  font-size: 10.5px;
  color: var(--text-secondary);
}
.service-inspector dd {
  font-size: 12px;
  overflow-wrap: anywhere;
  margin: 5px 0 0;
}

/* History Timeline Table */
.inspector-history-section {
  margin: 18px 0 10px;
  border-top: 1px dashed var(--border);
  padding-top: 14px;
}
.inspector-history-section h4 {
  font-size: 13px;
  font-weight: 700;
  margin: 0 0 10px;
  color: var(--text-primary);
}
.inspector-history-table {
  display: flex;
  flex-direction: column;
  gap: 5px;
  max-height: 220px;
  overflow-y: auto;
}
.history-row-item {
  display: grid;
  grid-template-columns: 140px 90px 70px 80px 110px 1fr;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  background: var(--card-subtle);
  font-size: 11px;
}
.history-row-item.is-current {
  background: rgba(59, 130, 246, 0.08);
  border: 1px solid rgba(59, 130, 246, 0.3);
}
.hist-time {
  font-size: 10.5px;
  color: var(--text-secondary);
}
.hist-badge {
  display: flex;
  align-items: center;
  gap: 5px;
  font-weight: 600;
}
.hist-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.hist-dot.good { background: #10b981; }
.hist-dot.bad { background: #ef4444; }
.hist-dot.limited { background: #f59e0b; }
.hist-dur {
  font-weight: 700;
  font-family: monospace;
}
.hist-http {
  color: var(--text-secondary);
  font-family: monospace;
}
.hist-ip {
  color: var(--text-secondary);
  font-family: monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hist-summary {
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.service-inspector summary {
  font-size: 12px;
  cursor: pointer;
  margin-top: 10px;
  color: var(--text-secondary);
}

/* Flapping Badge & Filter Styles */
.flapping-btn {
  background: #fef3c7;
  color: #92400e;
  border: 1px solid #fde68a;
  cursor: pointer;
}
.flapping-btn.active {
  background: #f59e0b;
  color: #ffffff;
  border-color: #d97706;
}
:global(.dark) .flapping-btn {
  background: #38260f;
  color: #fde68a;
  border-color: #78350f;
}
:global(.dark) .flapping-btn.active {
  background: #d97706;
  color: #ffffff;
}

.node-title-line {
  display: flex;
  align-items: center;
  gap: 6px;
}
.node-drift-badge {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 4px;
  background: #fef3c7;
  color: #b45309;
  border: 1px solid #fde68a;
}
:global(.dark) .node-drift-badge {
  background: #451a03;
  color: #fcd34d;
  border-color: #92400e;
}

.pill-flapping-tag {
  font-size: 9px;
  font-weight: 800;
  padding: 0 3px;
  border-radius: 3px;
  background: #fef3c7;
  color: #b45309;
  border: 1px solid #fcd34d;
  margin-left: 2px;
}
:global(.dark) .pill-flapping-tag {
  background: #451a03;
  color: #fcd34d;
  border-color: #92400e;
}

.card-tag.flapping {
  background: #fef3c7;
  color: #b45309;
  border: 1px solid #fde68a;
  font-weight: 700;
}
:global(.dark) .card-tag.flapping {
  background: #451a03;
  color: #fcd34d;
  border-color: #92400e;
}


/* Overview Mode Styles */
.overview-status {
  display: flex;
  gap: 15px;
  align-items: center;
  padding: 14px 2px;
  font-size: 13px;
  font-weight: 750;
}
.overview-status small {
  font-size: 11px;
  color: var(--text-secondary);
  font-weight: 400;
}
.service-overview {
  max-height: 650px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 11px;
}
.overview-row-v5 {
  display: grid;
  grid-template-columns: minmax(180px, 240px) 1fr;
  gap: 12px;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
}
.overview-row-v5:last-child {
  border-bottom: 0;
}
.overview-node-label {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  cursor: pointer;
}
.overview-flag {
  font-size: 14px;
}
.overview-node-text {
  min-width: 0;
}
.overview-node-text strong {
  display: block;
  font-size: 12.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.overview-node-text small {
  display: block;
  color: var(--text-secondary);
  font-size: 10.5px;
}

.overview-services-v5 {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.overview-service-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--card-bg);
  font-size: 11px;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
}
.overview-service-pill:hover {
  transform: translateY(-1px);
  border-color: var(--primary);
}
.overview-service-pill .pill-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
}
.overview-service-pill.good {
  border-color: rgba(16, 185, 129, 0.3);
  background: rgba(16, 185, 129, 0.08);
  color: #10b981;
}
.overview-service-pill.good .pill-dot {
  background: #10b981;
}
.overview-service-pill.warn,
.overview-service-pill.limited {
  border-color: rgba(245, 158, 11, 0.3);
  background: rgba(245, 158, 11, 0.08);
  color: #f59e0b;
}
.overview-service-pill.warn .pill-dot,
.overview-service-pill.limited .pill-dot {
  background: #f59e0b;
}
.overview-service-pill.bad {
  border-color: rgba(239, 68, 68, 0.25);
  background: rgba(239, 68, 68, 0.08);
  color: #ef4444;
}
.overview-service-pill.bad .pill-dot {
  background: #ef4444;
}
.overview-service-pill.unknown,
.overview-service-pill.untested {
  color: var(--text-muted);
  opacity: 0.7;
}
.overview-service-pill.unknown .pill-dot,
.overview-service-pill.untested .pill-dot {
  background: var(--text-muted);
}
.overview-service-pill .svc-name {
  color: var(--text-secondary);
}
.overview-service-pill .svc-dur {
  color: var(--text-muted);
  font-size: 9.5px;
  font-family: monospace;
}
.node-quick-test-btn {
  padding: 4px 8px;
  font-size: 11px;
  font-weight: 600;
  border-radius: 5px;
  border: 1px solid var(--border);
  background: var(--card-subtle);
  color: var(--text-primary);
  cursor: pointer;
  margin-left: auto;
  transition: all 0.15s ease;
}
.node-quick-test-btn:hover:not(:disabled) {
  border-color: var(--primary);
  background: var(--primary-subtle);
  color: var(--primary);
}

@media (max-width: 900px) {
  .results-tools {
    width: 100%;
    margin-left: 0;
  }
  .batch-action-group {
    width: 100%;
  }
  .tool-btn {
    flex: 1;
    text-align: center;
  }
}
@media (max-width: 700px) {
  .service-results {
    padding: 14px;
  }
  .service-node-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .result-summary {
    flex-wrap: wrap;
    gap: 12px;
  }
  .result-summary p {
    margin-left: 0;
  }
  .overview-row {
    display: block;
  }
  .overview-services {
    margin-top: 8px;
  }
  .outdated-banner {
    flex-direction: column;
    align-items: stretch;
  }
  .outdated-retest-btn {
    width: 100%;
  }
}

.service-nav-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  padding: 10px 14px;
  background: var(--card-bg, #ffffff);
  border: 1px solid var(--border, #e2e8f0);
  border-radius: 9px;
  margin-bottom: 14px;
}
.back-to-picker-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--card-subtle, #f8fafc);
  border: 1px solid var(--border, #cbd5e1);
  color: var(--text-main, #0f172a);
  font-size: 13px;
  font-weight: 600;
  padding: 7px 14px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.back-to-picker-btn:hover {
  background: var(--primary-subtle, #eff6ff);
  border-color: var(--primary, #3b82f6);
  color: var(--primary, #3b82f6);
}
.back-arrow {
  font-size: 15px;
}
.window-filter-group {
  display: flex;
  align-items: center;
  gap: 6px;
}
.window-filter-label {
  font-size: 12px;
  color: var(--text-secondary, #64748b);
  font-weight: 500;
}
.window-pill {
  padding: 4px 10px;
  font-size: 12px;
  border-radius: 5px;
  border: 1px solid var(--border, #cbd5e1);
  background: var(--card-bg, #ffffff);
  color: var(--text-secondary, #64748b);
  cursor: pointer;
  transition: all 0.12s ease;
}
.window-pill:hover {
  border-color: var(--primary, #3b82f6);
  color: var(--primary, #3b82f6);
}
.window-pill.active {
  background: var(--primary, #3b82f6);
  border-color: var(--primary, #3b82f6);
  color: #ffffff;
  font-weight: 600;
}
.card-sparkline-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 8px;
  background: var(--card-subtle, #f8fafc);
  border-radius: 6px;
  margin-bottom: 6px;
  border: 1px solid var(--border, #e2e8f0);
}
.sparkline-title {
  font-size: 11px;
  color: var(--text-secondary, #64748b);
  white-space: nowrap;
  font-weight: 500;
}

</style>
