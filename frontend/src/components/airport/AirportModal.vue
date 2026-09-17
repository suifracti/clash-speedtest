<script setup lang="ts">
import { ref } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import * as api from '../../api/bridge'
import type { Airport } from '../../types'

const store = useWorkbenchStore()

const newName = ref('')
const newUrl = ref('')
const isSubmitting = ref(false)
const errorMessage = ref('')
const editingAirport = ref<Airport | null>(null)

function closeModal() {
  store.isAirportModalOpen = false
  editingAirport.value = null
  newName.value = ''
  newUrl.value = ''
  errorMessage.value = ''
}

async function handleSave() {
  errorMessage.value = ''
  if (!newName.value.trim() || !newUrl.value.trim()) {
    errorMessage.value = '机场名称和订阅链接不能为空'
    return
  }

  isSubmitting.value = true
  try {
    if (editingAirport.value) {
      await api.updateAirport(editingAirport.value.id, newName.value.trim(), newUrl.value.trim())
    } else {
      await api.createAirport(newName.value.trim(), newUrl.value.trim())
    }
    await store.loadAirports()
    newName.value = ''
    newUrl.value = ''
    editingAirport.value = null
  } catch (e: any) {
    errorMessage.value = e.message || '保存机场失败'
  } finally {
    isSubmitting.value = false
  }
}

function startEdit(ap: Airport) {
  editingAirport.value = ap
  newName.value = ap.name
  newUrl.value = ap.url
}

async function handleDelete(id: string) {
  if (confirm('确定删除该机场订阅源吗？')) {
    try {
      await api.deleteAirport(id)
      await store.loadAirports()
    } catch (e: any) {
      alert(e.message || '删除失败')
    }
  }
}

async function handleRefresh(id: string) {
  try {
    await api.refreshAirport(id)
    await store.loadAirports()
  } catch (e: any) {
    alert(e.message || '刷新失败')
  }
}
</script>

<template>
  <div
    v-if="store.isAirportModalOpen"
    class="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none"
  >
    <div class="bg-card border border-border rounded-xl shadow-2xl w-full max-w-2xl max-h-[85vh] flex flex-col overflow-hidden text-xs">
      <!-- Modal Header -->
      <div class="p-4 border-b border-border flex items-center justify-between">
        <h2 class="text-sm font-bold text-content-main flex items-center gap-2">
          <span>✈️</span> 机场订阅源管理
        </h2>
        <button @click="closeModal" class="text-content-muted hover:text-content-main text-lg font-mono">
          ✕
        </button>
      </div>

      <!-- Modal Body -->
      <div class="p-4 flex-1 overflow-y-auto flex flex-col gap-5">
        <!-- Input Form -->
        <div class="bg-card-subtle p-3.5 rounded-lg border border-border flex flex-col gap-3">
          <span class="font-semibold text-content-main">
            {{ editingAirport ? '编辑机场订阅' : '添加新机场订阅源' }}
          </span>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-2">
            <input
              v-model="newName"
              placeholder="机场名称 (如: 专线01)"
              class="bg-card text-content-main border border-border rounded px-3 py-1.5 focus:outline-none focus:border-brand"
            />
            <input
              v-model="newUrl"
              placeholder="订阅链接 (HTTP URL 或本地路径)"
              class="md:col-span-2 bg-card text-content-main border border-border rounded px-3 py-1.5 focus:outline-none focus:border-brand font-mono"
            />
          </div>

          <div v-if="errorMessage" class="text-red-400 text-[11px]">
            {{ errorMessage }}
          </div>

          <div class="flex justify-end gap-2">
            <button
              v-if="editingAirport"
              @click="editingAirport = null; newName = ''; newUrl = ''"
              class="px-3 py-1 rounded border border-border hover:bg-card text-content-secondary"
            >
              取消编辑
            </button>
            <button
              @click="handleSave"
              :disabled="isSubmitting"
              class="px-4 py-1 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium disabled:opacity-50"
            >
              {{ isSubmitting ? '保存中...' : editingAirport ? '更新' : '添加' }}
            </button>
          </div>
        </div>

        <!-- Airport List Table -->
        <div class="flex flex-col gap-2">
          <span class="font-semibold text-content-secondary text-[11px] uppercase tracking-wider">
            已有机场列表 ({{ store.airports.length }})
          </span>

          <div class="border border-border rounded-lg overflow-hidden divide-y divide-border">
            <div
              v-for="ap in store.airports"
              :key="ap.id"
              class="p-3 bg-card hover:bg-card-hover flex items-center justify-between gap-4 transition-colors"
            >
              <div class="flex flex-col gap-0.5 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="font-bold text-content-main">{{ ap.name }}</span>
                  <span class="px-1.5 py-0.2 rounded text-[10px] bg-card-subtle text-content-muted border border-border">
                    {{ ap.node_count }} 节点
                  </span>
                </div>
                <span class="text-content-muted font-mono text-[11px] truncate max-w-md" :title="ap.url">
                  {{ ap.url }}
                </span>
              </div>

              <!-- Actions -->
              <div class="flex items-center gap-1.5 flex-shrink-0">
                <button
                  @click="handleRefresh(ap.id)"
                  class="px-2 py-1 rounded border border-border hover:border-brand hover:text-brand bg-card-subtle"
                  title="刷新拉取最新节点"
                >
                  刷新
                </button>
                <button
                  @click="startEdit(ap)"
                  class="px-2 py-1 rounded border border-border hover:border-content-main bg-card-subtle"
                >
                  编辑
                </button>
                <button
                  @click="handleDelete(ap.id)"
                  class="px-2 py-1 rounded border border-red-500/30 text-red-400 hover:bg-red-500/10"
                >
                  删除
                </button>
              </div>
            </div>

            <div v-if="store.airports.length === 0" class="p-8 text-center text-content-muted">
              暂无机场配置，请在上方输入链接添加
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
