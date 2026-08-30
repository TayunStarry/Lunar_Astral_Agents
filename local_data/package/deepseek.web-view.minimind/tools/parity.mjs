#!/usr/bin/env node
// Node-side parity checks: tokenizer ids vs HF dump; model forward vs HF logits.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { BPETokenizer } from '../js/tok.js';
import { parseSafetensors } from '../js/st.js';
import { Qwen3Model } from '../js/model.js';
import { createBackend } from '../js/mat.js';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

let fails = 0;
function assert(cond, msg) {
  if (!cond) { console.error('  FAIL:', msg); fails++; }
}

async function checkTokenizer(tok) {
  const cases = JSON.parse(readFileSync(path.join(ROOT, 'tools/tok_cases.json'), 'utf8'));
  let ok = 0;
  for (const c of cases) {
    const got = tok.encode(c.text);
    if (JSON.stringify(got) === JSON.stringify(c.ids)) { ok++; continue; }
    fails++;
    console.error(`TOK MISMATCH ${JSON.stringify(c.text)}`);
    console.error('  want:', c.ids);
    console.error('  got :', got);
  }
  console.log(`tokenizer: ${ok}/${cases.length} cases match`);
  return ok === cases.length;
}

function maxAbsDiff(a, b, stride = 1) {
  let m = 0;
  const n = Math.min(a.length, b.length);
  for (let i = 0; i < n; i += stride) m = Math.max(m, Math.abs(a[i] - b[i]));
  return m;
}

