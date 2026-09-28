<script setup lang="ts">
import { serviceOutcomeLabel } from "../../utils/serviceOutcome"
import { baselineLatencyTest, latencySiteResults, suiteHealth } from '../../utils/latencyTargets'
import MultiSiteLatencyTrend from './MultiSiteLatencyTrend.vue'
import AppIcon from '../common/AppIcon.vue'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { fetchMonitorNodeOptions, fetchMonitorJobs } from '../../api/monitor'
import * as api from '../../api/bridge'
import UiSelect, { type UiSelectOption } from '../common/UiSelect.vue'
import LatencySamplePlot from './LatencySamplePlot.vue'
import MetricHistoryPlot, { type MetricHistoryPoint } from './MetricHistoryPlot.vue'
import ServiceComparison from './ServiceComparison.vue'
import ServiceCatalogPicker from './ServiceCatalogPicker.vue'
import TestPlanDialog, { type TestPlan } from './TestPlanDialog.vue'
import { serviceTitle } from '../../utils/servicePresentation'
import DownloadComparison from './DownloadComparison.vue'
import ExportClashModal from './ExportClashModal.vue'
import ObservationWindowSelect from './ObservationWindowSelect.vue'
import { readObservationPreference, saveObservationPreference } from './observationPreference'
import IntraTestSamplePlot from './IntraTestSamplePlot.vue'
import WorkbenchPublicServicePanel from './WorkbenchPublicServicePanel.vue'
import WorkbenchDownloadPanel from './WorkbenchDownloadPanel.vue'
import { compareLatencyAttempts, computeNodeHealthReport, type LatencyAttemptComparison, type NodeHealthReport } from './nodeHealth'
import {
  extractNoticeLinks,
  isNoticeNode,
  loadNoticeOverrides,
  saveNoticeOverrides,
} from '../../utils/nodeFilter'
import { buildLogicalProfileChoices, logicalChoiceForProfile, type LogicalProfileChoice } from '../../utils/logicalProfiles'
import type { TimeBucket } from './latencyLod'
import {
  acceptsLatencyDetailResponse,
  freezeLatencyWindow,
  sameLatencyScope,
  type LatencyAttemptScope,
  type LatencyScope,
  type LatencyWindow,
  type LatencyWindowMode,
} from './latencyRequestGuard'
import type {
  MonitorNodeOption,
  MonitorJobPrefill,
  NodeDetailOrigin,
  NodeDetailRequest,
  WorkbenchDownloadAttempt,
  WorkbenchDownloadHistoryQuery,
  WorkbenchLatencyBatch,
  WorkbenchLatencyBatchItem,
  WorkbenchLatencySample,
  WorkbenchLatencyTest,
  WorkbenchPublicServiceAttempt,
  WorkbenchPublicServiceHistoryQuery,
  WorkbenchPublicServiceRule,
  WorkbenchSaveRetryRequest,
} from '../../types'
import { decideMonitorSelection } from './monitorCreateSelection'

type WorkbenchProject = 'latency' | 'throughput' | 'service'
type IndexMap = Record<string, number | null | undefined>
type HistoryMeta = { hasMore: boolean; complete: boolean }
type HistoryLoadState = 'loading' | 'ready' | 'error'

interface NodeDownloadSummary {
  speedMbps: number
  durationMs: number
  bytes: number
  timestamp: string
  outcome?: string
}

interface NodeServiceSummary {
  outcome: string
  durationMs: number
  httpStatus?: number
  timestamp: string
}

const props = defineProps<{ saveRetryRequest?: WorkbenchSaveRetryRequest | null; initialProfileId?: string; visible?: boolean }>()
const emit = defineEmits<{
  (event: 'open-monitor', payload: MonitorJobPrefill): void
  (event: 'open-node-detail', payload: NodeDetailRequest): void
  (event: 'save-retry-request-resolved', attemptID: string): void
}>()
const pendingSaveRetry = ref<WorkbenchSaveRetryRequest | null>(null)
const options = ref<MonitorNodeOption[]>([])
const optionsLoading = ref(false)
const optionsError = ref('')
const historyLoading = ref(false)
const historyError = ref('')
const selectedProfileId = ref(props.initialProfileId || 'all')
const selectedProfileIds = ref<string[]>(props.initialProfileId && props.initialProfileId !== 'all' ? [props.initialProfileId] : [])
const profilePickerOpen = ref(false)
function onProfilePickerToggle(event: Event): void {
  profilePickerOpen.value = (event.currentTarget as HTMLDetailsElement).open
}
function setProfileScope(value: string): void {
  if (value === 'multiple') return
  const logical = logicalChoiceForProfile(logicalProfileChoices.value, value)
  selectedProfileIds.value = value === 'all' ? [] : logical?.profileIds || [value]
  selectedProfileId.value = logical?.id || value
}
function profileInScope(profileId: string): boolean {
  return selectedProfileIds.value.length === 0 || selectedProfileIds.value.includes(profileId)
}
watch(() => props.initialProfileId, id => { if (id) setProfileScope(id) })
const selectedRegion = ref('all')
const searchText = ref('')
const serviceNodeRegion = ref('全部地区')
const serviceNodeSearch = ref('')
const sortBy = ref<'region' | 'recent' | 'p50' | 'p95' | 'health' | 'name'>('region')
const windowMode = ref<LatencyWindowMode>(readObservationPreference('selected'))
const activeWindow = ref<LatencyWindow>(freezeLatencyWindow(windowMode.value))
const activeProject = ref<WorkbenchProject>('latency')
const nodeCategoryFilter = ref<'proxies' | 'notices' | 'all'>('proxies')
const isExportModalOpen = ref(false)
const workbenchViewMode = ref<'compact' | 'detailed'>(
  typeof localStorage !== 'undefined' && localStorage.getItem('cst_workbench_view_mode') === 'compact'
    ? 'compact'
    : 'detailed'
)
function setWorkbenchViewMode(mode: 'compact' | 'detailed') {
  workbenchViewMode.value = mode
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('cst_workbench_view_mode', mode)
  }
}
const expandedKeys = ref<Set<string>>(new Set())
function toggleRowExpanded(key: string) {
  if (expandedKeys.value.has(key)) {
    expandedKeys.value.delete(key)
  } else {
    expandedKeys.value.add(key)
  }
}
const concurrencyLevel = ref<number>(16)
const filterOnlyAlive = ref<boolean>(false)

function compactLatencyClass(key: string): string {
  const test = latestTestForKey(key)
  if (!test) return 'nodata'
  if (test.status === 'failed' || test.latency_ms <= 0) return 'fail'
  if (test.latency_ms < 120) return 'fast'
  if (test.latency_ms < 250) return 'medium'
  return 'slow'
}

const noticeOverrides = ref<Record<string, boolean>>(loadNoticeOverrides())
const selectedKeys = ref<string[]>([])
const latencyTargetID = ref('all')
const topNodePickerOpen = ref(false)
const topPickerNodes = computed(() => visibleOptions.value.filter(node => !isNoticeNode(node, noticeOverrides.value, scopeKey(node))))
const topPickerAllSelected = computed(() => topPickerNodes.value.length > 0 && topPickerNodes.value.every(node => selectedKeys.value.includes(scopeKey(node))))
function toggleTopPickerAll() {
  const keys = new Set(topPickerNodes.value.map(scopeKey))
  selectedKeys.value = topPickerAllSelected.value ? selectedKeys.value.filter(key => !keys.has(key)) : [...new Set([...selectedKeys.value, ...keys])]
}
function matchesLatencyTarget(_test: WorkbenchLatencyTest) { return true }
const showAlternateConfigs = ref(false)
const focusedKey = ref('')
const hoveredByKey = ref<IndexMap>({})
const pinnedByKey = ref<IndexMap>({})
const hoveredBucketByKey = ref<Record<string, TimeBucket | null>>({})
const historyByKey = ref<Record<string, WorkbenchLatencyTest[]>>({})
const historyMetaByKey = ref<Record<string, HistoryMeta>>({})
const historyLoadStateByKey = ref<Record<string, HistoryLoadState>>({})
const downloadHistoryByKey = ref<Record<string, WorkbenchDownloadAttempt[]>>({})
const serviceHistoryByKey = ref<Record<string, WorkbenchPublicServiceAttempt[]>>({})
const projectHistoryStateByKey = ref<Record<string, HistoryLoadState>>({})
const projectHistoryMetaByKey = ref<Record<string, HistoryMeta>>({})
const projectHoveredByKey = ref<IndexMap>({})
const projectPinnedByKey = ref<IndexMap>({})
let projectHistoryRequestID = 0
const currentByKey = ref<Record<string, WorkbenchLatencyTest | undefined>>({})
const expanded = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
const detailTest = ref<WorkbenchLatencyTest | null>(null)
const selectedAttemptID = ref('')
const testError = ref('')
const batchMessage = ref('')
const batchStarting = ref(false)
const activeBatch = ref<WorkbenchLatencyBatch | null>(null)
const displayedBatch = ref<WorkbenchLatencyBatch | null>(null)
const batchView = ref<'all' | 'attention' | 'finished'>('all')
const batchItems = computed(() => displayedBatch.value?.items || [])
function batchItemEnded(item: NonNullable<WorkbenchLatencyBatch['items']>[number]) {
  return !['queued', 'running'].includes(item.execution_state)
}
function batchItemNeedsAttention(item: NonNullable<WorkbenchLatencyBatch['items']>[number]) {
  return item.persistence_state === 'failed' || ['failed', 'skipped_config', 'interrupted'].includes(item.execution_state) || (item.result?.failure_samples || 0) > 0
}
const batchCounts = computed(() => ({
  ended: batchItems.value.filter(batchItemEnded).length,
  running: batchItems.value.filter(item => item.execution_state === 'running').length,
  queued: batchItems.value.filter(item => item.execution_state === 'queued').length,
  saved: batchItems.value.filter(item => item.persistence_state === 'saved').length,
  measuredPassed: batchItems.value.filter(item => item.persistence_state === 'saved' && item.execution_state === 'completed').length,
  measuredFailed: batchItems.value.filter(item => item.persistence_state === 'saved' && item.execution_state === 'failed').length,
  interrupted: batchItems.value.filter(item => item.execution_state === 'interrupted').length,
  saveFailed: batchItems.value.filter(item => item.persistence_state === 'failed').length,
  attention: batchItems.value.filter(batchItemNeedsAttention).length,
  saving: batchItems.value.filter(item => item.persistence_state === 'saving').length,
}))
const batchProgress = computed(() => Math.min(100, Math.round(batchCounts.value.ended / Math.max(1, displayedBatch.value?.item_count || 0) * 100)))
const visibleBatchItems = computed(() => {
  const items = batchItems.value.filter(item => batchView.value === 'attention' ? batchItemNeedsAttention(item) : batchView.value === 'finished' ? batchItemEnded(item) : true)
  const rank = (item: typeof items[number]) => item.execution_state === 'running' || item.persistence_state === 'saving' ? 0 : batchItemNeedsAttention(item) ? 1 : item.execution_state === 'queued' ? 2 : 3
  return [...items].sort((a, b) => rank(a) - rank(b))
})
const recentBatches = ref<WorkbenchLatencyBatch[]>([])
const latestBatchDetail = ref<WorkbenchLatencyBatch | null>(null)
const batchHistoryError = ref('')
const timeoutSeconds = ref(5)

const customSampleCount = ref<number>((() => {
  try {
    const saved = typeof localStorage !== 'undefined' ? localStorage.getItem('clash-speedtest-custom-sample-count') : null
    const num = saved ? Number.parseInt(saved, 10) : 10
    return Number.isFinite(num) && num >= 1 && num <= 50 ? num : 10
  } catch {
    return 10
  }
})())

const sampleCount = ref<number>((() => {
  try {
    const saved = typeof localStorage !== 'undefined' ? localStorage.getItem('clash-speedtest-sample-count') : null
    const num = saved ? Number.parseInt(saved, 10) : 6
    return Number.isFinite(num) && num >= 1 && num <= 50 ? num : 6
  } catch {
    return 6
  }
})())

watch(sampleCount, (val) => {
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('clash-speedtest-sample-count', String(val))
      if (val !== 6) {
        customSampleCount.value = val
        localStorage.setItem('clash-speedtest-custom-sample-count', String(val))
      }
    }
  } catch {}
})

watch(customSampleCount, (val) => {
  if (val >= 1 && val <= 50) {
    try {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem('clash-speedtest-custom-sample-count', String(val))
      }
    } catch {}
    if (sampleCount.value !== 6) {
      sampleCount.value = val
    }
  }
})

const sampleCountSelectOptions = computed<UiSelectOption[]>(() => {
  const customVal = customSampleCount.value === 6 ? 10 : customSampleCount.value
  return [
    { value: 6, label: '6 次（默认）' },
    { value: customVal, label: `自定义 (${customVal} 次)` },
  ]
})

const barDownloadRepeatCount = ref(1)
const barDownloadMaxMiB = ref(20)
const barDownloadTimeout = ref(10)

const barServiceId = ref('cloudflare_204')
const selectedServiceIds = ref<string[]>(['cloudflare_204', 'google_204', 'github_api_root'])
const selectedServiceOptions = computed(() => barServiceOptions.value.filter(s => selectedServiceIds.value.includes(String(s.value))))
const testPlan = ref<TestPlan | null>(null)
let resolveTestPlan: ((confirmed: boolean) => void) | null = null
function confirmTestPlan(plan: TestPlan): Promise<boolean> {
  if (testPlan.value) return Promise.resolve(false)
  testPlan.value = plan
  return new Promise(resolve => { resolveTestPlan = resolve })
}
function answerTestPlan(confirmed: boolean) {
  testPlan.value = null
  resolveTestPlan?.(confirmed)
  resolveTestPlan = null
}
onUnmounted(() => answerTestPlan(false))
const barServiceTimeout = ref(10)
const barServiceRepeatCount = ref(1)
const publicServiceCatalog = ref<WorkbenchPublicServiceRule[]>([])

const barServiceOptions = computed<UiSelectOption[]>(() => publicServiceCatalog.value.length
  ? publicServiceCatalog.value.map(rule => ({ value: rule.service_id, label: `${rule.category || '服务'} · ${rule.name}` }))
  : [
      { value: 'cloudflare_204', label: '基础网络 · Cloudflare 204' },
      { value: 'google_204', label: '基础网络 · Google 204' },
      { value: 'github_api_root', label: '开发服务 · GitHub API' },
      { value: 'antigravity', label: 'AI 服务 · Google Antigravity' },
    ])

function publicServiceName(serviceId: string): string {
  return publicServiceCatalog.value.find(rule => rule.service_id === serviceId)?.name || serviceId
}

const repeatCountOptions: UiSelectOption[] = [
  { value: 1, label: '1 次' },
  { value: 2, label: '2 次' },
  { value: 3, label: '3 次' },
  { value: 5, label: '5 次' },
]

const downloadMiBOptions: UiSelectOption[] = [
  { value: 20, label: '20 MiB' },
  { value: 50, label: '50 MiB' },
  { value: 100, label: '100 MiB' },
]

const downloadTimeoutOptions: UiSelectOption[] = [
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
  { value: 20, label: '20 秒' },
  { value: 30, label: '30 秒' },
]

const serviceTimeoutOptions: UiSelectOption[] = [
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
  { value: 20, label: '20 秒' },
]

const showCompositeModal = ref(false)
const compositeRunning = ref(false)
const compositeCancelled = ref(false)
const compositeStatusTitle = ref('')
const compositeStatusDetail = ref('')
const compositeActiveNodeKey = ref('')

const compositeIncludeLatency = ref(true)
const compositeLatencySampleCount = ref(6)
const compositeLatencyTimeout = ref(5)

const compositeIncludeDownload = ref(true)
const compositeDownloadRepeatCount = ref(1)
const compositeDownloadMaxMiB = ref(20)
const compositeDownloadTimeout = ref(10)

const compositeIncludeService = ref(true)
const compositeServiceRepeatCount = ref(1)
const compositeSelectedServices = ref<string[]>(['cloudflare_204', 'google_204', 'github_api_root'])
const compositeServiceTimeout = ref(10)

const downloadResultByKey = ref<Record<string, NodeDownloadSummary>>({})
const serviceResultsByKey = ref<Record<string, Record<string, NodeServiceSummary>>>({})
const activeDownloadAttempt = ref<{ attempt_id: string; query: WorkbenchDownloadHistoryQuery } | null>(null)
const activeServiceAttempt = ref<{ attempt_id: string; query: WorkbenchPublicServiceHistoryQuery } | null>(null)

const availableCompositeServices = computed(() => publicServiceCatalog.value.length
  ? publicServiceCatalog.value.map(rule => ({ id: rule.service_id, name: `${rule.category || '服务'} · ${rule.name}` }))
  : [
      { id: 'cloudflare_204', name: '基础网络 · Cloudflare 204' },
      { id: 'google_204', name: '基础网络 · Google 204' },
      { id: 'github_api_root', name: '开发服务 · GitHub API' },
      { id: 'antigravity', name: 'AI 服务 · Google Antigravity' },
    ])

function toggleCompositeService(id: string): void {
  const index = compositeSelectedServices.value.indexOf(id)
  if (index >= 0) {
    compositeSelectedServices.value.splice(index, 1)
  } else {
    compositeSelectedServices.value.push(id)
  }
}

const canStartComposite = computed(() => {
  if (selectedKeys.value.length === 0 || compositeRunning.value || batchBusy.value) return false
  if (!compositeIncludeLatency.value && !compositeIncludeDownload.value && !compositeIncludeService.value) return false
  if (compositeIncludeService.value && compositeSelectedServices.value.length === 0) return false
  return true
})

const estimatedDurationText = computed(() => {
  const nodeCount = selectedKeys.value.length || 1
  let seconds = 0
  if (compositeIncludeLatency.value) seconds += compositeLatencyTimeout.value + 2
  if (compositeIncludeDownload.value) seconds += nodeCount * compositeDownloadRepeatCount.value * (compositeDownloadTimeout.value * 0.6 + 1)
  if (compositeIncludeService.value) seconds += nodeCount * compositeSelectedServices.value.length * compositeServiceRepeatCount.value * 0.8
  return `约 ${Math.round(seconds)} 秒`
})

function downloadMbpsFor(key: string): number | null {
  return downloadResultByKey.value[key]?.speedMbps ?? null
}

function serviceSummaryBadge(key: string): string {
  const map = serviceResultsByKey.value[key]
  if (!map) return ''
  const entries = Object.values(map)
  if (!entries.length) return ''
  const successes = entries.filter((e) => e.outcome === 'matched').length
  return `服务 ${successes}/${entries.length} 正常`
}

function serviceDetailTitle(key: string): string {
  const map = serviceResultsByKey.value[key]
  if (!map) return ''
  return Object.entries(map).map(([svc, info]) => `${svc}: ${info.outcome === 'matched' ? `${Math.round(info.durationMs)}ms ✓` : serviceOutcomeLabel(info.outcome)}`).join(' · ')
}

let optionsRequestID = 0
let historyRequestID = 0
let detailRequestID = 0
let pendingBatchRequestID = ''
let activeBatchID = ''
let unsubscribeEvents: (() => void) | null = null
let batchPollTimer: ReturnType<typeof setTimeout> | null = null

const workbenchProjects = [
  { id: 'latency' as const, label: '延迟与稳定性', available: true },
  { id: 'throughput' as const, label: '下载速度', available: true },
  { id: 'service' as const, label: '公共服务即时检测', available: true },
]

const profileOptions = computed(() => {
  const byId = new Map<string, { id: string; name: string; count: number }>()
  for (const option of options.value) {
    const current = byId.get(option.profileId)
    if (current) current.count += 1
    else byId.set(option.profileId, { id: option.profileId, name: option.profileName, count: 1 })
  }
  return Array.from(byId.values()).sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
})
const logicalProfileChoices = computed(() => buildLogicalProfileChoices(
  options.value,
  node => !isNoticeNode(node, noticeOverrides.value, scopeKey(node)),
))
function sourceName(profileId: string, fallback: string): string {
  const choice = logicalChoiceForProfile(logicalProfileChoices.value, profileId)
  if (!choice) return fallback
  if (showAlternateConfigs.value && choice.configCount > choice.count) {
    const subscription = fallback.split(' · ').slice(1).join(' · ').trim()
    if (subscription) return `${choice.name} / ${subscription}`
  }
  return choice.name
}

const profileSelectOptions = computed(() => [
  { value: 'all', label: `全部来源（${logicalProfileChoices.value.length}）` },
  ...(selectedProfileIds.value.length > 1 && !logicalProfileChoices.value.some(choice => choice.profileIds.length === selectedProfileIds.value.length && choice.profileIds.every(id => selectedProfileIds.value.includes(id))) ? [{ value: 'multiple', label: `已选 ${selectedProfileIds.value.length} 个订阅` }] : []),
  ...logicalProfileChoices.value.map((profile) => {
    const notices = options.value.filter(node => profile.profileIds.includes(node.profileId) && isNoticeNode(node, noticeOverrides.value, scopeKey(node))).length
    return { value: profile.id, label: `${profile.name}（${profile.count} 条线路${notices ? ` · ${notices} 条公告` : ''}${profile.mergedSourceCount > 1 ? ` · ${profile.mergedSourceCount} 个订阅` : ''}）` }
  }),
])
const airportProfileGroups = computed(() => {
  const groups = new Map<string, LogicalProfileChoice[]>()
  for (const profile of logicalProfileChoices.value) {
    const airportName = profile.airportName
    const group = groups.get(airportName) || []
    group.push(profile)
    groups.set(airportName, group)
  }
  return Array.from(groups, ([name, profiles]) => ({ name, profiles }))
})
const selectedProfileScopeLabel = computed(() => {
  if (!selectedProfileIds.value.length) return '全部订阅'
  const logical = logicalProfileChoices.value.find(choice => choice.profileIds.length === selectedProfileIds.value.length && choice.profileIds.every(id => selectedProfileIds.value.includes(id)))
  if (logical) return logical.mergedSourceCount > 1 ? `${logical.name} · ${logical.mergedSourceCount} 个同节点订阅已合并` : logical.name
  if (selectedProfileIds.value.length === 1) return profileOptions.value.find(profile => profile.id === selectedProfileIds.value[0])?.name || '1 个订阅'
  return `${selectedProfileIds.value.length} 个订阅`
})
function syncProfileSelect(): void {
  const logical = logicalProfileChoices.value.find(choice => choice.profileIds.length === selectedProfileIds.value.length && choice.profileIds.every(id => selectedProfileIds.value.includes(id)))
  selectedProfileId.value = selectedProfileIds.value.length === 0 ? 'all' : logical?.id || (selectedProfileIds.value.length === 1 ? selectedProfileIds.value[0] : 'multiple')
  selectedRegion.value = 'all'
}
function logicalProfileScopeChecked(profile: LogicalProfileChoice): boolean {
  return selectedProfileIds.value.length === 0 || profile.profileIds.every(id => selectedProfileIds.value.includes(id))
}
function toggleProfileScope(profile: LogicalProfileChoice): void {
  const next = new Set(selectedProfileIds.value.length === 0 ? profileOptions.value.map(profile => profile.id) : selectedProfileIds.value)
  const selected = profile.profileIds.every(id => next.has(id))
  for (const id of profile.profileIds) selected ? next.delete(id) : next.add(id)
  selectedProfileIds.value = Array.from(next)
  syncProfileSelect()
}
function setAllProfileScopes(selectAll: boolean): void {
  selectedProfileIds.value = selectAll ? profileOptions.value.map(profile => profile.id) : []
  syncProfileSelect()
}
function setServiceSources(ids: string[]): void {
  selectedProfileIds.value = ids
  selectedKeys.value = []
  serviceNodeRegion.value = '全部地区'
  syncProfileSelect()
}
function toggleAirportScope(profileIds: string[]): void {
  const next = new Set(selectedProfileIds.value.length === 0 ? profileOptions.value.map(profile => profile.id) : selectedProfileIds.value)
  const allSelected = profileIds.every(id => next.has(id))
  for (const id of profileIds) allSelected ? next.delete(id) : next.add(id)
  selectedProfileIds.value = Array.from(next)
  syncProfileSelect()
}
const sortSelectOptions = [
  { value: 'region', label: '按地区 · 区内延迟优先' },
  { value: 'p50', label: '平时延迟 从低到高' },
  { value: 'p95', label: '较慢时延迟 从低到高' },
  { value: 'health', label: '健康优先' },
  { value: 'name', label: '名称' },
  { value: 'recent', label: '最近测试优先' },
]
const regionNames: Record<string, string> = { SG: '新加坡', US: '美国', JP: '日本', HK: '香港', TW: '台湾', KR: '韩国', CN: '中国大陆', GB: '英国', DE: '德国', FR: '法国', CA: '加拿大', AU: '澳大利亚', NL: '荷兰', IN: '印度', RU: '俄罗斯', OTHER: '未知地区' }
function regionCode(node: MonitorNodeOption): string { return node.countryCode?.trim().toUpperCase() || 'OTHER' }
function regionLabel(code: string): string { return regionNames[code] || code }
const regionSelectOptions = computed<UiSelectOption[]>(() => {
  const counts = new Map<string, number>()
  for (const node of subscriptionOptions.value) {
    if (isNoticeNode(node, noticeOverrides.value, scopeKey(node))) continue
    const code = regionCode(node)
    counts.set(code, (counts.get(code) || 0) + 1)
  }
  return [{ value: 'all', label: '全部地区' }, ...Array.from(counts).sort(([a], [b]) => a === 'OTHER' ? 1 : b === 'OTHER' ? -1 : regionLabel(a).localeCompare(regionLabel(b), 'zh-CN')).map(([code, count]) => ({ value: code, label: `${regionLabel(code)}（${count}）` }))]
})
const timeoutSelectOptions: UiSelectOption[] = [
  { value: 1, label: '1 秒' },
  { value: 3, label: '3 秒' },
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
  { value: 30, label: '30 秒' },
]
function scopeKey(option: Pick<MonitorNodeOption, 'profileId' | 'nodeKey'>): string {
  return `${option.profileId}\u0000${option.nodeKey}`
}

function optionForKey(key: string): MonitorNodeOption | null {
  return options.value.find((option) => scopeKey(option) === key) || null
}

function allSiteTestsForKey(key: string): WorkbenchLatencyTest[] {
  const saved = (historyByKey.value[key] || []).filter((test) => matchesLatencyTarget(test) && samplesInActiveWindow(test.samples).length > 0)
  const current = currentByKey.value[key]
  const visibleCurrent = current && matchesLatencyTarget(current) && samplesInActiveWindow(current.samples).length > 0 ? current : undefined
  if (!visibleCurrent) return saved
  return [visibleCurrent, ...saved.filter((test) => test.attempt_id !== visibleCurrent.attempt_id)]
}

function testsForKey(key: string): WorkbenchLatencyTest[] {
  return allSiteTestsForKey(key).filter(test => !test.target || test.target === 'multi://latency-v1' || test.target === 'https://speed.cloudflare.com/__down?bytes=1').map(baselineLatencyTest)
}

function siteResultsForKey(key: string) { return latencySiteResults(allSiteTestsForKey(key)) }
function suiteHealthForKey(key: string) { return suiteHealth(allSiteTestsForKey(key)) }

function latestTestForKey(key: string): WorkbenchLatencyTest | null {
  return testsForKey(key)[0] || null
}

function latestBatchItemForKey(key: string): WorkbenchLatencyBatchItem | null {
  const node = optionForKey(key)
  if (!node) return null
  const batches = [activeBatch.value, latestBatchDetail.value]
    .filter((batch): batch is WorkbenchLatencyBatch => !!batch)
    .sort((a, b) => Date.parse(b.requested_at) - Date.parse(a.requested_at))
  const latest = batches[0]
  return latest?.items?.find(item => item.profile_id === node.profileId
    && item.node_key === node.nodeKey
    && item.node_identity_key === node.nodeIdentityKey
    && item.config_revision_key === node.configRevisionKey) || null
}

function testFailureReason(test: WorkbenchLatencyTest | null | undefined): string {
  if (!test || test.failure_samples <= 0) return ''
  const sampleReasons = [...new Set((test.samples || []).filter(sample => !sample.success && sample.error).map(sample => sample.error!.trim()))]
  return sampleReasons.length ? sampleReasons.join('；') : test.error_message?.trim() || '探测未返回详细错误'
}

function explainFailure(reason: string): string {
  if (!reason) return '探针未返回详细错误；可打开批次记录查看原始信息。'
  if (/connect failed: dial tcp|connect error: connect failed/i.test(reason) && /timed out|timeout|not properly respond|failed to respond|deadline exceeded/i.test(reason)) return '无法连接节点服务器：连接超时或服务器无响应。'
  if (/connection refused|connectex: No connection could be made/i.test(reason)) return '节点服务器拒绝连接，请检查节点配置或稍后重试。'
  if (/timeout|deadline exceeded|timed out/i.test(reason)) return '请求超时，节点或检测目标未及时响应。'
  if (/tls|certificate/i.test(reason)) return 'TLS 连接或证书验证失败。'
  if (/unexpected EOF|\bEOF\b/i.test(reason)) return '连接被提前关闭（EOF）。'
  return reason.length > 180 ? `${reason.slice(0, 180)}…` : reason
}

