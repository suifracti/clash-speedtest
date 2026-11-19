<script setup lang="ts">
import {computed,inject,nextTick,onBeforeUnmount,onMounted,ref,useId,watch} from 'vue'
import '../../home-design.css'
import './workbench-layout.css'
import {useWorkspace} from '../../workspace'
import {date,isReputationRule,key,projectNames,targets,type NodeOption,type Project} from '../../domain'
import {api,type ClashUsage} from '../../api'
import {nodeAttrMeaning,nodeDisplayName,nodeDisplayParts,nodeRegion,nodeRegionLabel,protocolBadgeClass,regionBadgeClass} from '../../nodePresentation'
import {airportNodeCount} from '../../airportViewing'
import Icon from '../Icon.vue'
import UiSelect from '../UiSelect.vue'
import UnlockPicker from '../UnlockPicker.vue'
import SubscriptionOverview from '../SubscriptionOverview.vue'
import AirportSidebar from '../AirportSidebar.vue'
import SiteDisplayPicker from '../SiteDisplayPicker.vue'
import UnlockServiceMatrix from './UnlockServiceMatrix.vue'
import NodeInspector from './NodeInspector.vue'
import NodeResponseStrip from './NodeResponseStrip.vue'
import WorkbenchSortHeader from './WorkbenchSortHeader.vue'
import NodeReaderNav from './NodeReaderNav.vue'
import NodeCompare from './NodeCompare.vue'
import Modal from '../Modal.vue'
import type {TrendPoint} from '../../presentation'
import {homeModelKey,weeklyHistoryKey} from './homeModel'
import {exitLanding} from '../../exitLanding'
import {entryCountry,isAnycast,lineLabel,routeKind,routeLabel,routeTone} from '../../entryPath'
import {unlockGroups,unlockGroupSummary,unlockSummary} from '../../unlock'
const w=useWorkspace(),m=inject(homeModelKey)!
const readingHintId=useId()
const exitMode=ref(false),serviceView=ref('nodes')
const clashUsage=ref<ClashUsage[]>([])
const dominantClashUsage=computed(()=>clashUsage.value[0])
const dominantClashNode=computed(()=>w.nodes.value.find(n=>n.display_name===dominantClashUsage.value?.name))
const dominantClashName=computed(()=>{const usage=dominantClashUsage.value;if(!usage)return '';const node=w.nodes.value.find(n=>n.display_name===usage.name);return node?nodeDisplayName(node):usage.name})
const dominantClashChains=computed(()=>dominantClashUsage.value?.chains.join(' → ')||'')
function isClashUsing(n:NodeOption){return clashUsage.value.some(usage=>usage.name===n.display_name)}
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
const chosen=m.chosen
const exitRows=computed(()=>Object.fromEntries(m.pageNodes.value.map(n=>{
 const landing=exitLanding(n,[...(m.qualityData.value[key(n)]?.services||[]),...(w.services.value[key(n)]||[])])
 const entry=w.entryFor(n),kind=routeKind(entryCountry(entry),landing.consensusRegion,isAnycast(entry),w.localPosition.value?.country_code)
 // 出口没测过时线路是按节点名称推的，标签置灰并在悬停里说明，别让人当成实测结论。
 const guessed=!landing.exitKnown&&kind!=='anycast'
 return [key(n),{...landing,ipTag:landing.ipKind.tag,route:routeLabel(kind),routeTone:guessed?'empty':routeTone(kind),routeTitle:guessed?'出口还没检测，线路按节点名称推断':routeLabel(kind)==='任播'?'入口是 Cloudflare 任播地址，不代表真实机房':'入口与出口国家相同为直连（本机在大陆、入口在境外时为跨境直连），不同为中转，入口在中国大陆为国内中转',line:lineLabel(entry,landing.consensusRegion,landing.org)}]
})))
function selectInspectorProject(id:Project|'exit'){if(id==='exit')selectExit();else{exitMode.value=false;w.project.value=id}}
function selectExit(){exitMode.value=true;m.headerSort.value='';m.qualityGate.value='all';m.qualitySort.value='existing';if(['latency','download','service'].includes(w.sort.value))w.sort.value='region'}
// 出口表只读缓存；真要解析要按“解析入口”或跑一次出口检测。
watch([exitMode,()=>m.pageNodes.value],()=>{if(exitMode.value)void w.readEntries(m.pageNodes.value,false).catch(()=>{})},{immediate:true})
const exitColumns=['线路','出口 IP','真实落地','IP 类型','风险']
const exitFields=['line','ip','label','ipTag','riskLabel'] as const
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
const gates=[{id:'all',label:'全部节点',short:'全部'},{id:'samples',label:'测得够多的（成功样本 ≥5 次）',short:'测得够多的'},{id:'response',label:'稳定能连的（近 7 天响应率 ≥90%）',short:'稳定能连的'},{id:'fresh',label:'最近 24 小时测过的',short:'最近 24 小时测过的'},{id:'download',label:'有过有效下载速度的',short:'有过有效下载速度的节点'},{id:'service',label:'已选服务检测通过的',short:'已选服务检测通过的节点'}]
// 每个标签只给和它相关的条件；综合标签保留全部。
const tabGateIds:Record<Project,string[]>={combined:['all','samples','response','fresh','download','service'],latency:['all','samples','response','fresh'],download:['all','download'],service:['all','service']}
const tabGates=computed(()=>gates.filter(g=>(exitMode.value?['all']:tabGateIds[w.project.value]).includes(g.id)))
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
const rowScroller=ref<HTMLElement>(),toolbar=ref<HTMLElement>(),workArea=ref<HTMLElement>(),toolbarHeight=ref(0),horizontalScroll=ref(0)
let toolbarObserver:ResizeObserver|undefined
function syncTableHeading(){horizontalScroll.value=rowScroller.value?.scrollLeft||0}
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
function selected(n:NodeOption){return w.selectedKeys.value.includes(key(n))}
const verdicts=computed(()=>Object.fromEntries(m.pageNodes.value.map(n=>[key(n),m.rowVerdict(n)])))
function groupSummary(n:NodeOption,id:string){return unlockGroupSummary(id,catalog.value,serviceId=>m.shownService(n,serviceId),w.nodeUnlockRegion(n),w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions,exitRows.value[key(n)]?.ipKind.type)}
function services(n:NodeOption){const summary=unlockSummary(catalog.value,id=>m.shownService(n,id),w.nodeUnlockRegion(n),w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions,exitRows.value[key(n)]?.ipKind.type);return {...summary,label:summary.tone==='empty'?'未测':summary.label}}
function run(){if(filterScopePending.value)return;if(exitMode.value){m.closeHover();w.openReputationPlan([...m.runNodes.value]);return}if(w.project.value==='combined')m.openCombinedPlan();else{m.closeHover();w.openPlan([...m.runNodes.value])}}
const showVerdict=computed(()=>!exitMode.value&&['combined','latency'].includes(w.project.value))
const overviewMode=computed(()=>!exitMode.value&&w.project.value==='combined')
const showSites=computed(()=>!exitMode.value&&w.project.value==='latency')
const showDownload=computed(()=>!exitMode.value&&['combined','download'].includes(w.project.value))
// Grid tracks follow the active project so each tab shows its own columns.
const columns=computed(()=>overviewMode.value?['28px','minmax(190px,1.5fr)',...sites.value.map(()=>'minmax(70px,.55fr)'),'minmax(90px,.7fr)','minmax(115px,.85fr)','60px'].join(' '):exitMode.value?['28px','minmax(190px,1.8fr)','minmax(90px,1.1fr)','78px','minmax(0,1.05fr)','88px','66px'].join(' '):showSites.value?['28px','minmax(188px,1.45fr)','82px','64px','86px','78px',...sites.value.map(()=>'minmax(105px,1fr)')].join(' '):['36px','minmax(220px,2.2fr)',...(showDownload.value?['minmax(110px,1fr)']:[]),...(w.project.value==='download'?['minmax(160px,1.4fr)']:[]),...(w.project.value==='service'?unlockGroups.map(()=>'minmax(90px,1fr)'):[])].join(' '))
const overviewMinWidth=computed(()=>28+190+sites.value.length*70+90+115+60+(sites.value.length+4)*10+28)
const latencyMinWidth=computed(()=>28+188+82+64+86+78+sites.value.length*105+(sites.value.length+5)*10+28)
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
const hoveredAttr=ref<{term:string;short:string;detail:string;x:number;y:number}|null>(null)
function showAttrTip(e:MouseEvent|FocusEvent,part:string){
 const info=nodeAttrMeaning(part)
 if(!info){hoveredAttr.value=null;return}
 const rect=(e.currentTarget as HTMLElement).getBoundingClientRect()
 hoveredAttr.value={...info,x:rect.left+rect.width/2,y:Math.max(12,rect.top-6)}
}
function clearAttrTip(){hoveredAttr.value=null}
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
  <section class="hd hd-workbench-layout" :style="{'--hd-toolbar-size':toolbarHeight+'px'}">
    <header class="hd-head">
      <div class="hd-title">
        <h1>线路工作台</h1>
        <p v-if="filteredAirport"><strong class="hd-head-airport">{{filteredAirport.name}}</strong> · {{filteredAirportNodes}} 个节点 · <button class="hd-link" @click="w.airportFilter.value='all'">看全部 {{chosen.length}} 个机场</button></p>
        <p v-else><button class="hd-link" @click="m.picker.value=true">{{chosen.length?chosen.length+' 个机场':'选择机场'}}</button> · {{chosenNodeCount}} 个节点</p>
        <button v-if="dominantClashUsage" class="hd-clash-usage" :disabled="!dominantClashNode" title="查看正在用的节点；按当前连接累计流量排序，每 30 秒更新" @click="inspectClashUsage"><span class="hd-clash-usage-dot" aria-hidden="true"></span>Clash 正在用：<strong>{{dominantClashName}}</strong><template v-if="dominantClashChains">（经策略组：{{dominantClashChains}}）</template></button>
      </div>
      <div class="hd-head-actions">
        <button class="hd-btn ghost hd-nav-shortcut" @click="w.page.value='recommend'">帮我选一条<Icon name="arrowRight"/></button>
        <button v-if="!matrixMode&&!m.compareOpen.value" class="hd-btn" :aria-pressed="inspectorOpen" :title="inspectorOpen?'收起详情，全宽显示节点表':'展开节点详情'" @click="toggleDetails"><Icon name="panel"/>{{inspectorOpen?'全宽列表':'展开详情'}}</button>
      </div>
    </header>

    <div class="hd-workspace">
    <section class="hd-source" :class="{'ledger-expanded':ledgerExpanded}" aria-label="机场选择">
      <header class="hd-source-heading">
        <h2>来源与套餐</h2>
        <span v-if="w.airports.value.length" class="hd-muted small">已纳 {{chosen.length}}/{{w.airports.value.length}}</span>
      </header>
      <div class="hd-source-details"><AirportSidebar @select="id=>w.viewAirport(id)"/></div>
      <SubscriptionOverview :airports="chosen" :notices="w.notices.value" :airport-id="w.airportFilter.value" @viewed="id=>{w.airportFilter.value=id}">
        <template #additional-actions>
          <div class="hd-source-tools">
            <button class="hd-ledger-toggle hd-link" :aria-expanded="ledgerExpanded" @click="ledgerExpanded=!ledgerExpanded">{{ledgerExpanded?'收起用量明细':'展开用量明细'}}</button>
          </div>
        </template>
      </SubscriptionOverview>
  </section>

    <div ref="workArea" class="hd-results" aria-label="线路对照区">
    <div ref="toolbar" class="hd-toolbar">
      <div class="hd-toolbar-top">
        <nav class="hd-seg" aria-label="检测项目"><button v-for="p in projects" :key="p" :aria-pressed="!exitMode&&w.project.value===p" @click="exitMode=false;w.project.value=p">{{projectNames[p]}}</button><button :aria-pressed="exitMode" @click="selectExit">出口与信誉</button></nav>
        <div class="hd-toolbar-actions">
          <button class="hd-btn ghost" :disabled="w.loading.value" title="重新载入节点列表" @click="w.refreshNodes().catch(w.fail)"><Icon name="refresh"/>重新载入</button>
          <button v-if="exitMode" class="hd-btn" :disabled="w.busy.value||!m.runNodes.value.length" title="只解析节点入口地址，不经过节点，不算一次真实检测" @click="w.resolveEntries(m.runNodes.value)">解析入口</button>
          <button v-if="!exitMode&&w.project.value!=='combined'" class="hd-btn hd-run-combined" :disabled="w.busy.value||!m.runNodes.value.length||filterScopePending" title="按当前显示的节点范围打开组合检测" @click="m.openCombinedPlan()">组合检测</button>
          <button class="hd-btn primary hd-run" :disabled="w.busy.value||!m.runNodes.value.length||filterScopePending" @click="run"><Icon name="play"/>{{exitMode?'检测出口与信誉':m.runLabel(w.project.value)}} · {{scopeLabel}}</button>
        </div>
      </div>
      <div class="hd-toolbar-filters">
        <label class="hd-search"><Icon name="search"/><input v-model="w.search.value" type="search" placeholder="搜节点、机场、地区" aria-label="搜索节点、机场或地区" title="可搜索节点名、机场名或地区"></label>
        <UiSelect v-model="w.region.value" aria-label="地区"><option value="all">全部地区</option><option v-if="selectedRegions.length>1" :value="w.region.value">已选 {{selectedRegions.length}} 个地区</option><option v-for="r in m.filterRegions.value" :key="r" :value="r">{{nodeRegionLabel(r)}}</option></UiSelect>
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
        <details v-if="!exitMode&&(w.project.value==='service'||w.project.value==='combined')" class="hd-services"><summary>选择解锁项目<small>{{m.displayServices.value.length?'已选 '+m.displayServices.value.length+' 项':'还没选'}}</small></summary><UnlockPicker v-model="w.serviceIds.value" :catalog="catalog"/></details>
        <div v-if="scopeAirportNodes.length" class="hd-region-strip" role="group" aria-label="按地区快选">
          <button type="button" class="hd-region-chip all" :aria-pressed="w.region.value==='all'&&!w.excludeRegion.value" @click="w.excludeRegion.value=false;w.region.value='all'">全部<small>{{scopeAirportNodes.length}}</small></button>
          <button v-for="rc in regionChips" :key="rc.code" type="button" class="hd-region-chip" :class="[regionBadgeClass(rc.code),{excluded:w.excludeRegion.value&&selectedRegions.includes(rc.code)}]" :aria-pressed="selectedRegions.includes(rc.code)" @click="toggleRegionChip(rc.code)">{{w.excludeRegion.value&&selectedRegions.includes(rc.code)?`排除 ${rc.label}`:rc.label}}<small>{{rc.count}}</small></button>
          <button type="button" class="hd-region-chip exclude-toggle" :aria-pressed="w.excludeRegion.value" title="开启后，点选的地区将被排除；也可以先选地区再点此反选排除" @click="toggleExcludeMode">排除所选</button>
          <span class="hd-quick-divider" aria-hidden="true"/>
          <button type="button" class="hd-quick-select" :disabled="!m.visibleNodes.value.length" :title="allVisibleSelected?'取消勾选当前筛选出来的节点':'一键勾选当前筛选出来的全部节点'" @click="w.toggleAll(m.visibleNodes.value)">{{allVisibleSelected?'取消全选当前':`勾选当前 ${m.visibleNodes.value.length} 个`}}</button>
          <button v-if="w.selectedNodes.value.length" type="button" class="hd-quick-select clear" @click="w.cancelSelection()">清空已选 {{w.selectedNodes.value.length}}</button>
        </div>
        <div v-if="latencyTab" class="hd-view-tail">
          <p :id="readingHintId" class="hd-reading-hint">{{overviewMode?'各站最近读数 · 短线为单次样本':'最近手动轮次 · 竖条按采样顺序，历史方格按时间，虚线框为监测'}}</p>
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

    <nav v-if="!exitMode&&w.project.value==='service'" class="hd-seg matrix-switch" aria-label="解锁结果视图"><button :aria-pressed="serviceView==='nodes'" @click="serviceView='nodes'">按节点</button><button :aria-pressed="serviceView==='services'" @click="serviceView='services'">按服务</button></nav>
    <UnlockServiceMatrix v-if="matrixMode"/>
    <nav v-if="!matrixMode" class="hd-list-pagination" aria-label="节点列表分页">
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
    <div v-if="!matrixMode" class="hd-body" :class="{'overview-mode':overviewMode,'latency-mode':showSites,'exit-mode':exitMode,'full-table':detailsOverlay||!inspectorOpen}" :style="{'--hd-side-w':visibleSideWidth+'px','--hd-overview-width':overviewMinWidth+'px','--hd-latency-width':latencyMinWidth+'px'}">
      <div ref="table" class="hd-table" role="table" aria-label="节点列表" :aria-describedby="latencyTab?readingHintId:undefined" tabindex="0" :style="{'--hd-cols':columns,'--hd-horizontal-offset':horizontalScroll+'px'}" @keydown="keyNav">
        <div class="hd-table-head" role="rowgroup">
        <div class="hd-tr hd-th" role="row">
          <span role="columnheader"><input type="checkbox" :checked="allOnPage" aria-label="选择本页全部节点" @change="w.toggleAll(m.pageNodes.value)"></span>
          <WorkbenchSortHeader column="name" label="节点"/><template v-if="exitMode"><span v-for="label in exitColumns" :key="label" role="columnheader">{{label}}</span></template>
          <template v-if="showSites">
            <WorkbenchSortHeader column="verdict" label="结论"/>
            <WorkbenchSortHeader column="purity" label="纯净度"/>
            <WorkbenchSortHeader column="responseRate" label="7 天响应率"/>
            <WorkbenchSortHeader column="volatility" label="波动幅度"/>
          </template>
          <WorkbenchSortHeader v-for="s in latencyTab?sites:[]" :key="s.id" class="hd-station-head" :column="'site:'+s.id" :label="s.name"/>
          <WorkbenchSortHeader v-if="showDownload" column="download" label="下载"/>
          <WorkbenchSortHeader v-if="!exitMode&&w.project.value==='download'" column="downloadTime" label="测量时间"/>
          <WorkbenchSortHeader v-if="overviewMode" column="service" label="服务解锁"/>
          <WorkbenchSortHeader v-if="overviewMode" column="purity" label="纯净度"/>
          <template v-if="!exitMode&&w.project.value==='service'"><WorkbenchSortHeader v-for="g in unlockGroups" :key="g.id" :column="'unlock:'+g.id" :label="g.name"/></template>
        </div>
        </div>
        <div ref="rowScroller" class="hd-row-scroll" role="rowgroup" @scroll="syncTableHeading">
        <div v-for="n in m.pageNodes.value" :key="key(n)" :data-node-key="key(n)" class="hd-tr" role="row" :class="{focus:isFocus(n)&&(inspectorOpen||!!focusKey),selected:selected(n),'clash-using':isClashUsing(n)}" :aria-selected="selected(n)" @click="focus(n)">
          <span role="cell" @click.stop><input type="checkbox" :checked="selected(n)" :aria-label="'选择 '+nodeDisplayName(n)" @change="w.toggleNode(n)"></span>
          <span role="cell" class="hd-node" :class="regionBadgeClass(n.country_code)" :title="n.display_name+' · '+m.sourceLabel(n)+' · '+n.type"><span><div class="hd-node-title"><span class="hd-flag" :class="regionBadgeClass(n.country_code)">{{n.country_code||'—'}}</span><span v-if="isClashUsing(n)" class="hd-clash-active-badge" aria-label="Clash 正在使用" title="Clash 正在使用"><i class="hd-clash-node-dot" aria-hidden="true"/>在用</span><button class="hd-node-open" :tabindex="isFocus(n)?0:-1" :aria-label="'查看 '+nodeDisplayName(n)+' · '+m.sourceLabel(n)" @focus="focusKey=key(n)" @click.stop="focus(n)"><strong><span class="hd-node-region">{{nodeDisplayParts(n).region}}</span><template v-for="(part,idx) in nodeDisplayParts(n).parts" :key="idx"><span class="hd-node-sep"> · </span><span class="hd-node-part" :class="part.kind" :title="nodeAttrMeaning(part.text)?.detail" @mouseenter="showAttrTip($event,part.text)" @mouseleave="clearAttrTip">{{part.text}}</span></template></strong></button></div><small><span class="hd-source-label">{{exitMode?w.nodeAirportName(n):m.sourceLabel(n)}}</span> · <b class="hd-proto-tag" :class="protocolBadgeClass(n.type)">{{n.type}}</b></small><span v-if="overviewMode" class="hd-route-verdict" :class="verdicts[key(n)].tone">{{verdicts[key(n)].label}}</span></span></span>
          <NodeResponseStrip v-if="overviewMode" :node="n"/>
          <template v-if="exitMode"><template v-for="field in exitFields" :key="field"><span v-if="field==='line'" role="cell" class="hd-exit-cell hd-line-cell"><span class="hd-line-text" :title="exitRows[key(n)].line">{{exitRows[key(n)].line}}</span><span v-if="exitRows[key(n)].route" class="hd-route-tag" :class="exitRows[key(n)].routeTone" :title="exitRows[key(n)].routeTitle">{{exitRows[key(n)].route}}</span></span><span v-else-if="field==='ipTag'" role="cell" class="hd-exit-cell hd-ip-cell"><span class="hd-ip-tag" :class="exitRows[key(n)].ipKind.tone" :title="'IP 类型：'+exitRows[key(n)].ipKind.detail">{{exitRows[key(n)].ipTag}}</span></span><span v-else role="cell" class="hd-num hd-exit-cell" :class="field==='label'?exitRows[key(n)].tone:field==='riskLabel'?exitRows[key(n)].riskTone:''" :title="field==='label'?exitRows[key(n)].detail:field==='riskLabel'?'IPPure 与机器流量占比取较高值':''">{{exitRows[key(n)][field]}}</span></template></template>
          <span v-if="showVerdict&&!overviewMode" role="cell"><b class="hd-pill" :class="[verdicts[key(n)].tone,{'partial-sites':verdicts[key(n)].label.startsWith('部分站点不通')}]">{{verdicts[key(n)].label}}</b></span>
          <span v-if="showVerdict&&!overviewMode" role="cell" class="hd-num" :class="exitRows[key(n)].purity.tone" :title="exitRows[key(n)].purity.title">{{exitRows[key(n)].purity.label}}</span>
          <template v-if="!exitMode&&w.project.value==='latency'"><span role="cell" class="hd-num hd-history-cell" :class="weeklyTone(n)" :title="weeklyTitle(n,'responseRate')">{{weeklyText(n,'responseRate')}}<small v-if="weeklyOld(n)" class="old-tag">旧</small></span><span role="cell" class="hd-num hd-history-cell" :class="weeklyStat(n)?.all.volatility==null?'empty':weeklyTone(n)" :title="weeklyTitle(n,'volatility')">{{weeklyText(n,'volatility')}}<small v-if="weeklyOld(n)" class="old-tag">旧</small></span></template>
          <NodeResponseStrip v-if="showSites" :node="n" detailed @round="(point,target)=>openLatencyRound(n,point,target)"/>
          <span v-if="showDownload" role="cell" class="hd-num" :class="m.downloadMetricTone(n)" :title="m.downloadMetricContext(n)">{{m.downloadMetric(n)}}</span>
          <span v-if="!exitMode&&w.project.value==='download'" role="cell" class="hd-num empty">{{m.downloadMetricContext(n)}}</span>
          <span v-if="overviewMode" role="cell" class="hd-overview-services"><span v-for="g in unlockGroups" :key="g.id" :class="groupSummary(n,g.id).tone" :title="groupSummary(n,g.id).title"><small>{{g.name}}</small><b>{{groupSummary(n,g.id).label}}</b></span></span>
          <span v-if="overviewMode" role="cell" class="hd-num" :class="exitRows[key(n)].purity.tone" :title="exitRows[key(n)].purity.title">{{exitRows[key(n)].purity.label}}</span>
          <template v-if="!exitMode&&w.project.value==='service'"><button v-for="g in unlockGroups" :key="g.id" role="cell" class="hd-svc-cell hd-num" :class="groupSummary(n,g.id).tone" :title="groupSummary(n,g.id).title" @click.stop="focus(n)">{{groupSummary(n,g.id).label}}</button></template>
        </div>
        <div v-if="!m.pageNodes.value.length" class="hd-empty"><Icon name="layers"/><strong>{{!w.airports.value.length?'先添加一份机场订阅':!chosen.length?'先选择一个机场':hasSourceNodes?'节点被筛选条件隐藏了':'这个机场还没有节点'}}</strong><button class="hd-btn" @click="!w.airports.value.length?w.page.value='airports':!chosen.length?m.picker.value=true:hasSourceNodes?clearFilters():w.page.value='airports'">{{!w.airports.value.length?'添加机场':!chosen.length?'选择机场':hasSourceNodes?'清除筛选':'管理订阅'}}</button></div>
        </div>
      </div>
      <template v-if="!detailsOverlay">
        <div v-if="inspectorOpen" class="hd-resize" role="separator" aria-orientation="vertical" aria-label="调整检查器宽度" :aria-valuenow="visibleSideWidth" :aria-valuemin="SIDE_MIN" :aria-valuemax="maxSideWidth" tabindex="0" @pointerdown="startResize" @keydown.left.prevent="setSide(visibleSideWidth+24)" @keydown.right.prevent="setSide(visibleSideWidth-24)" @dblclick="setSide(420)"/>
        <NodeInspector v-if="readingNode&&inspectorOpen" :key="key(readingNode)+readingNode.node_key" class="hd-side" :node="readingNode" :exit-only="exitMode" :clash-usage="clashUsage" :read-only="!readingCurrent" :context-notice="readingNotice" @collapse="closeDetails" @select-project="selectInspectorProject" @refresh-clash-usage="refreshClashUsage"><template #navigation><NodeReaderNav :index="readingIndex" :count="readingNodes.length" :selected-count="w.selectedNodes.value.length" :previous="readerLabel(-1)" :next="readerLabel(1)" @previous="moveReading(-1)" @next="moveReading(1)" @compare="m.openComparison()"/></template></NodeInspector>
      </template>
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
      <button class="hd-link" :disabled="m.qualityFiltersPending.value" @click="w.toggleAll(m.visibleNodes.value)">{{m.visibleNodes.value.length&&m.visibleNodes.value.every(selected)?'取消范围选择':'全选筛选范围'}}</button>
      <button class="hd-btn" :disabled="w.selectedNodes.value.length<2" @click="m.openComparison()">比较已选 {{w.selectedNodes.value.length}} 个节点</button>
      <button class="hd-btn primary" :disabled="w.busy.value" @click="exitMode?w.openReputationPlan():w.openPlan()"><Icon name="play"/>检测已选</button>
      <button class="hd-btn" :disabled="w.busy.value||!w.selectedNodes.value.length" title="只解析节点入口地址，不经过节点，不算一次真实检测" @click="w.resolveEntries(w.selectedNodes.value)">解析入口</button>
      <button class="hd-btn" @click="w.monitorNodes.value=w.selectedNodes.value.map(n=>({...n}))"><Icon name="monitor"/>持续监测延迟</button>
      <button class="hd-btn" @click="w.exportSelection().catch(w.fail)">导出配置</button>
      <button class="hd-link" @click="w.selectedKeys.value=[]">清空已选</button>
    </div>

    <footer v-if="!matrixMode" class="hd-foot">
      <span>{{m.visibleNodes.value.length}} 个节点<template v-if="w.selectedKeys.value.length"> · 已选 {{w.selectedKeys.value.length}}</template> · ↑↓ 浏览 · Enter 查看 <button class="hd-link hd-select-range" :disabled="m.qualityFiltersPending.value||!m.visibleNodes.value.length" @click="w.toggleAll(m.visibleNodes.value)">{{m.visibleNodes.value.length&&m.visibleNodes.value.every(selected)?'取消范围选择':'全选筛选范围'}}</button></span>
      <span class="hd-legend"><span class="good">正常</span><span class="warn">部分</span><span class="bad">失败</span><span class="empty">未测</span></span>
      <nav v-if="m.totalPages.value>1" class="hd-pages" aria-label="列表底部分页"><button class="hd-btn" :disabled="m.currentPage.value<=1" @click="changePage(-1,true)">上一页</button><strong>{{m.currentPage.value}} / {{m.totalPages.value}} 页</strong><button class="hd-btn" :disabled="m.currentPage.value>=m.totalPages.value" @click="changePage(1,true)">下一页</button></nav>
    </footer>
    </div>
    </div>
    <div v-if="hoveredAttr" class="hd-attr-popover" role="tooltip" :style="{left:hoveredAttr.x+'px',top:hoveredAttr.y+'px'}"><strong>{{hoveredAttr.term}} · {{hoveredAttr.short}}</strong><small>{{hoveredAttr.detail}}</small></div>
  </section>
