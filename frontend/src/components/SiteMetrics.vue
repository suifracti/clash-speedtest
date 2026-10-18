<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {date,type LatencyTest} from '../domain'
import {type SiteQualitySummary} from '../nodeQuality'
import {latencyTargets,latencyTargetId} from '../latencyTargets'
import {siteRoundHistory} from '../siteRoundPresentation'
import TrendChart from './TrendChart.vue'
import HealthBars from './HealthBars.vue'
import RoundResults from './RoundResults.vue'
const props=defineProps<{summaries?:SiteQualitySummary[];targetIds:string[];tests?:LatencyTest[];viewMode?:'latest'|'window';error?:string}>()
const emit=defineEmits<{history:[target:string]}>(),active=ref(''),fixedRound=ref(''),preview=ref<Record<string,string>>({})
const states:Record<string,string>={unmeasured:'未测',responded:'已响应',partial:'部分响应',failed:'无响应',cancelled:'已取消',not_executed:'未执行',source_limited:'源限流'}
const sites=computed(()=>props.targetIds.flatMap(id=>{
 const site=latencyTargets.find(t=>t.id===id),aggregate=props.summaries?.find(s=>s.targetId===id)
 if(!site)return []
 const rounds=siteRoundHistory(aggregate,props.tests),selectedId=preview.value[id]||(active.value===id?fixedRound.value:''),selected=rounds.find(r=>r.point.id===selectedId)
 const summary=selected?.summary||(props.viewMode==='window'?aggregate:rounds[0]?.summary||aggregate)
 return [{site,aggregate,summary,rounds,selected,points:rounds.map(r=>r.point).reverse()}]
}))
const caption=(selected:boolean)=>selected?'所选轮次':props.viewMode==='window'?'时间范围汇总':'最近一次'
const ms=(value:number|null|undefined)=>value==null?'—':Math.round(value)+' ms'
const tone=(s?:SiteQualitySummary)=>!s?.total?'muted':s.successRate===100?'good':s.success?'warn':'bad'
const state=(s?:SiteQualitySummary)=>props.error||s?.readError?'读取失败':!s?'读取中…':!s.readComplete&&!s.total?'读取不完整':states[s.state]||'未确认'
const current=computed(()=>sites.value.find(s=>s.site.id===active.value))
const selectedPoint=computed(()=>current.value?.selected?.point)
function choose(id:string,round=''){active.value=id;fixedRound.value=round}
watch(()=>props.viewMode,()=>{fixedRound.value='';preview.value={}})
watch(()=>props.targetIds,ids=>{if(active.value&&!ids.includes(active.value)){active.value='';fixedRound.value=''}},{deep:true})
function note(id:string){
 const data=sites.value.find(s=>s.site.id===id),test=data?.selected?.test||(props.viewMode==='window'?undefined:data?.rounds[0]?.test)
 if(!test)return ''
 const errors=(test.samples||[]).filter(s=>latencyTargetId(s.target||test.target||'')===id).map(s=>s.error||'')
 if(errors.some(s=>/HTTP\s*429/.test(s)))return '429 限流'
 const code=errors.map(e=>e.match(/HTTP\s*(\d{3})/)?.[1]).find(c=>c&&+c>=400)
 if(code)return code+' 拒绝'
 if(errors.some(e=>/deadline|timeout|超时/i.test(e)))return '请求超时'
 if(errors.some(e=>/\bEOF\b/i.test(e)))return '连接提前结束'
 return ''
}
</script>
<template>
 <section class="site-metrics" aria-label="逐站响应率、样本和延迟分布">
  <div class="site-view-caption"><strong>{{viewMode==='window'?'时间范围汇总':'最近一次检测'}}</strong><span>每个站点独立显示，不用旧成功填补本次失败</span></div>
  <div class="site-results-scroll" tabindex="0" aria-label="逐站指标，可横向滚动">
   <div class="site-results-grid" :style="{'--sites':Math.max(1,sites.length)}">
    <article v-for="{site,summary:s,rounds,points,aggregate,selected} in sites" :key="site.id" class="site-metric" :data-site="site.id" :class="{'round-pinned':active===site.id&&!!fixedRound}">
     <button class="site-name" type="button" :aria-label="'查看 '+site.name+' 原始历史'" :aria-expanded="active===site.id" @click="choose(site.id)">{{site.name}} <span aria-hidden="true">↗</span></button>
     <button type="button" class="site-latest" :class="error||s?.readError?'bad':tone(s)" :aria-label="site.name+' 本次结果与历史对比'" @click="choose(site.id)">{{s?.p50!=null&&!error&&!s.readError?ms(s.p50):state(s)}}</button>
     <span class="site-response" v-if="s?.total&&!error&&!s.readError">{{s.success}} / {{s.total}} 次响应</span><span v-else class="site-response">{{s?.excluded?'未执行或取消 '+s.excluded+' 次':'尚无有效样本'}}</span>
     <small class="site-metric-note warn">{{selected&&selected.summary.group?.key!==rounds[0]?.summary.group?.key?'与最近一次条件不同':note(site.id)}}</small>
     <div v-if="points.length" class="site-history-choice"><span>{{caption(!!selected)}}</span><HealthBars :points="points" :selected-id="active===site.id?fixedRound:''" :label="site.name+' 历史轮次'" @open="choose(site.id,$event.id)" @inspect="preview[site.id]=$event.id" @leave="delete preview[site.id]"/></div>
     <time>{{selected?(active===site.id&&fixedRound===selected.point.id?'已固定 · ':'预览 · '):''}}{{date(selected?.test.finished_at||(viewMode==='window'?undefined:rounds[0])?.test.finished_at||s?.lastAt)}}</time>
     <small v-if="s?.success&&s.success>=20">尾部延迟 P95 {{ms(s.p95)}}</small>
     <button v-if="active===site.id&&fixedRound" type="button" class="text-button site-return-latest" @click="fixedRound='';delete preview[site.id]">返回{{viewMode==='window'?'汇总':'最近一次'}}</button>
     <small v-if="aggregate&&!aggregate.readComplete" class="warn">历史读取不完整</small>
    </article>
   </div>
  </div>
  <div class="site-measurement-method"><span>历史条：每格一轮，左旧右新 · 悬停预览，点击固定并看曲线</span><span>这里测的是 HTTP 首字节响应，不代表服务解锁</span></div>
  <section v-if="current" class="site-history-panel" :aria-label="current.site.name+' 延迟历史对比'">
   <header><div><strong>{{current.site.name}} · 逐轮历史</strong><small>{{fixedRound?'正在查看固定轮次':'选择一轮查看数值、曲线和采样明细'}}</small></div><button type="button" @click="active='';fixedRound=''">收起 ×</button></header>
   <TrendChart :points="current.points" unit="ms" :label="current.site.name+' 延迟走势'"/>
   <div class="site-round-list"><button v-for="(round,i) in current.rounds.slice(0,8)" :key="round.test.attempt_id" type="button" :aria-pressed="fixedRound===round.point.id" @click="fixedRound=round.point.id"><span>{{i===0?'本次':i===1?'上次':'历史'}} · {{date(round.test.finished_at)}}</span><strong :class="tone(round.summary)">{{round.summary.p50!=null?ms(round.summary.p50):states[round.summary.state]}}</strong><span>{{round.summary.success}} / {{round.summary.total}} 次响应</span></button></div>
   <details v-if="selectedPoint" class="site-round-details"><summary>查看所选轮次的逐次采样</summary><RoundResults :point="selectedPoint"/></details>
   <button class="text-button site-all-history" type="button" @click="emit('history',current.site.id)">完整历史与多节点对比 →</button>
  </section>
 </section>
