import {expect,it} from 'vitest'
import {clashVerdict} from '../clashComparison'

it('判断四种 CST 与 Clash 对照结果',()=>{
 expect([[true,true],[false,false],[false,true],[true,false]].map(([cst,clash])=>clashVerdict(cst,clash))).toEqual([
  {text:'两边都正常',tone:'good'},
  {text:'节点本身不通',tone:'bad'},
  {text:'可能是 CST 的检测问题',tone:'warn'},
  {text:'可能是 Clash 侧的配置或分流问题',tone:'muted'},
 ])
})
