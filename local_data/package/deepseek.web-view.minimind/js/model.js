// model.js — Qwen3ForCausalLM forward pass, faithful to HF `transformers`
// layout, with full intermediate-state capture for visualization.
//
// Pipeline per token:
//   x = embed[id]
//   for each layer l:
//     h1 = RMSNorm(x, input_layernorm)
//     q,h1@Wq ; k,h1@Wk ; v,h1@Wv          (no bias)
//     q/k: per-head RMSNorm(q_norm/k_norm) then RoPE rotation (rotate-half)
//     attention over cached keys (GQA repeat_kv, scale = 1/sqrt(head_dim))
//     concat heads @ Wo  -> attnContribution; x += attnContribution
//     h2 = RMSNorm(x, post_attention_layernorm)
//     mlp = W_down( silu(W_gate h2) * (W_up h2) ) -> mlpContribution; x += mlp
//   logits(t) = RMSNorm(x, model.norm)(t) @ embed_tokens.T   (tied embeddings)

import { linearJS } from './mat.js';
import { quantizeInt8PerRow, dequantInt8, transposeInt8, linearJSInt8 } from './quant.js';

export function rmsNorm_(x, w, off, d, eps, out, outOff) {
  // out[outOff+j] = x[off+j] * w[j] / sqrt(mean_j(x^2)+eps)
  let ss = 0;
  for (let j = 0; j < d; j++) { const v = x[off + j]; ss += v * v; }
  const inv = 1 / Math.sqrt(ss / d + eps);
  for (let j = 0; j < d; j++) out[outOff + j] = x[off + j] * w[j] * inv;
}

export function silu(v) { return v / (1 + Math.exp(-v)); }

/** Build rotary cos/sin tables for positions [0..maxPos); dim must be even. */
export function buildRopeTables(maxPos, dim, theta) {
  const half = dim >> 1;
  const freqs = new Float64Array(half);
  for (let i = 0; i < half; i++) freqs[i] = Math.pow(theta, (-2 * i) / dim);
  const cosT = new Float32Array(maxPos * half);
  const sinT = new Float32Array(maxPos * half);
  for (let p = 0; p < maxPos; p++) {
    for (let i = 0; i < half; i++) {
      const a = p * freqs[i];
      cosT[p * half + i] = Math.cos(a);
      sinT[p * half + i] = Math.sin(a);
    }
  }
  return { cosT, sinT, half };
}

function transposeInto(data, R, C) {
  const out = new Float32Array(R * C);
  for (let r = 0; r < R; r++) {
    const ro = r * C;
    for (let c = 0; c < C; c++) out[c * R + r] = data[ro + c];
  }
  return out;
}

export class Qwen3Model {
  constructor({ weights, config, backend, maxCtx = 384, quantMap = null, log = () => {} }) {
    this.w = weights;
    this.cfg = config;
    this.backend = backend;
    this.maxCtx = maxCtx;
    this.log = log;

    const c = config;
    this.dModel = c.hidden_size;
    this.nLayers = c.num_hidden_layers;
    this.nHeads = c.num_attention_heads;
    this.nKvHeads = c.num_key_value_heads;
    this.headDim = c.head_dim || Math.floor(this.dModel / this.nHeads);
    this.intermediate = c.intermediate_size;
    this.eps = c.rms_norm_eps ?? 1e-6;
    this.theta = c.rope_theta ?? 10000.0;
    this.vocab = c.vocab_size;
    this.scale = 1 / Math.sqrt(this.headDim);
    this.rep = this.nHeads / this.nKvHeads;

    this.rope = buildRopeTables(maxCtx, this.headDim, this.theta);

    this.kCache = [];
    this.vCache = [];
    for (let l = 0; l < this.nLayers; l++) {
      this.kCache.push(new Float32Array(maxCtx * this.nKvHeads * this.headDim));
      this.vCache.push(new Float32Array(maxCtx * this.nKvHeads * this.headDim));
    }
    this.curLen = 0;

    this.quantMap0 = quantMap;   // INT8 原生加载（v2 内嵌包）
    if (this.quantMap0) {
      this.quant = this.quantMap0;
      this.precision = 'int8';
      this.quantNative = true;
      this.hasFp32 = false;
      this._assertShape('model.layers.0.input_layernorm.weight', [this.dModel]);
    } else {
      this._assertShape('model.embed_tokens.weight', [this.vocab, this.dModel]);
      this.precision = 'fp32';
      this.quantNative = false;
      this.hasFp32 = true;
    }
    this.embed = this.quantNative ? null : this.w.get('model.embed_tokens.weight').data;
    this._embT = null;            // lazy transposed embedding for lm head
    this._embTQ = null;           // int8 transposed embedding for lm head
    this.scratch = {};
    this.stepCount = 0;
    this.totalTokens = 0;

    // ---- precision: 'fp32' | 'int8' (W8A16 weight-only quantization) ----
    this.quant = this.quantNative ? this.quant : null;  // Map name -> {q, scales, dIn, dOut}
    this.quantStats = null;
  }

