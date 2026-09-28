<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'
import { fetchMonitorNodeOptions } from '../../api/monitor'
import { extractNoticeLinks, isNoticeNode } from '../../utils/nodeFilter'
import type { Airport, MonitorNodeOption, Subscription } from '../../types'
import AirportMaintenancePanel from './AirportMaintenancePanel.vue'

const store = useWorkbenchStore()
const props = withDefaults(defineProps<{ embedded?: boolean; visible?: boolean }>(), { embedded: false })
const emit = defineEmits<{ (event: 'browse-profile', id: string): void }>()
const selectedAirportID = ref('')
const attentionOnly = ref(false)
const showReadAttention = ref(false)
const attentionStorageKey = 'speedtest.subscription-attention-read.v1'
type ReadAttention = { reason: string; remaining: number; total: number; expire: number }
function readAttentionState(): Record<string, ReadAttention> {
  try { const value = JSON.parse(localStorage.getItem(attentionStorageKey) || '{}'); return value && typeof value === 'object' && !Array.isArray(value) ? value : {} } catch { return {} }
}
const readAttention = ref<Record<string, ReadAttention>>(readAttentionState())
const readAttentionCount = computed(() => allEntries().filter(({ sub }) => attentionReason(sub) && isAttentionRead(sub)).length)
const displayedAttention = computed(() => allEntries().filter(({ sub }) => attentionReason(sub) && isAttentionRead(sub) === showReadAttention.value))
function attentionKey(sub: Subscription) { return `${sub.airport_id}:${sub.id}` }
function isAttentionRead(sub: Subscription) {
  const saved = readAttention.value[attentionKey(sub)]
  const reason = attentionReason(sub)
  if (!saved || !reason) return false
  const sameReason = reason.startsWith('仅剩 ') && saved.reason.startsWith('仅剩 ') || saved.reason === reason
  return sameReason && (sub.usage?.total || 0) === saved.total && (sub.usage?.expire || 0) === saved.expire && (saved.remaining <= 0 || remaining(sub) > saved.remaining / 2)
}
function markAttention(sub: Subscription, read: boolean) {
  const next = { ...readAttention.value }
  if (read) next[attentionKey(sub)] = { reason: attentionReason(sub), remaining: remaining(sub), total: sub.usage?.total || 0, expire: sub.usage?.expire || 0 }
  else delete next[attentionKey(sub)]
  readAttention.value = next
  try { localStorage.setItem(attentionStorageKey, JSON.stringify(next)) } catch { feedback.value = '已读状态仅在本次打开期间有效，浏览器未允许保存。' }
  if (attentionOnly.value && !attentionCount.value) attentionOnly.value = false
}
const nodeOptions = ref<MonitorNodeOption[]>([])
const selectedAirport = computed(() => visibleAirports.value.find(ap => ap.id === selectedAirportID.value) || visibleAirports.value[0])
const detailAirports = computed(() => props.embedded ? (selectedAirport.value ? [selectedAirport.value] : []) : visibleAirports.value)
const attentionCount = computed(() => allEntries().filter(({ sub }) => needsAttention(sub)).length)
function attentionReason(sub: Subscription) {
  if (sub.status === 'error') return '无可用节点'
  if (!sub.has_cache) return '尚未缓存节点'
  return usageWarning(sub)
}
function needsAttention(sub: Subscription) { return !!attentionReason(sub) && !isAttentionRead(sub) }
function airportAttentionSummary(ap: Airport) { return [...new Set((ap.subscriptions || []).map(attentionReason).filter(Boolean))].join('、') }
function selectAirport(id: string) { if (!busy.value) { resetForms(); selectedAirportID.value = id } }

// State modes: 'list' | 'add_airport' | 'edit_airport' | 'add_sub' | 'edit_sub'
const mode = ref<'list' | 'add_airport' | 'edit_airport' | 'add_sub' | 'edit_sub'>('list')

// Selected / Editing contexts
const targetAirport = ref<Airport | null>(null)
const editingSub = ref<Subscription | null>(null)

// Airport form fields
const airportName = ref('')
const airportWebsite = ref('')
const airportBackup = ref('')
const airportNote = ref('')
// Optional initial subscription fields when creating airport
const initialSubName = ref('')
const initialSubUrl = ref('')

// Subscription form fields
const subAlias = ref('')
const subUrl = ref('')
const subNote = ref('')

const isSubmitting = ref(false)
const isLoadingURL = ref(false)
const errorMessage = ref('')
const search = ref('')
const refreshBusy = ref(false)
const refreshDone = ref(0)
const refreshTotal = ref(0)
const refreshResults = ref<Record<string, { state: 'queued' | 'running' | 'success' | 'error'; message: string }>>({})
const feedback = ref('')
const visibleAirports = computed(() => store.airports.filter(ap =>
  `${ap.name} ${ap.website_url || ''} ${ap.note || ''} ${(ap.subscriptions || []).map(s => s.name).join(' ')}`.toLowerCase().includes(search.value.trim().toLowerCase())
  && (!attentionOnly.value || (ap.subscriptions || []).some(needsAttention))))
