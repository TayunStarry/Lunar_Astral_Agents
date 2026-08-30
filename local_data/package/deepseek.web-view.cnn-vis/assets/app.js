/* =========================================================================
 * app.js — CNN 手写数字识别演示 · 交互与可视化
 * 依赖: engine.js, viz3d.js, model_data.js, samples.js (均本地无外部依赖)
 * ========================================================================= */
'use strict';

/* ---------------- 小工具 ---------------- */
const $ = id => document.getElementById(id);
const clamp = (v, a, b) => v < a ? a : v > b ? b : v;
const f2 = v => (v >= 0 ? '+' : '') + v.toFixed(2);

function log(msg, cls) {
  const box = $('consoleLog');
  const d = document.createElement('div');
  d.textContent = timeStr() + '  ' + msg;
  if (cls) d.className = cls;
  box.appendChild(d);
  while (box.children.length > 400) box.removeChild(box.firstChild);
  box.scrollTop = 999999;
}
function timeStr() {
  const t = new Date();
  const p = n => String(n).padStart(2, '0');
  return `${p(t.getHours())}:${p(t.getMinutes())}:${p(t.getSeconds())}`;
}

/* 灰度纹理: Float32Array -> canvas (按 min-max 归一化) */
function makeTex(arr, w, h) {
  const c = document.createElement('canvas');
  c.width = w; c.height = h;
  const ctx = c.getContext('2d');
  const img = ctx.createImageData(w, h);
  let mn = Infinity, mx = -Infinity;
  for (let i = 0; i < arr.length; i++) { if (arr[i] < mn) mn = arr[i]; if (arr[i] > mx) mx = arr[i]; }
  const span = mx - mn || 1;
  for (let i = 0; i < arr.length; i++) {
    const g = Math.round(((arr[i] - mn) / span) * 255);
    img.data[i * 4] = Math.round(18 + g * 0.82);
    img.data[i * 4 + 1] = Math.round(24 + g * 0.88);
    img.data[i * 4 + 2] = Math.round(34 + g * 0.92);
    img.data[i * 4 + 3] = 255;
  }
  ctx.putImageData(img, 0, 0);
  return c;
}

/* 把单张特征图画到指定 canvas (自动放大, 像素风) */
function drawMap(cv, arr, w, h, cssSize) {
  cv.width = w; cv.height = h;
  if (cssSize) { cv.style.width = cssSize + 'px'; cv.style.height = cssSize + 'px'; }
  const ctx = cv.getContext('2d');
  const tex = makeTex(arr, w, h);
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(tex, 0, 0);
}

/* 全局状态 */
const Engine = window.CNNEngine;
const W = {};                 // 权重段
let engine = null;
let PAD = new Float32Array(784);      // 手绘缓冲 [0,1]
let SNAP = null;                      // 最近一次推理的输入 (预处理后)
let LAST = null;                      // 最近一次推理的全部中间量
let SEL = { conv1: 0, conv2: 0 };     // 选中的卷积核索引
let GAL = {};                         // 缩略图 canvas 引用
let viz = null;

/* =========================================================================
 * 启动流程
 * ========================================================================= */
function boot() {
  const t0 = performance.now();
  try {
    const all = Engine.loadWeights(window.CNN_WEIGHTS_B64);
    Object.assign(W, all);
    engine = Engine.makeEngine(all);
    $('ledModel').className = 'led ok';
    $('metaParams').textContent =
      `${window.CNN_WEIGHTS_META.params.toLocaleString()} params · FP32`;
    log(`权重加载完成 (${(performance.now() - t0).toFixed(1)} ms) · 来源 ${window.CNN_WEIGHTS_META.source}`);
  } catch (e) {
    $('ledModel').className = 'led err';
    log('权重加载失败: ' + e.message, 'err');
    return;
  }

  // 引擎自检
  const probe = new Float32Array(784);          // 全零输入走一遍
  const po = engine.forwardNorm(probe);
  const s = po.probs.reduce((a, b) => a + b, 0);
  if (Math.abs(s - 1) < 1e-5 && po.conv1.length === 25088) {
    $('ledEngine').className = 'led ok';
    log(`引擎自检通过 · softmax Σ=${s.toFixed(6)} · 引擎 v${Engine.VERSION}`);
  } else {
    $('ledEngine').className = 'led err';
    log('引擎自检失败', 'err');
  }

  initPad();
  initStations();
  initViz();
  initMac();
  wireControls();
  startClock();

  // 自动载入示例 "7" 并跑一次, 让页面打开即有内容
  loadSample(7, true);
}

function startClock() {
  const el = $('clock');
  setInterval(() => { el.textContent = new Date().toLocaleTimeString('zh-CN', { hour12: false }); }, 1000);
}

/* =========================================================================
 * 手绘板 (28×28 逻辑网格)
 * ========================================================================= */
const PAD_CSS = 336;                     // 显示尺寸
let padCtx, padDirty = true;

