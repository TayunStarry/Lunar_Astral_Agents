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
    fetchDocumentCallback,
    getPromptFromDatabase,
    savePromptToDatabase
} from '../../FileSystem/index';
import { ProtoAgent } from '../index';

/** 视频分析智能体 */
class VideoAnalysis extends ProtoAgent {
    /**
     * 处理视频文件
     *
     * @param {File} videoFile - 视频文件对象
     * 
     * @param {string} userNeeds - 用户需求
     * 
     * @returns {Promise<void>} - 处理完成后的 Promise
     */
    protected async analysisVideoFile(videoFile: File, userNeeds: string): Promise<void> {
        /** 检查是否已处理过该视频 */
        const cachedPrompt = getPromptFromDatabase(videoFile.name);
        // 如果视频已处理过，直接添加到未读上下文
        if (cachedPrompt) {
            this.unreadContext.push({ role: 'user', content: cachedPrompt });
            return;
        }
        /** FormData 对象，用于上传视频文件 */
        const formData = new FormData();
        // 添加视频文件到 FormData
        formData.append('video', videoFile, 'video.mp4');
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
            // 覆写 视频描述模型 上下文
            this.videoDescription.coverContext({ role: 'user', content: batchFrames });
            /** 调用模型进行画面总结 */
            const summaryRequest = await (await this.videoDescription.run as Response).json();
            /** 模型总结结果 */
            const summary = summaryRequest?.choices?.[0]?.message?.content;
            // 过滤空字符串和仅包含空格的字符串
            if (summary && summary.trim().length > 0) sandboxMessages.push(summary);
        }
        // 判断是否包含多个批处理片段
        if (sandboxMessages.length > 1) {
            // 覆写 视频摘要模型 上下文
            this.videoSummary.coverContext({ role: 'user', content: sandboxMessages });
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
        if (videoSummary) savePromptToDatabase(videoFile.name, videoSummary);
    }
}

/** 聊天消息智能体 */
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

export class AgentSkill extends ChatMessage { }