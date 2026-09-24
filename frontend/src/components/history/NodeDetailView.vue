<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import * as bridge from '../../api/bridge'
import { fetchMonitorNodeOptions, fetchMonitorStats, queryMonitorSamplesCursor } from '../../api/monitor'
import LatencySamplePlot from '../workbench/LatencySamplePlot.vue'
import type {
  DerivedStats,
  MonitorNodeOption,
  MonitorSample,
  NodeDetailRequest,
  NodeHistoryRevision,
  WorkbenchLatencyBatch,
  WorkbenchLatencyBatchItem,
  WorkbenchLatencyTest,
  WorkbenchPublicServiceAttempt,
  WorkbenchDownloadAttempt,
  WorkbenchSaveRetryRequest,
} from '../../types'

const props = defineProps<{ scope: NodeDetailRequest }>()
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'open-workbench-save-retry', payload: WorkbenchSaveRetryRequest): void
}>()

type SourceView = 'regular_observation' | 'regular' | 'focus' | 'sparse' | 'diagnostic' | 'legacy_unknown' | 'all'
type WindowView = 'context' | '24h' | '7d' | '30d'

const selectedRevision = ref(props.scope.configRevisionKey)
const selectedNodeKey = ref(props.scope.nodeKey)
const selectedNodeName = ref(props.scope.displayName)
const selectedSource = ref<SourceView>(props.scope.origin?.kind === 'monitor_sample' && props.scope.origin.samplingTier
  ? props.scope.origin.samplingTier
  : 'regular_observation')
const selectedWindow = ref<WindowView>(props.scope.origin?.observedAt ? 'context' : '7d')
const options = ref<MonitorNodeOption[]>([])
const optionsError = ref('')
const revisions = ref<NodeHistoryRevision[]>([])
const revisionError = ref('')
const monitorRows = ref<MonitorSample[]>([])
const monitorStats = ref<DerivedStats | null>(null)
const monitorHasMore = ref(false)
const monitorNextCursor = ref('')
const monitorLoading = ref(false)
const monitorMoreLoading = ref(false)
const monitorError = ref('')
const monitorStatsError = ref('')
const monitorLoaded = ref(false)
const workbenchRows = ref<WorkbenchLatencyTest[]>([])
const workbenchHasMore = ref(false)
const workbenchLoading = ref(false)
const workbenchMoreLoading = ref(false)
const workbenchError = ref('')
const workbenchLoaded = ref(false)
const publicServiceRows = ref<WorkbenchPublicServiceAttempt[]>([])
const publicServiceHasMore = ref(false)
const publicServiceLoading = ref(false)
const publicServiceMoreLoading = ref(false)
const publicServiceError = ref('')
const publicServiceLoaded = ref(false)
const downloadRows = ref<WorkbenchDownloadAttempt[]>([])
const downloadHasMore = ref(false)
const downloadLoading = ref(false)
const downloadMoreLoading = ref(false)
const downloadError = ref('')
const downloadLoaded = ref(false)
const selectedPublicServiceID = ref(props.scope.origin?.kind === 'public_service_attempt' ? props.scope.origin.serviceId : '')
const originTest = ref<WorkbenchLatencyTest | null>(null)
const originPublicServiceAttempt = ref<WorkbenchPublicServiceAttempt | null>(null)
const originDownloadAttempt = ref<WorkbenchDownloadAttempt | null>(null)
const originBatch = ref<WorkbenchLatencyBatch | null>(null)
const originBatchItem = ref<WorkbenchLatencyBatchItem | null>(null)
const originError = ref('')
const originRevisionMismatch = ref(false)
let generation = 0

const activeRevision = computed(() => revisions.value.find((item) => item.config_revision_key === selectedRevision.value))
const activeNodeKey = computed(() => activeRevision.value?.node_key || selectedNodeKey.value)
const activeDisplayName = computed(() => {
  const current = currentRevisionOption.value
  return current?.displayName || activeRevision.value?.display_name || selectedNodeName.value || props.scope.nodeKey
})
const activeNodeType = computed(() => selectedRevision.value === props.scope.configRevisionKey
  ? props.scope.nodeType || currentRevisionOption.value?.type || '节点类型未知'
  : currentRevisionOption.value?.type || '节点类型未知')
const currentProfileOptions = computed(() => options.value.filter((item) => item.profileId === props.scope.profileId))
const currentIdentityOptions = computed(() => currentProfileOptions.value.filter((item) => item.nodeIdentityKey === props.scope.nodeIdentityKey))
const currentRevisionOption = computed(() => currentIdentityOptions.value.find((item) => item.configRevisionKey === selectedRevision.value))
const profileName = computed(() => currentProfileOptions.value[0]?.profileName || props.scope.profileName || props.scope.profileId)
const currentStateLabel = computed(() => {
  if (optionsError.value) return '当前状态未知：当前节点列表读取失败'
  if (currentProfileOptions.value.length === 0) return '历史节点 · 当前订阅缓存中不可用'
  if (currentIdentityOptions.value.length === 0) return '历史节点 · 当前配置中没有此稳定身份'
  if (!currentRevisionOption.value) return '历史 revision · 当前节点使用另一配置 revision'
  return '当前节点与 revision 可用'
})
const revisionChoices = computed(() => {
  const found = new Map<string, NodeHistoryRevision>()
  for (const revision of revisions.value) found.set(revision.config_revision_key, revision)
  const current = currentIdentityOptions.value.find((item) => item.configRevisionKey === selectedRevision.value)
  for (const currentOption of currentIdentityOptions.value) {
    if (!found.has(currentOption.configRevisionKey)) found.set(currentOption.configRevisionKey, {
      config_revision_key: currentOption.configRevisionKey,
      node_key: currentOption.nodeKey,
      display_name: currentOption.displayName,
      last_observed_at: '',
    })
  }
  if (!found.has(props.scope.configRevisionKey)) found.set(props.scope.configRevisionKey, {
    config_revision_key: props.scope.configRevisionKey,
    node_key: props.scope.nodeKey,
    display_name: props.scope.displayName,
    last_observed_at: '',
  })
  return Array.from(found.values())
})
const window = computed(() => {
  const untilMs = Date.now()
  if (selectedWindow.value === 'context' && props.scope.origin?.observedAt) {
    const recordAt = Date.parse(props.scope.origin.observedAt)
    if (Number.isFinite(recordAt)) return { sinceMs: recordAt - 12 * 60 * 60 * 1000, untilMs: recordAt + 12 * 60 * 60 * 1000 }
  }
  const duration = selectedWindow.value === '24h' ? 24 * 60 * 60 * 1000 : selectedWindow.value === '30d' ? 30 * 24 * 60 * 60 * 1000 : 7 * 24 * 60 * 60 * 1000
  return { sinceMs: untilMs - duration, untilMs }
})
const windowLabel = computed(() => selectedWindow.value === 'context' ? '记录前后各 12 小时' : selectedWindow.value === '24h' ? '最近 24 小时' : selectedWindow.value === '30d' ? '最近 30 天' : '最近 7 天')
const sourceLabel = computed(() => ({ regular_observation: '常规观测（regular / focus / sparse）', regular: 'regular', focus: 'focus', sparse: 'sparse', diagnostic: 'diagnostic', legacy_unknown: 'legacy_unknown', all: '全部历史来源' } as Record<SourceView, string>)[selectedSource.value])
const statSummaryLabel = computed(() => selectedSource.value === 'regular_observation'
  ? '样本加权合并统计；仅表示所选样本，不是时间可用率或公平排名。'
  : selectedSource.value === 'all' ? '全部来源仅浏览 raw；请选择单一来源查看统计。' : `仅统计 ${sourceLabel.value} 来源。`)

