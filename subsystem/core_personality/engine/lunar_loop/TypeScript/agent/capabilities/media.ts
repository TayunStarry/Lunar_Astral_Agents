// ===== 媒体理解能力（感知者侧） =====
//
// 三类来源（前端已落盘的本地文件 / QQ 等渠道的远程 URL / 内联 data URI）在 Go 侧
// 统一归一，本模块只负责「拿句柄 → 写理解结论 → 产出带句柄的文本」。
//
// 为什么视频/音频先本地化再登记：本地化（转码/分段/WAV 归一）本来就要下载一次，
// 登记其产物可以避免对同一远程地址重复下载；产物落在 multimedia/footage 与临时目录，
// 由 Go 侧按内容哈希去重。
//
// 模块划分：
//   - registry.ts      角色单例注入（解除 media → roles 的静态循环引用）
//   - admission.ts     媒体入库、命名与理解结论写回
//   - understanding.ts 观看与缓存（视频 / 序列帧共用）
//   - video.ts         视频理解与资源化
//   - audio.ts         音频转写与资源化
//   - animated.ts      动态图理解与原位置换
//   - pipeline.ts      未读消息中的媒体原位置换流水线

export { LiteImageFile } from './media/pipeline';
export { registerMediaRoles } from './media/registry';
