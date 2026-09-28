<script setup lang="ts">
import { ref } from 'vue'
import { createMonitorJob, fetchMonitorJobs, fetchMonitorNodeOptions } from '../../api/monitor'
import { openDataFolder } from '../../api/bridge'
import type { MonitorJobCreateRequest } from '../../types'
const emit = defineEmits<{ (event: 'imported'): void }>()
const busy = ref(false)
const message = ref('')
const preview = ref<MonitorJobCreateRequest[]>([])
async function openFolder() { try { await openDataFolder('monitor'); message.value = '已请求打开监测数据文件夹。数据库运行中请勿直接编辑。' } catch (e) { message.value = String(e) } }
async function exportConfig() {
  busy.value = true
  try {
    const jobs = await fetchMonitorJobs()
    const config = jobs.map(job => ({ name: job.name, profile_id: job.profileId, node_keys: job.nodes.map(node => node.nodeKey), node_contexts: job.nodes.map(node => ({ node_key: node.nodeKey, node_identity_key: node.nodeIdentityKey, config_revision_key: node.configRevisionKey })), probe_set: job.probeSet, sampling_tier: job.samplingTier, interval_seconds: job.intervalSeconds, timeout_seconds: job.timeoutSeconds }))
    const url = URL.createObjectURL(new Blob([JSON.stringify({ format: 'speedtest-monitor', version: 1, jobs: config }, null, 2)], { type: 'application/json' }))
    const link = document.createElement('a'); link.href = url; link.download = 'monitor-config.json'; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
    message.value = `已导出 ${jobs.length} 项配置，不含凭据、历史样本和运行状态。`
  } catch (e) { message.value = String(e) } finally { busy.value = false }
}
async function inspectFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]; input.value = ''
  if (!file) return
  preview.value = []; busy.value = true; message.value = ''
  try {
    if (file.size > 1024 * 1024) throw new Error('配置文件不能超过 1 MiB')
    const value = JSON.parse(await file.text())
    if (value.format !== 'speedtest-monitor' || value.version !== 1 || !Array.isArray(value.jobs) || value.jobs.length < 1 || value.jobs.length > 100) throw new Error('请选择有效的 Monitor 配置文件（1–100 项任务）')
    const options = await fetchMonitorNodeOptions()
    const requests: MonitorJobCreateRequest[] = []
    for (const job of value.jobs) {
      if (!job || typeof job.name !== 'string' || job.name.length > 200 || typeof job.profile_id !== 'string' || !['light', 'service', 'heavy'].includes(job.probe_set) || !['regular', 'focus', 'sparse'].includes(job.sampling_tier) || !Number.isInteger(job.interval_seconds) || job.interval_seconds < 1 || !Number.isInteger(job.timeout_seconds) || job.timeout_seconds < 1 || !Array.isArray(job.node_keys) || !job.node_keys.length || job.node_keys.length > 500 || !Array.isArray(job.node_contexts)) throw new Error('任务配置格式或检测参数无效')
      const nodes = job.node_keys.map((key: string) => {
        const node = options.find(node => node.profileId === job.profile_id && node.nodeKey === key)
        const context = job.node_contexts.find((context: { node_key: string }) => context?.node_key === key)
        if (!node || !context || node.nodeIdentityKey !== context.node_identity_key || node.configRevisionKey !== context.config_revision_key) throw new Error(`“${job.name}”的订阅或节点配置不匹配，请先导入对应订阅或重新导出配置`)
        return { node_key: key, node_identity_key: node.nodeIdentityKey, config_revision_key: node.configRevisionKey }
      })
      requests.push({ name: job.name, profile_id: job.profile_id, node_keys: [...new Set<string>(job.node_keys)], node_contexts: nodes, probe_set: job.probe_set, sampling_tier: job.sampling_tier, interval_seconds: job.interval_seconds, timeout_seconds: job.timeout_seconds })
    }
    preview.value = requests
  } catch (e) { message.value = e instanceof Error ? e.message : String(e) } finally { busy.value = false }
}
async function confirmImport() {
  if (busy.value) return
  busy.value = true; let count = 0
  try {
    while (preview.value.length) { await createMonitorJob(preview.value[0]); preview.value.shift(); count++ }
    message.value = `已新增 ${count} 项任务，均为停止状态，未覆盖原任务。`
  } catch (e) { message.value = `已新增 ${count} 项，剩余未导入：${String(e)}` }
  finally { busy.value = false; if (count) emit('imported') }
}
</script>
<template>
  <section class="monitor-transfer">
    <div class="transfer-actions"><strong>监测文件</strong><button :disabled="busy" @click="openFolder">打开数据文件夹</button><button :disabled="busy" @click="exportConfig">导出配置</button><label class="file-button">导入配置<input type="file" accept=".json,application/json" :disabled="busy" @change="inspectFile" /></label></div>
    <p>配置只包含任务与节点身份，不含订阅密钥。导入先预览，确认后新增为停止状态。</p>
    <div v-if="preview.length" class="import-preview"><strong>待导入 {{ preview.length }} 项</strong><ul><li v-for="(job, i) in preview" :key="i">{{ job.name || '未命名任务' }} · {{ job.node_keys.length }} 节点 · {{ job.probe_set }} · 每 {{ job.interval_seconds }} 秒</li></ul><button :disabled="busy" @click="confirmImport">确认新增任务（不启动）</button><button :disabled="busy" @click="preview = []">取消</button></div>
    <p v-if="message" role="status">{{ message }}</p>
  </section>
</template>
<style scoped>
.monitor-transfer { padding: 16px 20px; background: var(--card); border: 1px solid var(--border); border-radius: 10px; font-size: 12px; }
.transfer-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; } strong { margin-right: auto; } button, .file-button { padding: 7px 10px; background: var(--card); border: 1px solid var(--border); border-radius: 5px; color: var(--primary); cursor: pointer; }
.file-button { position: relative; overflow: hidden; } input { position: absolute; inset: 0; opacity: 0; width: 100%; cursor: pointer; } p { color: var(--text-secondary); margin-top: 8px; } .import-preview { margin-top: 12px; padding: 12px; background: var(--primary-subtle); } ul { max-height: 160px; overflow: auto; margin: 8px 0; } button:disabled { opacity: .5; }
</style>
