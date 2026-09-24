<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as api from '../../api/bridge'
import type { MonitorNodeOption, NodeDetailRequest, WorkbenchDownloadAttempt, WorkbenchDownloadHistoryQuery, WorkbenchSaveRetryRequest } from '../../types'

const props = defineProps<{ node: MonitorNodeOption | null; saveRetryRequest?: WorkbenchSaveRetryRequest | null }>()
const emit = defineEmits<{
  (event: 'open-node-detail', payload: NodeDetailRequest): void
  (event: 'save-retry-request-resolved', attemptID: string): void
}>()

const attempt = ref<WorkbenchDownloadAttempt | null>(null)
const failedAttempts = ref<WorkbenchDownloadAttempt[]>([])
const error = ref('')
const historyError = ref('')
const reading = ref(false)
const historyLoading = ref(false)
const historyHasMore = ref(false)
const historyCursor = ref<WorkbenchDownloadAttempt | null>(null)
const timeoutSeconds = ref(10)
const maximumMiB = ref(20)
const progressBytes = ref(0)
const progressSamples = ref<{ elapsed_ns: number; interval_ns: number; delta_bytes: number; cumulative_bytes: number; speed_mbps?: number }[]>([])
let requestToken = 0
let historyToken = 0
let pollTimer: ReturnType<typeof setTimeout> | null = null
let unsubscribeEvents: (() => void) | null = null

const active = computed(() => !!attempt.value && ['queued', 'running', 'cancelling'].includes(attempt.value.execution_state))
const saving = computed(() => attempt.value?.persistence_state === 'saving')
const saveFailed = computed(() => attempt.value?.persistence_state === 'failed' && !!attempt.value?.result)
const capBytes = computed(() => maximumMiB.value * 1024 * 1024)
const canStart = computed(() => !!props.node && !reading.value && !active.value && !saving.value && !saveFailed.value)

function messageFor(value: unknown): string { return value instanceof Error ? value.message : String(value || '未知错误') }
function newRequestID(): string { return typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `download-${Date.now()}-${Math.random().toString(16).slice(2)}` }
function queryFor(value: WorkbenchDownloadAttempt): WorkbenchDownloadHistoryQuery {
  return { profile_id: value.profile_id, node_key: value.node_key, node_identity_key: value.node_identity_key, config_revision_key: value.config_revision_key }
}
function matchesNode(value: WorkbenchDownloadAttempt, node: MonitorNodeOption): boolean {
  return value.profile_id === node.profileId && value.node_key === node.nodeKey && value.node_identity_key === node.nodeIdentityKey && value.config_revision_key === node.configRevisionKey
}
function sameNodeScope(left: MonitorNodeOption | null, right: MonitorNodeOption): boolean {
  return !!left && left.profileId === right.profileId && left.nodeKey === right.nodeKey &&
    left.nodeIdentityKey === right.nodeIdentityKey && left.configRevisionKey === right.configRevisionKey
}
function sameRetryScope(value: WorkbenchDownloadAttempt, request: WorkbenchSaveRetryRequest): boolean {
  return value.attempt_id === request.attempt_id && value.profile_id === request.profile_id && value.node_key === request.node_key &&
    value.node_identity_key === request.node_identity_key && value.config_revision_key === request.config_revision_key
}
function isActive(value: WorkbenchDownloadAttempt): boolean { return ['queued', 'running', 'cancelling'].includes(value.execution_state) || value.persistence_state === 'saving' }

function historyQuery(node: MonitorNodeOption, cursor?: WorkbenchDownloadAttempt): WorkbenchDownloadHistoryQuery {
  const query: WorkbenchDownloadHistoryQuery = {
    profile_id: node.profileId, node_key: node.nodeKey, node_identity_key: node.nodeIdentityKey,
    config_revision_key: node.configRevisionKey, limit: 25,
  }
  const beforeAt = cursor?.result?.finished_at || cursor?.finished_at || cursor?.started_at || cursor?.requested_at
  if (cursor && beforeAt) { query.before_at = beforeAt; query.before_attempt_id = cursor.attempt_id }
  return query
}

