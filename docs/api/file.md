# 文件管理 API

本文档介绍文件管理相关的 API 接口。

## 读取文件

读取指定路径的文件内容。

- **路径**: `/read/{path}`
- **方法**: `GET`

### 响应

文件二进制内容或文本。

## 删除文件

删除指定文件。

- **路径**: `/delete/`
- **方法**: `DELETE`

### 请求体

```json
{
    "path": "images/generated/20260102_150405.png"
}
```

### 响应示例

```json
{
    "success": true,
    "message": "文件删除成功"
}
```

## 获取文件列表

获取指定目录下的文件列表。

- **路径**: `/file_list/`
- **方法**: `POST`

### 请求体

```json
{
    "path": "images/generated"
}
```

### 响应示例

```json
{
    "path": "images/generated",
    "files": [
        {
            "name": "20260102_150405.png",
            "size": 102400,
            "modified": "2026-01-02T15:04:05Z",
            "is_directory": false
        }
    ]
}
```

## 下载文件

下载指定文件。

- **路径**: `/download/`
- **方法**: `GET`

### 查询参数

| 参数 | 类型 | 说明 |
|------|------|------|
| path | string | 文件路径 |

## 保存文件

保存文件到服务器。

- **路径**: `/save`
- **方法**: `POST`

### 请求体

```json
{
    "path": "documents/test.txt",
    "content": "文件内容...",
    "encoding": "utf-8"
}
```

## 文件归档

创建文件归档包。

- **路径**: `/archive`
- **方法**: `POST`

### 请求体

```json
{
    "source_paths": ["./images", "./documents"],
    "output_path": "./archive.zip",
    "compression_level": 5
}
```

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md) | [下一篇：数据库](./database.md)
