import { ToolCall } from "./tool";

/** 模型协议请求体 */
export interface ModelProtocol {
    /** HTTP 方法，固定为 POST */
    method: 'POST';
    /** 是否允许跨域请求 */
    crossDomain: boolean;
    /** 认证和内容类型头部 */
    headers: AuthHeaders;
    /**
     * 推理请求负载（JSON 字符串格式）
     * 包含模型参数、消息、工具定义等
     */
    body: string;
    /** 可选的请求中止信号，用于取消请求 */
    signal?: AbortSignal;
}

/** 鉴权请求头 - 用于 API 认证 */
export interface AuthHeaders {
    /** Bearer Token 授权头，格式: `Bearer ${token}` */
    Authorization: `Bearer ${string}`;
    /** 内容类型，固定为 JSON */
    'Content-Type': 'application/json';
}

/** 模型推理请求负载 */
export interface InferencePayload {
    /** 要使用的模型名称 */
    model: string;
    /**
     * 对话消息列表（兼容旧版字段）
     * @deprecated 建议使用 `messages` 字段
     */
    messages?: (PostMessage | string)[];
    /** 对话消息列表（标准字段） */
    input?: (PostMessage | string)[];
    /** 是否启用流式响应 */
    stream: boolean;
    /** 可用的工具定义数组，最多支持 128 个工具 */
    tools?: ToolCall[];
    /** 工具调用选择策略 */
    tool_choice?: string;
}

/** 嵌入向量响应结构 */
export interface EmbeddingResponse {
    /** 响应数据数组 */
    data: Array<{
        /** 文本的嵌入向量 */
        embedding: number[];
    }>;
}

/** 模型消息角色类型 */
export type PostMessageRole = 'user' | 'assistant' | 'system' | 'tool';

/** 模型推理请求的消息对象 */
export type PostMessage = TextMessage | MultimodalMessage | ToolResponseMessage;

/** 纯文本消息 */
export interface TextMessage {
    /** 消息角色 */
    role: PostMessageRole;
    /** 消息文本内容 */
    content: string;
}

/** 多媒体混合消息） */
export interface MultimodalMessage {
    /** 消息角色（通常为 'user'） */
    role: PostMessageRole;
    /** 混合内容数组，可包含图片和文本 */
    content: Array<ImageContent | TextContent>;
}

/** 工具调用响应消息 */
export interface ToolResponseMessage {
    /** 消息角色，固定为 'tool' */
    role: 'tool';
    /** 工具执行结果 */
    content: string;
    /** 关联的工具调用 ID，必须与请求中的 ToolCall.id 对应 */
    tool_call_id: string;
}

/** 图片内容块 */
export interface ImageContent {
    /** 内容类型，固定为 'image_url' */
    type: 'image_url';
    /** 图片 URL 信息 */
    image_url: {
        /** 图片的 URL 地址 */
        url: string;
    };
}

/** 文本内容块 */
export interface TextContent {
    /** 内容类型，固定为 'text' */
    type: 'text';
    /** 文本内容 */
    text: string;
}
/**
 * 模型对话完成响应结构
 * 对应 OpenAI Chat Completions API 响应格式
 */
export interface modelResponse {
    /** HTTP 响应头信息 */
    headers: AuthHeaders;
    /** 响应主体内容 */
    body: ModelResponseBody;
    /** HTTP 状态码（如 200 表示成功） */
    status: number;
}

/** Token 使用统计 */
export interface TokenUsage {
    /** 生成的 completion tokens 数量 */
    completion_tokens: number;
    /** 输入的 prompt tokens 数量 */
    prompt_tokens: number;
    /** 总共使用的 tokens 数量 */
    total_tokens: number;
}

/** 各阶段耗时性能指标 */
export interface Timings {
    /** 缓存命中的 token 数量 */
    cache_n: number;
    /** Prompt 处理速度（tokens/秒） */
    prompt_per_second: number;
    /** 生成内容的总耗时（毫秒） */
    predicted_ms: number;
    /** 每个 token 的平均生成耗时（毫秒） */
    predicted_per_token_ms: number;
    /** 内容生成速度（tokens/秒） */
    predicted_per_second: number;
    /** Prompt 处理的 token 数量 */
    prompt_n: number;
    /** Prompt 处理总耗时（毫秒） */
    prompt_ms: number;
    /** 每个 prompt token 的平均处理耗时（毫秒） */
    prompt_per_token_ms: number;
    /** 生成的 token 数量 */
    predicted_n: number;
}

/** 工具调用函数信息 */
export interface ToolCallFunction {
    /** 工具函数名称 */
    name: string;
    /** 工具函数的参数（JSON 字符串格式） */
    arguments: string;
}

/** 工具调用项 */
export interface ToolCallItem {
    /** 工具调用类型，固定为 "function" */
    type: "function";
    /** 工具调用唯一 ID */
    id: string;
    /** 被调用的工具函数信息 */
    function: ToolCallFunction;
}

/** 助手回复消息 */
export interface AssistantMessage {
    /** 消息角色，固定为 "assistant" */
    role: 'assistant';
    /** 消息文本内容 */
    content: string;
    /** 推理内容（部分模型支持，如 Qwen3-VL） */
    reasoning_content?: string;
    /** 工具调用列表（当模型需要调用工具时出现） */
    tool_calls?: ToolCallItem[];
}

/** 对话完成选项 */
export interface Choice {
    /** 停止原因：stop=正常停止, length=达到最大长度, content_filter=内容过滤等 */
    finish_reason: string;
    /** 选项索引 */
    index: number;
    /** 助手回复消息 */
    message: AssistantMessage;
}

/** 模型响应主体 */
export interface ModelResponseBody {
    /** 实际使用的模型名称 */
    model: string;
    /** 系统指纹，用于标识模型配置变更 */
    system_fingerprint: string;
    /** 响应对象类型，固定为 "chat.completion" */
    object: 'chat.completion';
    /** Token 使用统计 */
    usage: TokenUsage;
    /** 本次响应的唯一标识 ID */
    id: string;
    /** 各阶段耗时性能指标 */
    timings: Timings;
    /** 对话完成选项列表 */
    choices: Choice[];
    /** 响应创建时间的 Unix 时间戳 */
    created: number;
}