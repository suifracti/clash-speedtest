<script setup lang="ts">
import {computed,inject,nextTick,onBeforeUnmount,onMounted,ref,shallowRef,useId,watch} from 'vue'
import '../../home-design.css'
import './workbench-layout.css'
import {useWorkspace} from '../../workspace'
import {date,downloadAverageSpeed,downloadPeakSpeed,downloadReading,downloadSourceName,isReputationRule,key,median,projectNames,targets,type NodeOption,type Project} from '../../domain'
import {attemptObservedAt,downloadEvidence} from '../../nodeQuality'
import {api,type ClashUsage} from '../../api'
import {nodeAttrMeaning,nodeDisplayName,nodeDisplayParts,nodeRegion,nodeRegionLabel,protocolBadgeClass,regionBadgeClass} from '../../nodePresentation'
import {airportNodeCount} from '../../airportViewing'
import Icon from '../Icon.vue'
import UiSelect from '../UiSelect.vue'
import UnlockPicker from '../UnlockPicker.vue'
import SubscriptionOverview from '../SubscriptionOverview.vue'
import SiteDisplayPicker from '../SiteDisplayPicker.vue'
import UnlockServiceMatrix from './UnlockServiceMatrix.vue'
import NodeInspector from './NodeInspector.vue'
import NodeResponseStrip from './NodeResponseStrip.vue'
import LatencyHistoryMarks from './LatencyHistoryMarks.vue'
import WorkbenchSortHeader from './WorkbenchSortHeader.vue'
import NodeReaderNav from './NodeReaderNav.vue'
import NodeCompare from './NodeCompare.vue'
import NodeAttrPopover from './NodeAttrPopover.vue'
import Modal from '../Modal.vue'
import type {TrendPoint} from '../../presentation'
import {latencySiteSamples,stableLatency} from '../../inspectorLatency'
import {computeTipAnchor,hideChartTip,showChartTip} from '../../chartTip'
import {homeModelKey,weeklyHistoryKey} from './homeModel'
import {exitLanding} from '../../exitLanding'
import {entryCountry,isAnycast,lineLabel,routeKind,routeLabel,routeTone} from '../../entryPath'
import {unlockGroupDetailFor} from '../../unlock'
import {displayUnlockGroups} from '../../unlockGroupStore'
const w=useWorkspace(),m=inject(homeModelKey)!
const readingHintId=useId()
const exitMode=ref(false),serviceView=ref('nodes')
const latencyReview=shallowRef<{node:NodeOption;point:TrendPoint;target:string}>()
watch([()=>w.project.value,exitMode,()=>w.hours.value],()=>{latencyReview.value=undefined})
const clashUsage=ref<ClashUsage[]>([])
const dominantClashUsage=computed(()=>clashUsage.value[0])
const dominantClashNode=computed(()=>w.nodes.value.find(n=>n.display_name===dominantClashUsage.value?.name))
const dominantClashName=computed(()=>{const usage=dominantClashUsage.value;if(!usage)return '';const node=w.nodes.value.find(n=>n.display_name===usage.name);return node?nodeDisplayName(node):usage.name})
const dominantClashChains=computed(()=>dominantClashUsage.value?.chains.join(' → ')||'')
const clashUsingSet=computed(()=>new Set(clashUsage.value.map(usage=>usage.name)))
function isClashUsing(n:NodeOption){return clashUsingSet.value.has(n.display_name)}
async function refreshClashUsage(){try{clashUsage.value=await api.clashUsage()}catch{clashUsage.value=[]}}
let clashUsageTimer:number|undefined
function startClashUsagePolling(){
 if(clashUsageTimer!==undefined)window.clearInterval(clashUsageTimer)
 if(document.visibilityState!=='visible')return
 void refreshClashUsage()
 clashUsageTimer=window.setInterval(()=>{if(document.visibilityState==='visible')void refreshClashUsage()},30_000)
}
function onClashVisibilityChange(){
 if(document.visibilityState==='visible')startClashUsagePolling()
 else if(clashUsageTimer!==undefined){window.clearInterval(clashUsageTimer);clashUsageTimer=undefined}
}
onMounted(()=>{document.addEventListener('visibilitychange',onClashVisibilityChange);startClashUsagePolling()})
onBeforeUnmount(()=>{document.removeEventListener('visibilitychange',onClashVisibilityChange);if(clashUsageTimer!==undefined)window.clearInterval(clashUsageTimer)})
const matrixMode=computed(()=>!exitMode.value&&w.project.value==='service'&&serviceView.value==='services')
const showVerdict=computed(()=>!exitMode.value&&['combined','latency'].includes(w.project.value))
const overviewMode=computed(()=>!exitMode.value&&w.project.value==='combined')
const showSites=computed(()=>!exitMode.value&&w.project.value==='latency')
const showDownload=computed(()=>!exitMode.value&&['combined','download'].includes(w.project.value))
const showServiceGroups=computed(()=>!exitMode.value&&w.project.value==='service'&&!matrixMode.value)
const chosen=m.chosen
const exitRows=computed(()=>(overviewMode.value||exitMode.value||showVerdict.value||showServiceGroups.value)?Object.fromEntries(m.pageNodes.value.map(n=>{
 const landing=exitLanding(n,[...(m.qualityData.value[key(n)]?.services||[]),...(w.services.value[key(n)]||[])])
 const entry=w.entryFor(n),kind=routeKind(entryCountry(entry),landing.consensusRegion,isAnycast(entry),w.localPosition.value?.country_code)
 // 出口没测过时线路是按节点名称推的，标签置灰并在悬停里说明，别让人当成实测结论。
 const guessed=!landing.exitKnown&&kind!=='anycast'
 return [key(n),{...landing,ipTag:landing.ipKind.tag,route:routeLabel(kind),routeTone:guessed?'empty':routeTone(kind),routeTitle:guessed?'出口还没检测，线路按节点名称推断':routeLabel(kind)==='任播'?'入口是 Cloudflare 任播地址，不代表真实机房':'入口与出口国家相同为直连（本机在大陆、入口在境外时为跨境直连），不同为中转，入口在中国大陆为国内中转',line:lineLabel(entry,landing.consensusRegion,landing.org)}]
})):{})
function selectInspectorProject(id:Project|'exit'){if(id==='exit')selectExit();else{exitMode.value=false;w.project.value=id}}
function selectExit(){exitMode.value=true;m.headerSort.value='';m.qualityGate.value='all';m.qualitySort.value='existing';if(['latency','download','service'].includes(w.sort.value))w.sort.value='region'}
// 出口表只读缓存；真要解析要按“解析入口”或跑一次出口检测。
watch([exitMode,()=>m.pageNodes.value],()=>{if(exitMode.value)void w.readEntries(m.pageNodes.value,false).catch(()=>{})},{immediate:true})
const exitColumns=['线路','出口 IP','真实落地','IP 类型','CF 盾','纯净度']
const exitFields=['line','ip','label','ipTag','cfChallenge','purity'] as const
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)))
// 同名节点的七天统计包含配置变化前后的历史。
const weeklyProgress=m.weeklyProgress
function weeklyStat(n:NodeOption){return m.weeklyStats.value[weeklyHistoryKey(n)]}
function weeklyOld(n:NodeOption){return !!weeklyStat(n)?.allOld}
function weeklyText(n:NodeOption,metric:'responseRate'|'volatility'){
 const s=weeklyStat(n)
 if(!s)return m.weeklyErrors.value[weeklyHistoryKey(n)]?'读取失败':'读取中'
 const value=s.all[metric]
 if(metric==='responseRate')return value!=null?Math.round(value)+'%':'未测'
 return value!=null?Math.round(value)+' ms':'—'
}
function weeklyTone(n:NodeOption){const s=weeklyStat(n);return !s||s.all.responseRate==null?'empty':s.all.responseRate>=90?'good':'warn'}
function weeklyTitle(n:NodeOption,metric:'responseRate'|'volatility'){
 const s=weeklyStat(n),error=m.weeklyErrors.value[weeklyHistoryKey(n)]
 if(error)return error
 if(metric==='volatility'&&(!s||s.all.volatility==null))return '满 5 轮后显示'
 if(!s)return '近 7 天 · 0 轮'
 const show=(v:number|null)=>v==null?'—':metric==='responseRate'?Math.round(v)+'%':Math.round(v)+' ms'
 const base=metric==='responseRate'?'近 7 天':'慢的时候比快的时候多多少（P90−P10）'
 if(s.allOld)return '近 7 天只有旧配置记录 · 当前配置还没测过'
 if(s.old.total)return `当前配置 ${s.current.total} 轮 ${show(s.current[metric])} · 旧配置 ${s.old.total} 轮 ${show(s.old[metric])}`
 return metric==='responseRate'?`近 7 天 · ${s.all.total} 轮`:base
}
const catalog=computed(()=>w.catalog.value.filter(r=>!isReputationRule(r)))
const projects:Project[]=['combined','latency','download','service']
const chosenNodeCount=computed(()=>chosen.value.reduce((count,airport)=>count+airportNodeCount(airport,w.nodes.value),0))
const hasSourceNodes=computed(()=>w.proxyNodes.value.some(n=>w.airportFilter.value==='all'||w.nodeAirportId(n)===w.airportFilter.value))
function clearFilters(){w.search.value='';w.region.value='all';w.excludeRegion.value=false;w.subscription.value='all';w.downloadFullOnly.value=false;m.protocolFilter.value='all';m.qualityGate.value='all';m.currentPage.value=1}
async function inspectClashUsage(){
 const node=dominantClashNode.value;if(!node)return
 clearFilters();w.viewAirport('all');m.qualitySort.value='existing';m.headerSort.value='';m.compareOpen.value=false
 exitMode.value=false;serviceView.value='nodes';w.latencyReportFocus.value=null
 await nextTick();w.viewRequest.value=node
}
const sorts=computed(()=>[{id:'region',label:'地区 → 机场'},{id:'airport',label:'机场 → 地区'},{id:'name',label:'名称'},
 ...(!exitMode.value&&['latency','combined'].includes(w.project.value)?[{id:'latency',label:'已保存延迟 / 监测'}]:[]),
 ...(!exitMode.value&&['download','combined'].includes(w.project.value)?[{id:'download',label:'最近下载速度'}]:[]),
 ...(!exitMode.value&&['service','combined'].includes(w.project.value)?[{id:'service',label:'自选服务通过'}]:[])])
