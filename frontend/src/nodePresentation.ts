import {key,type Airport,type NodeOption} from './domain'

export interface NodeSource {airportId:string;airportName:string;subscriptionId:string;subscriptionName:string}
export type NodeSort='region'|'airport'|'name'|'latency'|'download'|'service'
const names=new Intl.Collator('zh-CN',{numeric:true,sensitivity:'base'})

export function nodeSourceIndex(airports:Airport[]):Map<string,NodeSource>{
  const sources=new Map<string,NodeSource>()
  for(const airport of airports){
    sources.set(airport.id,{airportId:airport.id,airportName:airport.name,subscriptionId:airport.id,subscriptionName:'默认订阅'})
    for(const subscription of airport.subscriptions||[])sources.set(subscription.id,{airportId:airport.id,airportName:airport.name,subscriptionId:subscription.id,subscriptionName:subscription.name})
  }
  return sources
}

export function sourceForNode(node:NodeOption,sources:Map<string,NodeSource>):NodeSource{
  return sources.get(node.profile_id)||{airportId:node.profile_id,airportName:node.profile_name,subscriptionId:node.profile_id,subscriptionName:node.profile_name}
}

// Combined browsing can collapse a shared endpoint across accounts; account
// histories and explicitly selected measurement scopes remain independent.
export function uniqueAirportNodes(nodes:NodeOption[],sources:Map<string,NodeSource>):NodeOption[]{
  const seen=new Set<string>()
  return nodes.filter(n=>{
    if(!n.node_identity_key)return true
    const identity=JSON.stringify([sourceForNode(n,sources).airportId,n.node_identity_key,n.display_name.trim().toLowerCase()])
    if(seen.has(identity))return false
    seen.add(identity);return true
  })
}

export function nodeRegion(node:NodeOption){const code=node.country_code?.trim().toUpperCase();return /^[A-Z]{2}$/.test(code||'')&&!['XX','ZZ'].includes(code)?code:'unknown'}

const regionLabels:Record<string,string>={HK:'香港',MO:'澳门',TW:'台湾',JP:'日本',SG:'新加坡',US:'美国',KR:'韩国',UK:'英国',GB:'英国',DE:'德国',FR:'法国',NL:'荷兰',AU:'澳大利亚',CA:'加拿大',RU:'俄罗斯',TH:'泰国',MY:'马来西亚',VN:'越南',PH:'菲律宾',ID:'印尼',IN:'印度',TR:'土耳其',AE:'阿联酋',CN:'中国',AR:'阿根廷',BR:'巴西',IT:'意大利',ES:'西班牙',SE:'瑞典',CH:'瑞士',PL:'波兰',ZA:'南非',KH:'柬埔寨',EG:'埃及',NG:'尼日利亚',PK:'巴基斯坦'}
const nameRegions=Object.entries(regionLabels).filter(([code])=>code!=='GB').sort((a,b)=>b[1].length-a[1].length)
export function nodeRegionLabel(code:string){return regionLabels[code]||(['OTHER','unknown','XX','ZZ'].includes(code)?'未识别地区':code)}

const protocolClasses:Record<string,string>={vmess:'proto-vmess',vless:'proto-vless',trojan:'proto-trojan',anytls:'proto-anytls',ss:'proto-ss',shadowsocks:'proto-ss',ssr:'proto-ssr',shadowsocksr:'proto-ssr',hysteria2:'proto-hy2',hysteria:'proto-hy2',hy2:'proto-hy2',tuic:'proto-tuic',wireguard:'proto-wg',wg:'proto-wg'}
export function protocolBadgeClass(type?:string){return protocolClasses[(type||'').trim().toLowerCase()]||'proto-other'}

const regionClasses:Record<string,string>={HK:'region-hk',TW:'region-tw',JP:'region-jp',SG:'region-sg',KR:'region-kr',US:'region-us',UK:'region-uk',GB:'region-uk',DE:'region-eu',FR:'region-eu',NL:'region-eu',CH:'region-eu',SE:'region-eu',IT:'region-eu',ES:'region-eu',PL:'region-eu',CA:'region-ca',AU:'region-ca',RU:'region-eu'}
export function regionBadgeClass(code?:string){const c=(code||'').trim().toUpperCase();if(!c||c==='UNKNOWN'||c==='XX'||c==='ZZ'||c==='—')return 'region-unknown';return regionClasses[c]||'region-other'}

