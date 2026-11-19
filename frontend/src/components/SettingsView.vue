<script setup lang="ts">
import UnlockRegionEditor from './UnlockRegionEditor.vue'
import {unlockGroups,applicableRegions} from '../unlock'
import UiSelect from './UiSelect.vue'
import {computed,nextTick,onBeforeUnmount,onMounted,ref,watch} from 'vue'
import {api,desktop,request} from '../api'
import {useWorkspace} from '../workspace'
import {bytes,type Settings} from '../domain'
import {checkedRetentionPreview,retentionCount,retentionFailure,retentionSummary,validRetentionCount,type MonitorStorageUsage,type RetentionPreview,type RetentionRequest,type RetentionResult} from '../monitorRetention'
import Modal from './Modal.vue'
import Icon from './Icon.vue'
import {applyTheme,savedThemePreference,type ThemePreference} from '../theme'
const w=useWorkspace(),settings=ref<Settings|null>(null),busy=ref(false),loading=ref(true),error=ref(''),feedback=ref(''),operation=ref(''),tokenInput=ref(''),tokenForm=ref(false),theme=ref(savedThemePreference()),warningGiB=ref<number|null>(null),hardGiB=ref<number|null>(null),responseKiB=ref<number|null>(null),preview=ref<RetentionPreview|null>(null)
const settingSection=w.settingsSection
const storage=ref<MonitorStorageUsage|null>(null),storageLoading=ref(false),storageError=ref(''),storageReadAt=ref('')
const previewRequest=ref<RetentionRequest|null>(null),previewNeedsRefresh=ref(false),previewAttempted=ref(false)
const previewEmpty=computed(()=>!!preview.value&&preview.value.samples_to_delete===0&&preview.value.runs_to_delete===0)
let storageGeneration=0,alive=true
function storageBytes(value:unknown){return typeof value==='number'&&Number.isFinite(value)&&value>=0?bytes(value):'未知'}
function acceptStorage(value:MonitorStorageUsage|null){storage.value=value;storageError.value='';storageReadAt.value=new Date().toLocaleTimeString()}
async function refreshStorage(){const generation=++storageGeneration;storageLoading.value=true;try{const value=await api.storage();if(alive&&generation===storageGeneration)acceptStorage(value||null)}catch(cause){if(alive&&generation===storageGeneration)storageError.value=cause instanceof Error?cause.message:String(cause)}finally{if(alive&&generation===storageGeneration)storageLoading.value=false}}
watch(settingSection,value=>{if(value==='storage')void refreshStorage()},{immediate:true})
onBeforeUnmount(()=>{alive=false;storageGeneration++})
const settingSections=[['appearance','外观与链接'],['connection','Clash 连接'],['services','解锁地区规则'],['monitor','监测请求预算'],['storage','历史容量与保留'],['notifications','本机通知'],['credentials','账号凭据']]
const savedSettings=ref('')
const settingsStamp=()=>JSON.stringify([settings.value,warningGiB.value,hardGiB.value,responseKiB.value])
const settingsDirty=computed(()=>!!savedSettings.value&&savedSettings.value!==settingsStamp())
const notificationDenied=ref(false)
async function changeNotifications(){
 notificationDenied.value=false
 if(!settings.value?.local_notifications?.enabled)return
 await perform('请求通知授权',async()=>{try{await request('','POST',undefined,'EnableLocalNotifications')}catch{settings.value!.local_notifications!.enabled=false;notificationDenied.value=true}})
}
async function openNotificationSettings(){await perform('打开系统通知设置',async()=>{await request('','POST',undefined,'OpenSystemNotificationSettings')})}
let dialogTrigger:HTMLElement|null=null
function rememberFocus(){dialogTrigger=document.activeElement instanceof HTMLElement?document.activeElement:null}
function restoreFocus(){void nextTick(()=>dialogTrigger?.isConnected&&dialogTrigger.focus())}
function openToken(){if(busy.value)return;rememberFocus();error.value='';feedback.value='';tokenForm.value=true}
async function perform(label:string,fn:()=>Promise<void>){if(busy.value)return;busy.value=true;error.value='';feedback.value='';operation.value=label;try{await fn()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false;operation.value=''}}
async function load(){loading.value=true;await perform('加载设置',async()=>{settings.value=await api.settings();settings.value.unlock_service_regions=Object.fromEntries(unlockGroups.flatMap(g=>g.ids.map(id=>[id,applicableRegions(id,settings.value?.unlock_service_regions)])));settings.value.skip_by_applicable_regions??=true;settings.value.local_notifications??={enabled:false,failure_percent:50,remaining_percent:10,expiry_days:7};settings.value.clash_connection??={mode:'auto',endpoint:'',secret:''};warningGiB.value=settings.value.monitor_storage_warning_bytes==null?null:settings.value.monitor_storage_warning_bytes/1073741824;hardGiB.value=settings.value.monitor_storage_hard_bytes==null?null:settings.value.monitor_storage_hard_bytes/1073741824;responseKiB.value=settings.value.monitor_budget_response_bytes==null?null:settings.value.monitor_budget_response_bytes/1024;savedSettings.value=settingsStamp()});loading.value=false}
async function setTheme(value:ThemePreference){await perform('切换外观',async()=>{try{await applyTheme(value);feedback.value='外观已更新，无需另行保存'}finally{theme.value=savedThemePreference()}})}
function validSettings(){
  if(!settings.value)return false
  if(typeof warningGiB.value!=='number'||!Number.isFinite(warningGiB.value)||typeof hardGiB.value!=='number'||!Number.isFinite(hardGiB.value)||warningGiB.value<=0||warningGiB.value>=hardGiB.value||hardGiB.value>1048576){settingSection.value='storage';error.value='容量告警阈值须小于保护阈值，且都大于 0；保护阈值最多为 1048576 GiB';return false}
  const concurrency=settings.value.monitor_budget_max_concurrent
  if(typeof concurrency!=='number'||!Number.isInteger(concurrency)||concurrency<1||concurrency>64||typeof responseKiB.value!=='number'||!Number.isFinite(responseKiB.value)||responseKiB.value<1){settingSection.value='monitor';error.value='并发请求须为 1–64 的整数，响应读取上限须至少为 1 KiB';return false}
  const n=settings.value.local_notifications;if(n?.enabled&&(![n.failure_percent,n.remaining_percent].every(v=>Number.isFinite(v)&&v>=1&&v<=100)||!Number.isInteger(n.expiry_days)||n.expiry_days<0)){settingSection.value='notifications';error.value='通知比例须为 1–100，到期提前天数须为非负整数';return false}
  return validRetention()
}
function validRetention(){const days=settings.value?.monitor_retention_custom_days;if(settings.value?.monitor_retention_policy==='custom'&&(!Number.isInteger(days)||days!<1||days!>36500)){settingSection.value='storage';error.value='自定义保存天数须为 1–36500 的整数';return false}return true}
async function save(){if(busy.value||!settings.value)return;error.value='';feedback.value='';if(!validSettings())return;const s={...settings.value!,monitor_storage_warning_bytes:Math.round(warningGiB.value!*1073741824),monitor_storage_hard_bytes:Math.round(hardGiB.value!*1073741824),monitor_budget_response_bytes:Math.round(responseKiB.value!*1024)};await perform('保存设置',async()=>{await api.saveSettings(s);w.settings.value=s;savedSettings.value=settingsStamp();feedback.value='设置已保存';w.notify(feedback.value);if(settingSection.value==='storage')await refreshStorage()})}
async function setToken(){if(!tokenInput.value.trim())return;await perform('保存凭据',async()=>{await api.setToken(tokenInput.value);tokenInput.value='';tokenForm.value=false;feedback.value='凭据已保存';w.notify(feedback.value);w.token.value=await api.token()});if(!tokenForm.value)restoreFocus()}
async function discoverToken(){await perform('检测本地凭据',async()=>{const res=await api.discoverToken();w.token.value=res;if(res.has_token){feedback.value=`已检测并绑定本地凭据（来自 ${res.source||'本地发现'}）`;w.notify(feedback.value)}else{feedback.value='未在已知路径检测到可用凭据'}})}
async function refreshToken(){await perform('刷新凭据状态',async()=>{w.token.value=await api.token();feedback.value=w.token.value?.has_token?`凭据状态已刷新（已绑定 · ${w.token.value.source}）`:'凭据状态已刷新（尚未绑定）'})}
async function login(){await perform('打开账号登录',async()=>{await api.oauth();feedback.value='已打开账号登录，完成后请刷新状态';w.notify(feedback.value)})}
function closeToken(){if(busy.value)return;tokenForm.value=false;tokenInput.value='';error.value='';feedback.value='已取消凭据输入';restoreFocus()}
function closePreview(){if(busy.value)return;preview.value=null;error.value='';feedback.value=previewAttempted.value?'已关闭清理窗口；已有处理结果不会撤销。':'已取消清理，监测记录保持不变';restoreFocus()}
async function readRetentionPreview(body:RetentionRequest){
  await perform('预览旧采样清理',async()=>{
    const value=checkedRetentionPreview(await request('/api/monitor/retention/preview','POST',body,'PreviewMonitorRetention',[body]),body)
    if(!alive)return
    preview.value=value;previewRequest.value={...body,cutoff_time:value.cutoff};previewNeedsRefresh.value=false
    if(value.storage){storageGeneration++;acceptStorage(value.storage);storageLoading.value=false}
  })
}
async function retentionPreview(){
  if(busy.value||!settings.value)return
  error.value='';if(!validRetention()||settings.value.monitor_retention_policy==='keep_all')return
  rememberFocus();previewAttempted.value=false;previewNeedsRefresh.value=false
  await readRetentionPreview({policy:settings.value.monitor_retention_policy,custom_days:settings.value.monitor_retention_custom_days})
}
async function recheckRetention(){if(!busy.value&&previewRequest.value)await readRetentionPreview({...previewRequest.value})}
async function applyRetention(){
  if(busy.value||!preview.value||!previewRequest.value||previewNeedsRefresh.value||previewEmpty.value)return
  const body={...previewRequest.value};previewAttempted.value=true
  await perform('清理旧采样',async()=>{
    try{
      const result=await request<RetentionResult>('/api/monitor/retention','POST',body,'ApplyRetention',[body])
      if(result?.partial)throw Object.assign(new Error(result.error_message||'后端返回部分清理结果'),{details:result})
      if(!result||!validRetentionCount(result.samples_deleted)||!validRetentionCount(result.runs_deleted))throw new Error('后端未提供完整处理数量')
      if(!alive)return
      preview.value=null;feedback.value=`已清理 ${retentionSummary(result)}。删除不保证数据库文件立即缩小。`;w.notify(`已清理 ${retentionSummary(result)}`)
    }catch(cause){if(alive){previewNeedsRefresh.value=true;error.value=retentionFailure(cause)}}
    finally{if(alive)await refreshStorage()}
  })
  if(!preview.value)restoreFocus()
}
onMounted(load)
</script>
<template>
  <section class="page-view settings-view preferences-view" :aria-busy="busy">
    <div class="page-header"><div><h1>偏好设置</h1><p>外观、监测请求与保存策略。</p></div><button class="button primary" :disabled="busy||!settings" @click="save">保存设置</button></div>
    <p v-if="error&&!tokenForm&&!preview" class="inline-error" role="alert">{{error}}</p>
    <p class="operation-status" role="status" aria-live="polite">{{operation?`正在${operation}…`:feedback||'监测与保存策略修改后，请点击保存设置。'}}</p>
    <div v-if="!settings&&!loading" class="load-retry"><button class="button" :disabled="busy" @click="load">重试加载</button></div>
    <div class="preferences-layout">
    <nav class="preferences-index" aria-label="设置分类"><button v-for="[id,label] in settingSections" :key="id" :aria-pressed="settingSection===id" @click="settingSection=id">{{label}}</button><p v-if="settingsDirty" class="preferences-unsaved" role="status">有未保存的设置</p></nav>
    <fieldset :disabled="busy" class="settings-fields">
      <div class="preference-grid">
        <section v-show="settingSection==='appearance'" class="settings-section"><h2>外观<span class="cst-effect now">立即生效</span></h2><p class="section-note">立即应用到当前界面。</p><div class="theme-choices"><button class="button" :class="{active:theme==='light'}" :aria-pressed="theme==='light'" @click="setTheme('light')"><Icon name="sun"/>浅色</button><button class="button" :class="{active:theme==='dark'}" :aria-pressed="theme==='dark'" @click="setTheme('dark')"><Icon name="moon"/>深色</button><button class="button" :class="{active:theme==='system'}" :aria-pressed="theme==='system'" @click="setTheme('system')">跟随系统</button></div><div class="theme-samples" role="group" aria-label="主题外观预览">
          <button type="button" class="theme-sample theme-sample-light" :class="{active:theme==='light'}" :aria-pressed="theme==='light'" @click="setTheme('light')">
            <span class="theme-sample-window" aria-hidden="true"><span class="theme-sample-toolbar"><i/><i/><i/></span><span class="theme-sample-orbit"/><span class="theme-sample-lines"><i/><i/><i/></span></span>
            <strong>浅色</strong><small>中性灰底，状态色只标异常</small>
          </button>
          <button type="button" class="theme-sample theme-sample-dark" :class="{active:theme==='dark'}" :aria-pressed="theme==='dark'" @click="setTheme('dark')">
            <span class="theme-sample-window" aria-hidden="true"><span class="theme-sample-toolbar"><i/><i/><i/></span><span class="theme-sample-orbit"/><span class="theme-sample-lines"><i/><i/><i/></span></span>
            <strong>深色</strong><small>保留对比与状态层级</small>
          </button>
        </div></section>
        <section v-if="settings" v-show="settingSection==='appearance'" class="settings-section"><h2>打开链接<span class="cst-effect">保存后生效</span></h2><label class="form-stack">首选浏览器<UiSelect v-model="settings.preferred_browser"><option value="auto">自动</option><option value="default">系统默认</option><option value="chrome">Chrome</option><option value="edge">Edge</option></UiSelect></label><p class="section-note">{{desktop()?'由桌面端按可用浏览器打开链接。':'Web 模式由当前浏览器处理链接；此偏好供桌面端使用。'}}</p></section>
      </div>
      <template v-if="settings">
        <UnlockRegionEditor v-show="settingSection==='services'" v-model="settings.unlock_service_regions!" v-model:skip="settings.skip_by_applicable_regions" :catalog="w.catalog.value"/>
        <section v-show="settingSection==='connection'" class="settings-section clash-connection-settings"><div class="section-heading"><h2>Clash 内核连接<span class="cst-effect">保存后生效</span></h2></div><label>连接方式<UiSelect v-model="settings.clash_connection!.mode"><option value="auto">自动连接</option><option value="tcp">TCP 地址</option></UiSelect></label><p v-if="settings.clash_connection?.mode==='auto'" class="section-note">自动使用默认 socket（/var/run/clash-verge-service/users/&lt;uid&gt;/verge-mihomo.sock）或本机 Clash Verge 控制地址。</p><div v-else class="form-grid"><label>TCP 地址<input v-model="settings.clash_connection!.endpoint" placeholder="127.0.0.1:9090" autocomplete="off"></label><label>Secret 密码<input v-model="settings.clash_connection!.secret" type="password" autocomplete="new-password"></label></div></section>
        <div class="settings-control-grid">
        <section v-show="settingSection==='monitor'" class="settings-section"><div class="section-heading"><h2>监测请求<span class="cst-effect">保存后 · 影响下一轮</span></h2><span>配置请求预算</span></div><div class="form-grid"><label>最大并发请求<input v-model.number="settings.monitor_budget_max_concurrent" type="number" min="1" max="64" step="1"></label><label>单次响应读取上限 · KiB<input v-model.number="responseKiB" type="number" min="1" placeholder="尚未读取"></label></div><p class="section-note">保存仅更新请求预算；监测任务的启动与恢复由监测页操作。</p></section>
        <section v-show="settingSection==='storage'" class="settings-section storage-settings">
          <div class="section-heading"><h2>历史容量与保留</h2><button class="button ghost" :disabled="storageLoading||busy" @click="refreshStorage">{{storageLoading?'读取容量中…':'刷新容量'}}</button></div>
          <div class="storage-ledger" :aria-busy="storageLoading">
            <div class="storage-total"><span>历史文件占用</span><strong>{{storage?storageBytes(storage.total_bytes):'尚无可用容量读数'}}</strong><span v-if="storage" :class="{'storage-protected':storage.protected,'storage-warning':storage.warning&&!storage.protected}">{{storage.protected===true?'已触发容量保护':storage.warning===true?'已达到告警阈值':storage.protected===false&&storage.warning===false?'未达告警阈值':'保护状态未提供'}}</span></div>
            <p v-if="storageError" class="inline-error" role="alert">容量读取失败{{storageReadAt?`；显示上次成功读取于 ${storageReadAt}`:'；尚无可用容量读数'}}：{{storageError}}</p>
            <p v-else-if="storageReadAt" class="storage-read-at">最近成功读取于 {{storageReadAt}}</p>
            <dl v-if="storage" class="storage-files"><div><dt>主数据库 · SQLite</dt><dd>{{storageBytes(storage.database_bytes)}}</dd></div><div><dt>写入日志 · WAL</dt><dd>{{storageBytes(storage.wal_bytes)}}</dd></div><div><dt>共享内存</dt><dd>{{storageBytes(storage.shared_memory_bytes)}}</dd></div></dl>
            <p v-if="storage" class="section-note">当前已保存阈值：告警 {{storageBytes(storage.warning_bytes)}} · 保护 {{storageBytes(storage.hard_bytes)}}。</p>
            <p class="section-note">这里是 SQLite 文件占用，不是磁盘剩余空间。清理通常释放库内可复用空间，数据库文件不一定立即缩小。</p>
            <p v-if="storage?.protected" class="storage-protection-note">{{storageError?'上次读取时容量保护生效；当前状态需刷新确认。':'容量保护已生效，新的监测后台轮次受限。'}}达到阈值不会自动删除历史，也不把未执行采样记为节点失败。</p>
          </div>
          <div class="storage-preferences-heading"><h3>增长保护与保留偏好</h3><span class="cst-effect">保存后生效</span></div>
          <div class="form-grid"><label>告警阈值 · GiB<input v-model.number="warningGiB" type="number" min="0.01" max="1048576" step="0.1" placeholder="尚未读取"></label><label>保护阈值 · GiB<input v-model.number="hardGiB" type="number" min="0.01" max="1048576" step="0.1" placeholder="尚未读取"></label><label>监测原始样本保存<UiSelect v-model="settings.monitor_retention_policy"><option value="keep_all">全部保留</option><option value="30d">最近 30 天</option><option value="90d">最近 90 天</option><option value="180d">最近 180 天</option><option value="custom">自定义天数</option></UiSelect></label><label v-if="settings.monitor_retention_policy==='custom'">保存天数<input v-model.number="settings.monitor_retention_custom_days" type="number" min="1" max="36500" step="1"></label></div>
          <p class="section-note">告警阈值须小于保护阈值。提高阈值只允许文件继续增长，不会释放磁盘空间。保存设置不会删除历史，也不会立即启动监测。</p>
          <div class="retention-actions"><div><h3>按需清理旧监测记录</h3><p class="section-note">预览采用当前选择，不会保存偏好。核对范围后单独确认清理；删除后无法从应用恢复，建议先导出备份。</p></div><button v-if="settings.monitor_retention_policy!=='keep_all'" class="button" @click="retentionPreview">预览旧采样清理</button><p v-else class="section-note">当前选择全部保留。</p></div>
        </section>
        </div>
      </template>
      <div class="preference-grid">
      <section v-if="settings?.local_notifications" v-show="settingSection==='notifications'" class="settings-section"><div class="section-heading"><h2>本机通知<span class="cst-effect">保存后生效 · 默认关闭</span></h2><label class="notification-toggle"><input v-model="settings.local_notifications.enabled" type="checkbox" :disabled="!desktop()" @change="changeNotifications">启用桌面通知</label></div><div v-if="notificationDenied" class="notification-denied" role="alert"><span>系统未允许 CST 发送通知</span><button class="button" @click="openNotificationSettings">打开系统通知设置</button></div><div class="form-grid"><label>无响应节点达到 · %<input v-model.number="settings.local_notifications.failure_percent" type="number" min="1" max="100"></label><label>套餐剩余低于 · %<input v-model.number="settings.local_notifications.remaining_percent" type="number" min="1" max="100"></label><label>到期提前 · 天<input v-model.number="settings.local_notifications.expiry_days" type="number" min="0"></label></div><p class="section-note">应用运行时检查已保存检测与套餐信息；相同告警只通知一次，恢复后可再次提醒。{{desktop()?'启用时由系统询问通知权限。':'请在桌面 App 中启用。'}}</p></section>
      <section v-show="settingSection==='credentials'" class="settings-section"><div class="section-heading"><h2>账号凭据</h2><span class="badge" :class="w.token.value?.has_token?'good':''">{{!w.token.value?'状态尚未读取':w.token.value.has_token?`已绑定 · ${w.token.value.source||'来源未确认'}`:'尚未绑定 Antigravity 凭据'}}</span></div><p class="section-note">用于 Antigravity 真实模型可用性探测。探测前可先绑定或从本机环境自动发现凭据。</p><div class="button-group"><button class="button" @click="login"><Icon name="key"/>浏览器登录</button><button class="button" @click="discoverToken"><Icon name="search"/>检测查找凭据</button><button class="button ghost" @click="openToken">手动输入 Token</button><button class="button ghost" @click="refreshToken">刷新状态</button></div></section>
      </div>
    </fieldset>
    </div>
    <Modal v-if="tokenForm" title="手动绑定凭据" :closable="!busy" @close="closeToken"><fieldset class="dialog-fields" :disabled="busy"><label class="form-stack">Token<textarea v-model="tokenInput" rows="4" autocomplete="off" spellcheck="false" placeholder="输入 Token 或 Authorization: Bearer …"/></label></fieldset><p v-if="error" class="inline-error" role="alert">{{error}}</p><p v-if="busy" role="status">正在{{operation}}…</p><template #footer><button class="button" :disabled="busy" @click="closeToken">取消</button><button class="button primary" :disabled="busy||!tokenInput.trim()" @click="setToken">保存凭据</button></template></Modal>
    <Modal v-if="preview" title="清理预览" :closable="!busy" @close="closePreview">
      <div class="retention-preview">
        <p v-if="previewNeedsRefresh" class="inline-warning">以下是执行前的预览，须重新核对剩余数量后再确认。</p>
        <p class="retention-cutoff">仅清理此时间之前的监测记录<time :datetime="preview.cutoff">{{new Date(preview.cutoff).toLocaleString('zh-CN',{hour12:false})}}</time></p>
        <dl class="retention-counts" :class="{stale:previewNeedsRefresh}"><div><dt>监测采样</dt><dd><strong>{{retentionCount(preview.samples_to_delete)}}</strong><span>条</span></dd></div><div><dt>符合条件的已结束轮次</dt><dd><strong>{{retentionCount(preview.runs_to_delete)}}</strong><span>条</span></dd></div></dl>
        <p v-if="previewEmpty&&!previewNeedsRefresh" class="section-note">没有需要清理的监测记录。</p>
        <p class="section-note">数量为预览时读数，执行时可能变化；时间边界保持不变。手动检测记录、任务定义与旧版记录保留，不改变正在运行的任务。</p>
        <p class="section-note">删除后不能从应用恢复，也不保证数据库文件立即缩小。</p>
        <p v-if="error" class="inline-error" role="alert">{{error}}</p><p v-if="busy" role="status">正在{{operation}}…</p>
      </div>
      <template #footer><button class="button" :disabled="busy" @click="closePreview">返回</button><button v-if="previewNeedsRefresh" class="button primary" :disabled="busy" @click="recheckRetention">重新预览剩余记录</button><button v-else class="button danger-fill" :disabled="busy||previewEmpty" @click="applyRetention">执行清理</button></template>
    </Modal>
  </section>
