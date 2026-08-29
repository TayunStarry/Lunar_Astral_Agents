import { ModelBuilder, modelResponse, ImageContent } from '../../index';

/** 关键帧数据 */
interface KeyFrameData {
	/** 帧数据（base64 JPEG） */
	data: string;
	/** 时间戳 */
	timestamp: string;
}

/** 观影者角色 */
export class ViewerRole extends ModelBuilder {
	/** 每批帧数 */
	private readonly BATCH_SIZE = 20;
	/** 二次摘要间隔（每5轮 = 100帧） */
	private readonly SECONDARY_SUMMARY_INTERVAL = 5;
	/** 最大轮数（40轮 × 20帧 = 800帧上限） */
	private readonly MAX_ROUNDS = 40;

	constructor() {
		super(fileView('prompts/viewerRole.md')[0]);
	}

	/**
	 * 观看视频（主入口）
	 *
	 * 接收关键帧数组，分批处理，生成二次摘要和三次摘要，
	 * 最终返回一份完整的视频观后感。
	 *
	 * @param keyframes 关键帧数据数组
	 * @returns 最终视频观后感摘要
	 */
	public async watchVideo(keyframes: KeyFrameData[]): Promise<string> {
		const totalFrames = Math.min(keyframes.length, this.MAX_ROUNDS * this.BATCH_SIZE);
		const totalRounds = Math.ceil(totalFrames / this.BATCH_SIZE);

		console.log(`[观影者] 开始观看视频，共 ${totalFrames} 帧，${totalRounds} 轮`);

		/** 所有轮次的评价 */
		const evaluations: string[] = [];
		/** 二次摘要列表 */
		const secondarySummaries: string[] = [];

		for (let round = 0; round < totalRounds; round++) {
			const start = round * this.BATCH_SIZE;
			const batch = keyframes.slice(start, start + this.BATCH_SIZE);
			if (batch.length === 0) break;

			console.log(`[观影者] 第 ${round + 1}/${totalRounds} 轮，处理 ${batch.length} 帧`);

			// 第一步：对当前批次生成评价
			const evaluation = await this.evaluateBatch(batch, round + 1);
			if (evaluation) {
				evaluations.push(evaluation);
				console.log(`[观影者] 第 ${round + 1} 轮评价完成`);
			}

			// 第二步：每5轮（或最后一轮）执行二次摘要
			const isLastRound = round === totalRounds - 1;
			const shouldSummarize = (round + 1) % this.SECONDARY_SUMMARY_INTERVAL === 0 || isLastRound;

			if (shouldSummarize && evaluations.length > 0) {
				const recentEvals = evaluations.slice(-this.SECONDARY_SUMMARY_INTERVAL);
				const secondarySummary = await this.generateSecondarySummary(recentEvals);
				if (secondarySummary) {
					secondarySummaries.push(secondarySummary);
					console.log(`[观影者] 二次摘要完成（第 ${secondarySummaries.length} 份）`);
				}
			}
		}

		// 第三步：全部批次完成后，执行三次摘要整合
		if (secondarySummaries.length === 0) {
			console.warn('[观影者] 未产生任何二次摘要');
			return '月华观看了这个视频，但没有获取到足够的信息。';
		}

		if (secondarySummaries.length === 1) {
			console.log('[观影者] 仅一份摘要，直接返回');
			return secondarySummaries[0];
		}

		const finalSummary = await this.generateTertiarySummary(secondarySummaries);
		console.log('[观影者] 三次摘要（最终观后感）完成');
		return finalSummary || secondarySummaries.join('\n\n');
	}

	/**
	 * 对一批关键帧生成评价
	 *
	 * @param frames 关键帧批次（最多20帧）
	 * @param round 当前轮次编号
	 * @returns 评价文本
	 */
	private async evaluateBatch(frames: KeyFrameData[], round: number): Promise<string> {
		/** 将关键帧转换为 ImageContent 数组 */
		const imageContents: ImageContent[] = frames.map(frame => ({
			type: 'image_url',
			image_url: { url: `data:image/jpeg;base64,${frame.data}` }
		}));

		/** 构建提示词 */
		const prompt = `请观看以下视频的第 ${round} 批关键帧（共 ${frames.length} 帧），以月华的身份描述你的观影感受和发现的关键信息。
时间范围：${frames[0]?.timestamp || '?'} ~ ${frames[frames.length - 1]?.timestamp || '?'}

请按以下格式输出：
【感受】
（以月华的第一人称写2-4句话）

【关键信息】
- 人物：...
- 场景：...
- 事件：...
- 变化：...`;

		// 覆写上下文：用户消息包含提示词 + 图片
		this.coverContext({
			role: 'user',
			content: [
				{ type: 'text', text: prompt },
				...imageContents
			]
		});
		this.runtimeMessages = [];

		/** 调用模型 */
		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error(`[观影者] 第 ${round} 轮推理失败:`, error);
			return '';
		}

		const content = response.body?.choices?.[0]?.message?.content || '';
		if (!content.trim()) {
			console.warn(`[观影者] 第 ${round} 轮返回空内容`);
		}
		return content;
	}

	/**
	 * 生成二次摘要（整合5轮评价）
	 *
	 * @param evaluations 最近5轮的评价文本
	 * @returns 二次摘要
	 */
	private async generateSecondarySummary(evaluations: string[]): Promise<string> {
		const prompt = `请将以下 ${evaluations.length} 段视频片段评价整合为一份连贯的摘要。

【评价内容】
${evaluations.map((e, i) => `--- 片段${i + 1} ---\n${e}`).join('\n\n')}

【整合要求】
1. 保持月华的第一人称视角
2. 使用活泼可爱的女孩语气
3. 突出最重要的感受和发现
4. 按时间线或逻辑线组织内容
5. 字数控制在200-400字

仅输出摘要内容，不要包含其他说明文字。`;

		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];

		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[观影者] 二次摘要推理失败:', error);
			return '';
		}

		return response.body?.choices?.[0]?.message?.content || '';
	}

	/**
	 * 生成三次摘要（最终观后感）
	 *
	 * @param secondarySummaries 所有二次摘要
	 * @returns 最终观后感
	 */
	private async generateTertiarySummary(secondarySummaries: string[]): Promise<string> {
		const prompt = `请将以下 ${secondarySummaries.length} 份视频片段摘要整合为一份完整的视频观后感。

【片段摘要】
${secondarySummaries.map((s, i) => `--- 摘要${i + 1} ---\n${s}`).join('\n\n')}

【整合要求】
1. 以月华的身份，用第一人称视角写一份完整的观后感
2. 使用活泼可爱的女孩语气
3. 描述月华对整个视频的整体感受和印象
4. 包含视频的主要内容概述、最打动月华的部分、月华的个人感受
5. 字数控制在300-500字
6. 结构清晰，有开头、主体和结尾

仅输出观后感内容，不要包含其他说明文字。`;

		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];

		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[观影者] 三次摘要推理失败:', error);
			return '';
		}

		return response.body?.choices?.[0]?.message?.content || '';
	}
}