function initPad() {
  const cv = $('pad');
  cv.width = 28 * 12; cv.height = 28 * 12;         // 内部分辨率 336
  cv.style.width = PAD_CSS + 'px'; cv.style.height = PAD_CSS + 'px';
  padCtx = cv.getContext('2d');

  let drawing = false, lx = -1, ly = -1;
  const pos = e => {
    const r = cv.getBoundingClientRect();
    return [(e.clientX - r.left) * 28 / r.width, (e.clientY - r.top) * 28 / r.height];
  };
  const paint = (x, y) => {
    const R = 1.45;
    for (let dy = -2; dy <= 2; dy++) for (let dx = -2; dx <= 2; dx++) {
      const gx = Math.round(x) + dx, gy = Math.round(y) + dy;
      if (gx < 0 || gy < 0 || gx > 27 || gy > 27) continue;
      const d = Math.hypot(gx - x, gy - y);
      const a = clamp(1.15 - (d / R) ** 2, 0, 1);
      const i = gy * 28 + gx;
      PAD[i] = Math.max(PAD[i], a);                // max 叠加, 避免反复涂黑
    }
    // 补线段插值
    if (lx >= 0 && (Math.abs(x - lx) > 0.9 || Math.abs(y - ly) > 0.9)) {
      const n = Math.ceil(Math.hypot(x - lx, y - ly));
      for (let k = 1; k < n; k++) {
        const ix = lx + (x - lx) * k / n, iy = ly + (y - ly) * k / n;
        paintNoRec.call(null, ix, iy);
      }
    }
    function paintNoRec(ix, iy) {
      const RR = 1.45;
      for (let dy = -2; dy <= 2; dy++) for (let dx = -2; dx <= 2; dx++) {
        const gx = Math.round(ix) + dx, gy = Math.round(iy) + dy;
        if (gx < 0 || gy < 0 || gx > 27 || gy > 27) continue;
        const dd = Math.hypot(gx - ix, gy - iy);
        const aa = clamp(1.15 - (dd / RR) ** 2, 0, 1);
        const ii = gy * 28 + gx;
        PAD[ii] = Math.max(PAD[ii], aa);
      }
    }
    lx = x; ly = y;
    padDirty = true;
    $('ledInput').className = 'led ok';
  };

  cv.addEventListener('pointerdown', e => {
    drawing = true;
    try { cv.setPointerCapture(e.pointerId); } catch (_) { /* 合成事件无真实指针 */ }
    const [x, y] = pos(e); lx = -1; paint(x, y);
  });
  cv.addEventListener('pointermove', e => {
    if (!drawing) return;
    const [x, y] = pos(e); paint(x, y);
  });
  const stop = () => { drawing = false; lx = -1; };
  cv.addEventListener('pointerup', stop);
  cv.addEventListener('pointercancel', stop);

  (function padLoop() {
    if (padDirty) { renderPad(); padDirty = false; }
    requestAnimationFrame(padLoop);
  })();
}

function renderPad() {
  const S = 12;                                     // 28*12 = 336
  padCtx.fillStyle = '#000'; padCtx.fillRect(0, 0, 336, 336);
  if ($('chkGrid').checked) {
    padCtx.strokeStyle = 'rgba(120,150,180,.13)';
    padCtx.lineWidth = 1;
    padCtx.beginPath();
    for (let i = 1; i < 28; i++) {
      padCtx.moveTo(i * S + .5, 0); padCtx.lineTo(i * S + .5, 336);
      padCtx.moveTo(0, i * S + .5); padCtx.lineTo(336, i * S + .5);
    }
    padCtx.stroke();
    // MNIST 字形参考框 (20×20 居中)
    padCtx.strokeStyle = 'rgba(255,176,0,.35)';
    padCtx.setLineDash([4, 4]);
    padCtx.strokeRect(4 * S + .5, 4 * S + .5, 20 * S, 20 * S);
    padCtx.setLineDash([]);
  }
  const img = padCtx.createImageData(28, 28);
  for (let i = 0; i < 784; i++) {
    const g = Math.round(PAD[i] * 255);
    img.data[i * 4] = g; img.data[i * 4 + 1] = g; img.data[i * 4 + 2] = g;
    img.data[i * 4 + 3] = 255;
  }
  const off = document.createElement('canvas'); off.width = 28; off.height = 28;
  off.getContext('2d').putImageData(img, 0, 0);
  padCtx.imageSmoothingEnabled = false;
  padCtx.drawImage(off, 0, 0, 336, 336);
}

function clearPad() {
  PAD.fill(0);
  padDirty = true;
  $('ledInput').className = 'led off';
  log('画板已清空');
}

