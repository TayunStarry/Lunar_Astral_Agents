/**
 * 对话者工具轮次护栏的冒烟测试（手工执行，不属于构建产物）
 *
 * 为什么需要它：工具的「轮次上限」是纯运行时状态机，TypeScript 编译看不出问题，
 * 而一旦写错就是**静默失效**——用户看到的是"助手从此不会用工具了"或"永久进行中"。
 * 回归背景（用户实测）：对话者连续调度三次绘制后撞上上限，之后：
 *   1) 触顶那一批 tool_calls 没有配对结果，上下文不配对；
 *   2) 轮次计数不再归零，本次会话后续所有工具调用都被"已达上限"直接丢弃；
 *   3) 流式桥先结束轮次再上报工具，实况屏留下永远"进行中"的幽灵轮次。
 *
 * 本脚本用桩替换模型调用，真的把这段状态机跑一遍。
 *
 * 执行（在本目录）：node dialogue.smoke.cjs
 */
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

const HERE = __dirname;                                     // .../TypeScript/roles
const PKG_ROOT = path.resolve(HERE, '../../../..');         // .../core_personality
const PROMPTS = path.join(PKG_ROOT, 'prompts');
const OUT = fs.mkdtempSync(path.join(os.tmpdir(), 'dialogue-smoke-'));

// ==== 1. 隔离编译（CommonJS，临时目录）====
const tsc = path.join(PKG_ROOT, 'node_modules', 'typescript', 'bin', 'tsc');
if (!fs.existsSync(tsc)) {
	console.error('缺少 typescript 依赖：请先在 core_personality 下执行 npm install');
	process.exit(2);
}
try {
	execFileSync(process.execPath, [
		tsc, path.join(HERE, 'dialogue.ts'),
		'--module', 'commonjs', '--target', 'es2022', '--moduleResolution', 'node',
		'--skipLibCheck', '--removeComments', '--outDir', OUT,
	], { stdio: 'pipe' });
}
catch (error) {
	if (!fs.existsSync(path.join(OUT, 'roles', 'dialogue.js'))) {
		console.error('编译失败：', String(error.stdout || error));
		process.exit(2);
	}
}

