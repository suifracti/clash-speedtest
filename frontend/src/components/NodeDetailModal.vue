<script setup lang="ts">
import {nodeDisplayName} from '../nodePresentation'
import {downloadGroup,serviceAvailability} from '../sharedResults'
import {attemptTone,serviceConclusion} from '../measurementRounds'
import UiSelect from './UiSelect.vue'
import {computed,onBeforeUnmount,ref,watch} from 'vue'
import {api,saveFile} from '../api'
import {useWorkspace} from '../workspace'
import {bytes,date,downloadSpeed,downloadTone,key,median,outcomeLabel,outcomeTone,sampleFor,stateLabel,targets,type Attempt,type HistoryPage,type LatencyTest,type NodeOption,type Project,type MonitorSample} from '../domain'
import {attemptStatus,downloadConditionLabel,triggerLabel} from '../measurementRounds'
import {manualLatencyPoint,latencyPointLabel,latencySampleState,monitorLatencyPoints,monitorHTTPPoints,monitorHTTPServices,type TrendPoint} from '../presentation'
import {latencySiteSamples} from '../inspectorLatency'
import Icon from './Icon.vue'
import Modal from './Modal.vue'
import TrendChart from './TrendChart.vue'
import MeasurementResults from './MeasurementResults.vue'
const props=defineProps<{node:NodeOption;initialServiceId?:string;initialTargetId?:string;initialProject?:Exclude<Project,'combined'>;initialSource?:'manual'|'monitor';initialUntil?:string}>(),emit=defineEmits<{close:[]}>(),w=useWorkspace(),project=ref<Exclude<Project,'combined'>>(props.initialProject||(w.project.value==='combined'?'latency':w.project.value)),target=ref(props.initialTargetId||w.displayTarget.value),serviceId=ref(props.initialServiceId??w.serviceIds.value[0]??''),tests=ref<LatencyTest[]>([]),attempts=ref<Attempt[]>([]),loading=ref(false),error=ref(''),more=ref(false),raw=ref(false),samplePage=ref(1),revisions=ref<{config_revision_key:string;node_key:string;display_name:string;last_observed_at?:string}[]>([]),revision=ref(props.node.config_revision_key),record=ref<Attempt|null>(null)
const revisionError=ref(''),revisionLoading=ref(false)
const source=ref<'manual'|'monitor'>(props.initialSource||(w.page.value==='monitor'?'monitor':'manual')),monitorProbe=ref('monitor_google_http'),monitorSamples=ref<MonitorSample[]>([]),monitorCursor=ref('')
if(source.value==='monitor'&&project.value==='download')project.value='latency'
let version=0,revisionVersion=0;const until=ref(props.initialUntil||new Date().toISOString()),since=computed(()=>new Date(Date.parse(until.value)-w.hours.value*3600000).toISOString())
const nodeScope=computed(()=>({...props.node,config_revision_key:revision.value,node_key:revisions.value.find(r=>r.config_revision_key===revision.value)?.node_key||props.node.node_key}))
const liveNode=computed(()=>{
 const candidates=w.nodes.value.filter(n=>n.profile_id===props.node.profile_id&&n.node_identity_key===props.node.node_identity_key)
 return candidates.find(n=>n.node_key===props.node.node_key&&n.config_revision_key===props.node.config_revision_key)||(candidates.length===1?candidates[0]:undefined)
})
const viewingCurrent=computed(()=>!!liveNode.value&&key(liveNode.value)===key(nodeScope.value)&&liveNode.value.node_key===nodeScope.value.node_key)
function revisionLabel(r:{node_key:string;config_revision_key:string}){
 const current=liveNode.value
 return current&&current.node_key===r.node_key&&current.config_revision_key===r.config_revision_key?'当前配置':r.config_revision_key?'历史配置 · '+r.config_revision_key.slice(-8):'历史配置 · 版本未记录'
}
function runCurrent(){if(liveNode.value){w.openPlan([liveNode.value]);emit('close')}}
function monitorCurrent(){if(liveNode.value){w.monitorNodes.value=[{...liveNode.value}];emit('close')}}
async function load(append=false){const token=++version;loading.value=true;error.value='';if(!append){tests.value=[];attempts.value=[];monitorSamples.value=[];monitorCursor.value='';more.value=false}try{
if(source.value==='monitor'){const probe=project.value==='service'?monitorHTTPServices.find(p=>p.id===monitorProbe.value):{probeType:'rtt',target:undefined};if(!probe)return;const n={...nodeScope.value};const result=await api.monitorSamples({profile_id:n.profile_id,node_identity_key:n.node_identity_key,config_revision_key:n.config_revision_key,probe_type:probe.probeType,target:probe.target,since:since.value,until:until.value,cursor:append?monitorCursor.value:undefined,limit:project.value==='latency'?300:50,order_desc:true});if(token!==version)return;monitorSamples.value=append?[...monitorSamples.value,...(result.items||[])]:result.items||[];monitorCursor.value=result.next_cursor;more.value=result.has_more;return}
const last=project.value==='latency'?tests.value[tests.value.length-1]:attempts.value[attempts.value.length-1];const extra:Record<string,unknown>={since:since.value,until:until.value,limit:50};if(append&&last){extra.before_attempt_id=last.attempt_id;extra[project.value==='latency'?'before_finished_at':'before_at']=project.value==='latency'?(last as LatencyTest).finished_at:last.finished_at||last.requested_at}if(project.value==='service'&&serviceId.value)extra.service_id=serviceId.value;const result=project.value==='latency'?await api.latencyHistory(nodeScope.value,extra):await api.attemptHistory(project.value,nodeScope.value,extra);if(token!==version)return;more.value=result.has_more;if(project.value==='latency'){const items=(result as HistoryPage<LatencyTest>).tests||[];tests.value=append?[...tests.value,...items]:items;if(revision.value===props.node.config_revision_key)items.forEach(w.mergeLatency)}else{const items=(result as HistoryPage<Attempt>).attempts||[];attempts.value=append?[...attempts.value,...items]:items;if(revision.value===props.node.config_revision_key)items.forEach(a=>w.mergeAttempt(project.value as 'download'|'service',a))}}catch(e){if(token===version)error.value=e instanceof Error?e.message:String(e)}finally{if(token===version)loading.value=false}}
watch(source,()=>{if(source.value==='monitor'&&project.value==='download')project.value='latency'})
watch([source,monitorProbe,project,serviceId,revision,()=>props.node,()=>w.hours.value],()=>{tests.value=[];attempts.value=[];samplePage.value=1;record.value=null;until.value=props.initialUntil||new Date().toISOString();void load()},{immediate:true})
async function loadRevisions(){
 const token=++revisionVersion,n=props.node;revisionLoading.value=true;revisionError.value=''
 try{const result=await api.revisions(n);if(token===revisionVersion)revisions.value=result||[]}
 catch(e){if(token===revisionVersion)revisionError.value=e instanceof Error?e.message:String(e)}
 finally{if(token===revisionVersion)revisionLoading.value=false}
}
watch(()=>props.node,n=>{revision.value=n.config_revision_key;revisions.value=[];void loadRevisions()},{immediate:true})
const revisionOptions=computed(()=>[props.node,...revisions.value.filter(r=>r.config_revision_key!==props.node.config_revision_key)])
function refreshHistory(){until.value=props.initialUntil||new Date().toISOString();record.value=null;void load()}
const recordCount=computed(()=>source.value==='monitor'?monitorSamples.value.length:project.value==='latency'?tests.value.length:attempts.value.length)
const selectedRecordPoint=computed<TrendPoint|null>(()=>record.value?{id:record.value.attempt_id,time:record.value.finished_at||record.value.requested_at,value:project.value==='download'?downloadSpeed(record.value):serviceAvailability(record.value),tone:project.value==='download'?downloadTone(record.value):attemptTone(record.value),description:`执行：${stateLabel(record.value.execution_state)} · 保存：${stateLabel(record.value.persistence_state)}`,source:'manual',attempts:[record.value]}:null)

