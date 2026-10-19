<script setup lang="ts">
import {computed,ref} from 'vue'
import {date,type Attempt} from '../domain'
import {attemptObservedAt,latestServices} from '../nodeQuality'
import {serviceIsProfile} from '../sharedResults'
import {toExitRecord,completeExitFamilies,exitAddressRows,reputationRows,riskScore,formattedRisk,detailFields,compactExitStatus,reputationAddressHint,exitField,type ExitRecord} from '../exitSummaryPresentation'
import Modal from './Modal.vue'

const props=withDefaults(defineProps<{attempts?:Attempt[];compact?:boolean}>(),{attempts:()=>[]})
const open=ref(false)
const currentRecords=computed(()=>completeExitFamilies(latestServices(props.attempts).filter(a=>serviceIsProfile(a)&&!String(a.service_id||a.rule.service_id).startsWith('ipok_')).map(a=>toExitRecord(a,attemptObservedAt(a)))))
const records=computed(()=>{
 const groups=new Map<string,typeof currentRecords.value>()
 for(const record of currentRecords.value){
  const identity=JSON.stringify([record.attempt.profile_id,record.attempt.node_identity_key||record.attempt.node_key,record.attempt.config_revision_key])
  const key=record.ip?`${identity}\u0000${record.family}\u0000${record.ip}`:`${identity}\u0000${record.family}\u0000attempt:${record.attempt.attempt_id}`
  const group=groups.get(key)||[];group.push(record);groups.set(key,group)
 }
 return [...groups].map(([key,items])=>{const data=items.filter(r=>r.confirmed);return {key,family:items[0].family,ip:items[0].ip,region:data.find(r=>r.region)?.region||'',ipType:data.find(r=>r.ipType)?.ipType||'',records:items}}).sort((a,b)=>Number(b.records.some(r=>r.reputationSource&&r.confirmed))-Number(a.records.some(r=>r.reputationSource&&r.confirmed))||Number(!!b.ip)-Number(!!a.ip))
})
const addresses=computed(()=>exitAddressRows(currentRecords.value).filter(row=>!!row.record))
const reputations=computed(()=>reputationRows(currentRecords.value).filter(row=>!!row.record))
const historicalRecords=computed(()=>{
 const latestByService=new Map(latestServices(props.attempts).map(a=>[a.service_id||a.rule.service_id||'',a]))
 const seen=new Set<string>(),history:Attempt[]=[]
 for(const a of [...props.attempts].sort((a,b)=>(Date.parse(attemptObservedAt(b))||0)-(Date.parse(attemptObservedAt(a))||0))){
  const id=a.service_id||a.rule.service_id||'',latest=latestByService.get(id)
  if(!seen.has(id)&&latest&&latest.attempt_id!==a.attempt_id&&latest.result?.outcome!=='profiled'&&a.result?.outcome==='profiled'&&serviceIsProfile(a)&&!id.startsWith('ipok_')){seen.add(id);history.push(a)}
 }
 return completeExitFamilies(history.map(a=>toExitRecord(a,attemptObservedAt(a))))
})
const detailCount=computed(()=>currentRecords.value.length+historicalRecords.value.length)
const latestAt=computed(()=>currentRecords.value.reduce((v,r)=>(Date.parse(r.observedAt)||0)>(Date.parse(v)||0)?r.observedAt:v,''))
const pureRecord=computed(()=>currentRecords.value.find(r=>r.serviceId==='ippure_ip_quality'))
const metricNumber=(key:string)=>{const raw=pureRecord.value?exitField(pureRecord.value.attempt,key):'';if(!/^\d+(?:\.\d+)?$/.test(raw))return undefined;const n=Number(raw);return n>=0&&n<=100?n:undefined}
const cfRisk=computed(()=>pureRecord.value&&exitField(pureRecord.value.attempt,'cf_status')==='已取得'?metricNumber('cf_risk_score'):undefined)
const traffic=computed(()=>{const human=metricNumber('asn_human_percent'),bot=metricNumber('asn_bot_percent');return human!==undefined&&bot!==undefined&&Math.abs(human+bot-100)<=0.1?{human,bot}:undefined})
const pureField=(key:string)=>pureRecord.value?exitField(pureRecord.value.attempt,key):''
// Display-only captions. Classification and source matching stay in the existing helper.
const providerCaption=(r?:ExitRecord)=>!r?'没有保存记录':!r.confirmed?[r.attempt.result?.http_status?`HTTP ${r.attempt.result.http_status}`:'','未取得风险分'].filter(Boolean).join(' · '):riskScore(r)!==undefined?'风险分 · 越低越好':r.risk?`风险分 ${r.risk} · 刻度未声明`:'未返回风险分'
</script>

