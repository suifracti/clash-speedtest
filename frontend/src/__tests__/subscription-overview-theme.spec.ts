import {readFileSync} from 'node:fs'
import {URL as NodeURL,fileURLToPath} from 'node:url'
import postcss,{type Declaration,type Rule} from 'postcss'
import {describe,expect,it} from 'vitest'

const component=readFileSync(fileURLToPath(new NodeURL('../components/SubscriptionOverview.vue',import.meta.url)),'utf8')
const style=component.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1]
if(!style)throw new Error('SubscriptionOverview.vue scoped styles not found')
const rules=postcss.parse(style)
const homeRules=postcss.parse(readFileSync(fileURLToPath(new NodeURL('../home-design.css',import.meta.url)),'utf8'))

function declaration(selector:string,property:string){
  const rule=rules.nodes.find((node):node is Rule=>node.type==='rule'&&node.selector===selector)
  return rule?.nodes.find((node):node is Declaration=>node.type==='decl'&&node.prop===property)?.value
}
function homeDeclaration(selector:string,property:string){
  const rule=homeRules.nodes.find((node):node is Rule=>node.type==='rule'&&node.selector===selector)
  return rule?.nodes.find((node):node is Declaration=>node.type==='decl'&&node.prop===property)?.value
}

describe('SubscriptionOverview theme tokens',()=>{
  it('uses the shared surface and foreground tokens for the subscription card',()=>{
    expect(declaration('.subscription-overview','background')).toBe('var(--surface)')
    expect(declaration('.subscription-overview','color')).toBe('var(--ink)')
    expect(declaration('.subscription-overview','display')).toBe('grid')
    expect(declaration('.subscription-account>small','color')).toBe('var(--muted)')
    expect(declaration('.usage-track','background')).toBe('var(--blue-soft)')
    expect(declaration('.usage-track i','background')).toBe('var(--blue)')
    expect(homeDeclaration('.hd-orbit','height')).toBe('124px')
    expect(homeDeclaration('.hd-orbit','align-self')).toBe('center')
    expect(style).not.toContain('--surface-muted')
    expect(style).not.toContain('--accent')
  })

  it('makes the remaining amount primary and reserves a labeled daily chart column',()=>{
    expect(declaration('.subscription-main','display')).toBe('grid')
    expect(declaration('.subscription-overview','padding')).toBe('8px 14px')
    expect(declaration('.subscription-overview','min-height')).toBe('120px')
    expect(declaration('.subscription-main','grid-template-columns')).toBe('minmax(205px,1.2fr) minmax(150px,.95fr) minmax(210px,1.35fr) minmax(116px,.75fr)')
    expect(declaration('.subscription-remaining-value','font-size')).toBe('26px')
    expect(declaration('.subscription-remaining-value','color')).toBe('var(--blue)')
    expect(declaration('.subscription-remaining-percent','background')).toBe('var(--blue-soft)')
    expect(declaration('.subscription-daily','border-left')).toBe('1px solid var(--line)')
    expect(declaration('.usage-track','height')).toBe('9px')
    expect(declaration('.usage-track','border-radius')).toBe('999px')
    expect(declaration('.usage-track','background')).toBe('var(--blue-soft)')
    expect(declaration('.subscription-quota','background')).toBeUndefined()
  })
})