/* ---------------- 预处理: MNIST 式自动居中 ---------------- */
function autoCenter(src) {
  let minX = 29, minY = 29, maxX = -1, maxY = -1, ink = 0;
  for (let y = 0; y < 28; y++) for (let x = 0; x < 28; x++) {
    const v = src[y * 28 + x];
    if (v > 0.12) {
      ink++;
      if (x < minX) minX = x; if (x > maxX) maxX = x;
      if (y < minY) minY = y; if (y > maxY) maxY = y;
    }
  }
  if (!ink || maxX < 0) return null;               // 无笔迹
  const bw = maxX - minX + 1, bh = maxY - minY + 1;
  const scale = 20 / Math.max(bw, bh);             // 最长边缩到 20 (MNIST 规范)
  const out = new Float32Array(784);
  const cx0 = (minX + maxX + 1) / 2, cy0 = (minY + maxY + 1) / 2;
  for (let y = 0; y < 28; y++) for (let x = 0; x < 28; x++) {
    // 目标像素 -> 源坐标 (以笔画质心为基准映射)
    const sx = cx0 + (x - 13.5) / scale, sy = cy0 + (y - 13.5) / scale;
    const x0 = Math.floor(sx), y0 = Math.floor(sy);
    if (x0 < 0 || y0 < 0 || x0 >= 27 || y0 >= 28 || sx < 0 || sy < 0) continue;
    const fx = sx - x0, fy = sy - y0;
    const g = (xx, yy) => (xx < 0 || yy < 0 || xx > 27 || yy > 27) ? 0 : src[yy * 28 + xx];
    out[y * 28 + x] =
      g(x0, y0) * (1 - fx) * (1 - fy) + g(x0 + 1, y0) * fx * (1 - fy) +
      g(x0, y0 + 1) * (1 - fx) * fy + g(x0 + 1, y0 + 1) * fx * fy;
  }
  return out;
}

/* =========================================================================
 * 推理 + 各阶段渲染
 * ========================================================================= */
function runInference(silent) {
  const src = ($('chkCenter').checked ? autoCenter(PAD) : PAD.slice());
  if (!src) { log('画板为空 —— 请先写一个数字再执行推理', 'warn'); return; }
  SNAP = src;
  const t0 = performance.now();
  LAST = engine.forwardRaw(SNAP);
  const dt = performance.now() - t0;

  renderStage0(); renderConv1(); renderPoolCmp(1); renderConv2();
  renderPoolCmp(2); renderFlat(); renderFc1(); renderSoftmax();
  vizSetData();
  updatePrediction(dt);

  $('ledInput').className = 'led ok';
  log(`推理完成: 预测=${argmax(LAST.probs)} 置信度=` +
      `${(Math.max(...LAST.probs) * 100).toFixed(1)}% · 耗时 ${dt.toFixed(2)} ms` +
      ` · conv1 ${(LAST.timingMs.conv1).toFixed(2)}ms conv2 ${(LAST.timingMs.conv2).toFixed(2)}ms`,
      'hl');
  if (!silent) $('btnRun').classList.remove('flash'), void $('btnRun').offsetWidth, $('btnRun').classList.add('flash');
}
const argmax = a => a.indexOf(Math.max(...a));

/* 建立两层卷积的缩略图画廊 */
function initStations() {
  buildGallery('conv1', 'gal1', 32, 28, 28, 42, i => selectConv1(i));
  buildGallery('conv2', 'gal2', 64, 14, 14, 42, i => selectConv2(i));
}

/* --- STG00 输入/预处理 --- */
function renderStage0() {
  drawMap($('cRaw'), PAD, 28, 28, 140);
  drawMap($('cCtr'), SNAP, 28, 28, 140);
  const nrm = Engine.normalize(SNAP);
  const st = Engine.stats(nrm);
  $('st0stats').textContent =
    `标准化 (x−μ)/σ · μ=0.1307 σ=0.3081 → 输出范围 [${st.min.toFixed(3)}, ${st.max.toFixed(3)}] 均值 ${st.mean.toFixed(3)}`;
}

/* --- 缩略图画廊 --- */
function buildGallery(key, contId, count, mapW, mapH, px, onPick) {
  const cont = $(contId);
  cont.innerHTML = '';
  GAL[key] = [];
  for (let i = 0; i < count; i++) {
    const c = document.createElement('canvas');
    c.width = mapW; c.height = mapH;
    c.style.width = px + 'px'; c.style.height = px + 'px';
    c.className = 'thumb';
    c.title = 'feature map #' + i;
    c.addEventListener('click', () => onPick(i));
    cont.appendChild(c);
    GAL[key].push(c);
  }
}
function fillGallery(key, f32, count, mapWH) {
  const mp = mapWH * mapWH;
  for (let i = 0; i < count; i++) {
    const cv = GAL[key][i];
    drawMap(cv, f32.subarray(i * mp, (i + 1) * mp), mapWH, mapWH);
    cv.classList.toggle('sel', i === (key === 'conv1' ? SEL.conv1 : SEL.conv2));
  }
}

/* --- STG01 CONV1 + ReLU --- */
function renderConv1() {
  fillGallery('conv1', LAST.conv1, 32, 28);
  drawMap($('big1'), LAST.conv1.subarray(SEL.conv1 * 784, (SEL.conv1 + 1) * 784), 28, 28, 168);
  $('info1').textContent =
    `通道 #${SEL.conv1} · 28×28=784 像素 · min ${Engine.stats(LAST.conv1.subarray(SEL.conv1 * 784, (SEL.conv1 + 1) * 784)).min.toFixed(2)}` +
    ` / max ${Engine.stats(LAST.conv1.subarray(SEL.conv1 * 784, (SEL.conv1 + 1) * 784)).max.toFixed(2)} · ReLU 后负值全为 0`;
  renderKernelTable();
  macReset(true);
}

