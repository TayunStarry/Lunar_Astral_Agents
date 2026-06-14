import { ChatCache, RandomFloor, AgentDefine, ImageContent, TextContent, PostMessageRole, OnlyData, parseContent, checkDueItems } from '../index';

/** 月华智能体 */
class LunarAgent extends AgentDefine {
	/** 发言权重 */
	protected speakWeight: number = 1;
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
	/** 创建聊天消息 */
	public async createChatMessage(): Promise<string> {
		/** 初始化聊天缓存 */
		const cache: ChatCache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
		// 发送请求并获取响应
		await this.dialogueRole.callMultimediaAndToolParsing(cache, this);
		// 减少发言权重
		this.speakWeight--;
		// 返回最终应答
		return this.finalResponse;
	}
	/** 思考链处理 */
	protected async thinkingChainProcess() {
		/** 错误计数 */
		let errorCount = 0;
		// 循环处理
		while (true) {
			try {
				// 拉取外部消息
				await this.pullExternalMessages();
				// 检查计划表到期项，将到期计划内容写入上下文
				const dueItems = checkDueItems();
				// 遍历到期计划项，将内容写入上下文
				for (const item of dueItems) {
					this.unreadContext.push({ role: 'tool', content: `[计划提醒] 预约时间已到，请执行以下计划：${item.content}` });
				}
				/** 消息长度 */
				const messageLength = this.unreadContext.length + this.unreadVideoUrl.length;
				/** 消息类型 */
				const messageType = messageLength === 0 ? 'response' : 'active';
				/** 是否允许发言 */
				const allowSpeak = RandomFloor(15, 100) < this.speakWeight;
				// 如果消息长度为0，且不允许发言，继续循环
				if (messageLength === 0 && !allowSpeak) {
					// 等待1秒
					await new Promise(resolve => setTimeout(resolve, 1000));
					// 进入下一次循环
					continue;
				}
				// 如果消息长度为0，且允许发言，重置发言权重
				else if (messageLength == 0 && allowSpeak) this.speakWeight = 0;
				// 批量处理视频文件
				await this.batchProcessVideoFiles();
				// 如果包含图像生成关键词，调用画家角色执行绘画循环
				this.painterRole.createImageRendering(this);
				// 创建消息
				await this.createChatMessage();
				// 如果消息响应为空，抛出异常
				if (!this.finalResponse.trim().length) throw new Error('消息响应为空');
				// 成功响应时重置错误计数
				else errorCount = 0;
				// 如果未读记录数超过10条，调用编纂者组织历史记录
				if (OnlyData.unreadRecords.length > 10) {
					setTimeout(() => this.organizeRole.organizeHistoricalRecords(), 0);
				}
				/** 解析原始文本：拆分思考区、代码块、正文切片 */
				const { thinkingBlocks, codeBlocks, textChunks } = parseContent(this.finalResponse);
				// 如果正文切片为空，抛出异常
				if (!textChunks.length) throw new Error('清洗后的文本为空');
				// 第一步：按顺序逐一发送思考区内容（不参与语音合成）
				for (const thinking of thinkingBlocks) {
					pushContext(messageType, thinking, '');
				}
				// 第二步：按顺序逐一发送代码块内容（不参与语音合成）
				for (const code of codeBlocks) {
					pushContext(messageType, code, '');
				}
				// 第三步：按顺序逐一发送正文切片，合成语音并推送
				for (const chunk of textChunks) {
					/** 语音合成结果 */
					let audio = '';
					try {
						const [audioData, err] = tts(chunk);
						if (!err && audioData) audio = audioData;
					}
					catch (e) {
						console.error(`TTS合成异常: [${chunk}]`, e);
					}
					pushContext(messageType, chunk, audio);
				}
			}
			catch (error) {
				/** 获取提示音数据 */
				const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
				// 如果读取提示音失败，打印错误信息
				if (readErr) console.error('读取提示音失败:', readErr);
				// 打印错误信息
				console.error((error as Error).message, ' || ', (error as Error).stack);
				// 错误次数增加
				errorCount++;
				// 推送兜底消息
				pushContext('active', this.randomDefaultMessage, promptSound);
				// 错误累积达阈值，重置状态并重新循环
				if (errorCount >= 3) {
					this.resetAgentState();
					errorCount = 0;
					continue;
				}
			}
		}
	}
	/** 错误累积达阈值后重置智能体状态 */
	protected resetAgentState(): void {
		// 清空全部子智能体的messages
		this.queryKeywords.coverContext([]);
		this.emotionManager.coverContext([]);
		this.recorderRole.coverContext([]);
		this.summaryRole.coverContext([]);
		this.descriptionRole.coverContext([]);
		this.dialogueRole.coverContext([]);
		this.painterRole.coverContext([]);
		this.organizeRole.coverContext([]);
		// 清除主智能体的unreadContext和unreadVideoUrl
		this.unreadContext = [];
		this.unreadVideoUrl = [];
	}
	/** 拉取外部消息 */
	protected async pullExternalMessages() {
		// 合并消息
		pullContext().forEach(message => this.writeMessage(message.role, message.content))
		// 合并视频URL
		pullVideoUrl().forEach(videoUrl => { this.writeVideoUrl(videoUrl); })
		// 等待1秒
		await new Promise(resolve => setTimeout(resolve, 1000));
	}
	/** 写入消息 */
	public writeMessage(role: PostMessageRole, messages: Array<ImageContent | TextContent>) {
		// 从外部写入消息
		this.unreadContext.push({ role, content: messages });
		// 增加随机的发言权重
		this.speakWeight += RandomFloor(1, 3);
		// 如果消息是字符串，将其转换为文本消息
		if (typeof messages === 'string') messages = [{ type: 'text', text: messages }];
		// 打印文本消息
		messages.forEach(message => { if (message.type === 'text') console.log(message.text); })
	}
	/** 写入视频文件 */
	public writeVideoUrl(videoUrl: string) {
		console.log('写入视频文件:' + videoUrl);
		// 从外部写入视频文件
		this.unreadVideoUrl.push(videoUrl);
		// 增加随机的发言权重
		this.speakWeight += RandomFloor(1, 3);
	}
	/** 测试消息写入 */
	public async testMessageWrite(role: PostMessageRole, messages: Array<ImageContent | TextContent>, timeout: number) {
		// 等待指定超时时间
		await new Promise(resolve => setTimeout(resolve, timeout));
		// 如果消息数组非空，写入消息
		if (messages.length > 0) this.writeMessage(role, messages);
	}
	/** 构建智能体 并 初始化各个子模型的系统提示词 */
	public constructor() { super(); this.thinkingChainProcess(); }
}

const AgentExample = new LunarAgent();
/** 测试消息写入 */
const message: Array<ImageContent | TextContent> = [
	{
		type: 'text',
		text: '你好呀~'
	}
];
AgentExample.testMessageWrite('user', message, 1500);
