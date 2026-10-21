<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,reactive,ref} from 'vue'
import {api} from '../api'
import {useWorkspace} from '../workspace'
import {date} from '../domain'
import {scheduleLabels,reportStatusLabels,type AutoPickConfig,type AutoPickSchedule,type AutoPickStatus} from '../autoPick'
import Icon from './Icon.vue'

const w=useWorkspace()
const status=ref<AutoPickStatus|null>(null)
const loading=ref(true),saving=ref(false),error=ref(''),notice=ref('')
const confirmRun=ref(false),polling=ref<ReturnType<typeof setInterval>|null>(null)
const form=reactive<AutoPickConfig>({enabled:false,schedule:'6h',daily_hour:6,airport_ids:[],region_filter:'',include_ai:false,include_streaming:false,min_response_rate:90,max_latency_ms:800,sample_count:6})

const airports=computed(()=>w.airports.value)
const projects=computed(()=>1+(form.include_ai?3:0)+(form.include_streaming?3:0))
const estimatedNodes=computed(()=>airports.value.filter(a=>form.airport_ids.includes(a.id)).reduce((sum,a)=>sum+(a.node_count||0),0))
const estimatedRequests=computed(()=>estimatedNodes.value*projects.value)
const estimateWarn=computed(()=>estimatedRequests.value>500)
const canRun=computed(()=>!status.value?.running&&!saving.value&&form.airport_ids.length>0&&estimatedNodes.value>0)

function apply(next:AutoPickStatus){
  status.value=next
  Object.assign(form,{...next.config,airport_ids:[...(next.config.airport_ids||[])]})
}
function errorMessage(e:unknown){return e instanceof Error?e.message:String(e)}
async function load(){
  loading.value=true;error.value=''
  try{apply(await api.autoPick())}catch(e){error.value=errorMessage(e)}finally{loading.value=false}
}
async function save(){
  if(saving.value)return
  saving.value=true;error.value='';notice.value=''
  try{
    const next=await api.saveAutoPick({...form,airport_ids:[...form.airport_ids],region_filter:form.region_filter||''})
    apply(next);notice.value=next.config.enabled?'设置已保存，将按时间自动运行':'设置已保存，自动运行保持关闭'
  }catch(e){error.value=errorMessage(e)}finally{saving.value=false}
}
function askRun(){confirmRun.value=true}
async function run(){
  confirmRun.value=false
  error.value='';notice.value=''
  try{
    apply(await api.runAutoPick())
    notice.value='已开始运行；完成后上一轮结果会更新'
    startPolling()
  }catch(e){error.value=errorMessage(e)}
}
function startPolling(){
  stopPolling()
  polling.value=setInterval(async()=>{
    try{
      const next=await api.autoPick()
      apply(next)
      if(!next.running){stopPolling();notice.value='本轮已结束，可查看报告'}
    }catch{/* 下一次轮询继续 */}
  },3000)
}
function stopPolling(){if(polling.value){clearInterval(polling.value);polling.value=null}}
async function copyURL(){
  const url=status.value?.subscription.url
  if(!url)return
  try{
    if(navigator.clipboard?.writeText)await navigator.clipboard.writeText(url)
    else{const area=document.createElement('textarea');area.value=url;document.body.appendChild(area);area.select();document.execCommand('copy');area.remove()}
    notice.value='订阅地址已复制'
  }catch(e){error.value='复制失败：'+errorMessage(e)}
}
function openReport(id?:string){if(id)w.openAutoPickReport(id)}
onMounted(()=>{void load()})
onBeforeUnmount(stopPolling)
</script>

