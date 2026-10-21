import {expect,it} from 'vitest'
import {exitLanding} from '../exitLanding'
import type {Attempt} from '../domain'
let serial=0
const record=(id:string,details:Record<string,string>)=>({attempt_id:String(++serial),service_id:id,execution_state:'completed',persistence_state:'saved',rule:{service_id:id},result:{outcome:'profiled',details,finished_at:new Date(Date.UTC(2026,9,21,0,serial)).toISOString()}} as unknown as Attempt)
const records=[record('ippure_ip_quality',{ip:'1.2.3.4',country_code:'SG',risk_score:'48'}),record('cloudflare_trace',{country_code:'SG'}),record('google_region',{service_region:'SG'})]
it('combines the three region sources and masks the exit IP',()=>{expect(exitLanding({country_code:'SG'},records)).toMatchObject({ip:'1.*.*.4',country:'SG',label:'落地一致',tone:'good'})})
it('highlights a Google mismatch and makes any mainland source red',()=>{
 expect(exitLanding({country_code:'SG'},[...records,record('google_region',{service_region:'HK'})]).label).toContain('Google 判定 HK')
 expect(exitLanding({country_code:'SG'},[record('cloudflare_trace',{country_code:'CN'})])).toMatchObject({tone:'bad',label:'Cloudflare 判定 CN（中国大陆）'})
})
