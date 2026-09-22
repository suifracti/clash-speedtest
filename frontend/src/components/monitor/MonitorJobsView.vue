<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as api from '../../api/monitor'
import UiSelect from '../common/UiSelect.vue'
import MonitorRetentionPanel from './MonitorRetentionPanel.vue'
import type { MonitorJob, MonitorJobNode, MonitorJobPrefill, MonitorNodeOption, MonitorNodeSelectionContext, MonitorRun } from '../../types'

const props = defineProps<{ prefill?: MonitorJobPrefill | null }>()

const emit = defineEmits<{
  (event: 'open-timeline', payload: { profileId: string; node: MonitorJobNode }): void
  (event: 'prefill-consumed'): void
}>()

const options = ref<MonitorNodeOption[]>([])
const jobs = ref<MonitorJob[]>([])
const recentRuns = ref<Record<string, MonitorRun[]>>({})
const runErrors = ref<Record<string, string>>({})

const loading = ref(false)
const loadError = ref('')
const feedback = ref('')
const creating = ref(false)
const busyJob = ref<{ id: string; action: api.MonitorJobAction } | null>(null)

const name = ref('')
const profileId = ref('')
const selectedNodeKeys = ref<string[]>([])
const probeSet = ref<'light' | 'service' | 'heavy'>('light')
const intervalSeconds = ref(60)
const timeoutSeconds = ref(10)
const prefillActive = ref(false)
const prefillError = ref('')
const prefillContexts = ref<Record<string, MonitorNodeSelectionContext>>({})

let refreshTimer: ReturnType<typeof setInterval> | null = null
let refreshInFlight = false

const profileChoices = computed(() => {
  const byId = new Map<string, { id: string; name: string; count: number }>()
  for (const node of options.value) {
    const current = byId.get(node.profileId)
    if (current) current.count += 1
    else byId.set(node.profileId, { id: node.profileId, name: node.profileName, count: 1 })
  }
  return Array.from(byId.values()).sort((a, b) => a.name.localeCompare(b.name))
})

const visibleNodes = computed(() => options.value.filter((node) => node.profileId === profileId.value))
const canCreate = computed(
  () =>
    !creating.value &&
    !prefillError.value &&
    !!profileId.value &&
    selectedNodeKeys.value.length > 0 &&
    intervalSeconds.value >= 1 &&
    timeoutSeconds.value >= 1
)
const selectedNodeNames = computed(() => selectedNodeKeys.value
  .map((key) => visibleNodes.value.find((node) => node.nodeKey === key)?.displayName || key)
  .filter(Boolean))

const probeOptions = [
  { value: 'light', label: 'Light（延迟 / 轻量 HTTP）' },
  { value: 'service', label: 'Service（常用服务）' },
  { value: 'heavy', label: 'Heavy（深度诊断）' },
]
const intervalOptions = [
  { value: 10, label: '10 秒' },
  { value: 30, label: '30 秒' },
  { value: 60, label: '1 分钟' },
  { value: 300, label: '5 分钟' },
  { value: 900, label: '15 分钟' },
]
const timeoutOptions = [
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
  { value: 30, label: '30 秒' },
  { value: 60, label: '60 秒' },
]

function selectProfile(next: string | number): void {
  if (prefillActive.value && String(next) !== profileId.value) {
    prefillActive.value = false
    prefillContexts.value = {}
  }
  profileId.value = String(next)
  const visible = new Set(visibleNodes.value.map((node) => node.nodeKey))
  selectedNodeKeys.value = selectedNodeKeys.value.filter((key) => visible.has(key))
}

function contextFromOption(node: MonitorNodeOption): MonitorNodeSelectionContext {
  return {
    node_key: node.nodeKey,
    node_identity_key: node.nodeIdentityKey,
    config_revision_key: node.configRevisionKey,
  }
}

