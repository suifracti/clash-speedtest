<script setup lang="ts">
import type {SiteQualitySummary} from '../nodeQuality'
import {latencyTargets} from '../latencyTargets'
import {siteReason} from '../siteQualityView'
import {date} from '../domain'
defineProps<{summaries?:SiteQualitySummary[];error?:string}>()
defineEmits<{history:[target:string];load:[]}>()
const states:Record<string,string>={unmeasured:'未测',responded:'已响应',partial:'部分响应',failed:'采样失败',cancelled:'已取消',not_executed:'未执行',source_limited:'源限流'}
const number=(value:number|null|undefined)=>value==null?'—':value.toFixed(1)
</script>
<template>
 <details class="site-quality" @toggle="($event.target as HTMLDetailsElement).open && $emit('load')">
  <summary>六站逐站质量 · {{error?'读取失败':!summaries?'尚未读取（展开读取）':summaries.filter(s=>s.total>0).length+' / 6 站有样本'}} · 查看统计与条件</summary>
  <p v-if="error" role="alert">{{error}}</p>
  <p class="small muted">HTTP 首字节响应率不等于业务可用。同条件观察值排序可用1个成功样本，不代表稳定性。P50 / 响应率排名需 ≥5 成功样本，P95 需 ≥20；均需完整、24h 内且条件一致。</p>
  <div class="site-quality-scroll" tabindex="0" aria-label="六站质量证据表，可横向滚动">
   <table><thead><tr><th>站点 / 状态</th><th>P50 / P95</th><th>成功 / 总数 · 响应率</th><th>时效 / 准入</th><th>测量条件 / 原始历史</th></tr></thead>
    <tbody><tr v-for="(site,i) in latencyTargets" :key="site.id">
     <th scope="row">{{site.name}}<small>{{summaries?states[summaries[i]?.state]:'尚未读取'}}</small></th>
     <td>{{number(summaries?.[i]?.p50)}} / {{number(summaries?.[i]?.p95)}} ms</td>
     <td>{{summaries?.[i]?.success??'—'}} / {{summaries?.[i]?.total??'—'}} · {{number(summaries?.[i]?.successRate)}}%<small v-if="summaries?.[i]">排除 {{summaries[i].excluded}} · 取消 {{summaries[i].observations.cancelled+summaries[i].attemptStates.cancelled}} · 未执行 {{summaries[i].observations.notExecuted+summaries[i].attemptStates.notExecuted}} · 源限流 {{summaries[i].observations.sourceLimited}}</small></td>
     <td>{{date(summaries?.[i]?.lastAt)}}<small>{{summaries?.[i]?.readError}}</small><small v-if="summaries?.[i]">{{summaries[i].reasons.map(siteReason).join(' · ')||'证据满足排名门槛'}}<br>P50 {{summaries[i].rankable.p50?'可排名':'不可排名'}} · P95 {{summaries[i].rankable.p95?'可排名':'不可排名'}}</small></td>
     <td><details v-if="summaries?.[i]?.group"><summary>{{summaries[i].group!.method}} · {{summaries[i].group!.source==='monitor'?'旧 RTT 诊断':'当前方法'}}</summary><code>{{summaries[i].group!.condition}}</code><small>窗口 {{date(summaries[i].since)}} — {{date(summaries[i].until)}}</small></details><span v-else>条件未确认</span><button class="text-button" :aria-label="`查看 ${site.name} 原始历史`" @click="$emit('history',site.id)">原始历史</button></td>
    </tr></tbody>
   </table>
  </div>
 </details>
</template>
<style scoped>
.site-quality{grid-column:1/-1;min-width:0;margin:8px 0;color:var(--ink)}
.site-quality>summary{cursor:pointer;font-size:13px;padding:8px;background:var(--surface-soft);border-radius:8px}
.site-quality-scroll{overflow-x:auto;max-width:100%}table{width:100%;min-width:720px;border-collapse:collapse;font-size:13px}th,td{text-align:left;padding:10px;border-bottom:1px solid var(--line);vertical-align:top}th{font-weight:600}small{display:block;color:var(--muted);font-size:12px;line-height:1.5}code{display:block;max-width:300px;overflow-wrap:anywhere;white-space:normal}button{display:block}
</style>
