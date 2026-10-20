<script setup lang="ts">
import {computed,ref} from 'vue'
import {nodeDisplayName} from '../nodePresentation'
import {bytes,isReputationRule,projectNames,targets,type Project} from '../domain'
import {useWorkspace} from '../workspace'
import UiSelect from './UiSelect.vue'
import Modal from './Modal.vue'
import ServicePicker from './ServicePicker.vue'

const w=useWorkspace(),submitting=ref(false),validation=ref('')
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
const downloadVolume=computed(()=>w.plan.value?.projects.includes('download')?w.plan.value.nodes.length*w.plan.value.downloadMiB*1048576*w.plan.value.repeats:0)
const selectedServiceIds=computed(()=>{
  const p=w.plan.value
  if(!p)return []
  return [...new Set([...(p.projects.includes('service')?p.serviceIds.filter(id=>!isReputationId(id)):[]),...(p.includeReputation?(p.reputationServiceIds||[]).filter(isReputationId):[])])]
})
const plannedRequests=computed(()=>{const p=w.plan.value;if(!p)return 0;return p.nodes.length*p.repeats*((p.projects.includes('latency')?(p.target==='all'?targets.length:1)*p.samples:0)+(p.projects.includes('download')?1:0)+selectedServiceIds.value.length*(p.serviceSamples??4))})
const groups=computed(()=>[...new Map(w.plan.value?.nodes.map(n=>[n.profile_id,{id:n.profile_id,name:n.profile_name,count:w.plan.value!.nodes.filter(x=>x.profile_id===n.profile_id).length}])||[]).values()])

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
// 只有 Antigravity 会用已绑定账号发真实模型请求；其余 AI 项是网页/资源可达性检查。
const modelServices=computed(()=>selectedServiceIds.value.filter(id=>id==='antigravity'))
const latencySamples=computed(()=>{const p=w.plan.value;return p&&p.projects.includes('latency')?p.nodes.length*p.repeats*(p.target==='all'?targets.length:1)*p.samples:0})
const serviceObservations=computed(()=>{const p=w.plan.value;return p?p.nodes.length*p.repeats*selectedServiceIds.value.length*(p.serviceSamples??4):0})
async function submit(){
  validation.value=''
  const p=w.plan.value
  if(!p)return
  if(!p.nodes.length){validation.value='请先选择节点';return}
  if(!p.projects.length&&!p.includeReputation){validation.value='至少选择一个检测项目';return}
  if(p.projects.includes('service')&&!standardServiceIds.value.length){validation.value='至少选择一个服务';return}
  if(p.includeReputation&&!reputationServiceIds.value.length){validation.value='至少选择一个 IP 信誉/纯净度项目';return}
  submitting.value=true
  try{await w.runPlan()}catch(e){validation.value=e instanceof Error?e.message:String(e)}finally{submitting.value=false}
}
</script>

