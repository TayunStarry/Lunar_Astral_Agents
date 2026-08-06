import { GlobalConfig, modelResponse, ModelBuilder, PostMessage, BaseConfig } from '../index';

// ==== 档案类型定义 ====

/** 人物档案 */
interface PersonArchive {
	/** 名称（必填） */
	name: string;
	/** 外号/别名 */
	nickname?: string;
	/** 性别 */
	gender?: string;
	/** 性格特征 */
	personality?: string;
	/** 服饰特点 */
	clothing?: string;
	/** 所在地点 */
	location?: string;
	/** 饮食偏好 */
	dietaryPrefs?: string;
	/** 当前活动状态 */
	currentActivity?: string;
}

/** 事件档案 */
interface EventArchive {
	/** 事件简称（必填） */
	name: string;
	/** 事件类型 */
	type?: string;
	/** 发生时间 */
	time?: string;
	/** 发生地点 */
	location?: string;
	/** 关键注意事项 */
	keyNotes?: string;
}

/** 自我档案 */
interface SelfArchive {
	/** 当前心情状态 */
	mood?: string;
	/** 服饰描述 */
	clothing?: string;
	/** 正在进行的活动 */
	activity?: string;
	/** 当前需求或期望获取的物品/信息 */
	needs?: string;
}

/** 记忆库记录 */
interface MemoryRecord {
	id: string;
	content: string;
	similarity: number;
}

/** 档案提取结果（LLM 返回） */
interface ExtractResult<T> {
	/** 提取出的档案列表 */
	items: T[];
}

/** 档案合并结果（LLM 返回） */
interface MergeResultItem {
	/** 档案标识名称（用于匹配前缀） */
	name: string;
	/** 是否需要删除旧档案 */
	delete_old: boolean;
	/** 合并后的完整档案内容 */
	merged_content: string;
}

// ==== 档案前缀常量 ====

const PERSON_PREFIX = '[人物档案 - ';
const EVENT_PREFIX = '[事件档案 - ';
const SELF_PREFIX = '[自我档案]';

// ==== 提示词构建器 ====

/** 组织者提示词构建 */
class Prompt extends ModelBuilder {
	/** 当前地址缓存 */
	private currentLocation: string = '';

	/** 获取当前位置信息 */
	private getCurrentLocation(): string {
		if (this.currentLocation) return this.currentLocation;
		const [addressResult, error] = address();
		if (error || !addressResult || addressResult.length === 0) {
			this.currentLocation = '未知地点';
		} else {
			this.currentLocation = addressResult.join(' ');
		}
		return this.currentLocation;
	}

	/** 获取当前时间字符串 */
	private getCurrentTime(): string {
		return new Date().toLocaleString('zh-CN', {
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit',
			hour12: false
		});
	}

	/** 将消息列表格式化为文本 */
	private formatMessages(records: PostMessage[]): string {
		return records.map((msg, idx) => {
			const content = typeof msg.content === 'string'
				? msg.content
				: JSON.stringify(msg.content);
			const preview = content.length > 600 ? content.slice(0, 600) + '...' : content;
			return `[消息${idx + 1}] 角色:${msg.role} | ${preview}`;
		}).join('\n');
	}

	// ==== 人物档案提示词 ====

