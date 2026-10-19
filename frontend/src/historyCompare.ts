import type {BatchItem,LatencyBatch,LatencyTest} from './domain'

// 「和上次比」：在已读取的批次里找同一节点（订阅、身份、配置版本）、同一检测目标的最近一次记录。
// 不跳过最近的失败去找成功；未执行（没有任何采样）不算一次记录。条件证据不一致时只并排原始事实，不判断升降。
export type DeltaState='better'|'worse'|'same'|'lost'|'regained'|'both_none'|'different'|'none'
export interface LatencyDelta {state:DeltaState;text:string;tone:string;current?:number;previous?:number;previousAt?:string;previousBatch?:string}

const identity=(n:Pick<BatchItem,'profile_id'|'node_identity_key'|'config_revision_key'>)=>`${n.profile_id}\u0000${n.node_identity_key}\u0000${n.config_revision_key}`
const known=(v:unknown)=>typeof v==='string'&&!!v&&v!=='unknown'&&v!=='unverified'
export function latencyCondition(t:LatencyTest){
 const p=t.network_path
 const key=JSON.stringify([t.method||'unknown',t.method_version??'unknown',t.target||'legacy',p?.method||'unverified',p?.interface||'unknown',p?.dns_mode||'unknown',p?.dns_bind_verified??'unknown',p?.socket_bind_verified??'unknown',p?.address_family||'unknown',p?.address_source||'unknown'])
 const verified=known(t.method)&&t.method_version!==undefined&&known(p?.method)&&known(p?.interface)&&known(p?.dns_mode)&&p?.dns_bind_verified===true&&p?.socket_bind_verified===true&&known(p?.address_family)&&known(p?.address_source)
 return {key,verified}
}
const measured=(item?:BatchItem)=>!!item?.result&&item.result.total_samples>0
const value=(t?:LatencyTest)=>t&&t.success_samples>0&&Number.isFinite(t.latency_ms)?t.latency_ms:undefined
const ms=(n:number)=>`${Math.round(n)} ms`

/** 返回当前批次之前（按请求时间）最近的同节点同目标记录。older 需按时间从新到旧排列。 */
export function previousItem(current:LatencyBatch,item:BatchItem,older:LatencyBatch[]){
 const id=identity(item)
 for(const b of older){
  if(b.batch_id===current.batch_id||Date.parse(b.requested_at)>=Date.parse(current.requested_at))continue
  if((b.target_id||'')!==(current.target_id||''))continue
  const found=b.items?.find(i=>identity(i)===id&&measured(i))
  if(found)return {batch:b,item:found}
 }
 return undefined
}

export function compareLatencyItem(current:LatencyBatch,item:BatchItem,older:LatencyBatch[]):LatencyDelta{
 if(!measured(item))return {state:'none',text:'本次未执行，不比较',tone:'empty'}
 const prev=previousItem(current,item,older)
 if(!prev)return {state:'none',text:'已读取范围内未找到上次同条件记录',tone:'empty'}
 const cur=value(item.result),old=value(prev.item.result),base={current:cur,previous:old,previousAt:prev.batch.requested_at,previousBatch:prev.batch.batch_id}
 if(cur===undefined&&old===undefined)return {...base,state:'both_none',text:'本次和上次都没有取得响应',tone:'bad'}
 if(cur===undefined)return {...base,state:'lost',text:`本次未取得响应（上次 ${ms(old!)}）`,tone:'bad'}
 if(old===undefined)return {...base,state:'regained',text:`本次取得响应 ${ms(cur)}；上次没有响应`,tone:'good'}
 const a=latencyCondition(item.result!),b=latencyCondition(prev.item.result!)
 if(!a.verified||!b.verified||a.key!==b.key)return {...base,state:'different',text:`条件不同，不判断升降 · 本次 ${ms(cur)} · 上次 ${ms(old)}`,tone:'empty'}
 const diff=cur-old
 if(Math.abs(diff)<Math.max(5,old*.03))return {...base,state:'same',text:`与上次相近（${ms(cur)} / 上次 ${ms(old)}）`,tone:''}
 return diff<0?{...base,state:'better',text:`比上次少 ${ms(-diff)}`,tone:'good'}:{...base,state:'worse',text:`比上次多 ${ms(diff)}`,tone:'warn'}
}
