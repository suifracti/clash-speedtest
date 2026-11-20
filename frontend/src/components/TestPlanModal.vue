<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {nodeDisplayName} from '../nodePresentation'
import {bytes,isReputationRule,projectNames,targets,type Project} from '../domain'
import {useWorkspace} from '../workspace'
import {api} from '../api'
import UiSelect from './UiSelect.vue'
import Modal from './Modal.vue'
import ServicePicker from './ServicePicker.vue'
import UnlockPicker from './UnlockPicker.vue'
import {unlockServiceSkipReason} from '../unlock'

const w=useWorkspace(),validation=ref(''),discoveringToken=ref(false),tokenNotice=ref('')
async function refreshLocalToken(){
  if(discoveringToken.value)return
  discoveringToken.value=true;tokenNotice.value='';validation.value=''
  try{
    const status=await api.discoverToken()
    w.token.value=status
    tokenNotice.value=status.has_token?`已绑定本地凭据（${status.source||'本地自动检测'}）`:'未在已知路径检测到有效凭据'
  }catch(e){
    tokenNotice.value=e instanceof Error?e.message:String(e)
  }finally{
    discoveringToken.value=false
  }
}
const isReputationId=(id:string)=>isReputationRule(w.catalog.value.find(rule=>rule.service_id===id)||{service_id:id})
const standardCatalog=computed(()=>w.catalog.value.filter(rule=>!isReputationRule(rule)))
const reputationCatalog=computed(()=>w.catalog.value.filter(rule=>isReputationRule(rule)&&!rule.service_id.startsWith('ipok_')))
const standardServiceIds=computed({
  get:()=>w.plan.value?.serviceIds.filter(id=>!isReputationId(id))||[],
  set:(ids:string[])=>{if(w.plan.value)w.plan.value.serviceIds=[...new Set(ids.filter(id=>!isReputationId(id)))]}
})
const reputationServiceIds=computed({
  get:()=>w.plan.value?.reputationServiceIds?.filter(isReputationId)||[],
  set:(ids:string[])=>{if(w.plan.value)w.plan.value.reputationServiceIds=[...new Set(ids.filter(isReputationId))]}
})
const adaptiveDownload=computed(()=>w.plan.value?.downloadMode==='adaptive')
const preferredDownloadSource=computed(()=>w.plan.value?.downloadSource||(adaptiveDownload.value?'ovh':'cloudflare'))
const downloadVolume=computed(()=>w.plan.value?.projects.includes('download')&&!adaptiveDownload.value?w.plan.value.nodes.length*w.plan.value.downloadMiB*1048576*w.plan.value.repeats:0)
const selectedServiceIds=computed(()=>{
  const p=w.plan.value
  if(!p)return []
  return [...new Set([...(p.projects.includes('service')?p.serviceIds.filter(id=>!isReputationId(id)):[]),...(p.includeReputation?(p.reputationServiceIds||[]).filter(isReputationId):[])])]
})
const plannedRequests=computed(()=>{const p=w.plan.value;if(!p)return 0;return p.nodes.length*p.repeats*((p.projects.includes('latency')?(p.target==='all'?targets.length:1)*p.samples:0)+(p.projects.includes('download')?1:0))+p.nodes.reduce((sum,n)=>sum+w.allowedUnlockServices(n,selectedServiceIds.value).length,0)*p.repeats*(p.serviceSamples??4)})
const plannedKinds=computed(()=>{const p=w.plan.value;return [...(p?.projects.includes('latency')?['延迟采样']:[]),...(selectedServiceIds.value.length?['服务观察']:[]),...(p?.projects.includes('download')?['每节点 1 次下载']:[])].join(' + ')})
function sourceGroupName(n:typeof w.plan.value extends infer P?P extends {nodes:(infer N)[]}?N:never:never){
  const airport=w.nodeAirportName(n),sub=w.nodeSubscriptionName(n)
  return sub&&sub!==airport?`${airport} · ${sub}`:airport
}
const groups=computed(()=>[...new Map(w.plan.value?.nodes.map(n=>[n.profile_id,{id:n.profile_id,name:sourceGroupName(n),count:w.plan.value!.nodes.filter(x=>x.profile_id===n.profile_id).length}])||[]).values()])
const modelServices=computed(()=>selectedServiceIds.value.filter(id=>id==='antigravity'))
const modelRisk=computed(()=>{
  const p=w.plan.value
  if(!p||!modelServices.value.length||p.nodes.length<=3)return ''
  return `所选解锁包含 Antigravity，将从 ${p.nodes.length} 个节点的 IP 发起请求，有风控风险；每次都会使用已绑定账号真实模型额度。`
})
const selectedSections=computed(()=>[...(w.plan.value?.projects||[]),...(w.plan.value?.includeReputation?['reputation']:[])])
const activeSection=ref<string>(selectedSections.value[0]||'service')
watch(selectedSections,(current,previous)=>{
 const added=current.find(id=>!previous.includes(id))
 if(added)activeSection.value=added
 else if(!current.includes(activeSection.value))activeSection.value=current[0]||''
})
const sectionTabs=computed(()=>{
 const p=w.plan.value;if(!p)return []
 const tabs:{id:string;label:string;meta:string}[]=[]
 if(p.projects.includes('latency'))tabs.push({id:'latency',label:'延迟检测',meta:`${p.target==='all'?'四站':targets.find(t=>t.id===p.target)?.name||p.target} · ${p.samples}次`})
 if(p.projects.includes('download'))tabs.push({id:'download',label:'下载测速',meta:`${preferredDownloadSource.value==='ovh'?'OVH':'CF'} · ${adaptiveDownload.value?'稳定停':p.downloadMiB+' MiB'}`})
 if(p.projects.includes('service'))tabs.push({id:'service',label:'解锁检测',meta:standardServiceIds.value.length?`已选 ${standardServiceIds.value.length} 项`:'未选'})
 if(p.includeReputation)tabs.push({id:'reputation',label:'IP信誉/纯净度',meta:`已选 ${reputationServiceIds.value.length} 项`})
 return tabs
})
const skippedServices=computed(()=>{
  const names=new Map(w.catalog.value.map(s=>[s.service_id,s.name])),groups=new Map<string,{id:string;name:string;reason:string;count:number;accounts:Map<string,{name:string;count:number}>}>()
  for(const node of w.plan.value?.nodes||[]){
    const region=w.nodeUnlockRegion(node)
    for(const id of selectedServiceIds.value){
      const reason=unlockServiceSkipReason(id,region,w.skipByApplicableRegions.value,w.settings.value?.unlock_service_regions)
      if(!reason)continue
      const groupKey=id+'|'+reason
      if(!groups.has(groupKey))groups.set(groupKey,{id:groupKey,name:names.get(id)||id,reason,count:0,accounts:new Map()})
      const group=groups.get(groupKey)!;group.count++
      const account=group.accounts.get(node.profile_id)||{name:node.profile_name,count:0};account.count++;group.accounts.set(node.profile_id,account)
    }
  }
  return [...groups.values()]
})
const skippedCount=computed(()=>skippedServices.value.reduce((sum,s)=>sum+s.count,0))

