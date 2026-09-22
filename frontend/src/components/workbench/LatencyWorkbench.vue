<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { fetchMonitorNodeOptions } from '../../api/monitor'
import * as api from '../../api/bridge'
import UiSelect from '../common/UiSelect.vue'
import LatencySamplePlot from './LatencySamplePlot.vue'
import {
  acceptsLatencyDetailResponse,
  acceptsLatencyTestResult,
  freezeLatencyWindow,
  sameLatencyScope,
  type LatencyAttemptScope,
  type LatencyScope,
  type LatencyWindow,
  type LatencyWindowMode,
} from './latencyRequestGuard'
import type { MonitorNodeOption, WorkbenchLatencySample, WorkbenchLatencyTest } from '../../types'

type WorkbenchProject = 'latency' | 'throughput' | 'service'
type IndexMap = Record<string, number | null | undefined>
type HistoryMeta = { hasMore: boolean; complete: boolean }

const emit = defineEmits<{ (event: 'open-monitor'): void }>()
const options = ref<MonitorNodeOption[]>([])
const optionsLoading = ref(false)
const optionsError = ref('')
const historyLoading = ref(false)
const historyError = ref('')
const selectedProfileId = ref('all')
const searchText = ref('')
const sortBy = ref<'p50' | 'p95' | 'health' | 'name'>('p50')
const windowMode = ref<LatencyWindowMode>('4h')
const activeWindow = ref<LatencyWindow>(freezeLatencyWindow(windowMode.value))
const activeProject = ref<WorkbenchProject>('latency')
const selectedService = ref<'antigravity' | 'public-api' | 'streaming'>('antigravity')
const selectedKeys = ref<string[]>([])
const focusedKey = ref('')
const hoveredByKey = ref<IndexMap>({})
const pinnedByKey = ref<IndexMap>({})
const historyByKey = ref<Record<string, WorkbenchLatencyTest[]>>({})
const historyMetaByKey = ref<Record<string, HistoryMeta>>({})
const currentByKey = ref<Record<string, WorkbenchLatencyTest | undefined>>({})
const pendingPersistence = new Map<string, WorkbenchLatencyTest>()
const expanded = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
const detailTest = ref<WorkbenchLatencyTest | null>(null)
const selectedAttemptID = ref('')
const testError = ref('')
const testingKey = ref('')
const batchMessage = ref('')

let optionsRequestID = 0
let historyRequestID = 0
let detailRequestID = 0
let runRequestID = 0
let activeRunID = 0
let unsubscribeEvents: (() => void) | null = null

const workbenchProjects = [
  { id: 'latency' as const, label: '延迟与稳定性', available: true },
  { id: 'throughput' as const, label: '吞吐', available: false },
  { id: 'service' as const, label: '服务可用性', available: false },
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

const profileSelectOptions = computed(() => [
  { value: 'all', label: `全部订阅（${profileOptions.value.length}）` },
  ...profileOptions.value.map((profile) => ({ value: profile.id, label: `${profile.name}（${profile.count} 节点）` })),
])
const windowSelectOptions = [
  { value: '4h', label: '最近 4 小时' },
  { value: '24h', label: '最近 24 小时' },
]
const sortSelectOptions = [
  { value: 'p50', label: 'P50 从低到高' },
  { value: 'p95', label: 'P95 从低到高' },
  { value: 'health', label: '健康优先' },
  { value: 'name', label: '名称' },
]
const serviceSelectOptions = [
  { value: 'antigravity', label: 'Antigravity · 已支持接口待接入工作台' },
  { value: 'public-api', label: '公共服务 · 未接入', disabled: true },
  { value: 'streaming', label: '流媒体 · 未接入', disabled: true },
]

function scopeKey(option: Pick<MonitorNodeOption, 'profileId' | 'nodeKey'>): string {
  return `${option.profileId}\u0000${option.nodeKey}`
}

function optionForKey(key: string): MonitorNodeOption | null {
  return options.value.find((option) => scopeKey(option) === key) || null
}

function testsForKey(key: string): WorkbenchLatencyTest[] {
  const saved = (historyByKey.value[key] || []).filter((test) => samplesInActiveWindow(test.samples).length > 0)
  const current = currentByKey.value[key]
  const visibleCurrent = current && samplesInActiveWindow(current.samples).length > 0 ? current : undefined
  if (!visibleCurrent) return saved
  return [visibleCurrent, ...saved.filter((test) => test.attempt_id !== visibleCurrent.attempt_id)]
}

function latestTestForKey(key: string): WorkbenchLatencyTest | null {
  return testsForKey(key)[0] || null
}

function samplesInActiveWindow(samples: WorkbenchLatencySample[] | undefined): WorkbenchLatencySample[] {
  const sinceMs = Date.parse(activeWindow.value.since)
  const untilMs = Date.parse(activeWindow.value.until)
  if (!Number.isFinite(sinceMs) || !Number.isFinite(untilMs)) return []
  return (samples || []).filter((sample) => {
    const timestampMs = Date.parse(sample.timestamp)
    return Number.isFinite(timestampMs) && timestampMs >= sinceMs && timestampMs < untilMs
  })
}

function displayTestForActiveWindow(test: WorkbenchLatencyTest): WorkbenchLatencyTest {
  return { ...test, samples: samplesInActiveWindow(test.samples) }
}

function samplesForKey(key: string): WorkbenchLatencySample[] {
  const samples = testsForKey(key).flatMap((test) => test.samples || [])
  return samplesInActiveWindow(samples).map((sample, index) => ({ ...sample, seq: index + 1 })).sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp))
}

function historyMetaForKey(key: string): HistoryMeta | null {
  return historyMetaByKey.value[key] || null
}

