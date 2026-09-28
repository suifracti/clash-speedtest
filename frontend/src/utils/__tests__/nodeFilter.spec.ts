import { describe, expect, it } from 'vitest'
import { extractNoticeLinks, isNoticeNode } from '../nodeFilter'

describe('nodeFilter', () => {
  it('detects typical airport announcement / non-proxy entries', () => {
    // Exact items from the user's screenshot
    expect(isNoticeNode({ displayName: '电报群https://t.me/feiniaoyunjichang', countryCode: 'OTHER', type: 'Trojan' })).toBe(true)
    expect(isNoticeNode({ displayName: '防失联页https://github.com/feiniaoyun', countryCode: 'OTHER', type: 'Trojan' })).toBe(true)
    expect(isNoticeNode({ displayName: '每次使用前更新订阅到最新，用不了，换个客户...', countryCode: 'OTHER', type: 'Trojan' })).toBe(true)

    // Common airport notice nodes
    expect(isNoticeNode({ displayName: '官网: https://example.com', countryCode: '', type: 'Shadowsocks' })).toBe(true)
    expect(isNoticeNode({ displayName: '到期时间：2026-12-31', countryCode: 'OTHER', type: 'Trojan' })).toBe(true)
    expect(isNoticeNode({ displayName: '剩余流量：500.00 GB', countryCode: 'OTHER', type: 'Trojan' })).toBe(true)
    expect(isNoticeNode({ displayName: '公告: 节点已全部维护完毕', countryCode: 'OTHER', type: 'Vless' })).toBe(true)
    expect(isNoticeNode({ displayName: '【通知】请务必更新订阅', countryCode: 'OTHER', type: 'Trojan' })).toBe(true)
    expect(isNoticeNode({ displayName: '建议：感到卡顿请切换到专线节点', countryCode: 'OTHER', type: 'Trojan' })).toBe(true)
    for (const displayName of ['❗如果只显示此节点|0.1x', '❗是客户端太旧导致|0.1x', '❗更新一下客户端|0.1x', '❗请到网站使用教程|0.1x', '订阅地址失效', '请重新复制导入']) {
      for (const countryCode of ['OTHER', 'CN']) {
        expect(isNoticeNode({ displayName, countryCode, type: 'Vless' })).toBe(true)
      }
    }
  })

  it('preserves legitimate proxy nodes', () => {
    expect(isNoticeNode({ displayName: '🇭🇰 香港 01 [x1.0]', countryCode: 'HK', type: 'Trojan' })).toBe(false)
    expect(isNoticeNode({ displayName: '🇯🇵 日本 Tokyo BGP 02', countryCode: 'JP', type: 'Shadowsocks' })).toBe(false)
    expect(isNoticeNode({ displayName: '🇺🇸 美国 洛杉矶 05', countryCode: 'US', type: 'Vless' })).toBe(false)
    expect(isNoticeNode({ displayName: '🇸🇬 新加坡 01 4K', countryCode: 'SG', type: 'Trojan' })).toBe(false)
    expect(isNoticeNode({ displayName: '🇹🇼 台湾 03 流媒体', countryCode: 'TW', type: 'Trojan' })).toBe(false)
    expect(isNoticeNode({ displayName: '德国 01', countryCode: 'DE', type: 'Trojan' })).toBe(false)
    expect(isNoticeNode({ displayName: '⬇️下载专用|0.1x', countryCode: 'OTHER', type: 'Vless' })).toBe(false)
    expect(isNoticeNode({ displayName: '香港HKT05|v6入口', countryCode: 'HK', type: 'AnyTLS' })).toBe(false)
  })

  it('extracts links from notice texts', () => {
    const tgLinks = extractNoticeLinks('电报群https://t.me/feiniaoyunjichang')
    expect(tgLinks).toHaveLength(1)
    expect(tgLinks[0].kind).toBe('telegram')
    expect(tgLinks[0].url).toBe('https://t.me/feiniaoyunjichang')

    const ghLinks = extractNoticeLinks('防失联页https://github.com/feiniaoyun')
    expect(ghLinks).toHaveLength(1)
    expect(ghLinks[0].kind).toBe('github')
    expect(ghLinks[0].url).toBe('https://github.com/feiniaoyun')

    const webLinks = extractNoticeLinks('官网 https://feiniao.com 请收藏')
    expect(webLinks).toHaveLength(1)
    expect(webLinks[0].url).toBe('https://feiniao.com')
  })

  it('respects manual overrides', () => {
    const node = { displayName: '特别定制专线', countryCode: 'OTHER', type: 'Trojan' }
    // Naturally not a notice
    expect(isNoticeNode(node, { 'custom-key': true }, 'custom-key')).toBe(true)
    // Forced false
    const noticeNode = { displayName: '官网: https://example.com', countryCode: '', type: 'Trojan' }
    expect(isNoticeNode(noticeNode, { 'notice-key': false }, 'notice-key')).toBe(false)
  })
})
