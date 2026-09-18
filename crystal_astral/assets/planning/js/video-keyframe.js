// ==== 视频抽帧（本地节点实现） ====
// 流程：拉取视频URL → multipart 上传至琉璃 /keyframe 提取关键帧（后端每秒5帧 + 相似度去重）
//       → 前端把关键帧序列合成为 GIF 动图 → 输出 GIF base64（data URI）。
// 分层：GIF89a 编码器（中位切法量化 + LZW，可在无浏览器环境验证）
//      → 节点执行（runVideoKeyframe，依赖 image-confusion.js 的 loadImageElement）

// ==== GIF89a 编码器 ====
// 调色板：对所有帧抽样像素做中位切法，量化为 ≤256 色的全局调色板
// 像素索引：15 位色桶（5bit/通道）缓存就近调色板索引，避免逐像素全表检索
const GIF_SAMPLE_TARGET = 40000; // 调色板抽样像素总量上限（平衡质量与速度）

// 从多帧 RGBA 数据收集抽样像素（打包为 0xRRGGBB）
function gifCollectSamples(frames) {
    const total = frames.reduce((a, f) => a + (f.data.length / 4), 0);
    const stride = Math.max(1, Math.floor(total / GIF_SAMPLE_TARGET));
    const samples = [];
    let n = 0;
    for (const f of frames) {
        const d = f.data;
        for (let i = 0; i < d.length; i += 4) {
            if ((n++ % stride) !== 0) continue;
            samples.push((d[i] << 16) | (d[i + 1] << 8) | d[i + 2]);
        }
    }
    return samples;
}

// 创建色盒：统计盒内各通道极值，记录最宽通道与跨度
function gifBoxCreate(px) {
    const min = [255, 255, 255], max = [0, 0, 0];
    for (let i = 0; i < px.length; i++) {
        const v = px[i], r = v >> 16, g = (v >> 8) & 0xff, b = v & 0xff;
        if (r < min[0]) min[0] = r; if (r > max[0]) max[0] = r;
        if (g < min[1]) min[1] = g; if (g > max[1]) max[1] = g;
        if (b < min[2]) min[2] = b; if (b > max[2]) max[2] = b;
    }
    let ch = 0, range = -1;
    for (let c = 0; c < 3; c++) { const d = max[c] - min[c]; if (d > range) { range = d; ch = c; } }
    return { px, ch, range };
}

// 中位切法：反复沿最宽通道按像素数中位拆分色盒，直至达到 maxColors 或全部不可分
function gifMedianCut(samples, maxColors) {
    if (!samples.length) return [[0, 0, 0]];
    let boxes = [gifBoxCreate(samples)];
    while (boxes.length < maxColors) {
        // 找到跨度最大且可拆分（≥2 像素）的盒子
        let bi = -1, best = 0;
        for (let i = 0; i < boxes.length; i++) {
            if (boxes[i].px.length < 2) continue;
            if (boxes[i].range > best) { best = boxes[i].range; bi = i; }
        }
        if (bi < 0 || best <= 0) break; // 无可拆盒子（颜色已收敛）
        const box = boxes[bi], shift = [16, 8, 0][box.ch];
        const arr = box.px.slice().sort((a, b) => ((a >> shift) & 0xff) - ((b >> shift) & 0xff));
        const mid = arr.length >> 1;
        boxes.splice(bi, 1, gifBoxCreate(arr.slice(0, mid)), gifBoxCreate(arr.slice(mid)));
    }
    // 每个盒子的平均颜色作为调色板项
    return boxes.map(b => {
        let r = 0, g = 0, bl = 0;
        for (let i = 0; i < b.px.length; i++) {
            const v = b.px[i];
            r += v >> 16; g += (v >> 8) & 0xff; bl += v & 0xff;
        }
        const n = b.px.length || 1;
        return [Math.round(r / n), Math.round(g / n), Math.round(bl / n)];
    });
}

// 像素 → 调色板索引：15 位色桶缓存就近匹配结果（同桶像素共用一次检索）
function gifPaletteIndices(frames, palette) {
    const cache = new Int16Array(32768).fill(-1);
    const out = [];
    for (const f of frames) {
        const d = f.data, idx = new Uint8Array(f.width * f.height);
        for (let p = 0; p < idx.length; p++) {
            const r = d[4 * p], g = d[4 * p + 1], b = d[4 * p + 2];
            const key = ((r >> 3) << 10) | ((g >> 3) << 5) | (b >> 3);
            let ci = cache[key];
            if (ci < 0) {
                let bd = Infinity; ci = 0;
                for (let i = 0; i < palette.length; i++) {
                    const dr = r - palette[i][0], dg = g - palette[i][1], db = b - palette[i][2];
                    const dist = dr * dr + dg * dg + db * db;
                    if (dist < bd) { bd = dist; ci = i; }
                }
                cache[key] = ci;
            }
            idx[p] = ci;
        }
        out.push(idx);
    }
    return out;
}