function schedulePoll(attemptID: string, token: number, delay = 650): void {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = setTimeout(() => { void refresh(attemptID, token) }, delay)
}

async function restoreActive(node: MonitorNodeOption): Promise<void> {
  const token = ++requestToken
  try {
    const page = await api.fetchWorkbenchDownloadHistory({ profile_id: node.profileId, node_key: node.nodeKey, node_identity_key: node.nodeIdentityKey, config_revision_key: node.configRevisionKey, limit: 10 })
    if (token !== requestToken || !page.attempts.length || attempt.value) return
    const latest = page.attempts.find(isActive)
    if (!latest || !matchesNode(latest, node)) return
    attempt.value = latest
    progressBytes.value = latest.result?.bytes_read || 0
    progressSamples.value = latest.result?.samples || []
    schedulePoll(latest.attempt_id, token)
  } catch {
    // This read only restores a still-active request. History errors are shown on Node Detail.
  }
}

async function loadFailedHistory(node: MonitorNodeOption, append = false): Promise<void> {
  const token = append ? historyToken : ++historyToken
  const cursor = append ? historyCursor.value || undefined : undefined
  if (append && !cursor) return
  historyLoading.value = true
  historyError.value = ''
  try {
    const page = await api.fetchWorkbenchDownloadHistory(historyQuery(node, cursor))
    if (token !== historyToken || !sameNodeScope(props.node, node)) return
    if (page.attempts.some((item) => !matchesNode(item, node))) {
      historyError.value = '待保存历史响应的 profile、identity 或 revision 与当前节点不一致。'
      return
    }
    const retryable = page.attempts.filter((item) => item.persistence_state === 'failed' && !!item.result)
    if (append) {
      const seen = new Set(failedAttempts.value.map((item) => item.attempt_id))
      failedAttempts.value = [...failedAttempts.value, ...retryable.filter((item) => !seen.has(item.attempt_id))]
    } else failedAttempts.value = retryable
    historyCursor.value = page.attempts[page.attempts.length - 1] || historyCursor.value
    historyHasMore.value = page.has_more
  } catch (cause) {
    if (token === historyToken) historyError.value = messageFor(cause)
  } finally {
    if (token === historyToken) historyLoading.value = false
  }
}

async function selectFailedAttempt(candidate: WorkbenchDownloadAttempt): Promise<void> {
  const token = ++requestToken
  if (pollTimer) clearTimeout(pollTimer)
  reading.value = true
  error.value = ''
  try {
    const latest = await api.fetchWorkbenchDownloadAttempt(candidate.attempt_id, queryFor(candidate))
    if (token !== requestToken) return
    if (!matchesAttempt(latest, candidate)) { error.value = '待保存结果的身份范围不一致。'; return }
    attempt.value = latest
    progressBytes.value = latest.result?.bytes_read || 0
    progressSamples.value = latest.result?.samples || []
  } catch (cause) {
    if (token === requestToken) error.value = messageFor(cause)
  } finally {
    if (token === requestToken) reading.value = false
  }
}

async function loadSaveRetryRequest(request: WorkbenchSaveRetryRequest): Promise<void> {
  if (request.domain !== 'download') return
  const token = ++requestToken
  if (pollTimer) clearTimeout(pollTimer)
  reading.value = true
  error.value = ''
  try {
    const query: WorkbenchDownloadHistoryQuery = {
      profile_id: request.profile_id, node_key: request.node_key,
      node_identity_key: request.node_identity_key, config_revision_key: request.config_revision_key,
    }
    const latest = await api.fetchWorkbenchDownloadAttempt(request.attempt_id, query)
    if (token !== requestToken) return
    if (!sameRetryScope(latest, request)) { error.value = '待保存结果的身份范围与历史记录不一致。'; return }
    attempt.value = latest
    progressBytes.value = latest.result?.bytes_read || 0
    progressSamples.value = latest.result?.samples || []
    emit('save-retry-request-resolved', request.attempt_id)
  } catch (cause) {
    if (token === requestToken) error.value = `读取待保存结果失败：${messageFor(cause)}`
  } finally {
    if (token === requestToken) reading.value = false
  }
}

