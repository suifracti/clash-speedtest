import type { WorkbenchPublicServiceAttempt, WorkbenchPublicServiceRule } from '../types'

export function serviceTitle(rule: Pick<WorkbenchPublicServiceRule, 'name' | 'service_id'>): string {
  return rule.name.replace(/ (网页与 Cloudflare 挑战|公共 API 根端点|日本网页可达性|英国网页可达性|韩国网页可达性|网页可达性|可用性|连通性|解锁分档)$/, '')
}
export function serviceEvidence(rule?: WorkbenchPublicServiceRule): string {
  if (rule?.service_id === 'antigravity') return '真实模型回答'
  return ({ connectivity: '网络检查', exit_profile: '出口观察', ip_quality: '第三方风险画像', streaming_unlock: '地区 / 片目检查', account_availability: '账号功能检查' } as Record<string, string>)[rule?.result_kind || ''] || '仅网页访问'
}
export function serviceGroup(rule: WorkbenchPublicServiceRule): string {
  if (rule.category === 'AI 服务' || /^(gemini|claude|xai)_/.test(rule.service_id)) return 'AI 助手'
  if (/^(x_web|discord|steam|reddit|nintendo|pixiv)/.test(rule.service_id)) return '社交与游戏'
  if (rule.category === '出口与 IP') return '出口与 IP'
  if (rule.category === '基础网络') return '基础网络'
  if (rule.category === '开发服务') return '开发工具'
  if (rule.result_kind === 'streaming_unlock' || /netflix|disney|prime|youtube|tver|abema|nhk|unext|u_next|hulu|fod|niconico|danime|iplayer|itv|peacock|paramount|spotify|appletv|viu|kktv|linetv|mytv|nowe|max_web|lemino|radiko|dmm_tv|line_music/i.test(rule.service_id)) return '视频与音乐'
  return '生活与地区网站'
}
export function summarizeService(attempts: WorkbenchPublicServiceAttempt[]) {
  const ordered = [...new Map(attempts.map(a => [a.attempt_id, a])).values()]
    .filter(a => a.result && !['cancelled'].includes(a.result.outcome))
    .sort((a, b) => Date.parse(a.result!.finished_at) - Date.parse(b.result!.finished_at))
  // Different rules or models are different questions, not evidence of instability.
  const latest = ordered.at(-1)
  const sameRule = ordered.filter(a => a.rule.rule_version === latest?.rule.rule_version)
  const model = [...sameRule].reverse().find(a => a.result?.model)?.result?.model
  // Early connection/account failures may not reach model discovery. Keep them
  // in the denominator, otherwise a broken connection would improve the rate.
  const samples = sameRule.filter(a => !a.result?.model || a.result.model === model)
  const passed = samples.filter(a => ['matched', 'reachable', 'profiled', 'unlocked'].includes(a.result!.outcome)).length
  let changes = 0, exitChanges = 0, exitPairs = 0
  for (let i = 1; i < samples.length; i++) {
    if (samples[i].result!.outcome !== samples[i - 1].result!.outcome) changes++
    // Missing observations break adjacency: never infer an unobserved change.
    const before = samples[i - 1].result!.details?.ip
    const after = samples[i].result!.details?.ip
    if (before && after) { exitPairs++; if (before !== after) exitChanges++ }
  }
  return { total: samples.length, passed, changes, exitChanges, exitPairs,
    rate: samples.length ? Math.round(passed / samples.length * 100) : null,
    latest: samples.at(-1), samples }
}
