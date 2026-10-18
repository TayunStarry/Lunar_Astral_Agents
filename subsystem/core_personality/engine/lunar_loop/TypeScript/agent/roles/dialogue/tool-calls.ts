import { ChatCache } from '../../../config/config';
import { GlobalConfig } from '../../../config/global';
import { PostMessage } from '../../../config/model';
import { attachImageTags } from '../../capabilities/image-tag';
import { maybeSendFastAck } from './ack';

/**
 * 批量执行工具调用
 *
 * @param messages 对话者上下文（原位追加 tool 回填与工具图片消息）
 * @param state 本轮聊天缓存（执行完清空工具调用状态）
 * @returns 是否至少执行过一个工具
 */
export async function executeToolCalls(messages: PostMessage[], state: ChatCache): Promise<boolean> {
	/** 工具调用标志 */
	let hasToolCalls = false;
	// 遍历所有工具调用
	for (const toolCall of state.toolCalls) {
		/** 工具函数名称 */
		const functionName = toolCall.function.name;
		/** 工具函数参数 */
		const functionArgs = toolCall.function.arguments;
		/** 查询对应的月华工具包 */
		const lunarToolPackage = GlobalConfig.LTPfunction.get(functionName);
		// 检查是否有对应的工具包
		if (!lunarToolPackage) {
			messages.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
			continue;
		}
		// 高耗时任务：在真正执行前先给用户一条快速应答（一轮仅一次）
		maybeSendFastAck(functionName, functionArgs);
		try {
			/** 工具函数执行结果：string[]格式，下标0=文本内容，下标1=图片base64数据（无则为空字符串） */
			const toolResult = await lunarToolPackage(functionArgs);
			/** 提取文本内容 */
			const textContent = Array.isArray(toolResult) ? toolResult[0] : String(toolResult);
			/** 提取图片base64数据 */
			const base64Image = Array.isArray(toolResult) ? toolResult[1] : '';
			// 将工具响应添加到消息历史中：文本恒以 tool 角色回填，保证 tool_call_id 配对完整
			messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
			// 工具返回的图片（如截图）作为紧随其后的用户多模态消息注入：
			// 与用户图片走同一套入队赋签机制（入库得 [@I.00x] 旁注），
			// 一样参与窗口降级与按句柄引用
			if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
				/** 工具图片消息 */
				const message: PostMessage = {
					role: "user",
					content: [
						{ type: "text", text: `【工具返回的图片】${textContent}` },
						{ type: "image_url", image_url: { url: base64Image } }
					]
				};
				attachImageTags(message);
				messages.push(message);
				console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
			}
			// 标记有工具调用
			hasToolCalls = true;
		}
		catch (error) {
			// 将工具调用失败信息添加到消息历史中
			messages.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
		}
	}
	// 处理完所有工具调用后,清空状态
	state.currentToolCallIndex = -1;
	state.currentFunctionArgs = "";
	state.currentFunctionName = "";
	state.currentToolCall = null;
	state.toolCalls = [];
	// 标记有工具调用
	return hasToolCalls;
}
