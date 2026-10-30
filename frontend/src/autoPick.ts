export type AutoPickSchedule='6h'|'12h'|'daily'
export interface AutoPickConfig {
  enabled:boolean
  schedule:AutoPickSchedule
  daily_hour:number
  airport_ids:string[]
  region_filter?:string
  include_ai:boolean
  include_streaming:boolean
  ai_required:string[]
  streaming_required:string[]
  min_response_rate:number
  max_latency_ms:number
  sample_count:number
  last_run_at?:string
  next_run_at?:string
  updated_at?:string
}
export interface AutoPickEstimate {nodes:number;projects:number;requests:number;warn:boolean}
export interface AutoPickSubscription {url:string;path:string;exists:boolean;updated_at?:string;node_count:number}
export interface AutoPickReportMeta {
  report_id:string;created_at:string;trigger:string;status:string;summary:string
  selected_count:number;daily_count:number;streaming_count:number;ai_count:number;kept_previous:boolean
}
export interface AutoPickNode {
  display_name:string;profile_id:string;airport_id:string;airport_name?:string
  node_key:string;node_identity_key:string;config_revision_key:string
  latency_ms:number;response_rate:number;samples:number;success_samples:number
  purposes:string[];services?:Record<string,string>;excluded_reason?:string
}
export interface AutoPickReport extends AutoPickReportMeta {
  batch_id?:string
  scope:{airport_ids:string[];airport_names?:string[];region_filter?:string}
  rules:{min_response_rate:number;max_latency_ms:number;groups?:string[];required_services?:Record<string,string[]>;sample_count:number}
  measured:number;latency_passed:number;group_counts:Record<string,number>
  nodes:AutoPickNode[]
  notes?:string[]
  subscription_path?:string
  previous_report_id?:string;previous_at?:string
}
export interface AutoPickStatus {
  config:AutoPickConfig
  running:boolean
  last_error?:string
  estimate:AutoPickEstimate
  subscription:AutoPickSubscription
  last_report?:AutoPickReport
  reports?:AutoPickReportMeta[]
}
export const scheduleLabels:Record<AutoPickSchedule,string>={ '6h':'每 6 小时','12h':'每 12 小时',daily:'每天固定时间'}
export const reportStatusLabels:Record<string,string>={selected:'成功',kept_previous:'沿用上次结果',failed:'本轮失败'}
// 与 core/autopick 的 GroupServices / DefaultRequiredServices 保持一致。
export const groupServices:Record<'ai'|'streaming',{id:string;label:string}[]>={
  ai:[
    {id:'chatgpt_unlock',label:'ChatGPT'},
    {id:'claude_unlock',label:'Claude'},
    {id:'gemini_unlock',label:'Gemini'}
  ],
  streaming:[
    {id:'netflix_unlock',label:'Netflix 完整片库'},
    {id:'youtube_premium',label:'YouTube Premium'},
    {id:'disney_unlock',label:'Disney+'}
  ]
}
export const defaultRequiredServices:{ai:string[];streaming:string[]}={ai:['chatgpt_unlock','claude_unlock'],streaming:['netflix_unlock']}
