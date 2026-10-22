import {date} from './domain'
import {inspectorMedian} from './inspectorLatency'

// One saved latency round, already reduced to what the history views need.
// The inspector builds these with latencyRounds(); the airport page builds
// them from saved tests. Presentation only: nothing here writes history.
export interface PulseRound {id:string;time:string;responses:number;total:number;stable:number|null;oldConfig:boolean}
export type PulseTone = 'good' | 'warn' | 'bad'

export function pulseTone(round: PulseRound): PulseTone {
 if (!round.total || round.responses <= 0) return 'bad'
 return round.responses >= round.total ? 'good' : 'warn'
}

export function pulseTitle(round: PulseRound) {
 const stable = round.stable === null ? '稳定延迟 —' : '稳定延迟 ' + Math.round(round.stable) + ' ms'
 return date(round.time) + ' · ' + stable + ' · ' + round.responses + '/' + round.total + ' 次响应' + (round.oldConfig ? ' · 旧配置' : '')
}

// The three windows are fixed, independent of the history range selector: a
// 24-hour rate must not shrink just because the chart is showing 24 hours.
const rateWindows = [{hours: 24, label: '24 小时'}, {hours: 168, label: '7 天'}, {hours: 720, label: '30 天'}]
export function responseWindows(rounds: PulseRound[], now = Date.now()) {
 return rateWindows.map(({hours, label}) => {
  const start = now - hours * 3600000
  const inWindow = rounds.filter(round => {
   const at = Date.parse(round.time)
   return Number.isFinite(at) && at >= start && at <= now
  })
  const responses = inWindow.reduce((n, round) => n + round.responses, 0)
  const samples = inWindow.reduce((n, round) => n + round.total, 0)
  return {hours, label, rounds: inWindow.length, responses, samples, rate: samples ? responses / samples * 100 : null}
 })
}

export function heartbeatRounds(rounds: PulseRound[], limit = 50) {
 return rounds.slice(-limit)
}

// A time-of-day cell is only meaningful with at least two rounds. Anything
// thinner stays blank; a missing number is not a zero.
export interface HourBucket {hour: number; rounds: number; median: number | null; failureRate: number | null}
export function hourBuckets(rounds: PulseRound[], minRounds = 2): HourBucket[] {
 const buckets = Array.from({length: 24}, (_, hour) => ({hour, rounds: 0, values: [] as number[], failed: 0, samples: 0}))
 for (const round of rounds) {
  const at = Date.parse(round.time)
  if (!Number.isFinite(at)) continue
  const bucket = buckets[new Date(at).getHours()]
  bucket.rounds++
  bucket.failed += Math.max(0, round.total - round.responses)
  bucket.samples += round.total
  if (round.stable !== null) bucket.values.push(round.stable)
 }
 return buckets.map(bucket => ({
  hour: bucket.hour,
  rounds: bucket.rounds,
  median: bucket.rounds >= minRounds ? inspectorMedian(bucket.values) : null,
  failureRate: bucket.rounds >= minRounds && bucket.samples ? bucket.failed / bucket.samples * 100 : null,
 }))
}

export function hourTitle(bucket: HourBucket) {
 if (bucket.median === null) return `${bucket.hour} 点 · ${bucket.rounds} 轮，不足 2 轮，不显示数字`
 const failure = bucket.failureRate === null ? '失败率 —' : '失败率 ' + bucket.failureRate.toFixed(0) + '%'
 return `${bucket.hour} 点 · ${bucket.rounds} 轮 · 稳定延迟 ${Math.round(bucket.median)} ms · ${failure}`
}

// Rank the hours that actually have enough rounds for a median. One bucket is
// not a ranking, so the line stays empty until two hours qualify.
export function slowestHours(buckets: HourBucket[], limit = 2) {
 const ranked = buckets.filter(bucket => bucket.median !== null).sort((a, b) => (b.median as number) - (a.median as number))
 if (ranked.length < limit) return ''
 const parts = ranked.slice(0, limit).map(bucket => `${bucket.hour} 点 ${Math.round(bucket.median as number).toLocaleString()} ms`)
 return '最慢：' + parts.join(' · ')
}

const blocks = [0, 4, 8, 12, 16, 20]
function blockLabel(start: number) {
 const period = start < 6 ? '凌晨' : start < 9 ? '早上' : start < 12 ? '上午' : start < 14 ? '中午' : start < 18 ? '下午' : '晚'
 return `${period} ${start}–${start + 3} 点`
}
// Fixed four-hour blocks keep the sentence stable: "20–23 点" always means the
// same hours, instead of a sliding window that moves with the data.
export function hourConclusion(rounds: PulseRound[]) {
 const timed = rounds.flatMap(round => {
  const at = Date.parse(round.time)
  return Number.isFinite(at) && round.stable !== null ? [{hour: new Date(at).getHours(), value: round.stable}] : []
 })
 if (timed.length < 6) return ''
 let best = {start: 0, ratio: 0}
 for (const start of blocks) {
  const inside = timed.filter(x => x.hour >= start && x.hour < start + 4).map(x => x.value)
  const outside = timed.filter(x => !(x.hour >= start && x.hour < start + 4)).map(x => x.value)
  if (inside.length < 3 || outside.length < 3) continue
  const slow = inspectorMedian(inside) as number, rest = inspectorMedian(outside) as number
  if (!rest) continue
  const ratio = slow / rest
  if (ratio > best.ratio) best = {start, ratio}
 }
 if (best.ratio < 1.5) return ''
 return `${blockLabel(best.start)}稳定延迟约为其他时段的 ${best.ratio.toFixed(1)} 倍`
}
