import { OnlyData, ImageContent, TextContent, PostMessage, GOkeyframe, GOfetch, GOresize } from '../../config/index';
import { fetchDocumentCallback, getFileContent, getPromptFromDatabase, savePromptToDatabase } from '../../hierarchy/index';
import { ModelBuilder, ChatDialogueRole, PainterRole } from '../index';

/** 智能体定义 */
export class AgentDefine {
    /** 构建计划 */
    protected compilePlan: ModelBuilder = new ModelBuilder();
    /** 推理关键词 */
    protected queryKeywords: ModelBuilder = new ModelBuilder();
    /** 情感管理器 */
    protected emotionManager: ModelBuilder = new ModelBuilder();

    /** 书记者角色(编写记忆) */
    protected recorderRole: ModelBuilder = new ModelBuilder();
    /** 摘要者角色(视频摘要) */
    protected summaryRole: ModelBuilder = new ModelBuilder();
    /** 描述者角色(视频描述) */
    protected descriptionRole: ModelBuilder = new ModelBuilder();
    /** 绘图师角色(图片生成) */
    protected painterRole: PainterRole = new PainterRole();
    /** 聊天者角色(用户交互) */
    protected chatDialogueRole: ChatDialogueRole = new ChatDialogueRole();

    /** 嵌入向量 */
    public embedding: ModelBuilder = new ModelBuilder().useEmbedding();
    /** 未读上下文 */
    public unreadContext: PostMessage[] = [];
    /** 未读视频文件 */
    public unreadVideoUrl: string[] = [];
    /** 最终应答 */
    public finalResponse: string = "";
    /** 响应速度 */
    public responseSpeed: number = 0;
    /** 默认应答 */
    public defaultAnswer: string = "月华不知道哦";
    /** 构建智能体 并 初始化各个子模型的系统提示词 */
    protected constructor() {
        // 初始化 全部模型 的 系统提示词
        this.compilePlan.useMultimodal(getFileContent('resources/prompts/compilePlan.md'));
        this.queryKeywords.useMultimodal(getFileContent('resources/prompts/queryKeywords.md'));
        this.emotionManager.useMultimodal(getFileContent('resources/prompts/emotionManager.md'));
        this.recorderRole.useMultimodal(getFileContent('resources/prompts/recorderRole.md'));
        this.summaryRole.useMultimodal(getFileContent('resources/prompts/summaryRole.md'));
        this.descriptionRole.useMultimodal(getFileContent('resources/prompts/descriptionRole.md'));
        // 初始化 自定义配置 信息
        fetchDocumentCallback('lunar_config.json').then(content => OnlyData.customConfig = JSON.parse(content));
        // TODO 初始化 工具调用配置
        // fetchDocumentCallback('resources/toolCall.json').then(content => OnlyData.toolCall = JSON.parse(content));
        // TODO 初始化 聊天记录
        // fetchDocumentCallback('resources/chatRecord.json')
    }
    /**
     * 处理视频文件
     *
     * @param {File} videoUrl - 视频文件对象
     * 
     * @param {string} userNeeds - 用户需求
     * 
     * @returns {Promise<void>} - 处理完成后的 Promise
     */
    protected async analysisVideoFile(videoUrl: string, userNeeds: string): Promise<void> {
        /** 检查是否已处理过该视频 */
        const cachedPrompt = getPromptFromDatabase(videoUrl);
        // 如果视频已处理过,直接添加到未读上下文
        if (cachedPrompt) {
            this.unreadContext.push({ role: 'user', content: cachedPrompt });
            return;
        }
        /** 关键帧提取API响应 */
        const [keyFrames, error] = GOkeyframe(videoUrl, './cache');
        // 检查提取关键帧是否成功
        if (!keyFrames || keyFrames.length === 0 || error) throw new Error('提取关键帧失败');
        /** 沙箱消息数组 */
        const sandboxMessages: Array<TextContent> = [];
        /** 模型对视频总结结果 */
        let videoSummary = '';
        /** 关键帧消息数组 */
        const frameMessages: Array<ImageContent> = keyFrames.map(frame => ({ type: "image_url", image_url: { url: `data:image/jpeg;base64,${frame.data}` } }));
        // 处理关键帧,每20张调用一次模型进行画面总结
        for (let i = 0; i < frameMessages.length; i += 20) {
            /** 当前批次20张关键帧消息*/
            const batchFrames = frameMessages.slice(i, i + 20);
            // 覆写 视频描述模型 上下文
            this.descriptionRole.coverContext({ role: 'user', content: batchFrames });
            /** 调用模型进行画面总结 */
            const summaryRequest = await (await this.descriptionRole.run as Response).json();
            /** 模型总结结果 */
            const summary = summaryRequest?.choices?.[0]?.message?.content;
            // 过滤空字符串和仅包含空格的字符串
            if (summary && summary.trim().length > 0) sandboxMessages.push(summary);
        }
        // 判断是否包含多个批处理片段
        if (sandboxMessages.length > 1) {
            // 覆写 视频摘要模型 上下文
            this.summaryRole.coverContext({ role: 'user', content: sandboxMessages });
            /** 调用模型进行视频总结 */
            const summaryRequest = await (await this.summaryRole.run as Response).json();
            /** 模型视频总结结果 */
            videoSummary = summaryRequest?.choices?.[0]?.message?.content;
        }
        // 如果仅包含一个批处理片段,使用该片段作为总结
        else if (sandboxMessages.length === 1) videoSummary = sandboxMessages[0].text;
        // 否则使用默认应答
        else videoSummary = this.defaultAnswer;
        // 将视频总结结果添加到消息数组
        if (videoSummary) this.unreadContext.push({ role: 'user', content: videoSummary });
        // 如果用户需求非空,添加到消息数组
        if (userNeeds.trim().length > 0) this.unreadContext.push({ role: 'user', content: userNeeds });
        // 缓存处理结果到数据库
        if (videoSummary) savePromptToDatabase(videoUrl, videoSummary);
    }
    /**
     * 遍历未读上下文数组,处理图片文件
     *
     * @returns {Promise<void>} - 处理完成后的 Promise
     */
    public async LiteImageFile(): Promise<void> {
        for (let message of this.unreadContext) {
            // 跳过纯文本消息
            if (typeof message.content === 'string') continue;
            // 遍历消息内容中的每个项
            for (let item of message.content) {
                // 跳过文本项
                if (item.type == 'text') continue;
                // 检查是否为支持的视频文件格式
                if (OnlyData.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format))) {
                    // 处理视频文件
                    await this.analysisVideoFile(item.image_url.url, '');
                }
                else if (!item.image_url.url.startsWith("data:image")) {
                    // 获取图片文件内容
                    const [response, error] = GOfetch({ url: item.image_url.url, execute: { crossDomain: true } });
                    // 检查请求是否成功
                    if (error) throw new Error('获取图片文件失败');
                    // 检查响应是否成功
                    if (!response.ok) throw new Error(`获取图片文件失败: ${response.status} ${response.statusText}`);
                    /** 从响应中获取图片 Blob 对象 */
                    const blob = await response.blob();
                    /** 缩放图片 */
                    const [resizedBlob, error1] = GOresize(blob);
                    // 检查缩放是否成功
                    if (error1) throw new Error('缩放图片失败');
                    // 处理缩放后的图片文件
                    item.image_url.url = resizedBlob.base64;
                }
            }
        }
    }
}