function retryLoadSaveRetryRequest(): void {
  const request = props.saveRetryRequest
  if (request?.domain === 'download') void loadSaveRetryRequest(request)
}

async function start(): Promise<void> {
  const node = props.node
  if (!node || !canStart.value) return
  const token = ++requestToken
  if (pollTimer) clearTimeout(pollTimer)
  error.value = ''
  reading.value = true
  progressBytes.value = 0
  progressSamples.value = []
  try {
    const created = await api.startWorkbenchDownloadTest({
      request_id: newRequestID(), profile_id: node.profileId, node_key: node.nodeKey,
      node_identity_key: node.nodeIdentityKey, config_revision_key: node.configRevisionKey,
      maximum_bytes: capBytes.value, timeout_seconds: timeoutSeconds.value,
    })
    if (token !== requestToken) return
    if (!matchesNode(created, node)) { error.value = '响应所属 profile、identity 或 revision 与本次选择不一致。'; return }
    attempt.value = created
    if (created.result) {
      progressBytes.value = created.result.bytes_read
      progressSamples.value = created.result.samples
    }
    schedulePoll(created.attempt_id, token, 250)
  } catch (cause) {
    if (token === requestToken) error.value = messageFor(cause)
  } finally {
    if (token === requestToken) reading.value = false
  }
}

async function refresh(attemptID = attempt.value?.attempt_id || '', token = requestToken): Promise<void> {
  const current = attempt.value
  if (!current || !attemptID || current.attempt_id !== attemptID || token !== requestToken) return
  reading.value = true
  try {
    const latest = await api.fetchWorkbenchDownloadAttempt(attemptID, queryFor(current))
    if (token !== requestToken || attempt.value?.attempt_id !== attemptID) return
    if (!matchesAttempt(latest, current)) { error.value = '下载检测响应的身份范围不一致。'; return }
    attempt.value = latest
    progressBytes.value = latest.result?.bytes_read ?? progressBytes.value
    progressSamples.value = latest.result?.samples ?? progressSamples.value
    error.value = ''
    if (isActive(latest)) schedulePoll(attemptID, token)
  } catch (cause) {
    if (token === requestToken) { error.value = `状态读取失败：${messageFor(cause)}`; schedulePoll(attemptID, token, 1400) }
  } finally {
    if (token === requestToken) reading.value = false
  }
}

async function cancel(): Promise<void> {
  const current = attempt.value
  if (!current || !active.value || current.execution_state === 'cancelling') return
  error.value = ''
  try {
    attempt.value = await api.cancelWorkbenchDownloadTest(current.attempt_id, queryFor(current))
    schedulePoll(current.attempt_id, requestToken, 200)
  } catch (cause) { error.value = messageFor(cause) }
}

async function retrySave(): Promise<void> {
  const current = attempt.value
  if (!current || !saveFailed.value) return
  reading.value = true
  error.value = ''
  try {
    attempt.value = await api.retrySaveWorkbenchDownloadTest(current.attempt_id, queryFor(current))
    if (attempt.value.persistence_state === 'saved') failedAttempts.value = failedAttempts.value.filter((item) => item.attempt_id !== current.attempt_id)
  }
  catch (cause) { error.value = messageFor(cause) }
  finally { reading.value = false }
}

function matchesAttempt(value: WorkbenchDownloadAttempt, expected: WorkbenchDownloadAttempt): boolean {
  return value.attempt_id === expected.attempt_id && value.profile_id === expected.profile_id && value.node_key === expected.node_key &&
    value.node_identity_key === expected.node_identity_key && value.config_revision_key === expected.config_revision_key
}

