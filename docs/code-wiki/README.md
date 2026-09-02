# 星月智能 —— 综合文档门户

> 本页是两套文档系统的统一入口：
> - **① 产品 / 架构文档**（面向使用与整体认识）：根 [README.md](../../README.md)、[ARCHITECTURE.md](../../ARCHITECTURE.md)、各主程序与子系统 README、[docs/agents](../../docs/agents/)。
> - **② Code Wiki**（面向代码理解，本目录）：按模块深入剖析关键类与函数、依赖与运行。
>
> 两套文档互相超链接、查漏补缺，共同构成一套完整联通的综合文档体系。

---

## 文档地图

### Ⅰ. 产品 / 架构文档（概览侧）

| 文档 | 相对 code-wiki 定位 | 内容 |
|------|------|------|
| [根 README](../../README.md) | 总览 → [01](01-项目架构总览.md) | 项目简介、环境要求、编译、子系统导航 |
| [ARCHITECTURE.md](../../ARCHITECTURE.md) | 架构图 → [01](01-项目架构总览.md) | 系统分层、模块职责、依赖图 |
| [月亮核心 README](../../lunar_astral/README.md) | → [02](02-核心系统-钛宇-月华.md) | 月华启动时序、模块、API |
| [琉璃扩展 README](../../crystal_astral/README.md) | → [03](03-扩展系统-钛宇-琉璃.md) | 琉璃功能与代理路由 |
| [general_config](../../subsystem/general_config/README.md) | → [04](04-公共子系统.md) | 全局配置中枢 |
| [browser_client](../../subsystem/browser_client/README.md) | → [04](04-公共子系统.md) | WebView 窗口 |
| [file_manager](../../subsystem/file_manager/README.md) | → [04](04-公共子系统.md) | 文件/知识库/记忆库 |
| [image_processor](../../subsystem/image_processor/README.md) | → [04](04-公共子系统.md) | 图像/截图/关键帧 |
| [logger_general](../../subsystem/logger_general/README.md) | → [04](04-公共子系统.md) | 彩色终端日志 |
| [agent_search](../../subsystem/agent_search/README.md) | → [05](05-独立AI引擎与运维工具.md) | 智能网络检索 |
| [qwen3_tts](../../subsystem/qwen3_tts/README.md) | → [05](05-独立AI引擎与运维工具.md) | 语音合成 |
| [qwen_asr](../../subsystem/qwen_asr/README.md) | → [05](05-独立AI引擎与运维工具.md) | 语音识别 |
| [environment_repair](../../subsystem/environment_repair/README.md) | → [05](05-独立AI引擎与运维工具.md) | 运维工具箱 |
| [docs/agents](../../docs/agents/) | 规范侧（issue-tracking/triage/domain） | 团队协作与发版规范 |

### Ⅱ. Code Wiki（代码理解侧）

| 文档 | 内容 | 相关源文档 |
|------|------|-----------|
| [01-项目架构总览](01-项目架构总览.md) | 架构入口、前端共享资源机制、章节导航 | [README](../../README.md)、[ARCHITECTURE](../../ARCHITECTURE.md) |
| [02-核心系统-钛宇-月华](02-核心系统-钛宇-月华.md) | Go 后端 + TS 智能体 | [lunar_astral/README](../../lunar_astral/README.md) |
| [03-扩展系统-钛宇-琉璃](03-扩展系统-钛宇-琉璃.md) | 工具集成扩展 + 代理路由 | [crystal_astral/README](../../crystal_astral/README.md) |
| [04-公共子系统](04-公共子系统.md) | 五基础子系统 | [subsystem/*/README](../../subsystem/) |
| [05-独立AI引擎与运维工具](05-独立AI引擎与运维工具.md) | TTS/ASR/搜索/运维 | [subsystem/*/README](../../subsystem/) |
| [06-前端资源库](06-前端资源库.md) | standard_dependency 与自定义模块 | [package/](../../local_data/package/) |
| [07-依赖关系](07-依赖关系.md) | Go 模块图、三方库、端点 | 各 [go.mod](../../lunar_astral/go.mod) |
| [08-构建运行与配置](08-构建运行与配置.md) | 环境、编译、运行、配置 | [README](../../README.md)、[lunar_config.json](../../local_data/lunar_config.json) |
| [09-LTPX协议-月华工具包](09-LTPX协议-月华工具包.md) | 工具包协议、分支与版本演进、AtoA 调用链路 | [ltpx_remote.go](../../lunar_astral/adapters/ltpx_remote.go)、LTPX/Mini-LTP 包 |

---

## 阅读建议

- **刚接触项目**：读 ① 的根 README + ARCHITECTURE → 再进 Code Wiki 01 架构总览。
- **要改某模块代码**：从 ① 的对应子系统 README 了解意图 → 进入 Code Wiki 对应章节读代码级细节 → 改完按 [08](08-构建运行与配置.md) 编译验证。
- **排查问题 / 架构评审**：以 Code Wiki 07 依赖关系 与 02/03 为准。

---

*本门户随两套文档同步维护：Code Wiki 面向代码，产品 README 面向使用。产品级速览（三大程序说明、核心端口、环境要求、编译产物等）请以其上对等的产品文档为准，本页不再复述；具体端口值以运行时[配置](08-构建运行与配置.md)与代码为准。*