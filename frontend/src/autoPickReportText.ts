import {groupServices,type AutoPickReport} from './autoPick'
import {outcomeReasons} from './unlock'

// 报告只保存服务 ID 与 outcome，没有地区与详细原因：这里只翻译已存下的信息。
const serviceOrder=['chatgpt_unlock','claude_unlock','gemini_unlock','antigravity','netflix_unlock','youtube_premium','disney_unlock','prime_video','spotify_unlock','tiktok_unlock']
const shortLabels:Record<string,string>={chatgpt_unlock:'ChatGPT',claude_unlock:'Claude',gemini_unlock:'Gemini',antigravity:'Antigravity',netflix_unlock:'Netflix',youtube_premium:'YouTube Premium',disney_unlock:'Disney+',prime_video:'Prime Video',spotify_unlock:'Spotify',tiktok_unlock:'TikTok'}
const fullLabels:Record<string,string>={}
for(const group of Object.values(groupServices))for(const service of group)fullLabels[service.id]=service.label

export function reportServiceLabel(id:string){return shortLabels[id]||fullLabels[id]||id}
// 与后端 autoPickServiceUsable 一致。
export const reportServicePassed=(outcome?:string)=>outcome==='unlocked'||outcome==='matched'

const verdictLabels:Record<string,string>={unlocked:'可用',matched:'可用',verification_required:'需人机验证',originals_only:'仅自制剧',banned:'封禁',service_rejected:'服务拒绝',challenge:'需完成验证',suspected_challenge:'疑似需验证',region_blocked:'地区受限',region_limited:'地区受限',rule_missing:'缺少检测规则',not_executed:'未执行',cancelled:'已取消',user_cancelled:'已取消',interrupted:'已中断',unknown:'无法判断'}

export function reportServiceVerdict(id:string,outcome?:string){
  if(!outcome)return '未记录'
  if(id==='youtube_premium'&&(outcome==='region_blocked'||outcome==='region_limited'))return '不向该 IP 提供'
  return verdictLabels[outcome]||outcomeReasons[outcome]||'未记录'
}

function order(id:string){const index=serviceOrder.indexOf(id);return index<0?serviceOrder.length:index}

// 有问题的项目排前面，其余按固定服务顺序，长文字交给 CSS 换行。
export function reportServiceText(services?:Record<string,string>){
  const entries=Object.entries(services||{})
  if(!entries.length)return ''
  entries.sort((a,b)=>Number(reportServicePassed(a[1]))-Number(reportServicePassed(b[1]))||order(a[0])-order(b[0]))
  return entries.map(([id,outcome])=>`${reportServiceLabel(id)} ${reportServiceVerdict(id,outcome)}`).join(' · ')
}

const gapLabel=(group:string)=>/^[\x20-\x7E]+$/.test(group)?`未入 ${group}：`:`未入${group}：`

// 按这份报告保存的 required_services、节点 services 与 purposes 判断；缺依据只写“未记录”。
export function reportPurposeText(node:{services?:Record<string,string>;purposes?:string[]},rules?:AutoPickReport['rules']){
  const groups=rules?.groups||[]
  if(!groups.length)return ''
  const purposes=node.purposes||[],services=node.services||{},lines:string[]=[]
  for(const group of groups){
    if(purposes.includes(group))continue
    const required=rules?.required_services?.[group]||[]
    const failed=required.filter(id=>services[id]&&!reportServicePassed(services[id]))
    lines.push(failed.length?gapLabel(group)+failed.map(id=>`${reportServiceLabel(id)} ${reportServiceVerdict(id,services[id])}`).join('、'):gapLabel(group)+'未记录')
  }
  return lines.length?lines.join('；'):'所选用途条件均通过'
}
