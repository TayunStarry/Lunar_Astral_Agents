# 文件列表接口

**功能**：获取指定目录下的文件列表

**请求方式**: GET
**路径**: `/file_list/[目录路径]`
_目录路径相对于 local_data 目录，留空则获取根目录_

## 响应示例 (200 OK)

```json
[
  {
    "name": "document.pdf",
    "size": 10240,
    "isDir": false,
    "lastModified": "2023-07-20T14:23:04Z",
    "path": "documents/reports/document.pdf"
  },
  {
    "name": "images",
    "size": 0,
    "isDir": true,
    "lastModified": "2023-07-20T10:15:30Z",
    "path": "documents/images"
  }
]
```

## 字段说明

| 字段           | 类型    | 说明                   |
| -------------- | ------- | ---------------------- |
| `name`         | string  | 文件/目录名            |
| `size`         | number  | 文件大小(字节)         |
| `isDir`        | boolean | 是否为目录             |
| `lastModified` | string  | 最后修改时间(ISO 格式) |
| `path`         | string  | 相对路径               |

## 错误响应

- `403 Forbidden`: 访问受限
- `404 Not Found`: 目录不存在

## 示例代码

```javascript
// 获取根目录文件列表
fetch("/files")
  .then(res => res.json())
  .then(files => files.forEach(file => consoleLogFileList(file)));

// 获取指定目录文件列表
fetch("/file_list/documents/reports")
  .then((res) => res.json())
  .then((files) => consoleLogFileList(files));

// 打印文件列表
function consoleLogFileList(files) {
  files.forEach(file => console.log(`${file.isDir ? "[目录]" : "[文件]"} ${file.name} (${file.size} bytes)`));
}
```
