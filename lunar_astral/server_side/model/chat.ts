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
            // 从 chromem-go 查询相关历史消息作为 RAG 上下文
            this.queryRagMessages();
            /** 向处理器模型发送请求并等待响应 */
            const response = this.run(this.ragMessages);
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
        // 如果消息数组为空,则不处理
        if (this.messages.length === 0) return;
        /** 用于查重的文本消息映射表 */
        const textMessageMap = new Set<string>();
        /** 文本消息数组 */
        const textMessages: PostMessage[] = [];
        /** 视觉消息数组 */
        const visionMessages: PostMessage[] = [];
        /** 格式化后的消息数组 */
        const formatMessages: PostMessage[] = [];
        // 遍历并规整化消息数组
        for (const message of this.messages) {
            // 如果消息内容为字符串,则直接添加到文本消息数组
            if (typeof message.content === 'string') textMessages.push(message)
            // 如果消息内容为数组,则遍历并添加到文本消息数组或视觉消息数组
            else for (let index = 0; index < message.content.length; index++) {
                /** 当前消息内容 */
                const content = message.content[index];
                // 如果消息内容为文本,则添加到文本消息数组
                if (content.type == 'text') textMessages.push({ role: message.role, content: content.text })
                // 如果消息内容为视觉,则添加到视觉消息数组
                else visionMessages.push({ role: message.role, content: [content] })
            }
        }
        // 遍历文本消息数组并去除重复消息
        for (const message of textMessages) {
            // 过滤掉无效的消息
            if (typeof message.content !== 'string' || textMessageMap.has(message.content)) continue;
            // 将提取出来的文本消息合并到格式化消息数组中
            formatMessages.push(message);
            // 将文本消息内容添加到映射表中
            textMessageMap.add(message.content);
        }
        // 如果视觉消息数量小于等于10,则合并到格式化消息数组中
        if (visionMessages.length <= 10) formatMessages.push(...visionMessages);
        // 如果视觉消息数量大于10,则分批次处理
        else for (let i = 0; i < visionMessages.length; i += 10) {
            /** 截取当前批次的视觉消息（每批次最多10条） */
            const batchFrames = visionMessages.slice(i, i + 10);
            // 覆盖描述角色的上下文，传入当前批次的视觉消息
            source.descriptionRole.coverContext(batchFrames);
            /** 执行描述角色的模型运行，获取总结请求响应 */
            const summaryRequest = source.descriptionRole.run([]);
            /** 模型总结结果 */
            const summary = summaryRequest.body?.choices?.[0]?.message?.content;
            // 过滤空字符串和仅包含空格的字符串
            if (summary && summary.trim().length > 0) formatMessages.push({ role: 'user', content: summary });
        }
        // 覆写处理器模型的上下文为格式化后的消息数组
        this.messages = formatMessages;
        /** 最新消息的角色 */
        const latestRole = this.messages.slice(-1)[0].role;
        // 如果最新消息是用户,则不处理
        if (latestRole === 'user') return;
        // 如果最新消息是模型,则添加提示消息
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
                console.log(`预测令牌数: ${message.timings.predicted_per_second}`);
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
        super(fileView('prompts/chatRole.md')[0]);
    }
}