const subscriptionCount = computed(() => store.airports.reduce((n, ap) => n + (ap.subscriptions?.length || 0), 0))
const busy = computed(() => isSubmitting.value || refreshBusy.value)
watch(selectedAirport, ap => { if (ap) selectedAirportID.value = ap.id })
watch(() => props.visible, visible => {
  if (visible === false && !busy.value) resetForms()
  if (visible === true && !busy.value) { void store.loadAirports(); void loadNodeOptions() }
})
watch(() => store.isAirportModalOpen, visible => { if (visible) void loadNodeOptions() }, { immediate: true })
let statusPollBusy = false
const statusPoll = setInterval(async () => {
  if (!props.visible || busy.value || statusPollBusy || mode.value !== 'list') return
  statusPollBusy = true
  try { await store.loadAirports() } finally { statusPollBusy = false }
}, 30000)
onBeforeUnmount(() => clearInterval(statusPoll))
let requestID = 0
async function loadNodeOptions() {
  try { nodeOptions.value = await fetchMonitorNodeOptions() } catch { nodeOptions.value = [] }
}
function noticeLabel(text: string, url: string, fallback: string) {
  const cleaned = text.replace(url, '').replace(/[：:\s·—-]+$/g, '').trim()
  return cleaned && cleaned.length <= 80 ? cleaned : fallback.replace(/^打开\s*/, '')
}
function discoveredLinks(ap: Airport) {
  const profileIDs = new Set((ap.subscriptions || []).map(sub => sub.id))
  const found = new Map<string, { label: string; url: string }>()
  for (const node of nodeOptions.value) {
    if (!profileIDs.has(node.profileId) || !isNoticeNode(node)) continue
    for (const link of extractNoticeLinks(node.displayName)) {
      if (!found.has(link.url)) found.set(link.url, { label: noticeLabel(node.displayName, link.url, link.label), url: link.url })
    }
  }
  return Array.from(found.values())
}

function resetForms() {
  requestID += 1
  mode.value = 'list'
  targetAirport.value = null
  editingSub.value = null
  airportName.value = ''
  airportWebsite.value = ''
  airportBackup.value = ''
  airportNote.value = ''
  initialSubName.value = ''
  initialSubUrl.value = ''
  subAlias.value = ''
  subUrl.value = ''
  subNote.value = ''
  isLoadingURL.value = false
  errorMessage.value = ''
}

function closeModal() {
  if (busy.value) return
  store.isAirportModalOpen = false
  resetForms()
}

// Open Airport creation
function openAddAirport() {
  if (busy.value) return
  resetForms()
  mode.value = 'add_airport'
}

// Open Airport edit
function openEditAirport(ap: Airport) {
  if (busy.value) return
  resetForms()
  targetAirport.value = ap
  airportName.value = ap.name
  airportWebsite.value = ap.website_url || ''
  airportBackup.value = ap.backup_url || ''
  airportNote.value = ap.note || ''
  mode.value = 'edit_airport'
}

// Open Subscription creation under an airport
function openAddSub(ap: Airport) {
  if (busy.value) return
  resetForms()
  targetAirport.value = ap
  subAlias.value = ''
  subUrl.value = ''
  subNote.value = ''
  mode.value = 'add_sub'
}

// Open Subscription edit
async function openEditSub(ap: Airport, sub: Subscription) {
  if (busy.value) return
  const currentReq = ++requestID
  targetAirport.value = ap
  editingSub.value = sub
  subAlias.value = sub.name
  subNote.value = sub.note || ''
  subUrl.value = ''
  mode.value = 'edit_sub'
  isLoadingURL.value = true
  errorMessage.value = ''
  try {
    const fullURL = await api.getSubscriptionURL(sub.id, ap.id)
    if (currentReq !== requestID) return
    subUrl.value = fullURL
  } catch (e: any) {
    if (currentReq !== requestID) return
    errorMessage.value = e.message || '读取完整订阅链接失败'
  } finally {
    if (currentReq === requestID) isLoadingURL.value = false
  }
}

// Save Airport (Create or Update)
async function handleSaveAirport() {
  if (busy.value) return
  errorMessage.value = ''
  if (!airportName.value.trim()) {
    errorMessage.value = '机场主体名称不能为空'
    return
  }

  isSubmitting.value = true
  try {
    if (mode.value === 'edit_airport' && targetAirport.value) {
      await api.updateAirport(
        targetAirport.value.id,
        airportName.value.trim(),
        undefined,
        airportWebsite.value.trim(),
        airportBackup.value.trim(),
        airportNote.value.trim()
      )
    } else {
      await api.createAirport(
        airportName.value.trim(),
        initialSubUrl.value.trim() || undefined,
        airportWebsite.value.trim(),
        airportBackup.value.trim(),
        airportNote.value.trim(),
        initialSubName.value.trim() || undefined
      )
    }
    await store.loadProfileSetup()
    await store.loadAirports()
    resetForms()
  } catch (e: any) {
    errorMessage.value = e.message || '保存机场失败'
  } finally {
    isSubmitting.value = false
  }
}

// Save Subscription (Create or Update)
async function handleSaveSub() {
  if (busy.value) return
  if (!targetAirport.value) return
  errorMessage.value = ''
  if (!subAlias.value.trim() || !subUrl.value.trim()) {
    errorMessage.value = '订阅别名和订阅链接不能为空'
    return
  }

  isSubmitting.value = true
  try {
    if (mode.value === 'edit_sub' && editingSub.value) {
      await api.updateSubscription(
        targetAirport.value.id,
        editingSub.value.id,
        subAlias.value.trim(),
        subUrl.value.trim(),
        subNote.value.trim()
      )
    } else {
      await api.addSubscription(
        targetAirport.value.id,
        subAlias.value.trim(),
        subUrl.value.trim(),
        subNote.value.trim()
      )
    }
    await store.loadProfileSetup()
    await store.loadAirports()
    resetForms()
  } catch (e: any) {
    errorMessage.value = e.message || '保存订阅失败'
  } finally {
    isSubmitting.value = false
  }
}

// Delete Airport
async function handleDeleteAirport(ap: Airport) {
  if (busy.value) return
  if (confirm(`确定删除机场“${ap.name}”及其下属的所有订阅链接吗？此操作不可撤销。`)) {
    try {
      await api.deleteAirport(ap.id)
      await store.loadProfileSetup()
      await store.loadAirports()
    } catch (e: any) {
      alert(e.message || '删除机场失败')
    }
  }
}

// Refresh entire Airport (all subscriptions)
async function handleRefreshAirport(id: string) {
  const ap = store.airports.find(a => a.id === id)
  if (ap) await refreshEntries((ap.subscriptions || []).map(sub => ({ ap, sub })))
}

