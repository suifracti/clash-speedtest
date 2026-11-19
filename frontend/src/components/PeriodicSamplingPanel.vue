<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,reactive,ref,watch} from 'vue'
import {api} from '../api'
import {bytes,date} from '../domain'
import type {PeriodicSamplingConfig,PeriodicSamplingStatus,PeriodicChecks} from '../periodicSampling'
const status=ref<PeriodicSamplingStatus|null>(null),error=ref(''),readError=ref(''),busy=ref(false)
const emit=defineEmits<{state:[value:{status:PeriodicSamplingStatus|null;error:string}]}>()
watch([status,error,readError],()=>emit('state',{status:status.value,error:readError.value||error.value}),{immediate:true})
const form=reactive({latency:2,service:5,download:60,mib:10,concurrency:16,antigravity:true})
const checks=ref<PeriodicChecks|null>(null),checkNodes=ref<PeriodicChecks['nodes']>([]),checksError=ref(''),checksOpen=ref(false),checkNode=ref(''),checksLoading=ref(false),reading=ref(false)
const stateLabel=computed(()=>readError.value?(status.value?'状态需确认':'状态未知'):!status.value?'读取中…':status.value.pause_reason?'容量保护暂停':status.value.running?'巡检已启动':status.value.config.enabled?'计划已启用 · 当前未运行':'未启动')
let ticks=0,readGeneration=0,checksGeneration=0,mounted=true
let timer:ReturnType<typeof setInterval>|undefined
function message(e:unknown){return e instanceof Error?e.message:String(e)}
async function loadChecks(){
 const token=++checksGeneration,[profile,node]=checkNode.value.split('\u0000')
 checksLoading.value=true;checks.value=null;checksError.value=''
 try{
  const next=await api.periodicChecks(profile||'',node||'')
  if(!mounted||token!==checksGeneration)return
  checks.value=next;checkNodes.value=next.nodes;checkNode.value=next.node.profile_id+'\u0000'+next.node.node_key
 }catch(e){if(mounted&&token===checksGeneration)checksError.value=message(e)}
 finally{if(mounted&&token===checksGeneration)checksLoading.value=false}
}
function checkToggle(event:Event){checksOpen.value=(event.target as HTMLDetailsElement).open;if(checksOpen.value)void loadChecks()}
function syncForm(cfg:PeriodicSamplingConfig){form.latency=cfg.latency_interval_seconds/60;form.service=cfg.service_interval_seconds/60;form.download=cfg.download_interval_seconds/60;form.mib=cfg.download_mib;form.concurrency=cfg.service_concurrency||16;form.antigravity=cfg.include_antigravity}
async function load(){
 if(reading.value||busy.value)return
 const token=++readGeneration;reading.value=true
 try{
  const value=await api.periodicSampling()
  if(!mounted||token!==readGeneration)return
  if(!status.value)syncForm(value.config)
  status.value=value;readError.value=''
 }catch(e){if(mounted&&token===readGeneration)readError.value=message(e)}
 finally{if(mounted&&token===readGeneration)reading.value=false}
}
async function save(enabled:boolean){
 if(!status.value||busy.value)return
 const cfg:PeriodicSamplingConfig=enabled?{enabled:true,latency_interval_seconds:Math.round(form.latency*60),service_interval_seconds:Math.round(form.service*60),download_interval_seconds:Math.round(form.download*60),download_mib:form.mib,include_antigravity:form.antigravity,service_concurrency:form.concurrency}:{...status.value.config,enabled:false}
 ++readGeneration;reading.value=false;busy.value=true;error.value=''
 try{
  const result=await api.configurePeriodicSampling(cfg)
  if(!mounted)return
  status.value=result;readError.value='';syncForm(result.config)
 }catch(e){if(mounted)error.value=message(e)}finally{if(mounted)busy.value=false}
}
onMounted(()=>{void load();timer=setInterval(()=>{if(!busy.value){void load();if(checksOpen.value&&!checksLoading.value&&++ticks%3===0)void loadChecks()}},10000)})
onBeforeUnmount(()=>{mounted=false;readGeneration++;checksGeneration++;clearInterval(timer)})
const cycleNames:Record<string,string>={service:'服务持续巡检',download:'下载测速'}
function duration(seconds:number|null|undefined){if(seconds===undefined||seconds===null)return '—';const s=Math.max(0,Math.round(seconds));return `${Math.floor(s/3600)}小时${Math.floor(s%3600/60)}分${s%60}秒`}
</script>
<template>
<article class="monitor-card periodic-panel">
 <header class="periodic-head">
  <div>
   <h2>全部节点持续巡检</h2>
   <small class="muted">当前覆盖 {{status?.node_count??'—'}} 个节点 · {{status?.service_count??'—'}} 项服务规则</small>
  </div>
  <div class="periodic-head-actions">
   <span class="badge" :class="readError?'warn':status?.running?'good':''">{{stateLabel}}</span>
   <div class="button-group">
    <button data-test="refresh-status" class="button ghost" :disabled="reading||busy" @click="load">{{reading?'读取中…':'刷新状态'}}</button>
    <button data-test="start" class="button primary" :disabled="busy||!status" @click="save(true)">保存并启动</button>
    <button data-test="stop" class="button" :disabled="busy||!status||(!status.running&&!status.config.enabled)" @click="save(false)">停止定时采样</button>
   </div>
  </div>
 </header>
 <fieldset class="periodic-fields" :disabled="busy||!status">
  <legend>巡检频率与用量</legend>
  <div class="patrol-lane">
   <div class="patrol-purpose"><h3>四站延迟</h3><p>观察线路响应与波动</p><small>Cloudflare · Google · GitHub · Apple</small></div>
   <div class="patrol-inputs"><label>延迟周期（分钟）<input data-test="latency" type="number" min="0.167" max="1440" step="1" v-model.number="form.latency"></label></div>
  </div>
  <div class="patrol-lane">
   <div class="patrol-purpose"><h3>服务可用性</h3><p>按节点和服务轮转检测</p><small>共 {{status?.service_count??'未知'}} 项规则，实际覆盖见本轮进度</small></div>
   <div class="patrol-inputs"><label>巡检计划间隔（分钟）<input type="number" min="0.167" max="1440" step="1" v-model.number="form.service"></label><label>服务并发上限（1–64）<input data-test="service-concurrency" type="number" min="1" max="64" step="1" v-model.number="form.concurrency"></label></div>
   <label class="check-field patrol-model"><input type="checkbox" v-model="form.antigravity">包含 Antigravity 真实模型请求（会消耗模型额度）</label>
  </div>
  <div class="patrol-lane">
   <div class="patrol-purpose"><h3>下载测速</h3><p>单路采样，避开服务请求</p><small>每批最多 4 个节点，完成后交让巡检</small></div>
   <div class="patrol-inputs"><label>下载周期（分钟）<input type="number" min="0.167" max="1440" step="1" v-model.number="form.download"></label><label>每节点下载上限（MiB）<input type="number" min="1" max="100" step="1" v-model.number="form.mib"></label></div>
  </div>
 </fieldset>
 <p class="muted small patrol-expectation">计划间隔决定调度节奏。5 分钟全覆盖目标尚未实现，以实际耗时和覆盖进度为准。</p>
 <p v-if="status?.config.selections?.length" class="inline-warning">当前为 {{status.config.selections.length}} 个节点的验证范围；点“保存并启动”切换为全部当前节点。</p>
 <p v-if="error||readError||status?.error||status?.pause_reason" class="inline-warning" role="alert">{{error||(readError?'状态读取失败'+(status?'；保留上次读数':'')+'：'+readError:status?.pause_reason||status?.error)}}</p>
 <div v-if="status?.cycles&&Object.keys(status.cycles).length" class="cst-cycle-grid">
  <div v-for="(cycle,kind) in status?.cycles" :key="kind" class="cst-cycle-card periodic-progress">
   <div class="cst-cycle-head">
    <strong>{{cycleNames[kind]||kind}} · {{cycle.running?'本轮执行中':'本轮结束'}} {{cycle.processed}} / {{cycle.total}}</strong>
    <span class="badge" :class="cycle.running?'good':cycle.interrupted?'warn':''">{{cycle.total?Math.round(cycle.processed/cycle.total*100)+'%':'0%'}}</span>
   </div>
   <div class="cst-cycle-bar"><i :style="{width:cycle.total?`${Math.min(100,cycle.processed/cycle.total*100)}%`:'0%'}"/></div>
   <div class="cst-cycle-stats">
    <span>实际耗时 {{duration(cycle.elapsed_seconds)}} · 本轮并发上限 {{cycle.concurrency??(kind==='download'?1:form.concurrency)}} · 入场资源等待累计 {{duration(cycle.wait_seconds)}}（各任务相加）</span>
    <span>待调度 {{Math.max(0,cycle.total-cycle.processed-(cycle.active??0)-(cycle.waiting??0))}} · 资源排队 {{cycle.waiting??0}} · 当前网络测量 {{cycle.network_active??cycle.active??0}} · 当前处理（含保存） {{cycle.active??0}} · 累计启动 {{cycle.legacy_observation?'旧记录未知':cycle.started??0}} · 未执行 {{cycle.not_executed??cycle.outcomes.not_measured??0}}（不计服务失败）</span>
    <span v-if="cycle.timing_version">队列投递等待 {{duration(cycle.queue_wait_seconds)}} · 下载互斥等待 {{duration(cycle.download_exclusion_wait_seconds)}} · 启动入场等待 {{duration(cycle.admission_wait_seconds)}}（各项累计，保存锁等待见新结果）</span><span v-else>旧轮次未分开记录队列、下载互斥与保存锁等待；未知不按零处理。</span>
    <span>实际执行累计 {{duration(cycle.execution_seconds)}} · 探测超时 {{cycle.outcomes.timed_out??0}} · 限流 {{cycle.outcomes.rate_limited??0}} · 观测错误 {{cycle.outcomes.execution_observation_error??0}}</span>
    <span v-if="cycle.running">预计整轮 {{duration(cycle.estimated_total_seconds)}} · 预计剩余 {{duration(cycle.remaining_seconds)}}（按本轮实际吞吐估算，包含等待，非承诺）</span>
    <span v-if="kind==='service'">已尝试节点 {{cycle.covered_nodes??0}} / {{status?.node_count}} · 已尝试服务 {{cycle.covered_services??0}} / {{status?.service_count}} · 全部项完成的节点 {{cycle.completed_nodes??0}} / {{status?.node_count}}</span>
    <span v-if="cycle.interrupted" class="inline-warning">本轮已中断，保留已保存结果，未完成全覆盖。</span>
    <span v-else-if="!cycle.running&&kind==='service'">{{cycle.full_coverage?'本轮检测项已全部尝试；通过情况见真实结果':'本轮未完成全覆盖'}}</span>
    <span>已保存 {{cycle.saved}} · 执行后未确认保存 {{cycle.unsaved}} · 响应体 {{bytes(cycle.bytes_read)}} · 超期跳过 {{cycle.skipped_slots}} 轮</span>
    <span>本轮结束 {{cycle.finished_at?.startsWith('0001-')?'—':date(cycle.finished_at)}} · 下轮 {{cycle.next_due?.startsWith('0001-')?'本轮结束后确定':date(cycle.next_due)}}</span>
    <span v-if="cycle.error" class="inline-warning">{{cycle.error}}</span>
   </div>
  </div>
 </div>
 <details v-if="status?.recent_cycles?.length" class="cst-info-collapse"><summary>最近轮次实际耗时（{{status.recent_cycles.length}} 轮）</summary><div class="cst-info-collapse-body"><div v-for="(record,i) in status.recent_cycles.slice(-6).reverse()" :key="i" class="periodic-progress">{{cycleNames[record.kind]}} · {{date(record.cycle.finished_at)}} · {{duration(record.cycle.elapsed_seconds)}} · {{record.cycle.concurrency}} 路 · {{record.cycle.processed}} / {{record.cycle.total}} · {{record.cycle.interrupted?'中断':record.cycle.full_coverage?'本轮全覆盖':'覆盖未完成'}} · 未执行 {{record.cycle.not_executed??record.cycle.outcomes.not_measured??0}}</div></div></details>
 <details class="cst-info-collapse" @toggle="checkToggle"><summary>每项最后检测时间（按节点 · 当前配置版本）</summary>
 <div class="cst-info-collapse-body">
  <p class="muted">未执行或历史未知保持未知；显示最新实际结果，正在排队不会覆盖上次真实检测时间。</p>
  <p v-if="checksError" class="inline-warning">{{checksError}}</p>
  <div class="periodic-checks-box"><div v-if="checkNodes.length" class="periodic-checks-toolbar"><label>巡检节点<select v-model="checkNode" @change="loadChecks"><option v-for="n in checkNodes" :key="n.profile_id+n.node_key" :value="n.profile_id+'\u0000'+n.node_key">{{n.profile_name}} · {{n.display_name}}</option></select></label><button class="button" @click="loadChecks">刷新检测时间</button></div>
  <p v-if="checksLoading" class="muted" role="status">正在读取所选节点的检测时间…</p>
  <div v-if="checks" class="table-scroll"><table><thead><tr><th>服务</th><th>最后实际检测</th><th>真实结果</th><th>保存与竞争</th></tr></thead><tbody><tr v-for="c in checks.checks" :key="c.service_id"><td>{{c.name}}</td><td>{{c.last_detected_at?date(c.last_detected_at):'未执行 / 无实际结果 / 未知'}}</td><td>{{c.last_detected_at?c.outcome:'未知'}}</td><td>{{c.persistence}} {{c.competing_probes?'竞争：'+c.competing_probes:''}}</td></tr></tbody></table></div></div>
 </div>
 </details>
 <details class="cst-info-collapse"><summary>巡检调度规则与并发边界说明</summary><div class="cst-info-collapse-body">
  <p class="muted">全部 {{status?.service_count??'未知'}} 项作为持续巡检，按订阅、节点和服务公平轮转。计划间隔不等于全覆盖耗时；一轮完成后安排下一轮，超期跳过、不积压。</p>
  <p class="muted">下载保持单路、每节点最多 {{form.mib}} MiB，不与服务请求同时测量；每批最多 4 个节点并设 15 秒交让预算（正在执行的单节点完成后交让），批次间让巡检继续。配置保存后随服务启动恢复。</p>
  <p class="muted">新延迟任务默认 Cloudflare、Google、GitHub、Apple 四站；旧冻结任务保持原范围。全局站点有记录不代表每个节点都覆盖四站。延迟与其他探测可能重叠；应用内竞争写入结果标记，延迟或速度下降不能直接归因于节点。外部应用流量无法自动排除。</p>
 </div></details>
