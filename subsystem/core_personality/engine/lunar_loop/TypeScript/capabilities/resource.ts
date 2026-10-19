// ===== 统一资源归档层的能力封装（TypeScript 侧唯一入口） =====
//
// 权威存储与句柄分配在 Go 侧（engine/persistence + engine/archive.go 适配器）。
// 本模块只做三件事：包错误、做缓存、把「丢失」翻译成可用的降级文本。
//
// 禁止在其它模块直接调用 resourceAdmit / resourceRead 等全局函数。
//
// 模块划分：
//   - base.ts   句柄错误类型与 base64 编码
//   - admit.ts  统一入库（路径 / URL / 字节 / data URI / 按需收录）
//   - read.ts   读取与查询（正文 / 大纲 / 目录 / 搜索 / 条目）
//   - manage.ts 维护（摘要 / 段落表 / 删除 / 降级文案）

export { HandleError, bytesToBase64 } from './resource/base';
export {
	admitResource,
	admitLocalFile,
	admitRemoteURL,
	admitBytes,
	admitDataURI,
	admitImageDataURI,
	admitDocumentFile,
	admitDocumentByName
} from './resource/admit';
export {
	readResource,
	outlineResource,
	listResources,
	searchResources,
	resourceStatistics,
	findResourceByName,
	getResourceEntry,
	resourceSource,
	LOST_FILE_NOTICE,
	HANDLE_INVALID_NOTICE
} from './resource/read';
export {
	setResourceSummary,
	setResourceSections,
	deleteResource,
	degradeNotice
} from './resource/manage';
export type { ResourceSectionDraftInput } from './resource/manage';
