<script setup lang="ts">
/**
 * Timeline filter bar.
 *
 * Every control here changes the query, so each one restarts cursor pagination from the newest
 * page (see the store's `filtersSignature`). The node picker carries the legacy node key along so
 * PR#3 backfilled samples remain queryable.
 */
import { computed, ref } from 'vue'
import { RANGE_OPTIONS, useTimelineStore, type RangeKey } from '../../stores/timeline'

const store = useTimelineStore()

const showCustom = ref(false)

const customSince = ref('')
const customUntil = ref('')

const selectedNode = computed({
  get: () => store.nodeIdentityKey,
  set: (value: string) => {
    const node = store.availableNodes.find((n) => n.nodeIdentityKey === value)
    void store.setNodeFilter(value, node?.nodeKey ?? '')
  },
})

function toLocalInputValue(ms: number): string {
  const d = new Date(ms - new Date().getTimezoneOffset() * 60_000)
  return d.toISOString().slice(0, 16)
}

function openCustom(): void {
  showCustom.value = true
  if (!customSince.value) customSince.value = toLocalInputValue(store.domainStartMs)
  if (!customUntil.value) customUntil.value = toLocalInputValue(store.domainEndMs)
}

function applyCustom(): void {
  const since = Date.parse(customSince.value)
  const until = Date.parse(customUntil.value)
  if (!Number.isFinite(since) || !Number.isFinite(until) || since >= until) return
  void store.setCustomRange(since, until)
  showCustom.value = false
}

function onRangeClick(key: RangeKey): void {
  if (key === 'custom') {
    openCustom()
    return
  }
  showCustom.value = false
  void store.setRange(key)
}

function shortTarget(target: string): string {
  if (!target) return '(未设置)'
  try {
    const url = new URL(target)
    return `${url.host}${url.pathname === '/' ? '' : url.pathname}`
  } catch {
    return target
  }
}
</script>

<template>
  <div class="bg-card border-b border-border px-3 py-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-[11px]">
    <!-- Time range -->
    <div class="flex items-center gap-1.5">
      <span class="text-content-muted">时间范围</span>
      <div class="flex items-center bg-card-subtle rounded-md border border-border p-0.5">
        <button
          v-for="option in RANGE_OPTIONS"
          :key="option.key"
          @click="onRangeClick(option.key)"
          :class="
            store.rangeKey === option.key
              ? 'bg-card text-brand shadow-sm font-medium'
              : 'text-content-secondary hover:text-content-main'
          "
          class="px-2 py-0.5 rounded transition-all"
        >
          {{ option.label }}
        </button>
      </div>
    </div>

    <!-- Custom range -->
    <div v-if="showCustom || store.rangeKey === 'custom'" class="flex items-center gap-1.5">
      <input
        v-model="customSince"
        type="datetime-local"
        class="bg-card-subtle border border-border rounded px-1.5 py-0.5 font-mono text-content-main focus:outline-none focus:border-brand"
      />
      <span class="text-content-muted">→</span>
      <input
        v-model="customUntil"
        type="datetime-local"
        class="bg-card-subtle border border-border rounded px-1.5 py-0.5 font-mono text-content-main focus:outline-none focus:border-brand"
      />
      <button
        @click="applyCustom"
        class="px-2 py-0.5 rounded border border-border hover:border-brand hover:text-brand transition-colors"
      >
        应用
      </button>
    </div>

    <!-- Node -->
    <label class="flex items-center gap-1.5">
      <span class="text-content-muted">节点</span>
      <select
        v-model="selectedNode"
        class="bg-card-subtle border border-border rounded px-1.5 py-0.5 text-content-main max-w-[190px] focus:outline-none focus:border-brand"
      >
        <option value="">全部节点</option>
        <option v-for="node in store.availableNodes" :key="node.nodeIdentityKey" :value="node.nodeIdentityKey">
          {{ node.displayName || node.nodeIdentityKey }} ({{ node.sampleCount }})
        </option>
      </select>
    </label>

    <!-- Profile -->
    <label class="flex items-center gap-1.5">
      <span class="text-content-muted">Profile</span>
      <select
        :value="store.profileId"
        @change="store.setProfileFilter(($event.target as HTMLSelectElement).value)"
        class="bg-card-subtle border border-border rounded px-1.5 py-0.5 text-content-main max-w-[150px] focus:outline-none focus:border-brand"
      >
        <option value="">全部</option>
        <option v-for="p in store.availableProfiles" :key="p" :value="p">{{ p }}</option>
      </select>
    </label>

    <!-- Probe type -->
    <label class="flex items-center gap-1.5">
      <span class="text-content-muted">Probe</span>
      <select
        :value="store.probeType"
        @change="store.setProbeTypeFilter(($event.target as HTMLSelectElement).value)"
        class="bg-card-subtle border border-border rounded px-1.5 py-0.5 text-content-main max-w-[150px] focus:outline-none focus:border-brand"
      >
        <option value="">全部</option>
        <option v-for="p in store.availableProbeTypes" :key="p" :value="p">{{ p }}</option>
      </select>
    </label>

    <!-- Target -->
    <label class="flex items-center gap-1.5">
      <span class="text-content-muted">Target</span>
      <select
        :value="store.target"
        @change="store.setTargetFilter(($event.target as HTMLSelectElement).value)"
        class="bg-card-subtle border border-border rounded px-1.5 py-0.5 text-content-main max-w-[220px] focus:outline-none focus:border-brand"
      >
        <option value="">全部</option>
        <option v-for="t in store.availableTargets" :key="t" :value="t">{{ shortTarget(t) }}</option>
      </select>
    </label>

    <!-- Timezone + reset -->
    <div class="flex items-center gap-2 ml-auto">
      <button
        @click="store.useUtc = !store.useUtc"
        class="px-2 py-0.5 rounded border transition-colors"
        :class="store.useUtc ? 'border-brand text-brand' : 'border-border text-content-secondary hover:text-content-main'"
        title="切换时间轴与 Inspector 的时间基准"
      >
        {{ store.useUtc ? 'UTC' : '本地时间' }}
      </button>
      <button
        @click="store.resetFilters()"
        class="px-2 py-0.5 rounded border border-border text-content-secondary hover:text-content-main hover:border-border-subtle transition-colors"
      >
        重置
      </button>
    </div>
  </div>
</template>
