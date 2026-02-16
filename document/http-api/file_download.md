# 文件下载接口

**功能**：下载指定文件

**请求方式**: GET
**路径**: `/download/[文件路径]`
_文件路径相对于local_data目录_

## 响应

- 成功 (200 OK):
  - 响应头: `Content-Disposition: attachment; filename="filename.ext"`
  - 响应体: 文件二进制流

## 错误响应

- `400 Bad Request`: 请求的是目录
- `403 Forbidden`: 访问受限
- `404 Not Found`: 文件不存在

## 示例代码

```javascript
function createDownloadLink(blob, filename) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function handleDownloadResponse(response) {
  if (!response.ok) throw new Error("文件下载失败");
  return response.blob();
}

function downloadFile(filePath) {
  fetch(`/download/${filePath}`)
    .then(handleDownloadResponse)
    .then(blob => createDownloadLink(blob, filePath.split("/").pop()))
    .catch(err => console.error("文件下载失败:", err));
}

downloadFile("report.pdf");
downloadFile("documents/image.png");
```
