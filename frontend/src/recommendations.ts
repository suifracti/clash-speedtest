import {key,downloadSpeed,downloadTarget,type NodeOption,type ServiceRule} from './domain'
import {attemptObservedAt,latestServices,qualityGroups,type QualityEvidence} from './nodeQuality'
import {downloadGroup,servicePassed} from './sharedResults'
export const purposes=[{id:'streaming',name:'流媒体'},{id:'ai',name:'AI'},{id:'download',name:'下载'},{id:'daily',name:'日常'}] as const
export type Purpose=typeof purposes[number]['id']
export interface Candidate {node:NodeOption;condition:string;score:number;basis:string;count:number;at:string}
// Keep numeric rankings inside one frozen measurement condition.
function top(list:Candidate[]){
 const groups=new Map<string,Candidate[]>()
 for(const c of list){const group=groups.get(c.condition)||[];group.push(c);groups.set(c.condition,group)}
 const cohort=[...groups.values()].sort((a,b)=>b.length-a.length||Date.parse(b[0].at)-Date.parse(a[0].at))[0]||[]
 return cohort.sort((a,b)=>b.score-a.score||Date.parse(b.at)-Date.parse(a.at)).slice(0,3)
}
export function recommend(nodes:NodeOption[],data:Record<string,QualityEvidence>,catalog:ServiceRule[]):Record<Purpose,Candidate[]>{
 const lists:Record<Purpose,Candidate[]>={streaming:[],ai:[],download:[],daily:[]}
 for(const node of nodes){
  const e=data[key(node)];if(!e)continue
  const g=qualityGroups(e.tests,e.monitor,'cloudflare')[0]
  if(g&&g.comparable&&!g.insufficient&&g.success>0&&e.reads[g.source==='monitor'?'monitor':'latency'].complete)lists.daily.push({node,condition:g.key,score:(g.successRate||0)*10000-(g.p50||0),basis:`Cloudflare · ${g.success}/${g.total} 次响应 · 中位 ${g.p50?.toFixed(0)} ms`,count:g.total,at:g.lastAt})
  const latest=[...e.downloads].sort((a,b)=>Date.parse(attemptObservedAt(b))-Date.parse(attemptObservedAt(a)))[0],group=downloadGroup(latest),speed=downloadSpeed(latest)
  if(e.reads.download.complete&&group.comparable&&latest&&speed!==null)lists.download.push({node,condition:group.key,score:speed,basis:`${downloadTarget(latest)} · ${speed.toFixed(2)} MiB/s · ${group.kind==='partial'?'部分下载':'完整下载'}`,count:e.downloads.filter(a=>downloadGroup(a).key===group.key).length,at:attemptObservedAt(latest)})
  if(e.reads.service.complete)for(const purpose of ['ai','streaming'] as const){
   const ids=catalog.filter(s=>s.result_kind!=='endpoint_reachability'&&(purpose==='ai'?/AI/i:/流媒体|影音|解锁/).test(s.category)).map(s=>s.service_id)
   const passed=latestServices(e.services).filter(a=>ids.includes(a.service_id||'')&&servicePassed(a))
   if(!passed.length)continue
   const at=passed.map(attemptObservedAt).sort((a,b)=>Date.parse(b)-Date.parse(a))[0]
   lists[purpose].push({node,condition:'service-rules',score:passed.length,basis:passed.map(a=>`${a.rule.name||a.service_id}：规则通过`).join('；'),count:e.services.filter(a=>passed.some(p=>p.service_id===a.service_id)).length,at})
  }
 }
 return {streaming:top(lists.streaming),ai:top(lists.ai),download:top(lists.download),daily:top(lists.daily)}
}
