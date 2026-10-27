import {describe,expect,it} from 'vitest'
import {entryPlace,lineLabel,mainlandNotice,routeKind,routeLabel} from '../entryPath'
import type {NodeEntry} from '../entryPath'

const entry=(city:string,isp:string,country:string):NodeEntry=>({
 profile_id:'p',node_key:'n',node_identity_key:'i',config_revision_key:'r',host:'node.example.com',source:'dns',
 addresses:[{ip:'1.2.3.4',country_code:country,city,isp,country}],
})

describe('线路类型',()=>{
 it('入口和出口同一国家是直连，大陆入口是国内中转，其余是中转',()=>{
  expect(routeLabel(routeKind('SG','SG'))).toBe('直连')
  expect(routeLabel(routeKind('CN','SG'))).toBe('国内中转')
  expect(routeLabel(routeKind('JP','SG'))).toBe('中转')
  expect(routeLabel(routeKind('CN','CN'))).toBe('直连')
  expect(routeLabel(routeKind('','HK',true))).toBe('任播')
  expect(routeKind('','SG')).toBe('')
 })

 it('本机在大陆、入口在境外且与出口同国是跨境直连',()=>{
  expect(routeLabel(routeKind('JP','JP',false,'CN'))).toBe('跨境直连')
  expect(routeLabel(routeKind('HK','HK',false,'CN'))).toBe('跨境直连')
  expect(routeLabel(routeKind('JP','JP',false,'HK'))).toBe('直连')
  expect(routeLabel(routeKind('JP','SG',false,'CN'))).toBe('中转')
  expect(routeLabel(routeKind('CN','SG',false,'CN'))).toBe('国内中转')
  expect(routeLabel(routeKind('','HK',true,'CN'))).toBe('任播')
 })

 it('入口写法与服务商缩写用于线路列',()=>{
  expect(entryPlace(entry('Shanghai','China Unicom','CN'))).toBe('上海 联通')
  expect(lineLabel(entry('Shanghai','China Unicom','CN'),'SG','Amazon.com, Inc.')).toBe('上海 联通 → 新加坡 AWS')
  expect(lineLabel(undefined,'SG')).toBe('→ 新加坡')
 })

 it('本机在境外且入口在大陆才提示绕路',()=>{
  expect(mainlandNotice('CN','TW')).toContain('不是节点故障')
  expect(mainlandNotice('CN','CN')).toBe('')
  expect(mainlandNotice('HK','TW')).toBe('')
  expect(mainlandNotice('CN','')).toBe('')
 })
})