</template>
<style scoped>
.storage-ledger{border-block:1px solid var(--cst-line);padding:18px 0;margin:12px 0 24px}.storage-total{display:flex;align-items:baseline;gap:10px 18px;flex-wrap:wrap}.storage-total>span:first-child{font-size:12px;color:var(--cst-muted)}.storage-total strong{font:600 23px/1.4 var(--cst-mono)}.storage-total>span:last-child{font-size:12px}.storage-protected{color:var(--cst-bad)}.storage-warning{color:var(--cst-warn)}.storage-read-at{font-size:11px;color:var(--cst-muted);margin:5px 0 16px}.storage-files{display:flex;gap:12px 28px;flex-wrap:wrap;margin:16px 0}.storage-files>div{display:flex;align-items:baseline;gap:9px}.storage-files dt{font-size:11px;color:var(--cst-muted)}.storage-files dd{font:12px var(--cst-mono);margin:0}.storage-preferences-heading{display:flex;align-items:center;gap:12px;margin-bottom:16px}.storage-preferences-heading h3,.storage-settings .retention-actions h3{font-size:14px;font-weight:600;margin:0}.storage-protection-note{font-size:12px;color:var(--cst-warn);line-height:1.7}.storage-settings .retention-actions{margin-top:24px;padding-top:20px;border-top:1px solid var(--cst-line);align-items:flex-start}.retention-cutoff{font-size:13px;color:var(--cst-muted);line-height:1.8}.retention-cutoff time{display:block;color:var(--cst-text);font-size:17px;font-weight:600}.retention-counts{display:grid;grid-template-columns:1fr 1fr;gap:18px;border-block:1px solid var(--cst-line);padding:18px 0;margin:18px 0}.retention-counts dt{font-size:12px;color:var(--cst-muted)}.retention-counts dd{display:flex;align-items:baseline;gap:7px;margin:6px 0 0;font-size:13px}.retention-counts dd strong{font:600 23px var(--cst-mono)}.retention-counts.stale{opacity:.65}.retention-preview .inline-error{overflow-wrap:anywhere}.retention-preview .section-note{line-height:1.75}@media(max-width:680px){.storage-files{display:grid;gap:10px}.storage-files>div{justify-content:space-between}.storage-total strong{font-size:20px}.storage-settings .retention-actions{flex-wrap:wrap}.retention-counts{gap:12px}.retention-counts dd strong{font-size:20px}}
.preferences-view{max-width:1200px}.preferences-layout{display:grid;grid-template-columns:192px minmax(0,1fr);gap:24px;align-items:start}.preferences-index{position:sticky;top:calc(var(--cst-nav-height,58px) + 20px);display:grid;gap:4px;padding:8px;border:1px solid var(--cst-line);border-radius:12px;background:var(--cst-panel)}.preferences-index button{border:1px solid transparent;border-radius:9px;text-align:left;padding:9px 12px;background:none;color:var(--cst-muted);font-size:13px;font-weight:550;transition:background .14s,color .14s,border-color .14s}.preferences-index button:hover{background:var(--cst-hover);color:var(--cst-text)}.preferences-index button[aria-pressed=true]{background:var(--cst-accent-soft);border-color:color-mix(in srgb,var(--cst-accent) 22%,transparent);color:var(--cst-accent);font-weight:650}.preferences-unsaved{margin:10px 6px 2px;color:var(--cst-warn);font-size:11.5px;font-weight:600}.preferences-view .page-header{gap:12px;flex-wrap:wrap;margin-bottom:14px}.settings-fields,.dialog-fields{border:0;padding:0;margin:0;min-width:0}.preferences-view .settings-fields .preference-grid,.preferences-view .settings-fields .settings-control-grid{display:contents}.preferences-view .settings-section{padding:22px;margin-bottom:16px;min-width:0;border:1px solid var(--cst-line-strong);border-radius:14px;background:var(--cst-panel);box-shadow:var(--cst-shadow-card)}.preferences-view h2{font-size:15px;margin-bottom:10px}.section-heading{display:flex;align-items:center;justify-content:space-between;gap:8px;flex-wrap:wrap;margin-bottom:12px}.section-heading h2{display:flex;align-items:center;gap:8px;margin:0}.section-heading>span{font-size:12px;color:var(--ink)}.cst-effect{display:inline-flex;align-items:center;padding:2px 8px;border-radius:999px;background:var(--cst-panel-2);border:1px solid var(--cst-line);color:var(--cst-muted);font-size:10.5px;font-weight:600}.operation-status{min-height:22px;margin:0 0 12px;font-size:12px;color:var(--cst-muted)}.section-note{font-size:12px;line-height:1.65;color:var(--ink);opacity:.85;margin:8px 0}.preferences-view .form-grid{gap:12px}.preferences-view .form-grid label,.preferences-view label.form-stack{color:var(--ink);min-width:0}.preferences-view input{width:100%;min-width:0}.preferences-view .theme-choices,.preferences-view .button-group{display:flex;gap:8px;flex-wrap:wrap}.preferences-view .theme-choices .button{border-radius:9px}.preferences-view .theme-sample{border-radius:12px}.retention-actions{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-top:8px}.retention-actions p{flex:1 1 340px;margin:0}.retention-preview{overflow-wrap:anywhere}.load-retry{margin:0 0 12px}.preferences-view :is(button,input,textarea):focus-visible{outline:2px solid var(--blue);outline-offset:3px}
@media(max-width:800px){.preferences-layout{grid-template-columns:minmax(0,1fr);gap:16px}.preferences-index{position:static;display:flex;overflow:auto}.preferences-index button{white-space:nowrap;flex:none}.preferences-unsaved{display:none}.preferences-view .settings-section{padding:16px}.preference-grid{grid-template-columns:1fr}.preferences-view .page-header h1{font-size:23px}.preferences-view .form-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:560px){.preferences-view .form-grid{grid-template-columns:1fr}.section-heading{align-items:flex-start;flex-direction:column}}
</style>
<style src="./secondary-observatory.css" scoped></style>

<style scoped>.preferences-view .notification-toggle{display:inline-flex;align-items:center;gap:8px}.preferences-view .notification-toggle input[type=checkbox]{width:16px;height:16px;min-height:0;padding:0;margin:0;flex:none}.notification-denied{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin:12px 0;color:var(--cst-muted)}</style>
<style scoped>.clash-connection-settings>label{display:block;max-width:340px}</style>
