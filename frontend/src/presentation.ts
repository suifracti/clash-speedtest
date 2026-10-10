import {latencyTargetId} from './latencyTargets'
import {downloadGroup,serviceAvailability,serviceAvailabilitySummary} from './sharedResults'
import {durableRoundPoints,linkedAttemptIDs,triggerLabel,downloadCondition,downloadConditionLabel,attemptTone,attemptStatus} from './measurementRounds'
import {date,downloadSpeed,downloadTone,key,latencyFor,median,outcomeLabel,outcomeTone,sampleFor,targets,type Attempt,type LatencyTest,type MonitorSample,type NodeOption,type Project} from './domain'
import type {MeasurementRound} from './domain'
import {computed,type ComputedRef,type Ref} from 'vue'
import type {Workspace} from './workspace'
export type HealthTone='good'|'warn'|'bad'|'empty'
export interface SamplePoint {id:string;time:string;value:number|null;tone:HealthTone;description:string;label:string;error?:string;note?:string;executionState?:'cancelled'}
export interface SampleGroup {id:string;name:string;target:string;value:number|null;samples:SamplePoint[]}
export interface TrendPoint {id:string;time:string;value:number|null;tone:HealthTone;description:string;samples?:SamplePoint[];sampleGroups?:SampleGroup[];attempts?:Attempt[];trigger?:'manual'|'scheduled'|'diagnostic'|'unknown';planned?:number;conditionKey?:string;source?:'manual'|'monitor';monitorSamples?:MonitorSample[];latencyTests?:LatencyTest[]}
export function latencySampleState(sample:LatencyTest['samples'][number]):{tone:HealthTone;executionState?:'cancelled'} {
 if(!sample.success&&sample.error?.includes('context canceled'))return {tone:'empty',executionState:'cancelled'}
 return {tone:sample.success?(sample.error?'warn':'good'):'bad'}
}
export function latencyPointLabel(point?:TrendPoint,target='all') {
 if(point?.value!=null)return Math.round(point.value)+' ms'
 const samples=point?.sampleGroups?.filter(g=>target==='all'||latencyTargetId(g.target)===target).flatMap(g=>g.samples)||point?.samples||[]
 if(samples.length&&samples.every(s=>s.executionState==='cancelled'))return '已取消'
 return point?.tone==='bad'?'失败':'未测'
}
export interface NodeSeries {id:string;name:string;airport:string;points:TrendPoint[]}
export function groupSamples(samples:(SamplePoint&{target:string})[]):SampleGroup[]{
  const groups=new Map<string,SampleGroup>()
  for(const sample of samples){
    const target=sample.target||'旧版 Cloudflare',known=targets.find(t=>latencyTargetId(sample.target)===t.id),name=known?.name||'旧/未知目标 · '+target
    let group=groups.get(target)
    if(!group){group={id:target,name,target,value:null,samples:[]};groups.set(target,group)}
    group.samples.push({...sample,label:'第 '+(group.samples.length+1)+' 次'})
  }
  return Array.from(groups.values()).map(group=>({...group,value:median(group.samples.flatMap(s=>s.value===null?[]:[s.value]))}))
}
export function manualLatencyPoint(t:LatencyTest,target='cloudflare'):TrendPoint {
    const samples=sampleFor(t,target).slice().sort((a,b)=>a.seq-b.seq),count=samples.filter(s=>s.success).length
    const family=t.network_path?.address_family==='ipv4'||t.network_path?.address_family==='ipv6'?t.network_path.address_family:`unknown:${t.attempt_id}`,source=t.network_path?.address_source||`unknown:${t.attempt_id}`
    return {id:t.attempt_id,time:t.finished_at,value:latencyFor(t,target),tone:!samples.length||samples.every(s=>latencySampleState(s).tone==='empty')?'empty':!count?'bad':count<samples.length?'warn':'good',source:'manual',trigger:t.source?.startsWith('workbench_')?'manual':'unknown',latencyTests:[t],conditionKey:JSON.stringify([t.method||'unknown',t.method_version||'unknown',t.target||'legacy',t.network_path?.method||'unverified',t.network_path?.interface||'unknown',family,source]),
      description:date(t.finished_at)+' · '+(t.source||'来源未知')+' · '+(t.method||'方法未知')+' / v'+(t.method_version||'?')+' · '+(t.network_path?.interface||'出口未记录')+' · '+(t.network_path?.address_family||'地址族未知')+'/'+(t.network_path?.address_source||'来源未知')+' · '+(t.status==='not_executed'?'未执行：'+(t.error_message||t.network_path?.failure_reason||'预检未通过')+' · ':'')+count+'/'+samples.length+' 样本成功',
      samples:samples.map<SamplePoint>((s,i)=>({id:t.attempt_id+':'+s.seq,time:s.timestamp,value:s.success?s.latency_ms:null,...latencySampleState(s),label:'第 '+(i+1)+' 次',description:'第 '+(i+1)+' 次采样 · '+date(s.timestamp)+' · '+(s.success?s.latency_ms+' ms':s.error||'失败')})),
      sampleGroups:groupSamples((t.samples||[]).slice().sort((a,b)=>a.seq-b.seq).map((s,i)=>({id:t.attempt_id+':'+s.seq+':'+(s.target||''),target:s.target||'',time:s.timestamp,value:s.success?s.latency_ms:null,...latencySampleState(s),label:'第 '+(i+1)+' 次',note:s.success?s.error:undefined,error:s.success?undefined:s.error||'失败',description:date(s.timestamp)+' · '+(s.success?s.latency_ms+' ms':s.error||'失败')})))}
}
type RoundHistory={measurementRounds?:Ref<Record<string,MeasurementRound[]>>;downloads?:Ref<Record<string,Attempt[]>>;services:Ref<Record<string,Attempt[]>>;latency?:Ref<Record<string,LatencyTest[]>>}
function currentRoundResults(w:RoundHistory,n:NodeOption,project:Project){
  const k=key(n),rounds=w.measurementRounds?.value[k]||[]
  if(!rounds.length)return rounds
  const records=project==='download'?w.downloads?.value[k]:project==='service'?w.services.value[k]:w.latency?.value[k]
  const current=new Map<string,Attempt|LatencyTest>((records||[]).map(a=>[a.attempt_id,a]))
  return rounds.map(round=>{
    let changed=false
    const items=round.items.map(item=>{
      if(item.project!==project)return item
      const embedded=item.download||item.service||item.latency,fresh=embedded&&current.get(embedded.attempt_id)
      if(!fresh||fresh===embedded)return item
      changed=true
      return {...item,[item.project]:fresh,execution_state:item.project==='latency'?(fresh as LatencyTest).status:(fresh as Attempt).execution_state,persistence_state:fresh.persistence_state}
    })
    return changed?{...round,items}:round
  })
}
export function points(w:Pick<Workspace,'displayTarget'|'serviceIds'|'latency'|'downloads'|'services'>&{measurementRounds?:Ref<Record<string,MeasurementRound[]>>;catalog?:Workspace['catalog']},n:NodeOption,project:Project,target=w.displayTarget.value,serviceId=w.serviceIds.value[0]):TrendPoint[]{
  const k=key(n),rounds=currentRoundResults(w,n,project==='combined'?'latency':project),linked=linkedAttemptIDs(rounds,project==='combined'?'latency':project),durable=durableRoundPoints(rounds,project,target,serviceId,Object.fromEntries((w.catalog?.value||[]).map(rule=>[rule.service_id,rule.name])))
  if(project==='latency'||project==='combined')return (w.latency.value[k]||[]).filter(t=>!linked.has(t.attempt_id)).slice().reverse().map(t=>manualLatencyPoint(t,target)).concat(durable).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
  if(project==='download')return (w.downloads.value[k]||[]).filter(a=>!linked.has(a.attempt_id)).slice().reverse().map<TrendPoint>(a=>({id:a.attempt_id,time:a.finished_at||a.requested_at,value:downloadSpeed(a),tone:downloadTone(a),attempts:[a],source:'manual',trigger:(a.trigger_type||'unknown') as TrendPoint['trigger'],conditionKey:downloadGroup(a).key,description:date(a.finished_at||a.requested_at)+' · '+triggerLabel(a.trigger_type)+' · 轮次 '+(a.round_id||'关联未知')+' · '+attemptStatus(a,true)+' · '+downloadConditionLabel(a)})).concat(durable).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
  return (w.services.value[k]||[]).filter(a=>a.service_id===serviceId&&!linked.has(a.attempt_id)).slice().reverse().map<TrendPoint>(a=>({id:a.attempt_id,time:a.finished_at||a.requested_at,value:serviceAvailability(a),tone:attemptTone(a),attempts:[a],source:'manual',trigger:(a.trigger_type||'unknown') as TrendPoint['trigger'],description:date(a.finished_at||a.requested_at)+' · '+triggerLabel(a.trigger_type)+' · 轮次 '+(a.round_id||'关联未知')+' · '+attemptStatus(a,false)})).concat(durable).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
export function serviceHealthPoints(w:Pick<Workspace,'services'|'serviceIds'>&{measurementRounds?:Ref<Record<string,MeasurementRound[]>>;catalog?:Workspace['catalog']},n:NodeOption,ids=w.serviceIds.value):TrendPoint[]{
  const rounds=currentRoundResults(w,n,'service'),linked=linkedAttemptIDs(rounds,'service'),durable=durableRoundPoints(rounds,'service','cloudflare','',Object.fromEntries((w.catalog?.value||[]).map(rule=>[rule.service_id,rule.name])))
  const groups=new Map<string,Attempt[]>()
  for(const a of w.services.value[key(n)]||[]){if(!ids.includes(a.service_id||'')||linked.has(a.attempt_id))continue;const id=a.request_id.match(/^round:([^:]+):/)?.[1]||a.attempt_id,list=groups.get(id)||[];list.push(a);groups.set(id,list)}
  return Array.from(groups,([id,attempts]):TrendPoint=>{
    const ordered=attempts.slice().sort((a,b)=>Date.parse(a.requested_at)-Date.parse(b.requested_at)),samples=ordered.map<SamplePoint>(a=>({id:a.attempt_id,time:a.finished_at||a.requested_at,value:serviceAvailability(a),tone:attemptTone(a),label:a.rule.name||a.service_id||'服务',description:(a.rule.name||a.service_id)+' · '+(a.result?outcomeLabel(a.result.outcome):'未完成')})),summary=serviceAvailabilitySummary(ordered),time=ordered[ordered.length-1].finished_at||ordered[ordered.length-1].requested_at
    return {id,time,value:summary.rate,tone:samples.every(s=>s.tone==='empty')?'empty':samples.every(s=>s.tone==='good')?'good':samples.every(s=>s.tone==='bad')?'bad':'warn',source:'manual',trigger:ordered[0].request_id.startsWith('round:')?'manual':'unknown',samples,attempts:ordered,description:date(time)+' · '+(ordered[0].request_id.startsWith('round:')?'手动（旧版显式轮次）':'来源未知 · 轮次关联未知')+' · '+summary.passed+'/'+summary.confirmed+' 项判据通过 · '+summary.unknown+' 项未确认 · '+summary.profiles+' 项画像（不计通过率）'}
  }).concat(durable).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
export function monitorRoundPoints(samples:MonitorSample[]):TrendPoint[]{
  const rounds=new Map<string,MonitorSample[]>()
  for(const sample of samples){if(sample.probe_type!=='rtt')continue;const id=sample.run_id||sample.sample_id;const group=rounds.get(id)||[];group.push(sample);rounds.set(id,group)}
  return Array.from(rounds,([id,group]):TrendPoint=>{
    const ordered=group.slice().sort((a,b)=>Date.parse(a.timestamp)-Date.parse(b.timestamp)||a.sample_id.localeCompare(b.sample_id)),successful=ordered.filter(s=>s.success)
    return {id,time:ordered[ordered.length-1].timestamp,value:median(successful.map(s=>s.latency/1e6)),tone:ordered.every(s=>s.error_class==='not_executed')?'empty':!successful.length?'bad':successful.length<ordered.length||ordered.some(s=>s.success&&s.error_detail)?'warn':'good',conditionKey:JSON.stringify(['monitor-rtt-method-unrecorded',...new Set(ordered.map(s=>s.target))]),source:'monitor',trigger:ordered[0].trigger_type==='scheduled'?'scheduled':ordered[0].trigger_type==='manual'?'manual':'unknown',
      description:date(ordered[ordered.length-1].timestamp)+' · '+triggerLabel(ordered[0].trigger_type)+' · 持续监测 RTT · 方法/出口未记录 · 目标 '+[...new Set(ordered.map(s=>s.target))].join('、')+' · 已读 '+successful.length+'/'+ordered.length+' 样本成功',
      samples:ordered.map<SamplePoint>((s,i)=>({id:s.sample_id,time:s.timestamp,value:s.success?s.latency/1e6:null,tone:s.error_class==='not_executed'?'empty':s.success?(s.error_detail?'warn':'good'):'bad',label:'第 '+(i+1)+' 次',description:'第 '+(i+1)+' 次采样 · '+date(s.timestamp)+' · '+(s.success?(s.latency/1e6).toFixed(0)+' ms':s.error_detail||s.error_class||'失败')})),
      sampleGroups:groupSamples(ordered.map((s,i)=>({id:s.sample_id,target:s.target,time:s.timestamp,value:s.success?s.latency/1e6:null,tone:s.error_class==='not_executed'?'empty':s.success?(s.error_detail?'warn':'good'):'bad',label:'第 '+(i+1)+' 次',note:s.success?s.error_detail:undefined,error:s.success?undefined:s.error_detail||s.error_class||'失败',description:date(s.timestamp)+' · '+(s.success?(s.latency/1e6).toFixed(0)+' ms':s.error_detail||s.error_class||'失败')})))}
  }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
export function monitorLatencyPoints(samples:MonitorSample[],target='all'):TrendPoint[]{
 const rounds=monitorRoundPoints(samples)
 if(target==='all')return rounds
 const match=targets.find(t=>t.id===target)
 return rounds.flatMap(point=>{
  const groups=point.sampleGroups?.filter(g=>latencyTargetId(g.target)===target)||[]
  if(!groups.length)return []
  const selected=groups.flatMap(g=>g.samples),values=selected.flatMap(s=>s.value===null?[]:[s.value])
  const tone:HealthTone=selected.every(s=>s.tone==='empty')?'empty':selected.every(s=>s.tone==='good')?'good':values.length?'warn':'bad'
  return [{...point,value:median(values),tone,description:point.description+' · 所选站点 '+(match?.name||target)}]
 })
}
export function recentService(w:Workspace,n:NodeOption,serviceId:string):Attempt|undefined{return w.latestService(n,serviceId)}
export function serviceSummary(w:Workspace,n:NodeOption,ids=w.serviceIds.value){const latest=ids.map(id=>recentService(w,n,id));const summary=serviceAvailabilitySummary(latest);return {passed:summary.passed,measured:summary.confirmed,total:summary.confirmed}}

// These monitor probes use HTTP 2xx/3xx, not the workbench catalog's stricter rules.
export const monitorHTTPServices=[
  {id:'monitor_google_http',name:'Google 基础 HTTP',probeType:'service_google',target:'https://www.google.com/generate_204'},
  {id:'monitor_github_http',name:'GitHub 基础 HTTP',probeType:'service_github',target:'https://api.github.com'},
] as const
export function monitorHTTPPoints(samples:MonitorSample[],serviceId=''):TrendPoint[]{
  const selected=monitorHTTPServices.filter(p=>!serviceId||p.id===serviceId),groups=new Map<string,MonitorSample[]>()
  for(const sample of samples){if(!selected.some(p=>p.probeType===sample.probe_type&&p.target===sample.target))continue;const id=sample.run_id||sample.sample_id;groups.set(id,[...(groups.get(id)||[]),sample])}
  return Array.from(groups,([id,items]):TrendPoint=>{
    const ordered=items.slice().sort((a,b)=>Date.parse(a.timestamp)-Date.parse(b.timestamp)),good=ordered.filter(s=>s.success).length,time=ordered[ordered.length-1].timestamp
    return {id,time,value:good/ordered.length,tone:good===ordered.length?'good':good?'warn':'bad',conditionKey:JSON.stringify(['monitor-rtt-method-unrecorded',...new Set(ordered.map(s=>s.target))]),source:'monitor',trigger:ordered[0].trigger_type==='scheduled'?'scheduled':ordered[0].trigger_type==='manual'?'manual':'unknown',monitorSamples:ordered,
      description:date(time)+' · '+triggerLabel(ordered[0].trigger_type)+' · 持续监测 · 基础 HTTP · '+good+'/'+ordered.length+' 项成功',
      samples:ordered.map(s=>({id:s.sample_id,time:s.timestamp,value:s.success?1:0,tone:s.success?'good':'bad',label:monitorHTTPServices.find(p=>p.probeType===s.probe_type)?.name||s.probe_type,description:(monitorHTTPServices.find(p=>p.probeType===s.probe_type)?.name||s.probe_type)+' · '+(s.success?'HTTP 可达':s.error_detail||s.error_class||'失败')}))}
  }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}

// Owned by the view: cached projections keep stable props across hover renders.
// Computed dependencies include nested outcomes and replacement history arrays;
// retain/clear and the per-node LRU bound release projections after navigation.
export function createPresentationCache(w:Parameters<typeof points>[0],monitor:Ref<Record<string,MonitorSample[]>>,http:Ref<Record<string,MonitorSample[]>>){
  const nodes=new Map<string,Map<string,ComputedRef<TrendPoint[]>>>()
  const queryLimit=32
  function read(n:NodeOption,query:string,derive:()=>TrendPoint[]){
    const id=key(n);let queries=nodes.get(id)
    if(!queries){queries=new Map();nodes.set(id,queries)}
    let entry=queries.get(query)
    if(!entry)entry=computed(derive)
    else queries.delete(query)
    queries.set(query,entry)
    if(queries.size>queryLimit)queries.delete(queries.keys().next().value!)
    return entry.value
  }
  function cachedPoints(n:NodeOption,project:Project,target=w.displayTarget.value,serviceId=w.serviceIds.value[0]){return read(n,JSON.stringify(['points',project,target,serviceId]),()=>points(w,n,project,target,serviceId))}
  function cachedMonitor(n:NodeOption,target='all'){return read(n,JSON.stringify(['monitor',target]),()=>monitorLatencyPoints(monitor.value[key(n)]||[],target))}
  return {
    points:cachedPoints,
    monitor:cachedMonitor,
    http:(n:NodeOption,id='')=>read(n,JSON.stringify(['http',id]),()=>monitorHTTPPoints(http.value[key(n)]||[],id)),
    site:(n:NodeOption,target:string)=>read(n,JSON.stringify(['site',target]),()=>[...cachedPoints(n,'latency',target),...cachedMonitor(n,target)].sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))),
    services:(n:NodeOption,ids=w.serviceIds.value)=>{const selected=ids.slice();return read(n,JSON.stringify(['services',selected]),()=>serviceHealthPoints(w,n,selected))},
    retain:(list:NodeOption[])=>{const keep=new Set(list.map(key));for(const id of nodes.keys())if(!keep.has(id))nodes.delete(id)},
    clear:()=>nodes.clear(),
  }
}
