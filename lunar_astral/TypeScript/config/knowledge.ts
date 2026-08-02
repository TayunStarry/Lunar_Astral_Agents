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

/** 知识库操作 */
export type KnowledgeOperation = DataOperation | TableOperation | InfoOperation;

/** 知识库请求 */
export interface KnowledgeRequest {
	/** 操作列表 */
	operations: Array<KnowledgeOperation>;
	/** 是否使用事务 */
	transaction?: boolean;
}