const gates=[{id:'all',label:'全部节点',short:'全部'},{id:'samples',label:'测得够多的（成功样本 ≥5 次）',short:'测得够多的'},{id:'response',label:'稳定能连的（近 7 天响应率 ≥90%）',short:'稳定能连的'},{id:'fresh',label:'最近 24 小时测过的',short:'最近 24 小时测过的'},{id:'latency_issue',label:'延迟有异常（含 403 / 超时 / 丢包 / 高延迟）',short:'延迟有异常的'},{id:'purity_clean',label:'出口纯净度 ≥ 80（纯净）',short:'纯净度 ≥ 80'},{id:'download',label:'有过有效下载速度的',short:'有过有效下载速度的节点'},{id:'service',label:'已选服务检测通过的',short:'已选服务检测通过的节点'}]
// 每个标签只给和它相关的条件；综合标签保留全部。
const tabGateIds:Record<Project,string[]>={combined:['all','samples','response','fresh','latency_issue','purity_clean','download','service'],latency:['all','samples','response','fresh','latency_issue','purity_clean'],download:['all','download'],service:['all','service']}
const tabGates=computed(()=>gates.filter(g=>(exitMode.value?['all','purity_clean']:tabGateIds[w.project.value]).includes(g.id)))
const latencyTab=computed(()=>!exitMode.value&&['combined','latency'].includes(w.project.value))
const resultHint=computed(()=>({combined:'按已保存的检测记录筛选和排序',latency:'用已保存的延迟记录筛选，再按延迟统计排序',download:'用已保存的测速记录筛选',service:'用已保存的服务检测记录筛选'} as Record<Project,string>)[w.project.value])
watch(()=>w.project.value,p=>{if(!tabGateIds[p].includes(m.qualityGate.value))m.qualityGate.value='all';if(!['combined','latency'].includes(p)&&m.qualitySort.value!=='existing')m.qualitySort.value='existing'})
const qualitySorts=[{id:'existing',label:'不额外排序（用上方排序）'},{id:'observed',label:'最近一次延迟，低到高（仅供参考）'},{id:'p50',label:'典型延迟（中位数），低到高'},{id:'p95',label:'慢的时候（P95），低到高'},{id:'rate',label:'成功率，高到低'}]
const moreActive=computed(()=>[w.airportFilter.value!=='all',w.subscription.value!=='all',w.excludeRegion.value,m.protocolFilter.value!=='all',m.qualityGate.value!=='all',m.qualitySort.value!=='existing',w.downloadFullOnly.value&&w.project.value==='download'].filter(Boolean).length)
const filterScopePending=computed(()=>!w.selectedNodes.value.length&&m.qualityFiltersPending.value)
const scopeLabel=computed(()=>w.selectedNodes.value.length?`已选 ${w.selectedNodes.value.length} 个节点`:filterScopePending.value?'正在确认筛选范围':`筛选 ${m.runNodes.value.length} 个`)
const filterSummary=computed(()=>[
 w.airportFilter.value==='all'?'':chosen.value.find(a=>a.id===w.airportFilter.value)?.name,
 w.subscription.value==='all'?'':m.filterProfiles.value.find(([id])=>id===w.subscription.value)?.[1],
 w.region.value!=='all'&&w.excludeRegion.value?'排除 '+selectedRegions.value.map(nodeRegionLabel).join(' / '):'',
 m.protocolFilter.value==='all'?'':m.protocolFilter.value,
 m.qualityGate.value==='all'?'':gates.find(g=>g.id===m.qualityGate.value)?.label,
 m.qualitySort.value==='existing'?'':qualitySorts.find(q=>q.id===m.qualitySort.value)?.label,
 w.downloadFullOnly.value&&w.project.value==='download'?'只看达到结束条件':''].filter(Boolean).join(' · '))
