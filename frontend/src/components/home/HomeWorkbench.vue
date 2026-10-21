<script setup lang="ts">
import {computed,inject,nextTick,ref,watch} from 'vue'
import '../../home-design.css'
import {useWorkspace} from '../../workspace'
import {date,isReputationRule,key,projectNames,targets,type NodeOption,type Project} from '../../domain'
import {nodeDisplayName,nodeRegionLabel} from '../../nodePresentation'
import {airportNodeCount} from '../../airportViewing'
import Icon from '../Icon.vue'
import UiSelect from '../UiSelect.vue'
import UnlockPicker from '../UnlockPicker.vue'
import SubscriptionOverview from '../SubscriptionOverview.vue'
import NodeActivity from '../NodeActivity.vue'
import SiteDisplayPicker from '../SiteDisplayPicker.vue'
import NodeInspector from './NodeInspector.vue'
import NodeCompare from './NodeCompare.vue'
import {homeModelKey,latencyVerdict,type Verdict} from './homeModel'
import {unlockGroups,unlockGroupSummary} from '../../unlock'
const w=useWorkspace(),m=inject(homeModelKey)!
const chosen=m.chosen
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)))
const catalog=computed(()=>w.catalog.value.filter(r=>!isReputationRule(r)))
const projects:Project[]=['combined','latency','download','service']
const chosenNodeCount=computed(()=>chosen.value.reduce((count,airport)=>count+airportNodeCount(airport),0))
const hasSourceNodes=computed(()=>w.proxyNodes.value.some(n=>w.airportFilter.value==='all'||w.nodeAirportId(n)===w.airportFilter.value))
function clearFilters(){w.search.value='';w.region.value='all';w.excludeRegion.value=false;w.subscription.value='all';w.downloadFullOnly.value=false;m.protocolFilter.value='all';m.qualityGate.value='all';m.currentPage.value=1}
const sorts=computed(()=>[{id:'region',label:'地区 → 机场'},{id:'airport',label:'机场 → 地区'},{id:'name',label:'名称'},
 ...(['latency','combined'].includes(w.project.value)?[{id:'latency',label:'已保存延迟 / 监测'}]:[]),
 ...(['download','combined'].includes(w.project.value)?[{id:'download',label:'最近下载速度'}]:[]),
 ...(['service','combined'].includes(w.project.value)?[{id:'service',label:'自选服务通过'}]:[])])