function sourceFilter() {
  const common = {
    profileId: props.scope.profileId,
    nodeIdentityKey: props.scope.nodeIdentityKey,
    configRevisionKey: selectedRevision.value,
    sinceMs: window.value.sinceMs,
    untilMs: window.value.untilMs,
  }
  if (selectedSource.value === 'regular_observation') return { ...common, regularObservationOnly: true }
  if (selectedSource.value === 'all') return common
  return { ...common, samplingTier: selectedSource.value as Exclude<SourceView, 'regular_observation' | 'all'> }
}

function latencyQuery(cursor?: WorkbenchLatencyTest) {
  return {
    profile_id: props.scope.profileId,
    node_key: activeNodeKey.value,
    node_identity_key: props.scope.nodeIdentityKey,
    config_revision_key: selectedRevision.value,
    since: new Date(window.value.sinceMs).toISOString(),
    until: new Date(window.value.untilMs).toISOString(),
    limit: 50,
    ...(cursor ? { before_finished_at: cursor.finished_at, before_attempt_id: cursor.attempt_id } : {}),
  }
}

function publicServiceQuery(cursor?: WorkbenchPublicServiceAttempt) {
  const cursorAt = cursor?.result?.finished_at || cursor?.finished_at || cursor?.started_at || cursor?.requested_at
  return {
    profile_id: props.scope.profileId,
    node_key: activeNodeKey.value,
    node_identity_key: props.scope.nodeIdentityKey,
    config_revision_key: selectedRevision.value,
    ...(selectedPublicServiceID.value ? { service_id: selectedPublicServiceID.value } : {}),
    since: new Date(window.value.sinceMs).toISOString(),
    until: new Date(window.value.untilMs).toISOString(),
    limit: 50,
    ...(cursor && cursorAt ? { before_at: cursorAt, before_attempt_id: cursor.attempt_id } : {}),
  }
}

function downloadQuery(cursor?: WorkbenchDownloadAttempt) {
  const cursorAt = cursor?.result?.finished_at || cursor?.finished_at || cursor?.started_at || cursor?.requested_at
  return {
    profile_id: props.scope.profileId,
    node_key: activeNodeKey.value,
    node_identity_key: props.scope.nodeIdentityKey,
    config_revision_key: selectedRevision.value,
    since: new Date(window.value.sinceMs).toISOString(),
    until: new Date(window.value.untilMs).toISOString(),
    limit: 50,
    ...(cursor && cursorAt ? { before_at: cursorAt, before_attempt_id: cursor.attempt_id } : {}),
  }
}

function matchesActiveRequest(token: number): boolean { return token === generation }

async function load(): Promise<void> {
  const token = ++generation
  monitorRows.value = []
  monitorStats.value = null
  monitorHasMore.value = false
  monitorNextCursor.value = ''
  monitorLoading.value = true
  monitorError.value = ''
  monitorStatsError.value = ''
  monitorLoaded.value = false
  workbenchRows.value = []
  workbenchHasMore.value = false
  workbenchLoading.value = true
  workbenchError.value = ''
  workbenchLoaded.value = false
  publicServiceRows.value = []
  publicServiceHasMore.value = false
  publicServiceLoading.value = true
  publicServiceMoreLoading.value = false
  publicServiceError.value = ''
  publicServiceLoaded.value = false
  downloadRows.value = []
  downloadHasMore.value = false
  downloadLoading.value = true
  downloadMoreLoading.value = false
  downloadError.value = ''
  downloadLoaded.value = false
  monitorMoreLoading.value = false
  workbenchMoreLoading.value = false
  revisionError.value = ''
  optionsError.value = ''

  if (!props.scope.profileId || !props.scope.nodeKey || !props.scope.nodeIdentityKey || !props.scope.configRevisionKey) {
    monitorError.value = 'profile、node_key、稳定 identity 或 revision 不完整；请从既有 legacy 历史入口查看。'
    workbenchError.value = monitorError.value
    publicServiceError.value = monitorError.value
    downloadError.value = monitorError.value
    monitorLoading.value = false
    workbenchLoading.value = false
    publicServiceLoading.value = false
    downloadLoading.value = false
    return
  }

  const filter = sourceFilter()
  const tasks = [
    (async () => {
      try {
        const result = await fetchMonitorNodeOptions()
        if (matchesActiveRequest(token)) options.value = result
      } catch (error) {
        if (matchesActiveRequest(token)) optionsError.value = errorText(error)
      }
    })(),
    (async () => {
      try {
        const result = await bridge.fetchNodeHistoryRevisions(props.scope.profileId, props.scope.nodeIdentityKey)
        if (matchesActiveRequest(token)) revisions.value = Array.isArray(result) ? result : []
      } catch (error) {
        if (matchesActiveRequest(token)) revisionError.value = errorText(error)
      }
    })(),
    (async () => {
      try {
        const page = await queryMonitorSamplesCursor({ ...filter, limit: 50, orderDesc: true })
        if (!matchesActiveRequest(token)) return
        if (page.items.some((sample) => sample.profileId !== props.scope.profileId || sample.nodeIdentityKey !== props.scope.nodeIdentityKey || sample.configRevisionKey !== selectedRevision.value)) {
          monitorError.value = '响应中的 profile / identity / revision 与当前详情不一致。'
        } else {
          monitorRows.value = page.items
          monitorNextCursor.value = page.nextCursor
          monitorHasMore.value = page.hasMore
        }
      } catch (error) {
        if (matchesActiveRequest(token)) monitorError.value = errorText(error)
      } finally {
        if (matchesActiveRequest(token)) {
          monitorLoading.value = false
          monitorLoaded.value = !monitorError.value
        }
      }
    })(),
    (async () => {
      if (selectedSource.value === 'all') return
      try {
        const result = await fetchMonitorStats(filter)
        if (!matchesActiveRequest(token)) return
        if (result) monitorStats.value = result
        else monitorStatsError.value = '没有返回统计结果'
      } catch (error) {
        if (matchesActiveRequest(token)) monitorStatsError.value = errorText(error)
      }
    })(),
    (async () => {
      try {
        const page = await bridge.fetchWorkbenchLatencyHistory(latencyQuery())
        if (!matchesActiveRequest(token)) return
        if (page.tests.some((test) => test.profile_id !== props.scope.profileId || test.node_identity_key !== props.scope.nodeIdentityKey || test.config_revision_key !== selectedRevision.value)) {
          workbenchError.value = '响应中的 profile / identity / revision 与当前详情不一致。'
        } else {
          workbenchRows.value = page.tests
          workbenchHasMore.value = page.has_more
        }
      } catch (error) {
        if (matchesActiveRequest(token)) workbenchError.value = errorText(error)
      } finally {
        if (matchesActiveRequest(token)) {
          workbenchLoading.value = false
          workbenchLoaded.value = !workbenchError.value
        }
      }
    })(),
    (async () => {
      try {
        const page = await bridge.fetchWorkbenchPublicServiceHistory(publicServiceQuery())
        if (!matchesActiveRequest(token)) return
        if (page.attempts.some((attempt) => attempt.profile_id !== props.scope.profileId || attempt.node_key !== activeNodeKey.value || attempt.node_identity_key !== props.scope.nodeIdentityKey || attempt.config_revision_key !== selectedRevision.value || (selectedPublicServiceID.value && attempt.service_id !== selectedPublicServiceID.value))) {
          publicServiceError.value = '响应中的 profile / identity / revision / service 与当前详情不一致。'
        } else {
          publicServiceRows.value = page.attempts
          publicServiceHasMore.value = page.has_more
        }
      } catch (error) {
        if (matchesActiveRequest(token)) publicServiceError.value = errorText(error)
      } finally {
        if (matchesActiveRequest(token)) {
          publicServiceLoading.value = false
          publicServiceLoaded.value = !publicServiceError.value
        }
      }
    })(),
    (async () => {
      try {
        const page = await bridge.fetchWorkbenchDownloadHistory(downloadQuery())
        if (!matchesActiveRequest(token)) return
        if (page.attempts.some((attempt) => attempt.profile_id !== props.scope.profileId || attempt.node_key !== activeNodeKey.value || attempt.node_identity_key !== props.scope.nodeIdentityKey || attempt.config_revision_key !== selectedRevision.value)) {
          downloadError.value = '响应中的 profile / identity / revision 与当前详情不一致。'
        } else {
          downloadRows.value = page.attempts
          downloadHasMore.value = page.has_more
        }
      } catch (error) {
        if (matchesActiveRequest(token)) downloadError.value = errorText(error)
      } finally {
        if (matchesActiveRequest(token)) { downloadLoading.value = false; downloadLoaded.value = !downloadError.value }
      }
    })(),
    loadOrigin(token),
  ]
  await Promise.all(tasks)
}

