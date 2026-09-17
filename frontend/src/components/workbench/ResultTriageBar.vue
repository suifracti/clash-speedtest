<script setup lang="ts">
import { computed } from 'vue'
import { useWorkbenchStore } from '../../stores/workbench'
import type { TriageCategory } from '../../types'

const store = useWorkbenchStore()

const counts = computed(() => store.triageCounts)
const totalTested = computed(() => Object.keys(store.resultsMap).length)

function setFilter(filter: TriageCategory | 'all') {
  store.activeTriageFilter = filter
}

async function retestUnstable() {
  await store.retestCategory('unverified')
  await store.retestCategory('flapping')
}

async function retestFailed() {
  await store.retestCategory('failed')
}
</script>

<template>
  <div class="bg-card border-b border-border px-6 py-2.5 flex flex-wrap items-center justify-between gap-3 text-xs select-none">
    <!-- Left: Triage Category Filter Chips -->
    <div class="flex items-center gap-1.5 flex-wrap">
      <span class="text-xs font-semibold text-content-secondary mr-1">结果分诊:</span>

      <!-- All -->
      <button
        @click="setFilter('all')"
        class="px-2.5 py-1 rounded-md transition-all font-medium flex items-center gap-1.5 border"
        :class="store.activeTriageFilter === 'all'
          ? 'bg-content-main text-canvas border-content-main font-semibold'
          : 'bg-card-subtle text-content-secondary border-border hover:border-content-muted'"
      >
        <span>全部</span>
        <span class="px-1.5 py-0.2 rounded-full text-[10px] bg-black/10 dark:bg-white/10">{{ totalTested }}</span>
      </button>

      <!-- Stable -->
      <button
        @click="setFilter('stable')"
        class="px-2.5 py-1 rounded-md transition-all font-medium flex items-center gap-1.5 border"
        :class="store.activeTriageFilter === 'stable'
          ? 'bg-emerald-600 text-white border-emerald-600'
          : 'bg-emerald-500/10 text-emerald-400 border-emerald-500/25 hover:bg-emerald-500/15'"
      >
        <span>✔ 稳定可用</span>
        <span class="px-1.5 py-0.2 rounded-full text-[10px] bg-emerald-500/20">{{ counts.stable }}</span>
      </button>

      <!-- Single Pass -->
      <button
        @click="setFilter('single_pass')"
        class="px-2.5 py-1 rounded-md transition-all font-medium flex items-center gap-1.5 border"
        :class="store.activeTriageFilter === 'single_pass'
          ? 'bg-blue-600 text-white border-blue-600'
          : 'bg-blue-500/10 text-blue-400 border-blue-500/25 hover:bg-blue-500/15'"
        title="初次采样全通，尚需复测验证稳定性"
      >
        <span>◈ 初测全通</span>
        <span class="px-1.5 py-0.2 rounded-full text-[10px] bg-blue-500/20">{{ counts.single_pass }}</span>
      </button>

      <!-- Unverified -->
      <button
        @click="setFilter('unverified')"
        class="px-2.5 py-1 rounded-md transition-all font-medium flex items-center gap-1.5 border"
        :class="store.activeTriageFilter === 'unverified'
          ? 'bg-orange-600 text-white border-orange-600'
          : 'bg-orange-500/10 text-orange-400 border-orange-500/25 hover:bg-orange-500/15'"
        title="采样部分失败，未观测到IP漂移，状态可疑"
      >
        <span>▲ 持续不稳定</span>
        <span class="px-1.5 py-0.2 rounded-full text-[10px] bg-orange-500/20">{{ counts.unverified }}</span>
      </button>

      <!-- Flapping -->
      <button
        @click="setFilter('flapping')"
        class="px-2.5 py-1 rounded-md transition-all font-medium flex items-center gap-1.5 border"
        :class="store.activeTriageFilter === 'flapping'
          ? 'bg-amber-600 text-white border-amber-600'
          : 'bg-amber-500/10 text-amber-400 border-amber-500/25 hover:bg-amber-500/15'"
        title="确认为多出口IP轮换导致的负载均衡漂移"
      >
        <span>⇄ 确认多出口漂移</span>
        <span class="px-1.5 py-0.2 rounded-full text-[10px] bg-amber-500/20">{{ counts.flapping }}</span>
      </button>

      <!-- Blocked -->
      <button
        @click="setFilter('blocked')"
        class="px-2.5 py-1 rounded-md transition-all font-medium flex items-center gap-1.5 border"
        :class="store.activeTriageFilter === 'blocked'
          ? 'bg-red-600 text-white border-red-600'
          : 'bg-red-500/10 text-red-400 border-red-500/25 hover:bg-red-500/15'"
        title="服务商明确返回地区封锁 (FAILED_PRECONDITION)"
      >
        <span>✖ 地区明确阻断</span>
        <span class="px-1.5 py-0.2 rounded-full text-[10px] bg-red-500/20">{{ counts.blocked }}</span>
      </button>

      <!-- Failed -->
      <button
        @click="setFilter('failed')"
        class="px-2.5 py-1 rounded-md transition-all font-medium flex items-center gap-1.5 border"
        :class="store.activeTriageFilter === 'failed'
          ? 'bg-zinc-600 text-white border-zinc-600'
          : 'bg-zinc-500/10 text-zinc-400 border-zinc-500/25 hover:bg-zinc-500/15'"
        title="当前采样全部超时或连接失败"
      >
        <span>∅ 当前不可用</span>
        <span class="px-1.5 py-0.2 rounded-full text-[10px] bg-zinc-500/20">{{ counts.failed }}</span>
      </button>
    </div>

    <!-- Right: Quick Retest Actions -->
    <div class="flex items-center gap-2">
      <button
        v-if="counts.unverified > 0 || counts.flapping > 0"
        @click="retestUnstable"
        class="px-2.5 py-1 rounded bg-amber-500/10 text-amber-400 border border-amber-500/30 hover:bg-amber-500/20 transition-colors flex items-center gap-1"
        title="一键复测不稳定与漂移节点"
      >
        <span>⚡</span> 复测可疑节点 ({{ counts.unverified + counts.flapping }})
      </button>
      <button
        v-if="counts.failed > 0"
        @click="retestFailed"
        class="px-2.5 py-1 rounded bg-zinc-500/10 text-zinc-400 border border-zinc-500/30 hover:bg-zinc-500/20 transition-colors flex items-center gap-1"
        title="复测所有失败节点以确认是否持续不可用"
      >
        <span>🔄</span> 复测失败节点 ({{ counts.failed }})
      </button>
    </div>
  </div>
</template>
