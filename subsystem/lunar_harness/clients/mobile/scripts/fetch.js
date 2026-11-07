/**
 * 移动端后端接口：文件保存 / 发送消息统一走共享层。
 * saveFile 保留移动端原有的「返回原始响应 JSON」约定。
 */
import { writeHashedFile } from '/shared/core/files.js';

export { sendMessages } from '/shared/core/api.js';

/**
 * 保存文件到服务器
 * @param {File} file
 * @param {boolean} [overwrite=false]
 * @returns {Promise<object>} 服务端响应对象
 */
export async function saveFile(file, overwrite = false) {
	const { response } = await writeHashedFile(file, { overwrite, hashLength: 16 });
	return response;
}
