<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { fetchMonitorNodeOptions } from '../../api/monitor'
import * as api from '../../api/bridge'
import LatencySamplePlot from './LatencySamplePlot.vue'
import {
  acceptsLatencyDetailResponse,
  acceptsLatencyScopeResponse,
  acceptsLatencyTestResult,
  sameLatencyScope,
  testMatchesLatencyScope,
  type LatencyAttemptScope,
  type LatencyScope,
} from './latencyRequestGuard'
import type { MonitorNodeOption, WorkbenchLatencyTest } from '../../types'

const options = ref<MonitorNodeOption[]>([])
const optionsLoading = ref(false)
const optionsError = ref('')
const selectedProfileId = ref('')
const selectedNodeKey = ref('')

const history = ref<WorkbenchLatencyTest[]>([])
const historyLoading = ref(false)
const historyError = ref('')
const focusedTest = ref<WorkbenchLatencyTest | null>(null)
const currentResult = ref<WorkbenchLatencyTest | null>(null)
const selectedAttemptID = ref('')

const testing = ref(false)
const testError = ref('')
const persistenceNotice = ref('')
const detailLoading = ref(false)
const expanded = ref(false)
const hoveredIndex = ref<number | null>(null)
const pinnedIndex = ref<number | null>(null)

let historyRequestID = 0
let detailRequestID = 0
let runRequestID = 0
let activeRunID = 0
let unsubscribeEvents: (() => void) | null = null
const pendingPersistence = new Map<string, WorkbenchLatencyTest>()

const profileOptions = computed(() => {
  const seen = new Set<string>()
  return options.value.filter((option) => {
    if (seen.has(option.profileId)) return false
    seen.add(option.profileId)
    return true
  })
})

const nodeOptions = computed(() => options.value.filter((option) => option.profileId === selectedProfileId.value))
const selectedNode = computed(() => nodeOptions.value.find((option) => option.nodeKey === selectedNodeKey.value) || null)
const displayedTest = computed(() => focusedTest.value || currentResult.value || history.value[0] || null)
const activeSample = computed(() => {
  const test = displayedTest.value
  if (!test || test.samples.length === 0) return null
  const index = hoveredIndex.value ?? pinnedIndex.value ?? test.samples.length - 1
  return test.samples[Math.max(0, Math.min(index, test.samples.length - 1))]
})
const activeSampleIndex = computed(() => {
  const test = displayedTest.value
  if (!test || test.samples.length === 0) return null
  return hoveredIndex.value ?? pinnedIndex.value ?? test.samples.length - 1
})

function messageFor(error: unknown): string {
  if (error instanceof Error) return error.message
  return String(error || '请求失败')
}

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? '时间未知'
    : date.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function statusLabel(test: WorkbenchLatencyTest | null): string {
  if (!test) return '尚未测试'
  if (test.status === 'completed') return '本次样本全部通过'
  if (test.status === 'partial_failed') return '部分样本失败'
  return '本次样本全部失败'
}

