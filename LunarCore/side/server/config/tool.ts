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
    arguments?: Record<string, any> | string;
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
    [key: string]: any;
}
