import { Live2D, EmotionalStateEnum } from './live2d.js';
import { sendMessages } from './fetch.js';

/**
 * 触摸提示词素材库
 *
 * 四要素组合: <力量> + <动作> + <部位> + <态度>
 * 格式: "<力量><动作>了<部位>, 请做出<态度>的反应"
 *
 * 力量与动作始终保持随机加权，部位由分区决定。
 */
export const TOUCH_PROMPT_CONFIG = {
	/** 力量等级（5级，从轻到重） */
	force: [
		{ value: '轻轻', weight: 3 },
		{ value: '温柔', weight: 3 },
		{ value: '稍微', weight: 2 },
		{ value: '用力', weight: 1 },
		{ value: '使劲', weight: 1 },
	],
	/** 核心动作（手法） */
	action: [
		{ value: '摸', weight: 3 },
		{ value: '揉', weight: 2 },
		{ value: '捏', weight: 2 },
		{ value: '挠', weight: 2 },
		{ value: '拍', weight: 2 },
		{ value: '戳', weight: 1 },
		{ value: '抚', weight: 2 },
		{ value: '弹', weight: 1 },
	],
	/** 态度/情绪 */
	attitude: [
		{ value: '好奇', weight: 2 },
		{ value: '疑惑', weight: 2 },
		{ value: '不适', weight: 1 },
		{ value: '高兴', weight: 3 },
		{ value: '害羞', weight: 2 },
		{ value: '生气', weight: 1 },
		{ value: '惊讶', weight: 2 },
		{ value: '享受', weight: 1 },
	],
	/**
	 * 不合法组合规则
	 * 键为"力量|部位"或"动作|部位"，命中时重新生成
	 */
	excludeCombinations: new Set([
		'用力|脸颊', '使劲|脸颊',  // 敏感部位不能用重手
		'用力|胸部', '使劲|胸部',
		'用力|腹部', '使劲|腹部',
		'用力|臀部', '使劲|臀部',  // 敏感部位不能用重手
		'用力|胯部', '使劲|胯部',
		'用力|屁股', '使劲|屁股',
		'捏|裙子', '揉|裙子', '挠|裙子', '戳|裙子', '弹|裙子',  // 衣物不适合捏揉
		'捏|外套', '揉|外套', '挠|外套', '戳|外套', '弹|外套',
		'弹|胸部', '弹|腹部', '弹|大腿',  // 弹不适合大面积部位
		'弹|臀部', '弹|胯部', '弹|屁股',
		'弹|右大臂', '弹|右小臂', '弹|右臂',  // 弹不适合手臂大面积部位
		'弹|左大臂', '弹|左小臂', '弹|左臂',
	]),
};

/**
 * 触摸分区配置
 *
 * 5行×3列网格对应模型不同身体部位，每个分区拥有独立的部位素材池。
 * 力量与手法依旧从 TOUCH_PROMPT_CONFIG 随机抽取，仅部位由分区决定。
 */
export const TOUCH_REGIONS = {
	/** 脸蛋/头发：头部区域 */
	face_hair: {
		label: '脸蛋/头发',
		parts: [
			{ value: '头发', weight: 3 },
			{ value: '头顶', weight: 3 },
			{ value: '脸颊', weight: 2 },
			{ value: '发梢', weight: 2 },
		],
	},
	/** 右臂：右侧上肢 */
	right_arm: {
		label: '右臂',
		parts: [
			{ value: '右大臂', weight: 2 },
			{ value: '右小臂', weight: 2 },
			{ value: '右臂', weight: 1 },
		],
	},
	/** 胸部/腹部：躯干上部 */
	chest_belly: {
		label: '胸部/腹部',
		parts: [
			{ value: '胸部', weight: 1 },
			{ value: '腹部', weight: 1 },
		],
	},
	/** 左臂：左侧上肢 */
	left_arm: {
		label: '左臂',
		parts: [
			{ value: '左大臂', weight: 2 },
			{ value: '左小臂', weight: 2 },
			{ value: '左臂', weight: 1 },
		],
	},
	/** 右手：右侧手部 */
	right_hand: {
		label: '右手',
		parts: [
			{ value: '右手', weight: 3 },
			{ value: '右手背', weight: 1 },
			{ value: '右手指', weight: 1 },
		],
	},
	/** 臀部/胯部：躯干下部 */
	hip: {
		label: '臀部/胯部',
		parts: [
			{ value: '臀部', weight: 1 },
			{ value: '胯部', weight: 1 },
		],
	},
	/** 左手：左侧手部 */
	left_hand: {
		label: '左手',
		parts: [
			{ value: '左手', weight: 3 },
			{ value: '左手背', weight: 1 },
			{ value: '左手指', weight: 1 },
		],
	},
	/** 大腿：腿部上部 */
	thigh: {
		label: '大腿',
		parts: [
			{ value: '大腿', weight: 3 },
		],
	},
	/** 小腿/脚部：腿部下部 */
	leg_foot: {
		label: '小腿/脚部',
		parts: [
			{ value: '小腿', weight: 2 },
			{ value: '脚', weight: 2 },
			{ value: '脚踝', weight: 1 },
		],
	},
};