  get hasFp32Weights() { return this.hasFp32; }

  /** 运行时引入 FP32 原始张量（HTTP 模式从 minimind-3/model.safetensors 加载）。 */
  adoptFp32(fp32Tensors) {
    for (const [name, t] of fp32Tensors) this.w.set(name, t);
    this.hasFp32 = true;
    this.precision = 'fp32';
    this._embT = null; this._embTQ = null;
    this.backend.dispose?.();
  }

  /** Quantize all large matrices to per-channel symmetric INT8. */
  quantizeAll() {
    if (this.quant) {
      if (!this.quantStats) this._statsFromMap();
      return this.quantStats;
    }
    const Q = new Map();
    const d = this.dModel, hd = this.headDim;
    const dq = this.nHeads * hd, dkv = this.nKvHeads * hd, inter = this.intermediate;
    const V = this.vocab;
    const shapes = { 'model.embed_tokens.weight': [V, d] };
    for (let l = 0; l < this.nLayers; l++) {
      shapes[`model.layers.${l}.self_attn.q_proj.weight`] = [dq, d];
      shapes[`model.layers.${l}.self_attn.k_proj.weight`] = [dkv, d];
      shapes[`model.layers.${l}.self_attn.v_proj.weight`] = [dkv, d];
      shapes[`model.layers.${l}.self_attn.o_proj.weight`] = [d, dq];
      shapes[`model.layers.${l}.mlp.gate_proj.weight`] = [inter, d];
      shapes[`model.layers.${l}.mlp.up_proj.weight`] = [inter, d];
      shapes[`model.layers.${l}.mlp.down_proj.weight`] = [d, inter];
    }
    let params = 0, int8Bytes = 0, fp32Bytes = 0, maxErr = 0;
    for (const [name, [dOut, dIn]] of Object.entries(shapes)) {
      const data = this.w.get(name).data;
      const r = quantizeInt8PerRow(data, dOut, dIn);
      Q.set(name, { q: r.q, scales: r.scales, dIn, dOut });
      params += dOut * dIn;
      int8Bytes += dOut * dIn + dOut * 4;
      fp32Bytes += dOut * dIn * 4;
      if (r.maxErr > maxErr) maxErr = r.maxErr;
    }
    this.quant = Q;
    this.quantStats = {
      tensors: Q.size, params, int8Bytes, fp32Bytes, maxErr,
      compression: fp32Bytes / int8Bytes,
    };
    return this.quantStats;
  }

  _statsFromMap() {
    let params = 0, int8Bytes = 0;
    for (const e of this.quant.values()) {
      params += e.dIn * e.dOut;
      int8Bytes += e.dIn * e.dOut + e.dOut * 4;
    }
    this.quantStats = {
      tensors: this.quant.size, params, int8Bytes,
      fp32Bytes: params * 4, maxErr: null,
      compression: (params * 4) / int8Bytes,
    };
    return this.quantStats;
  }

