import { getPromptFromKnowledge, savePromptToKnowledge } from '../../file/io/knowledge';
import { AudioContent, ImageContent, TextContent, PostMessage } from '../../config/model';
import { GlobalConfig } from '../../config/global';
import { interactEvent } from './ltp-event';
import type { PerceiverRole } from '../roles/perceiver';

/** 角色单例注册表（由 roles.ts 在实例化后注入，解除 media → roles 的静态循环引用） */
let mediaRoles: { perceiverRole: PerceiverRole; randomDefaultMessage: () => string } | null = null;

/** 注入角色单例（roles.ts 模块初始化时调用一次） */
export function registerMediaRoles(roles: NonNullable<typeof mediaRoles>): void {
	mediaRoles = roles;
}

/** 处理视频文件（感知者智能体） */
async function analysisVideoFile(videoUrl: string, userNeeds: string): Promise<void> {
    // 缓存检查：如果已处理过该视频，直接返回缓存结果
    const cachedPrompt = getPromptFromKnowledge(videoUrl);
    if (cachedPrompt) {
        GlobalConfig.unreadContext.push({ role: 'user', content: cachedPrompt });
        console.log('[感知者] 命中视频缓存，直接返回');
        return;
    }
    // 第一步：视频本地化到 llama-server 媒体目录（长视频自动分段），获取 file:// 引用
    console.log('[感知者] 开始将视频写入媒体目录...');
    const [segments, mediaError] = videoMedia(videoUrl);
    if (!segments || segments.length === 0 || mediaError) {
        console.error('[感知者] 视频媒体化失败:', mediaError);
        throw new Error('视频媒体化失败');
    }
    /** 媒体片段 file:// 引用列表（llama-server 端 ffmpeg 自动抽帧） */
    const mediaUrls = segments.map(segment => `file://${segment.file}`);
    console.log(`[感知者] 视频媒体化完成，共 ${mediaUrls.length} 个片段`);
    // 第二步：交给感知者理解并写入上下文/缓存
    await analysisMediaSegments(mediaUrls, videoUrl, userNeeds);
}

/**
 * 观看媒体片段并写入上下文与缓存（视频与动态图共用）
 *
 * @param mediaUrls 媒体目录内的 file:// 引用列表
 * @param cacheKey 知识库缓存键（原始来源地址；传空字符串表示不缓存）
 * @param userNeeds 用户附加需求，非空时追加到上下文
 */
async function analysisMediaSegments(mediaUrls: string[], cacheKey: string, userNeeds: string): Promise<void> {
    // 第二步：调用感知者智能体观看
    console.log('[感知者] 开始观看...');
    const videoSummary = await mediaRoles!.perceiverRole.watchVideo(mediaUrls);
    console.log('[感知者] 观看完成');
    // 第三步：将理解文本添加到未读上下文
    if (videoSummary && videoSummary.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: videoSummary });
    }
    else GlobalConfig.unreadContext.push({ role: 'user', content: mediaRoles!.randomDefaultMessage() });
    // 如果用户需求非空，追加到上下文
    if (userNeeds.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: userNeeds });
    }
    // 第四步：缓存理解文本
    if (videoSummary && cacheKey) {
        savePromptToKnowledge(cacheKey, videoSummary);
        console.log('[感知者] 理解文本已缓存');
    }
}

/**
 * 处理动态图（感知者智能体）
 *
 * 动态图先编码为慢放视频写入媒体目录，再走与视频相同的理解链路：
 * llama-server 端按固定频率抽帧 + 感知者分段理解，时序与动作过程由视频采样点承载。
 *
 * @param imageSource 动态图数据或地址（字节 / HTTP(S) URL / data:image URI）
 * @param cacheKey 知识库缓存键（URL 来源可缓存，data URI 为一次性数据不缓存）
 */
