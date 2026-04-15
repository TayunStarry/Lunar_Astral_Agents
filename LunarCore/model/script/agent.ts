import { getFileContent, fetchDocumentCallback } from '../../FileSystem/index';
import { OnlyData, PostMessage, ChatCache } from '../../config/index';
import { ModelBuilder, AgentSkill } from '../index';

/** 智能体原型 */
export class ProtoAgent {
    /** 构建计划 */
    protected compilePlan: ModelBuilder = new ModelBuilder();
    /** 推理关键词 */
    protected queryKeywords: ModelBuilder = new ModelBuilder();
    /** 情感管理器 */
    protected emotionManager: ModelBuilder = new ModelBuilder();
    /** 视频摘要 */
    protected videoSummary: ModelBuilder = new ModelBuilder();
    /** 视频描述 */
    protected videoDescription: ModelBuilder = new ModelBuilder();
    /** 聊天回复 */
    protected chatReply: ModelBuilder = new ModelBuilder().useMultimodal();
    /** 嵌入向量 */
    public embedding: ModelBuilder = new ModelBuilder().useEmbedding();
    /** 未读上下文 */
    protected unreadContext: PostMessage[] = [];
    /** 未读视频文件 */
    protected unreadVideoUrl: string[] = [];
    /** 最终应答 */
    public finalResponse: string = "";
    /** 响应速度 */
    public responseSpeed: number = 0;
    /** 默认应答 */
    public defaultAnswer: string = "月华不知道哦";
    /** 构建智能体 并 初始化各个子模型的系统提示词 */
    constructor() {
        // 初始化 全部模型 的 系统提示词
        this.compilePlan.useMultimodal(getFileContent('/read/resources/prompts/compilePlan.md'));
        this.queryKeywords.useMultimodal(getFileContent('/read/resources/prompts/queryKeywords.md'));
        this.emotionManager.useMultimodal(getFileContent('/read/resources/prompts/emotionManager.md'));
        this.videoSummary.useMultimodal(getFileContent('/read/resources/prompts/videoSummary.md'));
        this.videoDescription.useMultimodal(getFileContent('/read/resources/prompts/videoDescription.md'));
        // 初始化 自定义配置 信息
        fetchDocumentCallback('resources/custom_config.json').then(content => OnlyData.customConfig = JSON.parse(content));
        // TODO 初始化 工具调用配置
        // fetchDocumentCallback('resources/toolCall.json').then(content => OnlyData.toolCall = JSON.parse(content));
        // TODO 初始化 聊天记录
        // fetchDocumentCallback('resources/chatRecord.json')
    }
}

/** 月华智能体 */
class LunarAgent extends AgentSkill {
    /**
     * 批量处理视频文件
     *
     * @param {string} [userNeeds] - 用户需求
     * 
     * @returns {Promise<void>} - 处理完成后的 Promise
     */
    public async batchProcessVideoFiles(userNeeds?: string): Promise<void> {
        // 如果未读视频文件数组为空，直接返回
        if (this.unreadVideoUrl.length === 0) return;
        //  遍历未读视频文件数组
        for (const videoUrl of this.unreadVideoUrl) {
            try {
                // 处理视频文件
                await this.analysisVideoFile(videoUrl, userNeeds || '');
                // 等待1秒，避免对服务器造成过大压力
                await new Promise(resolve => setTimeout(resolve, 1000));
            }
            catch (error) { continue; }
        }
        // 清空未读视频文件数组
        this.unreadVideoUrl = [];
    }
    /**
     * 创建消息
     *
     * @returns {Promise<string>} - 最终应答
     */
    public async createChatMessage(): Promise<string> {
        /** 初始化聊天缓存 */
        const cache: ChatCache = {
            currentToolCallIndex: -1,
            currentFunctionArgs: '',
            currentFunctionName: '',
            descriptionContent: '',
            thinkingContent: '',
            currentToolCall: null,
            toolCalls: [],
        };
        // 发送请求并获取响应
        await this.callMultimediaAndToolParsing(cache);
        // 返回最终应答
        return this.finalResponse;
    }
}

export default LunarAgent;