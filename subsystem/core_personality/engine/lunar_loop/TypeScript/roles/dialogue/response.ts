import { ChatCache } from '../../config/config';
import { ModelResponseBody } from '../../config/model';

/** 处理聊天消息响应 */
export function applyModelResponse(message: ModelResponseBody, cache: ChatCache): void {
	try {
		// 处理推理内容数据
		if (message.choices?.[0]?.message?.reasoning_content) {
			cache.thinkingContent = message.choices[0].message.reasoning_content;
		}
		// 检查是否有词元生成速度数据
		if (message.timings?.predicted_per_second) {
			console.log(`词元生成速度: ${message.timings.predicted_per_second}`);
		}
		// 检查缓存命中数量
		if (message.timings?.cache_n !== undefined) {
			console.log(`缓存命中数量: ${message.timings.cache_n}`);
		}
		// 处理工具调用
		if (message.choices?.[0]?.message?.tool_calls) {
			// 遍历所有工具调用
			for (const toolCall of message.choices[0].message.tool_calls) {
				try {
					// 解析arguments字段
					toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
					// 记录工具调用
					cache.toolCalls.push(toolCall);
				}
				catch (parseError) {
					console.error('工具调用参数解析错误:', parseError);
				}
			}
		}
		// 处理内容数据
		if (message.choices?.[0]?.message?.content) {
			cache.descriptionContent = message.choices[0].message.content;
		}
	}
	catch (error) {
		console.error('聊天消息响应处理错误:', error);
	}
}
