<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { MonitorNodeOption, WorkbenchPublicServiceAttempt, WorkbenchPublicServiceRule } from '../../types'
import { serviceDetailLabel, serviceOutcomeLabel } from '../../utils/serviceOutcome'
import { serviceEvidence, serviceTitle, summarizeService } from '../../utils/servicePresentation'
import UiSelect from '../common/UiSelect.vue'

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
}>()

const focus = ref('')
const showAllNodes = ref(false)
const inspected = ref<WorkbenchPublicServiceAttempt | null>(null)
const viewMode = ref<'all' | 'single'>('all')
const sortBy = ref<'region' | 'pass_rate' | 'status' | 'recent' | 'changes' | 'name'>('region')

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

// Current selection helpers
const currentKeys = computed(() =>
  (viewMode.value === 'all' ? sortedOverviewRows.value : sortedRows.value).map(r => r.key),
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
      <div v-if="viewMode !== 'all'">
        <h3>{{ rule ? serviceTitle(rule) : '先在上方选择服务' }}</h3>
        <p>
          {{
            `${serviceEvidence(rule)} · ${
              rule?.service_id === 'antigravity'
                ? '只有收到真实模型内容才通过；账号、额度、地区问题分别记录。'
                : '通过率只衡量本项判据，不代表整个服务的可用率。'
            }`
          }}
        </p>
      </div>
      <div class="results-tools">
        <label>查看 <UiSelect v-model="viewChoice" aria-label="查看服务结果" :options="[{ value: 'all', label: '全部已选服务' }, ...choices]" /></label>
        <label>排序 <UiSelect v-model="sortBy" aria-label="排序服务结果" :options="sortOptions" /></label>
        <label class="checkbox-inline"><input v-model="showAllNodes" type="checkbox">显示全部线路（含未测）</label>
        <div class="batch-action-group">
          <button type="button" class="tool-btn" @click="toggleSelectAllCurrent">
            {{ isAllSelected ? '取消全选' : `全选（${currentKeys.length}）` }}
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
        {{ overviewMeasured ? `${overviewMeasured} 个节点有已加载结果` : '当前范围尚无已加载的服务结果' }}
        <small>未检测和无法确认都不代表节点已被证实不可用。</small>
      </div>
      <div v-if="sortedOverviewRows.length" class="service-overview">
        <div v-for="row in sortedOverviewRows" :key="row.key" class="overview-row">
          <label>
            <input type="checkbox" :checked="selected.includes(row.key)" :aria-label="`选择 ${row.node.displayName}`" @change="emit('toggle', row.key)">
            <span>
              <strong>{{ row.node.displayName }}</strong>
              <small>{{ row.node.profileName }} · {{ row.node.countryCode }}</small>
            </span>
          </label>
          <div class="overview-services">
            <button
              v-for="item in row.reports"
              :key="item.service.value"
              type="button"
              :class="['overview-service', item.report.total ? tone(item.report.latest) : 'unknown']"
              @click="focus = item.service.value; viewMode = 'single'; emit('service', item.service.value)"
            >
              <span>{{ item.service.label }}</span>
              <em>{{ item.service.evidence }}</em>
              <strong>{{ item.report.total ? label(item.report.latest) : '未检测' }}</strong>
              <small v-if="item.report.total">
                {{ inconclusive(item.report) ? '未得出可用性结论' : `${item.report.passed}/${item.report.total} 次通过` }}
              </small>
              <small v-if="item.report.latest?.result?.details?.antigravity_progress" class="overview-reason">
                {{ item.report.latest.result.details.antigravity_progress }}
              </small>
              <small v-else-if="item.report.latest?.result?.error_message" class="overview-reason">
                {{ item.report.latest.result.error_message }}
              </small>
            </button>
            <button type="button" class="node-quick-test-btn" :disabled="running" title="针对此节点立即执行检测" @click.stop="emit('run', [row.key])">
              ⚡ 检测
            </button>
          </div>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="result-summary">
        <div><strong>{{ measured.length }}</strong><span>个节点有记录</span></div>
        <div><strong>{{ changed }}</strong><span>个节点结果有变化</span></div>
        <p>重复检查建议 3–5 次。统计来自当前观察窗口已加载的记录；没有测过，不算失败。</p>
      </div>

      <div class="service-node-grid">
        <article
          v-for="row in sortedRows"
          :key="row.key"
          class="service-node"
          :class="{ selected: selected.includes(row.key) }"
        >
          <header>
            <label>
              <input type="checkbox" :checked="selected.includes(row.key)" :aria-label="`选择 ${row.node.displayName}`" @change="emit('toggle', row.key)">
              <span>
                <strong>{{ row.node.displayName }}</strong>
                <small>{{ row.node.profileName }} · {{ row.node.countryCode }}</small>
              </span>
            </label>
            <div class="node-header-actions">
              <span class="outcome" :class="tone(row.report.latest)">
                {{ row.report.total ? label(row.report.latest) : states[row.key] === 'loading' ? '读取中' : states[row.key] === 'error' ? '读取失败' : '未检测' }}
              </span>
              <button type="button" class="card-retest-btn" :disabled="running" title="针对此节点立即执行本项检测" @click.stop="emit('run', [row.key])">
                ⚡ 检测
              </button>
            </div>
          </header>

          <template v-if="row.report.total">
            <div class="service-score">
              <strong :class="tone(row.report.latest)">
                {{ inconclusive(row.report) ? '—' : row.report.rate }}<small v-if="!inconclusive(row.report)">%</small>
              </strong>
              <span>
                {{ inconclusive(row.report) ? '尚未得出结论' : '通过本项检查' }}<br>
                <b>{{ inconclusive(row.report) ? `${row.report.total} 次未完成验证` : `${row.report.passed} / ${row.report.total} 次` }}</b>
              </span>
              <div>
                <span>结果变化 <b>{{ row.report.changes }} 次</b></span>
                <span>出口变化 <b>{{ row.report.exitPairs ? `${row.report.exitChanges} 次` : '样本不足' }}</b></span>
              </div>
            </div>

            <p v-if="row.report.latest?.result?.details?.antigravity_progress" class="service-stage">
              验证进度：{{ row.report.latest.result.details.antigravity_progress }}
              <template v-if="row.report.latest.result.details.checked_model"> · {{ row.report.latest.result.details.checked_model }}</template>
            </p>

            <p v-if="row.report.total < 3" class="service-sample-warning">
              目前只有 {{ row.report.total }} 次记录；百分比仅描述已做的检查，不能推断长期稳定。
            </p>

            <p class="service-conclusion">
              {{ row.report.latest?.result?.summary || row.report.latest?.result?.error_message || '点击下方记录查看本次判据。' }}
            </p>

            <div class="status-history" aria-label="逐次检测结果">
              <button
                v-for="a in row.report.samples.slice(-24)"
                :key="a.attempt_id"
                :class="tone(a)"
                :title="`${stamp(a)} · ${label(a)}`"
                :aria-label="`${stamp(a)} ${label(a)}`"
                @click="inspected = a"
              >
                {{ tone(a) === 'good' ? '✓' : tone(a) === 'bad' ? '×' : '!' }}
              </button>
            </div>

            <footer>
              <span>旧 → 新 · {{ stamp(row.report.latest!) }}</span>
              <button type="button" class="view-evidence-btn" @click="inspected = row.report.latest || null">查看依据 →</button>
            </footer>

            <p v-if="row.report.latest?.persistence_state !== 'saved'" class="save-warning">
              本次结果尚未保存。<button type="button" @click="emit('detail', row.node)">打开详情处理 →</button>
            </p>

            <div v-if="row.report.latest?.rule.rule_version !== rule?.rule_version" class="outdated-card-note">
              <span>旧版规则历史结果 (v{{ row.report.latest?.rule.rule_version }})</span>
              <button type="button" class="inline-retest-link" :disabled="running" @click.stop="emit('run', [row.key])">以新规则重测 →</button>
            </div>
          </template>

          <p v-else class="not-tested">
            {{ partial[row.key]?.hasMore ? '已加载的历史中没有本项结果' : '勾选节点，点击上方“开始检测”或卡片右上角“⚡ 检测”开始。' }}
          </p>
        </article>
      </div>
    </template>

    <p v-if="!(viewMode === 'all' ? sortedOverviewRows.length : sortedRows.length)" class="results-empty">
      {{ selected.length ? `已选 ${selected.length} 条线路，等待开始检测；结果会在这里出现。` : '选择好服务和节点，即可开始检测。' }}结果会显示每项服务的状态与失败原因。
    </p>

    <p v-if="overviewMeasured || measured.length" class="results-footnote">
      {{ Object.values(partial).some(p => p.hasMore || !p.complete) ? '部分历史未完整加载，统计不是全量。' : '' }}一次通过不保证持续可用。出口变化仅比较相邻成功获取的 IP；Cloudflare 出口观察不等于目标网站实际看到的出口。
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

.result-summary {
  display: flex;
  align-items: center;
  gap: 32px;
  padding: 16px 0;
  margin: 8px 0 14px;
  border-bottom: 1px solid var(--border);
}
.result-summary > div {
  display: flex;
  align-items: baseline;
  gap: 8px;
  white-space: nowrap;
}
.result-summary strong {
  font-size: 27px;
}
.result-summary span,
.result-summary p {
  font-size: 11px;
  color: var(--text-secondary);
}
.result-summary p {
  margin-left: auto;
  max-width: 420px;
  line-height: 1.7;
}

.service-node-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}
.service-node {
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 16px;
  background: var(--card-bg);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}
