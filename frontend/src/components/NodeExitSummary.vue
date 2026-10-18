<script setup lang="ts">
import {computed,ref} from 'vue'
import {date,type Attempt} from '../domain'
import {attemptObservedAt,latestServices} from '../nodeQuality'
import {serviceIsProfile} from '../sharedResults'
import {toExitRecord,completeExitFamilies,exitAddressRows,reputationRows,riskScore,formattedRisk,detailFields,compactExitStatus,reputationAddressHint} from '../exitSummaryPresentation'
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
// Both providers stay visible, including an IPPure challenge without an IP/score.
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
</script>
<template>
 <div class="node-exit-summary exit-ledger" :class="{compact}" aria-label="已保存的出口画像摘要">
  <template v-if="compact">
  <div class="exit-ledger-addresses" aria-label="IPv4 与 IPv6 分别记录">
   <div v-for="row in addresses" :key="row.family" class="exit-ledger-address" :data-family="row.family">
    <span class="exit-family">{{row.family}}</span>
    <span v-if="row.record?.confirmed&&row.record.ip" class="exit-address-result"><strong>{{row.region||'已取得地址'}}</strong><small>{{row.ipType||(row.region?'类型未返回':'地区未返回')}}</small></span>
    <span v-else class="exit-address-result"><strong class="muted">{{row.status}}</strong><small v-if="row.record">未取得本次地址</small></span>
   </div>
  </div>
  <div class="exit-ledger-providers" aria-label="不同来源的信誉结果">
   <div v-for="row in reputations" :key="row.serviceId" class="exit-provider" :data-provider="row.serviceId">
    <div class="exit-provider-heading"><strong>{{row.provider}}</strong><span class="exit-provider-family">{{reputationAddressHint(row.record,addresses)}}</span></div>
    <template v-if="riskScore(row.record)!==undefined">
     <div class="exit-provider-result node-exit-compact-risk"><strong>{{row.record!.risk}}<small> / 100</small></strong><span>风险分 · 越低越好</span></div>
     <span class="node-exit-risk-scale" role="img" :aria-label="`${row.provider} 分数 ${row.record!.risk}/100，0 到 100，越低越好`" :style="{'--risk-position':`${riskScore(row.record)}%`}"><span class="node-exit-risk-track"><span class="node-exit-risk-marker"/></span></span>
    </template>
    <div v-else class="exit-provider-result exit-provider-no-score"><strong>{{compactExitStatus(row.record)}}</strong><span>{{row.record?.confirmed&&row.record.risk?'风险分 '+row.record.risk+' · 刻度未声明':row.record?'未取得风险分':'没有保存记录'}}</span></div>
    <span v-if="row.record?.tier&&row.record.confirmed" class="exit-provider-tier">来源等级：{{row.record.tier}}</span>
   </div>
  </div>
  <div class="node-exit-footer"><time v-if="latestAt">{{date(latestAt)}} · 最近记录</time><button type="button" class="node-exit-compact node-exit-open" @click="open=true">IP 与来源详情<span aria-hidden="true"> ↗</span></button></div>
  </template>
  <template v-else>
   <div v-if="records.length" class="node-exit-summary-records">
    <article v-for="record in records" :key="record.key" class="node-exit-summary-record">
     <span v-if="record.ip" class="node-exit-summary-main">{{record.family}}<template v-if="record.region"> · {{record.region}}</template><template v-if="record.ipType"> · {{record.ipType}}</template></span>
     <code v-if="record.ip&&records.filter(r=>r.ip&&r.family===record.family).length>1" class="node-exit-address" :title="record.ip">{{record.ip}}</code>
     <div v-for="source in record.records.filter(s=>s.reputationSource)" :key="source.attempt.attempt_id" class="node-exit-summary-source"><strong v-if="source.confirmed&&source.risk" class="node-exit-risk">{{source.provider}} 风险分 {{formattedRisk(source)}}<template v-if="source.tier"> · 等级 {{source.tier}}</template></strong><span v-else :title="source.status">{{source.provider}} · {{source.status==='验证页拦截，未取得风险结果'?'验证页拦截':source.status}}</span></div>
     <small v-if="record.records.some(s=>s.exitSource&&s.confirmed)&&!record.records.some(s=>s.reputationSource&&s.confirmed)" class="node-exit-summary-address-only">已取得地址，尚无信誉结果</small>
    </article>
   </div><span v-else class="node-exit-summary-empty">出口画像：未查询</span>
   <div class="node-exit-footer"><time v-if="latestAt">{{date(latestAt)}}</time><button class="node-exit-open" type="button" @click="open=true">来源详情（{{detailCount}}）</button></div>
  </template>
 </div>
 <Modal v-if="open" title="已保存的出口画像" wide @close="open=false">
  <p class="small muted">IPv4 与 IPv6 是不同出口。Ping0、IPPure 各保留自己的分数和观测时间，不互相补分。最新查询失败时，旧成功仅列为历史。</p>
  <p v-if="!currentRecords.length" class="muted">出口画像：未查询。</p>
  <article v-for="record in currentRecords" :key="record.attempt.attempt_id" class="node-exit-evidence">
   <header><strong>{{record.provider}} · {{record.family}}</strong><span :class="record.confirmed?'good':'warn'">{{record.status}}</span></header>
   <dl class="node-exit-fields"><div v-for="item in detailFields(record)" :key="item.label"><dt>{{item.label}}</dt><dd><code v-if="item.label==='IP'">{{item.value}}</code><template v-else>{{item.value}}</template></dd></div></dl>
   <p v-if="record.attempt.result?.summary">服务摘要：{{record.attempt.result.summary}}</p>
   <p v-if="record.attempt.result?.error_message" class="warn">失败原因：{{record.attempt.result.error_message}}</p>
   <details><summary>原始服务记录</summary><pre>{{JSON.stringify(record.attempt,null,2)}}</pre></details>
  </article>
  <article v-for="record in historicalRecords" :key="`history-${record.attempt.attempt_id}`" class="node-exit-evidence node-exit-history">
   <header><strong>{{record.provider}} · {{record.family}} · 历史记录</strong><span class="warn">历史有效画像 · 不代表当前</span></header>
   <dl class="node-exit-fields"><div v-for="item in detailFields(record,true)" :key="item.label"><dt>{{item.label}}</dt><dd><code v-if="item.label==='IP'">{{item.value}}</code><template v-else>{{item.value}}</template></dd></div></dl>
   <details><summary>原始服务记录</summary><pre>{{JSON.stringify(record.attempt,null,2)}}</pre></details>
  </article>
  <p class="small muted">{{detailCount}} 条来源记录 · 仅查看已保存数据，不发起检测</p>
 </Modal>
