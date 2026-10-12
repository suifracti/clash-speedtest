import {ref} from 'vue'
import {api,mapLimited} from './api'
import {key,scope,type Attempt,type NodeOption} from './domain'
import {monitorLatencyPoints,points,type TrendPoint} from './presentation'
export interface SortResult {download?:Attempt;services:Attempt[];latency?:TrendPoint}
// All reads share a cutoff and identity/revision scope. History paging never mutates this snapshot.
export async function readSortSnapshot(nodes:NodeOption[],mode:string,target:string,ids:string[],since:string,until:string,onRead:()=>void){
 const snapshot:Record<string,SortResult>={},errors:string[]=[]
 await mapLimited(nodes,4,async n=>{try{
  const entry:SortResult={services:[]}
  if(mode==='download')entry.download=(await api.attemptHistory('download',n,{since,until,limit:1})).attempts?.[0]
  else if(mode==='service')for(const id of ids)entry.services.push(...((await api.attemptHistory('service',n,{since,until,limit:1,service_id:id})).attempts||[]))
  else {
   const manual=await api.latencyHistory(n,{since,until,limit:1,target_id:target})
   const urls:Record<string,string>={cloudflare:'https://speed.cloudflare.com/__down?bytes=1',google:'https://www.gstatic.com/generate_204',github:'https://api.github.com/zen',apple:'https://captive.apple.com/hotspot-detect.html',microsoft:'http://www.msftconnecttest.com/connecttest.txt',firefox:'https://detectportal.firefox.com/success.txt'}
   const monitor=await api.monitorSamples({...scope(n),probe_type:'rtt',target:target==='cloudflare'?undefined:urls[target],since,until,limit:480,order_desc:true})
   const projected=points({latency:ref({[key(n)]:manual.tests||[]}),downloads:ref({}),services:ref({}),serviceIds:ref(ids),displayTarget:ref(target)},n,'latency',target)
   entry.latency=[...projected,...monitorLatencyPoints(monitor.items||[],target)].sort((a,b)=>Date.parse(a.time)-Date.parse(b.time)).at(-1)
  }
  snapshot[key(n)]=entry;onRead()
 }catch(e){errors.push(n.display_name+': '+(e instanceof Error?e.message:String(e)))}})
 return {snapshot,errors}
}
