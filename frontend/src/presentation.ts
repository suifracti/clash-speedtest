import {date,downloadSpeed,downloadTone,key,latencyFor,median,outcomeLabel,outcomeTone,sampleFor,targets,type Attempt,type MonitorSample,type NodeOption,type Project} from './domain'
import type {Workspace} from './workspace'
export type HealthTone='good'|'warn'|'bad'|'empty'
export interface SamplePoint {id:string;time:string;value:number|null;tone:HealthTone;description:string;label:string;error?:string}
export interface SampleGroup {id:string;name:string;target:string;value:number|null;samples:SamplePoint[]}
export interface TrendPoint {id:string;time:string;value:number|null;tone:HealthTone;description:string;samples?:SamplePoint[];sampleGroups?:SampleGroup[];attempts?:Attempt[];source?:'manual'|'monitor';monitorSamples?:MonitorSample[]}
export interface NodeSeries {id:string;name:string;airport:string;points:TrendPoint[]}
function groupSamples(samples:(SamplePoint&{target:string})[]):SampleGroup[]{
  const groups=new Map<string,SampleGroup>()
  for(const sample of samples){
    const target=sample.target||'旧版 Cloudflare',known=targets.find(t=>sample.target.includes(t.match)),name=known?.name||(sample.target.includes('.cloudflare.com')?'Cloudflare':target)
    let group=groups.get(target)
    if(!group){group={id:target,name,target,value:null,samples:[]};groups.set(target,group)}
    group.samples.push({...sample,label:'第 '+(group.samples.length+1)+' 次'})
  }
  return Array.from(groups.values()).map(group=>({...group,value:median(group.samples.flatMap(s=>s.value===null?[]:[s.value]))}))
}
export function points(w:Pick<Workspace,'displayTarget'|'serviceIds'|'latency'|'downloads'|'services'>,n:NodeOption,project:Project,target=w.displayTarget.value,serviceId=w.serviceIds.value[0]):TrendPoint[]{
  const k=key(n)
  if(project==='latency'||project==='combined')return (w.latency.value[k]||[]).slice().reverse().map((t):TrendPoint=>{
    const samples=sampleFor(t,target).slice().sort((a,b)=>a.seq-b.seq),count=samples.filter(s=>s.success).length
    return {id:t.attempt_id,time:t.finished_at,value:latencyFor(t,target),tone:!samples.length?'empty':!count?'bad':count<samples.length?'warn':'good',source:'manual',
      description:date(t.finished_at)+' · '+count+'/'+samples.length+' 样本成功',
      samples:samples.map<SamplePoint>((s,i)=>({id:t.attempt_id+':'+s.seq,time:s.timestamp,value:s.success?s.latency_ms:null,tone:s.success?'good':'bad',label:'第 '+(i+1)+' 次',description:'第 '+(i+1)+' 次采样 · '+date(s.timestamp)+' · '+(s.success?s.latency_ms+' ms':s.error||'失败')})),
      sampleGroups:groupSamples((t.samples||[]).slice().sort((a,b)=>a.seq-b.seq).map((s,i)=>({id:t.attempt_id+':'+s.seq+':'+(s.target||''),target:s.target||'',time:s.timestamp,value:s.success?s.latency_ms:null,tone:s.success?'good':'bad',label:'第 '+(i+1)+' 次',error:s.success?undefined:s.error||'失败',description:date(s.timestamp)+' · '+(s.success?s.latency_ms+' ms':s.error||'失败')})))}
  })
  if(project==='download')return (w.downloads.value[k]||[]).slice().reverse().map(a=>({id:a.attempt_id,time:a.finished_at||a.requested_at,value:downloadSpeed(a),tone:downloadTone(a),attempts:[a],source:'manual',description:date(a.finished_at||a.requested_at)+' · '+outcomeLabel(a.result?.outcome)}))
  return (w.services.value[k]||[]).filter(a=>a.service_id===serviceId).slice().reverse().map(a=>({id:a.attempt_id,time:a.finished_at||a.requested_at,value:a.result?outcomeTone(a.result.outcome)==='good'?1:outcomeTone(a.result.outcome)==='warn'?.5:0:null,tone:a.result?(outcomeTone(a.result.outcome) as 'good'|'warn'|'bad'):'empty',attempts:[a],source:'manual',description:date(a.finished_at||a.requested_at)+' · '+outcomeLabel(a.result?.outcome)}))
}
export function serviceHealthPoints(w:Pick<Workspace,'services'|'serviceIds'>,n:NodeOption,ids=w.serviceIds.value):TrendPoint[]{
  const groups=new Map<string,Attempt[]>()
  for(const a of w.services.value[key(n)]||[]){if(!ids.includes(a.service_id||''))continue;const id=a.request_id.match(/^round:([^:]+):/)?.[1]||a.attempt_id,list=groups.get(id)||[];list.push(a);groups.set(id,list)}
  return Array.from(groups,([id,attempts]):TrendPoint=>{
    const ordered=attempts.slice().sort((a,b)=>Date.parse(a.requested_at)-Date.parse(b.requested_at)),samples=ordered.map<SamplePoint>(a=>({id:a.attempt_id,time:a.finished_at||a.requested_at,value:a.result?outcomeTone(a.result.outcome)==='good'?1:0:null,tone:a.result?outcomeTone(a.result.outcome) as HealthTone:'empty',label:a.rule.name||a.service_id||'服务',description:(a.rule.name||a.service_id)+' · '+(a.result?outcomeLabel(a.result.outcome):'未完成')})),good=samples.filter(s=>s.tone==='good').length,time=ordered[ordered.length-1].finished_at||ordered[ordered.length-1].requested_at
    return {id,time,value:samples.every(s=>s.tone==='empty')?null:good/samples.length,tone:samples.every(s=>s.tone==='empty')?'empty':samples.every(s=>s.tone==='good')?'good':samples.every(s=>s.tone==='bad')?'bad':'warn',source:'manual',samples,attempts:ordered,description:date(time)+' · '+good+'/'+ordered.length+' 项服务通过'}
  }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
export function monitorRoundPoints(samples:MonitorSample[]):TrendPoint[]{
  const rounds=new Map<string,MonitorSample[]>()
  for(const sample of samples){const id=sample.run_id||sample.sample_id;const group=rounds.get(id)||[];group.push(sample);rounds.set(id,group)}
  return Array.from(rounds,([id,group]):TrendPoint=>{
    const ordered=group.slice().sort((a,b)=>Date.parse(a.timestamp)-Date.parse(b.timestamp)||a.sample_id.localeCompare(b.sample_id)),successful=ordered.filter(s=>s.success)
    return {id,time:ordered[ordered.length-1].timestamp,value:median(successful.map(s=>s.latency/1e6)),tone:!successful.length?'bad':successful.length<ordered.length?'warn':'good',source:'monitor',
      description:date(ordered[ordered.length-1].timestamp)+' · 持续监测 · 已读 '+successful.length+'/'+ordered.length+' 样本成功',
      samples:ordered.map<SamplePoint>((s,i)=>({id:s.sample_id,time:s.timestamp,value:s.success?s.latency/1e6:null,tone:s.success?'good':'bad',label:'第 '+(i+1)+' 次',description:'第 '+(i+1)+' 次采样 · '+date(s.timestamp)+' · '+(s.success?(s.latency/1e6).toFixed(0)+' ms':s.error_detail||s.error_class||'失败')})),
      sampleGroups:groupSamples(ordered.map((s,i)=>({id:s.sample_id,target:s.target,time:s.timestamp,value:s.success?s.latency/1e6:null,tone:s.success?'good':'bad',label:'第 '+(i+1)+' 次',error:s.success?undefined:s.error_detail||s.error_class||'失败',description:date(s.timestamp)+' · '+(s.success?(s.latency/1e6).toFixed(0)+' ms':s.error_detail||s.error_class||'失败')})))}
  }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
export function recentService(w:Workspace,n:NodeOption,serviceId:string):Attempt|undefined{return w.latestService(n,serviceId)}
export function serviceSummary(w:Workspace,n:NodeOption,ids=w.serviceIds.value){const latest=ids.map(id=>recentService(w,n,id));return {passed:latest.filter(a=>a?.result&&outcomeTone(a.result.outcome)==='good').length,measured:latest.filter(a=>a?.result).length,total:latest.length}}

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
    return {id,time,value:good/ordered.length,tone:good===ordered.length?'good':good?'warn':'bad',source:'monitor',monitorSamples:ordered,
      description:date(time)+' · 持续监测 · 基础 HTTP · '+good+'/'+ordered.length+' 项成功',
      samples:ordered.map(s=>({id:s.sample_id,time:s.timestamp,value:s.success?1:0,tone:s.success?'good':'bad',label:monitorHTTPServices.find(p=>p.probeType===s.probe_type)?.name||s.probe_type,description:(monitorHTTPServices.find(p=>p.probeType===s.probe_type)?.name||s.probe_type)+' · '+(s.success?'HTTP 可达':s.error_detail||s.error_class||'失败')}))}
  }).sort((a,b)=>Date.parse(a.time)-Date.parse(b.time))
}
