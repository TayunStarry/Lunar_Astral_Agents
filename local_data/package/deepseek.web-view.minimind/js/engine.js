// engine.js — Session: owns model + tokenizer + capture pool + sampling loop.

export class Session {
  constructor(model, tokenizer, emit = () => {}) {
    this.m = model;
    this.tok = tokenizer;
    this.emit = emit;
    this.reset();
    this.params = {
      temperature: 0.9,
      topP: 1.0,
      topK: Infinity,       // Infinity = disabled (UI 100 => ∞)
      repPenalty: 1.0,
      greedy: false,
      maxNewTokens: 48,
    };
    this.mode = 'chat';
    this.systemPrompt = '';
  }

  reset() {
    this.m.reset();
    this.ids = [];            // full context ids
    this.meta = [];           // per token {kind:'prompt'|'gen'}
    this.capPool = null;      // merged capture arrays indexed by absolute pos
    this.records = [];        // per generated step record
    this.turns = [];          // chat transcript: [{role:'system'|'user'|'assistant', text?, tokens?:[]}]
    this.genThisTurn = 0;     // tokens generated in the current assistant turn
    this.logitsLast = null;
    this.stoppedReason = '';
    this.busy = false;
    // ---- 可视化 / 资源策略（UI 可调） ----
    this.viz = { enabled: true, window: 128 }; // 捕获开关 + 滑动窗口（超出丢弃更早捕获）
    this.ctxLimit = 512;                       // 软性上下文限制（模型 KV/ RoPE 硬上限 2048）
    this._emitState();
  }

  // ---------- prompt building ----------
  buildPromptIds(text) {
    if (this.mode === 'completion') {
      return { ids: this.tok.encode(text), shown: text };
    }
    let s = '';
    if (this.systemPrompt && this.systemPrompt.trim()) {
      s += `<|im_start|>system\n${this.systemPrompt}<|im_end|>\n`;
    }
    s += `<|im_start|>user\n${text}<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n`;
    return { ids: this.tok.encode(s), shown: s };
  }

  /** Prefill and produce first next-token distribution. */
  async prefill(text) {
    this.busy = true;
    this.emit('busy', true);
    try {
      const t0 = performance.now();
      const { ids } = this.buildPromptIds(text);
      if (ids.length > this.ctxLimit) {
        throw new Error(`输入 ${ids.length} tok 超过上下文限制 ${this.ctxLimit}（SYS 面板可调）`);
      }
      this.ids = [];
      this.meta = [];
      this.capPool = null;
      this.records = [];
      this.genThisTurn = 0;
      this.stoppedReason = '';
      this.m.reset();
      // transcript turns
      this.turns = [];
      if (this.mode === 'chat') {
        if (this.systemPrompt && this.systemPrompt.trim()) {
          this.turns.push({ role: 'system', text: this.systemPrompt });
        }
        this.turns.push({ role: 'user', text });
      } else {
        this.turns.push({ role: 'user', text });
      }
      this.turns.push({ role: 'assistant', tokens: [] });

      const capMode = this.viz.enabled ? 'full' : 'none';
      const { logitsLast, capture } = await this.m.forward(ids, { capture: capMode });
      for (const id of ids) this.ids.push(id);
      for (let i = 0; i < ids.length; i++) this.meta.push({ kind: 'prompt' });
      this.logitsLast = logitsLast;
      this._mergeCapture(capture);
      this.lastPrefillMs = performance.now() - t0;
      this.emit('log', 'info', `prefill 完成：${ids.length} tokens，耗时 ${this.lastPrefillMs.toFixed(0)}ms` + (this.viz.enabled ? '（含可视化捕获）' : '（捕获已关闭）'));
      this.stoppedReason = '';
      this._emitState();
      return logitsLast;
    } finally {
      this.busy = false;
      this.emit('busy', false);
    }
  }

