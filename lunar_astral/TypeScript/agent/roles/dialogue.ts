import { GlobalConfig, ChatCache, ModelResponseBody, ModelBuilder, PostMessage, LiteImageFile, descriptionRole, memorizerRole } from '../../index';

/** 聊天对话角色 */
export class DialogueRole extends ModelBuilder {
    /** 生成聊天对话 */
    public async generateDialogue(cache: ChatCache): Promise<void> {
        try {
            // 对消息中的图片文件进行压缩与解析处理
            await LiteImageFile();
            /** 检查是否强制使用工具调用 */
            const useTools = GlobalConfig.unreadContext.some(context => context.role == 'user' && typeof context.content === 'string' && context.content.startsWith('<律令>:'))
            if (useTools) console.log('强制月华使用工具调用中...');
            // 将未读上下文数组中的消息添加到处理器模型的上下文
            GlobalConfig.unreadContext.forEach(context => this.writeContext(context));
            // 清空未读上下文数组
            GlobalConfig.unreadContext = [];
            // 格式化历史消息
            this.formatHistoricalMessages();
            // 添加当前时间到运行时消息列表
            this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
            // 从向量数据库查询相关历史消息作为 RAG 上下文
            this.queryRagMessages();
            /** 向处理器模型发送请求并等待响应 */
            const response = this.run(this.ragMessages, GlobalConfig.LTPdefinition, useTools);
            // 处理响应文本内容
            this.analyzeMessageResponse(response.body, cache);
            // 如果有工具调用,处理它们并重新发送请求
            if (cache.toolCalls.length > 0) {
                // 在递归前写入 assistant 的 tool_call 消息，确保上下文完整
                this.writeContext(response.body.choices?.[0]?.message);
                /** 处理工具调用 */
                const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
                // 如果有处理过的工具调用,重新发送请求（包含工具调用结果）
                if (hasProcessedToolCalls) return await this.generateDialogue(cache);
            }
            // 在历史上下文中添加模型响应
            this.writeContext(response.body.choices?.[0]?.message);
        }
        catch (error) {
            console.error('请求处理错误:', error);
        }
        // 更新消息内容
        this.updateMessageContent(cache);
    }
    /** 格式化历史消息：图片总数≥20时先摘要再扁平化为纯文本 */
    public formatHistoricalMessages() {
        // 如果消息数组为空,则不处理
        if (this.messages.length === 0) return;
        /** 整个消息队列中的图片帧总数 */
        const totalImages = this.countTotalImages(this.messages);
        // 图片帧数≥20：对每个多媒体消息摘要图片，最终全部扁平化为纯文本
        if (totalImages >= 20) {
            /** 处理后的纯文本消息数组 */
            const processedMessages: PostMessage[] = [];
            // 遍历所有消息对象
            for (const message of this.messages) {
                // 已是纯文本：直接保留
                if (typeof message.content === 'string') {
                    processedMessages.push(message);
                    continue;
                }
                /** 对多媒体消息执行图片摘要，合并为纯文本 */
                const textResult = this.summarizeMessageImages(message);
                // 如果摘要结果为空或仅包含空格,则跳过
                if (!textResult || textResult.trim() === '') continue;
                // 保留摘要结果
                processedMessages.push({ role: message.role, content: textResult });
            }
            this.messages = processedMessages;
        }
        // 如果处理后消息数组为空（摘要全部失败等极端情况），跳过续写
        if (this.messages.length === 0) return;
        // 判定最后一条消息是否是用户消息
        if (this.messages.slice(-1)[0].role === 'user') return;
        // 如果不是用户消息就补充一条默认用户消息
        this.writeContext({ role: 'user', content: "继续" });
    }
    /** 对单个多媒体消息中的图片执行摘要，返回合并后的纯文本；无内容时返回 null */
    private summarizeMessageImages(message: PostMessage): string | null {
        if (typeof message.content === 'string') return message.content;
        /** 消息中的图片内容项 */
        const imageItems = message.content.filter((c: any) => c.type === 'image_url');
        /** 消息中的文本内容项 */
        const textItems = message.content.filter((c: any) => c.type === 'text');
        /** 原始文本部分 */
        const textPart = textItems.map((c: any) => c.text).join('\n');
        // 无图片时直接返回文本
        if (imageItems.length === 0) return textPart || null;
        try {
            // 将图片包装为独立消息，喂给描述角色进行摘要
            descriptionRole.coverContext({ role: 'user', content: imageItems });
            /** 运行描述角色模型，获取图片摘要 */
            const summaryRequest = descriptionRole.run([], []);
            /** 图片摘要结果 */
            const summary = summaryRequest.body?.choices?.[0]?.message?.content;
            // 检查摘要结果是否有效
            if (summary && summary.trim().length > 0) {
                return textPart ? `${textPart}\n[图片描述：${summary}]` : `[图片描述：${summary}]`;
            }
            // 摘要为空时仅保留文本
            return textPart || null;
        }
        catch (error) {
            console.error('[对话者] 图片摘要异常:', error);
            return textPart || null;
        }
    }
    /** 统计单条消息中的 image_url 项数量 */
    private countImagesInMessage(message: PostMessage): number {
        if (typeof message.content === 'string') return 0;
        return message.content.filter((c: any) => c.type === 'image_url').length;
    }
    /** 统计消息数组中所有图片帧的总数 */
    private countTotalImages(messages: PostMessage[]): number {
        return messages.reduce((sum, m) => sum + this.countImagesInMessage(m), 0);
    }
    /** 处理聊天消息响应 */
    protected analyzeMessageResponse(message: ModelResponseBody, cache: ChatCache): void {
        try {
            // 处理推理内容数据
            if (message.choices?.[0]?.message?.reasoning_content) {
                cache.thinkingContent = message.choices[0].message.reasoning_content;
            }
            // 检查是否有词元生成速度数据
            if (message.timings?.predicted_per_second) {
                console.log(`词元生成速度: ${message.timings.predicted_per_second}`);
            }
            // 检查缓存命中数量
            if (message.timings?.cache_n !== undefined) {
                console.log(`缓存命中数量: ${message.timings.cache_n}`);
            }
            // 处理工具调用
            if (message.choices?.[0]?.message?.tool_calls) {
                // 遍历所有工具调用
                for (const toolCall of message.choices[0].message.tool_calls) {
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
            if (message.choices?.[0]?.message?.content) {
                cache.descriptionContent = message.choices[0].message.content;
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
            /** 工具函数名称 */
            const functionName = toolCall.function.name;
            /** 工具函数参数 */
            const functionArgs = toolCall.function.arguments;
            /** 查询对应的月华工具包 */
            const lunarToolPackage = GlobalConfig.LTPfunction.get(functionName);
            // 检查是否有对应的工具包
            if (!lunarToolPackage) {
                this.messages.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
                continue;
            }
            try {
                /** 工具函数执行结果：string[]格式，下标0=文本内容，下标1=图片base64数据（无则为空字符串） */
                const toolResult = await lunarToolPackage(functionArgs);
                /** 提取文本内容 */
                const textContent = Array.isArray(toolResult) ? toolResult[0] : String(toolResult);
                /** 提取图片base64数据 */
                const base64Image = Array.isArray(toolResult) ? toolResult[1] : '';
                // 将工具响应添加到消息历史中，图片与文本合并为单条消息
                if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
                    /** 合并文本与图片消息 */
                    const message: PostMessage = {
                        role: "user",
                        content: [
                            { type: "text", text: textContent },
                            { type: "image_url", image_url: { url: base64Image } }
                        ]
                    }
                    this.messages.push(message);
                    console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
                }
                else this.messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
                // 标记有工具调用
                hasToolCalls = true;
            }
            catch (error) {
                // 将工具调用失败信息添加到消息历史中
                this.messages.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
            }
        }
        // 处理完所有工具调用后,清空状态
        state.currentToolCallIndex = -1;
        state.currentFunctionArgs = "";
        state.currentFunctionName = "";
        state.currentToolCall = null;
        state.toolCalls = [];
        // 标记有工具调用
        return hasToolCalls;
    };
    /** 更新消息内容 */
    protected updateMessageContent(state: ChatCache): string {
        // 检查推理内容是否为空
        if (state.thinkingContent.trim() !== "") {
            /** 新的思考标签内容 */
            const newThinkTag = '<think>\n' + state.thinkingContent + '\n</think>\n';
            // 合并为带有思考标签的描述内容
            GlobalConfig.finalResponse = state.descriptionContent;
            // 打印思考标签内容
            console.log(newThinkTag);
        }
        // 直接使用描述内容
        else GlobalConfig.finalResponse = state.descriptionContent;
        // 返回修正后的消息内容
        return GlobalConfig.finalResponse;
    }
    /** 获取最新的5条用户消息内容 */
    public getLatestUserMessages(): string[] {
        /** 收集到的用户消息文本 */
        const userTexts: string[] = [];
        // 从消息列表的末尾开始遍历，收集最新的5条用户消息
        for (let i = this.messages.length - 1; i >= 0 && userTexts.length < 5; i--) {
            /** 检查当前消息是否为用户消息 */
            const message = this.messages[i];
            if (message.role === 'user') {
                // 提取文本内容
                if (typeof message.content === 'string') {
                    userTexts.unshift(message.content);
                } else if (Array.isArray(message.content)) {
                    const textContent = message.content
                        .filter(item => item.type === 'text')
                        .map(item => item.text)
                        .join(' ');
                    if (textContent.trim()) userTexts.unshift(textContent);
                }
            }
        }
        return userTexts;
    }
    /**
     * 从 记忆库 检索并生成摘要，填充 ragMessages
     *
     * 由记忆者智能体承接：继承原有搜索机制检索长期记忆，对命中的内容碎片做一次
     * 总结与摘要，输出一篇硬切断 4096 的连贯摘要（摘要不回写记忆库），
     * 替代以往将碎片直接写入 rag 数组的做法。
     */
    public queryRagMessages(): this {
        /** 获取最新的5条用户消息作为查询条件 */
        const userMessages = this.getLatestUserMessages();
        /** 清理RAG消息并返回 */
        const clear = () => { this.ragMessages = []; return this; };
        // 如果没有用户消息，清理RAG消息并返回
        if (userMessages.length === 0) return clear();
        /** 由记忆者智能体检索长期记忆并生成摘要 */
        const digest = memorizerRole.queryRagSummary(userMessages);
        // 检索无命中或摘要失败时，清理RAG消息并返回
        if (!digest) return clear();
        // 将连贯摘要作为一条用户消息注入 ragMessages（替换碎片式上下文；
        // 用 user 角色而非 system，符合 OpenAI 协议建议——避免消息队列中堆积多条系统消息）
        this.ragMessages = [{ role: 'user', content: `【长期记忆摘要】\n${digest}` }];
        return this;
    }
    /** 构造函数 */
    public constructor() {
        super(fileView('prompts/dialogueRole.md')[0]);
    }
}