// Run from repository root: node docs/design/observatory/check.mjs
import {JSDOM} from '../../../frontend/node_modules/jsdom/lib/api.js';
import fs from 'node:fs';import assert from 'node:assert/strict';
const html=fs.readFileSync('docs/design/cst-observatory-fusion-20261018.html','utf8');
const dom=new JSDOM(html,{runScripts:'outside-only',url:'http://localhost'}),w=dom.window;
w.scrollTo=()=>{};w.HTMLElement.prototype.scrollIntoView=()=>{};
w.matchMedia=()=>({matches:false,addEventListener(){}});w.IntersectionObserver=class{observe(){}disconnect(){}};
w.eval([...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map(m=>m[1]).join('\n')+'\n'+fs.readFileSync('docs/design/observatory/fusion.js','utf8'));
const $=s=>w.document.querySelector(s);let passed=0;function check(name,fn){fn();passed++;console.log('PASS',name)}
check('root is not a theme button',()=>assert.equal(w.document.documentElement.onclick,null));
check('checkbox survives change and repaint',()=>{$('[data-select="a-jp1"]').click();assert.match($('.selection-dock').textContent,/已选 3/)});
check('cycle keeps selected scope',()=>{$('#cycle-source').click();assert.match($('.selection-dock').textContent,/已选 3/)});
check('selected services only',()=>{$('[data-mode="service"]').click();assert.equal(w.document.querySelectorAll('.service-cell').length,16)});
check('service column switch does not change selection',()=>{const b=$('[data-visible-service="0"]');b.click();assert.equal(w.document.querySelectorAll('.service-cell').length,12);assert.match($('.selection-dock').textContent,/已选 3/)});
check('compare has fixed per-metric table',()=>{$('[data-modal="compare"]').click();assert.equal(w.document.querySelectorAll('.fusion-compare tbody tr').length,8)});
check('compare returns to original history',()=>{$('[data-compare-history]').onclick=(()=>{const old=$('[data-compare-history]').onclick;return ()=>{w.HTMLElement.prototype.scrollIntoView=()=>{};old()}})();$('[data-compare-history]').click();assert.match($('.detail-sheet').textContent,/第 5 轮/)});
check('profile stays unknown, no invented risk',()=>{$('[data-ip]').click();assert.match($('.modal-content').textContent,/风险：未查询/);$('.modal [data-close]').click()});
check('all six pages render',()=>{for(const p of ['airports','history','monitor','settings','data','home']){$(`[data-page="${p}"]`).click();assert.ok($('#main-content').textContent.length>100)}});
check('cancel ends active demo stages',()=>{$('#demo-start').click();$('#demo-stop').click();assert.equal(w.document.querySelectorAll('[data-stage="running"],[data-stage="queued"],[data-stage="saving"]').length,0);assert.match($('#demo-state').textContent,/未发送网络/)});
check('no backend network primitives',()=>assert.ok(!/\b(fetch\s*\(|XMLHttpRequest|WebSocket|EventSource)\b/.test(html+fs.readFileSync('docs/design/observatory/fusion.js','utf8'))));
console.log(`${passed} focused preview checks passed`);dom.window.close();
