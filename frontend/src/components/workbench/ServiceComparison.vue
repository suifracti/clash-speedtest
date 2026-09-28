<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { MonitorNodeOption, WorkbenchPublicServiceAttempt, WorkbenchPublicServiceRule } from '../../types'
import { serviceDetailLabel, serviceOutcomeLabel } from '../../utils/serviceOutcome'
import { serviceEvidence, serviceTitle, summarizeService } from '../../utils/servicePresentation'
import UiSelect from '../common/UiSelect.vue'
import InteractiveTrendSparkline, { type TrendPoint } from './InteractiveTrendSparkline.vue'

const props = defineProps<{
  rows: { key: string; node: MonitorNodeOption }[]
  services: { value: string | number; label: string }[]
  catalog?: WorkbenchPublicServiceRule[]
  records: Record<string, WorkbenchPublicServiceAttempt[]>
  selected: string[]
  states: Record<string, string>
  partial: Record<string, { hasMore: boolean; complete: boolean }>
  running?: boolean
}>()

const emit = defineEmits<{
  (e: 'toggle', key: string): void
  (e: 'service', id: string): void
  (e: 'detail', node: MonitorNodeOption): void
  (e: 'run', keys?: string[]): void
  (e: 'select-all', keys: string[]): void
  (e: 'clear-selection'): void
  (e: 'back-to-picker'): void
}>()

const focus = ref('')
const showAllNodes = ref(false)
const inspected = ref<WorkbenchPublicServiceAttempt | null>(null)
const viewMode = ref<'all' | 'single'>('all')
const sortBy = ref<'region' | 'pass_rate' | 'status' | 'recent' | 'changes' | 'name'>('region')
const selectedTimeWindow = ref<'24h' | '48h' | '7d' | 'all'>('24h')

function nodeServiceTrendPoints(key: string): TrendPoint[] {
  const list = props.records[key] || []
  const filtered = viewMode.value === 'single' && focus.value
    ? list.filter(a => a.service_id === focus.value)
    : list
  return filtered
    .filter(a => a.result && a.result.outcome !== 'cancelled')
    .map(a => ({
      id: a.attempt_id,
      time: Date.parse(a.result?.finished_at || a.finished_at || a.requested_at),
      durationMs: a.result?.duration_ms,
      status: ['matched', 'reachable', 'profiled', 'unlocked'].includes(a.result?.outcome || '') ? 'completed' : 'failed',
      outcome: a.result?.outcome,
      summary: cleanSummary(a.result, a.rule),
    }))
    .sort((a, b) => a.time - b.time)
}

const sortOptions = [
  { value: 'region', label: '按地区' },
  { value: 'status', label: '状态优先 (可用在前)' },
  { value: 'pass_rate', label: '通过率优先' },
  { value: 'recent', label: '最新测试优先' },
  { value: 'changes', label: '结果变化优先' },
  { value: 'name', label: '按节点名称' },
]

const choices = computed(() => {
  const map = new Map<string, string>()
  for (const s of props.services) {
    map.set(String(s.value), s.label)
  }
  for (const list of Object.values(props.records)) {
    for (const item of list) {
      if (item.service_id && !map.has(item.service_id)) {
        const matched = props.catalog?.find(r => r.service_id === item.service_id)
        map.set(item.service_id, matched?.name || item.service_id)
      }
    }
  }
  return [...map.entries()].map(([value, label]) => {
    const matchedRule = props.catalog?.find(r => r.service_id === value)
    return { value, label: matchedRule?.name || label, evidence: serviceEvidence(matchedRule) }
  })
})

watch(choices, list => {
  if (!list.some(s => s.value === focus.value)) {
    focus.value = list[0]?.value || ''
  }
}, { immediate: true })

const rule = computed(() => props.catalog?.find(r => r.service_id === focus.value))

const viewChoice = computed({
  get: () => (viewMode.value === 'all' ? 'all' : focus.value),
  set: (value: string | number) => {
    if (value === 'all') {
      viewMode.value = 'all'
      return
    }
    focus.value = String(value)
    viewMode.value = 'single'
    emit('service', focus.value)
  },
})

function outcomeRank(outcome?: string): number {
  if (!outcome) return 0
  switch (outcome) {
    case 'matched':
    case 'unlocked':
      return 6
    case 'profiled':
    case 'reachable':
      return 5
    case 'challenge':
    case 'redirect':
      return 3
    case 'rate_limited':
    case 'permission_denied':
    case 'auth_failed':
    case 'setup_required':
      return 2
    case 'region_blocked':
    case 'transport_error':
    case 'timed_out':
    case 'criteria_mismatch':
      return 1
    default:
      return 1
  }
}

const rows = computed(() =>
  props.rows
    .map(row => ({
      ...row,
      report: summarizeService((props.records[row.key] || []).filter(a => a.service_id === focus.value)),
    }))
    .filter(row => showAllNodes.value || row.report.total > 0),
)

