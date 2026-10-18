/**
 * 提示词文档：把 Markdown 按标题分层拆成「标题路径 → 正文」的只读视图。
 *
 * 为什么要这层：角色外观、默认服装、默认表情/姿势等原本散落在 painter.ts 的
 * 字符串常量里，改一句提示词就要动 TypeScript 并重新打包。抽到 prompts/*.md 后，
 * 提示词成为纯数据，可以由策划直接维护；本模块只负责按 `## 角色` → `### 用途`
 * 的标题路径把对应段落取出来，供绘制者按需拼接。
 *
 * 约定：
 *   - 一级标题（`#`）是文档标题，不参与路径；
 *   - 二级及以下标题依次拼成路径，用 `/` 分隔（如「月华/服装/全尺寸」）；
 *   - `list()` 只收集 `-` / `*` 开头的列表项，适合表情、姿势、背景等预设。
 */

/** 提示词文档（按标题路径读取正文） */
export class PromptDocument {
	/** 标题路径 → 正文（保留原始换行） */
	private readonly sections: Map<string, string> = new Map();

	/**
	 * @param text 文档全文
	 * @param file 文件名（仅用于报错定位）
	 */
	constructor(text: string, private readonly file: string) {
		/** 当前标题栈（保留层级，用于在遇到同级/上级标题时回退） */
		let stack: Array<{ level: number; title: string }> = [];
		/** 当前正文归属的标题路径（空串表示还没进入二级标题） */
		let currentPath = '';

		for (const line of text.split(/\r?\n/)) {
			/** Markdown 标题行：1~6 个 # + 空白 + 标题 */
			const heading = /^(#{1,6})\s+(.+?)\s*$/.exec(line);
			if (heading) {
				const level = heading[1].length;
				const title = heading[2].trim();
				// 回退到最近的上级标题，再压入当前标题
				stack = stack.filter(item => item.level < level);
				stack.push({ level, title });
				// 一级标题只是文档名，不进入路径
				currentPath = stack.filter(item => item.level >= 2).map(item => item.title).join('/');
				if (currentPath && !this.sections.has(currentPath)) this.sections.set(currentPath, '');
				continue;
			}
			if (!currentPath) continue;
			this.sections.set(currentPath, (this.sections.get(currentPath) || '') + line + '\n');
		}
	}

	/**
	 * 读取某节的正文（已去除首尾空白）
	 *
	 * @param path 标题路径（如「月华/基础外观」）
	 * @returns 该节正文
	 */
	text(path: string): string {
		const body = this.sections.get(path);
		if (body === undefined) {
			throw new Error(`提示词文件 ${this.file} 缺少标题节「${path}」`);
		}
		const trimmed = body.trim();
		if (!trimmed) {
			throw new Error(`提示词文件 ${this.file} 的标题节「${path}」内容为空`);
		}
		return trimmed;
	}

	/**
	 * 读取某节的列表项（`-` / `*` 开头的行，已去除前缀）
	 *
	 * @param path 标题路径（如「默认表情」）
	 * @returns 列表项数组
	 */
	list(path: string): string[] {
		const body = this.text(path);
		const items = body
			.split(/\r?\n/)
			.map(line => line.trim())
			.filter(line => /^[-*]\s+/.test(line))
			.map(line => line.replace(/^[-*]\s+/, '').trim())
			.filter(item => item.length > 0);
		if (items.length === 0) {
			throw new Error(`提示词文件 ${this.file} 的标题节「${path}」没有任何列表项`);
		}
		return items;
	}
}

/**
 * 读取内嵌提示词并解析为可按标题取用的文档
 *
 * @param file 提示词虚拟路径（如 `prompts/painterCharacters.md`）
 * @returns 提示词文档
 */
export function loadPromptDocument(file: string): PromptDocument {
	const [content, error] = fileView(file);
	if (error) throw new Error(`读取提示词 ${file} 失败: ${error.message ?? String(error)}`);
	if (!content) throw new Error(`提示词 ${file} 内容为空`);
	return new PromptDocument(content, file);
}
