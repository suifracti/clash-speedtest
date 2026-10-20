<script setup lang="ts">
import UiSelect from './UiSelect.vue'
import {nextTick,onMounted,ref} from 'vue'
import {api,desktop,request} from '../api'
import {useWorkspace} from '../workspace'
import {type Settings} from '../domain'
import Modal from './Modal.vue'
import Icon from './Icon.vue'
import {applyTheme,savedThemePreference,type ThemePreference} from '../theme'
const w=useWorkspace(),settings=ref<Settings|null>(null),busy=ref(false),loading=ref(true),error=ref(''),feedback=ref(''),operation=ref(''),tokenInput=ref(''),tokenForm=ref(false),theme=ref(savedThemePreference()),warningGiB=ref<number|null>(null),hardGiB=ref<number|null>(null),responseKiB=ref<number|null>(null),preview=ref<Record<string,any>|null>(null),previewDays=ref(0)
let dialogTrigger:HTMLElement|null=null
function rememberFocus(){dialogTrigger=document.activeElement instanceof HTMLElement?document.activeElement:null}
function restoreFocus(){void nextTick(()=>dialogTrigger?.isConnected&&dialogTrigger.focus())}
function openToken(){if(busy.value)return;rememberFocus();error.value='';feedback.value='';tokenForm.value=true}
async function perform(label:string,fn:()=>Promise<void>){if(busy.value)return;busy.value=true;error.value='';feedback.value='';operation.value=label;try{await fn()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false;operation.value=''}}
async function load(){loading.value=true;await perform('加载设置',async()=>{settings.value=await api.settings();settings.value.local_notifications??={enabled:false,failure_percent:50,remaining_percent:10,expiry_days:7};warningGiB.value=settings.value.monitor_storage_warning_bytes==null?null:settings.value.monitor_storage_warning_bytes/1073741824;hardGiB.value=settings.value.monitor_storage_hard_bytes==null?null:settings.value.monitor_storage_hard_bytes/1073741824;responseKiB.value=settings.value.monitor_budget_response_bytes==null?null:settings.value.monitor_budget_response_bytes/1024});loading.value=false}
async function setTheme(value:ThemePreference){await perform('切换外观',async()=>{try{await applyTheme(value);feedback.value='外观已更新，无需另行保存'}finally{theme.value=savedThemePreference()}})}
function validSettings(){
  if(!settings.value)return false
  if(typeof warningGiB.value!=='number'||!Number.isFinite(warningGiB.value)||typeof hardGiB.value!=='number'||!Number.isFinite(hardGiB.value)||warningGiB.value<=0||warningGiB.value>=hardGiB.value){error.value='容量告警阈值须小于保护阈值，且都大于 0';return false}
  const concurrency=settings.value.monitor_budget_max_concurrent
  if(typeof concurrency!=='number'||!Number.isInteger(concurrency)||concurrency<1||concurrency>64||typeof responseKiB.value!=='number'||!Number.isFinite(responseKiB.value)||responseKiB.value<1){error.value='并发请求须为 1–64 的整数，响应读取上限须至少为 1 KiB';return false}
  const n=settings.value.local_notifications;if(n?.enabled&&(![n.failure_percent,n.remaining_percent].every(v=>Number.isFinite(v)&&v>=1&&v<=100)||!Number.isInteger(n.expiry_days)||n.expiry_days<0)){error.value='通知比例须为 1–100，到期提前天数须为非负整数';return false}
  return validRetention()
}
function validRetention(){const days=settings.value?.monitor_retention_custom_days;if(settings.value?.monitor_retention_policy==='custom'&&(!Number.isInteger(days)||days!<1||days!>36500)){error.value='自定义保存天数须为 1–36500 的整数';return false}return true}
async function save(){if(busy.value||!settings.value)return;error.value='';feedback.value='';if(!validSettings())return;const s={...settings.value!,monitor_storage_warning_bytes:Math.round(warningGiB.value!*1073741824),monitor_storage_hard_bytes:Math.round(hardGiB.value!*1073741824),monitor_budget_response_bytes:Math.round(responseKiB.value!*1024)};await perform('保存设置',async()=>{if(s.local_notifications?.enabled)await request('','POST',undefined,'EnableLocalNotifications');await api.saveSettings(s);w.settings.value=s;feedback.value='设置已保存';w.notify(feedback.value)})}
async function setToken(){if(!tokenInput.value.trim())return;await perform('保存凭据',async()=>{await api.setToken(tokenInput.value);tokenInput.value='';tokenForm.value=false;feedback.value='凭据已保存';w.notify(feedback.value);w.token.value=await api.token()});if(!tokenForm.value)restoreFocus()}
async function discoverToken(){await perform('检测本地凭据',async()=>{const res=await api.discoverToken();w.token.value=res;if(res.has_token){feedback.value=`已检测并绑定本地凭据（来自 ${res.source||'本地发现'}）`;w.notify(feedback.value)}else{feedback.value='未在已知路径检测到可用凭据'}})}
async function refreshToken(){await perform('刷新凭据状态',async()=>{w.token.value=await api.token();feedback.value=w.token.value?.has_token?`凭据状态已刷新（已绑定 · ${w.token.value.source}）`:'凭据状态已刷新（尚未绑定）'})}
async function login(){await perform('打开账号登录',async()=>{await api.oauth();feedback.value='已打开账号登录，完成后请刷新状态';w.notify(feedback.value)})}
function closeToken(){if(busy.value)return;tokenForm.value=false;tokenInput.value='';error.value='';feedback.value='已取消凭据输入';restoreFocus()}
function closePreview(){if(busy.value)return;preview.value=null;error.value='';feedback.value='已取消清理，监测记录保持不变';restoreFocus()}
async function retentionPreview(){if(busy.value||!settings.value)return;error.value='';if(!validRetention())return;rememberFocus();const body={policy:settings.value.monitor_retention_policy,custom_days:settings.value.monitor_retention_custom_days};await perform('预览旧采样清理',async()=>{preview.value=await request('/api/monitor/retention/preview','POST',body,'PreviewMonitorRetention',[body]);previewDays.value=body.custom_days})}
async function applyRetention(){if(!preview.value)return;const body={policy:preview.value.policy,custom_days:previewDays.value,cutoff_time:preview.value.cutoff};await perform('清理旧采样',async()=>{const result=await request<Record<string,any>>('/api/monitor/retention','POST',body,'ApplyRetention',[body]);preview.value=null;feedback.value=`已清理 ${result.samples_deleted??'未知数量的'} 条旧采样`;w.notify(feedback.value)});if(!preview.value)restoreFocus()}
onMounted(load)
</script>
<template>
  <section class="page-view settings-view preferences-view" :aria-busy="busy">
    <div class="page-header"><div><h1>偏好设置</h1><p>外观、监测请求与保存策略。</p></div><button class="button primary" :disabled="busy||!settings" @click="save">保存设置</button></div>
    <p v-if="error&&!tokenForm&&!preview" class="inline-error" role="alert">{{error}}</p>
    <p class="operation-status" role="status" aria-live="polite">{{operation?`正在${operation}…`:feedback||'监测与保存策略修改后，请点击保存设置。'}}</p>
    <div v-if="!settings&&!loading" class="load-retry"><button class="button" :disabled="busy" @click="load">重试加载</button></div>
    <fieldset :disabled="busy" class="settings-fields">
      <div class="preference-grid">
        <section class="settings-section"><h2>外观<span class="cst-effect now">立即生效</span></h2><p class="section-note">立即应用到当前界面。</p><div class="theme-choices"><button class="button" :class="{active:theme==='light'}" :aria-pressed="theme==='light'" @click="setTheme('light')"><Icon name="sun"/>浅色</button><button class="button" :class="{active:theme==='dark'}" :aria-pressed="theme==='dark'" @click="setTheme('dark')"><Icon name="moon"/>深色</button><button class="button" :class="{active:theme==='system'}" :aria-pressed="theme==='system'" @click="setTheme('system')">跟随系统</button></div><div class="theme-samples" role="group" aria-label="主题外观预览">
          <button type="button" class="theme-sample theme-sample-light" :class="{active:theme==='light'}" :aria-pressed="theme==='light'" @click="setTheme('light')">
            <span class="theme-sample-window" aria-hidden="true"><span class="theme-sample-toolbar"><i/><i/><i/></span><span class="theme-sample-orbit"/><span class="theme-sample-lines"><i/><i/><i/></span></span>
            <strong>浅色</strong><small>中性灰底，状态色只标异常</small>
          </button>
          <button type="button" class="theme-sample theme-sample-dark" :class="{active:theme==='dark'}" :aria-pressed="theme==='dark'" @click="setTheme('dark')">
            <span class="theme-sample-window" aria-hidden="true"><span class="theme-sample-toolbar"><i/><i/><i/></span><span class="theme-sample-orbit"/><span class="theme-sample-lines"><i/><i/><i/></span></span>
            <strong>深色</strong><small>保留对比与状态层级</small>
          </button>
        </div></section>
        <section v-if="settings" class="settings-section"><h2>打开链接<span class="cst-effect">保存后生效</span></h2><label class="form-stack">首选浏览器<UiSelect v-model="settings.preferred_browser"><option value="auto">自动</option><option value="default">系统默认</option><option value="chrome">Chrome</option><option value="edge">Edge</option></UiSelect></label><p class="section-note">{{desktop()?'由桌面端按可用浏览器打开链接。':'Web 模式由当前浏览器处理链接；此偏好供桌面端使用。'}}</p></section>
      </div>
      <template v-if="settings">
        <div class="settings-control-grid">
        <section class="settings-section"><div class="section-heading"><h2>监测请求<span class="cst-effect">保存后 · 影响下一轮</span></h2><span>配置请求预算</span></div><div class="form-grid"><label>最大并发请求<input v-model.number="settings.monitor_budget_max_concurrent" type="number" min="1" max="64" step="1"></label><label>单次响应读取上限 · KiB<input v-model.number="responseKiB" type="number" min="1" placeholder="尚未读取"></label></div><p class="section-note">保存仅更新请求预算；监测任务的启动与恢复由监测页操作。</p></section>
        <section class="settings-section"><div class="section-heading"><h2>历史容量与保留<span class="cst-effect">保存后生效</span></h2><span>清理前先预览</span></div><div class="form-grid"><label>告警阈值 · GiB<input v-model.number="warningGiB" type="number" min="0.01" step="0.1" placeholder="尚未读取"></label><label>保护阈值 · GiB<input v-model.number="hardGiB" type="number" min="0.01" step="0.1" placeholder="尚未读取"></label><label>监测原始样本保存<UiSelect v-model="settings.monitor_retention_policy"><option value="keep_all">全部保留</option><option value="30d">最近 30 天</option><option value="90d">最近 90 天</option><option value="180d">最近 180 天</option><option value="custom">自定义天数</option></UiSelect></label><label v-if="settings.monitor_retention_policy==='custom'">保存天数<input v-model.number="settings.monitor_retention_custom_days" type="number" min="1" max="36500" step="1"></label></div><div class="retention-actions"><p class="section-note">告警阈值须小于保护阈值。清理旧采样是独立操作，删除后无法从应用恢复，建议先导出备份。</p><button v-if="settings.monitor_retention_policy!=='keep_all'" class="button" @click="retentionPreview">预览旧采样清理</button></div></section>
        </div>
      </template>
      <section v-if="settings?.local_notifications" class="settings-section"><h2>本机通知<span class="cst-effect">保存后生效 · 默认关闭</span></h2><label class="checkbox"><input v-model="settings.local_notifications.enabled" type="checkbox" :disabled="!desktop()">启用桌面通知</label><div class="form-grid"><label>无响应节点达到 · %<input v-model.number="settings.local_notifications.failure_percent" type="number" min="1" max="100"></label><label>套餐剩余低于 · %<input v-model.number="settings.local_notifications.remaining_percent" type="number" min="1" max="100"></label><label>到期提前 · 天<input v-model.number="settings.local_notifications.expiry_days" type="number" min="0"></label></div><p class="section-note">应用运行时检查已保存检测与套餐信息；相同告警只通知一次，恢复后可再次提醒。{{desktop()?'启用时由系统询问通知权限。':'请在桌面 App 中启用。'}}</p></section>
      <section class="settings-section"><div class="section-heading"><h2>账号凭据</h2><span>{{!w.token.value?'状态尚未读取':w.token.value.has_token?`已绑定 · ${w.token.value.source||'来源未确认'}`:'尚未绑定 Antigravity 凭据'}}</span></div><div class="button-group"><button class="button" @click="login"><Icon name="key"/>浏览器登录</button><button class="button" @click="discoverToken"><Icon name="search"/>检测查找凭据</button><button class="button ghost" @click="openToken">手动输入 Token</button><button class="button ghost" @click="refreshToken">刷新状态</button></div></section>
    </fieldset>
    <Modal v-if="tokenForm" title="手动绑定凭据" :closable="!busy" @close="closeToken"><fieldset class="dialog-fields" :disabled="busy"><label class="form-stack">Token<textarea v-model="tokenInput" rows="4" autocomplete="off" spellcheck="false" placeholder="输入 Token 或 Authorization: Bearer …"/></label></fieldset><p v-if="error" class="inline-error" role="alert">{{error}}</p><p v-if="busy" role="status">正在{{operation}}…</p><template #footer><button class="button" :disabled="busy" @click="closeToken">取消</button><button class="button primary" :disabled="busy||!tokenInput.trim()" @click="setToken">保存凭据</button></template></Modal>
    <Modal v-if="preview" title="清理预览" :closable="!busy" @close="closePreview"><div class="retention-preview"><p>将删除 {{preview.samples_to_delete??'未知数量的'}} 条旧样本和 {{preview.runs_to_delete??'未知数量的'}} 条关联运行记录。</p><p class="section-note">仅处理 {{preview.cutoff||'尚未确认时间'}} 之前的监测记录。删除后无法从本应用恢复。</p><p v-if="error" class="inline-error" role="alert">{{error}}</p><p v-if="busy" role="status">正在{{operation}}…</p></div><template #footer><button class="button" :disabled="busy" @click="closePreview">返回</button><button class="button danger-fill" :disabled="busy||!preview.cutoff||!preview.policy" @click="applyRetention">执行清理</button></template></Modal>
  </section>