.service-node.selected {
  border-color: var(--primary);
  box-shadow: inset 3px 0 var(--primary);
}
.service-node header {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
}
.service-node label {
  display: flex;
  gap: 10px;
  align-items: center;
  min-width: 0;
  cursor: pointer;
}
.service-node label span {
  min-width: 0;
}
.service-node label strong {
  font-size: 13px;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.service-node small {
  font-size: 10px;
  color: var(--text-secondary);
  display: block;
  margin-top: 3px;
}

.node-header-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.card-retest-btn {
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 600;
  border-radius: 5px;
  border: 1px solid var(--border);
  background: var(--card-subtle);
  color: var(--text-primary);
  cursor: pointer;
  transition: all 0.15s ease;
}
.card-retest-btn:hover:not(:disabled) {
  border-color: var(--primary);
  background: var(--primary-subtle);
  color: var(--primary);
}
.card-retest-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.outcome {
  font-size: 10px;
  padding: 4px 7px;
  border-radius: 5px;
  flex-shrink: 0;
  max-width: 125px;
}
.good {
  color: var(--success);
  background: var(--success-bg);
}
.limited {
  color: var(--warning);
  background: var(--warning-bg);
}
.bad {
  color: var(--danger);
  background: var(--danger-bg);
}
.unknown {
  color: var(--text-secondary);
  background: var(--card-subtle);
}

.service-score {
  display: flex;
  gap: 12px;
  align-items: center;
  margin: 18px 0 12px;
}
.service-score > strong {
  font-size: 34px;
  background: none;
  letter-spacing: -1px;
}
.service-score strong small {
  display: inline;
  font-size: 16px;
  color: inherit;
}
.service-score > span {
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.8;
}
.service-score > div {
  margin-left: auto;
  display: grid;
  gap: 5px;
  font-size: 10px;
}
.service-conclusion {
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.6;
  min-height: 34px;
}
.status-history {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  margin: 10px 0;
}
.status-history button {
  width: 22px;
  height: 25px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 750;
  border: none;
  cursor: pointer;
}
.service-node footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 10px;
  color: var(--text-secondary);
  margin-top: 8px;
}
.view-evidence-btn {
  background: none;
  border: none;
  color: var(--primary);
  cursor: pointer;
  font-size: 10px;
  padding: 0;
}
.view-evidence-btn:hover {
  text-decoration: underline;
}
.not-tested {
  font-size: 11px;
  color: var(--text-secondary);
  margin: 14px 0 0;
  line-height: 1.6;
}
.results-footnote {
  font-size: 11px;
  line-height: 1.8;
  color: var(--text-secondary);
  margin-top: 20px;
}
.results-empty {
  padding: 40px 20px;
  text-align: center;
  background: var(--card-subtle);
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.8;
  color: var(--text-secondary);
  margin-top: 16px;
}
.save-warning {
  font-size: 11px;
  color: var(--warning);
}
.save-warning button {
  background: none;
  border: none;
  color: var(--primary);
  cursor: pointer;
}

