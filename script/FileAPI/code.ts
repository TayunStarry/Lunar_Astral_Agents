import * as EntryAPI from '../EntryAPI/code';

/** 聊天记录元数据版本 */
const chatHistoryMetaVersion = new Set(['2025-07-20', '2025-08-30', '25.1230']);

/**
 * 处理聊天记录导入的事件函数
 *
 * 该函数会创建一个隐藏的文件输入元素，让用户选择 JSON 文件，
 *
 * 当用户选择文件后，调用加载聊天记录的函数处理所选文件
 */
export function importChatInteractionEvent() {
	/**
	 * 创建一个文件输入元素，用于选择要导入的文件
	 */
	const input = document.createElement('input');
	// 设置输入元素的类型为文件选择
	input.type = 'file';
	// 设置允许选择的文件扩展名，仅允许 JSON 文件
	input.accept = '.json';
	// 监听文件输入元素的变化事件
	input.onchange = function (event) {
		/**
		 * 获取用户选择的第一个文件
		 */
		const file = (event.target as HTMLInputElement).files?.[0];
		/**
		 * 创建一个 FileReader 实例，用于读取文件内容
		 */
		const reader = new FileReader();
		// 若用户选择了文件，则调用加载聊天记录的函数处理该文件
		if (!file) return;
		reader.onload = event => {
			/**
			 * 解析文件内容为 JSON 格式
			 */
			const jsonData = JSON.parse((event.target as FileReader).result as string);
			// 验证解析后的 JSON 数据是否有效
			if (jsonData) loadChatHistory(jsonData);
		};
		// 以文本格式读取指定的文件
		reader.readAsText(file);
	};
	// 触发文件选择对话框
	input.click();
};

/**
 * 加载聊天记录到当前会话中
 *
 * 该函数接收一个包含聊天记录的 JSON 数据，验证其格式有效性后，
 *
 * 将聊天记录更新到当前会话，并重新渲染所有聊天消息。
 *
 * 若过程中出现错误，会在控制台输出错误信息，并给用户显示错误提示。
 *
 * @param {Object} jsonData - 包含聊天记录的 JSON 对象，预期包含 `history` 数组
 *
 * @throws {Error} - 当聊天记录格式无效时抛出错误
 */
export function loadChatHistory(jsonData: { history: EntryAPI.HistoryMessage[], promptCache: Array<[string, string]> }) {
	try {
		// 验证聊天记录格式是否有效，若无效则抛出错误
		if (!jsonData.history || !Array.isArray(jsonData.history)) throw new Error("无效的聊天记录格式");
		// 清空当前会话的聊天记录
		EntryAPI.OnlyData.historyMessage.length = 0;
		// 更新当前的聊天记录，将传入的历史记录赋值给全局的会话历史变量
		jsonData.history.forEach(x => EntryAPI.OnlyData.historyMessage.push(x));
		// 重新渲染所有聊天消息，将更新后的聊天记录显示在界面上
		EntryAPI.renderAllMessages(EntryAPI.chatHistoryPanel);
		// 显示系统状态提示，通知用户聊天记录导入成功
		EntryAPI.showSystemMessage("聊天记录导入成功！", "success");
		// 配置面板选项设置为任意值，用于后续操作
		EntryAPI.OnlyData.configurePanelOption = 'any';
		// 隐藏配置面板
		EntryAPI.eraseAllConfigurePanel();
		// 显示聊天记录容器面板
		EntryAPI.chatHistoryContainerPanel.style.display = "flex";
	}
	catch (error) {
		if (!(error instanceof Error)) return;
		// 捕获异常并显示错误信息
		EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
	}
};

/**
 * 处理文件拖拽放下事件的函数。
 * 当文件被拖拽到聊天历史面板并放下时，执行此函数。
 *
 * @param {DragEvent} event - 拖拽放下事件对象
 */