/* 当前选中卷积核的 3×3 权重表 */
function renderKernelTable() {
  const tbl = $('kernTable');
  tbl.innerHTML = '';
  const base = SEL.conv1 * 9;
  for (let ky = 0; ky < 3; ky++) {
    for (let kx = 0; kx < 3; kx++) {
      const v = W.conv1_w[base + ky * 3 + kx];
      const d = document.createElement('div');
      d.className = 'kcell ' + (v >= 0 ? 'pos' : 'neg');
      d.style.opacity = clamp(0.35 + Math.abs(v) * 1.6, 0.35, 1);
      d.textContent = f2(v);
      d.title = `w[${ky}][${kx}] = ${v.toFixed(6)}`;
      tbl.appendChild(d);
    }
  }
  $('bias1').textContent = f2(W.conv1_b[SEL.conv1]);
}

/* --- 池化对比 (stage 1 或 2) --- */
function renderPoolCmp(stage) {
  if (!LAST) return;
  if (stage === 1) {
    const i = SEL.conv1, pre = LAST.conv1.subarray(i * 784, (i + 1) * 784);
    const post = LAST.pool1.subarray(i * 196, (i + 1) * 196);
    drawPoolPair($('poolA'), pre, 28, $('poolB'), post, 14);
    const st = Engine.stats(post);
    $('st2stats').textContent =
      `通道 #${i}: 28×28=784 → 14×14=196 (保留 25% 数据) · 值域 [${st.min.toFixed(2)}, ${st.max.toFixed(2)}] · 每 2×2 取最大值`;
  } else {
    const i = SEL.conv2, pre = LAST.conv2.subarray(i * 196, (i + 1) * 196);
    const post = LAST.pool2.subarray(i * 49, (i + 1) * 49);
    drawPoolPair($('poolA2'), pre, 14, $('poolB2'), post, 7);
    const st = Engine.stats(post);
    $('st4stats').textContent =
      `通道 #${i}: 14×14=196 → 7×7=49 · 值域 [${st.min.toFixed(2)}, ${st.max.toFixed(2)}]`;
  }
}
function drawPoolPair(cA, pre, wPre, cB, post, wPost) {
  drawMap(cA, pre, wPre, wPre, wPre === 28 ? 140 : 126);
  drawMap(cB, post, wPost, wPost, wPost === 14 ? 140 : 126);
  // 在前图上叠加 2×2 分割线
  const ctx = cA.getContext('2d');
  const cell = cA.clientWidth / (wPre / 2);
  ctx.strokeStyle = 'rgba(255,80,80,.55)';
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let i = 2; i < wPre; i += 2) {
    const p = i * (cA.clientWidth / wPre);
    ctx.moveTo(p + .5, 0); ctx.lineTo(p + .5, cA.clientHeight);
    ctx.moveTo(0, p + .5); ctx.lineTo(cA.clientWidth, p + .5);
  }
  ctx.stroke();
}

/* --- STG03 CONV2 --- */
function renderConv2() {
  fillGallery('conv2', LAST.conv2, 64, 14);
  drawMap($('big2'), LAST.conv2.subarray(SEL.conv2 * 196, (SEL.conv2 + 1) * 196), 14, 14, 168);
  const sl = LAST.conv2.subarray(SEL.conv2 * 196, (SEL.conv2 + 1) * 196);
  const st = Engine.stats(sl);
  $('info2').textContent =
    `通道 #${SEL.conv2} · 每个输出像素 = Σ(该核 3×3×32 个权重 × 32 张输入特征图的对应邻域) + bias → ReLU` +
    ` · 值域 [${st.min.toFixed(2)}, ${st.max.toFixed(2)}]`;
  // 该核在某个输入通道上的切片展示
  const sliceSel = Math.min(SEL.conv1, 31);
  const kt = $('kern2Table'); kt.innerHTML = '';
  const base = (SEL.conv2 * 32 + sliceSel) * 9;
  for (let k = 0; k < 9; k++) {
    const v = W.conv2_w[base + k];
    const d = document.createElement('div');
    d.className = 'kcell sm ' + (v >= 0 ? 'pos' : 'neg');
    d.style.opacity = clamp(0.3 + Math.abs(v) * 2.5, 0.3, 1);
    d.textContent = f2(v);
    kt.appendChild(d);
  }
  $('kern2cap').textContent = `核#${SEL.conv2} 在输入通道 #${sliceSel} 上的 3×3 切片 (共 32 片)`;
}

