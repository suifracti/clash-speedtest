<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {date,type LatencyTest} from '../domain'
import {type SiteQualitySummary} from '../nodeQuality'
import {latencyTargets,latencyTargetId} from '../latencyTargets'
import {siteRoundHistory} from '../siteRoundPresentation'
import TrendChart from './TrendChart.vue'
import HealthBars from './HealthBars.vue'
import RoundResults from './RoundResults.vue'
const props=defineProps<{summaries?:SiteQualitySummary[];targetIds:string[];tests?:LatencyTest[];error?:string}>()
const emit=defineEmits<{history:[target:string]}>(),active=ref(''),fixedRound=ref('')
const states:Record<string,string>={unmeasured:'未测',responded:'已响应',partial:'部分响应',failed:'无响应',cancelled:'已取消',not_executed:'未执行',source_limited:'源限流'}
const sites=computed(()=>props.targetIds.flatMap(id=>{
 const site=latencyTargets.find(t=>t.id===id),aggregate=props.summaries?.find(s=>s.targetId===id)
 if(!site)return []
 const rounds=siteRoundHistory(aggregate,props.tests),summary=rounds[0]?.summary||aggregate
 return [{site,aggregate,summary,rounds,points:rounds.map(r=>r.point).reverse()}]
}))
const ms=(value:number|null|undefined)=>value==null?'—':Math.round(value)+' ms'
const tone=(s?:SiteQualitySummary)=>!s?.total?'muted':s.successRate===100?'good':s.success?'warn':'bad'
const state=(s?:SiteQualitySummary)=>props.error||s?.readError?'读取失败':!s?'读取中…':!s.readComplete&&!s.total?'读取不完整':states[s.state]
const current=computed(()=>sites.value.find(s=>s.site.id===active.value))
const selectedPoint=computed(()=>current.value?.points.find(p=>p.id===fixedRound.value))
function choose(id:string,round=''){active.value=id;fixedRound.value=round}
watch(()=>props.targetIds,ids=>{if(active.value&&!ids.includes(active.value)){active.value='';fixedRound.value=''}},{deep:true})
function note(id:string){const data=sites.value.find(s=>s.site.id===id),test=data?.rounds[0]?.test
 if(!test)return ''
 const errors=(test.samples||[]).filter(s=>latencyTargetId(s.target||test.target||'')===id).map(s=>s.error||'')
 if(errors.some(s=>/HTTP\s*429/.test(s)))return '429 限流'
 const code=errors.map(e=>e.match(/HTTP\s*(\d{3})/)?.[1]).find(c=>c&&+c>=400)
 if(code)return code+' 拒绝'
 if(errors.some(e=>/deadline|timeout|超时/i.test(e)))return '请求超时'
 return ''
}
</script>
<template>
 <section class="site-metrics" aria-label="逐站响应率、样本和延迟分布">
  <div class="site-results-scroll" tabindex="0" aria-label="逐站指标，可横向滚动">
   <div class="site-results-grid" :style="{'--sites':Math.max(1,sites.length)}">
    <article v-for="{site,summary:s,rounds,points,aggregate} in sites" :key="site.id" class="site-metric" :data-site="site.id">
     <button class="site-name" type="button" :aria-label="'查看 '+site.name+' 原始历史'" :aria-expanded="active===site.id" @click="choose(site.id)">{{site.name}} <span aria-hidden="true">⌁</span></button>
     <button type="button" class="site-latest" :class="error||s?.readError?'bad':tone(s)" :aria-label="site.name+' 本次结果与历史对比'" @click="choose(site.id)">{{s?.p50!=null&&!error&&!s.readError?ms(s.p50):state(s)}}</button>
     <span class="site-response" v-if="s?.total&&!error&&!s.readError">{{rounds.length?'本次':'窗口统计'}} {{s.success}} / {{s.total}} 次响应</span><span v-else class="site-response">{{s?.excluded?'未执行或取消 '+s.excluded+' 次':'尚无有效样本'}}</span>
     <small v-if="rounds[1]">上次 {{rounds[1].summary.p50!=null?ms(rounds[1].summary.p50):states[rounds[1].summary.state]}}<template v-if="rounds[0]?.summary.group?.key!==rounds[1].summary.group?.key"> · 条件不同</template></small>
     <small v-if="note(site.id)" class="warn">{{note(site.id)}}</small>
     <small v-if="s?.success&&s.success>=20">P95 {{ms(s.p95)}}</small>
     <HealthBars v-if="points.length" :points="points" :label="site.name+' 历史'" @open="choose(site.id,$event.id)" @inspect="()=>{}" @leave="()=>{}"/>
     <time>{{date(rounds[0]?.test.finished_at||s?.lastAt)}}</time>
     <small v-if="aggregate&&!aggregate.readComplete" class="warn">历史读取不完整</small>
    </article>
   </div>
  </div>
  <div class="site-measurement-method">本次单轮 · 响应次数 / 本轮采样数 · 小条左旧右新，点数值对比历史<span>HTTP 响应不等于业务通过</span></div>
  <section v-if="current" class="site-history-panel" :aria-label="current.site.name+' 延迟历史对比'">
   <header><strong>{{current.site.name}} · 本次与历史</strong><button type="button" @click="active='';fixedRound=''">收起</button></header>
   <TrendChart :points="current.points" unit="ms" :label="current.site.name+' 延迟走势'"/>
   <div class="site-round-list"><button v-for="(round,i) in current.rounds.slice(0,8)" :key="round.test.attempt_id" type="button" :aria-pressed="fixedRound===round.point.id" @click="fixedRound=round.point.id"><span>{{i===0?'本次':i===1?'上次':'历史'}} · {{date(round.test.finished_at)}}</span><strong :class="tone(round.summary)">{{round.summary.p50!=null?ms(round.summary.p50):states[round.summary.state]}}</strong><span>{{round.summary.success}} / {{round.summary.total}} 次响应</span></button></div>
   <RoundResults v-if="selectedPoint" :point="selectedPoint"/>
   <button class="text-button" type="button" @click="emit('history',current.site.id)">完整历史与多节点对比</button>
  </section>
 </section>
