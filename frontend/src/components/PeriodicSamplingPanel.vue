<script setup lang="ts">
import {onBeforeUnmount,onMounted,reactive,ref} from 'vue'
import {api} from '../api'
import {bytes,date} from '../domain'
import type {PeriodicSamplingConfig,PeriodicSamplingStatus,PeriodicChecks} from '../periodicSampling'
const status=ref<PeriodicSamplingStatus|null>(null),error=ref(''),readError=ref(''),busy=ref(false)
const form=reactive({latency:2,service:5,download:60,mib:10,concurrency:16,antigravity:true})
const checks=ref<PeriodicChecks|null>(null),checksError=ref(''),checksOpen=ref(false),checkNode=ref('')
async function loadChecks(){try{const [profile,node]=checkNode.value.split('\u0000');checks.value=await api.periodicChecks(profile||'',node||'');checkNode.value=checks.value.node.profile_id+'\u0000'+checks.value.node.node_key;checksError.value=''}catch(e){checksError.value=e instanceof Error?e.message:String(e)}}
function checkToggle(event:Event){checksOpen.value=(event.target as HTMLDetailsElement).open;if(checksOpen.value)void loadChecks()}
let ticks=0
let timer:ReturnType<typeof setInterval>|undefined
function syncForm(cfg:PeriodicSamplingConfig){form.latency=cfg.latency_interval_seconds/60;form.service=cfg.service_interval_seconds/60;form.download=cfg.download_interval_seconds/60;form.mib=cfg.download_mib;form.concurrency=cfg.service_concurrency||16;form.antigravity=cfg.include_antigravity}
async function load(initial=false){try{const value=await api.periodicSampling();status.value=value;readError.value='';if(initial)syncForm(value.config)}catch(e){readError.value=e instanceof Error?e.message:String(e)}}
async function save(enabled:boolean){if(!status.value)return;busy.value=true;error.value='';try{
 const cfg:PeriodicSamplingConfig=enabled?{enabled:true,latency_interval_seconds:Math.round(form.latency*60),service_interval_seconds:Math.round(form.service*60),download_interval_seconds:Math.round(form.download*60),download_mib:form.mib,include_antigravity:form.antigravity,service_concurrency:form.concurrency}:{...status.value.config,enabled:false}
 const result=await api.configurePeriodicSampling(cfg);status.value=result;syncForm(result.config)
}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
onMounted(()=>{void load(true);timer=setInterval(()=>{if(!busy.value){void load();if(checksOpen.value&&++ticks%3===0)void loadChecks()}},10000)})
onBeforeUnmount(()=>clearInterval(timer))
const cycleNames:Record<string,string>={service:'服务持续巡检',download:'下载测速'}
function duration(seconds:number|null|undefined){if(seconds===undefined||seconds===null)return '—';const s=Math.max(0,Math.round(seconds));return `${Math.floor(s/3600)}小时${Math.floor(s%3600/60)}分${s%60}秒`}
</script>
<template>
<article class="monitor-card periodic-panel">
 <header><div><h2>全部节点持续巡检</h2><small>当前 {{status?.node_count??'—'}} 个节点 · {{status?.service_count??'—'}} 个服务</small></div><span class="badge" :class="status?.running?'good':''">{{status?.pause_reason?'容量保护暂停':status?.running?'巡检已启动':'未启动'}}</span></header>
 <div class="periodic-fields">
  <label>延迟周期（分钟）<input data-test="latency" type="number" min="0.167" max="1440" step="1" v-model.number="form.latency"></label>
  <label>巡检计划间隔（分钟）<input type="number" min="0.167" max="1440" step="1" v-model.number="form.service"></label>
  <label>服务并发上限（1–64）<input data-test="service-concurrency" type="number" min="1" max="64" step="1" v-model.number="form.concurrency"></label>
  <label>下载周期（分钟）<input type="number" min="0.167" max="1440" step="1" v-model.number="form.download"></label>
  <label>每节点下载上限（MiB）<input type="number" min="1" max="100" step="1" v-model.number="form.mib"></label>
 </div>
 <label class="check-field"><input type="checkbox" v-model="form.antigravity">包含 Antigravity 真实模型请求（会消耗模型额度）</label>
 <p class="muted">全部 {{status?.service_count??82}} 项作为持续巡检，按订阅、节点和服务公平轮转。计划间隔不等于全覆盖耗时；一轮完成后安排下一轮，超期跳过、不积压。</p>
 <p class="inline-warning">5 分钟全覆盖目标尚未实现，以实际耗时和覆盖进度为准。</p>
 <p class="muted">下载保持单路、每节点最多 {{form.mib}} MiB，不与服务请求同时测量；每批最多 4 个节点并设 15 秒交让预算（正在执行的单节点完成后交让），批次间让巡检继续。配置保存后随服务启动恢复。</p>
 <p class="muted">新延迟任务默认 Cloudflare、Google、GitHub、Apple、Microsoft、Firefox 六站；旧冻结任务保持原范围，当前均未启动。全局站点有记录不代表每个节点都覆盖六站。延迟与其他探测可能重叠；应用内竞争写入结果标记，延迟或速度下降不能直接归因于节点。外部应用流量无法自动排除。</p>
 <p v-if="status?.config.selections?.length" class="inline-warning">当前为 {{status.config.selections.length}} 个节点的验证范围；点“保存并启动”切换为全部当前节点。</p>
 <p v-if="error||readError||status?.error||status?.pause_reason" class="inline-warning" role="alert">{{error||readError||status?.pause_reason||status?.error}}</p>
 <div v-for="(cycle,kind) in status?.cycles" :key="kind" class="periodic-progress">
  <strong>{{cycleNames[kind]||kind}} · {{cycle.running?'本轮执行中':'本轮结束'}} {{cycle.processed}} / {{cycle.total}}</strong>
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
 <details v-if="status?.recent_cycles?.length"><summary>最近轮次实际耗时</summary><div v-for="(record,i) in status.recent_cycles.slice(-6).reverse()" :key="i" class="periodic-progress">{{cycleNames[record.kind]}} · {{date(record.cycle.finished_at)}} · {{duration(record.cycle.elapsed_seconds)}} · {{record.cycle.concurrency}} 路 · {{record.cycle.processed}} / {{record.cycle.total}} · {{record.cycle.interrupted?'中断':record.cycle.full_coverage?'本轮全覆盖':'覆盖未完成'}} · 未执行 {{record.cycle.not_executed??record.cycle.outcomes.not_measured??0}}</div></details>
 <details @toggle="checkToggle"><summary>每项最后检测时间（按节点 · 当前配置版本）</summary>
 <p class="muted">未执行或历史未知保持未知；显示最新实际结果，正在排队不会覆盖上次真实检测时间。</p>
 <p v-if="checksError" class="inline-warning">{{checksError}}</p>
 <div v-if="checks"><label>巡检节点<select v-model="checkNode" @change="loadChecks"><option v-for="n in checks.nodes" :key="n.profile_id+n.node_key" :value="n.profile_id+'\u0000'+n.node_key">{{n.profile_name}} · {{n.display_name}}</option></select></label><button class="button" @click="loadChecks">刷新检测时间</button>
 <table><thead><tr><th>服务</th><th>最后实际检测</th><th>真实结果</th><th>保存与竞争</th></tr></thead><tbody><tr v-for="c in checks.checks" :key="c.service_id"><td>{{c.name}}</td><td>{{c.last_detected_at?date(c.last_detected_at):'未执行 / 无实际结果 / 未知'}}</td><td>{{c.last_detected_at?c.outcome:'未知'}}</td><td>{{c.persistence}} {{c.competing_probes?'竞争：'+c.competing_probes:''}}</td></tr></tbody></table></div>
 </details>
 <footer><div class="button-group"><button data-test="start" class="button primary" :disabled="busy||!status" @click="save(true)">保存并启动</button><button data-test="stop" class="button" :disabled="busy||!status?.running" @click="save(false)">停止定时采样</button></div></footer>
</article>
</template>
<style scoped>
.periodic-panel table{width:100%;font-size:12px;text-align:left}.periodic-panel td,.periodic-panel th{padding:6px;border-bottom:1px solid var(--border)}.periodic-panel select{max-width:100%}.periodic-panel{margin-bottom:20px}.periodic-fields{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;margin:16px 0}.periodic-fields label{display:flex;flex-direction:column;gap:6px}.periodic-progress{display:flex;flex-direction:column;gap:4px;padding:10px 0}.periodic-panel .muted{margin:12px 0;font-size:13px}
</style>