function nodeIssueForKey(key: string): string {
  const item = latestBatchItemForKey(key)
  const latest = latestTestForKey(key)
  if (item && (!latest || Date.parse(item.requested_at) >= Date.parse(latest.requested_at))) {
    if (item.execution_state === 'interrupted') return '本轮应用退出时中断，尚未测到此节点；不能判定节点故障。'
    if (item.execution_state === 'not_executed') return '本轮未执行到此节点；不能判定节点故障。'
    if (item.execution_state === 'cancelled') return '本轮已取消，未测到此节点；不能判定节点故障。'
    if (item.execution_state === 'skipped_config') return '本轮节点配置已变化，已跳过；请重新选择后测试。'
    if (item.persistence_state === 'failed') return `本轮已测但保存失败：${item.persistence_error || '请在批次进度中重试保存。'}`
    if (item.execution_state === 'failed' || (item.result?.failure_samples || 0) > 0) {
      const prefix = item.result?.success_samples ? '本轮部分探测失败' : '本轮探测失败'
      return `${prefix}：${explainFailure(testFailureReason(item.result) || item.error_message || '')}`
    }
    if (item.execution_state === 'queued' || item.execution_state === 'running') return '本轮正在等待或执行测试，结果尚未生成。'
    return ''
  }
  const reason = testFailureReason(latest)
  return reason ? `最近一次探测失败：${explainFailure(reason)}` : ''
}

function samplesInActiveWindow(samples: WorkbenchLatencySample[] | undefined): WorkbenchLatencySample[] {
  const sinceMs = Date.parse(activeWindow.value.since)
  const untilMs = Date.parse(activeWindow.value.until)
  if (!Number.isFinite(sinceMs)) return []
  const effectiveUntilMs = Number.isFinite(untilMs) ? Math.max(untilMs, Date.now() + 60000) : Date.now() + 60000
  return (samples || []).filter((sample) => {
    const timestampMs = Date.parse(sample.timestamp)
    return Number.isFinite(timestampMs) && timestampMs >= sinceMs && timestampMs <= effectiveUntilMs
  })
}

function historyLoadState(key: string): HistoryLoadState | undefined {
  return historyLoadStateByKey.value[key]
}

function displayTestForActiveWindow(test: WorkbenchLatencyTest): WorkbenchLatencyTest {
  return { ...test, samples: samplesInActiveWindow(test.samples) }
}

function samplesForKey(key: string): WorkbenchLatencySample[] {
  const samples = testsForKey(key).flatMap((test) =>
    (test.samples || []).map((s, idx) => ({ ...s, seq: s.seq ?? idx + 1 }))
  )
  return samplesInActiveWindow(samples).sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp))
}

function historyMetaForKey(key: string): HistoryMeta | null {
  return historyMetaByKey.value[key] || null
}

function hasCompleteHistoryWindow(key: string): boolean {
  return samplesForKey(key).length > 0
}

function successfulLatencies(key: string): number[] {
  return samplesForKey(key).filter((sample) => sample.success && sample.latency_ms > 0).map((sample) => sample.latency_ms).sort((a, b) => a - b)
}

function percentile(values: number[], fraction: number): number | null {
  if (!values.length) return null
  return Math.round(values[Math.min(values.length - 1, Math.max(0, Math.ceil(values.length * fraction) - 1))])
}

function p50ForKey(key: string): number | null { return hasCompleteHistoryWindow(key) ? percentile(successfulLatencies(key), 0.5) : null }
function p95ForKey(key: string): number | null { return hasCompleteHistoryWindow(key) ? percentile(successfulLatencies(key), 0.95) : null }

function visibleStatus(key: string): 'normal' | 'flaky' | 'failed' | 'nodata' {
  const test = latestTestForKey(key)
  if (!test) return 'nodata'
  if (test.status === 'failed') return 'failed'
  if (test.failure_samples > 0) return 'flaky'
  return 'normal'
}

function statusLabel(key: string): string {
  return { normal: '稳定', flaky: '有波动', failed: '失败', nodata: '没有历史' }[visibleStatus(key)]
}

function toggleNoticeOverride(key: string, isNotice: boolean): void {
  noticeOverrides.value = { ...noticeOverrides.value, [key]: isNotice }
  saveNoticeOverrides(noticeOverrides.value)
}

const openMenuKey = ref<string | null>(null)
function toggleRowMenu(key: string): void {
  openMenuKey.value = openMenuKey.value === key ? null : key
}
function closeRowMenu(): void {
  openMenuKey.value = null
}

const undoNoticeToast = ref<{
  nodeKey: string
  displayName: string
  becameNotice: boolean
  timerId: number
} | null>(null)

function handleToggleNotice(node: MonitorNodeOption): void {
  const key = scopeKey(node)
  const currentlyNotice = isNoticeNode(node, noticeOverrides.value, key)
  const nextNotice = !currentlyNotice
  toggleNoticeOverride(key, nextNotice)
  openMenuKey.value = null

  if (undoNoticeToast.value?.timerId) {
    window.clearTimeout(undoNoticeToast.value.timerId)
  }

  const timerId = window.setTimeout(() => {
    if (undoNoticeToast.value?.nodeKey === key) {
      undoNoticeToast.value = null
    }
  }, 5000)

  undoNoticeToast.value = {
    nodeKey: key,
    displayName: node.displayName,
    becameNotice: nextNotice,
    timerId,
  }
}

function undoNoticeToggle(): void {
  if (!undoNoticeToast.value) return
  const { nodeKey, becameNotice, timerId } = undoNoticeToast.value
  window.clearTimeout(timerId)
  toggleNoticeOverride(nodeKey, !becameNotice)
  undoNoticeToast.value = null
}

const monitoredNodeKeys = ref<Set<string>>(new Set())

async function refreshMonitoredNodes(): Promise<void> {
  try {
    const jobs = await fetchMonitorJobs()
    const keys = new Set<string>()
    for (const job of jobs || []) {
      const isRunning = job.runtimeState === 'running' || job.state === 'running'
      if (isRunning) {
        for (const n of job.nodes || []) {
          keys.add(`${job.profileId}\u0000${n.nodeKey}`)
        }
      }
    }
    monitoredNodeKeys.value = keys
  } catch {
    // Non-critical background query
  }
}

function canonicalizeNodes(includeAlternates: boolean): MonitorNodeOption[] {
  const seenConfigs = new Set<string>()
  const preferredProfileByLine = new Map<string, string>()
  const ordered = [...options.value].sort((a, b) => {
    const aNotice = isNoticeNode(a, noticeOverrides.value, scopeKey(a)) ? 1 : 0
    const bNotice = isNoticeNode(b, noticeOverrides.value, scopeKey(b)) ? 1 : 0
    if (aNotice !== bNotice) return aNotice - bNotice
    const aChoice = logicalChoiceForProfile(logicalProfileChoices.value, a.profileId)
    const bChoice = logicalChoiceForProfile(logicalProfileChoices.value, b.profileId)
    return Number(b.profileId === bChoice?.id) - Number(a.profileId === aChoice?.id)
  })
  return ordered.filter(node => {
    const logical = logicalChoiceForProfile(logicalProfileChoices.value, node.profileId)
    const lineKey = `${logical?.id || node.profileId}\u0000${node.nodeIdentityKey || node.nodeKey}`
    const configKey = `${lineKey}\u0000${node.configRevisionKey || node.nodeKey}`
    if (seenConfigs.has(configKey)) return false
    seenConfigs.add(configKey)
    // One airport line is shown once by default. A different subscription's
    // credential/config revision remains available when alternates are shown.
    const preferredProfile = preferredProfileByLine.get(lineKey)
    if (!preferredProfile) preferredProfileByLine.set(lineKey, node.profileId)
    else if (!includeAlternates && preferredProfile !== node.profileId) return false
    return true
  })
}
const canonicalOptions = computed(() => canonicalizeNodes(showAlternateConfigs.value))
const hiddenAlternateCount = computed(() => {
  const proxies = (nodes: MonitorNodeOption[]) => nodes.filter(node => !isNoticeNode(node, noticeOverrides.value, scopeKey(node))).length
  return proxies(canonicalizeNodes(true)) - proxies(canonicalizeNodes(false))
})
const servicePickerNodes = computed(() => canonicalOptions.value.filter(node => !isNoticeNode(node, noticeOverrides.value, scopeKey(node))))
const subscriptionOptions = computed(() => canonicalOptions.value.filter(node => profileInScope(node.profileId)))
const noticeCount = computed(() => subscriptionOptions.value.filter((o) => isNoticeNode(o, noticeOverrides.value, scopeKey(o))).length)
const proxyCount = computed(() => subscriptionOptions.value.length - noticeCount.value)
const noticeImportBusy = ref(false)
const noticeImportFeedback = ref<Record<string, string>>({})
async function saveNoticeEntrypoints(node: MonitorNodeOption) {
  if (noticeImportBusy.value) return
  noticeImportBusy.value = true
  const key = scopeKey(node)
  try {
    const airports = await api.fetchAirports()
    const airport = airports.find(ap => ap.subscriptions?.some(sub => sub.id === node.profileId))
    if (!airport) throw new Error('找不到所属机场，请先重新读取订阅')
    const settings = airport.maintenance || { refresh_hours: 0, links: [] }
    const links = [...(settings.links || [])]
    for (const link of extractNoticeLinks(node.displayName)) {
      if (!links.some(saved => saved.url === link.url)) links.push({ label: link.label.replace(/^打开\s*/, ''), url: link.url, monthly_day: 0 })
    }
    await api.saveAirportMaintenance(airport.id, { ...settings, links })
    noticeImportFeedback.value[key] = `已加入「${airport.name}」的常用入口`
  } catch (e) { noticeImportFeedback.value[key] = e instanceof Error ? e.message : String(e) }
  finally { noticeImportBusy.value = false }
}

const nodeCategorySelectOptions = computed<UiSelectOption[]>(() => [
  { value: 'proxies', label: `真实节点（${proxyCount.value}）` },
  { value: 'notices', label: `公告与信息（${noticeCount.value}）` },
  { value: 'all', label: `全部条目（${subscriptionOptions.value.length}）` },
])

function visibleOptionsForProject(): MonitorNodeOption[] {
  const latestItems = activeBatch.value?.items?.length ? activeBatch.value.items : latestBatchDetail.value?.items || []
  const recentlyTested = new Set(latestItems.map((item) => `${item.profile_id}\u0000${item.node_key}\u0000${item.node_identity_key}\u0000${item.config_revision_key}`))
  const query = searchText.value.trim().toLocaleLowerCase()
  const filtered = subscriptionOptions.value.filter((option) => {
    if (nodeCategoryFilter.value !== 'notices' && selectedRegion.value !== 'all' && regionCode(option) !== selectedRegion.value) return false
    const key = scopeKey(option)
    const isNotice = isNoticeNode(option, noticeOverrides.value, key)
    if (nodeCategoryFilter.value === 'proxies' && isNotice) return false
    if (nodeCategoryFilter.value === 'notices' && !isNotice) return false
    if (!query) return true
    return [option.displayName, option.profileName, regionLabel(regionCode(option)), option.countryCode, option.type].filter(Boolean).some((value) => value.toLocaleLowerCase().includes(query))
  })
  return filtered.sort((a, b) => {
    const aKey = scopeKey(a), bKey = scopeKey(b)
    const aRecent = recentlyTested.has(`${aKey}\u0000${a.nodeIdentityKey}\u0000${a.configRevisionKey}`)
    const bRecent = recentlyTested.has(`${bKey}\u0000${b.nodeIdentityKey}\u0000${b.configRevisionKey}`)
    if (sortBy.value === 'recent' && aRecent !== bRecent) return aRecent ? -1 : 1
    if (sortBy.value === 'region' && regionCode(a) !== regionCode(b)) {
      if (regionCode(a) === 'OTHER') return 1
      if (regionCode(b) === 'OTHER') return -1
      return regionLabel(regionCode(a)).localeCompare(regionLabel(regionCode(b)), 'zh-CN')
    }
    if (sortBy.value === 'name') return a.displayName.localeCompare(b.displayName, 'zh-CN')
    if (sortBy.value === 'health') {
      const rank = { normal: 0, flaky: 1, failed: 2, nodata: 3 }
      const difference = rank[visibleStatus(aKey)] - rank[visibleStatus(bKey)]
      if (difference) return difference
    }
    const aValue = sortBy.value === 'p95' ? p95ForKey(aKey) : p50ForKey(aKey)
    const bValue = sortBy.value === 'p95' ? p95ForKey(bKey) : p50ForKey(bKey)
    if (aValue === null && bValue === null) return a.displayName.localeCompare(b.displayName, 'zh-CN')
    if (aValue === null) return 1
    if (bValue === null) return -1
    return aValue - bValue
  })
}

const visibleOptions = computed(visibleOptionsForProject)

const isAllVisibleSelected = computed(() => {
  if (visibleOptions.value.length === 0) return false
  return visibleOptions.value.every((node) => selectedKeys.value.includes(scopeKey(node)))
})

