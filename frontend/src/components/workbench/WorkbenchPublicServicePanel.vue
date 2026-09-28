<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as api from '../../api/bridge'
import { useWorkbenchStore } from '../../stores/workbench'
import { serviceDetailLabel, serviceOutcomeLabel } from '../../utils/serviceOutcome'
function openCredentials(): void { useWorkbenchStore().isSettingsModalOpen = true }
import UiSelect, { type UiSelectOption } from '../common/UiSelect.vue'
import IntraTestSamplePlot, { type ServiceSampleItem } from './IntraTestSamplePlot.vue'
import type { MonitorNodeOption, NodeDetailRequest, WorkbenchPublicServiceAttempt, WorkbenchPublicServiceHistoryQuery, WorkbenchPublicServiceRule, WorkbenchSaveRetryRequest } from '../../types'

const publicServiceTimeoutOptions: UiSelectOption[] = [
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
  { value: 20, label: '20 秒' },
  { value: 30, label: '30 秒' },
]

const props = defineProps<{
  compact?: boolean
  node: MonitorNodeOption | null
  serviceId?: string
  selectedNodes?: MonitorNodeOption[]
  saveRetryRequest?: WorkbenchSaveRetryRequest | null
}>()
const emit = defineEmits<{
  (event: 'update:serviceId', value: string): void
  (event: 'open-node-detail', payload: NodeDetailRequest): void
  (event: 'save-retry-request-resolved', attemptID: string): void
  (event: 'start-batch', params: { serviceId: string; repeatCount: number; timeoutSeconds: number }): void
}>()

const catalog = ref<WorkbenchPublicServiceRule[]>([])
const catalogError = ref('')
const selectedServiceID = ref(props.serviceId || 'cloudflare_204')
const timeoutSeconds = ref(10)
const repeatCountOptions: UiSelectOption[] = [
  { value: 1, label: '1 次' },
  { value: 2, label: '2 次' },
  { value: 3, label: '3 次' },
  { value: 5, label: '5 次' },
]
const repeatCount = ref(1)
const currentRound = ref(1)
const attempt = ref<WorkbenchPublicServiceAttempt | null>(null)
const retryableAttempts = ref<WorkbenchPublicServiceAttempt[]>([])
const retryableHasMore = ref(false)
const retryableLoading = ref(false)
const retryableError = ref('')
const error = ref('')
const refreshing = ref(false)
let pollTimer: ReturnType<typeof setTimeout> | null = null
let requestToken = 0
let retryableRequestToken = 0
let saveRetryLoadToken = 0
const retryableCursor = ref<WorkbenchPublicServiceAttempt | null>(null)
const selectedCategory = ref('推荐')

type ServiceOption = { value: string; label: string; disabled?: boolean }
const serviceCategories = computed(() => ['推荐', '全部', ...Array.from(new Set(catalog.value.map(rule => rule.category || '其他')))])
const recommendedRules = computed(() => {
  const recommended = catalog.value.filter(rule => rule.batch_default)
  return recommended.length ? recommended : catalog.value
})
const visibleRules = computed(() => selectedCategory.value === '全部'
  ? catalog.value
  : selectedCategory.value === '推荐'
    ? recommendedRules.value
    : catalog.value.filter(rule => (rule.category || '其他') === selectedCategory.value))
const serviceOptions = computed<ServiceOption[]>(() => {
  return visibleRules.value.map((rule) => ({ value: rule.service_id, label: rule.name }))
})
const selectedRule = computed(() => catalog.value.find((rule) => rule.service_id === selectedServiceID.value) || null)
watch(() => props.serviceId, value => {
  if (!value) return
  selectedServiceID.value = value
  const rule = catalog.value.find(item => item.service_id === value)
  if (rule && !visibleRules.value.some(item => item.service_id === value)) selectedCategory.value = rule.category || '全部'
})
watch(selectedServiceID, value => emit('update:serviceId', value))
const active = computed(() => !!attempt.value && ['queued', 'running', 'cancelling'].includes(attempt.value.execution_state))
const saving = computed(() => attempt.value?.persistence_state === 'saving')
const saveFailed = computed(() => attempt.value?.persistence_state === 'failed' && !!attempt.value?.result)
const effectiveNodes = computed(() => (props.selectedNodes && props.selectedNodes.length > 0) ? props.selectedNodes : (props.node ? [props.node] : []))
const isMultiNode = computed(() => effectiveNodes.value.length > 1)
const canStart = computed(() => effectiveNodes.value.length > 0 && !!selectedRule.value && !active.value && !saving.value && !saveFailed.value)

