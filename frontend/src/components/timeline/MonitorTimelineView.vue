<script setup lang="ts">
/**
 * Monitor / Stability Timeline page container.
 *
 * Composition: filters → observation summary → raw-sample timeline + sample inspector.
 *
 * Two product rules drive the state handling below.
 *
 * 1. **Every state is real.** Loading, empty, partial-loaded, load-more, API error,
 *    "no samples in range", "legacy history available" and "filter has no match" are all
 *    distinct, honestly-labelled states. When there is no data the page says so; it never fills
 *    the canvas with mock samples, because demo dots would be indistinguishable from real
 *    history and would destroy the evidentiary value of the timeline.
 * 2. **No fact compression.** The timeline renders exactly the raw samples that were loaded.
 *    The readout inside the timeline makes the difference between *data loaded* and *geometry
 *    drawn* explicit, so viewport culling can never be mistaken for sampling.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useTimelineStore } from '../../stores/timeline'
import { OUTCOME_LABELS, type SampleOutcome } from '../../utils/timeline/encoding'
import TimelineFilterBar from './TimelineFilterBar.vue'
import TimelineEvidenceBar from './TimelineEvidenceBar.vue'
import SampleTimeline from './SampleTimeline.vue'
import TimelineInspector from './TimelineInspector.vue'
import WorkbenchBatchHistoryPanel from '../history/WorkbenchBatchHistoryPanel.vue'
import type { NodeDetailRequest } from '../../types'

const activeSubView = ref<'monitor' | 'workbench'>('monitor')
const store = useTimelineStore()
const emit = defineEmits<{ (event: 'open-node-detail', payload: NodeDetailRequest): void; (event: 'open-project-history', project: 'throughput' | 'service'): void }>()

/** Narrowing filters, i.e. everything except the time range itself. */
const hasActiveNarrowing = computed(
  () => !!(store.nodeIdentityKey || store.profileId || store.probeType || store.target || store.samplingTier)
)

/** Whether storage holds any samples at all, used to tell "empty range" from "empty history". */
const hasAnyHistory = computed(() => (store.facets?.nodes.length ?? 0) > 0)

const showTimeline = computed(() => store.samples.length > 0)

type EmptyReason = 'filter-no-match' | 'no-samples-in-range' | 'no-history'

const emptyReason = computed<EmptyReason>(() => {
  if (hasActiveNarrowing.value) return 'filter-no-match'
  if (hasAnyHistory.value) return 'no-samples-in-range'
  return 'no-history'
})

/** `null` means "render the timeline"; anything else is a real placeholder state. */
const stateKind = computed<'loading' | 'error' | 'empty' | null>(() => {
  if (showTimeline.value) return null
  if (store.loadState === 'error') return 'error'
  if (store.loadState === 'ready') return 'empty'
  return 'loading'
})

/** Shape grammar legend. Kept beside the plot so a glance is interpretable without hovering. */
const legend: { outcome: SampleOutcome; glyph: string }[] = [
  { outcome: 'success', glyph: '│•' },
  { outcome: 'transport_failure', glyph: '✕' },
  { outcome: 'service_failure', glyph: '╱' },
  { outcome: 'unknown_failure', glyph: '○' },
]

onMounted(async () => {
  await store.reload()
  store.startAutoRefresh()
})

onBeforeUnmount(() => {
  store.stopAutoRefresh()
})
</script>

