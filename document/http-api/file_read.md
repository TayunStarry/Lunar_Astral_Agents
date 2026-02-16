# 文件读取接口

**功能**：读取文件内容

**请求方式**: GET
**路径**: `/read/[文件路径]`
_文件路径相对于 local_data 目录_

## 响应

- 成功 (200 OK):
  - 自动设置 Content-Type
  - 响应体: 文件二进制流

## 支持的 MIME 类型

| 文件类型    | Content-Type             |
| ----------- | ------------------------ |
| 文本文件    | text/plain               |
| HTML/CSS/JS | 对应类型                 |
| 图片        | image/\*                 |
| PDF         | application/pdf          |
| 其他        | application/octet-stream |

## 错误响应

- `400 Bad Request`: 缺少文件名
- `403 Forbidden`: 访问受限
- `404 Not Found`: 文件不存在

## 示例代码

```javascript
// 读取文本文件
fetch("/read/note.txt")
  .then(res => res.text())
  .then(content => console.log("文件内容:", content))
  .catch(error => console.error("读取文本文件失败:", error));

// 读取图片文件
fetch("/read/image.png")
  .then(res => res.blob())
  .then(
    blob => {
      const img = document.createElement("img");
      img.src = URL.createObjectURL(blob);
      document.body.appendChild(img);
    }
  )
  .catch(error => console.error("读取图片失败:", error));

// 读取JSON文件
fetch("/read/data.json")
  .then(res => res.json())
  .then(data => console.log("JSON数据:", data))
  .catch(error => console.error("读取JSON文件失败:", error));
```