const serviceDisplaySamples = computed<ServiceSampleItem[]>(() => {
  const list: ServiceSampleItem[] = []
  if (attempt.value?.result) {
    list.push({
      name: attempt.value.rule.name,
      duration_ms: attempt.value.result.duration_ms,
      http_status: attempt.value.result.http_status,
      outcome: attempt.value.result.outcome,
      timestamp: attempt.value.result.finished_at,
    })
  }
  for (const r of retryableAttempts.value) {
    if (r.result && r.attempt_id !== attempt.value?.attempt_id) {
      list.push({
        name: r.rule.name,
        duration_ms: r.result.duration_ms,
        http_status: r.result.http_status,
        outcome: r.result.outcome,
        timestamp: r.result.finished_at,
      })
    }
  }
  return list
})

onMounted(async () => {
  try {
    catalog.value = await api.listWorkbenchPublicServiceCatalog()
    if (!catalog.value.some((rule) => rule.service_id === selectedServiceID.value)) selectedServiceID.value = catalog.value[0]?.service_id || ''
    const activeRule = catalog.value.find(rule => rule.service_id === selectedServiceID.value)
    if (activeRule && !visibleRules.value.some(rule => rule.service_id === selectedServiceID.value)) selectedCategory.value = activeRule.category || '全部'
  } catch (cause) {
    catalogError.value = messageFor(cause)
  }
})

function chooseCategory(value: string): void {
  selectedCategory.value = value
  if (!visibleRules.value.some(rule => rule.service_id === selectedServiceID.value)) {
    selectedServiceID.value = visibleRules.value[0]?.service_id || ''
  }
}

watch(() => [props.node?.profileId, props.node?.nodeKey, props.node?.nodeIdentityKey, props.node?.configRevisionKey].join('\u0000'), () => {
  void loadRetryableAttempts()
}, { immediate: true })

watch(() => props.saveRetryRequest, (request) => {
  if (request?.domain === 'public_service') void loadSaveRetryRequest(request)
}, { immediate: true })

onBeforeUnmount(() => {
  requestToken += 1
  if (pollTimer) clearTimeout(pollTimer)
})

function messageFor(value: unknown): string {
  return value instanceof Error ? value.message : String(value || '未知错误')
}

function attemptQuery(value: WorkbenchPublicServiceAttempt): WorkbenchPublicServiceHistoryQuery {
  return {
    profile_id: value.profile_id,
    node_key: value.node_key,
    node_identity_key: value.node_identity_key,
    config_revision_key: value.config_revision_key,
    service_id: value.service_id,
  }
}

function retryRequestQuery(request: WorkbenchSaveRetryRequest): WorkbenchPublicServiceHistoryQuery {
  return {
    profile_id: request.profile_id,
    node_key: request.node_key,
    node_identity_key: request.node_identity_key,
    config_revision_key: request.config_revision_key,
    service_id: request.service_id,
  }
}

function historyCursor(attempt: WorkbenchPublicServiceAttempt): { before_at: string; before_attempt_id: string } {
  return {
    before_at: attempt.result?.finished_at || attempt.finished_at || attempt.started_at || attempt.requested_at,
    before_attempt_id: attempt.attempt_id,
  }
}

async function loadRetryableAttempts(append = false): Promise<void> {
  const node = props.node
  if (!node) {
    retryableRequestToken++
    retryableAttempts.value = []
    retryableHasMore.value = false
    retryableCursor.value = null
    return
  }
  const token = ++retryableRequestToken
  retryableLoading.value = true
  retryableError.value = ''
  if (!append) {
    retryableAttempts.value = []
    retryableHasMore.value = false
    retryableCursor.value = null
  }
  const query: WorkbenchPublicServiceHistoryQuery = {
    profile_id: node.profileId,
    node_key: node.nodeKey,
    node_identity_key: node.nodeIdentityKey,
    config_revision_key: node.configRevisionKey,
    limit: 25,
    ...(append && retryableCursor.value ? historyCursor(retryableCursor.value) : {}),
  }
  try {
    const page = await api.fetchWorkbenchPublicServiceHistory(query)
    if (token !== retryableRequestToken) return
    const retryable = page.attempts.filter((item) => item.persistence_state === 'failed' && !!item.result)
    const existingIDs = new Set(retryableAttempts.value.map((item) => item.attempt_id))
    retryableAttempts.value = append
      ? [...retryableAttempts.value, ...retryable.filter((item) => !existingIDs.has(item.attempt_id))]
      : retryable
    retryableHasMore.value = page.has_more
    retryableCursor.value = page.attempts[page.attempts.length - 1] || retryableCursor.value
  } catch (cause) {
    if (token === retryableRequestToken) retryableError.value = `读取可重试保存历史失败：${messageFor(cause)}`
  } finally {
    if (token === retryableRequestToken) retryableLoading.value = false
  }
}

