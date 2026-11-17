<script setup lang="ts">
import {computed,onBeforeUnmount,ref} from 'vue'
import {api,request,saveFile} from '../api'
import {inspectMonitorImport,monitorConfigByteLimit,monitorProbeName,monitorTierName,parseMonitorConfig,serializeMonitorConfig,type MonitorImportRow} from '../monitorTransfer'
import {useWorkspace} from '../workspace'
import Icon from './Icon.vue'
import Modal from './Modal.vue'

interface ImportRow extends MonitorImportRow {selected:boolean;imported:boolean;createError:string}
const emit=defineEmits<{imported:[]}>()
const w=useWorkspace(),open=ref(false),busy=ref(false),operation=ref(''),error=ref(''),feedback=ref(''),filename=ref('')
const fileInput=ref<HTMLInputElement>(),rows=ref<ImportRow[]>([])
const pending=computed(()=>rows.value.filter(row=>row.selected&&!row.imported&&!row.error))
const importedCount=computed(()=>rows.value.filter(row=>row.imported).length)
let alive=true
onBeforeUnmount(()=>{alive=false})
const message=(cause:unknown)=>cause instanceof Error?cause.message:String(cause)
function show(){open.value=true;rows.value=[];filename.value='';error.value='';feedback.value=''}
function close(){if(!busy.value)open.value=false}
function dataPage(){if(busy.value)return;open.value=false;w.page.value='data'}
async function perform(label:string,action:()=>Promise<void>){
  if(busy.value)return
  busy.value=true;operation.value=label;error.value='';feedback.value=''
  try{await action()}catch(cause){if(alive)error.value=message(cause)}
  finally{if(alive){busy.value=false;operation.value=''}}
}
async function exportConfig(){
  await perform('读取任务配置',async()=>{
    const jobs=await api.jobs()
    if(!alive)return
    if(!jobs?.length){feedback.value='当前没有可导出的监测任务';return}
    const contents=serializeMonitorConfig(jobs)
    operation.value='导出任务配置'
    const saved=await saveFile(contents,'monitor-config.json','application/json')
    if(alive)feedback.value=saved?`已导出 ${jobs.length} 项任务配置`:'已取消导出'
  })
}
async function inspectFile(event:Event){
  const input=event.target as HTMLInputElement,file=input.files?.[0];input.value=''
  if(!file||busy.value)return
  rows.value=[];filename.value=file.name
  await perform('检查配置文件',async()=>{
    if(file.size>monitorConfigByteLimit)throw new Error('配置文件不能超过 1 MiB')
    const requests=parseMonitorConfig(await file.text())
    if(!alive)return
    const nodes=await api.nodes()
    if(!alive)return
    rows.value=inspectMonitorImport(requests,nodes||[]).map(row=>({...row,selected:!row.error,imported:false,createError:''}))
    feedback.value=pending.value.length?`已核对当前订阅缓存，${pending.value.length} 项任务的身份与配置匹配。确认前不会写入。`:'没有匹配当前订阅的任务，请先核对下方原因。'
  })
}
async function importConfig(){
  if(busy.value||!pending.value.length)return
  const selected=[...pending.value]
  let count=0
  await perform('重新核对节点',async()=>{
    // Recheck the source immediately before creation. The backend also checks
    // each frozen context, covering a cache change between individual writes.
    const checked=inspectMonitorImport(selected.map(row=>row.request),await api.nodes()||[])
    if(!alive)return
    checked.forEach((current,index)=>{const row=selected[index];row.error=current.error;row.profileName=current.profileName;row.createError='';if(current.error)row.selected=false})
    if(checked.some(row=>row.error))throw new Error('订阅缓存已变化，请核对标出的任务后重新确认剩余项。尚未新增任务。')
    for(const row of selected){
      if(!alive)break
      operation.value=`新增任务 ${count+1} / ${selected.length}`
      try{
        const created=await api.createJob(row.request)
        if(!alive)break
        row.imported=true;row.selected=false;count++
        if(created?.id&&!w.jobs.value.some(job=>job.id===created.id))w.jobs.value=[...w.jobs.value,created]
      }catch(cause){
        if(!alive)break
        row.createError=message(cause)
        throw new Error(`本次已新增 ${count} 项；其余尚未确认新增。${row.request.name||'未命名任务'}：${row.createError}`)
      }
    }
    if(alive)feedback.value=`本次已新增 ${count} 项任务，均为停止状态；原任务保持不变。`
  })
  if(alive&&count)emit('imported')
}
async function openFolder(){await perform('打开监测数据目录',async()=>{await request('/api/data/open-folder?kind=monitor','POST',undefined,'OpenMonitorDataFolder');if(alive)feedback.value='已请求打开监测数据目录。数据库运行期间请勿直接编辑文件。'})}
</script>

