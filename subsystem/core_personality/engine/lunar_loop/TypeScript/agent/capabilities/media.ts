import { getPromptFromKnowledge, savePromptToKnowledge } from '../../file/io/knowledge';
import { ImageContent, TextContent } from '../../config/model';
import { GlobalConfig } from '../../config/global';
import { guessImageExt, handleFromVisionURL } from './image-tag';
import { admitImageDataURI, admitResource, getResourceEntry, setResourceSummary } from './resource';
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

/** 是否采用图片序列帧模式解读视频/动态图（lunar_config.json 的 multimodal.video_input === 'frames'） */
function useFrameSequence(): boolean {
	return GlobalConfig.VideoInputMode === 'frames';
}

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
    // 序列帧模式：本地抽帧为图片序列（base64），不依赖服务端读取 file:// 视频文件
    if (useFrameSequence()) {
        console.log('[感知者] 开始将视频抽取为图片序列帧...');
        const [groups, framesError] = videoFrames(videoUrl);
        if (!groups || groups.length === 0 || framesError) {
            console.error('[感知者] 视频抽帧失败:', framesError);
            throw new Error('视频抽帧失败');
        }
        /** 采样帧总数（仅用于日志） */
        const frameCount = groups.reduce((sum, group) => sum + group.frames.length, 0);
        console.log(`[感知者] 视频抽帧完成，共 ${groups.length} 组 / ${frameCount} 帧`);
        // 第二步：交给感知者理解并返回理解文本
        return understandMediaSegments(() => mediaRoles!.perceiverRole.watchFrames(groups), videoUrl);
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
    return understandMediaSegments(() => mediaRoles!.perceiverRole.watchVideo(mediaUrls), videoUrl);
}

// ===== 资源化：把媒体收进统一资源归档层，并用句柄替换原始媒体项 =====
//
// 设计依据：docs/adr/0001「统一入库入口」。
// 三类来源（前端已落盘的本地文件 / QQ 等渠道的远程 URL / 内联 data URI）在 Go 侧
// 统一归一，因此这里只负责「拿句柄 → 写理解结论 → 产出带句柄的文本」。
//
// 为什么视频/音频先本地化再登记：本地化（转码/分段/WAV 归一）本来就要下载一次，
// 登记其产物可以避免对同一远程地址重复下载；产物落在 multimedia/footage 与临时目录，
// 由 Go 侧按内容哈希去重。

/** 资源化后的媒体：句柄 + 目录主键 */
type AdmittedMedia = { handle: string; id: number };

/**
 * 把媒体收进资源目录
 *
 * @param source 媒体来源（路径 / 服务地址 / 远程 URL / data URI）
 * @param name 建议文件名（决定类型码与扩展名）
 * @param origin 来源语义
 * @param kindHint 类型码建议值
 * @returns 句柄与目录主键；失败返回 null
 */