const more=ref<HTMLDetailsElement>(),moreSummary=ref<HTMLElement>()
function closeMore(){if(!more.value?.open)return;more.value.open=false;moreSummary.value?.focus({preventScroll:true})}
// 按机场筛选时，页头和来源带都切到这个机场，不再显示全部机场的汇总。
const filteredAirport=computed(()=>w.airportFilter.value==='all'?undefined:chosen.value.find(a=>a.id===w.airportFilter.value))
const filteredAirportNodes=computed(()=>airportNodeCount(w.airports.value.find(airport=>airport.id===w.airportFilter.value),w.nodes.value))
const qualityFiltering=computed(()=>m.qualityGate.value!=='all'||m.qualitySort.value!=='existing')
const inspectorOpen=ref(false)
const detailViewport=typeof window.matchMedia==='function'?window.matchMedia('(max-width: 1439px)'):undefined
const detailViewportNarrow=ref(detailViewport?.matches??false)
const workAreaWidth=ref(0)
// Leave enough of the list visible beside the reader, including its fixed identity column.
const MIN_LIST_WIDTH=640
const INLINE_READER_WIDTH=1080
const DIVIDER_WIDTH=12
const detailsOverlay=computed(()=>workAreaWidth.value>0?workAreaWidth.value<INLINE_READER_WIDTH:detailViewportNarrow.value)
function updateDetailViewport(){detailViewportNarrow.value=detailViewport?.matches??false}
onMounted(()=>detailViewport?.addEventListener('change',updateDetailViewport))
onBeforeUnmount(()=>detailViewport?.removeEventListener('change',updateDetailViewport))
function closeDetails(){if(m.compareOpen.value)m.closeComparison();else{inspectorOpen.value=false;void nextTick(focusReadingRow)}}
const ledgerExpanded=ref(false)
function savedSidebarCollapsed(){try{return window.localStorage.getItem('cst.sidebar-collapsed')==='true'}catch{return false}}
const sidebarCollapsed=ref(savedSidebarCollapsed()),sidebarSearchOpen=ref(false),sidebarSearch=ref('')
function setSidebarCollapsed(next:boolean){sidebarCollapsed.value=next;try{window.localStorage.setItem('cst.sidebar-collapsed',String(next))}catch{}}
const focusKey=ref<string|null>(null)
const focusNode=computed(()=>m.pageNodes.value.find(n=>key(n)===focusKey.value)||m.pageNodes.value[0])
const readingNode=ref<NodeOption|null>(null),readingNodes=ref<NodeOption[]>([]),readerBody=ref<HTMLElement>()
const sameReadingNode=(a:NodeOption,b:NodeOption)=>key(a)===key(b)&&a.node_key===b.node_key
const readingIndex=computed(()=>readingNode.value?readingNodes.value.findIndex(n=>sameReadingNode(n,readingNode.value!)):-1)
const readingCurrent=computed(()=>!!readingNode.value&&w.nodes.value.some(n=>sameReadingNode(n,readingNode.value!)))
const readingNotice=computed(()=>{
 const n=readingNode.value;if(!n)return ''
 if(!readingCurrent.value)return w.nodes.value.some(v=>v.profile_id===n.profile_id&&(v.node_key===n.node_key||v.node_identity_key===n.node_identity_key))?'配置已变化，保留打开时的记录；检测请回到列表选择当前配置。':'节点已不在当前清单，保留打开时的记录。'
 return m.visibleNodes.value.some(v=>sameReadingNode(v,n))?'':'这条节点在当前筛选之外，保留本次浏览。'
})
function readerLabel(offset:number){const n=readingNodes.value[readingIndex.value+offset];return n?nodeDisplayName(n)+' · '+m.sourceLabel(n):undefined}
function focusReadingRow(){const n=readingNode.value;if(!n)return;const row=readingCurrent.value?Array.from(table.value?.querySelectorAll<HTMLElement>('[data-node-key]')||[]).find(row=>row.dataset.nodeKey===key(n)):undefined;if(!row){table.value?.focus({preventScroll:true});return}row.querySelector<HTMLButtonElement>('.hd-node-open')?.focus({preventScroll:true});row.scrollIntoView?.({block:'nearest',inline:'nearest'})}
function showReadingNode(n:NodeOption){
 readingNode.value={...n};focusKey.value=key(n);m.closeHover()
 const index=m.visibleNodes.value.findIndex(v=>sameReadingNode(v,n));if(index>=0)m.currentPage.value=m.pageSize.value>0?Math.floor(index/m.pageSize.value)+1:1
 void nextTick(()=>readerBody.value?.closest<HTMLElement>('.modal-body')?.scrollTo?.({top:0}))
}
function startReading(n:NodeOption,clearReport=true){readingNodes.value=m.visibleNodes.value.map(n=>({...n}));if(clearReport)w.latencyReportFocus.value=null;showReadingNode(n);inspectorOpen.value=true}
function focus(n:NodeOption){startReading(n)}
async function openLatencyRound(n:NodeOption,point:TrendPoint,target:string){
 startReading(n)
 await nextTick()
 m.openRound(n,'latency',point,target)
}
function latencyReviewFor(n:NodeOption){
 const review=latencyReview.value
 return review&&sameReadingNode(review.node,n)?review:undefined
}
function reviewLatencyRound(n:NodeOption,point:TrendPoint,target:string){
 latencyReview.value={node:n,point,target}
 void openLatencyRound(n,point,target)
}
function onWholeHistorySelect(point:TrendPoint,target:string,node?:NodeOption){
 if(node)reviewLatencyRound(node,point,target)
}
function resetLatencyReview(n:NodeOption){
 latencyReview.value=undefined
 if(m.hoverMode.value==='latency'&&m.hoverNode.value&&sameReadingNode(m.hoverNode.value,n))m.closeHover()
}
function changePage(step:number,fromFooter=false){
 m.currentPage.value+=step
 if(fromFooter)void nextTick(()=>table.value?.scrollIntoView?.({block:'start'}))
}
function moveReading(offset:number){const n=readingNodes.value[readingIndex.value+offset];if(!n)return;w.latencyReportFocus.value=null;showReadingNode(n)}
function toggleDetails(){if(inspectorOpen.value)closeDetails();else if(focusNode.value)startReading(focusNode.value)}
// 检查器宽度可拖动（或用 ←→），记在本机。
const SIDE_MIN=340,SIDE_MAX=680
function savedSide(){try{const v=Number(window.localStorage.getItem('cst.inspector-width'));return v>=SIDE_MIN&&v<=SIDE_MAX?v:420}catch{return 420}}
const sideWidth=ref(savedSide())
const maxSideWidth=computed(()=>workAreaWidth.value>0?Math.max(SIDE_MIN,Math.min(SIDE_MAX,Math.floor(workAreaWidth.value-MIN_LIST_WIDTH-DIVIDER_WIDTH))):SIDE_MAX)
const visibleSideWidth=computed(()=>Math.min(sideWidth.value,maxSideWidth.value))
function setSide(v:number){sideWidth.value=Math.round(Math.max(SIDE_MIN,Math.min(maxSideWidth.value,v)));try{window.localStorage.setItem('cst.inspector-width',String(sideWidth.value))}catch{/* 只影响记忆 */}}
function startResize(e:PointerEvent){
 const start=e.clientX,width=visibleSideWidth.value,el=e.currentTarget as HTMLElement
 el.setPointerCapture?.(e.pointerId)
 const move=(ev:PointerEvent)=>setSide(width+start-ev.clientX)
 const up=()=>{el.removeEventListener('pointermove',move);el.removeEventListener('pointerup',up);document.body.classList.remove('cst-resizing')}
 document.body.classList.add('cst-resizing');el.addEventListener('pointermove',move);el.addEventListener('pointerup',up)
}
// ⌘K「查看节点」：只改查看中的节点和页码，不勾选、不动筛选；被筛掉的节点改用详情弹窗查看。
const table=ref<HTMLElement>()
const rowScroller=ref<HTMLElement>(),toolbar=ref<HTMLElement>(),workArea=ref<HTMLElement>(),toolbarHeight=ref(0)
let toolbarObserver:ResizeObserver|undefined,lastHorizontalScroll=-1
function syncTableHeading(){const next=rowScroller.value?.scrollLeft||0;if(next===lastHorizontalScroll)return;lastHorizontalScroll=next;table.value?.style.setProperty('--hd-horizontal-offset',next+'px')}
function measureToolbar(){toolbarHeight.value=toolbar.value?.getBoundingClientRect().height||0;workAreaWidth.value=workArea.value?.getBoundingClientRect().width||0;void nextTick(syncTableHeading)}
onMounted(()=>{measureToolbar();if(typeof ResizeObserver!=='undefined'&&toolbar.value){toolbarObserver=new ResizeObserver(measureToolbar);toolbarObserver.observe(toolbar.value);if(workArea.value)toolbarObserver.observe(workArea.value)}})
onBeforeUnmount(()=>toolbarObserver?.disconnect())
watch(()=>w.viewRequest.value,async n=>{
 if(!n)return
 w.viewRequest.value=null
 if(w.latencyReportFocus.value?.nodeKey===key(n)){exitMode.value=false;m.qualityGate.value='all';m.protocolFilter.value='all';m.qualitySort.value='existing';m.headerSort.value='';await nextTick()}
 const i=m.visibleNodes.value.findIndex(v=>sameReadingNode(v,n))
 if(i<0){w.inspectNode.value=n;return}
 startReading(n,false)
 void nextTick(()=>table.value?.querySelector<HTMLElement>('.hd-tr.focus')?.scrollIntoView?.({block:'nearest'}))
},{immediate:true})
function isFocus(n:NodeOption){const current=inspectorOpen.value&&readingNode.value?readingNode.value:focusNode.value;return !!current&&sameReadingNode(current,n)}
const selectedKeySet=computed(()=>new Set(w.selectedKeys.value))
function selected(n:NodeOption){return selectedKeySet.value.has(key(n))}
const verdicts=computed(()=>showVerdict.value?Object.fromEntries(m.pageNodes.value.map(n=>{
 const review=latencyReviewFor(n)
 return [key(n),m.rowVerdict(n,review?.target==='all'?review.point:undefined)]
})):{})
function run(){if(filterScopePending.value)return;if(exitMode.value){m.closeHover();w.openReputationPlan([...m.runNodes.value]);return}if(w.project.value==='combined')m.openCombinedPlan();else{m.closeHover();w.openPlan([...m.runNodes.value])}}
const wholeLatencyHistory=computed(()=>Object.fromEntries(showSites.value?m.pageNodes.value.map(n=>[
 key(n),m.sitePoints(n,'all').filter(p=>w.displayTargets.value.some(id=>latencySiteSamples(p,id).length)).slice(-8),
]):[]))
const downloadRows=computed(()=>showDownload.value?Object.fromEntries(m.pageNodes.value.map(n=>{
 const records=[...new Map([...(w.downloads.value[key(n)]||[]),...(m.qualityData.value[key(n)]?.downloads||[])].map(a=>[a.attempt_id,a])).values()]
 const {latest}=downloadEvidence(records),reading=downloadReading(latest)
 const val=reading?.value??null,peak=downloadPeakSpeed(latest),avg=downloadAverageSpeed(latest)
 const source=latest?downloadSourceName(latest):'',at=latest?attemptObservedAt(latest):''
 const pct=val!==null?Math.max(6,Math.min(100,Math.round(val/50*100))):0
 const secondary=peak!==null&&val!==null&&Math.abs(peak-val)>=0.2?`峰值 ${peak.toFixed(1)} MiB/s`:avg!==null?`全程 ${avg.toFixed(1)} MiB/s`:''
 const metric=m.downloadMetric(n),tone=m.downloadMetricTone(n),context=overviewMode.value?'':m.downloadMetricContext(n)
 return [key(n),{val,pct,label:reading?.label||'',source,at,secondary,hasAttempt:!!latest,metric,tone,context}]
})):{})
const nodeRowMeta=computed(()=>Object.fromEntries(m.pageNodes.value.map(n=>{
 const parsed=nodeDisplayParts(n)
 const source=exitMode.value?w.nodeAirportName(n):m.sourceLabel(n)
 const fullSource=m.sourceLabel(n)
 return [key(n),{
  region:parsed.region,
  parts:parsed.parts.map(part=>({...part,hintDetail:nodeAttrMeaning(part.text)?.detail})),
  displayName:nodeDisplayName(n),
  source,
  fullSource,
  regionClass:regionBadgeClass(n.country_code),
  protoClass:protocolBadgeClass(n.type),
 }]
})))
const overviewLatencyMap=computed(()=>overviewMode.value?Object.fromEntries(m.pageNodes.value.map(n=>{
 const verdict=verdicts.value[key(n)]
 const siteData=sites.value.map(s=>{
  const pt=m.latencyRowPoint(n,s.id)
  const samples=latencySiteSamples(pt,s.id)
  const responded=samples.filter(sp=>sp.value!==null).length,total=samples.length
  const httpCode=samples.map(sp=>sp.note?.match(/HTTP (\d+)/)?.[1]).find(Boolean)||''
  const httpCount=httpCode?samples.filter(sp=>sp.note?.includes('HTTP '+httpCode)).length:0
  const stableMs=stableLatency([{id:s.id,points:pt?[pt]:[]}])??(pt?.value??null)
  const shortName=s.name==='Cloudflare'?'CF':s.name
  return {id:s.id,name:s.name,shortName,pt,responded,total,httpCode,httpCount,stableMs}
 })
 const vals=siteData.map(s=>s.stableMs).filter((v):v is number=>v!=null&&Number.isFinite(v)).sort((a,b)=>a-b)
 const med=median(vals)
 const anomalies:Array<{label:string;tone:'warn'|'bad'}>=[]
 for(const s of siteData){
  if(s.total>0&&s.responded===0){anomalies.push({label:`${s.shortName} 超时`,tone:'bad'});continue}
  if(s.httpCode){
   const part=s.responded<s.total?`${s.responded}/${s.total}·`:''
   anomalies.push({label:`${s.shortName} ${part}${s.httpCode}`,tone:'warn'})
   continue
  }
  if(s.total>0&&s.responded<s.total){anomalies.push({label:`${s.shortName} ${s.responded}/${s.total}`,tone:'warn'});continue}
  if(s.stableMs!==null&&(s.stableMs>=1200||(med!==null&&s.stableMs>=800&&s.stableMs>=med*1.8))){
   anomalies.push({label:`${s.shortName} ${Math.round(s.stableMs)}ms`,tone:'warn'})
  }
 }
 const tipSites=siteData.map(s=>({
  name:s.name,
  value:s.stableMs!==null?`${Math.round(s.stableMs)} ms`:s.total>0?'超时':'未测',
  count:s.total>0?`${s.responded}/${s.total}`:'—',
  badge:s.httpCode?`HTTP ${s.httpCode}`:s.total>0&&s.responded===0?'无响应':s.total>0&&s.responded<s.total?'部分丢包':s.stableMs!==null&&s.stableMs>=1200?'延迟高':'',
  tone:s.total===0?'empty':s.responded===0?'bad':s.httpCode||s.responded<s.total||(s.stableMs!==null&&s.stableMs>=1200)?'warn':'good',
 }))
 const measured=verdict.tone!=='empty'||vals.length>0||siteData.some(s=>s.total>0)
 return [key(n),{measured,verdict,ms:med!==null?`${Math.round(med)} ms`:'',sub:vals.length?`${vals.length}/${sites.value.length} 站响应`:siteData.some(s=>s.total>0)?`0/${sites.value.length} 站响应`:'',anomalies,tipSites}]
})):{})
const overviewUnlockMap=computed(()=>overviewMode.value?Object.fromEntries(m.pageNodes.value.map(n=>{
 const region=w.nodeUnlockRegion(n),ipType=exitRows.value[key(n)]?.ipKind.type
 const groups=displayUnlockGroups.value.map(g=>({id:g.id,name:g.name,...unlockGroupDetailFor(g,catalog.value,serviceId=>m.shownService(n,serviceId),region,w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions,ipType)})).filter(g=>g.total>0||g.attemptedItems.length>0||(g.tone!=='empty'&&g.label!=='未测'&&g.label!=='—'))
 const itemRank=(tone:string)=>tone==='bad'?0:tone==='warn'?1:tone==='good'?2:3
 const seenIds=new Set<string>()
 const items=groups.flatMap(g=>g.attemptedItems.map(item=>({...item,groupName:g.name}))).filter(item=>{if(seenIds.has(item.id))return false;seenIds.add(item.id);return true}).sort((a,b)=>itemRank(a.tone)-itemRank(b.tone))
 return [key(n),{groups,items}]
})):{})
const serviceGroupMap=computed(()=>showServiceGroups.value?Object.fromEntries(m.pageNodes.value.map(n=>{
 const region=w.nodeUnlockRegion(n),ipType=exitRows.value[key(n)]?.ipKind.type
 return [key(n),displayUnlockGroups.value.map(g=>({id:g.id,name:g.name,...unlockGroupDetailFor(g,catalog.value,serviceId=>m.shownService(n,serviceId),region,w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions,ipType)}))]
})):{})
function showOverviewLatencyTip(e:MouseEvent,n:NodeOption){
 const row=overviewLatencyMap.value[key(n)]
 if(!row?.measured)return
 const anchor=computeTipAnchor(e.currentTarget as HTMLElement|null,154,180)
 if(!anchor)return
 showChartTip({kind:'overview-latency',title:`${nodeRowMeta.value[key(n)]?.displayName||n.display_name} · 延迟`,sub:row.sub||row.verdict.label,sites:row.tipSites,...anchor})
}
function showOverviewUnlockTip(e:MouseEvent,n:NodeOption){
 const data=overviewUnlockMap.value[key(n)]
 if(!data?.groups.length)return
 const anchor=computeTipAnchor(e.currentTarget as HTMLElement|null,168,210)
 if(!anchor)return
 showChartTip({
  kind:'unlock-detail',
  title:`${nodeRowMeta.value[key(n)]?.displayName||n.display_name} · 服务解锁`,
  groups:data.groups.map(g=>({name:g.name,ratio:g.total?`${g.available}/${g.total} 可用`:'待确认',items:g.attemptedItems.map(item=>({name:item.name,label:item.result.label,tone:item.tone,reason:item.result.reason&&item.result.reason!=='检测规则通过'?item.result.reason:''}))})),
  ...anchor,
 })
}
function showServiceGroupTip(e:MouseEvent,n:NodeOption,g:(typeof serviceGroupMap.value)[string][number]){
 if(!g.attemptedItems.length)return
 const anchor=computeTipAnchor(e.currentTarget as HTMLElement|null,168,210)
 if(!anchor)return
 showChartTip({
  kind:'unlock-detail',
  title:`${nodeRowMeta.value[key(n)]?.displayName||n.display_name} · ${g.name}`,
  groups:[{name:g.name,ratio:g.total?`${g.available}/${g.total} 可用`:'待确认',items:g.attemptedItems.map(item=>({name:item.name,label:item.result.label,tone:item.tone,reason:item.result.reason&&item.result.reason!=='检测规则通过'?item.result.reason:''}))}],
  ...anchor,
 })
}
const weeklyRowMap=computed(()=>showSites.value?Object.fromEntries(m.pageNodes.value.map(n=>[key(n),{
 old:weeklyOld(n),
 respText:weeklyText(n,'responseRate'),
 respTone:weeklyTone(n),
 respTitle:weeklyTitle(n,'responseRate'),
 volText:weeklyText(n,'volatility'),
 volTone:weeklyStat(n)?.all.volatility==null?'empty':weeklyTone(n),
 volTitle:weeklyTitle(n,'volatility'),
}])): {})
function quickPlanSingle(n:NodeOption,proj:Exclude<Project,'combined'>){
 m.closeHover()
 const prev=w.project.value;w.project.value=proj
 w.openPlan([n])
 w.project.value=prev
 if(w.plan.value){w.plan.value.projects=[proj];w.plan.value.includeReputation=false;w.plan.value.reputationServiceIds=[]}
}
// Grid tracks follow the active project so each tab shows its own columns.
const columns=computed(()=>overviewMode.value?['28px','minmax(210px,1.45fr)','minmax(168px,1.25fr)','minmax(122px,.9fr)','minmax(168px,1.25fr)','minmax(132px,.95fr)'].join(' '):exitMode.value?['28px','minmax(185px,1.55fr)','minmax(96px,1fr)','84px','minmax(92px,1fr)','80px','76px','minmax(112px,.95fr)'].join(' '):showSites.value?['28px','minmax(188px,1.45fr)','82px','96px','78px','72px',...sites.value.map(()=>'minmax(108px,1fr)')].join(' '):w.project.value==='download'?['36px','minmax(215px,1.65fr)','minmax(175px,1.15fr)','minmax(125px,.85fr)','minmax(195px,1.25fr)'].join(' '):['28px','minmax(205px,1.4fr)',...displayUnlockGroups.value.map(()=>'minmax(175px,1fr)')].join(' '))
const overviewMinWidth=computed(()=>28+210+168+122+168+132+5*12+28)
const latencyMinWidth=computed(()=>28+188+82+96+78+72+sites.value.length*108+(sites.value.length+5)*10+28)
const serviceMinWidth=computed(()=>28+205+displayUnlockGroups.value.length*175+(displayUnlockGroups.value.length+1)*10+28)
const exitMinWidth=computed(()=>28+185+96+84+92+80+76+112+7*10+28)
const shownRange=computed(()=>{
 const total=m.visibleNodes.value.length,size=m.pageSize.value
 if(!total||!size)return `共 ${total} 条`
 const start=(m.currentPage.value-1)*size+1,end=Math.min(start+size-1,total)
 return `第 ${start}–${end} 条，共 ${total} 条`
})
watch(columns,()=>{void nextTick(syncTableHeading)})
const allOnPage=computed(()=>m.pageNodes.value.length>0&&m.pageNodes.value.every(selected))
const scopeAirportNodes=computed(()=>w.proxyNodes.value.filter(n=>w.airportFilter.value==='all'||w.nodeAirportId(n)===w.airportFilter.value))
const regionChips=computed(()=>{
 const counts=new Map<string,number>()
 for(const n of scopeAirportNodes.value){const r=nodeRegion(n);counts.set(r,(counts.get(r)||0)+1)}
 return m.filterRegions.value.map(code=>({code,label:nodeRegionLabel(code),count:counts.get(code)||0}))
})
const allVisibleSelected=computed(()=>m.visibleNodes.value.length>0&&m.visibleNodes.value.every(selected))
const selectedRegions=computed(()=>{
 const raw=w.region.value.startsWith('exclude:')?w.region.value.slice(8):w.region.value
 return raw==='all'?[]:raw.split(',').map(s=>s.trim()).filter(Boolean)
})
const selectedRegionSet=computed(()=>new Set(selectedRegions.value))
const attrTip=ref<InstanceType<typeof NodeAttrPopover>|null>(null)
function showAttrTip(e:MouseEvent|FocusEvent,part:string){
 const info=nodeAttrMeaning(part)
 if(!info){attrTip.value?.clear();return}
 const rect=(e.currentTarget as HTMLElement).getBoundingClientRect()
 attrTip.value?.show({...info,x:rect.left+rect.width/2,y:Math.max(12,rect.top-6)})
}
function clearAttrTip(){attrTip.value?.clear()}
function toggleRegionChip(code:string){
 const next=selectedRegions.value.includes(code)?selectedRegions.value.filter(c=>c!==code):[...selectedRegions.value,code]
 w.region.value=next.length?next.join(','):'all'
}
function toggleExcludeMode(){
 w.excludeRegion.value=!w.excludeRegion.value
}
function keyNav(e:KeyboardEvent){
 const target=e.target as HTMLElement
 if(e.altKey||e.ctrlKey||e.metaKey||target!==table.value&&!target.closest('.hd-node-open'))return
 if(target===table.value&&(e.key==='ArrowRight'||e.key==='ArrowLeft')){e.preventDefault();rowScroller.value?.scrollBy?.({left:e.key==='ArrowRight'?96:-96});return}
 const list=m.pageNodes.value,i=list.findIndex(isFocus)
 const next=e.key==='ArrowDown'?list[Math.min(i+1,list.length-1)]:e.key==='ArrowUp'?list[Math.max(i-1,0)]:e.key==='Home'?list[0]:e.key==='End'?list.at(-1):undefined
 if(next){
  e.preventDefault();m.closeHover();w.latencyReportFocus.value=null;focusKey.value=key(next);if(inspectorOpen.value)startReading(next)
  void nextTick(()=>{
   const row=Array.from(table.value?.querySelectorAll<HTMLElement>('[data-node-key]')||[]).find(row=>row.dataset.nodeKey===key(next))
   row?.querySelector<HTMLButtonElement>('.hd-node-open')?.focus({preventScroll:true});row?.scrollIntoView?.({block:'nearest',inline:'nearest'})
  })
 }else if(e.key==='Enter'&&target===table.value&&focusNode.value){e.preventDefault();focus(focusNode.value)}
}
</script>
<template>
  <section class="hd hd-workbench-layout" :class="{'sidebar-collapsed':sidebarCollapsed}" :style="{'--hd-toolbar-size':toolbarHeight+'px'}">
    <header class="hd-head">
      <div class="hd-title">
        <div class="hd-subtitle-row">
          <button v-if="sidebarCollapsed" type="button" class="hd-sidebar-reveal" title="展开左侧来源与套餐栏" @click="setSidebarCollapsed(false)"><Icon name="panel"/><span>来源 {{chosen.length}}/{{w.airports.value.length}}</span></button>
          <p v-if="filteredAirport"><strong class="hd-head-airport">{{filteredAirport.name}}</strong> · {{filteredAirportNodes}} 个节点 · <button class="hd-link" @click="w.airportFilter.value='all'">看全部 {{chosen.length}} 个机场</button></p>
          <p v-else><button class="hd-link" @click="m.picker.value=true">{{chosen.length?chosen.length+' 个机场':'选择机场'}}</button> · {{chosenNodeCount}} 个节点</p>
          <button v-if="dominantClashUsage" class="hd-clash-usage" :disabled="!dominantClashNode" title="查看正在用的节点；按当前连接累计流量排序，每 30 秒更新" @click="inspectClashUsage"><span class="hd-clash-usage-dot" aria-hidden="true"></span>Clash 正在用：<strong>{{dominantClashName}}</strong><template v-if="dominantClashChains">（经策略组：{{dominantClashChains}}）</template></button>
        </div>
      </div>
    </header>

    <div class="hd-workspace" :class="{'sidebar-collapsed':sidebarCollapsed}">
    <section v-show="!sidebarCollapsed" class="hd-source" :class="{'ledger-expanded':ledgerExpanded,'is-all-airports':w.airportFilter.value==='all'}" aria-label="机场选择">
      <header class="hd-source-heading">
        <div class="hd-source-title">
          <h2>来源与套餐</h2>
          <span v-if="w.airports.value.length" class="hd-source-count">{{chosen.length}}/{{w.airports.value.length}}</span>
        </div>
        <div class="hd-source-head-ops">
          <button v-if="w.airports.value.length>1&&w.airports.value.length<=5" type="button" class="hd-source-icon-btn" :aria-pressed="sidebarSearchOpen" title="搜索机场" @click="sidebarSearchOpen=!sidebarSearchOpen;if(!sidebarSearchOpen)sidebarSearch=''"><Icon name="search"/></button>
          <button type="button" class="hd-source-icon-btn hd-sidebar-collapse-btn" title="收起左侧栏，全宽显示列表" @click="setSidebarCollapsed(true)"><Icon name="panel"/></button>
        </div>
      </header>
      <div v-if="w.airports.value.length>5||sidebarSearchOpen" class="hd-source-search-wrap">
        <input v-model="sidebarSearch" type="search" class="hd-source-search" placeholder="搜索机场…" aria-label="搜索侧栏机场">
      </div>
      <SubscriptionOverview unified :search-query="sidebarSearch" :airports="chosen" :notices="w.notices.value" :airport-id="w.airportFilter.value" @viewed="id=>w.viewAirport(id)">
        <template #additional-actions>
          <div class="hd-source-tools">
            <button class="hd-ledger-toggle hd-link" :aria-expanded="ledgerExpanded" @click="ledgerExpanded=!ledgerExpanded">{{ledgerExpanded?'收起用量明细':'展开用量明细'}}</button>
          </div>
        </template>
      </SubscriptionOverview>
  </section>

    <div ref="workArea" class="hd-results" aria-label="线路对照区">
    <div ref="toolbar" class="hd-toolbar" :class="{'is-overview':overviewMode,'is-specialized':!overviewMode}" :data-mode="exitMode?'exit':w.project.value">
      <div class="hd-toolbar-top">
        <nav class="hd-seg" aria-label="检测项目">
          <template v-for="p in projects" :key="p">
            <button :class="p==='combined'?'hd-seg-overview':'hd-seg-detail'" :aria-pressed="!exitMode&&w.project.value===p" @click="exitMode=false;w.project.value=p"><Icon v-if="p==='combined'" name="grid"/>{{projectNames[p]}}</button>
            <span v-if="p==='combined'" class="hd-seg-divider" aria-hidden="true"/>
          </template>
          <button class="hd-seg-detail" :aria-pressed="exitMode" @click="selectExit">出口与信誉</button>
        </nav>
        <div class="hd-toolbar-actions">
          <button v-if="!matrixMode&&!m.compareOpen.value" class="hd-btn hd-icon-toggle" :aria-pressed="inspectorOpen" :title="inspectorOpen?'收起节点详情，全宽显示列表':'展开节点详情侧栏'" @click="toggleDetails"><Icon name="panel"/></button>
          <button class="hd-btn" :disabled="w.loading.value" title="重新载入节点列表" @click="w.refreshNodes().catch(w.fail)"><Icon name="refresh"/>重新载入</button>
          <button v-if="exitMode" class="hd-btn" :disabled="w.busy.value||!m.runNodes.value.length" title="只解析节点入口地址，不经过节点，不算一次真实检测" @click="w.resolveEntries(m.runNodes.value)">解析入口</button>
          <button v-if="!exitMode&&w.project.value!=='combined'" class="hd-btn hd-run-combined" :disabled="w.busy.value||!m.runNodes.value.length||filterScopePending" title="按当前显示的节点范围打开组合检测" @click="m.openCombinedPlan()">组合检测</button>
          <button class="hd-btn primary hd-run" :disabled="w.busy.value||!m.runNodes.value.length||filterScopePending" @click="run"><Icon name="play"/>{{exitMode?'检测出口与信誉':m.runLabel(w.project.value)}} · {{scopeLabel}}</button>
        </div>
      </div>
      <div class="hd-toolbar-filters">
        <label class="hd-search"><Icon name="search"/><input v-model="w.search.value" type="search" placeholder="搜节点、机场、地区" aria-label="搜索节点、机场或地区" title="可搜索节点名、机场名或地区"></label>
        <UiSelect v-model="w.sort.value" aria-label="排序"><option v-for="s in sorts" :key="s.id" :value="s.id">排序：{{s.label}}</option></UiSelect>
        <UiSelect v-model.number="w.hours.value" aria-label="时间范围"><option :value="6">最近 6 小时</option><option :value="24">最近 24 小时</option><option :value="168">最近 7 天</option><option :value="720">最近 30 天</option></UiSelect>
        <details ref="more" class="hd-more" :class="{'has-active':moreActive>0}" @keydown.esc="closeMore"><summary ref="moreSummary">更多筛选<small>{{filterSummary?filterSummary+' · ':''}}{{m.qualityFiltersPending.value?'范围确认中':'符合 '+m.visibleNodes.value.length+' 个'}}</small></summary>
        <div class="hd-more-groups">
          <section class="hd-more-group" aria-labelledby="more-scope">
            <h4 id="more-scope">看哪些节点<small>按来源和协议缩小列表</small></h4>
            <div class="hd-more-grid">
              <label>机场<UiSelect v-model="w.airportFilter.value" aria-label="机场筛选"><option value="all">全部机场</option><option v-for="a in chosen" :key="a.id" :value="a.id">{{a.name}}</option></UiSelect></label>
              <label v-if="m.filterProfiles.value.length>1">订阅<UiSelect v-model="w.subscription.value" aria-label="订阅筛选"><option value="all">全部订阅</option><option v-for="[id,name] in m.filterProfiles.value" :key="id" :value="id">{{name}}</option></UiSelect></label>
              <label>协议<UiSelect v-model="m.protocolFilter.value" aria-label="协议筛选"><option value="all">全部协议</option><option v-for="p in m.protocolChoices.value" :key="p" :value="p">{{p}}</option></UiSelect></label>
              <label v-if="w.sort.value==='airport'&&chosen.length>1&&w.airportFilter.value==='all'">优先机场<UiSelect v-model="w.priorityAirportId.value" aria-label="优先机场"><option value="all">机场自然顺序</option><option v-for="a in chosen" :key="a.id" :value="a.id">{{a.name}}优先</option></UiSelect></label>
              <label v-if="w.region.value!=='all'" class="hd-check"><input v-model="w.excludeRegion.value" type="checkbox">反过来：排除「{{selectedRegions.map(nodeRegionLabel).join(' / ')}}」</label>
            </div>
  </section>
          <section class="hd-more-group" aria-labelledby="more-result">
            <h4 id="more-result">按{{w.project.value==='combined'?'检测':projectNames[w.project.value]}}结果<small>{{resultHint}}</small></h4>
            <div class="hd-more-grid">
              <label v-if="tabGates.length>2">只看<UiSelect v-model="m.qualityGate.value" aria-label="质量证据筛选"><option v-for="g in tabGates" :key="g.id" :value="g.id">{{g.label}}</option></UiSelect></label>
              <label v-else-if="tabGates.length===2" class="hd-check"><input type="checkbox" :checked="m.qualityGate.value===tabGates[1].id" @change="m.qualityGate.value=($event.target as HTMLInputElement).checked?tabGates[1].id:'all'">只看{{tabGates[1].short}}</label>
              <label v-if="!exitMode&&w.project.value==='download'" class="hd-check"><input v-model="w.downloadFullOnly.value" type="checkbox">只看达到结束条件的测速</label>
              <template v-if="latencyTab">
                <label>延迟排序<UiSelect v-model="m.qualitySort.value" aria-label="质量分组排序"><option v-for="q in qualitySorts" :key="q.id" :value="q.id">{{q.label}}</option></UiSelect></label>
                <label>延迟按哪个站点算<UiSelect v-model="w.displayTarget.value" aria-label="质量统计站点"><option v-for="s in targets" :key="s.id" :value="s.id">{{s.name}}</option></UiSelect></label>
              </template>
            </div>
            <p class="hd-muted hd-more-note">数据截止 {{date(m.qualityGate.value==='response'?weeklyProgress.until:m.qualityCutoff.value)}} · 各项分别测量，不作综合打分<button class="hd-link" :disabled="m.qualityGate.value==='response'?weeklyProgress.processed<weeklyProgress.total:m.qualityLoading.value" @click="m.qualityGate.value==='response'?m.refreshWeekly():m.readQualityNodes(qualityFiltering?w.proxyNodes.value:m.pageNodes.value,true)">{{m.qualityLoading.value?'读取中…':'更新质量记录'}}</button></p>
            <p v-if="qualityFiltering" class="hd-muted" role="status">{{m.qualityGate.value==='response'?(m.qualityFiltersPending.value?'正在读取近 7 天延迟历史…':`按近 7 天采样响应率 ≥90% 筛选 · 当前符合 ${m.visibleNodes.value.length} 个`):m.allQualityReady.value?`已读固定范围 ${w.proxyNodes.value.length} 个节点 · 当前符合 ${m.visibleNodes.value.length} 个 · 读取失败 ${Object.keys(m.qualityErrors.value).length} 个 · 存在缺页/读取错误 ${Object.values(m.qualityData.value).filter(e=>!e.complete).length} 个（只限制对应类型的证据）`:`正在读取全部 ${w.proxyNodes.value.length} 个节点的历史记录（已读 ${Object.keys(m.qualityData.value).length}）；读取完成后应用条件筛选与排序`}}</p>
  </section>
        </div>
        </details>
      </div>
      <div class="hd-view-options">
        <div v-if="scopeAirportNodes.length" class="hd-region-strip" role="group" aria-label="按地区快选">
          <button type="button" class="hd-region-chip all" :aria-pressed="w.region.value==='all'&&!w.excludeRegion.value" @click="w.excludeRegion.value=false;w.region.value='all'">全部<small>{{scopeAirportNodes.length}}</small></button>
          <button v-for="rc in regionChips" :key="rc.code" type="button" class="hd-region-chip" :class="[regionBadgeClass(rc.code),{excluded:w.excludeRegion.value&&selectedRegionSet.has(rc.code)}]" :aria-pressed="selectedRegionSet.has(rc.code)" @click="toggleRegionChip(rc.code)">{{w.excludeRegion.value&&selectedRegionSet.has(rc.code)?`排除 ${rc.label}`:rc.label}}<small>{{rc.count}}</small></button>
          <button type="button" class="hd-region-chip exclude-toggle" :aria-pressed="w.excludeRegion.value" title="开启后，点选的地区将被排除；也可以先选地区再点此反选排除" @click="toggleExcludeMode">排除所选</button>
          <span class="hd-quick-divider" aria-hidden="true"/>
          <button type="button" class="hd-quick-select" :disabled="!m.visibleNodes.value.length" :title="allVisibleSelected?'取消勾选当前筛选出来的节点':'一键勾选当前筛选出来的全部节点'" @click="w.toggleAll(m.visibleNodes.value)">{{allVisibleSelected?'取消全选当前':`勾选当前 ${m.visibleNodes.value.length} 个`}}</button>
          <button v-if="w.selectedNodes.value.length" type="button" class="hd-quick-select clear" @click="w.cancelSelection()">清空已选 {{w.selectedNodes.value.length}}</button>
        </div>
        <div v-if="showSites" class="hd-view-tail">
          <p :id="readingHintId" class="hd-reading-hint">竖条按采样顺序 · 点站点或整轮历史回看 · 虚线框为监测</p>
          <SiteDisplayPicker v-model="w.displayTargets.value"/>
        </div>
      </div>
    </div>
    <p v-if="['latency','download','service'].includes(w.sort.value)" class="hd-muted hd-sort-status" role="status">{{w.sortLoading.value?`读取全部节点 ${w.sortReadCount.value}/${w.proxyNodes.value.length}…`:w.sortError.value||(w.sortAt.value?'排序截止 '+date(w.sortAt.value)+' · 翻页不改变排名':'尚未读取全部节点，仅按已读记录排序')}}<button class="hd-link" :disabled="w.sortLoading.value" @click="w.refreshSort()">更新排序</button></p>
    <p v-if="!exitMode&&weeklyProgress.total&&(weeklyProgress.full||weeklyProgress.failed)" class="hd-muted hd-sort-status" role="status">
      {{weeklyProgress.processed<weeklyProgress.total?`正在读取近 7 天记录 ${weeklyProgress.processed}/${weeklyProgress.total}`:`近 7 天记录已读 ${weeklyProgress.processed-weeklyProgress.failed}/${weeklyProgress.total} · 截至 ${date(weeklyProgress.until)}`}}
      <template v-if="weeklyProgress.failed"> · {{weeklyProgress.failed}} 个节点读取失败</template>
      <button v-if="weeklyProgress.processed===weeklyProgress.total" class="hd-link" @click="m.refreshWeekly()">更新七天记录</button>
    </p>
    <p v-if="m.headerSort.value&&!m.headerSortReady.value&&!['responseRate','volatility'].includes(m.headerSort.value)" class="hd-muted hd-sort-status" role="status">正在读取排序所需的历史…</p>

    <div v-if="!exitMode&&w.project.value==='service'" class="hd-service-subbar">
      <nav class="hd-seg matrix-switch" aria-label="解锁结果视图"><button :aria-pressed="serviceView==='nodes'" @click="serviceView='nodes'">按节点</button><button :aria-pressed="serviceView==='services'" @click="serviceView='services'">按服务</button></nav>
      <details class="hd-services"><summary>选择解锁项目<small>{{m.displayServices.value.length?'已选 '+m.displayServices.value.length+' 项':'还没选'}}</small></summary><UnlockPicker v-model="w.serviceIds.value" :catalog="catalog"/></details>
    </div>
    <UnlockServiceMatrix v-if="matrixMode" @inspect="n=>{serviceView='nodes';focus(n)}"/>
    <div v-if="!matrixMode" class="hd-list-stage" :class="{'attached':detailsOverlay||!inspectorOpen}">
    <nav class="hd-list-pagination" aria-label="节点列表分页">
      <span class="hd-list-count">{{shownRange}}</span>
      <div class="hd-pages">
        <label class="hd-page-size">每页
          <UiSelect :model-value="m.pageSize.value" aria-label="每页显示节点数" @update:model-value="m.setPageSize(Number($event))"><option :value="10">10 条</option><option :value="20">20 条</option><option :value="50">50 条</option><option :value="100">100 条</option><option :value="0">全部</option></UiSelect>
        </label>
        <button class="hd-btn" :disabled="m.currentPage.value<=1" aria-label="上一页" @click="changePage(-1)">上一页</button>
        <strong aria-live="polite">{{m.currentPage.value}} / {{m.totalPages.value}} 页</strong>
        <button class="hd-btn" :disabled="m.currentPage.value>=m.totalPages.value" aria-label="下一页" @click="changePage(1)">下一页</button>
      </div>
    </nav>
    <div class="hd-body" :class="{'overview-mode':overviewMode,'latency-mode':showSites,'download-mode':!exitMode&&w.project.value==='download','service-mode':!exitMode&&w.project.value==='service','exit-mode':exitMode,'full-table':detailsOverlay||!inspectorOpen}" :style="{'--hd-side-w':visibleSideWidth+'px','--hd-overview-width':overviewMinWidth+'px','--hd-latency-width':latencyMinWidth+'px','--hd-service-width':serviceMinWidth+'px','--hd-exit-width':exitMinWidth+'px'}">
      <div ref="table" class="hd-table" role="table" aria-label="节点列表" :aria-describedby="latencyTab?readingHintId:undefined" tabindex="0" :style="{'--hd-cols':columns}" @keydown="keyNav">
        <div class="hd-table-head" role="rowgroup">
        <div class="hd-tr hd-th" role="row">
          <span role="columnheader"><input type="checkbox" :checked="allOnPage" aria-label="选择本页全部节点" @change="w.toggleAll(m.pageNodes.value)"></span>
          <WorkbenchSortHeader column="name" label="节点"/><template v-if="exitMode"><template v-for="label in exitColumns" :key="label"><WorkbenchSortHeader v-if="label==='CF 盾'" column="cfChallenge" label="CF 盾"/><WorkbenchSortHeader v-else-if="label==='纯净度'" column="purity" label="纯净度"/><span v-else role="columnheader">{{label}}</span></template></template>
          <template v-if="overviewMode">
            <WorkbenchSortHeader column="verdict" label="延迟总览"/>
            <WorkbenchSortHeader column="download" label="下载速度"/>
            <WorkbenchSortHeader column="service" label="服务解锁"/>
            <WorkbenchSortHeader column="purity" label="出口画像"/>
          </template>
          <template v-if="showSites">
            <WorkbenchSortHeader column="verdict" label="结论"/>
            <WorkbenchSortHeader column="purity" label="纯净度"/>
            <WorkbenchSortHeader column="responseRate" label="7 天响应率"/>
            <WorkbenchSortHeader column="volatility" label="波动幅度"/>
            <WorkbenchSortHeader v-for="s in sites" :key="s.id" class="hd-station-head" :column="'site:'+s.id" :label="s.name"/>
          </template>
          <template v-if="!exitMode&&w.project.value==='download'">
            <WorkbenchSortHeader column="download" label="有效速度"/>
            <span role="columnheader">峰值 / 全程</span>
            <WorkbenchSortHeader column="downloadTime" label="测速源与记录"/>
          </template>
          <template v-if="!exitMode&&w.project.value==='service'"><WorkbenchSortHeader v-for="g in displayUnlockGroups" :key="g.id" :column="'unlock:'+g.id" :label="g.name"/></template>
        </div>
        </div>
        <div ref="rowScroller" class="hd-row-scroll" role="rowgroup" @scroll="syncTableHeading">
        <div v-for="n in m.pageNodes.value" :key="key(n)" v-memo="[exitMode?'exit':w.project.value,isFocus(n)&&(inspectorOpen||!!focusKey),selected(n),isClashUsing(n),nodeRowMeta[key(n)],overviewMode?overviewLatencyMap[key(n)]:undefined,overviewMode||(!exitMode&&w.project.value==='download')?downloadRows[key(n)]:undefined,overviewMode?overviewUnlockMap[key(n)]:undefined,overviewMode||exitMode||showVerdict?exitRows[key(n)]:undefined,showVerdict&&!overviewMode?verdicts[key(n)]:undefined,showSites?weeklyRowMap[key(n)]:undefined,showSites?wholeLatencyHistory[key(n)]:undefined,showSites?latencyReviewFor(n):undefined,showSites?w.displayTargets.value.length:0,showServiceGroups?serviceGroupMap[key(n)]:undefined,!exitMode&&w.project.value==='download'?w.busy.value:false]" :data-node-key="key(n)" class="hd-tr" role="row" :class="{focus:isFocus(n)&&(inspectorOpen||!!focusKey),selected:selected(n),'clash-using':isClashUsing(n)}" :aria-selected="selected(n)" @click="focus(n)">
          <span role="cell" @click.stop><input type="checkbox" :checked="selected(n)" :aria-label="'选择 '+nodeRowMeta[key(n)].displayName" @change="w.toggleNode(n)"></span>
          <span role="cell" class="hd-node" :class="nodeRowMeta[key(n)].regionClass" :title="n.display_name+' · '+nodeRowMeta[key(n)].fullSource+' · '+n.type"><span><div class="hd-node-title"><span class="hd-flag" :class="nodeRowMeta[key(n)].regionClass">{{n.country_code||'—'}}</span><span v-if="isClashUsing(n)" class="hd-clash-active-badge" aria-label="Clash 正在使用" title="Clash 正在使用"><i class="hd-clash-node-dot" aria-hidden="true"/>在用</span><button class="hd-node-open" :tabindex="isFocus(n)?0:-1" :aria-label="'查看 '+nodeRowMeta[key(n)].displayName+' · '+nodeRowMeta[key(n)].fullSource" @focus="focusKey=key(n)" @click.stop="focus(n)"><strong><span class="hd-node-region">{{nodeRowMeta[key(n)].region}}</span><template v-for="(part,idx) in nodeRowMeta[key(n)].parts" :key="idx"><span class="hd-node-sep"> · </span><span class="hd-node-part" :class="part.kind" :title="part.hintDetail" @mouseenter="showAttrTip($event,part.text)" @mouseleave="clearAttrTip">{{part.text}}</span></template></strong></button></div><small><span class="hd-source-label">{{nodeRowMeta[key(n)].source}}</span> · <b class="hd-proto-tag" :class="nodeRowMeta[key(n)].protoClass">{{n.type}}</b></small>
            <span v-if="showSites&&wholeLatencyHistory[key(n)]?.length" class="hd-round-history"><span>整轮历史</span><LatencyHistoryMarks class="hd-round-marks" :node="n" :points="wholeLatencyHistory[key(n)]" target="all" label="整轮历史" :selected="latencyReviewFor(n)?.target==='all'?latencyReviewFor(n)?.point:undefined" @select="onWholeHistorySelect"/></span>
            <span v-if="showSites&&latencyReviewFor(n)" class="hd-round-review"><span>回看 {{date(latencyReviewFor(n)?.point.time)}}</span><button type="button" class="hd-link" @click.stop="resetLatencyReview(n)">回到最新</button></span>
          </span></span>
          <template v-if="overviewMode">
            <span role="cell" class="hd-ov-cell" @mouseenter="showOverviewLatencyTip($event,n)" @mouseleave="hideChartTip">
              <template v-if="overviewLatencyMap[key(n)].measured">
                <span class="hd-ov-main"><b class="hd-pill" :class="[overviewLatencyMap[key(n)].verdict.tone,{'partial-sites':overviewLatencyMap[key(n)].verdict.label.startsWith('部分站点不通')}]">{{overviewLatencyMap[key(n)].verdict.label}}</b><strong v-if="overviewLatencyMap[key(n)].ms" class="hd-ov-val">{{overviewLatencyMap[key(n)].ms}}</strong></span>
                <span v-if="overviewLatencyMap[key(n)].sub||overviewLatencyMap[key(n)].anomalies.length" class="hd-ov-meta-row">
                  <small v-if="overviewLatencyMap[key(n)].sub" class="hd-ov-sub">{{overviewLatencyMap[key(n)].sub}}</small>
                  <span v-for="a in overviewLatencyMap[key(n)].anomalies.slice(0,3)" :key="a.label" class="hd-ov-tag" :class="a.tone">{{a.label}}</span>
                  <span v-if="overviewLatencyMap[key(n)].anomalies.length>3" class="hd-ov-tag empty">+{{overviewLatencyMap[key(n)].anomalies.length-3}}</span>
                </span>
              </template>
              <span v-else class="hd-empty-pill">未测延迟</span>
            </span>
            <span role="cell" class="hd-ov-cell">
              <template v-if="downloadRows[key(n)]?.val!==null">
                <span class="hd-ov-main"><strong class="hd-ov-val" :class="downloadRows[key(n)].tone">{{downloadRows[key(n)].metric}}</strong></span>
                <span class="hd-speed-bar"><i :class="downloadRows[key(n)].tone" :style="{width:downloadRows[key(n)].pct+'%'}"/></span>
                <small class="hd-ov-sub">{{[downloadRows[key(n)].source,downloadRows[key(n)].label].filter(Boolean).join(' · ')}}</small>
              </template>
              <span v-else-if="downloadRows[key(n)].tone!=='empty'&&downloadRows[key(n)].metric!=='未测'" class="hd-ov-main"><b class="hd-pill" :class="downloadRows[key(n)].tone">{{downloadRows[key(n)].metric}}</b></span>
              <span v-else class="hd-empty-pill">未测速</span>
            </span>
            <span role="cell" class="hd-ov-cell" @mouseenter="showOverviewUnlockTip($event,n)" @mouseleave="hideChartTip">
              <template v-if="overviewUnlockMap[key(n)].groups.length">
                <span class="hd-ov-chips">
                  <span v-for="g in overviewUnlockMap[key(n)].groups" :key="g.id" class="hd-ov-chip" :class="g.tone" :title="g.title">{{g.name}} {{g.total?g.label:'待确认'}}</span>
                </span>
                <span v-if="overviewUnlockMap[key(n)].items.length" class="hd-ov-svc-list">
                  <button v-for="item in overviewUnlockMap[key(n)].items.slice(0,3)" :key="item.id" type="button" class="hd-ov-svc-chip" :class="item.tone" :title="item.name+'：'+item.result.label+(item.result.reason?' · '+item.result.reason:'')" @click.stop="hideChartTip();m.showService(n,item.id)"><b>{{item.shortName}}</b><span>{{item.shortStatus}}</span></button>
                  <span v-if="overviewUnlockMap[key(n)].items.length>3" class="hd-ov-tag empty">+{{overviewUnlockMap[key(n)].items.length-3}}</span>
                </span>
              </template>
              <span v-else class="hd-empty-pill">未测解锁</span>
            </span>
            <span role="cell" class="hd-ov-cell">
              <span v-if="exitRows[key(n)].purity.score!==null||exitRows[key(n)].cfChallenge.tone!=='empty'||exitRows[key(n)].exitKnown" class="hd-ov-chips">
                <span v-if="exitRows[key(n)].purity.score!==null" class="hd-ov-chip" :class="exitRows[key(n)].purity.tone" :title="exitRows[key(n)].purity.title">{{exitRows[key(n)].purity.grade}} {{exitRows[key(n)].purity.score}} · {{exitRows[key(n)].purity.typeLabel}}</span>
                <span v-else-if="exitRows[key(n)].exitKnown" class="hd-ov-chip" :class="exitRows[key(n)].tone" :title="exitRows[key(n)].detail">{{exitRows[key(n)].label}}</span>
                <span v-if="exitRows[key(n)].cfChallenge.tone!=='empty'" class="hd-ov-chip" :class="exitRows[key(n)].cfChallenge.tone" :title="exitRows[key(n)].cfChallenge.detail">CF {{exitRows[key(n)].cfChallenge.shortLabel}}</span>
              </span>
              <span v-else class="hd-empty-pill">未测出口</span>
            </span>
          </template>
          <template v-if="exitMode"><template v-for="field in exitFields" :key="field"><span v-if="field==='line'" role="cell" class="hd-exit-cell hd-line-cell"><span class="hd-line-badge" :title="exitRows[key(n)].line">{{exitRows[key(n)].line}}</span><span v-if="exitRows[key(n)].route" class="hd-route-tag" :class="exitRows[key(n)].routeTone" :title="exitRows[key(n)].routeTitle">{{exitRows[key(n)].route}}</span></span><span v-else-if="field==='ip'" role="cell" class="hd-exit-cell"><code v-if="exitRows[key(n)].ip&&exitRows[key(n)].ip!=='—'" class="hd-ip-code">{{exitRows[key(n)].ip}}</code><span v-else class="hd-empty-pill">待查询</span></span><span v-else-if="field==='ipTag'" role="cell" class="hd-exit-cell hd-ip-cell"><span class="hd-ip-tag" :class="exitRows[key(n)].ipKind.tone" :title="'IP 类型：'+exitRows[key(n)].ipKind.detail">{{exitRows[key(n)].ipTag}}</span></span><span v-else-if="field==='cfChallenge'" role="cell" class="hd-exit-cell hd-cf-cell"><span class="hd-cf-tag" :class="exitRows[key(n)].cfChallenge.tone" :title="exitRows[key(n)].cfChallenge.label+' · '+exitRows[key(n)].cfChallenge.detail">{{exitRows[key(n)].cfChallenge.shortLabel}}</span></span><span v-else-if="field==='purity'" role="cell" class="hd-exit-cell hd-purity-cell" :class="exitRows[key(n)].purity.tone" :title="exitRows[key(n)].purity.title+(exitRows[key(n)].riskLabel!=='未测'?' · 风险峰值 '+exitRows[key(n)].riskLabel:'')"><template v-if="exitRows[key(n)].purity.score!==null"><span class="hd-purity-top"><b class="hd-purity-badge" :class="exitRows[key(n)].purity.tone">{{exitRows[key(n)].purity.grade}} {{exitRows[key(n)].purity.score}}</b></span><span class="hd-purity-bar" aria-hidden="true"><i :class="exitRows[key(n)].purity.tone" :style="{width:Math.max(8,exitRows[key(n)].purity.score!)+'%'}"/></span><small v-if="exitRows[key(n)].riskLabel!=='未测'" class="hd-exit-purity-sub">{{exitRows[key(n)].riskLabel}}</small></template><span v-else-if="exitRows[key(n)].riskLabel!=='未测'" class="hd-exit-pill" :class="exitRows[key(n)].riskTone">{{exitRows[key(n)].riskLabel}}</span><span v-else class="hd-empty-pill">未测</span></span><span v-else role="cell" class="hd-exit-cell"><span class="hd-exit-pill" :class="exitRows[key(n)].tone" :title="exitRows[key(n)].detail">{{exitRows[key(n)].label==='未测'?'待检测':exitRows[key(n)].label}}</span></span></template></template>
          <span v-if="showVerdict&&!overviewMode" role="cell"><b class="hd-pill" :class="[verdicts[key(n)].tone,{'partial-sites':verdicts[key(n)].label.startsWith('部分站点不通')}]">{{verdicts[key(n)].label}}</b></span>
          <span v-if="showVerdict&&!overviewMode" role="cell" class="hd-num hd-purity-cell" :class="exitRows[key(n)].purity.tone" :title="exitRows[key(n)].purity.title">
            <template v-if="exitRows[key(n)].purity.score!==null">
              <span class="hd-purity-top"><b class="hd-purity-badge" :class="exitRows[key(n)].purity.tone">{{exitRows[key(n)].purity.grade}} {{exitRows[key(n)].purity.score}}</b><small class="hd-purity-type">{{exitRows[key(n)].purity.typeLabel}}</small></span>
              <span class="hd-purity-bar" aria-hidden="true"><i :class="exitRows[key(n)].purity.tone" :style="{width:Math.max(8,exitRows[key(n)].purity.score!)+'%'}"/></span>
            </template>
            <template v-else>—</template>
          </span>
          <template v-if="showSites"><span role="cell" class="hd-num hd-history-cell" :class="weeklyRowMap[key(n)].respTone" :title="weeklyRowMap[key(n)].respTitle">{{weeklyRowMap[key(n)].respText}}<small v-if="weeklyRowMap[key(n)].old" class="old-tag">旧</small></span><span role="cell" class="hd-num hd-history-cell" :class="weeklyRowMap[key(n)].volTone" :title="weeklyRowMap[key(n)].volTitle">{{weeklyRowMap[key(n)].volText}}<small v-if="weeklyRowMap[key(n)].old" class="old-tag">旧</small></span></template>
          <NodeResponseStrip v-if="showSites" :node="n" detailed :review="latencyReviewFor(n)" @round="openLatencyRound" @history="reviewLatencyRound"/>
          <template v-if="!exitMode&&w.project.value==='download'">
            <span role="cell" class="hd-num hd-dl-cell" :class="downloadRows[key(n)].tone" :title="downloadRows[key(n)].context">
              <template v-if="downloadRows[key(n)]?.val!==null">
                <strong class="hd-dl-val">{{downloadRows[key(n)].metric}}</strong>
                <span class="hd-speed-bar"><i :class="downloadRows[key(n)].tone" :style="{width:downloadRows[key(n)].pct+'%'}"/></span>
              </template>
              <b v-else-if="downloadRows[key(n)].tone!=='empty'&&downloadRows[key(n)].metric!=='未测'" class="hd-pill" :class="downloadRows[key(n)].tone">{{downloadRows[key(n)].metric}}</b>
              <span v-else class="hd-empty-pill">未测速</span>
            </span>
            <span role="cell" class="hd-dl-secondary">
              <span v-if="downloadRows[key(n)]?.secondary" class="hd-ov-chip empty">{{downloadRows[key(n)].secondary}}</span>
              <span v-else class="hd-empty-pill">—</span>
            </span>
            <span role="cell" class="hd-dl-context">
              <span :class="downloadRows[key(n)]?.hasAttempt?'hd-dl-meta':'hd-muted'">{{downloadRows[key(n)].context}}</span>
              <button type="button" class="hd-row-quick" :disabled="w.busy.value" @click.stop="quickPlanSingle(n,'download')">测速</button>
            </span>
          </template>
          <template v-if="showServiceGroups">
            <div v-for="g in serviceGroupMap[key(n)]" :key="g.id" role="cell" class="hd-svc-cell hd-num" :class="[g.tone,{'hd-svc-cell-rich':g.attemptedItems.length>0}]" :title="g.title" @mouseenter="showServiceGroupTip($event,n,g)" @mouseleave="hideChartTip" @click.stop="hideChartTip();focus(n)">
              <template v-if="g.attemptedItems.length">
                <div class="hd-svc-head-row">
                  <span class="hd-svc-pill compact" :class="g.tone"><i aria-hidden="true"/>{{g.total?g.label:'待确认'}}</span>
                  <span v-if="g.total>=2" class="hd-svc-bar" aria-hidden="true"><i :class="g.tone" :style="{width:Math.round(g.available/g.total*100)+'%'}"/></span>
                </div>
                <div class="hd-svc-items">
                  <button v-for="item in g.attemptedItems.slice(0,6)" :key="item.id" type="button" class="hd-svc-item" :class="item.tone" :title="item.name+'：'+item.result.label+(item.result.reason?' · '+item.result.reason:'')" @click.stop="hideChartTip();m.showService(n,item.id)"><b>{{item.shortName}}</b><small>{{item.shortStatus}}</small></button>
                  <span v-if="g.attemptedItems.length>6" class="hd-ov-tag empty">+{{g.attemptedItems.length-6}}</span>
                </div>
              </template>
              <span v-else class="hd-svc-pill" :class="g.tone"><i aria-hidden="true"/>{{g.label}}</span>
            </div>
          </template>
        </div>
        <div v-if="!m.pageNodes.value.length" class="hd-empty"><Icon name="layers"/><strong>{{!w.airports.value.length?'先添加一份机场订阅':!chosen.length?'先选择一个机场':hasSourceNodes?'节点被筛选条件隐藏了':'这个机场还没有节点'}}</strong><button class="hd-btn" @click="!w.airports.value.length?w.page.value='airports':!chosen.length?m.picker.value=true:hasSourceNodes?clearFilters():w.page.value='airports'">{{!w.airports.value.length?'添加机场':!chosen.length?'选择机场':hasSourceNodes?'清除筛选':'管理订阅'}}</button></div>
        </div>
      </div>
      <template v-if="!detailsOverlay">
        <div v-if="inspectorOpen" class="hd-resize" role="separator" aria-orientation="vertical" aria-label="调整检查器宽度" :aria-valuenow="visibleSideWidth" :aria-valuemin="SIDE_MIN" :aria-valuemax="maxSideWidth" tabindex="0" @pointerdown="startResize" @keydown.left.prevent="setSide(visibleSideWidth+24)" @keydown.right.prevent="setSide(visibleSideWidth-24)" @dblclick="setSide(420)"/>
        <NodeInspector v-if="readingNode&&inspectorOpen" :key="key(readingNode)+readingNode.node_key" class="hd-side" :node="readingNode" :exit-only="exitMode" :clash-usage="clashUsage" :read-only="!readingCurrent" :context-notice="readingNotice" @collapse="closeDetails" @select-project="selectInspectorProject" @refresh-clash-usage="refreshClashUsage"><template #navigation><NodeReaderNav :index="readingIndex" :count="readingNodes.length" :selected-count="w.selectedNodes.value.length" :previous="readerLabel(-1)" :next="readerLabel(1)" @previous="moveReading(-1)" @next="moveReading(1)" @compare="m.openComparison()"/></template></NodeInspector>
      </template>
    </div>
    </div>

    <Modal v-if="!matrixMode&&(m.compareOpen.value||detailsOverlay&&readingNode&&inspectorOpen)" :key="m.compareOpen.value?'comparison':'node-reader'" :title="m.compareOpen.value?'比较所选节点':'节点详情'" wide :variant="m.compareOpen.value?'workbench-detail-modal comparison-modal':'workbench-detail-modal node-reader-modal'" @close="closeDetails">
      <template #header-tools><NodeReaderNav v-if="!m.compareOpen.value" :index="readingIndex" :count="readingNodes.length" :selected-count="w.selectedNodes.value.length" :previous="readerLabel(-1)" :next="readerLabel(1)" @previous="moveReading(-1)" @next="moveReading(1)" @compare="m.openComparison()"/></template>
      <div ref="readerBody" class="hd hd-detail-sheet">
        <NodeCompare v-if="m.compareOpen.value"/>
        <NodeInspector v-else-if="readingNode" :key="key(readingNode)+readingNode.node_key" :node="readingNode" :exit-only="exitMode" :clash-usage="clashUsage" :read-only="!readingCurrent" :context-notice="readingNotice" autofocus-title @collapse="closeDetails" @select-project="selectInspectorProject" @refresh-clash-usage="refreshClashUsage"/>
      </div>
    </Modal>

    <div v-if="w.selectedKeys.value.length" class="hd-selection" aria-live="polite">
      <strong>已选 {{w.selectedNodes.value.length}} 个节点</strong><small v-if="m.hiddenSelectedCount.value" class="hd-muted">含 {{m.hiddenSelectedCount.value}} 个筛选外节点</small>
      <button class="hd-link" :disabled="m.qualityFiltersPending.value" @click="w.toggleAll(m.visibleNodes.value)">{{allVisibleSelected?'取消范围选择':'全选筛选范围'}}</button>
      <button class="hd-btn" :disabled="w.selectedNodes.value.length<2" @click="m.openComparison()">比较已选 {{w.selectedNodes.value.length}} 个节点</button>
      <button class="hd-btn primary" :disabled="w.busy.value" @click="exitMode?w.openReputationPlan():w.openPlan()"><Icon name="play"/>检测已选</button>
      <button class="hd-btn" :disabled="w.busy.value||!w.selectedNodes.value.length" title="只解析节点入口地址，不经过节点，不算一次真实检测" @click="w.resolveEntries(w.selectedNodes.value)">解析入口</button>
      <button class="hd-btn" @click="w.monitorNodes.value=w.selectedNodes.value.map(n=>({...n}))"><Icon name="monitor"/>持续监测延迟</button>
      <button class="hd-btn" @click="w.exportSelection().catch(w.fail)">导出配置</button>
      <button class="hd-link" @click="w.selectedKeys.value=[]">清空已选</button>
    </div>

    <footer v-if="!matrixMode" class="hd-foot">
      <span>{{m.visibleNodes.value.length}} 个节点<template v-if="w.selectedKeys.value.length"> · 已选 {{w.selectedKeys.value.length}}</template> · ↑↓ 浏览 · Enter 查看 <button class="hd-link hd-select-range" :disabled="m.qualityFiltersPending.value||!m.visibleNodes.value.length" @click="w.toggleAll(m.visibleNodes.value)">{{allVisibleSelected?'取消范围选择':'全选筛选范围'}}</button></span>
      <span class="hd-legend"><span class="good">正常</span><span class="warn">部分</span><span class="bad">失败</span><span class="empty">未测</span></span>
      <nav v-if="m.totalPages.value>1" class="hd-pages" aria-label="列表底部分页"><button class="hd-btn" :disabled="m.currentPage.value<=1" @click="changePage(-1,true)">上一页</button><strong>{{m.currentPage.value}} / {{m.totalPages.value}} 页</strong><button class="hd-btn" :disabled="m.currentPage.value>=m.totalPages.value" @click="changePage(1,true)">下一页</button></nav>
    </footer>
    </div>
    </div>
    <NodeAttrPopover ref="attrTip"/>
  </section>
