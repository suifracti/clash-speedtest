<script setup lang="ts">
import {nodeDisplayName} from '../nodePresentation'
import UiSelect from './UiSelect.vue'
import {computed,onBeforeUnmount,ref,watch} from 'vue'
import {api,request,saveFile} from '../api'
import {useWorkspace} from '../workspace'
import {date,stateLabel,type LatencyBatch,type MonitorSample,type NodeOption} from '../domain'
import HistoryAnalyticsPanel from './HistoryAnalyticsPanel.vue'
import Icon from './Icon.vue'
import Modal from './Modal.vue'
const w=useWorkspace(),tab=ref('batch'),batch=ref<LatencyBatch|null>(null),loading=ref(false),error=ref(''),nodeId=ref(''),profile=ref(''),target=ref(''),cursor=ref(''),hasMore=ref(false),samples=ref<MonitorSample[]>([]),facets=ref<{nodes:{node_key:string;node_identity_key:string;profile_id:string;display_name:string}[];profiles:string[];targets:string[]}>({nodes:[],profiles:[],targets:[]}),legacy=ref<Record<string,any>[]>([]),legacyDetail=ref<Record<string,any>|null>(null)
const monitorNode=ref(''),facetLoading=ref(false),facetsTruncated=ref(false),facetError=ref('')
let facetGeneration=0
async function readFacets(){
 const token=++facetGeneration,from=since.value,to=until.value;facetLoading.value=true;facetError.value='';facetsTruncated.value=false
 try{const result=await api.monitorFacets(from,to);if(token!==facetGeneration||tab.value!=='monitor')return;facets.value=result;facetsTruncated.value=(result as typeof result&{truncated?:boolean}).truncated===true}
 catch(e){if(token===facetGeneration)facetError.value=e instanceof Error?e.message:String(e)}
 finally{if(token===facetGeneration)facetLoading.value=false}
}
function facetNodeValue(n:(typeof facets.value.nodes)[number]){return facets.value.nodes.filter(v=>v.node_identity_key===n.node_identity_key).length===1?n.node_identity_key:JSON.stringify([n.profile_id,n.node_identity_key,n.node_key])}
const historicalNode=computed(()=>facets.value.nodes.find(n=>facetNodeValue(n)===monitorNode.value))
const sampleProfile=computed(()=>historicalNode.value?.profile_id||profile.value)
const labels=computed(()=>Object.fromEntries(facets.value.profiles.map(id=>{const airport=w.airports.value.find(a=>a.subscriptions?.some(sub=>sub.id===id));return [id,airport?`${airport.name} / ${airport.subscriptions?.find(sub=>sub.id===id)?.name}（当前归属）`:w.nodes.value.find(n=>n.profile_id===id)?.profile_name||id]})))
const context=computed(()=>({since:since.value,until:until.value,hasMore:hasMore.value,error:error.value,loading:loading.value,profile:sampleProfile.value,identity:historicalNode.value?.node_identity_key||'',target:target.value}))
const batchTests=computed(()=>batch.value?.items?.flatMap(i=>i.result?[i.result]:[])||[])
const batchContext=computed(()=>{const times=batchTests.value.flatMap(t=>t.samples.map(s=>Date.parse(s.timestamp))).filter(Number.isFinite);return {since:new Date(times.length?Math.min(...times):Date.parse(batch.value?.requested_at||until.value)).toISOString(),until:new Date(times.length?Math.max(...times):Date.parse(batch.value?.requested_at||until.value)).toISOString(),hasMore:false}})
let generation=0;const until=ref(new Date().toISOString()),since=computed(()=>new Date(Date.parse(until.value)-w.hours.value*3600000).toISOString())
async function refresh(){loading.value=true;error.value='';const gen=++generation,mode=tab.value;try{
  if(mode==='batch'){const result=await api.batches();if(gen===generation)w.batches.value=result||[]}
  if(mode==='monitor'){until.value=new Date().toISOString();await readFacets();if(gen!==generation)return;await readSamples(false)}
  if(mode==='legacy'){const result=await request<Record<string,any>[]>('/api/history','GET',undefined,'ListHistory');if(gen===generation)legacy.value=result||[]}
}catch(e){if(gen===generation)error.value=e instanceof Error?e.message:String(e)}finally{if(gen===generation)loading.value=false}}
async function readSamples(append=false){
 error.value='';loading.value=true;const gen=++generation,previous=cursor.value
 if(!append){samples.value=[];cursor.value='';hasMore.value=false}
 try{
  const node=historicalNode.value
  if(monitorNode.value&&!node?.node_identity_key)throw new Error('历史身份缺失，暂不能检索此节点')
  const result=await api.monitorSamples({since:since.value,until:until.value,profile_id:sampleProfile.value,node_identity_key:node?.node_identity_key||'',target:target.value,cursor:append?previous:undefined,limit:100,order_desc:true})
  if(gen!==generation)return
  const combined=append?[...samples.value,...(result.items||[])]:result.items||[]
  samples.value=[...new Map(combined.map(s=>[s.sample_id,s])).values()]
  hasMore.value=result.has_more
  if(result.has_more&&(!result.next_cursor||(append&&result.next_cursor===previous)))throw new Error('历史游标缺失或停滞；统计不完整，请刷新重读')
  cursor.value=result.next_cursor||''
 }catch(e){if(gen===generation)error.value=e instanceof Error?e.message:String(e)}finally{if(gen===generation)loading.value=false}
}
function cancelRead(){generation++;facetGeneration++;loading.value=false;facetLoading.value=false;error.value='读取已取消；仅保留已读样本，统计不完整'}
let detailGeneration=0
function closeDetails(){detailGeneration++;batch.value=null;legacyDetail.value=null}
async function openBatch(id:string){const token=++detailGeneration;try{const result=await api.batch(id);if(token===detailGeneration)batch.value=result}catch(e){if(token===detailGeneration)w.fail(e)}}
async function retryBatchItem(id:string){if(!batch.value)return;const token=detailGeneration,batchId=batch.value.batch_id;try{const result=await api.retryBatch(batchId,id);if(token!==detailGeneration||batch.value?.batch_id!==batchId)return;batch.value=result;w.absorbBatch(result)}catch(e){if(token===detailGeneration)w.fail(e)}}
function detailFromItem(item:NonNullable<LatencyBatch['items']>[number]):NodeOption{return {...item,display_name:item.display_name,profile_name:w.nodes.value.find(n=>n.profile_id===item.profile_id)?.profile_name||'历史订阅',type:item.result?.node_type||'',country_code:'',country_flag:''}}
async function openLegacy(id:string){const token=++detailGeneration;try{const result=await request<Record<string,any>>(`/api/history/${encodeURIComponent(id)}`,'GET',undefined,'GetHistory',[id]);if(token===detailGeneration)legacyDetail.value=result}catch(e){if(token===detailGeneration)w.fail(e)}}
function openNode(){const n=w.nodes.value.find(n=>n.profile_id+'|'+n.node_key===nodeId.value);if(n)w.inspectNode.value=n}
watch(profile,()=>{monitorNode.value=''},{flush:'sync'});watch(tab,()=>{facetGeneration++;closeDetails();void refresh()},{immediate:true});watch([profile,target,monitorNode,()=>w.hours.value],(_next,previous)=>{if(tab.value==='monitor'){until.value=new Date().toISOString();if(w.hours.value!==previous[3])void refresh();else void readSamples(false)}})
onBeforeUnmount(()=>{generation++;facetGeneration++;detailGeneration++})
</script>
<template><section class="page-view history-view"><div class="page-header"><div><h1>检测历史</h1><p>查看已保存的检测、采样和历史配置。</p></div><button class="button ghost" :disabled="loading" @click="refresh"><Icon name="refresh"/>刷新</button></div><nav class="project-tabs"><button v-for="[id,label] in [['batch','批量检测'],['node','节点记录'],['monitor','持续监测'],['legacy','旧版记录']]" :key="id" :aria-selected="tab===id" @click="tab=id">{{label}}</button></nav><p v-if="error" class="inline-error" role="alert">{{error}} <button class="text-button" :disabled="loading" @click="refresh">重新读取</button></p><div v-if="tab==='batch'" class="history-batches history-record-list"><button v-for="b in w.batches.value" :key="b.batch_id" class="history-batch-row" @click="openBatch(b.batch_id)"><span class="history-symbol"><Icon name="pulse"/></span><span><strong>{{b.target_id==='all'?'多站延迟检测':'延迟检测'}} · {{b.item_count}} 个节点</strong><small>{{date(b.requested_at)}}<template v-if="b.sample_count"> · 计划每站 {{b.sample_count}} 次</template></small></span><span class="badge" :class="b.state==='completed'?'good':''">{{stateLabel(b.state)}}</span><Icon name="chevron"/></button><div v-if="!loading&&!w.batches.value.length" class="empty-state">还没有批量检测记录。</div></div><div v-if="tab==='node'" class="settings-section"><h2>按节点查看三类检测</h2><p class="muted">延迟、下载和服务记录在节点详情中切换。</p><div class="inline-field"><UiSelect v-model="nodeId" aria-label="选择历史节点"><option value="">选择节点</option><option v-for="n in w.nodes.value" :key="n.profile_id+n.node_key" :value="n.profile_id+'|'+n.node_key">{{n.profile_name}} · {{nodeDisplayName(n)}}</option></UiSelect><button class="button primary" :disabled="!nodeId" @click="openNode">查看记录</button></div></div><template v-if="tab==='monitor'"><div class="history-filter-row"><label>历史范围<UiSelect v-model.number="w.hours.value"><option :value="6">最近 6 小时</option><option :value="24">最近 24 小时</option><option :value="168">最近 7 天</option><option :value="720">最近 30 天</option></UiSelect></label><label>订阅<UiSelect v-model="profile"><option value="">全部订阅</option><option v-for="id in facets.profiles" :key="id" :value="id">{{labels[id]||id}}</option></UiSelect></label><label>节点<UiSelect v-model="monitorNode" aria-label="监测历史节点"><option value="">全部节点</option><option v-for="n in facets.nodes.filter(n=>!profile||n.profile_id===profile)" :key="n.profile_id+n.node_identity_key+n.node_key" :value="facetNodeValue(n)" :disabled="!n.node_identity_key">{{nodeDisplayName(n)}}{{!n.node_identity_key?' · 身份缺失，暂不能检索':!w.nodes.value.some(current=>current.profile_id===n.profile_id&& current.node_identity_key===n.node_identity_key)?' · 历史节点（当前订阅中不存在）':''}}</option></UiSelect></label><label>目标<UiSelect v-model="target"><option value="">全部目标</option><option v-for="t in facets.targets" :key="t" :value="t">{{t}}</option></UiSelect></label><button class="button ghost" @click="saveFile(JSON.stringify(samples,null,2),'speedtest-monitor-samples.json','application/json').then(saved=>{if(saved)w.notify('样本已导出')}).catch(w.fail)">导出已读样本</button></div><p v-if="facetLoading" class="small muted">读取历史筛选项…</p><p v-if="facetError" class="small warn" role="alert">筛选项读取失败：{{facetError}} <button class="text-button" @click="readFacets">重试筛选项</button></p><p v-if="facetsTruncated" class="small warn">筛选项已截断；节点／订阅／目标列表并非全部历史。</p><p class="small muted">机场标签仅为当前订阅归属，历史机场归属未记录；缺少历史身份的已删除节点暂不能检索。</p><HistoryAnalyticsPanel :samples="samples" :context="context" :profile-labels="labels"/><div class="history-sample-records" role="list" aria-label="持续监测样本">
  <article v-for="s in samples" :key="s.sample_id" class="history-sample-record" role="listitem">
    <header class="history-sample-head">
      <time class="history-sample-time">{{date(s.timestamp)}}</time>
      <div class="history-sample-node"><strong>{{nodeDisplayName({display_name:s.display_name_snapshot})}}</strong><small>配置 {{s.config_revision_key?.slice(-8)||'旧版'}}</small></div>
      <span class="history-sample-outcome" :class="s.error_class==='not_executed'?'empty':s.success?(s.error_detail?'warn':'good'):'bad'">{{s.error_class==='not_executed'?'未执行':s.success?(s.error_detail?'已测得响应时延':'成功'):s.error_class||'失败'}}</span>
    </header>
    <div class="history-sample-fields">
      <div class="history-sample-target"><small>目标</small><strong class="break-text">{{s.target}}</strong></div>
      <div><small>采样类型</small><strong>{{s.sampling_tier==='regular'?'常规':s.sampling_tier==='focus'?'重点':s.sampling_tier==='sparse'?'稀疏':s.sampling_tier==='diagnostic'?'手动诊断':'旧版'}}</strong><small>{{s.probe_type}}</small></div>
      <div><small>响应时延</small><strong class="history-sample-latency">{{s.success?`${(s.latency/1e6).toFixed(0)} ms`:'—'}}</strong></div>
    </div>
    <p v-if="s.error_detail" class="history-sample-detail">{{s.error_detail}}</p>
  </article>
