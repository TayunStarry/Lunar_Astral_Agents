/** 知识表名：一个表对应 local_data/database/knowledge 下的一个 JSON 文件（格式 [key,text][]） */
type KnowledgeTable = 'video_summary' | 'file_mapping';

/**
 * 读取整表 [key,text][] 条目并构建 Map（Map 天然按 key 去重）
 *
 * @param table 表名（即 JSON 文件名，不含扩展名）
 * @returns 键值 Map，读取失败或文件不存在时返回空 Map
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
 * 将 Map 写回对应表对应的 JSON 文件（[key,text][] 落盘，按键去重）
 *
 * @param table 表名（即 JSON 文件名，不含扩展名）
 * @param map 待持久化的键值 Map
 * @returns 是否成功
 */
function saveKnowledge(table: KnowledgeTable, map: Map<string, string>): boolean {
	const [ok, error] = knowledgeSave(table, [...map.entries()]);
	if (error) console.error(`[知识库] 写入表 ${table} 失败:`, error);
	return !error && !!ok;
}

// =============================================================================
// 视频摘要缓存（原 KeyPrompt 表）— 存观影者生成的视频观后感，按键为视频 URL
// =============================================================================

/**
 * 从知识库中获取视频摘要（视频观后感缓存）
 *
 * @param {string} key 索引键（视频 URL）
 * @returns {string | null} 摘要或 null
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