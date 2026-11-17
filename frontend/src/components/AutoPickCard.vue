<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,reactive,ref,watch} from 'vue'
import {api} from '../api'
import {useWorkspace} from '../workspace'
import {date} from '../domain'
import {scheduleLabels,reportStatusLabels,groupServices,defaultRequiredServices,type AutoPickConfig,type AutoPickSchedule,type AutoPickStatus} from '../autoPick'
import Icon from './Icon.vue'

const w=useWorkspace()
const status=ref<AutoPickStatus|null>(null)
const loading=ref(true),saving=ref(false),error=ref(''),notice=ref('')
const emit=defineEmits<{state:[value:{status:AutoPickStatus|null;error:string}]}>()
watch([status,error],()=>emit('state',{status:status.value,error:error.value}),{immediate:true})
const confirmRun=ref(false),polling=ref<ReturnType<typeof setInterval>|null>(null)
const expanded=reactive({ai:false,streaming:false})
const form=reactive<AutoPickConfig>({enabled:false,schedule:'6h',daily_hour:6,airport_ids:[],region_filter:'',include_ai:false,include_streaming:false,ai_required:[...defaultRequiredServices.ai],streaming_required:[...defaultRequiredServices.streaming],min_response_rate:90,max_latency_ms:800,sample_count:6})
const baseline=ref('')

const airports=computed(()=>w.airports.value)
const missingRequired=computed(()=>(form.include_ai&&!form.ai_required.length)||(form.include_streaming&&!form.streaming_required.length))
const estimate=computed(()=>status.value?.estimate||null)
const dirty=computed(()=>baseline.value!==''&&canonical(form)!==baseline.value)
const canRun=computed(()=>!!status.value&&!status.value.running&&!saving.value&&!dirty.value&&(status.value.estimate?.nodes||0)>0)

