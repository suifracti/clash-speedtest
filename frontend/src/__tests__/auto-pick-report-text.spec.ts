import {describe,expect,it} from 'vitest'
import type {AutoPickReport} from '../autoPick'
import {reportPurposeText,reportServiceText} from '../autoPickReportText'

const rules:AutoPickReport['rules']={min_response_rate:90,max_latency_ms:800,groups:['AI','流媒体'],required_services:{AI:['chatgpt_unlock','claude_unlock'],流媒体:['netflix_unlock']},sample_count:6}

describe('auto pick report text',()=>{
  it('translates saved outcomes and explains the purposes a node missed',()=>{
    // 说明列：完整服务名 + 中文结论，失败项排在通过项前面。
    expect(reportServiceText({chatgpt_unlock:'unlocked',claude_unlock:'verification_required',netflix_unlock:'unlocked'}))
      .toBe('Claude 需人机验证 · ChatGPT 可用 · Netflix 可用')
    // 部分用途未入选。
    expect(reportPurposeText({services:{chatgpt_unlock:'verification_required',claude_unlock:'unlocked',netflix_unlock:'unlocked'},purposes:['日常','流媒体']},rules))
      .toBe('未入 AI：ChatGPT 需人机验证')
    // 所选用途全部通过。
    expect(reportPurposeText({services:{chatgpt_unlock:'unlocked',claude_unlock:'unlocked',netflix_unlock:'unlocked'},purposes:['日常','AI','流媒体']},rules))
      .toBe('所选用途条件均通过')
    // 没通过的服务写清结论，旧报告缺依据只写“未记录”。
    expect(reportPurposeText({services:{chatgpt_unlock:'unlocked',claude_unlock:'unlocked',netflix_unlock:'region_blocked'},purposes:['日常','AI']},rules))
      .toBe('未入流媒体：Netflix 地区受限')
    expect(reportPurposeText({purposes:['日常']},{min_response_rate:90,max_latency_ms:800,groups:['AI'],sample_count:6}))
      .toBe('未入 AI：未记录')
  })
})
