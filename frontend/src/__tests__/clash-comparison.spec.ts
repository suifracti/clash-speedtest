import {expect,it} from 'vitest'
import {clashVerdict,clashSampleVerdict} from '../clashComparison'

it('判断四种 CST 与 Clash 对照结果',()=>{
 expect([[true,true],[false,false],[false,true],[true,false]].map(([cst,clash])=>clashVerdict(cst,clash))).toEqual([
  {text:'两边都正常',tone:'good'},
  {text:'节点本身不通',tone:'bad'},
  {text:'可能是 CST 的检测问题',tone:'warn'},
  {text:'可能是 Clash 侧的配置或分流问题',tone:'muted'},
 ])
})

it('两边都有失败时判定节点本身不稳定，只有一边全通才指向那一侧',()=>{
 expect([
  clashSampleVerdict([false,false],[true,false]),
  clashSampleVerdict([true,false],[false,false]),
  clashSampleVerdict([false,false],[true,true]),
  clashSampleVerdict([true,true],[true,false]),
  clashSampleVerdict([false,false],[false,false]),
 ]).toEqual([
  {text:'节点本身不稳定',tone:'warn'},
  {text:'节点本身不稳定',tone:'warn'},
  {text:'可能是 CST 的检测问题',tone:'warn'},
  {text:'可能是 Clash 侧的配置或分流问题',tone:'muted'},
  {text:'节点本身不通',tone:'bad'},
 ])
})
