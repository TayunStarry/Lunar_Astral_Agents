﻿import { getPromptFromKnowledge, savePromptToKnowledge } from '../../file/io/knowledge';
import { AudioContent, ImageContent, TextContent, PostMessage } from '../../config/model';
import { GlobalConfig } from '../../config/global';
import { interactEvent } from './ltp-event';
import type { ModelBuilder } from '../base/builder';
import type { ViewerRole } from '../roles/viewer';

/** 角色单例注册表（由 roles.ts 在实例化后注入，解除 media → roles 的静态循环引用） */
let mediaRoles: { descriptionRole: ModelBuilder; viewerRole: ViewerRole; randomDefaultMessage: () => string } | null = null;

/** 注入角色单例（roles.ts 模块初始化时调用一次） */
export function registerMediaRoles(roles: NonNullable<typeof mediaRoles>): void {
	mediaRoles = roles;
}

/** 处理视频文件（观影者智能体） */
async function analysisVideoFile(videoUrl: string, userNeeds: string): Promise<void> {
    // 缓存检查：如果已处理过该视频，直接返回缓存结果
    const cachedPrompt = getPromptFromKnowledge(videoUrl);
    if (cachedPrompt) {
        GlobalConfig.unreadContext.push({ role: 'user', content: cachedPrompt });
        console.log('[影音者] 命中视频缓存，直接返回');
        return;
    }
    // 第一步：视频本地化到 llama-server 媒体目录（长视频自动分段），获取 file:// 引用
    console.log('[影音者] 开始将视频写入媒体目录...');
    const [segments, mediaError] = videoMedia(videoUrl);
    if (!segments || segments.length === 0 || mediaError) {
        console.error('[影音者] 视频媒体化失败:', mediaError);
        throw new Error('视频媒体化失败');
    }
    /** 媒体片段 file:// 引用列表（llama-server 端 ffmpeg 自动抽帧） */
    const mediaUrls = segments.map(segment => `file://${segment.file}`);
    console.log(`[影音者] 视频媒体化完成，共 ${mediaUrls.length} 个片段`);
    // 第二步：调用观影者智能体观看视频
    console.log('[影音者] 开始观看视频...');
    const videoSummary = await mediaRoles!.viewerRole.watchVideo(mediaUrls);
    console.log('[影音者] 视频观看完成');
    // 第三步：将视频理解文本添加到未读上下文
    if (videoSummary && videoSummary.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: videoSummary });
    }
    else GlobalConfig.unreadContext.push({ role: 'user', content: mediaRoles!.randomDefaultMessage() });
    // 如果用户需求非空，追加到上下文
    if (userNeeds.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: userNeeds });
    }
    // 第四步：缓存视频理解文本
    if (videoSummary) {
        savePromptToKnowledge(videoUrl, videoSummary);
        console.log('[影音者] 视频理解文本已缓存');
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
            mediaRoles!.descriptionRole.coverContext({
                role: 'user',
                content: batch.map(frame => ({ type: 'image_url', image_url: { url: frame } }))
            });
            /** 运行描述角色模型，获取本批摘要 */
            const summaryRequest = mediaRoles!.descriptionRole.run([], []);
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
            // 音频内容：交给影音者音频理解分支，理解文本写入未读上下文，音频块不再透传
            else if ('input_audio' in item) {
                const format = item.input_audio.format || 'wav';
                try {
                    await analysisAudioFile(`data:audio/${format};base64,${item.input_audio.data}`, '');
                }
                catch (error) {
                    console.error('[影音者] 音频理解失败，跳过该音频:', error);
                }
            }
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
                    /** 月华对动态图的视觉感知摘要 */
                    const visualSummary = await summarizeDynamicImages(resizedImages.map(image => image.base64));
                    if (visualSummary && visualSummary.trim().length > 0) {
                        // 以月华自身所见（assistant 角色）注入上下文，而非用户告知
                        GlobalConfig.unreadContext.push({ role: 'assistant', content: visualSummary.trim() });
                    }
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

/** 处理音频文件（影音者智能体，两段式：ASR 转写 → 内容理解） */
async function analysisAudioFile(audioSource: string, userNeeds: string): Promise<void> {
    // 缓存检查：URL 形式的音频按地址缓存（data URI 为一次性录音/上传，跳过缓存）
    const cacheable = !audioSource.startsWith('data:');
    const cachedPrompt = cacheable ? getPromptFromKnowledge(audioSource) : '';
    if (cachedPrompt) {
        GlobalConfig.unreadContext.push({ role: 'user', content: cachedPrompt });
        console.log('[影音者] 命中音频缓存，直接返回');
        return;
    }
    // 第一步：将音频本地化并转换为 16kHz 单声道 WAV
    console.log('[影音者] 开始转换音频...');
    const [wavBase64, audioError] = audioWav(audioSource);
    if (!wavBase64 || audioError) {
        console.error('[影音者] 音频转换失败:', audioError);
        throw new Error('音频转换失败');
    }
    // 第二步：调用语音识别模型转写（中英文统一走 ASR，多模态模型无需音频能力）
    console.log('[影音者] 开始语音转写...');
    const transcript = await transcribeAudio(wavBase64);
    if (!transcript.trim()) {
        console.error('[影音者] 语音转写为空');
        throw new Error('语音转写为空');
    }
    console.log(`[影音者] 语音转写完成: ${transcript.substring(0, 50)}`);
    // 第三步：调用影音者智能体基于转写文本生成音频理解
    console.log('[影音者] 开始理解音频内容...');
    const audioUnderstanding = await mediaRoles!.viewerRole.watchAudio(transcript);
    console.log('[影音者] 音频理解完成');
    // 第四步：将音频理解文本添加到未读上下文
    if (audioUnderstanding && audioUnderstanding.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: audioUnderstanding });
    }
    else GlobalConfig.unreadContext.push({ role: 'user', content: `（该段音频转写为：${transcript}）` });
    // 如果用户需求非空，追加到上下文
    if (userNeeds.trim().length > 0) {
        GlobalConfig.unreadContext.push({ role: 'user', content: userNeeds });
    }
    // 第五步：缓存音频理解文本
    if (audioUnderstanding && cacheable) {
        savePromptToKnowledge(audioSource, audioUnderstanding);
        console.log('[影音者] 音频理解文本已缓存');
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
        console.error('[影音者] 语音转写请求失败:', error.message);
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
