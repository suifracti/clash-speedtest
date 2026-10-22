import {unlockGroup,unlockServiceAllowed,unlockSupported} from './unlock'
import {exitLanding} from './exitLanding'
import type {EntryResult,LocalPosition,NodeEntry} from './entryPath'
import {computed,inject,reactive,ref,watch,type InjectionKey} from 'vue'
import {api,events,mapLimited,saveFile} from './api'
import {key,scope,isNotice,isReputationRule,sameHistoryRecords,latencyFor,sampleFor,downloadSpeed,downloadTone,outcomeTone,monitorGroups,targets,type MeasurementRound,type Airport,type Attempt,type HistoryPage,type LatencyBatch,type LatencyTest,type MonitorJob,type NodeOption,type Page,type Plan,type Project,type QueueItem,type ServiceRule,type Settings,type Setup,type TokenStatus} from './domain'
import {compareNodes,nodeDisplayName,nodeRegion,nodeSourceIndex,sourceForNode} from './nodePresentation'
import {downloadGroup,latencyComparisonGroup,serviceAvailabilitySummary,serviceComparisonGroup} from './sharedResults'
import {readSortSnapshot,type SortResult} from './sortSnapshot'
import {clearUsageCache} from './usageDaily'
const startRejected=(e:unknown)=>{const status=(e as {status?:number})?.status;return !!status&&status>=400&&status<500}
const terminal=(state:string)=>!['queued','running','cancelling','saving'].includes(state)
const delay=(ms:number)=>new Promise(resolve=>setTimeout(resolve,ms))
const uid=()=>crypto.randomUUID?.()||`request-${Date.now()}-${Math.random().toString(16).slice(2)}`
function savedSources():string[]|null{try{const value=JSON.parse(window.localStorage.getItem('speedtest.circle-airports')||'null');return Array.isArray(value)?value:null}catch{return null}}
function savedDisplaySites():string[]{try{const ids=JSON.parse(window.localStorage.getItem('speedtest.display-sites')||'null');if(Array.isArray(ids)){const valid=[...new Set(ids.filter((id):id is string=>typeof id==='string'&&targets.some(t=>t.id===id)))];if(valid.length)return valid}}catch{}return targets.map(t=>t.id)}
const selectedServicesKey='speedtest.selected-unlocks-v2'
function savedServices():string[]{try{
 // Old display-only choices were often automatic; they must not become detection defaults.
 const ids=JSON.parse(window.localStorage.getItem(selectedServicesKey)||'null')
 return Array.isArray(ids)?[...new Set(ids.filter((id):id is string=>typeof id==='string'&&!!id&&unlockSupported(id)))]:[]
}catch{return []}}

