// ═══════════════════════════════════════════════════════════
// 视频抽帧（video.*）
// ═══════════════════════════════════════════════════════════

/** 视频抽帧 */
export const video: {
	/**
	 * 从视频抽取多帧并编码为图片
	 *
	 * @param source 相对数据目录的视频路径，或 http(s) URL，或 data:video/...;base64 URI
	 * @param opts 可选参数（采样方式三选一，互斥优先级：times > count > fps）：
	 *             times 显式时间点（秒）数组，精确抽取
	 *             count 等距采样帧数（1-60，默认不启用）
	 *             fps   均匀抽帧频率（默认 5），超帧数上限自动等距抽样
	 *             dedup 相邻帧相似度去重（默认 true）
	 *             max_dim 帧等比缩放的边长上限（默认 640，超出则缩放）
	 *             format 帧编码格式（jpeg/png/webp，默认 jpeg）
	 *             quality jpeg/webp 质量（1-100，默认 85）
	 * @returns 成功返回 { success:true, frames:[{ data:dataURI, timestamp, width, height, format, index }], count, skipped? }；
	 *          skipped 为抽取失败或去重跳过的帧数（仅在大于 0 时出现）；失败返回 failure
	 */
	frames(source: string, opts?: { times?: number[]; count?: number; fps?: number; dedup?: boolean; max_dim?: number; format?: 'jpeg' | 'png' | 'webp'; quality?: number }): { success: boolean; frames?: Array<{ data: string; timestamp: string; width: number; height: number; format: string; index: number }>; count?: number; skipped?: number; error?: string }
}