const sortedRows = computed(() => {
  const list = [...rows.value]
  switch (sortBy.value) {
    case 'pass_rate':
      return list.sort((a, b) => {
        if (a.report.total === 0 && b.report.total > 0) return 1
        if (a.report.total > 0 && b.report.total === 0) return -1
        if (b.report.rate !== a.report.rate) return (b.report.rate ?? 0) - (a.report.rate ?? 0)
        return b.report.passed - a.report.passed
      })
    case 'status':
      return list.sort((a, b) => {
        const rankA = outcomeRank(a.report.latest?.result?.outcome)
        const rankB = outcomeRank(b.report.latest?.result?.outcome)
        if (rankB !== rankA) return rankB - rankA
        return (b.report.rate ?? 0) - (a.report.rate ?? 0)
      })
    case 'recent':
      return list.sort((a, b) => {
        const timeA = a.report.latest
          ? Date.parse(a.report.latest.result?.finished_at || a.report.latest.finished_at || a.report.latest.requested_at)
          : 0
        const timeB = b.report.latest
          ? Date.parse(b.report.latest.result?.finished_at || b.report.latest.finished_at || b.report.latest.requested_at)
          : 0
        return timeB - timeA
      })
    case 'changes':
      return list.sort((a, b) => {
        if (b.report.changes !== a.report.changes) return b.report.changes - a.report.changes
        return b.report.total - a.report.total
      })
    case 'name':
      return list.sort((a, b) => a.node.displayName.localeCompare(b.node.displayName, 'zh-CN'))
    case 'region':
    default:
      return list.sort((a, b) => {
        const regA = a.node.countryCode || 'OTHER'
        const regB = b.node.countryCode || 'OTHER'
        if (regA !== regB) return regA.localeCompare(regB)
        return a.node.displayName.localeCompare(b.node.displayName, 'zh-CN')
      })
  }
})

const overviewRows = computed(() =>
  props.rows
    .map(row => ({
      ...row,
      reports: choices.value.map(service => ({
        service,
        report: summarizeService((props.records[row.key] || []).filter(a => a.service_id === service.value)),
      })),
    }))
    .filter(row => showAllNodes.value || row.reports.some(item => item.report.total > 0)),
)

const sortedOverviewRows = computed(() => {
  const list = [...overviewRows.value]
  switch (sortBy.value) {
    case 'pass_rate':
      return list.sort((a, b) => {
        const totalA = a.reports.reduce((sum, item) => sum + item.report.total, 0)
        const totalB = b.reports.reduce((sum, item) => sum + item.report.total, 0)
        if (totalA === 0 && totalB > 0) return 1
        if (totalA > 0 && totalB === 0) return -1
        const passA = a.reports.reduce((sum, item) => sum + item.report.passed, 0)
        const passB = b.reports.reduce((sum, item) => sum + item.report.passed, 0)
        const rateA = totalA > 0 ? passA / totalA : 0
        const rateB = totalB > 0 ? passB / totalB : 0
        if (rateB !== rateA) return rateB - rateA
        return passB - passA
      })
    case 'status':
      return list.sort((a, b) => {
        const maxRankA = Math.max(0, ...a.reports.map(r => outcomeRank(r.report.latest?.result?.outcome)))
        const maxRankB = Math.max(0, ...b.reports.map(r => outcomeRank(r.report.latest?.result?.outcome)))
        return maxRankB - maxRankA
      })
    case 'recent':
      return list.sort((a, b) => {
        const latestTimeA = Math.max(
          0,
          ...a.reports.map(r =>
            r.report.latest
              ? Date.parse(r.report.latest.result?.finished_at || r.report.latest.finished_at || r.report.latest.requested_at)
              : 0,
          ),
        )
        const latestTimeB = Math.max(
          0,
          ...b.reports.map(r =>
            r.report.latest
              ? Date.parse(r.report.latest.result?.finished_at || r.report.latest.finished_at || r.report.latest.requested_at)
              : 0,
          ),
        )
        return latestTimeB - latestTimeA
      })
    case 'changes':
      return list.sort((a, b) => {
        const chA = a.reports.reduce((sum, item) => sum + item.report.changes, 0)
        const chB = b.reports.reduce((sum, item) => sum + item.report.changes, 0)
        return chB - chA
      })
    case 'name':
      return list.sort((a, b) => a.node.displayName.localeCompare(b.node.displayName, 'zh-CN'))
    case 'region':
    default:
      return list.sort((a, b) => {
        const regA = a.node.countryCode || 'OTHER'
        const regB = b.node.countryCode || 'OTHER'
        if (regA !== regB) return regA.localeCompare(regB)
        return a.node.displayName.localeCompare(b.node.displayName, 'zh-CN')
      })
  }
})

const overviewMeasured = computed(() =>
  overviewRows.value.filter(row => row.reports.some(item => item.report.total > 0)).length,
)
const measured = computed(() => rows.value.filter(r => r.report.total > 0))
const changed = computed(() => measured.value.filter(r => r.report.changes > 0).length)

// Outdated rule detection
const outdatedRows = computed(() => {
  if (!rule.value) return []
  return rows.value.filter(
    r => r.report.total > 0 && r.report.latest?.rule.rule_version !== rule.value?.rule_version,
  )
})
const outdatedCount = computed(() => outdatedRows.value.length)
const outdatedVersion = computed(() => outdatedRows.value[0]?.report.latest?.rule.rule_version ?? '旧')