<template>
<section class="settings-section auto-pick-card" :aria-busy="loading||saving">
  <div class="section-heading">
    <div><h2>自动精选</h2><p class="muted small">按时间挑选节点，生成 Clash 配置与本地订阅；默认关闭，不弹窗。</p></div>
    <span class="badge" :class="form.enabled?'good':''">{{form.enabled?'已开启':'已关闭'}}</span>
  </div>
  <p v-if="error" class="inline-error" role="alert">{{error}}</p>
  <p v-if="notice" class="muted small" role="status">{{notice}}</p>
  <p v-if="status?.last_error" class="inline-warning" role="alert">上一轮错误：{{status.last_error}}</p>

  <div class="auto-pick-form">
    <label class="check-field auto-pick-toggle"><input v-model="form.enabled" type="checkbox">启用自动精选</label>
    <label class="auto-pick-field">运行时间
      <select v-model="form.schedule">
        <option v-for="(label,id) in scheduleLabels" :key="id" :value="id">{{label}}</option>
      </select>
    </label>
    <label v-if="form.schedule==='daily'" class="auto-pick-field">每天
      <select v-model.number="form.daily_hour">
        <option v-for="hour in 24" :key="hour" :value="hour-1">{{String(hour-1).padStart(2,'0')}}:00</option>
      </select>
    </label>
    <fieldset class="auto-pick-scope">
      <legend>范围</legend>
      <div class="auto-pick-airports">
        <label v-for="airport in airports" :key="airport.id" class="check-field">
          <input v-model="form.airport_ids" type="checkbox" :value="airport.id">{{airport.name}}
          <small class="muted">{{airport.node_count}} 个节点</small>
        </label>
        <p v-if="!airports.length" class="muted small">还没有机场，请先在“机场与账户”添加订阅。</p>
      </div>
      <label class="auto-pick-field">地区筛选（可选）
        <input v-model="form.region_filter" type="text" placeholder="如 新加坡；留空表示全部节点">
      </label>
    </fieldset>
    <fieldset class="auto-pick-projects">
      <legend>检测项目</legend>
      <label class="check-field"><input type="checkbox" checked disabled>延迟（必选）</label>
      <label class="check-field"><input v-model="form.include_ai" type="checkbox">AI 组（ChatGPT／Claude／Gemini）</label>
      <label class="check-field"><input v-model="form.include_streaming" type="checkbox">流媒体组（Netflix／YouTube／Disney+）</label>
    </fieldset>
    <div class="auto-pick-rules">
      <label class="auto-pick-field">响应率 ≥ <input v-model.number="form.min_response_rate" type="number" min="50" max="100" step="1"> %</label>
      <label class="auto-pick-field">稳定延迟 ≤ <input v-model.number="form.max_latency_ms" type="number" min="100" max="5000" step="50"> ms</label>
      <small class="muted">稳定延迟取中位数；勾选解锁组后，节点还须通过所选组的全部服务。</small>
    </div>
  </div>

  <p class="auto-pick-estimate" :class="{'auto-pick-warn':estimateWarn}" role="status">
    本轮约 {{estimatedNodes}} 个节点 × {{projects}} 个项目 ≈ <strong>{{estimatedRequests}}</strong> 个请求
    <span v-if="estimateWarn">，超过 500，建议缩小机场或地区范围</span>
  </p>

  <div class="auto-pick-actions">
    <button class="button primary" :disabled="saving" @click="save">{{saving?'保存中…':'保存设置'}}</button>
    <button class="button" :disabled="!canRun" @click="askRun">立即运行一次</button>
    <span class="muted small">下次运行：{{status?.config.next_run_at?date(status.config.next_run_at):(form.enabled?'保存后计算':'未开启')}}</span>
  </div>
  <div v-if="confirmRun" class="auto-pick-confirm">
    <p>将检测 {{estimatedNodes}} 个节点、约 {{estimatedRequests}} 个请求，可能持续数分钟。</p>
    <div class="button-group"><button class="button primary" @click="run">确认运行</button><button class="button ghost" @click="confirmRun=false">取消</button></div>
  </div>
  <p v-if="status?.running" class="auto-pick-running" role="status"><Icon name="refresh"/>正在运行本轮精选…</p>

  <div class="auto-pick-last">
    <template v-if="status?.last_report">
      <span><span class="badge" :class="status.last_report.kept_previous?'warn':status.last_report.status==='selected'?'good':''">{{reportStatusLabels[status.last_report.status]||status.last_report.status}}</span>{{status.last_report.summary}}</span>
      <button class="text-button" @click="openReport(status.last_report.report_id)">查看报告 ›</button>
    </template>
    <span v-else class="muted small">还没有自动精选记录。</span>
  </div>

  <div class="auto-pick-sub">
    <div class="auto-pick-sub-head"><strong>本地订阅地址</strong><button class="text-button" :disabled="!status?.subscription.exists" @click="copyURL">复制</button></div>
    <code>{{status?.subscription.url||'正在读取…'}}</code>
    <p class="muted small">在 Clash Verge 里新建订阅，填这个地址，设成每 6 小时更新。</p>
    <p v-if="status&&!status.subscription.exists" class="muted small">首次成功运行后才会生成订阅文件。</p>
    <p v-else-if="status?.subscription.updated_at" class="muted small">文件更新于 {{date(status.subscription.updated_at)}} · {{status.subscription.node_count}} 个节点</p>
  </div>
</section>
</template>

<style scoped>
.auto-pick-card{display:grid;gap:14px}
.auto-pick-form{display:grid;gap:14px;grid-template-columns:repeat(auto-fit,minmax(220px,1fr))}
.auto-pick-toggle{align-self:end}
.auto-pick-field{display:grid;gap:6px;font-size:13px;color:var(--cst-muted)}
.auto-pick-field select,.auto-pick-field input[type=text],.auto-pick-field input[type=number]{min-height:34px;padding:6px 10px;border:1px solid var(--cst-line);border-radius:8px;background:var(--cst-panel);color:var(--cst-text)}
.auto-pick-scope,.auto-pick-projects{grid-column:1/-1;display:grid;gap:8px;border:1px solid var(--cst-line);border-radius:10px;padding:12px}
.auto-pick-scope legend,.auto-pick-projects legend{padding:0 6px;font-size:13px;color:var(--cst-muted)}
.auto-pick-airports{display:grid;gap:6px;grid-template-columns:repeat(auto-fit,minmax(200px,1fr))}
.auto-pick-airports small{margin-left:6px}
.auto-pick-rules{grid-column:1/-1;display:flex;flex-wrap:wrap;gap:14px;align-items:end}
.auto-pick-estimate{margin:0;padding:10px 12px;border-radius:8px;background:var(--cst-panel);border:1px solid var(--cst-line);font-size:13px}
.auto-pick-estimate.auto-pick-warn{border-color:var(--cst-warn);color:var(--cst-warn)}
.auto-pick-actions{display:flex;flex-wrap:wrap;gap:10px;align-items:center}
.auto-pick-confirm{padding:10px 12px;border:1px solid var(--cst-warn);border-radius:8px;background:var(--cst-panel)}
.auto-pick-confirm p{margin:0 0 8px}
.auto-pick-running{display:flex;align-items:center;gap:8px;margin:0}
.auto-pick-last{display:flex;justify-content:space-between;gap:12px;align-items:center;padding-top:10px;border-top:1px solid var(--cst-line);font-size:13px}
.auto-pick-last .badge{margin-right:8px}
.auto-pick-sub{display:grid;gap:6px;padding:12px;border:1px dashed var(--cst-line);border-radius:10px}
.auto-pick-sub-head{display:flex;justify-content:space-between;align-items:center}
.auto-pick-sub code{word-break:break-all;font-size:12px;background:var(--cst-panel);padding:6px 8px;border-radius:6px}
.auto-pick-sub p{margin:0}
</style>
