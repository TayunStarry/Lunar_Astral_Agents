import { ToolCall, PostMessage,RandomFloor, ModelBuilder, modelResponse, ToolCallItem, AgentDefine } from '../index';

/**
 * 音乐创作参数接口
 * 
 * 用于 LLM 调用 compose_music 工具时传递音乐创作所需的完整参数。
 */
export interface ComposeMusicParams {
	/** 音乐作品标题 */
	title: string;
	/** 使用的乐器列表，多个乐器用逗号分隔 */
	instruments?: string;
	/** 演奏速度（BPM） */
	tempo?: number;
	/** 音乐段落结构描述 */
	structure?: string;
	/** 调式，如 C大调、a小调 */
	key?: string;
	/** 拍号，如 4/4、3/4 */
	meter?: string;
	/** ABC记谱法格式的完整乐谱 */
	abc_notation: string;
}

/** 音乐创作角色提示词 */
class MusicPrompt extends ModelBuilder {
	/** 默认乐器选择 */
	protected defaultInstruments = [
		'钢琴',
		'小提琴',
		'长笛',
		'大提琴',
		'吉他',
		'竖琴',
		'单簧管',
		'双簧管',
	];
	/** 默认音乐风格 */
	protected defaultStyles = [
		'古典',
		'浪漫',
		'巴洛克',
		'现代简约',
		'民谣',
		'轻音乐',
		'爵士',
		'新世纪',
	];
	/** 默认节拍 */
	protected defaultMeters = ['4/4', '3/4', '6/8', '2/4'];
	/** 默认调式 */
	protected defaultKeys = ['C', 'G', 'D', 'F', 'a', 'e', 'd'];
}

/**
 * 音乐家工具链
 * 
 * 继承 MusicPrompt，提供音乐创作工具定义与处理逻辑。
 * 负责接收 LLM 的工具调用请求，生成 ABC 记谱法乐谱并推送到前端。
 */
class MusicToolchain extends MusicPrompt {
	/** 音乐创作工具定义 */
	protected musicTool: ToolCall[] = [
		{
			type: "function",
			function: {
				name: "compose_music",
				description: "创作音乐作品并生成ABC记谱法格式的乐谱。ABC记谱法是一种基于文本的音乐记谱格式，使用字母表示音符。请务必生成完整的、可直接播放的ABC乐谱，确保音符时值、小节线和调号正确。",
				parameters: {
					type: "object",
					properties: {
						"title": {
							type: "string",
							description: "音乐作品标题"
						},
						"instruments": {
							type: "string",
							description: "使用的乐器列表，多个乐器用逗号分隔，如'钢琴,小提琴'"
						},
						"tempo": {
							type: "number",
							description: "演奏速度（BPM），默认120"
						},
						"structure": {
							type: "string",
							description: "音乐段落结构描述，例如：'前奏(4小节)-主旋律(8小节)-副歌(8小节)-尾声(4小节)'"
						},
						"key": {
							type: "string",
							description: "调式，如 C、G、D、F、a、e、d"
						},
						"meter": {
							type: "string",
							description: "拍号，如 4/4、3/4、6/8"
						},
						"abc_notation": {
							type: "string",
							description: `ABC记谱法格式的完整乐谱。必须严格遵循ABC记谱法规范：

标题行格式:
X:1
T:作品标题
M:拍号
L:默认音符时值(如 1/8 表示八分音符)
Q:速度标记(如 1/4=120)
K:调号

音符与音高: 使用CDEFGAB表示音名，小写字母表示高八度，后面跟逗号表示低八度(如 C, D, E,)。升半音用^前缀(如 ^C)，降半音用_前缀(如 _B)。
时值: 数字后缀表示时值倍数，如 C2 表示两倍时值的C音，C/2 表示一半时值。
小节线: | 分隔小节，|| 表示双小节线，|] 表示结束。
休止符: z 表示休止符，时值规则同音符。

示例:
X:1
T:月光小夜曲
M:4/4
L:1/8
Q:1/4=100
K:C
C2 E2 G2 c'2 | e'2 d'2 c'2 G2 | E2 C2 D2 E2 | C8 |]`
						}
					},
					required: [
						"title",
						"abc_notation"
					]
				}
			}
		}
	];

