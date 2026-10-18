import { GlobalConfig } from '../config/global';
import { ChatCache } from '../config/config';
import { PostMessage } from '../config/model';
import { checkDueItems } from '../tools/schedule/index';
import { SCHEDULE_TRIGGER_PREFIX } from '../tools/schedule/defs';
import { parseContent, cleanTextForTTS } from '../text/index';
import { dialogueRole, randomDefaultMessage } from '../roles/index';
import { syncLTPXRemoteStatus } from '../capabilities/ltpx';
import { interactEvent } from '../capabilities/ltp-event';
import { queryEmotionSticker } from '../capabilities/memory';
import { persistConversation, pushAssistantContext, pushAssistantImage } from '../io/conversation';
import { guardASRUnload } from './turn/guards';
import { updatePreviousMemories } from './turn/memory';
import { performMatchedAction } from './turn/actions';
import { resetAgentState, writeMessage } from './turn/messages';

/** 创建聊天消息 */
async function createChatMessage(): Promise<string> {
    /** 初始化聊天缓存 */
    const cache: ChatCache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
    // 发送请求并获取响应
    await dialogueRole.generateDialogue(cache);
    // 返回最终应答
    return GlobalConfig.finalResponse;
}

/** 应答失败时对用户说的话 */
const FAILURE_REPLY = '刚才有点走神，你再说一遍好吗？';

