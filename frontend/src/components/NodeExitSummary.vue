<script setup lang="ts">
import {computed,ref} from 'vue'
import {date,type Attempt} from '../domain'
import {attemptObservedAt,latestServices} from '../nodeQuality'
import {serviceIsProfile} from '../sharedResults'
import Modal from './Modal.vue'

const props=withDefaults(defineProps<{attempts?:Attempt[]}>(),{attempts:()=>[]})
const open=ref(false)
type ExitRecord={attempt:Attempt;serviceId:string;family:string;region:string;ipType:string;provider:string;ip:string;confirmed:boolean;observedAt:string}
const field=(attempt:Attempt,...keys:string[])=>{
 const details=attempt.result?.details
 for(const key of keys){const value=details?.[key];if(typeof value==='string'&&value.trim())return value.trim();if(typeof value==='number'&&Number.isFinite(value))return String(value)}
 return ''
}
const booleanField=(attempt:Attempt,key:string):boolean|undefined=>{
 const value=attempt.result?.details?.[key]
 if(typeof value==='boolean')return value
 if(typeof value==='string'){const normalized=value.trim().toLowerCase();if(normalized==='true')return true;if(normalized==='false')return false}
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
const records=computed<ExitRecord[]>(()=>latestServices(props.attempts).filter(a=>serviceIsProfile(a)).map(attempt=>{
 const serviceId=attempt.service_id||attempt.rule.service_id||'未知服务',ip=field(attempt,'ip')
 const family=familyOf(attempt,serviceId,ip)
 const region=field(attempt,'country_code','country','location','city')||'地区未知'
 let ipType=field(attempt,'ip_type','origin_type')
 const isResidential=booleanField(attempt,'is_residential'),isIDC=booleanField(attempt,'is_idc')
 if(!ipType&&isResidential!==undefined)ipType=isResidential?'住宅':'非住宅'
 if(!ipType&&isIDC!==undefined)ipType=isIDC?'机房 IDC':'非 IDC（不等于家宽）'
 const provider=field(attempt,'provider')||providerNames[serviceId]||serviceId
 const confirmed=attempt.result?.outcome==='profiled'
 return {attempt,serviceId,family,region,ipType:ipType||'类型未知',provider,ip,confirmed,observedAt:attemptObservedAt(attempt)}
}))
const compactRecords=computed(()=>{
 const selected:ExitRecord[]=[]
 for(const family of ['IPv4','IPv6']){const record=records.value.find(item=>item.family===family);if(record)selected.push(record)}
 for(const record of records.value)if(selected.length<2&&!selected.includes(record))selected.push(record)
 return selected.sort((a,b)=>(Date.parse(b.observedAt)||0)-(Date.parse(a.observedAt)||0))
})
const detailValue=(attempt:Attempt,key:string)=>field(attempt,key)
const shownDetail=(attempt:Attempt,key:string)=>detailValue(attempt,key)||'未提供'
const exitObservedAt=(attempt:Attempt)=>detailValue(attempt,'exit_observed_at')||detailValue(attempt,'query_at')||'未提供'
</script>

<template>
 <div class="node-exit-summary" aria-label="已保存的出口画像摘要">
  <div v-if="compactRecords.length" class="node-exit-summary-records"><article v-for="record in compactRecords" :key="record.attempt.attempt_id" class="node-exit-summary-record"><span class="node-exit-summary-main">{{record.family}} · {{record.region}} · {{record.ipType}}</span><small class="node-exit-summary-source">{{record.provider}} · {{date(record.observedAt)}}<template v-if="!record.confirmed"> · 未确认</template></small></article></div>
  <span v-else class="node-exit-summary-empty">出口画像：未查询</span>
  <button class="node-exit-open" type="button" @click="open=true">查看全部来源（{{records.length}}）</button>
 </div>
 <Modal v-if="open" title="已保存的出口画像" wide @close="open=false">
  <p class="small muted">只显示当前节点范围传入的最近保存服务记录。每项服务按观测时间取最新一条；不同服务可能看到不同出口。</p>
  <p v-if="!records.length" class="muted">出口画像：未查询。</p>
  <article v-for="record in records" :key="record.attempt.attempt_id" class="node-exit-evidence">
   <header><strong>{{record.provider}} · {{record.family}}</strong><span :class="record.confirmed?'good':'warn'">{{record.confirmed?'已获取画像':'未确认'}}</span></header>
   <p>{{record.region}} · {{record.ipType}} · {{date(record.observedAt)}} · {{record.serviceId}}</p>
   <p v-if="record.ip">出口 IP：<code>{{record.ip}}</code></p>
   <p v-if="record.attempt.result?.summary">服务摘要：{{record.attempt.result.summary}}</p>
   <p v-if="record.attempt.result?.error_message" class="warn">失败原因：{{record.attempt.result.error_message}}</p>
   <p>观测时间：{{record.observedAt||'未提供'}} · 保存时间：{{record.attempt.finished_at||'未提供'}}</p>
   <p v-if="record.attempt.result?.details?.exit_scope||record.attempt.result?.details?.data_freshness||record.attempt.result?.details?.query_at">
   {{shownDetail(record.attempt,'data_freshness')}} · {{shownDetail(record.attempt,'exit_scope')}} · 查询 {{shownDetail(record.attempt,'query_at')}} · 出口观察 {{exitObservedAt(record.attempt)}} · {{detailValue(record.attempt,'cache_status')==='hit'?'复用本地结果缓存':'查询结果'}}
   </p>
   <details><summary>原始服务记录</summary><pre>{{JSON.stringify(record.attempt,null,2)}}</pre></details>
  </article>
 </Modal>
</template>
