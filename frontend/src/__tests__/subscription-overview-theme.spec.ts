import {readFileSync} from 'node:fs'
import {URL as NodeURL,fileURLToPath} from 'node:url'
import postcss,{type Declaration,type Rule} from 'postcss'
import {describe,expect,it} from 'vitest'

const component=readFileSync(fileURLToPath(new NodeURL('../components/SubscriptionOverview.vue',import.meta.url)),'utf8')
const style=component.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1]
if(!style)throw new Error('SubscriptionOverview.vue scoped styles not found')
const rules=postcss.parse(style)

function declaration(selector:string,property:string){
  const rule=rules.nodes.find((node):node is Rule=>node.type==='rule'&&node.selector===selector)
  return rule?.nodes.find((node):node is Declaration=>node.type==='decl'&&node.prop===property)?.value
}

describe('SubscriptionOverview theme tokens',()=>{
  it('uses the shared surface and foreground tokens for account cards and quota status',()=>{
    expect(declaration('.subscription-overview-grid>div','background')).toBe('var(--surface-soft)')
    expect(declaration('.subscription-overview-grid>div','color')).toBe('var(--ink)')
    expect(declaration('.subscription-overview-header p','color')).toBe('var(--ink)')
    expect(declaration('.usage-track','background')).toBe('var(--line)')
    expect(declaration('.usage-track i','background')).toBe('var(--blue)')
    expect(declaration('.subscription-overview-refresh .muted','color')).toBe('var(--ink)')
    expect(declaration('.subscription-overview-refresh .badge','color')).toBe('var(--ink)')
    expect(declaration('.subscription-overview-refresh .inline-error','color')).toBe('var(--ink)')
    expect(style).not.toContain('--surface-muted')
    expect(style).not.toContain('--accent')
  })
})
