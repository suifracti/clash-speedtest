<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useWorkbenchStore } from './stores/workbench'
import { useTimelineStore } from './stores/timeline'
import * as api from './api/bridge'

import SourceScopeBar from './components/workbench/SourceScopeBar.vue'
import LiveProgressStrip from './components/workbench/LiveProgressStrip.vue'
import ResultTriageBar from './components/workbench/ResultTriageBar.vue'
import TelemetryGrid from './components/workbench/TelemetryGrid.vue'
import SampleInspector from './components/workbench/SampleInspector.vue'
import StagingDock from './components/workbench/StagingDock.vue'
import AirportModal from './components/airport/AirportModal.vue'
import AirportConsolidatedMatrix from './components/history/AirportConsolidatedMatrix.vue'
import PreferencesModal from './components/settings/PreferencesModal.vue'
import MonitorTimelineView from './components/timeline/MonitorTimelineView.vue'
import MonitorJobsView from './components/monitor/MonitorJobsView.vue'
import type { MonitorJobNode } from './types'

const store = useWorkbenchStore()

/**
 * Top-level view switch.
 *
 * The workbench (batch speed-test triage) and the monitor timeline are separate working
 * surfaces. Only one is mounted at a time, so leaving the timeline stops its polling and
 * releasing the canvas; returning re-reads the newest page from the cursor API.
 */
const activeView = ref<'workbench' | 'monitor-jobs' | 'timeline'>('workbench')
const timelineStore = useTimelineStore()

async function openJobTimeline(payload: { profileId: string; node: MonitorJobNode }): Promise<void> {
	activeView.value = 'timeline'
	// The timeline keeps its existing cursor/pagination semantics; this only applies
	// the job's stable identity filters before showing the existing read-only view.
	try {
		await timelineStore.setNodeFilter(payload.node.nodeIdentityKey, payload.node.nodeKey)
		await timelineStore.setProfileFilter(payload.profileId)
	} catch {
		// The existing timeline renders the real read error; navigation itself remains available.
	}
}

let unsubscribeEvents: (() => void) | null = null

onMounted(async () => {
  // Subscribe to real-time streaming events (via Wails or SSE)
  unsubscribeEvents = api.subscribeEvents((type, payload) => {
    store.handleEvent(type, payload)
  })

  // Load initial data
  await Promise.all([
    store.loadAirports(),
    store.loadTokenStatus(),
    store.loadHistory(),
  ])
})

onUnmounted(() => {
  if (unsubscribeEvents) {
    unsubscribeEvents()
  }
})
</script>

<template>
  <div class="flex flex-col h-screen w-screen overflow-hidden bg-canvas text-content-main font-sans">
    <!-- Zone 1: Source & Scope Bar (shared app header, also hosts the view switch) -->
    <SourceScopeBar v-model:active-view="activeView" />

    <template v-if="activeView === 'workbench'">
      <!-- Zone 2: Live Progress Meter Strip -->
      <LiveProgressStrip />

      <!-- Zone 3: Result Triage Decision Band -->
      <ResultTriageBar />

      <!-- Zone 4 + Inspector: Workspace Grid & Detail Panel -->
      <main class="flex-1 flex overflow-hidden">
        <!-- Zone 4: Telemetry Table Grid -->
        <TelemetryGrid />

        <!-- Persistent Sample Inspector Dock -->
        <SampleInspector />
      </main>

      <!-- Zone 5: Staging & Export Dock -->
      <StagingDock />
    </template>

    <!-- Monitor / Stability Timeline: raw-sample telemetry console -->
    <MonitorJobsView v-else-if="activeView === 'monitor-jobs'" @open-timeline="openJobTimeline" />

    <MonitorTimelineView v-else />

    <!-- Modals -->
    <AirportModal />
    <AirportConsolidatedMatrix />
    <PreferencesModal />
  </div>
</template>
