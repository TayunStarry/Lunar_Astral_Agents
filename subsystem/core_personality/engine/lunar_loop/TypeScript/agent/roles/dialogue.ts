import { GlobalConfig } from '../../config/global';
import { ChatCache } from '../../config/config';
import { ToolCall } from '../../config/tool';
import { MessageContent, ModelResponseBody, PostMessage } from '../../config/model';
import { ModelBuilder } from '../base/builder';
import { LiteImageFile } from '../capabilities/media';
import {
    attachImageTags,
    composeImagePlaceholder,
    handleFromVisionURL,
    IMAGE_WINDOW_SIZE,
    sanitizeImageDesc
} from '../capabilities/image-tag';
import { getResourceEntry, setResourceSummary } from '../capabilities/resource';
import { getCrystalRemoteTools } from '../capabilities/ltpx';
import { agentControlTools } from '../../tool/agent-control';
import { imageRecallTools } from '../../tool/image-recall';
import { resourceTools } from '../../tool/resource';
import { scheduleTools } from '../../tool/schedule-defs';
import { screenshotTools } from '../../tool/screenshot';
import { RandomFloor } from '../../tool/math';

/** 对话者固有工具（除琉璃聚合工具外全部固定不变，定义随本模块静态聚合） */
const dialogueBaseTools: ToolCall[] = [...agentControlTools, ...imageRecallTools, ...resourceTools, ...scheduleTools, ...screenshotTools];

/** 单次应答允许的工具调用轮次上限（防御模型在工具调用间自循环） */
const MAX_TOOL_ROUNDS = 5;

/** JSON 容错解析（工具参数在个别路径上可能是字符串） */
function parseToolArgs(raw: unknown): Record<string, any> {
    if (raw && typeof raw === 'object') return raw as Record<string, any>;
    try {
        const parsed = JSON.parse(String(raw ?? '{}'));
        return parsed && typeof parsed === 'object' ? parsed : {};
    }
    catch {
        return {};
    }
}

// ==== 快速应答台词：按「起句 + 动作 + 收尾」拼接，贴合月华俏皮自信、口语化的人设 ====

/** 起句（俏皮、自信，不用「安排好了」这类状态播报） */
const ACK_LEADS = ['收到', '好嘞', '交给我吧', '没问题', '这个我来'];

/** 收尾（自然口语，带一点活泼的语气） */
const ACK_ENDS = ['～', '，稍等我一下～', '，等我好消息～', '，一会儿就好～'];

/** 绘图动作 */
const DRAW_ACTIONS = ['这就动笔', '这就开画', '马上开工', '先起个稿'];

/** 联网搜索动作 */
const SEARCH_ACTIONS = ['这就去翻资料', '这就去查', '马上去找', '先去翻一翻'];

/** 从数组里随机取一项 */
function pickAckPart(parts: string[]): string {
    return parts[RandomFloor(0, parts.length - 1)];
}

/** 拼接一条快速应答台词 */
function composeAck(actions: string[]): string {
    return `${pickAckPart(ACK_LEADS)}！${pickAckPart(actions)}${pickAckPart(ACK_ENDS)}`;
}

/**
 * 高耗时工具的快速应答文案
 *
 * 只覆盖「用户会明显等待」的两类任务：绘制者与 WebLTP 联网搜索。
 * 返回空串表示不是高耗时工具，不需要快速应答。
 */
function slowTaskAckText(functionName: string, rawArgs: unknown): string {
    if (functionName === 'dispatch_painter') return composeAck(DRAW_ACTIONS);
    if (functionName === 'use_the_program') {
        const target = String(parseToolArgs(rawArgs).tool || '').trim().toLowerCase();
        if (target === 'web_search') return composeAck(SEARCH_ACTIONS);
    }
    return '';
}

/**
 * 发送一次快速应答（不走模型思考，直接文本 + TTS）
 *
 * 一轮思考链内只触发一次（GlobalConfig.fastAckSent）。QQ 来源的消息把
 * auto 置为 true：QQ 桥接会正常转发，但不占用本轮应答份额，真正的最终回应随后生成。
 */
function maybeSendFastAck(functionName: string, rawArgs: unknown): void {
    if (GlobalConfig.fastAckSent) return;
    const text = slowTaskAckText(functionName, rawArgs);
    if (!text) return;
    GlobalConfig.fastAckSent = true;
    /** 语音数据（TTS 失败时留空，文本仍然照常推送） */
    let audio = '';
    try {
        const [voice, error] = tts(text);
        if (!error && voice) audio = voice;
    }
    catch (error) {
        console.error('[对话者] 快速应答 TTS 失败，仅推送文本:', error);
    }
    try {
        pushContext('text', text, audio, GlobalConfig.turnFromQQ);
        console.log(`[对话者] 已发送高耗时任务快速应答: ${text}`);
    }
    catch (error) {
        // 快速应答失败不影响主任务；标记保留，避免同一轮反复重试刷屏
        console.error('[对话者] 快速应答推送失败:', error);
    }
}

