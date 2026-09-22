<script setup lang="ts">
import { computed, ref } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'
import type { ProfileSource } from '../../types'

const store = useWorkbenchStore()
const selectedSource = ref<ProfileSource | null>(null)
const explicitPath = ref('')
const isBusy = ref(false)
const errorMessage = ref('')

const setup = computed(() => store.profileSetup)

async function migrateLegacyData() {
  const migration = setup.value?.migration
  if (!migration || migration.state !== 'pending') return
  if (!window.confirm(`确认将旧数据迁移到 canonical 数据根？\n\n来源：${migration.source_history_dir || '无 SQLite 来源'}\n目标：${migration.target_history_dir}\nSQLite：${migration.source_has_sqlite ? '会做一致备份' : '建立空库'}\nlegacy JSON：${migration.source_json_count} 份\n设置：${migration.source_has_settings ? '保留原内容' : '无'}\n\n旧来源会保留，不会刷新订阅。`)) {
    return
  }
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
  if (!window.confirm(`确认导入该本地来源？\n\n${source.path}\n配置 ${source.profile_count} 个，缓存 ${source.cache_count} 份。\n不会刷新网络订阅。`)) {
    return
  }
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
  if (!window.confirm('确认从空库开始？这不会导入当前目录或可执行文件目录中的数据。')) {
    return
  }
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
  if (!window.confirm('确认清理未完成的导入临时目录？canonical 数据不会被删除。')) {
    return
  }
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
</script>

<template>
  <div
    v-if="store.isProfileSetupOpen"
    class="fixed inset-0 bg-black/60 backdrop-blur-sm z-[60] flex items-center justify-center p-4"
  >
    <div class="bg-card border border-border rounded-xl shadow-2xl w-full max-w-3xl max-h-[88vh] flex flex-col overflow-hidden text-xs">
      <div class="p-4 border-b border-border flex items-center justify-between">
        <div>
          <h2 class="text-sm font-bold text-content-main">数据根与首次初始化</h2>
          <p class="text-content-muted mt-1">只显示本地路径和数量；选择后才会导入。</p>
        </div>
        <button @click="closeModal" class="text-content-muted hover:text-content-main text-lg font-mono" aria-label="关闭">✕</button>
      </div>

      <div class="p-4 flex-1 overflow-y-auto flex flex-col gap-4">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-2 text-[11px]">
          <div class="bg-card-subtle border border-border rounded-lg p-3">
            <div class="text-content-muted">当前数据根</div>
            <div class="font-mono text-content-main break-all mt-1">{{ setup?.data_root }}</div>
          </div>
          <div class="bg-card-subtle border border-border rounded-lg p-3">
            <div class="text-content-muted">Profile / History</div>
            <div class="font-mono text-content-main break-all mt-1">{{ setup?.profile_dir }}</div>
            <div class="font-mono text-content-secondary break-all">{{ setup?.history_dir }}</div>
          </div>
        </div>

        <div v-if="setup?.state === 'error'" class="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-red-300">
          {{ setup.error }}
        </div>

        <div v-if="setup?.migration?.state === 'pending'" class="rounded-lg border border-blue-500/30 bg-blue-500/10 p-3 text-blue-100">
          <div class="font-semibold">发现旧 History / Settings 数据</div>
          <div class="mt-1 text-blue-200/80">旧数据不会自动迁移。确认后会先做 SQLite 一致备份，再切换到 canonical 数据根；旧来源保留。</div>
          <div class="mt-2 font-mono text-[11px] break-all">{{ setup.migration.source_history_dir }}</div>
          <div class="font-mono text-[11px] break-all">→ {{ setup.migration.target_history_dir }}</div>
          <button @click="migrateLegacyData" :disabled="isBusy" class="mt-3 px-3 py-1.5 rounded bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50">确认迁移旧数据</button>
        </div>

        <div v-if="setup?.migration?.state === 'conflict'" class="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-amber-200">
          <div class="font-semibold">canonical 与旧数据同时存在</div>
          <div class="mt-1">不会自动覆盖、合并或按时间选择。{{ setup.migration.error }}</div>
        </div>

        <div v-if="setup?.migration?.state === 'invalid'" class="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-red-300">
          <div class="font-semibold">数据迁移已安全停止</div>
          <div class="mt-1">检测到无法识别或不完整的数据库，未进行覆盖或部分迁移。{{ setup.migration.error }}</div>
        </div>

        <div v-if="setup?.unfinished_staging?.length" class="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-amber-200">
          检测到未完成导入：{{ setup.unfinished_staging.join('、') }}。请清理后再重试；不会自动接管临时目录。
          <button @click="discardStaging" :disabled="isBusy" class="mt-2 px-3 py-1 rounded border border-amber-400/40 hover:bg-amber-400/10 disabled:opacity-50">清理未完成导入</button>
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
                selectedSource?.path === source.path ? 'border-brand bg-brand/10' : 'border-border bg-card-subtle',
                !source.available ? 'opacity-55 cursor-not-allowed' : 'hover:border-brand'
              ]"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="font-semibold text-content-main">{{ source.label }}</span>
                <span v-if="source.possible_test_data" class="text-amber-300">可能是测试数据</span>
              </div>
              <div class="font-mono text-content-secondary break-all mt-1">{{ source.path }}</div>
              <div v-if="source.available" class="text-content-muted mt-1">配置 {{ source.profile_count }} 个 · 可用缓存 {{ source.cache_count }} 份</div>
              <div v-else class="text-red-300 mt-1">{{ source.error || (source.missing || []).join('、') || '缺少可导入配置' }}</div>
            </button>
          </div>

          <div class="border-t border-border pt-3 flex flex-col gap-2">
            <div class="font-semibold text-content-main">或检查用户明确指定的目录</div>
            <div class="flex gap-2">
              <input v-model="explicitPath" placeholder="绝对路径，例如 D:\\old-profile" class="flex-1 bg-card text-content-main border border-border rounded px-3 py-2 font-mono focus:outline-none focus:border-brand" />
              <button @click="inspectExplicitSource" :disabled="isBusy" class="px-3 py-2 rounded border border-border hover:border-brand disabled:opacity-50">检查</button>
            </div>
          </div>

          <div v-if="selectedSource" class="rounded-lg border border-brand/40 bg-brand/5 p-3">
            已选择：<span class="font-mono break-all">{{ selectedSource.path }}</span>
            <div class="text-content-muted mt-1">配置 {{ selectedSource.profile_count }} 个 · 可用缓存 {{ selectedSource.cache_count }} 份<span v-if="selectedSource.missing?.length"> · {{ selectedSource.missing.join('、') }}</span></div>
          </div>

          <div class="flex flex-wrap justify-end gap-2">
            <button @click="initializeEmpty" :disabled="isBusy" class="px-3 py-2 rounded border border-border hover:bg-card-subtle disabled:opacity-50">从空库开始</button>
            <button @click="importSelectedSource" :disabled="isBusy || !selectedSource?.available" class="px-4 py-2 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium disabled:opacity-50">确认导入所选来源</button>
          </div>
        </template>

        <div v-if="setup?.state === 'ready'" class="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-emerald-200">
          canonical profile 已初始化。Profile ID 与缓存文件保持原样；后续启动只读取这份数据，不会按 cwd 重新迁移。
        </div>

        <div v-if="errorMessage" class="text-red-300 whitespace-pre-wrap">{{ errorMessage }}</div>
      </div>

      <div class="px-4 py-3 border-t border-border flex justify-end">
        <button @click="closeModal" class="px-3 py-1.5 rounded border border-border hover:bg-card-subtle">{{ setup?.state === 'ready' ? '关闭' : '稍后决定' }}</button>
      </div>
    </div>
  </div>
</template>
