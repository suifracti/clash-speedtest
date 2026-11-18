import {key,downloadReading,downloadSourceName,outcomeLabel,scope,type Attempt,type NodeScope,type NodeOption,type ServiceRule} from './domain'
import {attemptObservedAt,latestServices,qualityGroups,type QualityEvidence} from './nodeQuality'
import {nodeRegion,nodeRegionLabel} from './nodePresentation'
import {downloadGroup,serviceComparisonGroup,serviceIsProfile,servicePassed} from './sharedResults'
export const purposes=[{id:'streaming',name:'流媒体'},{id:'ai',name:'AI'},{id:'download',name:'下载'},{id:'daily',name:'日常'}] as const
export type Purpose=typeof purposes[number]['id']
export type ServicePurpose=Extract<Purpose,'ai'|'streaming'>
export interface Candidate {
 node:NodeOption;region:string;condition:string;score:number;basis:string;count:number;at:string
 comparable?:boolean;responseRate?:number;jitter?:number;downloadKind?:'stable'|'full'|'partial'
 inspectionScope?:NodeScope;inspectionServiceId?:string;configuration:'current'|'historical'|'mixed'|'unknown'
}
export function recommendationRegion(node:NodeOption){
 const code=nodeRegion(node)
 return code==='UK'?'GB':code
}
export function recommendationRegions(nodes:NodeOption[]){
 return [...new Set(nodes.map(recommendationRegion))].sort((a,b)=>Number(a==='unknown')-Number(b==='unknown')||nodeRegionLabel(a).localeCompare(nodeRegionLabel(b),'zh-CN'))
}
export function recommendationServices(catalog:ServiceRule[],purpose:ServicePurpose){
 const category=purpose==='ai'?/AI/i:/流媒体|影音|解锁/
 return catalog.filter(rule=>!rule.superseded_by&&rule.result_kind!=='endpoint_reachability'&&!serviceIsProfile(undefined,rule.service_id)&&!['exit_profile','ip_quality'].includes(rule.result_kind||'')&&category.test(rule.category))
}
export function recommendationService(options:ServiceRule[],purpose:ServicePurpose,selected?:string){
 const preferred=purpose==='ai'?'chatgpt_unlock':'netflix_unlock'
 return options.find(rule=>rule.service_id===selected)||options.find(rule=>rule.service_id===preferred)||options[0]
}
export function isRecommendation(candidate:Candidate){return candidate.configuration==='current'&&candidate.downloadKind!=='partial'}
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
const observedTime=(at:string)=>Date.parse(at)||0
const distinctNode=(candidate:Candidate)=>candidate.node.node_identity_key||key(candidate.node)
const serviceID=(a:Attempt)=>a.service_id||a.rule.service_id
const unmeasured=['queued','running','cancelled','user_cancelled','interrupted','not_executed','skipped_config']
const bySpeed=(a:Candidate,b:Candidate)=>b.score-a.score
const byDaily=(a:Candidate,b:Candidate)=>(b.responseRate??0)-(a.responseRate??0)||a.score-b.score||
 (a.jitter??Infinity)-(b.jitter??Infinity)||b.count-a.count

function chooseRegion(list:Candidate[],compare:(a:Candidate,b:Candidate)=>number){
 const confirmed=list.filter(isRecommendation)
 const current=list.filter(c=>c.configuration==='current')
 const eligible=confirmed.length?confirmed:current.length?current:list
 const preferred=eligible.some(c=>c.downloadKind==='stable')?eligible.filter(c=>c.downloadKind==='stable'):eligible
 const pool=preferred.some(c=>c.comparable===true)?preferred.filter(c=>c.comparable===true):preferred
 const groups=new Map<string,Candidate[]>()
 for(const c of pool){const group=groups.get(c.condition)||[];group.push(c);groups.set(c.condition,group)}
 const groupSize=(group:Candidate[])=>new Set(group.map(distinctNode)).size
 const newest=(group:Candidate[])=>group.reduce((at,c)=>Math.max(at,observedTime(c.at)),0)
 const cohort=[...groups.values()].sort((a,b)=>groupSize(b)-groupSize(a)||newest(b)-newest(a)||a[0].condition.localeCompare(b[0].condition))[0]
 return cohort.slice().sort((a,b)=>compare(a,b)||observedTime(b.at)-observedTime(a.at)||key(a.node).localeCompare(key(b.node)))[0]
}

