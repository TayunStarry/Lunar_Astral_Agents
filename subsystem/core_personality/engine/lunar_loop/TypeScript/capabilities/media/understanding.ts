import { GlobalConfig } from '../../config/global';
import { savePromptToKnowledge } from '../../io/knowledge';

/** 是否采用图片序列帧模式解读视频/动态图（lunar_config.json 的 multimodal.video_input === 'frames'） */
export function useFrameSequence(): boolean {
	return GlobalConfig.VideoInputMode === 'frames';
}

/**
 * 观看并返回理解文本（视频与动态图共用，兼容 file:// 引用与图片序列帧两种输入）
 *
 * 理解文本非空且存在缓存键时写入知识库缓存；理解为空时返回空字符串。
 *
 * @param watch 感知者观看回调（file 模式观看 file:// 片段引用，序列帧模式观看采样帧分组）
 * @param cacheKey 知识库缓存键（原始来源地址；传空字符串表示不缓存）
 */
export async function understandMediaSegments(watch: () => Promise<string>, cacheKey: string): Promise<string> {
	// 调用感知者智能体观看
	console.log('[感知者] 开始观看...');
	const summary = await watch();
	console.log('[感知者] 观看完成');
	// 理解文本非空时写入缓存
	if (summary && summary.trim().length > 0 && cacheKey) {
		savePromptToKnowledge(cacheKey, summary);
		console.log('[感知者] 理解文本已缓存');
	}
	return summary;
}
