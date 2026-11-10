<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {api,type ClashVergeSubscription} from '../api'
import {date} from '../domain'
import {useWorkspace} from '../workspace'
import Modal from './Modal.vue'
import UiSelect from './UiSelect.vue'

const emit=defineEmits<{close:[]}>()
const w=useWorkspace(),sources=ref<ClashVergeSubscription[]>([]),targets=ref<Record<string,string>>({}),results=ref<Record<string,string>>({}),loading=ref(false),syncing=ref(''),error=ref('')
async function load(){
 loading.value=true;error.value=''
 try{
  sources.value=await api.vergeSubscriptions()
  targets.value=Object.fromEntries(sources.value.map(source=>[source.uid,source.airport_id&&source.subscription_id?`${source.airport_id}:${source.subscription_id}`:'']))
 }catch(e){error.value=e instanceof Error?e.message:String(e)}
 finally{loading.value=false}
}
async function sync(source:ClashVergeSubscription){
 const target=targets.value[source.uid]
 if(!target||syncing.value)return
 syncing.value=source.uid;error.value='';results.value[source.uid]=''
 try{
  const [airport_id,subscription_id]=target.split(':')
  const result=await api.importVergeSubscription({uid:source.uid,...(target==='new'?{}:{airport_id,subscription_id})})
  source.airport_id=result.airport_id;source.subscription_id=result.id;source.linked=true;source.url_changed=false
  targets.value[source.uid]=`${result.airport_id}:${result.id}`
  results.value[source.uid]=`已同步 ${result.node_count} 个节点`
  await w.refreshNodes()
 }catch(e){error.value=e instanceof Error?e.message:String(e)}
 finally{syncing.value=''}
}
onMounted(()=>void load())
</script>

<template>
 <Modal title="从 Clash Verge 导入／同步" wide :closable="!syncing" @close="emit('close')">
  <p class="muted">使用 Clash Verge 本机已保存的订阅和节点。首次可关联已有账户，之后链接变化时同步到原账户。</p>
  <p class="muted small">只读取订阅信息与代理节点，来源更新时间见各行。</p>
  <p v-if="error" class="inline-error" role="alert">{{error}}</p>
  <p v-if="loading" role="status">正在读取本机订阅…</p>
  <p v-else-if="!sources.length&&!error" class="muted">Clash Verge 中还没有远程订阅。</p>
  <div v-for="source in sources" :key="source.uid" class="verge-source">
   <div class="verge-name">
    <strong>{{source.name||'Clash Verge 订阅'}}</strong>
    <small>来源更新 {{date(source.updated_at)}}</small>
    <small :class="source.url_changed?'warn':'muted'">{{source.linked?(source.url_changed?'链接已变化，将更新原账户':'已关联原账户'):source.subscription_id?'找到相同链接，可关联原账户':'请选择已有账户或新建机场'}}</small>
   </div>
   <UiSelect v-model="targets[source.uid]" :disabled="!!syncing||source.linked" :aria-label="source.name+' 导入位置'">
    <option value="">选择导入位置</option>
    <option value="new">新建机场</option>
    <template v-for="airport in w.airports.value" :key="airport.id"><option v-for="account in airport.subscriptions||[]" :key="account.id" :value="airport.id+':'+account.id">{{airport.name}} · {{account.name}}</option></template>
   </UiSelect>
   <button class="button primary" :disabled="loading||!!syncing||!targets[source.uid]" @click="sync(source)">{{syncing===source.uid?'同步中…':source.linked?'同步':'导入'}}</button>
   <p v-if="results[source.uid]" class="good small verge-result" role="status">{{results[source.uid]}}</p>
  </div>
  <template #footer><button class="button" :disabled="loading||!!syncing" @click="load">重新读取列表</button><button class="button" :disabled="!!syncing" @click="emit('close')">完成</button></template>
 </Modal>
</template>

<style scoped>
.verge-source{display:grid;grid-template-columns:minmax(160px,1fr) minmax(190px,1.3fr) auto;align-items:center;gap:12px;padding:14px 0;border-bottom:1px solid var(--line)}
.verge-name{display:flex;flex-direction:column;gap:4px;min-width:0;overflow-wrap:anywhere}.verge-name small{font-size:11px;color:var(--muted)}.verge-name small.warn{color:var(--amber)}.verge-source>.ui-select{width:100%}.verge-result{grid-column:1/-1;margin:0}
@media(max-width:640px){.verge-source{grid-template-columns:minmax(0,1fr) auto}.verge-name{grid-column:1/-1}}
</style>