<template>
  <button class="button ghost" @click="show"><Icon name="download"/>任务配置</button>
  <Modal v-if="open" title="监测任务配置" wide :closable="!busy" @close="close">
    <div class="monitor-transfer" :aria-busy="busy">
      <p class="transfer-intro">把任务配置带到另一份数据目录，或留存一份配置。订阅 URL、密钥、历史样本和运行状态不写入这个文件。</p>
      <div class="transfer-file-actions">
        <button class="button primary" :disabled="busy" @click="fileInput?.click()">选择配置文件</button>
        <input ref="fileInput" type="file" accept=".json,application/json" aria-label="监测任务配置文件" :disabled="busy" hidden @change="inspectFile">
        <button class="button" :disabled="busy" @click="exportConfig"><Icon name="download"/>导出当前任务</button>
        <span>JSON · 不超过 1 MiB</span>
      </div>
      <div v-if="filename" class="transfer-file-name"><strong>{{filename}}</strong><span v-if="rows.length">{{rows.length}} 项任务<template v-if="importedCount"> · 已新增 {{importedCount}} 项</template></span></div>
      <p class="transfer-status" role="status" aria-live="polite">{{busy?`正在${operation}…`:feedback||(importedCount?'已新增的任务不会再次提交。':'导入先核对，再选择要新增的任务。')}} </p>
      <p v-if="error" class="inline-error" role="alert">{{error}}</p>
      <div v-if="rows.length" class="transfer-ledger" aria-label="待导入任务">
        <div class="transfer-columns" aria-hidden="true"><span>任务 / 对应订阅</span><span>检测安排</span><span>核对结果</span></div>
        <article v-for="(row,index) in rows" :key="index" class="transfer-row" :class="{'transfer-invalid':row.error,'transfer-imported':row.imported}">
          <label class="transfer-task"><input v-model="row.selected" type="checkbox" :disabled="busy||row.imported||!!row.error" :aria-label="`新增 ${row.request.name||'未命名任务'}`"><span><strong>{{row.request.name||'未命名任务'}}</strong><small>{{row.profileName}}</small></span></label>
          <div class="transfer-schedule"><span>{{row.request.node_keys.length}} 个节点 · {{monitorProbeName(row.request.probe_set)}}</span><small>{{monitorTierName(row.request.sampling_tier)}} · 每 {{row.request.interval_seconds}} 秒 · 超时 {{row.request.timeout_seconds}} 秒</small></div>
          <div class="transfer-check"><strong :class="{'transfer-check-error':row.error||row.createError}">{{row.imported?'已新增 · 停止':row.error?'无法匹配':row.createError?'新增未确认':'身份与配置一致'}}</strong><small v-if="row.error||row.createError">{{row.error||row.createError}}</small></div>
          <details class="transfer-identity"><summary>查看节点身份与配置版本</summary><ul><li v-for="node in row.request.node_contexts" :key="node.node_key"><code>{{node.node_key}} · {{node.node_identity_key}} · {{node.config_revision_key}}</code></li></ul></details>
        </article>
      </div>
      <p class="transfer-note">新增任务保持停止，不覆盖已有任务。相同配置已存在时会由后端拒绝；部分成功后，继续提交仅处理尚未新增的所选任务。</p>
      <div class="transfer-data-links"><button class="text-button" :disabled="busy" @click="openFolder">打开监测数据目录</button><button class="text-button" :disabled="busy" @click="dataPage">完整数据与备份 <Icon name="chevron"/></button></div>
    </div>
    <template #footer><span class="transfer-footer-count">{{pending.length?`已选 ${pending.length} 项`:importedCount?`已新增 ${importedCount} 项`:'尚未选择任务'}}</span><button class="button" :disabled="busy" @click="close">{{importedCount?'完成':'返回'}}</button><button v-if="rows.some(row=>!row.imported)" class="button primary" :disabled="busy||!pending.length" @click="importConfig">{{busy?`正在${operation}…`:`新增所选 ${pending.length} 项（不启动）`}}</button></template>
  </Modal>
