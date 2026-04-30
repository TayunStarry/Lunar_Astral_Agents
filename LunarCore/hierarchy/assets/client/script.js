/**
 * WebSocket客户端类
 *
 * 提供WebSocket连接管理、自动重连、消息处理等功能
 */
class WebSocketClient {
	url;
	ws = null;
	messageHandlers = [];
	connectionHandlers = [];
	errorHandlers = [];
	reconnectAttempts = 0;
	maxReconnectAttempts = 5;
	reconnectDelay = 3000;
	reconnectTimer = null;
	/**
	 * 创建WebSocket客户端实例
	 *
	 * @param {string} url - WebSocket服务器地址
	 */
	constructor(url) {
		this.url = url;
	}
	/**
	 * 建立WebSocket连接
	 */
	connect() {
		if (this.ws?.readyState === WebSocket.OPEN) {
			return;
		}
		try {
			this.ws = new WebSocket(this.url);
			this.setupEventListeners();
		}
		catch (error) {
			console.error('WebSocket connection error:', error);
			this.scheduleReconnect();
		}
	}
	/**
	 * 设置WebSocket事件监听器
	 */
	setupEventListeners() {
		if (!this.ws)
			return;
		this.ws.onopen = () => {
			console.log('WebSocket connected');
			this.reconnectAttempts = 0;
			this.connectionHandlers.forEach(handler => handler());
		};
		this.ws.onmessage = (event) => {
			try {
				const message = JSON.parse(event.data);
				this.messageHandlers.forEach(handler => handler(message));
			}
			catch (error) {
				console.error('Failed to parse WebSocket message:', error);
			}
		};
		this.ws.onerror = (error) => {
			console.error('WebSocket error:', error);
			this.errorHandlers.forEach(handler => handler(error));
		};
		this.ws.onclose = () => {
			console.log('WebSocket closed');
			this.scheduleReconnect();
		};
	}
	/**
	 * 安排自动重连
	 */
	scheduleReconnect() {
		if (this.reconnectAttempts >= this.maxReconnectAttempts) {
			console.error('Max reconnection attempts reached');
			return;
		}
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
		}
		this.reconnectTimer = window.setTimeout(() => {
			this.reconnectAttempts++;
			console.log(`Reconnecting... attempt ${this.reconnectAttempts}`);
			this.connect();
		}, this.reconnectDelay);
	}
	/**
	 * 断开WebSocket连接
	 */
	disconnect() {
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
		if (this.ws) {
			this.ws.close();
			this.ws = null;
		}
	}
	/**
	 * 发送消息到服务器
	 *
	 * @param {string | object} data - 消息内容
	 */
	send(data) {
		if (this.ws?.readyState === WebSocket.OPEN) {
			const message = typeof data === 'string' ? data : JSON.stringify(data);
			this.ws.send(message);
		}
		else {
			console.warn('WebSocket is not connected');
		}
	}
	/**
	 * 注册消息处理器
	 *
	 * @param {(message: WebSocketMessage) => void} handler - 消息回调函数
	 */
	onMessage(handler) {
		this.messageHandlers.push(handler);
	}
	/**
	 * 注册连接成功处理器
	 *
	 * @param {() => void} handler - 连接成功回调函数
	 */
	onConnect(handler) {
		this.connectionHandlers.push(handler);
	}
	/**
	 * 注册错误处理器
	 *
	 * @param {(error: Event) => void} handler - 错误回调函数
	 */
	onError(handler) {
		this.errorHandlers.push(handler);
	}
	/**
	 * 检查是否已连接
	 *
	 * @returns {boolean} - 是否已连接
	 */
	isConnected() {
		return this.ws?.readyState === WebSocket.OPEN;
	}
}

const BORDER_COLORS = [
	'var(--status-218838)',
	'var(--status-3a5a8a)',
	'var(--status-4a6fa5)',
	'var(--status-6c9bcf)',
	'var(--status-8a2be2)',
	'var(--status-9d6bff)',
	'var(--status-dc3545)',
	'var(--status-fbbf24)',
	'var(--status-ffc107)',
	'var(--status-20c997)',
	'var(--status-ff6b9c)',
];
/**
 * 获取随机边框颜色
 *
 * @returns {string} - CSS颜色值
 */
function randomBorderColor() {
	return BORDER_COLORS[Math.floor(Math.random() * BORDER_COLORS.length)];
}
/**
 * HTML字符转义
 *
 * @param {string} text - 原始文本
 *
 * @returns {string} - 转义后的HTML安全字符串
 */
function escapeHtml(text) {
	const div = document.createElement('div');
	div.textContent = text;
	return div.innerHTML;
}
/**
 * 处理思考标签，将<think>转为可折叠的HTML
 *
 * @param {string} content - 原始内容
 *
 * @returns {string} - 处理后的HTML字符串
 */
function processThinkTags(content) {
	return content
		.replace(/<think>/gi, '<details class="think-block"><summary class="toggle_think_button">思考过程</summary>')
		.replace(/<\/think>/gi, '</details>');
}
/**
 * 渲染Markdown内容
 *
 * @param {string} content - Markdown文本
 *
 * @returns {Promise<string>} - 渲染后的HTML字符串
 */
async function renderMarkdown(content) {
	if (window.marked) {
		let html = await window.marked.parse(content);
		html = processThinkTags(html);
		return html;
	}
	return escapeHtml(content);
}
/**
 * 代码高亮处理
 *
 * @param {HTMLElement} container - 包含代码块的容器
 */
function highlightCode(container) {
	container.querySelectorAll('pre code').forEach((block) => {
		if (window.hljs) {
			window.hljs.highlightElement(block);
		}
	});
}
/**
 * 渲染Mermaid图表
 *
 * @param {HTMLElement} container - 包含Mermaid代码块的容器
 *
 * @returns {Promise<void>}
 */
