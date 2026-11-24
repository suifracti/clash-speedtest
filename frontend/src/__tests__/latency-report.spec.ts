import {describe,it,expect} from 'vitest'
import {mount} from '@vue/test-utils'
import LatencyReport from '../components/LatencyReport.vue'
import {manualLatencyPoint,latencyPointLabel} from '../presentation'
import {latencyVerdict} from '../components/home/homeModel'
import {activeLatencyTargets} from '../latencyTargets'
import {latencyHttpNotices,buildLatencyReport,buildLatencyRunDiff,compareLatencyReports,latencyAirportSummary} from '../latencyReport'
import {buildLatencyReportCSV,buildLatencyReportHTML} from '../latencyReportExport'
import type {LatencyBatch,BatchItem} from '../domain'
const item=(id:string,ms:number,success=true):BatchItem=>({item_id:id,batch_id:'b',profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',display_name:id,execution_state:'completed',persistence_state:'saved',result:{profile_id:'p',node_key:id,node_identity_key:id,config_revision_key:'r',attempt_id:id,display_name:id,node_type:'Vless',requested_at:'2026-10-21T11:00:00Z',finished_at:'2026-10-21T11:01:00Z',status:'completed',latency_ms:ms,total_samples:24,success_samples:success?24:0,failure_samples:success?0:24,persistence_state:'saved',samples:['https://speed.cloudflare.com/__down?bytes=1','https://www.gstatic.com/generate_204','https://api.github.com/zen','https://captive.apple.com/hotspot-detect.html'].flatMap(target=>Array.from({length:6},(_,i)=>({seq:i+1,target,timestamp:'2026-10-21T11:00:00Z',latency_ms:i?ms:9000,success,error:success?'':'context deadline exceeded'})))}})
const batch=(items:BatchItem[]):LatencyBatch=>({batch_id:'b',request_id:'b',requested_at:'2026-10-21T11:00:00Z',state:'completed',target_id:'all',timeout_seconds:5,sample_count:6,item_count:items.length,items})
describe('latency run report',()=>{
 it('uses the report slow verdict for a responsive workbench node',()=>{const node=item('slow-fixture',2000);expect(buildLatencyReport(batch([node])).rows[0].status).toBe('slow');expect(latencyVerdict(activeLatencyTargets.map(t=>manualLatencyPoint(node.result!,t.id)))).toMatchObject({label:'偏慢',tone:'warn'})})
 it('uses the report response threshold for workbench verdicts',()=>{
  for(const [responses,label] of [[11,'不稳定'],[22,'偶有失败']] as const){
   const node=item('fixture',400),test=node.result!;test.samples=test.samples.map((s,i)=>({...s,success:i<responses,error:i<responses?'':'timeout'}));test.success_samples=responses;test.failure_samples=24-responses
   expect(latencyVerdict(activeLatencyTargets.map(t=>manualLatencyPoint(test,t.id))).label).toBe(label)
   expect(buildLatencyReport(batch([node])).rows[0].status).toBe(responses===11?'unstable':'usable')
  }
 })

 it('shows five exclusive counts, airport totals and expandable problem rows',async()=>{
  const rows=Array.from({length:9},(_,i)=>({...item('node-'+i,0,false),profile_id:i<4?'a':'b'}))
  const wrapper=mount(LatencyReport,{props:{batch:batch(rows),airportNames:{a:'机场A',b:'机场B'}}})
  expect(wrapper.findAll('.latency-report-count')).toHaveLength(5)
  expect(wrapper.findAll('.latency-report-count strong').reduce((n,c)=>n+Number(c.text()),0)).toBe(9)
  expect(wrapper.findAll('.airport-summary tbody tr')).toHaveLength(2)
  const ranked=latencyAirportSummary(buildLatencyReport(batch([{...item('good',300),profile_id:'b'},{...item('bad',0,false),profile_id:'a'}]),{a:'机场A',b:'机场B'}));expect(ranked[0].airport).toBe('机场B')
  expect(wrapper.get('.airport-summary').text()).toContain('能用率')
  const group=wrapper.get('.latency-report-group.no-response');expect(group.findAll('li')).toHaveLength(6)
  expect(group.text()).toContain('机场A');await group.findAll('button').find(b=>b.text().includes('再看'))!.trigger('click');expect(group.findAll('li')).toHaveLength(9)
  wrapper.unmount()
 })

 it('names refused sites while keeping them connected',()=>{const refused=item('refused',300);refused.result!.samples[12]={...refused.result!.samples[12],error:'HTTP 403；响应时延已测，业务／地区解锁未确认'};const r=buildLatencyReport(batch([refused]));expect(r.counts.usable).toBe(1);expect(latencyHttpNotices(r)).toEqual(['GitHub 拒绝访问 1 次，涉及 1 个节点（已连通，可能是限流或风控）'])})
 it('states why a failed site failed',()=>{const failed=item('failed',0,false);expect(latencyPointLabel(manualLatencyPoint(failed.result!,'cloudflare'),'cloudflare')).toBe('超时')})
 it('shows shared cause detail once while keeping different node details',()=>{
  const wrapper=mount(LatencyReport,{props:{batch:batch([item('failed-a',0,false),item('failed-b',0,false),item('slow-a',2000),item('slow-b',3000)])}}),group=wrapper.get('.latency-report-group.no-response')
  expect(group.text().match(/已执行采样，但四站均未取得响应/g)).toHaveLength(1);expect(group.findAll('li small')).toHaveLength(0);const slow=wrapper.get('.latency-report-group.slow');expect(slow.text().match(/阈值 1,500 ms/g)).toHaveLength(1);expect(slow.findAll('li small').map(s=>s.text())).toEqual(['2000 ms','3000 ms']);wrapper.unmount()
 })
 it('uses stable medians, sorts fastest and compares the same nodes',()=>{const b=batch([item('fast',300),item('slow',3000)]),r=buildLatencyReport(b);expect(r.counts).toMatchObject({usable:1,slow:1,suspected:1});expect(r.fastest5[0].stableMs).toBe(300);const old={...batch([item('fast',500),item('slow',3100)]),batch_id:'old',requested_at:'2026-10-20T11:00:00Z'};expect(compareLatencyReports(b,r,[old])).toMatchObject({comparableNodes:2,stableMedianDeltaMs:-150})})
 it('keeps an unexecuted configuration out of no-response counts',()=>{const skipped={...item('skip',0),result:undefined,error_message:'配置冲突'},r=buildLatencyReport(batch([item('failed',0,false),skipped]));expect(r.counts).toMatchObject({noResponse:1,skipped:1});expect(r.groups.find(g=>g.kind==='no-response')?.reason).toBe('连接超时')})
 it('marks low-response nodes unstable and sorts them after usable ones',()=>{const low=item('low',3000);low.result!.samples=low.result!.samples.slice(0,6).map((s,i)=>({...s,success:i<2}));low.result!.total_samples=6;low.result!.success_samples=2;const r=buildLatencyReport(batch([low,item('fast',500),item('failed',0,false)]));expect(r.rows.map(row=>row.status)).toEqual(['usable','unstable','no-response']);expect(r.counts).toMatchObject({slow:0,unstable:1});expect(r.groups.find(g=>g.kind==='unstable')?.reason).toBe('不稳定')})
 it('groups a completely blocked site separately from instability',()=>{
  const node=item('partial',300)
  node.result!.samples=node.result!.samples.map(s=>s.target?.includes('gstatic')?{...s,success:false,error:'EOF'}:s)
  node.result!.success_samples=18
  const r=buildLatencyReport(batch([node]))
  expect(r.rows[0]).toMatchObject({status:'partial',partialSites:['Google']})
  expect(r.counts).toMatchObject({unstable:0,partial:1})
  expect(r.groups.find(g=>g.kind==='partial')?.reason).toBe('部分站点不通')
  expect(latencyVerdict(activeLatencyTargets.map(t=>manualLatencyPoint(node.result!,t.id))).label).toBe('部分站点不通（Google）')
 })

 it('organizes overview vs paginated searchable details and keeps node name separate from airport',async()=>{
  const rows=[
   ...Array.from({length:17},(_,i)=>({...item(`jp-${i}`,250+i,true),profile_id:'a'})),
   {...item('us-bad',0,false),profile_id:'a'}
  ]
  const wrapper=mount(LatencyReport,{props:{batch:batch(rows),airportNames:{a:'良心云'}}})
  expect(wrapper.findAll('.report-tab')).toHaveLength(2)
  expect(wrapper.get('.report-tab[aria-selected="true"]').text()).toContain('结果概览')
  const unstableOrBadBtn=wrapper.findAll('.latency-report-count').find(b=>b.text().includes('无响应'))!
  await unstableOrBadBtn.trigger('click')
  expect(wrapper.get('.report-tab[aria-selected="true"]').text()).toContain('逐个明细')
  expect(wrapper.findAll('.latency-report-table-wrap tbody tr')).toHaveLength(1)
  expect(wrapper.get('.latency-report-table-wrap tbody tr td:first-child button').text()).toBe('美国 · bad')
  expect(wrapper.get('.latency-report-table-wrap tbody tr td:first-child button').text()).not.toContain('良心云')
  await wrapper.findAll('.report-filter-chip').find(b=>b.text().includes('全部'))!.trigger('click')
  expect(wrapper.findAll('.latency-report-table-wrap tbody tr')).toHaveLength(15)
  expect(wrapper.get('.report-pagination').text()).toContain('第 1 / 2 页')
  await wrapper.findAll('.report-pagination button').find(b=>b.text()==='下一页')!.trigger('click')
  expect(wrapper.findAll('.latency-report-table-wrap tbody tr')).toHaveLength(3)
  await wrapper.get('.report-detail-search input').setValue('us-bad')
  expect(wrapper.findAll('.latency-report-table-wrap tbody tr')).toHaveLength(1)
  wrapper.unmount()
 })

 it('drills down from HTTP notices, bar segments, problem groups and opens in-report sample details before optional jump',async()=>{
  const refused=item('jp-refused',260)
  refused.result!.samples[12]={...refused.result!.samples[12],error:'HTTP 403；响应时延已测，业务／地区解锁未确认'}
  const clean=item('sg-clean',180)
  const failed=item('us-fail',0,false)
  const wrapper=mount(LatencyReport,{props:{batch:batch([refused,clean,failed]),airportNames:{p:'机场A'}}})
  await wrapper.get('.latency-report-http').trigger('click')
  expect(wrapper.get('.report-tab[aria-selected="true"]').text()).toContain('逐个明细')
  expect(wrapper.get('.report-subset-pill').text()).toContain('GitHub 拒绝访问')
  expect(wrapper.findAll('.latency-report-table-wrap tbody tr')).toHaveLength(1)
  expect(wrapper.get('.latency-report-table-wrap tbody tr').text()).toContain('GitHub 403 (1/6)')
  expect(wrapper.get('.site-chip-http').text()).toBe('403')
  await wrapper.get('.latency-report-table-wrap tbody tr').trigger('click')
  expect(wrapper.emitted('inspect')).toBeUndefined()
  const detail=wrapper.get('.report-node-detail')
  expect(detail.text()).toContain('GitHub 返回 HTTP 403')
  expect(detail.findAll('.report-site-card')).toHaveLength(4)
  expect(detail.findAll('.report-attempt-list li')).toHaveLength(24)
  await detail.findAll('button').find(b=>b.text().includes('去工作台看历史'))!.trigger('click')
  expect(wrapper.emitted('inspect')?.[0]?.[0]).toMatchObject({node_key:'jp-refused'})
  await detail.get('.report-back-btn').trigger('click')
  expect(wrapper.find('.report-node-detail').exists()).toBe(false)
  await wrapper.findAll('.report-tab')[0]!.trigger('click')
  await wrapper.get('.latency-report-fastest li').trigger('click')
  expect(wrapper.get('.report-node-detail').text()).toContain('新加坡 · clean')
  await wrapper.get('.report-back-btn').trigger('click')
  await wrapper.get('.report-group-drill').trigger('click')
  expect(wrapper.get('.report-tab[aria-selected="true"]').text()).toContain('逐个明细')
  expect(wrapper.findAll('.latency-report-table-wrap tbody tr')).toHaveLength(1)
  wrapper.unmount()
 })

 it('supports interactive run diff tab, region/protocol/show-all detail controls, and CSV/HTML offline report exports',async()=>{
  const prevBatch={...batch([{...item('jp-01',200,true),profile_id:'a'},{...item('hk-01',300,true),profile_id:'a'}]),batch_id:'prev-b',requested_at:'2026-10-20T10:00:00Z'}
  const curA={...item('jp-01',950,true),profile_id:'a'};curA.result!.node_type='Hysteria2'
  const curB={...item('hk-01',0,false),profile_id:'a'};curB.result!.node_type='Vless'
  const curBatch=batch([curA,curB])
  const built=buildLatencyReport(curBatch,{a:'机场A'})
  const diff=buildLatencyRunDiff(curBatch,built,[prevBatch],'auto',{a:'机场A'})
  expect(diff.counts).toMatchObject({all:2,newlyFailed:1,slower:1})
  const csv=buildLatencyReportCSV(built)
  expect(csv.startsWith('\uFEFF')).toBe(true)
  expect(csv).toContain('Cloudflare(ms)')
  expect(csv).toContain('950')
  const html=buildLatencyReportHTML(built,'测试汇总','对比说明',false)
  expect(html).toContain('<!doctype html>')
  expect(html).toContain('全部地区')
  expect(html).toContain('全部协议')
  expect(html).toContain('导出筛选 CSV')

  const wrapper=mount(LatencyReport,{props:{batch:curBatch,previous:[prevBatch],airportNames:{a:'机场A'}}})
  expect(wrapper.findAll('.report-tab')).toHaveLength(3)
  await wrapper.findAll('.report-tab')[2]!.trigger('click')
  expect(wrapper.get('.report-tab[aria-selected="true"]').text()).toContain('轮次对比')
  expect(wrapper.get('.latency-report-diff').text()).toContain('转离线')
  expect(wrapper.get('.latency-report-diff').text()).toContain('↑+750ms')
  await wrapper.findAll('.report-tab')[1]!.trigger('click')
  await wrapper.get('select[aria-label="按协议筛选明细"]').setValue('Hysteria2')
  expect(wrapper.findAll('.latency-report-details .latency-report-table-wrap tbody tr')).toHaveLength(1)
  expect(wrapper.get('.latency-report-details .latency-report-table-wrap tbody tr').text()).toContain('日本 · 01')
  wrapper.unmount()
 })

 it('shows left-column site & region overviews and concrete per-row specifics in problem groups',async()=>{
  const mkPartial=(name:string,ms:number)=>{
   const row=item(name,ms,true)
   row.result!.samples=[
    ...Array.from({length:6},(_,i)=>({seq:i+1,target:'https://speed.cloudflare.com/__down?bytes=1',timestamp:'2026-10-20T12:00:00Z',latency_ms:ms,success:true})),
    ...Array.from({length:6},(_,i)=>({seq:i+7,target:'https://www.gstatic.com/generate_204',timestamp:'2026-10-20T12:00:01Z',latency_ms:0,success:false,error:'context deadline exceeded'})),
   ]
   row.result!.total_samples=12;row.result!.success_samples=6;row.result!.failure_samples=6;row.result!.status='partial_failed'
   return row
  }
  const b=batch([
   {...item('新加坡 01',140,true),profile_id:'a'},
   {...mkPartial('美国 01',250),profile_id:'a'},
   {...mkPartial('美国 02',310),profile_id:'a'},
  ])
  const wrapper=mount(LatencyReport,{props:{batch:b,airportNames:{a:'机场A'}}})
  // Left column site & region summaries render
  expect(wrapper.find('.report-site-overview').text()).toContain('Google')
  expect(wrapper.find('.report-site-overview').text()).toContain('2 不通')
  expect(wrapper.find('.report-region-list').text()).toContain('新加坡')
  expect(wrapper.find('.report-region-list').text()).toContain('美国')
  // Right column problem rows always show concrete per-row response ratio & site/latency details
  const groupItems=wrapper.findAll('.latency-report-group li')
  expect(groupItems).toHaveLength(2)
  expect(groupItems[0]!.text()).toContain('6/12')
  expect(groupItems[0]!.text()).toContain('Google 0/6')
  expect(groupItems[0]!.text()).toContain('250ms')
  expect(groupItems[1]!.text()).toContain('310ms')
  // Clicking a region row drills directly into details filtered by that region
  const usRegionBtn=wrapper.findAll('.report-region-row').find(btn=>btn.text().includes('美国'))!
  await usRegionBtn.trigger('click')
  expect(wrapper.get('.report-tab[aria-selected="true"]').text()).toContain('逐个明细')
  expect(wrapper.findAll('.latency-report-details .latency-report-table-wrap tbody tr')).toHaveLength(2)
  const head=wrapper.findAll('.report-sort-head').find(b=>b.text().includes('稳定中位'))!
  const original=wrapper.findAll('.latency-report-details tbody tr').map(row=>row.text())
  await head.trigger('click')
  expect(head.element.parentElement?.getAttribute('aria-sort')).toBe('ascending')
  await head.trigger('click')
  expect(head.element.parentElement?.getAttribute('aria-sort')).toBe('none')
  expect(wrapper.findAll('.latency-report-details tbody tr').map(row=>row.text())).toEqual(original)
  wrapper.unmount()
 })

})
