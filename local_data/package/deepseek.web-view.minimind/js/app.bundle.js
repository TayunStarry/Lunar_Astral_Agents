(() => {
  var __defProp = Object.defineProperty;
  var __getOwnPropNames = Object.getOwnPropertyNames;
  var __esm = (fn, res, err) => function __init() {
    if (err) throw err[0];
    try {
      return fn && (res = (0, fn[__getOwnPropNames(fn)[0]])(fn = 0)), res;
    } catch (e) {
      throw err = [e], e;
    }
  };
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: true });
  };

  // js/st.js
  var st_exports = {};
  __export(st_exports, {
    bf16ToF32: () => bf16ToF32,
    f16ToF32: () => f16ToF32,
    loadSafetensorsUrl: () => loadSafetensorsUrl,
    parseSafetensors: () => parseSafetensors,
    product: () => product
  });
  function f16ToF32(h) {
    const s = (h & 32768) >> 15;
    const e = (h & 31744) >> 10;
    const m = h & 1023;
    let out;
    if (e === 0) {
      out = m / 1024 * Math.pow(2, -14);
    } else if (e === 31) {
      out = m !== 0 ? NaN : Infinity;
    } else {
      out = (1 + m / 1024) * Math.pow(2, e - 15);
    }
    return s ? -out : out;
  }
  function bf16ToF32(h) {
    return h << 16;
  }
  function product(shape) {
    let n = 1;
    for (let i = 0; i < shape.length; i++) n *= shape[i];
    return n;
  }
  function parseSafetensors(buffer) {
    const dv = new DataView(buffer);
    const lo = dv.getUint32(0, true);
    const hi = dv.getUint32(4, true);
    if (hi !== 0) throw new Error("safetensors too large");
    const headerLen = lo;
    const headerBytes = new Uint8Array(buffer, 8, headerLen);
    const headerText = new TextDecoder("utf-8").decode(headerBytes);
    const header = JSON.parse(headerText);
    const meta = header.__metadata__ || null;
    delete header.__metadata__;
    const base = 8 + headerLen;
    const tensors = /* @__PURE__ */ new Map();
    const bytes = new Uint8Array(buffer);
    for (const name of Object.keys(header)) {
      const rec = header[name];
      const [s, e] = rec.data_offsets;
      const numel = rec.shape.length ? rec.shape.reduce((a, b) => a * b, 1) : 1;
      const off = base + s;
      let data;
      switch (rec.dtype) {
        case "F32": {
          data = new Float32Array(numel);
          const src = new Float32Array(buffer, off, numel);
          data.set(src);
          break;
        }
        case "F64": {
          const src = new Float64Array(buffer, off, numel);
          data = new Float32Array(numel);
          for (let i = 0; i < numel; i++) data[i] = src[i];
          break;
        }
        case "F16": {
          data = new Float32Array(numel);
          for (let i = 0; i < numel; i++) {
            data[i] = f16ToF32(dv.getUint16(off + i * 2, true));
          }
          break;
        }
        case "BF16": {
          data = new Float32Array(numel);
          for (let i = 0; i < numel; i++) {
            data[i] = bf16ToF32(dv.getUint16(off + i * 2, true));
          }
          break;
        }
        case "I32":
        case "I16":
        case "I64": {
          data = new Int32Array(numel);
          const step = DT_SIZES[rec.dtype];
          for (let i = 0; i < numel; i++) data[i] = dv.getInt32(off + i * step, true);
          break;
        }
        default:
          throw new Error(`Unsupported dtype ${rec.dtype} for tensor ${name}`);
      }
      tensors.set(name, { dtype: rec.dtype, shape: rec.shape.slice(), data });
    }
    return { tensors, meta };
  }
  async function loadSafetensorsUrl(url, onProgress) {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status} fetching ${url}`);
    const total = Number(res.headers.get("content-length") || 0);
    let buf;
    if (res.body && typeof res.body.pipeThrough === "function" && total > 0) {
      const reader = res.body.getReader();
      const chunks = [];
      let got = 0;
      for (; ; ) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        got += value.byteLength;
        if (onProgress && total) onProgress(got / total, got, total);
      }
      buf = new Uint8Array(got);
      let p = 0;
      for (const c of chunks) {
        buf.set(c, p);
        p += c.byteLength;
      }
      buf = buf.buffer;
    } else {
      buf = await res.arrayBuffer();
      if (onProgress) onProgress(1, buf.byteLength, buf.byteLength);
    }
    return parseSafetensors(buf);
  }
  var DT_SIZES;
  var init_st = __esm({
    "js/st.js"() {
      DT_SIZES = { F64: 8, F32: 4, F16: 2, BF16: 2, I64: 8, I32: 4, I16: 2, I8: 1, U8: 1, BOOL: 1 };
    }
  });

  // js/quant.js
  function quantizeInt8PerRow(data, dOut, dIn) {
    const q = new Int8Array(dOut * dIn);
    const scales = new Float32Array(dOut);
    let maxErr = 0;
    for (let o = 0; o < dOut; o++) {
      const off = o * dIn;
      let maxAbs = 0;
      for (let i = 0; i < dIn; i++) {
        const a = Math.abs(data[off + i]);
        if (a > maxAbs) maxAbs = a;
      }
      const s = maxAbs / 127 || 1e-12;
      scales[o] = s;
      const inv = 1 / s;
      for (let i = 0; i < dIn; i++) {
        let v = Math.round(data[off + i] * inv);
        if (v > 127) v = 127;
        else if (v < -127) v = -127;
        q[off + i] = v;
        const err = Math.abs(v * s - data[off + i]);
        if (err > maxErr) maxErr = err;
      }
    }
    return { q, scales, maxErr };
  }
  function dequantInt8(q, scales, dOut, dIn) {
    const out = new Float32Array(dOut * dIn);
    for (let o = 0; o < dOut; o++) {
      const s = scales[o], off = o * dIn;
      for (let i = 0; i < dIn; i++) out[off + i] = q[off + i] * s;
    }
    return out;
  }
  function transposeInt8(q, dIn, dOut) {
    const t = new Int8Array(dIn * dOut);
    for (let o = 0; o < dOut; o++) {
      const ro = o * dIn;
      for (let i = 0; i < dIn; i++) t[i * dOut + o] = q[ro + i];
    }
    return t;
  }
  function linearJSInt8(x, T, dIn, Wt, scales, dOut, out) {
    if (!out) out = new Float32Array(T * dOut);
    else out.fill(0);
    const acc = new Float64Array(dOut);
    for (let t = 0; t < T; t++) {
      const xr = t * dIn, yr = t * dOut;
      acc.fill(0);
      for (let i = 0; i < dIn; i++) {
        const xi = x[xr + i];
        if (xi === 0) continue;
        const wb = i * dOut;
        for (let o = 0; o < dOut; o++) acc[o] += xi * Wt[wb + o];
      }
      for (let o = 0; o < dOut; o++) out[yr + o] = acc[o] * scales[o];
    }
    return out;
  }
  var init_quant = __esm({
    "js/quant.js"() {
    }
  });

  // js/mat.js
  var mat_exports = {};
  __export(mat_exports, {
    createBackend: () => createBackend,
    linearJS: () => linearJS
  });
  async function createBackend(kind, log = () => {
  }) {
    if (kind === "js") return makeJsBackend(log);
    if (typeof tf !== "undefined") {
      try {
        return await makeTfBackend(log);
      } catch (e) {
        log(`TF.js backend unavailable (${e.message}); falling back to pure-JS kernels.`);
        return makeJsBackend(log);
      }
    }
    return makeJsBackend(log);
  }
  function transposed(wData, dIn, dOut) {
    const wt = new Float32Array(dIn * dOut);
    for (let o = 0; o < dOut; o++) {
      const rowOff = o * dIn;
      for (let i = 0; i < dIn; i++) {
        wt[i * dOut + o] = wData[rowOff + i];
      }
    }
    return wt;
  }
  function linearJS(x, T, dIn, Wt, dOut, out) {
    if (!out) out = new Float32Array(T * dOut);
    else out.fill(0);
    for (let t = 0; t < T; t++) {
      const xr = t * dIn;
      const yr = t * dOut;
      for (let i = 0; i < dIn; i++) {
        const xi = x[xr + i];
        if (xi === 0) continue;
        const wb = i * dOut;
        let o = 0;
        for (; o <= dOut - 8; o += 8) {
          out[yr + o] += xi * Wt[wb + o];
          out[yr + o + 1] += xi * Wt[wb + o + 1];
          out[yr + o + 2] += xi * Wt[wb + o + 2];
          out[yr + o + 3] += xi * Wt[wb + o + 3];
          out[yr + o + 4] += xi * Wt[wb + o + 4];
          out[yr + o + 5] += xi * Wt[wb + o + 5];
          out[yr + o + 6] += xi * Wt[wb + o + 6];
          out[yr + o + 7] += xi * Wt[wb + o + 7];
        }
        for (; o < dOut; o++) out[yr + o] += xi * Wt[wb + o];
      }
    }
    return out;
  }
  function makeJsBackend(log) {
    const cache = /* @__PURE__ */ new Map();
    const qcache = /* @__PURE__ */ new Map();
    return {
      kind: "js",
      async warmup(weights, layerShapes) {
        log("backend=js ready (pure typed-array kernels)");
      },
      async linear(x, T, spec, weights, reuseBuf) {
        let c = cache.get(spec.name);
        if (!c) {
          const w = weights.get(spec.name);
          c = { wt: transposed(w.data, spec.dIn, spec.dOut), dIn: spec.dIn, dOut: spec.dOut };
          cache.set(spec.name, c);
        }
        return linearJS(x, T, c.dIn, c.wt, c.dOut, reuseBuf);
      },
      async linearQ(x, T, spec, quantMap) {
        let c = qcache.get(spec.name);
        if (!c) {
          const e = quantMap.get(spec.name);
          c = { wt: transposeInt8(e.q, spec.dIn, spec.dOut), scales: e.scales, dIn: spec.dIn, dOut: spec.dOut };
          qcache.set(spec.name, c);
        }
        return linearJSInt8(x, T, c.dIn, c.wt, c.scales, c.dOut);
      },
      dispose() {
        cache.clear();
        qcache.clear();
      }
    };
  }
  async function makeTfBackend(log) {
    if (typeof tf === "undefined") throw new Error("tfjs not loaded");
    try {
      await tf.setBackend("webgl");
    } catch (_) {
    }
    await tf.ready();
    const be = tf.getBackend();
    const tensors = /* @__PURE__ */ new Map();
    const qtensors = /* @__PURE__ */ new Map();
    return {
      kind: `tf:${be}`,
      async warmup() {
        log(`backend=${this.kind} webgl=${be}`);
      },
      async linearQ(x, T, spec, quantMap) {
        let wt = qtensors.get(spec.name);
        if (!wt) {
          const e = quantMap.get(spec.name);
          const wq = dequantInt8(e.q, e.scales, spec.dOut, spec.dIn);
          wt = tf.tensor2d(wq, [spec.dOut, spec.dIn]).transpose();
          qtensors.set(spec.name, wt);
        }
        const xt = tf.tensor2d(x, [T, spec.dIn]);
        const yt = tf.matMul(xt, wt);
        const arr = await yt.data();
        xt.dispose();
        yt.dispose();
        return arr instanceof Float32Array ? arr : new Float32Array(arr);
      },
      async linear(x, T, spec, weights, reuseBuf) {
        let wt = tensors.get(spec.name);
        if (!wt) {
          const w = weights.get(spec.name);
          wt = tf.tensor2d(w.data, [spec.dOut, spec.dIn]).transpose();
          tensors.set(spec.name, wt);
        }
        const xt = tf.tensor2d(x, [T, spec.dIn]);
        const yt = tf.matMul(xt, wt);
        const arr = await yt.data();
        xt.dispose();
        yt.dispose();
        return arr instanceof Float32Array ? arr : new Float32Array(arr);
      },
      dispose() {
        for (const t of tensors.values()) t.dispose();
        for (const t of qtensors.values()) t.dispose();
        tensors.clear();
        qtensors.clear();
      }
    };
  }
  var init_mat = __esm({
    "js/mat.js"() {
      init_quant();
    }
  });

  // js/main.js
  init_st();

  // js/tok.js
  var BYTE_ENCODER = (() => {
    const bs = [];
    for (let b = 33; b < 127; b++) bs.push(b);
    for (let b = 161; b < 173; b++) bs.push(b);
    for (let b = 174; b < 256; b++) bs.push(b);
    const cs = bs.slice();
    let n = 0;
    for (let b = 0; b < 256; b++) {
      if (!bs.includes(b)) {
        bs.push(b);
        cs.push(256 + n);
        n++;
      }
    }
    const enc = new Array(256);
    for (let i = 0; i < 256; i++) enc[bs[i]] = String.fromCharCode(cs[i]);
    return enc;
  })();
  var BYTE_DECODER = (() => {
    const m = new Array(65536);
    for (let b = 0; b < 256; b++) m[BYTE_ENCODER[b].charCodeAt(0)] = b;
    return m;
  })();
  var encoder_ = new TextEncoder();
  var decoder_ = new TextDecoder("utf-8", { fatal: false });
  var LETTER_RE = /^\p{L}$/u;
  var DIGIT_RE = /^\p{N}$/u;
  var WS_RE = /^\s$/u;
  var BPETokenizer = class _BPETokenizer {
    constructor(tokenizerJson) {
      const tj = typeof tokenizerJson === "string" ? JSON.parse(tokenizerJson) : tokenizerJson;
      this.vocabSize = Object.keys(tj.model.vocab).length;
      this.tokToId = /* @__PURE__ */ new Map();
      for (const [t, id] of Object.entries(tj.model.vocab)) this.tokToId.set(t, id);
      this.idToTok = new Array(this.vocabSize);
      for (const [t, id] of this.tokToId) this.idToTok[id] = t;
      this.ranks = /* @__PURE__ */ new Map();
      tj.model.merges.forEach((m, i) => {
        const parts = Array.isArray(m) ? m : m.split(" ");
        this.ranks.set(parts[0] + "\0" + parts[1], i);
      });
      this.special = [];
      this.specialById = /* @__PURE__ */ new Map();
      for (const at of tj.added_tokens || []) {
        this.special.push({ id: at.id, text: at.content });
        this.specialById.set(at.id, at.content);
      }
      this.special.sort((a, b) => b.text.length - a.text.length);
      this.specialTextId = new Map(this.special.map((s) => [s.text, s.id]));
    }
    static async load(url) {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`HTTP ${res.status} fetching ${url}`);
      return new _BPETokenizer(await res.json());
    }
    /**
     * Split text into pretokens, faithfully replicating the Rust ByteLevel regex
     *   's|'t|'re|'ve|'m|'ll|'d | ?\p{L}+ | ?\p{N}+ | ?[^\s\p{L}\p{N}]+ |\s+(?!\S)|\s+
     * (case-sensitive contractions, optional single literal space, backtracking
     *  \s+(?!\S) which strips the final whitespace of an interior run).
     */
    pretokenize(text) {
      const arr = Array.from(text);
      const segs = [];
      const cplen = arr.length;
      const isLet = (c) => LETTER_RE.test(c);
      const isDig = (c) => DIGIT_RE.test(c);
      const isWs = (c) => WS_RE.test(c);
      const isPunct = (c) => !isWs(c) && !isLet(c) && !isDig(c);
      let p = 0;
      while (p < cplen) {
        const start = p;
        let consumed = false;
        if (arr[p] === "'" && p + 1 < cplen && !isWs(arr[p + 1])) {
          const three = arr.slice(p + 1, p + 4).join("");
          for (const suf of ["s", "t", "re", "ve", "m", "ll", "d"]) {
            if (three.startsWith(suf)) {
              p += suf.length + 1;
              consumed = true;
              break;
            }
          }
        }
        if (!consumed) {
          for (const cls of [isLet, isDig, isPunct]) {
            let q = p;
            if (arr[q] === " ") q++;
            if (q < cplen && cls(arr[q])) {
              while (q < cplen && cls(arr[q])) q++;
              p = q;
              consumed = true;
              break;
            }
          }
        }
        if (!consumed) {
          let r = p;
          while (r < cplen && isWs(arr[r])) r++;
          if (r > p) {
            let L = r - p;
            if (r === cplen) {
              p = r;
              segs.push([start, p]);
              continue;
            }
            while (L >= 1 && !isWs(arr[p + L])) L--;
            if (L >= 1) {
              p += L;
              segs.push([start, p]);
              continue;
            }
          }
        }
        if (!consumed) {
          let r = p;
          while (r < cplen && isWs(arr[r])) r++;
          p = Math.max(r, p + 1);
        }
        segs.push([start, Math.max(p, start + 1)]);
      }
      return segs.map(([a, b]) => arr.slice(a, b).join(""));
    }
    /** GPT-2 style BPE over one pretoken (already byte-mapped). */
    bpe(word) {
      if (word.length <= 1) return [word];
      let syms = Array.from(word);
      for (; ; ) {
        let bestRank = Infinity, bestI = -1;
        for (let i = 0; i < syms.length - 1; i++) {
          const r = this.ranks.get(syms[i] + "\0" + syms[i + 1]);
          if (r !== void 0 && r < bestRank) {
            bestRank = r;
            bestI = i;
          }
        }
        if (bestI < 0) break;
        const merged = syms[bestI] + syms[bestI + 1];
        syms.splice(bestI, 2, merged);
      }
      return syms;
    }
    encode(text) {
      const outIds = [];
      let i = 0;
      outer: while (i < text.length) {
        for (const sp of this.special) {
          if (sp.text.length && text.startsWith(sp.text, i)) {
            outIds.push(sp.id);
            i += sp.text.length;
            continue outer;
          }
        }
        let j = i + 1;
        inner: while (j < text.length) {
          for (const sp of this.special) {
            if (sp.text.length && text.startsWith(sp.text, j)) break inner;
          }
          j++;
        }
        this._encodeSegment(text.slice(i, j), outIds);
        i = j;
      }
      return outIds;
    }
    _encodeSegment(seg, outIds) {
      if (!seg) return;
      const enc = new TextEncoder();
      for (const piece of this.pretokenize(seg)) {
        const bytes = encoder_.encode(piece);
        let mapped = "";
        for (let i = 0; i < bytes.length; i++) mapped += BYTE_ENCODER[bytes[i]];
        for (const sym of this.bpe(mapped)) {
          const id = this.tokToId.get(sym);
          if (id !== void 0) outIds.push(id);
          else {
            for (const ch of sym) {
              const id2 = this.tokToId.get(ch);
              if (id2 !== void 0) outIds.push(id2);
            }
          }
        }
      }
    }
    /** Decode ids -> text (byte-level round trip; specials render literally). */
    decode(ids, skipSpecial = false) {
      const buf = [];
      for (const id of ids) {
        const spText = this.specialById.get(id);
        if (spText !== void 0) {
          if (!skipSpecial) buf.push(spText);
          continue;
        }
        const tok = this.idToTok[id];
        if (tok === void 0) continue;
        for (let k = 0; k < tok.length; k++) {
          const b = BYTE_DECODER[tok.charCodeAt(k)];
          if (b !== void 0) buf.push(b);
        }
      }
      return decoder_.decode(new Uint8Array(buf));
    }
    pieceRepr(id) {
      const t = this.decode([id]);
      return t === "" ? "" : t;
    }
  };

  // js/model.js
  init_mat();
  init_quant();
  function rmsNorm_(x, w, off, d, eps, out, outOff) {
    let ss = 0;
    for (let j = 0; j < d; j++) {
      const v = x[off + j];
      ss += v * v;
    }
    const inv = 1 / Math.sqrt(ss / d + eps);
    for (let j = 0; j < d; j++) out[outOff + j] = x[off + j] * w[j] * inv;
  }
  function silu(v) {
    return v / (1 + Math.exp(-v));
  }
  function buildRopeTables(maxPos, dim, theta) {
    const half = dim >> 1;
    const freqs = new Float64Array(half);
    for (let i = 0; i < half; i++) freqs[i] = Math.pow(theta, -2 * i / dim);
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
  function transposeInto(data, R, C2) {
    const out = new Float32Array(R * C2);
    for (let r = 0; r < R; r++) {
      const ro = r * C2;
      for (let c = 0; c < C2; c++) out[c * R + r] = data[ro + c];
    }
    return out;
  }
  var Qwen3Model = class {
    constructor({ weights, config, backend, maxCtx = 384, quantMap = null, log = () => {
    } }) {
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
      this.theta = c.rope_theta ?? 1e4;
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
      this.quantMap0 = quantMap;
      if (this.quantMap0) {
        this.quant = this.quantMap0;
        this.precision = "int8";
        this.quantNative = true;
        this.hasFp32 = false;
        this._assertShape("model.layers.0.input_layernorm.weight", [this.dModel]);
      } else {
        this._assertShape("model.embed_tokens.weight", [this.vocab, this.dModel]);
        this.precision = "fp32";
        this.quantNative = false;
        this.hasFp32 = true;
      }
      this.embed = this.quantNative ? null : this.w.get("model.embed_tokens.weight").data;
      this._embT = null;
      this._embTQ = null;
      this.scratch = {};
      this.stepCount = 0;
      this.totalTokens = 0;
      this.quant = this.quantNative ? this.quant : null;
      this.quantStats = null;
    }
    get hasFp32Weights() {
      return this.hasFp32;
    }
    /** 运行时引入 FP32 原始张量（HTTP 模式从 minimind-3/model.safetensors 加载）。 */
    adoptFp32(fp32Tensors) {
      for (const [name, t] of fp32Tensors) this.w.set(name, t);
      this.hasFp32 = true;
      this.precision = "fp32";
      this._embT = null;
      this._embTQ = null;
      this.backend.dispose?.();
    }
    /** Quantize all large matrices to per-channel symmetric INT8. */
    quantizeAll() {
      if (this.quant) {
        if (!this.quantStats) this._statsFromMap();
        return this.quantStats;
      }
      const Q = /* @__PURE__ */ new Map();
      const d = this.dModel, hd = this.headDim;
      const dq = this.nHeads * hd, dkv = this.nKvHeads * hd, inter = this.intermediate;
      const V = this.vocab;
      const shapes = { "model.embed_tokens.weight": [V, d] };
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
        tensors: Q.size,
        params,
        int8Bytes,
        fp32Bytes,
        maxErr,
        compression: fp32Bytes / int8Bytes
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
        tensors: this.quant.size,
        params,
        int8Bytes,
        fp32Bytes: params * 4,
        maxErr: null,
        compression: params * 4 / int8Bytes
      };
      return this.quantStats;
    }
    /** Switch compute precision. Backend caches are dropped and rebuilt lazily. */
    setPrecision(mode) {
      if (mode !== "fp32" && mode !== "int8") throw new Error(`\u672A\u77E5\u7CBE\u5EA6 ${mode}`);
      if (mode === "int8") {
        if (!this.quant && !this.quantNative) this.quantizeAll();
        if (this.quantNative) this._statsFromMap();
      } else if (!this.hasFp32) {
        throw new Error("\u5F53\u524D\u4E3A INT8 \u5185\u5D4C\u6743\u91CD\uFF0C\u65E0 FP32 \u6570\u636E \u2014\u2014 \u8BF7\u5148\u901A\u8FC7 HTTP \u6A21\u5F0F\u52A0\u8F7D minimind-3/model.safetensors");
      }
      this.precision = mode;
      this._embT = null;
      this._embTQ = null;
      this.backend.dispose?.();
      if (mode === "int8" && !this.quantStats) this._statsFromMap();
      return this.quantStats;
    }
    _assertShape(name, want) {
      const t = this.w.get(name);
      if (!t) throw new Error(`missing tensor ${name}`);
      if (t.shape.join(",") !== want.join(",")) {
        throw new Error(`tensor ${name} shape [${t.shape}] != expected [${want}]`);
      }
    }
    _lay(name) {
      return this.w.get(name).data;
    }
    reset() {
      this.curLen = 0;
      for (let l = 0; l < this.nLayers; l++) {
        this.kCache[l].fill(0);
        this.vCache[l].fill(0);
      }
    }
    async _lin(x, T, name, dIn, dOut) {
      if (this.precision === "int8" && this.quant && this.quant.has(name)) {
        return this.backend.linearQ(x, T, { name, dIn, dOut }, this.quant);
      }
      return this.backend.linear(x, T, { name, dIn, dOut }, this.w, null);
    }
    /** rms-normalize one head then apply RoPE rotate-half; write to dst[dstOff]. */
    _headNormRope(srcBuf, srcOff, normW, pos, dst, dstOff) {
      const hd = this.headDim, half = this.rope.half;
      let ss = 0;
      for (let j = 0; j < hd; j++) {
        const v = srcBuf[srcOff + j];
        ss += v * v;
      }
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
      const captureOn = (opts.capture ?? "full") === "full";
      const startPos = this.curLen;
      const T = ids.length;
      if (startPos + T > this.maxCtx) {
        throw new Error(`context overflow: ${startPos}+${T} > maxCtx=${this.maxCtx}`);
      }
      const d = this.dModel, hd = this.headDim;
      const H = this.nHeads, KV = this.nKvHeads, dq = H * hd, dkv = KV * hd;
      const x = new Float32Array(T * d);
      const eq = this.precision === "int8" && this.quant ? this.quant.get("model.embed_tokens.weight") : null;
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
        layers: []
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
          attnRows: Array.from({ length: H }, () => new Array(T))
        } : null;
        const hn1 = new Float32Array(T * d);
        const wln1 = this._lay(`model.layers.${l}.input_layernorm.weight`);
        for (let t = 0; t < T; t++) rmsNorm_(x, wln1, t * d, d, this.eps, hn1, t * d);
        const qRaw = await this._lin(hn1, T, `model.layers.${l}.self_attn.q_proj.weight`, d, dq);
        const kRaw = await this._lin(hn1, T, `model.layers.${l}.self_attn.k_proj.weight`, d, dkv);
        const vRaw = await this._lin(hn1, T, `model.layers.${l}.self_attn.v_proj.weight`, d, dkv);
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
              scores[s] = e;
              denom += e;
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
        const oProj = await this._lin(attFlat, T, `model.layers.${l}.self_attn.o_proj.weight`, dq, d);
        for (let i = 0; i < T * d; i++) x[i] += oProj[i];
        if (captureOn) lc.attnContrib.set(oProj);
        const hn2 = new Float32Array(T * d);
        const wln2 = this._lay(`model.layers.${l}.post_attention_layernorm.weight`);
        for (let t = 0; t < T; t++) rmsNorm_(x, wln2, t * d, d, this.eps, hn2, t * d);
        const gate = await this._lin(hn2, T, `model.layers.${l}.mlp.gate_proj.weight`, d, this.intermediate);
        const up = await this._lin(hn2, T, `model.layers.${l}.mlp.up_proj.weight`, d, this.intermediate);
        for (let i = 0; i < T * this.intermediate; i++) gate[i] = silu(gate[i]) * up[i];
        const down = await this._lin(gate, T, `model.layers.${l}.mlp.down_proj.weight`, this.intermediate, d);
        for (let i = 0; i < T * d; i++) x[i] += down[i];
        if (captureOn) {
          lc.ln1.set(hn1);
          lc.ln2.set(hn2);
          lc.mlpContrib.set(down);
          lc.hiddenAfterLayer.set(x);
          cap.layers.push(lc);
        }
      }
      const wfin = this._lay("model.norm.weight");
      const fn = new Float32Array(T * d);
      for (let t = 0; t < T; t++) rmsNorm_(x, wfin, t * d, d, this.eps, fn, t * d);
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
      if (this.precision === "int8" && this.quant) {
        const eq = this.quant.get("model.embed_tokens.weight");
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
      return this.kCache[l].subarray(
        p * this.nKvHeads * hd + g * hd,
        p * this.nKvHeads * hd + (g + 1) * hd
      );
    }
    getValueVec(l, p, g) {
      const hd = this.headDim;
      return this.vCache[l].subarray(
        p * this.nKvHeads * hd + g * hd,
        p * this.nKvHeads * hd + (g + 1) * hd
      );
    }
  };

  // js/main.js
  init_mat();

  // js/engine.js
  var Session = class {
    constructor(model, tokenizer, emit = () => {
    }) {
      this.m = model;
      this.tok = tokenizer;
      this.emit = emit;
      this.reset();
      this.params = {
        temperature: 0.9,
        topP: 1,
        topK: Infinity,
        // Infinity = disabled (UI 100 => ∞)
        repPenalty: 1,
        greedy: false,
        maxNewTokens: 48
      };
      this.mode = "chat";
      this.systemPrompt = "";
    }
    reset() {
      this.m.reset();
      this.ids = [];
      this.meta = [];
      this.capPool = null;
      this.records = [];
      this.turns = [];
      this.genThisTurn = 0;
      this.logitsLast = null;
      this.stoppedReason = "";
      this.busy = false;
      this.viz = { enabled: true, window: 128 };
      this.ctxLimit = 512;
      this._emitState();
    }
    // ---------- prompt building ----------
    buildPromptIds(text) {
      if (this.mode === "completion") {
        return { ids: this.tok.encode(text), shown: text };
      }
      let s = "";
      if (this.systemPrompt && this.systemPrompt.trim()) {
        s += `<|im_start|>system