onBeforeUnmount(()=>{version++;revisionVersion++})
const chartPoints=computed<TrendPoint[]>(()=>{
 if(source.value==='monitor')return project.value==='service'?monitorHTTPPoints(monitorSamples.value,monitorProbe.value):monitorLatencyPoints(monitorSamples.value,target.value)
 if(project.value==='latency')return tests.value.slice().reverse().map(t=>manualLatencyPoint(t,target.value))
 return attempts.value.slice().reverse().map(a=>{
  const value=project.value==='download'?downloadSpeed(a):serviceAvailability(a),tone=project.value==='download'?downloadTone(a):attemptTone(a)
  return {id:a.attempt_id,time:a.finished_at||a.requested_at,value,tone:project.value==='service'&&value===null&&tone==='bad'?'warn':tone,conditionKey:project.value==='download'?downloadGroup(a).key:undefined,description:`${date(a.finished_at||a.requested_at)} · ${triggerLabel(a.trigger_type)} · 轮次 ${a.round_id||'关联未知'} · ${project.value==='download'?outcomeLabel(a.result?.outcome):serviceConclusion(a)}${project.value==='download'?' · '+downloadConditionLabel(a):''}`}
 })
})
const samples=computed(()=>tests.value.flatMap(t=>t.samples.map(s=>({...s,...latencySampleState(s),attempt_id:t.attempt_id}))).sort((a,b)=>Date.parse(b.timestamp)-Date.parse(a.timestamp))),sampleSlice=computed(()=>samples.value.slice((samplePage.value-1)*50,samplePage.value*50))
const latencyRounds=computed(()=>project.value!=='latency'?[]:source.value==='monitor'?monitorLatencyPoints(monitorSamples.value):chartPoints.value)
const outageStats=computed(()=>{
 let executed=0,failures=0,rounds=0
 for(const point of latencyRounds.value){
  const samples=target.value==='all'?point.samples||[]:latencySiteSamples(point,target.value)
  const measured=samples.filter(sample=>sample.tone==='good'||sample.tone==='bad')
  const failed=measured.filter(sample=>sample.tone==='bad').length
  executed+=measured.length;failures+=failed
  if(failed&&failed===samples.length)rounds++
 }
 return {executed,failures,rounds}
})
const siteStats=computed(()=>targets.map(t=>{
 const list=latencyRounds.value.flatMap(point=>latencySiteSamples(point,t.id))
 const values=list.flatMap(sample=>sample.value===null?[]:[sample.value])
 return {...t,value:median(values),success:values.length,total:list.length,failures:list.filter(sample=>sample.tone==='bad').length}
}))
async function retry(a:Attempt){const token=version,kind=project.value as 'download'|'service',n={...nodeScope.value};try{const saved=await api.attemptAction(kind,a.attempt_id,n,'retry-save',a.service_id);if(token!==version)return;record.value=saved;await load()}catch(e){if(token===version)error.value=e instanceof Error?e.message:String(e)}}
const previous=ref<Attempt|null>(null),compareLoading=ref(false),compareError=ref('')
watch(record,async current=>{
 previous.value=null;compareError.value='';compareLoading.value=!!current
 if(!current)return
 try{const result=await api.previousAttempt(project.value as 'download'|'service',current.attempt_id,current);if(record.value===current)previous.value=result}
 catch(e){if(record.value===current)compareError.value=e instanceof Error?e.message:String(e)}
 finally{if(record.value===current)compareLoading.value=false}
})
const comparison=computed(()=>{
 const now=record.value,old=previous.value;if(!now||!old)return ''
 if(project.value==='service')return `${serviceConclusion(old)} → ${serviceConclusion(now)}`
 const a=downloadGroup(now),b=downloadGroup(old),x=downloadSpeed(now),y=downloadSpeed(old)
 if(!a.comparable||!b.comparable||a.key!==b.key||x===null||y===null)return '已有同规则记录，速度或出口条件不足以作数值对比'
 const diff=x-y;return `本次 ${x.toFixed(2)} / 上次 ${y.toFixed(2)} MiB/s · ${diff>0?'增加':diff<0?'减少':'持平'}${diff===0?'':` ${Math.abs(diff).toFixed(2)} MiB/s`}`
})
</script>
<template>
<Modal :title="nodeDisplayName(node)" wide variant="cst-node-history" @close="emit('close')">
  <header class="node-history-context">
    <div><span class="history-account">{{node.profile_name}} · {{node.type}}</span><strong>{{revisionLabel(nodeScope)}}</strong></div>
    <div class="history-actions"><button class="button ghost" :disabled="w.busy.value||!liveNode" @click="runCurrent"><Icon name="play"/>{{viewingCurrent?'检测此节点':'检测当前配置'}}</button><button class="button ghost" :disabled="!liveNode" @click="monitorCurrent">{{viewingCurrent?'加入持续监测':'监测当前配置'}}</button></div>
  </header>
  <p v-if="!viewingCurrent" class="history-scope-note" role="status">{{liveNode?'正在看历史配置；重新检测会使用订阅中的当前配置。':'此配置已不在当前节点清单中，仅查看已保存记录。'}}</p>
  <div class="node-history-layout">
    <aside class="node-history-range" aria-label="节点记录范围">
      <nav class="view-switch" aria-label="节点记录来源"><button :aria-pressed="source==='manual'" @click="source='manual'">工作台记录</button><button :aria-pressed="source==='monitor'" @click="source='monitor'">持续监测</button></nav>
      <nav class="project-tabs" aria-label="记录项目"><button v-for="p in (['latency','download','service'] as Exclude<Project,'combined'>[])" :key="p" :aria-selected="project===p" :disabled="source==='monitor'&&p==='download'" @click="project=p">{{p==='latency'?'延迟':p==='download'?'下载':'服务'}}</button></nav>
      <div class="detail-filters">
        <label>范围<UiSelect v-model.number="w.hours.value" aria-label="节点历史时间范围"><option :value="6">最近 6 小时</option><option :value="24">最近 24 小时</option><option :value="168">最近 7 天</option><option :value="720">最近 30 天</option></UiSelect></label>
        <label v-if="revisionOptions.length>1">配置版本<UiSelect v-model="revision" aria-label="节点配置版本"><option v-for="r in revisionOptions" :key="r.config_revision_key" :value="r.config_revision_key">{{revisionLabel(r)}}</option></UiSelect></label>
        <label v-if="project==='latency'">站点<UiSelect v-model="target" aria-label="历史延迟站点"><option value="all">所有已测站点</option><option v-for="t in targets" :key="t.id" :value="t.id">{{t.name}}</option></UiSelect></label>
        <label v-if="source==='manual'&&project==='service'">服务<UiSelect v-model="serviceId" aria-label="历史服务项目"><option value="">全部服务</option><option v-if="serviceId&&!w.catalog.value.some(s=>s.service_id===serviceId)" :value="serviceId">{{attempts.find(a=>a.service_id===serviceId)?.rule.name||serviceId}}（历史项目）</option><option v-for="s in w.catalog.value" :key="s.service_id" :value="s.service_id">{{s.name}}</option></UiSelect></label>
        <label v-if="source==='monitor'&&project==='service'">监测服务<UiSelect v-model="monitorProbe" aria-label="节点监测服务"><option v-for="p in monitorHTTPServices" :key="p.id" :value="p.id">{{p.name}}</option></UiSelect></label>
      </div>
      <p v-if="revisionLoading" class="small muted">读取配置列表…</p>
      <div v-if="revisionError" class="history-revision-error" role="alert">配置列表读取失败：{{revisionError}}<button class="text-button" :disabled="revisionLoading" @click="loadRevisions">重试读取配置</button></div>
      <div class="history-time-window"><span>{{props.initialUntil?'指定时段':'读取时段'}}</span><time>{{date(since)}}</time><span>至</span><time>{{date(until)}}</time></div>
      <button class="button ghost history-refresh" :disabled="loading" @click="refreshHistory"><Icon name="refresh"/>刷新</button>
    </aside>
    <section class="node-history-content" aria-label="节点历史记录">
      <header class="node-history-heading"><h3>{{source==='monitor'?'持续监测': '工作台'}} · {{project==='latency'?'延迟记录':project==='download'?'下载记录':'服务记录'}}</h3><span>{{loading?'读取中…':error?'读取未完成':`已读 ${recordCount} ${source==='monitor'?'条样本':'条记录'}`}}</span></header>
      <p v-if="source==='monitor'&&project==='service'" class="history-scope-note">基础 HTTP：响应 2xx／3xx 记为成功，不代表解锁或工作台的严格服务规则通过。</p>
      <p v-if="error" class="inline-error" role="alert">{{error}}</p>
      <div v-if="loading&&!chartPoints.length" class="loading-state">读取记录中…</div>
      <template v-else>
        <p v-if="project==='latency'&&outageStats.executed" class="history-scope-note" role="status">已读采样 <b :class="{bad:outageStats.failures}">{{outageStats.failures}} / {{outageStats.executed}} 次无响应</b> · {{outageStats.rounds}} 轮完全无响应<span v-if="more"> · 还有历史未载入</span></p>
        <div v-if="project==='latency'&&recordCount" class="history-site-index" role="group" aria-label="已读站点样本汇总"><button v-for="s in siteStats" :key="s.id" :aria-pressed="target===s.id" @click="target=s.id"><span>{{s.name}}</span><strong>{{s.value===null?'—':`${s.value.toFixed(0)} ms`}}</strong><small>{{s.total?`${s.success}/${s.total} 样本成功`:'未检测'}}<b v-if="s.failures" class="bad"> · {{s.failures}} 次无响应</b></small></button></div>
        <p v-if="source==='manual'&&project==='latency'&&tests.length" class="history-scope-note">汇总仅描述已读样本；跨轮方法或出口可能不同，不能据此直接比较节点。各轮条件见“方法与出口”。</p>
        <TrendChart v-if="chartPoints.length&&(source==='monitor'||project!=='service'||serviceId)" :points="chartPoints" :unit="project==='latency'?'ms':project==='download'?'MiB/s':'通过'" label="所选节点检测历史"/>
        <p v-if="more" class="history-partial">当前显示部分历史，统计和导出仅涵盖已读取的记录。</p>
        <div class="detail-table-tools"><strong>{{source==='monitor'?'持续监测样本':project==='latency'&&raw?'原始样本':'检测记录'}}</strong><button v-if="source==='manual'&&project==='latency'" class="text-button" @click="raw=!raw">{{raw?'查看检测轮次':'查看原始样本'}}</button><button class="text-button" :disabled="!recordCount" title="导出当前已读取的记录" @click="saveFile(JSON.stringify(source==='monitor'?monitorSamples:project==='latency'?tests:attempts,null,2),'speedtest-node-history.json','application/json').then(saved=>{if(saved)w.notify('记录已导出')}).catch(w.fail)">导出记录</button></div>
