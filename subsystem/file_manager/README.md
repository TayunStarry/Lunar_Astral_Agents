# 子系统——文件管理（file_manager）

本地文件存储、SQLite 知识库、向量记忆库与扩展包管理模块，采用 Module（逻辑层）+ Server（HTTP 层）二层架构。

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

`file_manager` 子系统提供完整的本地文件与数据管理能力：

| 功能 | 说明 |
|------|------|
| 文件保存 | 接收上传文件并持久化到本地存储，支持覆盖控制 |
| 文件读取 | 读取文件内容并自动检测 MIME 类型 |
| 文件删除 | 删除指定文件或目录 |
| 文件下载 | 提供文件下载服务（Content-Disposition） |
| 文件列表 | 列出指定目录下的文件和子目录 |
| ZIP 归档 | 多文件 ZIP 压缩 + ZIP 解压 + 目录打包 |
| 文件预览 | 全局文件预览（图片/视频/文本） |
| 随机背景图 | 从图片目录随机选取背景图片 |
| 文件整理 | 按文件类型自动分类整理目录（text/code/image/video/audio/archive） |
| SQLite 知识库 | 批量增删改查/建表/删表/统计，支持事务与安全过滤 |
| 向量记忆库 | 嵌入向量检索记忆库：集合管理、消息/图片存储、语义查询、标签重建 |
| 扩展包管理 | 扩展包安装 / 导出 / 删除（`local_data/package/`） |

---

## 项目结构

### module/ — 核心业务逻辑层

| 文件 | 职责 |
|------|------|
| `type.go` | 全部数据结构定义（KnowledgeRequest、MemoryQueryResult、OrganizeRequest 等） |
| `variable.go` | 全局变量与常量 |
| `save.go` | 文件保存 + 并发文件锁 + 文件名解码 |
| `read.go` | 文件读取 + MIME 类型检测 |
| `delete.go` | 文件/目录删除 |
| `download.go` | 文件下载信息获取 |
| `filelist.go` | 目录列表 |
| `archive.go` | ZIP 压缩/解压 + 目录打包 |
| `background.go` | 随机背景图选取 |
| `preview.go` | 文件预览（图片/视频/文本） |
| `organize.go` | 批量文件整理（按类型分类） |
| `knowledge.go` | SQLite 知识库完整封装（批量操作/事务/安全过滤） |
| `memory.go` | 向量记忆库（嵌入向量存储 + 语义检索 + 集合管理） |
| `embedding.go` | 嵌入向量生成与相似度计算 |

### server/ — HTTP 服务层

| 文件 | 职责 |
|------|------|
| `save.go` | POST 保存处理器 |
| `read.go` | GET 读取处理器 |
| `delete.go` | DELETE 删除处理器 |
| `download.go` | 文件下载处理器 |
| `filelist.go` | 目录列表处理器 |
| `archive.go` | POST 压缩 / PUT 解压 + 扩展包安装/导出/删除 |
| `background.go` | GET 随机背景图 |
| `preview.go` | 文件预览处理器 |
| `organize.go` | 文件整理处理器 |
| `knowledge.go` | 知识库批量操作处理器 |
| `memory.go` | 记忆库统一端点处理器（/memory/ 子路由分发） |
| `type.go` / `variable.go` | 请求/响应类型与辅助函数 |

---

## 核心架构

### 二层分离设计

