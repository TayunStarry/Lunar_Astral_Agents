import * as EntryAPI from "../EntryAPI/code";

/** 当前使用的语音引擎类型 */
let currentSpeechEngineType: string = "system";

/** 当前正在播放的语音对象 */
let currentSpeech = null as SpeechSynthesisUtterance | null;

/** 当前正在播放的音频对象 */
let currentAudio = null as HTMLAudioElement | null;

/**
 * 系统 TTS 语音列表
 */
let availableVoices = [] as SpeechSynthesisVoice[];

/**
 * TTS支持状态指示器
 */
export const ttsSupportIndicator = document.getElementById("ttsSupportIndicator") as HTMLElement;

/**
 * 加载系统 TTS 语音列表并填充到选择框中
 */
export function loadSystemSpeechModelVoiceSelect() {
	// 检查浏览器是否支持语音合成 API
	if (!("speechSynthesis" in window)) {
		// 不支持时更新状态提示为警告信息
		if (!ttsSupportIndicator) return;
		ttsSupportIndicator.className = "tts-support-indicator tts-not-supported";
		ttsSupportIndicator.innerHTML = '<i class="fas fa-exclamation-triangle"></i> 浏览器不支持系统TTS功能';
		return;
	}
	// 获取当前可用的语音列表
	availableVoices = speechSynthesis.getVoices().sort((a, b) => a.name.localeCompare(b.name));
	/**
	 * 默认的音色配置名称
	 */
	const defaultVoiceName = 'Microsoft Xiaoxiao Online (Natural) - Chinese (Mainland)';
	// 清空语音选择下拉框内容
	EntryAPI.speechVoiceDropdown.innerHTML = "";
	// 遍历所有可用语音，添加选项到下拉框中
	availableVoices.forEach(
		(voice: SpeechSynthesisVoice) => {
			/**
			 * 创建一个选项元素
			 */
			const option = document.createElement("option");
			// 设置选项的文字为语音名称
			option.value = voice.name;
			// 设置选项的文字为语音名称和语言
			option.textContent = `${voice.name} (${voice.lang})`;
			// 将选项添加到下拉框中
			EntryAPI.speechVoiceDropdown.appendChild(option);
		}
	);
	// 如果之前有选中项，则尝试恢复选择
	if (defaultVoiceName && availableVoices.some((v: { name: string; }) => v.name === defaultVoiceName)) {
		EntryAPI.speechVoiceDropdown.value = defaultVoiceName;
	}
	// 如果是首次加载且未设置默认值，则尝试选择中文语音
	else {
		/**
		 * 查找第一个语言代码以 "zh" 或 "cmn" 开头的中文语音
		 */
		const chineseVoice = availableVoices.find((voice: SpeechSynthesisVoice) => voice.lang.startsWith("zh") || voice.lang.startsWith("cmn"));
		// 如果找到中文语音，则设为默认选中
		if (chineseVoice) EntryAPI.speechVoiceDropdown.value = chineseVoice.name;
	}
};

/**
 * 播放文本转语音 (TTS) 的主函数
 *
 * @param {string} text - 要朗读的文本内容（可选）
 */
