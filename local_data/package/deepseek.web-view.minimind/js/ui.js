// ui.js — panel wiring and interaction logic.

import { C, fmt, esc, fitCanvas, drawGridBg } from './util.js';import { drawFlow, flowHitTest, FLOW_TARGET, drawDotBars, drawSoftRow, drawSumViz, drawHeatmap, drawScatter, drawProbCurve } from './viz.js';

const $ = id => document.getElementById(id);

export class UI {
  constructor(session, tok, model, log) {
    this.s = session;
    this.tok = tok;
    this.m = model;
    this.log = log;
    this.sel = { qi: -1, ki: -1 };
    this.calc = { layer: 7, head: 0 };
    this.hm = { layer: 7, head: 0, allHeads: false };
    this.sc = { vec: 'q', layer: 7, head: 0, dx: 0, dy: 1, pca: false };
    this.mp = { layer: 7, tok: -1 };
    this.flowActive = null;
    this.autoTimer = null;
    this.hoverScatter = -1;
    this.bind();
  }

  tokens() {
    // derived token descriptors for the whole context
    return this.s.ids.map((id, i) => {
      const raw = this.tok.decode([id]);
      return {
        id,
        raw,
        short: shortTok(raw),
        kind: this.s.meta[i]?.kind || 'prompt',
      };
    });
  }

