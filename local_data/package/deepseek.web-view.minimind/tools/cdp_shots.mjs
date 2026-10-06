#!/usr/bin/env node
// CDP screenshot driver: load, wait for boot, exercise UI, capture screenshots.
import { spawn } from 'node:child_process';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

const BASE = process.argv[2] || 'http://127.0.0.1:8014/index.html';
const PORT = 9344;

const profile = mkdtempSync(path.join(tmpdir(), 'chrome-prof-'));
const chrome = spawn('google-chrome', [
  '--headless=new', '--disable-gpu', '--use-gl=swiftshader', '--enable-unsafe-swiftshader',
  `--remote-debugging-port=${PORT}`, `--user-data-dir=${profile}`,
  '--no-sandbox', '--window-size=1680,1000', '--hide-scrollbars', 'about:blank',
], { stdio: ['ignore', 'pipe', 'pipe'] });
chrome.stderr.on('data', () => {});

async function getWs() {
  for (let i = 0; i < 60; i++) {
    try {
      const j = await (await fetch(`http://127.0.0.1:${PORT}/json/version`)).json();
      return j.webSocketDebuggerUrl;
    } catch { await new Promise(r => setTimeout(r, 250)); }
  }
  throw new Error('no cdp');
}
const ws = new WebSocket(await getWs());
await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
let msgId = 0; const pending = new Map(); const events = [];
ws.onmessage = (ev) => {
  const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  else if (m.method) events.push(m);
};
const send = (method, params = {}, sessionId) => new Promise((res) => {
  const id = ++msgId; pending.set(id, res);
  ws.send(JSON.stringify({ id, method, params, sessionId }));
});
const { result: t0r } = await send('Target.createTarget', { url: 'about:blank' });
const { result: sess } = await send('Target.attachToTarget', { targetId: t0r.targetId, flatten: true });
const sId = sess.sessionId;
const evalJS = async (expr) => (await send('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true }, sId)).result?.result?.value;
await send('Page.enable', {}, sId);
await send('Emulation.setDeviceMetricsOverride', { width: 1680, height: 1000, deviceScaleFactor: 1, mobile: false }, sId);
await send('Page.navigate', { url: `${BASE}?autotest=1` }, sId);

// wait until title == AUTOTEST_OK
let title = '';
for (let i = 0; i < 100; i++) {
  await new Promise(r => setTimeout(r, 2000));
  title = (await evalJS('document.title')) || '';
  if (title === 'AUTOTEST_OK') break;
  if (title.startsWith('AUTOTEST_FAIL')) break;
}
console.log('title =', title);

async function shot(name) {
  const { result } = await send('Page.captureScreenshot', { format: 'png' }, sId);
  writeFileSync(`/tmp/ui_${name}.png`, Buffer.from(result.data, 'base64'));
  console.log('saved', `/tmp/ui_${name}.png`);
}
await new Promise(r => setTimeout(r, 1500));
await shot('main');

// chat view (default tab already calc)
await shot('chat');

// drag splitters via JS layout check: set widths then shot
await evalJS(`document.documentElement.style.setProperty('--wL','380px'); document.documentElement.style.setProperty('--wR','420px');`);
await evalJS(`window.dispatchEvent(new Event('resize'))`);
await new Promise(r => setTimeout(r, 600));
await shot('resized');

// open docs overlay
await evalJS(`document.getElementById('btnDoc').click()`);
await new Promise(r => setTimeout(r, 800));
await shot('docs');
await evalJS(`document.getElementById('docClose').click()`);

// switch to heatmap tab
await evalJS(`document.querySelector('.tabbar button[data-tab="heatmap"]').click()`);
await new Promise(r => setTimeout(r, 700));
await shot('heatmap');

// scatter tab w/ PCA
await evalJS(`document.querySelector('.tabbar button[data-tab="scatter"]').click()`);
await evalJS(`document.getElementById('scPCA').click()`);
await new Promise(r => setTimeout(r, 700));
await shot('scatter');

// mlp tab
await evalJS(`document.querySelector('.tabbar button[data-tab="mlp"]').click()`);
await new Promise(r => setTimeout(r, 700));
await shot('mlp');

// back to calc tab, select different pair via JS
await evalJS(`document.querySelector('.tabbar button[data-tab="calc"]').click()`);
await evalJS(`window._DSH_UI.sel.qi=${await evalJS('window._DSH_UI.s.ids.length') - 2}; window._DSH_UI.sel.ki=${await evalJS('window._DSH_UI.s.ids.length') - 4}; window._DSH_UI.renderCalc(true); window._DSH_UI.renderTokenStrip();`);
await new Promise(r => setTimeout(r, 700));
await shot('calc');
ws.close(); chrome.kill('SIGKILL');
process.exit(0);
