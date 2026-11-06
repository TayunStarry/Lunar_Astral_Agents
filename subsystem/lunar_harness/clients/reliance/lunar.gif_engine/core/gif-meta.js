// ==== gif-meta.js — GIF 帧延迟解析 ====
//
// 只走文件头与块结构，不解码像素：
//   遍历扩展块 / 图像描述符，累加图形控制扩展（GCE）的 delay（单位 1/100 秒），
//   得到整段动画一轮的播放时长，供播放器「播完再换下一张」使用。
// 非法结构、非 GIF、无帧时返回 0，由调用方按兜底时长处理。

/** 单轮播放时长的上下限（防止异常文件把排程卡死） */
const MIN_DURATION = 300;
const MAX_DURATION = 30000;

/**
 * 读取 GIF 一轮播放时长。
 * @param {Uint8Array} bytes GIF 原始字节
 * @returns {number} 毫秒；无法解析时为 0
 */
export function readGifDuration(bytes) {
    const len = bytes ? bytes.length : 0;
    if (len < 13) return 0;
    if (String.fromCharCode(bytes[0], bytes[1], bytes[2]) !== 'GIF') return 0;

    const packed = bytes[10];
    let pos = 13;
    // 全局颜色表：存在时按 3 字节 × 2^(N+1) 跳过
    if (packed & 0x80) pos += 3 * (1 << ((packed & 0x07) + 1));

    let total = 0;
    let delay = 0;
    let frames = 0;

    while (pos < len) {
        const marker = bytes[pos];
        if (marker === 0x3b) break; // 文件结束
        if (marker === 0x21) {
            const label = bytes[pos + 1];
            if (label === 0xf9 && pos + 7 < len) {
                // 图形控制扩展：延迟为小端序两字节，单位 1/100 秒
                delay = (bytes[pos + 4] | (bytes[pos + 5] << 8)) * 10;
            }
            pos = skipSubBlocks(bytes, pos + 2);
            continue;
        }
        if (marker === 0x2c) {
            if (pos + 9 >= len) break;
            const imagePacked = bytes[pos + 9];
            let next = pos + 10;
            // 局部颜色表
            if (imagePacked & 0x80) next += 3 * (1 << ((imagePacked & 0x07) + 1));
            if (next >= len) break;
            // 跳过一个字节的 LZW 最小码长，再跳过数据子块
            total += delay > 0 ? delay : 100; // 缺省 GCE 时浏览器按 100ms 处理
            frames++;
            delay = 0;
            pos = skipSubBlocks(bytes, next + 1);
            continue;
        }
        pos++; // 未知标记：前进一字节，避免死循环
    }

    if (frames === 0) return 0;
    return Math.min(MAX_DURATION, Math.max(MIN_DURATION, total));
}

/** 从 start（子块长度字节）开始跳过整段子块，返回终止符之后的位置 */
function skipSubBlocks(bytes, start) {
    let pos = start;
    const len = bytes.length;
    while (pos < len) {
        const size = bytes[pos++];
        if (size === 0) return pos;
        pos += size;
    }
    return pos;
}