	/** 构建人物档案提取提示词 */
	protected buildPersonExtractPrompt(records: PostMessage[]): string {
		const currentTime = this.getCurrentTime();
		const currentLocation = this.getCurrentLocation();
		return `请从以下对话消息中提取所有人物信息，为每个人物生成一份档案。

【对话消息】
${this.formatMessages(records)}

【系统上下文】
- 当前时间: ${currentTime}
- 当前位置: ${currentLocation}

【提取规则】
1. 从消息中识别所有被提及的人物（包括说话者自身），为每个独立人物生成一份档案
2. 同一个人物不要重复提取
3. 若某字段在消息中未提及，则留空（不要编造）
4. 字段说明：
   - name: 人物名称（必填，如"月华"、"星光阁"）
   - nickname: 外号或别名
   - gender: 性别
   - personality: 性格特征描述
   - clothing: 服饰特点描述
   - location: 当前所在地点
   - dietaryPrefs: 饮食偏好
   - currentActivity: 当前正在进行的活动

【输出格式】
请输出 JSON 对象，包含 items 数组：
\`\`\`json
{
  "items": [
    {
      "name": "人物名称",
      "nickname": "外号",
      "gender": "性别",
      "personality": "性格特征",
      "clothing": "服饰特点",
      "location": "所在地点",
      "dietaryPrefs": "饮食偏好",
      "currentActivity": "当前活动"
    }
  ]
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
	}

	/** 构建人物档案合并提示词 */
	protected buildPersonMergePrompt(existingArchive: string, newInfo: PersonArchive): string {
		return `请将以下人物档案的新信息合并到已有档案中，补充和更新档案内容。

【已有档案】
${existingArchive}

【新信息】
${JSON.stringify(newInfo, null, 2)}

【合并规则】
1. 保留已有档案中所有仍然有效的信息
2. 用新信息补充和更新对应字段
3. 若新信息与已有信息冲突，以新信息为准（新信息更有时效性）
4. 不要删除已有档案中未与新信息冲突的字段

【输出格式】
请输出合并后的完整档案 JSON：
\`\`\`json
{
  "name": "人物名称",
  "nickname": "外号",
  "gender": "性别",
  "personality": "性格特征",
  "clothing": "服饰特点",
  "location": "所在地点",
  "dietaryPrefs": "饮食偏好",
  "currentActivity": "当前活动"
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
	}

	// ==== 事件档案提示词 ====

	/** 构建事件档案提取提示词 */
	protected buildEventExtractPrompt(records: PostMessage[]): string {
		const currentTime = this.getCurrentTime();
		const currentLocation = this.getCurrentLocation();
		return `请从以下对话消息中提取所有事件信息，为每个独立事件生成一份档案。

【对话消息】
${this.formatMessages(records)}

【系统上下文】
- 当前时间: ${currentTime}
- 当前位置: ${currentLocation}

【提取规则】
1. 从消息中识别所有已发生或正在发生的事件
2. 仅提取具有明确信息的事件，不要编造
3. 字段说明：
   - name: 事件简称（必填，如"星月祭典"、"代码审查"）
   - type: 事件类型（如"社交活动"、"工作会议"、"个人事务"）
   - time: 发生时间（从消息中提取，若无则使用当前时间）
   - location: 发生地点
   - keyNotes: 关键注意事项或重要细节

【输出格式】
请输出 JSON 对象，包含 items 数组：
\`\`\`json
{
  "items": [
    {
      "name": "事件简称",
      "type": "事件类型",
      "time": "发生时间",
      "location": "发生地点",
      "keyNotes": "关键注意事项"
    }
  ]
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
	}

	/** 构建事件档案合并提示词 */
	protected buildEventMergePrompt(existingArchive: string, newInfo: EventArchive): string {
		return `请将以下事件档案的新信息合并到已有档案中，补充和更新档案内容。

【已有档案】
${existingArchive}

【新信息】
${JSON.stringify(newInfo, null, 2)}

【合并规则】
1. 保留已有档案中所有仍然有效的信息
2. 用新信息补充和更新对应字段
3. 若新信息与已有信息冲突，以新信息为准

【输出格式】
请输出合并后的完整档案 JSON：
\`\`\`json
{
  "name": "事件简称",
  "type": "事件类型",
  "time": "发生时间",
  "location": "发生地点",
  "keyNotes": "关键注意事项"
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
	}

	// ==== 自我档案提示词 ====

	/** 构建自我档案提取提示词 */
	protected buildSelfExtractPrompt(records: PostMessage[]): string {
		return `请从以下对话消息中提取关于"月华"（即说话者自身）的当前状态信息。

【对话消息】
${this.formatMessages(records)}

【提取规则】
1. 仅提取关于月华自身的信息
2. 字段说明：
   - mood: 当前心情状态（如"开心"、"疲惫"、"专注"）
   - clothing: 当前服饰描述
   - activity: 正在进行的活动
   - needs: 当前需求或期望获取的物品/信息
3. 若某字段在消息中未提及，则留空

【输出格式】
请输出 JSON 对象：
\`\`\`json
{
  "mood": "心情状态",
  "clothing": "服饰描述",
  "activity": "正在进行的活动",
  "needs": "当前需求"
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
	}

	/** 构建自我档案合并提示词 */
	protected buildSelfMergePrompt(existingArchive: string, newInfo: SelfArchive): string {
		return `请将以下自我档案的新信息合并到已有档案中，补充和更新档案内容。

【已有档案】
${existingArchive}

【新信息】
${JSON.stringify(newInfo, null, 2)}

【合并规则】
1. 保留已有档案中所有仍然有效的信息
2. 用新信息补充和更新对应字段
3. 若新信息与已有信息冲突，以新信息为准

【输出格式】
请输出合并后的完整档案 JSON：
\`\`\`json
{
  "mood": "心情状态",
  "clothing": "服饰描述",
  "activity": "正在进行的活动",
  "needs": "当前需求"
}
\`\`\`

仅输出 JSON，不要包含其他说明文字。`;
	}

