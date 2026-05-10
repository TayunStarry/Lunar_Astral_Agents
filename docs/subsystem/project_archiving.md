# Project Archiving 子系统 - 项目归档功能文档

> 📦 **Project Archiving子系统**是星月智能的项目归档模块，负责对项目文件进行打包、压缩和管理。

---

## 🏗️ 架构设计

### 模块结构

```
subsystem/project_archiving/
├── main.go                   # 主程序入口
├── lunar_config.json         # 配置文件
├── build.ps1                 # 构建脚本
├── component/
│   ├── config.go            # 配置管理
│   ├── config_loader.go     # 配置加载
│   ├── check.go             # 检查组件
│   ├── clean.go             # 清理组件
│   ├── create.go            # 创建归档
│   ├── execute.go           # 执行管理
│   ├── progress.go          # 进度追踪
│   ├── source.go            # 源文件管理
│   └── utils.go             # 工具函数
├── go.mod
└── go.sum
```

---

## 🎯 功能特性

### 归档操作

- **创建归档**：将项目文件夹打包为压缩文件
- **清理归档**：删除临时文件和缓存
- **检查完整性**：验证归档文件完整性

### 压缩格式

支持多种压缩格式：
| 格式 | 扩展名 | 说明 |
|------|--------|------|
| ZIP | .zip | 通用压缩格式 |
| TAR | .tar | Unix归档格式 |
| TAR.GZ | .tar.gz | gzip压缩归档 |
| 7Z | .7z | 高压缩比格式 |

### 配置管理

- 排除规则（.git, node_modules等）
- 压缩级别设置
- 归档命名规则
- 输出目录配置

---

## 🔧 使用方式

### 命令行使用

```powershell
# 创建归档
.\project_archiving.exe create -source ./myproject -output ./archives

# 创建ZIP归档
.\project_archiving.exe create -source ./myproject -format zip

# 清理临时文件
.\project_archiving.exe clean -path ./myproject

# 检查归档完整性
.\project_archiving.exe check -archive ./archives/project.zip
```

### 配置文件

```json
{
  "archive": {
    "default_format": "zip",
    "compression_level": 6,
    "exclude_patterns": [
      ".git",
      "node_modules",
      "*.log",
      "__pycache__"
    ],
    "output_dir": "./archives"
  }
}
```

---

## 📡 API接口

### 归档创建接口

#### POST /archive/create

**功能**：创建项目归档

**请求体**：
```json
{
  "source_path": "./myproject",
  "output_path": "./archives/project.zip",
  "format": "zip",
  "compression_level": 6,
  "exclude_patterns": [".git", "node_modules"]
}
```

**响应格式**：
```json
{
  "success": true,
  "archive_path": "./archives/project.zip",
  "original_size": 1024000,
  "compressed_size": 512000,
  "file_count": 150
}
```

---

### 归档检查接口

#### POST /archive/check

**功能**：检查归档完整性

**请求体**：
```json
{
  "archive_path": "./archives/project.zip"
}
```

**响应格式**：
```json
{
  "success": true,
  "valid": true,
  "file_count": 150,
  "corrupted_files": []
}
```

---

### 归档解压接口

#### POST /archive/extract

**功能**：解压归档文件

**请求体**：
```json
{
  "archive_path": "./archives/project.zip",
  "output_dir": "./restored"
}
```

**响应格式**：
```json
{
  "success": true,
  "extracted_count": 150,
  "output_dir": "./restored"
}
```

---

## 📁 组件说明

### ConfigLoader

负责加载和解析配置文件：

```go
type ArchiveConfig struct {
    DefaultFormat     string   `json:"default_format"`
    CompressionLevel  int      `json:"compression_level"`
    ExcludePatterns   []string `json:"exclude_patterns"`
    OutputDir         string   `json:"output_dir"`
}
```

### CreateArchiver

负责创建归档文件：

```go
type CreateOptions struct {
    SourcePath        string
    OutputPath        string
    Format           string
    CompressionLevel int
    ExcludePatterns  []string
}
```

### ProgressTracker

负责跟踪归档进度：

```go
type Progress struct {
    CurrentFile   string
    ProcessedFiles int
    TotalFiles    int
    Percentage    float64
}
```

---

## 🔗 关联文档

- [主项目README](../../README.md)
- [星图·月华 文档](../luna_astral.md)
- [星图·琉璃 文档](../crystal_astral.md)
- [存储子系统文档](storage.md)