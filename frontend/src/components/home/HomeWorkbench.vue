<script setup lang="ts">
import {computed,inject,nextTick,onBeforeUnmount,onMounted,ref,watch} from 'vue'
import '../../home-design.css'
import {useWorkspace} from '../../workspace'
import {date,isReputationRule,key,projectNames,targets,type NodeOption,type Project} from '../../domain'
import {api,type ClashUsage} from '../../api'
import {nodeDisplayName,nodeRegionLabel} from '../../nodePresentation'
import {airportNodeCount} from '../../airportViewing'
import Icon from '../Icon.vue'
import UiSelect from '../UiSelect.vue'
import UnlockPicker from '../UnlockPicker.vue'
import SubscriptionOverview from '../SubscriptionOverview.vue'
import NodeActivity from '../NodeActivity.vue'
import SiteDisplayPicker from '../SiteDisplayPicker.vue'
import UnlockServiceMatrix from './UnlockServiceMatrix.vue'
import NodeInspector from './NodeInspector.vue'
import NodeCompare from './NodeCompare.vue'
import {stableLatency} from '../../inspectorLatency'
import {homeModelKey,latencyVerdict,type Verdict} from './homeModel'
import {exitLanding} from '../../exitLanding'
import {entryCountry,isAnycast,lineLabel,routeKind,routeLabel,routeTone} from '../../entryPath'
import {unlockGroups,unlockGroupSummary,unlockSummary} from '../../unlock'
const w=useWorkspace(),m=inject(homeModelKey)!
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
function selectExit(){exitMode.value=true;m.qualityGate.value='all';m.qualitySort.value='existing';if(['latency','download','service'].includes(w.sort.value))w.sort.value='region'}
// 出口表只读缓存；真要解析要按“解析入口”或跑一次出口检测。
watch([exitMode,()=>m.pageNodes.value],()=>{if(exitMode.value)void w.readEntries(m.pageNodes.value,false).catch(()=>{})},{immediate:true})
const exitColumns=['线路','出口 IP','真实落地','IP 类型','风险']
const exitFields=['line','ip','label','ipTag','riskLabel'] as const
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)))
// 同名节点的七天统计包含配置变化前后的历史。
function weeklyStat(n:NodeOption){return m.weeklyStats.value[key(n)]}
function weeklyOld(n:NodeOption){return !!weeklyStat(n)?.allOld}
function weeklyText(n:NodeOption,metric:'responseRate'|'volatility'){
 const s=weeklyStat(n)
 if(!s)return m.weeklyErrors.value[key(n)]?'读取失败':'读取中'
 const value=s.all[metric]
 if(metric==='responseRate')return value!=null?Math.round(value)+'%':'未测'
 return value!=null?Math.round(value)+' ms':'—'
}
function weeklyTone(n:NodeOption){const s=weeklyStat(n);return !s||s.all.responseRate==null?'empty':s.all.responseRate>=90?'good':'warn'}
function weeklyTitle(n:NodeOption,metric:'responseRate'|'volatility'){
 const s=weeklyStat(n),error=m.weeklyErrors.value[key(n)]
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
const chosenNodeCount=computed(()=>chosen.value.reduce((count,airport)=>count+airportNodeCount(airport),0))
const hasSourceNodes=computed(()=>w.proxyNodes.value.some(n=>w.airportFilter.value==='all'||w.nodeAirportId(n)===w.airportFilter.value))
function clearFilters(){w.search.value='';w.region.value='all';w.excludeRegion.value=false;w.subscription.value='all';w.downloadFullOnly.value=false;m.protocolFilter.value='all';m.qualityGate.value='all';m.currentPage.value=1}
async function inspectClashUsage(){
 const node=dominantClashNode.value;if(!node)return
 clearFilters();w.viewAirport('all');m.qualitySort.value='existing';m.weeklySort.value='';m.compareOpen.value=false
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
const scopeLabel=computed(()=>w.selectedNodes.value.length?`已选 ${w.selectedNodes.value.length} 个节点`:`筛选 ${m.runNodes.value.length} 个`)
const filterSummary=computed(()=>[
 w.airportFilter.value==='all'?'':chosen.value.find(a=>a.id===w.airportFilter.value)?.name,
 w.subscription.value==='all'?'':m.filterProfiles.value.find(([id])=>id===w.subscription.value)?.[1],
 w.region.value!=='all'&&w.excludeRegion.value?'排除 '+nodeRegionLabel(w.region.value):'',
 m.protocolFilter.value==='all'?'':m.protocolFilter.value,
 m.qualityGate.value==='all'?'':gates.find(g=>g.id===m.qualityGate.value)?.label,
 m.qualitySort.value==='existing'?'':qualitySorts.find(q=>q.id===m.qualitySort.value)?.label,
 w.downloadFullOnly.value&&w.project.value==='download'?'只看完整读取':''].filter(Boolean).join(' · '))
const more=ref<HTMLDetailsElement>(),moreSummary=ref<HTMLElement>()
function closeMore(){if(!more.value?.open)return;more.value.open=false;moreSummary.value?.focus({preventScroll:true})}
// 按机场筛选时，页头和来源带都切到这个机场，不再显示全部机场的汇总。
const filteredAirport=computed(()=>w.airportFilter.value==='all'?undefined:chosen.value.find(a=>a.id===w.airportFilter.value))
const filteredAirportNodes=computed(()=>airportNodeCount(w.airports.value.find(airport=>airport.id===w.airportFilter.value)))
const qualityFiltering=computed(()=>m.qualityGate.value!=='all'||m.qualitySort.value!=='existing')
const focusKey=ref<string|null>(null)
const focusNode=computed(()=>m.pageNodes.value.find(n=>key(n)===focusKey.value)||m.pageNodes.value[0])
function focus(n:NodeOption){w.latencyReportFocus.value=null;if(focusNode.value&&key(focusNode.value)===key(n))return;m.closeHover();focusKey.value=key(n)}
// 检查器宽度可拖动（或用 ←→），记在本机。
const SIDE_MIN=340,SIDE_MAX=680
function savedSide(){try{const v=Number(window.localStorage.getItem('cst.inspector-width'));return v>=SIDE_MIN&&v<=SIDE_MAX?v:420}catch{return 420}}
const sideWidth=ref(savedSide())
function setSide(v:number){sideWidth.value=Math.round(Math.min(SIDE_MAX,Math.max(SIDE_MIN,v)));try{window.localStorage.setItem('cst.inspector-width',String(sideWidth.value))}catch{/* 只影响记忆 */}}
function startResize(e:PointerEvent){
 const start=e.clientX,width=sideWidth.value,el=e.currentTarget as HTMLElement
 el.setPointerCapture?.(e.pointerId)
 const move=(ev:PointerEvent)=>setSide(width+start-ev.clientX)
 const up=()=>{el.removeEventListener('pointermove',move);el.removeEventListener('pointerup',up);document.body.classList.remove('cst-resizing')}
 document.body.classList.add('cst-resizing');el.addEventListener('pointermove',move);el.addEventListener('pointerup',up)
}
// ⌘K「查看节点」：只改查看中的节点和页码，不勾选、不动筛选；被筛掉的节点改用详情弹窗查看。
const table=ref<HTMLElement>()
watch(()=>w.viewRequest.value,async n=>{
 if(!n)return
 w.viewRequest.value=null
 if(w.latencyReportFocus.value?.nodeKey===key(n)){exitMode.value=false;m.qualityGate.value='all';m.protocolFilter.value='all';m.qualitySort.value='existing';m.weeklySort.value='';await nextTick()}
 const k=key(n),i=m.visibleNodes.value.findIndex(v=>key(v)===k)
 if(i<0){w.inspectNode.value=n;return}
 m.currentPage.value=m.pageSize.value>0?Math.floor(i/m.pageSize.value)+1:1;m.closeHover();focusKey.value=k
 void nextTick(()=>table.value?.querySelector<HTMLElement>('.hd-tr.focus')?.scrollIntoView?.({block:'nearest'}))
},{immediate:true})
function isFocus(n:NodeOption){return !!focusNode.value&&key(focusNode.value)===key(n)}
function selected(n:NodeOption){return w.selectedKeys.value.includes(key(n))}
const verdicts=computed<Record<string,Verdict>>(()=>Object.fromEntries(m.pageNodes.value.map(n=>[key(n),latencyVerdict(sites.value.map(s=>m.metricPoint(n,s.id)),stableLatency(sites.value.map(s=>({id:s.id,points:[m.metricPoint(n,s.id)].filter((p):p is NonNullable<typeof p>=>!!p)}))),weeklyStat(n)?.allOld?null:weeklyStat(n)?.all.responseRate,sites.value.map(s=>s.name))])))
function groupSummary(n:NodeOption,id:string){return unlockGroupSummary(id,catalog.value,serviceId=>m.shownService(n,serviceId),w.nodeUnlockRegion(n),w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions,exitRows.value[key(n)]?.ipKind.type)}
function services(n:NodeOption){const summary=unlockSummary(catalog.value,id=>m.shownService(n,id),w.nodeUnlockRegion(n),w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions,exitRows.value[key(n)]?.ipKind.type);return {...summary,label:summary.tone==='empty'?'未测':summary.label}}
function run(){if(exitMode.value){m.closeHover();w.openReputationPlan([...m.runNodes.value]);return}if(w.project.value==='combined')m.openCombinedPlan();else{m.closeHover();w.openPlan([...m.runNodes.value])}}
const showVerdict=computed(()=>!exitMode.value&&['combined','latency'].includes(w.project.value))
const showSites=showVerdict
const showDownload=computed(()=>!exitMode.value&&['combined','download'].includes(w.project.value))
// Grid tracks follow the active project so each tab shows its own columns.
const columns=computed(()=>exitMode.value?['28px','minmax(190px,1.8fr)','minmax(90px,1.1fr)','78px','minmax(0,1.05fr)','88px','66px'].join(' '):['36px','minmax(220px,2.2fr)',...(showVerdict.value?['92px','104px']:[]),...(!exitMode.value&&w.project.value==='latency'?['106px','82px']:[]),...(showSites.value?sites.value.map(()=>'minmax(74px,.8fr)'):[]),...(showDownload.value?['minmax(110px,1fr)']:[]),...(w.project.value==='download'?['minmax(160px,1.4fr)']:[]),...(w.project.value==='combined'?['minmax(80px,.8fr)']:[]),...(w.project.value==='service'?unlockGroups.map(()=>'minmax(90px,1fr)'):[])].join(' '))
const allOnPage=computed(()=>m.pageNodes.value.length>0&&m.pageNodes.value.every(selected))
function keyNav(e:KeyboardEvent){
 const list=m.pageNodes.value,i=list.findIndex(isFocus)
 const next=e.key==='ArrowDown'?list[i+1]:e.key==='ArrowUp'?list[i-1]:undefined
 if(next){e.preventDefault();focus(next)}
}
</script>
<template>
  <section class="hd">
    <header class="hd-head">
      <div class="hd-title">
        <h1>线路工作台</h1>
        <p v-if="filteredAirport"><strong class="hd-head-airport">{{filteredAirport.name}}</strong> · {{filteredAirportNodes}} 个节点 · <button class="hd-link" @click="w.viewAirport('all')">看全部 {{chosen.length}} 个机场</button></p>
        <p v-else><button class="hd-link" @click="m.picker.value=true">{{chosen.length?m.chosenNames.value:'选择机场'}}</button> · {{chosenNodeCount}} 个节点</p>
        <button v-if="dominantClashUsage" class="hd-clash-usage" :disabled="!dominantClashNode" title="查看正在用的节点；按当前连接累计流量排序，每 30 秒更新" @click="inspectClashUsage"><span class="hd-clash-usage-dot" aria-hidden="true"></span>Clash 正在用：<strong>{{dominantClashName}}</strong><template v-if="dominantClashChains">（经 {{dominantClashChains}}）</template></button>
      </div>
    </header>

    <section class="hd-source" aria-label="机场选择">
      <SubscriptionOverview :airports="chosen" :notices="w.notices.value" :airport-id="w.airportFilter.value" @viewed="id=>{w.airportFilter.value=id}"/>
    </section>

    <div class="hd-toolbar">
      <nav class="hd-seg" aria-label="检测项目"><button v-for="p in projects" :key="p" :aria-pressed="!exitMode&&w.project.value===p" @click="exitMode=false;w.project.value=p">{{projectNames[p]}}</button><button :aria-pressed="exitMode" @click="selectExit">出口与信誉</button></nav>
      <label class="hd-search"><Icon name="search"/><input v-model="w.search.value" type="search" placeholder="搜节点、机场、地区" aria-label="搜索节点、机场或地区" title="可搜索节点名、机场名或地区"></label>
      <UiSelect v-model="w.region.value" aria-label="地区"><option value="all">全部地区</option><option v-for="r in m.filterRegions.value" :key="r" :value="r">{{nodeRegionLabel(r)}}</option></UiSelect>
      <UiSelect v-model="w.sort.value" aria-label="排序"><option v-for="s in sorts" :key="s.id" :value="s.id">排序：{{s.label}}</option></UiSelect>
      <UiSelect v-model.number="w.hours.value" aria-label="时间范围"><option :value="6">最近 6 小时</option><option :value="24">最近 24 小时</option><option :value="168">最近 7 天</option><option :value="720">最近 30 天</option></UiSelect>
      <button class="hd-btn ghost" :disabled="w.loading.value" title="重新载入节点列表" @click="w.refreshNodes().catch(w.fail)"><Icon name="refresh"/>重新载入</button>
      <button class="hd-btn primary hd-run" :disabled="w.busy.value||!m.runNodes.value.length" @click="run"><Icon name="play"/>{{exitMode?'检测出口与信誉':m.runLabel(w.project.value)}} · {{scopeLabel}}</button>
      <button v-if="!exitMode&&w.project.value!=='combined'" class="hd-btn hd-run-combined" :disabled="w.busy.value||!m.runNodes.value.length" title="按当前显示的节点范围打开组合检测" @click="m.openCombinedPlan()">组合检测</button>
    </div>
    <details ref="more" class="hd-more" @keydown.esc="closeMore"><summary ref="moreSummary">更多筛选<small>{{filterSummary?filterSummary+' · ':''}}{{m.qualityFiltersPending.value?'范围确认中':'符合 '+m.visibleNodes.value.length+' 个'}}</small></summary>
      <div class="hd-more-groups">
        <section class="hd-more-group" aria-labelledby="more-scope">
          <h4 id="more-scope">看哪些节点<small>按来源和协议缩小列表</small></h4>
          <div class="hd-more-grid">
            <label>机场<UiSelect v-model="w.airportFilter.value" aria-label="机场筛选"><option value="all">全部机场</option><option v-for="a in chosen" :key="a.id" :value="a.id">{{a.name}}</option></UiSelect></label>
            <label v-if="m.filterProfiles.value.length>1">订阅<UiSelect v-model="w.subscription.value" aria-label="订阅筛选"><option value="all">全部订阅</option><option v-for="[id,name] in m.filterProfiles.value" :key="id" :value="id">{{name}}</option></UiSelect></label>
            <label>协议<UiSelect v-model="m.protocolFilter.value" aria-label="协议筛选"><option value="all">全部协议</option><option v-for="p in m.protocolChoices.value" :key="p" :value="p">{{p}}</option></UiSelect></label>
            <label v-if="w.sort.value==='airport'&&chosen.length>1&&w.airportFilter.value==='all'">优先机场<UiSelect v-model="w.priorityAirportId.value" aria-label="优先机场"><option value="all">机场自然顺序</option><option v-for="a in chosen" :key="a.id" :value="a.id">{{a.name}}优先</option></UiSelect></label>
            <label v-if="w.region.value!=='all'" class="hd-check"><input v-model="w.excludeRegion.value" type="checkbox">反过来：排除「{{nodeRegionLabel(w.region.value)}}」</label>
          </div>
        </section>
        <section class="hd-more-group" aria-labelledby="more-result">
          <h4 id="more-result">按{{w.project.value==='combined'?'检测':projectNames[w.project.value]}}结果<small>{{resultHint}}</small></h4>
          <div class="hd-more-grid">
            <label v-if="tabGates.length>2">只看<UiSelect v-model="m.qualityGate.value" aria-label="质量证据筛选"><option v-for="g in tabGates" :key="g.id" :value="g.id">{{g.label}}</option></UiSelect></label>
            <label v-else-if="tabGates.length===2" class="hd-check"><input type="checkbox" :checked="m.qualityGate.value===tabGates[1].id" @change="m.qualityGate.value=($event.target as HTMLInputElement).checked?tabGates[1].id:'all'">只看{{tabGates[1].short}}</label>
            <label v-if="!exitMode&&w.project.value==='download'" class="hd-check"><input v-model="w.downloadFullOnly.value" type="checkbox">只看完整读完的测速</label>
            <template v-if="latencyTab">
              <label>延迟排序<UiSelect v-model="m.qualitySort.value" aria-label="质量分组排序"><option v-for="q in qualitySorts" :key="q.id" :value="q.id">{{q.label}}</option></UiSelect></label>
              <label>延迟按哪个站点算<UiSelect v-model="w.displayTarget.value" aria-label="质量统计站点"><option v-for="s in targets" :key="s.id" :value="s.id">{{s.name}}</option></UiSelect></label>
            </template>
          </div>
          <p class="hd-muted hd-more-note">数据截止 {{date(m.qualityCutoff.value)}} · 各项分别测量，不作综合打分<button class="hd-link" :disabled="m.qualityLoading.value" @click="m.qualityGate.value==='response'?m.readWeekly(true):m.readQualityNodes(qualityFiltering?w.proxyNodes.value:m.pageNodes.value,true)">{{m.qualityLoading.value?'读取中…':'更新质量记录'}}</button></p>
          <p v-if="qualityFiltering" class="hd-muted" role="status">{{m.qualityGate.value==='response'?(m.qualityFiltersPending.value?'正在读取近 7 天延迟历史…':`按近 7 天采样响应率 ≥90% 筛选 · 当前符合 ${m.visibleNodes.value.length} 个`):m.allQualityReady.value?`已读固定范围 ${w.proxyNodes.value.length} 个节点 · 当前符合 ${m.visibleNodes.value.length} 个 · 读取失败 ${Object.keys(m.qualityErrors.value).length} 个 · 存在缺页/读取错误 ${Object.values(m.qualityData.value).filter(e=>!e.complete).length} 个（只限制对应类型的证据）`:`正在读取全部 ${w.proxyNodes.value.length} 个节点的历史记录（已读 ${Object.keys(m.qualityData.value).length}）；读取完成后应用条件筛选与排序`}}</p>
        </section>
        <section class="hd-more-group" aria-labelledby="more-view">
          <h4 id="more-view">怎么显示</h4>
          <div class="hd-more-grid">
            <label>每页<UiSelect :model-value="m.pageSize.value" aria-label="每页显示节点数" @update:model-value="m.setPageSize(Number($event))"><option :value="10">10 个节点</option><option :value="20">20 个节点</option><option :value="50">50 个节点</option><option :value="100">100 个节点</option><option :value="0">全部</option></UiSelect></label>
            <div v-if="latencyTab" class="hd-more-sites"><SiteDisplayPicker v-model="w.displayTargets.value"/></div>
          </div>
        </section>
      </div>
    </details>
    <p v-if="['latency','download','service'].includes(w.sort.value)" class="hd-muted hd-sort-status" role="status">{{w.sortLoading.value?`读取全部节点 ${w.sortReadCount.value}/${w.proxyNodes.value.length}…`:w.sortError.value||(w.sortAt.value?'排序截止 '+date(w.sortAt.value)+' · 翻页不改变排名':'尚未读取全部节点，仅按已读记录排序')}}<button class="hd-link" :disabled="w.sortLoading.value" @click="w.refreshSort()">更新排序</button></p>
    <details v-if="!exitMode&&(w.project.value==='service'||w.project.value==='combined')" class="hd-services"><summary>选择解锁项目<small>{{m.displayServices.value.length?'已选 '+m.displayServices.value.length+' 项':'还没选'}}</small></summary><UnlockPicker v-model="w.serviceIds.value" :catalog="catalog"/></details>

    <div v-if="w.selectedKeys.value.length" class="hd-selection" aria-live="polite">
      <strong>已选 {{w.selectedNodes.value.length}} 个节点</strong><small v-if="m.hiddenSelectedCount.value" class="hd-muted">含 {{m.hiddenSelectedCount.value}} 个筛选外节点</small>
      <button class="hd-link" :disabled="m.qualityFiltersPending.value" @click="w.toggleAll(m.visibleNodes.value)">{{m.visibleNodes.value.length&&m.visibleNodes.value.every(selected)?'取消范围选择':'全选筛选范围'}}</button>
      <button class="hd-btn" :disabled="w.selectedNodes.value.length<2" @click="m.openComparison()">比较已选 {{w.selectedNodes.value.length}} 个节点</button>
      <button class="hd-btn primary" :disabled="w.busy.value" @click="exitMode?w.openReputationPlan():w.openPlan()"><Icon name="play"/>检测已选</button>
      <button v-if="exitMode" class="hd-btn" :disabled="w.busy.value||!w.selectedNodes.value.length" title="只解析节点入口地址，不经过节点，不算一次真实检测" @click="w.resolveEntries(w.selectedNodes.value)">解析入口</button>
      <button class="hd-btn" @click="w.monitorNodes.value=w.selectedNodes.value.map(n=>({...n}))"><Icon name="monitor"/>持续监测延迟</button>
      <button class="hd-btn" @click="w.exportSelection().catch(w.fail)">导出配置</button>
      <button class="hd-link" @click="w.selectedKeys.value=[]">清空已选</button>
    </div>
    <div v-if="w.busy.value" class="hd-live"><span class="spinner"/>检测进行中{{w.queue.value.length?` · ${w.done.value}/${w.queue.value.length}`:''}}<button class="hd-link" @click="w.queueOpen.value=true">查看进度</button><button class="hd-link bad" @click="w.cancelTests()">{{w.cancelRequested.value?'正在取消…':'取消'}}</button></div>

    <nav v-if="!exitMode&&w.project.value==='service'" class="hd-seg matrix-switch" aria-label="解锁结果视图"><button :aria-pressed="serviceView==='nodes'" @click="serviceView='nodes'">按节点</button><button :aria-pressed="serviceView==='services'" @click="serviceView='services'">按服务</button></nav>
    <UnlockServiceMatrix v-if="matrixMode"/>
    <div v-else class="hd-body" :class="{'exit-mode':exitMode,comparing:m.compareOpen.value,'compare-wide':m.compareOpen.value&&m.compareNodes.value.length>2}" :style="{'--hd-side-w':sideWidth+'px'}">
      <div ref="table" class="hd-table" role="table" aria-label="节点列表" tabindex="0" :style="{'--hd-cols':columns}" @keydown="keyNav">
        <div class="hd-tr hd-th" role="row">
          <span role="columnheader"><input type="checkbox" :checked="allOnPage" aria-label="选择本页全部节点" @change="w.toggleAll(m.pageNodes.value)"></span>
          <span role="columnheader">节点</span><template v-if="exitMode"><span v-for="label in exitColumns" :key="label" role="columnheader">{{label}}</span></template>
          <span v-if="showVerdict" role="columnheader">结论</span>
          <span v-if="showVerdict" role="columnheader" :aria-sort="m.weeklySort.value==='purity'?(m.weeklyDescending.value?'descending':'ascending'):'none'"><button class="hd-link" @click="m.sortWeekly('purity')">纯净度 {{m.weeklySort.value==='purity'?(m.weeklyDescending.value?'↓':'↑'):''}}</button></span>
          <template v-if="!exitMode&&w.project.value==='latency'"><span v-for="[metric,label] in [['responseRate','7 天响应率'],['volatility','波动幅度']]" :key="metric" role="columnheader" :aria-sort="m.weeklySort.value===metric?(m.weeklyDescending.value?'descending':'ascending'):'none'"><button class="hd-link history-sort" @click="m.sortWeekly(metric)">{{label}} {{m.weeklySort.value===metric?(m.weeklyDescending.value?'↓':'↑'):''}}</button></span></template>
          <template v-if="showSites"><span v-for="s in sites" :key="s.id" role="columnheader">{{s.name}}</span></template>
          <span v-if="showDownload" role="columnheader">下载</span>
          <span v-if="!exitMode&&w.project.value==='download'" role="columnheader">测量时间</span>
          <span v-if="!exitMode&&w.project.value==='combined'" role="columnheader">服务</span>
          <template v-if="!exitMode&&w.project.value==='service'"><span v-for="g in unlockGroups" :key="g.id" role="columnheader">{{g.name}}</span></template>
        </div>
        <div v-for="n in m.pageNodes.value" :key="key(n)" class="hd-tr" role="row" :class="{focus:isFocus(n),selected:selected(n)}" :aria-selected="isFocus(n)" @click="focus(n)">
          <span role="cell" @click.stop><input type="checkbox" :checked="selected(n)" :aria-label="'选择 '+nodeDisplayName(n)" @change="w.toggleNode(n)"></span>
          <span role="cell" class="hd-node" :title="n.display_name+' · '+m.sourceLabel(n)+' · '+n.type"><span class="hd-flag">{{n.country_code||'—'}}</span><span><strong><i v-if="isClashUsing(n)" class="hd-clash-node-dot" aria-label="Clash 正在使用" title="Clash 正在使用"></i>{{nodeDisplayName(n)}}</strong><small>{{exitMode?w.nodeAirportName(n):m.sourceLabel(n)}} · {{n.type}}</small></span><NodeActivity :state="m.nodeActivities.value[key(n)]?.state" :label="m.nodeActivities.value[key(n)]?.label"/></span>
          <template v-if="exitMode"><template v-for="field in exitFields" :key="field"><span v-if="field==='line'" role="cell" class="hd-exit-cell hd-line-cell"><span class="hd-line-text" :title="exitRows[key(n)].line">{{exitRows[key(n)].line}}</span><span v-if="exitRows[key(n)].route" class="hd-route-tag" :class="exitRows[key(n)].routeTone" :title="exitRows[key(n)].routeTitle">{{exitRows[key(n)].route}}</span></span><span v-else-if="field==='ipTag'" role="cell" class="hd-exit-cell hd-ip-cell"><span class="hd-ip-tag" :class="exitRows[key(n)].ipKind.tone" :title="'IP 类型：'+exitRows[key(n)].ipKind.detail">{{exitRows[key(n)].ipTag}}</span></span><span v-else role="cell" class="hd-num hd-exit-cell" :class="field==='label'?exitRows[key(n)].tone:field==='riskLabel'?exitRows[key(n)].riskTone:''" :title="field==='label'?exitRows[key(n)].detail:field==='riskLabel'?'IPPure 与机器流量占比取较高值':''">{{exitRows[key(n)][field]}}</span></template></template>
          <span v-if="showVerdict" role="cell"><b class="hd-pill" :class="[verdicts[key(n)].tone,{'partial-sites':verdicts[key(n)].label.startsWith('部分站点不通')}]">{{verdicts[key(n)].label}}</b></span>
          <span v-if="showVerdict" role="cell" class="hd-num" :class="exitRows[key(n)].purity.tone" :title="exitRows[key(n)].purity.title">{{exitRows[key(n)].purity.label}}</span>
          <template v-if="!exitMode&&w.project.value==='latency'"><span role="cell" class="hd-num hd-history-cell" :class="weeklyTone(n)" :title="weeklyTitle(n,'responseRate')">{{weeklyText(n,'responseRate')}}<small v-if="weeklyOld(n)" class="old-tag">旧</small></span><span role="cell" class="hd-num hd-history-cell" :class="weeklyStat(n)?.all.volatility==null?'empty':weeklyTone(n)" :title="weeklyTitle(n,'volatility')">{{weeklyText(n,'volatility')}}<small v-if="weeklyOld(n)" class="old-tag">旧</small></span></template>
          <template v-if="showSites"><span v-for="s in sites" :key="s.id" role="cell" class="hd-num" :class="m.metricPoint(n,s.id)?.tone||'empty'" :title="m.siteContext(n,s.id)">{{m.siteMetric(n,s.id)}}</span></template>
          <span v-if="showDownload" role="cell" class="hd-num" :class="m.downloadMetricTone(n)" :title="m.downloadMetricContext(n)">{{m.downloadMetric(n)}}</span>
          <span v-if="!exitMode&&w.project.value==='download'" role="cell" class="hd-num empty">{{m.downloadMetricContext(n)}}</span>
          <span v-if="!exitMode&&w.project.value==='combined'" role="cell" class="hd-num" :class="services(n).tone">{{services(n).label}}</span>
          <template v-if="!exitMode&&w.project.value==='service'"><button v-for="g in unlockGroups" :key="g.id" role="cell" class="hd-svc-cell hd-num" :class="groupSummary(n,g.id).tone" :title="groupSummary(n,g.id).title" @click.stop="focus(n)">{{groupSummary(n,g.id).label}}</button></template>
        </div>
        <div v-if="!m.pageNodes.value.length" class="hd-empty"><Icon name="layers"/><strong>{{!chosen.length?'先选择一个机场':hasSourceNodes?'节点被筛选条件隐藏了':'这个机场还没有节点'}}</strong><button class="hd-btn" @click="!chosen.length?m.picker.value=true:hasSourceNodes?clearFilters():w.page.value='airports'">{{!chosen.length?'选择机场':hasSourceNodes?'清除筛选':'管理订阅'}}</button></div>
      </div>
      <div class="hd-resize" role="separator" aria-orientation="vertical" aria-label="调整检查器宽度" :aria-valuenow="sideWidth" :aria-valuemin="SIDE_MIN" :aria-valuemax="SIDE_MAX" tabindex="0" @pointerdown="startResize" @keydown.left.prevent="setSide(sideWidth+24)" @keydown.right.prevent="setSide(sideWidth-24)" @dblclick="setSide(420)"/>
      <NodeCompare v-if="m.compareOpen.value" class="hd-side"/>
      <NodeInspector v-else-if="focusNode" :key="key(focusNode)" class="hd-side" :node="focusNode" :exit-only="exitMode" :clash-usage="clashUsage" @select-project="selectInspectorProject" @refresh-clash-usage="refreshClashUsage"/>
    </div>

    <footer v-if="!matrixMode" class="hd-foot">
      <span>{{m.visibleNodes.value.length}} 个节点<template v-if="w.selectedKeys.value.length"> · 已选 {{w.selectedKeys.value.length}}</template> · ↑↓ 切换节点 <button class="hd-link hd-select-range" :disabled="m.qualityFiltersPending.value||!m.visibleNodes.value.length" @click="w.toggleAll(m.visibleNodes.value)">{{m.visibleNodes.value.length&&m.visibleNodes.value.every(selected)?'取消范围选择':'全选筛选范围'}}</button></span>
      <span class="hd-legend"><span class="good">正常</span><span class="warn">部分</span><span class="bad">失败</span><span class="empty">未测</span></span>
      <span class="hd-pages"><button class="hd-btn ghost icon" :disabled="m.currentPage.value<=1" aria-label="上一页" @click="m.currentPage.value--">‹</button>{{m.currentPage.value}} / {{m.totalPages.value}}<button class="hd-btn ghost icon" :disabled="m.currentPage.value>=m.totalPages.value" aria-label="下一页" @click="m.currentPage.value++">›</button></span>
    </footer>
  </section>
</template>

<style scoped>
.hd-pill.partial-sites{max-width:100%;white-space:normal;line-height:1.4;border-radius:8px;box-sizing:border-box}
.history-sort{font-size:11px;white-space:nowrap;text-align:left}
.old-tag{margin-left:2px;padding:0 2px;border-radius:2px;background:var(--hd-soft);color:var(--hd-muted);font-size:9px;font-weight:600}
.hd-clash-usage{display:inline-flex;align-items:center;gap:5px;margin-top:5px;padding:3px 7px;border:1px solid var(--hd-line);border-radius:999px;background:var(--hd-panel);font-size:11px;line-height:1.4;color:var(--hd-muted)}.hd-clash-usage{cursor:pointer}.hd-clash-usage:hover{border-color:var(--hd-accent)}.hd-clash-usage strong{color:var(--hd-text)}.hd-clash-usage-dot,.hd-clash-node-dot{display:inline-block;flex:none;width:7px;height:7px;border-radius:50%;background:var(--hd-good)}.hd-clash-node-dot{width:6px;height:6px;margin-right:5px;vertical-align:middle}
.hd-body.exit-mode .hd-tr{padding-top:9px;padding-bottom:9px}.hd-body.exit-mode .hd-exit-cell.good{color:var(--hd-good)}.hd-source{display:block}.hd-exit-cell{font-size:11px;white-space:normal;line-height:1.4}.hd-exit-cell.bad{color:#c6343e;font-weight:600}.hd-exit-cell.warn{color:#ad680b}
.hd .hd-svc-cell.empty{color:var(--hd-empty);font-weight:400}.hd .hd-svc-cell.warn{color:var(--hd-warn)}
</style>

<style scoped>
.history-sort{font-size:11px;white-space:nowrap;text-align:left}.matrix-switch{display:inline-flex;margin:0 0 12px}
.hd-line-cell{display:flex;align-items:flex-start;gap:6px;font-size:11px;line-height:1.45;white-space:normal}
.hd-line-text{min-width:0;color:var(--hd-text,inherit)}
.hd-route-tag{flex:none;margin-top:1px;padding:0 5px;border-radius:4px;border:1px solid var(--hd-line);color:var(--hd-muted);background:var(--hd-panel);font-size:10px;line-height:1.5;white-space:nowrap}
.hd-route-tag.good{color:var(--hd-good);border-color:color-mix(in srgb,var(--hd-good) 35%,var(--hd-line))}
.hd-route-tag.warn{color:#ad680b;border-color:#e3c184;background:#fff6e5}
.hd-ip-cell{display:flex;align-items:center}
.hd-ip-tag{display:inline-block;padding:0 5px;border-radius:4px;border:1px solid var(--hd-line);background:var(--hd-panel-2);color:var(--hd-muted);font-size:10px;line-height:1.6;white-space:nowrap}
.hd-ip-tag.good{color:var(--hd-good);background:var(--hd-good-bg);border-color:color-mix(in srgb,var(--hd-good) 30%,var(--hd-line))}
.hd-ip-tag.warn{color:var(--hd-warn);background:var(--hd-warn-bg);border-color:color-mix(in srgb,var(--hd-warn) 30%,var(--hd-line))}
.hd-ip-tag.empty{background:transparent;border-style:dashed}
</style>
