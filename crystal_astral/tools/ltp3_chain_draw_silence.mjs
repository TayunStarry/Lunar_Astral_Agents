// LTP3 画图链路 + 沉默链路专项测试
// 目标:
//   A) 画图链路: generate_picture 走真实链路（提示词优化 → 火山方舟 Seedream → 生成图片 → 发送 image）
//   B) 沉默链路: set_silence 进入沉默 → before_process 窥屏 → after_process 阻断 → @打断解除沉默
//
// 用法: node tools/ltp3_chain_draw_silence.mjs [wsurl]
//   默认 ws://localhost:14186/ws
// 说明: 画图会真实调用绘图后端并消耗 1 次当日额度；沉默仅操作内存记录，无副作用。

import WebSocket from 'ws';

const wsUrl = process.argv[2] || 'ws://localhost:14186/ws';
const ws = new WebSocket(wsUrl);

let seq = 0;
const rid = (p) => `chain-${p}-${++seq}`;

const pending = new Map();   // request_id|type → {resolve,reject,toc}
const sends = [];            // 收集所有 ltp3/send（含单播回执）

function repair(reqId, type, timeoutMs) {
  return new Promise((resolve, reject) => {
    const key = reqId + '|' + type;
    const toc = setTimeout(() => { pending.delete(key); reject(new Error(`等待回执超时(${timeoutMs}ms) ${type}`)); }, timeoutMs);
    pending.set(key, { resolve, reject, toc });
  });
}
function sendReq(obj, type, timeoutMs = 20000) {
  const p = repair(obj.request_id, type, timeoutMs);
  ws.send(JSON.stringify(obj));
  return p;
}

ws.on('message', (raw) => {
  let m; try { m = JSON.parse(raw.toString()); } catch { return; }
  if (!m || !String(m.type).startsWith('ltp3/')) return;
  if (m.type === 'ltp3/send') { sends.push(m); return; }
  if (!m.request_id) return;
  const h = pending.get(m.request_id + '|' + m.type);
  if (h) { pending.delete(m.request_id + '|' + m.type); clearTimeout(h.toc); h.resolve(m); }
});

let pass = 0, fail = 0;
const ok = (name, cond, detail) => {
  if (cond) { pass++; console.log(`  ✓ ${name}`); if (detail) console.log(`      ${detail}`); }
  else { fail++; console.log(`  ✗ ${name}  ${detail || ''}`); }
};
const first = (m) => (m.results && m.results[0]) || null;
const txt = (r) => (r && r.result !== null && r.result !== undefined) ? (typeof r.result === 'string' ? r.result : JSON.stringify(r.result)) : '';

const ctx = (gid, extra = {}) => Object.assign({ groupId: gid, platform: 'crystal_astral' }, extra);

async function silenceChain() {
  console.log('══ 沉默链路 ──');
  const group = 'chain-sil-g1';

  // 1) 进入沉默
  const s1 = await sendReq({ type: 'ltp3/tool', request_id: rid('sil'), tool: 'set_silence', payload: { case: 'medium' }, context: ctx(group) }, 'ltp3/tool_result');
  ok('set_silence(medium) 进入沉默', s1.summary.subscribed === 1 && s1.summary.errored === 0 && txt(first(s1)).includes('已进入沉默状态'), txt(first(s1)));

  // 2) before_process：沉默中窥屏（非@）
  const b1 = await sendReq({ type: 'ltp3/hook', request_id: rid('pre'), hook: 'chat.receive.before_process', payload: { id: 'm1', groupId: group, senderId: 'u1', platform: 'qq', isAtMe: false, content: '你好' }, context: { session: 's1' } }, 'ltp3/hook_result');
  const r1 = (first(b1) || {}).result || {};
  ok('before_process 检测到沉默(窥屏)', b1.summary.subscribed === 1 && r1.logSuffix === '(窥屏ing)', `logSuffix=${r1.logSuffix}`);

  // 3) after_process：沉默中阻断发言
  const a1 = await sendReq({ type: 'ltp3/hook', request_id: rid('post'), hook: 'chat.receive.after_process', payload: { id: 'm2', groupId: group, senderId: 'u2', platform: 'qq', isAtMe: false, content: '测试' }, context: { session: 's1' } }, 'ltp3/hook_result');
  const r2 = (first(a1) || {}).result || {};
  ok('after_process 沉默中阻断回复', a1.summary.subscribed === 1 && r2.allowContinue === false, `allowContinue=${r2.allowContinue}`);

  // 4) @打断：解除沉默
  const b2 = await sendReq({ type: 'ltp3/hook', request_id: rid('at'), hook: 'chat.receive.before_process', payload: { id: 'm3', groupId: group, senderId: 'u3', platform: 'qq', isAtMe: true, content: '在吗' }, context: { session: 's1' } }, 'ltp3/hook_result');
  const r3 = (first(b2) || {}).result || {};
  ok('@打断解除沉默(不再窥屏)', b2.summary.subscribed === 1 && r3.allowContinue === true && !r3.logSuffix, `logSuffix=${r3.logSuffix}, allowContinue=${r3.allowContinue}`);

  // 5) 解除后 after_process 应放行
  const a2 = await sendReq({ type: 'ltp3/hook', request_id: rid('post2'), hook: 'chat.receive.after_process', payload: { id: 'm4', groupId: group, senderId: 'u4', platform: 'qq', isAtMe: false, content: '还在吗' }, context: { session: 's1' } }, 'ltp3/hook_result');
  const r4 = (first(a2) || {}).result || {};
  ok('解除后 after_process 放行', a2.summary.subscribed === 1 && r4.allowContinue === true, `allowContinue=${r4.allowContinue}`);
}