</template>

<style scoped>
.hd-pill.partial-sites{max-width:100%;white-space:normal;line-height:1.4;border-radius:8px;box-sizing:border-box}
.history-sort{font-size:11px;white-space:nowrap;text-align:left}
.old-tag{margin-left:2px;padding:0 2px;border-radius:2px;background:var(--hd-soft);color:var(--hd-muted);font-size:9px;font-weight:600}
.hd-clash-usage{display:inline-flex;align-items:center;gap:7px;padding:3px 11px;border:1px solid color-mix(in srgb,var(--hd-good) 24%,transparent);border-radius:999px;background:color-mix(in srgb,var(--hd-good) 9%,var(--cst-panel-solid));font-size:11.5px;font-weight:550;line-height:1.45;color:color-mix(in srgb,var(--hd-good) 85%,var(--hd-text));box-shadow:var(--cst-shadow-xs);transition:border-color .15s,background .15s,transform .14s}.hd-clash-usage{cursor:pointer}.hd-clash-usage:hover{border-color:color-mix(in srgb,var(--hd-good) 52%,transparent);background:color-mix(in srgb,var(--hd-good) 14%,var(--cst-panel-solid));transform:translateY(-.5px)}.hd-clash-usage strong{color:var(--hd-text)}.hd-clash-usage-dot,.hd-clash-node-dot{display:inline-block;flex:none;width:6px;height:6px;border-radius:50%;background:var(--hd-good);box-shadow:0 0 0 3px color-mix(in srgb,var(--hd-good) 22%,transparent)}.hd-clash-active-badge{display:inline-flex;align-items:center;gap:4px;flex:none;padding:1px 7px;border-radius:999px;font-size:10px;font-weight:700;color:var(--hd-good);background:var(--hd-good-bg);border:1px solid color-mix(in srgb,var(--hd-good) 28%,transparent);line-height:1.45}
.hd-body.exit-mode .hd-tr{padding-top:9px;padding-bottom:9px}.hd-body.exit-mode .hd-exit-cell.good{color:var(--hd-good)}.hd-exit-cell{font-size:11px;white-space:normal;line-height:1.4}.hd-exit-cell.bad{color:var(--hd-bad);font-weight:600}.hd-exit-cell.warn{color:var(--hd-warn)}
.hd .hd-svc-cell.empty{color:var(--hd-empty);font-weight:400}.hd .hd-svc-cell.warn{color:var(--hd-warn)}
</style>