<div class="table-scroll"><table v-if="source==='monitor'"><thead><tr><th>时间</th><th>目标</th><th>{{project==='latency'?'延迟':'HTTP 结果'}}</th><th>结果</th></tr></thead><tbody><tr v-for="s in monitorSamples" :key="s.sample_id"><td>{{date(s.timestamp)}}<small v-if="s.metadata?.tun_bypass_interface" class="muted">已绕开本机 TUN（{{s.metadata.tun_bypass_interface}}）</small><small v-if="s.metadata?.competing_probes?.length" class="warn">并发竞争：{{s.metadata.competing_probes.join('、')}} · 不宜单独判断节点质量</small></td><td class="break-text">{{s.target}}</td><td>{{project==='latency'?(s.success?`${(s.latency/1e6).toFixed(0)} ms`:'—'):s.success?'HTTP 可达':'失败'}}</td><td :class="s.error_class==='not_executed'?'empty':s.success?(s.error_detail?'warn':'good'):'bad'">{{s.success?'成功':s.error_class||'失败'}}<small v-if="s.error_detail">{{s.error_detail}}</small></td></tr></tbody></table><table v-else-if="project==='latency'&&!raw"><thead><tr><th>完成时间</th><th>所选站点</th><th>成功 / 样本</th><th>保存状态</th></tr></thead><tbody><tr v-for="t in tests" :key="t.attempt_id"><td>{{date(t.finished_at)}}</td><td>{{latencyPointLabel(manualLatencyPoint(t,target),target)}}<details class="small"><summary>方法与出口</summary><p>{{t.source||'来源未知'}} · {{t.method||'方法未知'}} / v{{t.method_version||'?'}} · {{t.attempt_id}}</p><pre>{{JSON.stringify(t.network_path||{evidence:'旧结果无出口证据'},null,2)}}</pre></details></td><td>{{sampleFor(t,target).filter(s=>s.success).length}} / {{sampleFor(t,target).length}}<small v-if="t.status==='not_executed'" class="muted">未执行 · {{t.error_message||t.network_path?.failure_reason||'原因未记录'}}</small></td><td><span :class="t.persistence_state==='failed'?'bad':''">{{stateLabel(t.persistence_state)}}</span><small v-if="t.persistence_error">{{t.persistence_error}}</small></td></tr></tbody></table><table v-else-if="project==='latency'"><thead><tr><th>时间</th><th>目标</th><th>延迟</th><th>结果</th></tr></thead><tbody><tr v-for="s in sampleSlice" :key="s.attempt_id+s.seq+s.target"><td>{{date(s.timestamp)}}</td><td class="break-text">{{s.target||'旧版 Cloudflare'}}</td><td>{{s.success?`${s.latency_ms} ms`:'—'}}</td><td :class="s.tone">{{s.executionState==='cancelled'?'已取消':s.success?'成功':s.error||'失败'}}<small v-if="s.executionState==='cancelled'">{{s.error}}</small></td></tr></tbody></table><table v-else><thead><tr><th>时间</th><th>{{project==='download'?'下载速度':'服务'}}</th><th>结果</th><th>保存</th><th/></tr></thead><tbody><tr v-for="a in attempts" :key="a.attempt_id"><td>{{date(a.finished_at||a.requested_at)}}</td><td>{{project==='download'?downloadSpeed(a)===null?'—':`${downloadSpeed(a)?.toFixed(2)} MiB/s`:a.rule.name||a.service_id}}</td><td><span :class="project==='download'?downloadTone(a):a.result?outcomeTone(a.result.outcome):''">{{attemptStatus(a,project==='download')}}</span><small v-if="project==='download'">{{bytes(a.result?.bytes_read)}}</small></td><td>{{stateLabel(a.persistence_state)}}</td><td><button class="text-button" @click="record=record?.attempt_id===a.attempt_id?null:a">详情</button></td></tr></tbody></table></div><div v-if="source==='manual'&&raw&&project==='latency'" class="pagination"><button class="button ghost" :disabled="samplePage<=1" @click="samplePage--">上一页</button>{{samplePage}} / {{Math.max(1,Math.ceil(samples.length/50))}}<button class="button ghost" :disabled="samplePage*50>=samples.length" @click="samplePage++">下一页</button></div>
        <section v-if="record&&selectedRecordPoint&&project!=='latency'" class="measurement-detail">
          <header class="selected-record-heading"><h3>记录详情</h3><button class="text-button" @click="record=null">收起详情</button></header>
          <p class="selected-record-meta">{{date(record.finished_at||record.requested_at)}} · {{triggerLabel(record.trigger_type)}} · {{record.round_id?`轮次 ${record.round_id}`:'轮次关联未知'}}</p>
