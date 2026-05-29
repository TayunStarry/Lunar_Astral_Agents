import { OnlyData, PostMessage, InferencePayload, ModelProtocol, AuthHeaders, modelResponse, PostMessageRole } from '../index';

/** 当前的真实地址位置 */
let currentAddress: string[] = [];

/** 基础配置 */
class BaseConfig {
    /** 是否启用流式响应 */
    protected stream: boolean = false;
    /** 是否启用工具调用 */
    protected enableTools: boolean = true;
    /** 消息列表 */
    protected messages: PostMessage[] = [];
    /** RAG消息列表 */
    protected ragMessages: PostMessage[] = [];
    /** 系统提示 */
    protected systemPrompt: string = "你的名字叫做月华, 是一个女孩子";
    /** chromem是否已初始化 */
    protected static chromemReady: boolean = false;
    /** 私有化构造函数，防止外部实例化 */
    protected constructor() { }
    /** 初始化chromem-go向量数据库 */
    protected static initChromem(): void {
        if (BaseConfig.chromemReady) return;
        const [_, err] = chromemInit(OnlyData.systemUrl, OnlyData.SystemKey, OnlyData.EmbeddingName);
        if (err) console.error('chromem 初始化失败:', err);
        else BaseConfig.chromemReady = true;
    }
}

/** 提示词处理器 */
class PromptProcessor extends BaseConfig {
    /** 生成提示词 */
    protected promptCompletion(prompt: string): string {
        /** 当前地址文本 */
        let addressText = "";
        // 若当前地址为空，查询真实地址
        if (currentAddress.length === 0) {
            /** 查询真实地址 */
            const addressResult = address();
            // 设置当前地址
            currentAddress = addressResult[0];
            addressText = currentAddress.join(' ');
        }
        // 否则使用缓存地址
        else addressText = currentAddress.join(' ');
        // 返回替换后的系统提示词
        return prompt
            // 转换用户名称
            .replace(/{name}/g, OnlyData.userName)
            // 转换当前地址
            .replace(/{current-address}/g, addressText);
    }
    /** 从消息中提取文本 */
    protected extractTextFromMessages(messages: PostMessage[]): string[] {
        return messages.map(message => {
            // 处理纯文本消息和工具响应消息
            if (typeof message.content === 'string') {
                return message.content;
            }
            // 处理多模态消息和连续多模态消息
            else if (Array.isArray(message.content)) {
                // 提取所有文本内容并拼接
                const textContents = message.content
                    .filter(item => item.type === 'text')
                    .map(item => item.text);
                return textContents.join(' ');
            }
            // 默认为空字符串
            return '';
        }).filter(text => text.trim() !== ''); // 过滤空字符串
    }
}

/** 配置修改器 */
class ConfigModifier extends PromptProcessor {
    /** 设置流式响应 */
    public setStream(stream: boolean = false): this {
        this.stream = stream;
        return this;
    }
    /** 设置工具调用 */
    public setEnableTools(enable: boolean = true): this {
        this.enableTools = enable;
        return this;
    }
    /** 写入上下文 */
    public writeContext(context: PostMessage): this {
        if (this.messages.length > 20) {
            const discarded = this.messages.slice(0, this.messages.length - 20);
            this.messages = this.messages.slice(-20);
            this.messages.push(context);
            console.log(JSON.stringify(discarded));
            this.persistDiscardedMessages(discarded);
        }
        else this.messages.push(context);
        console.log(this.messages.length);
        return this;
    }
    /** 将被抛弃的消息持久化到 chromem-go */
    private persistDiscardedMessages(discarded: PostMessage[]): void {
        if (!BaseConfig.chromemReady) BaseConfig.initChromem();
        if (!BaseConfig.chromemReady) return;
        for (const message of discarded) {
            const content = typeof message.content === 'string'
                ? message.content
                : JSON.stringify(message.content);
            chromemAdd(message.role, content);
        }
    }
    /** 覆写上下文 */
    public coverContext(context: PostMessage[] | PostMessage): this {
        this.messages = Array.isArray(context) ? context : [context];
        return this;
    }
}

/** 模型构建器 */
export class ModelBuilder extends ConfigModifier {
    /** 运行模型，可输入额外的上下文补充 */
    public run(appendContext: PostMessage[]): modelResponse {
        /** 检查消息列表中是否包含工具调用消息 */
        const isIncludesTools = this.messages.some((message) => message.role === 'tool');
        /** 构建发给推理模型的请求体 */
        const requestBody: InferencePayload = {
            model: OnlyData.MultimodalName,
            messages: [{ role: 'system', content: this.systemPrompt }, ...appendContext, ...this.messages],
            stream: this.stream,
            tools: isIncludesTools ? [] : OnlyData.toolCall,
            tool_choice: isIncludesTools ? 'none' : 'auto',
        };
        // 如果禁用工具调用，则删除 tool_choice 和 tools 字段
        if (!this.enableTools || !isIncludesTools) {
            delete requestBody.tool_choice;
            delete requestBody.tools;
        };
        /** 构建请求头 */
        const headers: AuthHeaders = {
            Authorization: `Bearer ${encodeURIComponent(OnlyData.SystemKey)}`,
            "Content-Type": "application/json",
        };
        /** 构建模型请求 */
        const modelRequest: ModelProtocol = {
            method: "POST",
            crossDomain: true,
            headers,
            body: JSON.stringify(requestBody)
        };
        /** 定义API端点 */
        const endpoint = "/chat/completions";
        /** 直接调用Go函数处理请求 */
        const [result, error] = syncFetch({ url: OnlyData.systemUrl + endpoint, execute: modelRequest });
        // 抛出错误
        if (error) throw error;
        // 返回模型响应
        return result;
    }
    /** 从 chromem-go 查询相关消息并填充 ragMessages */
    public queryRagMessages(): this {
        /** 获取最新的用户消息内容作为查询条件 */
        const latestUserMessage = this.getLatestUserMessageContent();
        // 如果没有最新的用户消息，直接返回
        if (!latestUserMessage) return this;
        // 初始化 chromem-go
        if (!BaseConfig.chromemReady) BaseConfig.initChromem();
        // 如果初始化失败，直接返回
        if (!BaseConfig.chromemReady) return this;
        /** 查询 chromem-go 相关消息 */
        const [results, error] = chromemQuery(latestUserMessage, 10);
        // 如果查询失败，直接返回
        if (error) {
            console.error('chromem 查询失败:', error);
            return this;
        }
        // 如果查询结果为空，直接返回
        if (results && results.length > 0) {
            this.ragMessages = results.map((r: { role: string, content: string }) => ({ role: r.role as PostMessageRole, content: r.content, }));
        }
        return this;
    }
    /** 获取最新的用户消息内容作为查询条件 */
    private getLatestUserMessageContent(): string | null {
        // 从消息列表的末尾开始遍历，找到最新的用户消息
        for (let i = this.messages.length - 1; i >= 0; i--) {
            /** 检查当前消息是否为用户消息 */
            const message = this.messages[i];
            // 如果是用户消息，直接返回其内容
            if (message.role === 'user' && typeof message.content === 'string') {
                return message.content;
            }
        }
        return null;
    }
    /** 构建模型响应实例 */
    public constructor(prompt: string) {
        super();
        // 补全系统提示词
        this.systemPrompt = this.promptCompletion(prompt);
    }
}