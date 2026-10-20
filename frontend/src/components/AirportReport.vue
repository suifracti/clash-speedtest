<script setup lang="ts">
import {computed,onMounted,ref,watch} from 'vue'
import {api} from '../api'
import {useWorkspace} from '../workspace'
import {bytes,isNotice,type Settings} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {airportReport} from '../airportReport'
import {readUsage,shanghaiDay,shiftDay,type UsageReport} from '../usageDaily'
import TrendChart from './TrendChart.vue'
import UiSelect from './UiSelect.vue'
const w=useWorkspace(),airport=ref(w.airports.value[0]?.id||''),data=ref<Record<string,QualityEvidence>>({}),usage=ref<UsageReport|null>(null),settings=ref<Settings|null>(null),price=ref(0),busy=ref(false),error=ref('')
const nodes=computed(()=>w.nodes.value.filter(n=>!isNotice(n)&&w.nodeAirportId(n)===airport.value)),report=computed(()=>airportReport(nodes.value,data.value))
const traffic=computed(()=>{const accounts=new Set(usage.value?.accounts.filter(a=>a.airport_id===airport.value).map(a=>a.account_key));const values=usage.value?.intervals.filter(i=>accounts.has(i.account_key)&&i.bytes!==null).map(i=>i.bytes!)||[];return values.length?values.reduce((a,b)=>a+b,0):null})
const cost=computed(()=>price.value>0&&traffic.value!==null&&traffic.value>0?price.value/(traffic.value/1e9):null)
watch(airport,()=>void refresh())
async function refresh(){busy.value=true;error.value='';data.value={};usage.value=null;const until=new Date().toISOString(),since=new Date(Date.parse(until)-30*86400000).toISOString(),today=shanghaiDay();try{settings.value=await api.settings();price.value=settings.value.airport_monthly_prices?.[airport.value]||0;const [evidence,u]=await Promise.all([readQualityBatch(nodes.value,since,until),readUsage({from:shiftDay(today,-29),until:today,period:'day'},0)]);data.value=evidence;usage.value=u}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
async function savePrice(){try{const current=await api.settings();current.airport_monthly_prices={...current.airport_monthly_prices,[airport.value]:price.value};await api.saveSettings(current);settings.value=current;w.notify('月费已保存到本机设置')}catch(e){w.fail(e)}}
onMounted(()=>void refresh())
</script>
<template>
 <section class="settings-section">
  <div class="section-heading"><div><h2>机场体检 · 近 30 天</h2><p class="muted small">同一订阅节点含历史配置的已保存记录；按节点检测轮次统计，一轮任一采样有响应即记可用。</p></div><div class="button-group"><UiSelect v-model="airport" aria-label="体检机场"><option v-for="a in w.airports.value" :key="a.id" :value="a.id">{{a.name}}</option></UiSelect><button class="button" :disabled="busy" @click="refresh">{{busy?'读取中…':'刷新报告'}}</button></div></div>
  <p v-if="error" class="inline-error" role="alert">{{error}}</p><p v-if="report.incomplete" class="muted small">部分节点历史未读完整，以下仅统计已读取记录。</p>
  <p v-if="busy" class="muted" role="status">正在读取历史…</p>
  <p v-else-if="!report.total" class="report-empty muted">这个机场近 30 天没有检测记录<span>已观测用量 {{traffic===null?'未知':bytes(traffic)}}</span></p>
  <template v-else>
   <div class="report-metrics">
    <div><small>可用率（节点轮次）</small><strong>{{report.rate!.toFixed(1)}}%</strong><small>{{report.available}} / {{report.total}} 轮有响应</small></div>
    <div><small>近 30 日已观测用量</small><strong>{{traffic===null?'未知':bytes(traffic)}}</strong><small>缺失时段未补零</small></div>
    <div><small>每 GB 成本估算</small><strong>{{cost===null?'—':'¥'+cost.toFixed(2)}}</strong><small>月费 ÷ 已观测 GB，记录不足时会偏高</small><div class="report-price"><label>月费（元）<input v-model.number="price" type="number" min="0" step="0.01" aria-label="机场月费"></label><button class="button" :disabled="!airport" @click="savePrice">保存月费</button></div></div>
   </div>
   <h3>平均响应延迟走势</h3><p class="muted small">按天汇总有响应样本，站点和检测条件可能不同。</p><TrendChart :points="report.trend" unit="ms" label="近30日平均响应延迟"/>
   <h3>失败类型</h3><p v-if="!report.failures.length" class="muted">已读取记录没有失败样本</p><div v-for="[kind,count] in report.failures" :key="kind" class="compact-row"><span>{{kind}}</span><strong>{{count}} 次</strong></div>
  </template>
 </section>
</template>
<style scoped>.report-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;margin:16px 0}.report-metrics>div{display:flex;flex-direction:column;gap:5px}.report-metrics strong{font-size:22px}.report-metrics small{color:var(--cst-muted)}.report-price{display:flex;align-items:end;gap:8px;margin-top:10px}.report-price label{font-size:12px}.report-price input{width:100px;display:block;margin-top:5px}.report-empty{display:flex;justify-content:space-between;gap:16px;flex-wrap:wrap;margin:16px 0}@media(max-width:800px){.report-metrics{grid-template-columns:1fr}}</style>
