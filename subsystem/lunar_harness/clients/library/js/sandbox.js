// sandbox.js 代码运行：在页面内的 iframe 沙箱中执行，不在宿主机起进程。

import { fileURL } from './api.js';

const MAX_SOURCE_CHARS = 300000;

// runInSandbox 把文件内容装进沙箱 iframe 运行（.js 包一层日志，.html 原样执行）。
export async function runInSandbox(entry) {
    const frame = document.getElementById('sandbox');
    let source = '';
    try {
        const res = await fetch(fileURL(entry.path));
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        source = await res.text();
    } catch (err) {
        throw new Error(`读取文件失败：${err.message || err}`);
    }
    if (source.length > MAX_SOURCE_CHARS) {
        throw new Error('文件过大，沙箱只运行前 30 万字符');
    }
    const ext = entry.name.slice(entry.name.lastIndexOf('.')).toLowerCase();
    const html = ext === '.js' || ext === '.mjs' ? wrapScript(source) : source;
    frame.src = URL.createObjectURL(new Blob([html], { type: 'text/html' }));
    document.getElementById('sandbox-panel').classList.remove('hidden');
    const title = document.getElementById('sandbox-title');
    if (title) title.textContent = entry.name;
    return frame;
}

// hideSandbox 关闭沙箱并释放 Blob URL。
export function hideSandbox() {
    const panel = document.getElementById('sandbox-panel');
    if (!panel || panel.classList.contains('hidden')) return;
    panel.classList.add('hidden');
    panel.classList.remove('expanded');
    const frame = document.getElementById('sandbox');
    const old = frame.src;
    frame.src = 'about:blank';
    if (old.startsWith('blob:')) URL.revokeObjectURL(old);
}

// wrapScript 为纯脚本补一层控制台捕获，异常与输出都留在沙箱里。
function wrapScript(source) {
    const safe = source.replace(/<\/script/gi, '<\\/script');
    return `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
body{margin:0;padding:12px;font-family:'Cascadia Code','Consolas',monospace;font-size:12px;background:#fff;color:#222}
pre{white-space:pre-wrap;word-break:break-word;margin:0}
</style></head><body><pre id="log"></pre><script>
(function(){
var out=document.getElementById('log');
function write(text){out.textContent+=text+'\\n';}
['log','info','warn','error'].forEach(function(level){
  var original=console[level];
  console[level]=function(){var parts=[].slice.call(arguments).map(function(v){
    try{return typeof v==='string'?v:JSON.stringify(v);}catch(e){return String(v);}
  });write('['+level+'] '+parts.join(' '));if(original)original.apply(console,arguments);};
});
window.addEventListener('error',function(event){write('[异常] '+event.message);});
window.addEventListener('unhandledrejection',function(event){write('[异常] '+event.reason);});
try{
${safe}
}catch(error){write('[异常] '+(error&&error.message?error.message:error));}
})();
</script></body></html>`;
}