function retestOutdated() {
  const keys = outdatedRows.value.map(r => r.key)
  emit('run', keys)
}

const selectedRegion = ref('全部地区')
const searchKeyword = ref('')
const statusFilter = ref<'all' | 'good' | 'bad' | 'untested'>('all')

const statusFilterOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'good', label: '仅可用 (通过)' },
  { value: 'bad', label: '仅异常 / 受限' },
  { value: 'untested', label: '仅未检测' },
]

const regionOptions = computed(() => {
  const set = new Set<string>()
  for (const r of props.rows) {
    if (r.node.countryCode && r.node.countryCode !== 'OTHER') set.add(r.node.countryCode)
  }
  const sorted = [...set].sort()
  return [{ value: '全部地区', label: '全部地区' }, ...sorted.map(c => ({ value: c, label: c }))]
})

const filteredRows = computed(() => {
  return sortedRows.value.filter(row => {
    if (selectedRegion.value !== '全部地区' && row.node.countryCode !== selectedRegion.value) {
      return false
    }
    if (searchKeyword.value.trim()) {
      const q = searchKeyword.value.trim().toLowerCase()
      const name = (row.node.displayName || '').toLowerCase()
      const reg = (row.node.countryCode || '').toLowerCase()
      const profile = (row.node.profileName || '').toLowerCase()
      if (!name.includes(q) && !reg.includes(q) && !profile.includes(q)) {
        return false
      }
    }
    if (statusFilter.value !== 'all') {
      const isGood = ['matched', 'unlocked', 'reachable', 'profiled'].includes(row.report.latest?.result?.outcome || '')
      if (statusFilter.value === 'good' && !isGood) return false
      if (statusFilter.value === 'bad' && (isGood || row.report.total === 0)) return false
      if (statusFilter.value === 'untested' && row.report.total > 0) return false
    }
    return true
  })
})

const filteredOverviewRows = computed(() => {
  return sortedOverviewRows.value.filter(row => {
    if (selectedRegion.value !== '全部地区' && row.node.countryCode !== selectedRegion.value) {
      return false
    }
    if (searchKeyword.value.trim()) {
      const q = searchKeyword.value.trim().toLowerCase()
      const name = (row.node.displayName || '').toLowerCase()
      const reg = (row.node.countryCode || '').toLowerCase()
      const profile = (row.node.profileName || '').toLowerCase()
      if (!name.includes(q) && !reg.includes(q) && !profile.includes(q)) {
        return false
      }
    }
    return true
  })
})

// Current selection helpers
const currentKeys = computed(() =>
  (viewMode.value === 'all' ? filteredOverviewRows.value : filteredRows.value).map(r => r.key),
)
const isAllSelected = computed(
  () => currentKeys.value.length > 0 && currentKeys.value.every(k => props.selected.includes(k)),
)

function toggleSelectAllCurrent() {
  if (isAllSelected.value) {
    emit('clear-selection')
  } else {
    emit('select-all', currentKeys.value)
  }
}

function handleRunBatch() {
  const keys = props.selected.length > 0 ? props.selected : currentKeys.value
  emit('run', keys)
}

function tone(a?: WorkbenchPublicServiceAttempt) {
  const outcome = a?.result?.outcome
  return !outcome
    ? 'unknown'
    : ['matched', 'profiled', 'reachable', 'unlocked'].includes(outcome)
      ? 'good'
      : ['transport_error', 'timed_out'].includes(outcome)
        ? 'bad'
        : 'limited'
}

function label(a?: WorkbenchPublicServiceAttempt) {
  if (a?.service_id === 'antigravity' && a.result?.outcome === 'matched') return '模型已回答'
  if (a?.rule.result_kind === 'exit_profile' && a.result?.outcome === 'profiled') return '出口已获取'
  return serviceOutcomeLabel(a?.result?.outcome)
}

function cleanSummary(result?: WorkbenchPublicServiceAttempt['result'], rule?: WorkbenchPublicServiceRule): string {
  if (!result) return '尚无检测记录'
  if (result.outcome === 'matched') {
    if (result.details?.checked_model) {
      return `真实模型响应成功 (${result.details.checked_model})`
    }
    return '检测通过，真实服务响应正常'
  }
  if (result.outcome === 'unlocked') {
    return '完整版权片目已解锁'
  }
  if (result.outcome === 'originals_only') {
    return '仅可访问自制剧内容'
  }
  if (result.outcome === 'region_blocked') {
    return '服务明确限制该地区访问 (403)'
  }
  if (result.outcome === 'challenge') {
    return '遇到 Cloudflare 验证盾阻拦'
  }
  if (result.outcome === 'rate_limited') {
    return '请求频次或配额受限'
  }
  if (result.outcome === 'credentials_required') {
    return '请先在系统设置中绑定 Google 账号凭据'
  }
  if (result.outcome === 'timed_out') {
    return '网络请求超时，未能收到响应'
  }
  if (result.error_message?.includes('读取上限') || result.summary?.includes('读取上限')) {
    return '旧规则响应截断，请点击右侧“⚡ 检测”以新规则重测'
  }
  return result.summary || result.error_message || serviceOutcomeLabel(result.outcome)
}