```
┌─────────────────────────────────────────────────┐
│            HTTP 层（server/）                     │
│  SaveHandler  ReadHandler  MemoryHandler  ...    │
│  解析 HTTP 请求 → 调用 module 函数 → 写 HTTP 响应  │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│          业务逻辑层（module/）                     │
│  文件操作: 保存/读取/删除/下载/列表/归档/预览/整理    │
│  知识库: 批量 CRUD/建表/统计 + 事务                 │
│  记忆库: 集合管理 + 向量检索 + 标签重建             │
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

### 2. SQLite 知识库（KnowledgeDB）

**初始化配置**：

```
模式: WAL (Write-Ahead Logging)
busy_timeout: 10000ms
连接池: 10 最大 / 5 空闲 / 5 分钟生命周期
PRAGMA: synchronous=NORMAL, cache_size=10000, foreign_keys=ON
```

**批量操作 API**（`ExecuteKnowledgeRequest`）：

```go
type KnowledgeRequest struct {
    Operations  []interface{}   // 数据操作 | 表操作 | 信息操作
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

### 3. 向量记忆库（MemoryDB）

基于嵌入向量的语义记忆存储，提供集合（collection）化管理：

- **实例初始化**：`/memory/init` 初始化嵌入服务与 LLM 标签生成服务
- **集合管理**：创建/打开/删除集合、集合统计、清空、重建标签向量
- **消息存储**：文本消息与图片的统一增删查（语义检索）
- **文档分页**：`documents` 端点支持分页浏览集合内容
- **跨进程安全**：JSON 文件存储，检测跨进程修改并自动重载

### 4. 扩展包管理

基于 ZIP 归档的扩展包安装体系：

- **安装**：解压上传的归档到 `local_data/package/<包名>/` 目录
- **导出**：将包目录打包导出
- **删除**：移除已安装的扩展包

### 5. ZIP 压缩/解压

- **压缩**：全内存操作（`bytes.Buffer` → `zip.NewWriter`），Deflate 压缩
- **解压**：读取全部内容到内存后返回文件列表（name/size/content/last_modified/extension）

---

## API 接口定义

> `file_manager` 作为库集成，路由由宿主程序（如 crystal_astral）注册。

### RESTful 端点总览

| 方法 | 路径 | 功能 |
|------|------|------|
| POST | `/file/write` | 保存文件（Header: `X-File-Name` Base64） |
| GET | `/file/read/{path}` | 读取文件 |
| DELETE | `/file/delete/{path}` | 删除文件/目录 |
| GET | `/file/download/{path}` | 下载文件 |
| POST | `/file/list/{path}` | 列出目录 |
| POST | `/file/archive` | 创建 ZIP |
| GET | `/file/preview` | 文件预览（图片/视频/文本） |
| POST | `/file/organize` | 批量文件整理 |
| POST | `/file/package/install` | 安装扩展包 |
| POST | `/file/package/export` | 导出扩展包 |
| POST | `/file/package/delete` | 删除扩展包 |
| GET | `/background` | 随机背景图 |
| POST | `/knowledge/` | 知识库批量操作 |
| ANY | `/memory/` | 记忆库（初始化/集合/消息/文档/重建） |

### 记忆库端点（/memory/）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/memory/init` | 实例初始化（嵌入服务 + LLM 服务） |
| GET | `/memory/stats` | 全局统计（聚合所有集合） |
| GET | `/memory/collections` | 列出所有集合 |
| POST | `/memory/{name}` | 创建/打开集合 |
| DELETE | `/memory/{name}` | 删除集合 |
| GET | `/memory/{name}/stats` | 集合统计 |
| POST/GET/DELETE | `/memory/{name}/messages` | 消息/图片统一增删查 |
| GET | `/memory/{name}/documents` | 文档分页列表 |
| POST | `/memory/{name}/rebuild` | 重建标签向量 |
| POST | `/memory/{name}/clear` | 清空集合 |

### 知识库操作请求格式

```json
{
  "operations": [
    {
      "type": "create",
      "table": "users",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary_key": true},
        {"name": "name", "type": "TEXT", "not_null": true}
      ]
    },
    {
      "type": "insert",
      "table": "users",
      "data": {"name": "Alice"}
    },
    {
      "type": "select",
      "table": "users",
      "where": {"name": {"$like": "%li%"}},
      "order_by": "name ASC",
      "limit": 10,
      "offset": 0
    }
  ],
  "transaction": true
}
```

---

## 使用示例

### Go 代码中使用

```go
package main

import (
    "strings"

    "LunarSubsystem/FileManager/module"
)

func main() {
    // 初始化知识库
    _ = module.InitKnowledgeDB("local_data/knowledge.db")

    // 知识库批量操作
    req := module.KnowledgeRequest{
        Operations: []interface{}{
            map[string]interface{}{
                "type":  "select",
                "table": "messages",
                "where": map[string]interface{}{"role": "user"},
                "limit": 10,
            },
        },
        Transaction: false,
    }
    result := module.ExecuteKnowledgeRequest(req)

    // 文件保存 / 读取
    data := strings.NewReader("Hello World")
    _, _, _ = module.SaveFile("test.txt", false, data)
    content, size, mimeType, _ := module.ReadFile("test.txt")
    _ = content
    _ = size
    _ = mimeType

    // 记忆库（向量检索）
    _ = module.InitMemoryDB("local_data/memory")
    _ = module.MemoryAddMessage(nil, "lunar_messages", "user", "你好")
}
```

### HTTP API 调用

```bash
# 保存文件
curl -X POST http://localhost:PORT/file/write \
  -H "X-File-Name: $(echo -n 'example.txt' | base64)" \
  --data-binary @example.txt

# 读取文件
curl http://localhost:PORT/file/read/example.txt

# 知识库操作
curl -X POST http://localhost:PORT/knowledge/ \
  -H "Content-Type: application/json" \
  -d '{"operations":[{"type":"tables"}],"transaction":false}'

# 记忆库初始化
curl -X POST http://localhost:PORT/memory/init \
  -H "Content-Type: application/json" \
  -d '{}'
```

---

## 常见问题

### Q: 文件保存在哪里？

所有文件保存在 `config.LocalDir` 指定的目录下（默认 `local_data/`）。使用 `X-File-Name` 指定相对于该目录的路径。

### Q: 如何防止路径遍历攻击？

模块层对所有路径执行 `filepath.Clean` + `strings.HasPrefix` 检查，确保实际访问的路径始终在以 `config.LocalDir` 为根的子目录内。

### Q: 知识库和记忆库有什么区别？

- **知识库**（KnowledgeDB）：普通 SQLite 数据库，适合结构化数据（表/行/列）的批量增删改查。
- **记忆库**（MemoryDB）：基于嵌入向量的语义记忆存储，适合自然语言消息的语义检索，支持图片记忆与集合管理。

### Q: 事务失败会自动回滚吗？

是的。当 `Transaction: true` 时，所有操作在单个事务内执行。任一操作失败会立即回滚，之前已成功的操作也会被撤销。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../general_config/README.md) —— `LocalDir`、`Database` 路径配置
- [星图·琉璃](../../crystal_astral/README.md) —— 文件管理 HTTP 端点使用方
- [星图·月华](../../lunar_astral/README.md) —— 适配器层调用方
