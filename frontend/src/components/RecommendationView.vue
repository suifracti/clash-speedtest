<script setup lang="ts">
import {computed,onMounted,ref} from 'vue'
import {useWorkspace} from '../workspace'
import {api,saveFile} from '../api'
import {date,isNotice,key,scope,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {recommend,purposes,type Purpose} from '../recommendations'
import {nodeDisplayName} from '../nodePresentation'
const w=useWorkspace(),loading=ref(false),error=ref(''),data=ref<Record<string,QualityEvidence>>({}),readAt=ref('')
const nodes=computed(()=>w.nodes.value.filter(n=>!isNotice(n))),results=computed(()=>recommend(nodes.value,data.value,w.catalog.value))
const hasCandidates=computed(()=>Object.values(results.value).some(list=>list.length))
const missing={streaming:'近 7 天没有流媒体站点通过的记录',ai:'近 7 天没有 AI 站点通过的记录',download:'近 7 天没有可比较的有效下载记录',daily:'近 7 天没有足够的同条件 Cloudflare 响应样本'}
async function refresh(){loading.value=true;error.value='';data.value={};const until=new Date().toISOString(),since=new Date(Date.parse(until)-7*86400000).toISOString();try{data.value=await readQualityBatch(nodes.value,since,until);readAt.value=until;if(Object.values(data.value).some(e=>!e.complete))error.value='部分历史读取不完整，仅展示依据完整的候选'}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{loading.value=false}}
async function exportCandidates(purpose:Purpose){try{const candidates=results.value[purpose],result=await api.exportClash(candidates.map(c=>scope(c.node)),purposes.find(p=>p.id===purpose)!.name+'候选');if(await saveFile(result.yaml_content,'speedtest-'+purpose+'.yaml','application/x-yaml'))w.notify(`已导出 ${result.node_count} 个候选节点`)}catch(e){w.fail(e)}}
function inspect(node:NodeOption,at:string){const e=data.value[key(node)],record=[...e.tests,...e.downloads,...e.services].find(r=>r.finished_at===at);w.historyNode.value=record?{...node,node_key:record.node_key,config_revision_key:record.config_revision_key}:node}
onMounted(()=>void refresh())
</script>
<template><section class="page-view"><div class="page-header"><div><h1>现在用哪个</h1><p>依据近 7 天已保存的检测，每种用途最多 3 个候选。只覆盖下方列出的站点与规则。</p></div><button class="button" :disabled="loading" @click="refresh">{{loading?'读取中…':'刷新推荐'}}</button></div><p v-if="error" class="inline-warning" role="alert">{{error}}</p><p v-if="readAt" class="muted small">读取截至 {{date(readAt)}} · 本页只读历史，不发起检测。</p>
 <p v-if="loading" class="muted" role="status">正在读取历史…</p>
 <section v-else-if="!hasCandidates" class="settings-section recommendation-empty"><p>近 7 天的检测记录还不够，没法推荐</p><button class="button primary" @click="w.page.value='home'">去工作台检测</button></section>
 <template v-else><p class="muted small">包含同一订阅、同一节点身份的历史配置记录，不代表当前配置已检测；导出使用当前节点配置。</p>
 <section v-for="purpose in purposes" :key="purpose.id" class="settings-section" :class="{'recommendation-missing':!results[purpose.id].length}"><div class="section-heading"><h2>{{purpose.name}}</h2><button v-if="results[purpose.id].length" class="button" @click="exportCandidates(purpose.id)">导出 Clash 配置组</button></div><p v-if="!results[purpose.id].length" class="muted small">{{missing[purpose.id]}}</p><article v-for="(candidate,i) in results[purpose.id]" :key="key(candidate.node)" class="compact-row"><div><strong>{{i+1}}. {{nodeDisplayName(candidate.node)}}</strong><small>{{w.nodeAirportName(candidate.node)}} · {{w.nodeSubscriptionName(candidate.node)}}</small><p class="small break-text">{{candidate.basis}}</p><small>{{candidate.count}} {{purpose.id==='daily'?'次采样':'次已保存检测'}} · 最近 {{date(candidate.at)}}</small></div><button class="text-button" @click="inspect(candidate.node,candidate.at)">查看记录</button></article></section></template>
</section></template>
<style scoped>.recommendation-empty{display:flex;align-items:center;justify-content:space-between;gap:16px}.recommendation-missing{display:flex;align-items:center;gap:20px;padding:14px 20px}.recommendation-missing .section-heading{margin:0}.recommendation-missing h2,.recommendation-missing p{margin:0}.recommendation-missing h2{font-size:14px}@media(max-width:740px){.recommendation-empty,.recommendation-missing{flex-wrap:wrap}}</style>