	// ==== 格式化方法 ====

	/** 将人物档案格式化为存储文本 */
	protected formatPersonArchive(archive: PersonArchive): string {
		const fields: string[] = [];
		if (archive.name) fields.push(`名称: ${archive.name}`);
		if (archive.nickname) fields.push(`外号: ${archive.nickname}`);
		if (archive.gender) fields.push(`性别: ${archive.gender}`);
		if (archive.personality) fields.push(`性格: ${archive.personality}`);
		if (archive.clothing) fields.push(`服饰: ${archive.clothing}`);
		if (archive.location) fields.push(`地点: ${archive.location}`);
		if (archive.dietaryPrefs) fields.push(`饮食: ${archive.dietaryPrefs}`);
		if (archive.currentActivity) fields.push(`活动: ${archive.currentActivity}`);
		return `${PERSON_PREFIX}${archive.name}]\n${fields.join('\n')}`;
	}

	/** 将事件档案格式化为存储文本 */
	protected formatEventArchive(archive: EventArchive): string {
		const fields: string[] = [];
		if (archive.name) fields.push(`事件: ${archive.name}`);
		if (archive.type) fields.push(`类型: ${archive.type}`);
		if (archive.time) fields.push(`时间: ${archive.time}`);
		if (archive.location) fields.push(`地点: ${archive.location}`);
		if (archive.keyNotes) fields.push(`注意事项: ${archive.keyNotes}`);
		return `${EVENT_PREFIX}${archive.name}]\n${fields.join('\n')}`;
	}

	/** 将自我档案格式化为存储文本 */
	protected formatSelfArchive(archive: SelfArchive): string {
		const fields: string[] = [];
		if (archive.mood) fields.push(`心情: ${archive.mood}`);
		if (archive.clothing) fields.push(`服饰: ${archive.clothing}`);
		if (archive.activity) fields.push(`活动: ${archive.activity}`);
		if (archive.needs) fields.push(`需求: ${archive.needs}`);
		return `${SELF_PREFIX}\n${fields.join('\n')}`;
	}

	/** 从存储文本中解析人物档案 */
	protected parsePersonArchive(content: string): PersonArchive | null {
		try {
			const result: PersonArchive = { name: '' };
			const lines = content.replace(PERSON_PREFIX, '').replace(/\]$/, '').split('\n');
			// 第一行是名称（在 ] 之前的部分）
			const headerMatch = content.match(/\[人物档案 - (.+?)\]/);
			if (headerMatch) result.name = headerMatch[1];
			for (const line of lines) {
				if (line.startsWith('名称: ')) result.name = result.name || line.slice(4);
				else if (line.startsWith('外号: ')) result.nickname = line.slice(4);
				else if (line.startsWith('性别: ')) result.gender = line.slice(4);
				else if (line.startsWith('性格: ')) result.personality = line.slice(4);
				else if (line.startsWith('服饰: ')) result.clothing = line.slice(4);
				else if (line.startsWith('地点: ')) result.location = line.slice(4);
				else if (line.startsWith('饮食: ')) result.dietaryPrefs = line.slice(4);
				else if (line.startsWith('活动: ')) result.currentActivity = line.slice(4);
			}
			return result.name ? result : null;
		} catch { return null; }
	}

	/** 从存储文本中解析事件档案 */
	protected parseEventArchive(content: string): EventArchive | null {
		try {
			const result: EventArchive = { name: '' };
			const headerMatch = content.match(/\[事件档案 - (.+?)\]/);
			if (headerMatch) result.name = headerMatch[1];
			const lines = content.replace(EVENT_PREFIX, '').replace(/\]$/, '').split('\n');
			for (const line of lines) {
				if (line.startsWith('事件: ')) result.name = result.name || line.slice(4);
				else if (line.startsWith('类型: ')) result.type = line.slice(4);
				else if (line.startsWith('时间: ')) result.time = line.slice(4);
				else if (line.startsWith('地点: ')) result.location = line.slice(4);
				else if (line.startsWith('注意事项: ')) result.keyNotes = line.slice(4);
			}
			return result.name ? result : null;
		} catch { return null; }
	}

	/** 从存储文本中解析自我档案 */
	protected parseSelfArchive(content: string): SelfArchive | null {
		try {
			const result: SelfArchive = {};
			const lines = content.replace(SELF_PREFIX, '').split('\n');
			for (const line of lines) {
				if (line.startsWith('心情: ')) result.mood = line.slice(4);
				else if (line.startsWith('服饰: ')) result.clothing = line.slice(4);
				else if (line.startsWith('活动: ')) result.activity = line.slice(4);
				else if (line.startsWith('需求: ')) result.needs = line.slice(4);
			}
			return (result.mood || result.clothing || result.activity || result.needs) ? result : null;
		} catch { return null; }
	}
}

