import { GlobalConfig } from '../config/global';
import { ChatCache } from '../config/config';
import { PostMessage, PostMessageRole, MessageContent } from '../config/model';
import { processUnreadFiles } from './roles/reader';
import { checkDueItems } from '../tool/schedule';
import { SCHEDULE_TRIGGER_PREFIX } from '../tool/schedule-defs';
import { parseContent } from '../file/parse/interface';
import { descriptionRole, painterRole, musicianRole, dialogueRole, perceiverRole, actorRole, memorizerRole, randomDefaultMessage } from './roles/roles';
import { batchProcessVideoFiles, batchProcessAudioFiles } from './capabilities/media';
import { syncLTPXRemoteStatus } from './capabilities/ltpx';
import { interactEvent } from './capabilities/ltp-event';
import { queryEmotionSticker, extractTextFromMessage } from './capabilities/memory';
import { processDecrees } from './decrees';

/** 创建聊天消息 */
async function createChatMessage(): Promise<string> {
    /** 初始化聊天缓存 */
    const cache: ChatCache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
    // 发送请求并获取响应
    await dialogueRole.generateDialogue(cache);
    // 返回最终应答
    return GlobalConfig.finalResponse;
}

/** 已完成应答计数（用于周期性显存守卫） */
let completedResponseCount = 0;

/** 周期性显存守卫：每完成 N 次应答检查一次可用显存，不足时卸载本地模型，释放随上下文增长的 KV 缓存占用（下次应答会自动重载模型） */
function guardChatVRAM(): void {
    /** 守卫开关（默认开启） */
    const enabled = GlobalConfig.customConfig?.server?.chat_vram_guard ?? true;
    if (!enabled) return;
    /** 检查间隔：每完成 N 次应答触发一次（默认 16） */
    const interval = GlobalConfig.customConfig?.server?.chat_vram_guard_interval ?? 16;
    // 非法间隔视为关闭守卫
    if (!Number.isFinite(interval) || interval <= 0) return;
    /** 可用显存阈值（MiB，默认 1024） */
    const thresholdMiB = GlobalConfig.customConfig?.server?.chat_vram_guard_mib ?? 1024;
    // 应答计数，未达到间隔时跳过
    completedResponseCount++;
    if (completedResponseCount % interval !== 0) return;
    // 调用显存守卫端点，由 Go 层检测显存并按需卸载
    const [result, error] = syncFetch({
        url: url()[0] + '/vram/guard',
        execute: {
            method: 'POST',
            crossDomain: true,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ threshold_mib: thresholdMiB }),
        },
    });
    // 守卫失败静默跳过，不影响应答流程
    if (error) {
        console.error('显存守卫执行失败:', error.message);
        return;
    }
    if (result?.body?.triggered) {
        console.log(`显存守卫: 可用显存 ${result.body.free_mib} MiB 低于阈值 ${result.body.threshold_mib} MiB, 已卸载模型: ${(result.body.unloaded || []).join(', ')}`);
    } else if (GlobalConfig.debugMode) {
        console.log(`显存守卫: 可用显存 ${result?.body?.free_mib} MiB, 无需卸载`);
    }
}

/** 深度回忆意图匹配正则：未读用户消息中明确表达需要回忆、整理记忆时才触发 AI 摘要 */
const DEEP_RECALL_PATTERN = /(回忆|记忆|记得|回想|想起来)/;

/** 更新远期记忆摘要 */
function updatePreviousMemories(): void {
    // 消息缓冲池非空时，触发记忆者智能体：将缓冲消息逐个写入记忆库后清空
    if (GlobalConfig.unreadRecords.length >= 1) memorizerRole.persistUnreadRecords();
    /** 提取未读消息中的用户消息文本（作为记忆库查询条件） */
    const userMessages = GlobalConfig.unreadContext
        .filter(message => message.role === 'user')
        .map(message => extractTextFromMessage(message).trim())
        .filter(text => text.length > 0);
    /** 清理RAG消息并返回 */
    const clear = () => { dialogueRole.ragMessages = []; };
    // 如果没有用户消息，清理RAG消息并返回
    if (userMessages.length === 0) return clear();
    /** 深度回忆意图判定：命中时由 AI 整理摘要，否则默认按时间顺序拼接时间线 */
    const deepRecall = userMessages.some(text => DEEP_RECALL_PATTERN.test(text));
    /** 记忆库返回的记忆文本（AI 摘要或时间线拼接） */
    const digest = memorizerRole.queryRagDigest(userMessages, deepRecall);
    // 记忆库返回为空时，清理RAG消息并返回（跳过拼接与可能的回忆者智能体调用）
    if (!digest) return clear();
    // 将记忆文本作为一条用户消息注入 ragMessages（替换碎片式上下文）
    dialogueRole.ragMessages = [{ role: 'user', content: `【长期记忆摘要】\n${digest}` }];
}

