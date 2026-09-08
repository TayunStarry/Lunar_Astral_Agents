// LTP9 工具调用自检：直连琉璃 /ws 集线器，验证 ltp9/call(event/stats/probe) 结果正确回传。
// 用法: node test_ltp9_call.js [wsurl]   （默认 ws://127.0.0.1:23577/ws）
// 内置最小 WebSocket 客户端（Node 无 ws/WebSocket 时可用）。
const net = require('net');
const crypto = require('crypto');

function wsConnect(url) {
  return new Promise((resolve, reject) => {
    const u = new URL(url);
    const key = crypto.randomBytes(16).toString('base64');
    const accept = crypto.createHash('sha1').update(key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
    const port = Number(u.port) || (u.protocol === 'wss:' ? 443 : 80);
    const path = u.pathname + u.search;
    const sock = net.connect(port, u.hostname);
    const listeners = { message: [], close: [], error: [] };
    let buf = Buffer.alloc(0);
    let done = false;
    const emit = (ev, ...a) => (listeners[ev] || []).forEach(f => f(...a));

    sock.on('connect', () => {
      sock.write('GET ' + path + ' HTTP/1.1\r\nHost: ' + u.host + '\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n' +
        'Sec-WebSocket-Key: ' + key + '\r\nSec-WebSocket-Version: 13\r\n\r\n');
    });
    sock.on('error', (e) => { if (!done) reject(new Error('连接失败: ' + e.message)); else emit('error', e); });
    sock.on('data', (chunk) => {
      buf = Buffer.concat([buf, chunk]);
      if (!done) {
        const headEnd = buf.indexOf('\r\n\r\n');
        if (headEnd >= 0) {
          const head = buf.slice(0, headEnd).toString();
          const statusLine = head.split('\r\n')[0];
          if (!/^HTTP\/1\.[01] 101/.test(statusLine)) { reject(new Error('握手失败: ' + statusLine)); sock.destroy(); return; }
          done = true;
          buf = buf.slice(headEnd + 4);
          resolve({ send, json, on, close: () => sock.end() });
        }
        if (done) pump();
      } else pump();
    });

    function parseFrame(b) {
      if (b.length < 2) return null;
      const opcode = b[0] & 0x0f, masked = b[1] & 0x80;
      let len = b[1] & 0x7f, off = 2;
      if (len === 126) { if (b.length < off + 2) return null; len = b.readUInt16BE(off); off += 2; }
      else if (len === 127) { if (b.length < off + 8) return null; len = Number(b.readBigUInt64BE(off)); off += 8; }
      if (!masked) { if (b.length < off + len) return null; return { opcode, payload: b.slice(off, off + len), rest: b.slice(off + len) }; }
      if (b.length < off + 4) return null;
      const mask = b.slice(off, off + 4); off += 4;
      if (b.length < off + len) return null;
      const payload = Buffer.alloc(len);
      for (let i = 0; i < len; i++) payload[i] = b[off + i] ^ mask[i & 3];
      return { opcode, payload, rest: b.slice(off + len) };
    }
    function pump() {
      for (;;) {
        const f = parseFrame(buf); if (!f) break; buf = f.rest;
        if (f.opcode === 1) emit('message', f.payload.toString());
        else if (f.opcode === 8) { emit('close'); sock.end(); }
      }
    }
    function encode(opcode, data) {
      const len = data.length;
      let head;
      if (len < 126) head = Buffer.from([0x80 | opcode, 0x80 | len]);
      else if (len < 65536) { head = Buffer.alloc(4); head[0] = 0x80 | opcode; head[1] = 0x80 | 126; head.writeUInt16BE(len, 2); }
      else { head = Buffer.alloc(10); head[0] = 0x80 | opcode; head[1] = 0x80 | 127; head.writeBigUInt64BE(BigInt(len), 2); }
      const key = crypto.randomBytes(4);
      const masked = Buffer.alloc(len);
      for (let i = 0; i < len; i++) masked[i] = data[i] ^ key[i & 3];
      const frame = Buffer.alloc(head.length + 4 + len);
      head.copy(frame, 0); key.copy(frame, head.length); masked.copy(frame, head.length + 4);
      return frame;
    }
    function send(text) { const d = Buffer.from(typeof text === 'string' ? text : JSON.stringify(text)); sock.write(encode(1, d)); }
    function json(obj) { send(JSON.stringify(obj)); }
    function on(ev, f) { listeners[ev].push(f); }
  });
}

const HOST = process.argv[2] || 'ws://127.0.0.1:23577/ws';

(async () => {
  const ws = await wsConnect(HOST);
  console.log('[连接] ' + HOST + ' -> OK');
  // 打印所有收到的 ltp9 消息，便于定位服务端究竟回传了什么
  ws.on('message', (msg) => {
    let m; try { m = JSON.parse(msg); } catch (e) { return; }
    if (typeof m.type === 'string' && (m.type.startsWith('ltp9/') || m.type.startsWith('l9_'))) {
      console.log('  << ' + m.type + ' rid=' + (m.request_id || '') + ' ' + JSON.stringify(m).slice(0, 300));
    }
  });

  const results = [];
  function waitResult(name, requestType, rid, timeout) {
    return new Promise((resolve) => {
      const timer = setTimeout(() => resolve({ name, ok: false, type: 'timeout', error: '超时（无结果回执）' }), timeout);
      function onMsg(msg) {
        let m; try { m = JSON.parse(msg); } catch (e) { return; }
        if (m.request_id !== rid) return;
        // 忽略 /ws 集线器对我们自己请求的“回声”，只接收宿主的处理结果回执
        if (m.type === requestType) return;
        clearTimeout(timer);
        const value = m.result && typeof m.result === 'object' ? m.result.value : m.result;
        const pluginsPresent = Array.isArray(m.plugins);
        let ok = false;
        if (requestType === 'ltp9/call') ok = value != null;
        else if (requestType === 'ltp9/event') ok = !!(m.result && m.result.summary && m.result.summary.subscribed > 0);
        else ok = pluginsPresent; // stats / probe
        resolve({ name, ok, type: m.type, value, plugins: m.plugins, raw: m });
      }
      ws.on('message', onMsg);
    });
  }

  // 1) stats
  { const rid = 't_stats_' + Date.now(); const p = waitResult('stats', 'ltp9/stats', rid, 15000); ws.json({ type: 'ltp9/stats', request_id: rid }); results.push(await p); }
  // 2) call: queryWeather 南京
  { const rid = 't_call_' + Date.now(); const p = waitResult('queryWeather(南京)', 'ltp9/call', rid, 40000); ws.json({ type: 'ltp9/call', request_id: rid, plugin: 'com.yaraflow.weather-ltp9', fn: 'queryWeather', args: ['南京'] }); results.push(await p); }
  // 2b) call: ping（验证引擎是否执行最新同步 execute.js）
  { const rid = 't_ping_' + Date.now(); const p = waitResult('ping', 'ltp9/call', rid, 10000); ws.json({ type: 'ltp9/call', request_id: rid, plugin: 'com.yaraflow.weather-ltp9', fn: 'ping', args: [] }); results.push(await p); }
  // 3) event: weather.query 广州
  { const rid = 't_event_' + Date.now(); const p = waitResult('weather.query(广州)', 'ltp9/event', rid, 40000); ws.json({ type: 'ltp9/event', request_id: rid, topic: 'weather.query', payload: { city: '广州' } }); results.push(await p); }
  // 4) probe
  { const rid = 't_probe_' + Date.now(); const p = waitResult('probe', 'ltp9/probe', rid, 15000); ws.json({ type: 'ltp9/probe', request_id: rid }); results.push(await p); }

  ws.close();
  console.log('\n===== LTP9 自检结果 =====');
  let allOk = true;
  for (const r of results) {
    const pass = r.ok ? 'PASS' : 'FAIL'; if (!r.ok) allOk = false;
    console.log(`[${pass}] ${r.name}  type=${r.type}`);
    if (r.value != null) console.log('       value = ' + JSON.stringify(r.value).slice(0, 400));
    else if (r.plugins) console.log('       plugins = ' + r.plugins.length + ' 个 -> ' + r.plugins.map(p => p.id).join(', '));
    else if (r.error) console.log('       error = ' + r.error);
    else console.log('       value = null');
  }
  console.log(allOk ? '\n全部通过 ✔' : '\n存在失败 ✘（需重新编译 crystal_astral 并重启后再跑）');
  process.exit(allOk ? 0 : 1);
})().catch((e) => { console.error('[脚本错误]', e && e.message ? e.message : e); process.exit(2); });