<template>
  <div class="prototype-history-page flex flex-col flex-1 min-h-0 bg-canvas">
    <section class="history-page-heading flex flex-wrap justify-between items-end gap-3">
      <div>
        <p class="history-eyebrow">记录档案 / 回看变化</p>
        <h2>历史记录</h2>
        <p>手动测试看单次结果，自动监测看长期变化。</p>
      </div>
      <div class="history-tabs flex gap-2">
        <button
          type="button"
          class="prototype-tab-button"
          :class="{ active: activeSubView === 'monitor' }"
          @click="activeSubView = 'monitor'"
        >
          <strong>自动监测</strong><small>按时间观察稳定性</small>
        </button>
        <button
          type="button"
          class="prototype-tab-button"
          :class="{ active: activeSubView === 'workbench' }"
          @click="activeSubView = 'workbench'"
        >
          <strong>手动延迟测试</strong><small>按测试批次回看</small>
        </button>
      </div>
    </section>

    <div class="history-shortcuts"><span>其他手动记录</span><button @click="emit('open-project-history', 'throughput')">↓ 下载测速记录 →</button><button @click="emit('open-project-history', 'service')">◈ 服务检查记录 →</button><small>进入对应工作台回看已保存结果，不会开始测试。</small></div>
    <WorkbenchBatchHistoryPanel v-if="activeSubView === 'workbench'" @open-node-detail="emit('open-node-detail', $event)" />

    <template v-else>
    <p class="history-source-note">这里保存定时监测产生的样本。你主动发起的延迟测试在 <button @click="activeSubView = 'workbench'">手动测试 →</button>，两类记录不会混算。</p>
    <!-- Filters: any change restarts cursor pagination from the newest page -->
    <TimelineFilterBar class="history-filters" />

    <!-- Observation summary: current sample vs bounded evidence windows -->
    <TimelineEvidenceBar v-if="showTimeline" />

    <!-- Legacy history notice: migration-bridged rows are still raw facts, but flagged -->
    <div
      v-if="store.hasLegacySamples"
      class="px-3 py-1 border-b border-border bg-amber-500/10 text-[11px] text-amber-700 dark:text-amber-300"
    >
      当前区间包含迁移桥接的历史样本（NodeIdentityKey 由 NodeKey 回填）。它们仍可逐条查看，
      但身份标识与迁移后新增的样本不同。
    </div>

    <!-- Partial load: state the boundary explicitly and offer an explicit load-more -->
    <div
      v-if="store.hasMore && store.samples.length > 0"
      class="px-3 py-1 border-b border-border bg-card flex flex-wrap items-center gap-3 text-[11px]"
    >
      <span class="text-content-muted">
        已加载 {{ store.pagesLoaded }} 页 / {{ store.samples.length }} 条原始样本，左侧仍有更旧的历史未加载。
      </span>
      <button
        @click="store.loadOlder()"
        :disabled="store.loadingOlder"
        class="px-2 py-0.5 rounded border border-border hover:border-brand hover:text-brand transition-colors disabled:opacity-50"
      >
        {{ store.loadingOlder ? '加载中…' : '加载更旧样本' }}
      </button>
      <span class="text-content-muted">
        向左平移时间轴到已加载边界时也会自动续读。
      </span>
    </div>

    <main class="history-canvas flex-1 flex min-h-0">
      <div class="flex-1 flex flex-col min-w-0">
        <!-- Shape grammar -->
        <div v-if="showTimeline" class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-1 border-b border-border text-[10px] text-content-muted">
          <span class="text-content-secondary">结果图例</span>
          <span v-for="item in legend" :key="item.outcome" class="flex items-center gap-1 font-mono">
            <span class="text-content-main">{{ item.glyph }}</span>
            <span>{{ OUTCOME_LABELS[item.outcome] }}</span>
          </span>
          <span class="ml-auto">
            传输失败（✕）与服务层失败（╱）分开表达：地区/策略阻断不等于节点不可用
          </span>
        </div>

        <SampleTimeline v-if="showTimeline" />

        <!-- Real states. No mock data is ever substituted here. -->
        <div v-else class="flex-1 flex items-center justify-center px-6">
          <!-- Loading -->
          <div v-if="stateKind === 'loading'" class="flex flex-col items-center gap-2 text-center">
            <span
              class="w-5 h-5 rounded-full border-2 border-border border-t-brand animate-spin"
              aria-hidden="true"
            ></span>
            <span class="text-xs text-content-secondary">正在读取最近的检测记录…</span>
            <span class="text-[10px] text-content-muted">
              请稍候。
            </span>
          </div>

          <!-- API error -->
          <div v-else-if="stateKind === 'error'" class="max-w-[520px] flex flex-col items-center gap-2 text-center">
            <span class="text-xs font-medium text-red-600 dark:text-red-400">读取监控样本失败</span>
            <span class="text-[11px] text-content-muted font-mono break-all">{{ store.loadError }}</span>
            <button
              @click="store.reload()"
              class="mt-1 px-3 py-1 rounded border border-border hover:border-brand hover:text-brand text-[11px] transition-colors"
            >
              重试
            </button>
          </div>

          <!-- Empty: three genuinely different reasons -->
          <div v-else class="max-w-[560px] flex flex-col items-center gap-2 text-center">
            <div class="history-empty-icon" aria-hidden="true">↶</div>
            <template v-if="emptyReason === 'filter-no-match'">
              <span class="text-xs font-medium text-content-main">当前筛选条件在该时间范围内没有匹配样本</span>
              <span class="text-[11px] text-content-muted leading-relaxed">
                还有其他记录，试试更换节点、订阅、检查内容或检测目标。
              </span>
              <button
                @click="store.resetFilters()"
                class="mt-1 px-3 py-1 rounded border border-border hover:border-brand hover:text-brand text-[11px] transition-colors"
              >
                重置筛选条件
              </button>
            </template>

            <template v-else-if="emptyReason === 'no-samples-in-range'">
              <span class="text-xs font-medium text-content-main">所选时间范围内没有样本</span>
              <span class="text-[11px] text-content-muted leading-relaxed">
                历史库中仍有其它时间段的样本。可以切换到更大的时间范围继续查看。
              </span>
              <button
                @click="store.setCustomRange(Date.now() - 30 * 86400000, Date.now())"
                class="mt-1 px-3 py-1 rounded border border-border hover:border-brand hover:text-brand text-[11px] transition-colors"
              >
                查看最近 30 天
              </button>
            </template>

            <template v-else>
              <span class="text-xs font-medium text-content-main">监控历史中还没有任何样本</span>
              <span class="text-[11px] text-content-muted leading-relaxed">
                在“持续监测”中创建并启动任务后，这里会积累记录。
                刚才主动点击测试了？可以切换到“手动测试”查看。
              </span>
            </template>
          </div>
        </div>
      </div>

      <!-- Stable evidence entry point: click-selected, keyboard reachable -->
      <TimelineInspector v-if="showTimeline" @open-node-detail="emit('open-node-detail', $event)" />
    </main>
    </template>
  </div>
