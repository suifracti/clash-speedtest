<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import type {MonitorSample,LatencyTest} from '../domain'
import {date} from '../domain'
import {analyzeHistory,type HistoryContext,type HistoryPoint} from '../historyAnalytics'
import Modal from './Modal.vue'
const props=defineProps<{samples:MonitorSample[];context:HistoryContext;tests?:LatencyTest[];profileLabels?:Record<string,string>}>()
const result=computed(()=>analyzeHistory(props.samples,props.context,props.tests)),group=ref(''),hover=ref(''),fixed=ref(''),raw=ref(false)
const selected=computed(()=>result.value.groups.find(g=>g.key===group.value)||result.value.groups[0])
const points=computed(()=>result.value.points.filter(p=>p.group===selected.value?.key))
const active=computed(()=>points.value.find(p=>p.id===(fixed.value||hover.value)))
const first=computed(()=>points.value[0]?.at||0),last=computed(()=>points.value.at(-1)?.at||first.value)
const max=computed(()=>Math.max(1,...points.value.filter(p=>p.success&&!p.excluded).map(p=>p.value||0)))
const x=(p:HistoryPoint)=>last.value===first.value?400:30+(p.at-first.value)/(last.value-first.value)*740
const y=(p:HistoryPoint)=>p.success&&!p.excluded&&p.value!==null?140-p.value/max.value*110:158
const ms=(v:number|null)=>v===null?'未测':v.toFixed(1)+' ms'
function pick(p:HistoryPoint){fixed.value=p.id;hover.value=p.id}
function select(key:string){group.value=key;fixed.value='';hover.value='';raw.value=false}
watch(()=>[props.context.since,props.context.until,props.context.profile,props.context.identity,props.context.target],()=>{group.value='';fixed.value='';hover.value='';raw.value=false})
watch(points,()=>{if(fixed.value&&!points.value.some(p=>p.id===fixed.value)){fixed.value='';raw.value=false}})
</script>
<template>
<section class="history-analytics" aria-label="已读历史健康分析">
 <h2>已读样本健康分析</h2>
 <p class="small muted">窗口 {{date(context.since)}} — {{date(context.until)}} · 接口已读 {{result.loaded}} 条 · 去重后窗口内 {{result.inWindow}} 条 · 重复／窗口外／无效时间 {{result.discarded}} 条 · {{context.loading?'读取中，统计暂不完整':result.complete?'窗口分页已读完':context.error?'读取失败，统计不完整':'仍有缺页，仅统计已读样本'}}</p>
 <p class="small muted">P50 为中位数，P95 为最近秩；仅有效成功延迟参与。成功率 = 成功 / 已执行且非取消、非本地阻断、非目标 429 的样本。业务／解锁结论未确认。日期覆盖按 UTC；监测按 run_id，手动检测按 attempt_id 计数，样本数不代表跨天稳定性。</p>
 <p v-if="!result.groups.length" class="empty-state">{{context.loading?'等待读取样本…':context.error?'未取得可统计样本，请重试读取。':'没有可统计样本（未测）。'}}</p>
 <div v-else class="table-scroll"><table><thead><tr><th>节点 / 订阅</th><th>目标 / 条件</th><th>P50 / P95</th><th>成功率 / 样本</th><th>时间覆盖</th></tr></thead><tbody>
 <tr v-for="g in result.groups" :key="g.key" :class="{chosen:selected?.key===g.key}"><td><button class="text-button" @click="select(g.key)">{{g.name||'旧名称未知'}}</button><small>{{profileLabels?.[g.profile]||g.profile}} · 身份 {{g.identity||'unknown'}} · 配置 {{g.revision||'unknown'}}</small></td><td class="break-text">{{g.target}}<small>{{g.probe}} · {{g.tier}}</small><details><summary>{{g.comparable?'已记录条件（仅此组可比）':'条件 unknown，不提供同条件排名'}}</summary><code>{{g.condition}}</code></details></td><td>{{ms(g.p50)}} / {{ms(g.p95)}}<small v-if="g.invalidTiming">无有效延迟 {{g.invalidTiming}} 条</small></td><td>{{g.successRate===null?'未测':g.successRate.toFixed(1)+'%'}}<small>成功 {{g.success}} / {{g.eligible}} · 原始 {{g.count}}</small><small>超时 {{g.timeout}} · 失败 {{g.failed}} · 排除 {{g.excluded}}</small></td><td>{{date(g.firstAt)}} — {{date(g.lastAt)}}<small>{{g.rounds}} 个已知轮次／检测尝试 · {{g.days}} 个 UTC 日期<template v-if="g.unknownRounds"> · {{g.unknownRounds}} 条轮次 unknown</template></small><small>跨度 {{((Date.parse(g.lastAt)-Date.parse(g.firstAt))/3600000).toFixed(1)}} 小时 · 不作稳定性推断</small></td></tr>
 </tbody></table></div>
 <template v-if="points.length"><div class="history-chart-head"><strong>{{selected?.name}} · {{selected?.target}} · 原始时刻趋势</strong><button v-if="fixed" class="text-button" @click="fixed='';raw=false">解除冻结</button></div>
 <p class="small muted">横轴为真实测试时间；纵轴为成功延迟（ms），底部标记失败／排除／无有效延迟。聚合值与原始子样本分别显示。悬停或键盘聚焦查看，点击或 Enter 固定。</p>
 <svg class="history-chart" viewBox="0 0 800 195" role="group" aria-label="历史真实时间趋势">
 <line x1="30" y1="140" x2="770" y2="140" class="axis"/><text x="30" y="18">{{max.toFixed(1)}} ms</text><text x="30" y="190">{{date(points[0]!.timestamp)}}</text><text x="770" y="190" text-anchor="end">{{date(points.at(-1)!.timestamp)}}</text>
 <circle v-for="p in points" :key="p.id" class="history-point" :class="{excluded:p.excluded,failed:!p.success,active:active?.id===p.id}" :cx="x(p)" :cy="y(p)" r="5" tabindex="0" role="button" :aria-label="`${date(p.timestamp)} ${p.id} ${p.excluded?'排除':p.success?ms(p.value):p.error||'失败'}`" @mouseenter="hover=p.id" @mouseleave="hover=''" @focus="hover=p.id" @blur="hover=''" @click="pick(p)" @keydown.enter.prevent="pick(p)" @keydown.space.prevent="pick(p)"><title>{{p.timestamp}} · {{p.id}} · {{p.error||ms(p.value)}}</title></circle>
 </svg>
 <div v-if="active" class="history-tooltip" aria-live="polite"><strong>{{fixed?'已固定':'样本'}} · {{active.timestamp}}</strong> · {{active.id}} · {{active.excluded?'排除':active.success?ms(active.value):active.error||'失败'}} · 轮次 {{active.round||'unknown'}} <button class="text-button" @click="pick(active);raw=true">原始 JSON / 轮次</button></div>
 </template>
 <Modal v-if="raw&&active" title="原始样本与已读轮次" wide @close="raw=false"><p class="muted">仅展示当前窗口已读的同轮证据；未读部分及跨节点／跨站点结论不推断。轮次 {{active.round||'unknown'}}</p><pre class="record-json">{{JSON.stringify(active.raw,null,2)}}</pre><details><summary>当前已读同轮子结果</summary><pre class="record-json">{{JSON.stringify(result.points.filter(p=>active?.round?p.round===active.round:p.id===active?.id).map(p=>p.raw),null,2)}}</pre></details></Modal>
