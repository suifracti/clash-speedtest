<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {api,type ClashVergeSubscription} from '../api'
import {date} from '../domain'
import {useWorkspace} from '../workspace'
import Modal from './Modal.vue'
import UiSelect from './UiSelect.vue'

const emit=defineEmits<{close:[]}>()
const w=useWorkspace(),sources=ref<ClashVergeSubscription[]>([]),targets=ref<Record<string,string>>({}),names=ref<Record<string,string>>({}),syncNames=ref<Record<string,boolean>>({}),results=ref<Record<string,string>>({}),loading=ref(false),syncing=ref(''),error=ref('')
function syncNameFor(source:ClashVergeSubscription){return (names.value[source.uid]??source.name??'').trim()||(source.name||'').trim()||'Clash Verge 订阅'}
async function load(){
 loading.value=true;error.value=''
 try{
  sources.value=await api.vergeSubscriptions()
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
  source.name=syncedName;source.airport_id=result.airport_id;source.subscription_id=result.id;source.linked=true;source.url_changed=false;source.name_changed=false
  names.value[source.uid]=syncedName
  targets.value[source.uid]=`${result.airport_id}:${result.id}`
  results.value[source.uid]=`已同步 ${result.node_count} 个节点（名称：${syncedName}）`
  if(w.setup.value&&w.setup.value.state!=='ready')await w.boot();else await w.refreshNodes()
 }catch(e){error.value=e instanceof Error?e.message:String(e)}
 finally{syncing.value=''}
}
onMounted(()=>void load())
</script>

<template>
 <Modal title="从 Clash Verge 导入／同步" wide :closable="!syncing" @close="emit('close')">
  <p class="muted">使用 Clash Verge 本机已保存的订阅和节点。首次可关联已有账户或新建机场，支持同步或自定义订阅名称。</p>
  <p class="muted small">只读取订阅信息与代理节点，来源更新时间见各行，可在导入或同步前直接修改同步名称。</p>
  <p v-if="error" class="inline-error" role="alert">{{error}}</p>
  <p v-if="loading" role="status">正在读取本机订阅…</p>
  <p v-else-if="!sources.length&&!error" class="muted">Clash Verge 中还没有远程订阅。</p>
  <div v-for="source in sources" :key="source.uid" class="verge-source">
   <div class="verge-name">
    <strong>{{source.name||'Clash Verge 订阅'}}</strong>
    <small>来源更新 {{date(source.updated_at)}}</small>
    <small :class="source.url_changed||source.name_changed?'warn':'muted'">{{source.linked?(source.url_changed&&source.name_changed?'链接与名称已变化，将同步原账户':source.url_changed?'链接已变化，将更新原账户':source.name_changed?'名称已变化，可勾选同步名称':'已关联原账户'):source.subscription_id?'找到相同链接，可关联原账户':'请选择已有账户或新建机场'}}</small>
   </div>
   <div class="verge-name-box">
    <input v-model="names[source.uid]" class="verge-name-input" :disabled="!!syncing" :aria-label="(source.name||'Clash Verge 订阅')+' 同步名称'" placeholder="同步名称">
    <label v-if="targets[source.uid]&&targets[source.uid]!=='new'" class="verge-sync-name-toggle"><input v-model="syncNames[source.uid]" type="checkbox" :disabled="!!syncing">同步名字</label>
   </div>
   <UiSelect v-model="targets[source.uid]" :disabled="!!syncing||source.linked" :aria-label="source.name+' 导入位置'">
    <option value="">选择导入位置</option>
    <option value="new">新建机场（{{syncNameFor(source)}}）</option>
    <template v-for="airport in w.airports.value" :key="airport.id"><option v-for="account in airport.subscriptions||[]" :key="account.id" :value="airport.id+':'+account.id">{{airport.name}} · {{account.name}}</option></template>
   </UiSelect>
   <button class="button primary" :disabled="loading||!!syncing||!targets[source.uid]" @click="sync(source)">{{syncing===source.uid?'同步中…':source.linked?'同步':'导入'}}</button>
   <p v-if="results[source.uid]" class="good small verge-result" role="status">{{results[source.uid]}}</p>
  </div>
  <template #footer><button class="button" :disabled="loading||!!syncing" @click="load">重新读取列表</button><button class="button" :disabled="!!syncing" @click="emit('close')">完成</button></template>
 </Modal>
</template>

<style scoped>
.verge-source{display:grid;grid-template-columns:minmax(140px,1fr) minmax(150px,1fr) minmax(180px,1.2fr) auto;align-items:center;gap:10px;padding:14px 0;border-bottom:1px solid var(--line)}
.verge-name{display:flex;flex-direction:column;gap:4px;min-width:0;overflow-wrap:anywhere}.verge-name small{font-size:11px;color:var(--muted)}.verge-name small.warn{color:var(--amber)}.verge-name-box{display:flex;flex-direction:column;gap:4px;min-width:0}.verge-name-input{width:100%;min-width:0}.verge-sync-name-toggle{display:inline-flex;align-items:center;gap:6px;font-size:12px;color:var(--ink);cursor:pointer}.verge-sync-name-toggle input[type=checkbox]{width:14px;height:14px;min-height:0;padding:0;margin:0;flex:none}.verge-source>.ui-select{width:100%}.verge-result{grid-column:1/-1;margin:0}
@media(max-width:720px){.verge-source{grid-template-columns:minmax(0,1fr) auto}.verge-name,.verge-name-box{grid-column:1/-1}}
</style>
