import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {DOMWrapper,flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import TestPlanModal from '../components/TestPlanModal.vue'
import QueueModal from '../components/QueueModal.vue'
import {createWorkspace,workspaceKey,type Workspace} from '../workspace'
import {api} from '../api'
import {key,type Attempt,type NodeOption,type LatencyTest} from '../domain'

vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,airports:vi.fn(),nodes:vi.fn(),startLatency:vi.fn(),startAttempt:vi.fn(),attemptAction:vi.fn(),createRound:vi.fn(),finishRound:vi.fn(),measurementRounds:vi.fn(),attemptHistory:vi.fn()}}
})
const node:NodeOption={profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',profile_name:'Airport A',display_name:'Node A',type:'ss',country_code:'SG',country_flag:''}
const airport={id:'airport-a',name:'Airport A',node_count:1,has_cache:true,url_display:'',subscriptions:[{id:'sub-a',airport_id:'airport-a',name:'Default',url_display:'',url_configured:true,node_count:1,has_cache:true}]}
let wrapper:VueWrapper|undefined,w:Workspace
function attach(component:any){wrapper=mount(component,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}});return wrapper}
function button(text:string){const found=Array.from(document.querySelectorAll('dialog button')).find(b=>b.textContent?.includes(text)) as HTMLButtonElement|undefined;expect(found).toBeDefined();return found!}
beforeEach(()=>{
  vi.resetAllMocks();window.sessionStorage.clear()
  HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn()
  vi.mocked(api.createRound).mockResolvedValue({});vi.mocked(api.finishRound).mockResolvedValue({});vi.mocked(api.measurementRounds).mockResolvedValue([]);vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[],has_more:false} as any)
  w=createWorkspace();w.airports.value=[airport];w.nodes.value=[node];w.setAirports(['airport-a'])
})
afterEach(()=>{wrapper?.unmount();wrapper=undefined;w.dispose();document.body.innerHTML=''})

it('keeps the configuration dialog open when a node changed after selecting the test scope',async()=>{
  w.project.value='latency';w.openPlan([node]);attach(TestPlanModal)
  vi.mocked(api.airports).mockResolvedValue([airport]);vi.mocked(api.nodes).mockResolvedValue([{...node,config_revision_key:'revision-new'}])
  await w.refreshNodes();await flushPromises()
  button('开始检测').click();await flushPromises()
  expect(document.querySelector('[role="alert"]')?.textContent).toContain('部分节点配置已变化，请重新选择')
  expect(w.plan.value?.nodes[0].config_revision_key).toBe('revision-a')
  expect(w.queueOpen.value).toBe(false)
  expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
  button('返回调整').click();await flushPromises();expect(w.plan.value).toBe(null)
})