  bind() {
    // ---- inputs & run controls
    $('inpText').addEventListener('input', () => this.updateTokCount());
    $('btnRun').onclick = () => this.runPrefill();
    $('btnStep').onclick = () => this.stepOnce();
    $('btnAuto').onclick = () => this.toggleAuto();
    $('btnReset').onclick = () => { this.s.reset(); this.fullRefresh(); this.log('info', '上下文已重置'); };
    $('selMode').onchange = () => {
      const chat = $('selMode').value === 'chat';
      $('chatFields').style.display = chat ? '' : 'none';
      $('sbMode').textContent = $('selMode').value;
      this.updateTokCount();
    };
    $('inpSystem').addEventListener('input', () => { this.s.systemPrompt = $('inpSystem').value; this.updateTokCount(); });
    this.s.systemPrompt = $('inpSystem').value;

    // ---- chat transcript: follow-up input
    $('btnSend').onclick = () => this.sendChat();
    $('chatInput').addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' && !ev.shiftKey) { ev.preventDefault(); this.sendChat(); }
    });
    $('chatInput').addEventListener('input', () => {
      const el = $('chatInput');
      el.style.height = 'auto';
      el.style.height = `${Math.min(96, el.scrollHeight)}px`;
    });

    // ---- embedded docs overlay
    $('btnDoc').onclick = () => $('docOverlay').classList.add('open');
    $('docClose').onclick = () => $('docOverlay').classList.remove('open');
    document.addEventListener('keydown', (ev) => {
      if (ev.key === 'Escape') $('docOverlay').classList.remove('open');
    });

    // ---- precision (INT8 quantization)
    if (this.m.quantNative) {
      // v2 内嵌包：INT8 原生。FP32 需要原始权重（仅 HTTP 模式可加载）。
      $('selPrec').value = 'int8';
      $('vPrec').textContent = 'INT8';
      for (const o of $('selPrec').options) {
        if (o.value === 'fp32') {
          o.disabled = location.protocol === 'file:';
          o.textContent = location.protocol === 'file:'
            ? 'FP32（file:// 下无法加载原始权重）'
            : 'FP32（从 minimind-3/ 原始权重加载 · 需联网读取 122MB）';
        }
      }
      this.log('ok', '已加载 INT8 内嵌原生权重（W8A16 · 常驻 61MB）。FP32 基线对照可通过下拉切换（HTTP 模式）。');
    }
    $('selPrec').onchange = async () => {
      const mode = $('selPrec').value;
      let st;
      try {
        if (mode === 'fp32' && this.m.quantNative) {
          if (location.protocol === 'file:') {
            this.log('warn', 'file:// 下浏览器禁止读取本地权重文件 —— 请用 ./start.sh 以 HTTP 模式运行后再切换 FP32。');
            $('selPrec').value = 'int8';
            return;
          }
          this.log('info', '加载 minimind-3/model.safetensors（122MB）以构建 FP32 基线…');
          const { loadSafetensorsUrl } = await import('./st.js');
          const { tensors } = await loadSafetensorsUrl('minimind-3/model.safetensors',
            (f, got, total) => { this.log('info', `下载原始权重 ${(got / 1048576).toFixed(0)}/${(total / 1048576).toFixed(0)} MB`); });
          this.m.adoptFp32(tensors);
          this.log('ok', 'FP32 原始权重已就绪（244MB）—— 现可执行 FP32↔INT8 对照。');
          this.renderStatus();
          return;
        }
        st = this.m.setPrecision(mode);   // throws on unknown
      } catch (e) {
        this.log('err', `精度切换失败: ${e.message}`);
        $('selPrec').value = this.m.precision;
        return;
      }
      $('vPrec').textContent = mode.toUpperCase();
      if (mode === 'int8' && st && !this.m.quantNative) {
        this.log('ok', `已量化 INT8：${st.tensors} 张量 / ${(st.params / 1e6).toFixed(1)}M 参数，` +
          `${(st.fp32Bytes / 1048576).toFixed(0)}MB → ${(st.int8Bytes / 1048576).toFixed(1)}MB（${st.compression.toFixed(2)}×），` +
          `权重最大量化误差 ${st.maxErr.toExponential(2)}`);
        this.log('info', 'INT8 为 W8A16 权重量化：激活保持 FP32；公式 scale=max|W行|/127。详见文档 §7.6。');
      } else if (mode === 'int8') {
        this.log('info', '已切回 INT8 内嵌原生权重（常驻 61MB）。');
      } else {
        this.log('info', '已切回 FP32 全精度。');
      }
      this.renderStatus();
    };

    // ---- resource / viz-capture controls
    $('chkCapture').onchange = () => {
      this.s.viz.enabled = $('chkCapture').checked;
      this.log('warn', `可视化捕获已${this.s.viz.enabled ? '开启' : '关闭'}（关闭期间散点/热力图/演算台不再更新；KV Cache 不受影响）`);
      this.renderStatus();
    };
    $('selVizWin').onchange = () => {
      const v = +$('selVizWin').value;
      this.s.viz.window = v === 0 ? Infinity : v;
      $('vVizWin').textContent = v === 0 ? '全部' : v;
      this.log('info', `可视化窗口 = ${v === 0 ? '全部保留（内存随上下文平方增长）' : v + ' tokens（更早的捕获将被丢弃）'}`);
      this.renderStatus();
    };
    $('selCtxLim').onchange = () => {
      const v = +$('selCtxLim').value;
      this.s.ctxLimit = v;
      $('vCtxLim').textContent = v;
      this.log('warn', `上下文限制 = ${v}（新会话生效；若当前 ctx 已超出，生成将停止，请重置）`);
      if (this.s.ids.length > v) {
        this.s.stoppedReason = `上下文限制调整为 ${v}，当前 ${this.s.ids.length} 已超出 —— 请重置`;
        if (this.autoTimer) this.toggleAuto();
      }
      this.renderStatus();
    };

    // ---- layout splitters (draggable panel borders)
    this.initSplitters();

    // ---- sampling params
    const bindRange = (id, fmtv, cb) => {
      const el = $(id);
      const f = () => { $(el.dataset.v).textContent = fmtv(+el.value); cb(+el.value); };
      el.dataset.v = `v${id.slice(1)}`;
      el.addEventListener('input', f); f();
    };
    bindRange('rTemp', v => (v / 100).toFixed(2), v => this.s.params.temperature = v / 100);
    bindRange('rTopP', v => (v / 100).toFixed(2), v => this.s.params.topP = v / 100);
    bindRange('rTopK', v => (v >= 100 ? '∞ (≥100→关)' : String(v)), v => this.s.params.topK = v >= 100 ? Infinity : v);
    bindRange('rRep', v => (v / 100).toFixed(2), v => this.s.params.repPenalty = v / 100);
    bindRange('rDelay', v => String(v), v => { if (this.autoTimer) { this.toggleAuto(); this.toggleAuto(); } });
    bindRange('rMaxNew', v => String(v), v => this.s.params.maxNewTokens = v);
    $('chkGreedy').onchange = () => this.s.params.greedy = $('chkGreedy').checked;
    $('btnSelfTest').onclick = () => this.selfTest();

    // ---- calculator controls
    $('cLayer').addEventListener('input', () => { this.calc.layer = +$('cLayer').value; $('vLayer').textContent = this.calc.layer; this.syncPairSel(); this.renderCalc(); this.renderHeatmap(); });
    $('cHead').addEventListener('input', () => { this.calc.head = +$('cHead').value; $('vHead').textContent = this.calc.head; this.renderCalc(); this.renderHeatmap(); });
    $('cQuery').addEventListener('change', () => { this.sel.qi = +$('cQuery').value; if (this.sel.ki > this.sel.qi) this.sel.ki = this.sel.qi; this.syncPairSel(); this.renderCalc(); this.renderHeatmap(); });
    $('cKey').addEventListener('change', () => { this.sel.ki = +$('cKey').value; this.syncPairSel(); this.renderCalc(); });

    // ---- heatmap controls
    $('hmLayer').addEventListener('input', () => { this.hm.layer = +$('hmLayer').value; $('hmLv').textContent = this.hm.layer; this.hm.head = this.hm.head; this.renderHeatmap(); });
    $('hmHead').addEventListener('input', () => { this.hm.head = +$('hmHead').value; $('hmHd').textContent = this.hm.head; this.renderHeatmap(); });
    $('hmAllHeads').onchange = () => { this.hm.allHeads = $('hmAllHeads').checked; this.renderHeatmap(); };

    // ---- scatter controls
    $('scVec').onchange = () => { this.sc.vec = $('scVec').value; this.renderScatter(); };
    $('scLayer').addEventListener('input', () => { this.sc.layer = +$('scLayer').value; $('scLv').textContent = this.sc.layer; this.renderScatter(); });
    $('scHead').addEventListener('input', () => { this.sc.head = Math.min(3, +$('scHead').value); $('scHd').textContent = this.sc.head; this.renderScatter(); });
    $('scDX').onchange = () => { this.sc.dx = +$('scDX').value; this.renderScatter(); };
    $('scDY').onchange = () => { this.sc.dy = +$('scDY').value; this.renderScatter(); };
    $('scPCA').onchange = () => { this.sc.pca = $('scPCA').checked; this.renderScatter(); };

    // ---- mlp controls
    $('mpLayer').addEventListener('input', () => { this.mp.layer = +$('mpLayer').value; $('mpLv').textContent = this.mp.layer; this.renderMlp(); });
    $('mpTok').onchange = () => { this.mp.tok = +$('mpTok').value; this.renderMlp(); };

    // ---- tabs
    for (const b of document.querySelectorAll('.tabbar button')) {
      b.onclick = () => {
        document.querySelectorAll('.tabbar button').forEach(x => x.classList.remove('active'));
        b.classList.add('active');
        for (const p of document.querySelectorAll('.tabpage')) p.hidden = p.id !== `tab-${b.dataset.tab}`;
        this.refreshViz();
      };
    }

    // ---- flow canvas interactions
    const fc = $('flowCanvas');
    fc.addEventListener('mousemove', (ev) => {
      const r = fc.getBoundingClientRect();
      const node = flowHitTest(fc, ev.clientX - r.left, ev.clientY - r.top);
      fc.style.cursor = node ? 'pointer' : 'default';
      if (node) showTip(ev, `<span class="tt-t">${node.toUpperCase()}</span> 点击跳转对应监控面板`);
      else hideTip();
    });
    fc.addEventListener('mouseleave', hideTip);
    fc.addEventListener('click', (ev) => {
      const r = fc.getBoundingClientRect();
      const node = flowHitTest(fc, ev.clientX - r.left, ev.clientY - r.top);
      if (!node) return;
      const t = FLOW_TARGET[node];
      if (t === 'calc') activateTab('calc');
      else if (t === 'heatmap') activateTab('heatmap');
      else if (t === 'scatter-e') { this.sc.vec = 'e'; $('scVec').value = 'e'; activateTab('scatter'); }
      else if (t === 'scatter-q') { this.sc.vec = 'q'; $('scVec').value = 'q'; activateTab('scatter'); }
      else if (t === 'mlp') activateTab('mlp');
      else if (t === 'cand') $('candTable').scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      this.log('info', `流水线视图 → ${node}`);
    });

    // keyboard
    document.addEventListener('keydown', (ev) => {
      if (ev.target.matches('textarea, input[type=text], select')) return;
      if (ev.key === 's' || ev.key === 'S') this.stepOnce();
      else if (ev.key === 'a' || ev.key === 'A') this.toggleAuto();
      else if (ev.key === 'r' || ev.key === 'R') { this.s.reset(); this.fullRefresh(); }
    });

    window.addEventListener('resize', () => this.refreshViz());
  }

  // =================== actions ===================

  async runPrefill() {
    const text = $('inpText').value;
    if (!text.trim()) { this.log('warn', '输入为空'); return; }
    this.s.mode = $('selMode').value;
    $('btnRun').disabled = true;
    try {
      await this.s.prefill(text);
      // default selection: last prompt token as query
      this.sel.qi = this.s.ids.length - 1;
      this.sel.ki = this.s.ids.length - 1;
      this.fullRefresh(true);
      const d = this.s.distribution(this.s.logitsLast, this.s.params);
      this.log('ok', `下一个 token 候选：` + d.top.slice(0, 4).map(t => `${JSON.stringify(shortTok(t.txt))}(${(t.pPost * 100).toFixed(1)}%)`).join(' '));
      // 连续输出模式：发送后模型自动持续生成
      if ($('chkStream').checked) this.ensureAuto();
    } catch (e) {
      this.log('err', `预填失败: ${e.message}`);
      console.error(e);
    } finally {
      $('btnRun').disabled = false;
    }
  }

  async stepOnce() {
    if (!this.s.canStep()) return;
    try {
      const rec = await this.s.stepOnce();
      if (rec) {
        this.sel.qi = rec.pos; // auto-track latest
        if (this.sel.ki > rec.pos) this.sel.ki = rec.pos;
        this.fullRefresh();
        this.flowPulse(['head', 'dist', 'embed', 'ln1', 'qkv', 'rope', 'att', 'ores', 'ln2', 'ffn', 'res2', 'loop', 'fnorm']);
        this.log('info', `step@${rec.pos}: 采样 ${JSON.stringify(shortTok(rec.chosenTxt))} (P=${(rec.pPostChosen * 100).toFixed(2)}%, T=${rec.temperature}, greedy=${this.s.params.greedy}) ${rec.ms.toFixed(0)}ms`);
      }
    } catch (e) {
      this.log('err', `推理步失败: ${e.message}`);
      console.error(e);
    }
  }

  toggleAuto() {
    if (this.autoTimer) {
      clearInterval(this.autoTimer); this.autoTimer = null;
      $('btnAuto').classList.remove('active');
      $('btnAuto').textContent = '自动播放 ▸';
      this.log('info', '自动播放已暂停');
      return;
    }
    this.startAuto();
  }

  startAuto() {
    if (this.autoTimer) return;
    if (!this.s.canStep() && this.s.logitsLast == null) { this.log('warn', '请先预填输入'); return; }
    const delay = +$('rDelay').value;
    $('btnAuto').classList.add('active');
    $('btnAuto').textContent = '⏸ 停止';
    this.log('info', `连续输出开始（间隔 ${delay}ms）`);
    const tick = async () => {
      if (!this.s.canStep()) { this.toggleAuto(); return; }
      await this.stepOnce();
    };
    tick();
    this.autoTimer = setInterval(tick, delay);
  }

  ensureAuto() {
    if (!this.autoTimer && this.s.canStep()) this.startAuto();
  }

  /** 追问一轮：有上下文则增量追加 user turn；否则作为首条输入。 */
  async sendChat() {
    const text = $('chatInput').value.trim();
    if (!text) return;
    $('chatInput').value = '';
    $('chatInput').style.height = 'auto';
    if (this.autoTimer) this.toggleAuto(); // 先停连续输出，避免与增量预填竞速
    if (this.s.logitsLast == null) {
      // 尚未开始：作为初始输入
      $('inpText').value = text;
      this.updateTokCount();
      await this.runPrefill();
      return;
    }
    try {
      const r = await this.s.appendUserTurn(text);
      if (!r) return;
      this.sel.qi = this.s.ids.length - 1;
      this.sel.ki = this.s.ids.length - 1;
      this.fullRefresh(true);
      if ($('chkStream').checked) this.ensureAuto();
      else this.log('info', '单步模式：每点一次「单步 +1 Token」输出一个 Token');
    } catch (e) {
      this.log('err', `追问失败: ${e.message}`);
      console.error(e);
    }
  }

  initSplitters() {
    const persist = () => {
      try {
        const cs = getComputedStyle(document.documentElement);
        localStorage.setItem('dsh_layout_v2', JSON.stringify({
          wL: cs.getPropertyValue('--wL').trim(),
          wR: cs.getPropertyValue('--wR').trim(),
          hTop: $('centerTop').style.height || '',
          hChat: $('chatPanel').style.flexBasis || '',
        }));
      } catch { /* ignore */ }
    };
    const restore = () => {
      try {
        const saved = JSON.parse(localStorage.getItem('dsh_layout_v2') || 'null');
        if (!saved) return;
        if (saved.wL) document.documentElement.style.setProperty('--wL', saved.wL);
        if (saved.wR) document.documentElement.style.setProperty('--wR', saved.wR);
        if (saved.hTop) $('centerTop').style.height = saved.hTop;
        if (saved.hChat) $('chatPanel').style.flexBasis = saved.hChat;
      } catch { /* ignore */ }
    };
    restore();

    const drag = (el, onMove) => {
      el.addEventListener('pointerdown', (ev) => {
        ev.preventDefault();
        el.setPointerCapture(ev.pointerId);
        el.classList.add('dragging');
        document.body.style.userSelect = 'none';
        const move = (e2) => onMove(e2);
        const up = () => {
          el.classList.remove('dragging');
          document.body.style.userSelect = '';
          el.removeEventListener('pointermove', move);
          el.removeEventListener('pointerup', up);
          persist();
          this.refreshViz();
        };
        el.addEventListener('pointermove', move);
        el.addEventListener('pointerup', up);
      });
      el.addEventListener('dblclick', () => { /* reserved: snap to default */ });
    };

    drag($('splitL'), (e2) => {
      const w = Math.max(210, Math.min(560, e2.clientX - 6));
      document.documentElement.style.setProperty('--wL', `${w}px`);
    });
    drag($('splitR'), (e2) => {
      const w = Math.max(240, Math.min(560, window.innerWidth - e2.clientX - 6));
      document.documentElement.style.setProperty('--wR', `${w}px`);
    });
    drag($('splitC'), (e2) => {
      const top = $('centerTop');
      const h = Math.max(90, Math.min(window.innerHeight * 0.6, e2.clientY - top.getBoundingClientRect().top));
      top.style.height = `${h}px`;
    });
    drag($('splitChat'), (e2) => {
      const panel = $('chatPanel');
      const colR = $('colRight').getBoundingClientRect();
      const h = Math.max(140, Math.min(colR.bottom - e2.clientY, window.innerHeight * 0.8));
      panel.style.flex = `0 0 ${h}px`;
    });
    // re-render canvases once layout settles after any drag
    window.addEventListener('pointerup', () => setTimeout(() => this.refreshViz(), 30));
  }

  async selfTest() {
    this.log('info', '交叉验证：TF(当前后端) vs 纯JS 内核对同一前向的 logits 偏差…');
    const { createBackend } = await import('./mat.js');
    const ids = this.s.ids.slice(0, Math.min(this.s.ids.length, 24));
    if (ids.length < 2) { this.log('warn', '需要先预填≥2个token'); return; }
    const qm = this.m.quantNative ? this.m.quant : null;   // INT8 原生时对照模型也用同一量化包
    const mb = new this.m.constructor({ weights: this.m.w, config: this.m.cfg, backend: this.m.backend, maxCtx: this.m.maxCtx, quantMap: qm, log: () => {} });
    const mj = new this.m.constructor({ weights: this.m.w, config: this.m.cfg, backend: await createBackend('js'), maxCtx: this.m.maxCtx, quantMap: qm, log: () => {} });
    const rb = await mb.forward(ids, { capture: 'none' });
    const rj = await mj.forward(ids, { capture: 'none' });
    let mx = 0;
    for (let i = 0; i < rj.logitsLast.length; i++) mx = Math.max(mx, Math.abs(rj.logitsLast[i] - rb.logitsLast[i]));
    this.log('ok', `交叉验证完成：max|Δlogits| = ${mx.toExponential(2)}  ${mx < 5e-2 ? '✓ 一致' : '⚠ 偏差较大'}`);

    // ---- FP32 vs INT8 (weight-only quantization) comparison ----
    if (!this.m.hasFp32) {
      this.log('info', 'INT8 内嵌模式无 FP32 基线 —— 如需量化对照，请用 HTTP 模式并在精度下拉加载原始权重。');
      return;
    }
    try {
      const mf = new this.m.constructor({ weights: this.m.w, config: this.m.cfg, backend: await createBackend('js'), maxCtx: this.m.maxCtx, log: () => {} });
      const mi = new this.m.constructor({ weights: this.m.w, config: this.m.cfg, backend: await createBackend('js'), maxCtx: this.m.maxCtx, log: () => {} });
      mi.setPrecision('int8');
      const rf = await mf.forward(ids, { capture: 'none' });
      const ri = await mi.forward(ids, { capture: 'none' });
      let dq = 0; let topF = 0, topI = 0;
      for (let i = 0; i < ri.logitsLast.length; i++) {
        if (Math.abs(ri.logitsLast[i] - rf.logitsLast[i]) > dq) dq = Math.abs(ri.logitsLast[i] - rf.logitsLast[i]);
        if (rf.logitsLast[i] > rf.logitsLast[topF]) topF = i;
        if (ri.logitsLast[i] > ri.logitsLast[topI]) topI = i;
      }
      // distribution agreement: Σ min(P_fp32, P_int8) over softmax (1 = identical)
      const sm = (arr) => {
        let mx = -Infinity; for (const v of arr) if (v > mx) mx = v;
        let sum = 0; const p = new Float64Array(arr.length);
        for (let i = 0; i < arr.length; i++) { p[i] = Math.exp(arr[i] - mx); sum += p[i]; }
        for (let i = 0; i < p.length; i++) p[i] /= sum;
        return p;
      };
      const pf = sm(rf.logitsLast), pi = sm(ri.logitsLast);
      let agree = 0;
      for (let i = 0; i < pf.length; i++) agree += Math.min(pf[i], pi[i]);
      this.log('ok', `量化对照 FP32↔INT8：max|Δlogits| = ${dq.toExponential(2)}，Top-1 ${topF === topI ? '一致' : `不一致(${topF} vs ${topI})`}，分布重合度 ${(agree * 100).toFixed(1)}%`);
    } catch (e2) { this.log('err', `量化对照失败: ${e2.message}`); }
  }

  // =================== rendering ===================

  updateTokCount() {
    const text = $('inpText').value;
    const n = this.tok.encode(text).length;
    $('tokCount').textContent = `${n} tok`;
  }

  fullRefresh(rebuild = false) {
    this.renderTokenStrip(rebuild);
    this.renderCalc(true);
    this.renderHeatmap();
    this.renderScatter();
    this.renderMlp();
    this.renderCands();
    this.renderGenStream();
    this.renderStatus();
    this.updateTokCount();
  }

  refreshViz() {
    this.renderCalc(true);
    this.renderHeatmap();
    this.renderScatter();
    this.renderMlp();
  }

  // ---------- token strip ----------
  renderTokenStrip(rebuild) {
    const strip = $('tokenStrip');
    const toks = this.tokens();
    if (rebuild || strip.childElementCount !== toks.length) {
      strip.innerHTML = '';
      toks.forEach((t, i) => {
        const el = document.createElement('span');
        el.className = 'tok-chip' + (t.kind === 'gen' ? ' gen' : '') + (isSpecialText(t.raw) ? ' special' : '');
        el.dataset.i = i;
        el.innerHTML = `<span class="n">${i}</span>${esc(shortTok(t.raw))}`;
        el.onclick = (ev) => {
          if (ev.altKey) { this.sel.ki = i; }
          else if (ev.shiftKey) { this.sel.ki = i; }
          else { this.sel.qi = i; if (this.sel.ki > i) this.sel.ki = i; }
          this.syncPairSel(); this.renderCalc(); this.renderHeatmap();
        };
        el.oncontextmenu = (ev) => { ev.preventDefault(); this.sel.ki = i; this.syncPairSel(); this.renderCalc(); };
        strip.appendChild(el);
      });
    }
    strip.style.display = '';
    [...strip.children].forEach((el, i) => {
      el.classList.toggle('qsel', i === this.sel.qi);
      el.classList.toggle('ksel', i === this.sel.ki);
    });
    $('pairStatus').innerHTML =
      `查询 Query：<b>${this.sel.qi >= 0 ? `${this.sel.qi}·${esc(shortTok(toks[this.sel.qi]?.raw ?? ''))}` : '—'}</b>` +
      `　键 Key：<b>${this.sel.ki >= 0 ? `${this.sel.ki}·${esc(shortTok(toks[this.sel.ki]?.raw ?? ''))}` : '—'}</b>` +
      `　<span style="color:var(--faint)">(左键选Q / 右键或Shift选K)</span>`;
    // calculator dropdowns
    const qsel = $('cQuery'), ksel = $('cKey');
    if (qsel.options.length !== toks.length) {
      qsel.innerHTML = toks.map((t, i) => `<option value="${i}">${i}·${esc(shortTok(t.raw))}</option>`).join('');
      ksel.innerHTML = qsel.innerHTML;
    }
    if (this.mp.tok < 0) {
      const msel = $('mpTok');
      msel.innerHTML = toks.map((t, i) => `<option value="${i}">${i}·${esc(shortTok(t.raw))}</option>`).join('');
      this.mp.tok = toks.length - 1;
    }
  }

  syncPairSel() {
    const n = this.s.ids.length;
    if (this.sel.qi >= n) this.sel.qi = n - 1;
    if (this.sel.ki > this.sel.qi) this.sel.ki = this.sel.qi;
    if (this.sel.ki < 0) this.sel.ki = Math.min(this.sel.qi, n - 1);
    $('cQuery').value = this.sel.qi;
    $('cKey').value = this.sel.ki;
  }

  // ---------- calculator ----------
  renderCalc(full = false) {
    const s = this.s, m = this.m;
    if (!s.capPool || s.ids.length < 1) return;
    if (full) this.syncPairSel();
    const { layer: l, head: h } = this.calc;
    const i = Math.max(0, this.sel.qi), j = Math.min(this.sel.ki, i);
    const hd = m.headDim;
    const g = Math.floor(h / m.rep);
    const qVec = s.capPool.layers[l].q[i]?.subarray(h * hd, (h + 1) * hd);
    const kVec = m.getKeyVec(l, j, g);
    const row = s.capPool.layers[l].attnRows[i]?.[h];
    if (!s.viz.enabled) {
      $('fDot').innerHTML = '<span style="color:var(--faint)">可视化捕获已关闭 —— 打开 Module02 的「可视化捕获」开关以启用演算台。</span>';
      $('fSoft').innerHTML = ''; $('fSum').innerHTML = '';
      return;
    }
    if (!qVec || !row) {
      const msg = '<span style="color:var(--amber)">该 token 的捕获已随可视化窗口（' + (s.viz.window === Infinity ? '全部' : s.viz.window) + '）滚动丢弃 —— 选择更近的 token、调大窗口或重置。</span>';
      $('fDot').innerHTML = msg; $('fSoft').innerHTML = ''; $('fSum').innerHTML = '';
      return;
    }

    // ---- B: dot bars
    if (qVec && kVec) {
      let dot = 0;
      for (let d = 0; d < hd; d++) dot += qVec[d] * kVec[d];
      drawDotBars($('dotBars'), qVec, kVec, { highlightDim: -1 });
      $('fDot').innerHTML =
        `<span class="fn">q</span><sub>${i},${h}</sub> · <span class="fn">k</span><sub>${j},KV${g}</sub> = ` +
        `<span class="num">${fmt(dot, 2)}</span> &nbsp;<span class="op">/</span>&nbsp; √${hd}=` +
        `<span class="num">${Math.sqrt(hd).toFixed(3)}</span> = ` +
        `<span class="res">score(${i}→${j}) = ${fmt(dot * m.scale, 4)}</span>`;
    }

    // ---- C: softmax row
    if (row) {
      drawSoftRow($('softRow'), row, j, null, `layer=${l} head=${h} · 行和=${row.reduce((a, b) => a + b, 0).toFixed(6)}`);
      // reconstruct score_ij for formula display (deterministic recompute from rotated vecs)
      let scoreIJ = 0, maxScore = -Infinity;
      if (qVec && kVec) {
        for (let d2 = 0; d2 < hd; d2++) scoreIJ += qVec[d2] * kVec[d2];
        scoreIJ *= m.scale;
      }
      const gIdx = Math.floor(h / m.rep);
      for (let t = 0; t <= i; t++) {
        const kt = m.getKeyVec(l, t, gIdx);
        let d2 = 0;
        for (let dd = 0; dd < hd; dd++) d2 += qVec[dd] * kt[dd];
        d2 *= m.scale;
        if (d2 > maxScore) maxScore = d2;
      }
      $('fSoft').innerHTML =
        `score = <span class="num">${fmt(scoreIJ, 3)}</span>（行内最大 <span class="num">${fmt(maxScore, 3)}</span>）` +
        ` &nbsp;<span class="op">→</span>&nbsp; exp(score−max) = <span class="num">${fmt(Math.exp(scoreIJ - maxScore), 5)}</span>` +
        ` &nbsp;<span class="op">÷</span>&nbsp; Σ_t exp = ` +
        `&nbsp;<span class="op">⇒</span>&nbsp; <span class="res">w(${i}→${j}) = ${row[j] !== undefined ? fmt(row[j], 5) : '—'}</span>` +
        `<br><span style="color:var(--faint)">softmax(s)_j = exp(s_j − s_max) / Σ exp(s_t − s_max)　·　数值稳定减最大值防溢出</span>`;
      // mask check stage
      $('maskCheck').innerHTML = j <= i
        ? `<div class="note">j=${j} ≤ i=${i} → <b style="color:var(--green)">允许</b>：Key 不在 Query 未来。注意力可读取该 token。</div>`
        : `<div class="note">j=${j} &gt; i=${i} → <b style="color:var(--red)">因果掩码屏蔽</b>：score 置 −∞，softmax 后权重为 0。语言模型不能偷看未来！</div>`;
      // outflow
      const pos = i;
      const outVec = s.capPool.layers[l].attnContrib[pos];
      let onorm = 0;
      if (outVec) { for (let d2 = 0; d2 < outVec.length; d2++) onorm += outVec[d2] * outVec[d2]; }
      $('outFlowNote').innerHTML =
        `该行加权结果经 <code>W_o</code> (768×768) 合并 8 头 → 加进残差流。<br>` +
        `此 token 此刻的注意力贡献范数 ‖attn‖≈<b>${outVec ? fmt(Math.sqrt(onorm), 2) : '—'}</b>，` +
        `MLP 贡献随后叠加（见 ⑤ FFN 视图）。`;
    }

    // ---- D: weighted sum viz
    const vs = [];
    if (row) {
      const kk = Math.min(i, j);
      // show contributions of the 8 largest weights incl. selected
      const idxs = Array.from({ length: row.length }, (_, t) => t);
      idxs.sort((a, b) => row[b] - row[a]);
      const chosen = new Set(idxs.slice(0, 7)); chosen.add(kk);
      const list = [...chosen].sort((a, b) => a - b);
      for (const t of list) {
        vs.push({
          j: t,
          weight: row[t],
          txt: shortTok(this.tok.decode([s.ids[t]])),
          color: t === kk ? C.amber : `rgba(77,163,255,${0.3 + 0.6 * (row[t] / Math.max(...row))})`,
        });
      }
    }
    if (row && vs.length) {
      const outHead = new Float32Array(hd);
      const g2 = Math.floor(h / m.rep);
      for (let t = 0; t < row.length; t++) {
        const vv = m.getValueVec(l, t, g2);
        for (let d = 0; d < hd; d++) outHead[d] += row[t] * vv[d];
      }
      drawSumViz($('sumViz'), vs, outHead, outHead, j <= i ? vs.findIndex(v => v.j === j) : -1,
        `Σw·v → o(${i},h${h})`);
      $('fSum').innerHTML =
        `o<sub>${i},${h}</sub> = Σ<sub>t≤${i}</sub> w<sub>t</sub>·v<sub>t</sub> &nbsp;` +
        `(显示贡献最大的 ${vs.length} 项，★为选中 Key j=${j}，w=${row[j] !== undefined ? fmt(row[j], 4) : '—'})`;
    }

    // statusbar
    if (row && row[j] !== undefined) {
      $('sbLayer').textContent = l; $('sbHead').textContent = h; $('sbGroup').textContent = g;
      let dot2 = 0;
      if (qVec && kVec) for (let d = 0; d < hd; d++) dot2 += qVec[d] * kVec[d];
      $('sbScore').textContent = fmt(dot2 * m.scale, 3);
      $('sbProb').textContent = this.s.logitsLast ? '' : '';
    }
  }

  // ---------- heatmap ----------
  renderHeatmap() {
    drawHeatmap($('heatCanvas'), this.s, this.hm.layer, this.hm.head, this.tokens(), this.hm.allHeads,
      (qi, kj) => {
        this.sel.qi = qi; this.sel.ki = kj;
        this.calc.layer = this.hm.layer; $('cLayer').value = this.hm.layer; $('vLayer').textContent = this.hm.layer;
        this.calc.head = this.hm.head; $('cHead').value = this.hm.head; $('vHead').textContent = this.hm.head;
        this.syncPairSel(); this.renderCalc(); this.renderTokenStrip();
        activateTab('calc');
        this.log('info', `热力图选中 (${qi}→${kj}) 已同步至演算台`);
      },
      (cell) => {
        if (!cell) { hideTip(); return; }
        const toks = this.tokens();
        showTipAt(event, `<span class="tt-t">(${cell.i} → ${cell.j})</span> ` +
          `${esc(shortTok(toks[cell.i].raw))} ⇒ ${esc(shortTok(toks[cell.j].raw))}` +
          `${cell.wgt != null ? ` · w=${cell.wgt.toFixed(4)}` : ' · 被掩码'}`);
      });
  }

  // ---------- scatter ----------
  renderScatter() {
    const s = this.s;
    if (!s.capPool) return;
    const P = s.capPool;
    const nT = s.ids.length;
    const l = this.sc.layer;
    let pts = [];
    const vecFor = (t) => {
      switch (this.sc.vec) {
        case 'q': { const hd = this.m.headDim; const base = P.layers[l].q[t]; if (!base) return null; const hOff = this.sc.head * hd; return base.subarray(hOff, hOff + hd); }
        case 'k': return this.m.getKeyVec(l, t, this.sc.head);
        case 'v': return this.m.getValueVec(l, t, this.sc.head);
        case 'e': return P.embed[t];
        case 'ho': return P.hiddenAll[t];
      }
    };
    let dims = [this.sc.dx, this.sc.dy];
    const hdQ = this.m.headDim, dV = this.sc.vec === 'e' || this.sc.vec === 'ho' ? this.m.dModel : hdQ;
    const normDim = (d) => d < 0 ? dV + d : d;
    dims = dims.map(normDim);
    dims = dims.map(d => Math.max(0, Math.min(dV - 1, d)));
    if (this.sc.pca) {
      pts = computePCA2(vecFor, nT, this.sc.vec);
    } else {
      for (let t = 0; t < nT; t++) {
        const vec = vecFor(t);
        if (!vec) continue;
        const types = this.sc.vec === 'q' ? ['Q'] : this.sc.vec === 'k' ? ['K'] : this.sc.vec === 'v' ? ['V'] : ['Q', 'K', 'V'];
        void types;
        const typ = this.sc.vec === 'q' ? 'Q' : this.sc.vec === 'k' ? 'K' : this.sc.vec === 'v' ? 'V' : 'E';
        pts.push({ x: vec[dims[0]], y: vec[dims[1]], type: typ, t });
      }
    }
    const cv = $('scatterCanvas');
    drawScatter(cv, pts, { dxLabel: this.sc.pca ? 'PC1' : dims[0], dyLabel: this.sc.pca ? 'PC2' : dims[1], pca: this.sc.pca }, this.hoverScatter);
    cv.onmousemove = (ev) => {
      const r = cv.getBoundingClientRect();
      const mx = ev.clientX - r.left, my = ev.clientY - r.top;
      let best = -1, bd = 64;
      if (cv._scMap && cv._scPts) {
        cv._scPts.forEach((p, idx) => {
          const [px, py] = cv._scMap(p);
          const dd = (px - mx) ** 2 + (py - my) ** 2;
          if (dd < bd) { bd = dd; best = idx; }
        });
      }
      if (best !== this.hoverScatter) {
        this.hoverScatter = best;
        drawScatter(cv, cv._scPts, { dxLabel: this.sc.pca ? 'PC1' : dims[0], dyLabel: this.sc.pca ? 'PC2' : dims[1], pca: this.sc.pca }, best);
      }
      if (best >= 0) {
        const p = cv._scPts[best];
        showTipAt(ev, `<span class="tt-t">#${p.t} ${esc(shortTok(this.tok.decode([s.ids[p.t]])))}</span><br>` +
          `${p.type} vec: x=${fmt(p.x, 3)} y=${fmt(p.y, 3)}${this.sc.pca ? ' (PCA)' : ` (dim${dims[0]},${dims[1]})`}`);
      } else hideTip();
    };
    cv.onmouseleave = () => { this.hoverScatter = -1; hideTip(); this.renderScatter(); };
    // head slider max depends on vec type
    $('scHead').max = (this.sc.vec === 'e' || this.sc.vec === 'ho') ? 3 : (this.sc.vec === 'q' ? 3 : 3);
  }

  // ---------- mlp view ----------
  renderMlp() {
    const s = this.s;
    if (!s.capPool || this.mp.tok < 0 || this.mp.tok >= s.ids.length) return;
    const l = this.mp.layer, t = this.mp.tok;
    const gate = s.capPool.layers[l].ln2[t];   // input vector to mlp (post ln2)
    if (!s.viz.enabled || !gate) {
      $('fSwiglu').innerHTML = '<span style="color:var(--amber)">该 token 的捕获已丢弃或捕获已关闭。</span>';
      return;
    }
    // recompute gate/up on demand using backend (small, sync-ish)
    const m = this.m;
    const d = m.dModel, inter = m.intermediate;
    (async () => {
      const gateP = await m._lin(gate, 1, `model.layers.${l}.mlp.gate_proj.weight`, d, inter);
      const upP = await m._lin(gate, 1, `model.layers.${l}.mlp.up_proj.weight`, d, inter);
      const act = new Float32Array(inter);
      for (let i2 = 0; i2 < inter; i2++) act[i2] = (gateP[i2] / (1 + Math.exp(-gateP[i2]))) * upP[i2];
      const contrib = s.capPool.layers[l].mlpContrib[t];
      drawSwiglu($('swigluCanvas'), gateP, upP, act, contrib);
      // formula box: show top-5 channels
      const tops = Array.from({ length: inter }, (_, i3) => i3).sort((a, b) => act[b] - act[a]).slice(0, 5);
      $('fSwiglu').innerHTML =
        `输入 ‖ln2(x)‖=${fmt(norm(gate), 2)}；激活谱前5通道：` +
        tops.map(i3 => `ch${i3}: <span class="num">${fmt(act[i3], 2)}</span>`).join('， ') +
        `；输出贡献范数 ‖mlp‖=<span class="res">${fmt(norm(contrib), 2)}</span>`;
    })();
  }

  // ---------- candidates ----------
  renderCands() {
    const s = this.s;
    const body = $('candBody');
    if (s.logitsLast == null) {
      body.innerHTML = `<tr><td colspan="5" style="color:var(--faint)">等待预填…</td></tr>`;
      $('entVal').textContent = '—'; $('pplVal').textContent = '—'; $('entFill').style.width = '0%';
      drawProbCurve($('probCurve'), null, null, null);
      return;
    }
    const dist = s.distribution(s.logitsLast, s.params);
    const lastRec = s.records[s.records.length - 1];
    body.innerHTML = dist.top.map((t, k) => {
      const isChosen = lastRec && !lastRec.eos ? lastRec.chosen === t.id : false;
      const outCls = t.filteredOut ? 'out' : '';
      return `<tr class="cand-row ${isChosen ? 'chosen' : ''} ${outCls}" data-id="${t.id}">` +
        `<td>${k + 1}</td>` +
        `<td>${esc(shortTok(t.txt))}<span style="color:var(--faint);font-size:9px"> ${t.id}</span></td>` +
        `<td>${pctStr(t.pRaw)}</td>` +
        `<td><div class="cbar post"><i style="width:${(t.pPost * 100).toFixed(1)}%"></i></div><span style="font-size:9.5px;color:var(--dim)">${pctStr(t.pPost)}</span></td>` +
        `<td>${(t.cum * 100).toFixed(2)}%</td></tr>`;
    }).join('');
    for (const tr of body.querySelectorAll('.cand-row')) {
      tr.onclick = async () => {
        const id = +tr.dataset.id;
        this.log('warn', `手动强制选择 token ${id} ${JSON.stringify(shortTok(this.tok.decode([id])))}`);
        await this.stepOnce(id);
      };
    }
    $('entVal').textContent = dist.entropyBits.toFixed(2);
    $('pplVal').textContent = dist.ppl.toFixed(1);
    $('entFill').style.width = `${Math.min(100, dist.entropyBits / 12.6 * 100).toFixed(0)}%`;
    drawProbCurve($('probCurve'), dist.top, dist.pPost, lastRec?.chosen);
    $('sbProb').textContent = dist.top[0] ? `${(dist.top[0].pPost * 100).toFixed(1)}%` : '—';
  }

  // ---------- chat transcript (user ⇄ assistant turns) ----------
  renderGenStream() {
    const gs = $('chatStream');
    const s = this.s;
    if (!s.ids.length || !s.turns.length) {
      gs.innerHTML = `<div class="msg system">尚未开始 —— 在左侧输入并发送，或点击「预填并推理首 Token」</div>`;
      return;
    }
    // records chronologically align with generated tokens across turns
    let recIdx = 0;
    let html = '';
    for (const turn of s.turns) {
      if (turn.role === 'system') {
        html += `<div class="msg system" title="system prompt（模板化后注入上下文最前部）">${esc(turn.text)}</div>`;
      } else if (turn.role === 'user') {
        html += `<div class="msg user"><span class="who">我 · user</span>${esc(turn.text)}</div>`;
      } else if (turn.role === 'assistant') {
        let inner = '';
        for (const id of (turn.tokens || [])) {
          const raw = this.tok.decode([id]);
          const rec = s.records[recIdx++];
          const cls = rec && rec.eos ? 'g-eos' : 'g-tok';
          const tip = rec ? `#${rec.pos} P=${(rec.pPostChosen * 100).toFixed(1)}% T=${rec.temperature}${rec.forced ? ' 手动强制' : ''}` : '';
          inner += `<span class="${cls}" title="${tip}">${esc(raw)}</span>`;
        }
        html += `<div class="msg assistant"><span class="who">模型 · assistant（逐 Token 采样）</span>${inner || '<span style="color:var(--faint)">（等待生成…）</span>'}<span class="caret" style="${s.busy ? '' : 'display:none'}"></span></div>`;
      }
    }
    gs.innerHTML = html;
    gs.parentElement.scrollTop = gs.parentElement.scrollHeight;
  }

  // ---------- flow pulse & status ----------
  flowPulse(nodes) {
    this.flowActive = { nodes, t0: performance.now() };
    drawFlow($('flowCanvas'), this);
    clearTimeout(this._pulseT);
    this._pulseT = setTimeout(() => { this.flowActive = null; drawFlow($('flowCanvas'), this); }, 1200);
  }

  renderStatus() {
    const s = this.s;
    $('sbPos').textContent = s.ids.length;
    $('mToks').textContent = s.ids.length;
    const times = s.records.map(r => r.ms).filter(Boolean);
    const avg = times.length ? times.reduce((a, b) => a + b, 0) / times.length : 0;
    $('mRate').textContent = avg ? (1000 / avg).toFixed(1) : '—';
    $('mCtx').textContent = `${(s.ids.length / s.ctxLimit * 100).toFixed(0)}%`;
    const capMem = s.captureBytes();
    $('mRAM').textContent = s.viz.enabled ? `${(capMem / 1048576).toFixed(1)}MB` : '关闭';
    $('btnStep').disabled = !s.canStep();
    $('btnAuto').disabled = !s.canStep();

    // ---- 内存账单 & 预估（公式见原理文档 §7.4） ----
    const MB = 1048576;
    const d = this.m.dModel, L = this.m.nLayers, KVH = this.m.nKvHeads, hd = this.m.headDim;
    const kvPerTok = 2 * L * KVH * hd * 4;                 // B/token (K+V)
    const kvPre = this.m.maxCtx * kvPerTok;                // 预分配
    const kvAtLimit = s.ctxLimit * kvPerTok;
    const precInt8 = this.m.precision === 'int8';
    const weightsMB = precInt8 ? 63912192 * 1.001 / MB : 63912192 * 4 / MB; // INT8+scales 或 FP32
    const W = s.viz.window === Infinity ? s.ctxLimit : s.viz.window;
    const effW = Math.min(W, s.ctxLimit);
    const fixedPerTok = (3 * d + L * 5 * d) * 4;           // 捕获线性部分 B/token
    const capProj = s.viz.enabled
      ? effW * fixedPerTok + L * this.m.nHeads * (effW * s.ctxLimit - effW * effW / 2) * 4
      : 0;
    const capNow = s.viz.enabled ? capMem : 0;
    $('memLines').innerHTML =
      `权重常驻 <b>${weightsMB.toFixed(0)}MB</b>（${precInt8 ? (this.m.quantNative ? 'INT8 内嵌原生' : 'INT8，FP32 副本仍在 244MB 可随时切回') : 'FP32'}）` +
      `${this.m.backend.kind.includes('webgl') ? ' + GPU≈同量' : ''}` +
      ` · KV Cache <b>${(kvAtLimit / MB).toFixed(1)}MB</b>（@ctx=${s.ctxLimit}，预分配 ${(kvPre / MB).toFixed(0)}MB）<br>` +
      `捕获池现用 <b>${(capNow / MB).toFixed(1)}MB</b>（窗口 ${s.viz.window === Infinity ? '全部' : s.viz.window}）` +
      ` · 满上下文预估 <b>${(capProj / MB).toFixed(0)}MB</b>${s.viz.enabled ? '' : '（捕获已关闭→0）'}`;
    const warn = $('memWarn');
    warn.classList.remove('warn', 'err');
    if (!s.viz.enabled) {
      warn.innerHTML = '✓ 可视化捕获已关闭：内存只剩权重 + KV Cache，可跑满上下文；散点/热力图/演算台将无数据。';
    } else if (capProj > 400 * MB) {
      warn.classList.add('err');
      warn.innerHTML = `⚠ 高内存预警：满上下文捕获预估 ${(capProj / MB).toFixed(0)}MB（>400MB）——建议缩小可视化窗口或关闭捕获。`;
    } else if (capProj > 150 * MB) {
      warn.classList.add('warn');
      warn.innerHTML = `⚠ 内存提示：满上下文捕获预估 ${(capProj / MB).toFixed(0)}MB（>150MB），低配设备建议缩小窗口。`;
    } else {
      warn.innerHTML = `✓ 内存健康：满上下文捕获预估 ${(capProj / MB).toFixed(0)}MB，KV Cache ${(kvAtLimit / MB).toFixed(1)}MB。`;
    }
  }

  drawFlow() { drawFlow($('flowCanvas'), this); }
}

