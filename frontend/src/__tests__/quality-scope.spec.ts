import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import HomeView from '../components/HomeView.vue'
import {createWorkspace,workspaceKey} from '../workspace'
import {api} from '../api'
import {key,type LatencyTest,type NetworkPath,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'

const readQualityMock=vi.hoisted(()=>vi.fn())
const localStorageItems=vi.hoisted(()=>new Map<string,string>())
vi.mock('../nodeQuality',async importOriginal=>{const actual=await importOriginal<typeof import('../nodeQuality')>();return {...actual,readQuality:readQualityMock}})
vi.mock('../api',async importOriginal=>{const actual=await importOriginal<typeof import('../api')>();return {...actual,api:{...actual.api,refreshJobs:vi.fn(async()=>[]),latencyHistory:vi.fn(async()=>({tests:[],has_more:false,complete:true})),measurementRounds:vi.fn(async()=>[]),createRound:vi.fn(async()=>undefined),finishRound:vi.fn(async()=>undefined),monitorSamples:vi.fn(async()=>({items:[],has_more:false,next_cursor:''})),latencySummaries:vi.fn(async()=>[]),attemptHistory:vi.fn(async()=>({attempts:[],has_more:false,complete:true})),jobs:vi.fn(async()=>[]),createJob:vi.fn(),jobAction:vi.fn(async()=>{}),startLatency:vi.fn(),startAttempt:vi.fn(),attempt:vi.fn(),attemptAction:vi.fn()}}})
vi.hoisted(()=>Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(k:string)=>localStorageItems.get(k)||null,setItem:(k:string,v:string)=>localStorageItems.set(k,v),removeItem:(k:string)=>localStorageItems.delete(k),clear:()=>localStorageItems.clear()}}))

const stale:NodeOption={profile_id:'sub-a',node_key:'stale',node_identity_key:'stale-identity',config_revision_key:'stale-revision',profile_name:'机场 A',display_name:'旧范围节点',type:'trojan',country_code:'HK',country_flag:''}
const fresh=Array.from({length:21},(_,i):NodeOption=>({profile_id:'sub-b',node_key:`b-${i+1}`,node_identity_key:`b-identity-${i+1}`,config_revision_key:`b-revision-${i+1}`,profile_name:'机场 B',display_name:`香港 ${String(i+1).padStart(2,'0')}`,type:'trojan',country_code:'HK',country_flag:''}))
const at='2026-10-10T00:00:00Z',target='https://speed.cloudflare.com/__down?bytes=1'
let wrapper:VueWrapper|undefined,w:ReturnType<typeof createWorkspace>|undefined

function evidence(node:NodeOption,value:number,complete=true):QualityEvidence{
 const path:NetworkPath={method:'physical_socket_v1',interface:'en1',dns_mode:'physical_interface_dns_v1',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:'IPv4',address_source:'A'}
 const test:LatencyTest={...node,attempt_id:`latency-${node.node_key}`,display_name:node.display_name,node_type:node.type,source:'workbench',method:'http_get_via_proxy_first_byte',method_version:1,network_path:path,target,requested_at:at,finished_at:at,status:'completed',latency_ms:value,total_samples:5,success_samples:5,failure_samples:0,persistence_state:'saved',samples:Array.from({length:5},(_,i)=>({seq:i+1,target,timestamp:at,latency_ms:value+i,success:true}))}
 const reads={latency:{complete},monitor:{complete:true},download:{complete:true},service:{complete:true}}
 return {tests:[test],monitor:[],downloads:[],services:[],reads,complete:Object.values(reads).every(read=>read.complete),since:'',until:at}
}

beforeEach(()=>{localStorageItems.clear();localStorageItems.set('speedtest-home-view','quick');window.sessionStorage.clear();vi.clearAllMocks();HTMLDialogElement.prototype.showModal=vi.fn();HTMLDialogElement.prototype.close=vi.fn();vi.stubGlobal('ResizeObserver',class{observe(){}disconnect(){}})})
afterEach(()=>{wrapper?.unmount();w?.dispose();wrapper=undefined;w=undefined;document.body.innerHTML='';vi.unstubAllGlobals()})

it('loads and sorts the full new node scope, then ignores a late result from the previous scope',async()=>{
 let resolveStale!:(value:QualityEvidence)=>void
 const staleResult=new Promise<QualityEvidence>(resolve=>resolveStale=resolve)
 readQualityMock.mockImplementation((node:NodeOption)=>node.node_identity_key==='stale-identity'?staleResult:Promise.resolve(evidence(node,Number(node.node_key.slice(2))*10)))
 w=createWorkspace();w.airports.value=[{id:'airport-a',name:'机场 A',url_display:'',node_count:1,has_cache:true,subscriptions:[{id:'sub-a',airport_id:'airport-a',name:'默认',url_display:'',url_configured:true,node_count:1,has_cache:true}]},{id:'airport-b',name:'机场 B',url_display:'',node_count:fresh.length,has_cache:true,subscriptions:[{id:'sub-b',airport_id:'airport-b',name:'默认',url_display:'',url_configured:true,node_count:fresh.length,has_cache:true}]}];w.nodes.value=[stale,...fresh];w.setAirports(['airport-a'])
 wrapper=mount(HomeView,{attachTo:document.body,global:{provide:{[workspaceKey as symbol]:w}}})
 await wrapper.get('select[aria-label="质量证据筛选"]').setValue('samples')
 await wrapper.get('select[aria-label="质量分组排序"]').setValue('p50')
 w.setAirports(['airport-b'])
 await vi.waitFor(()=>expect(new Set(readQualityMock.mock.calls.filter(call=>call[0].profile_id==='sub-b').map(call=>call[0].node_identity_key)).size).toBe(fresh.length))
 await flushPromises()
 expect(readQualityMock.mock.calls.filter(call=>call[0].profile_id==='sub-b').every(call=>call[2]===readQualityMock.mock.calls.find(old=>old[0].node_identity_key==='stale-identity')?.[2])).toBe(true)
 expect(wrapper.get('[role="status"]').text()).toContain(`已读固定范围 ${fresh.length} 个节点`)
 expect(wrapper.get('[role="status"]').text()).toContain(`当前符合 ${fresh.length} 个`)
 expect(wrapper.findAll('.node-name strong').map(node=>node.text()).slice(0,3)).toEqual(['香港 · 01','香港 · 02','香港 · 03'])
 resolveStale(evidence(stale,1,false));await flushPromises()
 expect(wrapper.get('[role="status"]').text()).toContain('存在缺页/读取错误 0 个')
 expect(wrapper.get('[role="status"]').text()).toContain(`已读固定范围 ${fresh.length} 个节点`)
 expect(wrapper.findAll('.node-name strong').map(node=>node.text()).slice(0,3)).toEqual(['香港 · 01','香港 · 02','香港 · 03'])
 expect(api.startLatency).not.toHaveBeenCalled();expect(api.startAttempt).not.toHaveBeenCalled()
})
