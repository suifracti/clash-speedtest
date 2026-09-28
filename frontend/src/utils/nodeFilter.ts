import type { MonitorNodeOption } from '../types'

const OVERRIDE_STORAGE_KEY = 'clash_speedtest_node_notice_overrides'

export interface ExtractedNoticeLink {
  kind: 'telegram' | 'github' | 'backup' | 'website' | 'general'
  label: string
  url: string
}

/**
 * Heuristics to determine whether a node entry in a subscription is actually a
 * non-proxy announcement, notice, website link, Telegram group, or anti-lost page.
 */
export function isNoticeNode(node: Pick<MonitorNodeOption, 'displayName' | 'countryCode' | 'type'>, customOverrides?: Record<string, boolean>, key?: string): boolean {
  if (key && customOverrides && typeof customOverrides[key] === 'boolean') {
    return customOverrides[key]
  }

  const rawName = (node.displayName || '').trim()
  const lower = rawName.toLowerCase()
  if (!rawName) return false

  // 1. High-confidence explicit URLs & platforms
  if (/https?:\/\/|t\.me\/|tg:\/\/|github\.com|feishu\.cn|discord\.(gg|com)/i.test(rawName)) {
    return true
  }

  // 2. High-confidence community & link keywords
  if (/电报群|tg群|交流群|官方群|粉丝群|讨论群|通知群|频道|客服|工单/.test(rawName)) {
    return true
  }
  if (/防失联|失联页|回家页|发布页|最新地址|备用地址|备用域名|官网|网址|网站/.test(rawName)) {
    return true
  }

  // 3. High-confidence account, traffic, or rule notices
  // Subscription compatibility instructions may carry a country flag and a
  // valid proxy protocol; classify the message, not the protocol or multiplier.
  if (/如果只显示[此这]节点|客户端(?:太旧|过旧|版本过低)|更新一下客户端|订阅地址(?:已)?失效|请重新复制(?:订阅)?(?:链接)?导入/.test(rawName)) {
    return true
  }
  if (/到期时间|剩余流量|重置时间|已用流量|账户余额|距离到期|套餐到期|会员到期|续费/.test(rawName)) {
    return true
  }
  if (/每次使用前|更新订阅|更新到最新|请更新|换个客户|客户端下载|订阅转换|最新配置/.test(rawName)) {
    return true
  }
  if (/^(公告|通知|提示|说明|注意|规则|指南|教程|免责声明)[:：\s]/.test(rawName) || /^建议[:：].*(请|切换|使用|更新)/.test(rawName)) {
    return true
  }
  if (/禁止BT|禁止P2P|严禁|测速无效|不计流量|只看不测|非节点/.test(rawName)) {
    return true
  }

  // 4. Common English keywords in airport dummy entries
  if (/^(notice|announcement|info|expire|traffic|reset|website|telegram|support)[:：\s-]/i.test(rawName)) {
    return true
  }

  // 5. If country is OTHER or empty, check for domain extensions or suspicious airport words
  const isOther = !node.countryCode || node.countryCode === 'OTHER'
  if (isOther) {
    if (/\.(com|net|org|io|xyz|top|cc|me|link|site|app|pages\.dev|workers\.dev)/i.test(lower)) {
      return true
    }
    if (/群|提示|说明|教程|流量|到期|时间|官网|规则|客服|更新|订阅/.test(rawName)) {
      return true
    }
  }

  return false
}

/**
 * Extracts links (Telegram, GitHub anti-lost page, website, etc.) from a notice node name.
 */
export function extractNoticeLinks(text: string): ExtractedNoticeLink[] {
  const links: ExtractedNoticeLink[] = []
  if (!text) return links

  // Match full URLs
  const urlRegex = /(https?:\/\/[^\s，,）)\]》>]+)/gi
  let match: RegExpExecArray | null

  while ((match = urlRegex.exec(text)) !== null) {
    const rawUrl = match[1].replace(/[.,;:!?)]+$/, '')
    let kind: ExtractedNoticeLink['kind'] = 'general'
    let label = '打开链接'

    if (/t\.me\//i.test(rawUrl)) {
      kind = 'telegram'
      label = '打开电报群/频道'
    } else if (/github\.com/i.test(rawUrl)) {
      kind = 'github'
      label = '打开 GitHub / 防失联页'
    } else if (/防失联|失联|发布页|回家/i.test(text)) {
      kind = 'backup'
      label = '打开防失联页'
    } else if (/官网|网址|网站/i.test(text)) {
      kind = 'website'
      label = '打开官网'
    }

    links.push({ kind, label, url: rawUrl })
  }

  // Match t.me/xxx without https://
  if (!links.some((l) => l.kind === 'telegram')) {
    const tgMatch = /(?:^|[^\w/])(t\.me\/[a-zA-Z0-9_+]+)/i.exec(text)
    if (tgMatch) {
      links.push({
        kind: 'telegram',
        label: '打开电报群/频道',
        url: `https://${tgMatch[1]}`,
      })
    }
  }

  // Match github.com/xxx without https://
  if (!links.some((l) => l.kind === 'github')) {
    const ghMatch = /(?:^|[^\w/])(github\.com\/[a-zA-Z0-9_\-\/]+)/i.exec(text)
    if (ghMatch) {
      links.push({
        kind: 'github',
        label: '打开 GitHub / 防失联页',
        url: `https://${ghMatch[1]}`,
      })
    }
  }

  return links
}

/**
 * Reads user custom overrides from localStorage.
 */
export function loadNoticeOverrides(): Record<string, boolean> {
  try {
    const raw = localStorage.getItem(OVERRIDE_STORAGE_KEY)
    if (raw) return JSON.parse(raw)
  } catch {
    // ignore
  }
  return {}
}

/**
 * Saves user custom overrides to localStorage.
 */
export function saveNoticeOverrides(overrides: Record<string, boolean>): void {
  try {
    localStorage.setItem(OVERRIDE_STORAGE_KEY, JSON.stringify(overrides))
  } catch {
    // ignore
  }
}
