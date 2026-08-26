import { match } from 'assert';
import {
    getPromptFromKnowledge,
    savePromptToKnowledge,
    fetchDocumentCallback,
    PostMessageRole,
    MessageContent,
    checkDueItems,
    parseContent,
    GlobalConfig,
    ImageContent,
    TextContent,
    PostMessage,
    ModelBuilder,
    PainterRole,
    MusicianRole,
    DialogueRole,
    LearnerRole,
    RandomFloor,
    ViewerRole,
    ChatCache,
    ActorRole,
    processUnreadFiles,
} from '../index';

/** 描述者角色(视觉内容描述) */
export const descriptionRole: ModelBuilder = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
/** 学习者角色(深度调研与信息查证) */
export const learnerRole: LearnerRole = new LearnerRole();
/** 绘制者角色(图片生成) */
export const painterRole: PainterRole = new PainterRole();
/** 演奏者角色(演奏音乐) */
export const musicianRole: MusicianRole = new MusicianRole();
/** 行动者角色(3D动画/位移/空间感知) */
export const actorRole: ActorRole = new ActorRole();
/** 对话者角色(与用户交互) */
const dialogueRole: DialogueRole = new DialogueRole();
/** 观影者角色(视频观看) */
const viewerRole: ViewerRole = new ViewerRole();
/** 随机回答 */
export function randomDefaultMessage(): string {
    return ['月华在哦', '怎么了吗?', '详细说说?'][RandomFloor(0, 2)];
}
/** 表情包记忆库集合名（image 类型集合，由 memory.store 前端管理） */
const STICKER_COLLECTION = 'stickers';
/** 表情包集合是否已确认就绪（避免重复初始化） */
let stickerCollectionReady = false;
/** 处理视频文件（观影者智能体） */
async function analysisVideoFile(videoUrl: string, userNeeds: string): Promise<void> {
    // 缓存检查：如果已处理过该视频，直接返回缓存结果
    const cachedPrompt = getPromptFromKnowledge(videoUrl);
    if (cachedPrompt) {
        GlobalConfig.unreadContext.push({ role: 'user', content: cachedPrompt });
        console.log('[观影者] 命中视频缓存，直接返回');
        return;
    }
    // 第一步：提取关键帧
    console.log('[观影者] 开始提取视频关键帧...');
    const [images, error] = keyframe(videoUrl, './cache');
    if (images.length === 0 || error) {
        console.error('[观影者] 关键帧提取失败:', error);
        throw new Error('提取关键帧失败');
    }
    console.log(`[观影者] 关键帧提取完成，共 ${images.length} 帧`);
    // 第二步：将关键帧转换为观影者所需格式
    /** 关键帧数据数组 */
    const keyframes = images.map(frame => ({ data: frame.data, timestamp: frame.timestamp || '' }));
    // 第三步：调用观影者智能体观看视频
    console.log('[观影者] 开始观看视频...');
    const videoSummary = await viewerRole.watchVideo(keyframes);
    console.log('[观影者] 视频观看完成');
    // 第四步：将观后感添加到未读上下文
    if (videoSummary && videoSummary.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: videoSummary });
    }
    else GlobalConfig.unreadContext.push({ role: 'user', content: randomDefaultMessage() });
    // 如果用户需求非空，追加到上下文
    if (userNeeds.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: userNeeds });
    }
    // 第五步：缓存观后感
    if (videoSummary) {
        savePromptToKnowledge(videoUrl, videoSummary);
        console.log('[观影者] 观后感已缓存');
    }
}
/** 动态图逐帧摘要：将多帧图片分批喂给描述者角色，生成文本摘要（仿照视频分批逐帧处理流程） */
async function summarizeDynamicImages(frames: string[]): Promise<string> {
    if (frames.length === 0) return '';
    /** 摘要结果片段 */
    const summaries: string[] = [];
    /** 单批最大帧数，避免单次请求超限 */
    const BATCH_SIZE = 8;
    // 分批逐帧处理：每批图片喂给描述者角色，汇总各批摘要
    for (let i = 0; i < frames.length; i += BATCH_SIZE) {
        const batch = frames.slice(i, i + BATCH_SIZE);
        try {
            // 将本批帧包装为多模态内容项
            descriptionRole.coverContext({
                role: 'user',
                content: batch.map(frame => ({ type: 'image_url', image_url: { url: frame } }))
            });
            /** 运行描述角色模型，获取本批摘要 */
            const summaryRequest = descriptionRole.run([], []);
            /** 本批摘要结果 */
            const summary = summaryRequest.body?.choices?.[0]?.message?.content;
            if (summary && summary.trim().length > 0) summaries.push(summary.trim());
        }
        catch (error) {
            console.error('[动态图摘要] 批次摘要失败:', error);
        }
    }
    return summaries.join('\n');
}
/** 处理图片文件 */
export async function LiteImageFile(): Promise<void> {
    // 遍历未读上下文数组中的每个消息
    for (let message of GlobalConfig.unreadContext) {
        // 跳过纯文本消息
        if (typeof message.content === 'string') continue;
        /** 新内容数组 */
        const newContent: Array<ImageContent | TextContent> = [];
        // 遍历消息内容中的每个项
        for (let item of message.content) {
            // 如果是文本项,直接添加到新内容数组
            if (item.type == 'text') newContent.push(item);
            // 检查是否为支持的视频文件格式
            else if (item.image_url && GlobalConfig.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format))) {
                // 处理视频文件
                await analysisVideoFile(item.image_url.url, '');
            }
            else if (item.image_url && !item.image_url.url.startsWith("data:image")) {
                // 获取图片文件内容
                const [response, error] = syncFetch({ url: item.image_url.url, execute: { crossDomain: true } });
                // 检查请求是否成功
                if (error) {
                    console.error('[获取图片文件失败]:', error.message, error.stack);
                    continue;
                };
                /** 缩放图片，返回图片数据数组（动态图多帧，静态图单帧） */
                const [resizedImages, error1] = resizeImage(response.body);
                // 检查缩放是否成功
                if (error1) {
                    console.error('[缩放图片失败]:', error1.message, error1.stack);
                    continue;
                };
                // 动态图（多帧）：仿照视频逐帧摘要为文本消息，不再直接将原图返回消息列表
                if (resizedImages.length > 1) {
                    newContent.push({ type: 'text', text: await summarizeDynamicImages(resizedImages.map(image => image.base64)) || '' });
                    continue;
                }
                // 静态图：遍历缩放结果，每帧作为独立的 image_url 内容项添加
                resizedImages.forEach(image => newContent.push({ type: 'image_url', image_url: { url: image.base64 } }));
            }
        }
        // 替换消息内容
        message.content = newContent;
    }
}
/** 批量处理视频文件 */
async function batchProcessVideoFiles(userNeeds?: string): Promise<void> {
    // 如果未读视频文件数组为空，直接返回
    if (GlobalConfig.unreadVideoUrl.length === 0) return;
    //  遍历未读视频文件数组
    for (const videoUrl of GlobalConfig.unreadVideoUrl) {
        try {
            // 处理视频文件
            await analysisVideoFile(videoUrl, userNeeds || '');
            // 等待1秒，避免对服务器造成过大压力
            await new Promise(resolve => setTimeout(resolve, 1000));
        }
        catch (error) { continue; }
    }
    // 清空未读视频文件数组
    GlobalConfig.unreadVideoUrl = [];
}
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
async function thoughtLoopTickEvent(): Promise<void> {
    // 如果正在思考中，直接返回
    if (GlobalConfig.reasoningInProgress) return;
    // 思考循环开始
    try {
        // 标记为思考中
        GlobalConfig.reasoningInProgress = true;
        // 查询 LTPX 工具状态，同步加载/卸载
        syncLTPXToolStatus();
        // 拉取外部消息
        await pullExternalMessages();
        // 检查计划表到期项，将到期计划内容写入上下文
        for (const item of checkDueItems()) {
            GlobalConfig.unreadContext.push({ role: 'user', content: `[计划提醒] 预约时间已到，请执行以下计划：${item.content}` })
        }
        /** 消息长度 */
        const messageLength = GlobalConfig.unreadContext.length + GlobalConfig.unreadVideoUrl.length;
        // 如果消息长度为0，跳过当前循环
        if (messageLength === 0) {
            // 标记为思考完成
            GlobalConfig.reasoningInProgress = false;
            // 进入下一次循环
            return;
        }
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
        const validMessage = textChunks.map(chunk => chunk.display).join('').trim();
        // 如果正文切片为空，抛出异常
        if (!validMessage.length) throw new Error('清洗后的文本为空');
        // 如果解析出行动区内容（动作区与情感区合并），分别交给行动者推理动作、记忆库匹配表情包
        if (actionBlocks.length) {
            // 调用行动者推理动作
            await actorRole.createCreativeWork(actionBlocks.join('|'));
            // 从记忆库匹配表情包并推送图片数据
            pushImage([await queryEmotionSticker(validMessage)], true);
        }
        // 未解析出行动区内容，且正文长度小于等于35字时，按概率基于正文推理表情包
        else if (validMessage.length <= 35 && Math.random() < 0.55) {
            pushImage([await queryEmotionSticker(validMessage)], true);
        }
        // 第一步：按顺序逐一发送思考区内容（不参与语音合成）
        for (const thinking of thinkingBlocks) {
            pushContext('text', thinking, '');
        }
        // 第二步：按顺序逐一发送代码块内容（不参与语音合成）
        for (const code of codeBlocks) {
            pushContext('text', code, '');
        }
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
/** 同步 LTPX 工具状态：查询 Go 层状态并执行加载/卸载 */
function syncLTPXToolStatus(): void {
    try {
        const statusJSON = getLTPXToolStatus();
        if (!statusJSON || statusJSON === '{}') return;
        const status = JSON.parse(statusJSON);
        // 处理待加载和待卸载
        if ((status.pendingLoads && status.pendingLoads.length > 0) ||
            (status.pendingUnloads && status.pendingUnloads.length > 0)) {
            processLTPXChanges(statusJSON);
        }
    }
    catch (e) {
        console.error('LTPX 工具状态同步失败:', e);
    }
}
/** 基于查询文本从表情包记忆库检索表情包图片，返回 base64；无结果或失败时返回 null */
async function queryEmotionSticker(query: string): Promise<string | null> {
    if (!query || !query.trim()) return null;
    try {
        // 首次使用时确保表情包集合存在（幂等：已存在则直接打开，不清空数据）
        if (!stickerCollectionReady) {
            const [ready] = memoryInit(STICKER_COLLECTION, 'image');
            if (!ready) return null;
            stickerCollectionReady = true;
        }
        /** 查询表情包记忆库 */
        const [results, error] = memoryQuery(STICKER_COLLECTION, query.trim(), 3);
        // 查询失败或无结果时返回 null
        if (error || !results || results.length === 0) return null;
        /** 在查询结果中随机选择一个结果 */
        const image = (results[RandomFloor(0, results.length - 1)] as { image?: string }).image;
        // 返回随机选择的图片或 null
        return image || null;
    }
    catch (error) {
        console.error('[表情包] 检索失败:', error);
        return null;
    }
}
/** 从消息中提取全部文本内容（多模态消息剔除图片，仅保留文本项；无文本时返回空字符串） */
export function extractTextFromMessage(message: PostMessage): string {
    // 纯文本消息和工具响应消息直接返回
    if (typeof message.content === 'string') return message.content;
    // 多模态消息：提取所有文本内容并拼接，剔除图片等非文本项
    if (Array.isArray(message.content)) {
        return message.content
            .filter(item => item.type === 'text')
            .map(item => item.text)
            .join(' ');
    }
    return '';
}
/** 初始化记忆库 */
function initMemory(): void {
    if (GlobalConfig.memoryReady) return;
    const [_, err] = memoryInit('lunar_messages', 'text');
    if (err) console.error('记忆库初始化失败:', err);
    else GlobalConfig.memoryReady = true;
}
/** 确保记忆库已初始化，返回是否就绪 */
export function ensureMemoryReady(): boolean {
    if (!GlobalConfig.memoryReady) initMemory();
    return GlobalConfig.memoryReady;
}
/** 记忆未读消息到记忆库 */
export function memorizeUnreadRecords(): void {
    // 缓冲池为空时跳过
    if (GlobalConfig.unreadRecords.length === 0) return;
    // 记忆库未就绪时保留缓冲消息，等待下次触发
    if (!ensureMemoryReady()) {
        console.warn('[记忆] 记忆库未就绪，保留缓冲消息待下次触发');
        return;
    }
    /** 成功写入的消息数量 */
    let written = 0;
    // 遍历未读消息缓冲池
    for (const message of GlobalConfig.unreadRecords) {
        // 过滤掉工具调用消息
        if (message.role === 'tool') continue;
        /** 提取文本内容并过滤空字符串 */
        const content = extractTextFromMessage(message).trim();
        // 过滤掉空字符串或长度小于等于5的消息
        if (!content || content.length <= 5) continue;
        /** 写入记忆库 */
        const [, error] = memoryAdd('lunar_messages', message.role, content);
        // 记录写入失败的错误信息
        if (error) console.error('[记忆] 写入记忆库失败:', error);
        // 记录成功写入的消息数量
        else written++;
    }
    console.log(`[记忆] 已写入 ${written} 条消息到记忆库`);
    // 清空消息缓冲池
    GlobalConfig.unreadRecords = [];
}
// 初始化 自定义配置 信息
fetchDocumentCallback('lunar_config.json').then(content => GlobalConfig.customConfig = content);
// 每秒执行一次思考循环
setInterval(() => thoughtLoopTickEvent(), 1000);
