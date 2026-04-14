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
    TextContent
} from '../../config/index';

import {
    fetchMarkdown,
    fetchDocumentCallback,
    getPromptFromDatabase,
    savePromptToDatabase
} from '../../FileSystem/index';

/** 当前的真实地址位置 */
let currentAddress: string[] = [];

class data {
    protected isMultimodal: boolean = true;
    /** 是否启用流式响应 */
    protected stream: boolean = false;
    /** 是否启用工具调用 */
    protected enableTools: boolean = true;
    /** 消息列表 */
    protected messages: PostMessage[] = [];
    /** 中止信号 */
    protected signal: AbortSignal | undefined = undefined;
    /** 系统提示 */
    protected systemPrompt: string = "你的名字叫做月华, 是一个女孩子";
    /** 私有化构造函数，防止外部实例化 */
    protected constructor() { }
}

class tool extends data {
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

class style extends tool {
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
    public useEmbedding(): this {
        // 设置为嵌入模式   
        this.isMultimodal = false;
        return this;
    }
}

class alter extends style {
    public setStream(stream: boolean = false): this {
        this.stream = stream;
        return this;
    }
    public setEnableTools(enable: boolean = true): this {
        this.enableTools = enable;
        return this;
    }
    public writeContext(context: PostMessage): this {
        if (this.messages.length > 30) this.messages.slice(-30).push(context);
        else this.messages.push(context);
        return this;
    }
    public setContext(contexts: PostMessage[]): this {
        this.messages = contexts;
        return this;
    }
    public setSignal(signal: AbortSignal): this {
        this.signal = signal;
        return this;
    }
}

class build extends alter {
    public get run(): Promise<Response> | Promise<number[]> {
        if (this.isMultimodal) return this.runMultimodal();
        else return this.runEmbedding();
    }
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
    public constructor() { super(); }
}

class LunarAgent {
    /** 构建计划 */
    protected compilePlan: build = new build();
    /** 推理关键词 */
    protected queryKeywords: build = new build();
    /** 情感管理器 */
    protected emotionManager: build = new build();
    /** 视频摘要 */
    protected videoSummary: build = new build();
    /** 视频描述 */
    protected videoDescription: build = new build();
    /** 聊天回复 */
    protected chatReply: build = new build().useMultimodal();
    /** 嵌入向量 */
    public embedding: build = new build().useEmbedding();
    /** 未读上下文 */
    protected unreadContext: PostMessage[] = [];
    /** 未读视频URL */
    protected unreadVideoUrl: string[] = [];
    /** 构建智能体 并 初始化各个子模型的系统提示词 */
    constructor() {
        // 初始化全部模型的系统提示词
        fetchMarkdown('/read/resources/prompts/compilePlan.md').then(content => this.compilePlan.useMultimodal(content));
        fetchMarkdown('/read/resources/prompts/queryKeywords.md').then(content => this.queryKeywords.useMultimodal(content));
        fetchMarkdown('/read/resources/prompts/emotionManager.md').then(content => this.emotionManager.useMultimodal(content));
        fetchMarkdown('/read/resources/prompts/videoSummary.md').then(content => this.videoSummary.useMultimodal(content));
        fetchMarkdown('/read/resources/prompts/videoDescription.md').then(content => this.videoDescription.useMultimodal(content));
        // 初始化 自定义配置文件
        fetchDocumentCallback('resources/custom_config.json').then(content => OnlyData.customConfig = JSON.parse(content));
        // TODO 初始化 工具调用配置
        // fetchDocumentCallback('resources/toolCall.json').then(content => OnlyData.toolCall = JSON.parse(content));
        // TODO 初始化 聊天记录
        // fetchDocumentCallback('resources/chatRecord.json')
    }
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
        else videoSummary = sandboxMessages[0].text;
        // 将视频总结结果添加到消息数组
        if (videoSummary) this.unreadContext.push({ role: 'user', content: videoSummary });
        // 如果用户需求非空，添加到消息数组
        if (userNeeds.trim().length > 0) this.unreadContext.push({ role: 'user', content: userNeeds });
        // 缓存处理结果到数据库
        if (videoSummary) await savePromptToDatabase(videoUrl, videoSummary);
    }
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
            // 处理视频文件
            await this.analysisVideoFile(videoUrl, userNeeds || '');
            // 等待1秒，避免对服务器造成过大压力
            await new Promise(resolve => setTimeout(resolve, 1000));
        }
        // 清空未读视频URL数组
        this.unreadVideoUrl = [];
    }
}