import {bytes,date,downloadSpeed,downloadTone,key,median,sampleFor,outcomeLabel,outcomeTone,stateLabel,type Attempt,type MeasurementRound,type NodeOption,type Project} from './domain'
import {downloadGroup,servicePassed,serviceExecuted,serviceAvailability,serviceAvailabilitySummary} from './sharedResults'
import {groupSamples,latencySampleState,type TrendPoint,type SamplePoint,type HealthTone} from './presentation'
export type Trigger='manual'|'scheduled'|'diagnostic'|'unknown'
export const triggerLabel=(trigger?:string)=>({manual:'手动',scheduled:'定时',diagnostic:'有限诊断',unknown:'来源未知'}[trigger||'unknown']||'来源未知')
export function downloadCondition(a:Attempt){return JSON.stringify([a.rule.target_url,a.rule.method||'GET',a.rule.rule_version??'unknown',a.rule.maximum_bytes??'unknown',a.rule.maximum_duration_ns??'unknown',a.rule.sample_every_bytes??'unknown',a.rule.sample_every_ns??'unknown',a.rule.network_path_method??'unverified',a.rule.physical_interface??'unknown',a.rule.dns_mode??'unknown',a.result?.network_path?.dns_bind_verified??'unknown',a.result?.network_path?.socket_bind_verified??'unknown',a.node_type??'unknown'])}
export function downloadConditionLabel(a:Attempt){return `${a.rule.target_url} · ${a.rule.method||'方法未知'} / v${a.rule.rule_version??'?'} · 上限 ${a.rule.maximum_bytes===undefined?'未知':a.rule.maximum_bytes/1048576+' MiB'} · ${a.rule.maximum_duration_ns===undefined?'时限未知':a.rule.maximum_duration_ns/1e9+' 秒'} · 出口 ${a.rule.network_path_method==='physical_socket_v1'?'物理网卡 '+a.rule.physical_interface+' / 独立 DNS':'旧出口未验证'}`}
export function attemptTone(a:Attempt):HealthTone {if(!a.result||!serviceExecuted(a)||a.result.details?.execution_status==='not_executed'||['not_executed','cancelled','interrupted'].includes(a.execution_state))return 'empty';const tone=outcomeTone(a.result.outcome);return tone==='good'&&a.persistence_state==='failed'?'warn':tone}
export function serviceConclusion(a:Attempt){
 const r=a.result;if(!r)return stateLabel(a.execution_state)
 if(a.execution_state==='not_executed'||r.details?.execution_status==='not_executed'||r.outcome==='not_executed')return '未执行'
 if(['cancelled','interrupted','not_executed'].includes(r.outcome))return outcomeLabel(r.outcome)
 if(r.outcome==='reachable')return 'HTTP 可达（业务／解锁未验证）'
 if(r.outcome==='matched')return '探针规则通过（仅此端点）'
 if(r.outcome==='unlocked')return '解锁规则通过（播放未验证）'
 if(r.outcome==='profiled')return '已获取端点画像'
 if(serviceAvailability(a)===0)return '判据未通过（'+outcomeLabel(r.outcome)+'）'
 return '未确认（'+outcomeLabel(r.outcome)+'）'
}
export function attemptStatus(a:Attempt,download=false){const r=a.result;return `${a.execution_state==='not_executed'?'未执行':r?(!download?(serviceExecuted(a)?'已执行 · ':'')+serviceConclusion(a):download&&r.http_status===429?'测速源限流（429）':download&&r.outcome==='byte_limit'?'完成·'+bytes(r.bytes_read).replace('.0 MiB',' MiB'):download&&r.outcome==='time_limit'&&r.bytes_read>0?'部分测量 · '+bytes(r.bytes_read)+' / '+((r.duration_ns||0)/1e9).toFixed(2)+' 秒 · 达到时限':outcomeLabel(r.outcome)):stateLabel(a.execution_state)}${a.persistence_state==='failed'?' · 保存失败':a.persistence_state==='saving'?' · 保存中':''}`}
export function durableRoundPoints(rounds:MeasurementRound[],project:Project,target='cloudflare',serviceId='',serviceNames:Record<string,string>={}):TrendPoint[]{
 if(project==='combined')project='latency'
 return rounds.flatMap(r=>{
  const all=r.items.filter(i=>i.project===project);if(!all.length||project==='service'&&serviceId&&!all.some(i=>i.service_id===serviceId))return []
  const attempts=all.map(i=>i.download||i.service||({...i,attempt_id:'planned:'+r.round_id+':'+i.request_id+':'+i.service_id,requested_at:r.started_at,rule:{name:serviceNames[i.service_id||'']||i.service_id,target_url:''}} as Attempt))
  const samples:SamplePoint[]=all.map((i,index)=>{const a=attempts[index],tone=project==='download'?downloadTone(a):attemptTone(a);return {id:a.attempt_id,time:a.finished_at||r.started_at,value:project==='service'?serviceAvailability(a):a.result?(tone==='good'?1:tone==='warn'?.5:0):null,tone,label:a.rule.name||i.service_id||project,description:(a.rule.name||i.service_id||project)+' · '+attemptStatus(a,project==='download')+(i.not_executed_reason?' · '+i.not_executed_reason:'')}})
  const pending=all.filter(i=>i.execution_state==='queued'||i.execution_state==='running'||i.execution_state==='cancelling').length
  const skipped=all.filter(i=>i.execution_state==='not_executed').length,savedFail=all.filter(i=>i.persistence_state==='failed').length,executed=all.filter(i=>i.execution_state!=='not_executed'&&i.service?.result?.details?.execution_status!=='not_executed'&&(i.download?.result||i.service?.result||i.latency)).length
  const actual=all.flatMap(i=>i.latency?[i.latency]:[]),chosen=serviceId?samples.filter((_,i)=>all[i].service_id===serviceId):samples
  const selectedLatencySamples=actual.flatMap(t=>sampleFor(t,target)).sort((a,b)=>Date.parse(a.timestamp)-Date.parse(b.timestamp)),successfulLatency=selectedLatencySamples.filter(s=>s.success),latencyTone:HealthTone=!selectedLatencySamples.length||selectedLatencySamples.every(s=>latencySampleState(s).tone==='empty')?'empty':!successfulLatency.length?'bad':successfulLatency.length<selectedLatencySamples.length||selectedLatencySamples.some(s=>latencySampleState(s).tone==='warn')||all.some(i=>i.persistence_state==='failed')?'warn':'good'
  const tone:HealthTone=project==='latency'?latencyTone:chosen.every(s=>s.tone==='empty')?'empty':chosen.every(s=>s.tone==='good')?'good':chosen.every(s=>s.tone==='bad')?'bad':'warn'
  const first=attempts.find(a=>a.result)
  const chosenAttempts=serviceId?attempts.filter((_,index)=>all[index].service_id===serviceId):attempts
  return [{id:r.round_id,time:project==='latency'&&target!=='all'&&selectedLatencySamples.length?selectedLatencySamples[selectedLatencySamples.length-1].timestamp:r.started_at,trigger:r.trigger_type,source:'manual',planned:all.length,attempts:project==='latency'?undefined:attempts,latencyTests:project==='latency'?actual:undefined,samples:project==='latency'?actual.flatMap(t=>t.samples.map(s=>({id:t.attempt_id+':'+s.seq,time:s.timestamp,value:s.success?s.latency_ms:null,tone:s.success?'good':'bad',label:s.target||'采样',description:s.target+' · '+(s.success?s.latency_ms+' ms':s.error)}))):samples,value:project==='download'?downloadSpeed(first):project==='latency'?median(successfulLatency.map(s=>s.latency_ms)):serviceAvailabilitySummary(chosenAttempts).rate,tone,
  sampleGroups:project==='latency'?groupSamples(actual.flatMap(t=>t.samples.map(s=>({id:t.attempt_id+':'+s.seq,target:s.target||'',time:s.timestamp,value:s.success?s.latency_ms:null,tone:s.success?'good' as const:'bad' as const,label:'采样 '+s.seq,note:s.success?s.error:undefined,error:s.success?undefined:s.error,description:(s.target||'旧目标未知')+' · '+(s.success?s.latency_ms+' ms':s.error||'失败')})))):undefined,
  conditionKey:project==='download'&&first?downloadCondition(first):undefined,
  description:`${date(r.started_at)} · ${triggerLabel(r.trigger_type)} · 轮次 ${r.round_id} · 计划 ${all.length} 项 · 实际执行 ${executed} · 未执行 ${skipped} · 排队/执行中 ${pending} · 保存失败 ${savedFail}${project==='download'&&first?' · '+attemptStatus(first,true)+' · '+downloadConditionLabel(first):''}`} as TrendPoint]
 }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
export function linkedAttemptIDs(rounds:MeasurementRound[],project:Project){return new Set(rounds.flatMap(r=>r.items.filter(i=>i.project===project).flatMap(i=>i.download?[i.download.attempt_id]:i.service?[i.service.attempt_id]:i.latency?[i.latency.attempt_id]:[])))}
