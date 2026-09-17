<script setup lang="ts">
import { computed } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'

const store = useWorkbenchStore()

const isRunning = computed(() => store.testStatus.is_running)
const percent = computed(() => store.testStatus.percent)
const currentIdx = computed(() => store.testStatus.current_index)
const totalNodes = computed(() => store.testStatus.total_nodes)
const currentNode = computed(() => store.testStatus.current_node || '等待调度')
</script>

<template>
  <div class="bg-card-subtle border-b border-border px-6 py-2.5 flex items-center justify-between gap-6 text-xs select-none">
    <!-- Left: Running Indicator & Current Target -->
    <div class="flex items-center gap-3 min-w-0">
      <div class="flex items-center gap-1.5">
        <span
          class="w-2 h-2 rounded-full"
          :class="isRunning ? 'bg-blue-500 animate-ping' : 'bg-emerald-500'"
        ></span>
        <span class="font-medium text-content-main">
          {{ isRunning ? '测试进行中' : totalNodes > 0 ? '测试就绪' : '等待开始' }}
        </span>
      </div>

      <div class="h-3.5 w-px bg-border"></div>

      <div class="flex items-center gap-1.5 text-content-muted truncate">
        <span>当前节点:</span>
        <span class="font-mono text-content-main font-medium truncate max-w-xs" :title="currentNode">
          {{ currentNode }}
        </span>
      </div>
    </div>

    <!-- Right: Progress Meter Bar & Fraction -->
    <div class="flex items-center gap-4 flex-shrink-0">
      <div class="flex items-center gap-2">
        <div class="w-48 h-2 rounded-full bg-border overflow-hidden">
          <div
            class="h-full bg-blue-500 transition-all duration-300 rounded-full"
            :style="{ width: `${percent}%` }"
          ></div>
        </div>
        <span class="font-mono text-content-secondary font-medium w-10 text-right">
          {{ percent }}%
        </span>
      </div>

      <div class="h-3.5 w-px bg-border"></div>

      <div class="font-mono text-content-secondary">
        <span class="text-content-main font-bold">{{ currentIdx }}</span>
        <span class="text-content-muted"> / </span>
        <span>{{ totalNodes }}</span>
      </div>
    </div>
  </div>
</template>
