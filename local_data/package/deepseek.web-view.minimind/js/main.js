// main.js — bootstrap: load tokenizer + weights, construct model & UI.

import { loadSafetensorsUrl, parseSafetensors } from './st.js';
import { BPETokenizer } from './tok.js';
import { Qwen3Model } from './model.js';
import { createBackend } from './mat.js';
import { Session } from './engine.js';
import { UI } from './ui.js';
import { drawFlow } from './viz.js';
import { esc } from './util.js';

const $ = id => document.getElementById(id);
const MODEL_DIR = 'minimind-3';

/* ---------- 资源获取：内嵌 JS 权重（默认） / HTTP fetch / file:// 本地文件夹授权 ---------- */

/** Decode concatenated base64 chunks (each length %4==0) into an ArrayBuffer. */
async function decodeEmbeddedWeights(onProgress) {
  const parts = window.__MM_B64 || [];
  if (!parts.length) throw new Error('未找到内嵌权重 (window.__MM_B64)');
  const totalChars = parts.reduce((a, s) => a + s.length, 0);
  const totalBytes = Math.floor(totalChars / 4 * 3);
  const SLICE = 4 * 1024 * 1024; // 4M chars = 3M bytes, multiple of 4
  const out = new Uint8Array(totalBytes);
  let off = 0, done = 0;
  for (const s of parts) {
    for (let i = 0; i < s.length; i += SLICE) {
      const seg = s.slice(i, i + SLICE);      // slice len is multiple of 4
      const bin = atob(seg);
      const n = bin.length;
      for (let j = 0; j < n; j++) out[off + j] = bin.charCodeAt(j);
      off += n; done += seg.length;
      if ((done / SLICE) % 3 === 0) {
        onProgress(done / totalChars);
        await new Promise(r => setTimeout(r, 0)); // keep UI alive
      }
    }
  }
  onProgress(1);
  return out.buffer;
}

function b64ToBytes(b64) {
  const bin = atob(b64);
  const u8 = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) u8[i] = bin.charCodeAt(i);
  return u8;
}

/** v2：内嵌 INT8 权重（qpack + FP32 norms），常驻内存仅 ~61MB。 */
async function loadResourcesEmbeddedQ(setMsg, setBar) {
  const parts = window.__MM_QPACK || [];
  if (!parts.length) throw new Error('未找到内嵌 INT8 权重 (window.__MM_QPACK)');
  const quantMap = new Map();
  let done = 0;
  for (const p of parts) {
    const q = new Int8Array(b64ToBytes(p.data));
    const scales = new Float32Array(b64ToBytes(p.scales).buffer);
    quantMap.set(p.name, { q, scales, dIn: p.dIn, dOut: p.dOut });
    done++;
    setBar(done / parts.length * 0.85);
    setMsg(`解码内嵌 INT8 权重 ${done}/${parts.length}（${p.name.split('.').slice(-2).join('.')}）…`);
    if (done % 8 === 0) await new Promise(r => setTimeout(r, 0));
  }
  const weights = new Map();
  for (const t of (window.__MM_MISC || [])) {
    weights.set(t.name, { dtype: 'F32', shape: t.shape, data: new Float32Array(b64ToBytes(t.b64).buffer) });
  }
  setMsg('初始化 INT8 权重布局…');
  return { config: window.__MM_CONFIG, tokenizerJson: window.__MM_TOKJSON, quantMap, tensors: weights, source: '内嵌 INT8 权重（v2 · 61MB 常驻）' };
}

async function loadResourcesEmbedded(setMsg, setBar) {
  setMsg('解码内置权重 JS（base64 → 122MB 二进制）…');
  const buf = await decodeEmbeddedWeights(f => setBar(f * 0.9));
  setMsg('解析 safetensors 并解码 FP16 → FP32…');
  const { tensors } = parseSafetensors(buf);
  return {
    config: window.__MM_CONFIG,
    tokenizerJson: window.__MM_TOKJSON,
    tensors,
    source: '内置权重（JS 代码直嵌，无需任何文件）',
  };
}

async function loadResourcesHTTP(setMsg, setBar) {
  setMsg('载入模型配置 config.json…');
  const config = await (await fetch(`${MODEL_DIR}/config.json`)).json();
  setMsg('载入分词器 tokenizer.json…');
  const tokenizerJson = await (await fetch(`${MODEL_DIR}/tokenizer.json`)).json();
  setMsg('下载权重 model.safetensors (122MB)…');
  const { tensors } = await loadSafetensorsUrl(`${MODEL_DIR}/model.safetensors`,
    (f, got, total) => { setBar(f * 0.92); setMsg(`下载权重… ${(got / 1048576).toFixed(0)}/${(total / 1048576).toFixed(0)} MB`); });
  return { config, tokenizerJson, tensors, source: `HTTP · ${MODEL_DIR}/` };
}

