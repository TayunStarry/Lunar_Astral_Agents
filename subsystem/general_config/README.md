# 子系统——配置管理（general_config）

> 📚 代码级文档参见 [Code Wiki 04·公共子系统](../../docs/code-wiki/04-公共子系统.md)，入口 [Code Wiki 门户](../../docs/code-wiki/README.md)。

全局配置中枢模块，负责聚合所有命令行参数与 JSON 配置文件，为其他子系统提供统一的配置访问点。

---

## 功能概述

`general_config` 是平台的**基础设施层**，不实现业务逻辑，仅负责为其他子系统提供统一、集中的配置访问。

| 能力 | 说明 | 适用场景 |
|------|------|----------|
| 命令行参数聚合 | 所有启动参数集中定义与管理 | 通过启动命令按需定制服务行为 |
| JSON 配置覆盖 | 以 `lunar_config.json` 增量覆盖默认值 | 持久化保存常用配置，无需反复带参数 |
| 共享运行状态 | 维护模型就绪状态、端口映射、静态资源映射等 | 各子系统运行时读取统一状态 |

配置采用「命令行参数 + JSON 配置文件」双层体系：命令行参数为所有配置提供默认值，`lunar_config.json` 中的非空字段会覆盖默认值。配置文件在每次程序启动时重新读取，修改后重启即生效，无需重新编译。

---

## 运行与使用方式

- 指定基础端口启动：`.\Lunar_Astral.exe -basic-port 36800`
- 开发模式（启用调试日志）：`.\Lunar_Astral.exe -developer`
- 禁用扩散图像生成：`.\Lunar_Astral.exe -allow-diffusion=false`
- 覆盖核心智能体多模态模型服务地址：`.\Lunar_Astral.exe -agent-multimodal-url http://127.0.0.1:36789/v1`
- 配置文件 `lunar_config.json` 放置在 `{LocalDir}/` 下（默认可执行文件同目录的 `local_data/`）

命令行参数的完整清单、JSON 配置分组（`models`/`server`/`agent`/`memory`/`search`）及各项默认值见 [Code Wiki 04 §4.1](../../docs/code-wiki/04-公共子系统.md)，此处不重复。

---

## 依赖

- 仅使用 Go 标准库，无任何外部第三方依赖
- 配置所引用的 JSON 文件、TLS 证书、本地资源等存放在 `LocalDir` 指定目录（默认 `local_data/`）

---

## 常见问题

### Q: lunar_config.json 放在哪里？

放置在 `{LocalDir}/` 目录下。默认 `LocalDir` 为 `local_data`，即 `<exe所在目录>/local_data/lunar_config.json`。

### Q: 哪些配置只能在命令行中设置？

端口号、功能开关、图像参数、WebView 窗口等参数通过命令行设置；`lunar_config.json` 的 `models`/`server`/`agent`/`memory`/`search` 分组可覆盖对应的模型路径、功能开关及模型服务地址。

### Q: 配置修改后需要重新编译吗？

不需要。`lunar_config.json` 在每次启动时重新读取，命令行参数同样无需重新编译。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与编译流程
- [星图·月华](../../lunar_astral/README.md) —— 配置使用方
- [星图·琉璃](../../crystal_astral/README.md) —— 配置使用方
- [网页前端子系统](../browser_client/README.md) —— WebView 窗口参数使用