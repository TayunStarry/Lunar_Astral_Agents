import * as EntryAPI from '../EntryAPI/code';
/** 嵌入向量响应结构 */
interface EmbeddingResponse {
    /** 响应数据数组 */
    data: Array<{
        /** 文本的嵌入向量 */
        embedding: number[];
    }>;
}
/**
 * 多模态推理请求类
 * 统一处理文本和图像的AI请求
 */
export class MultimodalRequest {
    /** 推理模型响应 */
    public response: Promise<Response>;
    /** 模型路由端口 */
    protected port = "/chat/completions";
    /**
     * 多模态推理请求体
     *
     * @param {PostMessage[]} messages 对话消息列表（支持文本和图像内容）
     *
     * @param {boolean} enableStopSignal 是否启用中止信号
     *
     * @param {boolean} stream 是否启用流式响应
     *
     * @param {boolean} enableTools 是否启用工具调用
     */
    constructor(public messages: EntryAPI.PostMessage[], public enableStopSignal: boolean, public stream: boolean, public enableTools: boolean = true) {
        /** 检查消息列表中是否包含工具调用消息 */
        const isIncludesTools = messages.some((message) => message.role === 'tool');
        /** 构建发给推理模型的请求体 */
        const requestBody: EntryAPI.InferencePayload = {
            model: EntryAPI.OnlyData.MultimodalName,
            messages,
            stream,
            tools: EntryAPI.OnlyData.toolCall,
            tool_choice: isIncludesTools ? 'none' : 'auto',
        };
        // 如果禁用工具调用，则删除 tool_choice 和 tools 字段
        if (!enableTools) {
            delete requestBody.tool_choice;
            delete requestBody.tools;
        };
        /**
         * 配置请求选项
         */
        const requestOption: EntryAPI.MultimodalProtocol = {
            method: "POST",
            crossDomain: true,
            headers: {
                Authorization: `Bearer ${encodeURIComponent(EntryAPI.OnlyData.MultimodalKey)}`,
                "Content-Type": "application/json",
            },
            body: JSON.stringify(requestBody)
        };
        // 设置中止信号，用于后续可能的请求中止操作
        if (enableStopSignal) requestOption.signal = EntryAPI.OnlyData.abortController?.signal;
        // 发送请求并返回响应
        this.response = fetch(EntryAPI.OnlyData.MultimodalUrl + this.port, requestOption as any);
    }
};

/**
 * 嵌入请求类，用于获取文本的向量表示。
 */
export class EmbeddingRequest {
    /** 推理模型响应 */
    public response: Promise<Response>;
    /** 模型路由端口 */
    protected port = "/embeddings";
    /**
     * 嵌入模型请求体
     *
     * @param {string[] | string} messages 对话消息列表（支持文本和图像内容）
     *
     * @param {boolean} enableStopSignal 是否启用中止信号
     *
     * @param {boolean} stream 是否启用流式响应
     */
    constructor(public messages: string[] | string, public enableStopSignal: boolean, public stream: boolean) {
        /** 限制消息长度，防止超出模型最大输入长度 */
        const validMessages = (Array.isArray(messages) ? messages : [messages]).map(message => message.slice(0, 4096))
        /** 构建发给推理模型的请求体 */
        const requestBody: EntryAPI.InferencePayload = {
            model: EntryAPI.OnlyData.EmbeddingName,
            input: validMessages,
            stream
        };
        /**
         * 配置请求选项
         */
        const requestOption: EntryAPI.MultimodalProtocol = {
            method: "POST",
            crossDomain: true,
            headers: {
                Authorization: `Bearer ${encodeURIComponent(EntryAPI.OnlyData.EmbeddingKey)}`,
                "Content-Type": "application/json",
            },
            body: JSON.stringify(requestBody)
        };
        // 设置中止信号，用于后续可能的请求中止操作
        if (enableStopSignal) requestOption.signal = EntryAPI.OnlyData.abortController?.signal;
        // 发送请求并返回响应
        this.response = fetch(EntryAPI.OnlyData.EmbeddingUrl + this.port, requestOption as any);
    }
    /**
     * 解析嵌入模型响应，返回嵌入向量
     *
     * @returns {Promise<number[]>} 嵌入向量数组
     */
    public async output(): Promise<number[]> {
        /** 解析响应体为 JSON 格式 */
        const response = await this.response.then((response) => response.json() as Promise<EmbeddingResponse>);
        // 截取嵌入向量的前 256 个元素，作为模型输入
        return response.data[0].embedding.slice(0, 256);
    }
};