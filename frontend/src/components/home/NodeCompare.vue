<script setup lang="ts">
import {computed,inject,onMounted,ref} from 'vue'
import {useWorkspace} from '../../workspace'
import {date,key,type NodeOption} from '../../domain'
import {latencyTargets} from '../../latencyTargets'
import {latestServices,qualityGroups} from '../../nodeQuality'
import {nodeDisplayName} from '../../nodePresentation'
import QualityMetrics from '../QualityMetrics.vue'
import UiSelect from '../UiSelect.vue'
import Icon from '../Icon.vue'
import {homeModelKey} from './homeModel'
const w=useWorkspace(),m=inject(homeModelKey)!
const section=ref<'all'|'latency'|'download'|'service'>('all')
const sections=[{id:'all',label:'全部指标'},{id:'latency',label:'延迟'},{id:'download',label:'下载'},{id:'service',label:'服务与出口'}] as const
const services=computed(()=>Object.fromEntries(m.compareNodes.value.map(n=>[key(n),latestServices(m.qualityData.value[key(n)]?.services||[])])))
type Field='distribution'|'response'|'download'|'service'|'time'|'details'
interface Row {id:string;field:Field;label:string;serviceId?:string}
const serviceRows=computed<Row[]>(()=>{
 const names=new Map<string,string>()
 for(const attempts of Object.values(services.value))for(const a of attempts){const id=a.service_id||a.rule.service_id!;names.set(id,w.catalog.value.find(s=>s.service_id===id)?.name||a.rule.name||id)}
 const order=new Map(w.catalog.value.map((s,i)=>[s.service_id,i]))
 return [...names].sort(([a,an],[b,bn])=>(order.get(a)??Infinity)-(order.get(b)??Infinity)||an.localeCompare(bn)).map(([id,label])=>({id:'service:'+id,field:'service',serviceId:id,label}))
})
const rows=computed<Row[]>(()=>[
 ...(section.value==='all'||section.value==='latency'?[{id:'distribution',field:'distribution' as const,label:'延迟分布'},{id:'response',field:'response' as const,label:'成功率 / 样本'}]:[]),
 ...(section.value==='all'||section.value==='download'?[{id:'download',field:'download' as const,label:'下载'}]:[]),
 ...(section.value==='all'||section.value==='service'?serviceRows.value.length?serviceRows.value:[{id:'service',field:'service' as const,label:'服务结论'}]:[]),
 ...(section.value!=='service'?[{id:'time',field:'time' as const,label:'测量时间'}]:[]),{id:'details',field:'details',label:'完整证据'}
])
const root=ref<HTMLElement>()
function history(n:NodeOption){
 const evidence=m.qualityData.value[key(n)],project=section.value==='all'?'latency':section.value
 const source=project==='latency'&&evidence&&qualityGroups(evidence.tests,evidence.monitor,m.compareTarget.value)[0]?.source==='monitor'?'monitor':'manual'
 m.closeComparison()
 w.inspectNode.value={...n,historyTargetId:m.compareTarget.value,historyProject:project,historySource:source,historyUntil:evidence?.until||m.qualityCutoff.value,...(project==='service'?{historyServiceId:''}:{})}
}
function serviceEvidence(n:NodeOption,id:string){const a=services.value[key(n)]?.find(a=>(a.service_id||a.rule.service_id)===id);if(a)m.showServiceEvidence(a)}
onMounted(()=>root.value?.querySelector<HTMLElement>('.hd-compare-head button')?.focus({preventScroll:true}))
</script>
<template>
 <aside ref="root" class="hd-compare" aria-label="节点比较" @keydown.esc.stop="m.closeComparison()">
  <header class="hd-compare-head">
   <div><h2>比较 {{m.compareNodes.value.length}} 个已选节点</h2><p class="hd-muted quality-compare-meta">截止 {{date(m.qualityCutoff.value)}} · 已保存记录</p></div>
   <button class="hd-btn" type="button" @click="m.closeComparison()"><Icon name="close"/>退出比较</button>
  </header>
  <div class="comparison-tools">
   <div class="comparison-sections" role="group" aria-label="比较内容"><button v-for="item in sections" :key="item.id" type="button" :aria-pressed="section===item.id" @click="section=item.id">{{item.label}}</button></div>
   <label v-if="section==='all'||section==='latency'">延迟站点<UiSelect v-model="m.compareTarget.value" aria-label="比较延迟站点"><option v-for="target in latencyTargets" :key="target.id" :value="target.id">{{target.name}}{{['microsoft','firefox'].includes(target.id)?'（历史）':''}}</option></UiSelect></label>
  </div>
  <div class="comparison-verdicts">
   <p v-if="section==='all'||section==='latency'" class="quality-compare-status" :class="m.compareCompatible.value?'good':'warn'"><span>延迟</span>{{m.compareCompatible.value?'最新组条件一致，可比较描述统计。':'不可比较：条件不同、样本不足、未测或读取不完整。'}}</p>
   <p v-if="section==='all'||section==='download'" class="comparison-download-status hd-muted"><span>下载</span>{{m.comparisonDownloadStatus.value.replace(/^下载/,'')}}</p>
   <p v-if="section==='service'" class="hd-muted">每行对应一个服务；点击结论查看该条保存证据。HTTP 响应不等于业务可用。</p>
  </div>
  <details class="quality-compare-boundaries hd-compare-rules"><summary>比较口径与边界</summary><p>延迟按所选站点的最新条件组展示；P95 至少 20 个成功样本，少于 5 个成功样本不排名，缺少出口或地址族/来源证据时只展示原值。下载仅同源、方法、上限、结束类型及已记录的出口地址条件相同时可比较，缺少必要出口证据时不做数值排序；历史有效速度不代表最近检测成功。服务逐项核对同一规则、方法、地址条件和时间，HTTP 响应不等于业务可用。切换比较内容和站点不改变检测范围、勾选和工作台站点。</p></details>
  <p v-if="m.compareNodes.value.length>2" id="comparison-scroll-hint" class="comparison-scroll-hint hd-muted">← 超出视野时横向滚动；左侧指标固定。聚焦表格可用左右方向键。 →</p>
  <div class="quality-compare hd-compare-table" role="region" aria-label="节点逐项比较表" tabindex="0" :aria-describedby="m.compareNodes.value.length>2?'comparison-scroll-hint':undefined" :style="{'--compare-count':m.compareNodes.value.length}">
   <table><colgroup><col class="comparison-label-column"><col v-for="n in m.compareNodes.value" :key="key(n)"></colgroup>
    <thead><tr><th scope="col">指标 / 服务</th><th v-for="(n,i) in m.compareNodes.value" :key="key(n)" scope="col"><span class="comparison-node-number">{{String(i+1).padStart(2,'0')}}</span><strong>{{nodeDisplayName(n)}}</strong><small>{{m.sourceLabel(n)}}</small><small>{{n.type}}</small></th></tr></thead>
    <tbody>
     <tr v-for="row in rows" :key="row.id"><th scope="row">{{row.label}}</th><td v-for="n in m.compareNodes.value" :key="key(n)"><QualityMetrics :comparison-field="row.field" :project="section==='all'?'combined':section" :visible-service-ids="row.serviceId?[row.serviceId]:row.field==='time'?[]:undefined" :evidence="m.qualityData.value[key(n)]" :error="m.qualityErrors.value[key(n)]" :target="m.compareTarget.value" :catalog="w.catalog.value" @select-service="serviceEvidence(n,$event)"/></td></tr>
     <tr><th scope="row">原始历史</th><td v-for="n in m.compareNodes.value" :key="key(n)"><button class="hd-btn" @click="history(n)">查看原始历史</button></td></tr>
    </tbody>
   </table>
  </div>
 </aside>
