<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { fetchSettings, saveSettings } from '../../api/bridge'
import { applyMonitorRetention, fetchMonitorStorageUsage, previewMonitorRetention } from '../../api/monitor'
import type { AppSettings, MonitorRetentionPolicy, MonitorRetentionPreview, MonitorStorageUsage } from '../../types'

const settings = ref<AppSettings | null>(null)
const storage = ref<MonitorStorageUsage | null>(null)
const selectedPolicy = ref<MonitorRetentionPolicy>('keep_all')
const customDays = ref(90)
const preview = ref<MonitorRetentionPreview | null>(null)
const confirmed = ref(false)
const busy = ref(false)
const error = ref('')
const result = ref('')
let timer: ReturnType<typeof setInterval> | null = null

const savedPolicy = computed(() => settings.value?.monitor_retention_policy || 'keep_all')
const hasUnsavedPreference = computed(() => selectedPolicy.value !== savedPolicy.value ||
  (selectedPolicy.value === 'custom' && customDays.value !== (settings.value?.monitor_retention_custom_days || 0)))
const validCustomDays = computed(() => selectedPolicy.value !== 'custom' || (Number.isInteger(customDays.value) && customDays.value >= 1 && customDays.value <= 36500))

function messageFor(errorValue: unknown): string {
  return errorValue instanceof Error ? errorValue.message : String(errorValue || '操作失败')
}