async function checkModel(tok) {
  const buf = readFileSync(path.join(ROOT, 'minimind-3/model.safetensors'));
  const bytes = buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  const t0 = performance.now ? performance.now() : Date.now();
  const parsed = parseSafetensors(bytes);
  const dt = (performance.now ? performance.now() : Date.now()) - t0;
  console.log(`safetensors: ${parsed.tensors.size} tensors in ${dt.toFixed(0)}ms`);

  const config = JSON.parse(readFileSync(path.join(ROOT, 'minimind-3/config.json'), 'utf8'));
  const backend = await createBackend('js');
  const model = new Qwen3Model({ weights: parsed.tensors, config, backend, maxCtx: 256 });

  const ref = JSON.parse(readFileSync(path.join(ROOT, 'tools/ref_sample.json'), 'utf8'));

  for (const entry of ref.prompts) {
    const ids = tok.encode(entry.prompt);
    assert(JSON.stringify(ids) === JSON.stringify(entry.ids),
           `prompt ids differ: ${entry.prompt}`);
    const { logitsLast, capture } = await model.forward(ids, { capture: 'full' });

    const gotTop = Array.from(logitsLast)
      .map((v, i) => [i, v])
      .sort((a, b) => b[1] - a[1]).slice(0, 5);
    const wantTop = entry.topk_ids.slice(0, 5);
    const topOk = wantTop.every((wid, k) => gotTop[k][0] === wid);
    console.log(`\nprompt: "${entry.prompt}"`);
    console.log(`  logits max|Δ| last-pos = ${maxAbsDiff(logitsLast, new Float32Array(entry.logits_last)).toFixed(6)}`);
    console.log(`  top-5 got:  ${gotTop.map(([i, v]) => `${i}(${v.toFixed(3)})`).join(' ')}`);
    console.log(`  top-5 want: ${wantTop.map((id, k) => `${id}(${(entry.topk_probs[k]).toFixed(3)})`).join(' ')}`);
    assert(topOk, `top-5 ids mismatch`);

    // hidden-state snapshots after each block, token index len//2.
    // NOTE: transformers v5 appends the POST-FINAL-NORM state as the last
    // hidden_states entry, so its last snapshot equals our capture.finalNorm.
    const midIdx = Math.floor(entry.ids.length / 2);
    for (let l = 0; l < 7; l++) {
      const lc = capture.layers[l];
      const off = midIdx * model.dModel;
      const mine = lc.hiddenAfterLayer.subarray(off, off + model.dModel);
      const wantS = new Float32Array(entry.hidden_snapshots_tok2[l + 1]); // 0 = embedding
      const dmax = maxAbsDiff(mine, wantS);
      if (dmax > 3e-3) {
        fails++;
        console.error(`  hidden mismatch layer ${l}: maxΔ=${dmax.toExponential(2)}`);
      }
    }
    {
      const off = midIdx * model.dModel;
      const mine = capture.finalNorm.subarray(off, off + model.dModel);
      const wantS = new Float32Array(entry.hidden_snapshots_tok2[8]);
      const dmax = maxAbsDiff(mine, wantS);
      console.log(`  final-norm snapshot max|Δ| = ${dmax.toExponential(2)}`);
      assert(dmax < 3e-3, 'final normed hidden differs');
    }
    // attention row compare (layer2 head5, query mid+1 — absolute positions match prefill)
    const ar = entry.attn_row_L2H5_qmid;
    const rowMine = capture.layers[ar.layer].attnRows[ar.head][ar.query - capture.startPos];
    const dAttn = maxAbsDiff(rowMine, new Float32Array(ar.weights));
    console.log(`  attn L2H5 q=${ar.query} max|Δw| = ${dAttn.toExponential(2)} (sum=${rowMine.reduce((a,b)=>a+b,0).toFixed(6)})`);
    assert(dAttn < 2e-4, 'attention weights differ too much');

    // reset for next prompt
    model.reset();
  }

  // ---- INT8 weight-only quantization accuracy ----
  {
    const e = ref.prompts[1];
    const ids = tok.encode(e.prompt);
    model.reset();
    model.setPrecision('int8');
    const st = model.quantStats;
    console.log(`\nINT8 量化: ${st.tensors} tensors, ${(st.fp32Bytes / 1048576).toFixed(0)}MB → ${(st.int8Bytes / 1048576).toFixed(1)}MB (${st.compression.toFixed(2)}×), 权重 maxErr=${st.maxErr.toExponential(2)}`);
    const ri = await model.forward(ids, { capture: 'none' });
    const li = Array.from(ri.logitsLast);
    const lf = new Float32Array(e.logits_last);
    let dq = 0; let topI = 0, topF = 0;
    for (let i = 0; i < li.length; i++) {
      if (Math.abs(li[i] - lf[i]) > dq) dq = Math.abs(li[i] - lf[i]);
      if (li[i] > li[topI]) topI = i;
      if (lf[i] > lf[topF]) topF = i;
    }
    const t5f = new Set(Array.from(lf).map((v, i) => [i, v]).sort((a, b) => b[1] - a[1]).slice(0, 5).map(x => x[0]));
    const t5i = Array.from(li).map((v, i) => [i, v]).sort((a, b) => b[1] - a[1]).slice(0, 5);
    const overlap = t5i.filter(x => t5f.has(x[0])).length;
    console.log(`  FP32↔INT8 max|Δlogits| = ${dq.toExponential(2)}, Top-1 ${topI === topF ? '一致' : topF + '→' + topI}, Top-5 重叠 ${overlap}/5`);
    // 实测标定：W8A16 逐通道量化 max|Δlogits|≈0.44（量程±15），Top-5 5/5
    assert(dq < 0.6, 'INT8 quantization error too large');
    assert(overlap >= 4, 'INT8 top-5 overlap < 4/5');
    model.setPrecision('fp32');
  }

  // ---- KV-cache incremental decode equivalence ----
  const e = ref.prompts[1];
  const ids = tok.encode(e.prompt);
  model.reset();
  const rA = await model.forward(ids, { capture: 'none' });
  const la = Array.from(rA.logitsLast);
  model.reset();
  const rb = await model.forward(ids.slice(0, -1), { capture: 'none' });
  const rc = await model.forward([ids[ids.length - 1]], { capture: 'full' });
  const lb = Array.from(rc.logitsLast);
  const dCache = maxAbsDiff(new Float32Array(la), new Float32Array(lb));
  console.log(`\nKV-cache step equivalence (${e.prompt}):`);
  console.log(`  one-shot vs cached-step logits max|Δ| = ${dCache.toExponential(2)}`);
  assert(dCache < 5e-5, 'cached decode diverges from one-shot prefill');
  const row = rc.capture.layers[2].attnRows[5][0];
  console.log(`  stepped attn row len=${row.length} sum=${row.reduce((a,b)=>a+b,0).toFixed(7)} (T=${ids.length})`);
  assert(row.length === ids.length && Math.abs(row.reduce((a,b)=>a+b,0) - 1) < 1e-6,
         'stepped attention row malformed');
}

// ---------- main ----------
const tj = JSON.parse(readFileSync(path.join(ROOT, 'minimind-3/tokenizer.json'), 'utf8'));
const tok = new BPETokenizer(tj);
await checkTokenizer(tok);
await checkModel(tok);
console.log(fails === 0 ? '\nALL PARITY CHECKS PASSED' : `\n${fails} FAILURES`);
process.exit(fails ? 1 : 0);