/**
 * 加权随机选择
 *
 * @param {Array<{ value: string; weight: number }>} items - 带权重的选项数组
 * @returns {string} - 选中的值
 */
export function weightedRandom(items) {
	const totalWeight = items.reduce((sum, item) => sum + item.weight, 0);
	let random = Math.random() * totalWeight;
	for (const item of items) {
		random -= item.weight;
		if (random <= 0) {
			return item.value;
		}
	}
	return items[items.length - 1].value;
}

/**
 * 态度→情绪状态映射
 *
 * 将触摸提示词中的态度词映射到 EmotionalStateEnum，
 * 用于点击时触发对应的 Live2D 动作。
 */
const ATTITUDE_TO_EMOTION = {
	'好奇': 'QUESTION',
	'疑惑': 'QUESTION',
	'不适': 'SHY',
	'高兴': 'HAPPY',
	'害羞': 'SHY',
	'生气': 'ANGRY',
	'惊讶': 'QUESTION',
	'享受': 'HAPPY',
};

/**
 * 全局回退部位池（仅在未识别分区时使用）
 */
const FALLBACK_PART_POOL = [
	{ value: '头发', weight: 3 },
	{ value: '头顶', weight: 3 },
	{ value: '脸颊', weight: 2 },
	{ value: '发梢', weight: 2 },
	{ value: '手', weight: 2 },
	{ value: '大臂', weight: 1 },
	{ value: '小臂', weight: 1 },
];

/**
 * 生成随机触摸提示词
 *
 * 力量与态度从全局素材库随机加权抽取；
 * 部位由分区决定（从分区部位池随机选择），未指定分区时回退到全局部位池。
 * 通过排除规则过滤不合法组合，确保生成质量。
 *
 * @param {string} [region] - 分区标识（对应 TOUCH_REGIONS 的键名）
 * @returns {{ prompt: string, attitude: string }} - 提示词及态度词
 */
export function generateTouchPrompt(region) {
	const config = TOUCH_PROMPT_CONFIG;
	let force, action, part, attitude;
	let attempts = 0;
	const maxAttempts = 50;

	// 分区指定时使用分区专属部位池，否则回退到全局部位池
	const regionConfig = region ? TOUCH_REGIONS[region] : null;
	const partPool = regionConfig?.parts || FALLBACK_PART_POOL;

	// 循环生成直到获得合法组合
	do {
		force = weightedRandom(config.force);
		action = weightedRandom(config.action);
		part = weightedRandom(partPool);
		attitude = weightedRandom(config.attitude);
		attempts++;
	} while (
		(config.excludeCombinations.has(`${force}|${part}`) ||
			config.excludeCombinations.has(`${action}|${part}`)) &&
		attempts < maxAttempts
	);

	// 格式: "<力量><动作>了<部位>, 请做出<态度>的反应"
	return {
		prompt: `${force}${action}了你的${part}, 请做出${attitude}的反应`,
		attitude,
	};
}

/**
 * 触摸交互处理器（分区版）
 *
 * 在Live2D模型区域上覆盖一层 5×3 分区网格，每个分区对应独立身体部位。
 * 通过事件委托识别点击位置所属分区，触发该分区专属的触摸反应事件。
 * 力量与手法依旧保持随机加权抽取，仅部位由分区决定。
 * 支持PC端（click）和移动端（touchstart/touchend）事件。
 */
export class TouchInteractionHandler {
	touchInteraction = null;
	touchRipple = null;
	touchStartTime = 0;
	touchStartPos = { x: 0, y: 0 };

	/**
	 * @param {HTMLElement} touchInteraction - 触摸交互层元素（包含分区子节点）
	 * @param {HTMLElement} touchRipple - 涟漪动画元素
	 * @param {{ setLoadingState: (loading: boolean) => void, showError: (msg: string) => void, isLoading: () => boolean }} callbacks - 回调函数
	 */
	constructor(touchInteraction, touchRipple, callbacks) {
		this.touchInteraction = touchInteraction;
		this.touchRipple = touchRipple;
		this.callbacks = callbacks;
		this.init();
	}

