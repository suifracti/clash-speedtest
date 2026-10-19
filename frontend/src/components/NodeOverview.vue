<script setup lang="ts">
import {computed} from 'vue'
import SiteMetrics from './SiteMetrics.vue'
import {siteRoundHistory} from '../siteRoundPresentation'
import {downloadEvidence,latestServices,attemptObservedAt,type QualityEvidence,type SiteQualitySummary} from '../nodeQuality'
import {targets,date,bytes,downloadSpeed,outcomeLabel,type Attempt,type ServiceRule} from '../domain'
import {attemptStatus,attemptTone} from '../measurementRounds'
import {servicePassed,serviceAvailability,downloadGroup} from '../sharedResults'
const props=defineProps<{evidence?:QualityEvidence;summaries?:SiteQualitySummary[];error?:string;target:string;targetIds:string[];viewMode?:'latest'|'window';serviceIds:string[];catalog:ServiceRule[]}>()
defineEmits<{service:[id:string];history:[id:string]}>()
const caption=computed(()=>props.viewMode==='window'?'时间范围汇总':'最近一次')
const shown=(s:SiteQualitySummary|undefined)=>props.viewMode==='window'?s:siteRoundHistory(s,props.evidence?.tests)[0]?.summary||s
const sites=computed(()=>targets.filter(t=>props.targetIds.includes(t.id)).map(site=>({site,summary:shown(props.summaries?.find(s=>s.targetId===site.id)),rounds:siteRoundHistory(props.summaries?.find(s=>s.targetId===site.id),props.evidence?.tests)})))
const successfulSites=computed(()=>sites.value.filter(s=>(s.summary?.success||0)>0).length)
const measuredSites=computed(()=>sites.value.filter(s=>(s.summary?.total||0)>0).length)
const roundCount=computed(()=>Math.max(0,...sites.value.map(s=>s.rounds.length)))
const download=computed(()=>downloadEvidence(props.evidence?.downloads||[])),speed=computed(()=>downloadSpeed(download.value.valid))
const services=computed(()=>props.serviceIds.map(id=>({id,rule:props.catalog.find(r=>r.service_id===id),attempt:latestServices(props.evidence?.services||[]).find(a=>(a.service_id||a.rule.service_id)===id)})))
const name=(rule?:ServiceRule)=>rule?.name.replace(/\s*(网页与(?:\s*Cloudflare\s*)?挑战|片目与地区检查|地区判定|连通性|网页入口|入口 GET).*$/,'')||'服务'
const serviceLabel=(a?:Attempt)=>{
 if(!a)return '未测'
 if(a.persistence_state==='failed')return '保存失败'
 if(a.execution_state==='not_executed'||a.result?.outcome==='not_executed')return '未执行'
 if(!a.result)return attemptStatus(a)
 if(servicePassed(a))return '规则通过'
 if(a.result.outcome==='reachable')return '网页可达'
 if(a.result.outcome==='profiled')return '已获取画像'
 if(['matched','unlocked','flapping'].includes(a.result.outcome)&&serviceAvailability(a)===null)return '未确认'
 return outcomeLabel(a.result.outcome)
}
const observedAt=computed(()=>sites.value.map(s=>props.viewMode==='window'?s.summary?.lastAt:s.rounds[0]?.test.finished_at).filter((s):s is string=>!!s).sort().at(-1))
const downloadRead=computed(()=>download.value.latest?.result?.bytes_read)
const downloadPhase=computed(()=>download.value.latest?.result?.failure_phase||download.value.latest?.result?.phase||'')
const zeroBytesTLS=computed(()=>downloadRead.value===0&&downloadPhase.value==='tls')
</script>
<template>
 <section class="overview-latency" aria-label="六站延迟与历史比较">
  <header class="overview-section-head"><strong>{{sites.length===6?'六站响应':'所选站点响应'}}</strong><span>{{caption}}</span></header>
  <div class="overview-latency-title"><span>{{!measuredSites?'尚无对应测量':successfulSites===0?(viewMode==='window'?'窗口内未取得有效延迟':'本次未取得有效延迟'):successfulSites+' / '+sites.length+' 站测得延迟'}}</span><time v-if="observedAt">{{date(observedAt)}}</time></div>
  <SiteMetrics compact :summaries="summaries" :target-ids="targetIds" :tests="evidence?.tests" :view-mode="viewMode" :error="error" @history="$emit('history',$event)"/>
  <button type="button" class="overview-history-link" @click="$emit('history',target)">查看历史曲线与逐轮对比{{roundCount?' · '+roundCount+'轮':''}} <span aria-hidden="true">→</span></button>
  <small v-if="sites.some(s=>s.summary&&!s.summary.readComplete)" class="warn">部分站点的历史读取不完整</small>
 </section>
 <section class="overview-download quality-download" aria-label="最近下载">
  <header class="overview-section-head"><strong>下载测速</strong><span>最近一次</span></header>
  <template v-if="download.latest">
   <div class="overview-main" v-if="download.valid?.attempt_id===download.latest.attempt_id"><strong>{{speed?.toFixed(2)}} <small>MiB/s</small></strong></div>
   <strong v-else class="overview-download-status" :class="attemptTone(download.latest)">{{attemptStatus(download.latest,true)}}</strong>
   <p v-if="zeroBytesTLS" class="overview-download-explanation">未开始传输正文<br>不代表带宽为零</p>
   <div class="overview-download-read"><span>实际读取</span><strong>{{typeof downloadRead==='number'&&Number.isFinite(downloadRead)?bytes(downloadRead):'未记录'}}</strong></div>
   <small v-if="download.valid&&download.valid.attempt_id!==download.latest.attempt_id" class="muted">历史有效 {{speed?.toFixed(2)}} MiB/s · {{date(attemptObservedAt(download.valid))}}</small>
   <time>{{date(attemptObservedAt(download.latest))}}</time>
   <small v-if="download.valid?.attempt_id===download.latest.attempt_id">{{downloadGroup(download.latest).kind==='partial'?'有效部分测量':'完整读取'}}</small>
  </template><span v-else class="muted">{{!evidence?'读取中…':'未测'}}</span>
  <small v-if="evidence&&!evidence.reads.download.complete" class="warn">下载读取不完整</small>
 </section>
 <section class="overview-services quality-services" aria-label="所选服务结论">
  <header class="overview-section-head"><strong>所选服务</strong><span>{{services.length}} 项</span></header>
  <button v-for="s in services" :key="s.id" type="button" class="overview-service-result" :class="s.attempt?s.attempt.result?.outcome==='reachable'?'reachable':servicePassed(s.attempt)?'good':attemptTone(s.attempt):'muted'" :title="s.rule?.name+' · '+(s.attempt?.result?.error_message||s.attempt?.result?.summary||'未测')" @click="$emit('service',s.id)"><span class="overview-service-name">{{name(s.rule)}}</span><strong><i/>{{serviceLabel(s.attempt)}}<span aria-hidden="true">›</span></strong></button>
  <span v-if="!services.length" class="muted">未选择服务</span><small v-if="evidence&&!evidence.reads.service.complete" class="warn">服务读取不完整</small>
  <small v-if="services.length" class="overview-service-help">点选查看判据<br>网页可达不等于业务可用</small>
 </section>
