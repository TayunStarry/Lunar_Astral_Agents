import { ChatCache } from '../../config/index';
import { RandomFloor } from '../../math/index';
import { AgentDefine } from '../index';

/** 月华智能体 */
class LunarAgent extends AgentDefine {
    /** 消息权重 */
    protected messageWeight: number = 1;
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
    /** 思考链处理 */
    public async thinkingChainProcess() {
        while (true) {
            console.log('思考链处理');
            // 等待1秒
            await new Promise(resolve => setTimeout(resolve, 1000));
            /** 消息长度 */
            const messageLength = this.unreadContext.length + this.unreadVideoUrl.length;
            // 如果消息长度为0，且随机数小于等于消息权重，继续循环
            if (messageLength === 0 && RandomFloor(0, 100) <= this.messageWeight) continue;
            // 等待1秒
            await new Promise(resolve => setTimeout(resolve, 1000));
            // 批量处理视频文件
            await this.batchProcessVideoFiles();
            // 等待1秒
            await new Promise(resolve => setTimeout(resolve, 1000));
            // 创建消息
            await this.createChatMessage();
            // 等待1秒
            await new Promise(resolve => setTimeout(resolve, 1000));
            console.log(this.finalResponse);
        }
    }
}

//export default LunarAgent;
// 100ms 后执行 new LunarAgent()
setTimeout(awakenAgent, 1000);
async function awakenAgent() {
    console.log('智能体系统已唤醒');
    const agent = new LunarAgent();
    setTimeout(() => agent.unreadContext.push({ role: 'user', content: '你好' }), 5000);
    setTimeout(() => console.log(JSON.stringify(agent.unreadContext)), 5000);
    setTimeout(() => agent.unreadContext.push({ role: 'user', content: '你叫什么名字' }), 10000);
    setTimeout(() => console.log(JSON.stringify(agent.unreadContext)), 10000);
    setTimeout(() => agent.unreadContext.push({ role: 'user', content: '你的哥哥叫什么名字' }), 15000);
    setTimeout(() => console.log(JSON.stringify(agent.unreadContext)), 15000);
    await agent.thinkingChainProcess();
}