function stampRelative(a: WorkbenchPublicServiceAttempt): string {
  const t = Date.parse(a.result?.finished_at || a.finished_at || a.requested_at)
  if (!t || isNaN(t)) return ''
  const diffSec = Math.floor((Date.now() - t) / 1000)
  if (diffSec < 60) return '刚刚'
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}分钟前`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}小时前`
  return `${Math.floor(diffSec / 86400)}天前`
}

function inconclusive(report: ReturnType<typeof summarizeService>): boolean {
  return (
    report.total > 0 &&
    report.passed === 0 &&
    report.samples.every(a =>
      [
        'unknown',
        'timed_out',
        'transport_error',
        'rate_limited',
        'credentials_required',
        'setup_required',
        'auth_failed',
        'permission_denied',
      ].includes(a.result?.outcome || ''),
    )
  )
}

function stamp(a: WorkbenchPublicServiceAttempt) {
  return new Date(a.result?.finished_at || a.finished_at || a.requested_at).toLocaleString()
}
</script>

<template>
  <div class="service-results">
    <div class="results-heading">
      <div v-if="viewMode !== 'all'" class="heading-title-box">
        <h3>{{ rule ? serviceTitle(rule) : '先在上方选择服务' }}</h3>
        <p>
          {{
            `${serviceEvidence(rule)} · ${
              rule?.service_id === 'antigravity'
                ? '只有收到真实模型内容才标记通过；账号、配额、地区问题清晰展示。'
                : '通过率仅衡量当前服务判据。'
            }`
          }}
        </p>
      </div>
      <div class="results-tools">
        <input
          v-model="searchKeyword"
          type="search"
          placeholder="🔍 搜索节点 / 地区…"
          class="service-search-input"
        >
        <label>地区 <UiSelect v-model="selectedRegion" aria-label="筛选地区" :options="regionOptions" /></label>
        <label v-if="viewMode !== 'all'">状态 <UiSelect v-model="statusFilter" aria-label="筛选状态" :options="statusFilterOptions" /></label>
        <label>查看 <UiSelect v-model="viewChoice" aria-label="查看服务结果" :options="[{ value: 'all', label: '全部服务概览' }, ...choices]" /></label>
        <label>排序 <UiSelect v-model="sortBy" aria-label="排序服务结果" :options="sortOptions" /></label>
        <label class="checkbox-inline"><input v-model="showAllNodes" type="checkbox">含未测</label>
        <div class="batch-action-group">
          <button type="button" class="tool-btn" @click="toggleSelectAllCurrent">
            {{ isAllSelected ? '取消全选' : `全选 (${currentKeys.length})` }}
          </button>
          <button type="button" class="tool-btn primary" :disabled="running || !currentKeys.length" @click="handleRunBatch">
            {{ running ? '检测中…' : selected.length ? `⚡ 检测已选 (${selected.length})` : `⚡ 检测全部 (${currentKeys.length})` }}
          </button>
        </div>
      </div>
    </div>

    <!-- Outdated rule notice banner -->
    <div v-if="viewMode !== 'all' && outdatedCount > 0" class="outdated-banner">
      <div class="outdated-info">
        <div class="outdated-tag-line">
          <span class="outdated-tag">⚠️ 历史规则缓存</span>
          <strong>发现 {{ outdatedCount }} 个节点的历史结果基于旧版规则 (v{{ outdatedVersion }})</strong>
        </div>
        <p>旧版规则因响应体读取上限（64 KiB）导致模型无法完整识别。当前新版规则已解除读取限制，建议重新检测以获得最新真实结果。</p>
      </div>
      <button type="button" class="outdated-retest-btn" :disabled="running" @click="retestOutdated">
        {{ running ? '检测中…' : `⚡ 重新检测这 ${outdatedCount} 个节点` }}
      </button>
    </div>

    <template v-if="viewMode === 'all'">
      <div v-if="overviewMeasured" class="overview-status">
        <span>已测节点：<b>{{ overviewMeasured }}</b> 个</span>
        <small>未测节点点击“⚡ 检测”即可一键验证可用性。</small>
      </div>
      <div v-if="filteredOverviewRows.length" class="service-overview">
        <div v-for="row in filteredOverviewRows" :key="row.key" class="overview-row-v5">
          <label class="overview-node-label">
            <input type="checkbox" :checked="selected.includes(row.key)" :aria-label="`选择 ${row.node.displayName}`" @change="emit('toggle', row.key)">
            <span class="overview-flag">{{ row.node.countryFlag || '🌐' }}</span>
            <div class="overview-node-text">
              <strong :title="row.node.displayName">{{ row.node.displayName }}</strong>
              <small>{{ row.node.countryCode || 'OTHER' }} · {{ row.node.type || '节点' }}</small>
            </div>
          </label>
          <div class="overview-services-v5">
            <button
              v-for="item in row.reports"
              :key="item.service.value"
              type="button"
              :class="['overview-service-pill', item.report.total ? tone(item.report.latest) : 'untested']"
              :title="`${item.service.label}：${item.report.total ? label(item.report.latest) : '未检测'} · 点击深入查看`"
              @click="focus = item.service.value; viewMode = 'single'; emit('service', item.service.value)"
            >
              <span class="pill-dot"></span>
              <span class="svc-name">{{ item.service.label.split(' ')[0] }}</span>
              <strong class="svc-status">{{ item.report.total ? label(item.report.latest) : '未测' }}</strong>
              <small v-if="item.report.latest?.result?.duration_ms" class="svc-dur">{{ item.report.latest.result.duration_ms }}ms</small>
            </button>
            <button type="button" class="node-quick-test-btn" :disabled="running" title="针对此节点立即执行检测" @click.stop="emit('run', [row.key])">
              ⚡ 检测
            </button>
          </div>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="result-summary-v5">
        <div class="summary-stat-group">
          <span class="stat-pill">已测节点 <b>{{ measured.length }}</b> / {{ rows.length }}</span>
          <span v-if="changed > 0" class="stat-pill warn">波动节点 <b>{{ changed }}</b></span>
        </div>
      </div>

      <div class="service-node-grid-v5">
        <article
          v-for="row in filteredRows"
          :key="row.key"
          class="service-node-v5"
          :class="{ selected: selected.includes(row.key), 'is-outdated': row.report.latest?.rule.rule_version !== rule?.rule_version && row.report.total > 0 }"
        >
          <header class="card-header-v5">
            <label class="card-node-info">
              <input type="checkbox" :checked="selected.includes(row.key)" :aria-label="`选择 ${row.node.displayName}`" @change="emit('toggle', row.key)">
              <span class="card-flag">{{ row.node.countryFlag || '🌐' }}</span>
              <div class="card-title-group">
                <strong class="card-node-name" :title="row.node.displayName">{{ row.node.displayName }}</strong>
                <div class="card-tags-line">
                  <span v-if="row.node.countryCode" class="card-tag region">{{ row.node.countryCode }}</span>
                  <span class="card-tag protocol">{{ row.node.type || '节点' }}</span>
                </div>
              </div>
            </label>
            <div class="card-actions-v5">
              <span class="outcome-badge-v5" :class="tone(row.report.latest)">
                <span class="badge-dot"></span>
                {{ row.report.total ? label(row.report.latest) : states[row.key] === 'loading' ? '读取中…' : '未检测' }}
              </span>
              <button
                type="button"
                class="card-retest-btn-v5"
                :disabled="running"
                title="针对此节点立即执行本项检测"
                @click.stop="emit('run', [row.key])"
              >
                ⚡ 检测
              </button>
            </div>
          </header>

          <!-- Card Body -->
          <div v-if="row.report.total" class="card-body-v5">
            <!-- Interactive Trend Sparkline -->
            <div class="card-sparkline-row">
              <span class="sparkline-title">历史走势 (悬浮看对比):</span>
              <InteractiveTrendSparkline
                :points="nodeServiceTrendPoints(row.key)"
                type="service"
                :window="selectedTimeWindow"
                :height="30"
              />
            </div>
            <div class="card-evidence-line">
              <span v-if="row.report.latest?.result?.details?.checked_model" class="pill-model" title="已验证可用模型">
                🤖 {{ row.report.latest.result.details.checked_model }}
              </span>
              <span v-if="row.report.latest?.result?.duration_ms" class="pill-duration">
                ⏱️ {{ row.report.latest.result.duration_ms }} ms
              </span>
              <span v-if="row.report.latest?.rule.rule_version !== rule?.rule_version" class="pill-outdated" title="旧版规则限制了读取体，以新版规则重测可获取完整模型">
                ⚠️ 旧规则 (v{{ row.report.latest?.rule.rule_version }})
              </span>
              <span class="evidence-desc" :title="cleanSummary(row.report.latest?.result, rule)">
                {{ cleanSummary(row.report.latest?.result, rule) }}
              </span>
            </div>

            <div class="card-meta-line">
              <span class="meta-stat">
                通过率 <b>{{ row.report.rate }}%</b> <small>({{ row.report.passed }}/{{ row.report.total }}次)</small>
              </span>
              <span v-if="row.report.changes > 0" class="meta-fluctuation">
                ⚠️ 波动 {{ row.report.changes }} 次
              </span>
              <span class="meta-time">{{ stampRelative(row.report.latest!) }}</span>
              <button type="button" class="btn-evidence-detail" @click="inspected = row.report.latest || null">
                详情 →
              </button>
            </div>
          </div>

          <!-- Untested State -->
          <div v-else class="card-untested-v5">
            <span>尚未检测本项服务</span>
            <button type="button" class="btn-quick-run-inline" :disabled="running" @click.stop="emit('run', [row.key])">
              立即探测
            </button>
          </div>
        </article>
      </div>
    </template>

    <p v-if="!(viewMode === 'all' ? filteredOverviewRows.length : filteredRows.length)" class="results-empty">
      {{ selected.length ? `已选 ${selected.length} 条线路，等待开始检测；结果会在这里出现。` : '选择好服务和节点，即可开始检测。' }}
    </p>

    <section v-if="inspected" class="service-inspector" role="region" aria-label="服务检测记录">
      <button class="close-inspector" aria-label="关闭检测记录" @click="inspected = null">×</button>
      <span>检测依据 · {{ stamp(inspected) }}</span>
      <h3>{{ inspected.display_name }} · {{ inspected.rule.name }}</h3>
      <strong class="outcome" :class="tone(inspected)">{{ label(inspected) }}</strong>
      <p>{{ inspected.result?.summary || inspected.result?.error_message }}</p>
      <dl>
        <div v-for="(value, key) in inspected.result?.details" :key="key">
          <dt>{{ serviceDetailLabel(String(key)) }}</dt>
          <dd>{{ value || '未提供' }}</dd>
        </div>
      </dl>
      <details>
        <summary>技术信息与保存状态</summary>
        <p>HTTP {{ inspected.result?.http_status ?? '无响应' }} · {{ inspected.result?.duration_ms ?? '—' }} ms · {{ inspected.persistence_state === 'saved' ? '已保存' : '尚未保存' }}</p>
        <p>{{ inspected.rule.success_criterion }}</p>
        <p>{{ inspected.persistence_error }}</p>
      </details>
    </section>
  </div>
