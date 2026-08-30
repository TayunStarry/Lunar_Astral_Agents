// 从包 index.html 中移除已注入的 MiniLTP Agent 脚本块（恢复原本项目代码）
// 用法：node tools/remove_agent.mjs <包目录名> [更多包目录名...]
import fs from 'node:fs';
import path from 'node:path';

const args = process.argv.slice(2);
if (args.length === 0) { console.error('用法: node tools/remove_agent.mjs <包目录名> [...]'); process.exit(1); }

let anyChanged = false;
for (const pkg of args) {
    const p = path.resolve(new URL('../../local_data/package/', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'), pkg, 'index.html');
    if (!fs.existsSync(p)) { console.log('SKIP (no index.html): ' + pkg); continue; }
    let html = fs.readFileSync(p, 'utf8');
    const before = (html.match(/MiniLTP Agent/g) || []).length;
    // 移除 agent 脚本块：<script> ... /* === ... MiniLTP Agent ... </script>
    const re = /<script>\s*\/\* =+[\s\S]*?MiniLTP Agent[\s\S]*?<\/script>\s*/g;
    html = html.replace(re, '');
    const after = (html.match(/MiniLTP Agent/g) || []).length;
    fs.writeFileSync(p, html, 'utf8');
    console.log(`${pkg}: removed ${before - after} agent block(s)`);
    if (before !== after) anyChanged = true;
}
console.log(anyChanged ? 'DONE' : 'NO_CHANGE');