function applyPrefill(prefill: MonitorJobPrefill): void {
  const requestedKeys = [...new Set(prefill.nodeKeys)]
  const contexts = Object.fromEntries(prefill.nodeContexts.map((context) => [context.node_key, context]))
  const currentNodes = requestedKeys.map((key) => options.value.find((node) => node.profileId === prefill.profileId && node.nodeKey === key))
  const missingKey = currentNodes.some((node) => !node)
  const staleContext = currentNodes.some((node) => {
    if (!node) return true
    const context = contexts[node.nodeKey]
    return !context || context.node_identity_key !== node.nodeIdentityKey || context.config_revision_key !== node.configRevisionKey
  })

  if (!prefill.profileId || requestedKeys.length === 0 || requestedKeys.length !== prefill.nodeContexts.length || missingKey || staleContext) {
    prefillActive.value = false
    prefillError.value = '工作台选择已失效：订阅、节点身份或配置 revision 已变化，请返回工作台重新选择。'
    selectedNodeKeys.value = []
    prefillContexts.value = {}
    emit('prefill-consumed')
    return
  }

  profileId.value = prefill.profileId
  selectedNodeKeys.value = requestedKeys
  prefillContexts.value = contexts
  prefillActive.value = true
  prefillError.value = ''
  feedback.value = `已从工作台带入 ${selectedNodeNames.value.length} 个节点：${selectedNodeNames.value.join('、')}。请确认创建，任务不会自动启动。`
  emit('prefill-consumed')
}

function cancelPrefill(): void {
  prefillActive.value = false
  prefillError.value = ''
  prefillContexts.value = {}
  selectedNodeKeys.value = []
  feedback.value = '已取消本次工作台选择，尚未创建 Monitor 任务。'
}

function selectedNodeContexts(): MonitorNodeSelectionContext[] | undefined {
  if (!prefillActive.value) return undefined
  const currentByKey = new Map(options.value.map((node) => [node.nodeKey, contextFromOption(node)]))
  const contexts = selectedNodeKeys.value.map((key) => prefillContexts.value[key] || currentByKey.get(key))
  return contexts.every((context): context is MonitorNodeSelectionContext => !!context) ? contexts : undefined
}

function setProbeSet(value: string | number): void {
  if (value === 'light' || value === 'service' || value === 'heavy') probeSet.value = value
}

function setIntervalSeconds(value: string | number): void {
  const next = Number(value)
  if (Number.isFinite(next)) intervalSeconds.value = next
}

function setTimeoutSeconds(value: string | number): void {
  const next = Number(value)
  if (Number.isFinite(next)) timeoutSeconds.value = next
}

function formatState(state: MonitorJob['state']): string {
  return { stopped: '已停止', running: '运行中', paused: '已暂停', blocked: '已阻塞' }[state] ?? state
}

function stateClass(state: MonitorJob['state']): string {
  return {
    stopped: 'text-content-muted bg-card-subtle border-border',
    running: 'text-emerald-700 dark:text-emerald-300 bg-emerald-500/10 border-emerald-500/25',
    paused: 'text-amber-700 dark:text-amber-300 bg-amber-500/10 border-amber-500/25',
    blocked: 'text-red-700 dark:text-red-300 bg-red-500/10 border-red-500/25',
  }[state]
}

function probeLabel(probe: MonitorJob['probeSet']): string {
  return { light: '轻量连通性', service: '服务响应', heavy: '完整探针组合' }[probe] ?? probe
}

function runLabel(status: MonitorRun['status']): string {
  return {
    running: '执行中',
    completed: '完成',
    partial_failed: '部分失败',
    failed: '失败',
    skipped: '跳过（重叠）',
    persistence_failed: '保存失败（未持久化完整）',
  }[status] ?? status
}

