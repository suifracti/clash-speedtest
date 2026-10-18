<script setup lang="ts">
import {computed} from 'vue'
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
const value=(s?:SiteQualitySummary)=>props.error||s?.readError?'读取失败':s?.p50!=null?Math.round(s.p50)+' ms':s?.total?'未测通':s?.state==='cancelled'?'已取消':s?.state==='not_executed'?'未执行':!props.evidence?'读取中…':'未测'
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
const tone=(s?:SiteQualitySummary)=>s?.success?s.success===s.total?'good':'warn':s?.total?'bad':'muted'
const observedAt=computed(()=>sites.value.map(s=>props.viewMode==='window'?s.summary?.lastAt:s.rounds[0]?.test.finished_at).filter((s):s is string=>!!s).sort().at(-1))
const downloadRead=computed(()=>download.value.latest?.result?.bytes_read)
const downloadPhase=computed(()=>download.value.latest?.result?.failure_phase||download.value.latest?.result?.phase||'')
const zeroBytesTLS=computed(()=>downloadRead.value===0&&downloadPhase.value==='tls')
</script>
<template>
 <section class="overview-latency" aria-label="六站延迟与历史比较">
  <header class="overview-section-head"><strong>{{sites.length===6?'六站响应':'所选站点响应'}}</strong><span>{{caption}}</span></header>
  <div class="overview-latency-title"><span>{{!measuredSites?'尚无对应测量':successfulSites===0?(viewMode==='window'?'窗口内未取得有效延迟':'本次未取得有效延迟'):successfulSites+' / '+sites.length+' 站测得延迟'}}</span><time v-if="observedAt">{{date(observedAt)}}</time></div>
  <div class="overview-sites" aria-label="各站延迟">
   <button v-for="{site,summary:s} in sites" :key="site.id" type="button" :class="tone(s)" :title="site.name+' · '+caption+' · '+(s?.total?'共测'+s.total+'次，'+s.success+'次收到HTTP响应':'没有对应记录')" :aria-label="'查看 '+site.name+' 原始历史'" @click="$emit('history',site.id)">
    <span class="overview-site-name">{{site.name}}<span aria-hidden="true">↗</span></span>
    <strong>{{value(s)}}</strong>
    <small v-if="s?.total&&!error&&!s.readError">{{s.success}} / {{s.total}} 次响应</small><small v-else>{{s?.state==='cancelled'?'不计作节点故障':'不以旧成功填补'}}</small>
   </button>
  </div>
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
.overview-latency,.overview-download,.overview-services{min-width:0;align-self:start;font-size:13px;line-height:1.6;color:var(--ink)}.overview-section-head{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin:0 0 10px}.overview-section-head>strong{font-size:14px;font-weight:650;color:var(--ink)}.overview-section-head>span{font-size:11px;color:var(--muted);font-weight:400}.overview-latency-title{display:flex;flex-wrap:wrap;align-items:baseline;justify-content:space-between;gap:3px 8px;font-size:11px;color:var(--muted);margin-bottom:8px}.overview-latency-title time{font-variant-numeric:tabular-nums;white-space:nowrap}.overview-sites{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:7px}.overview-sites>button{min-width:0;display:flex;flex-direction:column;align-items:stretch;gap:4px;text-align:left;padding:9px 10px;border:1px solid var(--line,#dce3ef);border-radius:8px;background:var(--surface-soft,#f5f7fb);cursor:pointer;color:var(--ink)}.overview-site-name{display:flex;align-items:center;justify-content:space-between;gap:4px;font-size:12px;font-weight:550;color:var(--muted);white-space:nowrap}.overview-site-name>span{font-size:10px;opacity:.6}.overview-sites>button>strong{font:600 17px/1.45 ui-sans-serif,system-ui,sans-serif;letter-spacing:-.4px;font-variant-numeric:tabular-nums;white-space:nowrap;color:var(--ink)}.overview-sites>button.bad>strong{color:var(--bad,#b5475e)}.overview-sites>button>small{font-size:10px;color:var(--muted);line-height:1.5;letter-spacing:0}.overview-sites>button:hover,.overview-sites>button:focus-visible{border-color:var(--blue);outline-offset:3px}.overview-history-link{display:flex;align-items:center;gap:9px;padding:6px 0 0;margin-top:6px;background:none;border:0;font-size:12px;line-height:1.8;color:var(--blue);cursor:pointer;text-align:left}.overview-download-status{display:block;font-size:15px;line-height:1.6;overflow-wrap:anywhere;font-weight:600}.overview-download-explanation{font-size:12px;color:var(--muted);margin:8px 0 12px;line-height:1.7}.overview-main>strong{font-size:25px;font-weight:650;letter-spacing:-.7px;font-variant-numeric:tabular-nums}.overview-main small{font-size:12px;font-weight:400;letter-spacing:0;color:var(--muted)}.overview-download-read{display:flex;flex-wrap:wrap;gap:8px;border-top:1px solid var(--line);padding-top:9px;margin-top:10px;font-size:12px}.overview-download-read>span{color:var(--muted)}.overview-download-read>strong{font-variant-numeric:tabular-nums;font-weight:550}.overview-download>small,.overview-download>time{display:block;color:var(--muted);font-size:11px;margin-top:8px}.overview-services{display:flex;flex-direction:column;gap:0;align-content:start}.overview-service-result{display:flex;flex-direction:column;align-items:stretch;gap:4px;padding:8px 0;border:0;border-bottom:1px solid var(--line);border-radius:0;background:none;text-align:left;min-width:0;cursor:pointer}.overview-service-name{font-size:12px;line-height:1.55;color:var(--ink);font-weight:550;overflow-wrap:anywhere;white-space:normal}.overview-service-result>strong{display:flex;align-items:center;gap:6px;font-size:12px;font-weight:500;color:inherit}.overview-service-result>strong>span{margin-left:auto}.overview-service-result i{width:5px;height:5px;border-radius:50%;background:currentColor;flex:none}.overview-service-result:hover>.overview-service-name{text-decoration:underline}.overview-service-help{display:block;font-size:10px;color:var(--muted);line-height:1.7;margin-top:10px}.warn{color:var(--warning,#946315)}
@media(max-width:620px){.overview-sites{grid-template-columns:repeat(2,minmax(0,1fr))}.overview-sites>button>strong{font-size:18px}.overview-services{gap:2px}.overview-service-result{flex-direction:row;align-items:center;justify-content:space-between;gap:12px}.overview-service-name{flex:1}.overview-service-result>strong{flex:none}}
</style>
<style>
/* Only the combined node using this component is rearranged. Other pages,
   selection, sorting, history state and measurement callbacks stay unchanged. */
.observatory-app .home-view .node-list:has(> .node-row > .overview-latency) > .quality-heading.project-combined{display:none!important}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency){display:grid!important;grid-template-columns:24px minmax(210px,1fr) minmax(340px,1.7fr) minmax(145px,.72fr) minmax(170px,.85fr) 94px!important;gap:20px!important;align-items:start!important;padding:20px 16px!important}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-check{grid-column:1;align-self:start;margin-top:7px}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-identity-block{grid-column:2!important;grid-row:1!important;min-width:0;align-self:start}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-latency{grid-column:3!important;grid-row:1!important}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-download{grid-column:4!important;grid-row:1!important}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-services{grid-column:5!important;grid-row:1!important}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.row-run, .observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-action,.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-actions{grid-column:6!important;grid-row:1!important;align-self:start}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency) .node-name-button{align-items:flex-start!important;white-space:normal!important;min-width:0}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency) .node-name strong{font-size:15px!important;line-height:1.65!important;overflow-wrap:anywhere;white-space:normal!important}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency) .node-name small{font-size:11px!important;line-height:1.6!important}
@media(max-width:1250px){.observatory-app .home-view .node-list .node-row:has(> .overview-latency){grid-template-columns:24px minmax(190px,.9fr) minmax(0,1.7fr) 94px!important;gap:18px!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-identity-block{grid-column:2!important;grid-row:1 / span 3!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-latency{grid-column:3 / 5!important;grid-row:1!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-download{grid-column:3!important;grid-row:2!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-services{grid-column:3 / 5!important;grid-row:3!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.row-run, .observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-action,.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-actions{grid-column:4!important;grid-row:2!important}}
@media(max-width:620px){.observatory-app .home-view .node-list .node-row:has(> .overview-latency){grid-template-columns:20px minmax(0,1fr)!important;gap:18px 10px!important;padding:16px 12px!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-identity-block{grid-column:2!important;grid-row:1!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-latency{grid-column:2!important;grid-row:2!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-download{grid-column:2!important;grid-row:3!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-services{grid-column:2!important;grid-row:4!important}.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.row-run, .observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-action,.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-actions{grid-column:2!important;grid-row:5!important}}

.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-services{flex-direction:column!important;flex-wrap:nowrap!important;gap:0!important;align-content:start!important}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-health,
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.node-inline-results{grid-column:2 / -1!important;grid-row:auto!important;min-width:0}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.row-run{min-width:94px!important;min-height:36px!important;font-size:12px!important}
@media(min-width:621px) and (max-width:1250px){
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-services{display:grid!important;grid-template-columns:repeat(3,minmax(0,1fr));gap:0 12px!important}
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-services>.overview-section-head,
.observatory-app .home-view .node-list .node-row:has(> .overview-latency)>.overview-services>small{grid-column:1 / -1}
}
</style>
