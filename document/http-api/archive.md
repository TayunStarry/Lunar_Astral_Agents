# ZIP压缩与解压接口

**功能**：ZIP文件的压缩与解压处理

**请求方式**：POST（压缩）/ PUT（解压）
**路径**: `/archive`

## 压缩功能 (POST)

**功能**：将多个文件压缩为ZIP文件并直接返回

### 请求格式

- **Content-Type**: `multipart/form-data`
- **参数**:
  - `files` (必需): 要压缩的文件列表（多文件）
  - `zip_name` (可选): 输出的ZIP文件名，默认为"archive.zip"

### 响应

- **成功 (200 OK)**:
  - Content-Type: `application/zip`
  - Content-Disposition: `attachment; filename=[zip_name]`
  - 响应体: ZIP文件二进制流

### 示例代码

```javascript
// 压缩多个文件
const formData = new FormData();
files.forEach((file) => formData.append("files", file));
formData.append("zip_name", "my_files.zip");

fetch("/archive", {
  method: "POST",
  body: formData,
})
  .then((response) => response.blob())
  .then((blob) => {
    // 创建下载链接
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "my_files.zip";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  })
  .catch((error) => console.error("压缩失败:", error));
```

## 解压功能 (PUT)

**功能**：解压ZIP文件并返回文件列表

### 解压请求格式

- **Content-Type**: `multipart/form-data`
- **参数**:
  - `zip_file` (必需): 要解压的ZIP文件

### 响应示例 (200 OK)

```json
{
  "total_files": 3,
  "original_zip": "archive.zip",
  "extracted_files": [
    {
      "name": "document.txt",
      "size": 1024,
      "content": "base64编码的文件内容",
      "last_modified": "2023-10-01T12:00:00Z",
      "extension": ".txt"
    }
  ]
}
```

### 解压文件对象结构

| 字段            | 类型   | 说明                    |
| --------------- | ------ | ----------------------- |
| `name`          | string | 文件名                  |
| `size`          | number | 文件大小（字节）        |
| `content`       | byte[] | 文件内容的字节数组      |
| `last_modified` | string | 最后修改时间（ISO格式） |
| `extension`     | string | 文件扩展名（小写）      |

### 解压示例代码

```javascript
// 解压ZIP文件
const formData = new FormData();
formData.append("zip_file", zipFile);

fetch("/archive", {
  method: "PUT",
  body: formData,
})
  .then((response) => response.json())
  .then((data) => {
    console.log(`解压成功，共 ${data.total_files} 个文件`);

    // 处理解压后的文件
    data.extracted_files.forEach((file) => {
      if (!file.is_dir) {
        // 将字节数组转换为Blob进行下载
        const uint8Array = new Uint8Array(file.content);
        const blob = new Blob([uint8Array]);
        const url = URL.createObjectURL(blob);

        const a = document.createElement("a");
        a.href = url;
        a.download = file.name;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
      }
    });
  })
  .catch((error) => console.error("解压失败:", error));
```

## 错误响应

- `400 Bad Request`: 请求参数错误或格式不正确
- `405 Method Not Allowed`: 不支持的HTTP方法
- `500 Internal Server Error`: 服务器处理错误

## 注意事项

- **内存操作**: 所有压缩和解压操作均在内存中进行，不保存到服务器本地
- **直接返回**: 压缩结果直接返回ZIP文件流，解压结果返回文件对象数组
- **客户端处理**: 客户端负责处理返回的二进制数据并实现文件下载
- **大文件限制**: 建议单个ZIP文件不超过32MB，可通过服务器配置调整
- **安全考虑**: 支持路径安全检查，防止ZIP Slip攻击