function chatHistoryPanelDragAfterEvent(event: DragEvent) {
	// 阻止事件的默认行为，防止浏览器对文件进行默认处理
	event.preventDefault();
	// 移除 Live2D 容器上的所有附加样式，恢复初始状态
	EntryAPI.live2dContainer.removeAttribute('style');
	// 隐藏文件导入覆盖层
	EntryAPI.displayImportOverlay(EntryAPI.chatHistoryPanel, false);
	/**
	 * 获取通过拖拽传递的文件列表
	 */
	const files = event.dataTransfer?.files || [];
	// 若文件列表为空，显示错误提示信息并终止函数执行
	if (!files.length) {
		EntryAPI.showSystemMessage('请拖入有效的文本文件', 'error');
		return;
	}
	/**
	 * 获取文件列表中的第一个文件
	 */
	const file = files[0];
	// 调用文件处理函数处理选中的文件
	handleFile(file);
};

/**
 * 处理文件拖拽相关事件的函数，包括拖拽经过和拖拽离开事件。
 *
 * @param {DragEvent} event - 拖拽事件对象
 */
function chatHistoryPanelDragEvent(event: DragEvent) {
	// 阻止事件的默认行为，防止浏览器对文件进行默认处理
	event.preventDefault();
	// 若事件类型为拖拽经过
	if (event.type === 'dragover') {
		// 若当前没有文件正在被拖拽
		if (!EntryAPI.OnlyData.isFileDragging) {
			// 标记有文件正在被拖拽
			EntryAPI.OnlyData.isFileDragging = true;
			// 为 Live2D 容器添加边框脉冲动画
			EntryAPI.live2dContainer.style.animation = 'border-pulse 4.0s infinite';
			// 显示文件导入覆盖层
			EntryAPI.displayImportOverlay(EntryAPI.chatHistoryPanel, true);
		}
	}
	// 若事件类型为拖拽离开
	else if (event.type === 'dragleave') {
		/**
		 * 获取与当前事件相关的目标元素
		 */
		const relatedTarget = event.relatedTarget;
		// 若相关目标元素不在聊天历史面板内
		if (!EntryAPI.chatHistoryPanel.contains(relatedTarget as Node)) {
			// 标记没有文件正在被拖拽
			EntryAPI.OnlyData.isFileDragging = false;
			// 移除 Live2D 容器上的所有样式，恢复初始状态
			EntryAPI.live2dContainer.removeAttribute('style');
			// 隐藏文件导入覆盖层
			EntryAPI.displayImportOverlay(EntryAPI.chatHistoryPanel, false);
		}
	}
};

/**
 * 处理输入终端点击事件的函数。
 *
 * 该函数会创建一个隐藏的文件输入元素，监听其变化事件，
 *
 * 当用户选择文件后，调用文件处理函数，并清理临时创建的输入元素。
 */
function inputFileButtonClickEvent() {
	/** 创建一个文件输入元素 */
	const fileInput = document.createElement('input');
	// 设置输入元素的类型为文件选择
	fileInput.type = 'file';
	// 设置允许选择的文件扩展名，包含支持的扩展名和所有文本类型
	fileInput.accept = EntryAPI.OnlyData.fileValidExtensions.join(',');
	// 将输入元素隐藏
	fileInput.style.display = 'none';
	// 监听文件输入元素的变化事件
	fileInput.addEventListener('change',
		event => {
			/** 获取文件列表 */
			const files = (event.target as HTMLInputElement).files;
			// 检查文件列表是否存在且包含至少一个文件
			if (files && files.length > 0) handleFile(files[0]);
			// 清理临时创建的输入元素
			document.body.removeChild(fileInput);
		}
	);
	// 将文件输入元素添加到页面中
	document.body.appendChild(fileInput);
	// 触发文件选择对话框
	fileInput.click();
};

/**
 * 统一文件处理函数（用于点击和拖拽）
 *
 * @param {File} file - 要处理的文件对象
 */
