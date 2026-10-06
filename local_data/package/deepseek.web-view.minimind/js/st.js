// st.js — Safetensors reader with F16/BF16 → F32 decoding.
// Zero-dependency, works in browser (fetch) and Node (caller passes ArrayBuffer).

/** Decode one IEEE-754 half-precision float. */
export function f16ToF32(h) {
  const s = (h & 0x8000) >> 15;
  const e = (h & 0x7c00) >> 10;
  const m = h & 0x03ff;
  let out;
  if (e === 0) {
    out = (m / 1024) * Math.pow(2, -14); // subnormal: m/1024 * 2^-14
  } else if (e === 31) {
    out = m !== 0 ? NaN : Infinity;
  } else {
    out = (1 + m / 1024) * Math.pow(2, e - 15);
  }
  return s ? -out : out;
}

/** BF16 (upper 16 bits of an F32) → F32. */
export function bf16ToF32(h) {
  return (h << 16);
}

export function product(shape) {
  let n = 1;
  for (let i = 0; i < shape.length; i++) n *= shape[i];
  return n;
}

const DT_SIZES = { F64: 8, F32: 4, F16: 2, BF16: 2, I64: 8, I32: 4, I16: 2, I8: 1, U8: 1, BOOL: 1 };

/**
 * Parse a safetensors file from an ArrayBuffer.
 * Returns Map name -> { dtype, shape, data: Float32Array | Int32Array }.
 * All floats are materialized as F32 for uniform downstream math.
 */
export function parseSafetensors(buffer) {
  const dv = new DataView(buffer);
  // u64 little-endian header length
  const lo = dv.getUint32(0, true);
  const hi = dv.getUint32(4, true);
  if (hi !== 0) throw new Error('safetensors too large');
  const headerLen = lo;
  const headerBytes = new Uint8Array(buffer, 8, headerLen);
  const headerText = new TextDecoder('utf-8').decode(headerBytes);
  const header = JSON.parse(headerText);
  const meta = header.__metadata__ || null;
  delete header.__metadata__;
  const base = 8 + headerLen;
  const tensors = new Map();
  const bytes = new Uint8Array(buffer);

  for (const name of Object.keys(header)) {
    const rec = header[name];
    const [s, e] = rec.data_offsets;
    const numel = rec.shape.length ? rec.shape.reduce((a, b) => a * b, 1) : 1;
    const off = base + s;
    let data;
    switch (rec.dtype) {
      case 'F32': {
        data = new Float32Array(numel);
        const src = new Float32Array(buffer, off, numel); // aligned view
        data.set(src);
        break;
      }
      case 'F64': {
        const src = new Float64Array(buffer, off, numel);
        data = new Float32Array(numel);
        for (let i = 0; i < numel; i++) data[i] = src[i];
        break;
      }
      case 'F16': {
        data = new Float32Array(numel);
        for (let i = 0; i < numel; i++) {
          data[i] = f16ToF32(dv.getUint16(off + i * 2, true));
        }
        break;
      }
      case 'BF16': {
        data = new Float32Array(numel);
        for (let i = 0; i < numel; i++) {
          data[i] = bf16ToF32(dv.getUint16(off + i * 2, true));
        }
        break;
      }
      case 'I32':
      case 'I16':
      case 'I64': {
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

/** Browser helper: fetch + parse. Node callers read the file themselves. */
export async function loadSafetensorsUrl(url, onProgress) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`HTTP ${res.status} fetching ${url}`);
  const total = Number(res.headers.get('content-length') || 0);
  let buf;
  if (res.body && typeof res.body.pipeThrough === 'function' && total > 0) {
    const reader = res.body.getReader();
    const chunks = [];
    let got = 0;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
      got += value.byteLength;
      if (onProgress && total) onProgress(got / total, got, total);
    }
    buf = new Uint8Array(got);
    let p = 0;
    for (const c of chunks) { buf.set(c, p); p += c.byteLength; }
    buf = buf.buffer;
  } else {
    buf = await res.arrayBuffer();
    if (onProgress) onProgress(1, buf.byteLength, buf.byteLength);
  }
  return parseSafetensors(buf);
}
