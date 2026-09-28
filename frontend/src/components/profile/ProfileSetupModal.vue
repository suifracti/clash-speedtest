<script setup lang="ts">
import { computed, ref } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'
import type { ProfileSource } from '../../types'
import MonitorTransfer from '../monitor/MonitorTransfer.vue'

const store = useWorkbenchStore()
const selectedSource = ref<ProfileSource | null>(null)
const explicitPath = ref('')
const isBusy = ref(false)
const errorMessage = ref('')
type ConfirmationKind = 'migrate' | 'import' | 'empty' | 'discard'
const confirmation = ref<{ kind: ConfirmationKind; title: string; detail: string; action: string } | null>(null)
async function openFolder() { try { await api.openDataFolder('data') } catch (e) { errorMessage.value = String(e) } }

const setup = computed(() => store.profileSetup)

async function migrateLegacyData() {
  const migration = setup.value?.migration
  if (!migration || migration.state !== 'pending') return
  errorMessage.value = ''
  isBusy.value = true
  try {
    await api.migrateLegacyData()
    await store.loadProfileSetup()
    await store.loadAirports()
    closeModal()
  } catch (e: any) {
    errorMessage.value = e.message || '迁移失败，旧数据仍保持原 authority'
    await store.loadProfileSetup()
  } finally {
    isBusy.value = false
  }
}

function closeModal() {
  store.isProfileSetupOpen = false
  errorMessage.value = ''
}

function chooseSource(source: ProfileSource) {
  if (!source.available) return
  selectedSource.value = source
  errorMessage.value = ''
}

async function inspectExplicitSource() {
  errorMessage.value = ''
  if (!explicitPath.value.trim()) {
    errorMessage.value = '请输入绝对路径'
    return
  }
  isBusy.value = true
  try {
    selectedSource.value = await api.inspectProfileSource(explicitPath.value.trim())
  } catch (e: any) {
    selectedSource.value = null
    errorMessage.value = e.message || '无法检查该来源'
  } finally {
    isBusy.value = false
  }
}

async function importSelectedSource() {
  if (!selectedSource.value) {
    errorMessage.value = '请先选择并检查一个来源'
    return
  }
  const source = selectedSource.value
  if (!source.available) return
  errorMessage.value = ''
  isBusy.value = true
  try {
    await api.importProfileSource(source.path)
    await store.loadProfileSetup()
    await store.loadAirports()
    selectedSource.value = null
    closeModal()
  } catch (e: any) {
    errorMessage.value = e.message || '导入失败，canonical 数据未切换'
    await store.loadProfileSetup()
  } finally {
    isBusy.value = false
  }
}

async function initializeEmpty() {
  errorMessage.value = ''
  isBusy.value = true
  try {
    await api.initializeEmptyProfileStore()
    await store.loadProfileSetup()
    await store.loadAirports()
    closeModal()
  } catch (e: any) {
    errorMessage.value = e.message || '创建空库失败，未覆盖既有数据'
  } finally {
    isBusy.value = false
  }
}

async function discardStaging() {
  errorMessage.value = ''
  isBusy.value = true
  try {
    await api.discardProfileImport()
    await store.loadProfileSetup()
  } catch (e: any) {
    errorMessage.value = e.message || '清理失败，可能仍有其它实例正在导入'
  } finally {
    isBusy.value = false
  }
}

function askConfirmation(kind: ConfirmationKind): void {
  if (isBusy.value) return
  const migration = setup.value?.migration
  const source = selectedSource.value
  if (kind === 'migrate' && migration?.state === 'pending') confirmation.value = {
    kind, title: '迁移旧数据？', action: '确认迁移',
    detail: `来源：${migration.source_history_dir || '无 SQLite 来源'}\n目标：${migration.target_history_dir}\nSQLite：${migration.source_has_sqlite ? '先做一致备份' : '建立空库'}；旧来源会保留，不会刷新订阅。`,
  }
  if (kind === 'import' && source?.available) confirmation.value = {
    kind, title: '导入这个本地来源？', action: '确认导入',
    detail: `${source.path}\n配置 ${source.profile_count} 个，缓存 ${source.cache_count} 份。不会刷新网络订阅。`,
  }
  if (kind === 'empty') confirmation.value = {
    kind, title: '从空库开始？', action: '确认建立空库',
    detail: '不会导入当前目录或可执行文件目录中的旧数据。请确认你不需要迁移它们。',
  }
  if (kind === 'discard' && setup.value?.unfinished_staging?.length) confirmation.value = {
    kind, title: '清理未完成的导入？', action: '确认清理',
    detail: `仅清理未完成的临时目录：${setup.value.unfinished_staging.join('、')}。当前数据根不会被删除。`,
  }
}

async function confirmAction(): Promise<void> {
  const kind = confirmation.value?.kind
  confirmation.value = null
  if (kind === 'migrate') await migrateLegacyData()
  if (kind === 'import') await importSelectedSource()
  if (kind === 'empty') await initializeEmpty()
  if (kind === 'discard') await discardStaging()
}
</script>

