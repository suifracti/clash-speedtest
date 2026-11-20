<script setup lang="ts">
import {exitLanding} from '../exitLanding'
import {inspectorSamples,latencySiteSamples,latencyCellValue,stableLatency} from '../inspectorLatency'
import {readLatencyHistoryWithConfig,latencyHistorySplit} from '../latencyHistory'
import {sortSiteRows} from '../siteQualityView'
import {latestServices,readQuality,multiSiteQuality,qualityGroups,downloadEvidence,latencyReadComplete,qualityMatchesGate,type QualityEvidence,type SiteQualityMetric} from '../nodeQuality'
import {mapLimited} from '../api'
import {latencyTargetId} from '../latencyTargets'
import {serviceAvailability,downloadGroup,servicePassed} from '../sharedResults'
import {attemptTone} from '../measurementRounds'
import {computed,nextTick,onBeforeUnmount,onMounted,provide,ref,watch} from 'vue'
import HomeWorkbench from './home/HomeWorkbench.vue'
import {homeModelKey,latencyVerdict,weeklyHistoryKey,weeklySummary,type WeeklySummary} from './home/homeModel'
import {useWorkspace} from '../workspace'
import {api} from '../api'
import {date,isReputationRule,sameHistoryRecords,downloadReading,downloadPeakSpeed,downloadTone,downloadOutcome,downloadConnectionSeconds,downloadSourceName,key,outcomeLabel,sampleFor,targets,type NodeOption,type Project,type Attempt} from '../domain'
import {airportNodeCount as countAirportNodes} from '../airportViewing'
import {attemptStatus,downloadNoDataReason,triggerLabel} from '../measurementRounds'
import {latencyPointLabel,createPresentationCache,manualLatencyPoint,monitorLatencyPoints,monitorHTTPServices,type TrendPoint} from '../presentation'
import {unlockGroups,unlockGroupSummary} from '../unlock'
import Modal from './Modal.vue'
import MeasurementResults from './MeasurementResults.vue'
import {nodeDisplayName,nodeRegion} from '../nodePresentation'
const w=useWorkspace(),inlineTab=ref<'round'|'history'>('round'),keepOpen=ref(false),picker=ref(false),currentPage=ref(1),hoverNode=ref<NodeOption|null>(null),activeRound=ref<TrendPoint|null>(null),hoverMode=ref<Exclude<Project,'combined'>>('latency'),monitorErrors=ref<Record<string,string>>({}),monitorData=ref<Record<string,import('../domain').MonitorSample[]>>({}),monitorHTTPData=ref<Record<string,import('../domain').MonitorSample[]>>({}),monitorHTTPErrors=ref<Record<string,string>>({})
const serviceDetail=ref<Attempt|null>(null)
const serviceDetailPoint=computed<TrendPoint|null>(()=>serviceDetail.value?{id:serviceDetail.value.attempt_id,time:serviceDetail.value.finished_at||serviceDetail.value.requested_at,value:null,tone:attemptTone(serviceDetail.value),description:'已保存服务证据 · 端点响应不代表登录、播放或聊天可用',attempts:[serviceDetail.value],source:'manual',trigger:serviceDetail.value.trigger_type as TrendPoint['trigger']}:null)
function shownService(n:NodeOption,id:string){const saved=latestServices([...(qualityData.value[key(n)]?.services||[]),...(w.services.value[key(n)]||[])]).find(a=>(a.service_id||a.rule.service_id)===id);return w.sort.value==='service'?w.summaryService(n,id)||saved:saved}
function showServiceEvidence(attempt:Attempt){serviceDetail.value=attempt}
function showService(n:NodeOption,id:string){const a=shownService(n,id);if(a)serviceDetail.value=a;else w.inspectNode.value=n}
const nodeActivities=computed(()=>{const result:Record<string,{state:string;label:string}>={};for(const q of w.queue.value){const k=key(q.node),state=w.pendingStart.value&&(q.id===w.pendingStart.value.requestId||w.pendingStart.value.kind==='latency'&&q.project==='latency'&&q.state==='running'&&!q.batchId)?'unknown':q.attempt?.persistence_state==='saving'?'saving':q.attempt?.persistence_state==='failed'?'save_failed':q.state;const rank=(s:string)=>({running:6,cancelling:7,saving:5,queued:4,save_failed:3,failed:2,completed:1}[s]||0);if(!result[k]||rank(state)>rank(result[k].state))result[k]={state,label:({unknown:'启动待确认',running:'检测中',cancelling:'正在取消',saving:'结果已到 · 保存中',queued:'等待检测',save_failed:'保存失败 · 结果保留',failed:'检测已结束 · 查看结果',completed:'检测已结束',cancelled:'已取消',not_executed:'未执行'} as Record<string,string>)[state]||state}}return result})
const qualityView=ref(true),compareOpen=ref(false),compareTarget=ref(w.displayTarget.value),qualityGate=ref('all'),qualitySort=ref('existing'),protocolFilter=ref('all')
const qualityData=ref<Record<string,QualityEvidence>>({}),qualityErrors=ref<Record<string,string>>({}),qualityLoading=ref(false),qualityCutoff=ref(new Date().toISOString())
let qualityGeneration=0
const siteTarget=ref('cloudflare'),siteSort=ref<SiteQualityMetric|'existing'>('existing'),siteGate=ref('all')
const siteSummaries=computed(()=>Object.fromEntries(w.proxyNodes.value.flatMap(n=>{const e=qualityData.value[key(n)];return e?[[key(n),multiSiteQuality(n,e)]]:[]})))
const selectedSite=(n:NodeOption)=>siteSummaries.value[key(n)]?.find(s=>s.targetId===siteTarget.value)
const qualityInflight=new Map<string,Promise<void>>()
const qualityCompareNodes=computed(()=>w.selectedNodes.value)
async function readQualityNodes(nodes:NodeOption[],refresh=false){
 if(refresh){qualityGeneration++;qualityData.value={};qualityErrors.value={};qualityCutoff.value=new Date().toISOString()}
 const active=qualityGeneration,until=qualityCutoff.value,since=new Date(Date.parse(until)-w.hours.value*3600000).toISOString();qualityLoading.value=true
 await mapLimited(nodes.filter(n=>!qualityData.value[key(n)]),4,async n=>{const id=active+'|'+key(n);if(qualityInflight.has(id))return qualityInflight.get(id);const task=(async()=>{try{const e=await readQuality(n,since,until);if(active===qualityGeneration){qualityData.value[key(n)]=e;w.landingEvidence.value[key(n)]=e.services;delete qualityErrors.value[key(n)]}}catch(e){if(active===qualityGeneration)qualityErrors.value[key(n)]=e instanceof Error?e.message:String(e)}finally{qualityInflight.delete(id)}})();qualityInflight.set(id,task);await task})
 if(active===qualityGeneration)qualityLoading.value=false
}
// Only persisted terminal results invalidate the saved-history snapshot. Defer
// while a plan/recovered task is active so its frozen comparison window stays put.
const savedQualityVersion=computed(()=>JSON.stringify([
 ...Object.values(w.latency.value).flat().filter(t=>t.persistence_state==='saved').map(t=>['latency',key(t),t.attempt_id,t.finished_at]),
 ...(['download','service'] as const).flatMap(kind=>Object.values((kind==='download'?w.downloads:w.services).value).flat().filter(a=>a.persistence_state==='saved'&&!['queued','running','cancelling','saving'].includes(a.execution_state)).map(a=>[kind,key(a),a.attempt_id,a.finished_at||a.result?.finished_at||a.requested_at]))
].sort((a,b)=>JSON.stringify(a).localeCompare(JSON.stringify(b)))))
let appliedQualityVersion=savedQualityVersion.value
watch([savedQualityVersion,()=>w.busy.value],([version,busy])=>{
 if(busy||version===appliedQualityVersion)return
 appliedQualityVersion=version
 qualityGeneration++;qualityData.value={};qualityErrors.value={};qualityLoading.value=false;qualityCutoff.value=new Date().toISOString()
 if(w.page.value!=='home')return
 const quick=qualityView.value,full=metricHeader.value||(quick?(qualityGate.value!=='all'&&qualityGate.value!=='response'||qualitySort.value!=='existing'):(siteSort.value!=='existing'||siteGate.value!=='all'))
 if(quick||full||compareOpen.value)void readQualityNodes(full?w.proxyNodes.value:[...new Map([...pageNodes.value,...(compareOpen.value?qualityCompareNodes.value:[])].map(n=>[key(n),n])).values()])
})
const compareCompatible=computed(()=>{const g=qualityCompareNodes.value.map(n=>{const e=qualityData.value[key(n)];const g=e?qualityGroups(e.tests,e.monitor,compareTarget.value)[0]:undefined;return e&&latencyReadComplete(e,g)?g:undefined});return g.length>1&&g.every(v=>v?.comparable&&!v.insufficient&&v.key===g[0]?.key)})
const comparisonDownloadStatus=computed(()=>{
 const evidence=qualityCompareNodes.value.map(n=>qualityData.value[key(n)])
 if(evidence.some(e=>!e||!e.reads.download.complete))return '下载不可比较：读取尚未完整。'
 const attempts=evidence.map(e=>downloadEvidence(e.downloads).valid)
 if(attempts.some(a=>!a))return '下载不可比较：未测或无有效速度。'
 const groups=attempts.map(a=>downloadGroup(a))
 if(groups.some(g=>g.key!==groups[0]?.key))return '下载不可比较：条件或结束类型不同。'
 if(attempts.some(a=>!a!.rule.method||a!.rule.rule_version===undefined||a!.rule.maximum_bytes===undefined||a!.rule.maximum_duration_ns===undefined||!a!.rule.network_path_method||!a!.rule.physical_interface||!a!.rule.dns_mode||!a!.node_type||a!.result?.network_path?.method!==a!.rule.network_path_method||a!.result?.network_path?.interface!==a!.rule.physical_interface||a!.result?.network_path?.dns_mode!==a!.rule.dns_mode||!a!.result?.network_path?.socket_bind_verified||!a!.result?.network_path?.dns_bind_verified||!a!.result?.network_path?.address_family||!a!.result?.network_path?.address_source))return '下载不可比较：方法、上限或出口证据不足。'
 return '历史有效速度条件一致；最近检测状态仍单独核对。'
})
// 比较使用独立对照表；退出时恢复原来的滚动位置，查看中的节点不受影响。
let compareScroll=0
async function openComparison(){compareTarget.value=w.displayTarget.value;compareScroll=window.scrollY;compareOpen.value=true;await readQualityNodes(qualityCompareNodes.value)}
function closeComparison(){if(!compareOpen.value)return;compareOpen.value=false;void nextTick(()=>window.scrollTo({top:compareScroll}))}
watch(()=>qualityCompareNodes.value.length,n=>{if(n<2)closeComparison()})
const presentation=createPresentationCache(w,monitorData,monitorHTTPData)
function savedFollowControls(){try{return window.localStorage.getItem('speedtest-follow-controls')==='true'}catch{return false}}
const followControls=ref(savedFollowControls())
watch(followControls,value=>{try{window.localStorage.setItem('speedtest-follow-controls',String(value))}catch{}})
let monitorGeneration=0,hoverTimer:ReturnType<typeof setTimeout>|undefined,monitorTimer:ReturnType<typeof setInterval>|undefined
const qualitySummaries=computed(()=>Object.fromEntries(Object.entries(qualityData.value).map(([id,e])=>[id,qualityGroups(e.tests,e.monitor,w.displayTarget.value)[0]])))
const allQualityReady=computed(()=>w.proxyNodes.value.every(n=>qualityData.value[key(n)]||qualityErrors.value[key(n)]))
const qualityScopeKey=computed(()=>JSON.stringify(w.proxyNodes.value.map(key).sort()))
const siteRows=computed(()=>{
 let list=w.filteredNodes.value
 if(allQualityReady.value&&siteGate.value!=='all')list=list.filter(n=>{const s=selectedSite(n);return !!s&&(siteGate.value==='rankable'?s.rankable[siteSort.value==='existing'?'p50':siteSort.value]:siteGate.value==='fresh'?s.gates.fresh:siteGate.value==='response'?s.gates.response:s.gates.samples)})
 return list.map(node=>({node,summary:selectedSite(node)}))
})
const siteSortResult=computed(()=>sortSiteRows(siteRows.value,siteSort.value==='existing'?'p50':siteSort.value))
const weeklyStats=ref<Record<string,WeeklySummary>>({}),weeklyErrors=ref<Record<string,string>>({})
const headerSort=ref(''),weeklyUntil=ref(new Date().toISOString())
const weeklyColumn=computed(()=>['responseRate','volatility'].includes(headerSort.value))
const metricHeader=computed(()=>!!headerSort.value&&headerSort.value!=='name'&&!weeklyColumn.value)
const usesWeeklyHistory=computed(()=>['latency','combined'].includes(w.project.value)||qualityGate.value==='response')
const weeklyFullScope=computed(()=>qualityGate.value==='response'||weeklyColumn.value)
function weeklyRead(id:string){return !!(weeklyStats.value[id]||weeklyErrors.value[id])}
const weeklyReady=computed(()=>w.proxyNodes.value.every(n=>weeklyRead(weeklyHistoryKey(n))))
const headerSortReady=computed(()=>headerSort.value==='name'||(weeklyColumn.value?weeklyReady.value:allQualityReady.value))
function sortHeader(column:string){
 headerSort.value=headerSort.value===column?'':column
 currentPage.value=1;closeHover()
 if(headerSort.value&&headerSort.value!=='name'&&!weeklyColumn.value)void readQualityNodes(w.proxyNodes.value)
}
watch([()=>w.sort.value,()=>w.project.value],()=>headerSort.value='')
watch(()=>w.displayTargets.value,ids=>{if(headerSort.value.startsWith('site:')&&!ids.includes(headerSort.value.slice(5)))headerSort.value=''})
const baseVisibleNodes=computed(()=>{let list=w.filteredNodes.value;if(!qualityView.value)return (siteSort.value!=='existing'&&allQualityReady.value?siteSortResult.value.rows:siteRows.value).map(r=>r.node);list=list.filter(n=>protocolFilter.value==='all'||n.type===protocolFilter.value)
 if(qualityGate.value==='response'&&weeklyReady.value)list=list.filter(n=>(weeklyStats.value[weeklyHistoryKey(n)]?.all.responseRate??-1)>=90)
 else if(qualityGate.value!=='all'&&qualityGate.value!=='response'&&allQualityReady.value)list=list.filter(n=>qualityMatchesGate(qualityData.value[key(n)],qualitySummaries.value[key(n)],qualityGate.value,qualityCutoff.value))
 if(qualitySort.value!=='existing'&&allQualityReady.value)list=[...list].sort((a,b)=>{const ga=qualitySummaries.value[key(a)],gb=qualitySummaries.value[key(b)],ea=qualityData.value[key(a)],eb=qualityData.value[key(b)];const valid=(g:typeof ga,e:QualityEvidence|undefined)=>!!g&&!!e&&latencyReadComplete(e,g)&&g.comparable&&(qualitySort.value==='observed'?g.success>=1:!g.insufficient)&&(qualitySort.value!=='p95'||g.success>=20);const av=valid(ga,ea),bv=valid(gb,eb);if(av!==bv)return av?-1:1;if(!av)return 0;const group=ga.key.localeCompare(gb.key);if(group)return group;return qualitySort.value==='rate'?(gb.successRate||0)-(ga.successRate||0):(qualitySort.value==='p95'?ga.p95! -gb.p95!:ga.p50! -gb.p50!)})
 return list})
