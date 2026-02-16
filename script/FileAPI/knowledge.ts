import * as EntryAPI from '../EntryAPI/code';

/**
 * 文件大小限制（10MB）
 */
const MAX_FILE_SIZE = 10 * 1024 * 1024;

/**
 * 已保存的历史记录 UUID 集合
 */
const savedHistoryTsg = new Set<string>();
/**
 * 处理文件拖拽相关事件，包括拖拽经过和拖拽离开事件。
 *
 * @param event 拖拽事件对象
 */
function lunarNotesPanelDragEvent(event: DragEvent): void {
	// 阻止默认事件
	event.preventDefault();
	// 根据类型处理拖拽事件
	switch (event.type) {
		case 'dragover':
			if (!EntryAPI.OnlyData.isFileDragging) {
				EntryAPI.OnlyData.isFileDragging = true;
				EntryAPI.lunarNotesPanel.style.animation = 'border-pulse 4.0s infinite';
				EntryAPI.displayImportOverlay(EntryAPI.lunarNotesPanel);
			};
			break;

		case 'dragleave':
			const relatedTarget = event.relatedTarget as Node | null;
			if (!EntryAPI.lunarNotesPanel.contains(relatedTarget)) {
				resetDragState();
			};
			break;
	}
}

/**
 * 重置拖拽状态
 */
function resetDragState(): void {
	EntryAPI.OnlyData.isFileDragging = false;
	EntryAPI.lunarNotesPanel.removeAttribute('style');
	EntryAPI.displayImportOverlay(EntryAPI.lunarNotesPanel, false);
}

/**
 * 处理文件拖放（drop）事件：读取拖入的文本文件，将其内容拆分为片段并导入知识库。
 * 1. 阻止默认拖放行为；
 * 2. 重置拖拽状态（边框动画、遮罩等）；
 * 3. 提取并校验文件列表，过滤出非视觉、≤10MB 的文本文件；
 * 4. 读取合法文件内容并按行/段拆分；
 * 5. 将每个片段封装为 HistoryMessage 并追加到全局 knowledgeArray；
 * 6. 刷新界面展示，持久化到 lunar_notes.json；
 * 7. 给出成功或失败提示。
 *
 * @param event 拖放事件对象
 */
async function lunarNotesPanelDragAfterEvent(event: DragEvent): Promise<void> {
	// 阻止默认拖放行为
	event.preventDefault();
	try {
		// 恢复面板样式：移除动画与遮罩
		resetDragState();
		/** 获取用户拖入的文件列表 */
		const files = Array.from(event.dataTransfer?.files || []);
		// 校验文件列表是否为空
		if (!files.length) {
			EntryAPI.showSystemMessage('请拖入有效的文本文件', 'error');
			return;
		}
		/** 过滤掉视觉类或大文件，保留合法文本文件 */
		const validFiles = filterValidFiles(files);
		// 校验过滤后的文件列表是否为空
		if (!validFiles.length) {
			EntryAPI.showSystemMessage('请拖入有效的文本文件（文件大小不能超过 10MB 且不能包含图片或视频文件）', 'error');
			return;
		}
		/** 读取并拆分所有文件内容为片段 */
		const allFragments = await readAndSplitFiles(validFiles);
		/** 将片段转为知识库消息对象 */
		const messages = await createKnowledgeMessages(allFragments);
		// 刷新界面展示知识库
		loadPagesIntoWindow(messages);
		// 立即持久化到磁盘
		await batchProcessingKnowledgeWrite("knowledge/lunar_notes.json", messages);
		// 显示成功导入提示
		EntryAPI.showSystemMessage(`成功导入 ${allFragments.length} 个文本片段`, 'success');
	}
	catch (error) {
		if (error instanceof Error) {
			EntryAPI.showSystemMessage(`处理拖放文件时发生错误：${error.message}\n${error.stack}`, 'error');
		}
		else {
			EntryAPI.showSystemMessage('处理拖放文件时发生未知错误', 'error');
		}
	}
}