export async function playSpeechModel(text?: string) {
	/**
	 * 取要播放的文本内容
	 */
	let textToPlay = text || "";
	// 如果未提供文本，则查找最后一条 AI 发言作为默认内容
	if (!textToPlay) {
		/**
		 * 获取最后一条 AI 发言
		 */
		const lastAssistantMsg = [...EntryAPI.OnlyData.historyMessage].reverse().find((msg) => msg.role === "assistant");
		// 提取结论部分
		if (lastAssistantMsg) textToPlay = lastAssistantMsg.content;
	}
	/**
	 * 提取文本中的结论部分用于朗读
	 */
	let finalText = EntryAPI.extractConclusion(textToPlay);
	// 如果没有找到结论，使用原始文本
	if (!finalText) finalText = textToPlay
	/**
	 * 清理文本，移除括号和尖括号内容
	 */
	const cleanedText = EntryAPI.cleanTextForTTS(finalText);
	// 如果清理后无内容，显示错误并退出
	if (!cleanedText) {
		EntryAPI.showSystemMessage("没有可用的AI消息用于TTS", "error");
		return;
	}
	/**
	 * 截断过长文本以适应 TTS 限制（最多2000字符）
	 */
	const truncatedText = cleanedText.length > 2000 ? cleanedText.substring(0, 2000) + "..." : cleanedText;
	// 尝试播放语音
	try {
		// 停止当前正在播放的语音（防止叠加播放）
		stopSpeechModel();
		// 停止语音识别
		speechRecognitionExample?.stop();
		// 设置情绪状态为正在说话
		EntryAPI.setEmotionState(EntryAPI.EmotionalState.SPEAKING);
		// 禁用按钮，防止重复点击
		EntryAPI.playSpeechModelButton.disabled = true;
		// 显示正在播放的图标
		EntryAPI.playSpeechModelButton.innerHTML = currentSpeechEngineType === "custom"
			? '<i class="fas fa-spinner fa-spin"></i> 生成中...'
			: '<i class="fas fa-volume-up"></i> 播放中...';
		// 根据当前语音引擎类型调用对应的播放方法
		if (currentSpeechEngineType === "custom") await playCustomTTS(truncatedText);
		// 播放系统语音
		else playSystemTTS(truncatedText);
	}
	catch (error) {
		if (!(error instanceof Error)) return;
		// 捕获异常并显示错误信息
		EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
		// 启用播放按钮，允许用户再次尝试播放
		EntryAPI.playSpeechModelButton.disabled = false;
		// 恢复播放按钮的初始显示状态
		EntryAPI.playSpeechModelButton.innerHTML = '<i class="fas fa-play"></i> 播放';
		// 设置情绪状态为错误状态
		EntryAPI.setEmotionState(EntryAPI.EmotionalState.ERROR);
	}
};

/**
 * 使用自定义 TTS 服务播放语音
 *
 * @param {string} text - 要朗读的文本内容
 */
export async function playCustomTTS(text: string) {
};

/**
 * 使用浏览器内置系统 TTS 播放语音
 *
 * @param {string} text - 要朗读的文本内容
 */
export function playSystemTTS(text: string | undefined) {
	// 停止当前正在播放的任何语音（防止冲突）
	speechSynthesis.cancel();
	/**
	 * 创建新的语音合成对象
	 */
	const utterance = new SpeechSynthesisUtterance(text);
	// 设置语速和音量参数
	utterance.rate = parseFloat(EntryAPI.speechSpeedSlider.value);
	utterance.volume = parseFloat(EntryAPI.speechVolumeSlider.value);
	/**
	 * 查找用户选择的语音并应用
	 */
	const selectedVoice = availableVoices.find((voice: { name: string; }) => voice.name === EntryAPI.speechVoiceDropdown.value);
	// 如果找到匹配的语音，则应用
	if (selectedVoice) utterance.voice = selectedVoice;
	// 开始播放语音
	speechSynthesis.speak(utterance);
	// 设置语音播放结束时的回调逻辑
	utterance.onend = () => {
		// 将情绪状态设置为空闲状态
		EntryAPI.setEmotionState(EntryAPI.EmotionalState.IDLE);
		// 更新播放按钮状态为可点击
		EntryAPI.playSpeechModelButton.disabled = false;
		// 恢复播放按钮的初始显示状态
		EntryAPI.playSpeechModelButton.innerHTML = '<i class="fas fa-play"></i> 播放';
		// 若语音识别可用，启动语音识别
		if (AllowSpeechRecognition) speechRecognitionExample?.start();
	};
	// 保存当前语音实例以便后续控制或中断
	currentSpeech = utterance;
};

/**
 * 停止当前正在播放的语音，包括自定义音频和系统TTS语音
 */
export function stopSpeechModel() {
	// 当声音正在播放时，才切换到愤怒状态
	if (currentAudio || (speechSynthesis && speechSynthesis.speaking)) {
		EntryAPI.setEmotionState(EntryAPI.EmotionalState.ANGRY);
	}
	// 检查是否存在自定义音频实例，如果存在则停止播放并重置进度
	if (currentAudio) {
		// 暂停当前播放的音频
		currentAudio.pause();
		// 将音频播放进度重置为开始位置
		currentAudio.currentTime = 0;
		// 恢复播放按钮的初始显示状态
		EntryAPI.playSpeechModelButton.innerHTML = '<i class="fas fa-play"></i> 播放';
	}
	// 检查浏览器是否支持语音合成 API，如果支持则取消当前正在播放的系统语音
	if (speechSynthesis) speechSynthesis.cancel();
	// 启用播放按钮
	EntryAPI.playSpeechModelButton.disabled = false;
	// 恢复播放按钮的初始显示状态
	EntryAPI.playSpeechModelButton.innerHTML = '<i class="fas fa-play"></i> 播放';
};

