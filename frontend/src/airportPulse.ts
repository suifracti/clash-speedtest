import type {LatencyTest} from './domain'

// Airport-level heartbeat and outages, rebuilt from saved latency tests. No
// table and no synthetic round: a round is a cluster of node tests that were
// saved within the same stretch of time. A round needs at least two nodes;
// a single-node check is an inspection, not an airport round.
export interface AirportNodeTests {key: string; name: string; tests: LatencyTest[]}
export interface AirportBeatNode {key: string; name: string; responded: boolean}
export interface AirportBeat {id: string; start: string; end: string; nodes: AirportBeatNode[]; responded: number; total: number}
export interface AirportFault {kind: 'round' | 'node'; start: string; end: string | null; nodes: number; names: string[]; detail: string}

const roundGapMs = 10 * 60000
function observedAt(test: LatencyTest) {
 const at = Date.parse(test.finished_at || test.requested_at)
 return Number.isFinite(at) ? at : null
}
// Cancelled or preflight-rejected attempts never probed the node, so they must
// not count as "no response" and manufacture an outage.
function probed(test: LatencyTest) {
 return test.status !== 'cancelled' && test.status !== 'not_executed' && (test.total_samples || 0) > 0
}

export function airportBeats(nodes: AirportNodeTests[]): AirportBeat[] {
 const events: {key: string; name: string; at: number; responded: boolean}[] = []
 for (const node of nodes) for (const test of node.tests) {
  const at = observedAt(test)
  if (at === null || !probed(test)) continue
  events.push({key: node.key, name: node.name, at, responded: (test.success_samples || 0) > 0})
 }
 events.sort((a, b) => a.at - b.at)
 const clusters: {start: number; end: number; nodes: Map<string, AirportBeatNode>}[] = []
 for (const event of events) {
  let cluster = clusters.at(-1)
  if (!cluster || event.at - cluster.end > roundGapMs) {
   cluster = {start: event.at, end: event.at, nodes: new Map()}
   clusters.push(cluster)
  }
  cluster.end = event.at
  const previous = cluster.nodes.get(event.key)
  cluster.nodes.set(event.key, {key: event.key, name: event.name, responded: !!previous?.responded || event.responded})
 }
 return clusters.filter(cluster => cluster.nodes.size >= 2).map(cluster => {
  const list = [...cluster.nodes.values()]
  return {
   id: 'beat-' + new Date(cluster.start).toISOString(),
   start: new Date(cluster.start).toISOString(),
   end: new Date(cluster.end).toISOString(),
   nodes: list,
   responded: list.filter(node => node.responded).length,
   total: list.length,
  }
 })
}

export function beatTone(beat: AirportBeat): 'good' | 'warn' | 'bad' {
 const ratio = beat.total ? beat.responded / beat.total : 0
 return ratio >= .9 ? 'good' : ratio >= .5 ? 'warn' : 'bad'
}

export function airportFaults(beats: AirportBeat[]): AirportFault[] {
 const faults: AirportFault[] = []
 let span: {start: string; end: string; names: Set<string>; worst: number; worstTotal: number} | null = null
 const close = () => {
  if (!span) return
  faults.push({kind: 'round', start: span.start, end: span.end, nodes: span.names.size, names: [...span.names], detail: `整轮 ${span.worst}/${span.worstTotal} 个节点无响应`})
  span = null
 }
 for (const beat of beats) {
  const dead = beat.nodes.filter(node => !node.responded)
  if (dead.length * 2 > beat.total) {
   if (span) {
    span.end = beat.end
    for (const node of dead) span.names.add(node.name)
    span.worst = Math.max(span.worst, dead.length)
    span.worstTotal = Math.max(span.worstTotal, beat.total)
   } else {
    span = {start: beat.start, end: beat.end, names: new Set(dead.map(node => node.name)), worst: dead.length, worstTotal: beat.total}
   }
  } else close()
 }
 close()

 const sequences = new Map<string, {name: string; states: {at: string; responded: boolean}[]}>()
 for (const beat of beats) for (const node of beat.nodes) {
  const entry = sequences.get(node.key) || {name: node.name, states: []}
  entry.states.push({at: beat.start, responded: node.responded})
  sequences.set(node.key, entry)
 }
 for (const {name, states} of sequences.values()) {
  let start = '', streak = 0
  for (const state of states) {
   if (state.responded) {
    if (streak >= 3) faults.push({kind: 'node', start, end: state.at, nodes: 1, names: [name], detail: `连续 ${streak} 轮无响应`})
    start = ''
    streak = 0
   } else {
    if (!streak) start = state.at
    streak++
   }
  }
  if (streak >= 3) faults.push({kind: 'node', start, end: null, nodes: 1, names: [name], detail: `连续 ${streak} 轮无响应，仍未恢复`})
 }
 return faults.sort((a, b) => Date.parse(b.start) - Date.parse(a.start))
}