function hasCompleteHistoryWindow(key: string): boolean {
  const meta = historyMetaForKey(key)
  return !meta || (meta.complete && !meta.hasMore)
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

function visibleOptionsForProject(): MonitorNodeOption[] {
  const query = searchText.value.trim().toLocaleLowerCase()
  const filtered = options.value.filter((option) => {
    if (selectedProfileId.value !== 'all' && option.profileId !== selectedProfileId.value) return false
    if (!query) return true
    return [option.displayName, option.profileName, option.countryCode, option.type].filter(Boolean).some((value) => value.toLocaleLowerCase().includes(query))
  })
  return filtered.sort((a, b) => {
    const aKey = scopeKey(a), bKey = scopeKey(b)
    if (sortBy.value === 'name') return a.displayName.localeCompare(b.displayName, 'zh-CN')
    if (sortBy.value === 'health') return visibleStatus(aKey).localeCompare(visibleStatus(bKey))
    const aValue = sortBy.value === 'p95' ? p95ForKey(aKey) : p50ForKey(aKey)
    const bValue = sortBy.value === 'p95' ? p95ForKey(bKey) : p50ForKey(bKey)
    if (aValue === null && bValue === null) return a.displayName.localeCompare(b.displayName, 'zh-CN')
    if (aValue === null) return 1
    if (bValue === null) return -1
    return aValue - bValue
  })
}

const visibleOptions = computed(visibleOptionsForProject)
const focusedOption = computed(() => optionForKey(focusedKey.value))
const focusedTests = computed(() => (focusedKey.value ? testsForKey(focusedKey.value) : []))
const focusedDisplayedTest = computed(() => {
  const test = detailTest.value || latestTestForKey(focusedKey.value)
  return test ? displayTestForActiveWindow(test) : null
})
const canRun = computed(() => activeProject.value === 'latency' && selectedKeys.value.length === 1 && !testingKey.value)
const projectLabel = computed(() => workbenchProjects.find((project) => project.id === activeProject.value)?.label || '延迟与稳定性')
const windowLabel = computed(() => windowMode.value === '4h' ? '最近 4 小时' : '最近 24 小时')
const historyAxisLabels = computed(() => windowMode.value === '4h'
  ? ['最近 4 小时', '−3h', '−2h', '−1h', '现在']
  : ['最近 24 小时', '−18h', '−12h', '−6h', '现在'])

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
  if (sample.success && sample.latency_ms > 0) return `${formatShortTime(sample.timestamp)} · 成功`
  return `${formatShortTime(sample.timestamp)} · ${sample.error || '连接失败'}`
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

function readoutState(key: string): string {
  if (typeof hoveredByKey.value[key] === 'number') return '悬停'
  if (typeof pinnedByKey.value[key] === 'number') return '已固定'
  return '最新'
}

function readoutValue(key: string): string {
  const sample = activeSampleFor(key)
  return sample && sample.success && sample.latency_ms > 0 ? String(Math.round(sample.latency_ms)) : sampleLabel(sample)
}
function readoutTime(key: string): string { const sample = activeSampleFor(key); return sample ? `${formatShortTime(sample.timestamp)}${readoutState(key) === '最新' ? ' · 最新' : ''}` : '没有记录' }
function readoutStatus(key: string): string { const sample = activeSampleFor(key); return sample && !(sample.success && sample.latency_ms > 0) ? sample.error || '连接失败' : statusLabel(key) }

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
  if (!tests.length) return '没有保存记录；完成一次真实延迟测试后会在这里出现。'
  const meta = historyMetaForKey(key)
  if (meta?.hasMore || meta?.complete === false) {
    return `已加载 ${tests.length} 次测试、${samples.length} 条原始样本；${windowLabel.value} 数据未完整加载，暂不计算完整 P50/P95。`
  }
  return `共 ${tests.length} 次测试，${samples.length} 条原始样本，${failed} 次失败；P50/P95 由这些样本派生。`
}

function statsLabel(key: string): string {
  const meta = historyMetaForKey(key)
  if (meta?.hasMore || meta?.complete === false) return '窗口数据未完整加载'
  const p50 = p50ForKey(key)
  const p95 = p95ForKey(key)
  return p50 === null ? '暂无延迟历史' : `P50 ${p50} ms · P95 ${p95 ?? '—'} ms`
}

function monitorStatusText(_key: string): string { return '持续监测状态请在“持续监测”查看' }
function testMatchesKey(test: WorkbenchLatencyTest, key: string): boolean { const option = optionForKey(key); return !!option && test.profile_id === option.profileId && test.node_key === option.nodeKey }

function upsertHistory(key: string, test: WorkbenchLatencyTest): void {
  const next = [test, ...(historyByKey.value[key] || []).filter((item) => item.attempt_id !== test.attempt_id)]
  next.sort((a, b) => Date.parse(b.finished_at) - Date.parse(a.finished_at))
  historyByKey.value = { ...historyByKey.value, [key]: next }
}

function takePendingPersistence(test: WorkbenchLatencyTest): WorkbenchLatencyTest {
  const pending = pendingPersistence.get(test.attempt_id)
  const key = scopeKey({ profileId: test.profile_id, nodeKey: test.node_key })
  if (!pending || !testMatchesKey(pending, key)) return test
  pendingPersistence.delete(test.attempt_id)
  return pending
}

function isWorkbenchLatencyTestPayload(payload: unknown): payload is WorkbenchLatencyTest {
  if (!payload || typeof payload !== 'object') return false
  const test = payload as Partial<WorkbenchLatencyTest>
  return typeof test.attempt_id === 'string' && typeof test.profile_id === 'string' && typeof test.node_key === 'string' && (test.persistence_state === 'saving' || test.persistence_state === 'saved' || test.persistence_state === 'failed')
}

function handlePersistenceEvent(type: string, payload: unknown): void {
  if (type !== 'workbench_latency_test_persistence_updated' || !isWorkbenchLatencyTestPayload(payload)) return
  const key = scopeKey({ profileId: payload.profile_id, nodeKey: payload.node_key })
  pendingPersistence.set(payload.attempt_id, payload)
  while (pendingPersistence.size > 20) {
    const oldest = pendingPersistence.keys().next().value
    if (oldest) pendingPersistence.delete(oldest)
    else break
  }
  currentByKey.value = { ...currentByKey.value, [key]: payload }
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
    const validKeys = new Set(loaded.map(scopeKey))
    selectedKeys.value = selectedKeys.value.filter((key) => validKeys.has(key))
    if (focusedKey.value && !validKeys.has(focusedKey.value)) { focusedKey.value = ''; detailTest.value = null }
    await loadHistories(loaded)
  } catch (error) {
    if (requestID === optionsRequestID) optionsError.value = messageFor(error)
  } finally {
    if (requestID === optionsRequestID) optionsLoading.value = false
  }
}