function handleFile(file: File) {
	/** 获取文件的扩展名并转换为小写 */
	const extension = file.name.slice(file.name.lastIndexOf('.')).toLowerCase();
	// 检查是否为图片文件或视频文件
	if (EntryAPI.OnlyData.visionExtensions.includes(extension) || file.type.startsWith('image/') || file.type.startsWith('video/')) {
		// 检查文件大小是否超过 20 MB
		if (file.size > 20 * 1024 * 1024) return EntryAPI.showSystemMessage('图片太大了, 能给月华换一个吗?', 'error');
		// 处理图片文件或视频文件
		processImageFile(file);
	}
	// 检查是否为ZIP文件
	else if (extension === '.zip') processZipFile(file);
	// 优先使用扩展名检测，若扩展名在支持的列表中，则按文本格式读取文件
	else if (EntryAPI.OnlyData.fileValidExtensions.includes(extension)) readFileAsText(file);
	// 备用MIME类型检测（仅作为后备方案），若文件类型为文本类型或在支持的MIME类型列表中，则按文本格式读取文件
	else if (file.type.startsWith('text/') || EntryAPI.OnlyData.fileValidTypes.includes(file.type)) readFileAsText(file);
	// 最后尝试读取小文件（文件大小小于1MB），使用安全尝试方式读取未知类型文件
	else if (file.size < 1024 * 1024) attemptReadAsText(file);
	// 若以上条件都不满足，说明无法读取该文件，显示错误提示信息
	else EntryAPI.showSystemMessage('月华无法阅读这个文件', 'error');
};

/**
 * 安全尝试读取未知类型文件
 *
 * 该函数会尝试以文本格式读取文件，读取完成后检测文件内容是否包含二进制数据，
 *
 * 若包含则显示错误信息，否则将文件内容传递给 函数处理。
 *
 * @param {File} file - 需要读取的文件对象
 */
function attemptReadAsText(file: File) {
	/** 创建一个 FileReader 实例，用于读取文件内容 */
	const reader = new FileReader();
	// 当文件读取成功完成时触发的回调函数
	reader.onload = event => {
		/** 获取文件读取结果 */
		const content = event.target?.result;
		/** 检测文件内容是否包含非可打印字符（二进制数据） */
		const hasBinary = content && /[\x00-\x08\x0E-\x1F]/.test(content as string);
		// 根据检测结果处理文件：若包含二进制数据则显示错误信息，否则处理文件内容
		hasBinary ? EntryAPI.showSystemMessage('文件包含二进制数据', 'error') : tryCaptureConfig(file.name, content as string);
	};
	// 当文件读取过程中发生错误时，显示文件读取失败的错误提示信息
	reader.onerror = error => EntryAPI.showSystemMessage(`文件读取失败: ${error.type}`, "error");
	// 以文本格式读取指定的文件
	reader.readAsText(file);
};

/**
 * 尝试根据文件类型和内容配置进行相应处理
 *
 * @param {string} fileName - 文件名
 *
 * @param {string} fileContent - 文件内容
 */
function tryCaptureConfig(fileName: string, fileContent: string) {
	try {
		// 检查文件是否为 JSON 文件
		if (fileName.toLowerCase().endsWith('.json')) {
			/** 解析 JSON 文件内容 */
			const config = JSON.parse(fileContent);
			/** 获取 JSON 数据的元版本 */
			const version = config?.meta?.version;
			// 检查 JSON 数据的元版本是否在支持的版本列表中
			if (chatHistoryMetaVersion.has(version)) loadChatHistory(config);
			// 若版本不匹配，则按文件分块导入处理
			else fileSliceImport(fileName, fileContent);
		}
		// 若不是 JSON 文件，同样按文件分块导入处理
		else fileSliceImport(fileName, fileContent);
	}
	catch (error) {
		if (!(error instanceof SyntaxError)) return;
		// 捕获异常并显示错误信息
		EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
	}
};

/**
 * 将文件内容分块处理，并创建对应的消息对象。
 *
 * 对于大文件会进行特定处理，如导出聊天记录、清理历史记录、移除注释和格式化内容等。
 *
 * @param {string} fileName - 文件名
 *
 * @param {string} fileContent - 文件内容
 */