async function loadOrigin(token: number): Promise<void> {
  originTest.value = null
  originPublicServiceAttempt.value = null
  originDownloadAttempt.value = null
  originBatch.value = null
  originBatchItem.value = null
  originError.value = ''
  originRevisionMismatch.value = false
  const origin = props.scope.origin
  if (!origin || origin.kind === 'monitor_sample') return
  if (selectedRevision.value !== props.scope.configRevisionKey) {
    originRevisionMismatch.value = true
    return
  }
  if (origin.kind === 'public_service_attempt') {
    if (origin.snapshot) originPublicServiceAttempt.value = origin.snapshot
    try {
      const attempt = await bridge.fetchWorkbenchPublicServiceAttempt(origin.attemptId, {
        profile_id: props.scope.profileId, node_key: props.scope.nodeKey, node_identity_key: props.scope.nodeIdentityKey,
        config_revision_key: selectedRevision.value, service_id: origin.serviceId,
      })
      if (!matchesActiveRequest(token)) return
      if (attempt.profile_id !== props.scope.profileId || attempt.node_identity_key !== props.scope.nodeIdentityKey || attempt.config_revision_key !== selectedRevision.value || attempt.service_id !== origin.serviceId) {
        originError.value = '原始服务 attempt 的身份、revision 或 service 不匹配。'
        return
      }
      originPublicServiceAttempt.value = attempt
    } catch (error) {
      if (matchesActiveRequest(token)) originError.value = originPublicServiceAttempt.value
        ? `原始服务 attempt 尚未能从历史库读取，当前显示入口快照：${errorText(error)}`
        : errorText(error)
    }
    return
  }
  if (origin.kind === 'workbench_download_attempt') {
    if (origin.snapshot) originDownloadAttempt.value = origin.snapshot
    try {
      const attempt = await bridge.fetchWorkbenchDownloadAttempt(origin.attemptId, {
        profile_id: props.scope.profileId, node_key: props.scope.nodeKey, node_identity_key: props.scope.nodeIdentityKey,
        config_revision_key: selectedRevision.value,
      })
      if (!matchesActiveRequest(token)) return
      if (attempt.profile_id !== props.scope.profileId || attempt.node_key !== props.scope.nodeKey || attempt.node_identity_key !== props.scope.nodeIdentityKey || attempt.config_revision_key !== selectedRevision.value) {
        originError.value = '原始下载 attempt 的身份或 revision 不匹配。'
        return
      }
      originDownloadAttempt.value = attempt
    } catch (error) {
      if (matchesActiveRequest(token)) originError.value = originDownloadAttempt.value
        ? `原始下载 attempt 尚未能从历史库读取，当前显示入口快照：${errorText(error)}`
        : errorText(error)
    }
    return
  }
  if (origin.snapshot) originTest.value = origin.snapshot
  try {
    if (origin.kind === 'workbench_batch_item') {
      const batch = await bridge.fetchWorkbenchLatencyBatch(origin.batchId)
      if (!matchesActiveRequest(token)) return
      const item = batch.items?.find((candidate) => candidate.item_id === origin.itemId)
      if (!item || item.profile_id !== props.scope.profileId || item.node_identity_key !== props.scope.nodeIdentityKey || item.config_revision_key !== selectedRevision.value) {
        originError.value = '原批次项未找到或身份/revision 不匹配。'
        return
      }
      originBatch.value = batch
      originBatchItem.value = item
      if (origin.attemptId) {
        const test = await bridge.fetchWorkbenchLatencyTest({
          profile_id: props.scope.profileId, node_key: item.node_key, node_identity_key: props.scope.nodeIdentityKey,
          config_revision_key: selectedRevision.value, attempt_id: origin.attemptId,
        })
        if (matchesActiveRequest(token)) originTest.value = test
      }
      return
    }
    const test = await bridge.fetchWorkbenchLatencyTest({
      profile_id: props.scope.profileId, node_key: props.scope.nodeKey, node_identity_key: props.scope.nodeIdentityKey,
      config_revision_key: selectedRevision.value, attempt_id: origin.attemptId,
    })
    if (matchesActiveRequest(token)) originTest.value = test
  } catch (error) {
    if (matchesActiveRequest(token)) originError.value = originTest.value
      ? `原始 attempt 尚未能从历史库读取，当前显示入口快照：${errorText(error)}`
      : errorText(error)
  }
}

async function loadMorePublicService(): Promise<void> {
  if (publicServiceMoreLoading.value || !publicServiceHasMore.value) return
  const token = generation
  const cursor = publicServiceRows.value[publicServiceRows.value.length - 1]
  if (!cursor) return
  publicServiceMoreLoading.value = true
  try {
    const page = await bridge.fetchWorkbenchPublicServiceHistory(publicServiceQuery(cursor))
    if (!matchesActiveRequest(token)) return
    if (page.attempts.some((attempt) => attempt.profile_id !== props.scope.profileId || attempt.node_key !== activeNodeKey.value || attempt.node_identity_key !== props.scope.nodeIdentityKey || attempt.config_revision_key !== selectedRevision.value || (selectedPublicServiceID.value && attempt.service_id !== selectedPublicServiceID.value))) {
      publicServiceError.value = '响应中的 profile / identity / revision / service 与当前详情不一致。'
      return
    }
    const seen = new Set(publicServiceRows.value.map((attempt) => attempt.attempt_id))
    publicServiceRows.value = [...publicServiceRows.value, ...page.attempts.filter((attempt) => !seen.has(attempt.attempt_id))]
    publicServiceHasMore.value = page.has_more
  } catch (error) {
    if (matchesActiveRequest(token)) publicServiceError.value = errorText(error)
  } finally {
    if (matchesActiveRequest(token)) publicServiceMoreLoading.value = false
  }
}

