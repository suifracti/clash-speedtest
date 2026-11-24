const collator=new Intl.Collator('zh-CN',{numeric:true,sensitivity:'base'})

export function ascending(a:number|string|null,b:number|string|null){
 const missing=(value:typeof a)=>value===null||(typeof value==='number'&&!Number.isFinite(value))
 if(missing(a))return missing(b)?0:1
 if(missing(b))return -1
 return typeof a==='number'&&typeof b==='number'?a-b:collator.compare(String(a),String(b))
}
