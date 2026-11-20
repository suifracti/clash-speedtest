import {type Attempt,type NodeOption} from './domain'
import {attemptObservedAt,latestServices} from './nodeQuality'
import {compactExitStatus,exitField,riskScore,toExitRecord} from './exitSummaryPresentation'
import {ipKindFromLatest} from './ipKind'
import {nodeRegionLabel} from './nodePresentation'
import {unlockResult} from './unlock'
const codes='CN HK MO TW JP SG US KR GB DE FR NL AU CA RU TH MY VN PH ID IN TR AE AR BR IT ES SE CH PL ZA KH EG NG PK'.split(' ')
const english=new Intl.DisplayNames(['en'],{type:'region'})
function country(value?:string){const v=(value||'').trim().toUpperCase();if(['XX','ZZ'].includes(v))return '';if(/香港|HONG\s*KONG/i.test(v))return 'HK';if(/澳门|MACAU|MACAO/i.test(v))return 'MO';if(/台湾|TAIWAN/i.test(v))return 'TW';if(/^[A-Z]{2}$/.test(v))return v==='UK'?'GB':v;return codes.find(c=>[nodeRegionLabel(c),english.of(c)].some(name=>name?.toUpperCase()===v))||(/中国大陆|中国|MAINLAND CHINA/i.test(v)?'CN':'')}
export function maskedIP(value:string){if(value.includes(':')){const p=value.split(':');return p[0]+':…:'+p.at(-1)}const p=value.split('.');return p.length===4?`${p[0]}.*.*.${p[3]}`:value||'—'}
export interface ReputationSourceItem {
 id:string
 name:string
 risk:number|null
 tone:'good'|'warn'|'bad'|'empty'
 status:string
 detail:string
}
function riskTone(score:number):'good'|'warn'|'bad'{return score<=25?'good':score<=60?'warn':'bad'}
export function exitLanding(node:Pick<NodeOption,'country_code'>,attempts:Attempt[]){
 const latest=latestServices(attempts),find=(id:string)=>latest.find(a=>(a.service_id||a.rule.service_id)===id),pure=find('ippure_ip_quality'),ping=find('ping0_ipv4')||find('ping0_ip_quality'),ping6=find('ping0_ipv6'),ipquery=find('ipquery_ip_quality'),iplogs=find('iplogs_ip_quality'),cf=find('cloudflare_trace')
 const valid=(a?:Attempt)=>a?.result?.outcome==='profiled'
 const ipRecord=[find('exit_ipv4'),pure,ping,ipquery,iplogs,cf,find('exit_ipv6')].find(a=>valid(a)&&exitField(a!,'ip'))
 const library=[...new Set([pure,ping,ping6,ipquery,iplogs].filter(a=>valid(a)).map(a=>country(exitField(a!,'country_code','country','location'))).filter(Boolean))]
 const google=latest.filter(a=>['google_region','youtube_premium','gemini_unlock','gemini_web'].includes(a.service_id||a.rule.service_id||'')).map(a=>({a,region:country(exitField(a,'service_region'))})).filter(s=>s.region).sort((a,b)=>Date.parse(attemptObservedAt(b.a))-Date.parse(attemptObservedAt(a.a)))[0]?.region||''
 const cfRegion=valid(cf)?country(exitField(cf!,'country_code')):''
 const sources=[...library.map(region=>({name:'IP 库',region})),{name:'Cloudflare',region:cfRegion},{name:'Google',region:google}],known=sources.filter(s=>s.region),expected=country(node.country_code),mismatch=known.find(s=>expected&&s.region!==expected),mainland=known.find(s=>s.region==='CN')
 const votes=[library[0],cfRegion,expected].filter(Boolean)
 const consensusRegion=votes.find(c=>votes.filter(v=>v===c).length>=2)||library[0]||cfRegion||expected
 const googleNote=google&&consensusRegion&&google!==consensusRegion?`Google 判定 ${google}，与落地 ${consensusRegion} 不一致`:''
 const count=Number(!!library.length)+Number(!!cfRegion)+Number(!!google)
 const label=mainland?(mainland.name==='Google'?'Google 判定为中国大陆':`${mainland.name} 判定 CN（中国大陆）`):mismatch?`名称 ${expected}，${mismatch.name} 判定 ${mismatch.region}`:count===3&&expected?'落地一致':count?`待补来源（${count}/3）`:'未测'
 const tags=[`名称 ${expected||'未识别'}`,`IP 库 ${library.join(' / ')||'未取得'}`,`Cloudflare ${cfRegion||'未取得'}`,`Google ${google||'未取得'}`]
 const field=(a:Attempt|undefined,...keys:string[])=>a&&valid(a)?exitField(a,...keys)||'—':'—'
 const riskOptions=[{label:'IPPure',value:field(pure,'risk_score','fraud_score')},{label:'机器',value:field(pure,'asn_bot_percent')}].filter(s=>s.value!=='—'&&Number.isFinite(Number(s.value))).sort((a,b)=>Number(b.value)-Number(a.value)),risk=riskOptions[0],riskValue=risk?Number(risk.value):undefined
 const ipKind=ipKindFromLatest(latest)

 // Collect ALL independent reputation stations side-by-side instead of letting
 // a single fallback provider masquerade as an authoritative "纯净 85" grade.
 const stationSpecs:[string,string,Attempt|undefined][]=[
  ['ippure_ip_quality','IPPure',pure],
  ['ping0','Ping0',ping||ping6],
  ['ipquery_ip_quality','IPQuery',ipquery],
  ['iplogs_ip_quality','IPLogs',iplogs],
  ['ipok','IPOK',find('ipok_ipv4')||find('ipok_ipv6')],
 ]
 const reputationSources:ReputationSourceItem[]=[]
 const reputationIssues:string[]=[]
 const scoredRecords:Array<{record:ReturnType<typeof toExitRecord>;risk:number;name:string}>=[]
 for(const [id,name,attempt] of stationSpecs){
  if(!attempt)continue
  const rec=toExitRecord(attempt,attemptObservedAt(attempt))
  const score=riskScore(rec)
  if(score!==undefined){
   const rounded=Math.round(score)
   scoredRecords.push({record:rec,risk:rounded,name})
   reputationSources.push({id,name,risk:rounded,tone:riskTone(rounded),status:'已取得',detail:`${name} 风险分 ${rounded}/100（越低越好）`})
  }else if(attempt.result||['failed','cancelled','interrupted'].includes(attempt.execution_state)){
   const status=compactExitStatus(rec)
   reputationIssues.push(`${name} ${status}`)
   reputationSources.push({id,name,risk:null,tone:'empty',status,detail:`${name}：${rec.status}`})
  }
 }
 if(pure){
  const cfStatus=exitField(pure,'cf_status')
  const cfScoreRaw=exitField(pure,'cf_risk_score')
  if(cfStatus==='已取得'&&/^\d+(?:\.\d+)?$/.test(cfScoreRaw)){
   const cfNum=Math.round(Number(cfScoreRaw))
   reputationSources.push({id:'cf_risk',name:'CF系数',risk:cfNum,tone:riskTone(cfNum),status:'已取得',detail:`Cloudflare 风险系数 ${cfNum}/100（越低越好）`})
  }else if(cfStatus&&cfStatus!=='未查询'){
   reputationIssues.push(`CF系数 ${cfStatus}`)
  }
 }

 const sourceCount=scoredRecords.length
 const avgRisk=sourceCount?Math.round(scoredRecords.reduce((sum,item)=>sum+item.risk,0)/sourceCount):null
 const riskMin=sourceCount?Math.min(...scoredRecords.map(i=>i.risk)):null
 const riskMax=sourceCount?Math.max(...scoredRecords.map(i=>i.risk)):null
 const divergent=riskMin!==null&&riskMax!==null&&riskMax-riskMin>=30
 const singleFallback=sourceCount===1&&reputationIssues.length>0
 const reputation=scoredRecords[0]?.record
 const purityScore=avgRisk!==null?Math.round(100-avgRisk):null
 const observed=reputation?.attempt||latest.find(a=>valid(a)&&['ippure_ip_quality','ping0_ipv4','ping0_ipv6','ping0_ip_quality','ipquery_ip_quality','iplogs_ip_quality','ipok_ipv4','ipok_ipv6','cloudflare_trace'].includes(a.service_id||a.rule.service_id||''))
 const purityAt=observed?exitField(observed,'query_at')||attemptObservedAt(observed):''
 const typeText=ipKind.type==='idc'?'机房 IP':ipKind.type==='residential'?'住宅 IP':ipKind.type==='mobile'?'移动 IP':'类型未知'
 const originText=ipKind.origin==='native'?`原生 ${library[0]||consensusRegion||'地区未知'}`:ipKind.origin==='broadcast'?'广播 IP':'原生未知'
 const purityDate=purityAt?new Date(purityAt).toLocaleDateString('en-US',{month:'2-digit',day:'2-digit'}):''
 const purityGrade=purityScore===null?'':purityScore>=80?'纯净':purityScore>=60?'较净':purityScore>=40?'一般':'较差'
 const rawRisk=avgRisk
 const multiBreakdown=scoredRecords.map(i=>`${i.name} ${i.risk}`).join(' · ')
 const issueNote=reputationIssues.length?` · 异常：${reputationIssues.join('、')}`:''
 const purityTitle=sourceCount>=2
  ?`多源信誉（${sourceCount} 站均值纯净 ${purityScore}/100，风险 ${avgRisk}/100，越低越好）：${multiBreakdown}${divergent?'（各站分歧较大，请以分站为准）':''}${issueNote} · ${typeText}${purityAt?' · '+purityAt:''}`
  :reputation
   ?`纯净度 ${purityScore}/100（${purityGrade}，越高越干净）· 由 ${reputation.provider} 风险分 ${riskScore(reputation)} 换算 · 越高越干净${singleFallback?'（其余站点未返回，仅单源参考）':''}${issueNote} · ${typeText}${purityAt?' · '+purityAt:''}`
   :reputationIssues.length?`信誉查询异常：${reputationIssues.join('、')}`:'没有可换算的风险分'
 const purityTone=purityScore===null?'empty':singleFallback||(divergent&&purityScore>=80)?'warn':purityScore>=80?'good':purityScore>=50?'warn':'bad'
 const purity={
  score:purityScore,grade:purityGrade,rawRisk,typeLabel:ipKind.typeLabel,
  sourceCount,divergent,singleFallback,riskMin,riskMax,
  label:purityScore===null?'—':`${purityScore} · ${ipKind.typeLabel}`,
  tone:purityTone as 'good'|'warn'|'bad'|'empty',
  title:purityTitle,
  summary:sourceCount>=2
   ?`多源均值风险 ${avgRisk}（${multiBreakdown}）· ${typeText} · ${originText}`+(purityDate?` · ${purityDate} 查询`:'')
   :`纯净度 ${purityScore??'—'} · ${typeText} · ${originText}`+(purityDate?` · ${purityDate} 查询`:''),
 }
 const cfChallengeAttempt=find('cloudflare_challenge')
 const cfChallengeRes=cfChallengeAttempt?unlockResult(cfChallengeAttempt,'cloudflare_challenge'):undefined
 const cfChallenge=cfChallengeRes&&(cfChallengeRes.tone==='good'||cfChallengeRes.tone==='warn'||cfChallengeRes.tone==='bad')
  ?{
   shortLabel:cfChallengeRes.tone==='good'?'直通':cfChallengeRes.tone==='warn'?'部分弹盾':'易弹盾',
   label:cfChallengeRes.label,
   tone:cfChallengeRes.tone as 'good'|'warn'|'bad',
   detail:cfChallengeRes.reason,
   rank:cfChallengeRes.tone==='good'?0:cfChallengeRes.tone==='warn'?1:2,
  }
  :{shortLabel:cfChallengeAttempt?.result?'异常':'未测',label:cfChallengeAttempt?.result?cfChallengeRes?.label||'无法判断':'未测',tone:'empty' as const,detail:cfChallengeRes?.reason||'尚未进行 Cloudflare 人机验证检测',rank:null as number|null}
 return {purity,reputationSources,reputationIssues,cfChallenge,consensusRegion,googleNote,region:mainland?.region||google||cfRegion||library[0]||expected,org:ipRecord?exitField(ipRecord,'organization','org','isp'):'',exitKnown:!!library.length||!!cfRegion||!!google,riskLabel:risk?risk.label+' '+Math.round(riskValue!)+(risk.label==='机器'?'%':''):'未测',riskTone:riskValue===undefined?'empty':riskValue>=70?'bad':riskValue>=30?'warn':'good',ip:maskedIP(ipRecord?exitField(ipRecord,'ip'):''),country:library.join(' / ')||'—',label,tone:mainland?'bad':mismatch?'warn':count===3&&expected?'good':'empty',tags,detail:tags.join(' · '),pure:field(pure,'risk_score','fraud_score'),ping:[field(ping,'risk_score'),field(ping6,'risk_score')].join(' / '),cf:field(pure,'cf_risk_score'),traffic:valid(pure)&&exitField(pure!,'asn_human_percent')&&exitField(pure!,'asn_bot_percent')?`${exitField(pure!,'asn_human_percent')}% / ${exitField(pure!,'asn_bot_percent')}%`:'—',ipKind}
}
