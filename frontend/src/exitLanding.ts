import {type Attempt,type NodeOption} from './domain'
import {attemptObservedAt,latestServices} from './nodeQuality'
import {exitField,riskScore,toExitRecord} from './exitSummaryPresentation'
import {ipKindFromLatest} from './ipKind'
import {nodeRegionLabel} from './nodePresentation'
const codes='CN HK MO TW JP SG US KR GB DE FR NL AU CA RU TH MY VN PH ID IN TR AE AR BR IT ES SE CH PL ZA KH EG NG PK'.split(' ')
const english=new Intl.DisplayNames(['en'],{type:'region'})
function country(value:string){const v=value.trim().toUpperCase();if(['XX','ZZ'].includes(v))return '';if(/香港|HONG\s*KONG/i.test(value))return 'HK';if(/澳门|MACAU|MACAO/i.test(value))return 'MO';if(/台湾|TAIWAN/i.test(value))return 'TW';if(/^[A-Z]{2}$/.test(v))return v==='UK'?'GB':v;return codes.find(c=>[nodeRegionLabel(c),english.of(c)].some(name=>name?.toUpperCase()===v))||(/中国大陆|中国|MAINLAND CHINA/i.test(value)?'CN':'')}
export function maskedIP(value:string){if(value.includes(':')){const p=value.split(':');return p[0]+':…:'+p.at(-1)}const p=value.split('.');return p.length===4?`${p[0]}.*.*.${p[3]}`:value||'—'}
export function exitLanding(node:Pick<NodeOption,'country_code'>,attempts:Attempt[]){
 const latest=latestServices(attempts),find=(id:string)=>latest.find(a=>(a.service_id||a.rule.service_id)===id),pure=find('ippure_ip_quality'),ping=find('ping0_ipv4')||find('ping0_ip_quality'),cf=find('cloudflare_trace')
 const valid=(a?:Attempt)=>a?.result?.outcome==='profiled'
 const ipRecord=[find('exit_ipv4'),pure,ping,cf,find('exit_ipv6')].find(a=>valid(a)&&exitField(a!,'ip'))
 const library=[...new Set([pure,ping,find('ping0_ipv6')].filter(a=>valid(a)).map(a=>country(exitField(a!,'country_code','country','location'))).filter(Boolean))]
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
 const reputation=[pure,ping,find('ipok_ipv4'),find('ipok_ipv6')].filter((a):a is Attempt=>!!a).map(a=>toExitRecord(a,attemptObservedAt(a))).find(r=>riskScore(r)!==undefined)
 const purityScore=reputation?Math.round(100-riskScore(reputation)!):null
 const observed=reputation?.attempt||latest.find(a=>valid(a)&&['ippure_ip_quality','ping0_ipv4','ping0_ipv6','ping0_ip_quality','ipok_ipv4','ipok_ipv6','cloudflare_trace'].includes(a.service_id||a.rule.service_id||''))
 const purityAt=observed?exitField(observed,'query_at')||attemptObservedAt(observed):''
 const typeText=ipKind.type==='idc'?'机房 IP':ipKind.type==='residential'?'住宅 IP':ipKind.type==='mobile'?'移动 IP':'类型未知'
 const originText=ipKind.origin==='native'?`原生 ${library[0]||consensusRegion||'地区未知'}`:ipKind.origin==='broadcast'?'广播 IP':'原生未知'
 const purityDate=purityAt?new Date(purityAt).toLocaleDateString('en-US',{month:'2-digit',day:'2-digit'}):''
 const purity={score:purityScore,label:purityScore===null?'—':`${purityScore} · ${ipKind.typeLabel}`,tone:purityScore===null?'empty':purityScore>=80?'good':purityScore>=50?'warn':'bad',title:reputation?`${reputation.provider}：100 − 风险分 ${riskScore(reputation)}；越高越纯净 · ${purityAt}`:'没有可换算的风险分',summary:`纯净度 ${purityScore??'—'} · ${typeText} · ${originText}`+(purityDate?` · ${purityDate} 查询`:'')}
 return {purity,consensusRegion,googleNote,region:mainland?.region||google||cfRegion||library[0]||expected,org:ipRecord?exitField(ipRecord,'organization','org','isp'):'',exitKnown:!!library.length||!!cfRegion||!!google,riskLabel:risk?risk.label+' '+Math.round(riskValue!)+(risk.label==='机器'?'%':''):'未测',riskTone:riskValue===undefined?'empty':riskValue>=70?'bad':riskValue>=30?'warn':'good',ip:maskedIP(ipRecord?exitField(ipRecord,'ip'):''),country:library.join(' / ')||'—',label,tone:mainland?'bad':mismatch?'warn':count===3&&expected?'good':'empty',tags,detail:tags.join(' · '),pure:field(pure,'risk_score','fraud_score'),ping:[field(ping,'risk_score'),field(find('ping0_ipv6'),'risk_score')].join(' / '),cf:field(pure,'cf_risk_score'),traffic:valid(pure)&&exitField(pure!,'asn_human_percent')&&exitField(pure!,'asn_bot_percent')?`${exitField(pure!,'asn_human_percent')}% / ${exitField(pure!,'asn_bot_percent')}%`:'—',ipKind}
}