async function drawChain() {
  console.log('══ 画图链路 ──');
  const group = 'chain-draw-g1';
  const reqId = rid('draw');
  let msg;
  try {
    // 真实绘图：提示词优化(LLM) → Seedream(火山方舟) → 生成 → 发送 image
    msg = await sendReq({
      type: 'ltp3/tool', request_id: reqId, tool: 'generate_picture',
      payload: { prompt: '一只戴着草帽的柴犬，坐在海边看夕阳，治愈系插画' },
      context: ctx(group)
    }, 'ltp3/tool_result', 200000);
  } catch (e) {
    ok('generate_picture 真实生图', false, e.message); return;
  }
  const r = first(msg);
  const text = txt(r);
  ok('工具正常执行(无引擎错误)',
    msg.summary.subscribed === 1 && msg.summary.errored === 0 && !(r && r.error),
    r && r.error ? 'result.error=' + r.error : text);
  ok('绘图成功并提示已发送', text.includes('已生成图片并发送到群聊') && text.includes('剩余'), `返回=${text.slice(0, 120)}`);
  if (text.indexOf('绘图失败') >= 0 || text.indexOf('失败') >= 0) {
    console.log(`      ⚠ 返回文本含失败: ${text}`);
  }

  // 校验真实发出了 image 包（base64 已生成）
  const drawSend = sends.find(s => s.request_id === reqId && s.kind === 'image' && s.plugin_id === 'com.yaraflow.art');
  ok('插件向群聊发送了图片(base64)',
    !!drawSend && !!drawSend.image && String(drawSend.image).length > 100,
    drawSend ? `group=${drawSend.group_id} 图片base64长度=${String(drawSend.image || '').length}` : '未收到 image 发送广播');
}

async function main() {
  try { const pong = await sendReq({ type: 'ltp3/ping', request_id: rid('ping') }, 'ltp3/pong', 8000); ok('引擎在线', pong.engine === 'LTP3', `插件数=${pong.plugins}`); }
  catch (e) { ok('引擎在线', false, e.message); }

  await silenceChain();
  await drawChain();

  console.log(`\n==== 汇总：通过 ${pass}，失败 ${fail} ====`);
  process.exitCode = fail > 0 ? 1 : 0;
  try { ws.close(); } catch (e) {}
  setTimeout(() => process.exit(process.exitCode), 200);
}

ws.on('open', () => { console.log(`已连接 ${wsUrl}`); main().catch(e => { console.error('异常:', e.message); process.exit(1); }); });
ws.on('error', (e) => { console.error('连接出错:', e.message); process.exit(1); });
ws.on('close', () => { if (!process.exitCode) process.exit(0); });
setTimeout(() => { console.log('全局超时退出'); process.exit(fail > 0 ? 1 : 0); }, 200000);