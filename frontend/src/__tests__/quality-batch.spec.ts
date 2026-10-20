import {expect,it,vi} from 'vitest'
import {readQuality} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {key,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
import {api} from '../api'
vi.mock('../nodeQuality',()=>({readQuality:vi.fn()}))
vi.mock('../api',()=>({api:{revisions:vi.fn(async()=>[])}}))
it('reads all 128 nodes with at most six active reads',async()=>{
 let active=0,peak=0
 vi.mocked(readQuality).mockImplementation(async()=>{peak=Math.max(peak,++active);await Promise.resolve();active--;return {since:'since',until:'until',tests:[],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true} as QualityEvidence})
 const nodes=Array.from({length:128},(_,i)=>({profile_id:'p',node_identity_key:String(i),config_revision_key:'r'}) as NodeOption)
 const data=await readQualityBatch(nodes,'since','until')
 expect(peak).toBe(6);expect(Object.keys(data)).toHaveLength(128);expect(data[key(nodes[127])].complete).toBe(true);expect(readQuality).toHaveBeenCalledTimes(128)
})
it('reads the saved revision scope used by node history after a subscription update',async()=>{
 const node={profile_id:'p',node_key:'new-key',node_identity_key:'i',config_revision_key:'new'} as NodeOption
 vi.mocked(api.revisions).mockResolvedValueOnce([{node_key:'old-key',config_revision_key:'old',display_name:'N',last_observed_at:'2026-10-18T00:00:00Z'}])
 vi.mocked(readQuality).mockImplementation(async n=>({since:'since',until:'until',tests:n.config_revision_key==='old'?[{attempt_id:'saved',config_revision_key:'old'}]:[],monitor:[],downloads:[],services:[],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true}) as unknown as QualityEvidence)
 const data=await readQualityBatch([node],'since','until')
 expect(readQuality).toHaveBeenLastCalledWith({...node,node_key:'old-key',config_revision_key:'old'},'since','until');expect(data[key(node)].tests[0].attempt_id).toBe('saved');expect(data[key(node)].complete).toBe(true)
})
