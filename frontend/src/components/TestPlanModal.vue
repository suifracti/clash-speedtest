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
  <Modal v-if="w.plan.value" title="配置这次检测" wide @close="w.plan.value=null">
    <div class="plan-scope"><strong>{{w.plan.value.nodes.length}} 个节点</strong><span v-for="g in groups" :key="g.id">{{g.name}} · {{g.count}}</span></div>
    <p class="plan-budget muted small">范围已冻结：{{w.plan.value.nodes.length}} 节点 · {{w.plan.value.repeats}} 轮 · {{plannedRequests}} 次计划采样<span v-if="selectedServiceIds.length"> · 服务采样 {{w.plan.value.serviceSamples??4}} 次/项</span>（跳转、多请求服务及协议开销另计） · 下载正文最多 {{bytes(downloadVolume)}}</p>
    <div class="plan-projects">
      <label v-for="p in (['latency','download','service'] as Exclude<Project,'combined'>[])" :key="p" :class="{checked:w.plan.value.projects.includes(p)}"><input v-model="w.plan.value.projects" type="checkbox" :value="p">{{projectNames[p]}}</label>
      <label :class="{checked:w.plan.value.includeReputation}"><input v-model="w.plan.value.includeReputation" type="checkbox">IP信誉/纯净度</label>
    </div>

    <div v-if="w.plan.value.projects.includes('latency')" class="plan-section">
      <h3>延迟检测</h3>
      <div class="form-grid">
        <label>检测站点<UiSelect v-model="w.plan.value.target"><option value="all">四站综合检测</option><option v-for="target in targets" :key="target.id" :value="target.id">{{target.name}}</option></UiSelect></label>
        <label>每站采样次数<UiSelect v-model.number="w.plan.value.samples"><option v-for="n in [1,3,6,10,20]" :key="n" :value="n">{{n}} 次</option></UiSelect></label>
        <label>请求超时<UiSelect v-model.number="w.plan.value.timeout"><option v-for="n in [3,5,10,20,30]" :key="n" :value="n">{{n}} 秒</option></UiSelect></label>
        <label>节点并发<UiSelect v-model.number="w.plan.value.concurrency"><option v-for="n in [1,4,8,16]" :key="n" :value="n">{{n}} 个节点</option></UiSelect></label>
      </div>
    </div>

    <div v-if="w.plan.value.projects.includes('download')" class="plan-section">
      <h3>下载测速</h3>
      <div class="form-grid">
        <label>每节点最多读取<UiSelect v-model.number="w.plan.value.downloadMiB"><option v-for="n in [5,10,20,50,100]" :key="n" :value="n">{{n}} MiB</option></UiSelect></label>
        <label>每节点最多持续<UiSelect v-model.number="w.plan.value.downloadSeconds"><option v-for="n in [5,10,20,30]" :key="n" :value="n">{{n}} 秒</option></UiSelect></label>
      </div>
      <p class="muted small">逐个下载，避免抢占带宽。本次最多读取 {{bytes(downloadVolume)}}；实际扣费还受机场倍率和协议开销影响。</p>
    </div>

    <div v-if="w.plan.value.projects.includes('service')" class="plan-section">
      <h3>服务可用性</h3>
      <label class="inline-field">每服务采样次数<UiSelect v-model.number="w.plan.value.serviceSamples"><option v-for="n in [1,2,4]" :key="n" :value="n">{{n}} 次</option></UiSelect></label>
      <p class="muted small">每项共用15秒总时限；1次只代表本次观察，多次未完成会保留已取得的样本。</p>
      <ServicePicker v-model="standardServiceIds" :catalog="standardCatalog"/>
      <p v-if="standardServiceIds.includes('antigravity')" class="inline-warning">Antigravity 会用已绑定账号发起真实模型请求，可能消耗额度。</p>
      <p class="muted small">连接复用按实际观测记录。关闭 HTTP 空闲连接不等于节点重连，也不能保证出口稳定。</p>
    </div>

    <div v-if="w.plan.value.includeReputation" class="plan-section reputation-section">
      <h3>IP信誉/纯净度</h3>
      <ServicePicker v-model="reputationServiceIds" :catalog="reputationCatalog"/>
      <p class="muted small">IPPure 包含 IPv4 风险、Cloudflare 请求环境系数和 ASN 人机流量比。IPPure 本身不提供 IPv6 风险分；各项结果独立保存。</p>
    </div>

    <label class="inline-field">重复检测<UiSelect v-model.number="w.plan.value.repeats"><option v-for="n in [1,2,3,5]" :key="n" :value="n">{{n}} 轮</option></UiSelect></label>
    <details class="selection-details"><summary>核对所选 {{w.plan.value.nodes.length}} 个节点</summary><div v-for="n in w.plan.value.nodes" :key="n.profile_id+n.node_key" class="compact-row"><span>{{nodeDisplayName(n)}}</span><small>{{n.profile_name}}</small></div></details>
    <p v-if="validation" class="inline-error" role="alert">{{validation}}</p>
    <template #footer><button class="button" @click="w.plan.value=null">返回调整</button><button class="button primary" :disabled="submitting||w.busy.value" @click="submit">开始检测 {{w.plan.value.nodes.length}} 个节点</button></template>
  </Modal>
</template>
