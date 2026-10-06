#!/usr/bin/env node
// E2E test for file:// double-click mode: bundle loads, folder picker authorizes, autotest passes.
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const FILE_URL = `file://${ROOT}/index.html?autotest=1`;
const PORT = 9366;
const FILES = [
  path.join(ROOT, 'minimind-3/config.json'),
  path.join(ROOT, 'minimind-3/tokenizer.json'),
  path.join(ROOT, 'minimind-3/model.safetensors'),
];

const profile = mkdtempSync(path.join(tmpdir(), 'chrome-prof-'));
const chrome = spawn('google-chrome', [
  '--headless=new', '--disable-gpu', '--use-gl=swiftshader', '--enable-unsafe-swiftshader',
  `--remote-debugging-port=${PORT}`, `--user-data-dir=${profile}`,
  '--no-sandbox', '--window-size=1680,1000', 'about:blank',
], { stdio: ['ignore', 'pipe', 'pipe'] });
chrome.stderr.on('data', () => {});

let wsUrl;
for (let i = 0; i < 60; i++) {
  try { wsUrl = (await (await fetch(`http://127.0.0.1:${PORT}/json/version`)).json()).webSocketDebuggerUrl; break; }
  catch { await new Promise(r => setTimeout(r, 250)); }
}
const ws = new WebSocket(wsUrl);
await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
let msgId = 0; const pending = new Map();
ws.onmessage = (ev) => { const m = JSON.parse(ev.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } };
const send = (method, params = {}, sessionId) => new Promise((res) => { const id = ++msgId; pending.set(id, res); ws.send(JSON.stringify({ id, method, params, sessionId })); });
const evalJS = async (expr) => (await send('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true }, sId)).result?.result?.value;

const { result: t } = await send('Target.createTarget', { url: 'about:blank' });
const { result: s } = await send('Target.attachToTarget', { targetId: t.targetId, flatten: true });
const sId = s.sessionId;
await send('Page.enable', {}, sId);
await send('Runtime.enable', {}, sId);
await send('DOM.enable', {}, sId);
await send('Page.navigate', { url: FILE_URL }, sId);

// wait for picker input to appear (bundle loaded + file:// branch)
let inputNode = null;
for (let i = 0; i < 40; i++) {
  await new Promise(r => setTimeout(r, 500));
  const q = await send('DOM.getDocument', {}, sId);
  const root = q.result.root.nodeId;
  const qs = await send('DOM.querySelectorAll', { nodeId: root, selector: 'input[type=file]:not([webkitdirectory])' }, sId);
  if (qs.result.nodeIds.length > 0) { inputNode = qs.result.nodeIds[0]; break; }
}
if (!inputNode) { console.log('FAIL: file picker input never appeared'); chrome.kill('SIGKILL'); process.exit(1); }
console.log('picker input found, injecting files…');
await send('DOM.setFileInputFiles', { files: FILES, nodeId: inputNode }, sId);
// CDP sets .files but may not fire the change event — dispatch it like a real user pick
await evalJS(`(() => { const i = document.querySelector('#loadOverlay input[type=file]:not([webkitdirectory])'); i.dispatchEvent(new Event('change', { bubbles: true })); return 'dispatched'; })()`);

let title = '';
const t0 = Date.now();
while (Date.now() - t0 < 180000) {
  await new Promise(r => setTimeout(r, 2500));
  try { title = (await evalJS('document.title')) || ''; } catch { }
  process.stdout.write(`\r t=${((Date.now() - t0) / 1000).toFixed(0)}s title="${title}"  `);
  if (title.startsWith('AUTOTEST')) break;
}
console.log('\nfinal title:', title);
const logs = (await evalJS(`Array.from(document.querySelectorAll('#logConsole .log-line')).map(e=>e.textContent.slice(0,150))`)) || [];
for (const l of logs.slice(0, 8)) console.log('  ', l);
console.log(title === 'AUTOTEST_OK' ? 'FILE-MODE PASS' : 'FILE-MODE FAIL');
ws.close(); chrome.kill('SIGKILL');
process.exit(title === 'AUTOTEST_OK' ? 0 : 1);