function statusClass(test: WorkbenchLatencyTest | null): string {
  if (!test) return 'text-content-muted'
  if (test.status === 'completed') return 'text-emerald-600 dark:text-emerald-400'
  if (test.status === 'partial_failed') return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

function sampleLabel(sample: WorkbenchLatencyTest['samples'][number] | null): string {
  if (!sample) return '还没有原始样本'
  if (!sample.success) return `失败：${sample.error || '连接失败或超时'}`
  return `${sample.latency_ms} ms`
}

function currentScope(): LatencyScope {
  return { profileId: selectedProfileId.value, nodeKey: selectedNodeKey.value }
}

function takePendingPersistence(test: WorkbenchLatencyTest): WorkbenchLatencyTest {
  const update = pendingPersistence.get(test.attempt_id)
  if (!update || !testMatchesLatencyScope(update, currentScope())) return test
  pendingPersistence.delete(test.attempt_id)
  return update
}

function isWorkbenchLatencyTestPayload(payload: unknown): payload is WorkbenchLatencyTest {
  if (!payload || typeof payload !== 'object') return false
  const test = payload as Partial<WorkbenchLatencyTest>
  return typeof test.attempt_id === 'string' &&
    typeof test.profile_id === 'string' &&
    typeof test.node_key === 'string' &&
    (test.persistence_state === 'saving' || test.persistence_state === 'saved' || test.persistence_state === 'failed')
}

function handlePersistenceEvent(type: string, payload: unknown) {
  if (type !== 'workbench_latency_test_persistence_updated' || !isWorkbenchLatencyTestPayload(payload)) return

  pendingPersistence.set(payload.attempt_id, payload)
  while (pendingPersistence.size > 20) {
    const oldest = pendingPersistence.keys().next().value
    if (oldest) pendingPersistence.delete(oldest)
    else break
  }

  const scope = currentScope()
  if (!testMatchesLatencyScope(payload, scope)) return

  let applied = false
  if (currentResult.value?.attempt_id === payload.attempt_id && testMatchesLatencyScope(currentResult.value, scope)) {
    currentResult.value = payload
    applied = true
  }
  if (focusedTest.value?.attempt_id === payload.attempt_id && testMatchesLatencyScope(focusedTest.value, scope)) {
    focusedTest.value = payload
    applied = true
  }
  const historyIndex = history.value.findIndex((item) => item.attempt_id === payload.attempt_id)
  if (historyIndex >= 0 && testMatchesLatencyScope(history.value[historyIndex], scope)) {
    history.value.splice(historyIndex, 1, payload)
    applied = true
  }

  if (applied) {
    pendingPersistence.delete(payload.attempt_id)
    if (payload.persistence_state === 'failed') {
      persistenceNotice.value = payload.persistence_error || '测试完成，但历史保存失败'
    } else if (payload.persistence_state === 'saved') {
      persistenceNotice.value = ''
      void loadHistory()
    }
  }
}

async function loadOptions() {
  optionsLoading.value = true
  optionsError.value = ''
  try {
    options.value = await fetchMonitorNodeOptions()
    if (!selectedProfileId.value && profileOptions.value.length > 0) {
      selectedProfileId.value = profileOptions.value[0].profileId
    }
    if (!selectedNodeKey.value && nodeOptions.value.length > 0) {
      selectedNodeKey.value = nodeOptions.value[0].nodeKey
    }
  } catch (error) {
    optionsError.value = messageFor(error)
  } finally {
    optionsLoading.value = false
  }
}

async function loadHistory() {
  const scope = currentScope()
  const requestID = ++historyRequestID
  if (!scope.profileId || !scope.nodeKey) {
    history.value = []
    focusedTest.value = null
    historyLoading.value = false
    return
  }
  historyLoading.value = true
  historyError.value = ''
  try {
    const loaded = await api.fetchWorkbenchLatencyHistory({
      profile_id: scope.profileId,
      node_key: scope.nodeKey,
      limit: 20,
    })
    if (!acceptsLatencyScopeResponse(requestID, historyRequestID, scope, currentScope())) return
    if (loaded.some((test) => !testMatchesLatencyScope(test, scope))) {
      historyError.value = '历史响应归属不一致，已忽略本次结果，请重试'
      history.value = []
      focusedTest.value = null
      return
    }
    history.value = loaded
    const preserved = selectedAttemptID.value ? loaded.find((test) => test.attempt_id === selectedAttemptID.value) : null
    focusedTest.value = preserved || loaded[0] || null
    selectedAttemptID.value = focusedTest.value?.attempt_id || ''
  } catch (error) {
    if (acceptsLatencyScopeResponse(requestID, historyRequestID, scope, currentScope())) {
      historyError.value = messageFor(error)
    }
  } finally {
    if (requestID === historyRequestID && sameLatencyScope(scope, currentScope())) historyLoading.value = false
  }
}

watch(selectedProfileId, async () => {
  const first = nodeOptions.value[0]
  selectedNodeKey.value = first?.nodeKey || ''
  currentResult.value = null
  pinnedIndex.value = null
  hoveredIndex.value = null
  history.value = []
  focusedTest.value = null
  selectedAttemptID.value = ''
  detailRequestID++
  await loadHistory()
})

watch(selectedNodeKey, async () => {
  currentResult.value = null
  focusedTest.value = null
  selectedAttemptID.value = ''
  pinnedIndex.value = null
  hoveredIndex.value = null
  history.value = []
  detailRequestID++
  await loadHistory()
})

async function runTest() {
  if (testing.value || !selectedNode.value) return
  const scope = currentScope()
  const requestID = ++runRequestID
  activeRunID = requestID
  testing.value = true
  testError.value = ''
  persistenceNotice.value = ''
  currentResult.value = null
  focusedTest.value = null
  pinnedIndex.value = null
  hoveredIndex.value = null
  try {
    const result = await api.runWorkbenchLatencyTest({
      profile_id: scope.profileId,
      node_key: scope.nodeKey,
      test_project: 'latency_stability',
      timeout_seconds: 5,
    })
    if (!acceptsLatencyTestResult(requestID, runRequestID, scope, currentScope(), result)) return
    const boundResult = takePendingPersistence(result)
    currentResult.value = boundResult
    focusedTest.value = boundResult
    selectedAttemptID.value = boundResult.attempt_id
    if (boundResult.persistence_state === 'failed') {
      persistenceNotice.value = boundResult.persistence_error || '测试完成，但历史保存失败'
    } else if (boundResult.persistence_state === 'saved') {
      persistenceNotice.value = ''
      await loadHistory()
    }
  } catch (error) {
    if (requestID === activeRunID && sameLatencyScope(scope, currentScope())) testError.value = messageFor(error)
  } finally {
    if (requestID === activeRunID) {
      activeRunID = 0
      testing.value = false
    }
  }
}

async function selectHistory(test: WorkbenchLatencyTest) {
  const scope = currentScope()
  if (!testMatchesLatencyScope(test, scope) || !test.attempt_id) return
  const requestID = ++detailRequestID
  const requested: LatencyAttemptScope = { ...scope, attemptId: test.attempt_id }
  selectedAttemptID.value = test.attempt_id
  focusedTest.value = test
  historyError.value = ''
  detailLoading.value = true
  try {
    const loaded = await api.fetchWorkbenchLatencyTest({
      profile_id: requested.profileId,
      node_key: requested.nodeKey,
      attempt_id: requested.attemptId,
    })
    if (!acceptsLatencyDetailResponse(requestID, detailRequestID, requested, currentScope(), selectedAttemptID.value, loaded)) return
    focusedTest.value = loaded
  } catch (error) {
    if (acceptsLatencyDetailResponse(requestID, detailRequestID, requested, currentScope(), selectedAttemptID.value, test)) {
      historyError.value = messageFor(error)
    }
  } finally {
    if (requestID === detailRequestID && sameLatencyScope(scope, currentScope())) detailLoading.value = false
  }
  currentResult.value = null
  pinnedIndex.value = null
  hoveredIndex.value = null
}

function onHover(index: number | null) {
  hoveredIndex.value = index
}

function onPin(index: number) {
  pinnedIndex.value = index
  hoveredIndex.value = null
}

function clearPin() {
  pinnedIndex.value = null
  hoveredIndex.value = null
}

onMounted(async () => {
  unsubscribeEvents = api.subscribeEvents(handlePersistenceEvent)
  await loadOptions()
})

onUnmounted(() => {
  unsubscribeEvents?.()
  unsubscribeEvents = null
})
</script>

<template>
  <main class="flex-1 overflow-auto bg-canvas">
    <div class="mx-auto max-w-[1440px] px-6 py-5 space-y-5">
      <section class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.16em] text-content-muted">工作台 / 单节点闭环</p>
          <h2 class="mt-1 text-xl font-semibold text-content-main">延迟与稳定性</h2>
          <p class="mt-1 text-sm text-content-secondary">选择真实订阅节点，立即测试；结果和每条原始样本会保存到本地历史。</p>
        </div>
        <button
          type="button"
          class="rounded-md border border-border bg-card px-3 py-2 text-xs font-medium text-content-secondary hover:border-brand hover:text-brand disabled:opacity-50"
          :disabled="optionsLoading"
          @click="loadOptions"
        >
          {{ optionsLoading ? '读取节点中…' : '重新读取节点' }}
        </button>
      </section>

      <section class="rounded-lg border border-border bg-card p-4 shadow-sm">
        <div class="grid gap-4 md:grid-cols-[minmax(180px,0.7fr)_minmax(320px,1.3fr)_auto] md:items-end">
          <label class="block">
            <span class="mb-1.5 block text-xs font-medium text-content-secondary">订阅</span>
            <select
              v-model="selectedProfileId"
              class="w-full rounded-md border border-border bg-card-subtle px-3 py-2 text-sm text-content-main outline-none focus:border-brand"
              :disabled="optionsLoading || profileOptions.length === 0"
            >
              <option v-for="profile in profileOptions" :key="profile.profileId" :value="profile.profileId">
                {{ profile.profileName }}
              </option>
            </select>
          </label>
          <label class="block">
            <span class="mb-1.5 block text-xs font-medium text-content-secondary">节点</span>
            <select
              v-model="selectedNodeKey"
              class="w-full rounded-md border border-border bg-card-subtle px-3 py-2 text-sm text-content-main outline-none focus:border-brand"
              :disabled="optionsLoading || nodeOptions.length === 0"
            >
              <option v-for="node in nodeOptions" :key="node.nodeKey" :value="node.nodeKey">
                {{ node.displayName }} · {{ node.type }} · {{ node.countryCode || '未知地区' }}
              </option>
            </select>
          </label>
          <button
            type="button"
            class="rounded-md bg-blue-600 px-5 py-2 text-sm font-semibold text-white hover:bg-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="testing || !selectedNode"
            @click="runTest"
          >
            {{ testing ? '测试中…' : '立即测试' }}
          </button>
        </div>
        <div v-if="selectedNode" class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-content-muted">
          <span class="font-medium text-content-main">{{ selectedNode.displayName }}</span>
          <span>{{ selectedNode.type }}</span>
          <span>{{ selectedNode.profileName }}</span>
          <span>测试项目：延迟与稳定性</span>
        </div>
        <p v-if="optionsError" class="mt-3 rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
          节点读取失败：{{ optionsError }}。请刷新订阅缓存后重试。
        </p>
        <p v-else-if="!optionsLoading && options.length === 0" class="mt-3 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          当前没有可运行节点。请先刷新订阅，并确认缓存中存在有效节点配置。
        </p>
        <p v-if="testError" class="mt-3 rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">
          测试未开始：{{ testError }}
        </p>
        <p v-if="persistenceNotice" class="mt-3 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          测试结果已返回，但没有写入历史：{{ persistenceNotice }}。可保留当前结果，修复存储后重新测试。
        </p>
      </section>

      <section class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div class="rounded-lg border border-border bg-card p-4 shadow-sm">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="text-xs font-semibold text-content-muted">当前结果</p>
              <h3 class="mt-1 text-base font-semibold text-content-main">{{ displayedTest?.display_name || selectedNode?.displayName || '选择一个节点' }}</h3>
            </div>
            <div v-if="displayedTest" class="text-right text-xs text-content-muted">
              <div :class="['font-medium', statusClass(displayedTest)]">{{ statusLabel(displayedTest) }}</div>
              <div>{{ formatTime(displayedTest.finished_at) }}</div>
              <div v-if="displayedTest.persistence_state === 'saving'" class="mt-1 text-amber-600 dark:text-amber-400">历史保存中…</div>
              <div v-else-if="displayedTest.persistence_state === 'saved'" class="mt-1 text-emerald-600 dark:text-emerald-400">历史已保存</div>
              <div v-else class="mt-1 text-red-600 dark:text-red-400">历史保存失败</div>
            </div>
          </div>

          <div v-if="displayedTest" class="mt-4 grid gap-4 md:grid-cols-[150px_minmax(0,1fr)] md:items-start">
            <div class="border-r border-border pr-4">
              <div class="text-[11px] text-content-muted">平均延迟</div>
              <div class="mt-1 whitespace-nowrap text-2xl font-semibold leading-none text-content-main">
                {{ displayedTest.latency_ms > 0 ? displayedTest.latency_ms : '—' }}
                <span v-if="displayedTest.latency_ms > 0" class="text-sm font-normal text-content-secondary">ms</span>
              </div>
              <div class="mt-3 text-xs text-content-secondary">抖动 {{ displayedTest.jitter_ms }} ms</div>
              <div class="text-xs text-content-secondary">失败 {{ displayedTest.failure_samples }} / {{ displayedTest.total_samples }}</div>
              <div class="mt-3 text-[11px] text-content-muted">最近指向样本</div>
              <div class="mt-1 text-sm font-medium text-content-main">{{ sampleLabel(activeSample) }}</div>
              <div v-if="activeSample" class="text-[11px] text-content-muted">{{ formatTime(activeSample.timestamp) }}</div>
              <div v-if="pinnedIndex !== null" class="mt-2 flex items-center gap-2 text-[11px] text-content-secondary">
                <span>已固定第 {{ pinnedIndex + 1 }} 条</span>
                <button type="button" class="underline hover:text-brand" @click="clearPin">取消固定</button>
              </div>
            </div>

            <div class="min-w-0">
              <div class="mb-2 flex flex-wrap items-center justify-between gap-2 text-xs text-content-muted">
                <span>原始延迟样本 · 横轴为真实采样时间 · 失败不连线</span>
                <span>悬停就近读取，点击或回车固定</span>
              </div>
              <LatencySamplePlot
                :samples="displayedTest.samples"
                :hovered-index="hoveredIndex"
                :pinned-index="pinnedIndex"
                :height="96"
                @hover="onHover"
                @pin="onPin"
                @unpin="clearPin"
              />
              <div class="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-content-muted">
                <span>{{ displayedTest.success_samples }} 次成功，{{ displayedTest.failure_samples }} 次失败</span>
                <button type="button" class="font-medium text-brand hover:underline" @click="expanded = true">展开大图与样本</button>
              </div>
            </div>
          </div>

          <div v-else class="flex min-h-[180px] items-center justify-center text-center text-sm text-content-muted">
            选择节点后点击“立即测试”，这里会显示真实结果和逐条原始样本。
          </div>

          <div v-if="displayedTest?.error_message" class="mt-4 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
            本次测试原因：{{ displayedTest.error_message }}
          </div>
        </div>

        <aside class="rounded-lg border border-border bg-card p-4 shadow-sm">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-xs font-semibold text-content-muted">已保存历史</p>
              <h3 class="mt-1 text-base font-semibold text-content-main">这个节点的测试记录</h3>
            </div>
            <span v-if="historyLoading || detailLoading" class="text-xs text-content-muted">读取中…</span>
          </div>
          <p v-if="historyError" class="mt-3 text-sm text-red-600 dark:text-red-400">历史读取失败：{{ historyError }}</p>
          <p v-else-if="!historyLoading && history.length === 0" class="mt-4 text-sm leading-6 text-content-secondary">
            还没有保存记录。完成一次真实延迟测试后，刷新页面或重启应用仍可从这里查看。
          </p>
          <div v-else class="mt-3 space-y-1.5">
            <button
              v-for="test in history"
              :key="test.attempt_id"
              type="button"
              class="w-full rounded-md border px-3 py-2 text-left transition-colors"
              :class="selectedAttemptID === test.attempt_id ? 'border-blue-500 bg-blue-50 dark:border-blue-400/60 dark:bg-blue-500/10' : 'border-border hover:border-border-subtle hover:bg-card-subtle'"
              @click="selectHistory(test)"
            >
              <div class="flex items-center justify-between gap-2 text-sm">
                <span class="font-medium text-content-main">{{ test.latency_ms > 0 ? `${test.latency_ms} ms` : '无有效延迟' }}</span>
                <span :class="['text-xs', statusClass(test)]">{{ test.status === 'completed' ? '完成' : test.status === 'partial_failed' ? '部分失败' : '失败' }}</span>
              </div>
              <div class="mt-1 flex items-center justify-between gap-2 text-[11px] text-content-muted">
                <span>{{ formatTime(test.finished_at) }}</span>
                <span>{{ test.success_samples }}/{{ test.total_samples }} 成功</span>
              </div>
            </button>
          </div>
        </aside>
      </section>
    </div>

    <div v-if="expanded && displayedTest" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-6" @click.self="expanded = false">
      <section class="max-h-[90vh] w-full max-w-[1180px] overflow-auto rounded-xl border border-border bg-card p-5 shadow-2xl">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p class="text-xs font-semibold text-content-muted">历史详情</p>
            <h3 class="mt-1 text-lg font-semibold text-content-main">{{ displayedTest.display_name }} · 延迟与稳定性</h3>
            <p class="mt-1 text-xs text-content-secondary">{{ formatTime(displayedTest.started_at) }} – {{ formatTime(displayedTest.finished_at) }} · {{ statusLabel(displayedTest) }}</p>
          </div>
          <div class="flex items-center gap-2">
            <button v-if="pinnedIndex !== null" type="button" class="rounded-md border border-border px-3 py-1.5 text-xs text-content-secondary hover:text-brand" @click="clearPin">取消固定</button>
            <button type="button" class="rounded-md border border-border px-3 py-1.5 text-xs text-content-secondary hover:text-brand" @click="expanded = false">关闭</button>
          </div>
        </div>

        <div class="mt-5 rounded-lg border border-border bg-card-subtle p-3">
          <LatencySamplePlot
            :samples="displayedTest.samples"
            :hovered-index="hoveredIndex"
            :pinned-index="pinnedIndex"
            :width="1120"
            :height="300"
            @hover="onHover"
            @pin="onPin"
            @unpin="clearPin"
          />
        </div>

        <div class="mt-4 grid gap-3 md:grid-cols-2">
          <div class="rounded-md border border-border p-3 text-sm">
            <div class="text-xs font-semibold text-content-muted">当前指向</div>
            <div class="mt-1 font-medium text-content-main">{{ activeSample ? formatTime(activeSample.timestamp) : '—' }}</div>
            <div class="mt-1 text-content-secondary">{{ sampleLabel(activeSample) }}</div>
          </div>
          <div class="rounded-md border border-border p-3 text-sm">
            <div class="text-xs font-semibold text-content-muted">保存记录</div>
            <div class="mt-1 font-medium text-content-main">
              {{ displayedTest.persistence_state === 'saving' ? '保存中' : displayedTest.persistence_state === 'saved' ? '已保存' : '保存失败' }}
            </div>
            <div class="mt-1 text-content-secondary">{{ displayedTest.attempt_id }}</div>
          </div>
        </div>

        <div class="mt-4 overflow-x-auto rounded-md border border-border">
          <table class="w-full min-w-[560px] text-left text-sm">
            <thead class="bg-card-subtle text-xs text-content-secondary">
              <tr>
                <th class="px-3 py-2 font-medium">样本</th>
                <th class="px-3 py-2 font-medium">实际时间</th>
                <th class="px-3 py-2 font-medium">结果</th>
                <th class="px-3 py-2 font-medium">原因</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              <tr v-for="(sample, index) in displayedTest.samples" :key="`${sample.seq}-${sample.timestamp}`" :class="activeSampleIndex === index ? 'bg-blue-50 dark:bg-blue-500/10' : ''">
                <td class="px-3 py-2 font-mono text-xs text-content-muted">#{{ sample.seq }}</td>
                <td class="px-3 py-2 text-content-secondary">{{ formatTime(sample.timestamp) }}</td>
                <td class="px-3 py-2 font-medium" :class="sample.success ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">
                  {{ sample.success ? `${sample.latency_ms} ms` : '失败' }}
                </td>
                <td class="px-3 py-2 text-content-secondary">{{ sample.error || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </main>
</template>