function handleEvent(type: string, payload: any): void {
  const current = attempt.value
  if (!current || !payload || payload.attempt_id !== current.attempt_id) return
  if (type === 'workbench_download_progress') {
    if (Number.isFinite(payload.bytes_read)) progressBytes.value = payload.bytes_read
    if (payload.sample && Number.isFinite(payload.sample.cumulative_bytes)) {
      const existing = progressSamples.value
      if (!existing.some((sample) => sample.cumulative_bytes === payload.sample.cumulative_bytes)) progressSamples.value = [...existing, payload.sample]
    }
    return
  }
  if (type === 'workbench_download_attempt_updated') {
    const updated = payload as WorkbenchDownloadAttempt
    if (matchesAttempt(updated, current)) {
      attempt.value = updated
      if (updated.result) { progressBytes.value = updated.result.bytes_read; progressSamples.value = updated.result.samples }
      if (isActive(updated)) schedulePoll(updated.attempt_id, requestToken)
    }
  }
}

function executionLabel(state: string): string {
  return ({ queued: '等待执行', running: '下载中', cancelling: '正在取消；请求尚未确认停止', completed: '测量完成', byte_limit: '达到响应体上限', time_limit: '达到时长上限', user_cancelled: '用户取消', interrupted: '应用退出时中断', connection_failed: '连接失败', http_rejected: '目标拒绝请求', redirect: '目标重定向，未跟随', transfer_interrupted: '响应体传输中断' } as Record<string, string>)[state] || '状态未知'
}
function outcomeLabel(outcome: string): string { return executionLabel(outcome) }
function persistenceLabel(state: string): string { return ({ not_started: '尚未保存', saving: '结果保存中', saved: '已保存', failed: '保存失败；可重试保存', not_applicable: '无可保存结果' } as Record<string, string>)[state] || '保存状态未知' }
function bytesText(bytes: number): string { return `${(bytes / (1024 * 1024)).toFixed(2)} MiB (${bytes.toLocaleString()} 字节)` }
function limitText(bytes: number): string { return bytes >= 1024 * 1024 ? `${(bytes / (1024 * 1024)).toFixed(0)} MiB` : `${bytes.toLocaleString()} 字节` }
function timeText(value?: string): string { if (!value) return '时间未知'; const d = new Date(value); return Number.isNaN(d.getTime()) ? '时间未知' : d.toLocaleString() }
function openDetail(): void {
  const current = attempt.value
  if (!current) return
  emit('open-node-detail', {
    profileId: current.profile_id, nodeKey: current.node_key, nodeIdentityKey: current.node_identity_key,
    configRevisionKey: current.config_revision_key, displayName: current.display_name || current.node_key, nodeType: current.node_type,
    origin: { kind: 'workbench_download_attempt', attemptId: current.attempt_id, observedAt: current.result?.finished_at, snapshot: current },
  })
}

watch(() => props.saveRetryRequest, (request) => {
  if (request?.domain === 'download') void loadSaveRetryRequest(request)
}, { immediate: true })
watch(() => props.node && `${props.node.profileId}\u0000${props.node.nodeKey}\u0000${props.node.nodeIdentityKey}\u0000${props.node.configRevisionKey}`, () => {
  const node = props.node
  historyToken++
  failedAttempts.value = []
  historyHasMore.value = false
  historyCursor.value = null
  historyError.value = ''
  if (!node) return
  void loadFailedHistory(node)
  if (!attempt.value && props.saveRetryRequest?.domain !== 'download') {
    reading.value = false
    void restoreActive(node)
  }
}, { immediate: true })
onMounted(() => { unsubscribeEvents = api.subscribeEvents(handleEvent) })
onBeforeUnmount(() => { requestToken++; if (pollTimer) clearTimeout(pollTimer); unsubscribeEvents?.(); unsubscribeEvents = null })
</script>

