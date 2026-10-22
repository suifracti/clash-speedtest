import {describe,expect,it} from 'vitest'
import type {Attempt} from '../domain'
import {ipKindOf} from '../ipKind'

const record=(service_id:string,details:Record<string,unknown>,at='2026-10-21T05:49:00.000Z')=>({
 service_id,attempt_id:service_id,display_name:'节点',requested_at:at,finished_at:at,
 execution_state:'completed',persistence_state:'saved',rule:{service_id,target_url:''},
 result:{outcome:'profiled',finished_at:at,details},
} as unknown as Attempt)

describe('IP 类型多数判定',()=>{
 it('机房两票压过非住宅一票，原生只剩未知',()=>{
  const kind=ipKindOf([
   record('ping0_ipv4',{ip_type:'机房 IDC',asn:'AS16509',organization:'Amazon.com, Inc.'}),
   record('ipok_ipv4',{ip_type:'hosting'}),
   record('ippure_ip_quality',{ip_type:'未提供',origin_type:'未提供'}),
  ])
  expect(kind).toMatchObject({type:'idc',origin:'unknown',tag:'机房·未知',tone:'idc',sources:['Ping0','IPOK'],organization:'Amazon.com, Inc.'})
 })

 it('两个来源冲突记未知，原生按注册国与实际使用国判定',()=>{
  const tie=ipKindOf([record('ping0_ipv4',{ip_type:'机房 IDC'}),record('ippure_ip_quality',{ip_type:'住宅（第三方判断）'})])
  expect(tie).toMatchObject({type:'unknown',origin:'unknown',tag:'未知'})
  const home=ipKindOf([record('ippure_ip_quality',{ip_type:'住宅（第三方判断）',origin_type:'原生 IP'})])
  expect(home).toMatchObject({type:'residential',origin:'native',tag:'家宽·原生',tone:'good'})
  const broadcast=ipKindOf([
   record('ippure_ip_quality',{ip_type:'住宅（第三方判断）',origin_type:'原生 IP'}),
   record('ipok_ipv4',{ip_type:'hosting',country_code:'SG',registered_country_code:'US'}),
  ])
  expect(broadcast).toMatchObject({type:'unknown',origin:'unknown',tag:'未知'})
  const shifted=ipKindOf([
   record('ippure_ip_quality',{ip_type:'住宅（第三方判断）',origin_type:'广播 IP'}),
   record('ipok_ipv4',{ip_type:'residential',country_code:'SG',registered_country_code:'US'}),
  ])
  expect(shifted).toMatchObject({type:'residential',origin:'broadcast',tag:'家宽·广播',tone:'warn'})
 })
})
