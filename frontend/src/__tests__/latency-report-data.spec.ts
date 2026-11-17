import {beforeEach,expect,it,vi} from 'vitest'
import {api,request} from '../api'
import {readLatencyReports} from '../latencyReportData'
import type {LatencyBatch} from '../domain'
vi.mock('../api',()=>({api:{batches:vi.fn(),batch:vi.fn()},request:vi.fn(),mapLimited:async(items:any[],_limit:number,fn:(item:any)=>Promise<any>)=>Promise.all(items.map(fn))}))
const batch:LatencyBatch={batch_id:'manual',request_id:'manual',state:'completed',requested_at:'2026-11-18T08:00:00Z',item_count:0,items:[]}
beforeEach(()=>vi.resetAllMocks())
it('retains saved manual reports when the monitor run directory fails',async()=>{
 vi.mocked(api.batches).mockResolvedValue([batch]);vi.mocked(request).mockRejectedValue(new Error('monitor offline'))
 const read=await readLatencyReports([])
 expect(read.reports).toEqual([{batch,monitor:false}]);expect(read.errors.join()).toContain('monitor offline')
})
it('keeps a valid report beside a failed batch and never treats a partial run as complete',async()=>{
 vi.mocked(api.batches).mockResolvedValue([batch,{...batch,batch_id:'bad',items:undefined}]);vi.mocked(api.batch).mockRejectedValue(new Error('batch unavailable'))
 vi.mocked(request).mockImplementation(async(path)=>{if(path.includes('/runs'))return [{run_id:'run',started_at:batch.requested_at}];throw new Error('samples unavailable')})
 const read=await readLatencyReports([])
 expect(read.reports).toEqual([{batch,monitor:false}]);expect(read.errors).toHaveLength(2);expect(read.errors.join()).toContain('samples unavailable')
})
it('keeps monitor reports when manual history is unavailable',async()=>{
 vi.mocked(api.batches).mockRejectedValue(new Error('manual offline'))
 vi.mocked(request).mockImplementation(async(path)=>path.includes('/runs')?[{run_id:'run',started_at:batch.requested_at,status:'completed'}]:[{profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',probe_type:'rtt',timestamp:batch.requested_at,display_name_snapshot:'Node',success:true,latency:1000000,target:'https://www.gstatic.com/generate_204'}])
 const read=await readLatencyReports([])
 expect(read.reports).toHaveLength(1);expect(read.reports[0].monitor).toBe(true);expect(read.errors.join()).toContain('manual offline')
})