<template>
  <section class="download-panel" aria-labelledby="download-panel-title">
    <header><div><h2 id="download-panel-title">单节点下载测量</h2><p>只读取固定目标的响应体；字节数不等于系统总流量，也不代表完整文件已下载。</p></div></header>
    <div class="download-controls">
      <label>节点<span class="download-node" :class="{ muted: !node }">{{ node ? `${node.displayName} · ${node.profileName} · ${node.type || '节点'}` : '请只勾选一个节点' }}</span></label>
      <label>读取上限<select v-model.number="maximumMiB" :disabled="active || saving"><option :value="20">20 MiB</option><option :value="50">50 MiB</option><option :value="100">100 MiB</option></select></label>
      <label>时长上限<select v-model.number="timeoutSeconds" :disabled="active || saving"><option :value="5">5 秒</option><option :value="10">10 秒</option><option :value="20">20 秒</option><option :value="30">30 秒</option></select></label>
      <button type="button" class="download-primary" :disabled="!canStart" @click="start">{{ reading ? '正在准备…' : active ? '下载中…' : '执行一次下载测量' }}</button>
      <button v-if="active" type="button" class="download-button" :disabled="attempt?.execution_state === 'cancelling'" @click="cancel">{{ attempt?.execution_state === 'cancelling' ? '正在取消…' : '取消下载' }}</button>
    </div>
    <p class="download-rule">GET {{ attempt?.rule.target_url || 'https://speed.cloudflare.com/__down?bytes=（读取上限+1）' }} · 规则 v{{ attempt?.rule.rule_version || 1 }} · 最多 {{ attempt ? limitText(attempt.rule.maximum_bytes) : `${maximumMiB} MiB` }} / {{ attempt ? (attempt.rule.maximum_duration_ns / 1e9).toFixed(0) : timeoutSeconds }} 秒 · 不使用 Monitor 预算，不进入 Monitor 推荐证据。</p>
    <p class="download-note">采样仅由实际收到的响应体字节和单调时钟计算；达到上限或用户取消都不表示节点故障。无有效样本时不显示 0 Mbps。</p>
    <div v-if="failedAttempts.length || historyLoading || historyError" class="download-retry-history" aria-label="待重试保存的下载历史">
      <strong>可重试保存的历史结果</strong>
      <p v-if="historyError" class="download-error">读取待保存历史失败：{{ historyError }}</p>
      <p v-else-if="historyLoading && failedAttempts.length === 0" class="download-note">正在读取此节点 revision 的待保存结果…</p>
      <button v-for="candidate in failedAttempts" :key="candidate.attempt_id" type="button" class="download-button" :aria-pressed="attempt?.attempt_id === candidate.attempt_id" :disabled="reading" @click="selectFailedAttempt(candidate)">
        选择 {{ timeText(candidate.result?.finished_at || candidate.finished_at || candidate.started_at || candidate.requested_at) }} · attempt {{ candidate.attempt_id }}
      </button>
      <button v-if="historyHasMore" type="button" class="download-button" :disabled="historyLoading" @click="props.node && loadFailedHistory(props.node, true)">{{ historyLoading ? '正在加载…' : '加载更多待保存结果' }}</button>
    </div>
    <button v-if="saveRetryRequest?.domain === 'download' && error" type="button" class="download-button" :disabled="reading" @click="retryLoadSaveRetryRequest">重新读取指定 attempt</button>
    <p v-if="error" class="download-error">{{ error }}</p>
    <div v-if="attempt" class="download-result" aria-live="polite">
      <div class="download-result-heading"><strong>{{ executionLabel(attempt.execution_state) }}</strong><span>{{ persistenceLabel(attempt.persistence_state) }}</span></div>
      <p>结果属于 {{ attempt.display_name || attempt.node_key }} · {{ attempt.profile_id }} · revision {{ attempt.config_revision_key }} · attempt {{ attempt.attempt_id }}</p>
      <p>{{ attempt.result ? outcomeLabel(attempt.result.outcome) : executionLabel(attempt.execution_state) }} · {{ attempt.result ? bytesText(attempt.result.bytes_read) : bytesText(progressBytes) }}<template v-if="attempt.result"> · 用时 {{ (attempt.result.duration_ns / 1e9).toFixed(2) }} 秒<template v-if="attempt.result.http_status"> · HTTP {{ attempt.result.http_status }}</template></template></p>
      <p v-if="attempt.result?.failure_phase">结束位置：{{ attempt.result.failure_phase }}<template v-if="attempt.result.error_message"> · {{ attempt.result.error_message }}</template></p>
      <p v-if="attempt.persistence_error" class="download-error">{{ attempt.persistence_error }}</p>
      <div class="download-actions"><button type="button" class="download-button" :disabled="reading" @click="refresh()">{{ reading ? '读取中…' : '重新读取状态' }}</button><button v-if="saveFailed" type="button" class="download-button" :disabled="reading" @click="retrySave">重试保存（不重新下载）</button><button type="button" class="download-button" @click="openDetail">查看节点详情与历史</button></div>
      <details v-if="progressSamples.length"><summary>实际过程样本 {{ progressSamples.length }} 条</summary><ol><li v-for="sample in progressSamples" :key="sample.cumulative_bytes">{{ (sample.elapsed_ns / 1e9).toFixed(2) }} 秒 · +{{ bytesText(sample.delta_bytes) }} · 累计 {{ bytesText(sample.cumulative_bytes) }}<template v-if="sample.speed_mbps !== undefined"> · {{ sample.speed_mbps.toFixed(2) }} Mbps</template></li></ol></details>
    </div>
  </section>
