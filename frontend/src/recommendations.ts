import {key,downloadReading,downloadSourceName,median,outcomeLabel,scope,type Attempt,type NodeScope,type NodeOption,type ServiceRule} from './domain'
import {activeLatencyTargets} from './latencyTargets'
import {attemptObservedAt,latestServices,qualityGroups,type QualityEvidence,type QualityGroup} from './nodeQuality'
import {nodeRegion,nodeRegionLabel} from './nodePresentation'
import {downloadGroup,serviceComparisonGroup,serviceIsProfile,servicePassed} from './sharedResults'
import {unlockResult} from './unlock'
import {exitLanding} from './exitLanding'
export const purposes=[{id:'streaming',name:'流媒体与解锁'},{id:'ai',name:'AI'},{id:'download',name:'下载'},{id:'daily',name:'日常'}] as const
export type Purpose=typeof purposes[number]['id']
export type ServicePurpose=Extract<Purpose,'ai'|'streaming'>
export interface CfChallengeHint {
 tone:'good'|'warn'|'bad';label:string;detail:string
}
export interface PurityHint {
 score:number;grade:string;tone:'good'|'warn'|'bad';typeLabel:string;title:string
}
export interface Candidate {
 node:NodeOption;region:string;condition:string;score:number;basis:string;count:number;at:string
 comparable?:boolean;responseRate?:number;jitter?:number;downloadKind?:'stable'|'full'|'partial'
 siteCount?:number;respondingSites?:number;blockedSites?:string[];cfChallenge?:CfChallengeHint;purity?:PurityHint
 inspectionScope?:NodeScope;inspectionServiceId?:string;inspectionTargetId?:string;configuration:'current'|'historical'|'mixed'|'unknown'
}
export function recommendationRegion(node:NodeOption){
 const code=nodeRegion(node)
 return code==='UK'?'GB':code
}
export function recommendationRegions(nodes:NodeOption[]){
 return [...new Set(nodes.map(recommendationRegion))].sort((a,b)=>Number(a==='unknown')-Number(b==='unknown')||nodeRegionLabel(a).localeCompare(nodeRegionLabel(b),'zh-CN'))
}
export function recommendationServices(catalog:ServiceRule[],purpose:ServicePurpose){
 const category=purpose==='ai'?/AI/i:/流媒体|影音|解锁|日本/
 return catalog.filter(rule=>!rule.superseded_by&&rule.result_kind!=='endpoint_reachability'&&!serviceIsProfile(undefined,rule.service_id)&&!['exit_profile','ip_quality'].includes(rule.result_kind||'')&&category.test(rule.category))
}
export function recommendationService(options:ServiceRule[],purpose:ServicePurpose,selected?:string){
 const preferred=purpose==='ai'?'gemini_unlock':'netflix_unlock'
 return options.find(rule=>rule.service_id===selected)||options.find(rule=>rule.service_id===preferred)||options[0]
}
export function isRecommendation(candidate:Candidate){
 return candidate.configuration==='current'&&candidate.downloadKind!=='partial'&&!(candidate.blockedSites&&candidate.blockedSites.length)
}
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
function dailySourcesForTarget(e:QualityEvidence,targetId:string,condition:string):EvidenceSource[]{
 return [
  ...e.tests.flatMap(record=>qualityGroups([record],[],targetId).filter(g=>g.key===condition&&g.total>0).map(g=>({record,at:g.lastAt}))),
  ...e.monitor.flatMap(record=>qualityGroups([],[record],targetId).filter(g=>g.key===condition&&g.total>0).map(g=>({record,at:g.lastAt}))),
 ]
}
const observedTime=(at:string)=>Date.parse(at)||0
const distinctNode=(candidate:Candidate)=>candidate.node.node_identity_key||key(candidate.node)
const serviceID=(a:Attempt)=>a.service_id||a.rule.service_id
const unmeasured=['queued','running','cancelled','user_cancelled','interrupted','not_executed','skipped_config']
const bySpeed=(a:Candidate,b:Candidate)=>b.score-a.score
const byDaily=(a:Candidate,b:Candidate)=>(b.responseRate??0)-(a.responseRate??0)||a.score-b.score||
 (a.jitter??Infinity)-(b.jitter??Infinity)||b.count-a.count

