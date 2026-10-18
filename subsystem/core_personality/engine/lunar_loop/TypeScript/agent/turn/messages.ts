import { GlobalConfig } from '../../config/global';
import { MessageContent, PostMessageRole } from '../../config/model';
import { attachImageTags } from '../../capabilities/image-tag';
import { appendUserMessage } from '../../io/conversation';
import {
	descriptionRole,
	dialogueRole,
	musicianRole,
	painterRole,
	perceiverRole
} from '../../roles/index';

/** 写入消息（content 为文本、多模态内容数组或宿主下发的媒体URL内容项） */
export function writeMessage(role: PostMessageRole, content: any) {
	// 宿主统一队列下发的媒体URL内容项为裸对象形态（{"type":"video_url"/"audio_url", ...}）：
	// 包装为单内容项数组，与多模态消息同构（LiteImageFile 可迭代原位置换）
	if (content && typeof content === 'object' && !Array.isArray(content) && (content.type === 'video_url' || content.type === 'audio_url')) content = [content];
	/** 待入队消息 */
	const incoming = { role, content };
	// 入队赋签：图片项在此刻入库获得 [@I.00x] 标签（旁注紧贴图片项之后，
	// 标签与文件的绑定落在资源目录数据库），后续压缩/入上下文/降级只是搬运
	attachImageTags(incoming);
	// 入缓存：此时图片来源已改写为视觉读取地址，缓存里转回 /file/read/ 供前端查看原图
	if (role === 'user') appendUserMessage(content);
	// 写入统一未读队列
	GlobalConfig.unreadContext.push(incoming);
	// 字符串消息转换为文本内容项后统一打印
	const items: MessageContent[] = typeof content === 'string' ? [{ type: 'text', text: content }] : Array.isArray(content) ? content : [];
	for (const message of items) {
		if (message.type === 'text') console.log('收到文本: ' + message.text);
		else if (message.type === 'image_url') console.log('收到图片: ' + message.image_url?.url?.substring(0, 50));
		else if (message.type === 'input_audio') console.log('收到音频: ' + message.input_audio?.data?.substring(0, 30));
		else if (message.type === 'video_url') console.log('收到视频: ' + message.video_url?.url);
		else if (message.type === 'audio_url') console.log('收到音频: ' + message.audio_url?.url);
	}
}

/** 错误累积达阈值后重置智能体状态 */
export function resetAgentState(): void {
	// 清空全部子智能体的messages
	descriptionRole.coverContext([]);
	dialogueRole.coverContext([]);
	painterRole.coverContext([]);
	musicianRole.coverContext([]);
	perceiverRole.coverContext([]);
	// 清空绘图者的参考图状态（每次创作任务开工时也会自清，这里兜住异常路径）
	painterRole.resetReference();
	// 清空主智能体的统一未读队列
	GlobalConfig.unreadContext = [];
}
