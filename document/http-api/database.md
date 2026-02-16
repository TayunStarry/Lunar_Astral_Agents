# 数据库操作接口

**功能**：提供统一的数据库操作接口，支持数据增删改查、表管理和信息查询

**请求方式**: POST
**路径**: `/database/`

## 请求参数

| 参数名        | 类型    | 必填 | 说明                         |
| ------------- | ------- | ---- | ---------------------------- |
| `operations`  | array   | 是   | 操作数组，包含多个数据库操作 |
| `transaction` | boolean | 否   | 是否启用事务，默认为 false   |

### 操作类型

#### 数据操作 (DataOperation)

| 字段名   | 类型                   | 必填 | 说明                                     |
| -------- | ---------------------- | ---- | ---------------------------------------- |
| `type`   | string                 | 是   | 操作类型: insert, update, delete, select |
| `table`  | string                 | 是   | 表名                                     |
| `data`   | map[string]interface{} | 否   | 数据（insert/update 时使用）             |
| `filter` | map[string]interface{} | 否   | 过滤条件（where 子句）                   |
| `limit`  | int                    | 否   | 限制返回记录数                           |
| `offset` | int                    | 否   | 偏移量                                   |
| `order`  | array                  | 否   | 排序规则                                 |

#### 表操作 (TableOperation)

| 字段名       | 类型              | 必填 | 说明                             |
| ------------ | ----------------- | ---- | -------------------------------- |
| `type`       | string            | 是   | 操作类型: create, drop, truncate |
| `table`      | string            | 是   | 表名                             |
| `definition` | \*TableDefinition | 否   | 表定义（create 时使用）          |

#### 信息操作 (InfoOperation)

| 字段名  | 类型   | 必填 | 说明                               |
| ------- | ------ | ---- | ---------------------------------- |
| `type`  | string | 是   | 操作类型: tables, structure, count |
| `table` | string | 否   | 表名（structure, count 时使用）    |

## 响应示例 (200 OK)

```json
{
  "success": true,
  "results": [
    {
      "success": true,
      "operation": "select",
      "table": "users",
      "rows": [
        {
          "id": 1,
          "name": "John Doe",
          "email": "john@example.com"
        }
      ]
    },
    {
      "success": true,
      "operation": "insert",
      "table": "logs",
      "affected_rows": 1,
      "last_insert_id": 10
    }
  ],
  "total_time_ms": 15,
  "operations": 2
}
```

## 请求示例

### 1. 数据查询

```javascript
fetch("/database/", {
  method: "POST",
  headers: {
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    "operations": [
      {
        "type": "select",
        "table": "users",
        "filter": {
          "age": {
            "$gt": 18
          }
        },
        "limit": 10,
        "order": [
          {"column": "id", "direction": "desc"}
        ]
      }
    ]
  })
})
.then(response => response.json())
.then(data => console.log("查询结果:", data));
```

### 2. 数据插入

```javascript
fetch("/database/", {
  method: "POST",
  headers: {
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    "operations": [
      {
        "type": "insert",
        "table": "users",
        "data": {
          "name": "John Doe",
          "email": "john@example.com",
          "age": 30
        }
      }
    ]
  })
})
.then(response => response.json())
.then(data => console.log("插入结果:", data));
```

### 3. 表操作

```javascript
fetch("/database/", {
  method: "POST",
  headers: {
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    "operations": [
      {
        "type": "create",
        "table": "users",
        "create_sql": "CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT UNIQUE, age INTEGER)"
      }
    ]
  })
})
.then(response => response.json())
.then(data => console.log("创建表结果:", data));
```

### 4. 信息查询

```javascript
fetch("/database/", {
  method: "POST",
  headers: {
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    "operations": [
      {
        "type": "tables"
      },
      {
        "type": "structure",
        "table": "users"
      },
      {
        "type": "count",
        "table": "users"
      }
    ]
  })
})
.then(response => response.json())
.then(data => console.log("信息查询结果:", data));
```

## 过滤条件操作符

| 操作符  | 说明     | 示例                                         |
| ------- | -------- | -------------------------------------------- |
| `$eq`   | 等于     | `{"age": {"$eq": 30}}`                       |
| `$ne`   | 不等于   | `{"status": {"$ne": "active"}}`              |
| `$gt`   | 大于     | `{"salary": {"$gt": 5000}}`                  |
| `$gte`  | 大于等于 | `{"score": {"$gte": 60}}`                    |
| `$lt`   | 小于     | `{"age": {"$lt": 18}}`                       |
| `$lte`  | 小于等于 | `{"price": {"$lte": 100}}`                   |
| `$like` | 模糊匹配 | `{"name": {"$like": "%John%"}}`              |
| `$in`   | 包含于   | `{"status": {"$in": ["active", "pending"]}}` |

## 错误响应

| 状态码 | 描述             |
| ------ | ---------------- |
| `400`  | 请求参数错误     |
| `405`  | 不允许的请求方法 |
| `500`  | 服务器内部错误   |

```json
{
  "success": false,
  "error": "数据库连接失败: sql: database is closed",
  "results": [],
  "total_time_ms": 0,
  "operations": 0
}
```

## 注意事项

- 所有表名和列名会经过安全过滤，防止 SQL 注入
- 支持批量操作，可在一个请求中执行多个操作
- 启用事务时，所有操作要么全部成功，要么全部失败
- 对于大型操作，建议分批执行，避免超时
- 数据库操作会记录详细日志（在开发模式下）