function formatBytes(bytes: number): string {
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`
}

function changePolicy(event: Event): void {
  selectedPolicy.value = (event.target as HTMLSelectElement).value as MonitorRetentionPolicy
  preview.value = null
  confirmed.value = false
}

function changeDays(event: Event): void {
  customDays.value = Number((event.target as HTMLInputElement).value)
  preview.value = null
  confirmed.value = false
}

async function refreshStorage(): Promise<void> {
  try { storage.value = await fetchMonitorStorageUsage() }
  catch (cause) { error.value = `读取 Monitor 容量失败：${messageFor(cause)}` }
}

async function load(): Promise<void> {
  try {
    const loaded = await fetchSettings()
    settings.value = loaded
    selectedPolicy.value = loaded.monitor_retention_policy || 'keep_all'
    customDays.value = loaded.monitor_retention_custom_days || 90
    await refreshStorage()
  } catch (cause) { error.value = `读取保留设置失败：${messageFor(cause)}` }
}

async function savePreference(): Promise<void> {
  if (!settings.value || !validCustomDays.value || busy.value) return
  busy.value = true
  error.value = ''
  result.value = ''
  preview.value = null
  try {
    const next: AppSettings = {
      ...settings.value,
      monitor_retention_policy: selectedPolicy.value,
      monitor_retention_custom_days: selectedPolicy.value === 'custom' ? customDays.value : 0,
    }
    await saveSettings(next)
    settings.value = next
    result.value = '保留偏好已保存。保存设置不会删除历史；删除需另行预览并确认。'
  } catch (cause) { error.value = `保存偏好失败：${messageFor(cause)}` }
  finally { busy.value = false }
}

async function previewDeletion(): Promise<void> {
  if (!settings.value || hasUnsavedPreference.value || savedPolicy.value === 'keep_all' || busy.value) return
  busy.value = true
  error.value = ''
  result.value = ''
  confirmed.value = false
  try {
    const next = await previewMonitorRetention({
      policy: savedPolicy.value,
      ...(savedPolicy.value === 'custom' ? { custom_days: settings.value.monitor_retention_custom_days } : {}),
    })
    preview.value = next
    storage.value = next.storage
  } catch (cause) { preview.value = null; error.value = `预览失败：${messageFor(cause)}` }
  finally { busy.value = false }
}

async function executeDeletion(): Promise<void> {
  if (!preview.value || !confirmed.value || busy.value) return
  busy.value = true
  error.value = ''
  result.value = ''
  try {
    const applied = await applyMonitorRetention({
      policy: preview.value.policy,
      cutoff_time: preview.value.cutoff,
      ...(preview.value.policy === 'custom' ? { custom_days: settings.value?.monitor_retention_custom_days } : {}),
    })
    result.value = applied.partial
      ? `部分删除：${applied.samples_deleted} 条 Monitor 样本、${applied.runs_deleted} 条 run；请检查结果。`
      : `已删除 ${applied.samples_deleted} 条 Monitor 样本、${applied.runs_deleted} 条孤儿 run。`
  } catch (cause) {
    const detail = messageFor(cause)
    error.value = detail.includes('部分') || detail.includes('partially applied') ? `部分删除：${detail}` : `删除失败：${detail}`
  } finally {
    preview.value = null
    confirmed.value = false
    await refreshStorage()
    busy.value = false
  }
}

onMounted(() => {
  void load()
  timer = setInterval(() => void refreshStorage(), 5000)
})
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <section class="rounded-lg border border-border bg-card p-4 text-xs" aria-label="Monitor 历史保留与容量">
    <h3 class="text-sm font-semibold">Monitor 历史保留与容量</h3>
    <p class="mt-1 text-content-muted">当前保留偏好：{{ savedPolicy }}。仅作用于 Monitor raw 样本及符合条件的孤儿 run；Workbench 历史、任务定义、legacy JSON 和旧迁移来源不在删除范围内。</p>

    <div v-if="storage" class="mt-3 text-content-secondary">
      SQLite 文件占用 {{ formatBytes(storage.total_bytes) }}（history.db {{ formatBytes(storage.database_bytes) }}、WAL {{ formatBytes(storage.wal_bytes) }}、shm {{ formatBytes(storage.shared_memory_bytes) }}）。
      产品告警阈值 {{ formatBytes(storage.warning_bytes) }}，后台采集保护阈值 {{ formatBytes(storage.hard_bytes) }}；它们不是 SQLite 容量上限。
      <span v-if="storage.protected" class="block mt-1 text-red-700">容量保护已生效：新的 Monitor 后台轮次暂停。不会自动删除 raw，也不会把未采集记为节点失败。</span>
      <span v-else-if="storage.warning" class="block mt-1 text-amber-700">容量已接近保护阈值，请检查可用空间和保留偏好。</span>
    </div>

    <div class="mt-3 flex flex-wrap items-end gap-3">
      <label class="grid gap-1">保留偏好
        <select :value="selectedPolicy" :disabled="busy" class="rounded border border-border bg-card px-2 py-1" @change="changePolicy">
          <option value="keep_all">全部保留</option><option value="30d">30 天</option><option value="90d">90 天</option><option value="180d">180 天</option><option value="custom">自定义</option>
        </select>
      </label>
      <label v-if="selectedPolicy === 'custom'" class="grid gap-1">天数
        <input :value="customDays" type="number" min="1" max="36500" :disabled="busy" class="w-24 rounded border border-border bg-card px-2 py-1" @input="changeDays" />
      </label>
      <button type="button" :disabled="busy || !settings || !hasUnsavedPreference || !validCustomDays" class="rounded border border-border px-3 py-1.5 disabled:opacity-50" @click="savePreference">保存偏好（不删除）</button>
      <button type="button" :disabled="busy || !settings || hasUnsavedPreference || savedPolicy === 'keep_all'" class="rounded border border-border px-3 py-1.5 disabled:opacity-50" @click="previewDeletion">预览 Monitor raw 删除</button>
    </div>

    <div v-if="preview" class="mt-3 rounded border border-amber-500/40 bg-amber-500/10 p-3">
      <p>删除预览（按当前 SQLite 数据）：cutoff {{ new Date(preview.cutoff).toLocaleString() }} 之前的 Monitor 样本 {{ preview.samples_to_delete }} 条、符合条件的孤儿 run {{ preview.runs_to_delete }} 条。</p>
      <p class="mt-1">仅影响 Monitor 历史；Workbench、任务定义、legacy JSON 不受影响。删除 raw 历史不能自动恢复，删除后数据库文件也不一定立即缩小。</p>
      <label class="mt-2 flex items-center gap-2"><input v-model="confirmed" type="checkbox" :disabled="busy" />我确认删除预览范围内的 Monitor raw 历史</label>
      <button type="button" :disabled="!confirmed || busy" class="mt-2 rounded border border-amber-700 px-3 py-1.5 disabled:opacity-50" @click="executeDeletion">确认删除 Monitor raw</button>
    </div>
    <p v-if="result" class="mt-2 text-emerald-700" role="status">{{ result }}</p>
    <p v-if="error" class="mt-2 text-red-700" role="alert">{{ error }}</p>
  </section>
</template>
