# 安装配置

本文档介绍如何安装和配置 Lunar Astral Agents 系统。

## 系统要求

| 要求 | 最低配置 | 推荐配置 |
|------|----------|----------|
| 操作系统 | Windows 10+ | Windows 11 |
| Go 版本 | 1.21+ | 1.24+ |
| 内存 | 8GB | 16GB+ |
| 显存 | 4GB (GPU加速) | 8GB+ (GPU加速) |
| 磁盘空间 | 10GB | 20GB+ |

## 模型文件要求

项目需要以下模型文件（放置于 `local_data/models/` 目录）：

| 模型类型 | 文件名 | 说明 |
|----------|--------|------|
| 多模态模型 | `Qwen3.5-9B-Q4_K_M.gguf` | 主推理模型 |
| 投影模型 | `mmproj-Qwen3.5-9B-BF16.gguf` | 多模态投影层 |
| 嵌入模型 | `Qwen3-Embedding-0.6B-Q8_0.gguf` | 文本向量化 |
| 扩散模型 | `z_image_turbo-Q4_K.gguf` | 图像生成 |
| 变分模型 | `diffusion_pytorch_model.safetensors` | VAE 模型 |
| 提示词模型 | `Qwen3-4B-Instruct-2507-Q4_K_M.gguf` | 提示词优化 |

## 安装步骤

### 1. 克隆项目

```bash
git clone <repository_url>
cd Lunar_Astral_Agents
```

### 2. 安装依赖

```bash
# 安装 Go 依赖
cd LunarCore
go mod download

# 安装前端依赖
npm install
```

### 3. 准备模型文件

将所需的 GGUF 模型文件放置在 `local_data/models/` 目录下，并更新 `lunar_config.json` 中的路径配置。

### 4. 构建项目

```bash
# 构建 LunarCore
cd LunarCore
go build -o Lunar-Astral-Agents.exe .

# 或使用构建脚本
./build.ps1
```

### 5. 运行服务

```bash
# 基本运行（使用默认端口 36789）
./Lunar-Astral-Agents.exe

# 开发模式
./Lunar-Astral-Agents.exe -developer

# 指定端口
./Lunar-Astral-Agents.exe -basic-port 8080

# 启动时清除占用端口
./Lunar-Astral-Agents.exe -clear-port
```

## 配置文件

### 主配置文件

主配置文件位于 `local_data/lunar_config.json`：

```json
{
    "models": {
        "multimodal_model": "./local_data/models/Qwen3.5-9B-Q4_K_M.gguf",
        "mmproj_model": "./local_data/models/mmproj-Qwen3.5-9B-BF16.gguf",
        "embedding_model": "./local_data/models/Qwen3-Embedding-0.6B-Q8_0.gguf",
        "diffusion_model": "./local_data/models/z_image_turbo-Q4_K.gguf",
        "variational_model": "./local_data/models/diffusion_pytorch_model.safetensors",
        "prompt_refine_model": "./local_data/models/Qwen3-4B-Instruct-2507-Q4_K_M.gguf"
    },
    "server": {
        "tts_url": "http://localhost:7860",
        "developer": false,
        "clear_port": false,
        "allow_diffusion": true,
        "allow_multimodal": true
    },
    "qq_adapter": {
        "napcat_ws_server": "ws://localhost:4567",
        "napcat_ws_token": "your_token_here",
        "lunar_core_url": "http://localhost:36789",
        "lunar_ws_server": "ws://localhost:36789/ws",
        "poll_interval": 10,
        "listen_group_ids": ["262221051"],
        "trigger_keywords": ["月华", "3826713076"],
        "display_logs": false,
        "default_reply": "月华不知道哦~"
    }
}
```

## CLI 参数详解

### 系统参数

| 参数名 | 数据类型 | 默认值 | 说明 |
|--------|----------|--------|------|
| `-developer` | bool | `false` | 启用调试模式，显示详细日志信息 |
| `-basic-port` | int | `36789` | 系统 Web 服务的监听端口 |
| `-max-port` | int | `basic-port+15` | 系统 Web 服务的最大监听端口 |
| `-min-port` | int | `basic-port-5` | 系统 Web 服务的最小监听端口 |
| `-proxy-port` | int | `basic-port+5` | 系统代理服务的监听端口 |
| `-clear-port` | bool | `false` | 启动时自动检测并释放被占用的端口 |
| `-local-dir` | string | `local_data` | 本地目录路径 |

### TTS/云服务参数

| 参数名 | 数据类型 | 默认值 | 说明 |
|--------|----------|--------|------|
| `-tts-url` | string | `http://localhost:7860` | TTS 语音服务的地址 |
| `-cloud-model-url` | string | 空 | 云模型服务的地址 |

### 模型配置参数

| 参数名 | 数据类型 | 默认值 | 说明 |
|--------|----------|--------|------|
| `-infer-engine` | string | `{local-dir}/models/llama.cpp/llama-server.exe` | llama.cpp 推理引擎路径 |
| `-model-port` | int | `basic-port+1` | 模型服务的基础端口号 |
| `-allow-multimodal` | bool | `true` | 是否允许加载多模态模型 |
| `-embedding-model` | string | `{local-dir}/models/Qwen3.GGUF` | 嵌入模型路径 |
| `-multimodal-model` | string | `{local-dir}/models/Qwen3.GGUF` | 多模态模型路径 |
| `-mmproj-model` | string | `{local-dir}/models/mmproj-Qwen3.GGUF` | 多模态投影模型路径 |
| `-diffusion-model` | string | - | 扩散模型路径 |
| `-vae` | string | - | 变分自编码器模型路径 |
| `-llm` | string | - | 提示词优化语言模型路径 |
| `-prompt-mmproj-model` | string | - | 多模态提示词投影模型路径 |

### 使用示例

```bash
# 示例 1: 开发模式运行
./Lunar-Astral-Agents.exe -developer

# 示例 2: 使用自定义端口
./Lunar-Astral-Agents.exe -basic-port 8080 -model-port 8081

# 示例 3: 启用端口自动清理
./Lunar-Astral-Agents.exe -clear-port

# 示例 4: 配置云端模型
./Lunar-Astral-Agents.exe -cloud-model-url https://api.openai.com/v1

# 示例 5: 禁用多模态功能
./Lunar-Astral-Agents.exe -allow-multimodal=false

# 示例 6: 指定本地目录
./Lunar-Astral-Agents.exe -local-dir ./my_data

# 示例 7: 组合使用
./Lunar-Astral-Agents.exe -developer -clear-port -basic-port 36789 -tts-url http://localhost:7860
```

---

*文档版本：1.0 | 最后更新：2026-05-09*

[返回主页](./README.md) | [查看 API 文档](./api/index.md)
