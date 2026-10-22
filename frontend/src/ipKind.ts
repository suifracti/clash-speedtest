import type {Attempt} from './domain'
import {latestServices} from './nodeQuality'
import {exitField} from './exitSummaryPresentation'

// Presentation-only classification from already saved IP profile results.
// IPPure exposes isResidential/isBroadcast, Ping0 exposes isidc, IPOK exposes
// ipType. None of them classifies mobile networks, and none exposes an ASN
// type, so those dimensions stay unknown instead of being guessed.
export type IpType='idc'|'residential'|'mobile'|'unknown'
export type IpOrigin='native'|'broadcast'|'unknown'
export interface IpKindVote {provider:string;type:IpType;origin:IpOrigin;asn:string;organization:string}
export interface IpKind {
 type:IpType;origin:IpOrigin;asn:string;organization:string;
 sources:string[];typeSources:string[];originSources:string[];
 tag:string;tone:'idc'|'good'|'warn'|'empty';typeLabel:string;originLabel:string;
 detail:string;measured:boolean;
}

const providerOrder=['IPPure','Ping0','IPOK']
const typeLabel:Record<IpType,string>={idc:'机房',residential:'家宽',mobile:'移动',unknown:'未知'}
const originLabel:Record<IpOrigin,string>={native:'原生',broadcast:'广播',unknown:'未知'}
const sourceIds:[string,string[]][]=[
 ['IPPure',['ippure_ip_quality']],
 ['Ping0',['ping0_ipv4','ping0_ipv6','ping0_ip_quality']],
 ['IPOK',['ipok_ipv4','ipok_ipv6']],
]

function serviceId(a:Attempt){return a.service_id||a.rule.service_id||''}
function saved(a?:Attempt){return !!a&&a.result?.outcome==='profiled'}
function detailText(a:Attempt,key:string){
 const value=a.result?.details?.[key]
 if(typeof value==='boolean')return String(value)
 if(typeof value==='number'&&Number.isFinite(value))return String(value)
 return typeof value==='string'?value.trim():''
}

export function ipTypeFromText(raw:string):IpType {
 const value=String(raw||'').trim()
 if(!value||/^(?:未提供|未知|unknown|null|undefined)$/i.test(value))return 'unknown'
 // "非住宅" and "非 IDC" are explicit non-answers for the four-way split.
 if(/^非|not\s+(?:idc|residential)|non-?(?:idc|residential)/i.test(value))return 'unknown'
 if(/移动|mobile|cellular/i.test(value))return 'mobile'
 if(/住宅|residential/i.test(value))return 'residential'
 if(/机房|idc|hosting|datacenter|data\s*center/i.test(value))return 'idc'
 return 'unknown'
}

export function ipOriginFromAttempt(a:Attempt):IpOrigin {
 const raw=exitField(a,'origin_type','ip_origin','native_type').trim()
 if(/广播|broadcast/i.test(raw))return 'broadcast'
 if(/非原生|non-?native/i.test(raw))return 'broadcast'
 if(/原生|native/i.test(raw))return 'native'
 const broadcast=detailText(a,'is_broadcast')||detailText(a,'isBroadcast')
 if(broadcast)return broadcast.toLowerCase()==='true'?'broadcast':'native'
 const native=detailText(a,'is_native')||detailText(a,'isNative')
 if(native)return native.toLowerCase()==='true'?'native':'broadcast'
 // IP 注册国 vs 实际使用国：不同即广播。Only sources that expose both codes vote.
 const registered=exitField(a,'registered_country_code','registration_country_code','ip_registered_country_code').trim().toUpperCase()
 const used=exitField(a,'country_code','countryCode','location_country_code').trim().toUpperCase()
 if(registered&&used&&/^[A-Z]{2}$/.test(registered)&&/^[A-Z]{2}$/.test(used))return registered===used?'native':'broadcast'
 return 'unknown'
}