.outdated-card-note {
  margin-top: 8px;
  padding: 6px 10px;
  background: #fffbe6;
  border: 1px solid #ffe58f;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  font-size: 10px;
  color: #874d00;
}
:root.dark .outdated-card-note {
  background: #2b2111;
  border-color: #594214;
  color: #ffd591;
}
.inline-retest-link {
  background: none;
  border: none;
  color: #fa8c16;
  font-weight: 700;
  cursor: pointer;
  padding: 0;
}
.inline-retest-link:hover {
  text-decoration: underline;
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
.overview-row {
  display: grid;
  grid-template-columns: minmax(160px, 210px) minmax(0, 1fr);
  gap: 12px;
  align-items: center;
  padding: 11px 13px;
  border-bottom: 1px solid var(--border);
}
.overview-row:last-child {
  border-bottom: 0;
}
.overview-row > label {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 12px;
  cursor: pointer;
}
.overview-row > label span {
  min-width: 0;
}
.overview-row > label strong {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.overview-row > label small {
  display: block;
  margin-top: 3px;
  color: var(--text-secondary);
  font-size: 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.overview-services {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
}
.overview-service {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  border: 1px solid currentColor;
  border-radius: 8px;
  padding: 6px 9px;
  font-size: 11px;
  text-align: left;
  background: var(--card-bg);
  cursor: pointer;
}
.overview-service span {
  font-weight: 750;
}
.overview-service strong {
  font-size: 11px;
}
.overview-service small {
  opacity: 0.8;
}
.overview-service em {
  font-style: normal;
  font-size: 10px;
  color: var(--text-secondary);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 1px 4px;
}
.overview-service .overview-reason {
  flex-basis: 100%;
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.node-quick-test-btn {
  padding: 5px 10px;
  font-size: 11px;
  font-weight: 600;
  border-radius: 6px;
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
.service-stage,
.service-sample-warning {
  font-size: 11px;
  line-height: 1.5;
  color: var(--text-secondary);
  margin: 5px 0;
}
.service-stage {
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
</style>
