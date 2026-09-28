<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { MonitorNodeOption, WorkbenchPublicServiceAttempt, WorkbenchPublicServiceRule } from '../../types'
import { serviceDetailLabel, serviceOutcomeLabel } from '../../utils/serviceOutcome'
import { serviceEvidence, serviceTitle, summarizeService } from '../../utils/servicePresentation'
import UiSelect from '../common/UiSelect.vue'
const props = defineProps<{
  rows: { key: string; node: MonitorNodeOption }[]; services: { value: string | number; label: string }[]
  catalog?: WorkbenchPublicServiceRule[]; records: Record<string, WorkbenchPublicServiceAttempt[]>
  selected: string[]; states: Record<string, string>; partial: Record<string, { hasMore: boolean; complete: boolean }>
}>()
const emit = defineEmits<{ (e: 'toggle', key: string): void; (e: 'service', id: string): void; (e: 'detail', node: MonitorNodeOption): void }>()
const focus = ref(''), showAllNodes = ref(false), inspected = ref<WorkbenchPublicServiceAttempt | null>(null)
const viewMode = ref<'all' | 'single'>('all')
const choices = computed(() => props.services.map(s => {
  const matchedRule = props.catalog?.find(r => r.service_id === s.value)
  return { value: String(s.value), label: matchedRule?.name || s.label, evidence: serviceEvidence(matchedRule) }
}))
watch(choices, list => { if (!list.some(s => s.value === focus.value)) focus.value = list[0]?.value || '' }, { immediate: true })
const rule = computed(() => props.catalog?.find(r => r.service_id === focus.value))
const viewChoice = computed({ get: () => viewMode.value === 'all' ? 'all' : focus.value, set: (value: string | number) => {
  if (value === 'all') { viewMode.value = 'all'; return }
  focus.value = String(value)
  viewMode.value = 'single'
  emit('service', focus.value)
} })
const rows = computed(() => props.rows.map(row => ({ ...row, report: summarizeService((props.records[row.key] || []).filter(a => a.service_id === focus.value)) })).filter(row => showAllNodes.value || row.report.total > 0))
const overviewRows = computed(() => props.rows.map(row => ({ ...row, reports: choices.value.map(service => ({ service, report: summarizeService((props.records[row.key] || []).filter(a => a.service_id === service.value)) })) })).filter(row => showAllNodes.value || row.reports.some(item => item.report.total > 0)))
const overviewMeasured = computed(() => overviewRows.value.filter(row => row.reports.some(item => item.report.total > 0)).length)
const measured = computed(() => rows.value.filter(r => r.report.total > 0))
const changed = computed(() => measured.value.filter(r => r.report.changes > 0).length)
function tone(a?: WorkbenchPublicServiceAttempt) {
  const outcome = a?.result?.outcome
  return !outcome ? 'unknown' : ['matched', 'profiled', 'reachable', 'unlocked'].includes(outcome) ? 'good' : ['transport_error', 'timed_out'].includes(outcome) ? 'bad' : 'limited'
}
function label(a?: WorkbenchPublicServiceAttempt) {
  if (a?.service_id === 'antigravity' && a.result?.outcome === 'matched') return '模型已回答'
  if (a?.rule.result_kind === 'exit_profile' && a.result?.outcome === 'profiled') return '出口已获取'
  return serviceOutcomeLabel(a?.result?.outcome)
}
function inconclusive(report: ReturnType<typeof summarizeService>): boolean {
  return report.total > 0 && report.passed === 0 && report.samples.every(a => ['unknown', 'timed_out', 'transport_error', 'rate_limited', 'credentials_required', 'setup_required', 'auth_failed', 'permission_denied'].includes(a.result?.outcome || ''))
}
function stamp(a: WorkbenchPublicServiceAttempt) { return new Date(a.result?.finished_at || a.finished_at || a.requested_at).toLocaleString() }
</script>
<template>
  <div class="service-results">
    <div class="results-heading"><div v-if="viewMode !== 'all'"><h3>{{ rule ? serviceTitle(rule) : '先在上方选择服务' }}</h3><p>{{ `${serviceEvidence(rule)} · ${rule?.service_id === 'antigravity' ? '只有收到真实模型内容才通过；账号、额度、地区问题分别记录。' : '通过率只衡量本项判据，不代表整个服务的可用率。'}` }}</p></div><div class="results-tools"><label>查看 <UiSelect v-model="viewChoice" aria-label="查看服务结果" :options="[{ value: 'all', label: '全部已选服务' }, ...choices]" /></label><label><input v-model="showAllNodes" type="checkbox">显示全部线路（含未测）</label></div></div>
    <template v-if="viewMode === 'all'"><div v-if="overviewMeasured" class="overview-status">{{ overviewMeasured ? `${overviewMeasured} 个节点有已加载结果` : '当前范围尚无已加载的服务结果' }}<small>未检测和无法确认都不代表节点已被证实不可用。</small></div><div v-if="overviewRows.length" class="service-overview"><div v-for="row in overviewRows" :key="row.key" class="overview-row"><label><input type="checkbox" :checked="selected.includes(row.key)" :aria-label="`选择 ${row.node.displayName}`" @change="emit('toggle', row.key)"><span><strong>{{ row.node.displayName }}</strong><small>{{ row.node.profileName }} · {{ row.node.countryCode }}</small></span></label><div class="overview-services"><button v-for="item in row.reports" :key="item.service.value" type="button" :class="['overview-service', item.report.total ? tone(item.report.latest) : 'unknown']" @click="focus = item.service.value; viewMode = 'single'; emit('service', item.service.value)"><span>{{ item.service.label }}</span><em>{{ item.service.evidence }}</em><strong>{{ item.report.total ? label(item.report.latest) : '未检测' }}</strong><small v-if="item.report.total">{{ inconclusive(item.report) ? '未得出可用性结论' : `${item.report.passed}/${item.report.total} 次通过` }}</small><small v-if="item.report.latest?.result?.details?.antigravity_progress" class="overview-reason">{{ item.report.latest.result.details.antigravity_progress }}</small><small v-else-if="item.report.latest?.result?.error_message" class="overview-reason">{{ item.report.latest.result.error_message }}</small></button></div></div></div></template>
    <template v-else><div class="result-summary"><div><strong>{{ measured.length }}</strong><span>个节点有记录</span></div><div><strong>{{ changed }}</strong><span>个节点结果有变化</span></div><p>重复检查建议 3–5 次。统计来自当前观察窗口已加载的记录；没有测过，不算失败。</p></div>
    <div class="service-node-grid">
      <article v-for="row in rows" :key="row.key" class="service-node" :class="{ selected: selected.includes(row.key) }">
        <header><label><input type="checkbox" :checked="selected.includes(row.key)" :aria-label="`选择 ${row.node.displayName}`" @change="emit('toggle', row.key)"><span><strong>{{ row.node.displayName }}</strong><small>{{ row.node.profileName }} · {{ row.node.countryCode }}</small></span></label><span class="outcome" :class="tone(row.report.latest)">{{ row.report.total ? label(row.report.latest) : states[row.key] === 'loading' ? '读取中' : states[row.key] === 'error' ? '读取失败' : '未检测' }}</span></header>
        <template v-if="row.report.total">
          <div class="service-score"><strong :class="tone(row.report.latest)">{{ inconclusive(row.report) ? '—' : row.report.rate }}<small v-if="!inconclusive(row.report)">%</small></strong><span>{{ inconclusive(row.report) ? '尚未得出结论' : '通过本项检查' }}<br><b>{{ inconclusive(row.report) ? `${row.report.total} 次未完成验证` : `${row.report.passed} / ${row.report.total} 次` }}</b></span><div><span>结果变化 <b>{{ row.report.changes }} 次</b></span><span>出口变化 <b>{{ row.report.exitPairs ? `${row.report.exitChanges} 次` : '样本不足' }}</b></span></div></div>
          <p v-if="row.report.latest?.result?.details?.antigravity_progress" class="service-stage">验证进度：{{ row.report.latest.result.details.antigravity_progress }}<template v-if="row.report.latest.result.details.checked_model"> · {{ row.report.latest.result.details.checked_model }}</template></p>
          <p v-if="row.report.total < 3" class="service-sample-warning">目前只有 {{ row.report.total }} 次记录；百分比仅描述已做的检查，不能推断长期稳定。</p>
          <p class="service-conclusion">{{ row.report.latest?.result?.summary || row.report.latest?.result?.error_message || '点击下方记录查看本次判据。' }}</p>
          <div class="status-history" aria-label="逐次检测结果"><button v-for="a in row.report.samples.slice(-24)" :key="a.attempt_id" :class="tone(a)" :title="`${stamp(a)} · ${label(a)}`" :aria-label="`${stamp(a)} ${label(a)}`" @click="inspected = a">{{ tone(a) === 'good' ? '✓' : tone(a) === 'bad' ? '×' : '!' }}</button></div>
          <footer><span>旧 → 新 · {{ stamp(row.report.latest!) }}</span><button @click="inspected = row.report.latest || null">查看依据 →</button></footer>
          <p v-if="row.report.latest?.persistence_state !== 'saved'" class="save-warning">本次结果尚未保存。<button @click="emit('detail', row.node)">打开详情处理 →</button></p>
          <small v-if="row.report.latest?.rule.rule_version !== rule?.rule_version">旧版规则的历史结果，请重新检测获取当前判据。</small>
        </template>
        <p v-else class="not-tested">{{ partial[row.key]?.hasMore ? '已加载的历史中没有本项结果' : '勾选节点，点击上方“检查服务”开始。' }}</p>
      </article>
    </div>
    </template>
    <p v-if="!(viewMode === 'all' ? overviewRows.length : rows.length)" class="results-empty">{{ selected.length ? `已选 ${selected.length} 条线路，等待开始检测；结果会在这里出现。` : '选择好服务和节点，即可开始检测。' }}结果会显示每项服务的状态与失败原因。</p>
    <p v-if="overviewMeasured || measured.length" class="results-footnote">{{ Object.values(partial).some(p => p.hasMore || !p.complete) ? '部分历史未完整加载，统计不是全量。' : '' }}一次通过不保证持续可用。出口变化仅比较相邻成功获取的 IP；Cloudflare 出口观察不等于目标网站实际看到的出口。</p>
    <section v-if="inspected" class="service-inspector" role="region" aria-label="服务检测记录"><button class="close-inspector" aria-label="关闭检测记录" @click="inspected = null">×</button><span>检测依据 · {{ stamp(inspected) }}</span><h3>{{ inspected.display_name }} · {{ inspected.rule.name }}</h3><strong class="outcome" :class="tone(inspected)">{{ label(inspected) }}</strong><p>{{ inspected.result?.summary || inspected.result?.error_message }}</p><dl><div v-for="(value, key) in inspected.result?.details" :key="key"><dt>{{ serviceDetailLabel(String(key)) }}</dt><dd>{{ value || '未提供' }}</dd></div></dl><details><summary>技术信息与保存状态</summary><p>HTTP {{ inspected.result?.http_status ?? '无响应' }} · {{ inspected.result?.duration_ms ?? '—' }} ms · {{ inspected.persistence_state === 'saved' ? '已保存' : '尚未保存' }}</p><p>{{ inspected.rule.success_criterion }}</p><p>{{ inspected.persistence_error }}</p></details></section>
  </div>
