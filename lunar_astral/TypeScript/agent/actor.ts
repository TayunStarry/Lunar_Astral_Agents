import { ToolCall, ToolCallItem, CreativeRoleBase } from '../index';

/** 行动记录详情 */
interface ActionDetail {
	/** 工具名称 */
	toolName: string;
	/** 动作名称（play_action 专用） */
	actionName?: string;
	/** 目标坐标（agent_movement 专用） */
	targetPos?: string;
}

/** 引擎广播的动作定义 */
interface EngineActionDefinition {
	name: string;
	mouseTracking: boolean;
}

/** 引擎动画列表缓存响应 */
interface AnimationListResponse {
	actions?: EngineActionDefinition[];
	updated_at?: number;
}

/** 默认后备动作列表（引擎未就绪时使用） */
const FALLBACK_ACTIONS: string[] = ['荡秋千', '翻花绳'];

/** 行动者角色 */
export class ActorRole extends CreativeRoleBase<ActionDetail> {
	/** 最大推理迭代次数（行动规划可能需要多步） */
	protected MAX_ITERATIONS = 5;

	/** 构造函数 */
	public constructor() {
		super(fileView('prompts/actorRole.md')[0]);
	}

	/** 角色名称 */
	protected get roleName(): string { return '行动者' }

	/** 从引擎获取当前可用动作名称列表 */
	private getAvailableActionNames(): string[] {
		try {
			const raw = getAvailableActions();
			if (!raw || raw === '{}') return FALLBACK_ACTIONS;
			const parsed: AnimationListResponse = JSON.parse(raw);
			if (parsed.actions && Array.isArray(parsed.actions) && parsed.actions.length > 0) {
				return parsed.actions.map(a => a.name);
			}
		} catch {
			// 解析失败，使用后备列表
		}
		return FALLBACK_ACTIONS;
	}

	/** 获取工具定义（动态构建，从引擎获取可用动作列表） */
	protected getToolDefinitions(): ToolCall[] {
		const actionNames = this.getAvailableActionNames();

		return [
			{
				type: "function",
				function: {
					name: "play_action",
					description: "让月华执行预设动作。可用动作：" + actionNames.join('、') + "。",
					parameters: {
						type: "object",
						properties: {
							action_name: {
								type: "string",
								description: "动作名称",
								enum: actionNames
							}
						},
						required: ["action_name"]
					}
				}
			},
			{
				type: "function",
				function: {
					name: "agent_movement",
					description: "控制月华移动到指定3D坐标位置。移动期间会自动关闭鼠标追踪。",
					parameters: {
						type: "object",
						properties: {
							x: { type: "number", description: "目标X坐标" },
							y: { type: "number", description: "目标Y坐标（地面为0）" },
							z: { type: "number", description: "目标Z坐标" },
							resume_tracking: {
								type: "boolean",
								description: "移动结束后是否恢复鼠标追踪，默认为 true"
							}
						},
						required: ["x", "y", "z"]
					}
				}
			},
			{
				type: "function",
				function: {
					name: "query_agent_position",
					description: "查询月华当前在3D场景中的位置坐标。返回{x, y, z}格式坐标。",
					parameters: {
						type: "object",
						properties: {},
						required: []
					}
				}
			}
		];
	}

	/** 执行行动工具调用 */
	protected executeTool(toolCall: ToolCallItem): string {
		const funcName = toolCall.function.name;
		let args: Record<string, any> = {};
		try {
			args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
		} catch (parseError) {
			console.error(`[行动者] 工具调用参数解析失败:`, toolCall.function.arguments);
			return `工具调用参数解析失败: ${parseError}`;
		}

		switch (funcName) {
			case 'play_action': return this.handlePlayAction(args);
			case 'agent_movement': return this.handleAgentMovement(args);
			case 'query_agent_position': return this.handleQueryAgentPosition();
			default: return `未知工具: ${funcName}`;
		}
	}

	/** 从工具调用中收集行动详情 */
	protected collectDetail(toolCall: ToolCallItem, details: ActionDetail[]): void {
		try {
			const args = typeof toolCall.function.arguments === 'string'
				? JSON.parse(toolCall.function.arguments)
				: toolCall.function.arguments;
			const detail: ActionDetail = { toolName: toolCall.function.name };
			if (toolCall.function.name === 'play_action') {
				detail.actionName = args.action_name || '';
			} else if (toolCall.function.name === 'agent_movement') {
				detail.targetPos = `(${args.x}, ${args.y}, ${args.z})`;
			}
			details.push(detail);
		} catch {
			// 解析失败时跳过
		}
	}

	/** 构建行动结果摘要，使用月华话术格式 */
	protected buildSummary(details: ActionDetail[]): string {
		if (details.length === 0) return '月华没有执行任何行动';

		const parts: string[] = [];
		for (const d of details) {
			if (d.toolName === 'query_agent_position') continue; // 查询位置不纳入汇报
			if (d.toolName === 'play_action' && d.actionName) {
				parts.push(`月华${d.actionName}了`);
			} else if (d.toolName === 'agent_movement' && d.targetPos) {
				parts.push(`月华移动到了${d.targetPos}`);
			}
		}

		if (parts.length === 0) return '月华完成了行动任务';
		return parts.join('，') + '。';
	}

	/** 执行预设动作 */
	private handlePlayAction(args: Record<string, any>): string {
		const actionName = args.action_name || '';
		if (!actionName) return '执行动作失败：动作名称不能为空';

		const allowed = this.getAvailableActionNames();
		if (!allowed.includes(actionName)) {
			return `执行动作失败：不支持的动作 "${actionName}"，可用动作为：${allowed.join('、')}`;
		}

		sendToEngine('action', JSON.stringify({ action: actionName }));
		console.log(`[行动者] 执行动作: ${actionName}`);
		return `已执行动作：${actionName}`;
	}

	/** 执行智能体移动 */
	private handleAgentMovement(args: Record<string, any>): string {
		const x = Number(args.x);
		const y = Number(args.y);
		const z = Number(args.z);
		const resumeTracking = args.resume_tracking !== false;

		if (isNaN(x) || isNaN(y) || isNaN(z)) {
			return '移动失败：坐标参数 x、y、z 必须为有效数字';
		}

		sendToEngine('movement', JSON.stringify({
			position: { x, y, z },
			resumeTracking
		}));

		console.log(`[行动者] 移动到 (${x}, ${y}, ${z})，恢复追踪: ${resumeTracking}`);
		return `已移动到 (${x}, ${y}, ${z})`;
	}

	/** 查询智能体位置 */
	private handleQueryAgentPosition(): string {
		const pos = getAgentPosition();
		const result = `当前位置: x=${pos.x.toFixed(2)}, y=${pos.y.toFixed(2)}, z=${pos.z.toFixed(2)}`;
		console.log(`[行动者] ${result}`);
		return result;
	}
}