// ═══════════════════════════════════════════════════════════
// 文件下载（http.*）
//
// 通用网络请求一律使用全局 `fetch`（见 runtimeAPI.d.ts）；
// 此处只保留「把远端文件落盘到脚本数据目录」这一件 fetch 做不了的事。
// ═══════════════════════════════════════════════════════════

/** 下载能力 */
export const http: {
	/**
	 * 同步下载任意文件到数据目录并落盘；阻塞直到完成
	 *
	 * @param url 文件直链
	 * @param savePath 相对数据目录的保存路径；为空时按 URL 末段命名
	 * @returns { success:true, path: 相对路径, size: 字节数 }；失败返回 { success:false, error }
	 */
	download(url: string, savePath?: string): { success: boolean; path?: string; size?: number; error?: string }
}
