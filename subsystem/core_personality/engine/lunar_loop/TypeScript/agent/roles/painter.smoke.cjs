/**
 * 绘制者工具链冒烟测试（手工执行，不属于构建产物）
 *
 * 为什么需要它：绘制者的「工具名 → 处理函数 → 提示词底板」这条链是**纯字符串约定**，
 * TypeScript 编译只保证类型正确，编译不出「工具没注册」「模板取错人设」「默认服装失效」
 * 这类错误；Go 侧的 `TestBundlePromptReferencesAreEmbedded` 只能证明提示词文件存在。
 * 本脚本把 painter.ts 隔离编译成 CommonJS，用桩替换宿主全局，然后真的调用工具处理函数，
 * 断言生成参数（提示词内容 / cfg / 尺寸 / initImg）与汇报话术。
 *
 * 执行（在本目录）：node painter.smoke.cjs
 * 依赖：core_personality/node_modules 里的 typescript（已在 devDependencies 中）
 */
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

const HERE = __dirname;                                     // .../TypeScript/agent/roles
const PKG_ROOT = path.resolve(HERE, '../../../../..');      // .../core_personality
const PROMPTS = path.join(PKG_ROOT, 'prompts');
const OUT = fs.mkdtempSync(path.join(os.tmpdir(), 'painter-smoke-'));

// ==== 1. 把 painter.ts 隔离编译成 CommonJS（临时目录，不污染源码树）====
const tsc = path.join(PKG_ROOT, 'node_modules', 'typescript', 'bin', 'tsc');
if (!fs.existsSync(tsc)) {
	console.error('缺少 typescript 依赖：请先在 core_personality 下执行 npm install');
	process.exit(2);
}
try {
	execFileSync(process.execPath, [
		tsc, path.join(HERE, 'painter.ts'),
		'--module', 'commonjs', '--target', 'es2022', '--moduleResolution', 'node',
		'--skipLibCheck', '--removeComments', '--outDir', OUT,
	], { stdio: 'pipe' });
}
catch (error) {
	// 缺 globals.d.ts 时会报「找不到 fileView 等全局」——不影响产物生成，只在真编译错误时拦下
	if (!fs.existsSync(path.join(OUT, 'agent', 'roles', 'painter.js'))) {
		console.error('编译失败：', String(error.stdout || error));
		process.exit(2);
	}
}

