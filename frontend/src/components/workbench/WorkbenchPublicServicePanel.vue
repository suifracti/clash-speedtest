<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import * as api from '../../api/bridge'
import type { MonitorNodeOption, NodeDetailRequest, WorkbenchPublicServiceAttempt, WorkbenchPublicServiceHistoryQuery, WorkbenchPublicServiceRule } from '../../types'

const props = defineProps<{ node: MonitorNodeOption | null }>()
const emit = defineEmits<{ (event: 'open-node-detail', payload: NodeDetailRequest): void }>()

const catalog = ref<WorkbenchPublicServiceRule[]>([])
const catalogError = ref('')
const selectedServiceID = ref('cloudflare_204')
const timeoutSeconds = ref(10)
const attempt = ref<WorkbenchPublicServiceAttempt | null>(null)
const error = ref('')
const refreshing = ref(false)
let pollTimer: ReturnType<typeof setTimeout> | null = null
let requestToken = 0

type ServiceOption = { value: string; label: string; disabled?: boolean }
const serviceOptions = computed<ServiceOption[]>(() => [
  ...catalog.value.map((rule) => ({ value: rule.service_id, label: rule.name })),
  { value: 'antigravity', label: 'Antigravity · 本轮未接入', disabled: true },
  { value: 'streaming', label: '流媒体服务 · 本轮未接入', disabled: true },
])
const selectedRule = computed(() => catalog.value.find((rule) => rule.service_id === selectedServiceID.value) || null)
const active = computed(() => !!attempt.value && ['queued', 'running', 'cancelling'].includes(attempt.value.execution_state))
const saving = computed(() => attempt.value?.persistence_state === 'saving')
const saveFailed = computed(() => attempt.value?.persistence_state === 'failed' && !!attempt.value?.result)
const canStart = computed(() => !!props.node && !!selectedRule.value && !active.value && !saving.value && !saveFailed.value)

onMounted(async () => {
  try {
    catalog.value = await api.listWorkbenchPublicServiceCatalog()
    if (!catalog.value.some((rule) => rule.service_id === selectedServiceID.value)) selectedServiceID.value = catalog.value[0]?.service_id || ''
  } catch (cause) {
    catalogError.value = messageFor(cause)
  }
})

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

