# 贡献指南

感谢你对**星月智能（Lunar Astral Agents）**项目的关注！我们欢迎任何形式的贡献，包括但不限于代码提交、Bug 报告、功能建议、文档改进和问题讨论。

---

## 目录

- [行为准则](#行为准则)
- [开始前的准备](#开始前的准备)
- [如何贡献](#如何贡献)
  - [报告 Bug](#报告-bug)
  - [提出功能建议](#提出功能建议)
  - [提交代码](#提交代码)
  - [文档贡献](#文档贡献)
- [开发规范](#开发规范)
  - [Go 代码规范](#go-代码规范)
  - [前端代码规范](#前端代码规范)
  - [TypeScript 代码规范](#typescript-代码规范)
  - [C/C++ 代码规范](#cc-代码规范)
- [提交信息规范](#提交信息规范)
- [Pull Request 流程](#pull-request-流程)
- [开发环境配置](#开发环境配置)
- [许可协议](#许可协议)

---

## 行为准则

参与本项目的所有成员必须遵守 [行为准则](CODE_OF_CONDUCT.md)。请以尊重、友善的态度交流，共同维护健康的社区环境。

---

## 开始前的准备

1. **阅读文档**: 通读 [README.md](README.md) 和 [ARCHITECTURE.md](ARCHITECTURE.md)，了解项目的整体架构与各子系统职责
2. **分叉仓库**: 将项目 Fork 到你的 GitHub 账户下，在 Fork 后的仓库中进行开发
3. **创建分支**: 基于 `main` 分支创建功能分支，命名建议 `feat/描述` 或 `fix/描述`

---

## 如何贡献

### 报告 Bug

1. 前往 [Issues](../../issues) 页面，选择 **"Bug 报告"** 模板
2. 填写以下关键信息：
   - 运行环境（操作系统、Go 版本、Node 版本）
   - 复现步骤（尽可能详细，附截图或日志）
   - 预期行为 vs 实际行为
3. 为 Issue 添加适当的标签（如 `bug`、`优先级-高`）

### 提出功能建议

1. 前往 [Issues](../../issues) 页面，选择 **"功能请求"** 模板
2. 描述需求背景、期望效果以及可能的实现思路
3. 说明该功能可能影响哪些现有模块

### 提交代码

1. 确保你的代码符合项目规范（见下方开发规范章节）
2. 确保现有功能未被破坏（编译通过、无回归问题）
3. 为新增功能编写适当的注释说明
4. 将你的修改提交到 Fork 仓库的分支，并发起 Pull Request

### 文档贡献

文档改进同样重要。如果你发现 README、架构文档或代码注释中存在问题，欢迎提交修复。请确保文档内容清晰、准确，格式符合 Markdown 规范。

---

## 开发规范

### Go 代码规范

| 规则 | 说明 |
|------|------|
| 文件职责分离 | **类型定义**集中放在 `type.go`，**全局变量/常量**集中放在 `variable.go` |
| 包级变量 | 优先使用局部变量，避免不必要的包级状态共享 |
| 错误处理 | 必须显式处理 error，禁止使用 `_` 忽略 |
| 命名规范 | 导出符号使用 PascalCase，非导出符号使用 camelCase |
| 代码格式化 | 使用 `gofmt` 格式化，提交前执行 `go fmt ./...` |
| 导入分组 | 标准库、第三方库、项目内部包之间用空行分隔 |

```go
// 正确的导入顺序示例
import (
    "context"
    "fmt"

    "github.com/xxx/xxx"

    "lunar_astral/server/handlers"
)
```

### 前端代码规范

| 规则 | 说明 |
|------|------|
| 技术栈 | HTML5 + CSS3 + Vanilla JS (ES6+)，**禁止使用 Python** |
| 模块 | ES Modules (`type="module"`) |
| 依赖管控 | **仅限** `standard_dependency/` 中的 `script.js` 和 `styles.css`，禁止从 CDN 引入外部资源 |
| 设计风格 | 玻璃拟态 (Glassmorphism)，CSS 变量驱动主题 |
| 图标 | Font Awesome 6.4.0 |
| 样式命名 | kebab-case，状态类用 `.active` / `.visible` / `.hidden` |
| 响应式 | 支持 1024px / 768px / 480px 三断点 |

### TypeScript 代码规范

| 规则 | 说明 |
|------|------|
| 模块系统 | ES Modules，与前端 JS 统一 |
| 类型定义 | 所有导出函数与接口必须有明确的类型声明 |
| 文件命名 | kebab-case，如 `websearch.ts` |
| 编译 | 使用 Rollup 打包，配置参见 `rollup.config.js` |

### C/C++ 代码规范

| 规则 | 说明 |
|------|------|
| 适用场景 | ASR 语音识别、TTS 语音合成、Stable Diffusion 推理等底层模块 |
| 头文件 | 使用 `#pragma once` 或 include guard |
| 命名 | 函数使用 snake_case，类型使用 PascalCase |
| 内存管理 | 显式释放所有分配的资源，使用 RAII 或明确配对调用 |
| CGo 边界 | Go 调用 C 代码时，确保 `import "C"` 上方有正确的注释 |

---

## 提交信息规范

本项目使用结构化的中文提交信息格式。每次提交前请确保信息遵循以下模板：

```
[类型] 简短描述（≤50 字）

[心情小记]

本次更新概述：
- 主要变化描述
- 影响说明
- 机制变更

变更影响：
- 后续开发：说明对后续开发的影响
- 当前项目：说明对当前项目功能的影响
- 效果提升：描述性能、可读性或功能提升
- 机制变更：说明删除或修改的机制
```

**类型列表**:

| 类型 | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | 修复 Bug |
| `docs` | 文档更新 |
| `style` | 代码风格调整（不影响功能） |
| `refactor` | 代码重构 |
| `test` | 测试相关 |
| `chore` | 构建/依赖管理 |

---

## Pull Request 流程

1. **发起 PR**: 从你的 Fork 分支向 `main` 分支发起 Pull Request
2. **使用模板**: 按照 [PR 模板](.github/PULL_REQUEST_TEMPLATE.md) 填写描述信息
3. **关联 Issue**: 在描述中使用 `Closes #编号` 或 `Fixes #编号` 关联相关 Issue
4. **代码审查**: 维护者将对代码进行审查，请保持关注并及时响应反馈
5. **合并**: 经审查通过后，由维护者合并到 `main` 分支

### PR 自检清单

- [ ] 代码已通过 `go fmt` / `gofmt` 格式化
- [ ] Go 编译无错误（运行对应模块的 `build.ps1`）
- [ ] TypeScript 编译无错误（运行 `npx tsc --noEmit`）
- [ ] 前端无 ESLint 或明显语法错误
- [ ] 新功能有适当的注释
- [ ] 提交信息符合规范格式
- [ ] 已关联相关 Issue

---

## 开发环境配置

### 必需工具

| 工具 | 最低版本 | 用途 |
|------|----------|------|
| Go | 1.26+ | 后端服务 |
| Node.js | 20+ | 前端 & TypeScript |
| Git | 2.40+ | 版本控制 |

### Windows 环境（推荐）

```powershell
# 克隆仓库
git clone https://github.com/你的用户名/Lunar_Astral_Agents.git
cd Lunar_Astral_Agents

# 编译核心系统
.\build.ps1

# 编译子系统（按需）
.\subsystem\qwen3_tts\build.ps1
.\subsystem\qwen_asr\build.ps1
```

---

## 许可协议

本项目使用 [Lunar Astral Agents Non-Commercial License](LICENSE)（非商业许可）。提交贡献即表示你同意在此许可下分发你的贡献内容。请确保你提交的代码不包含任何第三方 GPL/商业许可下的代码，除非已获得明确授权。

---

> 如有任何疑问，欢迎通过 Issues 与我们联系！