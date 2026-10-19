import type {Attempt} from './domain'

// Presentation only. These functions consume saved results; they never query an IP.
export type ExitRecord={attempt:Attempt;serviceId:string;family:string;provider:string;ip:string;region:string;ipType:string;asn:string;organization:string;risk:string;tier:string;confirmed:boolean;exitSource:boolean;reputationSource:boolean;observedAt:string;status:string}
export type ExitAddress={family:string;record?:ExitRecord;region:string;ipType:string;status:string}
export type ReputationRow={serviceId:string;provider:string;record?:ExitRecord;status:string}
const providerNames:Record<string,string>={cloudflare_trace:'Cloudflare',exit_ipv4:'出口查询',exit_ipv6:'出口查询',ping0_ip_quality:'Ping0',ippure_ip_quality:'IPPure',ipok_ipv4:'IPOK',ipok_ipv6:'IPOK'}
export function exitField(a:Attempt,...keys:string[]):string {
 for(const key of keys){const value=a.result?.details?.[key];if(typeof value==='string'&&value.trim())return value.trim();if(typeof value==='number'&&Number.isFinite(value))return String(value)}
 return ''
}
function flag(a:Attempt,key:string){const v=a.result?.details?.[key];return v===true||v==='true'?true:v===false||v==='false'?false:undefined}
export function exitFamily(a:Attempt,id:string,ip:string):string {
 const f=exitField(a,'ip_family').toLowerCase()
 if(f==='4'||f==='ipv4')return 'IPv4';if(f==='6'||f==='ipv6')return 'IPv6'
 if(/(?:^|_)ipv4$/i.test(id))return 'IPv4';if(/(?:^|_)ipv6$/i.test(id))return 'IPv6'
 if(ip.includes(':'))return 'IPv6';if(/^\d{1,3}(?:\.\d{1,3}){3}$/.test(ip))return 'IPv4'
 // network_path.address_family describes the transport to the proxy, not its exit.
 return '地址族未返回'
}
export function exitIdentity(a:Attempt){return JSON.stringify([a.profile_id,a.node_identity_key||a.node_key,a.config_revision_key])}
function savedTLSFailure(a:Attempt){return a.result?.outcome==='transport_error'&&Number(exitField(a,'tls_handshake_failures'))>0&&exitField(a,'http_requests_written')==='0'}
function failureStatus(a:Attempt):string {
 const out=a.result?.outcome||''
 // Keep persisted backend conclusions intact. HTTP 429 is not a challenge score.
 if(['rate_limited','source_rate_limited'].includes(out)||a.result?.http_status===429)return '目标限流'
 if(['challenge','suspected_challenge'].includes(out)||/challenge|captcha|验证页|验证挑战/i.test([a.result?.summary,a.result?.error_message].join(' ')))return '验证页拦截，未取得风险结果'
 if(savedTLSFailure(a))return 'TLS 握手失败：'+(exitField(a,'tls_handshake_error')||'连接被中断，未发出 HTTP 请求')
 const raw=(a.result?.error_message||a.result?.summary||'未取得结果').replace(/\s+/g,' ').trim()
 return '查询未完成：'+(raw.length>120?raw.slice(0,117)+'…':raw)
}
export function compactExitStatus(r?:ExitRecord):string {
 if(!r)return '未查询'
 if(r.confirmed)return r.risk?'已取得风险结果':'未返回风险分'
 if(r.attempt.persistence_state==='failed')return '保存失败'
 const out=r.attempt.result?.outcome||''
 if(out==='rate_limited'||out==='source_rate_limited'||r.attempt.result?.http_status===429)return '目标限流'
 if(r.status.includes('验证页'))return out==='suspected_challenge'?'疑似验证页':'验证页拦截'
 if(r.attempt.execution_state==='not_executed')return '未执行'
 if(['queued','running','cancelling','saving'].includes(r.attempt.execution_state))return ({queued:'排队中',running:'查询中',cancelling:'正在取消',saving:'保存中'} as Record<string,string>)[r.attempt.execution_state]
 if(/cancel|interrupt/.test(out+r.attempt.execution_state))return '已取消或中断'
 if(out==='timed_out'||/timeout|deadline|超时/i.test(r.attempt.result?.error_message||''))return '查询超时'
 if(savedTLSFailure(r.attempt))return 'TLS 连接失败'
 return out==='http_rejected'?'HTTP 拒绝':'未取得结果'
}
export function toExitRecord(a:Attempt,observedAt:string):ExitRecord {
 const id=a.service_id||a.rule.service_id||'未知服务',ip=exitField(a,'ip'),kind=(a.rule as Attempt['rule']&{result_kind?:string}).result_kind
 const reputationSource=['ping0_ip_quality','ippure_ip_quality','ipok_ipv4','ipok_ipv6'].includes(id)||kind==='ip_quality'
 const exitSource=['cloudflare_trace','exit_ipv4','exit_ipv6'].includes(id)||kind==='exit_profile'
 const confirmed=a.result?.outcome==='profiled'&&a.persistence_state!=='failed'&&!['queued','running','cancelling','saving','cancelled','interrupted','not_executed'].includes(a.execution_state)
 let ipType=exitField(a,'ip_type','origin_type');const residential=flag(a,'is_residential'),idc=flag(a,'is_idc')
 if(!ipType&&residential!==undefined)ipType=residential?'住宅':'非住宅'
 if(!ipType&&idc!==undefined)ipType=idc?'机房 IDC':'非 IDC（不等于家宽）'
 const risk=exitField(a,'risk_score','fraud_score').replace(/^(未提供|未知|null|undefined)$/i,'')
 return {attempt:a,serviceId:id,family:exitFamily(a,id,ip),provider:providerNames[id]||exitField(a,'provider')||id,ip,region:exitField(a,'country_code','country','location','city'),ipType,asn:exitField(a,'asn'),organization:exitField(a,'organization','org','isp'),risk,tier:exitField(a,'risk_tier','risk_level','tier'),confirmed,exitSource,reputationSource,observedAt,status:confirmed?(reputationSource?(risk?'已取得风险结果':'未返回风险分'):'已取得地址'):failureStatus(a)}
}
export function completeExitFamilies(records:ExitRecord[]):ExitRecord[] {
 return records.map(r=>{
  if(!r.ip||r.family!=='地址族未返回')return r
  const families=[...new Set(records.filter(other=>other.confirmed&&other.ip===r.ip&&exitIdentity(other.attempt)===exitIdentity(r.attempt)&&['IPv4','IPv6'].includes(other.family)).map(other=>other.family))]
  return families.length===1?{...r,family:families[0]}:r
 })
}
const time=(r:ExitRecord)=>Date.parse(r.observedAt)||0
export function exitAddressRows(records:ExitRecord[]):ExitAddress[] {
 return ['IPv4','IPv6'].map(family=>{
  const candidates=records.filter(r=>r.family===family).sort((a,b)=>time(b)-time(a))
  // An explicit latest address-query failure must not disappear behind a profile.
  const record=candidates.find(r=>r.serviceId===(family==='IPv4'?'exit_ipv4':'exit_ipv6'))||candidates.find(r=>r.exitSource)||candidates.find(r=>r.confirmed&&r.ip)||candidates[0]
  if(!record)return {family,region:'',ipType:'',status:'未查询'}
  if(!record.confirmed||!record.ip)return {family,record,region:'',ipType:'',status:record.confirmed?'未返回地址':compactExitStatus(record)}
  const same=records.filter(r=>r.confirmed&&r.ip===record.ip&&r.family===family&&exitIdentity(r.attempt)===exitIdentity(record.attempt)).sort((a,b)=>time(b)-time(a))
  return {family,record,region:same.find(r=>r.region)?.region||'',ipType:same.find(r=>r.ipType)?.ipType||'',status:'已取得地址'}
 })
}
export function reputationRows(records:ExitRecord[]):ReputationRow[] {
 const ids=['ping0_ip_quality','ippure_ip_quality',...new Set(records.filter(r=>r.reputationSource&&!['ping0_ip_quality','ippure_ip_quality'].includes(r.serviceId)).map(r=>r.serviceId))]
 return ids.map(serviceId=>{const record=records.find(r=>r.serviceId===serviceId);return {serviceId,provider:providerNames[serviceId]||record?.provider||serviceId,record,status:record?.status||'未查询'}})
}
export function riskScore(r?:ExitRecord):number|undefined {
 if(!r?.confirmed)return undefined
 const scale=['ping0_ip_quality','ipok_ipv4','ipok_ipv6'].includes(r.serviceId)||r.serviceId==='ippure_ip_quality'&&exitField(r.attempt,'risk_scale_max')==='100'&&exitField(r.attempt,'risk_direction')==='lower_is_better'
 if(!scale||!/^\d+(?:\.\d+)?$/.test(r.risk))return undefined
 const n=Number(r.risk);return Number.isFinite(n)&&n>=0&&n<=100?n:undefined
}
export function formattedRisk(r:ExitRecord){return riskScore(r)===undefined?r.risk:`${r.risk}/100（越低越好）`}
export function detailFields(r:ExitRecord,historical=false){
 const queryAt=exitField(r.attempt,'query_at'),cache=exitField(r.attempt,'cache_status')
 return [
  {label:'IP',value:r.ip},{label:'地区',value:r.region},{label:'ASN',value:r.asn},{label:'运营商',value:r.organization},{label:'类型',value:r.ipType},{label:'风险',value:r.confirmed?formattedRisk(r):''},{label:'风险等级',value:r.confirmed?r.tier:''},
  {label:'状态',value:historical?'历史有效画像，不代表当前':r.status},{label:'观测时间',value:r.observedAt},{label:'查询时间',value:queryAt},{label:'出口观察时间',value:exitField(r.attempt,'exit_observed_at')||queryAt},{label:'数据新鲜度',value:exitField(r.attempt,'data_freshness')},{label:'出口范围',value:exitField(r.attempt,'exit_scope')},{label:'缓存',value:cache==='hit'?'复用本地结果缓存':cache?`缓存状态：${cache}`:''}
 ].filter(row=>!!row.value)
}

export function reputationAddressHint(r:ExitRecord|undefined,addresses:ExitAddress[]):string {
 if(!r?.ip)return '未返回 IP'
 const address=addresses.find(a=>a.family===r.family)?.record
 if(address?.confirmed&&address.ip&&address.ip!==r.ip)return r.family+' · 不同地址'
 return r.family
}