  /** Switch compute precision. Backend caches are dropped and rebuilt lazily. */
  setPrecision(mode) {
    if (mode !== 'fp32' && mode !== 'int8') throw new Error(`未知精度 ${mode}`);
    if (mode === 'int8') {
      if (!this.quant && !this.quantNative) this.quantizeAll();
      if (this.quantNative) this._statsFromMap();
    } else if (!this.hasFp32) {
      throw new Error('当前为 INT8 内嵌权重，无 FP32 数据 —— 请先通过 HTTP 模式加载 minimind-3/model.safetensors');
    }
    this.precision = mode;
    this._embT = null; this._embTQ = null;
    this.backend.dispose?.();
    if (mode === 'int8' && !this.quantStats) this._statsFromMap();
    return this.quantStats;
  }

  _assertShape(name, want) {
    const t = this.w.get(name);
    if (!t) throw new Error(`missing tensor ${name}`);
    if (t.shape.join(',') !== want.join(',')) {
      throw new Error(`tensor ${name} shape [${t.shape}] != expected [${want}]`);
    }
  }

  _lay(name) { return this.w.get(name).data; }

  reset() {
    this.curLen = 0;
    for (let l = 0; l < this.nLayers; l++) {
      this.kCache[l].fill(0);
      this.vCache[l].fill(0);
    }
  }

  async _lin(x, T, name, dIn, dOut) {
    if (this.precision === 'int8' && this.quant && this.quant.has(name)) {
      return this.backend.linearQ(x, T, { name, dIn, dOut }, this.quant);
    }
    return this.backend.linear(x, T, { name, dIn, dOut }, this.w, null);
  }

  /** rms-normalize one head then apply RoPE rotate-half; write to dst[dstOff]. */
  _headNormRope(srcBuf, srcOff, normW, pos, dst, dstOff) {
    const hd = this.headDim, half = this.rope.half;
    let ss = 0;
    for (let j = 0; j < hd; j++) { const v = srcBuf[srcOff + j]; ss += v * v; }
    const inv = 1 / Math.sqrt(ss / hd + this.eps);
    const coff = pos * half;
    const { cosT, sinT } = this.rope;
    for (let j = 0; j < half; j++) {
      const a = srcBuf[srcOff + j] * normW[j] * inv;
      const b = srcBuf[srcOff + j + half] * normW[j + half] * inv;
      const cs = cosT[coff + j], sn = sinT[coff + j];
      dst[dstOff + j] = a * cs - b * sn;
      dst[dstOff + j + half] = b * cs + a * sn;
    }
  }

