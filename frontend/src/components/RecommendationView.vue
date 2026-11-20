<script setup lang="ts">
import {computed,onMounted,ref} from 'vue'
import {useWorkspace} from '../workspace'
import {api,saveFile} from '../api'
import {date,isNotice,key,scope,type NodeOption} from '../domain'
import {type QualityEvidence} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {candidateDiff,isRecommendation,purposes,recommend,recommendationRegion,recommendationRegions,recommendationService,recommendationServices,type Purpose,type ServicePurpose,type Candidate} from '../recommendations'
import {nodeDisplayName,nodeRegionLabel} from '../nodePresentation'
import UiSelect from './UiSelect.vue'

const w=useWorkspace(),loading=ref(false),error=ref(''),data=ref<Record<string,QualityEvidence>>({}),readAt=ref('')
const nodes=computed(()=>w.nodes.value.filter(n=>!isNotice(n)))
const activePurpose=ref<Purpose>('daily'),selectedServices=ref<Partial<Record<ServicePurpose,string>>>({})
const isServicePurpose=(purpose:Purpose):purpose is ServicePurpose=>purpose==='ai'||purpose==='streaming'
const serviceOptions=computed(()=>isServicePurpose(activePurpose.value)?recommendationServices(w.catalog.value,activePurpose.value):[])
const activeService=computed(()=>isServicePurpose(activePurpose.value)?recommendationService(serviceOptions.value,activePurpose.value,selectedServices.value[activePurpose.value]):undefined)
const results=computed(()=>recommend(nodes.value,data.value,w.catalog.value,selectedServices.value))
const regions=computed(()=>recommendationRegions(nodes.value).map(code=>{
 const regionNodes=nodes.value.filter(n=>recommendationRegion(n)===code)
 return {
  code,label:nodeRegionLabel(code),nodes:regionNodes,picks:results.value[activePurpose.value].filter(candidate=>candidate.region===code),
 }
}))
const recommended=computed(()=>results.value[activePurpose.value].filter(isRecommendation))
const regionCount=computed(()=>regions.value.filter(region=>region.code!=='unknown').length)
const readyRegionCount=computed(()=>new Set(recommended.value.map(candidate=>candidate.region)).size)
const purposeName=computed(()=>purposes.find(purpose=>purpose.id===activePurpose.value)!.name)

const standards:Record<Purpose,string>={
 daily:'优先采用同条件四站综合（Cloudflare / Google / GitHub / Apple）记录，兼容单站历史。存在完全不通站点的线路不会作为推荐；其余按响应率、中位延迟与波动排序。',
 download:'优先使用速度已稳定的读数，其次用完整定量测速。同条件下速度高的优先。',
 ai:'所选 AI 服务最近一次检测通过（默认优先 Gemini）。条件相同时，通过次数多的优先，其次看最近检测时间。',
 streaming:'所选流媒体或地区解锁（含 Netflix、TikTok、DLsite 与日本服务）最近一次探针通过。条件相同时，通过次数多的优先，其次看最近检测时间。',
}
const missing:Record<Purpose,string>={
 daily:'缺少足够的延迟响应记录（至少 5 次成功采样）',
 download:'暂无有效下载读数',
 ai:'暂无所选服务的通过记录',
 streaming:'暂无所选服务的解锁记录',
}
const configurationNote:Record<Candidate['configuration'],string>={
 current:'',historical:'依据来自旧配置，当前配置待测',mixed:'依据包含多个配置版本，当前配置待测',unknown:'历史配置标识不足，当前配置待测',
}

