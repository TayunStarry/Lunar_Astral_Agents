import * as EntryAPI from '../EntryAPI/code';
/** 提取关键帧响应结构 */
interface ExtractKeyframesResponse {
	/** 关键帧 URL 数组 */
	keyFrames: ExtractKeyframesData[];
	/** 关键帧数量 */
	count: number;
}

/** 关键帧数据结构 */
interface ExtractKeyframesData {
	/** 关键帧文件路径 */
	filePath: string;
	/** 关键帧时间戳，格式：HH:mm:ss */
	timestamp: string;
	/** 关键帧帧号 */
	frameNum: number;
	/** 关键帧数据 */
	data: string;
};

/**
 * 将输入参数转换为 Base64 编码字符串
 *
 * 此函数会先对输入参数进行 URI 编码，然后将编码后的十六进制字符转换为对应的字符，最后进行 Base64 编码
 *
 * @param {string} params - 需要转换的输入参数
 * @returns {string} Base64 编码后的字符串
 */
export function toBtoaString(params: string): string {
	/**
	 * 对输入参数进行 URI 编码，确保特殊字符被正确处理
	 */
	const encodedParams = encodeURIComponent(params);
	/**
	 * 将 URI 编码后的十六进制字符转换为对应的字符
	 */
	const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
	// 对转换后的字符进行 Base64 编码并返回
	return btoa(decodedParams);
};

/**
 * 将 File 或 Blob 对象转换为 Base64 编码字符串
 *
 * 内部使用 FileReader 以 DataURL 方式读取文件内容，
 * 成功时返回完整的 data:[<mediatype>];base64, 前缀 + 编码字符串，
 * 失败时返回 rejected Promise 并携带具体错误信息。
 *
 * @param file - 需要转换的文件或二进制数据
 *
 * @returns {Promise<string>}  Base64 字符串（含 MIME 类型前缀）
 *
 * @throws {Error} 读取或转换失败时抛出
 */
export async function FileToBase64(file: File | Blob): Promise<string> {
	return new Promise(
		(resolve, reject) => {
			/** 创建 FileReader 实例，用于读取文件内容 */
			const reader = new FileReader();
			// 读取完成：将结果直接作为 Base64 字符串返回
			reader.onload = function (event) {
				/** 从事件目标中提取 Base64 编码字符串 */
				const base64String = event.target?.result as string;
				// 检查 Base64 字符串是否为空
				if (!base64String) throw new Error("文件转 Base64 失败: 空字符串");
				// 返回 Base64 字符串
				resolve(base64String);
			};
			// 读取异常：构造明确错误信息并拒绝 Promise
			reader.onerror = function (error) {
				reject(new Error(`文件转 Base64 失败: ${(error.target as FileReader).error?.code}`));
			};
			// 启动读取：以 DataURL 形式读取文件内容
			reader.readAsDataURL(file);
		}
	);
};

/**
 * 从数据库中获取提示词
 *
 * @param {string} key - 索引键
 *
 * @description 从数据库中查询指定索引键对应的提示词
 *
 * @returns {Promise<string | null>} - 提示词或null
 */
async function getPromptFromDatabase(key: string): Promise<string | null> {
	try {
		/** 定义数据库操作对象数组 */
		const operations: EntryAPI.DatabaseOperation[] = [
			{
				type: 'select',
				table: 'KeyPrompt',
				filter: {
					IndexKey: key
				},
				limit: 1
			}
		];
		/** 定义创建表操作 */
		const createTableOperation: EntryAPI.DatabaseOperation = {
			type: 'create',
			table: 'KeyPrompt',
			definition: {
				columns: [
					{ name: "ID", type: "INTEGER", primary_key: true, auto_increment: true },
					{ name: "IndexKey", type: "TEXT" },
					{ name: "Prompt", type: "TEXT" }
				]
			}
		};
		/** 解析数据库查询响应 */
		const result: EntryAPI.BatchResult = await EntryAPI.queryFromDatabase(operations, createTableOperation);
		// 检查查询结果是否有效
		if (result.success && result.results[0].success && result.results[0].rows) {
			return result.results[0].rows[0].Prompt as string;
		}
		// 查询结果为空，返回null
		return null;
	}
	catch (error) {
		return null;
	}
}