</section>
</template>
<style scoped>
.history-analytics{border:1px solid var(--line);background:var(--surface);border-radius:8px;padding:14px;margin:12px 0;min-width:0}.history-analytics h2{font-size:15px;margin:0 0 8px}.history-analytics p{margin:7px 0}.history-analytics table{font-size:12px}.history-analytics td{vertical-align:top;max-width:280px}.history-analytics small{display:block;color:var(--muted);overflow-wrap:anywhere}.history-analytics code{white-space:normal;overflow-wrap:anywhere;font-size:11px}.history-analytics summary{cursor:pointer;color:var(--muted)}.chosen{background:var(--surface-soft)}.history-chart{width:100%;height:auto;max-height:230px;overflow:visible}.history-chart text{fill:var(--muted);font-size:11px}.axis{stroke:var(--line)}.history-point{fill:var(--blue);stroke:var(--surface);stroke-width:2;cursor:pointer}.history-point.failed{fill:var(--red,#cf4758)}.history-point.excluded{fill:var(--muted)}.history-point.active,.history-point:focus-visible{stroke:var(--ink);stroke-width:3;outline:0}.history-chart-head{display:flex;flex-wrap:wrap;justify-content:space-between;gap:8px;font-size:12px;margin-top:12px}.history-tooltip{padding:8px;background:var(--surface-soft);font-size:12px;overflow-wrap:anywhere}.history-analytics button:focus-visible,.history-analytics summary:focus-visible{outline:2px solid var(--blue);outline-offset:2px}@media(max-width:850px){.history-analytics{padding:10px}.history-analytics table{min-width:750px}.history-chart{min-height:140px}}
</style>