function toggleSelectAllVisible(): void {
  if (isAllVisibleSelected.value) {
    const visibleKeys = new Set(visibleOptions.value.map(scopeKey))
    selectedKeys.value = selectedKeys.value.filter((k) => !visibleKeys.has(k))
  } else {
    const next = new Set(selectedKeys.value)
    for (const node of visibleOptions.value) {
      next.add(scopeKey(node))
    }
    selectedKeys.value = Array.from(next)
  }
}
const monitorSelection = computed(() => decideMonitorSelection(options.value, selectedKeys.value))
const canOpenMonitor = computed(() => !!monitorSelection.value.prefill)
const monitorSelectionHint = computed(() => monitorSelection.value.reason)
const focusedOption = computed(() => optionForKey(focusedKey.value))
const mergeExactDuplicates = ref(true)
function exactRunnableConfigKey(node: MonitorNodeOption): string {
  if (!node.nodeIdentityKey || !node.configRevisionKey) return scopeKey(node)
  return `${node.nodeIdentityKey}\u0000${node.configRevisionKey}`
}
function runnableNodesForKeys(keys: string[]): MonitorNodeOption[] {
  const nodes = keys.map(optionForKey).filter((node): node is MonitorNodeOption => !!node)
  if (!mergeExactDuplicates.value) return nodes
  const seen = new Set<string>()
  return nodes.filter((node) => {
    const key = exactRunnableConfigKey(node)
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}
const selectedRunnableNodes = computed(() => runnableNodesForKeys(selectedKeys.value))
const mergedDuplicateCount = computed(() => Math.max(0, selectedKeys.value.length - selectedRunnableNodes.value.length))
const publicServiceNode = computed(() => selectedRunnableNodes.value.length === 1 ? selectedRunnableNodes.value[0] : null)
const downloadNode = computed(() => selectedRunnableNodes.value.length === 1 ? selectedRunnableNodes.value[0] : null)
const publicServiceNodes = computed(() => selectedRunnableNodes.value)
const downloadNodes = computed(() => selectedRunnableNodes.value)
const focusedTests = computed(() => (focusedKey.value ? testsForKey(focusedKey.value) : []))
const focusedDisplayedTest = computed(() => {
  const test = detailTest.value || latestTestForKey(focusedKey.value)
  return test ? displayTestForActiveWindow(test) : null
})
const batchBusy = computed(() => batchStarting.value || !!activeBatch.value && ['queued', 'running', 'cancelling', 'saving'].includes(activeBatch.value.state))
const canRun = computed(() => activeProject.value === 'latency' && selectedRunnableNodes.value.length > 0 && !batchBusy.value)
const selectedProfilesLabel = computed(() => {
  const names = new Map<string, string>()
  for (const key of selectedKeys.value) {
    const option = optionForKey(key)
    if (option) names.set(option.profileId, option.profileName)
  }
  return Array.from(names.values()).join('、') || '无'
})
const selectedNodeNames = computed(() => selectedKeys.value.map((key) => optionForKey(key)?.displayName || '').filter(Boolean))
const selectedNodesLabel = computed(() => {
  if (!selectedNodeNames.value.length) return '已选 0 个节点'
  if (selectedNodeNames.value.length === 1) return `已选：${selectedNodeNames.value[0]}`
  const names = selectedNodeNames.value.slice(0, 2).join('、')
  return `已选 ${selectedNodeNames.value.length} 个：${names}${selectedNodeNames.value.length > 2 ? ' 等' : ''}`
})
const latencyTestButtonLabel = computed(() => {
  if (selectedNodeNames.value.length === 1) return `测试 ${selectedNodeNames.value[0]}`
  if (mergedDuplicateCount.value > 0) return `测试 ${selectedRunnableNodes.value.length} 个配置（已合并 ${mergedDuplicateCount.value} 个重复）`
  return `测试所选 ${selectedKeys.value.length} 个节点`
})
const projectLabel = computed(() => workbenchProjects.find((project) => project.id === activeProject.value)?.label || '延迟与稳定性')
const windowLabel = computed(() => `最近 ${windowMode.value}`)
const windowShortName = computed(() => windowMode.value)

function formatAxisTimestamp(timeMs: number, mode: LatencyWindowMode, isEnd: boolean): string {
  if (isEnd) {
    return '现在'
  }
  const date = new Date(timeMs)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  const m = pad(date.getMonth() + 1)
  const d = pad(date.getDate())
  const h = pad(date.getHours())
  const min = pad(date.getMinutes())

  const hours = Number(mode.slice(0, -1)) * (mode.endsWith('d') ? 24 : 1)
  if (hours <= 6) {
    return `${h}:${min}`
  }
  if (hours <= 48) {
    return `${m}/${d} ${h}:${min}`
  }
  return `${m}/${d}`
}

const historyAxisLabels = computed(() => {
  const sinceMs = Date.parse(activeWindow.value.since)
  const untilMs = Date.parse(activeWindow.value.until)
  if (!Number.isFinite(sinceMs) || !Number.isFinite(untilMs) || untilMs <= sinceMs) {
    return ['--', '--', '--', '--', '现在']
  }
  const span = untilMs - sinceMs
  const points = [
    sinceMs,
    sinceMs + span * 0.25,
    sinceMs + span * 0.5,
    sinceMs + span * 0.75,
    untilMs,
  ]
  return points.map((p, idx) => formatAxisTimestamp(p, windowMode.value, idx === 4))
})

function messageFor(error: unknown): string { return error instanceof Error ? error.message : String(error || '请求失败') }

function formatTime(value: string | undefined): string {
  if (!value) return '时间未知'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '时间未知' : date.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function formatShortTime(value: string | undefined): string {
  if (!value) return '时间未知'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '时间未知' : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function currentScope(key: string): LatencyScope {
  const option = optionForKey(key)
  return option ? { profileId: option.profileId, nodeKey: option.nodeKey } : { profileId: '', nodeKey: '' }
}

function sampleLabel(sample: WorkbenchLatencySample | null): string {
  if (!sample) return '暂无原始样本'
  if (sample.success && sample.latency_ms > 0) return `${Math.round(sample.latency_ms)} ms`
  return /timeout|timed out|超时/i.test(sample.error || '') ? '超时' : '失败'
}

function sampleDetail(sample: WorkbenchLatencySample | null): string {
  if (!sample) return '尚未执行真实延迟测试'
  const seqPrefix = sample.seq ? `第 ${sample.seq} 次 · ` : ''
  if (sample.success && sample.latency_ms > 0) return `${seqPrefix}${formatShortTime(sample.timestamp)} · 成功`
  return `${seqPrefix}${formatShortTime(sample.timestamp)} · ${sample.error || '连接失败'}`
}

function sampleClass(sample: WorkbenchLatencySample | null): string {
  if (!sample) return 'nodata'
  if (sample.success && sample.latency_ms > 0) return 'success'
  return /timeout|timed out|超时/i.test(sample.error || '') ? 'timeout' : 'fail'
}

function activeIndexFor(key: string, samples = samplesForKey(key)): number | null {
  if (!samples.length) return null
  const hovered = hoveredByKey.value[key]
  if (typeof hovered === 'number') return hovered
  const pinned = pinnedByKey.value[key]
  if (typeof pinned === 'number') return pinned
  return samples.length - 1
}

function activeSampleFor(key: string): WorkbenchLatencySample | null {
  const samples = samplesForKey(key)
  const index = activeIndexFor(key, samples)
  return index === null ? null : samples[index] || null
}

function activeTestFor(key: string, sample: WorkbenchLatencySample | null): WorkbenchLatencyTest | null {
  if (!sample) return null
  const tests = testsForKey(key)
  const sampleTime = Date.parse(sample.timestamp)
  return tests.find((test) => {
    if (test.samples?.some((s) => s.timestamp === sample.timestamp && s.seq === sample.seq)) return true
    const start = Date.parse(test.started_at || '')
    const finish = Date.parse(test.finished_at || '')
    return Number.isFinite(sampleTime) && Number.isFinite(start) && Number.isFinite(finish) && sampleTime >= start - 1000 && sampleTime <= finish + 1000
  }) || null
}

function readoutState(key: string): string {
  const bucket = hoveredBucketByKey.value[key]
  if (bucket) return '时段'
  const hovered = hoveredByKey.value[key]
  const pinned = pinnedByKey.value[key]
  const sample = activeSampleFor(key)
  const test = activeTestFor(key, sample)
  const totalSamples = test?.total_samples || test?.samples?.length
  const seqSuffix = sample?.seq
    ? totalSamples && totalSamples > 1
      ? ` · 第 ${sample.seq}/${totalSamples} 次`
      : ` · 第 ${sample.seq} 次`
    : ''
  if (typeof hovered === 'number') return `悬停${seqSuffix}`
  if (typeof pinned === 'number') return `已固定${seqSuffix}`
  return '最新'
}

function readoutValue(key: string): string {
  const bucket = hoveredBucketByKey.value[key]
  if (bucket) {
    return bucket.medianLatency !== null ? String(Math.round(bucket.medianLatency)) : '失败'
  }
  const sample = activeSampleFor(key)
  if (!sample && historyLoadState(key) === 'loading') return '读取中…'
  if (!sample && historyLoadState(key) === 'error') return '读取失败'
  return sample && sample.success && sample.latency_ms > 0 ? String(Math.round(sample.latency_ms)) : sampleLabel(sample)
}

function readoutHasLatencyUnit(key: string): boolean {
  const bucket = hoveredBucketByKey.value[key]
  if (bucket) return bucket.medianLatency !== null
  return !!activeSampleFor(key)?.success
}

function latencyValueClass(key: string): string {
  const bucket = hoveredBucketByKey.value[key]
  if (bucket) {
    return bucket.medianLatency !== null ? 'success' : 'fail'
  }
  return sampleClass(activeSampleFor(key))
}

function readoutTime(key: string): string {
  const bucket = hoveredBucketByKey.value[key]
  if (bucket) {
    const timeRange = bucket.startLabel === bucket.endLabel
      ? bucket.startLabel
      : `${bucket.startLabel} ~ ${bucket.endLabel}`
    return `${timeRange} · ${bucket.testCount}次`
  }
  const sample = activeSampleFor(key)
  if (!sample && historyLoadState(key) === 'loading') return '历史读取中'
  return sample ? formatShortTime(sample.timestamp) : '没有记录'
}

function readoutStatus(key: string): string {
  const bucket = hoveredBucketByKey.value[key]
  if (bucket) {
    if (bucket.medianLatency !== null) {
      const p95 = bucket.p95Latency !== null ? `${Math.round(bucket.p95Latency)} ms` : '-'
      return `平时 ${Math.round(bucket.medianLatency)} ms · 较慢时 ${p95} · 整体${statusLabel(key)}`
    }
    return `${bucket.failCount} 次全部失败 · 整体${statusLabel(key)}`
  }
  const sample = activeSampleFor(key)
  if (!sample) return statusLabel(key)
  if (!sample.success || sample.latency_ms <= 0) return sample.error || '连接失败'
  const test = activeTestFor(key, sample)
  if (test && test.total_samples > 1) {
    return `本次 ${test.success_samples}/${test.total_samples} 成功 · 整体${statusLabel(key)}`
  }
  return statusLabel(key)
}

function onModalTableRowClick(sample: WorkbenchLatencySample): void {
  if (!focusedKey.value) return
  const samples = samplesForKey(focusedKey.value)
  const idx = samples.findIndex((it) => it.timestamp === sample.timestamp && (it.seq ?? 0) === (sample.seq ?? 0))
  if (idx !== -1) {
    if (pinnedByKey.value[focusedKey.value] === idx) {
      clearPin(focusedKey.value)
    } else {
      onPin(focusedKey.value, idx)
    }
  }
}

function testStatusLabel(test: WorkbenchLatencyTest | null): string {
  if (!test) return '尚未测试'
  if (test.status === 'completed') return '全部通过'
  if (test.status === 'partial_failed') return '部分失败'
  return '全部失败'
}

function persistenceLabel(test: WorkbenchLatencyTest | null): string {
  if (!test) return ''
  if (test.persistence_state === 'saving') return '保存中'
  if (test.persistence_state === 'saved') return '已保存'
  return '保存失败'
}

function historySummary(key: string): string {
  const tests = testsForKey(key), samples = samplesForKey(key), failed = samples.filter((sample) => !sample.success).length
  if (!tests.length && historyLoadState(key) === 'loading') return '正在读取该节点的历史记录。'
  if (!tests.length && historyLoadState(key) === 'error') return '历史读取失败，请重新读取。'
  if (!tests.length) return '没有保存记录；完成一次真实延迟测试后会在这里出现。'
  const meta = historyMetaForKey(key)
  const moreNote = meta?.hasMore ? `；历史记录超出单页，已显示最新 ${tests.length} 次` : ''
  return `共 ${tests.length} 次测试，${samples.length} 条原始样本，${failed} 次失败${moreNote}。`
}

function statsLabel(key: string): string {
  if (historyLoadState(key) === 'loading' && !samplesForKey(key).length) return '历史读取中…'
  if (historyLoadState(key) === 'error' && !samplesForKey(key).length) return '历史读取失败'
  const p50 = p50ForKey(key)
  const p95 = p95ForKey(key)
  if (p50 === null) return '暂无延迟历史'
  const meta = historyMetaForKey(key)
  const tests = testsForKey(key)
  const suffix = meta?.hasMore ? `（近 ${tests.length} 次）` : ''
  return `平时 ${p50} ms · 较慢时 ${p95 ?? '—'} ms${suffix}`
}


const healthReportCache = computed(() => {
  const map: Record<string, NodeHealthReport | null> = {}
  for (const node of visibleOptions.value) {
    const key = scopeKey(node)
    map[key] = scopedHealthReport(key)
  }
  return map
})

const healthComparisonCache = computed(() => {
  const map: Record<string, LatencyAttemptComparison> = {}
  for (const node of visibleOptions.value) {
    const key = scopeKey(node)
    const saved = testsForKey(key)
      .filter((test) => testMatchesKey(test, key) && test.persistence_state === 'saved')
      .map((test) => ({ ...test, samples: samplesInActiveWindow(test.samples) }))
      .filter((test) => test.samples.length > 0)
    map[key] = compareLatencyAttempts(saved)
  }
  return map
})

function comparisonForKey(key: string): LatencyAttemptComparison {
  return healthComparisonCache.value[key] || compareLatencyAttempts([])
}

function usesAttemptAverage(key: string): boolean {
  return comparisonForKey(key).currentMs !== null
    && hoveredByKey.value[key] == null && pinnedByKey.value[key] == null
    && !hoveredBucketByKey.value[key]
}

function compactHealthLabel(key: string): string {
  const report = nodeHealthReport(key)
  if (!report) return '暂无记录'
  if (!historyMetaForKey(key)?.complete || historyMetaForKey(key)?.hasMore) return '历史未完整'
  return { stable: '稳定', improving: '变快', degrading: '变慢', spiking: '有波动', failing: '连接异常', insufficient: '样本不足' }[report.trend]
}

function comparisonChange(current: number | null, baseline: number | null, unavailable = '暂无可比记录'): string {
  if (current === null) return '本次未成功'
  if (baseline === null || baseline <= 0) return unavailable
  const difference = current - baseline
  if (difference === 0) return '基本持平'
  const percentage = Math.abs(difference / baseline * 100)
  return `${difference > 0 ? '慢' : '快'} ${Math.abs(difference)} ms（${percentage < 1 ? '不足 1' : Math.round(percentage)}%）`
}

function nodeHealthReport(key: string): NodeHealthReport | null {
  return healthReportCache.value[key] || null
}
function normalLatencyText(report: NodeHealthReport): string {
  if (report.normalMin === null || report.normalMax === null) return '暂无'
  return report.normalMin === report.normalMax ? `${report.normalMin} ms` : `${report.normalMin}–${report.normalMax} ms`
}
function scopedHealthReport(key: string): NodeHealthReport | null {
  const report = computeNodeHealthReport(samplesForKey(key))
  if (report && (!historyMetaByKey.value[key]?.complete || historyMetaByKey.value[key]?.hasMore)) {
    report.trend = 'insufficient'
    report.trendClass = 'info'
    report.trendText = '历史未完整加载'
    report.diagnosisTitle = '以下仅统计已加载记录'
    report.diagnosisDetail = '当前还有历史未加载，暂不判断整个观察窗口的健康与趋势。'
  }
  return report
}
function testMatchesKey(test: WorkbenchLatencyTest, key: string): boolean { const option = optionForKey(key); return !!option && test.profile_id === option.profileId && test.node_key === option.nodeKey && test.node_identity_key === option.nodeIdentityKey && test.config_revision_key === option.configRevisionKey }

function upsertHistory(key: string, test: WorkbenchLatencyTest): void {
  const next = [test, ...(historyByKey.value[key] || []).filter((item) => item.attempt_id !== test.attempt_id)]
  next.sort((a, b) => Date.parse(b.finished_at) - Date.parse(a.finished_at) || Date.parse(b.requested_at) - Date.parse(a.requested_at) || b.attempt_id.localeCompare(a.attempt_id))
  historyByKey.value = { ...historyByKey.value, [key]: next }
}

function isNewerWorkbenchLatencyTest(candidate: WorkbenchLatencyTest, current: WorkbenchLatencyTest): boolean {
  const finished = Date.parse(candidate.finished_at) - Date.parse(current.finished_at)
  if (finished !== 0) return finished > 0
  const requested = Date.parse(candidate.requested_at) - Date.parse(current.requested_at)
  if (requested !== 0) return requested > 0
  return candidate.attempt_id === current.attempt_id
}

function isWorkbenchLatencyTestPayload(payload: unknown): payload is WorkbenchLatencyTest {
  if (!payload || typeof payload !== 'object') return false
  const test = payload as Partial<WorkbenchLatencyTest>
  return typeof test.attempt_id === 'string' && typeof test.profile_id === 'string' && typeof test.node_key === 'string' && (test.persistence_state === 'saving' || test.persistence_state === 'saved' || test.persistence_state === 'failed')
}

function handlePersistenceEvent(type: string, payload: unknown): void {
  if (type === 'workbench_latency_batch_updated' && isWorkbenchLatencyBatchPayload(payload)) {
    handleBatchUpdated(payload)
    return
  }
  if (type !== 'workbench_latency_test_persistence_updated' || !isWorkbenchLatencyTestPayload(payload)) return
  const key = scopeKey({ profileId: payload.profile_id, nodeKey: payload.node_key })
  const current = currentByKey.value[key]
  if (!current || isNewerWorkbenchLatencyTest(payload, current)) currentByKey.value = { ...currentByKey.value, [key]: payload }
  upsertHistory(key, payload)
  if (focusedKey.value === key && selectedAttemptID.value === payload.attempt_id) detailTest.value = displayTestForActiveWindow(payload)
  if (payload.persistence_state === 'failed') testError.value = payload.persistence_error || '测试完成，但历史保存失败'
}

async function loadOptions(): Promise<void> {
  const requestID = ++optionsRequestID
  optionsLoading.value = true
  optionsError.value = ''
  historyError.value = ''
  try {
    const loaded = await fetchMonitorNodeOptions()
    if (requestID !== optionsRequestID) return
    options.value = loaded
    if (selectedProfileId.value !== 'all') setProfileScope(selectedProfileId.value)
    const canonical = canonicalOptions.value
    const validKeys = new Set(canonical.map(scopeKey))
    selectedKeys.value = selectedKeys.value.filter((key) => validKeys.has(key))
    if (focusedKey.value && !validKeys.has(focusedKey.value)) { focusedKey.value = ''; detailTest.value = null }
    await loadHistories(canonical)
    if (activeProject.value !== 'latency') void loadProjectHistories(canonical)
    await loadRecentBatches()
  } catch (error) {
    if (requestID === optionsRequestID) optionsError.value = messageFor(error)
  } finally {
    if (requestID === optionsRequestID) optionsLoading.value = false
  }
}

watch(showAlternateConfigs, () => {
  const canonical = canonicalOptions.value
  const validKeys = new Set(canonical.map(scopeKey))
  selectedKeys.value = selectedKeys.value.filter(key => validKeys.has(key))
  if (focusedKey.value && !validKeys.has(focusedKey.value)) closeHistory()
  if (!options.value.length) return
  void loadHistories(canonical)
  if (activeProject.value !== 'latency') void loadProjectHistories(canonical)
})

async function loadHistories(nodes: MonitorNodeOption[]): Promise<void> {
  const requestID = ++historyRequestID
  const requestedWindow = freezeLatencyWindow(windowMode.value)
  activeWindow.value = requestedWindow
  historyByKey.value = {}
  historyMetaByKey.value = {}
  historyLoadStateByKey.value = Object.fromEntries(nodes.map((node) => [scopeKey(node), 'loading' as HistoryLoadState]))
  if (!nodes.length) {
    historyLoading.value = false
    return
  }
  historyLoading.value = true
  try {
    const responses = await api.fetchWorkbenchLatencyHistories(nodes.map((node) => ({
      target_id: latencyTargetID.value,
      profile_id: node.profileId,
      node_key: node.nodeKey,
      node_identity_key: node.nodeIdentityKey,
      config_revision_key: node.configRevisionKey,
      since: requestedWindow.since,
      until: requestedWindow.until,
      limit: 100,
    })))
    if (requestID !== historyRequestID) return
    if (responses.length !== nodes.length) throw new Error('历史响应数量与节点不一致')
    const histories: Record<string, WorkbenchLatencyTest[]> = {}
    const metas: Record<string, HistoryMeta> = {}
    const states: Record<string, HistoryLoadState> = {}
    const errors: string[] = []
    for (let index = 0; index < nodes.length; index++) {
      const node = nodes[index]
      const key = scopeKey(node)
      const response = responses[index]
      if (!Array.isArray(response.tests) || response.tests.some((test) => !testMatchesKey(test, key))) {
        states[key] = 'error'
        errors.push(`${node.displayName || node.nodeKey}：响应归属不一致`)
        continue
      }
      histories[key] = response.tests
      metas[key] = { hasMore: response.has_more, complete: response.complete }
      states[key] = 'ready'
    }
    historyByKey.value = histories
    historyMetaByKey.value = metas
    historyLoadStateByKey.value = states
    historyError.value = errors.length > 0 ? `部分节点历史读取失败：${errors.slice(0, 2).join('；')}${errors.length > 2 ? '…' : ''}` : ''
  } catch (error) {
    if (requestID !== historyRequestID) return
    historyLoadStateByKey.value = Object.fromEntries(nodes.map((node) => [scopeKey(node), 'error' as HistoryLoadState]))
    historyError.value = `历史读取失败：${messageFor(error)}`
  } finally {
    if (requestID === historyRequestID) historyLoading.value = false
  }
}
watch(() => props.visible, visible => {
  if (visible && !batchBusy.value && !compositeRunning.value) void loadOptions()
})

function projectAttemptMatches(node: MonitorNodeOption, attempt: WorkbenchDownloadAttempt | WorkbenchPublicServiceAttempt): boolean {
  return attempt.profile_id === node.profileId && attempt.node_key === node.nodeKey
    && attempt.node_identity_key === node.nodeIdentityKey && attempt.config_revision_key === node.configRevisionKey
}
function mergeProjectHistory<T extends WorkbenchDownloadAttempt | WorkbenchPublicServiceAttempt>(loaded: T[], live: T[]): T[] {
  const byID = new Map(loaded.map((attempt) => [attempt.attempt_id, attempt]))
  for (const attempt of live) {
    const stored = byID.get(attempt.attempt_id)
    if (!stored || (attempt.result && !stored.result)) byID.set(attempt.attempt_id, attempt)
  }
  return Array.from(byID.values())
}

async function loadProjectHistories(nodes = options.value): Promise<void> {
  const requestID = ++projectHistoryRequestID
  const project = activeProject.value
  projectHoveredByKey.value = {}
  projectPinnedByKey.value = {}
  if (project === 'latency') return
  const window = activeWindow.value
  projectHistoryStateByKey.value = Object.fromEntries(nodes.map((node) => [scopeKey(node), 'loading' as HistoryLoadState]))
  projectHistoryMetaByKey.value = {}
  if (project === 'throughput') downloadHistoryByKey.value = {}
  else serviceHistoryByKey.value = {}

  // A project tab is allowed to read history, never to initiate measurements.
  // Bound the existing per-node API fan-out so dozens of rows do not flood the UI/server.
  let next = 0
  const worker = async () => {
    while (next < nodes.length && requestID === projectHistoryRequestID) {
      const node = nodes[next++]
      const key = scopeKey(node)
      const scope = {
        profile_id: node.profileId, node_key: node.nodeKey,
        node_identity_key: node.nodeIdentityKey, config_revision_key: node.configRevisionKey,
        since: window.since, until: window.until, limit: 100,
      }
      try {
        if (project === 'throughput') {
          const page = await api.fetchWorkbenchDownloadHistory(scope)
          if (requestID !== projectHistoryRequestID) return
          if (!page.attempts?.every((attempt) => projectAttemptMatches(node, attempt))) throw new Error('历史归属不一致')
          downloadHistoryByKey.value = { ...downloadHistoryByKey.value, [key]: mergeProjectHistory(page.attempts, downloadHistoryByKey.value[key] || []) }
          projectHistoryMetaByKey.value = { ...projectHistoryMetaByKey.value, [key]: { hasMore: page.has_more, complete: page.complete } }
        } else {
          const page = await api.fetchWorkbenchPublicServiceHistory(scope)
          if (requestID !== projectHistoryRequestID) return
          if (!page.attempts?.every((attempt) => projectAttemptMatches(node, attempt))) throw new Error('历史归属不一致')
          serviceHistoryByKey.value = { ...serviceHistoryByKey.value, [key]: mergeProjectHistory(page.attempts, serviceHistoryByKey.value[key] || []) }
          projectHistoryMetaByKey.value = { ...projectHistoryMetaByKey.value, [key]: { hasMore: page.has_more, complete: page.complete } }
        }
        projectHistoryStateByKey.value = { ...projectHistoryStateByKey.value, [key]: 'ready' }
      } catch {
        if (requestID !== projectHistoryRequestID) return
        projectHistoryStateByKey.value = { ...projectHistoryStateByKey.value, [key]: 'error' }
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(4, nodes.length) }, worker))
}

function upsertProjectAttempt(key: string, attempt: WorkbenchDownloadAttempt | WorkbenchPublicServiceAttempt): void {
  const node = optionForKey(key)
  if (!node || !projectAttemptMatches(node, attempt)) return
  const observedAt = attempt.result?.finished_at || attempt.finished_at || attempt.started_at || attempt.requested_at
  if (Date.parse(observedAt) >= Date.parse(activeWindow.value.until)) activeWindow.value = freezeLatencyWindow(windowMode.value)
  if ('service_id' in attempt) {
    const prior = serviceHistoryByKey.value[key] || []
    serviceHistoryByKey.value = { ...serviceHistoryByKey.value, [key]: [attempt, ...prior.filter((item) => item.attempt_id !== attempt.attempt_id)] }
  } else {
    const prior = downloadHistoryByKey.value[key] || []
    downloadHistoryByKey.value = { ...downloadHistoryByKey.value, [key]: [attempt, ...prior.filter((item) => item.attempt_id !== attempt.attempt_id)] }
  }
  projectHistoryStateByKey.value = { ...projectHistoryStateByKey.value, [key]: 'ready' }
}

function toggleSelected(key: string): void { selectedKeys.value = selectedKeys.value.includes(key) ? selectedKeys.value.filter((item) => item !== key) : [...selectedKeys.value, key] }
function clearSelection(): void { selectedKeys.value = [] }

function focusNode(key: string): void {
  focusedKey.value = key
  const latest = latestTestForKey(key)
  detailTest.value = latest ? displayTestForActiveWindow(latest) : null
  selectedAttemptID.value = latest?.attempt_id || ''
  detailError.value = ''
}

function onHover(key: string, index: number | null): void { hoveredByKey.value = { ...hoveredByKey.value, [key]: index } }
function onPin(key: string, index: number): void { pinnedByKey.value = { ...pinnedByKey.value, [key]: index }; hoveredByKey.value = { ...hoveredByKey.value, [key]: null } }
function clearPin(key: string): void {
  pinnedByKey.value = { ...pinnedByKey.value, [key]: null }
  hoveredByKey.value = { ...hoveredByKey.value, [key]: null }
  hoveredBucketByKey.value = { ...hoveredBucketByKey.value, [key]: null }
}
function onBucketHover(key: string, bucket: TimeBucket | null): void {
  hoveredBucketByKey.value = { ...hoveredBucketByKey.value, [key]: bucket }
}
function onBucketClick(key: string, bucket: TimeBucket): void {
  focusNode(key)
  expanded.value = true
  const samples = samplesForKey(key)
  const repSample = samples[bucket.representativeSampleIndex]
  const test = repSample ? activeTestFor(key, repSample) : null
  if (test) {
    void selectHistory(test)
  } else {
    const latest = latestTestForKey(key)
    if (latest) {
      void selectHistory(latest)
    }
  }
}

async function runTest(onlyKeys?: string[]): Promise<void> {
  if (activeProject.value !== 'latency') { testError.value = '当前正式接口只支持“延迟与稳定性”；吞吐和服务可用性先不显示演示结果。'; return }
  if (batchBusy.value) return
  const frozenKeys = [...(onlyKeys || selectedKeys.value)]
  if (!frozenKeys.length) { testError.value = '先勾选要测试的节点。'; return }
  const frozenSelections = runnableNodesForKeys(frozenKeys).map((option) => ({
    profile_id: option.profileId,
    node_key: option.nodeKey,
    node_identity_key: option.nodeIdentityKey,
    config_revision_key: option.configRevisionKey,
    display_name: option.displayName,
    node_type: option.type,
  }))
  if (!frozenSelections.length) { testError.value = '选择的节点已不在当前缓存中，请重新加载节点。'; return }
  const requestID = newWorkbenchRequestID()
  pendingBatchRequestID = requestID
  batchStarting.value = true
  testError.value = ''
  batchMessage.value = `正在创建批次：${frozenSelections.length} 个节点，${new Set(frozenSelections.map((item) => item.profile_id)).size} 个订阅，单项采样 ${sampleCount.value} 次，超时 ${timeoutSeconds.value} 秒。`
  try {
    const created = await api.startWorkbenchLatencyBatch({ target_id: latencyTargetID.value, request_id: requestID, test_project: 'latency_stability', sample_count: sampleCount.value, timeout_seconds: timeoutSeconds.value, concurrency: concurrencyLevel.value, selections: frozenSelections })
    if (pendingBatchRequestID !== requestID) return
    if (!activeBatch.value || activeBatch.value.batch_id !== created.batch_id) activeBatch.value = created
    activeBatchID = created.batch_id
    displayedBatch.value = activeBatch.value
    upsertRecentBatch(activeBatch.value)
    batchMessage.value = `已开始测试 ${created.item_count} 个节点；以开始时的配置为准，配置变化的节点会跳过。`
    scheduleBatchPoll(600)
  } catch (error) {
    if (pendingBatchRequestID === requestID) { testError.value = messageFor(error); batchMessage.value = '' }
  } finally {
    if (pendingBatchRequestID === requestID) { pendingBatchRequestID = ''; batchStarting.value = false }
  }
}

async function startCompositeTest(): Promise<void> {
  if (!canStartComposite.value) return
  const plan = {
    compositeIncludeLatency: compositeIncludeLatency.value,
    compositeLatencySampleCount: compositeLatencySampleCount.value,
    compositeLatencyTimeout: compositeLatencyTimeout.value,
    compositeIncludeDownload: compositeIncludeDownload.value,
    compositeDownloadMaxMiB: compositeDownloadMaxMiB.value,
    compositeDownloadRepeatCount: compositeDownloadRepeatCount.value,
    compositeDownloadTimeout: compositeDownloadTimeout.value,
    compositeIncludeService: compositeIncludeService.value,
    compositeSelectedServices: [...compositeSelectedServices.value],
    compositeServiceRepeatCount: compositeServiceRepeatCount.value,
    compositeServiceTimeout: compositeServiceTimeout.value,
  }
  showCompositeModal.value = false
  compositeCancelled.value = false
  testError.value = ''
  batchMessage.value = ''

  const targetNodes = runnableNodesForKeys(selectedKeys.value)
  if (!targetNodes.length) {
    compositeRunning.value = false
    testError.value = '所选节点已不在当前缓存中，请重新加载。'
    return
  }

  if (!await confirmTestPlan({ title: '确认组合测试与流量', nodes: targetNodes.length, rounds: plan.compositeDownloadRepeatCount, repeatSummary: [plan.compositeIncludeLatency ? `延迟 ${plan.compositeLatencySampleCount} 次` : '', plan.compositeIncludeDownload ? `下载 ${plan.compositeDownloadRepeatCount} 次` : '', plan.compositeIncludeService ? `每项服务 ${plan.compositeServiceRepeatCount} 次` : ''].filter(Boolean).join(' · '), downloadMiB: plan.compositeIncludeDownload ? targetNodes.length * plan.compositeDownloadRepeatCount * plan.compositeDownloadMaxMiB : 0, services: [...(plan.compositeIncludeLatency ? ['延迟与稳定性'] : []), ...(plan.compositeIncludeService ? plan.compositeSelectedServices.map(publicServiceName) : [])], antigravity: plan.compositeIncludeService && plan.compositeSelectedServices.includes('antigravity') })) return
  if (compositeRunning.value || batchBusy.value) return
  compositeRunning.value = true
  try {
    // 1. Latency test
    if (plan.compositeIncludeLatency && !compositeCancelled.value) {
      compositeStatusTitle.value = '组合测试 · 延迟探测中'
      compositeStatusDetail.value = `正在批量探测 ${targetNodes.length} 个节点（每节点采样 ${plan.compositeLatencySampleCount} 次）...`
      const frozenSelections = targetNodes.map((option) => ({
        profile_id: option.profileId,
        node_key: option.nodeKey,
        node_identity_key: option.nodeIdentityKey,
        config_revision_key: option.configRevisionKey,
        display_name: option.displayName,
        node_type: option.type,
      }))
      const requestID = newWorkbenchRequestID()
      pendingBatchRequestID = requestID
      batchStarting.value = true
      const created = await api.startWorkbenchLatencyBatch({
        target_id: latencyTargetID.value,
        request_id: requestID,
        test_project: 'latency_stability',
        timeout_seconds: plan.compositeLatencyTimeout,
        sample_count: plan.compositeLatencySampleCount,
        selections: frozenSelections,
      })
      activeBatch.value = created
      activeBatchID = created.batch_id
      displayedBatch.value = created
      upsertRecentBatch(created)
      batchStarting.value = false

      // Poll until batch finishes
      while (!compositeCancelled.value) {
        await new Promise((r) => setTimeout(r, 600))
        if (!activeBatchID) break
        const polled = await api.fetchWorkbenchLatencyBatch(activeBatchID)
        if (polled && polled.batch_id === activeBatchID) {
          handleBatchUpdated(polled)
          const isDone = ['completed', 'completed_with_save_failures', 'completed_with_issues', 'failed', 'cancelled'].includes(polled.state)
          if (isDone) break
        }
      }
    }

    // 2. Download speed test
    if (plan.compositeIncludeDownload && !compositeCancelled.value && typeof api.startWorkbenchDownloadTest === 'function') {
      compositeStatusTitle.value = '组合测试 · 下载测速中'
      const maxBytes = plan.compositeDownloadMaxMiB * 1024 * 1024
      for (let i = 0; i < targetNodes.length; i++) {
        if (compositeCancelled.value) break
        const node = targetNodes[i]
        const key = scopeKey(node)
        compositeActiveNodeKey.value = key
        for (let rep = 1; rep <= plan.compositeDownloadRepeatCount; rep++) {
          if (compositeCancelled.value) break
          compositeStatusDetail.value = `[${i + 1}/${targetNodes.length}] 正在测速 ${node.displayName || node.nodeKey} (${rep}/${plan.compositeDownloadRepeatCount} 次，${plan.compositeDownloadMaxMiB} MiB)...`
          try {
            const reqId = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `dl-${Date.now()}-${Math.random().toString(16).slice(2)}`
            const created = await api.startWorkbenchDownloadTest({
              request_id: reqId,
              profile_id: node.profileId,
              node_key: node.nodeKey,
              node_identity_key: node.nodeIdentityKey,
              config_revision_key: node.configRevisionKey,
              maximum_bytes: maxBytes,
              timeout_seconds: plan.compositeDownloadTimeout,
            })
            const query: WorkbenchDownloadHistoryQuery = {
              profile_id: node.profileId,
              node_key: node.nodeKey,
              node_identity_key: node.nodeIdentityKey,
              config_revision_key: node.configRevisionKey,
            }
            activeDownloadAttempt.value = { attempt_id: created.attempt_id, query }
            let polled = created
            const pollStart = Date.now()
            const maxWaitMs = (plan.compositeDownloadTimeout + 4) * 1000
            while (!compositeCancelled.value && (Date.now() - pollStart) < maxWaitMs) {
              await new Promise((r) => setTimeout(r, 400))
              polled = await api.fetchWorkbenchDownloadAttempt(created.attempt_id, query)
              if (!['queued', 'running', 'cancelling'].includes(polled.execution_state) && polled.persistence_state !== 'saving') {
                break
              }
            }
            activeDownloadAttempt.value = null
            upsertProjectAttempt(key, polled)
            if (polled.result && polled.result.bytes_read > 0 && polled.result.duration_ns > 0) {
              const sec = polled.result.duration_ns / 1e9
              const mbps = Math.round(((polled.result.bytes_read * 8) / (sec * 1e6)) * 10) / 10
              downloadResultByKey.value = {
                ...downloadResultByKey.value,
                [key]: {
                  speedMbps: mbps,
                  durationMs: Math.round(sec * 1000),
                  bytes: polled.result.bytes_read,
                  timestamp: polled.result.finished_at || new Date().toISOString(),
                  outcome: polled.result.outcome,
                },
              }
            }
          } catch (err) {
            testError.value = `部分测速未完成：${messageFor(err)}`
          }
        }
      }
      compositeActiveNodeKey.value = ''
    }

    // 3. Public services test
    if (plan.compositeIncludeService && !compositeCancelled.value && typeof api.startWorkbenchPublicServiceTest === 'function') {
      compositeStatusTitle.value = '组合测试 · 公共服务检测中'
      const services = plan.compositeSelectedServices
      for (let i = 0; i < targetNodes.length; i++) {
        if (compositeCancelled.value) break
        const node = targetNodes[i]
        const key = scopeKey(node)
        compositeActiveNodeKey.value = key
        for (const svcId of services) {
          if (compositeCancelled.value) break
          for (let rep = 1; rep <= plan.compositeServiceRepeatCount; rep++) {
            if (compositeCancelled.value) break
            const svcName = publicServiceName(svcId)
            compositeStatusDetail.value = `[${i + 1}/${targetNodes.length}] 正在检测 ${node.displayName || node.nodeKey} -> ${svcName} (${rep}/${plan.compositeServiceRepeatCount})...`
            try {
              const reqId = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `svc-${Date.now()}-${Math.random().toString(16).slice(2)}`
              const created = await api.startWorkbenchPublicServiceTest({
                request_id: reqId,
                profile_id: node.profileId,
                node_key: node.nodeKey,
                node_identity_key: node.nodeIdentityKey,
                config_revision_key: node.configRevisionKey,
                service_id: svcId,
                timeout_seconds: plan.compositeServiceTimeout,
              })
              const query: WorkbenchPublicServiceHistoryQuery = {
                profile_id: node.profileId,
                node_key: node.nodeKey,
                node_identity_key: node.nodeIdentityKey,
                config_revision_key: node.configRevisionKey,
                service_id: svcId,
              }
              activeServiceAttempt.value = { attempt_id: created.attempt_id, query }
              let polled = created
              const pollStart = Date.now()
              const maxWaitMs = (plan.compositeServiceTimeout + 4) * 1000
              while (!compositeCancelled.value && (Date.now() - pollStart) < maxWaitMs) {
                await new Promise((r) => setTimeout(r, 350))
                polled = await api.fetchWorkbenchPublicServiceAttempt(created.attempt_id, query)
                if (!['queued', 'running', 'cancelling'].includes(polled.execution_state) && polled.persistence_state !== 'saving') {
                  break
                }
              }
              activeServiceAttempt.value = null
              upsertProjectAttempt(key, polled)
              if (polled.result) {
                const nodeMap = serviceResultsByKey.value[key] || {}
                nodeMap[svcId] = {
                  outcome: polled.result.outcome,
                  durationMs: polled.result.duration_ms,
                  httpStatus: polled.result.http_status,
                  timestamp: polled.result.finished_at,
                }
                serviceResultsByKey.value = { ...serviceResultsByKey.value, [key]: { ...nodeMap } }
              }
            } catch (err) {
              testError.value = `部分服务检查未完成：${messageFor(err)}`
            }
          }
        }
      }
      compositeActiveNodeKey.value = ''
    }

    if (!compositeCancelled.value) {
      batchMessage.value = testError.value ? '组合测试结束，部分项目未完成，请查看错误信息。' : `组合测试结束，${targetNodes.length} 个节点的结果见下方。`
    }
  } catch (err) {
    testError.value = messageFor(err)
  } finally {
    compositeRunning.value = false
    compositeActiveNodeKey.value = ''
    activeDownloadAttempt.value = null
    activeServiceAttempt.value = null
  }
}

async function cancelCompositeTest(): Promise<void> {
  compositeCancelled.value = true
  compositeStatusTitle.value = '正在取消组合测试…'
  compositeStatusDetail.value = '正在停止当前进行的子项…'

  if (activeBatchID && batchBusy.value) {
    try {
      await api.cancelWorkbenchLatencyBatch(activeBatchID)
    } catch {
      // ignore
    }
  }
  if (activeDownloadAttempt.value && typeof api.cancelWorkbenchDownloadTest === 'function') {
    try {
      await api.cancelWorkbenchDownloadTest(activeDownloadAttempt.value.attempt_id, activeDownloadAttempt.value.query)
    } catch {
      // ignore
    }
  }
  if (activeServiceAttempt.value && typeof api.cancelWorkbenchPublicServiceTest === 'function') {
    try {
      await api.cancelWorkbenchPublicServiceTest(activeServiceAttempt.value.attempt_id, activeServiceAttempt.value.query)
    } catch {
      // ignore
    }
  }
  batchMessage.value = '已请求取消，正在等待当前项目结束。'
}

async function runDownloadBatch(params?: { repeatCount?: number; maximumMiB?: number; timeoutSeconds?: number }): Promise<void> {
  const targetKeys = [...selectedKeys.value]
  const targetNodes = runnableNodesForKeys(targetKeys)
  if (!targetNodes.length || compositeRunning.value || batchBusy.value) return

  const repeatCount = params?.repeatCount ?? 1
  const maxMiB = params?.maximumMiB ?? 20
  const timeoutSec = params?.timeoutSeconds ?? 10
  const maxBytes = maxMiB * 1024 * 1024

  if (!await confirmTestPlan({ title: '这次测速会用多少流量？', nodes: targetNodes.length, rounds: repeatCount, downloadMiB: targetNodes.length * repeatCount * maxMiB, services: [], antigravity: false })) return
  if (compositeRunning.value || batchBusy.value) return

  compositeRunning.value = true
  compositeCancelled.value = false
  compositeStatusTitle.value = '下载速度测速 · 排队测速中'
  testError.value = ''
  batchMessage.value = ''

  try {
    for (let i = 0; i < targetNodes.length; i++) {
      if (compositeCancelled.value) break
      const node = targetNodes[i]
      const key = scopeKey(node)
      compositeActiveNodeKey.value = key
      for (let rep = 1; rep <= repeatCount; rep++) {
        if (compositeCancelled.value) break
        compositeStatusDetail.value = `[${i + 1}/${targetNodes.length}] 正在测速 ${node.displayName || node.nodeKey} (${rep}/${repeatCount} 次，${maxMiB} MiB，独占带宽按序测速)...`
        try {
          const reqId = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `dl-${Date.now()}-${Math.random().toString(16).slice(2)}`
          const created = await api.startWorkbenchDownloadTest({
            request_id: reqId,
            profile_id: node.profileId,
            node_key: node.nodeKey,
            node_identity_key: node.nodeIdentityKey,
            config_revision_key: node.configRevisionKey,
            maximum_bytes: maxBytes,
            timeout_seconds: timeoutSec,
          })
          const query: WorkbenchDownloadHistoryQuery = {
            profile_id: node.profileId,
            node_key: node.nodeKey,
            node_identity_key: node.nodeIdentityKey,
            config_revision_key: node.configRevisionKey,
          }
          activeDownloadAttempt.value = { attempt_id: created.attempt_id, query }
          let polled = created
          const pollStart = Date.now()
          const maxWaitMs = (timeoutSec + 4) * 1000
          while (!compositeCancelled.value && (Date.now() - pollStart) < maxWaitMs) {
            await new Promise((r) => setTimeout(r, 400))
            polled = await api.fetchWorkbenchDownloadAttempt(created.attempt_id, query)
            if (!['queued', 'running', 'cancelling'].includes(polled.execution_state) && polled.persistence_state !== 'saving') {
              break
            }
          }
          activeDownloadAttempt.value = null
          upsertProjectAttempt(key, polled)
          if (polled.result && polled.result.bytes_read > 0 && polled.result.duration_ns > 0) {
            const sec = polled.result.duration_ns / 1e9
            const mbps = Math.round(((polled.result.bytes_read * 8) / (sec * 1e6)) * 10) / 10
            downloadResultByKey.value = {
              ...downloadResultByKey.value,
              [key]: {
                speedMbps: mbps,
                durationMs: Math.round(sec * 1000),
                bytes: polled.result.bytes_read,
                timestamp: polled.result.finished_at || new Date().toISOString(),
                outcome: polled.result.outcome,
              },
            }
          }
        } catch (err) {
          testError.value = `部分测速未完成：${messageFor(err)}`
        }
      }
    }
    compositeActiveNodeKey.value = ''
    if (!compositeCancelled.value) {
      batchMessage.value = testError.value ? '测速结束，部分项目未完成，请查看错误信息。' : `下载测速结束，${targetNodes.length} 个节点的结果见下方。`
    }
  } catch (err) {
    testError.value = messageFor(err)
  } finally {
    compositeRunning.value = false
    compositeActiveNodeKey.value = ''
    activeDownloadAttempt.value = null
  }
}

async function runServiceBatch(params?: { serviceId?: string; serviceIds?: string[]; repeatCount?: number; timeoutSeconds?: number }): Promise<void> {
  const targetKeys = [...selectedKeys.value]
  const targetNodes = runnableNodesForKeys(targetKeys)
  if (!targetNodes.length || compositeRunning.value || batchBusy.value) return

  const serviceIds = [...new Set(params?.serviceIds || [params?.serviceId || 'cloudflare_204'])]
  if (!serviceIds.length) return
  const repeatCount = params?.repeatCount ?? 1
  const timeoutSec = params?.timeoutSeconds ?? 10
  if (!await confirmTestPlan({ title: '确认服务检查', nodes: targetNodes.length, rounds: repeatCount, downloadMiB: 0, services: serviceIds.map(publicServiceName), antigravity: serviceIds.includes('antigravity') })) return
  if (compositeRunning.value || batchBusy.value) return

  compositeRunning.value = true
  compositeCancelled.value = false
  compositeStatusTitle.value = '公共服务检测 · 排队检测中'
  testError.value = ''
  batchMessage.value = ''

  try {
    for (let i = 0; i < targetNodes.length; i++) {
      if (compositeCancelled.value) break
      const node = targetNodes[i]
      const key = scopeKey(node)
      compositeActiveNodeKey.value = key
      for (const serviceId of serviceIds) {
      const svcName = publicServiceName(serviceId)
      for (let rep = 1; rep <= repeatCount; rep++) {
        if (compositeCancelled.value) break
        compositeStatusDetail.value = `[${i + 1}/${targetNodes.length}] 正在检测 ${node.displayName || node.nodeKey} -> ${svcName} (${rep}/${repeatCount})...`
        try {
          const reqId = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `svc-${Date.now()}-${Math.random().toString(16).slice(2)}`
          const created = await api.startWorkbenchPublicServiceTest({
            request_id: reqId,
            profile_id: node.profileId,
            node_key: node.nodeKey,
            node_identity_key: node.nodeIdentityKey,
            config_revision_key: node.configRevisionKey,
            service_id: serviceId,
            timeout_seconds: timeoutSec,
          })
          const query: WorkbenchPublicServiceHistoryQuery = {
            profile_id: node.profileId,
            node_key: node.nodeKey,
            node_identity_key: node.nodeIdentityKey,
            config_revision_key: node.configRevisionKey,
            service_id: serviceId,
          }
          activeServiceAttempt.value = { attempt_id: created.attempt_id, query }
          let polled = created
          const pollStart = Date.now()
          const maxWaitMs = (timeoutSec + 4) * 1000
          while (!compositeCancelled.value && (Date.now() - pollStart) < maxWaitMs) {
            await new Promise((r) => setTimeout(r, 350))
            polled = await api.fetchWorkbenchPublicServiceAttempt(created.attempt_id, query)
            if (!['queued', 'running', 'cancelling'].includes(polled.execution_state) && polled.persistence_state !== 'saving') {
              break
            }
          }
          activeServiceAttempt.value = null
          upsertProjectAttempt(key, polled)
          if (polled.result) {
            const nodeMap = serviceResultsByKey.value[key] || {}
            serviceResultsByKey.value = {
              ...serviceResultsByKey.value,
              [key]: {
                ...nodeMap,
                [serviceId]: {
                  outcome: polled.result.outcome,
                  durationMs: polled.result.duration_ms,
                  httpStatus: polled.result.http_status,
                  timestamp: polled.result.finished_at,
                },
              },
            }
          }
        } catch (err) {
          testError.value = `部分服务检查未完成：${messageFor(err)}`
        }
      }
      }
    }
    compositeActiveNodeKey.value = ''
    if (!compositeCancelled.value) {
      batchMessage.value = testError.value ? '服务检查结束，部分项目未完成，请查看错误信息。' : `服务检查结束，${targetNodes.length} 个节点的结果见下方。`
    }
  } catch (err) {
    testError.value = messageFor(err)
  } finally {
    compositeRunning.value = false
    compositeActiveNodeKey.value = ''
    activeServiceAttempt.value = null
  }
}

function handlePanelStartDownloadBatch(params: { repeatCount: number; maximumMiB: number; timeoutSeconds: number }): void {
  void runDownloadBatch(params)
}

function handlePanelStartServiceBatch(params: { serviceId: string; repeatCount: number; timeoutSeconds: number }): void {
  void runServiceBatch(params)
}

function triggerDownloadBatchFromBar(): void {
  void runDownloadBatch({
    repeatCount: barDownloadRepeatCount.value,
    maximumMiB: barDownloadMaxMiB.value,
    timeoutSeconds: barDownloadTimeout.value,
  })
}

function triggerServiceBatchFromBar(): void {
  void runServiceBatch({
    serviceIds: [...selectedServiceIds.value],
    repeatCount: barServiceRepeatCount.value,
    timeoutSeconds: barServiceTimeout.value,
  })
}

function scheduleBatchPoll(delayMs = 1000): void {
  if (batchPollTimer) clearTimeout(batchPollTimer)
  if (!activeBatchID) return
  batchPollTimer = setTimeout(async () => {
    if (!activeBatchID) return
    try {
      const polled = await api.fetchWorkbenchLatencyBatch(activeBatchID)
      if (polled && polled.batch_id === activeBatchID) {
        handleBatchUpdated(polled)
      }
    } catch {
      // ignore poll error, sse or next poll will handle
    }
    if (batchBusy.value) {
      scheduleBatchPoll(1200)
    }
  }, delayMs)
}

function isWorkbenchLatencyBatchPayload(payload: unknown): payload is WorkbenchLatencyBatch {
  if (!payload || typeof payload !== 'object') return false
  const batch = payload as Partial<WorkbenchLatencyBatch>
  return typeof batch.batch_id === 'string' && typeof batch.request_id === 'string' && typeof batch.state === 'string' && Array.isArray(batch.items)
}

function handleBatchUpdated(batch: WorkbenchLatencyBatch): void {
  if (batch.request_id === pendingBatchRequestID) {
    activeBatchID = batch.batch_id
    activeBatch.value = batch
    displayedBatch.value = batch
  } else if (batch.batch_id === activeBatchID) {
    activeBatch.value = batch
    if (displayedBatch.value?.batch_id === batch.batch_id) displayedBatch.value = batch
  } else return
  upsertRecentBatch(batch)
  for (const item of batch.items || []) {
    const result = item.result
    if (!result) continue
    const key = scopeKey({ profileId: item.profile_id, nodeKey: item.node_key })
    const current = currentByKey.value[key]
    if (!current || isNewerWorkbenchLatencyTest(result, current)) {
      currentByKey.value = { ...currentByKey.value, [key]: result }
      upsertHistory(key, result)
    }
  }
  const isTerminal = ['completed', 'completed_with_save_failures', 'completed_with_issues', 'failed', 'cancelled'].includes(batch.state)
  if (isTerminal) {
    if (batchPollTimer) {
      clearTimeout(batchPollTimer)
      batchPollTimer = null
    }
    activeWindow.value = freezeLatencyWindow(windowMode.value)
  }
}

function upsertRecentBatch(batch: WorkbenchLatencyBatch): void {
  recentBatches.value = [batch, ...recentBatches.value.filter((item) => item.batch_id !== batch.batch_id)].slice(0, 20)
}

async function loadRecentBatches(): Promise<void> {
  try {
    recentBatches.value = await api.fetchWorkbenchLatencyBatches(20)
    const latest = recentBatches.value[0]
    latestBatchDetail.value = latest ? (latest.items?.length ? latest : await api.fetchWorkbenchLatencyBatch(latest.batch_id)) : null
    batchHistoryError.value = ''
  } catch (error) { batchHistoryError.value = messageFor(error) }
}

async function selectBatch(batchID: string): Promise<void> {
  try { displayedBatch.value = await api.fetchWorkbenchLatencyBatch(batchID) }
  catch (error) { batchHistoryError.value = messageFor(error) }
}

async function cancelBatch(): Promise<void> {
  const batchID = activeBatchID
  if (!batchID || !activeBatch.value || !['queued', 'running'].includes(activeBatch.value.state)) return
  try {
    const cancelling = await api.cancelWorkbenchLatencyBatch(batchID)
    if (activeBatchID === batchID) { activeBatch.value = cancelling; displayedBatch.value = cancelling }
    batchMessage.value = '正在取消：已开始的探测结束并保存后，未开始项会停止派发。'
  } catch (error) { testError.value = messageFor(error) }
}

async function retryBatchSave(item: NonNullable<WorkbenchLatencyBatch['items']>[number]): Promise<void> {
  if (!displayedBatch.value) return
  try {
    const updated = await api.retryWorkbenchLatencyBatchItem(displayedBatch.value.batch_id, item.item_id)
    displayedBatch.value = updated
    if (activeBatchID === updated.batch_id) activeBatch.value = updated
    upsertRecentBatch(updated)
  } catch (error) { batchHistoryError.value = messageFor(error) }
}

function batchExecutionLabel(state: string): string {
  return ({ queued: '排队中', running: '执行中', completed: '完成', failed: '探测失败', cancelled: '已取消', skipped_config: '配置已变化，已跳过', not_executed: '未执行', interrupted: '应用退出时中断' } as Record<string, string>)[state] || state
}

function batchPersistenceLabel(state: string): string {
  return ({ pending: '待保存', saving: '保存中', saved: '已保存', failed: '保存失败', not_applicable: '无需保存' } as Record<string, string>)[state] || state
}

function batchStatusLabel(state: string): string {
  return ({ queued: '等待执行', running: '执行中', cancelling: '正在取消', saving: '测量已结束，结果保存中', completed: '完成', completed_with_issues: '完成，含跳过或失败项', completed_with_save_failures: '测量完成，部分保存失败', cancelled: '已取消', cancelled_with_issues: '已取消，含失败项', cancelled_with_save_failures: '已取消，部分结果保存失败', interrupted: '上次运行中断', interrupted_with_issues: '应用退出时中断，含失败项', interrupted_with_save_failures: '应用退出时中断，部分结果保存失败' } as Record<string, string>)[state] || state
}

function isBatchItemRunning(key: string): boolean {
  return !!displayedBatch.value?.items?.some((item) => scopeKey({ profileId: item.profile_id, nodeKey: item.node_key }) === key && item.execution_state === 'running')
}

function newWorkbenchRequestID(): string {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `workbench-${random}`
}

async function selectHistory(test: WorkbenchLatencyTest): Promise<void> {
  const key = scopeKey({ profileId: test.profile_id, nodeKey: test.node_key }), scope = currentScope(key)
  if (!scope.profileId || !scope.nodeKey || !testMatchesKey(test, key)) return
  const requestID = ++detailRequestID
  const requested: LatencyAttemptScope = { ...scope, attemptId: test.attempt_id }
  focusedKey.value = key; selectedAttemptID.value = test.attempt_id; detailTest.value = test; detailError.value = ''; detailLoading.value = true
  try {
    const loaded = await api.fetchWorkbenchLatencyTest({
      profile_id: requested.profileId,
      node_key: requested.nodeKey,
      node_identity_key: test.node_identity_key,
      config_revision_key: test.config_revision_key,
      attempt_id: requested.attemptId,
      since: activeWindow.value.since,
      until: activeWindow.value.until,
    })
    if (!acceptsLatencyDetailResponse(requestID, detailRequestID, requested, currentScope(key), selectedAttemptID.value, loaded)) return
    detailTest.value = displayTestForActiveWindow(loaded)
    upsertHistory(key, loaded)
  } catch (error) {
    if (acceptsLatencyDetailResponse(requestID, detailRequestID, requested, currentScope(key), selectedAttemptID.value, test)) detailError.value = messageFor(error)
  } finally {
    if (requestID === detailRequestID && sameLatencyScope(scope, currentScope(key))) detailLoading.value = false
  }
}

const modalActiveTab = ref<'chart' | 'health' | 'config'>('chart')
const focusedHealthReport = computed<NodeHealthReport | null>(() => {
  if (!focusedKey.value) return null
  return scopedHealthReport(focusedKey.value)
})

function openHistory(key: string, tab: 'chart' | 'health' | 'config' = 'chart'): void {
  modalActiveTab.value = tab
  focusNode(key)
  expanded.value = true
  const latest = latestTestForKey(key)
  if (latest) {
    void selectHistory(latest)
  }
}
function closeHistory(): void { expanded.value = false; detailError.value = '' }
function changeProject(project: WorkbenchProject): void { activeProject.value = project; batchMessage.value = ''; testError.value = '' }
defineExpose({ showProject: changeProject })
function projectUnavailableLabel(project: WorkbenchProject): string {
  if (project === 'throughput') {
    if (selectedKeys.value.length === 0) return '请先在下方勾选节点以执行下载测速。'
    if (selectedKeys.value.length === 1) return '将对当前所选节点执行下载测速；独占物理带宽测量。'
    return `已选 ${selectedKeys.value.length} 个节点，下载测速将排队依次执行（测完一个再测下一个，独占带宽保证测速准确性）。`
  }
  if (project === 'service') {
    if (selectedKeys.value.length === 0) return '请先在下方勾选节点以执行公共服务检测。'
    if (selectedKeys.value.length === 1) return '将对当前所选节点执行公共服务即时连通性检测。'
    return `已选 ${selectedKeys.value.length} 个节点，将排队依次执行公共服务连通性检测。`
  }
  return '未接入'
}
function buildProjectPoints(key: string): MetricHistoryPoint[] {
  const since = Date.parse(activeWindow.value.since)
  const until = Date.parse(activeWindow.value.until)
  if (activeProject.value === 'throughput') {
    return (downloadHistoryByKey.value[key] || []).map((attempt): MetricHistoryPoint => {
      const result = attempt.result
      const timestamp = result?.finished_at || attempt.finished_at || attempt.started_at || attempt.requested_at
      const measured = !!result && result.bytes_read > 0 && result.duration_ns > 0
      const speed = measured ? Math.round(result.bytes_read * 8 / (result.duration_ns / 1e9) / 1e6 * 10) / 10 : null
      return {
        id: attempt.attempt_id, timestamp, value: speed,
        label: speed === null ? '未测得速度' : `${speed} Mbps`, success: speed !== null,
        detail: `${measured ? `读取 ${(result.bytes_read / 1048576).toFixed(1)} MiB · 用时 ${(result.duration_ns / 1e9).toFixed(1)} 秒` : result?.error_message || result?.outcome || attempt.execution_state}${attempt.persistence_state === 'saved' ? '' : ' · 尚未保存'}`,
      }
    }).filter((point) => { const time = Date.parse(point.timestamp); return time >= since && time < until }).sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp))
  }
  if (activeProject.value === 'service') {
    return (serviceHistoryByKey.value[key] || []).filter((attempt) => attempt.service_id === barServiceId.value).map((attempt): MetricHistoryPoint => {
      const result = attempt.result
      const timestamp = result?.finished_at || attempt.finished_at || attempt.started_at || attempt.requested_at
      const success = result?.outcome === 'matched'
      return {
        id: attempt.attempt_id, timestamp, value: success && result.duration_ms > 0 ? result.duration_ms : null,
        label: success ? `${Math.round(result!.duration_ms)} ms` : result ? '未通过' : '未完成',
        success,
        detail: `${result ? serviceOutcomeLabel(result.outcome) : attempt.execution_state}${result?.http_status ? ` · HTTP ${result.http_status}` : ''}${attempt.persistence_state === 'saved' ? '' : ' · 尚未保存'}`,
      }
    }).filter((point) => { const time = Date.parse(point.timestamp); return time >= since && time < until }).sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp))
  }
  return []
}
const serviceResultNodes = computed(() => servicePickerNodes.value.filter(node =>
  profileInScope(node.profileId)
  && (serviceNodeRegion.value === '全部地区' || (node.countryCode?.toUpperCase() || '其他') === serviceNodeRegion.value)
  && (!serviceNodeSearch.value.trim() || `${node.displayName} ${node.profileName}`.toLowerCase().includes(serviceNodeSearch.value.trim().toLowerCase()))))