</template>
<style scoped>
.service-results{padding:24px}.results-heading{display:flex;justify-content:space-between;gap:20px;flex-wrap:wrap}.results-heading>div>span{font-size:11px;letter-spacing:1px;color:var(--primary);font-weight:750}.results-heading h3{font-size:24px;margin:6px 0;font-weight:750}.results-heading p{font-size:12px;color:var(--text-secondary);max-width:620px}.results-tools{display:flex;flex-direction:column;align-items:flex-end;gap:12px}.results-tools label{font-size:12px;display:flex;align-items:center;gap:7px}.result-summary{display:flex;align-items:center;gap:32px;padding:20px 0;margin:12px 0;border-bottom:1px solid var(--border)}.result-summary>div{display:flex;align-items:baseline;gap:8px;white-space:nowrap}.result-summary strong{font-size:27px}.result-summary span,.result-summary p{font-size:11px;color:var(--text-secondary)}.result-summary p{margin-left:auto;max-width:420px;line-height:1.7}.service-node-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(320px,1fr));gap:14px}.service-node{border:1px solid var(--border);border-radius:12px;padding:18px}.service-node.selected{border-color:var(--primary);box-shadow:inset 3px 0 var(--primary)}.service-node header{display:flex;justify-content:space-between;gap:12px;align-items:center}.service-node label{display:flex;gap:10px;align-items:center;min-width:0}.service-node label span{min-width:0}.service-node label strong{font-size:13px;display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.service-node small{font-size:10px;color:var(--text-secondary);display:block;margin-top:4px}.outcome{font-size:10px;padding:5px 8px;border-radius:5px;flex-shrink:0;max-width:125px}.good{color:var(--success);background:var(--success-bg)}.limited{color:var(--warning);background:var(--warning-bg)}.bad{color:var(--danger);background:var(--danger-bg)}.unknown{color:var(--text-secondary);background:var(--card-subtle)}.service-score{display:flex;gap:12px;align-items:center;margin:22px 0 14px}.service-score>strong{font-size:36px;background:none;letter-spacing:-1px}.service-score strong small{display:inline;font-size:17px;color:inherit}.service-score>span{font-size:11px;color:var(--text-secondary);line-height:1.8}.service-score>div{margin-left:auto;display:grid;gap:7px;font-size:10px}.service-conclusion{font-size:11px;color:var(--text-secondary);line-height:1.7;min-height:36px}.status-history{display:flex;gap:4px;flex-wrap:wrap;margin:13px 0}.status-history button{width:22px;height:25px;border-radius:4px;font-size:12px;font-weight:750}.service-node footer{display:flex;justify-content:space-between;font-size:10px;color:var(--text-secondary)}.service-node footer button{color:var(--primary)}.not-tested{font-size:11px;color:var(--text-secondary);margin:14px 0 0}.results-footnote{font-size:11px;line-height:1.8;color:var(--text-secondary);margin-top:20px}.results-empty{padding:24px;color:var(--text-secondary);font-size:13px}.save-warning{font-size:11px;color:var(--warning)}.service-inspector{position:relative;margin-top:20px;padding:24px;background:var(--card-subtle);border:1px solid var(--primary);border-radius:12px}.service-inspector h3{font-size:18px;margin:10px 0}.service-inspector>span,.service-inspector p{font-size:12px;color:var(--text-secondary);margin:12px 0;line-height:1.7}.close-inspector{position:absolute;right:16px;top:12px;font-size:24px}.service-inspector dl{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px;margin:15px 0}.service-inspector dl div{padding:10px;background:var(--card-bg);border-radius:7px}.service-inspector dt{font-size:10px;color:var(--text-secondary)}.service-inspector dd{font-size:12px;overflow-wrap:anywhere;margin:5px 0 0}.service-inspector summary{font-size:12px;cursor:pointer}@media(max-width:700px){.service-results{padding:14px}.service-node-grid{grid-template-columns:minmax(0,1fr)}.result-summary{flex-wrap:wrap;gap:12px}.result-summary p{margin-left:0}.results-tools{align-items:flex-start}}
</style>
<style scoped>
.result-tabs{display:flex;gap:6px;overflow:auto;padding:16px 0 12px;border-bottom:1px solid var(--border)}
.result-tabs button{flex:none;border:1px solid var(--border);background:var(--card-bg);border-radius:8px;padding:8px 12px;font-size:12px;color:var(--text-secondary)}
.result-tabs button.active{background:#6550a2;border-color:#6550a2;color:white;font-weight:750}
.results-tools{flex-direction:row;align-items:center;flex-wrap:wrap;margin-left:auto}
.results-empty{margin:18px 0 0;padding:40px 20px;text-align:center;background:var(--card-subtle);border-radius:8px;font-size:14px;line-height:1.8}
.results-heading:has(> .results-tools:only-child){justify-content:flex-end}
.service-results{padding:16px 20px}
.overview-status{display:flex;gap:15px;align-items:center;padding:17px 2px;font-size:13px;font-weight:750}
.overview-status small{font-size:11px;color:var(--text-secondary);font-weight:400}
.service-overview{max-height:650px;overflow:auto;border:1px solid var(--border);border-radius:11px}
.overview-row{display:grid;grid-template-columns:minmax(160px,210px) minmax(0,1fr);gap:12px;align-items:center;padding:11px 13px;border-bottom:1px solid var(--border)}
.overview-row:last-child{border-bottom:0}
.overview-row>label{display:flex;align-items:center;gap:8px;min-width:0;font-size:12px}
.overview-row>label span{min-width:0}
.overview-row>label strong{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.overview-row>label small{display:block;margin-top:3px;color:var(--text-secondary);font-size:10px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.overview-services{display:flex;flex-wrap:wrap;gap:7px}
.overview-service{display:flex;align-items:center;gap:6px;flex-wrap:wrap;border:1px solid currentColor;border-radius:8px;padding:7px 9px;font-size:11px;text-align:left}
.overview-service span{font-weight:750}.overview-service strong{font-size:11px}.overview-service small{opacity:.8}
.overview-service em{font-style:normal;font-size:10px;color:var(--text-secondary);border:1px solid var(--border);border-radius:4px;padding:1px 4px}.overview-service .overview-reason{flex-basis:100%;line-height:1.35;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.service-stage,.service-sample-warning{font-size:11px;line-height:1.5;color:var(--text-secondary);margin:5px 0}.service-stage{color:var(--primary)}
@media(max-width:700px){.overview-row{display:block}.overview-services{margin-top:8px}}
</style>