/**
 * 从文件列表中筛选出符合要求的文本文件。
 * 过滤规则：
 * 1. 排除视觉类文件（图片/视频等）；
 * 2. 文件大小不得超过 10MB。
 *
 * @param {File[]} files 待筛选的文件数组
 *
 * @returns {File[]} 符合条件的文件数组
 */
function filterValidFiles(files: File[]): File[] {
	return files.filter(
		file => {
			/** 获取文件扩展名并转为小写 */
			const extension = file.name.split('.').pop()?.toLowerCase() || '';
			/** 排除视觉类文件（图片/视频等） */
			const isNotVisionFile = !EntryAPI.OnlyData.visionExtensions.includes(extension);
			/** 检查文件大小是否不超过 10MB */
			const isWithinSizeLimit = file.size <= MAX_FILE_SIZE;
			// 同时满足以上两个条件则保留
			return isNotVisionFile && isWithinSizeLimit;
		}
	);
}

/**
 * 读取并拆分多个文本文件内容为片段数组。
 * 1. 遍历文件列表，逐个读取文本内容；
 * 2. 调用 FileAPI.splitTextToStrings 将文本拆分为片段；
 * 3. 收集所有片段到同一数组；
 * 4. 若读取失败，记录错误并提示用户。
 *
 * @param {File[]} files 待读取的文件数组
 * @returns {Promise<string[]>} 所有文件拆分后的片段数组
 */
async function readAndSplitFiles(files: File[]): Promise<string[]> {
	/** 所有文件的文本片段数组 */
	const allFragments: string[] = [];
	// 遍历每个文件
	for (const file of files) {
		try {
			/** 读取文件文本内容 */
			const textContent = await file.text();
			/** 拆分为片段 */
			const fragments = EntryAPI.splitTextToStrings(textContent);
			// 合并到总数组
			allFragments.push(...fragments);
		}
		catch (error) {
			EntryAPI.showSystemMessage(`读取文件 ${file.name} 时发生错误`, 'error');
		}
	}
	// 返回所有文件的片段数组
	return allFragments;
}

/**
 * 创建知识消息对象数组
 *
 * @param fragments 文本片段数组
 * @returns 消息对象数组
 */
async function createKnowledgeMessages(fragments: string[]): Promise<EntryAPI.HistoryMessage[]> {
	const messagePromises = fragments.map(
		async (text, index) => {
			// 添加延迟以避免对服务器造成过大压力
			if (index > 0) {
				await new Promise(resolve => setTimeout(resolve, 100));
			}
			return await EntryAPI.createMessageObject('assistant', text, false, true, false, null, true);
		}
	);
	return await Promise.all(messagePromises);
}

/**
 * 加载全局知识库数组中的所有消息到月华笔记面板中。
 * 1. 清空面板当前内容；
 * 2. 遍历知识库数组，对每条消息调用 renderMessage 函数渲染；
 * 3. 若消息渲染成功，绑定其内部的思考折叠按钮。
 */
async function loadPagesIntoWindow(dataSource: (EntryAPI.HistoryMessage)[]): Promise<void> {
	// 清空容器内的现有内容
	EntryAPI.lunarNotesPanel.innerHTML = '';
	// 滚动到容器顶部
	EntryAPI.lunarNotesPanel.scrollTo({ top: 0, behavior: 'smooth' });
	// 遍历对话历史中的每条消息
	dataSource.forEach(
		(message: EntryAPI.HistoryMessage) => {
			const newMessage = EntryAPI.renderMessage(message, EntryAPI.lunarNotesPanel);
			if (newMessage) {
				const toggleButtons = newMessage.querySelectorAll(".toggle_think_button") as NodeListOf<HTMLButtonElement>;
				toggleButtons.forEach(button => EntryAPI.bindFoldingButton(button));
			}
		}
	);
	// 若知识库为空，则显示占位符消息
	if (dataSource.length == 0) {
		await EntryAPI.renderingPagePlaceholders(EntryAPI.lunarNotesPanel);
	}
};

