<script setup lang="ts">
import {computed,nextTick,ref,watch} from 'vue'
import {api,backup,desktop,request,selectProfileSourceDirectory} from '../api'
import {useWorkspace} from '../workspace'
import Icon from './Icon.vue'
import Modal from './Modal.vue'
const w=useWorkspace(),source=ref(''),inspected=ref<Record<string,any>|null>(null),inspectedPath=ref(''),busy=ref(false),error=ref(''),feedback=ref(''),operation=ref('')
const refreshButton=ref<HTMLButtonElement|null>(null)
let dialogTrigger:HTMLElement|null=null
function restoreFocus(){void nextTick(()=>{const target=dialogTrigger?.isConnected?dialogTrigger:refreshButton.value;target?.focus()})}
const confirmation=ref<'import'|'migration'|null>(null)
const locked=computed(()=>busy.value||w.loading.value)
const pageError=computed(()=>error.value||w.setup.value?.error||w.error.value)
const setupLabel=computed(()=>w.loading.value?'加载中':(w.setup.value?.error||(!w.setup.value&&pageError.value))?'读取失败':!w.setup.value?'尚未读取':({ready:'已就绪',uninitialized:'待初始化',needs_setup:'待初始化',needs_choice:'待初始化',error:'读取失败',failed:'读取失败'} as Record<string,string>)[w.setup.value.state]||`未确认 · ${w.setup.value.state}`)
const migration=computed(()=>w.setup.value?.migration)
const migrationLabel=computed(()=>({pending:'待迁移',conflict:'存在冲突',failed:'迁移失败',error:'迁移失败',unavailable:'来源不可用',invalid:'来源无效'} as Record<string,string>)[migration.value?.state||'']||migration.value?.state||'尚未读取')
watch(source,()=>{inspected.value=null;inspectedPath.value='';confirmation.value=null;feedback.value=''},{flush:'sync'})
function count(value:unknown){return typeof value==='number'&&Number.isFinite(value)&&value>=0?value:'未知'}
async function perform(label:string,fn:()=>Promise<void>){if(locked.value)return;busy.value=true;operation.value=label;error.value='';feedback.value='';try{await fn()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false;operation.value=''}}
async function refresh(){await perform('刷新状态',async()=>{await w.boot();if(w.error.value)throw new Error(w.error.value);feedback.value='状态已刷新'})}
async function inspectPath(path:string){inspected.value=null;inspectedPath.value='';const result=await api.inspect(path);inspected.value=result;inspectedPath.value=path;feedback.value=result.available?'来源检查完成，请核对后确认导入':'来源不可用，请查看检查结果'}
async function inspect(){const path=source.value.trim();if(!path)return;await perform('检查来源',()=>inspectPath(path))}
async function browse(){await perform('选择来源',async()=>{const chosen=await selectProfileSourceDirectory();if(!chosen){feedback.value='已取消选择目录，当前来源保持不变';return}source.value=chosen;await inspectPath(chosen)})}
async function exportBackup(){await perform('导出备份',async()=>{feedback.value=await backup()?'备份已导出':'已取消备份导出';if(feedback.value==='备份已导出')w.notify(feedback.value)})}
async function openFolder(){await perform('打开数据目录',async()=>{await request('/api/data/open-folder','POST',undefined,'OpenDataFolder');feedback.value='已请求打开数据目录'})}
async function refreshAfterMutation(){await w.boot();if(w.error.value)error.value=`操作已完成，但部分状态刷新失败：${w.error.value}`}
async function initialize(){await perform('创建空白存储',async()=>{await api.initialize();feedback.value='空白存储已创建';await refreshAfterMutation()})}
function confirm(kind:'import'|'migration'){if(locked.value)return;dialogTrigger=document.activeElement instanceof HTMLElement?document.activeElement:null;error.value='';feedback.value='';confirmation.value=kind}
function cancel(){if(locked.value)return;confirmation.value=null;error.value='';feedback.value='已取消，当前数据保持不变';restoreFocus()}
async function apply(){const kind=confirmation.value;if(!kind||locked.value)return;if(kind==='import'&&(!inspected.value?.available||inspectedPath.value!==source.value.trim()))return;await perform(kind==='import'?'导入订阅':'迁移旧版数据',async()=>{if(kind==='import')await api.importSource(inspectedPath.value);else await api.migrate();confirmation.value=null;feedback.value=kind==='import'?'订阅来源已导入':'旧版历史与设置已迁移';if(kind==='import'){inspected.value=null;inspectedPath.value=''}w.notify(feedback.value);await refreshAfterMutation()});if(!confirmation.value)restoreFocus()}
</script>
<template>
  <section class="page-view settings-view data-view" :aria-busy="locked">
    <div class="page-header"><div><h1>数据与备份</h1><p>存储位置、订阅来源与完整备份。</p></div><button ref="refreshButton" class="button" :disabled="locked" @click="refresh"><Icon name="refresh"/>刷新状态</button></div>
    <p v-if="pageError&&!confirmation" class="inline-error" role="alert">{{pageError}}</p>
    <p class="operation-status" role="status" aria-live="polite">{{operation?`正在${operation}…`:w.loading.value?'正在加载数据状态…':feedback||'操作前请核对来源与目标位置。'}}</p>
    <section class="settings-section"><div class="section-heading"><h2>当前存储</h2><span class="badge" :class="{good:w.setup.value?.state==='ready'&&!pageError&&!w.loading.value}">{{setupLabel}}</span></div>
      <dl class="path-list"><div><dt>数据根目录</dt><dd>{{w.setup.value?.data_root||'尚未读取'}}</dd></div><div><dt>订阅目录</dt><dd>{{w.setup.value?.profile_dir||'尚未读取'}}</dd></div><div><dt>历史目录</dt><dd>{{w.setup.value?.history_dir||'尚未读取'}}</dd></div><div><dt>设置文件</dt><dd>{{w.setup.value?.settings_file||'尚未读取'}}</dd></div></dl>
      <div class="button-group"><button class="button" :disabled="locked||!w.setup.value?.data_root" @click="openFolder">打开数据目录</button><button class="button primary" :disabled="locked||!w.setup.value?.data_root" @click="exportBackup"><Icon name="download"/>导出备份</button></div>
      <p class="section-note">完整备份包含订阅、历史与设置；导出文件格式与保存方式沿用现有功能。下方订阅导入不等于完整备份恢复。</p>
    </section>
    <section v-if="w.setup.value&&w.setup.value.state!=='ready'&&!w.loading.value" class="settings-section"><h2>选择开始方式</h2><p class="section-note">可以检查已有订阅来源，或创建空白存储后添加机场。</p><button class="button primary" :disabled="locked||!!pageError" @click="initialize">创建空白存储</button></section>
    <section class="settings-section"><h2>导入现有订阅</h2><p class="section-note">选择目录 → 检查来源 → 确认导入。仅导入订阅来源，历史与设置迁移在下方单独确认。</p>
      <label class="source-label" for="profile-source">现有订阅数据目录</label><div class="source-controls"><input id="profile-source" v-model="source" :disabled="locked" placeholder="现有数据目录的绝对路径"><button v-if="desktop()" class="button" :disabled="locked" @click="browse"><Icon name="data"/>浏览目录</button><button class="button" :disabled="locked||!source.trim()" @click="inspect">检查来源</button></div>
      <p v-if="!desktop()" class="section-note">Web 模式不支持原生目录选择器，请输入服务器可访问的绝对路径。</p>
      <div v-if="inspected" class="source-inspection"><strong>{{inspected.label||inspected.path||inspectedPath}}</strong><p>{{count(inspected.profile_count)}} 个机场 · {{count(inspected.cache_count)}} 份缓存</p><p v-if="inspected.error" class="inline-error" role="alert">{{inspected.error}}</p><button class="button primary" :disabled="locked||!inspected.available" @click="confirm('import')">导入这个来源</button></div>
      <div v-for="s in w.setup.value?.sources" :key="s.path" class="compact-row"><span>{{s.label}}<small>{{s.path}}</small><small v-if="!s.available">{{s.error||'来源不可用'}}</small></span><button class="button" :disabled="!s.available||locked" @click="source=s.path;inspect()">检查</button></div>
      <p v-if="w.setup.value&&!w.setup.value.sources?.length" class="section-note">未发现候选来源，可手动选择或输入目录。</p>
    </section>
    <section v-if="migration&&migration.state!=='ready'&&migration.state!=='isolated'" class="settings-section"><div class="section-heading"><h2>旧版历史与设置迁移</h2><span class="badge">{{migrationLabel}}</span></div><p v-if="migration.error" class="inline-error" role="alert">{{migration.error}}</p><p class="section-note">迁移前建议导出备份。存在冲突时先解决冲突，再刷新状态。</p><button class="button" :disabled="locked||migration.state==='conflict'" @click="confirm('migration')">迁移旧版历史与设置</button></section>
    <Modal v-if="confirmation" :title="confirmation==='import'?'确认订阅来源导入':'确认旧版数据迁移'" :closable="!locked" @close="cancel">
      <div class="confirmation-content"><p>{{confirmation==='import'?'将使用已检查的订阅来源导入当前存储。请核对目录；此操作不会恢复完整备份中的历史与设置。':'将按现有迁移流程处理旧版历史与设置。请先确认已备份，并核对来源和目标。'}}</p><dl class="path-list"><div><dt>来源</dt><dd>{{confirmation==='import'?inspectedPath:migration?.source_history_dir||'由现有迁移流程检查'}}</dd></div><div><dt>目标</dt><dd>{{confirmation==='import'?w.setup.value?.profile_dir||'尚未读取':migration?.target_history_dir||'尚未读取'}}</dd></div></dl><p v-if="error" class="inline-error" role="alert">{{error}}</p><p v-if="busy" role="status">正在{{operation}}…</p></div>
      <template #footer><button class="button" :disabled="locked" @click="cancel">取消</button><button class="button primary" :disabled="locked" @click="apply">{{confirmation==='import'?'确认导入':'确认迁移'}}</button></template>
    </Modal>
  </section>
