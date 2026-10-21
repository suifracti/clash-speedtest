import {inspectorMedian} from '../../inspectorLatency'
import type {LatencyHistorySplit} from '../../latencyHistory'
import type {ComputedRef,InjectionKey,Ref} from 'vue'
import type {Attempt,MonitorSample,NodeOption,Project} from '../../domain'
import type {QualityEvidence,SiteQualitySummary} from '../../nodeQuality'
import type {Airport} from '../../domain'
import type {HealthTone,TrendPoint} from '../../presentation'

// HomeView keeps all state, loading and history logic; the redesigned layers
// only read it through this model so behaviour stays identical across designs.
export interface HomeModel {
 readWeekly:(refresh?:boolean)=>Promise<void>
 weeklyStats:Ref<Record<string,LatencyHistorySplit>>
 weeklyErrors:Ref<Record<string,string>>
 weeklySort:Ref<string>
 weeklyDescending:Ref<boolean>
 sortWeekly:(metric:string)=>void
 hiddenSelectedCount:ComputedRef<number>
 qualityFiltersPending:ComputedRef<boolean>
 monitorData:Ref<Record<string,MonitorSample[]>>
 monitorErrors:Ref<Record<string,string>>
 monitorHTTPErrors:Ref<Record<string,string>>
 httpPoints:(n:NodeOption,id?:string)=>TrendPoint[]
 servicePoints:(n:NodeOption,id:string)=>TrendPoint[]
 visibleServices:ComputedRef<{service_id:string;name:string}[]>
 servicePage:Ref<number>
 servicePages:ComputedRef<number>
 servicesPerPage:ComputedRef<number>
 chosen:ComputedRef<Airport[]>
 filterProfiles:ComputedRef<[string,string][]>
 protocolChoices:ComputedRef<string[]>
 protocolFilter:Ref<string>
 qualityGate:Ref<string>
 qualitySort:Ref<string>
 qualityLoading:Ref<boolean>
 qualityErrors:Ref<Record<string,string>>
 qualityCutoff:Ref<string>
 allQualityReady:ComputedRef<boolean>
 readQualityNodes:(nodes:NodeOption[],refresh?:boolean)=>Promise<void>
 siteSummaries:ComputedRef<Record<string,SiteQualitySummary[]>>
 pageSize:Ref<number>
 setPageSize:(size:number)=>void
 pageNodes:ComputedRef<NodeOption[]>
 visibleNodes:ComputedRef<NodeOption[]>
 runNodes:ComputedRef<NodeOption[]>
 currentPage:Ref<number>
 totalPages:ComputedRef<number>
 chosenNames:ComputedRef<string>
 picker:Ref<boolean>
 filterRegions:ComputedRef<string[]>
 qualityData:Ref<Record<string,QualityEvidence>>
 nodeActivities:ComputedRef<Record<string,{state:string;label:string}>>
 displayServices:ComputedRef<{service_id:string;name:string}[]>
 sourceLabel:(n:NodeOption)=>string
 metricPoint:(n:NodeOption,target:string)=>TrendPoint|undefined
 siteMetric:(n:NodeOption,target:string)=>string
 siteContext:(n:NodeOption,target:string)=>string
 siteReason:(n:NodeOption,target:string)=>string
 sitePoints:(n:NodeOption,target:string)=>TrendPoint[]
 rowPoints:(n:NodeOption,p:Project)=>TrendPoint[]
 downloadMetric:(n:NodeOption)=>string
 downloadMetricTone:(n:NodeOption)=>string
 downloadMetricContext:(n:NodeOption)=>string
 serviceMetric:(n:NodeOption,id:string)=>string
 serviceTone:(n:NodeOption,id:string)=>string
 shownService:(n:NodeOption,id:string)=>Attempt|undefined
 showService:(n:NodeOption,id:string)=>void
 hoverNode:Ref<NodeOption|null>
 hoverMode:Ref<Exclude<Project,'combined'>>
 hoverTarget:Ref<string>
 hoverService:Ref<string>
 hoverPoints:ComputedRef<TrendPoint[]>
 activeRound:Ref<TrendPoint|null>
 inlineTab:Ref<'round'|'history'>
 inlineHistorySource:ComputedRef<'manual'|'monitor'>
 keepOpen:Ref<boolean>
 inspectRound:(n:NodeOption,p:Project,point:TrendPoint,target?:string,serviceId?:string)=>void
 openRound:(n:NodeOption,p:Project,point:TrendPoint,target?:string,serviceId?:string)=>void
 leaveRound:()=>void
 keepRound:()=>void
 closeHover:()=>void
 expandHistory:()=>void
 openComparison:()=>Promise<void>
 compareOpen:Ref<boolean>
 compareNodes:ComputedRef<NodeOption[]>
 compareCompatible:ComputedRef<boolean>
 comparisonDownloadStatus:ComputedRef<string>
 closeComparison:()=>void
 openCombinedPlan:()=>void
 runLabel:(p:Project)=>string
}

export const homeModelKey:InjectionKey<HomeModel>=Symbol('homeModel')

export interface Verdict {tone:HealthTone;label:string;detail:string}

// Plain-language summary of the latest latency round across the shown sites.
// It only counts the tones the existing presentation already assigned, so it
// adds no new thresholds: success first, a nice median never hides failures.
export function latencyVerdict(points:(TrendPoint|undefined)[],stableMs?:number|null):Verdict{
 const measured=points.filter((p):p is TrendPoint=>!!p&&p.tone!=='empty')
 if(!measured.length)return {tone:'empty',label:'还没测',detail:'这个时间范围内没有延迟记录'}
 const ok=measured.filter(p=>p.tone==='good'||p.tone==='warn')
 const raw=stableMs===undefined?inspectorMedian(ok.flatMap(p=>{const v=inspectorMedian((p.samples||[]).slice(1).flatMap(s=>s.value===null?[]:[s.value]));return v===null?[]:[v]})):stableMs===null?null:Math.round(stableMs)
 const mid=raw===null?null:Math.round(raw)
 if(!ok.length)return {tone:'bad',label:'连不上',detail:`${measured.length} 个站点全部失败`}
 if(ok.length<measured.length)return {tone:'warn',label:'不稳定',detail:`${ok.length}/${measured.length} 个站点能连上${mid!==null?` · 通的约 ${mid} ms`:''}`}
 if(ok.some(p=>p.tone==='warn'))return {tone:'warn',label:'偶有失败',detail:`${ok.length} 个站点能连上，部分样本失败${mid!==null?` · 约 ${mid} ms`:''}`}
 return {tone:'good',label:'能用',detail:`${ok.length} 个站点都能连上${mid!==null?` · 约 ${mid} ms`:''}`}
}

export function toneLabel(tone:string){return ({good:'正常',warn:'部分',bad:'失败',reachable:'可达',empty:'未测'} as Record<string,string>)[tone]||tone}
