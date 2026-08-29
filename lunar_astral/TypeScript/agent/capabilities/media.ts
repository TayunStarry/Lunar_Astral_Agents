import { getPromptFromKnowledge, savePromptToKnowledge, ImageContent, TextContent, GlobalConfig } from '../../index';
import { descriptionRole, viewerRole, randomDefaultMessage } from '../roles/roles';

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