/** 音引擎类型 */
type SpeechEngineType = "system" | "custom";

/**
 * 切换语音引擎模式
 *
 * @param {SpeechEngineType} mode - 模式名称，可选值为 "system" 或 "custom"
 */
export function switchSpeechEngineMode(mode: SpeechEngineType) {
	// 设置当前语音引擎类型
	currentSpeechEngineType = mode;
	// 判断按钮的类型
	if (mode === "system") {
		// 启用系统语音引擎按钮样式
		EntryAPI.systemSpeechEngineButton?.classList.add("active");
		// 取消自定义语音引擎按钮样式
		EntryAPI.customSpeechEngineButton?.classList.remove("active");
		// 显示系统 TTS 面板，隐藏自定义 TTS 面板
		if (EntryAPI.systemSpeechEnginePanel) EntryAPI.systemSpeechEnginePanel.style.display = "block";
		if (EntryAPI.customSpeechEnginePanel) EntryAPI.customSpeechEnginePanel.style.display = "none";
	}
	else {
		// 启用自定义语音引擎按钮样式
		EntryAPI.customSpeechEngineButton?.classList.add("active");
		// 取消系统语音引擎按钮样式
		EntryAPI.systemSpeechEngineButton?.classList.remove("active");
		// 显示自定义 TTS 面板，隐藏系统 TTS 面板
		if (EntryAPI.systemSpeechEnginePanel) EntryAPI.systemSpeechEnginePanel.style.display = "none";
		if (EntryAPI.customSpeechEnginePanel) EntryAPI.customSpeechEnginePanel.style.display = "block";
	}
};

/**
 * 加载系统语音模型
 * 检查浏览器是否支持系统 TTS 功能，若支持则加载语音模型选择项，若不支持则给出提示并切换语音引擎
 */
export function loadSystemSpeechModel() {
	// 检查浏览器是否支持系统语音合成功能
	if ("speechSynthesis" in window) {
		// 当可用语音列表改变时，加载语音模型选择项并移除事件监听器
		speechSynthesis.onvoiceschanged = () => {
			loadSystemSpeechModelVoiceSelect();
			speechSynthesis.onvoiceschanged = null;
		};
		// 立即加载语音模型选择项
		loadSystemSpeechModelVoiceSelect();
	}
	else {
		// 设置 TTS 支持指示器的样式为不支持状态
		ttsSupportIndicator.className = "tts-support-indicator tts-not-supported";
		// 在 TTS 支持指示器中显示浏览器不支持系统 TTS 功能的提示
		ttsSupportIndicator.innerHTML = '<i class="fas fa-exclamation-triangle"></i> 浏览器不支持系统 TTS 功能';
	}
	// 切换语音引擎模式为系统语音引擎
	switchSpeechEngineMode("system");
};

/**
 * 实现语音识别接口
 */
interface SpeechRecognitionInterface {
	start(): void;
	stop(): void;
	lang: string;
	interimResults: boolean;
	continuous: boolean;
	onresult: (event: SpeechRecognitionEvent) => void;
	onerror: (event: { error: string; }) => void;
	onend: () => void;
}
/**
 * 语音识别事件接口
 */
interface SpeechRecognitionEvent {
	resultIndex: number;
	results: string | any[];
}
/**
 * 重启识别计时器
 */
let restartTimer: NodeJS.Timeout | null = null;
/**
 * 识别状态标志
 */
export let AllowSpeechRecognition = false;
/**
 * 处于语音识别状态下的提示语
 */
const VOICE_RECOGNITION_TIP = "月华正在聆听哦...";
/**
 * 尝试获取浏览器支持的语音识别 API，优先使用标准 API，若不支持则使用 WebKit 内核的 API
 */
