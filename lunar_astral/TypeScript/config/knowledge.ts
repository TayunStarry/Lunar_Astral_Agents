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