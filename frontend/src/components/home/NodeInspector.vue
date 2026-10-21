<script setup lang="ts">
import {computed,inject} from 'vue'
import {useWorkspace} from '../../workspace'
import {date,key,targets,type NodeOption,type Project} from '../../domain'
import {nodeDisplayName} from '../../nodePresentation'
import {monitorHTTPServices,type TrendPoint} from '../../presentation'
import Icon from '../Icon.vue'
import HealthBars from '../HealthBars.vue'
import UnlockInspector from './UnlockInspector.vue'
import {unlockSummary} from '../../unlock'
import InspectorLatency from './InspectorLatency.vue'
import {exitLanding} from '../../exitLanding'
import {stableLatency} from '../../inspectorLatency'
import NodeExitSummary from '../NodeExitSummary.vue'
import MeasurementResults from '../MeasurementResults.vue'
import InlineHistory from '../InlineHistory.vue'
import TrendChart from '../TrendChart.vue'
import {homeModelKey,latencyVerdict,toneLabel} from './homeModel'
import {attemptObservedAt,downloadEvidence} from '../../nodeQuality'
import {attemptStatus} from '../../measurementRounds'
import {downloadSpeed} from '../../domain'
const props=defineProps<{node:NodeOption;exitOnly?:boolean}>()
const w=useWorkspace(),m=inject(homeModelKey)!
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)))
// 检查器跟着项目标签变化：只展开当前项目，其余项目收成底部一行摘要，点一下就切过去。
const project=computed(()=>w.project.value)
const shows=(p:Project)=>!props.exitOnly&&(project.value==='combined'||project.value===p)
const landing=computed(()=>exitLanding(props.node,attempts.value))
const latencyVerdictValue=computed(()=>latencyVerdict(sites.value.map(s=>m.metricPoint(props.node,s.id)),stableLatency(latencySeries.value)))
const serviceVerdict=computed(()=>unlockSummary(w.catalog.value,id=>m.shownService(props.node,id)))
const downloadVerdict=computed(()=>({tone:m.downloadMetricTone(props.node)||'empty',label:m.downloadMetric(props.node),detail:m.downloadMetricContext(props.node)}))
const verdict=computed(()=>props.exitOnly?{tone:landing.value.tone,label:landing.value.label,detail:landing.value.detail}:project.value==='download'?downloadVerdict.value:project.value==='service'?serviceVerdict.value:latencyVerdictValue.value)
const others=computed(()=>props.exitOnly||project.value==='combined'?[]:([
 {id:'latency' as const,name:'延迟',v:latencyVerdictValue.value},
 {id:'download' as const,name:'下载',v:downloadVerdict.value},
 {id:'service' as const,name:'服务',v:serviceVerdict.value},
]).filter(o=>o.id!==project.value).map(o=>({...o,at:m.rowPoints(props.node,o.id).reduce((at,p)=>Date.parse(p.time)>Date.parse(at||'1970-01-01')?p.time:at,'')})))
const runText=computed(()=>props.exitOnly?'检测出口与信誉':project.value==='combined'?'检测这个节点':m.runLabel(project.value))
const attempts=computed(()=>[...new Map([...(m.qualityData.value[key(props.node)]?.services||[]),...(w.services.value[key(props.node)]||[])].map(a=>[a.attempt_id,a])).values()])
const readError=computed(()=>m.qualityErrors.value[key(props.node)])
const downloadRecords=computed(()=>[...new Map([...(w.downloads.value[key(props.node)]||[]),...(m.qualityData.value[key(props.node)]?.downloads||[])].map(a=>[a.attempt_id,a])).values()])
const downloadHistoryContext=computed(()=>{const {latest,valid}=downloadEvidence(downloadRecords.value),speed=downloadSpeed(valid);return valid&&speed!==null?`${valid.attempt_id===latest?.attempt_id?'最近有效速度':'历史有效速度'} ${speed.toFixed(2)} MiB/s（${date(attemptObservedAt(valid))}）`:''})
const downloadLatest=computed(()=>downloadEvidence(downloadRecords.value).latest)
const downloadValue=computed(()=>downloadSpeed(downloadLatest.value))
const downloadSource=computed(()=>{const url=downloadLatest.value?.rule.target_url||'';return url.includes('cachefly')?'Cachefly':url.includes('cloudflare')?'Cloudflare':url?new URL(url).hostname:'测速源未记录'})
const downloadFailure=computed(()=>{
 const a=downloadLatest.value,r=a?.result
 if(readError.value)return '已保存记录读取失败'
 if(!a)return '尚无测速记录'
 if(r?.http_status===429)return '测速网站限流（429），不是节点问题'
 if(r?.http_status===403)return '测速网站拒绝了请求（403）。不是节点的问题。'
 return attemptStatus(a,true)+((r?.error_message||a.not_executed_reason)?'：'+(r?.error_message||a.not_executed_reason):'')
})
const downloadTrend=computed(()=>m.rowPoints(props.node,'download').filter(p=>!p.id.startsWith('planned:')))
const downloadSpark=computed(()=>{
 const points=downloadTrend.value,values=points.flatMap(p=>p.value===null?[]:[p.value]),max=Math.max(1,...values);let started=false
 return points.map((p,i)=>{if(p.value===null){started=false;return ''}const cmd=started?'L':'M';started=true;return `${cmd}${4+i*344/Math.max(1,points.length-1)},${30-p.value/max*26}`}).join(' ')
})
function retryDownload(){
 w.openPlan([props.node]);if(w.plan.value){w.plan.value.projects=['download'];w.plan.value.includeReputation=false;w.plan.value.reputationServiceIds=[];w.plan.value.downloadSource=downloadSource.value==='Cachefly'?'cloudflare':'cachefly'}
}
function windowText(id:string){
 if(readError.value)return '已保存记录读取失败'
 const s=m.siteSummaries.value[key(props.node)]?.find(x=>x.targetId===id)
 if(!s?.total)return ''
 const range=({6:'6 小时',24:'24 小时',168:'7 天',720:'30 天'} as Record<number,string>)[w.hours.value]||w.hours.value+' 小时'
 return `${range}内 ${s.success}/${s.total} 次响应`+(s.p50!=null?` · 中位 ${Math.round(s.p50)} ms`:'')+(s.p95!=null&&s.success>=20?` · 尾部 P95 ${Math.round(s.p95)} ms`:'')
}
const latencySeries=computed(()=>sites.value.map(s=>({...s,points:m.sitePoints(props.node,s.id),context:(windowText(s.id)||m.siteContext(props.node,s.id))+(m.siteReason(props.node,s.id)?' · '+m.siteReason(props.node,s.id):'')})))
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
function run(){m.closeHover();props.exitOnly?w.openReputationPlan([props.node]):w.openPlan([props.node])}
</script>
<template>
  <section class="hd-inspector" :data-project="exitOnly?'exit':project" :aria-label="nodeDisplayName(node)+' 详情'">
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
      <p class="landing-result" :class="landing.tone" :title="landing.detail">{{landing.label}}</p><small class="hd-muted">{{landing.detail}}</small><NodeExitSummary compact :attempts="attempts"/>
    </div>

    <div v-if="shows('latency')" class="hd-ins-block hd-latency">
      <InspectorLatency :series="latencySeries"/>
      <small v-if="m.monitorErrors.value[key(node)]" class="bad">监测记录读取失败</small>
    </div>

    <div v-if="shows('download')" class="hd-ins-block hd-download">
      <h3>下载 <button class="hd-link" @click="openHistory('download',w.displayTarget.value)">记录</button></h3>
      <div v-if="downloadValue!==null&&!readError" class="download-success"><strong>{{downloadValue.toFixed(2)}} <small>MiB/s</small></strong></div>
      <div v-else class="download-failure" :class="{empty:!downloadLatest}"><span>{{downloadFailure}}</span><button v-if="downloadLatest&&!readError" class="hd-btn" :disabled="w.busy.value" @click="retryDownload">换源重测</button></div>
      <small v-if="downloadLatest" class="hd-saved-download" :title="downloadHistoryContext">{{downloadSource}} · {{date(attemptObservedAt(downloadLatest))}}</small>
      <svg v-if="downloadTrend.length>=3" class="download-spark" viewBox="0 0 352 34" role="img" aria-label="已有下载测速走势"><path :d="downloadSpark" fill="none" stroke="var(--hd-accent)" stroke-width="1.8"/><g v-for="(p,i) in downloadTrend" :key="p.id"><title>{{date(p.time)}} · {{p.value===null?'未取得速度':p.value.toFixed(2)+' MiB/s'}}</title><circle v-if="p.value!==null" :cx="4+i*344/Math.max(1,downloadTrend.length-1)" :cy="30-p.value/Math.max(1,...downloadTrend.map(p=>p.value||0))*26" r="2" fill="var(--hd-accent)"/></g></svg>
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
      <h3>解锁检测</h3><UnlockInspector :node="node"/>
    </div>

    <div v-if="!exitOnly&&(showRound||showHistory)&&m.hoverMode.value!=='latency'" class="hd-ins-detail" @mouseenter="m.keepRound" @mouseleave="m.leaveRound" @keydown.esc.stop="m.closeHover">
      <header><strong>{{showRound?'这一轮的数据':detailTitle}}</strong>
        <span>
          <button v-if="showRound" class="hd-link" @click="m.expandHistory">看完整走势</button>
          <button v-if="showHistory" class="hd-link" :disabled="!m.activeRound.value" @click="m.keepOpen.value=true;m.inlineTab.value='round';m.keepRound()">本轮数据</button>
          <button v-if="showRound" class="hd-link" :aria-pressed="m.keepOpen.value" @click="m.keepRound();m.keepOpen.value=!m.keepOpen.value">{{m.keepOpen.value?'已固定 · 取消':'固定这一轮'}}</button>
          <button class="hd-link" @click="m.closeHover">收起</button>
        </span>
      </header>
      <template v-if="showRound&&m.activeRound.value">
        <MeasurementResults :point="m.activeRound.value" :project="m.hoverMode.value"/>
        <TrendChart :points="m.hoverPoints.value" :unit="m.hoverMode.value==='download'?'MiB/s':m.hoverMode.value==='service'?'通过':'ms'" label="节点近期走势"/>
      </template>
      <InlineHistory v-else :node="node" :nodes="m.visibleNodes.value" :project="m.hoverMode.value" :source="m.inlineHistorySource.value" :monitor-service-id="m.hoverService.value" :anchor-points="m.hoverPoints.value" :target-ids="w.displayTargets.value" :service-ids="m.displayServices.value.map(s=>s.service_id)"/>
    </div>
    <nav v-if="others.length" class="hd-ins-others" aria-label="其他项目摘要"><span>其他项目</span><button v-for="o in others" :key="o.id" type="button" :title="'切到'+o.name+'：'+o.v.detail+' · '+date(o.at)" @click="w.project.value=o.id"><small>{{o.name}}</small><strong :class="o.v.tone">{{o.v.label}}</strong><time v-if="o.at" :datetime="o.at">{{date(o.at)}}</time><small v-else>尚未测量</small></button></nav>
    <p class="hd-legend"><span class="good">{{toneLabel('good')}}</span><span class="warn">{{toneLabel('warn')}}</span><span class="bad">{{toneLabel('bad')}}</span><span class="empty">{{toneLabel('empty')}}</span></p>
  </section>
