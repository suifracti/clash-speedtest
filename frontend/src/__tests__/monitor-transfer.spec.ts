import {describe,expect,it} from 'vitest'
import {inspectMonitorImport,parseMonitorConfig,serializeMonitorConfig} from '../monitorTransfer'
import type {MonitorCreate,MonitorJob,NodeOption} from '../domain'

const node:NodeOption={profile_id:'account-a',profile_name:'示例机场 / 日常账户',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',display_name:'示例节点',type:'ss',country_code:'HK',country_flag:''}
const config=(patch:Partial<MonitorCreate>={}):MonitorCreate=>({name:'日常观察',profile_id:node.profile_id,node_keys:[node.node_key],node_contexts:[{node_key:node.node_key,node_identity_key:node.node_identity_key,config_revision_key:node.config_revision_key}],probe_set:'latency_four_v1',sampling_tier:'regular',interval_seconds:300,timeout_seconds:10,...patch})
const file=(jobs:unknown[])=>JSON.stringify({format:'speedtest-monitor',version:1,jobs})
const job=(patch:Partial<MonitorJob>={}):MonitorJob=>({...config(),id:'job-a',profile_name:node.profile_name,nodes:[node],state:'running',resume_on_launch:true,updated_at:'2026-11-18T00:00:00Z',...patch})

describe('monitor task file compatibility and identity',()=>{
  it.each(['light','service','heavy','latency_four_v1','latency_six_v1'])('accepts the current and legacy %s probe scope without rewriting it',probe=>{
    const requests=parseMonitorConfig(file([config({probe_set:probe})]))
    expect(requests).toEqual([config({probe_set:probe})])
    expect(inspectMonitorImport(requests,[node])[0]).toMatchObject({profileName:node.profile_name,error:''})
  })

  it('exports only task configuration and frozen identity, excluding credentials and execution state',()=>{
    const frozen={...node,config_revision_key:'frozen-revision',raw_config:{password:'private-example'}}
    const text=serializeMonitorConfig([{...job({nodes:[frozen]}),url:'https://example.invalid/private',token:'private-token'} as MonitorJob])
    expect(JSON.parse(text)).toEqual({format:'speedtest-monitor',version:1,jobs:[config({node_contexts:[{node_key:node.node_key,node_identity_key:node.node_identity_key,config_revision_key:'frozen-revision'}]})]})
    expect(text).not.toMatch(/private|running|resume_on_launch|raw_config/)
    expect(inspectMonitorImport(parseMonitorConfig(text),[node])[0].error).toContain('配置已变化')
  })

  it('never substitutes the same name or identity from another account or a new node key',()=>{
    const requests=[config()]
    expect(inspectMonitorImport(requests,[{...node,profile_id:'account-b'}])[0].error).toContain('订阅账户')
    expect(inspectMonitorImport(requests,[{...node,node_key:'new-key'}])[0].error).toContain('节点已缺失')
    expect(inspectMonitorImport(requests,[{...node,node_identity_key:'new-identity'}])[0].error).toContain('身份已变化')
  })

  it('retains all rows and reports an invalid task separately from compatible tasks',()=>{
    const result=inspectMonitorImport([config(),config({name:'已移除来源',profile_id:'missing-account'})],[node])
    expect(result).toHaveLength(2)
    expect(result[0].error).toBe('')
    expect(result[1].error).toContain('订阅账户')
  })

  it.each([
    {node_keys:['node-a','node-a']},
    {node_contexts:[]},
    {node_contexts:[{node_key:'wrong-key',node_identity_key:'identity-a',config_revision_key:'revision-a'}]},
    {node_contexts:[{node_key:'node-a',node_identity_key:'',config_revision_key:'revision-a'}]},
    {sampling_tier:'normal'},
    {interval_seconds:86401},
    {timeout_seconds:121},
    {interval_seconds:1.5},
    {profile_id:''},
  ])('rejects malformed or unsupported task settings before any import: %j',patch=>{
    expect(()=>parseMonitorConfig(file([config(patch)]))).toThrow(/第 1 项/)
  })

  it('rejects invalid envelopes, oversized input and incomplete frozen export contexts',()=>{
    for(const text of ['null','[]','{}','not json',file([]),file(Array(101).fill(config())),JSON.stringify({format:'speedtest-monitor',version:2,jobs:[config()]})])expect(()=>parseMonitorConfig(text)).toThrow()
    expect(()=>parseMonitorConfig(' '.repeat(1024*1024+1))).toThrow('1 MiB')
    expect(()=>serializeMonitorConfig([job({nodes:[]})])).toThrow(/节点身份/)
  })

  it('strips unrecognized import fields instead of forwarding credentials or start flags',()=>{
    const parsed=parseMonitorConfig(file([{...config(),resume_on_launch:true,state:'running',raw_config:'private-example'}]))
    expect(parsed).toEqual([config()])
  })
})
