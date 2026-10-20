import {key,type NodeOption} from './domain'
import {readQuality,type QualityEvidence} from './nodeQuality'

export async function readQualityBatch(nodes:NodeOption[],since:string,until:string){
 const data:Record<string,QualityEvidence>={};let next=0
 async function worker(){
  while(next<nodes.length){
   const node=nodes[next++]
   data[key(node)]=await readQuality(node,since,until)
  }
 }
 await Promise.all(Array.from({length:Math.min(6,nodes.length)},()=>worker()))
 return data
}
