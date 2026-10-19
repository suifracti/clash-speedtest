import {describe,expect,it} from 'vitest'
import {compareLatencyItem} from '../historyCompare'
import type {BatchItem,LatencyBatch,LatencyTest} from '../domain'

// 合成数据：只验证“和上次比”的判定规则。
const path={method:'physical_socket_v3',interface:'en0',dns_mode:'physical_interface_dns_v1',dns_bind_verified:true,socket_bind_verified:true,tcp_bindings:1,udp_bindings:0,address_family:'ipv4',address_source:'dns'}
const node={profile_id:'p',node_key:'k',node_identity_key:'id',config_revision_key:'r'}
const test=(ms:number,ok=1,extra:Partial<LatencyTest>={}):LatencyTest=>({...node,attempt_id:'a'+ms,display_name:'合成',node_type:'ss',target:'cloudflare',method:'http_head',method_version:2,network_path:path,requested_at:'',finished_at:'',status:'completed',latency_ms:ms,total_samples:3,success_samples:ok,failure_samples:3-ok,samples:[],persistence_state:'saved',...extra})
const item=(id:string,result?:LatencyTest,other=node):BatchItem=>({...other,item_id:id,batch_id:id,display_name:'合成',execution_state:'completed',persistence_state:'saved',result})
const batch=(id:string,at:string,items:BatchItem[],target='cloudflare'):LatencyBatch=>({batch_id:id,request_id:id,target_id:target,state:'completed',requested_at:at,item_count:items.length,items})

describe('compare with the last same-condition record (synthetic data)',()=>{
  const now=batch('now','2026-10-20T03:00:00Z',[item('now',test(120))])
  it('reports a numeric change only when conditions match',()=>{
    const older=[batch('b1','2026-10-19T03:00:00Z',[item('b1',test(162))])]
    expect(compareLatencyItem(now,now.items![0],older)).toMatchObject({state:'better',text:'比上次少 42 ms',previousBatch:'b1'})
  })
  it('does not skip the latest failure to find an older success',()=>{
    const older=[batch('fail','2026-10-19T05:00:00Z',[item('fail',test(0,0))]),batch('ok','2026-10-18T05:00:00Z',[item('ok',test(90))])]
    expect(compareLatencyItem(now,now.items![0],older)).toMatchObject({state:'regained',previousBatch:'fail'})
  })
  it('flags lost responses and refuses to rank different conditions',()=>{
    const lostNow=batch('n2','2026-10-20T04:00:00Z',[item('n2',test(0,0))])
    expect(compareLatencyItem(lostNow,lostNow.items![0],[now])).toMatchObject({state:'lost',text:'本次未取得响应（上次 120 ms）'})
    const other=[batch('v6','2026-10-19T03:00:00Z',[item('v6',test(300,1,{network_path:{...path,address_family:'ipv6'}}))])]
    expect(compareLatencyItem(now,now.items![0],other).state).toBe('different')
  })
  it('ignores other targets, other revisions and not-executed items',()=>{
    const older=[batch('t','2026-10-19T03:00:00Z',[item('t',test(100))],'google'),batch('rev','2026-10-19T02:00:00Z',[item('rev',test(100),{...node,config_revision_key:'r2'})]),batch('skip','2026-10-19T01:00:00Z',[item('skip',test(100,0,{total_samples:0}))])]
    expect(compareLatencyItem(now,now.items![0],older)).toMatchObject({state:'none',text:'已读取范围内未找到上次同条件记录'})
  })
})