async function fileSliceImport(fileName: string, fileContent: string) {
	/** 定义需要保留格式的文件扩展名列表 */
	const preserveFormatTypes = ['.py', '.txt', '.csv'];
	/** 判断文件是否为大文件, 即文件内容长度是否大于等于 用户消息截断长度 */
	const isLargeFile = fileContent.length >= Number(EntryAPI.messageSliceLength.value);
	// 若文件内容长度一定比例, 则显示系统消息, 提醒用户下次提供重点段落
	if (isLargeFile) EntryAPI.showSystemMessage('这个文件有些长... 下次能给月华重点段落吗？', 'error');
	// 若文件是大文件，且文件扩展名不在需要保留格式的列表中
	if (isLargeFile && !preserveFormatTypes.some(ext => fileName.toLowerCase().endsWith(ext))) {
		// 移除文件内容中的代码注释
		fileContent = EntryAPI.removeCodeComments(fileContent);
		// 将文件内容中的多个换行符替换为单个空格
		fileContent = fileContent.replace(/[\r\n]+/g, ' ');
		// 将文件内容中的多个连续空格或制表符替换为单个空格
		fileContent = fileContent.replace(/[ \t]{2,}/g, ' ');
	}
	/** 将文件内容按段落分割为多个字符串 */
	let pages = EntryAPI.splitTextToStrings(fileContent);
	// 为每个文件块添加文件名和段落索引
	pages = pages.map((page, index) => `[ 文件: ${fileName} | 页码: ${index + 1}/${pages.length} ] |>\n ${page}`);
	// 为每个文件块创建对应的消息对象
	pages.forEach(
		async (page: string) => {
			// 每个消息对象创建之间添加 100ms 延迟，避免对服务器压力过大
			await new Promise(resolve => setTimeout(resolve, 100));
			EntryAPI.createMessageObject('assistant', page, true, true, false, null, true);
		}
	);
	// 创建一条询问消息，询问用户希望如何处理该文件，并渲染到聊天历史记录面板中
	EntryAPI.tracelessRenderMessage(`月华拿到📄< \`\`\` ${fileName} \`\`\` >啦～ 您希望了解那些内容呢？`, EntryAPI.chatHistoryPanel);
	// 判断是否为调试模式
	if (EntryAPI.OnlyData.isDebugMode) {
		/** 序列化消息数组 */
		const messagesJson = JSON.stringify(pages, null, 2);
		/** 消息格式的修饰符 */
		const modify = ['<think>\n```json\n', '\n```\n</think>']
		/** 渲染处理后的消息数组 */
		const messageElement = await EntryAPI.tracelessRenderMessage(modify[0] + messagesJson + modify[1], EntryAPI.chatHistoryPanel) as HTMLElement;
		// 为think区块添加折叠功能
		(messageElement.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>).forEach(EntryAPI.bindFoldingButton);
	};
};

/**
 * 处理用户上传的图片文件
 * 1. 先将图片保存到服务器，获得可访问的 URL
 * 2. 获取当前用户输入的多条消息（可能为空）
 * 3. 为每条消息创建对应的消息对象，仅在最后一条消息附加图片 URL
 * 4. 每条消息渲染后等待 1 秒，再调用 API 继续对话
 *
 * @param file - 用户拖拽或选择的图片文件
 */
async function processImageFile(file: File) {
	/** 把图片上传到服务器，返回可供前端访问的 URL */
	const imageUrl = await EntryAPI.saveImageToServer(file);
	/** 获取用户当前输入的所有消息 */
	const userMessage = EntryAPI.getUserMessage();
	/**
	 * 发送单条消息到聊天面板
	 *
	 * @param {string} message - 消息文本内容
	 *
	 * @param {number} index - 消息索引，用于判断是否为最后一条消息
	 */
	async function SendMessage(message: string, index: number) {
		/** 仅在最后一条消息携带图片 URL，其余传 null */
		const attachImageUrl = index >= userMessage.length - 1 ? imageUrl : null;
		/** 创建用户消息对象 */
		const messageObject = await EntryAPI.createMessageObject("user", message, true, false, false, attachImageUrl);
		// 创建并渲染消息对象
		EntryAPI.renderMessage(messageObject, EntryAPI.chatHistoryPanel);
		// 等待 1 秒，确保前端渲染完成后再继续
		await new Promise(resolve => setTimeout(resolve, 500));
	}
	// 若用户未输入任何消息，则发送空文本并附带图片
	if (userMessage.length === 0) SendMessage('', 0);
	// 遍历用户消息数组，依次发送每个消息
	else for (let i = 0; i < userMessage.length; i++) {
		await SendMessage(userMessage[i], i);
	}
	// 调用后端 API 继续对话流程
	EntryAPI.executeDialogueAndParse(EntryAPI.chatHistoryPanel);
}

/**
 * 处理ZIP文件
 *
 * @param {File} file - 要处理的ZIP文件对象
 */
