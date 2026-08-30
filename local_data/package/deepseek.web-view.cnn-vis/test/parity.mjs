/* JS 引擎 vs PyTorch 黄金向量对拍测试
 * 运行: node test/parity.mjs   (在 cnndemo 目录下)
 */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..');

// ---- 载入权重数据 (模拟浏览器全局 window) ----
globalThis.window = {};
eval(fs.readFileSync(path.join(ROOT, 'assets/model_data.js'), 'utf8'));
const B64 = globalThis.window.CNN_WEIGHTS_B64;

const Engine = require(path.join(ROOT, 'assets/engine.js'));
const golden = JSON.parse(fs.readFileSync(path.join(HERE, 'golden.json'), 'utf8'));

console.log('engine version:', Engine.VERSION,
            '| 权重参数量:', Buffer.byteLength(B64, 'base64') / 4);

const w = Engine.loadWeights(B64);
const eng = Engine.makeEngine(w);

const flat = v => Array.isArray(v) ? v.flatMap(flat) : v;
const x = new Float32Array(flat(golden.input_norm));
const out = eng.forwardNorm(x);

const stages = [
  ['conv1', out.conv1], ['pool1', out.pool1], ['conv2', out.conv2],
  ['pool2', out.pool2], ['fc1', out.fc1], ['logits', out.logits],
  ['probs', out.probs]
];

let ok = true;
for (const [name, got] of stages) {
  const want = flat(golden[name]);
  let maxDiff = 0, badIdx = -1;
  if (got.length !== want.length) {
    console.log(`✗ ${name} 长度不符 ${got.length} vs ${want.length}`);
    ok = false; continue;
  }
  for (let i = 0; i < want.length; i++) {
    const d = Math.abs(got[i] - want[i]);
    if (d > maxDiff) { maxDiff = d; badIdx = i; }
  }
  const pass = maxDiff < 2e-3;
  if (!pass) ok = false;
  console.log(`${pass ? '✓' : '✗'} ${name.padEnd(7)} 长度=${got.length}/${want.length}` +
              `  最大绝对误差=${maxDiff.toExponential(2)} @${badIdx}`);
}

const argmax = out.logits.indexOf(Math.max(...out.logits));
const psum = out.probs.reduce((a, b) => a + b, 0);
console.log(`\n预测数字 = ${argmax} (黄金标签 = ${golden.label})  Σp=${psum.toFixed(6)}`);
console.log('logits:', [...out.logits].map(v => v.toFixed(3)).join(', '));
if (argmax !== golden.label) ok = false;

console.log(ok ? '\n== 全部通过: JS 引擎与 PyTorch 数值一致 ==' : '\n!! 对拍失败');
process.exit(ok ? 0 : 1);
