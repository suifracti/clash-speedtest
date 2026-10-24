export function clashVerdict(cstSuccess:boolean,clashSuccess:boolean){
 if(cstSuccess&&clashSuccess)return {text:'两边都正常',tone:'good'}
 if(!cstSuccess&&!clashSuccess)return {text:'节点本身不通',tone:'bad'}
 if(clashSuccess)return {text:'可能是 CST 的检测问题',tone:'warn'}
 return {text:'可能是 Clash 侧的配置或分流问题',tone:'muted'}
}
// 两边都有失败时先归到节点本身；只有一边全通、另一边有失败，才指向那一侧。
export function clashSampleVerdict(cst:boolean[],clash:boolean[]){
 const cstAll=cst.every(Boolean),clashAll=clash.every(Boolean)
 if(cstAll||clashAll)return clashVerdict(cstAll,clashAll)
 if([...cst,...clash].every(success=>!success))return clashVerdict(false,false)
 return {text:'节点本身不稳定',tone:'warn'}
}
