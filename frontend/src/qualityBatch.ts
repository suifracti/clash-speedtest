import {key,type NodeOption} from './domain'
import {readQuality,type QualityEvidence} from './nodeQuality'
import {api} from './api'

// Use the same saved revision choices and exact scopes as NodeDetailModal.
async function readNodeHistory(node:NodeOption,since:string,until:string){
 const revisions=await api.revisions(node),evidence=await readQuality(node,since,until)
 for(const revision of revisions.filter(r=>r.config_revision_key!==node.config_revision_key)){
  const older=await readQuality({...node,node_key:revision.node_key,config_revision_key:revision.config_revision_key},since,until)
  evidence.tests.push(...older.tests);evidence.monitor.push(...older.monitor)
  evidence.downloads.push(...older.downloads);evidence.services.push(...older.services)
  for(const kind of ['latency','monitor','download','service'] as const)if(!older.reads[kind].complete)evidence.reads[kind]=older.reads[kind]
 }
 evidence.complete=Object.values(evidence.reads).every(r=>r.complete)
 return evidence
}

export async function readQualityBatch(nodes:NodeOption[],since:string,until:string){
 const data:Record<string,QualityEvidence>={};let next=0
 async function worker(){
  while(next<nodes.length){
   const node=nodes[next++]
   data[key(node)]=await readNodeHistory(node,since,until)
  }
 }
 await Promise.all(Array.from({length:Math.min(6,nodes.length)},()=>worker()))
 return data
}
