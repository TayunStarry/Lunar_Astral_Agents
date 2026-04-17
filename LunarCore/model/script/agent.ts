import { getFileContent, fetchDocumentCallback } from '../../hierarchy/index';
import { OnlyData, PostMessage, ChatCache } from '../../config/index';
import { ModelBuilder, AgentDefine, ChatDialogueRole } from '../index';

/** 月华智能体 */
class LunarAgent extends AgentDefine {
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
        await this.chatDialogueRole.callMultimediaAndToolParsing(cache, this);
        // 返回最终应答
        return this.finalResponse;
    }
    /** 构建智能体 并 初始化各个子模型的系统提示词 */
    public constructor() { super(); }
}

export default LunarAgent;