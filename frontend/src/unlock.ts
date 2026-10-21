import {type Attempt,type ServiceRule} from './domain'
import {servicePassed} from './sharedResults'
import config from './unlock-config.json'
export const unlockGroups=config.groups
export const unlockGroup=(id:string)=>unlockGroups.find(g=>(g.ids as readonly string[]).includes(id))?.id||'legacy'
export function unlockResult(a?:Attempt){
 const r=a?.result,details=r?.details||{},region=String(details.service_region||details.country_code||details.region||'').toUpperCase(),o=r?.outcome
 if(!r)return {state:'unknown',tone:'empty',label:a?.started_at?'无法判断':'未测',reason:a?.started_at?'检测未完成':'',region}
 const netflix=['netflix_unlock','netflix_unlock_ipv4'].includes(a?.service_id||a?.rule.service_id||'')
 if(netflix&&details.title_check_version!=='dual_title_v1'&&['region_blocked','region_limited','originals_only'].includes(o||''))return {state:'unknown',tone:'empty',label:'仅自制剧或不可用（旧检测）',reason:'未验证账号播放',region}
 if(o==='originals_only')return {state:'available',tone:'warn',label:'仅自制剧'+(region?'（'+region+'）':''),reason:'未验证账号播放',region}
 if(servicePassed(a))return {state:'available',tone:'good',label:netflix?'完整'+(region?'（'+region+'）':''):'可用 · '+(region||'地区未返回'),reason:netflix?'未验证账号播放':r.summary||'检测规则通过',region}
 if(['region_blocked','region_limited','service_rejected'].includes(o||''))return {state:'unavailable',tone:netflix?'bad':'warn',label:'不可用',reason:r.summary||r.error_message||'地区不支持',region}
 const reasons:Record<string,string>={challenge:'验证挑战',suspected_challenge:'疑似验证挑战',rate_limited:'网站限流',credentials_required:'需要凭据',auth_failed:'凭据失效',permission_denied:'账号权限不足',transport_error:'连接或读取失败',connection_failed:'连接失败',timed_out:'连接超时'}
 return {state:'unknown',tone:'empty',label:'无法判断',reason:reasons[o||'']||r.summary||r.error_message||'未取得明确结论',region}
}
export type ServiceRegions=Record<string,string[]>
export function applicableRegions(id:string,rules:ServiceRegions={}){return rules[id]??unlockGroups.find(g=>(g.ids as string[]).includes(id))?.regions??[]}
export function unlockServiceSkipReason(id:string,region:string,enabled=true,rules:ServiceRegions={}){
 const regions=applicableRegions(id,rules)
 if(!enabled||!regions.length||regions.includes(region))return ''
 return regions.length===1&&regions[0]==='JP'?'非日本节点，不检测':'不适用于此地区，不检测'
}
export const unlockServiceAllowed=(id:string,region:string,enabled=true,rules:ServiceRegions={})=>!unlockServiceSkipReason(id,region,enabled,rules)
export function unlockGroupSkipReason(groupId:string,region:string,enabled=true,rules:ServiceRegions={}){
 const ids=unlockGroups.find(g=>g.id===groupId)?.ids||[]
 return ids.length&&ids.every(id=>!unlockServiceAllowed(id,region,enabled,rules))?unlockServiceSkipReason(ids[0]!,region,enabled,rules):''
}
export function unlockGroupSummary(groupId:string,catalog:Pick<ServiceRule,'service_id'|'name'>[],lookup:(id:string)=>Attempt|undefined,region?:string,enabled=true,rules:ServiceRegions={}){
 const services=catalog.filter(s=>unlockGroup(s.service_id)===groupId),eligible=services.filter(s=>region===undefined||unlockServiceAllowed(s.service_id,region,enabled,rules))
 if((services.length&&!eligible.length)||(!services.length&&region!==undefined&&unlockGroupSkipReason(groupId,region,enabled,rules)))return {available:0,total:0,tone:'empty',label:'—',reason:'',title:services.length?unlockServiceSkipReason(services[0]!.service_id,region!,enabled,rules):unlockGroupSkipReason(groupId,region!,enabled,rules)}
 const list=eligible.map(s=>({...s,result:unlockResult(lookup(s.service_id))})),available=list.filter(s=>s.result.state==='available').length,unavailable=list.filter(s=>s.result.state==='unavailable')
 const total=available+unavailable.length
 return {available,total,tone:unavailable.length?'warn':available?'good':'empty',label:total?available+'/'+total:'未测',reason:unavailable.map(s=>s.name+'：'+s.result.reason).join('；'),title:list.map(s=>s.name+'：'+s.result.label+' · '+s.result.reason).join('\n')}
}
export function unlockSummary(catalog:Pick<ServiceRule,'service_id'|'name'>[],lookup:(id:string)=>Attempt|undefined,region?:string,enabled=true,rules:ServiceRegions={}){
 const groups=unlockGroups.map(g=>({...g,...unlockGroupSummary(g.id,catalog,lookup,region,enabled,rules)}))
 const measured=groups.some(g=>g.total)
 return {tone:groups.some(g=>g.reason)?'warn':measured?'good':'empty',label:measured?groups.map(g=>g.name+' '+g.label+(g.total?' 可用':'')).join(' · '):'还没做解锁检测',detail:groups.find(g=>g.reason)?.reason||(measured?'依据当前时间范围内已确认的结果':'无法判断与尚未检测不计入统计')}
}