const comparisonRows = computed(() => (activeProject.value === 'service' ? serviceResultNodes.value : visibleOptions.value.filter(node => !isNoticeNode(node, noticeOverrides.value, scopeKey(node)))).map(node => ({ key: scopeKey(node), node })))
function windowRecords<T extends WorkbenchDownloadAttempt | WorkbenchPublicServiceAttempt>(records: Record<string, T[]>): Record<string, T[]> {
  const since = Date.parse(activeWindow.value.since), until = Date.parse(activeWindow.value.until)
  return Object.fromEntries(Object.entries(records).map(([key, items]) => [key, items.filter(a => {
    const time = Date.parse(a.result?.finished_at || a.finished_at || a.requested_at)
    return time >= since && time < until
  })]))
}
const visibleDownloads = computed(() => windowRecords(downloadHistoryByKey.value))
const visibleServices = computed(() => windowRecords(serviceHistoryByKey.value))
const projectPointsCache = computed<Record<string, MetricHistoryPoint[]>>(() =>
  Object.fromEntries(options.value.map((node) => [scopeKey(node), buildProjectPoints(scopeKey(node))])),
)
function projectPointsFor(key: string): MetricHistoryPoint[] { return projectPointsCache.value[key] || [] }
function selectedProjectPoint(key: string): MetricHistoryPoint | null {
  const points = projectPointsFor(key)
  const index = projectHoveredByKey.value[key] ?? projectPinnedByKey.value[key] ?? points.length - 1
  return points[index] || null
}
function projectReadoutTime(key: string): string {
  const point = selectedProjectPoint(key)
  if (!point) return projectHistoryStateByKey.value[key] === 'loading' ? '历史读取中' : '没有记录'
  const prefix = projectHoveredByKey.value[key] != null ? '悬停 · ' : projectPinnedByKey.value[key] != null ? '已固定 · ' : ''
  return `${prefix}${formatShortTime(point.timestamp)}`
}
function projectReadout(key: string): string {
  if (activeProject.value === 'latency') return readoutValue(key)
  return selectedProjectPoint(key)?.label || (projectHistoryStateByKey.value[key] === 'loading' ? '读取中…' : activeProject.value === 'throughput' ? '未测速' : '未检测')
}
function projectHistoryText(key: string): string {
  if (activeProject.value === 'latency') return historySummary(key)
  const point = selectedProjectPoint(key)
  if (point) return `${point.detail}${projectHistoryMetaByKey.value[key]?.hasMore ? ' · 仅近 100 条' : ''}`
  if (projectHistoryStateByKey.value[key] === 'error') return '历史读取失败，请重新读取'
  return activeProject.value === 'throughput' ? '当前窗口暂无下载记录' : '当前窗口暂无该服务记录'
}
function isSelected(key: string): boolean { return selectedKeys.value.includes(key) }
function isTesting(key: string): boolean {
  if (isBatchItemRunning(key)) return true
  if (compositeRunning.value && compositeActiveNodeKey.value === key) return true
  return false
}

function openNodeDetail(option: MonitorNodeOption, origin?: NodeDetailOrigin): void {
  emit('open-node-detail', {
    latencyTargetId: latencyTargetID.value,
    profileId: option.profileId, profileName: option.profileName, nodeKey: option.nodeKey,
    nodeIdentityKey: option.nodeIdentityKey, configRevisionKey: option.configRevisionKey,
    displayName: option.displayName, nodeType: option.type, origin,
  })
}

function openBatchItemDetail(item: NonNullable<WorkbenchLatencyBatch['items']>[number]): void {
  emit('open-node-detail', {
    latencyTargetId: [activeBatch.value, latestBatchDetail.value, ...recentBatches.value].find(batch => batch?.batch_id === item.batch_id)?.target_id || 'cloudflare',
    profileId: item.profile_id,
    profileName: profileOptions.value.find((profile) => profile.id === item.profile_id)?.name || item.profile_id,
    nodeKey: item.node_key,
    nodeIdentityKey: item.node_identity_key,
    configRevisionKey: item.config_revision_key,
    displayName: item.display_name,
    nodeType: item.node_type,
    origin: { kind: 'workbench_batch_item', batchId: item.batch_id, itemId: item.item_id, attemptId: item.attempt_id, observedAt: item.finished_at || item.requested_at, snapshot: item.result },
  })
}

function openMonitor(): void {
  const prefill = monitorSelection.value.prefill
  if (!prefill) {
    batchMessage.value = monitorSelection.value.reason
    return
  }
  emit('open-monitor', prefill)
}

watch(windowMode, async () => {
  saveObservationPreference('selected', windowMode.value)
  activeWindow.value = freezeLatencyWindow(windowMode.value)
  if (activeProject.value !== 'latency') void loadProjectHistories()
  historyRequestID++
  detailRequestID++
  hoveredByKey.value = {}
  pinnedByKey.value = {}
  hoveredBucketByKey.value = {}
  if (options.value.length > 0) {
    await loadHistories(options.value)
  }
  if (focusedKey.value) {
    const latest = latestTestForKey(focusedKey.value)
    if (latest) {
      detailTest.value = displayTestForActiveWindow(latest)
      selectedAttemptID.value = latest.attempt_id
      void selectHistory(latest)
    }
  }
})

watch(activeProject, () => { void loadProjectHistories() })
watch(latencyTargetID, () => { if (options.value.length) void loadHistories(canonicalOptions.value) })

watch(() => props.saveRetryRequest, (request) => {
  if (!request) return
  pendingSaveRetry.value = request
  activeProject.value = request.domain === 'public_service' ? 'service' : 'throughput'
}, { immediate: true })

function resolveSaveRetryRequest(attemptID: string): void {
  if (pendingSaveRetry.value?.attempt_id !== attemptID) return
  pendingSaveRetry.value = null
  emit('save-retry-request-resolved', attemptID)
}

const isStickyEnabled = ref<boolean>((() => {
  try {
    const saved = localStorage.getItem('clash-speedtest-sticky-toolbar')
    return saved === null ? false : saved === 'true'
  } catch {
    return false
  }
})())

const isScrolled = ref(false)

function onWindowScroll() {
  isScrolled.value = window.scrollY > 40
}

function toggleSticky() {
  isStickyEnabled.value = !isStickyEnabled.value
  try {
    localStorage.setItem('clash-speedtest-sticky-toolbar', String(isStickyEnabled.value))
  } catch {}
}

function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(async () => {
  window.addEventListener('scroll', onWindowScroll, { passive: true })
  window.addEventListener('click', closeRowMenu)
  onWindowScroll()
  unsubscribeEvents = api.subscribeEvents(handlePersistenceEvent)
  let catalogLoad: Promise<void> = Promise.resolve()
  try {
    const loadCatalog = api.listWorkbenchPublicServiceCatalog
    if (typeof loadCatalog === 'function') {
      catalogLoad = loadCatalog().then((rules) => { publicServiceCatalog.value = rules }).catch(() => {})
    }
  } catch {
    // Older embedded/test bridges may not expose the catalog yet; keep the safe built-in fallback.
  }
  await Promise.all([
    loadOptions(),
    refreshMonitoredNodes(),
    catalogLoad,
  ])
})

onUnmounted(() => {
  window.removeEventListener('scroll', onWindowScroll)
  window.removeEventListener('click', closeRowMenu)
  if (undoNoticeToast.value?.timerId) {
    window.clearTimeout(undoNoticeToast.value.timerId)
  }
  if (batchPollTimer) {
    clearTimeout(batchPollTimer)
    batchPollTimer = null
  }
  unsubscribeEvents?.()
  unsubscribeEvents = null
})
</script>

