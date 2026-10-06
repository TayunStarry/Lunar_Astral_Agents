// mat.js — pluggable linear-algebra backends used by the model.
import { dequantInt8, transposeInt8, linearJSInt8 } from './quant.js';
//
//   linear(x /*[T,dIn]*/, Wname) -> Promise<Float32Array /*[T,dOut]*/>
//
// Two implementations:
//   - js : pure typed-array kernel (used by Node parity tests & CPU fallback)
//   - tf : TensorFlow.js matMul (WebGL/WebGPU accelerated in the browser)

const WEIGHT_CACHE = new WeakMap(); // model weights Map -> per-backend scratch

export async function createBackend(kind, log = () => {}) {
  if (kind === 'js') return makeJsBackend(log);
  // tf / auto
  if (typeof tf !== 'undefined') {
    try {
      return await makeTfBackend(log);
    } catch (e) {
      log(`TF.js backend unavailable (${e.message}); falling back to pure-JS kernels.`);
      return makeJsBackend(log);
    }
  }
  return makeJsBackend(log);
}

// ---------- pure JS ----------

function transposed(wData, dIn, dOut) {
  // W is [dOut, dIn] row-major from safetensors -> return [dIn, dOut]
  const wt = new Float32Array(dIn * dOut);
  for (let o = 0; o < dOut; o++) {
    const rowOff = o * dIn;
    for (let i = 0; i < dIn; i++) {
      wt[i * dOut + o] = wData[rowOff + i];
    }
  }
  return wt;
}

/** Column-accumulating GEMM: y[t,o] = Σ_i x[t,i] * Wt[i,o]. Sequential-friendly. */
export function linearJS(x, T, dIn, Wt, dOut, out) {
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
        out[yr + o]     += xi * Wt[wb + o];
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
  const cache = new Map(); // name -> {wt, dIn, dOut}
  const qcache = new Map(); // name -> {wt:int8, scales}
  return {
    kind: 'js',
    async warmup(weights, layerShapes) {
      // layerShapes ignored; transpose happens lazily on first use
      log('backend=js ready (pure typed-array kernels)');
    },
    async linear(x, T, spec, weights, reuseBuf) {
      // spec: {name, dIn, dOut}
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
    dispose() { cache.clear(); qcache.clear(); },
  };
}

// ---------- TF.js ----------

async function makeTfBackend(log) {
  if (typeof tf === 'undefined') throw new Error('tfjs not loaded');
  try { await tf.setBackend('webgl'); } catch (_) { /* keep current */ }
  await tf.ready();
  const be = tf.getBackend();
  const tensors = new Map(); // name -> tf tensor [dIn, dOut]
  const qtensors = new Map(); // name -> dequantized-from-int8 tf tensor

  return {
    kind: `tf:${be}`,
    async warmup() {
      log(`backend=${this.kind} webgl=${be}`);
    },
    async linearQ(x, T, spec, quantMap) {
      let wt = qtensors.get(spec.name);
      if (!wt) {
        const e = quantMap.get(spec.name);
        // WebGL has no int8 GEMM: simulate by dequantizing to FP32 on GPU.
        const wq = dequantInt8(e.q, e.scales, spec.dOut, spec.dIn);
        wt = tf.tensor2d(wq, [spec.dOut, spec.dIn]).transpose();
        qtensors.set(spec.name, wt);
      }
      const xt = tf.tensor2d(x, [T, spec.dIn]);
      const yt = tf.matMul(xt, wt);
      const arr = await yt.data();
      xt.dispose(); yt.dispose();
      return arr instanceof Float32Array ? arr : new Float32Array(arr);
    },
    async linear(x, T, spec, weights, reuseBuf) {
      let wt = tensors.get(spec.name);
      if (!wt) {
        const w = weights.get(spec.name);
        // store transposed [dIn, dOut] once so matMul(x[T,dIn]) yields [T,dOut]
        wt = tf.tensor2d(w.data, [spec.dOut, spec.dIn]).transpose();
        tensors.set(spec.name, wt);
      }
      const xt = tf.tensor2d(x, [T, spec.dIn]);
      const yt = tf.matMul(xt, wt);           // [T, dOut]
      const arr = await yt.data();
      xt.dispose(); yt.dispose();
      return arr instanceof Float32Array ? arr : new Float32Array(arr);
    },
    dispose() {
      for (const t of tensors.values()) t.dispose();
      for (const t of qtensors.values()) t.dispose();
      tensors.clear(); qtensors.clear();
    },
  };
}
