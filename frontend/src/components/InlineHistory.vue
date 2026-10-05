<script setup lang="ts">
import {nodeDisplayName} from '../nodePresentation'
import UiSelect from './UiSelect.vue'
import {computed,onBeforeUnmount,ref,watch} from 'vue'
import {useWorkspace} from '../workspace'
import {api} from '../api'
import {key,outcomeLabel,targets,type NodeOption,type Project} from '../domain'
import {monitorRoundPoints,monitorHTTPPoints,monitorHTTPServices,points,recentService,type NodeSeries,type TrendPoint} from '../presentation'
import MultiNodeChart from './MultiNodeChart.vue'
import HealthBars from './HealthBars.vue'
import MeasurementResults from './MeasurementResults.vue'
const props=defineProps<{node:NodeOption;nodes:NodeOption[];project:Exclude<Project,'combined'>;source?:'manual'|'monitor';monitorServiceId?:string;anchorPoints?:TrendPoint[];targetIds?:string[];serviceIds?:string[]}>(),w=useWorkspace()
const scope=ref<'node'|'selected'|'filtered'>('node'),service=ref(''),serviceSearch=ref(''),inspected=ref<TrendPoint|null>(null),loading=ref(false),error=ref(''),monitorHistory=ref<Record<string,TrendPoint[]>>({})
const comparisonNodes=computed(()=>scope.value==='node'?[props.node]:[...new Map([props.node,...(scope.value==='selected'?w.selectedNodes.value:props.nodes)].map(n=>[key(n),n])).values()])
const displayServiceIds=computed(()=>props.serviceIds??(w.displayServiceIds.value.length?w.displayServiceIds.value:w.serviceIds.value))
const displayServices=computed(()=>[...new Set(displayServiceIds.value)].flatMap(id=>{const rule=w.catalog.value.find(s=>s.service_id===id);return rule?[rule]:[]}))
const chosenServices=computed(()=>service.value?displayServices.value.filter(s=>s.service_id===service.value):displayServices.value)
const chosenService=computed(()=>service.value||displayServices.value[0]?.service_id||'')
const services=computed(()=>displayServices.value.filter(s=>s.name.toLowerCase().includes(serviceSearch.value.trim().toLowerCase())))
function airportLabel(n:NodeOption){return [...new Set([w.nodeAirportName(n),w.nodeSubscriptionName(n)].filter(Boolean))].join(' · ')||n.profile_name}
const displayTargets=computed(()=>{const ids=props.targetIds?.length?props.targetIds:[w.displayTarget.value],chosen=[...new Set(ids)].flatMap(id=>{const target=targets.find(t=>t.id===id);return target?[target]:[]});return chosen.length?chosen:[targets[0]]})
const series=computed(()=>comparisonNodes.value.flatMap((n):NodeSeries[]=>{
  if(props.project==='latency'&&props.source!=='monitor'&&displayTargets.value.length>1)return displayTargets.value.map(target=>({id:key(n)+':'+target.id,nodeId:key(n),targetId:target.id,targetName:target.name,name:nodeDisplayName(n),airport:airportLabel(n),points:points(w,n,props.project,target.id)}))
  if(props.project==='service'&&props.source!=='monitor')return chosenServices.value.map(rule=>({id:key(n)+':service:'+rule.service_id,nodeId:key(n),targetId:rule.service_id,targetName:rule.name,name:nodeDisplayName(n),airport:airportLabel(n),points:points(w,n,'service',displayTargets.value[0].id,rule.service_id)}))
  return [{id:key(n),name:nodeDisplayName(n),airport:airportLabel(n),points:props.source==='monitor'?(key(n)===key(props.node)?props.anchorPoints||[]:monitorHistory.value[key(n)]||[]):points(w,n,props.project,displayTargets.value[0].id,chosenService.value)}]
}))
const context=computed(()=>props.source==='monitor'?(props.project==='service'?'持续监测 · '+(monitorHTTPServices.find(s=>s.id===props.monitorServiceId)?.name||'基础 HTTP'):'持续监测 · Cloudflare'):props.project==='latency'?displayTargets.value.map(target=>target.name).join('、'):props.project==='download'?'下载速度':chosenServices.value.map(rule=>rule.name).join('、')||'服务可用性')
const historyError=computed(()=>[error.value,...(props.source==='monitor'?[]:comparisonNodes.value.flatMap(n=>{const message=w.historyReadError(n,props.project);return message?[`${nodeDisplayName(n)} · ${airportLabel(n)}：${message}`]:[]}))].filter(Boolean).join('；'))
const hasRecords=computed(()=>series.value.some(s=>s.points.some(p=>p.tone!=='empty')))
let generation=0
async function load(){
  const version=++generation,list=comparisonNodes.value.slice();loading.value=true;error.value=''
  try{
    if(props.source==='monitor'){
      for(const n of list){
        if(version!==generation)return
        if(key(n)===key(props.node))continue
        const probes=props.project==='service'?monitorHTTPServices.filter(s=>!props.monitorServiceId||s.id===props.monitorServiceId):[{probeType:'rtt',target:'https://cp.cloudflare.com/generate_204'}]
        const samples=[]
        for(const probe of probes){const result=await api.monitorSamples({profile_id:n.profile_id,node_identity_key:n.node_identity_key,config_revision_key:n.config_revision_key,probe_type:probe.probeType,target:probe.target,since:new Date(Date.now()-w.hours.value*3600000).toISOString(),until:new Date().toISOString(),limit:80,order_desc:true});if(version!==generation)return;samples.push(...(result.items||[]))}
        if(version===generation)monitorHistory.value[key(n)]=props.project==='service'?monitorHTTPPoints(samples,props.monitorServiceId):monitorRoundPoints(samples)
      }
    }else for(let i=0;i<list.length;i+=100){if(version!==generation)return;await w.ensureHistory(list.slice(i,i+100),[props.project])}
  }catch(e){if(version===generation)error.value=e instanceof Error?e.message:'历史读取失败'}finally{if(version===generation)loading.value=false}
}
function serviceStatus(id:string){const attempt=recentService(w,props.node,id);return attempt?.result?outcomeLabel(attempt.result.outcome):'暂无记录'}
function inspectService(_id:string,point:TrendPoint){inspected.value=point}
watch(()=>comparisonNodes.value.map(key).sort().join('|')+'|'+props.project+'|'+w.hours.value+'|'+props.source+'|'+props.monitorServiceId,()=>{inspected.value=null;void load()},{immediate:true})
watch(()=>props.project+'|'+w.hours.value+'|'+props.source+'|'+props.monitorServiceId,()=>{monitorHistory.value={}},{flush:'sync'})
watch(()=>displayServiceIds.value,ids=>{if(service.value&&!ids.includes(service.value))service.value='';inspected.value=null},{deep:true,immediate:true})
watch(()=>key(props.node),()=>{scope.value='node';monitorHistory.value={};inspected.value=null})
watch(()=>displayTargets.value.map(target=>target.id).join('|'),()=>{inspected.value=null})
onBeforeUnmount(()=>{generation++})
</script>
<template>
  <section class="inline-full-history" aria-label="行内完整走势">
    <header class="inline-history-tools">
      <nav class="view-switch" aria-label="走势对比范围">
        <button :aria-pressed="scope==='node'" @click="scope='node'">当前节点</button>
        <button :aria-pressed="scope==='selected'" :disabled="!w.selectedNodes.value.length" @click="scope='selected'">已选节点<span v-if="w.selectedNodes.value.length"> · {{w.selectedNodes.value.length}}</span></button>
        <button :aria-pressed="scope==='filtered'" @click="scope='filtered'">当前筛选 · {{nodes.length}}</button>
      </nav>
      <label v-if="project==='service'&&source!=='monitor'">对比服务<UiSelect v-model="service" :disabled="!displayServices.length" aria-label="走势对比服务" @change="inspected=null"><option value="">{{displayServices.length>1?'本页全部服务 · '+displayServices.length:displayServices.length?'全部显示服务':'暂无显示服务'}}</option><option v-for="s in displayServices" :key="s.service_id" :value="s.service_id">{{s.name}}</option></UiSelect></label>
      <span class="small muted">{{context}} · {{loading?'读取历史中…':'沿用上方历史范围'}}</span>
    </header>
    <p v-if="historyError" class="small bad" role="alert">历史读取失败：{{historyError}} <button class="text-button" aria-label="重新读取历史" :disabled="loading" @click="load">{{loading?'读取中…':'重新读取'}}</button></p>
    <MultiNodeChart v-if="!historyError||hasRecords" :series="series" :unit="project==='download'?'MiB/s':project==='service'?'状态':'ms'" :title="scope==='node'?nodeDisplayName(node)+' · 完整走势':'节点走势对比'" fold-legend/>
    <details v-if="project==='service'&&source!=='monitor'" class="inline-service-history">
      <summary>按服务查看 {{nodeDisplayName(node)}} 的健康记录 · {{displayServices.length}} 项显示服务</summary>
      <input v-if="displayServices.length>8" v-model="serviceSearch" type="search" aria-label="搜索历史服务" placeholder="搜索服务名称">
      <div class="inline-service-rows">
        <div v-for="s in services" :key="s.service_id" class="inline-service-row" :class="{active:chosenServices.some(rule=>rule.service_id===s.service_id)}">
          <button class="text-button" @click="service=s.service_id;inspected=null"><strong>{{s.name}}</strong><small>{{serviceStatus(s.service_id)}}</small></button>
          <HealthBars :points="points(w,node,'service',w.displayTarget.value,s.service_id)" :label="s.name" @inspect="inspectService(s.service_id,$event)" @open="inspectService(s.service_id,$event)"/>
        </div>
      </div>
      <MeasurementResults v-if="inspected" :point="inspected" project="service"/>
      <p v-if="!services.length" class="small muted">暂无匹配的服务，请在上方选择显示服务。</p>
    </details>
    <p class="small muted inline-history-caption">{{source==='monitor'?'持续监测历史':'已保存的检测历史'}} · 红叉表示失败，未测不会当作失败；切换对比范围只影响图表。</p>
  </section>
</template>