/* --- STG05 展平 --- */
function renderFlat() {
  const cv = $('flatStrip');
  const Wd = cv.parentElement.clientWidth - 4;
  cv.width = Wd; cv.height = 56;
  cv.style.width = Wd + 'px';
  const ctx = cv.getContext('2d');
  const img = ctx.createImageData(3136, 1);
  for (let i = 0; i < 3136; i++) {
    const t = clamp(LAST.pool2[i] / Math.max(...LAST.pool2, 1e-6), 0, 1);
    img.data[i * 4] = Math.round(20 + t * 235);
    img.data[i * 4 + 1] = Math.round(26 + t * 160);
    img.data[i * 4 + 2] = Math.round(38 + t * 60);
    img.data[i * 4 + 3] = 255;
  }
  const off = document.createElement('canvas'); off.width = 3136; off.height = 1;
  off.getContext('2d').putImageData(img, 0, 0);
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(off, 0, 0, Wd, 56);
  // 通道分块刻度 + 选中通道高亮
  const pxPer = Wd / 3136;
  ctx.strokeStyle = 'rgba(200,220,240,.25)';
  ctx.beginPath();
  for (let cch = 1; cch < 64; cch++) {
    const x = cch * 49 * pxPer;
    ctx.moveTo(x + .5, 0); ctx.lineTo(x + .5, 56);
  }
  ctx.stroke();
  ctx.fillStyle = 'rgba(255,176,0,.28)';
  ctx.fillRect(SEL.conv2 * 49 * pxPer, 0, 49 * pxPer, 56);
  const st = Engine.stats(LAST.pool2);
  $('st5stats').innerHTML =
    `展平顺序: 通道优先 (c,h,w) —— 第 ${SEL.conv2} 通道高亮, 共 64×7×7=3136 维 · ` +
    `非零占比 ${(st.nonzero * 100).toFixed(1)}% · 这 3136 个数将乘上 fc1 的 128×3136 权重矩阵`;
}

/* --- STG06 FC1 --- */
function renderFc1(mask) {
  const cv = $('fc1Bars');
  const Wd = cv.parentElement.clientWidth - 4;
  cv.width = Wd; cv.height = 120; cv.style.width = Wd + 'px';
  const ctx = cv.getContext('2d');
  ctx.clearRect(0, 0, Wd, 120);
  const vals = LAST.fc1;
  const mx = Math.max(...vals, 1e-6);
  const bw = Wd / 128;
  for (let i = 0; i < 128; i++) {
    let v = vals[i];
    let col = '#ffb000';
    if (mask && mask[i]) { v = v * 1.3333; col = '#4a5568'; }   // 训练式缩放
    const bh = (v / mx) * 104;
    ctx.fillStyle = col;
    ctx.fillRect(i * bw + .5, 112 - bh, bw - 1.2, bh);
    if (!(mask && mask[i]) && v === 0) {                       // ReLU 截断标记
      ctx.fillStyle = '#58a6ff33';
      ctx.fillRect(i * bw + .5, 110, bw - 1.2, 2);
    }
  }
  const st = Engine.stats(vals);
  $('st6stats').innerHTML =
    `y = ReLU(W·x + b) · W 是 128×3136 矩阵 (401,408 个权重, 占全网 95%) · ` +
    `输出 128 维, 其中 ${(st.nonzero * 100).toFixed(0)}% 非 ReLU 截断` +
    (mask ? ' · <span class="warn-t">当前处于 Dropout 模拟模式</span>' : '');
}

/* --- Dropout 模拟 --- */
let dropMask = null;
function toggleDropout(on) {
  if (on) {
    dropMask = new Array(128).fill(false).map(() => Math.random() < 0.25);
    const hz = LAST.fc1.map((v, i) => dropMask[i] ? 0 : v * 1.3333);
    const lg = new Array(10);
    Engine.fc(Float32Array.from(hz), W.fc2_w, W.fc2_b, 128, 10, lg);
    LAST.logitsDrop = lg;
    log('Dropout 模拟: 已随机屏蔽 25% 的 fc1 神经元并重新计算输出 (仅演示)', 'warn');
  } else {
    dropMask = null; delete LAST.logitsDrop;
    log('Dropout 模拟关闭, 恢复完整网络输出');
  }
  renderFc1(dropMask);
  renderSoftmax();
}