function newRequestID(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `service-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

async function start(): Promise<void> {
  const node = props.node
  const rule = selectedRule.value
  if (!node || !rule || !canStart.value) return
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

function outcomeLabel(value?: string): string {
  return ({ matched: '目标响应符合判据', http_rejected: '目标响应不符合判据', rate_limited: '目标响应指示请求受限', redirect: '目标返回重定向，未跟随', timed_out: '请求超时', cancelled: '用户取消', transport_error: '代理或 HTTP 传输失败，具体阶段未知', criteria_mismatch: '响应不符合判据' } as Record<string, string>)[value || ''] || '尚无测量结果'
}

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
  <section class="public-service-panel" aria-labelledby="public-service-title">
    <header class="public-service-heading"><div><h2 id="public-service-title">公共服务即时检测</h2><p>一次性单节点请求；不使用 Monitor 预算，也不进入 Monitor 推荐证据。</p></div></header>
    <div class="public-service-controls">
      <label>节点
        <span v-if="node" class="public-service-node">{{ node.displayName }} · {{ node.profileName }} · {{ node.type || '节点' }}</span>
        <span v-else class="public-service-node muted">请只勾选一个节点</span>
      </label>
      <label>公共服务<select v-model="selectedServiceID" aria-label="选择公共服务" :disabled="active || saving"><option v-for="option in serviceOptions" :key="option.value" :value="option.value" :disabled="option.disabled">{{ option.label }}</option></select></label>
      <label>超时<select v-model.number="timeoutSeconds" aria-label="公共服务检测超时" :disabled="active || saving"><option :value="5">5 秒</option><option :value="10">10 秒</option><option :value="20">20 秒</option><option :value="30">30 秒</option></select></label>
      <button type="button" class="public-service-primary" :disabled="!canStart || refreshing || !!catalogError" @click="start">{{ active ? (attempt?.execution_state === 'cancelling' ? '正在取消…' : '正在检测…') : saving ? '结果保存中…' : '执行一次检测' }}</button>
      <button v-if="active" type="button" class="public-service-button" :disabled="attempt?.execution_state === 'cancelling'" @click="cancel">取消检测</button>
    </div>
    <p v-if="catalogError" class="public-service-error">固定服务目录读取失败：{{ catalogError }}</p>
    <p v-else-if="selectedRule" class="public-service-rule">{{ selectedRule.method }} {{ selectedRule.target_url }} · 规则 v{{ selectedRule.rule_version }} · {{ selectedRule.success_criterion }} · 不跟随重定向 · 响应体最多读取 {{ selectedRule.maximum_body_bytes / 1024 }} KiB · 无账号、cookie 或认证头。</p>
    <p class="public-service-note">仅表示该固定目标是否符合列明的 HTTP 判据。未符合判据不能单独证明节点失效或地区封锁；GitHub 根端点符合判据也不代表登录、仓库读写或其他功能可用。</p>
    <p v-if="error" class="public-service-error">{{ error }}</p>
    <div v-if="attempt" class="public-service-result" aria-live="polite">
      <div class="public-service-result-heading"><strong>{{ attempt.rule.name }} · {{ executionLabel(attempt.execution_state) }}</strong><span>{{ persistenceLabel(attempt.persistence_state, !!attempt.result) }}</span></div>
      <p>{{ attempt.display_name || attempt.node_key }} · {{ attempt.profile_id }} · revision {{ attempt.config_revision_key }} · attempt {{ attempt.attempt_id }}</p>
      <p>{{ attempt.result ? outcomeLabel(attempt.result.outcome) : attempt.persistence_state === 'failed' ? '测量结果未能暂存' : '等待检测结果' }}<template v-if="attempt.result"> · HTTP {{ attempt.result.http_status ?? '无响应' }} · {{ attempt.result.duration_ms }} ms · 已读取 {{ attempt.result.bytes_read }} 字节</template></p>
      <p v-if="attempt.result?.failure_phase">失败位置：{{ attempt.result.failure_phase }}<template v-if="attempt.result.error_message"> · {{ attempt.result.error_message }}</template></p>
      <p v-if="attempt.persistence_error" class="public-service-error">{{ attempt.persistence_error }}</p>
      <div class="public-service-result-actions"><button type="button" class="public-service-button" :disabled="refreshing" @click="refresh()">{{ refreshing ? '读取中…' : '重新读取状态' }}</button><button v-if="saveFailed" type="button" class="public-service-button" :disabled="refreshing" @click="retrySave">重试保存（不重新检测）</button><button type="button" class="public-service-button" @click="openDetail">查看节点详情与历史</button></div>
    </div>
  </section>
</template>

<style scoped>
.public-service-panel { margin-bottom: 14px; padding: 15px 18px; border: 1px solid var(--border, #d6dde1); border-radius: 10px; background: var(--card-bg, #fff); color: var(--text-main, #1f2933); }
.public-service-heading h2 { margin: 0 0 4px; font-size: 15px; }.public-service-heading p, .public-service-rule, .public-service-note, .public-service-result p { margin: 4px 0; color: var(--text-secondary, #596873); font-size: 11px; line-height: 1.5; overflow-wrap: anywhere; }
.public-service-controls { display: flex; flex-wrap: wrap; align-items: end; gap: 9px 12px; margin: 12px 0 5px; }.public-service-controls label { display: grid; gap: 5px; min-width: 150px; color: var(--text-secondary, #596873); font-size: 10px; font-weight: 700; }.public-service-controls select { min-height: 31px; padding: 5px 8px; border: 1px solid var(--border, #d6dde1); border-radius: 5px; background: white; color: var(--text-main, #1f2933); font-size: 11px; }.public-service-node { display: inline-flex; align-items: center; min-height: 31px; padding: 0 8px; border: 1px solid var(--border, #d6dde1); border-radius: 5px; color: var(--text-main, #1f2933); font-size: 11px; font-weight: 500; }.public-service-node.muted { color: var(--text-muted, #78868f); }
.public-service-primary, .public-service-button { min-height: 31px; padding: 6px 10px; border: 1px solid var(--border, #d6dde1); border-radius: 5px; background: white; color: var(--primary, #256b78); font-size: 11px; font-weight: 700; }.public-service-primary { border-color: var(--primary, #256b78); background: var(--primary, #256b78); color: white; }.public-service-primary:disabled, .public-service-button:disabled { cursor: not-allowed; opacity: .55; }
.public-service-rule { overflow-wrap: anywhere; }.public-service-note { color: var(--text-muted, #75838c); }.public-service-error { margin: 7px 0; color: #a32f36; font-size: 11px; overflow-wrap: anywhere; }.public-service-result { margin-top: 11px; padding: 10px 12px; border: 1px solid var(--border, #d6dde1); border-radius: 7px; background: var(--card-subtle, #f8fafb); }.public-service-result-heading { display: flex; justify-content: space-between; gap: 10px; color: var(--text-main, #1f2933); font-size: 12px; }.public-service-result-heading span { color: var(--text-secondary, #596873); font-weight: 500; }.public-service-result-actions { display: flex; flex-wrap: wrap; gap: 7px; margin-top: 8px; }
</style>