</template>

<style scoped>
.monitor-transfer{font-size:13px;line-height:1.65}.transfer-intro{margin:0 0 18px;color:var(--cst-muted);max-width:62ch}.transfer-file-actions{display:flex;align-items:center;flex-wrap:wrap;gap:10px}.transfer-file-actions>span{font-size:11px;color:var(--cst-muted)}.transfer-file-name{display:flex;justify-content:space-between;flex-wrap:wrap;gap:8px;margin-top:24px;overflow-wrap:anywhere}.transfer-file-name span{font-size:12px;color:var(--cst-muted)}.transfer-status{color:var(--cst-muted);margin:12px 0;min-height:22px}.transfer-columns,.transfer-row{display:grid;grid-template-columns:minmax(0,1.2fr) minmax(0,1fr) minmax(0,1fr);gap:8px 18px}.transfer-columns{padding:9px 0;border-bottom:1px solid var(--cst-line);color:var(--cst-muted);font-size:11px}.transfer-row{padding:15px 0;border-bottom:1px solid var(--cst-line);align-items:start;overflow-wrap:anywhere}.transfer-task{display:flex;align-items:flex-start;gap:10px;cursor:pointer;min-width:0}.transfer-task input{width:15px;height:15px;flex-shrink:0;margin-top:4px}.transfer-task strong{font-weight:600}.transfer-row small{display:block;font-size:11px;color:var(--cst-muted);margin-top:3px}.transfer-check strong{font-size:12px;font-weight:500}.transfer-check-error{color:var(--cst-bad)}.transfer-imported .transfer-check strong{color:var(--cst-good)}.transfer-identity{grid-column:1/-1;margin-left:25px;font-size:11px;color:var(--cst-muted)}.transfer-identity summary{cursor:pointer;width:fit-content}.transfer-identity ul{list-style:none;margin:8px 0 0;padding:0}.transfer-identity li{margin:4px 0}.transfer-note{color:var(--cst-muted);font-size:12px;margin:18px 0 16px}.transfer-data-links{display:flex;justify-content:space-between;gap:10px;flex-wrap:wrap}.transfer-data-links .text-button{display:inline-flex;align-items:center;gap:4px;font-size:12px}.transfer-footer-count{margin-right:auto;font-size:12px;color:var(--cst-muted)}.transfer-task:has(input:focus-visible){outline:2px solid var(--cst-accent);outline-offset:4px}.inline-error{overflow-wrap:anywhere}
@media(max-width:680px){.transfer-columns{display:none}.transfer-row{grid-template-columns:minmax(0,1fr);gap:8px}.transfer-schedule,.transfer-check{margin-left:25px}.transfer-file-actions .button{flex:1}.transfer-file-actions>span{width:100%}.transfer-footer-count{flex:1 0 100%}.transfer-file-name{margin-top:18px}}
</style>
