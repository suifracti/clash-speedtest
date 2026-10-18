<script setup lang="ts">
import {computed,ref} from 'vue'
import {date,type Attempt} from '../domain'
import {attemptObservedAt,latestServices} from '../nodeQuality'
import {serviceIsProfile} from '../sharedResults'
import {toExitRecord,completeExitFamilies,exitAddressRows,reputationRows,riskScore,formattedRisk,detailFields,compactExitStatus,reputationAddressHint,type ExitRecord} from '../exitSummaryPresentation'
import Modal from './Modal.vue'

const props=withDefaults(defineProps<{attempts?:Attempt[];compact?:boolean}>(),{attempts:()=>[]})
const open=ref(false)
const currentRecords=computed(()=>completeExitFamilies(latestServices(props.attempts).filter(a=>serviceIsProfile(a)).map(a=>toExitRecord(a,attemptObservedAt(a)))))
const records=computed(()=>{
 const groups=new Map<string,typeof currentRecords.value>()
 for(const record of currentRecords.value){
  const identity=JSON.stringify([record.attempt.profile_id,record.attempt.node_identity_key||record.attempt.node_key,record.attempt.config_revision_key])
  const key=record.ip?`${identity}\u0000${record.family}\u0000${record.ip}`:`${identity}\u0000${record.family}\u0000attempt:${record.attempt.attempt_id}`
  const group=groups.get(key)||[];group.push(record);groups.set(key,group)
 }
 return [...groups].map(([key,items])=>{const data=items.filter(r=>r.confirmed);return {key,family:items[0].family,ip:items[0].ip,region:data.find(r=>r.region)?.region||'',ipType:data.find(r=>r.ipType)?.ipType||'',records:items}}).sort((a,b)=>Number(b.records.some(r=>r.reputationSource&&r.confirmed))-Number(a.records.some(r=>r.reputationSource&&r.confirmed))||Number(!!b.ip)-Number(!!a.ip))
})
const addresses=computed(()=>exitAddressRows(currentRecords.value))
const reputations=computed(()=>reputationRows(currentRecords.value))
const historicalRecords=computed(()=>{
 const latestByService=new Map(latestServices(props.attempts).map(a=>[a.service_id||a.rule.service_id||'',a]))
 const seen=new Set<string>(),history:Attempt[]=[]
 for(const a of [...props.attempts].sort((a,b)=>(Date.parse(attemptObservedAt(b))||0)-(Date.parse(attemptObservedAt(a))||0))){
  const id=a.service_id||a.rule.service_id||'',latest=latestByService.get(id)
  if(!seen.has(id)&&latest&&latest.attempt_id!==a.attempt_id&&latest.result?.outcome!=='profiled'&&a.result?.outcome==='profiled'&&serviceIsProfile(a)){seen.add(id);history.push(a)}
 }
 return completeExitFamilies(history.map(a=>toExitRecord(a,attemptObservedAt(a))))
})
const detailCount=computed(()=>currentRecords.value.length+historicalRecords.value.length)
const latestAt=computed(()=>currentRecords.value.reduce((v,r)=>(Date.parse(r.observedAt)||0)>(Date.parse(v)||0)?r.observedAt:v,''))
// Display-only captions. Classification and source matching stay in the existing helper.
const providerCaption=(r?:ExitRecord)=>!r?'没有保存记录':!r.confirmed?[r.attempt.result?.http_status?`HTTP ${r.attempt.result.http_status}`:'','未取得风险分'].filter(Boolean).join(' · '):riskScore(r)!==undefined?'风险分 · 越低越好':r.risk?`风险分 ${r.risk} · 刻度未声明`:'未返回风险分'
</script>

