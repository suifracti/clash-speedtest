import {beforeEach,describe,expect,it,vi} from 'vitest'
import {api} from '../api'
import {key,type Attempt,type NodeOption} from '../domain'
import {createWorkspace} from '../workspace'

vi.mock('../api',async importOriginal=>{
  const actual=await importOriginal<typeof import('../api')>()
  return {...actual,api:{...actual.api,attemptHistory:vi.fn(),measurementRounds:vi.fn()}}
})

const node:NodeOption={profile_id:'profile',node_key:'node',node_identity_key:'identity',config_revision_key:'revision',profile_name:'Airport',display_name:'Node',type:'ss',country_code:'SG',country_flag:''}
const attempt=(id:string,time:string):Attempt=>({...node,attempt_id:id,request_id:'request-'+id,service_id:'service-a',display_name:node.display_name,requested_at:time,started_at:time,finished_at:time,execution_state:'failed',persistence_state:'saved',rule:{name:'Service',target_url:'https://service.example'},result:{outcome:'timed_out',bytes_read:0,finished_at:time}})

describe('bounded history paging',()=>{
  beforeEach(()=>vi.resetAllMocks())

  it('retains the first page, follows the stable timestamp and ID cursor, and reports completeness',async()=>{
    const newer=attempt('newer','2026-10-06T11:00:00.000Z'),older=attempt('older','2026-10-06T10:00:00.000Z')
    vi.mocked(api.attemptHistory)
      .mockResolvedValueOnce({attempts:[newer],has_more:true,complete:false})
      .mockResolvedValueOnce({attempts:[older],has_more:false,complete:true})
    const w=createWorkspace()

    await w.ensureHistory([node],['service'])
    expect(w.services.value[key(node)]?.map(a=>a.attempt_id)).toEqual(['newer'])
    expect(w.historyMoreCount([node],'service')).toBe(1)

    await w.loadMoreHistory([node],['service'])
    expect(w.services.value[key(node)]?.map(a=>a.attempt_id)).toEqual(['newer','older'])
    expect(w.historyMoreCount([node],'service')).toBe(0)
    expect(api.attemptHistory).toHaveBeenCalledTimes(2)
    const first=vi.mocked(api.attemptHistory).mock.calls[0]?.[2]||{}
    const second=vi.mocked(api.attemptHistory).mock.calls[1][2]
    expect(second).toMatchObject({before_at:newer.finished_at,before_attempt_id:newer.attempt_id,limit:100,since:first.since,until:first.until})
    w.dispose()
  })

  it('deduplicates round reads, caches a fresh result, and refreshes after the short cache window',async()=>{
    vi.useFakeTimers();vi.setSystemTime(new Date('2026-10-06T12:00:00Z'))
    vi.mocked(api.measurementRounds).mockResolvedValue([])
    const w=createWorkspace()
    await Promise.all([w.loadMeasurementRounds([node]),w.loadMeasurementRounds([node])])
    expect(api.measurementRounds).toHaveBeenCalledTimes(1)
    await w.loadMeasurementRounds([node])
    expect(api.measurementRounds).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(5*60*1000+1)
    await w.loadMeasurementRounds([node])
    expect(api.measurementRounds).toHaveBeenCalledTimes(2)
    await w.loadMeasurementRounds([node],true)
    expect(api.measurementRounds).toHaveBeenCalledTimes(3)
    w.dispose();vi.useRealTimers()
  })
})
