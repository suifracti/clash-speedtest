import {type Attempt,type NodeOption} from './domain'
import {attemptObservedAt,latestServices} from './nodeQuality'
import {exitField} from './exitSummaryPresentation'
import {nodeRegionLabel} from './nodePresentation'
const codes='CN HK MO TW JP SG US KR GB DE FR NL AU CA RU TH MY VN PH ID IN TR AE AR BR IT ES SE CH PL ZA KH EG NG PK'.split(' ')
const english=new Intl.DisplayNames(['en'],{type:'region'})
function country(value:string){const v=value.trim().toUpperCase();if(/^[A-Z]{2}$/.test(v))return v==='UK'?'GB':v;return codes.find(c=>[nodeRegionLabel(c),english.of(c)].some(name=>name?.toUpperCase()===v))||(/中国大陆|中国|MAINLAND CHINA/i.test(value)?'CN':'')}
export function maskedIP(value:string){if(value.includes(':')){const p=value.split(':');return p[0]+':…:'+p.at(-1)}const p=value.split('.');return p.length===4?`${p[0]}.*.*.${p[3]}`:value||'—'}
export function exitLanding(node:Pick<NodeOption,'country_code'>,attempts:Attempt[]){
 const latest=latestServices(attempts),find=(id:string)=>latest.find(a=>(a.service_id||a.rule.service_id)===id),pure=find('ippure_ip_quality'),ping=find('ping0_ipv4')||find('ping0_ip_quality'),cf=find('cloudflare_trace')
 const valid=(a?:Attempt)=>a?.result?.outcome==='profiled'
 const ipRecord=[find('exit_ipv4'),pure,ping,cf,find('exit_ipv6')].find(a=>valid(a)&&exitField(a!,'ip'))
 const library=[pure,ping,find('ping0_ipv6')].filter(a=>valid(a)).map(a=>country(exitField(a!,'country_code','country','location'))).filter(Boolean)
 const google=latest.filter(a=>['google_region','youtube_premium','gemini_unlock','gemini_web'].includes(a.service_id||a.rule.service_id||'')).map(a=>({a,region:country(exitField(a,'service_region'))})).filter(s=>s.region).sort((a,b)=>Date.parse(attemptObservedAt(b.a))-Date.parse(attemptObservedAt(a.a)))[0]?.region||''
 const sources=[...library.map((region,i)=>({name:i?'IP 库（补充）':'IP 库',region})),{name:'Cloudflare',region:valid(cf)?country(exitField(cf!,'country_code')):''},{name:'Google',region:google}],known=sources.filter(s=>s.region),expected=country(node.country_code),mismatch=known.find(s=>expected&&s.region!==expected),mainland=known.find(s=>s.region==='CN')
 const count=Number(!!library.length)+Number(!!sources.find(s=>s.name==='Cloudflare')?.region)+Number(!!google)
 const label=mainland?`${mainland.name} 判定 CN（中国大陆）`:mismatch?`名称 ${expected}，${mismatch.name} 判定 ${mismatch.region}`:count===3&&expected?'落地一致':`待补来源（${count}/3）`
 const field=(a:Attempt|undefined,...keys:string[])=>a&&valid(a)?exitField(a,...keys)||'—':'—'
 return {ip:maskedIP(ipRecord?exitField(ipRecord,'ip'):''),country:library[0]||'—',label,tone:mainland?'bad':mismatch?'warn':count===3&&expected?'good':'empty',detail:`名称 ${expected||'未识别'} · `+sources.map(s=>s.name+' '+(s.region||'未取得')).join(' · '),pure:field(pure,'risk_score','fraud_score'),ping:[field(ping,'risk_score'),field(find('ping0_ipv6'),'risk_score')].join(' / '),cf:field(pure,'cf_risk_score'),traffic:valid(pure)&&exitField(pure!,'asn_human_percent')&&exitField(pure!,'asn_bot_percent')?`${exitField(pure!,'asn_human_percent')}% / ${exitField(pure!,'asn_bot_percent')}%`:'—'}
}