</template>

<style scoped>
.service-results {
  padding: 16px 20px;
}
.results-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.results-heading > div > span {
  font-size: 11px;
  letter-spacing: 1px;
  color: var(--primary);
  font-weight: 750;
}
.results-heading h3 {
  font-size: 24px;
  margin: 6px 0;
  font-weight: 750;
}
.results-heading p {
  font-size: 12px;
  color: var(--text-secondary);
  max-width: 620px;
  line-height: 1.6;
}
.results-tools {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-left: auto;
}
.results-tools label {
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-secondary);
}
.checkbox-inline {
  cursor: pointer;
  user-select: none;
}
.batch-action-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tool-btn {
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--card-bg);
  color: var(--text-primary);
  cursor: pointer;
  transition: all 0.15s ease;
}
.tool-btn:hover:not(:disabled) {
  border-color: var(--primary);
  background: var(--primary-subtle);
}
.tool-btn.primary {
  background: var(--primary);
  border-color: var(--primary);
  color: white;
}
.tool-btn.primary:hover:not(:disabled) {
  opacity: 0.9;
}
.tool-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

/* Outdated rule notice banner */
.outdated-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 18px;
  margin: 0 0 16px;
  background: #fff8e6;
  border: 1px solid #ffd591;
  border-radius: 10px;
}
:root.dark .outdated-banner {
  background: #2b2111;
  border-color: #594214;
}
.outdated-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.outdated-tag-line {
  display: flex;
  align-items: center;
  gap: 8px;
}
.outdated-tag {
  font-size: 11px;
  padding: 2px 6px;
  background: #ffe58f;
  color: #874d00;
  font-weight: 700;
  border-radius: 4px;
}
:root.dark .outdated-tag {
  background: #594214;
  color: #ffd591;
}
.outdated-banner strong {
  font-size: 13px;
  color: var(--text-primary);
}
.outdated-banner p {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0;
  line-height: 1.5;
}
.outdated-retest-btn {
  padding: 8px 16px;
  font-size: 12px;
  font-weight: 700;
  border-radius: 7px;
  background: #fa8c16;
  color: white;
  border: none;
  cursor: pointer;
  white-space: nowrap;
  flex-shrink: 0;
  transition: opacity 0.15s ease;
}
.outdated-retest-btn:hover:not(:disabled) {
  opacity: 0.9;
}
.outdated-retest-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.service-search-input {
  padding: 5px 10px;
  font-size: 12px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--card-bg);
  color: var(--text-primary);
  width: 170px;
  transition: all 0.15s ease;
}
.service-search-input:focus {
  outline: none;
  border-color: var(--primary);
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2);
}

