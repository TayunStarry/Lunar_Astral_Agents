# 星月智能（Lunar Astral Agents）—— 发行版说明

**版本**：v2026.08.22
**适用平台**：Windows 10/11（64 位）

---

## 项目简介

星月智能是一个**纯本地化**的桌面 AI 智能体平台，整合多模态对话、图像生成、语音识别与语音合成等 AI 能力。无需 Python 环境或云端依赖，所有推理均在本地完成。

平台由两个主程序组成：**月华**（`Lunar_Astral.exe`，AI 对话核心）与**琉璃**（`Crystal_Astral.exe`，工具集扩展系统）。

---

## 项目组成

### 1. 钛宇-月华（lunar_astral）— AI 桌面智能体核心

**功能**：多模态 AI 角色对话（文本/图片/视频输入）、TTS 语音合成（Qwen3-TTS）、文生图（stable-diffusion.cpp）、富文本渲染（Markdown/Mermaid/ECharts/KaTeX）、智能网络搜索（agent_search）、QQ 群聊适配器（NapCat）。

**运行**：`.\Lunar_Astral.exe`（默认端口 36789）。

**依赖**：WebView2 Runtime（Win11 已预装），GGUF 格式语言模型。

---

### 2. 钛宇-琉璃（crystal_astral）— 工具集扩展系统

**功能**：文件管理、扩展包管理、SQLite 知识库、向量记忆库、多显示器截图标注、AI 代理转发（OpenAI 兼容 API）、内置 Qwen3-ASR 语音识别与 Kokoro 语音合成、LTPX 扩展包运行时（Mini/Self-LTP 页面智能体、Auto-LTP 桌面智能体、LTP3/LTP9 插件引擎）。

**运行**：`.\Crystal_Astral.exe`（默认端口 36792，自动打开 WebView 窗口），AI 请求自动代理到月华后端。

**依赖**：WebView2 Runtime。

---

### 3. 环境修复工具（environment_repair）— 运维工具箱

**功能**：资源补全修复、端口占用释放、HTTPS 反向代理（自动 TLS 证书）、分卷打包归档。

**运行**：`.\Environment_Repair.exe`，交互式终端菜单。

**依赖**：7z（仅打包功能需要）。

---

### 4. 搜索智能体（agent_search）— AI 网络搜索

**功能**：多引擎搜索（Bing/百度/搜狗），页面提取 → AI 摘要 → 相关性判定 → 报告生成 → 记忆存储，自动过滤字典网站。以 Go 库形式集成于月华。

**依赖**：Chrome/Edge 浏览器，多模态与嵌入模型 API。

---

### 5. 语音识别（qwen_asr）— Qwen3-ASR 引擎

**功能**：纯本地语音识别，支持 30 种语言，双模型规模（0.6B/1.7B）、SIMD 加速。同一引擎以两种形态提供：独立程序与琉璃内置模块。

**运行**：`.\Qwen_ASR_Lunar.exe`，自动打开录音界面并提供 HTTP API。

**依赖**：SafeTensors 模型文件；FFmpeg / OpenBLAS（可选）。

---

### 6. 语音合成（qwen3_tts）— Qwen3-TTS 引擎

**功能**：纯本地中文文本转语音，支持音色克隆、LRU 缓存，CUDA/Vulkan/Metal 多后端加速。

**运行**：`.\Qwen3_TTS_Lunar.exe`，后台 HTTP 服务（默认端口 36789）；月华主程序亦内置同一引擎并直接暴露 `/tts` 端点。

**依赖**：GGUF 模型文件、参考音频；CUDA Toolkit（可选）。

---

## 运行环境

| 配置项 | 最低要求 | 推荐配置 |
|--------|---------|---------|
| 操作系统 | Windows 10 21H2+（64 位） | Windows 11 |
| CPU | x86_64，支持 AVX2 | 4 核以上 |
| 内存 | 8 GB | 16 GB |
| 显卡 | 无要求（纯 CPU 可运行） | NVIDIA CUDA 12.x（显存 8GB+） |
| 磁盘 | 20 GB | SSD 40 GB+ |

**运行时依赖**：WebView2 Runtime（Win11 已预装）、NVIDIA CUDA（可选）、FFmpeg（可选）。

---

## 快速开始

1. 将 GGUF 模型放入 `local_data\models\`，语言模型编辑 `local_data\models\models.ini`，辅助模型/API 配置编辑 `local_data\lunar_config.json`。
2. 启动核心智能体：`.\Lunar_Astral.exe`
3. 启动扩展系统：`.\Crystal_Astral.exe`

---

## 命令行参数与配置

运行参数（如 `-developer` 调试模式、`-basic-port` 指定端口）与 `lunar_config.json` 的分组结构、各项默认值，详见代码文档 [Code Wiki 08 构建运行与配置](docs/code-wiki/08-构建运行与配置.md)。

---

## 下载

> 链接：[https://www.123865.com/s/soKjTd-Z8F7?pwd=klKs#](https://www.123865.com/s/soKjTd-Z8F7?pwd=klKs#)
> 提取码：**klKs**

---

## 常见问题

**启动失败**：确认 WebView2 Runtime 已安装、路径不含中文、`lunar_config.json` 配置正确。

**内存不足**：使用量化模型（Q4_K_M），关闭不需要的功能（`allow_diffusion: false`），使用较小模型。

**支持云端模型**：在 `lunar_config.json` 的 `agent` 分组，将 `multimodal_url` 指向 OpenAI 兼容云端 API 并填写 `multimodal_key`，月华模型代理层将请求转发至该云端 API。

---

## 致谢

本项目使用了以下开源项目：llama.cpp、stable-diffusion.cpp、qwen3-tts.cpp、qwen3-asr、GGML、Kokoro-82M、ONNX Runtime、WebView2。