function pickLocalFolder() {
  return new Promise((resolve, reject) => {
    const wrap = document.createElement('div');
    wrap.style.cssText = 'display:flex;flex-direction:column;gap:12px;align-items:center;padding:18px 26px;border:1px dashed #0e7d99;background:#0b1622;max-width:600px;';
    wrap.innerHTML = `
      <div style="color:var(--cyan);font-family:var(--mono);font-size:13px;letter-spacing:.08em">检测到 file:// 直接打开</div>
      <div style="color:var(--txt);font-size:12.5px;line-height:1.9;text-align:left">
        浏览器安全策略禁止页面用 fetch 读取磁盘文件，<br>
        请<b style="color:var(--amber)">手动授权模型文件</b>（仅在本地内存读取，不会上传）：<br>
        方式一：选择项目根目录下的 <code style="color:var(--cyan)">minimind-3</code> 整个文件夹<br>
        方式二：分别选中 config.json / tokenizer.json / model.safetensors 三个文件<br>
        <span style="color:var(--dim)">(两者任选其一，凑齐三个文件即可)</span>
      </div>`;
    const mkInput = (dir) => {
      const input = document.createElement('input');
      input.type = 'file';
      if (dir) input.webkitdirectory = true;
      input.multiple = true;
      input.style.cssText = 'color:var(--dim);font-family:var(--mono);font-size:11px;';
      return input;
    };
    const inDir = mkInput(true);
    const inFiles = mkInput(false);
    const err = document.createElement('div');
    err.style.cssText = 'color:var(--red);font-size:11px;font-family:var(--mono);min-height:14px;';
    const lbl1 = document.createElement('div');
    lbl1.style.cssText = 'color:var(--dim);font-size:11px;font-family:var(--mono);';
    lbl1.textContent = '① 选择文件夹：';
    const lbl2 = document.createElement('div');
    lbl2.style.cssText = 'color:var(--dim);font-size:11px;font-family:var(--mono);margin-top:6px;';
    lbl2.textContent = '② 或逐个选择文件：';
    const tryResolve = () => {
      const files = [...Array.from(inDir.files || []), ...Array.from(inFiles.files || [])];
      const need = ['config.json', 'tokenizer.json', 'model.safetensors'];
      const missing = need.filter(n => !files.some(f => f.name === n));
      if (missing.length) {
        err.textContent = `还缺少: ${missing.join(' / ')}`;
        return;
      }
      resolve(files);
      wrap.remove();
    };
    inDir.onchange = tryResolve;
    inFiles.onchange = tryResolve;
    const cancel = document.createElement('button');
    cancel.textContent = '取消（改用 HTTP：先运行 ./start.sh）';
    cancel.onclick = () => { wrap.remove(); reject(new Error('已取消本地模式')); };
    wrap.appendChild(lbl1); wrap.appendChild(inDir);
    wrap.appendChild(lbl2); wrap.appendChild(inFiles);
    wrap.appendChild(cancel); wrap.appendChild(err);
    $('loadMsg').after(wrap);
  });
}

async function readLocalResources(files, setMsg, setBar) {
  const byName = new Map(files.map(f => [f.name, f]));
  setMsg('读取 config.json / tokenizer.json…');
  const config = JSON.parse(await byName.get('config.json').text());
  const tokenizerJson = JSON.parse(await byName.get('tokenizer.json').text());
  setBar(0.25);
  setMsg('读取 model.safetensors (122MB)…');
  const buf = await byName.get('model.safetensors').arrayBuffer();
  setBar(0.85);
  setMsg('解析 safetensors 头并解码 FP16 → FP32…');
  const { tensors } = parseSafetensors(buf);
  return { config, tokenizerJson, tensors, source: '本地文件夹 · file://' };
}

function logLine(kind, msg) {
  const box = $('logConsole');
  const t = new Date().toTimeString().slice(0, 8);
  const el = document.createElement('div');
  el.className = `log-line ${kind}`;
  el.innerHTML = `<span class="t">[${t}]</span> ${kind === 'info' ? `<b>${esc(msg)}</b>` : esc(msg)}`;
  box.appendChild(el);
  box.scrollTop = box.scrollHeight;
  while (box.childElementCount > 240) box.firstChild.remove();
}

