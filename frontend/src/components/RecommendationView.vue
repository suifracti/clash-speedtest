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
const activePurpose=ref<Purpose>('daily')
const missing={streaming:'近 7 天没有流媒体站点通过的记录',ai:'近 7 天没有 AI 站点通过的记录',download:'近 7 天没有可比较的有效下载记录',daily:'近 7 天没有足够的同条件 Cloudflare 响应样本'}
const purposeHints:Record<Purpose,string>={streaming:'Netflix / YouTube / Disney+ 等流媒体解锁优先',ai:'ChatGPT / Claude / Gemini 等大模型可用性优先',download:'同条件有效吞吐速度与稳定性优先',daily:'Cloudflare 稳定低延迟与高响应率优先'}
async function refresh(){loading.value=true;error.value='';data.value={};const until=new Date().toISOString(),since=new Date(Date.parse(until)-7*86400000).toISOString();try{data.value=await readQualityBatch(nodes.value,since,until);readAt.value=until;if(Object.values(data.value).some(e=>!e.complete))error.value='部分历史读取不完整，仅展示依据完整的候选'}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}}
async function exportCandidates(purpose:Purpose){try{const candidates=results.value[purpose],result=await api.exportClash(candidates.map(c=>scope(c.node)),purposes.find(p=>p.id===purpose)!.name+'候选');if(await saveFile(result.yaml_content,'speedtest-'+purpose+'.yaml','application/x-yaml'))w.notify(`已导出 ${result.node_count} 个候选节点`)}catch(e){w.fail(e)}}
function inspect(node:NodeOption,at:string,purpose:Purpose){
 const e=data.value[key(node)],record=e.tests.find(t=>t.samples.some(s=>s.timestamp===at))||[...e.downloads,...e.services].find(a=>attemptObservedAt(a)===at)
 w.hours.value=168;w.project.value=purpose==='daily'?'latency':purpose==='download'?'download':'service'
 w.inspectNode.value=record?{...node,node_key:record.node_key,config_revision_key:record.config_revision_key}:node
}
function openInWorkbench(node:NodeOption,purpose:Purpose){
 w.project.value=purpose==='daily'?'latency':purpose==='download'?'download':'service'
 if(w.viewRequest)w.viewRequest.value=node
 w.page.value='home'
}
onMounted(()=>void refresh())
</script>
<template><section class="page-view">
 <div class="page-header">
  <div>
   <h1>现在用哪个</h1>
   <p>先选用途，再看近 7 天记录支持的候选。</p>
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
  <div class="recommendation-desk">
   <nav class="purpose-index" aria-label="选线用途"><span>我想用来</span><button v-for="purpose in [...purposes].reverse()" :key="purpose.id" :aria-pressed="activePurpose===purpose.id" @click="activePurpose=purpose.id"><strong>{{purpose.name}}</strong><small>{{results[purpose.id].length?results[purpose.id].length+' 条候选':'暂无足够记录'}}</small></button><p>同一用途按相同检测条件比较，最多列出 3 条。</p></nav>
   <section v-for="purpose in purposes" v-show="activePurpose===purpose.id" :key="purpose.id" class="purpose-results" :class="{'recommendation-missing':!results[purpose.id].length}">
    <div class="cst-purpose-head section-heading">
     <div class="cst-purpose-title">
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
      <span class="candidate-order">{{String(i+1).padStart(2,'0')}}</span>
      <div class="cst-candidate-main">
       <div class="cst-candidate-name">
        <strong>{{nodeDisplayName(candidate.node)}}</strong>
        <span class="candidate-source">{{w.nodeAirportName(candidate.node)}} · {{w.nodeSubscriptionName(candidate.node)}}</span>
       </div>
       <p class="small break-text cst-candidate-meta">{{candidate.basis}}</p>
       <small class="muted">{{candidate.count}} {{purpose.id==='daily'?'次采样':'次已保存检测'}} · 最近 {{date(candidate.at)}}</small>
      </div>
      <div class="cst-candidate-actions">
       <button class="text-button" @click="inspect(candidate.node,candidate.at,purpose.id)">查看记录 ›</button>
       <button class="text-button" @click="openInWorkbench(candidate.node,purpose.id)">在工作台查看</button>
      </div>
     </article>
    </div>
   </section>
  </div>
 </template>
</section></template>
<style scoped>
.page-header h1{font-size:22px}.page-header p{font-size:12px}
.recommendation-empty{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:24px}
.recommendation-empty-copy{display:grid;gap:6px}.recommendation-empty-copy strong{font-size:15px}
.recommendation-desk{display:grid;grid-template-columns:200px minmax(0,1fr);gap:32px;margin-top:24px;align-items:start}
.purpose-index{display:grid;gap:6px;position:sticky;top:calc(var(--cst-nav-height,58px) + 20px)}.purpose-index>span{font-size:12px;color:var(--cst-muted);padding:0 12px 10px}
.purpose-index button{display:flex;align-items:baseline;justify-content:space-between;gap:8px;text-align:left;padding:15px 12px;border:0;border-left:3px solid transparent;border-radius:0;background:transparent;color:var(--cst-text)}
.purpose-index button[aria-pressed=true]{border-left-color:var(--cst-accent);background:var(--cst-panel)}.purpose-index button:hover{background:var(--cst-panel)}
.purpose-index strong{font-size:15px;font-weight:600}.purpose-index small{font-size:11px;color:var(--cst-muted)}.purpose-index p{font-size:12px;line-height:1.8;color:var(--cst-muted);padding:12px}
.purpose-results{min-width:0;border-top:2px solid var(--cst-text);padding:24px;background:var(--cst-panel)}
.purpose-results .cst-purpose-head{align-items:center;margin-bottom:0;padding-bottom:24px;flex-wrap:wrap}
.purpose-results .cst-purpose-title h2{font-size:23px;font-weight:600;letter-spacing:-.02em}.purpose-results .cst-purpose-title p{margin-top:8px;font-size:12px;line-height:1.7}
.purpose-results .cst-candidate-list{gap:0}
.purpose-results .cst-candidate-row{grid-template-columns:36px minmax(0,1fr);gap:14px;padding:24px 0;background:none;border:0;border-bottom:1px solid var(--cst-line);border-radius:0;align-items:start}
.candidate-order{font-size:23px;font-weight:400;font-variant-numeric:tabular-nums;color:var(--cst-muted);line-height:1.2}
.purpose-results .cst-candidate-main{gap:10px}.purpose-results .cst-candidate-name{display:grid;gap:5px}.purpose-results .cst-candidate-name strong{font-size:17px;font-weight:600}.candidate-source{color:var(--cst-muted);font-size:12px}
.purpose-results .cst-candidate-meta{font-size:13px;line-height:1.85;color:var(--cst-text);margin:0}.purpose-results .cst-candidate-main>small{font-size:12px}
.purpose-results .cst-candidate-actions{grid-column:2;justify-content:flex-start;gap:20px;margin-top:2px}
.recommendation-missing-note{padding:32px 0;color:var(--cst-muted);line-height:1.8}
@media(max-width:760px){.recommendation-empty{flex-wrap:wrap}.recommendation-desk{grid-template-columns:minmax(0,1fr);gap:16px}.purpose-index{display:flex;position:static;overflow:auto}.purpose-index>span,.purpose-index>p{display:none}.purpose-index button{flex-direction:column;gap:6px;min-width:120px;border-left:0;border-bottom:2px solid transparent}.purpose-index button[aria-pressed=true]{border-bottom-color:var(--cst-accent)}.purpose-results{padding:18px}}
</style>
