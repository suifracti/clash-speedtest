<script setup lang="ts">
import { computed, ref } from 'vue'
import type { MonitorNodeOption, WorkbenchDownloadAttempt } from '../../types'
import IntraTestSamplePlot from './IntraTestSamplePlot.vue'
import UiSelect from '../common/UiSelect.vue'

const props = defineProps<{
  rows: { key: string; node: MonitorNodeOption }[]
  records: Record<string, WorkbenchDownloadAttempt[]>
  selected: string[]
  states: Record<string, string>
  partial: Record<string, { hasMore: boolean; complete: boolean }>
}>()
const emit = defineEmits<{ (e: 'toggle', key: string): void; (e: 'detail', node: MonitorNodeOption): void }>()
const order = ref('speed')
const showAllNodes = ref(false)
const expanded = ref('')
const chosen = ref<Record<string, string>>({})
function time(a: WorkbenchDownloadAttempt) { return a.result?.finished_at || a.finished_at || a.requested_at }
function history(key: string) { return [...(props.records[key] || [])].sort((a, b) => Date.parse(time(b)) - Date.parse(time(a))) }
function speed(a?: WorkbenchDownloadAttempt) {
  const result = a?.result
  return result && result.bytes_read > 0 && result.duration_ns > 0 ? result.bytes_read * 8 / (result.duration_ns / 1e9) / 1e6 : null
}
function readAmount(bytes: number): string { return bytes <= 0 ? '未收到数据' : bytes < 104858 ? `${(bytes / 1024).toFixed(1)} KiB` : `${(bytes / 1048576).toFixed(1)} MiB` }
function duration(seconds: number): string { return seconds > 0 && seconds < 0.1 ? '<0.1 s' : `${seconds.toFixed(1)} s` }
function state(a?: WorkbenchDownloadAttempt) {
  if (!a) return '未测速'
  const outcome = a.result?.outcome
  return ({ completed: '已完成', byte_limit: '已读取预设数据量', time_limit: '已达到预设时长', user_cancelled: '已取消', connection_failed: '连接异常', transfer_interrupted: '传输中断', redirect: '重定向未跟随', http_rejected: '服务拒绝' } as Record<string, string>)[outcome || ''] || (a.execution_state === 'running' ? '测速中' : '未测得速度')
}
function stamp(a: WorkbenchDownloadAttempt) { return new Date(time(a)).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) }
const allRows = computed(() => props.rows.map(row => ({ ...row, latest: history(row.key)[0], mbps: speed(history(row.key)[0]) })))
const rows = computed(() => {
  const result = allRows.value.filter(row => showAllNodes.value || !!row.latest)
  if (order.value === 'speed') result.sort((a, b) => (b.mbps ?? -1) - (a.mbps ?? -1))
  return result
})
const maximum = computed(() => Math.max(1, ...rows.value.map(row => row.mbps ?? 0)))
const measuredCount = computed(() => allRows.value.filter(row => row.mbps !== null).length)
const leader = computed(() => [...allRows.value].filter(r => r.mbps !== null).sort((a, b) => b.mbps! - a.mbps!)[0])
function selectedAttempt(key: string) { return history(key).find(a => a.attempt_id === chosen.value[key]) || history(key)[0] }
</script>

