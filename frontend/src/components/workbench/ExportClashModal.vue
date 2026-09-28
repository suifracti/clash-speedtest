<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import * as api from '../../api/bridge'
import type { MonitorNodeOption } from '../../types'
import UiSelect from '../common/UiSelect.vue'

const props = defineProps<{
  visible: boolean
  selectedKeys: string[]
  nodes: MonitorNodeOption[]
  getLatency: (key: string) => number | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const groupName = ref('PROXY')
const exportScope = ref<'selected' | 'top10' | 'all_alive'>('selected')
const exportScopeOptions = computed(() => [
  { value: 'selected', label: `当前勾选节点 (${props.selectedKeys.length} 个)`, disabled: props.selectedKeys.length === 0 },
  { value: 'top10', label: '最低延迟 Top 10 节点' },
  { value: 'all_alive', label: '所有存活可用节点' },
])
const yamlContent = ref('')
const loading = ref(false)
const error = ref('')
const copied = ref(false)

const effectiveNodeKeys = computed(() => {
  if (exportScope.value === 'selected' && props.selectedKeys.length > 0) {
    return props.selectedKeys
  }
  if (exportScope.value === 'top10') {
    // Sort nodes with valid latency ascending
    const withLatency = props.nodes
      .map(n => ({ key: n.nodeKey, latency: props.getLatency(n.nodeKey) }))
      .filter((item): item is { key: string; latency: number } => item.latency !== null && item.latency > 0)
      .sort((a, b) => a.latency - b.latency)
      .slice(0, 10)
    return withLatency.map(item => item.key)
  }
  // All alive (with valid latency) or all nodes if no latency tested
  const alive = props.nodes
    .filter(n => {
      const lat = props.getLatency(n.nodeKey)
      return lat !== null && lat > 0
    })
    .map(n => n.nodeKey)
  return alive.length > 0 ? alive : props.nodes.map(n => n.nodeKey)
})

async function generateConfig() {
  if (effectiveNodeKeys.value.length === 0) {
    error.value = '当前范围内没有可选的节点'
    yamlContent.value = ''
    return
  }
  loading.value = true
  error.value = ''
  try {
    const res = await api.exportClashConfig({
      node_keys: effectiveNodeKeys.value,
      group_name: groupName.value.trim() || 'PROXY',
    })
    yamlContent.value = res.yaml_content
  } catch (err: any) {
    error.value = err.message || '生成配置失败'
  } finally {
    loading.value = false
  }
}

watch(() => props.visible, (val) => {
  if (val) {
    if (props.selectedKeys.length > 0) {
      exportScope.value = 'selected'
    } else {
      exportScope.value = 'top10'
    }
    generateConfig()
  } else {
    yamlContent.value = ''
    copied.value = false
  }
})

watch([exportScope, groupName], () => {
  if (props.visible) {
    generateConfig()
  }
})

async function copyToClipboard() {
  if (!yamlContent.value) return
  try {
    await navigator.clipboard.writeText(yamlContent.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch {
    error.value = '复制到剪贴板失败，请手动全选复制'
  }
}

function downloadFile() {
  if (!yamlContent.value) return
  const blob = new Blob([yamlContent.value], { type: 'application/x-yaml;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `clash-nodes-${exportScope.value}.yaml`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

async function copySubscriptionUrl() {
  if (!subscriptionUrl.value) return
  try {
    await navigator.clipboard.writeText(subscriptionUrl.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch {
    error.value = '复制链接失败'
  }
}

const subscriptionUrl = computed(() => {
  if (typeof window === 'undefined') return ''
  const base = window.location.origin
  const keys = effectiveNodeKeys.value.join(',')
  const group = encodeURIComponent(groupName.value.trim() || 'PROXY')
  const token = api.getAuthToken()
  const tokenPart = token ? `&token=${encodeURIComponent(token)}` : ''
  return `${base}/api/workbench/export-clash?download=1&group=${group}&node_keys=${encodeURIComponent(keys)}${tokenPart}`
})
</script>

<template>
  <div v-if="visible" class="export-modal-overlay" @click.self="emit('close')">
    <div class="export-modal-card" role="dialog" aria-modal="true" aria-labelledby="export-title">
      <div class="export-modal-header">
        <div>
          <h2 id="export-title">📤 导出节点为 Clash 配置</h2>
          <p>将测速挑选后的优质节点导出为标准 Clash / Mihomo 配置文件或直接订阅。</p>
        </div>
        <button type="button" class="export-close-btn" @click="emit('close')">✕</button>
      </div>

      <div class="export-modal-body">
        <div class="export-config-bar">
          <label class="config-item">
            <span>导出范围</span>
            <UiSelect v-model="exportScope" aria-label="导出范围" :options="exportScopeOptions" />
          </label>

          <label class="config-item">
            <span>策略组名称</span>
            <input v-model="groupName" type="text" placeholder="PROXY">
          </label>

          <div class="config-item-action">
            <span class="node-count-badge">已包含 {{ effectiveNodeKeys.length }} 个节点</span>
          </div>
        </div>

        <div v-if="error" class="export-error" role="alert">{{ error }}</div>

        <div class="export-preview-box">
          <div class="export-preview-header">
            <span>配置预览 (Clash YAML)</span>
            <div class="preview-actions">
              <button type="button" class="action-btn" :disabled="!yamlContent || loading" @click="copyToClipboard">
                {{ copied ? '✓ 已复制' : '📋 复制内容' }}
              </button>
              <button type="button" class="action-btn primary" :disabled="!yamlContent || loading" @click="downloadFile">
                💾 下载 .yaml
              </button>
            </div>
          </div>
          <pre class="export-code-block"><code>{{ loading ? '正在生成配置…' : yamlContent || '暂无内容' }}</code></pre>
        </div>

        <div class="export-sub-link-section">
          <label><strong>本地客户端订阅地址</strong>（可直接复制填入 Clash Verge / Meta 客户端中）</label>
          <div class="sub-link-input-group">
            <input type="text" readonly :value="subscriptionUrl" @click="($event.target as HTMLInputElement).select()">
            <button type="button" class="action-btn" @click="copySubscriptionUrl">复制链接</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.export-modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(10, 15, 29, 0.8);
  backdrop-filter: blur(6px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9990;
  padding: 1rem;
}

.export-modal-card {
  width: 100%;
  max-width: 820px;
  max-height: 90vh;
  background: #182234;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 14px;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
  color: #f1f5f9;
  overflow: hidden;
}

.export-modal-header {
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.export-modal-header h2 {
  font-size: 1.25rem;
  font-weight: 700;
  margin: 0 0 0.25rem 0;
}

.export-modal-header p {
  font-size: 0.85rem;
  color: #94a3b8;
  margin: 0;
}

.export-close-btn {
  background: none;
  border: none;
  color: #94a3b8;
  font-size: 1.2rem;
  cursor: pointer;
  padding: 0.25rem;
  border-radius: 6px;
}

.export-close-btn:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.1);
}

.export-modal-body {
  padding: 1.25rem 1.5rem;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.export-config-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  align-items: flex-end;
  background: rgba(255, 255, 255, 0.03);
  padding: 0.85rem 1rem;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.06);
}

.config-item {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  font-size: 0.825rem;
  color: #cbd5e1;
}

.config-item select,
.config-item input {
  background: #0f172a;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 6px;
  padding: 0.45rem 0.75rem;
  color: #fff;
  font-size: 0.875rem;
}

.config-item-action {
  margin-left: auto;
  align-self: center;
}

.node-count-badge {
  background: rgba(56, 189, 248, 0.15);
  color: #38bdf8;
  border: 1px solid rgba(56, 189, 248, 0.3);
  padding: 0.35rem 0.75rem;
  border-radius: 20px;
  font-size: 0.825rem;
  font-weight: 600;
}

.export-error {
  color: #f87171;
  font-size: 0.875rem;
  background: rgba(239, 68, 68, 0.1);
  padding: 0.75rem 1rem;
  border-radius: 8px;
}

.export-preview-box {
  display: flex;
  flex-direction: column;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  overflow: hidden;
  background: #0b1120;
}

.export-preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.6rem 1rem;
  background: rgba(255, 255, 255, 0.04);
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  font-size: 0.825rem;
  color: #94a3b8;
}

.preview-actions {
  display: flex;
  gap: 0.5rem;
}

.action-btn {
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.12);
  color: #f1f5f9;
  border-radius: 6px;
  padding: 0.35rem 0.75rem;
  font-size: 0.825rem;
  cursor: pointer;
  transition: all 0.2s;
}

.action-btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.16);
}

.action-btn.primary {
  background: #0284c7;
  border-color: #38bdf8;
  color: #fff;
  font-weight: 600;
}

.action-btn.primary:hover:not(:disabled) {
  background: #0369a1;
}

.action-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.export-code-block {
  margin: 0;
  padding: 1rem;
  max-height: 280px;
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.82rem;
  line-height: 1.5;
  color: #e2e8f0;
}

.export-sub-link-section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  font-size: 0.85rem;
  color: #cbd5e1;
}

.sub-link-input-group {
  display: flex;
  gap: 0.5rem;
}

.sub-link-input-group input {
  flex: 1;
  background: #0f172a;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 6px;
  padding: 0.5rem 0.75rem;
  color: #38bdf8;
  font-family: monospace;
  font-size: 0.825rem;
}
</style>
