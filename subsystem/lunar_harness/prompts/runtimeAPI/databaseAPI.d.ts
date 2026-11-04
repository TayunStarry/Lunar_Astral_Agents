// ═══════════════════════════════════════════════════════════
// SQLite 数据库（database.*）
//
// 统一使用单一固定库 knowledge.db（local_data/database/），不提供新建库文件能力。
// query / exec / transaction / migrate 均作用于该库。
// ═══════════════════════════════════════════════════════════

/** 数据库事务作用域对象（transaction 回调入参，操作落在当前事务内） */
interface DatabaseTx {
	/** 事务内 SELECT，返回 { success, rows } */
	query(sql: string, params?: unknown[]): { success: boolean; rows: Array<Record<string, unknown>>; error?: string }
	/** 事务内写语句，返回 { success, rows_affected } */
	exec(sql: string, params?: unknown[]): { success: boolean; rows_affected: number; error?: string }
}

/** SQLite 数据库 */
export const database: {
	/**
	 * 执行 SELECT 查询
	 *
	 * @param sql SELECT 语句
	 * @param params 绑定参数（? 占位符依次对应）
	 * @returns 成功返回 { success:true, rows:行数组 }；每行为「列名 → 值」的对象；失败含 error
	 */
	query(sql: string, params?: unknown[]): { success: boolean; rows: Array<Record<string, unknown>>; error?: string }
	/**
	 * 执行写语句
	 *
	 * @param sql INSERT/UPDATE/DELETE 等写语句
	 * @param params 绑定参数（? 占位符依次对应）
	 * @returns 含 success 与受影响行数 rows_affected 的结果
	 */
	exec(sql: string, params?: unknown[]): { success: boolean; rows_affected: number; error?: string }
	/**
	 * 同步事务：回调内执行 query/exec，全部成功则提交
	 *
	 * @param fn 事务回调，接收 { query, exec }（绑定到事务）；正常返回即提交，
	 *          抛异常或返回 Promise 则回滚（事务内禁异步）
	 * @returns { success, result: fn返回值 } 或 { success:false, error }
	 */
	transaction(fn: (tx: DatabaseTx) => unknown): { success: boolean; result?: unknown; error?: string }
	/**
	 * 按名称迁移：_migrations 表未记录则在事务内执行 upSql（支持多语句，禁含 BEGIN/COMMIT）并记录
	 *
	 * @param name 迁移名（唯一标识）
	 * @param upSql 建表/改表 SQL
	 * @returns { success, applied }（applied=false 表示此前已应用，本次跳过）
	 */
	migrate(name: string, upSql: string): { success: boolean; applied?: boolean; error?: string }
}