	/**
	 * 初始化触摸交互事件监听
	 *
	 * 使用事件委托绑定在父层，通过 e.target.closest('.touch-region') 定位分区。
	 * 点击空白区域（无分区元素）不触发任何反应。
	 */
	init() {
		if (!this.touchInteraction) return;

		// PC端：点击事件（事件委托）
		this.touchInteraction.addEventListener('click', (e) => {
			// 如果是从触摸事件转换过来的click，跳过（避免重复触发）
			if (e.detail === 0) return;
			const region = this.identifyRegionFromTarget(e.target);
			if (!region) return; // 点击空白区域不触发
			this.handleTouchInteraction(e.clientX, e.clientY, region);
		});

		// 移动端：触摸事件（使用touchstart+touchend防止滑动误触）
		this.touchInteraction.addEventListener('touchstart', (e) => {
			const touch = e.touches[0];
			this.touchStartTime = Date.now();
			this.touchStartPos = { x: touch.clientX, y: touch.clientY };
		}, { passive: true });

		this.touchInteraction.addEventListener('touchend', (e) => {
			// 判断是否为快速点击（非滑动操作）
			const duration = Date.now() - this.touchStartTime;
			const touch = e.changedTouches[0];
			const dx = touch.clientX - this.touchStartPos.x;
			const dy = touch.clientY - this.touchStartPos.y;
			const distance = Math.sqrt(dx * dx + dy * dy);

			// 只有短时间（<500ms）且短距离（<20px）的触摸才算点击
			if (duration < 500 && distance < 20) {
				const region = this.identifyRegionFromTarget(e.target);
				if (!region) return; // 触摸空白区域不触发
				this.handleTouchInteraction(touch.clientX, touch.clientY, region);
			}
		});
	}

	/**
	 * 从事件目标识别所属分区
	 *
	 * 通过 closest('.touch-region') 向上查找分区元素，读取其 data-region 属性。
	 * 若未命中分区（点击空白区域或父层），返回 null。
	 *
	 * @param {EventTarget|null} target - 事件目标
	 * @returns {string|null} - 分区标识，未识别时返回 null
	 */
	identifyRegionFromTarget(target) {
		if (!(target instanceof Element)) return null;
		const regionEl = target.closest('.touch-region');
		if (!regionEl) return null;
		const region = regionEl.dataset.region;
		// 校验分区标识在配置中存在
		return TOUCH_REGIONS[region] ? region : null;
	}

	/**
	 * 处理触摸交互（统一入口）
	 *
	 * 在触摸位置生成涟漪动画反馈，并构造该分区专属的触摸提示词触发AI交流。
	 *
	 * @param {number} clientX - 点击位置的水平坐标
	 * @param {number} clientY - 点击位置的垂直坐标
	 * @param {string} region - 分区标识
	 */
	handleTouchInteraction(clientX, clientY, region) {
		// 加载中时忽略触摸
		if (this.callbacks.isLoading()) return;

		// 触发涟漪反馈动画
		this.triggerRippleEffect(clientX, clientY);

		// 触发该分区专属的AI交流交互流程
		this.triggerTouchDialogue(region);
	}

	/**
	 * 触发涟漪反馈动画
	 *
	 * 在点击位置生成一个扩散的圆形紫色涟漪动画，提供视觉反馈。
	 * 涟漪使用CSS动画实现，动画结束后自动清理样式。
	 *
	 * @param {number} clientX - 点击位置的水平坐标
	 * @param {number} clientY - 点击位置的垂直坐标
	 */
	triggerRippleEffect(clientX, clientY) {
		if (!this.touchRipple) return;

		// 获取触摸层相对视口的位置
		const rect = this.touchInteraction.getBoundingClientRect();
		const rippleX = clientX - rect.left;
		const rippleY = clientY - rect.top;

		// 设置涟漪位置
		this.touchRipple.style.left = rippleX + 'px';
		this.touchRipple.style.top = rippleY + 'px';

		// 移除上一次动画状态（通过强制重绘重置动画）
		this.touchRipple.classList.remove('active');
		void this.touchRipple.offsetWidth; // 强制重绘
		this.touchRipple.classList.add('active');

		// 动画结束后移除active类
		const onAnimationEnd = () => {
			this.touchRipple?.classList.remove('active');
			this.touchRipple?.removeEventListener('animationend', onAnimationEnd);
		};
		this.touchRipple.addEventListener('animationend', onAnimationEnd);
	}

	/**
	 * 触发分区专属的触摸对话流程
	 *
	 * 力量与态度从全局素材库随机抽取，部位由分区决定。
	 * 构造触摸提示词发送至后端AI模型，触发角色对触摸的回应。
	 * 复用现有的消息发送和WebSocket响应处理流程。
	 *
	 * @param {string} region - 分区标识
	 */
	async triggerTouchDialogue(region) {
		this.callbacks.setLoadingState(true);

		try {
			// 随机生成力量/动作/态度，部位由分区决定，直接发送至后端，不在前端聊天记录中显示
			const { prompt, attitude } = generateTouchPrompt(region);
			// 根据态度词映射情绪状态，触发对应的 Live2D 动作
			const emotionKey = ATTITUDE_TO_EMOTION[attitude] || 'AWAIT';
			Live2D.setEmotionState(EmotionalStateEnum[emotionKey]);

			const openAIMessages = [{ role: 'user', content: prompt }];

			// 发送消息到后端（不添加到前端聊天记录中）
			await sendMessages(openAIMessages);
		} catch (error) {
			console.error('Touch interaction error:', error);
			this.callbacks.showError('触摸交互失败');
			this.callbacks.setLoadingState(false);
			Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
		}
	}
}
