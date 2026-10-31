// ═══════════════════════════════════════════════════════════
// 文件读写（file.*）
// ═══════════════════════════════════════════════════════════

/** 文件能力 */
export const file: {
	/**
	 * 写文件
	 *
	 * @param path 文件相对路径，限定在脚本数据目录内
	 * @param data 要写入的 UTF-8 文本内容
	 * @returns 成功返回 { success:true }；失败（如路径越界）返回 { success:false, error }
	 */
	write(path: string, data: string): { success: boolean; error?: string }
	/**
	 * 读文件
	 *
	 * @param path 文件相对路径，限定在脚本数据目录内
	 * @returns 成功返回 { success:true, text: 文件内容 }；不存在或读取失败返回 { success:false, error }
	 */
	read(path: string): { success: boolean; text?: string; error?: string }
	/**
	 * 删除文件
	 *
	 * @param path 文件相对路径，限定在脚本数据目录内
	 * @returns 成功返回 { success:true }；失败返回 { success:false, error }
	 */
	delete(path: string): { success: boolean; error?: string }
}
