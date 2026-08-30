#!/usr/bin/env node
// Verify viz toggle / sliding window / ctx limit / memory warnings (single instance, guaranteed cleanup).
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

const MB = 1048576;
const URL_TO_TEST = process.argv[2] || 'http://127.0.0.1:8000/index.html?autotest=1';
const PORT = 9399;
const profile = mkdtempSync(path.join(tmpdir(), 'chrome-prof-'));
const chrome = spawn('google-chrome', [
  '--headless=new', '--disable-gpu', '--use-gl=swiftshader', '--enable-unsafe-swiftshader',
  `--remote-debugging-port=${PORT}`, `--user-data-dir=${profile}`,
  '--no-sandbox', '--window-size=1680,1000', 'about:blank',
], { stdio: ['ignore', 'pipe', 'pipe'] });
chrome.stderr.on('data', () => {});
const cleanup = () => { try { chrome.kill('SIGKILL'); } catch { } };
process.on('exit', cleanup);

let ws;
try {
  let wsUrl;
  for (let i = 0; i < 60; i++) {
    try { wsUrl = (await (await fetch(`http://127.0.0.1:${PORT}/json/version`)).json()).webSocketDebuggerUrl; break; }
    catch { await new Promise(r => setTimeout(r, 250)); }
  }
  ws = new WebSocket(wsUrl);
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
  let id = 0; const pending = new Map();
  ws.onmessage = ev => { const m = JSON.parse(ev.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } };
  const send = (method, params = {}, sessionId) => new Promise(res => { const i = ++id; pending.set(i, res); ws.send(JSON.stringify({ id: i, method, params, sessionId })); });
  const { result: t } = await send('Target.createTarget', { url: 'about:blank' });
  const { result: s } = await send('Target.attachToTarget', { targetId: t.targetId, flatten: true });
  const sId = s.sessionId;
  const ev = async (expr) => (await send('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true }, sId)).result?.result?.value;
  await send('Page.enable', {}, sId);
  await send('Page.navigate', { url: URL_TO_TEST }, sId);
  let title = '';
  for (let i = 0; i < 90; i++) { await new Promise(r => setTimeout(r, 2000)); title = (await ev('document.title')) || ''; if (title === 'AUTOTEST_OK') break; }
  console.log('title:', title);
  if (title !== 'AUTOTEST_OK') throw new Error('autotest failed');

  // stop background autoplay from autotest's chkStream
  await ev(`if (window._DSH_UI.autoTimer) window._DSH_UI.toggleAuto();`);
  await new Promise(r => setTimeout(r, 800));

  // 1) sliding window prune (trigger a merge by one step)
  await ev(`document.getElementById('selVizWin').value='64'; document.getElementById('selVizWin').onchange();`);
  await ev(`window._DSH_UI.stepOnce()`);
  await new Promise(r => setTimeout(r, 3500));
  const ctx1 = await ev(`window._DSH_UI.s.ids.length`);
  const prunedTo = await ev(`window._DSH_UI.s.capPool.prunedTo`);
  const keptBytes = await ev(`window._DSH_UI.s.captureBytes()`);
  console.log(`[prune] ctx=${ctx1} prunedTo=${prunedTo} kept=${(keptBytes/MB).toFixed(1)}MB`, prunedTo >= ctx1 - 65 ? '✓' : '✗');

  // 2) capture toggle off → ctx grows, capture bytes frozen
  await ev(`document.getElementById('chkCapture').click();`);
  const bytesBefore = await ev(`window._DSH_UI.s.captureBytes()`);
  const ctxBefore = await ev(`window._DSH_UI.s.ids.length`);
  await ev(`window._DSH_UI.stepOnce()`);
  await new Promise(r => setTimeout(r, 3500));
  const bytesAfter = await ev(`window._DSH_UI.s.captureBytes()`);
  const ctxAfter = await ev(`window._DSH_UI.s.ids.length`);
  console.log(`[toggle] off: ctx ${ctxBefore}→${ctxAfter}, captureBytes ${bytesBefore}→${bytesAfter}`,
    ctxAfter > ctxBefore && bytesAfter === bytesBefore ? '✓' : '✗');
  await ev(`document.getElementById('chkCapture').click();`); // back on

  // 3) UI select applies real ctxLimit option
  await ev(`document.getElementById('selCtxLim').value='128'; document.getElementById('selCtxLim').onchange();`);
  const limUI = await ev(`window._DSH_UI.s.ctxLimit`);
  console.log(`[ctxlimit] UI select → s.ctxLimit=${limUI}`, limUI === 128 ? '✓' : '✗');

  // 4) engine boundary: set limit just above current ctx, step until stop
  const ctxNow = await ev(`window._DSH_UI.s.ids.length`);
  await ev(`window._DSH_UI.s.ctxLimit = ${ctxNow + 2}`);
  for (let k = 0; k < 5; k++) {
    if (!(await ev(`window._DSH_UI.s.canStep()`))) break;
    await ev(`window._DSH_UI.stepOnce()`);
    await new Promise(r => setTimeout(r, 2200));
  }
  const stopReason = await ev(`window._DSH_UI.s.stoppedReason`);
  console.log(`[ctxlimit] stoppedReason: "${stopReason}"`, stopReason.includes('上下文限制') ? '✓' : '✗');

  // 4) memory telemetry
  const memLines = await ev(`document.getElementById('memLines').textContent`);
  const memWarn = await ev(`document.getElementById('memWarn').textContent + ' [' + document.getElementById('memWarn').className + ']'`);
  console.log('[memLines]', memLines);
  console.log('[memWarn ]', memWarn);
} catch (e) {
  console.error('TEST ERROR:', e.message);
  process.exitCode = 1;
} finally {
  try { ws && ws.close(); } catch { }
  cleanup();
}
