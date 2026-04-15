import { ToolCall } from '../index';

/** 全局系统配置项 */
export interface Config {
    /** 文本嵌入服务接口地址 */
    embeddingModelUrl?: string;
    /** 文本嵌入模型名称 */
    embeddingModelName?: string;
    /** 文本嵌入服务 API 密钥 */
    embeddingModelKey?: string;

    /** 视觉理解服务接口地址 */
    multimodalModelUrl?: string;
    /** 视觉理解模型名称 */
    multimodalModelName?: string;
    /** 视觉理解服务 API 密钥 */
    multimodalModelKey?: string;

    /** 用户名 */
    userName?: string;
}

/** 鉴权请求头 - 用于 API 认证 */
export interface AuthHeaders {
    /** Bearer Token 授权头，格式: `Bearer ${token}` */
    Authorization: `Bearer ${string}`;
    /** 内容类型，固定为 JSON */
    'Content-Type': 'application/json';
}

/** 聊天缓存接口 */
export interface ChatCache {
    /** 累积所有工具调用的数组 */
    toolCalls: ToolCall[];
    /** 当前正在累积的工具调用对象，可能为 null */
    currentToolCall: ToolCall | null;
    /** 当前工具调用在流中的索引，用于识别是否属于同一次调用 */
    currentToolCallIndex: number;
    /** 累积当前工具调用的参数字符串 */
    currentFunctionArgs: string;
    /** 累积当前工具调用的函数名 */
    currentFunctionName: string;
    /** 提取思考内容的字符串 */
    thinkingContent: string;
    /** 提取描述内容的字符串 */
    descriptionContent: string;
}

/** 文件列表项属性 */
export interface FileListItem {
    /** 文件或目录名称 */
    name: string;
    /** 文件大小（字节） */
    size: number;
    /** 是否为目录 */
    isDir: boolean;
    /** 最后修改时间，格式：YYYY-MM-DD HH:mm:ss */
    lastModified: string;
    /** 文件的完整路径 */
    path: string;
}

/** 状态显示类型 */
export type ShowStatusType = 'success' | 'error';

/** 任务状态属性 */
export interface TaskStatus {
    /** 任务当前状态 */
    status: 'completed' | 'failed' | 'running';
    /** 任务唯一标识符 */
    task_id: string;
    /** 错误信息（仅当任务失败时提供） */
    error?: string;
}

/** 思考标签类型 */
export const ThinkType = [
    /<think>([\s\S]*?)<\/think>([\s\S]*)/,
    /<\|thought_start\|>([\s\S]*?)<\|thought_end\|>([\s\S]*)/,
];