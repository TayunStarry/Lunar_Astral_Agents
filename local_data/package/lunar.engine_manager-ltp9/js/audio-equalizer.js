// ==== 音频均衡器（本地节点实现） ====
// 三段均衡：低架(low shelf) / 峰值(peaking) / 高架(high shelf)，系数按 RBJ Audio EQ Cookbook 计算，
// 逐声道串联处理（Direct Form I），最后乘总输出增益，输出 16bit PCM WAV（base64 data URI，扬声器节点可直接连线播放）。
// 解码：WAV/RIFF 走内置解析（PCM 8/16/24/32bit 与 IEEE float，不依赖浏览器解码能力）；
//       其它格式（webm/opus、mp3、ogg、m4a）走 Web Audio decodeAudioData。
// 分层：纯 DSP / 编解码（biquadCoeffs / biquadApply / wavParse / wavEncode16，可在无浏览器环境验证）→ 节点执行（runAudioEqualizer）

// ==== base64 ↔ 字节 ====
function bytesToBase64(bytes) {
    let s = '';
    const CH = 0x8000; // 分块避免超长参数导致调用栈溢出
    for (let i = 0; i < bytes.length; i += CH) s += String.fromCharCode.apply(null, bytes.subarray(i, i + CH));
    return btoa(s);
}
function base64ToBytes(b64) {
    const bin = atob(String(b64).replace(/\s/g, ''));
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
}

// ==== WAV 解析（→ 分声道 Float32） ====
function wavParse(bytes) {
    const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    if (bytes.byteLength < 44 || dv.getUint32(0, false) !== 0x52494646 || dv.getUint32(8, false) !== 0x57415645) return null;
    let off = 12, fmt = null, dataOff = -1, dataLen = 0;
    while (off + 8 <= bytes.byteLength) {
        const id = dv.getUint32(off, false);
        const size = dv.getUint32(off + 4, true);
        const body = off + 8;
        if (id === 0x666d7420 && size >= 16) { // 'fmt '
            fmt = {
                format: dv.getUint16(body, true),
                channels: dv.getUint16(body + 2, true),
                sampleRate: dv.getUint32(body + 4, true),
                bits: dv.getUint16(body + 14, true)
            };
            if (fmt.format === 0xfffe && size >= 40) fmt.format = dv.getUint16(body + 24, true); // WAVE_FORMAT_EXTENSIBLE：取子格式
        } else if (id === 0x64617461) { dataOff = body; dataLen = size; } // 'data'
        off = body + size + (size % 2);
    }
    if (!fmt || dataOff < 0 || !fmt.channels || !fmt.sampleRate) return null;
    const bytesPerSample = fmt.bits >> 3;
    if (!bytesPerSample) return null;
    const frames = Math.min(Math.floor(dataLen / (bytesPerSample * fmt.channels)), Math.floor((bytes.byteLength - dataOff) / (bytesPerSample * fmt.channels)));
    if (frames <= 0) return null;
    const channels = [];
    for (let c = 0; c < fmt.channels; c++) channels.push(new Float32Array(frames));
    const isFloat = fmt.format === 3;
    for (let i = 0; i < frames; i++) {
        for (let c = 0; c < fmt.channels; c++) {
            const at = dataOff + (i * fmt.channels + c) * bytesPerSample;
            let v = 0;
            if (isFloat) v = fmt.bits === 64 ? dv.getFloat64(at, true) : dv.getFloat32(at, true);
            else if (fmt.bits === 8) v = (dv.getUint8(at) - 128) / 128; // 8bit 为无符号
            else if (fmt.bits === 16) v = dv.getInt16(at, true) / 32768;
            else if (fmt.bits === 24) {
                const b0 = dv.getUint8(at), b1 = dv.getUint8(at + 1), b2 = dv.getUint8(at + 2);
                let n = b0 | (b1 << 8) | (b2 << 16);
                if (n & 0x800000) n -= 0x1000000; // 符号扩展
                v = n / 8388608;
            } else if (fmt.bits === 32) v = dv.getInt32(at, true) / 2147483648;
            else return null; // 其它位深不支持
            channels[c][i] = v;
        }
    }
    return { sampleRate: fmt.sampleRate, channels };
}
// ==== WAV 编码（16bit PCM，输入分声道 Float32，内部做 ±1 钳位） ====
function wavEncode16(channels, sampleRate) {
    const ch = channels.length, frames = channels[0] ? channels[0].length : 0;
    const bytes = new Uint8Array(44 + frames * ch * 2);
    const dv = new DataView(bytes.buffer);
    const ascii = (at, s) => { for (let i = 0; i < s.length; i++) dv.setUint8(at + i, s.charCodeAt(i)); };
    ascii(0, 'RIFF'); dv.setUint32(4, 36 + frames * ch * 2, true); ascii(8, 'WAVE');
    ascii(12, 'fmt '); dv.setUint32(16, 16, true);
    dv.setUint16(20, 1, true); dv.setUint16(22, ch, true);
    dv.setUint32(24, sampleRate, true); dv.setUint32(28, sampleRate * ch * 2, true);
    dv.setUint16(32, ch * 2, true); dv.setUint16(34, 16, true);
    ascii(36, 'data'); dv.setUint32(40, frames * ch * 2, true);
    let at = 44;
    for (let i = 0; i < frames; i++) {
        for (let c = 0; c < ch; c++) {
            const v = Math.max(-1, Math.min(1, channels[c][i] || 0));
            dv.setInt16(at, v < 0 ? v * 32768 : v * 32767, true);
            at += 2;
        }
    }
    return bytes;
}

