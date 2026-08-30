/* =========================================================================
 * engine.js — 纯 JavaScript CNN 推理引擎 (与 train_mnist.py 的 Net 逐位对应)
 *
 * 张量存储约定: 所有特征图用单个扁平 Float32Array 按 NCHW 平铺
 *   idx(c,y,x) = (c*H + y)*W + x
 * 因此 pool2 (64×7×7) 的内存排布与 PyTorch torch.flatten(x,1) 完全一致,
 * "展平"步骤即原数组直接进入全连接层。
 *
 * 浏览器: window.CNNEngine   |   Node(测试): module.exports
 * ========================================================================= */
(function (root, factory) {
  if (typeof module !== 'undefined' && module.exports) module.exports = factory();
  else root.CNNEngine = factory();
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var MEAN = 0.1307, STD = 0.3081;

  /* 权重段偏移表 (与 tools/export_weights.py 的 LAYOUT 一致) */
  var SEG = [
    { name: 'conv1_w', n: 32 * 1 * 3 * 3 },
    { name: 'conv1_b', n: 32 },
    { name: 'conv2_w', n: 64 * 32 * 3 * 3 },
    { name: 'conv2_b', n: 64 },
    { name: 'fc1_w',   n: 128 * 3136 },
    { name: 'fc1_b',   n: 128 },
    { name: 'fc2_w',   n: 10 * 128 },
    { name: 'fc2_b',   n: 10 }
  ];

  function decodeB64(b64) {
    var bytes;
    if (typeof atob === 'function') {
      var bin = atob(b64);
      bytes = new Uint8Array(bin.length);
      for (var i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    } else {                                   // Node
      var buf = Buffer.from(b64, 'base64');
      bytes = new Uint8Array(buf.byteLength);
      for (var j = 0; j < buf.byteLength; j++) bytes[j] = buf[j];
    }
    return new Float32Array(bytes.buffer);
  }

  function loadWeights(b64) {
    var all = decodeB64(b64), off = 0, w = {};
    SEG.forEach(function (s) {
      w[s.name] = all.subarray(off, off + s.n);
      off += s.n;
    });
    return w;
  }

  /* ---------------- 算子 ---------------- */

  // 3×3 卷积, stride=1, padding=1 (same) —— 与 nn.Conv2d(...,padding=1) 一致
  function conv3x3(x, C, H, W, w, b, CO, out) {
    for (var o = 0; o < CO; o++) {
      var obase = o * H * W;
      for (var y = 0; y < H; y++) {
        for (var xx = 0; xx < W; xx++) {
          var s = b[o];
          for (var c = 0; c < C; c++) {
            var cb = c * H * W, wb = (o * C + c) * 9;
            for (var ky = 0; ky < 3; ky++) {
              var yy = y - 1 + ky; if (yy < 0 || yy >= H) continue;
              for (var kx = 0; kx < 3; kx++) {
                var xc = xx - 1 + kx; if (xc < 0 || xc >= W) continue;
                s += w[wb + ky * 3 + kx] * x[cb + yy * W + xc];
              }
            }
          }
          out[obase + y * W + xx] = s;
        }
      }
    }
    return out;
  }

  // ReLU
  function relu(x, out) {
    for (var i = 0; i < x.length; i++) out[i] = x[i] > 0 ? x[i] : 0;
    return out;
  }

  // 2×2 最大池化, stride 2 (H,W 为偶数)
  function maxpool2(x, C, H, W, out) {
    var HO = H >> 1, WO = W >> 1;
    for (var c = 0; c < C; c++) {
      var cb = c * H * W, ob = c * HO * WO;
      for (var oy = 0; oy < HO; oy++) {
        for (var ox = 0; ox < WO; ox++) {
          var y = oy << 1, xx = ox << 1;
          var a = x[cb + y * W + xx],       b = x[cb + y * W + xx + 1],
              d = x[cb + (y + 1) * W + xx], e = x[cb + (y + 1) * W + xx + 1];
          var m = a > b ? a : b; if (d > m) m = d; if (e > m) m = e;
          out[ob + oy * WO + ox] = m;
        }
      }
    }
    return out;
  }

  // 全连接 y = W·x + b, W 行主序 [OUT][IN]
  function fc(x, w, b, IN, OUT, out) {
    for (var o = 0; o < OUT; o++) {
      var s = b[o], base = o * IN;
      for (var i = 0; i < IN; i++) s += w[base + i] * x[i];
      out[o] = s;
    }
    return out;
  }

  // 数值稳定 softmax (T=温度)
  function softmax(logits, T, out) {
    T = T || 1;
    var m = -Infinity, i;
    for (i = 0; i < logits.length; i++) if (logits[i] > m) m = logits[i];
    var s = 0;
    for (i = 0; i < logits.length; i++) { out[i] = Math.exp((logits[i] - m) / T); s += out[i]; }
    for (i = 0; i < logits.length; i++) out[i] /= s;
    return out;
  }

  // 预处理: 原始像素[0,1] -> 训练同款标准化
  function normalize(raw01, out) {
    out = out || new Float32Array(raw01.length);
    for (var i = 0; i < raw01.length; i++) out[i] = (raw01[i] - MEAN) / STD;
    return out;
  }

  /* ---------------- 完整前向 ---------------- */

  function makeEngine(weights) {
    var bufs = {};                                // 复用的中间缓冲区
    function buf(k, n) { return bufs[k] || (bufs[k] = new Float32Array(n)); }

    // raw01: 784 个 [0,1] 像素; 返回全部中间激活量 (供可视化)
    function forwardRaw(raw01) {
      return forwardNorm(normalize(raw01));
    }

    // 已标准化的输入 (黄金对拍用)
    function forwardNorm(xn) {
      var t0 = now();
      var z1 = buf('z1', 32 * 784);
      conv3x3(xn, 1, 28, 28, weights.conv1_w, weights.conv1_b, 32, z1);
      var a1 = buf('a1', 32 * 784); relu(z1, a1);
      var t1 = now();
      var p1 = buf('p1', 32 * 14 * 14); maxpool2(a1, 32, 28, 28, p1);
      var t2 = now();
      var z2 = buf('z2', 64 * 196);
      conv3x3(p1, 32, 14, 14, weights.conv2_w, weights.conv2_b, 64, z2);
      var a2 = buf('a2', 64 * 196); relu(z2, a2);
      var t3 = now();
      var p2 = buf('p2', 64 * 49); maxpool2(a2, 64, 14, 14, p2);   // 即展平向量(3136)
      var t4 = now();
      var hz = buf('hz', 128); fc(p2, weights.fc1_w, weights.fc1_b, 3136, 128, hz);
      var h = buf('h', 128); relu(hz, h);
      var lg = buf('lg', 10); fc(h, weights.fc2_w, weights.fc2_b, 128, 10, lg);
      var pr = buf('pr', 10); softmax(lg, 1, pr);
      var t5 = now();

      return {
        inputNorm: xn.slice(),
        conv1Pre: z1.slice(), conv1: a1.slice(),
        pool1: p1.slice(),
        conv2Pre: z2.slice(), conv2: a2.slice(),
        pool2: p2.slice(),                       // 展平向量同一块数据
        fc1Pre: hz.slice(), fc1: h.slice(),
        dropoutMask: null,                       // 推理时 Dropout 关闭(恒等)
        logits: lg.slice(),
        probs: pr.slice(),
        timingMs: { conv1: t1 - t0, pool1: t2 - t1, conv2: t3 - t2,
                    pool2_flat: t4 - t3, fc_softmax: t5 - t4 }
      };
    }

    return { forwardRaw: forwardRaw, forwardNorm: forwardNorm };
  }

  function now() {
    return (typeof performance !== 'undefined' ? performance : Date).now();
  }

  /* ---------------- 工具 ---------------- */

  function stats(arr) {
    var mn = Infinity, mx = -Infinity, s = 0, nz = 0;
    for (var i = 0; i < arr.length; i++) {
      var v = arr[i];
      if (v < mn) mn = v; if (v > mx) mx = v; s += v; if (v !== 0) nz++;
    }
    return { min: mn, max: mx, mean: s / arr.length, nonzero: nz / arr.length };
  }

  return {
    MEAN: MEAN, STD: STD, SEG: SEG,
    loadWeights: loadWeights,
    makeEngine: makeEngine,
    conv3x3: conv3x3, relu: relu, maxpool2: maxpool2,
    fc: fc, softmax: softmax, normalize: normalize,
    stats: stats,
    VERSION: '1.0.0'
  };
});