  /**
   * Forward over newIds appended at the current cache position.
   * opts.capture = 'full' | 'none'
   * Returns { logitsLast, capture } where logitsLast covers next position.
   */
  async forward(ids, opts = {}) {
    const captureOn = (opts.capture ?? 'full') === 'full';
    const startPos = this.curLen;
    const T = ids.length;
    if (startPos + T > this.maxCtx) {
      throw new Error(`context overflow: ${startPos}+${T} > maxCtx=${this.maxCtx}`);
    }
    const d = this.dModel, hd = this.headDim;
    const H = this.nHeads, KV = this.nKvHeads, dq = H * hd, dkv = KV * hd;

    // ---- embeddings (INT8 mode: dequantize row on lookup)
    const x = new Float32Array(T * d);
    const eq = (this.precision === 'int8' && this.quant)
      ? this.quant.get('model.embed_tokens.weight') : null;
    for (let t = 0; t < T; t++) {
      const id = ids[t], off = id * d;
      if (eq) {
        const sc = eq.scales[id], q = eq.q;
        for (let j = 0; j < d; j++) x[t * d + j] = q[off + j] * sc;
      } else {
        x.set(this.embed.subarray(off, off + d), t * d);
      }
    }

    const cap = captureOn ? {
      ids: ids.slice(),
      startPos,
      nToks: T,
      embed: x.slice(),
      hiddenAll: null,
      layers: [],
    } : null;

    for (let l = 0; l < this.nLayers; l++) {
      const lc = captureOn ? {
        ln1: new Float32Array(T * d),
        ln2: new Float32Array(T * d),
        q: new Float32Array(T * dq),
        k: new Float32Array(T * dkv),
        v: new Float32Array(T * dkv),
        attnContrib: new Float32Array(T * d),
        mlpContrib: new Float32Array(T * d),
        hiddenAfterLayer: new Float32Array(T * d),
        attnRows: Array.from({ length: H }, () => new Array(T)),
      } : null;

      // ---- RMSNorm #1
      const hn1 = new Float32Array(T * d);
      const wln1 = this._lay(`model.layers.${l}.input_layernorm.weight`);
      for (let t = 0; t < T; t++) rmsNorm_(x, wln1, t * d, d, this.eps, hn1, t * d);

      // ---- QKV projections
      const qRaw = await this._lin(hn1, T, `model.layers.${l}.self_attn.q_proj.weight`, d, dq);
      const kRaw = await this._lin(hn1, T, `model.layers.${l}.self_attn.k_proj.weight`, d, dkv);
      const vRaw = await this._lin(hn1, T, `model.layers.${l}.self_attn.v_proj.weight`, d, dkv);

      // ---- per-head norm + RoPE; write K/V into cache
      const wqn = this._lay(`model.layers.${l}.self_attn.q_norm.weight`);
      const wkn = this._lay(`model.layers.${l}.self_attn.k_norm.weight`);
      const kc = this.kCache[l], vc = this.vCache[l];
      const qRot = new Float32Array(T * dq);
      for (let t = 0; t < T; t++) {
        const pos = startPos + t;
        for (let h = 0; h < H; h++) {
          this._headNormRope(qRaw, t * dq + h * hd, wqn, pos, qRot, t * dq + h * hd);
        }
        for (let g = 0; g < KV; g++) {
          const src = t * dkv + g * hd;
          this._headNormRope(kRaw, src, wkn, pos, kc, pos * dkv + g * hd);
          for (let j = 0; j < hd; j++) vc[pos * dkv + g * hd + j] = vRaw[src + j];
        }
      }
      if (captureOn) {
        lc.q.set(qRot);
        lc.k.set(kc.subarray(startPos * dkv, (startPos + T) * dkv));
        lc.v.set(vc.subarray(startPos * dkv, (startPos + T) * dkv));
      }

      // ---- attention
      const attFlat = new Float32Array(T * dq);
      for (let h = 0; h < H; h++) {
        const gIdx = Math.floor(h / this.rep);
        const kvBase = gIdx * hd;
        const scores = new Float32Array(this.maxCtx);
        for (let t = 0; t < T; t++) {
          const pos = startPos + t;
          const qOff = t * dq + h * hd;
          let maxScore = -Infinity;
          for (let s = 0; s <= pos; s++) {
            const kOff = s * dkv + kvBase;
            let dot = 0;
            for (let j = 0; j < hd; j++) dot += qRot[qOff + j] * kc[kOff + j];
            const sc = dot * this.scale;
            scores[s] = sc;
            if (sc > maxScore) maxScore = sc;
          }
          let denom = 0;
          for (let s = 0; s <= pos; s++) {
            const e = Math.exp(scores[s] - maxScore);
            scores[s] = e; denom += e;
          }
          const outOff = t * dq + h * hd;
          for (let j = 0; j < hd; j++) attFlat[outOff + j] = 0;
          if (captureOn) {
            const row = new Float32Array(pos + 1);
            for (let s = 0; s <= pos; s++) {
              const wt = scores[s] / denom;
              row[s] = wt;
              const vOff = s * dkv + kvBase;
              for (let j = 0; j < hd; j++) attFlat[outOff + j] += wt * vc[vOff + j];
            }
            lc.attnRows[h][pos - startPos] = row;
          } else {
            for (let s = 0; s <= pos; s++) {
              const wt = scores[s] / denom;
              const vOff = s * dkv + kvBase;
              for (let j = 0; j < hd; j++) attFlat[outOff + j] += wt * vc[vOff + j];
            }
          }
        }
      }

      // ---- o_proj + residual
      const oProj = await this._lin(attFlat, T, `model.layers.${l}.self_attn.o_proj.weight`, dq, d);
      for (let i = 0; i < T * d; i++) x[i] += oProj[i];
      if (captureOn) lc.attnContrib.set(oProj);

      // ---- RMSNorm #2 + SwiGLU MLP
      const hn2 = new Float32Array(T * d);
      const wln2 = this._lay(`model.layers.${l}.post_attention_layernorm.weight`);
      for (let t = 0; t < T; t++) rmsNorm_(x, wln2, t * d, d, this.eps, hn2, t * d);
      const gate = await this._lin(hn2, T, `model.layers.${l}.mlp.gate_proj.weight`, d, this.intermediate);
      const up = await this._lin(hn2, T, `model.layers.${l}.mlp.up_proj.weight`, d, this.intermediate);
      for (let i = 0; i < T * this.intermediate; i++) gate[i] = silu(gate[i]) * up[i];
      const down = await this._lin(gate, T, `model.layers.${l}.mlp.down_proj.weight`, this.intermediate, d);
      for (let i = 0; i < T * d; i++) x[i] += down[i];

      if (captureOn) {
        lc.ln1.set(hn1); lc.ln2.set(hn2);
        lc.mlpContrib.set(down);
        lc.hiddenAfterLayer.set(x);
        cap.layers.push(lc);
      }
    }

    // ---- final norm
    const wfin = this._lay('model.norm.weight');
    const fn = new Float32Array(T * d);
    for (let t = 0; t < T; t++) rmsNorm_(x, wfin, t * d, d, this.eps, fn, t * d);

    // ---- tied LM head (last row sufficient; caller may request others later)
    const logitsLast = await this._lmHead(fn.subarray((T - 1) * d, T * d));

    this.curLen += T;
    this.stepCount++;
    this.totalTokens += T;

    if (captureOn) {
      cap.finalNorm = fn;
      cap.hiddenAll = x.slice();
      cap.logitsLastIdx = T - 1;
    }
    return { logitsLast, capture: cap };
  }