<template>
 <div class="node-exit-summary" :class="{compact}" aria-label="已保存的出口画像摘要">
  <template v-if="compact">
   <button v-if="!currentRecords.length" type="button" class="node-exit-compact" @click="open=true">出口 / 信誉未查询</button>
   <template v-else>
   <button v-if="!reputations.length" type="button" class="node-exit-compact" @click="open=true">出口 / 信誉详情</button>
   <div v-if="reputations.length" class="node-exit-provider-grid" aria-label="各来源信誉结果，分数独立">
    <div v-for="provider in reputations" :key="provider.serviceId" :class="provider.serviceId==='ippure_ip_quality'?'ippure-group':'node-exit-provider-slot'">
    <button type="button" class="node-exit-provider" :data-provider="provider.serviceId" :class="{'has-score':provider.record?.confirmed&&!!provider.record.risk,'no-score':!provider.record?.confirmed||!provider.record.risk}" :title="provider.provider+' · '+reputationAddressHint(provider.record,addresses)+' · '+providerCaption(provider.record)+' · '+date(provider.record?.observedAt)" @click="open=true">
     <span class="node-exit-provider-head"><span class="node-exit-provider-title">{{provider.provider}}</span><span class="node-exit-provider-family">{{reputationAddressHint(provider.record,addresses)}}</span><strong v-if="provider.record?.confirmed&&provider.record.risk" class="node-exit-compact-risk">{{provider.record.risk}}</strong><strong v-else class="node-exit-provider-status">{{compactExitStatus(provider.record)}}</strong></span>
     <span v-if="provider.record&&riskScore(provider.record)!==undefined" class="node-exit-risk-scale" role="img" :aria-label="`${provider.provider} 分数 ${provider.record.risk}/100，0 到 100，越低越好`" :style="{'--risk-position':`${riskScore(provider.record)}%`}"><span>0</span><span class="node-exit-risk-track"><span class="node-exit-risk-marker"></span></span><span>100</span></span>
    </button>
    <template v-if="provider.serviceId==='ippure_ip_quality'">
     <button type="button" class="node-exit-provider" data-metric="cf" :title="[pureField('cf_scope'),pureField('cf_query_at')].filter(Boolean).join(' · ')" @click="open=true">
      <span class="node-exit-provider-head"><span class="node-exit-provider-title">Cloudflare 系数</span><span>{{pureField('cf_ip_family')}}</span><strong v-if="cfRisk!==undefined" class="node-exit-compact-risk">{{cfRisk}}</strong><span v-else>{{pureField('cf_status')||'未查询'}}</span></span>
      <span v-if="cfRisk!==undefined" class="node-exit-risk-scale" role="img" :aria-label="`Cloudflare 风险系数 ${cfRisk}，0到100，越低越好`" :style="{'--risk-position':`${cfRisk}%`}"><span>0</span><span class="node-exit-risk-track"><span class="node-exit-risk-marker"></span></span><span>100</span></span>
     </button>
     <button type="button" class="ippure-traffic node-exit-provider" data-metric="asn-traffic" :title="'整个ASN的统计，并非当前节点流量 · '+pureField('asn_traffic_query_at')" @click="open=true">
      <span class="node-exit-provider-head"><span class="node-exit-provider-title">ASN 人机流量比</span><span>{{pureField('asn_traffic_asn')}}</span></span>
      <template v-if="traffic"><span class="ippure-traffic-track" role="img" :aria-label="`人类 ${traffic.human.toFixed(2)}%，机器 ${traffic.bot.toFixed(2)}%`"><span :style="{width:`${traffic.human}%`}"></span></span><span class="ippure-traffic-values"><span>人类 {{traffic.human.toFixed(2)}}%</span><span>机器 {{traffic.bot.toFixed(2)}}%</span></span></template>
      <span v-else>{{pureField('asn_traffic_status')||'未查询'}}</span>
     </button>
     <span class="ippure-note" data-provider="ippure_ipv6" title="IPPure官方暂不计算IPv6风险系数">IPPure IPv6：不支持评分</span>
    </template>
    </div>
   </div>
   </template>
  </template>
  <template v-else>
   <div v-if="records.length" class="node-exit-summary-records">
    <article v-for="record in records" :key="record.key" class="node-exit-summary-record">
     <span class="node-exit-summary-main" v-if="record.ip">{{record.family}}<template v-if="record.region"> · {{record.region}}</template><template v-if="record.ipType"> · {{record.ipType}}</template></span>
     <code v-if="record.ip&&records.filter(r=>r.ip&&r.family===record.family).length>1" class="node-exit-address" :title="record.ip">{{record.ip}}</code>
     <div v-for="source in record.records.filter(s=>s.reputationSource)" :key="source.attempt.attempt_id" class="node-exit-summary-source"><strong v-if="source.confirmed&&source.risk" class="node-exit-risk">{{source.provider}} 风险分 {{formattedRisk(source)}}<template v-if="source.tier"> · 等级 {{source.tier}}</template></strong><span v-else :title="source.status">{{source.provider}} · {{source.status==='验证页拦截，未取得风险结果'?'验证页拦截':source.status}}</span></div>
     <small v-if="record.records.some(s=>s.exitSource&&s.confirmed)&&!record.records.some(s=>s.reputationSource&&s.confirmed)" class="node-exit-summary-address-only">已取得地址，尚无信誉结果</small>
    </article>
   </div>
   <span v-else class="node-exit-summary-empty">出口画像：未查询</span>
   <div class="node-exit-footer"><time v-if="latestAt">{{date(latestAt)}}</time><button class="node-exit-open" type="button" @click="open=true">来源详情（{{detailCount}}）</button></div>
  </template>
 </div>
 <Modal v-if="open" title="已保存的出口画像" wide @close="open=false">
  <p class="small muted">各来源独立显示。地址已取得不代表信誉正常；绑定网卡不等于实际绕过 TUN。最新查询失败时，过去的成功只作为历史记录。</p>
  <p v-if="!currentRecords.length" class="muted">出口画像：未查询。</p>
  <article v-for="record in currentRecords" :key="record.attempt.attempt_id" class="node-exit-evidence">
   <header><strong>{{record.provider}} · {{record.family}}</strong><span :class="record.confirmed?'good':'warn'">{{record.status}}</span></header>
   <dl class="node-exit-fields"><div v-for="item in detailFields(record)" :key="item.label"><dt>{{item.label}}</dt><dd><code v-if="item.label==='IP'">{{item.value}}</code><template v-else>{{item.value}}</template></dd></div></dl>
   <p v-if="record.attempt.result?.summary">服务摘要：{{record.attempt.result.summary}}</p><p v-if="record.attempt.result?.error_message" class="warn">失败原因：{{record.attempt.result.error_message}}</p>
   <details><summary>原始服务记录</summary><pre>{{JSON.stringify(record.attempt,null,2)}}</pre></details>
  </article>
  <article v-for="record in historicalRecords" :key="`history-${record.attempt.attempt_id}`" class="node-exit-evidence node-exit-history">
   <header><strong>{{record.provider}} · {{record.family}} · 历史记录</strong><span class="warn">历史有效画像 · 不代表当前</span></header>
   <dl class="node-exit-fields"><div v-for="item in detailFields(record,true)" :key="item.label"><dt>{{item.label}}</dt><dd><code v-if="item.label==='IP'">{{item.value}}</code><template v-else>{{item.value}}</template></dd></div></dl>
   <details><summary>原始服务记录</summary><pre>{{JSON.stringify(record.attempt,null,2)}}</pre></details>
  </article>
 </Modal>