function selectRetryableAttempt(value: WorkbenchPublicServiceAttempt): void {
  if (value.persistence_state !== 'failed' || !value.result) return
  requestToken++
  if (pollTimer) clearTimeout(pollTimer)
  selectedServiceID.value = value.service_id
  attempt.value = value
  error.value = ''
}

async function loadSaveRetryRequest(request: WorkbenchSaveRetryRequest): Promise<void> {
  const token = ++saveRetryLoadToken
  requestToken++
  if (pollTimer) clearTimeout(pollTimer)
  refreshing.value = true
  error.value = ''
  try {
    const loaded = await api.fetchWorkbenchPublicServiceAttempt(request.attempt_id, retryRequestQuery(request))
    if (token !== saveRetryLoadToken) return
    if (loaded.attempt_id !== request.attempt_id || loaded.profile_id !== request.profile_id || loaded.node_key !== request.node_key ||
      loaded.node_identity_key !== request.node_identity_key || loaded.config_revision_key !== request.config_revision_key ||
      (request.service_id && loaded.service_id !== request.service_id)) {
      error.value = '待处理 attempt 的身份范围不一致；未执行保存或检测。'
      return
    }
    selectedServiceID.value = loaded.service_id
    attempt.value = loaded
    emit('save-retry-request-resolved', loaded.attempt_id)
  } catch (cause) {
    if (token === saveRetryLoadToken) error.value = `读取待处理 attempt 失败：${messageFor(cause)}`
  } finally {
    if (token === saveRetryLoadToken) refreshing.value = false
  }
}

function retryLoadSaveRetryRequest(): void {
  const request = props.saveRetryRequest
  if (request?.domain === 'public_service') void loadSaveRetryRequest(request)
}

