# 任务描述

## 概述

本项目实现一个智能体（QQ 机器人）适配器，负责在 Napcat（QQ 客户端接口）与 `lunar_core` 核心服务之间进行双向消息转发。适配器需处理多种消息类型（文本、图片、回复、@提及、转发消息、文件等），并将消息格式转换为 OpenAI 兼容的 JSON 格式。

---

## 一、参数说明

适配器在接收 Napcat 推送的消息时，会附带以下参数：

| 参数               | 说明                                                         |
| ------------------ | ------------------------------------------------------------ |
| `self_id`          | 当前智能体的 ID                                              |
| `user_id`          | 发送当前消息的用户的 ID                                      |
| `message_id`       | 当前消息的 ID                                                |
| `sender.role`      | 发送当前消息的用户在群组中的角色（例如 `"owner"`、`"member"`） |
| `group_id`         | 当前消息所属的群组 ID                                        |
| `message`          | 当前消息的内容（对象数组，具体格式见下文）                   |
| `sender.nickname`  | 发送当前消息的用户的昵称                                     |

### 参数处理规则

- `group_id` 必须在 `lunar_config.json` 中 `qq_adapter` → `listen_group_ids` 定义的群组 ID 列表中。
- 若 `user_id` 等于 `self_id`，则忽略该消息（不处理自己发送的消息）。

---

## 二、消息格式详解

`message` 字段是一个对象数组，数组中的每个元素代表一个消息段（segment）。以下列举所有可能的消息段类型及示例。

### 1. 纯文本消息

```json
"message": [
    {
        "type": "text",
        "data": {
            "text": "月华"
        }
    }
]
```

### 2. 回复 + 文本消息

```json
"message": [
    {
        "type": "reply",
        "data": {
            "id": "1949108027"
        }
    },
    {
        "type": "text",
        "data": {
            "text": "叫啦"
        }
    }
]
```

> `type: "reply"` 中的 `data.id` 是被回复消息的 ID。

### 3. 图片消息

```json
"message": [
    {
        "type": "image",
        "data": {
            "summary": "",
            "file": "A5D7EA810FA443F1F46EC19BB5CC9D14.png",
            "sub_type": 0,
            "url": "https://multimedia.nt.qq.com.cn/download?appid=1407&fileid=...",
            "file_size": "863849"
        }
    }
]
```

> `type: "image"` 中的 `data.url` 是图片的 URL。

### 4. @ 提及消息

```json
"message": [
    {
        "type": "at",
        "data": {
            "qq": "3970426755"
        }
    },
    {
        "type": "text",
        "data": {
            "text": " 现在几点了 "
        }
    }
]
```

> `type: "at"` 中的 `data.qq` 是被 @ 的用户的 ID。

### 5. 转发消息

```json
"message": [
    {
        "type": "forward",
        "data": {
            "id": "7633414336058823837"
        }
    }
]
```

> `type: "forward"` 中的 `data.id` 是转发消息的 ID（可通过 Napcat 接口获取详情）。

### 6. 文件消息

```json
"message": [
    {
        "type": "file",
        "data": {
            "file": "Lunar_Astral_Agents.code-workspace",
            "file_id": "/9d0e6df9-568e-4173-aed3-a5671dad77ae",
            "file_size": "259",
            "url": "https://gzc-download.ftn.qq.com/ftn_handler/..."
        }
    }
]
```

> - `data.file` – 文件名称  
> - `data.file_id` – 文件的 ID  
> - `data.file_size` – 文件大小（字节）  
> - `data.url` – 文件的下载 URL

---

## 三、Napcat HTTP 接口

适配器通过 Napcat 提供的 HTTP API 获取消息详情、历史记录及发送消息。

### 1. 获取群历史消息

```shell
curl --location --request POST '/get_group_msg_history' \
--header 'Authorization: Bearer <token>' \
--header 'Content-Type: application/json' \
--data-raw '{
    "group_id": 0,
    "message_seq": 0,
    "count": 20,
    "reverseOrder": false
}'
```

