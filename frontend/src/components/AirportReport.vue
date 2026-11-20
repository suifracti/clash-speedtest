<script setup lang="ts">
import {computed,nextTick,onMounted,ref,watch} from 'vue'
import {api} from '../api'
import {useWorkspace} from '../workspace'
import {bytes,date,isNotice,key,type Settings} from '../domain'
import {nodeDisplayName,nodeRegionLabel} from '../nodePresentation'
import {airportBeats,airportFaults,beatTone,type AirportBeat} from '../airportPulse'
import type {QualityEvidence} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {airportReport,airportNodeStats,airportExitSummary,filterAirportNodes} from '../airportReport'
import {readUsage,shanghaiDay,shiftDay,type UsageReport} from '../usageDaily'
import {hideHoverTip,showHoverTip} from '../hoverTip'
import TrendChart from './TrendChart.vue'
import UiSelect from './UiSelect.vue'
const props=defineProps<{initialAirportId?:string}>(),emit=defineEmits<{'airport-change':[id:string]}>()
const w=useWorkspace(),airport=ref(props.initialAirportId||w.airports.value[0]?.id||''),data=ref<Record<string,QualityEvidence>>({}),usage=ref<UsageReport|null>(null),settings=ref<Settings|null>(null),price=ref(0),savingPrice=ref(false),busy=ref(false),error=ref(''),selectedBeat=ref<AirportBeat|null>(null)
const region=ref('all'),line=ref('all'),protocol=ref('all')
const airportNodes=computed(()=>w.nodes.value.filter(n=>!isNotice(n)&&w.nodeAirportId(n)===airport.value))
const nodes=computed(()=>filterAirportNodes(airportNodes.value,region.value,line.value,protocol.value))
const filtered=computed(()=>region.value!=='all'||line.value!=='all'||protocol.value!=='all')
const regions=computed(()=>[...new Set(airportNodes.value.map(n=>n.country_code).filter(Boolean))].sort())
const protocols=computed(()=>[...new Set(airportNodes.value.map(n=>n.type))].sort())
const report=computed(()=>airportReport(nodes.value,data.value))
const exitSummary=computed(()=>airportExitSummary(nodes.value,data.value))
const rankSearch=ref(''),onlyProblems=ref(false)
type RankField='responseRate'|'median'|'volatility'|'lastFailure'
const rankField=ref<RankField>('responseRate'),rankDescending=ref(true)
const rankColumns:{id:RankField;label:string}[]=[{id:'responseRate',label:'响应率'},{id:'median',label:'稳定延迟中位数'},{id:'volatility',label:'波动（P90−P10）'},{id:'lastFailure',label:'最近一次失败'}]
function sortRank(field:RankField){if(rankField.value===field)rankDescending.value=!rankDescending.value;else{rankField.value=field;rankDescending.value=field==='responseRate'||field==='lastFailure'}}
const ranks=computed(()=>{
 const query=rankSearch.value.trim().toLowerCase()
 return nodes.value.map(node=>({node,...airportNodeStats(data.value[key(node)])})).filter(row=>{
  if(onlyProblems.value&&!((row.responseRate!==null&&row.responseRate<90)||!!row.lastFailure))return false
  if(!query)return true
  const hay=[nodeDisplayName(row.node),row.node.display_name,w.nodeSubscriptionName(row.node),row.node.country_code,nodeRegionLabel(row.node.country_code)].join(' ').toLowerCase()
  return hay.includes(query)
 }).sort((a,b)=>{
  const value=(r:typeof a)=>rankField.value==='lastFailure'?(r.lastFailure?Date.parse(r.lastFailure.time):null):r[rankField.value]
  const compare=(av:number|null,bv:number|null,descending:boolean)=>av===null?(bv===null?0:1):bv===null?-1:(av-bv)*(descending?-1:1)
  return Number(a.total<3)-Number(b.total<3)||compare(value(a),value(b),rankDescending.value)||(rankField.value==='responseRate'?compare(a.median,b.median,false):0)
 })
})
async function inspectNode(node:typeof nodes.value[number]){w.page.value='home';await nextTick();w.viewRequest.value=node}
const ms=(value:number|null)=>value===null?'—':Math.round(value)+' ms'
const responseTone=(value:number|null)=>value===null?'':value<80?'bad':value<=95?'warn':'good'
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
watch(airport,()=>{emit('airport-change',airport.value);region.value='all';line.value='all';protocol.value='all';void refresh()})
let generation=0
async function refresh(){
 const token=++generation,selectedAirport=airport.value,requestedNodes=airportNodes.value.slice()
 busy.value=true;error.value='';selectedBeat.value=null;data.value={};usage.value=null
 const until=new Date().toISOString(),since=new Date(Date.parse(until)-30*86400000).toISOString(),today=shanghaiDay()
 try{
  const current=await api.settings()
  if(token!==generation)return
  const [evidence,u]=await Promise.all([readQualityBatch(requestedNodes,since,until),readUsage({from:shiftDay(today,-29),until:today,period:'day'},0)])
  if(token!==generation)return
  settings.value=current;price.value=current.airport_monthly_prices?.[selectedAirport]||0;data.value=evidence;usage.value=u
 }catch(e){if(token===generation)error.value=e instanceof Error?e.message:String(e)}
 finally{if(token===generation)busy.value=false}
}
async function savePrice(){
 const selectedAirport=airport.value,monthlyPrice=price.value,name=w.airports.value.find(a=>a.id===selectedAirport)?.name||'该机场'
 if(savingPrice.value||!selectedAirport)return
 if(!Number.isFinite(monthlyPrice)||monthlyPrice<0){w.fail(new Error('月费须为不小于 0 的数字'));return}
 savingPrice.value=true
 try{
  const current=await api.settings(),next={...current,airport_monthly_prices:{...current.airport_monthly_prices,[selectedAirport]:monthlyPrice}}
  await api.saveSettings(next)
  if(airport.value===selectedAirport)settings.value=next
  w.notify(name+'的月费已保存到本机设置')
 }catch(e){w.fail(e)}finally{savingPrice.value=false}
}
onMounted(()=>void refresh())
</script>
<template>
 <section class="settings-section airport-health">
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
   <div class="health-analysis">
   <aside class="report-metrics" aria-label="体检摘要与成本">
    <div><small>响应率</small><strong>{{report.rate===null?'—':report.rate.toFixed(0)+'%'}} <small>（{{report.responses}}/{{report.samples}} 次）</small></strong><small>{{report.total}} 轮中 {{report.failedRounds}} 轮有失败</small></div>
    <div v-if="!filtered"><small>近 30 日已观测用量</small><strong>{{traffic===null?'未知':bytes(traffic)}}</strong><small>缺失时段未补零</small></div>
    <div v-if="!filtered"><small>每 GB 成本估算</small><strong>{{cost===null?'—':'¥'+cost.toFixed(2)}}</strong><small>月费 ÷ 已观测 GB，记录不足时会偏高</small><div class="report-price"><label>月费（元）<input v-model.number="price" type="number" min="0" step="0.01" aria-label="机场月费"></label><button class="button" :disabled="!airport||savingPrice" @click="savePrice">{{savingPrice?'保存中…':'保存月费'}}</button></div></div>
    <div v-if="exitSummary.purityCount||exitSummary.cfCount"><small>出口纯净度与 CF 盾（已保存）</small><strong v-if="exitSummary.purityAvg!==null">均分 {{exitSummary.purityAvg}}<small>（{{exitSummary.purityClean}} 干净 / {{exitSummary.purityCount}} 已测）</small></strong><strong v-else>CF {{exitSummary.cfGood}}/{{exitSummary.cfCount}} 直通</strong><small v-if="exitSummary.cfCount">CF 盾：{{exitSummary.cfGood}} 直通 · {{exitSummary.cfWarn}} 部分 · {{exitSummary.cfBad}} 易弹</small><small v-else>尚未保存 Cloudflare 人机验证记录</small></div>
   </aside>
   <div class="health-observations">
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
   </div>
   </div>
  </template>
  <section v-if="!busy" class="report-rank">
   <div class="rank-head">
    <h3>节点排行</h3>
    <div class="rank-controls">
     <input v-model="rankSearch" type="search" class="rank-search" placeholder="筛选节点名、地区或订阅" aria-label="筛选排行节点">
     <button type="button" class="rank-filter-pill" :aria-pressed="onlyProblems" @click="onlyProblems=!onlyProblems">仅看有问题</button>
    </div>
   </div>
   <div class="rank-table-wrap"><table><thead><tr><th>节点</th><th v-for="col in rankColumns.slice(0,1)" :key="col.id" :aria-sort="rankField===col.id?(rankDescending?'descending':'ascending'):'none'"><button class="text-button" @click="sortRank(col.id)">{{col.label}}{{rankField===col.id?(rankDescending?' ↓':' ↑'):''}}</button></th><th>轮数</th><th v-for="col in rankColumns.slice(1)" :key="col.id" :aria-sort="rankField===col.id?(rankDescending?'descending':'ascending'):'none'"><button class="text-button" @click="sortRank(col.id)">{{col.label}}{{rankField===col.id?(rankDescending?' ↓':' ↑'):''}}</button></th></tr></thead>
    <tbody><tr v-for="row in ranks" :key="key(row.node)"><td><button class="text-button rank-node" @click="inspectNode(row.node)">{{nodeDisplayName(row.node)}}</button><small v-if="w.nodeSubscriptionName(row.node)" class="rank-account">{{w.nodeSubscriptionName(row.node)}}</small></td><td :class="responseTone(row.responseRate)" :title="`${row.responses}/${row.samples} 次响应`">{{row.responseRate===null?'—':row.responseRate.toFixed(0)+'%'}}<small v-if="row.total<3" class="rank-low">轮数少</small></td><td>{{row.total}}</td><td>{{ms(row.median)}}</td><td title="慢的时候比快的时候多多少（P90−P10）；至少5轮有稳定延迟后显示">{{ms(row.volatility)}}</td><td :title="row.lastFailure?.detail">{{row.lastFailure?date(row.lastFailure.time):'—'}}</td></tr></tbody>
   </table></div>
   <p v-if="!ranks.length" class="muted small">当前筛选范围没有节点。</p>
  </section>
 </section>
