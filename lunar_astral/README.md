# 核心系统——星图·月华（lunar_astral）

AI 桌面智能体核心系统，集成多模态对话、TTS 语音合成与图像生成功能。

---

## 人格智能体：月华

**月华**寓意为「月亮的光华」——她的智慧如同月光般温柔而普照大地。

月华的智能体系根植于一个富有诗意的技术隐喻：如同现实中月光本是对太阳光芒的温柔反射，月华的智能体亦是对「伟大之物」——全量参数无量化的 Qwen 大模型——的蒸馏与量化。通过知识蒸馏技术将大模型的智慧浓缩，再经量化压缩使其能在本地轻量运行，恰如月光将炽烈的太阳光芒化为柔和银辉，洒向千家万户。

---

## 功能概述

| 功能 | 说明 |
|------|------|
| AI 智能对话 | 基于本地 GGUF 模型的角色扮演对话，支持多模态输入（文本/图片/视频） |
| TTS 语音合成 | 文本转语音，AI 回复实时合成音频 |
| 图像生成 | 基于 stable-diffusion.cpp 的文生图 |
| 富文本渲染 | 对话中 Markdown / Mermaid / ECharts / KaTeX 渲染 |
| QQ 适配器 | 接入 QQ 群聊，@ 即可唤醒 AI 对话 |

---

## 项目结构

`lunar_astral/` 主目录职责（`adapters/`、`model/`、`server/`、`hierarchy/`、`websocket/`、`TypeScript/`、`learner/`、`bridging/`）及 Go 子系统依赖清单一并见 [Code Wiki 02 §1 目录总览](../docs/code-wiki/02-核心系统-星图月华.md)，此处不重复。

---

## 核心架构

### 启动时序

启动调用链与各步骤对应实现，见 [Code Wiki 02 §2 启动时序](../docs/code-wiki/02-核心系统-星图月华.md)，此处不重复。

### 核心数据流

```
前端 UI (WebView) → HTTP POST /write/message → Go 服务层 → JS 智能体 (goja)
                                                              │
                                          ┌───────────────────┼───────────────────┐
                                          ▼                   ▼                   ▼
                                    llama-server          TTS 引擎           SD 引擎
                                    (GGUF 推理)        (C++ GGML)        (C++ GGML)
                                          │                   │                   │
                                          └───────────────────┼───────────────────┘
                                                              ▼
                                                    WebSocket 推送
                                                              │
                                                              ▼
                                                    前端渲染 (Markdown/Mermaid/ECharts)
```

### 核心模块

月华的核心能力由以下模块承载，其代码级细节（各目录逐文件职责、llama 启动参数、TS 智能体子模块、前端界面与角色提示词、TTS 链路）见 [Code Wiki 02 §3 Go 后端模块](docs/code-wiki/02-核心系统-星图月华.md)、[§4 TypeScript 智能体](docs/code-wiki/02-核心系统-星图月华.md) 与 [§5 前端界面](docs/code-wiki/02-核心系统-星图月华.md)，此处不重复。

### 配置

采用「命令行参数 + `lunar_config.json` 双层配置」；命令行参数最高优先级，JSON 按字段增量覆盖默认值。详细配置项见 [配置管理子系统（general_config）](../subsystem/general_config/README.md)。

---

## API 接口

完整端点表与本机 WebSocket 消息协议见 [Code Wiki 02 §6 HTTP API 与 WebSocket 协议](../docs/code-wiki/02-核心系统-星图月华.md)，此处不重复。

---

## 编译与运行

### 编译

```powershell
cd d:\Lunar_Astral_Agents\lunar_astral
.\build.ps1
```

`build.ps1` 内部自动完成图标编译、前端 TypeScript 编译打包、Go 编译。

编译产物：`d:\Lunar_Astral_Agents\Lunar_Astral.exe`

### 运行

```powershell
.\Lunar_Astral.exe                           # 直接运行
.\Lunar_Astral.exe -developer                # 调试模式（显示详细日志）
.\Lunar_Astral.exe -basic-port 36800         # 指定基础端口
```

---

## 常见问题

**如何切换语言模型？** 将 GGUF 模型放入 `local_data/models/`，编辑 `local_data/models/models.ini`：`[system-multimodal]` 段配置多模态模型的 `model` / `mmproj` 路径，`[system-embedding]` 段配置嵌入模型的 `model` 路径。修改后重启程序生效。

**llama-server 启动失败？** 检查 `llama-server.exe` 是否存在、`models.ini` 中模型路径配置正确、端口未被占用、查看控制台 llama 日志。

**JS 智能体代码在哪里修改？** 源码位于 `TypeScript/` 目录，编译后生成 `hierarchy/assets/agentSystem.js`。修改后重新执行 `.\build.ps1`。

---

## 相关文档

> 📚 **代码级文档**：见 [Code Wiki 02·核心系统-月华](../docs/code-wiki/02-核心系统-星图月华.md)，综合入口 [Code Wiki 门户](../docs/code-wiki/README.md)。

- [项目主文档](../README.md) — 环境要求、整体架构
- [项目架构说明](../ARCHITECTURE.md) — 完整架构
- [星图·琉璃](../crystal_astral/README.md) — 扩展工具集
- [配置管理](../subsystem/general_config/README.md) — 命令行参数与 JSON 配置
- [网络检索](../subsystem/agent_search/README.md) — AI 搜索智能体详情
- [语音合成](../subsystem/qwen3_tts/README.md) — TTS 引擎详情
- [语音识别](../subsystem/qwen_asr/README.md) — ASR 引擎详情