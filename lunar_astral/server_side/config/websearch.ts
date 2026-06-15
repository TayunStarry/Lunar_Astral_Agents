/** 网络检索子系统配置接口 */
export interface WebSearchConfig {
    /** LLM 服务基础 URL */
    baseURL: string;
    /** LLM API 密钥 */
    apiKey: string;
    /** LLM 模型名称 */
    model: string;
    /** 最大生成 token 数 */
    maxTokens: number;
    /** 生成温度 */
    temperature: number;
}

/** 搜索模式类型 */
export type SearchMode = 'shallow' | 'deep' | 'research';
