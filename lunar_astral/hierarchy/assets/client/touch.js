import { Live2D, EmotionalStateEnum } from './live2d.js';
import { sendMessages } from './fetch.js';

/**
 * 触摸提示词素材库
 *
 * 四要素组合: <力量> + <动作> + <部位> + <态度>
 * 格式: "<力量><动作>了<部位>, 请做出<态度>的反应"
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
	/** 核心动作 */
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
	/** 身体部位与物品 */
	part: [
		{ value: '头发', weight: 3 },
		{ value: '头顶', weight: 3 },
		{ value: '脸颊', weight: 2 },
		{ value: '发梢', weight: 2 },
		{ value: '胸部', weight: 1 },
		{ value: '腹部', weight: 1 },
		{ value: '大腿', weight: 1 },
		{ value: '小腿', weight: 1 },
		{ value: '脚', weight: 1 },
		{ value: '手', weight: 2 },
		{ value: '大臂', weight: 1 },
		{ value: '小臂', weight: 1 },
		{ value: '裙子', weight: 1 },
		{ value: '外套', weight: 1 },
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
	 * 键为"力量|部位"，当随机生成的结果命中时重新生成
	 */
	excludeCombinations: new Set([
		'用力|脸颊', '使劲|脸颊',  // 敏感部位不能用重手
		'用力|胸部', '使劲|胸部',
		'用力|腹部', '使劲|腹部',
		'捏|裙子', '揉|裙子', '挠|裙子', '戳|裙子', '弹|裙子',  // 衣物不适合捏揉
		'捏|外套', '揉|外套', '挠|外套', '戳|外套', '弹|外套',
		'弹|胸部', '弹|腹部', '弹|大腿',  // 弹不适合大面积部位
	]),
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
 * 生成随机触摸提示词
 *
 * 从素材库中随机组合 <力量> + <动作> + <部位> + <态度>,
 * 通过排除规则过滤不合法组合, 确保生成质量。
 *
 * @returns {string} - 格式化的触摸提示词
 */
export function generateTouchPrompt() {
	const config = TOUCH_PROMPT_CONFIG;
	let force, action, part, attitude;
	let attempts = 0;
	const maxAttempts = 50;

	// 循环生成直到获得合法组合
	do {
		force = weightedRandom(config.force);
		action = weightedRandom(config.action);
		part = weightedRandom(config.part);
		attitude = weightedRandom(config.attitude);
		attempts++;
	} while (
		(config.excludeCombinations.has(`${force}|${part}`) ||
			config.excludeCombinations.has(`${action}|${part}`)) &&
		attempts < maxAttempts
	);

	// 格式: "<力量><动作>了< 部位>, 请做出<态度>的反应"
	return `${force}${action}了你的${part}, 请做出${attitude}的反应`;
}

/**
 * 触摸交互处理器
 *
 * 在Live2D模型区域上覆盖一层透明触摸层，统一检测用户对整个模型区域的点击操作。
 * 支持PC端（click）和移动端（touchstart/touchend）事件。
 * 点击时生成涟漪反馈动画，并触发AI交流交互流程。
 */
export class TouchInteractionHandler {
	touchInteraction = null;
	touchRipple = null;
	touchStartTime = 0;
	touchStartPos = { x: 0, y: 0 };

	/**
	 * @param {HTMLElement} touchInteraction - 触摸交互层元素
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
	 */
	init() {
		if (!this.touchInteraction) return;

		// PC端：点击事件
		this.touchInteraction.addEventListener('click', (e) => {
			// 如果是从触摸事件转换过来的click，跳过（避免重复触发）
			if (e.detail === 0) return;
			this.handleTouchInteraction(e.clientX, e.clientY);
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
				this.handleTouchInteraction(touch.clientX, touch.clientY);
			}
		});
	}

	/**
	 * 处理触摸交互（统一入口）
	 *
	 * 在触摸位置生成涟漪动画反馈，并构造触摸提示词触发AI交流。
	 *
	 * @param {number} clientX - 点击位置的水平坐标
	 * @param {number} clientY - 点击位置的垂直坐标
	 */
	handleTouchInteraction(clientX, clientY) {
		// 加载中时忽略触摸
		if (this.callbacks.isLoading()) return;

		// 触发涟漪反馈动画
		this.triggerRippleEffect(clientX, clientY);

		// 触发AI交流交互流程
		this.triggerTouchDialogue();
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
	 * 触发触摸对话流程
	 *
	 * 构造触摸交互提示词，发送至后端AI模型，触发角色对触摸的回应。
	 * 复用现有的消息发送和WebSocket响应处理流程。
	 */
	async triggerTouchDialogue() {
		this.callbacks.setLoadingState(true);
		Live2D.setEmotionState(EmotionalStateEnum.AWAIT);

		try {
			// 随机生成触摸提示词，直接发送至后端，不在前端聊天记录中显示
			const touchPrompt = generateTouchPrompt();
			const openAIMessages = [{ role: 'user', content: touchPrompt }];

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
