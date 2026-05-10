# Storage子系统 - 数据存储架构文档

> 📦 **Storage子系统**是星月智能的核心数据存储模块，负责管理文件读写、数据库操作、归档备份等功能。

---

## 🏗️ 架构设计

### 整体架构

```
┌─────────────────────────────────────────────────────────┐
│                    Storage子系统                       │
├─────────────────────────────────────────────────────────┤
│  ┌───────────────────┐   ┌─────────────────────────┐   │
│  │      Server层     │   │         Module层        │   │
│  │  HTTP Handler     │   │  核心业务逻辑           │   │
│  └─────────┬─────────┘   └───────────┬─────────────┘   │
│            │                         │                 │
│            │ HTTP请求                 │ 内部调用        │
│            ▼                         ▼                 │
│  ┌─────────────────────────────────────────────────┐   │
│  │              Storage Core                       │   │
│  │  文件系统操作 / 数据库操作 / 归档管理            │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                              │
│                         ▼                              │
│  ┌─────────────────────────────────────────────────┐   │
│  │              持久化层                             │   │
│  │  本地文件系统 / SQLite数据库 / 归档文件          │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### 模块划分

| 模块 | 文件 | 职责 |
|------|------|------|
| **Server** | `server/*.go` | HTTP接口处理 |
| **Module** | `module/*.go` | 核心业务逻辑 |
| **Type** | `module/type.go` | 类型定义 |

---

## 🔌 API接口

### 基础路径

所有API接口的基础路径为：`http://localhost:{port}/`

---

### 1. 文件读取接口

#### GET /read/{file_path}

**功能**：读取指定路径的文件内容

**路径参数**：
| 参数 | 类型 | 说明 |
|------|------|------|
| `file_path` | string | 文件相对路径 |

**成功响应**：
- Content-Type: 根据文件类型自动设置
- Body: 文件二进制内容

**失败响应**：
```json
{
  "error": "文件未找到",
  "status": 404
}
```

---

### 2. 文件保存接口

#### POST /save

**功能**：保存文件内容

**请求体**：
```json
{
  "path": "/documents/note.txt",
  "content": "Hello World!",
  "encoding": "utf-8",
  "overwrite": true
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 文件路径 |
| `content` | string | 是 | 文件内容 |
| `encoding` | string | 否 | 编码格式，默认utf-8 |
| `overwrite` | bool | 否 | 是否覆盖，默认true |

**响应格式**：
```json
{
  "success": true,
  "message": "File saved successfully"
}
```

---

### 3. 文件列表接口

#### POST /file_list/

**功能**：获取指定目录下的文件列表

**请求体**：
```json
{
  "path": "/documents/",
  "recursive": false,
  "filter": "*.txt"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 目录路径 |
| `recursive` | bool | 否 | 是否递归，默认false |
| `filter` | string | 否 | 文件过滤器 |

**响应格式**：
```json
{
  "success": true,
  "files": [
    {
      "name": "note.txt",
      "path": "/documents/note.txt",
      "size": 12,
      "is_dir": false,
      "modified_time": "2024-01-15T10:30:00Z",
      "extension": ".txt"
    }
  ]
}
```

---

### 4. 文件删除接口

#### DELETE /delete/{file_path}

**功能**：删除文件或目录

**路径参数**：
| 参数 | 类型 | 说明 |
|------|------|------|
| `file_path` | string | 文件或目录路径 |

**响应格式**：
```json
{
  "success": true,
  "message": "Deleted successfully"
}
```

---

### 5. 文件下载接口

#### GET /download/{file_path}

**功能**：下载指定文件

**路径参数**：
| 参数 | 类型 | 说明 |
|------|------|------|
| `file_path` | string | 文件路径 |

**成功响应**：
- Content-Type: 根据文件类型自动设置
- Content-Disposition: `attachment; filename="xxx"`
- Body: 文件二进制内容

---

### 6. 文件归档接口

#### POST /archive

**功能**：将指定目录归档为ZIP文件

**请求体**：
```json
{
  "source_path": "/data/files/",
  "archive_path": "/backup/archive.zip",
  "compression_level": 6
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `source_path` | string | 是 | 源目录路径 |
| `archive_path` | string | 是 | 归档文件路径 |
| `compression_level` | int | 否 | 压缩级别1-9，默认6 |

**响应格式**：
```json
{
  "success": true,
  "archive_path": "/backup/archive.zip",
  "original_size": 1024000,
  "compressed_size": 512000
}
```

---

### 7. 数据库操作接口

#### POST /database/

**功能**：执行数据库操作（查询、插入、更新、删除）

**请求体 - 查询**：
```json
{
  "action": "query",
  "table": "messages",
  "conditions": {
    "user_id": "user_001"
  },
  "order_by": "created_at DESC",
  "limit": 10,
  "offset": 0
}
```

**请求体 - 插入**：
```json
{
  "action": "insert",
  "table": "messages",
  "data": {
    "user_id": "user_001",
    "content": "Hello",
    "type": "text",
    "created_at": "2024-01-15T10:30:00Z"
  }
}
```

**请求体 - 更新**：
```json
{
  "action": "update",
  "table": "messages",
  "conditions": {
    "id": "msg_001"
  },
  "data": {
    "content": "Updated content"
  }
}
```

**请求体 - 删除**：
```json
{
  "action": "delete",
  "table": "messages",
  "conditions": {
    "user_id": "user_001"
  }
}
```

**响应格式**：
```json
{
  "success": true,
  "data": [...],
  "affected_rows": 1
}
```

---

## 💾 数据持久化方案

### 文件存储

| 类型 | 存储位置 | 说明 |
|------|----------|------|
| 用户数据 | `local_data/` | 用户上传和生成的文件 |
| 模型文件 | `local_data/models/` | AI模型权重文件 |
| 扩展包 | `local_data/package/` | 系统扩展包 |
| 音频资源 | `local_data/audios/` | TTS语音文件 |

### 数据库存储

使用SQLite作为轻量级数据库：

**数据库文件**：`local_data/database.sqlite`

**数据表**：

| 表名 | 用途 |
|------|------|
| `messages` | 消息记录 |
| `users` | 用户信息 |
| `applications` | 应用配置 |
| `models` | 模型配置 |
| `settings` | 系统设置 |

### 归档策略

- **自动归档**：定期对旧数据进行归档
- **压缩格式**：ZIP压缩，支持不同压缩级别
- **备份位置**：`local_data/backup/`

---

## 📁 目录结构

```
subsystem/storage/
├── module/           # 核心业务模块
│   ├── type.go       # 类型定义
│   ├── read.go       # 文件读取
│   ├── save.go       # 文件保存
│   ├── delete.go     # 文件删除
│   ├── filelist.go   # 文件列表
│   ├── download.go   # 文件下载
│   ├── archive.go    # 文件归档
│   └── database.go   # 数据库操作
├── server/           # HTTP服务层
│   ├── read.go       # 读取接口
│   ├── save.go       # 保存接口
│   ├── delete.go     # 删除接口
│   ├── filelist.go   # 列表接口
│   ├── download.go   # 下载接口
│   ├── archive.go    # 归档接口
│   └── database.go   # 数据库接口
├── go.mod
└── go.sum
```

Storage子系统为星月智能提供数据持久化支持，[月华智能体](../luna_astral.md)和[琉璃智能体](../crystal_astral.md)都依赖此子系统进行文件和数据库操作。配置管理由[配置子系统](config.md)负责。

---

## 🔗 关联文档

- [主项目README](../../README.md)
- [星图·月华文档](../luna_astral.md)
- [星图·琉璃文档](../crystal_astral.md)
- [配置子系统文档](config.md)