function admitMedia(
	source: string,
	name: string,
	origin: 'client-upload' | 'channel' | 'tool-output' | 'engine-temp',
	kindHint?: 'D' | 'I' | 'V' | 'A'
): AdmittedMedia | null {
	if (!source) return null;
	const lower = source.toLowerCase();
	const sourceKind = lower.startsWith('data:') ? 'data' : (/^https?:\/\//i.test(source) ? 'url' : 'path');
	const result = admitResource({ sourceKind, source, name, origin, kindHint });
	return result ? { handle: result.handle, id: result.id } : null;
}

/** 把媒体的理解结论写回资源目录（供 list_resources / read_resource 复用） */
function storeMediaSummary(media: AdmittedMedia, summary: string): void {
	if (!media || !summary || !summary.trim()) return;
	setResourceSummary(media.id, summary.trim().slice(0, 1000));
}

/**
 * 判断资源是否已有理解结论（避免重复调用感知者）
 *
 * @param media 入库后的媒体
 * @returns 已有的结论文本；无则空串
 */
function existingSummary(media: AdmittedMedia): string {
	const entry = getResourceEntry(media.handle);
	return entry?.summary || '';
}

/**
 * 视频资源化：本地化 → 登记 → 理解 → 写回结论 → 产出带句柄文本
 *
 * @param originalSource 用户/渠道给出的原始视频来源（理解链路的输入）
 * @param name 建议文件名
 * @param origin 来源语义
 * @param admitSource 登记进资源目录的来源（本地化产物路径；缺省用原始来源）
 * @returns 形如 `@V.001【视频理解】…` 的文本；失败返回空串由调用方兜底
 */
async function resourceVideo(originalSource: string, name: string, origin: 'client-upload' | 'channel', admitSource?: string): Promise<string> {
	const media = admitMedia(admitSource || originalSource, name, origin, 'V');
	if (!media) return '';
	const cached = existingSummary(media);
	if (cached) return `${media.handle}【视频理解】${cached}`;
	try {
		const summary = await understandVideo(originalSource);
		if (!summary || !summary.trim()) return `${media.handle}`;
		storeMediaSummary(media, summary);
		return `${media.handle}【视频理解】${summary}`;
	}
	catch (error) {
		console.error('[感知者] 视频理解失败（句柄已保留）:', error);
		return `${media.handle}${VIDEO_FALLBACK_TEXT}`;
	}
}

/**
 * 音频资源化：转写 → 登记 → 写回结论 → 产出带句柄文本
 *
 * @param originalSource 原始音频来源
 * @param name 建议文件名
 * @param origin 来源语义
 * @returns 形如 `@A.002【语音转写】…` 的文本；失败返回空串由调用方兜底
 */
async function resourceAudio(originalSource: string, name: string, origin: 'client-upload' | 'channel'): Promise<string> {
	const media = admitMedia(originalSource, name, origin, 'A');
	if (!media) return '';
	const cached = existingSummary(media);
	if (cached) return `${media.handle}${cached}`;
	try {
		const transcript = await understandAudio(originalSource);
		storeMediaSummary(media, transcript);
		return `${media.handle}${transcript}`;
	}
	catch (error) {
		console.error('[感知者] 音频理解失败（句柄已保留）:', error);
		return `${media.handle}${AUDIO_FALLBACK_TEXT}`;
	}
}

/** 从来源推断文件名（媒体资源命名用；无法推断时按类型码兜底） */
function mediaName(source: string, fallbackExt: string): string {
	const lower = (source || '').toLowerCase();
	const dataMatch = /^data:(image|video|audio)\/(\w+)[;,]/.exec(lower);
	if (dataMatch) return `media.${dataMatch[2] === 'jpeg' ? 'jpg' : dataMatch[2]}`;
	const path = lower.split('?')[0].split('#')[0];
	const base = path.slice(path.lastIndexOf('/') + 1);
	if (base && /\.[a-z0-9]{2,5}$/.test(base)) return base;
	return `media${fallbackExt}`;
}

/**
 * 观看并返回理解文本（视频与动态图共用，兼容 file:// 引用与图片序列帧两种输入）
 *
 * 理解文本非空且存在缓存键时写入知识库缓存；理解为空时返回空字符串。
 *
 * @param watch 感知者观看回调（file 模式观看 file:// 片段引用，序列帧模式观看采样帧分组）
 * @param cacheKey 知识库缓存键（原始来源地址；传空字符串表示不缓存）
 */
async function understandMediaSegments(watch: () => Promise<string>, cacheKey: string): Promise<string> {
    // 调用感知者智能体观看
    console.log('[感知者] 开始观看...');
    const videoSummary = await watch();
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
 * 动态图先编码为慢放视频再走与视频相同的理解链路（file 模式下写媒体目录并由服务端抽帧，
 * 序列帧模式下本地抽帧为 base64 图片序列），时序与动作过程由视频采样点承载。
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
    // 序列帧模式：慢放转码后抽帧为图片序列（base64），不依赖服务端读取 file:// 视频文件
    if (useFrameSequence()) {
        const [groups, framesError] = animatedImageToFrames(imageSource);
        if (!groups || groups.length === 0 || framesError) {
            console.error('[感知者] 动态图抽帧失败:', framesError);
            throw new Error('动态图抽帧失败');
        }
        /** 采样帧总数（仅用于日志） */
        const frameCount = groups.reduce((sum, group) => sum + group.frames.length, 0);
        console.log(`[感知者] 动态图抽帧完成，共 ${groups.length} 组 / ${frameCount} 帧`);
        // 第二步：交给感知者理解并返回理解文本
        return understandMediaSegments(() => mediaRoles!.perceiverRole.watchFrames(groups), cacheKey);
    }
    const [segments, mediaError] = animatedImageToVideo(imageSource);
    if (!segments || segments.length === 0 || mediaError) {
        console.error('[感知者] 动态图转视频失败:', mediaError);
        throw new Error('动态图转视频失败');
    }
    /** 媒体片段 file:// 引用列表 */
    const mediaUrls = segments.map(segment => `file://${segment.file}`);
    console.log(`[感知者] 动态图转视频完成，共 ${mediaUrls.length} 个片段`);
    // 第二步：交给感知者理解并返回理解文本
    return understandMediaSegments(() => mediaRoles!.perceiverRole.watchVideo(mediaUrls), cacheKey);
}

/**
 * 处理图片文件
 *
 * 静态图在推入未读数组时已由入队赋签机制改写为视觉读取形态（约束尺寸 + 句柄叠印副本，
 * 或 base64 形态的同一变换产物），本函数只负责会动的与听得着的媒体：
 *   · 动态图：感知者理解后原位置换为「句柄 + 理解文本」（不占多模态窗口）
 *   · 视频 / 音频：理解或转写后原位置换为「句柄 + 理解/转写文本」
 * 静态图原样透传，保证理解结果与消息内其他内容保持宿主队列的到达时序。
 */
export async function LiteImageFile(): Promise<void> {
    // 遍历未读上下文数组中的每个消息
    for (let message of GlobalConfig.unreadContext) {
        // 跳过纯文本消息
        if (typeof message.content === 'string') continue;
        /** 新内容数组 */
        const newContent: Array<ImageContent | TextContent> = [];
        // 遍历消息内容中的每个项
        for (let item of message.content as any[]) {
            // 如果是文本项,直接添加到新内容数组
            if (item.type == 'text') {
                newContent.push(item);
                continue;
            }
            // 视频URL内容块（宿主统一时序队列下发）：收进资源目录后以「句柄 + 理解文本」原位置换
            if (item.type === 'video_url') {
                newContent.push({ type: 'text', text: await resourceVideoOrFallback(item.video_url.url) });
                continue;
            }
            // 音频URL内容块（宿主统一时序队列下发）：收进资源目录后以「句柄 + 转写文本」原位置换
            if (item.type === 'audio_url') {
                newContent.push({ type: 'text', text: await resourceAudioOrFallback(item.audio_url.url) });
                continue;
            }
            // 音频内容（内联 base64）：转写为文本后原位替换音频块
            if ('input_audio' in item) {
                const format = item.input_audio.format || 'wav';
                newContent.push({ type: 'text', text: await resourceAudioOrFallback(`data:audio/${format};base64,${item.input_audio.data}`) });
                continue;
            }
            if (!item.image_url) continue;
            /** 图片来源地址（入队时已改写为视觉读取 URL / data URI） */
            const imageSource = item.image_url.url;
            // 视频（扩展名匹配或内联 data:video URI）：收进资源目录后以「句柄 + 理解文本」原位置换
            if (imageSource.toLowerCase().startsWith('data:video/') || GlobalConfig.videoFormatsExtensions.some(format => imageSource.toLowerCase().endsWith(format))) {
                newContent.push({ type: 'text', text: await resourceVideoOrFallback(imageSource) });
                continue;
            }
            /** 是否为动态图（多帧 GIF / APNG / 动态 WebP；视觉端点对动图原样直通，判定依然有效） */
            const [isAnimated, animatedError] = isAnimatedImage(imageSource);
            if (animatedError) console.error('[动态图判定失败]:', animatedError.message);
            // 动态图：置换为「句柄 + 理解文本」
            if (!animatedError && isAnimated) {
                // 句柄优先取 URL 自带的 tag 参数（入队赋签写入）；缺失时现场入库兜底
                let handle = handleFromVisionURL(imageSource);
                if (!handle) {
                    const admitted = admitImageDataURI(imageSource, `image${guessImageExt(imageSource)}`);
                    handle = admitted?.handle || '';
                }
                newContent.push({ type: 'text', text: await replaceAnimatedImage(imageSource, handle) });
                continue;
            }
            // 静态图：已是视觉读取形态（尺寸受约束、句柄烙在图上），原样进入多模态窗口
            newContent.push(item);
        }
        // 替换消息内容
        message.content = newContent;
    }
}

/**
 * 动态图原位置换：感知者理解 → 结论写回资源目录 → 产出「句柄 + 理解文本」
 *
 * @param imageSource 动态图数据或地址
 * @param handle 入队赋签已分配的句柄（可空：结论写回跳过，仅返回理解文本）
 * @returns 置换用文本（理解为空时以默认应答兜底）
 */
async function replaceAnimatedImage(imageSource: string, handle: string): Promise<string> {
    const media = resolveAdmittedMedia(handle);
    /** 理解文本（内联 data URI 为一次性上传数据不缓存；URL 来源以原地址为缓存键） */
    let summary = '';
    try {
        summary = await understandAnimatedImage(imageSource, imageSource.startsWith('data:') ? '' : imageSource);
    }
    catch (error) {
        console.error('[感知者] 动态图理解失败，以兜底文本置换:', error);
    }
    if (media && summary.trim().length > 0) storeMediaSummary(media, summary);
    // 句柄优先，理解文本紧随其后（理解为空时以默认应答兜底）
    const body = summary.trim().length > 0 ? `【动态图理解】${summary}` : mediaRoles!.randomDefaultMessage();
    return media ? `${media.handle}${body}` : body;
}

/** 按句柄构造资源化媒体引用（供理解结论写回资源目录） */
function resolveAdmittedMedia(handle: string): { handle: string; id: number } | null {
    const entry = getResourceEntry(handle);
    return entry ? { handle: entry.handle, id: entry.id } : null;
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

/**
 * 视频资源化包装：收进资源目录后返回「句柄 + 理解文本」
 *
 * 资源化失败（目录不可用等）时退化为纯理解文本，保证对话链路不中断。
 *
 * @param videoSource 视频来源（本地路径 / 服务地址 / 远程 URL / data URI）
 * @returns 置换用文本
 */
async function resourceVideoOrFallback(videoSource: string): Promise<string> {
    try {
        const text = await resourceVideo(videoSource, mediaName(videoSource, '.mp4'), videoSource.startsWith('data:') ? 'client-upload' : 'channel');
        if (text) return text;
    }
    catch (error) {
        console.error('[感知者] 视频资源化失败，退化为纯理解文本:', error);
    }
    return understandVideoOrFallback(videoSource);
}

/**
 * 音频资源化包装：收进资源目录后返回「句柄 + 转写文本」
 *
 * @param audioSource 音频来源
 * @returns 置换用文本
 */
async function resourceAudioOrFallback(audioSource: string): Promise<string> {
    try {
        const text = await resourceAudio(audioSource, mediaName(audioSource, '.wav'), audioSource.startsWith('data:') ? 'client-upload' : 'channel');
        if (text) return text;
    }
    catch (error) {
        console.error('[感知者] 音频资源化失败，退化为纯转写文本:', error);
    }
    return understandAudioOrFallback(audioSource);
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
