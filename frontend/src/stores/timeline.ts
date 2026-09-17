/**
 * Timeline state management.
 *
 * Design constraints this store encodes:
 *
 * 1. **Raw samples stay raw.** The store keeps every loaded sample keyed by `sample_id`. Nothing
 *    is aggregated, bucketed, or replaced by an interval summary. Aggregation only ever happens
 *    in the separate evidence-stats read model, which is explicitly labelled as a window summary.
 * 2. **Single-direction cursor.** The history layer exposes a keyset cursor that only walks
 *    *older* (newest-first). There is no prev-cursor, so "load newer" is implemented by
 *    re-querying the newest page and merging by `sample_id`, never by fabricating a reverse
 *    cursor.
 * 3. **Filter changes discard the cursor.** Any change to range / node / profile / probe / target
 *    restarts pagination from the newest page.
 * 4. **Background refresh never moves the viewport.** New samples are merged in place; the
 *    viewport only follows the live edge when the user is actually at the live edge.
 */

import { computed, ref, shallowRef } from 'vue'
import { defineStore } from 'pinia'
import type { DerivedStats, MonitorSample, MonitorSampleFacets } from '../types'
import { fetchMonitorFacets, fetchMonitorStats, queryMonitorSamplesCursor } from '../api/monitor'
import { buildLanes, compareSamplesAsc, detectRevisionBoundaries, isLegacyBackfilledSample } from '../utils/timeline/lanes'
import {
  clampViewportToDomain,
  isAtLiveEdge,
  needsOlderData,
  panViewport as panViewportPure,
  viewportForRange,
  viewportSpanMs,
  zoomViewport as zoomViewportPure,
  type TimelineViewport,
} from '../utils/timeline/viewport'

const HOUR = 3_600_000
const DAY = 24 * HOUR

export type RangeKey = '1h' | '6h' | '24h' | '7d' | 'custom'

export const RANGE_OPTIONS: { key: RangeKey; label: string; durationMs: number | null }[] = [
  { key: '1h', label: '最近 1 小时', durationMs: HOUR },
  { key: '6h', label: '最近 6 小时', durationMs: 6 * HOUR },
  { key: '24h', label: '最近 24 小时', durationMs: DAY },
  { key: '7d', label: '最近 7 天', durationMs: 7 * DAY },
  { key: 'custom', label: '自定义区间', durationMs: null },
]

export type LoadState = 'idle' | 'loading' | 'ready' | 'error'

/** Samples per cursor page. Kept within the API's 1..1000 bound. */
export const PAGE_LIMIT = 500

