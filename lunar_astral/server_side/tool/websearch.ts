import { OnlyData, WebSearchConfig, SearchMode } from '../index';

/** 网络检索子系统是否已初始化 */
let webSearchInitialized = false;

/**
 * 初始化网络检索子系统
 * 使用 global.ts 中定义的 LLM API 配置作为后端服务支持
 *
 * @returns {boolean} 是否初始化成功
 */
export function initWebSearch(): boolean {
	if (webSearchInitialized) {
		console.log('[网络检索] 子系统已初始化，跳过重复初始化');
		return true;
	}

	/** 构建配置，使用 OnlyData 中的 LLM API 配置 */
	const config: WebSearchConfig = {
		baseURL: OnlyData.systemUrl,
		apiKey: OnlyData.SystemKey,
		model: OnlyData.MultimodalName,
		maxTokens: 4096,
		temperature: 0.7,
	};

	try {
		const [success, err] = webSearchInit(
			config.baseURL,
			config.apiKey,
			config.model,
			config.maxTokens,
			config.temperature
		);
		if (err) {
			console.error('[网络检索] 初始化失败:', err);
			return false;
		}
		webSearchInitialized = true;
		console.log('[网络检索] 子系统初始化成功');
		return true;
	} catch (e) {
		console.error('[网络检索] 初始化异常:', e);
		return false;
	}
}

/**
 * 检查网络检索子系统是否已初始化
 *
 * @returns {boolean} 是否已初始化
 */
export function isWebSearchReady(): boolean {
	return webSearchInitialized && webSearchIsReady();
}

/**
 * 执行网络搜索
 *
 * @param {string} query - 搜索查询
 * @param {SearchMode} mode - 搜索模式
 *
 * @returns {[string, Error | null]} 搜索结果和错误信息
 */
export function executeWebSearch(query: string, mode: SearchMode = 'webpage'): [string, Error | null] {
	try {
		if (mode === 'depth') {
			const [result, err] = webSearchDepth(query);
			if (err) return ['', err];
			return [result, null];
		}
		if (mode === 'webpage') {
			const [result, err] = webSearchWebpage(query);
			if (err) return ['', err];
			return [result, null];
		}
		const [result, err] = webSearchSimple(query);
		if (err) return ['', err];
		return [result, null];
	} catch (e) {
		return ['', e as Error];
	}
}

// ==== 模块级注册 ====
// web_search 已迁移至学习者角色（LearnerRole）作为私有工具
// 本模块仅保留初始化与类型基础设施