export type NodeDisplayPartKind='num'|'line'|'media'|'proto'|'mult'|'attr'
export interface NodeDisplayParts{region:string;parts:{text:string;kind:NodeDisplayPartKind}[]}
function classifyNodePart(part:string):NodeDisplayPartKind{
  if(/^\d+(?:\.\d+)?×$/i.test(part))return 'mult'
  if(/^(?:[A-Z]{1,2}\d+|\d{1,3}(?:（[²³]）)?)$/i.test(part))return 'num'
  if(/^(?:hy2|vless|vmess|anytls|trojan|tuic|ss|ssr|wg|ipv?[46]|v[46]入口)$/i.test(part))return 'proto'
  if(/(?:^ai$|chatgpt|claude|gemini|grok|copilot|流媒体|netflix|youtube|disney|tiktok|spotify|hulu|abema|dmm|unext|google\s*play)/i.test(part))return 'media'
  if(/(?:家宽|住宅|原生|专线|优化|直连|iplc|iepl|cn2|gia|bgp|^aws$|^gcp$|azure|oracle)/i.test(part))return 'line'
  return 'attr'
}
export function nodeDisplayParts(node:Pick<NodeOption,'display_name'>&Partial<Pick<NodeOption,'country_code'>>):NodeDisplayParts{
  const segments=nodeDisplayName(node).split(' · ')
  return {region:segments[0]||'',parts:segments.slice(1).map(text=>({text,kind:classifyNodePart(text)}))}
}

export interface NodeAttrHint{term:string;short:string;detail:string}
const attrRules:[RegExp,string,string][]=[
  [/^高速$/,'高优先级/高带宽','机场标注的高带宽或高优先级线路（仍以实测速度为准）'],
  [/^移动优化$/,'移动线路优化','针对中国移动网络路径做过回程或接入优化'],
  [/^电信优化$/,'电信线路优化','针对中国电信网络路径做过回程或接入优化'],
  [/^联通优化$/,'联通线路优化','针对中国联通网络路径做过回程或接入优化'],
  [/^优化$/,'路由优化','针对国内访客做过接入或回程路由优化'],
  [/^直连$/,'直连链路','未绕转多层中转的直接连接节点（实际路径以出口检测为准）'],
  [/^专线$/,'跨境专线','专线接入链路，通常延迟更稳、高峰丢包更少'],
  [/^iplc$/i,'IPLC 专线','国际私有租用专线，过境独立传输，延迟低且稳定'],
  [/^iepl$/i,'IEPL 专线','国际以太网专线，高优先级跨境低延迟专线'],
  [/^gia$/i,'CN2 GIA 优线','中国电信最高级别 CN2 双向优化路由，晚高峰表现稳'],
  [/^cn2$/i,'电信 CN2 线路','中国电信下一代承载网优化线路'],
  [/^bgp$/i,'多线智能路由','BGP 多线接入，可动态选择较优运营商出口'],
  [/^cmcu$/i,'移动/联通双线','同时针对中国移动（CM）与中国联通（CU）直连/优化'],
  [/^cmi$/i,'移动国际 CMI','中国移动国际骨干网优化路由'],
  [/^cmcc$/i,'移动线路','中国移动接入或优化线路'],
  [/^wap$/i,'特定蜂窝/分流入口','通常表示移动蜂窝网络适配或特定分流入口节点'],
  [/^aws$/i,'亚马逊云机房','托管于 Amazon Web Services 云机房'],
  [/^gcp$/i,'谷歌云机房','托管于 Google Cloud 云机房'],
  [/^azure$/i,'微软云机房','托管于 Microsoft Azure 云机房'],
  [/^oracle$/i,'甲骨文云机房','托管于 Oracle Cloud 云机房'],
  [/^hy2$/i,'Hysteria2 协议','基于 QUIC 的低延迟抗丢包传输协议'],
  [/^v6入口$|^ipv?6$/i,'IPv6 接入','使用 IPv6 入口建立连接（需本机网络支持 IPv6）'],
  [/^家宽$|^住宅(?:ip)?$/i,'住宅/家宽 IP','接近真实宽带用户的 ISP 出口，风控更友好'],
  [/^原生$/,'原生地区 IP','IP 注册归属与实际地区一致，适合流媒体与低风控场景'],
  [/流媒体/,'声明解锁流媒体','机场名称标注支持流媒体解锁（以实际检测为准）'],
  [/^(?:ai|gpt⁺?)$/i,'声明支持 AI','机场名称标注支持 AI 服务（以实际检测为准）'],
  [/^(\d+(?:\.\d+)?)×$/,'流量计费倍率','该节点消耗流量按标注倍率扣减套餐额度'],
]
export function nodeAttrMeaning(part:string):NodeAttrHint|null{
  const trimmed=part.trim()
  if(!trimmed)return null
  for(const [re,short,detail] of attrRules){
    const m=trimmed.match(re)
    if(m)return {term:trimmed,short:trimmed.endsWith('×')?`${m[1]} 倍扣流量`:short,detail:trimmed.endsWith('×')?`使用该节点会按 ${m[1]} 倍扣减套餐流量`:detail}
  }
  return null
}
export function nodeAttrHints(node:Pick<NodeOption,'display_name'>&Partial<Pick<NodeOption,'country_code'>>):NodeAttrHint[]{
  const seen=new Set<string>(),out:NodeAttrHint[]=[]
  for(const p of nodeDisplayParts(node).parts){
    const h=nodeAttrMeaning(p.text)
    if(h&&!seen.has(h.term)){seen.add(h.term);out.push(h)}
  }
  return out
}