<template>
  <div
    v-if="store.isProfileSetupOpen"
    class="fixed inset-0 bg-black/40 backdrop-blur-sm z-[60] flex items-center justify-center p-4 select-none"
  >
    <div class="prototype-modal w-full max-w-3xl max-h-[88vh] flex flex-col overflow-hidden text-xs">
      <div class="prototype-modal-header">
        <div>
          <h2 class="prototype-modal-title">
            <svg class="w-4 h-4 text-primary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M4 7v10c0 2 1.5 3 3.5 3h9c2 0 3.5-1 3.5-3V7c0-2-1.5-3-3.5-3h-9C5.5 4 4 5 4 7z" />
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12h6m-6 4h4" />
            </svg>
            数据与备份
          </h2>
          <p class="text-content-muted mt-1 text-[11px]">文件位置、备份导出与监测配置迁移。</p>
        </div>
        <button @click="closeModal" class="prototype-close-btn font-mono" aria-label="关闭">✕</button>
      </div>

      <div class="prototype-modal-body flex-1 overflow-y-auto flex flex-col gap-4">
        <section class="data-actions"><div><strong>本机数据</strong><p>历史与配置保存在本机，打开文件夹不会修改数据。</p></div><button class="tool-button" @click="openFolder">打开数据文件夹 ↗</button></section>
        <MonitorTransfer />
        <div class="grid grid-cols-1 md:grid-cols-2 gap-2 text-[11px]">
          <div class="bg-card-subtle border border-border rounded-lg p-3 flex flex-col justify-between">
            <div>
              <div class="text-content-muted">当前数据根</div>
              <div class="font-mono text-content-main break-all mt-1">{{ setup?.data_root }}</div>
            </div>
            <div class="mt-2.5 pt-2 border-t border-border flex items-center justify-between">
              <span class="text-content-muted text-[10px]">跨平台归档备份</span>
              <a
                :href="api.exportDataRootURL()"
                download
                class="tool-button flex items-center gap-1.5 text-content-main hover:text-primary transition-colors text-[11px]"
                title="导出当前数据根（profiles, history, settings）为跨平台标准的 .zip 压缩包"
              >
                <svg class="w-3.5 h-3.5 text-primary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                </svg>
                导出数据根 (ZIP)
              </a>
            </div>
          </div>
          <div class="bg-card-subtle border border-border rounded-lg p-3">
            <div class="text-content-muted">Profile / History</div>
            <div class="font-mono text-content-main break-all mt-1">{{ setup?.profile_dir }}</div>
            <div class="font-mono text-content-secondary break-all">{{ setup?.history_dir }}</div>
          </div>
        </div>

        <div v-if="setup?.state === 'error'" class="notice-box notice-box--danger">
          {{ setup.error }}
        </div>

        <div v-if="setup?.migration?.state === 'pending'" class="notice-box notice-box--info">
          <div class="font-semibold text-content-main">发现旧 History / Settings 数据</div>
          <div class="mt-1 text-content-secondary">旧数据不会自动迁移。确认后会先做 SQLite 一致备份，再切换到 canonical 数据根；旧来源保留。</div>
          <div class="mt-2 font-mono text-[11px] break-all text-content-secondary">{{ setup.migration.source_history_dir }}</div>
          <div class="font-mono text-[11px] break-all text-content-secondary">→ {{ setup.migration.target_history_dir }}</div>
          <button @click="askConfirmation('migrate')" :disabled="isBusy" class="mt-3 prototype-btn-primary">迁移旧数据…</button>
        </div>

        <div v-if="setup?.migration?.state === 'conflict'" class="notice-box notice-box--warning">
          <div class="font-semibold">canonical 与旧数据同时存在</div>
          <div class="mt-1">不会自动覆盖、合并或按时间选择。{{ setup.migration.error }}</div>
        </div>

        <div v-if="setup?.migration?.state === 'invalid'" class="notice-box notice-box--danger">
          <div class="font-semibold">数据迁移已安全停止</div>
          <div class="mt-1">检测到无法识别或不完整的数据库，未进行覆盖或部分迁移。{{ setup.migration.error }}</div>
        </div>

        <div v-if="setup?.unfinished_staging?.length" class="notice-box notice-box--warning">
          检测到未完成导入：{{ setup.unfinished_staging.join('、') }}。请清理后再重试；不会自动接管临时目录。
          <button @click="askConfirmation('discard')" :disabled="isBusy" class="mt-2 tool-button">清理未完成导入…</button>
        </div>

        <template v-if="setup?.state !== 'error' && setup?.state !== 'ready'">
          <div class="flex flex-col gap-2">
            <div class="font-semibold text-content-main">选择旧数据来源</div>
            <button
              v-for="source in setup?.sources || []"
              :key="source.path"
              type="button"
              @click="chooseSource(source)"
              :disabled="!source.available || isBusy"
              :class="[
                'text-left rounded-lg border p-3 transition-colors',
                selectedSource?.path === source.path ? 'border-primary bg-primary-subtle' : 'border-border bg-card-subtle',
                !source.available ? 'opacity-55 cursor-not-allowed' : 'hover:border-primary'
              ]"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="font-semibold text-content-main">{{ source.label }}</span>
                <span v-if="source.possible_test_data" class="badge badge--warning">可能是测试数据</span>
              </div>
              <div class="font-mono text-content-secondary break-all mt-1">{{ source.path }}</div>
              <div v-if="source.available" class="text-content-muted mt-1">配置 {{ source.profile_count }} 个 · 可用缓存 {{ source.cache_count }} 份</div>
              <div v-else class="text-red-500 mt-1">{{ source.error || (source.missing || []).join('、') || '缺少可导入配置' }}</div>
            </button>
          </div>

          <div class="border-t border-border pt-3 flex flex-col gap-2">
            <div class="font-semibold text-content-main">或检查用户明确指定的目录</div>
            <div class="flex gap-2">
              <input v-model="explicitPath" placeholder="绝对路径，例如 D:\\old-profile" class="prototype-input flex-1 font-mono" />
              <button @click="inspectExplicitSource" :disabled="isBusy" class="tool-button">检查</button>
            </div>
          </div>

          <div v-if="selectedSource" class="notice-box notice-box--info">
            已选择：<span class="font-mono break-all font-semibold">{{ selectedSource.path }}</span>
            <div class="text-content-muted mt-1">配置 {{ selectedSource.profile_count }} 个 · 可用缓存 {{ selectedSource.cache_count }} 份<span v-if="selectedSource.missing?.length"> · {{ selectedSource.missing.join('、') }}</span></div>
          </div>

          <div class="flex flex-wrap justify-end gap-2">
            <button @click="askConfirmation('empty')" :disabled="isBusy" class="tool-button">从空库开始…</button>
            <button @click="askConfirmation('import')" :disabled="isBusy || !selectedSource?.available" class="prototype-btn-primary">导入所选来源…</button>
          </div>
        </template>

        <div v-if="setup?.state === 'ready'" class="notice-box notice-box--success">
          当前数据目录已就绪。全量 ZIP 为备份归档（可能包含订阅密钥），请妥善保管；监测配置 JSON 可在上方预览导入。这里不直接覆盖恢复 ZIP。
        </div>

        <div v-if="errorMessage" class="text-red-500 text-[11px] whitespace-pre-wrap">{{ errorMessage }}</div>
      </div>

      <div class="px-4 py-3 border-t border-border flex items-center justify-between">
        <a
          :href="api.exportDataRootURL()"
          download
          class="tool-button flex items-center gap-1.5 text-content-main hover:text-primary transition-colors text-[11px]"
          title="导出当前数据根为跨平台标准的 .zip 压缩包"
        >
          <svg class="w-3.5 h-3.5 text-primary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
          </svg>
          导出数据根归档 (跨平台 ZIP)
        </a>
        <button @click="closeModal" class="tool-button">{{ setup?.state === 'ready' ? '关闭' : '稍后决定' }}</button>
      </div>
    </div>
    <div v-if="confirmation" class="confirm-backdrop" role="presentation">
      <section class="confirm-dialog" role="alertdialog" aria-modal="true" :aria-label="confirmation.title">
        <span class="confirm-kicker">请核对本机数据</span>
        <h3>{{ confirmation.title }}</h3>
        <p>{{ confirmation.detail }}</p>
        <div class="confirm-actions"><button type="button" class="tool-button" @click="confirmation = null">取消</button><button type="button" class="prototype-btn-primary" @click="confirmAction">{{ confirmation.action }}</button></div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.prototype-modal { border-radius: 16px; }
