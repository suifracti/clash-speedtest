import {readFileSync} from 'node:fs'
import {URL as NodeURL,fileURLToPath} from 'node:url'
import postcss,{type Declaration,type Rule} from 'postcss'
import {describe,expect,it} from 'vitest'

const css=readFileSync(fileURLToPath(new NodeURL('./home-design.css',import.meta.url)),'utf8')
const rules=postcss.parse(css)

function declaration(selector:string,property:string){
  const rule=rules.nodes.find((node):node is Rule=>node.type==='rule'&&node.selector===selector)
  return rule?.nodes.find((node):node is Declaration=>node.type==='decl'&&node.prop===property)?.value
}

describe('home search styling',()=>{
  it('makes the search hint legible and gives focus a clear outer-field state',()=>{
    expect(declaration('.hd-search','height')).toBe('36px')
    expect(declaration('.hd-search:focus-within','border-color')).toBe('var(--hd-accent)')
    expect(declaration('.hd-search:focus-within','box-shadow')).toContain('color-mix(')
    expect(declaration('.hd .hd-search input','font')).toBe('500 13px/1.2 var(--hd-font)')
    expect(declaration('.hd .hd-search input:focus','outline')).toBe('none')
    expect(declaration('.hd .hd-search input:focus','box-shadow')).toBe('none')
    expect(declaration('.hd .hd-search input::placeholder','color')).toBe('var(--hd-muted)')
    expect(declaration('.hd .hd-search input::placeholder','font')).toBe('400 12px/1.2 var(--hd-font)')
  })
})
