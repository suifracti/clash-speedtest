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
 return {kind,key:valid?downloadCondition(a!)+'|'+kind:'invalid',label:valid?(kind==='partial'?'有效部分下载':'完整读取')+' · '+downloadConditionLabel(a!):'未测／无有效速度'}
}

export function serviceComparisonGroup(attempts:(Attempt|undefined)[],ids:string[]){
 const byId=new Map(attempts.filter((a):a is Attempt=>!!a).map(a=>[a.service_id,a]))
 const businessIDs=ids.filter(id=>!serviceIsProfile(byId.get(id),id))
 const complete=businessIDs.length>0&&businessIDs.every(id=>serviceAvailability(byId.get(id))!==null)
 const key=JSON.stringify(businessIDs.map(id=>{const r=byId.get(id)?.rule;return [id,r?[r.rule_version??'unknown',r.method??'unknown',r.target_url,r.success_criterion??'unknown',r.maximum_bytes??'unknown',r.maximum_duration_ns??'unknown',r.network_path_method??'unknown',r.physical_interface??'unknown',r.dns_mode??'unknown',servicePath(byId.get(id))]:null]}))
 return {complete,key}
}

export function latencyComparisonGroup(point?:TrendPoint){const valid=!!point&&point.value!==null;return {valid,key:valid?JSON.stringify([point!.source||'unknown',point!.conditionKey||'method-unrecorded']):'invalid',label:valid?point!.description:'未测／无有效响应时延'}}
function servicePath(a?:Attempt){const raw=a?.result?.details?.network_path;try{const p=typeof raw==='string'?JSON.parse(raw):raw as Record<string,unknown>|undefined;return [p?.method||'unverified',p?.interface||'unknown',p?.dns_mode||'unknown',p?.dns_bind_verified??'unknown',p?.socket_bind_verified??'unknown']}catch{return ['unverified']}}
