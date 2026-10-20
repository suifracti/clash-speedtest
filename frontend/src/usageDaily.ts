import {query,request} from './api'

// 机场账户流量（订阅计数）按天读取。未知就是未知：不补 0、不平摊、不把计数重置当成省流量。
export interface UsageBucket {date:string;bytes:number|null;upload:number|null;download:number|null;intervals:number;estimated_intervals:number}
export interface UsageSnapshot {account_key:string;airport_id?:string;subscription_id?:string;airport_name:string;subscription_name:string;captured_at:string;status:string;counter_kind?:string;upload:number|null;download:number|null;remaining?:number;total?:number;source:string;message?:string}
export interface UsageInterval {account_key:string;name:string;from?:string;until:string;bucket?:string;counter_kind?:string;bytes:number|null;upload:number|null;download:number|null;status:string;estimated:boolean;message?:string}
export interface UsageReport {from:string;until:string;period:string;buckets:UsageBucket[];intervals:UsageInterval[];accounts:UsageSnapshot[];observed_bytes:number|null;unallocated_bytes:number|null;unknown_intervals:number;daily_enabled:boolean;refresh?:{last_attempt:string;next_refresh:string;state:string;message:string}}
export interface DayUsage {date:string;label:string;bytes:number|null;estimated:boolean;recorded:boolean;sourceKind?:string;message?:string}

const zone='Asia/Shanghai'
export function shanghaiDay(now=new Date()){const parts=new Intl.DateTimeFormat('en-CA',{timeZone:zone,year:'numeric',month:'2-digit',day:'2-digit'}).formatToParts(now);return ['year','month','day'].map(k=>parts.find(p=>p.type===k)?.value).join('-')}
function shanghaiDayAt(value:string){const parts=new Intl.DateTimeFormat('en-CA',{timeZone:zone,year:'numeric',month:'2-digit',day:'2-digit'}).formatToParts(new Date(value));return ['year','month','day'].map(k=>parts.find(p=>p.type===k)?.value).join('-')}
export function shiftDay(day:string,delta:number){const d=new Date(day+'T00:00:00Z');d.setUTCDate(d.getUTCDate()+delta);return d.toISOString().slice(0,10)}

const cache=new Map<string,{at:number;value:Promise<UsageReport>}>()
export function readUsage(params:{from:string;until:string;period:'day'|'week'|'month';account?:string},maxAge=60_000):Promise<UsageReport>{
 const id=JSON.stringify(params),hit=cache.get(id)
 if(hit&&Date.now()-hit.at<maxAge)return hit.value
 const value=request<UsageReport>('/api/subscription-usage?'+query(params),'GET',undefined,'GetSubscriptionUsage',[params.from,params.until,params.period,params.account||''])
 cache.set(id,{at:Date.now(),value});value.catch(()=>cache.delete(id))
 return value
}
export function clearUsageCache(){cache.clear()}

/** 找到某个订阅对应的用量账户：后端只列出该范围内有记录的账户，所以用同一日期范围读账户列表。 */
export async function usageAccount(airportId:string,subscriptionId:string,from:string,until:string){
 const index=await readUsage({from,until,period:'day'})
 return index.accounts.find(a=>a.airport_id===airportId&&a.subscription_id===subscriptionId)
}

/** 按账户读取最近 days 天（含今天）的日用量；跨月由后端按日期范围处理。 */
export async function accountDailyUsage(airportId:string,subscriptionId:string,days=14,today=shanghaiDay()){
 const from=shiftDay(today,-(days-1)),account=await usageAccount(airportId,subscriptionId,from,today)
 if(!account)return {account:undefined,report:undefined,days:[] as DayUsage[]}
 const report=await readUsage({from,until:today,period:'day',account:account.account_key})
 return {account,report,days:dailySeries(report,today,days)}
}

const names=['今天','昨天','前天']
export function dailySeries(report:UsageReport,today:string,days:number):DayUsage[]{
 const byDate=new Map(report.buckets.map(b=>[b.date,b]))
 const intervalsByDate=new Map<string,UsageInterval[]>()
 for(const interval of report.intervals){const date=interval.bucket||shanghaiDayAt(interval.until);intervalsByDate.set(date,[...(intervalsByDate.get(date)||[]),interval])}
 return Array.from({length:days},(_,i)=>{
  const date=shiftDay(today,-i),b=byDate.get(date),intervals=intervalsByDate.get(date)||[]
  return {date,label:names[i]||date.slice(5),bytes:b?.bytes??null,estimated:!!b?.estimated_intervals||intervals.some(interval=>interval.estimated),recorded:!!b?.intervals||intervals.length>0,sourceKind:intervals.find(interval=>interval.counter_kind)?.counter_kind,message:intervals.find(interval=>interval.message)?.message}
 })
}
