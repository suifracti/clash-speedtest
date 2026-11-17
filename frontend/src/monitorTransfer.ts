import type {MonitorCreate,MonitorJob,NodeOption} from './domain'

export const monitorConfigByteLimit=1024*1024
const probeNames:Record<string,string>={latency_four_v1:'四站延迟',latency_six_v1:'六站延迟',light:'轻量检测',service:'服务检测',heavy:'深度检测'}
const tierNames:Record<string,string>={regular:'常规',focus:'重点',sparse:'低频'}
export const monitorProbeName=(value:string)=>probeNames[value]||value
export const monitorTierName=(value:string)=>tierNames[value]||value
export interface MonitorImportRow {request:MonitorCreate;profileName:string;error:string}

const record=(value:unknown):value is Record<string,unknown>=>typeof value==='object'&&value!==null&&!Array.isArray(value)
const nonempty=(value:unknown):value is string=>typeof value==='string'&&value.trim().length>0
const seconds=(value:unknown,max:number):value is number=>typeof value==='number'&&Number.isInteger(value)&&value>=1&&value<=max

function parseJob(value:unknown,index:number):MonitorCreate{
  const fail=(reason:string):never=>{throw new Error(`第 ${index+1} 项任务：${reason}`)}
  if(!record(value)||typeof value.name!=='string'||value.name.length>200||!nonempty(value.profile_id))return fail('任务名称或订阅账户无效')
  if(typeof value.probe_set!=='string'||!Object.prototype.hasOwnProperty.call(probeNames,value.probe_set)||typeof value.sampling_tier!=='string'||!Object.prototype.hasOwnProperty.call(tierNames,value.sampling_tier))return fail('检测方案或采样档位不受支持')
  if(!seconds(value.interval_seconds,86400)||!seconds(value.timeout_seconds,120))return fail('间隔须为 1–86400 秒、超时须为 1–120 秒的整数')
  if(!Array.isArray(value.node_keys)||!value.node_keys.length||value.node_keys.length>500||!value.node_keys.every(nonempty))return fail('须包含 1–500 个有效节点')
  const keys=value.node_keys as string[]
  if(new Set(keys.map(key=>key.trim())).size!==keys.length)return fail('节点不能重复')
  if(!Array.isArray(value.node_contexts)||value.node_contexts.length!==keys.length)return fail('节点身份与配置版本不完整')
  const contexts=new Map<string,MonitorCreate['node_contexts'][number]>()
  for(const context of value.node_contexts){
    if(!record(context)||!nonempty(context.node_key)||!nonempty(context.node_identity_key)||!nonempty(context.config_revision_key))return fail('节点身份与配置版本不完整')
    if(contexts.has(context.node_key))return fail('节点身份不能重复')
    contexts.set(context.node_key,{node_key:context.node_key,node_identity_key:context.node_identity_key,config_revision_key:context.config_revision_key})
  }
  if(keys.some(key=>!contexts.has(key)))return fail('节点身份与所选节点不一致')
  // Deliberately copy only the creation contract. A file cannot carry source
  // credentials, scheduler IDs, running state or resume-on-launch settings.
  return {name:value.name,profile_id:value.profile_id,node_keys:[...keys],node_contexts:keys.map(key=>contexts.get(key)!),probe_set:value.probe_set,sampling_tier:value.sampling_tier,interval_seconds:value.interval_seconds,timeout_seconds:value.timeout_seconds}
}

export function parseMonitorConfig(text:string):MonitorCreate[]{
  if(new TextEncoder().encode(text).byteLength>monitorConfigByteLimit)throw new Error('配置文件不能超过 1 MiB')
  let value:unknown
  try{value=JSON.parse(text.replace(/^\uFEFF/,''))}catch{throw new Error('无法读取 JSON，请选择导出的监测任务配置文件')}
  if(!record(value)||value.format!=='speedtest-monitor'||value.version!==1||!Array.isArray(value.jobs)||!value.jobs.length||value.jobs.length>100)throw new Error('请选择有效的监测任务配置文件（版本 1，1–100 项任务）')
  return value.jobs.map(parseJob)
}

export function serializeMonitorConfig(jobs:MonitorJob[]):string{
  const requests=jobs.map((job,index)=>{
    const nodes=new Map(job.nodes?.map(node=>[node.node_key,node])||[])
    const value={name:job.name,profile_id:job.profile_id,node_keys:job.node_keys,node_contexts:job.node_keys?.map(key=>{const node=nodes.get(key);return {node_key:key,node_identity_key:node?.node_identity_key,config_revision_key:node?.config_revision_key}}),probe_set:job.probe_set,sampling_tier:job.sampling_tier,interval_seconds:job.interval_seconds,timeout_seconds:job.timeout_seconds}
    return parseJob(value,index)
  })
  const text=JSON.stringify({format:'speedtest-monitor',version:1,jobs:requests},null,2)
  // An exported file must be accepted by this same version of the importer.
  parseMonitorConfig(text)
  return text
}

export function inspectMonitorImport(requests:MonitorCreate[],nodes:NodeOption[]):MonitorImportRow[]{
  const byAccount=new Map<string,Map<string,NodeOption>>()
  for(const node of nodes){let account=byAccount.get(node.profile_id);if(!account){account=new Map();byAccount.set(node.profile_id,account)}account.set(node.node_key,node)}
  return requests.map(request=>{
    const account=byAccount.get(request.profile_id)
    let error=account?'':'当前没有此订阅账户的节点，请先导入对应订阅并更新缓存'
    for(const context of request.node_contexts){
      if(error)break
      const node=account!.get(context.node_key)
      if(!node)error='节点已缺失，请更新订阅缓存或从原任务重新导出'
      else if(node.node_identity_key!==context.node_identity_key)error='节点身份已变化，请核对原任务与当前订阅'
      else if(node.config_revision_key!==context.config_revision_key)error='节点配置已变化，请核对原任务与当前订阅'
    }
    return {request,profileName:account?.values().next().value?.profile_name||request.profile_id,error}
  })
}