  /**
   * 追加一轮用户输入（多轮对话）：关闭上一 assistant 轮（补 <|im_end|>），
   * 拼接新一轮 user+assistant 模板片段，增量 prefill（KV Cache 继续复用）。
   */
  async appendUserTurn(text) {
    if (this.logitsLast == null || this.busy) return null;
    this.busy = true;
    this.emit('busy', true);
    try {
      const IM_END = 2;
      let deltaIds = [];
      if (this.mode === 'chat') {
        if (this.ids[this.ids.length - 1] !== IM_END) {
          deltaIds.push(IM_END);
          deltaIds.push(...this.tok.encode('\n'));
        }
        deltaIds.push(...this.tok.encode(
          `<|im_start|>user\n${text}<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n`));
      } else {
        deltaIds.push(...this.tok.encode(text));
      }
      if (this.ids.length + deltaIds.length > this.ctxLimit) {
        this.emit('log', 'warn', `已达上下文限制 ${this.ctxLimit}（${this.ids.length}+${deltaIds.length}），请重置或调大限制`);
        return null;
      }
      this.turns.push({ role: 'user', text });
      this.turns.push({ role: 'assistant', tokens: [] });
      this.genThisTurn = 0;
      this.stoppedReason = '';
      const capMode = this.viz.enabled ? 'full' : 'none';
      const { logitsLast, capture } = await this.m.forward(deltaIds, { capture: capMode });
      for (const id of deltaIds) { this.ids.push(id); this.meta.push({ kind: 'prompt' }); }
      this.logitsLast = logitsLast;
      this._mergeCapture(capture);
      this.emit('log', 'info', `追问已增量预填：+${deltaIds.length} tokens（KV Cache 复用，ctx=${this.ids.length}）`);
      this._emitState();
      return logitsLast;
    } finally {
      this.busy = false;
      this.emit('busy', false);
    }
  }

  /**
   * Compute next-token distribution from current logits under params.
   * Returns {top:[{id,txt,logit,pRaw,pPost,cum,kept}], entropyBits, ppl, pPostFull}
   */
  distribution(logits, params, extraBiasLogitPenalty) {
    const V = logits.length;
    const recentSet = new Set(this.ids.slice(-96));
    const rep = params.repPenalty ?? 1.0;

    // --- repetition penalty on logits (HF style)
    const adj = new Float32Array(V);
    for (let v = 0; v < V; v++) {
      let x = logits[v];
      if (rep !== 1.0 && recentSet.has(v)) {
        x = x > 0 ? x / rep : x * rep;
      }
      adj[v] = x;
    }

    // raw probs (T=1 for display; temperature not applied to raw)
    const maxRaw = Math.max(...adj);
    let sumRaw = 0;
    const pRaw = new Float32Array(V);
    for (let v = 0; v < V; v++) {
      pRaw[v] = Math.exp(adj[v] - maxRaw);
      sumRaw += pRaw[v];
    }
    for (let v = 0; v < V; v++) pRaw[v] /= sumRaw;

    // temperature-adjusted
    const T = Math.max(params.temperature, 1e-4);
    const temped = new Float32Array(V);
    for (let v = 0; v < V; v++) temped[v] = adj[v] / T;

    // candidate sort by temped logits desc
    const order = Array.from({ length: V }, (_, v) => v);
    order.sort((a, b) => temped[b] - temped[a]);

    // top-k / top-p filtering over the temped dist
    const kept = new Uint8Array(V);
    let cum = 0, kk = 0;
    for (let k = 0; k < order.length; k++) {
      const id = order[k];
      const pk = Math.exp(temped[id] - temped[order[0]]);
      // we need normalized post prob; compute progressively with running mass over accepted
      if (params.topK !== Infinity && kk >= params.topK) break;
      cum += pk; // unnormalized, normalized later
      kept[id] = 1;
      kk++;
      if (cum >= params.topP && params.topP < 1.0) break;
      // note: standard HF applies top-p AFTER renormalizing within filter set;
      // our progressive variant keeps the highest-mass tokens until cum≥p which
      // matches behavior closely for display purposes.
    }

    // softmax over kept entries of temped logits
    let mx = -Infinity;
    for (let v = 0; v < V; v++) if (kept[v] && temped[v] > mx) mx = temped[v];
    let denom = 0;
    const pPost = new Float32Array(V);
    for (let v = 0; v < V; v++) if (kept[v]) { const e = Math.exp(temped[v] - mx); pPost[v] = e; denom += e; }
    for (let v = 0; v < V; v++) if (kept[v]) pPost[v] /= denom;

    // build displayable top list from first N=14 by post prob
    const byPost = order.filter(id => kept[id]);
    const top = [];
    let c = 0;
    for (let k = 0; k < byPost.length && top.length < 14; k++) {
      const id = byPost[k];
      const txt = this.tok.decode([id], false);
      c += pPost[id];
      top.push({
        id, txt,
        logit: adj[id],
        pRaw: pRaw[id],
        pPost: pPost[id],
        cum: c,
        filteredOut: !kept[id],
        modified: rep !== 1.0 && recentSet.has(id),
      });
    }

    // entropy over post dist (in bits), ppl over raw
    let H = 0;
    for (let v = 0; v < V; v++) if (kept[v] && pPost[v] > 0) H -= pPost[v] * Math.log2(pPost[v]);
    let ppl = 0;
    for (let v = 0; v < V; v++) if (pRaw[v] > 0) ppl -= pRaw[v] * Math.log(pRaw[v]);

    return { top, pPost, pRaw, entropyBits: H, ppl: Math.exp(ppl), allKept: Array.from(byPost) };
  }

