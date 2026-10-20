import {expect,it,vi} from 'vitest'
import {readQuality} from '../nodeQuality'
import {readQualityBatch} from '../qualityBatch'
import {key,type NodeOption} from '../domain'
import type {QualityEvidence} from '../nodeQuality'
vi.mock('../nodeQuality',()=>({readQuality:vi.fn()}))
it('reads all 128 nodes with at most six active reads',async()=>{
 let active=0,peak=0
 vi.mocked(readQuality).mockImplementation(async()=>{peak=Math.max(peak,++active);await Promise.resolve();active--;return {complete:true} as QualityEvidence})
 const nodes=Array.from({length:128},(_,i)=>({profile_id:'p',node_identity_key:String(i),config_revision_key:'r'}) as NodeOption)
 const data=await readQualityBatch(nodes,'since','until')
 expect(peak).toBe(6);expect(Object.keys(data)).toHaveLength(128);expect(data[key(nodes[127])].complete).toBe(true);expect(readQuality).toHaveBeenCalledTimes(128)
})
