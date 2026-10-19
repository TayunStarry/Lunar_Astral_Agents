import { GlobalConfig } from '../config/global';
import { ChatCache } from '../config/config';
import { ToolCall } from '../config/tool';
import { ModelResponseBody, PostMessage } from '../config/model';
import { ModelBuilder } from '../agent/builder';
import { LiteImageFile } from '../capabilities/media';
import { IMAGE_WINDOW_SIZE, sanitizeImageDesc } from '../capabilities/image-tag';
import { getResourceEntry, setResourceSummary } from '../capabilities/resource';
import { getCrystalRemoteTools } from '../capabilities/ltpx';
import { agentControlTools } from '../tools/agent-control';
import { imageRecallTools } from '../tools/image-recall';
import { memoryTools } from '../tools/memory';
import { resourceTools } from '../tools/resource';
import { scheduleTools } from '../tools/schedule/defs';
import { screenshotTools } from '../tools/screenshot';
import { describeError, notifyAgentSuccess, reportAgentFailure } from './dialogue/errors';
import { applyModelResponse } from './dialogue/response';
import { demoteOverflowImagesIn } from './dialogue/history';
import { executeToolCalls } from './dialogue/tool-calls';

/** 对话者固有工具（除琉璃聚合工具外全部固定不变，定义随本模块静态聚合） */
const dialogueBaseTools: ToolCall[] = [...agentControlTools, ...imageRecallTools, ...memoryTools, ...resourceTools, ...scheduleTools, ...screenshotTools];

/** 单次应答允许的工具调用轮次上限（防御模型在工具调用间自循环） */
const MAX_TOOL_ROUNDS = 5;

/** 「未调用工具却宣称已完成」的判定：打开/启动/切换这类会改变用户可见状态的动作 */
const ACTION_CLAIM_PATTERN = /(已经|已|这就|马上|现在)[^，。！？\n]{0,12}(打开|启动|运行|调出|叫出|切换|准备好|准备完毕)/;

/** 动作声明纠偏提示：作为一次性追加上下文重新请求，不写入历史 */
const ACTION_CLAIM_GUARD = '【系统校验】你刚才声称已经完成了打开/启动类操作，但本轮没有调用任何工具，用户看不到任何变化。请直接调用 use_the_program 执行（只打开应用只填 tool），或如实说明没有完成，不要只口头宣称。';

/** 姐妹基础外观（直接复用绘制者已解析的角色提示词，避免重复解析 Markdown） */
export interface SisterAppearances {
    /** 月华的基础外观 */
    lunar: string;
    /** 琉璃的基础外观 */
    crystal: string;
}

/**
 * 组装对话者系统提示词：把绘制者已解析的姐妹基础外观填入占位符
 *
 * 外观数据来自 prompts/painterCharacters.md 的「基础外观」节，
 * 与绘制者共用同一份提示词，改外观只需维护该文件。
 *
 * @param appearances 姐妹基础外观
 * @returns 补全后的系统提示词
 */
function composeDialoguePrompt(appearances: SisterAppearances): string {
    return fileView('prompts/dialogueRole.md')[0]
        .split('{lunar-appearance}').join(appearances.lunar)
        .split('{crystal-appearance}').join(appearances.crystal);
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
        // 每个应答周期最多纠偏一次「未调用工具却宣称已完成」
        this.actionClaimGuarded = false;
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
    private async continueDialogue(cache: ChatCache, guardNote: PostMessage | null = null): Promise<void> {
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
            const response = this.run(guardNote ? [...this.ragMessages, guardNote] : this.ragMessages, [...dialogueBaseTools, ...getCrystalRemoteTools()]);
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
            // 本轮尚未执行任何工具，却宣称已完成打开/启动类动作：丢弃这句话并纠偏重答一次。
            // 历史中存在同类「已打开」说法，模型会模仿；仅靠提示词无法保证，故在生成后确定性拦截。
            if (!this.actionClaimGuarded
                && cache.toolCalls.length === 0
                && this.toolRounds === 0
                && ACTION_CLAIM_PATTERN.test(cache.descriptionContent)) {
                this.actionClaimGuarded = true;
                console.warn('[对话者] 未调用工具却宣称已完成动作，触发纠偏重答');
                // 这句话不写入上下文：否则会污染历史，下一轮继续复读同样的假宣称
                cache.descriptionContent = '';
                cache.thinkingContent = '';
                return await this.continueDialogue(cache, { role: 'user', content: ACTION_CLAIM_GUARD });
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
    /** 本轮是否已触发过动作声明纠偏（每周期最多一次，防重答循环） */
    private actionClaimGuarded = false;

    /** 格式化历史消息：窗内图片超限时增量降级最老的图片，末尾非用户消息时补续写指令 */
    public formatHistoricalMessages() {
        // 如果消息数组为空,则不处理
        if (this.messages.length === 0) return;
        // 增量降级：只摘除超出窗口的最老图片（避免一次性改写整个历史，保持前缀缓存局部性）
        this.demoteOverflowImages();
        // 如果处理后消息数组为空（极端情况），跳过续写
        if (this.messages.length === 0) return;
        /** 最后一条消息的角色 */
        const lastRole = this.messages.slice(-1)[0].role;
        // 用户消息与工具结果都不需要续写指令：
        //   - 用户消息本身就是本轮输入；
        //   - 工具结果是「同一应答周期」的中间态，模型应直接基于结果收尾；
        //     若在这里补「继续」，会被模型当成新的创作指令（实测：一次画图请求连画 4 张）。
        if (lastRole === 'user' || lastRole === 'tool') return;
        // 如果不是用户消息就补充一条默认用户消息
        this.writeContext({ role: 'user', content: "继续" });
    }

    /** 增量降级：窗内多模态图片超过上限时，从最老的消息开始摘图留签 */
    private demoteOverflowImages(): void {
        demoteOverflowImagesIn(this.messages, this.IMAGE_WINDOW, (tag, source) => this.describeImage(tag, source));
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
        applyModelResponse(message, cache);
    }

    /** 批量执行工具调用 */
    protected async batchExecutionToolCall(state: ChatCache): Promise<boolean> {
        return executeToolCalls(this.messages, state);
    }

    /** 构造函数 */
    public constructor(private descriptionRole: ModelBuilder, appearances: SisterAppearances) {
        super(composeDialoguePrompt(appearances));
    }
}
