import {afterEach,expect,it,vi} from 'vitest'
import {nextTick} from 'vue'
import {flushPromises} from '@vue/test-utils'
import {api} from '../api'
import {readSortSnapshot} from '../sortSnapshot'
import {createWorkspace} from '../workspace'
import {key,scope,type Attempt,type LatencyTest,type NodeOption} from '../domain'
vi.mock('../api',async original=>{const actual=await original<typeof import('../api')>();return {...actual,api:{...actual.api,latencyHistory:vi.fn(),monitorSamples:vi.fn(),attemptHistory:vi.fn()}}})
const n:NodeOption={profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',profile_name:'p',display_name:'n',type:'ss',country_code:'HK',country_flag:''}
const at='2026-10-09T00:00:00Z'
const download=(id:string,speed:number):Attempt=>({...n,attempt_id:id,request_id:id,requested_at:at,display_name:'n',node_type:'ss',execution_state:'completed',persistence_state:'saved',rule:{target_url:'https://example.test',method:'GET',rule_version:1,maximum_bytes:10485760,maximum_duration_ns:10000000000,network_path_method:'physical_socket_v1',physical_interface:'en1',dns_mode:'physical_interface_dns_v1'},result:{outcome:'time_limit',bytes_read:speed*1048576,duration_ns:1e9,finished_at:at,network_path:{method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:'IPv4',address_source:'A'}}})
afterEach(()=>vi.clearAllMocks())
it('reads every identity with one cutoff and each service filter before limiting',async()=>{
 vi.mocked(api.attemptHistory).mockResolvedValue({attempts:[],has_more:false,complete:true})
 const nodes=Array.from({length:25},(_,i)=>({...n,node_identity_key:'id'+i,node_key:'node'+i}))
 const result=await readSortSnapshot(nodes,'service','google',['one','two'],at,at,()=>{})
 expect(Object.keys(result.snapshot)).toHaveLength(25)
 expect(api.attemptHistory).toHaveBeenCalledTimes(50)
 for(const [kind,node,q] of vi.mocked(api.attemptHistory).mock.calls){expect(kind).toBe('service');expect(q).toMatchObject({since:at,until:at,limit:1});expect(['one','two']).toContain(q?.service_id);expect(node.config_revision_key).toBe('r')}
})
it('uses the newer same-site stopped-monitor round including refusal latency',async()=>{
 vi.mocked(api.latencyHistory).mockResolvedValue({tests:[],has_more:false,complete:true})
 vi.mocked(api.monitorSamples).mockResolvedValue({items:[{...n,sample_id:'s',run_id:'m',timestamp:at,display_name_snapshot:'n',target:'https://api.github.com/zen',probe_type:'rtt',success:true,latency:368e6,ttfb:0,sampling_tier:'regular',error_detail:'HTTP 403'}],has_more:false,next_cursor:''})
 const result=await readSortSnapshot([n],'latency','github',[],at,at,()=>{})
 expect(result.snapshot[key(n)].latency).toMatchObject({value:368,tone:'warn',source:'monitor'})
 expect(api.latencyHistory).toHaveBeenCalledWith(n,{target_id:'github',since:at,until:at,limit:1})
 expect(api.monitorSamples).toHaveBeenCalledWith(expect.objectContaining({...scope(n),probe_type:'rtt',target:'https://api.github.com/zen'}))
})
it('freezes ranking across later history merges and updates only on explicit refresh',async()=>{
 const w=createWorkspace(),other={...n,node_key:'other',node_identity_key:'other'}
 w.airports.value=[{id:'airport',name:'a',url_display:'',node_count:2,has_cache:true,subscriptions:[{id:'p',airport_id:'airport',name:'s',url_display:'',url_configured:true,node_count:2,has_cache:true}]}];w.nodes.value=[n,other];w.setAirports(['airport']);w.sort.value='download'
 vi.mocked(api.attemptHistory).mockImplementation(async(_kind,node)=>({attempts:[{...download(node.node_key,node.node_key==='n'?5:2),...node}],has_more:false,complete:true}))
 await nextTick();await w.refreshSort()
 expect(w.filteredNodes.value.map(key)).toEqual([key(n),key(other)])
 w.mergeAttempt('download',{...download('new',20),...other})
 expect(w.filteredNodes.value.map(key)).toEqual([key(n),key(other)])
 expect(w.summaryDownload(other)?.result?.bytes_read).toBe(2*1048576)
 vi.mocked(api.attemptHistory).mockImplementation(async(_kind,node)=>({attempts:[{...download(node.node_key,node.node_key==='n'?5:20),...node}],has_more:false,complete:true}))
 await w.refreshSort();expect(w.filteredNodes.value.map(key)).toEqual([key(other),key(n)]);w.dispose()
})

it('does not numerically rank saved latency values when their route evidence is missing',async()=>{
 const first={...n,node_key:'first',node_identity_key:'first',display_name:'香港 01'},second={...n,node_key:'second',node_identity_key:'second',display_name:'香港 02'}
 const legacy=(node:NodeOption,id:string,value:number):LatencyTest=>({...node,attempt_id:id,display_name:node.display_name,node_type:'vless',requested_at:at,finished_at:at,status:'completed',latency_ms:value,total_samples:1,success_samples:1,failure_samples:0,samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,latency_ms:value,success:true}],persistence_state:'saved',source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1})
 const w=createWorkspace();w.airports.value=[{id:'airport',name:'fixture',url_display:'',node_count:2,has_cache:true,subscriptions:[{id:'p',airport_id:'airport',name:'fixture',url_display:'',url_configured:true,node_count:2,has_cache:true}]}];w.nodes.value=[first,second];w.setAirports(['airport']);w.sort.value='latency'
 vi.mocked(api.latencyHistory).mockResolvedValue({tests:[],has_more:false,complete:true});vi.mocked(api.monitorSamples).mockResolvedValue({items:[],has_more:false,next_cursor:''})
 await nextTick();await flushPromises()
 w.sortSnapshots.value={[key(first)]:{services:[],latency:{id:'first',time:at,value:100,tone:'good',source:'manual',conditionKey:'unknown',description:'legacy',latencyTests:[legacy(first,'first-test',100)]}},[key(second)]:{services:[],latency:{id:'second',time:at,value:1,tone:'good',source:'manual',conditionKey:'unknown',description:'legacy',latencyTests:[legacy(second,'second-test',1)]}}}
 expect(w.filteredNodes.value.map(key)).toEqual([key(first),key(second)])
 w.dispose()
})
