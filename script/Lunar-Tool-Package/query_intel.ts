import { ToolCallParameters, subscriptionToolCall, HistoryMessage, addImageRendering, createImageMessage, ToolCall } from '../EntryAPI/code';

// 注册工具函数
subscriptionToolCall("query_intel",
	async (args: ToolCallParameters, _: HTMLElement, messageObject: HistoryMessage) => {
		// 根据查询类型执行不同的操作
		switch (args.query_type) {
			case "weather": return await getWeather(args.sheng!, args.place!);

			case "news": return await getNews(messageObject);

			default: return `不支持的查询类型: ${args.query_type}`;
		}
	}
);

/** 获取天气信息 */
async function getWeather(sheng: string, place: string): Promise<any> {
	// 验证省和市参数是否存在
	if (!sheng || !place) return '天气查询需要提供省和市';
	/** 随机选择一个URL */
	const selectedUrl = `https://cn.apihz.cn/api/tianqi/tqyb.php?id=88888888&key=88888888&sheng=${sheng}&place=${place}`;
	/** 发送GET请求 */
	const response = await fetch(selectedUrl);
	// 检查响应状态
	if (!response.ok) return `天气查询API返回错误状态: ${response.status}`;
	// 返回JSON响应
	return await response.text();
}

/** 获取新闻信息并显示图片 */
async function getNews(messageObject: HistoryMessage): Promise<string> {
	/** 新闻查询API地址 */
	const url = "https://60s.7se.cn/v2/60s";
	/** 发送GET请求 */
	const response = await fetch(url);
	// 检查响应状态
	if (!response.ok) return `新闻查询API返回错误状态: ${response.status}`;
	/** 解析响应为JSON格式 */
	const decode = await response.json();
	/** 创建图片消息对象 */
	const imageMessage = createImageMessage('assistant', '包含新闻内容的图片', decode.data.image);
	// 添加图片渲染到消息元素
	addImageRendering(imageMessage);
	// 存储图片URL到消息对象, 用于后续引用
	messageObject.imageUrl = decode.data.image;
	// 返回新闻内容
	return decode.data.news.join('\n');
}