async function refresh(){
 loading.value=true;error.value='';data.value={}
 const until=new Date().toISOString(),since=new Date(Date.parse(until)-7*86400000).toISOString()
 try{
  data.value=await readQualityBatch(nodes.value,since,until)
  readAt.value=until
  if(Object.values(data.value).some(e=>!e.complete))error.value='部分历史读取不完整，推荐使用已核对的记录。'
 }catch(e){error.value=e instanceof Error?e.message:String(e)}
 finally{loading.value=false}
}
async function exportCandidates(){
 const purpose=activePurpose.value,candidates=recommended.value,name=activeService.value?.name||purposeName.value
 try{
  const result=await api.exportClash(candidates.map(c=>scope(c.node)),name+'地区推荐')
  if(await saveFile(result.yaml_content,'speedtest-'+purpose+'.yaml','application/x-yaml'))w.notify(`已导出 ${result.node_count} 条地区推荐`)
 }catch(e){w.fail(e)}
}
function selectService(value:string|number){
 const purpose=activePurpose.value
 if(isServicePurpose(purpose))selectedServices.value[purpose]=String(value)
}
function projectFor(purpose:Purpose){return purpose==='daily'?'latency':purpose==='download'?'download':'service'}
function inspect(candidate:Candidate){
 const purpose=activePurpose.value
 w.hours.value=168;w.project.value=projectFor(purpose)
 w.inspectNode.value={...candidate.node,...candidate.inspectionScope,historyServiceId:candidate.inspectionServiceId,historyTargetId:purpose==='daily'?candidate.inspectionTargetId:undefined}
}
function openInWorkbench(node:NodeOption){
 w.project.value=projectFor(activePurpose.value)
 if(w.viewRequest)w.viewRequest.value=node
 w.page.value='home'
}
function canPlanRegion(region:{code:string;nodes:NodeOption[];picks:Candidate[]}){
 if(region.code==='unknown'||!region.nodes.length)return false
 if(isServicePurpose(activePurpose.value)&&!activeService.value)return false
 return !region.picks.some(isRecommendation)||(region.picks.length===1&&region.nodes.length>1)
}
function openRegionPlan(region:{nodes:NodeOption[]}){
 const purpose=activePurpose.value
 if(typeof w.openPlan!=='function'){w.project.value=projectFor(purpose);w.page.value='home';return}
 w.project.value=projectFor(purpose)
 w.openPlan(region.nodes)
 if(!w.plan?.value)return
 w.plan.value.includeReputation=false
 w.plan.value.reputationServiceIds=[]
 if(purpose==='daily'){w.plan.value.projects=['latency'];w.plan.value.target='all';w.plan.value.samples=6}
 else if(purpose==='download'){w.plan.value.projects=['download']}
 else if(activeService.value){w.plan.value.projects=['service'];w.plan.value.serviceIds=[activeService.value.service_id]}
}
function secondPickNote(picks:Candidate[],index:number){
 if(!index||picks.length<2)return ''
 const first=picks[0]!,second=picks[index]!
 if(second.condition!==first.condition)return '检测条件不同'
 return candidateDiff(first,second,activePurpose.value)
}
onMounted(()=>void refresh())
</script>

<template>
 <section class="page-view recommendation-page">
  <header class="page-header">
   <div>
    <h1>为你选线</h1>
    <p>按用途看各地区的首选和次选。依据近 7 天记录<template v-if="readAt">，截至 {{date(readAt)}}</template>。</p>
   </div>
   <button class="button" :disabled="loading" @click="refresh">{{loading?'读取中…':'刷新推荐'}}</button>
  </header>
  <p v-if="error" class="inline-warning" role="alert">{{error}}</p>
  <p v-if="loading" class="muted" role="status">正在读取历史…</p>
  <section v-else-if="!nodes.length" class="settings-section recommendation-empty">
   <div><strong>还没有可选线路</strong><p class="muted small">在工作台添加订阅后，可按地区查看推荐。</p></div>
   <button class="button primary" @click="w.page.value='home'">去工作台</button>
  </section>
  <template v-else>
   <nav class="recommendation-tabs" aria-label="选线用途">
    <button v-for="purpose in [...purposes].reverse()" :key="purpose.id" :aria-pressed="activePurpose===purpose.id" @click="activePurpose=purpose.id">{{purpose.name}}</button>
   </nav>

   <section class="purpose-results" :aria-label="purposeName+'地区推荐'">
    <div class="recommendation-criteria">
     <div>
      <label v-if="serviceOptions.length" class="recommendation-service">选择服务
       <UiSelect :model-value="activeService?.service_id" @update:model-value="selectService">
        <option v-for="rule in serviceOptions" :key="rule.service_id" :value="rule.service_id">{{rule.name}}</option>
       </UiSelect>
      </label>
      <p>{{standards[activePurpose]}}</p>
      <details class="recommendation-method">
       <summary>怎样选</summary>
       <p>地区按节点标注分组。每区优先用当前配置的证据。延迟和下载优先采用已核验出口的记录。按同条件组比较，覆盖不同线路较多的组优先，同数时取记录较新的组。次选优先沿用首选的检测条件。导出使用已推荐线路的当前配置。</p>
      </details>
     </div>
     <div class="recommendation-export">
      <small class="muted">{{readyRegionCount}} / {{regionCount}} 个地区有推荐</small>
      <button v-if="recommended.length" class="button" @click="exportCandidates">导出 Clash 配置组</button>
      <button v-else class="text-button" @click="w.page.value='home'">去工作台检测</button>
     </div>
    </div>

    <div class="region-results">
     <section v-for="region in regions" :key="region.code" class="region-result" :aria-label="region.label+'推荐'">
      <div class="region-head">
       <h3>{{region.label}}</h3>
       <button v-if="canPlanRegion(region)" class="text-button region-plan-btn" @click="openRegionPlan(region)">补测本区（{{region.nodes.length}}）</button>
      </div>
      <div class="region-options">
       <p v-if="!region.picks.length" class="region-note">{{region.code==='unknown'?'地区待确认，可在线路工作台查看':isServicePurpose(activePurpose)&&!activeService?'暂无可选服务':missing[activePurpose]}}</p>
       <article v-for="(candidate,index) in region.picks" :key="key(candidate.node)" class="region-candidate">
        <div class="candidate-slot" :class="{'candidate-reference':!isRecommendation(candidate)}">
         {{!isRecommendation(candidate)?'待复测':index?'次选':'首选'}}
        </div>
        <div class="candidate-evidence">
         <div class="candidate-title">
          <strong>{{nodeDisplayName(candidate.node)}}</strong>
          <small>{{w.nodeAirportName(candidate.node)}} · {{w.nodeSubscriptionName(candidate.node)}}</small>
          <span v-if="candidate.cfChallenge" class="cf-chip" :class="candidate.cfChallenge.tone" :title="candidate.cfChallenge.detail">{{candidate.cfChallenge.label}}</span>
         </div>
         <p>{{candidate.basis}}</p>
         <small>{{candidate.count}} {{activePurpose==='daily'?'次采样':isServicePurpose(activePurpose)?'次规则通过':'次同条件读数'}} · 最近 {{date(candidate.at)}}<template v-if="secondPickNote(region.picks,index)"> · {{secondPickNote(region.picks,index)}}</template></small>
         <p v-if="configurationNote[candidate.configuration]" class="candidate-reference">{{configurationNote[candidate.configuration]}}</p>
        </div>
        <div class="candidate-actions">
         <button class="text-button" @click="inspect(candidate)">查看记录</button>
         <button class="text-button" @click="openInWorkbench(candidate.node)">在线路查看</button>
        </div>
       </article>
       <p v-if="region.picks.length===1&&isRecommendation(region.picks[0])" class="region-note">次选待测</p>
      </div>
     </section>
    </div>
   </section>
  </template>
 </section>