// ==== 2. 宿主全局桩（只实现绘制路径会碰到的那些）====
const captured = [];
globalThis.fileView = (p) => {
	const file = path.join(PROMPTS, String(p).replace(/^prompts\//, ''));
	if (!fs.existsSync(file)) return ['', new Error('missing ' + p)];
	return [fs.readFileSync(file, 'utf8'), null];
};
globalThis.generateImage = (params) => {
	captured.push(params);
	return [{ path: 'generated/test.png', base64: 'iVBORw0KGgo=', width: params.width, height: params.height, seed: 12345 }, null];
};
globalThis.pushImage = () => true;
globalThis.readResource = () => ({ base64: '', mime: 'image/png', lost: false, truncated: false });
globalThis.listResources = () => [];
// 视觉引用固定走 /file/read-multimodal/ 端点形态
globalThis.url = () => ['http://127.0.0.1:0'];
// 图片入库桩：返回一个句柄 + blob 路径，供 attachImageTags 改写为视觉副本
globalThis.resourceAdmit = () => [{ handle: '@I.900', id: 900, blob_path: 'multimedia/stub.png' }, null];
globalThis.upscaleImage = () => [null, null];
globalThis.ltpInteractEvent = () => ({ return: undefined });
// 视觉读取桩：记录调用参数，返回固定副本（宽高与原图等比，供比例推导）
const VISION_URI = 'data:image/jpeg;base64,/9j/4AAQSkZJRg==';
const visionCalls = [];
globalThis.visionRead = (relPath, tag) => {
	visionCalls.push({ relPath, tag });
	return [VISION_URI, null, 800, 1066];
};

// ==== 3. 加载并实例化绘制者 ====
const { GlobalConfig } = require(path.join(OUT, 'config', 'global.js'));
GlobalConfig.currentAddress = ['http://127.0.0.1:0/v1/chat/completions'];
const { PainterRole } = require(path.join(OUT, 'agent', 'roles', 'painter.js'));
const painter = new PainterRole();

const tools = painter.getToolDefinitions().map(t => t.function.name);
console.log('工具清单:', tools.join(', '));

function call(name, args) {
	captured.length = 0;
	const out = painter.executeTool({ function: { name, arguments: JSON.stringify(args) } });
	return { out, params: captured[0] };
}

let failures = 0;
function check(label, cond, detail) {
	if (cond) console.log('PASS ' + label);
	else { failures++; console.log('FAIL ' + label + (detail ? ' :: ' + detail : '')); }
}

// ==== 4. 工具面：四个立绘工具已合并为一个 portrait ====
check('工具清单含 portrait', tools.includes('portrait'), tools.join(','));
for (const gone of ['self_portrait', 'chibi_self_portrait', 'crystal_portrait', 'chibi_crystal_portrait']) {
	check('旧工具已移除: ' + gone, !tools.includes(gone), tools.join(','));
}
check('落选工具仍可用（image_to_image）', tools.includes('image_to_image'), tools.join(','));

const BASE_ARGS = { expression: '俏皮的表情,单眼眨眼', posture: '一手比V字手势', environment: '淡蓝色天空与星光' };

// ==== 5. 月华（全尺寸 / Q版） ====
{
	const { out, params } = call('portrait', { ...BASE_ARGS, character: '月华' });
	check('月华全尺寸未返回失败', !String(out).includes('失败'), String(out));
	if (params) {
		check('月华底板（深红瞳 + 绯红发梢）', params.prompt.includes('深红色眼睛') && params.prompt.includes('绯红'), params.prompt);
		check('月华不含琉璃特征', !params.prompt.includes('蓝紫色眼睛'), params.prompt);
		check('全尺寸缺省 2:3 → 1024x1536', params.width === 1024 && params.height === 1536, params.width + 'x' + params.height);
		check('提示词无换行', !params.prompt.includes('\n'), params.prompt);
		check('cfg=1.0 且无 initImg', params.cfgScale === 1.0 && !params.initImg, String(params.initImg));
	} else check('月华全尺寸产出生成参数', false);
}
{
	const { params } = call('portrait', { ...BASE_ARGS, character: '月华', chibi: true });
	if (params) {
		check('月华Q版模板含二头身', params.prompt.includes('二头身'), params.prompt);
		check('Q版缺省 1:1 → 1024x1024', params.width === 1024 && params.height === 1024, params.width + 'x' + params.height);
	} else check('月华Q版产出生成参数', false);
}

// ==== 6. 琉璃（全尺寸 / Q版） ====
{
	const { params } = call('portrait', { ...BASE_ARGS, character: '琉璃', ratio: '2:3' });
	if (params) {
		check('琉璃底板（银白 + 淡蓝）', params.prompt.includes('银白') && params.prompt.includes('淡蓝'), params.prompt);
		check('琉璃默认服装（星星裙 + 白外套 + 乐福鞋）',
			params.prompt.includes('星星') && params.prompt.includes('外套') && params.prompt.includes('乐福鞋'), params.prompt);
		check('琉璃不含月华特征', !params.prompt.includes('绯红') && !params.prompt.includes('哥特'), params.prompt);
	} else check('琉璃全尺寸产出生成参数', false);
}
{
	const { params } = call('portrait', { ...BASE_ARGS, character: '琉璃', chibi: true, outfit: '白色兔耳卫衣' });
	if (params) {
		check('琉璃Q版含二头身', params.prompt.includes('二头身'), params.prompt);
		check('自定义 outfit 覆盖默认服装', params.prompt.includes('兔耳卫衣'), params.prompt);
		check('Q版仍是琉璃底板', params.prompt.includes('银白') && params.prompt.includes('蓝紫色眼睛'), params.prompt);
	} else check('琉璃Q版产出生成参数', false);
}

// ==== 7. 通用角色（男女底板折进角色枚举） ====
{
	const { params } = call('portrait', {
		...BASE_ARGS, character: '通用-女',
		appearance: '银发蓝眼的精灵少女,尖耳朵,穿深绿色斗篷,气质清冷',
		expression: '平静的表情', environment: '幽深森林',
	});
	if (params) {
		check('通用模板用「1女孩」', params.prompt.includes('1女孩'), params.prompt);
		check('通用女性底板生效', params.prompt.includes('少女体型'), params.prompt);
		check('绘制者外观描述已注入', params.prompt.includes('精灵少女') && params.prompt.includes('深绿色斗篷'), params.prompt);
		check('其他角色不混入月华/琉璃人设',
			!params.prompt.includes('绯红') && !params.prompt.includes('蓝紫色眼睛') && !params.prompt.includes('星星发饰'), params.prompt);
		check('空槽位不留连续逗号', !params.prompt.includes('，，'), params.prompt);
	} else check('通用角色（女）产出生成参数', false);
}
{
	const { params } = call('portrait', {
		...BASE_ARGS, character: '通用-男',
		appearance: '黑发少年,戴眼镜,穿深色学院制服',
		expression: '自信的表情', environment: '黄昏校舍',
	});
	if (params) {
		check('通用模板用「1男孩」', params.prompt.includes('1男孩'), params.prompt);
		check('通用男性底板生效', params.prompt.includes('少年体型'), params.prompt);
	} else check('通用角色（男）产出生成参数', false);
}
{
	// 枚举未生效时模型可能把具体人名填进 character：当作通用角色并把名字并入外观，不报错
	const { out, params } = call('portrait', { ...BASE_ARGS, character: '艾莉', appearance: '银发精灵' });
	check('未识别角色降级为通用角色', !!params && !String(out).includes('失败'), String(out));
	if (params) check('未识别角色名并入外观描述', params.prompt.includes('名为「艾莉」的角色'), params.prompt);
}
{
	const { params } = call('portrait', { ...BASE_ARGS, character: '通用-女', appearance: '' });
	if (params) {
		check('缺 appearance 时仍可出图（兜底服装）', params.prompt.includes('日常服装'), params.prompt);
	} else check('通用角色（缺外观）产出生成参数', false);
}

// ==== 8. 汇报话术 ====
{
	const cases = [
		[{ toolName: 'portrait', character: '月华', chibi: false, expression: '微笑', environment: '窗边' }, '全尺寸自画像'],
		[{ toolName: 'portrait', character: '月华', chibi: true, expression: '微笑', environment: '窗边' }, 'Q版自画像'],
		[{ toolName: 'portrait', character: '琉璃', chibi: false, expression: '微笑', environment: '星空' }, '为琉璃绘制了一幅全尺寸立绘'],
		[{ toolName: 'portrait', character: '琉璃', chibi: true, expression: '微笑', environment: '星空' }, '为琉璃绘制了一幅Q版立绘'],
		[{ toolName: 'portrait', character: '通用-女', chibi: false, expression: '微笑', environment: '森林' }, '女性角色立绘'],
		[{ toolName: 'portrait', character: '通用-男', chibi: true, expression: '微笑', environment: '校舍' }, 'Q版男性角色立绘'],
	];
	for (const [detail, expect] of cases) {
		const text = painter.buildSummary([detail]);
		check(`汇报话术含「${expect}」`, text.includes(expect), text);
	}
}

// ==== 9. 图生图 / 合照不受影响 ====
{
	painter.referenceImage = 'multimedia/ref.png';
	painter.referenceHandle = '@I.001';
	painter.referenceSize = { width: 768, height: 1024 };
	const { params } = call('image_to_image', { prompt: 'a girl in snow', strength: 0.85 });
	if (params) {
		check('图生图未给 cfg → 1.0（引导蒸馏模型）', params.cfgScale === 1.0, String(params.cfgScale));
		check('图生图带 initImg 与 strength', !!params.initImg && params.strength === 0.85, JSON.stringify({ initImg: params.initImg, strength: params.strength }));
		check('尺寸随参考图比例（768x1024 → 960x1280）', params.width === 960 && params.height === 1280, params.width + 'x' + params.height);
	} else check('image_to_image 产出生成参数', false);
	painter.resetReference();
}
{
	const { params } = call('group_photo', { companion: '琉璃', expression: '微笑', posture: '并肩站立', environment: '星空下' });
	if (params) check('合照对象为琉璃时注入内置外观', params.prompt.includes('银白') && params.prompt.includes('星星'), params.prompt);
	else check('group_photo 产出生成参数', false);
}

// ==== 10. 参考图只以视觉副本进入任务消息（resizeImage 已从运行时卸载）====
//
// 这条守的是 0:23 / 1:11 / 1:18 三次 llama-server 断言退出的真实病因：
// 绘制者把 3072×4096 原图 base64 直接塞进 image_url，视觉 token 超出 ubatch，
// 多模态实例整个退出。修复后模型只能看到 visionRead 产出的 ≤vision_max_side 副本。
(async () => {
	// 宿主不再提供 resizeImage：任何一处调用都会在这里炸成 ReferenceError
	check('resizeImage 未暴露给智能体运行时', typeof globalThis.resizeImage === 'undefined', typeof globalThis.resizeImage);

	visionCalls.length = 0;
	painter.messages.length = 0;
	await painter.createCreativeWork('画一个魔幻风格的运算设备', {
		handle: '@I.016',
		source: 'multimedia/1a36dad78319a588.png',
	});
	check('参考图经 visionRead 探测副本（不再读原图）',
		visionCalls.length === 1
		&& visionCalls[0].relPath === 'multimedia/1a36dad78319a588.png'
		&& visionCalls[0].tag === '@I.016',
		JSON.stringify(visionCalls));

	const taskMessage = painter.messages[0];
	const imageItem = taskMessage && Array.isArray(taskMessage.content)
		? taskMessage.content.find(item => item.type === 'image_url')
		: null;
	check('任务消息里的参考图是 /file/read-multimodal/ 端点 URL',
		!!imageItem && imageItem.image_url.url.includes('/file/read-multimodal/'),
		imageItem && imageItem.image_url.url);
	check('任务消息不含原图字节（无 3072 级 base64）',
		!JSON.stringify(taskMessage || {}).includes('AAAA') && JSON.stringify(taskMessage || {}).length < 4096,
		String(JSON.stringify(taskMessage || {}).length));
	check('参考图比例取自副本（800x1066）',
		painter.referenceSize && painter.referenceSize.width === 800 && painter.referenceSize.height === 1066,
		JSON.stringify(painter.referenceSize));

	// 作品回写历史同样必须走入库 + 视觉读取形态；入库失败时退化为文本占位
	painter.messages.length = 0;
	painter.recordPainting('扩散图像生成', 'iVBORw0KGgoAAAANSUhEUg==');
	const paintingMessage = painter.messages[0];
	const paintingImage = paintingMessage && Array.isArray(paintingMessage.content)
		? paintingMessage.content.find(item => item.type === 'image_url')
		: null;
	const paintingURL = (paintingImage && paintingImage.image_url.url) || '';
	check('画作回写历史只走 /file/read-multimodal/ 端点 URL',
		paintingURL.includes('/file/read-multimodal/'),
		paintingURL);
	check('画作消息里不残留原图 base64',
		!JSON.stringify(paintingMessage || {}).includes('iVBORw0KGgoAAAANSUhEUg=='),
		JSON.stringify(paintingMessage || {}).substring(0, 160));

	fs.rmSync(OUT, { recursive: true, force: true });
	console.log(failures === 0 ? '\n全部通过' : '\n' + failures + ' 项失败');
	process.exit(failures === 0 ? 0 : 1);
})();