// 预设只设置延迟参数和轮数（并确保勾上延迟），不动用户已勾的下载/服务/信誉项；这些项仍在名称旁列出。
type Preset={id:string;label:string;hint:string;target:string;samples:number;timeout:number;repeats:number}
const presets:Preset[]=[
 {id:'quick',label:'快速观察',hint:'四站各 3 次 · 1 轮',target:'all',samples:3,timeout:5,repeats:1},
 {id:'sites',label:'四站检查',hint:'四站各 10 次 · 1 轮',target:'all',samples:10,timeout:10,repeats:1},
 {id:'recheck',label:'小范围复测',hint:'四站各 6 次 · 3 轮',target:'all',samples:6,timeout:5,repeats:3},
]
const activePreset=computed(()=>{const p=w.plan.value;if(!p||!p.projects.includes('latency'))return undefined;return presets.find(x=>x.target===p.target&&x.samples===p.samples&&x.timeout===p.timeout&&x.repeats===p.repeats)})
function applyPreset(x:Preset){const p=w.plan.value;if(!p)return;if(!p.projects.includes('latency'))p.projects=['latency',...p.projects];p.target=x.target;p.samples=x.samples;p.timeout=x.timeout;p.repeats=x.repeats}
const extras=computed(()=>{const p=w.plan.value;if(!p)return [];return [...p.projects.filter(x=>x!=='latency').map(x=>projectNames[x]),...(p.includeReputation?['IP信誉/纯净度']:[])]})
const latencySamples=computed(()=>{const p=w.plan.value;return p&&p.projects.includes('latency')?p.nodes.length*p.repeats*(p.target==='all'?targets.length:1)*p.samples:0})
const serviceObservations=computed(()=>{const p=w.plan.value;return p?p.nodes.reduce((sum,n)=>sum+w.allowedUnlockServices(n,selectedServiceIds.value).length,0)*p.repeats*(p.serviceSamples??4):0})
async function submit(){
  validation.value=''
  const p=w.plan.value
  if(!p)return
  if(!p.nodes.length){validation.value='请先选择节点';return}
  if(!p.projects.length&&!p.includeReputation){validation.value='至少选择一个检测项目';return}
  if(p.projects.includes('service')&&!standardServiceIds.value.length){validation.value='至少选择一个服务';return}
  if(p.includeReputation&&!reputationServiceIds.value.length){validation.value='至少选择一个 IP 信誉/纯净度项目';return}
  try{await w.runPlan()}catch(e){if(w.plan.value===p)validation.value=e instanceof Error?e.message:String(e)}
}
</script>

