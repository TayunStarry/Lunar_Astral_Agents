import * as EntryAPI from '../EntryAPI/code';

/**
 * 异步从指定 URL 获取 Markdown 文件内容，并对内容进行格式化处理
 *
 * 支持控制是否移除换行符，最终会将多个连续空白字符替换为单个空格
 *
 * @param {string} url - Markdown 文件的 URL 地址
 *
 * @param {boolean} [removeNewLines=false] - 是否剔除换行符，默认不剔除
 *
 * @returns {Promise<string>} - 解析并格式化后的 Markdown 文件内容
 *
 * @throws {Error} - 当网络请求失败时抛出错误
 */
export async function fetchMarkdown(url: RequestInfo | URL, removeNewLines: boolean = false): Promise<string> {
    /**
     * 发送网络请求，获取指定 URL 的 Markdown 文件内容
     */
    const response = await fetch(url);
    // 检查响应状态，判断请求是否成功
    if (response.ok) {
        /**
         * 从响应中获取 Markdown 文件的字符串属性
         */
        const markdown = await response.text();
        // 初始化处理后的 Markdown 内容
        let processedMarkdown = markdown;
        // 根据参数决定是否移除换行符
        if (removeNewLines) processedMarkdown = processedMarkdown.replace(/[\r\n]+/g, '');
        // 将多个连续的空格或制表符替换为单个空格，并返回处理结果
        return processedMarkdown.replace(/[ \t]+/g, ' ');
    }
    else {
        // 请求失败时，显示系统状态提示，告知用户 Markdown 文件加载失败的原因
        EntryAPI.showSystemMessage('markdown文件 加载失败: ' + response.statusText, "error");
        // 返回空字符串，避免后续处理错误
        return '';
    }
};

/**
 * 异步函数，用于将图片文件保存到服务器，使用内容哈希作为文件名
 *
 * @param {File} file - 需要保存的图片文件对象
 *
 * @returns {Promise<string>} - 保存成功后返回图片的读取路径，失败则抛出错误
 */
export async function saveImageToServer(file: File): Promise<string> {
    try {
        /** 计算文件的SHA-256哈希值（取前16个字符，保持较短长度） */
        const fileHash = await calculateFileHash(file);
        /** 获取文件扩展名 */
        const fileExtension = file.name.slice(file.name.lastIndexOf('.')).toLowerCase();
        /** 使用哈希值 + 扩展名作为新文件名 */
        const newFileName = `${fileHash}${fileExtension}`;
        /** 将包含图片文件名的路径进行 Base64 编码，用于设置请求头中的文件名 */
        const base64FileName = EntryAPI.toBtoaString('images/' + newFileName);
        /** 向服务器发送 POST 请求，尝试保存图片文件 */
        const response = await fetch('/save', { method: 'POST', headers: { 'X-File-Name': base64FileName, 'X-Overwrite': 'true' }, body: file });
        // 检查响应是否成功，若失败则抛出错误
        if (!response.ok) throw new Error('图片保存失败');
        // 保存成功，返回图片的读取路径
        return `/read/images/${newFileName}`;
    }
    catch (error) {
        if (!(error instanceof Error)) return '';
        // 捕获异常并显示错误信息
        EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
        // 保存失败，返回空字符串
        return '';
    }
};

/**
 * 异步函数，用于计算文件的SHA-256哈希值，并截取前16个字符
 *
 * @param {File} file - 文件对象
 *
 * @returns {Promise<string>} - 16字符的十六进制哈希值
 */
async function calculateFileHash(file: File): Promise<string> {
    /** 定义处理文件读取的异步函数 */
    function process(resolve: (value: string | PromiseLike<string>) => void) {
        /** 创建FileReader实例，用于读取文件内容 */
        const reader = new FileReader();
        // 为FileReader的onload事件添加回调函数，文件读取成功时触发
        reader.onload = async function (e) {
            try {
                /** 从FileReader事件对象中获取文件的ArrayBuffer数据 */
                const arrayBuffer = e.target?.result as ArrayBuffer;
                /** 使用crypto.subtle.digest方法计算ArrayBuffer的SHA-256哈希值 */
                const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
                /** 将哈希结果的ArrayBuffer转换为Uint8Array数组 */
                const hashArray = Array.from(new Uint8Array(hashBuffer));
                /** 将Uint8Array数组中的每个字节转换为两位的十六进制字符串，并拼接成完整的哈希字符串 */
                const fullHash = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
                /** 截取完整哈希字符串的前16个字符 */
                const shortHash = fullHash.substring(0, 16);
                // 将截取后的短哈希值作为Promise的成功结果返回
                resolve(shortHash);
            }
            catch {
                // 返回文件名的 Base64 编码
                resolve(EntryAPI.toBtoaString(file.name).slice(-16));
            }
        };
        // 为FileReader的onerror事件添加回调函数，文件读取失败时触发
        reader.onerror = async (error) => {
            if (!(error instanceof Error)) return;
            // 显示文件读取失败的系统消息
            EntryAPI.showSystemMessage(`${error.name} | ${error.message} | ${error.stack}`, "error");
        };
        // 以ArrayBuffer格式读取文件内容
        reader.readAsArrayBuffer(file);
    };
    // 返回一个Promise，用于处理异步操作
    return new Promise(process);
};

