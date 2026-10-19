<script setup lang="ts">
import {computed,inject} from 'vue'
import {useWorkspace} from '../../workspace'
import {key,targets,type NodeOption,type Project} from '../../domain'
import {nodeDisplayName} from '../../nodePresentation'
import type {TrendPoint} from '../../presentation'
import Icon from '../Icon.vue'
import HealthBars from '../HealthBars.vue'
import NodeExitSummary from '../NodeExitSummary.vue'
import RoundResults from '../RoundResults.vue'
import MeasurementResults from '../MeasurementResults.vue'
import InlineHistory from '../InlineHistory.vue'
import {homeModelKey,latencyVerdict,toneLabel} from './homeModel'
const props=defineProps<{node:NodeOption;closable?:boolean}>(),emit=defineEmits<{close:[]}>()
const w=useWorkspace(),m=inject(homeModelKey)!
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)))
const verdict=computed(()=>latencyVerdict(sites.value.map(s=>m.metricPoint(props.node,s.id))))
const attempts=computed(()=>m.qualityData.value[key(props.node)]?.services||w.services.value[key(props.node)]||[])
const mine=computed(()=>!!m.hoverNode.value&&key(m.hoverNode.value)===key(props.node))
const showRound=computed(()=>mine.value&&m.inlineTab.value==='round'&&!!m.activeRound.value)
const showHistory=computed(()=>mine.value&&m.inlineTab.value==='history')
const detailTitle=computed(()=>m.hoverMode.value==='latency'?(targets.find(t=>t.id===m.hoverTarget.value)?.name||'延迟')+' · 历史':m.hoverMode.value==='download'?'下载 · 历史':(w.catalog.value.find(s=>s.service_id===m.hoverService.value)?.name||'服务')+' · 历史')
function openHistory(project:Exclude<Project,'combined'>,target='all',serviceId=''){
 m.closeHover();m.hoverNode.value=props.node;m.hoverMode.value=project;m.hoverTarget.value=target;m.hoverService.value=serviceId;m.activeRound.value=null;m.keepOpen.value=true;m.inlineTab.value='history'
}
function inspect(p:Project,point:TrendPoint,target?:string){m.inspectRound(props.node,p,point,target)}
function open(p:Project,point:TrendPoint,target?:string){m.openRound(props.node,p,point,target)}
function run(){m.closeHover();w.openPlan([props.node])}
</script>
<template>
  <section class="hd-inspector" :aria-label="nodeDisplayName(node)+' 详情'" @keydown.esc.stop="emit('close')">
    <header class="hd-ins-head">
      <div class="hd-ins-name">
        <span class="hd-flag">{{node.country_code||'—'}}</span>
        <div><h2>{{nodeDisplayName(node)}}</h2><p>{{m.sourceLabel(node)}} · {{node.type}}</p></div>
      </div>
      <div class="hd-ins-actions">
        <button class="hd-btn primary" :disabled="w.busy.value" @click="run"><Icon name="play"/>检测这个节点</button>
        <button v-if="closable" class="hd-btn ghost icon" aria-label="收起详情" @click="emit('close')"><Icon name="close"/></button>
      </div>
    </header>
    <div class="hd-ins-verdict" :class="verdict.tone"><strong>{{verdict.label}}</strong><span>{{verdict.detail}}</span></div>

    <div class="hd-ins-block">
      <h3>出口与 IP 信誉</h3>
      <NodeExitSummary compact :attempts="attempts"/>
    </div>

    <div class="hd-ins-block">
      <h3>延迟 <small>每格一轮 · 左旧右新 · 悬停看这一轮 · 点击固定</small></h3>
      <div v-for="s in sites" :key="s.id" class="hd-site">
        <button class="hd-site-name" :title="'查看 '+s.name+' 的历史'" @click="openHistory('latency',s.id)">{{s.name}}</button>
        <button class="hd-site-value" :class="m.metricPoint(node,s.id)?.tone||'empty'" :title="m.siteContext(node,s.id)+(m.siteReason(node,s.id)?' · '+m.siteReason(node,s.id):'')" @click="openHistory('latency',s.id)">{{m.siteMetric(node,s.id)}}</button>
        <HealthBars defer :points="m.sitePoints(node,s.id)" :label="s.name+' 延迟轮次'" @inspect="inspect('latency',$event,s.id)" @leave="m.leaveRound" @open="open('latency',$event,s.id)"/>
      </div>
    </div>

    <div class="hd-ins-block">
      <h3>下载</h3>
      <div class="hd-site">
        <button class="hd-site-name" @click="openHistory('download',w.displayTarget.value)">最近一次</button>
        <button class="hd-site-value" :class="m.downloadMetricTone(node)" :title="m.downloadMetricContext(node)" @click="openHistory('download',w.displayTarget.value)">{{m.downloadMetric(node)}}</button>
        <HealthBars defer :points="m.rowPoints(node,'download')" label="下载轮次" @inspect="inspect('download',$event)" @leave="m.leaveRound" @open="open('download',$event)"/>
      </div>
    </div>

    <div class="hd-ins-block">
      <h3>服务 <small v-if="m.displayServices.value.length">点名称看判断依据</small></h3>
      <p v-if="!m.displayServices.value.length" class="hd-muted">还没选服务。在上方「选择服务」里挑想测的服务。</p>
      <div v-else class="hd-service-grid">
        <button v-for="s in m.displayServices.value" :key="s.service_id" class="hd-service" :class="m.serviceTone(node,s.service_id)" @click="m.showService(node,s.service_id)">
          <span>{{s.name}}</span><strong>{{m.serviceMetric(node,s.service_id)}}</strong>
        </button>
      </div>
    </div>

    <div v-if="showRound||showHistory" class="hd-ins-detail" @mouseenter="m.keepRound" @mouseleave="m.leaveRound">
      <header><strong>{{showRound?'这一轮的数据':detailTitle}}</strong>
        <span>
          <button v-if="showRound" class="hd-link" @click="m.expandHistory">看完整走势</button>
          <button v-if="showRound" class="hd-link" :aria-pressed="m.keepOpen.value" @click="m.keepRound();m.keepOpen.value=!m.keepOpen.value">{{m.keepOpen.value?'已固定 · 取消':'固定这一轮'}}</button>
          <button class="hd-link" @click="m.closeHover">收起</button>
        </span>
      </header>
      <template v-if="showRound&&m.activeRound.value">
        <RoundResults v-if="m.hoverMode.value==='latency'" :point="m.activeRound.value"/>
        <MeasurementResults v-else :point="m.activeRound.value" :project="m.hoverMode.value"/>
      </template>
      <InlineHistory v-else :node="node" :nodes="m.visibleNodes.value" :project="m.hoverMode.value" :source="m.inlineHistorySource.value" :monitor-service-id="m.hoverService.value" :anchor-points="m.hoverPoints.value" :target-ids="m.hoverMode.value==='latency'&&m.hoverTarget.value!=='all'?[m.hoverTarget.value]:w.displayTargets.value" :service-ids="m.displayServices.value.map(s=>s.service_id)"/>
    </div>
    <p class="hd-legend"><span class="good">{{toneLabel('good')}}</span><span class="warn">{{toneLabel('warn')}}</span><span class="bad">{{toneLabel('bad')}}</span><span class="empty">{{toneLabel('empty')}}</span></p>
  </section>
</template>
