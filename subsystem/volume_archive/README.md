# 子系统——分卷归档（volume_archive）

项目分卷归档管理工具，支持按计划打包、分卷压缩、进度追踪与旧文件清理，用于将星月智能平台产物打包为可分发的分卷压缩包。

---

## 目录

- [功能概述](#功能概述)
- [项目结构](#项目结构)
- [核心架构](#核心架构)
- [核心模块说明](#核心模块说明)
- [编译与运行](#编译与运行)
- [使用示例](#使用示例)
- [常见问题](#常见问题)

---

## 功能概述

`volume_archive` 模块提供项目产物的分卷归档能力，将指定文件和目录打包为 7z 分卷压缩包，便于分发与存储。

| 功能 | 说明 |
|------|------|
| **计划打包** | 通过配置文件定义多个打包计划，按需选择执行 |
| **分卷压缩** | 支持自定义分卷大小（MB），生成 7z 分卷压缩包 |
| **压缩级别** | 支持 0-9 共 10 级压缩，平衡速度与压缩率 |
| **排除规则** | 支持通配符排除不需要的文件（日志、临时文件、构建产物等） |
| **进度追踪** | 实时显示压缩进度条与耗时统计 |
| **自动清理** | 打包前自动清理旧的分卷文件，避免残留 |
| **7z 自动发现** | 自动搜索系统中的 7z 工具路径 |

---

## 项目结构

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>volume_archive/</strong></li>
    <li style="padding-left: 1.5em;"><code>main.go</code> <span style="color: #6a737d;">— 程序入口（参数解析、配置加载、执行调度）</span></li>
    <li style="padding-left: 1.5em;"><code>go.mod</code> <span style="color: #6a737d;">— Go 模块定义</span></li>
    <li style="padding-left: 1.5em;"><code>build.ps1</code> <span style="color: #6a737d;">— 构建脚本</span></li>
    <li style="padding-left: 1.5em;"><code>icon.ico</code> <span style="color: #6a737d;">— 应用图标</span></li>
    <li style="padding-left: 1.5em;">
      <strong>component/</strong> <span style="color: #6a737d;">— 核心业务逻辑</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>config.go</code> <span style="color: #6a737d;">— 配置结构定义（ArchivingConfig / PackageConfig / ExecuteParams）</span></li>
        <li><code>config_loader.go</code> <span style="color: #6a737d;">— 配置文件加载与解析（JSON → Plans 映射）</span></li>
        <li><code>execute.go</code> <span style="color: #6a737d;">— 执行编排（参数验证 → 配置加载 → 源文件获取 → 清理 → 压缩）</span></li>
        <li><code>source.go</code> <span style="color: #6a737d;">— 源文件处理（按计划获取路径、路径解析与验证）</span></li>
        <li><code>create.go</code> <span style="color: #6a737d;">— 分卷压缩创建（目录扫描、文件列表生成、7z 进程调用）</span></li>
        <li><code>check.go</code> <span style="color: #6a737d;">— 工具检查（7z 路径自动发现、文件存在性检查）</span></li>
        <li><code>clean.go</code> <span style="color: #6a737d;">— 旧文件清理（匹配并删除旧的分卷压缩包）</span></li>
        <li><code>progress.go</code> <span style="color: #6a737d;">— 进度追踪（进度条渲染、耗时统计、Spinner 动画）</span></li>
        <li><code>utils.go</code> <span style="color: #6a737d;">— 工具函数（路径解析、基准目录计算、日志封装）</span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;">
      <strong>local_data/</strong> <span style="color: #6a737d;">— 本地配置</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>lunar_config.json</code> <span style="color: #6a737d;">— 打包配置文件（计划定义、排除规则、默认参数）</span></li>
      </ul>
    </li>
  </ul>
</div>

### 依赖关系

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><code>volume_archive</code></li>
    <li style="padding-left: 1.5em;"><code>logger</code> <span style="color: #6a737d;">(../logger) — 彩色终端日志</span></li>
  </ul>
</div>

---

## 核心架构

### 执行流程

```
程序入口 main()
    │
    ▼
参数解析（flag 包）
    │  -config       配置文件路径
    │  -output_path  输出文件基础名称
    │  -part_size_mb 分卷大小（MB）
    │  -compression_level 压缩级别（0-9）
    │  -package_plan 打包计划名称
    │
    ▼
applyDefaults() — 从配置文件填充缺失参数
    │
    ▼
component.Execute()
    │
    ├── ① ValidateParams()      — 参数合法性校验
    ├── ② LoadPackageConfig()   — 加载 JSON 配置文件
    ├── ③ GetSourcesByPlan()     — 按计划获取源文件路径列表
    ├── ④ cleanOldParts()       — 清理旧的分卷压缩包
    └── ⑤ createVolume()        — 创建分卷压缩包
         │
         ├── scanDirectory()    — 递归扫描目录（跳过排除项）
         ├── 生成文件列表        — 写入临时文件 @listfile
         ├── find7zPath()       — 自动发现 7z 工具路径
         └── exec.Command(7z)  — 调用 7z 进程执行压缩
              │
              └── ProgressTracker — 实时解析输出，渲染进度条
```

### 配置体系

```
lunar_config.json
    │
    ├── project_archiving
    │   ├── plan-1 / plan-2 / plan-3  ← 打包计划（名称 → 路径列表）
    │   ├── exclude                   ← 排除规则（通配符）
    │   ├── sevenzip_paths            ← 7z 工具搜索路径
    │   └── defaults                  ← 默认参数
    │       ├── output_path           ← 输出文件基础名称
    │       ├── part_size_mb          ← 分卷大小（MB）
    │       ├── compression_level     ← 压缩级别
    │       └── package_plan          ← 默认打包计划
```

### 进度追踪机制

```
7z 进程 stdout/stderr
    │
    ▼
ProgressTracker.UpdateProgress(line)
    │
    ├── 正则匹配 (\d+)% → 提取百分比
    │
    ├── displayProgress(percent)
    │   └── 渲染进度条: [████████░░░░░░░░░░░░]  45%  耗时: 12.3s
    │       ├── 0-25%:  亮黄色
    │       ├── 25-50%: 黄色
    │       ├── 50-75%: 深黄色
    │       ├── 75-100%: 绿色
    │       └── 100%:   [████████████████████████] ✓ 完成!
    │
    └── displayPreparing()
        └── Spinner 动画: ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏ 正在准备压缩...
```

---

## 核心模块说明

### component/config.go — 配置结构

| 结构体 | 字段 | 说明 |
|--------|------|------|
| `ArchivingConfig` | `Plans`, `Exclude`, `SevenZipPaths`, `Defaults` | 归档配置顶层结构 |
| `DefaultConfig` | `OutputPath`, `PartSizeMB`, `CompressionLevel`, `PackagePlan` | 默认参数 |
| `PackageConfig` | `ProjectArchiving` | 完整配置结构（JSON 映射） |
| `ExecuteParams` | `ConfigPath`, `OutputPath`, `PartSizeMB`, `CompressionLevel`, `PackagePlan`, `StartTime` | 运行时参数 |

### component/config_loader.go — 配置加载

| 函数 | 说明 |
|------|------|
| `LoadPackageConfig(configPath)` | 加载 JSON 配置文件，解析打包计划为 `map[string][]string` |
| `GetExcludePatterns()` | 获取排除规则列表 |
| `GetSevenZipPaths()` | 获取 7z 工具搜索路径列表 |
| `IsExcluded(name, isDir)` | 检查文件/目录是否匹配排除规则 |

### component/execute.go — 执行编排

| 函数 | 说明 |
|------|------|
| `Execute(params)` | 主执行流程：验证 → 加载配置 → 获取源文件 → 清理 → 压缩 |
| `ValidateParams(params)` | 参数合法性校验（路径非空、分卷大小 > 0、压缩级别 0-9） |

### component/create.go — 分卷压缩

| 函数 | 说明 |
|------|------|
| `createVolume(sources, outputPath, partSizeMB, compressionLevel)` | 创建分卷压缩包 |
| `scanDirectory(dir, baseDir)` | 递归扫描目录，跳过排除项，返回相对路径列表 |

### component/progress.go — 进度追踪

| 结构体/方法 | 说明 |
|------------|------|
| `ProgressTracker` | 进度追踪器（百分比、耗时、Spinner 索引） |
| `NewProgressTracker()` | 创建新的进度追踪器 |
| `UpdateProgress(output)` | 从 7z 输出解析进度百分比 |
| `displayProgress(percent)` | 渲染彩色进度条 |
| `displayPreparing()` | 渲染 Spinner 准备动画 |

---

## 编译与运行

### 编译

```powershell
cd d:\Lunar_Astral_Agents\subsystem\volume_archive

# 一键构建（推荐）
.\build.ps1

# 交叉编译 Linux 版本
.\build.ps1 -TargetOS linux -TargetArch amd64
```

构建脚本会自动：
1. 处理图标资源（`icon.ico` → `icon.syso`）
2. 设置 `CGO_ENABLED=1`
3. 编译带 `-tags webview` 的可执行文件

编译产物：`d:\Lunar_Astral_Agents\volume_archive.exe`

### 手动构建

```powershell
cd d:\Lunar_Astral_Agents\subsystem\volume_archive

$env:CGO_ENABLED = "1"

go build -tags webview -ldflags="-s -w" -trimpath -o ../../volume_archive.exe
```

### 运行

```powershell
# 使用默认配置
.\volume_archive.exe

# 指定配置文件和参数
.\volume_archive.exe -config local_data/lunar_config.json -package_plan plan-2

# 完整参数示例
.\volume_archive.exe `
  -config local_data/lunar_config.json `
  -output_path "./Lunar-Astral-Agents-Release" `
  -part_size_mb 2048 `
  -compression_level 5 `
  -package_plan plan-3
```

### 命令行参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-config` | string | `local_data/lunar_config.json` | 配置文件路径 |
| `-output_path` | string | 配置文件中的 `defaults.output_path` | 输出文件基础名称 |
| `-part_size_mb` | int | 配置文件中的 `defaults.part_size_mb` | 分卷大小（MB） |
| `-compression_level` | int | 配置文件中的 `defaults.compression_level` | 压缩级别（0-9） |
| `-package_plan` | string | 配置文件中的 `defaults.package_plan` | 打包计划名称 |

> 未指定的参数将从配置文件的 `defaults` 节自动填充。

---

## 使用示例

### 配置文件示例

```json
{
  "project_archiving": {
    "plan-1": [
      "../.././Lunar-Astral-Agents.exe",
      "../.././local_data/certs",
      "../.././local_data/model_config.json"
    ],
    "plan-2": [
      "../.././Lunar-Astral-Agents.exe",
      "../.././webpage",
      "../.././local_data/certs",
      "../.././subsystem"
    ],
    "exclude": [
      "*.log",
      "*.tmp",
      "*.bak",
      ".git/",
      "node_modules/",
      "*.pyc",
      "*.pyo",
      "dist/",
      "build/"
    ],
    "sevenzip_paths": [
      "./local_data/package/archive/7z.exe",
      "C:/Program Files/7-Zip/7z.exe"
    ],
    "defaults": {
      "output_path": "./Lunar-Astral-Agents-Release",
      "part_size_mb": 2048,
      "compression_level": 5,
      "package_plan": "plan-2"
    }
  }
}
```

### 排除规则说明

| 规则 | 匹配目标 | 说明 |
|------|---------|------|
| `*.log` | 文件 | 排除所有 .log 文件 |
| `*.tmp` | 文件 | 排除所有 .tmp 文件 |
| `.git/` | 目录 | 排除 .git 目录（尾随 `/` 表示目录） |
| `node_modules/` | 目录 | 排除 node_modules 目录 |
| `dist/` | 目录 | 排除构建输出目录 |
| `.DS_Store` | 文件 | 排除 macOS 系统文件 |

---

## 常见问题

### Q: 提示"未找到 7z 命令行工具"怎么办？

程序会按以下顺序搜索 7z 工具：
1. 配置文件中 `sevenzip_paths` 列表的路径
2. 系统 PATH 环境变量中的 `7z`

解决方案：
- 安装 [7-Zip](https://7-zip.org/) 并添加到 PATH
- 或在配置文件的 `sevenzip_paths` 中指定 7z.exe 的完整路径

### Q: 如何创建新的打包计划？

在 `lunar_config.json` 的 `project_archiving` 下添加新的键值对，键为计划名称，值为文件/目录路径数组：

```json
"my-plan": [
  "../.././Lunar-Astral-Agents.exe",
  "../.././local_data"
]
```

然后通过 `-package_plan my-plan` 参数指定使用。

### Q: 分卷大小如何选择？

- **2048 MB**（默认）：适合大多数场景，兼容 FAT32 文件系统
- **4096 MB**：适合大容量存储，减少分卷数量
- **700 MB**：适合 CD 介质分发

### Q: 压缩级别如何选择？

| 级别 | 速度 | 压缩率 | 适用场景 |
|------|------|--------|---------|
| 0 | 最快 | 无压缩 | 仅分卷，不压缩 |
| 1-3 | 快 | 低 | 快速打包 |
| 5 | 中 | 中 | **默认，推荐** |
| 7-9 | 慢 | 高 | 最小体积，耗时较长 |

### Q: 打包前会自动清理旧文件吗？

是的，`cleanOldParts()` 会在压缩前自动删除与输出路径同名的旧分卷文件（匹配 `*.7z.*` 模式）和完整压缩包，避免残留。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [日志子系统](../logger/README.md) —— 彩色终端日志
- [配置管理子系统](../config/README.md) —— 平台配置管理
