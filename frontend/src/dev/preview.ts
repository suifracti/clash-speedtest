/// <reference types="vite/client" />
// Separate Vite development entry. Not imported by the application or emitted
// by its production build. All application requests stay inside this fixture.
import type {Airport,NodeOption,LatencyTest,Attempt,Settings,MonitorJob,MonitorSample} from '../domain'
import {latencyTargets} from '../latencyTargets'
import {shanghaiDay,shiftDay} from '../usageDaily'
if (!import.meta.env.DEV) throw new Error('UI preview is development-only')
const now=Date.now(),at=new Date(now-18*60_000).toISOString()
const airports:Airport[]=['青屿','远山','白帆'].map((name,i)=>({id:`demo-airport-${i}`,name:`${name} · 示例`,url_display:'example.invalid',node_count:4,has_cache:true,updated_at:at,subscriptions:[{id:`demo-account-${i}`,airport_id:`demo-airport-${i}`,name:i===0?'日常套餐':i===1?'备用套餐':'旅行套餐',url_display:'https://example.invalid/…',url_configured:true,node_count:4,has_cache:true,updated_at:at,usage:{upload:i===2?null:3*2**30,download:(30+i*19)*2**30,total:200*2**30,expire:Math.floor(now/1000)+86400*(23-i*7),updated_at:at}}]}))
const regions=['HK','JP','SG','US'],names=['香港 · 日常 01','日本 · 东京 02','新加坡 · 狮城 03','美国 · 西岸 04']
const nodes:NodeOption[]=airports.flatMap((a,i)=>names.map((name,j)=>({profile_id:a.subscriptions![0].id,profile_name:`${a.name} / ${a.subscriptions![0].name}`,node_key:`demo-${i}-${j}`,node_identity_key:`identity-${i}-${j}`,config_revision_key:`revision-${i}-${j}`,display_name:name,type:j===2?'trojan':'ss',country_code:regions[j],country_flag:''})))
const path={method:'physical_socket_auto_family_v1',interface:'示例网卡',dns_mode:'physical_interface_dns_v1',dns_bind_verified:false,socket_bind_verified:false,tcp_bindings:0,udp_bindings:0,address_family:'ipv4',address_source:'dns'}
const tests:LatencyTest[]=nodes.slice(0,10).map((n,i)=>({...n,attempt_id:`latency-${i}`,node_type:n.type,requested_at:at,finished_at:at,status:i===6?'failed':'completed',latency_ms:35+i*19,total_samples:24,success_samples:i===6?0:24,failure_samples:i===6?24:0,persistence_state:'saved',method:'http_ttfb',method_version:1,network_path:path,samples:latencyTargets.slice(0,4).flatMap((t,k)=>Array.from({length:6},(_,s)=>({seq:k*6+s,target:t.urls[0],timestamp:new Date(now-18*60_000+s*1000).toISOString(),latency_ms:i===6?0:35+i*19+k*8+s*3,success:i!==6,...(i===6?{error:'示例：连接超时'}:{})})))}))
const downloads:Attempt[]=nodes.slice(0,8).map((n,i)=>({...n,node_type:n.type,attempt_id:`download-${i}`,request_id:`request-${i}`,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://example.invalid/download',method:'bounded_get',rule_version:4,maximum_bytes:20*2**20,maximum_duration_ns:15e9},result:{outcome:'byte_limit',bytes_read:20*2**20,phase_timings_ns:{response_body:(1+i*.32)*1e9,connection:140e6},network_path:path,finished_at:at}}))
const catalog=[['netflix_unlock','Netflix','流媒体'],['youtube_premium','YouTube Premium','流媒体'],['chatgpt_unlock','ChatGPT','AI'],['claude_unlock','Claude','AI'],['gemini_unlock','Gemini','AI'],['exit_ipv4','出口 IPv4','Network'],['ping0_ip_quality','Ping0 IP 质量','Network']].map(([service_id,name,category])=>({service_id,name,category,description:'仅用于界面预览的合成记录',result_kind:service_id.includes('exit')?'exit_profile':'unlock',success_criterion:'依据示例记录展示',timeout_seconds:15}))
const services:Attempt[]=nodes.slice(0,9).flatMap((n,i)=>catalog.slice(0,5).map((s,j)=>({...n,node_type:n.type,attempt_id:`service-${i}-${j}`,request_id:`service-request-${i}-${j}`,service_id:s.service_id,requested_at:at,finished_at:at,execution_state:'completed',persistence_state:'saved',rule:{service_id:s.service_id,name:s.name,target_url:'https://example.invalid/service',rule_version:1},result:{outcome:i===4?'region_restricted':'unlocked',bytes_read:220,finished_at:at,summary:i===4?'示例：地区受限':'示例：判据通过',details:{region:n.country_code}}})))
// Include an old configuration at the same timestamp as current latency, so
// recommendation provenance and its history link can be inspected in the UI.
for(const record of services.filter(a=>a.node_key===nodes[0].node_key&&['chatgpt_unlock','claude_unlock','gemini_unlock'].includes(a.service_id||''))){record.node_key='demo-old-key';record.config_revision_key='demo-old-revision'}
const jobs:MonitorJob[]=airports.slice(0,2).map((a,i)=>({id:`demo-job-${i}`,name:i?'备用线路 · 定点观察':'日常线路 · 响应观察',profile_id:a.subscriptions![0].id,profile_name:a.name,nodes:nodes.filter(n=>n.profile_id===a.subscriptions![0].id),node_keys:nodes.filter(n=>n.profile_id===a.subscriptions![0].id).map(n=>n.node_key),probe_set:'latency_four_v1',sampling_tier:'normal',interval_seconds:300,timeout_seconds:10,state:i?'blocked':'paused',resume_on_launch:false,updated_at:at,...(i?{blocked_reason:'示例：订阅中一条节点配置已变更，请确认任务范围。'}:{})}))
const monitorSamples:MonitorSample[]=jobs.flatMap((job,j)=>Array.from({length:36},(_,i)=>({...job.nodes[0],sample_id:`demo-monitor-${j}-${i}`,display_name_snapshot:job.nodes[0].display_name,timestamp:new Date(now-(36-i)*1800_000).toISOString(),target:latencyTargets[0].urls[0],probe_type:'http_latency',success:i!==22,latency:(42+j*30+i)*1e6,ttfb:(42+j*30+i)*1e6,sampling_tier:'normal',trigger_type:'scheduled'})))
const batch={batch_id:'demo-batch',request_id:'demo-request',target_id:'all',state:'completed',requested_at:at,sample_count:6,item_count:tests.length,items:tests.map(t=>({...t,item_id:t.attempt_id,batch_id:'demo-batch',execution_state:'completed',result:t}))}
const settings:Settings={preferred_browser:'default',airport_monthly_prices:{'demo-airport-0':30,'demo-airport-1':18},monitor_retention_policy:'90d',monitor_retention_custom_days:90,monitor_storage_warning_bytes:2**30,monitor_storage_hard_bytes:2**31,monitor_budget_max_concurrent:4,monitor_budget_response_bytes:65536,local_notifications:{enabled:false,failure_percent:50,remaining_percent:10,expiry_days:7},clash_connection:{mode:'auto',endpoint:'',secret:''},subscription_daily_update_enabled:false}
const page=(items:unknown[],kind='tests')=>({[kind]:items,has_more:false,complete:true,next_cursor:''})
function matching<T extends {profile_id:string;node_key:string;node_identity_key?:string;config_revision_key?:string}>(items:T[],q:URLSearchParams|Record<string,unknown>){const get=(k:string)=>q instanceof URLSearchParams?q.get(k):q[k];return items.filter(n=>(!get('profile_id')||n.profile_id===get('profile_id'))&&(!get('node_key')||n.node_key===get('node_key'))&&(!get('node_identity_key')||n.node_identity_key===get('node_identity_key'))&&(!get('config_revision_key')||n.config_revision_key===get('config_revision_key')))}
async function handle(url:URL,body:any,method:string):Promise<unknown>{
 const p=url.pathname,q=url.searchParams
 if(p==='/api/auth/status')return {auth_required:false,authenticated:true}
 if(p==='/api/profile/setup')return {state:'ready',initialized:true,data_root:'界面预览 · 内存数据',profile_dir:'不读取真实目录',history_dir:'不读取真实历史',settings_file:'仅当前预览',schema_version:11,migration:{state:'isolated',target_history_dir:''}}
 if(p==='/api/airports')return airports
 if(p==='/api/monitor/nodes')return nodes
 if(p==='/api/settings'){if(method!=='GET')Object.assign(settings,body);return settings}
 if(p==='/api/antigravity/status')return {has_token:false,source:'',preview:''}
 if(p==='/api/workbench/public-service-catalog')return catalog
 if(p==='/api/subscription-usage/state')return {state:'idle',last_attempt:at,next_refresh:'',message:''}
 if(p==='/api/clash/usage')return []
 if(p==='/api/clash/node-status')return {connected:false,loaded:false,name:'',endpoint:''}
 if(p==='/api/workbench/entries')return {entries:[],local_position:null}
 if(p==='/api/workbench/latency-tests/query')return body.nodes.map((n:any)=>page(matching(tests,n)))
 if(p==='/api/workbench/latency-tests')return page(matching(tests,q))
 if(p==='/api/workbench/download-tests')return page(matching(downloads,q),'attempts')
 if(p==='/api/workbench/public-service-tests')return page(matching(services,q).filter(a=>!q.get('service_id')||a.service_id===q.get('service_id')),'attempts')
 if(p.endsWith('/previous'))return null
 if(p==='/api/history/node-revisions')return [...new Map(matching([...nodes,...tests,...downloads,...services],q).map(n=>[n.config_revision_key,{...n,last_observed_at:at}])).values()]
 if(p==='/api/history/latency-scopes')return matching(nodes,q).filter(n=>n.display_name===q.get('display_name'))
 if(p==='/api/workbench/latency-batches')return [batch]
 if(p==='/api/workbench/latency-batches/demo-batch')return batch
 if(p==='/api/monitor/jobs')return jobs
 if(['/api/measurement-rounds','/api/subscription-refresh-jobs','/api/history','/api/auto-pick/reports','/api/monitor/runs','/api/clash/verge-subscriptions'].includes(p))return []
 if(p==='/api/monitor/samples'||p==='/api/monitor/samples/cursor')return page(matching(monitorSamples,q),'items')
 if(p==='/api/monitor/facets')return {nodes:nodes.slice(0,8),profiles:airports.slice(0,2).map(a=>a.subscriptions![0].id),targets:[latencyTargets[0].urls[0]]}
 if(p==='/api/monitor/storage')return {total_bytes:286720,sample_count:monitorSamples.length,warning_bytes:2**30,hard_limit_bytes:2**31}
 if(p==='/api/monitor/budget')return {usage:{requests_used:72,bytes_used:49152},active_requests:0,max_concurrent:4,max_response_bytes:65536}
 if(p==='/api/auto-pick')return {config:{enabled:false,schedule:'daily',daily_hour:8,airport_ids:airports.map(a=>a.id),include_ai:true,include_streaming:true,ai_required:['chatgpt_unlock','claude_unlock'],streaming_required:['netflix_unlock'],min_response_rate:90,max_latency_ms:500,sample_count:6},running:false,estimate:{nodes:12,projects:4,requests:72,warn:false},subscription:{url:'',path:'',exists:false,node_count:0}}
 if(p==='/api/periodic-sampling')return {config:{enabled:false,latency_interval_seconds:120,service_interval_seconds:300,download_interval_seconds:3600,download_mib:10,service_concurrency:16,include_antigravity:false},running:false,node_count:nodes.length,service_count:catalog.length,monitor_job_ids:[],cycles:{}}
 if(p==='/api/periodic-sampling/checks')return {nodes,node:nodes[0],checks:[]}
 if(p==='/api/subscription-usage'){
  const today=shanghaiDay(),account=q.get('account')
  const accounts=airports.map((a,i)=>({account_key:`demo-usage-${i}`,airport_id:a.id,subscription_id:a.subscriptions![0].id,airport_name:a.name,subscription_name:a.subscriptions![0].name,captured_at:at,status:i===2?'partial':'ok',upload:a.subscriptions![0].usage!.upload,download:a.subscriptions![0].usage!.download,total:200*2**30,source:'subscription_header'}))
  const selectedAccounts=account?accounts.filter(a=>a.account_key===account):accounts
  const intervals=Array.from({length:14},(_,i)=>({day:shiftDay(today,-i),offset:i})).filter(({day})=>day>=(q.get('from')||'')&&day<=(q.get('until')||today)).flatMap(({day,offset:i})=>selectedAccounts.map(a=>{
   const index=accounts.findIndex(other=>other.account_key===a.account_key),unknown=i===2||i===0&&index===2,value=unknown?null:Math.round((.4+((i*7)%13)/10)*2**30*(.7+.3*index))
   return {account_key:a.account_key,name:a.airport_name+' · '+a.subscription_name,from:day+'T00:00:00+08:00',until:i===0?at:shiftDay(day,1)+'T00:00:00+08:00',bucket:day,bytes:value,upload:value===null?null:Math.round(value*.08),download:value===null?null:value-Math.round(value*.08),status:unknown?'missing':'observed',estimated:false,...(unknown?{message:'示例：这段时间没有完整账户计数'}:{})}
  }))
  const period=q.get('period')||'day',groups=new Map<string,typeof intervals>()
  for(const interval of intervals){const day=interval.bucket,d=new Date(day+'T00:00:00Z'),bucket=period==='month'?day.slice(0,7)+'-01':period==='week'?shiftDay(day,-(d.getUTCDay()+6)%7):day;groups.set(bucket,[...(groups.get(bucket)||[]),interval])}
  const sum=(rows:typeof intervals,field:'bytes'|'upload'|'download')=>rows.some(row=>row[field]!==null)?rows.reduce((n,row)=>n+(row[field]||0),0):null
  const buckets=[...groups].map(([date,rows])=>({date,bytes:sum(rows,'bytes'),upload:sum(rows,'upload'),download:sum(rows,'download'),intervals:rows.length,estimated_intervals:0})).sort((a,b)=>b.date.localeCompare(a.date))
  return {from:q.get('from'),until:q.get('until'),period,accounts:selectedAccounts,buckets,intervals,today:selectedAccounts.map(a=>({account_key:a.account_key,bytes:sum(intervals.filter(row=>row.account_key===a.account_key&&row.bucket===today),'bytes'),estimated:false,lower_bound:true,since:today+'T00:00:00+08:00'})),observed_bytes:sum(intervals,'bytes'),unallocated_bytes:null,unknown_intervals:intervals.filter(row=>row.bytes===null).length,daily_enabled:false}

 }
 if(p.includes('usage'))return {accounts:[],observations:[],days:[]}
 throw new Error('合成预览未接入此操作，不会连接真实服务：'+p)
}
window.fetch=async(input,init)=>{
 const url=new URL(typeof input==='string'?input:input instanceof URL?input.href:input.url,location.href)
 try{if(url.origin!==location.origin||!url.pathname.startsWith('/api/'))throw new Error('合成预览已阻止外部请求');const value=await handle(url,typeof init?.body==='string'?JSON.parse(init.body):undefined,init?.method||'GET');return new Response(JSON.stringify(value),{status:200,headers:{'Content-Type':'application/json'}})}catch(e){return new Response(JSON.stringify({error:String(e)}),{status:409})}
}
class PreviewEvents {onopen:((event:Event)=>void)|null=null;onerror=null;onmessage=null;constructor(){queueMicrotask(()=>this.onopen?.(new Event('open')))}close(){}}
window.EventSource=PreviewEvents as unknown as typeof EventSource
document.documentElement.dataset.uiPreview='true'
const notice=document.createElement('div');notice.className='preview-notice';notice.textContent='界面预览 · 所有机场、节点和记录均为合成数据';document.body.append(notice)
const style=document.createElement('style');style.textContent='.preview-notice{position:fixed;bottom:10px;left:50%;transform:translateX(-50%);z-index:99999;font:11px system-ui;padding:6px 14px;border:1px solid #ccc;background:#fff9e7;color:#625230;border-radius:20px;pointer-events:none}';document.head.append(style)
void import('../main')
