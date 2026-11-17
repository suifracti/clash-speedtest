import {isNotice,type Airport,type NodeOption,type Subscription} from './domain'
import {nodeSourceIndex,sourceForNode,uniqueAirportNodes} from './nodePresentation'

export interface AirportAccount {airport:Airport;subscription:Subscription;key:string}

export function airportNodeCount(airport:Airport|undefined,nodes?:NodeOption[]):number{
  if(!airport)return 0
  if(nodes){const sources=nodeSourceIndex([airport]);return uniqueAirportNodes(nodes.filter(n=>sourceForNode(n,sources).airportId===airport.id&&!isNotice(n)),sources).length}
  const cachedCounts=(airport.subscriptions||[]).filter(subscription=>subscription.has_cache).map(subscription=>subscription.node_count||0)
  return cachedCounts.length?Math.max(...cachedCounts):airport.node_count||0
}

export function airportNodeCounts(airports:Airport[],nodes?:NodeOption[]):Record<string,number>{
  const counts:Record<string,number>={}
  for(const airport of airports){
    const count=airportNodeCount(airport,nodes)
    if(count>0)counts[airport.id]=count
  }
  return counts
}

export function airportAccounts(airports:Airport[]):AirportAccount[]{
  const accounts:AirportAccount[]=[]
  for(const airport of airports){
    if(!airport.subscriptions?.length&&!airport.url_display&&!airport.has_cache)continue
    const subscriptions=airport.subscriptions?.length?airport.subscriptions:[{
      id:airport.id,airport_id:airport.id,name:'默认订阅',url_display:airport.url_display,
      url_configured:!!airport.url_display,node_count:airport.node_count,has_cache:airport.has_cache,
      updated_at:airport.updated_at,
    }]
    for(const subscription of subscriptions)accounts.push({airport,subscription,key:JSON.stringify([airport.id,subscription.id])})
  }
  return accounts
}

export function noticesForAccount(notices:NodeOption[],account:AirportAccount|undefined):NodeOption[]{
  if(!account)return []
  const sources=nodeSourceIndex([account.airport])
  return notices.filter(node=>{
    const source=sourceForNode(node,sources)
    return source.airportId===account.airport.id&&source.subscriptionId===account.subscription.id
  })
}

export function safeAirportLink(value?:string):string|undefined{
  if(!value)return undefined
  try{const link=new URL(value);return link.protocol==='http:'||link.protocol==='https:'?link.href:undefined}catch{return undefined}
}

// Read only cached notice text. Saving one of these shortcuts never visits it.
export function maintenanceNoticeLinks(airport:Airport,notices:NodeOption[]):{label:string;url:string}[]{
  const saved=new Set((airport.maintenance?.links||[]).map(link=>safeAirportLink(link.url)))
  const found=new Map<string,{label:string;url:string}>()
  for(const account of airportAccounts([airport]))for(const node of noticesForAccount(notices,account)){
    const text=node.display_name||''
    const candidates=[...(text.match(/https?:\/\/[^\s，,）)\]》>]+/gi)||[]),...Array.from(text.matchAll(/(?:^|[^\w/])((?:t\.me\/[a-zA-Z0-9_+]+|github\.com\/[a-zA-Z0-9_\-/]+))/gi),match=>'https://'+match[1])]
    for(const candidate of candidates){
      const raw=candidate.replace(/[.,;:!?)]+$/g,''),url=safeAirportLink(raw)
      if(!url||saved.has(url)||found.has(url))continue
      const parsed=new URL(url);if(parsed.username||parsed.password)continue
      const label=text.replace(raw,'').replace(/[：:\s·—-]+$/g,'').trim()
      found.set(url,{url,label:label&&label.length<=80?label:parsed.hostname})
    }
  }
  return [...found.values()]
}

export function parseTrafficBytes(numStr:string,unitStr:string):number{
  const num=parseFloat(numStr)
  if(isNaN(num))return 0
  const u=unitStr.toUpperCase()
  if(u.startsWith('P'))return num*1024*1024*1024*1024*1024
  if(u.startsWith('T'))return num*1024*1024*1024*1024
  if(u.startsWith('G'))return num*1024*1024*1024
  if(u.startsWith('M'))return num*1024*1024
  if(u.startsWith('K'))return num*1024
  return num
}

export interface NoticeParsedUsage{remaining?:number;used?:number;total?:number;expireText?:string;isPerpetual?:boolean}

export function parseNoticeUsage(notices:NodeOption[]):NoticeParsedUsage{
  const res:NoticeParsedUsage={}
  for(const n of notices){
    const text=(n.display_name||'').replace(/^[#\s]+/,'').trim()
    const remMatch=text.match(/(?:距离下次重置剩余|剩余(?:可用)?(?:流量)?|可用流量)[：:\s]+([0-9.]+)\s*([A-Za-z]+)/i)
    if(remMatch)res.remaining=parseTrafficBytes(remMatch[1],remMatch[2])
    const usedMatch=text.match(/(?:已用流量|已用|本月已用|已产生流量)[：:\s]+([0-9.]+)\s*([A-Za-z]+)/i)
    if(usedMatch)res.used=parseTrafficBytes(usedMatch[1],usedMatch[2])
    const totalMatch=text.match(/(?:总流量|总计|套餐流量|总计可用)[：:\s]+([0-9.]+)\s*([A-Za-z]+)/i)
    if(totalMatch)res.total=parseTrafficBytes(totalMatch[1],totalMatch[2])
    const expMatch=text.match(/(?:套餐到期|到期时间|过期时间|会员到期|服务到期|账号到期)[：:\s]+([^\r\n]+)/i)
    if(expMatch){
      const rawExp=expMatch[1].trim()
      if(/长期有效|永久有效|不限时|永不过期/.test(rawExp)){res.expireText='长期有效';res.isPerpetual=true}
      else res.expireText=rawExp
    }
  }
  return res
}