/**
 * 把任意错误值转成可读文本
 *
 * 为什么要专门处理：`syncFetch` 失败时抛出的是 **Go 侧 error**（goja 导出为只有内部字段的
 * 对象），`console.error('...', error)` 打出来就是 `{}`，日志里只留下一句
 * 「请求处理错误: {}」——真实原因（模型服务崩了 / 连接被拒）无从查起。
 */
function describeError(error: unknown): string {
    if (!error) return '未知错误';
    if (typeof error === 'string') return error;
    const anyError = error as { message?: string; stack?: string };
    if (typeof anyError.message === 'string' && anyError.message) return anyError.message;
    const text = String(error);
    return text && text !== '[object Object]' ? text : '模型调用失败（宿主未提供错误详情）';
}

/** 通知宿主「本轮模型调用成功」——用于清零宿主的连续失败计数（宿主未注册时静默跳过） */
function notifyAgentSuccess(): void {
    try {
        if (typeof notifyAgentIdle === 'function') notifyAgentIdle();
    }
    catch {
        // 上报失败不影响主流程
    }
}

/** 通知宿主「本轮应答失败」：连续失败达到阈值时宿主会自动重载引擎 */
function reportAgentFailure(reason: string): void {
    try {
        if (typeof notifyAgentFailure === 'function') notifyAgentFailure(reason);
    }
    catch {
        // 上报失败不影响主流程
    }
}

/** 聊天对话角色 */
export class DialogueRole extends ModelBuilder {
    /** 智能体角色标签（TUI 实况屏显示） */
    protected roleTag: string = "对话者";

    /** 生成聊天对话（一次应答周期的入口） */
    public async generateDialogue(cache: ChatCache): Promise<void> {
        /**
         * 每次「新的应答周期」都重置工具轮次计数。
         *
         * 上限是防御模型在工具间自循环的护栏，不是对本会话的永久封禁。此前只在上下文为空时
         * 重置（`messages.length === 0`），于是一旦触顶，计数器就永远停在 5：之后每一次工具调用
         * 都会在真正执行前被判定"已达上限"并直接丢掉，表现为助手从此再也不会用工具、也不再推进任务。
         */
        this.toolRounds = 0;
        // 每轮应答都从"没有失败"开始：这一格只描述本轮，不继承上一轮的失败
        GlobalConfig.lastFailure = '';
        await this.continueDialogue(cache);
    }

