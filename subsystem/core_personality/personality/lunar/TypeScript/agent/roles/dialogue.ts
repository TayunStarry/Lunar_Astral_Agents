import { GlobalConfig } from '../../config/global';
import { ChatCache } from '../../config/config';
import { ToolCall } from '../../config/tool';
import { MessageContent, ModelResponseBody, PostMessage } from '../../config/model';
import { ModelBuilder } from '../base/builder';
import { LiteImageFile } from '../capabilities/media';
import {
    clearPendingTags,
    demoteContentImages,
    getCacheEntry,
    putCacheEntry,
    sanitizeImageDesc,
    setCacheDesc,
    stashPendingTag,
    takePendingTag,
    unpinCacheTag
} from '../capabilities/image-cache';
import { getCrystalRemoteTools } from '../capabilities/ltpx';
import { agentControlTools } from '../../tool/agent-control';
import { imageRecallTools } from '../../tool/image-recall';
import { scheduleTools } from '../../tool/schedule-defs';
import { screenshotTools } from '../../tool/screenshot';

/** 对话者固有工具（除琉璃聚合工具外全部固定不变，定义随本模块静态聚合） */
const dialogueBaseTools: ToolCall[] = [...agentControlTools, ...imageRecallTools, ...scheduleTools, ...screenshotTools];

/** 单次应答允许的工具调用轮次上限（防御模型在工具调用间自循环） */
const MAX_TOOL_ROUNDS = 5;

/** 聊天对话角色 */
export class DialogueRole extends ModelBuilder {
    /** 智能体角色标签（TUI 实况屏显示） */
    protected roleTag: string = "对话者";

    /** 生成聊天对话 */
    public async generateDialogue(cache: ChatCache): Promise<void> {
        try {
            // 对消息中的图片文件进行压缩与解析处理
            await LiteImageFile();
            // 首轮调用（上下文为空即为新一轮应答）重置工具轮次计数；递归轮次保留计数
            if (this.messages.length === 0) this.toolRounds = 0;
            // 将未读上下文数组中的消息添加到处理器模型的上下文
            GlobalConfig.unreadContext.forEach(context => this.writeContext(context));
            // 清空未读上下文数组
            GlobalConfig.unreadContext = [];
            // 格式化历史消息
            this.formatHistoricalMessages();
            // 添加当前时间到运行时消息列表
            this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
            /** 向处理器模型发送请求并等待响应（固有工具 + 琉璃聚合工具常驻槽位，工具链变化时刷新定义） */
            const response = this.run(this.ragMessages, [...dialogueBaseTools, ...getCrystalRemoteTools()]);
            // 处理响应文本内容
            this.analyzeMessageResponse(response.body, cache);
            // 如果有工具调用,处理它们并重新发送请求
            if (cache.toolCalls.length > 0) {
                // 在递归前写入 assistant 的 tool_call 消息，确保上下文完整
                this.writeContext(response.body.choices?.[0]?.message);
                // 达到工具调用轮次上限：停止递归，避免模型在工具间自循环
                if (this.toolRounds >= MAX_TOOL_ROUNDS) {
                    console.warn(`[对话者] 工具调用轮次达到上限（${MAX_TOOL_ROUNDS}），停止继续调用`);
                    GlobalConfig.finalResponse = cache.descriptionContent;
                    return;
                }
                /** 处理工具调用 */
                const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
                // 如果有处理过的工具调用,重新发送请求（包含工具调用结果）
                if (hasProcessedToolCalls) {
                    this.toolRounds++;
                    return await this.generateDialogue(cache);
                }
            }
            // 在历史上下文中添加模型响应
            this.writeContext(response.body.choices?.[0]?.message);
        }
        catch (error) {
            console.error('请求处理错误:', error);
        }
        // 更新消息内容
        GlobalConfig.finalResponse = cache.descriptionContent;
    }
    /** 多模态图片窗口上限：更早的图片降级为 [图片.00x] 占位标签，可经 describe_images 按标签取回 */
    private readonly IMAGE_WINDOW = 20;
    /** 本轮已发生的工具调用轮次（防御模型自循环） */
    private toolRounds = 0;

    /** 格式化历史消息：窗内图片超限时增量降级最老的图片，末尾非用户消息时补续写指令 */
    public formatHistoricalMessages() {
        // 如果消息数组为空,则不处理
        if (this.messages.length === 0) return;
        // 增量降级：只摘除超出窗口的最老图片（避免一次性改写整个历史，保持前缀缓存局部性）
        this.demoteOverflowImages();
        // 如果处理后消息数组为空（极端情况），跳过续写
        if (this.messages.length === 0) return;
        // 判定最后一条消息是否是用户消息
        if (this.messages.slice(-1)[0].role === 'user') return;
        // 如果不是用户消息就补充一条默认用户消息
        this.writeContext({ role: 'user', content: "继续" });
    }

