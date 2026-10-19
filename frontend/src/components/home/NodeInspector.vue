<script setup lang="ts">
import {computed,inject} from 'vue'
import {useWorkspace} from '../../workspace'
import {date,key,targets,type NodeOption,type Project} from '../../domain'
import {nodeDisplayName} from '../../nodePresentation'
import {monitorHTTPServices,type TrendPoint} from '../../presentation'
import Icon from '../Icon.vue'
import HealthBars from '../HealthBars.vue'
import NodeExitSummary from '../NodeExitSummary.vue'
import RoundResults from '../RoundResults.vue'
import MeasurementResults from '../MeasurementResults.vue'
import InlineHistory from '../InlineHistory.vue'
import TrendChart from '../TrendChart.vue'
import {homeModelKey,latencyVerdict,toneLabel} from './homeModel'
import {attemptObservedAt,downloadEvidence} from '../../nodeQuality'
import {attemptStatus} from '../../measurementRounds'
import {downloadSpeed} from '../../domain'
const props=defineProps<{node:NodeOption}>()
const w=useWorkspace(),m=inject(homeModelKey)!
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)))
// 检查器跟着项目标签变化：只展开当前项目，其余项目收成底部一行摘要，点一下就切过去。
const project=computed(()=>w.project.value)
const shows=(p:Project)=>project.value==='combined'||project.value===p
const latencyVerdictValue=computed(()=>latencyVerdict(sites.value.map(s=>m.metricPoint(props.node,s.id))))
const serviceVerdict=computed(()=>{
 const list=m.displayServices.value
 if(!list.length)return {tone:'empty',label:'未选服务',detail:'在上方「选择服务」里挑想测的服务'}
 const tones=list.map(s=>m.serviceTone(props.node,s.service_id)),measured=tones.filter(t=>t!=='empty').length,pass=tones.filter(t=>t==='good').length
 if(!measured)return {tone:'empty',label:'还没测',detail:`已选 ${list.length} 项服务，这个节点都没有记录`}
 return {tone:pass===list.length?'good':pass||tones.some(t=>t==='reachable'||t==='warn')?'warn':'bad',label:`${pass}/${list.length} 通过`,detail:measured<list.length?`${list.length-measured} 项还没测`:'已选服务都有记录'}
})
const downloadVerdict=computed(()=>({tone:m.downloadMetricTone(props.node)||'empty',label:m.downloadMetric(props.node),detail:m.downloadMetricContext(props.node)}))
const verdict=computed(()=>project.value==='download'?downloadVerdict.value:project.value==='service'?serviceVerdict.value:latencyVerdictValue.value)
const others=computed(()=>project.value==='combined'?[]:([
 {id:'latency' as const,name:'延迟',v:latencyVerdictValue.value},
 {id:'download' as const,name:'下载',v:downloadVerdict.value},
 {id:'service' as const,name:'服务',v:serviceVerdict.value},
]).filter(o=>o.id!==project.value))
const runText=computed(()=>project.value==='combined'?'检测这个节点':m.runLabel(project.value))
const attempts=computed(()=>m.qualityData.value[key(props.node)]?.services||w.services.value[key(props.node)]||[])
const readError=computed(()=>m.qualityErrors.value[key(props.node)])
// Saved-history snapshot for download: latest check and the last valid speed stay separate.
const savedDownload=computed(()=>{
 const e=m.qualityData.value[key(props.node)]
 if(!e)return ''
 const {latest,valid}=downloadEvidence(e.downloads),speed=downloadSpeed(valid)
 const parts=['最近检测：'+(latest?attemptStatus(latest,true):'未测')]
 if(valid&&speed!==null)parts.push(`${valid.attempt_id===latest?.attempt_id?'最近有效速度':'历史有效速度'} ${speed.toFixed(2)} MiB/s（${date(attemptObservedAt(valid))}）`)
 else parts.push(latest?'本次未取得有效速度':'尚无测速记录')
 if(!e.reads.download.complete)parts.push('下载读取不完整，状态未确认')
 return parts.join(' · ')
})
function windowText(id:string){
 if(readError.value)return '已保存记录读取失败'
 const s=m.siteSummaries.value[key(props.node)]?.find(x=>x.targetId===id)
 if(!s?.total)return ''
 const range=({6:'6 小时',24:'24 小时',168:'7 天',720:'30 天'} as Record<number,string>)[w.hours.value]||w.hours.value+' 小时'
 return `${range}内 ${s.success}/${s.total} 次响应`+(s.p50!=null?` · 中位 ${Math.round(s.p50)} ms`:'')+(s.p95!=null&&s.success>=20?` · 尾部 P95 ${Math.round(s.p95)} ms`:'')
}
const mine=computed(()=>!!m.hoverNode.value&&key(m.hoverNode.value)===key(props.node))
const showRound=computed(()=>mine.value&&m.inlineTab.value==='round'&&!!m.activeRound.value)
const showHistory=computed(()=>mine.value&&m.inlineTab.value==='history')
const detailTitle=computed(()=>m.hoverMode.value==='latency'?(targets.find(t=>t.id===m.hoverTarget.value)?.name||'延迟')+' · 历史':m.hoverMode.value==='download'?'下载 · 历史':(w.catalog.value.find(s=>s.service_id===m.hoverService.value)?.name||'服务')+' · 历史')
function openHistory(project:Exclude<Project,'combined'>,target='all',serviceId=''){
 m.closeHover();m.hoverNode.value=props.node;m.hoverMode.value=project;m.hoverTarget.value=target;m.hoverService.value=serviceId;m.activeRound.value=null;m.keepOpen.value=true;m.inlineTab.value='history'
}
function inspect(p:Project,point:TrendPoint,target?:string,serviceId?:string){m.inspectRound(props.node,p,point,target,serviceId)}
function open(p:Project,point:TrendPoint,target?:string,serviceId?:string){m.openRound(props.node,p,point,target,serviceId)}
const httpError=computed(()=>monitorHTTPServices.some(s=>m.monitorHTTPErrors.value[key(props.node)+':'+s.probeType]))
const hasHTTP=computed(()=>m.httpPoints(props.node).length>0||httpError.value)
function httpMetric(id:string){const p=m.httpPoints(props.node,id).at(-1);return p?p.tone==='good'?'HTTP 可达':'失败':'未测'}
function run(){m.closeHover();w.openPlan([props.node])}
</script>
<template>
  <section class="hd-inspector" :data-project="project" :aria-label="nodeDisplayName(node)+' 详情'">
    <header class="hd-ins-head">
      <div class="hd-ins-name">
        <span class="hd-flag">{{node.country_code||'—'}}</span>
        <div><h2>{{nodeDisplayName(node)}}</h2><p>{{m.sourceLabel(node)}} · {{node.type}}</p></div>
      </div>
      <div class="hd-ins-actions">
        <button class="hd-btn primary" :disabled="w.busy.value" :title="project==='combined'?'按组合检测配置这个节点':'只'+m.runLabel(project)+'，范围是这个节点'" @click="run"><Icon name="play"/>{{runText}}</button>
      </div>
    </header>
    <div class="hd-ins-verdict" :class="verdict.tone"><strong>{{verdict.label}}</strong><span>{{verdict.detail}}</span></div>

    <div class="hd-ins-block hd-exit">
      <h3>出口与 IP 信誉</h3>
      <NodeExitSummary compact :attempts="attempts"/>
    </div>

    <div v-if="shows('latency')" class="hd-ins-block hd-latency">
      <h3>延迟 <button class="hd-link" @click="openHistory('latency','all')">完整走势</button> <small>每格一轮 · 左旧右新 · 悬停看这一轮 · 点击固定</small><small v-if="m.monitorErrors.value[key(node)]" class="bad">监测记录读取失败</small><small v-else-if="m.monitorData.value[key(node)]?.length">含持续监测 · 各站独立记录</small></h3>
      <div v-for="s in sites" :key="s.id" class="hd-site">
        <button class="hd-site-name" :title="'查看 '+s.name+' 的历史'" @click="openHistory('latency',s.id)">{{s.name}}</button>
        <button class="hd-site-value" :class="m.metricPoint(node,s.id)?.tone||'empty'" :title="m.siteContext(node,s.id)+(m.siteReason(node,s.id)?' · '+m.siteReason(node,s.id):'')" @click="openHistory('latency',s.id)">{{m.siteMetric(node,s.id)}}</button>
        <HealthBars defer :points="m.sitePoints(node,s.id)" :label="s.name+' 延迟轮次'" @inspect="inspect('latency',$event,s.id)" @leave="m.leaveRound" @open="open('latency',$event,s.id)"/>
        <small v-if="windowText(s.id)" class="hd-site-window">{{windowText(s.id)}}</small>
      </div>
    </div>

    <div v-if="shows('download')" class="hd-ins-block hd-download">
      <h3>下载</h3>
      <div class="hd-site">
        <button class="hd-site-name" @click="openHistory('download',w.displayTarget.value)">最近一次</button>
        <button class="hd-site-value" :class="m.downloadMetricTone(node)" :title="m.downloadMetricContext(node)" @click="openHistory('download',w.displayTarget.value)">{{m.downloadMetric(node)}}</button>
        <HealthBars defer :points="m.rowPoints(node,'download')" label="下载轮次" @inspect="inspect('download',$event)" @leave="m.leaveRound" @open="open('download',$event)"/>
        <small class="hd-site-window hd-download-context">{{m.downloadMetricContext(node)}}</small>
        <small v-if="readError||savedDownload" class="hd-site-window hd-saved-download" :class="{bad:readError}">{{readError?'已保存记录读取失败':savedDownload}}</small>
      </div>
    </div>

    <div v-if="hasHTTP&&shows('service')" class="hd-ins-block hd-http">
      <h3>基础 HTTP 监测 <small>持续监测保存的端点结果，不代表服务目录检测通过</small></h3>
      <p v-if="httpError" class="hd-muted bad" role="alert">监测 HTTP 记录读取失败</p>
      <div v-for="s in monitorHTTPServices" :key="s.id" class="hd-site">
        <span class="hd-site-name">{{s.name}}</span>
        <span class="hd-site-value" :class="m.httpPoints(node,s.id).at(-1)?.tone||'empty'" :title="date(m.httpPoints(node,s.id).at(-1)?.time)">{{httpMetric(s.id)}}</span>
        <HealthBars :points="m.httpPoints(node,s.id)" :label="s.name" @inspect="inspect('service',$event,w.displayTarget.value,s.id)" @leave="m.leaveRound" @open="open('service',$event,w.displayTarget.value,s.id)"/>
      </div>
    </div>

    <div v-if="shows('service')" class="hd-ins-block hd-services-block">
      <h3>服务 <small v-if="m.displayServices.value.length">点名称或结论看判断依据</small></h3>
      <p v-if="!m.displayServices.value.length&&project==='combined'" class="hd-muted">还没选服务。在上方「选择服务」里挑想测的服务。</p>
      <div v-for="s in m.displayServices.value" :key="s.service_id" class="hd-site hd-service-row">
        <button class="hd-site-name" :title="s.name" @click="m.showService(node,s.service_id)">{{s.name}}</button>
        <button class="hd-site-value" :class="m.serviceTone(node,s.service_id)" @click="m.showService(node,s.service_id)">{{m.serviceMetric(node,s.service_id)}}</button>
        <HealthBars defer :points="m.servicePoints(node,s.service_id)" :label="s.name+' 服务轮次'" @inspect="inspect('service',$event,w.displayTarget.value,s.service_id)" @leave="m.leaveRound" @open="open('service',$event,w.displayTarget.value,s.service_id)"/>
      </div>
    </div>

    <div v-if="showRound||showHistory" class="hd-ins-detail" @mouseenter="m.keepRound" @mouseleave="m.leaveRound" @keydown.esc.stop="m.closeHover">
      <header><strong>{{showRound?'这一轮的数据':detailTitle}}</strong>
        <span>
          <button v-if="showRound" class="hd-link" @click="m.expandHistory">看完整走势</button>
          <button v-if="showHistory" class="hd-link" :disabled="!m.activeRound.value" @click="m.keepOpen.value=true;m.inlineTab.value='round';m.keepRound()">本轮数据</button>
          <button v-if="showRound" class="hd-link" :aria-pressed="m.keepOpen.value" @click="m.keepRound();m.keepOpen.value=!m.keepOpen.value">{{m.keepOpen.value?'已固定 · 取消':'固定这一轮'}}</button>
          <button class="hd-link" @click="m.closeHover">收起</button>
        </span>
      </header>
      <template v-if="showRound&&m.activeRound.value">
        <RoundResults v-if="m.hoverMode.value==='latency'" :point="m.activeRound.value"/>
        <MeasurementResults v-else :point="m.activeRound.value" :project="m.hoverMode.value"/>
        <TrendChart :points="m.hoverPoints.value" :unit="m.hoverMode.value==='download'?'MiB/s':m.hoverMode.value==='service'?'通过':'ms'" label="节点近期走势"/>
      </template>
      <InlineHistory v-else :node="node" :nodes="m.visibleNodes.value" :project="m.hoverMode.value" :source="m.inlineHistorySource.value" :monitor-service-id="m.hoverService.value" :anchor-points="m.hoverPoints.value" :target-ids="m.hoverMode.value==='latency'&&m.hoverTarget.value!=='all'?[m.hoverTarget.value]:w.displayTargets.value" :service-ids="m.displayServices.value.map(s=>s.service_id)"/>
    </div>
    <nav v-if="others.length" class="hd-ins-others" aria-label="其他项目摘要"><span>其他项目</span><button v-for="o in others" :key="o.id" type="button" :title="'切到'+o.name+'：'+o.v.detail" @click="w.project.value=o.id"><small>{{o.name}}</small><strong :class="o.v.tone">{{o.v.label}}</strong></button></nav>
    <p class="hd-legend"><span class="good">{{toneLabel('good')}}</span><span class="warn">{{toneLabel('warn')}}</span><span class="bad">{{toneLabel('bad')}}</span><span class="empty">{{toneLabel('empty')}}</span></p>
  </section>
</template>