const speechRecognitionModule = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
/**
 * 语音识别实例
 */
export let speechRecognitionExample: SpeechRecognitionInterface | null = null;
/**
 * 清除重启计时器
 */
function clearRestartTimer() {
	if (restartTimer) {
		clearTimeout(restartTimer);
		restartTimer = null;
	}
};
/**
 * 重启语音识别
 * 若识别未被手动停止，且识别器未处于活动状态，则尝试重启识别
 */
function restartRecognition() {
	// 只有在识别未被手动停止的情况下才重启
	if (!AllowSpeechRecognition) return;
	try {
		speechRecognitionExample.start();
	}
	catch (error) {
		console.error("重启语音识别失败:", error);
		// 重启失败时再重置按钮状态
		AllowSpeechRecognition = false;
		EntryAPI.voiceReleaseButton.title = "语音输入";
		EntryAPI.voiceReleaseButton.classList.remove("activate");
		const icon = EntryAPI.voiceReleaseButton.querySelector('i');
		if (icon) icon.className = "fas fa-microphone";
	}
}
/**
 * 处理语音识别结果事件
 * 若识别结果为最终结果，将其追加到 transcript 中；若为中间结果，直接更新 transcript
 * 最后根据输入框类型将 transcript 填充到对应的输入框中
 */
function SpeechRecognitionAppearResult(event: SpeechRecognitionEvent) {
	/**
	 * 用于存储识别结果的字符串
	 */
	let transcript = '';
	// 遍历所有识别结果
	for (let i = event.resultIndex; i < event.results.length; i++) {
		// 若为最终结果，将其追加到 transcript 中
		if (event.results[i].isFinal) transcript += event.results[i][0].transcript;
		// 若为中间结果，直接更新 transcript
		else transcript = event.results[i][0].transcript;
	}
	// 若 live2d 输入框存在，将识别结果填充到该输入框中
	if (EntryAPI.live2dWriteArea) EntryAPI.live2dWriteArea.value = transcript;
	// 若聊天输入框存在，将识别结果填充到该输入框中
	if (EntryAPI.chatWriteArea) EntryAPI.chatWriteArea.value = transcript;
}
/**
 * 处理语音识别结束事件
 * 若识别未被手动停止，且识别器未处于活动状态，则设置识别状态为 false，更新按钮状态和图标
 */
function SpeechRecognitionTerminateExecution() {
	// 清除可能存在的重启计时器
	clearRestartTimer();
	// 若 live2d 输入面板隐藏，则清空聊天输入框
	if (getComputedStyle(EntryAPI.live2dInputPanel).display !== 'none') {
		EntryAPI.chatWriteArea.value = "";
	}
	// 若 live2d 输入面板显示，则清空 live2d 输入框
	else EntryAPI.live2dWriteArea.value = "";
	// 检查输入框的值是否为语音识别状态下的提示语，若任一输入框为提示语
	if (EntryAPI.chatWriteArea.value === VOICE_RECOGNITION_TIP || EntryAPI.live2dWriteArea.value === VOICE_RECOGNITION_TIP) {
		// 清空 live2d 输入框
		EntryAPI.live2dWriteArea.value = "";
		// 清空聊天输入框
		EntryAPI.chatWriteArea.value = "";
		// 提前返回，结束当前函数逻辑
		return;
	}
	// 若语音识别未被禁用，则延迟后调用发送聊天消息到后端模型的函数
	if (!EntryAPI.OnlyData.isDisableVoiceRecognition && EntryAPI.chatWriteArea.value !== "") {
		setTimeout(() => EntryAPI.sendChatMessageToBackendModel(), 100);
	}
	// 停止语音识别 避免重复识别
	speechRecognitionExample.stop();
}
/**
 * 处理语音识别错误事件
 * 若错误类型为用户手动停止，不显示消息；否则显示错误信息并重启识别
 */
function SpeechRecognitionErrorOccurred(event: { error: string; }) {
	// 如果是用户手动停止的错误，不显示消息
	if (event.error === 'aborted' || event.error === 'not-allowed') {
		return;
	}
	// 显示错误信息
	//EntryAPI.showSystemMessage('月华... 听不到你说的话', 'success');
	// 清除可能存在的重启计时器
	clearRestartTimer();
	// 延迟3000ms后重启下一轮识别
	restartTimer = setTimeout(restartRecognition, 500);
}
/**
 * 创建语音识别实例
 * 若浏览器不支持语音识别 API，则显示错误消息并返回
 */
