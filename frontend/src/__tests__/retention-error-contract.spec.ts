import {afterEach,expect,it,vi} from 'vitest'
import {request} from '../api'
vi.hoisted(()=>{Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})})
afterEach(()=>{vi.unstubAllGlobals();delete window.go})
const details={error:'disk full',partial:true,samples_deleted:1000,runs_deleted:0,policy:'30d',cutoff:'2026-10-01T00:00:00Z'}
it('retains structured partial-cleanup evidence on non-2xx HTTP responses',async()=>{
  vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify(details),{status:500})))
  await expect(request('/api/monitor/retention','POST',{},'ApplyRetention',[{}])).rejects.toMatchObject({message:'disk full',status:500,details})
})
it('decodes the same partial-cleanup evidence from the native bridge',async()=>{
  window.go={desktop:{App:{ApplyRetention:vi.fn(async()=>{throw 'CST_RETENTION_ERROR:'+JSON.stringify(details)})}}}
  await expect(request('/api/monitor/retention','POST',{},'ApplyRetention',[{}])).rejects.toMatchObject({message:'disk full',details})
})