<template>
  <div class="download-comparison">
    <div v-if="leader" class="download-spotlight"><div><span>当前范围 · 最近实测</span><h3>最高下载速度</h3><p>{{ leader.node.displayName }}</p></div><strong>{{ leader.mbps!.toFixed(1) }} <small>Mbps</small></strong><div class="speed-unit-note"><b>{{ (leader.mbps! / 8).toFixed(2) }} MB/s</b><span>不同时间与测试参数下的结果仅供参考</span></div></div>
    <div class="download-comparison-toolbar"><div><strong>{{ measuredCount }} <span>/ {{ allRows.length }} 条线路有测速记录</span></strong><p>{{ measuredCount ? '默认只显示已测线路；展开可查看每次下载过程。' : selected.length ? `已选 ${selected.length} 条线路，等待开始测速。` : '还没有测速记录。先选择线路，确认预计流量后开始。' }}</p></div><div class="download-view-controls"><button type="button" class="choose-download-nodes" :aria-expanded="showAllNodes" @click="showAllNodes = !showAllNodes">{{ showAllNodes ? '收起未测线路' : `选择线路 (${allRows.length})` }}</button><UiSelect v-model="order" aria-label="下载结果排序" :options="[{value:'scope',label:'跟随节点排序'},{value:'speed',label:'速度优先'}]" /></div></div>
    <div class="download-column-labels"><span>节点</span><span>最近一次平均下载速度</span><span>实际读取 / 用时</span></div>
    <article v-for="row in rows" :key="row.key" class="download-result-row" :class="{ selected: selected.includes(row.key), unmeasured: !row.latest }">
      <div class="download-result-main">
        <label class="download-node-identity"><input type="checkbox" :aria-label="`选择 ${row.node.displayName}`" :checked="selected.includes(row.key)" @change="emit('toggle', row.key)"><span><strong>{{ row.node.displayName }}</strong><small>{{ row.node.profileName }} · {{ row.node.countryCode }}</small></span></label>
        <div class="download-speed">
          <div v-if="row.mbps !== null" class="speed-value"><strong>{{ row.mbps.toFixed(1) }} <small>Mbps</small></strong><span>≈ {{ (row.mbps / 8).toFixed(2) }} MB/s</span></div>
          <span v-else class="download-no-value">{{ states[row.key] === 'loading' ? '读取中…' : states[row.key] === 'error' ? '历史读取失败' : state(row.latest) }}</span>
          <div v-if="row.mbps !== null" class="speed-track"><i :style="{ width: `${row.mbps / maximum * 100}%` }"></i></div>
        </div>
        <div class="download-measure-meta" v-if="row.latest"><strong v-if="row.latest.result">{{ readAmount(row.latest.result.bytes_read) }} <span> / {{ duration(row.latest.result.duration_ns / 1e9) }}</span></strong><small>{{ state(row.latest) }} · {{ stamp(row.latest) }}</small><small v-if="row.latest.persistence_state !== 'saved'" class="save-warning">尚未保存</small></div><div v-else class="download-measure-meta"><small>—</small></div>
        <button v-if="row.latest" class="download-expand" :aria-expanded="expanded === row.key" @click="expanded = expanded === row.key ? '' : row.key">{{ expanded === row.key ? '收起' : '过程与历史' }}</button>
      </div>
      <div v-if="expanded === row.key && selectedAttempt(row.key)" class="download-process">
        <div class="download-process-heading"><strong>单次下载过程</strong><span>{{ stamp(selectedAttempt(row.key)) }} · {{ state(selectedAttempt(row.key)) }}</span><button @click="emit('detail', row.node)">原始记录与保存状态 →</button></div>
        <p v-if="selectedAttempt(row.key).rule" class="download-boundary">本次设定：这个节点最多读取 {{ (selectedAttempt(row.key).rule.maximum_bytes / 1048576).toFixed(0) }} MiB，最多测试 {{ selectedAttempt(row.key).rule.maximum_duration_ns / 1e9 }} 秒。达到任一条件就停止；这不是实际用量。</p>
        <IntraTestSamplePlot v-if="selectedAttempt(row.key).result?.samples.length" :key="selectedAttempt(row.key).attempt_id" mode="throughput" :throughput-samples="selectedAttempt(row.key).result!.samples" :height="150" />
        <p v-else class="download-no-value">本次没有有效过程样本，不以 0 Mbps 代替。</p>
        <div class="download-attempts" aria-label="选择下载历史"><button v-for="a in history(row.key)" :key="a.attempt_id" :aria-pressed="selectedAttempt(row.key).attempt_id === a.attempt_id" @click="chosen[row.key] = a.attempt_id"><strong>{{ speed(a)?.toFixed(1) ?? '—' }} Mbps</strong><small>{{ stamp(a) }}</small><small>{{ a.persistence_state === 'saved' ? state(a) : '尚未保存' }}</small></button></div>
        <p v-if="partial[row.key]?.hasMore" class="download-no-value">仅显示当前窗口近 100 条记录。</p>
      </div>
    </article>
    <p v-if="!rows.length" class="download-no-value empty">{{ allRows.length ? selected.length ? '已选线路尚未测速。点击上方“测速所选节点”开始。' : '还没有测速结果。点击“选择线路”勾选要测速的节点。' : '当前来源或筛选范围没有线路。' }}</p>
    <p class="download-boundary">按节点依次测速，避免相互抢带宽。平均速度来自实际读取字节与用时；本次预设的数据量、时长和目标会影响结果，不代表线路极限。未测速不计为 0。</p>
  </div>
