import { ResourceAdmitInput, ResourceAdmitResult } from '../../config/resource';
import { bytesToBase64 } from './base';

/**
 * 统一入库：归一来源 → 落盘 → 登记资源目录 → 返回句柄
 *
 * @param input 入库入参（sourceKind 决定归一分支）
 * @returns 入库结果；失败返回 null（已打印错误日志）
 */
export function admitResource(input: ResourceAdmitInput): ResourceAdmitResult | null {
	try {
		// 字节入参统一走 base64 文本通道（goja 侧按字符串解码，避免二进制跨运行时失真）
		const payload = input.data instanceof Uint8Array
			? { ...input, data: bytesToBase64(input.data) }
			: input;
		const [result, error] = resourceAdmit(payload);
		if (error || !result) {
			console.error('[资源] 入库失败:', error?.message || '返回为空', input.sourceKind, input.name || input.source || '');
			return null;
		}
		return result;
	}
	catch (error) {
		console.error('[资源] 入库抛异常:', error);
		return null;
	}
}

/**
 * 按「documents/原文件名」入库或刷新文档（同路径复用句柄）
 *
 * 与 admitBytes 的区别：不按内容哈希另存副本，而是直接引用文档库中的原文件名实体；
 * 同一路径内容变化时只刷新哈希与段落表，句柄保持不变。
 *
 * @param path 相对 LocalDir 的路径（必须在 documents/ 下）
 * @param name 原始文件名
 * @returns 入库结果；失败返回 null
 */
export function admitDocumentFile(path: string, name: string): ResourceAdmitResult | null {
	try {
		const [result, error] = resourceAdmitDocument(path, name);
		if (error || !result) {
			console.error('[资源] 文档入库失败:', error?.message || '返回为空', path);
			return null;
		}
		return result;
	}
	catch (error) {
		console.error('[资源] 文档入库抛异常:', error);
		return null;
	}
}

/**
 * 入库前端已落盘的本地文件（零拷贝路径：只登记不复制）
 *
 * @param source 来源（相对 LocalDir 路径 / 本机服务地址 / file:// 前缀）
 * @param name 原始文件名
 * @param origin 来源语义
 * @param retain 保留等级建议
 * @returns 入库结果；失败返回 null
 */
export function admitLocalFile(
	source: string,
	name: string,
	origin: ResourceAdmitInput['origin'] = 'client-upload',
	retain?: ResourceAdmitInput['retain']
): ResourceAdmitResult | null {
	return admitResource({ sourceKind: 'path', source, name, origin, retain });
}

/**
 * 入库远程地址（QQ 等渠道仅给 URL 的图片 / 视频 / 音频）
 *
 * @param url 远程地址
 * @param name 建议文件名（可空：按 URL 路径推断）
 * @param origin 来源语义（默认 channel）
 * @param retain 保留等级建议
 * @returns 入库结果；失败返回 null
 */
export function admitRemoteURL(
	url: string,
	name = '',
	origin: ResourceAdmitInput['origin'] = 'channel',
	retain?: ResourceAdmitInput['retain']
): ResourceAdmitResult | null {
	return admitResource({ sourceKind: 'url', source: url, name, origin, retain });
}

/**
 * 入库字节 / data URI（工具产出、转发内联）
 *
 * @param data 原始字节或 base64 字符串
 * @param name 建议文件名（决定扩展名与类型码）
 * @param origin 来源语义（默认 tool-output）
 * @param retain 保留等级建议
 * @returns 入库结果；失败返回 null
 */
export function admitBytes(
	data: string | Uint8Array,
	name: string,
	origin: ResourceAdmitInput['origin'] = 'tool-output',
	retain?: ResourceAdmitInput['retain']
): ResourceAdmitResult | null {
	return admitResource({ sourceKind: 'bytes', data, name, origin, retain });
}

/**
 * 入库 data URI（内联图片直传场景）
 *
 * @param dataURI data:image/...;base64,... 形态
 * @param name 建议文件名
 * @param origin 来源语义
 * @param retain 保留等级建议
 * @returns 入库结果；失败返回 null
 */
