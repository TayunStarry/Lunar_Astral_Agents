import { GlobalConfig } from '../config/global';
import type { ToolCall } from '../config/tool';

// ==== 琉璃（LTPX）聚合工具（清单按需查询） ====
// 工具定义为固定瘦身版：description 不内嵌应用清单，避免每轮模型调用都在 tools
// 字段里携带大段描述。可用应用清单改为经 list 参数按需查询（实时拉取进程内工具链，
// 包增删即时反映），由月华自行决定何时读取、读不读。
// 处理函数在模块加载时注册一次，调用时实时转发琉璃执行。

/** 聚合工具名（必须与琉璃侧 integratedToolsName 一致） */
const CRYSTAL_TOOL_NAME = 'use_the_program';

/** 瘦身工具定义：清单不进 description，经 list 参数按需获取 */
const crystalTool: ToolCall = {
	type: 'function',
	function: {
		name: CRYSTAL_TOOL_NAME,
		description: [
			'调用星月智能的扩展应用（如文件管理、文件编辑、窗口操作、网络搜索）。',
			'tool 填应用名，本身即表示「打开这个应用」；instruction 填打开后要该应用完成的任务。',
			'只打开应用时只填 tool；查询有哪些应用时只传 list=true，不要传 tool 或 instruction。'
		].join(''),
		parameters: {
			type: 'object',
			properties: {
				list: {
					type: 'boolean',
					description: '仅查询可用应用清单，不执行任何操作。为 true 时不要传 tool 或 instruction（即使传了也会被忽略）'
				},
				tool: {
					type: 'string',
					description: '要使用的应用名，必须取自 list 清单（如 file_manager、file_editor）。只填本项表示打开该应用'
				},
				instruction: {
					type: 'string',
					description: '打开应用后交给它执行的具体任务，如：进入 D 盘根目录；查询北京今天的天气；把 README.md 移动到 docs 目录。只打开应用时省略；不要填「打开文件管理器」这类与 tool 重复的内容'
				}
			},
			required: []
		}
	}
};

/** 读取琉璃聚合工具定义（对话者每次 run 时拼接；定义为固定瘦身版，不再随工具链刷新） */
export function getCrystalRemoteTools(): ToolCall[] {
	return [crystalTool];
}

/** 工具链状态对象（工具链未就绪时 tool 缺省） */
type LTPXToolchainStatus = {
	/** 聚合工具（description 内嵌「- 应用名：功能简述」清单） */
	tool?: {
		name: string;
		description?: string;
	};
};

/**
 * 查询当前可用的扩展应用清单（list=true 时调用）
 *
 * 实时拉取进程内工具链（getLTPXRemoteStatus 每次动态扫描，包增删即时反映），
 * 从聚合工具 description 中提取「- 应用名：功能简述」清单行作为查询结果。
 */
function queryCrystalToolchain(): string {
	try {
		const statusJSON = getLTPXRemoteStatus();
		const status: LTPXToolchainStatus = statusJSON ? JSON.parse(statusJSON) : {};
		const lines = String(status.tool?.description || '')
			.split('\n')
			.map(line => line.trim())
			.filter(line => line.startsWith('- '));
		if (!lines.length) return '当前没有可用的扩展应用（琉璃侧工具链为空）。';
		return `【可用应用清单（共 ${lines.length} 个，本次仅查询，尚未打开任何应用）】\n${lines.join('\n')}\n\n选定后再次调用本工具：只打开应用只填 tool；要让应用执行任务，再填 instruction 说明任务。`;
	}
	catch (e) {
		console.error('[LTPX] 应用清单查询失败:', e);
		return '查询扩展应用清单失败，请稍后重试。';
	}
}

// ==== 模块级注册：处理函数固定一次，调用时实时转发琉璃 ====

GlobalConfig.LTPfunction.set(CRYSTAL_TOOL_NAME, async (args) => {
	const parsed: any = typeof args === 'string' ? JSON.parse(args || '{}') : (args || {});
	const tool = String(parsed.tool || '').trim();
	const instruction = String(parsed.instruction || '').trim();
	// list 查询：只返回实时清单，不发起实际调用；误传的 tool/instruction 一律忽略并提示
	if (parsed.list) {
		const note = (tool || instruction)
			? '\n\n提示：list=true 只查询清单，本次传入的 tool/instruction 已忽略；确定应用名后再单独调用（只打开应用只填 tool）。'
			: '';
		return [queryCrystalToolchain() + note, ''];
	}
	if (!tool) {
		return ['调用失败：tool 不能为空。请先传 list=true 查询可用应用清单，再从中选择应用名填入 tool；只打开应用时无需 instruction。', ''];
	}
	// 异步调用：Go 侧在独立 goroutine 中执行（内置智能体直跑 / 前端包经 /ws 广播），
	// Promise 完成后返回结果，长任务不会阻塞月华事件循环。
	// instruction 为空表示只打开应用，由琉璃侧直接展示页面并回执，不向包派发空指令
	const argsJSON = JSON.stringify({ tool, instruction });
	const text = await callLTPXRemoteTool(CRYSTAL_TOOL_NAME, argsJSON);
	return [text, ''];
});
