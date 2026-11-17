import {mount} from '@vue/test-utils'
import {expect,it} from 'vitest'
import QualityMetrics from '../components/QualityMetrics.vue'
import type {QualityEvidence} from '../nodeQuality'
import {oldService} from './fixtures/quality-evidence'

it('keeps HTTP reachability and unknown outcomes unconfirmed in the comparison',()=>{
 for(const outcome of ['reachable','unrecognized_result']){
  const evidence:QualityEvidence={tests:[],monitor:[],downloads:[],services:[{...oldService,service_id:'legacy_service',rule:{...oldService.rule,name:'历史服务'},result:{...oldService.result!,outcome}}],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:true}},complete:true,since:'2026-10-09T00:00:00Z',until:'2026-10-10T00:00:00Z'}
  const ui=mount(QualityMetrics,{props:{evidence,target:'cloudflare',catalog:[],comparisonField:'service',visibleServiceIds:['legacy_service']}})
  expect(ui.get('[role=button]').attributes('aria-label')).toContain('历史服务 保存证据：无法判断');expect(ui.find('.good').exists()).toBe(false)
  ui.unmount()
 }
})
it('retains explicit cancellation and partial reads beside a service conclusion',()=>{
 const evidence:QualityEvidence={tests:[],monitor:[],downloads:[],services:[{...oldService,execution_state:'cancelled',result:{...oldService.result!,outcome:'cancelled'}}],reads:{latency:{complete:true},monitor:{complete:true},download:{complete:true},service:{complete:false,error:'read failed'}},complete:false,since:'2026-10-09T00:00:00Z',until:'2026-10-10T00:00:00Z'}
 const ui=mount(QualityMetrics,{props:{evidence,target:'cloudflare',catalog:[],comparisonField:'service',visibleServiceIds:['netflix_unlock']}})
 expect(ui.text()).toContain('已取消');expect(ui.text()).toContain('服务读取不完整');expect(ui.text()).toContain('不可比较');expect(ui.find('.good').exists()).toBe(false);ui.unmount()
})