async function loadMoreDownload(): Promise<void> {
  if (downloadMoreLoading.value || !downloadHasMore.value) return
  const token = generation
  const cursor = downloadRows.value[downloadRows.value.length - 1]
  if (!cursor) return
  downloadMoreLoading.value = true
  try {
    const page = await bridge.fetchWorkbenchDownloadHistory(downloadQuery(cursor))
    if (!matchesActiveRequest(token)) return
    if (page.attempts.some((attempt) => attempt.profile_id !== props.scope.profileId || attempt.node_key !== activeNodeKey.value || attempt.node_identity_key !== props.scope.nodeIdentityKey || attempt.config_revision_key !== selectedRevision.value)) {
      downloadError.value = '响应中的 profile / identity / revision 与当前详情不一致。'
      return
    }
    const seen = new Set(downloadRows.value.map((attempt) => attempt.attempt_id))
    downloadRows.value = [...downloadRows.value, ...page.attempts.filter((attempt) => !seen.has(attempt.attempt_id))]
    downloadHasMore.value = page.has_more
  } catch (error) {
    if (matchesActiveRequest(token)) downloadError.value = errorText(error)
  } finally { if (matchesActiveRequest(token)) downloadMoreLoading.value = false }
}

function publicServiceExecutionLabel(state: string): string {
  return ({ queued: '等待执行', running: '执行中', cancelling: '正在取消', completed: '已完成', failed: '未符合判据', cancelled: '已取消', interrupted: '应用退出时中断' } as Record<string, string>)[state] || '状态未知'
}

function publicServiceOutcomeLabel(outcome?: string): string {
  return ({ matched: '符合判据', http_rejected: 'HTTP 状态不符合判据', rate_limited: '目标响应指示请求受限', redirect: '重定向未跟随', timed_out: '超时', cancelled: '用户取消', transport_error: '代理/传输失败，阶段未知', criteria_mismatch: '响应未符合判据' } as Record<string, string>)[outcome || ''] || '无测量结果'
}

async function loadMoreMonitor(): Promise<void> {
  if (monitorMoreLoading.value || !monitorHasMore.value || !monitorNextCursor.value) return
  const token = generation
  monitorMoreLoading.value = true
  try {
    const page = await queryMonitorSamplesCursor({ ...sourceFilter(), limit: 50, orderDesc: true, cursor: monitorNextCursor.value })
    if (!matchesActiveRequest(token)) return
    if (page.items.some((sample) => sample.profileId !== props.scope.profileId || sample.nodeIdentityKey !== props.scope.nodeIdentityKey || sample.configRevisionKey !== selectedRevision.value)) {
      monitorError.value = '响应中的 profile / identity / revision 与当前详情不一致。'
      return
    }
    const seen = new Set(monitorRows.value.map((sample) => sample.sampleId))
    monitorRows.value = [...monitorRows.value, ...page.items.filter((sample) => !seen.has(sample.sampleId))]
    monitorNextCursor.value = page.nextCursor
    monitorHasMore.value = page.hasMore
  } catch (error) {
    if (matchesActiveRequest(token)) monitorError.value = errorText(error)
  } finally {
    if (matchesActiveRequest(token)) monitorMoreLoading.value = false
  }
}

async function loadMoreWorkbench(): Promise<void> {
  if (workbenchMoreLoading.value || !workbenchHasMore.value) return
  const token = generation
  const cursor = workbenchRows.value[workbenchRows.value.length - 1]
  if (!cursor) return
  workbenchMoreLoading.value = true
  try {
    const page = await bridge.fetchWorkbenchLatencyHistory(latencyQuery(cursor))
    if (!matchesActiveRequest(token)) return
    if (page.tests.some((test) => test.profile_id !== props.scope.profileId || test.node_identity_key !== props.scope.nodeIdentityKey || test.config_revision_key !== selectedRevision.value)) {
      workbenchError.value = '响应中的 profile / identity / revision 与当前详情不一致。'
      return
    }
    const seen = new Set(workbenchRows.value.map((test) => test.attempt_id))
    workbenchRows.value = [...workbenchRows.value, ...page.tests.filter((test) => !seen.has(test.attempt_id))]
    workbenchHasMore.value = page.has_more
  } catch (error) {
    if (matchesActiveRequest(token)) workbenchError.value = errorText(error)
  } finally {
    if (matchesActiveRequest(token)) workbenchMoreLoading.value = false
  }
}

function changeRevision(value: string): void {
  if (!value || value === selectedRevision.value) return
  selectedRevision.value = value
  const item = revisionChoices.value.find((revision) => revision.config_revision_key === value)
  if (item) { selectedNodeKey.value = item.node_key; selectedNodeName.value = item.display_name }
  void load()
}

function changeFilters(): void { void load() }
function errorText(error: unknown): string { return error instanceof Error ? error.message : String(error) }
function shortKey(value: string): string { return value.length > 22 ? `${value.slice(0, 10)}…${value.slice(-8)}` : value || '未知' }
function timeText(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') return '未知'
  const date = typeof value === 'number' ? new Date(value) : new Date(value)
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString()
}
function downloadLimitText(bytes: number): string { return bytes >= 1024 * 1024 ? `${(bytes / (1024 * 1024)).toFixed(0)} MiB` : `${bytes.toLocaleString()} 字节` }
function tierText(tier: string, trigger: string): string {
  const label: Record<string, string> = { regular: 'regular', focus: 'focus', sparse: 'sparse', diagnostic: 'diagnostic', legacy_unknown: '来源未知' }
  const triggerLabel: Record<string, string> = { scheduled: '周期', manual: '手动', legacy_unknown: '触发未知' }
  return `${label[tier] || '来源未知'} · ${triggerLabel[trigger] || '触发未知'}`
}
function close(): void { generation++; emit('close') }

function openPublicServiceSaveRetry(attempt: WorkbenchPublicServiceAttempt): void {
  if (attempt.persistence_state !== 'failed' || !attempt.result) return
  emit('open-workbench-save-retry', {
    domain: 'public_service', attempt_id: attempt.attempt_id, profile_id: attempt.profile_id,
    node_key: attempt.node_key, node_identity_key: attempt.node_identity_key,
    config_revision_key: attempt.config_revision_key, service_id: attempt.service_id,
  })
}

function openDownloadSaveRetry(attempt: WorkbenchDownloadAttempt): void {
  if (attempt.persistence_state !== 'failed' || !attempt.result) return
  emit('open-workbench-save-retry', {
    domain: 'download', attempt_id: attempt.attempt_id, profile_id: attempt.profile_id,
    node_key: attempt.node_key, node_identity_key: attempt.node_identity_key,
    config_revision_key: attempt.config_revision_key,
  })
}

onMounted(() => { void load() })
onBeforeUnmount(() => { generation++ })
</script>