</template>

<style scoped>
.download-comparison { padding: 22px 24px; }
.download-comparison-toolbar { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 15px; margin-bottom: 24px; }.download-comparison-toolbar strong { font-size: 22px; }.download-comparison-toolbar strong span { font-size: 12px; font-weight: 400; color: var(--text-secondary); }.download-comparison-toolbar p { font-size: 11px; color: var(--text-secondary); margin-top: 4px; }
.download-view-controls { display: flex; gap: 16px; align-items: center; font-size: 12px; }.download-view-controls label { display: flex; gap: 7px; align-items: center; }.download-view-controls select { padding: 7px 10px; background: var(--card-bg); border: 1px solid var(--border); border-radius: 6px; }
.choose-download-nodes { padding: 8px 11px; border: 1px solid var(--border); border-radius: 7px; background: var(--card-bg); color: var(--primary); font-size: 12px; font-weight: 650; }
.choose-download-nodes[aria-expanded="true"] { background: var(--primary-subtle); border-color: var(--primary); }
.download-spotlight { padding: 18px 22px; margin-bottom: 18px; min-height: 0; }
.download-column-labels, .download-result-main { display: grid; grid-template-columns: 190px minmax(160px, 1fr) 210px 80px; align-items: center; gap: 22px; }.download-column-labels { padding: 10px 0; color: var(--text-muted); font-size: 10px; border-bottom: 1px solid var(--border); }
.download-result-row { border-bottom: 1px solid var(--border); }.download-result-main { min-height: 94px; padding: 14px 0; }.download-result-row.selected { background: color-mix(in srgb, var(--primary-subtle) 30%, transparent); }
.download-result-row.unmeasured .download-result-main { min-height: 68px; }
.download-node-identity { display: flex; align-items: center; gap: 12px; font-size: 12px; }.download-node-identity small, .download-measure-meta small { display: block; font-size: 10px; color: var(--text-secondary); margin-top: 5px; }.download-node-identity strong { font-weight: 650; }
.speed-value { display: flex; align-items: baseline; gap: 12px; }.speed-value strong { font-size: 23px; font-variant-numeric: tabular-nums; letter-spacing: -.6px; }.speed-value small { font-size: 11px; font-weight: 500; letter-spacing: 0; }.speed-value > span { font-size: 11px; color: var(--text-secondary); }.speed-track { height: 7px; border-radius: 4px; background: var(--card-subtle); margin-top: 9px; }.speed-track i { display: block; height: 100%; border-radius: inherit; background: var(--primary); min-width: 2px; }
.download-measure-meta strong { font-size: 12px; font-weight: 500; }.download-measure-meta strong span { color: var(--text-secondary); }.download-measure-meta .save-warning { color: var(--warning); }.download-expand { font-size: 11px; color: var(--primary); }.download-no-value { color: var(--text-muted); font-size: 12px; }.download-no-value.empty { padding: 30px; text-align: center; }
.download-process { padding: 20px; background: var(--card-subtle); border-radius: 8px; margin-bottom: 16px; }.download-process-heading { display: flex; flex-wrap: wrap; align-items: baseline; gap: 12px; font-size: 12px; }.download-process-heading span { font-size: 10px; color: var(--text-secondary); }.download-process-heading button { margin-left: auto; color: var(--primary); font-size: 11px; }
.download-attempts { display: flex; gap: 8px; overflow-x: auto; padding-top: 12px; }.download-attempts button { min-width: 116px; padding: 9px 12px; text-align: left; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); }.download-attempts button[aria-pressed=true] { border-color: var(--primary); }.download-attempts strong { display: block; font-size: 12px; }.download-attempts small { display: block; font-size: 10px; color: var(--text-secondary); margin-top: 4px; }
.download-boundary { color: var(--text-secondary); font-size: 11px; line-height: 1.7; margin-top: 20px; }
@media(max-width: 1200px) { .download-column-labels, .download-result-main { grid-template-columns: 175px minmax(140px, 1fr) 175px; gap: 15px; }.download-expand { grid-column: 3; text-align: right; } }
@media(max-width: 700px) { .download-comparison { padding: 14px; }.download-column-labels { display: none; }.download-result-main { grid-template-columns: 1fr 1fr; }.download-expand { grid-column: auto; }.speed-value > span { display: none; } }
</style>