function cfChallengeHint(node:NodeOption,e:QualityEvidence):CfChallengeHint|undefined{
 if(!e.reads.service.complete)return undefined
 const saved=e.services.filter(a=>a.persistence_state==='saved'&&serviceID(a)==='cloudflare_challenge')
 const recent=latestServices(saved)[0]
 if(!recent)return undefined
 if(recent.profile_id!==node.profile_id||recent.node_key!==node.node_key||recent.node_identity_key!==node.node_identity_key||recent.config_revision_key!==node.config_revision_key)return undefined
 const res=unlockResult(recent,'cloudflare_challenge')
 if(res.tone==='good'||res.tone==='warn'||res.tone==='bad')return {tone:res.tone,label:res.label,detail:res.reason}
 return undefined
}

function purityHint(node:NodeOption,e:QualityEvidence):PurityHint|undefined{
 if(!e.reads.service.complete)return undefined
 const currentSaved=e.services.filter(a=>a.persistence_state==='saved'&&a.profile_id===node.profile_id&&a.node_key===node.node_key&&a.node_identity_key===node.node_identity_key&&a.config_revision_key===node.config_revision_key)
 if(!currentSaved.length)return undefined
 const p=exitLanding(node,currentSaved).purity
 if(p.score===null||p.tone==='empty')return undefined
 return {score:p.score,grade:p.grade,tone:p.tone as 'good'|'warn'|'bad',typeLabel:p.typeLabel,title:p.title}
}

function dailyCandidate(node:NodeOption,e:QualityEvidence,region:string,cfChallenge?:CfChallengeHint,purity?:PurityHint):Candidate|undefined{
 const cohorts=new Map<string,{target:typeof activeLatencyTargets[number];g:QualityGroup}[]>()
 for(const target of activeLatencyTargets){
  const g=qualityGroups(e.tests,e.monitor,target.id)[0]
  if(!g||!g.total||!g.lastAt)continue
  if(!(g.comparable||(g.source!=='monitor'&&g.method!=='旧方法未记录')))continue
  if(!e.reads[g.source==='monitor'?'monitor':'latency'].complete)continue
  const baseCond=JSON.stringify([g.source,g.method,g.condition])
  const list=cohorts.get(baseCond)||[]
  list.push({target,g})
  cohorts.set(baseCond,list)
 }
 if(!cohorts.size)return undefined
 const newestAt=(list:{g:QualityGroup}[])=>list.reduce((best,item)=>Math.max(best,observedTime(item.g.lastAt)),0)
 const [baseCond,sites]=[...cohorts.entries()].sort((a,b)=>
  (a[1][0]!.g.source==='monitor'?1:0)-(b[1][0]!.g.source==='monitor'?1:0)||
  newestAt(b[1])-newestAt(a[1])||b[1].length-a[1].length||a[0].localeCompare(b[0])
 )[0]!

 if(sites.length===1){
  const {target,g}=sites[0]!
  if(g.p50===null||g.insufficient)return undefined
  return {
   ...provenance(node,dailySourcesForTarget(e,target.id,g.key)),
   node,region,condition:g.key,comparable:g.comparable,
   responseRate:g.successRate??0,score:g.p50,jitter:g.jitter??undefined,
   siteCount:1,respondingSites:1,cfChallenge,purity,inspectionTargetId:target.id,
   basis:`${target.name} · ${g.success}/${g.total} 次响应 · 中位 ${g.p50.toFixed(0)} ms${g.jitter===null?'':` · 波动 ${g.jitter.toFixed(0)} ms`}`,
   count:g.total,at:g.lastAt,
  }
 }

 const responding=sites.filter(s=>s.g.success>0&&s.g.p50!==null)
 const blocked=sites.filter(s=>s.g.total>=5&&s.g.success===0)
 const totalSamples=sites.reduce((sum,s)=>sum+s.g.total,0)
 const successSamples=sites.reduce((sum,s)=>sum+s.g.success,0)
 if(!responding.length||totalSamples<5||successSamples<5||!responding.some(s=>!s.g.insufficient))return undefined
 const p50s=responding.map(s=>s.g.p50!).sort((a,b)=>a-b)
 const score=median(p50s)
 if(score===null)return undefined
 const jitters=responding.map(s=>s.g.jitter).filter((v):v is number=>v!==null)
 const jitter=jitters.length?jitters.reduce((a,b)=>a+b,0)/jitters.length:undefined
 const responseRate=totalSamples?(successSamples/totalSamples)*100:0
 const comparable=sites.every(s=>s.g.comparable)
 const lastAt=sites.reduce((best,s)=>observedTime(s.g.lastAt)>observedTime(best)?s.g.lastAt:best,'')
 const condition=JSON.stringify(['multi-site',baseCond,sites.map(s=>s.target.id).sort()])
 const blockedNames=blocked.map(s=>s.target.name)
 const sources=sites.flatMap(s=>dailySourcesForTarget(e,s.target.id,s.g.key))
 const siteLabel=sites.length===4?'四站综合':`${sites.length} 站综合`
 const basis=blockedNames.length
  ?`${siteLabel}（${responding.length}/${sites.length} 站可用） · 部分站点不通（${blockedNames.join('、')}） · 响应 ${successSamples}/${totalSamples} · 中位 ${score.toFixed(0)} ms`
  :`${siteLabel}（${responding.length}/${sites.length} 站） · ${successSamples}/${totalSamples} 次响应 · 中位 ${score.toFixed(0)} ms${jitter===undefined?'':` · 波动 ${jitter.toFixed(0)} ms`}`
 return {
  ...provenance(node,sources),
  node,region,condition,comparable,responseRate,score,jitter,
  siteCount:sites.length,respondingSites:responding.length,
  blockedSites:blockedNames.length?blockedNames:undefined,
  cfChallenge,purity,
  basis,count:totalSamples,at:lastAt,
 }
}

