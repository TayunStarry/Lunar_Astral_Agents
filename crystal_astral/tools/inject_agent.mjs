// 注入/更新 MiniLTP Agent 到包 index.html
// 用法：node tools/inject_agent.mjs <包目录名>
//  - 已存在 agent：安全切片替换（避免 String.replace 的 $ 展开问题）
//  - 不存在 agent：在 </body> 前插入；无 </body> 则追加到文件末尾
import fs from 'node:fs';
import path from 'node:path';

const pkg = process.argv[2];
if (!pkg) { console.error('用法: node tools/inject_agent.mjs <包目录名>'); process.exit(1); }

const agent = fs.readFileSync(new URL('../assets/mini_ltp_agent.js', import.meta.url), 'utf8');
const p = path.resolve(new URL('../../local_data/package/', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'), pkg, 'index.html');
let html = fs.readFileSync(p, 'utf8');
const scriptBlock = '<script>\n' + agent + '\n</script>';

// 已存在 agent → 切片替换
const startRe = /<script>\s*\/\* =+\s*\*\s*MiniLTP Agent/;
const m = html.match(startRe);
if (m) {
    const startIdx = m.index;
    const closeIdx = html.indexOf('</script>', startIdx + m[0].length);
    const closeEnd = closeIdx + '</script>'.length;
    html = html.slice(0, startIdx) + scriptBlock + html.slice(closeEnd);
    console.log('REPLACED (upgrade)');
} else {
    // 未注入 → 插入 </body> 前
    const lower = html.toLowerCase();
    const bodyIdx = lower.lastIndexOf('</body>');
    if (bodyIdx >= 0) {
        html = html.slice(0, bodyIdx) + scriptBlock + '\n' + html.slice(bodyIdx);
    } else {
        html = html + '\n' + scriptBlock + '\n';
    }
    console.log('INSERTED (fresh)');
}
fs.writeFileSync(p, html, 'utf8');

const chk = fs.readFileSync(p, 'utf8');
console.log('len=' + chk.length);
console.log('agent_occ=' + (chk.match(/MiniLTP Agent/g) || []).length);
console.log('has_loadModelConfig=' + chk.includes('loadModelConfig'));
console.log('keep_open_true=' + chk.includes('keep_open: true'));