/* --- STG08 Softmax --- */
function renderSoftmax() {
  const T = parseFloat($('tTemp').value);
  $('tempVal').textContent = 'T = ' + T.toFixed(2);
  const lgSrc = (dropMask && LAST.logitsDrop) ? LAST.logitsDrop : LAST.logits;
  const pr = new Array(10);
  Engine.softmax(lgSrc, T, pr);
  const cv = $('smBars');
  const Wd = cv.parentElement.clientWidth - 4;
  cv.width = Wd; cv.height = 230; cv.style.width = Wd + 'px';
  const ctx = cv.getContext('2d');
  ctx.clearRect(0, 0, Wd, 230);
  const rowH = 21, labelW = 26, barMax = Wd - labelW - 86;
  const best = argmax(pr);
  ctx.font = '12px ui-monospace,Menlo,monospace';
  for (let d = 0; d < 10; d++) {
    const y = d * rowH;
    ctx.fillStyle = d === best ? '#ffffff' : '#8b949e';
    ctx.fillText(String(d), 8, y + 15);
    ctx.fillStyle = '#21262d';
    ctx.fillRect(labelW, y + 4, barMax, 13);
    const grad = ctx.createLinearGradient(labelW, 0, labelW + barMax, 0);
    grad.addColorStop(0, '#1f6feb'); grad.addColorStop(1, d === best ? '#ffb000' : '#3fb950');
    ctx.fillStyle = grad;
    ctx.fillRect(labelW, y + 4, Math.max(pr[d] * barMax, 1.5), 13);
    ctx.fillStyle = d === best ? '#ffb000' : '#c9d1d9';
    ctx.fillText((pr[d] * 100).toFixed(2) + '%', labelW + barMax + 8, y + 15);
    ctx.fillStyle = '#4a5568';
    ctx.fillText('z=' + lgSrc[d].toFixed(1), labelW + barMax + 62, y + 15);
  }
  const order = [...pr.keys()].sort((a, b) => pr[b] - pr[a]).slice(0, 3);
  $('top3').innerHTML = 'TOP-3: ' + order.map((d, i) =>
    `<b>${d}</b> ${(pr[d] * 100).toFixed(1)}%`).join(' · ') +
    ` &nbsp;|&nbsp; Σexp(z)/Σexp(z)=100% · 温度T只改变分布陡峭程度, 不改变 argmax`;
}

/* --- 预测横幅 --- */
function updatePrediction(dtMs) {
  const p = LAST.probs, best = argmax(p);
  $('predDigit').textContent = best;
  $('predConf').textContent = (p[best] * 100).toFixed(2) + '%';
  $('predTime').textContent = dtMs.toFixed(2) + ' ms';
}

/* =========================================================================
 * 卷积滑动动画 (STG01 内嵌)
 * ========================================================================= */
const MAC = { play: false, y: 0, x: 0, spf: 1, acc: 0, timer: null };

function initMac() {
  ['macIn', 'macOut'].forEach(id => {
    const c = $(id); c.width = 240; c.height = 240;
    c.style.width = '180px'; c.style.height = '180px';
  });
  const sel = $('macKernel');
  for (let i = 0; i < 32; i++) {
    const o = document.createElement('option');
    o.value = i; o.textContent = '核 #' + i;
    sel.appendChild(o);
  }
  sel.addEventListener('change', () => { selectConv1(+sel.value); });
  $('macPlay').addEventListener('click', () => macToggle());
  $('macStep').addEventListener('click', () => { MAC.play = false; updPlayBtn(); macStep(); });
  $('macReset').addEventListener('click', () => macReset());
  $('macSpeed').addEventListener('change', e => { MAC.spf = +e.target.value; });
  macReset();
}

function macToggle() {
  if (MAC.play) { MAC.play = false; }
  else {
    if (!SNAP) { log('请先执行一次推理 (动画需要输入数据)', 'warn'); return; }
    if (MAC.y >= 28) macReset();
    MAC.play = true;
    requestAnimationFrame(macFrame);
  }
  updPlayBtn();
}
function updPlayBtn() { $('macPlay').textContent = MAC.play ? '❚❚ 暂停' : '▶ 播放'; }

function macReset(soft) {
  MAC.y = 0; MAC.x = 0; MAC.acc = 0;
  if (!soft) MAC.play = false;
  updPlayBtn();
  if (!SNAP) return;
  const c = $('macOut').getContext('2d');
  c.fillStyle = '#000'; c.fillRect(0, 0, 240, 240);
  macDrawInput(-9, -9);
  // 预填: 核数值立即可见, 补丁/乘积为占位
  const kb = SEL.conv1 * 9;
  let kh = '';
  for (let i = 0; i < 9; i++)
    kh += `<div class="maccell kv">${f2(W.conv1_w[kb + i])}</div>`;
  $('macKernV').innerHTML = kh;
  $('macPatch').innerHTML = '<div class="maccell dim" style="grid-column:1/-1">点击 ▶ 播放</div>';
  $('macProd').innerHTML = '<div class="maccell dim" style="grid-column:1/-1">—</div>';
  $('macCalc').innerHTML = '<span class="dim">等待播放… (窗口将从左上角逐格扫过整张图)</span>';
}

