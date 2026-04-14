import { toBtoaString } from '../index';
/**
 * 使用 Fetch API 异步保存文件到服务器
 *
 * @param {Blob|File|FormData|string} fileData - 要保存的文件数据
 *
 * @param {string} fileName - 文件名
 *
 * @param {boolean} [overwrite=false] - 是否覆盖已存在的文件，默认为 false
 *
 * @returns {Promise<Object>} - 包含保存结果的 Promise，成功时返回服务器响应的 JSON 数据
 *
 * @throws {Error} - 当文件保存失败时抛出错误，包含错误名称、消息和栈信息
 */
export async function saveFileWithFetch(fileData: Blob | File | FormData | string, fileName: string, overwrite: boolean = false): Promise<object> {
    try {
        /**
         * 移除文件名中可能导致路径问题的特殊字符，将其替换为空格
         */
        const safeFileName = fileName.replace(/[:*?"<>|]/g, '_');
        /**
         * 发起 POST 请求，将文件数据保存到服务器
         */
        const response = await fetch('/save',
            {
                method: 'POST',
                // 设置请求头，包含编码后的文件名和是否覆盖的标志
                headers: {
                    'X-File-Name': toBtoaString(safeFileName),
                    'X-Overwrite': overwrite.toString()
                },
                body: fileData
            }
        );
        // 检查响应状态，若请求失败则抛出错误
        if (!response.ok) throw new Error(`文件保存失败: ${response.status}`);
        /**
         * 解析服务器返回的 JSON 数据
         */
        const result = await response.json();
        // 返回解析后的 JSON 数据
        return result;
    }
    catch (error) {
        // 捕获异常并返回错误信息
        if (error instanceof Error) return { error: `${error.name} | ${error.message} | ${error.stack}` }
        // 若捕获到的异常不是 Error 实例，显示未知错误信息
        else return { error: `未知错误: ${error}` }
    }
};