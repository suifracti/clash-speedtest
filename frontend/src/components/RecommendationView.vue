<script setup lang="ts">
import {computed,onMounted,ref} from 'vue'
import {useWorkspace} from '../workspace'
import {api,saveFile} from '../api'
import {date,isNotice,key,scope,type NodeOption} from '../domain'
import {attemptObservedAt,type QualityEvidence} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {recommend,purposes,type Purpose} from '../recommendations'
import {nodeDisplayName} from '../nodePresentation'
const w=useWorkspace(),loading=ref(false),error=ref(''),data=ref<Record<string,QualityEvidence>>({}),readAt=ref('')
const nodes=computed(()=>w.nodes.value.filter(n=>!isNotice(n))),results=computed(()=>recommend(nodes.value,data.value,w.catalog.value))
const hasCandidates=computed(()=>Object.values(results.value).some(list=>list.length))
const missing={streaming:'近 7 天没有流媒体站点通过的记录',ai:'近 7 天没有 AI 站点通过的记录',download:'近 7 天没有可比较的有效下载记录',daily:'近 7 天没有足够的同条件 Cloudflare 响应样本'}
const purposeIcons:Record<Purpose,string>={streaming:'TV',ai:'AI',download:'DL',daily:'⚡'}
const purposeHints:Record<Purpose,string>={streaming:'Netflix / YouTube / Disney+ 等流媒体解锁优先',ai:'ChatGPT / Claude / Gemini 等大模型可用性优先',download:'同条件有效吞吐速度与稳定性优先',daily:'Cloudflare 稳定低延迟与高响应率优先'}
async function refresh(){loading.value=true;error.value='';data.value={};const until=new Date().toISOString(),since=new Date(Date.parse(until)-7*86400000).toISOString();try{data.value=await readQualityBatch(nodes.value,since,until);readAt.value=until;if(Object.values(data.value).some(e=>!e.complete))error.value='部分历史读取不完整，仅展示依据完整的候选'}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}}
async function exportCandidates(purpose:Purpose){try{const candidates=results.value[purpose],result=await api.exportClash(candidates.map(c=>scope(c.node)),purposes.find(p=>p.id===purpose)!.name+'候选');if(await saveFile(result.yaml_content,'speedtest-'+purpose+'.yaml','application/x-yaml'))w.notify(`已导出 ${result.node_count} 个候选节点`)}catch(e){w.fail(e)}}
function inspect(node:NodeOption,at:string,purpose:Purpose){
 const e=data.value[key(node)],record=e.tests.find(t=>t.samples.some(s=>s.timestamp===at))||[...e.downloads,...e.services].find(a=>attemptObservedAt(a)===at)
 w.hours.value=168;w.project.value=purpose==='daily'?'latency':purpose==='download'?'download':'service'
 w.inspectNode.value=record?{...node,node_key:record.node_key,config_revision_key:record.config_revision_key}:node
}
onMounted(()=>void refresh())
</script>
<template><section class="page-view">
 <div class="page-header">
  <div>
   <h1>现在用哪个</h1>
   <p>基于近 7 天真实探测历史，按四大使用场景自动遴选 Top 3 候选节点。</p>
  </div>
  <div class="page-header-actions">
   <span v-if="readAt" class="badge">截至 {{date(readAt)}}</span>
   <button class="button" :disabled="loading" @click="refresh">{{loading?'读取中…':'刷新推荐'}}</button>
  </div>
 </div>
 <p v-if="error" class="inline-warning" role="alert">{{error}}</p>
 <p v-if="loading" class="muted" role="status">正在读取历史…</p>
 <section v-else-if="!hasCandidates" class="settings-section recommendation-empty">
  <div class="recommendation-empty-copy">
   <strong>近 7 天的检测记录还不够，没法推荐</strong>
   <span class="muted small">前往线路工作台运行一次延迟、下载或解锁检测后，这里会自动生成分场景推荐。</span>
  </div>
  <button class="button primary" @click="w.page.value='home'">去工作台检测</button>
 </section>
 <template v-else>
  <details class="cst-info-collapse"><summary>推荐依据与配置说明</summary><div class="cst-info-collapse-body"><p>包含同一订阅、同一节点身份的历史配置记录，不代表当前配置已检测；导出使用当前节点配置。本页只读历史，不主动发起网络请求。</p></div></details>
  <div class="cst-purpose-grid">
   <section v-for="purpose in purposes" :key="purpose.id" class="cst-purpose-card settings-section" :class="{'recommendation-missing':!results[purpose.id].length}">
    <div class="cst-purpose-head section-heading">
     <div class="cst-purpose-title">
      <span class="cst-purpose-badge" aria-hidden="true">{{purposeIcons[purpose.id]}}</span>
      <div>
       <h2>{{purpose.name}}</h2>
       <p>{{purposeHints[purpose.id]}}</p>
      </div>
     </div>
     <button v-if="results[purpose.id].length" class="button" @click="exportCandidates(purpose.id)">导出 Clash 配置组</button>
    </div>
    <p v-if="!results[purpose.id].length" class="muted small recommendation-missing-note">{{missing[purpose.id]}}</p>
    <div v-else class="cst-candidate-list">
     <article v-for="(candidate,i) in results[purpose.id]" :key="key(candidate.node)" class="cst-candidate-row compact-row">
      <span class="cst-rank-medal" :class="`rank-${i+1}`">#{{i+1}}</span>
      <div class="cst-candidate-main">
       <div class="cst-candidate-name">
        <strong>{{nodeDisplayName(candidate.node)}}</strong>
        <span class="badge">{{w.nodeAirportName(candidate.node)}} · {{w.nodeSubscriptionName(candidate.node)}}</span>
       </div>
       <p class="small break-text cst-candidate-meta">{{candidate.basis}}</p>
       <small class="muted">{{candidate.count}} {{purpose.id==='daily'?'次采样':'次已保存检测'}} · 最近 {{date(candidate.at)}}</small>
      </div>
      <button class="text-button" @click="inspect(candidate.node,candidate.at,purpose.id)">查看记录 ›</button>
     </article>
    </div>
   </section>
  </div>
 </template>
</section></template>
<style scoped>
.recommendation-empty{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:24px}
.recommendation-empty-copy{display:grid;gap:4px}
.recommendation-empty-copy strong{font-size:15px}
.recommendation-missing{opacity:.88;background:color-mix(in srgb,var(--cst-panel-2) 60%,var(--cst-panel))}
.recommendation-missing-note{margin:4px 0 0;padding:12px;border-radius:8px;border:1px dashed var(--cst-line);background:var(--cst-panel-2);text-align:center}
@media(max-width:740px){.recommendation-empty{flex-wrap:wrap}}
</style>