// GIF LZW 压缩（可变码长 9~12 位，字典满 4096 时发清除码重置）
function gifLzwEncode(pixels, minCodeSize) {
    const clearCode = 1 << minCodeSize, eoiCode = clearCode + 1;
    let codeSize = minCodeSize + 1, codeMask = (1 << codeSize) - 1, freeCode = eoiCode + 1;
    const bytes = [];
    let cur = 0, curShift = 0;
    const emit = c => { // LSB-first 写码；发射后按需提升码长（omggif 同规则）
        cur |= c << curShift;
        curShift += codeSize;
        while (curShift >= 8) { bytes.push(cur & 0xff); cur >>= 8; curShift -= 8; }
        if (codeSize < 12 && freeCode > codeMask) { codeSize++; codeMask = (1 << codeSize) - 1; }
    };
    const dict = new Map();
    emit(clearCode);
    let ib = pixels[0];
    for (let i = 1; i < pixels.length; i++) {
        const k = pixels[i], key = (ib << 8) | k;
        const code = dict.get(key);
        if (code !== undefined) { ib = code; continue; }
        emit(ib);
        if (freeCode < 4096) dict.set(key, freeCode++);
        else { // 字典满：发清除码重置（不发新表项）
            emit(clearCode);
            dict.clear(); codeSize = minCodeSize + 1; codeMask = (1 << codeSize) - 1; freeCode = eoiCode + 1;
        }
        ib = k;
    }
    emit(ib); emit(eoiCode);
    if (curShift > 0) bytes.push(cur & 0xff);
    return bytes;
}

// 多帧 ImageData → GIF89a 字节流（无限循环；delayCs 帧间隔，单位 1/100 秒）
function encodeGif(frames, delayCs) {
    const w = frames[0].width, h = frames[0].height;
    const palette = gifMedianCut(gifCollectSamples(frames), 256);
    const indices = gifPaletteIndices(frames, palette);
    const minCodeSize = 8;
    const out = [];
    const w16 = v => { out.push(v & 0xff, (v >> 8) & 0xff); };
    // 文件头 + 逻辑屏幕描述符（全局色表 256 色）
    out.push(0x47, 0x49, 0x46, 0x38, 0x39, 0x61); // 'GIF89a'
    w16(w); w16(h);
    out.push(0xf7, 0x00, 0x00); // 全局色表标志|色深7|表尺寸7(256项)，背景0，宽高比0
    for (let i = 0; i < 256; i++) { // 全局色表（不足补黑）
        const c = palette[i] || [0, 0, 0];
        out.push(c[0], c[1], c[2]);
    }
    // 网景扩展：无限循环
    out.push(0x21, 0xff, 0x0b, 0x4e, 0x45, 0x54, 0x53, 0x43, 0x41, 0x50, 0x45, 0x32, 0x2e, 0x30, 0x03, 0x01);
    w16(0); out.push(0x00);
    // 逐帧：图形控制扩展 + 图像描述符 + LZW 数据
    for (const idx of indices) {
        out.push(0x21, 0xf9, 0x04, 0x04); // 处置方式=不处置
        w16(delayCs); out.push(0x00, 0x00); // 延迟，无透明色
        out.push(0x2c); w16(0); w16(0); w16(w); w16(h); out.push(0x00); // 无局部色表/不隔行
        out.push(minCodeSize);
        const lzw = gifLzwEncode(idx, minCodeSize);
        for (let i = 0; i < lzw.length; i += 255) { // 数据子块 ≤255 字节
            const n = Math.min(255, lzw.length - i);
            out.push(n);
            for (let j = 0; j < n; j++) out.push(lzw[i + j]);
        }
        out.push(0x00); // 子块结束符
    }
    out.push(0x3b); // 结束符
    return new Uint8Array(out);
}