/**
 * 向数据库中存储提示词
 *
 * @param {string} key - 索引键
 *
 * @param {string} prompt - 提示词
 *
 * @returns {Promise<boolean>} - 是否成功
 */
async function savePromptToDatabase(key: string, prompt: string): Promise<boolean> {
	try {
		/** 检查是否存在相同索引键的记录 */
		const existingPrompt = await getPromptFromDatabase(key);
		/** 定义数据库操作对象数组 */
		const operations: EntryAPI.DataOperation[] = [];
		// 更新现有记录
		if (existingPrompt) operations.push({ type: 'update', table: 'KeyPrompt', data: { Prompt: prompt }, filter: { IndexKey: key } });
		// 插入新记录
		else operations.push({ type: 'insert', table: 'KeyPrompt', data: { IndexKey: key, Prompt: prompt } });
		/** 定义创建表操作 */
		const createTableOperation: EntryAPI.DatabaseOperation = {
			type: 'create',
			table: 'KeyPrompt',
			definition: {
				columns: [
					{ name: "ID", type: "INTEGER", primary_key: true, auto_increment: true },
					{ name: "IndexKey", type: "TEXT" },
					{ name: "Prompt", type: "TEXT" }
				]
			}
		};
		/** 解析数据库查询响应 */
		const result: EntryAPI.BatchResult = await EntryAPI.queryFromDatabase(operations, createTableOperation);
		// 检查操作是否成功
		return result.success && result.results[0].success;
	}
	catch (error) {
		console.error('向数据库存储提示词失败:', error);
		return false;
	}
}

/**
 * 处理视频文件，提取关键帧
 *
 * @param {string} videoUrl - 视频URL
 *
 * @param {string} text - 相关文本
 *
 * @param {EntryAPI.PostMessageRole} role - 消息角色
 *
 * @param {Array<EntryAPI.PostMessage>} processedMessages - 已处理消息数组
 *
 * @returns {Promise<EntryAPI.PostMessage[]>} - 包含关键帧和文本消息的数组
 */
