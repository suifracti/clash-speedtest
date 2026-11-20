<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref} from 'vue'
import {api,saveFile} from '../api'
import {bytes,date,key,scope,stateLabel,type ManualRoundDetail,type ManualRoundSummary,type MeasurementRoundItem,type NodeOption} from '../domain'
import {manualRoundCount,manualRoundLabel,manualRoundState} from '../measurementRounds'
import {
 analyzeManualRoundItems,
 buildManualRoundCSV,
 buildManualRoundRetestDraft,
 collectPassedRoundNodes,
 summarizeManualRound,
 type ManualRoundRowAnalysis,
 type ManualRoundRowStatus,
} from '../manualRoundReport'
import {nodeSourceIndex} from '../nodePresentation'
import {useWorkspace} from '../workspace'
import Modal from './Modal.vue'

const props=defineProps<{summary:ManualRoundSummary}>()
const emit=defineEmits<{close:[]}>()
const w=useWorkspace()
const detail=ref<ManualRoundDetail>(),loading=ref(false),loadingAll=ref(false),error=ref('')
const offset=ref(0),requestedOffset=ref(0),visited=ref<number[]>([])
const allItems=ref<MeasurementRoundItem[]|null>(null)

const statusFilter=ref<'all'|ManualRoundRowStatus|'save_failed'>('all')
const issueFilter=ref<string>('all')
const projectFilter=ref<string>('all')
const regionFilter=ref<string>('all')
const search=ref('')
const exportBusy=ref(false)
const actionNotice=ref('')

const sources=computed(()=>nodeSourceIndex(w.airports.value))
const plannedTotal=computed(()=>manualRoundCount(props.summary))
const activeItems=computed<MeasurementRoundItem[]>(()=>allItems.value??detail.value?.items??[])
const isPartialView=computed(()=>!allItems.value&&(Boolean(detail.value?.next_offset)||offset.value>0))
const analyzedRows=computed(()=>analyzeManualRoundItems(activeItems.value,w.catalog.value))
const stats=computed(()=>summarizeManualRound(analyzedRows.value))

let mounted=true,loadToken=0
async function read(next:number){
 if(loading.value||loadingAll.value)return
 const token=++loadToken
 loading.value=true;error.value='';requestedOffset.value=next
 try{
  const page=await api.manualRound(props.summary.round_id,next)
  if(mounted&&token===loadToken){
   detail.value=page;offset.value=next
   if(!visited.value.includes(next))visited.value.push(next)
   if(next===0&&!page.next_offset)allItems.value=page.items
  }
 }catch(e){if(mounted&&token===loadToken)error.value=e instanceof Error?e.message:String(e)}
 finally{if(mounted&&token===loadToken)loading.value=false}
}

async function loadAllPages(){
 if(loading.value||loadingAll.value||allItems.value)return
 const token=++loadToken
 loadingAll.value=true;error.value=''
 try{
  const collected:MeasurementRoundItem[]=[]
  const seenOffsets=new Set<number>()
  let cursor:number|undefined=0
  while(cursor!==undefined&&!seenOffsets.has(cursor)){
   seenOffsets.add(cursor)
   const page:ManualRoundDetail=cursor===offset.value&&detail.value?detail.value:await api.manualRound(props.summary.round_id,cursor)
   if(!mounted||token!==loadToken)return
   collected.push(...(page.items||[]))
   cursor=page.next_offset
  }
  if(mounted&&token===loadToken)allItems.value=collected
 }catch(e){if(mounted&&token===loadToken)error.value=e instanceof Error?e.message:String(e)}
 finally{if(mounted&&token===loadToken)loadingAll.value=false}
}

onMounted(()=>void read(0))
onBeforeUnmount(()=>{mounted=false;loadToken++})

function accountName(item:MeasurementRoundItem){
 const source=sources.value.get(item.profile_id)
 if(!source)return `历史账户 ${item.profile_id}`
 return source.subscriptionName&&source.subscriptionName!==source.airportName
  ?`${source.airportName} · ${source.subscriptionName}`
  :source.airportName||source.subscriptionName
}

function stateTone(v?:string){
 if(['completed','saved'].includes(v||''))return 'good'
 if(['failed','interrupted'].includes(v||''))return 'bad'
 if(['cancelled','user_cancelled','not_executed','pending','not_started','skipped_config'].includes(v||''))return 'empty'
 return 'warn'
}

function toggleStatus(next:'all'|ManualRoundRowStatus|'save_failed'){
 statusFilter.value=statusFilter.value===next&&next!=='all'?'all':next
 if(statusFilter.value!=='all')issueFilter.value='all'
}

