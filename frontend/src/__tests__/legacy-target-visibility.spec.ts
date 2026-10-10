import {expect,it} from 'vitest'
import {mount} from '@vue/test-utils'
import HealthBars from '../components/HealthBars.vue'
import {monitorLatencyPoints} from '../presentation'
import {targets,type MonitorSample} from '../domain'
import realShape from './fixtures/legacy-cp-rtt.json'

it('renders the real cursor RTT shape in the Cloudflare microtrack with other five sites unmeasured',()=>{
 const points=monitorLatencyPoints(realShape as MonitorSample[],'all')
 const ui=mount(HealthBars,{props:{points,label:'历史',sites:targets}})
 const records=ui.findAll('.health-cell.has-record')
 expect(points).toHaveLength(17)
 expect(records.length).toBeGreaterThan(0)
 for(const record of records){
  const layers=record.findAll('.health-segment')
  expect(layers[0].classes()).toContain('bad')
  expect(layers.slice(1).every(s=>s.classes().includes('empty'))).toBe(true)
 }
 expect(points.every(p=>p.sampleGroups?.[0].target==='https://cp.cloudflare.com/generate_204')).toBe(true)
 ui.unmount()
})

it('keeps an unrecognized RTT target inspectable without attributing it to a known site',async()=>{
 const sample={...realShape[0],sample_id:'unknown',run_id:'unknown',target:'https://cp.cloudflare.com.evil.test/generate_204',success:true,latency:80000000} as MonitorSample
 const points=monitorLatencyPoints([sample],'all'),ui=mount(HealthBars,{props:{points,label:'历史',sites:targets}})
 const record=ui.find('.health-cell.has-record'),segments=record.findAll('.health-segment')
 expect(segments).toHaveLength(7)
 expect(segments.slice(0,6).every(s=>s.classes().includes('empty'))).toBe(true)
 expect(segments[6].attributes('title')).toContain('旧/未知目标')
 await record.trigger('click');expect(ui.emitted('open')?.[0]?.[0]).toEqual(points[0])
 expect(monitorLatencyPoints([sample],'cloudflare')).toEqual([])
 ui.unmount()
})

it('does not treat basic HTTP monitoring as six-site RTT evidence',()=>{
 const sample={...realShape[0],probe_type:'service_google',target:'https://www.google.com/generate_204',success:true} as MonitorSample
 expect(monitorLatencyPoints([sample],'all')).toEqual([])
})
