// ===== 媒体处理能力（消息入口） =====
//
// 消息入口只做「入库 + 句柄」：视频与动态图不再自动解析，内容描述由句柄系统
// （Go 侧 agents/Descriptor）在 use_resource 时按需生成并写回。
//
// 模块划分：
//   - admission.ts 媒体入库、命名
//   - video.ts     视频入库（只出句柄）
//   - audio.ts     音频转写与资源化
//   - pipeline.ts  未读消息中的媒体原位置换流水线

export { LiteImageFile } from './media/pipeline';
