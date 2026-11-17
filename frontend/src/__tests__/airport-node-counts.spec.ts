import {describe,expect,it} from 'vitest'
import type {Airport,NodeOption} from '../domain'
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
    const nodes=['a1','a2'].flatMap(profile_id=>['Node A','Node B','公告：使用说明'].map(display_name=>({profile_id,node_key:profile_id+display_name,node_identity_key:display_name,config_revision_key:profile_id,display_name,country_code:''} as NodeOption)))
    expect(airportNodeCounts(airports,nodes)).toEqual({a:2})
  })
  it('counts the union of account inventories without merging aliases, distinct endpoints or other airports',()=>{
    const shared={profile_id:'a1',node_key:'first',node_identity_key:'shared',config_revision_key:'a1-rev',display_name:'香港 01',country_code:'HK'} as NodeOption
    const nodes=[shared,{...shared,profile_id:'a2',node_key:'second',config_revision_key:'a2-rev'},{...shared,node_key:'alias',config_revision_key:'alias-rev',display_name:'香港 01 · 别名'},{...shared,profile_id:'a2',node_key:'other-endpoint',node_identity_key:'different'},{...shared,profile_id:'b',node_key:'other-airport'},{...shared,display_name:'公告：使用说明'}]
    expect(airportNodeCounts(airports,nodes)).toEqual({a:3,b:1})
  })
  it('does not assume two same-name records without identity are the same node',()=>{
    const nodes=['a1','a2'].map(profile_id=>({profile_id,node_key:profile_id,node_identity_key:'',config_revision_key:'',display_name:'香港 01',country_code:'HK'} as NodeOption))
    expect(airportNodeCount(airports[0],nodes)).toBe(2)
  })
})
