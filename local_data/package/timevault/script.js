// ==================== 工具函数 ====================
/**
 * 生成 [min, max] 范围内的随机整数
 */
function RandomFloor(min, max) {
	return Math.floor(Math.random() * (max - min + 1)) + min;
}

// ==================== 初始化图表 ====================
const hourChart = echarts.init(document.getElementById('hourChart'));
const minuteChart = echarts.init(document.getElementById('minuteChart'));
const secondChart = echarts.init(document.getElementById('secondChart'));

// 获取DOM元素
const timeDisplay = document.getElementById('timeDisplay');
const timeFormatRadios = document.querySelectorAll('input[name="timeFormat"]');
const dayValue = document.getElementById('dayValue');
const hourValue = document.getElementById('hourValue');
const minuteValue = document.getElementById('minuteValue');
const secondValue = document.getElementById('secondValue');

// 番茄钟元素
const pomodoroStatus = document.getElementById('pomodoroStatus');
const pomodoroTimer = document.getElementById('pomodoroTimer');
const focusMinutesInput = document.getElementById('focusMinutes');
const breakMinutesInput = document.getElementById('breakMinutes');
const startBtn = document.getElementById('startPomodoro');
const pauseBtn = document.getElementById('pausePomodoro');
const resetBtn = document.getElementById('resetPomodoro');

// 配置
let timeFormat = '12';       // '12' 或 '24'
let isRunning = false;
let pomodoroSeconds = 1500;  // 默认25分钟
let isFocus = true;          // 专注模式/休息模式
let pomodoroInterval = null;

// ==================== 创建仪表盘配置 ====================
const createOption = (max, name, colorStops) => ({
	backgroundColor: 'transparent',
	series: [
		{
			name: name,
			type: 'gauge',
			radius: '100%',
			startAngle: 0,
			endAngle: 360,
			min: 0,
			max: max,
			progress: {
				show: true,
				width: 12,
				roundCap: true,
				itemStyle: {
					color: {
						type: 'linear',
						x: 0, y: 0, x2: 1, y2: 0,
						colorStops: colorStops
					}
				}
			},
			axisLine: {
				lineStyle: {
					width: 12,
					color: [[1, 'rgba(255, 255, 255, 0.8)']]
				}
			},
			axisTick: { show: false },
			splitLine: { show: false },
			axisLabel: { show: false },
			pointer: { show: false },
			anchor: { show: false },
			detail: {
				valueAnimation: true,
				formatter: '{value}',
				color: '#ff6b9d',
				fontSize: 20,
				offsetCenter: [0, 0],
				fontWeight: 'bold'
			},
			data: [{ value: 0, name: name }]
		}
	]
});

// 小时表
const hourOption = createOption(24, '小时', [
	{ offset: 0, color: '#ffb6c1' },
	{ offset: 1, color: '#ff6b9d' }
]);
// 分钟表
const minuteOption = createOption(60, '分钟', [
	{ offset: 0, color: '#ff8fb1' },
	{ offset: 1, color: '#ff6b9d' }
]);
// 秒表
const secondOption = createOption(60, '秒钟', [
	{ offset: 0, color: '#ff6b9d' },
	{ offset: 1, color: '#ff4785' }
]);

// 初始化渲染
hourChart.setOption(hourOption);
minuteChart.setOption(minuteOption);
secondChart.setOption(secondOption);

// ==================== 时钟更新 ====================
function updateCharts() {
	const now = new Date();
	let hourVal, minuteVal, secondVal;

	if (timeFormat === '12') {
		hourVal = now.getHours() % 12 || 12;
	} else {
		hourVal = now.getHours();
	}

	minuteVal = now.getMinutes();
	secondVal = now.getSeconds();

	// 数字时间显示
	let displayTime;
	if (timeFormat === '12') {
		displayTime = now.toLocaleTimeString('en-US', { hour12: true });
	} else {
		displayTime = now.toLocaleTimeString('en-US', { hour12: false });
	}
	timeDisplay.textContent = displayTime;

	// 天/时/分/秒数值
	const dayOfYear = Math.floor((now - new Date(now.getFullYear(), 0, 0)) / (1000 * 60 * 60 * 24));
	dayValue.textContent = dayOfYear;
	hourValue.textContent = String(now.getHours()).padStart(2, '0');
	minuteValue.textContent = String(now.getMinutes()).padStart(2, '0');
	secondValue.textContent = String(now.getSeconds()).padStart(2, '0');

	// 更新图表值
	hourChart.setOption({ series: [{ data: [{ value: hourVal }] }] });
	minuteChart.setOption({ series: [{ data: [{ value: minuteVal }] }] });
	secondChart.setOption({ series: [{ data: [{ value: secondVal }] }] });
}