</template>
<style scoped>
.preferences-view{max-width:1080px}.preferences-view .page-header{gap:12px;flex-wrap:wrap;margin-bottom:10px}.settings-fields,.dialog-fields{border:0;padding:0;margin:0;min-width:0}.preference-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.preferences-view .settings-section{padding:16px 18px;margin-bottom:12px;min-width:0}.preferences-view h2{font-size:15px;margin-bottom:10px}.section-heading{display:flex;align-items:center;justify-content:space-between;gap:8px;flex-wrap:wrap;margin-bottom:12px}.section-heading h2{margin:0}.section-heading>span{font-size:12px;color:var(--ink)}.operation-status{min-height:22px;margin:0 0 12px;font-size:12px;color:var(--ink)}.section-note{font-size:12px;line-height:1.65;color:var(--ink);opacity:.85;margin:8px 0}.preferences-view .form-grid{gap:12px}.preferences-view .form-grid label,.preferences-view label.form-stack{color:var(--ink);min-width:0}.preferences-view input{width:100%;min-width:0}.preferences-view .theme-choices,.preferences-view .button-group{display:flex;gap:8px;flex-wrap:wrap}.retention-actions{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-top:8px}.retention-actions p{flex:1 1 340px;margin:0}.retention-preview{overflow-wrap:anywhere}.load-retry{margin:0 0 12px}.preferences-view :is(button,input,textarea):focus-visible{outline:2px solid var(--blue);outline-offset:3px}
@media(max-width:800px){.preferences-view .settings-section{padding:14px}.preference-grid{grid-template-columns:1fr}.preferences-view .page-header h1{font-size:23px}.preferences-view .form-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:560px){.preferences-view .form-grid{grid-template-columns:1fr}.section-heading{align-items:flex-start;flex-direction:column}}
</style>
<style src="./secondary-observatory.css" scoped></style>
