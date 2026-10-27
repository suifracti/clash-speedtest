export const latencyTargets=[
 {id:'cloudflare',name:'Cloudflare',match:'speed.cloudflare.com',urls:['https://speed.cloudflare.com/__down?bytes=1','https://cp.cloudflare.com/generate_204']},
 {id:'google',name:'Google',match:'gstatic.com',urls:['https://www.gstatic.com/generate_204']},
 {id:'github',name:'GitHub',match:'api.github.com',urls:['https://api.github.com/zen']},
 {id:'apple',name:'Apple',match:'captive.apple.com',urls:['https://captive.apple.com/hotspot-detect.html']},
 {id:'microsoft',name:'Microsoft',match:'msftconnecttest.com',urls:['http://www.msftconnecttest.com/connecttest.txt']},
 {id:'firefox',name:'Firefox',match:'detectportal.firefox.com',urls:['https://detectportal.firefox.com/success.txt']},
]
export const activeLatencyTargets=latencyTargets.filter(t=>!['microsoft','firefox'].includes(t.id))
// Attribution is separate from comparison conditions. Keep the original URL.
export function latencyTargetId(raw?:string){
 if(!raw||raw==='旧版 Cloudflare')return 'cloudflare'
 try{
  const u=new URL(raw)
  if(!['http:','https:'].includes(u.protocol)||u.username||u.password||u.port)return undefined
  return latencyTargets.find(t=>t.urls.some(raw=>{const known=new URL(raw);return u.hostname===known.hostname&&u.pathname===known.pathname}))?.id
 }catch{return undefined}
}

// A completely blocked site is separate from intermittent failures elsewhere.
export function partialLatencySites(sites:{name:string;total:number;success:number}[]):string[]{
 const measured=sites.filter(s=>s.total>0),failed=measured.filter(s=>s.total>=6&&s.success===0)
 return failed.length>0&&failed.length<measured.length&&measured.every(s=>failed.includes(s)||s.success===s.total)?failed.map(s=>s.name):[]
}
export function partialLatencyLabel(names:string[]){return `部分站点不通（${names.join('、')}）`}
