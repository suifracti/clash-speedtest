<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {date,type LatencyTest} from '../domain'
import {type SiteQualitySummary} from '../nodeQuality'
import {latencyTargets,latencyTargetId} from '../latencyTargets'
import {siteRoundHistory} from '../siteRoundPresentation'
import TrendChart from './TrendChart.vue'
import HealthBars from './HealthBars.vue'
import RoundResults from './RoundResults.vue'
import Modal from './Modal.vue'
const props=defineProps<{summaries?:SiteQualitySummary[];targetIds:string[];tests?:LatencyTest[];viewMode?:'latest'|'window';error?:string;compact?:boolean}>()
const emit=defineEmits<{history:[target:string]}>(),active=ref(''),fixedRound=ref(''),preview=ref<Record<string,string>>({})
const states:Record<string,string>={unmeasured:'未测',responded:'已响应',partial:'部分响应',failed:'无响应',cancelled:'已取消',not_executed:'未执行',source_limited:'源限流'}
const siteRows=(ids:string[])=>ids.flatMap(id=>{
 const site=latencyTargets.find(t=>t.id===id),aggregate=props.summaries?.find(s=>s.targetId===id)
 if(!site)return []
 const rounds=siteRoundHistory(aggregate,props.tests),selectedId=preview.value[id]||(active.value===id?fixedRound.value:''),selected=rounds.find(r=>r.point.id===selectedId)
 const summary=selected?.summary||(props.viewMode==='window'?aggregate:rounds[0]?.summary||aggregate)
 return [{site,aggregate,summary,rounds,selected,points:rounds.map(r=>r.point).reverse()}]
})
const sites=computed(()=>siteRows(props.targetIds)),historySites=computed(()=>siteRows(latencyTargets.map(s=>s.id)))
const caption=(selected:boolean)=>selected?'所选轮次':props.viewMode==='window'?'时间范围汇总':'最近一次'
const ms=(value:number|null|undefined)=>value==null?'—':Math.round(value)+' ms'
const tone=(s?:SiteQualitySummary)=>!s?.total?'muted':s.successRate===100?'good':s.success?'warn':'bad'
const state=(s?:SiteQualitySummary)=>props.error||s?.readError?'读取失败':!s?'读取中…':!s.readComplete&&!s.total?'读取不完整':states[s.state]||'未确认'
const current=computed(()=>historySites.value.find(s=>s.site.id===active.value))
const selectedPoint=computed(()=>current.value?.selected?.point)
function choose(id:string,round=''){active.value=id;fixedRound.value=round}
function showAll(){active.value='all';fixedRound.value='';preview.value={}}
defineExpose({showAll})
function fullHistory(id:string){active.value='';fixedRound.value='';emit('history',id)}
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
 <section class="site-metrics" :class="{compact}" aria-label="逐站响应率、样本和延迟分布">
  <div v-if="!compact" class="site-view-caption"><strong>{{viewMode==='window'?'时间范围汇总':'最近一次检测'}}</strong><span>每个站点独立显示，不用旧成功填补本次失败</span></div>
  <div class="site-results-scroll" tabindex="0" aria-label="逐站指标，可横向滚动">
   <div class="site-results-grid" :class="{'overview-sites':compact}" :style="{'--sites':Math.max(1,sites.length)}">
    <article v-for="{site,summary:s,rounds,points,aggregate,selected} in sites" :key="site.id" class="site-metric" :data-site="site.id" :class="{'round-pinned':active===site.id&&!!fixedRound}">
     <button class="site-name" type="button" :aria-label="'查看 '+site.name+' 原始历史'" :aria-expanded="active===site.id" @click="choose(site.id)">{{site.name}}</button>
     <button type="button" class="site-latest" :class="error||s?.readError?'bad':tone(s)" :aria-label="site.name+' 本次结果与历史对比'" @click="choose(site.id)">{{s?.p50!=null&&!error&&!s.readError?ms(s.p50):state(s)}}</button>
     <span class="site-response" v-if="s?.total&&!error&&!s.readError">{{s.success}} / {{s.total}} 次响应</span><span v-else class="site-response">{{s?.excluded?'未执行或取消 '+s.excluded+' 次':'尚无有效样本'}}</span>
     <small class="site-metric-note warn">{{selected&&selected.summary.group?.key!==rounds[0]?.summary.group?.key?'与最近一次条件不同':note(site.id)}}</small>
     <div v-if="points.length" class="site-history-choice"><span v-if="!compact">{{caption(!!selected)}}</span><HealthBars :points="points" :selected-id="active===site.id?fixedRound:''" :label="site.name+' 历史轮次'" @open="choose(site.id,$event.id)" @inspect="preview[site.id]=$event.id" @leave="delete preview[site.id]"/></div>
     <time v-if="!compact||points.length">{{selected?(active===site.id&&fixedRound===selected.point.id?'已固定 · ':'预览 · '):''}}{{date(selected?.test.finished_at||(viewMode==='window'?undefined:rounds[0])?.test.finished_at||s?.lastAt)}}</time>
     <small v-if="s?.success&&s.success>=20">尾部延迟 P95 {{ms(s.p95)}}</small>
     <button v-if="active===site.id&&fixedRound" type="button" class="text-button site-return-latest" @click="fixedRound='';delete preview[site.id]">返回{{viewMode==='window'?'汇总':'最近一次'}}</button>
     <small v-if="aggregate&&!aggregate.readComplete" class="warn">历史读取不完整</small>
    </article>
   </div>
  </div>
  <div class="site-measurement-method"><span>{{compact?'每格一轮 · 左旧右新 · 悬停预览，点击固定':'历史条：每格一轮，左旧右新 · 悬停预览，点击固定并看曲线'}}</span><span v-if="!compact">这里测的是 HTTP 首字节响应，不代表服务解锁</span></div>
  <Modal v-if="active" :title="active==='all'?'六站延迟历史':(current?.site.name||'站点')+' · 逐轮历史'" wide @close="active='';fixedRound=''">
   <div v-if="active==='all'" class="site-history-gallery" aria-label="六站历史曲线">
    <section v-for="{site,rounds,points,summary:s} in historySites" :key="site.id" :aria-label="site.name+' 历史曲线'">
     <header><button type="button" class="text-button" @click="choose(site.id)">{{site.name}}</button><strong :class="tone(s)">{{s?.p50!=null?ms(s.p50):state(s)}}</strong></header>
     <TrendChart compact :points="points" unit="ms" :label="site.name+' 延迟走势'"/>
     <small>{{date(rounds[0]?.test.finished_at||s?.lastAt)}}<template v-if="s?.total"> · {{s.success}} / {{s.total}} 次响应</template></small>
    </section>
   </div>
   <section v-else-if="current" class="site-history-panel" :aria-label="current.site.name+' 延迟历史对比'">
    <div class="site-history-single">
     <TrendChart compact :points="current.points" unit="ms" :label="current.site.name+' 延迟走势'"/>
     <div><div class="site-round-list"><button v-for="(round,i) in current.rounds.slice(0,8)" :key="round.test.attempt_id" type="button" :aria-pressed="fixedRound===round.point.id" @click="fixedRound=round.point.id"><span>{{i===0?'本次':i===1?'上次':'历史'}} · {{date(round.test.finished_at)}}</span><strong :class="tone(round.summary)">{{round.summary.p50!=null?ms(round.summary.p50):states[round.summary.state]}}</strong><span>{{round.summary.success}} / {{round.summary.total}} 次响应</span></button></div>
     <details v-if="selectedPoint" class="site-round-details"><summary>逐次采样</summary><RoundResults :point="selectedPoint"/></details></div>
    </div>
    <button class="text-button site-all-history" type="button" @click="fullHistory(current.site.id)">完整历史与多节点对比</button>
   </section>
  </Modal>
 </section>