<template>
  <Modal v-if="w.plan.value" title="配置这次检测" wide variant="cst-plan" @close="w.plan.value=null">
    <p v-if="w.preparingPlan.value" class="plan-preparing" role="status"><span class="spinner"/>正在准备检测 · 解析节点入口；此时取消不会启动检测</p>
    <div class="plan-layout">
      <fieldset class="plan-editor" :disabled="w.preparingPlan.value" aria-label="检测项目与参数">
    <section class="cst-plan-step" aria-labelledby="plan-project-title">
      <h3 id="plan-project-title">检测项目</h3>
      <div class="cst-presets" role="group" aria-label="检测预设">
        <button v-for="x in presets" :key="x.id" type="button" class="cst-preset" :aria-pressed="activePreset?.id===x.id" @click="applyPreset(x)"><strong>{{x.label}}</strong><small>{{x.hint}}</small></button>
        <p class="cst-preset-state" role="status">当前：<strong>{{activePreset?.label||'自定义'}}</strong><template v-if="extras.length"> · 另含 {{extras.join('、')}}</template><template v-if="modelServices.length"> · <span class="warn">含模型类服务</span></template></p>
      </div>
      <div class="plan-projects">
        <label v-for="p in (['latency','download','service'] as Exclude<Project,'combined'>[])" :key="p" :class="{checked:w.plan.value.projects.includes(p)}"><input v-model="w.plan.value.projects" type="checkbox" :value="p">{{projectNames[p]}}</label>
        <label :class="{checked:w.plan.value.includeReputation}"><input v-model="w.plan.value.includeReputation" type="checkbox">IP信誉/纯净度</label>
      </div>
    </section>

    <section class="cst-plan-step" aria-labelledby="plan-param-title">
      <div class="plan-param-head">
        <h3 id="plan-param-title">检测参数</h3>
        <div v-if="sectionTabs.length" class="plan-tabs" role="tablist" aria-label="检测参数分类">
          <button v-for="tab in sectionTabs" :key="tab.id" type="button" role="tab" :aria-selected="activeSection===tab.id" :class="{active:activeSection===tab.id}" @click="activeSection=tab.id">
            <strong>{{tab.label}}</strong>
            <span>{{tab.meta}}</span>
          </button>
        </div>
      </div>
      <p v-if="!sectionTabs.length" class="muted small">请先在上方勾选至少一个检测项目。</p>
      <div v-if="w.plan.value.projects.includes('latency')" v-show="activeSection==='latency'" class="plan-section plan-tab-panel" role="tabpanel">
        <div class="form-grid">
          <label>检测站点<UiSelect v-model="w.plan.value.target"><option value="all">四站综合检测</option><option v-for="target in targets" :key="target.id" :value="target.id">{{target.name}}</option></UiSelect></label>
          <label>每站采样次数<UiSelect v-model.number="w.plan.value.samples"><option v-for="n in [1,3,6,10,20]" :key="n" :value="n">{{n}} 次</option></UiSelect></label>
          <label>请求超时<UiSelect v-model.number="w.plan.value.timeout"><option v-for="n in [3,5,10,20,30]" :key="n" :value="n">{{n}} 秒</option></UiSelect></label>
          <label>节点并发<UiSelect v-model.number="w.plan.value.concurrency"><option v-for="n in [1,4,8,16]" :key="n" :value="n">{{n}} 个节点</option></UiSelect></label>
        </div>
      </div>
      <div v-if="w.plan.value.projects.includes('download')" v-show="activeSection==='download'" class="plan-section plan-tab-panel" role="tabpanel">
        <div class="form-grid">
          <label>结束条件<UiSelect :model-value="w.plan.value.downloadMode||'fixed'" @update:model-value="w.plan.value.downloadMode=$event==='adaptive'?'adaptive':'fixed'"><option value="adaptive">速度稳定后结束</option><option value="fixed">达到指定流量</option></UiSelect></label>
          <label>首选测速源<UiSelect :model-value="preferredDownloadSource" @update:model-value="w.plan.value.downloadSource=String($event)"><option value="cloudflare">Cloudflare</option><option value="ovh">OVH</option></UiSelect></label>
          <label v-if="!adaptiveDownload">每节点最多读取<UiSelect v-model.number="w.plan.value.downloadMiB"><option v-for="n in [5,10,20,50,100]" :key="n" :value="n">{{n}} MiB</option></UiSelect></label>
          <label>正文时长上限<UiSelect v-model.number="w.plan.value.downloadSeconds"><option v-for="n in [5,10,15,20,30]" :key="n" :value="n">{{n}} 秒</option></UiSelect></label>
        </div>
        <p v-if="adaptiveDownload" class="muted small">节点逐个测速。启动后连续三个约 1 秒的读数相差不超过中间值的 10% 就结束。连续 5 秒无数据会停止。{{preferredDownloadSource==='cloudflare'?'Cloudflare 的文件可能先传完。':''}}</p>
        <p v-else class="muted small">节点逐个测速。达到流量或时长上限即停止，正文不足 2 秒会标为短测均速。</p>
        <p class="muted small">遇到 403／429 自动换源一次。连接另计时，含换源每节点最多 {{w.plan.value.downloadSeconds*2}} 秒。</p>
      </div>
      <div v-if="w.plan.value.projects.includes('service')" v-show="activeSection==='service'" class="plan-section plan-tab-panel" role="tabpanel">
        <UnlockPicker v-model="standardServiceIds" :catalog="standardCatalog"/>
        <div v-if="standardServiceIds.includes('antigravity')" class="inline-warning">
          <p>Antigravity 会用已绑定账号核查 Google 内部地区（fetchUserInfo）并发起真实模型生成请求（可能消耗少量额度）；如需排查多出口负载均衡偶发送中，可将下方「每服务采样次数」设为 4 次或 8 次。</p>
          <p class="plan-token-row">
            <span>凭据状态：<strong>{{w.token.value?.has_token?`已绑定 · ${w.token.value.source||'来源未记录'}`:'尚未绑定或待检测'}}</strong><template v-if="tokenNotice"> · {{tokenNotice}}</template></span>
            <button type="button" class="button ghost small" :disabled="discoveringToken" @click="refreshLocalToken">{{discoveringToken?'检测刷新中…':'检测 / 刷新凭据'}}</button>
          </p>
        </div>
        <p class="muted small">连接复用按实际观测记录。关闭 HTTP 空闲连接不等于节点重连，也不能保证出口稳定。</p>
      </div>
      <div v-if="w.plan.value.includeReputation" v-show="activeSection==='reputation'" class="plan-section plan-tab-panel reputation-section" role="tabpanel">
        <ServicePicker v-model="reputationServiceIds" :catalog="reputationCatalog"/>
        <p class="muted small">IPPure（含 CF 系数与 ASN 人机比）、Ping0、IPQuery、IPLogs 均按独立源站保存，便于多站横向比对；Cloudflare 人机验证并行探测 3 个代表站点（仅观察不绕过）。</p>
      </div>
      <div class="plan-repetition">
        <label v-if="w.plan.value.projects.includes('service')||w.plan.value.includeReputation" class="inline-field">每服务采样次数<UiSelect v-model.number="w.plan.value.serviceSamples"><option v-for="n in [1,2,4,8]" :key="n" :value="n">{{n===4?'4 次（推荐排查漂移）':n===8?'8 次（深度稳定性）':`${n} 次`}}</option></UiSelect></label>
        <label class="inline-field">重复检测<UiSelect v-model.number="w.plan.value.repeats"><option v-for="n in [1,2,3,5]" :key="n" :value="n">{{n}} 轮</option></UiSelect></label>
        <p v-if="w.plan.value.projects.includes('service')||w.plan.value.includeReputation" class="muted small">服务与 IP 检测使用同一采样次数；每项 15 秒总时限（4/8 次采样放宽至 30 秒），未完成会保留已取得的样本。</p>
      </div>
    </section>

      </fieldset>
      <aside class="plan-receipt" aria-label="本次检测范围与预计用量">
    <section class="cst-plan-step" aria-labelledby="plan-scope-title">
      <h3 id="plan-scope-title">本次范围</h3>
      <div class="plan-scope"><strong>{{w.plan.value.nodes.length}} 个节点</strong><span v-for="g in groups" :key="g.id">{{g.name}} · {{g.count}}</span><small class="muted">范围已冻结，检测中不会随筛选变化</small></div>
      <details class="selection-details"><summary>核对所选 {{w.plan.value.nodes.length}} 个节点</summary><div v-for="n in w.plan.value.nodes" :key="n.profile_id+n.node_key" class="compact-row"><span>{{nodeDisplayName(n)}}</span><small>{{n.profile_name}}</small></div></details>
    </section>

    <section class="cst-plan-step plan-budget" aria-labelledby="plan-budget-title">
      <h3 id="plan-budget-title">预计用量</h3>
      <dl class="cst-budget">
        <div v-if="w.plan.value.projects.includes('latency')"><dt>延迟</dt><dd><strong>{{latencySamples}}</strong> 次计划采样<small>{{w.plan.value.nodes.length}} 节点 × {{w.plan.value.target==='all'?targets.length+' 站':'1 站'}} × {{w.plan.value.samples}} 次 × {{w.plan.value.repeats}} 轮</small></dd></div>
        <div v-if="selectedServiceIds.length"><dt>服务与 IP</dt><dd><strong>{{serviceObservations}}</strong> 次计划观察<small>{{selectedServiceIds.length}} 项 · 服务采样 {{w.plan.value.serviceSamples??4}} 次/项；服务内部步骤、跳转和协议开销另计<template v-if="w.plan.value.projects.includes('service')&&standardServiceIds.includes('netflix_unlock')">（Netflix 一次观察约 5 个请求）</template></small></dd></div>
        <div v-if="w.plan.value.projects.includes('download')"><dt>下载</dt><dd v-if="adaptiveDownload">流量随实测速度与下载时长变化<small>实际扣费还受机场倍率和协议开销影响</small></dd><dd v-else>最多读取正文 <strong>{{bytes(downloadVolume)}}</strong><small>实际扣费还受机场倍率和协议开销影响</small></dd></div>
        <div v-if="modelServices.length" class="warn"><dt>模型类</dt><dd>会用已绑定账号发起真实请求<small>可能消耗账号额度，单独计算</small></dd></div>
      </dl>
      <details v-if="skippedCount" class="plan-skipped"><summary>地区规则跳过 {{skippedCount}} 个节点/服务组合</summary><p v-for="item in skippedServices" :key="item.id"><strong>{{item.name}} · 跳过 {{item.count}} 个节点</strong><small>{{item.reason}}</small><small v-for="[id,account] in item.accounts" :key="id">{{account.name}} · {{account.count}}</small></p></details>
      <p class="muted small">合计 {{plannedRequests}} 次计划采样<template v-if="plannedKinds">（{{plannedKinds}}）</template>；这是计划次数，不是实际请求总数。</p>
    </section>
      </aside>
    </div>
    <template #footer><div v-if="validation||modelRisk" class="plan-feedback"><p v-if="validation" class="inline-error" role="alert">{{validation}}</p><p v-if="modelRisk" class="inline-warning">{{modelRisk}}</p></div><button class="button" @click="w.plan.value=null">{{w.preparingPlan.value?'取消准备':'返回调整'}}</button><button class="button primary" :disabled="w.busy.value||w.preparingPlan.value" @click="submit">{{w.preparingPlan.value?'正在准备…':`开始检测 ${w.plan.value.nodes.length} 个节点`}}</button></template>
  </Modal>
