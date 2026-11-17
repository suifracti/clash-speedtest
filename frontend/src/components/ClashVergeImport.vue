<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {api,type ClashVergeSubscription} from '../api'
import {date} from '../domain'
import {useWorkspace} from '../workspace'
import Modal from './Modal.vue'
import UiSelect from './UiSelect.vue'

const emit=defineEmits<{close:[]}>()
const w=useWorkspace(),sources=ref<ClashVergeSubscription[]>([]),targets=ref<Record<string,string>>({}),names=ref<Record<string,string>>({}),syncNames=ref<Record<string,boolean>>({}),results=ref<Record<string,string>>({}),loading=ref(false),syncing=ref(''),error=ref(''),refreshFailed=ref(false)
function syncNameFor(source:ClashVergeSubscription){return (names.value[source.uid]??source.name??'').trim()||(source.name||'').trim()||'Clash Verge 订阅'}
function targetAvailable(source:ClashVergeSubscription){return w.airports.value.some(a=>a.id===source.airport_id&&a.subscriptions?.some(s=>s.id===source.subscription_id))}
async function refreshAccounts(){
 refreshFailed.value=false;error.value=''
 try{if(w.setup.value&&w.setup.value.state!=='ready')await w.boot();else await w.refreshNodes()}
 catch(e){refreshFailed.value=true;error.value=`已导入，但列表刷新失败：${e instanceof Error?e.message:String(e)}。无需再次导入，可重试加载账户。`}
}
async function load(){
 loading.value=true;error.value=''
 try{
  sources.value=await api.vergeSubscriptions()
  results.value={}
  targets.value=Object.fromEntries(sources.value.map(source=>[source.uid,source.airport_id&&source.subscription_id?`${source.airport_id}:${source.subscription_id}`:w.airports.value.length?'':'new']))
  names.value=Object.fromEntries(sources.value.map(source=>[source.uid,source.name||'Clash Verge 订阅']))
  syncNames.value=Object.fromEntries(sources.value.map(source=>[source.uid,!!source.name_changed]))
 }catch(e){error.value=e instanceof Error?e.message:String(e)}
 finally{loading.value=false}
}
async function sync(source:ClashVergeSubscription){
 const target=targets.value[source.uid]
 if(!target||syncing.value)return
 syncing.value=source.uid;error.value='';results.value[source.uid]=''
 try{
  const [airport_id,subscription_id]=target.split(':')
  const customName=syncNameFor(source),defaultName=(source.name||'').trim()||'Clash Verge 订阅',wantSyncName=!!syncNames.value[source.uid]||customName!==defaultName
  const result=await api.importVergeSubscription({uid:source.uid,...(target==='new'?{}:{airport_id,subscription_id}),...(customName!==defaultName?{name:customName}:wantSyncName&&target!=='new'?{sync_name:true}:{})})
  const syncedName=result.name||customName
  source.airport_id=result.airport_id;source.subscription_id=result.id;source.linked=true;source.url_changed=false;source.name_changed=false
  names.value[source.uid]=syncedName
  targets.value[source.uid]=`${result.airport_id}:${result.id}`
  results.value[source.uid]=`已同步 ${result.node_count} 个节点（名称：${syncedName}）`
  w.notify(`已导入「${syncedName}」的 ${result.node_count} 个节点`)
  await refreshAccounts()
 }catch(e){error.value=e instanceof Error?e.message:String(e)}
 finally{syncing.value=''}
}
onMounted(()=>void load())
</script>