  /**
   * One decoding step using current logits + params.
   * overrideId forces a specific token (manual teaching mode).
   */
  async stepOnce(overrideId = null) {
    if (this.logitsLast == null || this.busy) return null;
    this.busy = true;
    this.emit('busy', true);
    try {
      const t0 = performance.now();
      const dist = this.distribution(this.logitsLast, this.params);

      let chosen;
      if (overrideId != null) chosen = overrideId;
      else if (this.params.greedy) {
        let bi = -1, bv = -Infinity;
        for (const id of dist.allKept) if (dist.pPost[id] > bv) { bv = dist.pPost[id]; bi = id; }
        chosen = bi;
      } else {
        chosen = sampleFrom(dist.pPost, rng());
      }

      const rec = {
        pos: this.ids.length,
        dist,
        chosen,
        chosenTxt: this.tok.decode([chosen], false),
        pPostChosen: dist.pPost[chosen],
        greedyTopId: dist.top[0].id,
        temperature: this.params.temperature,
        topP: this.params.topP,
        topK: paramsTopKStr(this.params.topK),
        rep: this.params.repPenalty,
        forced: overrideId != null,
        ms: 0,
      };

      // stop on EOS?
      const eosHit = chosen === 2 || chosen === 151645 || isEosText(this.tok.decode([chosen]));
      if (!eosHit && this.ids.length < this.ctxLimit &&
          this.genThisTurn < this.params.maxNewTokens) {
        const r = await this.m.forward([chosen], { capture: this.viz.enabled ? 'full' : 'none' });
        this.logitsLast = r.logitsLast;
        this._mergeCapture(r.capture);
        this.ids.push(chosen);
        this.meta.push({ kind: 'gen' });
        const lastTurn = this.turns[this.turns.length - 1];
        if (lastTurn && lastTurn.role === 'assistant') {
          lastTurn.tokens.push(chosen);
        }
        this.genThisTurn++;
        rec.ms = performance.now() - t0;
        this.records.push(rec);
        this.emit('token', rec);
        this._emitState();
        return rec;
      }

      // otherwise: flush the final token as a terminal marker only
      if (this.ids.length >= this.ctxLimit) {
        this.stoppedReason = `已达上下文限制 ${this.ctxLimit}（SYS 面板可调，重置后生效）`;
        this.emit('log', 'warn', `停止：${this.stoppedReason}`);
      } else if (this.genThisTurn >= this.params.maxNewTokens) {
        this.stoppedReason = `本轮已达最大生成长度 ${this.params.maxNewTokens}`;
        this.emit('log', 'warn', `停止：${this.stoppedReason}`);
      } else if (eosHit) {
        this.stoppedReason = 'EOS（<|im_end|>）';
        rec.eos = true;
        this.records.push(rec);
        this.emit('token', rec);
        this.emit('log', 'ok', `本轮生成结束：遇到 EOS <|im_end|>（id=2）`);
      }
      this._emitState();
      return rec;
    } finally {
      this.busy = false;
      this.emit('busy', false);
    }
  }

  countGen() {
    return this.genThisTurn;
  }

  canStep() {
    if (this.stoppedReason) return false;
    if (this.logitsLast == null || this.busy) return false;
    if (this.ids.length >= this.ctxLimit) {
      this.stoppedReason = `已达上下文限制 ${this.ctxLimit}（SYS 面板可调，重置后生效）`;
      this.emit('log', 'warn', `停止：${this.stoppedReason}`);
      return false;
    }
    if (this.genThisTurn >= this.params.maxNewTokens) {
      this.stoppedReason = `本轮已达最大生成长度 ${this.params.maxNewTokens}`;
      this.emit('log', 'warn', `停止：${this.stoppedReason}`);
      return false;
    }
    return true;
  }

