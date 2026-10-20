/**
 * 自定义角色库（SQLite 持久化）
 *
 * 存储复用共享知识条目通道 `knowledgeLoad / knowledgeSave`：
 * 实际落盘在 `local_data/database/character.db` 的 `painter_characters` 表，
 * 每行 key=角色名、value=角色档案 JSON。这样绘制者创建的角色在重启后仍然可用，
 * 也能被立绘与合照两个工具共同引用。
 *
 * 只存「角色是谁」（基础外观 + 默认服装），不存表情/姿势等一次性的演出参数。
 */

/** 自定义角色档案 */
export interface CustomCharacterProfile {
	/** 角色名（唯一键，也是 portrait 的 character / group_photo 的 roles 引用名） */
	name: string;
	/** 性别：女 / 男（决定通用立绘模板里的 1女孩 / 1男孩） */
	gender: string;
	/** 基础外观：年龄感、发型发色、瞳色、面部特征、配饰、体型 */
	appearance: string;
	/** 默认全尺寸服装（可空：空时用简洁日常服装兜底） */
	outfitFull?: string;
	/** 默认Q版服装（可空：空时用 outfitFull，再空用简洁日常服装兜底） */
	outfitChibi?: string;
}

/** 角色库表名（同时受 Go 侧表名白名单约束：只允许字母/数字/下划线/短横线） */
const CHARACTER_TABLE = 'painter_characters';

/**
 * 读取全部自定义角色
 *
 * @returns 角色名 → 角色档案（读取失败或表不存在时返回空 Map）
 */
export function loadCustomCharacters(): Map<string, CustomCharacterProfile> {
	const characters = new Map<string, CustomCharacterProfile>();
	try {
		const [entries, error] = knowledgeLoad(CHARACTER_TABLE);
		if (error) {
			console.error(`[角色库] 读取 ${CHARACTER_TABLE} 失败:`, error.message ?? String(error));
			return characters;
		}
		for (const pair of entries ?? []) {
			if (!Array.isArray(pair) || typeof pair[0] !== 'string' || !pair[0]) continue;
			const name = pair[0];
			try {
				const raw = JSON.parse(String(pair[1] ?? ''));
				const profile = normalizeProfile(raw, name);
				if (profile) characters.set(profile.name, profile);
			}
			catch (parseError) {
				console.warn(`[角色库] 角色「${name}」的档案不是合法 JSON，已跳过:`, parseError);
			}
		}
	}
	catch (error) {
		console.error('[角色库] 读取自定义角色失败:', error);
	}
	return characters;
}

/**
 * 创建或更新一个自定义角色（整表读改写；角色数量很小，无需增量 SQL）
 *
 * @param profile 角色档案
 * @returns 是否保存成功
 */
export function saveCustomCharacter(profile: CustomCharacterProfile): boolean {
	try {
		const characters = loadCustomCharacters();
		characters.set(profile.name, profile);
		const entries: Array<[string, string]> = [...characters.entries()]
			.map(([name, value]) => [name, JSON.stringify(value)]);
		const [ok, error] = knowledgeSave(CHARACTER_TABLE, entries);
		if (error) {
			console.error(`[角色库] 写入 ${CHARACTER_TABLE} 失败:`, error.message ?? String(error));
			return false;
		}
		return !!ok;
	}
	catch (error) {
		console.error('[角色库] 保存自定义角色失败:', error);
		return false;
	}
}

/** 把一条 SQLite 记录规整为角色档案；缺关键字段时返回 null */
function normalizeProfile(raw: any, fallbackName: string): CustomCharacterProfile | null {
	if (!raw || typeof raw !== 'object') return null;
	const name = String(raw.name || fallbackName).trim();
	const appearance = String(raw.appearance || '').trim();
	if (!name || !appearance) return null;
	const gender = raw.gender === '男' ? '男' : '女';
	return {
		name,
		gender,
		appearance,
		outfitFull: String(raw.outfitFull || '').trim() || undefined,
		outfitChibi: String(raw.outfitChibi || '').trim() || undefined,
	};
}
