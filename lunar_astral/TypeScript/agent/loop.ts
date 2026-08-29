import { GlobalConfig, ChatCache, processUnreadFiles, checkDueItems, SCHEDULE_TRIGGER_PREFIX, parseContent, PostMessageRole, MessageContent } from '../index';
import { descriptionRole, learnerRole, painterRole, musicianRole, dialogueRole, viewerRole, actorRole, randomDefaultMessage } from './roles/roles';
import { batchProcessVideoFiles } from './capabilities/media';
import { syncLTPXRemoteStatus } from './capabilities/ltpx';
import { queryEmotionSticker, memorizeUnreadRecords } from './capabilities/memory';

/** 创建聊天消息 */
async function createChatMessage(): Promise<string> {
    /** 初始化聊天缓存 */
    const cache: ChatCache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
    // 发送请求并获取响应
    await dialogueRole.generateDialogue(cache);
    // 返回最终应答
    return GlobalConfig.finalResponse;
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
        const messageLength = GlobalConfig.unreadContext.length + GlobalConfig.unreadVideoUrl.length;
        // 如果消息长度为0，跳过当前循环
        if (messageLength === 0) {
            // 检查计划表到期项，将到期计划内容写入上下文
            checkDueItems().forEach(item => GlobalConfig.unreadContext.push({ role: 'user', content: `${SCHEDULE_TRIGGER_PREFIX} 预约时间已到，请执行以下计划：${item.content}` }));
            // 标记为思考完成
            GlobalConfig.reasoningInProgress = false;
            // 进入下一次循环
            return;
        }
        // 同步琉璃（远程 LTPX）状态：在线则注入工具链，离线则移除
        syncLTPXRemoteStatus();
        // 批量处理视频文件
        await batchProcessVideoFiles();
        // 阅读者智能体：处理文件导入块与引用，将结果置换到未读消息
        await processUnreadFiles();
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
            // 调用行动者推理动作
            await actorRole.createCreativeWork(actionBlocks.join('|'));
            // 从记忆库匹配表情包并推送图片数据
            pushImage([await queryEmotionSticker(validMessage)], true);
        }
        // 未解析出行动区内容，且正文长度小于等于36字时，按概率基于正文推理表情包
        else if (validMessage.length <= 36 && Math.random() < 0.15) {
            pushImage([await queryEmotionSticker(validMessage)], true);
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
        // 消息缓冲池非空时，触发信息记忆流程：逐个写入记忆库后清空
        if (GlobalConfig.unreadRecords.length >= 1) memorizeUnreadRecords();
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
        else console.log('收到图片: ' + message.image_url?.url?.substring(0, 50));
    }
}

/** 写入视频文件 */
function writeVideoUrl(videoUrl: string) {
    console.log('收到视频: ' + videoUrl);
    // 从外部写入视频文件
    GlobalConfig.unreadVideoUrl.push(videoUrl);
}

/** 错误累积达阈值后重置智能体状态 */
function resetAgentState(): void {
    // 清空全部子智能体的messages
    descriptionRole.coverContext([]);
    dialogueRole.coverContext([]);
    learnerRole.messages = [];
    painterRole.coverContext([]);
    musicianRole.coverContext([]);
    viewerRole.coverContext([]);
    actorRole.coverContext([]);
    // 清除主智能体的unreadContext和unreadVideoUrl
    GlobalConfig.unreadContext = [];
    GlobalConfig.unreadVideoUrl = [];
}