function formatTime(value: string | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : value
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

async function loadRunHistory(nextJobs: MonitorJob[]): Promise<void> {
	const nextErrors: Record<string, string> = {}
	const entries = await Promise.all(
		nextJobs.map(async (job) => {
		try {
			return [job.id, await api.fetchMonitorRuns(job.id, 5)] as const
		} catch (error) {
			nextErrors[job.id] = errorMessage(error)
			return [job.id, recentRuns.value[job.id] ?? []] as const
		}
	})
	)
	runErrors.value = nextErrors
	recentRuns.value = Object.fromEntries(entries)
}

async function reload(showSpinner = false): Promise<void> {
  if (refreshInFlight) return
  refreshInFlight = true
  if (showSpinner) loading.value = true
  loadError.value = ''
  try {
    const [nextOptions, nextJobs] = await Promise.all([api.fetchMonitorNodeOptions(), api.fetchMonitorJobs()])
    options.value = nextOptions
    jobs.value = nextJobs

    if (!profileChoices.value.some((profile) => profile.id === profileId.value)) {
      selectProfile(profileChoices.value[0]?.id ?? '')
    } else {
      const visible = new Set(visibleNodes.value.map((node) => node.nodeKey))
      selectedNodeKeys.value = selectedNodeKeys.value.filter((key) => visible.has(key))
    }
    await loadRunHistory(nextJobs)
  } catch (error) {
    loadError.value = errorMessage(error)
  } finally {
    loading.value = false
    refreshInFlight = false
  }
}

async function createJob(): Promise<void> {
  if (!canCreate.value) return
  const nodeContexts = selectedNodeContexts()
  if (prefillActive.value && !nodeContexts) {
    prefillError.value = '工作台选择上下文已失效，请返回工作台重新选择。'
    return
  }
  creating.value = true
  feedback.value = ''
  loadError.value = ''
  try {
    const request = {
      name: name.value.trim(),
      profile_id: profileId.value,
      node_keys: [...selectedNodeKeys.value],
      probe_set: probeSet.value,
      interval_seconds: Number(intervalSeconds.value),
      timeout_seconds: Number(timeoutSeconds.value),
      ...(nodeContexts ? { node_contexts: nodeContexts } : {}),
    }
    const created = await api.createMonitorJob(request)
    feedback.value = `已创建“${created.name}”，当前状态为已停止；请明确点击“启动”。`
    name.value = ''
    selectedNodeKeys.value = []
    prefillActive.value = false
    prefillContexts.value = {}
    await reload()
  } catch (error) {
    loadError.value = `创建失败：${errorMessage(error)}`
  } finally {
    creating.value = false
  }
}

async function applyAction(job: MonitorJob, action: api.MonitorJobAction): Promise<void> {
  if (busyJob.value || (action === 'start' && job.state === 'running') || (action === 'pause' && job.state !== 'running') || (action === 'resume' && job.state !== 'paused')) {
    return
  }
  busyJob.value = { id: job.id, action }
  feedback.value = ''
  try {
    await api.controlMonitorJob(job.id, action)
    await reload()
    feedback.value = `已请求${action === 'start' ? '启动' : action === 'pause' ? '暂停' : action === 'resume' ? '恢复' : '停止'}，列表显示的是后端实际状态。`
  } catch (error) {
    loadError.value = `操作失败：${errorMessage(error)}`
    await reload()
  } finally {
    busyJob.value = null
  }
}

function isBusy(job: MonitorJob, action?: api.MonitorJobAction): boolean {
  return busyJob.value?.id === job.id && (!action || busyJob.value.action === action)
}

function openTimeline(job: MonitorJob, node: MonitorJobNode): void {
  emit('open-timeline', { profileId: job.profileId, node })
}

onMounted(async () => {
  await reload(true)
  if (props.prefill) applyPrefill(props.prefill)
  // Poll only the read model. Leaving this page never calls Stop/Pause.
  refreshTimer = setInterval(() => void reload(), 5000)
})

watch(() => props.prefill, (prefill) => {
  if (prefill && options.value.length > 0) applyPrefill(prefill)
})

onBeforeUnmount(() => {
  if (refreshTimer !== null) clearInterval(refreshTimer)
})
</script>

<template>
  <main class="flex-1 min-h-0 overflow-auto bg-canvas px-4 py-4">
    <div class="max-w-6xl mx-auto flex flex-col gap-4">
      <section class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-base font-semibold text-content-main">持续监测</h2>
          <p class="mt-1 text-xs text-content-muted">
            观察哪些节点、检查什么、每隔多久，以及最近检查和下一次检查。任务定义会保存；重启后只恢复为停止或阻塞，不会自动启动或产生探测。
          </p>
        </div>
        <button
          @click="reload(true)"
          :disabled="loading"
          class="px-3 py-1.5 rounded border border-border text-xs hover:border-brand hover:text-brand disabled:opacity-50"
        >
          {{ loading ? '读取中…' : '重新读取实际状态' }}
        </button>
      </section>

      <div v-if="loadError" class="rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-300">
        {{ loadError }}
      </div>
      <div v-if="feedback" class="rounded border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-700 dark:text-emerald-300">
        {{ feedback }}
      </div>

      <MonitorRetentionPanel />
      <div v-if="prefillError" class="rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-300">
        {{ prefillError }}
        <button type="button" class="ml-2 underline" @click="cancelPrefill">清除这次选择</button>
      </div>

      <section class="rounded-lg border border-border bg-card p-4">
        <div class="flex items-center justify-between gap-2">
          <div>
            <h3 class="text-sm font-semibold">新建持续监测</h3>
            <p class="mt-1 text-[11px] text-content-muted">
              节点来自已缓存订阅的真实配置；前端只提交稳定 node_key，不接触订阅凭据。
            </p>
          </div>
          <span v-if="options.length === 0 && !loading" class="text-[11px] text-amber-700 dark:text-amber-300">暂无可运行节点</span>
        </div>

        <div v-if="prefillActive" class="mt-3 rounded border border-blue-500/30 bg-blue-500/10 px-3 py-2 text-xs text-blue-800 dark:text-blue-200">
          工作台已选择：<strong>{{ profileChoices.find((profile) => profile.id === profileId)?.name || profileId }}</strong> · {{ selectedNodeNames.join('、') }}。确认后只创建已停止任务，不会自动发起探测。
          <button type="button" class="ml-2 underline" :disabled="creating" @click="cancelPrefill">取消本次预填</button>
        </div>

        <div class="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-[240px_1fr_180px_160px_auto]">
          <label class="flex flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">订阅</span>
            <UiSelect
              :model-value="profileId"
              @update:model-value="selectProfile"
              :disabled="profileChoices.length === 0 || creating"
              aria-label="选择订阅"
              :options="[{ value: '', label: '请选择订阅' }, ...profileChoices.map((profile) => ({ value: profile.id, label: `${profile.name || profile.id}（${profile.count} 节点）` }))]"
            />
          </label>

          <div class="flex min-w-0 flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">节点（{{ selectedNodeKeys.length }} 已选）</span>
            <div v-if="visibleNodes.length > 0" class="max-h-40 overflow-auto rounded border border-border bg-card-subtle p-2 space-y-1">
              <label v-for="node in visibleNodes" :key="node.nodeKey" class="flex items-start gap-2 rounded px-1.5 py-1 hover:bg-card">
                <input v-model="selectedNodeKeys" type="checkbox" :value="node.nodeKey" :disabled="creating" class="mt-0.5 accent-blue-600" />
                <span class="min-w-0">
                  <span class="block truncate text-content-main">{{ node.countryFlag }} {{ node.displayName }} <span class="text-content-muted">({{ node.type }})</span></span>
                </span>
              </label>
            </div>
            <span v-else class="rounded border border-dashed border-border px-2 py-3 text-content-muted">先选择有缓存的订阅</span>
          </div>

          <label class="flex flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">探针</span>
            <UiSelect :model-value="probeSet" @update:model-value="setProbeSet" :disabled="creating" aria-label="选择探针" :options="probeOptions" />
          </label>

          <label class="flex flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">间隔</span>
            <UiSelect :model-value="intervalSeconds" @update:model-value="setIntervalSeconds" :disabled="creating" aria-label="选择检查间隔" :options="intervalOptions" />
          </label>

          <label class="flex flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">单节点超时</span>
            <UiSelect :model-value="timeoutSeconds" @update:model-value="setTimeoutSeconds" :disabled="creating" aria-label="选择单节点超时" :options="timeoutOptions" />
          </label>
        </div>

        <div class="mt-3 flex flex-wrap items-end gap-3">
          <label class="flex min-w-[240px] flex-1 flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">任务名称（可选）</span>
            <input v-model="name" :disabled="creating" placeholder="例如：晚高峰稳定性" class="rounded border border-border bg-card-subtle px-2 py-1.5 text-xs" />
          </label>
          <button
            @click="createJob"
            :disabled="!canCreate"
            class="rounded bg-blue-600 px-4 py-1.5 text-xs font-semibold text-white hover:bg-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {{ creating ? '创建中…' : prefillActive ? '确认创建（不会自动启动）' : '创建（不会自动启动）' }}
          </button>
        </div>
      </section>

      <section class="rounded-lg border border-border bg-card p-4">
        <div class="flex items-center justify-between gap-2">
          <h3 class="text-sm font-semibold">正在持续监测</h3>
          <span class="text-[11px] text-content-muted">{{ jobs.length }} 项</span>
        </div>

        <div v-if="jobs.length === 0" class="mt-4 rounded border border-dashed border-border px-4 py-8 text-center text-xs text-content-muted">
          尚未创建任务。创建后仍需明确点击“启动”。
        </div>

        <div v-else class="mt-4 grid grid-cols-1 gap-3 2xl:grid-cols-2">
          <article v-for="job in jobs" :key="job.id" class="rounded border border-border bg-card-subtle p-3">
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0">
                <h4 class="truncate text-sm font-semibold">{{ job.name }}</h4>
                <p class="mt-1 truncate text-[11px] text-content-muted">{{ job.profileName || job.profileId }} · {{ probeLabel(job.probeSet) }} · 每 {{ job.intervalSeconds }} 秒 · 超时 {{ job.timeoutSeconds }} 秒</p>
              </div>
              <span class="rounded-full border px-2 py-0.5 text-[11px]" :class="stateClass(job.state)">{{ formatState(job.state) }}</span>
            </div>

            <div class="mt-3 flex flex-wrap gap-1.5">
              <button v-if="job.state === 'stopped'" @click="applyAction(job, 'start')" :disabled="!!busyJob" class="rounded border border-emerald-500/40 px-2 py-1 text-[11px] text-emerald-700 hover:bg-emerald-500/10 disabled:opacity-50">
                {{ isBusy(job, 'start') ? '启动中…' : '启动' }}
              </button>
              <button v-else-if="job.state === 'running'" @click="applyAction(job, 'pause')" :disabled="!!busyJob" class="rounded border border-amber-500/40 px-2 py-1 text-[11px] text-amber-700 hover:bg-amber-500/10 disabled:opacity-50">
                {{ isBusy(job, 'pause') ? '暂停中…' : '暂停' }}
              </button>
              <button v-else-if="job.state === 'paused'" @click="applyAction(job, 'resume')" :disabled="!!busyJob" class="rounded border border-blue-500/40 px-2 py-1 text-[11px] text-blue-700 hover:bg-blue-500/10 disabled:opacity-50">
                {{ isBusy(job, 'resume') ? '恢复中…' : '恢复' }}
              </button>
              <button @click="applyAction(job, 'stop')" :disabled="job.state === 'stopped' || job.state === 'blocked' || !!busyJob" class="rounded border border-red-500/40 px-2 py-1 text-[11px] text-red-700 hover:bg-red-500/10 disabled:opacity-40">
                {{ isBusy(job, 'stop') ? '停止中…' : '停止' }}
              </button>
            </div>

            <div v-if="job.state === 'blocked'" class="mt-2 text-[11px] text-red-700 dark:text-red-300">
              无法安全恢复：{{ job.blockedReason || '当前订阅或节点配置已不可用，请重新创建任务。' }}
            </div>

            <div v-if="job.persistenceState === 'degraded'" class="mt-2 rounded border border-red-500/30 bg-red-500/10 px-2 py-1.5 text-[11px] text-red-700 dark:text-red-300">
              持久化异常：本轮测速结果可能未完整保存，不能作为完整 durable evidence。{{ job.persistenceError }}
            </div>
            <div v-if="job.storageState === 'storage_protected'" class="mt-2 rounded border border-amber-500/40 bg-amber-500/10 px-2 py-1.5 text-[11px] text-amber-800">
              容量保护：{{ job.storageReason }}。这段时间未采集，不代表节点失败。
            </div>

            <div class="mt-3 border-t border-border pt-2">
              <div class="mb-1 text-[11px] font-medium text-content-secondary">节点与时间轴</div>
              <div class="space-y-1">
                <div v-for="node in job.nodes" :key="node.nodeKey" class="flex flex-wrap items-center justify-between gap-2 text-[11px]">
                  <span class="min-w-0 truncate text-content-main">{{ node.displayName }} <span class="text-content-muted">({{ node.type }})</span></span>
                  <button @click="openTimeline(job, node)" class="shrink-0 text-brand hover:underline">查看历史记录</button>
                </div>
              </div>
            </div>

            <div class="mt-3 border-t border-border pt-2">
              <div class="mb-1 text-[11px] font-medium text-content-secondary">最近运行结果</div>
              <div v-if="runErrors[job.id]" class="text-[11px] text-red-600 dark:text-red-400">读取失败：{{ runErrors[job.id] }}</div>
              <div v-else-if="(recentRuns[job.id] ?? []).length === 0" class="text-[11px] text-content-muted">暂无实际运行记录；启动后首轮会立即按现有 Scheduler 语义执行。</div>
              <div v-else class="space-y-1">
                <div v-for="run in recentRuns[job.id]" :key="run.runId" class="flex flex-wrap items-center justify-between gap-2 text-[11px]">
                  <span class="text-content-muted">{{ formatTime(run.startedAt) }}</span>
                  <span :class="run.status === 'completed' ? 'text-emerald-700 dark:text-emerald-300' : run.status === 'running' ? 'text-blue-700 dark:text-blue-300' : 'text-amber-700 dark:text-amber-300'">{{ runLabel(run.status) }}</span>
                  <span class="text-content-muted">{{ run.successNodes }}/{{ run.totalNodes }} 节点成功</span>
                  <span v-if="run.errorMessage" class="max-w-full truncate text-red-600 dark:text-red-400" :title="run.errorMessage">{{ run.errorMessage }}</span>
                </div>
              </div>
            </div>
          </article>
        </div>
      </section>
    </div>
  </main>
</template>
