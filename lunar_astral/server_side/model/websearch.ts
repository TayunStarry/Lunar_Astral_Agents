import { ToolCall, OnlyData, WebSearchConfig, SearchMode } from '../index';

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
export function executeWebSearch(query: string, mode: SearchMode = 'deep'): [string, Error | null] {
    try {
        if (mode === 'research') {
            const [result, err] = webSearchResearch(query);
            if (err) return ['', err];
            return [result, null];
        }
        if (mode === 'deep') {
            const [result, err] = webSearchDeep(query);
            if (err) return ['', err];
            return [result, null];
        }
        const [result, err] = webSearchShallow(query);
        if (err) return ['', err];
        return [result, null];
    } catch (e) {
        return ['', e as Error];
    }
}

// ==== 工具定义 ====

/** 网络检索工具定义 */
export const webSearchTools: ToolCall[] = [
    {
        type: "function",
        function: {
            name: "web_search",
            description: "执行网络搜索，获取实时信息。当用户的问题涉及实时数据、最新资讯、事实查询等需要联网获取的信息时，应使用此工具。支持三种模式：shallow（普通搜索，仅返回搜索结果摘要）、deep（深度搜索，抓取网页内容并用AI总结，适合需要详细信息的场景）、research（研究搜索，将问题拆解为多个子问题并行搜索，去重后生成综合研究报告，适合需要全面深入分析的场景）。默认使用 deep 模式。",
            parameters: {
                type: "object",
                properties: {
                    query: {
                        type: "string",
                        description: "搜索查询关键词或问题"
                    },
                    mode: {
                        type: "string",
                        description: "搜索模式：shallow（普通搜索）、deep（深度搜索，默认）或 research（研究搜索，全面深入分析）",
                        enum: ["shallow", "deep", "research"]
                    }
                },
                required: ["query"]
            }
        }
    }
];

/** 处理网络搜索工具调用 */
async function handleWebSearch(args?: Record<string, any> | string): Promise<string> {
    const parsed = typeof args === 'string' ? JSON.parse(args) : (args || {});
    const { query, mode } = parsed;

    if (!query || query.trim().length === 0) {
        return '搜索失败：查询关键词不能为空';
    }

    const searchMode: SearchMode = mode || 'deep';

    // 确保子系统已初始化
    if (!isWebSearchReady()) {
        const initResult = initWebSearch();
        if (!initResult) {
            return '搜索失败：网络检索子系统初始化失败';
        }
    }

    console.log(`[网络检索] 工具调用: query="${query}", mode="${searchMode}"`);

    const [result, err] = executeWebSearch(query.trim(), searchMode);
    if (err) {
        console.error(`[网络检索] 搜索失败: ${err.message || String(err)}`);
        return `搜索失败：${err.message || String(err)}`;
    }

    console.log(`[网络检索] 查询结果:\n${result || '未找到相关搜索结果'}`);
    return result || '未找到相关搜索结果';
}

// ==== 模块级注册 ====

// 将网络搜索工具注册到月华工具协议映射表
OnlyData.lunarToolPackageMap.set('web_search', handleWebSearch);