</template>

<style scoped>
.node-exit-summary{min-width:0;font-size:12px;line-height:1.5;color:var(--muted)}
.node-exit-compact,.node-exit-provider{border:0;background:none;padding:0;text-align:left;color:inherit;font:inherit;cursor:pointer}

.node-exit-provider-slot{min-width:0}.ippure-group{grid-column:1/-1;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:5px 10px;border-top:1px solid var(--line);padding-top:5px}.ippure-traffic,.ippure-note{grid-column:1/-1}.ippure-note{font-size:11px;color:var(--muted)}.ippure-traffic-track{display:block;width:100%;height:4px;background:#ff718f;border-radius:3px;overflow:hidden}.ippure-traffic-track>span{display:block;height:100%;background:#48c78e}.ippure-traffic-values{display:flex;justify-content:space-between;width:100%;font-size:11px}.ippure-traffic-values>span:first-child{color:#208858}.ippure-traffic-values>span:last-child{color:#b1405c}
.node-exit-provider-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:5px 10px;margin-top:5px}.node-exit-provider{display:flex;flex-direction:column;gap:2px;min-width:0}.node-exit-provider-head{display:flex;align-items:baseline;flex-wrap:wrap;gap:2px 4px;width:100%}.node-exit-provider-title{font-weight:550;color:var(--ink)}.node-exit-provider-title>span{display:none}.node-exit-provider-family{font-size:12px}.node-exit-compact-risk,.node-exit-provider-status{font-size:12px;font-weight:550;color:var(--ink)}.node-exit-provider-caption,.node-exit-provider-tier,.node-exit-provider time{font-size:12px;color:var(--muted)}.node-exit-provider.no-score .node-exit-provider-status{color:var(--warning,#986700)}.node-exit-provider:hover,.node-exit-compact:hover{color:var(--blue)}
.node-exit-risk-scale{width:100%;display:inline-flex;align-items:center;gap:3px;vertical-align:middle;font-size:10px;color:var(--muted)}.node-exit-risk-track{position:relative;display:inline-block;width:auto;flex:1;min-width:25px;height:4px;border-radius:4px;background:linear-gradient(90deg,#75a983,#c5bb72 50%,#c97470)}.node-exit-risk-marker{position:absolute;top:50%;left:var(--risk-position);width:6px;height:6px;transform:translate(-50%,-50%);border:1px solid var(--surface,#fff);border-radius:50%;background:var(--ink)}.node-exit-open{margin-top:4px;color:var(--blue);font-size:12px;border:0;background:none;padding:0;text-align:left;cursor:pointer}
.node-exit-summary-records{display:grid;gap:7px}.node-exit-summary-main,.node-exit-summary-source,.node-exit-summary-address-only{display:block}.node-exit-address{display:block;max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px/1.5 ui-monospace,monospace}.node-exit-footer{display:flex;gap:4px 10px;align-items:center;margin-top:5px}.node-exit-fields{display:grid;grid-template-columns:max-content minmax(0,1fr);gap:4px 10px;margin:8px 0}.node-exit-fields>div{display:contents}.node-exit-fields dt{font-size:12px;color:var(--muted)}.node-exit-fields dd{margin:0;overflow-wrap:anywhere}.node-exit-history{border-top:1px solid var(--line);padding-top:8px}
</style>
