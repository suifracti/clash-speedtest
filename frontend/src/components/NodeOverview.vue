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
const primary=computed(()=>shown(props.summaries?.find(s=>s.targetId===props.target)))
const sites=computed(()=>targets.filter(t=>props.targetIds.includes(t.id)).map(site=>({site,summary:shown(props.summaries?.find(s=>s.targetId===site.id)),rounds:siteRoundHistory(props.summaries?.find(s=>s.targetId===site.id),props.evidence?.tests)})))
const successfulSites=computed(()=>sites.value.filter(s=>(s.summary?.success||0)>0).length)
const measuredSites=computed(()=>sites.value.filter(s=>(s.summary?.total||0)>0).length)
const roundCount=computed(()=>Math.max(0,...sites.value.map(s=>s.rounds.length)))
const value=(s?:SiteQualitySummary)=>props.error||s?.readError?'读取失败':s?.p50!=null?Math.round(s.p50)+' ms':s?.total?'未测通':s?.state==='cancelled'?'已取消':s?.state==='not_executed'?'未执行':'未测'
const download=computed(()=>downloadEvidence(props.evidence?.downloads||[])),speed=computed(()=>downloadSpeed(download.value.valid))
const services=computed(()=>props.serviceIds.map(id=>({id,rule:props.catalog.find(r=>r.service_id===id),attempt:latestServices(props.evidence?.services||[]).find(a=>(a.service_id||a.rule.service_id)===id)})))
const name=(rule?:ServiceRule)=>rule?.name.replace(/\s*(网页与挑战|片目与地区检查|地区判定|连通性|网页入口|入口 GET).*$/,'')||'服务'
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
</script>
<template>
 <section class="overview-latency overview-readable" aria-label="六站延迟与历史比较">
  <div class="overview-latency-title"><strong>{{sites.length===6?'六站延迟':'所选站点延迟'}}</strong><span>{{caption}}<template v-if="measuredSites"> · {{successfulSites}} / {{sites.length}} 站测得延迟</template></span></div>
  <div class="overview-sites" aria-label="各站延迟">
   <button v-for="{site,summary:s} in sites" :key="site.id" type="button" :class="tone(s)" :title="site.name+' · '+caption+' · '+(s?.total?'共测'+s.total+'次，'+s.success+'次收到HTTP响应':'没有对应记录')" :aria-label="'查看 '+site.name+' 原始历史'" @click="$emit('history',site.id)">
    <span>{{site.name}}</span><strong>{{value(s)}}</strong><small v-if="s?.total&&!error&&!s.readError">成功 {{s.success}} / {{s.total}}</small>
   </button>
  </div>
  <button type="button" class="overview-history-link" @click="$emit('history',target)">历史曲线与逐轮对比{{roundCount?' · '+roundCount+'轮':''}} <span aria-hidden="true">↗</span></button>
  <small v-if="primary&&!primary.readComplete" class="warn">读取不完整</small>
 </section>
 <section class="overview-download quality-download overview-readable" aria-label="最近下载">
  <template v-if="download.latest">
   <div class="overview-main" v-if="download.valid?.attempt_id===download.latest.attempt_id"><strong>{{speed?.toFixed(2)}} <small>MiB/s</small></strong></div>
   <strong v-else class="overview-download-status" :class="attemptTone(download.latest)">{{attemptStatus(download.latest,true)}}</strong>
   <small v-if="download.latest.result" class="overview-download-bytes"><template v-if="Number.isFinite(download.latest.result.bytes_read)">已读取 {{bytes(download.latest.result.bytes_read)}}<template v-if="download.latest.result.bytes_read===0"> · 未取得下载正文</template></template><template v-else>读取字节未记录</template></small>
   <small>{{date(attemptObservedAt(download.latest))}}</small>
   <small v-if="download.valid?.attempt_id===download.latest.attempt_id">{{downloadGroup(download.latest).kind==='partial'?'有效部分测量':'完整读取'}} · 短测平均</small>
   <small v-if="download.valid&&download.valid.attempt_id!==download.latest.attempt_id" class="overview-old-download">历史有效 {{speed?.toFixed(2)}} MiB/s · {{date(attemptObservedAt(download.valid))}}</small>
  </template><span v-else class="muted">{{!evidence?'读取中…':'未测'}}</span>
  <small v-if="evidence&&!evidence.reads.download.complete" class="warn">下载读取不完整</small>
 </section>
 <section class="overview-services quality-services overview-readable" aria-label="所选服务结论">
  <button v-for="s in services" :key="s.id" type="button" :class="s.attempt?s.attempt.result?.outcome==='reachable'?'reachable':servicePassed(s.attempt)?'good':attemptTone(s.attempt):'muted'" :title="s.rule?.name+' · '+(s.attempt?.result?.error_message||s.attempt?.result?.summary||'未测')" @click="$emit('service',s.id)"><span class="overview-service-name">{{name(s.rule)}}</span><span class="overview-service-conclusion"><i aria-hidden="true"/>{{serviceLabel(s.attempt)}}<span class="overview-service-arrow" aria-hidden="true"> ›</span></span></button>
  <span v-if="!services.length" class="muted">未选择服务</span><small v-if="evidence&&!evidence.reads.service.complete" class="warn">服务读取不完整</small>
 </section>