.prototype-modal-header { background: var(--primary-subtle); padding: 22px 24px; }
.prototype-modal-body { padding: 22px 24px; }
.data-actions { display: flex; justify-content: space-between; gap: 15px; align-items: center; padding: 16px; border-left: 3px solid #5276aa; background: var(--card-subtle); }
.data-actions strong { font-size: 14px; } .data-actions p { color: var(--text-secondary); margin-top: 5px; font-size: 11px; }
.confirm-backdrop { position: fixed; inset: 0; z-index: 80; display: grid; place-items: center; padding: 18px; background: rgba(14, 25, 35, .55); }
.confirm-dialog { width: min(100%, 540px); padding: 24px; border: 1px solid var(--border); border-radius: 14px; background: var(--card-bg); box-shadow: 0 24px 70px rgba(0, 0, 0, .23); }
.confirm-kicker { color: var(--primary); font-size: 11px; font-weight: 750; letter-spacing: .08em; }
.confirm-dialog h3 { margin: 9px 0 12px; color: var(--text-main); font-size: 21px; }
.confirm-dialog p { color: var(--text-secondary); line-height: 1.7; white-space: pre-wrap; overflow-wrap: anywhere; }
.confirm-actions { display: flex; justify-content: flex-end; gap: 9px; margin-top: 22px; }
</style>
