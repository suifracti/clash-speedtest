<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as api from '../../api/monitor'
import UiSelect from '../common/UiSelect.vue'
import MonitorRetentionPanel from './MonitorRetentionPanel.vue'
import MonitorBudgetPanel from './MonitorBudgetPanel.vue'
import MonitorTransfer from './MonitorTransfer.vue'
import { isNoticeNode, loadNoticeOverrides } from '../../utils/nodeFilter'
import { buildLogicalProfileChoices, logicalChoiceForProfile, type LogicalProfileChoice } from '../../utils/logicalProfiles'
import type { MonitorJob, MonitorJobNode, MonitorJobPrefill, MonitorNodeOption, MonitorNodeSelectionContext, MonitorRun, MonitorSamplingTier, NodeDetailRequest } from '../../types'

const props = defineProps<{ prefill?: MonitorJobPrefill | null }>()

const emit = defineEmits<{
  (event: 'open-timeline', payload: { profileId: string; node: MonitorJobNode }): void
  (event: 'open-node-detail', payload: NodeDetailRequest): void
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
const triggeringJobId = ref('')
const samplingTierDrafts = ref<Record<string, MonitorSamplingTier>>({})
const savingSamplingTierJobId = ref('')
const savingRecoveryPreferenceJobId = ref('')
const deletingJobId = ref('')
const pendingDeleteJob = ref<MonitorJob | null>(null)

const name = ref('')
const profileId = ref('')
const profileIds = ref<string[]>([])
const nodeSearch = ref('')
const region = ref('all')
const hideNotices = ref(true)
const noticeOverrides = loadNoticeOverrides()
const selectedNodeKeys = ref<string[]>([])
const probeSet = ref<'light' | 'service' | 'heavy'>('light')
const extraProbes = ref<('light' | 'service' | 'heavy')[]>([])
const samplingTier = ref<'regular' | 'focus' | 'sparse'>('regular')
const intervalSeconds = ref(60)
const intervalManuallySet = ref(false)
const timeoutSeconds = ref(10)
const prefillActive = ref(false)
const prefillError = ref('')
const prefillContexts = ref<Record<string, MonitorNodeSelectionContext>>({})

let refreshTimer: ReturnType<typeof setInterval> | null = null
let refreshInFlight = false

const profileChoices = computed(() => {
  return buildLogicalProfileChoices(options.value, node => !isNoticeNode(node, noticeOverrides, selectionKey(node)))
})
const selectedLogicalProfileId = computed(() => logicalChoiceForProfile(profileChoices.value, profileId.value)?.id || profileId.value)

function selectionKey(node: MonitorNodeOption) { return `${node.profileId}\u0000${node.nodeKey}` }
const scopedNodes = computed(() => options.value.filter(node => profileIds.value.includes(node.profileId)))
const regions = computed(() => [...new Set(scopedNodes.value.map(node => node.countryCode || '未知'))].sort())
const visibleNodes = computed(() => scopedNodes.value.filter(node =>
  (!hideNotices.value || !isNoticeNode(node, noticeOverrides, selectionKey(node))) &&
  (region.value === 'all' || (node.countryCode || '未知') === region.value) &&
  `${node.displayName} ${node.profileName}`.toLowerCase().includes(nodeSearch.value.trim().toLowerCase())
).sort((a, b) => (a.countryCode || 'ZZ').localeCompare(b.countryCode || 'ZZ') || a.displayName.localeCompare(b.displayName)))
function selectVisible() { selectedNodeKeys.value = [...new Set([...selectedNodeKeys.value, ...visibleNodes.value.map(selectionKey)])] }
watch(profileIds, () => {
  const available = new Set(scopedNodes.value.map(selectionKey))
  selectedNodeKeys.value = selectedNodeKeys.value.filter(key => available.has(key))
}, { deep: true })
const canCreate = computed(
  () =>
    !creating.value &&
    !prefillError.value &&
    profileIds.value.length > 0 &&
    selectedNodeKeys.value.length > 0 &&
    intervalSeconds.value >= 1 &&
    timeoutSeconds.value >= 1
)
const selectedNodeNames = computed(() => selectedNodeKeys.value
  .map((key) => scopedNodes.value.find((node) => selectionKey(node) === key)?.displayName || key)
  .filter(Boolean))

const probeOptions = [
  { value: 'light', label: '线路基础检查 · 延迟与轻量请求' },
  { value: 'service', label: '预设服务检查 · 固定目标' },
  { value: 'heavy', label: '深入检查 · 额外请求与流量' },
]
const intervalOptions = [
  { value: 10, label: '10 秒' },
  { value: 30, label: '30 秒' },
  { value: 60, label: '1 分钟' },
  { value: 120, label: '2 分钟' },
  { value: 300, label: '5 分钟' },
  { value: 900, label: '15 分钟' },
  { value: 1800, label: '30 分钟' },
]
const samplingTierOptions = [
  { value: 'regular', label: '日常观察' },
  { value: 'focus', label: '重点关注 · 建议每 2 分钟' },
  { value: 'sparse', label: '低频观察 · 建议每 30 分钟' },
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
  profileIds.value = profileId.value ? logicalChoiceForProfile(profileChoices.value, profileId.value)?.profileIds || [profileId.value] : []
  const visible = new Set(scopedNodes.value.map(selectionKey))
  selectedNodeKeys.value = selectedNodeKeys.value.filter((key) => visible.has(key))
}

function toggleProfileGroup(profile: LogicalProfileChoice): void {
  const next = new Set(profileIds.value)
  const selected = profile.profileIds.every(id => next.has(id))
  for (const id of profile.profileIds) selected ? next.delete(id) : next.add(id)
  profileIds.value = [...next]
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
  profileIds.value = [prefill.profileId]
  selectedNodeKeys.value = requestedKeys.map(key => `${prefill.profileId}\u0000${key}`)
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
  const currentByKey = new Map(options.value.map((node) => [selectionKey(node), contextFromOption(node)]))
  const contexts = selectedNodeKeys.value.map((key) => prefillContexts.value[key.split('\u0000')[1]] || currentByKey.get(key))
  return contexts.every((context): context is MonitorNodeSelectionContext => !!context) ? contexts : undefined
}

function setProbeSet(value: string | number): void {
  if (value === 'light' || value === 'service' || value === 'heavy') probeSet.value = value
}

function setSamplingTier(value: string | number): void {
  if (value !== 'regular' && value !== 'focus' && value !== 'sparse') return
  samplingTier.value = value
  if (!intervalManuallySet.value) {
    intervalSeconds.value = value === 'focus' ? 120 : value === 'sparse' ? 1800 : 60
  }
}

function setIntervalSeconds(value: string | number): void {
  const next = Number(value)
  if (Number.isFinite(next)) {
    intervalSeconds.value = next
    intervalManuallySet.value = true
  }
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

function samplingTierLabel(tier: MonitorJob['samplingTier']): string {
  return { regular: 'regular 常规', focus: 'focus 关注', sparse: 'sparse 低频' }[tier] ?? tier
}

function recoveryStatus(job: MonitorJob): string {
  if (job.recoveryState === 'blocked') return `恢复被阻塞：${job.recoveryReason || job.blockedReason || '需要处理阻塞条件'}`
  if (job.recoveryState === 'restoring') return '本次启动正在等待共享预算准入'
  if (job.recoveryState === 'restored') return '本次启动已恢复；首轮只测当前时刻'
  if (job.desiredState === 'paused') return '上次已暂停，不会自动恢复'
  if (job.desiredState === 'stopped') return '上次已停止，不会自动恢复'
  if (!job.resumeOnLaunch) return '应用启动时恢复未启用'
  if (job.state === 'running') return '本进程正在运行；应用重开时允许恢复'
  return '已允许应用启动恢复，等待明确运行意图'
}

function jobSamplingTierDraft(job: MonitorJob): MonitorSamplingTier {
  return samplingTierDrafts.value[job.id] ?? job.samplingTier
}

function setJobSamplingTierDraft(job: MonitorJob, value: string | number): void {
  const tier = String(value)
  if (tier !== 'regular' && tier !== 'focus' && tier !== 'sparse') return
  samplingTierDrafts.value = { ...samplingTierDrafts.value, [job.id]: tier }
}

function runSourceLabel(run: MonitorRun): string {
  const tier = run.samplingTier === 'legacy_unknown' ? '旧来源未知' : run.samplingTier
  const trigger = run.triggerType === 'manual' ? '手动触发' : run.triggerType === 'scheduled' ? '周期触发' : '旧触发未知'
  return `${tier} · ${trigger}`
}

function runLabel(status: MonitorRun['status']): string {
  return {
    running: '执行中',
    completed: '完成',
    partial_failed: '部分失败',
    failed: '失败',
    skipped: '跳过（重叠）',
    resource_limited: '资源额度限制（未计为节点失败）',
    persistence_failed: '保存失败（未持久化完整）',
    interrupted: '应用未观测期间中断',
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

    if (!profileChoices.value.some((profile) => profile.profileIds.includes(profileId.value))) {
      selectProfile(profileChoices.value[0]?.id ?? '')
    } else {
      const visible = new Set(scopedNodes.value.map(selectionKey))
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
  let createdCount = 0
  try {
    const chosen = scopedNodes.value.filter(node => selectedNodeKeys.value.includes(selectionKey(node)))
    for (const selectedProfile of profileIds.value) {
    const nodes = chosen.filter(node => node.profileId === selectedProfile)
    if (!nodes.length) continue
    for (const probe of [...new Set([probeSet.value, ...extraProbes.value])]) {
    const request = {
      name: name.value.trim(),
      profile_id: selectedProfile,
      node_keys: nodes.map(node => node.nodeKey),
      probe_set: probe,
      sampling_tier: samplingTier.value,
      interval_seconds: Number(intervalSeconds.value),
      timeout_seconds: Number(timeoutSeconds.value),
      node_contexts: nodes.map(node => prefillActive.value && node.profileId === profileId.value ? prefillContexts.value[node.nodeKey] || contextFromOption(node) : contextFromOption(node)),
    }
    const created = await api.createMonitorJob(request)
    createdCount++
    feedback.value = `已创建“${created.name}”，当前状态为已停止；请明确点击“启动”。`
    }
    // Successfully created groups are removed so retry does not duplicate them.
    selectedNodeKeys.value = selectedNodeKeys.value.filter(key => !nodes.some(node => selectionKey(node) === key))
    }
    feedback.value = `已创建 ${createdCount} 项周期任务，均未启动；点击任务上的“启动”后按间隔检测。`
    name.value = ''
    selectedNodeKeys.value = []
    prefillActive.value = false
    prefillContexts.value = {}
    await reload()
  } catch (error) {
    if (createdCount) { selectedNodeKeys.value = []; await reload() }
    loadError.value = `创建失败：${errorMessage(error)}${createdCount ? `。已创建 ${createdCount} 项，请先查看任务列表，勿重复提交。` : ''}`
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

async function saveJobSamplingTier(job: MonitorJob): Promise<void> {
  const tier = jobSamplingTierDraft(job)
  if (tier === job.samplingTier || savingSamplingTierJobId.value || busyJob.value) return
  savingSamplingTierJobId.value = job.id
  feedback.value = ''
  loadError.value = ''
  try {
    await api.updateMonitorJobSamplingTier(job.id, tier)
    const drafts = { ...samplingTierDrafts.value }
    delete drafts[job.id]
    samplingTierDrafts.value = drafts
    await reload()
    feedback.value = `已保存“${job.name}”的 ${samplingTierLabel(tier)} 层级；任务仍为${formatState(job.state)}，未启动或恢复周期。`
  } catch (error) {
    loadError.value = `保存层级失败：${errorMessage(error)}`
  } finally {
    savingSamplingTierJobId.value = ''
  }
}

async function setResumeOnLaunch(job: MonitorJob, event: Event): Promise<void> {
  if (busyJob.value || savingRecoveryPreferenceJobId.value) return
  const enabled = (event.target as HTMLInputElement).checked
  savingRecoveryPreferenceJobId.value = job.id
  feedback.value = ''
  loadError.value = ''
  try {
    await api.updateMonitorJobResumeOnLaunch(job.id, enabled)
    await reload()
    feedback.value = `已保存“${job.name}”的应用启动恢复设置；此设置不会启动或停止当前任务。`
  } catch (error) {
    loadError.value = `恢复设置保存失败：${errorMessage(error)}`
    await reload()
  } finally {
    savingRecoveryPreferenceJobId.value = ''
  }
}

async function deleteJob(job: MonitorJob): Promise<void> {
  if (busyJob.value || deletingJobId.value) return
  pendingDeleteJob.value = null
  deletingJobId.value = job.id
  feedback.value = ''
  loadError.value = ''
  try {
    await api.deleteMonitorJob(job.id)
    await reload()
    feedback.value = `已删除任务“${job.name}”；历史运行和原始样本仍保留。`
  } catch (error) {
    loadError.value = `删除任务失败：${errorMessage(error)}`
    await reload()
  } finally {
    deletingJobId.value = ''
  }
}

async function triggerDiagnostic(job: MonitorJob): Promise<void> {
  if ((job.state !== 'running' && job.state !== 'paused') || busyJob.value || triggeringJobId.value) return
  triggeringJobId.value = job.id
  feedback.value = ''
  loadError.value = ''
  try {
    const run = await api.triggerMonitorJob(job.id)
    await reload()
    feedback.value = `已完成一次 diagnostic 手动诊断（${run.runId}）；任务仍为${formatState(job.state)}，周期设置未改变。`
  } catch (error) {
    loadError.value = `手动诊断失败：${errorMessage(error)}`
    await reload()
  } finally {
    triggeringJobId.value = ''
  }
}

function isBusy(job: MonitorJob, action?: api.MonitorJobAction): boolean {
  return busyJob.value?.id === job.id && (!action || busyJob.value.action === action)
}

function openTimeline(job: MonitorJob, node: MonitorJobNode): void {
  emit('open-timeline', { profileId: job.profileId, node })
}

function openNodeDetail(job: MonitorJob, node: MonitorJobNode): void {
  emit('open-node-detail', {
    profileId: job.profileId,
    profileName: job.profileName,
    nodeKey: node.nodeKey,
    nodeIdentityKey: node.nodeIdentityKey,
    configRevisionKey: node.configRevisionKey,
    displayName: node.displayName,
    nodeType: node.type,
  })
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
  <main class="monitor-workspace">
    <div class="monitor-layout">
      <section class="monitor-heading flex flex-wrap items-start justify-between gap-3">
        <div>
          <p class="monitor-eyebrow">自动观察 / 定时任务</p>
          <h2 class="text-base font-semibold text-content-main">持续监测</h2>
          <p class="mt-1 text-xs text-content-muted">
            定时检查节点，记录连接状态和变化。任务由你手动启用。
          </p>
        </div>
        <button
          @click="reload(true)"
          :disabled="loading"
          class="px-3 py-1.5 rounded border border-border text-xs hover:border-brand hover:text-brand disabled:opacity-50"
        >
          {{ loading ? '读取中…' : '刷新状态' }}
        </button>
      </section>

      <section class="monitor-pulse" aria-label="监测概览"><div class="pulse-orbit" aria-hidden="true">◷</div><div><strong>{{ jobs.filter(j => j.state === 'running').length }} <small>项正在监测</small></strong><p>{{ jobs.length ? '任务只有在后台程序运行时才会执行，关闭网页不影响。' : '还没有自动检查任务。从下面选择节点、检查内容与间隔。' }}</p></div><div class="pulse-secondary"><b>{{ jobs.filter(j => j.state === 'blocked').length }}</b><span>项需要处理</span></div><div class="pulse-secondary"><b>{{ jobs.length }}</b><span>项任务总计</span></div></section>

      <div v-if="loadError" class="rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-300">
        {{ loadError }}
      </div>
      <div v-if="feedback" class="rounded border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-700 dark:text-emerald-300">
        {{ feedback }}
      </div>

      <details class="monitor-settings">
        <summary><strong>流量保护与记录保存</strong><span>避免后台请求过多或记录占满磁盘</span></summary>
        <div class="monitor-settings-body">
          <MonitorBudgetPanel />
          <MonitorRetentionPanel />
          <p class="text-xs text-content-muted">仅明确启用“应用启动时恢复”、且此前运行的周期任务可恢复；暂停、停止或保护阻塞时不会恢复。恢复继续消耗预算，手动诊断不会自动恢复。</p>
        </div>
      </details>
      <MonitorTransfer style="order: 4" @imported="reload()" />
      <div v-if="prefillError" class="rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-300">
        {{ prefillError }}
        <button type="button" class="ml-2 underline" @click="cancelPrefill">清除这次选择</button>
      </div>

      <section class="monitor-create rounded-lg border border-border bg-card p-4">
        <div class="flex items-center justify-between gap-2">
          <div>
            <h3 class="text-sm font-semibold">新建监测任务</h3>
            <p class="mt-1 text-[11px] text-content-muted">
              选择节点来源与节点，再设置检查频率；节点完全相同的订阅会自动合并。
            </p>
          </div>
          <span v-if="options.length === 0 && !loading" class="text-[11px] text-amber-700 dark:text-amber-300">暂无可运行节点</span>
        </div>

        <div v-if="prefillActive" class="mt-3 rounded border border-blue-500/30 bg-blue-500/10 px-3 py-2 text-xs text-blue-800 dark:text-blue-200">
          工作台已选择：<strong>{{ profileChoices.find((profile) => profile.profileIds.includes(profileId))?.name || profileId }}</strong> · {{ selectedNodeNames.join('、') }}。确认后只创建已停止任务，不会自动发起探测。
          <button type="button" class="ml-2 underline" :disabled="creating" @click="cancelPrefill">取消本次预填</button>
        </div>

        <div class="monitor-create-grid">
          <div class="flex flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">① 从哪里选节点</span>
            <UiSelect
              :model-value="selectedLogicalProfileId"
              @update:model-value="selectProfile"
              :disabled="profileChoices.length === 0 || creating"
              aria-label="选择节点来源"
              :options="[{ value: '', label: '请选择节点来源' }, ...profileChoices.map((profile) => ({ value: profile.id, label: `${profile.name || profile.id}（${profile.count} 条线路${profile.configCount > profile.count ? ` · ${profile.configCount} 个配置` : ''}${profile.mergedSourceCount > 1 ? ` · ${profile.mergedSourceCount} 个订阅` : ''}）` }))]"
            />
            <details v-if="profileChoices.length > 1" class="profile-multiselect"><summary>组合多个独立节点来源（{{ profileIds.length }} 份订阅）</summary><label v-for="profile in profileChoices" :key="profile.id"><input type="checkbox" :value="profile.id" :checked="profile.profileIds.every(id => profileIds.includes(id))" :disabled="creating" @change="toggleProfileGroup(profile)" />{{ profile.name }} · {{ profile.count }} 条线路<span v-if="profile.configCount > profile.count"> · {{ profile.configCount }} 个配置</span><span v-if="profile.mergedSourceCount > 1"> · {{ profile.mergedSourceCount }} 份订阅归为一组</span></label></details>
          </div>

          <div class="flex min-w-0 flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">节点（{{ selectedNodeKeys.length }} 已选）</span>
            <section class="node-filter-controls"><input v-model="nodeSearch" placeholder="搜索节点" aria-label="搜索监测节点" /><UiSelect v-model="region" aria-label="筛选地区" :options="[{ value: 'all', label: '全部地区' }, ...regions.map(code => ({ value: code, label: code }))]" /><label><input v-model="hideNotices" type="checkbox" />过滤公告</label><button type="button" :disabled="creating" @click="selectVisible">全选当前 {{ visibleNodes.length }} 个</button><button type="button" :disabled="creating" @click="selectedNodeKeys = []">清空</button></section>
            <div v-if="visibleNodes.length > 0" class="node-picker max-h-40 overflow-auto rounded border border-border bg-card-subtle p-2 space-y-1">
              <label v-for="node in visibleNodes" :key="selectionKey(node)" class="flex items-start gap-2 rounded px-1.5 py-1 hover:bg-card" :title="`${node.profileName} · ${node.displayName}`">
                <input v-model="selectedNodeKeys" type="checkbox" :value="selectionKey(node)" :disabled="creating" class="mt-0.5" />
                <span class="min-w-0">
                  <span class="block truncate text-content-main">{{ node.countryFlag }} {{ node.displayName }} <span class="text-content-muted">({{ node.type }})</span></span>
                </span>
              </label>
            </div>
            <span v-else class="rounded border border-dashed border-border px-2 py-3 text-content-muted">没有匹配的节点，请调整订阅或筛选。</span>
          </div>

          <label class="flex flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">② 检查什么</span>
            <UiSelect :model-value="probeSet" @update:model-value="setProbeSet" :disabled="creating" aria-label="选择探针" :options="probeOptions" />
            <div class="extra-probes"><span>同时添加</span><label v-for="probe in probeOptions.filter(item => item.value !== probeSet)" :key="probe.value"><input v-model="extraProbes" type="checkbox" :value="probe.value" :disabled="creating" />{{ probe.label }}</label></div>
          </label>

          <label class="flex flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">观察方式</span>
            <UiSelect :model-value="samplingTier" @update:model-value="setSamplingTier" :disabled="creating" aria-label="选择周期采样层级" :options="samplingTierOptions" />
            <span class="text-[10px] text-content-muted">仅适用于新建任务；建议间隔可自行调整。诊断只通过手动触发记录。</span>
          </label>

          <label class="flex flex-col gap-1 text-xs">
            <span class="font-medium text-content-secondary">③ 多久检查一次</span>
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
            class="monitor-create-button rounded px-4 py-1.5 text-xs font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50"
          >
            {{ creating ? '创建中…' : prefillActive ? '确认创建（不会自动启动）' : '创建（不会自动启动）' }}
          </button>
        </div>
      </section>

      <section class="monitor-tasks rounded-lg border border-border bg-card p-4">
        <div class="flex items-center justify-between gap-2">
          <h3 class="text-sm font-semibold">监测任务</h3>
          <span class="text-[11px] text-content-muted">{{ jobs.length }} 项</span>
        </div>

        <div v-if="jobs.length === 0" class="mt-4 rounded border border-dashed border-border px-4 py-8 text-center text-xs text-content-muted">
          没有运行中的检查。创建任务后，点击“启动”才会开始消耗流量。
        </div>

        <div v-else class="mt-4 grid grid-cols-1 gap-3 2xl:grid-cols-2">
          <article v-for="job in jobs" :key="job.id" class="rounded border border-border bg-card-subtle p-3">
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0">
                <h4 class="truncate text-sm font-semibold">{{ job.name }}</h4>
                <p class="mt-1 truncate text-[11px] text-content-muted">{{ job.profileName || job.profileId }} · {{ samplingTierLabel(job.samplingTier) }} · {{ probeLabel(job.probeSet) }} · 每 {{ job.intervalSeconds }} 秒 · 超时 {{ job.timeoutSeconds }} 秒</p>
              </div>
              <span class="rounded-full border px-2 py-0.5 text-[11px]" :class="stateClass(job.state)">{{ formatState(job.state) }}</span>
            </div>

            <div class="mt-2 rounded border border-border bg-card px-2 py-2 text-[11px] text-content-secondary">
              <div>{{ recoveryStatus(job) }}</div>
              <div class="mt-1">运行意图：{{ formatState(job.desiredState) }} · 本进程状态：{{ formatState(job.runtimeState) }}</div>
              <label class="mt-2 flex items-start gap-2 text-content-main">
                <input
                  type="checkbox"
                  :checked="job.resumeOnLaunch"
                  :disabled="!!busyJob || !!savingRecoveryPreferenceJobId"
                  @change="setResumeOnLaunch(job, $event)"
                  class="mt-0.5"
                />
                <span>
                  <span class="font-medium">应用启动时恢复</span>
                  <span class="block text-[10px] text-content-muted">只保存许可，不会立即启动；只恢复此前明确运行的周期任务。</span>
                </span>
              </label>
              <div v-if="job.intentPersistenceError" class="mt-1 text-red-700 dark:text-red-300">{{ job.intentPersistenceError }}</div>
            </div>

            <div class="mt-3 flex flex-wrap gap-1.5">
              <button v-if="job.state === 'stopped' || job.state === 'blocked'" @click="applyAction(job, 'start')" :disabled="!!busyJob" class="rounded border border-emerald-500/40 px-2 py-1 text-[11px] text-emerald-700 hover:bg-emerald-500/10 disabled:opacity-50">
                {{ isBusy(job, 'start') ? '启动中…' : job.state === 'blocked' ? '重新检查并启动' : '启动' }}
              </button>
              <button v-else-if="job.state === 'running'" @click="applyAction(job, 'pause')" :disabled="!!busyJob" class="rounded border border-amber-500/40 px-2 py-1 text-[11px] text-amber-700 hover:bg-amber-500/10 disabled:opacity-50">
                {{ isBusy(job, 'pause') ? '暂停中…' : '暂停' }}
              </button>
              <button v-else-if="job.state === 'paused'" @click="applyAction(job, 'resume')" :disabled="!!busyJob" class="rounded border border-blue-500/40 px-2 py-1 text-[11px] text-blue-700 hover:bg-blue-500/10 disabled:opacity-50">
                {{ isBusy(job, 'resume') ? '恢复中…' : '恢复' }}
              </button>
              <button @click="applyAction(job, 'stop')" :disabled="job.desiredState === 'stopped' && job.state !== 'running' && job.state !== 'paused' || !!busyJob" class="rounded border border-red-500/40 px-2 py-1 text-[11px] text-red-700 hover:bg-red-500/10 disabled:opacity-40">
                {{ isBusy(job, 'stop') ? '停止中…' : '停止' }}
              </button>
              <button @click="pendingDeleteJob = job" :disabled="!!busyJob || !!deletingJobId" class="rounded border border-border px-2 py-1 text-[11px] text-content-secondary hover:bg-card disabled:opacity-40">
                {{ deletingJobId === job.id ? '删除中…' : '删除任务' }}
              </button>
              <button
                v-if="job.state === 'running' || job.state === 'paused'"
                @click="triggerDiagnostic(job)"
                :disabled="!!busyJob || !!triggeringJobId"
                class="rounded border border-violet-500/40 px-2 py-1 text-[11px] text-violet-700 hover:bg-violet-500/10 disabled:opacity-50"
              >
                {{ triggeringJobId === job.id ? '诊断中…' : '手动诊断（一次）' }}
              </button>
            </div>

            <div class="mt-2 flex flex-wrap items-center gap-2">
              <label class="text-[11px] text-content-secondary" :for="`sampling-tier-${job.id}`">周期层级</label>
              <UiSelect
                :id="`sampling-tier-${job.id}`"
                :model-value="jobSamplingTierDraft(job)"
                @update:model-value="setJobSamplingTierDraft(job, $event)"
                :disabled="!!busyJob || !!savingSamplingTierJobId"
                :aria-label="`${job.name} 的周期采样层级`"
                :options="samplingTierOptions"
              />
              <button
                @click="saveJobSamplingTier(job)"
                :disabled="jobSamplingTierDraft(job) === job.samplingTier || !!busyJob || !!savingSamplingTierJobId"
                class="rounded border border-border px-2 py-1 text-[11px] text-content-secondary hover:bg-card disabled:opacity-40"
              >
                {{ savingSamplingTierJobId === job.id ? '保存中…' : '保存层级' }}
              </button>
            </div>

            <div v-if="job.state === 'blocked' || job.recoveryState === 'blocked'" class="mt-2 text-[11px] text-red-700 dark:text-red-300">
              无法安全恢复：{{ job.recoveryReason || job.blockedReason || '当前订阅、节点配置或资源条件不可用，请处理后明确点击启动。' }}
            </div>

            <div v-if="job.persistenceState === 'degraded'" class="mt-2 rounded border border-red-500/30 bg-red-500/10 px-2 py-1.5 text-[11px] text-red-700 dark:text-red-300">
              持久化异常：本轮测速结果可能未完整保存，不能作为完整 durable evidence。{{ job.persistenceError }}
            </div>
            <div v-if="job.storageState === 'storage_protected'" class="mt-2 rounded border border-amber-500/40 bg-amber-500/10 px-2 py-1.5 text-[11px] text-amber-800">
              容量保护：{{ job.storageReason }}。这段时间未采集，不代表节点失败。
            </div>
            <div v-if="job.budgetState && job.budgetState !== 'ok'" class="mt-2 rounded border border-amber-500/40 bg-amber-500/10 px-2 py-1.5 text-[11px] text-amber-800">
              Monitor 预算／调度：{{ job.budgetReason }}。未执行的周期不计为节点失败。
            </div>
            <p v-if="job.skippedRounds || job.resourceSkippedRounds" class="mt-1 text-[11px] text-content-muted">跳过周期 {{ job.skippedRounds || 0 }}；资源／等待跳过 {{ job.resourceSkippedRounds || 0 }}</p>

            <div class="mt-3 border-t border-border pt-2">
              <div class="mb-1 text-[11px] font-medium text-content-secondary">节点与时间轴</div>
              <div class="space-y-1">
                <div v-for="node in job.nodes" :key="node.nodeKey" class="flex flex-wrap items-center justify-between gap-2 text-[11px]">
                  <span class="min-w-0 truncate text-content-main">{{ node.displayName }} <span class="text-content-muted">({{ node.type }})</span></span>
                  <div class="flex shrink-0 items-center gap-3"><button @click="openNodeDetail(job, node)" :disabled="!node.nodeIdentityKey || !node.configRevisionKey" class="text-brand hover:underline disabled:opacity-50">节点详情</button><button @click="openTimeline(job, node)" class="text-brand hover:underline">查看历史记录</button></div>
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
                  <span class="rounded border border-border px-1 py-0.5 text-content-secondary">{{ runSourceLabel(run) }}</span>
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
  <div v-if="pendingDeleteJob" class="monitor-confirm-backdrop">
    <section class="monitor-confirm-dialog" role="alertdialog" aria-modal="true" aria-label="删除监测任务">
      <span>任务管理</span><h3>删除“{{ pendingDeleteJob.name }}”？</h3>
      <p>这会移除定时任务，但保留历史运行和原始样本。以后不会继续自动监测这项任务。</p>
      <div><button type="button" @click="pendingDeleteJob = null">取消</button><button type="button" class="danger" @click="deleteJob(pendingDeleteJob)">删除任务</button></div>
    </section>
  </div>
</template>

<style scoped>
.monitor-layout { display: flex; flex-direction: column; gap: 20px; }
.monitor-heading { padding: 14px 0 8px; }
.monitor-eyebrow { margin: 0 0 8px; color: var(--primary); font-size: 11px; font-weight: 700; letter-spacing: .12em; }
.monitor-heading h2 { font-size: 26px; letter-spacing: -.03em; margin-bottom: 8px; }
.monitor-heading button { background: var(--card); min-height: 34px; }
.monitor-tasks { order: 1; }
.monitor-create { order: 2; }
.monitor-settings { order: 3; border: 1px solid var(--border); border-radius: 12px; background: var(--card); }
.monitor-settings summary { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 18px 20px; cursor: pointer; font-size: 13px; }
.monitor-settings summary::before { content: '+'; color: var(--primary); font-size: 18px; }
.monitor-settings[open] summary::before { content: '−'; }
.monitor-settings summary span { color: var(--text-secondary); font-size: 12px; }
.monitor-settings-body { display: grid; gap: 14px; padding: 0 20px 20px; }
.monitor-create, .monitor-tasks { padding: 22px; border-radius: 12px; }
.monitor-create-grid { margin-top: 20px; display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 18px; align-items: start; }
.monitor-create-grid > :first-child { grid-column: span 1; }
.monitor-create-grid > :nth-child(2) { grid-column: span 3; }
.node-picker { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 4px; }
.node-filter-controls { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; margin: 5px 0; }
.node-filter-controls > input, .node-filter-controls select { background: var(--card); border: 1px solid var(--border); border-radius: 5px; padding: 6px; min-width: 0; }
.node-filter-controls button { color: var(--primary); }
.profile-multiselect { margin-top: 8px; padding: 10px; background: var(--card-subtle); border-radius: 6px; }
.profile-multiselect summary { cursor: pointer; color: var(--primary); }
.profile-multiselect label, .extra-probes label { display: flex; gap: 6px; padding-top: 8px; }
.extra-probes { font-size: 11px; color: var(--text-secondary); }
.monitor-create { border-top: 3px solid #5475ac; }
.monitor-tasks { border-top: 3px solid #4a8870; }
.monitor-create-button { background: var(--primary); min-height: 34px; }
.monitor-confirm-backdrop { position: fixed; inset: 0; z-index: 70; display: grid; place-items: center; padding: 18px; background: rgba(14,25,35,.55); }
.monitor-confirm-dialog { width: min(100%, 460px); padding: 24px; border: 1px solid var(--border); border-radius: 14px; background: var(--card); box-shadow: 0 24px 70px rgba(0,0,0,.23); }
.monitor-confirm-dialog>span { color: var(--primary); font-size: 11px; font-weight: 750; }
.monitor-confirm-dialog h3 { margin: 9px 0; font-size: 20px; font-weight: 750; }
.monitor-confirm-dialog p { color: var(--text-secondary); font-size: 13px; line-height: 1.7; }
.monitor-confirm-dialog>div { display: flex; justify-content: flex-end; gap: 9px; margin-top: 20px; }
.monitor-confirm-dialog button { padding: 8px 13px; border: 1px solid var(--border); border-radius: 7px; }
.monitor-confirm-dialog button.danger { border-color: var(--danger); background: var(--danger); color: white; }
@media (max-width: 850px) {
  .monitor-create-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .monitor-create-grid > :first-child, .monitor-create-grid > :nth-child(2) { grid-column: 1 / -1; }
  .monitor-create, .monitor-tasks { padding: 16px; }
}
@media (max-width: 480px) { .monitor-create-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