async function analysisAnimatedImage(imageSource: any, cacheKey: string): Promise<void> {
    // 缓存检查：同一张动态图不重复做转码与理解
    const cachedPrompt = cacheKey ? getPromptFromKnowledge(cacheKey) : '';
    if (cachedPrompt) {
        GlobalConfig.unreadContext.push({ role: 'user', content: cachedPrompt });
        console.log('[感知者] 命中动态图缓存，直接返回');
        return;
    }
    // 第一步：动态图编码为慢放视频（补足抽帧采样点数）
    console.log('[感知者] 检测到动态图，开始编码为视频...');
    const [segments, mediaError] = animatedImageToVideo(imageSource);
    if (!segments || segments.length === 0 || mediaError) {
        console.error('[感知者] 动态图转视频失败:', mediaError);
        throw new Error('动态图转视频失败');
    }
    /** 媒体片段 file:// 引用列表 */
    const mediaUrls = segments.map(segment => `file://${segment.file}`);
    console.log(`[感知者] 动态图转视频完成，共 ${mediaUrls.length} 个片段`);
    // 第二步：交给感知者理解并写入上下文/缓存
    await analysisMediaSegments(mediaUrls, cacheKey, '');
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
            // 音频内容：转写为文本写入未读上下文（见 analysisAudioFile），音频块不再透传
            else if ('input_audio' in item) {
                const format = item.input_audio.format || 'wav';
                try {
                    await analysisAudioFile(`data:audio/${format};base64,${item.input_audio.data}`, '');
                }
                catch (error) {
                    console.error('[感知者] 音频理解失败，跳过该音频:', error);
                }
            }
            // 检查是否为视频文件（扩展名匹配或内联 data:video URI）
            else if (item.image_url && (item.image_url.url.toLowerCase().startsWith('data:video/') || GlobalConfig.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format)))) {
                // 处理视频文件：理解文本写入未读上下文，原视频块不再透传
                try {
                    await analysisVideoFile(item.image_url.url, '');
                }
                catch (error) {
                    console.error('[感知者] 视频理解失败，跳过该视频:', error);
                }
            }
            // 图片（HTTP(S) URL 与内联 data:image URI 统一处理）：
            // 地址交由 Go 侧解码为字节，再按标准流程分流——动态图转视频走感知者链路，静态图走标准图片处理
            else if (item.image_url) {
                /** 图片来源地址（URL 或 data URI，下载/解码由 Go 侧完成） */
                const imageSource = item.image_url.url;
                /** 是否为动态图（多帧 GIF / APNG / 动态 WebP） */
                const [isAnimated, animatedError] = isAnimatedImage(imageSource);
                if (animatedError) console.error('[动态图判定失败]:', animatedError.message);
                // 动态图：编码为慢放视频后交给感知者理解，原图块不再透传
                else if (isAnimated) {
                    // 内联 data URI 为一次性上传数据，不做知识库缓存；URL 来源以原地址为缓存键
                    const cacheKey = imageSource.startsWith('data:') ? '' : imageSource;
                    try {
                        await analysisAnimatedImage(imageSource, cacheKey);
                    }
                    catch (error) {
                        console.error('[感知者] 动态图理解失败，跳过该图片:', error);
                    }
                    continue;
                }
                // 内联 base64 静态图已是处理管线的最终形态（data:image URI），直接透传，不再重复缩放
                if (imageSource.toLowerCase().startsWith('data:image/')) {
                    newContent.push(item);
                    continue;
                }
                /** 标准图片处理：解码 → 格式校验/转码 → 等比例缩放到 1024 → base64 */
                const [resizedImages, resizeError] = resizeImage(imageSource);
                if (resizeError) {
                    console.error('[图片处理失败]:', resizeError.message, resizeError.stack);
                    continue;
                }
                // 静态图：将处理结果作为独立的 image_url 内容项添加
                resizedImages.forEach(image => newContent.push({ type: 'image_url', image_url: { url: image.base64 } }));
            }
        }
        // 替换消息内容
        message.content = newContent;
    }
}

