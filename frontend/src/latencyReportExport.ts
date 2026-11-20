import {partialLatencyError} from './inspectorLatency'
import {partialLatencyLabel} from './latencyTargets'
import {nodeDisplayName,nodeDisplayParts} from './nodePresentation'
import type {LatencyReport,LatencyReportRow} from './latencyReport'

function csvCell(value:string|number|null|undefined):string{
 const raw=value===null||value===undefined?'':String(value)
 return /[",\r\n]/.test(raw)?`"${raw.replace(/"/g,'""')}"`:raw
}

function rowStatusText(row:LatencyReportRow):string{
 return row.status==='partial'
  ?partialLatencyLabel(row.partialSites)
  :({usable:'能用',slow:'偏慢',unstable:'不稳定',partial:'部分站点不通','no-response':'无响应',skipped:'未执行'} as const)[row.status]
}

function rowNoteText(row:LatencyReportRow):string{
 const parts:string[]=[]
 for(const n of row.httpNotes)parts.push(`${n.siteName} HTTP ${n.code} (${n.count}/${n.total})`)
 for(const s of row.sites){
  if(s.failCount>0)parts.push(`${s.name} ${s.failCount}次${s.failReasons[0]||'失败'}`)
 }
 const extra=partialLatencyError(row.item.result)||row.suspectedReasons.join('；')||row.skipReason||''
 if(extra&&!parts.length)parts.push(extra)
 return parts.join('；')
}

export function buildLatencyReportCSV(report:LatencyReport):string{
 const headers=['节点','地区','机场','协议','状态','首次中位(ms)','稳定中位(ms)','Cloudflare(ms)','Google(ms)','GitHub(ms)','Apple(ms)','成功采样','总采样','说明']
 const lines=[headers.map(csvCell).join(',')]
 const siteMs=(row:LatencyReportRow,id:string)=>{
  const s=row.sites.find(x=>x.id===id)
  if(!s||!s.attemptedSamples)return ''
  if(s.responseSamples===0)return s.failReasons[0]||'超时'
  return s.stableMs!==null?Math.round(s.stableMs):''
 }
 for(const row of report.rows){
  const region=nodeDisplayParts(row.item).region||'未识别地区'
  lines.push([
   nodeDisplayName(row.item),
   region,
   row.airportName,
   row.nodeType,
   rowStatusText(row),
   row.firstMedianMs!==null?Math.round(row.firstMedianMs):'',
   row.stableMs!==null?Math.round(row.stableMs):'',
   siteMs(row,'cloudflare'),
   siteMs(row,'google'),
   siteMs(row,'github'),
   siteMs(row,'apple'),
   row.responseSamples,
   row.attemptedSamples,
   rowNoteText(row),
  ].map(csvCell).join(','))
 }
 return '\uFEFF'+lines.join('\r\n')
}

function escHTML(text:string):string{
 return text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;')
}

function safeJSON(data:unknown):string{
 return JSON.stringify(data).replace(/</g,'\\u003c').replace(/>/g,'\\u003e').replace(/&/g,'\\u0026')
}

export function buildLatencyReportHTML(report:LatencyReport,summary:string,comparison:string,monitor=false):string{
 const payload={
  title:monitor?'ClashST PRO 持续监测离线报告':'ClashST PRO 延迟检测离线报告',
  summary,
  comparison,
  counts:report.counts,
  rows:report.rows.map(row=>({
   key:row.key,
   name:nodeDisplayName(row.item),
   region:nodeDisplayParts(row.item).region||'未识别地区',
   airport:row.airportName,
   type:row.nodeType,
   status:row.status,
   statusLabel:rowStatusText(row),
   firstMs:row.firstMedianMs!==null?Math.round(row.firstMedianMs):null,
   stableMs:row.stableMs!==null?Math.round(row.stableMs):null,
   ok:row.responseSamples,
   total:row.attemptedSamples,
   note:rowNoteText(row),
   sites:row.sites.map(s=>({
    id:s.id,
    name:s.name,
    shortName:s.name==='Cloudflare'?'CF':s.name,
    firstMs:s.firstMs!==null?Math.round(s.firstMs):null,
    stableMs:s.stableMs!==null?Math.round(s.stableMs):null,
    ok:s.responseSamples,
    total:s.attemptedSamples,
    httpCodes:s.httpCodes,
    failCount:s.failCount,
    failReasons:s.failReasons,
    samples:s.samples.map(sp=>({
     seq:sp.seq,
     ms:sp.latencyMs!==null?Math.round(sp.latencyMs):null,
     ok:sp.success,
     http:sp.httpStatus||'',
     err:sp.error||sp.reason||'',
    })),
   })),
  })),
 }
 const pct=report.counts.total?Math.round(report.counts.usable/report.counts.total*1000)/10:0
 return `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>${escHTML(payload.title)}</title>
<style>
:root{--bg:#f4f7fb;--panel:#fff;--panel2:#f7f9fc;--line:#dfe6f1;--text:#122033;--muted:#5c6c82;--accent:#3b66f5;--good:#0d9468;--goodbg:#e7f8f1;--warn:#c26d0a;--warnbg:#fef5e7;--bad:#d92d4b;--badbg:#fdecef}
@media(prefers-color-scheme:dark){:root:not([data-theme=light]){--bg:#0b1120;--panel:#131c2e;--panel2:#19243a;--line:#26344d;--text:#edf2fb;--muted:#8fa1bc;--accent:#6895ff;--good:#34d399;--goodbg:rgba(16,185,129,.15);--warn:#fbbf24;--warnbg:rgba(245,158,11,.15);--bad:#fb7185;--badbg:rgba(244,63,94,.16)}}
[data-theme=dark]{--bg:#0b1120;--panel:#131c2e;--panel2:#19243a;--line:#26344d;--text:#edf2fb;--muted:#8fa1bc;--accent:#6895ff;--good:#34d399;--goodbg:rgba(16,185,129,.15);--warn:#fbbf24;--warnbg:rgba(245,158,11,.15);--bad:#fb7185;--badbg:rgba(244,63,94,.16)}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
.wrap{max-width:1180px;margin:0 auto;padding:24px 18px 48px;display:grid;gap:16px}
.head{display:flex;justify-content:space-between;align-items:flex-start;gap:12px;flex-wrap:wrap}
.head h1{margin:0;font-size:20px;font-weight:750}.meta{margin:4px 0 0;color:var(--muted);font-size:12.5px}
.actions{display:flex;gap:8px;flex-wrap:wrap}.btn{padding:7px 13px;border-radius:9px;border:1px solid var(--line);background:var(--panel);color:var(--text);font-size:12.5px;font-weight:600;cursor:pointer}.btn:hover{border-color:var(--accent);color:var(--accent)}
.card{background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:16px}
.hero{display:grid;grid-template-columns:200px 1fr;gap:18px;align-items:center}
.kpi-big{font-size:36px;font-weight:800;line-height:1.05;font-variant-numeric:tabular-nums}.kpi-big small{font-size:16px;color:var(--muted);font-weight:500}
.bar{display:flex;height:10px;border-radius:999px;overflow:hidden;background:var(--panel2);gap:2px;margin-bottom:12px}
.seg-good{background:linear-gradient(90deg,#10b981,#34d399)}.seg-warn{background:linear-gradient(90deg,#f59e0b,#fbbf24)}.seg-bad{background:linear-gradient(90deg,#e11d48,#fb7185)}.seg-empty{background:var(--line)}
.chips{display:flex;flex-wrap:wrap;gap:8px}.chip{padding:6px 11px;border-radius:10px;border:1px solid var(--line);background:var(--panel2);font-size:12px;color:var(--muted);cursor:pointer;display:inline-flex;gap:7px;align-items:baseline}
.chip strong{font-size:15px;font-variant-numeric:tabular-nums;color:var(--text)}.chip.good strong{color:var(--good)}.chip.warn strong{color:var(--warn)}.chip.bad strong{color:var(--bad)}
.chip.active{border-color:var(--accent);background:var(--panel);box-shadow:0 0 0 2px rgba(59,102,245,.2)}
.compare{padding:10px 14px;border-radius:11px;border:1px solid var(--line);background:var(--panel2);font-size:12.5px;color:var(--muted)}
.tools{display:flex;gap:8px;flex-wrap:wrap;align-items:center}.tools input,.tools select{padding:7px 10px;border-radius:8px;border:1px solid var(--line);background:var(--panel);color:var(--text);font-size:12.5px}
.table-wrap{overflow:auto;border:1px solid var(--line);border-radius:12px;background:var(--panel)}
table{width:100%;border-collapse:collapse;font-size:12.5px;min-width:740px}th,td{padding:10px 11px;border-bottom:1px solid var(--line);text-align:left;vertical-align:top}
th{background:var(--panel2);color:var(--muted);font-size:11.5px;font-weight:650;position:sticky;top:0;white-space:nowrap}th.sort{cursor:pointer}th.sort:hover{color:var(--accent)}
tbody tr{cursor:pointer}tbody tr:hover{background:var(--panel2)}.num{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}
.pill{display:inline-flex;padding:2px 8px;border-radius:999px;font-size:11px;font-weight:650;white-space:nowrap}
.pill.usable{color:var(--good);background:var(--goodbg)}.pill.slow,.pill.unstable,.pill.partial{color:var(--warn);background:var(--warnbg)}.pill.no-response{color:var(--bad);background:var(--badbg)}.pill.skipped{color:var(--muted);background:var(--panel2)}
.sites{display:flex;flex-wrap:wrap;gap:4px}.site{padding:2px 6px;border-radius:6px;border:1px solid var(--line);background:var(--panel2);font-size:11px;font-variant-numeric:tabular-nums;display:inline-flex;gap:4px}
.site.warn{background:var(--warnbg);color:var(--warn)}.site.bad{background:var(--badbg);color:var(--bad)}
.modal-bg{position:fixed;inset:0;background:rgba(10,16,30,.55);display:none;align-items:center;justify-content:center;padding:16px;z-index:50}.modal-bg.open{display:flex}
.modal{background:var(--panel);border:1px solid var(--line);border-radius:16px;max-width:780px;width:100%;max-height:85vh;overflow:auto;padding:18px;display:grid;gap:14px}
.site-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:10px}
.site-card{border:1px solid var(--line);border-radius:11px;padding:11px;background:var(--panel2);display:grid;gap:7px}
.sample-row{display:grid;grid-template-columns:34px 1fr 62px;gap:8px;align-items:center;font-size:11.5px;font-variant-numeric:tabular-nums}
.sbar{height:7px;border-radius:999px;background:var(--line);overflow:hidden}.sbar i{display:block;height:100%;background:var(--good)}.sbar i.warn{background:var(--warn)}.sbar i.bad{background:var(--bad)}
@media(max-width:720px){.hero{grid-template-columns:1fr}}
</style>
</head>
<body>
<div class="wrap">
 <header class="head">
  <div><h1>${escHTML(payload.title)}</h1><p class="meta">${escHTML(summary)}</p></div>
  <div class="actions">
   <button class="btn" id="btn-theme" type="button">切换深浅主题</button>
   <button class="btn" id="btn-csv" type="button">导出筛选 CSV</button>
  </div>
 </header>
 <section class="card hero">
  <div>
   <div style="font-size:12px;color:var(--muted)">能用节点（${pct}%）</div>
   <div class="kpi-big">${report.counts.usable}<small> / ${report.counts.total}</small></div>
  </div>
  <div>
   <div class="bar" id="dist-bar"></div>
   <div class="chips" id="status-chips"></div>
  </div>
 </section>
 <div class="compare">${escHTML(comparison)}</div>
 <section class="tools">
  <input id="q" type="search" placeholder="搜索节点、机场、协议…" style="min-width:210px">
  <select id="f-region"><option value="all">全部地区</option></select>
  <select id="f-proto"><option value="all">全部协议</option></select>
  <button class="btn" id="btn-reset" type="button">重置筛选</button>
  <span id="row-count" class="meta" style="margin-left:auto"></span>
 </section>
 <div class="table-wrap">
  <table>
   <thead><tr><th class="sort" data-sort="name">节点</th><th>地区 / 协议</th><th>状态</th><th class="num sort" data-sort="firstMs">首次中位</th><th class="num sort" data-sort="stableMs">稳定中位</th><th>各站稳定延迟</th><th>说明</th></tr></thead>
   <tbody id="tbody"></tbody>
  </table>
 </div>
</div>
<div class="modal-bg" id="modal-bg"><div class="modal" id="modal"></div></div>
<script>
const DATA=${safeJSON(payload)};
let state={status:'all',region:'all',proto:'all',q:'',sort:'default',dir:1};
const $=id=>document.getElementById(id);
const esc=s=>String(s??'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
const uniq=a=>[...new Set(a.filter(Boolean))].sort((x,y)=>x.localeCompare(y,'zh-CN'));
for(const r of uniq(DATA.rows.map(x=>x.region))){const o=document.createElement('option');o.value=r;o.textContent=r;$('f-region').appendChild(o)}
for(const p of uniq(DATA.rows.map(x=>x.type))){const o=document.createElement('option');o.value=p;o.textContent=p;$('f-proto').appendChild(o)}
const segs=[['usable','good',DATA.counts.usable],['slow','warn',DATA.counts.slow],['unstable','warn',DATA.counts.unstable],['partial','warn',DATA.counts.partial],['no-response','bad',DATA.counts.noResponse],['skipped','empty',DATA.counts.skipped]].filter(s=>s[2]>0);
$('dist-bar').innerHTML=segs.map(s=>'<span class="seg-'+s[1]+'" style="width:'+(DATA.counts.total?s[2]/DATA.counts.total*100:0)+'%"></span>').join('');
const chipDefs=[['all','','全部',DATA.rows.length],['usable','good','能用',DATA.counts.usable],['slow','warn','偏慢',DATA.counts.slow],['unstable','warn','不稳定',DATA.counts.unstable],['partial','warn','部分不通',DATA.counts.partial],['no-response','bad','无响应',DATA.counts.noResponse],...(DATA.counts.skipped?[['skipped','','未执行',DATA.counts.skipped]]:[])];
function filtered(){
 const q=state.q.trim().toLowerCase();
 let list=DATA.rows.filter(r=>(state.status==='all'||r.status===state.status)&&(state.region==='all'||r.region===state.region)&&(state.proto==='all'||r.type===state.proto)&&(!q||(r.name+' '+r.airport+' '+r.type+' '+r.region+' '+r.statusLabel).toLowerCase().includes(q)));
 if(state.sort!=='default'){
  list=[...list].sort((a,b)=>{
   if(state.sort==='name')return a.name.localeCompare(b.name,'zh-CN')*state.dir;
   const av=a[state.sort],bv=b[state.sort];
   if(av===null&&bv===null)return 0;if(av===null)return 1;if(bv===null)return -1;
   return (av-bv)*state.dir;
  });
 }
 return list;
}
function render(){
 $('status-chips').innerHTML=chipDefs.map(c=>'<button type="button" class="chip '+c[1]+(state.status===c[0]?' active':'')+'" data-st="'+c[0]+'">'+c[2]+'<strong>'+c[3]+'</strong></button>').join('');
 const list=filtered();
 $('row-count').textContent='显示 '+list.length+' / '+DATA.rows.length+' 个节点（点击任意行查看逐次采样）';
 $('tbody').innerHTML=list.map(r=>' <tr data-key="'+esc(r.key)+'"><td><strong>'+esc(r.name)+'</strong><div class="meta">'+esc(r.airport)+'</div></td><td>'+esc(r.region)+' · '+esc(r.type)+'</td><td><span class="pill '+esc(r.status)+'">'+esc(r.statusLabel)+'</span></td><td class="num">'+(r.firstMs!==null?r.firstMs+' ms':'—')+'</td><td class="num">'+(r.stableMs!==null?r.stableMs+' ms':'—')+'</td><td><div class="sites">'+r.sites.map(s=>'<span class="site '+(s.total>0&&s.ok===0?'bad':s.failCount||s.httpCodes.length||(s.stableMs&&s.stableMs>1500)?'warn':'')+'">'+esc(s.shortName)+' <b>'+(s.stableMs!==null?s.stableMs+'ms':s.total>0?'超时':'未测')+'</b></span>').join('')+'</div></td><td class="meta">'+esc(r.note||'—')+'</td></tr>').join('')||'<tr><td colspan="7" style="text-align:center;color:var(--muted);padding:20px">没有符合筛选条件的节点</td></tr>';
}
function openModal(row){
 const peak=Math.max(1,...row.sites.flatMap(s=>s.samples.map(sp=>sp.ms||0)));
 $('modal').innerHTML='<div class="head"><div><h2 style="margin:0;font-size:17px">'+esc(row.name)+'</h2><p class="meta">'+esc(row.airport)+' · '+esc(row.type)+' · 响应 '+row.ok+'/'+row.total+'</p></div><button class="btn" id="m-close" type="button">关闭</button></div><div class="site-grid">'+row.sites.map(s=>'<div class="site-card"><div class="head"><strong>'+esc(s.name)+'</strong><span class="meta">'+s.ok+'/'+s.total+' · 稳定 '+(s.stableMs!==null?s.stableMs+' ms':'—')+'</span></div>'+s.samples.map((sp,i)=>'<div class="sample-row"><span>#'+(i+1)+'</span><span class="sbar"><i class="'+(sp.ok?(sp.http||sp.ms>1500?'warn':''):'bad')+'" style="width:'+(sp.ms!==null?Math.max(8,Math.min(100,sp.ms/peak*100)):100)+'%"></i></span><strong style="text-align:right">'+(sp.ms!==null?sp.ms+' ms':esc(sp.err||'失败'))+'</strong></div>').join('')+'</div>').join('')+'</div>';
 $('modal-bg').classList.add('open');
 $('m-close').onclick=()=>$('modal-bg').classList.remove('open');
}
$('status-chips').onclick=e=>{const b=e.target.closest('button[data-st]');if(!b)return;state.status=b.dataset.st;render()};
$('q').oninput=e=>{state.q=e.target.value;render()};
$('f-region').onchange=e=>{state.region=e.target.value;render()};
$('f-proto').onchange=e=>{state.proto=e.target.value;render()};
$('btn-reset').onclick=()=>{state={status:'all',region:'all',proto:'all',q:'',sort:'default',dir:1};$('q').value='';$('f-region').value='all';$('f-proto').value='all';render()};
document.querySelectorAll('th.sort').forEach(th=>th.onclick=()=>{const k=th.dataset.sort;state.dir=state.sort===k?-state.dir:1;state.sort=k;render()});
$('tbody').onclick=e=>{const tr=e.target.closest('tr[data-key]');if(!tr)return;const row=DATA.rows.find(r=>r.key===tr.dataset.key);if(row)openModal(row)};
$('modal-bg').onclick=e=>{if(e.target===$('modal-bg'))$('modal-bg').classList.remove('open')};
document.addEventListener('keydown',e=>{if(e.key==='Escape')$('modal-bg').classList.remove('open')});
$('btn-theme').onclick=()=>{const cur=document.documentElement.dataset.theme;document.documentElement.dataset.theme=cur==='dark'?'light':'dark'};
$('btn-csv').onclick=()=>{
 const list=filtered(),cell=v=>/[",\r\n]/.test(String(v??''))?'"'+String(v).replace(/"/g,'""')+'"':String(v??'');
 const lines=[['节点','地区','机场','协议','状态','首次中位(ms)','稳定中位(ms)','说明'].join(','),...list.map(r=>[r.name,r.region,r.airport,r.type,r.statusLabel,r.firstMs??'',r.stableMs??'',r.note].map(cell).join(','))];
 const a=document.createElement('a');a.href=URL.createObjectURL(new Blob(['\uFEFF'+lines.join('\r\n')],{type:'text/csv;charset=utf-8'}));a.download='CST-report-filtered.csv';a.click();
};
render();
</script>
</body>
</html>`
}