${this.systemPrompt}<|im_end|>
`;
      }
      s += `<|im_start|>user
${text}<|im_end|>
<|im_start|>assistant
<think>

</think>

`;
      return { ids: this.tok.encode(s), shown: s };
    }
    /** Prefill and produce first next-token distribution. */
    async prefill(text) {
      this.busy = true;
      this.emit("busy", true);
      try {
        const t0 = performance.now();
        const { ids } = this.buildPromptIds(text);
        if (ids.length > this.ctxLimit) {
          throw new Error(`\u8F93\u5165 ${ids.length} tok \u8D85\u8FC7\u4E0A\u4E0B\u6587\u9650\u5236 ${this.ctxLimit}\uFF08SYS \u9762\u677F\u53EF\u8C03\uFF09`);
        }
        this.ids = [];
        this.meta = [];
        this.capPool = null;
        this.records = [];
        this.genThisTurn = 0;
        this.stoppedReason = "";
        this.m.reset();
        this.turns = [];
        if (this.mode === "chat") {
          if (this.systemPrompt && this.systemPrompt.trim()) {
            this.turns.push({ role: "system", text: this.systemPrompt });
          }
          this.turns.push({ role: "user", text });
        } else {
          this.turns.push({ role: "user", text });
        }
        this.turns.push({ role: "assistant", tokens: [] });
        const capMode = this.viz.enabled ? "full" : "none";
        const { logitsLast, capture } = await this.m.forward(ids, { capture: capMode });
        for (const id of ids) this.ids.push(id);
        for (let i = 0; i < ids.length; i++) this.meta.push({ kind: "prompt" });
        this.logitsLast = logitsLast;
        this._mergeCapture(capture);
        this.lastPrefillMs = performance.now() - t0;
        this.emit("log", "info", `prefill \u5B8C\u6210\uFF1A${ids.length} tokens\uFF0C\u8017\u65F6 ${this.lastPrefillMs.toFixed(0)}ms` + (this.viz.enabled ? "\uFF08\u542B\u53EF\u89C6\u5316\u6355\u83B7\uFF09" : "\uFF08\u6355\u83B7\u5DF2\u5173\u95ED\uFF09"));
        this.stoppedReason = "";
        this._emitState();
        return logitsLast;
      } finally {
        this.busy = false;
        this.emit("busy", false);
      }
    }
    /**
     * 追加一轮用户输入（多轮对话）：关闭上一 assistant 轮（补 <|im_end|>），
     * 拼接新一轮 user+assistant 模板片段，增量 prefill（KV Cache 继续复用）。
     */
    async appendUserTurn(text) {
      if (this.logitsLast == null || this.busy) return null;
      this.busy = true;
      this.emit("busy", true);
      try {
        const IM_END = 2;
        let deltaIds = [];
        if (this.mode === "chat") {
          if (this.ids[this.ids.length - 1] !== IM_END) {
            deltaIds.push(IM_END);
            deltaIds.push(...this.tok.encode("\n"));
          }
          deltaIds.push(...this.tok.encode(
            `<|im_start|>user
${text}<|im_end|>
<|im_start|>assistant
<think>

</think>

