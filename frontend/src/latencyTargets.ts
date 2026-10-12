export const latencyTargets=[
 {id:'cloudflare',name:'Cloudflare',match:'speed.cloudflare.com',urls:['https://speed.cloudflare.com/__down?bytes=1','https://cp.cloudflare.com/generate_204']},
 {id:'google',name:'Google',match:'gstatic.com',urls:['https://www.gstatic.com/generate_204']},
 {id:'github',name:'GitHub',match:'api.github.com',urls:['https://api.github.com/zen']},
 {id:'apple',name:'Apple',match:'captive.apple.com',urls:['https://captive.apple.com/hotspot-detect.html']},
 {id:'microsoft',name:'Microsoft',match:'msftconnecttest.com',urls:['http://www.msftconnecttest.com/connecttest.txt']},
 {id:'firefox',name:'Firefox',match:'detectportal.firefox.com',urls:['https://detectportal.firefox.com/success.txt']},
]
// Attribution is separate from comparison conditions. Keep the original URL.
export function latencyTargetId(raw?:string){
 if(!raw||raw==='旧版 Cloudflare')return 'cloudflare'
 try{
  const u=new URL(raw)
  if(!['http:','https:'].includes(u.protocol)||u.username||u.password||u.port)return undefined
  return latencyTargets.find(t=>t.urls.some(raw=>{const known=new URL(raw);return u.hostname===known.hostname&&u.pathname===known.pathname}))?.id
 }catch{return undefined}
}