<template>
  <Modal v-if="w.plan.value" title="配置这次检测" wide variant="cst-plan" @close="w.plan.value=null">
    <section class="cst-plan-step" aria-labelledby="plan-scope-title">
      <h3 id="plan-scope-title"><span>1</span>范围</h3>
      <div class="plan-scope"><strong>{{w.plan.value.nodes.length}} 个节点</strong><span v-for="g in groups" :key="g.id">{{g.name}} · {{g.count}}</span><small class="muted">范围已冻结，检测中不会随筛选变化</small></div>
      <details class="selection-details"><summary>核对所选 {{w.plan.value.nodes.length}} 个节点</summary><div v-for="n in w.plan.value.nodes" :key="n.profile_id+n.node_key" class="compact-row"><span>{{nodeDisplayName(n)}}</span><small>{{n.profile_name}}</small></div></details>
    </section>

    <section class="cst-plan-step" aria-labelledby="plan-project-title">
      <h3 id="plan-project-title"><span>2</span>项目</h3>
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
      <h3 id="plan-param-title"><span>3</span>参数</h3>
      <div v-if="w.plan.value.projects.includes('latency')" class="plan-section">
        <h4>延迟检测</h4>
        <div class="form-grid">
          <label>检测站点<UiSelect v-model="w.plan.value.target"><option value="all">四站综合检测</option><option v-for="target in targets" :key="target.id" :value="target.id">{{target.name}}</option></UiSelect></label>
          <label>每站采样次数<UiSelect v-model.number="w.plan.value.samples"><option v-for="n in [1,3,6,10,20]" :key="n" :value="n">{{n}} 次</option></UiSelect></label>
          <label>请求超时<UiSelect v-model.number="w.plan.value.timeout"><option v-for="n in [3,5,10,20,30]" :key="n" :value="n">{{n}} 秒</option></UiSelect></label>
          <label>节点并发<UiSelect v-model.number="w.plan.value.concurrency"><option v-for="n in [1,4,8,16]" :key="n" :value="n">{{n}} 个节点</option></UiSelect></label>
        </div>
      </div>
      <div v-if="w.plan.value.projects.includes('download')" class="plan-section">
        <h4>下载测速</h4>
        <div class="form-grid">
          <label>测速源<UiSelect :model-value="w.plan.value.downloadSource||'cloudflare'" @update:model-value="w.plan.value.downloadSource=String($event)"><option value="cloudflare">Cloudflare</option><option value="cachefly">Cachefly</option></UiSelect></label>
          <label>每节点最多读取<UiSelect v-model.number="w.plan.value.downloadMiB"><option v-for="n in [5,10,20,50,100]" :key="n" :value="n">{{n}} MiB</option></UiSelect></label>
          <label>每节点最多持续<UiSelect v-model.number="w.plan.value.downloadSeconds"><option v-for="n in [5,10,20,30]" :key="n" :value="n">{{n}} 秒</option></UiSelect></label>
        </div>
        <p class="muted small">逐个下载，避免抢占带宽。</p>
      </div>
      <div v-if="w.plan.value.projects.includes('service')" class="plan-section">
        <h4>服务可用性</h4>
        <label class="inline-field">每服务采样次数<UiSelect v-model.number="w.plan.value.serviceSamples"><option v-for="n in [1,2,4]" :key="n" :value="n">{{n}} 次</option></UiSelect></label>
        <p class="muted small">每项共用15秒总时限；1次只代表本次观察，多次未完成会保留已取得的样本。</p>
        <ServicePicker v-model="standardServiceIds" :catalog="standardCatalog"/>
        <p v-if="standardServiceIds.includes('antigravity')" class="inline-warning">Antigravity 会用已绑定账号发起真实模型请求，可能消耗额度。</p>
        <p class="muted small">连接复用按实际观测记录。关闭 HTTP 空闲连接不等于节点重连，也不能保证出口稳定。</p>
      </div>
      <div v-if="w.plan.value.includeReputation" class="plan-section reputation-section">
        <h4>IP信誉/纯净度</h4>
        <ServicePicker v-model="reputationServiceIds" :catalog="reputationCatalog"/>
        <p class="muted small">IPPure 包含 IPv4 风险、Cloudflare 请求环境系数和 ASN 人机流量比。IPPure 本身不提供 IPv6 风险分；各项结果独立保存。</p>
      </div>
      <label class="inline-field">重复检测<UiSelect v-model.number="w.plan.value.repeats"><option v-for="n in [1,2,3,5]" :key="n" :value="n">{{n}} 轮</option></UiSelect></label>
    </section>

    <section class="cst-plan-step plan-budget" aria-labelledby="plan-budget-title">
      <h3 id="plan-budget-title"><span>4</span>预算确认</h3>
      <dl class="cst-budget">
        <div v-if="w.plan.value.projects.includes('latency')"><dt>延迟</dt><dd><strong>{{latencySamples}}</strong> 次计划采样<small>{{w.plan.value.nodes.length}} 节点 × {{w.plan.value.target==='all'?targets.length+' 站':'1 站'}} × {{w.plan.value.samples}} 次 × {{w.plan.value.repeats}} 轮</small></dd></div>
        <div v-if="selectedServiceIds.length"><dt>服务与 IP</dt><dd><strong>{{serviceObservations}}</strong> 次计划观察<small>{{selectedServiceIds.length}} 项 · 服务采样 {{w.plan.value.serviceSamples??4}} 次/项；多请求服务（如 Netflix 一次观察约 5 个请求）、跳转和协议开销另计</small></dd></div>
        <div v-if="w.plan.value.projects.includes('download')"><dt>下载</dt><dd>最多读取正文 <strong>{{bytes(downloadVolume)}}</strong><small>实际扣费还受机场倍率和协议开销影响</small></dd></div>
        <div v-if="modelServices.length" class="warn"><dt>模型类</dt><dd>会用已绑定账号发起真实请求<small>可能消耗账号额度，单独计算</small></dd></div>
      </dl>
      <p class="muted small">合计 {{plannedRequests}} 次计划采样（延迟采样 + 服务观察{{w.plan.value.projects.includes('download')?' + 每节点 1 次下载':''}}）；这是计划次数，不是实际请求总数。</p>
    </section>
    <p v-if="validation" class="inline-error" role="alert">{{validation}}</p>
    <template #footer><button class="button" @click="w.plan.value=null">返回调整</button><button class="button primary" :disabled="submitting||w.busy.value" @click="submit">开始检测 {{w.plan.value.nodes.length}} 个节点</button></template>
  </Modal>
</template>