// ==== 双二阶滤波器（RBJ Cookbook） ====
function eqNum(v, def) { const n = Number(v); return isFinite(n) ? n : def; }
function eqClamp(v, lo, hi) { return Math.min(hi, Math.max(lo, v)); }
function biquadCoeffs(kind, freq, sampleRate, gainDb, q) {
    const A = Math.pow(10, eqNum(gainDb, 0) / 40);
    const w0 = 2 * Math.PI * eqClamp(eqNum(freq, 1000), 1, sampleRate * 0.495) / sampleRate;
    const cosw = Math.cos(w0), sinw = Math.sin(w0);
    let b0, b1, b2, a0, a1, a2;
    if (kind === 'lowshelf' || kind === 'highshelf') {
        const tsa = 2 * Math.sqrt(A) * (sinw / 2 * Math.SQRT2); // 架式斜率 S = 1
        const Ap1 = A + 1, Am1 = A - 1;
        if (kind === 'lowshelf') {
            b0 = A * (Ap1 - Am1 * cosw + tsa); b1 = 2 * A * (Am1 - Ap1 * cosw); b2 = A * (Ap1 - Am1 * cosw - tsa);
            a0 = Ap1 + Am1 * cosw + tsa; a1 = -2 * (Am1 + Ap1 * cosw); a2 = Ap1 + Am1 * cosw - tsa;
        } else {
            b0 = A * (Ap1 + Am1 * cosw + tsa); b1 = -2 * A * (Am1 + Ap1 * cosw); b2 = A * (Ap1 + Am1 * cosw - tsa);
            a0 = Ap1 - Am1 * cosw + tsa; a1 = 2 * (Am1 - Ap1 * cosw); a2 = Ap1 - Am1 * cosw - tsa;
        }
    } else { // peaking
        const alpha = sinw / (2 * eqClamp(eqNum(q, 1), 0.05, 20));
        b0 = 1 + alpha * A; b1 = -2 * cosw; b2 = 1 - alpha * A;
        a0 = 1 + alpha / A; a1 = -2 * cosw; a2 = 1 - alpha / A;
    }
    return { b0: b0 / a0, b1: b1 / a0, b2: b2 / a0, a1: a1 / a0, a2: a2 / a0 };
}
// 就地串联处理（Direct Form I）；增益为 0dB 时系数退化为恒等
function biquadApply(c, data) {
    let x1 = 0, x2 = 0, y1 = 0, y2 = 0;
    for (let i = 0; i < data.length; i++) {
        const x = data[i];
        const y = c.b0 * x + c.b1 * x1 + c.b2 * x2 - c.a1 * y1 - c.a2 * y2;
        x2 = x1; x1 = x; y2 = y1; y1 = y;
        data[i] = Math.max(-4, Math.min(4, y)); // 内部浮点不削波（仅防极端参数发散），输出编码时再钳位
    }
}
// 三段滤波器系数：低架(低频) → 峰值(中频) → 高架(高频)
function eqBands(p, sampleRate) {
    return [
        biquadCoeffs('lowshelf', eqNum(p.lowFreq, 250), sampleRate, eqClamp(eqNum(p.lowGain, 0), -24, 24)),
        biquadCoeffs('peaking', eqNum(p.midFreq, 1000), sampleRate, eqClamp(eqNum(p.midGain, 0), -24, 24), eqNum(p.midQ, 1)),
        biquadCoeffs('highshelf', eqNum(p.highFreq, 4000), sampleRate, eqClamp(eqNum(p.highGain, 0), -24, 24))
    ];
}

