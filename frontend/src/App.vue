<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useWorkbenchStore } from './stores/workbench'
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

const store = useWorkbenchStore()

/**
 * Top-level view switch.
 *
 * The workbench (batch speed-test triage) and the monitor timeline are separate working
 * surfaces. Only one is mounted at a time, so leaving the timeline stops its polling and
 * releasing the canvas; returning re-reads the newest page from the cursor API.
 */
const activeView = ref<'workbench' | 'timeline'>('workbench')

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
    <MonitorTimelineView v-else />

    <!-- Modals -->
    <AirportModal />
    <AirportConsolidatedMatrix />
    <PreferencesModal />
  </div>
</template>
