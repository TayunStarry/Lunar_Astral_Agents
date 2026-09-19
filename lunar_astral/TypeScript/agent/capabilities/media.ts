import { getPromptFromKnowledge, savePromptToKnowledge } from '../../file/io/knowledge';
import { ImageContent, TextContent } from '../../config/model';
import { GlobalConfig } from '../../config/global';
import type { PerceiverRole } from '../roles/perceiver';

/** 角色单例注册表（由 roles.ts 在实例化后注入，解除 media → roles 的静态循环引用） */
let mediaRoles: { perceiverRole: PerceiverRole; randomDefaultMessage: () => string } | null = null;

/** 注入角色单例（roles.ts 模块初始化时调用一次） */
export function registerMediaRoles(roles: NonNullable<typeof mediaRoles>): void {
	mediaRoles = roles;
}

/** 视频理解失败时的兜底置换文本 */
const VIDEO_FALLBACK_TEXT = '月华看不了这个视频呢';

/** 音频转写失败时的兜底置换文本 */
const AUDIO_FALLBACK_TEXT = '月华听不懂这段语音呢';

/**
 * 理解视频并返回理解文本（含缓存读写）
 *
 * 媒体化失败时抛出异常；理解文本为空时返回空字符串，由调用方决定兜底提示。
 * 缓存命中时直接返回缓存的理解文本。
 */
