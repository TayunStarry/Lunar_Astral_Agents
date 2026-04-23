import { OnlyData, PostMessage, InferencePayload, ModelProtocol, AuthHeaders, GOaddress, GOfetch } from '../../config/index';

/** 当前的真实地址位置 */
let currentAddress: string[] = [];

/** 基础配置 */
class BaseConfig {
    /** 是否启用多模态 */
    protected isMultimodal: boolean = true;
    /** 是否启用流式响应 */
    protected stream: boolean = false;
    /** 是否启用工具调用 */
    protected enableTools: boolean = true;
    /** 消息列表 */
    protected messages: PostMessage[] = [];
    /** 系统提示 */
    protected systemPrompt: string = "你的名字叫做月华, 是一个女孩子";
    /** 私有化构造函数，防止外部实例化 */
    protected constructor() { }
}

/** 提示词处理器 */
class PromptProcessor extends BaseConfig {
    /** 生成提示词 */
    protected promptCompletion(prompt: string): string {
        /** 当前地址 */
        let address = "";
        // 若当前地址为空，查询真实地址
        if (currentAddress.length === 0) address = (GOaddress()[0]).join(' ');
        // 否则使用缓存地址
        else address = currentAddress.join(' ');
        // 返回替换后的系统提示词
        return prompt
            // 转换用户名称
            .replace(/{name}/g, OnlyData.userName)
            // 转换当前时间
            .replace(/{current-time}/g, new Date().toLocaleString())
            // 转换当前地址
            .replace(/{current-address}/g, address);
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

/** 模式配置 */
class ModeConfig extends PromptProcessor {
    /** 启用多模态 */
    public useMultimodal(prompt: string): this {
        // 补全系统提示词
        this.systemPrompt = this.promptCompletion(prompt);
        // 设置为多模态模式
        this.isMultimodal = true;
        // 返回当前实例
        return this;
    }
    /** 启用嵌入模式 */
    public useEmbedding(): this {
        // 设置为嵌入模式   
        this.isMultimodal = false;
        return this;
    }
}

/** 配置修改器 */
class ConfigModifier extends ModeConfig {
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
        if (this.messages.length > 30) this.messages.slice(-30).push(context);
        else this.messages.push(context);
        return this;
    }
    /** 覆写上下文 */
    public coverContext(context: PostMessage[] | PostMessage): this {
        this.messages = Array.isArray(context) ? context : [context];
        return this;
    }
}

/** 模型构建器 */
export class ModelBuilder extends ConfigModifier {
    /** 运行模型 */
    public get run(): Promise<any> | Promise<number[]> {
        if (this.isMultimodal) return this.runMultimodal();
        else return this.runEmbedding();
    }
    /** 运行多模态模型 */
    protected async runMultimodal(): Promise<any> {
        /** 检查消息列表中是否包含工具调用消息 */
        const isIncludesTools = this.messages.some((message) => message.role === 'tool');
        /** 构建发给推理模型的请求体 */
        const requestBody: InferencePayload = {
            model: OnlyData.MultimodalName,
            messages: [{ role: 'system', content: this.systemPrompt }, ...this.messages],
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
            Authorization: `Bearer ${encodeURIComponent(OnlyData.MultimodalKey)}`,
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
        const [result, error] = GOfetch({ url: OnlyData.MultimodalUrl + endpoint, execute: modelRequest });
        // 抛出错误
        if (error) throw error;
        // 返回模型响应
        return result;
    }
    /** 运行嵌入模型 */
    protected async runEmbedding(): Promise<number[]> {
        /** 剔除其他内容, 仅保留文本内容 */
        const validMessages = this.extractTextFromMessages(this.messages);
        /** 构建发给推理模型的请求体 */
        const requestBody: InferencePayload = {
            model: OnlyData.EmbeddingName,
            input: validMessages,
            stream: this.stream,
        };
        /** 构建请求头 */
        const headers: AuthHeaders = {
            Authorization: `Bearer ${encodeURIComponent(OnlyData.EmbeddingKey)}`,
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
        const endpoint = "/embeddings";
        /** 直接调用Go函数处理请求 */
        const [result, error] = GOfetch({ url: OnlyData.EmbeddingUrl + endpoint, execute: modelRequest });
        // 抛出错误
        if (error) throw error;
        // 截取嵌入向量的前 256 个元素，作为模型输入
        return result.data[0].embedding.slice(0, 256);
    }
    /** 构建模型响应实例 */
    public constructor() { super(); }
}