/**
 * 异步尝试获取文件内容并调用回调函数处理
 *
 * @param {RequestInfo | URL} url - 文件的 URL 地址
 *
 * @param {string} [initializeContent='{}'] - 初始化内容，默认值为空 JSON 字符串
 *
 * @param {(content: string) => any} [callback] - 处理文件内容的回调函数，可选，默认使用 JSON.parse
 *
 * @returns {Promise<any>} - 回调函数处理后的结果
 *
 * @throws {Error} - 当获取 文件内容失败或回调函数处理出错时抛出错误
 */
export async function fetchDocumentCallback(url: RequestInfo | URL, initializeContent: string = '{}', callback?: (content: string) => any): Promise<any> {
    /** 默认回调函数：尝试将文本解析为 JSON */
    const defaultCallback = (content: string) => JSON.parse(content);
    /** 应用回调函数，默认使用默认回调 */
    const applyCallback = callback ?? defaultCallback;
    /** 统一兜底逻辑：当文件不存在或读取失败时，保存默认内容并返回 */
    const fallback = async () => {
        await EntryAPI.saveFileWithFetch(initializeContent, url.toString(), true);
        return applyCallback(initializeContent);
    };
    try {
        /** 拆分文件路径 */
        const filePath = url.toString().split(/[\/\\]/);
        /** 获取文件列表 */
        const listRes = await fetch('/file_list/' + filePath.slice(0, -1).join('/'));
        // 检查文件列表响应是否成功
        if (!listRes.ok) return await fallback();
        /** 解析文件列表 JSON 数据 */
        const fileList = await listRes.json() as EntryAPI.FileListItem[];
        /** 检查文件是否存在且不是目录 */
        const exists = fileList.some(item => item.name === filePath[filePath.length - 1] && !item.isDir);
        // 检查文件是否存在
        if (!exists) return await fallback();
        /** 读取文件内容 */
        const contentRes = await fetch(`/read/${url.toString()}`);
        // 检查文件内容响应是否成功
        if (!contentRes.ok) return await fallback();
        /** 解析文件内容为文本 */
        const text = await contentRes.text();
        // 检查文件内容是否为空
        if (!text) return await fallback();
        // 执行回调函数处理文件内容
        return applyCallback(text);
    }
    // 任何异常都走兜底逻辑
    catch (error) {
        if (error instanceof Error) EntryAPI.showSystemMessage('文件处理异常: ' + error.message, "error");
        return await fallback();
    }
};

/**
 * 工具定义提取与注册器
 * @param {string} markdownContent - 包含工具定义的 Markdown 文本
 * @returns {Object} 提取结果
 */
export function registerToolFromMarkdown(markdownContent: string): { success: boolean, message: string } {
    let actualToolName = '智能体工具'
    try {
        const jsonRegex = /```json\s*([\s\S]*?)\s*```/;
        const jsonMatch = markdownContent.match(jsonRegex);
        if (!jsonMatch) return { success: false, message: '未找到 JSON 工具定义' };
        const jsonContent = jsonMatch[1].trim();
        const toolDefinition = JSON.parse(jsonContent) as EntryAPI.ToolCall;
        // 3. 验证工具定义结构
        if (!toolDefinition.type || toolDefinition.type !== 'function') return { success: false, message: '工具定义类型必须为 function' };
        if (!toolDefinition.function || !toolDefinition.function.name) return { success: false, message: '工具定义必须包含函数名称' };
        actualToolName = toolDefinition.function.name;
        // 4. 检查是否已存在同名工具
        const existingIndex = EntryAPI.OnlyData.toolCall.findIndex((tool: EntryAPI.ToolCall) => tool.function && tool.function.name === actualToolName);
        // 更新现有工具
        if (existingIndex >= 0) EntryAPI.OnlyData.toolCall[existingIndex] = toolDefinition;
        // 添加新工具
        else EntryAPI.OnlyData.toolCall.push(toolDefinition);
        // 5. 提取 JavaScript 实现
        const codeRegex = /```(javascript)\s*([\s\S]*?)\s*```/;
        const projectCode = markdownContent.match(codeRegex)[2].trim();
        if (projectCode) {
            const script = document.createElement('script');
            script.type = "module";
            script.textContent = projectCode;
            document.head.appendChild(script);
        }
        else return { success: false, message: '未找到 JavaScript 实现代码' };
    }
    catch (error) {
        return { success: false, message: `工具 "${actualToolName}" 注册失败: ${error}` };
    }
    return { success: true, message: `工具 "${actualToolName}" 已成功注册` };
}

/**
 * 加载全部月华协议工具包
 *
 * @returns {Promise<{ success: boolean, message: string }>} 注册结果
 */
export async function EnableLunarToolPackageProtocol(): Promise<{ success: boolean, message: string }> {
    /** 获取文件列表 */
    const listRes = await fetch('/file_list/resources/package');
    // 检查文件列表响应是否成功
    if (!listRes.ok) return { success: false, message: `获取工具文件列表失败: ${listRes.status}` };
    /** 解析文件列表 JSON 数据 */
    const fileList = await listRes.json() as EntryAPI.FileListItem[];
    /** 过滤出工具文件 */
    const toolFiles = fileList.filter(item => item.name.endsWith('.ltp.md') && !item.isDir);
    /** 批量注册工具 */
    toolFiles.forEach(file => fetchMarkdown(`/read/resources/package/${file.name}`).then(content => registerToolFromMarkdown(content)));
    return { success: true, message: `已成功注册 ${toolFiles.length} 个工具` };
};