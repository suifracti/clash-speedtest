<script setup lang="ts">
import {computed,ref} from 'vue'
import {date,outcomeLabel,type Attempt} from '../domain'
import {attemptObservedAt,latestServices} from '../nodeQuality'
import {serviceIsProfile} from '../sharedResults'
import Modal from './Modal.vue'

const props=withDefaults(defineProps<{attempts?:Attempt[];compact?:boolean}>(),{attempts:()=>[]})
const open=ref(false)

type ExitRecord={
 attempt:Attempt
 serviceId:string
 family:string
 provider:string
 ip:string
 region:string
 ipType:string
 asn:string
 organization:string
 risk:string
 tier:string
 confirmed:boolean
 exitSource:boolean
 reputationSource:boolean
 observedAt:string
 status:string
}
type ExitProjection={key:string;family:string;ip:string;region:string;ipType:string;records:ExitRecord[];observedAt:string}
type DetailField={label:string;value:string}

const field=(attempt:Attempt,...keys:string[])=>{
 const details=attempt.result?.details
 for(const key of keys){
  const value=details?.[key]
  if(typeof value==='string'&&value.trim())return value.trim()
  if(typeof value==='number'&&Number.isFinite(value))return String(value)
 }
 return ''
}
const booleanField=(attempt:Attempt,key:string):boolean|undefined=>{
 const value=attempt.result?.details?.[key]
 if(typeof value==='boolean')return value
 if(typeof value==='string'){
  const normalized=value.trim().toLowerCase()
  if(normalized==='true')return true
  if(normalized==='false')return false
 }
 return undefined
}
const familyOf=(attempt:Attempt,serviceId:string,ip:string)=>{
 const raw=field(attempt,'ip_family').toLowerCase()
 if(raw==='ipv4'||raw==='4')return 'IPv4'
 if(raw==='ipv6'||raw==='6')return 'IPv6'
 if(/(?:^|_)ipv4$/i.test(serviceId))return 'IPv4'
 if(/(?:^|_)ipv6$/i.test(serviceId))return 'IPv6'
 if(ip.includes(':'))return 'IPv6'
 if(/^\d{1,3}(?:\.\d{1,3}){3}$/.test(ip))return 'IPv4'
 return '地址族未知'
}
const providerNames:Record<string,string>={cloudflare_trace:'Cloudflare',exit_ipv4:'出口查询',exit_ipv6:'出口查询',ping0_ip_quality:'Ping0',ippure_ip_quality:'IPPure',ipok_ipv4:'IPOK',ipok_ipv6:'IPOK'}
const isChallenge=(attempt:Attempt,serviceId:string)=>{
 if(serviceId!=='ippure_ip_quality')return false
 const outcome=attempt.result?.outcome||''
 const evidence=[outcome,attempt.result?.summary||'',attempt.result?.error_message||''].join(' ')
 return ['challenge','suspected_challenge'].includes(outcome)||/challenge|captcha|验证页|验证挑战/i.test(evidence)
}
const shortFailure=(attempt:Attempt)=>{
 const raw=attempt.result?.error_message||attempt.result?.summary||''
 const short=raw.replace(/\s+/g,' ').trim()
 if(short)return short.length>120?`${short.slice(0,117)}…`:short
 return outcomeLabel(attempt.result?.outcome)
}
const makeRecord=(attempt:Attempt):ExitRecord=>{
 const serviceId=attempt.service_id||attempt.rule.service_id||'未知服务'
 const ip=field(attempt,'ip')
 const outcome=attempt.result?.outcome||''
 const confirmed=outcome==='profiled'
 const region=field(attempt,'country_code','country','location','city')
 let ipType=field(attempt,'ip_type','origin_type')
 const isResidential=booleanField(attempt,'is_residential'),isIDC=booleanField(attempt,'is_idc')
 if(!ipType&&isResidential!==undefined)ipType=isResidential?'住宅':'非住宅'
 if(!ipType&&isIDC!==undefined)ipType=isIDC?'机房 IDC':'非 IDC（不等于家宽）'
 const resultKind=(attempt.rule as (Attempt['rule']&{result_kind?:string})).result_kind
 const reputationSource=['ping0_ip_quality','ippure_ip_quality','ipok_ipv4','ipok_ipv6'].includes(serviceId)||resultKind==='ip_quality'
 const exitSource=['cloudflare_trace','exit_ipv4','exit_ipv6'].includes(serviceId)||resultKind==='exit_profile'
 const provider=field(attempt,'provider')||providerNames[serviceId]||serviceId
 const status=confirmed
  ? reputationSource?'已取得信誉结果':exitSource?'已取得地址':'已取得画像'
  : isChallenge(attempt,serviceId)?'验证页拦截，未取得风险结果':`查询未完成：${shortFailure(attempt)}`
 return {
  attempt,serviceId,family:familyOf(attempt,serviceId,ip),provider,ip,region,ipType,
  asn:field(attempt,'asn'),organization:field(attempt,'organization','org','isp'),
  risk:field(attempt,'risk_score','fraud_score').replace(/^(未提供|未知|null|undefined)$/i,''),
  tier:field(attempt,'risk_tier','risk_level','tier'),confirmed,exitSource,reputationSource,
  observedAt:attemptObservedAt(attempt),status
 }
}

