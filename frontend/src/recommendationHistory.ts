import {readQualityBatch} from './qualityBatch'
import type {QualityEvidence} from './nodeQuality'
import type {NodeOption} from './domain'

const freshnessMs=10*60_000
type Snapshot={scope:string;data:Record<string,QualityEvidence>;until:string;loadedAt:number}
type Cache={last?:Snapshot;pending?:{scope:string;task:Promise<Snapshot>}}
const caches=new WeakMap<object,Cache>()

export function recommendationHistory(workspace:object){
 const cache=caches.get(workspace)||{}
 caches.set(workspace,cache)
 return {
  current:(scope:string)=>cache.last?.scope===scope?cache.last:undefined,
  read(scope:string,nodes:NodeOption[],force:boolean){
   if(cache.pending?.scope===scope)return cache.pending.task
   if(!force&&cache.last?.scope===scope&&Date.now()-cache.last.loadedAt<freshnessMs)return Promise.resolve(cache.last)
   const until=new Date().toISOString()
   const since=new Date(Date.parse(until)-7*86400000).toISOString()
   const task=readQualityBatch(nodes,since,until).then(data=>{
    const snapshot={scope,data,until,loadedAt:Date.now()}
    if(cache.pending?.task===task)cache.last=snapshot
    return snapshot
   }).finally(()=>{if(cache.pending?.task===task)cache.pending=undefined})
   cache.pending={scope,task}
   return task
  },
 }
}