async function boot() {
  const setMsg = (s) => { $('loadMsg').textContent = s; };
  const setBar = (f) => { $('loadBarFill').style.width = `${(f * 100).toFixed(1)}%`; };

  try {
    // ---- acquire resources: embedded JS weights > HTTP fetch > file:// picker
    const t0 = performance.now();
    let res;
    if (window.__MM_QPACK && window.__MM_QPACK.length && window.__MM_CONFIG && window.__MM_TOKJSON) {
      res = await loadResourcesEmbeddedQ(setMsg, setBar);
    } else if (window.__MM_B64 && window.__MM_B64.length && window.__MM_CONFIG && window.__MM_TOKJSON) {
      res = await loadResourcesEmbedded(setMsg, setBar);
    } else if (location.protocol === 'file:') {
      const files = await pickLocalFolder();
      res = await readLocalResources(files, setMsg, setBar);
    } else {
      res = await loadResourcesHTTP(setMsg, setBar);
    }
    const { config, tokenizerJson, tensors, source } = res;

    // ---- tokenizer
    const tok = new BPETokenizer(tokenizerJson);
    logLine('ok', `分词器就绪：vocab=${tok.vocabSize}，merges=${tok.ranks.size}，special=${tok.special.length}`);

    const dlMs = performance.now() - t0;
    setMsg('初始化计算后端…');
    await new Promise(r => setTimeout(r, 30));
    logLine('ok', `权重解析完成：${res.quantMap ? res.quantMap.size + ' 个 INT8 张量（INT8 原生 · 常驻 61MB）' : tensors.size + ' 张量'}，加载 ${ (dlMs/1000).toFixed(1) }s（来源：${source}）`);

    // ---- backend & model
    let backend;
    try {
      backend = await createBackend('auto', (s) => logLine('info', s));
    } catch (e) {
      backend = await createBackend('js', (s) => logLine('info', s));
    }
    await backend.warmup?.();
    const model = new Qwen3Model({ weights: tensors, config, backend, maxCtx: 2048, quantMap: res.quantMap || null, log: (s) => logLine('info', s) });
    $('tbBackend').textContent = backend.kind;
    $('tbLoadTime').textContent = `${((performance.now() - t0) / 1000).toFixed(1)}s`;
    logLine('ok', `模型就绪：${config.num_hidden_layers} 层 × ${config.num_attention_heads}Q/${config.num_key_value_heads}KV 头，d=${config.hidden_size}，θ=${config.rope_theta}`);

    // ---- session & ui
    const session = new Session(model, tok);
    const ui = new UI(session, tok, model, logLine);
    window._DSH_UI = ui; // debug handle

    ui.updateTokCount();

    // initial flow render + animation loop
    ui.drawFlow();
    const loop = () => { drawFlow($('flowCanvas'), ui); requestAnimationFrame(loop); };
    requestAnimationFrame(loop);

    setBar(1);
    logLine('info', '提示：① 左侧选择模式与输入 → 「预填并推理首 Token」（默认连续输出）；② 右下对话框可追问多轮；③ 关闭「连续输出」后用「单步 +1 Token」逐 Token 观察；④ 顶栏「📖 原理文档」内嵌全文。');
    logLine('info', '点击右上「📖 原理文档」阅读 8000 字详解。');

    // hide overlay
    setTimeout(() => { $('loadOverlay').style.opacity = '0'; $('loadOverlay').style.transition = 'opacity .4s'; setTimeout(() => $('loadOverlay').remove(), 450); }, 250);

    // error surface hook
    window.addEventListener('error', (ev) => logLine('err', `JS错误: ${ev.message}`));

    // ---- automated end-to-end test mode (?autotest=1)
    if (new URLSearchParams(location.search).has('autotest')) {
      (async () => {
        try {
          logLine('info', '[AUTOTEST] 预填默认输入…');
          await ui.runPrefill();
          logLine('info', `[AUTOTEST] prefill ok, ctx=${session.ids.length}`);
          for (let k = 0; k < 6; k++) await ui.stepOnce();
          logLine('info', `[AUTOTEST] 6 steps ok, ctx=${session.ids.length}`);
          await ui.selfTest();
          // multi-turn follow-up path
          await session.appendUserTurn('你叫什么名字？');
          for (let k = 0; k < 3; k++) await ui.stepOnce();
          logLine('info', `[AUTOTEST] follow-up turn ok, ctx=${session.ids.length}, turns=${session.turns.length}`);
          // pair-inspector sanity: recompute one dot & weight row
          const pool = session.capPool;
          const row = pool.layers[7].attnRows[session.ids.length - 1][3];
          const sum = row.reduce((a, b) => a + b, 0);
          logLine('info', `[AUTOTEST] attnRow(L7,H3,last) sum=${sum.toFixed(6)} len=${row.length}`);
          const turnsOk = session.turns.filter(t => t.role === 'user').length === 2;
          document.title = (Math.abs(sum - 1) < 1e-4 && turnsOk) ? 'AUTOTEST_OK' : 'AUTOTEST_FAIL_ATTN';
        } catch (e) {
          console.error(e);
          logLine('err', `[AUTOTEST] 失败: ${e.message}`);
          document.title = 'AUTOTEST_FAIL';
        }
      })();
    }
  } catch (e) {
    console.error(e);
    setMsg('加载失败：' + e.message);
    $('loadOverlay').querySelector('.spin')?.remove();
    logLine('err', `加载失败: ${e.message}`);
    const ov = $('loadOverlay');
    let hint = ov.querySelector('.hint');
    if (!hint) {
      hint = document.createElement('div');
      hint.className = 'lsys hint';
      hint.innerHTML = `请确认通过 HTTP 服务器访问本页面（例如 <b>./start.sh</b> 或 <b>python3 -m http.server</b>），<br>且 minimind-3/ 目录与 index.html 同级。<br><br>错误详情: ${esc(e.message)}`;
      ov.appendChild(hint);
    }
  }
}

boot();
