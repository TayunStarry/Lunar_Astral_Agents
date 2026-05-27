import { OnlyData, ChatCache, modelResponse, ModelResponseBody, AgentDefine, ModelBuilder, PostMessage } from '../index';

/** 聊天对话角色 */
export class ChatDialogueRole extends ModelBuilder {
    /** 发送请求并获取响应 */
    public async callMultimediaAndToolParsing(cache: ChatCache, source: AgentDefine): Promise<void> {
        try {
            // 对消息中的图片文件进行压缩与解析处理
            await source.LiteImageFile();
            // 将未读上下文数组中的消息添加到处理器模型的上下文
            source.unreadContext.forEach(context => this.writeContext(context));
            // 清空未读上下文数组
            source.unreadContext = [];
            // 格式化历史消息
            this.formatHistoricalMessages(source);
            // 替换系统提示词中的时间占位符
            this.systemPrompt = this.systemPrompt.replace(/{current-time}/g, new Date().toLocaleString());
            /** 向处理器模型发送请求并等待响应 */
            const response = this.run as modelResponse;
            // 处理响应文本内容
            this.analyzeMessageResponse(response.body, cache, source);
            // 如果有工具调用,处理它们并重新发送请求
            if (cache.toolCalls.length > 0) {
                /** 处理工具调用 */
                const hasProcessedToolCalls = await this.batchExecutionToolCall(cache, source);
                // 如果有处理过的工具调用,重新发送请求（包含工具调用结果）
                if (hasProcessedToolCalls) return await this.callMultimediaAndToolParsing(cache, source);
            }
            // 在历史上下文中添加模型响应
            this.writeContext(response.body.choices?.[0]?.message);
        }
        catch (error) {
            console.error('请求处理错误:', error);
        }
        // 更新消息内容
        this.updateMessageContent(cache, source);
    }
    /** 文本消息最大长度阈值 */
    private static readonly MAX_TEXT_LENGTH = 512;

    /** 格式化历史消息 */
    public formatHistoricalMessages(source: AgentDefine) {
        // 如果历史消息为空,直接返回
        if (this.messages.length === 0) return;
        /** 文本消息去重集合 */
        const textMessageMap = new Set<string>();
        /** 文本消息数组,用于存储去重后的文本消息 */
        const textMessages: PostMessage[] = [];
        /** 视觉消息数组,用于存储视觉消息 */
        const visionMessages: PostMessage[] = [];
        /** 格式化后的消息数组 */
        const formatMessages: PostMessage[] = [];
        // 遍历历史消息,将文本消息和视觉消息分别存储到对应的数组中
        for (const message of this.messages) {
            // 如果消息内容为字符串,则直接添加到文本消息数组
            if (typeof message.content === 'string') textMessages.push(message);
            // 如果消息内容为数组,则遍历数组,将每个元素添加到对应的数组中
            else for (let index = 0; index < message.content.length; index++) {
                /** 当前消息内容 */
                const content = message.content[index];
                // 如果当前消息内容为文本,则添加到文本消息数组
                if (content.type === 'text') textMessages.push({ role: message.role, content: content.text });
                // 如果当前消息内容为视觉,则添加到视觉消息数组
                else visionMessages.push({ role: message.role, content: [content] });
            }
        }

        for (const message of textMessages) {
            if (typeof message.content !== 'string' || textMessageMap.has(message.content)) continue;
            formatMessages.push(message);
            textMessageMap.add(message.content);
        }

        if (visionMessages.length <= 10) {
            formatMessages.push(...visionMessages);
        }
        else for (let i = 0; i < visionMessages.length; i += 10) {
            const batchFrames = visionMessages.slice(i, i + 10);
            source.descriptionRole.coverContext(batchFrames);
            const summaryRequest = source.descriptionRole.run as modelResponse;
            const summary = summaryRequest.body?.choices?.[0]?.message?.content;
            if (summary && summary.trim().length > 0) formatMessages.push({ role: 'user', content: summary });
        }

        for (let i = 0; i < formatMessages.length; i++) {
            const msg = formatMessages[i];
            if (typeof msg.content === 'string' && msg.content.length > ChatDialogueRole.MAX_TEXT_LENGTH) {
                const chunks: PostMessage[] = [];
                for (let j = 0; j < msg.content.length; j += ChatDialogueRole.MAX_TEXT_LENGTH) {
                    chunks.push({ role: msg.role, content: msg.content.slice(j, j + ChatDialogueRole.MAX_TEXT_LENGTH) });
                }
                formatMessages.splice(i, 1, ...chunks);
                i += chunks.length - 1;
            }
        }

        this.messages = formatMessages;

        if (this.messages.length === 0) return;

        const latestRole = this.messages[this.messages.length - 1].role;
        if (latestRole === 'user') return;

        this.writeContext({ role: 'user', content: '请继续之前的话题，或者对之前的内容进行优化完善。' });
    }
    /** 处理聊天消息响应 */
    protected analyzeMessageResponse(message: ModelResponseBody, cache: ChatCache, source: AgentDefine): void {
        try {
            // 处理推理内容数据
            if (message.choices?.[0]?.message?.reasoning_content) {
                cache.thinkingContent = message.choices[0].message.reasoning_content;
            }
            // 检查是否有预测令牌数
            if (message.timings?.predicted_per_second) {
                source.responseSpeed = message.timings.predicted_per_second;
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
    protected async batchExecutionToolCall(state: ChatCache, source: AgentDefine): Promise<boolean> {
        /** 工具调用标志 */
        let hasToolCalls = false;
        // 遍历所有工具调用
        for (const toolCall of state.toolCalls) {
            /** 工具函数名称 */
            const functionName = toolCall.function.name;
            /** 工具函数参数 */
            const functionArgs = toolCall.function.arguments;
            /** 查询对应的月华工具包 */
            const lunarToolPackage = OnlyData.lunarToolPackageMap.get(functionName);
            // 检查是否有对应的工具包
            if (!lunarToolPackage) {
                source.unreadContext.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
                continue;
            }
            try {
                /** 工具函数执行结果 */
                const toolResult = await lunarToolPackage(functionArgs);
                // 将工具响应添加到消息历史中
                source.unreadContext.push({ role: "tool", content: toolResult, tool_call_id: toolCall.id });
                // 标记有工具调用
                hasToolCalls = true;
            }
            catch (error) {
                // 将工具调用失败信息添加到消息历史中
                source.unreadContext.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
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
    protected updateMessageContent(state: ChatCache, source: AgentDefine): string {
        // 检查推理内容是否为空
        if (state.thinkingContent.trim() !== "") {
            /** 新的思考标签内容 */
            const newThinkTag = '<think>\n' + state.thinkingContent + '\n</think>\n';
            // 合并为带有思考标签的描述内容
            // source.finalResponse = newThinkTag + state.descriptionContent;
            source.finalResponse = state.descriptionContent;
            // 打印思考标签内容
            console.log(newThinkTag);
        }
        // 直接使用描述内容
        else source.finalResponse = state.descriptionContent;
        // 返回修正后的消息内容
        return source.finalResponse;
    }
    /** 构造函数 */
    public constructor() {
        super();
        this.useMultimodal(fileView('prompts/chatRole.md')[0]);
    }
}