import {afterEach,expect,it,vi} from 'vitest'
import {api} from '../api'
import type {Attempt,NodeScope} from '../domain'

vi.hoisted(()=>{
  Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:()=>null,setItem:()=>{}}})
})

afterEach(()=>{vi.unstubAllGlobals();delete window.go})

it('identifies service attempts by service_id and all four node scope fields over HTTP and the desktop bridge',async()=>{
  const node:NodeScope={profile_id:'profile-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a'}
  const serviceId='google_204',attemptId='attempt/service A'
  const expectedQuery={profile_id:'profile-a',node_key:'node-a',node_identity_key:'identity-a',config_revision_key:'revision-a',service_id:'google_204'}
  const response:Attempt={...node,service_id:serviceId,attempt_id:attemptId,request_id:'request-a',display_name:'Node A',requested_at:'2026-10-02T12:00:00Z',execution_state:'completed',persistence_state:'saved',rule:{service_id:serviceId,target_url:'https://probe.invalid'},result:{outcome:'matched',bytes_read:0,finished_at:'2026-10-02T12:00:01Z'}}
  const calls:[string,string,()=>Promise<Attempt>][]=[
    ['GET','',()=>api.attempt('service',attemptId,node,serviceId)],
    ['POST','/cancel',()=>api.attemptAction('service',attemptId,node,'cancel',serviceId)],
    ['POST','/retry-save',()=>api.attemptAction('service',attemptId,node,'retry-save',serviceId)],
  ]
  const requests:{url:URL;method:string}[]=[]
  const fetchMock=vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
    const url=new URL(String(input),'https://contract.invalid')
    requests.push({url,method:init?.method||'GET'})
    if(url.searchParams.get('service_id')!==serviceId)return new Response(JSON.stringify({error:'service_id is required'}),{status:400})
    return new Response(JSON.stringify(response),{status:200})
  })
  vi.stubGlobal('fetch',fetchMock)
  delete window.go
  for(const [method,suffix,call] of calls){
    expect(await call()).toEqual(response)
    const request=requests[requests.length-1]
    expect(request.method).toBe(method)
    expect(request.url.pathname).toBe(`/api/workbench/public-service-tests/attempt%2Fservice%20A${suffix}`)
    expect(Object.fromEntries(request.url.searchParams)).toEqual(expectedQuery)
  }
  expect(fetchMock).toHaveBeenCalledTimes(3)

  const get=vi.fn(async()=>response),cancel=vi.fn(async()=>response),retrySave=vi.fn(async()=>response)
  window.go={desktop:{App:{GetWorkbenchPublicServiceAttempt:get,CancelWorkbenchPublicServiceTest:cancel,RetrySaveWorkbenchPublicServiceTest:retrySave}}}
  for(const [index,[,,call]] of calls.entries()){
    expect(await call()).toEqual(response)
    expect([get,cancel,retrySave][index]).toHaveBeenCalledTimes(1)
    expect([get,cancel,retrySave][index]).toHaveBeenCalledWith(attemptId,expectedQuery)
  }
  expect(fetchMock).toHaveBeenCalledTimes(3)
})