function majority<T extends string>(votes:T[]):T|'unknown' {
 if(!votes.length)return 'unknown'
 const counts=new Map<T,number>()
 for(const vote of votes)counts.set(vote,(counts.get(vote)||0)+1)
 const max=Math.max(...counts.values())
 const winners=[...counts].filter(([,count])=>count===max)
 return winners.length===1?winners[0][0]:'unknown'
}

export function shortOrganization(value:string){
 const text=String(value||'').trim()
 if(!text)return ''
 if(/amazon|\baws\b/i.test(text))return 'AWS'
 if(/fdcservers/i.test(text))return 'FDCservers'
 if(/gsl\s*networks/i.test(text))return 'GSL'
 if(/digitalocean/i.test(text))return 'DigitalOcean'
 if(/cloudflare/i.test(text))return 'Cloudflare'
 if(/google/i.test(text))return 'Google'
 if(/microsoft|azure/i.test(text))return 'Azure'
 if(/oracle/i.test(text))return 'Oracle'
 if(/alibaba|aliyun/i.test(text))return '阿里云'
 if(/tencent/i.test(text))return '腾讯云'
 return text.split(/[,，]/)[0].replace(/\s+(?:Inc|Ltd|LLC|Pty|Corp|Corporation|Company|Co)\.?$/i,'').slice(0,20)
}

export function ipKindFromLatest(latest:Attempt[]):IpKind {
 const votes:IpKindVote[]=[]
 for(const [provider,ids] of sourceIds){
  const pool=ids.map(id=>latest.find(a=>serviceId(a)===id&&saved(a))).filter((a):a is Attempt=>!!a)
  if(!pool.length)continue
  // Prefer a record that actually carries a type or origin signal; IPv4 first.
  const typed=pool.find(a=>ipTypeFromText(exitField(a,'ip_type','ipType','ip_network_type'))!=='unknown')||pool[0]
  const originRecord=pool.find(a=>ipOriginFromAttempt(a)!=='unknown')||typed
  votes.push({
   provider,
   type:ipTypeFromText(exitField(typed,'ip_type','ipType','ip_network_type')),
   origin:ipOriginFromAttempt(originRecord),
   asn:exitField(typed,'asn')||exitField(originRecord,'asn'),
   organization:exitField(typed,'organization','org','isp')||exitField(originRecord,'organization','org','isp'),
  })
 }
 const typeVotes=votes.filter(v=>v.type!=='unknown'),originVotes=votes.filter(v=>v.origin!=='unknown')
 const type=majority(typeVotes.map(v=>v.type)) as IpType
 const origin=majority(originVotes.map(v=>v.origin)) as IpOrigin
 const contributing=votes.filter(v=>v.type!=='unknown'||v.origin!=='unknown')
 const sources=providerOrder.filter(name=>contributing.some(v=>v.provider===name))
 const typeSources=providerOrder.filter(name=>typeVotes.some(v=>v.provider===name))
 const originSources=providerOrder.filter(name=>originVotes.some(v=>v.provider===name))
 const orgRecord=contributing.find(v=>v.organization)||votes.find(v=>v.organization)
 const organization=shortOrganization(orgRecord?.organization||'')
 const asn=orgRecord?.asn||''
 const tag=type==='unknown'&&origin==='unknown'?'未知':`${typeLabel[type]}·${originLabel[origin]}`
 const tone=origin==='broadcast'?'warn':type==='residential'?'good':type==='idc'?'idc':'empty'
 const typeText=type==='unknown'?'未知':typeLabel[type]+(organization?`（${organization}）`:'')
 const originText=origin==='unknown'?'原生未知':originLabel[origin]
 const detail=contributing.length?`${typeText} · ${originText} · 来源 ${sources.join('、')}`:'未测（IPPure / Ping0 / IPOK 没有类型记录）'
 return {type,origin,asn,organization:orgRecord?.organization||'',sources,typeSources,originSources,tag,tone,typeLabel:typeLabel[type],originLabel:originLabel[origin],detail,measured:contributing.length>0}
}

export function ipKindOf(attempts:Attempt[]):IpKind {
 return ipKindFromLatest(latestServices(attempts))
}