</div><div v-if="!samples.length&&!loading&&!error" class="empty-state">这个范围没有监测样本。</div><button v-if="hasMore" class="button" :disabled="loading" @click="readSamples(true)">{{loading?'读取中…':'读取更早样本'}}</button></template><div v-if="tab==='legacy'" class="history-batches"><button v-for="r in legacy" :key="r.id" class="history-batch-row" @click="openLegacy(r.id)"><span class="history-symbol"><Icon name="history"/></span><span><strong>{{r.airport_name||'旧版测速'}} · {{r.total_nodes||r.node_count||0}} 个节点</strong><small>{{date(r.created_at)}}</small></span><Icon name="chevron"/></button><div v-if="!legacy.length&&!loading" class="empty-state">没有旧版测速记录。</div></div><div v-if="loading" class="loading-state">读取记录中… <button class="text-button" @click="cancelRead">取消读取</button></div><Modal v-if="batch" title="批次记录" wide @close="closeDetails"><p class="muted">{{date(batch.requested_at)}} · {{batch.item_count}} 个节点 · {{stateLabel(batch.state)}}</p><HistoryAnalyticsPanel :samples="[]" :tests="batchTests" :context="batchContext"/><details><summary>批次原始 JSON</summary><pre class="record-json">{{JSON.stringify(batch,null,2)}}</pre></details><button class="text-button" @click="saveFile(JSON.stringify(batch,null,2),'speedtest-batch-history.json','application/json').catch(w.fail)">导出批次记录</button><div v-for="item in batch.items" :key="item.item_id" class="compact-row"><button class="text-button" @click="w.inspectNode.value=detailFromItem(item);closeDetails()">{{nodeDisplayName(item)}}</button><div class="button-group"><span>{{stateLabel(item.execution_state)}} · {{stateLabel(item.persistence_state)}}</span><button v-if="item.persistence_state==='failed'&&item.result" class="text-button" @click="retryBatchItem(item.item_id)">重试保存</button><small v-else-if="item.persistence_state==='failed'" class="warn">原结果无法恢复</small></div><p v-if="item.result" class="small muted">实际采样 {{item.result.total_samples}} 次 · 成功 {{item.result.success_samples}} 次</p><p v-if="item.error_message||item.persistence_error" class="bad small">{{item.error_message||item.persistence_error}}</p></div></Modal><Modal v-if="legacyDetail" title="旧版测速记录" wide @close="closeDetails"><pre class="record-json">{{JSON.stringify(legacyDetail,null,2)}}</pre><template #footer><button class="button" @click="saveFile(JSON.stringify(legacyDetail,null,2),'speedtest-legacy-history.json','application/json').then(saved=>{if(saved)w.notify('记录已导出')}).catch(w.fail)">导出原始记录</button></template></Modal></section></template>
<style src="./secondary-observatory.css" scoped></style>