<section class="record-comparison" aria-label="和上次比"><h3>和上次比</h3><p v-if="compareLoading">读取上次记录中…</p><p v-else-if="compareError" class="inline-error">对比读取失败：{{compareError}}</p><template v-else-if="previous"><p>{{comparison}}</p><small class="muted">同一节点、配置和检测规则 · 上次 {{date(previous.finished_at||previous.requested_at)}}</small></template><p v-else class="muted">没有更早的同条件记录</p></section>
          <MeasurementResults :point="selectedRecordPoint" :project="project" embedded/>
          <p v-if="record.persistence_error&&record.persistence_state!=='failed'" class="inline-warning">{{record.persistence_error}}</p>
          <div v-if="record.persistence_state==='failed'" class="record-save-actions"><button v-if="record.result" class="button ghost" @click="retry(record)">重试保存</button><span v-else class="muted small">原结果无法恢复</span></div>
        </section>
        <button v-if="more" class="button history-more" :disabled="loading" @click="load(true)">{{loading?'读取中…':'读取更早记录'}}</button>
        <p v-if="!loading&&!error&&!recordCount" class="history-empty">当前范围没有记录；{{liveNode?'可以扩大历史范围或开始检测。':'可以扩大历史范围或切换配置。'}}</p>
      </template>
    </section>
  </div>