```json
{
    "status": "ok",
    "retcode": 0,
    "data": {
        "messages": [
            {
                "self_id": 0,
                "user_id": 0,
                "time": 0,
                "message_id": 0,
                "message_seq": 0,
                "real_id": 0,
                "real_seq": "string",
                "message_type": "string",
                "sender": {
                    "user_id": 0,
                    "nickname": "string",
                    "sex": "male",
                    "age": 0,
                    "card": "string",
                    "level": "string",
                    "role": "owner"
                },
                "raw_message": "string",
                "font": 0,
                "sub_type": "string",
                "message": [ {} ],
                "message_format": "string",
                "post_type": "string",
                "group_id": 0
            }
        ]
    },
    "message": "string",
    "echo": "string",
    "wording": "string",
    "stream": "stream-action"
}
```

### 2. 获取单条消息详情

```shell
curl --location --request POST '/get_msg' \
--header 'Authorization: Bearer <token>' \
--header 'Content-Type: application/json' \
--data-raw '{
    "message_id": 0
}'
```

```json
{
    "status": "ok",
    "retcode": 0,
    "data": {},
    "message": "string",
    "echo": "string",
    "wording": "string",
    "stream": "stream-action"
}
```

### 3. 获取转发消息详情

```shell
curl --location --request POST '/get_forward_msg' \
--header 'Authorization: Bearer <token>' \
--header 'Content-Type: application/json' \
--data-raw '{
    "message_id": 0
}'
```

```json
{
    "status": "ok",
    "retcode": 0,
    "data": {
        "messages": [ {} ]
    },
    "message": "string",
    "echo": "string",
    "wording": "string",
    "stream": "stream-action"
}
```

### 4. 获取图片消息详情

```shell
curl --location --request POST '/get_image' \
--header 'Authorization: Bearer <token>' \
--header 'Content-Type: application/json' \
--data-raw '{
    "file_id": "string",
    "file": "string"
}'
```

```json
{
    "status": "ok",
    "retcode": 0,
    "data": {
        "file": "string",
        "url": "string",
        "file_size": "string",
        "file_name": "string",
        "base64": "string"
    },
    "message": "string",
    "echo": "string",
    "wording": "string",
    "stream": "stream-action"
}
```

### 5. 发送群文本消息

```shell
curl --location --request POST '/send_group_msg' \
--header 'Authorization: Bearer <token>' \
--header 'Content-Type: application/json' \
--data-raw '{
    "group_id": "nulla cillum dolore et commodo",
    "message": [
        {
            "type": "text",
            "data": {
                "text": "计象最起派。主立制区手压手白。千织机。"
            }
        },
        {
            "type": "text",
            "data": {
                "text": "性件厂组山极历严形。"
            }
        },
        {
            "type": "text",
            "data": {
                "text": "北路都。头商题积物来拉。类市专。小情便记。"
            }
        }
    ]
}'
```

```json
{
    "status": "ok",
    "retcode": 0,
    "data": {
        "message_id": 696124706
    },
    "message": "",
    "wording": ""
}
```

### 6. 发送群图片消息

```shell
curl --location --request POST '/send_group_msg' \
--header 'Authorization: Bearer <token>' \
--header 'Content-Type: application/json' \
--data-raw '{
    "group_id": 0,
    "message": [
        {
            "type": "image",
            "data": {
                "path": "text",
                "thumb": "string",
                "name": "string",
                "file": "string",
                "url": "string",
                "summary": "string",
                "sub_type": 0
            }
        }
    ]
}'
```

```json
{
    "status": "ok",
    "retcode": 0,
    "data": {
        "message_id": 696124706
    },
    "message": "",
    "wording": ""
}
```

---

## 四、lunar_core HTTP 接口

`lunar_core` 服务通过 WebSocket 推送消息，并通过 HTTP 接口接收消息和视频 URL。

### 1. WebSocket 推送的消息格式

#### 上下文消息（响应/主动内容）

```json
{ 
    "type": "context", 
    "data": { 
        "type": "response/active", 
        "content": "..." 
    } 
}
```

#### 图片消息（Base64 数组）

```json
{ 
    "type": "image", 
    "data": { 
        "type": "image", 
        "images": ["base64..."] 
    } 
}
```