</template>
<style scoped>
.exit-ledger{min-width:0;font-size:12px;line-height:1.45;color:var(--ink);text-align:left;font-variant-numeric:tabular-nums}.exit-ledger-addresses{display:grid;gap:6px}.exit-ledger-address{display:grid;grid-template-columns:36px minmax(0,1fr);align-items:baseline;gap:6px}.exit-family{font-size:11px;font-weight:650;color:var(--muted);letter-spacing:.02em}.exit-address-result{display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 6px;min-width:0}.exit-address-result strong{font-size:12px;font-weight:550;overflow-wrap:anywhere}.exit-address-result small{color:var(--muted);font-size:11px}.exit-ledger-providers{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px;margin-top:10px;padding-top:9px;border-top:1px solid var(--line,var(--border,#dce2e9))}.exit-provider{min-width:0}.exit-provider+.exit-provider{border-left:1px solid var(--line,var(--border,#dce2e9));padding-left:8px}.exit-provider-heading{display:flex;flex-wrap:wrap;align-items:baseline;gap:2px 5px}.exit-provider-heading>strong{font-size:12px;font-weight:650;letter-spacing:.01em}.exit-provider-family{font-size:10px;color:var(--muted)}.exit-provider-result{margin-top:4px;min-height:39px;display:flex;flex-direction:column;align-items:flex-start;gap:1px}.exit-provider-result>strong{font-size:18px;line-height:1.3;font-weight:650;color:var(--ink)}.exit-provider-result>strong small{font-size:11px;color:var(--muted);font-weight:400}.exit-provider-result>span,.exit-provider-tier{font-size:10px;line-height:1.5;color:var(--muted);overflow-wrap:anywhere}.exit-provider-no-score>strong{font-size:12px;line-height:1.6;color:var(--warning,#9b651e);font-weight:550}.exit-provider-tier{display:block}.node-exit-risk-scale{display:block;width:100%;padding:4px 3px 0 0}.node-exit-risk-track{position:relative;display:block;width:100%;height:3px;border-radius:2px;background:linear-gradient(90deg,#63a785,#c5ad67,#c87674)}.node-exit-risk-marker{position:absolute;top:50%;left:var(--risk-position);width:6px;height:6px;transform:translate(-50%,-50%);border:1px solid var(--surface,#fff);border-radius:50%;background:var(--ink,#223046)}.node-exit-footer{display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:2px 7px;margin-top:9px;font-size:10px;color:var(--muted)}.node-exit-compact.node-exit-open{display:inline-flex;align-items:center;width:auto;min-height:26px;margin:0;border:0;padding:1px 0;background:none;color:var(--blue);font-size:11px;font-weight:500;cursor:pointer}.node-exit-compact.node-exit-open:hover{text-decoration:underline}.node-exit-open:focus-visible{outline:2px solid var(--blue);outline-offset:3px;border-radius:3px}.node-exit-evidence{padding:14px 0;border-bottom:1px solid var(--line,var(--border,#ddd))}.node-exit-evidence>header{display:flex;align-items:baseline;justify-content:space-between;flex-wrap:wrap;gap:5px 12px}.node-exit-fields{display:grid;grid-template-columns:max-content minmax(0,1fr);gap:5px 14px;margin:10px 0}.node-exit-fields>div{display:contents}.node-exit-fields dt{font-size:12px;color:var(--muted)}.node-exit-fields dd{margin:0;overflow-wrap:anywhere}.node-exit-evidence pre{max-height:340px;overflow:auto;font-size:11px}.node-exit-history{border-top:1px dashed var(--line,var(--border,#ddd))}
.node-exit-summary-records{display:grid;gap:7px}.node-exit-summary-record{font-size:12px;line-height:1.5}.node-exit-summary-main,.node-exit-address{display:block}.node-exit-address{overflow-wrap:anywhere}.node-exit-summary-address-only{color:var(--muted)}.node-exit-risk{font-weight:550}.node-exit-open{border:0;background:none;padding:2px 0;color:var(--blue);cursor:pointer}
</style>