const names=new Intl.Collator('zh-CN',{numeric:true,sensitivity:'base'})
const visibleNodes=computed(()=>{
 const list=baseVisibleNodes.value,field=headerSort.value
 if(!field||!headerSortReady.value)return list
 if(field==='name')return [...list].sort((a,b)=>names.compare(nodeDisplayName(a),nodeDisplayName(b)))
 const values=new Map(list.map(node=>[key(node),headerValue(node,field)]))
 return [...list].sort((a,b)=>{
  const first=values.get(key(a)),second=values.get(key(b))
  return first==null?(second==null?0:1):second==null?-1:first-second
 })
})
function headerValue(node:NodeOption,field:string):number|null{
 const evidence=qualityData.value[key(node)]
 if(field==='responseRate'||field==='volatility')return weeklyStats.value[weeklyHistoryKey(node)]?.all[field]??null
 if(field.startsWith('site:')||field==='verdict'){
  if(evidence&&(!evidence.reads.latency.complete||!evidence.reads.monitor.complete))return null
  if(field==='verdict')return ({bad:0,warn:1,good:2,empty:null} as const)[rowVerdict(node).tone]??null
  const target=field.slice(5),detailed=w.project.value==='latency'
  return latencyCellValue(detailed?latestLatencyRound(node):metricPoint(node,target),target,detailed)
 }
 if(field==='download'||field==='downloadTime'){
  if(evidence&&!evidence.reads.download.complete)return null
  const attempt=tableDownload(node)
  return field==='download'?downloadReading(attempt)?.value??null:Date.parse(attempt?.finished_at||attempt?.result?.finished_at||'')||null
 }
 if(evidence&&!evidence.reads.service.complete)return null
 const landing=exitLanding(node,[...(evidence?.services||[]),...(w.services.value[key(node)]||[])])
 if(field==='purity')return landing.purity.score
 if(field==='cfChallenge')return landing.cfChallenge.rank
 const groups=field==='service'?unlockGroups:unlockGroups.filter(g=>g.id===field.slice(7))
 const catalog=w.catalog.value.filter(rule=>!isReputationRule(rule))
 const results=groups.map(g=>unlockGroupSummary(g.id,catalog,id=>shownService(node,id),w.nodeUnlockRegion(node),w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions,landing.ipKind.type))
 const total=results.reduce((sum,r)=>sum+r.total,0)
 return total?results.reduce((sum,r)=>sum+r.available,0)/total:null
}
watch([qualityGate,qualitySort],()=>{currentPage.value=1;if(qualityGate.value==='download'&&w.project.value!=='combined')w.project.value='download';else if(qualityGate.value==='service'&&w.project.value!=='combined')w.project.value='service';if(qualityGate.value!=='all'&&qualityGate.value!=='response'||qualitySort.value!=='existing')void readQualityNodes(w.proxyNodes.value)})
watch(protocolFilter,()=>currentPage.value=1)
watch([siteTarget,siteSort,siteGate],()=>{currentPage.value=1;closeHover();if(siteSort.value!=='existing'||siteGate.value!=='all')void readQualityNodes(w.proxyNodes.value)})
watch(()=>w.region.value,r=>{if(r==='all')w.excludeRegion.value=false;currentPage.value=1})
watch(()=>w.excludeRegion.value,()=>currentPage.value=1)
const pageSize=ref(20)
function setPageSize(size:number){pageSize.value=size;currentPage.value=1;try{window.localStorage.setItem('speedtest-page-size',String(size))}catch{}}
try{const raw=window.localStorage?.getItem('speedtest-page-size');if(raw!==null&&raw!==''){const s=Number(raw);if([10,20,50,100,0].includes(s))pageSize.value=s}}catch{}
const chosen=computed(()=>w.airports.value.filter(a=>w.selectedAirportIds.value?.includes(a.id))),totalPages=computed(()=>pageSize.value<=0?1:Math.max(1,Math.ceil(visibleNodes.value.length/pageSize.value))),pageNodes=computed(()=>pageSize.value<=0?visibleNodes.value.slice():visibleNodes.value.slice((currentPage.value-1)*pageSize.value,currentPage.value*pageSize.value))