<template>
  <main class="prototype-page prototype-latency-workbench" :class="`workspace-${activeProject}`">
    <TestPlanDialog v-if="testPlan" :plan="testPlan" @answer="answerTestPlan" />
    <ExportClashModal
      :visible="isExportModalOpen"
      :selected-keys="selectedKeys"
      :nodes="visibleOptions"
      :get-latency="(k) => latestTestForKey(k)?.latency_ms ?? null"
      @close="isExportModalOpen = false"
    />
    <section class="prototype-page-heading">
      <div><h1>节点工作台</h1><p class="workspace-intro">{{ activeProject === 'latency' ? '六站延迟、连接状态与历史趋势' : activeProject === 'throughput' ? '下载速度与实际流量消耗' : '服务访问、地区限制与检测依据' }}</p></div>
      <div class="workspace-context">{{ proxyCount }} 条线路<template v-if="noticeCount"> · {{ noticeCount }} 条公告</template> · {{ windowLabel }}</div>
    </section>

    <nav class="workspace-modes" aria-label="选择测试方式">
      <button v-for="project in workbenchProjects" :key="project.id" :class="[project.id, { active: activeProject === project.id }]" :aria-pressed="activeProject === project.id" @click="changeProject(project.id)">
        <AppIcon :name="project.id === 'latency' ? 'pulse' : project.id === 'throughput' ? 'download' : 'service'" /><span><strong>{{ project.id === 'latency' ? '延迟检测' : project.id === 'throughput' ? '下载测速' : '服务检测' }}</strong></span>
      </button>
    </nav>
    <ServiceCatalogPicker v-if="activeProject === 'service'" v-model="selectedServiceIds" :catalog="publicServiceCatalog" :disabled="batchBusy || compositeRunning || !!testPlan" :sources="logicalProfileChoices" :selected-source-ids="selectedProfileIds" :nodes="servicePickerNodes" :selected-node-keys="selectedKeys" :node-region="serviceNodeRegion" :node-search="serviceNodeSearch" :show-alternate-configs="showAlternateConfigs" :hidden-alternate-count="hiddenAlternateCount" :repeat-count="barServiceRepeatCount" :timeout-seconds="barServiceTimeout" @update:selected-source-ids="setServiceSources" @update:selected-node-keys="selectedKeys = $event" @update:node-region="serviceNodeRegion = $event" @update:node-search="serviceNodeSearch = $event" @update:show-alternate-configs="showAlternateConfigs = $event" @update:repeat-count="barServiceRepeatCount = $event" @update:timeout-seconds="barServiceTimeout = $event" @run="triggerServiceBatchFromBar" />
    <div v-if="activeProject !== 'service'" class="workbench-sticky-wrapper" :class="{ 'is-sticky': isStickyEnabled }">
      <div class="workbench-control-shell" :class="{ 'is-scrolled': isScrolled && isStickyEnabled }" aria-label="工作台控制面板">
        <!-- Row 1: Scope & Filter Row -->
        <div class="control-row scope-row" aria-label="范围筛选">
          <div class="scope-controls-group">
            <span class="scope-title">范围</span>
            <label class="scope-control">节点来源<UiSelect :model-value="selectedProfileId" variant="scope" aria-label="选择节点来源" :options="profileSelectOptions" @update:model-value="setProfileScope(String($event))" /></label>
            <button v-if="hiddenAlternateCount" type="button" class="alternate-config-toggle" :aria-pressed="showAlternateConfigs" :disabled="batchBusy || compositeRunning" :title="'相同线路在不同订阅中的连接配置可能不同；默认使用首选订阅，打开后可分别测试。'" @click="showAlternateConfigs = !showAlternateConfigs">{{ showAlternateConfigs ? '收起其他配置' : `查看其他配置 (${hiddenAlternateCount})` }}</button>
            <details class="profile-scope-picker" @toggle="onProfilePickerToggle">
              <summary class="profile-scope-summary" :title="`当前范围：${selectedProfileScopeLabel}`">多选</summary>
              <div v-if="profilePickerOpen" class="profile-scope-popover">
                <div class="profile-scope-head">
                  <div><strong>选择节点来源</strong><small>{{ selectedProfileScopeLabel }}</small></div>
                  <div class="profile-scope-actions"><button type="button" @click="setAllProfileScopes(true)">全选</button><button type="button" @click="setAllProfileScopes(false)">全部订阅</button></div>
                </div>
                <div class="profile-scope-groups">
                  <section v-for="group in airportProfileGroups" :key="group.name" class="profile-scope-group">
                    <button type="button" class="airport-scope-toggle" @click="toggleAirportScope(group.profiles.flatMap(profile => profile.profileIds))">
                      <span>{{ group.name }}</span><small>{{ group.profiles.filter(logicalProfileScopeChecked).length }}/{{ group.profiles.length }} 个节点来源</small>
                    </button>
                    <label v-for="profile in group.profiles" :key="profile.id" class="profile-scope-option">
                      <input type="checkbox" :checked="logicalProfileScopeChecked(profile)" @change="toggleProfileScope(profile)">
                      <span>{{ profile.name }}<em>{{ profile.mergedSourceCount > 1 ? `${profile.mergedSourceCount} 个同线路订阅归为一组` : '独立节点集' }}</em></span><small>{{ profile.count }} 条线路<template v-if="profile.configCount > profile.count"> · {{ profile.configCount }} 个待测配置</template></small>
                    </label>
                  </section>
                </div>
                <label class="merge-duplicates-option">
                  <input v-model="mergeExactDuplicates" type="checkbox">
                  <span><strong>合并完全相同的节点配置</strong><small>地址、协议、凭据等配置都相同才只测一次；同名但配置不同仍分别测试。</small></span>
                </label>
              </div>
            </details>
            <label v-if="nodeCategoryFilter !== 'notices'" class="scope-control">地区<UiSelect v-model="selectedRegion" variant="scope" aria-label="筛选地区" :options="regionSelectOptions" /></label>
            <span class="scope-separator" aria-hidden="true"></span>
            <label class="scope-control scope-search-control">搜索<input v-model="searchText" class="scope-search" type="search" placeholder="节点、订阅或地区" aria-label="搜索节点、订阅或地区"></label>
            <label class="scope-control">排序<UiSelect v-model="sortBy" variant="scope" aria-label="节点排序" :options="sortSelectOptions" /></label>
            <span class="scope-separator" aria-hidden="true"></span>
            <label class="scope-control">条目类型<UiSelect v-model="nodeCategoryFilter" variant="scope" aria-label="筛选条目类型" :options="nodeCategorySelectOptions" /></label>
            <span v-if="activeProject === 'latency'" class="scope-separator" aria-hidden="true"></span>
            <label v-if="activeProject === 'latency'" class="alive-filter-label" title="仅显示有测试成功记录的节点">
              <input v-model="filterOnlyAlive" type="checkbox">
              <span>仅看存活</span>
            </label>
            <span v-if="activeProject === 'latency'" class="scope-separator" aria-hidden="true"></span>
            <label v-if="activeProject === 'latency'" class="scope-control">并发
              <select v-model.number="concurrencyLevel" class="concurrency-select" aria-label="测速并发数">
                <option :value="16">16 并发 (极速)</option>
                <option :value="8">8 并发 (平衡)</option>
                <option :value="4">4 并发 (温和)</option>
              </select>
            </label>
          </div>

          <div class="scope-actions-end">
            <button
              type="button"
              class="scope-refresh-btn"
              :disabled="optionsLoading"
              title="重新从本地缓存加载节点"
              @click="loadOptions"
            >
              {{ optionsLoading ? '读取中…' : '重新读取' }}
            </button>
            <button
              type="button"
              class="follow-mode-toggle"
              :class="{ active: isStickyEnabled }"
              :title="isStickyEnabled ? '跟随模式已开启（向下滚动时面板吸顶，点击可关闭）' : '点击开启跟随模式（向下滚动时面板吸顶）'"
              @click="toggleSticky"
            >
              <span class="pin-icon">📌</span> {{ isStickyEnabled ? '跟随中' : '跟随' }}
            </button>
            <button
              type="button"
              class="scroll-top-button"
              title="回到页面顶部"
              @click="scrollToTop"
            >
              ↑ 顶部
            </button>
          </div>
        </div>

        <!-- Row 2: Test Action Row -->
        <div class="top-node-picker">
          <div class="top-node-picker-actions"><button type="button" class="prototype-button" :aria-expanded="topNodePickerOpen" @click="topNodePickerOpen = !topNodePickerOpen">{{ topNodePickerOpen ? '收起节点选择' : '选择节点' }} · 已选 {{ selectedKeys.length }}</button><button type="button" class="prototype-button" :disabled="batchBusy || compositeRunning || !topPickerNodes.length" @click="toggleTopPickerAll">{{ topPickerAllSelected ? '取消当前范围选择' : `全选当前范围（${topPickerNodes.length}）` }}</button><button v-if="selectedKeys.length" type="button" class="prototype-button" :disabled="batchBusy || compositeRunning" @click="selectedKeys = []">清空已选</button><span>按上方来源、地区和搜索筛选；选择与下方列表同步。</span></div>
          <div v-if="topNodePickerOpen" class="top-node-grid"><label v-for="node in topPickerNodes" :key="scopeKey(node)" :title="node.displayName" :class="{ selected: isSelected(scopeKey(node)) }"><input type="checkbox" :checked="isSelected(scopeKey(node))" :disabled="batchBusy || compositeRunning" @change="toggleSelected(scopeKey(node))"><span>{{ node.displayName }}<small>{{ sourceName(node.profileId, node.profileName) }}</small></span></label><p v-if="!topPickerNodes.length">当前范围没有可选节点，请调整上方筛选。</p></div>
        </div>
        <div class="control-row project-row" aria-label="测试项目与执行">
          <div class="project-row-main">
            <div class="project-row-left">
              <div class="project-bar-heading">
                <span class="scope-title">测试项目</span>
                <span class="project-bar-note">先选节点；结果直接出现在节点行</span>
              </div>
              <span class="scope-separator" aria-hidden="true"></span>
              <label class="timeout-control window-control">
                观察窗口
                <ObservationWindowSelect v-model="windowMode" label="选择观察窗口" />
              </label>
              <span class="live-line">{{ windowLabel }} · 截至 {{ formatShortTime(activeWindow.until) }}</span>
            </div>

            <div class="project-bar-actions">
              <span class="selection-summary highlight" :title="selectedNodeNames.join('、')">{{ selectedNodesLabel }}<template v-if="mergedDuplicateCount > 0"> · 实测 {{ selectedRunnableNodes.length }} 个配置</template></span>
              <template v-if="activeProject === 'latency'">
                <span class="latency-suite-hint">综合检测 · 6 个站点 × 每站 {{ sampleCount }} 次</span>
                <label class="timeout-control flex items-center gap-1">
                  每站次数
                  <UiSelect v-model="sampleCount" variant="compact" aria-label="测试次数" :options="sampleCountSelectOptions" :disabled="batchBusy || compositeRunning" />
                  <input
                    v-if="sampleCount !== 6"
                    v-model.number="customSampleCount"
                    type="number"
                    min="1"
                    max="50"
                    class="custom-sample-input font-mono"
                    title="修改自定义采样次数"
                    :disabled="batchBusy || compositeRunning"
                  />
                </label>
                <label class="timeout-control">单项超时<UiSelect v-model="timeoutSeconds" variant="compact" aria-label="单项超时" :options="timeoutSelectOptions" :disabled="batchBusy || compositeRunning" /></label>
                <button
                  type="button"
                  class="prototype-button primary"
                  :disabled="!canRun || compositeRunning"
                  @click="runTest()"
                >
                  {{ batchBusy ? (activeBatch?.state === 'cancelling' ? '正在取消…' : activeBatch?.state === 'saving' ? '结果保存中…' : '批次执行中…') : latencyTestButtonLabel }}
                </button>
                <button
                  v-if="batchBusy && activeBatch?.state !== 'saving'"
                  type="button"
                  class="prototype-button"
                  :disabled="activeBatch?.state === 'cancelling'"
                  @click="cancelBatch"
                >
                  {{ activeBatch?.state === 'cancelling' ? '正在取消…' : '取消本批次' }}
                </button>
              </template>

              <template v-else-if="activeProject === 'throughput'">
                <label class="timeout-control">
                  测速次数
                  <UiSelect v-model="barDownloadRepeatCount" variant="compact" aria-label="下载测速次数" :options="repeatCountOptions" :disabled="batchBusy || compositeRunning" />
                </label>
                <label class="timeout-control">
                  每节点最多下载
                  <UiSelect v-model="barDownloadMaxMiB" variant="compact" aria-label="每节点最多下载" :options="downloadMiBOptions" :disabled="batchBusy || compositeRunning" />
                </label>
                <label class="timeout-control">
                  最多持续
                  <UiSelect v-model="barDownloadTimeout" variant="compact" aria-label="每节点最多测试多久" :options="downloadTimeoutOptions" :disabled="batchBusy || compositeRunning" />
                </label>
                <button
                  type="button"
                  class="prototype-button primary"
                  :disabled="selectedKeys.length === 0 || batchBusy || compositeRunning"
                  @click="triggerDownloadBatchFromBar"
                >
                  {{ compositeRunning ? '测速进行中…' : selectedKeys.length > 1 ? `排队测速所选节点 (${selectedKeys.length} 个)` : '测速所选节点' }}
                </button>
              </template>

              <button
                type="button"
                class="prototype-button primary composite-trigger"
                :disabled="selectedKeys.length === 0 || batchBusy || compositeRunning"
                @click="showCompositeModal = true"
              >
                🎯 自选组合测试…
              </button>
              <button
                v-if="compositeRunning"
                type="button"
                class="prototype-button"
                @click="cancelCompositeTest"
              >
                取消组合测试
              </button>
              <button
                type="button"
                class="prototype-button"
                :disabled="!canOpenMonitor"
                :title="monitorSelectionHint"
                @click="openMonitor"
              >
                加入持续监测
              </button>
            </div>
          </div>

          <div v-if="selectedKeys.length > 0 && !canOpenMonitor" class="batch-test-status blocked">
            {{ monitorSelectionHint }}
          </div>
          <div
            class="batch-test-status"
            :class="{ running: batchBusy || compositeRunning, complete: !!batchMessage && !batchBusy && !compositeRunning }"
            aria-live="polite"
          >
            {{ testError || batchMessage || (selectedKeys.length ? `已选 ${selectedKeys.length} 个节点 · ${activeProject === 'latency' ? '按当前配置采样，完成后更新下方走势' : activeProject === 'throughput' ? '逐个下载，避免带宽争抢' : '检测上方选定服务；多项检测可使用自选组合测试'}` : '勾选下方节点后开始测试。浏览历史和切换视图不会发起请求。') }}
          </div>
        </div>
      </div>
    </div>

    <div v-if="compositeRunning" class="composite-progress-banner">
      <div class="composite-spinner" aria-hidden="true"></div>
      <div class="composite-banner-text">
        <strong>{{ compositeStatusTitle }}</strong>
        <span>{{ compositeStatusDetail }}</span>
      </div>
      <button type="button" class="prototype-button" @click="cancelCompositeTest">取消测试</button>
    </div>

    <WorkbenchPublicServicePanel
      compact
      v-model:service-id="barServiceId"
      v-if="activeProject === 'service'"
      :node="publicServiceNode"
      :selected-nodes="publicServiceNodes"
      :save-retry-request="pendingSaveRetry?.domain === 'public_service' ? pendingSaveRetry : null"
      @start-batch="handlePanelStartServiceBatch"
      @save-retry-request-resolved="resolveSaveRetryRequest"
      @open-node-detail="emit('open-node-detail', $event)"
    />
    <WorkbenchDownloadPanel
      compact
      v-show="activeProject === 'throughput'"
      :node="downloadNode"
      :selected-nodes="downloadNodes"
      :save-retry-request="pendingSaveRetry?.domain === 'download' ? pendingSaveRetry : null"
      @start-batch="handlePanelStartDownloadBatch"
      @save-retry-request-resolved="resolveSaveRetryRequest"
      @open-node-detail="emit('open-node-detail', $event)"
    />

    <section v-if="displayedBatch" class="prototype-panel batch-panel progress-panel" aria-labelledby="latency-batches-title">
      <div class="prototype-panel-header">
        <div>
          <h2 id="latency-batches-title">批量测速进度</h2>
          <p>正在测试和保存的节点排在前面；完成后可在下方比较结果。</p>
        </div>
        <div class="flex items-center gap-3">
          <button v-if="!batchBusy" type="button" class="text-action" @click="displayedBatch = null">收起进度</button>
        </div>
      </div>
      <div v-if="batchHistoryError" class="inline-error">{{ batchHistoryError }}</div>
      <div class="batch-detail">
        <div class="batch-detail-heading">
          <div>
            <strong>{{ batchStatusLabel(displayedBatch.state) }} · {{ batchCounts.ended }} / {{ displayedBatch.item_count }} 个节点已结束测量</strong>
            <span>这 {{ displayedBatch.item_count }} 个节点中：{{ batchCounts.saved }} 个结果已保存<template v-if="batchCounts.saved">（{{ batchCounts.measuredPassed }} 个测到延迟、{{ batchCounts.measuredFailed }} 个探测失败<template v-if="batchCounts.saved > batchCounts.measuredPassed + batchCounts.measuredFailed">、其余 {{ batchCounts.saved - batchCounts.measuredPassed - batchCounts.measuredFailed }} 个已保存</template>）</template><template v-if="batchCounts.interrupted">；{{ batchCounts.interrupted }} 个因应用退出中断、未测得结果</template><template v-if="batchCounts.saveFailed">；{{ batchCounts.saveFailed }} 个保存失败</template><template v-if="batchCounts.running || batchCounts.queued || batchCounts.saving">；测试中 {{ batchCounts.running }}、排队 {{ batchCounts.queued }}、保存中 {{ batchCounts.saving }}</template>。总数已包含这些状态；括号内是已保存结果的细分。</span>
          </div>
          <button v-if="activeBatchID === displayedBatch.batch_id && batchBusy && displayedBatch.state !== 'saving'" type="button" class="prototype-button" :disabled="displayedBatch.state === 'cancelling'" @click="cancelBatch">
            {{ displayedBatch.state === 'cancelling' ? '正在取消…' : '取消本批次' }}
          </button>
        </div>
        <progress class="batch-progress" :value="batchCounts.ended" :max="Math.max(1, displayedBatch.item_count)" :aria-label="`测量进度 ${batchProgress}%`">{{ batchProgress }}%</progress>
        <div class="batch-list-toolbar">
          <div class="batch-filters" role="group" aria-label="筛选测试进度">
            <button type="button" :aria-pressed="batchView === 'all'" @click="batchView = 'all'">全部 {{ displayedBatch.item_count }}</button>
            <button type="button" :aria-pressed="batchView === 'attention'" @click="batchView = 'attention'">需关注 {{ batchCounts.attention }}</button>
            <button type="button" :aria-pressed="batchView === 'finished'" @click="batchView = 'finished'">已结束 {{ batchCounts.ended }}</button>
          </div>
          <details class="batch-identifier"><summary>批次信息</summary><code>{{ displayedBatch.batch_id }}</code></details>
        </div>
        <div class="batch-scroll" tabindex="0" aria-label="节点测试进度列表">
        <p v-if="!visibleBatchItems.length" class="batch-empty">{{ batchView === 'attention' ? '目前没有需要处理的项目。' : '还没有已结束的节点。' }}</p>
        <div v-for="item in visibleBatchItems" :key="item.item_id" class="batch-item-row">
          <div class="batch-item-identity">
            <strong>{{ item.display_name || item.node_key }}</strong>
            <span>{{ sourceName(item.profile_id, profileOptions.find((profile) => profile.id === item.profile_id)?.name || item.profile_id) }} · {{ item.node_type || '节点' }}</span>
            <details class="batch-technical"><summary>记录信息</summary><code>{{ item.node_identity_key }} · {{ item.config_revision_key }}</code><span v-if="item.result">{{ item.result.method }} · {{ item.result.target }}</span></details>
          </div>
          <div class="batch-item-state">
            <strong>{{ item.execution_state === 'completed' && item.result?.failure_samples ? '测量完成 · 部分请求失败' : batchExecutionLabel(item.execution_state) }}</strong>
            <span>{{ batchPersistenceLabel(item.persistence_state) }}</span>
            <span v-if="item.error_message && !item.result" class="batch-error-detail">{{ item.error_message }}</span>
            <span v-if="item.persistence_error" class="batch-error-detail">{{ item.persistence_error }}</span>
            <button v-if="item.node_identity_key && item.config_revision_key" type="button" class="text-action" @click="openBatchItemDetail(item)">查看记录</button>
          </div>
          <div v-if="item.result" class="batch-item-result">
            <strong>{{ item.result.success_samples }} 成功 / {{ item.result.failure_samples }} 失败样本</strong>
            <span v-if="item.result.target === 'multi://latency-v1'">六站综合检测 · 各站延迟和失败原因见下方</span>
            <span v-else>延迟 {{ item.result.latency_ms > 0 ? `${Math.round(item.result.latency_ms)} ms` : '无有效值' }} · 波动 {{ item.result.jitter_ms }} ms</span>
            <span v-if="testFailureReason(item.result)" class="batch-error-detail">{{ item.result.success_samples ? '失败请求原因' : '失败原因' }}：{{ explainFailure(testFailureReason(item.result)) }}</span>
            <button v-if="item.persistence_state === 'failed'" type="button" class="text-action mt-1" :disabled="batchBusy" @click="retryBatchSave(item)">重试保存（不重新测速）</button>
          </div>
          <div v-else class="batch-item-result batch-no-result">
            {{ item.execution_state === 'skipped_config' ? '节点配置已变化，已跳过；请重新选择后测试。' : item.execution_state === 'cancelled' || item.execution_state === 'not_executed' || item.execution_state === 'interrupted' ? '未测，不计作节点探测失败。' : item.error_message || '等待结果' }}
          </div>
        </div>
        </div>
      </div>
    </section>

    <section class="prototype-panel comparison-panel latency-table" aria-labelledby="comparison-title">
      <div class="prototype-panel-header">
        <div>
          <h2 id="comparison-title">{{ activeProject === 'service' ? '检测结果' : nodeCategoryFilter === 'notices' ? '订阅公告与信息' : activeProject === 'latency' ? '延迟与健康走势' : '下载速度比较' }}</h2>
          <p v-if="activeProject !== 'service' && nodeCategoryFilter === 'notices'">已自动归类非代理节点条目（官网、电报群、防失联、到期提醒等）；可直接点击外链访问，不会干扰代理测速。</p>
          <p v-else-if="activeProject === 'latency'">左侧汇总最近一轮六站连通情况 · 各站延迟分别展示 · × 表示失败，不计为 0 ms</p>
          <p v-else-if="activeProject === 'throughput'">相同单位比较速度，保留每次测量的过程与条件。</p>
          <p v-if="activeProject === 'latency' && sortBy === 'recent' && (activeBatch?.items?.length || latestBatchDetail?.items?.length)" class="recent-batch-order-note">最近一次测速的节点排在前面，其余节点按延迟排序。</p>
        </div>
        <div v-if="activeProject === 'service'" class="service-result-controls">
          <label>结果时间 <ObservationWindowSelect v-model="windowMode" label="服务结果时间范围" /></label>
          <button type="button" class="prototype-button" :disabled="optionsLoading" @click="loadOptions">{{ optionsLoading ? '读取中…' : '重新读取节点' }}</button>
        </div>
        <div v-else class="category-tabs-group">
          <div class="category-tabs" role="tablist" aria-label="条目分类切换">
            <button
              type="button"
              class="category-tab"
              :class="{ active: nodeCategoryFilter === 'proxies' }"
              role="tab"
              :aria-selected="nodeCategoryFilter === 'proxies'"
              @click="nodeCategoryFilter = 'proxies'"
            >
              🚀 代理节点 ({{ proxyCount }})
            </button>
            <button
              type="button"
              class="category-tab"
              :class="{ active: nodeCategoryFilter === 'notices' }"
              role="tab"
              :aria-selected="nodeCategoryFilter === 'notices'"
              @click="nodeCategoryFilter = 'notices'"
            >
              📢 公告与信息 ({{ noticeCount }})
            </button>
            <button
              type="button"
              class="category-tab"
              :class="{ active: nodeCategoryFilter === 'all' }"
              role="tab"
              :aria-selected="nodeCategoryFilter === 'all'"
              @click="nodeCategoryFilter = 'all'"
            >
              全部 ({{ subscriptionOptions.length }})
            </button>
          </div>
          <div class="selection-actions flex items-center gap-2.5">
            <button v-if="visibleOptions.length > 0" type="button" class="selection-button" :aria-pressed="isAllVisibleSelected" @click="toggleSelectAllVisible">
              {{ isAllVisibleSelected ? '取消当前页选择' : `全选当前 ${visibleOptions.length} 个` }}
            </button>
            <span class="selection-count" :title="selectedNodeNames.join('、')">{{ selectedNodesLabel }}</span><button type="button" class="selection-button secondary" :disabled="selectedKeys.length === 0" @click="clearSelection">清空选择</button>
          </div>
        </div>
      </div>
      <div v-if="nodeCategoryFilter !== 'notices' && activeProject === 'latency'" class="comparison-head" aria-hidden="true">
        <div></div>
        <div>节点 / 订阅</div>
        <div>当前读数</div>
        <div>{{ activeProject === 'latency' ? '延迟走势' : projectLabel }}
          <div class="history-axis">
            <span>{{ historyAxisLabels[0] }}</span>
            <span>{{ historyAxisLabels[1] }}</span>
            <span>{{ historyAxisLabels[2] }}</span>
            <span>{{ historyAxisLabels[3] }}</span>
            <span>{{ historyAxisLabels[4] }}</span>
          </div>
        </div>
      </div>
      <div v-if="nodeCategoryFilter !== 'notices' && activeProject === 'latency'" class="history-legend" aria-label="历史图例">
        <span class="legend-item"><i class="legend-mark"></i>成功</span>
        <span class="legend-item"><i class="legend-mark fail"></i>失败</span>
        <span v-if="activeProject === 'latency'" class="legend-item"><i class="legend-mark timeout"></i>超时</span>
        <span class="legend-item"><i class="legend-mark missing"></i>无记录</span>
      </div>

      <div v-if="optionsError" class="prototype-state error-state"><strong>节点列表读取失败</strong><span>{{ optionsError }}</span><button type="button" class="prototype-button" @click="loadOptions">重试读取</button></div>
      <div v-else-if="optionsLoading && options.length === 0" class="prototype-state"><strong>正在读取节点与历史</strong><span>不显示旧的成功结果覆盖当前状态。</span></div>
      <div v-else-if="options.length === 0" class="prototype-state"><strong>还没有可比较的节点</strong><span>先刷新订阅；节点必须来自真实缓存配置。</span></div>
      <div v-else-if="activeProject === 'service' ? comparisonRows.length === 0 : visibleOptions.length === 0" class="prototype-state">
        <strong>{{ activeProject !== 'service' && nodeCategoryFilter === 'notices' ? '暂无非节点或公告条目' : '当前筛选没有节点' }}</strong>
        <span>{{ activeProject !== 'service' && nodeCategoryFilter === 'notices' ? '当前订阅中所有条目均为标准代理节点。' : activeProject === 'service' ? '可以在上方选择其他来源、地区或清空节点搜索。' : '可以清空搜索、切换订阅范围或切换条目类型。' }}</span>
      </div>

      <!-- 专有非节点公告与链接页面 -->
      <div v-else-if="activeProject !== 'service' && nodeCategoryFilter === 'notices'" class="notice-page-wrap">
        <div class="notice-page-banner">
          <div class="notice-page-banner-icon">📢</div>
          <div class="notice-page-banner-content">
            <strong>已自动隔离 {{ visibleOptions.length }} 个非代理节点条目</strong>
            <p>这些条目通常为机场订阅自带的官方网站、电报群/频道、GitHub 防失联发布页、到期与剩余流量提醒、使用规则等。它们已被默认从节点列表与测速批次中隔离，避免测试时误测或超时。</p>
          </div>
        </div>

        <ul class="notice-list" role="list" aria-label="订阅公告与信息列表">
          <li v-for="node in visibleOptions" :key="scopeKey(node)" class="notice-item-card">
            <div class="notice-item-main">
              <div class="notice-item-icon">📢</div>
              <div class="notice-item-info">
                <strong class="notice-item-title">{{ node.displayName }}</strong>
                <div class="notice-item-meta">
                  <span class="notice-profile-badge">{{ sourceName(node.profileId, node.profileName) }}</span>
                  <span class="notice-type-badge">{{ node.type || '非节点' }}</span>
                  <span v-if="node.countryCode && node.countryCode !== 'OTHER'" class="notice-country-badge">{{ node.countryCode }}</span>
                </div>
                <div v-if="extractNoticeLinks(node.displayName).length" class="notice-links-row">
                  <button class="notice-link-btn" :disabled="noticeImportBusy" @click="saveNoticeEntrypoints(node)">＋加入机场管理</button>
                  <a
                    v-for="link in extractNoticeLinks(node.displayName)"
                    :key="link.url"
                    :href="link.url"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="notice-link-btn"
                    :class="link.kind"
                  >
                    <span v-if="link.kind === 'telegram'">✈️</span>
                    <span v-else-if="link.kind === 'github'">🐱</span>
                    <span v-else-if="link.kind === 'backup'">🛡️</span>
                    <span v-else>🌐</span>
                    {{ link.label }}：{{ link.url }} ↗
                  </a>
                </div>
                <p v-if="noticeImportFeedback[scopeKey(node)]" role="status">{{ noticeImportFeedback[scopeKey(node)] }}</p>
              </div>
            </div>
            <div class="notice-item-actions">
              <button type="button" class="prototype-button" @click="toggleNoticeOverride(scopeKey(node), false)">
                恢复为代理节点
              </button>
            </div>
          </li>
        </ul>
      </div>

      <ServiceComparison v-else-if="activeProject === 'service'" :rows="comparisonRows" :services="selectedServiceOptions" :catalog="publicServiceCatalog" :records="visibleServices" :selected="selectedKeys" :states="projectHistoryStateByKey" :partial="projectHistoryMetaByKey" @toggle="toggleSelected" @service="barServiceId = $event" @detail="openNodeDetail" />
      <DownloadComparison v-else-if="activeProject === 'throughput'" :rows="comparisonRows" :records="visibleDownloads" :selected="selectedKeys" :states="projectHistoryStateByKey" :partial="projectHistoryMetaByKey" @toggle="toggleSelected" @detail="openNodeDetail" />
      <ul v-else class="node-list" role="listbox" aria-label="节点列表">
        <li v-for="node in visibleOptions" :key="scopeKey(node)" class="node-row" :class="{ 'is-focused': focusedKey === scopeKey(node), 'no-latency-history': activeProject === 'latency' && !allSiteTestsForKey(scopeKey(node)).length && !isTesting(scopeKey(node)), 'latency-health': activeProject === 'latency' && !!nodeHealthReport(scopeKey(node)), 'compact-mode-row': activeProject === 'latency' && workbenchViewMode === 'compact' && !expandedKeys.has(scopeKey(node)) }" :aria-selected="isSelected(scopeKey(node))">
                    <!-- Compact View for Latency -->
          <template v-if="activeProject === 'latency' && workbenchViewMode === 'compact' && !expandedKeys.has(scopeKey(node))">
            <label class="select-cell compact-cell-check" :aria-label="`选择 ${node.displayName}`" @click.stop>
              <input type="checkbox" :checked="isSelected(scopeKey(node))" @change="toggleSelected(scopeKey(node))">
            </label>
            <div class="compact-cell-main" @click="focusNode(scopeKey(node))">
              <span class="compact-flag">{{ node.countryFlag || '🌐' }}</span>
              <strong class="compact-name" :title="node.displayName">{{ node.displayName || '未命名节点' }}</strong>
              <span class="compact-tag">{{ node.type || '节点' }}</span>
              <span class="compact-meta">{{ sourceName(node.profileId, node.profileName) }}</span>
              <span v-if="downloadResultByKey[scopeKey(node)]" class="composite-badge download">↓ {{ downloadResultByKey[scopeKey(node)].speedMbps }}M</span>
              <span v-if="serviceSummaryBadge(scopeKey(node))" class="composite-badge service">{{ serviceSummaryBadge(scopeKey(node)) }}</span>
            </div>
            <div class="compact-cell-metrics">
              <template v-if="isTesting(scopeKey(node))">
                <span class="row-readout-state running">测试中…</span>
              </template>
              <template v-else-if="latestTestForKey(scopeKey(node))">
                <div class="compact-badge" :class="compactLatencyClass(scopeKey(node))">
                  <span>{{ latestTestForKey(scopeKey(node))!.latency_ms }} ms</span>
                </div>
                <span class="compact-health-text">
                  {{ suiteHealthForKey(scopeKey(node)) ? `${suiteHealthForKey(scopeKey(node))!.success}/6 连通` : (latestTestForKey(scopeKey(node))!.status !== 'failed' ? '成功' : '失败') }}
                </span>
              </template>
              <template v-else>
                <span class="compact-badge nodata">未测试</span>
              </template>
            </div>
            <div class="compact-cell-actions">
              <button type="button" class="row-action-btn" :disabled="batchBusy" @click.stop="runTest([scopeKey(node)])">⚡ 测速</button>
              <button type="button" class="row-action-btn" title="展开六站走势图与详细样本" @click.stop="toggleRowExpanded(scopeKey(node))">展开走势 ▼</button>
              <button type="button" class="row-action-btn" title="查看节点完整配置与详情" @click.stop="openHistory(scopeKey(node), 'config')">详情</button>
            </div>
          </template>
          <template v-else>
            <label class="select-cell" :aria-label="`选择 ${node.displayName}`" @click.stop><input type="checkbox" :checked="isSelected(scopeKey(node))" @change="toggleSelected(scopeKey(node))"></label>
          <div class="node-evidence" @click="focusNode(scopeKey(node))">
            <span class="node-name"><strong :title="node.displayName">{{ node.displayName || '未命名节点' }}</strong></span>
            <span class="node-meta">
              <span>{{ sourceName(node.profileId, node.profileName) }}</span>
              <span>{{ node.countryCode || '未知地区' }}</span>
              <span>{{ node.type || '节点' }}</span>
              <span v-if="monitoredNodeKeys.has(scopeKey(node))" class="badge-monitoring" title="该节点已被持续监测任务包含并正在定期探测">● 监测中</span>
              <span v-if="isNoticeNode(node, noticeOverrides, scopeKey(node))" class="notice-tag">📢 非节点/公告</span>
            </span>
            <div class="node-row-actions">
              <button
                type="button"
                class="row-action-btn"
                title="查看节点完整配置与详情"
                @click.stop="openHistory(scopeKey(node), 'config')"
              >
                详情
              </button>
              <div class="row-menu-dropdown-wrapper" @click.stop>
                <button
                  type="button"
                  class="row-action-btn icon-only"
                  :title="isNoticeNode(node, noticeOverrides, scopeKey(node)) ? '节点管理选项（当前处于公告状态）' : '节点管理选项'"
                  :aria-expanded="openMenuKey === scopeKey(node)"
                  @click="toggleRowMenu(scopeKey(node))"
                >
                  ⋯
                </button>
                <div v-if="openMenuKey === scopeKey(node)" class="row-menu-popover">
                  <button
                    type="button"
                    class="row-menu-item"
                    @click="handleToggleNotice(node)"
                  >
                    <span>{{ isNoticeNode(node, noticeOverrides, scopeKey(node)) ? '↩ 移回代理列表' : '📢 移至公告与信息' }}</span>
                  </button>
                </div>
              </div>
            </div>
            <div v-if="downloadResultByKey[scopeKey(node)] || serviceSummaryBadge(scopeKey(node))" class="node-composite-badges">
              <span v-if="downloadResultByKey[scopeKey(node)]" class="composite-badge download" :title="`最新下载测速：${downloadResultByKey[scopeKey(node)].speedMbps} Mbps`">
                ↓ {{ downloadResultByKey[scopeKey(node)].speedMbps }} Mbps
              </span>
              <span v-if="serviceSummaryBadge(scopeKey(node))" class="composite-badge service" :title="serviceDetailTitle(scopeKey(node))">
                {{ serviceSummaryBadge(scopeKey(node)) }}
              </span>
            </div>
          </div>
          <div class="row-readout" :data-state="readoutState(scopeKey(node))">
            <template v-if="isTesting(scopeKey(node))">
              <span class="row-readout-state running">测试中</span>
              <strong class="row-readout-value">…</strong>
              <span class="row-readout-time">正在等待真实结果</span>
            </template>
            <template v-else-if="activeProject === 'latency'">
              <div v-if="suiteHealthForKey(scopeKey(node))" class="suite-health-side">
                <span>六站综合 · 最近一轮</span>
                <strong>{{ suiteHealthForKey(scopeKey(node))!.label }}</strong>
                <dl>
                  <div><dt>成功请求</dt><dd>{{ suiteHealthForKey(scopeKey(node))!.success }} / {{ suiteHealthForKey(scopeKey(node))!.count }}</dd></div>
                  <div v-for="site in suiteHealthForKey(scopeKey(node))!.failures" :key="site.label" class="suite-failure-site"><dt>{{ site.label }}</dt><dd>失败 {{ site.failed }}/{{ site.count }} 次</dd></div>
                </dl>
                <small v-if="suiteHealthForKey(scopeKey(node))!.failures.length">其余 {{ suiteHealthForKey(scopeKey(node))!.complete }} 站全部成功</small>
                <small>{{ new Date(suiteHealthForKey(scopeKey(node))!.time).toLocaleTimeString() }} · 已测 {{ suiteHealthForKey(scopeKey(node))!.tested }}/6 站</small>
                <p>超时不等于节点故障；历史统计见右侧各站记录。</p>
              </div>
              <div v-else class="readout-observation">
                <div class="readout-header">
                  <span class="row-readout-state">{{ usesAttemptAverage(scopeKey(node)) ? '基准平均 · Cloudflare' : readoutState(scopeKey(node)) }}</span>
                  <span class="row-readout-time">{{ readoutTime(scopeKey(node)) }}</span>
                </div>
                <div class="readout-main-val">
                  <strong class="row-readout-value" :class="usesAttemptAverage(scopeKey(node)) ? 'average' : latencyValueClass(scopeKey(node))">
                    {{ usesAttemptAverage(scopeKey(node)) ? comparisonForKey(scopeKey(node)).currentMs : readoutValue(scopeKey(node)) }}
                    <small v-if="usesAttemptAverage(scopeKey(node)) || readoutHasLatencyUnit(scopeKey(node))">ms</small>
                  </strong>
                  <span v-if="pinnedByKey[scopeKey(node)] !== null && pinnedByKey[scopeKey(node)] !== undefined" class="pin-control">
                    <button type="button" @click.stop="clearPin(scopeKey(node))">取消固定</button>
                  </span>
                </div>
              </div>
              <div v-if="!nodeHealthReport(scopeKey(node))" class="readout-no-history">
                <span class="row-readout-status">{{ historyLoadState(scopeKey(node)) === 'loading' ? '历史读取中…' : historyLoadState(scopeKey(node)) === 'error' ? '历史读取失败' : statusLabel(scopeKey(node)) }}</span>
              </div>
            </template>
            <template v-else>
              <span class="row-readout-state">{{ projectLabel }}</span>
              <strong class="row-readout-value" :class="selectedProjectPoint(scopeKey(node))?.success ? 'success' : 'nodata'">
                {{ projectReadout(scopeKey(node)) }}
              </strong>
              <span class="row-readout-time">{{ projectReadoutTime(scopeKey(node)) }}</span>
              <span v-if="projectPinnedByKey[scopeKey(node)] != null" class="pin-control"><button type="button" @click.stop="projectPinnedByKey[scopeKey(node)] = null">取消固定</button></span>
            </template>
          </div>
          <div class="history-cell">
            <template v-if="activeProject === 'latency'">
              <div v-if="allSiteTestsForKey(scopeKey(node)).length" class="latency-site-results">
                <div v-for="site in siteResultsForKey(scopeKey(node))" :key="site.value" class="latency-site-result" :class="{ failed: site.count > site.successes }">
                  <span>{{ site.label }}</span><strong>{{ site.count ? site.latency === null ? '失败' : `${site.latency} ms` : '未检测' }}</strong>
                  <small v-if="site.count">成功 {{ site.successes }}/{{ site.count }} · {{ new Date(site.time).toLocaleTimeString() }}</small>
                  <details v-if="site.error"><summary>失败原因</summary>{{ site.error }}</details>
                </div>
              </div>
              <div v-if="!samplesForKey(scopeKey(node)).length && historyLoadState(scopeKey(node)) === 'loading'" class="history-loading-cell" role="status">正在读取该节点的历史…</div>
              <div v-else-if="!samplesForKey(scopeKey(node)).length && historyLoadState(scopeKey(node)) === 'error'" class="history-loading-cell error" role="alert">历史读取失败，请点击上方“重新读取”</div>
              <MultiSiteLatencyTrend v-if="allSiteTestsForKey(scopeKey(node)).some(test => test.target === 'multi://latency-v1' || (test.target && test.target !== 'https://speed.cloudflare.com/__down?bytes=1'))" :tests="allSiteTestsForKey(scopeKey(node))" :since="activeWindow.since" :until="activeWindow.until" />
              <LatencySamplePlot
                v-else-if="samplesForKey(scopeKey(node)).length"
                :samples="samplesForKey(scopeKey(node))"
                :window-since="activeWindow.since"
                :window-until="activeWindow.until"
                :hovered-index="hoveredByKey[scopeKey(node)] ?? null"
                :pinned-index="pinnedByKey[scopeKey(node)] ?? null"
                :height="132"
                :show-time-bounds-labels="false"
                @hover="onHover(scopeKey(node), $event)"
                @pin="onPin(scopeKey(node), $event)"
                @unpin="clearPin(scopeKey(node))"
                @bucket-hover="onBucketHover(scopeKey(node), $event)"
                @bucket-click="onBucketClick(scopeKey(node), $event)"
              />
              <p v-if="nodeIssueForKey(scopeKey(node))" class="row-test-issue" role="status">{{ nodeIssueForKey(scopeKey(node)) }}</p>
              <details v-if="nodeHealthReport(scopeKey(node)) && !suiteHealthForKey(scopeKey(node))" class="row-health-overview" :class="nodeHealthReport(scopeKey(node))!.trendClass" open>
                <summary>Cloudflare 同站历史 · 延迟区间、波动与样本分布（不代表六站整体）</summary>
                <div class="row-health-summary">
                  <span class="latency-suite-hint">Cloudflare 基准健康</span>
                  <div class="row-health-heading health-reason-trigger" tabindex="0" :aria-label="compactHealthLabel(scopeKey(node))" :aria-describedby="`health-reason-${encodeURIComponent(scopeKey(node))}`">
                    <strong>{{ compactHealthLabel(scopeKey(node)) }}</strong>
                    <span class="health-reason-tooltip" role="tooltip" :id="`health-reason-${encodeURIComponent(scopeKey(node))}`">{{ nodeHealthReport(scopeKey(node))!.diagnosisDetail }}</span>
                  </div>
                  <div class="health-key-metrics">
                    <div class="health-key-metric"><span>平时延迟</span><strong>{{ normalLatencyText(nodeHealthReport(scopeKey(node))!) }}</strong></div>
                    <div class="health-key-metric"><span>较慢时</span><strong>{{ nodeHealthReport(scopeKey(node))!.p95 !== null ? `${nodeHealthReport(scopeKey(node))!.p95} ms` : '无成功样本' }}</strong></div>
                    <div class="health-key-metric"><span>连通</span><strong :class="nodeHealthReport(scopeKey(node))!.successRateClass">{{ nodeHealthReport(scopeKey(node))!.successRateText }}</strong></div>
                    <div class="health-key-metric"><span>明显卡顿</span><strong>{{ nodeHealthReport(scopeKey(node))!.spikeCount }} 次</strong></div>
                  </div>
                  <button type="button" class="row-health-detail" @click.stop="openHistory(scopeKey(node), 'health')">详情 →</button>
                </div>
                <div v-if="comparisonForKey(scopeKey(node)).priorCount > 0" class="row-health-comparisons">
                  <template v-if="comparisonForKey(scopeKey(node)).priorCount > 0">
                    <div class="row-health-compare"><span>比上次</span><strong>{{ comparisonChange(comparisonForKey(scopeKey(node)).currentMs, comparisonForKey(scopeKey(node)).previousMs, comparisonForKey(scopeKey(node)).previousExists ? '上次未成功' : '还没有上次') }}</strong><small v-if="comparisonForKey(scopeKey(node)).previousMs !== null">上次 {{ comparisonForKey(scopeKey(node)).previousMs }} ms</small></div>
                    <div class="row-health-compare"><span>比此前平均</span><strong>{{ comparisonChange(comparisonForKey(scopeKey(node)).currentMs, comparisonForKey(scopeKey(node)).priorAverageMs) }}</strong><small v-if="comparisonForKey(scopeKey(node)).priorAverageMs !== null">此前平均 {{ comparisonForKey(scopeKey(node)).priorAverageMs }} ms</small></div>
                    <div class="row-health-compare"><span>比此前最慢一轮</span><strong>{{ comparisonChange(comparisonForKey(scopeKey(node)).currentMs, comparisonForKey(scopeKey(node)).priorWorstMs) }}</strong><small v-if="comparisonForKey(scopeKey(node)).priorWorstMs !== null">此前最慢 {{ comparisonForKey(scopeKey(node)).priorWorstMs }} ms</small></div>
                  </template>
                </div>
                <div class="row-health-expanded">
                  <div class="row-health-extra">
                    <span :title="`中位数 ${nodeHealthReport(scopeKey(node))!.p50 ?? '—'} ms · 常见区间占 ${nodeHealthReport(scopeKey(node))!.normalPercentage}%`">最快—最慢 <b>{{ nodeHealthReport(scopeKey(node))!.minLatency ?? '—' }}—{{ nodeHealthReport(scopeKey(node))!.maxLatency ?? '—' }} ms</b></span>
                    <span v-if="nodeHealthReport(scopeKey(node))!.failCount" class="health-failure-note">失败 {{ nodeHealthReport(scopeKey(node))!.failCount }} 次</span>
                  </div>
                  <div v-if="nodeHealthReport(scopeKey(node))!.histogramBins.length" class="row-health-distribution">
                    <span class="row-health-distribution-title">延迟分布</span>
                    <div class="row-health-bin-grid">
                      <div v-for="bin in nodeHealthReport(scopeKey(node))!.histogramBins" :key="bin.label" class="row-health-bin">
                        <span>{{ bin.label }}</span><span>{{ bin.percentage }}% · {{ bin.count }} 次</span>
                        <div class="row-health-bin-track"><i :style="{ width: `${bin.percentage}%` }" :class="{ normal: bin.isNormalRange }"></i></div>
                      </div>
                    </div>
                  </div>
                  <p class="row-health-scope">{{ windowLabel }} · {{ nodeHealthReport(scopeKey(node))!.testCount }} 次测试 · {{ nodeHealthReport(scopeKey(node))!.sampleCount }} 条样本<span v-if="!historyMetaForKey(scopeKey(node))?.complete || historyMetaForKey(scopeKey(node))?.hasMore"> · 仅已加载记录</span></p>
                </div>
              </details>
              <div v-else-if="!suiteHealthForKey(scopeKey(node))" class="history-cell-footer flex items-center justify-between mt-1 text-[11px] px-1">
                <span class="stats-label text-content-secondary font-mono font-medium">{{ statsLabel(scopeKey(node)) }}</span>
                <button type="button" class="history-open text-primary hover:underline font-semibold" @click.stop="openHistory(scopeKey(node), 'health')">健康与走势 →</button>
              </div>
            </template>
            <template v-else>
              <div v-if="projectHistoryStateByKey[scopeKey(node)] === 'loading'" class="history-loading-cell" role="status">正在读取历史…</div>
              <div v-else-if="projectHistoryStateByKey[scopeKey(node)] === 'error'" class="history-loading-cell error" role="alert">历史读取失败，请重新读取</div>
              <template v-else>
                <MetricHistoryPlot
                  :points="projectPointsFor(scopeKey(node))"
                  :unit="activeProject === 'throughput' ? 'Mbps' : 'ms'"
                  :window-since="activeWindow.since" :window-until="activeWindow.until"
                  :hovered-index="projectHoveredByKey[scopeKey(node)] ?? null"
                  :pinned-index="projectPinnedByKey[scopeKey(node)] ?? null"
                  @hover="projectHoveredByKey[scopeKey(node)] = $event"
                  @pin="projectPinnedByKey[scopeKey(node)] = $event"
                  @unpin="projectPinnedByKey[scopeKey(node)] = null"
                />
                <div class="metric-row-footer">
                  <span v-if="projectPointsFor(scopeKey(node)).length">{{ projectHistoryText(scopeKey(node)) }}</span>
                  <span>{{ projectPointsFor(scopeKey(node)).length }} 次{{ activeProject === 'service' ? ` · ${barServiceOptions.find(option => option.value === barServiceId)?.label || barServiceId}` : '' }}{{ projectHistoryMetaByKey[scopeKey(node)]?.hasMore ? ' · 仅近 100 条' : '' }}</span>
                </div>
              </template>
            </template>
          </div>
          </template>
        </li>
      </ul>
      <div class="compare-footer">
        <span v-if="activeProject === 'service'">当前范围 {{ comparisonRows.length }} / {{ servicePickerNodes.length }} 条线路 · 默认显示已测结果</span>
        <span v-else>显示 {{ visibleOptions.length }} / {{ subscriptionOptions.length }} 个条目<template v-if="nodeCategoryFilter === 'proxies' && noticeCount > 0">（{{ noticeCount }} 个公告条目已隔离）</template></span>
        <span>{{ historyLoading ? '历史读取中…' : historyError || '指标基于当前窗口内全部真实测试样本' }}</span>
      </div>
    </section>

    <div v-if="expanded && (focusedDisplayedTest || focusedOption)" class="prototype-modal-backdrop" @click.self="closeHistory">
      <section class="prototype-history-modal" role="dialog" aria-modal="true" aria-labelledby="history-modal-title">
        <div class="modal-header">
          <div>
            <p class="prototype-eyebrow">节点详情</p>
            <h2 id="history-modal-title">{{ focusedOption?.displayName }} · {{ modalActiveTab === 'health' ? '健康概览' : modalActiveTab === 'config' ? '节点配置与订阅信息' : '延迟历史走势' }}</h2>
            <p>{{ focusedOption ? sourceName(focusedOption.profileId, focusedOption.profileName) : '未知来源' }} · {{ focusedOption?.countryCode || '未知地区' }}</p>
          </div>
          <div class="modal-actions">
            <button v-if="activeProject === 'latency'" type="button" class="prototype-button primary" :disabled="batchBusy" @click="runTest([focusedKey])">{{ batchBusy ? '测试中…' : '⚡ 立即测试' }}</button>
            <button v-if="pinnedByKey[focusedKey] !== null && pinnedByKey[focusedKey] !== undefined" type="button" class="prototype-button" @click="clearPin(focusedKey)">取消固定</button>
            <button type="button" class="close-button" aria-label="关闭历史详情" @click="closeHistory">×</button>
          </div>
        </div>

        <div class="modal-tabs">
          <button
            type="button"
            class="modal-tab-btn"
            :class="{ active: modalActiveTab === 'chart' }"
            @click="modalActiveTab = 'chart'"
          >
            延迟走势
          </button>
          <button
            type="button"
            class="modal-tab-btn"
            :class="{ active: modalActiveTab === 'health' }"
            @click="modalActiveTab = 'health'"
          >
            健康概览
          </button>
          <button
            type="button"
            class="modal-tab-btn"
            :class="{ active: modalActiveTab === 'config' }"
            @click="modalActiveTab = 'config'"
          >
            节点信息
          </button>
        </div>

        <template v-if="focusedDisplayedTest || samplesForKey(focusedKey).length > 0">
          <div v-show="modalActiveTab !== 'config'" class="history-controls">
            <label>时间范围<ObservationWindowSelect v-model="windowMode" label="历史时间范围" /></label>
            <span>{{ windowLabel }} · 截至 {{ formatShortTime(activeWindow.until) }} · {{ modalActiveTab === 'health' ? '根据此范围内已加载的测试记录分析' : '横轴为真实采样时间 · 纵轴为毫秒' }}</span>
            <span v-if="detailLoading">正在读取详情…</span>
          </div>

          <div v-show="modalActiveTab === 'chart'" class="modal-chart-tab-content">
            <div class="modal-chart">
              <div class="history-layer-label mb-1">宏观时间线走势 · 左右为不同时间点（共 {{ samplesForKey(focusedKey).length }} 条原始样本，可上下滑动鼠标或使用滚轮精准微调）</div>
              <LatencySamplePlot
                :samples="samplesForKey(focusedKey)"
                :window-since="activeWindow.since"
                :window-until="activeWindow.until"
                :hovered-index="hoveredByKey[focusedKey] ?? null"
                :pinned-index="pinnedByKey[focusedKey] ?? null"
                :width="1120"
                :height="260"
                :show-time-bounds-labels="true"
                @hover="onHover(focusedKey, $event)"
                @pin="onPin(focusedKey, $event)"
                @unpin="clearPin(focusedKey)"
                @bucket-hover="onBucketHover(focusedKey, $event)"
                @bucket-click="onBucketClick(focusedKey, $event)"
              />
            </div>
            <div v-if="focusedDisplayedTest?.samples?.length" class="modal-intra-chart mt-3">
              <div class="history-layer-label mb-1">选定测试内样本时序（{{ formatTime(focusedDisplayedTest.started_at) }} · {{ focusedDisplayedTest.success_samples }}/{{ focusedDisplayedTest.total_samples }} 成功）</div>
              <IntraTestSamplePlot mode="latency" :latency-samples="focusedDisplayedTest.samples" :height="110" :compact="false" />
            </div>
            <div class="modal-current">
              <strong>{{ sampleLabel(activeSampleFor(focusedKey)) }}</strong>
              <span>{{ sampleDetail(activeSampleFor(focusedKey)) }}</span>
              <span>{{ focusedDisplayedTest ? `${testStatusLabel(focusedDisplayedTest)} · ${persistenceLabel(focusedDisplayedTest)}` : '' }}</span>
            </div>
            <div v-if="focusedDisplayedTest?.samples?.length" class="modal-table-wrap">
              <table>
                <thead>
                  <tr><th>样本</th><th>实际时间</th><th>结果</th><th>原因</th></tr>
                </thead>
                <tbody>
                  <tr v-for="(sample, index) in focusedDisplayedTest.samples" :key="`${sample.seq}-${sample.timestamp}`" :class="{ selected: activeIndexFor(focusedKey, focusedDisplayedTest.samples) === index, 'cursor-pointer': true }" @click="onModalTableRowClick(sample)">
                    <td>#{{ sample.seq }}</td>
                    <td>{{ formatTime(sample.timestamp) }}</td>
                    <td :class="sample.success ? 'success' : 'fail'">{{ sample.success ? `${Math.round(sample.latency_ms)} ms` : sampleLabel(sample) }}</td>
                    <td>{{ sample.error || '—' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div v-if="focusedTests.length > 1" class="modal-history-list">
              <div class="history-layer-label mb-1">切换历史测试批次（共 {{ focusedTests.length }} 条记录）</div>
              <div class="modal-history-pills">
                <button
                  v-for="test in focusedTests"
                  :key="test.attempt_id"
                  type="button"
                  class="modal-history-pill"
                  :class="{ active: selectedAttemptID === test.attempt_id }"
                  @click="selectHistory(test)"
                >
                  <span>{{ testStatusLabel(test) }} · {{ formatTime(test.finished_at) }}</span>
                  <strong>{{ test.latency_ms > 0 ? `${Math.round(test.latency_ms)} ms` : '失败' }}</strong>
                </button>
              </div>
            </div>
          </div>

          <div v-show="modalActiveTab === 'health'" class="modal-health-tab-content">
            <div v-if="focusedHealthReport" class="health-view-body">
              <div class="health-diagnosis-card" :class="focusedHealthReport.trendClass">
                <div class="diagnosis-badge">
                  {{ focusedHealthReport.diagnosisTitle }}
                </div>
                <div class="diagnosis-text">
                  <h4>{{ focusedHealthReport.diagnosisDetail }}</h4>
                  <p class="diagnosis-sub">
                    {{ windowLabel }} 内已加载 <strong>{{ focusedHealthReport.sampleCount }}</strong> 条样本，这些样本的请求成功率为 <strong>{{ focusedHealthReport.successRateText }}</strong>。
                  </p>
                </div>
              </div>
              <p v-if="nodeIssueForKey(focusedKey)" class="row-test-issue modal-test-issue">{{ nodeIssueForKey(focusedKey) }}</p>

              <div class="health-grid-cards">
                <div class="health-card highlight">
                  <span class="card-label">平时延迟区间</span>
                  <strong class="card-val font-mono text-primary">{{ focusedHealthReport.normalRangeText }}</strong>
                  <span class="card-desc">成功样本最集中的范围；占比见上方，不一定超过一半</span>
                </div>
                <div class="health-card">
                  <span class="card-label">连接通畅率</span>
                  <strong class="card-val font-mono" :class="focusedHealthReport.successRateClass">{{ focusedHealthReport.successRateText }}</strong>
                  <span class="card-desc">成功 {{ focusedHealthReport.successCount }} 次 · 失败 {{ focusedHealthReport.failCount }} 次</span>
                </div>
                <div class="health-card">
                  <span class="card-label">较慢时的延迟</span>
                  <strong class="card-val font-mono">{{ focusedHealthReport.p95 !== null ? `${focusedHealthReport.p95} ms` : '—' }}</strong>
                  <span class="card-desc">约 95% 的成功样本不超过此值；不是最慢的一次</span>
                </div>
              </div>
              <div class="health-more"><h3 class="health-section-title">延迟波动与变化</h3><div class="health-grid-cards">
                <div class="health-card">
                  <span class="card-label">平时大概延迟</span>
                  <strong class="card-val font-mono">{{ focusedHealthReport.p50 !== null ? `${focusedHealthReport.p50} ms` : '—' }}</strong>
                  <span class="card-desc">最好 {{ focusedHealthReport.minLatency ?? '—' }}ms · 最差 {{ focusedHealthReport.maxLatency ?? '—' }}ms</span>
                </div>
                <div class="health-card">
                  <span class="card-label">明显变慢的样本</span>
                  <strong class="card-val font-mono" :class="{ 'text-danger': focusedHealthReport.spikeCount > 0 }">{{ focusedHealthReport.spikeRateText }}</strong>
                  <span class="card-desc">共 {{ focusedHealthReport.spikeCount }} 次延迟明显高于平时</span>
                </div>
                <div class="health-card">
                  <span class="card-label">综合稳定性</span>
                  <strong class="card-val">{{ focusedHealthReport.trendText }}</strong>
                  <span class="card-desc">{{ focusedHealthReport.trend === 'insufficient' ? '继续积累测试后再比较' : `按样本先后分成两半：${focusedHealthReport.driftRatio > 0.05 ? `后半延迟上升约 ${Math.round(focusedHealthReport.driftRatio * 100)}%` : focusedHealthReport.driftRatio < -0.05 ? `后半延迟降低约 ${Math.abs(Math.round(focusedHealthReport.driftRatio * 100))}%` : '两部分延迟接近'}` }}</span>
                </div>
              </div></div>

              <div class="health-histogram-section">
                <div class="histogram-head">
                  <h3>延迟通常落在哪</h3>
                  <span>横条越长，说明这个延迟范围出现得越多。</span>
                </div>
                <div v-if="focusedHealthReport.histogramBins.length" class="histogram-bars">
                  <div
                    v-for="bin in focusedHealthReport.histogramBins"
                    :key="bin.label"
                    class="histogram-bar-row"
                    :class="{ 'is-normal': bin.isNormalRange }"
                  >
                    <span class="bin-label font-mono">{{ bin.label }}</span>
                    <div class="bin-track">
                      <div
                        class="bin-fill"
                        :style="{ width: `${Math.max(bin.percentage > 0 ? 3 : 0, bin.percentage)}%` }"
                        :class="{ 'fill-normal': bin.isNormalRange }"
                      ></div>
                    </div>
                    <span class="bin-percent font-mono">{{ bin.percentage }}%</span>
                    <span class="bin-count font-mono">({{ bin.count }}次)</span>
                    <span v-if="bin.isNormalRange" class="bin-tag">最常出现</span>
                  </div>
                </div>
                <div v-else class="histogram-empty">
                  没有成功的延迟样本，因此无法绘制延迟分布；失败原因请看上方节点测试记录。
                </div>
              </div>
            </div>
            <div v-else class="modal-empty-state">
              <h3>暂无足够的样本数据用于健康评估</h3>
              <p v-if="nodeIssueForKey(focusedKey)">{{ nodeIssueForKey(focusedKey) }}</p>
            </div>
          </div>
        </template>

        <div v-show="modalActiveTab === 'config'" class="modal-config-tab-content">
          <div class="modal-config-card">
            <h3 class="text-sm font-bold text-content-main mb-3">节点基本信息与订阅配置</h3>
            <div class="node-config-grid">
              <div class="config-item">
                <span class="config-label">节点名称</span>
                <span class="config-val font-semibold text-content-main">{{ focusedOption?.displayName || '未命名节点' }}</span>
              </div>
              <div class="config-item">
                <span class="config-label">所属订阅</span>
                <span class="config-val">{{ focusedOption ? sourceName(focusedOption.profileId, focusedOption.profileName) : '—' }}</span>
              </div>
              <div class="config-item">
                <span class="config-label">协议类型</span>
                <span class="config-val font-mono uppercase">{{ focusedOption?.type || '未知' }}</span>
              </div>
              <div class="config-item">
                <span class="config-label">地区出口</span>
                <span class="config-val">{{ focusedOption?.countryFlag }} {{ focusedOption?.countryCode || '未知地区' }}</span>
              </div>
            </div>
            <details class="advanced-details"><summary>高级信息 · 身份与原始记录</summary>
              <p>节点标识用于区分同名节点，避免把不同节点的历史混在一起。配置版本用于区分同一节点修改前后的设置，防止旧结果被当成当前表现。</p>
              <div class="node-config-grid">
              <div class="config-item">
                <span class="config-label">配置版本</span>
                <span class="config-val font-mono text-xs">{{ focusedOption?.configRevisionKey || '—' }}</span>
              </div>
              <div class="config-item">
                <span class="config-label">节点唯一标识</span>
                <span class="config-val font-mono text-xs">{{ focusedOption?.nodeIdentityKey || focusedOption?.nodeKey || '—' }}</span>
              </div>
            </div>
              <p>原始记录用于核对每次测试的来源、结果和保存状态；日常选节点看“健康概览”和“延迟走势”即可。</p>
            <div class="config-actions-row mt-4 pt-3 border-t border-border flex items-center gap-3">
              <button
                type="button"
                class="prototype-button"
                @click="focusedOption && handleToggleNotice(focusedOption)"
              >
                {{ focusedOption && isNoticeNode(focusedOption, noticeOverrides, scopeKey(focusedOption)) ? '↩ 移回代理列表' : '📢 移至公告与信息' }}
              </button>
              <button
                v-if="focusedOption"
                type="button"
                class="prototype-button"
                @click="closeHistory(); openNodeDetail(focusedOption, focusedDisplayedTest ? { kind: 'workbench_attempt', attemptId: focusedDisplayedTest.attempt_id, observedAt: focusedDisplayedTest.finished_at, snapshot: focusedDisplayedTest } : undefined)"
              >
                查看原始记录与保存状态
              </button>
            </div>
            </details>
          </div>
        </div>

        <div v-if="modalActiveTab !== 'config' && (!focusedDisplayedTest && samplesForKey(focusedKey).length === 0)" class="modal-empty-state">
          <div class="modal-empty-icon">📊</div>
          <h3>该节点尚未执行延迟测试</h3>
          <p>暂无真实采样历史数据。点击下方按钮立即测试此节点，获取实时 HTTP Ping 延迟和真实采样证据。</p>
          <div class="modal-empty-actions">
            <button type="button" class="prototype-button primary" :disabled="batchBusy" @click="runTest([focusedKey])">
              {{ batchBusy ? '批次执行中…' : '⚡ 立即测试此节点' }}
            </button>
          </div>
        </div>
      </section>
    </div>

    <!-- 组合测试弹出框 / Modal -->
    <div v-if="showCompositeModal" class="prototype-modal-backdrop" @click.self="showCompositeModal = false">
      <section class="composite-modal" role="dialog" aria-modal="true" aria-labelledby="composite-modal-title">
        <div class="modal-header">
          <div>
            <p class="prototype-eyebrow">自选组合测试</p>
            <h2 id="composite-modal-title">选择要测试的项目与参数</h2>
            <p>已勾选 <strong>{{ selectedKeys.length }}</strong> 个节点（{{ selectedProfilesLabel }}）。一键顺序执行多项测试，无需手动切换页面。</p>
          </div>
          <button type="button" class="close-button" aria-label="关闭" @click="showCompositeModal = false">×</button>
        </div>

        <div class="composite-body">
          <!-- 延迟探测 -->
          <div class="composite-group" :class="{ disabled: !compositeIncludeLatency }">
            <div class="composite-group-head">
              <label class="composite-check-label">
                <input v-model="compositeIncludeLatency" type="checkbox">
                <strong>延迟与稳定性探测 (HTTP Ping)</strong>
              </label>
              <span class="composite-tag">批量并发</span>
            </div>
            <div v-if="compositeIncludeLatency" class="composite-group-fields">
              <label class="composite-field">
                <span>测试次数</span>
                <UiSelect v-model="compositeLatencySampleCount" variant="compact" aria-label="延迟测试次数" :options="sampleCountSelectOptions" />
              </label>
              <label class="composite-field">
                <span>单项超时</span>
                <UiSelect v-model="compositeLatencyTimeout" variant="compact" aria-label="延迟单项超时" :options="timeoutSelectOptions" />
              </label>
            </div>
          </div>

          <!-- 下载测速 -->
          <div class="composite-group" :class="{ disabled: !compositeIncludeDownload }">
            <div class="composite-group-head">
              <label class="composite-check-label">
                <input v-model="compositeIncludeDownload" type="checkbox">
                <strong>下载速度测量 (Throughput)</strong>
              </label>
              <span class="composite-tag">单节点独占</span>
            </div>
            <div v-if="compositeIncludeDownload" class="composite-group-fields">
              <label class="composite-field">
                <span>测速次数</span>
                <UiSelect v-model="compositeDownloadRepeatCount" variant="compact" aria-label="下载测速次数" :options="repeatCountOptions" />
              </label>
              <label class="composite-field">
                <span>每节点最多下载</span>
                <UiSelect v-model="compositeDownloadMaxMiB" variant="compact" aria-label="每节点最多下载" :options="downloadMiBOptions" />
              </label>
              <label class="composite-field">
                <span>单次超时</span>
                <UiSelect v-model="compositeDownloadTimeout" variant="compact" aria-label="下载单次超时" :options="downloadTimeoutOptions" />
              </label>
            </div>
            <div v-if="compositeIncludeDownload" class="composite-tip">
              * 为保证测速准确性与独占带宽，对所选节点逐个排队测量。
            </div>
          </div>

          <!-- 公共服务检测 -->
          <div class="composite-group" :class="{ disabled: !compositeIncludeService }">
            <div class="composite-group-head">
              <label class="composite-check-label">
                <input v-model="compositeIncludeService" type="checkbox">
                <strong>公共服务即时连通性 (Public Services)</strong>
              </label>
              <span class="composite-tag">可用性判据</span>
            </div>
            <div v-if="compositeIncludeService" class="composite-group-fields">
              <label class="composite-field">
                <span>检测次数</span>
                <UiSelect v-model="compositeServiceRepeatCount" variant="compact" aria-label="服务检测次数" :options="repeatCountOptions" />
              </label>
              <label class="composite-field">
                <span>单项超时</span>
                <UiSelect v-model="compositeServiceTimeout" variant="compact" aria-label="服务单项超时" :options="serviceTimeoutOptions" />
              </label>
            </div>
            <ServiceCatalogPicker v-if="compositeIncludeService" v-model="compositeSelectedServices" :catalog="publicServiceCatalog" :show-targets="false" />
          </div>
        </div>

        <div class="modal-footer">
          <div class="modal-footer-meta">
            <span>预计耗时：{{ estimatedDurationText }}</span>
            <span v-if="selectedKeys.length === 0" class="text-warning">（请先选择至少一个节点）</span>
          </div>
          <div class="modal-footer-buttons">
            <button type="button" class="prototype-button" @click="showCompositeModal = false">取消</button>
            <button type="button" class="prototype-button primary" :disabled="!canStartComposite" @click="startCompositeTest">
              开始执行组合测试
            </button>
          </div>
        </div>
      </section>
    </div>

    <!-- 移至公告/撤销提示 Toast -->
    <transition name="toast-slide">
      <div v-if="undoNoticeToast" class="workbench-toast" role="status">
        <span class="toast-text">
          已将「<strong>{{ undoNoticeToast.displayName }}</strong>」{{ undoNoticeToast.becameNotice ? '移至“公告与信息”' : '移回代理列表' }}
        </span>
        <button type="button" class="toast-undo-btn" @click="undoNoticeToggle">撤销</button>
        <button type="button" class="toast-close-btn" aria-label="关闭提示" @click="undoNoticeToast = null">×</button>
      </div>
    </transition>
  </main>
</template>

<style scoped>
.prototype-page { max-width: 1600px; margin: 0 auto; padding: 14px 34px 52px; color: var(--text-main); }
.prototype-page-heading { display: flex; align-items: center; justify-content: space-between; gap: 18px; min-height: 24px; margin-bottom: 12px; padding-bottom: 10px; border-bottom: 1px solid var(--border); }
.prototype-eyebrow { margin: 0; color: var(--primary); font-size: 12px; font-weight: 750; letter-spacing: .08em; }
.prototype-demo-note { max-width: 310px; padding: 5px 9px; border-left: 2px solid var(--warning); background: var(--warning-bg); color: var(--warning); font-size: 11px; line-height: 1.35; border-radius: 0 4px 4px 0; }
.service-result-controls { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.service-result-controls label { display: inline-flex; align-items: center; flex-wrap: wrap; gap: 7px; color: var(--text-secondary); font-size: 12px; }
.alternate-config-toggle { padding: 6px 9px; border: 1px dashed var(--border); border-radius: 7px; background: var(--card-bg); color: var(--primary); font-size: 11px; white-space: nowrap; }
.alternate-config-toggle[aria-pressed="true"] { border-style: solid; background: var(--primary-subtle); }
/* Unified Workbench Control Shell */
.workbench-sticky-wrapper {
  position: relative;
  transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}

.workbench-sticky-wrapper.is-sticky {
  position: sticky;
  top: 0;
  z-index: 45;
  background: var(--canvas);
  margin-left: -34px;
  margin-right: -34px;
  padding: 8px 34px 6px;
}

.workbench-control-shell {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  margin-bottom: 14px;
  overflow: visible;
  transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}

.workbench-control-shell.is-scrolled {
  box-shadow: 0 12px 30px -4px rgba(0, 0, 0, 0.12), 0 3px 8px -1px rgba(0, 0, 0, 0.06);
  background: color-mix(in srgb, var(--card-bg) 96%, transparent);
  backdrop-filter: blur(14px);
  margin-bottom: 4px;
}

/* Row 1: Scope Row */
.control-row.scope-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  justify-content: space-between;
  gap: 10px 14px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--border);
  transition: padding 0.15s ease;
}

.scope-controls-group {
  display: flex;
  flex-wrap: wrap;
  overflow: visible;
  align-items: center;
  gap: 10px 14px;
  min-width: 0;
}

.workbench-control-shell.is-scrolled .control-row.scope-row {
  padding: 7px 16px;
}

.scope-title {
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.scope-control {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--text-secondary);
  font-size: 12px;
  white-space: nowrap;
}

.scope-control select {
  min-width: 130px;
  padding: 4px 19px 4px 2px;
  border: 0;
  border-bottom: 1px solid var(--border-subtle);
  border-radius: 0;
  background: transparent;
  color: var(--text-main);
}

.profile-scope-picker { position: relative; }
.profile-scope-summary {
  list-style: none;
  cursor: pointer;
  padding: 5px 9px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card-subtle);
  color: var(--primary);
  font-size: 12px;
  font-weight: 650;
}
.profile-scope-summary::-webkit-details-marker { display: none; }
.profile-scope-picker[open] .profile-scope-summary { border-color: var(--primary); background: var(--primary-subtle); }
.profile-scope-popover {
  position: absolute;
  z-index: 80;
  top: calc(100% + 8px);
  left: 0;
  width: min(520px, calc(100vw - 48px));
  padding: 14px;
  border: 1px solid color-mix(in srgb, var(--primary) 35%, var(--border));
  border-radius: 10px;
  background: var(--card-bg);
  box-shadow: 0 18px 45px rgba(15, 43, 57, .16);
}
.profile-scope-head, .airport-scope-toggle, .profile-scope-option, .merge-duplicates-option { display: flex; align-items: center; }
.profile-scope-head { justify-content: space-between; gap: 12px; padding-bottom: 11px; border-bottom: 1px solid var(--border); }
.profile-scope-head strong, .merge-duplicates-option strong { display: block; font-size: 12px; }
.profile-scope-head small, .merge-duplicates-option small { display: block; margin-top: 2px; color: var(--text-muted); font-size: 10px; line-height: 1.45; }
.profile-scope-actions { display: flex; gap: 6px; }
.profile-scope-actions button, .airport-scope-toggle { border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); color: var(--text-secondary); font-size: 11px; }
.profile-scope-actions button { padding: 5px 8px; }
.profile-scope-groups { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; max-height: 310px; padding: 11px 0; overflow: auto; }
.profile-scope-group { min-width: 0; border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
.airport-scope-toggle { justify-content: space-between; width: 100%; padding: 7px 9px; border: 0; border-bottom: 1px solid var(--border); border-radius: 0; background: var(--card-subtle); font-weight: 650; text-align: left; }
.airport-scope-toggle small, .profile-scope-option small { color: var(--text-muted); font-size: 10px; font-weight: 400; }
.profile-scope-option { gap: 7px; padding: 7px 9px; color: var(--text-main); font-size: 11px; }
.profile-scope-option + .profile-scope-option { border-top: 1px solid color-mix(in srgb, var(--border) 65%, transparent); }
.profile-scope-option span { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.profile-scope-option em { display: block; margin-top: 2px; color: var(--text-muted); font-size: 9px; font-style: normal; font-weight: 400; }
.profile-scope-option input, .merge-duplicates-option input { accent-color: var(--primary); }
.merge-duplicates-option { align-items: flex-start; gap: 8px; padding: 10px; border-radius: 8px; background: var(--primary-subtle); color: var(--text-main); }

.scope-search-control { min-width: 170px; }
.scope-search {
  min-width: 170px;
  padding: 5px 8px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card-subtle);
  color: var(--text-main);
  font-size: 12px;
  transition: border-color .15s ease;
}
.scope-search:focus { outline: none; border-color: var(--border-focus); }
.scope-search::placeholder { color: var(--text-muted); }

.scope-separator {
  width: 1px;
  height: 18px;
  background: var(--border);
  opacity: 0.7;
}

.live-line {
  color: var(--text-muted);
  font-size: 11px;
  white-space: nowrap;
}

.scope-actions-end {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  white-space: nowrap;
}

.scope-refresh-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 32px;
  padding: 5px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card-subtle);
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.scope-refresh-btn:hover:not(:disabled) {
  border-color: var(--primary);
  color: var(--primary);
  background: var(--card-hover);
}
.scope-refresh-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.follow-mode-toggle {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-height: 32px;
  padding: 5px 11px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card-subtle);
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.follow-mode-toggle:hover {
  border-color: var(--primary);
  color: var(--primary);
  background: var(--card-hover);
}
.follow-mode-toggle.active {
  border-color: var(--primary);
  background: var(--primary-subtle);
  color: var(--primary);
  font-weight: 700;
  box-shadow: 0 1px 3px rgba(36, 107, 134, 0.12);
}
.follow-mode-toggle .pin-icon {
  font-size: 12px;
  line-height: 1;
  transform: rotate(-15deg);
  display: inline-block;
}

