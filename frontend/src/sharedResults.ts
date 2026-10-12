import type {TrendPoint} from './presentation'
import {downloadSpeed,type Attempt} from './domain'
import {downloadCondition,downloadConditionLabel} from './measurementRounds'
export function serviceExecuted(a?:Attempt){return !!a?.result&&!['not_executed','cancelled','interrupted'].includes(a.execution_state)&&a.result.details?.execution_status!=='not_executed'&&!['credentials_required','not_executed','cancelled','user_cancelled','interrupted'].includes(a.result.outcome)}
// Profiles describe an exit, never business availability. Exact IDs cover old snapshots.
export function serviceIsProfile(a?:Attempt,id=a?.service_id||a?.rule.service_id){
 const kind=(a?.rule as (Attempt['rule']&{result_kind?:string})|undefined)?.result_kind
 return ['exit_profile','ip_quality'].includes(kind||'')||['cloudflare_trace','exit_ipv4','exit_ipv6','ping0_ip_quality','ippure_ip_quality','ipok_ipv4','ipok_ipv6'].includes(id||'')||a?.result?.outcome==='profiled'
}
export function servicePassed(a?:Attempt){return serviceExecuted(a)&&!serviceIsProfile(a)&&['matched','unlocked'].includes(a!.result!.outcome)}
// 0 requires observed rejection/criterion evidence; unknown/transport/quota is not 0.
export function serviceAvailability(a?:Attempt):0|1|null {
 if(!serviceExecuted(a)||serviceIsProfile(a))return null
 const r=a!.result!
 if(servicePassed(a))return 1
 if(['region_blocked','region_limited','originals_only','service_rejected','challenge'].includes(r.outcome))return 0
 if(r.outcome==='http_rejected'&&r.http_status!==undefined&&r.http_status>=400&&r.http_status<500&&![401,429].includes(r.http_status))return 0
 if(r.outcome==='criteria_mismatch'&&!['response_limit','response_evidence_limit','json_document','response_body'].includes(r.failure_phase||'')&&r.http_status!==undefined)return 0
 return null
}
export function serviceAvailabilitySummary(attempts:(Attempt|undefined)[]){
 const values=attempts.map(serviceAvailability).filter((v):v is 0|1=>v!==null),passed=values.filter(v=>v===1).length
 return {passed,confirmed:values.length,profiles:attempts.filter(a=>serviceIsProfile(a)).length,unknown:attempts.filter(a=>!serviceIsProfile(a)&&serviceAvailability(a)===null).length,rate:values.length?passed/values.length:null}
}
export function downloadGroup(a?:Attempt){
 const valid=!!a?.result&&downloadSpeed(a)!==null&&a.result.http_status!==429&&!['not_executed','cancelled','interrupted'].includes(a.execution_state)
 const kind=!valid?'invalid':a!.result!.outcome==='time_limit'?'partial':'full'
 const path=a?.result?.network_path,comparable=valid&&!!a!.rule.method&&a!.rule.rule_version!==undefined&&a!.rule.maximum_bytes!==undefined&&a!.rule.maximum_duration_ns!==undefined&&!!a!.rule.network_path_method&&!!a!.rule.physical_interface&&!!a!.rule.dns_mode&&!!a!.node_type&&path?.method===a!.rule.network_path_method&&path.interface===a!.rule.physical_interface&&path.dns_mode===a!.rule.dns_mode&&path.dns_bind_verified===true&&path.socket_bind_verified===true&&!!path.address_family&&path.address_family!=='unknown'&&!!path.address_source&&path.address_source!=='unknown'
 return {kind,key:valid?downloadCondition(a!)+'|'+kind:'invalid',comparable,label:valid?(kind==='partial'?'有效部分下载':'完整读取')+' · '+downloadConditionLabel(a!)+(comparable?'':' · 出口条件证据不足'):'未测／无有效速度'}
}

export function serviceComparisonGroup(attempts:(Attempt|undefined)[],ids:string[]){
 const byId=new Map(attempts.filter((a):a is Attempt=>!!a).map(a=>[a.service_id,a]))
 const businessIDs=ids.filter(id=>!serviceIsProfile(byId.get(id),id))
 const complete=businessIDs.length>0&&businessIDs.every(id=>serviceAvailability(byId.get(id))!==null)
 const key=JSON.stringify(businessIDs.map(id=>{const r=byId.get(id)?.rule;return [id,r?[r.rule_version??'unknown',r.method??'unknown',r.target_url,r.success_criterion??'unknown',r.maximum_bytes??'unknown',r.maximum_duration_ns??'unknown',r.network_path_method??'unknown',r.physical_interface??'unknown',r.dns_mode??'unknown',servicePath(byId.get(id))]:null]}))
 return {complete,key}
}

export function latencyComparisonGroup(point?:TrendPoint){
 const tests=point?.latencyTests||[],conditions=tests.map(t=>{const p=t.network_path;return [t.method||'unknown',t.method_version??'unknown',t.target||'legacy',p?.method||'unverified',p?.interface||'unknown',p?.dns_mode||'unknown',p?.dns_bind_verified??'unknown',p?.socket_bind_verified??'unknown',p?.address_family||'unknown',p?.address_source||'unknown']})
 const known=(value:unknown)=>typeof value==='string'&&!!value&&value!=='unknown'&&value!=='unverified'
 const comparable=tests.length>0&&tests.every(t=>{const p=t.network_path;return known(t.method)&&t.method_version!==undefined&&known(p?.method)&&known(p?.interface)&&known(p?.dns_mode)&&p?.dns_bind_verified===true&&p?.socket_bind_verified===true&&known(p?.address_family)&&known(p?.address_source)})&&new Set(conditions.map(condition=>JSON.stringify(condition))).size===1
 const valid=!!point&&typeof point.value==='number'&&Number.isFinite(point.value)&&comparable
 const label=valid?point!.description:point&&typeof point.value==='number'&&Number.isFinite(point.value)?'有时延值 · 条件证据不足，不参与数值排名':'未测／无有效响应时延'
 return {valid,key:valid?JSON.stringify([point!.source||'unknown',conditions[0]]):'invalid',label}
}
function servicePath(a?:Attempt){const raw=a?.result?.network_path??a?.result?.details?.network_path;try{const p=typeof raw==='string'?JSON.parse(raw):raw as Record<string,unknown>|undefined;return [p?.method||'unverified',p?.interface||'unknown',p?.dns_mode||'unknown',p?.dns_bind_verified??'unknown',p?.socket_bind_verified??'unknown',p?.address_family||'unknown',p?.address_source||'unknown']}catch{return ['unverified','unknown','unknown','unknown','unknown','unknown','unknown']}}
