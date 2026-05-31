# 子系统——文件管理（storage）

本地文件存储与 SQLite 数据库管理模块，提供文件 CRUD、ZIP 归档、数据库批量操作等功能，采用 Module（逻辑层）+ Server（HTTP 层）二层架构。

---

## 目录

- [功能概述](#功能概述)
- [项目结构](#项目结构)
- [核心架构](#核心架构)
- [核心模块说明](#核心模块说明)
- [API 接口定义](#api-接口定义)
- [使用示例](#使用示例)
- [常见问题](#常见问题)

---

## 功能概述

`storage` 子系统提供完整的本地文件与数据库管理能力：

| 功能 | 说明 |
|------|------|
| 文件保存 | 接收上传文件并持久化到本地存储，支持覆盖控制 |
| 文件读取 | 读取文件内容并自动检测 MIME 类型 |
| 文件删除 | 删除指定文件或目录 |
| 文件下载 | 提供文件下载服务（Content-Disposition） |
| 文件列表 | 列出指定目录下的文件和子目录 |
| ZIP 归档 | 多文件 ZIP 压缩 + ZIP 解压 |
| 随机背景图 | 从图片目录随机选取背景图片 |
| SQLite 数据库 | 批量增删改查/建表/删表/统计，支持事务 |

---

## 项目结构

```
storage/
├── go.mod                          ← 模块定义（依赖 go-sqlite3）
├── module/                         ← 核心业务逻辑层
│   ├── type.go                     ← 全部数据结构定义
│   ├── save.go                     ← 文件保存 + 并发文件锁
│   ├── read.go                     ← 文件读取 + MIME 类型检测
│   ├── delete.go                   ← 文件/目录删除
│   ├── download.go                 ← 文件下载信息获取
│   ├── filelist.go                 ← 目录列表 + 标识符过滤
│   ├── archive.go                  ← ZIP 压缩/解压（全内存操作）
│   ├── background.go               ← 随机背景图选取
│   └── database.go                 ← SQLite 数据库完整封装（批量操作/事务/安全过滤）
│
└── server/                         ← HTTP 服务层
    ├── save.go                     ← POST 保存处理器
    ├── read.go                     ← GET 读取处理器
    ├── delete.go                   ← DELETE 删除处理器
    ├── download.go                 ← 文件下载处理器
    ├── filelist.go                 ← 目录列表处理器
    ├── archive.go                  ← POST 压缩 / PUT 解压
    ├── background.go               ← GET 随机背景图
    └── database.go                 ← POST 数据库批量操作
```

---

## 核心架构

### 二层分离设计

```
┌─────────────────────────────────────────────────┐
│            HTTP 层（server/）                     │
│  SaveHandler  ReadHandler  DeleteHandler  ...    │
│  解析 HTTP 请求 → 调用 module 函数 → 写 HTTP 响应  │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│          业务逻辑层（module/）                     │
│  文件操作: Save/file/read/file/delete/file/download/List/file/archive │
│  数据库: CRUD/Table/Info + 事务 + 批量             │
│  安全机制: 路径防越权 / 文件名校验 / 并发文件锁      │
└──────────────────────┬──────────────────────────┘
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
     ┌─────────┐  ┌──────────┐  ┌──────────┐
     │ 文件系统  │  │  SQLite  │  │  config  │
     │ (os 包)  │  │(go-sqlite3)│  │ (内部模块) │
     └─────────┘  └──────────┘  └──────────┘
```

---

## 核心模块说明

### 1. 文件安全机制

所有文件操作遵循统一的路径安全模型：

```
请求路径
  │
  ├── filepath.Clean(filepath.Join(config.LocalDir, 请求路径))
  │     └── 规范化拼接基准目录与请求路径
  │
  ├── strings.HasPrefix(完整路径, config.LocalDir)
  │     └── 防止路径遍历攻击（../../../etc/passwd → 拒绝）
  │
  └── 执行实际操作
```

**文件名处理**：

- HTTP 头 `X-File-Name` 传递 Base64 编码的文件名
- `DecodeFileName()` 解码后检查 `./` 和 `../` 路径穿越
- 不覆盖模式自动追加时间戳避免冲突

**并发文件锁**：

```go
var FileLocks sync.Map   // 每个文件路径对应一个 *sync.Mutex

func GetFileLock(path string) *sync.Mutex {
    lock, _ := FileLocks.LoadOrStore(path, &sync.Mutex{})
    return lock.(*sync.Mutex)
}
```

### 2. SQLite 数据库引擎

**初始化配置**：

```
模式: WAL (Write-Ahead Logging)
busy_timeout: 10000ms
连接池: 10 最大 / 5 空闲 / 5 分钟生命周期
PRAGMA: synchronous=NORMAL, cache_size=10000, foreign_keys=ON
```

**批量操作 API**（`ExecuteDatabaseRequest`）：

```go
type DatabaseRequest struct {
    Operations  []interface{}   // DataOperation | TableOperation | InfoOperation
    Transaction bool            // 是否启用事务包裹
}

type BatchResult struct {
    Success    bool
    Results    []interface{}
    TotalTime  int64
    Operations int
}
```

事务内任一操作失败立即回滚。

**支持的操作类型**：

| 分类 | 操作类型 | SQL 生成 |
|------|---------|---------|
| 数据操作 | `insert` | `INSERT INTO table (cols) VALUES (...)` |
| | `update` | `UPDATE table SET ... WHERE ...` |
| | `delete` | `DELETE FROM table WHERE ...` |
| | `select` | `SELECT * FROM table WHERE ... ORDER BY ... LIMIT ...` |
| 表操作 | `create` | `CREATE TABLE IF NOT EXISTS table (columns, constraints, indexes)` |
| | `drop` | `DROP TABLE IF EXISTS table` |
| | `truncate` | `DELETE FROM table` |
| 信息操作 | `tables` | 查询 `sqlite_master` |
| | `structure` | `PRAGMA table_info(table)` |
| | `count` | `SELECT COUNT(*) FROM table` |

**WHERE 子句支持 MongoDB 风格操作符**：

| 操作符 | SQL 转换 | 示例 |
|--------|---------|------|
| `$eq` | `= ?` | `{"age": {"$eq": 25}}` |
| `$ne` | `!= ?` | `{"status": {"$ne": "deleted"}}` |
| `$gt` | `> ?` | `{"age": {"$gt": 18}}` |
| `$gte` | `>= ?` | `{"age": {"$gte": 18}}` |
| `$lt` | `< ?` | `{"age": {"$lt": 60}}` |
| `$lte` | `<= ?` | `{"age": {"$lte": 60}}` |
| `$like` | `LIKE ?` | `{"name": {"$like": "%test%"}}` |
| `$in` | `IN (?, ?, ...)` | `{"role": {"$in": ["admin","user"]}}` |
| 默认 | `= ?` | `{"id": 123}` |

**SQL 注入防护**（`sanitizeIdentifier`）：

- 移除危险字符：`;` `'` `"` `\` `--` `/*` `*/` `(` `)` `[` `]`
- 检测 SQL 关键字冲突（SELECT/INSERT/DROP 等），自动前缀 `_` 避免

### 3. ZIP 压缩/解压

- **压缩**：全内存操作（`bytes.Buffer` → `zip.NewWriter`），Deflate 压缩
- **解压**：读取全部内容到内存后返回文件列表（name/size/content/last_modified/extension）

---

## API 接口定义

### RESTful 端点总览

| 方法 | 路径 | 功能 | 特殊 Header/参数 |
|------|------|------|-----------------|
| POST | `/file/write/` | 保存文件 | `X-File-Name`(Base64), `X-Overwrite` |
| GET | `/file/read/{path}` | 读取文件 | - |
| DELETE | `/file/delete/{path}` | 删除文件/目录 | - |
| GET | `/file/download/{path}` | 下载文件 | - |
| POST | `/file/list/{path}` | 列出目录 | - |
| POST | `/file/archive/` | 创建 ZIP | multipart: `files`, `zip_name` |
| PUT | `/file/archive/` | 解压 ZIP | multipart: `zip_file` |
| GET | `/background/` | 随机背景图 | - |
| POST | `/database/` | 数据库批量操作 | JSON body（见下方） |

### 数据库操作请求格式

```json
{
  "operations": [
    {
      "type": "create",
      "table": "users",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary_key": true},
        {"name": "name", "type": "TEXT", "not_null": true},
        {"name": "email", "type": "TEXT"}
      ],
      "indexes": [{"name": "idx_name", "columns": ["name"]}]
    },
    {
      "type": "insert",
      "table": "users",
      "data": {"name": "Alice", "email": "alice@example.com"}
    },
    {
      "type": "select",
      "table": "users",
      "where": {"name": {"$like": "%li%"}},
      "order_by": "name ASC",
      "limit": 10,
      "offset": 0
    },
    {
      "type": "count",
      "table": "users"
    }
  ],
  "transaction": true
}
```

### 文件保存请求格式

```
POST /file/write/
Headers:
  X-File-Name: <Base64 编码的文件名>  
  X-Overwrite: true/false (可选)
Body: <原始文件字节流>
```

---

## 使用示例

### Go 代码中使用

```go
package main

import (
    storage "storage/module"
)

func main() {
    // 初始化数据库
    storage.InitDatabase()

    // 批量操作
    req := storage.DatabaseRequest{
        Operations: []interface{}{
            storage.DataOperation{
                Type:  "select",
                Table: "messages",
                Where: map[string]interface{}{"role": "user"},
                Limit: 10,
            },
        },
        Transaction: false,
    }
    result := storage.ExecuteDatabaseRequest(req)

    // 文件操作
    data := []byte("Hello World")
    storage.SaveFile("test.txt", data, false)
    content, mimeType, _ := storage.ReadFile("test.txt")
}
```

### HTTP API 调用

```bash
# 保存文件
curl -X POST http://localhost:36789/file/write/ \
  -H "X-File-Name: $(echo -n 'example.txt' | base64)" \
  --data-binary @example.txt

# 读取文件
curl http://localhost:36789/file/read/example.txt

# 数据库操作
curl -X POST http://localhost:36789/database/ \
  -H "Content-Type: application/json" \
  -d '{"operations":[{"type":"tables"}],"transaction":false}'
```

---

## 常见问题

### Q: 文件保存在哪里？

所有文件保存在 `config.LocalDir` 指定的目录下（默认 `local_data/`）。使用 `X-File-Name` 指定相对于该目录的路径。

### Q: 如何防止路径遍历攻击？

模块层对所有路径执行 `filepath.Clean` + `strings.HasPrefix` 检查，确保实际访问的路径始终在以 `config.LocalDir` 为根的子目录内。

### Q: 数据库支持哪些数据类型？

SQLite 原生支持 INTEGER、REAL、TEXT、BLOB、NULL 五种类型。建表时建议使用这些类型确保兼容性。

### Q: 事务失败会自动回滚吗？

是的。当 `Transaction: true` 时，所有操作在单个事务内执行。任一操作失败会立即回滚，之前已成功的操作也会被撤销。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../config/README.md) —— `LocalDir`、`Database` 路径配置
- [星图·琉璃](../../crystal_astral/README.md) —— 文件管理 HTTP 端点使用方
- [星图·月华](../../lunar_astral/README.md) —— 适配器层调用方