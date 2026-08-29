/** 文本分割入口 — 聚合普通文本/Markdown/代码拆分模块，保持公共 API 不变 */
import { resolveCodeLang, splitCodeFile } from "./code-split";
import { splitPlainText } from './split-plain';
import { splitMarkdown } from './split-markdown';

/** 文本分割选项 */
export type SplitOptions = {
	/** 理想的分割长度，默认从 SliderAPI.messageSliceLengthSlider 获取 */
	idealLen?: number;
	/** 路径前缀格式，默认 "[类别]: " */
	pathPrefix?: string;
	/** 是否将路径单独放在一行，默认 true */
	pathOnNewLine?: boolean;
	/** 是否跳过只有标题没有内容的片段，默认 true */
	skipTitleOnly?: boolean;
	/** 是否在内容中包含原始标题，默认 false */
	includeOriginalTitle?: boolean;
	/** 指定代码语言（如 "go"/"ts"/"py"...，来自 resolveCodeLang 支持的扩展名）。
	 *  传入后将走代码感知拆分（捕获 Function/Class），否则按 MD/普通文本拆分。 */
	lang?: string;
};

/**
 * 将输入文本按指定长度拆分成若干字符串片段。
 *
 * 若文本像 Markdown，则按标题层级拆分并附带路径前缀；
 *
 * 否则按普通文本规则拆分。
 *
 * @param {string} input - 原始文本
 *
 * @param {SplitOptions} options - 拆分行为选项
 *
 * @returns {string[]} - 拆分后的字符串数组
 */
export function splitTextToStrings(input: string, options: SplitOptions = {}): string[] {
	/** 合并默认选项，确保后续逻辑一定能拿到完整配置 */
	const option: Required<SplitOptions> = {
		idealLen: options.idealLen ?? 1024,
		pathPrefix: options.pathPrefix ?? "*标题> ",
		pathOnNewLine: options.pathOnNewLine ?? true,
		skipTitleOnly: options.skipTitleOnly ?? true,
		includeOriginalTitle: options.includeOriginalTitle ?? false,
		lang: options.lang ?? "",
	};
	/** 统一换行符，避免 Windows 换行导致后续处理不一致 */
	const text = (input ?? "").replace(/\r\n/g, "\n");
	// 空文本直接返回空数组，避免无意义处理
	if (!text.trim()) return [];
	// 若显式指定了代码语言，走代码感知拆分（捕获 Function/Class）
	if (option.lang) {
		const codeLang = resolveCodeLang(option.lang);
		if (codeLang) return splitCodeFile(text, codeLang, option.idealLen);
	}
	/** 判断是否为 Markdown：通过常见语法特征快速识别 */
	const isMarkdown = looksLikeMarkdown(text);
	// 按类型分流：普通文本直接按长度拆分；Markdown 需保留结构
	if (!isMarkdown) {
		return splitPlainText(text, option.idealLen);
	}
	// 按 Markdown 标题层级拆分
	return splitMarkdown(text, option);
};

/** 判断文本是否看起来像 Markdown 格式 */
function looksLikeMarkdown(text: string): boolean {
	/** 是否包含标题行 */
	const hasHeading = /(^|\n)#{1,6}\s+\S/.test(text);
	/** 是否包含代码块围栏 */
	const hasFence = /(^|\n)```/.test(text);
	/** 是否包含列表项 */
	const hasList = /(^|\n)\s*([-*+]|\d+\.)\s+\S/.test(text);
	/** 是否包含引用块 */
	const hasQuote = /(^|\n)>\s+\S/.test(text);
	/** 是否包含表格 */
	const hasTable = /(^|\n)\s*\|.*\|/.test(text);
	// 返回判断结果
	return hasHeading || hasFence || hasList || hasQuote || hasTable;
};