/* ================= helpers ================= */

function promptLen(s) {
  // number of prompt tokens = ids minus generated
  return s.ids.length - s.countGen();
}

function shortTok(t) {
  if (!t) return '';
  return t.replace(/\n/g, '⏎').replace(/\r/g, '\\r').replace(/\t/g, '⇥');
}

function isSpecialText(t) { return t.startsWith('<|') || t.startsWith('<think'); }

function pctStr(p) {
  const v = p * 100;
  return v >= 0.01 ? `${v.toFixed(2)}%` : v > 0 ? `${v.toExponential(1)}%` : '0%';
}

function norm(v) { let s = 0; for (let i = 0; i < v.length; i++) s += v[i] * v[i]; return Math.sqrt(s); }

function activateTab(name) {
  const btn = document.querySelector(`.tabbar button[data-tab="${name}"]`);
  if (btn) btn.click();
}

/* tooltip */
const tipEl = () => document.getElementById('tooltip');
function showTip(ev, html) { const t = tipEl(); t.innerHTML = html; t.style.display = 'block'; t.style.left = `${ev.clientX + 14}px`; t.style.top = `${ev.clientY + 12}px`; }
function showTipAt(ev, html) { if (ev) showTip(ev, html); }
function hideTip() { tipEl().style.display = 'none'; }