<template>
 <Modal title="从 Clash Verge 导入／同步" wide :closable="!syncing" @close="emit('close')">
  <p class="verge-intro">把本机 Clash Verge 已保存的节点带入账户台账。为每份订阅选定位置，之后会持续同步到这个账户。</p>
  <p v-if="error" class="inline-error" role="alert">{{error}}</p>
  <button v-if="refreshFailed" class="button" :disabled="!!syncing||loading" @click="refreshAccounts">重试加载账户</button>
  <p v-if="loading" role="status">正在读取本机订阅…</p>
  <p v-else-if="!sources.length&&!error" class="muted">Clash Verge 中还没有远程订阅。</p>
  <div v-if="sources.length" class="verge-list">
   <div v-for="source in sources" :key="source.uid" class="verge-source" :class="{linked:source.linked}">
    <div class="verge-name">
     <span class="verge-field-label">Clash Verge 来源</span>
     <strong>{{source.name||'Clash Verge 订阅'}}</strong>
     <small>来源更新 {{date(source.updated_at)}}</small>
     <small :class="source.url_changed||source.name_changed?'warn':'muted'">{{source.linked?(source.url_changed&&source.name_changed?'链接与名称已变化，将同步原账户':source.url_changed?'链接已变化，将更新原账户':source.name_changed?'名称已变化，可勾选同步名称':'已关联原账户'):source.subscription_id?'找到相同链接，可关联原账户':'请选择已有账户或新建机场'}}</small>
    </div>
    <div class="verge-destination">
    <label class="verge-target"><span class="verge-field-label">导入到 CST 账户</span>
    <UiSelect v-model="targets[source.uid]" :disabled="!!syncing||source.linked" :aria-label="source.name+' 导入位置'">
     <option value="">选择导入位置</option>
     <option value="new">新建机场（{{syncNameFor(source)}}）</option>
     <template v-for="airport in w.airports.value" :key="airport.id"><option v-for="account in airport.subscriptions||[]" :key="account.id" :value="airport.id+':'+account.id">{{airport.name}} · {{account.name}}</option></template>
     <option v-if="source.linked&&!targetAvailable(source)" :value="source.airport_id+':'+source.subscription_id">已关联账户 · {{names[source.uid]}}</option>
    </UiSelect>
    </label>
    <div class="verge-name-box">
     <label><span class="verge-field-label">账户名称</span><input v-model="names[source.uid]" class="verge-name-input" :disabled="!!syncing" :aria-label="(source.name||'Clash Verge 订阅')+' 同步名称'" placeholder="同步名称"></label>
     <label v-if="targets[source.uid]&&targets[source.uid]!=='new'" class="verge-sync-name-toggle"><input v-model="syncNames[source.uid]" type="checkbox" :disabled="!!syncing">同时更新账户名称</label>
     <small v-if="syncNameFor(source)!==(source.name||'').trim()" class="muted">自定义名称会一并保存到目标账户。</small>
    </div>
    </div>
    <div class="verge-action"><span class="muted small">{{source.linked?'已固定同步位置':'首次导入'}}</span><button class="button primary" :disabled="loading||!!syncing||!targets[source.uid]" @click="sync(source)">{{syncing===source.uid?'同步中…':source.linked?'同步':'导入'}}</button></div>
    <p v-if="results[source.uid]" class="good small verge-result" role="status">{{results[source.uid]}}</p>
   </div>
  </div>
  <template #footer><button class="button" :disabled="loading||!!syncing" @click="load">重新读取列表</button><button class="button" :disabled="!!syncing" @click="emit('close')">完成</button></template>
 </Modal>
</template>

<style scoped>
.verge-intro{max-width:62ch;line-height:1.8;color:var(--cst-muted);margin:0 0 24px}
.verge-list{border-top:1px solid var(--cst-line);background:var(--cst-panel)}
.verge-source{display:grid;grid-template-columns:minmax(140px,.85fr) minmax(0,1.3fr);gap:18px 28px;padding:24px 0;border-bottom:1px solid var(--cst-line)}
.verge-source:last-child{border-bottom:0}
.verge-field-label{display:block;font-size:12px;font-weight:500;color:var(--cst-muted);margin-bottom:7px}
.verge-name{display:flex;flex-direction:column;gap:5px;min-width:0;overflow-wrap:anywhere}
.verge-name strong{font-size:17px;font-weight:650;color:var(--cst-text);line-height:1.6}
.verge-name small{font-size:11.5px;color:var(--cst-muted)}
.verge-name small.warn{color:var(--cst-warn);font-weight:500}
.verge-destination{border-left:2px solid var(--cst-line);padding-left:24px;display:grid;gap:14px;min-width:0}
.verge-target{min-width:0}.verge-target .ui-select{width:100%}
.verge-source.linked .verge-target :deep(.ui-select-trigger:disabled){opacity:1;color:var(--cst-text);background:var(--cst-panel-2)}
.verge-name-box{display:flex;flex-direction:column;gap:7px;min-width:0}
.verge-name-input{width:100%;min-width:0;height:36px;padding:6px 10px;font-size:13px}
.verge-sync-name-toggle{display:inline-flex;align-items:center;gap:6px;font-size:11.5px;font-weight:500;color:var(--cst-muted);cursor:pointer;user-select:none}
.verge-sync-name-toggle input[type=checkbox]{width:14px;height:14px;min-height:0;padding:0;margin:0;flex:none}
.verge-action{grid-column:2;display:flex;align-items:center;justify-content:space-between;gap:12px;padding-left:26px}
.verge-result{grid-column:1/-1;margin:0;color:var(--cst-good);font-weight:500}
@media(max-width:640px){.verge-source{grid-template-columns:minmax(0,1fr);gap:16px}.verge-destination{padding-left:14px}.verge-action{grid-column:1;padding-left:16px}.verge-name strong{font-size:16px}}
</style>
