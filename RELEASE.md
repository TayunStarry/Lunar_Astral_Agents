# 星月智能（Lunar Astral Agents）—— 发行版说明

**版本**：v2026.08.10  
**适用平台**：Windows 10/11（64 位）

---

## 项目简介

星月智能是一个**纯本地化**的桌面 AI 智能体平台，整合多模态对话、图像生成、语音识别与合成等 AI 能力。无需 Python 环境或云端依赖，所有推理均在本地完成。

项目由两位人格智能体驱动：**月华**（AI 对话核心）与 **琉璃**（工具集扩展），二者相辅相成。

---

## 项目组成

### 1. 星图·月华（lunar_astral）— AI 桌面智能体核心

**功能**：多模态 AI 角色对话、TTS 语音合成、文生图、富文本渲染（Markdown/Mermaid/ECharts/KaTeX）、QQ 群聊适配器。

**技术栈**：Go 后端 + TypeScript 智能体（goja 运行时）+ WebView2 前端。底层依赖 llama.cpp 进行 GGUF 模型推理。

**运行**：`.\Lunar_Astral.exe`，可选 `-developer`（开发模式）、`-basic-port`（指定端口）等参数。

**依赖**：WebView2 Runtime（Win11 已预装），GGUF 格式模型文件。

---

### 2. 星图·琉璃（crystal_astral）— 工具集扩展程序

**功能**：文件管理、SQLite 数据库可视化 CRUD、多显示器截图标注、AI 代理转发（OpenAI 兼容 API）、外部应用加载器。

**技术栈**：Go 后端 + 原生 HTML/CSS/JS 前端（毛玻璃风格），复用 file_manager 和 image_processor 子系统。

**运行**：`.\Crystal_Astral.exe`，自动随机端口（10000~40000），WebView 窗口（1500×1050）。AI 请求自动代理到月华后端。

**依赖**：WebView2 Runtime。

---

### 3. 环境修复工具（environment_repair）— 运维工具箱

**功能**：资源补全修复（从内嵌资源释放缺失文件）、端口占用释放、HTTPS 反向代理（自动 TLS 证书 + WebSocket 隧道 + CORS）、分卷打包归档。

**技术栈**：Go 命令行程序，交互式终端菜单。

**运行**：`.\Environment_Repair.exe`，终端交互式菜单选择功能。

**依赖**：7z（仅打包功能需要）。

---

### 4. 搜索智能体（lunar_chromedp）— AI 网络搜索

**功能**：基于 Chromedp 的多引擎搜索（Bing/百度/搜狗），页面内容提取 → AI 摘要 → 深度搜索 → 记忆存储。自动过滤字典网站。

**技术栈**：Go 库，通过 Chromedp 控制浏览器，调用多模态模型进行视觉理解与摘要。

**依赖**：Chrome/Edge 浏览器，多模态模型 API 与嵌入模型 API。

---

### 5. 语音识别（qwen_asr_lunar）— Qwen3-ASR 引擎

**功能**：纯本地语音识别，支持中/英/粤/日/韩等 30 种语言。双模型规模（0.6B/1.7B），BF16 高精度推理，AVX2/AVX-512/NEON SIMD 加速。

**技术栈**：纯 C 推理引擎 + Go HTTP 服务（CGO 桥接），浏览器端 MediaRecorder API 录音。

**运行**：`.\Qwen_ASR_Lunar.exe`，自动打开 WebView 窗口（648×960），提供 HTTP API（`POST /asr`）。

**依赖**：SafeTensors 格式模型文件，FFmpeg（可选），OpenBLAS（可选加速）。

---

### 6. 语音合成（qwen3_tts）— Qwen3-TTS 引擎

**功能**：纯本地中文文本转语音，支持音色克隆（参考音频）、流式输出、LRU 缓存。支持 CUDA/Vulkan/Metal 多 GPU 后端加速。

**技术栈**：C++ GGML 推理引擎（DLL）+ Go HTTP/WebSocket 服务（CGO 调用）。

**运行**：`.\Qwen3_TTS_Lunar.exe`，默认端口 36365。提供 HTTP API（`POST /tts/`）和 WebSocket 流式接口。

**依赖**：GGUF 格式模型文件，参考音频文件，CUDA Toolkit（可选）。

---

## 运行环境

| 配置项 | 最低要求 | 推荐配置 |
|--------|---------|---------|
| 操作系统 | Windows 10 21H2+（64 位） | Windows 11 |
| CPU | x86_64，支持 AVX2 | 4 核以上 |
| 内存 | 8 GB | 16 GB |
| 显卡 | 无要求（纯 CPU 可运行） | NVIDIA CUDA 12.x（显存 4GB+） |
| 磁盘 | 10 GB | SSD 20 GB+ |

**运行时依赖**：WebView2 Runtime（Win11 已预装）、NVIDIA CUDA（可选）、FFmpeg（可选）。

---

## 快速开始

1. 将 GGUF 模型文件放入 `local_data\models\`，编辑 `local_data\lunar_config.json` 配置模型路径
2. 启动核心智能体：`.\Lunar_Astral.exe`
3. 启动扩展工具：`.\Crystal_Astral.exe`
4. 独立模块（可选）：`.\Qwen_ASR_Lunar.exe` / `.\Qwen3_TTS_Lunar.exe`

---

## 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-basic-port` | HTTP 服务端口 | 36789 |
| `-developer` | 开发模式（直读文件系统） | false |
| `-clear-port` | 启动前清理端口 | false |
| `-allow-diffusion` | 启用图像生成 | true |
| `-allow-multimodal` | 启用多模态模型 | true |

---

## 配置说明

核心配置项（`lunar_config.json`）：

```json
{
  "models": {
    "asr_model": "./local_data/models/Qwen3-ASR-0.6B",
    "diffusion_model": "./local_data/models/z_image_turbo-Q4_K.gguf",
    "prompt_analysis_model": "./local_data/models/Qwen3-4B-Instruct-Q4_K_M.gguf"
  },
  "server": {
    "developer": false,
    "allow_diffusion": true
  },
  "cloud": {
    "cloud_model_url": "",
    "multimodal_model_name": ""
  }
}
```

---

## 下载

> 链接：[https://www.123865.com/s/soKjTd-Z8F7?pwd=klKs#](https://www.123865.com/s/soKjTd-Z8F7?pwd=klKs#)  
> 提取码：**klKs**

---

## 常见问题

**启动失败**：确认 WebView2 Runtime 已安装、路径不含中文、`lunar_config.json` 配置正确。

**内存不足**：使用量化模型（Q4_K_M），关闭不需要的功能（`allow_diffusion: false`），使用较小模型。

**支持云端模型**：在 `lunar_config.json` 中配置 `cloud.cloud_model_url` 即可切换到 OpenAI 兼容 API。

---

## 致谢

本项目使用了以下开源项目：llama.cpp、stable-diffusion.cpp、qwen3-tts.cpp、qwen3-asr、GGML、WebView2。

> 🌟 月华和琉璃永远陪伴着大家 ~ 一起探索 AI 的无限可能吧！