</template>

<style scoped>
.hd-pill.partial-sites{max-width:100%;white-space:normal;line-height:1.4;border-radius:8px;box-sizing:border-box}
.history-sort{font-size:11px;white-space:nowrap;text-align:left}
.old-tag{margin-left:2px;padding:0 2px;border-radius:2px;background:var(--hd-soft);color:var(--hd-muted);font-size:9px;font-weight:600}
.hd-clash-usage{display:inline-flex;align-items:center;gap:6px;padding:3px 10px;border:1px solid color-mix(in srgb,var(--hd-good) 32%,var(--hd-line));border-radius:999px;background:color-mix(in srgb,var(--hd-good) 9%,var(--hd-panel));font-size:11.5px;font-weight:550;line-height:1.4;color:color-mix(in srgb,var(--hd-good) 85%,var(--hd-text));transition:border-color .14s,background .14s}.hd-clash-usage{cursor:pointer}.hd-clash-usage:hover{border-color:var(--hd-good);background:color-mix(in srgb,var(--hd-good) 14%,var(--hd-panel))}.hd-clash-usage strong{color:var(--hd-text)}.hd-clash-usage-dot,.hd-clash-node-dot{display:inline-block;flex:none;width:6px;height:6px;border-radius:50%;background:var(--hd-good)}.hd-clash-active-badge{display:inline-flex;align-items:center;gap:4px;flex:none;padding:1px 7px;border-radius:999px;font-size:10px;font-weight:700;color:var(--hd-good);background:var(--hd-good-bg);border:1px solid color-mix(in srgb,var(--hd-good) 32%,transparent);line-height:1.45}
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
