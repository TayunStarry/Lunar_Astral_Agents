# 星月智能 - 本地部署桌面端智能体

> 🌙 **星月智能**是一套基于 Go + TypeScript 构建的本地部署桌面端智能体系统，包含两位已实现的拟人化智能体——**月华**与**琉璃**，以及规划中的**蔷薇**，为用户提供自然语言交互、图像生成、文件管理等丰富功能。

## 📁 项目结构

```
Lunar_Astral_Agents/
├── luna_astral/          # 星图-月华 - 核心智能体
│   ├── server/          # HTTP服务模块
│   ├── model/           # 模型管理模块
│   ├── adapters/        # JavaScript运行时环境（goja）与适配器函数
│   ├── hierarchy/       # 前端资源（含agentSystem.js）
│   ├── server_side/     # TypeScript服务端逻辑
│   ├── control/         # 流程控制模块（延迟/限流/计划）
│   └── release/         # 进程管理模块（端口释放/进程终止）
├── crystal_astral/       # 星图-琉璃 - 扩展智能体
│   └── assets/          # 前端资源
├── subsystem/            # 通用子系统
│   ├── storage/         # 数据存储子系统（含module/server双架构）
│   ├── config/          # 配置管理子系统
│   ├── browser/         # 浏览器集成子系统
│   ├── screenshot/      # 截图子系统
│   ├── LunarTick/       # 编程语言解释器
│   ├── project_archiving/# 项目归档子系统
│   ├── webp/            # WebP图像处理
│   ├── bridge_adapter/  # 桥接适配器
│   └── proxy/           # 代理子系统
└── local_data/          # 本地数据目录
    ├── models/          # AI模型文件
    ├── package/         # 扩展包
    └── audios/          # 音频资源
```

**子系统文档：**

- [数据存储子系统](docs/subsystem/storage.md)
- [配置管理子系统](docs/subsystem/config.md)
- [浏览器集成子系统](docs/subsystem/browser.md)
- [截图子系统](docs/subsystem/screenshot.md)
- [编程语言解释器](docs/subsystem/LunarTick.md)
- [项目归档子系统](docs/subsystem/project_archiving.md)
- [WebP图像处理](docs/subsystem/webp.md)
- [桥接适配器](docs/subsystem/bridge_adapter.md)
- [代理子系统](docs/subsystem/proxy.md)
- [扩展包](docs/package/index.md)

---

## 🎭 智能体介绍

### 星图·月华 (luna_astral) 

> 俏皮可爱的邻家少女，隶属于星月智能的核心智能体。她拥有温暖耐心的性格，擅长处理复杂任务，是您最可靠的AI伙伴。月华是琉璃的姐姐。详细信息请参阅[星图·月华 文档](docs/luna_astral.md)。

**核心能力**：

- 🧠 自然语言对话（支持多种LLM模型）
- 🎨 AI图像生成（Stable Diffusion）
- 🔊 TTS语音合成（Qwen3-TTS）
- 📁 文件管理与数据库操作
- 🎬 视频关键帧提取

### 星图·琉璃 (crystal_astral) 

> 优雅灵动的扩展智能体，专注于应用管理与系统增强。她是月华的妹妹，为整个系统提供扩展支持能力。详细信息请参阅[星图·琉璃 文档](docs/crystal_astral.md)。

**核心能力**：

- 🖼️ 动态背景管理
- 🚀 应用快捷启动
- 📷 屏幕截图功能
- 📐 图片缩放处理

### 蔷薇 - 规划中

> 星月智能的新姐妹，目前处于规划阶段。她将整合鉴权、代理和多应用适配等综合职能，成为系统的安全守护者和网络桥梁。详细规划请参阅[星图·蔷薇 文档](docs/reserved_agents.md)。

**预计核心能力**：

- 🔐 用户身份认证与授权管理
- 🌐 统一API网关与请求代理
- 🔄 多应用适配与协议转换

---

## 🚀 快速开始

### 环境要求

#### 操作系统支持
- **操作系统**: Windows 10/11 (64位)

#### 开发环境
- **Go版本**: >= 1.24
- **Node.js**: >= 20.x (用于TypeScript编译)