type ActiveWork=({kind:'latency';requestId:string;batchId?:string}|{kind:'download'|'service';requestId:string;node:NodeOption;attemptId?:string;serviceId?:string})&{body?:Record<string,unknown>}
type HistoryCursor={at:string;attemptId:string}
type HistoryWindow={since:string;until:string}
const measurementRoundCacheMs=5*60*1000
const activeWorkKey='speedtest.active-work'
const serviceScope=(n:NodeOption,serviceId?:string)=>serviceId?{...n,service_id:serviceId}:n
function savedWork():ActiveWork|null{try{const value=JSON.parse(window.sessionStorage.getItem(activeWorkKey)||'null');if(!value||!['latency','download','service'].includes(value.kind)||typeof value.requestId!=='string')return null;if(value.kind!=='latency'){if(!value.node||!value.node.profile_id||!value.node.node_key||!value.node.node_identity_key||!value.node.config_revision_key)return null;value.node={...scope(value.node),profile_name:value.node.profile_id,display_name:'待确认节点',type:'',country_code:'',country_flag:''}}return value}catch{return null}}
export function createWorkspace(){
  const latencyReportFocus=ref<{nodeKey:string;id:string;test:LatencyTest}|null>(null)
  const reportSelection=ref<{batch:LatencyBatch;monitor:boolean;previous:LatencyBatch[]}|null>(null)
  const autoPickReportId=ref('')
  function openAutoPickReport(id:string){autoPickReportId.value=id}
  function openLatencyReport(batch:LatencyBatch,monitor=false,previous:LatencyBatch[]=[]){reportSelection.value={batch,monitor,previous};queueOpen.value=true}
  const latestReportBatch=computed(()=>{const ids=new Set(queue.value.filter(q=>q.project==='latency').map(q=>q.batchId));return batches.value.find(b=>terminal(b.state)&&b.items?.length&&(!queue.value.length||ids.has(b.batch_id)))})
  const page=ref<Page>('home'),project=ref<Project>('combined'),airports=ref<Airport[]>([]),nodes=ref<NodeOption[]>([]),catalog=ref<ServiceRule[]>([]),jobs=ref<MonitorJob[]>([]),batches=ref<LatencyBatch[]>([]),setup=ref<Setup|null>(null),settings=ref<Settings|null>(null),token=ref<TokenStatus|null>(null)
  const landingEvidence=ref<Record<string,Attempt[]>>({})
  // 入口链：解析走物理网卡，不经过节点；页面打开时只读缓存，只有显式刷新才发 DNS/查询。
  const entries=ref<Record<string,NodeEntry>>({}),localPosition=ref<LocalPosition|null>(null),entryPending=ref<Record<string,boolean>>({}),entryErrors=ref<Record<string,string>>({})
  function mergeEntries(result:EntryResult){if(result.local)localPosition.value=result.local;for(const item of result.nodes||[])entries.value[key(item)]=item}
  async function readEntries(list:NodeOption[],refresh=false){const unique=[...new Map(list.map(n=>[key(n),n])).values()];if(!unique.length)return;for(const n of unique)entryPending.value[key(n)]=true;try{const result=await api.entries(unique,refresh);mergeEntries(result);for(const n of unique)delete entryErrors.value[key(n)]}catch(e){for(const n of unique)entryErrors.value[key(n)]=(e as Error).message}finally{for(const n of unique)delete entryPending.value[key(n)]}}
  async function ensureEntry(node:NodeOption){const k=key(node);if(entries.value[k]?.addresses?.length)return;await readEntries([node],false);if(!entries.value[k]?.addresses?.length)await readEntries([node],true)}
  const entryFor=(node:NodeOption)=>entries.value[key(node)]
  const resolveEntries=(list:NodeOption[])=>readEntries(list,true)
  const usageRefreshVersion=ref(0),regionSkipDefault=ref(true)
  const skipByApplicableRegions=computed({get:()=>settings.value?.skip_by_applicable_regions??regionSkipDefault.value,set:(enabled:boolean)=>{
   regionSkipDefault.value=enabled
   if(settings.value){const next={...settings.value,skip_by_applicable_regions:enabled};settings.value=next;void api.saveSettings(next).catch(fail)}
  }})
  const selectedAirportIds=ref<string[]|null>(savedSources()),selectedKeys=ref<string[]>([]),multiSelect=ref(false),search=ref(''),region=ref('all'),excludeRegion=ref(false),subscription=ref('all'),airportFilter=ref('all'),priorityAirportId=ref('all'),sort=ref('region'),hours=ref(24),displayTarget=ref('cloudflare'),displayTargets=ref<string[]>(savedDisplaySites()),serviceIds=ref<string[]>(savedServices()),displayServiceIds=serviceIds
  const loading=ref(false),authRequired=ref(false),connected=ref(false),error=ref(''),toast=ref(''),inspectNode=ref<NodeOption|null>(null),viewRequest=ref<NodeOption|null>(null),historyNode=ref<NodeOption|null>(null),plan=ref<Plan|null>(null),monitorNodes=ref<NodeOption[]|null>(null),queueOpen=ref(false),queue=ref<QueueItem[]>([]),running=ref(false),cancelRequested=ref(false),activeBatch=ref<LatencyBatch|null>(null),activeAttempt=ref<{kind:'download'|'service';node:NodeOption;attempt:Attempt}|null>(null)
  const downloadFullOnly=ref(false)
  const sortSnapshots=ref<Record<string,SortResult>>({}),sortLoading=ref(false),sortError=ref(''),sortAt=ref(''),sortReadCount=ref(0)
  let sortGeneration=0
  const measurementRounds=ref<Record<string,MeasurementRound[]>>({})
  const latency=ref<Record<string,LatencyTest[]>>({}),downloads=ref<Record<string,Attempt[]>>({}),services=ref<Record<string,Attempt[]>>({}),readErrors=ref<Record<string,string>>({}),pendingReads=ref(0),loaded=new Set<string>(),historyStarted=new Set<string>(),inflight=new Set<string>(),historyCursors=new Map<string,HistoryCursor>(),historyWindows=new Map<string,HistoryWindow>(),historyHasMore=ref<Record<string,boolean>>({}),roundLoadedAt=new Map<string,number>(),roundInflight=new Map<string,Promise<void>>(),historyErrors=reactive(new Map<string,Partial<Record<Project,string>>>()),pendingStart=ref<ActiveWork|null>(savedWork()),retryingStart=ref(false)
  const nodeSources=computed(()=>nodeSourceIndex(airports.value))
  const nodeAirportId=(n:NodeOption)=>sourceForNode(n,nodeSources.value).airportId
  const nodeAirportName=(n:NodeOption)=>sourceForNode(n,nodeSources.value).airportName
  const nodeSubscriptionName=(n:NodeOption)=>sourceForNode(n,nodeSources.value).subscriptionName
  const sourceNodes=computed(()=>nodes.value.filter(n=>{const source=nodeSources.value.get(n.profile_id);return !!source&&(selectedAirportIds.value||[]).includes(source.airportId)}))
  const proxyNodes=computed(()=>sourceNodes.value.filter(n=>!isNotice(n)))
  const notices=computed(()=>sourceNodes.value.filter(isNotice))
  const selectedNodes=computed(()=>proxyNodes.value.filter(n=>selectedKeys.value.includes(key(n))))
  const filteredNodes=computed(()=>{
    const q=search.value.trim().toLowerCase();
    const isExcluded=excludeRegion.value||region.value.startsWith('exclude:');
    const targetRegion=region.value.startsWith('exclude:')?region.value.slice(8):region.value;
    let visible=proxyNodes.value.filter(n=>{
      if(downloadFullOnly.value&&project.value==='download'&&downloadGroup(summaryDownload(n)).kind!=='full')return false;
      if(airportFilter.value!=='all'&&nodeAirportId(n)!==airportFilter.value)return false;
      if(subscription.value!=='all'&&n.profile_id!==subscription.value)return false;
      if(targetRegion!=='all'){
        const matches=nodeRegion(n)===targetRegion||(n.country_code||'unknown')===targetRegion;
        if(isExcluded?matches:!matches)return false;
      }
      return !q||(n.display_name+' '+nodeDisplayName(n)+' '+n.profile_name+' '+nodeAirportName(n)+' '+nodeSubscriptionName(n)+' '+n.country_code).toLowerCase().includes(q);
    });
    if(subscription.value==='all'){
      const seen=new Set<string>();
      visible=visible.filter(n=>{
        if(!n.node_identity_key)return true;
        const duplicateKey=JSON.stringify([nodeAirportId(n),n.node_identity_key,n.display_name.trim().toLowerCase()]);
        if(seen.has(duplicateKey))return false;
        seen.add(duplicateKey);
        return true;
      });
    }
    return visible.sort((a,b)=>{if(sort.value==='latency'){const x=latencyComparisonGroup(sortSnapshots.value[key(a)]?.latency),y=latencyComparisonGroup(sortSnapshots.value[key(b)]?.latency);if(x.valid!==y.valid)return x.valid?-1:1;if(x.key!==y.key)return x.key.localeCompare(y.key)}if(sort.value==='download'){const x=downloadGroup(summaryDownload(a)),y=downloadGroup(summaryDownload(b));if(x.comparable!==y.comparable)return x.comparable?-1:1;if(x.key!==y.key)return x.kind==='invalid'?1:y.kind==='invalid'?-1:x.key.localeCompare(y.key)}if(sort.value==='service'){const group=(n:NodeOption)=>serviceComparisonGroup(displayServiceIds.value.map(id=>summaryService(n,id)),displayServiceIds.value);const x=group(a),y=group(b);if(x.complete!==y.complete)return x.complete?-1:1;if(x.key!==y.key)return x.key.localeCompare(y.key)}return compareNodes(a,b,nodeSources.value,sort.value,priorityAirportId.value,sortMetric)})})
  const busy=computed(()=>running.value||!!pendingStart.value||!!activeBatch.value&&!terminal(activeBatch.value.state)||!!activeAttempt.value&&(!terminal(activeAttempt.value.attempt.execution_state)||activeAttempt.value.attempt.persistence_state==='saving'))
  const done=computed(()=>queue.value.filter(q=>terminal(q.state)).length)
  let historyVersion=0,unsubscribe:(()=>void)|null=null,toastTimer:ReturnType<typeof setTimeout>|undefined,disposed=false,polling=0,lifetime=0
  const alive=(version:number)=>!disposed&&version===lifetime
  function remember(work:ActiveWork){if(disposed)return;try{const current=savedWork();if(current&&current.requestId!==work.requestId&&pendingStart.value?.requestId!==work.requestId)return;window.sessionStorage.setItem(activeWorkKey,JSON.stringify(work.kind==='latency'?work:{...work,node:scope(work.node)}))}catch{}}
  function forget(requestId?:string){if(disposed)return;try{if(requestId&&savedWork()?.requestId!==requestId)return;window.sessionStorage.removeItem(activeWorkKey)}catch{}}
  function historyReadError(n:NodeOption,p:Project){const errors=historyErrors.get(key(n));return errors?(p==='combined'?Object.values(errors).filter(Boolean).join('；'):errors[p]||''):readErrors.value[key(n)]||''}
  function setReadError(n:NodeOption,p:Project,message?:string){const k=key(n),errors=historyErrors.get(k)||{};if(message)errors[p]=message;else delete errors[p];historyErrors.set(k,errors);const text=Object.values(errors).filter(Boolean).join('；');if(text)readErrors.value[k]=text;else delete readErrors.value[k]}
  function notify(message:string){toast.value=message;clearTimeout(toastTimer);toastTimer=setTimeout(()=>toast.value='',6500)}
  function fail(e:unknown){error.value=e instanceof Error?e.message:String(e)}
  function latest(n:NodeOption){return latency.value[key(n)]?.[0]}
  function latestForTarget(n:NodeOption,target:string){return latency.value[key(n)]?.find(test=>sampleFor(test,target).length>0)}
  function latestService(n:NodeOption,serviceId:string){return services.value[key(n)]?.find(attempt=>attempt.service_id===serviceId)}
  function summaryDownload(n:NodeOption){const entry=sortSnapshots.value[key(n)];return entry?entry.download:downloads.value[key(n)]?.[0]}
  function summaryService(n:NodeOption,id:string){const entry=sortSnapshots.value[key(n)];return entry?entry.services.find(a=>a.service_id===id):latestService(n,id)}
  async function refreshSort(){
    const generation=++sortGeneration,mode=sort.value,list=proxyNodes.value.slice(),until=new Date().toISOString(),since=new Date(Date.now()-hours.value*3600000).toISOString()
    if(!['latency','download','service'].includes(mode))return
    sortLoading.value=true;sortError.value='';sortReadCount.value=0
    if(!sortAt.value)sortSnapshots.value=Object.fromEntries(list.map(n=>{const test=latestForTarget(n,displayTarget.value);return [key(n),{download:downloads.value[key(n)]?.[0],services:(services.value[key(n)]||[]).slice(),latency:test?{id:test.attempt_id,time:test.finished_at,value:latencyFor(test,displayTarget.value),tone:'good' as const,source:'manual' as const,conditionKey:JSON.stringify([test.method||'unknown',test.method_version||'unknown',test.target||'legacy',test.network_path?.method||'unverified',test.network_path?.interface||'unknown',test.network_path?.dns_mode||'unknown',test.network_path?.dns_bind_verified??'unknown',test.network_path?.socket_bind_verified??'unknown',test.network_path?.address_family||'unknown',test.network_path?.address_source||'unknown']),description:'已读工作台记录',latencyTests:[test]}:undefined}] }))
    const result=await readSortSnapshot(list,mode,displayTarget.value,displayServiceIds.value.slice(),since,until,()=>{if(generation===sortGeneration)sortReadCount.value++})
    if(generation!==sortGeneration||disposed)return
    sortLoading.value=false
    if(result.errors.length){sortError.value=`${result.errors.length} 个节点读取失败；仅沿用已读记录，尚非完整排名。`+result.errors[0];return}
    sortSnapshots.value=result.snapshot;sortAt.value=until
  }
  function sortMetric(n:NodeOption){
    if(sort.value==='download'){const attempt=summaryDownload(n);return downloadGroup(attempt).comparable?downloadSpeed(attempt):null}
    if(sort.value==='service'){const measured=displayServiceIds.value.map(id=>summaryService(n,id));return serviceComparisonGroup(measured,displayServiceIds.value).complete?serviceAvailabilitySummary(measured).rate:null}
    const summary=sortSnapshots.value[key(n)];let latency=summary?.latency
    if(!summary){const test=latestForTarget(n,displayTarget.value);if(test)latency={id:test.attempt_id,time:test.finished_at,value:latencyFor(test,displayTarget.value),tone:'good' as const,source:'manual' as const,latencyTests:[test],description:'本地历史记录'}}
    return latencyComparisonGroup(latency).valid?latency!.value:null
  }
  watch([sort,displayTarget,displayServiceIds,hours,proxyNodes],()=>{sortGeneration++;sortSnapshots.value={};sortAt.value='';sortLoading.value=false;if(['latency','download','service'].includes(sort.value))void refreshSort()},{deep:true})
  function toggleNode(n:NodeOption){multiSelect.value=true;const id=key(n);selectedKeys.value=selectedKeys.value.includes(id)?selectedKeys.value.filter(k=>k!==id):[...selectedKeys.value,id]}
  function cancelSelection(){selectedKeys.value=[];multiSelect.value=false}
  function toggleAll(list:NodeOption[]){multiSelect.value=true;const ids=list.map(key);selectedKeys.value=ids.every(id=>selectedKeys.value.includes(id))?selectedKeys.value.filter(id=>!ids.includes(id)):[...new Set([...selectedKeys.value,...ids])]}
  function resetUnavailableAirports(ids:string[]){if(!ids.includes(airportFilter.value))airportFilter.value='all';if(!ids.includes(priorityAirportId.value))priorityAirportId.value='all'}
  // 侧栏 / ⌘K 的机场快捷切换：不在测量范围里的机场先加进来，否则筛选结果会是空的。'all' = 全部机场都纳入并取消机场筛选。
  function viewAirport(id:string){const all=airports.value.map(a=>a.id),current=selectedAirportIds.value||[];if(id==='all'){if(all.some(x=>!current.includes(x)))selectedAirportIds.value=all;airportFilter.value='all';return}if(!current.includes(id))selectedAirportIds.value=[...current,id];airportFilter.value=airportFilter.value===id?'all':id}
  function setAirports(ids:string[]){selectedAirportIds.value=ids;resetUnavailableAirports(ids);subscription.value='all';region.value='all';inspectNode.value=null;plan.value=null}
  watch([sourceNodes,nodes],()=>{const available=new Set(proxyNodes.value.map(key));selectedKeys.value=selectedKeys.value.filter(k=>available.has(k));if(inspectNode.value&&!available.has(key(inspectNode.value)))inspectNode.value=null})
  watch(selectedAirportIds,ids=>{resetUnavailableAirports(ids||[]);window.localStorage.setItem('speedtest.circle-airports',JSON.stringify(ids))},{deep:true})
  watch(airportFilter,()=>{subscription.value='all';if(region.value!=='all'&&!proxyNodes.value.some(n=>(airportFilter.value==='all'||nodeAirportId(n)===airportFilter.value)&&(nodeRegion(n)===region.value||(n.country_code||'unknown')===region.value)))region.value='all'},{flush:'sync'})
  watch(project,value=>{if(value!=='combined'&&['latency','download','service'].includes(sort.value))sort.value=value},{flush:'sync'})
  const targetIds=new Set(targets.map(t=>t.id))
  watch(displayTargets,ids=>{const valid=[...new Set(ids.filter(id=>targetIds.has(id)))];if(!valid.length)valid.push('cloudflare');if(valid.length!==ids.length||valid.some((id,i)=>id!==ids[i]))displayTargets.value=valid;try{window.localStorage.setItem('speedtest.display-sites',JSON.stringify(valid))}catch{};},{deep:true,flush:'sync'})
  watch(displayTarget,id=>{if(!targetIds.has(id)){displayTarget.value=displayTargets.value[0]||'cloudflare';return}},{flush:'sync'})
  function isReputationServiceId(id:string){return isReputationRule(catalog.value.find(rule=>rule.service_id===id)||{service_id:id})}
  function normalizeServices(ids:string[]){return [...new Set(ids.filter(id=>unlockSupported(id)&&!isReputationServiceId(id)&&(!catalog.value.length||catalog.value.some(rule=>rule.service_id===id&&!rule.superseded_by))))]}
  function saveServices(){
    const ids=normalizeServices(serviceIds.value)
    if(ids.length!==serviceIds.value.length||ids.some((id,i)=>id!==serviceIds.value[i])){serviceIds.value=ids;return}
    try{window.localStorage.setItem(selectedServicesKey,JSON.stringify(ids))}catch{}
  }
  watch(catalog,()=>{if(catalog.value.length){if(catalog.value.some(s=>s.service_id==='chatgpt_unlock')&&(window.localStorage.getItem(selectedServicesKey)===null||serviceIds.value.some(id=>!catalog.value.some(s=>s.service_id===id))))serviceIds.value=catalog.value.filter(s=>unlockGroup(s.service_id)!=='legacy').map(s=>s.service_id);saveServices()}},{deep:true,flush:'sync'})
  watch(serviceIds,saveServices,{deep:true,flush:'sync'})
  watch(hours,()=>{historyVersion++;loaded.clear();historyStarted.clear();historyCursors.clear();historyWindows.clear();historyHasMore.value={};latency.value={};downloads.value={};services.value={};readErrors.value={};historyErrors.clear()})
  function mergeLatency(test:LatencyTest){
    const k=key(test),existing=latency.value[k]||[],prior=existing.find(t=>t.attempt_id===test.attempt_id)
    // Round readback embeds history.LatencyTest without persistence fields;
    // it and late saving events must not erase an already confirmed save.
    if(prior&&(prior.persistence_state==='saved'&&test.persistence_state!=='saved'||prior.persistence_state==='failed'&&(!test.persistence_state||['saving','pending','not_started'].includes(test.persistence_state))))return
    if(prior&&sameHistoryRecords([prior],[test]))return
    latency.value[k]=[test,...existing.filter(t=>t.attempt_id!==test.attempt_id)].sort((a,b)=>Date.parse(b.finished_at)-Date.parse(a.finished_at))
  }
  function mergeAttempt(kind:'download'|'service',a:Attempt){const map=kind==='download'?downloads:services,k=key(a),prior=map.value[k]?.find(t=>t.attempt_id===a.attempt_id);if(prior&&(terminal(prior.execution_state)&&!terminal(a.execution_state)||prior.persistence_state==='saved'&&a.persistence_state!=='saved'||prior.persistence_state==='failed'&&['saving','pending','not_started'].includes(a.persistence_state)))a=prior;if(prior&&sameHistoryRecords([prior],[a]))a=prior;if(a!==prior)map.value[k]=[a,...(map.value[k]||[]).filter(t=>t.attempt_id!==a.attempt_id)].sort((a,b)=>Date.parse(b.requested_at)-Date.parse(a.requested_at));const pending=pendingStart.value,matchedPending=pending?.kind===kind&&pending.requestId===a.request_id&&key(pending.node)===key(a);if(activeAttempt.value?.kind===kind&&activeAttempt.value.attempt.attempt_id===a.attempt_id||matchedPending){const node=matchedPending?nodes.value.find(n=>key(n)===key(pending.node))||{...pending.node,display_name:a.display_name}:activeAttempt.value!.node;activeAttempt.value={kind,node,attempt:a};if(matchedPending)pendingStart.value=null;if(terminal(a.execution_state)&&a.persistence_state!=='saving')forget(a.request_id);else remember({kind,node,requestId:a.request_id,attemptId:a.attempt_id,serviceId:a.service_id})}for(const q of queue.value)if(q.project===kind&&(q.attempt?.attempt_id===a.attempt_id||q.id===a.request_id)){q.attempt=a;q.state=a.execution_state;q.error=a.result?.error_message||a.persistence_error}}
  function absorbBatch(batch:LatencyBatch){const prior=batches.value.find(b=>b.batch_id===batch.batch_id);if(prior&&terminal(prior.state)&&!terminal(batch.state))batch=prior;const pending=pendingStart.value?.kind==='latency'&&pendingStart.value.requestId===batch.request_id;batches.value=[batch,...batches.value.filter(b=>b.batch_id!==batch.batch_id)].slice(0,30);for(const item of batch.items||[])if(item.result)mergeLatency(item.result);if(activeBatch.value?.batch_id===batch.batch_id||pending){activeBatch.value=batch;if(pending)pendingStart.value=null;if(terminal(batch.state))forget(batch.request_id);else remember({kind:'latency',requestId:batch.request_id,batchId:batch.batch_id})}for(const q of queue.value){if(pending&&q.project==='latency'&&q.state==='running'&&!q.batchId){const item=batch.items?.find(i=>key(i)===key(q.node));if(item){q.batchId=batch.batch_id;q.itemId=item.item_id}}if(q.batchId!==batch.batch_id)continue;const item=batch.items?.find(i=>i.item_id===q.itemId);if(item){q.state=item.execution_state;q.error=item.error_message||item.persistence_error;if(item.persistence_state==='failed')q.error=`保存失败：${item.persistence_error||'可在历史中重试保存'}`}}}
  async function refreshNodes(){const [a,n]=await Promise.all([api.airports(),api.nodes()]);airports.value=a||[];nodes.value=n||[];if(selectedAirportIds.value===null)selectedAirportIds.value=airports.value.map(a=>a.id);else selectedAirportIds.value=selectedAirportIds.value.filter(id=>airports.value.some(a=>a.id===id));usageRefreshVersion.value++;landingEvidence.value={};clearUsageCache();historyVersion++;loaded.clear();historyStarted.clear();historyCursors.clear();historyWindows.clear();historyHasMore.value={};roundLoadedAt.clear();measurementRounds.value={};readErrors.value={};historyErrors.clear()}
  async function refreshJobs(){jobs.value=await api.jobs()||[]}
  async function boot(){loading.value=true;error.value='';disposed=false;try{const status=await api.auth();authRequired.value=status.auth_required&&!status.authenticated;if(authRequired.value)return;setup.value=await api.setup();if(setup.value.state!=='ready'){page.value='data';return}const tasks=await Promise.allSettled([refreshNodes(),refreshJobs(),api.catalog().then(c=>{catalog.value=c||[]}),api.settings().then(s=>settings.value=s),api.token().then(t=>token.value=t),api.batches().then(async b=>{batches.value=b||[];const latest=batches.value.find(b=>terminal(b.state));if(latest){const full=await api.batch(latest.batch_id);batches.value=batches.value.map(row=>row.batch_id===full.batch_id?full:row)};const active=batches.value.find(b=>!terminal(b.state));if(active&&!pendingStart.value){activeBatch.value=active;remember({kind:'latency',requestId:active.request_id,batchId:active.batch_id})}})]);const failures=tasks.filter((r):r is PromiseRejectedResult=>r.status==='rejected');if(failures.length)error.value=failures.map(r=>r.reason?.message||String(r.reason)).join('；');unsubscribe?.();unsubscribe=events((type,payload)=>{if(type==='workbench_latency_batch_updated'&&payload?.batch_id)absorbBatch(payload);if(type.startsWith('workbench_latency_test_')&&payload?.attempt_id)mergeLatency(payload);if(type==='workbench_download_attempt_updated'&&payload?.attempt_id)mergeAttempt('download',payload);if(type==='workbench_public_service_attempt_updated'&&payload?.attempt_id)mergeAttempt('service',payload);if(type==='antigravity_token_updated')void api.token().then(t=>token.value=t).catch(fail);if(type==='antigravity_login_failed')fail(payload?.error||'账号登录失败');resumePolling()},value=>connected.value=value);await recoverActive();resumePolling()}catch(e){fail(e)}finally{loading.value=false}}
  function historyID(kind:Exclude<Project,'combined'>,n:NodeOption,version=historyVersion){return `${version}|${kind}|${key(n)}`}
  function historyKinds(projects:Project[]){return [...new Set(projects.flatMap<Exclude<Project,'combined'>>(p=>p==='combined'?['latency','download','service']:p))]}
  async function loadMeasurementRounds(list:NodeOption[],force=false){
    const version=historyVersion;if(!api.measurementRounds)return
    await mapLimited(list,3,async n=>{
      const id=`${version}|${key(n)}`
      if(!force&&(roundLoadedAt.get(key(n))!==undefined&&Date.now()-roundLoadedAt.get(key(n))!<measurementRoundCacheMs))return
      const current=roundInflight.get(id)
      if(current){await current;if(!force)return}
      if(version!==historyVersion||disposed)return
      const request=(async()=>{try{
        const rounds=await api.measurementRounds!(n,16)
        if(version!==historyVersion||disposed)return
        if(!sameHistoryRecords(measurementRounds.value[key(n)],rounds))measurementRounds.value[key(n)]=rounds;roundLoadedAt.set(key(n),Date.now())
        for(const r of rounds)for(const i of r.items){if(i.download)mergeAttempt('download',i.download);if(i.service)mergeAttempt('service',i.service);if(i.latency)mergeLatency(i.latency)}
        setReadError(n,project.value)
      }catch(e){if(version===historyVersion&&!disposed)setReadError(n,project.value,'轮次关联读取失败：'+(e instanceof Error?e.message:String(e)))}finally{roundInflight.delete(id)}})()
      roundInflight.set(id,request);await request
    })
  }
  async function readHistory(list:NodeOption[],projects:Project[],older:boolean){
    const kinds=historyKinds(projects),version=historyVersion,until=new Date().toISOString(),since=new Date(Date.parse(until)-hours.value*3600000).toISOString()
    for(const kind of kinds){
      const pending=list.filter(n=>{const id=historyID(kind,n,version);return older?historyHasMore.value[id]===true:!historyStarted.has(id)}).filter(n=>!inflight.has(historyID(kind,n,version)))
      if(!pending.length)continue
      const ids=pending.map(n=>historyID(kind,n,version));ids.forEach(id=>inflight.add(id));pendingReads.value++
      const windowFor=(n:NodeOption)=>{const id=historyID(kind,n,version);let window=historyWindows.get(id);if(!window){window={since,until};historyWindows.set(id,window)}return window}
      const applyPage=(n:NodeOption,hasMore:boolean,complete:boolean|undefined,records:Array<LatencyTest|Attempt>)=>{
        if(version!==historyVersion||disposed)return
        if(!hasMore&&complete===false&&!records.length){setReadError(n,kind,'历史读取失败');return}
        for(const record of records){if(kind==='latency')mergeLatency(record as LatencyTest);else mergeAttempt(kind,record as Attempt)}
        const id=historyID(kind,n,version);historyStarted.add(id)
        const last=records.at(-1),at=kind==='latency'?(last as LatencyTest|undefined)?.finished_at:((last as Attempt|undefined)?.finished_at||(last as Attempt|undefined)?.started_at||(last as Attempt|undefined)?.requested_at)
        const attemptId=last?.attempt_id
        if(hasMore&&at&&attemptId){historyCursors.set(id,{at,attemptId});historyHasMore.value={...historyHasMore.value,[id]:true};loaded.delete(`${kind}|${key(n)}`)}
        else if(hasMore){historyHasMore.value={...historyHasMore.value,[id]:false};historyCursors.delete(id)}
        else{historyHasMore.value={...historyHasMore.value,[id]:false};historyCursors.delete(id);loaded.add(`${kind}|${key(n)}`)}
        setReadError(n,kind,hasMore&&(!at||!attemptId)?'历史分页缺少游标，已停止读取以避免重复记录':undefined)
      }
      try{
        if(kind==='latency'&&!older){
          const first=windowFor(pending[0]),sameWindow=pending.every(n=>{const w=windowFor(n);return w.since===first.since&&w.until===first.until})
          if(!sameWindow){for(const n of pending){const w=windowFor(n);const page=await api.latencyHistory(n,{...w,limit:80});applyPage(n,page.has_more,page.complete,page.tests||[])}}
          else{const pages=await api.latencySummaries(pending,first.since,first.until,80);if(version!==historyVersion)continue;pending.forEach((n,i)=>{const page=pages[i];if(!page){setReadError(n,kind,'延迟历史读取失败');return}applyPage(n,page.has_more,page.complete,page.tests||[])})}
        }else await mapLimited(pending,3,async n=>{
          try{
            const w=windowFor(n),cursor=older?historyCursors.get(historyID(kind,n,version)):undefined,limit=kind==='latency'?80:100
            if(older&&!cursor){historyHasMore.value={...historyHasMore.value,[historyID(kind,n,version)]:false};return}
            const extra=kind==='latency'?{...w,limit,...(cursor?{before_finished_at:cursor.at,before_attempt_id:cursor.attemptId}:{})}:{...w,limit,...(cursor?{before_at:cursor.at,before_attempt_id:cursor.attemptId}:{})}
            const page=kind==='latency'?await api.latencyHistory(n,extra):await api.attemptHistory(kind,n,extra)
            if(version!==historyVersion)return
            applyPage(n,page.has_more,page.complete,kind==='latency'?(page.tests||[]):(page.attempts||[]))
          }catch(e){if(version===historyVersion)setReadError(n,kind,e instanceof Error?e.message:'历史读取失败')}
        })
      }catch(e){if(version===historyVersion)pending.forEach(n=>setReadError(n,kind,e instanceof Error?e.message:'历史读取失败'))}
      finally{ids.forEach(id=>inflight.delete(id));pendingReads.value--}
    }
  }
  async function ensureHistory(list:NodeOption[],projects:Project[]=[project.value]){await readHistory(list,projects,false)}
  async function loadMoreHistory(list:NodeOption[],projects:Project[]=[project.value]){await readHistory(list,projects,true)}
  function historyMoreCount(list:NodeOption[],p:Project=project.value){const nodesWithMore=new Set<string>();for(const kind of historyKinds([p]))for(const n of list)if(historyHasMore.value[historyID(kind,n)])nodesWithMore.add(key(n));return nodesWithMore.size}
  // 目录里有可用的 IP 信誉项时才默认勾上；目录为空（或读取失败）时不勾，否则每次检测都会被“至少选一个信誉项”挡住。
  function nodeUnlockRegion(n:NodeOption){return exitLanding(n,[...(landingEvidence.value[key(n)]||[]),...(services.value[key(n)]||[])]).consensusRegion}
  function allowedUnlockServices(n:NodeOption,ids:string[]){return ids.filter(id=>unlockServiceAllowed(id,nodeUnlockRegion(n),skipByApplicableRegions.value,settings.value?.unlock_service_regions))}
  function defaultReputation(){const ids=[...new Set(catalog.value.filter(rule=>isReputationRule(rule)&&!rule.service_id.startsWith('ipok_')).map(rule=>rule.service_id))];return {includeReputation:ids.length>0,reputationServiceIds:ids}}
  function openPlan(list=selectedNodes.value){if(!list.length){notify('先选择要检测的节点');return}plan.value={nodes:list.map(n=>({...n})),projects:project.value==='combined'?['latency','download','service']:[project.value],target:'all',samples:6,timeout:5,concurrency:8,downloadMiB:10,downloadSeconds:10,serviceIds:[...new Set(serviceIds.value.filter(id=>!isReputationServiceId(id)))],serviceSamples:1,repeats:1,...defaultReputation()}}
  function openReputationPlan(list=selectedNodes.value){if(!list.length){openPlan(list);return}openPlan(list);if(!plan.value)return;plan.value.projects=[]}
  async function pollBatch(batch:LatencyBatch){const version=lifetime;if(!alive(version))return batch;activeBatch.value=batch;absorbBatch(batch);polling++;let readError='';try{while(alive(version)){
    batch=activeBatch.value?.batch_id===batch.batch_id?activeBatch.value:batch;if(terminal(batch.state))break
    try{if(cancelRequested.value&&['queued','running'].includes(batch.state)){const updated=await api.cancelBatch(batch.batch_id);if(!alive(version))break;absorbBatch(updated)}
      await delay(1200);if(!alive(version))break;batch=activeBatch.value?.batch_id===batch.batch_id?activeBatch.value:batch;if(terminal(batch.state))break
      const updated=await api.batch(batch.batch_id);if(!alive(version))break;absorbBatch(updated);if(error.value===readError)error.value=''
    }catch(e){if(!alive(version))break;fail(e);readError=error.value;await delay(1200)}
  }if(alive(version)&&error.value===readError)error.value='';return activeBatch.value?.batch_id===batch.batch_id?activeBatch.value:batch}finally{if(version===lifetime)polling--}}
  async function pollAttempt(kind:'download'|'service',node:NodeOption,a:Attempt){const version=lifetime;if(!alive(version))return a;activeAttempt.value={kind,node,attempt:a};mergeAttempt(kind,a);polling++;let readError='';try{while(alive(version)){
    a=activeAttempt.value?.attempt.attempt_id===a.attempt_id?activeAttempt.value.attempt:a;if(terminal(a.execution_state)&&a.persistence_state!=='saving')break
    try{if(cancelRequested.value&&!terminal(a.execution_state)){const updated=await api.attemptAction(kind,a.attempt_id,serviceScope(node,a.service_id),'cancel');if(!alive(version))break;mergeAttempt(kind,updated)}
      await delay(1000);if(!alive(version))break;a=activeAttempt.value?.attempt.attempt_id===a.attempt_id?activeAttempt.value.attempt:a;if(terminal(a.execution_state)&&a.persistence_state!=='saving')break
      const updated=await api.attempt(kind,a.attempt_id,serviceScope(node,a.service_id));if(!alive(version))break;a=updated;mergeAttempt(kind,a);if(error.value===readError)error.value=''
    }catch(e){if(!alive(version))break;fail(e);readError=error.value;await delay(1000)}
  }if(alive(version)&&error.value===readError)error.value='';return activeAttempt.value?.attempt.attempt_id===a.attempt_id?activeAttempt.value.attempt:a}finally{if(version===lifetime)polling--}}
  function resumePolling(){if(disposed||running.value||polling||pendingStart.value)return;if(activeBatch.value&&!terminal(activeBatch.value.state))void pollBatch(activeBatch.value).catch(fail);else if(activeAttempt.value&&(!terminal(activeAttempt.value.attempt.execution_state)||activeAttempt.value.attempt.persistence_state==='saving')){const a=activeAttempt.value;void pollAttempt(a.kind,a.node,a.attempt).catch(fail)}}
  async function recoverActive(){const version=lifetime,work=pendingStart.value;if(!work||!alive(version))return;try{if(work.kind==='latency'){const batch=work.batchId?await api.batch(work.batchId):(await api.batches()).find(b=>b.request_id===work.requestId);if(!alive(version))return;if(!batch)throw new Error('启动结果尚未确认，请稍后刷新进度；为避免重复检测，暂不启动新测试');activeBatch.value=batch;absorbBatch(batch)}else{const serviceId=work.serviceId||String(work.body?.service_id||''),a=work.attemptId&&(work.kind!=='service'||serviceId)?await api.attempt(work.kind,work.attemptId,serviceScope(work.node,serviceId)):(await api.attemptHistory(work.kind,work.node,{limit:100,...(serviceId?{service_id:serviceId}:{})})).attempts?.find(a=>work.attemptId?a.attempt_id===work.attemptId:a.request_id===work.requestId);if(!alive(version))return;if(!a)throw new Error('启动结果尚未确认，请稍后刷新进度；为避免重复检测，暂不启动新测试');if(!queue.value.some(q=>q.id===work.requestId))queue.value.push({id:work.requestId,node:work.node,project:work.kind,serviceId:a.service_id,state:a.execution_state,attempt:a});mergeAttempt(work.kind,a)}error.value=''}catch(e){if(alive(version))fail(e)}if(alive(version))resumePolling()}
  async function runPlan(){const version=lifetime,frozen=plan.value;if(!frozen||busy.value||!alive(version))return;const regularServices=[...new Set(frozen.serviceIds.filter(id=>!isReputationServiceId(id)))],reputationServices=[...new Set((frozen.reputationServiceIds||[]).filter(isReputationServiceId))];if(!frozen.nodes.length)throw new Error('请选择节点和检测项目');if(frozen.projects.includes('service')&&!regularServices.length)throw new Error('请选择至少一个服务');if(frozen.includeReputation&&!reputationServices.length)throw new Error('请选择至少一个 IP 信誉/纯净度项目');const projects:Plan['projects']=frozen.projects.filter(p=>p!=='service');if(frozen.projects.includes('service')||frozen.includeReputation)projects.push('service');if(!projects.length)throw new Error('请选择节点和检测项目');const serviceList=[...new Set([...(frozen.projects.includes('service')?regularServices:[]),...(frozen.includeReputation?reputationServices:[])])];const available=new Map(proxyNodes.value.map(n=>[key(n),n.node_key]));if(frozen.nodes.some(n=>available.get(key(n))!==n.node_key))throw new Error('部分节点配置已变化，请重新选择');if(frozen.projects.includes('service'))serviceIds.value=[...regularServices]
    // 出口检测与入口解析一起做；解析失败不影响检测本身。
    if(frozen.includeReputation)await readEntries(frozen.nodes,true)
    plan.value=null;reportSelection.value=null;running.value=true;cancelRequested.value=false;error.value='';queueOpen.value=true;queue.value=[]
    const roundIds=Array.from({length:frozen.repeats},()=>uid())
    for(const roundId of roundIds)for(const p of projects)for(const node of frozen.nodes)for(const serviceId of p==='service'?allowedUnlockServices(node,serviceList):[undefined])queue.value.push({id:`round:${roundId}:${uid()}`,node,project:p,serviceId,state:'queued'})
    const plans=new Map<string,{round_id:string;items:Record<string,unknown>[]}>()
    const latencyRequests=new Map<string,string>()
    for(const q of queue.value){const id=q.id.match(/^round:([^:]+):/)![1];let plan=plans.get(id);if(!plan){plan={round_id:id,items:[]};plans.set(id,plan)}const requestId=q.project==='latency'?`round:${id}:latency:${Math.floor(frozen.nodes.findIndex(n=>key(n)===key(q.node))/200)}`:q.id;if(q.project==='latency')latencyRequests.set(q.id,requestId);plan.items.push({...scope(q.node),display_name:q.node.display_name,project:q.project,service_id:q.serviceId,request_id:requestId})}
    let starting:QueueItem[]=[]
    try{for(const p of plans.values())await api.createRound(p);for(let r=0;r<frozen.repeats&&alive(version);r++){for(const p of projects){
      if(cancelRequested.value||!alive(version))break
      if(p==='latency'){for(let offset=0;offset<frozen.nodes.length;offset+=200){
        if(cancelRequested.value||!alive(version))break
        const chunk=frozen.nodes.slice(offset,offset+200),entries=queue.value.filter(q=>q.project==='latency'&&q.state==='queued'&&chunk.some(n=>key(n)===key(q.node))).slice(0,chunk.length),requestId=latencyRequests.get(entries[0].id)!
        starting=entries;entries.forEach(q=>q.state='running')
        const body={request_id:requestId,test_project:'latency_stability',target_id:frozen.target,timeout_seconds:frozen.timeout,sample_count:frozen.samples,concurrency:frozen.concurrency,selections:chunk.map(n=>({...scope(n),display_name:n.display_name,node_type:n.type}))}
        pendingStart.value={kind:'latency',requestId,body};remember(pendingStart.value)
        const batch=await api.startLatency(body)
        if(!alive(version))return
        entries.forEach(q=>{const item=batch.items?.find(i=>key(i)===key(q.node));q.batchId=batch.batch_id;q.itemId=item?.item_id;q.state=item?.execution_state||'queued'})
        starting=[];await pollBatch(batch)
      }}else{
        const entries=queue.value.filter(q=>q.project===p&&q.state==='queued'&&q.id.startsWith('round:'+roundIds[r]+':'))
        const runEntry=async(q:QueueItem)=>{
          if(cancelRequested.value||!alive(version))return
          starting.push(q);q.state='running'
          const body={...scope(q.node),request_id:q.id,...(p==='download'?{maximum_bytes:frozen.downloadMiB*1048576,timeout_seconds:frozen.downloadSeconds,...(frozen.downloadSource?{source_id:frozen.downloadSource}:{})}:{service_id:q.serviceId,timeout_seconds:15,drift_samples:frozen.serviceSamples??4})}
          pendingStart.value={kind:p,requestId:q.id,node:q.node,body,serviceId:q.serviceId};remember(pendingStart.value)
          try{const a=await api.startAttempt(p,body);if(!alive(version))return;q.attempt=a;starting=starting.filter(item=>item!==q);await pollAttempt(p,q.node,a)}catch(e){if(!alive(version))return;if(!startRejected(e))throw e;q.state='not_executed';q.error=e instanceof Error?e.message:String(e);if(pendingStart.value?.requestId===q.id)pendingStart.value=null;forget(q.id);starting=starting.filter(item=>item!==q)}
        }
        if(p==='service')for(const node of frozen.nodes){
          const list=entries.filter(q=>key(q.node)===key(node));let next=0;const failures:unknown[]=[]
          await Promise.all(Array.from({length:Math.min(6,list.length)},async()=>{while(next<list.length&&!cancelRequested.value&&alive(version)){const q=list[next++];try{await runEntry(q)}catch(e){failures.push(e)}}}))
          if(failures.length)throw failures[0]
        }else for(const q of entries)await runEntry(q)
      }
    }}if(alive(version))notify(cancelRequested.value?'检测已取消，已完成结果保留':'检测队列已结束，查看结果与未执行原因')
    }catch(e){if(!alive(version))return;fail(e);const rejected=startRejected(e);if(rejected){pendingStart.value=null;forget()}for(const q of starting)if(!q.attempt&&!q.batchId){q.state=rejected?'not_executed':'running';q.error=rejected?error.value:`启动结果尚未确认：${error.value}。请刷新进度继续查询；暂不启动新测试。`};if(pendingStart.value)error.value=`启动结果尚未确认：${error.value}。请刷新进度继续查询；暂不启动新测试。`
    }finally{if(alive(version)){for(const q of queue.value)if(q.state==='queued')q.state=cancelRequested.value?'cancelled':'not_executed';for(const id of plans.keys())try{await api.finishRound(id,pendingStart.value||cancelRequested.value||error.value?'interrupted':'finished')}catch(e){fail(e)};await loadMeasurementRounds(frozen.nodes,true);running.value=false;resumePolling();if(!pendingStart.value&&latestReportBatch.value)openLatencyReport(latestReportBatch.value)}}}
  async function retryPending(){if(disposed||running.value||retryingStart.value||!pendingStart.value)return;retryingStart.value=true;try{await recoverActive()}finally{retryingStart.value=false}}
  async function cancelTests(){const version=lifetime;if(!alive(version))return;cancelRequested.value=true;try{await recoverActive();if(!alive(version))return;if(pendingStart.value){error.value='尚未找到该检测的后台任务，无法确认取消。可刷新进度继续查询后取消。';return}if(activeBatch.value&&['queued','running','cancelling'].includes(activeBatch.value.state)){const updated=await api.cancelBatch(activeBatch.value.batch_id);if(!alive(version))return;absorbBatch(updated)}if(activeAttempt.value&&!terminal(activeAttempt.value.attempt.execution_state)){const a=activeAttempt.value,updated=await api.attemptAction(a.kind,a.attempt.attempt_id,serviceScope(a.node,a.attempt.service_id),'cancel');if(!alive(version))return;mergeAttempt(a.kind,updated)}}catch(e){if(alive(version))fail(e)}}
  async function syncActive(){const version=lifetime;if(!alive(version))return;try{await recoverActive();if(!alive(version))return;if(activeBatch.value&&!terminal(activeBatch.value.state)){const updated=await api.batch(activeBatch.value.batch_id);if(!alive(version))return;absorbBatch(updated)}if(activeAttempt.value&&(!terminal(activeAttempt.value.attempt.execution_state)||activeAttempt.value.attempt.persistence_state==='saving')){const a=activeAttempt.value,updated=await api.attempt(a.kind,a.attempt.attempt_id,serviceScope(a.node,a.attempt.service_id));if(!alive(version))return;mergeAttempt(a.kind,updated)}resumePolling()}catch(e){if(alive(version))fail(e)}}
  async function exportSelection(){const selected=selectedNodes.value;if(!selected.length)return;const result=await api.exportClash(selected);if(await saveFile(result.yaml_content,'speedtest-selected.yaml','application/x-yaml'))notify(`已导出 ${result.node_count} 个节点`)}
  function dispose(){disposed=true;sortGeneration++;lifetime++;polling=0;running.value=false;retryingStart.value=false;for(const q of queue.value)if(q.state==='queued')q.state='not_executed';unsubscribe?.();clearTimeout(toastTimer)}
  return {latencyReportFocus,reportSelection,autoPickReportId,openAutoPickReport,openLatencyReport,latestReportBatch,landingEvidence,entries,localPosition,entryPending,entryErrors,readEntries,ensureEntry,entryFor,resolveEntries,skipByApplicableRegions,nodeUnlockRegion,allowedUnlockServices,downloadFullOnly,sortSnapshots,sortLoading,sortError,sortAt,sortReadCount,refreshSort,summaryDownload,summaryService,page,project,airports,nodes,catalog,jobs,batches,setup,settings,token,selectedAirportIds,selectedKeys,multiSelect,search,region,excludeRegion,subscription,airportFilter,priorityAirportId,sort,hours,displayTarget,displayTargets,serviceIds,displayServiceIds,usageRefreshVersion,loading,authRequired,connected,error,toast,inspectNode,viewRequest,historyNode,plan,monitorNodes,queueOpen,queue,running,cancelRequested,activeBatch,activeAttempt,pendingStart,retryingStart,retryPending,measurementRounds,loadMeasurementRounds,latency,downloads,services,readErrors,historyReadError,pendingReads,historyMoreCount,loadMoreHistory,sourceNodes,proxyNodes,notices,selectedNodes,filteredNodes,nodeAirportId,nodeAirportName,nodeSubscriptionName,busy,done,notify,fail,latest,latestForTarget,latestService,toggleNode,cancelSelection,toggleAll,setAirports,viewAirport,refreshNodes,refreshJobs,boot,ensureHistory,openPlan,openReputationPlan,runPlan,cancelTests,syncActive,mergeLatency,mergeAttempt,absorbBatch,exportSelection,dispose,monitorGroups}
}
export type Workspace=ReturnType<typeof createWorkspace>
export const workspaceKey:InjectionKey<Workspace>=Symbol('workspace')
export function useWorkspace(){const workspace=inject(workspaceKey);if(!workspace)throw new Error('Missing workspace');return workspace}
