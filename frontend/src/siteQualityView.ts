import {compareSiteQuality,type SiteQualitySummary,type SiteQualityMetric} from './nodeQuality'
// Validate the entire fixed set before sorting. Incomparable pairs never become ties.
export function sortSiteRows<T extends {summary?:SiteQualitySummary}>(rows:T[],metric:SiteQualityMetric){
 const reasons=new Set<string>(),anchor=rows[0]?.summary
 for(const row of rows){
  if(!anchor||!row.summary){reasons.add('读取未完成');continue}
  const result=compareSiteQuality(anchor,row.summary,metric)
  result.reasons.forEach(reason=>reasons.add(reason))
  if(!result.comparable&&!result.reasons.length)reasons.add('不可比较')
 }
 if(reasons.size||!rows.length)return {rows,comparable:false,reasons:[...reasons]}
 return {rows:[...rows].sort((a,b)=>compareSiteQuality(a.summary!,b.summary!,metric).order!),comparable:true,reasons:[] as string[]}
}
export const siteReasonLabels:Record<string,string>={unknown_identity:'身份不完整',invalid_window:'时间窗未知',unmeasured:'未测',incomplete_read:'缺页 / 读取失败',unknown_observation_time:'采样时间未知',unknown_conditions:'测量条件未确认',insufficient_samples:'成功样本不足 5',insufficient_p95_samples:'P95 样本不足 20',stale:'超过 24 小时',ambiguous_latest_group:'最新条件不唯一',invalid_samples:'无效样本',latest_unobserved_attempt:'最近尝试未取得样本',conflicting_observations:'观察记录冲突',target_mismatch:'站点不同',window_mismatch:'时间窗不同',condition_mismatch:'URL / 方法 / 出口条件不同'}
export function siteReason(reason:string){const clean=reason.replace(/^[ab]:/,'');return siteReasonLabels[clean]||reason}