	/**
	 * 处理音乐创作工具调用
	 * 
	 * 接收 LLM 生成的 ABC 记谱法乐谱，验证格式后推送到前端进行渲染和播放。
	 * 
	 * @param {ComposeMusicParams} args - 音乐创作参数
	 * 
	 * @returns {string} - 创作结果描述
	 */
	protected handleComposeMusic(args: ComposeMusicParams): string {
		try {
			/** 音乐标题 */
			const title = args.title || '未命名作品';
			/** ABC 记谱法乐谱 */
			const abcNotation = args.abc_notation || '';

			console.log(`[音乐家] 创作音乐: "${title}"`);
			if (args.instruments) console.log(`  乐器: ${args.instruments}`);
			if (args.tempo) console.log(`  速度: ${args.tempo} BPM`);
			if (args.structure) console.log(`  结构: ${args.structure}`);

			// 验证 ABC 记谱法基本格式
			if (!abcNotation.trim()) {
				return '音乐创作失败：ABC记谱法乐谱为空';
			}

			// 检查必要的 ABC 头部字段
			const hasX = /^X:\s*\d+/m.test(abcNotation);
			const hasT = /^T:\s*.+/m.test(abcNotation);
			const hasK = /^K:\s*.+/m.test(abcNotation);

			if (!hasX || !hasK) {
				console.warn('[音乐家] ABC乐谱缺少必要字段 (X:/K:)，尝试自动补充');
				// 自动补充缺失的头部字段
				let fixedAbc = abcNotation;
				if (!hasX) fixedAbc = 'X:1\n' + fixedAbc;
				if (!hasT) fixedAbc = fixedAbc.replace(/^(X:\s*\d+\n)/m, `$1T:${title}\n`);
				if (!hasK) fixedAbc = fixedAbc.replace(/^(T:.*\n)/m, `$1K:C\n`);
				// 推送修复后的乐谱
				const pushSuccess = pushContext('music', fixedAbc, '');
				if (!pushSuccess) {
					console.warn('[音乐家] 推送乐谱到前端失败');
				}
				return `音乐作品"${title}"创作成功（已自动补全格式）。乐谱已推送到前端进行渲染播放。`;
			}

			// 推送乐谱到前端
			const pushSuccess = pushContext('music', abcNotation, '');
			if (!pushSuccess) {
				console.warn('[音乐家] 推送乐谱到前端失败');
			}

			console.log(`[音乐家] 乐谱推送成功，长度: ${abcNotation.length} 字符`);
			return `音乐作品"${title}"创作成功。乐谱已推送到前端进行渲染播放。`;
		}
		catch (error) {
			console.error('[音乐家] 音乐创作处理异常:', error);
			return `音乐创作异常: ${error}`;
		}
	}
}

/** 音乐创作详情记录（用于向对话者传递作品信息） */
interface MusicPieceDetail {
	title: string;
	instruments: string;
	tempo: number;
	structure: string;
	key: string;
	meter: string;
	abcLength: number;
}

/**
 * 音乐家角色
 * 
 * 参考 PainterRole 的技术架构实现。
 * 
 * 职责：
 * 1. 接收用户输入请求并进行解析
 * 2. 生成初步音乐创作计划，包括乐器选择、段落结构规划
 * 3. 根据总体音乐规范和段落独立规划进行乐器二次筛选与确认
 * 4. 调用 LLM 生成 ABC 记谱法乐谱
 * 5. 通过 pushContext('music', ...) 推送到前端渲染
 * 6. 将作品详情写入对话者上下文，确保对话者知晓创作了什么
 */
