﻿import { ModelBuilder } from '../base/builder';
import { modelResponse } from '../../config/model';

/** 观影者角色 */
export class ViewerRole extends ModelBuilder {
	/** 拼接理解文本超过该长度时触发客观摘要（字符数） */
	private readonly SUMMARY_THRESHOLD = 4096;

	constructor() {
		super(fileView('prompts/viewerRole.md')[0]);
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
			console.warn('[影音者] 未收到任何视频片段');
			return '月华观看了这个视频，但没有获取到足够的信息。';
		}

		console.log(`[影音者] 开始观看视频，共 ${mediaUrls.length} 个片段`);

		/** 所有片段的内容理解 */
		const understandings: string[] = [];

		for (let index = 0; index < mediaUrls.length; index++) {
			console.log(`[影音者] 观看第 ${index + 1}/${mediaUrls.length} 段`);
			const understanding = await this.evaluateSegment(mediaUrls[index], index + 1, mediaUrls.length);
			if (understanding.trim().length > 0) {
				understandings.push(understanding);
				console.log(`[影音者] 第 ${index + 1} 段理解完成`);
			}
		}

		if (understandings.length === 0) {
			console.warn('[影音者] 未产生任何片段理解');
			return '月华观看了这个视频，但没有获取到足够的信息。';
		}

		/** 拼接所有片段的文本理解 */
		const concatenated = understandings.join('\n\n');

		// 拼接文本未超阈值时直接返回，不做任何总结
		if (concatenated.length <= this.SUMMARY_THRESHOLD) {
			console.log(`[影音者] 拼接理解文本 ${concatenated.length} 字符，未超 ${this.SUMMARY_THRESHOLD}，直接返回`);
			return concatenated;
		}

		// 超过阈值时触发无人格化的客观摘要
		console.log(`[影音者] 拼接理解文本 ${concatenated.length} 字符，超过 ${this.SUMMARY_THRESHOLD}，触发客观摘要`);
		const summary = await this.generateObjectiveSummary(concatenated);
		console.log('[影音者] 客观摘要完成');
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
		/** 构建提示词：任务格式与行为准则由系统提示（viewerRole.md）定义 */
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
			console.error(`[影音者] 第 ${index} 段推理失败:`, error);
			return '';
		}

		const content = response.body?.choices?.[0]?.message?.content || '';
		if (!content.trim()) {
			console.warn(`[影音者] 第 ${index} 段返回空内容`);
		}
		return content;
	}

	/**
	 * 基于语音转写文本生成客观内容理解
	 *
	 * 音频转写由 system-asr 模型在上游完成（多模态模型无需音频能力），
	 * 本方法仅消费转写文本，按系统提示的「音频理解任务」格式输出理解。
	 *
	 * @param transcript ASR 转写文本
	 * @returns 音频内容理解文本
	 */
	public async watchAudio(transcript: string): Promise<string> {
		// 覆写上下文：转写文本（纯文本，音频块不再透传给多模态模型）
		this.coverContext({
			role: 'user',
			content: `以下是系统语音识别模型对一段音频的转写文本，请按「音频理解任务」的格式输出该音频的客观内容理解。

【转写文本】
${transcript}`
		});
		this.runtimeMessages = [];

		/** 调用模型 */
		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[影音者] 音频理解推理失败:', error);
			return '';
		}

		const content = response.body?.choices?.[0]?.message?.content || '';
		if (!content.trim()) {
			console.warn('[影音者] 音频理解返回空内容');
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
		/** 构建提示词：整合要求由系统提示（viewerRole.md）的「客观摘要任务」定义 */
		const prompt = `请按「客观摘要任务」的要求，将以下视频各片段的内容理解文本整合为一份客观摘要。

【理解文本】
${concatenated}`;

		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];

		let response: modelResponse;
		try {
			response = this.run([], []);
		} catch (error) {
			console.error('[影音者] 客观摘要推理失败:', error);
			return '';
		}

		return response.body?.choices?.[0]?.message?.content || '';
	}
}
