/** 文件列表项属性 */
interface FileListItem {
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
/** 列定义 */
export interface ColumnDefinition {
    /** 列名 */
    name: string;
    /** 列类型 */
    type: string;
    /** 是否为主键 */
    primary_key?: boolean;
    /** 是否自动递增 */
    auto_increment?: boolean;
    /** 是否非空 */
    not_null?: boolean;
    /** 是否唯一 */
    unique?: boolean;
    /** 默认值 */
    default?: any;
}

/** 索引定义 */
export interface IndexDefinition {
    /** 索引名称 */
    name: string;
    /** 索引列 */
    columns: string[];
    /** 是否唯一索引 */
    unique?: boolean;
}

/** 表定义 */
export interface TableDefinition {
    /** 列定义 */
    columns?: ColumnDefinition[];
    /** 索引定义 */
    indexes?: IndexDefinition[];
}

/** 数据操作 */
export interface DataOperation {
    /** 操作类型：insert, update, delete, select */
    type: 'insert' | 'update' | 'delete' | 'select';
    /** 表名 */
    table: string;
    /** 数据（insert/update） */
    data?: Record<string, any>;
    /** 过滤条件（where） */
    filter?: Record<string, any>;
    /** 限制数量 */
    limit?: number;
    /** 偏移量 */
    offset?: number;
    /** 排序规则 */
    order?: Array<Record<string, string>>;
}

/** 表操作 */
export interface TableOperation {
    /** 操作类型：create, drop, truncate */
    type: 'create' | 'drop' | 'truncate';
    /** 表名 */
    table?: string;
    /** 表定义 */
    definition?: TableDefinition;
}

/** 信息操作 */
export interface InfoOperation {
    /** 操作类型：tables, structure, count */
    type: 'tables' | 'structure' | 'count';
    /** 表名 */
    table?: string;
}

/** 单个操作结果 */
export interface OperationResult {
    /** 是否成功 */
    success: boolean;
    /** 错误信息 */
    error?: string;
    /** 操作类型 */
    operation: string;
    /** 结果行 */
    rows?: Array<Record<string, any>>;
    /** 影响的行数 */
    affected_rows?: number;
    /** 最后插入的ID */
    last_insert_id?: number;
    /** 表名 */
    table?: string;
    /** 表结构 */
    structure?: Array<Record<string, any>>;
    /** 表列表 */
    tables?: string[];
    /** 记录数 */
    count?: number;
}

/** 批量操作结果 */
export interface BatchResult {
    /** 是否成功 */
    success: boolean;
    /** 错误信息 */
    error?: string;
    /** 操作结果列表 */
    results: OperationResult[];
    /** 总耗时（毫秒） */
    total_time_ms: number;
    /** 操作数量 */
    operations: number;
}

/** 数据库操作 */
export type DatabaseOperation = DataOperation | TableOperation | InfoOperation;

/** 数据库请求 */
export interface DatabaseRequest {
    /** 操作列表 */
    operations: Array<DatabaseOperation>;
    /** 是否使用事务 */
    transaction?: boolean;
}

/**
 * 在磁盘中保存文件
 * 
 * @param {string} fileName 文件名
 * 
 * @param {boolean} overwrite 是否覆盖已存在的文件
 * 
 * @param {*} fileData 文件数据
 * 
 * @returns {[string, string, Error | null]} 包含保存结果的元组，[保存的文件名, 保存的文件路径, 错误信息]
 * */
export declare function SaveFile(fileName: string, overwrite: boolean, fileData: Blob | File | FormData | string): [string, string, Error | null];

/**
 * 从磁盘中读取文件
 * 
 * @param {string} filePath 文件路径
 * 
 * @returns {[Blob, number, string, Error | null]} 包含读取结果的元组，[文件内容, 文件大小, MIME类型, 错误信息]
 * */
export declare function ReadFile(filePath: string): [Blob, number, string, Error | null];

/**
 * 获取指定目录下的所有文件列表
 * 
 * @param {string} path 目录路径
 * 
 * @returns {[FileListItem[], Error | null]} 包含文件列表的元组，[文件列表, 错误信息]
 * */
export declare function GetFileList(path: string): [FileListItem[], Error | null];

/**
 * 执行数据库请求
 * 
 * @param {DatabaseRequest} request 数据库请求对象
 * 
 * @returns {[BatchResult, Error | null]} 包含数据库操作结果的元组，[操作结果, 错误信息]
 * */
export declare function ExecuteDatabaseRequest(request: DatabaseRequest): [BatchResult, Error | null];

/**
 * 获取当前地址信息
 * 
 * @returns {[string[], Error | null]} 包含地址信息的元组，[省份, 城市, 错误信息]
 * */
export declare function QueryCurrentAddress(): [string[], Error | null];

/**
 * 处理OpenAI API请求的代理
 * 
 * @param {string} url API URL
 * 
 * @param {any} requestBody 请求体
 * 
 * @param {any} headers 请求头
 * 
 * @returns {[any, Error | null]} 包含响应结果的元组，[响应结果, 错误信息]
 * */
export declare function AgentProxy(url: string, requestBody: any, headers: any): [any, Error | null];

/**
 * 关键帧结构
 */
export interface KeyFrame {
    /** 关键帧文件名 */
    filePath: string;
    /** 关键帧时间戳 */
    timestamp: string;
    /** 关键帧编号 */
    frameNum: number;
    /** 关键帧图像数据 */
    data: string;
}

/**
 * 提取视频关键帧
 * 
 * @param {string} inputFile 视频文件路径
 * 
 * @param {string} cacheDir 缓存目录
 * 
 * @returns {[KeyFrame[], Error | null]} 包含关键帧列表的元组，[关键帧列表, 错误信息]
 * */
export declare function ExtractKeyFramesWithLocalCache(inputFile: string, cacheDir: string): [KeyFrame[], Error | null];
