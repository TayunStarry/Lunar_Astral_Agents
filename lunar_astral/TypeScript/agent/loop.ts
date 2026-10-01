import { GlobalConfig } from '../config/global';
import { ChatCache } from '../config/config';
import { PostMessage, PostMessageRole, MessageContent } from '../config/model';
import { processUnreadFiles } from './roles/reader';
import { checkDueItems } from '../tool/schedule';
import { SCHEDULE_TRIGGER_PREFIX } from '../tool/schedule-defs';
import { parseContent } from '../file/parse/interface';
import { descriptionRole, painterRole, musicianRole, dialogueRole, perceiverRole, memorizerRole, randomDefaultMessage } from './roles/roles';
import { syncLTPXRemoteStatus } from './capabilities/ltpx';
import { interactEvent } from './capabilities/ltp-event';
import { queryEmotionSticker, extractTextFromMessage, syncActionsToMemory, queryBestAction } from './capabilities/memory';
import { RandomFloat } from '../tool/math';
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

/** ASR 卸载标记：本轮消息含语音时重置，无语音的轮次发起卸载后置位，避免重复请求路由服务器 */
let asrUnloaded = false;

/** ASR 卸载守卫：检查本轮是否有语音消息（包含音频文件或 URL），无语音的轮次发起卸载后置位，避免重复请求路由服务器 */
function guardASRUnload(): void {
    // 检查本轮是否有语音消息（包含音频文件或 URL）
    asrUnloaded = GlobalConfig.unreadContext.some(
        message => {
            const items = Array.isArray(message.content) ? message.content : [];
            return items.some(item => item.type === 'input_audio' || item.type === 'audio_url');
        }
    );
    // 如果有语音消息，或当前轮次无消息，不卸载模型
    if (asrUnloaded || GlobalConfig.unreadContext.length === 0) return;
    /** 发起卸载请求，由 Go 层处理模型卸载（如模型未加载则静默跳过） */
    const [_, error] = syncFetch(
        {
            url: url()[0] + '/vram/unload',
            execute: {
                method: 'POST',
                crossDomain: true,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ model: GlobalConfig.AsrName }),
            },
        }
    );
    // 卸载失败静默跳过，不影响思考链流程
    if (error) console.error('ASR 卸载执行失败:', error.message);
}

/** 周期性显存守卫：每完成 N 次应答检查一次可用显存，不足时卸载本地模型，释放随上下文增长的 KV 缓存占用（下次应答会自动重载模型） */
function guardChatVRAM(): void {
    /** 守卫开关（默认开启） */
    const enabled = GlobalConfig.customConfig?.chat?.chat_vram_guard ?? true;
    if (!enabled) return;
    /** 检查间隔：每完成 N 次应答触发一次（默认 16） */
    const interval = GlobalConfig.customConfig?.chat?.chat_vram_guard_interval ?? 16;
    // 非法间隔视为关闭守卫
    if (!Number.isFinite(interval) || interval <= 0) return;
    /** 可用显存阈值（MiB，默认 1024） */
    const thresholdMiB = GlobalConfig.customConfig?.chat?.chat_vram_guard_mib ?? 1024;
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
    }
    else if (GlobalConfig.debugMode) {
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
    const userMessages = GlobalConfig.unreadContext.filter(message => message.role === 'user').map(message => extractTextFromMessage(message).trim()).filter(text => text.length > 0);
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

/** 默认后备动作列表（引擎未就绪时使用） */
const FALLBACK_ACTIONS: string[] = ['荡秋千', '翻花绳'];

/** 从前端渲染引擎获取当前可用动作名称列表 */
function getAvailableActionNames(): string[] {
    try {
        const raw = getAvailableActions();
        if (!raw || raw === '{}') return FALLBACK_ACTIONS;
        const parsed: { actions?: Array<{ name: string }> } = JSON.parse(raw);
        if (parsed.actions && Array.isArray(parsed.actions) && parsed.actions.length > 0) {
            return parsed.actions.map(item => item.name);
        }
    }
    catch {
        // 解析失败，使用后备列表
    }
    return FALLBACK_ACTIONS;
}

/** 执行匹配到的动作 */
async function performMatchedAction(query: string): Promise<void> {
    /** 获取当前可用动作名称列表 */
    const actionNames = getAvailableActionNames();
    // 判定动作组是否存入记忆库
    if (!syncActionsToMemory(actionNames)) return;
    /** 文本与动作库执行一次匹配，取最高匹配度结果 */
    const bestAction = queryBestAction(query);
    // 如果没有匹配到动作，直接返回
    if (!bestAction) return;
    /** 获取当前代理位置 */
    const pos = getAgentPosition();
    // 随机移动代理位置，避免重复执行相同动作
    sendToEngine('movement', JSON.stringify({ position: { x: pos.x + RandomFloat(-0.5, 0.5), y: pos.y, z: pos.z + RandomFloat(-0.5, 0.5), }, resumeTracking: true, }));
    // 执行匹配到的动作
    sendToEngine('action', JSON.stringify({ action: bestAction }));
    // 行动摘要推送到前端「行动」标签
    pushContext('action', `月华${bestAction}了`, '');
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
        pullContext().forEach(message => writeMessage(message.role, message.content))
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
        // 如果解析出行动区内容：动作组入库 → 文本与动作库匹配 → 执行最高匹配度动作（附带小幅随机位移）
        if (actionBlocks.length) {
            // 事件 -> 做出行动前：推送行动块，插件可经 return.actions 改写匹配参考文本
            const feedback: { actions?: string[] } = interactEvent('take_action_before', { actions: actionBlocks }).return;
            const actBlocks = (feedback && Array.isArray(feedback.actions)) ? feedback.actions : actionBlocks;
            // 以行动区内容为匹配参考文本（为空时回退正文），与动作库匹配后执行最高匹配度动作
            await performMatchedAction(actBlocks.join(' ').trim() || validMessage);
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

/** 写入消息（content 为文本、多模态内容数组或宿主下发的媒体URL内容项） */
function writeMessage(role: PostMessageRole, content: any) {
    // 宿主统一队列下发的媒体URL内容项为裸对象形态（{"type":"video_url"/"audio_url", ...}）：
    // 包装为单内容项数组，与多模态消息同构（LiteImageFile 可迭代原位置换）
    if (content && typeof content === 'object' && !Array.isArray(content) && (content.type === 'video_url' || content.type === 'audio_url')) content = [content];
    // 写入统一未读队列
    GlobalConfig.unreadContext.push({ role, content });
    // 字符串消息转换为文本内容项后统一打印
    const items: MessageContent[] = typeof content === 'string' ? [{ type: 'text', text: content }] : Array.isArray(content) ? content : [];
    for (const message of items) {
        if (message.type === 'text') console.log('收到文本: ' + message.text);
        else if (message.type === 'image_url') console.log('收到图片: ' + message.image_url?.url?.substring(0, 50));
        else if (message.type === 'input_audio') console.log('收到音频: ' + message.input_audio?.data?.substring(0, 30));
        else if (message.type === 'video_url') console.log('收到视频: ' + message.video_url?.url);
        else if (message.type === 'audio_url') console.log('收到音频: ' + message.audio_url?.url);
    }
}

/** 错误累积达阈值后重置智能体状态 */
function resetAgentState(): void {
    // 清空全部子智能体的messages
    descriptionRole.coverContext([]);
    dialogueRole.coverContext([]);
    painterRole.coverContext([]);
    musicianRole.coverContext([]);
    perceiverRole.coverContext([]);
    memorizerRole.coverContext([]);
    // 清空主智能体的统一未读队列
    GlobalConfig.unreadContext = [];
}
