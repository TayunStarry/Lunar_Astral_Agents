# 文件保存接口

**功能**：接收并保存客户端上传的文件

**请求方式**: POST
**路径**: `/save`

## 请求头

| 字段名           | 必填 | 默认值 | 说明                           |
| ---------------- | ---- | ------ | ------------------------------ |
| `X-File-Name`    | 是   | -      | 文件名，必须经过 Base64 编码   |
| `X-Overwrite`    | 否   | false  | 是否覆盖已存在文件，true/false |
| `Content-Length` | 是   | -      | 文件大小（字节）               |

## 请求体

文件二进制数据

## 响应示例 (200 OK)

```json
{
  "filename": "report.pdf",
  "path": "local_data/report.pdf",
  "overwrite": false,
  "size": 10240,
  "success": true
}
```

## 处理流程

1. Base64 解码获取原始文件名
2. 安全检查：防止路径遍历攻击
3. 检查文件是否存在
4. 若文件存在且未设置覆盖，则添加时间戳重命名
5. 创建目录并保存文件

## 错误响应

| 状态码 | 说明                 |
| ------ | -------------------- |
| `400`  | 缺少文件名或解码失败 |
| `403`  | 非法路径或权限不足   |
| `413`  | 文件过大             |
| `500`  | 服务器错误           |

```json
{
  "error": {
    "code": 400,
    "message": "文件名解码失败",
    "details": "invalid base64 encoding"
  }
}
```

## 完整示例代码

### 前端上传示例

```javascript
class FileUploader {
  // Base64编码文件名（支持中文）
  static encodeFileName(filename) {
    return btoa(unescape(encodeURIComponent(filename)));
  }

  // 上传文件
  static async uploadFile(file, overwrite = false) {
    const formData = new FormData();
    formData.append("file", file);

    const response = await fetch("/save", {
      method: "POST",
      headers: {
        "X-File-Name": this.encodeFileName(file.name),
        "X-Overwrite": overwrite.toString(),
      },
      body: file,
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.message || "上传失败");
    }

    return await response.json();
  }
}

// 使用示例
const fileInput = document.getElementById("fileInput");
fileInput.addEventListener("change", async (event) => {
  const file = event.target.files[0];
  if (!file) return;

  try {
    const result = await FileUploader.uploadFile(file, false);
    console.log("上传成功:", result);
    alert(`文件保存成功: ${result.filename}`);
  } catch (error) {
    console.error("上传失败:", error);
    alert(`上传失败: ${error.message}`);
  }
});
```

### 命令行上传示例

```bash
# 使用 curl 上传文件
curl -X POST http://localhost:36789/save \
  -H "X-File-Name: $(echo -n '报告.pdf' | base64)" \
  -H "X-Overwrite: false" \
  --data-binary "@./报告.pdf"
```

## 注意事项

- 文件名支持所有 Unicode 字符，通过 Base64 编码传输
- 默认情况下，重名文件会自动添加时间戳避免覆盖
- 文件路径相对于 `local_data` 目录
- 建议单文件不超过 100MB（可在服务器配置中调整）
