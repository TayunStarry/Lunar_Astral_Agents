#!/usr/bin/env node
// CDP-driven end-to-end test: real browser, real network, waits for AUTOTEST title.
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

const URL_TO_TEST = process.argv[2] || 'http://127.0.0.1:8014/index.html?autotest=1';
const PORT = 9333;
const WAIT_MS = 240000;

const profile = mkdtempSync(path.join(tmpdir(), 'chrome-prof-'));
const chrome = spawn('google-chrome', [
  '--headless=new', '--disable-gpu', '--use-gl=swiftshader', '--enable-unsafe-swiftshader',
  `--remote-debugging-port=${PORT}`, `--user-data-dir=${profile}`,
  '--no-sandbox', '--window-size=1600,950', 'about:blank',
], { stdio: ['ignore', 'pipe', 'pipe'] });
chrome.stderr.on('data', () => {});

async function getWsUrl() {
  for (let i = 0; i < 50; i++) {
    try {
      const res = await fetch(`http://127.0.0.1:${PORT}/json/version`);
      const j = await res.json();
      return j.webSocketDebuggerUrl;
    } catch { await new Promise(r => setTimeout(r, 200)); }
  }
  throw new Error('chrome CDP not reachable');
}

const wsUrl = await getWsUrl();
const ws = new WebSocket(wsUrl);
await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });

let msgId = 0;
const pending = new Map();
ws.onmessage = (ev) => {
  const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
};
function send(method, params = {}, sessionId) {
  return new Promise((res) => {
    const id = ++msgId;
    pending.set(id, res);
    ws.send(JSON.stringify({ id, method, params, sessionId }));
  });
}

const { result: tgtInfo } = await send('Target.createTarget', { url: 'about:blank' });
const targetId = tgtInfo.targetId;
const { result: sess } = await send('Target.attachToTarget', { targetId, flatten: true });
const sId = sess.sessionId;

const evalJS = async (expr) => {
  const r = await send('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true }, sId);
  return r.result?.result?.value;
};
await send('Page.enable', {}, sId);
await send('Runtime.enable', {}, sId);
await send('Page.navigate', { url: URL_TO_TEST }, sId);

const t0 = Date.now();
let title = '';
let lastLogs = [];
while (Date.now() - t0 < WAIT_MS) {
  await new Promise(r => setTimeout(r, 3000));
  try {
    title = await evalJS('document.title') || '';
    lastLogs = await evalJS(`Array.from(document.querySelectorAll('#logConsole .log-line')).map(e=>e.textContent.slice(0,170))`) || [];
  } catch (e) { /* page reloading */ }
  process.stdout.write(`\r t=${((Date.now()-t0)/1000).toFixed(0)}s title="${title}" logs=${(lastLogs||[]).length}   `);
  if (title.startsWith('AUTOTEST')) break;
}
console.log('\n--- final title:', title);
for (const l of (lastLogs || [])) console.log('  ', l);
console.log(title === 'AUTOTEST_OK' ? '\nE2E PASS' : '\nE2E FAIL');
ws.close();
chrome.kill('SIGKILL');
process.exit(title === 'AUTOTEST_OK' ? 0 : 1);