function macDrawInput(wy, wx) {
  // 输入图: 中心 28×28 + 四周 1 格 zero-padding 环 (8px/格, 共 30 格 -> 只画中 28, 环用边框色示意)
  const ctx = $('macIn').getContext('2d');
  const cs = 8, off = cs;                    // 左上留 1 格表示 padding
  ctx.fillStyle = '#0a0f16'; ctx.fillRect(0, 0, 240, 240);
  ctx.fillStyle = '#101826';                 // padding 区
  ctx.fillRect(0, 0, 240, cs); ctx.fillRect(0, 232 - cs, 240, cs);
  ctx.fillRect(0, 0, cs, 240); ctx.fillRect(240 - cs, 0, cs, 240);
  for (let y = 0; y < 28; y++) for (let x = 0; x < 28; x++) {
    const v = SNAP[y * 28 + x];
    const g = Math.round(v * 255);
    ctx.fillStyle = `rgb(${g},${g},${g})`;
    ctx.fillRect(off + x * cs, off + y * cs, cs, cs);
  }
  // 网格线
  ctx.strokeStyle = 'rgba(120,150,180,.10)';
  ctx.beginPath();
  for (let i = 0; i <= 28; i += 2) {
    ctx.moveTo(off + i * cs + .5, off); ctx.lineTo(off + i * cs + .5, off + 28 * cs);
    ctx.moveTo(off, off + i * cs + .5); ctx.lineTo(off + 28 * cs, off + i * cs + .5);
  }
  ctx.stroke();
  // 滑窗 (含越界部分)
  if (wy >= 0) {
    const x0 = off + (wx - 1) * cs, y0 = off + (wy - 1) * cs;
    ctx.strokeStyle = '#ff4d4d'; ctx.lineWidth = 2;
    ctx.strokeRect(Math.max(2, x0) + 1, Math.max(2, y0) + 1,
      3 * cs - 2 + Math.min(0, x0), 3 * cs - 2 + Math.min(0, y0));
    ctx.strokeStyle = 'rgba(255,77,77,.25)';
    ctx.strokeRect(off + (wx - 1) * cs + .5, off + (wy - 1) * cs + .5, 3 * cs, 3 * cs);
  }
  $('macPos').textContent = wy >= 0 ? `窗口位置 行${wy} 列${wx} (${wy * 28 + wx + 1}/784)` : '-';
}

function macStep() {
  if (!SNAP) return;
  const o = SEL.conv1, kb = o * 9, b = W.conv1_b[o];
  let sum = b;
  const patch = [], prod = [];
  for (let ky = 0; ky < 3; ky++) for (let kx = 0; kx < 3; kx++) {
    const yy = MAC.y - 1 + ky, xx = MAC.x - 1 + kx;
    const v = (yy < 0 || xx < 0 || yy > 27 || xx > 27) ? 0 : SNAP[yy * 28 + xx];
    const wv = W.conv1_w[kb + ky * 3 + kx];
    patch.push(v); prod.push(v * wv); sum += v * wv;
  }
  const post = Math.max(0, sum);
  MAC.acc = post;
  macDrawInput(MAC.y, MAC.x);
  // 写输出像素
  const oc = $('macOut').getContext('2d');
  const t = post / Math.max(...LAST.conv1.subarray(o * 784, (o + 1) * 784), 1e-6);
  const g = Math.round(clamp(t, 0, 1) * 255);
  oc.fillStyle = `rgb(${Math.round(18 + g * .82)},${Math.round(24 + g * .88)},${Math.round(34 + g * .92)})`;
  oc.fillRect(MAC.x * 8, MAC.y * 8, 8, 8);

  const cell = (val, cls, dim) =>
    `<div class="maccell ${cls}" ${dim ? 'style="opacity:.4"' : ''}>${val}</div>`;
  let html = '';
  for (let i = 0; i < 9; i++)
    html += cell(patch[i].toFixed(2), 'pv', patch[i] === 0);
  $('macPatch').innerHTML = html;
  html = '';
  for (let i = 0; i < 9; i++) html += cell(f2(W.conv1_w[kb + i]), 'kv');
  $('macKernV').innerHTML = html;
  html = '';
  for (let i = 0; i < 9; i++) html += cell(prod[i].toFixed(2), 'prodv');
  $('macProd').innerHTML = html;
  const reluTag = sum < 0 ? `<span class="relu-neg">ReLU(${sum.toFixed(3)}) → 0</span>`
                          : `ReLU(${sum.toFixed(3)}) = <b>${sum.toFixed(3)}</b>`;
  $('macCalc').innerHTML =
    `Σ(9次乘积) + bias(${f2(b)}) = <b>${sum.toFixed(4)}</b> &nbsp;→&nbsp; ${reluTag}`;

  // 前进
  MAC.x++;
  if (MAC.x >= 28) { MAC.x = 0; MAC.y++; }
  if (MAC.y >= 28) { MAC.play = false; updPlayBtn(); log(`卷积动画: 核#${o} 扫描完成 (784 次乘加)`); }
}

function macFrame() {
  if (!MAC.play) return;
  for (let i = 0; i < MAC.spf && MAC.play; i++) macStep();
  if (MAC.play) requestAnimationFrame(macFrame);
}

/* =========================================================================
 * 3D 探索器
 * ========================================================================= */
function initViz() {
  viz = new Viz3D.Viz3D($('view3d'), {});
  viz.onSelect = (gid, idx) => {
    if (gid === 'conv1') selectConv1(idx);
    else if (gid === 'conv2') selectConv2(idx);
  };
  $('btnResetView').addEventListener('click', () => viz.resetView());
}