it('retries a failed service save from its queue row and retains it after a rejected retry',async()=>{
  const failed:Attempt={...node,attempt_id:'service-a',request_id:'request-a',service_id:'cloudflare_204',requested_at:'2026-10-02T12:00:00Z',execution_state:'completed',persistence_state:'failed',persistence_error:'disk temporarily unavailable',rule:{target_url:'https://cp.cloudflare.com/generate_204'},result:{outcome:'matched',bytes_read:0,http_status:204,finished_at:'2026-10-02T12:00:01Z'}}
  w.queue.value=[{id:failed.request_id,node,project:'service',serviceId:failed.service_id,state:'completed',attempt:failed,error:failed.persistence_error}]
  w.mergeAttempt('service',failed);attach(QueueModal)
  vi.mocked(api.attemptAction).mockRejectedValueOnce(new Error('save retry unavailable')).mockResolvedValueOnce({...failed,persistence_state:'saved',persistence_error:undefined})
  button('重试保存').click();await flushPromises()
  expect(w.error.value).toBe('save retry unavailable')
  expect(button('重试保存').disabled).toBe(false)
  expect(w.queue.value[0].attempt?.persistence_state).toBe('failed')
  button('重试保存').click();await flushPromises()
  expect(api.attemptAction).toHaveBeenLastCalledWith('service','service-a',expect.objectContaining({profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a'}),'retry-save','cloudflare_204')
  expect(w.queue.value[0].attempt?.persistence_state).toBe('saved')
  expect(w.services.value[key(node)][0].persistence_state).toBe('saved')
  expect(document.body.textContent).not.toContain('重试保存')
  expect(document.body.textContent).not.toContain('disk temporarily unavailable')
})

it('shows exact service outcomes instead of green task completion and keeps old timeout evidence intact',()=>{
 const at='2026-10-15T09:27:25Z'
 const saved:Attempt={...node,attempt_id:'saved-timeout',request_id:'saved-request',service_id:'cloudflare_204',requested_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://cp.cloudflare.com/generate_204'},result:{outcome:'timed_out',duration_ms:5058,bytes_read:0,finished_at:at,failure_phase:'timeout',error_message:'请求超过设定超时',details:{proxy_connect_ms:'5002.3',request_headers_network_ms:'5002.4',http_requests_written:'0'}}}
 const raw=JSON.stringify(saved)
 const attempts=[saved,{...saved,attempt_id:'matched',result:{...saved.result!,outcome:'matched',http_status:204,details:{http_requests_written:'1'}}},{...saved,attempt_id:'limited',result:{...saved.result!,outcome:'rate_limited',http_status:429,details:{http_requests_written:'1'}}},{...saved,attempt_id:'unknown',result:{...saved.result!,outcome:'unknown',details:{}}},{...saved,attempt_id:'missing',result:undefined},{...saved,attempt_id:'http-rejected',result:{...saved.result!,outcome:'http_rejected',http_status:403,details:{http_requests_written:'1'}}},{...saved,attempt_id:'response-timeout',result:{...saved.result!,details:{http_requests_written:'1',proxy_connect_ms:'12.3'}}}]
 w.queue.value=attempts.map(a=>({id:a.attempt_id,node,project:'service',serviceId:'cloudflare_204',state:'completed',attempt:a}))
 attach(QueueModal)
 const rows=Array.from(document.querySelectorAll('.queue-row')).map(el=>new DOMWrapper(el))
 expect(rows[0].find('.badge').text()).toBe('超时 · 未确认')
 expect(rows[0].find('.badge').classes()).toContain('warn')
 expect(rows[0].text()).toContain('代理连接耗时约 5.00 秒')
 expect(rows[0].text()).toContain('未记录完整 HTTP 请求写出')
 expect(rows[1].find('.badge').text()).toBe('通过')
 expect(rows[1].find('.badge').classes()).toContain('good')
 expect(rows[2].find('.badge').text()).toBe('限流 · 未确认')
 expect(rows[3].find('.badge').text()).toBe('未确认')
 expect(rows[4].find('.badge').text()).toBe('已结束 · 无结果')
 expect(rows[5].find('.badge').text()).toBe('HTTP 拒绝')
 expect(rows[5].find('.badge').classes()).toContain('bad')
 expect(rows[6].find('.badge').text()).toBe('超时 · 未确认')
 expect(rows[6].text()).not.toContain('代理连接耗时约')
 for(const i of [0,2,3,4])expect(rows[i].find('.badge').classes()).not.toContain('good')
 expect(JSON.stringify(saved)).toBe(raw)
 expect(api.startAttempt).not.toHaveBeenCalled()
})

function downloadPlan(){const nodes=[node,{...node,node_key:'node-b',node_identity_key:'identity-b',display_name:'Node B'},{...node,node_key:'node-c',node_identity_key:'identity-c',display_name:'Node C'}];w.nodes.value=nodes;w.project.value='download';w.openPlan(nodes);return nodes}
function savedDownload(body:any):Attempt{return {...body,attempt_id:body.request_id,requested_at:'2026-10-12T01:00:00Z',execution_state:'completed',persistence_state:'saved',result:{outcome:'completed'}}}
it('continues after one explicitly rejected admission without counting it as a measurement failure',async()=>{
 downloadPlan();vi.mocked(api.startAttempt).mockImplementation(async(_kind,body:any)=>{if(body.node_key==='node-b')throw Object.assign(new Error('configuration no longer available'),{status:400,admission:'not_accepted'});return savedDownload(body)})
 await w.runPlan();expect(api.startAttempt).toHaveBeenCalledTimes(3);expect(w.queue.value.map(q=>q.state)).toEqual(['completed','not_executed','completed']);expect(w.queue.value[1].error).toContain('configuration');expect(w.done.value).toBe(3);expect(w.pendingStart.value).toBeNull()
})
it('pauses on storage or unknown admission and confirms only the original request ID without reposting',async()=>{
 downloadPlan();vi.mocked(api.startAttempt).mockRejectedValue(new Error('storage unavailable'))
 await w.runPlan();expect(api.finishRound).toHaveBeenCalledWith(expect.any(String),'interrupted');const original=w.pendingStart.value!.requestId;expect(api.startAttempt).toHaveBeenCalledTimes(1);expect(w.queue.value.map(q=>q.state)).toEqual(['running','not_executed','not_executed'])
 await w.retryPending();expect(w.pendingStart.value?.requestId).toBe(original);expect(api.startAttempt).toHaveBeenCalledTimes(1)
 vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[savedDownload({...node,request_id:original})],has_more:false} as any)
 await w.retryPending();expect(w.pendingStart.value).toBeNull();expect(w.queue.value[0].state).toBe('completed');expect(api.startAttempt).toHaveBeenCalledTimes(1)
})
it('cancellation after the first saved result prevents subsequent dispatch and preserves the result',async()=>{
 downloadPlan();vi.mocked(api.startAttempt).mockImplementation(async(_kind,body:any)=>{w.cancelRequested.value=true;return savedDownload(body)})
 await w.runPlan();expect(api.startAttempt).toHaveBeenCalledTimes(1);expect(w.queue.value.map(q=>q.state)).toEqual(['completed','cancelled','cancelled']);expect(w.queue.value[0].attempt?.persistence_state).toBe('saved');expect(w.done.value).toBe(3)
})


it('shows response coverage for each exact latency batch item instead of green completion',()=>{
 const items=[
  {name:'partial',state:'completed',status:'partial_failed',ok:5,bad:1,error:'EOF'},
  {name:'none',state:'failed',status:'failed',ok:0,bad:6,error:'context deadline exceeded'},
  {name:'all',state:'completed',status:'completed',ok:6,bad:0,error:undefined},
 ].map((v,index)=>{
  const n={...node,node_key:v.name,node_identity_key:'identity-'+v.name,display_name:v.name}
  w.queue.value.push({id:'queue-'+index,node:n,project:'latency',state:'running',batchId:'batch-current',itemId:v.name})
  return {...n,item_id:v.name,batch_id:'batch-current',execution_state:v.state,persistence_state:'saved',error_message:v.error,result:{...n,attempt_id:'test-'+v.name,node_type:'ss',requested_at:'2026-10-15T06:00:00Z',finished_at:'2026-10-15T06:00:01Z',status:v.status,latency_ms:123,total_samples:6,success_samples:v.ok,failure_samples:v.bad,samples:[],persistence_state:'saved'} as LatencyTest}
 })
 // A newer result for the same node must not replace this queue item's own evidence.
 w.mergeLatency({...items[0].result,attempt_id:'unrelated-newer',requested_at:'2026-10-15T07:00:00Z',success_samples:6,failure_samples:0,status:'completed'})
 w.absorbBatch({batch_id:'batch-current',request_id:'req-current',state:'completed',requested_at:'2026-10-15T06:00:00Z',item_count:3,items})
 attach(QueueModal)
 const rows=Array.from(document.querySelectorAll('.queue-row')).map(el=>new DOMWrapper(el))
 expect(rows[0].find('.badge').text()).toBe('部分响应 · 5/6')
 expect(rows[0].find('.badge').classes()).toContain('warn')
 expect(rows[0].find('.badge').classes()).not.toContain('good')
 expect(rows[0].text()).toContain('EOF')
 expect(rows[1].find('.badge').text()).toBe('无响应 · 0/6')
 expect(rows[1].find('.badge').classes()).toContain('bad')
 expect(rows[2].find('.badge').text()).toBe('已响应 · 6/6')
 expect(rows[2].find('.badge').classes()).toContain('good')
 expect(w.done.value).toBe(3)
 expect(document.querySelector('.queue-counts')?.textContent).toContain('已处理 3 / 3')
})


it('does not turn cancellation, missing results or save failure into successful coverage',()=>{
 const result:LatencyTest={...node,attempt_id:'test-a',display_name:'Node A',node_type:'ss',requested_at:'2026-10-15T06:00:00Z',finished_at:'2026-10-15T06:00:01Z',status:'completed',latency_ms:123,total_samples:1,success_samples:1,failure_samples:0,samples:[],persistence_state:'saved'}
 const items=[
  {...node,item_id:'cancelled',batch_id:'batch-a',display_name:'cancelled',execution_state:'cancelled',persistence_state:'saved',result},
  {...node,item_id:'missing',batch_id:'batch-a',display_name:'missing',execution_state:'completed',persistence_state:'saved'},
  {...node,item_id:'save-failed',batch_id:'batch-a',display_name:'save-failed',execution_state:'completed',persistence_state:'failed',persistence_error:'disk full',result},
 ]
 w.queue.value=items.map(i=>({id:i.item_id,node,project:'latency',state:'running',batchId:'batch-a',itemId:i.item_id}))
 w.absorbBatch({batch_id:'batch-a',request_id:'request-a',state:'completed',requested_at:result.requested_at,item_count:3,items})
 attach(QueueModal)
 const badges=Array.from(document.querySelectorAll('.queue-row .badge'))
 expect(badges.map(b=>b.textContent)).toEqual(['已取消','已结束 · 无采样','保存失败'])
 expect(badges.every(b=>!b.classList.contains('good'))).toBe(true)
 expect(document.querySelector('.queue-counts')?.textContent).toContain('保存失败 1')
 expect(document.body.textContent).toContain('重试保存')
})

it('labels a saved local IP-version failure as unexecuted in the queue and its totals',()=>{
 const target='https://speed.cloudflare.com/__down?bytes=1',at='2026-10-15T08:33:30Z'
 const result:LatencyTest={...node,attempt_id:'legacy-ip-family',display_name:'Node A',node_type:'Hysteria2',requested_at:at,finished_at:at,status:'failed',persistence_state:'saved',latency_ms:0,total_samples:1,success_samples:0,failure_samples:1,samples:[{seq:1,target,timestamp:at,success:false,latency_ms:0,error:`Get "${target}": ip version error`}]}
 w.queue.value=[{id:'family',node,project:'latency',state:'running',batchId:'family-batch',itemId:'family-item'}]
 w.absorbBatch({batch_id:'family-batch',request_id:'family',state:'completed_with_issues',requested_at:at,item_count:1,items:[{...node,item_id:'family-item',batch_id:'family-batch',display_name:'Node A',execution_state:'failed',persistence_state:'saved',result}]})
 attach(QueueModal)
 expect(document.querySelector('.queue-row .badge')?.textContent).toBe('未执行 · 条件不足')
 expect(document.querySelector('.queue-row .badge')?.classList.contains('bad')).toBe(false)
 expect(document.querySelector('.queue-counts')?.textContent).toContain('未运行/取消 1')
 expect(w.done.value).toBe(1)
})

it('shows exact download outcomes and speeds instead of green task completion',()=>{
 const at='2026-10-15T09:27:25Z'
 const success:Attempt={...node,attempt_id:'download-success',request_id:'req-success',requested_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://speed.cloudflare.com/__down?bytes=10485760'},result:{outcome:'completed',bytes_read:10485760,duration_ns:2000000000,finished_at:at}}
 const failed:Attempt={...node,attempt_id:'download-failed',request_id:'req-failed',requested_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://speed.cloudflare.com/__down?bytes=10485760'},result:{outcome:'transport_error',bytes_read:0,duration_ns:500000000,finished_at:at,error_message:'connection reset'}}
 const timeout:Attempt={...node,attempt_id:'download-timeout',request_id:'req-timeout',requested_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://speed.cloudflare.com/__down?bytes=10485760'},result:{outcome:'timed_out',bytes_read:0,duration_ns:5000000000,finished_at:at}}
 const rateLimited:Attempt={...node,attempt_id:'download-429',request_id:'req-429',requested_at:at,execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://speed.cloudflare.com/__down?bytes=10485760'},result:{outcome:'source_rate_limited',http_status:429,bytes_read:0,duration_ns:200000000,finished_at:at}}
 const saveFailed:Attempt={...node,attempt_id:'download-save-fail',request_id:'req-save-fail',requested_at:at,execution_state:'completed',persistence_state:'failed',persistence_error:'disk write failed',rule:{target_url:'https://speed.cloudflare.com/__down?bytes=10485760'},result:{outcome:'completed',bytes_read:5242880,duration_ns:1000000000,finished_at:at}}
 w.queue.value=[success,failed,timeout,rateLimited,saveFailed].map(a=>({id:a.attempt_id,node,project:'download',state:a.execution_state,attempt:a,error:a.persistence_error||a.result?.error_message}))
 attach(QueueModal)
 const rows=Array.from(document.querySelectorAll('.queue-row')).map(el=>new DOMWrapper(el))
 expect(rows[0].find('.badge').text()).toBe('5.00 MB/s · 10.0MB')
 expect(rows[0].find('.badge').classes()).toContain('good')
 expect(rows[1].find('.badge').text()).toBe('连接或读取失败 · 未确认')
 expect(rows[1].find('.badge').classes()).not.toContain('good')
 expect(rows[1].find('.badge').classes()).toContain('warn')
 expect(rows[2].find('.badge').text()).toBe('超时 · 未读到下载数据')
 expect(rows[2].find('.badge').classes()).not.toContain('good')
 expect(rows[2].find('.badge').classes()).toContain('warn')
 expect(rows[3].find('.badge').text()).toBe('下载源限流')
 expect(rows[3].find('.badge').classes()).toContain('warn')
 expect(rows[4].find('.badge').text()).toBe('保存失败')
 expect(rows[4].find('.badge').classes()).toContain('bad')
 expect(rows[4].text()).toContain('重试保存')
})

it('defaults to one explicit service observation and retains optional repeated sampling',async()=>{
 w.project.value='service';w.serviceIds.value=['cloudflare_204'];w.openPlan([node]);attach(TestPlanModal);await flushPromises()
 const field=Array.from(document.querySelectorAll('dialog label')).map(el=>new DOMWrapper(el)).find(l=>l.text().includes('每服务采样次数'))
 expect(field).toBeDefined();expect(field!.find('select').element.value).toBe('1')
 await field!.find('select').setValue('4');await flushPromises()
 expect(document.querySelector('.plan-budget')?.textContent).toContain('服务采样 4 次')
 await field!.find('select').setValue('1');await flushPromises()
 expect(document.querySelector('.plan-budget')?.textContent).toContain('服务采样 1 次')
 vi.mocked(api.startAttempt).mockImplementation(async(_kind,body:any)=>({...node,attempt_id:'single-service',request_id:body.request_id,service_id:body.service_id,requested_at:'2026-10-18T05:00:00Z',execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://cp.cloudflare.com/generate_204'},result:{outcome:'matched',http_status:204,bytes_read:0,finished_at:'2026-10-18T05:00:01Z'}}))
 button('开始检测').click();await flushPromises()
 expect(api.startAttempt).toHaveBeenCalledTimes(1)
 expect(api.startAttempt).toHaveBeenCalledWith('service',expect.objectContaining({profile_id:'sub-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',service_id:'cloudflare_204',timeout_seconds:15,drift_samples:1}))
 expect(w.queue.value[0].attempt?.persistence_state).toBe('saved')
})

it('offers one Netflix choice and retains the temporary alias for history only',async()=>{
 w.project.value='service';w.serviceIds.value=['netflix_unlock'];
 const rule={service_id:'netflix_unlock',name:'Netflix',category:'Streaming',description:'Automatic selected-node DNS',result_kind:'streaming_unlock',success_criterion:'',timeout_seconds:15}
 w.catalog.value=[rule,{...rule,service_id:'netflix_unlock_ipv4',name:'Netflix temporary alias',superseded_by:'netflix_unlock'}];w.openPlan([node]);attach(TestPlanModal);await flushPromises()
 const labels=Array.from(document.querySelectorAll('.service-picker-list label'))
 expect(labels).toHaveLength(1);expect(labels[0].textContent).toContain('Netflix');expect(labels[0].textContent).not.toContain('temporary alias')
 expect(w.catalog.value).toHaveLength(2);expect(w.plan.value?.serviceIds).toEqual(['netflix_unlock'])
})

it('does not pre-select IP reputation when the catalog offers no reputation items',()=>{
  w.catalog.value=[];w.project.value='latency';w.openPlan([node])
  expect(w.plan.value).toMatchObject({includeReputation:false,reputationServiceIds:[]})
})

it('omits off-region unlock items from both the round plan and progress',async()=>{
 const jp={...node,node_key:'jp',node_identity_key:'jp',country_code:'JP'},overseas={...node,node_key:'us',node_identity_key:'us',country_code:'JP'}
 w.nodes.value=[jp,overseas];w.services.value[key(overseas)]=[{...overseas,attempt_id:'geo',service_id:'google_region',rule:{service_id:'google_region'},result:{outcome:'profiled',details:{service_region:'US'}}} as unknown as Attempt]
 w.project.value='service';w.serviceIds.value=['chatgpt_unlock','abema_unlock'];w.openPlan([jp,overseas]);w.plan.value!.includeReputation=false
 vi.mocked(api.startAttempt).mockImplementation(async(_kind,body:any)=>savedDownload(body))
 await w.runPlan()
 expect(w.queue.value.map(q=>[q.node.node_key,q.serviceId])).toEqual([['jp','chatgpt_unlock'],['jp','abema_unlock'],['us','chatgpt_unlock']])
 expect((vi.mocked(api.createRound).mock.calls[0][0] as {items:unknown[]}).items).toHaveLength(3)
 w.japanOnly.value=false;expect(w.allowedUnlockServices(overseas,['chatgpt_unlock','abema_unlock'])).toEqual(['chatgpt_unlock','abema_unlock'])
})
