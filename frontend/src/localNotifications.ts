import {onBeforeUnmount,watch} from 'vue'
import {api,desktop,request} from './api'
import {subscriptionUsed,type Airport,type LatencyBatch,type MonitorSample,type NotificationSettings} from './domain'
import type {Workspace} from './workspace'
export interface HealthObservation {airportId:string;at:string;failed:number;total:number;source:string}
export interface LocalAlert {id:string;title:string;body:string}
export function localAlerts(airports:Airport[],settings:NotificationSettings,health:HealthObservation[],now=Date.now()):LocalAlert[]{
 if(!settings.enabled)return []
 const alerts:LocalAlert[]=[]
 for(const a of airports){
  const latest=health.filter(h=>h.airportId===a.id).sort((x,y)=>Date.parse(y.at)-Date.parse(x.at))[0]
  if(latest&&latest.total&&latest.failed/latest.total*100>=settings.failure_percent)alerts.push({id:'failure:'+a.id,title:a.name+' · 大面积无响应',body:`${latest.source}：${latest.failed}/${latest.total} 个已记录节点无响应`})
  for(const s of a.subscriptions||[]){
   const used=subscriptionUsed(s.usage),total=s.usage?.total
   if(used!==null&&total&&Math.max(0,total-used)/total*100<settings.remaining_percent)alerts.push({id:'quota:'+s.id,title:a.name+' · 剩余流量较低',body:`${s.name} 剩余 ${(Math.max(0,total-used)/total*100).toFixed(1)}%`})
   const expire=s.usage?.expire
   if(expire&&Math.ceil((expire*1000-now)/86400000)<=settings.expiry_days)alerts.push({id:'expiry:'+s.id+':'+expire,title:a.name+(expire*1000<now?' · 套餐已到期':' · 套餐即将到期'),body:`${s.name} · ${new Date(expire*1000).toLocaleDateString('zh-CN')} 到期`})
  }
 }
 return alerts
}
export function useLocalNotifications(w:Workspace){
 const cache=new Map<string,LatencyBatch>();let seen=new Set<string>()
 try{seen=new Set(JSON.parse(localStorage.getItem('cst-notified-alerts')||'[]'))}catch{}
 async function check(){
  const settings=w.settings.value?.local_notifications;if(!desktop()||!settings?.enabled)return
  try{
   const health:HealthObservation[]=[],batches=await Promise.all(w.batches.value.filter(b=>b.state.startsWith('completed')).map(async b=>{const detail=b.items?b:cache.get(b.batch_id)||await api.batch(b.batch_id);cache.set(b.batch_id,detail);return detail}))
   for(const a of w.airports.value){const ids=a.subscriptions?.map(s=>s.id)||[a.id],batch=batches.find(b=>b.items?.some(i=>ids.includes(i.profile_id))),items=batch?.items?.filter(i=>ids.includes(i.profile_id)&&i.result&&i.result.total_samples>0)||[];if(batch&&items.length)health.push({airportId:a.id,at:batch.requested_at,total:items.length,failed:items.filter(i=>i.result!.success_samples===0).length,source:'最近延迟批次'})}
   for(const job of w.jobs.value){
    const runs=await request<{run_id:string;finished_at?:string}[]>('','GET',undefined,'QueryMonitorRuns',[job.id,2]);const latest=runs?.find(r=>r.finished_at),run=latest?.run_id;if(!run)continue
    const samples=await request<MonitorSample[]>('','GET',undefined,'QueryMonitorSamples',[{JobID:job.id,RunID:run,ProbeType:'rtt',Limit:10000}])
    for(const a of w.airports.value){const ids=a.subscriptions?.map(s=>s.id)||[a.id],nodes=new Map<string,boolean>();for(const s of samples||[])if(ids.includes(s.profile_id)&&s.error_class!=='not_executed'&&s.error_class!=='cancelled')nodes.set(s.profile_id+'|'+s.node_identity_key,!!nodes.get(s.profile_id+'|'+s.node_identity_key)||s.success);if(nodes.size)health.push({airportId:a.id,at:latest!.finished_at!,total:nodes.size,failed:[...nodes.values()].filter(v=>!v).length,source:'最近监测轮次'})}
   }
   if(!w.settings.value?.local_notifications?.enabled)return
   const alerts=localAlerts(w.airports.value,settings,health),active=new Set(alerts.map(a=>a.id));seen=new Set([...seen].filter(id=>active.has(id)))
   for(const alert of alerts)if(!seen.has(alert.id)){await request('','POST',undefined,'SendLocalNotification',[alert.id,alert.title,alert.body]);seen.add(alert.id);localStorage.setItem('cst-notified-alerts',JSON.stringify([...seen]))}
   localStorage.setItem('cst-notified-alerts',JSON.stringify([...seen]))
  }catch(e){w.fail(e)}
 }
 const stop=watch(w.settings,()=>void check(),{immediate:true}),timer=setInterval(()=>void check(),300_000)
 onBeforeUnmount(()=>{stop();clearInterval(timer)})
}