// Delete Subscription
async function handleDeleteSub(ap: Airport, sub: Subscription) {
  if (busy.value) return
  if (confirm(`确定删除“${ap.name}”下的订阅“${sub.name}”吗？`)) {
    try {
      await api.deleteSubscription(ap.id, sub.id)
      await store.loadProfileSetup()
      await store.loadAirports()
    } catch (e: any) {
      alert(e.message || '删除订阅失败')
    }
  }
}

// Refresh Subscription
async function handleRefreshSub(ap: Airport, sub: Subscription) {
  await refreshEntries([{ ap, sub }])
}

function subKey(ap: Airport, sub: Subscription) { return `${ap.id}:${sub.id}` }
function allEntries() { return store.airports.flatMap(ap => (ap.subscriptions || []).map(sub => ({ ap, sub }))) }
async function refreshEntries(entries: { ap: Airport; sub: Subscription }[]) {
  if (busy.value || !entries.length) return
  refreshBusy.value = true
  refreshDone.value = 0
  refreshTotal.value = entries.length
  feedback.value = ''
  refreshResults.value = Object.fromEntries(entries.map(({ ap, sub }) => [subKey(ap, sub), { state: 'queued', message: '等待刷新' }]))
  let failed = 0
  try {
    for (const { ap, sub } of entries) {
      const key = subKey(ap, sub)
      refreshResults.value[key] = { state: 'running', message: '正在获取节点与用量…' }
      try {
        const updated = await api.refreshSubscription(ap.id, sub.id)
        const current = store.airports.find(a => a.id === ap.id)
        const index = current?.subscriptions?.findIndex(s => s.id === sub.id) ?? -1
        if (current?.subscriptions && index >= 0) {
          current.subscriptions[index] = updated
          current.node_count = current.subscriptions.reduce((n, s) => n + s.node_count, 0)
        }
        refreshResults.value[key] = { state: 'success', message: `已更新 · ${updated.node_count} 个节点${updated.usage ? ' · 用量已同步' : ' · 未提供有效用量'}` }
      } catch {
        failed++
        refreshResults.value[key] = { state: 'error', message: '刷新失败，未确认更新成功；可重试或检查订阅地址' }
      }
      refreshDone.value++
    }
    feedback.value = failed ? `刷新结束：${entries.length - failed} 成功，${failed} 失败` : `已更新 ${entries.length} 个订阅`
  } finally { refreshBusy.value = false }
}
function retryFailed() {
  return refreshEntries(allEntries().filter(({ ap, sub }) => refreshResults.value[subKey(ap, sub)]?.state === 'error'))
}
const hasRefreshErrors = computed(() => Object.values(refreshResults.value).some(r => r.state === 'error'))
function safeWebsite(raw?: string) {
  try { const url = new URL(raw || ''); return ['https:', 'http:'].includes(url.protocol) && !url.username && !url.password ? url.href : undefined } catch { return undefined }
}
function websiteHost(raw?: string) { try { return new URL(raw || '').host } catch { return '' } }
function bytes(value: number) {
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MiB`
  return `${(value / 1024 ** 3).toFixed(2)} GiB`
}
function used(sub: Subscription) { return sub.usage ? sub.usage.upload + sub.usage.download : 0 }
function usagePercent(sub: Subscription) { return sub.usage?.total ? Math.min(100, used(sub) / sub.usage.total * 100) : 0 }
function remaining(sub: Subscription) { return sub.usage?.total ? Math.max(0, sub.usage.total - used(sub)) : 0 }
function usageWarning(sub: Subscription) {
  if (sub.usage?.expire && sub.usage.expire * 1000 <= Date.now()) return '已到期'
  if (sub.usage?.total && used(sub) >= sub.usage.total) return '流量已用尽'
  if (sub.usage?.expire && sub.usage.expire * 1000 - Date.now() <= 7 * 86400000) return '7 天内到期'
  if (sub.usage?.total && usagePercent(sub) >= 90 && remaining(sub) <= 20 * 1024 ** 3) return `仅剩 ${bytes(remaining(sub))}`
  return ''
}
async function copySubscription(ap: Airport, sub: Subscription) {
  try {
    const url = await api.getSubscriptionURL(sub.id, ap.id)
    await navigator.clipboard.writeText(url)
    feedback.value = '完整订阅地址已复制，请勿分享给他人'
  } catch { feedback.value = '复制失败，请通过编辑订阅查看地址' }
}

function formatDate(iso?: string): string {
  if (!iso) return '未刷新'
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() <= 1970) return '未拉取'
  return d.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

</script>

<template>
  <div
    v-if="embedded || store.isAirportModalOpen"
    :class="embedded ? 'airport-workspace' : 'fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none'"
  >
    <div class="subscription-manager prototype-modal w-full flex flex-col overflow-hidden" :class="{ 'max-h-[88vh]': !embedded }" :role="embedded ? 'region' : 'dialog'" :aria-modal="embedded ? undefined : true" aria-label="机场与订阅">
      <!-- Modal Header -->
      <div class="prototype-modal-header flex items-center justify-between p-4 border-b border-border bg-card-subtle">
        <div class="flex items-center gap-2.5">
          <span class="manager-mark" aria-hidden="true">↗</span>
          <div>
            <h2 class="manager-title">机场与订阅</h2>
            <p class="manager-subtitle">管理入口、套餐与节点来源</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button
            v-if="mode === 'list'"
            :disabled="busy"
            type="button"
            class="manager-add"
            @click="openAddAirport"
          >
            <span>+</span> 添加机场
          </button>
          <button
            v-if="!embedded"
            type="button"
            class="prototype-close-btn font-mono text-content-muted hover:text-content-main p-1 rounded"
            aria-label="关闭"
            :disabled="busy"
            @click="closeModal"
          >
            ✕
          </button>
        </div>
      </div>

      <div v-if="embedded" class="manager-overview">
        <div><span>机场</span><strong>{{ store.airports.length }}</strong></div>
        <div><span>订阅</span><strong>{{ subscriptionCount }}</strong></div>
        <button type="button" :aria-pressed="attentionOnly" :title="attentionCount ? '包括：无可用节点、尚未缓存、7 天内到期、已到期、流量用尽，或剩余不超过 20 GiB 且低于套餐 10%' : '当前没有需要处理的订阅'" @click="attentionOnly = !attentionOnly"><span>需要关注</span><strong :class="{ warning: attentionCount }">{{ attentionCount }}<small> 条订阅</small></strong></button>
        <p>用量来自机场提供的快照<br>刷新订阅时同步更新，不合并共享套餐额度</p>
      </div>

      <section v-if="embedded && (attentionCount || readAttentionCount)" class="attention-inbox" aria-label="需要处理的订阅">
        <div class="attention-heading"><span class="attention-icon">{{ attentionCount ? '!' : '✓' }}</span><div><h2>{{ attentionCount ? `${attentionCount} 条订阅需要你看一眼` : '当前提醒均已读' }}</h2><p>每次标为已读后，剩余流量降至当时的一半再提醒；出现新问题也会提醒。</p></div><button @click="showReadAttention = !showReadAttention">{{ showReadAttention ? `未读（${attentionCount}）` : `查看已读（${readAttentionCount}）` }}</button><button @click="attentionOnly = !attentionOnly">{{ attentionOnly ? '返回全部订阅' : '只看需处理订阅' }}</button></div>
        <div class="attention-items"><div v-for="entry in displayedAttention" :key="attentionKey(entry.sub)" class="attention-entry"><button class="attention-open" :disabled="busy" @click="attentionOnly = !showReadAttention; selectAirport(entry.ap.id)"><span><strong>{{ entry.ap.name }} · {{ entry.sub.name }}</strong><small>{{ attentionReason(entry.sub) }}</small></span><b>查看套餐 →</b></button><button class="attention-read" :disabled="busy" @click="markAttention(entry.sub, !showReadAttention)">{{ showReadAttention ? '标为未读' : '标为已读' }}</button></div><p v-if="!displayedAttention.length" class="attention-empty">{{ showReadAttention ? '暂无已读提醒' : '暂无未读提醒' }}</p></div>
      </section>

      <div class="manager-layout">
      <aside v-if="embedded" class="airport-browser" aria-label="机场列表">
        <input v-model="search" class="prototype-input" placeholder="查找机场或订阅" aria-label="查找机场或订阅">
        <div class="airport-browser-caption"><span>{{ attentionOnly ? '需关注的机场' : '我的机场' }}</span><button v-if="attentionOnly" type="button" @click="attentionOnly = false">显示全部</button></div>
        <button v-for="ap in visibleAirports" :key="ap.id" type="button" class="airport-browser-item" :aria-pressed="selectedAirport?.id === ap.id" :disabled="busy" @click="selectAirport(ap.id)">
          <span class="airport-monogram">{{ ap.name.slice(0, 1) }}</span><span><strong>{{ ap.name }}</strong><small>{{ ap.subscriptions?.length || 0 }} 条订阅 · {{ ap.node_count }} 节点<span v-if="airportAttentionSummary(ap)"> · {{ airportAttentionSummary(ap) }}</span></small></span><i v-if="ap.subscriptions?.some(needsAttention)" :title="airportAttentionSummary(ap)">!</i>
        </button>
        <p v-if="!visibleAirports.length" class="usage-note">没有符合条件的机场</p>
      </aside>

      <!-- Modal Body -->
      <div class="prototype-modal-body flex-1 overflow-y-auto p-4 flex flex-col gap-4">
        <!-- 1. Form: Add / Edit Airport Brand -->
        <div
          v-if="mode === 'add_airport' || mode === 'edit_airport'"
          class="bg-card-subtle p-4 rounded-lg border border-primary/30 flex flex-col gap-3.5 shadow-sm"
        >
          <div class="flex items-center justify-between">
            <span class="font-bold text-content-main text-xs flex items-center gap-1.5">
              <span class="w-2 h-2 rounded-full bg-primary"></span>
              {{ mode === 'edit_airport' ? `编辑机场主体信息【${targetAirport?.name}】` : '添加新机场主体 / 品牌' }}
            </span>
            <button type="button" class="text-content-muted hover:text-content-main text-[11px]" @click="resetForms">
              返回列表
            </button>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label class="block text-[11px] font-semibold text-content-secondary mb-1">机场名称 <span class="text-red-500">*</span></label>
              <input
                v-model="airportName"
                placeholder="例如: 飞鸟云 / 蓝岸 / 专线机场"
                class="prototype-input w-full"
              />
            </div>
            <div>
              <label class="block text-[11px] font-semibold text-content-secondary mb-1">机场备注 (选填)</label>
              <input
                v-model="airportNote"
                placeholder="如: 年付套餐 / 常用主力 / 香港专线"
                class="prototype-input w-full"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label class="block text-[11px] font-semibold text-content-secondary mb-1">官网链接 (选填)</label>
              <input
                v-model="airportWebsite"
                placeholder="https://..."
                class="prototype-input w-full font-mono text-[11px]"
              />
            </div>
            <div>
              <label class="block text-[11px] font-semibold text-content-secondary mb-1">防失联发布页链接 (选填)</label>
              <input
                v-model="airportBackup"
                placeholder="https://... 或 GitHub 发布页"
                class="prototype-input w-full font-mono text-[11px]"
              />
            </div>
          </div>

          <!-- Initial Subscription for Add Airport Mode -->
          <div v-if="mode === 'add_airport'" class="pt-3 border-t border-border flex flex-col gap-2.5">
            <div class="flex items-center justify-between">
              <span class="text-[11px] font-semibold text-content-main">初始订阅链接 (选填，也可稍后在机场下添加)</span>
            </div>
            <div class="grid grid-cols-1 md:grid-cols-3 gap-2">
              <input
                v-model="initialSubName"
                placeholder="订阅别名 (默认: 默认订阅)"
                class="prototype-input"
              />
              <input
                v-model="initialSubUrl"
                placeholder="订阅链接 (HTTP URL 或 本地路径)"
                class="prototype-input md:col-span-2 font-mono text-[11px]"
              />
            </div>
          </div>

          <div v-if="errorMessage" class="text-red-500 text-[11px] bg-red-500/10 p-2 rounded border border-red-500/20">
            {{ errorMessage }}
          </div>

          <div class="flex justify-end gap-2 pt-1">
            <button
              type="button"
              class="tool-button"
              @click="resetForms"
            >
              取消
            </button>
            <button
              type="button"
              :disabled="isSubmitting"
              class="prototype-btn-primary"
              @click="handleSaveAirport"
            >
              {{ isSubmitting ? '保存中...' : mode === 'edit_airport' ? '保存机场' : '创建机场' }}
            </button>
          </div>
        </div>

        <!-- 2. Form: Add / Edit Subscription -->
        <div
          v-else-if="mode === 'add_sub' || mode === 'edit_sub'"
          class="bg-card-subtle p-4 rounded-lg border border-primary/30 flex flex-col gap-3.5 shadow-sm"
        >
          <div class="flex items-center justify-between">
            <span class="font-bold text-content-main text-xs flex items-center gap-1.5">
              <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
              {{ mode === 'edit_sub' ? `编辑订阅【${targetAirport?.name} · ${editingSub?.name}】` : `为【${targetAirport?.name}】添加新订阅链接` }}
            </span>
            <button type="button" class="text-content-muted hover:text-content-main text-[11px]" @click="resetForms">
              返回列表
            </button>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label class="block text-[11px] font-semibold text-content-secondary mb-1">订阅别名 <span class="text-red-500">*</span></label>
              <input
                v-model="subAlias"
                placeholder="如: 主号 / 备用号 / 500G流量包 / 家人号"
                class="prototype-input w-full"
              />
            </div>
            <div>
              <label class="block text-[11px] font-semibold text-content-secondary mb-1">订阅备注 (选填)</label>
              <input
                v-model="subNote"
                placeholder="如: 到期日 2026-12 / 独享IP"
                class="prototype-input w-full"
              />
            </div>
          </div>

          <div>
            <label class="block text-[11px] font-semibold text-content-secondary mb-1">订阅完整链接 <span class="text-red-500">*</span></label>
            <input
              v-model="subUrl"
              placeholder="订阅链接 (HTTP URL 或 本地路径)"
              class="prototype-input w-full font-mono text-[11px]"
              :disabled="isLoadingURL"
            />
            <span v-if="isLoadingURL" class="text-[10px] text-content-muted mt-1 block">正在读取完整地址…</span>
          </div>

          <div v-if="errorMessage" class="text-red-500 text-[11px] bg-red-500/10 p-2 rounded border border-red-500/20">
            {{ errorMessage }}
          </div>

          <div class="flex justify-end gap-2 pt-1">
            <button
              type="button"
              class="tool-button"
              @click="resetForms"
            >
              取消
            </button>
            <button
              type="button"
              :disabled="isSubmitting || isLoadingURL"
              class="prototype-btn-primary"
              @click="handleSaveSub"
            >
              {{ isSubmitting ? '保存中...' : mode === 'edit_sub' ? '更新订阅' : '添加订阅' }}
            </button>
          </div>
        </div>

        <!-- 3. List: Two-Level Airport & Subscriptions Hierarchy -->
        <div v-else class="flex flex-col gap-4">
          <div class="manager-toolbar">
            <div class="manager-intro"><strong>{{ embedded ? '订阅管理' : '我的机场' }}</strong><span>{{ embedded ? '完整订阅地址仅在编辑或复制时读取' : `${store.airports.length} 个机场 · ${subscriptionCount} 条订阅` }}</span></div>
            <input v-if="!embedded && (store.airports.length > 2 || subscriptionCount > 4)" v-model="search" class="prototype-input" placeholder="搜索机场或订阅" aria-label="搜索机场、网址或订阅">
            <button type="button" class="tool-button refresh-all" :disabled="busy || !subscriptionCount" @click="refreshEntries(allEntries())"><span aria-hidden="true">↻</span> 刷新所有订阅</button>
          </div>
          <div v-if="refreshTotal || feedback" class="refresh-summary" role="status" aria-live="polite">
            <div>{{ refreshBusy ? `正在刷新 ${refreshDone} / ${refreshTotal} 个订阅` : feedback }}<button v-if="hasRefreshErrors && !refreshBusy" type="button" class="tool-button" @click="retryFailed">重试失败项</button></div>
            <progress v-if="refreshTotal" :max="refreshTotal" :value="refreshDone" aria-label="订阅刷新进度"></progress>
          </div>
          <div v-if="store.airports.length === 0" class="p-10 text-center text-content-muted border border-dashed border-border rounded-xl flex flex-col items-center gap-3">
            <div class="text-3xl">✈️</div>
            <span class="text-sm font-semibold text-content-main">暂无机场订阅配置</span>
            <span class="text-xs max-w-sm">点击下方按钮添加您的第一个机场品牌及订阅链接，支持同一个机场配置多个账号与套餐。</span>
            <button type="button" class="prototype-btn-primary mt-2" @click="openAddAirport">
              + 添加第一个机场
            </button>
          </div>

          <div
            v-for="ap in detailAirports"
            :key="ap.id"
            class="airport-card"
          >
            <!-- Level 1: Airport Brand Bar -->
            <div class="airport-header">
              <div class="airport-identity">
                <span class="airport-monogram" aria-hidden="true">{{ ap.name.slice(0, 1) }}</span>
                <div class="airport-heading">
                  <div class="airport-name-line"><strong>{{ ap.name }}</strong><span>{{ ap.subscriptions?.length || 0 }} 条订阅 · {{ ap.node_count }} 个节点</span></div>
                  <div class="airport-links">
                    <a
                      v-if="safeWebsite(ap.website_url)"
                      :href="safeWebsite(ap.website_url)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="airport-link"
                      title="打开机场官网"
                      @click.stop
                    >官网 · {{ websiteHost(ap.website_url) }} ↗</a>
                    <a
                      v-if="safeWebsite(ap.backup_url)"
                      :href="safeWebsite(ap.backup_url)"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="airport-link"
                      title="打开防失联页"
                      @click.stop
                    >备用入口 ↗</a>
                    <span v-if="ap.note" class="airport-note">{{ ap.note }}</span>
                  </div>
                </div>
              </div>

              <!-- Brand Actions -->
              <fieldset :disabled="busy" class="airport-actions">
                <button
                  type="button"
                  class="tool-button add-subscription"
                  title="为此机场添加新的订阅链接"
                  @click="openAddSub(ap)"
                >
                  + 添加订阅
                </button>
                <button
                  type="button"
                  v-if="store.airports.length > 1"
                  class="tool-button"
                  title="刷新该机场下所有订阅链接"
                  @click="handleRefreshAirport(ap.id)"
                >
                  刷新全部
                </button>
                <button
                  type="button"
                  class="tool-button"
                  title="修改机场主体名称、官网与防失联信息"
                  @click="openEditAirport(ap)"
                >
                  编辑
                </button>
                <button
                  type="button"
                  class="tool-button destructive"
                  title="删除整个机场及其全部订阅"
                  @click="handleDeleteAirport(ap)"
                >
                  删除
                </button>
              </fieldset>
            </div>

            <!-- Level 2: Subscriptions List -->
            <AirportMaintenancePanel :airport="ap" :discovered-links="discoveredLinks(ap)" @saved="store.loadAirports()" />
            <div class="subscriptions-list">
              <template v-if="ap.subscriptions && ap.subscriptions.length > 0">
                <div
                  v-for="sub in ap.subscriptions.filter(item => !attentionOnly || needsAttention(item))"
                  :key="sub.id"
                  class="subscription-row"
                  :class="{ 'needs-attention': needsAttention(sub) }"
                >
                  <!-- Sub identity -->
                  <div class="subscription-main">
                    <div class="subscription-title-line">
                      <strong>{{ sub.name }}</strong>
                      <span
                        class="subscription-status"
                        :class="sub.status === 'normal' ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/30' : sub.status === 'error' ? 'bg-red-500/10 text-red-500 border-red-500/30' : 'bg-border/30 text-content-muted border-border'"
                      >
                        {{ sub.status === 'normal' ? '节点已缓存' : sub.status === 'error' ? '无可用节点' : '未刷新' }}
                      </span>
                      <span class="subscription-node-count">{{ sub.node_count }} 个节点</span>
                    </div>
                    <div class="subscription-meta">
                      <span class="subscription-source" :title="sub.url_display">{{ sub.url_display }}</span>
                      <span>更新于 {{ formatDate(sub.updated_at) }}</span>
                    </div>
                    <p v-if="sub.note" class="subscription-note">{{ sub.note }}</p>
                    <div v-if="refreshResults[subKey(ap, sub)]" class="refresh-result" :class="refreshResults[subKey(ap, sub)].state" role="status">
                      <progress v-if="refreshResults[subKey(ap, sub)].state === 'running'" aria-label="正在刷新订阅"></progress>
                      {{ refreshResults[subKey(ap, sub)].message }}
                    </div>
                    <div v-if="sub.usage" class="subscription-usage" :class="{ warning: usageWarning(sub) }">
                      <div class="usage-heading"><span>{{ sub.usage.total ? '剩余可用' : '已使用' }} <strong>{{ bytes(sub.usage.total ? remaining(sub) : used(sub)) }}</strong><span v-if="sub.usage.total"> / {{ bytes(sub.usage.total) }}</span></span><span v-if="usageWarning(sub)" class="usage-warning">{{ usageWarning(sub) }}</span></div>
                      <progress v-if="sub.usage.total" :value="usagePercent(sub)" max="100" aria-label="套餐流量使用比例"></progress>
                      <div class="usage-details"><span>{{ sub.usage.total ? `剩余 ${bytes(remaining(sub))}` : '总额度未提供' }}</span><span>上传 {{ bytes(sub.usage.upload) }} · 下载 {{ bytes(sub.usage.download) }}</span><span>{{ sub.usage.expire ? `到期 ${new Date(sub.usage.expire * 1000).toLocaleDateString()}` : '到期时间未提供' }}</span></div>
                      <small>机场用量快照 · {{ formatDate(sub.usage.updated_at) }}<span v-if="refreshResults[subKey(ap, sub)]?.state === 'error'"> · 本次刷新失败，显示上次数据</span></small>
                    </div>
                    <div v-else class="usage-unavailable">暂无用量 · 刷新后仍为空表示机场未提供</div>
                  </div>

                  <!-- Sub Actions -->
                  <fieldset :disabled="busy" class="subscription-actions">
                    <button v-if="embedded" type="button" class="tool-button" :disabled="!sub.has_cache" @click="emit('browse-profile', sub.id)">查看节点 →</button>
                    <button
                      type="button"
                      class="tool-button refresh-subscription"
                      title="单独刷新此订阅节点"
                      @click="handleRefreshSub(ap, sub)"
                    >
                      ↻ 刷新
                    </button>
                    <button type="button" class="tool-button" @click="copySubscription(ap, sub)">复制地址</button>
                    <button
                      type="button"
                      class="tool-button"
                      title="编辑订阅别名与链接"
                      @click="openEditSub(ap, sub)"
                    >
                      编辑
                    </button>
                    <button
                      type="button"
                      class="tool-button destructive"
                      title="删除此订阅链接"
                      @click="handleDeleteSub(ap, sub)"
                    >
                      删除
                    </button>
                  </fieldset>
                </div>
              </template>

              <!-- Empty Subscriptions under airport -->
              <div v-else class="p-4 text-center text-content-muted flex items-center justify-center gap-2 text-xs">
                <span>该机场暂无订阅链接。</span>
                <button
                  type="button"
                  class="text-primary hover:underline font-semibold"
                  @click="openAddSub(ap)"
                >
                  立即添加订阅链接 →
                </button>
              </div>
            </div>
          </div>
          <p v-if="store.airports.length && !visibleAirports.length" class="usage-note">没有匹配的机场或订阅。</p>
          <p v-if="store.airports.length" class="usage-note">用量随订阅刷新更新，同一账号的订阅可能共享额度。</p>
        </div>
      </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.airport-workspace { padding: 28px 32px; max-width: 1550px; margin: 0 auto; }
.airport-workspace .subscription-manager { max-width: none; border: 0; border-radius: 0; box-shadow: none; background: transparent; }
.airport-workspace .subscription-manager .prototype-modal-header { padding: 0 0 25px; border: 0; background: transparent; }
.airport-workspace .manager-title { font-size: 26px; letter-spacing: -.6px; }
.airport-workspace .manager-subtitle { margin-top: 5px; font-size: 12px; }
.airport-workspace .manager-mark { display: none; }
.manager-overview { display: flex; align-items: stretch; gap: 32px; padding: 20px 24px; margin-bottom: 24px; background: var(--card-bg); border: 1px solid var(--border); border-radius: 12px; }
.manager-overview > div, .manager-overview > button { display: flex; flex-direction: column; gap: 5px; min-width: 100px; text-align: left; }
.manager-overview span { color: var(--text-secondary); font-size: 12px; }
.manager-overview strong { font-size: 27px; font-weight: 650; font-variant-numeric: tabular-nums; }
.manager-overview small { font-size: 11px; font-weight: 400; }
.manager-overview .warning { color: var(--warning); }
.manager-overview > button[aria-pressed=true] { color: var(--primary); }
.manager-overview p { margin-left: auto; align-self: center; font-size: 11px; line-height: 1.8; color: var(--text-muted); }
.manager-layout { display: flex; min-width: 0; }
.airport-browser { flex: 0 0 240px; padding: 0 20px 0 0; border-right: 1px solid var(--border); }
.airport-browser input { width: 100%; }
.airport-browser-caption { display: flex; justify-content: space-between; margin: 22px 4px 12px; font-size: 11px; color: var(--text-secondary); }
.airport-browser-caption button { color: var(--primary); }
.airport-browser-item { display: flex; width: 100%; align-items: center; gap: 10px; padding: 13px 10px; margin-bottom: 7px; border-radius: 9px; text-align: left; }
.airport-browser-item[aria-pressed=true] { background: #d7e8ef; box-shadow: inset 3px 0 var(--primary); color: var(--primary); }
.airport-browser-item strong { display: block; font-size: 13px; }
.airport-browser-item small { display: block; margin-top: 4px; font-size: 10px; color: var(--text-secondary); }
.airport-browser-item i { color: var(--warning); font-style: normal; margin-left: auto; }
.airport-workspace .prototype-modal-body { min-width: 0; padding: 0 0 0 26px; overflow: visible; }
.airport-workspace .airport-header { background: linear-gradient(110deg, var(--primary-subtle), var(--card)); padding: 24px; border-bottom: 2px solid #b9d1db; }
.airport-workspace .airport-name-line strong { font-size: 21px; }
.airport-workspace .subscription-row { display: flex; flex-direction: column; padding: 24px; gap: 16px; }
.airport-workspace .subscription-main { width: 100%; }
.airport-workspace .subscription-title-line > strong { font-size: 15px; }
.airport-workspace .subscription-usage { max-width: none; padding: 20px; margin-top: 18px; background: #edf5f1; border-left: 3px solid #4a8870; }
.airport-workspace .usage-heading strong { font-size: 24px; }
.airport-workspace .subscription-actions { align-self: flex-end; flex-wrap: wrap; }
@media (max-width: 800px) {
 .airport-workspace { padding: 20px 14px; } .manager-layout { flex-direction: column; }
 .airport-browser { flex: auto; border: 0; padding: 0 0 18px; } .airport-browser-item { display: inline-flex; width: auto; margin-right: 8px; }
 .airport-workspace .prototype-modal-body { padding: 0; } .manager-overview { gap: 20px; flex-wrap: wrap; } .manager-overview p { display: none; }
}
.subscription-manager { max-width: 800px; color: var(--text-main); font-size: 13px; }
.subscription-manager .prototype-modal-header { padding: 18px 22px; background: var(--card-bg); }
.manager-mark { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 9px; background: var(--primary-subtle); color: var(--primary); font-size: 20px; font-weight: 700; }
.manager-title { margin: 0; color: var(--text-main); font-size: 17px; font-weight: 720; line-height: 1.35; }
.manager-subtitle { margin: 2px 0 0; color: var(--text-secondary); font-size: 11px; }
.subscription-manager .manager-add { display: inline-flex; align-items: center; gap: 5px; min-height: 34px; padding: 0 12px; border: 1px solid var(--primary); border-radius: 7px; background: var(--primary); color: white; font-size: 12px; font-weight: 650; cursor: pointer; }
.subscription-manager .manager-add:hover:not(:disabled) { background: var(--primary-hover); }
.subscription-manager .prototype-modal-body { padding: 20px 22px 18px; }
.subscription-manager fieldset { display: flex; margin: 0; padding: 0; border: 0; min-width: 0; }
.subscription-manager button:disabled, .subscription-manager fieldset:disabled button { opacity: .45; cursor: not-allowed; }
.subscription-manager .tool-button { display: inline-flex; align-items: center; justify-content: center; min-height: 31px; padding: 5px 10px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); color: var(--text-secondary); font-size: 11px; font-weight: 600; line-height: 1.2; white-space: nowrap; cursor: pointer; transition: border-color .15s ease, color .15s ease, background .15s ease; }
.subscription-manager .tool-button:hover:not(:disabled) { border-color: var(--border-focus); background: var(--primary-subtle); color: var(--primary); }
.subscription-manager .tool-button:focus-visible, .subscription-manager .manager-add:focus-visible, .subscription-manager .airport-link:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
.subscription-manager .tool-button.destructive { border-color: transparent; background: transparent; color: var(--text-muted); }
.subscription-manager .tool-button.destructive:hover:not(:disabled) { background: var(--danger-bg); color: var(--danger); }
.manager-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
.manager-intro { display: flex; flex-direction: column; gap: 2px; margin-right: auto; }
.manager-intro strong { font-size: 14px; font-weight: 680; }
.manager-intro span { color: var(--text-secondary); font-size: 11px; }
.manager-toolbar input { width: 190px; }
.subscription-manager .refresh-all { min-height: 34px; color: var(--primary); border-color: var(--border-focus); }
.subscription-manager .refresh-all span { font-size: 16px; line-height: .8; }
.airport-card { overflow: hidden; border: 1px solid var(--border); border-radius: 10px; background: var(--card-bg); }
.airport-header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 14px; padding: 16px 18px; background: var(--card-subtle); border-bottom: 1px solid var(--border); }
.airport-identity { display: flex; align-items: center; gap: 12px; min-width: 0; }
.airport-monogram { display: grid; flex: none; place-items: center; width: 38px; height: 38px; border-radius: 9px; background: var(--primary-subtle); color: var(--primary); font-size: 17px; font-weight: 750; }
.airport-heading { min-width: 0; }
.airport-name-line { display: flex; align-items: baseline; flex-wrap: wrap; gap: 5px 10px; }
.airport-name-line strong { color: var(--text-main); font-size: 15px; font-weight: 720; }
.airport-name-line span, .airport-note { color: var(--text-secondary); font-size: 11px; }
.airport-links { display: flex; align-items: center; flex-wrap: wrap; gap: 4px 13px; margin-top: 4px; }
.airport-link { color: var(--primary); font-size: 11px; text-decoration: none; }
.airport-link:hover { text-decoration: underline; }
.airport-actions { gap: 5px; flex-wrap: wrap; }
.subscription-manager .add-subscription { color: var(--primary); border-color: var(--border-focus); }
.subscriptions-list > :not(:last-child) { border-bottom: 1px solid var(--border); }
.subscription-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 16px; align-items: start; padding: 18px; }
.subscription-main { min-width: 0; }
.subscription-title-line { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 9px; }
.subscription-title-line > strong { font-size: 13px; font-weight: 700; }
.subscription-status { padding: 2px 7px; border: 1px solid var(--border); border-radius: 4px; font-size: 10px; font-weight: 600; }
.subscription-node-count { color: var(--text-secondary); font-size: 11px; }
.subscription-meta { display: flex; align-items: center; flex-wrap: wrap; gap: 4px 11px; margin-top: 8px; color: var(--text-muted); font-size: 11px; }
.subscription-source { display: inline-block; max-width: min(350px, 100%); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.subscription-note { margin: 7px 0 0; color: var(--text-secondary); font-size: 11px; }
.subscription-actions { justify-content: flex-end; flex-wrap: nowrap; gap: 5px; }
.subscription-manager .refresh-subscription { color: var(--primary); border-color: var(--border-focus); }
.usage-note, .usage-unavailable { margin: 0; color: var(--text-secondary); font-size: 11px; line-height: 1.55; }
.usage-note { padding: 0 2px; }
.usage-unavailable { margin-top: 10px; }
.subscription-usage { max-width: 550px; margin-top: 11px; padding: 11px 13px; border-radius: 7px; background: var(--card-subtle); }
.usage-heading { display: flex; justify-content: space-between; gap: 12px; color: var(--text-secondary); }
.usage-heading strong { color: var(--text-main); font-size: 15px; font-variant-numeric: tabular-nums; }
progress { display: block; width: 100%; height: 6px; margin: 8px 0; accent-color: var(--primary); border: 0; border-radius: 4px; overflow: hidden; }
progress::-webkit-progress-bar { background: var(--border); }
progress::-webkit-progress-value { background: var(--primary); }
.warning progress { accent-color: var(--warning); }
.warning progress::-webkit-progress-value { background: var(--warning); }
.usage-warning { color: var(--warning); font-weight: 600; }
.usage-details { display: flex; flex-wrap: wrap; gap: 5px 16px; margin-top: 7px; color: var(--text-secondary); font-size: 11px; }
.subscription-usage small { display: block; margin-top: 8px; color: var(--text-muted); font-size: 10px; }
.refresh-summary { padding: 10px 14px; background: var(--primary-subtle); border-radius: 7px; }
.refresh-summary > div { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.refresh-result { margin-top: 7px; font-size: 11px; color: var(--text-secondary); }
.refresh-result.success { color: var(--success); }
.refresh-result.error { color: var(--danger); }
.refresh-result progress { max-width: 180px; }
@media (max-width: 700px) {
  .subscription-manager .prototype-modal-header { padding: 15px; }
  .subscription-manager .prototype-modal-body { padding: 15px; }
  .airport-header { align-items: flex-start; }
  .subscription-row { grid-template-columns: 1fr; }
  .subscription-actions { justify-content: flex-start; flex-wrap: wrap; }
  .manager-toolbar input { width: 100%; }
}
</style>