</template>
<style scoped>
.overview-latency,.overview-download,.overview-services{min-width:0;align-self:start;font-size:12px;line-height:1.5;color:var(--ink)}.overview-section-head{display:flex;align-items:baseline;gap:8px;margin-bottom:5px}.overview-section-head>strong{font-size:13px;font-weight:550}.overview-section-head>span{font-size:12px;color:var(--muted)}.overview-latency-title{display:flex;align-items:baseline;flex-wrap:wrap;gap:3px 8px;font-size:12px;color:var(--muted);margin-bottom:5px}.overview-latency-title time{font-variant-numeric:tabular-nums}
.overview-history-link{display:block;margin-top:6px;padding:0;border:0;background:none;font-size:12px;color:var(--blue);text-align:left;cursor:pointer}
.overview-main>strong{font-size:16px;font-weight:600;font-variant-numeric:tabular-nums}.overview-main small{font-size:12px;font-weight:400;color:var(--muted)}.overview-download-status{font-size:13px;font-weight:550;overflow-wrap:anywhere}.overview-download-explanation{font-size:12px;color:var(--muted);margin:4px 0}.overview-download-read{display:flex;gap:7px;flex-wrap:wrap;margin-top:5px;font-size:12px}.overview-download-read>span{color:var(--muted)}.overview-download-read>strong{font-weight:500}.overview-download>small,.overview-download>time{display:block;color:var(--muted);font-size:12px;margin-top:4px}
.overview-services{display:flex;flex-direction:column;gap:3px}.overview-service-result{display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 7px;min-width:0;padding:2px 0;border:0;background:none;text-align:left;font-size:12px;color:inherit;cursor:pointer}.overview-service-name{font-size:12px;overflow-wrap:anywhere}.overview-service-result>strong{display:flex;align-items:center;gap:4px;font-size:12px;font-weight:500}.overview-service-result i{width:4px;height:4px;background:currentColor}.overview-service-result:hover>.overview-service-name{text-decoration:underline}.overview-service-help{font-size:12px;color:var(--muted);line-height:1.5;margin-top:3px}.warn{color:var(--warning,#946315)}
</style>
