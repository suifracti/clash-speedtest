import {computed,ref} from 'vue'
import {unlockGroups,unlockSupported} from './unlock'

export const customUnlockGroupsKey='cst.custom-unlock-groups'
export const editableUnlockGroupsKey='cst.unlock-groups.v2'
export const optionalUnlockIds=new Set(['chatgpt_unlock','claude_unlock','spotify_unlock','youtube_premium'])

export interface EditableUnlockGroup {
 id:string
 name:string
 ids:string[]
 custom?:boolean
}

export interface DisplayUnlockGroup {
 id:string
 name:string
 ids:readonly string[]
 regions:readonly string[]
 custom:boolean
}

const canonicalById=new Map<string,(typeof unlockGroups)[number]>(unlockGroups.map(g=>[g.id,g]))
const japanIds=new Set<string>(canonicalById.get('japan')?.ids||[])

export function defaultPresetGroups():EditableUnlockGroup[]{
 return unlockGroups.map(g=>({
  id:g.id,
  name:g.name,
  ids:g.ids.filter(id=>id!=='antigravity'&&!optionalUnlockIds.has(id)&&unlockSupported(id)),
  custom:false,
 }))
}

function readStorage(key:string){
 try{return typeof window!=='undefined'&&window.localStorage?window.localStorage.getItem(key):null}
 catch{return null}
}

let lastSnapshot:string|null|undefined=undefined

function loadUnlockGroups():EditableUnlockGroup[]{
 const raw=readStorage(editableUnlockGroupsKey)
 const legacyRaw=readStorage(customUnlockGroupsKey)
 lastSnapshot=`${raw??''}\u0000${legacyRaw??''}`
 try{
  if(raw){
   const parsed=JSON.parse(raw)
   if(Array.isArray(parsed)){
    return parsed.filter((g):g is EditableUnlockGroup=>!!g&&typeof g.id==='string'&&typeof g.name==='string'&&Array.isArray(g.ids))
   }
  }
  const legacy=legacyRaw?JSON.parse(legacyRaw):[]
  const extras=Array.isArray(legacy)
   ?legacy.filter((g):g is EditableUnlockGroup=>!!g&&typeof g.id==='string'&&typeof g.name==='string'&&Array.isArray(g.ids)).map(g=>({...g,custom:true}))
   :[]
  return [...defaultPresetGroups(),...extras]
 }catch{
  return defaultPresetGroups()
 }
}

export const editableUnlockGroups=ref<EditableUnlockGroup[]>(loadUnlockGroups())

export function syncUnlockGroupsFromStorage(){
 const snap=`${readStorage(editableUnlockGroupsKey)??''}\u0000${readStorage(customUnlockGroupsKey)??''}`
 if(snap!==lastSnapshot){
  editableUnlockGroups.value=loadUnlockGroups()
 }
}

export function persistUnlockGroups(){
 try{
  const serialized=JSON.stringify(editableUnlockGroups.value)
  const customSerialized=JSON.stringify(editableUnlockGroups.value.filter(g=>g.custom))
  window.localStorage.setItem(editableUnlockGroupsKey,serialized)
  window.localStorage.setItem(customUnlockGroupsKey,customSerialized)
  lastSnapshot=`${serialized}\u0000${customSerialized}`
 }catch{}
}

export function resetDefaultUnlockGroups(){
 const customOnly=editableUnlockGroups.value.filter(g=>g.custom)
 editableUnlockGroups.value=[...defaultPresetGroups(),...customOnly]
 persistUnlockGroups()
}

export const displayUnlockGroups=computed<DisplayUnlockGroup[]>(()=>{
 const presets=new Map(defaultPresetGroups().map(g=>[g.id,g]))
 const list=editableUnlockGroups.value.map(g=>{
  const canonical=canonicalById.get(g.id)
  const preset=presets.get(g.id)
  const isUneditedBuiltIn=!!canonical&&!!preset&&!g.custom&&g.name===preset.name&&g.ids.join(',')===preset.ids.join(',')
  if(isUneditedBuiltIn){
   return {id:canonical.id,name:canonical.name,ids:canonical.ids,regions:canonical.regions,custom:false}
  }
  const allJapan=g.ids.length>0&&g.ids.every(id=>japanIds.has(id))
  return {
   id:g.id,
   name:g.name,
   ids:g.ids,
   regions:allJapan?['JP']:[],
   custom:true,
  }
 })
 return list
})