/** 思考循环事件 */
export async function thoughtLoopTickEvent(): Promise<void> {
    // 如果正在思考中，直接返回
    if (GlobalConfig.reasoningInProgress) return;
    // 思考循环开始
    try {
        // 标记为思考中
        GlobalConfig.reasoningInProgress = true;
        // 拉取外部消息
        await pullExternalMessages();
        /** 消息长度 */
        const messageLength = GlobalConfig.unreadContext.length + GlobalConfig.unreadVideoUrl.length + GlobalConfig.unreadAudioUrl.length;
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
        // 事件 -> 收到消息前：把待处理的消息上下文推送到琉璃，插件可经 return 改写后再消费
        const feedback: { messages?: PostMessage[]; videos?: string[]; audios?: string[] } = interactEvent('message_received_before', { messages: GlobalConfig.unreadContext, videos: GlobalConfig.unreadVideoUrl, audios: GlobalConfig.unreadAudioUrl }).return;
        // 如果插件返回了新的消息上下文，更新全局上下文
        if (feedback) {
            if (Array.isArray(feedback.messages)) GlobalConfig.unreadContext = feedback.messages;
            if (Array.isArray(feedback.videos)) GlobalConfig.unreadVideoUrl = feedback.videos;
            if (Array.isArray(feedback.audios)) GlobalConfig.unreadAudioUrl = feedback.audios;
        }
        // 批量处理视频文件
        await batchProcessVideoFiles();
        // 批量处理音频文件
        await batchProcessAudioFiles();
        // 阅读者智能体：处理文件导入块与引用，将结果置换到未读消息
        await processUnreadFiles();
        // 律令指令处理：命中已定义的 <指令> 时输出默认应答（含 TTS），剔除携带律令的消息，
        // 跳过本轮 AI 应答；未读消息中的其他内容等待下个周期再解析
        if (processDecrees()) {
            GlobalConfig.reasoningInProgress = false;
            return;
        }
        // 创建消息（对话者作为主智能体，消费上下文并生成最终应答）
        await createChatMessage();
        // 如果消息响应为空，抛出异常
        if (!GlobalConfig.finalResponse.trim().length) {
            pushContext('text', randomDefaultMessage(), tts(randomDefaultMessage())[0])
            return
        };
        /** 解析原始文本：拆分思考区、代码块、行动区、正文切片（含display和tts双版本） */
        const { thinkingBlocks, codeBlocks, actionBlocks, textChunks } = parseContent(GlobalConfig.finalResponse);
        /** 清洗并合并后的正文消息 */
        const validMessage = textChunks.map(chunk => chunk.display).join('\n').trim();
        // 如果正文切片为空，抛出异常
        if (!validMessage.length) throw new Error('清洗后的文本为空');
        // 如果解析出行动区内容，分别交给行动者推理动作、记忆库匹配表情包
        if (actionBlocks.length) {
            // 事件 -> 做出行动前：推送行动块，插件可经 return.actions 改写后再交给行动者
            const feedback: { actions?: string[] } = interactEvent('take_action_before', { actions: actionBlocks }).return;
            const actBlocks = (feedback && Array.isArray(feedback.actions)) ? feedback.actions : actionBlocks;
            // 调用行动者推理动作
            await actorRole.createCreativeWork(actBlocks.join('|'));
            // 从记忆库匹配表情包并推送图片数据
            pushImage([await queryEmotionSticker(validMessage)], true);
        }
        // 未解析出行动区内容，且正文长度小于等于36字时，按概率基于正文推理表情包
        else if (validMessage.length <= 36 && Math.random() < 0.15) {
            // 事件 -> 表达情感前：推送正文，插件可经 return.text 改写表情参考文本
            const feedback: { text?: string } = interactEvent('express_emotions_before', { text: validMessage }).return;
            const emoText = (feedback && typeof feedback.text === 'string') ? feedback.text : validMessage;
            // 从记忆库匹配表情包并推送图片数据
            pushImage([await queryEmotionSticker(emoText)], true);
        }
        // 第一步：按顺序逐一发送思考区内容（不参与语音合成）
        thinkingBlocks.forEach(thinking => pushContext('text', thinking, ''))
        // 第二步：按顺序逐一发送代码块内容（不参与语音合成）
        codeBlocks.forEach(code => pushContext('text', code, ''))
        // 第三步：按顺序逐一发送正文切片，display用于显示，tts用于合成语音
        for (const chunk of textChunks) {
            /** 语音合成结果 */
            let audio = '';
            /** 语音合成 */
            const [audioData, err] = tts(chunk.tts);
            // 如果语音合成成功，将结果赋值给audio
            if (!err && audioData) audio = audioData;
            // 推送消息（包含显示内容和语音数据）
            pushContext('text', chunk.display, audio);
        }
        // 思考链业务结束：执行周期性显存守卫（应答计数达到间隔时检查显存并按需卸载模型）
        // guardChatVRAM();
    }
    catch (error) {
        /** 获取提示音数据 */
        const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
        // 如果读取提示音失败，打印错误信息
        if (readErr) console.error('读取提示音失败:', readErr);
        // 打印错误信息
        console.error((error as Error).message, ' || ', (error as Error).stack);
        // 推送兜底消息
        pushContext('text', randomDefaultMessage(), promptSound);
        // 重置智能体状态
        resetAgentState();
    }
    // 标记为思考完成
    GlobalConfig.reasoningInProgress = false;
}

