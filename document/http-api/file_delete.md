# 文件/目录删除接口

**功能**：删除指定文件或目录

**请求方式**: DELETE
**路径**: `/delete/[路径]`
_路径相对于local_data目录，可以是文件或目录路径_

## 响应示例 (200 OK)

```json
{
  "path": "local_data/report.pdf"
}
```

## 注意事项

- 支持删除文件和非空目录
- 删除目录时会**执行递归删除**
- 包含嵌套文件/子目录
- 删除操作不可逆，无回收机制
- 建议删除前进行二次确认

## 错误响应

- `403 Forbidden`: 访问受限
- `404 Not Found`: 文件不存在
- `500 Internal Server Error`: 删除失败

## 示例代码

```javascript
// 删除文件
fetch("/delete/report.pdf", { method: "DELETE" })
  .then((response) => {
    if (!response.ok) throw new Error("删除失败");
    return response.json();
  })
  .then((data) => console.log("删除成功:", data))
  .catch((error) => console.error("删除失败:", error));

// 删除目录
fetch("/delete/documents", { method: "DELETE" })
  .then((response) => {
    if (!response.ok) throw new Error("删除失败");
    return response.json();
  })
  .then((data) => console.log("删除成功:", data))
  .catch((error) => console.error("删除失败:", error));
```