// ==== 节点执行 ====
// 视频URL（上游连线优先）→ 拉取视频 → POST /keyframe（multipart video 字段）→ 关键帧 data(base64)
// → 等比缩放绘制 canvas → 合成 GIF → 输出 data URI（连线到图像显示可直接展示）
const KEYFRAME_VIDEO_EXTS = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.mkv', '.webm', '.m4v']; // 与后端支持格式一致
async function runVideoKeyframe(node) {
    const p = node.params || {};
    // 1. 取视频URL：上游连线覆盖参数
    const link = incomingLinks(node.id).find(x => x.to.port === 'url');
    let url = p.url;
    if (link) { const sv = srcOut(nodeById(link.from.node), link.from.port); if (sv !== undefined && sv !== null) url = sv; }
    url = typeof url === 'string' ? url.trim() : '';
    if (!url) { node.outValue = { error: '无视频URL（请连线或在参数中填写）' }; node._s = 'err'; return; }
    // 2. 拉取视频（琉璃同源资源或允许跨域的外部资源）
    let blob;
    try {
        const resp = await fetch(url);
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        blob = await resp.blob();
    } catch (e) { node.outValue = { error: '拉取视频失败: ' + String(e && e.message || e) }; node._s = 'err'; return; }
    if (!blob.size) { node.outValue = { error: '视频内容为空' }; node._s = 'err'; return; }
    // 3. 上传至 /keyframe（文件名需带后端支持的视频扩展名，否则默认 .mp4）
    let ext = '';
    try { ext = (new URL(url, location.href).pathname.match(/\.[a-z0-9]+$/i) || [''])[0].toLowerCase(); } catch (e) { /* 非法URL则用默认扩展名 */ }
    const filename = 'video' + (KEYFRAME_VIDEO_EXTS.includes(ext) ? ext : '.mp4');
    let data;
    try {
        const fd = new FormData(); fd.append('video', blob, filename);
        const resp = await fetch('/keyframe', { method: 'POST', body: fd });
        const text = await resp.text();
        if (!resp.ok) throw new Error(text.replace(/^ExtractKeyFrames请求\[ERROR\] -> /, '').slice(0, 300));
        data = JSON.parse(text);
    } catch (e) { node.outValue = { error: '关键帧提取失败: ' + String(e && e.message || e) }; node._s = 'err'; return; }
    const frames = (data && data.keyFrames) || [];
    if (!frames.length) { node.outValue = { error: '未提取到关键帧' }; node._s = 'err'; return; }
    // 4. 关键帧 → 等比缩放的 ImageData（data 字段为 JPEG base64）
    const maxWidth = Math.min(1024, Math.max(16, Number(p.maxWidth) || 480));
    const maxFrames = Math.max(0, Number(p.maxFrames) || 0);
    const list = maxFrames > 0 ? frames.slice(0, maxFrames) : frames;
    const imageDatas = [];
    try {
        for (const f of list) {
            if (!f || !f.data) continue;
            const im = await loadImageElement('data:image/jpeg;base64,' + f.data);
            const scale = Math.min(1, maxWidth / (im.naturalWidth || im.width || maxWidth));
            const w = Math.max(1, Math.round((im.naturalWidth || im.width || 1) * scale));
            const h = Math.max(1, Math.round((im.naturalHeight || im.height || 1) * scale));
            const cvs = document.createElement('canvas'); cvs.width = w; cvs.height = h;
            const ctx = cvs.getContext('2d');
            ctx.drawImage(im, 0, 0, w, h);
            imageDatas.push(ctx.getImageData(0, 0, w, h));
        }
    } catch (e) { node.outValue = { error: '关键帧解码失败: ' + String(e && e.message || e) }; node._s = 'err'; return; }
    if (!imageDatas.length) { node.outValue = { error: '关键帧解码为空' }; node._s = 'err'; return; }
    // 5. 合成 GIF（帧间隔按 1/100 秒取整，最小 2 即 20ms，低于此值播放器会忽略）
    const delayCs = Math.max(2, Math.round((Number(p.delayMs) || 200) / 10));
    let uri;
    try {
        const bytes = encodeGif(imageDatas, delayCs);
        let bin = '';
        for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
        uri = 'data:image/gif;base64,' + btoa(bin);
    } catch (e) { node.outValue = { error: 'GIF 合成失败: ' + String(e && e.message || e) }; node._s = 'err'; return; }
    node.outValue = uri; node._s = 'ok';
    node._pop = { kind: 'image', v: uri }; // 运行收尾弹出展示
    addLog({
        dir: 'send', type: 'node:video_keyframe', label: '视频抽帧', req: url, env: { node: node.id }, isError: false,
        summary: `关键帧 ${imageDatas.length} 帧（后端 ${frames.length} 帧）→ GIF ${imageDatas[0].width}×${imageDatas[0].height} × ${delayCs}00ms`
    });
}