/**
 * 上传用户输入的知识到全局知识库数组中。
 * 1. 获取用户输入的消息；
 * 2. 若消息为空则直接返回；
 * 3. 将文本内容拆分为多个可存储的小片段；
 * 4. 将每个片段封装为知识消息对象，期间插入 100ms 延迟降低服务器瞬时压力；
 * 5. 批量插入知识数组并持久化到 lunar_notes.json。
 */
export async function uploadKnowledgeBase(): Promise<void> {
	const message = EntryAPI.getUserMessage();
	if (!message || message.length === 0) return;
	try {
		const messages = await createKnowledgeMessages(message);
		loadPagesIntoWindow(messages);
		await batchProcessingKnowledgeWrite("knowledge/lunar_notes.json", messages);
		EntryAPI.showSystemMessage(`成功上传 ${messages.length} 个文本片段`, 'success');
	}
	catch (error) {
		EntryAPI.showSystemMessage('上传知识库时发生错误', 'error');
	}
}

/**
 * 基于描述文本, 匹配情感模式与表情包
 *
 * @param {string} text - 进行情感匹配的文本
 */
export async function matchEmotionalPatterns(text: string): Promise<void> {
	try {
		// 等待 50ms 以对齐视觉效果
		await new Promise(resolve => setTimeout(resolve, 50));
		/** 生成输入文本的嵌入向量 */
		const embedVector = await new EntryAPI.EmbeddingRequest(text, false, false).output();
		/** 选择相似度最高的情感 */
		const selectedEmotion = (await captureKnowledgeRanking("knowledge/emotional_model.json", embedVector))[0].content;
		/** 若输入文本包含“害羞”，则强制设置为 Live2D 模型的 SHY 状态 */
		const correctedEmotion = /害羞/.test(text) ? EntryAPI.EmotionalState.SHY : selectedEmotion;
		// 更新 Live2D 模型情绪状态
		EntryAPI.setStateWithTimeout(correctedEmotion);
		// 50% 概率返回，不进入表情包匹配流程
		if (Math.random() > 0.5) return;
		/** 选中的表情包消息 */
		const selectedMeme = (await captureKnowledgeRanking("knowledge/meme_model.json", embedVector))[EntryAPI.RandomFloor(0, 4)];
		// 若该表情包无图片链接，则直接返回
		if (selectedMeme.imageUrl === null) return;
		/** 创建图片消息对象 */
		const imageMessage = EntryAPI.createImageMessage('assistant', '月华的表情包', selectedMeme.imageUrl);
		// 渲染表情包消息到聊天面板
		EntryAPI.addImageRendering(imageMessage);
	}
	// 若匹配表情包失败，则直接返回尴尬状态
	catch (error) {
		EntryAPI.setStateWithTimeout(EntryAPI.EmotionalState.EMBARRASSED);
		if (!(error instanceof Error)) return EntryAPI.showSystemMessage('匹配表情包时发生未知错误', 'error');
		EntryAPI.showSystemMessage('匹配表情包时发生错误：' + error.message + '\n' + error.stack, 'error');
	}
}

/**
 * 计算知识库消息与输入文本的相似度，返回带权重的消息数组。
 * 1. 过滤出已生成嵌入向量的知识库消息；
 * 2. 计算每个消息与输入文本的余弦相似度；
 * 3. 按相似度降序排序，返回前 maxContextMessages 条消息。
 * @param {EntryAPI.HistoryMessage[]} dataSource 知识库消息数组，包含用户和助手的交互记录
 *
 * @param {number[]} embedVector 智能体输入的文本生成的嵌入向量，用于与知识库消息进行相似度匹配
 *
 * @param {number} keepRecentCount 保留最近消息数量，默认值为 5
 *
 * @returns {EntryAPI.WeightedHistoryMessage[]} 带权重的知识库消息数组，按相似度降序排序
 */