/** 思考循环事件 */
export async function thoughtLoopTickEvent(): Promise<void> {
    // 如果正在思考中，直接返回
    if (GlobalConfig.reasoningInProgress) return;
    // 新一轮思考链：快速应答与来源标记都从零开始
    GlobalConfig.fastAckSent = false;
    GlobalConfig.turnFromQQ = false;
    // 思考循环开始
    try {
        // 标记为思考中
        GlobalConfig.reasoningInProgress = true;
        // 拉取外部消息；QQ 适配器消息带 source=qq，用于快速应答的应答份额判定
        const incoming = pullContext();
        for (const message of incoming) {
            if ((message as { source?: string }).source === 'qq') GlobalConfig.turnFromQQ = true;
            writeMessage(message.role, message.content);
        }
        // ASR 资源守卫：本轮待处理消息不含语音时卸载 ASR 模型（拉取后判定，保证刚到达的语音不被误卸）
        guardASRUnload();
        /** 消息长度（统一时序队列：文本与媒体URL混排，一个队列即为全部未读） */
        const messageLength = GlobalConfig.unreadContext.length;
        // 如果消息长度为0，跳过当前循环
        if (messageLength === 0) {
            // 检查计划表到期项，将到期计划内容写入上下文
            checkDueItems().forEach(
                item => {
                    // 事件 -> 执行计划前：推送到期计划，插件可经 return.plan 改写计划内容
                    const feedback: { plan?: string } = interactEvent('execution_schedule_before', { plan: item.content }).return;
                    const content = (feedback && typeof feedback.plan === 'string') ? feedback.plan : item.content;
                    GlobalConfig.unreadContext.push({ role: 'user', content: `${SCHEDULE_TRIGGER_PREFIX} 预约时间已到，请执行以下计划：${content}` })
                }
            );
            // 标记为思考完成
            GlobalConfig.reasoningInProgress = false;
            // 进入下一次循环
            return;
        }
        // 更新远期记忆摘要
        updatePreviousMemories();
        // 拉取琉璃工具链
        syncLTPXRemoteStatus();
        // 事件 -> 收到消息前：把待处理的消息上下文（含内嵌的视频/音频URL内容项）推送到琉璃，插件可经 return.messages 改写后再消费
        const feedback: { messages?: PostMessage[] } = interactEvent('message_received_before', { messages: GlobalConfig.unreadContext }).return;
        // 如果插件返回了新的消息上下文，更新全局上下文
        if (feedback && Array.isArray(feedback.messages)) GlobalConfig.unreadContext = feedback.messages;
        // 创建消息（对话者作为主智能体，消费上下文并生成最终应答）
        await createChatMessage();
        // 如果消息响应为空：先分清「模型说没话说」与「模型根本没接通」再决定怎么回应
        if (!GlobalConfig.finalResponse.trim().length) {
            /** 本轮失败原因（空串表示模型正常答复了空内容） */
            const failure = GlobalConfig.lastFailure;
            if (failure) {
                // 推送失败回复 （不参与语音合成）
                pushAssistantContext('text', FAILURE_REPLY, '');
                // 连续失败由宿主计数，达阈值会自动重载引擎；这里只保证本轮不把标记锁死
                console.warn('[思考链] 本轮应答失败:', failure);
            }
            else pushAssistantContext('text', randomDefaultMessage(), tts(randomDefaultMessage())[0])
            return
        };
        /** 解析原始文本：拆分思考区、代码块、行动区，并得到完整正文与切片 */
        const { thinkingBlocks, codeBlocks, actionBlocks, textChunks, displayText } = parseContent(GlobalConfig.finalResponse);
        /** 清洗并合并后的正文消息（用于行动 / 表情匹配） */
        const validMessage = textChunks.map(chunk => chunk.display).join('\n').trim();
        // 如果正文为空，抛出异常
        if (!validMessage.length) throw new Error('清洗后的文本为空');
        /** 整段正文（未切片，保留原始排版） */
        const replyText = displayText.trim() || validMessage;
        // 如果解析出行动区内容：动作组入库 → 文本与动作库匹配 → 执行最高匹配度动作（附带小幅随机位移）
        if (actionBlocks.length) {
            // 事件 -> 做出行动前：推送行动块，插件可经 return.actions 改写匹配参考文本
            const feedback: { actions?: string[] } = interactEvent('take_action_before', { actions: actionBlocks }).return;
            const actBlocks = (feedback && Array.isArray(feedback.actions)) ? feedback.actions : actionBlocks;
            // 以行动区内容为匹配参考文本（为空时回退正文），与动作库匹配后执行最高匹配度动作
            await performMatchedAction(actBlocks.join(' ').trim() || validMessage);
            // 从记忆库匹配表情包并推送图片数据
            pushAssistantImage([await queryEmotionSticker(validMessage)], true);
        }
        // 未解析出行动区内容，且正文长度小于等于36字时，按概率基于正文推理表情包
        else if (validMessage.length <= 36 && Math.random() < 0.15) {
            // 事件 -> 表达情感前：推送正文，插件可经 return.text 改写表情参考文本
            const feedback: { text?: string } = interactEvent('express_emotions_before', { text: validMessage }).return;
            const emoText = (feedback && typeof feedback.text === 'string') ? feedback.text : validMessage;
            // 从记忆库匹配表情包并推送图片数据
            pushAssistantImage([await queryEmotionSticker(emoText)], true);
        }
        // 第一步：按顺序逐一发送思考区内容（不参与语音合成）
        thinkingBlocks.forEach(thinking => pushAssistantContext('text', thinking, ''))
        // 第二步：按顺序逐一发送代码块内容（不参与语音合成）
        codeBlocks.forEach(code => pushAssistantContext('text', code, ''))
        /** 语音合成结果 */
        let replyAudio = '';
        /** 整段正文一次合成语音 */
        const [replyAudioData, replyErr] = tts(cleanTextForTTS(replyText));
        // 如果语音合成成功，将结果赋值给replyAudio
        if (!replyErr && replyAudioData) replyAudio = replyAudioData;
        // 推送整段正文（包含显示内容和语音数据）
        pushAssistantContext('text', replyText, replyAudio);
        // 思考链业务结束：执行周期性显存守卫（应答计数达到间隔时检查显存并按需卸载模型）
        // guardChatVRAM();
    }
    catch (error) {
        /** 获取提示音数据 */
        const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
        // 如果读取提示音失败，打印错误信息
        if (readErr) console.error('读取提示音失败:', readErr);
        // 打印错误信息（Go 侧 error 需要先转成可读文本，否则只留一句 [object Object]）
        console.error('思考链异常:', `${(error as Error)?.message ?? error}`, ' || ', (error as Error)?.stack ?? '');
        // 推送兜底消息
        pushAssistantContext('text', randomDefaultMessage(), promptSound);
        // 重置智能体状态
        resetAgentState();
    }
    finally {
        // 思考中标记必须在**任何**退出路径上复位。
        GlobalConfig.reasoningInProgress = false;
        // 本轮结束后把近期记忆同步到缓存文件（推送时已写过一次，这里兜底空应答/异常路径）
        persistConversation();
    }
}