</template>

<style scoped>
.download-panel { margin-bottom: 14px; padding: 15px 18px; border: 1px solid var(--border, #d6dde1); border-radius: 10px; background: var(--card-bg, #fff); color: var(--text-main, #1f2933); }
.download-panel h2 { margin: 0 0 4px; font-size: 15px; }.download-panel header p, .download-rule, .download-note, .download-result p { margin: 4px 0; color: var(--text-secondary, #596873); font-size: 11px; line-height: 1.5; overflow-wrap: anywhere; }
.download-controls { display: flex; flex-wrap: wrap; align-items: end; gap: 9px 12px; margin: 12px 0 5px; }.download-controls label { display: grid; gap: 5px; min-width: 150px; color: var(--text-secondary, #596873); font-size: 10px; font-weight: 700; }.download-controls select { min-height: 31px; padding: 5px 8px; border: 1px solid var(--border, #d6dde1); border-radius: 5px; background: white; color: var(--text-main, #1f2933); font-size: 11px; }.download-node { display: inline-flex; align-items: center; min-height: 31px; padding: 0 8px; border: 1px solid var(--border, #d6dde1); border-radius: 5px; color: var(--text-main, #1f2933); font-size: 11px; font-weight: 500; }.download-node.muted { color: var(--text-muted, #78868f); }
.download-primary, .download-button { min-height: 31px; padding: 6px 10px; border: 1px solid var(--border, #d6dde1); border-radius: 5px; background: white; color: var(--primary, #256b78); font-size: 11px; font-weight: 700; }.download-primary { border-color: var(--primary, #256b78); background: var(--primary, #256b78); color: white; }.download-primary:disabled, .download-button:disabled { cursor: not-allowed; opacity: .55; }.download-note { color: var(--text-muted, #75838c); }.download-error { margin: 7px 0; color: #a32f36; font-size: 11px; overflow-wrap: anywhere; }.download-retry-history { display: flex; flex-wrap: wrap; align-items: center; gap: 7px; margin-top: 9px; padding: 8px 10px; border: 1px solid var(--border, #d6dde1); border-radius: 7px; background: var(--card-subtle, #f8fafb); color: var(--text-secondary, #596873); font-size: 11px; }.download-retry-history strong, .download-retry-history p { width: 100%; margin: 0; }.download-retry-history strong { color: var(--text-main, #1f2933); }.download-result { margin-top: 11px; padding: 10px 12px; border: 1px solid var(--border, #d6dde1); border-radius: 7px; background: var(--card-subtle, #f8fafb); }.download-result-heading, .download-actions { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; color: var(--text-main, #1f2933); font-size: 12px; }.download-actions { justify-content: flex-start; margin-top: 8px; }.download-result details { margin-top: 8px; color: var(--text-secondary, #596873); font-size: 11px; }.download-result details summary { cursor: pointer; color: var(--primary, #256b78); }.download-result ol { display: grid; gap: 5px; margin: 7px 0; padding-left: 20px; }
</style>
