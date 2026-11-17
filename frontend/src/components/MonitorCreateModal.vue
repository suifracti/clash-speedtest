<script setup lang="ts">
import {nodeDisplayName} from '../nodePresentation'
import UiSelect from './UiSelect.vue'
import {computed,ref} from 'vue'
import {api} from '../api'
import {useWorkspace} from '../workspace'
import {key,monitorGroups,type MonitorJob} from '../domain'
import Modal from './Modal.vue'
const w=useWorkspace(),name=ref('常用节点监测'),interval=ref(300),timeout=ref(5),probe=ref('latency_four_v1'),tier=ref('regular'),startNow=ref(true),resume=ref(false),working=ref(false),error=ref(''),selectionChanged=ref(false),created=ref<Record<string,MonitorJob>>({}),started=ref<string[]>([]),configured=ref<string[]>([])
const groups=computed(()=>monitorGroups(w.monitorNodes.value||[],name.value,interval.value,timeout.value,probe.value,tier.value))
function requireCurrentSelection(){const available=new Map(w.proxyNodes.value.map(n=>[key(n),n.node_key])),selected=w.monitorNodes.value||[];if(!selected.length||selected.some(n=>available.get(key(n))!==n.node_key)){selectionChanged.value=true;throw new Error('所选节点范围或配置已变化，请返回重新选择。')}}
async function create(){error.value='';selectionChanged.value=false;if(!name.value.trim()){error.value='请填写任务名称';return}working.value=true;try{requireCurrentSelection();for(const g of groups.value){requireCurrentSelection();let job=created.value[g.profile_id];if(!job){job=await api.createJob(g);created.value[g.profile_id]=job}requireCurrentSelection();if(!configured.value.includes(job.id)){await api.jobAction(job.id,'resume-on-launch',{enabled:resume.value});configured.value.push(job.id)}requireCurrentSelection();if(startNow.value&&!started.value.includes(job.id)){await api.jobAction(job.id,'start');started.value.push(job.id)}}await w.refreshJobs();w.notify(`已创建 ${Object.keys(created.value).length} 个监测任务`);w.monitorNodes.value=null;w.page.value='monitor'}catch(e){error.value=e instanceof Error?e.message:String(e);await w.refreshJobs().catch(()=>{})}finally{working.value=false}}
</script>
<template>
 <Modal title="加入持续监测" wide :closable="!working" @close="w.monitorNodes.value=null">
  <p class="modal-intro">已选 {{w.monitorNodes.value?.length}} 个节点，按订阅创建 {{groups.length}} 个任务。</p>
  <div class="monitor-create-layout">
   <fieldset :disabled="working||Object.keys(created).length>0" class="plain-fieldset monitor-create-editor">
    <h3>怎样监测</h3>
    <div class="form-grid">
     <label class="monitor-create-name">任务名称<input v-model="name" maxlength="100" placeholder="例如：常用节点监测"></label>
     <label class="monitor-create-name">检测项目<UiSelect v-model="probe"><option value="latency_four_v1">四站延迟 · Cloudflare / Google / GitHub / Apple</option><option value="light">延迟与连接 · Cloudflare</option><option value="service">Cloudflare / Google / GitHub</option><option value="heavy">深度诊断 · 含流量采样</option></UiSelect></label>
     <label>检测间隔<UiSelect v-model.number="interval"><option :value="30">每 30 秒</option><option :value="60">每 1 分钟</option><option :value="300">每 5 分钟</option><option :value="900">每 15 分钟</option><option :value="3600">每 1 小时</option></UiSelect></label>
     <label>单项超时<UiSelect v-model.number="timeout"><option v-for="n in [3,5,10,20,30]" :key="n" :value="n">{{n}} 秒</option></UiSelect></label>
     <label class="monitor-create-name">采样类型<UiSelect v-model="tier"><option value="regular">常规监测</option><option value="focus">重点监测</option><option value="sparse">稀疏监测</option></UiSelect></label>
    </div>
    <div class="monitor-create-switches"><label class="check-field"><input v-model="startNow" type="checkbox">创建后立即启动</label><label class="check-field"><input v-model="resume" type="checkbox">应用下次启动时继续运行</label></div>
    <p class="muted small">启动与恢复仍受共享请求预算和历史容量保护限制，任务页会显示未启动的原因。</p>
   </fieldset>
   <aside class="monitor-create-scope" aria-label="本次监测范围">
    <h3>要监测哪些节点</h3><p>范围固定在创建时；订阅更新不会自动添加其他节点。</p>
    <section class="monitor-group" v-for="g in groups" :key="g.profile_id">
     <header><strong>{{g.name}}</strong><span class="badge" :class="created[g.profile_id]?'good':''">{{created[g.profile_id]?started.includes(created[g.profile_id].id)?'已启动':'已创建':'待创建'}} · {{g.node_keys.length}} 个节点</span></header>
     <small>{{w.nodeSubscriptionName(w.monitorNodes.value!.find(n=>n.profile_id===g.profile_id)!)}}</small>
     <ul><li v-for="node in w.monitorNodes.value?.filter(n=>n.profile_id===g.profile_id)" :key="node.node_key">{{nodeDisplayName(node)}}</li></ul>
    </section>
   </aside>
  </div>
  <p v-if="error" class="inline-error" role="alert">{{error}}<span v-if="Object.keys(created).length">{{selectionChanged?'已成功创建的任务保留，可在持续监测页查看。':'。已成功创建的任务保留，再次提交只继续未完成部分。'}}</span></p>
  <template #footer><button class="button" :disabled="working" @click="w.monitorNodes.value=null">返回</button><button class="button primary" :disabled="working||!groups.length" @click="create">{{working?'正在创建…':Object.keys(created).length?'继续未完成任务':`创建 ${groups.length} 个任务`}}</button></template>
 </Modal>
</template>
<style scoped>
.monitor-create-layout{display:grid;grid-template-columns:minmax(0,1.1fr) minmax(260px,1fr);gap:30px;align-items:start}
.monitor-create-layout h3{font-size:14px;margin:0 0 18px}.monitor-create-editor{min-width:0}.monitor-create-name{grid-column:1/-1}
.monitor-create-editor .form-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.monitor-create-switches{display:grid;gap:12px;margin:22px 0 16px}.monitor-create-editor>p{line-height:1.8}
.monitor-create-scope{min-width:0;border-left:1px solid var(--cst-line);padding-left:26px}.monitor-create-scope>p{margin:0 0 20px;font-size:12px;line-height:1.8;color:var(--cst-muted)}
.monitor-create-scope .monitor-group{margin:0;padding:16px 0;border:0;border-top:1px solid var(--cst-line);background:none;border-radius:0}
.monitor-group header{display:flex;justify-content:space-between;gap:10px;align-items:flex-start;font-size:13px}.monitor-group small{display:block;margin-top:6px;color:var(--cst-muted);font-size:11px}
.monitor-group ul{display:grid;gap:8px;list-style:none;padding:0;margin:16px 0 0;font-size:12px;max-height:220px;overflow:auto}.monitor-group li{overflow-wrap:anywhere;padding-left:10px;border-left:2px solid var(--cst-line-strong)}
@media(max-width:720px){.monitor-create-layout{grid-template-columns:minmax(0,1fr);gap:20px}.monitor-create-scope{border:0;border-top:1px solid var(--cst-line);padding:20px 0 0}}
</style>