export const useTimelineStore = defineStore('timeline', () => {
  // --- Filters -------------------------------------------------------------
  const rangeKey = ref<RangeKey>('24h')
  const customSinceMs = ref<number | null>(null)
  const customUntilMs = ref<number | null>(null)
  const nodeIdentityKey = ref('')
  const legacyNodeKey = ref('')
  const profileId = ref('')
  const probeType = ref('')
  const target = ref('')

  /** Display timestamps in UTC instead of the host timezone. Local is the default. */
  const useUtc = ref(false)

  /**
   * Identity of the active filter set. Any change here invalidates the cursor and all loaded
   * samples, because a keyset cursor is only valid for the exact filter that produced it.
   */
  const filtersSignature = computed(() =>
    [
      rangeKey.value,
      customSinceMs.value ?? '',
      customUntilMs.value ?? '',
      nodeIdentityKey.value,
      legacyNodeKey.value,
      profileId.value,
      probeType.value,
      target.value,
      'order:desc',
    ].join('\u0001')
  )

  // --- Facets --------------------------------------------------------------
  const facets = ref<MonitorSampleFacets | null>(null)
  const facetsLoading = ref(false)
  const facetsError = ref<string | null>(null)

  // --- Raw sample store ----------------------------------------------------
  /** Non-reactive source of truth so a 10k-sample merge does not proxy every object. */
  const sampleIndex = new Map<string, MonitorSample>()
  const samples = shallowRef<MonitorSample[]>([])

  const nextCursor = ref('')
  const hasMore = ref(false)
  const pagesLoaded = ref(0)

  const loadState = ref<LoadState>('idle')
  const loadError = ref<string | null>(null)
  const loadingOlder = ref(false)

  // --- Live refresh --------------------------------------------------------
  const newSampleCount = ref(0)
  const lastRefreshAtMs = ref<number | null>(null)
  const followLive = ref(true)
  const refreshIntervalMs = ref(15_000)
  let refreshTimer: ReturnType<typeof setInterval> | null = null
  /** Guards against a refresh landing after a filter change reset the dataset. */
  let activeSignature = ''

  // --- Viewport ------------------------------------------------------------
  const widthPx = ref(1000)
  const viewport = shallowRef<TimelineViewport>({ startMs: 0, endMs: 0, widthPx: 1 })

  // --- Evidence stats ------------------------------------------------------
  const rangeStats = ref<DerivedStats | null>(null)
  const evidence24h = ref<DerivedStats | null>(null)
  const evidence7d = ref<DerivedStats | null>(null)
  const statsLoading = ref(false)
  const statsError = ref<string | null>(null)

  // --- Selection -----------------------------------------------------------
  const selectedSampleId = ref<string | null>(null)
  /** Samples sharing the selected sample's pixel column, nearest-first. */
  const candidates = ref<string[]>([])
  const candidateIndex = ref(0)

  // --- Derived -------------------------------------------------------------
  const domainEndMs = computed(() => {
    if (rangeKey.value === 'custom') return customUntilMs.value ?? Date.now()
    return Date.now()
  })

  const domainStartMs = computed(() => {
    if (rangeKey.value === 'custom') {
      return customSinceMs.value ?? domainEndMs.value - DAY
    }
    const option = RANGE_OPTIONS.find((o) => o.key === rangeKey.value)
    return domainEndMs.value - (option?.durationMs ?? DAY)
  })

  const lanes = computed(() => buildLanes(samples.value))
  const revisionBoundaries = computed(() => detectRevisionBoundaries(samples.value))

  const oldestLoadedMs = computed(() =>
    samples.value.length > 0 ? samples.value[0].timestampMs : null
  )
  const newestLoadedMs = computed(() =>
    samples.value.length > 0 ? samples.value[samples.value.length - 1].timestampMs : null
  )

  const latestSample = computed(() =>
    samples.value.length > 0 ? samples.value[samples.value.length - 1] : null
  )

  const selectedSample = computed(() => {
    if (!selectedSampleId.value) return null
    return sampleIndex.get(selectedSampleId.value) ?? null
  })

  const selectedLaneKey = computed(() => {
    const sample = selectedSample.value
    if (!sample) return ''
    const lane = lanes.value.find((l) => l.samples.some((s) => s.sampleId === sample.sampleId))
    return lane?.key ?? ''
  })

  /** True once the dataset is known to be missing older samples for the active range. */
  const isPartiallyLoaded = computed(() => hasMore.value && samples.value.length > 0)

  const isEmpty = computed(() => loadState.value === 'ready' && samples.value.length === 0)

  /** True when the loaded set contains PR#3 rows that the migration bridged by copying node_key. */
  const hasLegacySamples = computed(() => samples.value.some(isLegacyBackfilledSample))

  const availableProbeTypes = computed(() => {
    const fromFacets = facets.value?.probeTypes ?? []
    if (fromFacets.length > 0) return fromFacets
    return Array.from(new Set(samples.value.map((s) => s.probeType))).filter(Boolean).sort()
  })

  const availableTargets = computed(() => {
    const fromFacets = facets.value?.targets ?? []
    if (fromFacets.length > 0) return fromFacets
    return Array.from(new Set(samples.value.map((s) => s.target))).filter(Boolean).sort()
  })

  const availableProfiles = computed(() => {
    const fromFacets = facets.value?.profiles ?? []
    if (fromFacets.length > 0) return fromFacets
    return Array.from(new Set(samples.value.map((s) => s.profileId))).filter(Boolean).sort()
  })

  const availableNodes = computed(() => facets.value?.nodes ?? [])

  // --- Internal helpers ----------------------------------------------------
  function publish(): void {
    samples.value = Array.from(sampleIndex.values()).sort(compareSamplesAsc)
  }

  function clearDataset(): void {
    sampleIndex.clear()
    samples.value = []
    nextCursor.value = ''
    hasMore.value = false
    pagesLoaded.value = 0
    selectedSampleId.value = null
    candidates.value = []
    candidateIndex.value = 0
    newSampleCount.value = 0
  }

  function currentFilter() {
    return {
      nodeIdentityKey: nodeIdentityKey.value || undefined,
      legacyNodeKey: legacyNodeKey.value || undefined,
      profileId: profileId.value || undefined,
      probeType: probeType.value || undefined,
      target: target.value || undefined,
      sinceMs: domainStartMs.value,
      untilMs: domainEndMs.value,
    }
  }

  function syncViewportToDomain(preserveSpan: boolean): void {
    const start = domainStartMs.value
    const end = domainEndMs.value
    const currentSpan = viewportSpanMs(viewport.value)

    if (!preserveSpan || currentSpan <= 0) {
      viewport.value = viewportForRange(start, end, widthPx.value)
      return
    }

    const span = Math.min(currentSpan, Math.max(1, end - start))
    viewport.value = clampViewportToDomain(
      { startMs: end - span, endMs: end, widthPx: widthPx.value },
      start,
      end
    )
  }

  function setWidth(px: number): void {
    const next = Math.max(1, Math.floor(px))
    if (next === widthPx.value) return
    widthPx.value = next
    viewport.value = { ...viewport.value, widthPx: next }
  }

  // --- Loading -------------------------------------------------------------

  /** Loads the newest page and replaces the dataset. Always restarts pagination. */
  async function loadNewestPage(): Promise<void> {
    const signature = filtersSignature.value
    activeSignature = signature
    loadState.value = 'loading'
    loadError.value = null

    try {
      const page = await queryMonitorSamplesCursor({ ...currentFilter(), limit: PAGE_LIMIT, orderDesc: true })
      if (signature !== activeSignature) return

      clearDataset()
      for (const sample of page.items) sampleIndex.set(sample.sampleId, sample)
      nextCursor.value = page.nextCursor
      hasMore.value = page.hasMore
      pagesLoaded.value = 1
      publish()
      loadState.value = 'ready'
    } catch (e) {
      if (signature !== activeSignature) return
      loadState.value = 'error'
      loadError.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** Appends the next (older) page using the cursor produced by the previous page. */
  async function loadOlder(): Promise<void> {
    if (loadingOlder.value || !hasMore.value || !nextCursor.value) return
    if (loadState.value === 'loading') return

    const signature = activeSignature
    loadingOlder.value = true
    try {
      const page = await queryMonitorSamplesCursor({
        ...currentFilter(),
        limit: PAGE_LIMIT,
        orderDesc: true,
        cursor: nextCursor.value,
      })
      if (signature !== activeSignature) return

      let added = 0
      for (const sample of page.items) {
        if (sampleIndex.has(sample.sampleId)) continue
        sampleIndex.set(sample.sampleId, sample)
        added += 1
      }
      nextCursor.value = page.nextCursor
      hasMore.value = page.hasMore
      pagesLoaded.value += 1
      if (added > 0) publish()
    } catch (e) {
      if (signature !== activeSignature) return
      loadError.value = e instanceof Error ? e.message : String(e)
    } finally {
      loadingOlder.value = false
    }
  }

  /**
   * Re-queries the newest page and merges by `sample_id`.
   *
   * The cursor API is single-direction, so "load newer" cannot use a reverse cursor. Instead we
   * re-read the head of the stream and merge. Already-loaded samples are never duplicated and
   * the viewport is only advanced when the user is actually sitting at the live edge.
   */
  async function refreshNewest(): Promise<number> {
    if (loadState.value !== 'ready') return 0
    const signature = activeSignature
    if (signature !== filtersSignature.value) return 0

    try {
      const page = await queryMonitorSamplesCursor({ ...currentFilter(), limit: PAGE_LIMIT, orderDesc: true })
      if (signature !== activeSignature) return 0

      let added = 0
      for (const sample of page.items) {
        if (sampleIndex.has(sample.sampleId)) continue
        sampleIndex.set(sample.sampleId, sample)
        added += 1
      }

      if (added > 0) {
        publish()
        if (followLive.value) {
          const latest = newestLoadedMs.value
          if (latest !== null) {
            const span = viewportSpanMs(viewport.value)
            viewport.value = clampViewportToDomain(
              { startMs: latest - span, endMs: latest, widthPx: widthPx.value },
              domainStartMs.value,
              Math.max(domainEndMs.value, latest)
            )
          }
        } else {
          newSampleCount.value += added
        }
      }

      lastRefreshAtMs.value = Date.now()
      return added
    } catch {
      // A background refresh failure must not destroy the loaded view.
      return 0
    }
  }

  /** Full (re)load for the active filter: facets, newest page, evidence stats. */
  async function reload(): Promise<void> {
    newSampleCount.value = 0
    followLive.value = true
    syncViewportToDomain(false)
    await Promise.all([loadNewestPage(), loadFacets(), loadEvidence()])
  }

  async function loadFacets(): Promise<void> {
    facetsLoading.value = true
    facetsError.value = null
    try {
      // Facets describe what exists in storage, so they are deliberately read over a wider
      // window than the active range — otherwise switching node would be impossible.
      facets.value = await fetchMonitorFacets(Date.now() - 30 * DAY, Date.now())
    } catch (e) {
      facetsError.value = e instanceof Error ? e.message : String(e)
    } finally {
      facetsLoading.value = false
    }
  }

  async function loadEvidence(): Promise<void> {
    statsLoading.value = true
    statsError.value = null
    const now = Date.now()
    const base = currentFilter()

    try {
      const [range, day, week] = await Promise.all([
        fetchMonitorStats({ ...base, sinceMs: domainStartMs.value, untilMs: domainEndMs.value }),
        fetchMonitorStats({ ...base, sinceMs: now - DAY, untilMs: now }),
        fetchMonitorStats({ ...base, sinceMs: now - 7 * DAY, untilMs: now }),
      ])
      rangeStats.value = range
      evidence24h.value = day
      evidence7d.value = week
    } catch (e) {
      statsError.value = e instanceof Error ? e.message : String(e)
    } finally {
      statsLoading.value = false
    }
  }

  // --- Filter mutations ----------------------------------------------------

  function setRange(key: RangeKey): Promise<void> {
    if (rangeKey.value === key) return Promise.resolve()
    rangeKey.value = key
    return reload()
  }

  function setCustomRange(sinceMs: number, untilMs: number): Promise<void> {
    customSinceMs.value = sinceMs
    customUntilMs.value = untilMs
    rangeKey.value = 'custom'
    return reload()
  }

  /**
   * Applies a node filter.
   *
   * `legacyKey` is passed through as well so PR#3 samples — whose `node_identity_key` was
   * backfilled from `node_key` by the migration — stay reachable through the documented bridge
   * predicate instead of disappearing behind the newer identity.
   */
  function setNodeFilter(identity: string, legacyKey = ''): Promise<void> {
    if (nodeIdentityKey.value === identity && legacyNodeKey.value === legacyKey) return Promise.resolve()
    nodeIdentityKey.value = identity
    legacyNodeKey.value = identity ? legacyKey : ''
    // A node filter also clears the node-scoped probe/target narrowing.
    probeType.value = ''
    target.value = ''
    return reload()
  }

  function setProfileFilter(value: string): Promise<void> {
    if (profileId.value === value) return Promise.resolve()
    profileId.value = value
    return reload()
  }

  function setProbeTypeFilter(value: string): Promise<void> {
    if (probeType.value === value) return Promise.resolve()
    probeType.value = value
    return reload()
  }

  function setTargetFilter(value: string): Promise<void> {
    if (target.value === value) return Promise.resolve()
    target.value = value
    return reload()
  }

  function resetFilters(): Promise<void> {
    nodeIdentityKey.value = ''
    legacyNodeKey.value = ''
    profileId.value = ''
    probeType.value = ''
    target.value = ''
    rangeKey.value = '24h'
    return reload()
  }

  // --- Viewport interactions ----------------------------------------------

  function zoom(factor: number, anchorPx: number): void {
    viewport.value = clampViewportToDomain(
      zoomViewportPure(viewport.value, factor, anchorPx),
      domainStartMs.value,
      Math.max(domainEndMs.value, newestLoadedMs.value ?? domainEndMs.value)
    )
    syncFollowLive()
  }

  function pan(deltaPx: number): void {
    viewport.value = clampViewportToDomain(
      panViewportPure(viewport.value, deltaPx),
      domainStartMs.value,
      Math.max(domainEndMs.value, newestLoadedMs.value ?? domainEndMs.value)
    )
    syncFollowLive()
  }

  function panToRange(startMs: number, endMs: number): void {
    viewport.value = clampViewportToDomain(
      { startMs, endMs, widthPx: widthPx.value },
      domainStartMs.value,
      Math.max(domainEndMs.value, newestLoadedMs.value ?? domainEndMs.value)
    )
    syncFollowLive()
  }

  function resetToLatest(): void {
    const span = Math.max(1, viewportSpanMs(viewport.value))
    const end = Math.max(domainEndMs.value, newestLoadedMs.value ?? domainEndMs.value)
    const start = Math.max(domainStartMs.value, end - span)
    viewport.value = { startMs: start, endMs: end, widthPx: widthPx.value }
    followLive.value = true
    newSampleCount.value = 0
  }

  function syncFollowLive(): void {
    const latest = Math.max(newestLoadedMs.value ?? 0, domainEndMs.value)
    followLive.value = isAtLiveEdge(viewport.value, latest, Math.max(1000, viewportSpanMs(viewport.value) * 0.02))
    if (followLive.value) newSampleCount.value = 0
  }

  /** Requests the next older page once the viewport reaches the loaded left edge. */
  function maybeLoadOlder(): void {
    if (!hasMore.value || loadingOlder.value) return
    if (needsOlderData(viewport.value, oldestLoadedMs.value, 160)) {
      void loadOlder()
    }
  }

  // --- Selection -----------------------------------------------------------

  function selectSample(sampleId: string, hitCandidates: string[] = []): void {
    if (!sampleIndex.has(sampleId)) return
    selectedSampleId.value = sampleId
    candidates.value = hitCandidates.length > 0 ? hitCandidates : [sampleId]
    const idx = candidates.value.indexOf(sampleId)
    candidateIndex.value = idx >= 0 ? idx : 0
  }

  function clearSelection(): void {
    selectedSampleId.value = null
    candidates.value = []
    candidateIndex.value = 0
  }

  /** Steps through samples that share the same pixel column. */
  function cycleCandidate(direction: 1 | -1): void {
    if (candidates.value.length <= 1) return
    const next = (candidateIndex.value + direction + candidates.value.length) % candidates.value.length
    candidateIndex.value = next
    selectedSampleId.value = candidates.value[next]
  }

  /**
   * Moves the selection to the previous/next sample in the same lane, in time order.
   * This is the keyboard path to evidence, so it never depends on hover or the tooltip.
   */
  function selectRelativeInLane(direction: 1 | -1): void {
    const current = selectedSample.value
    if (!current) {
      const lane = lanes.value[0]
      if (lane && lane.samples.length > 0) {
        selectSample(lane.samples[direction === 1 ? 0 : lane.samples.length - 1].sampleId)
      }
      return
    }

    const lane = lanes.value.find((l) => l.key === selectedLaneKey.value)
    if (!lane) return

    const position = lane.samples.findIndex((s) => s.sampleId === current.sampleId)
    if (position < 0) return
    const next = position + direction
    if (next < 0 || next >= lane.samples.length) return
    selectSample(lane.samples[next].sampleId)
  }

  /** Moves the selection to the first sample of the adjacent lane. */
  function selectAdjacentLane(direction: 1 | -1): void {
    if (lanes.value.length === 0) return
    const currentKey = selectedLaneKey.value
    const index = lanes.value.findIndex((l) => l.key === currentKey)
    const targetIndex = index < 0 ? 0 : index + direction
    if (targetIndex < 0 || targetIndex >= lanes.value.length) return
    const lane = lanes.value[targetIndex]
    if (lane.samples.length > 0) selectSample(lane.samples[0].sampleId)
  }

  // --- Auto refresh --------------------------------------------------------

  function startAutoRefresh(): void {
    stopAutoRefresh()
    refreshTimer = setInterval(() => {
      void refreshNewest()
    }, refreshIntervalMs.value)
  }

  function stopAutoRefresh(): void {
    if (refreshTimer !== null) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
  }

  return {
    // filters
    rangeKey,
    customSinceMs,
    customUntilMs,
    nodeIdentityKey,
    legacyNodeKey,
    profileId,
    probeType,
    target,
    useUtc,
    filtersSignature,
    // facets
    facets,
    facetsLoading,
    facetsError,
    availableNodes,
    availableProfiles,
    availableProbeTypes,
    availableTargets,
    // data
    samples,
    nextCursor,
    hasMore,
    pagesLoaded,
    loadState,
    loadError,
    loadingOlder,
    isPartiallyLoaded,
    isEmpty,
    hasLegacySamples,
    oldestLoadedMs,
    newestLoadedMs,
    latestSample,
    lanes,
    revisionBoundaries,
    // live
    newSampleCount,
    lastRefreshAtMs,
    followLive,
    refreshIntervalMs,
    // viewport
    widthPx,
    viewport,
    domainStartMs,
    domainEndMs,
    // stats
    rangeStats,
    evidence24h,
    evidence7d,
    statsLoading,
    statsError,
    // selection
    selectedSampleId,
    selectedSample,
    selectedLaneKey,
    candidates,
    candidateIndex,
    // actions
    reload,
    loadNewestPage,
    loadOlder,
    refreshNewest,
    loadFacets,
    loadEvidence,
    setWidth,
    setRange,
    setCustomRange,
    setNodeFilter,
    setProfileFilter,
    setProbeTypeFilter,
    setTargetFilter,
    resetFilters,
    zoom,
    pan,
    panToRange,
    resetToLatest,
    maybeLoadOlder,
    selectSample,
    clearSelection,
    cycleCandidate,
    selectRelativeInLane,
    selectAdjacentLane,
    startAutoRefresh,
    stopAutoRefresh,
  }
})