async function understandVideo(videoUrl: string): Promise<string> {
    // 缓存检查：如果已处理过该视频，直接返回缓存结果
    const cachedPrompt = getPromptFromKnowledge(videoUrl);
    if (cachedPrompt) {
        console.log('[感知者] 命中视频缓存，直接返回');
        return cachedPrompt;
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
    // 第二步：交给感知者理解并返回理解文本
    return understandMediaSegments(mediaUrls, videoUrl);
}

/**
 * 观看媒体片段并返回理解文本（视频与动态图共用）
 *
 * 理解文本非空且存在缓存键时写入知识库缓存；理解为空时返回空字符串。
 *
 * @param mediaUrls 媒体目录内的 file:// 引用列表
 * @param cacheKey 知识库缓存键（原始来源地址；传空字符串表示不缓存）
 */
async function understandMediaSegments(mediaUrls: string[], cacheKey: string): Promise<string> {
    // 调用感知者智能体观看
    console.log('[感知者] 开始观看...');
    const videoSummary = await mediaRoles!.perceiverRole.watchVideo(mediaUrls);
    console.log('[感知者] 观看完成');
    // 理解文本非空时写入缓存
    if (videoSummary && videoSummary.trim().length > 0 && cacheKey) {
        savePromptToKnowledge(cacheKey, videoSummary);
        console.log('[感知者] 理解文本已缓存');
    }
    return videoSummary;
}

/**
 * 理解动态图（感知者智能体）
 *
 * 动态图先编码为慢放视频写入媒体目录，再走与视频相同的理解链路：
 * llama-server 端按固定频率抽帧 + 感知者分段理解，时序与动作过程由视频采样点承载。
 * 返回理解文本；转码失败时抛出异常，缓存命中时直接返回缓存文本。
 *
 * @param imageSource 动态图数据或地址（字节 / HTTP(S) URL / data:image URI）
 * @param cacheKey 知识库缓存键（URL 来源可缓存，data URI 为一次性数据不缓存）
 */
async function understandAnimatedImage(imageSource: any, cacheKey: string): Promise<string> {
    // 缓存检查：同一张动态图不重复做转码与理解
    const cachedPrompt = cacheKey ? getPromptFromKnowledge(cacheKey) : '';
    if (cachedPrompt) {
        console.log('[感知者] 命中动态图缓存，直接返回');
        return cachedPrompt;
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
    // 第二步：交给感知者理解并返回理解文本
    return understandMediaSegments(mediaUrls, cacheKey);
}

/**
 * 处理图片文件
 *
 * 消息内嵌与统一队列下发的音频/视频/动态图均不再追加到未读上下文末尾，
 * 而是将理解文本（或失败兜底文本）原位替换对应的媒体内容项，
 * 确保理解结果与消息内其他内容保持宿主队列的到达时序。
 */
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
            // 视频URL内容块（宿主统一时序队列下发）：理解文本原位置换视频块
            else if (item.type === 'video_url') {
                newContent.push({ type: 'text', text: await understandVideoOrFallback(item.video_url.url) });
            }
            // 音频URL内容块（宿主统一时序队列下发）：转写文本原位置换音频块
            else if (item.type === 'audio_url') {
                newContent.push({ type: 'text', text: await understandAudioOrFallback(item.audio_url.url) });
            }
            // 音频内容（内联 base64）：转写为文本后原位替换音频块
            else if ('input_audio' in item) {
                const format = item.input_audio.format || 'wav';
                newContent.push({ type: 'text', text: await understandAudioOrFallback(`data:audio/${format};base64,${item.input_audio.data}`) });
            }
            // 检查是否为视频文件（扩展名匹配或内联 data:video URI）
            else if (item.image_url && (item.image_url.url.toLowerCase().startsWith('data:video/') || GlobalConfig.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format)))) {
                // 视频：理解文本原位替换视频块（失败时以兜底文本置换）
                newContent.push({ type: 'text', text: await understandVideoOrFallback(item.image_url.url) });
            }
            // 图片（HTTP(S) URL 与内联 data:image URI 统一处理）：
            // 地址交由 Go 侧解码为字节，再按标准流程分流——动态图转视频走感知者链路，静态图走标准图片处理
            else if (item.image_url) {
                /** 图片来源地址（URL 或 data URI，下载/解码由 Go 侧完成） */
                const imageSource = item.image_url.url;
                /** 是否为动态图（多帧 GIF / APNG / 动态 WebP） */
                const [isAnimated, animatedError] = isAnimatedImage(imageSource);
                if (animatedError) console.error('[动态图判定失败]:', animatedError.message);
                // 动态图：理解文本原位替换动态图块（失败时以视频兜底文本置换）
                else if (isAnimated) {
                    // 内联 data URI 为一次性上传数据，不做知识库缓存；URL 来源以原地址为缓存键
                    const cacheKey = imageSource.startsWith('data:') ? '' : imageSource;
                    let summary = '';
                    try {
                        summary = await understandAnimatedImage(imageSource, cacheKey);
                    }
                    catch (error) {
                        console.error('[感知者] 动态图理解失败，以兜底文本置换:', error);
                    }
                    // 原位替换：理解文本写回动态图块的位置（理解为空时以默认应答兜底）
                    newContent.push({ type: 'text', text: summary.trim().length > 0 ? summary : mediaRoles!.randomDefaultMessage() });
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

/** 理解视频并返回理解文本；失败或理解为空时返回兜底/默认提示（调用方原位置换） */
async function understandVideoOrFallback(videoUrl: string): Promise<string> {
    try {
        /** 视频理解文本 */
        const summary = await understandVideo(videoUrl);
        if (summary.trim().length > 0) return summary;
        // 理解为空时以默认应答兜底
        return mediaRoles!.randomDefaultMessage();
    }
    catch (error) {
        console.error('[感知者] 视频理解失败，以兜底文本置换:', error);
        return VIDEO_FALLBACK_TEXT;
    }
}

/** 转写音频并返回转写文本；失败时返回兜底提示（调用方原位置换） */
async function understandAudioOrFallback(audioSource: string): Promise<string> {
    try {
        return await understandAudio(audioSource);
    }
    catch (error) {
        console.error('[感知者] 音频理解失败，以兜底文本置换:', error);
        return AUDIO_FALLBACK_TEXT;
    }
}

/**
 * 理解音频（两段式：ASR 转写 → 返回转写文本）
 *
 * 返回文本含【语音转写】前缀；转换/转写失败时抛出异常。
 * URL 形式的音频按地址缓存（data URI 为一次性录音/上传，跳过缓存）。
 */
async function understandAudio(audioSource: string): Promise<string> {
    // 缓存检查
    const cacheable = !audioSource.startsWith('data:');
    const cachedPrompt = cacheable ? getPromptFromKnowledge(audioSource) : '';
    if (cachedPrompt) {
        console.log('[感知者] 命中音频缓存，直接返回');
        return cachedPrompt;
    }
    // 第一步：将音频本地化并转换为 16kHz 单声道 WAV
    console.log('[感知者] 开始转换音频...');
    const [wavBase64, audioError] = audioWav(audioSource);
    if (!wavBase64 || audioError) {
        console.error('[感知者] 音频转换失败:', audioError);
        throw new Error('音频转换失败');
    }
    // 第二步：调用语音识别模型转写
    console.log('[感知者] 开始语音转写...');
    const transcript = await transcribeAudio(wavBase64);
    if (!transcript.trim()) {
        console.error('[感知者] 语音转写为空');
        throw new Error('语音转写为空');
    }
    console.log(`[感知者] 语音转写完成: ${transcript.substring(0, 50)}`);
    // 第三步：转写文本直接作为音频内容的完整表达，并写入缓存
    const text = `【语音转写】${transcript}`;
    if (cacheable) {
        savePromptToKnowledge(audioSource, text);
        console.log('[感知者] 语音转写文本已缓存');
    }
    return text;
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
