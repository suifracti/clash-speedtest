import {key,downloadSpeed,downloadTarget,scope,type NodeScope,type NodeOption,type ServiceRule} from './domain'
import {downloadCondition} from './measurementRounds'
import {attemptObservedAt,latestServices,qualityGroups,type QualityEvidence} from './nodeQuality'
import {downloadGroup,servicePassed} from './sharedResults'
export const purposes=[{id:'streaming',name:'流媒体'},{id:'ai',name:'AI'},{id:'download',name:'下载'},{id:'daily',name:'日常'}] as const
export type Purpose=typeof purposes[number]['id']
export interface Candidate {node:NodeOption;condition:string;score:number;basis:string;count:number;at:string;comparable?:boolean;inspectionScope?:NodeScope;inspectionServiceId?:string;configuration:'current'|'historical'|'mixed'|'unknown'}
interface EvidenceSource {record:NodeScope;at:string;serviceId?:string}
// Bind navigation to the evidence used for this purpose, never to a timestamp
// shared by unrelated probes. A query fixes the account; legacy missing identity
// fields stay empty rather than borrowing the current configuration identity.
function provenance(node:NodeOption,sources:EvidenceSource[]){
 const ordered=[...sources].sort((a,b)=>(Date.parse(b.at)||0)-(Date.parse(a.at)||0))
 const scopes=ordered.map(({record})=>({...scope(record),profile_id:record.profile_id||node.profile_id,node_key:record.node_key||'',node_identity_key:record.node_identity_key||'',config_revision_key:record.config_revision_key||''}))
 const matches=(s:NodeScope)=>s.profile_id===node.profile_id&&s.node_key===node.node_key&&s.node_identity_key===node.node_identity_key&&s.config_revision_key===node.config_revision_key
 const current=scopes.some(matches),older=scopes.some(s=>!matches(s))
 const unknown=!scopes.length||scopes.some(s=>!s.node_key||!s.node_identity_key||!s.config_revision_key)
 const configuration:Candidate['configuration']=unknown?'unknown':current?(older?'mixed':'current'):'historical'
 return {inspectionScope:scopes[0],inspectionServiceId:ordered[0]?.serviceId,configuration}
}
function dailySources(e:QualityEvidence,condition:string):EvidenceSource[]{
 return [
  ...e.tests.flatMap(record=>qualityGroups([record],[],'cloudflare').filter(g=>g.key===condition&&g.total>0).map(g=>({record,at:g.lastAt}))),
  ...e.monitor.flatMap(record=>qualityGroups([],[record],'cloudflare').filter(g=>g.key===condition&&g.total>0).map(g=>({record,at:g.lastAt}))),
 ]
}
// Keep numeric rankings inside one frozen measurement condition.
function top(list:Candidate[]){
 const pool=list.some(c=>c.comparable!==false)?list.filter(c=>c.comparable!==false):list
 const groups=new Map<string,Candidate[]>()
 for(const c of pool){const group=groups.get(c.condition)||[];group.push(c);groups.set(c.condition,group)}
 const cohort=[...groups.values()].sort((a,b)=>b.length-a.length||Date.parse(b[0].at)-Date.parse(a[0].at))[0]||[]
 return cohort.sort((a,b)=>b.score-a.score||Date.parse(b.at)-Date.parse(a.at)).slice(0,3)
}
export function recommend(nodes:NodeOption[],data:Record<string,QualityEvidence>,catalog:ServiceRule[]):Record<Purpose,Candidate[]>{
 const lists:Record<Purpose,Candidate[]>={streaming:[],ai:[],download:[],daily:[]}
 for(const node of nodes){
  const e=data[key(node)];if(!e)continue
  const g=qualityGroups(e.tests,e.monitor,'cloudflare')[0]
  const dailyValid=!!g&&(g.comparable||g.source!=='monitor'&&g.method!=='旧方法未记录'&&g.p50!==null)&&!g.insufficient&&g.success>0&&e.reads[g.source==='monitor'?'monitor':'latency'].complete
  if(dailyValid)lists.daily.push({...provenance(node,dailySources(e,g.key)),node,condition:g.key,comparable:g.comparable,score:(g.successRate||0)*10000-(g.p50||0),basis:`Cloudflare · ${g.success}/${g.total} 次响应 · 中位 ${g.p50?.toFixed(0)} ms`,count:g.total,at:g.lastAt})
  const latest=[...e.downloads].sort((a,b)=>Date.parse(attemptObservedAt(b))-Date.parse(attemptObservedAt(a)))[0],group=downloadGroup(latest),speed=downloadSpeed(latest)
  const downloadValid=e.reads.download.complete&&group.kind!=='invalid'&&!!latest&&speed!==null&&(group.comparable||!!latest.rule.method&&latest.rule.rule_version!==undefined)
  if(downloadValid){const cond=downloadCondition(latest);lists.download.push({...provenance(node,[{record:latest,at:attemptObservedAt(latest)}]),node,condition:cond,comparable:group.comparable,score:speed,basis:`${downloadTarget(latest)} · ${speed.toFixed(2)} MiB/s · ${latest.rule.mode==='adaptive'?(group.kind==='full'?'速度已稳定':'未稳定均速'):(group.kind==='partial'?'部分下载':'完整下载')}`,count:e.downloads.filter(a=>downloadGroup(a).kind!=='invalid'&&downloadCondition(a)===cond).length,at:attemptObservedAt(latest)})}
  if(e.reads.service.complete)for(const purpose of ['ai','streaming'] as const){
   const ids=catalog.filter(s=>s.result_kind!=='endpoint_reachability'&&(purpose==='ai'?/AI/i:/流媒体|影音|解锁/).test(s.category)).map(s=>s.service_id)
   const passed=latestServices(e.services).filter(a=>ids.includes(a.service_id||'')&&servicePassed(a))
   if(!passed.length)continue
   const at=passed.map(attemptObservedAt).sort((a,b)=>Date.parse(b)-Date.parse(a))[0]
   lists[purpose].push({...provenance(node,passed.map(record=>({record,at:attemptObservedAt(record),serviceId:record.service_id||record.rule.service_id}))),node,condition:'service-rules',score:passed.length,basis:passed.map(a=>`${a.rule.name||a.service_id}：规则通过`).join('；'),count:e.services.filter(a=>passed.some(p=>p.service_id===a.service_id)).length,at})
  }
 }
 return {streaming:top(lists.streaming),ai:top(lists.ai),download:top(lists.download),daily:top(lists.daily)}
}