// ==== 工具链 ====

/** 组织者工具链 */
class Toolchain extends Prompt {
	/** 查询记忆库 */
	protected queryMemory(queryText: string, topK: number = 10): MemoryRecord[] {
		if (!queryText || queryText.trim().length === 0) return [];
		const [results, error] = memoryQuery('lunar_messages', queryText.trim(), topK);
		if (error) {
			console.error('[组织者] 记忆库查询失败:', error);
			return [];
		}
		return results || [];
	}

	/** 按前缀查询档案（向量搜索 + 客户端前缀过滤） */
	protected queryArchiveByPrefix(prefix: string, topK: number = 50): MemoryRecord[] {
		const allResults = this.queryMemory(prefix, topK);
		return allResults.filter(r => r.content.startsWith(prefix));
	}

	/** 删除记忆库中的记录 */
	protected deleteRecords(ids: string[]): void {
		const uniqueIds = [...new Set(ids.filter(id => id && id.trim()))];
		if (uniqueIds.length === 0) return;
		console.log(`[组织者] 删除 ${uniqueIds.length} 条旧档案`);
		for (const id of uniqueIds) {
			const [, error] = memoryDelete('lunar_messages', id.trim());
			if (error) console.error(`[组织者] 删除记录 ${id} 失败:`, error);
			else console.log(`[组织者] 已删除记录 ${id}`);
		}
	}

	/** 写入档案到记忆库 */
	protected writeArchive(content: string): void {
		if (!content || content.trim().length === 0) return;
		const [, error] = memoryAdd('lunar_messages', 'assistant', content.trim());
		if (error) console.error('[组织者] 写入档案失败:', error);
		else console.log(`[组织者] 已写入档案: ${content.slice(0, 60)}...`);
	}

	/** 解析模型返回的 JSON */
	protected parseJsonResponse<T>(content: string): T | null {
		try {
			const jsonMatch = content.match(/```(?:json)?\s*([\s\S]*?)```/);
			const jsonStr = jsonMatch ? jsonMatch[1].trim() : content.trim();
			return JSON.parse(jsonStr) as T;
		} catch (error) {
			console.error('[组织者] JSON 解析失败:', error, '原始内容:', content.slice(0, 200));
			return null;
		}
	}

	/** 执行 LLM 推理并返回文本内容 */
	protected runLLM(prompt: string): string {
		this.coverContext({ role: 'user', content: prompt });
		this.runtimeMessages = [];
		try {
			const response = this.run([], []);
			return response.body?.choices?.[0]?.message?.content || '';
		} catch (error) {
			console.error('[组织者] LLM 推理失败:', error);
			return '';
		}
	}

	/** 执行 LLM 合并推理，返回合并后的对象 */
	protected runMergeLLM<T>(prompt: string): T | null {
		const content = this.runLLM(prompt);
		if (!content) return null;
		return this.parseJsonResponse<T>(content);
	}
}

// ==== 组织者角色 ====

/** 组织者角色 */
export class OrganizeRole extends Toolchain {
	/** 档案查询 topK */
	private readonly ARCHIVE_QUERY_TOPK = 50;

	constructor() {
		super(fileView('prompts/organizeRole.md')[0]);
	}

	/** 组织历史记录（主入口） */
	public organizeHistoricalRecords(): void {
		console.log('[组织者] 开始档案收集与整理');
		if (GlobalConfig.unreadRecords.length === 0) {
			console.log('[组织者] 没有未读记录需要整理');
			return;
		}

		if (!BaseConfig.memoryReady) {
			BaseConfig.initMemory();
			if (!BaseConfig.memoryReady) {
				console.warn('[组织者] 记忆库未就绪，保留未读记录待下次整理');
				return;
			}
		}

		const records = [...GlobalConfig.unreadRecords];

		try {
			// 严格按照 人物档案 → 事件档案 → 自我档案 的顺序处理
			this.processPersonArchives(records);
			this.processEventArchives(records);
			this.processSelfArchive(records);

			console.log('[组织者] 档案整理完成');
			GlobalConfig.unreadRecords = [];
		}
		catch (error) {
			console.error('[组织者] 档案整理失败，保留未读记录待下次重试:', error);
		}
	}

