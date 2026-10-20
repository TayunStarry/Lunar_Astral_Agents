import { GlobalConfig } from '../config/global';
import type { ToolCall } from '../config/tool';

// ==== 琉璃（LTPX）聚合工具（启动枚举 + 调用后刷新 + 按名查详情） ====
// 旧机制要求模型先 list 拿清单、再凭清单拼 tool，实测并不可靠：模型经常跳过 list、
// 或照清单抄错应用名。现在改为：
//   1. 启动（模块加载）时拉取全部可用应用名，直接写进 tool 参数的 enum，模型无需先查询；
//   2. 每次 use_the_program 调用结束后自动重拉一次清单（应用名未变则跳过重建）；
//   3. list 语义替换为 details：按工具名回查该应用的详细说明，不再负责列清单。
// 处理函数在模块加载时注册一次，调用时实时转发琉璃执行。

/** 聚合工具名（必须与琉璃侧 integratedToolsName 一致） */
const CRYSTAL_TOOL_NAME = 'use_the_program';

/** 单个可用应用（工具名 + 能力描述） */
interface CrystalProgram {
	/** 应用名，即 tool 参数的枚举值 */
	name: string;
	/** 应用能力描述，供 details 查询返回 */
	description: string;
}

/** 最近一次同步到的可用应用清单（启动时拉取；每次调用后刷新） */
let programs: CrystalProgram[] = [];

/** 聚合工具定义：tool 参数的 enum 随应用清单刷新；清单未变时复用同一对象 */
let crystalTool: ToolCall = buildCrystalTool(programs);

/** 取应用名清单（枚举值与日志共用） */
function programNames(list: CrystalProgram[]): string[] {
	return list.map(program => program.name);
}

/**
 * 用应用清单构建聚合工具定义
 *
 * 工具名全部写进 tool 参数的 enum：模型在请求里直接看到可选值，不必再调 list
 * 把清单查回来再抄一遍。清单为空时不设 enum（避免空枚举把工具调用锁死）。
 */
function buildCrystalTool(list: CrystalProgram[]): ToolCall {
	const names = programNames(list);
	return {
		type: 'function',
		function: {
			name: CRYSTAL_TOOL_NAME,
			description: [
				'调用星月智能的扩展应用（如文件管理、文件编辑、窗口操作、网络搜索）。',
				'tool 只能取参数枚举中的应用名，填入即表示「打开这个应用」；instruction 填打开后要该应用完成的任务。',
				'只打开应用时只填 tool；需要先了解某个应用的能力时传 details=true 与 tool，此时不执行任何操作。'
			].join(''),
			parameters: {
				type: 'object',
				properties: {
					details: {
						type: 'boolean',
						description: '为 true 时只返回 tool 所指应用的详细说明，不执行任何操作；此时仍需传 tool'
					},
					tool: {
						type: 'string',
						enum: names.length ? names : undefined,
						description: names.length
							? '要使用的应用名，只能取枚举值；只填本项表示打开该应用'
							: '要使用的应用名（当前没有可用应用）'
					},
					instruction: {
						type: 'string',
						description: '打开应用后交给它执行的具体任务，如：进入 D 盘根目录；查询北京今天的天气；把 README.md 移动到 docs 目录。只打开应用时省略；不要填「打开文件管理器」这类与 tool 重复的内容'
					}
				},
				required: names.length ? ['tool'] : []
			}
		}
	};
}

/**
 * 从琉璃读取当前可用应用清单
 *
 * 返回 null 表示「工具链尚未就绪」（响应里没有 tool），调用方保留现有清单——
 * 启动阶段琉璃可能晚于月华就绪，不能把这种情况误判成「应用被全部卸载」而清空枚举。
 */
function fetchCrystalPrograms(): CrystalProgram[] | null {
	try {
		const statusJSON = getLTPXRemoteStatus();
		const status: any = statusJSON ? JSON.parse(statusJSON) : {};
		if (!status?.tool?.name) return null;
		const tools: any[] = Array.isArray(status.tools) ? status.tools : [];
		return tools
			.map(item => ({
				name: String(item?.name || '').trim(),
				description: String(item?.description || '').trim()
			}))
			.filter(program => program.name);
	}
	catch (e) {
		console.error('[LTPX] 应用清单拉取失败:', e);
		return null;
	}
}