</template>

<style scoped>
.recommendation-page .page-header p{font-size:13px}
.recommendation-empty{display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:12px}
.recommendation-tabs{display:flex;gap:24px;flex-wrap:wrap;border-bottom:1px solid var(--cst-line)}
.recommendation-tabs button{padding:11px 5px;border:0;border-bottom:2px solid transparent;background:none;color:var(--cst-muted);cursor:pointer;font-size:14px}
.recommendation-tabs button[aria-pressed=true]{font-weight:650;color:var(--cst-text);border-color:var(--cst-accent)}
.recommendation-criteria{display:flex;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;gap:14px;padding:18px 0;border-bottom:1px solid var(--cst-line)}
.recommendation-criteria p,.recommendation-method{font-size:13px;line-height:1.65;color:var(--cst-muted)}
.recommendation-criteria p{margin:0}
.recommendation-method{margin-top:6px;max-width:62ch}
.recommendation-method summary{width:fit-content;cursor:pointer;color:var(--cst-accent)}
.recommendation-service{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:8px;font-size:13px}
.recommendation-service .ui-select{max-width:min(100%,360px)}
.recommendation-export{display:flex;align-items:center;gap:10px;flex-wrap:wrap;flex:none}
.region-result{display:grid;grid-template-columns:minmax(110px,17%) minmax(0,1fr);gap:18px;padding:15px 0;border-bottom:1px solid var(--cst-line)}
.region-head{display:flex;flex-direction:column;align-items:flex-start;gap:6px}
.region-result h3{font-size:16px;font-weight:650;margin:1px 0 0;overflow-wrap:anywhere}
.region-plan-btn{font-size:12px;padding:0}
.region-options{min-width:0}
.region-candidate{display:grid;grid-template-columns:38px minmax(0,1fr) auto;gap:12px;align-items:start;padding:3px 0 9px}
.region-candidate+.region-candidate{padding-top:12px;border-top:1px solid var(--cst-line)}
.candidate-slot{font-size:12px;color:var(--cst-accent);font-weight:650;padding-top:2px}
.candidate-reference{color:var(--cst-warn)!important}
.candidate-title{display:flex;align-items:center;gap:4px 10px;flex-wrap:wrap;overflow-wrap:anywhere}
.candidate-title strong{font-size:14px;font-weight:650}
.cf-chip{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;font-size:11px;font-weight:600;line-height:1.5;border:1px solid transparent}
.cf-chip.good{color:var(--cst-good,#15803d);background:color-mix(in srgb,var(--cst-good,#15803d) 10%,transparent);border-color:color-mix(in srgb,var(--cst-good,#15803d) 24%,transparent)}
.cf-chip.warn{color:var(--cst-warn,#b45309);background:color-mix(in srgb,var(--cst-warn,#b45309) 12%,transparent);border-color:color-mix(in srgb,var(--cst-warn,#b45309) 28%,transparent)}
.cf-chip.bad{color:var(--cst-bad,#b91c1c);background:color-mix(in srgb,var(--cst-bad,#b91c1c) 10%,transparent);border-color:color-mix(in srgb,var(--cst-bad,#b91c1c) 26%,transparent)}
.candidate-evidence p{margin:4px 0;font-size:13px;line-height:1.5;overflow-wrap:anywhere}
.candidate-evidence small,.region-note{font-size:12px;line-height:1.6;color:var(--cst-muted)}
.region-note{margin:2px 0}
.candidate-actions{display:flex;gap:8px 14px;flex-wrap:wrap;font-size:12px}
@media(max-width:760px){
 .region-result{grid-template-columns:minmax(0,1fr);gap:9px}
 .region-candidate{grid-template-columns:36px minmax(0,1fr);gap:6px 10px}
 .candidate-actions{grid-column:2}
}
</style>