.result-summary-v5 {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 6px 0 12px;
}
.summary-stat-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
.stat-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 9px;
  border-radius: 6px;
  background: var(--card-subtle);
  border: 1px solid var(--border);
  font-size: 11.5px;
  color: var(--text-secondary);
}
.stat-pill b {
  color: var(--text-primary);
}
.stat-pill.warn {
  color: #f59e0b;
  border-color: rgba(245, 158, 11, 0.3);
  background: rgba(245, 158, 11, 0.08);
}
.stat-pill.warn b {
  color: #f59e0b;
}

.service-node-grid-v5 {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 10px;
}

.service-node-v5 {
  border: 1px solid var(--border);
  border-radius: 9px;
  padding: 10px 14px;
  background: var(--card-bg);
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.service-node-v5:hover {
  border-color: rgba(99, 102, 241, 0.4);
}
.service-node-v5.selected {
  border-color: var(--primary);
  box-shadow: inset 3px 0 var(--primary);
  background: rgba(99, 102, 241, 0.03);
}
.service-node-v5.is-outdated {
  border-left: 3px solid #fa8c16;
}

.card-header-v5 {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
}
.card-node-info {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  cursor: pointer;
  flex: 1;
}
.card-flag {
  font-size: 15px;
  line-height: 1;
}
.card-title-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.card-node-name {
  font-size: 13px;
  font-weight: 650;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
}
.card-tags-line {
  display: flex;
  align-items: center;
  gap: 4px;
}
.card-tag {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  font-weight: 550;
  line-height: 1.2;
}
.card-tag.region {
  background: rgba(14, 165, 233, 0.12);
  color: #0284c7;
}
.card-tag.protocol {
  background: rgba(99, 102, 241, 0.1);
  color: #6366f1;
}
:root.dark .card-tag.region {
  color: #38bdf8;
}

.card-actions-v5 {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.outcome-badge-v5 {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  font-weight: 600;
  padding: 3px 8px;
  border-radius: 999px;
  white-space: nowrap;
}
.outcome-badge-v5 .badge-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.outcome-badge-v5.good {
  background: rgba(16, 185, 129, 0.12);
  color: #10b981;
}
.outcome-badge-v5.good .badge-dot {
  background: #10b981;
  box-shadow: 0 0 4px #10b981;
}
.outcome-badge-v5.warn,
.outcome-badge-v5.limited {
  background: rgba(245, 158, 11, 0.12);
  color: #f59e0b;
}
.outcome-badge-v5.warn .badge-dot,
.outcome-badge-v5.limited .badge-dot {
  background: #f59e0b;
}
.outcome-badge-v5.bad {
  background: rgba(239, 68, 68, 0.12);
  color: #ef4444;
}
.outcome-badge-v5.bad .badge-dot {
  background: #ef4444;
}
.outcome-badge-v5.unknown {
  background: var(--card-subtle);
  color: var(--text-secondary);
}
.outcome-badge-v5.unknown .badge-dot {
  background: var(--text-muted);
}

.card-retest-btn-v5 {
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 600;
  border-radius: 5px;
  border: 1px solid var(--border);
  background: var(--card-subtle);
  color: var(--text-primary);
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.card-retest-btn-v5:hover:not(:disabled) {
  border-color: var(--primary);
  background: var(--primary);
  color: white;
}
.card-retest-btn-v5:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.card-body-v5 {
  display: flex;
  flex-direction: column;
  gap: 5px;
  border-top: 1px dashed var(--border);
  padding-top: 6px;
}
.card-evidence-line {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pill-model {
  display: inline-flex;
  align-items: center;
  font-size: 10.5px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(99, 102, 241, 0.12);
  color: #6366f1;
  flex-shrink: 0;
}
.pill-duration {
  font-size: 10.5px;
  font-family: monospace;
  color: var(--text-secondary);
  flex-shrink: 0;
}
.pill-outdated {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  background: #ffe58f;
  color: #874d00;
  flex-shrink: 0;
}
:root.dark .pill-outdated {
  background: #594214;
  color: #ffd591;
}
.evidence-desc {
  font-size: 11px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-meta-line {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 10.5px;
  color: var(--text-secondary);
}
.meta-stat b {
  color: var(--text-primary);
}
.meta-fluctuation {
  color: #f59e0b;
  font-weight: 600;
}
.meta-time {
  margin-left: auto;
}
.btn-evidence-detail {
  background: none;
  border: none;
  color: var(--primary);
  font-size: 10.5px;
  cursor: pointer;
  padding: 0;
}
.btn-evidence-detail:hover {
  text-decoration: underline;
}

.card-untested-v5 {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 6px;
  border-top: 1px dashed var(--border);
  font-size: 11px;
  color: var(--text-secondary);
}
.btn-quick-run-inline {
  background: none;
  border: 1px dashed var(--border);
  border-radius: 4px;
  padding: 2px 7px;
  font-size: 10.5px;
  color: var(--primary);
  cursor: pointer;
  transition: all 0.15s ease;
}
.btn-quick-run-inline:hover:not(:disabled) {
  background: var(--primary-subtle);
  border-color: var(--primary);
}

.service-inspector {
  position: relative;
  margin-top: 20px;
  padding: 24px;
  background: var(--card-subtle);
  border: 1px solid var(--primary);
  border-radius: 12px;
}
.service-inspector h3 {
  font-size: 18px;
  margin: 10px 0;
}
.service-inspector > span,
.service-inspector p {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 12px 0;
  line-height: 1.7;
}
.close-inspector {
  position: absolute;
  right: 16px;
  top: 12px;
  font-size: 24px;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-secondary);
}
.service-inspector dl {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px;
  margin: 15px 0;
}
.service-inspector dl div {
  padding: 10px;
  background: var(--card-bg);
  border-radius: 7px;
}
.service-inspector dt {
  font-size: 10px;
  color: var(--text-secondary);
}
.service-inspector dd {
  font-size: 12px;
  overflow-wrap: anywhere;
  margin: 5px 0 0;
}
.service-inspector summary {
  font-size: 12px;
  cursor: pointer;
}

/* Overview Mode Styles */
.overview-status {
  display: flex;
  gap: 15px;
  align-items: center;
  padding: 14px 2px;
  font-size: 13px;
  font-weight: 750;
}
.overview-status small {
  font-size: 11px;
  color: var(--text-secondary);
  font-weight: 400;
}
.service-overview {
  max-height: 650px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 11px;
}
.overview-row-v5 {
  display: grid;
  grid-template-columns: minmax(180px, 240px) 1fr;
  gap: 12px;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
}
.overview-row-v5:last-child {
  border-bottom: 0;
}
.overview-node-label {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  cursor: pointer;
}
.overview-flag {
  font-size: 14px;
}
.overview-node-text {
  min-width: 0;
}
.overview-node-text strong {
  display: block;
  font-size: 12.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.overview-node-text small {
  display: block;
  color: var(--text-secondary);
  font-size: 10.5px;
}

.overview-services-v5 {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.overview-service-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--card-bg);
  font-size: 11px;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
}
.overview-service-pill:hover {
  transform: translateY(-1px);
  border-color: var(--primary);
}
.overview-service-pill .pill-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
}
.overview-service-pill.good {
  border-color: rgba(16, 185, 129, 0.3);
  background: rgba(16, 185, 129, 0.08);
  color: #10b981;
}
.overview-service-pill.good .pill-dot {
  background: #10b981;
}
.overview-service-pill.warn,
.overview-service-pill.limited {
  border-color: rgba(245, 158, 11, 0.3);
  background: rgba(245, 158, 11, 0.08);
  color: #f59e0b;
}
.overview-service-pill.warn .pill-dot,
.overview-service-pill.limited .pill-dot {
  background: #f59e0b;
}
.overview-service-pill.bad {
  border-color: rgba(239, 68, 68, 0.25);
  background: rgba(239, 68, 68, 0.08);
  color: #ef4444;
}
.overview-service-pill.bad .pill-dot {
  background: #ef4444;
}
.overview-service-pill.unknown,
.overview-service-pill.untested {
  color: var(--text-muted);
  opacity: 0.7;
}
.overview-service-pill.unknown .pill-dot,
.overview-service-pill.untested .pill-dot {
  background: var(--text-muted);
}
.overview-service-pill .svc-name {
  color: var(--text-secondary);
}
.overview-service-pill .svc-dur {
  color: var(--text-muted);
  font-size: 9.5px;
  font-family: monospace;
}
.node-quick-test-btn {
  padding: 4px 8px;
  font-size: 11px;
  font-weight: 600;
  border-radius: 5px;
  border: 1px solid var(--border);
  background: var(--card-subtle);
  color: var(--text-primary);
  cursor: pointer;
  margin-left: auto;
  transition: all 0.15s ease;
}
.node-quick-test-btn:hover:not(:disabled) {
  border-color: var(--primary);
  background: var(--primary-subtle);
  color: var(--primary);
}

@media (max-width: 900px) {
  .results-tools {
    width: 100%;
    margin-left: 0;
  }
  .batch-action-group {
    width: 100%;
  }
  .tool-btn {
    flex: 1;
    text-align: center;
  }
}
@media (max-width: 700px) {
  .service-results {
    padding: 14px;
  }
  .service-node-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .result-summary {
    flex-wrap: wrap;
    gap: 12px;
  }
  .result-summary p {
    margin-left: 0;
  }
  .overview-row {
    display: block;
  }
  .overview-services {
    margin-top: 8px;
  }
  .outdated-banner {
    flex-direction: column;
    align-items: stretch;
  }
  .outdated-retest-btn {
    width: 100%;
  }
}

.service-nav-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  padding: 10px 14px;
  background: var(--card-bg, #ffffff);
  border: 1px solid var(--border, #e2e8f0);
  border-radius: 9px;
  margin-bottom: 14px;
}
.back-to-picker-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--card-subtle, #f8fafc);
  border: 1px solid var(--border, #cbd5e1);
  color: var(--text-main, #0f172a);
  font-size: 13px;
  font-weight: 600;
  padding: 7px 14px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.back-to-picker-btn:hover {
  background: var(--primary-subtle, #eff6ff);
  border-color: var(--primary, #3b82f6);
  color: var(--primary, #3b82f6);
}
.back-arrow {
  font-size: 15px;
}
.window-filter-group {
  display: flex;
  align-items: center;
  gap: 6px;
}
.window-filter-label {
  font-size: 12px;
  color: var(--text-secondary, #64748b);
  font-weight: 500;
}
.window-pill {
  padding: 4px 10px;
  font-size: 12px;
  border-radius: 5px;
  border: 1px solid var(--border, #cbd5e1);
  background: var(--card-bg, #ffffff);
  color: var(--text-secondary, #64748b);
  cursor: pointer;
  transition: all 0.12s ease;
}
.window-pill:hover {
  border-color: var(--primary, #3b82f6);
  color: var(--primary, #3b82f6);
}
.window-pill.active {
  background: var(--primary, #3b82f6);
  border-color: var(--primary, #3b82f6);
  color: #ffffff;
  font-weight: 600;
}
.card-sparkline-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 8px;
  background: var(--card-subtle, #f8fafc);
  border-radius: 6px;
  margin-bottom: 6px;
  border: 1px solid var(--border, #e2e8f0);
}
.sparkline-title {
  font-size: 11px;
  color: var(--text-secondary, #64748b);
  white-space: nowrap;
  font-weight: 500;
}

</style>