</article>
</template>
<style scoped>
.periodic-panel.monitor-card{margin:0;padding:0;border:0;border-radius:0;box-shadow:none;background:none;display:grid;gap:14px}
.periodic-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;padding-bottom:12px}
.periodic-head h2{margin:0 0 7px;font-size:18px;font-weight:650}
.periodic-head-actions{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.periodic-fields{min-width:0;margin:0;padding:0;border:0;border-top:2px solid var(--cst-text)}
.periodic-fields legend{padding:0 12px 0 0;font-size:12px;font-weight:600;color:var(--cst-muted)}
.patrol-lane{display:grid;grid-template-columns:minmax(220px,1fr) minmax(300px,1.3fr);gap:10px 30px;border-bottom:1px solid var(--cst-line);padding:22px 0}
.patrol-purpose h3{font-size:15px;margin:0 0 8px}.patrol-purpose p{font-size:12px;margin:0 0 6px;color:var(--cst-muted)}.patrol-purpose small{font-size:11px;color:var(--cst-muted)}
.patrol-inputs{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px;align-items:center;min-width:0}
.patrol-inputs label{display:grid;gap:8px;font-size:12px;color:var(--cst-muted)}
.patrol-inputs input{width:100%;min-width:0;height:36px;padding:6px 10px;border:1px solid var(--cst-line-strong);border-radius:5px;background:var(--cst-panel);color:var(--cst-text);font-size:14px;font-variant-numeric:tabular-nums}
.patrol-model{grid-column:2;font-size:12px}.patrol-expectation{margin:0;line-height:1.7}
@media(max-width:720px){.patrol-lane{grid-template-columns:minmax(0,1fr);gap:16px}.patrol-model{grid-column:1}.periodic-head-actions{width:100%;justify-content:space-between}}
.periodic-checks-toolbar{display:flex;align-items:flex-end;gap:10px;flex-wrap:wrap;margin-bottom:10px}
.periodic-checks-toolbar label{display:flex;flex-direction:column;gap:4px;font-size:12px;color:var(--cst-muted);flex:1;min-width:220px}
.periodic-checks-toolbar select{min-height:34px;padding:6px 10px;border:1px solid var(--cst-line-strong);border-radius:8px;background:var(--cst-panel);color:var(--cst-text)}
.periodic-panel table{width:100%;font-size:12px;text-align:left;border-collapse:collapse}
.periodic-panel td,.periodic-panel th{padding:8px 10px;border-bottom:1px solid var(--cst-line)}
.periodic-panel th{background:var(--cst-panel-2);color:var(--cst-muted);font-weight:650}
</style>
