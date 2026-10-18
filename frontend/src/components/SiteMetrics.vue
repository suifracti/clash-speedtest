<script setup lang="ts">
import {computed} from 'vue'
import {date,type LatencyTest} from '../domain'
import {qualityGroups,type SiteQualitySummary} from '../nodeQuality'
import {latencyTargets,latencyTargetId} from '../latencyTargets'
const props=defineProps<{summaries?:SiteQualitySummary[];targetIds:string[];tests?:LatencyTest[];error?:string}>()
defineEmits<{history:[target:string]}>()
const states:Record<string,string>={unmeasured:'未测',responded:'已响应',partial:'部分响应',failed:'无响应',cancelled:'已取消',not_executed:'未执行',source_limited:'源限流'}
const sites=computed(()=>props.targetIds.map(id=>({site:latencyTargets.find(t=>t.id===id)!,summary:props.summaries?.find(s=>s.targetId===id)})).filter(s=>s.site))
const ms=(value:number|null|undefined)=>value==null?'—':Math.round(value)+' ms'
const tone=(s?:SiteQualitySummary)=>!s?.total?'muted':s.successRate===100?'good':s.success?'warn':'bad'
const state=(s?:SiteQualitySummary)=>props.error||s?.readError?'读取失败':!s?'读取中…':!s.readComplete&&!s.total?'读取不完整':states[s.state]
const hasP95=computed(()=>sites.value.some(({summary:s})=>s&&s.success>=20))
const commonTime=computed(()=>{const times=sites.value.map(({summary:s})=>s?.lastAt).filter(Boolean);return times.length&&new Set(times.map(t=>date(t))).size===1?times[0]:''})
const notes=computed(()=>Object.fromEntries(sites.value.map(({site,summary:s})=>{
 const notes=new Set<string>()
 if(s?.observations.sourceLimited)notes.add('429 限流')
 if(s?.group)for(const test of props.tests||[]){
  if(!qualityGroups([test],[],site.id).some(g=>g.key===s.group!.key))continue
  for(const sample of test.samples||[]){
   const at=Date.parse(sample.timestamp)
   if(!Number.isFinite(at)||latencyTargetId(sample.target||test.target||'')!==site.id||at<Date.parse(s.since)||at>Date.parse(s.until))continue
   const status=sample.error?.match(/\bHTTP\s*(\d{3})\b/i)?.[1]
   if(status&&Number(status)>=400&&status!=='429')notes.add(status+' 拒绝')
  }
 }
 if(s&&!s.readComplete)notes.add('读取不完整')
 else if(s?.reasons.includes('ambiguous_latest_group'))notes.add('条件不唯一')
 else if(s?.group&&!s.group.comparable)notes.add('条件未确认')
 if(s?.success&&s.success<5)notes.add('样本少')
 return [site.id,[...notes].join(' · ')]
})))
</script>

