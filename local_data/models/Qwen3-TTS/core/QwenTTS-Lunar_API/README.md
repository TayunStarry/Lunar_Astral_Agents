# Qwen TTS 音频克隆 API

一个精简版的 Qwen TTS API，专注于音频克隆功能，不包含 WebUI。

## 启动方式

### Windows

#### 方式 1：使用启动脚本（推荐）

1. 先运行原来的 `start.bat` 一次，确保虚拟环境已创建并安装了所有依赖
2. 然后直接运行 `start-api.bat`

```cmd
start-api.bat
```

#### 方式 2：使用 PowerShell

1. 先运行原来的 `launch.ps1` 一次，确保虚拟环境已创建并安装了所有依赖
2. 然后运行 `start-api.ps1`

```powershell
.\start-api.ps1
```

#### 方式 3：手动启动

进入虚拟环境后手动启动：

```cmd
cd d:\Lunar_Astral_Agents\local_data\models\TTS\Qwen3\core
call venv\Scripts\activate.bat
python -m qwen-tts-api --host 0.0.0.0 --port 8000
```

## API 文档

启动成功后访问：`http://localhost:8000/docs`

## API 接口

### POST /voice-clone

音频克隆接口

**请求体示例：**
```json
{
  "model_name": "Qwen/Qwen3-TTS-12Hz-1.7B-Base",
  "text": "你好，这是一段测试语音",
  "language": "zh",
  "ref_audio_base64": "base64编码的音频数据",
  "ref_text": "参考音频对应的文本（可选）",
  "do_sample": true,
  "temperature": 0.9
}
```

**响应示例：**
```json
{
  "audio_base64": "生成的音频base64编码",
  "info": "音频生成成功，耗时：1.23s"
}
```

## 注意事项

1. 需要先运行一次原来的 WebUI 启动脚本（`start.bat` 或 `launch.ps1`），确保虚拟环境已正确配置并安装了所有依赖包
2. 模型会自动从 ModelScope 下载，首次使用可能需要较长时间
3. 生成的音频文件会保存在 `outputs` 目录中
