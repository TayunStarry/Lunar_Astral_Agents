/** 资源句柄语法错误（调用方据此区分「参数不合法」与「运行时失败」） */
export class HandleError extends Error {
	constructor(message: string) {
		super(message);
		this.name = 'HandleError';
	}
}

/** base64 编码（本地实现，避免依赖运行时 atob/btoa 的可用性） */
const BASE64_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

/**
 * 把字节数组编码为 base64 字符串
 *
 * @param bytes 原始字节
 * @returns base64 文本
 */
export function bytesToBase64(bytes: Uint8Array): string {
	let out = '';
	for (let i = 0; i < bytes.length; i += 3) {
		const b0 = bytes[i];
		const b1 = i + 1 < bytes.length ? bytes[i + 1] : 0;
		const b2 = i + 2 < bytes.length ? bytes[i + 2] : 0;
		out += BASE64_ALPHABET[b0 >> 2];
		out += BASE64_ALPHABET[((b0 & 0x03) << 4) | (b1 >> 4)];
		out += i + 1 < bytes.length ? BASE64_ALPHABET[((b1 & 0x0f) << 2) | (b2 >> 6)] : '=';
		out += i + 2 < bytes.length ? BASE64_ALPHABET[b2 & 0x3f] : '=';
	}
	return out;
}
