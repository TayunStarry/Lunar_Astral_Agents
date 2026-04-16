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
    tools?: any[];
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
