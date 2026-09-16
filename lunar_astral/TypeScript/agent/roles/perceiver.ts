import { ModelBuilder } from '../base/builder';
import { modelResponse } from '../../config/model';

/** 感知者角色 */
export class PerceiverRole extends ModelBuilder {
	/** 拼接理解文本超过该长度时触发客观摘要（字符数） */
	private readonly SUMMARY_THRESHOLD = 4096;
	/** 客观摘要任务模板（占位符 {content} 由拼接的理解文本填充） */
	private readonly summaryTaskTemplate = fileView('prompts/perceiverSummaryTask.md')[0];
	/** 单次观看最多评估的片段数：超出部分等距抽样，避免长视频串行推理长时间阻塞智能体 */
	private readonly MAX_SEGMENTS = 8;
	/** 片段推理失败后的重试等待（毫秒）：多模态实例可能已被压崩，等待路由器重启后再重试 */
	private readonly RETRY_WAIT_MS = 25000;
	/** 每个片段的最大尝试次数（1 次原始尝试 + 1 次重试） */
	private readonly MAX_ATTEMPTS = 2;

	constructor() {
		super(fileView('prompts/perceiverRole.md')[0]);
	}

	/**
	 * 观看视频（主入口）
	 *
	 * 接收 llama-server 媒体目录内的视频引用列表（file:// 形式），
	 * 服务端通过 ffmpeg 自动抽帧（--video-fps 控制频率），
	 * 逐段生成内容理解并直接拼接；片段数超过 MAX_SEGMENTS 时等距抽样；
	 * 仅当拼接结果超过 SUMMARY_THRESHOLD 时，触发一次无人格化的客观摘要。
	 *
	 * 全部片段失败时返回空字符串（由调用方决定兜底提示，且不写入缓存），
	 * 摘要失败时退回拼接文本并硬切断，避免超长文本撑爆对话上下文。
	 *
	 * @param mediaUrls 视频片段引用列表（file://<文件名>）
	 * @returns 视频内容理解文本（拼接或摘要），全部失败时为空字符串
	 */
	public async watchVideo(mediaUrls: string[]): Promise<string> {
		if (mediaUrls.length === 0) {
			console.warn('[感知者] 未收到任何视频片段');
			return '';
		}

		/** 本次实际评估的片段引用列表 */
		const watched = this.sampleSegments(mediaUrls);
		if (watched.length < mediaUrls.length) {
			console.log(`[感知者] 片段数 ${mediaUrls.length} 超过单次上限 ${this.MAX_SEGMENTS}，等距抽样观看 ${watched.length} 段`);
		}
		console.log(`[感知者] 开始观看视频，共 ${watched.length} 个片段`);

		/** 所有片段的内容理解 */
		const understandings: string[] = [];

		for (let index = 0; index < watched.length; index++) {
			console.log(`[感知者] 观看第 ${index + 1}/${watched.length} 段`);
			const understanding = await this.evaluateSegment(watched[index], index + 1, watched.length);
			if (understanding.trim().length > 0) {
				understandings.push(understanding);
				console.log(`[感知者] 第 ${index + 1} 段理解完成`);
			}
		}

		if (understandings.length === 0) {
			console.warn('[感知者] 未产生任何片段理解');
			return '';
		}

		/** 拼接所有片段的内容理解 */
		const concatenated = understandings.join('\n\n');

		// 拼接文本未超阈值时直接返回，不做任何总结
		if (concatenated.length <= this.SUMMARY_THRESHOLD) {
			console.log(`[感知者] 拼接理解文本 ${concatenated.length} 字符，未超 ${this.SUMMARY_THRESHOLD}，直接返回`);
			return concatenated;
		}

		// 超过阈值时触发无人格化的客观摘要
		console.log(`[感知者] 拼接理解文本 ${concatenated.length} 字符，超过 ${this.SUMMARY_THRESHOLD}，触发客观摘要`);
		const summary = await this.generateObjectiveSummary(concatenated);
		if (summary.trim().length > 0) {
			console.log(`[感知者] 客观摘要完成（${summary.length} 字符）`);
			return summary;
		}
		// 摘要失败时退回拼接文本并硬切断，避免超长文本撑爆对话上下文
		console.warn(`[感知者] 客观摘要失败，退回拼接理解文本（硬切断至 ${this.SUMMARY_THRESHOLD} 字符）`);
		return concatenated.slice(0, this.SUMMARY_THRESHOLD);
	}

	/**
	 * 等距抽样片段引用：最多 MAX_SEGMENTS 段，恒包含首段与末段
	 *
	 * @param mediaUrls 视频片段引用列表
	 * @returns 抽样后的片段引用列表（不超过 MAX_SEGMENTS）
	 */
	private sampleSegments(mediaUrls: string[]): string[] {
		if (mediaUrls.length <= this.MAX_SEGMENTS) return mediaUrls;
		const picked: string[] = [];
		for (let i = 0; i < this.MAX_SEGMENTS; i++) {
			picked.push(mediaUrls[Math.round(i * (mediaUrls.length - 1) / (this.MAX_SEGMENTS - 1))]);
		}
		// 取整可能产生重复索引，去重后保持顺序
		return [...new Set(picked)];
	}

	/**
	 * 观看一段视频并生成内容理解（失败自动重试一次）
	 *
	 * @param mediaUrl 视频片段引用（file://<文件名>）
	 * @param index 片段序号（从1开始）
	 * @param total 片段总数
	 * @returns 该片段的内容理解文本，失败时为空字符串
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

		for (let attempt = 1; attempt <= this.MAX_ATTEMPTS; attempt++) {
			try {
				/** 调用模型 */
				const response = this.run([], []);
				const content = response.body?.choices?.[0]?.message?.content || '';
				if (!content.trim()) {
					console.warn(`[感知者] 第 ${index} 段返回空内容`);
				}
				return content;
			}
			catch (error) {
				console.error(`[感知者] 第 ${index} 段第 ${attempt} 次推理失败:`, error);
				// 推理失败常伴随多模态实例被压崩：等待路由器重启实例后再重试
				if (attempt < this.MAX_ATTEMPTS) {
					console.warn(`[感知者] 等待 ${this.RETRY_WAIT_MS / 1000} 秒后重试第 ${index} 段`);
					await new Promise(resolve => setTimeout(resolve, this.RETRY_WAIT_MS));
				}
			}
		}
		return '';
	}

	/**
	 * 生成无人格化的客观摘要
	 *
	 * @param concatenated 拼接后的全部片段理解文本
	 * @returns 客观摘要文本，失败时为空字符串
	 */
	private async generateObjectiveSummary(concatenated: string): Promise<string> {
		/** 构建提示词：任务模板来自 perceiverSummaryTask.md，整合要求由系统提示（perceiverRole.md）的「客观摘要任务」定义 */
		const prompt = this.summaryTaskTemplate.replace('{content}', concatenated);

		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];

		try {
			const response = this.run([], []);
			return response.body?.choices?.[0]?.message?.content || '';
		}
		catch (error) {
			console.error('[感知者] 客观摘要推理失败:', error);
			return '';
		}
	}
}