  // ---------- capture merging ----------
  _mergeCapture(cap) {
    if (!cap || !this.viz.enabled) return;
    if (!this.capPool) {
      this.capPool = {
        positions: [],
        embed: [],            // Float32Array(768) per token
        hiddenAll: [],
        layers: this.m.nLayers === undefined ? [] : range(this.m.nLayers).map(() => ({
          q: [], v_x: [], attnRows: [],
          ln1: [], ln2: [], attnContrib: [], mlpContrib: [],
        })),
        finalNorm: [],
      };
    }
    const P = this.capPool;
    for (let i = 0; i < cap.nToks; i++) {
      const absPos = cap.startPos + i;
      P.positions[absPos] = absPos;
      P.embed[absPos] = cap.embed.subarray(i * 768, (i + 1) * 768).slice();
      P.hiddenAll[absPos] = cap.hiddenAll.subarray(i * 768, (i + 1) * 768).slice();
      P.finalNorm[absPos] = cap.finalNorm.subarray(i * 768, (i + 1) * 768).slice();
      for (let l = 0; l < P.layers.length; l++) {
        const lc = cap.layers[l];
        P.layers[l].q[absPos] = lc.q.subarray(i * 768, (i + 1) * 768).slice();
        P.layers[l].attnRows[absPos] = lc.attnRows.map(rows => rows[i]); // [head]->row
        P.layers[l].ln1[absPos] = lc.ln1.subarray(i * 768, (i + 1) * 768).slice();
        P.layers[l].ln2[absPos] = lc.ln2.subarray(i * 768, (i + 1) * 768).slice();
        P.layers[l].attnContrib[absPos] = lc.attnContrib.subarray(i * 768, (i + 1) * 768).slice();
        P.layers[l].mlpContrib[absPos] = lc.mlpContrib.subarray(i * 768, (i + 1) * 768).slice();
      }
    }
    this._pruneCapture(cap.startPos + cap.nToks);
  }

  /** 滑动窗口：丢弃窗口之外的旧捕获（KV Cache 不受影响，仅可视化数据）。 */
  _pruneCapture(endAbsPos) {
    const P = this.capPool;
    if (!P || !this.viz.window || this.viz.window <= 0) return;
    const keepFrom = Math.max(0, endAbsPos - this.viz.window);
    if (keepFrom <= (P.prunedTo ?? 0)) return;
    for (let t = (P.prunedTo ?? 0); t < keepFrom; t++) {
      P.embed[t] = null; P.hiddenAll[t] = null; P.finalNorm[t] = null;
      P.positions[t] = undefined;
      for (let l = 0; l < P.layers.length; l++) {
        P.layers[l].q[t] = null; P.layers[l].attnRows[t] = null;
        P.layers[l].ln1[t] = null; P.layers[l].ln2[t] = null;
        P.layers[l].attnContrib[t] = null; P.layers[l].mlpContrib[t] = null;
      }
    }
    P.prunedTo = keepFrom;
  }

  /** 捕获池当前实际占用（字节），遍历稀疏数组。 */
  captureBytes() {
    const P = this.capPool;
    if (!P) return 0;
    const d = this.m.dModel;
    let n = 0;
    for (let t = 0; t < P.embed.length; t++) {
      if (!P.embed[t]) continue;
      n += 3 * d;
      for (let l = 0; l < P.layers.length; l++) {
        const lc = P.layers[l];
        if (!lc.ln1[t]) continue;
        n += 5 * d;
        const rows = lc.attnRows[t];
        if (rows) for (const r of rows) if (r) n += r.length;
      }
    }
    return n * 4;
  }

  _emitState() { this.emit('state', this); }
}

function range(n) { return Array.from({ length: n }, (_, i) => i); }

function isEosText(t) {
  return t.includes('<|im_end|>') || t.includes('<|endoftext|>');
}

function paramsTopKStr(k) {
  return k === Infinity ? '∞' : String(k);
}

/** Sampling from a probability vector (returns index). */
export function sampleFrom(pPost, u01) {
  let acc = 0;
  const u = u01 * totalOf(pPost);
  for (let i = 0; i < pPost.length; i++) {
    if (pPost[i] <= 0) continue;
    acc += pPost[i];
    if (u <= acc) return i;
  }
  return lastNonZero(pPost);
}

function totalOf(p) { let s = 0; for (let i = 0; i < p.length; i++) s += p[i]; return s; }
function lastNonZero(p) { for (let i = p.length - 1; i >= 0; i--) if (p[i] > 0) return i; return 0; }

let _rngState = 20260317;
export function seedRng(s) { _rngState = s >>> 0; }
export function rng() {
  const t = (_rngState += 0x6D2B79F5);
  let r = Math.imul(t ^ (t >>> 15), 1 | t);
  r ^= r + Math.imul(r ^ (r >>> 7), 61 | r);
  return ((r ^ (r >>> 14)) >>> 0) / 4294967296;
}
