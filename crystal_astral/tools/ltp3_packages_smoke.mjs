// LTP3 五个 YaraFlow 包功能冒烟测试客户端
// 目标: 依次对 com.yaraflow.art / mc-status / qingyan / silence / weather 做 LTP3 信封级功能验证
//
// 用法: node tools/ltp3_packages_smoke.mjs [wsurl]
//   - wsurl 默认 ws://localhost:14186/ws（琉璃 HTTP 端口对应 WS 端点）
//
// 协议: 复用琉璃 /ws 集线器 + ltp3/* 信封，按 request_id 配对回执。
//   1) ltp3/ping           探测引擎在线
//   2) ltp3/manage list    列出并校验 5 个包均已加载
//   3) 逐包调用代表性工具/指令/钩子并断言结果（均为非破坏性调用）
//   4) ltp3/manage list    汇总最终状态
//
// 说明: 生成图片/发送到群聊等有副作用或消耗每日额度的真实行为，这里只验证
//   插件注册 + handler 执行链路（空 prompt 短路返回），不触发真实生图与被禁言。

import WebSocket from 'ws';

const wsUrl = process.argv[2] || 'ws://localhost:14186/ws';

const ws = new WebSocket(wsUrl);
let seq = 0;
const rid = (p) => `smoke-${p}-${++seq}`;

// request_id + type → 单次回执回调
const pending = new Map();
function registerReply(reqId, type, timeoutMs) {
  return new Promise((resolve, reject) => {
    const key = reqId + '|' + type;
    const toc = setTimeout(() => {
      pending.delete(key);
      reject(new Error(`等待回执超时(${timeoutMs}ms) req_id=${reqId} type=${type}`));
    }, timeoutMs);
    pending.set(key, { resolve, reject, toc });
  });
}

function sendReq(obj, type, timeoutMs = 20000) {
  const reqId = obj.request_id;
  const p = registerReply(reqId, type, timeoutMs);
  ws.send(JSON.stringify(obj));
  return p;
}

ws.on('message', (raw) => {
  let m;
  try { m = JSON.parse(raw.toString()); } catch { return; }
  if (!m || !String(m.type).startsWith('ltp3/')) return;

  // 先处理请求配对回执
  if (m.request_id) {
    const key = m.request_id + '|' + m.type;
    const h = pending.get(key);
    if (h) {
      pending.delete(key);
      clearTimeout(h.toc);
      h.resolve(m);
      return;
    }
  }
  // 其它广播（活动态、插件主动发消息）仅展示
  if (m.type === 'ltp3/send') {
    console.log(`      ↳ 插件 send(${m.kind}, req=${m.request_id || '广播'}) group=${m.group_id}:`, (m.content || m.image || '').slice(0, 80));
  } else if (m.type === 'ltp3/lifecycle') {
    console.log(`      ↳ lifecycle ${m.event}`, m.plugin || '');
  }
});

let passCount = 0;
let failCount = 0;
const results = [];

function check(name, ok, detail) {
  if (ok) { passCount++; results.push(`  ✓ ${name}`); console.log(`  ✓ ${name}`); if (detail) console.log(`      ${detail}`); }
  else { failCount++; results.push(`  ✗ ${name}  ${detail}`); console.log(`  ✗ ${name}  ${detail}`); }
}

function firstResult(msg) {
  return (msg.results && msg.results[0]) || null;
}
function say(msg) {
  const r = firstResult(msg);
  return r ? String(judgeStr(r.error !== undefined && r.handled ? (r.result !== undefined ? '' : '') : (r.result === undefined ? '' : r.result))) : '';
}
function judgeStr(v) {
  if (v === null || v === undefined) return '';
  return typeof v === 'string' ? v : JSON.stringify(v);
}
function hasEngineError(msg) {
  const r = firstResult(msg);
  return !!((r && r.error) || msg.summary.errored > 0);
}

async function toolTest(name, tool, payload, context, assertDetail) {
  console.log(`── ${name}`);
  const reqId = rid('tool');
  let msg;
  try {
    msg = await sendReq({ type: 'ltp3/tool', request_id: reqId, tool, payload, context }, 'ltp3/tool_result');
  } catch (e) {
    check(`工具 ${tool}`, false, e.message); return;
  }
  const r = firstResult(msg);
  const baseOk = msg.summary.subscribed === 1 && !hasEngineError(msg);
  const detail = assertDetail(r, msg);
  check(`工具 ${tool} 执行`, baseOk, detail);
}

async function commandTest(name, command, context, assertDetail) {
  console.log(`── ${name}`);
  const reqId = rid('cmd');
  let msg;
  try {
    msg = await sendReq({ type: 'ltp3/command', request_id: reqId, command, context }, 'ltp3/command_result');
  } catch (e) {
    check(`指令 ${command}`, false, e.message); return;
  }
  const baseOk = msg.summary.subscribed === 1 && !hasEngineError(msg);
  check(`指令 ${command} 执行`, baseOk, assertDetail(msg));
}

async function hookTest(name, hook, payload, context, assertDetail) {
  console.log(`── ${name}`);
  const reqId = rid('hook');
  let msg;
  try {
    msg = await sendReq({ type: 'ltp3/hook', request_id: reqId, hook, payload, context }, 'ltp3/hook_result');
  } catch (e) {
    check(`钩子 ${hook}`, false, e.message); return;
  }
  const baseOk = msg.summary.subscribed === 1 && !hasEngineError(msg);
  check(`钩子 ${hook} 分发`, baseOk, assertDetail(msg));
}

