import {type Attempt,type ServiceRule} from './domain'
import {servicePassed} from './sharedResults'
export const unlockGroups=[{id:'ai',name:'AI',ids:['chatgpt_unlock','claude_unlock','gemini_unlock','antigravity']},{id:'streaming',name:'流媒体',ids:['netflix_unlock','youtube_premium','disney_unlock','prime_video','spotify_unlock','tiktok_unlock']},{id:'japan',name:'日本',ids:['abema_unlock','unext_unlock','dmm_unlock','radiko_unlock','hulu_jp_unlock','fod_unlock']}] as const
export const unlockGroup=(id:string)=>unlockGroups.find(g=>(g.ids as readonly string[]).includes(id))?.id||'legacy'
export function unlockResult(a?:Attempt){
 const r=a?.result,details=r?.details||{},region=String(details.service_region||details.country_code||details.region||'').toUpperCase(),o=r?.outcome
 if(!r)return {state:'unknown',tone:'empty',label:'无法判断',reason:a?'检测未完成':'尚未检测',region}
 if(o==='originals_only')return {state:'available',tone:'good',label:'可用 · 仅自制'+(region?' · '+region:''),reason:'仅 Netflix 自制内容',region}
 if(servicePassed(a))return {state:'available',tone:'good',label:'可用 · '+((a?.service_id||a?.rule.service_id)==='netflix_unlock'?'完整 · ':'')+(region||'地区未返回'),reason:r.summary||'检测规则通过',region}
 if(['region_blocked','region_limited','service_rejected'].includes(o||''))return {state:'unavailable',tone:'warn',label:'不可用',reason:r.summary||r.error_message||'地区不支持',region}
 const reasons:Record<string,string>={challenge:'验证挑战',suspected_challenge:'疑似验证挑战',rate_limited:'网站限流',credentials_required:'需要凭据',auth_failed:'凭据失效',permission_denied:'账号权限不足',transport_error:'连接或读取失败',connection_failed:'连接失败',timed_out:'连接超时'}
 return {state:'unknown',tone:'empty',label:'无法判断',reason:reasons[o||'']||r.summary||r.error_message||'未取得明确结论',region}
}
export function unlockGroupSummary(groupId:string,catalog:Pick<ServiceRule,'service_id'|'name'>[],lookup:(id:string)=>Attempt|undefined){
 const list=catalog.filter(s=>unlockGroup(s.service_id)===groupId).map(s=>({...s,result:unlockResult(lookup(s.service_id))})),available=list.filter(s=>s.result.state==='available').length,unavailable=list.filter(s=>s.result.state==='unavailable')
 const total=available+unavailable.length
 return {available,total,tone:unavailable.length?'warn':available?'good':'empty',label:total?available+'/'+total:'未测',reason:unavailable.map(s=>s.name+'：'+s.result.reason).join('；'),title:list.map(s=>s.name+'：'+s.result.label+' · '+s.result.reason).join('\n')}
}

export function unlockSummary(catalog:Pick<ServiceRule,'service_id'|'name'>[],lookup:(id:string)=>Attempt|undefined){
 const groups=unlockGroups.map(g=>({...g,...unlockGroupSummary(g.id,catalog,lookup)}))
 const measured=groups.some(g=>g.total)
 return {tone:groups.some(g=>g.reason)?'warn':measured?'good':'empty',label:measured?groups.map(g=>g.name+' '+g.label+(g.total?' 可用':'')).join(' · '):'还没做解锁检测',detail:groups.find(g=>g.reason)?.reason||(measured?'依据当前时间范围内已确认的结果':'无法判断与尚未检测不计入统计')}
}