</template>

<style scoped>
.plan-layout{display:grid;grid-template-columns:minmax(0,1fr) 268px;gap:22px;align-items:start}
.plan-editor{min-width:0;margin:0;padding:0;border:0}
.plan-editor[disabled]{opacity:.7}
.plan-preparing{display:flex;align-items:center;gap:9px;margin:0 0 14px;padding:10px 12px;border-radius:10px;background:var(--cst-accent-soft);color:var(--cst-accent);font-size:12px}
.plan-editor .cst-plan-step>h3,.plan-param-head h3{margin:0 0 10px;font-size:13px;font-weight:700;color:var(--cst-text)}
.plan-param-head{display:grid;gap:8px;margin-bottom:10px}
.plan-param-head h3{margin:0}
.plan-tabs{display:flex;flex-wrap:wrap;gap:6px;padding:4px;border:1px solid var(--cst-line);border-radius:11px;background:var(--cst-panel-2)}
.plan-tabs button{display:inline-flex;align-items:center;gap:6px;padding:7px 11px;border:1px solid transparent;border-radius:8px;background:transparent;color:var(--cst-muted);font-size:12px;cursor:pointer;transition:background .14s,border-color .14s,color .14s}
.plan-tabs button:hover{color:var(--cst-text);background:var(--cst-hover)}
.plan-tabs button.active{background:var(--cst-panel);border-color:var(--cst-line-strong);color:var(--cst-accent);box-shadow:var(--cst-shadow)}
.plan-tabs button strong{font-size:12.5px;font-weight:650;white-space:nowrap}
.plan-tabs button span{font-size:10.5px;padding:1px 6px;border-radius:999px;background:var(--cst-panel-2);color:var(--cst-muted);white-space:nowrap}
.plan-tabs button.active span{background:var(--cst-accent-soft);color:var(--cst-accent)}
.plan-section.plan-tab-panel{padding:16px;margin:0 0 10px;border:1px solid var(--cst-line-strong);border-radius:12px;background:var(--cst-panel);box-shadow:var(--cst-shadow)}
.plan-repetition{border-top:1px solid var(--cst-line);display:flex;align-items:center;flex-wrap:wrap;gap:0 20px;padding-top:10px;margin-top:6px}
.plan-repetition .inline-field{margin:8px 0}
.plan-repetition>p{flex-basis:100%;line-height:1.6}
.plan-skipped{margin:10px 0;padding:8px 10px;border-radius:10px;background:var(--cst-panel);border:1px solid var(--cst-line);font-size:11px}
.plan-skipped>summary{line-height:1.5}
.plan-skipped p{padding:7px 0;border-bottom:1px solid var(--cst-line);line-height:1.55}
.plan-skipped p:last-child{border-bottom:0;padding-bottom:0}
.plan-skipped small{display:block;color:var(--cst-muted)}
.plan-feedback{flex-basis:100%}
.plan-feedback p{margin:0}
.plan-feedback p+p{margin-top:8px}
.plan-token-row{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap;margin:8px 0 0;padding-top:8px;border-top:1px solid color-mix(in srgb,var(--cst-line) 70%,transparent)}
.plan-receipt{position:sticky;top:0;min-width:0;display:grid;gap:12px}
.plan-receipt .cst-plan-step{padding:14px;border:1px solid var(--cst-line-strong);border-radius:12px;background:var(--cst-panel-2)}
.plan-receipt .cst-plan-step h3{margin:0 0 8px;font-size:12px;font-weight:700;color:var(--cst-muted)}
.plan-receipt .plan-scope{display:flex;flex-direction:column;align-items:flex-start;gap:6px}
.plan-receipt .plan-scope strong{font-size:22px;font-weight:750;line-height:1.25;margin-bottom:2px;color:var(--cst-text)}
.plan-receipt .plan-scope>span{display:inline-flex;align-items:center;padding:2px 9px;border-radius:999px;background:var(--cst-panel);border:1px solid var(--cst-line);font-size:11px;font-weight:600;color:var(--cst-text)}
.plan-receipt .selection-details{margin:10px 0 0;padding-top:10px;border-top:1px solid var(--cst-line)}
.plan-receipt .selection-details[open]{max-height:220px;overflow:auto}
.plan-receipt .compact-row{display:flex;flex-direction:column;align-items:flex-start;gap:2px;overflow-wrap:anywhere;padding:6px 0}
.plan-receipt .cst-budget{display:grid;gap:8px;margin:0}
.plan-receipt .cst-budget>div{border:1px solid var(--cst-line);border-radius:9px;background:var(--cst-panel);padding:9px 11px}
.plan-receipt .cst-budget>div.warn{border-color:color-mix(in srgb,var(--cst-warn) 45%,var(--cst-line));background:color-mix(in srgb,var(--cst-warn) 7%,var(--cst-panel))}
.plan-receipt .cst-budget dt{font-size:11px;font-weight:600;color:var(--cst-muted)}
.plan-receipt .cst-budget dd{margin:3px 0 0;font-size:12px}
.plan-receipt .cst-budget dd strong{font-size:19px;font-weight:750}
.plan-receipt .cst-budget dd small{display:block;margin-top:3px;color:var(--cst-muted);font-size:10.5px;line-height:1.45}
.plan-editor .cst-presets{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;margin-bottom:14px}
.plan-editor .cst-preset{display:flex;flex-direction:column;align-items:flex-start;min-width:0;border:1px solid var(--cst-line);border-radius:10px;background:var(--cst-panel-2);padding:9px 11px;gap:3px;text-align:left;transition:border-color .14s,background .14s,box-shadow .14s}
.plan-editor .cst-preset:hover{border-color:var(--cst-accent);background:var(--cst-panel)}
.plan-editor .cst-preset[aria-pressed=true]{border-color:var(--cst-accent);background:var(--cst-panel);box-shadow:var(--cst-shadow);color:var(--cst-accent)}
.plan-editor .cst-preset strong{font-size:12.5px;font-weight:650}
.plan-editor .cst-preset small{font-size:10.5px;color:var(--cst-muted)}
.plan-editor .cst-preset-state{grid-column:1/-1;margin:2px 0 0;font-size:11.5px;color:var(--cst-muted)}
.plan-editor .plan-projects{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:18px}
.plan-editor .plan-projects label{display:inline-flex;align-items:center;gap:6px;min-height:32px;padding:4px 12px;border:1px solid var(--cst-line);border-radius:999px;background:var(--cst-panel-2);color:var(--cst-muted);font-size:12px;font-weight:550;cursor:pointer;transition:border-color .14s,background .14s,color .14s}
.plan-editor .plan-projects label:hover{border-color:var(--cst-accent);color:var(--cst-text)}
.plan-editor .plan-projects label.checked{background:var(--cst-accent-soft);border-color:color-mix(in srgb,var(--cst-accent) 35%,var(--cst-line));color:var(--cst-accent);font-weight:650}
@media(max-width:850px){.plan-layout{grid-template-columns:minmax(0,1fr)}.plan-receipt{position:static;grid-template-columns:1fr 1fr}.plan-receipt .plan-scope strong{font-size:19px}}
@media(max-width:560px){.plan-receipt{grid-template-columns:1fr}.plan-editor .cst-presets{grid-template-columns:1fr}.plan-editor .cst-preset{flex-direction:row;align-items:center;justify-content:space-between;padding:10px 12px}.plan-editor .cst-preset small{font-size:11px}}
</style>
