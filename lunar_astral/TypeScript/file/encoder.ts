/**
 * 将输入参数转换为 Base64 编码字符串
 *
 * 此函数会先对输入参数进行 URI 编码，然后将编码后的十六进制字符转换为对应的字符，最后进行 Base64 编码
 *
 * @param {string} params - 需要转换的输入参数
 * @returns {string} Base64 编码后的字符串
 */
export function toBtoaString(params: string): string {
	/**
	 * 对输入参数进行 URI 编码，确保特殊字符被正确处理
	 */
	const encodedParams = encodeURIComponent(params);
	/**
	 * 将 URI 编码后的十六进制字符转换为对应的字符
	 */
	const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
	// 对转换后的字符进行 Base64 编码并返回
	return btoa(decodedParams);
};

/**
 * 将 File 或 Blob 对象转换为 Base64 编码字符串
 *
 * 内部使用 FileReader 以 DataURL 方式读取文件内容，
 * 成功时返回完整的 data:[<mediatype>];base64, 前缀 + 编码字符串，
 * 失败时返回 rejected Promise 并携带具体错误信息。
 *
 * @param file - 需要转换的文件或二进制数据
 *
 * @returns {Promise<string>}  Base64 字符串（含 MIME 类型前缀）
 *
 * @throws {Error} 读取或转换失败时抛出
 */
export async function FileToBase64(file: File | Blob): Promise<string> {
	return new Promise(
		(resolve, reject) => {
			/** 创建 FileReader 实例，用于读取文件内容 */
			const reader = new FileReader();
			// 读取完成：将结果直接作为 Base64 字符串返回
			reader.onload = function (event) {
				/** 从事件目标中提取 Base64 编码字符串 */
				const base64String = event.target?.result as string;
				// 检查 Base64 字符串是否为空
				if (!base64String) throw new Error("文件转 Base64 失败: 空字符串");
				// 返回 Base64 字符串
				resolve(base64String);
			};
			// 读取异常：构造明确错误信息并拒绝 Promise
			reader.onerror = function (error) {
				reject(new Error(`文件转 Base64 失败: ${(error.target as FileReader).error?.code}`));
			};
			// 启动读取：以 DataURL 形式读取文件内容
			reader.readAsDataURL(file);
		}
	);
};

/**
 * 异步函数，用于计算文件的SHA-256哈希值，并截取前16个字符
 *
 * @param {File} file - 文件对象
 *
 * @returns {Promise<string>} - 16字符的十六进制哈希值
 */
export async function calculateFileHash(file: File): Promise<string> {
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
				resolve(toBtoaString(file.name).slice(-16));
			}
		};
		// 为FileReader的onerror事件添加回调函数，文件读取失败时触发
		reader.onerror = async (error) => {
			if (!(error instanceof Error)) return;
			// 显示文件读取失败的系统消息
			resolve(`${error.name} | ${error.message} | ${error.stack}`);
		};
		// 以ArrayBuffer格式读取文件内容
		reader.readAsArrayBuffer(file);
	};
	// 返回一个Promise，用于处理异步操作
	return new Promise(process);
};