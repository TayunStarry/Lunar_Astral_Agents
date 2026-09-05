// LTP3 引擎简易测试客户端
// 用法: node tools/ltp3_test_client.mjs [wsurl] [城市]
//   - wsurl  默认 ws://localhost:37336/ws（琉璃随机端口，按实际填写）
//   - 城市   默认 北京
//
// 说明: 该客户端连上琉璃 /ws 集线器，演示几条 ltp3/* 信封：
//   1) ltp3/ping       —— 探测引擎在线与已加载插件数
//   2) ltp3/manage list —— 列出引擎当前加载的插件（确认 com.yaraflow.weather 已在）
//   3) ltp3/tool get_weather —— 调用天气包核心工具（走 Open-Meteo/wttr 免费 API）
//   4) ltp3/hook chat.receive.after_process —— 演示钩子分发（该包未订阅钩子，预期 subscribed:0）
// 出站广播按 request_id 配对，只打印自己发起的回执。

// 使用全局 WebSocket（Node >= 22）；旧版 Node 回退到 npm 的 ws。
let WebSocketImpl = typeof globalThis.WebSocket !== 'undefined'
  ? globalThis.WebSocket
  : (await import('ws')).default;

const wsUrl = process.argv[2] || 'ws://localhost:37336/ws';
const city = process.argv[3] || '北京';

const ws = new WebSocketImpl(wsUrl);
let seq = 0;
const rid = (p) => `t-${p}-${++seq}`;

function send(obj) {
  console.log('[→]', JSON.stringify(obj));
  ws.send(JSON.stringify(obj));
}

ws.addEventListener('open', () => {
  console.log('已连接', wsUrl);

  // 1. 探测
  send({ type: 'ltp3/ping', request_id: rid('ping') });

  // 2. 列出插件
  setTimeout(() => send({ type: 'ltp3/manage', request_id: rid('list'), action: 'list' }), 300);

  // 3. 调用天气工具（间隔留给插件执行外部 HTTP）
  setTimeout(() => send({
    type: 'ltp3/tool',
    request_id: rid('tool'),
    tool: 'get_weather',
    payload: { city },
    context: { groupId: 'test-g1', platform: 'crystal_astral' }
  }), 1200);

  // 4. 演示钩子分发（该包未订阅钩子）
  setTimeout(() => send({
    type: 'ltp3/hook',
    request_id: rid('hook'),
    hook: 'chat.receive.after_process',
    payload: { id: 'm1', groupId: 'test-g1', senderName: 'tester', content: '今天天气如何', platform: 'qq' }
  }), 2000);

  // 5. 汇总后退出
  setTimeout(() => { console.log('--- 测试序列已发送，等待回执 8s 后退出 ---'); }, 2500);
});

ws.addEventListener('message', (ev) => {
  let m;
  try { m = JSON.parse(typeof ev.data === 'string' ? ev.data : ''); } catch { return; }
  if (!m || !String(m.type).startsWith('ltp3/')) return;

  switch (m.type) {
    case 'ltp3/pong':
      console.log('[←] pong  引擎在线，已加载插件数 =', m.plugins);
      break;
    case 'ltp3/manage_ack': {
      const ok = m.plugins || [];
      console.log(`[←] manage_ack (${m.action}) ok=${m.ok} 插件数=${ok.length}`);
      for (const p of ok) console.log('      ·', p.id, p.dir_name, p.loaded ? 'loaded' : `ERR:${p.error || ''}`);
      break;
    }
    case 'ltp3/tool_result': {
      console.log(`[←] tool_result "${m.tool}" subscribed=${m.summary.subscribed} errored=${m.summary.errored}`);
      for (const r of m.results) {
        if (r.error) { console.log('      ✗', r.plugin_id, 'error:', r.error); continue; }
        const d = r.result || {};
        if (d.error) { console.log('      ✗', r.plugin_id, '查询失败:', d.error); continue; }
        const cur = d.current || {};
        console.log(`      ✓ ${r.plugin_id}: ${d.city} ${cur.temperature}°C ${cur.condition} (来源 ${d.source})`);
        if (d.forecast) {
          for (const f of d.forecast) console.log(`        预报 ${f.date}: ${f.low}~${f.high}°C ${f.condition}`);
        }
      }
      break;
    }
    case 'ltp3/hook_result':
      console.log(`[←] hook_result "${m.hook}" subscribed=${m.summary.subscribed} errored=${m.summary.errored} allow_continue=${m.summary.allow_continue}`);
      break;
    case 'ltp3/error':
      console.log('[←] 引擎错误:', m.error);
      break;
    case 'ltp3/lifecycle':
      console.log(`[←] lifecycle ${m.event}`, m.plugin, m.title || '');
      break;
    case 'ltp3/send':
      console.log(`[←] 插件 send(${m.kind}, req=${m.request_id || '广播'}) group=${m.group_id}`, m.content || m.image || m.emoji || '');
      break;
    case 'ltp3/json': /* 忽略 */
    default:
      console.log('[←] 其它:', m.type);
  }
});

ws.addEventListener('close', () => { console.log('连接已关闭'); process.exit(0); });
ws.addEventListener('error', (e) => { console.error('连接出错:', e.message || e); });

setTimeout(() => { console.log('等待超时，退出'); process.exit(0); }, 12000);