export function candidateDiff(first:Candidate,second:Candidate,purpose:Purpose){
 if(second.condition!==first.condition)return ''
 if(purpose==='daily'){
  const rateDiff=Math.round((first.responseRate??0)-(second.responseRate??0))
  if(rateDiff>=1)return `响应率比首选低 ${rateDiff}%`
  const msDiff=Math.round(second.score-first.score)
  if(msDiff>=1)return `中位延迟比首选高 ${msDiff} ms`
  if(first.jitter!==undefined&&second.jitter!==undefined){
   const jitterDiff=Math.round(second.jitter-first.jitter)
   if(jitterDiff>=1)return `波动比首选高 ${jitterDiff} ms`
  }
  return '与首选表现相近'
 }
 if(purpose==='download'){
  const speedDiff=first.score-second.score
  if(speedDiff>=0.05)return `速度比首选低 ${speedDiff.toFixed(2)} MiB/s`
  return '与首选速度相近'
 }
 const countDiff=first.count-second.count
 if(countDiff>=1)return `通过次数比首选少 ${countDiff} 次`
 if(observedTime(first.at)>observedTime(second.at))return '最近通过早于首选'
 return '与首选同为当前通过'
}

function chooseRegion(list:Candidate[],compare:(a:Candidate,b:Candidate)=>number){
 const confirmed=list.filter(isRecommendation)
 const current=list.filter(c=>c.configuration==='current')
 const eligible=confirmed.length?confirmed:current.length?current:list
 const preferred=eligible.some(c=>c.downloadKind==='stable')?eligible.filter(c=>c.downloadKind==='stable'):eligible
 const multiSite=preferred.filter(c=>(c.siteCount??1)>1)
 const sitePool=multiSite.length?multiSite:preferred
 const pool=sitePool.some(c=>c.comparable===true)?sitePool.filter(c=>c.comparable===true):sitePool
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
  const cfChallenge=cfChallengeHint(node,e),purity=purityHint(node,e)

  const daily=dailyCandidate(node,e,region,cfChallenge,purity)
  if(daily)lists.daily.push(daily)

  const savedDownloads=e.downloads.filter(a=>a.persistence_state==='saved'&&a.result&&!unmeasured.includes(a.execution_state))
  const latest=savedDownloads.sort((a,b)=>observedTime(attemptObservedAt(b))-observedTime(attemptObservedAt(a))||b.attempt_id.localeCompare(a.attempt_id))[0]
  const reading=downloadReading(latest),group=downloadGroup(latest)
  if(e.reads.download.complete&&latest&&reading&&group.kind!=='invalid'&&(group.comparable||!!latest.rule.method&&latest.rule.rule_version!==undefined)){
   const at=attemptObservedAt(latest)
   if(at)lists.download.push({...provenance(node,[{record:latest,at}]),node,region,condition:group.key,comparable:group.comparable,
    downloadKind:group.kind==='partial'?'partial':latest.rule.mode==='adaptive'?'stable':'full',cfChallenge,purity,
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
   lists[purpose].push({...provenance(node,[{record:recent,at,serviceId:id}]),node,region,condition,score:count,cfChallenge,purity,
    basis:`${rule.name} · 规则通过`,count,at})
  }
 }
 return {streaming:regionalPicks(lists.streaming),ai:regionalPicks(lists.ai),download:regionalPicks(lists.download),daily:regionalPicks(lists.daily,byDaily)}
}