</template>
<style scoped>
.site-metrics{min-width:0;color:var(--ink);text-align:left}.site-view-caption{display:flex;flex-wrap:wrap;gap:5px 12px;align-items:baseline;margin-bottom:6px}.site-view-caption>strong{font-size:13px;font-weight:550}.site-view-caption>span{font-size:12px;color:var(--muted)}.site-results-scroll{max-width:100%;overflow-x:auto;overscroll-behavior-x:contain}.site-results-grid{display:grid;grid-template-columns:repeat(var(--sites),minmax(112px,1fr));min-width:calc(var(--sites) * 120px);gap:0}.site-metric{min-width:0;padding:3px 10px 4px;border-left:1px solid var(--line);border-radius:0;background:none;font-size:12px;line-height:1.6}.site-name,.site-latest{display:block;border:0;background:transparent;padding:0;text-align:left;color:var(--ink);cursor:pointer}.site-name{display:flex;justify-content:space-between;align-items:center;gap:6px;width:100%;font-size:13px;font-weight:550;margin-bottom:5px;white-space:nowrap}.site-name span{color:var(--muted);font-size:12px}.site-latest{font-size:16px;font-weight:600;line-height:1.5;margin-bottom:3px;min-height:24px;letter-spacing:-.45px;font-variant-numeric:tabular-nums}.site-latest:hover,.site-name:hover{text-decoration:underline}.site-response,.site-metric small,.site-metric time{display:block;font-size:12px;line-height:1.6;color:var(--muted)}.site-response{font-size:12px}.site-metric-note{min-height:18px;margin:3px 0 5px}.site-metric .warn{color:var(--warning,#986700)}.site-history-choice{padding-top:3px;font-size:12px;color:var(--muted)}.site-history-choice>.health-bars{height:25px;max-width:none;margin:4px 0 5px}.site-metric time{font-variant-numeric:tabular-nums;font-size:12px}.site-metric.round-pinned{box-shadow:inset 0 2px var(--blue)}.site-return-latest{font-size:12px;margin-top:7px}.site-measurement-method{display:flex;justify-content:space-between;flex-wrap:wrap;gap:4px 12px;color:var(--muted);font-size:12px;line-height:1.7;margin-top:10px}.site-history-panel{margin:0;min-width:0}.site-history-single{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:16px;align-items:start}.site-history-gallery{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px 16px}.site-history-gallery>section{min-width:0}.site-history-gallery header{display:flex;align-items:baseline;justify-content:space-between;font-size:13px}.site-history-gallery small{font-size:12px;color:var(--muted)}.site-history-panel>header{display:flex;justify-content:space-between;align-items:start;gap:10px;font-size:14px}.site-history-panel>header small{display:block;color:var(--muted);font-size:12px;margin-top:4px}.site-history-panel button{cursor:pointer}.site-history-panel>header button{border:0;background:none;color:var(--muted);font-size:12px;padding:4px}.site-round-list{display:grid;gap:3px;margin:0 0 6px;max-height:170px;overflow:auto}.site-round-list>button{display:grid;grid-template-columns:minmax(0,1fr) minmax(60px,.55fr) minmax(76px,.55fr);align-items:center;gap:8px;border:1px solid transparent;border-radius:7px;padding:5px 7px;background:var(--surface-soft);text-align:left;font-size:12px;color:var(--ink);font-variant-numeric:tabular-nums}.site-round-list>button[aria-pressed=true]{border-color:var(--blue)}.site-round-list>button>span:last-child{text-align:right;color:var(--muted)}.site-all-history{margin-top:12px;font-size:12px}.site-round-details{font-size:12px;max-height:210px;overflow:auto}.site-round-details>summary{cursor:pointer;color:var(--blue);padding:7px 0}

@media(max-width:740px){.site-results-grid{grid-template-columns:repeat(2,minmax(0,1fr));min-width:0}.site-round-list>button{display:flex;flex-wrap:wrap;gap:5px 12px}.site-round-list>button>span:first-child{flex-basis:100%}.site-round-list>button>span:last-child{margin-left:auto}.site-view-caption>span{flex-basis:100%}}
.site-metric:first-child{border-left:0;padding-left:0}
@media(max-width:740px){.site-results-grid{gap:12px 0}.site-metric:nth-child(odd){border-left:0;padding-left:0}}
.site-metric :deep(.health-cell:not(.has-record)){display:none}
.site-metric :deep(.health-cell.has-record){min-width:4px;max-width:20px}
.site-metric :deep(.health-bars){justify-content:flex-start}
.site-metrics.compact .site-results-grid{grid-template-columns:repeat(3,minmax(0,1fr));min-width:0;gap:10px 0}
.site-metrics.compact .site-metric{padding:2px 9px 3px}
.site-metrics.compact .site-metric:nth-child(3n+1){border-left:0;padding-left:0}
.site-metrics.compact .site-latest{font-size:15px;min-height:22px;letter-spacing:0}
.site-metrics.compact .site-name{font-size:12px;margin-bottom:3px}
.site-metrics.compact .site-metric-note{min-height:0;margin:2px 0}
.site-metrics.compact .site-metric-note:empty{display:none}
.site-metrics.compact .site-history-choice{padding-top:0}
.site-metrics.compact .site-history-choice>.health-bars{height:28px;margin:5px 0 3px}
.site-metrics.compact .site-measurement-method{margin-top:5px;font-size:12px}
@media(max-width:740px){.site-metrics.compact .site-results-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.site-metrics.compact .site-metric:nth-child(n){border-left:1px solid var(--line);padding-left:9px}.site-metrics.compact .site-metric:nth-child(odd){border-left:0;padding-left:0}}
@media(max-width:740px){.site-history-gallery{grid-template-columns:repeat(2,minmax(0,1fr))}.site-history-single{grid-template-columns:minmax(0,1fr)}}
@media(max-width:520px){.site-history-gallery{grid-template-columns:minmax(0,1fr)}}
</style>