<template>
 <section class="site-metrics" aria-label="逐站响应率、样本和延迟分布">
  <div class="site-metrics-scroll" tabindex="0" aria-label="逐站指标，可横向滚动">
   <table class="site-metrics-table">
    <thead><tr><th scope="col">站点</th><th v-for="{site} in sites" :key="site.id" scope="col"><button type="button" :aria-label="'查看 '+site.name+' 原始历史'" @click="$emit('history',site.id)">{{site.name}} <span aria-hidden="true">↗</span></button></th></tr></thead>
    <tbody>
     <tr><th scope="row">中位 P50</th><td v-for="{site,summary:s} in sites" :key="site.id" :data-site="site.id"><strong v-if="s?.total&&!error&&!s.readError">{{ms(s.p50)}}</strong><span v-else :class="error||s?.readError?'bad':'muted'">{{state(s)}}</span></td></tr>
     <tr><th scope="row" title="成功响应次数 / 总样本数；HTTP 响应不等于业务验证通过">响应 / 样本</th><td v-for="{site,summary:s} in sites" :key="site.id"><template v-if="s?.total&&!error&&!s.readError"><span :class="tone(s)">{{s.successRate==null?'—':s.successRate.toFixed(0)+'%'}}</span><span class="site-samples"> · {{s.success}}/{{s.total}}</span></template><span v-else class="muted">—</span></td></tr>
     <tr v-if="hasP95"><th scope="row">尾部 P95</th><td v-for="{site,summary:s} in sites" :key="site.id"><span v-if="s&&s.success>=20">{{ms(s.p95)}}</span><span v-else class="muted">样本不足</span></td></tr>
     <tr v-if="sites.some(s=>notes[s.site.id])"><th scope="row">提示</th><td v-for="{site} in sites" :key="site.id" class="site-note warn">{{notes[site.id]}}</td></tr>
     <tr v-if="!commonTime&&sites.some(s=>s.summary?.lastAt)"><th scope="row">测量时间</th><td v-for="{site,summary:s} in sites" :key="site.id"><time>{{date(s?.lastAt)}}</time></td></tr>
    </tbody>
   </table>
  </div>
  <div class="site-metrics-mobile"><small class="site-mobile-label">中位 P50 · 响应率 / 成功与总样本</small>
   <article v-for="{site,summary:s} in sites" :key="site.id" class="site-metric" :data-site="site.id">
    <button type="button" :aria-label="'查看 '+site.name+' 原始历史'" @click="$emit('history',site.id)">{{site.name}} ↗</button>
    <p v-if="s?.total&&!error&&!s.readError"><strong>{{ms(s.p50)}}</strong><span :class="tone(s)">{{s.successRate?.toFixed(0)}}% · {{s.success}}/{{s.total}}</span></p><p v-else class="muted">{{state(s)}}</p>
    <small v-if="hasP95">P95 {{s&&s.success>=20?ms(s.p95):'样本不足'}}</small>
    <small v-if="notes[site.id]" class="warn">{{notes[site.id]}}</small><time v-if="!commonTime">{{date(s?.lastAt)}}</time>
   </article>
  </div>
  <div class="site-measurement-method"><span>{{summaries?.some(s=>s.group?.source==='monitor')?'含旧监测 RTT':'HTTP 首字节（非业务验证）'}}<template v-if="commonTime"> · {{date(commonTime)}}</template></span><span v-if="!hasP95">P95 样本不足，需20个成功样本</span></div>
 </section>
</template>

<style scoped>
.site-metrics{min-width:0;color:var(--ink)}.site-metrics-scroll{max-width:100%;overflow-x:auto;overscroll-behavior-x:contain}.site-metrics-table{width:100%;min-width:540px;border-collapse:collapse;table-layout:fixed;font-size:12px;line-height:1.5}.site-metrics-table th,.site-metrics-table td{text-align:left;padding:4px 6px;vertical-align:top;border:0}.site-metrics-table th:first-child{width:78px;padding-left:0;font-size:12px;font-weight:400;color:var(--muted);white-space:nowrap}.site-metrics-table thead th{padding-bottom:7px}.site-metrics-table button,.site-metric>button{border:0;background:none;padding:0;font-size:12px;font-weight:550;text-align:left;white-space:nowrap;color:var(--ink)}.site-metrics-table button span{color:var(--blue);font-size:11px}.site-metrics-table button:hover,.site-metric>button:hover{color:var(--blue)}.site-metrics-table td>strong{font-size:15px;font-weight:600;white-space:nowrap}.site-samples,.site-metrics-table time{color:var(--muted)}.site-note{font-size:12px;overflow-wrap:anywhere}.site-measurement-method{display:flex;flex-wrap:wrap;gap:3px 12px;color:var(--muted);font-size:12px;line-height:1.5;margin-top:7px}.site-metrics-mobile{display:none}
@media(max-width:740px){.site-metrics-scroll{display:none}.site-metrics-mobile{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px 12px}.site-mobile-label{grid-column:1/-1;color:var(--muted);font-size:12px}.site-metric{min-width:0;border-bottom:1px solid var(--line);padding-bottom:7px;font-size:12px}.site-metric p{display:flex;justify-content:space-between;flex-wrap:wrap;gap:4px;margin:4px 0 0}.site-metric strong{font-size:15px}.site-metric small,.site-metric time{display:block;font-size:12px;line-height:1.5;color:var(--muted)}}
</style>
