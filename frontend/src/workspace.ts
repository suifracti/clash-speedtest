import {computed,inject,reactive,ref,watch,type InjectionKey} from 'vue'
import {api,events,mapLimited,saveFile} from './api'
import {key,scope,isNotice,sameHistoryRecords,latencyFor,sampleFor,downloadSpeed,downloadTone,outcomeTone,monitorGroups,targets,type MeasurementRound,type Airport,type Attempt,type HistoryPage,type LatencyBatch,type LatencyTest,type MonitorJob,type NodeOption,type Page,type Plan,type Project,type QueueItem,type ServiceRule,type Settings,type Setup,type TokenStatus} from './domain'
import {compareNodes,nodeDisplayName,nodeRegion,nodeSourceIndex,sourceForNode} from './nodePresentation'
import {downloadGroup,latencyComparisonGroup,serviceAvailabilitySummary,serviceComparisonGroup} from './sharedResults'
import {readSortSnapshot,type SortResult} from './sortSnapshot'
const terminal=(state:string)=>!['queued','running','cancelling','saving'].includes(state)
const delay=(ms:number)=>new Promise(resolve=>setTimeout(resolve,ms))
const uid=()=>crypto.randomUUID?.()||`request-${Date.now()}-${Math.random().toString(16).slice(2)}`
function savedSources():string[]|null{try{const value=JSON.parse(window.localStorage.getItem('speedtest.circle-airports')||'null');return Array.isArray(value)?value:null}catch{return null}}
type ActiveWork=({kind:'latency';requestId:string;batchId?:string}|{kind:'download'|'service';requestId:string;node:NodeOption;attemptId?:string;serviceId?:string})&{body?:Record<string,unknown>}
type HistoryCursor={at:string;attemptId:string}
type HistoryWindow={since:string;until:string}
const measurementRoundCacheMs=5*60*1000
const activeWorkKey='speedtest.active-work'
const serviceScope=(n:NodeOption,serviceId?:string)=>serviceId?{...n,service_id:serviceId}:n
function savedWork():ActiveWork|null{try{const value=JSON.parse(window.sessionStorage.getItem(activeWorkKey)||'null');if(!value||!['latency','download','service'].includes(value.kind)||typeof value.requestId!=='string')return null;if(value.kind!=='latency'){if(!value.node||!value.node.profile_id||!value.node.node_key||!value.node.node_identity_key||!value.node.config_revision_key)return null;value.node={...scope(value.node),profile_name:value.node.profile_id,display_name:'待确认节点',type:'',country_code:'',country_flag:''}}return value}catch{return null}}
export function createWorkspace(){
  const page=ref<Page>('home'),project=ref<Project>('latency'),airports=ref<Airport[]>([]),nodes=ref<NodeOption[]>([]),catalog=ref<ServiceRule[]>([]),jobs=ref<MonitorJob[]>([]),batches=ref<LatencyBatch[]>([]),setup=ref<Setup|null>(null),settings=ref<Settings|null>(null),token=ref<TokenStatus|null>(null)
  const selectedAirportIds=ref<string[]|null>(savedSources()),selectedKeys=ref<string[]>([]),multiSelect=ref(false),search=ref(''),region=ref('all'),subscription=ref('all'),airportFilter=ref('all'),priorityAirportId=ref('all'),sort=ref('region'),hours=ref(24),displayTarget=ref('cloudflare'),displayTargets=ref<string[]>(targets.map(t=>t.id)),serviceIds=ref<string[]>([]),displayServiceIds=ref<string[]>([])
  const loading=ref(false),authRequired=ref(false),connected=ref(false),error=ref(''),toast=ref(''),inspectNode=ref<NodeOption|null>(null),historyNode=ref<NodeOption|null>(null),plan=ref<Plan|null>(null),monitorNodes=ref<NodeOption[]|null>(null),queueOpen=ref(false),queue=ref<QueueItem[]>([]),running=ref(false),cancelRequested=ref(false),activeBatch=ref<LatencyBatch|null>(null),activeAttempt=ref<{kind:'download'|'service';node:NodeOption;attempt:Attempt}|null>(null)
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
  const filteredNodes=computed(()=>{const q=search.value.trim().toLowerCase();return proxyNodes.value.filter(n=>(!downloadFullOnly.value||project.value!=='download'||downloadGroup(summaryDownload(n)).kind==='full')&&(airportFilter.value==='all'||nodeAirportId(n)===airportFilter.value)&&(subscription.value==='all'||n.profile_id===subscription.value)&&(region.value==='all'||nodeRegion(n)===region.value||(n.country_code||'unknown')===region.value)&&(!q||(n.display_name+' '+nodeDisplayName(n)+' '+n.profile_name+' '+nodeAirportName(n)+' '+nodeSubscriptionName(n)+' '+n.country_code).toLowerCase().includes(q))).sort((a,b)=>{if(sort.value==='latency'){const x=latencyComparisonGroup(sortSnapshots.value[key(a)]?.latency),y=latencyComparisonGroup(sortSnapshots.value[key(b)]?.latency);if(x.valid!==y.valid)return x.valid?-1:1;if(x.key!==y.key)return x.key.localeCompare(y.key)}if(sort.value==='download'){const x=downloadGroup(summaryDownload(a)),y=downloadGroup(summaryDownload(b));if(x.key!==y.key)return x.kind==='invalid'?1:y.kind==='invalid'?-1:x.key.localeCompare(y.key)}if(sort.value==='service'){const group=(n:NodeOption)=>serviceComparisonGroup(displayServiceIds.value.map(id=>summaryService(n,id)),displayServiceIds.value);const x=group(a),y=group(b);if(x.complete!==y.complete)return x.complete?-1:1;if(x.key!==y.key)return x.key.localeCompare(y.key)}return compareNodes(a,b,nodeSources.value,sort.value,priorityAirportId.value,sortMetric)})})
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
    if(!sortAt.value)sortSnapshots.value=Object.fromEntries(list.map(n=>[key(n),{download:downloads.value[key(n)]?.[0],services:(services.value[key(n)]||[]).slice(),latency:latestForTarget(n,displayTarget.value)?{id:latestForTarget(n,displayTarget.value)!.attempt_id,time:latestForTarget(n,displayTarget.value)!.finished_at,value:latencyFor(latestForTarget(n,displayTarget.value),displayTarget.value),tone:'good' as const,source:'manual' as const,conditionKey:JSON.stringify([latestForTarget(n,displayTarget.value)?.method||'unknown',latestForTarget(n,displayTarget.value)?.method_version||'unknown',latestForTarget(n,displayTarget.value)?.target||'legacy',latestForTarget(n,displayTarget.value)?.network_path?.method||'unverified']),description:'已读工作台记录'}:undefined}]))
    const result=await readSortSnapshot(list,mode,displayTarget.value,displayServiceIds.value.slice(),since,until,()=>{if(generation===sortGeneration)sortReadCount.value++})
    if(generation!==sortGeneration||disposed)return
    sortLoading.value=false
    if(result.errors.length){sortError.value=`${result.errors.length} 个节点读取失败；仅沿用已读记录，尚非完整排名。`+result.errors[0];return}
    sortSnapshots.value=result.snapshot;sortAt.value=until
  }
  function sortMetric(n:NodeOption){
    if(sort.value==='download'){const attempt=summaryDownload(n);return downloadGroup(attempt).kind!=='invalid'?downloadSpeed(attempt):null}
    if(sort.value==='service'){const measured=displayServiceIds.value.map(id=>summaryService(n,id));return serviceComparisonGroup(measured,displayServiceIds.value).complete?serviceAvailabilitySummary(measured).rate:null}
    const summary=sortSnapshots.value[key(n)];return summary?summary.latency?.value??null:latencyFor(latestForTarget(n,displayTarget.value),displayTarget.value)
  }
  watch([sort,displayTarget,displayServiceIds,hours,proxyNodes],()=>{sortGeneration++;sortSnapshots.value={};sortAt.value='';sortLoading.value=false;if(['latency','download','service'].includes(sort.value))void refreshSort()},{deep:true})
  function toggleNode(n:NodeOption){multiSelect.value=true;const id=key(n);selectedKeys.value=selectedKeys.value.includes(id)?selectedKeys.value.filter(k=>k!==id):[...selectedKeys.value,id]}
  function cancelSelection(){selectedKeys.value=[];multiSelect.value=false}
  function toggleAll(list:NodeOption[]){multiSelect.value=true;const ids=list.map(key);selectedKeys.value=ids.every(id=>selectedKeys.value.includes(id))?selectedKeys.value.filter(id=>!ids.includes(id)):[...new Set([...selectedKeys.value,...ids])]}
  function resetUnavailableAirports(ids:string[]){if(!ids.includes(airportFilter.value))airportFilter.value='all';if(!ids.includes(priorityAirportId.value))priorityAirportId.value='all'}
  function setAirports(ids:string[]){selectedAirportIds.value=ids;resetUnavailableAirports(ids);subscription.value='all';region.value='all';inspectNode.value=null;plan.value=null}
  watch([sourceNodes,nodes],()=>{const available=new Set(proxyNodes.value.map(key));selectedKeys.value=selectedKeys.value.filter(k=>available.has(k));if(inspectNode.value&&!available.has(key(inspectNode.value)))inspectNode.value=null})
  watch(selectedAirportIds,ids=>{resetUnavailableAirports(ids||[]);window.localStorage.setItem('speedtest.circle-airports',JSON.stringify(ids))},{deep:true})
  watch(airportFilter,()=>{subscription.value='all';if(region.value!=='all'&&!proxyNodes.value.some(n=>(airportFilter.value==='all'||nodeAirportId(n)===airportFilter.value)&&(nodeRegion(n)===region.value||(n.country_code||'unknown')===region.value)))region.value='all'},{flush:'sync'})
  watch(project,value=>{if(value!=='combined'&&['latency','download','service'].includes(sort.value))sort.value=value},{flush:'sync'})
  const targetIds=new Set(targets.map(t=>t.id))
  watch(displayTargets,ids=>{const valid=[...new Set(ids.filter(id=>targetIds.has(id)))];if(!valid.length)valid.push('cloudflare');if(valid.length!==ids.length||valid.some((id,i)=>id!==ids[i]))displayTargets.value=valid;if(displayTarget.value!==valid[0])displayTarget.value=valid[0]},{deep:true,flush:'sync'})
  watch(displayTarget,id=>{if(!targetIds.has(id)){displayTarget.value=displayTargets.value[0]||'cloudflare';return}if(displayTargets.value[0]!==id)displayTargets.value=displayTargets.value.includes(id)?[id,...displayTargets.value.filter(t=>t!==id)]:[id,...displayTargets.value.slice(1)]},{flush:'sync'})
  const validDisplayServices=(ids:string[])=>[...new Set(ids.filter(id=>catalog.value.some(rule=>rule.service_id===id)))]
  const defaultDisplayServices=()=>[...new Set([...validDisplayServices(serviceIds.value),...catalog.value.map(rule=>rule.service_id)])].slice(0,3)
  function replaceDisplayServices(ids:string[]){if(ids.length!==displayServiceIds.value.length||ids.some((id,i)=>id!==displayServiceIds.value[i]))displayServiceIds.value=ids}
  watch(catalog,()=>{const valid=validDisplayServices(displayServiceIds.value);replaceDisplayServices(valid.length?valid:defaultDisplayServices())},{deep:true,flush:'sync'})
  watch(displayServiceIds,(ids,previous)=>{const valid=validDisplayServices(ids);if(!valid.length&&catalog.value.length)valid.push(validDisplayServices(previous||[])[0]||defaultDisplayServices()[0]);replaceDisplayServices(valid)},{deep:true,flush:'sync'})
  watch(hours,()=>{historyVersion++;loaded.clear();historyStarted.clear();historyCursors.clear();historyWindows.clear();historyHasMore.value={};latency.value={};downloads.value={};services.value={};readErrors.value={};historyErrors.clear()})
  function mergeLatency(test:LatencyTest){const k=key(test);const existing=latency.value[k]||[];const prior=existing.find(t=>t.attempt_id===test.attempt_id);if(prior&&sameHistoryRecords([prior],[test]))return;latency.value[k]=[test,...existing.filter(t=>t.attempt_id!==test.attempt_id)].sort((a,b)=>Date.parse(b.finished_at)-Date.parse(a.finished_at))}
  function mergeAttempt(kind:'download'|'service',a:Attempt){const map=kind==='download'?downloads:services,k=key(a),prior=map.value[k]?.find(t=>t.attempt_id===a.attempt_id);if(prior&&(terminal(prior.execution_state)&&!terminal(a.execution_state)||prior.persistence_state==='saved'&&a.persistence_state!=='saved'||prior.persistence_state==='failed'&&['saving','pending','not_started'].includes(a.persistence_state)))a=prior;if(prior&&sameHistoryRecords([prior],[a]))a=prior;if(a!==prior)map.value[k]=[a,...(map.value[k]||[]).filter(t=>t.attempt_id!==a.attempt_id)].sort((a,b)=>Date.parse(b.requested_at)-Date.parse(a.requested_at));const pending=pendingStart.value,matchedPending=pending?.kind===kind&&pending.requestId===a.request_id&&key(pending.node)===key(a);if(activeAttempt.value?.kind===kind&&activeAttempt.value.attempt.attempt_id===a.attempt_id||matchedPending){const node=matchedPending?nodes.value.find(n=>key(n)===key(pending.node))||{...pending.node,display_name:a.display_name}:activeAttempt.value!.node;activeAttempt.value={kind,node,attempt:a};if(matchedPending)pendingStart.value=null;if(terminal(a.execution_state)&&a.persistence_state!=='saving')forget(a.request_id);else remember({kind,node,requestId:a.request_id,attemptId:a.attempt_id,serviceId:a.service_id})}for(const q of queue.value)if(q.project===kind&&(q.attempt?.attempt_id===a.attempt_id||q.id===a.request_id)){q.attempt=a;q.state=a.execution_state;q.error=a.result?.error_message||a.persistence_error}}
  function absorbBatch(batch:LatencyBatch){const prior=batches.value.find(b=>b.batch_id===batch.batch_id);if(prior&&terminal(prior.state)&&!terminal(batch.state))batch=prior;const pending=pendingStart.value?.kind==='latency'&&pendingStart.value.requestId===batch.request_id;batches.value=[batch,...batches.value.filter(b=>b.batch_id!==batch.batch_id)].slice(0,30);for(const item of batch.items||[])if(item.result)mergeLatency(item.result);if(activeBatch.value?.batch_id===batch.batch_id||pending){activeBatch.value=batch;if(pending)pendingStart.value=null;if(terminal(batch.state))forget(batch.request_id);else remember({kind:'latency',requestId:batch.request_id,batchId:batch.batch_id})}for(const q of queue.value){if(pending&&q.project==='latency'&&q.state==='running'&&!q.batchId){const item=batch.items?.find(i=>key(i)===key(q.node));if(item){q.batchId=batch.batch_id;q.itemId=item.item_id}}if(q.batchId!==batch.batch_id)continue;const item=batch.items?.find(i=>i.item_id===q.itemId);if(item){q.state=item.execution_state;q.error=item.error_message||item.persistence_error;if(item.persistence_state==='failed')q.error=`保存失败：${item.persistence_error||'可在历史中重试保存'}`}}}
  async function refreshNodes(){const [a,n]=await Promise.all([api.airports(),api.nodes()]);airports.value=a||[];nodes.value=n||[];if(selectedAirportIds.value===null)selectedAirportIds.value=airports.value.map(a=>a.id);else selectedAirportIds.value=selectedAirportIds.value.filter(id=>airports.value.some(a=>a.id===id));historyVersion++;loaded.clear();historyStarted.clear();historyCursors.clear();historyWindows.clear();historyHasMore.value={};roundLoadedAt.clear();measurementRounds.value={};readErrors.value={};historyErrors.clear()}
  async function refreshJobs(){jobs.value=await api.jobs()||[]}
  async function boot(){loading.value=true;error.value='';disposed=false;try{const status=await api.auth();authRequired.value=status.auth_required&&!status.authenticated;if(authRequired.value)return;setup.value=await api.setup();if(setup.value.state!=='ready'){page.value='data';return}const tasks=await Promise.allSettled([refreshNodes(),refreshJobs(),api.catalog().then(c=>{if(!serviceIds.value.length)serviceIds.value=['chatgpt_web','netflix_unlock','youtube_premium'].filter(id=>(c||[]).some(r=>r.service_id===id));catalog.value=c||[]}),api.settings().then(s=>settings.value=s),api.token().then(t=>token.value=t),api.batches().then(b=>{batches.value=b||[];const active=batches.value.find(b=>!terminal(b.state));if(active&&!pendingStart.value){activeBatch.value=active;remember({kind:'latency',requestId:active.request_id,batchId:active.batch_id})}})]);const failures=tasks.filter((r):r is PromiseRejectedResult=>r.status==='rejected');if(failures.length)error.value=failures.map(r=>r.reason?.message||String(r.reason)).join('；');unsubscribe?.();unsubscribe=events((type,payload)=>{if(type==='workbench_latency_batch_updated'&&payload?.batch_id)absorbBatch(payload);if(type.startsWith('workbench_latency_test_')&&payload?.attempt_id)mergeLatency(payload);if(type==='workbench_download_attempt_updated'&&payload?.attempt_id)mergeAttempt('download',payload);if(type==='workbench_public_service_attempt_updated'&&payload?.attempt_id)mergeAttempt('service',payload);if(type==='antigravity_token_updated')void api.token().then(t=>token.value=t).catch(fail);if(type==='antigravity_login_failed')fail(payload?.error||'账号登录失败');resumePolling()},value=>connected.value=value);await recoverActive();resumePolling()}catch(e){fail(e)}finally{loading.value=false}}
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
  function openPlan(list=selectedNodes.value){if(!list.length){notify('先选择要检测的节点');return}plan.value={nodes:list.map(n=>({...n})),projects:project.value==='combined'?['latency','download','service']:[project.value],target:'all',samples:6,timeout:5,concurrency:8,downloadMiB:10,downloadSeconds:10,serviceIds:[...serviceIds.value],repeats:1}}
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
      const updated=await api.attempt(kind,a.attempt_id,serviceScope(node,a.service_id));if(!alive(version))break;mergeAttempt(kind,updated);if(error.value===readError)error.value=''
    }catch(e){if(!alive(version))break;fail(e);readError=error.value;await delay(1000)}
  }if(alive(version)&&error.value===readError)error.value='';return activeAttempt.value?.attempt.attempt_id===a.attempt_id?activeAttempt.value.attempt:a}finally{if(version===lifetime)polling--}}
  function resumePolling(){if(disposed||running.value||polling||pendingStart.value)return;if(activeBatch.value&&!terminal(activeBatch.value.state))void pollBatch(activeBatch.value).catch(fail);else if(activeAttempt.value&&(!terminal(activeAttempt.value.attempt.execution_state)||activeAttempt.value.attempt.persistence_state==='saving')){const a=activeAttempt.value;void pollAttempt(a.kind,a.node,a.attempt).catch(fail)}}
  async function recoverActive(){const version=lifetime,work=pendingStart.value;if(!work||!alive(version))return;try{if(work.kind==='latency'){const batch=work.batchId?await api.batch(work.batchId):(await api.batches()).find(b=>b.request_id===work.requestId);if(!alive(version))return;if(!batch)throw new Error('启动结果尚未确认，请稍后刷新进度；为避免重复检测，暂不启动新测试');activeBatch.value=batch;absorbBatch(batch)}else{const serviceId=work.serviceId||String(work.body?.service_id||''),a=work.attemptId&&(work.kind!=='service'||serviceId)?await api.attempt(work.kind,work.attemptId,serviceScope(work.node,serviceId)):(await api.attemptHistory(work.kind,work.node,{limit:100,...(serviceId?{service_id:serviceId}:{})})).attempts?.find(a=>work.attemptId?a.attempt_id===work.attemptId:a.request_id===work.requestId);if(!alive(version))return;if(!a)throw new Error('启动结果尚未确认，请稍后刷新进度；为避免重复检测，暂不启动新测试');if(!queue.value.some(q=>q.id===work.requestId))queue.value.push({id:work.requestId,node:work.node,project:work.kind,serviceId:a.service_id,state:a.execution_state,attempt:a});mergeAttempt(work.kind,a)}error.value=''}catch(e){if(alive(version))fail(e)}if(alive(version))resumePolling()}
  async function runPlan(){const version=lifetime,frozen=plan.value;if(!frozen||busy.value||!alive(version))return;if(!frozen.projects.length||!frozen.nodes.length)throw new Error('请选择节点和检测项目');if(frozen.projects.includes('service')&&!frozen.serviceIds.length)throw new Error('请选择至少一个服务');const available=new Map(proxyNodes.value.map(n=>[key(n),n.node_key]));if(frozen.nodes.some(n=>available.get(key(n))!==n.node_key))throw new Error('部分节点配置已变化，请重新选择');plan.value=null;running.value=true;cancelRequested.value=false;error.value='';queueOpen.value=true;queue.value=[]
    for(let r=0;r<frozen.repeats;r++){const roundId=uid();for(const p of frozen.projects)for(const node of frozen.nodes)for(const serviceId of p==='service'?frozen.serviceIds:[undefined])queue.value.push({id:`round:${roundId}:${uid()}`,node,project:p,serviceId,state:'queued'})}
    const plans=new Map<string,{round_id:string;items:Record<string,unknown>[]}>()
    const latencyRequests=new Map<string,string>()
    for(const q of queue.value){const id=q.id.match(/^round:([^:]+):/)![1];let plan=plans.get(id);if(!plan){plan={round_id:id,items:[]};plans.set(id,plan)}const requestId=q.project==='latency'?`round:${id}:latency:${Math.floor(frozen.nodes.findIndex(n=>key(n)===key(q.node))/200)}`:q.id;if(q.project==='latency')latencyRequests.set(q.id,requestId);plan.items.push({...scope(q.node),display_name:q.node.display_name,project:q.project,service_id:q.serviceId,request_id:requestId})}
    let starting:QueueItem[]=[]
    try{for(const p of plans.values())await api.createRound(p);for(let r=0;r<frozen.repeats&&alive(version);r++){for(const p of frozen.projects){
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
        const entries=queue.value.filter(q=>q.project===p&&q.state==='queued').slice(0,frozen.nodes.length*(p==='service'?frozen.serviceIds.length:1))
        for(const q of entries){
          if(cancelRequested.value||!alive(version))break
          starting=[q];q.state='running'
          const body={...scope(q.node),request_id:q.id,...(p==='download'?{maximum_bytes:frozen.downloadMiB*1048576,timeout_seconds:frozen.downloadSeconds}:{service_id:q.serviceId,timeout_seconds:15})}
          pendingStart.value={kind:p,requestId:q.id,node:q.node,body,serviceId:q.serviceId};remember(pendingStart.value)
          const a=await api.startAttempt(p,body);if(!alive(version))return;q.attempt=a;starting=[];await pollAttempt(p,q.node,a)
        }
      }
    }}if(alive(version))notify(cancelRequested.value?'检测已取消，已完成结果保留':'检测完成，结果已更新')
    }catch(e){if(!alive(version))return;fail(e);const status=(e as {status?:number})?.status,rejected=!!status&&status>=400&&status<500;if(rejected){pendingStart.value=null;forget()}for(const q of starting)if(!q.attempt&&!q.batchId){q.state=rejected?'failed':'running';q.error=rejected?error.value:`启动结果尚未确认：${error.value}。请刷新进度继续查询，或重试启动；暂不启动新测试。`};if(pendingStart.value)error.value=`启动结果尚未确认：${error.value}。请刷新进度继续查询，或重试启动；暂不启动新测试。`
    }finally{if(alive(version)){for(const q of queue.value)if(q.state==='queued')q.state=cancelRequested.value?'cancelled':'not_executed';if(!pendingStart.value)for(const id of plans.keys())try{await api.finishRound(id,cancelRequested.value||error.value?'interrupted':'finished')}catch(e){fail(e)};await loadMeasurementRounds(frozen.nodes,true);running.value=false;resumePolling()}}}
  async function retryPending(){const version=lifetime;if(!alive(version)||running.value||retryingStart.value||!pendingStart.value?.body)return;retryingStart.value=true;running.value=true;cancelRequested.value=false;try{await recoverActive();const work=pendingStart.value;if(!work?.body||!alive(version))return;if(work.kind==='latency'){const batch=await api.startLatency(work.body);if(!alive(version))return;activeBatch.value=batch;absorbBatch(batch)}else{const a=await api.startAttempt(work.kind,work.body);if(!alive(version))return;mergeAttempt(work.kind,a)}error.value=''}catch(e){if(!alive(version))return;fail(e);const status=(e as {status?:number})?.status;if(status&&status>=400&&status<500){const work=pendingStart.value;pendingStart.value=null;forget();for(const q of queue.value)if(q.state==='running'&&(work?.kind==='latency'&&q.project==='latency'&&!q.batchId||q.id===work?.requestId)){q.state='failed';q.error=error.value}}}finally{if(alive(version)){running.value=false;retryingStart.value=false;resumePolling()}}}
  async function cancelTests(){const version=lifetime;if(!alive(version))return;cancelRequested.value=true;try{await recoverActive();if(!alive(version))return;if(pendingStart.value){error.value='尚未找到该检测的后台任务，无法确认取消。可刷新进度继续查询，或重试启动后取消。';return}if(activeBatch.value&&['queued','running','cancelling'].includes(activeBatch.value.state)){const updated=await api.cancelBatch(activeBatch.value.batch_id);if(!alive(version))return;absorbBatch(updated)}if(activeAttempt.value&&!terminal(activeAttempt.value.attempt.execution_state)){const a=activeAttempt.value,updated=await api.attemptAction(a.kind,a.attempt.attempt_id,serviceScope(a.node,a.attempt.service_id),'cancel');if(!alive(version))return;mergeAttempt(a.kind,updated)}}catch(e){if(alive(version))fail(e)}}
  async function syncActive(){const version=lifetime;if(!alive(version))return;try{await recoverActive();if(!alive(version))return;if(activeBatch.value&&!terminal(activeBatch.value.state)){const updated=await api.batch(activeBatch.value.batch_id);if(!alive(version))return;absorbBatch(updated)}if(activeAttempt.value&&(!terminal(activeAttempt.value.attempt.execution_state)||activeAttempt.value.attempt.persistence_state==='saving')){const a=activeAttempt.value,updated=await api.attempt(a.kind,a.attempt.attempt_id,serviceScope(a.node,a.attempt.service_id));if(!alive(version))return;mergeAttempt(a.kind,updated)}resumePolling()}catch(e){if(alive(version))fail(e)}}
  async function exportSelection(){const selected=selectedNodes.value;if(!selected.length)return;const result=await api.exportClash(selected);if(await saveFile(result.yaml_content,'speedtest-selected.yaml','application/x-yaml'))notify(`已导出 ${result.node_count} 个节点`)}
  function dispose(){disposed=true;sortGeneration++;lifetime++;polling=0;running.value=false;retryingStart.value=false;for(const q of queue.value)if(q.state==='queued')q.state='not_executed';unsubscribe?.();clearTimeout(toastTimer)}
  return {downloadFullOnly,sortSnapshots,sortLoading,sortError,sortAt,sortReadCount,refreshSort,summaryDownload,summaryService,page,project,airports,nodes,catalog,jobs,batches,setup,settings,token,selectedAirportIds,selectedKeys,multiSelect,search,region,subscription,airportFilter,priorityAirportId,sort,hours,displayTarget,displayTargets,serviceIds,displayServiceIds,loading,authRequired,connected,error,toast,inspectNode,historyNode,plan,monitorNodes,queueOpen,queue,running,cancelRequested,activeBatch,activeAttempt,pendingStart,retryingStart,retryPending,measurementRounds,loadMeasurementRounds,latency,downloads,services,readErrors,historyReadError,pendingReads,historyMoreCount,loadMoreHistory,sourceNodes,proxyNodes,notices,selectedNodes,filteredNodes,nodeAirportId,nodeAirportName,nodeSubscriptionName,busy,done,notify,fail,latest,latestForTarget,latestService,toggleNode,cancelSelection,toggleAll,setAirports,refreshNodes,refreshJobs,boot,ensureHistory,openPlan,runPlan,cancelTests,syncActive,mergeLatency,mergeAttempt,absorbBatch,exportSelection,dispose,monitorGroups}
}
export type Workspace=ReturnType<typeof createWorkspace>
export const workspaceKey:InjectionKey<Workspace>=Symbol('workspace')
export function useWorkspace(){const workspace=inject(workspaceKey);if(!workspace)throw new Error('Missing workspace');return workspace}