// ==================== 番茄钟逻辑 ====================
function updatePomodoroDisplay() {
	const mins = Math.floor(pomodoroSeconds / 60);
	const secs = pomodoroSeconds % 60;
	pomodoroTimer.textContent = `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
}

function updatePomodoroStatus() {
	if (isRunning) {
		pomodoroStatus.textContent = isFocus ? '🍅 专注中' : '☕ 休息中';
		pomodoroStatus.style.color = isFocus ? '#ff6b9d' : '#4caf50';
	} else {
		pomodoroStatus.textContent = isFocus ? '准备专注' : '准备休息';
		pomodoroStatus.style.color = 'var(--primary)';
	}
}

// 使用随机按钮音效播放提示音
function playAlertSound() {
	const audio = new Audio(`/read/audios/button-${RandomFloor(0, 11)}.mp3`);
	audio.volume = 1.0;
	audio.play().catch(err => {
		console.warn('音效播放失败:', err);
	});
}

// 完成一个番茄阶段
function finishPhase() {
	clearInterval(pomodoroInterval);
	isRunning = false;
	playAlertSound();

	// 自动切换模式
	if (isFocus) {
		isFocus = false;
		pomodoroSeconds = parseInt(breakMinutesInput.value) * 60;
	} else {
		isFocus = true;
		pomodoroSeconds = parseInt(focusMinutesInput.value) * 60;
	}
	updatePomodoroDisplay();
	updatePomodoroStatus();
}

// 开始计时
function startPomodoro() {
	if (isRunning) return;
	isRunning = true;

	// 如果计时器已归零，重新读取设置
	if (pomodoroSeconds === 0) {
		pomodoroSeconds = isFocus ? parseInt(focusMinutesInput.value) * 60 : parseInt(breakMinutesInput.value) * 60;
		updatePomodoroDisplay();
	}

	pomodoroInterval = setInterval(() => {
		if (pomodoroSeconds > 0) {
			pomodoroSeconds--;
			updatePomodoroDisplay();
		}
		if (pomodoroSeconds === 0) {
			finishPhase();
		}
	}, 1000);

	updatePomodoroStatus();
}

// 暂停计时
function pausePomodoro() {
	if (!isRunning) return;
	clearInterval(pomodoroInterval);
	isRunning = false;
	pomodoroStatus.textContent = isFocus ? '⏸️ 专注已暂停' : '⏸️ 休息已暂停';
	pomodoroStatus.style.color = 'var(--primary)';
}

// 重置计时
function resetPomodoro() {
	clearInterval(pomodoroInterval);
	isRunning = false;
	isFocus = true;
	pomodoroSeconds = parseInt(focusMinutesInput.value) * 60;
	updatePomodoroDisplay();
	pomodoroStatus.textContent = '准备开始';
	pomodoroStatus.style.color = 'var(--primary)';
}

// ==================== 事件绑定 ====================
startBtn.addEventListener('click', startPomodoro);
pauseBtn.addEventListener('click', pausePomodoro);
resetBtn.addEventListener('click', resetPomodoro);

focusMinutesInput.addEventListener('change', function () {
	if (!isRunning) {
		isFocus = true;
		pomodoroSeconds = parseInt(focusMinutesInput.value) * 60;
		updatePomodoroDisplay();
		pomodoroStatus.textContent = '准备开始';
		pomodoroStatus.style.color = 'var(--primary)';
	}
});

breakMinutesInput.addEventListener('change', function () {
	if (!isRunning) {
		isFocus = false;
		pomodoroSeconds = parseInt(breakMinutesInput.value) * 60;
		updatePomodoroDisplay();
		pomodoroStatus.textContent = '准备开始';
		pomodoroStatus.style.color = 'var(--primary)';
	}
});

timeFormatRadios.forEach(radio => {
	radio.addEventListener('change', function () {
		timeFormat = this.value;
		updateCharts();
	});
});

window.addEventListener('resize', () => {
	hourChart.resize();
	minuteChart.resize();
	secondChart.resize();
});

// ==================== 启动 ====================
updatePomodoroDisplay();
updatePomodoroStatus();
updateCharts();
setInterval(updateCharts, 1000);