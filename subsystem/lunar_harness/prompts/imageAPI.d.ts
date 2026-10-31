// ═══════════════════════════════════════════════════════════
// 图像处理（image.*）
// ═══════════════════════════════════════════════════════════

/** 图像处理 */
export const image: {
	/**
	 * 读取数据目录内图片并校验，返回 base64
	 *
	 * @param path 相对数据目录的路径
	 * @returns 成功返回 { success:true, text: base64 }；非图片返回 failure
	 */
	loadValid(path: string): { success: boolean; text?: string; error?: string }
	/**
	 * 下载图片到数据目录并校验，返回 base64
	 *
	 * @param url 图片直链
	 * @param fileName 相对数据目录的保存文件名
	 * @returns 成功返回 { success:true, text: base64 }；失败返回 failure
	 */
	download(url: string, fileName: string): { success: boolean; text?: string; error?: string }
	/**
	 * 缩放图片并按需重编码（等比缩放或指定精确宽高）
	 *
	 * @param path 相对数据目录的路径，或 data:image/...;base64 内联数据
	 * @param opts 可选参数：
	 *             max_dim 等比缩放到不超过该边长（默认 0=不缩放）
	 *             width|height 精确目标宽高（两者需同时给出，与 max_dim 互斥）
	 *             format 输出编码格式（jpeg/png/webp，默认保留源格式；PNG 保透明）
	 *             quality jpeg/webp 质量（1-100，默认 90）
	 * @returns 成功返回 { success:true, text: data URI, width, height, format }；失败返回 failure
	 */
	resize(path: string, opts?: { max_dim?: number; width?: number; height?: number; format?: 'jpeg' | 'png' | 'webp'; quality?: number }): { success: boolean; text?: string; width?: number; height?: number; format?: string; error?: string }
	/**
	 * 仅重编码图片（编码格式化，不改尺寸）
	 *
	 * @param path 相对数据目录的路径，或 data:image/...;base64 内联数据
	 * @param format 目标编码格式（jpeg/png/webp）
	 * @param opts 可选参数 { quality? }；jpeg/webp 质量（1-100，默认 90）
	 * @returns 成功返回 { success:true, text: data URI, width, height, format }；失败返回 failure
	 */
	convert(path: string, format: 'jpeg' | 'png' | 'webp', opts?: { quality?: number }): { success: boolean; text?: string; width?: number; height?: number; format?: string; error?: string }
}
