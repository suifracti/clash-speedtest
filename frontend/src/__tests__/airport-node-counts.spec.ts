import {describe,expect,it} from 'vitest'
import type {Airport} from '../domain'
import {airportNodeCount,airportNodeCounts} from '../airportViewing'

const airports:Airport[]=[
  {id:'a',name:'A',node_count:4,has_cache:true,url_display:'',subscriptions:[
    {id:'a1',airport_id:'a',name:'primary',url_display:'',url_configured:true,node_count:4,has_cache:true},
    {id:'a2',airport_id:'a',name:'mirror',url_display:'',url_configured:true,node_count:4,has_cache:true},
  ]},
  {id:'b',name:'B',node_count:3,has_cache:true,url_display:''},
]

describe('airport sidebar node counts',()=>{
  it('shows one airport inventory when several subscriptions contain the same nodes',()=>{
    expect(airportNodeCounts(airports)).toEqual({a:4,b:3})
    expect(airportNodeCount(airports[0])).toBe(4)
  })
})
