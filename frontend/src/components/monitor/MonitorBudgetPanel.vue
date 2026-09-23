<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { fetchSettings, saveSettings } from '../../api/bridge'
import { fetchMonitorBudgetStatus } from '../../api/monitor'
import type { AppSettings, MonitorBudgetStatus } from '../../types'

const settings = ref<AppSettings | null>(null)
const status = ref<MonitorBudgetStatus | null>(null)
const concurrent = ref(4)
const requests = ref(20000)
const dailyMiB = ref(32)
const responseKiB = ref(256)
const busy = ref(false)
const error = ref('')
const feedback = ref('')
let timer: ReturnType<typeof setInterval> | null = null
const mib = 1024 * 1024
const kib = 1024

const valid = computed(() => Number.isInteger(concurrent.value) && concurrent.value >= 1 && concurrent.value <= 64 &&
  Number.isInteger(requests.value) && requests.value >= 1 && requests.value <= 1_000_000_000 &&
  Number.isInteger(dailyMiB.value) && dailyMiB.value >= 1 && dailyMiB.value <= 1_048_576 &&
  Number.isInteger(responseKiB.value) && responseKiB.value >= 1 && responseKiB.value <= 16_384 &&
  responseKiB.value * kib <= dailyMiB.value * mib)
const changed = computed(() => concurrent.value !== (settings.value?.monitor_budget_max_concurrent ?? 4) ||
  requests.value !== (settings.value?.monitor_budget_daily_requests ?? 20000) ||
  dailyMiB.value * mib !== (settings.value?.monitor_budget_daily_bytes ?? 32 * mib) ||
  responseKiB.value * kib !== (settings.value?.monitor_budget_response_bytes ?? 256 * kib))

function message(cause: unknown): string { return cause instanceof Error ? cause.message : String(cause) }
function readInput(field: 'concurrent' | 'requests' | 'daily' | 'response', event: Event): void {
  const value = Number((event.target as HTMLInputElement).value)
  if (field === 'concurrent') concurrent.value = value
  else if (field === 'requests') requests.value = value
  else if (field === 'daily') dailyMiB.value = value
  else responseKiB.value = value
}

async function refresh(): Promise<void> {
  try { status.value = await fetchMonitorBudgetStatus(); error.value = '' }
  catch (cause) { error.value = `读取 Monitor 预算失败：${message(cause)}` }
}

async function load(): Promise<void> {
  try {
    settings.value = await fetchSettings()
    concurrent.value = settings.value.monitor_budget_max_concurrent ?? 4
    requests.value = settings.value.monitor_budget_daily_requests ?? 20000
    dailyMiB.value = (settings.value.monitor_budget_daily_bytes ?? 32 * mib) / mib
    responseKiB.value = (settings.value.monitor_budget_response_bytes ?? 256 * kib) / kib
    await refresh()
  } catch (cause) { error.value = `读取 Monitor 预算设置失败：${message(cause)}` }
}

async function save(): Promise<void> {
  if (!settings.value || !valid.value || !changed.value || busy.value) return
  busy.value = true
  error.value = ''
  feedback.value = ''
  try {
    const next: AppSettings = {
      ...settings.value,
      monitor_budget_max_concurrent: concurrent.value,
      monitor_budget_daily_requests: requests.value,
      monitor_budget_daily_bytes: dailyMiB.value * mib,
      monitor_budget_response_bytes: responseKiB.value * kib,
    }
    await saveSettings(next)
    settings.value = next
    await refresh()
    feedback.value = '预算已保存。已用额度不会清零，也不会立即启动或触发探测。'
  } catch (cause) { error.value = `保存 Monitor 预算失败：${message(cause)}` }
  finally { busy.value = false }
}

onMounted(() => { void load(); timer = setInterval(() => void refresh(), 5000) })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <section class="rounded-lg border border-border bg-card p-4 text-xs" aria-label="Monitor 全局预算">
    <h3 class="text-sm font-semibold">Monitor 全局预算与错峰</h3>
    <p class="mt-1 text-content-muted">同一应用内所有 Monitor 任务及手动立即执行共用额度。Workbench 即时测速不包含在内。请求额度在发送前扣除，失败请求及每次重定向仍计数；响应体只统计本应用实际读取的字节，不代表代理、TLS 或系统总流量。</p>
    <div v-if="status" class="mt-2 text-content-secondary">
      今日（UTC {{ status.usage.utc_day }}）请求 {{ status.usage.requests_used }} / {{ status.limits.daily_requests }}，响应体读取 {{ (status.usage.bytes_used / mib).toFixed(2) }} / {{ (status.limits.daily_bytes / mib).toFixed(2) }} MiB；当前并发 {{ status.active_requests }} / {{ status.limits.max_concurrent }}。下次重置 {{ status.reset_at }}（UTC）。
      <p v-if="status.blocked_reason" class="mt-1 text-amber-700">{{ status.blocked_reason }}。未采集不是节点失败。</p>
    </div>
    <p class="mt-1 text-content-muted">HTTP 最多重定向 3 次，单请求最多 30 秒；明确选择的重型探测并发最多 1。到期错峰、有界等待，过期周期跳过不补跑。</p>
    <div class="mt-3 flex flex-wrap items-end gap-3">
      <label class="grid gap-1">最大并发<input :value="concurrent" type="number" min="1" max="64" step="1" :disabled="busy" class="w-24 rounded border border-border bg-card px-2 py-1" @input="readInput('concurrent', $event)" /></label>
      <label class="grid gap-1">每日请求数<input :value="requests" type="number" min="1" max="1000000000" step="1" :disabled="busy" class="w-28 rounded border border-border bg-card px-2 py-1" @input="readInput('requests', $event)" /></label>
      <label class="grid gap-1">每日读取（MiB）<input :value="dailyMiB" type="number" min="1" max="1048576" step="1" :disabled="busy" class="w-28 rounded border border-border bg-card px-2 py-1" @input="readInput('daily', $event)" /></label>
      <label class="grid gap-1">单响应读取上限（KiB）<input :value="responseKiB" type="number" min="1" max="16384" step="1" :disabled="busy" class="w-28 rounded border border-border bg-card px-2 py-1" @input="readInput('response', $event)" /></label>
      <button type="button" :disabled="busy || !settings || !valid || !changed" class="rounded border border-border px-3 py-1.5 disabled:opacity-50" @click="save">保存 Monitor 预算</button>
    </div>
    <p v-if="!valid" class="mt-1 text-red-700">预算必须是有效正整数，且单响应上限不超过每日读取额度。</p>
    <p v-if="feedback" class="mt-2 text-emerald-700" role="status">{{ feedback }}</p>
    <p v-if="error" class="mt-2 text-red-700" role="alert">{{ error }}</p>
  </section>
</template>