### 2. HTTP 接口 – 消息写入

- **URL**: `POST /write/message`
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

### 3. HTTP 接口 – 视频 URL 写入

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

## 五、项目开发目标与设计思路

### 整体架构

```
QQ客户端 <---> Napcat <---> 适配器 <---> lunar_core
                           (本服务)
```

适配器同时扮演两个角色：

1. **Napcat 消费者**：接收 Napcat WebSocket 推送的 QQ 消息，转换为 OpenAI 兼容格式，转发给 `lunar_core`。
2. **lunar_core 消费者**：接收 `lunar_core` WebSocket 推送的回复（文本/图片），通过 Napcat HTTP API 发送回 QQ 群。

### 目标 1：实现 Napcat → lunar_core 的消息推送

**触发条件**：收到 `napcat_ws_server` 推送的消息（即 QQ 群内的新消息）。

**处理流程**：

1. 解析 `message` 数组，依次处理每个消息段（segment）。
2. 根据 `type` 字段，进行如下转换：
   - **`text`**：直接提取 `data.text` 作为文本内容。
   - **`reply`**：调用 `/get_msg` 接口获取被回复消息的完整内容，将其作为上文一并打包（或作为引用内容）。
   - **`image`**：下载图片或直接使用 `url`，转换为 Base64 或 URL 格式（根据 `lunar_core` 要求）。
   - **`at`**：提取被 @ 的用户 ID，可转换为文本形式如 `@用户ID`。
   - **`forward`**：调用 `/get_forward_msg` 接口获取转发消息的内容，递归转换为文本描述。
   - **`file`**：提取文件名和下载链接，可转换为文本描述。
3. 将转换后的内容组装成 OpenAI 标准消息格式：

```json
{
    "messages": [
        {
            "role": "user",
            "content": "你好"
        },
        {
            "role": "assistant",
            "content": [
                {
                    "type": "image_url",
                    "image_url": {
                        "url": "data:image/jpeg;base64,..."
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

> 注意：适配器通常将 QQ 用户消息作为 `role: "user"` 发送；而 `lunar_core` 可能生成 `role: "assistant"` 的回复，此处仅展示格式示例。

4. 将组装好的 JSON 通过 HTTP POST 发送到 `http://localhost:36789/write/message`。

### 目标 2：实现 lunar_core → Napcat 的消息发送

**触发条件**：收到 `lunar_ws_server` 推送的消息（可能为文本或图片）。

**处理流程**：

1. 识别推送消息的类型：
   - **`type: "context"`**：`data.content` 为文本回复内容。
   - **`type: "image"`**：`data.images` 为 Base64 编码的图片数组。
2. 从 `lunar_config.json` 中读取 `qq_adapter.listen_group_ids`，**随机选择一个群组 ID** 作为发送目标。
3. 构造 Napcat 的 `/send_group_msg` 请求体：
   - 对于文本：生成 `{ "type": "text", "data": { "text": content } }`。
   - 对于图片：将 Base64 字符串上传或直接构造为 `image` 消息段（Napcat 支持 `base64` 字段或 `url` 字段）。
4. 调用 Napcat HTTP API 发送消息。

### 设计要点

- **消息完整性**：对于 `reply` 和 `forward` 类型，必须主动获取原始消息内容，避免上下文丢失。
- **图片处理**：Napcat 推送的图片带有 `url`，适配器可下载后转为 Base64 再发送给 `lunar_core`；反之，`lunar_core` 推送的 Base64 图片需还原为 Napcat 可识别的格式（如直接使用 `base64` 字段）。
- **随机群发**：为避免固定群组导致负载不均，采用随机选择 `listen_group_ids` 中的群组。
- **错误处理**：所有 HTTP/WebSocket 调用应有超时和重试机制，并记录日志。
- **队列缓冲**：`/write/message` 接口返回队列长度，适配器可据此控制发送速率，防止积压。

---

## 附录：完整配置示例（`lunar_config.json`）

```json
{
    "qq_adapter": {
        "listen_group_ids": [123456789, 987654321]
    }
}
```