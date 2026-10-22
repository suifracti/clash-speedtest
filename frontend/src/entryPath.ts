import {nodeRegionLabel} from './nodePresentation'

// Entry chain: where a node's server address sits (resolved through the
// physical interface, never through the node), where the exit sits, and what
// kind of line that combination is. Presentation only.
export interface EntryAddress {ip:string;country_code?:string;country?:string;city?:string;isp?:string;asn?:string;queried_at?:string;error?:string}
export interface NodeEntry {profile_id:string;node_key:string;node_identity_key:string;config_revision_key:string;host:string;source:string;addresses?:EntryAddress[];resolved_at?:string;error?:string}
export interface LocalPosition {country_code?:string;country?:string;isp?:string;ip_masked?:string;queried_at?:string;error?:string}
export interface EntryResult {local?:LocalPosition;nodes:NodeEntry[]}
export type RouteKind='direct'|'transit'|'domestic'|'anycast'

const cityNames:Record<string,string>={
 Shanghai:'上海',Beijing:'北京',Guangzhou:'广州',Shenzhen:'深圳',Hangzhou:'杭州',Chengdu:'成都',Chongqing:'重庆',Nanjing:'南京',Wuhan:'武汉',"Xi'an":'西安',Xian:'西安',Tianjin:'天津',Qingdao:'青岛',Suzhou:'苏州',Xiamen:'厦门',Changsha:'长沙',Zhengzhou:'郑州',Jinan:'济南',Hefei:'合肥',Fuzhou:'福州',Nanning:'南宁',Liuzhou:'柳州',Guilin:'桂林',Kunming:'昆明',Dalian:'大连',Shenyang:'沈阳',Harbin:'哈尔滨',Shijiazhuang:'石家庄',Taiyuan:'太原',Changchun:'长春',Nanchang:'南昌',Guiyang:'贵阳',Lanzhou:'兰州',Haikou:'海口',Urumqi:'乌鲁木齐',
 'Hong Kong':'香港',Kowloon:'九龙',Taipei:'台北','New Taipei':'新北',Taichung:'台中',Kaohsiung:'高雄',Hsinchu:'新竹',
 Tokyo:'东京',Osaka:'大阪',Yokohama:'横滨',Nagoya:'名古屋',Fukuoka:'福冈',Sapporo:'札幌',Kobe:'神户',
 Seoul:'首尔',Busan:'釜山',Singapore:'新加坡',Bangkok:'曼谷','Kuala Lumpur':'吉隆坡',Jakarta:'雅加达',Manila:'马尼拉',Hanoi:'河内','Ho Chi Minh City':'胡志明市',
 Mumbai:'孟买',Dubai:'迪拜',London:'伦敦',Frankfurt:'法兰克福',Amsterdam:'阿姆斯特丹',Paris:'巴黎',Moscow:'莫斯科',
 'Los Angeles':'洛杉矶','San Jose':'圣何塞','San Francisco':'旧金山',Seattle:'西雅图','New York':'纽约',Ashburn:'阿什本',Dallas:'达拉斯',Chicago:'芝加哥',Miami:'迈阿密',Toronto:'多伦多',Vancouver:'温哥华',Sydney:'悉尼',Melbourne:'墨尔本',Auckland:'奥克兰',
}
const ispNames:Record<string,string>={
 'china unicom':'联通','china telecom':'电信','chinanet':'电信','china mobile':'移动','china education and research network':'教育网','cernet':'教育网',
 alibaba:'阿里云',aliyun:'阿里云',tencent:'腾讯云',huawei:'华为云',baidu:'百度云',ucloud:'UCloud',
 amazon:'AWS','amazon.com':'AWS','amazon technologies':'AWS','amazon web services':'AWS',aws:'AWS',google:'Google','google llc':'Google',microsoft:'Azure',oracle:'Oracle',cloudflare:'Cloudflare',digitalocean:'DigitalOcean',vultr:'Vultr','the constant company':'Vultr',linode:'Linode',akamai:'Akamai','choopa':'Vultr','it7 networks':'搬瓦工',bandwagonhost:'搬瓦工',
 'chunghwa telecom':'中华电信','taiwan mobile':'台湾大哥大','far east tone':'远传电信','fareastone':'远传电信',hkbn:'HKBN','hkt limited':'HKT',pccw:'HKT',hutchison:'3HK',
 ntt:'NTT',kddi:'KDDI','softbank':'SoftBank',iij:'IIJ','singtel':'Singtel','starhub':'StarHub','m1 limited':'M1',
}
function tidy(value?:string){return (value||'').trim().replace(/[,.]+$/,'')}
export function shortISP(isp?:string){
 const raw=tidy(isp);if(!raw)return ''
 const lower=raw.toLowerCase()
 for(const [needle,name] of Object.entries(ispNames))if(lower.includes(needle))return name
 return raw.replace(/\b(network|networks|communications?|telecom(munications)?|technology|technologies|company|limited|ltd|inc|llc|corp(oration)?|gmbh|co\.?)\b\.?/gi,'').replace(/\s{2,}/g,' ').trim()||raw
}
export function shortOrg(org?:string){return shortISP(org)}
export function cityLabel(city?:string){const raw=tidy(city);if(!raw)return '';return cityNames[raw]||cityNames[raw.replace(/\s+(city|shi)$/i,'')]||raw}
export function countryLabel(code?:string){const value=(code||'').toUpperCase();return value?nodeRegionLabel(value):''}
// Cloudflare's anycast address answers from whatever edge is nearest, so the IP
// library's country (usually Canada) describes the registration, not the entry.
export function isAnycast(entry?:NodeEntry){
 const primary=entry?.addresses?.[0]
 if(!primary)return false
 return (primary.asn||'').toUpperCase()==='AS13335'||/cloudflare/i.test(primary.isp||'')
}
export function entryCountry(entry?:NodeEntry){return isAnycast(entry)?'':(entry?.addresses?.[0]?.country_code||'').toUpperCase()}
export function entryPlace(entry?:NodeEntry){
 const primary=entry?.addresses?.[0]
 if(!entry||entry.error||!primary)return ''
 if(isAnycast(entry))return 'Cloudflare 任播'
 const city=cityLabel(primary.city)||countryLabel(primary.country_code)
 const isp=shortISP(primary.isp)
 return [city,isp].filter(Boolean).join(' ')
}
export function exitPlace(countryCode?:string,org?:string){
 const where=countryLabel(countryCode)||(countryCode||'').toUpperCase()
 const isp=shortOrg(org)
 return [where,isp].filter(Boolean).join(' ')
}
// The one rule set: same country is direct; a mainland entry is a domestic
// relay; everything else is a relay.
export function routeKind(entry?:string,exit?:string,anycast=false):RouteKind|''{
 if(anycast)return 'anycast'
 const from=(entry||'').toUpperCase(),to=(exit||'').toUpperCase()
 if(!from||!to)return ''
 if(from===to)return 'direct'
 if(from==='CN')return 'domestic'
 return 'transit'
}
export function routeLabel(kind:RouteKind|''){return kind==='direct'?'直连':kind==='transit'?'中转':kind==='domestic'?'国内中转':kind==='anycast'?'任播':''}
export function routeTone(kind:RouteKind|''){return kind==='direct'?'good':kind==='domestic'?'warn':'empty'}
export function lineLabel(entry?:NodeEntry,exitCountry?:string,exitOrg?:string){
 const from=entryPlace(entry)||'入口未解析'
 const to=exitPlace(exitCountry,exitOrg)||'出口未测'
 return `${from} → ${to}`
}
export function pathSteps(local:LocalPosition|undefined,entry:NodeEntry|undefined,exitCountry?:string,exitOrg?:string){
 const localValue=[countryLabel(local?.country_code),shortISP(local?.isp)].filter(Boolean).join('·')||'本机未取得'
 const entryValue=entryPlace(entry).replace(' ','·')||'入口未解析'
 const exitValue=exitPlace(exitCountry,exitOrg).replace(' ','·')||'出口未测'
 return [{id:'local',label:'本机',value:localValue},{id:'entry',label:'入口',value:entryValue},{id:'exit',label:'出口',value:exitValue}]
}
export const mainlandNoticeText='入口在中国大陆，你从境外连接会绕路，延迟偏高属于线路特性，不是节点故障'
export function mainlandNotice(entryCountryCode?:string,localCountryCode?:string){
 const entry=(entryCountryCode||'').toUpperCase(),local=(localCountryCode||'').toUpperCase()
 if(entry!=='CN'||!local||local==='CN')return ''
 return mainlandNoticeText
}