const gates=[{id:'all',label:'全部节点',short:'全部'},{id:'samples',label:'测得够多的（成功样本 ≥5 次）',short:'测得够多的'},{id:'response',label:'稳定能连的（≥5 次且成功率 ≥90%）',short:'稳定能连的'},{id:'fresh',label:'最近 24 小时测过的',short:'最近 24 小时测过的'},{id:'download',label:'有过有效下载速度的',short:'有过有效下载速度的节点'},{id:'service',label:'已选服务检测通过的',short:'已选服务检测通过的节点'}]
// 每个标签只给和它相关的条件；综合标签保留全部。
const tabGateIds:Record<Project,string[]>={combined:['all','samples','response','fresh','download','service'],latency:['all','samples','response','fresh'],download:['all','download'],service:['all','service']}
const tabGates=computed(()=>gates.filter(g=>tabGateIds[w.project.value].includes(g.id)))
const latencyTab=computed(()=>['combined','latency'].includes(w.project.value))
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
function focus(n:NodeOption){if(focusNode.value&&key(focusNode.value)===key(n))return;m.closeHover();focusKey.value=key(n)}
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
watch(()=>w.viewRequest.value,n=>{
 if(!n)return
 w.viewRequest.value=null
 const k=key(n),i=m.visibleNodes.value.findIndex(v=>key(v)===k)
 if(i<0){w.inspectNode.value=n;return}
 m.currentPage.value=Math.floor(i/m.pageSize.value)+1;m.closeHover();focusKey.value=k
 void nextTick(()=>table.value?.querySelector<HTMLElement>('.hd-tr.focus')?.scrollIntoView?.({block:'nearest'}))
},{immediate:true})
function isFocus(n:NodeOption){return !!focusNode.value&&key(focusNode.value)===key(n)}
function selected(n:NodeOption){return w.selectedKeys.value.includes(key(n))}
const verdicts=computed<Record<string,Verdict>>(()=>Object.fromEntries(m.pageNodes.value.map(n=>[key(n),latencyVerdict(sites.value.map(s=>m.metricPoint(n,s.id)))])))
function groupSummary(n:NodeOption,id:string){return unlockGroupSummary(id,catalog.value,serviceId=>m.shownService(n,serviceId))}
function services(n:NodeOption){const counts=unlockGroups.map(g=>({...g,...groupSummary(n,g.id)}));return {tone:counts.some(g=>g.reason)?'warn':counts.some(g=>g.available)?'good':'empty',label:counts.filter(g=>g.total).map(g=>g.name+' '+g.label).join(' · ')||'未选解锁项目'}}
function run(){if(w.project.value==='combined')m.openCombinedPlan();else{m.closeHover();w.openPlan([...m.runNodes.value])}}
const showVerdict=computed(()=>['combined','latency'].includes(w.project.value))
const showSites=showVerdict
const showDownload=computed(()=>['combined','download'].includes(w.project.value))
// Grid tracks follow the active project so each tab shows its own columns.
const columns=computed(()=>['36px','minmax(220px,2.2fr)',...(showVerdict.value?['92px']:[]),...(showSites.value?sites.value.map(()=>'minmax(74px,.8fr)'):[]),...(showDownload.value?['minmax(110px,1fr)']:[]),...(w.project.value==='download'?['minmax(160px,1.4fr)']:[]),...(w.project.value==='combined'?['minmax(80px,.8fr)']:[]),...(w.project.value==='service'?unlockGroups.map(()=>'minmax(90px,1fr)'):[])].join(' '))
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
      </div>
    </header>

    <section class="hd-source" aria-label="机场选择">
      <SubscriptionOverview :airports="chosen" :notices="w.notices.value" :airport-id="w.airportFilter.value" @viewed="id=>{w.airportFilter.value=id}"/>
    </section>

    <div class="hd-toolbar">
      <nav class="hd-seg" aria-label="检测项目"><button v-for="p in projects" :key="p" :aria-pressed="w.project.value===p" @click="w.project.value=p">{{projectNames[p]}}</button></nav>
      <label class="hd-search"><Icon name="search"/><input v-model="w.search.value" type="search" placeholder="搜节点、机场、地区" aria-label="搜索节点、机场或地区" title="可搜索节点名、机场名或地区"></label>
      <UiSelect v-model="w.region.value" aria-label="地区"><option value="all">全部地区</option><option v-for="r in m.filterRegions.value" :key="r" :value="r">{{nodeRegionLabel(r)}}</option></UiSelect>
      <UiSelect v-model="w.sort.value" aria-label="排序"><option v-for="s in sorts" :key="s.id" :value="s.id">排序：{{s.label}}</option></UiSelect>
      <UiSelect v-model.number="w.hours.value" aria-label="时间范围"><option :value="6">最近 6 小时</option><option :value="24">最近 24 小时</option><option :value="168">最近 7 天</option><option :value="720">最近 30 天</option></UiSelect>
      <button class="hd-btn ghost" :disabled="w.loading.value" title="重新载入节点列表" @click="w.refreshNodes().catch(w.fail)"><Icon name="refresh"/>重新载入</button>
      <button class="hd-btn primary hd-run" :disabled="w.busy.value||!m.runNodes.value.length" @click="run"><Icon name="play"/>{{m.runLabel(w.project.value)}} · {{scopeLabel}}</button>
      <button v-if="w.project.value!=='combined'" class="hd-btn hd-run-combined" :disabled="w.busy.value||!m.runNodes.value.length" title="按当前显示的节点范围打开组合检测" @click="m.openCombinedPlan()">组合检测</button>
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
            <label v-if="w.project.value==='download'" class="hd-check"><input v-model="w.downloadFullOnly.value" type="checkbox">只看完整读完的测速</label>
            <template v-if="latencyTab">
              <label>延迟排序<UiSelect v-model="m.qualitySort.value" aria-label="质量分组排序"><option v-for="q in qualitySorts" :key="q.id" :value="q.id">{{q.label}}</option></UiSelect></label>
              <label>延迟按哪个站点算<UiSelect v-model="w.displayTarget.value" aria-label="质量统计站点"><option v-for="s in targets" :key="s.id" :value="s.id">{{s.name}}</option></UiSelect></label>
            </template>
          </div>
          <p class="hd-muted hd-more-note">数据截止 {{date(m.qualityCutoff.value)}} · 各项分别测量，不作综合打分<button class="hd-link" :disabled="m.qualityLoading.value" @click="m.readQualityNodes(qualityFiltering?w.proxyNodes.value:m.pageNodes.value,true)">{{m.qualityLoading.value?'读取中…':'更新质量记录'}}</button></p>
          <p v-if="qualityFiltering" class="hd-muted" role="status">{{m.allQualityReady.value?`已读固定范围 ${w.proxyNodes.value.length} 个节点 · 当前符合 ${m.visibleNodes.value.length} 个 · 读取失败 ${Object.keys(m.qualityErrors.value).length} 个 · 存在缺页/读取错误 ${Object.values(m.qualityData.value).filter(e=>!e.complete).length} 个（只限制对应类型的证据）`:`正在读取全部 ${w.proxyNodes.value.length} 个节点的历史记录（已读 ${Object.keys(m.qualityData.value).length}）；读取完成后应用条件筛选与排序`}}</p>
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
    <details v-if="w.project.value==='service'||w.project.value==='combined'" class="hd-services"><summary>选择解锁项目<small>{{m.displayServices.value.length?'已选 '+m.displayServices.value.length+' 项':'还没选'}}</small></summary><UnlockPicker v-model="w.serviceIds.value" :catalog="catalog"/></details>

    <div v-if="w.selectedKeys.value.length" class="hd-selection" aria-live="polite">
      <strong>已选 {{w.selectedNodes.value.length}} 个节点</strong><small v-if="m.hiddenSelectedCount.value" class="hd-muted">含 {{m.hiddenSelectedCount.value}} 个筛选外节点</small>
      <button class="hd-link" :disabled="m.qualityFiltersPending.value" @click="w.toggleAll(m.visibleNodes.value)">{{m.visibleNodes.value.length&&m.visibleNodes.value.every(selected)?'取消范围选择':'全选筛选范围'}}</button>
      <button class="hd-btn" :disabled="w.selectedNodes.value.length<2" @click="m.openComparison()">比较已选 {{w.selectedNodes.value.length}} 个节点</button>
      <button class="hd-btn primary" :disabled="w.busy.value" @click="w.openPlan()"><Icon name="play"/>检测已选</button>
      <button class="hd-btn" @click="w.monitorNodes.value=w.selectedNodes.value.map(n=>({...n}))"><Icon name="monitor"/>持续监测延迟</button>
      <button class="hd-btn" @click="w.exportSelection().catch(w.fail)">导出配置</button>
      <button class="hd-link" @click="w.selectedKeys.value=[]">清空已选</button>
    </div>
    <div v-if="w.busy.value" class="hd-live"><span class="spinner"/>检测进行中{{w.queue.value.length?` · ${w.done.value}/${w.queue.value.length}`:''}}<button class="hd-link" @click="w.queueOpen.value=true">查看进度</button><button class="hd-link bad" @click="w.cancelTests()">{{w.cancelRequested.value?'正在取消…':'取消'}}</button></div>

    <div class="hd-body" :class="{comparing:m.compareOpen.value,'compare-wide':m.compareOpen.value&&m.compareNodes.value.length>2}" :style="{'--hd-side-w':sideWidth+'px'}">
      <div ref="table" class="hd-table" role="table" aria-label="节点列表" tabindex="0" :style="{'--hd-cols':columns}" @keydown="keyNav">
        <div class="hd-tr hd-th" role="row">
          <span role="columnheader"><input type="checkbox" :checked="allOnPage" aria-label="选择本页全部节点" @change="w.toggleAll(m.pageNodes.value)"></span>
          <span role="columnheader">节点</span>
          <span v-if="showVerdict" role="columnheader">结论</span>
          <template v-if="showSites"><span v-for="s in sites" :key="s.id" role="columnheader">{{s.name}}</span></template>
          <span v-if="showDownload" role="columnheader">下载</span>
          <span v-if="w.project.value==='download'" role="columnheader">测量时间</span>
          <span v-if="w.project.value==='combined'" role="columnheader">服务</span>
          <template v-if="w.project.value==='service'"><span v-for="g in unlockGroups" :key="g.id" role="columnheader">{{g.name}}</span></template>
        </div>
        <div v-for="n in m.pageNodes.value" :key="key(n)" class="hd-tr" role="row" :class="{focus:isFocus(n),selected:selected(n)}" :aria-selected="isFocus(n)" @click="focus(n)">
          <span role="cell" @click.stop><input type="checkbox" :checked="selected(n)" :aria-label="'选择 '+nodeDisplayName(n)" @change="w.toggleNode(n)"></span>
          <span role="cell" class="hd-node" :title="n.display_name+' · '+m.sourceLabel(n)+' · '+n.type"><span class="hd-flag">{{n.country_code||'—'}}</span><span><strong>{{nodeDisplayName(n)}}</strong><small>{{m.sourceLabel(n)}} · {{n.type}}</small></span><NodeActivity :state="m.nodeActivities.value[key(n)]?.state" :label="m.nodeActivities.value[key(n)]?.label"/></span>
          <span v-if="showVerdict" role="cell"><b class="hd-pill" :class="verdicts[key(n)].tone">{{verdicts[key(n)].label}}</b></span>
          <template v-if="showSites"><span v-for="s in sites" :key="s.id" role="cell" class="hd-num" :class="m.metricPoint(n,s.id)?.tone||'empty'" :title="m.siteContext(n,s.id)">{{m.siteMetric(n,s.id)}}</span></template>
          <span v-if="showDownload" role="cell" class="hd-num" :class="m.downloadMetricTone(n)" :title="m.downloadMetricContext(n)">{{m.downloadMetric(n)}}</span>
          <span v-if="w.project.value==='download'" role="cell" class="hd-num empty">{{m.downloadMetricContext(n)}}</span>
          <span v-if="w.project.value==='combined'" role="cell" class="hd-num" :class="services(n).tone">{{services(n).label}}</span>
          <template v-if="w.project.value==='service'"><button v-for="g in unlockGroups" :key="g.id" role="cell" class="hd-svc-cell hd-num" :class="groupSummary(n,g.id).tone" :title="groupSummary(n,g.id).title" @click.stop="focus(n)">{{groupSummary(n,g.id).label}}</button></template>
        </div>
        <div v-if="!m.pageNodes.value.length" class="hd-empty"><Icon name="layers"/><strong>{{!chosen.length?'先选择一个机场':hasSourceNodes?'节点被筛选条件隐藏了':'这个机场还没有节点'}}</strong><button class="hd-btn" @click="!chosen.length?m.picker.value=true:hasSourceNodes?clearFilters():w.page.value='airports'">{{!chosen.length?'选择机场':hasSourceNodes?'清除筛选':'管理订阅'}}</button></div>
      </div>
      <div class="hd-resize" role="separator" aria-orientation="vertical" aria-label="调整检查器宽度" :aria-valuenow="sideWidth" :aria-valuemin="SIDE_MIN" :aria-valuemax="SIDE_MAX" tabindex="0" @pointerdown="startResize" @keydown.left.prevent="setSide(sideWidth+24)" @keydown.right.prevent="setSide(sideWidth-24)" @dblclick="setSide(420)"/>
      <NodeCompare v-if="m.compareOpen.value" class="hd-side"/>
      <NodeInspector v-else-if="focusNode" :key="key(focusNode)" class="hd-side" :node="focusNode"/>
    </div>

    <footer class="hd-foot">
      <span>{{m.visibleNodes.value.length}} 个节点<template v-if="w.selectedKeys.value.length"> · 已选 {{w.selectedKeys.value.length}}</template> · ↑↓ 切换节点 <button class="hd-link hd-select-range" :disabled="m.qualityFiltersPending.value||!m.visibleNodes.value.length" @click="w.toggleAll(m.visibleNodes.value)">{{m.visibleNodes.value.length&&m.visibleNodes.value.every(selected)?'取消范围选择':'全选筛选范围'}}</button></span>
      <span class="hd-legend"><span class="good">正常</span><span class="warn">部分</span><span class="bad">失败</span><span class="empty">未测</span></span>
      <span class="hd-pages"><button class="hd-btn ghost icon" :disabled="m.currentPage.value<=1" aria-label="上一页" @click="m.currentPage.value--">‹</button>{{m.currentPage.value}} / {{m.totalPages.value}}<button class="hd-btn ghost icon" :disabled="m.currentPage.value>=m.totalPages.value" aria-label="下一页" @click="m.currentPage.value++">›</button></span>
    </footer>
  </section>
</template>

<style scoped>
.hd-source{display:block}
</style>