/* PCA-2 via power iteration on covariance of vectors */
function computePCA2(vecFor, nT, kind) {
  const vecs = [];
  for (let t = 0; t < nT; t++) { const v = vecFor(t); if (v) vecs.push({ t, v }); }
  if (!vecs.length) return [];
  const d = vecs[0].v.length;
  const mean = new Float64Array(d);
  for (const { v } of vecs) for (let i = 0; i < d; i++) mean[i] += v[i];
  for (let i = 0; i < d; i++) mean[i] /= vecs.length;
  const cov = new Float64Array(d * d);
  for (const { v } of vecs) {
    for (let i = 0; i < d; i++) {
      const a = v[i] - mean[i];
      for (let j = i; j < d; j++) cov[i * d + j] += a * (v[j] - mean[j]);
    }
  }
  for (let i = 0; i < d; i++) for (let j = 0; j < i; j++) cov[i * d + j] = cov[j * d + i];
  const power = () => {
    let v = new Float64Array(d).fill(1);
    for (let it = 0; it < 24; it++) {
      const nv = new Float64Array(d);
      for (let i = 0; i < d; i++) {
        let s = 0;
        for (let j = 0; j < d; j++) s += cov[i * d + j] * v[j];
        nv[i] = s;
      }
      const nn = norm64(nv) || 1;
      for (let i = 0; i < d; i++) nv[i] /= nn;
      v = nv;
    }
    return v;
  };
  const p1 = power();
  const proj1 = vecs.map(({ v }) => dot64(p1, v));
  // deflation
  const cov2 = Float64Array.from(cov);
  for (let i = 0; i < d; i++) for (let j = 0; j < d; j++) cov2[i * d + j] -= p1[i] * p1[j] * dot64(p1, p1);
  const old = cov; /* swap by monkey not possible; inline second power with cov2 */
  const power2 = () => {
    let v = new Float64Array(d).fill(0.5);
    for (let it = 0; it < 24; it++) {
      const nv = new Float64Array(d);
      for (let i = 0; i < d; i++) {
        let s = 0;
        for (let j = 0; j < d; j++) s += cov2[i * d + j] * v[j];
        nv[i] = s;
      }
      const nn = norm64(nv) || 1;
      for (let i = 0; i < d; i++) nv[i] /= nn;
      v = nv;
    }
    return v;
  };
  const p2 = power2();
  const proj2 = vecs.map(({ v }) => dot64(p2, v));
  void old;
  const typ = kind === 'q' ? 'Q' : kind === 'k' ? 'K' : kind === 'v' ? 'V' : 'E';
  return vecs.map(({ t }, i) => ({ x: proj1[i], y: proj2[i], type: typ, t }));
}
function norm64(v) { let s = 0; for (let i = 0; i < v.length; i++) s += v[i] * v[i]; return Math.sqrt(s); }
function dot64(a, b) { let s = 0; for (let i = 0; i < a.length; i++) s += a[i] * b[i]; return s; }