/**
 * 重新拉取应用清单
 *
 * 「无更新则跳过」：应用名清单不变时不重建工具定义（保持同一对象引用），
 * 但仍同步最新描述，保证 details 查到的是当前说明。
 *
 * @returns 工具定义是否发生变化
 */
function refreshCrystalPrograms(): boolean {
	const latest = fetchCrystalPrograms();
	if (!latest) return false;
	const oldNames = programNames(programs);
	const newNames = programNames(latest);
	programs = latest;
	// 名字与顺序都未变：无需重建定义（描述变化只影响 details，不动 enum）
	if (oldNames.length === newNames.length && oldNames.every((name, index) => name === newNames[index])) {
		return false;
	}
	crystalTool = buildCrystalTool(programs);
	console.info(`[LTPX] 可用应用清单已更新（${programs.length} 个）：${newNames.join('、') || '无'}`);
	return true;
}

/** 按工具名返回应用详情（details 调用） */
function queryCrystalProgramDetails(name: string): string {
	if (!name) {
		return `查询应用详情失败：details=true 时还需传 tool（应用名）。当前可用应用：${programNames(programs).join('、') || '无'}。`;
	}
	let program = programs.find(item => item.name === name);
	// 缓存里没有：可能是刚安装的包，先重拉一次再判断（避免把新应用误报为不存在）
	if (!program) {
		refreshCrystalPrograms();
		program = programs.find(item => item.name === name);
	}
	if (!program) {
		return `未找到应用 ${name}；当前可用应用：${programNames(programs).join('、') || '无'}。`;
	}
	const description = program.description || '（该应用未提供说明）';
	return `【${program.name}】${description}\n\n用法：只打开该应用传 tool=${program.name}；要它执行任务再填 instruction。`;
}

// ==== 模块级注册：处理函数固定一次，调用时实时转发琉璃 ====

GlobalConfig.LTPfunction.set(CRYSTAL_TOOL_NAME, async (args) => {
	const parsed: any = typeof args === 'string' ? JSON.parse(args || '{}') : (args || {});
	const tool = String(parsed.tool || '').trim();
	const instruction = String(parsed.instruction || '').trim();
	// details：布尔（配合 tool 使用）；也兼容模型把字段直接写成应用名（字符串）
	const detailsArg = parsed.details;
	const detailsName = typeof detailsArg === 'string'
		&& detailsArg !== 'true' && detailsArg !== 'false'
		? detailsArg.trim()
		: '';
	if (detailsArg === true || detailsArg === 'true' || detailsName) {
		return [queryCrystalProgramDetails(detailsName || tool), ''];
	}
	if (!tool) {
		const available = programNames(programs).join('、') || '无';
		return [`调用失败：tool 不能为空，只能取以下应用名：${available}。只打开应用时无需 instruction。`, ''];
	}
	// 异步调用：Go 侧在独立 goroutine 中执行（内置智能体直跑 / 前端包经 /ws 广播），
	// Promise 完成后返回结果，长任务不会阻塞月华事件循环。
	// instruction 为空表示只打开应用，由琉璃侧直接展示页面并回执，不向包派发空指令
	const argsJSON = JSON.stringify({ tool, instruction });
	try {
		return [await callLTPXRemoteTool(CRYSTAL_TOOL_NAME, argsJSON), ''];
	}
	finally {
		// 调用后自动刷新：新装/卸载的包在下一轮请求的枚举里生效（应用名未变则跳过）
		refreshCrystalPrograms();
	}
});

// ==== 启动拉取：模块加载即把全部应用名写进枚举 ====
refreshCrystalPrograms();

/**
 * 读取琉璃聚合工具定义（对话者每次 run 时调用）
 *
 * 正常情况下返回启动时/上次调用后构建的定义；若启动时琉璃尚未就绪导致清单为空，
 * 这里补拉一次，工具链就绪后无需等到下一次工具调用才恢复枚举。
 */
export function getCrystalRemoteTools(): ToolCall[] {
	if (programs.length === 0) refreshCrystalPrograms();
	return [crystalTool];
}
