import { ModelBuilder } from '../base/builder';
import { modelResponse } from '../../config/model';

/** 感知者角色 */
export class PerceiverRole extends ModelBuilder {
	/** 拼接理解文本超过该长度时触发客观摘要（字符数） */
	private readonly SUMMARY_THRESHOLD = 4096;
	/** 客观摘要任务模板（占位符 {content} 由拼接的理解文本填充） */
	private readonly summaryTaskTemplate = fileView('prompts/perceiverSummaryTask.md')[0];

	constructor() {
		super(fileView('prompts/perceiverRole.md')[0]);
	}

	/**
	 * 观看视频（主入口）
	 *
	 * 接收 llama-server 媒体目录内的视频引用列表（file:// 形式），
	 * 服务端通过 ffmpeg 自动抽帧（--video-fps 控制频率），
	 * 逐段生成客观内容理解并直接拼接；仅当拼接结果超过
	 * SUMMARY_THRESHOLD 时，触发一次无人格化的客观摘要。
	 *
	 * @param mediaUrls 视频片段引用列表（file://<文件名>）
	 * @returns 视频内容理解文本（拼接或摘要）
	 */
	public async watchVideo(mediaUrls: string[]): Promise<string> {
		if (mediaUrls.length === 0) {
			console.warn('[感知者] 未收到任何视频片段');
			return '月华观看了这个视频，但没有获取到足够的信息。';
		}

		console.log(`[感知者] 开始观看视频，共 ${mediaUrls.length} 个片段`);

		/** 所有片段的内容理解 */
		const understandings: string[] = [];

		for (let index = 0; index < mediaUrls.length; index++) {
			console.log(`[感知者] 观看第 ${index + 1}/${mediaUrls.length} 段`);
			const understanding = await this.evaluateSegment(mediaUrls[index], index + 1, mediaUrls.length);
			if (understanding.trim().length > 0) {
				understandings.push(understanding);
				console.log(`[感知者] 第 ${index + 1} 段理解完成`);
			}
		}

		if (understandings.length === 0) {
			console.warn('[感知者] 未产生任何片段理解');
			return '月华观看了这个视频，但没有获取到足够的信息。';
		}

		/** 拼接所有片段的文本理解 */
		const concatenated = understandings.join('\n\n');

		// 拼接文本未超阈值时直接返回，不做任何总结
		if (concatenated.length <= this.SUMMARY_THRESHOLD) {
			console.log(`[感知者] 拼接理解文本 ${concatenated.length} 字符，未超 ${this.SUMMARY_THRESHOLD}，直接返回`);
			return concatenated;
		}

		// 超过阈值时触发无人格化的客观摘要
		console.log(`[感知者] 拼接理解文本 ${concatenated.length} 字符，超过 ${this.SUMMARY_THRESHOLD}，触发客观摘要`);
		const summary = await this.generateObjectiveSummary(concatenated);
		console.log('[感知者] 客观摘要完成');
		return summary || concatenated;
	}

	/**
	 * 观看一段视频并生成客观内容理解
	 *
	 * @param mediaUrl 视频片段引用（file://<文件名>）
	 * @param index 片段序号（从1开始）
	 * @param total 片段总数
	 * @returns 该片段的客观内容理解文本
	 */
	private async evaluateSegment(mediaUrl: string, index: number, total: number): Promise<string> {
		/** 构建提示词：任务格式与行为准则由系统提示（perceiverRole.md）定义 */
		const position = total === 1 ? '' : `（第 ${index}/${total} 段）`;
		const prompt = `请观看这段视频${position}，按「片段理解任务」的格式输出该片段的客观内容理解。`;

		// 覆写上下文：文本提示词 + 视频引用（服务端自动抽帧解码）
		this.coverContext({
			role: 'user',
			content: [
				{ type: 'text', text: prompt },
				{ type: 'image_url', image_url: { url: mediaUrl } }
			]
		});
		this.runtimeMessages = [];

		/** 调用模型 */
		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error(`[感知者] 第 ${index} 段推理失败:`, error);
			return '';
		}

		const content = response.body?.choices?.[0]?.message?.content || '';
		if (!content.trim()) {
			console.warn(`[感知者] 第 ${index} 段返回空内容`);
		}
		return content;
	}

	/**
	 * 生成无人格化的客观摘要
	 *
	 * @param concatenated 拼接后的全部片段理解文本
	 * @returns 客观摘要文本
	 */
	private async generateObjectiveSummary(concatenated: string): Promise<string> {
		/** 构建提示词：任务模板来自 perceiverSummaryTask.md，整合要求由系统提示（perceiverRole.md）的「客观摘要任务」定义 */
		const prompt = this.summaryTaskTemplate.replace('{content}', concatenated);

		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];

		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[感知者] 客观摘要推理失败:', error);
			return '';
		}

		return response.body?.choices?.[0]?.message?.content || '';
	}
}