</Modal>
</template>

<style>
.modal.wide.cst-node-history{width:min(1120px,calc(100vw - 32px))}
</style>
<style scoped>
.node-history-context{display:flex;justify-content:space-between;align-items:center;gap:14px;flex-wrap:wrap;margin-bottom:14px}
.node-history-context>div:first-child{display:grid;gap:6px}.history-account{font-size:13px;color:var(--muted)}.node-history-context strong{font-size:12px;font-weight:500}
.history-actions{display:flex;gap:8px;flex-wrap:wrap}.history-actions .button{font-size:12px}
.history-scope-note{font-size:12px;color:var(--muted);line-height:1.7;margin:10px 0 16px}
.node-history-layout{display:grid;grid-template-columns:180px minmax(0,1fr);border-top:1px solid var(--line);margin-top:18px;gap:26px}
.node-history-range{border-right:1px solid var(--line);padding:20px 20px 12px 0;min-width:0}
.node-history-range .view-switch{display:flex;flex-wrap:wrap;gap:3px;margin:0 0 16px;background:transparent;border:0;padding:0}
.node-history-range .view-switch button{padding:6px 3px;font-size:11px;border:0;border-radius:0;box-shadow:none;background:none;color:var(--muted);border-bottom:2px solid transparent}
.node-history-range .view-switch button[aria-pressed=true]{color:var(--ink);border-color:var(--blue)}
.node-history-range .project-tabs{display:flex;flex-direction:column;align-items:stretch;margin:0;border:0;padding:0;gap:3px}
.node-history-range .project-tabs button{border:0;border-left:2px solid transparent;border-radius:0;text-align:left;font-size:13px;padding:9px 12px;background:none;color:var(--muted)}
.node-history-range .project-tabs button[aria-selected=true]{background:var(--blue-soft);color:var(--blue);border-left-color:var(--blue);font-weight:600}
.node-history-range .project-tabs button:disabled{opacity:.4}
.detail-filters{display:grid;gap:16px;padding:20px 0 16px;align-items:start}
.detail-filters label{display:grid;gap:7px;font-size:11px}.detail-filters :deep(.ui-select){width:100%;min-width:0;max-width:100%}
.detail-filters :deep(select){max-width:100%;min-width:0;width:100%}
.history-time-window{font-size:11px;color:var(--muted);display:grid;gap:5px;margin:18px 0;line-height:1.5}
.history-time-window>span:first-child{font-weight:600;color:var(--ink)}.history-refresh{width:100%;font-size:12px}
.history-revision-error{font-size:11px;color:var(--amber);line-height:1.6;overflow-wrap:anywhere}.history-revision-error button{display:block;font-size:11px;margin-top:6px}
.node-history-content{min-width:0;padding-top:20px}.node-history-heading{display:flex;align-items:baseline;justify-content:space-between;gap:10px;margin-bottom:18px;flex-wrap:wrap}
.node-history-heading h3{font-size:15px;margin:0}.node-history-heading>span{font-size:11px;color:var(--muted)}
.history-site-index{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));border-top:1px solid var(--line);border-bottom:1px solid var(--line)}
.history-site-index button{display:grid;grid-template-columns:1fr auto;gap:5px 8px;text-align:left;padding:12px 10px;background:none;border:0;border-left:2px solid transparent;cursor:pointer;min-width:0;color:var(--muted)}
.history-site-index button[aria-pressed=true]{background:var(--blue-soft);border-left-color:var(--blue)}
.history-site-index span{font-size:11px}.history-site-index strong{font-size:13px;font-weight:600;color:var(--ink);white-space:nowrap}.history-site-index small{grid-column:1/-1;font-size:10px}
.history-partial{font-size:12px;line-height:1.6;color:var(--amber);padding:8px 0}
.detail-table-tools{display:flex;gap:12px;align-items:center;flex-wrap:wrap;border-top:1px solid var(--line);padding-top:16px;margin:18px 0 12px}
.detail-table-tools strong{font-size:13px}.detail-table-tools button{font-size:11px}
.table-scroll table{min-width:520px}.table-scroll td:first-child{white-space:nowrap}
.measurement-detail{background:none;border:1px solid var(--line);border-radius:4px;margin-top:20px;padding:18px}
.selected-record-heading{display:flex;align-items:baseline;justify-content:space-between;gap:10px}.selected-record-heading h3{font-size:14px;margin:0}.selected-record-heading button{font-size:11px}
.selected-record-meta{font-size:11px;color:var(--muted);overflow-wrap:anywhere;line-height:1.6}
.record-comparison{padding:12px 0 16px;border-bottom:1px solid var(--line);margin-bottom:14px}.record-comparison h3{font-size:12px;font-weight:500}
.record-save-actions{margin-top:12px}.history-more{margin-top:20px}.history-empty{padding:26px 0;color:var(--muted);font-size:13px}
@media(max-width:760px){
 .node-history-layout{grid-template-columns:minmax(0,1fr);gap:0}.node-history-range{border-right:0;border-bottom:1px solid var(--line);padding:16px 0}
 .node-history-range .project-tabs{flex-direction:row;gap:8px}.node-history-range .project-tabs button{border-left:0;border-bottom:2px solid transparent;padding:8px 16px}.node-history-range .project-tabs button[aria-selected=true]{border-bottom-color:var(--blue)}
 .detail-filters{grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.history-time-window{display:flex;flex-wrap:wrap;gap:8px;margin:0 0 12px}.history-refresh{width:auto}
 .node-history-heading{margin-bottom:12px}.history-site-index{grid-template-columns:repeat(2,minmax(0,1fr))}.node-history-context{align-items:flex-start}
 .measurement-detail{padding:12px}.history-actions .button{font-size:11px}
}
</style>
