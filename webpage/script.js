// 监听 DOM 内容加载完成事件，确保 DOM 元素加载完成后再执行代码
document.addEventListener('DOMContentLoaded', function () {
	// 获取进入按钮元素
	const enterBtn = document.getElementById('enterBtn');
	// 获取进度条元素
	const progressBar = document.getElementById('progressBar');
	// 获取进度文本元素
	const progressText = document.getElementById('progressText');
	// 获取粒子容器元素，用于创建和管理粒子效果
	const particlesContainer = document.getElementById('particlesContainer');
	// 获取内容区域元素
	const content = document.querySelector('.content');
	// 定义技术名称数组，用于随机显示在伪造按钮上
	const techNames = [
		'JavaScript', 'Python', 'Java', 'C++', 'PHP', 'Go', 'Rust', 'TypeScript',
		'HTML', 'CSS', 'JSON', 'XML', 'SQL',
		'DeepSeek', 'Qwen', 'ChatGLM', 'MiniCPM'
	];
	// 为内容区域添加显示进度的类
	content.classList.add('show-progress');
	// 调用函数创建伪造按钮并执行动画
	createFakeButtons();
	// 调用函数创建粒子效果
	createParticles();
	// 为进入按钮添加点击事件监听器
	enterBtn.addEventListener('click', function () {
		// 如果按钮处于禁用状态，则不执行后续操作
		if (this.disabled) return;
		// 点击时缩小按钮，产生点击反馈效果
		this.style.transform = 'scale(0.95)';
		// 10 毫秒后跳转到聊天页面
		setTimeout(() => window.location.href = '/chat-dialogue', 10);
	});

	/**
	 * 创建伪造按钮并设置其初始位置和动画
	 */
	function createFakeButtons() {
		// 存储所有创建的伪造按钮
		const fakeButtons = [];
		// 记录已完成动画的按钮数量
		let completedAnimations = 0;
		// 定义要创建的伪造按钮总数
		const totalFakeButtons = 8;

		// 循环创建指定数量的伪造按钮
		for (let i = 0; i < totalFakeButtons; i++) {
			// 创建一个新的按钮元素
			const fakeBtn = document.createElement('button');
			// 为按钮添加类名
			fakeBtn.className = 'fake-btn';
			// 从技术名称数组中随机选择一个名称作为按钮文本
			fakeBtn.textContent = techNames[Math.floor(Math.random() * techNames.length)];

			// 定义按钮的初始坐标
			let [startX, startY] = [0, 0];
			// 随机选择按钮的初始位置，从四个方向（上、右、下、左）出现
			switch (Math.floor(Math.random() * 4)) {
				case 0: [startX, startY] = [Math.random() * window.innerWidth, -100]; break;
				case 1: [startX, startY] = [window.innerWidth + 100, Math.random() * window.innerHeight]; break;
				case 2: [startX, startY] = [Math.random() * window.innerWidth, window.innerHeight + 100]; break;
				default: [startX, startY] = [-100, Math.random() * window.innerHeight]; break;
			}

			// 设置按钮的初始位置
			[fakeBtn.style.left, fakeBtn.style.top] = [`${startX}px`, `${startY}px`];
			// 将按钮添加到页面中
			document.body.appendChild(fakeBtn);
			// 将按钮添加到数组中
			fakeButtons.push(fakeBtn);

			// 延迟一段时间后执行按钮动画，每个按钮的延迟时间递增
			setTimeout(() => animateFakeButton(fakeBtn, i, totalFakeButtons, () => {
				// 动画完成后，已完成动画的按钮数量加 1
				completedAnimations++;
				// 根据已完成动画的按钮比例调整进入按钮的大小
				enterBtn.style.transform = `scale(${completedAnimations / totalFakeButtons})`;
				// 动态更新进入按钮的文本，显示已完成的按钮数量
				enterBtn.textContent = `少女祈祷中 ${'.'.repeat(completedAnimations)}`;
				// 更新进度文本，显示完成百分比
				progressText.textContent = `${Math.round((completedAnimations / totalFakeButtons) * 100)}%`;
				// 如果所有按钮的动画都已完成
				if (completedAnimations === totalFakeButtons) {
					// 调用函数完成最终动画处理
					finalizeAnimation(fakeButtons);
				}
			}), 1000 + (i * 300));
		}
	}

	/**
	 * 对单个伪造按钮执行动画，使其移动到进入按钮位置
	 * @param {HTMLElement} button - 要执行动画的按钮元素
	 * @param {number} index - 按钮的索引
	 * @param {number} total - 按钮的总数
	 * @param {Function} onComplete - 动画完成后的回调函数
	 */
	function animateFakeButton(button, index, total, onComplete) {
		// 根据按钮索引计算当前进度百分比
		const progress = (index + 1) / total * 100;
		// 更新进度条的宽度
		progressBar.style.width = `${progress}%`;
		// 获取进入按钮的边界矩形信息
		const rect = enterBtn.getBoundingClientRect();
		// 计算进入按钮的水平中心位置
		const mainBtnX = window.innerWidth / 2;
		// 计算进入按钮的垂直中心位置
		const mainBtnY = rect.top + rect.height / 2;
		// 设置按钮动画的过渡效果，持续 3 秒，缓动函数为 ease-in-out
		button.style.transition = 'all 3s ease-in-out';
		// 设置按钮移动到进入按钮的位置
		button.style.left = `${mainBtnX}px`;
		button.style.top = `${mainBtnY}px`;
		// 设置按钮的缩放和偏移效果
		button.style.transform = 'translate(-30%, -30%) scale(1.1)';
		// 设置按钮的透明度为 0.5
		button.style.opacity = '0.5';
		// 300 毫秒后将按钮的透明度设置为 0
		setTimeout(() => button.style.opacity = '0', 300);
		// 监听按钮的过渡结束事件，执行回调函数，且只执行一次
		button.addEventListener('transitionend', onComplete, { once: true });
	}

	/**
	 * 完成所有伪造按钮动画后的最终处理
	 * @param {Array<HTMLElement>} fakeButtons - 所有伪造按钮的数组
	 */
	function finalizeAnimation(fakeButtons) {
		// 遍历所有伪造按钮
		fakeButtons.forEach(btn => {
			// 如果按钮存在父元素，则从 DOM 中移除该按钮
			if (btn.parentNode) btn.parentNode.removeChild(btn);
		});
		// 启用进入按钮
		enterBtn.disabled = false;
		// 移除内容区域的显示进度类
		content.classList.remove('show-progress');
		// 为进入按钮添加 enabled 类
		enterBtn.classList.add('enabled');
		// 修改进入按钮的文本
		enterBtn.textContent = '欢迎光临, 阁下!';
	}

	/**
	 * 创建粒子效果
	 * 此函数会根据窗口大小计算需要创建的粒子数量，然后创建对应数量的粒子元素，
	 * 为每个粒子随机设置尺寸、位置、动画持续时间和延迟，最后将粒子添加到粒子容器中。
	 */
	function createParticles() {
		// 计算粒子数量，取 50 和 (窗口面积 / 4000) 的较小值，避免粒子过多
		const particleCount = Math.min(50, Math.floor(window.innerWidth * window.innerHeight / 4000));
		// 循环创建指定数量的粒子
		for (let i = 0; i < particleCount; i++) {
			// 创建一个 div 元素作为粒子
			const particle = document.createElement('div');
			// 为粒子添加 'particle' 类，用于样式设置
			particle.classList.add('particle');
			// 随机生成粒子的尺寸，范围在 3px 到 15px 之间
			const size = Math.random() * 12 + 3;
			// 设置粒子的宽度
			particle.style.width = `${size}px`;
			// 设置粒子的高度
			particle.style.height = `${size}px`;
			// 随机设置粒子的水平位置，范围在 0% 到 100% 之间
			particle.style.left = `${Math.random() * 100}%`;
			// 随机设置粒子的垂直位置，范围在 0% 到 100% 之间
			particle.style.top = `${100 - (Math.random() * 100) / 2}%`;
			// 随机生成粒子动画的持续时间，范围在 10s 到 30s 之间
			const animationDuration = Math.random() * 20 + 10;
			// 设置粒子动画的持续时间
			particle.style.animationDuration = `${animationDuration}s`;
			// 随机生成粒子动画的延迟时间，范围在 0s 到 -20s 之间，使粒子动画不同步
			particle.style.animationDelay = `-${Math.random() * 20}s`;
			// 随机设置粒子的模糊程度
			particle.style.filter = `blur(${Math.random() * 3}px)`;
			// 将创建好的粒子添加到粒子容器中
			particlesContainer.appendChild(particle);
		}
	}
});