	// ==== 人物档案处理 ====

	/** 处理人物档案 */
	private processPersonArchives(records: PostMessage[]): void {
		console.log('[组织者] === 阶段一：人物档案处理 ===');

		// 第一步：从消息中提取人物信息
		const prompt = this.buildPersonExtractPrompt(records);
		const content = this.runLLM(prompt);
		if (!content) {
			console.log('[组织者] 人物档案提取未获得有效结果');
			return;
		}

		const result = this.parseJsonResponse<ExtractResult<PersonArchive>>(content);
		if (!result || !result.items || result.items.length === 0) {
			console.log('[组织者] 未提取到人物信息');
			return;
		}

		console.log(`[组织者] 提取到 ${result.items.length} 个人物档案`);

		// 第二步：对每个人物，查询已有档案并合并
		for (const person of result.items) {
			if (!person.name) continue;
			this.processSinglePersonArchive(person);
		}
	}

	/** 处理单个人物档案 */
	private processSinglePersonArchive(newInfo: PersonArchive): void {
		const prefix = `${PERSON_PREFIX}${newInfo.name}]`;
		console.log(`[组织者] 处理人物档案: ${newInfo.name}`);

		// 查询记忆库中是否存在该人物的旧档案
		const existingRecords = this.queryArchiveByPrefix(prefix, this.ARCHIVE_QUERY_TOPK);

		if (existingRecords.length === 0) {
			// 无旧档案，直接写入
			const archiveText = this.formatPersonArchive(newInfo);
			this.writeArchive(archiveText);
			console.log(`[组织者] 新增人物档案: ${newInfo.name}`);
			return;
		}

		// 有旧档案，执行合并更新
		console.log(`[组织者] 发现 ${newInfo.name} 的旧档案 ${existingRecords.length} 条，执行合并`);
		for (const record of existingRecords) {
			// 解析旧档案
			const oldArchive = this.parsePersonArchive(record.content);
			if (!oldArchive) {
				// 旧档案格式异常，删除并写入新档案
				this.deleteRecords([record.id]);
				continue;
			}

			// 合并
			const mergePrompt = this.buildPersonMergePrompt(record.content, newInfo);
			const merged = this.runMergeLLM<PersonArchive>(mergePrompt);
			if (merged && merged.name) {
				// 删除旧档案，写入合并后的新档案
				this.deleteRecords([record.id]);
				const archiveText = this.formatPersonArchive(merged);
				this.writeArchive(archiveText);
				console.log(`[组织者] 合并更新人物档案: ${merged.name}`);
			} else {
				// 合并失败，保留旧档案，写入新信息作为补充
				const archiveText = this.formatPersonArchive(newInfo);
				this.writeArchive(archiveText);
				console.log(`[组织者] 合并失败，新信息作为补充写入: ${newInfo.name}`);
			}
		}
	}

	// ==== 事件档案处理 ====

	/** 处理事件档案 */
	private processEventArchives(records: PostMessage[]): void {
		console.log('[组织者] === 阶段二：事件档案处理 ===');

		// 第一步：从消息中提取事件信息
		const prompt = this.buildEventExtractPrompt(records);
		const content = this.runLLM(prompt);
		if (!content) {
			console.log('[组织者] 事件档案提取未获得有效结果');
			return;
		}

		const result = this.parseJsonResponse<ExtractResult<EventArchive>>(content);
		if (!result || !result.items || result.items.length === 0) {
			console.log('[组织者] 未提取到事件信息');
			return;
		}

		console.log(`[组织者] 提取到 ${result.items.length} 个事件档案`);

		// 第二步：对每个事件，查询已有档案并合并
		for (const event of result.items) {
			if (!event.name) continue;
			this.processSingleEventArchive(event);
		}
	}

