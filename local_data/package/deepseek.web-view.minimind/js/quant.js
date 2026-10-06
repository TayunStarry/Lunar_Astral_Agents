// quant.js — per-output-channel symmetric INT8 weight quantization (W8A16).
//
//   scale[o] = max|W[o,:]| / 127
//   q[o,i]   = round(W[o,i] / scale[o])   ∈ [-127, 127]
//   W'[o,i]  = q[o,i] × scale[o]
//
// Activations stay FP32; matmul accumulates in FP32/FP64 and multiplies the
// per-channel scale once at the end. All large weight matrices are quantized:
// q/k/v/o/gate/up/down per layer + tied embedding (= LM head).

export function quantizeInt8PerRow(data, dOut, dIn) {
  const q = new Int8Array(dOut * dIn);
  const scales = new Float32Array(dOut);
  let maxErr = 0; // max |W - W'|
  for (let o = 0; o < dOut; o++) {
    const off = o * dIn;
    let maxAbs = 0;
    for (let i = 0; i < dIn; i++) { const a = Math.abs(data[off + i]); if (a > maxAbs) maxAbs = a; }
    const s = maxAbs / 127 || 1e-12;
    scales[o] = s;
    const inv = 1 / s;
    for (let i = 0; i < dIn; i++) {
      let v = Math.round(data[off + i] * inv);
      if (v > 127) v = 127; else if (v < -127) v = -127;
      q[off + i] = v;
      const err = Math.abs(v * s - data[off + i]);
      if (err > maxErr) maxErr = err;
    }
  }
  return { q, scales, maxErr };
}

export function dequantInt8(q, scales, dOut, dIn) {
  const out = new Float32Array(dOut * dIn);
  for (let o = 0; o < dOut; o++) {
    const s = scales[o], off = o * dIn;
    for (let i = 0; i < dIn; i++) out[off + i] = q[off + i] * s;
  }
  return out;
}

/** transpose int8 [dOut,dIn] row-major -> [dIn,dOut] for the column kernel */
export function transposeInt8(q, dIn, dOut) {
  const t = new Int8Array(dIn * dOut);
  for (let o = 0; o < dOut; o++) {
    const ro = o * dIn;
    for (let i = 0; i < dIn; i++) t[i * dOut + o] = q[ro + i];
  }
  return t;
}

/**
 * W8A16 GEMM: y[t,o] = scales[o] × Σ_i x[t,i] × Wt[i,o],  Wt = transposed int8.
 * Accumulation in FP64 per output channel (cheap at these sizes, avoids drift).
 */
export function linearJSInt8(x, T, dIn, Wt, scales, dOut, out) {
  if (!out) out = new Float32Array(T * dOut); else out.fill(0);
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