<style scoped>
.history-sort{font-size:11px;white-space:nowrap;text-align:left}.matrix-switch{display:inline-flex;margin:0 0 12px}
.hd-line-cell{display:flex;align-items:flex-start;gap:6px;font-size:11px;line-height:1.45;white-space:normal}
.hd-line-text{min-width:0;color:var(--hd-text,inherit)}
.hd-route-tag{flex:none;margin-top:1px;padding:1px 7px;border-radius:999px;border:1px solid var(--hd-line);color:var(--hd-muted);background:var(--hd-panel-2);font-size:10px;font-weight:600;line-height:1.45;white-space:nowrap}
.hd-route-tag.good{color:var(--hd-good);background:var(--hd-good-bg);border-color:color-mix(in srgb,var(--hd-good) 35%,var(--hd-line))}
.hd-route-tag.warn{color:var(--hd-warn);border-color:color-mix(in srgb,var(--hd-warn) 35%,var(--hd-line));background:var(--hd-warn-bg)}
.hd-ip-cell{display:flex;align-items:center}
.hd-ip-tag{display:inline-block;padding:1px 7px;border-radius:999px;border:1px solid var(--hd-line);background:var(--hd-panel-2);color:var(--hd-muted);font-size:10px;font-weight:600;line-height:1.5;white-space:nowrap}
.hd-ip-tag.good{color:var(--hd-good);background:var(--hd-good-bg);border-color:color-mix(in srgb,var(--hd-good) 30%,var(--hd-line))}
.hd-ip-tag.warn{color:var(--hd-warn);background:var(--hd-warn-bg);border-color:color-mix(in srgb,var(--hd-warn) 30%,var(--hd-line))}
.hd-ip-tag.empty{background:transparent;border-style:dashed}
</style>
