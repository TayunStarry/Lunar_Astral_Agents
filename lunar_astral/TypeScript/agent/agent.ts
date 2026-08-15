import { ChatCache, RandomFloor, AgentDefine, PostMessageRole, GlobalConfig, parseContent, checkDueItems, MessageContent } from '../index';

/** 月华智能体 */
class LunarAgent extends AgentDefine {
	/** 发言权重 */
	protected speakWeight: number = 1;
	/** 沉默计数（连续不允许发言的循环次数） */
	protected silenceCount: number = 0;
	/** 是否正在思考中 */
	protected reasoningInProgress: boolean = false;
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
	/** 思考循环事件 */
	protected async thoughtLoopTickEvent(): Promise<void> {
		// 如果正在思考中，直接返回
		if (this.reasoningInProgress) return;
		// 思考循环开始
		try {
			// 标记为思考中
			this.reasoningInProgress = true;
			// 查询 LTPX 工具状态，同步加载/卸载
			this.syncLTPXToolStatus();
			// 拉取外部消息
			await this.pullExternalMessages();
			// 检查计划表到期项，将到期计划内容写入上下文
			for (const item of checkDueItems()) {
				this.unreadContext.push({ role: 'user', content: `[计划提醒] 预约时间已到，请执行以下计划：${item.content}` })
			}
			/** 消息长度 */
			const messageLength = this.unreadContext.length + this.unreadVideoUrl.length;
			/** 消息类型 */
			const messageType = messageLength === 0 ? 'response' : 'active';
			/** 是否允许发言 */
			const allowSpeak = RandomFloor(15, 100) < this.speakWeight;
			// 如果消息长度为0，且不允许发言，跳过当前循环
			if (messageLength === 0 && !allowSpeak) {
				// 沉默计数+1（上限100）
				this.silenceCount = Math.min(this.silenceCount + 1, 100);
				// 标记为思考完成
				this.reasoningInProgress = false;
				// 进入下一次循环
				return;
			}
			// 如果消息长度为0，且允许发言，但沉默计数不足30，跳过当前循环
			if (messageLength === 0 && allowSpeak && this.silenceCount < 30) {
				// 沉默计数+1（上限100）
				this.silenceCount = Math.min(this.silenceCount + 1, 100);
				// 标记为思考完成
				this.reasoningInProgress = false;
				// 进入下一次循环
				return;
			}
			// 允许发言，重置沉默计数和发言权重
			this.silenceCount = 0;
			// 如果消息长度为0，发言权重设为0
			if (messageLength === 0) this.speakWeight = 0;
			// 批量处理视频文件
			await this.batchProcessVideoFiles();
			// 创建消息（对话者作为主智能体，消费上下文并生成最终应答）
			await this.createChatMessage();
			// 如果消息响应为空，抛出异常
			if (!this.finalResponse.trim().length) throw new Error('消息响应为空');
			// 如果未读记录数超过30条，调用记忆者处理历史记录
			if (GlobalConfig.unreadRecords.length > 30) this.memoryRole.memorizeHistoricalRecords();
			/** 解析原始文本：拆分思考区、代码块、动作区、情感区、正文切片（含display和tts双版本） */
			const { thinkingBlocks, codeBlocks, actionBlocks, emotionBlocks, textChunks } = parseContent(this.finalResponse);
			// 如果正文切片为空，抛出异常
			if (!textChunks.length) throw new Error('清洗后的文本为空');
			// 动作区与情感区仅打印到终端，不发送到前端、不参与语音合成
			if (actionBlocks.length) console.log('[动作区]', actionBlocks.join(' | '));
			if (emotionBlocks.length) console.log('[情感区]', emotionBlocks.join(' | '));
			// 第一步：按顺序逐一发送思考区内容（不参与语音合成）
			for (const thinking of thinkingBlocks) {
				pushContext(messageType, thinking, '');
			}
			// 第二步：按顺序逐一发送代码块内容（不参与语音合成）
			for (const code of codeBlocks) {
				pushContext(messageType, code, '');
			}
			// 第三步：按顺序逐一发送正文切片，display用于显示，tts用于合成语音
			for (const chunk of textChunks) {
				/** 语音合成结果 */
				let audio = '';
				/** 语音合成 */
				const [audioData, err] = tts(chunk.tts);
				// 如果语音合成成功，将结果赋值给audio
				if (!err && audioData) audio = audioData;
				// 推送消息（包含显示内容和语音数据）
				pushContext(messageType, chunk.display, audio);
			}
			// 思考链结尾：导出所有子智能体上下文快照
			this.dumpAllContexts();
		}
		catch (error) {
			/** 获取提示音数据 */
			const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
			// 如果读取提示音失败，打印错误信息
			if (readErr) console.error('读取提示音失败:', readErr);
			// 打印错误信息
			console.error((error as Error).message, ' || ', (error as Error).stack);
			// 推送兜底消息
			pushContext('active', this.randomDefaultMessage, promptSound);
			// 重置智能体状态
			this.resetAgentState();
		}
		// 标记为思考完成
		this.reasoningInProgress = false;
	}
	/** 错误累积达阈值后重置智能体状态 */
	protected resetAgentState(): void {
		// 清空全部子智能体的messages
		this.descriptionRole.coverContext([]);
		this.dialogueRole.coverContext([]);
		this.learnerRole.messages = [];
		this.painterRole.coverContext([]);
		this.musicianRole.coverContext([]);
		this.viewerRole.coverContext([]);
		this.actorRole.coverContext([]);
		this.memoryRole.coverContext([]);
		// 清除主智能体的unreadContext和unreadVideoUrl
		this.unreadContext = [];
		this.unreadVideoUrl = [];
	}
	/** 同步 LTPX 工具状态：查询 Go 层状态并执行加载/卸载 */
	protected syncLTPXToolStatus(): void {
		try {
			const statusJSON = getLTPXToolStatus();
			if (!statusJSON || statusJSON === '{}') return;
			const status = JSON.parse(statusJSON);
			// 处理待加载和待卸载
			if ((status.pendingLoads && status.pendingLoads.length > 0) ||
				(status.pendingUnloads && status.pendingUnloads.length > 0)) {
				processLTPXChanges(statusJSON);
			}
		} catch (e) {
			console.error('LTPX 工具状态同步失败:', e);
		}
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
	public writeMessage(role: PostMessageRole, messages: Array<MessageContent>) {
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
	public async messageWrite(role: PostMessageRole, messages: Array<MessageContent>, timeout: number) {
		// 等待指定超时时间
		await new Promise(resolve => setTimeout(resolve, timeout));
		// 如果消息数组非空，写入消息
		if (messages.length > 0) this.writeMessage(role, messages);
	}
	/** 构建智能体 并 初始化各个子模型的系统提示词 */
	public constructor() {
		super();
		// 设置全局单例引用，供工具处理函数访问子智能体实例
		AgentDefine.instance = this;
		//this.thinkingChainProcess();
		setInterval(() => this.thoughtLoopTickEvent(), 1000);
	}
}
/** 初始化月华智能体实例 */
const AgentRuntime = new LunarAgent();
/** 构建随机的问候语和话题 */
function buildRandomEntranceLines(): string {
	/** 初始化问候语变体（按时段划分，用户 → 月华） */
	const initializationGreetings: Record<string, string[]> = {
		morning: ['月华，早上好呀~', '早安呀，月华~', '月华，起床啦~'],
		afternoon: ['月华，下午好~', '月华，中午好呀~', '月华在吗~'],
		evening: ['月华，晚上好呀~', '月华，晚上好~', '月华在不在呀~'],
		night: ['月华，这么晚还没睡吗~', '月华，夜深啦~', '月华还在吗~'],
	};
	/** 初始化话题变体（用户 → 月华） */
	const initializationTopics: string[] = [
		'今天陪我聊聊天吧',
		'今天有什么新鲜事吗',
		'准备好了吗，我们开始吧',
		'想你了，月华~',
		'今天也要元气满满哦',
		'一起来做点有趣的事吧',
	];
	/** 当前小时 */
	const currentHour = new Date().getHours();
	/** 根据当前时段选择的问候语池 */
	let greetingPool = initializationGreetings.night;
	if (currentHour >= 5 && currentHour < 11) greetingPool = initializationGreetings.morning;
	else if (currentHour >= 11 && currentHour < 17) greetingPool = initializationGreetings.afternoon;
	else if (currentHour >= 17 && currentHour < 23) greetingPool = initializationGreetings.evening;
	/** 随机选择一个问候语 */
	const greeting = greetingPool[RandomFloor(0, greetingPool.length - 1)];
	/** 随机选择一个话题 */
	const topic = initializationTopics[RandomFloor(0, initializationTopics.length - 1)];
	// 拼接问候语和话题
	return greeting + topic;
};
/** 初始化消息（按时段的随机问候语 + 随机话题拼接） */
const initializationMessage: MessageContent[] = [{ type: 'text', text: buildRandomEntranceLines() }];
// 向模型发送用户消息
AgentRuntime.messageWrite('user', initializationMessage, 1500);
