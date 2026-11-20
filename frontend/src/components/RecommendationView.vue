<script setup lang="ts">
import {computed,onMounted,ref} from 'vue'
import {useWorkspace} from '../workspace'
import {api,saveFile} from '../api'
import {date,isNotice,key,scope,type NodeOption} from '../domain'
import {type QualityEvidence} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {candidateDiff,isRecommendation,purposes,recommend,recommendationRegion,recommendationRegions,recommendationService,recommendationServices,type Purpose,type ServicePurpose,type Candidate} from '../recommendations'
import {nodeDisplayName,nodeRegionLabel,regionBadgeClass} from '../nodePresentation'
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
const activeRegions=computed(()=>regions.value.filter(r=>r.picks.length>0).sort((a,b)=>Number(b.picks.some(isRecommendation))-Number(a.picks.some(isRecommendation))||b.picks.length-a.picks.length))
const emptyRegions=computed(()=>regions.value.filter(r=>!r.picks.length))
const recommended=computed(()=>results.value[activePurpose.value].filter(isRecommendation))
const regionCount=computed(()=>regions.value.filter(region=>region.code!=='unknown').length)
const readyRegionCount=computed(()=>new Set(recommended.value.map(candidate=>candidate.region)).size)
const coveragePercent=computed(()=>regionCount.value?Math.round(readyRegionCount.value/regionCount.value*100):0)
const purposeName=computed(()=>purposes.find(purpose=>purpose.id===activePurpose.value)!.name)
function candidateSource(node:NodeOption){
 const airport=w.nodeAirportName(node),sub=w.nodeSubscriptionName(node)
 return airport&&sub&&airport!==sub?`${airport} · ${sub}`:airport||sub||node.profile_name||'当前来源'
}
function candidateHero(candidate:Candidate){
 if(activePurpose.value==='daily')return `${Math.round(candidate.score)} ms`
 if(activePurpose.value==='download')return `${candidate.score.toFixed(2)} MiB/s`
 return '已通过'
}

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
   <div class="recommendation-hero-card">
    <div class="recommendation-hero-top">
     <nav class="recommendation-tabs" aria-label="选线用途">
      <button v-for="purpose in [...purposes].reverse()" :key="purpose.id" :aria-pressed="activePurpose===purpose.id" @click="activePurpose=purpose.id">{{purpose.name}}</button>
     </nav>
     <div class="recommendation-export">
      <div class="coverage-meter" :title="`${readyRegionCount} / ${regionCount} 个地区有可用推荐`">
       <span><strong>{{readyRegionCount}}</strong> / {{regionCount}} 个地区有推荐</span>
       <div class="coverage-track"><i :style="{width:coveragePercent+'%'}"/></div>
      </div>
      <button v-if="recommended.length" class="button primary" @click="exportCandidates">导出 Clash 配置组</button>
      <button v-else class="button" @click="w.page.value='home'">去工作台检测</button>
     </div>
    </div>

    <div class="recommendation-criteria">
     <div class="criteria-main">
      <label v-if="serviceOptions.length" class="recommendation-service"><span>目标服务</span>
       <UiSelect :model-value="activeService?.service_id" @update:model-value="selectService">
        <option v-for="rule in serviceOptions" :key="rule.service_id" :value="rule.service_id">{{rule.name}}</option>
       </UiSelect>
      </label>
      <p>{{standards[activePurpose]}}</p>
     </div>
     <details class="recommendation-method">
      <summary>选线规则说明</summary>
      <p>地区按节点标注分组。每区优先用当前配置的证据。延迟和下载优先采用已核验出口的记录。按同条件组比较，覆盖不同线路较多的组优先，同数时取记录较新的组。次选优先沿用首选的检测条件。导出使用已推荐线路的当前配置。</p>
     </details>
    </div>
   </div>

   <section class="purpose-results" :aria-label="purposeName+'地区推荐'">
    <div v-if="activeRegions.length" class="region-cards-grid">
     <section v-for="region in activeRegions" :key="region.code" class="region-result region-card" :aria-label="region.label+'推荐'">
      <header class="region-head">
       <div class="region-title-wrap">
        <span class="region-flag" :class="regionBadgeClass(region.code)">{{region.code==='unknown'?'?':region.code}}</span>
        <h3>{{region.label}}</h3>
        <span class="region-count-pill">{{region.nodes.length}} 条线路</span>
       </div>
       <button v-if="canPlanRegion(region)" class="region-plan-btn" @click="openRegionPlan(region)">补测本区（{{region.nodes.length}}）</button>
      </header>
      <div class="region-options">
       <article v-for="(candidate,index) in region.picks" :key="key(candidate.node)" class="region-candidate" :class="{first:!index&&isRecommendation(candidate),reference:!isRecommendation(candidate)}">
        <div class="candidate-top">
         <span class="candidate-slot" :class="{'candidate-reference':!isRecommendation(candidate),secondary:!!index&&isRecommendation(candidate)}">
          {{!isRecommendation(candidate)?'待复测':index?'次选':'首选'}}
         </span>
         <div class="candidate-title">
          <strong>{{nodeDisplayName(candidate.node)}}</strong>
          <small class="candidate-source">{{candidateSource(candidate.node)}} · {{candidate.node.type}}</small>
          <span v-if="candidate.purity" class="cf-chip" :class="candidate.purity.tone" :title="candidate.purity.title">{{candidate.purity.badgeLabel}}</span>
          <span v-if="candidate.cfChallenge" class="cf-chip" :class="candidate.cfChallenge.tone" :title="candidate.cfChallenge.detail">{{candidate.cfChallenge.label}}</span>
         </div>
         <span class="candidate-hero" :class="{'candidate-reference':!isRecommendation(candidate)}">{{candidateHero(candidate)}}</span>
        </div>
        <div class="candidate-evidence">
         <p>{{candidate.basis}}</p>
         <div v-if="candidate.purity&&(candidate.purity.sources.length>=2||candidate.purity.issues.length)" class="candidate-rep-strip" :title="candidate.purity.title">
          <span v-for="src in candidate.purity.sources" :key="src.id" class="candidate-rep-chip" :class="src.tone" :title="src.detail"><b>{{src.name}}</b><i>{{src.risk}}</i></span>
          <span v-for="iss in candidate.purity.issues.slice(0,2)" :key="iss" class="candidate-rep-chip issue">{{iss}}</span>
          <small v-if="candidate.purity.divergent" class="candidate-rep-note">多站分歧大 · 风险分越低越净</small>
          <small v-else-if="candidate.purity.singleFallback" class="candidate-rep-note">仅单站返回 · 谨慎参考</small>
         </div>
         <div class="candidate-foot">
          <small>{{candidate.count}} {{activePurpose==='daily'?'次采样':isServicePurpose(activePurpose)?'次规则通过':'次同条件读数'}} · 最近 {{date(candidate.at)}}<template v-if="secondPickNote(region.picks,index)"> · <em>{{secondPickNote(region.picks,index)}}</em></template></small>
          <div class="candidate-actions">
           <button class="candidate-btn" @click="inspect(candidate)">查看记录</button>
           <button class="candidate-btn" @click="openInWorkbench(candidate.node)">在线路查看</button>
          </div>
         </div>
         <p v-if="configurationNote[candidate.configuration]" class="candidate-reference candidate-config-warn">{{configurationNote[candidate.configuration]}}</p>
        </div>
       </article>
       <p v-if="region.picks.length===1&&isRecommendation(region.picks[0])" class="region-note">次选待测 · 可补测本区其余节点补充备选</p>
      </div>
     </section>
    </div>

    <div v-if="emptyRegions.length" class="empty-regions-block">
     <header class="empty-regions-head">
      <h4>待检测地区 <span>{{emptyRegions.length}}</span></h4>
      <small>以下地区近 7 天尚无满足当前用途的检测记录，点“补测本区”可直接生成小范围检测草稿</small>
     </header>
     <div class="region-empty-grid">
      <section v-for="region in emptyRegions" :key="region.code" class="region-result region-empty-card" :aria-label="region.label+'推荐'">
       <div class="region-empty-top">
        <div class="region-title-wrap">
         <span class="region-flag" :class="regionBadgeClass(region.code)">{{region.code==='unknown'?'?':region.code}}</span>
         <h3>{{region.label}}</h3>
         <span class="region-count-pill">{{region.nodes.length}}</span>
        </div>
        <button v-if="canPlanRegion(region)" class="region-plan-btn" @click="openRegionPlan(region)">补测本区（{{region.nodes.length}}）</button>
       </div>
       <p class="region-note">{{region.code==='unknown'?'地区待确认，可在线路工作台查看':isServicePurpose(activePurpose)&&!activeService?'暂无可选服务':missing[activePurpose]}}</p>
      </section>
     </div>
    </div>
   </section>
  </template>
 </section>
