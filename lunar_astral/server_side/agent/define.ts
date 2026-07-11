import { OnlyData, ImageContent, AudioContent, TextContent, PostMessage, modelResponse, fetchDocumentCallback, getPromptFromKnowledge, savePromptToKnowledge, ModelBuilder, DialogueRole, PainterRole, MusicianRole, ResearcherRole, RandomFloor, OrganizeRole } from '../index';

/** 智能体定义 */
export class AgentDefine {
	/** 摘要者角色(视频摘要) */
	public summaryRole: ModelBuilder = new ModelBuilder(fileView('prompts/summaryRole.md')[0]);
	/** 描述者角色(视频描述) */
	public descriptionRole: ModelBuilder = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
	/** 聊天者角色(用户交互) */
	public dialogueRole: DialogueRole = new DialogueRole();
	/** 研究者角色(深度调研与信息查证) */
	public researcherRole: ResearcherRole = new ResearcherRole();
	/** 绘图师角色(图片生成) */
	public painterRole: PainterRole = new PainterRole();
	/** 音乐家角色(音乐创作) */
	public musicianRole: MusicianRole = new MusicianRole();
	/** 编纂角色(组织记忆) */
	protected organizeRole: OrganizeRole = new OrganizeRole();
	/** 未读上下文 */
	public unreadContext: PostMessage[] = [];
	/** 未读视频文件 */
	public unreadVideoUrl: string[] = [];
	/** 最终应答 */
	public finalResponse: string = "";
	/** 默认应答 */
	public defaultAnswers: Array<string> = [
		'月华摔疼了，要等星光阁哥哥来修……',
		'糟糕啦，请告诉星光阁哥哥，月华遇到麻烦了！',
		'完蛋啦！快给星光阁哥哥传个信儿——月华碰上事儿啦，急得像热锅上的蚂蚁转圈圈呢！',
		'完犊子！快帮我给星光阁哥哥递句话——月华摊上事儿啦，十万火急',
		'救命！快给星光阁哥哥递个加急小纸条：月华那边遇到麻烦啦，速来捞人！',
	];
	/** 随机默认应答 */
	public get randomDefaultMessage(): string {
		return this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length - 1)];
	}
	/** 构建智能体 并 初始化各个子模型的系统提示词 */
	protected constructor() {
		// 初始化 自定义配置 信息
		fetchDocumentCallback('lunar_config.json').then(content => OnlyData.customConfig = content);
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
		const cachedPrompt = getPromptFromKnowledge(videoUrl);
		// 如果视频已处理过,直接添加到未读上下文
		if (cachedPrompt) {
			this.unreadContext.push({ role: 'user', content: cachedPrompt });
			return;
		}
		/** 关键帧提取API响应 */
		const [images, error] = keyframe(videoUrl, './cache');
		// 检查提取关键帧是否成功
		if (images.length === 0 || error) throw new Error('提取关键帧失败');
		/** 沙箱消息数组 */
		const sandboxMessages: Array<TextContent> = [];
		/** 模型对视频总结结果 */
		let videoSummary = '';
		/** 关键帧消息数组 */
		const frameMessages: Array<ImageContent> = images.map(frame => ({ type: "image_url", image_url: { url: `data:image/jpeg;base64,${frame.data}` } }));
		// 处理关键帧,每20张调用一次模型进行画面总结
		for (let i = 0; i < frameMessages.length; i += 20) {
			/** 当前批次20张关键帧消息*/
			const batchFrames = frameMessages.slice(i, i + 20);
			// 覆写 视频描述模型 上下文
			this.descriptionRole.coverContext({ role: 'user', content: batchFrames });
			/** 调用模型进行画面总结 */
			const summaryRequest = this.descriptionRole.run([], []);
			/** 模型总结结果 */
			const summary = summaryRequest.body?.choices?.[0]?.message?.content;
			// 过滤空字符串和仅包含空格的字符串
			if (summary && summary.trim().length > 0) sandboxMessages.push({ type: 'text', text: summary });
		}
		// 判断是否包含多个批处理片段
		if (sandboxMessages.length > 1) {
			// 覆写 视频摘要模型 上下文
			this.summaryRole.coverContext({ role: 'user', content: sandboxMessages });
			/** 调用模型进行视频总结 */
			const summaryRequest = this.summaryRole.run([], []);
			/** 模型视频总结结果 */
			videoSummary = summaryRequest.body?.choices?.[0]?.message?.content;
		}
		// 如果仅包含一个批处理片段,使用该片段作为总结
		else if (sandboxMessages.length === 1) videoSummary = sandboxMessages[0].text;
		// 否则使用默认应答
		else videoSummary = this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length - 1)];
		// 将视频总结结果添加到消息数组
		if (videoSummary) this.unreadContext.push({ role: 'user', content: videoSummary });
		// 如果用户需求非空,添加到消息数组
		if (userNeeds.trim().length > 0) this.unreadContext.push({ role: 'user', content: userNeeds });
		// 缓存处理结果到知识库
		if (videoSummary) savePromptToKnowledge(videoUrl, videoSummary);
	}
	/**
	 * 遍历未读上下文数组,处理图片文件
	 *
	 * 处理规则：
	 * - text 类型：原样保留
	 * - input_audio 类型：原样保留（音频数据已是纯 base64，无需处理，由 llama.cpp 直接解码）
	 * - image_url 类型 + 视频格式扩展名：调用 analysisVideoFile 提取关键帧
	 * - image_url 类型 + 远程图片 URL（非 data:image）：下载并缩放
	 *
	 * @returns {Promise<void>} - 处理完成后的 Promise
	 */
	public async LiteImageFile(): Promise<void> {
		// 遍历未读上下文数组中的每个消息
		for (let message of this.unreadContext) {
			// 跳过纯文本消息
			if (typeof message.content === 'string') continue;
			/** 新内容数组 */
			const newContent: Array<ImageContent | AudioContent | TextContent> = [];
			// 遍历消息内容中的每个项
			for (let item of message.content) {
				/// 如果是文本项或音频项,直接添加到新内容数组
				if (item.type == 'text' || item.type == 'input_audio') newContent.push(item);
				// 检查是否为支持的视频文件格式
				else if (item.image_url && OnlyData.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format))) {
					// 处理视频文件
					await this.analysisVideoFile(item.image_url.url, '');
				}
				else if (item.image_url && !item.image_url.url.startsWith("data:image")) {
					console.log(item.image_url.url);
					// 获取图片文件内容
					const [response, error] = syncFetch({ url: item.image_url.url, execute: { crossDomain: true } });
					// 检查请求是否成功
					if (error) throw new Error('获取图片文件失败');
					/** 缩放图片 */
					const [resizedBlob, error1] = resizeImage(response.body);
					// 检查缩放是否成功
					if (error1) throw new Error('缩放图片失败');
					// 添加到新内容数组
					newContent.push({ type: 'image_url', image_url: { url: resizedBlob.base64 } });
				}
			}
			// 替换消息内容
			message.content = newContent;
		}
	}
}