<script setup lang="ts">
import { onMounted, ref } from 'vue'
import * as api from '../../api/bridge'
import IntraTestSamplePlot from '../workbench/IntraTestSamplePlot.vue'
import type { NodeDetailRequest, WorkbenchLatencyBatch, WorkbenchLatencyBatchItem } from '../../types'

const emit = defineEmits<{
  (event: 'open-node-detail', payload: NodeDetailRequest): void
}>()

const recentBatches = ref<WorkbenchLatencyBatch[]>([])
const displayedBatch = ref<WorkbenchLatencyBatch | null>(null)
const loading = ref(false)
const historyError = ref('')
let detailRequest = 0

async function loadBatches(): Promise<void> {
  loading.value = true
  historyError.value = ''
  try {
    const batches = await api.fetchWorkbenchLatencyBatches(30)
    recentBatches.value = batches
    const id = batches.find(batch => batch.batch_id === displayedBatch.value?.batch_id)?.batch_id || batches[0]?.batch_id
    if (id) await selectBatch(id)
    else { displayedBatch.value = null; detailRequest++ }
  } catch (err) {
    historyError.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function selectBatch(batchID: string): Promise<void> {
  const request = ++detailRequest
  historyError.value = ''
  try {
    const detail = await api.fetchWorkbenchLatencyBatch(batchID)
    if (request === detailRequest) displayedBatch.value = detail
  } catch (err) {
    if (request === detailRequest) historyError.value = err instanceof Error ? err.message : String(err)
  }
}

async function retryBatchSave(item: WorkbenchLatencyBatchItem): Promise<void> {
  if (!displayedBatch.value) return
  try {
    const updated = await api.retryWorkbenchLatencyBatchItem(displayedBatch.value.batch_id, item.item_id)
    displayedBatch.value = updated
    const idx = recentBatches.value.findIndex((b) => b.batch_id === updated.batch_id)
    if (idx >= 0) recentBatches.value[idx] = updated
  } catch (err) {
    historyError.value = err instanceof Error ? err.message : String(err)
  }
}

function openBatchItemDetail(item: WorkbenchLatencyBatchItem): void {
  if (!item.node_identity_key || !item.config_revision_key) return
  emit('open-node-detail', {
    profileId: item.profile_id,
    nodeKey: item.node_key,
    nodeIdentityKey: item.node_identity_key,
    configRevisionKey: item.config_revision_key,
    displayName: item.display_name || item.node_key,
    nodeType: item.node_type,
    origin: {
      kind: 'workbench_batch_item',
      batchId: displayedBatch.value?.batch_id || '',
      itemId: item.item_id,
      attemptId: item.attempt_id,
      observedAt: item.result?.finished_at || item.requested_at,
    },
  })
}

function batchStatusLabel(state: string): string {
  return ({
    queued: '排队中',
    running: '执行中',
    cancelling: '正在取消',
    saving: '结果保存中',
    completed: '完成',
    completed_with_save_failures: '部分保存失败',
    completed_with_issues: '存在异常',
    failed: '探测失败',
    cancelled: '已取消',
  } as Record<string, string>)[state] || state
}

function batchExecutionLabel(state: string): string {
  return ({
    queued: '排队中',
    running: '执行中',
    completed: '完成',
    failed: '探测失败',
    cancelled: '已取消',
    skipped_config: '配置已变化，已跳过',
    not_executed: '未执行',
    interrupted: '应用退出时中断',
  } as Record<string, string>)[state] || state
}

function batchPersistenceLabel(state: string): string {
  return ({
    pending: '等待保存',
    saving: '保存中',
    saved: '已保存',
    failed: '保存失败',
    not_applicable: '不适用',
  } as Record<string, string>)[state] || state
}

function formatTime(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

onMounted(loadBatches)
</script>

<template>
  <div class="batch-history-panel">
    <div class="batch-history-header">
      <div>
        <h3>手动延迟测试记录</h3>
        <p>主动延迟测试的已保存批次。下载与公共服务记录在节点工作台对应项目中查看。</p>
      </div>
      <button type="button" class="prototype-button" :disabled="loading" @click="loadBatches">
        {{ loading ? '读取中…' : '重新读取历史' }}
      </button>
    </div>

    <div v-if="historyError" class="inline-error">{{ historyError }}</div>

    <div class="batch-history-layout">
      <!-- Left side: batches list -->
      <aside class="batch-sidebar">
        <div v-if="recentBatches.length === 0 && !loading" class="batch-empty-tip">
          尚无批量测速历史记录。
        </div>
        <button
          v-for="batch in recentBatches"
          :key="batch.batch_id"
          type="button"
          class="batch-sidebar-item"
          :class="{ selected: displayedBatch?.batch_id === batch.batch_id }"
          @click="selectBatch(batch.batch_id)"
        >
          <div class="batch-sidebar-top">
            <strong :class="`state-${batch.state}`">{{ batchStatusLabel(batch.state) }}</strong>
            <small>{{ formatTime(batch.requested_at) }}</small>
          </div>
          <div class="batch-sidebar-sub">
            <span>{{ batch.item_count }} 个节点</span>
            <span>超时 {{ batch.timeout_seconds }}s</span>
          </div>
        </button>
      </aside>

      <!-- Right side: batch detail -->
      <main class="batch-main">
        <div v-if="displayedBatch" class="batch-detail-card">
          <div class="batch-card-header">
            <div>
              <h4>批次 {{ displayedBatch.batch_id }}</h4>
              <p>
                状态：{{ batchStatusLabel(displayedBatch.state) }} ·
                {{ displayedBatch.items?.filter((i) => ['completed', 'failed', 'cancelled', 'skipped_config', 'not_executed', 'interrupted'].includes(i.execution_state)).length || 0 }} / {{ displayedBatch.item_count }} 项已结束
                · 请求于 {{ formatTime(displayedBatch.requested_at) }}
              </p>
            </div>
          </div>

          <div class="batch-items-container">
            <div v-for="item in displayedBatch.items || []" :key="item.item_id" class="batch-item-card">
              <div class="batch-item-left">
                <strong>{{ item.display_name || item.node_key }}</strong>
                <span>{{ item.node_type || '节点' }} · {{ item.profile_id }}</span>
                <code>{{ item.node_identity_key }} · rev {{ item.config_revision_key }}</code>
              </div>

              <div class="batch-item-center">
                <span class="badge" :class="item.execution_state">{{ item.execution_state === 'completed' && item.result?.failure_samples ? '测量完成 · 部分请求失败' : batchExecutionLabel(item.execution_state) }}</span>
                <span class="badge-persist">{{ batchPersistenceLabel(item.persistence_state) }}</span>
                <span v-if="item.error_message" class="item-error">{{ item.result?.success_samples ? '失败请求原因：' : '' }}{{ item.error_message }}</span>
                <span v-if="item.persistence_error" class="item-error">{{ item.persistence_error }}</span>
                <button
                  v-if="item.node_identity_key && item.config_revision_key"
                  type="button"
                  class="text-link"
                  @click="openBatchItemDetail(item)"
                >
                  节点详情 / 原批次项 →
                </button>
              </div>

              <div v-if="item.result" class="batch-item-right">
                <div class="result-stats">
                  <strong>{{ item.result.success_samples }} 成功 / {{ item.result.failure_samples }} 失败</strong>
                  <span v-if="item.result.target === 'multi://latency-v1'">六站综合检测 · 查看记录获取各站结果</span>
                  <span v-else>延迟 {{ item.result.latency_ms > 0 ? `${Math.round(item.result.latency_ms)} ms` : '无有效值' }} · jitter {{ item.result.jitter_ms }} ms</span>
                </div>
                <div v-if="item.result.samples?.length" class="result-plot-wrap">
                  <IntraTestSamplePlot
                    mode="latency"
                    :latency-samples="item.result.samples"
                    :height="60"
                    :compact="true"
                  />
                </div>
                <button
                  v-if="item.persistence_state === 'failed'"
                  type="button"
                  class="retry-save-btn"
                  @click="retryBatchSave(item)"
                >
                  重试保存（沿用原 attempt）
                </button>
              </div>

              <div v-else class="batch-item-right empty-result">
                {{ item.execution_state === 'skipped_config' ? '未发出请求；所选身份或 revision 已失效。' : item.execution_state === 'cancelled' || item.execution_state === 'not_executed' ? '未测，不计作节点探测失败。' : item.error_message || '等待结果' }}
              </div>
            </div>
          </div>
        </div>

        <div v-else class="batch-select-hint">
          请在左侧选择一个历史批次查看详情。
        </div>
      </main>
    </div>
  </div>
</template>

<style scoped>
.batch-history-panel { display: flex; flex-direction: column; gap: 14px; padding: 18px 20px; }
.batch-history-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
.batch-history-header h3 { margin: 0 0 4px; font-size: 16px; }
.batch-history-header p { margin: 0; color: var(--text-secondary); font-size: 12px; }
.inline-error { padding: 8px 12px; border-left: 3px solid var(--danger); background: var(--danger-bg); color: var(--danger); font-size: 12px; }
.batch-history-layout { display: grid; grid-template-columns: 280px minmax(0, 1fr); gap: 16px; border: 1px solid var(--border); border-radius: 10px; background: var(--card-bg); overflow: hidden; min-height: 480px; }
.batch-sidebar { display: flex; flex-direction: column; border-right: 1px solid var(--border); background: var(--card-subtle); overflow-y: auto; max-height: 680px; }
.batch-sidebar-item { display: flex; flex-direction: column; gap: 4px; padding: 10px 14px; border: 0; border-bottom: 1px solid var(--border); background: transparent; text-align: left; cursor: pointer; transition: background .15s ease; color: var(--text-main); }
.batch-sidebar-item:hover { background: var(--card-hover); }
.batch-sidebar-item.selected { background: var(--primary-subtle); border-left: 3px solid var(--primary); }
.batch-sidebar-top { display: flex; justify-content: space-between; align-items: baseline; font-size: 12px; }
.batch-sidebar-top small { color: var(--text-muted); font-size: 11px; }
.batch-sidebar-sub { display: flex; justify-content: space-between; color: var(--text-secondary); font-size: 11px; }
.batch-empty-tip, .batch-select-hint { padding: 24px; color: var(--text-muted); font-size: 13px; text-align: center; }
.batch-main { padding: 16px 20px; overflow-y: auto; max-height: 680px; }
.batch-card-header h4 { margin: 0 0 4px; font-size: 15px; }
.batch-card-header p { margin: 0 0 14px; color: var(--text-secondary); font-size: 11px; }
.batch-items-container { display: flex; flex-direction: column; gap: 10px; }
.batch-item-card { display: grid; grid-template-columns: minmax(180px, .9fr) minmax(140px, .6fr) minmax(240px, 1.4fr); gap: 12px; align-items: start; padding: 10px 12px; border: 1px solid var(--border); border-radius: 8px; background: var(--card-subtle); font-size: 11px; }
.batch-item-left { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.batch-item-left strong { font-size: 13px; color: var(--text-main); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.batch-item-left span { color: var(--text-secondary); }
.batch-item-left code { font-size: 9px; color: var(--text-muted); overflow-wrap: anywhere; }
.batch-item-center { display: flex; flex-direction: column; gap: 4px; }
.badge { display: inline-block; width: fit-content; padding: 2px 6px; border-radius: 4px; font-size: 10px; font-weight: 700; background: var(--border); color: var(--text-main); }
.badge.completed { background: var(--primary-subtle); color: var(--primary); }
.badge.failed { background: var(--danger-bg); color: var(--danger); }
.badge-persist { color: var(--text-muted); font-size: 10px; }
.item-error { color: var(--danger); }
.text-link { border: 0; padding: 0; background: transparent; color: var(--primary); font-size: 11px; cursor: pointer; text-align: left; margin-top: 4px; }
.batch-item-right { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.result-stats { display: flex; flex-direction: column; gap: 2px; }
.result-stats strong { color: var(--text-main); font-size: 12px; }
.result-stats span { color: var(--text-secondary); }
.result-plot-wrap { width: 100%; border-radius: 6px; overflow: hidden; }
.retry-save-btn { align-self: flex-start; padding: 3px 8px; border: 1px solid var(--border); border-radius: 4px; background: var(--card-bg); color: var(--primary); font-size: 11px; cursor: pointer; }
.empty-result { color: var(--text-muted); display: flex; align-items: center; }
.prototype-button { display: inline-flex; align-items: center; justify-content: center; min-height: 32px; padding: 5px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-subtle); color: var(--text-main); font-size: 12px; font-weight: 600; cursor: pointer; transition: all .15s ease; }
.prototype-button:hover:not(:disabled) { border-color: var(--border-focus); color: var(--primary); }
@media (max-width: 860px) {
  .batch-history-layout { grid-template-columns: 1fr; }
  .batch-sidebar { max-height: 220px; }
  .batch-item-card { grid-template-columns: 1fr; }
}
</style>
