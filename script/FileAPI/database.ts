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
 * 向数据库查询数据
 *
 * @param {DatabaseOperation[]} operations - 数据库操作列表
 *
 * @param {DatabaseOperation} [createTableOperation] - 表不存在时创建表的操作
 *
 * @returns {Promise<BatchResult>} - 数据库查询结果
 */
export async function queryFromDatabase(operations: DatabaseOperation[], createTableOperation: DatabaseOperation): Promise<BatchResult> {
    /** 构建数据库查询请求体 */
    const requestBody: DatabaseRequest = {
        operations,
        transaction: false
    };
    /** 构建数据库查询请求 */
    const buildRequest: RequestInit = {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(requestBody)
    };
    /** 发送数据库查询请求 */
    let response = await fetch('/database/', buildRequest);
    // 检查响应状态是否成功
    if (!response.ok) throw new Error('数据库查询失败');
    /** 解析数据库查询响应 */
    let result: BatchResult = await response.json();
    // 检查查询结果是否有效
    if (!result.success || !result.results[0].success) {
        /** 提取错误信息 */
        const errorMessage = result.error || result.results[0].error || '';
        // 检查是否是因为表不存在的错误，并且提供了创建表的操作
        if (errorMessage.includes('no such table') && createTableOperation) {
            /** 构建创建表请求体 */
            const createTableRequest: DatabaseRequest = {
                operations: [createTableOperation],
                transaction: false
            };
            /** 构建创建表请求 */
            const createTableBuildRequest: RequestInit = {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(createTableRequest)
            };
            /** 发送创建表请求 */
            const createTableResponse = await fetch('/database/', createTableBuildRequest);
            // 检查创建表响应状态是否成功
            if (!createTableResponse.ok) throw new Error('创建表失败');
            /** 解析创建表响应 */
            const createTableResult: BatchResult = await createTableResponse.json();
            /** 检查创建表操作是否成功 */
            if (!createTableResult.success) throw new Error('创建表失败');
            // 重新执行原始查询操作
            response = await fetch('/database/', buildRequest);
            // 检查重新执行查询响应状态是否成功
            if (!response.ok) throw new Error('数据库查询失败');
            // 解析重新执行查询响应
            result = await response.json();
            // 检查重新执行查询操作是否成功
            if (!result.success || !result.results[0].success) throw new Error('数据库查询失败');
        }
        else throw new Error('数据库查询失败');
    }
    // 返回查询结果
    return result;
}
