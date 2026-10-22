/** 知识逻辑表名：宿主按键路由到对应领域库（video_summary → media.db，file_mapping → text.db），其余落 knowledge.db */
type KnowledgeTable = 'video_summary' | 'file_mapping';

/**
 * 读取整表 [key,text][] 条目并构建 Map（Map 天然按 key 去重）
 *
 * @param table 逻辑表名（由宿主按领域路由到实际 SQLite 库）
 * @returns 键值 Map，读取失败或表不存在时返回空 Map
 */
function loadKnowledge(table: KnowledgeTable): Map<string, string> {
	const [entries, error] = knowledgeLoad(table);
	const map = new Map<string, string>();
	if (error || !Array.isArray(entries)) {
		if (error) console.error(`[知识库] 读取表 ${table} 失败:`, error);
		return map;
	}
	for (const pair of entries) {
		if (Array.isArray(pair) && pair.length >= 1 && typeof pair[0] === 'string') {
			map.set(pair[0], typeof pair[1] === 'string' ? pair[1] : String(pair[1] ?? ''));
		}
	}
	return map;
}

/**
 * 将 Map 写回对应逻辑表（[key,text][] 落盘，按键去重）
 *
 * @param table 逻辑表名（由宿主按领域路由到实际 SQLite 库）
 * @param map 待持久化的键值 Map
 * @returns 是否成功
 */
function saveKnowledge(table: KnowledgeTable, map: Map<string, string>): boolean {
	const [ok, error] = knowledgeSave(table, [...map.entries()]);
	if (error) console.error(`[知识库] 写入表 ${table} 失败:`, error);
	return !error && !!ok;
}

// =============================================================================
// 媒体理解缓存 — 存语音转写文本与旧版视频/动态图理解文本，按键为媒体来源地址
// =============================================================================

/**
 * 从知识库中获取媒体理解缓存
 *
 * @param {string} key 索引键（媒体来源地址）
 * @returns {string | null} 缓存文本或 null
 */
export function getPromptFromKnowledge(key: string): string | null {
	try {
		return loadKnowledge('video_summary').get(key) ?? null;
	}
	catch (error) {
		console.error('[知识库] 读取视频摘要失败:', error);
		return null;
	}
}

/**
 * 从媒体理解缓存中按多个候选键查找
 *
 * 同一张动图在不同链路里可能以 /file/read/、/file/miniature/、带 ?tag= 的
 * 视觉地址或 multimedia/ 相对路径出现；调用方给出候选键，这里只加载一次表。
 *
 * @param keys 候选索引键
 * @returns 第一条非空缓存文本或 null
 */
export function getPromptFromKnowledgeAny(keys: string[]): string | null {
	if (!keys.length) return null;
	try {
		const map = loadKnowledge('video_summary');
		for (const key of keys) {
			const value = map.get(key);
			if (typeof value === 'string' && value.trim()) return value;
		}
		return null;
	}
	catch (error) {
		console.error('[知识库] 读取媒体理解缓存失败:', error);
		return null;
	}
}

/**
 * 向知识库中存储视频摘要（存在则更新，按键去重）
 *
 * @param {string} key 索引键（视频 URL）
 * @param {string} prompt 视频摘要
 * @returns {boolean} 是否成功
 */
export function savePromptToKnowledge(key: string, prompt: string): boolean {
	try {
		return saveKnowledge('video_summary', loadKnowledge('video_summary').set(key, prompt));
	}
	catch (error) {
		console.error('[知识库] 存储视频摘要失败:', error);
		return false;
	}
}

// =============================================================================
// 文件映射表（原 FileMapping 表）— 存放“识别ID #fileName.ext → 文件索引”的映射
// =============================================================================

/**
 * 从文件映射表获取索引值
 *
 * @param {string} key 识别ID（#fileName.ext）
 * @returns {string | null} 索引 JSON 或 null
 */
export function getFileIndexFromKnowledge(key: string): string | null {
	try {
		return loadKnowledge('file_mapping').get(key) ?? null;
	}
	catch (error) {
		console.error('[知识库] 读取文件映射失败:', error);
		return null;
	}
}

/**
 * 向文件映射表写入索引值（存在则更新，按键去重）
 *
 * @param {string} key 识别ID（#fileName.ext）
 * @param {string} value 索引 JSON（传入空串可清除残留索引）
 * @returns {boolean} 是否成功
 */
export function saveFileIndexToKnowledge(key: string, value: string): boolean {
	try {
		return saveKnowledge('file_mapping', loadKnowledge('file_mapping').set(key, value));
	}
	catch (error) {
		console.error('[知识库] 写入文件映射失败:', error);
		return false;
	}
}