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

export function nodeRegion(node:NodeOption){const code=node.country_code?.trim().toUpperCase();return /^[A-Z]{2}$/.test(code||'')&&!['XX','ZZ'].includes(code)?code:'unknown'}

const regionLabels:Record<string,string>={HK:'香港',MO:'澳门',TW:'台湾',JP:'日本',SG:'新加坡',US:'美国',KR:'韩国',UK:'英国',GB:'英国',DE:'德国',FR:'法国',NL:'荷兰',AU:'澳大利亚',CA:'加拿大',RU:'俄罗斯',TH:'泰国',MY:'马来西亚',VN:'越南',PH:'菲律宾',ID:'印尼',IN:'印度',TR:'土耳其',AE:'阿联酋',CN:'中国',AR:'阿根廷',BR:'巴西',IT:'意大利',ES:'西班牙',SE:'瑞典',CH:'瑞士',PL:'波兰',ZA:'南非',KH:'柬埔寨',EG:'埃及',NG:'尼日利亚',PK:'巴基斯坦'}
const nameRegions=Object.entries(regionLabels).filter(([code])=>code!=='GB').sort((a,b)=>b[1].length-a[1].length)
export function nodeRegionLabel(code:string){return regionLabels[code]||(['OTHER','unknown','XX','ZZ'].includes(code)?'未识别地区':code)}

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
