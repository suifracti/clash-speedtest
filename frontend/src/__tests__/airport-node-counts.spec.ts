import {describe,expect,it} from 'vitest'
import type {Airport,NodeOption} from '../domain'
import {airportNodeCounts} from '../airportViewing'

const airports:Airport[]=[
  {id:'a',name:'A',node_count:4,has_cache:true,url_display:''},
  {id:'b',name:'B',node_count:3,has_cache:true,url_display:''},
]
const node=(profile_id:string,display_name:string):NodeOption=>({
  profile_id,profile_name:profile_id,node_key:display_name,node_identity_key:display_name,config_revision_key:'r',display_name,type:'ss',country_code:'US',country_flag:'🇺🇸',
})

describe('airport sidebar node counts',()=>{
  it('counts all cached proxy nodes independent of selected measurement airports',()=>{
    expect(airportNodeCounts([node('a','US 01'),node('a','剩余流量: 10 GiB'),node('b','SG 01')],airports)).toEqual({a:1,b:1})
  })
})