const weeklyDemand=computed(()=>{
 const nodes=w.page.value!=='home'||!usesWeeklyHistory.value?[]:weeklyFullScope.value?w.proxyNodes.value:pageNodes.value
 return new Map(nodes.map(node=>[weeklyHistoryKey(node),node]))
})
const weeklyProgress=computed(()=>{
 let processed=0,failed=0
 for(const id of weeklyDemand.value.keys()){
  if(weeklyRead(id))processed++
  if(weeklyErrors.value[id])failed++
 }
 return {full:weeklyFullScope.value,total:weeklyDemand.value.size,processed,failed,until:weeklyUntil.value}
})
let weeklyGeneration=0
let weeklyWaiting=new Map<string,NodeOption>()
const weeklyInFlight=new Map<string,number>()
function fillWeeklySlots(){
 for(const [id,node] of weeklyWaiting){
  if(weeklyInFlight.size>=6)break
  if(!weeklyDemand.value.has(id)){weeklyWaiting.delete(id);continue}
  if(weeklyInFlight.has(id))continue
  weeklyWaiting.delete(id)
  const generation=weeklyGeneration
  weeklyInFlight.set(id,generation)
  void readLatencyHistoryWithConfig(node,168,weeklyUntil.value).then(history=>{
   if(generation===weeklyGeneration)weeklyStats.value[id]=weeklySummary(latencyHistorySplit(history.series))
  }).catch(error=>{
   if(generation===weeklyGeneration)weeklyErrors.value[id]=(error instanceof Error?error.message:String(error??''))||'读取失败'
  }).finally(()=>{weeklyInFlight.delete(id);fillWeeklySlots()})
 }
}
function scheduleWeeklyReads(demand:Map<string,NodeOption>){
 weeklyWaiting=new Map([...demand].filter(([id])=>!weeklyRead(id)&&weeklyInFlight.get(id)!==weeklyGeneration))
 fillWeeklySlots()
}
function refreshWeekly(){
 weeklyGeneration++
 weeklyUntil.value=new Date().toISOString()
 weeklyStats.value={}
 weeklyErrors.value={}
 scheduleWeeklyReads(weeklyDemand.value)
}
watch(weeklyDemand,scheduleWeeklyReads,{immediate:true,flush:'post'})
watch(()=>w.busy.value,(busy,previous)=>{if(previous&&!busy)refreshWeekly()})