/* ================= SwiGLU view ================= */
function drawSwiglu(cv, gate, up, act, contrib) {
  const { ctx, w, h } = fitCanvas(cv);
  drawGridBg(ctx, w, h);
  const n = Math.min(300, act.length);
  const step = act.length / n;
  const cols = [
    { data: gate, color: 'rgba(53,224,255,.75)', label: 'gate=ln2@W_g' },
    { data: up, color: 'rgba(255,179,71,.75)', label: 'up=ln2@W_u' },
    { data: act, color: 'rgba(82,255,122,.8)', label: 'silu(gate)⊙up' },
  ];
  const secW = (w - 20) / 3;
  cols.forEach((c0, ci) => {
    const ox = 8 + ci * secW;
    let mAbs = 1e-9;
    for (let i = 0; i < act.length; i++) mAbs = Math.max(mAbs, Math.abs(c0.data[i]));
    const bh = h - 34;
    for (let i = 0; i < n; i++) {
      const idx = Math.floor(i * step);
      const v = c0.data[idx] / mAbs;
      const bw = secW / n;
      ctx.fillStyle = c0.color;
      ctx.fillRect(ox + i * bw, v >= 0 ? bh / 2 - v * bh / 2 : bh / 2, Math.max(1, bw - .6), Math.abs(v) * bh / 2);
    }
    ctx.strokeStyle = C.line;
    ctx.beginPath(); ctx.moveTo(ox, bh / 2 + .5); ctx.lineTo(ox + secW - 6, bh / 2 + .5); ctx.stroke();
    ctx.fillStyle = C.dim; ctx.font = '9.5px monospace'; ctx.textAlign = 'left';
    ctx.fillText(c0.label, ox + 2, h - 16);
    ctx.fillText(`max|·|=${fmt(mAbs, 1)}`, ox + 2, h - 5);
  });
  // contribution overlay right panel? keep third column = act; add contrib summary bar
  if (contrib) {
    let nn = 0;
    for (let i = 0; i < contrib.length; i++) nn += contrib[i] * contrib[i];
    ctx.fillStyle = C.green;
    ctx.textAlign = 'right';
    ctx.fillText(`→ @W_down: ‖out‖=${fmt(Math.sqrt(nn), 2)} (加回残差)`, w - 8, 12);
  }
}
