# 星月智能 —— 综合文档门户

> 本文档为项目的**统一文档中心**。所有模块（月华、琉璃、公共子系统、独立 AI 引擎、前端资源库、外部引擎参数）的文档集中于本目录，每个模块在此**有且只有一份**详细完整的文档。

---

## 文档地图

### Ⅰ. 产品 / 概览文档（仓库根目录）

| 文档 | 定位 | 内容 |
|------|------|------|
| [根 README](../../README.md) | 仓库入口 | 项目简介、人格智能体、环境要求、编译、FAQ |
| [RELEASE.md](../../RELEASE.md) | 发行版说明 | 版本信息、下载链接、运行环境 |
| [CONTRIBUTING.md](../../CONTRIBUTING.md) | 贡献指南 | 开发规范、PR 流程 |
| [docs/agents](../../docs/agents/) | 协作规范 | issue-tracking / triage / domain 约定 |

### Ⅱ. Code Wiki（代码理解侧，本目录）

| 文档 | 覆盖模块 | 内容 |
|------|----------|------|
| [01-项目架构总览](01-项目架构总览.md) | 全项目 | 人格智能体、整体架构图、文件夹结构、数据流、技术栈、前端共享资源机制 |
| [02-核心系统-钛宇-月华](02-核心系统-钛宇-月华.md) | lunar_astral | 月华功能、启动时序、Go 各目录逐文件函数表、TS 智能体、HTTP/WS 协议、FAQ |
| [03-扩展系统-钛宇-琉璃](03-扩展系统-钛宇-琉璃.md) | crystal_astral | 琉璃功能、启动/代理路由、逐文件函数表、内置智能体（Auto-LTP / Web-LTP / StarLTP / YaraLTP）、端点总览、运行与 FAQ |
| [04-公共子系统](04-公共子系统.md) | subsystem 基础模块 | general_config · browser_client · file_manager · multimodal_analysis · logger_general · lunar_decoder · lunar_goja · media_tools 及辅助工具的原理、关键文件与函数、运行方式、FAQ |
| [05-独立AI引擎与运维工具](05-独立AI引擎与运维工具.md) | subsystem 引擎/运维 | agent_search · qwen3_tts · qwen_asr · environment_repair 的推理管线、关键函数、编译运行、FAQ |
| [06-前端资源库](06-前端资源库.md) | local_data/package | standard_dependency 注入清单、自定义模块与关键类、扩展包总览 |
| [07-依赖关系](07-依赖关系.md) | 全项目 | Go 模块依赖图、三方库、外部工具、前后端通信端点、WebSocket 链路 |
| [08-构建运行与配置](08-构建运行与配置.md) | 全项目 | 环境要求、构建命令具体形态、双层配置结构、模型切换 |
| [09-LTPX协议-月华工具包](09-LTPX协议-月华工具包.md) | LTPX 生态 | 工具包协议、分支体系、包注册元数据、AtoA 调用链路、LTP3/LTP9 插件引擎 |
| [10-llama.cpp 参数参考](10-llama.cpp-参数参考.md) | 外部引擎 | llama-server 全部 CLI 参数速查 |
| [11-stable-diffusion.cpp 参数参考](11-stable-diffusion.cpp-参数参考.md) | 外部引擎 | sd-cli 全部 CLI 参数速查 |

---

## 阅读建议

- **刚接触项目**：读根 README → [01 架构总览](01-项目架构总览.md) 建立整体认识。
- **要改某模块代码**：直接进入 Code Wiki 对应章节（02/03/04/05/06）读代码级细节 → 按 [08](08-构建运行与配置.md) 编译验证。
- **排查问题 / 架构评审**：以 [07 依赖关系](07-依赖关系.md) 与 02/03 为准。
- **调整引擎参数**：见 [10](10-llama.cpp-参数参考.md) / [11](11-stable-diffusion.cpp-参数参考.md)。

---

*具体端口值与配置以运行时[配置](08-构建运行与配置.md)与代码为准。*
