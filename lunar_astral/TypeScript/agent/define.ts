import { GlobalConfig, ImageContent, AudioContent, TextContent, PostMessage, fetchDocumentCallback, getPromptFromKnowledge, savePromptToKnowledge, ModelBuilder, DialogueRole, PainterRole, MusicianRole, LearnerRole, ViewerRole, ActorRole, RandomFloor, OrganizeRole } from '../index';

/** 智能体定义 */
export class AgentDefine {
	/** 全局单例引用，供工具处理函数访问子智能体实例 */
	public static instance: AgentDefine;
	/** 描述者角色(视觉内容描述) */
	public descriptionRole: ModelBuilder = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
	/** 对话者角色(用户交互) */
	public dialogueRole: DialogueRole = new DialogueRole();
	/** 学习者角色(深度调研与信息查证) */
	public learnerRole: LearnerRole = new LearnerRole();
	/** 绘制者角色(图片生成) */
	public painterRole: PainterRole = new PainterRole();
	/** 演奏者角色(音乐创作) */
	public musicianRole: MusicianRole = new MusicianRole();
	/** 观影者角色(视频观看) */
	public viewerRole: ViewerRole = new ViewerRole();
	/** 行动者角色(3D动画/位移/空间感知) */
	public actorRole: ActorRole = new ActorRole();
	/** 组织者角色(组织记忆) */
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
		fetchDocumentCallback('lunar_config.json').then(content => GlobalConfig.customConfig = content);
		// TODO 初始化 聊天记录
		// fetchDocumentCallback('resources/chatRecord.json').then(content => GlobalConfig.chatRecord = content);
	}
	/**
	 * 处理视频文件（观影者智能体）
	 *
	 * 提取关键帧后交由观影者子智能体分批观看，
	 * 生成月华视角的观后感摘要，并缓存结果。
	 *
	 * @param {string} videoUrl - 视频文件路径或URL
	 * @param {string} userNeeds - 用户需求
	 *
	 * @returns {Promise<void>} - 处理完成后的 Promise
	 */
	protected async analysisVideoFile(videoUrl: string, userNeeds: string): Promise<void> {
		// 缓存检查：如果已处理过该视频，直接返回缓存结果
		const cachedPrompt = getPromptFromKnowledge(videoUrl);
		if (cachedPrompt) {
			this.unreadContext.push({ role: 'user', content: cachedPrompt });
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
		const keyframes = images.map((frame: { data: string; timestamp: string }) => ({
			data: frame.data,
			timestamp: frame.timestamp || ''
		}));

		// 第三步：调用观影者智能体观看视频
		console.log('[观影者] 开始观看视频...');
		const videoSummary = await this.viewerRole.watchVideo(keyframes);
		console.log('[观影者] 视频观看完成');

		// 第四步：将观后感添加到未读上下文
		if (videoSummary && videoSummary.trim().length > 0) {
			this.unreadContext.push({ role: 'user', content: videoSummary });
		} else {
			// 兜底：使用默认应答
			this.unreadContext.push({
				role: 'user',
				content: this.defaultAnswers[RandomFloor(0, this.defaultAnswers.length - 1)]
			});
		}

		// 如果用户需求非空，追加到上下文
		if (userNeeds.trim().length > 0) {
			this.unreadContext.push({ role: 'user', content: userNeeds });
		}

		// 第五步：缓存观后感
		if (videoSummary) {
			savePromptToKnowledge(videoUrl, videoSummary);
			console.log('[观影者] 观后感已缓存');
		}
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
				else if (item.image_url && GlobalConfig.videoFormatsExtensions.some(format => item.image_url.url.toLowerCase().endsWith(format))) {
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
	/**
	 * 一键导出所有子智能体的运行时上下文到本地文件（覆写模式）
	 *
	 * 将对话者、学习者、绘制者、演奏者、组织者的上下文分别导出为独立 JSON 文件，
	 * 同时生成一份汇总索引文件，方便统一查看所有智能体状态。
	 *
	 * @param outputDir 输出目录（默认 d:\Lunar_Astral_Agents\local_data\debug）
	 * @returns 导出文件路径数组
	 */
	public dumpAllContexts(outputDir?: string): string[] {
		// 调试模式关闭时跳过导出
		if (!GlobalConfig.debugMode) return [];

		const dir = outputDir || 'd:\\Lunar_Astral_Agents\\local_data\\debug';
		const results: string[] = [];

		// 对话者
		const dialoguePath = this.dialogueRole.dumpContext('对话者', `${dir}\\agent_debug_对话者.json`);
		if (dialoguePath) results.push(dialoguePath);

		// 学习者（需要对话历史和未读上下文）
		const learnerPath = this.learnerRole.dumpContext(
			this.dialogueRole.messages,
			this.unreadContext,
			`${dir}\\agent_debug_学习者.json`
		);
		if (learnerPath) results.push(learnerPath);

		// 绘制者
			const painterPath = this.painterRole.dumpContext('绘制者', `${dir}\\agent_debug_绘制者.json`);
			if (painterPath) results.push(painterPath);

			// 演奏者
			const musicianPath = this.musicianRole.dumpContext('演奏者', `${dir}\\agent_debug_演奏者.json`);
			if (musicianPath) results.push(musicianPath);

		// 观影者
		const viewerPath = this.viewerRole.dumpContext('观影者', `${dir}\\agent_debug_观影者.json`);
		if (viewerPath) results.push(viewerPath);

		// 行动者
		const actorPath = this.actorRole.dumpContext('行动者', `${dir}\\agent_debug_行动者.json`);
		if (actorPath) results.push(actorPath);

		// 组织者
			const organizePath = this.organizeRole.dumpContext('组织者', `${dir}\\agent_debug_组织者.json`);
		if (organizePath) results.push(organizePath);

		// 生成汇总索引文件
		const indexData = {
			timestamp: new Date().toLocaleString('zh-CN', {
				year: 'numeric', month: '2-digit', day: '2-digit',
				hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
			}),
			unreadContextCount: this.unreadContext.length,
			unreadVideoUrlCount: this.unreadVideoUrl.length,
			unreadRecordsCount: GlobalConfig.unreadRecords.length,
			finalResponse: this.finalResponse,
			exportedFiles: results,
		};
		const indexPath = `${dir}\\agent_debug_index.json`;
		const [, indexError] = saveDebugFile(indexPath, JSON.stringify(indexData, null, 2));
		if (!indexError) results.push(indexPath);

		console.log(`[智能体] 已导出 ${results.length} 个上下文文件到 ${dir}`);
		return results;
	}
}