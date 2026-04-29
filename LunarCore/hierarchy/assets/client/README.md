# README

## 概述

本项目是一个基于 HTML/CSS/JS 的 Web 前端应用，用于与 `lunar_core` 后端进行交互。用户可以通过拖拽或上传的方式引用图片、视频等媒体文件，并将组合后的消息发送至 `lunar_core`，同时实时接收并展示后端的回复消息。

---

## 一、界面设计要求

### 整体风格

- 半透明包边玻璃风格（毛玻璃效果）
- 页面中心为一个 **宽高比 3:4** 的 Live2D 窗口（具体实现参考示例附件）
- 窗口底部包含一个输入框，支持：
  - 文本输入
  - 图片、视频等媒体文件的**拖拽引用**
- 输入框右侧有一个**发送按钮**，点击后将输入框中的内容发送至 `lunar_core` 后端

### 文件引用方式

1. **拖拽文件到输入框区域**即可引用：
   - 图片文件
   - 视频文件
   - 文本文件
2. 媒体类文件通过 `lunar_core` 提供的 HTTP 接口（`/save`）保存到服务器，并获得该文件的访问 URL。
3. 点击发送按钮时，将 URL 插入到消息中：
   - 图片：可作为 `image_url` 类型
   - 视频：可插入消息内容，或通过 `POST /videourl/batch` 接口单独传递视频 URL

### 消息记录模态框

- 页面**左下角**有一个按钮，点击后显示模态框。
- 模态框内容：当前前端**发送与接收**的消息记录，渲染逻辑参考示例附件。
- 再次点击左下角按钮则**隐藏**模态框。
- 模态框**默认隐藏**。

---

## 二、消息格式规范

前端发送给 `lunar_core` 的消息需组装为 **OpenAI 标准消息格式**：

```json
{
    "messages": [
        {
            "role": "user",
            "content": "你好"
        },
        {
            "role": "user",
            "content": [
                {
                    "type": "image_url",
                    "image_url": {
                        "url": "data:image/jpeg;base64,..."
                    }
                },
                {
                    "type": "image_url",
                    "image_url": {
                        "url": "https://example.com/image.jpg"
                    }
                },
                {
                    "type": "text",
                    "text": "描述一下图片内容！"
                }
            ]
        }
    ]
}
```

> `content` 可以是纯字符串（仅文本），也可以是数组（多模态消息）。

---

## 三、后端接口说明

### 1. 文件读取接口

**功能**：读取服务器上已保存的文件内容

| 项目     | 说明                             |
| -------- | -------------------------------- |
| 请求方式 | `GET`                            |
| 路径     | `/read/[文件路径]`               |
| 路径说明 | 文件路径相对于 `local_data` 目录 |

#### 响应

- **成功 (200 OK)**  
  - 自动设置正确的 `Content-Type`  
  - 响应体为文件二进制流

| 文件类型        | Content-Type             |
| --------------- | ------------------------ |
| 文本文件        | `text/plain`             |
| HTML/CSS/JS     | 对应类型                 |
| 图片            | `image/*`                |
| PDF             | `application/pdf`        |
| 其他            | `application/octet-stream` |

#### 错误响应

| 状态码 | 说明                   |
| ------ | ---------------------- |
| `400`  | 缺少文件名             |
| `403`  | 访问受限或路径非法     |
| `404`  | 文件不存在             |

#### 示例代码

```javascript
// 读取文本文件
fetch("/read/note.txt")
  .then(res => res.text())
  .then(content => console.log("文件内容:", content))
  .catch(error => console.error("读取文本文件失败:", error));

// 读取图片文件
fetch("/read/image.png")
  .then(res => res.blob())
  .then(blob => {
    const img = document.createElement("img");
    img.src = URL.createObjectURL(blob);
    document.body.appendChild(img);
  })
  .catch(error => console.error("读取图片失败:", error));

// 读取JSON文件
fetch("/read/data.json")
  .then(res => res.json())
  .then(data => console.log("JSON数据:", data))
  .catch(error => console.error("读取JSON文件失败:", error));
```

---

### 2. 文件保存接口

**功能**：接收客户端上传的文件并保存到服务器

| 项目     | 说明                               |
| -------- | ---------------------------------- |
| 请求方式 | `POST`                             |
| 路径     | `/save`                            |
| 请求体   | 文件二进制数据                     |

#### 请求头