/** 批量处理视频文件 */
export async function batchProcessVideoFiles(userNeeds?: string): Promise<void> {
    // 如果未读视频文件数组为空，直接返回
    if (GlobalConfig.unreadVideoUrl.length === 0) return;
    // 事件 -> 观看视频前：推送待处理视频列表，插件可改写后再逐个观看
    const feedback: PostMessage[] = interactEvent('watch_video_before', { userNeeds, videoUrls: GlobalConfig.unreadVideoUrl }).return;
    // 如果插件返回了视频解析结果则直接添加到上下文数组中
    if (feedback && Array.isArray(feedback) && feedback.every(r => r.content && r.role !== undefined)) {
        GlobalConfig.unreadContext.push(...feedback);
    }
    // 否则遍历未读视频文件数组，逐个处理
    else for (const videoUrl of GlobalConfig.unreadVideoUrl) {
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

/** 处理音频文件（两段式：ASR 转写 → 转写文本直入上下文） */
async function analysisAudioFile(audioSource: string, userNeeds: string): Promise<void> {
    // 缓存检查：URL 形式的音频按地址缓存（data URI 为一次性录音/上传，跳过缓存）
    const cacheable = !audioSource.startsWith('data:');
    const cachedPrompt = cacheable ? getPromptFromKnowledge(audioSource) : '';
    if (cachedPrompt) {
        GlobalConfig.unreadContext.push({ role: 'user', content: cachedPrompt });
        console.log('[感知者] 命中音频缓存，直接返回');
        return;
    }
    // 第一步：将音频本地化并转换为 16kHz 单声道 WAV
    console.log('[感知者] 开始转换音频...');
    const [wavBase64, audioError] = audioWav(audioSource);
    if (!wavBase64 || audioError) {
        console.error('[感知者] 音频转换失败:', audioError);
        throw new Error('音频转换失败');
    }
    // 第二步：调用语音识别模型转写（中英文统一走 ASR，多模态模型无需音频能力）
    console.log('[感知者] 开始语音转写...');
    const transcript = await transcribeAudio(wavBase64);
    if (!transcript.trim()) {
        console.error('[感知者] 语音转写为空');
        throw new Error('语音转写为空');
    }
    console.log(`[感知者] 语音转写完成: ${transcript.substring(0, 50)}`);
    // 第三步：转写文本直接入上下文（ASR 输出即音频内容的完整表达，无需再过模型）
    GlobalConfig.unreadContext.push({ role: 'user', content: `【语音转写】${transcript}` });
    // 如果用户需求非空，追加到上下文
    if (userNeeds.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: userNeeds });
    }
    // 第四步：缓存转写文本
    if (cacheable) {
        savePromptToKnowledge(audioSource, `【语音转写】${transcript}`);
        console.log('[感知者] 语音转写文本已缓存');
    }
}

/**
 * 调用语音识别模型（system-asr）转写 16kHz WAV 音频
 *
 * @param wavBase64 16kHz 单声道 WAV 音频的 base64 编码
 * @returns 转写文本（已剥离 ASR 输出的 "language xxx<asr_text>" 前缀）
 */
async function transcribeAudio(wavBase64: string): Promise<string> {
    const [result, error] = syncFetch({
        url: GlobalConfig.MultimodalUrl + '/chat/completions',
        execute: {
            method: 'POST',
            crossDomain: true,
            headers: {
                Authorization: `Bearer ${encodeURIComponent(GlobalConfig.MultimodalKey)}`,
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                model: GlobalConfig.AsrName,
                messages: [{
                    role: 'user',
                    content: [{ type: 'input_audio', input_audio: { data: wavBase64, format: 'wav' } }]
                }],
                temperature: 0,
            }),
        },
    });
    if (error) {
        console.error('[感知者] 语音转写请求失败:', error.message);
        return '';
    }
    /** ASR 原始输出（形如 "language Chinese<asr_text>转写内容"） */
    const raw = result?.body?.choices?.[0]?.message?.content || '';
    // 剥离语言标记前缀，仅保留转写正文
    const marker = '<asr_text>';
    return raw.includes(marker) ? raw.substring(raw.indexOf(marker) + marker.length).trim() : raw.trim();
}

/** 批量处理音频文件 */
export async function batchProcessAudioFiles(userNeeds?: string): Promise<void> {
    // 如果未读音频文件数组为空，直接返回
    if (GlobalConfig.unreadAudioUrl.length === 0) return;
    // 遍历未读音频文件数组，逐个处理
    for (const audioSource of GlobalConfig.unreadAudioUrl) {
        try {
            // 处理音频文件
            await analysisAudioFile(audioSource, userNeeds || '');
            // 等待1秒，避免对服务器造成过大压力
            await new Promise(resolve => setTimeout(resolve, 1000));
        }
        catch (error) { continue; }
    }
    // 清空未读音频文件数组
    GlobalConfig.unreadAudioUrl = [];
}
