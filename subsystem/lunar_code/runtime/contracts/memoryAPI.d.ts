// ═══════════════════════════════════════════════════════════
// 记忆库（memory.*）
//
// 记忆按类型全局检索，集合只是内容自动归入的分组，调用方无需感知集合名。
// ═══════════════════════════════════════════════════════════

/** 记忆命中项（文本） */
interface MemoryHit {
	/** 记忆正文 */
	content: string
	/** 相似度 */
	similarity: number
	/** 记忆类型（text / image / audio 等） */
	type: string
	/** 归属集合（自动分组名） */
	collection: string
	/** 媒体地址（媒体类记忆且已落盘时） */
	url?: string
}

/** 图片记忆命中项 */
interface ImageMemoryHit {
	/** 相似度 */
	similarity: number
	/** 图片地址（/file/read/...）；历史记忆可能只有 base64 */
	url?: string
	/** 内联图片数据（仅当没有 url 时出现） */
	base64?: string
}

/** 向量记忆库 */
export const memory: {
	/**
	 * 写入一条记忆
	 *
	 * @param v 记忆内容：content 为正文（文本记忆）；base64 为图片数据（图片记忆，优先于 content）；
	 *          tags 为可检索的标签列表（文本记忆）
	 * @returns 成功返回 { success:true, id }；失败返回 { success:false, error }
	 */
	store(v: { content?: string; base64?: string; tags?: string[] }): { success: boolean; id?: string | number; error?: string }
	/**
	 * 按语义搜索记忆
	 *
	 * @param v 查询参数：query 为待检索文本；limit 最多返回条数（默认 5，上限 50）；
	 *          type 记忆类型（默认 text）；force_global=true 跳过快速路由、全局检索
	 * @returns { success, results: 相似度降序的命中列表 }；失败返回 failure
	 */
	search(v: { query: string; limit?: number; type?: string; force_global?: boolean }): { success: boolean; results?: MemoryHit[]; error?: string }
	/**
	 * 按查询语义检索图片
	 *
	 * @param query 检索文本
	 * @param opts { limit? } 最多返回条数（默认 5）
	 * @returns { success, results: [{ url?/base64?, similarity }] }
	 */
	searchImage(query: string, opts?: { limit?: number }): { success: boolean; results?: ImageMemoryHit[]; error?: string }
	/**
	 * 添加一张图片记忆（base64；标签由记忆库模型自动生成，内容自动归组）
	 *
	 * @param image 待添加的图片 base64 或 data URI
	 */
	storeImage(image: string): { success: boolean; id?: string; error?: string }
	/**
	 * 基于查询语义随机返回一张图片记忆
	 *
	 * @param query 检索文本；为空时按通用语义检索
	 * @returns { success, url?/base64? }
	 */
	randomImage(query?: string): { success: boolean; url?: string; base64?: string; error?: string }
}
