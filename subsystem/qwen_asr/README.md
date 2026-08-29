# 独立系统——语音识别（qwen_asr）

> 📚 代码级文档参见 [Code Wiki 05·独立AI引擎与运维工具](../../docs/code-wiki/05-独立AI引擎与运维工具.md)，入口 [Code Wiki 门户](../../docs/code-wiki/README.md)。

基于 Qwen3-ASR 模型的本地语音识别（Automatic Speech Recognition）引擎，采用纯 C 语言推理引擎 + Go HTTP 服务的混合架构，支持 0.6B 和 1.7B 两种模型规模。

---

## 功能概述

Qwen ASR Lunar 是一个全本地化的语音识别引擎，支持多语言音频转录。

| 特性 | 说明 |
|------|------|
| 离线识别 | 纯本地运行，无需网络连接 |
| 多语言支持 | 支持中/英/粤/日/韩/法/德/西/俄等 30 种语言 |
| 双模型 | 自动检测 0.6B 或 1.7B 模型规模 |
| 高精度 BF16 | BF16 权重零拷贝推理，保持模型精度 |
| SIMD 加速 | AVX2/AVX-512/NEON 编译时自动选择 |
| 实时录音 | 浏览器端 MediaRecorder API 录音 |
| 音频格式自适应 | 非 WAV 格式自动通过 FFmpeg 转换 |

---

## 编译与运行

### 编译

```powershell
cd d:\Lunar_Astral_Agents\subsystem\qwen_asr

# 编译（可选 OpenBLAS 加速）
.\build.ps1
```

编译产物：`d:\Lunar_Astral_Agents\Qwen_ASR_Lunar.exe`

### 运行

```powershell
.\Qwen_ASR_Lunar.exe
```

程序自动打开 WebView 窗口（648×960），提供文件上传、浏览器录音与转写结果展示。

> 完整端点表、请求/响应格式、推理管线及内核实现见 [Code Wiki 05 §5.3](../../docs/code-wiki/05-独立AI引擎与运维工具.md)，此处不重复。

---

## 常见问题

### Q: 支持哪些音频格式？

推荐使用 **16kHz, 16-bit, mono, PCM WAV** 格式以获得最佳效果。其他格式（MP3、WebM、OGG、FLAC 等）会自动通过 FFmpeg 转换为标准格式。

### Q: 如何启用 OpenBLAS 加速？

编译时启用 OpenBLAS 标志并在链接路径中放置 OpenBLAS 库文件即可，具体编译参数见 [Code Wiki 05 §5.3](../../docs/code-wiki/05-独立AI引擎与运维工具.md)。

### Q: 0.6B 和 1.7B 模型如何选择？

引擎会自动检测模型目录中的 SafeTensors 文件，根据模型规模自动判断。0.6B 速度更快，1.7B 精度更高。

### Q: 为什么转写速度慢？

1. 启用 OpenBLAS 加速（编译时）
2. 使用 0.6B 模型替代 1.7B
3. 检查 CPU 是否有 AVX2 指令集支持
4. 长音频使用分段模式

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与编译流程
- [配置管理子系统](../general_config/README.md) —— 模型路径配置
- [语音合成独立系统](../qwen3_tts/README.md) —— TTS 文本转语音引擎
- [钛宇-月华](../../lunar_astral/README.md) —— ASR 引擎可集成使用方