// ==== 解码入口 ====
// 压缩格式（webm/opus、mp3、ogg、m4a）交给 Web Audio；注意 Web Audio 会把解码结果重采样到音频上下文
// 采样率（通常 48kHz），故压缩输入的输出采样率随浏览器而定；WAV 输入由内置解析处理，采样率原样保留。
async function decodeAudioViaWebAudio(bytes) {
    const AC = (typeof window !== 'undefined') && (window.AudioContext || window.webkitAudioContext);
    if (!AC) return null;
    const ctx = new AC();
    try {
        const ab = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
        const buf = await new Promise((resolve, reject) => {
            const r = ctx.decodeAudioData(ab, resolve, reject); // 兼容回调式实现
            if (r && typeof r.then === 'function') r.then(resolve, reject);
        });
        const channels = [];
        for (let i = 0; i < buf.numberOfChannels; i++) channels.push(Float32Array.from(buf.getChannelData(i)));
        return { sampleRate: buf.sampleRate || 44100, channels };
    } finally { try { if (ctx.close) await ctx.close(); } catch (e) { /* 忽略关闭异常 */ } }
}
function audioSourceOf(val) {
    const md = findMedia(val);
    if (md && md.audio) return md.audio;
    if (typeof val === 'string' && val.trim()) return b64ToDataUri(val.trim()); // 兜底：裸 base64（按 magic bytes 判类型）
    return null;
}
async function decodeAudioPcm(uri) {
    const comma = uri.indexOf(',');
    const bytes = base64ToBytes(comma >= 0 ? uri.slice(comma + 1) : uri);
    if (!bytes.length) throw new Error('音频数据为空');
    const isWav = bytes.length > 12 && bytes[0] === 0x52 && bytes[1] === 0x49 && bytes[2] === 0x46 && bytes[3] === 0x46
        && bytes[8] === 0x57 && bytes[9] === 0x41 && bytes[10] === 0x56 && bytes[11] === 0x45;
    if (isWav) { const w = wavParse(bytes); if (w) return w; }
    const wa = await decodeAudioViaWebAudio(bytes);
    if (wa) return wa;
    throw new Error(isWav ? 'WAV 解析失败（仅支持 PCM / IEEE float 编码）' : '无法解码该音频（浏览器不支持该格式，或先转为 WAV）');
}

// ==== 节点执行 ====
async function runAudioEqualizer(node) {
    const p = node.params || {};
    const link = incomingLinks(node.id).find(x => x.to.port === 'audio');
    let val = p.audio;
    if (link) { const sv = srcOut(nodeById(link.from.node), link.from.port); if (sv !== undefined && sv !== null) val = sv; }
    const uri = audioSourceOf(val);
    if (!uri) { node.outValue = { error: '无音频输入（请连线或在参数中填入 base64 / data URI）' }; node._s = 'err'; return; }
    try {
        const pcm = await decodeAudioPcm(uri);
        if (!pcm.channels.length || !pcm.channels[0].length) { node.outValue = { error: '音频长度为 0' }; node._s = 'err'; return; }
        const bands = eqBands(p, pcm.sampleRate);
        const gain = Math.pow(10, eqClamp(eqNum(p.outGain, 0), -48, 24) / 20);
        let peak = 0;
        pcm.channels.forEach(ch => {
            bands.forEach(c => biquadApply(c, ch));
            if (gain !== 1) for (let i = 0; i < ch.length; i++) ch[i] *= gain;
            for (let i = 0; i < ch.length; i++) { const a = Math.abs(ch[i]); if (a > peak) peak = a; }
        });
        const clipped = peak > 1; // 削波只在编码前钳位，便于日志提示
        const uriOut = 'data:audio/wav;base64,' + bytesToBase64(wavEncode16(pcm.channels, pcm.sampleRate));
        node.outValue = uriOut; node._s = 'ok'; // 不回写 params.audio：避免巨大 base64 进入蓝图存档
        addLog({
            dir: 'send', type: 'node:audio_eq', label: '音频均衡器', req: '', env: { node: node.id }, isError: false,
            summary: `三段均衡 低${fmtDb(p.lowGain)}/中${fmtDb(p.midGain)}/高${fmtDb(p.highGain)} dB · ${pcm.channels.length}声道 ${pcm.sampleRate}Hz · 峰值 ${peak.toFixed(2)}`
                + (clipped ? '（已削波，建议降低增益）' : '')
        });
    } catch (e) {
        node.outValue = { error: '音频处理失败: ' + String(e && e.message || e) }; node._s = 'err';
    }
}
function fmtDb(v) { const n = eqNum(v, 0); return (n > 0 ? '+' : '') + n.toFixed(1); }
