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
  it('uses the shared surface and foreground tokens for the compact status strip',()=>{
    expect(declaration('.subscription-overview','background')).toBe('var(--surface)')
    expect(declaration('.subscription-overview','color')).toBe('var(--ink)')
    expect(declaration('.subscription-account>small','color')).toBe('var(--muted)')
    expect(declaration('.usage-track','background')).toBe('var(--line)')
    expect(declaration('.usage-track i','background')).toBe('var(--blue)')
    expect(style).not.toContain('--surface-muted')
    expect(style).not.toContain('--accent')
  })

  it('gives the quota readout and progress bar a clearer visual hierarchy',()=>{
    expect(declaration('.subscription-quota-copy>strong','font-size')).toBe('13px')
    expect(declaration('.subscription-quota-copy>strong','font-weight')).toBe('600')
    expect(declaration('.subscription-quota-copy>strong','color')).toBe('var(--blue)')
    expect(declaration('.subscription-quota-copy>small','color')).toBe('var(--muted)')
    expect(declaration('.usage-track','height')).toBe('5px')
    expect(declaration('.usage-track','border-radius')).toBe('999px')
    expect(declaration('.subscription-quota','background')).toBe('var(--blue-soft)')
    expect(declaration('.subscription-quota','border-radius')).toBe('8px')
  })
})
