<script setup lang="ts">
import {computed,nextTick,onMounted,ref,watch} from 'vue'
import {api} from '../api'
import {useWorkspace} from '../workspace'
import {bytes,date,isNotice,key,type Settings} from '../domain'
import {nodeDisplayName,nodeRegionLabel} from '../nodePresentation'
import {airportBeats,airportFaults,beatTone,type AirportBeat} from '../airportPulse'
import type {QualityEvidence} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {airportReport,airportNodeStats,filterAirportNodes} from '../airportReport'
import {readUsage,shanghaiDay,shiftDay,type UsageReport} from '../usageDaily'
import {hideHoverTip,showHoverTip} from '../hoverTip'
import TrendChart from './TrendChart.vue'
import UiSelect from './UiSelect.vue'
const w=useWorkspace(),airport=ref(w.airports.value[0]?.id||''),data=ref<Record<string,QualityEvidence>>({}),usage=ref<UsageReport|null>(null),settings=ref<Settings|null>(null),price=ref(0),busy=ref(false),error=ref(''),selectedBeat=ref<AirportBeat|null>(null)
const region=ref('all'),line=ref('all'),protocol=ref('all')
const airportNodes=computed(()=>w.nodes.value.filter(n=>!isNotice(n)&&w.nodeAirportId(n)===airport.value))
const nodes=computed(()=>filterAirportNodes(airportNodes.value,region.value,line.value,protocol.value))
const filtered=computed(()=>region.value!=='all'||line.value!=='all'||protocol.value!=='all')
const regions=computed(()=>[...new Set(airportNodes.value.map(n=>n.country_code).filter(Boolean))].sort())
const protocols=computed(()=>[...new Set(airportNodes.value.map(n=>n.type))].sort())
const report=computed(()=>airportReport(nodes.value,data.value))
type RankField='responseRate'|'median'|'volatility'|'lastFailure'
const rankField=ref<RankField>('median'),rankDescending=ref(false)
const rankColumns:{id:RankField;label:string}[]=[{id:'responseRate',label:'响应率'},{id:'median',label:'稳定延迟中位数'},{id:'volatility',label:'波动（P90−P10）'},{id:'lastFailure',label:'最近一次失败'}]
function sortRank(field:RankField){if(rankField.value===field)rankDescending.value=!rankDescending.value;else{rankField.value=field;rankDescending.value=field==='responseRate'||field==='lastFailure'}}
const ranks=computed(()=>nodes.value.map(node=>({node,...airportNodeStats(data.value[key(node)])})).sort((a,b)=>{
 const value=(r:typeof a)=>rankField.value==='lastFailure'?(r.lastFailure?Date.parse(r.lastFailure.time):null):r[rankField.value]
 const av=value(a),bv=value(b);return av===null?(bv===null?0:1):bv===null?-1:(av-bv)*(rankDescending.value?-1:1)
}))
async function inspectNode(node:typeof nodes.value[number]){w.page.value='home';await nextTick();w.viewRequest.value=node}
const ms=(value:number|null)=>value===null?'—':Math.round(value)+' ms'
watch([region,line,protocol],()=>{selectedBeat.value=null;hideHoverTip()})
// 心跳和故障都用已读到的历史记录拼：轮次是同一段时间内保存的节点测试，
// 单节点检查不算一轮；故障事件也只看这些轮次。
const pulse=computed(()=>{
 const beats=airportBeats(nodes.value.map(n=>({key:key(n),name:nodeDisplayName(n),tests:data.value[key(n)]?.tests||[]})))
 return {beats:beats.slice(-60),faults:airportFaults(beats)}
})
function beatTitle(b:AirportBeat){const failed=b.nodes.filter(n=>!n.responded).map(n=>n.name),shown=failed.slice(0,8).join('、')+(failed.length>8?` 等 ${failed.length} 个`:'');return date(b.start)+' · '+b.responded+'/'+b.total+' 个节点有响应 · '+(failed.length?'失败：'+shown:'全部节点有响应')}
function selectBeat(b:AirportBeat){selectedBeat.value=b;hideHoverTip()}
const traffic=computed(()=>{const accounts=new Set(usage.value?.accounts.filter(a=>a.airport_id===airport.value).map(a=>a.account_key));const values=usage.value?.intervals.filter(i=>accounts.has(i.account_key)&&i.bytes!==null).map(i=>i.bytes!)||[];return values.length?values.reduce((a,b)=>a+b,0):null})
const cost=computed(()=>price.value>0&&traffic.value!==null&&traffic.value>0?price.value/(traffic.value/1e9):null)
watch(airport,()=>{region.value='all';line.value='all';protocol.value='all';void refresh()})
async function refresh(){busy.value=true;error.value='';selectedBeat.value=null;data.value={};usage.value=null;const until=new Date().toISOString(),since=new Date(Date.parse(until)-30*86400000).toISOString(),today=shanghaiDay();try{settings.value=await api.settings();price.value=settings.value.airport_monthly_prices?.[airport.value]||0;const [evidence,u]=await Promise.all([readQualityBatch(airportNodes.value,since,until),readUsage({from:shiftDay(today,-29),until:today,period:'day'},0)]);data.value=evidence;usage.value=u}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
async function savePrice(){try{const current=await api.settings();current.airport_monthly_prices={...current.airport_monthly_prices,[airport.value]:price.value};await api.saveSettings(current);settings.value=current;w.notify('月费已保存到本机设置')}catch(e){w.fail(e)}}
onMounted(()=>void refresh())
</script>
<template>
 <section class="settings-section">
  <div class="section-heading"><div><h2>机场体检 · 近 30 天</h2><p class="muted small">同一订阅节点含历史配置的已保存记录。</p></div><div class="button-group"><UiSelect v-model="airport" aria-label="体检机场"><option v-for="a in w.airports.value" :key="a.id" :value="a.id">{{a.name}}</option></UiSelect><button class="button" :disabled="busy" @click="refresh">{{busy?'读取中…':'刷新报告'}}</button></div></div>
  <div class="report-filters">
   <label>地区<UiSelect v-model="region" aria-label="体检地区"><option value="all">全部地区</option><option v-for="code in regions" :key="code" :value="code">{{nodeRegionLabel(code)}}</option></UiSelect></label>
   <label>线路<UiSelect v-model="line" aria-label="体检线路"><option value="all">全部线路</option><option v-for="name in ['专线','高速','流媒体']" :key="name" :value="name">{{name}}</option></UiSelect></label>
   <label>协议<UiSelect v-model="protocol" aria-label="体检协议"><option value="all">全部协议</option><option v-for="name in protocols" :key="name" :value="name">{{name}}</option></UiSelect></label>
   <span class="muted small">{{nodes.length}} / {{airportNodes.length}} 个节点</span>
  </div>
  <p v-if="filtered" class="muted small">账户用量无法按节点筛选拆分；清除筛选后查看用量和成本。</p>
  <p v-if="error" class="inline-error" role="alert">{{error}}</p><p v-if="report.incomplete" class="muted small">部分节点历史未读完整，以下仅统计已读取记录。</p>
  <p v-if="busy" class="muted" role="status">正在读取历史…</p>
  <p v-else-if="!report.total" class="report-empty muted">{{filtered?'当前范围':'这个机场'}}近 30 天没有检测记录<span v-if="!filtered">已观测用量 {{traffic===null?'未知':bytes(traffic)}}</span></p>
  <template v-else>
   <div class="report-metrics">
    <div><small>响应率</small><strong>{{report.rate===null?'—':report.rate.toFixed(0)+'%'}} <small>（{{report.responses}}/{{report.samples}} 次）</small></strong><small>{{report.total}} 轮中 {{report.failedRounds}} 轮有失败</small></div>
    <div v-if="!filtered"><small>近 30 日已观测用量</small><strong>{{traffic===null?'未知':bytes(traffic)}}</strong><small>缺失时段未补零</small></div>
    <div v-if="!filtered"><small>每 GB 成本估算</small><strong>{{cost===null?'—':'¥'+cost.toFixed(2)}}</strong><small>月费 ÷ 已观测 GB，记录不足时会偏高</small><div class="report-price"><label>月费（元）<input v-model.number="price" type="number" min="0" step="0.01" aria-label="机场月费"></label><button class="button" :disabled="!airport" @click="savePrice">保存月费</button></div></div>
   </div>
   <h3>平均响应延迟走势</h3><p class="muted small">按天汇总有响应样本，站点和检测条件可能不同。</p><TrendChart :points="report.trend" unit="ms" label="近30日平均响应延迟"/>
   <h3>节点心跳</h3><p class="muted small">同一轮里先后保存的节点测试算一轮（间隔不超过 10 分钟，至少 2 个节点）；每格一轮，颜色是这轮有响应的节点比例。</p>
   <template v-if="pulse.beats.length">
    <div class="airport-beats" role="group" :aria-label="'近 30 天 '+pulse.beats.length+' 轮节点响应比例'"><button v-for="b in pulse.beats" :key="b.id" type="button" class="beat-cell" :class="[beatTone(b),{selected:selectedBeat?.id===b.id}]" :aria-label="beatTitle(b)" @mouseenter="showHoverTip($event,beatTitle(b))" @mouseleave="hideHoverTip" @click="selectBeat(b)"/></div>
    <p class="beat-legend">绿＝≥90% 节点有响应 · 橙＝50%–90% · 红＝<50%</p>
    <p v-if="selectedBeat" class="beat-detail">{{beatTitle(selectedBeat)}} <button class="text-button" @click="selectedBeat=null">收起</button></p>
   </template>
   <p v-else class="muted small">近 30 天没有成批的检测轮次，心跳条留空。</p>
   <h3>故障记录</h3><p class="muted small">只记两类：某一轮超过一半节点无响应；单个节点连续 3 轮无响应。</p>
   <p v-if="!pulse.faults.length" class="muted">已读取的记录里没有这两类故障。</p>
   <div v-else class="fault-list"><div v-for="(f,index) in pulse.faults" :key="index" class="fault-row"><span class="fault-kind" :class="f.kind">{{f.kind==='round'?'整轮故障':'节点连续离线'}}</span><span class="fault-time">{{date(f.start)}} – {{f.end?date(f.end):'仍在持续'}}</span><strong>{{f.nodes}} 个节点</strong><small>{{f.detail}}<template v-if="f.names.length"> · {{f.names.slice(0,3).join('、')}}{{f.names.length>3?' 等':''}}</template></small></div></div>
   <h3>失败类型</h3><p v-if="!report.failures.length" class="muted">已读取记录没有失败样本</p><div v-for="[kind,count] in report.failures" :key="kind" class="compact-row"><span>{{kind}}</span><strong>{{count}} 次</strong></div>
  </template>
  <section v-if="!busy" class="report-rank">
   <h3>节点排行</h3>
   <div class="rank-table-wrap"><table><thead><tr><th>节点</th><th v-for="col in rankColumns" :key="col.id" :aria-sort="rankField===col.id?(rankDescending?'descending':'ascending'):'none'"><button class="text-button" @click="sortRank(col.id)">{{col.label}}{{rankField===col.id?(rankDescending?' ↓':' ↑'):''}}</button></th></tr></thead>
    <tbody><tr v-for="row in ranks" :key="key(row.node)"><td><button class="text-button rank-node" @click="inspectNode(row.node)">{{nodeDisplayName(row.node)}}</button></td><td :title="`${row.responses}/${row.samples} 次响应 · 共 ${row.total} 轮`">{{row.responseRate===null?'—':row.responseRate.toFixed(0)+'%'}}</td><td>{{ms(row.median)}}</td><td title="慢的时候比快的时候多多少（P90−P10）；满5轮后显示">{{ms(row.volatility)}}</td><td :title="row.lastFailure?.detail">{{row.lastFailure?date(row.lastFailure.time):'—'}}</td></tr></tbody>
   </table></div>
   <p v-if="!ranks.length" class="muted small">当前筛选范围没有节点。</p>
  </section>
 </section>
</template>
<style scoped>
.report-filters{display:flex;align-items:end;gap:12px;flex-wrap:wrap;margin:14px 0}.report-filters label{display:flex;flex-direction:column;gap:4px;font-size:12px;color:var(--cst-muted)}.report-filters .small{margin-bottom:8px}.report-rank{margin-top:22px}.rank-table-wrap{overflow:auto}.report-rank table{width:100%;border-collapse:collapse;font-size:12px;margin-top:10px}.report-rank th,.report-rank td{text-align:left;padding:9px 10px;border-bottom:1px solid var(--cst-line)}.report-rank th{white-space:nowrap}.report-rank td:not(:first-child){white-space:nowrap;font-variant-numeric:tabular-nums}.rank-node{white-space:normal;text-align:left}
.report-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;margin:16px 0}.report-metrics>div{display:flex;flex-direction:column;gap:5px}.report-metrics strong{font-size:22px}.report-metrics strong small{font-size:12px;font-weight:400}.report-metrics small{color:var(--cst-muted)}.report-price{display:flex;align-items:end;gap:8px;margin-top:10px}.report-price label{font-size:12px}.report-price input{width:100px;display:block;margin-top:5px}.report-empty{display:flex;justify-content:space-between;gap:16px;flex-wrap:wrap;margin:16px 0}.airport-beats{display:flex;gap:2px;height:18px;margin:8px 0 6px}.beat-legend{margin:0 0 14px;font-size:10px;line-height:1.5;color:var(--muted)}.beat-cell{flex:1 1 0;min-width:2px;padding:0;border:0;border-radius:2px;background:var(--surface-soft);cursor:pointer}.beat-cell.good{background:var(--green)}.beat-cell.warn{background:var(--amber)}.beat-cell.bad{background:var(--red)}.beat-cell.selected{outline:2px solid var(--blue);outline-offset:2px}.beat-detail{margin:0 0 14px;font-size:11px;line-height:1.5;color:var(--muted)}.fault-list{display:flex;flex-direction:column;margin:6px 0 16px}.fault-row{display:grid;grid-template-columns:80px 156px 62px minmax(0,1fr);gap:8px;align-items:baseline;padding:7px 0;border-top:1px solid var(--line);font-size:11px}.fault-kind{display:inline-block;padding:1px 5px;border-radius:4px;font-size:10px;text-align:center;white-space:nowrap}.fault-kind.round{background:var(--red-soft);color:var(--red)}.fault-kind.node{background:var(--amber-soft);color:var(--amber)}.fault-time{color:var(--muted);white-space:nowrap;font-variant-numeric:tabular-nums}.fault-row strong{font-weight:600;font-variant-numeric:tabular-nums}.fault-row small{color:var(--muted);line-height:1.5}@media(max-width:800px){.report-metrics{grid-template-columns:1fr}.fault-row{grid-template-columns:76px minmax(0,1fr) 56px}.fault-row small{grid-column:1/-1}}</style>