</template>

<style scoped>
.recommendation-page .page-header p{font-size:13px}
.recommendation-empty{display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:12px}
.recommendation-hero-card{padding:18px 20px;border:1px solid var(--cst-line);border-radius:var(--cst-radius);background:var(--cst-panel);box-shadow:var(--cst-shadow-xs);margin-bottom:20px;display:grid;gap:14px}
.recommendation-hero-top{display:flex;align-items:center;justify-content:space-between;gap:14px;flex-wrap:wrap}
.recommendation-tabs{display:inline-flex;gap:4px;flex-wrap:wrap;padding:4px;border-radius:11px;background:var(--cst-panel-2);border:1px solid var(--cst-line)}
.recommendation-tabs button{padding:7px 16px;border:1px solid transparent;border-radius:8px;background:transparent;color:var(--cst-muted);cursor:pointer;font-size:13.5px;font-weight:550;transition:all .14s}
.recommendation-tabs button:hover:not([aria-pressed=true]){color:var(--cst-text);background:color-mix(in srgb,var(--cst-panel) 65%,transparent)}
.recommendation-tabs button[aria-pressed=true]{font-weight:650;color:var(--cst-accent);background:var(--cst-panel);border-color:color-mix(in srgb,var(--cst-accent) 24%,var(--cst-line));box-shadow:var(--cst-shadow)}
.recommendation-export{display:flex;align-items:center;gap:14px;flex-wrap:wrap}
.coverage-meter{display:flex;flex-direction:column;gap:4px;min-width:140px}
.coverage-meter span{font-size:12px;color:var(--cst-muted)}
.coverage-meter strong{color:var(--cst-text);font-weight:700;font-variant-numeric:tabular-nums}
.coverage-track{height:6px;border-radius:999px;background:var(--cst-aurora-track);overflow:hidden}
.coverage-track>i{display:block;height:100%;border-radius:999px;background:var(--cst-aurora-bar);box-shadow:var(--cst-aurora-glow);transition:width .4s cubic-bezier(.22,1,.36,1)}
.recommendation-criteria{display:flex;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;gap:12px;padding-top:12px;border-top:1px solid var(--cst-line)}
.criteria-main{display:grid;gap:8px;flex:1;min-width:240px}
.recommendation-criteria p,.recommendation-method{font-size:12.5px;line-height:1.6;color:var(--cst-muted);margin:0}
.recommendation-method{max-width:56ch}
.recommendation-method summary{width:fit-content;cursor:pointer;color:var(--cst-accent);font-weight:600;font-size:12.5px}
.recommendation-method p{margin-top:6px}
.recommendation-service{display:inline-flex;align-items:center;gap:8px;flex-wrap:wrap;font-size:13px;font-weight:600;color:var(--cst-text)}
.recommendation-service .ui-select{min-width:220px;max-width:min(100%,340px)}
.region-cards-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px;align-items:start}
.region-card{display:flex;flex-direction:column;gap:12px;padding:18px 20px;border:1px solid var(--cst-line);border-radius:var(--cst-radius);background:var(--cst-panel);box-shadow:var(--cst-shadow-xs)}
.region-head{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}
.region-title-wrap{display:inline-flex;align-items:center;gap:8px;min-width:0}
.region-flag{display:inline-flex;align-items:center;justify-content:center;min-width:26px;height:20px;padding:0 6px;border-radius:6px;font-size:11px;font-weight:750;background:color-mix(in srgb,var(--region,var(--cst-accent)) 14%,var(--cst-panel-2));color:var(--region,var(--cst-accent));border:1px solid color-mix(in srgb,var(--region,var(--cst-accent)) 28%,transparent)}
.region-result h3{font-size:16px;font-weight:700;margin:0;overflow-wrap:anywhere}
.region-count-pill{font-size:11px;font-weight:600;color:var(--cst-muted);padding:2px 8px;border-radius:999px;background:var(--cst-panel-2);border:1px solid var(--cst-line);font-variant-numeric:tabular-nums}
.region-plan-btn{padding:4px 11px;border-radius:999px;border:1px solid color-mix(in srgb,var(--cst-accent) 32%,var(--cst-line));background:color-mix(in srgb,var(--cst-accent) 7%,var(--cst-panel));color:var(--cst-accent);font-size:11.5px;font-weight:600;cursor:pointer;transition:all .14s;white-space:nowrap}
.region-plan-btn:hover{background:var(--cst-accent);color:#fff;border-color:var(--cst-accent)}
.region-options{display:grid;gap:10px;min-width:0}
.region-candidate{display:grid;gap:8px;padding:13px 14px;border-radius:12px;border:1px solid var(--cst-line);background:var(--cst-panel-2)}
.region-candidate.first{background:color-mix(in srgb,var(--cst-accent) 5%,var(--cst-panel));border-color:color-mix(in srgb,var(--cst-accent) 30%,var(--cst-line))}
.region-candidate.reference{border-style:dashed;border-color:color-mix(in srgb,var(--cst-warn) 40%,var(--cst-line))}
.candidate-top{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.candidate-slot{display:inline-flex;align-items:center;justify-content:center;padding:2px 10px;border-radius:999px;font-size:11.5px;font-weight:700;background:var(--cst-aurora-primary);box-shadow:0 3px 10px -2px rgba(79,110,247,.38);color:#fff;flex:none}
.candidate-slot.secondary{background:var(--cst-panel);color:var(--cst-text);border:1px solid var(--cst-line-strong)}
.candidate-slot.candidate-reference{background:var(--cst-warn-bg);color:var(--cst-warn)!important;border:1px solid color-mix(in srgb,var(--cst-warn) 32%,transparent)}
.candidate-reference{color:var(--cst-warn)!important}
.candidate-title{display:flex;align-items:center;gap:4px 8px;flex-wrap:wrap;flex:1;min-width:160px;overflow-wrap:anywhere}
.candidate-title strong{font-size:14px;font-weight:650}
.candidate-source{font-size:11.5px;color:var(--cst-muted)}
.candidate-hero{margin-left:auto;font-size:15px;font-weight:750;font-variant-numeric:tabular-nums;color:var(--cst-accent);white-space:nowrap}
.cf-chip{display:inline-flex;align-items:center;padding:1px 7px;border-radius:999px;font-size:11px;font-weight:600;line-height:1.5;border:1px solid transparent}
.cf-chip.good{color:var(--cst-good,#15803d);background:color-mix(in srgb,var(--cst-good,#15803d) 10%,transparent);border-color:color-mix(in srgb,var(--cst-good,#15803d) 24%,transparent)}
.cf-chip.warn{color:var(--cst-warn,#b45309);background:color-mix(in srgb,var(--cst-warn,#b45309) 12%,transparent);border-color:color-mix(in srgb,var(--cst-warn,#b45309) 28%,transparent)}
.cf-chip.bad{color:var(--cst-bad,#b91c1c);background:color-mix(in srgb,var(--cst-bad,#b91c1c) 10%,transparent);border-color:color-mix(in srgb,var(--cst-bad,#b91c1c) 26%,transparent)}
.candidate-rep-strip{display:flex;align-items:center;flex-wrap:wrap;gap:4px 6px;margin-top:6px}
.candidate-rep-chip{display:inline-flex;align-items:center;gap:3px;padding:1px 6px;border-radius:6px;border:1px solid var(--cst-line);background:var(--cst-panel);font-size:10.5px;line-height:1.45;font-variant-numeric:tabular-nums}
.candidate-rep-chip b{font-weight:600;color:var(--cst-muted)}
.candidate-rep-chip i{font-style:normal;font-weight:750}
.candidate-rep-chip.good i{color:var(--cst-good,#15803d)}.candidate-rep-chip.good{border-color:color-mix(in srgb,var(--cst-good,#15803d) 24%,var(--cst-line))}
.candidate-rep-chip.warn i{color:var(--cst-warn,#b45309)}.candidate-rep-chip.warn{border-color:color-mix(in srgb,var(--cst-warn,#b45309) 28%,var(--cst-line))}
.candidate-rep-chip.bad i{color:var(--cst-bad,#b91c1c)}.candidate-rep-chip.bad{border-color:color-mix(in srgb,var(--cst-bad,#b91c1c) 26%,var(--cst-line))}
.candidate-rep-chip.issue{color:var(--cst-warn,#b45309);background:var(--cst-warn-bg);border-color:color-mix(in srgb,var(--cst-warn,#b45309) 28%,transparent);font-weight:600}
.candidate-rep-note{font-size:10.5px;color:var(--cst-warn,#b45309);font-weight:550}
.candidate-evidence p{margin:0;font-size:12.5px;line-height:1.5;color:var(--cst-text);overflow-wrap:anywhere}
.candidate-foot{display:flex;align-items:center;justify-content:space-between;gap:8px 12px;flex-wrap:wrap;margin-top:6px}
.candidate-foot small,.region-note{font-size:11.5px;line-height:1.55;color:var(--cst-muted)}
.candidate-foot em{font-style:normal;color:var(--cst-text);font-weight:550}
.candidate-config-warn{margin-top:4px!important;font-size:11.5px!important}
.region-note{margin:2px 0}
.candidate-actions{display:inline-flex;gap:6px;flex-wrap:wrap}
.candidate-btn{padding:3px 9px;border-radius:7px;border:1px solid var(--cst-line-strong);background:var(--cst-panel);color:var(--cst-text);font-size:11.5px;font-weight:550;cursor:pointer;transition:border-color .14s,color .14s}
.candidate-btn:hover{border-color:var(--cst-accent);color:var(--cst-accent)}
.empty-regions-block{margin-top:24px;padding-top:20px;border-top:1px dashed var(--cst-line-strong)}
.empty-regions-head{display:flex;align-items:baseline;justify-content:space-between;gap:8px 16px;flex-wrap:wrap;margin-bottom:12px}
.empty-regions-head h4{margin:0;font-size:14px;font-weight:650;display:inline-flex;align-items:center;gap:6px}
.empty-regions-head h4 span{padding:1px 7px;border-radius:999px;background:var(--cst-panel-2);border:1px solid var(--cst-line);font-size:11.5px;color:var(--cst-muted)}
.empty-regions-head small{font-size:12px;color:var(--cst-muted)}
.region-empty-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:12px}
.region-empty-card{padding:13px 15px;border-radius:12px;border:1px dashed var(--cst-line-strong);background:var(--cst-panel);display:grid;gap:8px}
.region-empty-top{display:flex;align-items:center;justify-content:space-between;gap:8px}
.region-empty-card h3{font-size:14px}
@media(max-width:960px){
 .region-cards-grid{grid-template-columns:minmax(0,1fr)}
}
</style>