const hoverTarget=ref(w.displayTarget.value),hoverService=ref(''),servicePage=ref(1),servicesPerPage=computed(()=>w.project.value==='service'?12:3)
const inlineHistorySource=computed(()=>activeRound.value?.source||(hoverNode.value&&(hoverMode.value==='latency'&&presentation.monitor(hoverNode.value,hoverTarget.value).length&&!presentation.points(hoverNode.value,'latency',hoverTarget.value).some(p=>p.tone!=='empty')||hoverMode.value==='service'&&httpPoints(hoverNode.value).length&&!presentation.services(hoverNode.value,visibleServiceIds.value).length)?'monitor':'manual'))
const hoverPoints=computed(()=>hoverNode.value?inlineHistorySource.value==='monitor'?(hoverMode.value==='service'?httpPoints(hoverNode.value,hoverService.value):monitorPoints(hoverNode.value)):hoverMode.value==='service'&&!hoverService.value?presentation.services(hoverNode.value):presentation.points(hoverNode.value,hoverMode.value,hoverTarget.value,hoverService.value):[])
const runNodes=computed(()=>w.selectedNodes.value.length?w.selectedNodes.value:visibleNodes.value)
const airportNodes=computed(()=>w.proxyNodes.value.filter(n=>w.airportFilter.value==='all'||w.nodeAirportId(n)===w.airportFilter.value))
const hiddenSelectedCount=computed(()=>w.selectedNodes.value.filter(n=>!visibleNodes.value.some(v=>key(v)===key(n))).length)
const qualityFiltersPending=computed(()=>qualityGate.value==='response'?!weeklyReady.value:(qualityGate.value!=='all'&&qualityGate.value!=='response'||qualitySort.value!=='existing')&&!allQualityReady.value)
const filterRegions=computed(()=>[...new Set(airportNodes.value.map(nodeRegion))].sort())
const filterProfiles=computed(()=>[...new Map(airportNodes.value.map(n=>[n.profile_id,sourceLabel(n)])).entries()])
const protocolChoices=computed(()=>[...new Set(w.proxyNodes.value.map(n=>n.type))].sort())
const displayServices=computed(()=>w.serviceIds.value.map(id=>w.catalog.value.find(s=>s.service_id===id&&!s.superseded_by&&!isReputationRule(s))).filter((s):s is NonNullable<typeof s>=>!!s))
const servicePages=computed(()=>Math.max(1,Math.ceil(displayServices.value.length/servicesPerPage.value)))
const visibleServices=computed(()=>displayServices.value.slice((servicePage.value-1)*servicesPerPage.value,servicePage.value*servicesPerPage.value))
const visibleServiceIds=computed(()=>visibleServices.value.map(s=>s.service_id))
function sourceLabel(n:NodeOption){const airport=w.nodeAirportName(n),sub=w.nodeSubscriptionName(n);return sub&&sub!==airport?`${airport} · ${sub}`:airport}
function airportNodeCount(id:string){return countAirportNodes(w.airports.value.find(a=>a.id===id),w.nodes.value)}
function serviceMetric(n:NodeOption,id:string){const a=shownService(n,id);if(!a)return '未测';if(!a.result)return attemptStatus(a);const o=a.result.outcome;if(o==='flapping')return serviceAvailability(a)===0?'检测结论不一致':'未确认';if(['matched','unlocked'].includes(o)&&!servicePassed(a))return '未确认';return outcomeLabel(o)}
function serviceTone(n:NodeOption,id:string){const a=shownService(n,id);return a?.result?.outcome==='reachable'?'reachable':a?attemptTone(a):'empty'}
function hoveredDownloadPoint(n:NodeOption){return hoverMode.value==='download'&&hoverNode.value&&key(hoverNode.value)===key(n)?activeRound.value:null}
function tableDownload(n:NodeOption){
 const displayed=w.summaryDownload(n)
 if(!['download','downloadTime'].includes(headerSort.value))return displayed
 return downloadEvidence([...(qualityData.value[key(n)]?.downloads||[]),...(displayed?[displayed]:[])]).latest
}
function downloadMetric(n:NodeOption){
 const point=hoveredDownloadPoint(n),a=point?.attempts?.[0]||tableDownload(n)
 const reading=downloadReading(a),value=point?point.value:reading?.value??null
 if(a?.result?.error_message?.includes('备用源'))return a.result.error_message
 if(a?.result?.http_status===429)return '测速源限流（429）'
 const noData=downloadNoDataReason(a)
 if(noData)return noData
 if(a?.result&&downloadTone(a)==='bad'){
  if(a.result.outcome==='time_limit')return '时限内未读取数据'
  if((a.result.phase||a.result.failure_phase||'').includes('handshake'))return '代理握手失败'
  return outcomeLabel(downloadOutcome(a))
 }
 if(value!==null)return `${value<.01?'<0.01':value.toFixed(value<1?2:1)} MiB/s${reading?.label==='短测均速'?' · 短测':reading?.label==='未稳定均速'?' · 未稳定':''}`
 return a?attemptStatus(a,true):'未测'
}
function downloadMetricTone(n:NodeOption){const point=hoveredDownloadPoint(n),a=point?.attempts?.[0]||tableDownload(n);return point?point.tone:a?downloadTone(a):'empty'}
function downloadMetricContext(n:NodeOption){const point=hoveredDownloadPoint(n),attempt=point?.attempts?.[0]||tableDownload(n),peak=downloadPeakSpeed(attempt),connection=downloadConnectionSeconds(attempt),suffix=(peak!==null?` · 峰值 ${peak.toFixed(2)} MiB/s`:'')+(attempt?` · ${downloadSourceName(attempt)}`:'')+(connection===null?'':` · 连接 ${connection.toFixed(2)} 秒`);if(point)return `${date(point.time)} · ${triggerLabel(point.trigger)} · ${attempt?attemptStatus(attempt,true).replace(' · 达到时限',''):point.description}${suffix}`;return attempt?`${date(attempt.finished_at||attempt.requested_at)} · ${triggerLabel(attempt.trigger_type)} · ${attemptStatus(attempt,true).replace(' · 达到时限','')}${suffix}`:'没有下载记录'}
function openCombinedPlan(){const nodes=[...runNodes.value];closeHover();w.project.value='combined';w.openPlan(nodes)}
function runLabel(p:Project){return p==='latency'?'测延迟':p==='download'?'测速':p==='service'?'解锁检测':'组合检测'}
function toggleAirport(id:string){const selected=w.selectedAirportIds.value||[];w.setAirports(selected.includes(id)?selected.filter(s=>s!==id):[...selected,id])}
function qualityLatencyPoint(n:NodeOption,target:string){
 const evidence=qualityData.value[key(n)]
 const test=evidence?.tests.filter(t=>sampleFor(t,target).length).reduce<import('../domain').LatencyTest|undefined>((recent,t)=>!recent||Date.parse(t.finished_at)>Date.parse(recent.finished_at)?t:recent,undefined)
 return test?manualLatencyPoint(test,target):undefined
}
function newestPoint(first?:TrendPoint,second?:TrendPoint){return !first||second&&Date.parse(second.time)>Date.parse(first.time)?second:first}
function metricPoint(n:NodeOption,target:string){
 const sorting=headerSort.value==='verdict'||headerSort.value==='site:'+target
 if(!sorting&&w.sort.value==='latency'&&target===w.displayTarget.value&&w.sortSnapshots.value[key(n)])return w.sortSnapshots.value[key(n)].latency
 const displayed=sitePoints(n,target).filter(p=>latencySiteSamples(p,target).length).at(-1)
 if(!sorting)return displayed
 const evidence=qualityData.value[key(n)]
 const manual=qualityLatencyPoint(n,target)
 const monitored=monitorLatencyPoints(evidence?.monitor||[],target).at(-1)
 // Keep the durable round reference when the same saved attempt is already loaded.
 return newestPoint(newestPoint(displayed,displayed?.latencyTests?.some(t=>t.attempt_id===manual?.id)?undefined:manual),monitored)
}
function latestLatencyRound(n:NodeOption){
 const saved=presentation.points(n,'latency','all').filter(p=>p.sampleGroups?.some(group=>group.samples.length)).at(-1)
 const sorting=headerSort.value==='verdict'||headerSort.value.startsWith('site:')
 const fallback=sorting?qualityLatencyPoint(n,'all'):undefined
 const manual=newestPoint(saved,saved?.latencyTests?.some(t=>t.attempt_id===fallback?.id)?undefined:fallback)
 return manual||newestPoint(presentation.monitor(n).at(-1),sorting?monitorLatencyPoints(qualityData.value[key(n)]?.monitor||[]).at(-1):undefined)
}
function latencyRowPoint(n:NodeOption,target:string,review?:TrendPoint){
 const detailed=w.project.value==='latency'
 const round=review??(detailed?latestLatencyRound(n):metricPoint(n,target)),samples=latencySiteSamples(round,target)
 if(!round||!samples.length)return detailed?undefined:round
 const responses=samples.filter(s=>s.value!==null).length
 const tone=samples.every(s=>s.tone==='empty')?'empty':!responses?'bad':responses<samples.length?'warn':'good'
 return {...round,samples,tone,value:detailed?latencyCellValue(round,target,true):round.value} satisfies TrendPoint
}
function rowVerdict(n:NodeOption,review?:TrendPoint){
 const sites=targets.filter(s=>w.displayTargets.value.includes(s.id))
 const array=sites.map(s=>latencyRowPoint(n,s.id,review))
 const stable=stableLatency(sites.map((s,i)=>({id:s.id,points:array[i]?[array[i]!]:[]})))
 const weekly=weeklyStats.value[weeklyHistoryKey(n)]
 return latencyVerdict(array,stable,review||weekly?.allOld?null:weekly?.all.responseRate,sites.map(s=>s.name))
}
function siteMetric(n:NodeOption,target:string){const point=metricPoint(n,target);const stable=stableLatency([{id:target,points:point?[point]:[]}]);return stable===null?(point?.value!=null?'—':latencyPointLabel(point,target)):Math.round(stable)+' ms'}
function siteContext(n:NodeOption,target:string){const p=metricPoint(n,target);const first=p?inspectorSamples(p,target)[0]?.value:null;return p?date(p.time)+(p.source==='monitor'?' · 监测':' · 手动')+(first!=null?' · 首次连接 '+Math.round(first)+' ms':''):'未测'}
function siteReason(n:NodeOption,target:string){return [...new Set(metricPoint(n,target)?.sampleGroups?.find(g=>latencyTargetId(g.target)===target)?.samples.map(s=>s.note||s.error).filter(Boolean)||[])].join('；')}
function expanded(n:NodeOption){return !!hoverNode.value&&key(hoverNode.value)===key(n)&&!!(activeRound.value||inlineTab.value==='history')}
function httpPoints(n:NodeOption,id=''){return presentation.http(n,id)}
function monitorPoints(n:NodeOption){return presentation.monitor(n)}
function rowPoints(n:NodeOption,p:Project){return p==='latency'?sitePoints(n,w.displayTarget.value):p==='service'?presentation.services(n):presentation.points(n,p,w.displayTarget.value)}
function sitePoints(n:NodeOption,target:string){return presentation.site(n,target)}
watch([pageNodes,hoverNode],([nodes,hover])=>presentation.retain(hover?[...nodes,hover]:nodes),{immediate:true,flush:'sync'})
watch(hoverPoints,list=>{const old=activeRound.value;if(!old||inlineTab.value!=='round')return;const updated=list.find(p=>p.id===old.id&&p.source===old.source);if(updated)activeRound.value=updated;else if(!keepOpen.value)closeHover()},{flush:'post'})
async function readMonitor(){
  void w.loadMeasurementRounds(pageNodes.value)
  const generation=++monitorGeneration,list=pageNodes.value,project=w.project.value,until=new Date().toISOString(),since=new Date(Date.now()-w.hours.value*3600000).toISOString()
  const queries=[...(project==='latency'||project==='combined'?[{probeType:'rtt',target:undefined}]:[]),...(project==='service'||project==='combined'?monitorHTTPServices:[])]
  for(const n of list)for(const q of queries)try{
    const result=await api.monitorSamples({profile_id:n.profile_id,node_identity_key:n.node_identity_key,config_revision_key:n.config_revision_key,probe_type:q.probeType,target:q.target,since,until,limit:q.probeType==='rtt'?480:80,order_desc:true})
    if(generation!==monitorGeneration)return
    if(q.probeType==='rtt'){if(!sameHistoryRecords(monitorData.value[key(n)],result.items||[]))monitorData.value[key(n)]=result.items||[];delete monitorErrors.value[key(n)]}
    else{if(!sameHistoryRecords((monitorHTTPData.value[key(n)]||[]).filter(s=>s.probe_type===q.probeType),result.items||[]))monitorHTTPData.value[key(n)]=[...(monitorHTTPData.value[key(n)]||[]).filter(s=>s.probe_type!==q.probeType),...(result.items||[])];delete monitorHTTPErrors.value[key(n)+':'+q.probeType]}
  }catch(e){if(generation!==monitorGeneration)return;const message=e instanceof Error?e.message:'监测记录读取失败';if(q.probeType==='rtt')monitorErrors.value[key(n)]=message;else monitorHTTPErrors.value[key(n)+':'+q.probeType]=message}
}