</template>

<style scoped>
.hd-ins-head{order:-3}.hd-ins-verdict{order:-2}.hd-inspector[data-project="latency"] .hd-latency,.hd-inspector[data-project="download"] .hd-download,.hd-inspector[data-project="service"] .hd-services-block{order:-1}

.hd-inspector[data-project="service"] .hd-ins-verdict{display:block}.hd-inspector[data-project="service"] .hd-ins-verdict strong{display:block;font-size:13px;line-height:1.5}.hd-inspector[data-project="service"] .hd-ins-verdict span{display:block;margin-top:4px;font-size:11px}.download-success strong{font-size:25px;font-weight:600;font-variant-numeric:tabular-nums}.download-success small{font-size:12px;font-weight:400;color:var(--hd-muted)}.download-failure{display:flex;align-items:center;flex-wrap:wrap;gap:8px;padding:10px 12px;background:#fff0d3;color:#805211;border-radius:8px;font-size:12px;line-height:1.55}.download-failure span{white-space:nowrap;font-size:11px}.download-failure .hd-btn{margin-left:auto;flex-shrink:0;background:transparent;border-color:#cfad72;color:inherit}.download-failure.empty{padding:0;background:none;color:var(--hd-muted)}.hd-saved-download{display:block;margin-top:7px;color:var(--hd-muted);font-size:10px}.download-spark{display:block;width:100%;height:34px;margin-top:8px}
</style>