<template>
 <div class="node-exit-summary" :class="{compact}" aria-label="已保存的出口画像摘要">
  <template v-if="compact">
   <div class="node-exit-addresses" aria-label="IPv4 与 IPv6 分开记录">
    <button v-for="address in addresses" :key="address.family" type="button" class="node-exit-address-row exit-ledger-address" :data-family="address.family" @click="open=true">
     <span class="node-exit-family">{{address.family}}</span>
     <span class="node-exit-address-copy"><strong>{{address.status}}</strong><small>{{address.record?.confirmed?[address.region||'地区未返回',address.ipType].filter(Boolean).join(' · '):address.record?.attempt.result?.http_status?'HTTP '+address.record.attempt.result.http_status:'无当前地址记录'}}</small></span>
     <span class="node-exit-compact-arrow" aria-hidden="true">↗</span>
    </button>
   </div>
   <div class="node-exit-provider-grid" aria-label="各来源信誉结果，分数独立">
    <button v-for="provider in reputations" :key="provider.serviceId" type="button" class="node-exit-provider" :data-provider="provider.serviceId" :class="{'has-score':provider.record?.confirmed&&!!provider.record.risk,'no-score':!provider.record?.confirmed||!provider.record.risk}" @click="open=true">
     <span class="node-exit-provider-title">{{provider.provider}}<span aria-hidden="true">↗</span></span><span class="node-exit-provider-family">{{reputationAddressHint(provider.record,addresses)}}</span>
     <strong v-if="provider.record?.confirmed&&provider.record.risk" class="node-exit-compact-risk">{{provider.record.risk}}<small v-if="riskScore(provider.record)!==undefined"> / 100</small></strong>
     <strong v-else class="node-exit-provider-status">{{compactExitStatus(provider.record)}}</strong>
     <span class="node-exit-provider-caption">{{providerCaption(provider.record)}}</span>
     <span v-if="provider.record&&riskScore(provider.record)!==undefined" class="node-exit-risk-scale" role="img" :aria-label="`${provider.provider} 分数 ${provider.record.risk}/100，0 到 100，越低越好`" :style="{'--risk-position':`${riskScore(provider.record)}%`}"><span>0</span><span class="node-exit-risk-track"><span class="node-exit-risk-marker"></span></span><span>100</span></span>
     <span v-if="provider.record?.confirmed&&provider.record.tier" class="node-exit-provider-tier">来源等级 {{provider.record.tier}}</span>
     <time v-if="provider.record?.observedAt">{{date(provider.record.observedAt)}}</time>
    </button>
   </div>
   <button class="node-exit-open node-exit-compact" type="button" @click="open=true">全部来源与时间 · {{detailCount}} 条记录</button>
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
.node-exit-summary{min-width:0;max-width:100%;font-size:13px;line-height:1.55}.node-exit-summary.compact{margin:8px 0 0!important;padding:0!important;max-width:100%!important;width:100%!important;color:var(--ink);text-align:left}.node-exit-addresses{display:grid;gap:4px;margin-bottom:10px}.node-exit-address-row{width:100%;display:flex;gap:9px;align-items:center;padding:3px 0;border:0;background:none;text-align:left;color:var(--ink);cursor:pointer}.node-exit-family{flex:none;font:600 12px/1.4 ui-monospace,monospace;color:var(--muted);width:35px}.node-exit-address-copy{flex:1;min-width:0}.node-exit-address-copy strong{display:block;font-size:12px;font-weight:550}.node-exit-address-copy small{display:block;color:var(--muted);font-size:11px;overflow-wrap:anywhere}.node-exit-compact-arrow{color:var(--blue);font-size:12px}.node-exit-provider-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:7px}.node-exit-provider{display:flex;flex-direction:column;align-items:stretch;gap:3px;min-width:0;border:1px solid var(--line,#d8deeb);border-radius:9px;padding:9px 10px;background:var(--surface-soft,#f5f7fb);text-align:left;color:var(--ink);cursor:pointer}.node-exit-provider-title{display:flex;align-items:center;justify-content:space-between;gap:5px;font-weight:650;font-size:12px}.node-exit-provider-title>span{font-size:11px;color:var(--muted)}.node-exit-compact-risk{font:650 24px/1.35 ui-sans-serif,system-ui,sans-serif;font-variant-numeric:tabular-nums;letter-spacing:-.7px}.node-exit-compact-risk small{font-size:11px;font-weight:400;color:var(--muted);letter-spacing:0}.node-exit-provider-status{min-height:32px;display:flex;align-items:center;font-size:12px;font-weight:650;color:var(--warning,#946315)}.node-exit-provider-family,.node-exit-provider-tier{font-size:10px;color:var(--muted);line-height:1.5}.node-exit-provider-caption{font-size:10px;line-height:1.55;color:var(--muted);overflow-wrap:anywhere}.node-exit-no-gauge{font-size:10px;color:var(--muted);min-height:13px}.node-exit-provider time{font-size:10px;font-variant-numeric:tabular-nums;color:var(--muted);margin-top:3px}.node-exit-provider:hover,.node-exit-provider:focus-visible{border-color:var(--blue);outline-offset:3px}.node-exit-risk-scale{display:flex;align-items:center;gap:4px;font-size:9px;line-height:1.5;color:var(--muted);margin:4px 0}.node-exit-risk-track{position:relative;display:block;flex:1;min-width:12px;height:4px;border-radius:4px;background:linear-gradient(90deg,#75a983 0%,#c5bb72 50%,#c97470 100%)}.node-exit-risk-marker{position:absolute;top:50%;left:var(--risk-position);width:7px;height:7px;transform:translate(-50%,-50%);border:1px solid var(--surface,#fff);border-radius:50%;background:var(--ink,#333);box-shadow:0 0 0 1px rgb(0 0 0 / 18%)}.node-exit-open{display:inline-block;border:0;background:none;color:var(--blue);font-size:11px;line-height:1.6;padding:7px 0 0;cursor:pointer}.node-exit-summary-records{display:grid;gap:10px}.node-exit-summary-main{display:block;font-size:13px}.node-exit-address{display:block;max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px/1.5 ui-monospace,monospace}.node-exit-risk{font-size:13px;font-weight:600}.node-exit-summary-source,.node-exit-summary-address-only{display:block;font-size:12px}.node-exit-summary-address-only,.node-exit-footer{color:var(--muted)}.node-exit-footer{display:flex;flex-wrap:wrap;gap:10px;align-items:center;font-size:12px}.node-exit-evidence{margin:16px 0;border-top:1px solid var(--line);padding-top:14px}.node-exit-evidence header{display:flex;gap:12px;flex-wrap:wrap;justify-content:space-between}.node-exit-fields{display:grid;grid-template-columns:max-content minmax(0,1fr);gap:6px 16px;margin:12px 0}.node-exit-fields>div{display:contents}.node-exit-fields dt{font-size:12px;color:var(--muted)}.node-exit-fields dd{margin:0;overflow-wrap:anywhere;font-size:13px}.node-exit-evidence pre{max-height:300px;overflow:auto;white-space:pre-wrap;font-size:11px}.node-exit-history{opacity:.85}.node-exit-history header{font-size:13px}
</style>