// 只比较用户可编辑的字段；数组排序后再比，避免勾选顺序被当成修改。
function canonical(cfg:AutoPickConfig):string{
  return JSON.stringify({
    enabled:!!cfg.enabled,
    schedule:cfg.schedule,
    daily_hour:Number(cfg.daily_hour)||0,
    airport_ids:[...(cfg.airport_ids||[])].sort(),
    region_filter:(cfg.region_filter||'').trim(),
    include_ai:!!cfg.include_ai,
    include_streaming:!!cfg.include_streaming,
    ai_required:[...(cfg.ai_required||[])].sort(),
    streaming_required:[...(cfg.streaming_required||[])].sort(),
    min_response_rate:Number(cfg.min_response_rate)||0,
    max_latency_ms:Number(cfg.max_latency_ms)||0,
    sample_count:Number(cfg.sample_count)||0
  })
}
function fillForm(cfg:AutoPickConfig){
  Object.assign(form,{...cfg,airport_ids:[...(cfg.airport_ids||[])],ai_required:[...(cfg.ai_required||defaultRequiredServices.ai)],streaming_required:[...(cfg.streaming_required||defaultRequiredServices.streaming)]})
  baseline.value=canonical(form)
}
function apply(next:AutoPickStatus){status.value=next;fillForm(next.config)}
// 轮询只刷新运行状态与结果；用户正在编辑时不碰表单。
function applyStatus(next:AutoPickStatus){status.value=next;if(!dirty.value)fillForm(next.config)}
function errorMessage(e:unknown){return e instanceof Error?e.message:String(e)}
async function load(){
  loading.value=true;error.value=''
  try{
    const next=await api.autoPick()
    apply(next)
    if(next.running)startPolling()
  }catch(e){error.value=errorMessage(e)}finally{loading.value=false}
}
async function save(){
  if(saving.value)return
  if(missingRequired.value){error.value='勾选的组至少要选一个必需服务';return}
  saving.value=true;error.value='';notice.value=''
  try{
    const next=await api.saveAutoPick({...form,airport_ids:[...form.airport_ids],ai_required:[...form.ai_required],streaming_required:[...form.streaming_required],region_filter:form.region_filter||''})
    apply(next);notice.value=next.config.enabled?'设置已保存，将按时间自动运行':'设置已保存，自动运行保持关闭'
  }catch(e){error.value=errorMessage(e)}finally{saving.value=false}
}
function askRun(){if(!canRun.value)return;confirmRun.value=true}
async function run(){
  if(dirty.value){error.value='设置有修改，请先保存';confirmRun.value=false;return}
  if(!status.value||status.value.running){confirmRun.value=false;return}
  confirmRun.value=false
  error.value='';notice.value=''
  try{
    apply(await api.runAutoPick())
    notice.value='已开始运行；完成后上一轮结果会更新'
    startPolling()
  }catch(e){error.value=errorMessage(e)}
}
function startPolling(){
  if(polling.value)return
  polling.value=setInterval(async()=>{
    try{
      const next=await api.autoPick()
      applyStatus(next)
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
  <div class="section-heading auto-pick-header">
    <div><h2>自动精选与本地订阅</h2><p class="muted small">按设定周期遴选低延迟高可用节点，自动生成本地 Clash 订阅供客户端拉取。</p></div>
    <div class="auto-pick-head-controls">
      <label class="check-field auto-pick-toggle"><input v-model="form.enabled" type="checkbox">启用自动精选</label>
      <span class="badge" :class="form.enabled?'good':''">{{form.enabled?'已开启':'已关闭'}}</span>
    </div>
  </div>
  <p v-if="error" class="inline-error" role="alert">{{error}}</p>
  <p v-if="notice" class="muted small auto-pick-notice" role="status">{{notice}}</p>
  <p v-if="status?.last_error" class="inline-warning" role="alert">上一轮错误：{{status.last_error}}</p>

  <div class="auto-pick-bento">
    <div class="auto-pick-form">
      <div class="auto-pick-schedule-row">
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
        <label class="auto-pick-field">地区筛选（可选）
          <input v-model="form.region_filter" type="text" placeholder="如 新加坡；留空表示全部节点">
        </label>
      </div>
      <fieldset class="auto-pick-scope">
        <legend>机场范围</legend>
        <div class="auto-pick-airports">
          <label v-for="airport in airports" :key="airport.id" class="check-field">
            <input v-model="form.airport_ids" type="checkbox" :value="airport.id">{{airport.name}}
            <small class="muted">{{airport.node_count}} 个节点</small>
          </label>
          <p v-if="!airports.length" class="muted small">还没有机场，请先在“机场与账户”添加订阅。</p>
        </div>
      </fieldset>
      <fieldset class="auto-pick-projects">
        <legend>检测项目与准入阈值</legend>
        <div class="auto-pick-project-pills">
          <label class="check-field"><input type="checkbox" checked disabled>延迟（必选）</label>
          <div class="auto-pick-group-head">
            <label class="check-field"><input v-model="form.include_ai" type="checkbox">AI 组</label>
            <button v-if="form.include_ai" type="button" class="text-button" @click="expanded.ai=!expanded.ai">
              必需 {{form.ai_required.length}} 项{{expanded.ai?' · 收起':' · 展开'}}
            </button>
          </div>
          <div class="auto-pick-group-head">
            <label class="check-field"><input v-model="form.include_streaming" type="checkbox">流媒体组</label>
            <button v-if="form.include_streaming" type="button" class="text-button" @click="expanded.streaming=!expanded.streaming">
              必需 {{form.streaming_required.length}} 项{{expanded.streaming?' · 收起':' · 展开'}}
            </button>
          </div>
        </div>
        <div v-if="form.include_ai&&expanded.ai" class="auto-pick-services">
          <label v-for="service in groupServices.ai" :key="service.id" class="check-field">
            <input v-model="form.ai_required" type="checkbox" :value="service.id">{{service.label}}
          </label>
          <p class="muted small">只检测勾上的必需服务。Gemini 和 Antigravity 常卡在验证页或凭据上，默认不要求。</p>
        </div>
        <div v-if="form.include_streaming&&expanded.streaming" class="auto-pick-services">
          <label v-for="service in groupServices.streaming" :key="service.id" class="check-field">
            <input v-model="form.streaming_required" type="checkbox" :value="service.id">{{service.label}}
          </label>
          <p class="muted small">只检测勾上的必需服务。默认只要求 Netflix 完整片库，“仅自制剧”算没通过。</p>
        </div>
        <p v-if="missingRequired" class="inline-warning">勾选的组至少要保留一个必需服务。</p>
        <div class="auto-pick-rules">
          <label class="auto-pick-field">响应率 ≥ <input v-model.number="form.min_response_rate" type="number" min="50" max="100" step="1"> %</label>
          <label class="auto-pick-field">稳定延迟 ≤ <input v-model.number="form.max_latency_ms" type="number" min="100" max="5000" step="50"> ms</label>
          <small class="muted">稳定延迟取去掉首次采样后的中位数；解锁没通过只影响用途组，节点仍可进日常组。</small>
        </div>
      </fieldset>
    </div>

    <div class="auto-pick-side">
      <div class="auto-pick-estimate-wrap">
        <p class="auto-pick-estimate" :class="{'auto-pick-warn':!dirty&&estimate?.warn}" role="status">
          <template v-if="!dirty&&estimate">本轮 {{estimate.nodes}} 个节点 · 每节点 {{estimate.projects}} 项检测 · 共 <strong>{{estimate.requests}}</strong> 项检测<span v-if="estimate.warn">，超过 500 项，建议缩小机场或地区范围</span></template>
          <template v-else>保存后更新范围与数量</template>
        </p>
        <p v-if="!dirty&&status" class="muted small auto-pick-sample">延迟每节点采样 {{status.config.sample_count}} 次；解锁项目可能包含多次请求。</p>
      </div>

      <p v-if="dirty" class="inline-warning auto-pick-dirty" role="status">设置有修改，请先保存</p>

      <div class="auto-pick-actions">
        <button class="button primary" :disabled="saving" @click="save">{{saving?'保存中…':'保存设置'}}</button>
        <button class="button" :disabled="!canRun" :title="dirty?'先保存设置':''" @click="askRun">立即运行一次</button>
        <span class="muted small">下次运行：{{status?.config.next_run_at?date(status.config.next_run_at):(form.enabled?'保存后计算':'未开启')}}</span>
      </div>
      <div v-if="confirmRun&&estimate" class="auto-pick-confirm">
        <p>将检测 {{estimate.nodes}} 个节点、共 {{estimate.requests}} 项检测，可能持续数分钟。</p>
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
        <div class="auto-pick-sub-head"><strong>本地订阅地址</strong><button class="text-button" :disabled="!status?.subscription.exists" @click="copyURL">复制链接</button></div>
        <code>{{status?.subscription.url||'正在读取…'}}</code>
        <p class="muted small">在 Clash Verge 里新建订阅，填这个地址，设成每 6 小时更新。</p>
        <p v-if="status&&!status.subscription.exists" class="muted small">首次成功运行后才会生成订阅文件。</p>
        <p v-else-if="status?.subscription.updated_at" class="muted small">文件更新于 {{date(status.subscription.updated_at)}} · {{status.subscription.node_count}} 个节点</p>
      </div>
      <div class="auto-pick-footnotes">
        <p class="muted small auto-pick-lifetime">自动精选的延迟只测 Cloudflare 一站，所以数字可能和工作台四站的稳定延迟略有不同。</p>
        <p class="muted small auto-pick-lifetime">定时任务和订阅地址只在 CST 运行时有效。</p>
      </div>
    </div>
  </div>
</section>
</template>

<style scoped>
.auto-pick-card{display:grid;gap:14px}
.auto-pick-header{padding-bottom:12px;border-bottom:1px solid var(--cst-line)}
.auto-pick-head-controls{display:flex;align-items:center;gap:12px}
.auto-pick-toggle{margin:0;font-weight:600;color:var(--cst-text)}
.auto-pick-notice{padding:6px 10px;border-radius:7px;background:var(--cst-accent-soft);color:var(--cst-accent);margin:0}
.auto-pick-bento{display:grid;grid-template-columns:minmax(0,1.35fr) minmax(300px,1fr);gap:18px;align-items:start}
@media(max-width:980px){.auto-pick-bento{grid-template-columns:1fr}}
.auto-pick-form,.auto-pick-side{display:grid;gap:12px}
.auto-pick-schedule-row{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px}
.auto-pick-field{display:grid;gap:5px;font-size:12px;font-weight:550;color:var(--cst-muted)}
.auto-pick-field select,.auto-pick-field input[type=text],.auto-pick-field input[type=number]{min-height:34px;padding:6px 10px;border:1px solid var(--cst-line-strong);border-radius:8px;background:var(--cst-panel);color:var(--cst-text);font-size:13px}
.auto-pick-scope,.auto-pick-projects{display:grid;gap:10px;border:1px solid var(--cst-line);border-radius:10px;padding:12px 14px;background:var(--cst-panel-2)}
.auto-pick-scope legend,.auto-pick-projects legend{padding:0 6px;font-size:12px;font-weight:650;color:var(--cst-muted)}
.auto-pick-project-pills{display:flex;flex-wrap:wrap;gap:10px 18px;align-items:center}
.auto-pick-group-head{display:flex;align-items:center;gap:8px}
.auto-pick-services{display:flex;flex-wrap:wrap;gap:8px 16px;padding:10px 12px;border:1px dashed var(--cst-line-strong);border-radius:8px;background:var(--cst-panel)}
.auto-pick-services p{flex:1 0 100%;margin:0}
.auto-pick-airports{display:grid;gap:6px;grid-template-columns:repeat(auto-fit,minmax(180px,1fr))}
.auto-pick-airports small{margin-left:6px}
.auto-pick-rules{display:flex;flex-wrap:wrap;gap:12px;align-items:center;padding-top:8px;border-top:1px solid var(--cst-line)}
.auto-pick-rules .auto-pick-field{display:flex;align-items:center;gap:6px;white-space:nowrap}
.auto-pick-rules input[type=number]{width:80px}
.auto-pick-estimate-wrap{display:grid;gap:6px}
.auto-pick-estimate{margin:0;padding:11px 14px;border-radius:9px;background:var(--cst-panel-2);border:1px solid var(--cst-line);font-size:13px}
.auto-pick-estimate.auto-pick-warn{border-color:var(--cst-warn);background:var(--cst-warn-bg);color:var(--cst-warn)}
.auto-pick-sample,.auto-pick-dirty{margin:0}
.auto-pick-actions{display:flex;flex-wrap:wrap;gap:8px;align-items:center}
.auto-pick-confirm{padding:12px 14px;border:1px solid var(--cst-warn);border-radius:9px;background:var(--cst-warn-bg)}
.auto-pick-confirm p{margin:0 0 8px}
.auto-pick-running{display:flex;align-items:center;gap:8px;margin:0;color:var(--cst-accent);font-weight:550}
.auto-pick-last{display:flex;justify-content:space-between;gap:12px;align-items:center;padding:10px 12px;border:1px solid var(--cst-line);border-radius:9px;background:var(--cst-panel-2);font-size:13px}
.auto-pick-last .badge{margin-right:8px}
.auto-pick-sub{display:grid;gap:6px;padding:12px 14px;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-2)}
.auto-pick-sub-head{display:flex;justify-content:space-between;align-items:center}
.auto-pick-sub code{word-break:break-all;font:12px/1.45 var(--cst-mono);background:var(--cst-panel);border:1px solid var(--cst-line);padding:7px 10px;border-radius:7px;color:var(--cst-accent)}
.auto-pick-sub p{margin:0}
.auto-pick-footnotes{display:grid;gap:2px;padding-top:4px}
.auto-pick-lifetime{margin:0;font-size:11.5px;opacity:.85}
</style>