</template>
<style scoped>
.site-metrics{min-width:0;color:var(--ink);text-align:left}.site-view-caption{display:flex;flex-wrap:wrap;gap:5px 12px;align-items:baseline;margin-bottom:12px}.site-view-caption>strong{font-size:14px;font-weight:650}.site-view-caption>span{font-size:11px;color:var(--muted)}.site-results-scroll{max-width:100%;overflow-x:auto;overscroll-behavior-x:contain}.site-results-grid{display:grid;grid-template-columns:repeat(var(--sites),minmax(112px,1fr));min-width:calc(var(--sites) * 120px);gap:7px}.site-metric{min-width:0;padding:12px 10px;border:1px solid var(--line,#d8deeb);border-radius:9px;background:var(--surface-soft,#f5f7fb);font-size:12px;line-height:1.6}.site-name,.site-latest{display:block;border:0;background:transparent;padding:0;text-align:left;color:var(--ink);cursor:pointer}.site-name{display:flex;justify-content:space-between;align-items:center;gap:6px;width:100%;font-size:13px;font-weight:550;margin-bottom:8px;white-space:nowrap}.site-name span{color:var(--muted);font-size:11px}.site-latest{font-size:20px;font-weight:600;line-height:1.5;margin-bottom:3px;min-height:30px;letter-spacing:-.45px;font-variant-numeric:tabular-nums}.site-latest:hover,.site-name:hover{text-decoration:underline}.site-response,.site-metric small,.site-metric time{display:block;font-size:11px;line-height:1.6;color:var(--muted)}.site-response{font-size:12px}.site-metric-note{min-height:18px;margin:3px 0 9px}.site-metric .warn{color:var(--warning,#986700)}.site-history-choice{padding-top:8px;border-top:1px solid var(--line);font-size:10px;color:var(--muted)}.site-history-choice>.health-bars{height:25px;max-width:none;margin:4px 0 5px}.site-metric time{font-variant-numeric:tabular-nums;font-size:10px}.site-metric.round-pinned{border-color:var(--blue)}.site-return-latest{font-size:11px;margin-top:7px}.site-measurement-method{display:flex;justify-content:space-between;flex-wrap:wrap;gap:4px 12px;color:var(--muted);font-size:11px;line-height:1.7;margin-top:10px}.site-history-panel{margin-top:16px;padding-top:16px;border-top:1px solid var(--line);min-width:0}.site-history-panel>header{display:flex;justify-content:space-between;align-items:start;gap:10px;font-size:14px}.site-history-panel>header small{display:block;color:var(--muted);font-size:11px;margin-top:4px}.site-history-panel button{cursor:pointer}.site-history-panel>header button{border:0;background:none;color:var(--muted);font-size:12px;padding:4px}.site-round-list{display:grid;gap:5px;margin:10px 0}.site-round-list>button{display:grid;grid-template-columns:minmax(0,1fr) minmax(84px,.55fr) minmax(100px,.55fr);align-items:center;gap:8px;border:1px solid transparent;border-radius:7px;padding:9px 12px;background:var(--surface-soft);text-align:left;font-size:12px;color:var(--ink);font-variant-numeric:tabular-nums}.site-round-list>button[aria-pressed=true]{border-color:var(--blue)}.site-round-list>button>span:last-child{text-align:right;color:var(--muted)}.site-all-history{margin-top:12px;font-size:12px}.site-round-details{font-size:12px}.site-round-details>summary{cursor:pointer;color:var(--blue);padding:7px 0}
@media(max-width:1100px){.site-results-grid{grid-template-columns:repeat(3,minmax(0,1fr));min-width:0}.site-metric{padding:10px}.site-measurement-method{font-size:11px}}
@media(max-width:620px){.site-results-grid{grid-template-columns:repeat(2,minmax(0,1fr));min-width:0}.site-round-list>button{display:flex;flex-wrap:wrap;gap:5px 12px}.site-round-list>button>span:first-child{flex-basis:100%}.site-round-list>button>span:last-child{margin-left:auto}.site-view-caption>span{flex-basis:100%}}
.site-metric :deep(.health-cell:not(.has-record)){display:none}
.site-metric :deep(.health-cell.has-record){min-width:4px;max-width:20px}
.site-metric :deep(.health-bars){justify-content:flex-start}
</style>