export function knowledgeRanking(dataSource: EntryAPI.HistoryMessage[], embedVector: number[], keepRecentCount: number = 5): EntryAPI.HistoryMessage[] {
	/**
	 * 将知识库消息转换为带权重的消息对象，权重即与输入文本的相似度
	 *
	 * @param {EntryAPI.HistoryMessage} source 原始知识库消息
	 *
	 * @returns {EntryAPI.WeightedHistoryMessage} 带权重的消息对象
	 */
	function transformation(source: EntryAPI.HistoryMessage): EntryAPI.WeightedHistoryMessage {
		return {
			message: source,
			weight: EntryAPI.calculateCosineSimilarity(embedVector, source.embedVector!)
		};
	};
	/** 过滤出已生成嵌入向量的知识库消息，计算相似度并降序排序 */
	const dataProcessing = dataSource
		.filter(msg => msg.embedVector && msg.embedVector.length > 0)
		.map(transformation)
		.sort((a, b) => b.weight - a.weight);
	// 返回处理后的知识库消息数组
	return dataProcessing.slice(0, -keepRecentCount).map(item => item.message);
};


/**
 * 从指定 URL 查询知识库消息，根据输入文本生成的嵌入向量匹配相似度最高的消息。
 * 1. 发送 POST 请求到 '/knowledge/query' 接口，包含文件路径、查询向量和返回消息数量；
 * 2. 若查询成功，解析返回的知识库消息数组，计算相似度并降序排序；
 * 3. 若查询失败，返回空数组；
 * 4. 捕获并提示可能出现的异常。
 *
 * @param {string} url 知识库文件路径，用于指定查询的知识库
 *
 * @param {number[]} embedVector 智能体输入的文本生成的嵌入向量，用于与知识库消息进行相似度匹配
 *
 * @param {number} maxContextMessages 返回的知识库消息数量，默认值为 5
 *
 * @returns {Promise<EntryAPI.KnowledgeMessage[]>} 带权重的知识库消息数组，按相似度降序排序
 */
export async function captureKnowledgeRanking(url: string, embedVector: number[], maxContextMessages: number = 5): Promise<EntryAPI.KnowledgeMessage[]> {
	/** 知识库查询结果 */
	const remoteExecution = await fetch('/knowledge/query',
		{
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
			},
			body: JSON.stringify({ "filePath": url, "queryVector": embedVector, "topK": maxContextMessages }),
		}
	);
	// 若查询失败，返回空数组
	if (!remoteExecution.ok) return [];
	// 解析并返回知识库查询结果
	return await remoteExecution.json() as EntryAPI.KnowledgeMessage[];
};

/**
 * 向指定 URL 写入知识库消息，并发处理多个消息。
 * 1. 定义知识库消息写入处理器，每个消息发送 POST 请求到 '/knowledge/write' 接口，包含文件路径和消息内容；
 * 2. 将所有消息并发处理，确保写入顺序；
 * 3. 最后发送 POST 请求到 '/knowledge/flush' 接口，刷新知识库缓存。
 *
 * @param {string} url 知识库文件路径，用于指定写入的知识库
 *
 * @param {EntryAPI.HistoryMessage[]} messages 待写入的知识库消息数组
 */
export async function batchProcessingKnowledgeWrite(url: string, messages: EntryAPI.HistoryMessage[]): Promise<void> {
	/** 知识库消息写入处理器 */
	async function processor(message: EntryAPI.HistoryMessage) {
		// 若消息已保存，跳过写入
		if (savedHistoryTsg.has(message.uuid)) return;
		// 写入知识库消息
		await fetch('/knowledge/write',
			{
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({ "filePath": url, "message": message }),
			}
		);
		// 标记消息为已保存
		savedHistoryTsg.add(message.uuid);
	}
	// 并发写入知识库消息
	await Promise.all(messages.map(processor));
	// 刷新知识库缓存
	await fetch('/knowledge/flush',
		{
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
			},
			body: JSON.stringify({ "filePath": url }),
		}
	);
};