export class MusicianRole extends MusicToolchain {
	/** 音乐家独立历史（跨周期持久化，供对话者消费后清空） */
	private _history: PostMessage[] = [];
	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/musicianRole.md')[0]);
	}
	/** 获取音乐家历史摘要（对话者调用后清空） */
	public consumeHistory(): PostMessage[] {
		const result = [...this._history];
		this._history = [];
		return result;
	}

	/**
	 * 创建音乐创作上下文
	 *
	 * 覆写音乐家智能体的上下文，检查用户消息是否包含音乐创作关键词，
	 * 如果包含则执行音乐创作循环。
	 *
	 * @param {AgentDefine} source - 智能体定义实例
	 * @param {PostMessage[]} unreadContext - 当前未读上下文快照
	 * @param {number} count - 检查的消息数量
	 *
	 * @returns {boolean} - 是否执行了音乐创作
	 */
	public createMusicComposition(source: AgentDefine, unreadContext: PostMessage[], count: number = 10): boolean {
		// 构建音乐家上下文：对话历史（最近15条）+ 音乐家自身历史（最近5条）+ 当前未读
		const dialogueHistory = source.dialogueRole.messages.slice(-15);
		const musicianHistory = this._history.slice(-5);
		this.coverContext([...dialogueHistory, ...musicianHistory, ...unreadContext]);

		/** 未读消息文本内容 */
		const unreadTexts: string[] = [];
		// 遍历未读消息，提取文本内容
		for (const message of unreadContext.slice(-count)) {
			// 如果消息内容是字符串，直接添加
			if (typeof message.content === 'string') unreadTexts.push(message.content);
			// 如果消息内容是数组，遍历添加文本内容
			else message.content.forEach(item => { if (item.type === 'text') unreadTexts.push(item.text); });
		}

		/** 检查是否允许生成音乐 */
		let allowComposition = false;
		/** 音乐创作关键词模式 — 匹配用户明确要求创作音乐的意图 */
		const musicKeywords = [
			/创作(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
			/生成(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
			/写(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
			/制作(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
			/编(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|曲|钢琴曲|古典乐|轻音乐)/,
			/(?:帮我|给我|为我)(?:创作|生成|写|制作|编|做|弄|整)(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)?/,
			/(?:做|弄|整)(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
			/来(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|乐谱|钢琴曲|古典乐|轻音乐)/,
			/作曲/,
			/编曲/,
			/谱写/,
			/演奏(?:一(?:首|段|曲))?.*(?:音乐|乐曲|歌曲|曲子|旋律|钢琴曲|古典乐|轻音乐)/,
			/(?:弹|拉|吹)(?:一(?:首|段|曲))?.*(?:钢琴|小提琴|吉他|笛子|古筝|曲子|音乐|旋律)/,
		];

		// 遍历消息文本，检查是否包含音乐创作关键词
		unreadTexts.forEach(text => musicKeywords.forEach(keyword => { if (keyword.test(text)) allowComposition = true; }));

		// 如果没有包含音乐创作关键词，直接返回
		if (!allowComposition) return true;

		/** 最大迭代次数 */
		const MAX_ITERATIONS = 3;

		/** 创作记录 — 收集所有生成的音乐作品详情，用于向对话者传递 */
		const createdPieces: MusicPieceDetail[] = [];

		// 执行音乐创作推理循环
		for (let i = 0; i < MAX_ITERATIONS; i++) {
			console.log(`[音乐家] 第 ${i + 1} 轮音乐创作推理`);

			/** LLM 响应 */
			let response: modelResponse;
			try {
				response = this.run([], this.musicTool);
			}
			catch (error) {
				console.error(`[音乐家] 第 ${i + 1} 轮推理失败:`, error);
				break;
			}

			/** 模型返回的选项 */
			const choice = response.body?.choices?.[0];
			if (!choice) {
				console.log('[音乐家] 模型返回空结果，结束音乐创作循环');
				break;
			}

			/** 工具调用列表 */
			const toolCalls = choice.message?.tool_calls;
			if (!toolCalls || toolCalls.length === 0) {
				/** 模型回复内容 */
				const replyContent = choice.message?.content || '';
				// 如果模型回复内容不为空，将模型回复内容写入到聊天模型的未读上下文
				if (replyContent) source.unreadContext.push({ role: 'tool', content: `[音乐创作] ${replyContent}` });
				break;
			}

			// 将助手消息写入上下文（包含工具调用信息）
			this.writeContext(choice.message);

			// 遍历执行所有工具调用
			for (const toolCall of toolCalls) {
				console.log(`[音乐家] 执行工具: ${toolCall.function.name}`);
				/** 工具执行结果 */
				const result = this.executeMusicTool(toolCall);
				// 将工具执行结果写入上下文
				this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });

				// 收集 compose_music 工具调用的详情，用于向对话者传递作品信息
				if (toolCall.function.name === 'compose_music') {
					this.collectMusicDetail(toolCall, createdPieces);
				}
			}
		}

		// 将创作的作品详情写入音乐家历史，供对话者消费
		if (createdPieces.length > 0) {
			const summary = this.buildMusicSummary(createdPieces);
			this._history.push({ role: 'tool', content: summary });
			console.log(`[音乐家] 已将 ${createdPieces.length} 首作品详情写入历史`);
		}

		return false;
	}

	/**
	 * 从工具调用中提取音乐作品详情
	 */
	private collectMusicDetail(toolCall: ToolCallItem, pieces: MusicPieceDetail[]): void {
		try {
			const args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
			if (args.title) {
				pieces.push({
					title: args.title,
					instruments: args.instruments || '',
					tempo: args.tempo || 0,
					structure: args.structure || '',
					key: args.key || '',
					meter: args.meter || '',
					abcLength: (args.abc_notation || '').length,
				});
			}
		} catch {
			// 解析失败时跳过，不阻断流程
		}
	}

	/**
	 * 构建音乐作品摘要，供对话者使用
	 * 
	 * 对话者将根据此摘要向用户介绍创作的音乐作品，
	 * 因此需要包含完整的作品信息以确保对话准确性。
	 */
	private buildMusicSummary(pieces: MusicPieceDetail[]): string {
		const parts: string[] = [];
		parts.push('[音乐创作记录] 你（月华）刚刚完成了以下音乐作品创作：');

		for (let i = 0; i < pieces.length; i++) {
			const p = pieces[i];
			const detailLines: string[] = [];
			detailLines.push(`作品${i + 1}：《${p.title}》`);
			if (p.instruments) detailLines.push(`  - 乐器配置：${p.instruments}`);
			if (p.key) detailLines.push(`  - 调式：${p.key}${p.key === p.key.toLowerCase() ? '小调' : '大调'}`);
			if (p.tempo > 0) detailLines.push(`  - 速度：${p.tempo} BPM`);
			if (p.meter) detailLines.push(`  - 拍号：${p.meter}`);
			if (p.structure) detailLines.push(`  - 段落结构：${p.structure}`);
			detailLines.push(`  - 乐谱长度：${p.abcLength} 字符`);
			parts.push(detailLines.join('\n'));
		}

		parts.push('\n注意：请基于以上真实创作信息向用户介绍音乐作品，切勿编造不存在的曲名、乐器或结构。乐谱已通过音乐播放器推送给用户，可以引导用户查看和播放。');
		return parts.join('\n');
	}

	/**
	 * 执行音乐创作工具调用
	 * 
	 * 解析工具调用参数，路由到对应的工具处理函数。
	 * 
	 * @param toolCall - 工具调用对象
	 * 
	 * @returns {string} - 工具执行结果描述
	 */
	private executeMusicTool(toolCall: ToolCallItem): string {
		/** 工具函数名称 */
		const funcName = toolCall.function.name;

		/** 工具函数参数 */
		let args: Record<string, any> = {};
		try {
			args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
		}
		catch (parseError) {
			console.error(`[音乐家] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
		}

		// 根据工具函数名称路由到对应的处理函数
		switch (funcName) {
			// 处理音乐创作工具调用
			case 'compose_music': return this.handleComposeMusic(args as ComposeMusicParams);
			// 处理未知工具调用
			default: return `未知工具: ${funcName}，可用工具为 compose_music`;
		}
	}
}