// The latest result for each rule is current. Older successful records are only
// surfaced in the detail view when that rule's latest result failed.
const currentRecords=computed(()=>latestServices(props.attempts).filter(a=>serviceIsProfile(a)).map(makeRecord))
const records=computed(()=>{
 const groups=new Map<string,ExitRecord[]>()
 for(const record of currentRecords.value){
  const identity=JSON.stringify([record.attempt.profile_id,record.attempt.node_identity_key||record.attempt.node_key,record.attempt.config_revision_key])
  const key=record.ip
   ? `${identity}\u0000${record.family}\u0000${record.ip}`
   : `${identity}\u0000${record.family}\u0000attempt:${record.attempt.attempt_id}`
  const group=groups.get(key)||[]
  group.push(record)
  groups.set(key,group)
 }
 const projections:ExitProjection[]=[]
 for(const [key,items] of groups){
  const data=items.filter(item=>item.confirmed)
  const freshest=data[0]
  const latestAt=items.reduce((latest,item)=>{
   const itemTime=Date.parse(item.observedAt),latestTime=Date.parse(latest)
   return Number.isFinite(itemTime)&&(!Number.isFinite(latestTime)||itemTime>latestTime)?item.observedAt:latest
  },'')
  projections.push({
   key,family:items[0].family,ip:items[0].ip,
   region:data.find(item=>item.region)?.region||'',
   ipType:data.find(item=>item.ipType)?.ipType||'',
   records:items,observedAt:latestAt||freshest?.observedAt||''
  })
 }
 return projections.sort((a,b)=>Number(b.records.some(r=>r.reputationSource&&r.confirmed))-Number(a.records.some(r=>r.reputationSource&&r.confirmed))||Number(!!b.ip)-Number(!!a.ip)||(Date.parse(b.observedAt)||0)-(Date.parse(a.observedAt)||0))
})
const compactRecords=computed(()=>{
 const latestByFamily=new Map<string,ExitProjection>()
 for(const record of records.value){
  if(!record.ip||!record.records.some(source=>source.confirmed&&source.ip))continue
  const current=latestByFamily.get(record.family)
  if(!current||(Date.parse(record.observedAt)||0)>(Date.parse(current.observedAt)||0))latestByFamily.set(record.family,record)
 }
 return [...latestByFamily.values()].sort((a,b)=>{
  const order=(family:string)=>family==='IPv4'?0:family==='IPv6'?1:2
  return order(a.family)-order(b.family)
 })
})
const compactReputations=(record:ExitProjection)=>record.records.filter(source=>source.reputationSource&&source.confirmed)
const compactHasAddressOnlyResult=(record:ExitProjection)=>record.records.some(source=>source.exitSource&&source.confirmed)&&!record.records.some(source=>source.reputationSource)
const riskScore=(record:ExitRecord):number|undefined=>{
 const hasScale=record.serviceId==='ping0_ip_quality'||record.serviceId==='ippure_ip_quality'&&field(record.attempt,'risk_scale_max')==='100'&&field(record.attempt,'risk_direction')==='lower_is_better'
 if(!hasScale||!/^\d+(?:\.\d+)?$/.test(record.risk))return undefined
 const score=Number(record.risk)
 return Number.isFinite(score)&&score>=0&&score<=100?score:undefined
}
const formattedRisk=(record:ExitRecord)=>riskScore(record)===undefined?record.risk:`${record.risk}/100（越低越好）`
const historicalRecords=computed(()=>{
 const latestByService=new Map(latestServices(props.attempts).map(attempt=>[attempt.service_id||attempt.rule.service_id||'',attempt]))
 const seen=new Set<string>(),historical:Attempt[]=[]
 const ordered=[...props.attempts].sort((a,b)=>(Date.parse(attemptObservedAt(b))||0)-(Date.parse(attemptObservedAt(a))||0))
 for(const attempt of ordered){
   const id=attempt.service_id||attempt.rule.service_id||''
   const latest=latestByService.get(id)
   if(!seen.has(id)&&latest&&latest.attempt_id!==attempt.attempt_id&&latest.result?.outcome!=='profiled'&&attempt.result?.outcome==='profiled'&&serviceIsProfile(attempt)){
    seen.add(id)
    historical.push(attempt)
   }
  }
 return historical.map(makeRecord)
})
const detailCount=computed(()=>currentRecords.value.length+historicalRecords.value.length)
const detailFields=(record:ExitRecord,historical=false):DetailField[]=>{
 const queryAt=field(record.attempt,'query_at')
 const exitAt=field(record.attempt,'exit_observed_at')||queryAt
 const cacheStatus=field(record.attempt,'cache_status')
 return [
  {label:'IP',value:record.ip},
  {label:'地区',value:record.region},
  {label:'ASN',value:record.asn},
  {label:'运营商',value:record.organization},
  {label:'类型',value:record.ipType},
  {label:'风险',value:formattedRisk(record)},
  {label:'风险等级',value:record.tier},
  {label:'状态',value:historical?'历史有效画像，不代表当前':record.status},
  {label:'观测时间',value:record.observedAt},
  {label:'查询时间',value:queryAt},
  {label:'出口观察时间',value:exitAt},
  {label:'数据新鲜度',value:field(record.attempt,'data_freshness')},
  {label:'出口范围',value:field(record.attempt,'exit_scope')},
  {label:'缓存',value:cacheStatus==='hit'?'复用本地结果缓存':cacheStatus?`缓存状态：${cacheStatus}`:''}
 ].filter(row=>!!row.value)
}
</script>

