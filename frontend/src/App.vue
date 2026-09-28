<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useWorkbenchStore } from './stores/workbench'
import { useTimelineStore } from './stores/timeline'
import * as api from './api/bridge'

import SourceScopeBar from './components/workbench/SourceScopeBar.vue'
import AirportModal from './components/airport/AirportModal.vue'
import PreferencesModal from './components/settings/PreferencesModal.vue'
import MonitorTimelineView from './components/timeline/MonitorTimelineView.vue'
import MonitorJobsView from './components/monitor/MonitorJobsView.vue'
import LatencyWorkbench from './components/workbench/LatencyWorkbench.vue'
import NodeDetailView from './components/history/NodeDetailView.vue'
import ProfileSetupModal from './components/profile/ProfileSetupModal.vue'
import type { MonitorJobNode, MonitorJobPrefill, NodeDetailRequest, WorkbenchSaveRetryRequest } from './types'

const store = useWorkbenchStore()

/**
 * Top-level view switch.
 *
 * The workbench (batch speed-test triage) and the monitor timeline are separate working
 * surfaces. Keep the workbench mounted so visiting subscription management does not
 * abandon a pending measurement or save retry. The timeline still unmounts on exit.
 */
const activeView = ref<'workbench' | 'airports' | 'monitor-jobs' | 'timeline'>('workbench')
const workbenchRef = ref<InstanceType<typeof LatencyWorkbench>>()
function openProjectHistory(project: 'throughput' | 'service') {
  activeView.value = 'workbench'
  workbenchRef.value?.showProject(project)
}
const airportProfile = ref('all')
watch(() => store.isAirportModalOpen, open => {
  if (open) { activeView.value = 'airports'; store.isAirportModalOpen = false }
})
function browseProfile(id: string) { airportProfile.value = id; activeView.value = 'workbench' }
const monitorPrefill = ref<MonitorJobPrefill | null>(null)
const nodeDetail = ref<NodeDetailRequest | null>(null)
const nodeDetailKey = ref(0)
const workbenchSaveRetry = ref<WorkbenchSaveRetryRequest | null>(null)
const timelineStore = useTimelineStore()

function openMonitorFromWorkbench(prefill: MonitorJobPrefill): void {
  monitorPrefill.value = prefill
  activeView.value = 'monitor-jobs'
}

function clearMonitorPrefill(): void {
  monitorPrefill.value = null
}

function openNodeDetail(request: NodeDetailRequest): void {
  nodeDetail.value = request
  nodeDetailKey.value += 1
}

function closeNodeDetail(): void { nodeDetail.value = null }

function openWorkbenchSaveRetry(request: WorkbenchSaveRetryRequest): void {
  workbenchSaveRetry.value = request
  nodeDetail.value = null
  activeView.value = 'workbench'
}

function clearWorkbenchSaveRetry(attemptID: string): void {
  if (workbenchSaveRetry.value?.attempt_id === attemptID) workbenchSaveRetry.value = null
}

watch(activeView, async (view) => {
  if (view !== 'monitor-jobs') monitorPrefill.value = null
  await nextTick()
  window.scrollTo({ top: 0, behavior: 'instant' })
})

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
  await store.loadProfileSetup()
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
  <div class="app-shell">
    <SourceScopeBar v-model:active-view="activeView" />

    <div v-show="activeView === 'workbench'">
      <LatencyWorkbench ref="workbenchRef" :visible="activeView === 'workbench'" :initial-profile-id="airportProfile" :save-retry-request="workbenchSaveRetry" @save-retry-request-resolved="clearWorkbenchSaveRetry" @open-monitor="openMonitorFromWorkbench" @open-node-detail="openNodeDetail" />
    </div>

    <div v-if="activeView === 'monitor-jobs'" class="app-page">
      <MonitorJobsView :prefill="monitorPrefill" @prefill-consumed="clearMonitorPrefill" @open-timeline="openJobTimeline" @open-node-detail="openNodeDetail" />
    </div>

    <div v-else-if="activeView === 'timeline'" class="app-page">
      <MonitorTimelineView @open-node-detail="openNodeDetail" @open-project-history="openProjectHistory" />
    </div>

    <!-- Modals -->
    <AirportModal v-show="activeView === 'airports'" :visible="activeView === 'airports'" embedded @browse-profile="browseProfile" />
    <ProfileSetupModal />
    <PreferencesModal />
    <NodeDetailView v-if="nodeDetail" :key="nodeDetailKey" :scope="nodeDetail" @close="closeNodeDetail" @open-workbench-save-retry="openWorkbenchSaveRetry" />
  </div>
</template>