function regionalPicks(list:Candidate[],compare=bySpeed){
 const regions=new Map<string,Candidate[]>()
 for(const candidate of list){
  const group=regions.get(candidate.region)||[]
  group.push(candidate)
  regions.set(candidate.region,group)
 }
 return Array.from(regions.values()).flatMap(group=>{
  const first=chooseRegion(group,compare)
  if(!isRecommendation(first))return [first]
  const alternatives=group.filter(c=>isRecommendation(c)&&distinctNode(c)!==distinctNode(first))
  if(!alternatives.length)return [first]
  const same=alternatives.filter(c=>c.condition===first.condition)
  return [first,chooseRegion(same.length?same:alternatives,compare)]
 })
}

export function recommend(nodes:NodeOption[],data:Record<string,QualityEvidence>,catalog:ServiceRule[],selected:Partial<Record<ServicePurpose,string>>={}):Record<Purpose,Candidate[]>{
 const lists:Record<Purpose,Candidate[]>={streaming:[],ai:[],download:[],daily:[]}
 const servicePurposes=['ai','streaming'] as const
 const wanted=servicePurposes.map(purpose=>({purpose,rule:recommendationService(recommendationServices(catalog,purpose),purpose,selected[purpose])}))
 for(const node of nodes){
  const e=data[key(node)],region=recommendationRegion(node)
  if(!e||region==='unknown')continue

  const g=qualityGroups(e.tests,e.monitor,'cloudflare')[0]
  if(g&&g.p50!==null&&g.lastAt&&(g.comparable||g.source!=='monitor'&&g.method!=='旧方法未记录')&&
     !g.insufficient&&e.reads[g.source==='monitor'?'monitor':'latency'].complete){
   lists.daily.push({...provenance(node,dailySources(e,g.key)),node,region,condition:g.key,comparable:g.comparable,
    responseRate:g.successRate??0,score:g.p50,jitter:g.jitter??undefined,
    basis:`Cloudflare · ${g.success}/${g.total} 次响应 · 中位 ${g.p50.toFixed(0)} ms${g.jitter===null?'':` · 波动 ${g.jitter.toFixed(0)} ms`}`,
    count:g.total,at:g.lastAt})
  }

  const savedDownloads=e.downloads.filter(a=>a.persistence_state==='saved'&&a.result&&!unmeasured.includes(a.execution_state))
  const latest=savedDownloads.sort((a,b)=>observedTime(attemptObservedAt(b))-observedTime(attemptObservedAt(a))||b.attempt_id.localeCompare(a.attempt_id))[0]
  const reading=downloadReading(latest),group=downloadGroup(latest)
  if(e.reads.download.complete&&latest&&reading&&group.kind!=='invalid'&&(group.comparable||!!latest.rule.method&&latest.rule.rule_version!==undefined)){
   const at=attemptObservedAt(latest)
   if(at)lists.download.push({...provenance(node,[{record:latest,at}]),node,region,condition:group.key,comparable:group.comparable,
    downloadKind:group.kind==='partial'?'partial':latest.rule.mode==='adaptive'?'stable':'full',
    score:reading.value,basis:`${downloadSourceName(latest)} · ${reading.label} ${reading.value.toFixed(2)} MiB/s${group.kind==='partial'?` · ${outcomeLabel(latest.result?.end_reason||latest.result?.outcome)}`:''}`,
    count:savedDownloads.filter(a=>downloadGroup(a).key===group.key).length,at})
  }

  if(!e.reads.service.complete)continue
  const savedServices=e.services.filter(a=>a.persistence_state==='saved')
  const latestByService=latestServices(savedServices)
  for(const {purpose,rule} of wanted){
   if(!rule)continue
   const id=rule.service_id
   const recent=latestByService.find(a=>serviceID(a)===id)
   if(!recent||!servicePassed(recent))continue
   const at=attemptObservedAt(recent)
   if(!at||!savedServices.filter(a=>serviceID(a)===id&&observedTime(attemptObservedAt(a))===observedTime(at)).every(servicePassed))continue
   const condition=serviceComparisonGroup([recent],[id]).key
   const count=savedServices.filter(a=>serviceID(a)===id&&servicePassed(a)&&
    a.profile_id===recent.profile_id&&a.node_key===recent.node_key&&a.node_identity_key===recent.node_identity_key&&a.config_revision_key===recent.config_revision_key&&
    serviceComparisonGroup([a],[id]).key===condition).length
   lists[purpose].push({...provenance(node,[{record:recent,at,serviceId:id}]),node,region,condition,score:count,
    basis:`${rule.name} · 规则通过`,count,at})
  }
 }
 return {streaming:regionalPicks(lists.streaming),ai:regionalPicks(lists.ai),download:regionalPicks(lists.download),daily:regionalPicks(lists.daily,byDaily)}
}