// ==== 2. 宿主全局桩 ====
globalThis.fileView = (p) => {
	const file = path.join(PROMPTS, String(p).replace(/^prompts\//, ''));
	if (!fs.existsSync(file)) return ['', new Error('missing ' + p)];
	return [fs.readFileSync(file, 'utf8'), null];
};
globalThis.address = () => ['http://127.0.0.1:0/v1', null];
globalThis.url = () => 'http://127.0.0.1:0';
globalThis.ltpInteractEvent = () => ({ return: undefined });
globalThis.readResource = () => ({ base64: '', mime: 'image/png', lost: false, truncated: false });
globalThis.resourceList = () => [[], null];
globalThis.resourceRead = () => [null, null];
globalThis.resourceGrammar = () => [[], null];
globalThis.listResources = () => [];
// 宿主不提供 resizeImage（思维链不允许自己缩放图片，见 docs/adr/0008）
globalThis.visionRead = () => [null, null];
globalThis.mediaSegment = () => [];
// LTPX 桩：模块加载即拉取应用清单（工具名写进 use_the_program 的枚举）
globalThis.getLTPXRemoteStatus = () => JSON.stringify({
	tool: { name: 'use_the_program' },
	tools: [{ name: 'fake_tool', description: '冒烟测试桩工具' }],
});
globalThis.callLTPXRemoteTool = async () => '桩调用结果';
// 对话缓存写入桩：只记录调用，不落盘
globalThis.saveFile = (name) => [name, name, null];

const { GlobalConfig } = require(path.join(OUT, 'config', 'global.js'));
const { DialogueRole } = require(path.join(OUT, 'roles', 'dialogue.js'));

GlobalConfig.currentAddress = ['http://127.0.0.1:0/v1/chat/completions'];
GlobalConfig.unreadContext = [];
GlobalConfig.finalResponse = '';

// ==== 3. 模型调用桩：按队列返回，记录每次请求体 ====
const requests = [];
let queue = [];
globalThis.syncFetch = (req) => {
	const body = JSON.parse(req.execute.body);
	requests.push(body);
	const next = queue.shift();
	if (!next) throw new Error('模型调用次数超出预期（脚本没排到那么多应答）');
	return [{ status: 200, headers: {}, body: next }, null];
};

/** 造一个「模型要求调用工具」的应答 */
function toolCallResponse(name, id) {
	return {
		choices: [{
			message: {
				role: 'assistant', content: '',
				tool_calls: [{ id, type: 'function', function: { name, arguments: '{}' } }],
			},
		}],
		timings: {},
	};
}
/** 造一个「模型给出正文」的应答 */
function textResponse(text) {
	return { choices: [{ message: { role: 'assistant', content: text } }], timings: {} };
}

// ==== 4. 工具桩：统计真实执行次数 ====
let executed = 0;
GlobalConfig.LTPfunction = new Map();
GlobalConfig.LTPfunction.set('fake_tool', async () => {
	executed++;
	return [`工具执行结果 ${executed}`, ''];
});

const dialogue = new DialogueRole({ lunar: '黑发红瞳', crystal: '银发蓝瞳' });
function newCache() {
	return { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [] };
}

let failures = 0;
function check(label, cond, detail) {
	if (cond) console.log('PASS ' + label);
	else { failures++; console.log('FAIL ' + label + (detail ? ' :: ' + detail : '')); }
}

// 系统提示应把构造时传入的姐妹外观填入占位符
check('系统提示已注入姐妹外观',
	String(dialogue.systemPrompt || '').includes('黑发红瞳') && String(dialogue.systemPrompt || '').includes('银发蓝瞳'),
	String(dialogue.systemPrompt || '').substring(0, 160));

// ==== 5. 撞上限：6 批工具调用（上限 5）+ 一次终稿 ====
(async () => {
	for (let i = 1; i <= 6; i++) queue.push(toolCallResponse('fake_tool', `call_${i}`));
	queue.push(textResponse('整理好了'));
	requests.length = 0;
	executed = 0;

	await dialogue.generateDialogue(newCache());

	check('上限内执行 5 次工具', executed === 5, '实得 ' + executed);
	check('模型请求 6 次带工具 + 1 次不带工具', requests.length === 7, '实得 ' + requests.length);
	check('最后一次请求已摘掉 tools（终稿）', requests.length === 7 && !requests[6].tools, JSON.stringify(Object.keys(requests[6] || {})));
	check('最终应答来自终稿', GlobalConfig.finalResponse === '整理好了', JSON.stringify(GlobalConfig.finalResponse));

	// 每个 tool_call_id 都必须有配对结果（5 个真执行 + 1 个"未执行"）
	const toolMessages = dialogue.messages.filter(m => m.role === 'tool');
	const ids = new Set(toolMessages.map(m => m.tool_call_id));
	check('6 个 tool_call 全部有配对结果', ids.size === 6 && ids.has('call_6'), JSON.stringify([...ids]));
	const skipped = toolMessages.find(m => m.tool_call_id === 'call_6');
	check('触顶那批标记为未执行', !!skipped && String(skipped.content).includes('未执行'), skipped && skipped.content);
	check('上下文中不再残留未配对的 tool_calls', dialogue.messages.filter(m => m.tool_calls).every(m => {
		return (m.tool_calls || []).every(tc => ids.has(tc.id));
	}), '存在无结果的 tool_call');

	// ==== 6. 下一轮应答：计数必须归零，工具能再次执行 ====
	queue.push(toolCallResponse('fake_tool', 'call_next'));
	queue.push(textResponse('第二轮答完'));
	requests.length = 0;
	executed = 0;
	GlobalConfig.unreadContext = [{ role: 'user', content: '再画一张' }];

	await dialogue.generateDialogue(newCache());

	check('新一轮仍能执行工具（轮次计数已归零）', executed === 1, '实得 ' + executed);
	check('新一轮能正常收尾', GlobalConfig.finalResponse === '第二轮答完', JSON.stringify(GlobalConfig.finalResponse));

	// ==== 7. 模型调用失败：可读原因 + 上报宿主（供自动重载决策） ====
	// 回归背景：事故日志里只有一句「请求处理错误: {}」——Go 侧 error 经 goja 导出后
	// 没有可枚举字段，JSON 化就是 {}；真实原因（模型服务崩了）因此完全丢失。
	const reported = [];
	globalThis.notifyAgentFailure = (reason) => reported.push(reason);
	let idleCount = 0;
	globalThis.notifyAgentIdle = () => { idleCount++; };

	// 模拟 Go 侧 error：没有 message 属性，只有 String() 给出文本（与 goja 的导出一致）
	const goStyleError = { toString: () => 'Could not establish connection' };
	queue = [];
	const origFetch = globalThis.syncFetch;
	globalThis.syncFetch = () => [null, goStyleError];
	GlobalConfig.lastFailure = '';
	GlobalConfig.finalResponse = '';
	GlobalConfig.unreadContext = [{ role: 'user', content: '还在吗' }];

	await dialogue.generateDialogue(newCache());

	check('失败原因被解析成可读文本', GlobalConfig.lastFailure === 'Could not establish connection',
		JSON.stringify(GlobalConfig.lastFailure));
	check('失败已上报宿主（触发自动重载判据）',
		reported.length === 1 && reported[0] === 'Could not establish connection', JSON.stringify(reported));
	check('失败轮不会产出假应答', GlobalConfig.finalResponse === '', JSON.stringify(GlobalConfig.finalResponse));
	globalThis.syncFetch = origFetch;

	// ==== 8. 成功一轮：清零主机连续失败计数 ====
	idleCount = 0;
	queue = [textResponse('在的')];
	GlobalConfig.unreadContext = [{ role: 'user', content: '在吗' }];
	await dialogue.generateDialogue(newCache());
	check('成功一轮会通知宿主清零失败计数', idleCount >= 1, String(idleCount));

	// ==== 9. 高耗时任务快速应答：一轮一次、走 TTS、QQ 不占应答份额 ====
	const ackPushed = [];
	globalThis.tts = () => ['audio-b64', null];
	globalThis.pushContext = (type, content, audio, auto) => {
		ackPushed.push({ type, content, audio, auto });
		return true;
	};
	GlobalConfig.LTPfunction.set('dispatch_painter', async () => {
		executed++;
		return ['画好了', ''];
	});

	ackPushed.length = 0;
	executed = 0;
	GlobalConfig.fastAckSent = false;
	GlobalConfig.turnFromQQ = false;
	GlobalConfig.unreadContext = [];
	requests.length = 0;
	queue = [
		toolCallResponse('dispatch_painter', 'paint_1'),
		toolCallResponse('dispatch_painter', 'paint_2'),
		textResponse('画好了'),
	];
	await dialogue.generateDialogue(newCache());
	check('高耗时工具执行前发送快速应答', ackPushed.length === 1, JSON.stringify(ackPushed));
	check('快速应答走 TTS', ackPushed[0] && ackPushed[0].audio === 'audio-b64', JSON.stringify(ackPushed[0]));
	check('前端来源的快速应答 auto=false', ackPushed[0] && ackPushed[0].auto === false, JSON.stringify(ackPushed[0]));
	check('同一轮多次绘图只触发一次', executed === 2 && ackPushed.length === 1, `executed=${executed}, ack=${ackPushed.length}`);
	// 回归：工具结果后不得注入「继续」——它会被模型当成新的创作指令，
	// 实测一次画图请求因此连画 4 张。请求末尾应仍是 tool 结果。
	{
		const followUp = requests[1] || { messages: [] };
		const lastMessage = followUp.messages[followUp.messages.length - 1] || {};
		check('工具结果后直接请求收尾（不注入「继续」）', lastMessage.role === 'tool', JSON.stringify(lastMessage));
	}

	ackPushed.length = 0;
	GlobalConfig.fastAckSent = false;
	GlobalConfig.turnFromQQ = true;
	GlobalConfig.unreadContext = [];
	queue = [toolCallResponse('dispatch_painter', 'paint_3'), textResponse('画好了')];
	await dialogue.generateDialogue(newCache());
	check('QQ 来源的快速应答 auto=true（不占应答份额）',
		ackPushed.length === 1 && ackPushed[0].auto === true, JSON.stringify(ackPushed));

	// ==== 10. 追加设定：逐行拼在提示词末尾；空配置不残留 ====
	GlobalConfig.customConfig = { agent: { append_settings: ['喜欢猫', '讨厌被叫 AI'] } };
	const appendRole = new DialogueRole({ lunar: '月华外观', crystal: '琉璃外观' });
	appendRole.applyAppendSettings();
	check('追加设定逐行附加到提示词末尾',
		appendRole.systemPrompt.endsWith('喜欢猫\n讨厌被叫 AI'), JSON.stringify(appendRole.systemPrompt.slice(-40)));
	GlobalConfig.customConfig = { agent: {} };
	appendRole.applyAppendSettings();
	check('未配置追加设定时不残留内容',
		!appendRole.systemPrompt.includes('喜欢猫') && !appendRole.systemPrompt.includes('追加设定'),
		JSON.stringify(appendRole.systemPrompt.slice(-40)));

	fs.rmSync(OUT, { recursive: true, force: true });
	console.log(failures === 0 ? '\n全部通过' : '\n' + failures + ' 项失败');
	process.exit(failures === 0 ? 0 : 1);
})().catch((error) => {
	console.error('冒烟测试异常：', error);
	process.exit(1);
});