async function processZipFile(file: File) {
	try {
		/** 创建一个FormData对象，用于存储要上传的文件数据 */
		const formData = new FormData();
		// 将ZIP文件添加到FormData对象中
		formData.append('zip_file', file);
		/** 发送PUT请求到 '/archive' 端点，上传ZIP文件 */
		const response = await fetch('/archive', { method: 'PUT', body: formData });
		// 检查响应状态是否正常，若不正常则显示错误消息并终止函数
		if (!response.ok) return EntryAPI.showSystemMessage(`HTTP ${response.status}: ${response.statusText}`, 'error');
		/** 将响应数据解析为JSON格式 */
		const result = await response.json();
		// 检查是否解压出文件，若未解压出文件则显示提示消息并终止函数
		if (!result.extracted_files || result.extracted_files.length === 0) return EntryAPI.showSystemMessage(`月华未能从 ${file.name} 中提取出任何文件 !!`, 'success');
		/**
		 * 处理单个解压出的文件
		 *
		 * @param {Object} fileInfo - 包含文件信息的对象
		 */
		function fileProcess(fileInfo: { name: string; content: number[] }) {
			try {
				/** 尝试将文件内容的字节数组转换为Uint8Array */
				const uint8Array = new Uint8Array(fileInfo.content);
				/** 创建一个UTF-8解码器 */
				const decoder = new TextDecoder('utf-8');
				/** 将Uint8Array解码为文本内容 */
				const fileContent = decoder.decode(uint8Array);
				/** 检查文件内容是否包含二进制数据（非文本文件） */
				const hasBinary = /[\x00-\x08\x0E-\x1F]/.test(fileContent);
				// 若不包含二进制数据，则导入文件内容
				if (!hasBinary) fileSliceImport(fileInfo.name, fileContent);
				// 若包含二进制数据，则跳过该文件并显示提示消息
				else EntryAPI.showSystemMessage(`跳过二进制文件: ${fileInfo.name}`, 'success');
			}
			catch (error) {
				if (!(error instanceof Error)) return;
				// 处理文件时发生错误，显示错误信息
				EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
			}
		}
		// 遍历所有解压出的文件，对每个文件调用fileProcess函数进行处理
		result.extracted_files.forEach(fileProcess);
		// 所有文件处理完成后，在聊天面板中显示ZIP文件处理完成的消息
		EntryAPI.tracelessRenderMessage(`ZIP文件 ${file.name} 处理完成！`, EntryAPI.chatHistoryPanel);
	}
	catch (error) {
		if (!(error instanceof Error)) return;
		// 处理ZIP文件过程中发生错误，显示错误信息
		EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
	}
};

/**
 * 以文本格式读取文件内容
 *
 * @param {File} file - 需要读取的文件对象
 */
function readFileAsText(file: File) {
	/** 创建一个 FileReader 实例，用于读取文件内容 */
	const reader = new FileReader();
	// 当文件读取成功完成时，调用函数处理文件名和文件内容
	reader.onload = event => tryCaptureConfig(file.name, event.target?.result as string);
	// 当文件读取过程中发生错误时，显示文件读取失败的错误提示信息
	reader.onerror = event => {
		/** 从事件目标中获取错误信息 */
		const error = (event as ProgressEvent<FileReader>).target?.error;
		// 若存在错误信息，则显示错误消息
		if (error) EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
		// 若不存在错误信息，则显示未知错误消息
		else EntryAPI.showSystemMessage("文件读取失败: 未知错误", "error");
	};
	// 以文本格式读取指定的文件
	reader.readAsText(file);
};

//* 监听输入框的拖拽离开事件，触发 chatHistoryPanelDragEvent 函数处理事件
EntryAPI.mainContainerPanel.addEventListener('dragleave', event => chatHistoryPanelDragEvent(event));
//* 监听输入框的拖拽经过事件，触发 chatHistoryPanelDragEvent 函数处理事件
EntryAPI.mainContainerPanel.addEventListener('dragover', event => chatHistoryPanelDragEvent(event));
//* 监听输入框的拖拽放下事件，触发 chatHistoryPanelDragAfterEvent 函数处理事件
EntryAPI.mainContainerPanel.addEventListener('drop', event => chatHistoryPanelDragAfterEvent(event));
//* 监听文件输入按钮的点击事件，触发 inputFileButtonClickEvent 函数处理事件
EntryAPI.inputFileButton.addEventListener('click', () => inputFileButtonClickEvent());