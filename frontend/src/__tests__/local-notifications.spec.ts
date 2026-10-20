import {mount,flushPromises} from '@vue/test-utils'
import {defineComponent} from 'vue'
import {request} from '../api'
import {createWorkspace} from '../workspace'
import {useLocalNotifications} from '../localNotifications'
import {expect,it,vi} from 'vitest'
import {localAlerts} from '../localNotifications'
import type {Airport} from '../domain'
vi.mock('../api',async original=>({...await original<typeof import('../api')>(),desktop:()=>true,request:vi.fn(),api:{batch:vi.fn()}}))
it('defaults off and reports observed failure, low quota and expiry',()=>{
 const now=Date.parse('2026-10-20T10:00:00Z'),settings={enabled:false,failure_percent:50,remaining_percent:10,expiry_days:7}
 const airports=[{id:'a',name:'Airport',subscriptions:[{id:'s',name:'Account',usage:{upload:0,download:95,total:100,expire:(now+86400000)/1000}}]}] as Airport[]
 const health=[{airportId:'a',at:new Date(now).toISOString(),failed:2,total:3,source:'monitor'}]
 expect(localAlerts(airports,settings,health,now)).toEqual([])
 const alerts=localAlerts(airports,{...settings,enabled:true},health,now);expect(alerts.map(a=>a.id.split(':')[0])).toEqual(['failure','quota','expiry'])
 airports[0].subscriptions![0].usage={upload:null,download:95};expect(localAlerts(airports,{...settings,enabled:true},[],now)).toEqual([])
})

it('sends a native alert once and exposes delivery errors',async()=>{
 const values=new Map<string,string>();Object.defineProperty(window,'localStorage',{configurable:true,value:{getItem:(key:string)=>values.get(key)||null,setItem:(key:string,value:string)=>values.set(key,value)}})
 const w=createWorkspace();w.settings.value={local_notifications:{enabled:true,failure_percent:50,remaining_percent:10,expiry_days:7}} as any;w.airports.value=[{id:'a',name:'A',subscriptions:[{id:'s',name:'S',usage:{upload:0,download:95,total:100}}]}] as any
 w.batches.value=[{batch_id:'b',request_id:'r',state:'completed_with_issues',requested_at:'2026-10-20T10:00:00Z',item_count:1,items:[{profile_id:'s',result:{total_samples:1,success_samples:0}}]}] as any
 const fail=vi.spyOn(w,'fail').mockImplementation(()=>{});vi.mocked(request).mockResolvedValue(undefined)
 const component=defineComponent({setup(){useLocalNotifications(w);return()=>null}}),wrapper=mount(component);await flushPromises();expect(request).toHaveBeenCalledWith('','POST',undefined,'SendLocalNotification',['quota:s','A · 剩余流量较低','S 剩余 5.0%'])
 w.airports.value=[...w.airports.value];await flushPromises();expect(request).toHaveBeenCalledTimes(2)
 values.clear();wrapper.unmount();vi.mocked(request).mockRejectedValue(new Error('delivery denied'));const again=mount(component);await flushPromises();expect(fail).toHaveBeenCalledWith(new Error('delivery denied'));again.unmount();w.dispose()
})
