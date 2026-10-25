<script setup lang="ts">
import {computed,inject,ref,watch} from 'vue'
import {useWorkspace} from '../../workspace'
import {date,key,targets,type NodeOption,type Project} from '../../domain'
import {api,type ClashGroupOption,type ClashNodeStatus,type ClashSelectionResult,type ClashUsage} from '../../api'
import {nodeDisplayName} from '../../nodePresentation'
import {manualLatencyPoint,monitorLatencyPoints,monitorHTTPServices,type TrendPoint} from '../../presentation'
import Icon from '../Icon.vue'
import HealthBars from '../HealthBars.vue'
import UnlockInspector from './UnlockInspector.vue'
import {unlockSummary} from '../../unlock'
import InspectorLatency from './InspectorLatency.vue'
import {serviceIsProfile} from '../../sharedResults'
import {exitLanding} from '../../exitLanding'
import {entryCountry,mainlandNotice,pathSteps} from '../../entryPath'
import {stableLatency,inspectorMedian} from '../../inspectorLatency'
import NodeExitSummary from '../NodeExitSummary.vue'
import MeasurementResults from '../MeasurementResults.vue'
import InlineHistory from '../InlineHistory.vue'
import TrendChart from '../TrendChart.vue'
import Modal from '../Modal.vue'
import {homeModelKey,latencyVerdict,toneLabel} from './homeModel'
import {attemptObservedAt,downloadEvidence} from '../../nodeQuality'
import {attemptStatus} from '../../measurementRounds'
import {downloadSpeed,downloadTarget} from '../../domain'
const props=defineProps<{node:NodeOption;exitOnly?:boolean;clashUsage?:ClashUsage[]}>()
const emit=defineEmits<{(e:'select-project',id:Project|'exit'):void;(e:'refresh-clash-usage'):void}>()
const w=useWorkspace(),m=inject(homeModelKey)!
const isClashUsing=computed(()=>props.clashUsage?.some(usage=>usage.name===props.node.display_name)||false)
const clashStatus=ref<ClashNodeStatus|null>(null),clashStatusLoading=ref(false)
const clashDialog=ref(false),clashGroups=ref<ClashGroupOption[]>([]),selectedClashGroup=ref('')
const clashGroupsLoading=ref(false),clashSelectBusy=ref(false),clashError=ref('')
const clashSelection=ref<ClashSelectionResult|null>(null)
const selectedClashOption=computed(()=>clashGroups.value.find(option=>option.group===selectedClashGroup.value))
async function readClashStatus(){clashStatusLoading.value=true;clashStatus.value=null;try{clashStatus.value=await api.clashNodeStatus(props.node)}catch{clashStatus.value=null}finally{clashStatusLoading.value=false}}
watch(()=>key(props.node),()=>{void readClashStatus()},{immediate:true})
async function openClashDialog(){
 clashDialog.value=true;clashGroups.value=[];selectedClashGroup.value='';clashSelection.value=null;clashError.value='';clashGroupsLoading.value=true
 try{clashGroups.value=await api.clashNodeGroups(props.node)}catch(error){clashError.value=error instanceof Error?error.message:String(error)}finally{clashGroupsLoading.value=false}
}
async function confirmClashSelection(){
 if(!selectedClashGroup.value||clashSelectBusy.value||clashSelection.value?.verified)return
 clashSelectBusy.value=true;clashError.value=''
 try{clashSelection.value=await api.useClashNode(props.node,selectedClashGroup.value);if(clashSelection.value.verified)emit('refresh-clash-usage')}
 catch(error){clashError.value=error instanceof Error?error.message:String(error)}finally{clashSelectBusy.value=false}
}
const reportFocus=computed(()=>w.latencyReportFocus.value?.nodeKey===key(props.node)?w.latencyReportFocus.value:undefined)
const latencyFocus=ref<import('../../latencyHistory').LatencySeries[]|null>(null)
const sites=computed(()=>targets.filter(t=>w.displayTargets.value.includes(t.id)))
// 检查器跟着项目标签变化：只展开当前项目，其余项目收成底部一行摘要，点一下就切过去。
const project=computed(()=>w.project.value)
const shows=(p:Project)=>!props.exitOnly&&(project.value==='combined'||project.value===p)
const landing=computed(()=>exitLanding(props.node,attempts.value))
const nodeEntry=computed(()=>w.entryFor(props.node))
const exitPath=computed(()=>pathSteps(w.localPosition.value||undefined,nodeEntry.value,landing.value.consensusRegion,landing.value.org))
const pathNotice=computed(()=>mainlandNotice(entryCountry(nodeEntry.value),w.localPosition.value?.country_code))
// 出口模式一打开就补一次入口解析；缓存命中时不会发 DNS。组合视图只用已有缓存。
watch(()=>[key(props.node),props.exitOnly],()=>{if(props.exitOnly)void w.ensureEntry(props.node).catch(()=>{})},{immediate:true})
const latencyVerdictValue=computed(()=>latencyVerdict((latencyFocus.value||latencySeries.value).map(s=>s.points.at(-1)),stableLatency(latencyFocus.value||latencySeries.value),m.weeklyStats.value[key(props.node)]?.allOld?null:m.weeklyStats.value[key(props.node)]?.all.responseRate))
const serviceVerdict=computed(()=>unlockSummary(w.catalog.value,id=>m.shownService(props.node,id),w.nodeUnlockRegion(props.node),w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions,landing.value.ipKind.type))
const downloadVerdict=computed(()=>({tone:m.downloadMetricTone(props.node)||'empty',label:m.downloadMetric(props.node),detail:m.downloadMetricContext(props.node)}))
const verdict=computed(()=>props.exitOnly?{tone:landing.value.tone,label:landing.value.label,detail:landing.value.detail}:project.value==='download'?downloadVerdict.value:project.value==='service'?serviceVerdict.value:latencyVerdictValue.value)
const others=computed(()=>!props.exitOnly&&project.value==='combined'?[]:([
 {id:'latency' as const,name:'延迟',v:latencyVerdictValue.value},
 {id:'download' as const,name:'下载',v:downloadVerdict.value},
 {id:'service' as const,name:'解锁',v:serviceVerdict.value},
 {id:'exit' as const,name:'出口与信誉',v:{tone:landing.value.tone,label:landing.value.label,detail:landing.value.detail}},
]).filter(o=>o.id!==(props.exitOnly?'exit':project.value)))
const runText=computed(()=>props.exitOnly?'检测出口与信誉':project.value==='combined'?'检测这个节点':m.runLabel(project.value))
const attempts=computed(()=>[...new Map([...(m.qualityData.value[key(props.node)]?.services||[]),...(w.services.value[key(props.node)]||[])].map(a=>[a.attempt_id,a])).values()])
const hasExitData=computed(()=>landing.value.exitKnown||landing.value.ipKind.measured||attempts.value.some(a=>serviceIsProfile(a)&&!!a.result))
const readError=computed(()=>m.qualityErrors.value[key(props.node)])
const downloadRecords=computed(()=>[...new Map([...(w.downloads.value[key(props.node)]||[]),...(m.qualityData.value[key(props.node)]?.downloads||[])].map(a=>[a.attempt_id,a])).values()])
const downloadHistoryContext=computed(()=>{const {latest,valid}=downloadEvidence(downloadRecords.value),speed=downloadSpeed(valid);return valid&&speed!==null?`${valid.attempt_id===latest?.attempt_id?'最近有效速度':'历史有效速度'} ${speed.toFixed(2)} MiB/s（${date(attemptObservedAt(valid))}）`:''})
const downloadLatest=computed(()=>downloadEvidence(downloadRecords.value).latest)
const downloadValue=computed(()=>downloadSpeed(downloadLatest.value))
const downloadSource=computed(()=>{const url=downloadTarget(downloadLatest.value);return url.includes('cachefly')?'Cachefly':url.includes('cloudflare')?'Cloudflare':url?new URL(url).hostname:'测速源未记录'})
const downloadFailure=computed(()=>{
 const a=downloadLatest.value,r=a?.result
 if(readError.value)return '已保存记录读取失败'
 if(!a)return '尚无测速记录'
 if(r?.error_message==='测速源都限流')return '测速源都限流'
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
 const e=m.qualityData.value[key(props.node)],points=[...(e?.tests||[]).map(t=>manualLatencyPoint(t,id)),...monitorLatencyPoints(e?.monitor||[],id)],stable=inspectorMedian(points.flatMap(p=>{const v=stableLatency([{id,points:[p]}]);return v===null?[]:[v]}));return `${range}内 ${s.success}/${s.total} 次响应`+(stable!==null?` · 稳定中位 ${Math.round(stable)} ms`:'')
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
    <div class="hd-ins-clash-row">
      <span v-if="isClashUsing" class="hd-ins-clash-active"><i aria-hidden="true"></i>正在使用</span>
      <button class="hd-btn" :disabled="clashStatusLoading||!clashStatus?.loaded" :title="clashStatusLoading?'正在读取 Clash 状态…':clashStatus?.loaded?'选择 Clash 代理组路径':'Clash 当前没有加载这个节点'" @click="openClashDialog">{{clashStatusLoading?'读取 Clash…':'在 Clash 中使用'}}</button>
    </div>
    <div v-if="!exitOnly" class="hd-ins-verdict" :class="verdict.tone"><strong>{{verdict.label}}</strong><span>{{verdict.detail}}</span></div>

    <div v-if="exitOnly||project==='combined'" class="hd-ins-block hd-exit">
      <p v-if="!hasExitData" class="exit-unqueried">出口与信誉：未查询 <button class="hd-btn" @click="w.openReputationPlan([node])">查询</button></p>
      <template v-else>
      <p class="exit-purity" :title="landing.purity.title"><span>{{landing.purity.summary}}</span><button class="hd-btn" @click="w.openReputationPlan([node])">查询</button></p>
      <ol v-if="exitOnly||nodeEntry" class="exit-path" aria-label="线路路径">
        <li v-for="(step,index) in exitPath" :key="step.id"><small>{{step.label}}</small><span :title="step.id==='exit'&&!landing.exitKnown?'出口还没检测，这里按节点名称推断':''">{{step.value}}</span><i v-if="index<exitPath.length-1" aria-hidden="true">→</i></li>
      </ol>
      <p v-if="pathNotice" class="exit-path-notice">{{pathNotice}}</p>
      <p class="landing-result" :class="landing.tone">{{landing.label}}</p><div class="landing-sources"><span v-for="tag in landing.tags" :key="tag">{{tag}}</span></div><NodeExitSummary compact :attempts="attempts"/>
      </template>
    </div>

    <div v-if="shows('latency')" class="hd-ins-block hd-latency">
      <InspectorLatency :series="latencySeries" :node="node" :focus="reportFocus" @round-change="latencyFocus=$event" @return-latest="w.latencyReportFocus.value=null"/>
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
      <h3>解锁检测</h3><UnlockInspector :node="node" :ip-type="landing.ipKind.type"/>
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
    <nav v-if="others.length" class="hd-ins-others" aria-label="其他项目摘要"><span>其他项目</span><button v-for="o in others" :key="o.id" type="button" :title="'切到'+o.name+'：'+o.v.detail" @click="emit('select-project',o.id)"><small>{{o.name}}</small><strong :class="o.v.tone">{{o.v.label}}</strong></button></nav>
    <p class="hd-legend"><span class="good">{{toneLabel('good')}}</span><span class="warn">{{toneLabel('warn')}}</span><span class="bad">{{toneLabel('bad')}}</span><span class="empty">{{toneLabel('empty')}}</span></p>
    <Modal v-if="clashDialog" title="在 Clash 中使用这个节点" wide @close="clashDialog=false">
      <p class="clash-modal-note">切换只影响新连接；现有连接会继续使用原线路。</p>
      <p v-if="clashGroupsLoading" class="hd-muted">正在读取可选路径…</p>
      <p v-else-if="clashError" class="inline-error" role="alert">{{clashError}}</p>
      <p v-else-if="!clashGroups.length" class="hd-muted">没有找到包含这个节点的可选代理组路径。</p>
      <div v-else class="clash-options" role="radiogroup" aria-label="选择 Clash 代理组路径">
        <label v-for="option in clashGroups" :key="option.path.join(' → ')+'|'+option.group" class="clash-option">
          <input v-model="selectedClashGroup" type="radio" name="clash-group" :value="option.group" :disabled="clashSelectBusy||!!clashSelection?.verified">
          <span>{{option.path.join(' → ')}}</span>
        </label>
      </div>
      <div v-if="selectedClashOption&&!clashSelection?.verified" class="clash-steps" aria-label="选择将影响的代理组">
        <strong>将改变的选择</strong>
        <div v-for="step in selectedClashOption.steps" :key="step.group" class="clash-step"><span>{{step.group}}</span><b>{{step.before||'（未选择）'}} → {{step.after}}</b></div>
      </div>
      <p v-if="clashSelection?.verified" class="clash-select-result good" role="status">已在 Clash 中确认切换。</p>
      <p v-else-if="clashSelection" class="clash-select-result warn" role="status">Clash 尚未确认所有代理组选择，请在 Clash 中查看。</p>
      <div v-if="clashSelection?.verified" class="clash-steps" aria-label="Clash 已确认的选择">
        <strong>Clash 已确认</strong>
        <div v-for="step in clashSelection.steps" :key="step.group" class="clash-step"><span>{{step.group}}</span><b>{{step.before||'（未选择）'}} → {{step.after}}</b></div>
      </div>
      <template #footer>
        <button v-if="clashSelection?.verified" class="button primary" @click="clashDialog=false">关闭</button>
        <button v-if="!clashSelection?.verified" class="button" :disabled="clashSelectBusy" @click="clashDialog=false">取消</button>
        <button v-if="!clashSelection?.verified" class="button primary" :disabled="!selectedClashOption||clashGroupsLoading||clashSelectBusy" @click="confirmClashSelection">{{clashSelectBusy?'切换中…':'确认使用'}}</button>
      </template>
    </Modal>
  </section>
</template>

<style scoped>
.exit-purity{display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:12px}.exit-purity span{min-width:0;overflow-wrap:anywhere}
.exit-unqueried{display:flex;align-items:center;justify-content:space-between;gap:8px;color:var(--hd-muted);font-size:12px}
.hd-inspector[data-project="exit"] .hd-ins-verdict{display:block}.hd-inspector[data-project="exit"] .hd-ins-verdict strong{font-size:14px}.landing-sources{display:flex;flex-wrap:wrap;align-items:center;gap:4px;margin-top:6px}.landing-sources span{font-size:10px;line-height:1.5;padding:2px 5px;border:1px solid var(--hd-line);border-radius:4px;color:var(--hd-muted);background:var(--hd-panel);white-space:nowrap}
.exit-path{display:flex;flex-wrap:wrap;align-items:center;gap:4px;margin:2px 0 8px;padding:0;list-style:none}
.exit-path li{display:inline-flex;align-items:center;gap:4px;min-width:0;font-size:11px;line-height:1.5}
.exit-path small{color:var(--hd-muted);font-size:10px}
.exit-path span{color:var(--hd-text,inherit);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:128px}
.exit-path i{font-style:normal;color:var(--hd-muted);margin:0 2px}
.exit-path-notice{margin:0 0 8px;padding:6px 8px;border-radius:6px;background:#fff6e5;border:1px solid #e3c184;color:#805211;font-size:10px;line-height:1.55}
.exit-ipkind{margin:0 0 8px;font-size:11px;line-height:1.6;color:var(--hd-text)}.exit-ipkind.empty{color:var(--hd-muted)}
.hd-ins-head{order:-3}.hd-ins-verdict{order:-2}.hd-inspector[data-project="latency"] .hd-latency,.hd-inspector[data-project="download"] .hd-download,.hd-inspector[data-project="service"] .hd-services-block{order:-1}
.hd-ins-clash-row{display:flex;align-items:center;justify-content:flex-end;gap:7px;margin:-4px 0 8px}.hd-ins-clash-active{display:inline-flex;align-items:center;gap:5px;color:var(--hd-good);font-size:11px}.hd-ins-clash-active i{width:6px;height:6px;border-radius:50%;background:var(--hd-good)}.clash-modal-note{margin:0 0 12px;color:var(--hd-muted);font-size:12px}.clash-options{display:grid;gap:6px;max-height:36vh;overflow:auto}.clash-option{display:flex;align-items:flex-start;gap:8px;padding:8px 10px;border:1px solid var(--hd-line);border-radius:7px;cursor:pointer}.clash-option:has(input:checked){border-color:var(--hd-accent);background:var(--hd-panel)}.clash-option input{margin-top:3px;flex:none}.clash-option span{min-width:0;overflow-wrap:anywhere;font-size:12px}.clash-steps{display:grid;gap:6px;margin-top:14px;padding-top:12px;border-top:1px solid var(--hd-line);font-size:12px}.clash-step{display:flex;justify-content:space-between;gap:12px}.clash-step span{color:var(--hd-muted)}.clash-step b{text-align:right;font-weight:500;overflow-wrap:anywhere}.clash-select-result{margin:12px 0 0;font-size:12px}.clash-select-result.good{color:var(--hd-good)}.clash-select-result.warn{color:var(--hd-warn)}

.hd-inspector[data-project="service"] .hd-ins-verdict{display:block}.hd-inspector[data-project="service"] .hd-ins-verdict strong{display:block;font-size:13px;line-height:1.5}.hd-inspector[data-project="service"] .hd-ins-verdict span{display:block;margin-top:4px;font-size:11px}.download-success strong{font-size:25px;font-weight:600;font-variant-numeric:tabular-nums}.download-success small{font-size:12px;font-weight:400;color:var(--hd-muted)}.download-failure{display:flex;align-items:center;flex-wrap:wrap;gap:8px;padding:10px 12px;background:#fff0d3;color:#805211;border-radius:8px;font-size:12px;line-height:1.55}.download-failure span{white-space:nowrap;font-size:11px}.download-failure .hd-btn{margin-left:auto;flex-shrink:0;background:transparent;border-color:#cfad72;color:inherit}.download-failure.empty{padding:0;background:none;color:var(--hd-muted)}.hd-saved-download{display:block;margin-top:7px;color:var(--hd-muted);font-size:10px}.download-spark{display:block;width:100%;height:34px;margin-top:8px}
</style>
