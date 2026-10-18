<script setup lang="ts">
import {computed} from 'vue'
import {siteRoundHistory} from '../siteRoundPresentation'
import {downloadEvidence,latestServices,attemptObservedAt,type QualityEvidence,type SiteQualitySummary} from '../nodeQuality'
import {targets,date,downloadSpeed,outcomeLabel,type Attempt,type ServiceRule} from '../domain'
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
const ms=(n:number|null|undefined)=>n==null?'—':Math.round(n)
</script>
<template>
 <section class="overview-latency" aria-label="六站延迟与历史比较">
  <div class="overview-latency-title"><strong v-if="target!=='all'">{{targets.find(t=>t.id===target)?.name}} · {{value(primary)}}</strong><strong v-else>{{!measuredSites?'六站未测':successfulSites===0?'六站均未测通':successfulSites+' / '+sites.length+' 站测得延迟'}}</strong><span>{{caption}}</span></div>
  <div class="overview-sites" aria-label="各站延迟"><button v-for="{site,summary:s} in sites" :key="site.id" type="button" :class="tone(s)" :title="site.name+' · '+caption+' · '+(s?.total?'共测'+s.total+'次，'+s.success+'次收到HTTP响应':'没有对应记录')" :aria-label="'查看 '+site.name+' 原始历史'" @click="$emit('history',site.id)"><span>{{site.name}}</span><strong>{{value(s)}}</strong><small v-if="s?.total&&s.success>0&&s.success<s.total">{{s.success}}次成功 / {{s.total}}次</small></button></div>
  <button type="button" class="overview-history-link" @click="$emit('history',target)">查看历史曲线与逐轮对比{{roundCount?' · '+roundCount+'轮':''}} ↗</button>
  <small v-if="primary&&!primary.readComplete" class="warn">读取不完整</small>
 </section>
 <section class="overview-download quality-download" aria-label="最近下载">
  <template v-if="download.latest">
   <div class="overview-main" v-if="download.valid?.attempt_id===download.latest.attempt_id"><strong>{{speed?.toFixed(2)}} <small>MiB/s</small></strong></div>
   <strong v-else :class="attemptTone(download.latest)">{{attemptStatus(download.latest,true)}}</strong>
   <small v-if="download.valid&&download.valid.attempt_id!==download.latest.attempt_id" class="muted">历史有效 {{speed?.toFixed(2)}} MiB/s · {{date(attemptObservedAt(download.valid))}}</small>
   <small>{{download.latest.result?.bytes_read?((download.latest.result.bytes_read)/1048576).toFixed(1)+' MiB · ':''}}{{date(attemptObservedAt(download.latest))}}</small>
   <small v-if="download.valid?.attempt_id===download.latest.attempt_id">{{downloadGroup(download.latest).kind==='partial'?'有效部分测量':'完整读取'}}</small>
  </template><span v-else class="muted">{{!evidence?'读取中…':'未测'}}</span>
  <small v-if="evidence&&!evidence.reads.download.complete" class="warn">下载读取不完整</small>
 </section>
 <section class="overview-services quality-services" aria-label="所选服务结论"><button v-for="s in services" :key="s.id" type="button" :class="s.attempt?s.attempt.result?.outcome==='reachable'?'reachable':servicePassed(s.attempt)?'good':attemptTone(s.attempt):'muted'" :title="s.rule?.name+' · '+(s.attempt?.result?.error_message||s.attempt?.result?.summary||'未测')" @click="$emit('service',s.id)"><i/><span><span class="overview-service-name">{{name(s.rule)}}</span> {{serviceLabel(s.attempt)}}</span></button><span v-if="!services.length" class="muted">未选择服务</span><small v-if="evidence&&!evidence.reads.service.complete" class="warn">服务读取不完整</small></section>
</template>
<style scoped>
.overview-latency,.overview-download,.overview-services{min-width:0;align-self:center;font-size:12px;line-height:1.5}.overview-main{display:flex;align-items:baseline;flex-wrap:wrap;gap:7px}.overview-main>strong{font-size:17px;line-height:1.3;font-weight:650;white-space:nowrap}.overview-main small{font-size:12px;font-weight:400;color:var(--muted)}.overview-main>span{font-size:12px;color:var(--muted)}.overview-response,.overview-download>small{display:block;font-size:12px;color:var(--muted);margin-top:4px}.overview-six{display:flex;gap:5px;margin-top:6px}.overview-six button{display:grid;gap:3px;min-width:23px;padding:0;border:0;background:none;font:11px/1.4 ui-monospace,monospace}.overview-six i{display:block;width:100%;height:2px;background:currentColor;opacity:.7}.overview-six button.muted i{opacity:.25}.overview-services{display:flex;flex-wrap:wrap;gap:4px 9px;align-content:center}.overview-services button{display:flex;align-items:baseline;gap:4px;padding:0;border:0;background:none;text-align:left;font-size:12px;line-height:1.6}.overview-service-name{max-width:110px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;display:inline-block;vertical-align:bottom}.overview-services i{display:block;flex:none;width:4px;height:4px;background:currentColor;align-self:center}.overview-services button:hover span{text-decoration:underline}
.overview-latency-title{display:flex;align-items:baseline;flex-wrap:wrap;gap:7px;margin-bottom:6px}.overview-latency-title>strong{font-size:13px;font-weight:550}.overview-latency-title>span{font-size:12px;color:var(--muted)}.overview-sites{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:4px 14px}.overview-sites>button{display:flex;align-items:baseline;justify-content:space-between;flex-wrap:wrap;gap:3px 6px;padding:0;border:0;background:none;text-align:left;font-size:12px;color:var(--muted);min-width:0}.overview-sites>button>strong{font-size:13px;font-weight:550;color:var(--ink);white-space:nowrap}.overview-sites>button.bad>strong{color:#bd3b4a}.overview-sites>button>small{flex-basis:100%;font-size:12px}.overview-sites>button:hover{text-decoration:underline}.overview-history-link{display:block;margin-top:8px;padding:0;border:0;background:none;font-size:12px;color:var(--blue);text-align:left;cursor:pointer}
</style>
