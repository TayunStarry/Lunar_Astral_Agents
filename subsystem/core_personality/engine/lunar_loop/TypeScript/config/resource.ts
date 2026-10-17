/** 统一资源归档层的类型定义（与 Go 侧 FileManager/module 的 JSON 字段一一对应） */

/** 资源类型码：D 文档 / I 图片 / V 视频 / A 音频 / T 表格 */
export type ResourceKindCode = 'D' | 'I' | 'V' | 'A' | 'T';

/** 资源来源语义（决定默认保留等级与入库行为） */
export type ResourceOrigin = 'client-upload' | 'channel' | 'tool-output' | 'engine-temp';

/** 保留等级：normal（A 必存）/ soft（B 产出）/ ephemeral（C 参考） */
export type ResourceRetain = 'normal' | 'soft' | 'ephemeral';

/** 资源 blob 状态 */
export type ResourceState = 'active' | 'lost';

/** 单条资源目录记录（Go 侧 ArchiveResource 的 JSON 形态） */
export interface ResourceEntry {
	/** 资源目录自增主键 */
	id: number;
	/** 类型码 */
	kind: ResourceKindCode;
	/** 资源序号 */
	seq: number;
	/** 规范句柄，形如 @D.003 */
	handle: string;
	/** 原始文件名 */
	name: string;
	/** 小写扩展名（含点，可空） */
	ext: string;
	/** 规范 MIME */
	mime: string;
	/** 内容哈希（sha16） */
	content_hash: string;
	/** 相对 LocalDir 的物理路径（内容仓库位置） */
	blob_path: string;
	/** 字节数 */
	size_bytes: number;
	/** 图片宽度（可空） */
	width?: number;
	/** 图片高度（可空） */
	height?: number;
	/** 时长（毫秒，可空） */
	duration_ms?: number;
	/** 展示标题 */
	title: string;
	/** 摘要 / 描述 */
	summary: string;
	/** 标签 */
	tags: string[];
	/** 来源语义 */
	origin: string;
	/** 保留等级 */
	retain: ResourceRetain;
	/** blob 状态 */
	state: ResourceState;
	/** 段落数（文档） */
	section_count: number;
	/** 入库 Unix 秒时间戳 */
	created_at: number;
	/** 最近取用 Unix 秒时间戳（回收排序依据） */
	used_at: number;
}

/** 统一入库入参 */
export interface ResourceAdmitInput {
	/** 来源类型：path（LocalDir 内文件/本机服务地址）/ url（远程）/ data（data URI）/ bytes（字节）/ workspace（工作区文件，只读收录） */
	sourceKind: 'path' | 'url' | 'data' | 'bytes' | 'workspace';
	/** 来源值（相对路径 / 地址 / data URI）；workspace 时为相对工作目录的路径 */
	source?: string;
	/** 原始字节 */
	data?: string | Uint8Array;
	/** 建议文件名（可空：path 来源自动取基名） */
	name?: string;
	/** 类型码建议值（可空：按扩展名推断） */
	kindHint?: ResourceKindCode;
	/** 来源语义 */
	origin?: ResourceOrigin;
	/** 保留等级建议值 */
	retain?: ResourceRetain;
	/** 建议 MIME */
	mime?: string;
	/**
	 * 按内容去重：命中「同类型码 + 同内容哈希」的既有资源时复用它，不再新增目录行
	 *
	 * 图片没有文件名这类业务身份，内容哈希就是唯一身份；同一张图反复发送时应开启。
	 * 去重在 Go 侧完成（入库层算完哈希后查目录），JS 侧不参与二进制或哈希计算。
	 */
	dedup?: boolean;
}

/** 入库结果 */
export interface ResourceAdmitResult {
	/** 规范句柄（形如 @I.007） */
	handle: string;
	/** 类型码 */
	kind: ResourceKindCode;
	/** 资源序号 */
	seq: number;
	/** 资源目录主键（写摘要 / 段落表时使用） */
	id: number;
	/** 内容哈希（sha16） */
	content_hash: string;
	/** 相对 LocalDir 的物理路径（可直接交给图片处理管线） */
	blob_path: string;
	/** 规范 MIME */
	mime: string;
	/** 字节数 */
	size_bytes: number;
	/** 原始文件名 */
	name: string;
	/** 是否复用了既有 blob（内容去重命中） */
	deduped: boolean;
	/** blob 状态 */
	state: ResourceState;
}

/** 资源取回结果 */
export interface ResourceReadResult {
	/** 请求的句柄（含锚点） */
	handle: string;
	/** 类型码 */
	kind: ResourceKindCode;
	/** 展示标题 */
	title: string;
	/** 文本内容（文档 / 表格；携带锚点时为其完整行区间） */
	content?: string;
	/** 二进制内容 base64
	 *
	 * 图片**不是**原图：资源取回对图片一律返回视觉读取副本（≤ vision_max_side，
	 * 见 docs/adr/0008）。视频 / 音频仍为本体。
	 */
	base64?: string;
	/** 规范 MIME */
	mime: string;
	/** 内容仓库路径（相对 LocalDir） */
	blob_path: string;
	/** 图片副本的像素宽度（仅图片取回时填写） */
	width?: number;
	/** 图片副本的像素高度（仅图片取回时填写） */
	height?: number;
	/** blob 已丢失：句柄有效但本体不可取，应降级为描述 */
	lost: boolean;
	/** 内容按上限被截断 */
	truncated: boolean;
	/** 命中的章节标题（携带 § 锚点时） */
	section_title?: string;
	/** 实际返回的行区间（1 基，含端点） */
	line_start?: number;
	line_end?: number;
	/** 资源总行数（文本文档） */
	total_lines?: number;
}

/** 资源段落（索引单元，不进句柄文法） */
export interface ResourceSection {
	/** 段落主键 */
	id: number;
	/** 所属资源主键 */
	resource_id: number;
	/** 章节序号（句柄 |§n 使用） */
	ordinal: number;
	/** 标题层级 1..6（无标题文本为 0） */
	level: number;
	/** 标题文本 */
	title: string;
	/** 层级路径 */
	path: string;
	/** 正文起始行（1 基） */
	line_start: number;
	/** 正文结束行（含） */
	line_end: number;
	/** 正文字数 */
	chars: number;
}

/** 段落表写入草稿（字段名为 Go 侧驼峰约定） */
export interface ResourceSectionDraft {
	ordinal: number;
	level: number;
	title: string;
	path: string;
	lineStart: number;
	lineEnd: number;
	chars: number;
}

/** 跨资源搜索命中 */
export interface ResourceSearchHit {
	/** 规范句柄 */
	handle: string;
	/** 类型码 */
	kind: ResourceKindCode;
	/** 展示标题 */
	title: string;
	/** 摘要 */
	summary: string;
	/** 命中章节（文档） */
	sections?: ResourceSection[];
	/** 命中位置：card（资源卡片）/ section（段落标题） */
	matched_in: string;
}

/** 资源目录统计 */
export interface ResourceStats {
	/** 资源总条数 */
	total: number;
	/** 按类型码分布 */
	by_kind: Record<string, number>;
	/** 按 blob 状态分布 */
	by_state: Record<string, number>;
	/** 按保留等级分布 */
	by_retain: Record<string, number>;
	/** 目录记录总体积（字节） */
	bytes: number;
}