function toggleIssue(category:string){
 issueFilter.value=issueFilter.value===category?'all':category
 if(issueFilter.value!=='all')statusFilter.value='all'
}

const hasActiveFilters=computed(()=>statusFilter.value!=='all'||issueFilter.value!=='all'||projectFilter.value!=='all'||regionFilter.value!=='all'||search.value.trim().length>0)

function resetFilters(){
 statusFilter.value='all'
 issueFilter.value='all'
 projectFilter.value='all'
 regionFilter.value='all'
 search.value=''
}

const filteredRows=computed(()=>{
 const q=search.value.trim().toLowerCase()
 return analyzedRows.value.filter(row=>{
  if(statusFilter.value==='save_failed'&&!row.saveFailed)return false
  if(statusFilter.value!=='all'&&statusFilter.value!=='save_failed'&&row.status!==statusFilter.value)return false
  if(issueFilter.value!=='all'&&row.issueCategory!==issueFilter.value)return false
  if(projectFilter.value!=='all'&&row.projectKey!==projectFilter.value)return false
  if(regionFilter.value!=='all'&&row.region!==regionFilter.value)return false
  if(!q)return true
  const hay=[row.nodeName,row.region,accountName(row.item),row.projectLabel,row.resultLabel,row.evidenceText,row.issueLabel||''].join(' ').toLowerCase()
  return hay.includes(q)
 })
})

const issueRowsInView=computed(()=>filteredRows.value.filter(r=>r.status==='warn'||r.status==='bad'))
const retestDraft=computed(()=>buildManualRoundRetestDraft(issueRowsInView.value,w.nodes.value,w.catalog.value))
const passedExport=computed(()=>collectPassedRoundNodes(filteredRows.value,w.nodes.value))

const statusCards=computed(()=>[
 {key:'all' as const,label:'全部项目',value:stats.value.total,tone:''},
 {key:'good' as const,label:'通过 / 可用',value:stats.value.good,tone:'good'},
 {key:'warn' as const,label:'受限 / 待留意',value:stats.value.warn,tone:'warn'},
 {key:'bad' as const,label:'异常 / 失败',value:stats.value.bad,tone:'bad'},
 {key:'empty' as const,label:'未执行 / 取消',value:stats.value.empty,tone:'empty'},
 ...(stats.value.saveFailed?[{key:'save_failed' as const,label:'保存失败',value:stats.value.saveFailed,tone:'bad'}]:[]),
].filter(card=>card.key==='all'||card.value>0))

function inspectRow(row:ManualRoundRowAnalysis){
 const current=w.nodes.value.find(n=>key(n)===key(row.item)&&n.node_key===row.item.node_key)
 const fallback:NodeOption={
  profile_id:row.item.profile_id,
  node_key:row.item.node_key,
  node_identity_key:row.item.node_identity_key,
  config_revision_key:row.item.config_revision_key,
  display_name:row.item.display_name||row.nodeName,
  profile_name:accountName(row.item),
  type:row.item.service?.node_type||row.item.download?.node_type||row.item.latency?.node_type||'',
  country_code:'',
  country_flag:'',
 }
 w.project.value=row.item.project
 w.inspectNode.value={
  ...(current||fallback),
  historyServiceId:row.item.project==='service'?(row.item.service_id||row.item.service?.service_id):undefined,
 }
 emit('close')
}

async function exportCSV(){
 if(!filteredRows.value.length||exportBusy.value)return
 exportBusy.value=true;actionNotice.value=''
 try{
  const csv=buildManualRoundCSV(props.summary,filteredRows.value,accountName)
  const stamp=props.summary.started_at.slice(0,16).replace(/[:T]/g,'-')
  const saved=await saveFile(csv,`speedtest-round-${stamp}.csv`,'text/csv;charset=utf-8')
  if(saved)w.notify(`已导出 ${filteredRows.value.length} 条记录的 CSV`)
 }catch(e){w.fail(e)}
 finally{exportBusy.value=false}
}

async function exportPassedClash(){
 const {nodes,missingCount}=passedExport.value
 if(!nodes.length||exportBusy.value)return
 exportBusy.value=true;actionNotice.value=''
 try{
  const groupName=`${manualRoundLabel(props.summary)}通过节点`
  const result=await api.exportClash(nodes.map(n=>scope(n)),groupName)
  const saved=await saveFile(result.yaml_content,'speedtest-round-passed.yaml','application/x-yaml')
  if(saved){
   const suffix=missingCount?`（另有 ${missingCount} 个通过节点配置已变更或移除，未纳入导出）`:''
   w.notify(`已导出 ${result.node_count} 个通过节点${suffix}`)
  }
 }catch(e){w.fail(e)}
 finally{exportBusy.value=false}
}

