import {describe,expect,it,vi} from 'vitest'
import {mount} from '@vue/test-utils'
import MultiNodeChart from '../components/MultiNodeChart.vue'
import MeasurementResults from '../components/MeasurementResults.vue'
import HealthBars from '../components/HealthBars.vue'
import type {Attempt} from '../domain'
import type {NodeSeries,TrendPoint} from '../presentation'

describe('global charts and large service rounds',()=>{
  it('compares nodes on one real time axis including failed measurements and supports hiding a curve',async()=>{
    const series:NodeSeries[]=[{id:'a',name:'Node A',airport:'Airport A',points:[{id:'a0',time:'2026-10-01T00:00:00Z',value:80,tone:'good',description:'80 ms'},{id:'a1',time:'2026-10-01T00:20:00Z',value:null,tone:'bad',description:'timeout'}]},{id:'b',name:'Node B',airport:'Airport B',points:[{id:'b0',time:'2026-10-01T00:10:00Z',value:120,tone:'good',description:'120 ms'}]}]
    const wrapper=mount(MultiNodeChart,{props:{series,unit:'ms'}})
    expect(wrapper.findAll('.overview-series')).toHaveLength(2)
    expect(wrapper.find('[aria-label="Node A · Airport A"] circle').attributes('cx')).toBe('44')
    expect(wrapper.find('[aria-label="Node B · Airport B"] circle').attributes('cx')).toBe('495')
    expect(wrapper.findAll('.overview-failure')).toHaveLength(1)
    await wrapper.find('input[aria-label="图表显示 Node B · Airport B"]').setValue(false)
    expect(wrapper.findAll('.overview-series')).toHaveLength(1)
    expect(series[1].points[0].value).toBe(120)
    wrapper.unmount()
  })
  it('uses a preindexed time window instead of parsing every point on pointer movement',async()=>{
    const series:NodeSeries[]=Array.from({length:8},(_,s)=>({id:'series-'+s,name:'Node '+s,airport:'Airport',points:Array.from({length:80},(_,i)=>({id:`${s}-${i}`,time:new Date(Date.UTC(2026,0,1,0,i)).toISOString(),value:20+s+i/10,tone:'good' as const,description:`${s}-${i}`}))}))
    const wrapper=mount(MultiNodeChart,{props:{series,unit:'ms'}}),svg=wrapper.find('svg.overview-svg')
    vi.spyOn(svg.element,'getBoundingClientRect').mockReturnValue({left:0,top:0,width:960,height:300,right:960,bottom:300,x:0,y:0,toJSON:()=>({})})
    const parse=vi.spyOn(Date,'parse')
    await svg.trigger('pointermove',{clientX:480,clientY:120})
    expect(parse.mock.calls.length).toBeLessThan(32)
    expect(wrapper.find('.overview-readout').text()).toContain('·')
    parse.mockRestore();wrapper.unmount()
  })
  it('compresses a large service bar by outcome while keeping all results searchable after pagination',async()=>{
    const attempts:Attempt[]=Array.from({length:20},(_,i)=>({profile_id:'p',node_key:'n',node_identity_key:'identity',config_revision_key:'revision',attempt_id:'service-'+i,request_id:'round:r:item-'+i,display_name:'Node',service_id:'s'+i,requested_at:'2026-10-01T00:00:00Z',execution_state:'completed',persistence_state:'saved',rule:{name:'Service '+i,target_url:'https://example.test'},result:{outcome:i<8?'unlocked':i<14?'region_limited':'transport_error',bytes_read:0,finished_at:'2026-10-01T00:00:01Z',error_message:i>=14?'connection refused':undefined}}))
    const point:TrendPoint={id:'round-r',time:'2026-10-01T00:00:01Z',value:.4,tone:'warn',description:'8/20 项服务通过',attempts,samples:attempts.map((a,i)=>({id:a.attempt_id,time:a.requested_at,value:i<8?1:0,tone:i<8?'good':i<14?'warn':'bad',description:a.rule.name!,label:a.rule.name!}))}
    const bar=mount(HealthBars,{props:{points:[point],label:'服务可用性',aggregate:true}})
    const bands=bar.find('.health-cell.has-record').findAll('.health-segment')
    expect(bands.map(b=>b.attributes('title'))).toEqual(['8 项通过','6 项受限','6 项失败'])
    await bar.find('.health-cell.has-record').trigger('mouseenter')
    expect((bar.emitted('inspect')![0][0] as TrendPoint).attempts).toHaveLength(20)
    const results=mount(MeasurementResults,{props:{point,project:'service'}})
    expect(results.findAll('.measurement-result')).toHaveLength(12)
    expect(results.find('.service-round-counts').text()).toContain('共 20 项')
    await results.find('select[aria-label="筛选本轮服务状态"]').setValue('bad')
    expect(results.findAll('.measurement-result')).toHaveLength(6)
    await results.find('input[aria-label="搜索本轮服务结果"]').setValue('Service 19')
    expect(results.findAll('.measurement-result')).toHaveLength(1)
    expect(results.find('.measurement-result').text()).toContain('Service 19')
    expect(results.find('.service-round-counts').text()).toContain('探针未通过 6')
    results.unmount();bar.unmount()
  })
})

it('breaks download trend lines when source, method or read budget differs',()=>{
 const times=['2026-10-01T00:00:00Z','2026-10-01T01:00:00Z','2026-10-01T02:00:00Z','2026-10-01T03:00:00Z']
 const points:TrendPoint[]=times.map((time,i)=>({id:String(i),time,value:10,tone:'good',description:'download',conditionKey:i<2?'cloudflare/GET/v2/10MiB':'hetzner/GET/v2/10MiB'}))
 const wrapper=mount(MultiNodeChart,{props:{series:[{id:'node',name:'Node',airport:'A',points}],unit:'MiB/s'}})
 const path=wrapper.find('.overview-line').attributes('d')!
 expect(path.match(/M/g)).toHaveLength(2);expect(path.match(/L/g)).toHaveLength(2)
 wrapper.unmount()
})