/** 拉取外部消息 */
async function pullExternalMessages() {
    // 合并消息
    pullContext().forEach(message => writeMessage(message.role, message.content))
    // 合并视频URL
    pullVideoUrl().forEach(videoUrl => { writeVideoUrl(videoUrl); })
    // 合并音频URL
    pullAudioUrl().forEach(audioUrl => { writeAudioUrl(audioUrl); })
    // 等待1秒
    await new Promise(resolve => setTimeout(resolve, 1000));
}

/** 写入消息 */
function writeMessage(role: PostMessageRole, messages: Array<MessageContent>) {
    // 从外部写入消息
    GlobalConfig.unreadContext.push({ role, content: messages });
    // 如果消息是字符串，将其转换为文本消息
    if (typeof messages === 'string') messages = [{ type: 'text', text: messages }];
    // 打印文本消息
    for (const message of messages) {
        if (message.type === 'text') console.log('收到文本: ' + message.text);
        else if (message.type === 'image_url') console.log('收到图片: ' + message.image_url?.url?.substring(0, 50));
        else if (message.type === 'input_audio') console.log('收到音频: ' + message.input_audio?.data?.substring(0, 30));
    }
}

/** 写入视频文件 */
function writeVideoUrl(videoUrl: string) {
    console.log('收到视频: ' + videoUrl);
    // 从外部写入视频文件
    GlobalConfig.unreadVideoUrl.push(videoUrl);
}

/** 写入音频文件 */
function writeAudioUrl(audioUrl: string) {
    console.log('收到音频: ' + audioUrl.substring(0, 80));
    // 从外部写入音频文件
    GlobalConfig.unreadAudioUrl.push(audioUrl);
}

/** 错误累积达阈值后重置智能体状态 */
function resetAgentState(): void {
    // 清空全部子智能体的messages
    descriptionRole.coverContext([]);
    dialogueRole.coverContext([]);
    painterRole.coverContext([]);
    musicianRole.coverContext([]);
    perceiverRole.coverContext([]);
    actorRole.coverContext([]);
    memorizerRole.coverContext([]);
    // 清除主智能体的unreadContext、unreadVideoUrl和unreadAudioUrl
    GlobalConfig.unreadContext = [];
    GlobalConfig.unreadVideoUrl = [];
    GlobalConfig.unreadAudioUrl = [];
}