</template>
<style scoped>
.site-metrics{min-width:0;color:var(--ink)}.site-results-scroll{max-width:100%;overflow-x:auto;overscroll-behavior-x:contain}.site-results-grid{display:grid;grid-template-columns:repeat(var(--sites),minmax(120px,1fr));min-width:calc(var(--sites) * 120px);gap:0}.site-metric{min-width:0;padding:3px 10px 4px;border-left:1px solid var(--line);font-size:12px;line-height:1.5}.site-metric:first-child{border-left:0;padding-left:0}.site-name,.site-latest{display:block;border:0;background:transparent;padding:0;text-align:left;color:var(--ink);cursor:pointer}.site-name{font-size:12px;font-weight:550;margin-bottom:7px}.site-name span{color:var(--blue)}.site-latest{font-size:15px;font-weight:600;line-height:1.5;margin-bottom:3px;min-height:23px}.site-latest:hover,.site-name:hover{text-decoration:underline}.site-response,.site-metric small,.site-metric time{display:block;font-size:12px;line-height:1.6;color:var(--muted)}.site-metric .warn{color:var(--warning,#986700)}.site-metric .health-bars{height:24px;max-width:140px;margin:5px 0}.site-metric time{font-variant-numeric:tabular-nums}.site-measurement-method{display:flex;flex-wrap:wrap;gap:3px 12px;color:var(--muted);font-size:12px;line-height:1.6;margin-top:8px}.site-history-panel{margin-top:12px;padding-top:12px;border-top:1px solid var(--line);min-width:0}.site-history-panel>header{display:flex;justify-content:space-between;gap:10px;font-size:13px}.site-history-panel button{cursor:pointer}.site-history-panel>header button{border:0;background:none;color:var(--muted);font-size:12px}.site-round-list{display:grid;gap:3px;margin-bottom:9px}.site-round-list>button{display:flex;align-items:center;flex-wrap:wrap;gap:7px 15px;border:0;border-radius:4px;padding:6px 8px;background:var(--surface-soft);text-align:left;font-size:12px;color:var(--ink)}.site-round-list>button[aria-pressed=true]{outline:1px solid var(--blue)}.site-round-list>button>span:last-child{margin-left:auto;color:var(--muted)}
@media(max-width:740px){.site-results-grid{grid-template-columns:repeat(2,minmax(0,1fr));min-width:0;gap:12px 0}.site-metric:nth-child(odd){border-left:0;padding-left:0}.site-measurement-method{font-size:12px}.site-round-list>button>span:last-child{margin-left:0}}
</style>