function openRetestIssues(){
 const draft=retestDraft.value
 if(!draft.nodes.length){
  actionNotice.value=draft.missingNodeCount?`${draft.missingNodeCount} 个异常节点配置已变化或已移除，无法沿旧配置重测。`:'当前筛选下没有可重测的异常项目。'
  return
 }
 w.openPlan(draft.nodes)
 if(!w.plan.value)return
 w.plan.value.projects=draft.projects
 w.plan.value.serviceIds=draft.serviceIds
 w.plan.value.includeReputation=draft.includeReputation
 w.plan.value.reputationServiceIds=draft.reputationServiceIds
 if(draft.projects.includes('latency')){
  w.plan.value.target='all'
  w.plan.value.samples=6
 }
 w.page.value='home'
 if(draft.missingNodeCount)w.notify(`已预填 ${draft.nodes.length} 个异常节点的重测草稿（跳过 ${draft.missingNodeCount} 个已变化/移除节点）`)
 emit('close')
}
</script>

<template>
 <Modal :title="manualRoundLabel(summary)" wide @close="emit('close')">
  <div class="mr-meta-bar">
   <p class="muted small">
    {{date(summary.started_at)}} · {{summary.node_count}} 个节点 · {{plannedTotal}} 项 ·
    <span class="mr-pill" :class="stateTone(detail?.state||summary.state)">{{manualRoundState(detail?.state||summary.state)}}</span>
   </p>
   <div v-if="isPartialView" class="mr-partial-bar">
    <span class="muted small">当前页已读 {{activeItems.length}} / 共 {{plannedTotal}} 项</span>
    <button type="button" class="button ghost small-btn" :disabled="loadingAll||loading" @click="loadAllPages">
     {{loadingAll?'载入全部中…':`载入全部 (${plannedTotal})`}}
    </button>
   </div>
  </div>

  <p v-if="loading" role="status" class="muted small">正在读取轮次…</p>
  <p v-if="error" role="alert" class="inline-warning">轮次读取失败：{{error}} <button class="text-button" @click="read(requestedOffset)">重新读取</button></p>
  <p v-if="actionNotice" role="status" class="inline-warning">{{actionNotice}}</p>

  <template v-if="detail||allItems">
   <div v-if="analyzedRows.length" class="mr-summary-cards" role="group" aria-label="轮次状态汇总">
    <button
     v-for="card in statusCards"
     :key="card.key"
     type="button"
     class="mr-stat-card"
     :class="[card.tone,{active:statusFilter===card.key}]"
     @click="toggleStatus(card.key)"
    >
     <span>{{card.label}}</span>
     <strong>{{card.value}}</strong>
    </button>
   </div>

   <div v-if="stats.issueGroups.length" class="mr-issue-strip" role="group" aria-label="异常原因分布">
    <span class="mr-strip-label">异常归因</span>
    <button
     v-for="group in stats.issueGroups"
     :key="group.category"
     type="button"
     class="mr-issue-chip"
     :class="[group.tone,{active:issueFilter===group.category}]"
     :title="group.sampleReason"
     @click="toggleIssue(group.category)"
    >
     <span>{{group.label}}</span>
     <b>{{group.count}}</b>
    </button>
   </div>

   <div v-if="analyzedRows.length>1" class="mr-toolbar">
    <select v-if="stats.projects.length>1" v-model="projectFilter" aria-label="按检测项目筛选">
     <option value="all">全部项目 ({{stats.projects.length}})</option>
     <option v-for="proj in stats.projects" :key="proj.key" :value="proj.key">{{proj.label}} ({{proj.count}})</option>
    </select>
    <select v-if="stats.regions.length>1" v-model="regionFilter" aria-label="按地区筛选">
     <option value="all">全部地区</option>
     <option v-for="reg in stats.regions" :key="reg" :value="reg">{{reg}}</option>
    </select>
    <input v-model="search" type="search" class="mr-search" aria-label="搜索轮次明细" placeholder="搜索节点、项目、结果或报错…">
    <button v-if="hasActiveFilters" type="button" class="text-button" @click="resetFilters">重置筛选</button>
    <span class="mr-count-note muted small">显示 {{filteredRows.length}} / {{analyzedRows.length}} 项</span>
   </div>

   <p v-if="!analyzedRows.length" class="muted">此页没有检测项目。</p>
   <p v-else-if="!filteredRows.length" class="muted mr-empty-filter">当前筛选条件下没有匹配项。<button type="button" class="text-button" @click="resetFilters">显示全部</button></p>
   <div v-else class="manual-round-table">
    <table aria-label="轮次检测结果">
     <thead><tr><th>节点与项目</th><th>执行</th><th>检测结果</th><th>保存</th></tr></thead>
     <tbody>
      <tr v-for="row in filteredRows" :key="offset+row.index">
       <td>
        <strong>{{row.projectLabel}}</strong>
        <div class="mr-node-line">
         <button type="button" class="text-button mr-node-btn" :title="`点击查看历史 · 配置 ${row.item.config_revision_key}`" @click="inspectRow(row)">{{row.nodeName}}</button>
         <span class="mr-account-meta"> · {{accountName(row.item)}}</span>
        </div>
       </td>
       <td><span class="mr-pill" :class="stateTone(row.item.execution_state)">{{stateLabel(row.item.execution_state)}}</span></td>
       <td>
        <span class="mr-pill" :class="row.status">{{row.resultLabel}}</span>
        <small v-if="row.item.download?.result&&row.item.download.result.bytes_read>0" :title="row.item.download.result.bytes_read+' 字节'">已读 {{bytes(row.item.download.result.bytes_read)}}</small>
        <small v-if="row.evidenceText">{{row.evidenceText}}</small>
       </td>
       <td>
        <span class="mr-pill" :class="stateTone(row.item.persistence_state)">{{stateLabel(row.item.persistence_state)}}</span>
        <small v-if="row.item.service?.persistence_error||row.item.download?.persistence_error">{{row.item.service?.persistence_error||row.item.download?.persistence_error}}</small>
       </td>
      </tr>
     </tbody>
    </table>
   </div>

   <div v-if="!allItems&&(detail?.next_offset||offset)" class="manual-round-pages">
    <span>{{offset+1}}–{{offset+(detail?.items.length||0)}} / {{plannedTotal}} 项</span>
    <button class="button" :disabled="loading||loadingAll||!offset" @click="read(visited[visited.indexOf(offset)-1]||0)">上一页</button>
    <button class="button" :disabled="loading||loadingAll||!detail?.next_offset" @click="read(detail!.next_offset!)">下一页</button>
   </div>
  </template>

  <template #footer>
   <div class="mr-footer-actions">
    <div class="mr-footer-left">
     <button type="button" class="button ghost" :disabled="!filteredRows.length||exportBusy" @click="exportCSV">导出 CSV</button>
     <button
      v-if="passedExport.nodes.length"
      type="button"
      class="button"
      :disabled="exportBusy"
      :title="passedExport.missingCount?`另有 ${passedExport.missingCount} 个通过节点配置已变更`:'导出当前通过节点的 Clash 配置'"
      @click="exportPassedClash"
     >导出通过节点 ({{passedExport.nodes.length}})</button>
    </div>
    <div class="mr-footer-right">
     <button
      v-if="issueRowsInView.length"
      type="button"
      class="button primary"
      :disabled="exportBusy"
      @click="openRetestIssues"
     >重测异常 ({{retestDraft.nodes.length||issueRowsInView.length}})</button>
    </div>
   </div>
  </template>
 </Modal>
