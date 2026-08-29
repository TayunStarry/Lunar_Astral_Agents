# 独立系统——语音合成（qwen3_tts）

> 📚 代码级文档参见 [Code Wiki 05·独立AI引擎与运维工具](../../docs/code-wiki/05-独立AI引擎与运维工具.md)，入口 [Code Wiki 门户](../../docs/code-wiki/README.md)。

基于 Qwen3-TTS 模型的本地文本转语音（Text-to-Speech）引擎，采用 C++ GGML 推理后端 + Go HTTP 服务的混合架构。

---

## 功能概述

Qwen3-TTS Lunar 是一个全本地化的语音合成引擎，支持将中文文本转换为自然流畅的语音输出。

| 特性 | 说明 |
|------|------|
| 文本转语音 | 支持中文文本合成 |
| 音色克隆 | 通过参考音频（reference audio）控制合成音色 |
| GPU 加速 | 通过 CUDA、Vulkan、Metal 等多后端加速推理 |
| 本地运行 | 纯 C++/Go 实现，无需 Python 环境 |
| 纯后端服务 | 仅提供 HTTP API，无前端界面 |
| 音频缓存 | LRU + singleflight 缓存机制，避免重复合成 |

---

## 编译与运行

### 编译

```powershell
cd d:\Lunar_Astral_Agents\subsystem\qwen3_tts
.\build.ps1
```

`build.ps1` 是**一站式构建入口**，自动按顺序完成三个阶段：编译 GGML 张量计算库 → 编译 Qwen3-TTS C++ 推理引擎 → 编译 Go 服务层（`build_ggml.ps1` 与 `build_cpp.ps1` 为内部脚本，由 `build.ps1` 自动调用，无需手动执行）。

可选参数：

```powershell
.\build.ps1 -SkipGGML        # 跳过 GGML 编译（已编译过时使用）
.\build.ps1 -SkipCPP         # 跳过 C++ 编译
.\build.ps1 -SkipGo          # 跳过 Go 编译
.\build.ps1 -Clean           # 清理后重新编译
.\build.ps1 -BuildType Debug # Debug 模式编译
```

编译产物：`d:\Lunar_Astral_Agents\Qwen3_TTS_Lunar.exe`

### 运行要求

1. 确保模型文件（GGUF 格式）放置在 `{LocalDir}/models/` 目录
2. 确保参考音频文件存在（默认 `{LocalDir}/audios/lunar-template.wav`）

### 运行

```powershell
.\Qwen3_TTS_Lunar.exe
```

程序启动后作为后台 HTTP 服务运行，通过 `http://localhost:36789` 提供 TTS API 接口（默认端口 `-basic-port` 可改）。

> 完整端点表、请求/响应格式及缓存机制见 [Code Wiki 05 §5.2](../../docs/code-wiki/05-独立AI引擎与运维工具.md)，此处不重复。

---

## 常见问题

### Q: 模型文件在哪里下载？

Qwen3-TTS 的 GGUF 格式模型可从 Hugging Face 或 ModelScope 获取。将下载的 `.gguf` 文件放入 `{LocalDir}/models/` 目录即可。

### Q: 如何获得更好的音色克隆效果？

使用高质量的参考音频（建议 16kHz 或 24kHz，单声道，PCM WAV 格式），时长建议 5-15 秒，内容涵盖丰富的语音特征。

### Q: CUDA 加速如何启用？

在编译 GGML 时启用 CUDA 支持，确保系统已安装 CUDA Toolkit 12.x 或 13.x。编译完成后引擎会自动检测并使用 GPU。

### Q: 生成速度慢怎么办？

1. 启用 GPU 加速（CUDA/Vulkan）
2. 减少单次生成长度
3. 使用量化模型（Q4_K_M 等）减少推理计算量

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与编译流程
- [配置管理子系统](../general_config/README.md) —— 模型路径配置
- [钛宇-月华](../../lunar_astral/README.md) —— TTS 引擎的集成使用方
- [语音识别独立系统](../qwen_asr/README.md) —— ASR 语音转文本引擎