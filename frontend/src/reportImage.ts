import html2canvas from 'html2canvas'
import {desktop,request} from './api'

// Export display labels only; raw endpoints and error strings never enter the image.
export function reportImageText(value:string){return value.replace(/https?:\/\/\S+/gi,'[地址已隐藏]').replace(/\b(?:\d{1,3}\.){3}\d{1,3}\b/g,'[IP 已隐藏]').replace(/\b(?:[\da-f]{0,4}:){2,}[\da-f:]{0,4}\b/gi,'[IP 已隐藏]')}
export async function saveReportImage(element:HTMLElement,filename:string):Promise<boolean>{
 await document.fonts?.ready
 const canvas=await html2canvas(element,{backgroundColor:'#ffffff',scale:2,logging:false,windowWidth:1100})
 const png=canvas.toDataURL('image/png')
 if(desktop())return request<boolean>('','POST',undefined,'SavePNGFile',[png.slice(png.indexOf(',')+1),filename])
 const link=document.createElement('a');link.href=png;link.download=filename;link.click();return true
}
