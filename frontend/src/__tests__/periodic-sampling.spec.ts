import {afterEach,describe,expect,it,vi} from 'vitest'
import {flushPromises,mount,type VueWrapper} from '@vue/test-utils'
import PeriodicSamplingPanel from '../components/PeriodicSamplingPanel.vue'
import {api} from '../api'
vi.mock('../api',()=>({api:{periodicSampling:vi.fn(),configurePeriodicSampling:vi.fn(),periodicChecks:vi.fn()}}))
const config={enabled:false,latency_interval_seconds:120,service_interval_seconds:300,download_interval_seconds:3600,download_mib:10,service_concurrency:16,include_antigravity:true}
const status={config,running:false,node_count:258,service_count:82,monitor_job_ids:[],cycles:{}}
let wrapper:VueWrapper|undefined
afterEach(()=>{wrapper?.unmount();vi.clearAllMocks()})
describe('periodic sampling controls',()=>{
 it('saves all-node schedules and the selected byte bound through the service',async()=>{
  vi.mocked(api.periodicSampling).mockResolvedValue(status)
  vi.mocked(api.configurePeriodicSampling).mockResolvedValue({...status,running:true,config:{...config,enabled:true}})
  wrapper=mount(PeriodicSamplingPanel);await flushPromises()
  expect(wrapper.text()).toContain('258');expect(wrapper.text()).toContain('82')
  await wrapper.get('[data-test="start"]').trigger('click');await flushPromises()
  expect(api.configurePeriodicSampling).toHaveBeenCalledWith({...config,enabled:true})
 })
 it('stops using the saved configuration even while the form is being edited',async()=>{
  vi.mocked(api.periodicSampling).mockResolvedValue({...status,running:true,config:{...config,enabled:true}})
  vi.mocked(api.configurePeriodicSampling).mockResolvedValue(status)
  wrapper=mount(PeriodicSamplingPanel);await flushPromises()
  await wrapper.get('[data-test="latency"]').setValue('7')
  await wrapper.get('[data-test="stop"]').trigger('click');await flushPromises()
  expect(api.configurePeriodicSampling).toHaveBeenCalledWith(config)
 })
 it('reports a failed save instead of claiming the service has started',async()=>{
  vi.mocked(api.periodicSampling).mockResolvedValue(status)
  vi.mocked(api.configurePeriodicSampling).mockRejectedValue(new Error('disk full'))
  wrapper=mount(PeriodicSamplingPanel);await flushPromises()
  await wrapper.get('[data-test="start"]').trigger('click');await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('disk full')
  expect(wrapper.text()).toContain('未启动')
 })
})

it('labels all 82 services as continuous patrol and exposes a bounded concurrency setting',async()=>{
 vi.mocked(api.periodicSampling).mockResolvedValue(status)
 vi.mocked(api.configurePeriodicSampling).mockResolvedValue({...status,running:true,config:{...config,enabled:true,service_concurrency:32}})
 wrapper=mount(PeriodicSamplingPanel);await flushPromises()
 expect(wrapper.text()).toContain('持续巡检')
 expect(wrapper.text()).toContain('5 分钟全覆盖目标尚未实现')
 await wrapper.get('[data-test="service-concurrency"]').setValue('32')
 await wrapper.get('[data-test="start"]').trigger('click');await flushPromises()
 expect(api.configurePeriodicSampling).toHaveBeenCalledWith(expect.objectContaining({service_concurrency:32,download_mib:10}))
})

it('separates unexecuted from save failure and exposes honest estimates and last checks',async()=>{
 vi.mocked(api.periodicSampling).mockResolvedValue({...status,cycles:{service:{running:true,total:100,processed:20,saved:18,unsaved:0,not_executed:2,started:20,active:2,waiting:4,elapsed_seconds:60,execution_seconds:50,estimated_total_seconds:330,remaining_seconds:270,bytes_read:0,skipped_slots:0,next_due:'0001-01-01T00:00:00Z',finished_at:'0001-01-01T00:00:00Z',outcomes:{timed_out:3,rate_limited:1}}}})
 wrapper=mount(PeriodicSamplingPanel);await flushPromises()
 expect(wrapper.text()).toContain('未执行 2')
 expect(wrapper.text()).toContain('预计整轮')
 expect(wrapper.text()).toContain('限流 1')
 expect(wrapper.text()).toContain('每项最后检测时间')
})

it('keeps missing last-check data unknown and shows competition beside real results',async()=>{
 vi.mocked(api.periodicSampling).mockResolvedValue(status)
 const n={profile_id:'p',profile_name:'source',node_key:'n',node_identity_key:'i',config_revision_key:'r',display_name:'node',type:'http',country_code:'US',country_flag:''}
 vi.mocked(api.periodicChecks).mockResolvedValue({nodes:[n],node:n,checks:[{service_id:'google_204',name:'Google',last_detected_at:null,execution:'unknown',persistence:'not_applicable',outcome:'unknown'},{service_id:'cloudflare_204',name:'Cloudflare',last_detected_at:'2026-10-06T09:00:00Z',execution:'completed',persistence:'saved',outcome:'matched',competing_probes:'download'}]})
 wrapper=mount(PeriodicSamplingPanel);await flushPromises()
 const detail=wrapper.get('details');detail.element.setAttribute('open','');await detail.trigger('toggle');await flushPromises()
 const rows=wrapper.findAll('tbody tr')
 expect(rows[0].text()).toContain('无实际结果');expect(rows[0].text()).toContain('未知');expect(rows[0].text()).not.toContain('matched')
 expect(rows[1].text()).toContain('matched');expect(rows[1].text()).toContain('竞争：download')
})
