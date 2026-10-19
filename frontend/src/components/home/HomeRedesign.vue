<script setup lang="ts">
import {computed,inject,ref,watch} from 'vue'
import '../../home-design.css'
import {useWorkspace} from '../../workspace'
import {isReputationRule,key,projectNames,targets,type NodeOption,type Project} from '../../domain'
import {nodeDisplayName,nodeRegionLabel} from '../../nodePresentation'
import Icon from '../Icon.vue'
import UiSelect from '../UiSelect.vue'
import ServicePicker from '../ServicePicker.vue'
import SubscriptionOverview from '../SubscriptionOverview.vue'
import NodeExitSummary from '../NodeExitSummary.vue'
import NodeActivity from '../NodeActivity.vue'
import NodeInspector from './NodeInspector.vue'
import {homeModelKey,latencyVerdict,type HomeDesign,type Verdict} from './homeModel'
const props=defineProps<{design:Exclude<HomeDesign,'current'>}>(),emit=defineEmits<{design:[d:HomeDesign]}>()
const w=useWorkspace(),m=inject(homeModelKey)!
const designs:{id:HomeDesign;label:string}[]=[{id:'a',label:'A 检查器'},{id:'b',label:'B 成绩单'},{id:'c',label:'C 卡片墙'},{id:'current',label:'现版'}]
const chosen=computed(()=>w.airports.value.filter(a=>w.selectedAirportIds.value?.includes(a.id)))
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)))
const catalog=computed(()=>w.catalog.value.filter(r=>!isReputationRule(r)))
const projects:Project[]=['combined','latency','download','service']
const sorts:{id:string;label:string}[]=[{id:'region',label:'地区 → 机场'},{id:'airport',label:'机场 → 地区'},{id:'name',label:'名称'},{id:'latency',label:'延迟（已保存）'},{id:'download',label:'下载（最近）'},{id:'service',label:'服务通过'}]
const focusKey=ref<string|null>(null)
const focusNode=computed(()=>m.pageNodes.value.find(n=>key(n)===focusKey.value)||(props.design==='a'?m.pageNodes.value[0]:undefined))
watch(()=>props.design,()=>{focusKey.value=null})
function toggleFocus(n:NodeOption){m.closeHover();focusKey.value=props.design!=='a'&&focusKey.value===key(n)?null:key(n)}
function isFocus(n:NodeOption){return !!focusNode.value&&key(focusNode.value)===key(n)}
function selected(n:NodeOption){return w.selectedKeys.value.includes(key(n))}
const verdicts=computed<Record<string,Verdict>>(()=>Object.fromEntries(m.pageNodes.value.map(n=>[key(n),latencyVerdict(sites.value.map(s=>m.metricPoint(n,s.id)))])))
function download(n:NodeOption){return {tone:m.downloadMetricTone(n),label:m.downloadMetric(n)}}
function services(n:NodeOption){
 const list=m.displayServices.value
 if(!list.length)return {tone:'empty',label:'未选服务'}
 const tones=list.map(s=>m.serviceTone(n,s.service_id)),measured=tones.filter(t=>t!=='empty').length,pass=tones.filter(t=>t==='good').length
 if(!measured)return {tone:'empty',label:'未测'}
 return {tone:pass===list.length?'good':pass||tones.some(t=>t==='reachable'||t==='warn')?'warn':'bad',label:`${pass}/${list.length} 通过`}
}
function attempts(n:NodeOption){return m.qualityData.value[key(n)]?.services||w.services.value[key(n)]||[]}
function run(){if(w.project.value==='combined')m.openCombinedPlan();else{m.closeHover();w.openPlan([...m.runNodes.value])}}
const allOnPage=computed(()=>m.pageNodes.value.length>0&&m.pageNodes.value.every(selected))
</script>
<template>
  <section class="hd" :data-design="design">
    <header class="hd-head">
      <div class="hd-title">
        <h1>线路工作台</h1>
        <p><button class="hd-link" @click="m.picker.value=true">{{chosen.length?m.chosenNames.value:'选择机场'}}</button> · {{w.proxyNodes.value.length}} 个节点</p>
      </div>
      <nav class="hd-design-switch" aria-label="首页设计"><span>设计</span><button v-for="d in designs" :key="d.id" :aria-pressed="design===d.id" @click="emit('design',d.id)">{{d.label}}</button></nav>
    </header>

    <details class="hd-accounts"><summary><Icon name="layers"/>账户、流量与公告<small>{{chosen.map(a=>a.name).join(' · ')||'未选择机场'}}</small></summary><SubscriptionOverview :airports="chosen" :notices="w.notices.value"/></details>

    <div class="hd-toolbar">
      <nav class="hd-seg" aria-label="检测项目"><button v-for="p in projects" :key="p" :aria-pressed="w.project.value===p" @click="w.project.value=p">{{projectNames[p]}}</button></nav>
      <label class="hd-search"><Icon name="search"/><input v-model="w.search.value" type="search" placeholder="搜索节点、机场、地区" aria-label="搜索节点"></label>
      <UiSelect v-model="w.region.value" aria-label="地区"><option value="all">全部地区</option><option v-for="r in m.filterRegions.value" :key="r" :value="r">{{nodeRegionLabel(r)}}</option></UiSelect>
      <UiSelect v-model="w.sort.value" aria-label="排序"><option v-for="s in sorts" :key="s.id" :value="s.id">排序：{{s.label}}</option></UiSelect>
      <UiSelect v-model.number="w.hours.value" aria-label="时间范围"><option :value="6">最近 6 小时</option><option :value="24">最近 24 小时</option><option :value="168">最近 7 天</option><option :value="720">最近 30 天</option></UiSelect>
      <button class="hd-btn primary hd-run" :disabled="w.busy.value||!m.runNodes.value.length" @click="run"><Icon name="play"/>{{m.runLabel(w.project.value)}} · {{m.runNodes.value.length}} 个</button>
    </div>
    <details v-if="w.project.value==='service'||w.project.value==='combined'" class="hd-services"><summary>选择服务<small>{{m.displayServices.value.length?m.displayServices.value.map(s=>s.name).join(' · '):'还没选'}}</small></summary><ServicePicker v-model="w.serviceIds.value" :catalog="catalog"/></details>

    <div v-if="w.selectedKeys.value.length" class="hd-selection" aria-live="polite">
      <strong>已选 {{w.selectedKeys.value.length}} 个</strong>
      <button class="hd-btn" :disabled="w.selectedKeys.value.length<2" @click="m.openComparison()">比较</button>
      <button class="hd-btn" @click="w.exportSelection()">导出配置</button>
      <button class="hd-link" @click="w.selectedKeys.value=[]">清空</button>
    </div>
    <div v-if="w.busy.value" class="hd-live"><span class="spinner"/>检测进行中{{w.queue.value.length?` · ${w.done.value}/${w.queue.value.length}`:''}}<button class="hd-link" @click="w.queueOpen.value=true">查看进度</button><button class="hd-link bad" @click="w.cancelTests()">{{w.cancelRequested.value?'正在取消…':'取消'}}</button></div>

    <div class="hd-body">
      <!-- A：密集表格 + 右侧检查器 -->
      <div v-if="design==='a'" class="hd-table" role="table" aria-label="节点列表">
        <div class="hd-tr hd-th" role="row">
          <span role="columnheader"><input type="checkbox" :checked="allOnPage" aria-label="选择本页全部节点" @change="w.toggleAll(m.pageNodes.value)"></span>
          <span role="columnheader">节点</span><span role="columnheader">结论</span>
          <span v-for="s in sites" :key="s.id" role="columnheader">{{s.name}}</span>
          <span role="columnheader">下载</span><span role="columnheader">服务</span>
        </div>
        <div v-for="n in m.pageNodes.value" :key="key(n)" class="hd-tr" role="row" :class="{focus:isFocus(n),selected:selected(n)}" @click="toggleFocus(n)">
          <span role="cell" @click.stop><input type="checkbox" :checked="selected(n)" :aria-label="'选择 '+nodeDisplayName(n)" @change="w.toggleNode(n)"></span>
          <span role="cell" class="hd-node"><span class="hd-flag">{{n.country_code||'—'}}</span><span><strong>{{nodeDisplayName(n)}}</strong><small>{{m.sourceLabel(n)}} · {{n.type}}</small></span><NodeActivity :state="m.nodeActivities.value[key(n)]?.state" :label="m.nodeActivities.value[key(n)]?.label"/></span>
          <span role="cell"><b class="hd-pill" :class="verdicts[key(n)].tone">{{verdicts[key(n)].label}}</b></span>
          <span v-for="s in sites" :key="s.id" role="cell" class="hd-num" :class="m.metricPoint(n,s.id)?.tone||'empty'">{{m.siteMetric(n,s.id)}}</span>
          <span role="cell" class="hd-num" :class="download(n).tone">{{download(n).label}}</span>
          <span role="cell" class="hd-num" :class="services(n).tone">{{services(n).label}}</span>
        </div>
      </div>
      <NodeInspector v-if="design==='a'&&focusNode" :key="key(focusNode)" class="hd-side" :node="focusNode"/>

      <!-- B：成绩单 -->
      <div v-if="design==='b'" class="hd-report">
        <template v-for="n in m.pageNodes.value" :key="key(n)">
          <article class="hd-report-row" :class="{focus:isFocus(n),selected:selected(n)}">
            <input type="checkbox" :checked="selected(n)" :aria-label="'选择 '+nodeDisplayName(n)" @change="w.toggleNode(n)">
            <button class="hd-grade" :class="verdicts[key(n)].tone" :aria-expanded="isFocus(n)" @click="toggleFocus(n)"><strong>{{verdicts[key(n)].label}}</strong><small>{{verdicts[key(n)].detail}}</small></button>
            <button class="hd-report-name" :aria-expanded="isFocus(n)" @click="toggleFocus(n)"><span class="hd-flag">{{n.country_code||'—'}}</span><span><strong>{{nodeDisplayName(n)}}</strong><small>{{m.sourceLabel(n)}} · {{n.type}}</small></span></button>
            <div class="hd-report-ip"><NodeExitSummary compact :attempts="attempts(n)"/></div>
            <dl class="hd-report-facts">
              <div><dt>延迟</dt><dd><span v-for="s in sites" :key="s.id" class="hd-mini" :class="m.metricPoint(n,s.id)?.tone||'empty'" :title="s.name+' · '+m.siteContext(n,s.id)"><i>{{s.name}}</i>{{m.siteMetric(n,s.id)}}</span></dd></div>
              <div><dt>下载</dt><dd :class="download(n).tone">{{download(n).label}}</dd></div>
              <div><dt>服务</dt><dd :class="services(n).tone">{{services(n).label}}</dd></div>
            </dl>
            <NodeActivity :state="m.nodeActivities.value[key(n)]?.state" :label="m.nodeActivities.value[key(n)]?.label"/>
          </article>
          <NodeInspector v-if="isFocus(n)" class="hd-inline" :node="n" closable @close="toggleFocus(n)"/>
        </template>
      </div>

      <!-- C：卡片墙 -->
      <div v-if="design==='c'" class="hd-cards">
        <template v-for="n in m.pageNodes.value" :key="key(n)">
          <article class="hd-card" :class="[verdicts[key(n)].tone,{focus:isFocus(n),selected:selected(n)}]">
            <header><label class="hd-card-check"><input type="checkbox" :checked="selected(n)" :aria-label="'选择 '+nodeDisplayName(n)" @change="w.toggleNode(n)"></label><b class="hd-pill" :class="verdicts[key(n)].tone">{{verdicts[key(n)].label}}</b><NodeActivity :state="m.nodeActivities.value[key(n)]?.state" :label="m.nodeActivities.value[key(n)]?.label"/></header>
            <button class="hd-card-name" :aria-expanded="isFocus(n)" @click="toggleFocus(n)"><span class="hd-flag">{{n.country_code||'—'}}</span><span><strong>{{nodeDisplayName(n)}}</strong><small>{{m.sourceLabel(n)}} · {{n.type}}</small></span></button>
            <NodeExitSummary compact :attempts="attempts(n)"/>
            <div class="hd-card-sites"><span v-for="s in sites" :key="s.id" class="hd-mini" :class="m.metricPoint(n,s.id)?.tone||'empty'" :title="m.siteContext(n,s.id)"><i>{{s.name}}</i>{{m.siteMetric(n,s.id)}}</span></div>
            <footer><span :class="download(n).tone">下载 {{download(n).label}}</span><span :class="services(n).tone">服务 {{services(n).label}}</span></footer>
          </article>
          <NodeInspector v-if="isFocus(n)" class="hd-inline hd-card-detail" :node="n" closable @close="toggleFocus(n)"/>
        </template>
      </div>

      <div v-if="!m.pageNodes.value.length" class="hd-empty"><Icon name="layers"/><strong>{{!chosen.length?'先选择一个机场':'没有符合条件的节点'}}</strong><button class="hd-btn" @click="!chosen.length?m.picker.value=true:w.search.value=''">{{!chosen.length?'选择机场':'清空搜索'}}</button></div>
    </div>

    <footer class="hd-foot">
      <span>{{m.visibleNodes.value.length}} 个节点<template v-if="w.selectedKeys.value.length"> · 已选 {{w.selectedKeys.value.length}}</template></span>
      <span class="hd-legend"><span class="good">正常</span><span class="warn">部分</span><span class="bad">失败</span><span class="empty">未测</span></span>
      <span class="hd-pages"><button class="hd-btn ghost icon" :disabled="m.currentPage.value<=1" aria-label="上一页" @click="m.currentPage.value--">‹</button>{{m.currentPage.value}} / {{m.totalPages.value}}<button class="hd-btn ghost icon" :disabled="m.currentPage.value>=m.totalPages.value" aria-label="下一页" @click="m.currentPage.value++">›</button></span>
    </footer>
  </section>
</template>