<template>
  <div class="node-detail-backdrop" role="presentation" @keydown.esc="close">
    <main class="node-detail" role="dialog" aria-modal="true" aria-labelledby="node-detail-title">
      <header class="node-detail-header">
        <div class="node-detail-title">
          <p class="node-detail-eyebrow">节点历史事实</p>
          <h2 id="node-detail-title">{{ activeDisplayName }}</h2>
          <p>{{ profileName }} · {{ activeNodeType }} · {{ currentStateLabel }}</p>
        </div>
        <button type="button" class="node-detail-close" aria-label="关闭节点详情" @click="close">×</button>
      </header>

      <section class="node-detail-identity">
        <div class="node-detail-controls">
          <label>配置 revision
            <select :value="selectedRevision" aria-label="配置 revision" @change="changeRevision(($event.target as HTMLSelectElement).value)">
              <option v-for="revision in revisionChoices" :key="revision.config_revision_key" :value="revision.config_revision_key">{{ shortKey(revision.config_revision_key) }} · {{ revision.last_observed_at ? timeText(revision.last_observed_at) : '当前/入口版本' }}</option>
            </select>
          </label>
          <label>请求窗口
            <select v-model="selectedWindow" aria-label="请求窗口" @change="changeFilters">
              <option v-if="props.scope.origin?.observedAt" value="context">记录所在范围</option><option value="24h">最近 24 小时</option><option value="7d">最近 7 天</option><option value="30d">最近 30 天</option>
            </select>
          </label>
        </div>
        <div class="node-detail-scope-line"><span>节点身份 {{ shortKey(props.scope.nodeIdentityKey) }}</span><span>node_key {{ shortKey(activeNodeKey) }}</span><span>revision {{ shortKey(selectedRevision) }}</span></div>
        <details><summary>查看完整稳定身份与配置 revision</summary><dl><dt>profile_id</dt><dd>{{ props.scope.profileId }}</dd><dt>node_identity_key</dt><dd>{{ props.scope.nodeIdentityKey }}</dd><dt>node_key</dt><dd>{{ activeNodeKey }}</dd><dt>config_revision_key</dt><dd>{{ selectedRevision }}</dd></dl></details>
        <p v-if="revisionError" class="node-detail-error">revision 列表读取失败：{{ revisionError }} · 仍可查看当前入口所指版本。</p>
        <p v-if="optionsError" class="node-detail-warning">当前节点可用性未知：{{ optionsError }}。历史数据仍按已保存身份查询。</p>
      </section>

      <section v-if="props.scope.origin && props.scope.origin.kind !== 'monitor_sample'" class="node-detail-origin">
        <h3>{{ props.scope.origin.kind === 'public_service_attempt' ? '原始公共服务检测' : props.scope.origin.kind === 'workbench_download_attempt' ? '原始下载测量' : '原始 Workbench 来源' }}</h3>
        <p v-if="props.scope.origin.kind === 'public_service_attempt'">服务 {{ props.scope.origin.serviceId }} · Attempt {{ props.scope.origin.attemptId }}</p>
        <p v-if="props.scope.origin.kind === 'workbench_download_attempt'">固定目标下载 · Attempt {{ props.scope.origin.attemptId }}</p>
        <p v-if="props.scope.origin.kind === 'workbench_batch_item'">Batch {{ props.scope.origin.batchId }} · Item {{ props.scope.origin.itemId }} · Attempt {{ props.scope.origin.attemptId || '无 attempt' }}</p>
        <p v-else-if="props.scope.origin.kind === 'workbench_attempt'">Attempt {{ props.scope.origin.attemptId }}</p>
        <p v-if="originRevisionMismatch" class="node-detail-warning">当前查看的是另一 revision；入口 attempt/batch 仍属于打开详情时的 revision。</p>
        <p v-else-if="originError" class="node-detail-error">来源读取失败：{{ originError }}</p>
        <template v-else-if="originBatchItem">
          <p>执行：{{ originBatchItem.execution_state }} · 保存：{{ originBatchItem.persistence_state }}<template v-if="originBatchItem.error_message"> · {{ originBatchItem.error_message }}</template><template v-if="originBatchItem.persistence_error"> · {{ originBatchItem.persistence_error }}</template></p>
        </template>
        <template v-if="originTest"><p>测法 {{ originTest.method || '未知' }} v{{ originTest.method_version || '未知' }} · target {{ originTest.target || '未知' }} · 单位 {{ originTest.unit || '未知' }} · attempt {{ originTest.attempt_id }}</p><p>{{ originTest.success_samples }} 成功 / {{ originTest.failure_samples }} 失败 · {{ originTest.latency_ms }} ms · jitter {{ originTest.jitter_ms }} ms · {{ originTest.persistence_state }}</p></template>
        <template v-if="originPublicServiceAttempt"><p>{{ originPublicServiceAttempt.rule.method }} {{ originPublicServiceAttempt.rule.target_url }} · 规则 v{{ originPublicServiceAttempt.rule.rule_version }} · {{ originPublicServiceAttempt.rule.success_criterion }}</p><p>执行 {{ publicServiceExecutionLabel(originPublicServiceAttempt.execution_state) }} · 保存 {{ originPublicServiceAttempt.persistence_state }}<template v-if="originPublicServiceAttempt.persistence_error"> · {{ originPublicServiceAttempt.persistence_error }}</template></p><p v-if="originPublicServiceAttempt.result">{{ publicServiceOutcomeLabel(originPublicServiceAttempt.result.outcome) }} · HTTP {{ originPublicServiceAttempt.result.http_status ?? '无响应' }} · {{ originPublicServiceAttempt.result.duration_ms }} ms · 已读取 {{ originPublicServiceAttempt.result.bytes_read }} 字节<template v-if="originPublicServiceAttempt.result.failure_phase"> · {{ originPublicServiceAttempt.result.failure_phase }}</template><template v-if="originPublicServiceAttempt.result.error_message"> · {{ originPublicServiceAttempt.result.error_message }}</template></p></template>
        <template v-if="originPublicServiceAttempt"><p>{{ originPublicServiceAttempt.rule.method }} {{ originPublicServiceAttempt.rule.target_url }} · 规则 v{{ originPublicServiceAttempt.rule.rule_version }} · {{ originPublicServiceAttempt.rule.success_criterion }}</p><p>执行 {{ publicServiceExecutionLabel(originPublicServiceAttempt.execution_state) }} · 保存 {{ originPublicServiceAttempt.persistence_state }}<template v-if="originPublicServiceAttempt.persistence_error"> · {{ originPublicServiceAttempt.persistence_error }}</template></p><p v-if="originPublicServiceAttempt.result">{{ publicServiceOutcomeLabel(originPublicServiceAttempt.result.outcome) }} · HTTP {{ originPublicServiceAttempt.result.http_status ?? '无响应' }} · {{ originPublicServiceAttempt.result.duration_ms }} ms · 已读取 {{ originPublicServiceAttempt.result.bytes_read }} 字节<template v-if="originPublicServiceAttempt.result.failure_phase"> · {{ originPublicServiceAttempt.result.failure_phase }}</template><template v-if="originPublicServiceAttempt.result.error_message"> · {{ originPublicServiceAttempt.result.error_message }}</template></p><button v-if="originPublicServiceAttempt.persistence_state === 'failed' && originPublicServiceAttempt.result" type="button" class="node-detail-more" @click="openPublicServiceSaveRetry(originPublicServiceAttempt)">返回 Workbench 重试保存（不重新检测）</button></template>
        <template v-if="originDownloadAttempt"><p>{{ originDownloadAttempt.rule.method }} {{ originDownloadAttempt.rule.target_url }} · 规则 v{{ originDownloadAttempt.rule.rule_version }} · 上限 {{ downloadLimitText(originDownloadAttempt.rule.maximum_bytes) }} / {{ (originDownloadAttempt.rule.maximum_duration_ns / 1e9).toFixed(0) }} 秒</p><p>执行 {{ originDownloadAttempt.execution_state }} · 保存 {{ originDownloadAttempt.persistence_state }}<template v-if="originDownloadAttempt.persistence_error"> · {{ originDownloadAttempt.persistence_error }}</template></p><p v-if="originDownloadAttempt.result">{{ originDownloadAttempt.result.outcome }} · 已读取 {{ originDownloadAttempt.result.bytes_read }} 字节 · {{ (originDownloadAttempt.result.duration_ns / 1e9).toFixed(2) }} 秒<template v-if="originDownloadAttempt.result.failure_phase"> · {{ originDownloadAttempt.result.failure_phase }}</template><template v-if="originDownloadAttempt.result.error_message"> · {{ originDownloadAttempt.result.error_message }}</template></p><button v-if="originDownloadAttempt.persistence_state === 'failed' && originDownloadAttempt.result" type="button" class="node-detail-more" @click="openDownloadSaveRetry(originDownloadAttempt)">返回 Workbench 重试保存（不重新下载）</button></template>
        <span v-if="!originBatchItem && !originTest && !originPublicServiceAttempt && !originDownloadAttempt && !originError && !originRevisionMismatch">正在读取原始 attempt…</span>
      </section>

      <section class="node-detail-section">
        <div class="node-detail-section-heading"><div><h3>Monitor 观察</h3><p>{{ sourceLabel }} · {{ windowLabel }} · {{ new Date(window.sinceMs).toLocaleString() }} – {{ new Date(window.untilMs).toLocaleString() }}</p></div><label>来源
          <select v-model="selectedSource" aria-label="Monitor 来源" @change="changeFilters">
            <option value="regular_observation">常规观测</option><option value="regular">regular</option><option value="focus">focus</option><option value="sparse">sparse</option><option value="diagnostic">diagnostic</option><option value="legacy_unknown">legacy_unknown</option><option value="all">全部来源 raw</option>
          </select>
        </label></div>
        <p class="node-detail-note">{{ statSummaryLabel }}</p>
        <p class="node-detail-note">这里仅列已保存的 Monitor 原始样本。没有样本表示该窗口未观测；不会据此推断节点失败，也不把未采集、资源跳过或运行中断填入样本分母。</p>
        <div v-if="monitorStatsError" class="node-detail-error">Monitor 统计读取失败：{{ monitorStatsError }}</div>
        <div v-else-if="selectedSource !== 'all' && monitorStats" class="node-detail-stats">
          <strong>{{ monitorStats.sampleCount }} 个样本</strong><span>{{ monitorStats.successCount }} 成功 / {{ monitorStats.failureCount }} 探测失败</span><span>纳入来源 {{ monitorStats.includedSamplingTiers.join('、') || '无' }}</span><span>成功率 {{ (monitorStats.successRate * 100).toFixed(1) }}%</span><span>P50 {{ monitorStats.latencyP50Ms ?? '—' }} ms · P95 {{ monitorStats.latencyP95Ms ?? '—' }} ms</span><span>请求窗口 {{ windowLabel }}</span><span>实际样本 {{ monitorStats.sampleCount > 0 ? `${timeText(monitorStats.firstSampleAtMs)} – ${timeText(monitorStats.lastSampleAtMs)}` : '无样本' }}</span>
        </div>
        <div v-else-if="selectedSource === 'all'" class="node-detail-note">全部历史浏览模式不合并不同来源的统计。</div>
        <div v-if="monitorError" class="node-detail-error">Monitor raw 读取失败：{{ monitorError }}</div>
        <p v-else-if="monitorLoaded && monitorRows.length === 0 && !monitorHasMore" class="node-detail-empty">所选身份、revision、来源和窗口内没有 raw 样本。</p>
        <p v-if="monitorLoaded && monitorHasMore" class="node-detail-warning">已加载 {{ monitorRows.length }} 条 raw；此窗口还有更多结果，当前页面为部分结果。</p>
        <div v-if="monitorLoading" class="node-detail-empty">正在读取 Monitor raw…</div>
        <div v-if="monitorRows.length" class="node-detail-table-wrap"><table><thead><tr><th>时间 / 来源</th><th>探测</th><th>结果</th><th>原始定位</th></tr></thead><tbody>
          <tr v-for="sample in monitorRows" :key="sample.sampleId" :class="{ 'node-detail-highlight': props.scope.origin?.kind === 'monitor_sample' && props.scope.origin.sampleId === sample.sampleId }"><td>{{ timeText(sample.timestampIso) }}<small>{{ tierText(sample.samplingTier, sample.triggerType) }} · 策略 v{{ sample.samplingStrategyVersion }}</small></td><td>{{ sample.probeType || '未知测法' }}<small class="break-target">{{ sample.target || '目标未知' }}</small></td><td :class="sample.success ? 'node-success' : 'node-failure'">{{ sample.success ? (sample.latencyMs > 0 ? `${sample.latencyMs.toFixed(1)} ms` : '成功；无延迟值') : `探测失败 · ${sample.errorClass || '原因未知'}` }}<small v-if="sample.errorDetail">{{ sample.errorDetail }}</small><small v-if="sample.ttfbMs > 0">TTFB {{ sample.ttfbMs.toFixed(1) }} ms</small></td><td><details><summary>{{ shortKey(sample.sampleId) }}</summary><code>sample {{ sample.sampleId }}<br>run {{ sample.runId }}<br>profile {{ sample.profileId }}<br>identity {{ sample.nodeIdentityKey }}<br>node {{ sample.nodeKey }}<br>revision {{ sample.configRevisionKey }}</code></details></td></tr>
        </tbody></table></div>
        <button v-if="monitorHasMore" type="button" class="node-detail-more" :disabled="monitorMoreLoading" @click="loadMoreMonitor">{{ monitorMoreLoading ? '正在加载…' : '加载更多 Monitor raw' }}</button>
      </section>

      <section class="node-detail-section">
        <div class="node-detail-section-heading"><div><h3>Workbench 主动延迟测试</h3><p>与 Monitor 统计、推荐证据和预算隔离。</p></div></div>
        <div v-if="workbenchError" class="node-detail-error">Workbench 延迟历史读取失败：{{ workbenchError }}</div>
        <div v-else-if="workbenchLoaded && workbenchRows.length === 0 && !workbenchHasMore" class="node-detail-empty">此身份、revision 和请求窗口内没有已保存 attempt。</div>
        <p v-if="workbenchLoaded && workbenchHasMore" class="node-detail-warning">已加载 {{ workbenchRows.length }} 次 attempt；此窗口还有更多记录，当前结果未完整。</p>
        <div v-if="workbenchLoading" class="node-detail-empty">正在读取 Workbench 历史…</div>
        <div v-for="test in workbenchRows" :key="test.attempt_id" class="node-detail-attempt">
          <div><strong>{{ timeText(test.finished_at) }} · {{ test.status === 'completed' ? '成功' : test.status === 'partial_failed' ? '部分失败' : '失败' }}</strong><span>保存 {{ test.persistence_state }}<template v-if="test.persistence_error"> · {{ test.persistence_error }}</template></span><small>来源 {{ test.source || '未知' }} · 测法 {{ test.method || '未知' }} v{{ test.method_version || '未知' }} · target {{ test.target || '未知' }} · 单位 {{ test.unit || '未知' }}</small></div>
          <div class="node-detail-attempt-metrics"><b>{{ test.success_samples }} 成功 / {{ test.failure_samples }} 失败样本</b><span>延迟 {{ test.latency_ms > 0 ? `${test.latency_ms} ms` : '未知' }} · jitter {{ test.jitter_ms > 0 ? `${test.jitter_ms} ms` : '未知' }}</span><span>attempt {{ test.attempt_id }}</span></div>
          <details><summary>查看样本图与 {{ test.samples.length }} 条原始样本</summary><LatencySamplePlot :samples="test.samples" :window-since="test.started_at" :window-until="test.finished_at" :hovered-index="null" :pinned-index="null" :height="100" /><ol><li v-for="sample in test.samples" :key="`${test.attempt_id}-${sample.seq}`">{{ timeText(sample.timestamp) }} · {{ sample.success ? `${sample.latency_ms} ms` : `失败 · ${sample.error || '原因未知'}` }} · #{{ sample.seq }}</li></ol></details>
        </div>
        <button v-if="workbenchHasMore" type="button" class="node-detail-more" :disabled="workbenchMoreLoading" @click="loadMoreWorkbench">{{ workbenchMoreLoading ? '正在加载…' : '加载更多 Workbench attempt' }}</button>
      </section>

      <section class="node-detail-section">
        <div class="node-detail-section-heading"><div><h3>Workbench 公共服务检测</h3><p>独立 attempt 历史；与延迟、Monitor 统计和推荐证据分开。读取详情不会发起请求。</p></div><label>服务
          <select v-model="selectedPublicServiceID" aria-label="公共服务历史筛选" @change="changeFilters"><option value="">全部已接入服务</option><option value="cloudflare_204">Cloudflare 204 连通性</option><option value="google_204">Google 204 连通性</option><option value="github_api_root">GitHub 公共 API 根端点</option></select>
        </label></div>
        <p class="node-detail-note">请求窗口 {{ windowLabel }} · {{ new Date(window.sinceMs).toLocaleString() }} – {{ new Date(window.untilMs).toLocaleString() }}；结果只表示对应固定目标是否符合保存的判据，不表示完整业务可用性。</p>
        <div v-if="publicServiceError" class="node-detail-error">公共服务历史读取失败：{{ publicServiceError }}</div>
        <p v-else-if="publicServiceLoaded && publicServiceRows.length === 0 && !publicServiceHasMore" class="node-detail-empty">此 profile、稳定身份、revision、服务与请求窗口内没有检测记录。</p>
        <p v-if="publicServiceLoaded && publicServiceHasMore" class="node-detail-warning">已加载 {{ publicServiceRows.length }} 条记录；当前请求窗口仍有更多历史。</p>
        <div v-if="publicServiceLoading" class="node-detail-empty">正在读取公共服务历史…</div>
        <div v-for="attempt in publicServiceRows" :key="attempt.attempt_id" class="node-detail-attempt" :class="{ 'node-detail-highlight': props.scope.origin?.kind === 'public_service_attempt' && props.scope.origin.attemptId === attempt.attempt_id }">
          <div><strong>{{ attempt.rule.name }} · {{ timeText(attempt.result?.finished_at || attempt.finished_at || attempt.started_at || attempt.requested_at) }}</strong><span>执行 {{ publicServiceExecutionLabel(attempt.execution_state) }} · 保存 {{ attempt.persistence_state }}<template v-if="attempt.persistence_error"> · {{ attempt.persistence_error }}</template></span><small>来源 {{ attempt.source }} · {{ attempt.rule.method }} {{ attempt.rule.target_url }} · 规则 v{{ attempt.rule.rule_version }}</small><small>{{ attempt.rule.success_criterion }} · {{ attempt.rule.redirect_policy === 'do_not_follow' ? '不跟随重定向' : attempt.rule.redirect_policy }}</small></div>
          <div class="node-detail-attempt-metrics"><b>{{ publicServiceOutcomeLabel(attempt.result?.outcome) }}</b><span>HTTP {{ attempt.result?.http_status ?? '无响应' }} · {{ attempt.result?.duration_ms ?? '—' }} ms · 已读取 {{ attempt.result?.bytes_read ?? 0 }} 字节</span><span v-if="attempt.result?.failure_phase">失败位置 {{ attempt.result.failure_phase }}<template v-if="attempt.result.error_message"> · {{ attempt.result.error_message }}</template></span><span>attempt {{ attempt.attempt_id }}</span><button v-if="attempt.persistence_state === 'failed' && attempt.result" type="button" class="node-detail-more" @click="openPublicServiceSaveRetry(attempt)">返回 Workbench 重试保存（不重新检测）</button></div>
        </div>
        <button v-if="publicServiceHasMore" type="button" class="node-detail-more" :disabled="publicServiceMoreLoading" @click="loadMorePublicService">{{ publicServiceMoreLoading ? '正在加载…' : '加载更多公共服务检测' }}</button>
      </section>

      <section class="node-detail-section">
        <div class="node-detail-section-heading"><div><h3>Workbench 下载测量</h3><p>独立的手动响应体测量；与延迟、公共服务、Monitor 和推荐证据分开。</p></div></div>
        <p class="node-detail-note">请求窗口 {{ windowLabel }} · {{ new Date(window.sinceMs).toLocaleString() }} – {{ new Date(window.untilMs).toLocaleString() }}。读取字节数不代表系统总流量或完整文件下载。</p>
        <div v-if="downloadError" class="node-detail-error">下载历史读取失败：{{ downloadError }}</div>
        <p v-else-if="downloadLoaded && downloadRows.length === 0 && !downloadHasMore" class="node-detail-empty">此 profile、稳定身份、revision 与请求窗口内没有下载测量记录。</p>
        <p v-if="downloadLoaded && downloadHasMore" class="node-detail-warning">已加载 {{ downloadRows.length }} 次下载测量；当前窗口仍有更多记录。</p>
        <div v-if="downloadLoading" class="node-detail-empty">正在读取下载历史…</div>
        <div v-for="attempt in downloadRows" :key="attempt.attempt_id" class="node-detail-attempt" :class="{ 'node-detail-highlight': props.scope.origin?.kind === 'workbench_download_attempt' && props.scope.origin.attemptId === attempt.attempt_id }">
          <div><strong>{{ timeText(attempt.result?.finished_at || attempt.finished_at || attempt.started_at || attempt.requested_at) }} · {{ attempt.execution_state }}</strong><span>保存 {{ attempt.persistence_state }}<template v-if="attempt.persistence_error"> · {{ attempt.persistence_error }}</template></span><small>来源 {{ attempt.source }} · {{ attempt.rule.method }} {{ attempt.rule.target_url }} · 规则 v{{ attempt.rule.rule_version }}</small><small>读取上限 {{ downloadLimitText(attempt.rule.maximum_bytes) }} · 时长上限 {{ (attempt.rule.maximum_duration_ns / 1e9).toFixed(0) }} 秒</small></div>
          <div class="node-detail-attempt-metrics"><b>{{ attempt.result?.outcome || (attempt.execution_state === 'interrupted' ? '应用退出时中断；未自动重测' : '尚无测量结果') }}</b><span v-if="attempt.result">{{ attempt.result.bytes_read.toLocaleString() }} 字节 · {{ (attempt.result.duration_ns / 1e9).toFixed(2) }} 秒<template v-if="attempt.result.http_status"> · HTTP {{ attempt.result.http_status }}</template></span><span v-if="attempt.result?.failure_phase">结束位置 {{ attempt.result.failure_phase }}<template v-if="attempt.result.error_message"> · {{ attempt.result.error_message }}</template></span><span>attempt {{ attempt.attempt_id }}</span><button v-if="attempt.persistence_state === 'failed' && attempt.result" type="button" class="node-detail-more" @click="openDownloadSaveRetry(attempt)">返回 Workbench 重试保存（不重新下载）</button></div>
          <details v-if="attempt.result?.samples.length"><summary>实际过程样本 {{ attempt.result.samples.length }} 条</summary><ol><li v-for="sample in attempt.result.samples" :key="sample.cumulative_bytes">{{ (sample.elapsed_ns / 1e9).toFixed(2) }} 秒 · +{{ sample.delta_bytes.toLocaleString() }} 字节 · 累计 {{ sample.cumulative_bytes.toLocaleString() }} 字节<template v-if="sample.speed_mbps !== undefined"> · {{ sample.speed_mbps.toFixed(2) }} Mbps</template></li></ol></details>
        </div>
        <button v-if="downloadHasMore" type="button" class="node-detail-more" :disabled="downloadMoreLoading" @click="loadMoreDownload">{{ downloadMoreLoading ? '正在加载…' : '加载更多下载测量' }}</button>
      </section>
    </main>
  </div>