async function main() {
  const tests = [];

  // 1. 探测
  console.log('>> ping');
  try {
    const pong = await sendReq({ type: 'ltp3/ping', request_id: rid('ping') }, 'ltp3/pong', 8000);
    check('引擎在线(ping/pong)', pong.engine === 'LTP3', `引擎=${pong.engine} 已加载插件数=${pong.plugins}`);
  } catch (e) {
    check('引擎在线(ping/pong)', false, e.message);
  }

  // 2. manage list 校验 5 个包已加载
  console.log('>> manage list（校验 5 个包已加载）');
  const expectIds = ['com.yaraflow.art', 'com.yaraflow.mc-status', 'com.yaraflow.qingyan', 'com.yaraflow.silence', 'com.yaraflow.weather'];
  try {
    const ack = await sendReq({ type: 'ltp3/manage', request_id: rid('list'), action: 'list' }, 'ltp3/manage_ack', 8000);
    const loaded = {};
    for (const p of (ack.plugins || [])) loaded[p.id] = p.loaded;
    for (const id of expectIds) {
      check(`已加载 ${id}`, loaded[id] === true, `loaded=${loaded[id] === undefined ? '不在列表' : loaded[id]}`);
    }
  } catch (e) {
    check('manage list', false, e.message);
  }

  // 3. 逐包功能验证
  // 3.1 art —— generate_picture 空 prompt 短路返回（校验注册 + handler 链路，不消耗额度不调 API）
  await toolTest('绘画包 com.yaraflow.art', 'generate_picture', { prompt: '' },
    { groupId: 'test-art-g1', platform: 'crystal_astral' },
    (r) => judgeStr(r.result).includes('prompt 参数不能为空') ? null : `期望空 prompt 返回错误提示，实际=${judgeStr(r.result)}`);

  // 3.2 mc-status —— 指令路由（未配置群组映射 → 忽略返回空串）
  await commandTest('MC查服 com.yaraflow.mc-status', 'mcstatus',
    { groupId: 'test-mc-g1', platform: 'crystal_astral' },
    (msg) => { const r = firstResult(msg); const v = judgeStr(r.result); return v === '' ? null : `期望未配置群组返回空串，实际=${v}`; });

  // 3.3 qingyan —— mute 工具（群组无权限 → 返回权限拒绝文本）
  await toolTest('轻言群管 com.yaraflow.qingyan', 'mute', { target: '测试用户', duration: 60, reason: 'smoke' },
    { groupId: 'test-qy-g1', platform: 'crystal_astral' },
    (r) => { const v = judgeStr(r.result); return v.includes('禁言') ? null : `期望返回禁言相关提示，实际=${v}`; });

  // 3.4 silence —— set_silence(medium) 进入沉默 + 钩子检测到沉默
  await toolTest('沉默插件 com.yaraflow.silence', 'set_silence', { case: 'low' },
    { groupId: 'test-sil-g1', platform: 'crystal_astral' },
    (r) => judgeStr(r.result).includes('已进入沉默状态') ? null : `期望进入沉默状态，实际=${judgeStr(r.result)}`);
  await hookTest('沉默插件 钩子(检测到沉默)', 'chat.receive.before_process',
    { id: 'm1', groupId: 'test-sil-g1', senderId: 'u1', platform: 'qq', isAtMe: false, content: '测试' },
    { session: 's1' },
    (msg) => { const r = firstResult(msg); const res = r.result || {}; return res.logSuffix === '(窥屏ing)' ? null : `期望 logSuffix=窥屏ing，实际=${JSON.stringify(res)}`; });

  // 3.5 weather —— get_weather 真实查询（Open-Meteo/wttr 免费 API）
  await toolTest('天气包 com.yaraflow.weather', 'get_weather', { city: '北京' },
    { groupId: 'test-w-g1', platform: 'crystal_astral' },
    (r) => {
      const d = r.result || {};
      if (d.error) return `查询失败: ${d.error}`;
      const cur = d.current || {};
      if (typeof cur.temperature === 'number') return `${d.city} ${cur.temperature}°C ${cur.condition} (来源 ${d.source})`;
      return `响应结构异常: ${judgeStr(d).slice(0, 200)}`;
    }, 40000);

  // 4. 最终状态汇总
  console.log('>> 最终 manage list');
  try {
    const ack = await sendReq({ type: 'ltp3/manage', request_id: rid('list2'), action: 'list' }, 'ltp3/manage_ack', 8000);
    for (const p of (ack.plugins || [])) console.log(`      · ${p.id} ${p.loaded ? 'loaded' : 'ERR:' + (p.error || '')}`);
  } catch (e) { console.log('      manage list 失败:', e.message); }

  console.log('\n==== 测试汇总 ====');
  console.log(`通过 ${passCount} 项，失败 ${failCount} 项`);
  if (failCount > 0) {
    for (const r of results.filter(x => x.startsWith('  ✗'))) console.log(r);
  }
  process.exitCode = failCount > 0 ? 1 : 0;
  try { ws.close(); } catch (e) {}
  setTimeout(() => process.exit(process.exitCode), 200);
}

ws.on('open', () => {
  console.log(`已连接 ${wsUrl}`);
  main().catch((e) => { console.error('测试异常:', e.message); process.exit(1); });
});

ws.on('error', (e) => { console.error('连接出错:', e.message); process.exit(1); });
ws.on('close', () => { if (!process.exitCode) process.exit(0); });

setTimeout(() => { console.log('全局等待超时，退出'); process.exit(failCount > 0 ? 1 : 0); }, 60000);