</template>
<style scoped>
.hd-compare{gap:12px}
.hd-compare-head h2{font-size:18px;letter-spacing:-.3px}
.comparison-tools{display:flex;justify-content:space-between;gap:12px;align-items:center;flex-wrap:wrap}
.comparison-sections{display:flex;gap:3px;border-bottom:1px solid var(--hd-line)}
.comparison-sections button{background:none;border:0;border-bottom:2px solid transparent;border-radius:0;padding:9px 10px;margin-bottom:-1px;font-size:12px;color:var(--hd-muted);cursor:pointer;white-space:nowrap}
.comparison-sections button[aria-pressed=true]{color:var(--hd-accent);border-color:var(--hd-accent);font-weight:600}
.comparison-tools label{display:flex;align-items:center;gap:8px;font-size:12px;color:var(--hd-muted)}
.comparison-tools .ui-select{width:150px}
.comparison-verdicts{display:flex;flex-direction:column;gap:5px;font-size:12px;line-height:1.5}
.comparison-verdicts p{display:flex;gap:12px;align-items:baseline}
.comparison-verdicts p>span{flex:0 0 26px;color:var(--hd-muted);font-size:11px}
.hd-compare-table{max-height:65vh;border-radius:3px}
.hd-compare-table table{table-layout:fixed;min-width:calc(116px + var(--compare-count,2) * 240px)}
.hd-compare-table .comparison-label-column{width:116px}
.hd-compare-table th,.hd-compare-table td{padding:12px 16px}
.hd-compare-table thead th{vertical-align:bottom;z-index:3}
.hd-compare-table thead th:first-child{z-index:4}
.hd-compare-table thead strong{display:block;margin-bottom:4px;font-size:14px;color:var(--hd-text);font-weight:600}
.comparison-node-number{display:block;font-size:10px;color:var(--hd-accent);font-variant-numeric:tabular-nums;margin-bottom:6px;letter-spacing:1px}
.hd-compare-table tbody th{font-size:12px;line-height:1.6;color:var(--hd-muted);font-weight:500}
@media(max-width:600px){.comparison-tools{align-items:flex-start}.hd-compare-head h2{font-size:16px}.comparison-sections button{padding:8px}.hd-compare-table th,.hd-compare-table td{padding:10px 12px}.hd-compare-table{max-height:60vh}}
</style>