function createSpeechRecognition() {
	// 创建语音识别实例
	speechRecognitionExample = new speechRecognitionModule() as SpeechRecognitionInterface;
	// 设置语音识别语言为中文（中国大陆）
	speechRecognitionExample.lang = 'zh-CN';
	// 启用实时识别结果，即识别过程中也能获取中间结果
	speechRecognitionExample.interimResults = true;
	// 设置连续识别，即不结束识别会话
	speechRecognitionExample.continuous = false;
	// 当语音识别有结果时触发此事件
	speechRecognitionExample.onresult = SpeechRecognitionAppearResult;
	// 当语音识别出错时触发此事件
	speechRecognitionExample.onerror = SpeechRecognitionErrorOccurred;
	// 当语音识别会话结束时触发此事件
	speechRecognitionExample.onend = SpeechRecognitionTerminateExecution;
}
/**
 * 执行语音识别
 * 若语音识别实例不存在，则创建一个新实例；若已存在，则直接启动识别
 */
function executeSpeechRecognition() {
	// 若语音识别实例不存在，则创建一个新实例
	if (!speechRecognitionExample) createSpeechRecognition();
	// 启动语音识别
	speechRecognitionExample.start();
	// 停止语音模型
	stopSpeechModel();
	// 更新识别状态为正在识别
	AllowSpeechRecognition = true;
	// 修改按钮的提示文本为 "停止识别"
	EntryAPI.voiceReleaseButton.title = "停止识别";
	// 启用语音输入按钮的激活状态
	EntryAPI.voiceReleaseButton.classList.add("activate");
	/** 获取按钮中的图标元素 */
	const icon = EntryAPI.voiceReleaseButton.querySelector('i');
	// 若图标元素存在，则将其类名修改为表示暂停的图标类名
	if (icon) icon.className = "fas fa-pause-circle";
	// 更新输入框状态，提示用户正在聆听
	EntryAPI.live2dWriteArea.value = VOICE_RECOGNITION_TIP;
	EntryAPI.chatWriteArea.value = VOICE_RECOGNITION_TIP;
}
// 若当前正在进行语音识别，则停止识别并重置相关状态；否则检查浏览器是否支持语音识别并执行识别
EntryAPI.voiceReleaseButton.addEventListener('click',
	() => {
		// 清除可能存在的重启计时器
		clearRestartTimer();
		// 若当前正在进行语音识别，则停止识别并重置相关状态
		if (AllowSpeechRecognition) {
			// 停止语音识别
			speechRecognitionExample.stop();
			// 清空语音识别实例
			speechRecognitionExample = null;
			// 更新识别状态为未识别
			AllowSpeechRecognition = false;
			// 恢复按钮的提示文本为 "语音输入"
			EntryAPI.voiceReleaseButton.title = "语音输入";
			// 取消语音输入按钮的激活状态
			EntryAPI.voiceReleaseButton.classList.remove("activate");
			/** 获取按钮中的图标元素 */
			const icon = EntryAPI.voiceReleaseButton.querySelector('i');
			// 若图标元素存在，则将其类名恢复为表示麦克风的图标类名
			if (icon) icon.className = "fas fa-microphone";
			// 终止当前函数逻辑
			return;
		}
		// 检查浏览器是否支持语音识别
		if (!speechRecognitionModule) {
			// 若不支持，显示错误提示信息
			EntryAPI.showSystemMessage("您的浏览器不支持语音识别！", 'error');
			// 若语音输入按钮存在，则禁用该按钮
			if (EntryAPI.voiceReleaseButton) EntryAPI.voiceReleaseButton.disabled = true;
			// 终止当前函数逻辑
			return;
		}
		try {
			// 创建语音识别实例
			createSpeechRecognition();
			// 执行语音识别
			executeSpeechRecognition();
		}
		catch (error) {
			console.error("启动语音识别失败:", error);
			// 清空语音识别实例
			speechRecognitionExample = null;
			// 更新识别状态为未识别
			AllowSpeechRecognition = false;
		}
	}
)