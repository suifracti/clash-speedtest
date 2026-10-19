import {describe,expect,it} from 'vitest'
import {dailySeries,shiftDay,type UsageReport} from '../usageDaily'

const report=(buckets:UsageReport['buckets']):UsageReport=>({from:'2026-09-28',until:'2026-10-02',period:'day',buckets,intervals:[],accounts:[],observed_bytes:null,unallocated_bytes:null,unknown_intervals:0,daily_enabled:true})
describe('daily usage series (synthetic data)',()=>{
  it('crosses month boundaries and labels today, yesterday and the day before',()=>{
    expect(shiftDay('2026-10-01',-1)).toBe('2026-09-30')
    const s=dailySeries(report([{date:'2026-10-01',bytes:1024,upload:0,download:1024,intervals:1,estimated_intervals:0}]),'2026-10-02',4)
    expect(s.map(d=>[d.label,d.date])).toEqual([['今天','2026-10-02'],['昨天','2026-10-01'],['前天','2026-09-30'],['09-29','2026-09-29']])
  })
  it('keeps unknown days unknown instead of zero and separates recorded-but-unallocated days',()=>{
    const s=dailySeries(report([{date:'2026-10-02',bytes:null,upload:null,download:null,intervals:1,estimated_intervals:0},{date:'2026-10-01',bytes:0,upload:0,download:0,intervals:1,estimated_intervals:1}]),'2026-10-02',3)
    expect(s[0]).toMatchObject({bytes:null,recorded:true})
    expect(s[1]).toMatchObject({bytes:0,estimated:true})
    expect(s[2]).toMatchObject({bytes:null,recorded:false})
  })
})
