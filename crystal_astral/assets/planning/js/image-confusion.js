// ==== 图像混淆（本地节点实现） ====
// 算法与 local_data/package/lunar.image-confusion/index.html 完全一致：广义 Gilbert（希尔伯特）曲线
// 遍历像素 + 黄金比例偏移 [[Math.round((sqrt(5)-1)/2 * 像素总数)]] 做环形置换，使相邻像素在空间上被打散，
// 反向置换即解混淆。偏移量固定为黄金比例，才能与该工具互相混淆/解混淆（故不开放为参数）。
// 分层：纯算法（gilbertCurve / confuseOffset / confusePixels，可在无浏览器环境验证）
//      → 画布编解码（loadImageElement / canvas 逐像素置换）
//      → 节点执行（runImageConfusion）

// ==== 纯算法层 ====
// Gilbert 曲线坐标序列，扁平存放为 [x0,y0,x1,y1,...]，共 width*height 个点（顺序与参考实现 gilbert2d 一致）
function gilbertCurve(width, height) {
    const coords = new Int32Array(2 * width * height);
    let n = 0;
    const push = (x, y) => { coords[n++] = x; coords[n++] = y; };
    const gen = (x, y, ax, ay, bx, by) => {
        const w = Math.abs(ax + ay);
        const h = Math.abs(bx + by);
        const dax = Math.sign(ax), day = Math.sign(ay);
        const dbx = Math.sign(bx), dby = Math.sign(by);
        if (h === 1) { // 单行：沿 a 方向逐点
            for (let i = 0; i < w; i++) { push(x, y); x += dax; y += day; }
            return;
        }
        if (w === 1) { // 单列：沿 b 方向逐点
            for (let i = 0; i < h; i++) { push(x, y); x += dbx; y += dby; }
            return;
        }
        let ax2 = Math.floor(ax / 2), ay2 = Math.floor(ay / 2);
        let bx2 = Math.floor(bx / 2), by2 = Math.floor(by / 2);
        const w2 = Math.abs(ax2 + ay2);
        const h2 = Math.abs(bx2 + by2);
        if (2 * w > 3 * h) {
            if ((w2 % 2) && (w > 2)) { ax2 += dax; ay2 += day; }
            gen(x, y, ax2, ay2, bx, by);
            gen(x + ax2, y + ay2, ax - ax2, ay - ay2, bx, by);
        } else {
            if ((h2 % 2) && (h > 2)) { bx2 += dbx; by2 += dby; }
            gen(x, y, bx2, by2, ax2, ay2);
            gen(x + bx2, y + by2, ax, ay, bx - bx2, by - by2);
            gen(x + (ax - dax) + (bx2 - dbx), y + (ay - day) + (by2 - dby),
                -bx2, -by2, -(ax - ax2), -(ay - ay2));
        }
    };
    if (width >= height) gen(0, 0, width, 0, 0, height);
    else gen(0, 0, 0, height, width, 0);
    return coords;
}
// 黄金比例偏移（参考实现同式）：曲线上的第 i 个像素搬到第 (i + offset) % 总数 个位置
function confuseOffset(total) { return Math.round((Math.sqrt(5) - 1) / 2 * total); }
// 逐像素环形置换：op='confuse' 为混淆（曲线位置 i → i+offset），op='deconfuse' 为解混淆（逆向映射）
function confusePixels(src, width, height, op) {
    const total = width * height;
    const out = new Uint8ClampedArray(4 * total);
    const curve = gilbertCurve(width, height);
    const offset = confuseOffset(total);
    const reverse = op === 'deconfuse';
    for (let i = 0; i < total; i++) {
        const o = 2 * i, n = 2 * ((i + offset) % total);
        const oldP = 4 * (curve[o] + curve[o + 1] * width);
        const newP = 4 * (curve[n] + curve[n + 1] * width);
        const from = reverse ? newP : oldP;
        const to = reverse ? oldP : newP;
        out[to] = src[from]; out[to + 1] = src[from + 1]; out[to + 2] = src[from + 2]; out[to + 3] = src[from + 3];
    }
    return out;
}

// ==== 画布层 ====
// data URI / 裸 base64 → Image 元素（等待解码完成，保证 naturalWidth 可用）
function loadImageElement(src) {
    return new Promise((resolve, reject) => {
        const im = new Image();
        im.onload = () => resolve(im);
        im.onerror = () => reject(new Error('图像数据无法解码'));
        im.src = src;
    });
}

// ==== 节点执行 ====
// 运行模式：confuse=混淆 / deconfuse=解混淆 / restore=还原（不做像素变换，原样输出未混淆的原始图像，
// 对应参考工具的「还原」——节点没有文件历史，故以入线图像作为原始图像）
async function runImageConfusion(node) {
    const p = node.params || {};
    const link = incomingLinks(node.id).find(x => x.to.port === 'image');
    let val = p.image;
    if (link) { const sv = srcOut(nodeById(link.from.node), link.from.port); if (sv !== undefined && sv !== null) val = sv; }
    const md = findMedia(val); // 兼容 data URI / 裸 base64 / 上游对象包裹
    const src = (md && md.image) ? md.image : null;
    if (!src) { node.outValue = { error: '无图像输入（请连线或在参数中填入 base64 / data URI）' }; node._s = 'err'; return; }
    const mode = String(p.mode || 'confuse');
    if (mode === 'restore') { // 还原：原样透传原始图像，便于与混淆结果对照
        node.outValue = src; node._s = 'ok';
        node._pop = { kind: 'image', v: src };
        addLog({ dir: 'send', type: 'node:image_confuse', label: '图像混淆', req: '', env: { node: node.id }, isError: false, summary: '还原：原样输出未混淆的原始图像' });
        return;
    }
    try {
        const im = await loadImageElement(src);
        const w = im.naturalWidth || im.width || 0, h = im.naturalHeight || im.height || 0;
        if (!w || !h) { node.outValue = { error: '图像尺寸无效（0×0）' }; node._s = 'err'; return; }
        const cvs = document.createElement('canvas');
        cvs.width = w; cvs.height = h;
        const ctx = cvs.getContext('2d');
        ctx.drawImage(im, 0, 0, w, h);
        const before = ctx.getImageData(0, 0, w, h);
        const after = confusePixels(before.data, w, h, mode);
        const imgData = ctx.createImageData(w, h);
        imgData.data.set(after);
        ctx.putImageData(imgData, 0, 0);
        // PNG 无损（便于混淆→解混淆逐像素还原）；JPEG（体积小、有损）
        const fmt = String(p.format || 'jpeg');
        const uri = fmt === 'png' ? cvs.toDataURL('image/png') : cvs.toDataURL('image/jpeg', clampQuality(p.quality));
        node.outValue = uri; node._s = 'ok'; // 不回写 params.image：保持输入参数不变，重复运行结果一致
        node._pop = { kind: 'image', v: uri }; // 运行收尾弹出展示
        addLog({
            dir: 'send', type: 'node:image_confuse', label: '图像混淆', req: '', env: { node: node.id }, isError: false,
            summary: (mode === 'deconfuse' ? '解混淆' : '混淆') + ` ${w} × ${h} → ${fmt.toUpperCase()}`
        });
    } catch (e) {
        node.outValue = { error: '图像处理失败: ' + String(e && e.message || e) }; node._s = 'err';
    }
}
function clampQuality(v) {
    const q = Number(v);
    if (!isFinite(q) || q <= 0) return 0.95; // 与参考工具默认一致
    return Math.min(1, Math.max(0.1, q));
}