    /** 增量降级：窗内多模态图片超过上限时，从最老的消息开始摘图留签 */
    private demoteOverflowImages(): void {
        /** 窗内多模态图片计数 */
        const countImages = (): number => this.messages.reduce(
            (sum, message) => typeof message.content === 'string'
                ? sum
                : sum + message.content.filter((c: any) => c.type === 'image_url').length,
            0
        );
        for (let i = 0; i < this.messages.length && countImages() > this.IMAGE_WINDOW; i++) {
            const message = this.messages[i];
            // 纯文本消息与无图消息跳过
            if (typeof message.content === 'string') continue;
            if (!message.content.some((c: any) => c.type === 'image_url')) continue;
            this.demoteMessageImages(message);
        }
    }

    /** 将单条消息中的图片降级为占位标签：移除多模态项与旁注，改写为 [图片.00x]（描述） */
    private demoteMessageImages(message: PostMessage): void {
        if (typeof message.content === 'string') return;
        message.content = demoteContentImages(
            message.content,
            source => this.resolveImageTag(message, source),
            (tag, source) => this.describeImage(tag, source)
        ) as MessageContent[];
        // 降级结束：暂存标签已消费完毕，清理避免跨轮泄漏
        clearPendingTags(message);
    }

    /**
     * 取该图片的标签：优先本轮暂存标签，其次消息内相邻裸标签旁注，最后现注册兜底
     *
     * 兜底路径（工具结果内嵌图片等无旁注场景）以 pinned=true 注册：图片此刻仍在多模态
     * 窗口内，钉住可避免被 FIFO 淘汰；该条目随即随摘图一同解钉（见 describeImage）。
     */
    private resolveImageTag(message: PostMessage, source: string): string {
        const pending = takePendingTag(message, source);
        if (pending) return pending;
        if (!source) return '';
        return putCacheEntry(source, true);
    }

    /**
     * 获取图片条目描述（占位文本 [图片.00x]（描述）中括号内的内容来源）：
     * 缓存命中直接返回；未命中时用视觉模型对源图生成一次并回写缓存。
     * 返回值为已规整文本（折叠空白、剔除括号类结构字符、20 字上限），
     * 保证拼接进占位后不破坏标签形态、不残留多余文本。
     */
    private describeImage(tag: string, source: string): string {
        // 图片已离开多模态窗口：解除钉住，恢复其可淘汰资格（窗口内条目始终钉住）
        unpinCacheTag(tag);
        const entry = getCacheEntry(tag);
        if (entry?.desc) return sanitizeImageDesc(entry.desc, 20);
        if (!source) return '';
        try {
            // 图片包装为独立消息，喂给描述角色生成一次性摘要（单图单次调用）
            this.descriptionRole.coverContext({
                role: 'user',
                content: [
                    { type: 'text', text: '用一句不超过20字的中文描述这张图片的主体与场景，只输出描述本身。' },
                    { type: 'image_url', image_url: { url: source } }
                ]
            });
            const response = this.descriptionRole.run([], []);
            const desc = sanitizeImageDesc(response.body?.choices?.[0]?.message?.content || '', 20);
            if (desc) setCacheDesc(tag, desc);
            return desc;
        }
        catch (error) {
            console.error('[对话者] 图片描述生成失败:', error);
            return '';
        }
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
                // 将工具响应添加到消息历史中：文本恒以 tool 角色回填，保证 tool_call_id 配对完整
                this.messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
                // 工具返回的图片（如截图）作为紧随其后的用户多模态消息注入：
                // 注册共享缓存并打 [图片.00x] 标签旁注，使其与用户图片一样可按标签引用、
                // 一样参与窗口降级（此前该路径既不注册也不打标签，窗口期内无句柄可用）
                if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
                    /** 工具图片消息 */
                    const message: PostMessage = {
                        role: "user",
                        content: [
                            { type: "text", text: `【工具返回的图片】${textContent}` },
                            { type: "image_url", image_url: { url: base64Image } }
                        ]
                    };
                    const tag = putCacheEntry(base64Image, true);
                    if (tag) {
                        message.content.push({ type: "text", text: `[${tag}]` });
                        stashPendingTag(message, base64Image, tag);
                    }
                    this.messages.push(message);
                    console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节，标签=${tag || '注册失败'}`);
                }
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
    /** 构造函数 */
    public constructor(private descriptionRole: ModelBuilder) {
        super(fileView('prompts/dialogueRole.md')[0]);
    }
}