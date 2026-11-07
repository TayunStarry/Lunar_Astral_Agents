// ============================================================
//  星月智能 · 消息终端 — 音频播放队列（共享层适配）
//  桌面端保持直连输出（不启用移动端音色增强链路）。
// ============================================================

const AudioQueue = window.LunarShared
    ? window.LunarShared.createAudioQueue({ enhance: false })
    // 共享层缺失时先给空实现：由 shared-boot.js 展示错误层并阻止后续初始化
    : { enqueue() { }, stop() { } };
