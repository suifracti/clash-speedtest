import {describe,expect,it} from 'vitest'
import {hourBuckets,hourConclusion,type PulseRound} from '../historyPulse'
import {airportFaults,type AirportBeat} from '../airportPulse'

const round=(hour:number,stable:number,responses=4,total=4,day=21):PulseRound=>({id:`${day}-${hour}-${stable}-${responses}`,time:new Date(2026,9,day,hour,0).toISOString(),responses,total,stable,oldConfig:false})

describe('时段分布',()=>{
 it('按本地小时分组，不足 2 轮的时段留空',()=>{
  const buckets=hourBuckets([round(21,100),round(21,120),round(21,140,2,4),round(3,300)])
  expect(buckets[21]).toMatchObject({hour:21,rounds:3,median:120})
  expect(buckets[21].failureRate).toBeCloseTo(2/12*100)
  expect(buckets[3]).toMatchObject({rounds:1,median:null,failureRate:null})
  expect(buckets[9]).toMatchObject({rounds:0,median:null,failureRate:null})
 })

 it('晚高峰慢一倍以上才写结论，差距不够就不写',()=>{
  const evening=[0,1,2,3].map(i=>round(21,220+i)),morning=[0,1,2,3].map(i=>round(10,100+i))
  expect(hourConclusion([...evening,...morning])).toBe('晚 20–23 点稳定延迟约为其他时段的 2.2 倍')
  expect(hourConclusion([...evening.map(r=>({...r,stable:100})),...morning])).toBe('')
 })
})

const beat=(at:string,nodes:[string,boolean][]):AirportBeat=>({
 id:at,start:at,end:at,
 nodes:nodes.map(([name,responded])=>({key:name,name,responded})),
 responded:nodes.filter(([,responded])=>responded).length,total:nodes.length,
})

describe('故障事件',()=>{
 it('一轮超过一半节点无响应记一条整轮故障',()=>{
  const faults=airportFaults([beat('2026-10-21T11:00:00.000Z',[['a',false],['b',false],['c',false],['d',true],['e',true]])])
  expect(faults).toHaveLength(1)
  expect(faults[0]).toMatchObject({kind:'round',nodes:3,start:'2026-10-21T11:00:00.000Z'})
  expect(faults[0].detail).toContain('3/5')
 })

 it('单个节点连续 3 轮无响应，恢复时记结束时间',()=>{
  const faults=airportFaults([
   beat('2026-10-21T11:00:00.000Z',[['a',true],['b',true],['c',true]]),
   beat('2026-10-21T12:00:00.000Z',[['a',false],['b',true],['c',true]]),
   beat('2026-10-21T13:00:00.000Z',[['a',false],['b',true],['c',true]]),
   beat('2026-10-21T14:00:00.000Z',[['a',false],['b',true],['c',true]]),
   beat('2026-10-21T15:00:00.000Z',[['a',true],['b',true],['c',true]]),
  ])
  expect(faults).toHaveLength(1)
  expect(faults[0]).toMatchObject({kind:'node',start:'2026-10-21T12:00:00.000Z',end:'2026-10-21T15:00:00.000Z',nodes:1,names:['a']})
  expect(faults[0].detail).toBe('连续 3 轮无响应')
 })

 it('只断 2 轮不算故障',()=>{
  const faults=airportFaults([
   beat('2026-10-21T12:00:00.000Z',[['a',false],['b',true],['c',true]]),
   beat('2026-10-21T13:00:00.000Z',[['a',false],['b',true],['c',true]]),
   beat('2026-10-21T14:00:00.000Z',[['a',true],['b',true],['c',true]]),
  ])
  expect(faults).toHaveLength(0)
 })
})
