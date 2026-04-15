import { DatabaseOperation, DatabaseRequest, BatchResult, DataOperation, ExecuteDatabaseRequest } from '../../config/index';

/**
 * 向数据库查询数据
 *
 * @param {DatabaseOperation[]} operations - 数据库操作列表
 *
 * @param {DatabaseOperation} [createTableOperation] - 表不存在时创建表的操作
 *
 * @returns {BatchResult} - 数据库查询结果
 */
export function queryFromDatabase(operations: DatabaseOperation[], createTableOperation: DatabaseOperation): BatchResult {
    /** 构建数据库查询请求体 */
    const requestBody: DatabaseRequest = { operations, transaction: false };
    /** 发送数据库查询请求 */
    let [result, error] = ExecuteDatabaseRequest(requestBody);
    // 检查响应状态是否成功
    if (!error) throw new Error('数据库查询失败');
    // 检查查询结果是否有效
    if (!result.success || !result.results[0].success) {
        /** 提取错误信息 */
        const errorMessage = result.error || result.results[0].error || '';
        // 检查是否是因为表不存在的错误，并且提供了创建表的操作
        if (errorMessage.includes('no such table') && createTableOperation) {
            /** 构建创建表请求体 */
            const createTableRequest: DatabaseRequest = { operations: [createTableOperation], transaction: false };
            /** 发送创建表请求 */
            let [createTableResult, tableError] = ExecuteDatabaseRequest(createTableRequest);
            // 检查创建表响应状态是否成功
            if (!tableError) throw new Error('创建表失败');
            /** 检查创建表操作是否成功 */
            if (!createTableResult.success) throw new Error('创建表失败');
            // 重新执行原始查询操作
            [result, error] = ExecuteDatabaseRequest(requestBody);
            // 检查重新执行查询响应状态是否成功
            if (!error) throw new Error('数据库查询失败');
            // 检查重新执行查询操作是否成功
            if (!result.success || !result.results[0].success) throw new Error('数据库查询失败');
        }
        else throw new Error('数据库查询失败');
    }
    // 返回查询结果
    return result;
}

/**
 * 从数据库中获取提示词
 *
 * @param {string} key - 索引键
 *
 * @description 从数据库中查询指定索引键对应的提示词
 *
 * @returns {string | null} - 提示词或null
 */
export function getPromptFromDatabase(key: string): string | null {
    try {
        /** 定义数据库操作对象数组 */
        const operations: DatabaseOperation[] = [
            {
                type: 'select',
                table: 'KeyPrompt',
                filter: {
                    IndexKey: key
                },
                limit: 1
            }
        ];
        /** 定义创建表操作 */
        const createTableOperation: DatabaseOperation = {
            type: 'create',
            table: 'KeyPrompt',
            definition: {
                columns: [
                    { name: "ID", type: "INTEGER", primary_key: true, auto_increment: true },
                    { name: "IndexKey", type: "TEXT" },
                    { name: "Prompt", type: "TEXT" }
                ]
            }
        };
        /** 解析数据库查询响应 */
        const result: BatchResult = queryFromDatabase(operations, createTableOperation);
        // 检查查询结果是否有效
        if (result.success && result.results[0].success && result.results[0].rows) {
            return result.results[0].rows[0].Prompt as string;
        }
        // 查询结果为空，返回null
        return null;
    }
    catch (error) {
        return null;
    }
}

/**
 * 向数据库中存储提示词
 *
 * @param {string} key - 索引键
 *
 * @param {string} prompt - 提示词
 *
 * @returns {boolean} - 是否成功
 */
export function savePromptToDatabase(key: string, prompt: string): boolean {
    try {
        /** 检查是否存在相同索引键的记录 */
        const existingPrompt = getPromptFromDatabase(key);
        /** 定义数据库操作对象数组 */
        const operations: DataOperation[] = [];
        // 更新现有记录
        if (existingPrompt) operations.push({ type: 'update', table: 'KeyPrompt', data: { Prompt: prompt }, filter: { IndexKey: key } });
        // 插入新记录
        else operations.push({ type: 'insert', table: 'KeyPrompt', data: { IndexKey: key, Prompt: prompt } });
        /** 定义创建表操作 */
        const createTableOperation: DatabaseOperation = {
            type: 'create',
            table: 'KeyPrompt',
            definition: {
                columns: [
                    { name: "ID", type: "INTEGER", primary_key: true, auto_increment: true },
                    { name: "IndexKey", type: "TEXT" },
                    { name: "Prompt", type: "TEXT" }
                ]
            }
        };
        /** 解析数据库查询响应 */
        const result: BatchResult = queryFromDatabase(operations, createTableOperation);
        // 检查操作是否成功
        return result.success && result.results[0].success;
    }
    catch (error) {
        console.error('向数据库存储提示词失败:', error);
        return false;
    }
}