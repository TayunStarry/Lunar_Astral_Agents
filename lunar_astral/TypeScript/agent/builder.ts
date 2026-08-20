import { GlobalConfig, PostMessage, ModelProtocol, AuthHeaders, modelResponse, ToolCall } from '../index';

/** 模型构建器 */
export class ModelBuilder {
	/** 是否启用流式响应 */
	protected stream: boolean = false;
	/** 是否启用工具调用 */
	protected enableTools: boolean = true;
	/** 消息列表 */
	public messages: PostMessage[] = [];
	/** RAG消息列表 */
	protected ragMessages: PostMessage[] = [];
	/** 运行时消息列表 */
	protected runtimeMessages: PostMessage[] = [];
	/** 系统提示 */
	protected systemPrompt: string = "你的名字叫做月华, 是一个女孩子";
	/** 生成提示词 */
	protected promptCompletion(prompt: string): string {
		/** 当前地址文本 */
		let addressText = "";
		// 若当前地址为空，查询真实地址
		if (GlobalConfig.currentAddress.length === 0) {
			/** 查询真实地址 */
			const addressResult = address();
			// 设置当前地址
			GlobalConfig.currentAddress = addressResult[0];
			addressText = GlobalConfig.currentAddress.join(' ');
		}
		// 否则使用缓存地址
		else addressText = GlobalConfig.currentAddress.join(' ');
		// 返回替换后的系统提示词
		return prompt
			// 转换用户名称
			.replace(/{name}/g, GlobalConfig.userName)
			// 转换当前地址
			.replace(/{current-address}/g, addressText);
	}
	/** 写入上下文（自动剥离 reasoning_content，避免回传触发模型无限推理） */
	public writeContext(context: PostMessage): this {
		const cleaned = this.stripReasoningContent(context);
		if (this.messages.length >= 40) {
			const discarded = this.messages.slice(0, this.messages.length - 39);
			this.messages = this.messages.slice(-39).concat(cleaned);
			GlobalConfig.unreadRecords.push(...discarded);
		}
		else this.messages.push(cleaned);
		return this;
	}
	/** 剥离消息中的 reasoning_content 字段，防止回传给模型触发无限推理 */
	protected stripReasoningContent(message: PostMessage): PostMessage {
		if ('reasoning_content' in message) {
			const { reasoning_content, ...rest } = message as any;
			return rest as PostMessage;
		}
		return message;
	}
	/** 覆写上下文 */
	public coverContext(context: PostMessage[] | PostMessage): this {
		this.messages = Array.isArray(context) ? context : [context];
		return this;
	}
	/** 运行模型，可输入额外的上下文补充 */
	public run(appendContext: PostMessage[], toolCall: ToolCall[]): modelResponse {
		/** 模型请求体消息列表（拼接所有来源） */
		const rawMessages: PostMessage[] = [
			// 系统提示词
			{ role: 'system', content: this.systemPrompt },
			// 运行时消息
			...this.runtimeMessages,
			// 追加的上下文(RAG消息)
			...appendContext,
			// 历史上下文消息
			...this.messages
		];
		/** 构建发给推理模型的请求体 */
		const requestBody = {
			model: GlobalConfig.MultimodalName,
			messages: rawMessages,
			stream: this.stream,
			tools: toolCall,
			tool_choice: 'auto',
		};
		// 如果禁用工具调用或没有工具调用，删除 tool_choice 和 tools 字段
		if (!this.enableTools || toolCall.length === 0) {
			delete requestBody.tool_choice;
			delete requestBody.tools;
		};
		/** 构建请求头 */
		const headers: AuthHeaders = {
			Authorization: `Bearer ${encodeURIComponent(GlobalConfig.MultimodalKey)}`,
			"Content-Type": "application/json",
		};
		/** 构建模型请求 */
		const modelRequest: ModelProtocol = {
			method: "POST",
			crossDomain: true,
			headers,
			body: JSON.stringify(requestBody)
		};
		/** 定义API端点 */
		const endpoint = "/chat/completions";
		/** 直接调用Go函数处理请求 */
		const [result, error] = syncFetch({ url: GlobalConfig.MultimodalUrl + endpoint, execute: modelRequest });
		// 抛出请求级错误
		if (error) throw error;
		// 检查云端错误响应（网关返回 {error: {...}} 而非正常 choices）
		if (result?.body?.error) {
			const errMsg = typeof result.body.error === 'string'
				? result.body.error
				: result.body.error.message || JSON.stringify(result.body.error);
			throw new Error(`模型服务错误 [${result.status}]: ${errMsg}`);
		}
		// 检查响应体结构完整性
		if (!result?.body?.choices) {
			throw new Error(`模型响应异常: status=${result?.status}, body=${JSON.stringify(result?.body)?.substring(0, 200)}`);
		}
		// 返回模型响应
		return result;
	}
	/** 构建模型响应实例 */
	public constructor(prompt: string) {
		// 补全系统提示词
		this.systemPrompt = this.promptCompletion(prompt);
	}
}