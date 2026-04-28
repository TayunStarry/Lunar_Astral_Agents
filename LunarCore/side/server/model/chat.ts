import { OnlyData, ChatCache, modelResponse, ModelResponseBody, AgentDefine, ModelBuilder } from '../index';

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
            this.formatHistoricalMessages();
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
    /** 格式化历史消息 */
    public formatHistoricalMessages() {
        // 如果消息数组为空,则不处理
        if (this.messages.length === 0) return;
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
            const newThinkTag = '<think>\n' + state.thinkingContent + '\n</think>';
            // 修正复合描述内容
            source.finalResponse = newThinkTag + state.descriptionContent;
        }
        // 修正简单描述内容
        else source.finalResponse = state.descriptionContent;
        // 检查消息内容是否为空
        if (source.finalResponse.trim() === "") return source.defaultAnswer;
        return source.finalResponse;
    }
    /** 构造函数 */
    public constructor() {
        super();
        this.useMultimodal(fileView('prompts/chatRole.md')[0]);
    }
}