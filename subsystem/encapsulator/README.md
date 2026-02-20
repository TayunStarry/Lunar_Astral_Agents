# Encapsulator 子系统文档

## 1. 系统概述

Encapsulator 是 Lunar Astral Agents 的打包工具子系统，主要负责将项目文件打包成不同级别的分卷压缩包，以便于分发和更新。

## 2. 功能特点

- **多级打包**：支持三种打包级别，满足不同场景的需求
- **分卷压缩**：可自定义分卷大小，便于传输和存储
- **配置灵活**：通过配置文件和命令行参数双重控制
- **智能默认值**：当未指定参数时，自动使用内置默认配置
- **错误处理**：加载配置失败时，会使用内置默认配置并记录警告

## 3. 核心组件

### 3.1 配置管理

- **package-config.json**：定义打包级别、源文件路径和默认配置
- **LoadPackageConfig**：加载配置文件
- **GetDefaultConfig**：获取内置默认配置

### 3.2 打包流程

- **ExecutePackageProcess**：执行完整的打包流程
- **内部模块**：包含 check、clean、config、create、execute、progress、source、utils 等功能模块

## 4. 配置说明

### 4.1 配置文件结构

```json
{
  "package_levels": {
    "1": {
      "name": "核心文件",
      "description": "用于发布纯前端的功能补丁更新",
      "sources": [
        "../.././Lunar-Astral-Agents.exe",
        "../.././webpage",
        "../.././local_data/certs",
        "../.././local_data/resources",
        "../.././local_data/model_config.json",
        "../.././local_data/knowledge/meme_model.json",
        "../.././local_data/knowledge/emotional_model.json"
      ]
    },
    "2": {
      "name": "核心文件 + 扩展程序",
      "description": "用于发布包含扩展程序的完整可执行文件系统的功能更新",
      "sources": [
        "../.././Lunar-Astral-Agents.exe",
        "../.././webpage",
        "../.././local_data/certs",
        "../.././local_data/resources",
        "../.././local_data/model_config.json",
        "../.././local_data/knowledge/meme_model.json",
        "../.././local_data/knowledge/emotional_model.json",
        "../.././subsystem"
      ]
    },
    "3": {
      "name": "完整包",
      "description": "用于发布包含所有文件的完整可执行文件系统的功能更新",
      "sources": [
        "../.././models",
        "../.././Lunar-Astral-Agents.exe",
        "../.././webpage",
        "../.././local_data/certs",
        "../.././local_data/resources",
        "../.././local_data/model_config.json",
        "../.././local_data/knowledge/meme_model.json",
        "../.././local_data/knowledge/emotional_model.json",
        "../.././subsystem"
      ]
    }
  },
  "sevenzip_paths": [
    "./archive/7z.exe",
    "C:/Program Files/7-Zip/7z.exe",
    "C:/Program Files (x86)/7-Zip/7z.exe"
  ],
  "defaults": {
    "output_path": "./Lunar-Astral-Agents^2026-01-28",
    "part_size_mb": 2048,
    "compression_level": 9,
    "package_level": 2
  }
}
```

### 4.2 命令行参数

- `output_path`：输出文件路径
- `part_size_mb`：分卷大小（MB）
- `compression_level`：压缩级别
- `package_level`：打包级别
- `config_path`：配置文件路径

## 5. 使用方法

### 5.1 基本用法

```bash
# 使用默认配置打包
./encapsulator.exe

# 指定输出路径和分卷大小
./encapsulator.exe -output_path ./release -part_size_mb 1024

# 指定打包级别（1-3）
./encapsulator.exe -package_level 3

# 指定配置文件路径
./encapsulator.exe -config_path ./custom-config.json
```

### 5.2 打包级别说明

| 级别 | 名称 | 包含内容 | 用途 |
|------|------|----------|------|
| 1 | 核心文件 | 可执行文件、网页、证书、资源、模型配置、知识库 | 纯前端功能补丁更新 |
| 2 | 核心文件+扩展程序 | 核心文件 + 子系统目录 | 包含扩展程序的完整更新 |
| 3 | 完整包 | 核心文件+扩展程序 + 模型目录 | 完整可执行文件系统更新 |

## 6. 工作流程

1. 解析命令行参数
2. 加载配置文件（若失败则使用内置默认配置）
3. 检查用户是否指定了必要参数
4. 若未指定必要参数，使用默认值
5. 执行打包流程
   - 检查打包环境
   - 清理临时文件
   - 准备源文件
   - 执行分卷压缩
   - 生成打包结果

## 7. 依赖关系

- **7-Zip**：用于执行压缩操作，会自动搜索系统中的7-Zip可执行文件
- **Go语言**：开发语言，需要Go环境编译

## 8. 注意事项

- 打包过程需要足够的磁盘空间存储临时文件和最终结果
- 压缩级别越高，压缩率越大，但打包速度越慢
- 分卷大小应根据目标存储介质的限制进行调整
- 打包级别3包含模型目录，生成的包会比较大

## 9. 故障排除

- **配置文件加载失败**：检查配置文件路径和格式是否正确，系统会自动使用内置默认配置
- **7-Zip未找到**：确保7-Zip已正确安装，或在当前目录下放置7z.exe
- **打包失败**：检查源文件路径是否存在，磁盘空间是否充足

## 10. 示例

### 示例1：创建核心文件包（级别1）

```bash
./encapsulator.exe -package_level 1 -output_path ./core-update -part_size_mb 1024
```

### 示例2：创建完整包（级别3）

```bash
./encapsulator.exe -package_level 3 -output_path ./full-release -part_size_mb 2048
```

### 示例3：使用自定义配置文件

```bash
./encapsulator.exe -config_path ./custom.json -output_path ./custom-release
```