async function loadHistories(nodes: MonitorNodeOption[]): Promise<void> {
  const requestID = ++historyRequestID
  const requestedWindow = freezeLatencyWindow(windowMode.value)
  activeWindow.value = requestedWindow
  if (!nodes.length) {
    historyByKey.value = {}
    historyMetaByKey.value = {}
    historyLoading.value = false
    return
  }
  historyLoading.value = true
  const errors: string[] = []
  const entries = await Promise.all(nodes.map(async (node) => {
    const key = scopeKey(node)
    try {
      const response = await api.fetchWorkbenchLatencyHistory({
        profile_id: node.profileId,
        node_key: node.nodeKey,
        since: requestedWindow.since,
        until: requestedWindow.until,
        limit: 20,
      })
      if (response.tests.some((test) => !testMatchesKey(test, key))) {
        errors.push(`${node.displayName || node.nodeKey}：响应归属不一致`)
        return { key, tests: [] as WorkbenchLatencyTest[], meta: { hasMore: false, complete: false } as HistoryMeta }
      }
      return { key, tests: response.tests, meta: { hasMore: response.has_more, complete: response.complete } as HistoryMeta }
    } catch (error) {
      errors.push(`${node.displayName || node.nodeKey}：${messageFor(error)}`)
      return { key, tests: [] as WorkbenchLatencyTest[], meta: { hasMore: false, complete: false } as HistoryMeta }
    }
  }))
  if (requestID !== historyRequestID) return
  historyByKey.value = Object.fromEntries(entries.map((entry) => [entry.key, entry.tests]))
  historyMetaByKey.value = Object.fromEntries(entries.map((entry) => [entry.key, entry.meta]))
  historyError.value = errors.length > 0 ? `部分节点历史读取失败：${errors.slice(0, 2).join('；')}${errors.length > 2 ? '…' : ''}` : ''
  historyLoading.value = false
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
function clearPin(key: string): void { pinnedByKey.value = { ...pinnedByKey.value, [key]: null }; hoveredByKey.value = { ...hoveredByKey.value, [key]: null } }

async function runTest(): Promise<void> {
  if (activeProject.value !== 'latency') { testError.value = '当前正式接口只支持“延迟与稳定性”；吞吐和服务可用性先不显示演示结果。'; return }
  if (testingKey.value) return
  if (selectedKeys.value.length !== 1) { testError.value = selectedKeys.value.length === 0 ? '先勾选一个节点，再点击“立即测试”。' : '当前正式接口是单节点闭环，请只保留一个节点后测试。'; return }
  const key = selectedKeys.value[0], option = optionForKey(key)
  if (!option) return
  const scope = { profileId: option.profileId, nodeKey: option.nodeKey }
  const requestID = ++runRequestID
  activeRunID = requestID
  testingKey.value = key
  testError.value = ''
  batchMessage.value = `正在测试“${option.displayName}”……`
  try {
    const result = await api.runWorkbenchLatencyTest({ profile_id: scope.profileId, node_key: scope.nodeKey, test_project: 'latency_stability', timeout_seconds: 5 })
    if (!acceptsLatencyTestResult(requestID, runRequestID, scope, currentScope(key), result)) return
    const boundResult = takePendingPersistence(result)
    currentByKey.value = { ...currentByKey.value, [key]: boundResult }
    upsertHistory(key, boundResult)
    focusNode(key)
    detailTest.value = displayTestForActiveWindow(boundResult)
    selectedAttemptID.value = boundResult.attempt_id
    batchMessage.value = boundResult.persistence_state === 'failed' ? `测试完成，但历史保存失败：${boundResult.persistence_error || '未知原因'}` : boundResult.persistence_state === 'saving' ? '真实结果已返回，历史正在保存……' : '真实结果已返回，历史已保存。'
  } catch (error) {
    if (requestID === activeRunID && sameLatencyScope(scope, currentScope(key))) { testError.value = messageFor(error); batchMessage.value = '' }
  } finally {
    if (requestID === activeRunID) { activeRunID = 0; testingKey.value = '' }
  }
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

function openHistory(key: string): void { focusNode(key); if (!latestTestForKey(key)) { detailError.value = '这个节点还没有保存历史；完成一次真实延迟测试后再展开。'; return }; expanded.value = true }
function closeHistory(): void { expanded.value = false; detailError.value = '' }
function changeProject(project: WorkbenchProject): void { activeProject.value = project; batchMessage.value = ''; testError.value = '' }
function projectUnavailableLabel(project: WorkbenchProject): string { return project === 'throughput' ? '吞吐历史暂未接入正式工作台接口' : selectedService.value === 'antigravity' ? 'Antigravity 服务历史暂未接入正式工作台接口' : '该服务未接入，不能生成测试结果' }
function projectReadout(key: string): string { return activeProject.value === 'latency' ? readoutValue(key) : '未接入' }
function projectHistoryText(key: string): string { return activeProject.value === 'latency' ? historySummary(key) : projectUnavailableLabel(activeProject.value) }
function isSelected(key: string): boolean { return selectedKeys.value.includes(key) }
function isTesting(key: string): boolean { return testingKey.value === key }

watch(windowMode, () => {
  activeWindow.value = freezeLatencyWindow(windowMode.value)
  historyRequestID++
  detailRequestID++
  historyByKey.value = {}
  historyMetaByKey.value = {}
  detailTest.value = null
  detailError.value = ''
  detailLoading.value = false
  selectedAttemptID.value = ''
  expanded.value = false
  hoveredByKey.value = {}
  pinnedByKey.value = {}
  if (options.value.length > 0) void loadHistories(options.value)
})

onMounted(async () => { unsubscribeEvents = api.subscribeEvents(handlePersistenceEvent); await loadOptions() })
onUnmounted(() => { unsubscribeEvents?.(); unsubscribeEvents = null })
</script>

<template>
  <main class="prototype-page prototype-latency-workbench">
    <section class="prototype-page-heading">
      <p class="prototype-eyebrow">节点工作台 · 测试与观察</p>
      <div class="prototype-demo-note"><strong>实时数据</strong> · 节点来自当前订阅缓存，不展示凭据</div>
    </section>

    <section class="prototype-scope-bar" aria-label="范围筛选">
      <span class="scope-title">范围</span>
      <label class="scope-control">订阅<UiSelect v-model="selectedProfileId" variant="scope" aria-label="选择订阅" :options="profileSelectOptions" /></label>
      <span class="scope-separator" aria-hidden="true"></span>
      <label class="scope-control">观察窗口<UiSelect v-model="windowMode" variant="scope" aria-label="选择观察窗口" :options="windowSelectOptions" /></label>
      <span class="scope-separator" aria-hidden="true"></span>
      <label class="scope-control scope-search-control">搜索<input v-model="searchText" class="scope-search" type="search" placeholder="节点、订阅或地区" aria-label="搜索节点、订阅或地区"></label>
      <label class="scope-control">排序<UiSelect v-model="sortBy" variant="scope" aria-label="节点排序" :options="sortSelectOptions" /></label>
      <span class="live-line">{{ windowLabel }} · 截至 {{ formatShortTime(activeWindow.until) }}</span><span class="selection-summary">已选 {{ selectedKeys.length }} 个节点</span>
      <button type="button" class="prototype-button primary scope-refresh" :disabled="optionsLoading" @click="loadOptions">{{ optionsLoading ? '读取中…' : '重新读取' }}</button>
    </section>

    <section class="prototype-project-bar" aria-label="测试项目">
      <div class="project-bar-heading"><span class="scope-title">测试项目</span><span class="project-bar-note">先选节点；结果直接出现在节点行</span></div>
      <div class="project-tabs" role="tablist" aria-label="切换测试项目"><button v-for="project in workbenchProjects" :key="project.id" type="button" class="project-tab" :class="{ active: activeProject === project.id }" role="tab" :aria-selected="activeProject === project.id" @click="changeProject(project.id)">{{ project.label }}<span v-if="!project.available">尚未接入</span></button></div>
      <div class="project-bar-actions"><span class="selection-summary">{{ selectedKeys.length }} 个节点</span><button type="button" class="prototype-button primary" :disabled="!canRun" @click="runTest">{{ testingKey ? '测试中…' : '立即测试' }}</button><button type="button" class="prototype-button" :disabled="selectedKeys.length === 0" @click="emit('open-monitor')">加入持续监测</button></div>
      <div v-if="activeProject === 'service'" class="service-toolbar"><span class="service-toolbar-label">服务</span><UiSelect v-model="selectedService" variant="toolbar" aria-label="选择服务" :options="serviceSelectOptions" /><span class="service-toolbar-note">选择服务同时决定查看与测试目标；未接入服务不可测试</span></div>
      <div class="batch-test-status" :class="{ running: !!testingKey, blocked: activeProject !== 'latency', complete: !!batchMessage && !testingKey }" aria-live="polite">{{ testError || batchMessage || (activeProject === 'latency' ? '延迟结果来自真实单节点测试；失败、超时和未采样不会画成 0ms。' : projectUnavailableLabel(activeProject)) }}</div>
    </section>

    <section class="prototype-panel comparison-panel" aria-labelledby="comparison-title">
      <div class="prototype-panel-header"><div><h2 id="comparison-title">节点比较</h2><p>{{ activeProject === 'latency' ? '按同一真实时间轴扫过节点变化；悬停、键盘聚焦或点击都能定位原始样本。' : '当前分类先保留原型中的比较位置；正式接口未提供的数据明确留空。' }}</p></div><button type="button" class="text-action" :disabled="selectedKeys.length === 0" @click="clearSelection">清除选择</button></div>
      <div class="comparison-head" aria-hidden="true"><div></div><div>节点 / 订阅</div><div>当前读数</div><div>{{ activeProject === 'latency' ? '历史变化 · 真实时间' : projectLabel }}<div class="history-axis"><span>{{ historyAxisLabels[0] }}</span><span>{{ historyAxisLabels[1] }}</span><span>{{ historyAxisLabels[2] }}</span><span>{{ historyAxisLabels[3] }}</span><span>{{ historyAxisLabels[4] }}</span></div></div></div>
      <div class="history-legend" aria-label="历史图例"><span>每个点是一条真实原始样本；成功与异常之间不连线</span><span class="legend-item"><i class="legend-mark"></i>成功</span><span class="legend-item"><i class="legend-mark fail"></i>失败</span><span class="legend-item"><i class="legend-mark timeout"></i>超时</span><span class="legend-item"><i class="legend-mark missing"></i>未采样</span></div>

      <div v-if="optionsError" class="prototype-state error-state"><strong>节点列表读取失败</strong><span>{{ optionsError }}</span><button type="button" class="prototype-button" @click="loadOptions">重试读取</button></div>
      <div v-else-if="optionsLoading && options.length === 0" class="prototype-state"><strong>正在读取节点与历史</strong><span>不显示旧的成功结果覆盖当前状态。</span></div>
      <div v-else-if="options.length === 0" class="prototype-state"><strong>还没有可比较的节点</strong><span>先刷新订阅；节点必须来自真实缓存配置。</span></div>
      <div v-else-if="visibleOptions.length === 0" class="prototype-state"><strong>当前筛选没有节点</strong><span>可以清空搜索或切换订阅范围。</span></div>

      <ul v-else class="node-list" role="listbox" aria-label="节点列表">
        <li v-for="node in visibleOptions" :key="scopeKey(node)" class="node-row" :aria-selected="focusedKey === scopeKey(node)">
          <label class="select-cell" :aria-label="`选择 ${node.displayName}`" @click.stop><input type="checkbox" :checked="isSelected(scopeKey(node))" @change="toggleSelected(scopeKey(node))"></label>
          <button type="button" class="node-evidence" @click="focusNode(scopeKey(node))"><span class="node-name"><strong :title="node.displayName">{{ node.displayName || '未命名节点' }}</strong></span><span class="node-meta"><span>{{ node.profileName }}</span><span>{{ node.countryCode || '未知地区' }}</span><span>{{ node.type || '节点' }}</span></span><span class="node-detail-link">{{ focusedKey === scopeKey(node) ? '已展开证据' : '查看节点证据' }}</span></button>
          <div class="row-readout" :data-state="readoutState(scopeKey(node))">
            <template v-if="isTesting(scopeKey(node))"><span class="row-readout-state running">测试中</span><strong class="row-readout-value">…</strong><span class="row-readout-time">正在等待真实结果</span></template>
            <template v-else><span class="row-readout-state">{{ activeProject === 'latency' ? readoutState(scopeKey(node)) : projectLabel }}</span><strong class="row-readout-value" :class="activeProject === 'latency' ? sampleClass(activeSampleFor(scopeKey(node))) : 'nodata'">{{ projectReadout(scopeKey(node)) }}<small v-if="activeProject === 'latency' && activeSampleFor(scopeKey(node))?.success">ms</small></strong><span class="row-readout-time">{{ activeProject === 'latency' ? readoutTime(scopeKey(node)) : '暂无正式记录' }}</span><span class="row-readout-status">{{ activeProject === 'latency' ? readoutStatus(scopeKey(node)) : '未接入，不生成演示结果' }}</span><span class="row-readout-monitor">{{ monitorStatusText(scopeKey(node)) }}</span><span v-if="pinnedByKey[scopeKey(node)] !== null && pinnedByKey[scopeKey(node)] !== undefined" class="pin-control"><button type="button" @click.stop="clearPin(scopeKey(node))">取消固定</button></span></template>
          </div>
          <div class="history-cell">
            <template v-if="activeProject === 'latency'"><LatencySamplePlot :samples="samplesForKey(scopeKey(node))" :window-since="activeWindow.since" :window-until="activeWindow.until" :hovered-index="hoveredByKey[scopeKey(node)] ?? null" :pinned-index="pinnedByKey[scopeKey(node)] ?? null" :height="84" @hover="onHover(scopeKey(node), $event)" @pin="onPin(scopeKey(node), $event)" @unpin="clearPin(scopeKey(node))" /><div class="history-summary"><strong>{{ statsLabel(scopeKey(node)) }}</strong><span>{{ projectHistoryText(scopeKey(node)) }}</span></div><button type="button" class="history-open" @click.stop="openHistory(scopeKey(node))">展开大图与样本 →</button></template>
            <template v-else><div class="project-unavailable-cell"><strong>{{ projectReadout(scopeKey(node)) }}</strong><span>{{ projectHistoryText(scopeKey(node)) }}</span></div></template>
          </div>
        </li>
      </ul>
      <div class="compare-footer"><span>显示 {{ visibleOptions.length }} / {{ options.length }} 个真实节点</span><span>{{ historyLoading ? '历史读取中…' : historyError || 'P50/P95 仅作辅助，均由原始样本派生' }}</span></div>
    </section>

    <section v-if="focusedOption" class="prototype-panel evidence-panel" aria-labelledby="evidence-title">
      <div class="prototype-panel-header"><div><h2 id="evidence-title">{{ focusedOption.displayName }}</h2><p>{{ focusedOption.profileName }} · {{ focusedOption.countryCode || '未知地区' }} · 选中后查看同一份原始样本</p></div><div class="evidence-actions"><button type="button" class="prototype-button primary" :disabled="selectedKeys.length !== 1 || !!testingKey" @click="runTest">{{ testingKey ? '测试中…' : '立即测试此节点' }}</button><button type="button" class="prototype-button" @click="emit('open-monitor')">加入持续监测</button></div></div>
      <div v-if="focusedDisplayedTest" class="evidence-grid"><div class="evidence-facts"><span class="fact-label">节点判断</span><strong :class="`health-${visibleStatus(focusedKey)}`">{{ statusLabel(focusedKey) }}</strong><span>{{ historySummary(focusedKey) }}</span><span>任务状态与节点健康分开读取；{{ monitorStatusText(focusedKey) }}</span></div><div class="evidence-facts"><span class="fact-label">当前样本</span><strong>{{ sampleLabel(activeSampleFor(focusedKey)) }}</strong><span>{{ sampleDetail(activeSampleFor(focusedKey)) }}</span><span>{{ focusedDisplayedTest ? `本次 ${testStatusLabel(focusedDisplayedTest)} · ${persistenceLabel(focusedDisplayedTest)}` : '' }}</span></div><div class="evidence-facts"><span class="fact-label">辅助读数</span><strong>P50 {{ p50ForKey(focusedKey) ?? '—' }} <small>ms</small></strong><span>P95 {{ p95ForKey(focusedKey) ?? '—' }} ms</span><span>失败 {{ samplesForKey(focusedKey).filter((sample) => !sample.success).length }} 条</span></div></div>
      <div v-else class="evidence-empty">这个节点还没有真实延迟历史。点击“立即测试此节点”后，结果会先显示，再独立保存。</div>
      <div v-if="detailError" class="inline-error">{{ detailError }}</div>
      <div v-if="focusedTests.length" class="evidence-history-list"><div class="section-label">已保存历史</div><button v-for="test in focusedTests" :key="test.attempt_id" type="button" class="history-record" :class="{ selected: selectedAttemptID === test.attempt_id }" @click="selectHistory(test)"><span><strong>{{ testStatusLabel(test) }}</strong><small>{{ formatTime(test.finished_at) }}</small></span><span>{{ test.success_samples }}/{{ test.total_samples }} 成功</span><span>{{ test.latency_ms > 0 ? `${Math.round(test.latency_ms)} ms` : '无有效延迟' }}</span></button></div>
      <button v-if="focusedDisplayedTest" type="button" class="history-open evidence-expand" @click="expanded = true">展开完整历史与原始样本 →</button>
    </section>

    <div v-if="expanded && focusedDisplayedTest" class="prototype-modal-backdrop" @click.self="closeHistory"><section class="prototype-history-modal" role="dialog" aria-modal="true" aria-labelledby="history-modal-title"><div class="modal-header"><div><p class="prototype-eyebrow">历史证据 · 原始样本</p><h2 id="history-modal-title">{{ focusedOption?.displayName }} · 延迟历史</h2><p>{{ focusedOption?.profileName }} · {{ formatTime(focusedDisplayedTest.started_at) }} – {{ formatTime(focusedDisplayedTest.finished_at) }}</p></div><div class="modal-actions"><button v-if="pinnedByKey[focusedKey] !== null && pinnedByKey[focusedKey] !== undefined" type="button" class="prototype-button" @click="clearPin(focusedKey)">取消固定</button><button type="button" class="close-button" aria-label="关闭历史详情" @click="closeHistory">×</button></div></div><div class="history-controls"><label>时间范围<UiSelect v-model="windowMode" aria-label="历史时间范围" :options="windowSelectOptions" /></label><span>{{ windowLabel }} · 截至 {{ formatShortTime(activeWindow.until) }} · 横轴为真实采样时间 · 纵轴为毫秒</span><span v-if="detailLoading">正在读取详情…</span></div><div class="modal-chart"><LatencySamplePlot :samples="focusedDisplayedTest.samples" :window-since="activeWindow.since" :window-until="activeWindow.until" :hovered-index="hoveredByKey[focusedKey] ?? null" :pinned-index="pinnedByKey[focusedKey] ?? null" :width="1120" :height="300" @hover="onHover(focusedKey, $event)" @pin="onPin(focusedKey, $event)" @unpin="clearPin(focusedKey)" /></div><div class="modal-current"><strong>{{ sampleLabel(activeSampleFor(focusedKey)) }}</strong><span>{{ sampleDetail(activeSampleFor(focusedKey)) }}</span><span>{{ testStatusLabel(focusedDisplayedTest) }} · {{ persistenceLabel(focusedDisplayedTest) }}</span></div><div class="modal-table-wrap"><table><thead><tr><th>样本</th><th>实际时间</th><th>结果</th><th>原因</th></tr></thead><tbody><tr v-for="(sample, index) in focusedDisplayedTest.samples" :key="`${sample.seq}-${sample.timestamp}`" :class="{ selected: activeIndexFor(focusedKey, focusedDisplayedTest.samples) === index }"><td>#{{ sample.seq }}</td><td>{{ formatTime(sample.timestamp) }}</td><td :class="sample.success ? 'success' : 'fail'">{{ sample.success ? `${Math.round(sample.latency_ms)} ms` : sampleLabel(sample) }}</td><td>{{ sample.error || '—' }}</td></tr></tbody></table></div></section></div>
  </main>
</template>

<style scoped>
.prototype-page { max-width: 1600px; margin: 0 auto; padding: 14px 34px 52px; color: var(--text-main); }
.prototype-page-heading { display: flex; align-items: center; justify-content: space-between; gap: 18px; min-height: 24px; margin-bottom: 12px; padding-bottom: 10px; border-bottom: 1px solid var(--border); }
.prototype-eyebrow { margin: 0; color: var(--primary); font-size: 12px; font-weight: 750; letter-spacing: .08em; }
.prototype-demo-note { max-width: 310px; padding: 5px 9px; border-left: 2px solid var(--warning); background: #fffaf0; color: #74531f; font-size: 11px; line-height: 1.35; }
.prototype-scope-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 18px; margin-bottom: 18px; padding: 13px 15px; border: 1px solid var(--border); border-radius: 12px; background: var(--card-bg); }
.scope-title { color: var(--text-secondary); font-size: 12px; font-weight: 700; }
.scope-control { display: flex; align-items: center; gap: 7px; color: var(--text-secondary); font-size: 12px; }
.scope-control select { min-width: 130px; padding: 4px 19px 4px 2px; border: 0; border-bottom: 1px solid var(--border-subtle); border-radius: 0; background: transparent; color: var(--text-main); }
.scope-search-control { min-width: 190px; }.scope-search { min-width: 190px; padding: 6px 8px; border: 1px solid var(--border-subtle); border-radius: 6px; background: white; color: var(--text-main); font-size: 12px; }.scope-search::placeholder { color: var(--text-muted); }
.scope-separator { width: 1px; height: 21px; background: var(--border); }.live-line { color: var(--text-secondary); font-size: 11px; }.selection-summary { color: var(--primary); font-size: 11px; font-weight: 700; }.scope-refresh { margin-left: auto; }
.prototype-button { display: inline-flex; align-items: center; justify-content: center; min-height: 36px; padding: 7px 12px; border: 1px solid var(--border-subtle); border-radius: 7px; background: white; color: var(--text-main); font-size: 12px; font-weight: 700; }.prototype-button:hover:not(:disabled) { border-color: var(--primary); color: var(--primary); }.prototype-button.primary { border-color: var(--primary); background: var(--primary); color: white; }.prototype-button.primary:hover:not(:disabled) { background: var(--primary-hover); color: white; }.prototype-button:disabled { cursor: not-allowed; opacity: .52; }
.prototype-project-bar { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; grid-template-areas: "heading tabs actions" "service service service" "status status status"; gap: 10px 18px; align-items: center; margin-bottom: 14px; padding: 11px 15px; border: 1px solid var(--border); border-radius: 12px; background: var(--card-bg); }.project-bar-heading { grid-area: heading; display: flex; align-items: baseline; gap: 9px; white-space: nowrap; }.project-bar-note, .batch-test-status { color: var(--text-secondary); font-size: 11px; }.project-tabs { grid-area: tabs; display: flex; flex-wrap: wrap; gap: 6px; min-width: 0; }.project-tab { min-height: 32px; padding: 6px 10px; border: 1px solid transparent; border-radius: 6px; background: transparent; color: var(--text-secondary); font-size: 12px; font-weight: 700; }.project-tab:hover { border-color: var(--border-subtle); color: var(--text-main); }.project-tab.active { border-color: var(--primary); background: var(--primary-subtle); color: var(--text-main); }.project-tab span { display: block; margin-top: 2px; color: var(--warning); font-size: 9px; font-weight: 600; }.project-bar-actions { grid-area: actions; display: flex; align-items: center; justify-content: flex-end; gap: 8px; }.service-toolbar { grid-area: service; display: flex; align-items: center; gap: 9px; min-width: 0; padding-top: 8px; border-top: 1px solid #edf0f2; color: var(--text-secondary); font-size: 11px; }.service-toolbar-label { color: var(--text-main); font-weight: 750; }.service-toolbar select { max-width: 290px; padding: 5px 24px 5px 7px; border: 1px solid var(--border-subtle); border-radius: 5px; background: white; color: var(--text-main); font-size: 11px; }.service-toolbar-note { color: var(--text-muted); font-size: 10px; }.batch-test-status { grid-area: status; padding-top: 8px; border-top: 1px solid #edf0f2; }.batch-test-status.running { color: var(--primary); }.batch-test-status.complete { color: var(--success); }.batch-test-status.blocked { color: var(--warning); }
.prototype-panel { border: 1px solid var(--border); border-radius: 12px; background: var(--card-bg); box-shadow: 0 14px 35px rgba(37, 52, 64, .08); }.prototype-panel-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; padding: 19px 20px 14px; border-bottom: 1px solid var(--border); }.prototype-panel-header h2 { margin: 0 0 5px; font-size: 17px; letter-spacing: -.02em; }.prototype-panel-header p { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }.text-action { padding: 2px 0; border: 0; background: transparent; color: var(--primary); font-size: 12px; font-weight: 750; }.text-action:disabled { cursor: not-allowed; color: var(--text-muted); }
.comparison-head { display: grid; grid-template-columns: 34px minmax(230px, .86fr) minmax(150px, .54fr) minmax(560px, 2.5fr); gap: 12px; align-items: end; padding: 12px 20px 9px; border-bottom: 1px solid var(--border); color: var(--text-muted); font-size: 11px; font-weight: 700; }.history-axis { display: flex; justify-content: space-between; margin-top: 8px; color: var(--text-muted); font-size: 10px; font-weight: 500; }.history-legend { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 13px; padding: 8px 20px; border-bottom: 1px solid #edf0f2; color: var(--text-secondary); font-size: 10px; }.legend-item { display: inline-flex; align-items: center; gap: 4px; }.legend-mark { width: 8px; height: 8px; border-radius: 50%; background: var(--success); }.legend-mark.fail { border-radius: 0; background: var(--danger); transform: rotate(45deg); }.legend-mark.timeout { border: 2px solid var(--danger); background: white; border-radius: 2px; }.legend-mark.missing { width: 13px; height: 3px; border-radius: 0; background: var(--text-muted); }
.node-list { list-style: none; margin: 0; padding: 0; }.node-row { position: relative; display: grid; grid-template-columns: 34px minmax(230px, .86fr) minmax(150px, .54fr) minmax(560px, 2.5fr); gap: 12px; align-items: center; min-height: 132px; padding: 12px 20px; border-bottom: 1px solid #edf0f2; background: white; }.node-row:last-child { border-bottom: 0; }.node-row:hover { background: #f7fafb; }.node-row[aria-selected="true"] { background: #edf6f8; }.node-row[aria-selected="true"]::before { content: ""; position: absolute; left: 0; top: 9px; bottom: 9px; width: 3px; border-radius: 0 3px 3px 0; background: var(--primary); }.select-cell { display: grid; place-items: center; min-height: 32px; cursor: pointer; }.select-cell input { width: 15px; height: 15px; accent-color: var(--primary); }.node-evidence { min-width: 0; padding: 0; border: 0; background: transparent; color: inherit; text-align: left; }.node-evidence:hover .node-name strong { color: var(--primary); }.node-name { min-width: 0; }.node-name strong { display: block; overflow: hidden; color: var(--text-main); font-size: 14px; line-height: 1.35; text-overflow: ellipsis; white-space: nowrap; }.node-meta { display: flex; flex-wrap: wrap; gap: 5px 9px; margin-top: 5px; color: var(--text-secondary); font-size: 12px; }.node-meta span:first-child { color: var(--primary); }.node-detail-link { display: inline-block; margin-top: 6px; color: var(--primary); font-size: 11px; }
.row-readout { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 2px; }.row-readout-state { color: var(--text-secondary); font-size: 12px; line-height: 1.2; }.row-readout-state.running { color: var(--primary); font-weight: 750; }.row-readout-value { display: flex; align-items: baseline; gap: 4px; max-width: 100%; overflow: hidden; color: var(--text-main); font-size: 20px; font-weight: 780; line-height: 1.08; letter-spacing: -.02em; text-overflow: ellipsis; white-space: nowrap; }.row-readout-value small { color: var(--text-secondary); font-size: 13px; font-weight: 650; letter-spacing: 0; }.row-readout-value.fail, .row-readout-value.timeout { color: var(--danger); }.row-readout-value.success { color: var(--success); }.row-readout-value.nodata { color: var(--text-muted); }.row-readout-time, .row-readout-status, .row-readout-monitor { max-width: 100%; overflow: hidden; color: var(--text-secondary); font-size: 12px; line-height: 1.35; text-overflow: ellipsis; white-space: nowrap; }.row-readout-status { margin-top: 3px; color: var(--text-main); font-weight: 700; }.pin-control button { padding: 0; border: 0; background: transparent; color: var(--primary); font-size: 11px; }
.history-cell { min-width: 0; }.history-cell :deep(.relative) { min-height: 84px; }.history-cell :deep(svg) { width: 100%; }.history-summary { min-width: 0; color: var(--text-secondary); font-size: 11px; line-height: 1.5; }.history-summary strong { display: block; color: var(--text-main); font-size: 12px; }.history-summary span { display: block; margin-top: 3px; }.history-open { display: inline-flex; align-items: center; gap: 5px; margin-top: 5px; padding: 0; border: 0; background: transparent; color: var(--primary); font-size: 12px; font-weight: 750; }.project-unavailable-cell { display: flex; flex-direction: column; justify-content: center; min-height: 84px; padding: 12px; border-left: 3px solid var(--border-subtle); background: #fbfcfd; }.project-unavailable-cell strong { color: var(--text-secondary); font-size: 13px; }.project-unavailable-cell span { margin-top: 6px; color: var(--text-muted); font-size: 11px; line-height: 1.45; }.compare-footer { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; padding: 10px 20px; border-top: 1px solid var(--border); color: var(--text-secondary); font-size: 11px; }
.prototype-state { display: flex; flex-direction: column; align-items: center; gap: 9px; padding: 52px 24px; color: var(--text-secondary); text-align: center; font-size: 12px; }.prototype-state strong { color: var(--text-main); font-size: 17px; }.prototype-state.error-state strong { color: var(--danger); }.evidence-panel { margin-top: 14px; }.evidence-actions { display: flex; flex-wrap: wrap; gap: 8px; }.evidence-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 18px; padding: 17px 20px; border-bottom: 1px solid var(--border); }.evidence-facts { display: flex; flex-direction: column; gap: 5px; color: var(--text-secondary); font-size: 12px; line-height: 1.45; }.evidence-facts strong { color: var(--text-main); font-size: 18px; }.evidence-facts small { font-size: 12px; font-weight: 500; }.fact-label, .section-label { color: var(--text-muted); font-size: 11px; font-weight: 700; }.health-normal { color: var(--success) !important; }.health-flaky { color: var(--warning) !important; }.health-failed { color: var(--danger) !important; }.health-nodata { color: var(--text-muted) !important; }.evidence-empty { padding: 30px 20px; color: var(--text-secondary); font-size: 13px; }.inline-error { margin: 12px 20px; padding: 10px 12px; border-left: 3px solid var(--danger); background: var(--danger-bg); color: var(--danger); font-size: 12px; }.evidence-history-list { padding: 16px 20px 0; }.history-record { display: grid; grid-template-columns: minmax(200px, 1fr) 130px 150px; gap: 14px; width: 100%; padding: 10px 0; border: 0; border-top: 1px solid #edf0f2; background: transparent; color: var(--text-secondary); text-align: left; font-size: 12px; }.history-record:first-of-type { margin-top: 9px; }.history-record:hover, .history-record.selected { color: var(--primary); }.history-record strong, .history-record small { display: block; }.history-record small { margin-top: 3px; color: var(--text-secondary); font-size: 11px; }.evidence-expand { margin: 15px 20px 18px; }
.prototype-modal-backdrop { position: fixed; inset: 0; z-index: 50; display: flex; align-items: center; justify-content: center; padding: 14px; background: rgba(20, 31, 40, .38); }.prototype-history-modal { width: min(1240px, calc(100vw - 28px)); max-height: calc(100vh - 28px); overflow: auto; padding: 24px; border-radius: 13px; background: var(--card-bg); color: var(--text-main); box-shadow: 0 24px 70px rgba(20, 35, 45, .24); }.modal-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 15px; }.modal-header h2 { max-width: 900px; margin: 6px 0; font-size: 22px; line-height: 1.25; }.modal-header p:not(.prototype-eyebrow) { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }.modal-actions { display: flex; align-items: flex-start; gap: 8px; }.close-button { width: 30px; height: 30px; border: 1px solid var(--border); border-radius: 50%; background: white; color: var(--text-secondary); font-size: 18px; line-height: 1; }.history-controls { display: flex; flex-wrap: wrap; align-items: end; gap: 10px 14px; padding: 12px 13px; border: 1px solid var(--border); background: var(--card-subtle); color: var(--text-secondary); font-size: 11px; }.history-controls label { display: grid; gap: 5px; font-size: 10px; font-weight: 700; }.history-controls select { min-width: 170px; padding: 6px 8px; border: 1px solid var(--border-subtle); border-radius: 6px; background: white; color: var(--text-main); font-size: 11px; }.modal-chart { margin-top: 14px; padding: 16px 10px 10px; border: 1px solid var(--border); background: white; }.modal-current { display: flex; flex-wrap: wrap; gap: 10px 18px; margin-top: 12px; padding: 10px 12px; border-left: 3px solid var(--primary); background: var(--primary-subtle); color: var(--text-secondary); font-size: 12px; }.modal-current strong { color: var(--text-main); }.modal-table-wrap { margin-top: 18px; overflow: auto; border-top: 1px solid var(--border); }.modal-table-wrap table { width: 100%; min-width: 680px; border-collapse: collapse; font-size: 12px; }.modal-table-wrap th, .modal-table-wrap td { padding: 9px 8px; border-bottom: 1px solid var(--border); text-align: left; }.modal-table-wrap th { color: var(--text-secondary); font-weight: 700; }.modal-table-wrap tr.selected { background: #edf6f8; }.modal-table-wrap td.success { color: var(--success); font-weight: 700; }.modal-table-wrap td.fail { color: var(--danger); font-weight: 700; }
@media (max-width: 1120px) { .comparison-head, .node-row { grid-template-columns: 30px minmax(200px, .82fr) minmax(140px, .55fr) minmax(440px, 2.2fr); gap: 8px; } }
@media (max-width: 860px) { .prototype-page { padding: 23px 17px 40px; }.prototype-page-heading { align-items: flex-start; }.prototype-project-bar { grid-template-columns: 1fr; grid-template-areas: "heading" "tabs" "actions" "service" "status"; }.project-bar-actions { justify-content: flex-start; }.comparison-head, .node-row { grid-template-columns: 30px minmax(180px, .9fr) minmax(132px, .58fr) minmax(360px, 1.7fr); }.evidence-grid { grid-template-columns: 1fr 1fr; } }
@media (max-width: 560px) { .prototype-page { padding-left: 10px; padding-right: 10px; }.prototype-page-heading { align-items: center; }.prototype-demo-note { max-width: 210px; }.scope-separator, .scope-refresh { display: none; }.scope-control { width: 100%; justify-content: space-between; }.scope-control select { flex: 1; max-width: 220px; }.scope-search-control, .scope-search { width: 100%; }.comparison-head { display: none; }.node-row { grid-template-columns: 28px 1fr; gap: 8px; padding: 12px 16px; }.node-row > :nth-child(3), .node-row > :nth-child(4) { grid-column: 2; }.row-readout { padding: 8px 0 2px; }.history-cell { width: 100%; }.evidence-grid { grid-template-columns: 1fr; }.history-record { grid-template-columns: 1fr 1fr; }.history-record > :last-child { grid-column: 1 / -1; }.prototype-history-modal { padding: 17px; }.history-controls { align-items: flex-start; }.modal-header { flex-direction: column; }.modal-actions { align-self: stretch; justify-content: flex-end; } }
</style>