    /**
     * 应答周期内部的一步：请求模型 → 若有工具调用则执行并递归下一步。
     *
     * 拆成独立方法是为了把「重置计数」与「同周期递归」分开：递归调用不能重置计数，
     * 否则 MAX_TOOL_ROUNDS 形同虚设。
     */
    private async continueDialogue(cache: ChatCache): Promise<void> {
        try {
            // 对消息中的图片文件进行压缩与解析处理
            await LiteImageFile();
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
                    // 这批调用没有执行，必须逐个回填 tool 结果：assistant 的 tool_calls 若没有配对的
                    // tool 消息，上下文就是不配对的（模型会以为工具还在跑，或下一轮直接复读这次调用）
                    this.rejectPendingToolCalls(cache);
                    // 摘掉工具再要一次终稿：把已经画完/查完的结论整理成话说完整，而不是停在半句
                    this.finalizeWithoutTools();
                    return;
                }
                /** 处理工具调用 */
                const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
                // 如果有处理过的工具调用,重新发送请求（包含工具调用结果）
                if (hasProcessedToolCalls) {
                    this.toolRounds++;
                    return await this.continueDialogue(cache);
                }
            }
            // 在历史上下文中添加模型响应
            this.writeContext(response.body.choices?.[0]?.message);
            // 本轮确实打通了模型：清零宿主的连续失败计数
            notifyAgentSuccess();
        }
        catch (error) {
            // 记下可读原因：日志里长期只有一句「请求处理错误: {}」，无法判断是模型崩了还是连接被拒
            const reason = describeError(error);
            console.error('请求处理错误:', reason);
            // 交给外层循环：它据此上报宿主（连续失败达阈值 → 自动重载引擎），
            // 并保证 GlobalConfig.reasoningInProgress 在 finally 里复位
            GlobalConfig.lastFailure = reason;
            reportAgentFailure(reason);
        }
        // 更新消息内容
        GlobalConfig.finalResponse = cache.descriptionContent;
    }

    /**
     * 回填「未执行」的工具结果，保证每个 tool_call_id 都有配对消息
     *
     * 为什么必须回填：OpenAI 风格的函数调用要求 assistant.tool_calls 与后续 role=tool
     * 一一配对。缺了配对，下一次请求的上下文就是坏的——llama.cpp 的聊天模板会把整段
     * 工具调用当成"尚无结果"，模型只能重复调用或凭空编造结果。
     */
    private rejectPendingToolCalls(cache: ChatCache): void {
        const reason = `本轮工具调用已达上限（${MAX_TOOL_ROUNDS} 次），该调用未执行`;
        for (const toolCall of cache.toolCalls) {
            this.messages.push({ role: 'tool', content: reason, tool_call_id: toolCall.id });
        }
        cache.toolCalls = [];
    }

    /**
     * 终稿：不带工具再要一次回答
     *
     * 触顶时模型往往只吐出了工具调用、没有正文；直接把空串当最终应答推给前端，用户看到的就是
     * 「一边说进行中、一边什么都没发生」。摘掉工具再要一次，它能给出的就只剩文本。
     */
    private finalizeWithoutTools(): void {
        try {
            const finalResponse = this.run(this.ragMessages, []);
            const content = finalResponse.body?.choices?.[0]?.message?.content;
            if (content) {
                GlobalConfig.finalResponse = content;
                // 只回写纯文本：终稿不允许再挂未执行的 tool_calls
                this.writeContext({ role: 'assistant', content });
                return;
            }
        }
        catch (error) {
            console.error('[对话者] 终稿生成失败:', error);
        }
        // 终稿也没拿到文本：给一句自然的话收尾，不让前端留空（也不暴露工具机制）
        GlobalConfig.finalResponse = '这轮步骤有点多了，我先停在这里，你可以接着往下说。';
    }
    /**
     * 多模态图片窗口上限：只保留最近这么多张真实图片，更早的降级为「句柄 + 描述」占位文本
     *
     * 这就是「可见性」的唯一表达方式 —— 近期图片（最新 20 张）以「图片 + [@I.00x] 旁注」
     * 形态可见，远期图片降级为「[@I.00x]（描述）」文本。模型看得到图片就看得见。
     */
    private readonly IMAGE_WINDOW = IMAGE_WINDOW_SIZE;
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

    /**
     * 将单条消息中的图片降级为占位文本：图片项 → 一条「[@I.00x]（描述）」文本
     *
     * 句柄直接从图片项自身的视觉读取 URL（tag 参数）解析 —— 句柄随图走，
     * 无需任何相邻文本配对，连续多图也不会错位。
     */
    private demoteMessageImages(message: PostMessage): void {
        if (typeof message.content === 'string') return;
        const items = message.content as MessageContent[];
        const rebuilt: MessageContent[] = [];
        for (const item of items) {
            const content: any = item;
            if (content.type !== 'image_url') {
                rebuilt.push(item);
                continue;
            }
            // 相邻排布已废除：句柄从 URL 自带的 tag 参数解析
            const tag = handleFromVisionURL(content.image_url?.url);
            rebuilt.push({ type: 'text', text: tag
                ? composeImagePlaceholder(tag, this.describeImage(tag, content.image_url?.url || ''))
                : '（一张未能登记句柄的图片，已超出窗口）' });
        }
        message.content = rebuilt;
    }

    /**
     * 获取图片条目描述（占位文本 [@I.00x]（描述）中括号内的内容来源）：
     * 资源目录摘要命中直接返回；未命中时用视觉模型对源图生成一次并回写摘要。
     * 返回值为已规整文本（折叠空白、剔除括号类结构字符、20 字上限），
     * 保证拼接进占位后不破坏句柄形态、不残留多余文本。
     */
    private describeImage(tag: string, source: string): string {
        const entry = getResourceEntry(tag);
        if (entry?.summary) return sanitizeImageDesc(entry.summary, 20);
        const generated = this.generateImageDesc(source, tag);
        if (generated && entry) setResourceSummary(entry.id, generated);
        return generated;
    }

    /**
     * 用视觉模型为图片生成一次性描述并回写资源目录
     *
     * @param source 图片来源（消息内现存的压缩图片数据）
     * @param tag 规范句柄（@I.00x）
     * @returns 规整后的描述（失败返回空串）
     */
    private generateImageDesc(source: string, tag: string): string {
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
            return sanitizeImageDesc(response.body?.choices?.[0]?.message?.content || '', 20);
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
            // 高耗时任务：在真正执行前先给用户一条快速应答（一轮仅一次）
            maybeSendFastAck(functionName, functionArgs);
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
                // 与用户图片走同一套入队赋签机制（入库得 [@I.00x] 旁注），
                // 一样参与窗口降级与按句柄引用
                if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
                    /** 工具图片消息 */
                    const message: PostMessage = {
                        role: "user",
                        content: [
                            { type: "text", text: `【工具返回的图片】${textContent}` },
                            { type: "image_url", image_url: { url: base64Image } }
                        ]
                    };
                    attachImageTags(message);
                    this.messages.push(message);
                    console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
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