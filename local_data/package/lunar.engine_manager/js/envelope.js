// ==== 信封构建 ====
function gp(p, key) { return p[key]; }
function parseVal(v, arr) {
    if (typeof v !== 'string') return v; // 已是对象/数组
    const t = v.trim(); if (!t) return arr ? [] : {};
    try { return JSON.parse(t); } catch (e) { return v; }
}
function buildEnvelope(node) {
    const meta = metaOf(node.type), p = node.params || {};
    const s = (k) => { const v = gp(p, k); return v == null ? '' : (typeof v === 'string' ? v : JSON.stringify(v)); };
    switch (node.type) {
        case 'start': case 'event_start': case 'clock_start': case 'wait': case 'transform': case 'display': case 'and': case 'or': case 'not': case 'nand': case 'nor':
            return null;
        case 'event': {
            if (!(p.topic || '').trim()) { toast('请填写事件主题', 'error'); return null; }
            return { type: 'ltp9/event', topic: p.topic.trim(), payload: parseVal(gp(p, 'payload')) };
        }
        case 'broadcast_all':
            if (!gp(p, 'payload')) { toast('请填写广播载荷', 'error'); return null; }
            return { type: 'ltp9/broadcast', payload: parseVal(gp(p, 'payload')) };
        case 'broadcast_target': {
            if (!(p.target || '').trim()) { toast('请填写目标包ID', 'error'); return null; }
            return { type: 'ltp9/test', action: 'broadcast_target', target: p.target.trim(), payload: parseVal(gp(p, 'payload')) };
        }
        case 'call': {
            if (!(p.plugin || '').trim() || !(p.fn || '').trim()) { toast('请填写目标包与函数名', 'error'); return null; }
            let args = [];
            // 输入数据连线注入优先：作为首个参数
            const linked = gp(p, 'input');
            if (linked !== undefined && linked !== null && linked !== '') {
                args = [linked];
            } else {
                const raw = String(gp(p, 'args') != null ? gp(p, 'args') : '').trim();
                if (raw) {
                    try { const j = JSON.parse(raw); args = Array.isArray(j) ? j : [j]; }
                    catch (e) { args = [raw]; } // 单个文本 → 首个参数
                }
            }
            return { type: 'ltp9/call', plugin: p.plugin.trim(), fn: p.fn.trim(), args };
        }
        case 'agent': {
            if (!(p.plugin || '').trim() || !gp(p, 'instruction')) { toast('请填写智能体包ID与指令', 'error'); return null; }
            return { type: 'ltp9/test', action: 'agent', plugin: p.plugin.trim(), instruction: String(gp(p, 'instruction')) };
        }
        case 'db': {
            let params = parseVal(gp(p, 'params'), true); if (!Array.isArray(params)) params = [];
            return { type: 'ltp9/test', action: 'db', op: p.op || 'query', sql: s('sql'), params };
        }
        case 'memory':
            return { type: 'ltp9/test', action: 'memory', op: p.op || 'store', params: parseVal(gp(p, 'params')) };
        case 'file':
            return { type: 'ltp9/test', action: 'file', op: p.op || 'write', path: s('path'), data: s('data') };
        case 'crypto':
            return { type: 'ltp9/test', action: 'crypto', op: p.op || 'encode', key: s('key'), content: s('content') };
        case 'base64':
            return { type: 'ltp9/test', action: 'base64', op: p.op || 'encode', content: s('content') };
        case 'jwt':
            return { type: 'ltp9/test', action: 'jwt', claims: parseVal(gp(p, 'claims')) || {}, secret: s('secret'), alg: p.alg || 'HS256', kid: s('kid') };
        case 'llm': {
            const opts = parseVal(gp(p, 'opts')) || {};
            const raw = String(gp(p, 'content') != null ? gp(p, 'content') : '').trim();
            if (!raw) { toast('请填写消息内容', 'error'); return null; }
            // 单消息框：可解析且符合 [{role,content}] 消息数组格式 → 视为消息列表；否则视为一条用户消息
            let messages = null;
            try {
                const j = JSON.parse(raw);
                if (Array.isArray(j) && j.length && j.every(mm => mm && typeof mm.role === 'string')) messages = j;
            } catch (e) { /* 非 JSON → 单条用户消息 */ }
            if (!messages) messages = [{ role: 'user', content: raw }];
            if ((p.system || '').trim()) opts.system = p.system.trim();
            return { type: 'ltp9/test', action: 'llm', messages, opts };
        }
        case 'command': {
            if (!(p.text || '').trim()) { toast('请填写指令文本', 'error'); return null; }
            const env = { type: 'ltp9/test', action: 'command', text: p.text.trim(), context: parseVal(gp(p, 'context')) || {} };
            if ((p.plugin || '').trim()) env.plugin = p.plugin.trim();
            return env;
        }
        case 'tool': case 'platform':
            // LTP9-Flash 已移除 tool / platform 能力：节点不再产出信封
            toast('该能力已在 LTP9-Flash 中移除：' + node.type, 'error');
            return null;
        case 'emoji': {
            const op = p.op || 'search';
            const params = { query: s('query'), limit: Number(p.limit) || 5, image: s('image') };
            return { type: 'ltp9/test', action: 'emoji', op, params };
        }
        case 'embed': {
            if (!gp(p, 'input') && String(s('input')).trim() === '') { toast('请填写待嵌入文本', 'error'); return null; }
            return { type: 'ltp9/test', action: 'embed', input: parseVal(gp(p, 'input')) };
        }
        case 'kokoro_tts': {
            if (!(p.text || '').trim()) { toast('请填写合成文本', 'error'); return null; }
            return { type: 'ltp9/test', action: 'kokoro_tts', text: s('text'), voice: (p.voice || '').trim(), speed: Number(p.speed) || 1, lang: s('lang') || 'auto' };
        }
        case 'qwen_tts': {
            if (!(p.text || '').trim()) { toast('请填写合成文本', 'error'); return null; }
            const env = { type: 'ltp9/test', action: 'qwen_tts', text: s('text') };
            if ((p.ref_audio || '').trim()) env.ref_audio = p.ref_audio.trim();
            return env;
        }
        case 'qwen_asr': {
            if (!(p.audio || '').trim()) { toast('请填写音频 base64', 'error'); return null; }
            return { type: 'ltp9/test', action: 'qwen_asr', audio: s('audio'), format: p.format || 'wav' };
        }
        case 'network':
            return { type: 'ltp9/test', action: 'network', op: p.op || 'resolveDNS', args: {
                host: s('host'), port: Number(p.port) || 0, service: s('service'), proto: s('proto'),
                handle: gp(p, 'handle') != null ? String(gp(p, 'handle')) : '',
                data: gp(p, 'data') != null ? String(gp(p, 'data')) : '',
                timeout: Number(p.timeout) || 0
            } };
        case 'async':
            return { type: 'ltp9/test', action: 'async', op: p.op || 'run', params: {
                plugin: (p.plugin || '').trim(),
                fn: (p.fn || '').trim(),
                args: parseVal(gp(p, 'args'), true),
                ms: Number(p.ms) || 0,
                timeout: Number(p.timeout) || 0,
                task_id: Number(p.taskId) || 0,
                progress: parseVal(gp(p, 'progress'))
            } };
        case 'http':
            return { type: 'ltp9/test', action: 'http', method: p.method || 'GET', url: s('url'), body: s('body'), headers: parseVal(gp(p, 'headers')) || {} };
        case 'stats':
            return { type: 'ltp9/stats' };
    }
    return null;
}