/**
 * 向指定 URL 删除知识库消息，并发处理多个消息。
 * 1. 定义知识库消息删除处理器，每个消息发送 POST 请求到 '/knowledge/delete' 接口，包含文件路径和消息 UUID；
 * 2. 将所有消息并发处理，确保删除顺序；
 * 3. 最后发送 POST 请求到 '/knowledge/flush' 接口，刷新知识库缓存。
 *
 * @param {string} url 知识库文件路径，用于指定删除的知识库
 *
 * @param {string[]} uuidArray 待删除的知识库消息 UUID 数组
 */
export async function batchProcessingKnowledgeDelete(url: string, uuidArray: string[]): Promise<void> {
	/** 知识库消息删除处理器 */
	async function processor(uuid: string) {
		await fetch('/knowledge/delete',
			{
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({ "filePath": url, "uuid": uuid }),
			}
		);
	}
	// 并发删除知识库消息
	await Promise.all(uuidArray.map(processor));
	// 刷新知识库缓存
	await fetch('/knowledge/flush',
		{
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
			},
			body: JSON.stringify({ "filePath": url }),
		}
	);
};

/**
 * 从指定 URL 获取知识库消息列表。
 * 1. 发送 POST 请求到 '/knowledge/list' 接口，包含文件路径；
 * 2. 若查询成功，解析返回的知识库消息数组，按消息创建时间降序排序；
 * 3. 若查询失败，返回空数组；
 * 4. 捕获并提示可能出现的异常。
 *
 * @param {string} url 知识库文件路径，用于指定查询的知识库
 *
 * @returns {Promise<EntryAPI.HistoryMessage[]>} 知识库消息数组，按创建时间降序排序
 */
export async function captureKnowledgeList(url: string): Promise<EntryAPI.HistoryMessage[]> {
	/** 知识库消息列表 */
	const remoteExecution = await fetch('/knowledge/list',
		{
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
			},
			body: JSON.stringify({ "filePath": url }),
		}
	);
	// 若查询失败，返回空数组
	if (!remoteExecution.ok) return [];
	// 解析并返回知识库消息列表
	return await remoteExecution.json() as EntryAPI.HistoryMessage[];
};

/**
 * 刷新指定知识库页面。
 * 1. 调用 captureKnowledgeList 函数获取知识库消息列表；
 * 2. 对获取的消息进行完善化处理，转换为历史消息格式；
 * 3. 调用 loadPagesIntoWindow 函数刷新知识库页面。
 *
 * @param {string} url 知识库文件路径，用于指定刷新的知识库
 */
export async function refreshKnowledgePage(url: string): Promise<void> {
	/** 知识库消息列表 */
	const knowledgeMessages = await captureKnowledgeList(url);
	// 刷新知识库页面
	loadPagesIntoWindow(knowledgeMessages);
};

/**
 * 对最终消息数组进行去重，保留首次出现顺序
 *
 * @param {EntryAPI.MixedMessage[]} finalMessages - 最终消息数组
 *
 * @returns {EntryAPI.MixedMessage[]} - 去重后的消息数组
 */
export function uniqueFinalMessages(finalMessages: EntryAPI.MixedMessage[]): EntryAPI.MixedMessage[] {
	/** 按 uuid 去重，保留首次出现顺序 */
	const seen = new Set<string>();
	/** 去重后的最终消息数组 */
	return finalMessages.filter(
		message => {
			// 无 uuid 直接保留
			if (!message.uuid) return true;
			// 重复则丢弃
			if (seen.has(message.uuid)) return false;
			// 非重复消息，添加uuid到集合
			seen.add(message.uuid);
			return true;
		}
	);
};

// 事件监听器注册
EntryAPI.lunarNotesPanel.addEventListener('dragleave', (event: DragEvent) => lunarNotesPanelDragEvent(event));
EntryAPI.lunarNotesPanel.addEventListener('dragover', (event: DragEvent) => lunarNotesPanelDragEvent(event));
EntryAPI.lunarNotesPanel.addEventListener('drop', (event: DragEvent) => lunarNotesPanelDragAfterEvent(event));