import {isNotice,type Airport,type NodeOption,type Subscription} from './domain'
import {nodeSourceIndex,sourceForNode} from './nodePresentation'

export interface AirportAccount {airport:Airport;subscription:Subscription;key:string}

export function airportNodeCounts(nodes:NodeOption[],airports:Airport[]):Record<string,number>{
  const sources=nodeSourceIndex(airports),known=new Set(airports.map(airport=>airport.id)),counts:Record<string,number>={}
  for(const node of nodes){
    if(isNotice(node))continue
    const id=sourceForNode(node,sources).airportId
    if(known.has(id))counts[id]=(counts[id]||0)+1
  }
  return counts
}

export function airportAccounts(airports:Airport[]):AirportAccount[]{
  const accounts:AirportAccount[]=[]
  for(const airport of airports){
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

