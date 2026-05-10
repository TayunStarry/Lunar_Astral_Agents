# 星月智能 - 本地部署桌面端智能体

> 🌙 **星月智能**是一套基于 Go + TypeScript 构建的本地部署桌面端智能体系统，包含两位拟人化智能体——**月华**与**琉璃**，为用户提供自然语言交互、图像生成、文件管理等丰富功能。

---

## 📁 项目结构

```
Lunar_Astral_Agents/
├── luna_astral/          # 星图-月华 - 核心智能体
│   ├── server/           # HTTP服务模块
│   ├── model/            # 模型管理模块
│   ├── adapters/         # 适配器模块
│   └── hierarchy/        # 前端资源
├── crystal_astral/       # 星图-琉璃 - 扩展智能体
│   └── assets/           # 前端资源
├── subsystem/            # 通用子系统
│   ├── storage/          # 数据存储子系统
│   ├── config/           # 配置管理子系统
│   ├── browser/          # 浏览器集成子系统
│   ├── bridge_adapter/   # 桥接适配器（预留）
│   └── proxy/            # 代理子系统（预留）
└── local_data/           # 本地数据目录
    ├── models/           # AI模型文件
    ├── package/          # 扩展包
    └── audios/           # 音频资源
```

---

## 🎭 智能体介绍

### 星图·月华 (luna_astral)

> 俏皮可爱的邻家少女，隶属于星月智能的核心智能体。她拥有温暖耐心的性格，擅长处理复杂任务，是您最可靠的AI伙伴。

**核心能力**：
- 🧠 自然语言对话（支持多种LLM模型）
- 🎨 AI图像生成（Stable Diffusion）
- 🔊 TTS语音合成（Qwen3-TTS）
- 📁 文件管理与数据库操作
- 🎬 视频关键帧提取

### 星图·琉璃 (crystal_astral)

> 优雅灵动的扩展智能体，专注于应用管理与系统增强。她如同月华的姐妹，为整个系统提供扩展支持能力。

**核心能力**：
- 🖼️ 动态背景管理
- 🚀 应用快捷启动
- 📷 屏幕截图功能
- 📐 图片缩放处理

---

## 🚀 快速开始

### 环境要求

- **操作系统**: Windows 10/11 (64位)
- **Go版本**: >= 1.21
- **Node.js**: >= 20.x (用于TypeScript编译)

### 安装步骤

```powershell
# 1. 克隆项目
git clone https://github.com/LunarAstral/Lunar_Astral_Agents.git
cd Lunar_Astral_Agents

# 2. 构建月华智能体
cd luna_astral
.\build.ps1

# 3. 构建琉璃智能体
cd ../crystal_astral
.\build.ps1

# 4. 返回根目录运行
cd ..
.\build.ps1
```

### 运行方式

```powershell
# 启动月华核心服务
.\luna_astral\luna_astral.exe

# 启动琉璃扩展服务（可选）
.\crystal_astral\crystal_astral.exe
```

---

## 🏗️ 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                    用户界面层                              │
│  ┌──────────────┐    ┌──────────────┐                      │
│  │   WebView    │    │   API Client │                      │
│  └──────┬───────┘    └──────┬───────┘                      │
└─────────┼───────────────────┼───────────────────────────────┘
          │                   │
┌─────────▼───────────────────▼───────────────────────────────┐
│                    服务网关层                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              HTTP Server + WebSocket                │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────┬───────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────┐
│                    业务逻辑层                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │  Agent   │  │  Image   │  │  TTS     │  │  Video   │   │
│  │ Handler  │  │ Handler  │  │ Handler  │  │ Handler  │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
└─────────────────────────────┬───────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────┐
│                    基础设施层                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │ Storage  │  │  Config  │  │ Browser  │  │  Model   │   │
│  │ Subsystem│  │ Subsystem│  │ Subsystem│  │ Manager  │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## 📖 文档导航

| 文档 | 路径 | 说明 |
|------|------|------|
| **月华智能体文档** | [docs/luna_astral.md](docs/luna_astral.md) | 月华的完整技术文档 |
| **琉璃智能体文档** | [docs/crystal_astral.md](docs/crystal_astral.md) | 琉璃的完整技术文档 |
| **存储子系统文档** | [docs/subsystem/storage.md](docs/subsystem/storage.md) | 数据存储架构说明 |
| **配置子系统文档** | [docs/subsystem/config.md](docs/subsystem/config.md) | 配置管理机制 |
| **浏览器子系统文档** | [docs/subsystem/browser.md](docs/subsystem/browser.md) | 浏览器集成方案 |
| **预留智能体文档** | [docs/reserved_agents.md](docs/reserved_agents.md) | 姐妹智能体规划 |
| **项目计划** | [docs/project_plan.md](docs/project_plan.md) | 开发计划与进度 |

---

## 🤝 贡献指南

欢迎贡献代码！请遵循以下规范：

1. 代码风格：Go使用 `go fmt`，TypeScript使用 `prettier`
2. 提交信息：遵循 [Conventional Commits](https://www.conventionalcommits.org/)
3. PR流程：先创建Issue讨论，再提交PR

---

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

---

## 🌙 关于星月智能

星月智能致力于打造本地化、私有化的AI智能体系统，让每个用户都能拥有自己的专属AI伙伴。月华与琉璃，将陪伴您探索无限可能。

> *"在星空中寻找答案，在月光下创造美好"* 🌙✨