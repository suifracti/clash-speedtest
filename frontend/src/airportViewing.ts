import type {Airport,NodeOption,Subscription} from './domain'
import {nodeSourceIndex,sourceForNode} from './nodePresentation'

export interface AirportAccount {airport:Airport;subscription:Subscription;key:string}

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