function vizSetData() {
  const specs = [];
  specs.push({ id: 'input', title: 'INPUT 28×28', color: '#8b949e', x: 0,
               items: [{ tex: makeTex(SNAP, 28, 28), cw: 28, ch: 28 }], itemScale: .62 });
  const mk = (id, title, color, x, f32, n, mw, cols, sc) => {
    const items = [];
    for (let i = 0; i < n; i++)
      items.push({ tex: makeTex(f32.subarray(i * mw * mw, (i + 1) * mw * mw), mw, mw),
                   cw: mw, ch: mw });
    specs.push({ id, title, color, x, items, cols, itemScale: sc });
  };
  mk('conv1', 'CONV1·ReLU 32@28²', '#58a6ff', 42, LAST.conv1, 32, 28, 8, .34);
  mk('pool1', 'POOL1 32@14²', '#3fb950', 84, LAST.pool1, 32, 14, 8, .52);
  mk('conv2', 'CONV2·ReLU 64@14²', '#ffb000', 124, LAST.conv2, 64, 14, 8, .52);
  mk('pool2', 'POOL2 64@7²', '#ff7b72', 164, LAST.pool2, 64, 7, 8, .85);

  // FC1: 16×8 点阵面板
  const fcTex = document.createElement('canvas'); fcTex.width = 128; fcTex.height = 16;
  const fctx = fcTex.getContext('2d');
  const mx = Math.max(...LAST.fc1, 1e-6);
  for (let i = 0; i < 128; i++) {
    const v = LAST.fc1[i] / mx;
    fctx.fillStyle = `rgba(255,176,0,${.08 + v * .92})`;
    fctx.fillRect((i % 16) * 8 + 1, ((i / 16) | 0) * 4 + 1, 6, 2.6);
  }
  specs.push({ id: 'fc1', title: 'FC1·ReLU 128', color: '#bc8cff', x: 192,
               items: [{ tex: fcTex, cw: 128, ch: 16 }], itemScale: .55 });

  // SOFTMAX 概率条
  const smTex = document.createElement('canvas'); smTex.width = 40; smTex.height = 60;
  const sctx = smTex.getContext('2d');
  sctx.font = '10px monospace';
  for (let d = 0; d < 10; d++) {
    const p = LAST.probs[d];
    sctx.fillStyle = '#21262d'; sctx.fillRect(10, d * 6 + 1, 28, 4.5);
    sctx.fillStyle = p > .5 ? '#ffb000' : '#3fb950';
    sctx.fillRect(10, d * 6 + 1, Math.max(p * 28, 1), 4.5);
    sctx.fillStyle = '#8b949e'; sctx.fillText(d, 2, d * 6 + 6);
  }
  specs.push({ id: 'out', title: 'SOFTMAX P(class|x)', color: '#3fb950', x: 216,
               items: [{ tex: smTex, cw: 40, ch: 60 }], itemScale: .8 });

  viz.setScene(specs);
  viz.setSelected('conv1', SEL.conv1);        // 与 2D 面板选中状态同步
}

/* =========================================================================
 * 选择联动 & 控件绑定
 * ========================================================================= */
function selectConv1(i) {
  SEL.conv1 = i;
  $('macKernel').value = i;
  if (LAST) {
    fillGallery('conv1', LAST.conv1, 32, 28);
    renderConv1(); renderPoolCmp(1);
  }
}
function selectConv2(i) {
  SEL.conv2 = i;
  if (LAST) {
    fillGallery('conv2', LAST.conv2, 64, 14);
    renderConv2(); renderPoolCmp(2); renderFlat();
  }
}

function wireControls() {
  $('btnRun').addEventListener('click', () => runInference());
  $('btnClear').addEventListener('click', clearPad);
  $('chkDropout').addEventListener('change', e => toggleDropout(e.target.checked));
  $('tTemp').addEventListener('input', () => { if (LAST) renderSoftmax(); });

  // 示例下拉
  const sel = $('selSample');
  for (let d = 0; d < 10; d++) {
    const o = document.createElement('option');
    o.value = d; o.textContent = '示例数字 ' + d;
    sel.appendChild(o);
  }
  sel.addEventListener('change', e => loadSample(+e.target.value));

  window.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.target.matches('select,input,button')) runInference();
  });
}

/* 加载内置示例 (来自 MNIST 测试集) */
function loadSample(d, silent) {
  const raw = window.CNN_SAMPLES_B64;
  const bin = atob(raw);
  const base = d * 784;
  PAD.fill(0);
  for (let i = 0; i < 784; i++) PAD[i] = bin.charCodeAt(base + i) / 255;
  padDirty = true;
  $('ledInput').className = 'led ok';
  $('selSample').value = String(d);
  if (!silent) log(`已载入示例数字 ${d}`);
  runInference(true);
}

/* E2E 测试/调试句柄 (不影响正常使用) */
window.CNNDBG = {
  get state() { return { LAST, SNAP, SEL, viz, PAD, engine }; },
  macStep, runInference, loadSample
};

/* GO */
document.addEventListener('DOMContentLoaded', boot);
