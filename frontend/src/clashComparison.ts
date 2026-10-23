export function clashVerdict(cstSuccess:boolean,clashSuccess:boolean){
 if(cstSuccess&&clashSuccess)return {text:'两边都正常',tone:'good'}
 if(!cstSuccess&&!clashSuccess)return {text:'节点本身不通',tone:'bad'}
 if(clashSuccess)return {text:'可能是 CST 的检测问题',tone:'warn'}
 return {text:'可能是 Clash 侧的配置或分流问题',tone:'muted'}
}