  /** logits for a single [d]-dim row against transposed embedding matrix. */
  _lmHead(vec) {
    const V = this.vocab, d = this.dModel;
    const out = new Float32Array(V);
    if (this.precision === 'int8' && this.quant) {
      const eq = this.quant.get('model.embed_tokens.weight');
      if (!this._embTQ) this._embTQ = transposeInt8(eq.q, d, V);
      return Promise.resolve(linearJSInt8(vec, 1, d, this._embTQ, eq.scales, V, out));
    }
    if (!this._embT) this._embT = transposeInto(this.embed, V, d);
    return Promise.resolve(linearJS(vec, 1, d, this._embT, V, out));
  }

  /**
   * Recompute scaled-dot-product score between query token index qi (absolute
   * pos pQ) and key absolute pos pK for layer l/head h, using captured rotated
   * vectors (visualization aid; mirrors the values used inside forward()).
   */
  recomputeDot(l, h, qiArr, kjArr) {
    let dot = 0;
    for (let j = 0; j < this.headDim; j++) dot += qiArr[j] * kjArr[j];
    return { dot, scale: this.scale, score: dot * this.scale };
  }

  /** grab stored rotated key vec for layer l, absolute pos p, kv-group g */
  getKeyVec(l, p, g) {
    const hd = this.headDim;
    return this.kCache[l].subarray(p * this.nKvHeads * hd + g * hd,
                                   p * this.nKvHeads * hd + (g + 1) * hd);
  }
  getValueVec(l, p, g) {
    const hd = this.headDim;
    return this.vCache[l].subarray(p * this.nKvHeads * hd + g * hd,
                                   p * this.nKvHeads * hd + (g + 1) * hd);
  }
}
