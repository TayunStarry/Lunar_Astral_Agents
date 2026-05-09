# 图像生成 API

本文档介绍图像生成相关的 API 接口。

## 创建图像生成任务

创建图像生成任务并加入队列。

- **路径**: `/generate`
- **方法**: `POST`

### 请求体

```json
{
    "prompt": "a cute cat sitting on a windowsill, sunset, anime style",
    "negative_prompt": "low quality, blurry, ugly",
    "batch_size": 1,
    "width": 512,
    "height": 512,
    "steps": 20,
    "strength": 0.75,
    "cfg_scale": 7.5,
    "seed": 12345,
    "init_img": null
}
```

### 请求参数说明

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| prompt | string | 是 | - | 正向提示词 |
| negative_prompt | string | 否 | 空 | 负面提示词 |
| batch_size | int | 否 | 1 | 批处理数量 |
| width | int | 否 | 512 | 图像宽度 |
| height | int | 否 | 512 | 图像高度 |
| steps | int | 否 | 20 | 采样步数 |
| strength | float | 否 | 0.75 | 图生图强度 (0-1) |
| cfg_scale | float | 否 | 7.5 | CFG 规模 |
| seed | int64 | 否 | 随机 | 随机种子 |
| init_img | string | 否 | null | 初始图像路径（图生图模式） |

### 响应示例

```json
{
    "status": "queued",
    "message": "任务已加入队列",
    "task_id": "task_1734567890123456789",
    "queue_pos": 1
}
```

## 等待生成完成

轮询等待图像生成任务完成（SSE 实时推送）。

- **路径**: `/generate/wait`
- **方法**: `GET`

### 查询参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| task_id | string | 是 | 任务ID |

### 响应示例（完成时）

```json
{
    "task_id": "task_1734567890123456789",
    "status": "completed",
    "result": "local_data/images/generated/20260102_150405.png",
    "read_path": "/read/local_data/images/generated/20260102_150405.png"
}
```

### 响应示例（失败时）

```json
{
    "task_id": "task_1734567890123456789",
    "status": "failed",
    "error": "生成失败: 显存不足"
}
```

## 视频关键帧提取

从视频中提取关键帧。

- **路径**: `/extract/keyframes`
- **方法**: `POST`

### 请求体

```json
{
    "video_path": "videos/sample.mp4",
    "frame_count": 10,
    "output_dir": "images/keyframes"
}
```

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回 API 索引](./index.md) | [下一篇：消息队列](./message.md)
