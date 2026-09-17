/**
 * Lane construction and ConfigRevision boundary detection for the monitor timeline.
 *
 * A lane is one (node identity, probe type, target) triple. Keeping them separate is what lets
 * a user tell "the node's transport is down" apart from "only one service is unreachable".
 */

import type { MonitorSample } from '../../types'

export interface TimelineLane {
  key: string
  nodeIdentityKey: string
  legacyNodeKey: string
  displayName: string
  probeType: string
  target: string
  /** Human readable lane label, e.g. "JP01 · rtt · cp.cloudflare.com". */
  label: string
  /** Samples in ascending time order, tie-broken by sample id for determinism. */
  samples: MonitorSample[]
}

export function laneKeyFor(sample: MonitorSample): string {
  const identity = sample.nodeIdentityKey || sample.nodeKey || 'unknown-node'
  return `${identity}\u0000${sample.probeType}\u0000${sample.target}`
}

/** Deterministic ascending order: timestamp, then sample id. */
export function compareSamplesAsc(a: MonitorSample, b: MonitorSample): number {
  if (a.timestampMs !== b.timestampMs) return a.timestampMs - b.timestampMs
  return a.sampleId < b.sampleId ? -1 : a.sampleId > b.sampleId ? 1 : 0
}

/** Deterministic descending order: newest first, then sample id descending. */
export function compareSamplesDesc(a: MonitorSample, b: MonitorSample): number {
  return -compareSamplesAsc(a, b)
}

function shortenTarget(target: string): string {
  if (!target) return '—'
  try {
    const url = new URL(target)
    const path = url.pathname === '/' ? '' : url.pathname
    return `${url.host}${path}`
  } catch {
    return target
  }
}

/**
 * Groups samples into lanes.
 *
 * Lane order is deterministic (node, probe type, target) so the layout does not shuffle between
 * refreshes. Samples inside a lane are sorted ascending by time.
 */
export function buildLanes(samples: MonitorSample[]): TimelineLane[] {
  const map = new Map<string, TimelineLane>()

  for (const sample of samples) {
    const key = laneKeyFor(sample)
    let lane = map.get(key)
    if (!lane) {
      const identity = sample.nodeIdentityKey || sample.nodeKey || 'unknown-node'
      lane = {
        key,
        nodeIdentityKey: identity,
        legacyNodeKey: sample.nodeKey,
        displayName: sample.displayNameSnapshot || sample.nodeKey || identity,
        probeType: sample.probeType,
        target: sample.target,
        label: `${sample.displayNameSnapshot || identity} · ${sample.probeType || 'probe'} · ${shortenTarget(
          sample.target
        )}`,
        samples: [],
      }
      map.set(key, lane)
    }
    lane.samples.push(sample)
  }

  const lanes = Array.from(map.values())
  for (const lane of lanes) lane.samples.sort(compareSamplesAsc)
  lanes.sort((a, b) => {
    if (a.displayName !== b.displayName) return a.displayName < b.displayName ? -1 : 1
    if (a.probeType !== b.probeType) return a.probeType < b.probeType ? -1 : 1
    if (a.target !== b.target) return a.target < b.target ? -1 : 1
    return a.key < b.key ? -1 : a.key > b.key ? 1 : 0
  })

  return lanes
}

export interface RevisionBoundary {
  nodeIdentityKey: string
  atMs: number
  fromRevision: string
  toRevision: string
}

/**
 * Picks the revision in effect at one instant for a single node.
 *
 * Multiple probes sample the same node at the same instant; if they disagree (a revision
 * change landing mid-round) the most frequent value wins, tie-broken lexicographically so the
 * result never depends on array order.
 */
function revisionAtInstant(group: MonitorSample[]): string {
  const counts = new Map<string, number>()
  for (const s of group) {
    const rev = s.configRevisionKey || ''
    counts.set(rev, (counts.get(rev) ?? 0) + 1)
  }
  let best = ''
  let bestCount = -1
  for (const [rev, count] of Array.from(counts.entries()).sort((a, b) =>
    a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0
  )) {
    if (count > bestCount) {
      best = rev
      bestCount = count
    }
  }
  return best
}

/**
 * Detects ConfigRevision boundaries per node identity.
 *
 * Only non-empty revision keys participate: a legacy row without a revision must not fabricate a
 * boundary. The boundary is reported at the timestamp of the first sample observed under the new
 * revision.
 */
export function detectRevisionBoundaries(samples: MonitorSample[]): RevisionBoundary[] {
  const byNode = new Map<string, MonitorSample[]>()
  for (const sample of samples) {
    const identity = sample.nodeIdentityKey || sample.nodeKey
    if (!identity) continue
    const list = byNode.get(identity)
    if (list) list.push(sample)
    else byNode.set(identity, [sample])
  }

  const boundaries: RevisionBoundary[] = []

  for (const [identity, list] of byNode) {
    const byInstant = new Map<number, MonitorSample[]>()
    for (const sample of list) {
      const group = byInstant.get(sample.timestampMs)
      if (group) group.push(sample)
      else byInstant.set(sample.timestampMs, [sample])
    }

    const instants = Array.from(byInstant.keys()).sort((a, b) => a - b)
    let current: string | null = null

    for (const atMs of instants) {
      const rev = revisionAtInstant(byInstant.get(atMs)!)
      if (!rev) continue
      if (current === null) {
        current = rev
        continue
      }
      if (rev !== current) {
        boundaries.push({
          nodeIdentityKey: identity,
          atMs,
          fromRevision: current,
          toRevision: rev,
        })
        current = rev
      }
    }
  }

  boundaries.sort((a, b) => (a.atMs !== b.atMs ? a.atMs - b.atMs : a.nodeIdentityKey < b.nodeIdentityKey ? -1 : 1))
  return boundaries
}

/** Shortens an opaque key for display while keeping it recognisable. */
export function shortenKey(key: string, head = 6, tail = 4): string {
  if (!key) return '—'
  if (key.length <= head + tail + 1) return key
  return `${key.slice(0, head)}…${key.slice(-tail)}`
}

/**
 * True when a sample looks like a PR#3 legacy row backfilled by the migration
 * (node_identity_key was seeded from node_key). Used only to surface an informational notice.
 */
export function isLegacyBackfilledSample(sample: MonitorSample): boolean {
  return !!sample.nodeIdentityKey && sample.nodeIdentityKey === sample.nodeKey
}