function newRequestID(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `service-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

async function start(): Promise<void> {
  const rule = selectedRule.value
  if (!rule || !canStart.value) return
  if (isMultiNode.value) {
    emit('start-batch', {
      serviceId: rule.service_id,
      repeatCount: repeatCount.value,
      timeoutSeconds: timeoutSeconds.value,
    })
    return
  }
  const node = props.node || effectiveNodes.value[0]
  if (!node) return
  currentRound.value = 1
  void runRound(1)
}

async function runRound(round: number): Promise<void> {
  const node = props.node || effectiveNodes.value[0]
  const rule = selectedRule.value
  if (!node || !rule) return
  currentRound.value = round
  const token = ++requestToken
  if (pollTimer) clearTimeout(pollTimer)
  error.value = ''
  refreshing.value = true
  try {
    const created = await api.startWorkbenchPublicServiceTest({
      request_id: newRequestID(),
      profile_id: node.profileId,
      node_key: node.nodeKey,
      node_identity_key: node.nodeIdentityKey,
      config_revision_key: node.configRevisionKey,
      service_id: rule.service_id,
      timeout_seconds: timeoutSeconds.value,
    })
    if (token !== requestToken) return
    attempt.value = created
    if (!matchesNode(created, node) || created.service_id !== rule.service_id) {
      error.value = '响应所属身份与本次选择不一致；结果未用于当前选择。'
      return
    }
    schedulePoll(created.attempt_id, token, 250)
  } catch (cause) {
    if (token === requestToken) error.value = messageFor(cause)
  } finally {
    if (token === requestToken) refreshing.value = false
  }
}

function schedulePoll(attemptID: string, token: number, delay = 700): void {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = setTimeout(() => { void refresh(attemptID, token, true) }, delay)
}

async function refresh(attemptID = attempt.value?.attempt_id || '', token = requestToken, keepPolling = false): Promise<void> {
  const current = attempt.value
  if (!current || !attemptID || current.attempt_id !== attemptID) return
  refreshing.value = true
  try {
    const latest = await api.fetchWorkbenchPublicServiceAttempt(attemptID, attemptQuery(current))
    if (token !== requestToken || attempt.value?.attempt_id !== attemptID) return
    if (!matchesAttempt(latest, current)) {
      error.value = '服务检测响应的身份范围不一致。'
      return
    }
    attempt.value = latest
    error.value = ''
    if (keepPolling && (['queued', 'running', 'cancelling'].includes(latest.execution_state) || latest.persistence_state === 'saving')) {
      schedulePoll(attemptID, token, 700)
    } else if (currentRound.value < repeatCount.value && latest.execution_state === 'completed' && token === requestToken) {
      setTimeout(() => {
        if (currentRound.value < repeatCount.value && token === requestToken && !refreshing.value) {
          void runRound(currentRound.value + 1)
        }
      }, 500)
    }
  } catch (cause) {
    if (token === requestToken) {
      error.value = `状态读取失败：${messageFor(cause)}`
      if (keepPolling) schedulePoll(attemptID, token, 1500)
    }
  } finally {
    if (token === requestToken) refreshing.value = false
  }
}

async function cancel(): Promise<void> {
  const current = attempt.value
  if (!current || !active.value || current.execution_state === 'cancelling') return
  error.value = ''
  try {
    attempt.value = await api.cancelWorkbenchPublicServiceTest(current.attempt_id, attemptQuery(current))
    schedulePoll(current.attempt_id, requestToken, 250)
  } catch (cause) {
    error.value = messageFor(cause)
  }
}

async function retrySave(): Promise<void> {
  const current = attempt.value
  if (!current || !saveFailed.value) return
  error.value = ''
  refreshing.value = true
  try {
    attempt.value = await api.retrySaveWorkbenchPublicServiceTest(current.attempt_id, attemptQuery(current))
  } catch (cause) {
    error.value = messageFor(cause)
  } finally {
    refreshing.value = false
  }
}

function matchesNode(value: WorkbenchPublicServiceAttempt, node: MonitorNodeOption): boolean {
  return value.profile_id === node.profileId && value.node_key === node.nodeKey &&
    value.node_identity_key === node.nodeIdentityKey && value.config_revision_key === node.configRevisionKey
}

function matchesAttempt(value: WorkbenchPublicServiceAttempt, expected: WorkbenchPublicServiceAttempt): boolean {
  return value.attempt_id === expected.attempt_id && value.profile_id === expected.profile_id &&
    value.node_key === expected.node_key && value.node_identity_key === expected.node_identity_key &&
    value.config_revision_key === expected.config_revision_key && value.service_id === expected.service_id
}

function executionLabel(value?: string): string {
  return ({ queued: '等待执行', running: '检测中', cancelling: '正在取消；请求尚未确认停止', completed: '请求已完成', failed: '未满足判据', cancelled: '已取消', interrupted: '应用关闭时中断' } as Record<string, string>)[value || ''] || '状态未知'
}

function outcomeLabel(value?: string): string { return serviceOutcomeLabel(value) }
function detailLabel(value: string): string { return serviceDetailLabel(value) }

function persistenceLabel(value?: string, hasResult = true): string {
	if (value === 'failed' && !hasResult) return '结果未能暂存；需重新检测'
	return ({ not_started: '尚未保存', saving: '结果保存中', saved: '已保存', failed: '保存失败；可重试保存', not_applicable: '没有可保存的测量结果' } as Record<string, string>)[value || ''] || '保存状态未知'
}

function openDetail(): void {
  const current = attempt.value
  if (!current) return
  emit('open-node-detail', {
    profileId: current.profile_id,
    nodeKey: current.node_key,
    nodeIdentityKey: current.node_identity_key,
    configRevisionKey: current.config_revision_key,
    displayName: current.display_name || current.node_key,
    nodeType: current.node_type,
    origin: { kind: 'public_service_attempt', attemptId: current.attempt_id, serviceId: current.service_id, observedAt: current.result?.finished_at, snapshot: current },
  })
}
</script>

<template>
  <section v-if="!compact || attempt || error || catalogError || retryableError || retryableAttempts.length || retryableHasMore" class="public-service-panel" :class="{ compact }" aria-label="服务执行与保存状态">
    <header v-if="!compact" class="public-service-heading"><div><h2 id="public-service-title">公共服务即时检测</h2><p>按所选节点依次检测；不使用 Monitor 预算，也不进入 Monitor 推荐证据。</p></div></header>
    <button v-if="compact && active" type="button" class="public-service-button" :disabled="attempt?.execution_state === 'cancelling'" @click="cancel">取消检测</button>
    <div v-if="!compact" class="public-service-controls">
      <label>节点
        <span class="public-service-node" :class="{ muted: effectiveNodes.length === 0 }">
          {{ isMultiNode ? `已勾选 ${effectiveNodes.length} 个节点（将排队依次检测）` : (node ? `${node.displayName} · ${node.profileName} · ${node.type || '节点'}` : '请勾选至少一个节点') }}
        </span>
      </label>
      <label class="service-choice">公共服务<UiSelect v-model="selectedServiceID" aria-label="选择公共服务" :options="serviceOptions" :disabled="active || saving" /></label>
      <label>检测次数<UiSelect v-model="repeatCount" variant="compact" aria-label="检测次数" :options="repeatCountOptions" :disabled="active || saving" /></label>
      <label>超时<UiSelect v-model="timeoutSeconds" variant="compact" aria-label="公共服务检测超时" :options="publicServiceTimeoutOptions" :disabled="active || saving" /></label>
      <button type="button" class="public-service-primary" :disabled="!canStart || refreshing || !!catalogError" @click="start">{{ active ? (attempt?.execution_state === 'cancelling' ? '正在取消…' : repeatCount > 1 ? `正在检测 (${currentRound}/${repeatCount})…` : '正在检测…') : saving ? '结果保存中…' : isMultiNode ? `排队检测所选 ${effectiveNodes.length} 个节点${repeatCount > 1 ? ` (每节点 ${repeatCount} 次)` : ''}` : repeatCount > 1 ? `执行检测 (${repeatCount} 次)` : '执行一次检测' }}</button>
      <button v-if="active" type="button" class="public-service-button" :disabled="attempt?.execution_state === 'cancelling'" @click="cancel">取消检测</button>
    </div>
    <nav v-if="!compact && serviceCategories.length" class="service-category-tabs" aria-label="服务检测分类">
      <button v-for="category in serviceCategories" :key="category" type="button" :aria-pressed="selectedCategory === category" @click="chooseCategory(category)">{{ category }}<span>{{ category === '全部' ? catalog.length : category === '推荐' ? recommendedRules.length : catalog.filter(rule => (rule.category || '其他') === category).length }}</span></button>
    </nav>
    <p v-if="selectedServiceID === 'antigravity'" class="public-service-note">通过所选节点实际请求模型，可能消耗少量账号额度。未绑定或凭据过期时不会判为节点故障。<button type="button" class="public-service-button" @click="openCredentials">管理 Google 凭据</button></p>
    <p v-if="attempt?.service_id === 'antigravity' && attempt.result" class="public-service-note">{{ outcomeLabel(attempt.result.outcome) }} · {{ attempt.result.error_message }}<template v-if="attempt.result.model"> · 模型 {{ attempt.result.model }}</template> · 实际请求 {{ attempt.result.request_count ?? '未知' }} 次</p>
    <p v-if="catalogError" class="public-service-error">固定服务目录读取失败：{{ catalogError }}</p>
    <div v-else-if="selectedRule" class="public-service-rule-card" :data-kind="selectedRule.result_kind">
      <div><strong>{{ selectedRule.category || '服务检测' }}<template v-if="selectedRule.region"> · {{ selectedRule.region }}</template></strong><span>{{ selectedRule.result_kind === 'ip_quality' ? 'IP 画像' : selectedRule.result_kind === 'streaming_unlock' ? '解锁分档' : selectedRule.result_kind === 'regional_reachability' ? '地区网页' : '连通性' }}</span></div>
      <p>{{ selectedRule.description }}</p>
      <small>{{ selectedRule.method }} {{ selectedRule.target_url }} · 规则 v{{ selectedRule.rule_version }} · {{ selectedRule.success_criterion }} · 响应最多 {{ selectedRule.maximum_body_bytes / 1024 }} KiB</small>
    </div>
    <p class="public-service-note">出口画像、IP 质量、网页可达和流媒体解锁是不同维度；地区网页返回成功不等于账号、支付或播放功能已解锁。</p>
    <section v-if="retryableAttempts.length || retryableLoading || retryableError || retryableHasMore" class="public-service-retry-list" aria-label="可重试保存的服务检测">
      <div><strong>可重试保存的结果</strong><p>选择已有 attempt 后，明确点击重试保存；不会重新检测。</p></div>
      <button v-for="item in retryableAttempts" :key="item.attempt_id" type="button" class="public-service-button" :aria-pressed="attempt?.attempt_id === item.attempt_id" @click="selectRetryableAttempt(item)">{{ item.rule.name }} · {{ new Date(item.result?.finished_at || item.requested_at).toLocaleString() }} · {{ item.attempt_id }}</button>
      <span v-if="retryableLoading" class="public-service-note">正在读取历史…</span>
      <span v-if="retryableError" class="public-service-error">{{ retryableError }}</span>
      <button v-if="retryableHasMore" type="button" class="public-service-button" :disabled="retryableLoading" @click="loadRetryableAttempts(true)">加载更多待处理历史</button>
    </section>
    <p v-if="error" class="public-service-error">{{ error }}</p>
    <button v-if="saveRetryRequest?.domain === 'public_service' && error" type="button" class="public-service-button" :disabled="refreshing" @click="retryLoadSaveRetryRequest">重新读取指定 attempt</button>
    <div v-if="attempt" class="public-service-result" aria-live="polite">
      <div class="public-service-result-heading"><strong>{{ attempt.rule.name }} · {{ executionLabel(attempt.execution_state) }}</strong><span>{{ persistenceLabel(attempt.persistence_state, !!attempt.result) }}</span></div>
      <p>{{ attempt.display_name || attempt.node_key }} · {{ attempt.profile_id }} · revision {{ attempt.config_revision_key }} · attempt {{ attempt.attempt_id }}</p>
      <p class="public-service-outcome">{{ attempt.result ? outcomeLabel(attempt.result.outcome) : attempt.persistence_state === 'failed' ? '测量结果未能暂存' : '等待检测结果' }}<template v-if="attempt.result?.summary"> · {{ attempt.result.summary }}</template></p>
      <p v-if="attempt.result">HTTP {{ attempt.result.http_status ?? '无响应' }} · {{ attempt.result.duration_ms }} ms · {{ attempt.result.request_count || 1 }} 个请求 · 已读取 {{ attempt.result.bytes_read }} 字节</p>
      <dl v-if="attempt.result?.details && Object.keys(attempt.result.details).length" class="public-service-details">
        <div v-for="(value, key) in attempt.result.details" :key="key"><dt>{{ detailLabel(String(key)) }}</dt><dd>{{ value || '—' }}</dd></div>
      </dl>
      <p v-if="attempt.result?.failure_phase">失败位置：{{ attempt.result.failure_phase }}<template v-if="attempt.result.error_message"> · {{ attempt.result.error_message }}</template></p>
      <p v-if="attempt.persistence_error" class="public-service-error">{{ attempt.persistence_error }}</p>
      <div v-if="serviceDisplaySamples.length" class="mt-3">
        <IntraTestSamplePlot
          mode="service"
          :service-samples="serviceDisplaySamples"
          :height="80"
          :compact="false"
        />
      </div>
      <div class="public-service-result-actions mt-3"><button type="button" class="public-service-button" :disabled="refreshing" @click="refresh()">{{ refreshing ? '读取中…' : '重新读取状态' }}</button><button v-if="saveFailed" type="button" class="public-service-button" :disabled="refreshing" @click="retrySave">重试保存（不重新检测）</button><button type="button" class="public-service-button" @click="openDetail">查看节点详情与历史</button></div>
    </div>
  </section>
</template>

<style scoped>
.compact > .public-service-rule-card { display: none; }
.public-service-panel { margin-bottom: 14px; padding: 15px 18px; border: 1px solid var(--border); border-radius: 10px; background: var(--card-bg); color: var(--text-main); }
.public-service-heading h2 { margin: 0 0 4px; font-size: 15px; }.public-service-heading p, .public-service-rule, .public-service-note, .public-service-result p { margin: 4px 0; color: var(--text-secondary); font-size: 11px; line-height: 1.5; overflow-wrap: anywhere; }
.service-category-tabs { display: flex; gap: 5px; margin: 12px 0 2px; padding-bottom: 3px; overflow-x: auto; }.service-category-tabs button { display: inline-flex; align-items: center; gap: 5px; flex: 0 0 auto; min-height: 29px; padding: 5px 9px; border: 1px solid var(--border); border-radius: 999px; color: var(--text-secondary); background: var(--card-bg); font-size: 10px; }.service-category-tabs button[aria-pressed=true] { border-color: var(--primary); color: var(--primary); background: var(--primary-subtle); font-weight: 750; }.service-category-tabs span { min-width: 16px; padding: 1px 4px; border-radius: 999px; background: color-mix(in srgb, currentColor 11%, transparent); font-size: 9px; text-align: center; }
.public-service-controls { display: flex; flex-wrap: wrap; align-items: end; gap: 9px 12px; margin: 12px 0 5px; }.public-service-controls label { display: grid; gap: 5px; min-width: 150px; color: var(--text-secondary); font-size: 10px; font-weight: 700; }.public-service-controls select { min-height: 31px; padding: 5px 8px; border: 1px solid var(--border); border-radius: 5px; background: var(--card-subtle); color: var(--text-main); font-size: 11px; }.public-service-node { display: inline-flex; align-items: center; min-height: 31px; padding: 0 8px; border: 1px solid var(--border); border-radius: 5px; color: var(--text-main); font-size: 11px; font-weight: 500; }.public-service-node.muted { color: var(--text-muted); }
.public-service-primary, .public-service-button { min-height: 31px; padding: 6px 10px; border: 1px solid var(--border); border-radius: 5px; background: var(--card-subtle); color: var(--primary); font-size: 11px; font-weight: 700; transition: all .15s ease; }.public-service-primary { border-color: var(--primary); background: var(--primary); color: white; }.public-service-primary:hover:not(:disabled) { background: var(--primary-hover); }.public-service-button:hover:not(:disabled) { border-color: var(--border-focus); background: var(--card-hover); }.public-service-primary:disabled, .public-service-button:disabled { cursor: not-allowed; opacity: .55; }
.public-service-rule-card { margin: 10px 0 7px; padding: 11px 13px; border: 1px solid color-mix(in srgb, var(--primary) 28%, var(--border)); border-left: 3px solid var(--primary); border-radius: 7px; background: color-mix(in srgb, var(--primary-subtle) 55%, var(--card-bg)); overflow-wrap: anywhere; }.public-service-rule-card[data-kind="ip_quality"], .public-service-rule-card[data-kind="exit_profile"] { border-left-color: var(--success); background: color-mix(in srgb, var(--success-bg) 66%, var(--card-bg)); }.public-service-rule-card[data-kind="streaming_unlock"] { border-left-color: var(--warning); background: color-mix(in srgb, var(--warning-bg) 58%, var(--card-bg)); }.public-service-rule-card > div { display: flex; align-items: center; gap: 8px; }.public-service-rule-card strong { font-size: 11px; }.public-service-rule-card span { padding: 2px 6px; border-radius: 999px; background: var(--card-bg); color: var(--text-secondary); font-size: 9px; }.public-service-rule-card p { margin: 5px 0; font-size: 11px; color: var(--text-main); }.public-service-rule-card small { color: var(--text-muted); font-size: 10px; line-height: 1.45; }
.public-service-note { color: var(--text-muted); }.public-service-error { margin: 7px 0; color: var(--danger); font-size: 11px; overflow-wrap: anywhere; }.public-service-result { margin-top: 11px; padding: 12px 14px; border: 1px solid var(--border); border-radius: 8px; background: var(--card-subtle); }.public-service-result-heading { display: flex; justify-content: space-between; gap: 10px; color: var(--text-main); font-size: 12px; }.public-service-result-heading span { color: var(--text-secondary); font-weight: 500; }.public-service-outcome { color: var(--text-main) !important; font-size: 13px !important; font-weight: 700; }.public-service-details { display: grid; grid-template-columns: repeat(auto-fit, minmax(145px, 1fr)); gap: 7px; margin: 10px 0 0; }.public-service-details div { min-width: 0; padding: 8px 9px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); }.public-service-details dt { color: var(--text-muted); font-size: 9px; }.public-service-details dd { margin: 3px 0 0; color: var(--text-main); font-size: 11px; font-weight: 700; overflow-wrap: anywhere; }.public-service-result-actions { display: flex; flex-wrap: wrap; gap: 7px; margin-top: 8px; }
.public-service-retry-list { display: grid; justify-items: start; gap: 6px; margin: 10px 0; padding: 10px 12px; border: 1px solid var(--border); border-radius: 7px; background: var(--card-subtle); }.public-service-retry-list p { margin: 3px 0; color: var(--text-secondary); font-size: 11px; }
</style>