// Presentation only: names used by subscriptions, identity, exports and history stay intact.
// Unknown attributes and markers are retained; missing numbers/multipliers are never invented.
export function nodeDisplayName(node:Pick<NodeOption,'display_name'>&Partial<Pick<NodeOption,'country_code'>>){
  const raw=node.display_name.trim(),flag=raw.match(/^([\u{1F1E6}-\u{1F1FF}]{2})/u)?.[1]
  const flagCode=flag?[...flag].map(c=>String.fromCharCode(c.codePointAt(0)!-0x1F1E6+65)).join(''):''
  let body=raw.replace(/^(?:[\u{1F1E6}-\u{1F1FF}]{2}\uFE0F?\s*)+/u,'')
    .replace(/[【\[](?:亚洲|欧洲|北美洲|南美洲|非洲|大洋洲)[】\]]/g,'')
    .replace(/[\[\]【】✨]/g,' ').trim()
  const namedRegion=nameRegions.find(([,label])=>body.includes(label))
  const prefixCode=body.match(/^([A-Z]{2})(?=[²³\d\s_-])/i)?.[1].toUpperCase()
  const supplied=node.country_code?.toUpperCase()
  let code=(supplied&&regionLabels[supplied]?supplied:namedRegion?.[0]||flagCode||prefixCode||'unknown')
  if(code==='GB')code='UK'
  const label=nodeRegionLabel(code),regionText=namedRegion?.[0]===code?namedRegion[1]:regionLabels[code]
  const multipliers:string[]=[]
  body=body.replace(/(\d+(?:\.\d+)?)\s*(?:x|×|倍(?:率)?)(?=$|[\s|丨｜·-])/gi,(_,amount)=>{multipliers.push(`${amount}×`);return ' '})
  let number=''
  if(regionText&&body.includes(regionText)){
    const at=body.indexOf(regionText),after=body.slice(at+regionText.length),ordinal=after.match(/^\s*(\d{1,3})(?=$|[\s|丨｜·_-]|aws\b|v\d\b)/i)
    if(ordinal)number=ordinal[1].padStart(2,'0')
    body=body.slice(0,at)+after.slice(ordinal?.[0].length||0)
  }
  const codePrefix=body.match(/^([A-Z]{2})([²³]?)[\s_-]*(\d{1,3})?(?![\d.])/i)
  if(codePrefix&&(codePrefix[1].toUpperCase()===code||code==='UK'&&codePrefix[1].toUpperCase()==='GB')){
    if(!number&&codePrefix[3])number=codePrefix[3].padStart(2,'0')+(codePrefix[2]?`（${codePrefix[2]}）`:'')
    body=body.slice(codePrefix[0].length)
  }
  if(!number)body=body.replace(/(专线|高速|优化|直连)\s*(\d{1,3})(?![\d.a-z×倍])/i,(_,feature,digits)=>{number=digits.padStart(2,'0');return feature})
  const attributes=body.replace(/([\u3400-\u9FFF])-/g,'$1|').replace(/-(?=[\u3400-\u9FFF])/g,'|')
    .split(/[|丨｜_·]+/).map(s=>s.trim().replace(/^[\s-]+|[\s-]+$/g,'').replace(/\s+/g,' '))
    .filter(Boolean).map(s=>/^hy2$/i.test(s)?'HY2':/^aws$/i.test(s)?'AWS':s)
  return [label,number,...new Set(attributes),...new Set(multipliers)].filter(Boolean).join(' · ')
}

export function compareNodes(a:NodeOption,b:NodeOption,sources:Map<string,NodeSource>,sort:string,priorityAirportId='all',metric:(node:NodeOption)=>number|null=()=>null):number{
  const sourceA=sourceForNode(a,sources),sourceB=sourceForNode(b,sources)
  const regionA=nodeRegion(a),regionB=nodeRegion(b)
  const byRegion=Number(regionA==='unknown')-Number(regionB==='unknown')||(regionA==='unknown'?0:names.compare(regionA,regionB))
  const byAirport=names.compare(sourceA.airportName,sourceB.airportName)||names.compare(sourceA.airportId,sourceB.airportId)
  const bySubscription=names.compare(sourceA.subscriptionName,sourceB.subscriptionName)||names.compare(sourceA.subscriptionId,sourceB.subscriptionId)
  const byName=names.compare(nodeDisplayName(a),nodeDisplayName(b))
  const stable=names.compare(key(a),key(b))
  if(sort==='airport')return Number(sourceA.airportId!==priorityAirportId)-Number(sourceB.airportId!==priorityAirportId)||byAirport||byRegion||bySubscription||byName||stable
  if(sort==='name')return byName||byRegion||byAirport||bySubscription||stable
  if(['latency','download','service'].includes(sort)){
    const valueA=metric(a),valueB=metric(b)
    return Number(valueA===null)-Number(valueB===null)||(valueA!==null&&valueB!==null?(sort==='latency'?valueA-valueB:valueB-valueA):0)||byRegion||byAirport||bySubscription||byName||stable
  }
  return byRegion||byAirport||bySubscription||byName||stable
}