function keepRound(){clearTimeout(hoverTimer)}
function closeHover(){clearTimeout(hoverTimer);hoverNode.value=null;activeRound.value=null;inlineTab.value='round';keepOpen.value=false}
function leaveRound(){clearTimeout(hoverTimer);if(!keepOpen.value)hoverTimer=setTimeout(()=>{if(!keepOpen.value)closeHover()},140)}
function inspectRound(n:NodeOption,p:Project,point:TrendPoint,target=w.displayTarget.value,serviceId=''){keepRound();if(keepOpen.value)return;hoverNode.value=n;hoverMode.value=p==='combined'?'latency':p;hoverTarget.value=target;hoverService.value=serviceId||(p==='service'&&point.attempts?.length===1?point.attempts[0].service_id||'':'');activeRound.value=point;inlineTab.value='round'}
function openRound(n:NodeOption,p:Project,point:TrendPoint,target=w.displayTarget.value,serviceId=''){keepOpen.value=false;inspectRound(n,p,point,target,serviceId);keepOpen.value=true}
function expandHistory(){keepRound();keepOpen.value=true;inlineTab.value='history'}
watch([()=>w.search.value,()=>w.region.value,()=>w.subscription.value,()=>w.airportFilter.value,()=>w.sort.value,()=>w.priorityAirportId.value,()=>w.selectedAirportIds.value],()=>{currentPage.value=1;closeHover()},{deep:true})
watch(()=>w.hours.value,()=>{presentation.clear();monitorGeneration++;monitorData.value={};monitorErrors.value={};monitorHTTPData.value={};monitorHTTPErrors.value={}});watch(totalPages,count=>currentPage.value=Math.min(currentPage.value,count))
watch([pageNodes,qualityView,qualityScopeKey],([nodes,quick,scope],[,,previousScope])=>{
 if(previousScope!==undefined&&scope!==previousScope){qualityGeneration++;qualityData.value={};qualityErrors.value={};qualityLoading.value=false
  if((quick||siteSort.value!=='existing'||siteGate.value!=='all'||metricHeader.value)&&w.page.value==='home')void readQualityNodes(metricHeader.value||(quick?(qualityGate.value!=='all'&&qualityGate.value!=='response'||qualitySort.value!=='existing'):(siteSort.value!=='existing'||siteGate.value!=='all'))?w.proxyNodes.value:nodes)
  return
 }
 if((quick||siteSort.value!=='existing'||siteGate.value!=='all')&&w.page.value==='home')void readQualityNodes(nodes)
},{immediate:true})
watch(()=>w.hours.value,()=>{if(qualityView.value||siteSort.value!=='existing'||siteGate.value!=='all')void readQualityNodes(metricHeader.value||(qualityView.value?(qualityGate.value!=='all'&&qualityGate.value!=='response'||qualitySort.value!=='existing'):(siteSort.value!=='existing'||siteGate.value!=='all'))?w.proxyNodes.value:pageNodes.value,true);else{qualityGeneration++;qualityData.value={};qualityErrors.value={};qualityCutoff.value=new Date().toISOString()}})
watch([pageNodes,()=>w.project.value,()=>w.hours.value],()=>{if(w.page.value==='home'){void w.ensureHistory(pageNodes.value);void readMonitor()}},{immediate:true})
watch(()=>w.page.value,p=>{if(p==='home'){void w.ensureHistory(pageNodes.value);void readMonitor();if(qualityView.value||siteSort.value!=='existing'||siteGate.value!=='all')void readQualityNodes(metricHeader.value||(qualityView.value?(qualityGate.value!=='all'&&qualityGate.value!=='response'||qualitySort.value!=='existing'):(siteSort.value!=='existing'||siteGate.value!=='all'))?w.proxyNodes.value:pageNodes.value)}else{closeHover();presentation.clear()}})
watch(()=>w.serviceIds.value,closeHover,{deep:true})
watch(()=>w.displayServiceIds.value,()=>{servicePage.value=1;closeHover()},{deep:true})
watch(servicePage,closeHover)
watch(servicePages,count=>servicePage.value=Math.min(servicePage.value,count))
watch([()=>w.project.value,()=>w.hours.value,currentPage],closeHover)
watch(()=>w.displayTarget.value,()=>{if(keepOpen.value&&hoverNode.value){activeRound.value=null;inlineTab.value='history'}else closeHover()})
provide(homeModelKey,{refreshWeekly,weeklyStats,weeklyErrors,weeklyProgress,weeklyReady,headerSort,headerSortReady,sortHeader,compareOpen,compareTarget,compareNodes:qualityCompareNodes,compareCompatible,comparisonDownloadStatus,closeComparison,servicePoints:(n:NodeOption,id:string)=>presentation.points(n,'service',w.displayTarget.value,id),hiddenSelectedCount,qualityFiltersPending,monitorData,monitorErrors,monitorHTTPErrors,httpPoints,visibleServices,servicePage,servicePages,servicesPerPage,chosen,filterProfiles,protocolChoices,protocolFilter,qualityGate,qualitySort,qualityLoading,qualityErrors,qualityCutoff,allQualityReady,readQualityNodes,siteSummaries,pageSize,setPageSize,pageNodes,visibleNodes,runNodes,currentPage,totalPages,picker,filterRegions,qualityData,nodeActivities,displayServices,sourceLabel,metricPoint,latestLatencyRound,latencyRowPoint,rowVerdict,siteMetric,siteContext,siteReason,sitePoints,rowPoints,downloadMetric,downloadMetricTone,downloadMetricContext,serviceMetric,serviceTone,shownService,showService,showServiceEvidence,hoverNode,hoverMode,hoverTarget,hoverService,hoverPoints,activeRound,inlineTab,inlineHistorySource,keepOpen,inspectRound,openRound,leaveRound,keepRound,closeHover,expandHistory,openComparison,openCombinedPlan,runLabel})
onMounted(()=>monitorTimer=setInterval(()=>{if(w.page.value==='home'&&document.visibilityState==='visible')void readMonitor()},30000));onBeforeUnmount(()=>{weeklyGeneration++;weeklyWaiting.clear();qualityGeneration++;presentation.clear();monitorGeneration++;clearTimeout(hoverTimer);clearInterval(monitorTimer)})
</script>
<template>
  <section class="home-view">
    <HomeWorkbench/>
    <Modal v-if="serviceDetailPoint" title="服务结果与依据" wide @close="serviceDetail=null"><MeasurementResults :point="serviceDetailPoint" project="service"/></Modal>
    <Modal v-if="picker" title="选择机场" @close="picker=false"><p class="muted">选择会同步更新节点范围，已选节点保留在这个范围内。</p><div class="airport-pick-list"><label v-for="airport in w.airports.value" :key="airport.id" :class="{selected:w.selectedAirportIds.value?.includes(airport.id)}"><input type="checkbox" :checked="w.selectedAirportIds.value?.includes(airport.id)" @change="toggleAirport(airport.id)"><span class="airport-logo">{{airport.name.slice(0,1)}}</span><span><strong>{{airport.name}}</strong><small>{{airport.subscriptions?.length||1}} 个订阅</small></span><span class="muted">{{airportNodeCount(airport.id)}} 个节点</span></label></div><div v-if="!w.airports.value.length" class="empty-state">还没有机场订阅<button class="button" @click="picker=false;w.page.value='airports'">添加机场</button></div><template #footer><button class="button ghost" @click="w.setAirports([])">取消全部</button><button class="button" @click="w.setAirports(w.airports.value.map(a=>a.id))">全选机场</button><button class="button primary" @click="picker=false">完成</button></template></Modal>
  </section>
</template>

<style scoped>
.six-site-controls{display:flex;flex-wrap:wrap;gap:12px;align-items:center;margin:12px 0}.six-site-controls label{display:flex;align-items:center;gap:8px;font-size:13px}.six-site-controls p{flex-basis:100%;margin:0;font-size:13px;color:var(--muted)}
</style>