async function renderMermaid(container) {
	const mermaidBlocks = container.querySelectorAll('code.language-mermaid');
	for (const block of Array.from(mermaidBlocks)) {
		const textContent = block.textContent || '';
		if (textContent.length <= 20)
			continue;
		try {
			const graphDefinition = textContent;
			await window.mermaid.parse(graphDefinition);
			const id = `mermaid-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
			const { svg } = await window.mermaid.render(id, graphDefinition);
			const mermaidContainer = document.createElement('div');
			mermaidContainer.className = 'mermaid-container';
			const parser = new DOMParser();
			const doc = parser.parseFromString(svg, 'image/svg+xml');
			const svgElement = doc.documentElement;
			const chartType = svgElement.getAttribute('aria-roledescription');
			if (chartType === 'flowchart' || chartType === 'classDiagram') {
				const viewBox = svgElement.getAttribute('viewBox');
				if (viewBox) {
					const values = viewBox.split(/\s+/).map(parseFloat);
					if (values.length === 4 && values.every(v => !isNaN(v))) {
						if (chartType === 'flowchart') {
							values[0] *= 0.45;
							values[1] *= 0.45;
							values[2] *= 1.05;
							values[3] *= 1.05;
						}
						else {
							values[0] *= 0;
							values[1] *= 0.35;
							values[2] *= 1.05;
							values[3] *= 1.25;
						}
						svgElement.setAttribute('viewBox', values.join(' '));
					}
				}
			}
			const modifiedSVG = new XMLSerializer().serializeToString(svgElement);
			mermaidContainer.innerHTML = `<div style="width: 100%; border: 10px dashed #eee; padding: 0px">${modifiedSVG}</div>`;
			const parent = block.parentElement;
			if (parent) {
				parent.insertBefore(mermaidContainer, block);
				parent.removeChild(block);
			}
		}
		catch (error) {
			console.error('Mermaid rendering error:', error);
			const errorContainer = document.createElement('div');
			errorContainer.className = 'mermaid-error';
			errorContainer.textContent = error instanceof Error ? error.message : 'Mermaid rendering failed';
			const parent = block.parentElement;
			if (parent) {
				parent.insertBefore(errorContainer, block);
			}
		}
	}
}
/**
 * 渲染ECharts图表
 *
 * @param {HTMLElement} container - 包含ECharts占位符的容器
 */
function renderECharts(container) {
	container.querySelectorAll('.echarts-placeholder').forEach(async (placeholder) => {
		try {
			const chartData = placeholder.getAttribute('data-chart');
			if (!chartData)
				return;
			const config = JSON.parse(chartData);
			const chartContainer = document.createElement('div');
			chartContainer.style.width = '100%';
			chartContainer.style.height = '400px';
			placeholder.appendChild(chartContainer);
			const chart = window.echarts.init(chartContainer);
			chart.setOption(config);
		}
		catch (error) {
			console.error('ECharts rendering error:', error);
		}
	});
}
/**
 * 渲染数学公式（KaTeX）
 *
 * @param {HTMLElement} container - 包含数学公式的容器
 *
 * @returns {Promise<void>}
 */
async function renderMath(container) {
	if (window.katex && window.renderMathInElement) {
		try {
			window.renderMathInElement(container, {
				delimiters: [
					{ left: '$$', right: '$$', display: true },
					{ left: '$', right: '$', display: false },
					{ left: '\\[', right: '\\]', display: true },
					{ left: '\\(', right: '\\)', display: false },
				],
				throwOnError: false,
			});
		}
		catch (error) {
			console.error('KaTeX rendering error:', error);
		}
	}
}
/**
 * 编码文件路径（用于HTTP传输）
 * 后端要求将目录和文件名一起编码后放入 X-File-Name 头
 *
 * @param {string} filepath - 路径，如 "document/readme.txt"
 * @returns {string} - Base64编码后的字符串
 */
function encodeFilePath(filepath) {
	const encodedParams = encodeURIComponent(filepath);
	const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
	return btoa(decodedParams);
}
/**
 * 清空容器内容
 *
 * @param {HTMLElement} container - 目标容器
 */
function clearContainer(container) {
	container.innerHTML = '';
}
/**
 * 创建文件预览对象
 *
 * @param {File} file - 文件对象
 *
 * @returns {FilePreview} - 文件预览对象
 */
function createFilePreview(file) {
	return { file, url: URL.createObjectURL(file), type: getFileType(file), name: file.name };
}
/**
 * 获取文件类型
 *
 * @param {File} file - 文件对象
 *
 * @returns {'image' | 'video' | 'text'} - 文件类型
 */
function getFileType(file) {
	if (file.type.startsWith('image/'))
		return 'image';
	if (file.type.startsWith('video/'))
		return 'video';
	return 'text';
}
/**
 * 判断文件是否为媒体文件（图片或视频）
 *
 * @param {File} file - 文件对象
 *
 * @returns {boolean} - 是否为媒体文件
 */
function isMediaFile(file) {
	return file.type.startsWith('image/') || file.type.startsWith('video/');
}

/**
 * 判断文件是否为纯文本类文件（通过 MIME 或扩展名）
 * @param {File} file - 文件对象
 * @returns {boolean}
 */
function isTextFile(file) {
	/** 常见文本 MIME 类型 */
	const textMimeTypes = [
		'text/plain', 'text/html', 'text/css', 'text/javascript', 'text/markdown',
		'text/xml', 'text/csv', 'text/calendar', 'text/yaml', 'text/x-yaml',
		'application/json', 'application/javascript', 'application/xml', 'application/yaml',
		'application/typescript', 'application/x-httpd-php', 'application/rtf'
	];
	// 检查文件 MIME 类型是否在文本 MIME 类型列表中
	if (textMimeTypes.includes(file.type)) return true;
	/** 配置文件扩展名列表 */
	const configExtensions = ['ini', 'cfg', 'conf', 'properties', 'gitignore', 'dockerignore', 'editorconfig', 'code-workspace'];
	/** 数据文件扩展名列表 */
	const dataExtensions = ['json', 'xml', 'yaml', 'yml', 'toml', 'csv'];
	/** 文档文件扩展名列表 */
	const docExtensions = ['txt', 'md', 'log'];
	/** Web前端文件扩展名列表 */
	const webExtensions = ['html', 'htm', 'css', 'scss', 'js', 'ts', 'jsx', 'tsx'];
	/** 后端/脚本语言文件扩展名列表 */
	const scriptExtensions = ['php', 'py', 'rb', 'pl', 'sh', 'bash', 'ps1', 'bat', 'cmd', 'r'];
	/** 编译型语言文件扩展名列表 */
	const compiledExtensions = ['java', 'c', 'cpp', 'h', 'hpp', 'go', 'rs', 'swift', 'kt', 'dart', 'lua', 'sql'];
	/** 所有文本文件扩展名列表 */
	const textExtensions = [...configExtensions, ...dataExtensions, ...docExtensions, ...webExtensions, ...scriptExtensions, ...compiledExtensions];
	/** 文件扩展名 */
	const fileExtension = file.name.slice(file.name.lastIndexOf('.') + 1).toLowerCase();
	// 检查文件扩展名是否在文本文件扩展名列表中
	return textExtensions.includes(fileExtension);
}

/**
 * 根据文件类型决定存储目录前缀（后端要求将前缀拼接到文件名前）
 * @param {File} file - 文件对象
 * @returns {'document/' | 'images/'}
 */
function getStoragePrefix(file) {
	return isMediaFile(file) ? 'images/' : 'document/';
}

/**
 * 撤销文件预览对象的URL
 *
 * @param {FilePreview} preview - 文件预览对象
 */
function revokeFilePreview(preview) {
	if (preview.url.startsWith('blob:')) {
		URL.revokeObjectURL(preview.url);
	}
}
/**
 * 撤销所有文件预览对象的URL
 *
 * @param {FilePreview[]} previews - 文件预览对象数组
 */
function revokeAllFilePreviews(previews) {
	previews.forEach(revokeFilePreview);
}
/**
 * 获取视频缩略图
 *
 * @param {File} file - 视频文件对象
 *
 * @returns {Promise<string>} - 视频缩略图的Base64编码字符串
 */
async function getVideoThumbnail(file) {
	function execute(resolve, reject) {
		const video = document.createElement('video');
		video.preload = 'metadata';
		video.muted = true;
		video.onloadeddata = () => { video.currentTime = 1; };
		video.onseeked = () => {
			const canvas = document.createElement('canvas');
			canvas.width = video.videoWidth;
			canvas.height = video.videoHeight;
			const ctx = canvas.getContext('2d');
			if (ctx) {
				ctx.drawImage(video, 0, 0);
				resolve(canvas.toDataURL('image/jpeg'));
			}
			else
				reject(new Error('Failed to get video context'));
		};
		video.onerror = () => { reject(new Error('Failed to load video')); };
		video.src = URL.createObjectURL(file);
	}
	return new Promise(execute);
}
/**
 * 计算文件的SHA-256哈希值（16位）
 * @param {File} file - 文件对象
 * @returns {Promise<string>} - 文件的SHA-256哈希值（16位）
 */
async function calculateFileHash(file) {
	/** 定义处理文件读取的异步函数 */
	function process(resolve) {
		/** 创建FileReader实例，用于读取文件内容 */
		const reader = new FileReader();
		// 为FileReader的onload事件添加回调函数，文件读取成功时触发
		reader.onload = async function (e) {
			try {
				/** 从FileReader事件对象中获取文件的ArrayBuffer数据 */
				const arrayBuffer = e.target?.result;
				/** 使用crypto.subtle.digest方法计算ArrayBuffer的SHA-256哈希值 */
				const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
				/** 将哈希结果的ArrayBuffer转换为Uint8Array数组 */
				const hashArray = Array.from(new Uint8Array(hashBuffer));
				/** 将Uint8Array数组中的每个字节转换为两位的十六进制字符串，并拼接成完整的哈希字符串 */
				const fullHash = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
				/** 截取完整哈希字符串的前16个字符 */
				const shortHash = fullHash.substring(0, 16);
				// 将截取后的短哈希值作为Promise的成功结果返回
				resolve(shortHash);
			}
			catch {
				// 返回文件名的 Base64 编码
				resolve(encodeFilePath(file.name).slice(-16));
			}
		};
		// 为FileReader的onerror事件添加回调函数，文件读取失败时触发
		reader.onerror = async (error) => {
			if (!(error instanceof Error)) return;
			// 显示文件读取失败的系统消息
			window.app.showError(error.message);
		};
		// 以ArrayBuffer格式读取文件内容
		reader.readAsArrayBuffer(file);
	}
	// 返回一个Promise，用于处理异步操作
	return new Promise(process);
}
/**
 * 保存文件到服务器（使用文件名前缀区分目录）
 * @param {File} file - 文件对象
 * @param {boolean} [overwrite=false] - 是否覆盖已存在文件
 * @returns {Promise<SaveFileResponse>} - 保存文件的响应对象，其中 filename 为相对路径（含前缀）
 */
async function saveFile(file, overwrite = false) {
	const prefix = getStoragePrefix(file);
	/** 计算文件的SHA-256哈希值（取前16个字符，保持较短长度） */
	const fileHash = await calculateFileHash(file);
	/** 获取文件扩展名 */
	const fileExtension = file.name.slice(file.name.lastIndexOf('.')).toLowerCase();
	/** 使用哈希值 + 扩展名作为新文件名 */
	const newFileName = `${fileHash}${fileExtension}`;
	const encodedFileName = encodeFilePath(prefix + newFileName);
	const response = await fetch('/save', {
		method: 'POST',
		headers: {
			'X-File-Name': encodedFileName,
			'X-Overwrite': overwrite.toString(),
		},
		body: file,
	});
	if (!response.ok) {
		const errorData = await response.json().catch(() => ({ message: 'Upload failed' }));
		throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
	}
	return response.json();
}
/**
 * 发送消息到服务器
 *
 * @param {Array<{ role: string; content: unknown }>} messages - 消息数组
 *
 * @returns {Promise<{ success: boolean; length: number }>} - 服务器响应对象
 */
async function sendMessages(messages) {
	const response = await fetch('/write/message', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
		},
		body: JSON.stringify({ messages }),
	});
	if (!response.ok) {
		const errorData = await response.json().catch(() => ({ message: 'Request failed' }));
		throw new Error(errorData.message || `HTTP error! status: ${response.status}`);
	}
	return response.json();
}
/**
 * 从服务器获取Live2D设置
 *
 * @returns {Promise<{ name?: string; url?: string; scale?: number; x?: number; y?: number; autoInteract?: boolean }>} - Live2D设置对象
 */
async function fetchLive2DSetting() {
	try {
		const response = await fetch('/read/resources/live2d/setting.json');
		if (!response.ok) {
			throw new Error(`HTTP error! status: ${response.status}`);
		}
		const rawText = await response.text();
		const jsonText = rawText
			.replace(/\/\/.*$/gm, '')
			.replace(/\/\*[\s\S]*?\*\//g, '')
			.replace(/'/g, '"');
		const setting = JSON.parse(jsonText);
		return setting;
	}
	catch (error) {
		console.error('Failed to fetch Live2D setting:', error);
		return {};
	}
}

let pixiJSExample = null;
let Live2DModelInstance = null;
let currentLive2DModel = null;
let currentEmotionState = 'IDLE';
const EmotionalStateEnum = {
	IDLE: 'IDLE',
	THINKING: 'THINKING',
	AWAIT: 'AWAIT',
	SPEAKING: 'SPEAKING',
	HAPPY: 'HAPPY',
	SAD: 'SAD',
	ANGRY: 'ANGRY',
};
/**
 * Live2D核心模块
 *
 * 提供Live2D模型加载、渲染、情绪状态管理等功能
 */
const Live2D = {
	/**
	 * 初始化Live2D模块
	 *
	 * @returns {Promise<void>}
	 */
	async init() {
		const errorDiv = document.querySelector('.live2d-error-message');
		if (errorDiv)
			errorDiv.remove();
		try {
			await this.waitForPIXI();
			await this.loadLive2DPlugin();
			this.initApplication();
			currentLive2DModel = (await fetchLive2DSetting());
			await this.loadModel();
			window.addEventListener('resize', () => this.reloadContainer());
			this.setEmotionState(EmotionalStateEnum.IDLE);
			this.reloadContainer();
		}
		catch (error) {
			if (error instanceof Error) {
				this.showError(`初始化失败: ${error.message}`);
			}
			throw error;
		}
	},
	/**
	 * 等待PIXI库加载完成
	 *
	 * @returns {Promise<void>}
	 */
	waitForPIXI() {
		return new Promise((resolve) => {
			const check = () => {
				if (window.PIXI) {
					resolve();
				}
				else {
					setTimeout(check, 100);
				}
			};
			check();
		});
	},
	/**
	 * 等待Live2D插件加载完成
	 *
	 * @returns {Promise<void>}
	 */
	loadLive2DPlugin() {
		return new Promise((resolve) => {
			const check = () => {
				if (window.PIXI?.live2d) {
					resolve();
				}
				else {
					setTimeout(check, 100);
				}
			};
			check();
		});
	},
	/**
	 * 初始化PIXI应用
	 */
	initApplication() {
		if (pixiJSExample) {
			pixiJSExample.destroy(true);
		}
		const container = document.getElementById('live2dContainer');
		const wasHidden = container?.parentElement?.style.display === 'none';
		if (wasHidden && container?.parentElement) {
			container.parentElement.style.display = 'block';
			container.parentElement.style.visibility = 'hidden';
		}
		const canvas = document.getElementById('live2dCanvas');
		const parameters = {
			transparent: true,
			width: container?.clientWidth || 0,
			height: container?.clientHeight || 0,
			view: canvas,
			antialias: true,
		};
		pixiJSExample = new window.PIXI.Application(parameters);
		const modelInfo = document.getElementById('modelIntel');
		if (modelInfo) {
			modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
		}
		if (wasHidden && container?.parentElement) {
			container.parentElement.style.display = 'none';
			container.parentElement.style.visibility = 'visible';
		}
	},
	/**
	 * 加载Live2D模型
	 *
	 * @returns {Promise<void>}
	 */
	async loadModel() {
		const modelInfo = document.getElementById('modelIntel');
		try {
			if (Live2DModelInstance) {
				pixiJSExample?.stage.removeChild(Live2DModelInstance);
				Live2DModelInstance.destroy();
				Live2DModelInstance = null;
			}
			if (modelInfo) {
				modelInfo.textContent = `加载模型: ${currentLive2DModel?.name || '未知'}...`;
			}
			if (!currentLive2DModel) {
				throw new Error('No Live2D model configured');
			}
			const model = await window.PIXI.live2d.Live2DModel.from(currentLive2DModel.url, { autoInteract: currentLive2DModel.autoInteract });
			Live2DModelInstance = model;
			model.scale.set(currentLive2DModel.scale);
			model.anchor.set(0.5, 0.5);
			const container = document.getElementById('live2dContainer');
			model.x = (container?.clientWidth || 0) * currentLive2DModel.x;
			model.y = (container?.clientHeight || 0) * currentLive2DModel.y;
			pixiJSExample?.stage.addChild(Live2DModelInstance);
			if (modelInfo) {
				modelInfo.textContent = currentLive2DModel?.name || '未知';
			}
		}
		catch (error) {
			if (error instanceof Error) {
				this.showError(`Live2D 加载失败: ${error.message}`);
				if (modelInfo) {
					modelInfo.textContent = 'Live2D 加载失败';
				}
			}
			throw error;
		}
	},
	/**
	 * 显示错误信息
	 *
	 * @param {string} message - 错误信息
	 */
	showError(message) {
		const container = document.getElementById('live2dContainer');
		if (!container)
			return;
		const errorDiv = document.createElement('div');
		errorDiv.className = 'live2d-error-message';
		errorDiv.innerHTML = `
      <h2><i class="fas fa-exclamation-triangle"></i> 出错了</h2>
      <p>${message}</p>
      <p>请检查控制台获取详细信息</p>
      <button id="reload-btn" style="margin-top: 20px; padding: 10px 20px; cursor: pointer;">重新加载</button>
    `;
		container.appendChild(errorDiv);
		document.getElementById('reload-btn')?.addEventListener('click', () => {
			errorDiv.remove();
			this.init();
		});
	},
	/**
	 * 重新加载容器尺寸
	 */
	reloadContainer() {
		const container = document.getElementById('live2dContainer');
		if (!container)
			return;
		if (pixiJSExample) {
			pixiJSExample.renderer.resize(container.clientWidth, container.clientHeight);
		}
		if (Live2DModelInstance && currentLive2DModel) {
			const scale = container.clientHeight < 500
				? currentLive2DModel.scale * 0.65
				: currentLive2DModel.scale;
			Live2DModelInstance.scale.x = scale;
			Live2DModelInstance.scale.y = scale;
			Live2DModelInstance.x = container.clientWidth * currentLive2DModel.x;
			Live2DModelInstance.y = container.clientHeight * currentLive2DModel.y;
		}
	},
	/**
	 * 设置情绪状态
	 *
	 * @param {EmotionalState} state - 情绪状态
	 */
	setEmotionState(state) {
		currentEmotionState = state;
		if (Live2DModelInstance && Live2DModelInstance.motion) {
			const motionMap = {
				IDLE: 'idle',
				THINKING: 'thinking',
				AWAIT: 'waiting',
				SPEAKING: 'speaking',
				HAPPY: 'happy',
				SAD: 'sad',
				ANGRY: 'angry',
			};
			const motion = motionMap[state];
			if (motion) {
				try {
					Live2DModelInstance.motion(motion);
				}
				catch (e) {
					console.warn(`Motion ${motion} not available:`, e);
				}
			}
		}
	},
	/**
	 * 获取当前情绪状态
	 *
	 * @returns {EmotionalState} - 当前情绪状态
	 */
	getCurrentEmotionState() {
		return currentEmotionState;
	},
	/**
	 * 设置情绪状态并自动恢复
	 *
	 * @param {EmotionalState} state - 情绪状态
	 * @param {number} [duration=9000] - 持续时间（毫秒）
	 */
	setStateWithTimeout(state, duration = 9000) {
		this.setEmotionState(state);
		if (state !== EmotionalStateEnum.IDLE && state !== EmotionalStateEnum.THINKING) {
			setTimeout(() => {
				if (currentEmotionState === state) {
					this.setEmotionState(EmotionalStateEnum.IDLE);
				}
			}, duration);
		}
	},
	/**
	 * 获取Live2D模型实例
	 *
	 * @returns {Live2DModel | null} - 模型实例
	 */
	getModel() {
		return Live2DModelInstance;
	},
	/**
	 * 检查Live2D是否准备就绪
	 *
	 * @returns {boolean} - 是否准备就绪
	 */
	isReady() {
		return Live2DModelInstance !== null;
	},
};

/**
 * 创建消息DOM元素
 *
 * @param {Message} message - 消息对象
 *
 * @returns {HTMLElement} - 消息元素
 */
function createMessageElement(message) {
	const messageElement = document.createElement('div');
	messageElement.classList.add('message');
	if (message.role === 'user') {
		messageElement.classList.add('user-message');
	}
	else {
		messageElement.classList.add('assistant-message');
	}
	messageElement.style.borderColor = randomBorderColor();
	const header = document.createElement('div');
	header.className = 'message-header';
	header.textContent = message.role === 'user' ? '你' : '月华';
	const contentDiv = document.createElement('div');
	contentDiv.className = 'message-content';
	messageElement.appendChild(header);
	messageElement.appendChild(contentDiv);
	return messageElement;
}
/**
 * 渲染单条消息到容器
 *
 * @param {Message} message - 消息对象
 * @param {HTMLElement} container - 目标容器元素
 *
 * @returns {Promise<HTMLElement>} - 渲染后的消息元素
 */
async function renderMessage(message, container) {
	const messageElement = createMessageElement(message);
	const contentDiv = messageElement.querySelector('.message-content');
	if (message.imageUrls && message.imageUrls.length > 0) {
		const imagesContainer = document.createElement('div');
		imagesContainer.className = 'images-container';
		for (const imageUrl of message.imageUrls) {
			const imgContainer = document.createElement('div');
			imgContainer.className = 'labeled-image-container';
			const img = document.createElement('img');
			img.src = imageUrl;
			img.className = 'image-just-drawn';
			img.alt = typeof message.content === 'string' ? message.content : '图片';
			img.onerror = () => {
				img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
			};
			img.onclick = () => previewImage(imageUrl, typeof message.content === 'string' ? message.content : '图片');
			imgContainer.appendChild(img);
			imagesContainer.appendChild(imgContainer);
		}
		contentDiv.appendChild(imagesContainer);
	}
	else if (message.imageUrl) {
		const imgContainer = document.createElement('div');
		imgContainer.className = 'labeled-image-container';
		const img = document.createElement('img');
		img.src = message.imageUrl;
		img.className = 'image-just-drawn';
		img.alt = typeof message.content === 'string' ? message.content : '图片';
		img.onerror = () => {
			img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
		};
		img.onclick = () => previewImage(message.imageUrl, typeof message.content === 'string' ? message.content : '图片');
		imgContainer.appendChild(img);
		contentDiv.appendChild(imgContainer);
	}
	if (typeof message.content === 'string' && message.content) {
		const markdownContent = document.createElement('div');
		markdownContent.className = 'markdown-content';
		markdownContent.innerHTML = await renderMarkdown(message.content);
		contentDiv.appendChild(markdownContent);
		highlightCode(markdownContent);
		await renderMermaid(markdownContent);
		renderECharts(markdownContent);
		await renderMath(markdownContent);
	}
	container.appendChild(messageElement);
	container.scrollTo({ top: container.scrollHeight, behavior: 'smooth' });
	return messageElement;
}
/**
 * 渲染所有消息到容器
 *
 * @param {HTMLElement} container - 目标容器元素
 * @param {Message[]} messages - 消息数组
 * @param {boolean} [clearFirst=true] - 是否先清空容器
 *
 * @returns {Promise<void>}
 */
async function renderAllMessages(container, messages, clearFirst = true) {
	if (clearFirst) {
		clearContainer(container);
	}
	for (const message of messages) {
		await renderMessage(message, container);
	}
}

/**
 * 核心应用类
 *
 * 管理整个聊天应用的生命周期、UI交互、WebSocket通信等核心功能
 */
class LunarCoreApp {
	/** WebSocket客户端 */
	wsClient = null;
	/** 历史消息 */
	historyMessages = [];
	/** 文件预览 */
	filePreviews = [];
	/** 是否打开弹窗 */
	isModalOpen = false;
	/** 是否加载中 */
	isLoading = false;
	/** 消息输入框 */
	messageInput = null;
	/** 发送按钮 */
	sendButton = null;
	/** 文件预览区域 */
	filePreviewArea = null;
	/** 弹窗遮罩层 */
	modalOverlay = null;
	/** 弹窗主体 */
	modalBody = null;
	/** 错误提示 */
	errorToast = null;
	/** 构造函数 */
	constructor() { this.initElements(); this.initEventListeners(); this.initWebSocket(); this.initLive2D(); }
	/** 初始化元素 */
	initElements() {
		// 初始化 消息输入框 元素
		this.messageInput = document.getElementById('messageInput');
		// 初始化 发送按钮 元素
		this.sendButton = document.getElementById('sendButton');
		// 初始化 文件预览区域 元素
		this.filePreviewArea = document.getElementById('filePreviewArea');
		// 初始化 弹窗遮罩层 元素
		this.modalOverlay = document.getElementById('modalOverlay');
		// 初始化 弹窗主体 元素
		this.modalBody = document.getElementById('modalBody');
		// 初始化 错误提示 元素
		this.errorToast = document.getElementById('errorToast');
		// 初始化 聊天历史按钮 元素
		document.getElementById('chatHistoryButton')?.addEventListener('click', () => this.toggleModal());
		// 初始化 弹窗关闭按钮 元素
		document.getElementById('modalClose')?.addEventListener('click', () => this.toggleModal());
		// 初始化 弹窗遮罩层 点击事件 元素
		this.modalOverlay?.addEventListener('click', (e) => {
			if (e.target === this.modalOverlay)
				this.toggleModal();
		});
	}
	/** 初始化事件监听器 */
	initEventListeners() {
		// 初始化 发送按钮 点击事件 元素
		this.sendButton?.addEventListener('click', () => this.handleSend());
		// 初始化 消息输入框 键盘事件 元素
		this.messageInput?.addEventListener('keydown', (e) => {
			if (e.key === 'Enter' && !e.shiftKey) {
				e.preventDefault();
				this.handleSend();
			}
		});
		// 初始化 消息输入框 输入事件 元素
		this.messageInput?.addEventListener('input', () => { this.autoResizeTextarea(); });
		// 初始化 消息输入框 拖拽事件 元素
		this.setupDragAndDrop();
	}
	/** 初始化拖拽事件 */
	setupDragAndDrop() {
		const inputContainer = document.getElementById('mainContainerPanel');
		['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
			inputContainer?.addEventListener(eventName, (e) => { e.preventDefault(); e.stopPropagation(); });
		});
		['dragenter', 'dragover'].forEach(eventName => {
			inputContainer?.addEventListener(eventName, () => {
				inputContainer.classList.add('drag-over');
			});
		});
		['dragleave', 'drop'].forEach(eventName => {
			inputContainer?.addEventListener(eventName, () => {
				inputContainer.classList.remove('drag-over');
			});
		});
		inputContainer?.addEventListener('drop', async (e) => {
			const files = e.dataTransfer?.files;
			if (files) {
				await this.handleFileSelect(Array.from(files));
			}
		});
	}
	/**
	 * 处理文件选择
	 *
	 * @param {File[]} files - 文件数组
	 *
	 * @returns {Promise<void>}
	 */
	async handleFileSelect(files) {
		for (const file of files) {
			const preview = createFilePreview(file);
			this.filePreviews.push(preview);
			this.renderFilePreview(preview);
		}
	}
	/**
	 * 渲染文件预览项
	 *
	 * @param {FilePreview} preview - 文件预览对象
	 */
	renderFilePreview(preview) {
		const item = document.createElement('div');
		item.className = 'file-preview-item';
		item.dataset.name = preview.name;
		if (preview.type === 'image') {
			const img = document.createElement('img');
			img.src = preview.url;
			item.appendChild(img);
		}
		else if (preview.type === 'video') {
			const video = document.createElement('video');
			video.src = preview.url;
			video.muted = true;
			item.appendChild(video);
			getVideoThumbnail(preview.file).then(thumbnail => {
				const img = document.createElement('img');
				img.src = thumbnail;
				item.insertBefore(img, video);
				video.style.display = 'none';
			}).catch(e => {
				console.warn('Failed to get video thumbnail:', e);
			});
		}
		else {
			const icon = document.createElement('i');
			icon.className = 'fas fa-file-alt';
			icon.style.cssText = 'font-size: 24px; color: white; display: flex; align-items: center; justify-content: center; height: 100%;';
			item.appendChild(icon);
		}
		const label = document.createElement('div');
		label.className = 'file-label';
		label.textContent = preview.name;
		item.appendChild(label);
		const removeBtn = document.createElement('button');
		removeBtn.className = 'remove-btn';
		removeBtn.innerHTML = '<i class="fas fa-times"></i>';
		removeBtn.onclick = () => this.removeFilePreview(preview, item);
		item.appendChild(removeBtn);
		this.filePreviewArea?.appendChild(item);
	}
	/**
	 * 移除文件预览项
	 *
	 * @param {FilePreview} preview - 文件预览对象
	 * @param {HTMLElement} item - 预览项DOM元素
	 */
	removeFilePreview(preview, item) {
		const index = this.filePreviews.indexOf(preview);
		if (index > -1) {
			this.filePreviews.splice(index, 1);
		}
		if (preview.url.startsWith('blob:')) {
			URL.revokeObjectURL(preview.url);
		}
		item.remove();
	}
	/**
	 * 自动调整文本框高度
	 */
	autoResizeTextarea() {
		if (this.messageInput) {
			this.messageInput.style.height = 'auto';
			this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
			Live2D.reloadContainer();
		}
	}
	/**
	 * 初始化WebSocket连接
	 */
	initWebSocket() {
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//localhost:36797/ws`;
		this.wsClient = new WebSocketClient(wsUrl);
		this.wsClient.onConnect(() => {
			console.log('WebSocket connected');
			this.setLoadingState(false);
		});
		this.wsClient.onMessage((message) => {
			this.handleWebSocketMessage(message);
		});
		this.wsClient.onError((error) => {
			console.error('WebSocket error:', error);
			this.showError('连接错误，请刷新页面');
		});
		this.wsClient.connect();
	}
	/**
	 * 处理WebSocket消息
	 *
	 * @param {WebSocketMessage} message - WebSocket消息
	 *
	 * @returns {Promise<void>}
	 */
	async handleWebSocketMessage(message) {
		switch (message.type) {
			case 'context':
				if (message.data.type === 'response' || message.data.type === 'active') {
					await this.handleAssistantMessage(message.data.content || '');
				}
				break;
			case 'image':
				if (message.data.images) {
					for (const imageBase64 of message.data.images) {
						const byteString = atob(imageBase64);
						const mimeType = 'image/jpeg';
						const ab = new ArrayBuffer(byteString.length);
						const ia = new Uint8Array(ab);
						for (let i = 0; i < byteString.length; i++) {
							ia[i] = byteString.charCodeAt(i);
						}
						const blob = new Blob([ab], { type: mimeType });
						const file = new File([blob], `assistant_${Date.now()}.jpg`, { type: 'image/jpeg' });
						const fileUrl = `${window.location.origin}/read/${file.name}`;
						await this.handleAssistantMessage('', fileUrl);
					}
				}
				break;
			case 'error':
				this.showError(message.data.content || '发生错误');
				this.setLoadingState(false);
				Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
				break;
		}
	}
	/**
	 * 处理助手消息
	 *
	 * @param {string} content - 消息内容
	 * @param {string} [imageUrl] - 图片URL（可选）
	 *
	 * @returns {Promise<void>}
	 */
	async handleAssistantMessage(content, imageUrl) {
		const assistantMessage = {
			role: 'assistant',
			content: content,
			imageUrl: imageUrl || undefined,
			timestamp: Date.now(),
		};
		this.historyMessages.push(assistantMessage);
		// 仅当模态框打开时实时渲染消息
		if (this.isModalOpen) {
			await renderMessage(assistantMessage, this.modalBody);
		}
		this.setLoadingState(false);
		Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
	}
	/**
	 * 异步保存并提取文件信息
	 * @param {FilePreview} preview
	 * @returns {Promise<{fileUrl: string, textContent?: string, isText: boolean, fileName: string}>}
	 */
	async processFileUpload(preview) {
		const saveResult = await saveFile(preview.file, true);
		const fileUrl = `${window.location.origin}/read/${saveResult.filename}`;
		let textContent = null;
		let isText = false;
		if (isTextFile(preview.file)) {
			isText = true;
			try {
				const rawText = await preview.file.text();
				// 限制文本长度，避免超出上下文窗口（最大约50000字符）
				const MAX_TEXT_LEN = 50000;
				if (rawText.length > MAX_TEXT_LEN) {
					textContent = rawText.slice(0, MAX_TEXT_LEN) + '\n\n[文件内容过长，已截断]';
					this.showError(`文件 ${preview.file.name} 内容超过限制，仅截取前 ${MAX_TEXT_LEN} 字符`);
				} else {
					textContent = rawText;
				}
			} catch (err) {
				console.error(`读取文本文件失败: ${preview.file.name}`, err);
				this.showError(`无法读取文件 ${preview.file.name}，未加入上下文`);
			}
		}
		return { fileUrl, textContent, isText, fileName: preview.name };
	}
	/**
	 * 处理发送消息
	 *
	 * @returns {Promise<void>}
	 */
	async handleSend() {
		const text = this.messageInput?.value.trim();
		const hasFiles = this.filePreviews.length > 0;
		if (!text && !hasFiles) {
			return;
		}
		this.setLoadingState(true);
		Live2D.setEmotionState(EmotionalStateEnum.AWAIT);
		try {
			const openAIMessages = [];
			const contentBlocks = [];
			const uploadedFileUrls = [];   // 存储所有文件的URL用于显示消息卡片
			// 1. 先处理所有文件：上传、提取文本
			const filePromises = this.filePreviews.map(preview => this.processFileUpload(preview));
			const fileResults = await Promise.all(filePromises);
			// 2. 构建 content blocks
			for (const res of fileResults) {
				if (res.isText && res.textContent) {
					// 文本文件：将完整内容作为文本块，同时附上文件链接
					const textBlock = `【文件 ${res.fileName}】\n内容：\n\`\`\`\n${res.textContent}\n\`\`\`\n访问链接：${res.fileUrl}`;
					contentBlocks.push({
						type: 'text',
						text: textBlock
					});
					uploadedFileUrls.push(res.fileUrl);
				} else if (!res.isText) {
					// 媒体文件：图片或视频（当作图片url块处理，后端应支持多模态）
					contentBlocks.push({
						type: 'image_url',
						image_url: { url: res.fileUrl }
					});
					uploadedFileUrls.push(res.fileUrl);
				} else {
					// 文本文件但读取失败，只提供链接
					contentBlocks.push({
						type: 'text',
						text: `【文件 ${res.fileName}】无法读取内容，访问链接：${res.fileUrl}`
					});
					uploadedFileUrls.push(res.fileUrl);
				}
			}
			// 3. 添加用户输入的文本
			if (text) {
				contentBlocks.push({
					type: 'text',
					text: text
				});
			}
			// 最终用户消息内容（数组格式）
			const userContent = contentBlocks.length > 0 ? contentBlocks : text;
			openAIMessages.push({
				role: 'user',
				content: userContent,
			});
			// 4. 保存用户消息到本地历史
			const userMessage = {
				role: 'user',
				content: text || (hasFiles ? '[文件]' : ''),
				timestamp: Date.now(),
			};
			if (uploadedFileUrls.length > 0) {
				userMessage.imageUrls = uploadedFileUrls;
			}
			this.historyMessages.push(userMessage);
			if (this.isModalOpen) {
				await renderMessage(userMessage, this.modalBody);
			}
			// 5. 发送给后端
			await sendMessages(openAIMessages);
			// 6. 清空输入区和文件预览
			this.messageInput.value = '';
			this.clearFilePreviews();
			this.autoResizeTextarea();
		} catch (error) {
			console.error('Send error:', error);
			this.showError(error instanceof Error ? error.message : '发送失败');
			this.setLoadingState(false);
			Live2D.setStateWithTimeout(EmotionalStateEnum.IDLE);
		}
	}
	/**
	 * 清空所有文件预览
	 */
	clearFilePreviews() {
		revokeAllFilePreviews(this.filePreviews);
		this.filePreviews = [];
		if (this.filePreviewArea) {
			this.filePreviewArea.innerHTML = '';
		}
	}
	/**
	 * 设置加载状态
	 *
	 * @param {boolean} loading - 是否加载中
	 */
	setLoadingState(loading) {
		this.isLoading = loading;
		if (this.sendButton) {
			this.sendButton.disabled = loading;
			if (loading) {
				this.sendButton.innerHTML = '<span class="loading-indicator"></span>';
			}
			else {
				this.sendButton.innerHTML = '<i class="fas fa-paper-plane"></i>';
			}
		}
	}
	/**
	 * 切换弹窗显示状态
	 */
	toggleModal() {
		this.isModalOpen = !this.isModalOpen;
		if (this.isModalOpen) {
			this.modalOverlay?.classList.add('visible');
			if (this.modalBody) {
				this.modalBody.innerHTML = '';
			}
			renderAllMessages(this.modalBody, this.historyMessages, false);
		} else {
			this.modalOverlay?.classList.remove('visible');
		}
	}
	/**
	 * 显示错误提示
	 *
	 * @param {string} message - 错误信息
	 */
	showError(message) {
		if (this.errorToast) {
			this.errorToast.textContent = message;
			this.errorToast.classList.add('visible');
			setTimeout(() => {
				this.errorToast?.classList.remove('visible');
			}, 3000);
		}
	}
	/**
	 * 初始化Live2D
	 *
	 * @returns {Promise<void>}
	 */
	async initLive2D() {
		try {
			await Live2D.init();
		}
		catch (error) {
			console.error('Failed to initialize Live2D:', error);
			this.showError('Live2D 初始化失败');
		}
	}
}
document.addEventListener('DOMContentLoaded', () => { window.app = new LunarCoreApp(); });