| 字段名           | 必填 | 默认值 | 说明                                   |
| ---------------- | ---- | ------ | -------------------------------------- |
| `X-File-Name`    | 是   | -      | 文件名，必须经过 **Base64 编码**       |
| `X-Overwrite`    | 否   | false  | 是否覆盖已存在文件（`true` / `false`） |
| `Content-Length` | 是   | -      | 文件大小（字节）                       |

#### 成功响应示例 (200 OK)

```json
{
  "filename": "report.pdf",
  "path": "local_data/report.pdf",
  "overwrite": false,
  "size": 10240,
  "success": true
}
```

#### 处理流程

1. Base64 解码获取原始文件名（支持 Unicode）
2. 安全检查：防止路径遍历攻击
3. 检查文件是否已存在
4. 若存在且未设置覆盖，自动添加时间戳重命名
5. 创建目录并保存文件

#### 错误响应

| 状态码 | 说明                       |
| ------ | -------------------------- |
| `400`  | 缺少文件名或 Base64 解码失败 |
| `403`  | 非法路径或权限不足         |
| `413`  | 文件过大                   |
| `500`  | 服务器内部错误             |

```json
{
  "error": {
    "code": 400,
    "message": "文件名解码失败",
    "details": "invalid base64 encoding"
  }
}
```

#### 完整前端示例代码

```javascript
class FileUploader {
  // Base64编码文件名（支持中文）
  static encodeFileName(filename) {
    return btoa(unescape(encodeURIComponent(filename)));
  }

  // 上传文件
  static async uploadFile(file, overwrite = false) {
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

#### 命令行上传示例（curl）

```bash
curl -X POST http://localhost:36789/save \
  -H "X-File-Name: $(echo -n '报告.pdf' | base64)" \
  -H "X-Overwrite: false" \
  --data-binary "@./报告.pdf"
```

#### 注意事项

- 文件名支持所有 Unicode 字符（通过 Base64 编码传输）
- 默认情况下，重名文件会自动添加时间戳以避免覆盖
- 文件保存路径相对于 `local_data` 目录
- 建议单文件不超过 100MB（可在服务器配置中调整）

---

### 3. lunar_core HTTP 接口

`lunar_core` 服务通过 **WebSocket** 推送消息，并通过 HTTP 接口接收消息和视频 URL。

#### WebSocket 推送的消息格式

##### 上下文消息（响应/主动内容）

```json
{ 
    "type": "context", 
    "data": { 
        "type": "response/active", 
        "content": "..." 
    } 
}
```

##### 图片消息（Base64 数组）

```json
{ 
    "type": "image", 
    "data": { 
        "type": "image", 
        "images": ["base64..."] 
    } 
}
```

#### HTTP 接口 – 消息写入

- **URL**: `POST /message/batch`
- **说明**: 接收消息对象数组，写入 `adapters.UnreadContext` 队列。
- **请求体**:

```json
{
    "messages": [
        { "role": "user", "content": "..." }
    ]
}
```

- **响应**:

```json
{
    "success": true,
    "length": 当前队列长度
}
```

#### HTTP 接口 – 视频 URL 写入

- **URL**: `POST /videourl/batch`
- **说明**: 接收 URL 字符串数组，写入 `adapters.UnreadVideoUrl` 队列。
- **请求体**:

```json
{
    "urls": ["video_url1", "video_url2"]
}
```

- **响应**:

```json
{
    "success": true,
    "length": 当前队列长度
}
```

---

## 四、前端交互流程总结

1. **用户拖拽文件**到输入框 → 调用 `/save` 保存文件 → 获取文件访问 URL → 在输入框中显示引用标记（如 `[图片]` 或预览缩略图）。
2. **用户输入文本** + 引用文件 → 点击发送按钮。
3. 前端将消息组装为 OpenAI 标准格式：
   - 文本 → `type: "text"`
   - 图片 URL → `type: "image_url"`
   - 视频 URL → 可选择放入消息内容或单独通过 `/videourl/batch` 发送。
4. 发送到 `POST /write/message`或`POST /write/videourl`。
5. 通过 WebSocket 接收 `lunar_core` 的回复（`type: "context"` 或 `type: "image"`），实时更新界面。
6. 左下角按钮控制**消息记录模态框**的显示/隐藏，展示完整对话历史。

---

## 五、附录：推荐技术栈

- HTML5 + CSS3（玻璃态效果）
- JavaScript (ES6+)
- Live2D SDK（具体实现参考附件）
- WebSocket API（接收推送消息）
- Fetch API（调用 HTTP 接口）

---

> 本 README 遵循上一份文档的编写风格：结构化、表格化、保留所有代码示例，并优化了层次与可读性。