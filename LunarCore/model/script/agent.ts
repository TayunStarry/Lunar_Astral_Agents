import {
    OnlyData,
    PostMessage,
    InferencePayload,
    MultimodalProtocol,
    EmbeddingResponse,
    PostMessageRole,
    ExtractKeyframesResponse,
    ExtractKeyframesData,
    TextMessage,
    ImageContent,
    TextContent,
    ChatCache,
    ToolCall
} from '../../config/index';

import {
    fetchMarkdown,
    fetchDocumentCallback,
    getPromptFromDatabase,
    savePromptToDatabase
} from '../../FileSystem/index';

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
    /** 中止信号 */
    public signal: AbortSignal | undefined = undefined;
    /** 系统提示 */
    protected systemPrompt: string = "你的名字叫做月华, 是一个女孩子";
    /** 私有化构造函数，防止外部实例化 */
    protected constructor() { }
}

/** 提示词处理器 */
class PromptProcessor extends BaseConfig {
    /** 生成提示词 */
    protected async promptCompletion(): Promise<string> {
        /** 原始系统提示词 */
        const protoPrompt = await fetchMarkdown('/read/resources/prompts/systemPrompt.md');
        /** 当前地址 */
        let address = "";
        // 若当前地址为空，查询真实地址
        if (currentAddress.length === 0) address = (await this.realAddress()).join(' ');
        // 否则使用缓存地址
        else address = currentAddress.join(' ');
        // 返回替换后的系统提示词
        return protoPrompt
            // 转换用户名称
            .replace(/{name}/g, OnlyData.customConfig.userName || "你")
            // 转换当前时间
            .replace(/{current-time}/g, new Date().toLocaleString())
            // 转换当前地址
            .replace(/{current-address}/g, address);
    }
    /** 查询真实地址 */
    protected async realAddress(): Promise<string[]> {
        /** 从IP地址查询位置信息 */
        const addressRegion = await fetch('https://ipapi.co/json/')
        // 检查响应状态
        if (!addressRegion.ok) return ['江苏省', '南京市'];
        /** 解析JSON响应 */
        const data = await addressRegion.json();
        /** 提取省份信息 */
        const province = data.region;
        /** 提取城市信息 */
        const city = data.city;
        // 确保省份和城市信息存在
        if (!province || !city) return ['江苏省', '南京市'];
        // 缓存当前地址
        currentAddress = [province, city];
        // 返回省份和城市
        return [province, city];
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
    public useMultimodal(prompt?: string): this {
        // 若提示词长度超过100，直接使用提示词
        if (prompt && prompt.length > 100) this.systemPrompt = prompt;
        // 否则使用文件接口获取系统提示词
        else this.promptCompletion().then(content => this.systemPrompt = content);
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
    public coverContext(contexts: PostMessage[]): this {
        this.messages = contexts;
        return this;
    }
    /** 设置中止信号 */
    public setSignal(signal: AbortSignal): this {
        this.signal = signal;
        return this;
    }
}

/** 模型构建器 */
class ModelBuilder extends ConfigModifier {
    /** 运行模型 */
    public get run(): Promise<Response> | Promise<number[]> {
        if (this.isMultimodal) return this.runMultimodal();
        else return this.runEmbedding();
    }
    /** 运行多模态模型 */
    protected async runMultimodal(): Promise<Response> {
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
        /**
         * 配置请求选项
         */
        const requestOption: MultimodalProtocol = {
            method: "POST",
            crossDomain: true,
            headers: {
                Authorization: `Bearer ${encodeURIComponent(OnlyData.MultimodalKey)}`,
                "Content-Type": "application/json",
            },
            body: JSON.stringify(requestBody)
        };
        // 设置中止信号，用于后续可能的请求中止操作
        if (this.signal) requestOption.signal = this.signal;
        // 发送请求并返回响应
        return fetch(OnlyData.MultimodalUrl + "/chat/completions", requestOption as any);
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
        /**
         * 配置请求选项
         */
        const requestOption: MultimodalProtocol = {
            method: "POST",
            crossDomain: true,
            headers: {
                Authorization: `Bearer ${encodeURIComponent(OnlyData.EmbeddingKey)}`,
                "Content-Type": "application/json",
            },
            body: JSON.stringify(requestBody)
        };
        // 设置中止信号，用于后续可能的请求中止操作
        if (this.signal) requestOption.signal = this.signal;
        // 发送请求并返回响应
        const response = await fetch(OnlyData.EmbeddingUrl + "/embeddings", requestOption as any);
        /** 解析响应体为 JSON 格式 */
        const jsonResponse = await response.json() as EmbeddingResponse;
        // 截取嵌入向量的前 256 个元素，作为模型输入
        return jsonResponse.data[0].embedding.slice(0, 256);
    }
    /** 构建模型响应实例 */
    public constructor() { super(); }
}

class ProtoAgent {
    /** 构建计划 */
    protected compilePlan: ModelBuilder = new ModelBuilder();
    /** 推理关键词 */
    protected queryKeywords: ModelBuilder = new ModelBuilder();
    /** 情感管理器 */
    protected emotionManager: ModelBuilder = new ModelBuilder();
    /** 视频摘要 */
    protected videoSummary: ModelBuilder = new ModelBuilder();
    /** 视频描述 */
    protected videoDescription: ModelBuilder = new ModelBuilder();
    /** 聊天回复 */
    protected chatReply: ModelBuilder = new ModelBuilder().useMultimodal();
    /** 嵌入向量 */
    public embedding: ModelBuilder = new ModelBuilder().useEmbedding();
    /** 未读上下文 */
    protected unreadContext: PostMessage[] = [];
    /** 未读视频URL */
    protected unreadVideoUrl: string[] = [];
    /** 最终应答 */
    public finalResponse: string = "";
    /** 响应速度 */
    public responseSpeed: number = 0;
    /** 默认应答 */
    public defaultAnswer: string = "月华不知道哦";
    /** 构建智能体 并 初始化各个子模型的系统提示词 */
    constructor() {
        // 初始化 全部模型 的 系统提示词
        fetchMarkdown('/read/resources/prompts/compilePlan.md').then(content => this.compilePlan.useMultimodal(content));
        fetchMarkdown('/read/resources/prompts/queryKeywords.md').then(content => this.queryKeywords.useMultimodal(content));
        fetchMarkdown('/read/resources/prompts/emotionManager.md').then(content => this.emotionManager.useMultimodal(content));
        fetchMarkdown('/read/resources/prompts/videoSummary.md').then(content => this.videoSummary.useMultimodal(content));
        fetchMarkdown('/read/resources/prompts/videoDescription.md').then(content => this.videoDescription.useMultimodal(content));
        // 初始化 自定义配置 信息
        fetchDocumentCallback('resources/custom_config.json').then(content => OnlyData.customConfig = JSON.parse(content));
        // TODO 初始化 工具调用配置
        // fetchDocumentCallback('resources/toolCall.json').then(content => OnlyData.toolCall = JSON.parse(content));
        // TODO 初始化 聊天记录
        // fetchDocumentCallback('resources/chatRecord.json')
    }
}

class VideoAnalysis extends ProtoAgent {
    /**
     * 处理视频文件
     *
     * @param {string} videoUrl - 视频文件URL
     * 
     * @param {string} userNeeds - 用户需求
     * 
     * @returns {Promise<void>} - 处理完成后的 Promise
     */
    protected async analysisVideoFile(videoUrl: string, userNeeds: string): Promise<void> {
        /** 检查是否已处理过该视频 */
        const cachedPrompt = await getPromptFromDatabase(videoUrl);
        // 如果视频已处理过，直接添加到未读上下文
        if (cachedPrompt) {
            this.unreadContext.push({ role: 'user', content: cachedPrompt });
            return;
        }
        /** 获取视频文件 */
        const response = await fetch(videoUrl);
        /** 视频文件 Blob 对象 */
        const videoBlob = await response.blob();
        /** FormData 对象，用于上传视频文件 */
        const formData = new FormData();
        // 添加视频文件到 FormData
        formData.append('video', videoBlob, videoUrl.replace(/\\/g, '/').split('/').pop()?.trim() || 'video.mp4');
        /** 关键帧提取API响应 */
        const extractResponse = await fetch('/extract/keyframes', { method: 'POST', body: formData });
        // 检查响应状态
        if (!extractResponse.ok) throw new Error('提取关键帧失败');
        /** 关键帧提取API响应数据 */
        const result = await extractResponse.json() as ExtractKeyframesResponse;
        /** 提取到的关键帧数组 */
        const keyFrames = result.keyFrames || [];
        /** 沙箱消息数组 */
        const sandboxMessages: Array<TextContent> = [];
        /** 模型对视频总结结果 */
        let videoSummary = '';
        /** 关键帧消息数组 */
        const frameMessages: Array<ImageContent> = keyFrames.map(frame => ({ type: "image_url", image_url: { url: `data:image/jpeg;base64,${frame.data}` } }));
        // 处理关键帧，每20张调用一次模型进行画面总结
        for (let i = 0; i < frameMessages.length; i += 20) {
            /** 当前批次20张关键帧消息*/
            const batchFrames = frameMessages.slice(i, i + 20);
            // 写入视频描述上下文
            this.videoDescription.writeContext({ role: 'user', content: batchFrames });
            /** 调用模型进行画面总结 */
            const summaryRequest = await (await this.videoDescription.run as Response).json();
            /** 模型总结结果 */
            const summary = summaryRequest?.choices?.[0]?.message?.content;
            // 过滤空字符串和仅包含空格的字符串
            if (summary && summary.trim().length > 0) sandboxMessages.push(summary);
        }
        // 判断是否包含多个批处理片段
        if (sandboxMessages.length > 1) {
            // 写入视频摘要上下文
            this.videoSummary.writeContext({ role: 'user', content: sandboxMessages });
            /** 调用模型进行视频总结 */
            const summaryRequest = await (await this.videoSummary.run as Response).json();
            /** 模型视频总结结果 */
            videoSummary = summaryRequest?.choices?.[0]?.message?.content;
        }
        // 如果仅包含一个批处理片段，使用该片段作为总结
        else if (sandboxMessages.length === 1) videoSummary = sandboxMessages[0].text;
        // 否则使用默认应答
        else videoSummary = this.defaultAnswer;
        // 将视频总结结果添加到消息数组
        if (videoSummary) this.unreadContext.push({ role: 'user', content: videoSummary });
        // 如果用户需求非空，添加到消息数组
        if (userNeeds.trim().length > 0) this.unreadContext.push({ role: 'user', content: userNeeds });
        // 缓存处理结果到数据库
        if (videoSummary) await savePromptToDatabase(videoUrl, videoSummary);
    }
}

class ChatMessage extends VideoAnalysis {
    /** 更新消息内容 */
    protected updateMessageContent(state: ChatCache): string {
        // 检查推理内容是否为空
        if (state.thinkingContent.trim() !== "") {
            /** 新的思考标签内容 */
            const newThinkTag = '<think>\n' + state.thinkingContent + '\n</think>';
            // 修正复合描述内容
            this.finalResponse = newThinkTag + state.descriptionContent;
        }
        // 修正简单描述内容
        else this.finalResponse = state.descriptionContent;
        // 检查消息内容是否为空
        if (this.finalResponse.trim() === "") return this.defaultAnswer;
        return this.finalResponse;
    }
    /** 处理聊天消息响应 */
    protected async analyzeMessageResponse(message: string, cache: ChatCache): Promise<void> {
        try {
            /** 解析响应为JSON */
            const jsonData = JSON.parse(message);
            // 处理推理内容数据
            if (jsonData.choices?.[0]?.message?.reasoning_content) {
                cache.thinkingContent = jsonData.choices[0].message.reasoning_content;
            }
            // 检查是否有预测令牌数
            if (jsonData.timings?.predicted_per_second) {
                this.responseSpeed = jsonData.timings.predicted_per_second;
            }
            // 处理工具调用
            if (jsonData.choices?.[0]?.message?.tool_calls) {
                // 遍历所有工具调用
                for (const toolCall of jsonData.choices[0].message.tool_calls) {
                    try {
                        // 解析arguments字段
                        toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
                        // 记录工具调用
                        cache.toolCalls.push(toolCall);
                    }
                    catch (parseError) {
                        console.error('工具调用参数解析错误:', parseError);
                    }
                }
            }
            // 处理内容数据
            if (jsonData.choices?.[0]?.message?.content) {
                cache.descriptionContent = jsonData.choices[0].message.content;
            }
        }
        catch (error) {
            console.error('聊天消息响应处理错误:', error);
        }
    }
    /** 批量执行工具调用 */
    protected async batchExecutionToolCall(state: ChatCache): Promise<boolean> {
        /** 工具调用标志 */
        let hasToolCalls = false;
        // 遍历所有工具调用
        for (const toolCall of state.toolCalls) {
            // 仅处理函数类型的工具调用
            if (toolCall.type !== "function") continue;
            /** 工具函数名称 */
            const functionName = toolCall.function.name;
            /** 工具函数参数 */
            const functionArgs = toolCall.function.arguments;
            /** 查询对应的月华工具包 */
            const lunarToolPackage = OnlyData.lunarToolPackageMap.get(functionName);
            // 检查是否有对应的工具包
            if (!lunarToolPackage) {
                this.unreadContext.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
                continue;
            }
            try {
                /** 工具函数执行结果 */
                const toolResult = await lunarToolPackage(functionArgs);
                // 将工具响应添加到消息历史中
                this.unreadContext.push({ role: "tool", content: toolResult, tool_call_id: toolCall.id });
                // 标记有工具调用
                hasToolCalls = true;
            }
            catch (error) {
                // 将工具调用失败信息添加到消息历史中
                this.unreadContext.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
            }
        }
        // 处理完所有工具调用后，清空状态
        state.currentToolCallIndex = -1;
        state.currentFunctionArgs = "";
        state.currentFunctionName = "";
        state.currentToolCall = null;
        state.toolCalls = [];
        // 标记有工具调用
        return hasToolCalls;
    };
    /** 发送请求并获取响应 */
    protected async callMultimediaAndToolParsing(cache: ChatCache): Promise<void> {
        try {
            // 将未读上下文数组中的消息添加到处理器模型的上下文
            this.unreadContext.forEach(context => this.chatReply.writeContext(context));
            // 清空未读上下文数组
            this.unreadContext = [];
            /** 向处理器模型发送请求并等待响应 */
            const response = await this.chatReply.run as Response;
            // 如果未能获得期望中的响应，则抛出错误
            if (!response.ok) {
                this.finalResponse = `月华发现了一个错误: ${response.status} ${response.statusText}`;
                return;
            }
            // 读取响应文本内容
            const responseText = await response.text();
            // 处理响应文本内容
            await this.analyzeMessageResponse(responseText, cache);
            // 如果有工具调用，处理它们并重新发送请求
            if (cache.toolCalls.length > 0) {
                /** 处理工具调用 */
                const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
                // 如果有处理过的工具调用，重新发送请求（包含工具调用结果）
                if (hasProcessedToolCalls) return await this.callMultimediaAndToolParsing(cache);
            }
        }
        catch (error) {
            console.error('请求处理错误:', error);
            this.chatReply.signal = undefined;
        }
        // 更新消息内容
        this.updateMessageContent(cache);
    }
}

/** 月华智能体 */
class LunarAgent extends ChatMessage {
    /**
     * 批量处理视频文件
     *
     * @param {string} [userNeeds] - 用户需求
     * 
     * @returns {Promise<void>} - 处理完成后的 Promise
     */
    public async batchProcessVideoFiles(userNeeds?: string): Promise<void> {
        // 如果未读视频URL数组为空，直接返回
        if (this.unreadVideoUrl.length === 0) return;
        //  遍历未读视频URL数组
        for (const videoUrl of this.unreadVideoUrl) {
            try {
                // 处理视频文件
                await this.analysisVideoFile(videoUrl, userNeeds || '');
                // 等待1秒，避免对服务器造成过大压力
                await new Promise(resolve => setTimeout(resolve, 1000));
            }
            catch (error) { continue; }
        }
        // 清空未读视频URL数组
        this.unreadVideoUrl = [];
    }
    /**
     * 创建消息
     *
     * @returns {Promise<string>} - 最终应答
     */
    public async createChatMessage(): Promise<string> {
        /** 初始化聊天缓存 */
        const cache: ChatCache = {
            currentToolCallIndex: -1,
            currentFunctionArgs: '',
            currentFunctionName: '',
            descriptionContent: '',
            thinkingContent: '',
            currentToolCall: null,
            toolCalls: [],
        };
        // 发送请求并获取响应
        await this.callMultimediaAndToolParsing(cache);
        // 返回最终应答
        return this.finalResponse;
    }
}

export default LunarAgent;