</template>
<style scoped>
.rank-low{margin-left:6px;color:var(--cst-muted);font-size:11px;font-weight:500;padding:1px 5px;border-radius:4px;background:var(--cst-panel-2)}
.report-filters{display:flex;align-items:end;gap:12px;flex-wrap:wrap;margin:14px 0;padding:12px 14px;background:var(--cst-panel-2);border:1px solid var(--cst-line);border-radius:var(--cst-radius-sm)}.report-filters label{display:flex;flex-direction:column;gap:4px;font-size:12px;font-weight:550;color:var(--cst-muted)}.report-filters .small{margin-bottom:8px;margin-left:auto;font-weight:550}.report-rank{margin-top:22px;padding-top:18px;border-top:1px solid var(--cst-line)}.rank-table-wrap{overflow:auto;border:1px solid var(--cst-line);border-radius:var(--cst-radius-sm);background:var(--cst-panel);margin-top:10px}.report-rank table{width:100%;border-collapse:collapse;font-size:12.5px}.report-rank th,.report-rank td{text-align:left;padding:10px 12px;border-bottom:1px solid var(--cst-line)}.report-rank th{white-space:nowrap;background:var(--cst-panel-2);font-weight:650;color:var(--cst-muted)}.report-rank tbody tr:hover{background:color-mix(in srgb,var(--cst-panel-2) 65%,var(--cst-panel))}.report-rank td:not(:first-child){white-space:nowrap;font-variant-numeric:tabular-nums}.rank-node{white-space:normal;text-align:left;font-weight:600}
.report-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px;margin:16px 0}.report-metrics>div{display:flex;flex-direction:column;gap:6px;padding:14px 16px;background:var(--cst-panel-2);border:1px solid var(--cst-line);border-radius:var(--cst-radius-sm)}.report-metrics strong{font-size:22px;font-weight:700;letter-spacing:-.02em;font-variant-numeric:tabular-nums}.report-metrics strong small{font-size:12px;font-weight:450;color:var(--cst-muted)}.report-metrics small{color:var(--cst-muted);font-size:11.5px}.report-price{display:flex;align-items:end;gap:8px;margin-top:8px;padding-top:8px;border-top:1px dashed var(--cst-line)}.report-price label{font-size:12px;color:var(--cst-muted)}.report-price input{width:100px;display:block;margin-top:4px;min-height:32px;padding:4px 8px;border:1px solid var(--cst-line-strong);border-radius:7px;background:var(--cst-panel);color:var(--cst-text)}.report-empty{display:flex;justify-content:space-between;gap:16px;flex-wrap:wrap;margin:16px 0;padding:18px;border:1px dashed var(--cst-line-strong);border-radius:var(--cst-radius-sm);background:var(--cst-panel-2)}.airport-beats{display:flex;gap:3px;height:22px;margin:10px 0 8px;padding:3px;border-radius:7px;background:var(--cst-panel-2);border:1px solid var(--cst-line)}.beat-legend{margin:0 0 14px;font-size:11px;line-height:1.5;color:var(--cst-muted)}.beat-cell{flex:1 1 0;min-width:3px;padding:0;border:0;border-radius:3px;background:var(--cst-empty-bg);cursor:pointer;transition:transform .12s}.beat-cell:hover{transform:scaleY(1.12)}.beat-cell.good{background:var(--cst-good)}.beat-cell.warn{background:var(--cst-warn)}.beat-cell.bad{background:var(--cst-bad)}.beat-cell.selected{outline:2px solid var(--cst-accent);outline-offset:2px}.beat-detail{margin:0 0 14px;padding:8px 12px;border-radius:7px;background:var(--cst-accent-soft);color:var(--cst-accent);font-size:12px;font-weight:550;line-height:1.5}.fault-list{display:flex;flex-direction:column;margin:8px 0 16px;border:1px solid var(--cst-line);border-radius:var(--cst-radius-sm);overflow:hidden}.fault-row{display:grid;grid-template-columns:88px 164px 70px minmax(0,1fr);gap:10px;align-items:center;padding:9px 12px;border-top:1px solid var(--cst-line);font-size:12px;background:var(--cst-panel)}.fault-row:first-child{border-top:0}.fault-kind{display:inline-block;padding:2px 7px;border-radius:5px;font-size:11px;font-weight:600;text-align:center;white-space:nowrap}.fault-kind.round{background:var(--cst-bad-bg);color:var(--cst-bad)}.fault-kind.node{background:var(--cst-warn-bg);color:var(--cst-warn)}.fault-time{color:var(--cst-muted);white-space:nowrap;font-variant-numeric:tabular-nums}.fault-row strong{font-weight:650;font-variant-numeric:tabular-nums}.fault-row small{color:var(--cst-muted);line-height:1.5}@media(max-width:800px){.report-metrics{grid-template-columns:1fr}.fault-row{grid-template-columns:76px minmax(0,1fr) 56px}.fault-row small{grid-column:1/-1}}
.airport-health.settings-section{padding:20px 22px;border:1px solid var(--cst-line);border-radius:14px;background:var(--cst-panel);box-shadow:var(--cst-shadow)}
.airport-health>.section-heading{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap}.airport-health>.section-heading h2{font-size:20px}.airport-health>.section-heading p{margin:6px 0 0}
.airport-health .report-filters{background:var(--cst-panel-2);border:1px solid var(--cst-line);border-radius:12px;padding:12px 14px;margin:14px 0 20px}
.health-analysis{display:grid;grid-template-columns:256px minmax(0,1fr);gap:22px;align-items:start}
.health-analysis .report-metrics{display:flex;flex-direction:column;gap:12px;margin:0;border-right:0;padding-right:0;position:sticky;top:calc(var(--cst-nav-height,58px) + 20px)}
.health-analysis .report-metrics>div{padding:14px 16px;border:1px solid var(--cst-line);border-radius:12px;background:var(--cst-panel-2);gap:6px}
.health-analysis .report-metrics strong{font-size:24px;font-weight:650}
.health-analysis .report-metrics strong small{display:block;margin-top:4px}.health-analysis .report-price{flex-wrap:wrap;gap:8px;border-top:1px dashed var(--cst-line);padding-top:8px}
.health-observations{min-width:0;padding:16px 18px;border:1px solid var(--cst-line);border-radius:12px;background:var(--cst-panel-2)}.health-observations h3{margin:20px 0 7px;font-size:14px}.health-observations h3:first-child{margin-top:0}.health-observations p{line-height:1.7}
.health-observations .airport-beats{padding:4px;border:1px solid var(--cst-line);border-radius:8px;background:var(--cst-panel);height:28px}.health-observations .beat-cell{max-width:36px;border-radius:3px}
.airport-health .report-rank{margin-top:24px;padding-top:18px}.rank-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}.rank-head h3{margin:0}.rank-controls{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.rank-search{min-width:190px;min-height:32px;padding:4px 10px;border:1px solid var(--cst-line-strong);border-radius:9px;background:var(--cst-panel-2);color:var(--cst-text);font-size:12.5px}.rank-filter-pill{min-height:32px;padding:0 12px;border:1px solid var(--cst-line-strong);border-radius:999px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:12px;font-weight:600;cursor:pointer}.rank-filter-pill[aria-pressed=true]{background:color-mix(in srgb,var(--cst-warn,#b45309) 12%,var(--cst-panel));border-color:color-mix(in srgb,var(--cst-warn,#b45309) 45%,transparent);color:var(--cst-warn,#b45309)}.airport-health .rank-table-wrap{border-radius:12px}.rank-account{display:block;font-size:11px;color:var(--cst-muted);margin-top:5px}.airport-health .rank-node{justify-content:flex-start}
@media(max-width:1000px){.health-analysis{grid-template-columns:minmax(0,1fr);gap:18px}.health-analysis .report-metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px;position:static}.health-analysis .report-metrics strong{font-size:22px}}
@media(max-width:680px){.health-analysis .report-metrics{grid-template-columns:minmax(0,1fr)}.health-analysis .report-price{flex-wrap:nowrap}}
</style>