</template>
<style scoped>
.data-view{max-width:1080px}.data-view .page-header{gap:12px;flex-wrap:wrap;margin-bottom:10px}.data-view .settings-section{padding:16px 18px;margin-bottom:12px}.section-heading{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:10px}.data-view h2{font-size:15px;margin-bottom:10px}.section-heading h2{margin:0}.operation-status{min-height:22px;margin:0 0 12px;color:var(--ink);font-size:12px}.section-note{font-size:12px;line-height:1.65;color:var(--ink);opacity:.85;margin:10px 0}.path-list{margin:12px 0;display:grid;gap:0}.path-list>div{display:grid;grid-template-columns:100px minmax(0,1fr);gap:12px;padding:7px 0;border-bottom:1px solid var(--line)}.path-list dt{color:var(--ink);font-size:12px}.path-list dd{margin:0;color:var(--ink);font-size:12px;overflow-wrap:anywhere;white-space:normal;min-width:0}.data-view .button-group{flex-wrap:wrap;gap:8px}.source-label{display:block;font-size:12px;margin:10px 0 6px}.source-controls{display:flex;gap:8px;flex-wrap:wrap}.source-controls input{flex:1 1 280px;min-width:0;width:100%}.source-inspection{margin-top:12px;padding:12px;border:1px solid var(--line);border-radius:8px;background:var(--surface-soft);overflow-wrap:anywhere}.source-inspection p{margin:6px 0 10px;font-size:12px}.compact-row>span{flex:1;min-width:0}.compact-row small{overflow-wrap:anywhere;color:var(--ink)}.data-view :is(button,input):focus-visible,.confirmation-content~*:focus-visible{outline:2px solid var(--blue);outline-offset:3px}.confirmation-content{font-size:13px;overflow-wrap:anywhere}.confirmation-content .path-list>div{grid-template-columns:48px minmax(0,1fr)}
@media(max-width:800px){.data-view .settings-section{padding:14px}.path-list>div{grid-template-columns:82px minmax(0,1fr);gap:8px}.source-controls input{flex-basis:100%}.data-view .page-header h1{font-size:23px}}
@media(max-width:440px){.path-list>div{grid-template-columns:1fr;gap:3px}.source-controls .button{flex:1}}
</style>