</template>
<style scoped>
.overview-readable{min-width:0;align-self:start;font-size:12px;line-height:1.5;color:var(--ink);font-variant-numeric:tabular-nums}.overview-readable.overview-latency .overview-latency-title{display:flex;align-items:baseline;flex-wrap:wrap;gap:3px 8px;margin-bottom:9px}.overview-latency-title>strong{font-size:13px;font-weight:650}.overview-latency-title>span{font-size:11px;color:var(--muted)}.overview-readable .overview-sites{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:0;border-top:1px solid var(--line,#dce2e9);border-bottom:1px solid var(--line,#dce2e9)}.overview-readable .overview-sites>button{display:flex;flex-direction:column;align-items:flex-start;justify-content:center;gap:1px;min-height:66px;padding:7px 8px;border:0;background:none;text-align:left;min-width:0;font-size:11px;cursor:pointer;color:var(--muted)}.overview-sites>button:not(:nth-child(3n+1)){border-left:1px solid var(--line,#dce2e9)}.overview-sites>button:nth-child(n+4){border-top:1px solid var(--line,#dce2e9)}.overview-readable .overview-sites>button>strong{font-size:15px;font-weight:650;color:var(--ink);white-space:nowrap;letter-spacing:-.01em}.overview-readable .overview-sites>button.bad>strong{color:var(--danger,#ba4450)}.overview-readable .overview-sites>button>span{font-size:12px;white-space:nowrap}.overview-readable .overview-sites>button>small{font-size:10px;line-height:1.4}.overview-sites>button:hover{background:var(--surface-soft,#f3f6fa)}.overview-sites>button:focus-visible{outline:2px solid var(--blue);outline-offset:-2px}.overview-history-link{display:block;margin-top:9px;padding:1px 0;border:0;background:none;font-size:11px;color:var(--blue);text-align:left;cursor:pointer;min-height:26px}.overview-history-link:hover{text-decoration:underline}.overview-main>strong{font-size:20px;font-weight:650;white-space:nowrap}.overview-main small{font-size:12px;font-weight:400;color:var(--muted)}.overview-readable.overview-download>small{display:block;margin-top:5px;color:var(--muted);font-size:11px}.overview-download-status{display:block;font-size:14px;font-weight:600;line-height:1.6;overflow-wrap:anywhere}.overview-old-download{padding-top:6px;border-top:1px dashed var(--line,#dce2e9)}.overview-readable.overview-services{display:grid;grid-template-columns:minmax(0,1fr);gap:0;align-content:start}.overview-readable.overview-services>button{display:flex;flex-direction:column;align-items:flex-start;gap:3px;padding:7px 0;min-height:48px;border:0;border-bottom:1px solid var(--line,#dce2e9);background:none;text-align:left;cursor:pointer;font-size:12px;line-height:1.45;min-width:0}.overview-readable .overview-service-name{display:block;max-width:none;overflow:visible;text-overflow:clip;white-space:normal;overflow-wrap:anywhere;color:var(--ink);font-size:12px;font-weight:550}.overview-service-conclusion{display:flex;align-items:center;gap:5px;font-size:11px}.overview-service-conclusion i{width:4px;height:4px;flex:none;background:currentColor;border-radius:50%}.overview-service-arrow{margin-left:3px;color:var(--muted)}.overview-services>button:hover .overview-service-name{text-decoration:underline}.overview-services>button:focus-visible{outline:2px solid var(--blue);outline-offset:2px}.overview-services>.muted{font-size:12px}.overview-services>small{font-size:11px}
@media(max-width:740px){.overview-readable .overview-sites{grid-template-columns:repeat(2,minmax(0,1fr))}.overview-sites>button:not(:nth-child(3n+1)){border-left:0}.overview-sites>button:nth-child(even){border-left:1px solid var(--line,#dce2e9)}.overview-sites>button:nth-child(n+3){border-top:1px solid var(--line,#dce2e9)}}
</style>
