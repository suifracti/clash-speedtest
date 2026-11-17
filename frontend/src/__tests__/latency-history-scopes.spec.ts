import {beforeEach,expect,it,vi} from 'vitest'
import {readLatencyHistoryWithConfig,latencyRounds} from '../latencyHistory'
import {scope,type NodeOption,type LatencyTest} from '../domain'

const mocks=vi.hoisted(()=>({scopes:vi.fn(),batches:vi.fn(),batch:vi.fn(),revisions:vi.fn(),latency:vi.fn(),monitor:vi.fn()}))
vi.mock('../api',()=>({api:{latencyHistoryScopes:mocks.scopes,batches:mocks.batches,batch:mocks.batch,revisions:mocks.revisions,latencyHistory:mocks.latency,monitorSamples:mocks.monitor}}))
const node=(id:string):NodeOption=>({profile_id:'account-a',node_key:id,node_identity_key:id,config_revision_key:'revision-'+id,display_name:'Same node',profile_name:'Airport',type:'ss',country_code:'',country_flag:''})
beforeEach(()=>{
 vi.resetAllMocks()
 mocks.batches.mockResolvedValue([]);mocks.revisions.mockResolvedValue([]);mocks.monitor.mockResolvedValue({items:[],has_more:false})
 mocks.latency.mockImplementation(async(n:NodeOption)=>{
  const at=new Date(Date.now()-3600000).toISOString()
  const test:LatencyTest={...n,attempt_id:n.node_key,display_name:n.display_name,node_type:n.type,requested_at:at,finished_at:at,status:'completed',latency_ms:100,total_samples:1,success_samples:1,failure_samples:0,persistence_state:'saved',samples:[{seq:1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:at,latency_ms:100,success:true}]}
  return {tests:[test],has_more:false}
 })
})
it('finds saved older identities without relying on the recent batch list',async()=>{
 mocks.scopes.mockResolvedValue([scope(node('old')),scope(node('current'))])
 const result=await readLatencyHistoryWithConfig(node('current'),168)
 expect(latencyRounds(result.series)).toHaveLength(2)
 expect(mocks.latency.mock.calls.map(([n])=>n.node_identity_key).sort()).toEqual(['current','old'])
 expect(mocks.scopes).toHaveBeenCalledWith(node('current'),expect.objectContaining({since:expect.any(String),until:expect.any(String)}))
 expect(mocks.batches).not.toHaveBeenCalled()
 expect(result.series[0].points.filter(p=>p.oldConfig)).toHaveLength(1)
})
it('discovers newly saved scopes on a later read and never caches a failed lookup as empty history',async()=>{
 mocks.scopes.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error('history unavailable')).mockResolvedValueOnce([scope(node('previous'))])
 await readLatencyHistoryWithConfig(node('current'),168)
 await expect(readLatencyHistoryWithConfig(node('current'),168)).rejects.toThrow('history unavailable')
 const recovered=await readLatencyHistoryWithConfig(node('current'),168)
 expect(latencyRounds(recovered.series)).toHaveLength(2)
 expect(mocks.scopes).toHaveBeenCalledTimes(3)
})
it('keeps different identities separate even when a legacy revision key is shared',async()=>{
 const current={...node('current'),config_revision_key:'shared'},old={...node('old'),config_revision_key:'shared'}
 mocks.scopes.mockResolvedValue([scope(old),scope(current)])
 const result=await readLatencyHistoryWithConfig(current,168)
 expect(latencyRounds(result.series)).toHaveLength(2)
 expect(result.series[0].points.filter(p=>p.oldConfig)).toHaveLength(1)
 expect(result.changes).toHaveLength(1)
})
