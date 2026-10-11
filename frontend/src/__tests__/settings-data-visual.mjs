// Synthetic-only visual QA of the actual two components. Start Vite, then run:
// PLAYWRIGHT_CHANNEL=chrome NODE_PATH=<directory containing playwright> node src/__tests__/settings-data-visual.mjs <vite-url> <output-dir>
import {createRequire} from 'node:module'
import {mkdir,writeFile} from 'node:fs/promises'
import path from 'node:path'
const {chromium}=createRequire(import.meta.url)('playwright')
const base=process.argv[2]||'http://localhost:5188',out=path.resolve(process.argv[3]||'visual-evidence/settings-data')
await mkdir(out,{recursive:true})
const browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_CHANNEL?{channel:process.env.PLAYWRIGHT_CHANNEL}:{})})
const reports=[]
const vueUrl=(await (await fetch(base+'/src/main.ts')).text()).match(/from "([^" ]*vue\.js[^" ]*)"/)[1]
const html=`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><div id="app"></div><script type="module">
import {createApp,h,provide} from '${vueUrl}'
import {createWorkspace,workspaceKey} from '/src/workspace.ts'
import SettingsView from '/src/components/SettingsView.vue'
import DataView from '/src/components/DataView.vue'
import '/src/app.css';import '/src/native-ui.css';import '/src/closeout-ui.css'
const long='/synthetic/'+('long-directory-name/'.repeat(12)),setup={state:'ready',initialized:true,data_root:long+'data',profile_dir:long+'profiles',history_dir:long+'history',settings_file:long+'settings.json',sources:[{path:long+'source',label:'合成订阅来源',available:true,profile_count:2,cache_count:1}],migration:{state:'pending',source_history_dir:long+'legacy/history',target_history_dir:long+'history'}}
const settings={preferred_browser:'auto',monitor_retention_policy:'30d',monitor_retention_custom_days:30,monitor_storage_warning_bytes:1073741824,monitor_storage_hard_bytes:2147483648,monitor_budget_max_concurrent:4,monitor_budget_response_bytes:65536}
window.testCalls=[];window.testMode='success'
const bridge={GetSettings:()=>settings,SaveSettings:()=>{if(window.testMode==='error')throw Error('合成保存错误：磁盘暂不可写')},GetTokenStatus:()=>({has_token:false,source:'none',preview:''}),StartOAuthLogin:()=>{},SetToken:()=>{},SetWindowTheme:()=>{},ExportDataBackup:()=>false,OpenDataFolder:()=>{},SelectProfileSourceDirectory:()=>'',InspectProfileSource:p=>({path:p,label:'合成来源',available:true,profile_count:null,cache_count:0}),ImportProfileSource:()=>{},MigrateLegacyData:()=>{if(window.testMode==='error')throw Error('合成迁移错误：请检查目标权限')},InitializeEmptyProfileStore:()=>{},PreviewMonitorRetention:()=>({policy:'30d',cutoff:'2026-09-01T00:00:00Z',samples_to_delete:12,runs_to_delete:2}),ApplyRetention:()=>({samples_deleted:12})}
window.go={desktop:{App:Object.fromEntries(Object.entries(bridge).map(([key,fn])=>[key,async(...args)=>{window.testCalls.push({key,args});return fn(...args)}]))}}
document.documentElement.dataset.theme=new URLSearchParams(location.search).get('theme')||'light'
const w=createWorkspace();w.setup.value=setup;w.token.value=null;w.boot=async()=>{if(window.testCalls.some(call=>call.key==='MigrateLegacyData')&&window.testMode==='success')w.setup.value={...setup,migration:{...setup.migration,state:'ready'}}};window.testWorkspace=w
createApp({setup(){provide(workspaceKey,w);return()=>h('div',{class:'app-shell'},[h('div',{class:'app-content'},[h('header',{class:'workspace-header',style:'padding:14px 32px'},'SpeedTest · W08a 合成数据验收'),h('main',{class:'workspace-body'},[h(location.search.includes('settings')?SettingsView:DataView)])])])}}).mount('#app')
</script></body></html>`
try{
for(const width of [1440,800])for(const theme of ['light','dark'])for(const view of ['settings','data']){
 const page=await browser.newPage({viewport:{width,height:900}}),errors=[]
 page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/**',route=>route.abort())
 await page.route('**/w08a-preview?*',route=>route.fulfill({contentType:'text/html',body:html}))
 await page.addInitScript(theme=>{document.addEventListener('DOMContentLoaded',()=>document.documentElement.dataset.theme=theme)},theme)
 await page.goto(base+'/w08a-preview?'+view+'&theme='+theme);await page.locator('h1').waitFor();await page.waitForFunction(()=>!document.querySelector('section[aria-busy=true]'))
 const layout=await page.evaluate(()=>({width:innerWidth,scrollWidth:document.documentElement.scrollWidth,controls:[...document.querySelectorAll('.page-view input,.page-view button,.page-view .ui-select-trigger')].filter(el=>el.getBoundingClientRect().width>0).map(el=>{const r=el.getBoundingClientRect();return {text:el.textContent?.trim()||el.getAttribute('aria-label')||el.tagName,left:r.left,right:r.right}})}))
 if(layout.scrollWidth>width||layout.controls.some(r=>r.left<0||r.right>width))throw Error('Overflow '+JSON.stringify(layout))
 await page.screenshot({path:path.join(out,view+'-'+width+'-'+theme+'.png'),fullPage:true})
 if(view==='settings'){
  await page.getByRole('button',{name:'手动输入 Token',exact:true}).click();await page.locator('dialog').waitFor();await page.keyboard.press('Tab');if(!await page.evaluate(()=>document.activeElement.closest('dialog')!==null))throw Error('Dialog focus escaped')
  await page.keyboard.press('Escape');await page.locator('dialog').waitFor({state:'detached'});if(!await page.getByRole('button',{name:'手动输入 Token',exact:true}).evaluate(el=>el===document.activeElement))throw Error('Focus was not restored')
  await page.getByRole('button',{name:'预览旧采样清理',exact:true}).click();await page.locator('dialog').waitFor();await page.getByRole('button',{name:'执行清理',exact:true}).click();await page.locator('dialog').waitFor({state:'detached'});if(!await page.getByRole('button',{name:'预览旧采样清理',exact:true}).evaluate(el=>el===document.activeElement))throw Error('Retention success focus was not restored')
  await page.evaluate(()=>window.testMode='error');await page.getByRole('button',{name:'保存设置',exact:true}).click();await page.getByRole('alert').waitFor();await page.screenshot({path:path.join(out,view+'-error-'+width+'-'+theme+'.png'),fullPage:true})
 }else{
  await page.getByRole('button',{name:'导出备份',exact:true}).click();await page.getByRole('status').filter({hasText:'已取消备份导出'}).waitFor();await page.screenshot({path:path.join(out,view+'-cancel-'+width+'-'+theme+'.png'),fullPage:true})
  await page.getByRole('button',{name:'迁移旧版历史与设置',exact:true}).click();await page.locator('dialog').waitFor();await page.keyboard.press('Tab');if(!await page.evaluate(()=>document.activeElement.closest('dialog')!==null))throw Error('Migration focus escaped')
  await page.evaluate(()=>window.testMode='error');await page.getByRole('button',{name:'确认迁移',exact:true}).click();await page.locator('dialog [role=alert]').waitFor();await page.screenshot({path:path.join(out,view+'-dialog-error-'+width+'-'+theme+'.png'),fullPage:true})
  await page.getByRole('button',{name:'取消',exact:true}).click();if(!await page.getByRole('button',{name:'迁移旧版历史与设置',exact:true}).evaluate(el=>el===document.activeElement))throw Error('Migration focus was not restored')
  await page.getByRole('button',{name:'迁移旧版历史与设置',exact:true}).click();await page.evaluate(()=>window.testMode='success');await page.getByRole('button',{name:'确认迁移',exact:true}).click();await page.locator('dialog').waitFor({state:'detached'});if(!await page.getByRole('button',{name:'刷新状态',exact:true}).evaluate(el=>el===document.activeElement))throw Error('Migration success fallback focus was not restored')
 }
 if(errors.length)throw Error(errors.join('; '));reports.push({view,width,theme,horizontalOverflow:false,dialogFocus:'pass',errorOrCancel:'pass',consoleErrors:errors});await page.close()
}
await writeFile(path.join(out,'report.json'),JSON.stringify(reports,null,2));console.log(JSON.stringify(reports,null,2))
}finally{await browser.close()}