</template>

<style scoped>
.history-page-heading { padding: 14px 0 24px; }
.history-eyebrow { margin: 0 0 6px; color: var(--primary); font-size: 12px; font-weight: 750; letter-spacing: .08em; }
.history-page-heading h2 { margin: 0 0 8px; color: var(--text-main); font-size: 26px; font-weight: 650; letter-spacing: -.03em; }
.history-page-heading p:last-child { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
.prototype-tab-button {
  padding: 6px 14px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--card-subtle);
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all .15s ease;
}
.prototype-tab-button:hover {
  border-color: var(--border-focus);
  color: var(--text-main);
  background: var(--card-hover);
}
.prototype-tab-button.active {
  border-color: var(--primary);
  background: var(--primary);
  color: white;
}
.history-filters { padding: 18px 20px; border: 1px solid var(--border); border-radius: 12px 12px 0 0; }
.history-source-note { margin: 0 0 14px; font-size: 12px; color: var(--text-secondary); } .history-source-note button { color: var(--primary); font-weight: 600; }
.history-canvas { min-height: 380px; background: var(--card); border: 1px solid var(--border); border-top: 0; border-radius: 0 0 12px 12px; }
.history-canvas > div > div:first-child { padding: 12px 16px; }
.history-canvas :deep(aside) { width: 270px; background: var(--card-subtle); }
@media (max-width: 950px) { .history-canvas { flex-direction: column; } .history-canvas :deep(aside) { width: 100%; border-left: 0; border-top: 1px solid var(--border); } }
</style>
