/**
 * 星月智能 · 桌面端 / 移动端共享常量
 *
 * 这里只放两端语义完全一致的值：接口地址、存储键、固定文案。
 * 带端差异的常量（重连次数、超时、页面尺寸等）留在各端自己的配置里。
 */

/** 浏览器缓存缺失且尚未输入称呼时的兜底用户名 */
export const DEFAULT_USER_NAME = '子幽';

/** 用户称呼的浏览器存储键（桌面端与移动端共用同一个键） */
export const USER_NAME_STORAGE_KEY = 'lunar_user_name';

/** 后端接口 */
export const ENDPOINTS = {
	/** 发送消息（OpenAI 兼容消息数组） */
	sendMessage: '/write/message',
	/** 写入文件（X-File-Name 头传路径） */
	writeFile: '/file/write',
	/** 按句柄解析资源文件名 */
	resolveResource: '/file/resource/resolve',
	/** 通用配置（角色渲染器） */
	generalConfig: '/file/read/config/general.json',
	/** 共享视频平台域名表（webpage 信封声明 web_video 的唯一来源） */
	videoPlatformHosts: '/media/video-platform-hosts'
};