</template>

<style scoped>
.node-detail-backdrop { position: fixed; inset: 0; z-index: 70; display: flex; justify-content: center; align-items: stretch; padding: 22px; background: rgba(20, 31, 40, .42); }
.node-detail { width: min(1320px, 100%); max-height: 100%; overflow: auto; padding: 22px 26px 32px; border-radius: 13px; background: var(--card-bg, white); color: var(--text-main, #1f2933); box-shadow: 0 24px 70px rgba(20, 35, 45, .28); }
.node-detail-header, .node-detail-section-heading { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.node-detail-title h2 { margin: 2px 0 5px; font-size: 23px; }
.node-detail-title p:last-child, .node-detail-section-heading p { margin: 0; color: var(--text-secondary, #5a6872); font-size: 12px; }
.node-detail-eyebrow { margin: 0 0 5px; color: var(--primary, #256b78); font-size: 11px; font-weight: 750; letter-spacing: .08em; }
.node-detail-close { width: 34px; height: 34px; border: 1px solid var(--border, #d5dce0); border-radius: 50%; background: white; color: #53616b; font-size: 23px; line-height: 1; }
.node-detail-identity, .node-detail-origin { margin-top: 14px; padding: 12px 14px; border: 1px solid var(--border, #d5dce0); border-radius: 8px; background: var(--card-subtle, #f8fafb); }
.node-detail-controls { display: flex; flex-wrap: wrap; gap: 12px 24px; }
.node-detail-controls label, .node-detail-section-heading label { display: grid; gap: 5px; color: var(--text-secondary, #5a6872); font-size: 11px; font-weight: 700; }
.node-detail-controls select, .node-detail-section-heading select { min-width: 185px; padding: 6px 8px; border: 1px solid var(--border, #d5dce0); border-radius: 6px; background: white; color: var(--text-main, #1f2933); font-size: 12px; }
.node-detail-scope-line { display: flex; flex-wrap: wrap; gap: 6px 18px; margin: 10px 0; color: var(--text-secondary, #5a6872); font-size: 11px; }
.node-detail-identity details summary, .node-detail-table-wrap details summary, .node-detail-attempt details summary { cursor: pointer; color: var(--primary, #256b78); font-size: 11px; }
.node-detail-identity dl { display: grid; grid-template-columns: 150px 1fr; gap: 6px 12px; margin: 10px 0 0; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 10px; }
.node-detail-identity dt { color: var(--text-muted, #77858f); }.node-detail-identity dd { margin: 0; overflow-wrap: anywhere; }
.node-detail-error, .node-detail-warning, .node-detail-note { margin: 8px 0 0; color: var(--text-secondary, #5a6872); font-size: 11px; line-height: 1.5; }
.node-detail-error { color: #a32f36; }.node-detail-warning { color: #8a5b07; }
.node-detail-origin h3, .node-detail-section h3 { margin: 0 0 5px; font-size: 15px; }.node-detail-origin p { margin: 4px 0; color: var(--text-secondary, #5a6872); font-size: 11px; overflow-wrap: anywhere; }
.node-detail-section { margin-top: 17px; padding: 15px 15px 18px; border: 1px solid var(--border, #d5dce0); border-radius: 8px; }
.node-detail-stats { display: flex; flex-wrap: wrap; gap: 8px 18px; margin: 10px 0; padding: 10px; background: var(--card-subtle, #f8fafb); color: var(--text-secondary, #5a6872); font-size: 11px; }.node-detail-stats strong { color: var(--text-main, #1f2933); }
.node-detail-empty { padding: 14px 4px; color: var(--text-secondary, #5a6872); font-size: 12px; }.node-detail-table-wrap { margin-top: 10px; overflow: auto; }.node-detail-table-wrap table { width: 100%; min-width: 840px; border-collapse: collapse; font-size: 11px; }.node-detail-table-wrap th, .node-detail-table-wrap td { padding: 8px; border-bottom: 1px solid var(--border, #d5dce0); text-align: left; vertical-align: top; }.node-detail-table-wrap th { color: var(--text-secondary, #5a6872); }.node-detail-table-wrap small { display: block; margin-top: 4px; color: var(--text-muted, #77858f); line-height: 1.4; }.node-detail-table-wrap code { display: block; margin-top: 7px; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 9px; }.break-target { max-width: 300px; overflow-wrap: anywhere; }
.node-success { color: #167447; font-weight: 700; }.node-failure { color: #a32f36; font-weight: 700; }.node-detail-highlight { background: #fff6d9; }
.node-detail-more { margin-top: 10px; padding: 7px 11px; border: 1px solid var(--border, #d5dce0); border-radius: 6px; background: white; color: var(--primary, #256b78); font-size: 11px; font-weight: 700; }.node-detail-more:disabled { opacity: .6; }
.node-detail-attempt { display: grid; grid-template-columns: minmax(280px, 1fr) minmax(230px, .7fr) minmax(180px, .6fr); gap: 13px; align-items: start; padding: 11px 0; border-top: 1px solid var(--border, #d5dce0); font-size: 11px; }.node-detail-attempt > div { display: flex; flex-direction: column; gap: 5px; }.node-detail-attempt strong { font-size: 12px; }.node-detail-attempt span, .node-detail-attempt small { color: var(--text-secondary, #5a6872); overflow-wrap: anywhere; }.node-detail-attempt ol { margin: 7px 0 0; padding-left: 18px; color: var(--text-secondary, #5a6872); }
@media (max-width: 760px) { .node-detail-backdrop { padding: 5px; }.node-detail { padding: 17px 13px 25px; border-radius: 8px; }.node-detail-attempt { grid-template-columns: 1fr; }.node-detail-identity dl { grid-template-columns: 100px 1fr; }.node-detail-section-heading { flex-direction: column; } }
</style>
