# 流式 TTS WebSocket 接口文档

## 概述

`/tts/stream` 接口基于 WebSocket 协议提供流式文本转语音功能。客户端发送文本后，服务端会逐块返回 PCM 音频数据，实现边合成边播放的效果。

## 连接方式

```
ws://localhost:36365/tts/stream
```

## 数据交互流程

1. 客户端建立 WebSocket 连接
2. 客户端发送 JSON 格式的请求消息
3. 服务端持续推送音频数据块
4. 服务端发送最终完成消息
5. 连接保持开放或关闭

## 请求格式

### 请求消息（JSON）

```json
{
  "text": "你好，世界！",
  "ref_audio": "/path/to/reference.wav",
  "language_id": 2055,
  "chunk_frames": 50
}
```

### 请求参数

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| text | string | 是 | - | 需要合成的文本内容 |
| ref_audio | string | 否 | 系统预设 | 参考音频文件路径（WAV 格式，24kHz 单声道） |
| language_id | int32 | 否 | 2055 | 语言 ID（2050=英文，2055=中文，2058=日文等） |
| chunk_frames | int32 | 否 | 50 | 每个音频块包含的帧数（50帧≈1秒音频） |

## 响应格式

服务端通过 WebSocket 推送三种类型的 JSON 消息：

### 1. 音频数据块（audio_chunk）

```json
{
  "type": "audio_chunk",
  "audio": "base64编码的PCM数据",
  "total_samples": 24000,
  "sample_rate": 24000
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| type | string | 消息类型，固定为 "audio_chunk" |
| audio | string | Base64 编码的 PCM16 音频数据（小端序，16-bit，单声道） |
| total_samples | int | 累计已发送的采样数 |
| sample_rate | int | 采样率，固定为 24000 Hz |

### 2. 完成消息（final）

```json
{
  "type": "final",
  "total_samples": 48000,
  "sample_rate": 24000,
  "is_final": true
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| type | string | 消息类型，固定为 "final" |
| total_samples | int | 总采样数 |
| sample_rate | int | 采样率 |
| is_final | bool | 固定为 true |

### 3. 错误消息（error）

```json
{
  "type": "error",
  "error": "错误描述信息"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| type | string | 消息类型，固定为 "error" |
| error | string | 错误描述 |

## 音频参数

| 参数 | 值 |
|------|-----|
| 采样率 | 24000 Hz |
| 位深 | 16-bit |
| 声道 | 单声道（Mono） |
| 编码 | PCM（脉冲编码调制） |
| 字节序 | 小端序（Little-Endian） |
| 每帧采样数 | 480 samples（20ms） |

## 客户端示例

### JavaScript 实现

```javascript
const ws = new WebSocket('ws://localhost:36365/tts/stream');

const audioContext = new AudioContext({ sampleRate: 24000 });
let audioQueue = [];
let isPlaying = false;

ws.onopen = function() {
  // 发送合成请求
  ws.send(JSON.stringify({
    text: "你好，欢迎使用流式语音合成！",
    language_id: 2055,
    chunk_frames: 50
  }));
};

ws.onmessage = function(event) {
  const response = JSON.parse(event.data);

  switch (response.type) {
    case 'audio_chunk':
      // 解码 Base64 并播放
      const pcmData = atob(response.audio);
      const audioBuffer = audioContext.createBuffer(1, pcmData.length / 2, 24000);
      const channelData = audioBuffer.getChannelData(0);
      
      for (let i = 0; i < pcmData.length; i += 2) {
        const sample = pcmData.charCodeAt(i) | (pcmData.charCodeAt(i + 1) << 8);
        channelData[i / 2] = sample / 32768.0;
      }
      
      playAudioBuffer(audioBuffer);
      break;

    case 'final':
      console.log('合成完成，总采样数:', response.total_samples);
      break;

    case 'error':
      console.error('合成错误:', response.error);
      break;
  }
};

function playAudioBuffer(buffer) {
  const source = audioContext.createBufferSource();
  source.buffer = buffer;
  source.connect(audioContext.destination);
  source.start();
}

ws.onerror = function(error) {
  console.error('WebSocket 错误:', error);
};

ws.onclose = function() {
  console.log('连接已关闭');
};
```

### Python 实现

```python
import asyncio
import websockets
import json
import base64
import numpy as np
import sounddevice as sd

async def stream_tts(text, language_id=2055, chunk_frames=50):
    uri = "ws://localhost:36365/tts/stream"
    
    async with websockets.connect(uri) as ws:
        # 发送请求
        request = {
            "text": text,
            "language_id": language_id,
            "chunk_frames": chunk_frames
        }
        await ws.send(json.dumps(request))
        
        # 接收音频数据块
        async for message in ws:
            response = json.loads(message)
            
            if response["type"] == "audio_chunk":
                # 解码 PCM 数据并播放
                pcm_bytes = base64.b64decode(response["audio"])
                samples = np.frombuffer(pcm_bytes, dtype=np.int16) / 32768.0
                sd.play(samples, samplerate=24000, blocking=False)
                
            elif response["type"] == "final":
                print(f"合成完成，总采样数: {response['total_samples']}")
                break
                
            elif response["type"] == "error":
                print(f"合成错误: {response['error']}")
                break

# 使用示例
asyncio.run(stream_tts("你好，世界！"))
```

## 性能说明

| 指标 | 值 |
|------|-----|
| 每帧延迟 | 约 20-50ms（取决于硬件） |
| 每块延迟 | 约 1 秒（50 帧） |
| 首包延迟 | 约 1-3 秒（取决于文本长度和硬件） |
| 实时率 | 约 0.3-0.5x（CPU），0.5-0.8x（GPU） |

## 错误处理

| 错误场景 | 处理方式 |
|----------|----------|
| 文本为空 | 返回 error 消息 |
| 参考音频不存在 | 返回 error 消息 |
| TTS 引擎未初始化 | 返回 error 消息 |
| 合成过程中断 | 推送已生成的音频后发送 error 消息 |
| WebSocket 连接断开 | 自动清理流式上下文 |

## 注意事项

1. 每次连接只能处理一次合成请求
2. 客户端应准备 `AudioContext` 以处理流式播放
3. 建议 `chunk_frames` 设置为 50（约 1 秒音频），平衡延迟和播放流畅度
4. PCM 数据需要按小端序 16-bit 格式解码
5. 采样率固定为 24000 Hz
6. 如果客户端不需要继续接收数据，可以直接关闭 WebSocket 连接
