# Lunar Astral Agents EXE API 文档

## 1. 概述

EXE API 提供了与 Lunar Astral Agents 可执行文件交互的能力，允许通过命令行参数配置和控制系统行为。本 API 主要用于系统部署、配置管理、服务启动与停止等场景。

## 2. 主程序命令行参数

### 2.1 系统核心参数

| 参数名称 | 类型 | 默认值 | 描述 |
| -------- | ---- | ------ | ---- |
| `-basic-port` | int | 36789 | 系统Web服务的监听端口，用户可通过此端口访问客户端界面 |
| `-max-port` | int | 36804 | 系统Web服务的最大监听端口 |
| `-min-port` | int | 36784 | 系统Web服务的最小监听端口 |
| `-dev-mode` | bool | false | 启用调试模式，显示详细日志且不自动打开Web界面 |
| `-clear-port` | bool | true | 启动时自动释放被占用的端口 |
| `-cert-file` | string | `local_data/certs/localhost.pem` | 证书文件路径，用于HTTPS加密通信 |
| `-key-file` | string | `local_data/certs/localhost-key.pem` | 私钥文件路径，用于HTTPS加密通信 |
| `-database` | string | `local_data/SQLite.db` | SQLite数据库文件路径，用于存储系统数据 |

### 2.2 模型服务参数

| 参数名称 | 类型 | 默认值 | 描述 |
| -------- | ---- | ------ | ---- |
| `-infer-engine` | string | `./subsystem/neural_engines/llama-server.exe` | 图文推理工具的路径 |
| `-visual-engine` | string | `./subsystem/neural_engines/sd-cli.exe` | 绘图生成工具的路径 |
| `-model-port` | int | 36790 | 模型服务的基础端口号，用于分配模型运行端口 |
| `-allow-multimodal` | bool | true | 是否允许加载多模态模型进行推理 |

### 2.3 模型配置参数

| 参数名称 | 类型 | 默认值 | 描述 |
| -------- | ---- | ------ | ---- |
| `-embedding-model` | string | `./models/Qwen3-Embedding-0.6B-Q8_0.gguf` | 嵌入模型路径，用于文本向量化表示 |
| `-multimodal-model` | string | `./models/Qwen3-VL-30B-A3B-Instruct-UD-Q4_K_XL.gguf` | 多模态推理模型路径 |
| `-mmproj-model` | string | `./models/mmproj-BF16.gguf` | 多模态投影模型路径，用于图像与文本的联合编码 |
| `-allow-diffusion` | bool | true | 是否启用灵绘坊 |
| `-diffusion-model` | string | `./models/z_image_turbo-Q4_K.gguf` | 扩散模型路径，用于图像生成 |
| `-variational-model` | string | `./models/diffusion_pytorch_model.safetensors` | VAE模型路径，用于图像编码与解码 |
| `-prompt-model` | string | `./models/Qwen3-4B-Instruct-2507-Q4_K_M.gguf` | 大语言模型路径，用于优化图像提示词与负面提示词 |

### 2.4 其他配置参数

| 参数名称 | 类型 | 默认值 | 描述 |
| -------- | ---- | ------ | ---- |
| `-max-width` | int | 1920 | 最大宽度 |
| `-max-height` | int | 1080 | 最大高度 |
| `-jpeg-quality` | int | 80 | JPEG 压缩质量 (1-100) |
| `-format` | string | `png` | 图片格式 (png, jpg, jpeg) |

## 3. 子系统命令行参数

### 3.1 StarRelease 子系统

StarRelease 是用于项目发布和打包的工具。

| 参数名称 | 类型 | 默认值 | 描述 |
| -------- | ---- | ------ | ---- |
| `-config` | string | `` | 打包配置文件路径 |
| `-system_dev_mode` | bool | false | 是否使用调试模式 |
| `-output_path` | string | `Lunar-Astral-Agents` | 输出文件的基础名称（如需开启打包功能，需指定该参数） |
| `-part_size_mb` | int | 2048 | 分卷大小(MB) |
| `-compression_level` | int | 5 | 压缩级别 (0-9)，0表示不压缩，9表示固实压缩 |
| `-package_level` | int | 3 | 打包级别 (1-3)：<br>1: 核心文件 (可执行文件、网页、配置文件)<br>2: 级别1 + 扩展程序<br>3: 级别2 + 服务器文件 (所有文件) |

## 4. 使用示例

### 4.1 启动主服务

```bash
# 默认参数启动
./Lunar-Astral-Agents.exe

# 指定端口启动
./Lunar-Astral-Agents.exe -basic-port=36790

# 调试模式启动
./Lunar-Astral-Agents.exe -dev-mode=true

# 禁用端口释放启动
./Lunar-Astral-Agents.exe -clear-port=false

# 指定模型路径启动
./Lunar-Astral-Agents.exe -embedding-model="./models/custom-embedding.gguf" -multimodal-model="./models/custom-multimodal.gguf"

# 禁用灵绘坊启动
./Lunar-Astral-Agents.exe -allow-diffusion=false
```

### 4.2 使用 StarRelease 子系统

```bash
# 启动打包工具
cd subsystem/StarRelease
./Lunar-Astral-Agents.exe -output_path=my-release

# 指定打包级别和分卷大小
./Lunar-Astral-Agents.exe -output_path=my-release -package_level=2 -part_size_mb=1024

# 高压缩级别打包
./Lunar-Astral-Agents.exe -output_path=my-release -compression_level=9
```

## 5. 最佳实践

1. **端口配置**：为避免端口冲突，建议在部署时指定唯一的端口号
2. **调试模式**：开发和调试时启用调试模式，获取详细日志信息
3. **模型路径**：根据实际部署环境调整模型文件路径
4. **打包策略**：根据发布需求选择合适的打包级别和压缩参数
5. **配置备份**：定期备份配置文件，以便在系统迁移或恢复时使用

## 6. 故障排除

### 6.1 端口被占用

如果启动时提示端口被占用，可以使用以下命令释放端口：

```bash
./Lunar-Astral-Agents.exe -clear-port=true
```

### 6.2 模型加载失败

- 检查模型文件路径是否正确
- 确保模型文件格式正确
- 检查系统资源是否充足

### 6.3 子系统启动失败

- 检查子系统依赖是否完整
- 确认配置文件路径正确
- 查看控制台日志获取详细错误信息