#### 必须安装的运行时依赖
- [CUDA](https://developer.nvidia.com/cuda-downloads) 12 或 13 版本
- [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)
- [FFmpeg](https://ffmpeg.org/download.html)
- [CUDA Toolkit](https://developer.nvidia.com/cuda-toolkit)

### 运行时配置要求

#### TTS语音合成配置
- **Qwen TTS**（选择性启用）：GPU推理，需4GB显存
- **MOSS TTS**（选择性启用）：CPU推理，需2GB内存

#### 图像生成配置
- **图片生成功能**（选择性启用）：GPU推理，需8GB及以上显存

#### LLM模型配置
- **禁用本地LLM推理时**：内存1GB以上
  - ⚠️ 配置说明：请在配置文件中正确设置云端模型的API地址与对应的API密钥
- **启用本地LLM推理时**：
  - 推荐配置：内存32GB以上，显存12GB以上
  - 最低理想配置：内存16GB，显存8GB

### 安装步骤

```powershell
# 1. 克隆项目
git clone https://github.com/LunarAstral/Lunar_Astral_Agents.git
cd Lunar_Astral_Agents

# 2. 构建月华智能体
cd luna_astral
.\build.ps1

# 3. 构建星图·琉璃
cd ../crystal_astral
.\build.ps1

# 4. 返回根目录运行
cd ..
.\build.ps1
```

### 运行方式

```powershell
# 启动星图·月华核心服务
.\luna_astral\luna_astral.exe

# 启动星图·琉璃扩展服务（可选）
.\crystal_astral\crystal_astral.exe
```

---

## 🏗️ 系统架构

![架构图](image/架构图-整体架构.webp)

---

## 🤝 贡献指南

欢迎贡献代码！请遵循以下规范：

### 代码风格规范

#### Go 代码规范

**文件命名**
- 使用小写字母，单词之间用下划线分隔：`main.go`, `create.go`, `server_side.go`
- 类型定义文件：`type.go`
- 工具函数文件：`utils.go`, `helper.go`

**命名约定**
- **包名**：全部小写，无下划线，简洁且具描述性
- **函数名**：帕斯卡命名法（PascalCase），动词开头
  - 示例：`StartServer()`, `InitializeComponents()`, `GetLocalIP()`
- **变量名**：驼峰命名法（camelCase）
  - 示例：`server`, `config`, `httpMux`
- **常量**：全部大写，单词之间用下划线分隔
  - 示例：`MAX_ATTEMPTS`, `DEFAULT_PORT`
- **接口名**：以 `er` 结尾
  - 示例：`Handler`, `Reader`, `Writer`

**代码格式**
- 使用 `go fmt` 自动格式化
- 每行不超过 120 字符
- 使用 `gofmt` 检查格式
- 注释使用 `//`，包级注释使用 `/* */`

**最佳实践**
- 错误处理：始终检查并正确处理错误
- 日志记录：使用 `log.Printf()` 进行日志输出
- 避免全局变量：优先使用参数传递
- 函数单一职责：每个函数只做一件事

#### TypeScript/JavaScript 代码规范

**文件命名**
- 使用小写字母，单词之间用下划线分隔：`index.ts`, `agent.ts`, `config.ts`
- 工具函数文件：`utils.ts`, `helper.ts`

**命名约定**
- **类名**：帕斯卡命名法（PascalCase）
  - 示例：`LunarAgent`, `AgentDefine`, `ChatCache`
- **接口名**：帕斯卡命名法（PascalCase），以 `I` 前缀（可选）
  - 示例：`Config`, `ProxyFetchConfig`, `TaskStatus`
- **函数/方法名**：驼峰命名法（camelCase）
  - 示例：`createChatMessage()`, `writeMessage()`, `pullExternalMessages()`
- **变量/属性名**：驼峰命名法（camelCase）
  - 示例：`speakWeight`, `unreadContext`, `finalResponse`
- **常量**：全部大写，单词之间用下划线分隔
  - 示例：`MAX_RETRY`, `DEFAULT_TIMEOUT`

**代码格式**
- 使用 `prettier` 自动格式化
- 每行不超过 120 字符
- 使用 TypeScript 严格模式
- 接口和类型定义使用 JSDoc 风格注释

**最佳实践**
- 使用 `async/await` 处理异步操作
- 使用 `interface` 定义数据结构
- 合理使用 TypeScript 类型推断
- 避免 `any` 类型，使用更具体的类型

#### HTML/CSS 代码规范

**文件命名**
- 使用小写字母，单词之间用下划线分隔：`index.html`, `style.css`, `script.js`

**命名约定**
- **CSS 类名**：使用连字符分隔（kebab-case）
  - 示例：`main-container`, `chat-message`, `btn-primary`
- **HTML ID**：使用驼峰命名法或连字符分隔
  - 示例：`chatContainer`, `userInput`

### 提交信息规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/) 格式：

```
<类型>(<范围>): <描述>

<正文>

<页脚>
```

**类型说明**

| 类型 | 说明 |
|------|------|
| `feat` | 新增功能 |
| `fix` | 修复 bug |
| `docs` | 文档更新 |
| `style` | 代码风格（不影响代码运行的变动） |
| `refactor` | 代码重构（既不新增功能也不修复 bug） |
| `test` | 测试相关 |
| `chore` | 构建/依赖/工具更新 |

**示例**

```
feat(server): 添加 WebSocket 消息推送功能

实现了实时消息推送机制，支持客户端与服务端的双向通信。

- 添加 WebSocket 服务端实现
- 实现消息广播功能
- 添加连接状态管理
```

### PR 流程

1. **创建 Issue**：先在 GitHub/Gitee 上创建 Issue，描述要解决的问题或实现的功能
2. **讨论方案**：与维护者和其他贡献者讨论实现方案
3. **分支开发**：从 `main` 分支创建新分支，命名格式：`feature/<功能名>` 或 `fix/<bug描述>`
4. **提交代码**：遵循代码风格规范和提交信息规范
5. **创建 PR**：提交 Pull Request，关联相关 Issue
6. **代码审查**：等待维护者审查，根据反馈进行修改
7. **合并分支**：通过审查后，由维护者合并到主分支

### 项目结构规范

```
├── module_name/           # 模块目录（小写，下划线分隔）
│   ├── sub_module/        # 子模块目录
│   ├── main.go            # 主入口文件
│   ├── type.go            # 类型定义
│   ├── create.go          # 创建/初始化相关
│   └── handler.go         # 处理函数
```

### 构建流程

本项目使用 PowerShell 脚本进行构建：

```powershell
# 完整构建
.\build.ps1

# 指定目标平台
.\build.ps1 -TargetOS linux -TargetArch arm64
```

**环境要求**
- Go >= 1.24
- Node.js >= 20.x
- GCC（CGO 支持）
- rsrc 工具（图标嵌入）

---

## 📄 许可证

本项目采用 **Lunar Astral Agents Non-Commercial License**（星月智能非商业许可证）。

### 许可证要点

| 权限 | 允许 | 说明 |
|------|------|------|
| **复制** | ✅ | 制作项目材料的授权副本和重印本 |
| **修改** | ✅ | 创建项目源代码的修改版本 |
| **分发** | ✅ | 分发原始项目及其修改版本 |
| **非商业使用** | ✅ | 个人、教育和研究用途 |
| **商业使用** | ❌ | 禁止任何盈利目的的使用 |

### 重要条款

1. **禁止商业使用**：未经作者书面明确许可，不得将项目用于任何商业目的或盈利活动
2. **署名要求**：复制、修改或分发时必须保留原始版权声明和许可证条款
3. **免责声明**：项目按"原样"提供，不提供任何形式的保证
4. **责任限制**：作者不对因使用或无法使用本项目而产生的任何损害承担责任

### 联系方式

- **仓库地址**: https://gitee.com/TayunStarry/Lunar-Astral-Agents/tree/master
- **QQ群号**: 710834920
- **作者信息**: 钛宇-星光阁 (TayunStarry)

完整许可证文本请参阅 [LICENSE](LICENSE) 文件。

---

## 🌙 关于星月智能

星月智能致力于打造本地化、私有化的AI智能体系统，让每个用户都能拥有自己的专属AI伙伴。月华、琉璃与蔷薇（规划中）三位姐妹，将陪伴您探索无限可能。

> _"在星空中寻找答案，在月光下创造美好"_ 🌙✨