</template>

<style scoped>
.mr-meta-bar {display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:10px}
.mr-meta-bar p {margin:0}
.mr-partial-bar {display:inline-flex;align-items:center;gap:8px}
.small-btn {padding:4px 10px;font-size:12px}
.mr-summary-cards {display:flex;flex-wrap:wrap;gap:8px;margin-top:14px}
.mr-stat-card {display:inline-flex;align-items:center;gap:8px;padding:7px 12px;border-radius:10px;border:1px solid var(--cst-line);background:var(--cst-panel-2);color:var(--cst-text);cursor:pointer;font-size:12px;transition:all .14s}
.mr-stat-card span {color:var(--cst-muted);font-weight:550}
.mr-stat-card strong {font-size:14px;font-weight:750;font-variant-numeric:tabular-nums}
.mr-stat-card.good strong {color:var(--cst-good,#15803d)}
.mr-stat-card.warn strong {color:var(--cst-warn,#b45309)}
.mr-stat-card.bad strong {color:var(--cst-bad,#b91c1c)}
.mr-stat-card:hover,.mr-stat-card.active {border-color:var(--cst-accent,#2563eb);background:color-mix(in srgb,var(--cst-accent,#2563eb) 9%,var(--cst-panel));box-shadow:0 0 0 1px color-mix(in srgb,var(--cst-accent,#2563eb) 22%,transparent)}
.mr-issue-strip {display:flex;align-items:center;flex-wrap:wrap;gap:6px;margin-top:10px;padding:8px 11px;border-radius:10px;border:1px solid var(--cst-line);background:color-mix(in srgb,var(--cst-warn,#b45309) 4%,var(--cst-panel))}
.mr-strip-label {font-size:11.5px;font-weight:650;color:var(--cst-muted);margin-right:2px}
.mr-issue-chip {display:inline-flex;align-items:center;gap:5px;padding:3px 9px;border-radius:999px;border:1px solid var(--cst-line);background:var(--cst-panel);color:var(--cst-text);font-size:11.5px;font-weight:550;cursor:pointer;transition:all .14s}
.mr-issue-chip b {font-variant-numeric:tabular-nums;font-weight:750}
.mr-issue-chip.warn {border-color:color-mix(in srgb,var(--cst-warn,#b45309) 30%,transparent);color:var(--cst-warn,#b45309)}
.mr-issue-chip.bad {border-color:color-mix(in srgb,var(--cst-bad,#b91c1c) 30%,transparent);color:var(--cst-bad,#b91c1c)}
.mr-issue-chip.active {background:var(--cst-accent,#2563eb);border-color:var(--cst-accent,#2563eb);color:#fff}
.mr-toolbar {display:flex;align-items:center;flex-wrap:wrap;gap:8px;margin-top:12px}
.mr-toolbar select,.mr-search {height:32px;padding:0 10px;border-radius:8px;border:1px solid var(--cst-line-strong,var(--cst-line));background:var(--cst-panel);color:var(--cst-text);font-size:12.5px}
.mr-search {flex:1 1 200px;min-width:160px}
.mr-count-note {margin-left:auto;font-variant-numeric:tabular-nums}
.mr-empty-filter {margin-top:16px;padding:18px;border-radius:12px;border:1px dashed var(--cst-line);text-align:center}
.manual-round-table {overflow-x:auto;margin-top:12px;border:1px solid var(--cst-line);border-radius:12px;background:var(--cst-panel)}
.manual-round-table table {border-collapse:collapse;width:100%;min-width:540px;text-align:left;font-size:13px;table-layout:fixed}
.manual-round-table th,.manual-round-table td {padding:11px 12px;border-bottom:1px solid var(--cst-line);vertical-align:top;overflow-wrap:anywhere}
.manual-round-table thead th {background:var(--cst-panel-2);color:var(--cst-muted);font-weight:600;font-size:12px}
.manual-round-table tbody tr:last-child td {border-bottom:0}
.manual-round-table tbody tr:hover td {background:color-mix(in srgb,var(--cst-accent,#2563eb) 4%,transparent)}
.manual-round-table th:nth-child(1) {width:33%}
.manual-round-table th:nth-child(2),.manual-round-table th:nth-child(4) {width:13%}
.manual-round-table small {display:block;color:var(--cst-muted);margin-top:4px;line-height:1.5}
.mr-node-line {margin-top:4px;font-size:12px;line-height:1.45;color:var(--cst-muted)}
.mr-node-btn {display:inline;padding:0;font-size:12px;font-weight:550;color:var(--cst-accent,#2563eb);text-align:left}
.mr-account-meta {color:var(--cst-muted)}
.mr-pill {display:inline-flex;align-items:center;padding:1px 8px;border-radius:999px;font-size:11.5px;font-weight:600;line-height:1.45;border:1px solid var(--cst-line);background:var(--cst-panel-2);color:var(--cst-muted)}
.mr-pill.good {color:var(--cst-good,#15803d);background:color-mix(in srgb,var(--cst-good,#15803d) 12%,transparent);border-color:color-mix(in srgb,var(--cst-good,#15803d) 28%,transparent)}
.mr-pill.warn {color:var(--cst-warn,#b45309);background:color-mix(in srgb,var(--cst-warn,#b45309) 13%,transparent);border-color:color-mix(in srgb,var(--cst-warn,#b45309) 32%,transparent)}
.mr-pill.bad {color:var(--cst-bad,#b91c1c);background:color-mix(in srgb,var(--cst-bad,#b91c1c) 12%,transparent);border-color:color-mix(in srgb,var(--cst-bad,#b91c1c) 28%,transparent)}
.manual-round-pages {display:flex;align-items:center;justify-content:flex-end;gap:8px;margin-top:12px}
.manual-round-pages span {font-size:12px;color:var(--cst-muted)}
.mr-footer-actions {display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:10px;width:100%}
.mr-footer-left,.mr-footer-right {display:inline-flex;align-items:center;flex-wrap:wrap;gap:8px}
</style>