.scroll-top-button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 32px;
  padding: 5px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card-subtle);
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.scroll-top-button:hover {
  border-color: var(--border-focus);
  color: var(--text-main);
  background: var(--card-hover);
}

/* Row 2: Project Row */
.control-row.project-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 16px;
  transition: padding 0.15s ease;
}

.workbench-control-shell.is-scrolled .control-row.project-row {
  padding: 7px 16px;
}

.project-row-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  align-items: center;
  justify-content: space-between;
  gap: 10px 16px;
}

.project-row-left {
  display: flex;
  flex-wrap: nowrap;
  overflow-x: auto;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.project-bar-heading {
  display: flex;
  align-items: baseline;
  gap: 8px;
  white-space: nowrap;
}

.project-bar-note {
  color: var(--text-muted);
  font-size: 11px;
}

.workbench-control-shell.is-scrolled .project-bar-note,
.workbench-control-shell.is-scrolled .live-line {
  visibility: hidden;
}

.project-tabs {
  display: flex;
  flex-wrap: nowrap;
  gap: 5px;
  min-width: 0;
}

.project-tab {
  min-height: 30px;
  padding: 5px 9px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 700;
  transition: all 0.15s ease;
}
.project-tab:hover {
  border-color: var(--border-subtle);
  color: var(--text-main);
}
.project-tab.active {
  border-color: var(--primary);
  background: var(--primary-subtle);
  color: var(--primary);
}
.project-tab span {
  display: block;
  margin-top: 1px;
  color: var(--warning);
  font-size: 9px;
  font-weight: 600;
}

.project-bar-actions {
  display: flex;
  flex-wrap: nowrap;
  overflow-x: auto;
  min-width: 0;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
  margin-left: 0;
}

/* Keep individual controls intact; the scope group can wrap on narrow windows. */
.scope-controls-group > *,
.scope-actions-end > *,
.project-row-left > *,
.project-bar-actions > * {
  flex-shrink: 0;
  white-space: nowrap;
}
.scope-controls-group,
.project-row-left,
.project-bar-actions {
  padding-block: 3px;
  scrollbar-width: thin;
}
.workbench-control-shell .scope-control { width: auto; }
.workbench-control-shell .scope-search-control { width: auto; }
.workbench-control-shell .scope-search { width: 170px; }

.selection-summary.highlight {
  display: inline-flex;
  align-items: center;
  padding: 4px 9px;
  border-radius: 6px;
  background: var(--primary-subtle);
  color: var(--primary);
  font-size: 11px;
  font-weight: 750;
  white-space: nowrap;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.timeout-control {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--text-secondary);
  font-size: 11px;
  white-space: nowrap;
}

.batch-test-status {
  padding-top: 6px;
  border-top: 1px solid var(--border);
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 1.4;
}
.batch-test-status.running { color: var(--primary); }
.batch-test-status.complete { color: var(--success); }
.batch-test-status.blocked { color: var(--warning); }

.workbench-control-shell.is-scrolled .batch-test-status:not(.running):not(.complete):not(.blocked) {
  display: none;
}

.prototype-button { display: inline-flex; align-items: center; justify-content: center; min-height: 34px; padding: 6px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-subtle); color: var(--text-main); font-size: 12px; font-weight: 700; transition: all .15s ease; }.prototype-button:hover:not(:disabled) { border-color: var(--border-focus); color: var(--primary); background: var(--card-hover); }.prototype-button.primary { border-color: var(--primary); background: var(--primary); color: white; }.prototype-button.primary:hover:not(:disabled) { background: var(--primary-hover); color: white; }.prototype-button:disabled { cursor: not-allowed; opacity: .52; }
.prototype-panel { border: 1px solid var(--border); border-radius: 12px; background: var(--card-bg); box-shadow: 0 14px 35px rgba(0, 0, 0, .08); }.prototype-panel-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; padding: 19px 20px 14px; border-bottom: 1px solid var(--border); }.prototype-panel-header h2 { margin: 0 0 5px; font-size: 17px; letter-spacing: -.02em; }.prototype-panel-header p { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }.text-action { padding: 2px 0; border: 0; background: transparent; color: var(--primary); font-size: 12px; font-weight: 750; }.text-action:disabled { cursor: not-allowed; color: var(--text-muted); }
.comparison-head { display: grid; grid-template-columns: 34px minmax(210px, .75fr) minmax(190px, .7fr) minmax(540px, 2.4fr); gap: 12px; align-items: end; padding: 12px 20px 9px; border-bottom: 1px solid var(--border); color: var(--text-muted); font-size: 11px; font-weight: 700; }.history-axis { display: flex; justify-content: space-between; margin-top: 8px; padding: 0 16px 0 52px; color: var(--text-muted); font-size: 10px; font-weight: 500; }.history-legend { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 13px; padding: 8px 20px; border-bottom: 1px solid var(--border); color: var(--text-secondary); font-size: 10px; }.legend-item { display: inline-flex; align-items: center; gap: 4px; }.legend-mark { width: 8px; height: 8px; border-radius: 50%; background: var(--success); }.legend-mark.fail { border-radius: 0; background: var(--danger); transform: rotate(45deg); }.legend-mark.timeout { border: 2px solid var(--danger); background: var(--card-bg); border-radius: 2px; }.legend-mark.missing { width: 13px; height: 3px; border-radius: 0; background: var(--text-muted); }
.node-list { list-style: none; margin: 0; padding: 0; }.node-row { position: relative; display: grid; grid-template-columns: 34px minmax(210px, .75fr) minmax(190px, .7fr) minmax(540px, 2.4fr); gap: 12px; align-items: center; min-height: 132px; padding: 12px 20px; border-bottom: 1px solid var(--border); background: var(--card-bg); transition: background-color .15s ease; }.node-row:last-child { border-bottom: 0; }.node-row:hover { background: var(--card-hover); }.node-row[aria-selected="true"] { background: var(--primary-subtle); }.node-row[aria-selected="true"]::before { content: ""; position: absolute; left: 0; top: 9px; bottom: 9px; width: 3px; border-radius: 0 3px 3px 0; background: var(--primary); }.select-cell { display: grid; place-items: center; min-height: 32px; cursor: pointer; }.select-cell input { width: 16px; height: 16px; margin: 0; }.node-evidence { min-width: 0; padding: 0; border: 0; background: transparent; color: inherit; text-align: left; cursor: pointer; }.node-evidence:hover .node-name strong { color: var(--primary); }.node-name { min-width: 0; }.node-name strong { display: block; overflow: hidden; color: var(--text-main); font-size: 14px; line-height: 1.35; text-overflow: ellipsis; white-space: nowrap; }.node-meta { display: flex; flex-wrap: wrap; gap: 5px 9px; margin-top: 5px; color: var(--text-secondary); font-size: 12px; }.node-meta span:first-child { color: var(--primary); }.badge-monitoring { display: inline-flex; align-items: center; gap: 3px; padding: 1px 6px; border-radius: 4px; background: rgba(16, 185, 129, 0.12); color: var(--success); font-size: 10px; font-weight: 600; }.node-row-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-top: 6px; }.row-menu-dropdown-wrapper { position: relative; display: inline-flex; }.row-action-btn { display: inline-flex; align-items: center; gap: 3px; padding: 2px 8px; border: 1px solid var(--border); border-radius: 4px; background: var(--card-subtle); color: var(--text-secondary); font-size: 11px; font-weight: 600; cursor: pointer; transition: all .15s ease; }.row-action-btn:hover:not(:disabled) { border-color: var(--primary); color: var(--primary); background: var(--card-hover); }.row-action-btn.icon-only { padding: 2px 6px; font-size: 13px; line-height: 1; }.row-menu-popover { position: absolute; top: calc(100% + 4px); left: 0; z-index: 30; min-width: 130px; padding: 4px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18); animation: fadeIn 0.12s ease; }.row-menu-item { display: flex; align-items: center; gap: 6px; width: 100%; padding: 6px 10px; border: 0; border-radius: 4px; background: transparent; color: var(--text-main); font-size: 12px; text-align: left; cursor: pointer; transition: background-color 0.15s ease; }.row-menu-item:hover { background: var(--card-hover); color: var(--primary); }.row-action-btn.primary { border-color: var(--primary); background: var(--primary-subtle); color: var(--primary); }.row-action-btn.primary:hover:not(:disabled) { background: var(--primary); color: white; }.row-action-btn:disabled { opacity: 0.5; cursor: not-allowed; }.text-action-subtle { padding: 2px 7px; border: 1px dashed var(--border); border-radius: 4px; background: transparent; color: var(--text-muted); font-size: 11px; cursor: pointer; transition: all .15s ease; }.text-action-subtle:hover { border-color: var(--warning); color: var(--warning); }
.row-readout { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }.readout-observation { display: flex; flex-direction: column; gap: 2px; width: 100%; }.readout-header { display: flex; align-items: center; justify-content: space-between; gap: 6px; width: 100%; }.readout-main-val { display: flex; align-items: baseline; gap: 6px; }.readout-health-summary { display: flex; flex-direction: column; gap: 3px; width: 100%; padding-top: 5px; border-top: 1px dashed var(--border); font-size: 11px; }.health-summary-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }.health-badge { display: inline-block; padding: 1px 5px; border-radius: 3px; font-size: 10px; font-weight: 700; }.health-badge.success { background: rgba(16, 185, 129, 0.12); color: var(--success); }.health-badge.info { background: rgba(59, 130, 246, 0.12); color: var(--primary); }.health-badge.warning { background: rgba(245, 158, 11, 0.15); color: var(--warning, #f59e0b); }.health-badge.danger { background: rgba(239, 68, 68, 0.15); color: var(--danger); }.health-normal-val { color: var(--text-main); font-size: 11px; font-weight: 600; }.health-summary-metrics { color: var(--text-secondary); font-size: 10px; }.health-summary-metrics b { font-weight: 700; }.health-summary-metrics b.success { color: var(--success); }.health-summary-metrics b.warning { color: var(--warning, #f59e0b); }.health-summary-metrics b.danger { color: var(--danger); }.row-readout-state { color: var(--text-secondary); font-size: 12px; line-height: 1.2; }.row-readout-state.running { color: var(--primary); font-weight: 750; }.row-readout-value { display: flex; align-items: baseline; gap: 4px; max-width: 100%; overflow: hidden; color: var(--text-main); font-size: 20px; font-weight: 780; line-height: 1.08; letter-spacing: -.02em; text-overflow: ellipsis; white-space: nowrap; }.row-readout-value small { color: var(--text-secondary); font-size: 13px; font-weight: 650; letter-spacing: 0; }.row-readout-value.fail, .row-readout-value.timeout { color: var(--danger); }.row-readout-value.success { color: var(--success); }.row-readout-value.nodata { color: var(--text-muted); }.row-readout-time, .row-readout-status { max-width: 100%; overflow: hidden; color: var(--text-secondary); font-size: 12px; line-height: 1.35; text-overflow: ellipsis; white-space: nowrap; }.row-readout-status { margin-top: 3px; color: var(--text-main); font-weight: 700; }.pin-control button { padding: 0; border: 0; background: transparent; color: var(--primary); font-size: 11px; }
.workbench-toast { position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%); z-index: 60; display: flex; align-items: center; gap: 12px; padding: 10px 18px; border: 1px solid var(--border-focus, var(--primary)); border-radius: 8px; background: var(--card-bg); color: var(--text-main); box-shadow: 0 12px 36px rgba(0, 0, 0, 0.25); font-size: 12px; }.toast-text strong { color: var(--primary); }.toast-undo-btn { padding: 3px 10px; border: 1px solid var(--primary); border-radius: 4px; background: var(--primary-subtle); color: var(--primary); font-size: 12px; font-weight: 700; cursor: pointer; transition: all 0.15s ease; }.toast-undo-btn:hover { background: var(--primary); color: white; }.toast-close-btn { padding: 0 4px; border: 0; background: transparent; color: var(--text-muted); font-size: 16px; cursor: pointer; }.toast-close-btn:hover { color: var(--text-main); }.toast-slide-enter-active, .toast-slide-leave-active { transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1); }.toast-slide-enter-from, .toast-slide-leave-to { opacity: 0; transform: translate(-50%, 16px); }
.modal-table-wrap tr.cursor-pointer { cursor: pointer; }.modal-table-wrap tr.cursor-pointer:hover:not(.selected) { background: var(--card-hover); }
.history-cell { min-width: 0; }.history-cell :deep(.relative) { min-height: 60px; }.history-cell :deep(svg) { width: 100%; }.history-layer-label { font-size: 10px; color: var(--text-muted); font-weight: 600; margin-bottom: 2px; }.node-intra-test-wrap { width: 100%; margin-top: 6px; }.batch-samples-visual { width: 100%; margin-top: 6px; }.modal-intra-chart { width: 100%; margin-top: 10px; }.history-summary { min-width: 0; color: var(--text-secondary); font-size: 11px; line-height: 1.5; }.history-summary strong { display: block; color: var(--text-main); font-size: 12px; }.history-summary span { display: block; margin-top: 3px; }.history-open { display: inline-flex; align-items: center; gap: 5px; margin-top: 5px; padding: 0; border: 0; background: transparent; font-size: 12px; font-weight: 650; cursor: pointer; }.project-unavailable-cell { display: flex; flex-direction: column; justify-content: center; min-height: 84px; padding: 12px; border-left: 3px solid var(--border-subtle); background: var(--card-subtle); }.project-unavailable-cell strong { color: var(--text-secondary); font-size: 13px; }.project-unavailable-cell span { margin-top: 6px; color: var(--text-muted); font-size: 11px; line-height: 1.45; }.compare-footer { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; padding: 10px 20px; border-top: 1px solid var(--border); color: var(--text-secondary); font-size: 11px; }
.prototype-state { display: flex; flex-direction: column; align-items: center; gap: 9px; padding: 52px 24px; color: var(--text-secondary); text-align: center; font-size: 12px; }.prototype-state strong { color: var(--text-main); font-size: 17px; }.prototype-state.error-state strong { color: var(--danger); }.modal-history-list { margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border); }.modal-history-pills { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 6px; }.modal-history-pill { display: flex; align-items: center; gap: 8px; padding: 5px 10px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-subtle); color: var(--text-secondary); font-size: 11px; cursor: pointer; transition: all .15s ease; }.modal-history-pill:hover { border-color: var(--primary); color: var(--text-main); }.modal-history-pill.active { border-color: var(--primary); background: var(--primary-subtle); color: var(--primary); font-weight: 700; }
.modal-empty-state { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 48px 24px; text-align: center; background: var(--card-subtle); border: 1px dashed var(--border); border-radius: 12px; margin-top: 14px; }.modal-empty-icon { font-size: 38px; margin-bottom: 12px; }.modal-empty-state h3 { margin: 0 0 6px; font-size: 16px; color: var(--text-main); }.modal-empty-state p { margin: 0 0 18px; max-width: 480px; font-size: 12px; color: var(--text-secondary); line-height: 1.5; }.modal-empty-actions { display: flex; gap: 10px; }
.prototype-modal-backdrop { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 14px; background: rgba(0, 0, 0, .55); backdrop-filter: blur(4px); }.prototype-history-modal { width: min(1240px, calc(100vw - 28px)); max-height: calc(100vh - 28px); overflow: auto; padding: 24px; border: 1px solid var(--border); border-radius: 13px; background: var(--card-bg); color: var(--text-main); box-shadow: 0 24px 70px rgba(0, 0, 0, .35); }.modal-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 12px; }.modal-header h2 { max-width: 900px; margin: 6px 0; font-size: 22px; line-height: 1.25; }.modal-header p:not(.prototype-eyebrow) { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }.modal-actions { display: flex; align-items: flex-start; gap: 8px; }.close-button { width: 30px; height: 30px; border: 1px solid var(--border); border-radius: 50%; background: var(--card-subtle); color: var(--text-secondary); font-size: 18px; line-height: 1; transition: all .15s ease; }.close-button:hover { background: var(--card-hover); color: var(--text-main); }
.modal-tabs { display: flex; gap: 8px; margin-bottom: 12px; padding-bottom: 10px; border-bottom: 1px solid var(--border); }.modal-tab-btn { display: inline-flex; align-items: center; gap: 6px; padding: 6px 14px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-subtle); color: var(--text-secondary); font-size: 12px; font-weight: 600; cursor: pointer; transition: all 0.15s ease; }.modal-tab-btn:hover { background: var(--card-hover); color: var(--text-main); }.modal-tab-btn.active { border-color: var(--primary); background: var(--primary); color: white; }
.history-controls { display: flex; flex-wrap: wrap; align-items: end; gap: 10px 14px; padding: 12px 13px; border: 1px solid var(--border); border-radius: 8px; background: var(--card-subtle); color: var(--text-secondary); font-size: 11px; }.history-controls label { display: grid; gap: 5px; font-size: 10px; font-weight: 700; }.history-controls select { min-width: 170px; padding: 6px 8px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); color: var(--text-main); font-size: 11px; }.modal-chart { margin-top: 14px; padding: 16px 10px 10px; border: 1px solid var(--border); border-radius: 8px; background: var(--card-bg); }.modal-current { display: flex; flex-wrap: wrap; gap: 10px 18px; margin-top: 12px; padding: 10px 12px; border-left: 3px solid var(--primary); border-radius: 0 6px 6px 0; background: var(--primary-subtle); color: var(--text-secondary); font-size: 12px; }.modal-current strong { color: var(--text-main); }.modal-table-wrap { margin-top: 18px; overflow: auto; border-top: 1px solid var(--border); }.modal-table-wrap table { width: 100%; min-width: 680px; border-collapse: collapse; font-size: 12px; }.modal-table-wrap th, .modal-table-wrap td { padding: 9px 8px; border-bottom: 1px solid var(--border); text-align: left; }.modal-table-wrap th { color: var(--text-secondary); font-weight: 700; }.modal-table-wrap tr.selected { background: var(--primary-subtle); }.modal-table-wrap td.success { color: var(--success); font-weight: 700; }.modal-table-wrap td.fail { color: var(--danger); font-weight: 700; }
.modal-health-tab-content { margin-top: 14px; }.health-view-body { display: flex; flex-direction: column; gap: 16px; }.health-diagnosis-card { display: flex; align-items: flex-start; gap: 14px; padding: 14px 18px; border-radius: 10px; border: 1px solid var(--border); background: var(--card-subtle); }.health-diagnosis-card.success { border-color: rgba(16, 185, 129, 0.4); background: rgba(16, 185, 129, 0.08); }.health-diagnosis-card.warning { border-color: rgba(245, 158, 11, 0.4); background: rgba(245, 158, 11, 0.08); }.health-diagnosis-card.danger { border-color: rgba(239, 68, 68, 0.4); background: rgba(239, 68, 68, 0.08); }.diagnosis-badge { padding: 4px 10px; border-radius: 6px; font-size: 12px; font-weight: 750; white-space: nowrap; }.health-diagnosis-card.success .diagnosis-badge { background: var(--success); color: white; }.health-diagnosis-card.warning .diagnosis-badge { background: var(--warning, #f59e0b); color: white; }.health-diagnosis-card.danger .diagnosis-badge { background: var(--danger); color: white; }.diagnosis-text h4 { margin: 0 0 4px; font-size: 14px; font-weight: 700; color: var(--text-main); }.diagnosis-sub { margin: 0; font-size: 12px; color: var(--text-secondary); line-height: 1.5; }.health-grid-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }@media (max-width: 768px) { .health-grid-cards { grid-template-columns: repeat(2, 1fr); } }@media (max-width: 480px) { .health-grid-cards { grid-template-columns: 1fr; } }.health-card { display: flex; flex-direction: column; gap: 4px; padding: 12px 14px; border: 1px solid var(--border); border-radius: 8px; background: var(--card-bg); }.health-card.highlight { border-color: var(--primary); background: var(--primary-subtle); }.card-label { font-size: 11px; color: var(--text-secondary); font-weight: 600; }.card-val { font-size: 18px; font-weight: 750; color: var(--text-main); line-height: 1.2; }.card-val.success { color: var(--success); }.card-val.warning { color: var(--warning, #f59e0b); }.card-val.danger { color: var(--danger); }.card-desc { font-size: 10px; color: var(--text-muted); line-height: 1.4; margin-top: 2px; }.health-histogram-section { padding: 16px; border: 1px solid var(--border); border-radius: 8px; background: var(--card-bg); }.histogram-head h3 { margin: 0 0 4px; font-size: 14px; font-weight: 700; color: var(--text-main); }.histogram-head span { font-size: 11px; color: var(--text-secondary); }.histogram-bars { display: flex; flex-direction: column; gap: 8px; margin-top: 14px; }.histogram-bar-row { display: grid; grid-template-columns: 100px 1fr 45px 55px 90px; align-items: center; gap: 10px; font-size: 11px; }@media (max-width: 600px) { .histogram-bar-row { grid-template-columns: 80px 1fr 40px 45px; } }.bin-label { color: var(--text-main); font-weight: 600; text-align: right; white-space: nowrap; }.bin-track { height: 18px; border-radius: 4px; background: var(--card-subtle); overflow: hidden; }.bin-fill { height: 100%; border-radius: 4px; background: var(--border-focus, #94a3b8); transition: width 0.3s ease; }.bin-fill.fill-normal { background: var(--primary); }.bin-percent { color: var(--text-main); font-weight: 700; text-align: right; }.bin-count { color: var(--text-muted); text-align: right; }.bin-tag { display: inline-block; padding: 1px 6px; border-radius: 3px; background: var(--primary-subtle); color: var(--primary); font-size: 10px; font-weight: 700; white-space: nowrap; }.histogram-empty { padding: 24px; text-align: center; color: var(--text-muted); font-size: 12px; }
.batch-panel { margin-bottom: 14px; }.batch-history-list { display: flex; flex-direction: column; padding: 0 20px; }.batch-history-record { display: grid; grid-template-columns: minmax(180px, 1fr) 90px 120px; gap: 14px; width: 100%; padding: 9px 0; border: 0; border-bottom: 1px solid var(--border); background: transparent; color: var(--text-secondary); text-align: left; font-size: 12px; }.batch-history-record.selected { color: var(--primary); }.batch-history-record strong, .batch-history-record small { display: block; }.batch-history-record small { margin-top: 3px; color: var(--text-muted); }.batch-history-empty { padding: 10px 0 15px; color: var(--text-muted); font-size: 12px; }.batch-detail { padding: 14px 20px 18px; }.batch-detail-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 9px; }.batch-detail-heading strong, .batch-detail-heading span { display: block; }.batch-detail-heading strong { font-size: 13px; }.batch-detail-heading span { margin-top: 3px; color: var(--text-secondary); font-size: 11px; }.batch-item-row { display: grid; grid-template-columns: minmax(180px, .9fr) minmax(160px, .7fr) minmax(220px, 1.4fr); gap: 14px; align-items: start; padding: 11px 0; border-top: 1px solid var(--border); font-size: 11px; }.batch-item-identity, .batch-item-state, .batch-item-result { display: flex; flex-direction: column; gap: 4px; min-width: 0; }.batch-item-identity strong, .batch-item-state strong, .batch-item-result strong { color: var(--text-main); font-size: 12px; }.batch-item-identity span, .batch-item-state span, .batch-item-result span, .batch-no-result { color: var(--text-secondary); line-height: 1.4; overflow-wrap: anywhere; }.batch-item-identity code { color: var(--text-muted); font-size: 9px; overflow-wrap: anywhere; }.batch-error-detail { color: var(--danger) !important; }.batch-no-result { color: var(--text-muted); }
@media (max-width: 860px) { .batch-item-row { grid-template-columns: 1fr 1fr; }.batch-item-result { grid-column: 1 / -1; } }
@media (max-width: 560px) { .batch-history-record { grid-template-columns: 1fr 70px; }.batch-history-record > :last-child { grid-column: 1 / -1; }.batch-item-row { grid-template-columns: 1fr; }.batch-item-result { grid-column: auto; } }
@media (max-width: 1120px) { .comparison-head, .node-row { grid-template-columns: 30px minmax(190px, .75fr) minmax(180px, .7fr) minmax(420px, 2.1fr); gap: 8px; } }
@media (max-width: 860px) { .prototype-page { padding: 23px 17px 40px; }.prototype-page-heading { align-items: flex-start; }.project-bar-actions { justify-content: flex-start; }.comparison-head, .node-row { grid-template-columns: 30px minmax(170px, .8fr) minmax(170px, .75fr) minmax(340px, 1.6fr); } .workbench-sticky-wrapper.is-sticky { margin-left: -17px; margin-right: -17px; padding-left: 17px; padding-right: 17px; } }
@media (max-width: 560px) { .prototype-page { padding-left: 10px; padding-right: 10px; }.prototype-page-heading { align-items: center; }.prototype-demo-note { max-width: 210px; }.scope-separator { display: none; }.scope-control { width: 100%; justify-content: space-between; }.scope-control select { flex: 1; max-width: 220px; }.scope-search-control, .scope-search { width: 100%; }.profile-scope-popover { left: -70px; width: calc(100vw - 36px); }.profile-scope-groups { grid-template-columns: 1fr; }.comparison-head { display: none; }.node-row { grid-template-columns: 28px 1fr; gap: 8px; padding: 12px 16px; }.node-row > :nth-child(3), .node-row > :nth-child(4) { grid-column: 2; }.row-readout { padding: 8px 0 2px; }.history-cell { width: 100%; }.prototype-history-modal { padding: 17px; }.history-controls { align-items: flex-start; }.modal-header { flex-direction: column; }.modal-actions { align-self: stretch; justify-content: flex-end; } .workbench-sticky-wrapper.is-sticky { margin-left: -10px; margin-right: -10px; padding-left: 10px; padding-right: 10px; } }

.composite-trigger {
  border-color: var(--primary);
  background: var(--primary);
  color: white;
  white-space: nowrap;
}
.composite-progress-banner {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 14px;
  padding: 12px 18px;
  border: 1px solid var(--primary);
  border-radius: 10px;
  background: var(--primary-subtle);
  color: var(--text-main);
  animation: fadeIn 0.2s ease;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
.composite-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid var(--primary);
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  flex-shrink: 0;
}
.composite-banner-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}
.composite-banner-text strong {
  font-size: 13px;
  color: var(--primary);
}
.composite-banner-text span {
  font-size: 11px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.composite-modal {
  width: min(680px, calc(100vw - 32px));
  max-height: calc(100vh - 40px);
  overflow-y: auto;
  padding: 24px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--card-bg);
  color: var(--text-main);
  box-shadow: 0 24px 70px rgba(0, 0, 0, 0.4);
}
.composite-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin: 18px 0;
}
.composite-group {
  padding: 14px 16px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card-subtle);
  transition: opacity 0.15s ease;
}
.composite-group.disabled {
  opacity: 0.6;
}
.composite-group-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.composite-check-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-size: 13px;
}
.composite-check-label input {
  width: 16px;
  height: 16px;
}
.composite-tag {
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  background: var(--card-hover);
  color: var(--text-muted);
}
.composite-group-fields {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--border-subtle);
}
.composite-field {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
  color: var(--text-secondary);
}
.composite-tip {
  margin-top: 8px;
  font-size: 10px;
  color: var(--text-muted);
}
.composite-services-list {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
  font-size: 11px;
}
.composite-services-label {
  color: var(--text-secondary);
  font-size: 11px;
}
.service-checkbox {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
  padding: 3px 8px;
  border: 1px solid var(--border-subtle);
  border-radius: 5px;
  background: var(--card-bg);
}
.service-checkbox input {
  width: 14px;
  height: 14px;
}
.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--border);
}
.modal-footer-meta {
  font-size: 11px;
  color: var(--text-secondary);
}
.modal-footer-buttons {
  display: flex;
  align-items: center;
  gap: 10px;
}
.node-composite-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 5px;
}
.composite-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 2px 7px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
  line-height: 1.2;
}
.composite-badge.download {
  background: var(--primary-subtle);
  color: var(--primary);
  border: 1px solid var(--primary);
}
.composite-badge.service {
  background: rgba(16, 185, 129, 0.12);
  color: var(--success);
  border: 1px solid rgba(16, 185, 129, 0.35);
}
.project-result-cell {
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-height: 84px;
  padding: 12px;
  border-left: 3px solid var(--primary);
  background: var(--card-subtle);
}
.project-result-cell strong {
  color: var(--text-main);
  font-size: 13px;
}
.project-result-cell span {
  margin-top: 6px;
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 1.45;
}
.category-tabs-group {
  display: flex;
  align-items: center;
  gap: 12px;
}
.category-tabs {
  display: inline-flex;
  padding: 3px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card-subtle);
  gap: 4px;
}
.category-tab {
  padding: 5px 12px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
  transition: all .15s ease;
}
.category-tab:hover {
  color: var(--text-main);
  background: var(--card-hover);
}
.category-tab.active {
  background: var(--primary);
  color: white;
  box-shadow: 0 1px 3px rgba(0, 0, 0, .15);
}
.notice-page-wrap {
  display: flex;
  flex-direction: column;
}
.notice-page-banner {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin: 14px 20px;
  padding: 12px 16px;
  border: 1px solid var(--border);
  border-left: 3px solid var(--primary);
  border-radius: 8px;
  background: var(--primary-subtle);
}
.notice-page-banner-icon {
  font-size: 20px;
  line-height: 1;
}
.notice-page-banner-content strong {
  display: block;
  font-size: 13px;
  color: var(--text-main);
  margin-bottom: 4px;
}
.notice-page-banner-content p {
  margin: 0;
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.5;
}
.notice-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.notice-item-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 20px;
  border-bottom: 1px solid var(--border);
  transition: background-color .15s ease;
}
.notice-item-card:hover {
  background: var(--card-hover);
}
.notice-item-main {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
}
.notice-item-icon {
  font-size: 18px;
  padding-top: 2px;
}
.notice-item-info {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}
.notice-item-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-main);
  word-break: break-all;
}
.notice-item-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
}
.notice-profile-badge, .notice-type-badge, .notice-country-badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 4px;
  border: 1px solid var(--border);
  background: var(--card-subtle);
  color: var(--text-secondary);
  font-size: 10px;
}
.notice-links-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}
.notice-link-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 5px;
  border: 1px solid var(--primary);
  background: var(--primary-subtle);
  color: var(--primary);
  font-size: 11px;
  font-weight: 600;
  text-decoration: none;
  transition: all .15s ease;
}
.notice-link-btn:hover {
  background: var(--primary);
  color: white;
}
.notice-tag {
  display: inline-block;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--warning-bg, rgba(234, 179, 8, 0.15));
  color: var(--warning, #eab308);
  font-size: 10px;
  font-weight: 700;
}
.node-row-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 4px;
}
.text-action-subtle {
  border: 0;
  background: transparent;
  color: var(--text-muted);
  font-size: 11px;
  cursor: pointer;
  padding: 0 4px;
  transition: color .15s ease;
}
.text-action-subtle:hover {
  color: var(--primary);
  text-decoration: underline;
}
.custom-sample-input {
  width: 44px;
  height: 24px;
  margin-left: 2px;
  padding: 0 4px;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--card-bg, #fff);
  color: var(--text-main);
  font-size: 11px;
  text-align: center;
}
.modal-config-tab-content {
  margin-top: 14px;
}
.modal-config-card {
  padding: 18px 20px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card-subtle);
}
.node-config-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 14px 24px;
}
.config-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.config-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-muted);
}
.config-val {
  font-size: 13px;
  color: var(--text-main);
  word-break: break-all;
}

