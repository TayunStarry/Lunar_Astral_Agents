/**
 * AI 智能体多模态交互协议 - 类型定义
 * 提供完整的模型推理、工具调用、消息管理和系统配置类型
 */

// ==================== 核心协议 ====================

/** 多模态协议请求体 - 标准化的模型调用接口 */
export interface MultimodalProtocol {
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

// ==================== 模型推理相关 ====================

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
    tool_choice?: ToolChoice;
}

/** 模型消息角色类型 */
export type PostMessageRole = 'user' | 'assistant' | 'system' | 'tool';

/** 模型推理请求的消息对象 */
export type PostMessage = TextMessage | MultimodalMessage | ContinuousMultimodalMessage | ToolResponseMessage;

/** 纯文本消息 */
export interface TextMessage {
    /** 消息角色 */
    role: PostMessageRole;
    /** 消息文本内容 */
    content: string;
}

/** 图文混合消息（支持多模态输入） */
export interface MultimodalMessage {
    /** 消息角色（通常为 'user'） */
    role: PostMessageRole;
    /** 混合内容数组，可包含图片和文本 */
    content: [ImageContent, TextContent];
}

/** 连续图文混合消息（支持多模态输入） */
export interface ContinuousMultimodalMessage {
    /** 消息角色（通常为 'user'） */
    role: PostMessageRole;
    /** 混合内容数组，可包含图片和文本 */
    content: Array<ImageContent | TextContent>;
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

// ==================== 工具调用系统 ====================

/**
 * JSON Schema 参数类型定义
 * 用于工具函数参数的验证和描述
 */
export interface JSONSchema {
    /** 参数的数据类型 */
    type: 'string' | 'number' | 'integer' | 'boolean' | 'object' | 'array' | 'null';
    /** 参数的功能描述，用于指导模型理解参数用途 */
    description?: string;
    /** 允许的枚举值列表，限制参数只能取这些值 */
    enum?: any[];
    /** 对象的属性定义（仅当 type='object' 时有效） */
    properties?: Record<string, JSONSchema>;
    /** 必须提供的属性名称数组 */
    required?: string[];
    /** 数组项的类型定义（仅当 type='array' 时有效） */
    items?: JSONSchema;
    /** 参数的默认值 */
    default?: any;
    /** 数值的最小值 */
    minimum?: number;
    /** 数值的最大值 */
    maximum?: number;
    /** 字符串的最小长度 */
    minLength?: number;
    /** 字符串的最大长度 */
    maxLength?: number;
    /** 字符串必须匹配的正则表达式模式 */
    pattern?: string;
    /** 数据格式，如 'date-time', 'email' 等 */
    format?: string;
    /**
     * 是否允许未在 properties 中定义的额外属性
     * false: 严格模式，禁止额外属性
     * true 或 JSONSchema: 允许额外属性，并可指定其类型
     */
    additionalProperties?: boolean | JSONSchema;
}

/** 函数工具定义 */
export interface FunctionTool {
    /** 工具函数名称，用于模型识别和调用 */
    name: string;
    /**
     * 工具功能描述，模型据此决定是否调用此工具
     * 应清晰说明工具的用途和使用场景
     */
    description?: string;
    /** 函数参数定义，使用 JSON Schema 格式 */
    parameters?: JSONSchema;
    /**
     * 严格模式标志（注意：OpenAI API 实际忽略此字段）
     * 如需要严格校验，请在 parameters 中使用 additionalProperties: false
     */
    strict?: boolean;
    /** 预定义的参数值，用于工具调用时的默认参数 */
    arguments?: Record<string, any>;
}

/** 单个工具调用定义 */
export interface ToolCall {
    /** 工具类型，目前仅支持 'function' */
    type: 'function';
    /** 函数工具的具体定义 */
    function: FunctionTool;
    /**
     * 工具调用唯一标识符
     * 用于关联工具调用请求和返回结果
     */
    id?: string;
}

/** 工具调用选择策略 */
export type ToolChoice =
    /** 禁用工具调用，模型只能生成文本回复 */
    | 'none'
    /** 模型自主决定是否调用工具 */
    | 'auto'
    /** 强制模型必须调用至少一个工具 */
    | 'required';

/** 工具调用响应消息 */
export interface ToolResponseMessage {
    /** 消息角色，固定为 'tool' */
    role: 'tool';
    /** 工具执行结果 */
    content: string;
    /** 关联的工具调用 ID，必须与请求中的 ToolCall.id 对应 */
    tool_call_id: string;
}

/** 附件接口 */
export interface Attachment {
    /** 附件来源信息 */
    image_url: string;
}

/**
 * 流式处理状态接口
 */
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
    /** 独立推理内容的字符串累积 */
    reasoningContent: string;
    /** 提取思考内容的字符串累积 */
    thinkingContent: string;
    /** 提取描述内容的字符串累积 */
    descriptionContent: string;
}

// ==================== 历史消息管理 ====================

/** 历史消息（单条） - 用于对话历史记录 */
export interface HistoryMessage {
    /** 消息角色 */
    role: PostMessageRole;
    /** 消息正文内容 */
    content: string;
    /** 是否为系统提示消息 */
    isPrompt: boolean;
    /** 是否在 UI 界面中跳过渲染 */
    noRender: boolean;
    /** 附带图片的 URL 地址，无则为 null */
    imageUrl: string | null;
    /** 消息是否可被用户删除 */
    deletable: boolean | null;
    /** 消息的唯一标识符 */
    uuid: string;
    /** 消息内容的嵌入向量，用于语义检索 */
    embedVector: number[];
}

/** 知识库消息（单条） - 用于知识库检索 */
export interface KnowledgeMessage {
    /** 消息角色 */
    role: PostMessageRole;
    /** 消息正文内容 */
    content: string;
    /** 附带图片的 URL 地址，无则为 null */
    imageUrl: string | null;
    /** 消息的唯一标识符 */
    uuid: string;
}

/** 混合消息类型 - 可用于统一处理历史消息和知识库消息 */
export type MixedMessage = KnowledgeMessage | HistoryMessage;

/** 加权历史消息 - 用于带有权重的历史消息检索 */
export interface WeightedHistoryMessage {
    /** 历史消息对象 */
    message: HistoryMessage;
    /** 消息的权重值，影响检索优先级 */
    weight: number;
}

/** 历史会话导出文档结构 */
export interface HistoryDocument {
    /** 文档元信息 */
    meta: {
        /** 导出时间，格式：YYYY.MM.DD-HH:mm:ss */
        exportedAt: string;
        /** 文档版本号 */
        version: string;
    };
    /** 历史消息数组 */
    history: HistoryMessage[];
}

// ==================== 系统配置 ====================

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

// ==================== 辅助类型 ====================

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

/** 工具调用参数示例类型 */
export interface ToolCallParameters {
    /** 工具调用的原因描述 */
    reason?: string;
    /** 查询类型 */
    query_type?: 'weather' | 'news';
    /** 省份名称 */
    sheng?: string;
    /** 城市名称 */
    place?: string;
    /** 正向提示文本 */
    prompt?: string;
    /** 负向提示文本 */
    negative_prompt?: string;
    /** 详细描述所需内容 */
    description?: string;
    /** 是否使用上一次生成的图片作为参考 */
    use_reference?: boolean;
    /** 参考图片的强度 */
    strength?: number;
    /** 控制提示词权重 */
    cfg_scale?: number;
    /** 索引值 */
    index?: number;
    /** 其他自定义参数 */
    [key: string]: string | number | boolean;
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