export function admitDataURI(
	dataURI: string,
	name: string,
	origin: ResourceAdmitInput['origin'] = 'tool-output',
	retain?: ResourceAdmitInput['retain']
): ResourceAdmitResult | null {
	return admitResource({ sourceKind: 'data', source: dataURI, name, origin, retain });
}

/**
 * 入库图片并按**内容哈希**去重（同一张图反复发送时复用既有句柄）
 *
 * 为什么需要去重：入库层默认「每次入库一行、序号永不复用」，同一张图每发一次就会多一个
 * 目录行 —— 库里出现过同一张图占 `@I.002`/`@I.003`/`@I.004` 三条的情况。图片没有文件名
 * 这类业务身份，内容哈希就是它唯一的身份。
 *
 * **去重在 Go 侧完成**（`dedup: true`）：哈希与二进制都在入库层处理，JS 侧不参与。
 * 这是关键教训 —— 曾有一版在 JS 里用 `atob` 解码 base64 再取 `charCodeAt` 还原字节，
 * 而 `atob` 适配器返回的是 Go 字符串，非法 UTF-8 字节被逐字节替换为 U+FFFD（低字节 0xFD），
 * 导致所有经该路径入库的图片变成无法解码的垃圾数据。
 *
 * @param imageData 图片数据 URI（前端压缩后的图片）
 * @param name 建议文件名（决定扩展名与类型码）
 * @param origin 来源语义
 * @returns 入库结果（命中时复用既有句柄与实体）；失败返回 null
 */
export function admitImageDataURI(
	imageData: string,
	name: string,
	origin: ResourceAdmitInput['origin'] = 'client-upload'
): ResourceAdmitResult | null {
	if (!imageData) return null;
	// data URI 直接交给入库层解码：Go 侧解码是二进制安全的，且 hash 在那里一并算出
	if (imageData.startsWith('data:')) {
		return admitResource({ sourceKind: 'data', source: imageData, name, origin, kindHint: 'I', dedup: true });
	}
	// 非 data URI（已是本地路径或远程地址）：按来源归一，同样开启内容去重
	const sourceKind = /^https?:\/\//i.test(imageData) ? 'url' : 'path';
	return admitResource({ sourceKind, source: imageData, name, origin, kindHint: 'I', dedup: true });
}

/**
 * 按文件名把文件收进资源归档层（工作区文件与 LocalDir 文件都支持）
 *
 * 解析顺序（与用户心智一致：先系统资料，再项目文件）：
 *   1. 工作区相对路径（用户项目目录内的源码 / 文档）
 *   2. LocalDir 相对路径（系统数据目录内的文件）
 *
 * 资源名以**引用时书写的相对路径**为准，而不是裸文件名：项目里常有多个同名文件
 * （如各子系统的 build.ps1），只用裸名会让目录里出现多条无法区分的记录。
 *
 * 为什么需要它：对话里写 `@AGENTS.md` 这类引用时，文件可能从未被上传过，
 * 归档里自然没有它。此函数负责「按需收录」——把用户点名的文件读取进来建立存档副本，
 * 之后即可持久引用。工作区来源在 Go 侧只复制/硬链接，绝不移动或删除原文件。
 *
 * @param fileName 相对路径或文件名（可带目录前缀）
 * @returns 入库结果；两处都找不到或类型不受支持时返回 null
 */
export function admitDocumentByName(fileName: string): ResourceAdmitResult | null {
	const target = String(fileName || '').trim().replace(/^[#@]/, '');
	if (!target) return null;
	// 资源名保留用户书写的相对路径，避免同名文件在目录里无法区分
	const name = target.replace(/^\.\//, '');
	// 1. 工作区（项目目录）
	const fromWorkspace = admitResource({ sourceKind: 'workspace', source: target, name, origin: 'client-upload', retain: 'normal' });
	if (fromWorkspace) return fromWorkspace;
	// 2. LocalDir（系统数据目录）
	return admitResource({ sourceKind: 'path', source: target, name, origin: 'client-upload', retain: 'normal' });
}