/* Keep primary actions and progress readable at both narrow and wide sizes. */
.selection-actions { flex-wrap: wrap; gap: 8px; }
.selection-button { min-height: 34px; padding: 6px 12px; border: 1px solid var(--border); border-radius: 7px; background: var(--card-bg); color: var(--primary); font-size: 13px; font-weight: 600; white-space: nowrap; }
.selection-button:hover:not(:disabled), .selection-button[aria-pressed="true"] { background: var(--primary-subtle); border-color: var(--primary); }
.selection-button.secondary { color: var(--text-secondary); }
.selection-button:disabled { opacity: .45; cursor: default; }
.selection-count { color: var(--text-secondary); font-size: 12px; white-space: nowrap; max-width: 240px; overflow: hidden; text-overflow: ellipsis; }
.node-row.is-focused { box-shadow: inset 3px 0 var(--primary); }
.batch-progress { display: block; width: 100%; height: 9px; accent-color: var(--primary); margin: 12px 0; }
.batch-list-toolbar, .batch-filters { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.batch-list-toolbar { justify-content: space-between; margin: 14px 0 8px; }
.batch-filters button { padding: 7px 12px; border: 1px solid var(--border); border-radius: 7px; background: var(--card-bg); color: var(--text-secondary); font-size: 13px; }
.batch-filters button[aria-pressed="true"] { background: var(--primary-subtle); color: var(--primary); border-color: var(--primary); }
.batch-scroll { max-height: 340px; overflow: auto; scrollbar-gutter: stable; overscroll-behavior: contain; }
.batch-item-row { font-size: 13px; grid-template-columns: minmax(150px, 1fr) minmax(140px, .8fr) minmax(180px, 1fr); padding: 14px 8px; }
.batch-item-identity strong, .batch-item-state strong, .batch-item-result strong { font-size: 14px; }
.batch-detail-heading strong { font-size: 16px; }
.batch-detail-heading span { font-size: 13px; margin-top: 7px; }
.batch-technical, .batch-identifier { color: var(--text-muted); font-size: 12px; }
.batch-technical summary, .batch-identifier summary, .advanced-details summary { cursor: pointer; }
.batch-technical code, .batch-technical span, .batch-identifier code { display: block; overflow-wrap: anywhere; margin-top: 6px; }
.batch-empty { padding: 20px; color: var(--text-secondary); }
.history-open { padding: 7px 12px; border: 1px solid var(--border); border-radius: 7px; background: var(--card-bg); font-size: 13px; }
.prototype-history-modal .modal-tab-btn { padding: 10px 18px; font-size: 14px; }
.prototype-history-modal .modal-tabs { gap: 8px; flex-wrap: wrap; }
.health-grid-cards { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.health-card { padding: 18px; gap: 10px; }
.card-label { font-size: 13px; }
.card-val { font-size: 24px; line-height: 1.35; overflow-wrap: anywhere; }
.card-desc { font-size: 12px; line-height: 1.6; }
.health-diagnosis-card { flex-direction: column; gap: 8px; padding: 20px; }
.diagnosis-badge { font-size: 16px; padding: 0; }
.diagnosis-sub { font-size: 13px; }
.advanced-details { margin-top: 16px; border-top: 1px solid var(--border); padding-top: 14px; font-size: 13px; color: var(--text-secondary); }
.advanced-details summary { font-weight: 600; }
.advanced-details p { line-height: 1.7; margin: 12px 0; }
.health-more { margin: 0; padding: 12px 0; }
.health-section-title { margin: 3px 0 0; color: var(--text-main); font-size: 15px; font-weight: 700; }
.health-more .health-grid-cards { margin-top: 12px; }
.health-more .card-val { font-size: 18px; }
.histogram-empty { padding: 14px 0 0; text-align: left; }
.histogram-head span, .histogram-bar-row { font-size: 12px; }
.history-loading-cell { display: flex; align-items: center; justify-content: center; min-height: 84px; color: var(--text-secondary); font-size: 12px; }
.history-loading-cell.error { color: var(--danger); }
.row-test-issue { margin: 8px 0 0 52px; padding: 9px 12px; border-left: 3px solid var(--danger); border-radius: 4px; background: rgba(239, 68, 68, .07); color: var(--danger); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
.modal-test-issue { margin: 12px 0; }
@media (max-width: 860px) { .row-test-issue { margin-left: 0; } }
.metric-row-footer { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 4px 14px; margin: 1px 0 0 58px; color: var(--text-secondary); font-size: 11px; line-height: 1.45; }
.metric-row-footer span:first-child { color: var(--text-main); }
.row-health-overview {
  margin-top: 5px;
  padding: 11px 14px;
  border: 1px solid var(--border);
  border-left: 3px solid var(--primary);
  border-radius: 8px;
  background: var(--card-subtle);
}
.row-health-overview.success { border-left-color: var(--success); }
.row-health-overview.warning { border-left-color: var(--warning); background: rgba(245, 158, 11, .07); }
.row-health-overview.danger { border-left-color: var(--danger); background: rgba(239, 68, 68, .07); }
.row-health-heading, .row-health-metrics { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 13px; }
.row-health-heading strong { color: var(--text-main); font-size: 14px; line-height: 1.35; }
.row-health-metrics { margin-top: 7px; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
.row-health-metrics span { white-space: nowrap; }
.row-health-metrics b { color: var(--text-main); font-weight: 750; }
.row-health-metrics b.success { color: var(--success); }
.row-health-metrics b.warning { color: var(--warning); }
.row-health-metrics b.danger { color: var(--danger); }
.row-health-detail { margin-left: auto; padding: 0; border: 0; background: transparent; color: var(--primary); font-size: 12px; font-weight: 700; white-space: nowrap; cursor: pointer; }
.row-health-detail:hover { text-decoration: underline; }
.row-health-expanded { margin-top: 10px; border-top: 1px solid var(--border); padding-top: 10px; }
.row-health-diagnosis { margin: 0 0 10px; color: var(--text-main); font-size: 12px; line-height: 1.45; }
.row-health-comparisons { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 7px; }
.row-health-compare { display: flex; flex-direction: column; gap: 3px; min-width: 0; padding: 9px 10px; border: 1px solid var(--border); border-radius: 6px; background: var(--card); }
.row-health-compare span, .row-health-compare small { color: var(--text-secondary); font-size: 11px; }
.row-health-compare strong { color: var(--text-main); font-size: 14px; line-height: 1.35; }
.row-health-extra { display: flex; flex-wrap: wrap; gap: 5px 15px; margin-top: 10px; color: var(--text-secondary); font-size: 11px; }
.row-health-distribution { margin-top: 10px; }
.row-health-distribution-title { color: var(--text-main); font-size: 11px; font-weight: 700; }
.row-health-bin-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px 13px; margin-top: 5px; }
.row-health-bin { display: grid; grid-template-columns: 1fr auto; gap: 3px 8px; color: var(--text-secondary); font-size: 10px; }
.row-health-bin-track { grid-column: 1 / -1; height: 4px; border-radius: 3px; background: var(--border); overflow: hidden; }
.row-health-bin-track i { display: block; height: 100%; background: var(--primary); }
.row-health-bin-track i.normal { background: var(--success); }
.row-health-scope { margin: 9px 0 0; color: var(--text-secondary); font-size: 10px; }
.latency-table .row-readout-value.average { color: var(--text-main); }
.latency-table .readout-header { flex-direction: row; align-items: center; gap: 4px; }
.latency-table .row-readout-time { font-size: 10px; }
.latency-table .readout-observation { gap: 6px; }
.latency-table .node-meta { font-size: 11px; gap: 3px 6px; }
.latency-table .row-health-overview { display: block; padding: 10px 0 0; margin: 5px 0 0 52px; border: 0; border-top: 1px solid var(--border); border-radius: 0; background: transparent; }
.latency-table .row-health-summary { display: flex; align-items: center; gap: 12px; min-width: 0; }
.latency-table .row-health-heading { flex: none; }
.latency-table .row-health-heading strong { padding: 4px 8px; border-radius: 5px; background: var(--card-subtle); color: var(--text-secondary); font-size: 12px; }
.latency-table .row-health-overview.warning .row-health-heading strong { background: rgba(245,158,11,.10); color: var(--warning); }
.latency-table .row-health-overview.danger .row-health-heading strong { background: rgba(239,68,68,.10); color: var(--danger); }
.latency-table .row-health-overview.success .row-health-heading strong { background: rgba(16,185,129,.10); color: var(--success); }
.latency-table .health-key-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); flex: 1; min-width: 0; }
.latency-table .health-key-metric { display: flex; flex-direction: column; gap: 2px; min-width: 0; padding-left: 10px; border-left: 1px solid var(--border); }
.latency-table .health-key-metric span { color: var(--text-secondary); font-size: 10px; }
.latency-table .health-key-metric strong { overflow: hidden; color: var(--text-main); font-size: 13px; font-weight: 690; line-height: 1.25; white-space: nowrap; text-overflow: ellipsis; font-variant-numeric: tabular-nums; }
.latency-table .health-key-metric strong.success { color: var(--success); }
.latency-table .health-key-metric strong.warning { color: var(--warning); }
.latency-table .health-key-metric strong.danger { color: var(--danger); }
.latency-table .row-health-detail { flex: none; margin-left: 0; font-size: 11px; font-weight: 600; }
.latency-table .row-health-comparisons { display: flex; flex-wrap: wrap; gap: 6px 20px; margin: 10px 0 0; padding: 7px 10px; border: 0; border-radius: 5px; background: var(--card-subtle); }
.latency-table .row-health-compare { flex-direction: row; align-items: baseline; padding: 0; border: 0; border-radius: 0; background: transparent; gap: 5px; }
.latency-table .row-health-compare strong { font-size: 11px; font-weight: 600; font-variant-numeric: tabular-nums; }
.latency-table .row-health-compare small { display: none; }
.latency-table .health-no-comparison { color: var(--text-secondary); font-size: 11px; }
.latency-table .row-health-expanded { min-width: 0; margin: 6px 0 0; padding: 0; border: 0; }
.latency-table .row-health-extra { display: flex; gap: 4px 14px; margin: 0; color: var(--text-secondary); font-size: 10px; line-height: 1.5; }
.latency-table .row-health-extra b { font-weight: 500; color: var(--text-main); }
.latency-table .health-failure-note { color: var(--danger); font-weight: 600; }
.latency-table .row-health-explanation { margin: 4px 0 0; color: var(--text-secondary); font-size: 10px; }
@media (max-width: 680px) {
  .latency-table .row-health-overview { margin-left: 0; }
  .latency-table .row-health-summary { flex-wrap: wrap; }
  .latency-table .health-key-metrics { order: 3; flex-basis: 100%; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px 0; }
  .latency-table .row-health-detail { margin-left: auto; }
}
.latency-table .row-health-distribution { margin-top: 7px; }
.latency-table .row-health-distribution-title { color: var(--text-secondary); font-weight: 500; }
.latency-table .row-health-bin-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 5px 12px; margin-top: 3px; }
.latency-table .row-health-scope { margin-top: 5px; }
.health-reason-trigger { position: relative; cursor: help; outline-offset: 3px; }
.health-reason-tooltip { display: none; position: absolute; bottom: calc(100% + 7px); left: 0; z-index: 40; width: 290px; max-width: calc(100vw - 100px); padding: 10px 12px; border: 1px solid var(--border); border-radius: 8px; background: var(--card-bg); color: var(--text-main); box-shadow: 0 5px 18px rgba(0,0,0,.12); font-size: 12px; line-height: 1.6; font-weight: 400; }
.health-reason-trigger:hover .health-reason-tooltip, .health-reason-trigger:focus .health-reason-tooltip { display: block; }
@media (max-width: 1000px) { .latency-table .row-health-bin-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (min-width: 861px) {
  .latency-table .comparison-head, .latency-table .node-row { grid-template-columns: 20px 148px minmax(0, 1fr); column-gap: 12px; }
  .latency-table .comparison-head > :nth-child(3) { display: none; }
  .latency-table .node-row { min-height: 116px; padding-top: 16px; padding-bottom: 16px; grid-template-rows: auto 1fr; row-gap: 12px; align-items: start; }
  .latency-table .select-cell { grid-column: 1; grid-row: 1; margin-top: 4px; }
  .latency-table .node-evidence { grid-column: 2; grid-row: 1; padding-top: 8px; }
  .latency-table .row-readout { grid-column: 2; grid-row: 2; padding-top: 10px; border-top: 1px solid var(--border); margin-right: 8px; }
  .latency-table .row-readout-value { font-size: 22px; font-variant-numeric: tabular-nums; }
  .latency-table .row-readout-state { font-size: 11px; }
  .latency-table .history-cell { grid-column: 3; grid-row: 1 / 3; min-width: 0; }
  .latency-table .node-name strong { font-size: 13px; }
  .latency-table .node-row-actions { opacity: .75; }
  .latency-table .node-row-actions:hover, .latency-table .node-row-actions:focus-within { opacity: 1; }
}
@media (max-width: 860px) {
  .latency-table .comparison-head { display: none; }
  .latency-table .node-row { grid-template-columns: 24px minmax(120px, 1fr) 100px; gap: 12px; }
  .latency-table .history-cell { grid-column: 2 / -1; }
  .latency-table .row-readout { grid-column: 3; }
  .latency-table .node-evidence { grid-column: 2; }
}
@media (max-width: 680px) { .health-grid-cards { grid-template-columns: 1fr; } .batch-item-row { grid-template-columns: 1fr 1fr; } .batch-item-result { grid-column: 1 / -1; } .batch-scroll { max-height: 300px; } }
</style>

<style scoped>
.prototype-page { padding-top: 28px; }
.prototype-page-heading { border: 0; padding-bottom: 0; margin-bottom: 22px; align-items: flex-end; }
.prototype-page-heading h1 { margin: 7px 0 0; font-size: 26px; font-weight: 650; letter-spacing: -.7px; }
.prototype-eyebrow { color: var(--text-muted); font-size: 10px; font-weight: 500; letter-spacing: 1.3px; }
.workspace-context { font-size: 11px; color: var(--text-secondary); }
.prototype-panel { box-shadow: none; border-radius: 11px; }
.prototype-panel-header { align-items: center; padding: 18px 22px; flex-wrap: wrap; }
.prototype-panel-header h2 { font-size: 16px; font-weight: 650; }
.prototype-panel-header p { font-size: 11px; }
.workbench-control-shell { box-shadow: none; }
.project-bar-heading { display: none; }
.project-tabs { gap: 4px; padding: 3px; background: var(--card-subtle); border-radius: 7px; }
.project-tab { padding: 8px 14px; border: 0; font-size: 12px; font-weight: 550; border-radius: 5px; }
.project-tab.active { background: var(--primary); color: white; border-color: transparent; }
.project-bar-actions { padding-top: 7px; }
.batch-test-status { font-size: 10px; color: var(--text-secondary); }
.category-tabs { background: transparent; border: 0; }
.category-tab { padding: 6px 10px; font-size: 11px; }
.category-tab.active { background: var(--primary-subtle); color: var(--primary); box-shadow: none; }
.latency-table .node-row { border-bottom-color: color-mix(in srgb, var(--border) 70%, transparent); }
.latency-table .row-health-overview { background: transparent; }
@media(max-width: 700px) { .workspace-context { display: none; }.prototype-page-heading h1 { font-size: 22px; } }
</style>
