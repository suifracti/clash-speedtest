export interface MonitorStorageUsage {total_bytes?:number;database_bytes?:number;wal_bytes?:number;shared_memory_bytes?:number;warning_bytes?:number;hard_bytes?:number;warning?:boolean;protected?:boolean}
export interface RetentionRequest {policy:string;custom_days:number;cutoff_time?:string}
export interface RetentionPreview {policy:string;cutoff:string;samples_to_delete:number;runs_to_delete:number;storage?:MonitorStorageUsage}
export interface RetentionResult {samples_deleted?:number;runs_deleted?:number;partial?:boolean;error_message?:string}
export const validRetentionCount=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0
export const retentionCount=(value:unknown)=>validRetentionCount(value)?value.toLocaleString('zh-CN'):'未知数量的'
export const retentionSummary=(result:RetentionResult)=>`${retentionCount(result.samples_deleted)} 条采样、${retentionCount(result.runs_deleted)} 条运行记录`
export function checkedRetentionPreview(value:unknown,request:RetentionRequest):RetentionPreview{
  const p=value as RetentionPreview|null
  if(!p||p.policy!==request.policy||!['30d','90d','180d','custom'].includes(p.policy)||typeof p.cutoff!=='string'||!Number.isFinite(Date.parse(p.cutoff))||!validRetentionCount(p.samples_to_delete)||!validRetentionCount(p.runs_to_delete))throw new Error('清理预览不完整，请重新读取后确认；尚未执行清理。')
  if(request.cutoff_time&&p.cutoff!==request.cutoff_time)throw new Error('清理预览的时间边界已变化，请返回后重新选择。')
  return p
}
export function retentionFailure(cause:unknown):string{
  const message=cause instanceof Error?cause.message:String(cause)
  const details=(cause as {details?:RetentionResult})?.details
  if(details?.partial)return `部分清理：已删除 ${retentionSummary(details)}。${message}。请重新预览剩余记录；已删除的记录不能撤销。`
  // Older native cores only return a RetentionError string. Keep that evidence
  // visible without inferring exact counts from unstructured text.
  if(message.includes('retention partially applied'))return `部分清理：${message}。请重新预览剩余记录；已删除的记录不能撤销。`
  return `未能确认清理结果：${message}。请重新预览剩余记录后再操作。`
}