export async function processVideoFile(videoUrl: string, text: string, role: EntryAPI.PostMessageRole, processedMessages: Array<EntryAPI.PostMessage>): Promise<void> {
	/** 检查是否已处理过该视频 */
	const cachedPrompt = await getPromptFromDatabase(videoUrl);
	// 如果视频已处理过，直接添加到消息数组
	if (cachedPrompt) {
		processedMessages.push({ role, content: cachedPrompt });
		return;
	}
	/** 获取视频文件 */
	const response = await fetch(videoUrl);
	/** 视频文件 Blob 对象 */
	const videoBlob = await response.blob();
	/** FormData 对象，用于上传视频文件 */
	const formData = new FormData();
	// 添加视频文件到 FormData
	formData.append('video', videoBlob, videoUrl.replace(/\\/g, '/').split('/').pop().trim());
	/** 关键帧提取API响应 */
	const extractResponse = await fetch('/extract/keyframes', { method: 'POST', body: formData });
	// 检查响应状态
	if (!extractResponse.ok) throw new Error('提取关键帧失败');
	/** 关键帧提取API响应数据 */
	const result = await extractResponse.json() as ExtractKeyframesResponse;
	/** 提取到的关键帧数组 */
	const keyFrames = result.keyFrames || [];
	/** 沙箱消息数组 */
	const sandboxMessages: Array<EntryAPI.TextMessage> = [];
	/** 模型对视频总结结果 */
	let videoSummary = '';
	/** 关键帧消息数组 */
	const frameMessages: Array<EntryAPI.ImageContent> = keyFrames.map(
		(frame: ExtractKeyframesData) => {
			/** 关键帧 Base64 编码字符串 */
			const imageUrl = `data:image/jpeg;base64,${frame.data}`;
			/** 关键帧消息 */
			return { type: "image_url", image_url: { url: imageUrl } };
		}
	);
	// 处理关键帧，每20张调用一次模型进行画面总结
	for (let i = 0; i < frameMessages.length; i += 20) {
		/** 当前批次20张关键帧消息*/
		const batchFrames = frameMessages.slice(i, i + 20);
		/** 段落消息 */
		const paragraphMessage: EntryAPI.PostMessage = { role, content: [...batchFrames, { type: "text", text: EntryAPI.OnlyData.videoPrompt }] }
		/** 调用模型进行画面总结 */
		const summaryRequest = await (await new EntryAPI.MultimodalRequest([paragraphMessage], false, false, false).response).json();
		/** 模型总结结果 */
		const summary = summaryRequest?.choices?.[0]?.message?.content;
		// 如果启用调试模式, 则渲染处理后的消息数组
		if (EntryAPI.OnlyData.isDebugMode) await EntryAPI.tracelessRenderMessage('<think>\n' + summary + '\n</think>', EntryAPI.chatHistoryPanel);
		// 过滤空字符串和仅包含空格的字符串
		if (summary && summary.trim().length > 0) sandboxMessages.push({ role, content: summary });
	}
	// 判断是否包含多个批处理片段
	if (sandboxMessages.length > 1) {
		// 添加原始文本消息
		sandboxMessages.push({ role, content: EntryAPI.OnlyData.videoSummaryPrompt });
		/** 调用模型进行视频总结 */
		const summaryRequest = await (await new EntryAPI.MultimodalRequest(sandboxMessages, false, false, false).response).json();
		/** 模型视频总结结果 */
		videoSummary = summaryRequest?.choices?.[0]?.message?.content;
	}
	// 如果仅包含一个批处理片段，使用该片段作为总结
	else videoSummary = sandboxMessages[0].content;
	// 如果启用调试模式, 则渲染处理后的消息数组
	if (EntryAPI.OnlyData.isDebugMode) await EntryAPI.tracelessRenderMessage('<think>\n' + videoSummary + '\n</think>', EntryAPI.chatHistoryPanel);
	// 将视频总结结果添加到消息数组
	if (videoSummary) processedMessages.push({ role, content: videoSummary });
	// 如果文本非空，添加到消息数组
	if (text.trim().length > 0) processedMessages.push({ role, content: text });
	// 缓存处理结果到数据库
	if (videoSummary) await savePromptToDatabase(videoUrl, videoSummary);
}

/**
 * 提取视频首帧并设置为对应视频元素的封面图
 *
 * 通过调用 `/extract/firstframe` 接口获取首帧的 Base64 数据，
 *
 * 将其拼接为 DataURL 后赋值给页面中对应 ID 的 `<video>` 元素。
 *
 * @param {string} videoUrl - 视频地址，用于提取视频 ID 作为 DOM 元素 ID
 *
 * @returns {Promise<void>} 无返回值，出错时抛出异常
 *
 * @throws {Error} 提取或设置封面失败时抛出
 */
export async function loadVideoCoverFrame(videoUrl: string): Promise<void> {
	/** 获取视频文件 */
	const response = await fetch(videoUrl);
	/** 视频文件 Blob 对象 */
	const videoBlob = await response.blob();
	/** FormData 对象，用于上传视频文件 */
	const formData = new FormData();
	// 添加视频文件到 FormData
	formData.append('video', videoBlob, videoUrl.replace(/\\/g, '/').split('/').pop().trim());
	/** 关键帧提取API响应 */
	const extractResponse = await fetch('/extract/firstframe', { method: 'POST', body: formData });
	// 检查响应状态
	if (!extractResponse.ok) throw new Error('提取关键帧失败');
	/** 封面关键帧 Base64 编码字符串 */
	const imageUrl = 'data:image/jpeg;base64,' + await extractResponse.json().then(data => data.firstFrame.data);
	/** 提取视频 ID */
	const videoId = videoUrl.replace(/\\/g, '/').split('/').pop().split('.')[0];
	/** 检索视频元素 */
	const queryVideoElements = document.getElementById(videoId) as HTMLVideoElement;
	/** 视频元素父级元素 */
	const parentElement = queryVideoElements.parentElement;
	// 如果视频元素或父级元素不存在, 则直接返回
	if (!queryVideoElements || !parentElement) return;
	// 视频元素封面图
	queryVideoElements.src = imageUrl;
	// 设置视频元素父级元素的标签文本
	parentElement.style.setProperty('--image-label', `"视频文件"`);
};