<template>
 <div class="node-exit-summary" :class="{compact}" aria-label="已保存的出口画像摘要">
  <button v-if="compact" type="button" class="node-exit-compact" @click="open=true">
   <template v-if="compactRecords.length">
    <span v-for="record in compactRecords" :key="record.key" class="node-exit-compact-record">
     <span class="node-exit-address-line" :title="record.ip"><span>{{record.family}}</span><span>{{[record.region,record.ipType].filter(Boolean).join(' · ')||'已取得地址'}}</span></span>
     <template v-for="source in compactReputations(record)" :key="source.attempt.attempt_id">
      <span class="node-exit-compact-risk"><strong>{{source.provider}}<template v-if="source.risk"> 风险分 {{formattedRisk(source)}}</template><template v-else> · {{source.status}}</template><template v-if="source.tier"> · 等级 {{source.tier}}</template></strong></span>
      <span v-if="riskScore(source)!==undefined" class="node-exit-risk-scale" role="img" :aria-label="`${source.provider} 分数 ${source.risk}/100，0 到 100，越低越好`" :style="{'--risk-position':`${riskScore(source)}%`}"><span>0</span><span class="node-exit-risk-track"><span class="node-exit-risk-marker"></span></span><span>100</span></span>
     </template>
     <span v-if="compactHasAddressOnlyResult(record)" class="node-exit-compact-unchecked">信誉未查</span>
    </span>
   </template>
   <template v-else>出口 / 信誉{{records.length?'未取得结果':'未查询'}}</template>
   <span class="node-exit-compact-arrow" aria-hidden="true"> ↗</span>
  </button>
  <template v-else>
  <div v-if="records.length" class="node-exit-summary-records">
   <article v-for="record in records" :key="record.key" class="node-exit-summary-record">
    <span class="node-exit-summary-main" v-if="record.ip">
     {{record.family}}<template v-if="record.region"> · {{record.region}}</template><template v-if="record.ipType"> · {{record.ipType}}</template>
    </span>
    <code v-if="record.ip&&records.filter(r=>r.ip&&r.family===record.family).length>1" class="node-exit-address" :title="record.ip">{{record.ip}}</code>
    <div v-for="source in record.records.filter(s=>s.reputationSource)" :key="source.attempt.attempt_id" class="node-exit-summary-source">
     <strong v-if="source.confirmed&&source.risk" class="node-exit-risk">{{source.provider}} 风险分 {{formattedRisk(source)}}<template v-if="source.tier"> · 等级 {{source.tier}}</template></strong><span v-else :title="source.status">{{source.provider}} · {{source.status==='验证页拦截，未取得风险结果'?'验证页拦截':source.status}}</span>
    </div>
    <small v-if="record.records.some(source=>source.exitSource&&source.confirmed)&&!record.records.some(other=>other.reputationSource&&other.confirmed)" class="node-exit-summary-address-only">已取得地址，尚无信誉结果</small>
   </article>
  </div>
  <span v-else class="node-exit-summary-empty">出口画像：未查询</span>
  <div class="node-exit-footer"><time v-if="currentRecords[0]?.observedAt">{{date(currentRecords[0].observedAt)}}</time><button class="node-exit-open" type="button" @click="open=true">来源详情（{{detailCount}}）</button></div>
  </template>
 </div>
 <Modal v-if="open" title="已保存的出口画像" wide @close="open=false">
  <p class="small muted">先显示每项服务的最新记录；若最新失败，会补充一条明确标记的历史有效画像。不同来源的风险分数保持独立。</p>
  <p v-if="!currentRecords.length" class="muted">出口画像：未查询。</p>
  <article v-for="record in currentRecords" :key="record.attempt.attempt_id" class="node-exit-evidence">
   <header><strong>{{record.provider}} · {{record.family}}</strong><span :class="record.confirmed?'good':'warn'">{{record.status}}</span></header>
   <dl class="node-exit-fields">
    <div v-for="item in detailFields(record)" :key="item.label"><dt>{{item.label}}</dt><dd><code v-if="item.label==='IP'">{{item.value}}</code><template v-else>{{item.value}}</template></dd></div>
   </dl>
   <p v-if="record.attempt.result?.summary">服务摘要：{{record.attempt.result.summary}}</p>
   <p v-if="record.attempt.result?.error_message" class="warn">失败原因：{{record.attempt.result.error_message}}</p>
   <details><summary>原始服务记录</summary><pre>{{JSON.stringify(record.attempt,null,2)}}</pre></details>
  </article>
  <article v-for="record in historicalRecords" :key="`history-${record.attempt.attempt_id}`" class="node-exit-evidence node-exit-history">
   <header><strong>{{record.provider}} · {{record.family}} · 历史记录</strong><span class="warn">历史有效画像 · 不代表当前</span></header>
   <dl class="node-exit-fields">
    <div v-for="item in detailFields(record,true)" :key="item.label"><dt>{{item.label}}</dt><dd><code v-if="item.label==='IP'">{{item.value}}</code><template v-else>{{item.value}}</template></dd></div>
   </dl>
   <details><summary>原始服务记录</summary><pre>{{JSON.stringify(record.attempt,null,2)}}</pre></details>
  </article>
 </Modal>