`
          ));
        } else {
          deltaIds.push(...this.tok.encode(text));
        }
        if (this.ids.length + deltaIds.length > this.ctxLimit) {
          this.emit("log", "warn", `\u5DF2\u8FBE\u4E0A\u4E0B\u6587\u9650\u5236 ${this.ctxLimit}\uFF08${this.ids.length}+${deltaIds.length}\uFF09\uFF0C\u8BF7\u91CD\u7F6E\u6216\u8C03\u5927\u9650\u5236`);
          return null;
        }
        this.turns.push({ role: "user", text });
        this.turns.push({ role: "assistant", tokens: [] });
        this.genThisTurn = 0;
        this.stoppedReason = "";
        const capMode = this.viz.enabled ? "full" : "none";
        const { logitsLast, capture } = await this.m.forward(deltaIds, { capture: capMode });
        for (const id of deltaIds) {
          this.ids.push(id);
          this.meta.push({ kind: "prompt" });
        }
        this.logitsLast = logitsLast;
        this._mergeCapture(capture);
        this.emit("log", "info", `\u8FFD\u95EE\u5DF2\u589E\u91CF\u9884\u586B\uFF1A+${deltaIds.length} tokens\uFF08KV Cache \u590D\u7528\uFF0Cctx=${this.ids.length}\uFF09`);
        this._emitState();
        return logitsLast;
      } finally {
        this.busy = false;
        this.emit("busy", false);
      }
    }
    /**
     * Compute next-token distribution from current logits under params.
     * Returns {top:[{id,txt,logit,pRaw,pPost,cum,kept}], entropyBits, ppl, pPostFull}
     */
    distribution(logits, params, extraBiasLogitPenalty) {
      const V = logits.length;
      const recentSet = new Set(this.ids.slice(-96));
      const rep = params.repPenalty ?? 1;
      const adj = new Float32Array(V);
      for (let v = 0; v < V; v++) {
        let x = logits[v];
        if (rep !== 1 && recentSet.has(v)) {
          x = x > 0 ? x / rep : x * rep;
        }
        adj[v] = x;
      }
      const maxRaw = Math.max(...adj);
      let sumRaw = 0;
      const pRaw = new Float32Array(V);
      for (let v = 0; v < V; v++) {
        pRaw[v] = Math.exp(adj[v] - maxRaw);
        sumRaw += pRaw[v];
      }
      for (let v = 0; v < V; v++) pRaw[v] /= sumRaw;
      const T = Math.max(params.temperature, 1e-4);
      const temped = new Float32Array(V);
      for (let v = 0; v < V; v++) temped[v] = adj[v] / T;
      const order = Array.from({ length: V }, (_, v) => v);
      order.sort((a, b) => temped[b] - temped[a]);
      const kept = new Uint8Array(V);
      let cum = 0, kk = 0;
      for (let k = 0; k < order.length; k++) {
        const id = order[k];
        const pk = Math.exp(temped[id] - temped[order[0]]);
        if (params.topK !== Infinity && kk >= params.topK) break;
        cum += pk;
        kept[id] = 1;
        kk++;
        if (cum >= params.topP && params.topP < 1) break;
      }
      let mx = -Infinity;
      for (let v = 0; v < V; v++) if (kept[v] && temped[v] > mx) mx = temped[v];
      let denom = 0;
      const pPost = new Float32Array(V);
      for (let v = 0; v < V; v++) if (kept[v]) {
        const e = Math.exp(temped[v] - mx);
        pPost[v] = e;
        denom += e;
      }
      for (let v = 0; v < V; v++) if (kept[v]) pPost[v] /= denom;
      const byPost = order.filter((id) => kept[id]);
      const top = [];
      let c = 0;
      for (let k = 0; k < byPost.length && top.length < 14; k++) {
        const id = byPost[k];
        const txt = this.tok.decode([id], false);
        c += pPost[id];
        top.push({
          id,
          txt,
          logit: adj[id],
          pRaw: pRaw[id],
          pPost: pPost[id],
          cum: c,
          filteredOut: !kept[id],
          modified: rep !== 1 && recentSet.has(id)
        });
      }
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
      this.emit("busy", true);
      try {
        const t0 = performance.now();
        const dist = this.distribution(this.logitsLast, this.params);
        let chosen;
        if (overrideId != null) chosen = overrideId;
        else if (this.params.greedy) {
          let bi = -1, bv = -Infinity;
          for (const id of dist.allKept) if (dist.pPost[id] > bv) {
            bv = dist.pPost[id];
            bi = id;
          }
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
          ms: 0
        };
        const eosHit = chosen === 2 || chosen === 151645 || isEosText(this.tok.decode([chosen]));
        if (!eosHit && this.ids.length < this.ctxLimit && this.genThisTurn < this.params.maxNewTokens) {
          const r = await this.m.forward([chosen], { capture: this.viz.enabled ? "full" : "none" });
          this.logitsLast = r.logitsLast;
          this._mergeCapture(r.capture);
          this.ids.push(chosen);
          this.meta.push({ kind: "gen" });
          const lastTurn = this.turns[this.turns.length - 1];
          if (lastTurn && lastTurn.role === "assistant") {
            lastTurn.tokens.push(chosen);
          }
          this.genThisTurn++;
          rec.ms = performance.now() - t0;
          this.records.push(rec);
          this.emit("token", rec);
          this._emitState();
          return rec;
        }
        if (this.ids.length >= this.ctxLimit) {
          this.stoppedReason = `\u5DF2\u8FBE\u4E0A\u4E0B\u6587\u9650\u5236 ${this.ctxLimit}\uFF08SYS \u9762\u677F\u53EF\u8C03\uFF0C\u91CD\u7F6E\u540E\u751F\u6548\uFF09`;
          this.emit("log", "warn", `\u505C\u6B62\uFF1A${this.stoppedReason}`);
        } else if (this.genThisTurn >= this.params.maxNewTokens) {
          this.stoppedReason = `\u672C\u8F6E\u5DF2\u8FBE\u6700\u5927\u751F\u6210\u957F\u5EA6 ${this.params.maxNewTokens}`;
          this.emit("log", "warn", `\u505C\u6B62\uFF1A${this.stoppedReason}`);
        } else if (eosHit) {
          this.stoppedReason = "EOS\uFF08<|im_end|>\uFF09";
          rec.eos = true;
          this.records.push(rec);
          this.emit("token", rec);
          this.emit("log", "ok", `\u672C\u8F6E\u751F\u6210\u7ED3\u675F\uFF1A\u9047\u5230 EOS <|im_end|>\uFF08id=2\uFF09`);
        }
        this._emitState();
        return rec;
      } finally {
        this.busy = false;
        this.emit("busy", false);
      }
    }
    countGen() {
      return this.genThisTurn;
    }
    canStep() {
      if (this.stoppedReason) return false;
      if (this.logitsLast == null || this.busy) return false;
      if (this.ids.length >= this.ctxLimit) {
        this.stoppedReason = `\u5DF2\u8FBE\u4E0A\u4E0B\u6587\u9650\u5236 ${this.ctxLimit}\uFF08SYS \u9762\u677F\u53EF\u8C03\uFF0C\u91CD\u7F6E\u540E\u751F\u6548\uFF09`;
        this.emit("log", "warn", `\u505C\u6B62\uFF1A${this.stoppedReason}`);
        return false;
      }
      if (this.genThisTurn >= this.params.maxNewTokens) {
        this.stoppedReason = `\u672C\u8F6E\u5DF2\u8FBE\u6700\u5927\u751F\u6210\u957F\u5EA6 ${this.params.maxNewTokens}`;
        this.emit("log", "warn", `\u505C\u6B62\uFF1A${this.stoppedReason}`);
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
          embed: [],
          // Float32Array(768) per token
          hiddenAll: [],
          layers: this.m.nLayers === void 0 ? [] : range(this.m.nLayers).map(() => ({
            q: [],
            v_x: [],
            attnRows: [],
            ln1: [],
            ln2: [],
            attnContrib: [],
            mlpContrib: []
          })),
          finalNorm: []
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
          P.layers[l].attnRows[absPos] = lc.attnRows.map((rows) => rows[i]);
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
      for (let t = P.prunedTo ?? 0; t < keepFrom; t++) {
        P.embed[t] = null;
        P.hiddenAll[t] = null;
        P.finalNorm[t] = null;
        P.positions[t] = void 0;
        for (let l = 0; l < P.layers.length; l++) {
          P.layers[l].q[t] = null;
          P.layers[l].attnRows[t] = null;
          P.layers[l].ln1[t] = null;
          P.layers[l].ln2[t] = null;
          P.layers[l].attnContrib[t] = null;
          P.layers[l].mlpContrib[t] = null;
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
          if (rows) {
            for (const r of rows) if (r) n += r.length;
          }
        }
      }
      return n * 4;
    }
    _emitState() {
      this.emit("state", this);
    }
  };
  function range(n) {
    return Array.from({ length: n }, (_, i) => i);
  }
  function isEosText(t) {
    return t.includes("<|im_end|>") || t.includes("<|endoftext|>");
  }
  function paramsTopKStr(k) {
    return k === Infinity ? "\u221E" : String(k);
  }
  function sampleFrom(pPost, u01) {
    let acc = 0;
    const u = u01 * totalOf(pPost);
    for (let i = 0; i < pPost.length; i++) {
      if (pPost[i] <= 0) continue;
      acc += pPost[i];
      if (u <= acc) return i;
    }
    return lastNonZero(pPost);
  }
  function totalOf(p) {
    let s = 0;
    for (let i = 0; i < p.length; i++) s += p[i];
    return s;
  }
  function lastNonZero(p) {
    for (let i = p.length - 1; i >= 0; i--) if (p[i] > 0) return i;
    return 0;
  }
  var _rngState = 20260317;
  function rng() {
    const t = _rngState += 1831565813;
    let r = Math.imul(t ^ t >>> 15, 1 | t);
    r ^= r + Math.imul(r ^ r >>> 7, 61 | r);
    return ((r ^ r >>> 14) >>> 0) / 4294967296;
  }

  // js/util.js
  function fmt(x, nd = 3) {
    if (!isFinite(x)) return x > 0 ? "+\u221E" : isNaN(x) ? "NaN" : "\u2212\u221E";
    if (Math.abs(x) >= 1e5 || Math.abs(x) > 0 && Math.abs(x) < 1e-4) return x.toExponential(2);
    return x.toFixed(nd);
  }
  function esc(s) {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }
  function fitCanvas(cv) {
    const dpr = window.devicePixelRatio || 1;
    const r = cv.getBoundingClientRect();
    const w = Math.max(10, Math.round(r.width));
    let cssH = parseInt(cv.style.height, 10);
    if (!Number.isFinite(cssH) || cssH < 4) cssH = parseInt(cv.getAttribute("height") || "", 10);
    if (!Number.isFinite(cssH) || cssH < 4) cssH = Math.round(r.height);
    if (!Number.isFinite(cssH) || cssH < 4) cssH = 120;
    cv.width = Math.round(w * dpr);
    cv.height = Math.round(cssH * dpr);
    cv.style.height = `${cssH}px`;
    const ctx = cv.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    return { ctx, w, h: cssH };
  }
  var C = {
    cyan: "#35e0ff",
    cyanD: "#0e7d99",
    amber: "#ffb347",
    green: "#52ff7a",
    red: "#ff4757",
    magenta: "#ff5ad5",
    blue: "#4da3ff",
    yellow: "#ffe86e",
    dim: "#6d8294",
    faint: "#45596b",
    line: "#1b2a38",
    line2: "#24384c",
    panel: "#0d141c",
    txt: "#c8dce8",
    gridA: "rgba(53,224,255,.05)"
  };
  function drawGridBg(ctx, w, h, step = 26) {
    ctx.fillStyle = "#070d13";
    ctx.fillRect(0, 0, w, h);
    ctx.strokeStyle = "rgba(36,60,82,.22)";
    ctx.lineWidth = 1;
    ctx.beginPath();
    for (let x = step; x < w; x += step) {
      ctx.moveTo(x + 0.5, 0);
      ctx.lineTo(x + 0.5, h);
    }
    for (let y = step; y < h; y += step) {
      ctx.moveTo(0, y + 0.5);
      ctx.lineTo(w, y + 0.5);
    }
    ctx.stroke();
  }
  function heatColor(t) {
    t = Math.min(1, Math.max(0, t));
    const stops = [
      [0, [16, 24, 39]],
      [0.25, [14, 90, 122]],
      [0.5, [53, 224, 255]],
      [0.75, [255, 232, 110]],
      [1, [255, 90, 213]]
    ];
    let i = 1;
    while (i < stops.length - 1 && stops[i][0] < t) i++;
    const [t0, c0] = stops[i - 1], [t1, c1] = stops[i];
    const u = (t - t0) / (t1 - t0 || 1);
    return `rgb(${Math.round(c0[0] + u * (c1[0] - c0[0]))},${Math.round(c0[1] + u * (c1[1] - c0[1]))},${Math.round(c0[2] + u * (c1[2] - c0[2]))})`;
  }

  // js/viz.js
  var FLOW_NODES = [
    { id: "tok", label: "TOKENIZE\n\u5206\u8BCD BPE", col: 0, row: 0 },
    { id: "embed", label: "EMBEDDING\n\u67E5\u8868\u2192768\u7EF4", col: 1, row: 0 },
    { id: "ln1", label: "RMSNorm\n(\u8F93\u5165\u5C42\u8303\u6570)", col: 2, row: 0 },
    { id: "qkv", label: "Q\xB7K\xB7V \u6295\u5F71\n(8\u5934+4KV\u7EC4)", col: 3, row: 0 },
    { id: "rope", label: "RoPE \u65CB\u8F6C\n\u4F4D\u7F6E\u7F16\u7801", col: 4, row: 0 },
    { id: "att", label: "\u7F29\u653E\u70B9\u79EF\u6CE8\u610F\u529B\n\u03A3softmax(qk/\u221Ad)v", col: 5, row: 0 },
    { id: "ores", label: "O\u6295\u5F71+\u6B8B\u5DEE\nx += attn(x)", col: 6, row: 0 },
    { id: "ln2", label: "RMSNorm #2", col: 6, row: 1 },
    { id: "ffn", label: "SwiGLU FFN\ngate\u2299up\u2192down", col: 5, row: 1 },
    { id: "res2", label: "\u6B8B\u5DEE\u76F8\u52A0\nx += ffn(ln2)", col: 4, row: 1 },
    { id: "loop", label: "\xD78 \u5C42\u5FAA\u73AF\nL0 \u2192 L7", col: 3, row: 1 },
    { id: "fnorm", label: "Final Norm", col: 2, row: 1 },
    { id: "head", label: "LM HEAD\n(embedding.T)", col: 1, row: 1 },
    { id: "dist", label: "\u4E0B\u4E00\u4E2A Token\n\u6982\u7387\u5206\u5E03", col: 0, row: 1 }
  ];
  var FLOW_TARGET = {
    tok: null,
    embed: "scatter-e",
    ln1: "calc",
    qkv: "scatter-q",
    rope: "calc",
    att: "heatmap",
    ores: "mlp",
    ln2: "mlp",
    ffn: "mlp",
    res2: null,
    loop: "heatmap",
    fnorm: "calc",
    head: "cand",
    dist: "cand"
  };
  function drawFlow(cv, ui) {
    const { ctx, w, h } = fitCanvas(cv);
    drawGridBg(ctx, w, h);
    const cols = 7, rows = 2;
    const bw = Math.min(118, (w - 30) / cols - 10), bh = 62;
    const ox = (w - (cols * bw + (cols - 1) * 14)) / 2 + 4;
    const oy = h / 2 - bh - 9;
    const pos = {};
    for (const n of FLOW_NODES) {
      pos[n.id] = {
        x: ox + n.col * (bw + 14),
        y: oy + n.row * (bh + 22),
        w: bw,
        h: bh
      };
    }
    ctx.strokeStyle = "rgba(53,224,255,.35)";
    ctx.lineWidth = 1.2;
    const chainTop = ["tok", "embed", "ln1", "qkv", "rope", "att", "ores"];
    const chainBot = ["ores", "ln2", "ffn", "res2", "loop", "fnorm", "head", "dist"];
    ctx.setLineDash([]);
    for (let i = 0; i < chainTop.length - 1; i++) {
      const a = pos[chainTop[i]], b = pos[chainTop[i + 1]];
      arrow(ctx, a.x + a.w, a.y + a.h / 2, b.x, b.y + b.h / 2);
    }
    arrow(
      ctx,
      pos.ores.x + pos.ores.w / 2,
      pos.ores.y + pos.ores.h,
      pos.ln2.x + pos.ln2.w / 2,
      pos.ln2.y
    );
    for (let i = 0; i < chainBot.length - 1; i++) {
      if (chainBot[i] === "ores") continue;
      const a = pos[chainBot[i]], b = pos[chainBot[i + 1]];
      arrow(ctx, a.x - 0, a.y + a.h / 2, b.x + b.w, b.y + b.h / 2, true);
    }
    ctx.save();
    ctx.strokeStyle = "rgba(255,179,71,.55)";
    ctx.setLineDash([4, 3]);
    const l0 = pos.loop, t1 = pos.ln1;
    ctx.beginPath();
    ctx.moveTo(l0.x + l0.w / 2 - 20, l0.y);
    ctx.bezierCurveTo(l0.x + l0.w / 2 - 60, l0.y - 40, t1.x + t1.w / 2 + 46, t1.y - 30, t1.x + t1.w / 2 + 24, t1.y + 12);
    ctx.stroke();
    ctx.restore();
    const act = ui.flowActive;
    let pulseT = performance.now() % 1400 / 1400;
    for (const n of FLOW_NODES) {
      const p = pos[n.id];
      const isHot = act && act.nodes?.includes(n.id);
      ctx.fillStyle = isHot ? "rgba(53,224,255,.16)" : "#0b141d";
      ctx.strokeStyle = isHot ? C.cyan : act?.nodes?.length ? "rgba(53,224,255,.18)" : C.line2;
      ctx.lineWidth = isHot ? 1.6 : 1;
      roundRect(ctx, p.x, p.y, p.w, p.h, 2);
      ctx.fill();
      ctx.stroke();
      if (isHot) {
        ctx.strokeStyle = `rgba(53,224,255,${0.5 - 0.45 * pulseT})`;
        ctx.lineWidth = 3;
        roundRect(ctx, p.x - 2, p.y - 2, p.w + 4, p.h + 4, 3);
        ctx.stroke();
      }
      ctx.fillStyle = isHot ? C.cyan : C.dim;
      ctx.font = '10px "JetBrains Mono", monospace';
      ctx.textAlign = "center";
      const lines = n.label.split("\n");
      lines.forEach((s, i) => ctx.fillText(s, p.x + p.w / 2, p.y + 21 + i * 15));
      ctx.fillStyle = C.faint;
      ctx.font = "8px monospace";
      ctx.fillText(n.id.toUpperCase(), p.x + p.w / 2, p.y + p.h - 5);
      p.node = n.id;
    }
    cv._flowPos = pos;
    function arrow(ctx2, x1, y1, x2, y2, rtl = false) {
      ctx2.beginPath();
      ctx2.moveTo(x1, y1);
      ctx2.lineTo(x2, y2);
      ctx2.stroke();
      const dirX = rtl ? 1 : -1;
      ctx2.beginPath();
      ctx2.moveTo(x2, y2);
      ctx2.lineTo(x2 + dirX * 5, y2 - 3.2);
      ctx2.lineTo(x2 + dirX * 5, y2 + 3.2);
      ctx2.closePath();
      ctx2.fillStyle = "rgba(53,224,255,.5)";
      ctx2.fill();
    }
  }
  function roundRect(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
  }
  function flowHitTest(cv, mx, my) {
    const pos = cv._flowPos;
    if (!pos) return null;
    for (const k of Object.keys(pos)) {
      const p = pos[k];
      if (mx >= p.x && mx <= p.x + p.w && my >= p.y && my <= p.y + p.h) return p.node;
    }
    return null;
  }
  function drawDotBars(cv, q, k, opts = {}) {
    const { ctx, w, h } = fitCanvas(cv);
    drawGridBg(ctx, w, h);
    const hd = q.length;
    const prod = new Float32Array(hd);
    let dotSum = 0;
    for (let d = 0; d < hd; d++) {
      prod[d] = q[d] * k[d];
      dotSum += prod[d];
    }
    const mAbs = Math.max(1e-9, ...Array.from(prod).map(Math.abs));
    const padL = 30, padB = 26, topPad = 10;
    const bw = (w - padL - 8) / hd;
    const zeroY = topPad + (h - topPad - padB) / 2;
    const amp = (h - topPad - padB) / 2 - 2;
    ctx.font = '9px "JetBrains Mono",monospace';
    ctx.textAlign = "right";
    ctx.fillStyle = C.faint;
    ctx.fillText("+", padL - 4, topPad + 8);
    ctx.fillText("\u2212", padL - 4, h - padB - 4);
    ctx.strokeStyle = C.line;
    ctx.beginPath();
    ctx.moveTo(padL, zeroY + 0.5);
    ctx.lineTo(w - 8, zeroY + 0.5);
    ctx.stroke();
    for (let d = 0; d < hd; d++) {
      const v = prod[d] / mAbs;
      const bx = padL + d * bw;
      const bh2 = Math.abs(v) * amp;
      const hl = opts.highlightDim === d;
      ctx.fillStyle = v >= 0 ? hl ? C.yellow : "rgba(82,255,122,.72)" : hl ? C.red : "rgba(255,90,150,.66)";
      if (hl) {
        ctx.shadowColor = v >= 0 ? C.yellow : C.red;
        ctx.shadowBlur = 6;
      }
      ctx.fillRect(bx + 0.5, v >= 0 ? zeroY - bh2 : zeroY, Math.max(1, bw - 1.5), Math.max(bh2, 0.5));
      ctx.shadowBlur = 0;
    }
    ctx.textAlign = "right";
    ctx.fillStyle = C.txt;
    ctx.font = '11px "JetBrains Mono",monospace';
    ctx.fillText(`q\xB7k = ${fmt(dotSum, 2)}`, w - 10, 16);
    if (opts.running != null) {
      ctx.fillStyle = C.green;
      ctx.fillText(`\u7D2F\u8BA1 ${opts.running} dims = ${fmt(opts.runningVal, 2)}`, w - 10, 30);
    }
    ctx.fillStyle = C.faint;
    ctx.font = "9px monospace";
    ctx.textAlign = "left";
    ctx.fillText("dim \u2192 0\u202695\uFF0896 \u7EF4\uFF09", padL, h - 10);
    return dotSum;
  }
  function drawSoftRow(cv, weights, hiIdx, labels, metaTxt = "") {
    const { ctx, w, h } = fitCanvas(cv);
    drawGridBg(ctx, w, h);
    const n = weights.length;
    const padL = 8, padR = 56, padT = 12, padB = 20;
    const maxW = Math.max(1e-9, ...weights);
    const bw = (w - padL - padR) / n;
    for (let i = 0; i < n; i++) {
      const v = weights[i] / maxW;
      const bh2 = v * (h - padT - padB);
      const x = padL + i * bw;
      const isHi = i === hiIdx;
      ctx.fillStyle = isHi ? C.amber : `rgba(53,224,255,${0.28 + 0.55 * v})`;
      if (isHi) {
        ctx.shadowColor = C.amber;
        ctx.shadowBlur = 8;
      }
      ctx.fillRect(x + 1, h - padB - bh2, Math.max(1.2, bw - 2), bh2);
      ctx.shadowBlur = 0;
    }
    ctx.fillStyle = C.faint;
    ctx.font = "9px monospace";
    ctx.textAlign = "left";
    ctx.fillText("key token \u7D22\u5F15 j \u2192", padL + 2, h - 7);
    ctx.textAlign = "right";
    ctx.fillStyle = C.txt;
    ctx.font = '11px "JetBrains Mono",monospace';
    ctx.fillText(`P(key ${hiIdx}) = ${(weights[hiIdx] * 100).toFixed(2)}%`, w - 8, 14);
    if (metaTxt) {
      ctx.fillStyle = C.dim;
      ctx.font = "9px monospace";
      ctx.fillText(metaTxt, w - 8, 26);
    }
  }
  function drawSumViz(cv, vs, outHead, outFullBars, selIdx, metaTxt = "") {
    const { ctx, w, h } = fitCanvas(cv);
    drawGridBg(ctx, w, h);
    const midY = Math.floor(h * 0.42);
    const segH = Math.min(13, (midY - 12) / vs.length);
    const maxWv = Math.max(...vs.map((v) => Math.abs(v.weight)), 1e-9);
    vs.forEach((vv, idx) => {
      const y = 8 + idx * (segH + 2);
      const frac = Math.abs(vv.weight) / maxWv;
      const wid = frac * (w - 210);
      const isSel = idx === selIdx;
      ctx.fillStyle = isSel ? C.amber : vv.color || "rgba(77,163,255,.75)";
      if (isSel) {
        ctx.shadowColor = C.amber;
        ctx.shadowBlur = 7;
      }
      ctx.fillRect(64, y, Math.max(1, wid), segH);
      ctx.shadowBlur = 0;
      ctx.strokeStyle = C.line2;
      ctx.strokeRect(64, y, Math.max(1, wid), segH);
      ctx.fillStyle = C.dim;
      ctx.font = '9.5px "JetBrains Mono",monospace';
      ctx.textAlign = "left";
      ctx.fillText(`w${idx === selIdx ? "\u2605" : ""}=${vv.weight.toFixed(3)} \xB7 v[${vv.j}]`, 2, y + segH - 2.5);
      ctx.fillStyle = isSel ? C.amber : C.faint;
      ctx.fillText(vv.txt.slice(0, 12), 66 + wid + 4, y + segH - 2.5);
    });
    ctx.strokeStyle = C.line;
    ctx.beginPath();
    ctx.moveTo(8, midY + 6);
    ctx.lineTo(w - 8, midY + 6);
    ctx.stroke();
    const botY = midY + 14;
    const barAreaH = h - botY - 6;
    const mAbs = Math.max(1e-9, ...Array.from(outFullBars).map(Math.abs));
    const bdw = (w - 16) / outFullBars.length;
    const zy = botY + barAreaH / 2;
    ctx.strokeStyle = C.line;
    ctx.beginPath();
    ctx.moveTo(8, zy + 0.5);
    ctx.lineTo(w - 8, zy + 0.5);
    ctx.stroke();
    for (let d = 0; d < outFullBars.length; d++) {
      const v = outFullBars[d] / mAbs;
      const bh2 = Math.abs(v) * barAreaH / 2;
      ctx.fillStyle = v >= 0 ? "rgba(82,255,122,.85)" : "rgba(255,90,150,.8)";
      ctx.fillRect(8 + d * bdw + 0.5, v >= 0 ? zy - bh2 : zy, Math.max(1, bdw - 1.4), Math.max(0.6, bh2));
    }
    ctx.fillStyle = C.dim;
    ctx.font = "9px monospace";
    ctx.textAlign = "left";
    ctx.fillText("o_i = \u03A3_t w_t\xB7v_t \uFF0896 \u7EF4\u8F93\u51FA\uFF09", 10, botY - 2);
    if (metaTxt) {
      ctx.textAlign = "right";
      ctx.fillStyle = C.green;
      ctx.font = "10px mono";
      ctx.fillText(metaTxt, w - 8, botY - 2);
    }
  }
  function drawHeatmap(cv, session, layer, head, tokens, allHeads, onClickCell, onHoverCell) {
    const { ctx, w, h } = fitCanvas(cv);
    drawGridBg(ctx, w, h);
    const pool = session.capPool;
    const T = tokens.length;
    if (!pool || T === 0) {
      placeholder(ctx, w, h, "\u7B49\u5F85\u9884\u586B\u6570\u636E");
      return;
    }
    const headsToList = allHeads ? [0, 1, 2, 3, 4, 5, 6, 7] : [head];
    const cell = Math.min((h - 26) / T, (w - 130) / (allHeads ? T : T));
    const cellS = Math.max(3, Math.floor(cell));
    const gridW = cellS * T, gridH = cellS * T;
    const ox = 120 + (w - 130 - gridW) / 2, oy = 6;
    ctx.font = '9px "JetBrains Mono",monospace';
    for (let j = 0; j < T; j += Math.ceil(T / Math.max(4, Math.floor(gridW / 34)))) {
      ctx.save();
      ctx.translate(ox + j * cellS + cellS / 2, oy - 3);
      ctx.rotate(-Math.PI / 4);
      ctx.fillStyle = C.dim;
      ctx.textAlign = "left";
      ctx.fillText(`${j}`, 0, 0);
      ctx.restore();
    }
    for (let hj = 0; hj < headsToList.length; hj++) {
      const hh = headsToList[hj];
      if (allHeads) ctx.globalAlpha = 1 / headsToList.length;
      const lay = session.capPool.layers[layer];
      for (let i = 0; i < T; i++) {
        const row = lay.attnRows[i]?.[hh];
        if (!row) continue;
        for (let j = 0; j < row.length; j++) {
          const v = row[j];
          const mx = ox + j * cellS, my = oy + i * cellS;
          ctx.fillStyle = heatColor(Math.pow(v, 0.42));
          ctx.fillRect(mx, my, cellS - (cellS > 7 ? 1 : 0), cellS - (cellS > 7 ? 1 : 0));
        }
        for (let j = row.length; j < T; j++) {
          const mx = ox + j * cellS, my = oy + i * cellS;
          ctx.fillStyle = "rgba(10,16,23,.5)";
          ctx.fillRect(mx, my, cellS - (cellS > 7 ? 1 : 0), cellS - (cellS > 7 ? 1 : 0));
          if (cellS > 10 && (i + j) % 4 === 0) {
            ctx.strokeStyle = "rgba(70,90,110,.25)";
            ctx.lineWidth = 0.6;
            ctx.beginPath();
            ctx.moveTo(mx, my + cellS);
            ctx.lineTo(mx + cellS, my);
            ctx.stroke();
          }
        }
      }
      ctx.globalAlpha = 1;
    }
    ctx.textAlign = "right";
    for (let i = 0; i < T; i++) {
      if (i % Math.ceil(T / Math.max(6, Math.floor(gridH / 26))) !== 0 && i !== T - 1) continue;
      ctx.fillStyle = C.dim;
      ctx.fillText(`${i}\xB7${tokens[i].short}`, ox - 6, oy + i * cellS + cellS / 2 + 3);
    }
    const lgY = oy + gridH + 14;
    gradBar(ctx, ox, lgY, 160, 8);
    ctx.fillStyle = C.dim;
    ctx.font = "9px monospace";
    ctx.textAlign = "left";
    ctx.fillText("\u6743\u91CD 0 \u2192 \u6700\u5927", ox + 166, lgY + 8);
    cv._hmGeom = { ox, oy, cellS, T };
    cv.onclick = (ev) => {
      const r = cv.getBoundingClientRect();
      const mx = ev.clientX - r.left, my = ev.clientY - r.top;
      if (cv._hmGeom) {
        const { ox: g_ox, oy: g_oy, cellS: cs, T: tN } = cv._hmGeom;
        const j = Math.floor((mx - g_ox) / cs), i = Math.floor((my - g_oy) / cs);
        if (i >= 0 && i < tN && j >= 0 && j < tN && j <= i) onClickCell(i, j);
      }
    };
    cv.onmousemove = (ev) => {
      const r = cv.getBoundingClientRect();
      const mx = ev.clientX - r.left, my = ev.clientY - r.top;
      if (!cv._hmGeom) return;
      const { ox: g_ox, oy: g_oy, cellS: cs, T: tN } = cv._hmGeom;
      const j = Math.floor((mx - g_ox) / cs), i = Math.floor((my - g_oy) / cs);
      if (i >= 0 && i < tN && j >= 0 && j <= i && onHoverCell) {
        const row = session.capPool.layers[layer].attnRows[i]?.[head];
        onHoverCell({ i, j, wgt: row ? row[j] : null });
      } else if (onHoverCell) onHoverCell(null);
    };
    cv.onmouseleave = () => onHoverCell && onHoverCell(null);
  }
  function gradBar(ctx, x, y, w, h) {
    for (let i = 0; i < w; i++) {
      ctx.fillStyle = heatColor(Math.pow(i / (w - 1), 0.42));
      ctx.fillRect(x + i, y, 1, h);
    }
  }
  function placeholder(ctx, w, h, msg) {
    ctx.fillStyle = C.faint;
    ctx.font = "12px monospace";
    ctx.textAlign = "center";
    ctx.fillText(msg, w / 2, h / 2);
  }
  function drawScatter(cv, pts, axes, hoverIdx) {
    const { ctx, w, h } = fitCanvas(cv);
    drawGridBg(ctx, w, h, 30);
    if (!pts || !pts.length) {
      placeholder(ctx, w, h, "\u65E0\u6570\u636E \u2014\u2014 \u5148\u8FD0\u884C\u9884\u586B");
      return;
    }
    const pad = 34;
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    for (const p of pts) {
      minX = Math.min(minX, p.x);
      maxX = Math.max(maxX, p.x);
      minY = Math.min(minY, p.y);
      maxY = Math.max(maxY, p.y);
    }
    if (!isFinite(minX)) return;
    if (maxX - minX < 1e-6) {
      minX -= 1;
      maxX += 1;
    }
    if (maxY - minY < 1e-6) {
      minY -= 1;
      maxY += 1;
    }
    const spanX = maxX - minX, spanY = maxY - minY;
    const map = (p) => [
      pad + (p.x - minX) / spanX * (w - pad - 12),
      h - pad - (p.y - minY) / spanY * (h - pad - 14)
    ];
    if (minX < 0 && maxX > 0) {
      const [, yy] = map({ x: 0, y: minY });
      ctx.strokeStyle = "rgba(109,130,148,.35)";
      ctx.beginPath();
      ctx.moveTo(pad, yy + 0.5);
      ctx.lineTo(w - 12, yy + 0.5);
      ctx.stroke();
    }
    if (minY < 0 && maxY > 0) {
      const [xx] = map({ x: minX, y: 0 });
      ctx.strokeStyle = "rgba(109,130,148,.35)";
      ctx.beginPath();
      ctx.moveTo(xx + 0.5, pad - 10);
      ctx.lineTo(xx + 0.5, h - pad);
      ctx.stroke();
    }
    const marks = {
      Q: (x, y, s) => {
        ctx.fillStyle = s || "rgba(53,224,255,.9)";
        ctx.fillRect(x - 3, y - 3, 6, 6);
      },
      K: (x, y) => {
        ctx.beginPath();
        ctx.moveTo(x, y - 4);
        ctx.lineTo(x + 4, y);
        ctx.lineTo(x, y + 4);
        ctx.lineTo(x - 4, y);
        ctx.closePath();
        ctx.fillStyle = "rgba(255,179,71,.92)";
        ctx.fill();
      },
      V: (x, y) => {
        ctx.beginPath();
        ctx.arc(x, y, 3.4, 0, 7);
        ctx.fillStyle = "rgba(82,255,122,.9)";
        ctx.fill();
      }
    };
    for (let i = 0; i < pts.length; i++) {
      const p = pts[i];
      const [x, y] = map(p);
      if (hoverIdx === i) {
        ctx.strokeStyle = "rgba(255,232,110,.9)";
        ctx.setLineDash([2, 2]);
        ctx.beginPath();
        ctx.moveTo(pad, y);
        ctx.lineTo(x, y);
        ctx.lineTo(x, h - pad);
        ctx.stroke();
        ctx.setLineDash([]);
      }
      (marks[p.type] || marks.Q)(x, y, i === hoverIdx ? C.yellow : null);
      if (p.type === "Q" && hoverIdx === i) {
        ctx.strokeStyle = "rgba(255,232,110,.65)";
        ctx.strokeRect(x - 5.5, y - 5.5, 11, 11);
      }
    }
    ctx.font = "10px monospace";
    ctx.textAlign = "left";
    let lx = pad + 2;
    const present = [...new Set(pts.map((p) => p.type))];
    for (const t of present) {
      marks[t](lx + 5, h - 13, null);
      ctx.fillStyle = C.dim;
      ctx.fillText(t, lx + 14, h - 9);
      lx += 34;
    }
    ctx.fillStyle = C.faint;
    ctx.textAlign = "right";
    ctx.fillText(`X: dim${axes.dxLabel}  Y: dim${axes.dyLabel}${axes.pca ? " \xB7 PCA" : ""}`, w - 10, h - 9);
    ctx.fillText(`[${fmt(minX, 2)}, ${fmt(maxX, 2)}]`, w - 10, 14);
    cv._scMap = map;
    cv._scPts = pts;
  }
  function drawProbCurve(cv, top, postFull, sampledId) {
    const { ctx, w, h } = fitCanvas(cv);
    drawGridBg(ctx, w, h);
    if (!postFull) return;
    const sorted = Array.from(postFull).sort((a, b) => b - a).slice(0, 300);
    const maxP = Math.max(sorted[0], 1e-9);
    ctx.strokeStyle = "rgba(53,224,255,.85)";
    ctx.lineWidth = 1.4;
    ctx.beginPath();
    for (let i = 0; i < sorted.length; i++) {
      const x = 10 + i / 300 * (w - 20);
      const y = h - 10 - Math.log10(1 + sorted[i] / maxP * 999) / 3 * (h - 20);
      i ? ctx.lineTo(x, y) : ctx.moveTo(x, y);
    }
    ctx.stroke();
    ctx.lineTo(w - 10, h - 10);
    ctx.lineTo(10, h - 10);
    ctx.closePath();
    const g = ctx.createLinearGradient(0, 0, 0, h);
    g.addColorStop(0, "rgba(53,224,255,.30)");
    g.addColorStop(1, "rgba(53,224,255,.02)");
    ctx.fillStyle = g;
    ctx.fill();
    ctx.fillStyle = C.faint;
    ctx.font = "8.5px monospace";
    ctx.textAlign = "left";
    ctx.fillText("log-scale \xB7 \u6392\u5E8F\u540E\u7684\u5168\u8BCD\u8868\u540E\u9A8C\u5206\u5E03(Top300)", 12, 12);
  }

  // js/ui.js
  var $ = (id) => document.getElementById(id);
  var UI = class {
    constructor(session, tok, model, log) {
      this.s = session;
      this.tok = tok;
      this.m = model;
      this.log = log;
      this.sel = { qi: -1, ki: -1 };
      this.calc = { layer: 7, head: 0 };
      this.hm = { layer: 7, head: 0, allHeads: false };
      this.sc = { vec: "q", layer: 7, head: 0, dx: 0, dy: 1, pca: false };
      this.mp = { layer: 7, tok: -1 };
      this.flowActive = null;
      this.autoTimer = null;
      this.hoverScatter = -1;
      this.bind();
    }
    tokens() {
      return this.s.ids.map((id, i) => {
        const raw = this.tok.decode([id]);
        return {
          id,
          raw,
          short: shortTok(raw),
          kind: this.s.meta[i]?.kind || "prompt"
        };
      });
    }
    bind() {
      $("inpText").addEventListener("input", () => this.updateTokCount());
      $("btnRun").onclick = () => this.runPrefill();
      $("btnStep").onclick = () => this.stepOnce();
      $("btnAuto").onclick = () => this.toggleAuto();
      $("btnReset").onclick = () => {
        this.s.reset();
        this.fullRefresh();
        this.log("info", "\u4E0A\u4E0B\u6587\u5DF2\u91CD\u7F6E");
      };
      $("selMode").onchange = () => {
        const chat = $("selMode").value === "chat";
        $("chatFields").style.display = chat ? "" : "none";
        $("sbMode").textContent = $("selMode").value;
        this.updateTokCount();
      };
      $("inpSystem").addEventListener("input", () => {
        this.s.systemPrompt = $("inpSystem").value;
        this.updateTokCount();
      });
      this.s.systemPrompt = $("inpSystem").value;
      $("btnSend").onclick = () => this.sendChat();
      $("chatInput").addEventListener("keydown", (ev) => {
        if (ev.key === "Enter" && !ev.shiftKey) {
          ev.preventDefault();
          this.sendChat();
        }
      });
      $("chatInput").addEventListener("input", () => {
        const el = $("chatInput");
        el.style.height = "auto";
        el.style.height = `${Math.min(96, el.scrollHeight)}px`;
      });
      $("btnDoc").onclick = () => $("docOverlay").classList.add("open");
      $("docClose").onclick = () => $("docOverlay").classList.remove("open");
      document.addEventListener("keydown", (ev) => {
        if (ev.key === "Escape") $("docOverlay").classList.remove("open");
      });
      if (this.m.quantNative) {
        $("selPrec").value = "int8";
        $("vPrec").textContent = "INT8";
        for (const o of $("selPrec").options) {
          if (o.value === "fp32") {
            o.disabled = location.protocol === "file:";
            o.textContent = location.protocol === "file:" ? "FP32\uFF08file:// \u4E0B\u65E0\u6CD5\u52A0\u8F7D\u539F\u59CB\u6743\u91CD\uFF09" : "FP32\uFF08\u4ECE minimind-3/ \u539F\u59CB\u6743\u91CD\u52A0\u8F7D \xB7 \u9700\u8054\u7F51\u8BFB\u53D6 122MB\uFF09";
          }
        }
        this.log("ok", "\u5DF2\u52A0\u8F7D INT8 \u5185\u5D4C\u539F\u751F\u6743\u91CD\uFF08W8A16 \xB7 \u5E38\u9A7B 61MB\uFF09\u3002FP32 \u57FA\u7EBF\u5BF9\u7167\u53EF\u901A\u8FC7\u4E0B\u62C9\u5207\u6362\uFF08HTTP \u6A21\u5F0F\uFF09\u3002");
      }
      $("selPrec").onchange = async () => {
        const mode = $("selPrec").value;
        let st;
        try {
          if (mode === "fp32" && this.m.quantNative) {
            if (location.protocol === "file:") {
              this.log("warn", "file:// \u4E0B\u6D4F\u89C8\u5668\u7981\u6B62\u8BFB\u53D6\u672C\u5730\u6743\u91CD\u6587\u4EF6 \u2014\u2014 \u8BF7\u7528 ./start.sh \u4EE5 HTTP \u6A21\u5F0F\u8FD0\u884C\u540E\u518D\u5207\u6362 FP32\u3002");
              $("selPrec").value = "int8";
              return;
            }
            this.log("info", "\u52A0\u8F7D minimind-3/model.safetensors\uFF08122MB\uFF09\u4EE5\u6784\u5EFA FP32 \u57FA\u7EBF\u2026");
            const { loadSafetensorsUrl: loadSafetensorsUrl2 } = await Promise.resolve().then(() => (init_st(), st_exports));
            const { tensors } = await loadSafetensorsUrl2(
              "minimind-3/model.safetensors",
              (f, got, total) => {
                this.log("info", `\u4E0B\u8F7D\u539F\u59CB\u6743\u91CD ${(got / 1048576).toFixed(0)}/${(total / 1048576).toFixed(0)} MB`);
              }
            );
            this.m.adoptFp32(tensors);
            this.log("ok", "FP32 \u539F\u59CB\u6743\u91CD\u5DF2\u5C31\u7EEA\uFF08244MB\uFF09\u2014\u2014 \u73B0\u53EF\u6267\u884C FP32\u2194INT8 \u5BF9\u7167\u3002");
            this.renderStatus();
            return;
          }
          st = this.m.setPrecision(mode);
        } catch (e) {
          this.log("err", `\u7CBE\u5EA6\u5207\u6362\u5931\u8D25: ${e.message}`);
          $("selPrec").value = this.m.precision;
          return;
        }
        $("vPrec").textContent = mode.toUpperCase();
        if (mode === "int8" && st && !this.m.quantNative) {
          this.log("ok", `\u5DF2\u91CF\u5316 INT8\uFF1A${st.tensors} \u5F20\u91CF / ${(st.params / 1e6).toFixed(1)}M \u53C2\u6570\uFF0C${(st.fp32Bytes / 1048576).toFixed(0)}MB \u2192 ${(st.int8Bytes / 1048576).toFixed(1)}MB\uFF08${st.compression.toFixed(2)}\xD7\uFF09\uFF0C\u6743\u91CD\u6700\u5927\u91CF\u5316\u8BEF\u5DEE ${st.maxErr.toExponential(2)}`);
          this.log("info", "INT8 \u4E3A W8A16 \u6743\u91CD\u91CF\u5316\uFF1A\u6FC0\u6D3B\u4FDD\u6301 FP32\uFF1B\u516C\u5F0F scale=max|W\u884C|/127\u3002\u8BE6\u89C1\u6587\u6863 \xA77.6\u3002");
        } else if (mode === "int8") {
          this.log("info", "\u5DF2\u5207\u56DE INT8 \u5185\u5D4C\u539F\u751F\u6743\u91CD\uFF08\u5E38\u9A7B 61MB\uFF09\u3002");
        } else {
          this.log("info", "\u5DF2\u5207\u56DE FP32 \u5168\u7CBE\u5EA6\u3002");
        }
        this.renderStatus();
      };
      $("chkCapture").onchange = () => {
        this.s.viz.enabled = $("chkCapture").checked;
        this.log("warn", `\u53EF\u89C6\u5316\u6355\u83B7\u5DF2${this.s.viz.enabled ? "\u5F00\u542F" : "\u5173\u95ED"}\uFF08\u5173\u95ED\u671F\u95F4\u6563\u70B9/\u70ED\u529B\u56FE/\u6F14\u7B97\u53F0\u4E0D\u518D\u66F4\u65B0\uFF1BKV Cache \u4E0D\u53D7\u5F71\u54CD\uFF09`);
        this.renderStatus();
      };
      $("selVizWin").onchange = () => {
        const v = +$("selVizWin").value;
        this.s.viz.window = v === 0 ? Infinity : v;
        $("vVizWin").textContent = v === 0 ? "\u5168\u90E8" : v;
        this.log("info", `\u53EF\u89C6\u5316\u7A97\u53E3 = ${v === 0 ? "\u5168\u90E8\u4FDD\u7559\uFF08\u5185\u5B58\u968F\u4E0A\u4E0B\u6587\u5E73\u65B9\u589E\u957F\uFF09" : v + " tokens\uFF08\u66F4\u65E9\u7684\u6355\u83B7\u5C06\u88AB\u4E22\u5F03\uFF09"}`);
        this.renderStatus();
      };
      $("selCtxLim").onchange = () => {
        const v = +$("selCtxLim").value;
        this.s.ctxLimit = v;
        $("vCtxLim").textContent = v;
        this.log("warn", `\u4E0A\u4E0B\u6587\u9650\u5236 = ${v}\uFF08\u65B0\u4F1A\u8BDD\u751F\u6548\uFF1B\u82E5\u5F53\u524D ctx \u5DF2\u8D85\u51FA\uFF0C\u751F\u6210\u5C06\u505C\u6B62\uFF0C\u8BF7\u91CD\u7F6E\uFF09`);
        if (this.s.ids.length > v) {
          this.s.stoppedReason = `\u4E0A\u4E0B\u6587\u9650\u5236\u8C03\u6574\u4E3A ${v}\uFF0C\u5F53\u524D ${this.s.ids.length} \u5DF2\u8D85\u51FA \u2014\u2014 \u8BF7\u91CD\u7F6E`;
          if (this.autoTimer) this.toggleAuto();
        }
        this.renderStatus();
      };
      this.initSplitters();
      const bindRange = (id, fmtv, cb) => {
        const el = $(id);
        const f = () => {
          $(el.dataset.v).textContent = fmtv(+el.value);
          cb(+el.value);
        };
        el.dataset.v = `v${id.slice(1)}`;
        el.addEventListener("input", f);
        f();
      };
      bindRange("rTemp", (v) => (v / 100).toFixed(2), (v) => this.s.params.temperature = v / 100);
      bindRange("rTopP", (v) => (v / 100).toFixed(2), (v) => this.s.params.topP = v / 100);
      bindRange("rTopK", (v) => v >= 100 ? "\u221E (\u2265100\u2192\u5173)" : String(v), (v) => this.s.params.topK = v >= 100 ? Infinity : v);
      bindRange("rRep", (v) => (v / 100).toFixed(2), (v) => this.s.params.repPenalty = v / 100);
      bindRange("rDelay", (v) => String(v), (v) => {
        if (this.autoTimer) {
          this.toggleAuto();
          this.toggleAuto();
        }
      });
      bindRange("rMaxNew", (v) => String(v), (v) => this.s.params.maxNewTokens = v);
      $("chkGreedy").onchange = () => this.s.params.greedy = $("chkGreedy").checked;
      $("btnSelfTest").onclick = () => this.selfTest();
      $("cLayer").addEventListener("input", () => {
        this.calc.layer = +$("cLayer").value;
        $("vLayer").textContent = this.calc.layer;
        this.syncPairSel();
        this.renderCalc();
        this.renderHeatmap();
      });
      $("cHead").addEventListener("input", () => {
        this.calc.head = +$("cHead").value;
        $("vHead").textContent = this.calc.head;
        this.renderCalc();
        this.renderHeatmap();
      });
      $("cQuery").addEventListener("change", () => {
        this.sel.qi = +$("cQuery").value;
        if (this.sel.ki > this.sel.qi) this.sel.ki = this.sel.qi;
        this.syncPairSel();
        this.renderCalc();
        this.renderHeatmap();
      });
      $("cKey").addEventListener("change", () => {
        this.sel.ki = +$("cKey").value;
        this.syncPairSel();
        this.renderCalc();
      });
      $("hmLayer").addEventListener("input", () => {
        this.hm.layer = +$("hmLayer").value;
        $("hmLv").textContent = this.hm.layer;
        this.hm.head = this.hm.head;
        this.renderHeatmap();
      });
      $("hmHead").addEventListener("input", () => {
        this.hm.head = +$("hmHead").value;
        $("hmHd").textContent = this.hm.head;
        this.renderHeatmap();
      });
      $("hmAllHeads").onchange = () => {
        this.hm.allHeads = $("hmAllHeads").checked;
        this.renderHeatmap();
      };
      $("scVec").onchange = () => {
        this.sc.vec = $("scVec").value;
        this.renderScatter();
      };
      $("scLayer").addEventListener("input", () => {
        this.sc.layer = +$("scLayer").value;
        $("scLv").textContent = this.sc.layer;
        this.renderScatter();
      });
      $("scHead").addEventListener("input", () => {
        this.sc.head = Math.min(3, +$("scHead").value);
        $("scHd").textContent = this.sc.head;
        this.renderScatter();
      });
      $("scDX").onchange = () => {
        this.sc.dx = +$("scDX").value;
        this.renderScatter();
      };
      $("scDY").onchange = () => {
        this.sc.dy = +$("scDY").value;
        this.renderScatter();
      };
      $("scPCA").onchange = () => {
        this.sc.pca = $("scPCA").checked;
        this.renderScatter();
      };
      $("mpLayer").addEventListener("input", () => {
        this.mp.layer = +$("mpLayer").value;
        $("mpLv").textContent = this.mp.layer;
        this.renderMlp();
      });
      $("mpTok").onchange = () => {
        this.mp.tok = +$("mpTok").value;
        this.renderMlp();
      };
      for (const b of document.querySelectorAll(".tabbar button")) {
        b.onclick = () => {
          document.querySelectorAll(".tabbar button").forEach((x) => x.classList.remove("active"));
          b.classList.add("active");
          for (const p of document.querySelectorAll(".tabpage")) p.hidden = p.id !== `tab-${b.dataset.tab}`;
          this.refreshViz();
        };
      }
      const fc = $("flowCanvas");
      fc.addEventListener("mousemove", (ev) => {
        const r = fc.getBoundingClientRect();
        const node = flowHitTest(fc, ev.clientX - r.left, ev.clientY - r.top);
        fc.style.cursor = node ? "pointer" : "default";
        if (node) showTip(ev, `<span class="tt-t">${node.toUpperCase()}</span> \u70B9\u51FB\u8DF3\u8F6C\u5BF9\u5E94\u76D1\u63A7\u9762\u677F`);
        else hideTip();
      });
      fc.addEventListener("mouseleave", hideTip);
      fc.addEventListener("click", (ev) => {
        const r = fc.getBoundingClientRect();
        const node = flowHitTest(fc, ev.clientX - r.left, ev.clientY - r.top);
        if (!node) return;
        const t = FLOW_TARGET[node];
        if (t === "calc") activateTab("calc");
        else if (t === "heatmap") activateTab("heatmap");
        else if (t === "scatter-e") {
          this.sc.vec = "e";
          $("scVec").value = "e";
          activateTab("scatter");
        } else if (t === "scatter-q") {
          this.sc.vec = "q";
          $("scVec").value = "q";
          activateTab("scatter");
        } else if (t === "mlp") activateTab("mlp");
        else if (t === "cand") $("candTable").scrollIntoView({ behavior: "smooth", block: "nearest" });
        this.log("info", `\u6D41\u6C34\u7EBF\u89C6\u56FE \u2192 ${node}`);
      });
      document.addEventListener("keydown", (ev) => {
        if (ev.target.matches("textarea, input[type=text], select")) return;
        if (ev.key === "s" || ev.key === "S") this.stepOnce();
        else if (ev.key === "a" || ev.key === "A") this.toggleAuto();
        else if (ev.key === "r" || ev.key === "R") {
          this.s.reset();
          this.fullRefresh();
        }
      });
      window.addEventListener("resize", () => this.refreshViz());
    }
    // =================== actions ===================
    async runPrefill() {
      const text = $("inpText").value;
      if (!text.trim()) {
        this.log("warn", "\u8F93\u5165\u4E3A\u7A7A");
        return;
      }
      this.s.mode = $("selMode").value;
      $("btnRun").disabled = true;
      try {
        await this.s.prefill(text);
        this.sel.qi = this.s.ids.length - 1;
        this.sel.ki = this.s.ids.length - 1;
        this.fullRefresh(true);
        const d = this.s.distribution(this.s.logitsLast, this.s.params);
        this.log("ok", `\u4E0B\u4E00\u4E2A token \u5019\u9009\uFF1A` + d.top.slice(0, 4).map((t) => `${JSON.stringify(shortTok(t.txt))}(${(t.pPost * 100).toFixed(1)}%)`).join(" "));
        if ($("chkStream").checked) this.ensureAuto();
      } catch (e) {
        this.log("err", `\u9884\u586B\u5931\u8D25: ${e.message}`);
        console.error(e);
      } finally {
        $("btnRun").disabled = false;
      }
    }
    async stepOnce() {
      if (!this.s.canStep()) return;
      try {
        const rec = await this.s.stepOnce();
        if (rec) {
          this.sel.qi = rec.pos;
          if (this.sel.ki > rec.pos) this.sel.ki = rec.pos;
          this.fullRefresh();
          this.flowPulse(["head", "dist", "embed", "ln1", "qkv", "rope", "att", "ores", "ln2", "ffn", "res2", "loop", "fnorm"]);
          this.log("info", `step@${rec.pos}: \u91C7\u6837 ${JSON.stringify(shortTok(rec.chosenTxt))} (P=${(rec.pPostChosen * 100).toFixed(2)}%, T=${rec.temperature}, greedy=${this.s.params.greedy}) ${rec.ms.toFixed(0)}ms`);
        }
      } catch (e) {
        this.log("err", `\u63A8\u7406\u6B65\u5931\u8D25: ${e.message}`);
        console.error(e);
      }
    }
    toggleAuto() {
      if (this.autoTimer) {
        clearInterval(this.autoTimer);
        this.autoTimer = null;
        $("btnAuto").classList.remove("active");
        $("btnAuto").textContent = "\u81EA\u52A8\u64AD\u653E \u25B8";
        this.log("info", "\u81EA\u52A8\u64AD\u653E\u5DF2\u6682\u505C");
        return;
      }
      this.startAuto();
    }
    startAuto() {
      if (this.autoTimer) return;
      if (!this.s.canStep() && this.s.logitsLast == null) {
        this.log("warn", "\u8BF7\u5148\u9884\u586B\u8F93\u5165");
        return;
      }
      const delay = +$("rDelay").value;
      $("btnAuto").classList.add("active");
      $("btnAuto").textContent = "\u23F8 \u505C\u6B62";
      this.log("info", `\u8FDE\u7EED\u8F93\u51FA\u5F00\u59CB\uFF08\u95F4\u9694 ${delay}ms\uFF09`);
      const tick = async () => {
        if (!this.s.canStep()) {
          this.toggleAuto();
          return;
        }
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
      const text = $("chatInput").value.trim();
      if (!text) return;
      $("chatInput").value = "";
      $("chatInput").style.height = "auto";
      if (this.autoTimer) this.toggleAuto();
      if (this.s.logitsLast == null) {
        $("inpText").value = text;
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
        if ($("chkStream").checked) this.ensureAuto();
        else this.log("info", "\u5355\u6B65\u6A21\u5F0F\uFF1A\u6BCF\u70B9\u4E00\u6B21\u300C\u5355\u6B65 +1 Token\u300D\u8F93\u51FA\u4E00\u4E2A Token");
      } catch (e) {
        this.log("err", `\u8FFD\u95EE\u5931\u8D25: ${e.message}`);
        console.error(e);
      }
    }
    initSplitters() {
      const persist = () => {
        try {
          const cs = getComputedStyle(document.documentElement);
          localStorage.setItem("dsh_layout_v2", JSON.stringify({
            wL: cs.getPropertyValue("--wL").trim(),
            wR: cs.getPropertyValue("--wR").trim(),
            hTop: $("centerTop").style.height || "",
            hChat: $("chatPanel").style.flexBasis || ""
          }));
        } catch {
        }
      };
      const restore = () => {
        try {
          const saved = JSON.parse(localStorage.getItem("dsh_layout_v2") || "null");
          if (!saved) return;
          if (saved.wL) document.documentElement.style.setProperty("--wL", saved.wL);
          if (saved.wR) document.documentElement.style.setProperty("--wR", saved.wR);
          if (saved.hTop) $("centerTop").style.height = saved.hTop;
          if (saved.hChat) $("chatPanel").style.flexBasis = saved.hChat;
        } catch {
        }
      };
      restore();
      const drag = (el, onMove) => {
        el.addEventListener("pointerdown", (ev) => {
          ev.preventDefault();
          el.setPointerCapture(ev.pointerId);
          el.classList.add("dragging");
          document.body.style.userSelect = "none";
          const move = (e2) => onMove(e2);
          const up = () => {
            el.classList.remove("dragging");
            document.body.style.userSelect = "";
            el.removeEventListener("pointermove", move);
            el.removeEventListener("pointerup", up);
            persist();
            this.refreshViz();
          };
          el.addEventListener("pointermove", move);
          el.addEventListener("pointerup", up);
        });
        el.addEventListener("dblclick", () => {
        });
      };
      drag($("splitL"), (e2) => {
        const w = Math.max(210, Math.min(560, e2.clientX - 6));
        document.documentElement.style.setProperty("--wL", `${w}px`);
      });
      drag($("splitR"), (e2) => {
        const w = Math.max(240, Math.min(560, window.innerWidth - e2.clientX - 6));
        document.documentElement.style.setProperty("--wR", `${w}px`);
      });
      drag($("splitC"), (e2) => {
        const top = $("centerTop");
        const h = Math.max(90, Math.min(window.innerHeight * 0.6, e2.clientY - top.getBoundingClientRect().top));
        top.style.height = `${h}px`;
      });
      drag($("splitChat"), (e2) => {
        const panel = $("chatPanel");
        const colR = $("colRight").getBoundingClientRect();
        const h = Math.max(140, Math.min(colR.bottom - e2.clientY, window.innerHeight * 0.8));
        panel.style.flex = `0 0 ${h}px`;
      });
      window.addEventListener("pointerup", () => setTimeout(() => this.refreshViz(), 30));
    }
    async selfTest() {
      this.log("info", "\u4EA4\u53C9\u9A8C\u8BC1\uFF1ATF(\u5F53\u524D\u540E\u7AEF) vs \u7EAFJS \u5185\u6838\u5BF9\u540C\u4E00\u524D\u5411\u7684 logits \u504F\u5DEE\u2026");
      const { createBackend: createBackend2 } = await Promise.resolve().then(() => (init_mat(), mat_exports));
      const ids = this.s.ids.slice(0, Math.min(this.s.ids.length, 24));
      if (ids.length < 2) {
        this.log("warn", "\u9700\u8981\u5148\u9884\u586B\u22652\u4E2Atoken");
        return;
      }
      const qm = this.m.quantNative ? this.m.quant : null;
      const mb = new this.m.constructor({ weights: this.m.w, config: this.m.cfg, backend: this.m.backend, maxCtx: this.m.maxCtx, quantMap: qm, log: () => {
      } });
      const mj = new this.m.constructor({ weights: this.m.w, config: this.m.cfg, backend: await createBackend2("js"), maxCtx: this.m.maxCtx, quantMap: qm, log: () => {
      } });
      const rb = await mb.forward(ids, { capture: "none" });
      const rj = await mj.forward(ids, { capture: "none" });
      let mx = 0;
      for (let i = 0; i < rj.logitsLast.length; i++) mx = Math.max(mx, Math.abs(rj.logitsLast[i] - rb.logitsLast[i]));
      this.log("ok", `\u4EA4\u53C9\u9A8C\u8BC1\u5B8C\u6210\uFF1Amax|\u0394logits| = ${mx.toExponential(2)}  ${mx < 0.05 ? "\u2713 \u4E00\u81F4" : "\u26A0 \u504F\u5DEE\u8F83\u5927"}`);
      if (!this.m.hasFp32) {
        this.log("info", "INT8 \u5185\u5D4C\u6A21\u5F0F\u65E0 FP32 \u57FA\u7EBF \u2014\u2014 \u5982\u9700\u91CF\u5316\u5BF9\u7167\uFF0C\u8BF7\u7528 HTTP \u6A21\u5F0F\u5E76\u5728\u7CBE\u5EA6\u4E0B\u62C9\u52A0\u8F7D\u539F\u59CB\u6743\u91CD\u3002");
        return;
      }
      try {
        const mf = new this.m.constructor({ weights: this.m.w, config: this.m.cfg, backend: await createBackend2("js"), maxCtx: this.m.maxCtx, log: () => {
        } });
        const mi = new this.m.constructor({ weights: this.m.w, config: this.m.cfg, backend: await createBackend2("js"), maxCtx: this.m.maxCtx, log: () => {
        } });
        mi.setPrecision("int8");
        const rf = await mf.forward(ids, { capture: "none" });
        const ri = await mi.forward(ids, { capture: "none" });
        let dq = 0;
        let topF = 0, topI = 0;
        for (let i = 0; i < ri.logitsLast.length; i++) {
          if (Math.abs(ri.logitsLast[i] - rf.logitsLast[i]) > dq) dq = Math.abs(ri.logitsLast[i] - rf.logitsLast[i]);
          if (rf.logitsLast[i] > rf.logitsLast[topF]) topF = i;
          if (ri.logitsLast[i] > ri.logitsLast[topI]) topI = i;
        }
        const sm = (arr) => {
          let mx2 = -Infinity;
          for (const v of arr) if (v > mx2) mx2 = v;
          let sum = 0;
          const p = new Float64Array(arr.length);
          for (let i = 0; i < arr.length; i++) {
            p[i] = Math.exp(arr[i] - mx2);
            sum += p[i];
          }
          for (let i = 0; i < p.length; i++) p[i] /= sum;
          return p;
        };
        const pf = sm(rf.logitsLast), pi = sm(ri.logitsLast);
        let agree = 0;
        for (let i = 0; i < pf.length; i++) agree += Math.min(pf[i], pi[i]);
        this.log("ok", `\u91CF\u5316\u5BF9\u7167 FP32\u2194INT8\uFF1Amax|\u0394logits| = ${dq.toExponential(2)}\uFF0CTop-1 ${topF === topI ? "\u4E00\u81F4" : `\u4E0D\u4E00\u81F4(${topF} vs ${topI})`}\uFF0C\u5206\u5E03\u91CD\u5408\u5EA6 ${(agree * 100).toFixed(1)}%`);
      } catch (e2) {
        this.log("err", `\u91CF\u5316\u5BF9\u7167\u5931\u8D25: ${e2.message}`);
      }
    }
    // =================== rendering ===================
    updateTokCount() {
      const text = $("inpText").value;
      const n = this.tok.encode(text).length;
      $("tokCount").textContent = `${n} tok`;
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
      const strip = $("tokenStrip");
      const toks = this.tokens();
      if (rebuild || strip.childElementCount !== toks.length) {
        strip.innerHTML = "";
        toks.forEach((t, i) => {
          const el = document.createElement("span");
          el.className = "tok-chip" + (t.kind === "gen" ? " gen" : "") + (isSpecialText(t.raw) ? " special" : "");
          el.dataset.i = i;
          el.innerHTML = `<span class="n">${i}</span>${esc(shortTok(t.raw))}`;
          el.onclick = (ev) => {
            if (ev.altKey) {
              this.sel.ki = i;
            } else if (ev.shiftKey) {
              this.sel.ki = i;
            } else {
              this.sel.qi = i;
              if (this.sel.ki > i) this.sel.ki = i;
            }
            this.syncPairSel();
            this.renderCalc();
            this.renderHeatmap();
          };
          el.oncontextmenu = (ev) => {
            ev.preventDefault();
            this.sel.ki = i;
            this.syncPairSel();
            this.renderCalc();
          };
          strip.appendChild(el);
        });
      }
      strip.style.display = "";
      [...strip.children].forEach((el, i) => {
        el.classList.toggle("qsel", i === this.sel.qi);
        el.classList.toggle("ksel", i === this.sel.ki);
      });
      $("pairStatus").innerHTML = `\u67E5\u8BE2 Query\uFF1A<b>${this.sel.qi >= 0 ? `${this.sel.qi}\xB7${esc(shortTok(toks[this.sel.qi]?.raw ?? ""))}` : "\u2014"}</b>\u3000\u952E Key\uFF1A<b>${this.sel.ki >= 0 ? `${this.sel.ki}\xB7${esc(shortTok(toks[this.sel.ki]?.raw ?? ""))}` : "\u2014"}</b>\u3000<span style="color:var(--faint)">(\u5DE6\u952E\u9009Q / \u53F3\u952E\u6216Shift\u9009K)</span>`;
      const qsel = $("cQuery"), ksel = $("cKey");
      if (qsel.options.length !== toks.length) {
        qsel.innerHTML = toks.map((t, i) => `<option value="${i}">${i}\xB7${esc(shortTok(t.raw))}</option>`).join("");
        ksel.innerHTML = qsel.innerHTML;
      }
      if (this.mp.tok < 0) {
        const msel = $("mpTok");
        msel.innerHTML = toks.map((t, i) => `<option value="${i}">${i}\xB7${esc(shortTok(t.raw))}</option>`).join("");
        this.mp.tok = toks.length - 1;
      }
    }
    syncPairSel() {
      const n = this.s.ids.length;
      if (this.sel.qi >= n) this.sel.qi = n - 1;
      if (this.sel.ki > this.sel.qi) this.sel.ki = this.sel.qi;
      if (this.sel.ki < 0) this.sel.ki = Math.min(this.sel.qi, n - 1);
      $("cQuery").value = this.sel.qi;
      $("cKey").value = this.sel.ki;
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
        $("fDot").innerHTML = '<span style="color:var(--faint)">\u53EF\u89C6\u5316\u6355\u83B7\u5DF2\u5173\u95ED \u2014\u2014 \u6253\u5F00 Module02 \u7684\u300C\u53EF\u89C6\u5316\u6355\u83B7\u300D\u5F00\u5173\u4EE5\u542F\u7528\u6F14\u7B97\u53F0\u3002</span>';
        $("fSoft").innerHTML = "";
        $("fSum").innerHTML = "";
        return;
      }
      if (!qVec || !row) {
        const msg = '<span style="color:var(--amber)">\u8BE5 token \u7684\u6355\u83B7\u5DF2\u968F\u53EF\u89C6\u5316\u7A97\u53E3\uFF08' + (s.viz.window === Infinity ? "\u5168\u90E8" : s.viz.window) + "\uFF09\u6EDA\u52A8\u4E22\u5F03 \u2014\u2014 \u9009\u62E9\u66F4\u8FD1\u7684 token\u3001\u8C03\u5927\u7A97\u53E3\u6216\u91CD\u7F6E\u3002</span>";
        $("fDot").innerHTML = msg;
        $("fSoft").innerHTML = "";
        $("fSum").innerHTML = "";
        return;
      }
      if (qVec && kVec) {
        let dot = 0;
        for (let d = 0; d < hd; d++) dot += qVec[d] * kVec[d];
        drawDotBars($("dotBars"), qVec, kVec, { highlightDim: -1 });
        $("fDot").innerHTML = `<span class="fn">q</span><sub>${i},${h}</sub> \xB7 <span class="fn">k</span><sub>${j},KV${g}</sub> = <span class="num">${fmt(dot, 2)}</span> &nbsp;<span class="op">/</span>&nbsp; \u221A${hd}=<span class="num">${Math.sqrt(hd).toFixed(3)}</span> = <span class="res">score(${i}\u2192${j}) = ${fmt(dot * m.scale, 4)}</span>`;
      }
      if (row) {
        drawSoftRow($("softRow"), row, j, null, `layer=${l} head=${h} \xB7 \u884C\u548C=${row.reduce((a, b) => a + b, 0).toFixed(6)}`);
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
        $("fSoft").innerHTML = `score = <span class="num">${fmt(scoreIJ, 3)}</span>\uFF08\u884C\u5185\u6700\u5927 <span class="num">${fmt(maxScore, 3)}</span>\uFF09 &nbsp;<span class="op">\u2192</span>&nbsp; exp(score\u2212max) = <span class="num">${fmt(Math.exp(scoreIJ - maxScore), 5)}</span> &nbsp;<span class="op">\xF7</span>&nbsp; \u03A3_t exp = &nbsp;<span class="op">\u21D2</span>&nbsp; <span class="res">w(${i}\u2192${j}) = ${row[j] !== void 0 ? fmt(row[j], 5) : "\u2014"}</span><br><span style="color:var(--faint)">softmax(s)_j = exp(s_j \u2212 s_max) / \u03A3 exp(s_t \u2212 s_max)\u3000\xB7\u3000\u6570\u503C\u7A33\u5B9A\u51CF\u6700\u5927\u503C\u9632\u6EA2\u51FA</span>`;
        $("maskCheck").innerHTML = j <= i ? `<div class="note">j=${j} \u2264 i=${i} \u2192 <b style="color:var(--green)">\u5141\u8BB8</b>\uFF1AKey \u4E0D\u5728 Query \u672A\u6765\u3002\u6CE8\u610F\u529B\u53EF\u8BFB\u53D6\u8BE5 token\u3002</div>` : `<div class="note">j=${j} &gt; i=${i} \u2192 <b style="color:var(--red)">\u56E0\u679C\u63A9\u7801\u5C4F\u853D</b>\uFF1Ascore \u7F6E \u2212\u221E\uFF0Csoftmax \u540E\u6743\u91CD\u4E3A 0\u3002\u8BED\u8A00\u6A21\u578B\u4E0D\u80FD\u5077\u770B\u672A\u6765\uFF01</div>`;
        const pos = i;
        const outVec = s.capPool.layers[l].attnContrib[pos];
        let onorm = 0;
        if (outVec) {
          for (let d2 = 0; d2 < outVec.length; d2++) onorm += outVec[d2] * outVec[d2];
        }
        $("outFlowNote").innerHTML = `\u8BE5\u884C\u52A0\u6743\u7ED3\u679C\u7ECF <code>W_o</code> (768\xD7768) \u5408\u5E76 8 \u5934 \u2192 \u52A0\u8FDB\u6B8B\u5DEE\u6D41\u3002<br>\u6B64 token \u6B64\u523B\u7684\u6CE8\u610F\u529B\u8D21\u732E\u8303\u6570 \u2016attn\u2016\u2248<b>${outVec ? fmt(Math.sqrt(onorm), 2) : "\u2014"}</b>\uFF0CMLP \u8D21\u732E\u968F\u540E\u53E0\u52A0\uFF08\u89C1 \u2464 FFN \u89C6\u56FE\uFF09\u3002`;
      }
      const vs = [];
      if (row) {
        const kk = Math.min(i, j);
        const idxs = Array.from({ length: row.length }, (_, t) => t);
        idxs.sort((a, b) => row[b] - row[a]);
        const chosen = new Set(idxs.slice(0, 7));
        chosen.add(kk);
        const list = [...chosen].sort((a, b) => a - b);
        for (const t of list) {
          vs.push({
            j: t,
            weight: row[t],
            txt: shortTok(this.tok.decode([s.ids[t]])),
            color: t === kk ? C.amber : `rgba(77,163,255,${0.3 + 0.6 * (row[t] / Math.max(...row))})`
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
        drawSumViz(
          $("sumViz"),
          vs,
          outHead,
          outHead,
          j <= i ? vs.findIndex((v) => v.j === j) : -1,
          `\u03A3w\xB7v \u2192 o(${i},h${h})`
        );
        $("fSum").innerHTML = `o<sub>${i},${h}</sub> = \u03A3<sub>t\u2264${i}</sub> w<sub>t</sub>\xB7v<sub>t</sub> &nbsp;(\u663E\u793A\u8D21\u732E\u6700\u5927\u7684 ${vs.length} \u9879\uFF0C\u2605\u4E3A\u9009\u4E2D Key j=${j}\uFF0Cw=${row[j] !== void 0 ? fmt(row[j], 4) : "\u2014"})`;
      }
      if (row && row[j] !== void 0) {
        $("sbLayer").textContent = l;
        $("sbHead").textContent = h;
        $("sbGroup").textContent = g;
        let dot2 = 0;
        if (qVec && kVec) for (let d = 0; d < hd; d++) dot2 += qVec[d] * kVec[d];
        $("sbScore").textContent = fmt(dot2 * m.scale, 3);
        $("sbProb").textContent = this.s.logitsLast ? "" : "";
      }
    }
    // ---------- heatmap ----------
    renderHeatmap() {
      drawHeatmap(
        $("heatCanvas"),
        this.s,
        this.hm.layer,
        this.hm.head,
        this.tokens(),
        this.hm.allHeads,
        (qi, kj) => {
          this.sel.qi = qi;
          this.sel.ki = kj;
          this.calc.layer = this.hm.layer;
          $("cLayer").value = this.hm.layer;
          $("vLayer").textContent = this.hm.layer;
          this.calc.head = this.hm.head;
          $("cHead").value = this.hm.head;
          $("vHead").textContent = this.hm.head;
          this.syncPairSel();
          this.renderCalc();
          this.renderTokenStrip();
          activateTab("calc");
          this.log("info", `\u70ED\u529B\u56FE\u9009\u4E2D (${qi}\u2192${kj}) \u5DF2\u540C\u6B65\u81F3\u6F14\u7B97\u53F0`);
        },
        (cell) => {
          if (!cell) {
            hideTip();
            return;
          }
          const toks = this.tokens();
          showTipAt(event, `<span class="tt-t">(${cell.i} \u2192 ${cell.j})</span> ${esc(shortTok(toks[cell.i].raw))} \u21D2 ${esc(shortTok(toks[cell.j].raw))}${cell.wgt != null ? ` \xB7 w=${cell.wgt.toFixed(4)}` : " \xB7 \u88AB\u63A9\u7801"}`);
        }
      );
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
          case "q": {
            const hd = this.m.headDim;
            const base = P.layers[l].q[t];
            if (!base) return null;
            const hOff = this.sc.head * hd;
            return base.subarray(hOff, hOff + hd);
          }
          case "k":
            return this.m.getKeyVec(l, t, this.sc.head);
          case "v":
            return this.m.getValueVec(l, t, this.sc.head);
          case "e":
            return P.embed[t];
          case "ho":
            return P.hiddenAll[t];
        }
      };
      let dims = [this.sc.dx, this.sc.dy];
      const hdQ = this.m.headDim, dV = this.sc.vec === "e" || this.sc.vec === "ho" ? this.m.dModel : hdQ;
      const normDim = (d) => d < 0 ? dV + d : d;
      dims = dims.map(normDim);
      dims = dims.map((d) => Math.max(0, Math.min(dV - 1, d)));
      if (this.sc.pca) {
        pts = computePCA2(vecFor, nT, this.sc.vec);
      } else {
        for (let t = 0; t < nT; t++) {
          const vec = vecFor(t);
          if (!vec) continue;
          const types = this.sc.vec === "q" ? ["Q"] : this.sc.vec === "k" ? ["K"] : this.sc.vec === "v" ? ["V"] : ["Q", "K", "V"];
          void types;
          const typ = this.sc.vec === "q" ? "Q" : this.sc.vec === "k" ? "K" : this.sc.vec === "v" ? "V" : "E";
          pts.push({ x: vec[dims[0]], y: vec[dims[1]], type: typ, t });
        }
      }
      const cv = $("scatterCanvas");
      drawScatter(cv, pts, { dxLabel: this.sc.pca ? "PC1" : dims[0], dyLabel: this.sc.pca ? "PC2" : dims[1], pca: this.sc.pca }, this.hoverScatter);
      cv.onmousemove = (ev) => {
        const r = cv.getBoundingClientRect();
        const mx = ev.clientX - r.left, my = ev.clientY - r.top;
        let best = -1, bd = 64;
        if (cv._scMap && cv._scPts) {
          cv._scPts.forEach((p, idx) => {
            const [px, py] = cv._scMap(p);
            const dd = (px - mx) ** 2 + (py - my) ** 2;
            if (dd < bd) {
              bd = dd;
              best = idx;
            }
          });
        }
        if (best !== this.hoverScatter) {
          this.hoverScatter = best;
          drawScatter(cv, cv._scPts, { dxLabel: this.sc.pca ? "PC1" : dims[0], dyLabel: this.sc.pca ? "PC2" : dims[1], pca: this.sc.pca }, best);
        }
        if (best >= 0) {
          const p = cv._scPts[best];
          showTipAt(ev, `<span class="tt-t">#${p.t} ${esc(shortTok(this.tok.decode([s.ids[p.t]])))}</span><br>${p.type} vec: x=${fmt(p.x, 3)} y=${fmt(p.y, 3)}${this.sc.pca ? " (PCA)" : ` (dim${dims[0]},${dims[1]})`}`);
        } else hideTip();
      };
      cv.onmouseleave = () => {
        this.hoverScatter = -1;
        hideTip();
        this.renderScatter();
      };
      $("scHead").max = this.sc.vec === "e" || this.sc.vec === "ho" ? 3 : this.sc.vec === "q" ? 3 : 3;
    }
    // ---------- mlp view ----------
    renderMlp() {
      const s = this.s;
      if (!s.capPool || this.mp.tok < 0 || this.mp.tok >= s.ids.length) return;
      const l = this.mp.layer, t = this.mp.tok;
      const gate = s.capPool.layers[l].ln2[t];
      if (!s.viz.enabled || !gate) {
        $("fSwiglu").innerHTML = '<span style="color:var(--amber)">\u8BE5 token \u7684\u6355\u83B7\u5DF2\u4E22\u5F03\u6216\u6355\u83B7\u5DF2\u5173\u95ED\u3002</span>';
        return;
      }
      const m = this.m;
      const d = m.dModel, inter = m.intermediate;
      (async () => {
        const gateP = await m._lin(gate, 1, `model.layers.${l}.mlp.gate_proj.weight`, d, inter);
        const upP = await m._lin(gate, 1, `model.layers.${l}.mlp.up_proj.weight`, d, inter);
        const act = new Float32Array(inter);
        for (let i2 = 0; i2 < inter; i2++) act[i2] = gateP[i2] / (1 + Math.exp(-gateP[i2])) * upP[i2];
        const contrib = s.capPool.layers[l].mlpContrib[t];
        drawSwiglu($("swigluCanvas"), gateP, upP, act, contrib);
        const tops = Array.from({ length: inter }, (_, i3) => i3).sort((a, b) => act[b] - act[a]).slice(0, 5);
        $("fSwiglu").innerHTML = `\u8F93\u5165 \u2016ln2(x)\u2016=${fmt(norm(gate), 2)}\uFF1B\u6FC0\u6D3B\u8C31\u524D5\u901A\u9053\uFF1A` + tops.map((i3) => `ch${i3}: <span class="num">${fmt(act[i3], 2)}</span>`).join("\uFF0C ") + `\uFF1B\u8F93\u51FA\u8D21\u732E\u8303\u6570 \u2016mlp\u2016=<span class="res">${fmt(norm(contrib), 2)}</span>`;
      })();
    }
    // ---------- candidates ----------
    renderCands() {
      const s = this.s;
      const body = $("candBody");
      if (s.logitsLast == null) {
        body.innerHTML = `<tr><td colspan="5" style="color:var(--faint)">\u7B49\u5F85\u9884\u586B\u2026</td></tr>`;
        $("entVal").textContent = "\u2014";
        $("pplVal").textContent = "\u2014";
        $("entFill").style.width = "0%";
        drawProbCurve($("probCurve"), null, null, null);
        return;
      }
      const dist = s.distribution(s.logitsLast, s.params);
      const lastRec = s.records[s.records.length - 1];
      body.innerHTML = dist.top.map((t, k) => {
        const isChosen = lastRec && !lastRec.eos ? lastRec.chosen === t.id : false;
        const outCls = t.filteredOut ? "out" : "";
        return `<tr class="cand-row ${isChosen ? "chosen" : ""} ${outCls}" data-id="${t.id}"><td>${k + 1}</td><td>${esc(shortTok(t.txt))}<span style="color:var(--faint);font-size:9px"> ${t.id}</span></td><td>${pctStr(t.pRaw)}</td><td><div class="cbar post"><i style="width:${(t.pPost * 100).toFixed(1)}%"></i></div><span style="font-size:9.5px;color:var(--dim)">${pctStr(t.pPost)}</span></td><td>${(t.cum * 100).toFixed(2)}%</td></tr>`;
      }).join("");
      for (const tr of body.querySelectorAll(".cand-row")) {
        tr.onclick = async () => {
          const id = +tr.dataset.id;
          this.log("warn", `\u624B\u52A8\u5F3A\u5236\u9009\u62E9 token ${id} ${JSON.stringify(shortTok(this.tok.decode([id])))}`);
          await this.stepOnce(id);
        };
      }
      $("entVal").textContent = dist.entropyBits.toFixed(2);
      $("pplVal").textContent = dist.ppl.toFixed(1);
      $("entFill").style.width = `${Math.min(100, dist.entropyBits / 12.6 * 100).toFixed(0)}%`;
      drawProbCurve($("probCurve"), dist.top, dist.pPost, lastRec?.chosen);
      $("sbProb").textContent = dist.top[0] ? `${(dist.top[0].pPost * 100).toFixed(1)}%` : "\u2014";
    }
    // ---------- chat transcript (user ⇄ assistant turns) ----------
    renderGenStream() {
      const gs = $("chatStream");
      const s = this.s;
      if (!s.ids.length || !s.turns.length) {
        gs.innerHTML = `<div class="msg system">\u5C1A\u672A\u5F00\u59CB \u2014\u2014 \u5728\u5DE6\u4FA7\u8F93\u5165\u5E76\u53D1\u9001\uFF0C\u6216\u70B9\u51FB\u300C\u9884\u586B\u5E76\u63A8\u7406\u9996 Token\u300D</div>`;
        return;
      }
      let recIdx = 0;
      let html = "";
      for (const turn of s.turns) {
        if (turn.role === "system") {
          html += `<div class="msg system" title="system prompt\uFF08\u6A21\u677F\u5316\u540E\u6CE8\u5165\u4E0A\u4E0B\u6587\u6700\u524D\u90E8\uFF09">${esc(turn.text)}</div>`;
        } else if (turn.role === "user") {
          html += `<div class="msg user"><span class="who">\u6211 \xB7 user</span>${esc(turn.text)}</div>`;
        } else if (turn.role === "assistant") {
          let inner = "";
          for (const id of turn.tokens || []) {
            const raw = this.tok.decode([id]);
            const rec = s.records[recIdx++];
            const cls = rec && rec.eos ? "g-eos" : "g-tok";
            const tip = rec ? `#${rec.pos} P=${(rec.pPostChosen * 100).toFixed(1)}% T=${rec.temperature}${rec.forced ? " \u624B\u52A8\u5F3A\u5236" : ""}` : "";
            inner += `<span class="${cls}" title="${tip}">${esc(raw)}</span>`;
          }
          html += `<div class="msg assistant"><span class="who">\u6A21\u578B \xB7 assistant\uFF08\u9010 Token \u91C7\u6837\uFF09</span>${inner || '<span style="color:var(--faint)">\uFF08\u7B49\u5F85\u751F\u6210\u2026\uFF09</span>'}<span class="caret" style="${s.busy ? "" : "display:none"}"></span></div>`;
        }
      }
      gs.innerHTML = html;
      gs.parentElement.scrollTop = gs.parentElement.scrollHeight;
    }
    // ---------- flow pulse & status ----------
    flowPulse(nodes) {
      this.flowActive = { nodes, t0: performance.now() };
      drawFlow($("flowCanvas"), this);
      clearTimeout(this._pulseT);
      this._pulseT = setTimeout(() => {
        this.flowActive = null;
        drawFlow($("flowCanvas"), this);
      }, 1200);
    }
    renderStatus() {
      const s = this.s;
      $("sbPos").textContent = s.ids.length;
      $("mToks").textContent = s.ids.length;
      const times = s.records.map((r) => r.ms).filter(Boolean);
      const avg = times.length ? times.reduce((a, b) => a + b, 0) / times.length : 0;
      $("mRate").textContent = avg ? (1e3 / avg).toFixed(1) : "\u2014";
      $("mCtx").textContent = `${(s.ids.length / s.ctxLimit * 100).toFixed(0)}%`;
      const capMem = s.captureBytes();
      $("mRAM").textContent = s.viz.enabled ? `${(capMem / 1048576).toFixed(1)}MB` : "\u5173\u95ED";
      $("btnStep").disabled = !s.canStep();
      $("btnAuto").disabled = !s.canStep();
      const MB = 1048576;
      const d = this.m.dModel, L = this.m.nLayers, KVH = this.m.nKvHeads, hd = this.m.headDim;
      const kvPerTok = 2 * L * KVH * hd * 4;
      const kvPre = this.m.maxCtx * kvPerTok;
      const kvAtLimit = s.ctxLimit * kvPerTok;
      const precInt8 = this.m.precision === "int8";
      const weightsMB = precInt8 ? 63912192 * 1.001 / MB : 63912192 * 4 / MB;
      const W = s.viz.window === Infinity ? s.ctxLimit : s.viz.window;
      const effW = Math.min(W, s.ctxLimit);
      const fixedPerTok = (3 * d + L * 5 * d) * 4;
      const capProj = s.viz.enabled ? effW * fixedPerTok + L * this.m.nHeads * (effW * s.ctxLimit - effW * effW / 2) * 4 : 0;
      const capNow = s.viz.enabled ? capMem : 0;
      $("memLines").innerHTML = `\u6743\u91CD\u5E38\u9A7B <b>${weightsMB.toFixed(0)}MB</b>\uFF08${precInt8 ? this.m.quantNative ? "INT8 \u5185\u5D4C\u539F\u751F" : "INT8\uFF0CFP32 \u526F\u672C\u4ECD\u5728 244MB \u53EF\u968F\u65F6\u5207\u56DE" : "FP32"}\uFF09${this.m.backend.kind.includes("webgl") ? " + GPU\u2248\u540C\u91CF" : ""} \xB7 KV Cache <b>${(kvAtLimit / MB).toFixed(1)}MB</b>\uFF08@ctx=${s.ctxLimit}\uFF0C\u9884\u5206\u914D ${(kvPre / MB).toFixed(0)}MB\uFF09<br>\u6355\u83B7\u6C60\u73B0\u7528 <b>${(capNow / MB).toFixed(1)}MB</b>\uFF08\u7A97\u53E3 ${s.viz.window === Infinity ? "\u5168\u90E8" : s.viz.window}\uFF09 \xB7 \u6EE1\u4E0A\u4E0B\u6587\u9884\u4F30 <b>${(capProj / MB).toFixed(0)}MB</b>${s.viz.enabled ? "" : "\uFF08\u6355\u83B7\u5DF2\u5173\u95ED\u21920\uFF09"}`;
      const warn = $("memWarn");
      warn.classList.remove("warn", "err");
      if (!s.viz.enabled) {
        warn.innerHTML = "\u2713 \u53EF\u89C6\u5316\u6355\u83B7\u5DF2\u5173\u95ED\uFF1A\u5185\u5B58\u53EA\u5269\u6743\u91CD + KV Cache\uFF0C\u53EF\u8DD1\u6EE1\u4E0A\u4E0B\u6587\uFF1B\u6563\u70B9/\u70ED\u529B\u56FE/\u6F14\u7B97\u53F0\u5C06\u65E0\u6570\u636E\u3002";
      } else if (capProj > 400 * MB) {
        warn.classList.add("err");
        warn.innerHTML = `\u26A0 \u9AD8\u5185\u5B58\u9884\u8B66\uFF1A\u6EE1\u4E0A\u4E0B\u6587\u6355\u83B7\u9884\u4F30 ${(capProj / MB).toFixed(0)}MB\uFF08>400MB\uFF09\u2014\u2014\u5EFA\u8BAE\u7F29\u5C0F\u53EF\u89C6\u5316\u7A97\u53E3\u6216\u5173\u95ED\u6355\u83B7\u3002`;
      } else if (capProj > 150 * MB) {
        warn.classList.add("warn");
        warn.innerHTML = `\u26A0 \u5185\u5B58\u63D0\u793A\uFF1A\u6EE1\u4E0A\u4E0B\u6587\u6355\u83B7\u9884\u4F30 ${(capProj / MB).toFixed(0)}MB\uFF08>150MB\uFF09\uFF0C\u4F4E\u914D\u8BBE\u5907\u5EFA\u8BAE\u7F29\u5C0F\u7A97\u53E3\u3002`;
      } else {
        warn.innerHTML = `\u2713 \u5185\u5B58\u5065\u5EB7\uFF1A\u6EE1\u4E0A\u4E0B\u6587\u6355\u83B7\u9884\u4F30 ${(capProj / MB).toFixed(0)}MB\uFF0CKV Cache ${(kvAtLimit / MB).toFixed(1)}MB\u3002`;
      }
    }
    drawFlow() {
      drawFlow($("flowCanvas"), this);
    }
  };
  function shortTok(t) {
    if (!t) return "";
    return t.replace(/\n/g, "\u23CE").replace(/\r/g, "\\r").replace(/\t/g, "\u21E5");
  }
  function isSpecialText(t) {
    return t.startsWith("<|") || t.startsWith("<think");
  }
  function pctStr(p) {
    const v = p * 100;
    return v >= 0.01 ? `${v.toFixed(2)}%` : v > 0 ? `${v.toExponential(1)}%` : "0%";
  }
  function norm(v) {
    let s = 0;
    for (let i = 0; i < v.length; i++) s += v[i] * v[i];
    return Math.sqrt(s);
  }
  function activateTab(name) {
    const btn = document.querySelector(`.tabbar button[data-tab="${name}"]`);
    if (btn) btn.click();
  }
  var tipEl = () => document.getElementById("tooltip");
  function showTip(ev, html) {
    const t = tipEl();
    t.innerHTML = html;
    t.style.display = "block";
    t.style.left = `${ev.clientX + 14}px`;
    t.style.top = `${ev.clientY + 12}px`;
  }
  function showTipAt(ev, html) {
    if (ev) showTip(ev, html);
  }
  function hideTip() {
    tipEl().style.display = "none";
  }
  function computePCA2(vecFor, nT, kind) {
    const vecs = [];
    for (let t = 0; t < nT; t++) {
      const v = vecFor(t);
      if (v) vecs.push({ t, v });
    }
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
    const cov2 = Float64Array.from(cov);
    for (let i = 0; i < d; i++) for (let j = 0; j < d; j++) cov2[i * d + j] -= p1[i] * p1[j] * dot64(p1, p1);
    const old = cov;
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
    const typ = kind === "q" ? "Q" : kind === "k" ? "K" : kind === "v" ? "V" : "E";
    return vecs.map(({ t }, i) => ({ x: proj1[i], y: proj2[i], type: typ, t }));
  }
  function norm64(v) {
    let s = 0;
    for (let i = 0; i < v.length; i++) s += v[i] * v[i];
    return Math.sqrt(s);
  }
  function dot64(a, b) {
    let s = 0;
    for (let i = 0; i < a.length; i++) s += a[i] * b[i];
    return s;
  }
  function drawSwiglu(cv, gate, up, act, contrib) {
    const { ctx, w, h } = fitCanvas(cv);
    drawGridBg(ctx, w, h);
    const n = Math.min(300, act.length);
    const step = act.length / n;
    const cols = [
      { data: gate, color: "rgba(53,224,255,.75)", label: "gate=ln2@W_g" },
      { data: up, color: "rgba(255,179,71,.75)", label: "up=ln2@W_u" },
      { data: act, color: "rgba(82,255,122,.8)", label: "silu(gate)\u2299up" }
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
        ctx.fillRect(ox + i * bw, v >= 0 ? bh / 2 - v * bh / 2 : bh / 2, Math.max(1, bw - 0.6), Math.abs(v) * bh / 2);
      }
      ctx.strokeStyle = C.line;
      ctx.beginPath();
      ctx.moveTo(ox, bh / 2 + 0.5);
      ctx.lineTo(ox + secW - 6, bh / 2 + 0.5);
      ctx.stroke();
      ctx.fillStyle = C.dim;
      ctx.font = "9.5px monospace";
      ctx.textAlign = "left";
      ctx.fillText(c0.label, ox + 2, h - 16);
      ctx.fillText(`max|\xB7|=${fmt(mAbs, 1)}`, ox + 2, h - 5);
    });
    if (contrib) {
      let nn = 0;
      for (let i = 0; i < contrib.length; i++) nn += contrib[i] * contrib[i];
      ctx.fillStyle = C.green;
      ctx.textAlign = "right";
      ctx.fillText(`\u2192 @W_down: \u2016out\u2016=${fmt(Math.sqrt(nn), 2)} (\u52A0\u56DE\u6B8B\u5DEE)`, w - 8, 12);
    }
  }

  // js/main.js
  var $2 = (id) => document.getElementById(id);
  var MODEL_DIR = "minimind-3";
  async function decodeEmbeddedWeights(onProgress) {
    const parts = window.__MM_B64 || [];
    if (!parts.length) throw new Error("\u672A\u627E\u5230\u5185\u5D4C\u6743\u91CD (window.__MM_B64)");
    const totalChars = parts.reduce((a, s) => a + s.length, 0);
    const totalBytes = Math.floor(totalChars / 4 * 3);
    const SLICE = 4 * 1024 * 1024;
    const out = new Uint8Array(totalBytes);
    let off = 0, done = 0;
    for (const s of parts) {
      for (let i = 0; i < s.length; i += SLICE) {
        const seg = s.slice(i, i + SLICE);
        const bin = atob(seg);
        const n = bin.length;
        for (let j = 0; j < n; j++) out[off + j] = bin.charCodeAt(j);
        off += n;
        done += seg.length;
        if (done / SLICE % 3 === 0) {
          onProgress(done / totalChars);
          await new Promise((r) => setTimeout(r, 0));
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
  async function loadResourcesEmbeddedQ(setMsg, setBar) {
    const parts = window.__MM_QPACK || [];
    if (!parts.length) throw new Error("\u672A\u627E\u5230\u5185\u5D4C INT8 \u6743\u91CD (window.__MM_QPACK)");
    const quantMap = /* @__PURE__ */ new Map();
    let done = 0;
    for (const p of parts) {
      const q = new Int8Array(b64ToBytes(p.data));
      const scales = new Float32Array(b64ToBytes(p.scales).buffer);
      quantMap.set(p.name, { q, scales, dIn: p.dIn, dOut: p.dOut });
      done++;
      setBar(done / parts.length * 0.85);
      setMsg(`\u89E3\u7801\u5185\u5D4C INT8 \u6743\u91CD ${done}/${parts.length}\uFF08${p.name.split(".").slice(-2).join(".")}\uFF09\u2026`);
      if (done % 8 === 0) await new Promise((r) => setTimeout(r, 0));
    }
    const weights = /* @__PURE__ */ new Map();
    for (const t of window.__MM_MISC || []) {
      weights.set(t.name, { dtype: "F32", shape: t.shape, data: new Float32Array(b64ToBytes(t.b64).buffer) });
    }
    setMsg("\u521D\u59CB\u5316 INT8 \u6743\u91CD\u5E03\u5C40\u2026");
    return { config: window.__MM_CONFIG, tokenizerJson: window.__MM_TOKJSON, quantMap, tensors: weights, source: "\u5185\u5D4C INT8 \u6743\u91CD\uFF08v2 \xB7 61MB \u5E38\u9A7B\uFF09" };
  }
  async function loadResourcesEmbedded(setMsg, setBar) {
    setMsg("\u89E3\u7801\u5185\u7F6E\u6743\u91CD JS\uFF08base64 \u2192 122MB \u4E8C\u8FDB\u5236\uFF09\u2026");
    const buf = await decodeEmbeddedWeights((f) => setBar(f * 0.9));
    setMsg("\u89E3\u6790 safetensors \u5E76\u89E3\u7801 FP16 \u2192 FP32\u2026");
    const { tensors } = parseSafetensors(buf);
    return {
      config: window.__MM_CONFIG,
      tokenizerJson: window.__MM_TOKJSON,
      tensors,
      source: "\u5185\u7F6E\u6743\u91CD\uFF08JS \u4EE3\u7801\u76F4\u5D4C\uFF0C\u65E0\u9700\u4EFB\u4F55\u6587\u4EF6\uFF09"
    };
  }
  async function loadResourcesHTTP(setMsg, setBar) {
    setMsg("\u8F7D\u5165\u6A21\u578B\u914D\u7F6E config.json\u2026");
    const config = await (await fetch(`${MODEL_DIR}/config.json`)).json();
    setMsg("\u8F7D\u5165\u5206\u8BCD\u5668 tokenizer.json\u2026");
    const tokenizerJson = await (await fetch(`${MODEL_DIR}/tokenizer.json`)).json();
    setMsg("\u4E0B\u8F7D\u6743\u91CD model.safetensors (122MB)\u2026");
    const { tensors } = await loadSafetensorsUrl(
      `${MODEL_DIR}/model.safetensors`,
      (f, got, total) => {
        setBar(f * 0.92);
        setMsg(`\u4E0B\u8F7D\u6743\u91CD\u2026 ${(got / 1048576).toFixed(0)}/${(total / 1048576).toFixed(0)} MB`);
      }
    );
    return { config, tokenizerJson, tensors, source: `HTTP \xB7 ${MODEL_DIR}/` };
  }
  function pickLocalFolder() {
    return new Promise((resolve, reject) => {
      const wrap = document.createElement("div");
      wrap.style.cssText = "display:flex;flex-direction:column;gap:12px;align-items:center;padding:18px 26px;border:1px dashed #0e7d99;background:#0b1622;max-width:600px;";
      wrap.innerHTML = `
      <div style="color:var(--cyan);font-family:var(--mono);font-size:13px;letter-spacing:.08em">\u68C0\u6D4B\u5230 file:// \u76F4\u63A5\u6253\u5F00</div>
      <div style="color:var(--txt);font-size:12.5px;line-height:1.9;text-align:left">
        \u6D4F\u89C8\u5668\u5B89\u5168\u7B56\u7565\u7981\u6B62\u9875\u9762\u7528 fetch \u8BFB\u53D6\u78C1\u76D8\u6587\u4EF6\uFF0C<br>
        \u8BF7<b style="color:var(--amber)">\u624B\u52A8\u6388\u6743\u6A21\u578B\u6587\u4EF6</b>\uFF08\u4EC5\u5728\u672C\u5730\u5185\u5B58\u8BFB\u53D6\uFF0C\u4E0D\u4F1A\u4E0A\u4F20\uFF09\uFF1A<br>
        \u65B9\u5F0F\u4E00\uFF1A\u9009\u62E9\u9879\u76EE\u6839\u76EE\u5F55\u4E0B\u7684 <code style="color:var(--cyan)">minimind-3</code> \u6574\u4E2A\u6587\u4EF6\u5939<br>
        \u65B9\u5F0F\u4E8C\uFF1A\u5206\u522B\u9009\u4E2D config.json / tokenizer.json / model.safetensors \u4E09\u4E2A\u6587\u4EF6<br>
        <span style="color:var(--dim)">(\u4E24\u8005\u4EFB\u9009\u5176\u4E00\uFF0C\u51D1\u9F50\u4E09\u4E2A\u6587\u4EF6\u5373\u53EF)</span>
      </div>`;
      const mkInput = (dir) => {
        const input = document.createElement("input");
        input.type = "file";
        if (dir) input.webkitdirectory = true;
        input.multiple = true;
        input.style.cssText = "color:var(--dim);font-family:var(--mono);font-size:11px;";
        return input;
      };
      const inDir = mkInput(true);
      const inFiles = mkInput(false);
      const err = document.createElement("div");
      err.style.cssText = "color:var(--red);font-size:11px;font-family:var(--mono);min-height:14px;";
      const lbl1 = document.createElement("div");
      lbl1.style.cssText = "color:var(--dim);font-size:11px;font-family:var(--mono);";
      lbl1.textContent = "\u2460 \u9009\u62E9\u6587\u4EF6\u5939\uFF1A";
      const lbl2 = document.createElement("div");
      lbl2.style.cssText = "color:var(--dim);font-size:11px;font-family:var(--mono);margin-top:6px;";
      lbl2.textContent = "\u2461 \u6216\u9010\u4E2A\u9009\u62E9\u6587\u4EF6\uFF1A";
      const tryResolve = () => {
        const files = [...Array.from(inDir.files || []), ...Array.from(inFiles.files || [])];
        const need = ["config.json", "tokenizer.json", "model.safetensors"];
        const missing = need.filter((n) => !files.some((f) => f.name === n));
        if (missing.length) {
          err.textContent = `\u8FD8\u7F3A\u5C11: ${missing.join(" / ")}`;
          return;
        }
        resolve(files);
        wrap.remove();
      };
      inDir.onchange = tryResolve;
      inFiles.onchange = tryResolve;
      const cancel = document.createElement("button");
      cancel.textContent = "\u53D6\u6D88\uFF08\u6539\u7528 HTTP\uFF1A\u5148\u8FD0\u884C ./start.sh\uFF09";
      cancel.onclick = () => {
        wrap.remove();
        reject(new Error("\u5DF2\u53D6\u6D88\u672C\u5730\u6A21\u5F0F"));
      };
      wrap.appendChild(lbl1);
      wrap.appendChild(inDir);
      wrap.appendChild(lbl2);
      wrap.appendChild(inFiles);
      wrap.appendChild(cancel);
      wrap.appendChild(err);
      $2("loadMsg").after(wrap);
    });
  }
  async function readLocalResources(files, setMsg, setBar) {
    const byName = new Map(files.map((f) => [f.name, f]));
    setMsg("\u8BFB\u53D6 config.json / tokenizer.json\u2026");
    const config = JSON.parse(await byName.get("config.json").text());
    const tokenizerJson = JSON.parse(await byName.get("tokenizer.json").text());
    setBar(0.25);
    setMsg("\u8BFB\u53D6 model.safetensors (122MB)\u2026");
    const buf = await byName.get("model.safetensors").arrayBuffer();
    setBar(0.85);
    setMsg("\u89E3\u6790 safetensors \u5934\u5E76\u89E3\u7801 FP16 \u2192 FP32\u2026");
    const { tensors } = parseSafetensors(buf);
    return { config, tokenizerJson, tensors, source: "\u672C\u5730\u6587\u4EF6\u5939 \xB7 file://" };
  }
  function logLine(kind, msg) {
    const box = $2("logConsole");
    const t = (/* @__PURE__ */ new Date()).toTimeString().slice(0, 8);
    const el = document.createElement("div");
    el.className = `log-line ${kind}`;
    el.innerHTML = `<span class="t">[${t}]</span> ${kind === "info" ? `<b>${esc(msg)}</b>` : esc(msg)}`;
    box.appendChild(el);
    box.scrollTop = box.scrollHeight;
    while (box.childElementCount > 240) box.firstChild.remove();
  }
  async function boot() {
    const setMsg = (s) => {
      $2("loadMsg").textContent = s;
    };
    const setBar = (f) => {
      $2("loadBarFill").style.width = `${(f * 100).toFixed(1)}%`;
    };
    try {
      const t0 = performance.now();
      let res;
      if (window.__MM_QPACK && window.__MM_QPACK.length && window.__MM_CONFIG && window.__MM_TOKJSON) {
        res = await loadResourcesEmbeddedQ(setMsg, setBar);
      } else if (window.__MM_B64 && window.__MM_B64.length && window.__MM_CONFIG && window.__MM_TOKJSON) {
        res = await loadResourcesEmbedded(setMsg, setBar);
      } else if (location.protocol === "file:") {
        const files = await pickLocalFolder();
        res = await readLocalResources(files, setMsg, setBar);
      } else {
        res = await loadResourcesHTTP(setMsg, setBar);
      }
      const { config, tokenizerJson, tensors, source } = res;
      const tok = new BPETokenizer(tokenizerJson);
      logLine("ok", `\u5206\u8BCD\u5668\u5C31\u7EEA\uFF1Avocab=${tok.vocabSize}\uFF0Cmerges=${tok.ranks.size}\uFF0Cspecial=${tok.special.length}`);
      const dlMs = performance.now() - t0;
      setMsg("\u521D\u59CB\u5316\u8BA1\u7B97\u540E\u7AEF\u2026");
      await new Promise((r) => setTimeout(r, 30));
      logLine("ok", `\u6743\u91CD\u89E3\u6790\u5B8C\u6210\uFF1A${res.quantMap ? res.quantMap.size + " \u4E2A INT8 \u5F20\u91CF\uFF08INT8 \u539F\u751F \xB7 \u5E38\u9A7B 61MB\uFF09" : tensors.size + " \u5F20\u91CF"}\uFF0C\u52A0\u8F7D ${(dlMs / 1e3).toFixed(1)}s\uFF08\u6765\u6E90\uFF1A${source}\uFF09`);
      let backend;
      try {
        backend = await createBackend("auto", (s) => logLine("info", s));
      } catch (e) {
        backend = await createBackend("js", (s) => logLine("info", s));
      }
      await backend.warmup?.();
      const model = new Qwen3Model({ weights: tensors, config, backend, maxCtx: 2048, quantMap: res.quantMap || null, log: (s) => logLine("info", s) });
      $2("tbBackend").textContent = backend.kind;
      $2("tbLoadTime").textContent = `${((performance.now() - t0) / 1e3).toFixed(1)}s`;
      logLine("ok", `\u6A21\u578B\u5C31\u7EEA\uFF1A${config.num_hidden_layers} \u5C42 \xD7 ${config.num_attention_heads}Q/${config.num_key_value_heads}KV \u5934\uFF0Cd=${config.hidden_size}\uFF0C\u03B8=${config.rope_theta}`);
      const session = new Session(model, tok);
      const ui = new UI(session, tok, model, logLine);
      window._DSH_UI = ui;
      ui.updateTokCount();
      ui.drawFlow();
      const loop = () => {
        drawFlow($2("flowCanvas"), ui);
        requestAnimationFrame(loop);
      };
      requestAnimationFrame(loop);
      setBar(1);
      logLine("info", "\u63D0\u793A\uFF1A\u2460 \u5DE6\u4FA7\u9009\u62E9\u6A21\u5F0F\u4E0E\u8F93\u5165 \u2192 \u300C\u9884\u586B\u5E76\u63A8\u7406\u9996 Token\u300D\uFF08\u9ED8\u8BA4\u8FDE\u7EED\u8F93\u51FA\uFF09\uFF1B\u2461 \u53F3\u4E0B\u5BF9\u8BDD\u6846\u53EF\u8FFD\u95EE\u591A\u8F6E\uFF1B\u2462 \u5173\u95ED\u300C\u8FDE\u7EED\u8F93\u51FA\u300D\u540E\u7528\u300C\u5355\u6B65 +1 Token\u300D\u9010 Token \u89C2\u5BDF\uFF1B\u2463 \u9876\u680F\u300C\u{1F4D6} \u539F\u7406\u6587\u6863\u300D\u5185\u5D4C\u5168\u6587\u3002");
      logLine("info", "\u70B9\u51FB\u53F3\u4E0A\u300C\u{1F4D6} \u539F\u7406\u6587\u6863\u300D\u9605\u8BFB 8000 \u5B57\u8BE6\u89E3\u3002");
      setTimeout(() => {
        $2("loadOverlay").style.opacity = "0";
        $2("loadOverlay").style.transition = "opacity .4s";
        setTimeout(() => $2("loadOverlay").remove(), 450);
      }, 250);
      window.addEventListener("error", (ev) => logLine("err", `JS\u9519\u8BEF: ${ev.message}`));
      if (new URLSearchParams(location.search).has("autotest")) {
        (async () => {
          try {
            logLine("info", "[AUTOTEST] \u9884\u586B\u9ED8\u8BA4\u8F93\u5165\u2026");
            await ui.runPrefill();
            logLine("info", `[AUTOTEST] prefill ok, ctx=${session.ids.length}`);
            for (let k = 0; k < 6; k++) await ui.stepOnce();
            logLine("info", `[AUTOTEST] 6 steps ok, ctx=${session.ids.length}`);
            await ui.selfTest();
            await session.appendUserTurn("\u4F60\u53EB\u4EC0\u4E48\u540D\u5B57\uFF1F");
            for (let k = 0; k < 3; k++) await ui.stepOnce();
            logLine("info", `[AUTOTEST] follow-up turn ok, ctx=${session.ids.length}, turns=${session.turns.length}`);
            const pool = session.capPool;
            const row = pool.layers[7].attnRows[session.ids.length - 1][3];
            const sum = row.reduce((a, b) => a + b, 0);
            logLine("info", `[AUTOTEST] attnRow(L7,H3,last) sum=${sum.toFixed(6)} len=${row.length}`);
            const turnsOk = session.turns.filter((t) => t.role === "user").length === 2;
            document.title = Math.abs(sum - 1) < 1e-4 && turnsOk ? "AUTOTEST_OK" : "AUTOTEST_FAIL_ATTN";
          } catch (e) {
            console.error(e);
            logLine("err", `[AUTOTEST] \u5931\u8D25: ${e.message}`);
            document.title = "AUTOTEST_FAIL";
          }
        })();
      }
    } catch (e) {
      console.error(e);
      setMsg("\u52A0\u8F7D\u5931\u8D25\uFF1A" + e.message);
      $2("loadOverlay").querySelector(".spin")?.remove();
      logLine("err", `\u52A0\u8F7D\u5931\u8D25: ${e.message}`);
      const ov = $2("loadOverlay");
      let hint = ov.querySelector(".hint");
      if (!hint) {
        hint = document.createElement("div");
        hint.className = "lsys hint";
        hint.innerHTML = `\u8BF7\u786E\u8BA4\u901A\u8FC7 HTTP \u670D\u52A1\u5668\u8BBF\u95EE\u672C\u9875\u9762\uFF08\u4F8B\u5982 <b>./start.sh</b> \u6216 <b>python3 -m http.server</b>\uFF09\uFF0C<br>\u4E14 minimind-3/ \u76EE\u5F55\u4E0E index.html \u540C\u7EA7\u3002<br><br>\u9519\u8BEF\u8BE6\u60C5: ${esc(e.message)}`;
        ov.appendChild(hint);
      }
    }
  }
  boot();
})();