	/** 处理单个事件档案 */
	private processSingleEventArchive(newInfo: EventArchive): void {
		const prefix = `${EVENT_PREFIX}${newInfo.name}]`;
		console.log(`[组织者] 处理事件档案: ${newInfo.name}`);

		// 查询记忆库中是否存在该事件的旧档案
		const existingRecords = this.queryArchiveByPrefix(prefix, this.ARCHIVE_QUERY_TOPK);

		if (existingRecords.length === 0) {
			// 无旧档案，直接写入
			const archiveText = this.formatEventArchive(newInfo);
			this.writeArchive(archiveText);
			console.log(`[组织者] 新增事件档案: ${newInfo.name}`);
			return;
		}

		// 有旧档案，执行合并更新
		console.log(`[组织者] 发现 ${newInfo.name} 的旧档案 ${existingRecords.length} 条，执行合并`);
		for (const record of existingRecords) {
			// 合并
			const mergePrompt = this.buildEventMergePrompt(record.content, newInfo);
			const merged = this.runMergeLLM<EventArchive>(mergePrompt);
			if (merged && merged.name) {
				this.deleteRecords([record.id]);
				const archiveText = this.formatEventArchive(merged);
				this.writeArchive(archiveText);
				console.log(`[组织者] 合并更新事件档案: ${merged.name}`);
			} else {
				const archiveText = this.formatEventArchive(newInfo);
				this.writeArchive(archiveText);
				console.log(`[组织者] 合并失败，新信息作为补充写入: ${newInfo.name}`);
			}
		}
	}

	// ==== 自我档案处理 ====

	/** 处理自我档案 */
	private processSelfArchive(records: PostMessage[]): void {
		console.log('[组织者] === 阶段三：自我档案处理 ===');

		// 第一步：从消息中提取自我信息
		const prompt = this.buildSelfExtractPrompt(records);
		const content = this.runLLM(prompt);
		if (!content) {
			console.log('[组织者] 自我档案提取未获得有效结果');
			return;
		}

		const newInfo = this.parseJsonResponse<SelfArchive>(content);
		if (!newInfo || (!newInfo.mood && !newInfo.clothing && !newInfo.activity && !newInfo.needs)) {
			console.log('[组织者] 未提取到有效的自我信息');
			return;
		}

		console.log('[组织者] 处理自我档案');

		// 第二步：查询已有自我档案
		const existingRecords = this.queryArchiveByPrefix(SELF_PREFIX, this.ARCHIVE_QUERY_TOPK);

		if (existingRecords.length === 0) {
			// 无旧档案，直接写入
			const archiveText = this.formatSelfArchive(newInfo);
			this.writeArchive(archiveText);
			console.log('[组织者] 新增自我档案');
			return;
		}

		// 有旧档案，执行合并更新
		console.log(`[组织者] 发现自我旧档案 ${existingRecords.length} 条，执行合并`);
		for (const record of existingRecords) {
			const mergePrompt = this.buildSelfMergePrompt(record.content, newInfo);
			const merged = this.runMergeLLM<SelfArchive>(mergePrompt);
			if (merged && (merged.mood || merged.clothing || merged.activity || merged.needs)) {
				this.deleteRecords([record.id]);
				const archiveText = this.formatSelfArchive(merged);
				this.writeArchive(archiveText);
				console.log('[组织者] 合并更新自我档案');
			} else {
				const archiveText = this.formatSelfArchive(newInfo);
				this.writeArchive(archiveText);
				console.log('[组织者] 合并失败，新信息作为补充写入');
			}
		}
	}

	// ==== 公共 API（保持向后兼容） ====

	/** 持久化被抛弃的消息 */
	public persistDiscardedMessages(discarded: PostMessage[]): void {
		console.log('[组织者] 开始持久化被抛弃的消息');
		if (!BaseConfig.memoryReady) BaseConfig.initMemory();
		if (!BaseConfig.memoryReady) return;
		for (const message of discarded) {
			const content = typeof message.content === 'string' ? message.content : JSON.stringify(message.content);
			memoryAdd('lunar_messages', message.role, content);
		}
	}

	/** 查询历史记录 */
	public queryHistoricalRecords(queryText: string, topK: number = 10): (PostMessage & { id: string })[] {
		if (!BaseConfig.memoryReady) BaseConfig.initMemory();
		if (!BaseConfig.memoryReady) return [];

		const [results, error] = memoryQuery('lunar_messages', queryText, topK);
		if (error) {
			console.error('[组织者] 查询历史记录失败:', error);
			return [];
		}

		if (!results || results.length === 0) return [];

		return results.map((r: { id: string; role: string; content: string }) => ({
			id: r.id,
			role: r.role as PostMessage['role'],
			content: r.content
		}));
	}

	/** 获取历史记录上下文 */
	public getHistoricalContext(maxResults: number = 5): string {
		const records = this.queryHistoricalRecords('近期对话 重要事件', maxResults);
		if (records.length === 0) return '';
		return records.map(r => r.content).join('\n');
	}
}