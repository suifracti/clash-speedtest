import {it,expect} from 'vitest'
import {mount} from '@vue/test-utils'
import InspectorLatency from '../components/home/InspectorLatency.vue'
import {manualLatencyPoint} from '../presentation'
import {latencyRounds} from '../latencyHistory'
import {activeLatencyTargets} from '../latencyTargets'
import type {LatencyTest} from '../domain'
it('switches the current round from the history list and returns to latest',async()=>{
 const test=(id:string,ago:number,value:number):LatencyTest=>({profile_id:'p',node_key:'n',node_identity_key:'n',config_revision_key:'r',attempt_id:id,display_name:'Node',node_type:'Vless',requested_at:new Date(Date.now()-ago).toISOString(),finished_at:new Date(Date.now()-ago).toISOString(),status:'completed',latency_ms:value,total_samples:8,success_samples:8,failure_samples:0,persistence_state:'saved',samples:activeLatencyTargets.flatMap(t=>[900,value].map((latency_ms,i)=>({seq:i+1,target:t.urls[0],timestamp:new Date(Date.now()-ago).toISOString(),latency_ms,success:true})))})
 const old=test('old',3600000,200),latest=test('latest',1000,400),series=activeLatencyTargets.map(t=>({...t,context:'',points:[manualLatencyPoint(old,t.id),manualLatencyPoint(latest,t.id)]})),w=mount(InspectorLatency,{props:{series}})
 expect(w.find('.latency-metrics').text()).toContain('400 ms');await w.findAll('.round-row')[1].trigger('click');expect(w.find('.latency-metrics').text()).toContain('200 ms');expect(w.find('.round-banner').text()).toContain('正在看');await w.find('.round-banner button').trigger('click');expect(w.find('.latency-metrics').text()).toContain('400 ms');expect(w.find('.round-banner').exists()).toBe(false)
})
it('marks rounds saved under an older config revision',()=>{
 const at=new Date().toISOString(),test=(id:string):LatencyTest=>({profile_id:'p',node_key:'n',node_identity_key:'n',config_revision_key:'r',attempt_id:id,display_name:'Node',node_type:'Vless',requested_at:at,finished_at:at,status:'completed',latency_ms:100,total_samples:1,success_samples:1,failure_samples:0,persistence_state:'saved',samples:activeLatencyTargets.map(t=>({seq:1,target:t.urls[0],timestamp:at,latency_ms:100,success:true}))})
 const series=activeLatencyTargets.map(t=>({...t,context:'',points:[{...manualLatencyPoint(test('old'),t.id),oldConfig:true},manualLatencyPoint(test('new'),t.id)]}))
 expect(latencyRounds(series).map(r=>[r.id,r.oldConfig])).toEqual([['old',true],['new',false]])
})