</template>

<style scoped>
.node-exit-compact{display:block;width:100%;border:0;background:none;padding:0;text-align:left;font-size:12px;line-height:1.5;color:var(--muted);overflow-wrap:anywhere}.node-exit-compact strong{font-weight:550;color:var(--ink)}.node-exit-compact:hover{color:var(--blue)}.node-exit-compact-arrow{color:var(--blue)}.node-exit-compact-record{display:block;margin:0 0 4px}.node-exit-address-line{display:flex;gap:5px;align-items:baseline;min-width:0}.node-exit-address-line>span{flex:none}.node-exit-address-line code{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px/1.5 ui-monospace,monospace}.node-exit-location{display:block}.node-exit-compact-risk{display:block}.node-exit-compact-unchecked{display:inline;margin-left:4px}.node-exit-risk-scale{display:inline-flex;align-items:center;gap:3px;margin-left:5px;vertical-align:middle;font-size:9px;line-height:1;color:var(--muted)}.node-exit-risk-track{position:relative;display:inline-block;width:64px;height:4px;border-radius:4px;background:linear-gradient(90deg,#75a983 0%,#c5bb72 50%,#c97470 100%)}.node-exit-risk-marker{position:absolute;top:50%;left:var(--risk-position);width:7px;height:7px;transform:translate(-50%,-50%);border:1px solid var(--surface,#fff);border-radius:50%;background:var(--ink,#333);box-shadow:0 0 0 1px rgb(0 0 0 / 18%)}.node-exit-summary-records{display:grid;gap:7px}.node-exit-footer{display:flex;flex-wrap:wrap;gap:4px 10px;align-items:center;font-size:12px;color:var(--muted);margin-top:5px}.node-exit-footer .node-exit-open{margin:0;padding:0}.node-exit-summary-records .node-exit-summary-source{color:var(--muted)}
.node-exit-summary-record{font-size:14px;line-height:1.4}
.node-exit-summary-main{display:block;font-size:13px}.node-exit-address{display:block;max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px/1.5 ui-monospace,monospace;color:var(--muted)}.node-exit-risk{font-size:13px;font-weight:600;color:var(--ink)}.node-exit-summary-source small{font-size:12px;display:block}
.node-exit-summary-source,.node-exit-summary-address-only{display:block;font-size:12px;line-height:1.35}
.node-exit-summary-address-only{color:var(--muted,#777)}
.node-exit-fields{display:grid;grid-template-columns:max-content minmax(0,1fr);gap:4px 10px;margin:8px 0}
.node-exit-fields>div{display:contents}
.node-exit-fields dt{font-size:12px;color:var(--muted,#777)}
.node-exit-fields dd{margin:0;overflow-wrap:anywhere}
.node-exit-history{border-top:1px solid var(--border,#ddd);padding-top:8px}
</style>
