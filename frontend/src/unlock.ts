import {type Attempt,type ServiceRule} from './domain'
import {servicePassed} from './sharedResults'
import type {IpType} from './ipKind'
import config from './unlock-config.json'
export const antigravityBatchRisk=(nodes:number,ids:string[])=>nodes>3&&ids.includes('antigravity')?`Antigravity 将用同一账号从 ${nodes} 个节点的 IP 发起请求，有风控风险，也会消耗额度`:''
export const unlockGroups=config.groups
export const unlockGroup=(id:string)=>unlockGroups.find(g=>(g.ids as readonly string[]).includes(id))?.id||'legacy'
export const unlockSupported=(id:string)=>id!=='disney_unlock'
const datacenterHint=(ipType?:IpType)=>ipType==='idc'?'。出口是机房 IP，这类服务常拒绝或要求验证':''
// 报告只保存 outcome，这里的中文原因表供报告复用，口径与解锁检测一致。
export const outcomeReasons:Record<string,string>={challenge:'验证挑战',suspected_challenge:'疑似验证挑战',rate_limited:'网站限流',credentials_required:'需要凭据',auth_failed:'凭据失效',permission_denied:'账号权限不足',transport_error:'连接或读取失败',connection_failed:'连接失败',timed_out:'连接超时'}
export function unlockResult(a?:Attempt,serviceId?:string,ipType?:IpType){
 const id=a?.service_id||a?.rule.service_id||serviceId
 const r=a?.result,details=r?.details||{},region=String(details.service_region||details.country_code||details.region||'').toUpperCase(),o=r?.outcome
 if(id==='disney_unlock'&&!servicePassed(a))return {state:'unknown',tone:'empty',label:'暂不支持',reason:r?'当前自动检测暂不可用，旧记录保留':'',region}
 if(!r)return {state:'unknown',tone:'empty',label:a?.started_at?'无法判断':'未测',reason:a?.started_at?'检测未完成':'',region}
 if(id==='antigravity'){
  const model=String(details.checked_model||'').replace(/^models\//,'')
  const progress=String(details.antigravity_progress||'')
  const googleRegion=String(details.google_region||'').toUpperCase()
  const cfRegion=String(details.country_code||'').toUpperCase()
  const googleHint=googleRegion?` · Google 判定 ${googleRegion}`:''
  const exitHint=cfRegion?` · CF 观察 ${cfRegion}（不代表 Google）`:''
  let netHint=''
  try{const np=details.network_path?JSON.parse(String(details.network_path)):null;if(np?.socket_bind_verified&&np?.interface)netHint=` · ${np.interface} 绕过 TUN`}catch{}
  const reply=String(details.reply_preview||'').trim()
  const modelNote=reply?`（回答“${reply}”）`:''
  const ttfbHint=details.google_ttfb_ms?` · Google 响应 ${details.google_ttfb_ms}ms`:''
  const blockedHint=details.blocked_ips?` · 受阻出口 ${details.blocked_ips}`:''
  if(servicePassed(a))return {state:'available',tone:'good',label:model?`模型可用（${model}）`:'模型可用',reason:(progress||'真实模型已回答')+modelNote+ttfbHint+googleHint+exitHint+netHint,region:googleRegion||region}
  if(o==='flapping')return {state:'unavailable',tone:'warn',label:'偶发送中 / 漂移',reason:`${details.sample_success||0}/${details.sample_total||0} 次通过${blockedHint}${ttfbHint}${googleHint}${exitHint}${netHint}`,region:googleRegion||region}
  if(o==='region_blocked')return {state:'unavailable',tone:'warn',label:googleRegion==='CN'?'出口送中（CN）':'地区不支持',reason:(r.error_message||'Google 返回当前地区不支持')+googleHint+exitHint+netHint,region:googleRegion||region}
  if(o==='service_rejected')return {state:'unavailable',tone:'warn',label:'策略拦截',reason:(r.error_message||'模型请求被服务策略拦截')+googleHint+exitHint+netHint,region:googleRegion||region}
  const agLabels:Record<string,string>={credentials_required:'需要凭据',auth_failed:'凭据失效',setup_required:'账号待开通',permission_denied:'权限不足',rate_limited:'额度受限'}
  if(o&&agLabels[o])return {state:'unknown',tone:'warn',label:agLabels[o],reason:r.error_message||agLabels[o],region:googleRegion||region}
 }
 if(id==='cloudflare_challenge'){
  const detail=String(details.cf_challenge_detail||'').trim()
  if(o==='profiled')return {state:'available',tone:'good',label:'未触发 CF 盾',reason:detail||r.summary||'代表性 Cloudflare 站点均直通',region}
  if(o==='verification_required')return {state:'verification',tone:'warn',label:'部分弹 CF 盾',reason:detail||r.summary||'部分严格站点触发 Cloudflare 人机验证',region}
  if(o==='challenge')return {state:'unavailable',tone:'bad',label:'易弹 CF 人机验证',reason:detail||r.summary||'多数或全部站点返回 Cloudflare 挑战',region}
 }
 const google=['gemini_unlock','youtube_premium','google_region'].includes(a?.service_id||a?.rule.service_id||'')
 if(o==='verification_required'||(google&&/https:\/\/(?:[^/]+\.)?google\.com\/(?:sorry|recaptcha)/i.test(String(details.final_url||''))))return {state:'verification',tone:'warn',label:'需人机验证',reason:'Google 对该 IP 要求验证，实际使用时也会遇到'+(id==='gemini_unlock'?datacenterHint(ipType):''),region}
 if(google&&region==='CN'&&o==='region_blocked')return {state:'unavailable',tone:'bad',label:'Google 判定为中国大陆',reason:r.summary||'Google 判定为中国大陆',region}
 if((a?.service_id||a?.rule.service_id)==='youtube_premium'&&o==='region_blocked')return {state:'unavailable',tone:'warn',label:'不可用',reason:'YouTube 不向该 IP 提供 Premium（常见于机房/代理 IP）'+datacenterHint(ipType),region}
 if((a?.service_id||a?.rule.service_id)==='gemini_unlock'&&o==='region_blocked'&&region)return {state:'unavailable',tone:'warn',label:'不可用（'+region+'）',reason:r.summary||'该地区不可用',region}
 if(o==='banned')return {state:'unavailable',tone:'bad',label:'封禁',reason:r.summary||r.error_message||'服务封禁该 IP',region}
 const spotify=(a?.service_id||a?.rule.service_id)==='spotify_unlock'
 if(spotify&&o==='service_rejected'&&/代理|proxy/i.test(r.summary||r.error_message||''))return {state:'partial',tone:'warn',label:'限注册',reason:'识别为代理 IP，已有账号通常不受影响'+datacenterHint(ipType),region}
 const netflix=['netflix_unlock','netflix_unlock_ipv4'].includes(a?.service_id||a?.rule.service_id||'')
 if(netflix&&details.title_check_version!=='dual_title_v1'&&['region_blocked','region_limited','originals_only'].includes(o||''))return {state:'unknown',tone:'empty',label:'仅自制剧或不可用（旧检测）',reason:'未验证账号播放',region}
 if(o==='originals_only')return {state:'partial',tone:'warn',label:'仅自制剧'+(region?'（'+region+'）':''),reason:'未验证账号播放',region}
 if(servicePassed(a))return {state:'available',tone:'good',label:netflix?'完整'+(region?'（'+region+'）':''):'可用 · '+(region||'地区未返回'),reason:netflix?'未验证账号播放':r.summary||'检测规则通过',region}
 if(['region_blocked','region_limited','service_rejected'].includes(o||''))return {state:'unavailable',tone:netflix?'bad':'warn',label:'不可用',reason:r.summary||r.error_message||'地区不支持',region}
 return {state:'unknown',tone:'empty',label:'无法判断',reason:outcomeReasons[o||'']||r.summary||r.error_message||'未取得明确结论',region}
}
export const unlockMeasured=(state:string)=>['available','unavailable','partial','verification'].includes(state)
export type ServiceRegions=Record<string,string[]>
export function applicableRegions(id:string,rules:ServiceRegions={}){return rules[id]??unlockGroups.find(g=>(g.ids as string[]).includes(id))?.regions??[]}
// A leading "!" marks an excluded region; a service's list is either all included or all excluded.
export const excludedRegion=(code:string)=>code.startsWith('!')
export function unlockServiceSkipReason(id:string,region:string,enabled=true,rules:ServiceRegions={}){
 const regions=applicableRegions(id,rules)
 if(!enabled||!regions.length)return ''
 if(regions.some(excludedRegion))return regions.includes('!'+region)?'已排除此地区，不检测':''
 if(regions.includes(region))return ''
 return regions.length===1&&regions[0]==='JP'?'非日本节点，不检测':'不适用于此地区，不检测'
}
export const unlockServiceAllowed=(id:string,region:string,enabled=true,rules:ServiceRegions={})=>!unlockServiceSkipReason(id,region,enabled,rules)
export function unlockGroupSkipReason(groupId:string,region:string,enabled=true,rules:ServiceRegions={}){
 const ids=unlockGroups.find(g=>g.id===groupId)?.ids||[]
 return ids.length&&ids.every(id=>!unlockServiceAllowed(id,region,enabled,rules))?unlockServiceSkipReason(ids[0]!,region,enabled,rules):''
}
export function unlockGroupSummary(groupId:string,catalog:Pick<ServiceRule,'service_id'|'name'>[],lookup:(id:string)=>Attempt|undefined,region?:string,enabled=true,rules:ServiceRegions={},ipType?:IpType){
 const services=catalog.filter(s=>unlockGroup(s.service_id)===groupId),eligible=services.filter(s=>region===undefined||unlockServiceAllowed(s.service_id,region,enabled,rules))
 if((services.length&&!eligible.length)||(!services.length&&region!==undefined&&unlockGroupSkipReason(groupId,region,enabled,rules)))return {available:0,total:0,tone:'empty',label:'—',reason:'',title:services.length?unlockServiceSkipReason(services[0]!.service_id,region!,enabled,rules):unlockGroupSkipReason(groupId,region!,enabled,rules)}
 const list=eligible.map(s=>({...s,result:unlockResult(lookup(s.service_id),s.service_id,ipType)})),available=list.filter(s=>s.result.state==='available').length,unavailable=list.filter(s=>unlockMeasured(s.result.state)&&s.result.state!=='available')
 const total=available+unavailable.length
 return {available,total,tone:unavailable.length?'warn':available?'good':'empty',label:total?available+'/'+total:'未测',reason:unavailable.map(s=>s.name+'：'+(s.result.state==='partial'?s.result.label:s.result.reason)).join('；'),title:list.map(s=>s.name+'：'+s.result.label+' · '+s.result.reason).join('\n')}
}
export function unlockSummary(catalog:Pick<ServiceRule,'service_id'|'name'>[],lookup:(id:string)=>Attempt|undefined,region?:string,enabled=true,rules:ServiceRegions={},ipType?:IpType){
 const groups=unlockGroups.map(g=>({...g,...unlockGroupSummary(g.id,catalog,lookup,region,enabled,rules,ipType)}))
 const measured=groups.some(g=>g.total)
 return {tone:groups.some(g=>g.reason)?'warn':measured?'good':'empty',label:measured?groups.map(g=>g.name+' '+g.label+(g.total?' 可用':'')).join(' · '):'还没做解锁检测',detail:groups.find(g=>g.reason)?.reason||(measured?'依据当前时间范围内已确认的结果':'无法判断与尚未检测不计入统计')}
}
