import * as EntryAPI from '../EntryAPI/code';
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
                    'X-File-Name': EntryAPI.toBtoaString(safeFileName),
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
        // 显示系统消息，告知用户文件保存成功及保存的文件名
        EntryAPI.showSystemMessage(`文件保存成功: ${result.filename}`, "success");
        return result;
    }
    catch (error) {
        if (error instanceof Error) {
            // 捕获异常并显示错误信息
            EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
            throw error;
        }
        else {
            // 若捕获到的异常不是 Error 实例，显示未知错误信息
            EntryAPI.showSystemMessage(`未知错误: ${error}`, "error");
            throw new Error(`未知错误: ${error}`);
        }
    }
};

/**
 * 使用 Fetch API 导出聊天交互记录
 *
 * 该函数会收集聊天记录数据，将其转换为 JSON 格式，
 *
 * 创建一个 Blob 对象，然后调用 `saveFileWithFetch` 函数保存文件。
 *
 * @param {string} chatName - 聊天名称，用于生成文件名。若为空，则使用当前时间作为标识。
 *
 * @returns {Promise<boolean>} - 一个 Promise, 成功时返回 true, 失败时返回 false
 *
 * @throws {Error} - 当导出过程中出现错误时抛出错误。
 */
export async function exportChatInteractionWithFetch(chatName?: string): Promise<boolean> {
    try {
        // 检查聊天记录是否为空
        if (EntryAPI.OnlyData.historyMessage.length === 0) {
            // 若聊天记录为空，显示系统消息提示用户
            EntryAPI.showSystemMessage("聊天记录为空，无法导出", "success");
            // 导出失败，返回 false
            return false;
        };
        /**
         * 构建聊天记录数据对象，包含元数据和聊天历史记录
         */
        const chatData = {
            // 元数据，记录导出时间和版本号
            meta: {
                exportedAt: new Date().toLocaleString(),
                version: "25.1230"
            },
            // 聊天历史记录
            history: EntryAPI.OnlyData.historyMessage,
        };
        /**
         * 将聊天记录数据对象转换为格式化的 JSON 字符串
         */
        const jsonString = JSON.stringify(chatData);
        /**
         * 创建一个 MIME 类型为 application/json 的 Blob 对象
         */
        const blob = new Blob([jsonString], { type: "application/json" });
        /** 获取当前时间, 并拆分为日期和时间 */
        const currentTimeSplit = new Date().toLocaleString().split(' ');
        /** 提取当前日期, 并将其中的特殊字符替换为短横线 */
        const datePath = currentTimeSplit[0].replace(/[\/\\]/g, '-');
        /**
         * 生成当前日期的文件夹路径
         */
        const filePath = `knowledge/${datePath}/`
        /**
         * 生成文件名，包含聊天名称或当前时间
         */
        const fileName = chatName ? `${filePath}${chatName}.json` : `${filePath}${currentTimeSplit[1].replace(/[:]/g, '-')}.json`;
        // 调用 saveFileWithFetch 函数保存文件
        await saveFileWithFetch(blob, fileName, true);
        // 返回导出成功的结果
        return true;
    }
    catch (error) {
        if (!(error instanceof Error)) return false;
        // 捕获异常并显示错误信息
        EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
        // 抛出捕获的错误，以便上层调用者处理
        throw error;
    }
};