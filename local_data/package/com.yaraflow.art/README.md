# 绘画插件 com.yaraflow.art

YaraFlow 绘画工具插件，根据聊天内容自主生成图片并把图片发送回当前群聊。供 LLM（Planner）通过 Function Calling 自动调用。支持多个绘图后端切换、自我形象图生图（画自己）、引用聊天图片做参考图生图、每日次数上限零点自动重置，以及火山方舟 Seedream 5.0 pro 的图层拆分能力。

## 特点

- 🎨 **LLM 工具**：不是指令型插件，由 Planner 自主判断场景并调用
- 🔧 **多后端切换**：内置 8 种接口格式（openai / openai-chat / doubao / gemini / modelscope / shatangyun / mengyuai / zai / comfyui），可配置多个后端按需切换
- 🧍 **画自己（自我形象）**：使用配置的自我形象照片做图生图，保持身份特征不变；被明确要求换装时可按指令改服装
- 🖼️ **参考图生图**：引用聊天记录里的 `[图片N]` 做参考；多张图片或「参考图 + 画自己」自动进入多图融合（仅火山方舟后端支持）
- 🔢 **每日上限**：每日绘图次数上限，零点自动重置，持久化到 `data/daily_count.json`
- 🧱 **拆图（图层拆分）**：检测到火山方舟 Seedream 5.0 pro（即梦）后端时自动注册 `layer_split`，把图片拆为底图 + 最多 16 个透明 PNG 图层

## 工具

| 工具 | 作用 | 参考图片序号 | 说明 |
|------|------|-------------|------|
| `generate_picture` | 生成图片并发送到群聊 | `image_refs`（`"1"` / `"1,3"` / `"[1,3]"`） | 主绘图工具；`draw_self=true` 画自己；单张参考图做普通图生图，多张或「参考图+画自己」进入多图融合 |
| `layer_split` | 把图片拆为底图和多个独立图层 | `image_ref` | 仅火山方舟 Seedream 5.0 pro 支持；按配置模型条件注册，手动关闭优先 |

## 配置（config.yaml）

配置文件由插件首次运行时自动生成，全部含中文注释。节点总览：

```
plugin.enabled            插件总开关（true/false）
models.default_model_id   默认后端 ID（下拉框选项自动同步自 backends）
models.backends           后端列表：键为后端 ID，值为后端参数
selfie.enabled            是否启用自我形象功能
selfie.image_path         自我形象图片路径（相对插件目录，如 data/selfie.png）
selfie.role_description   角色设定描述（画自己时同步给优化模型作形象参考）
limits.daily_max          每日最多绘图次数（1-1000，零点自动重置）
layer_split.enabled       拆图工具开关（true 检测到 Seedream 即注册；false 手动关闭）
```

### 后端（models.backends）字段

| 字段 | 说明 |
|------|------|
| `name` | 后端显示名称（中文，供下拉框对照） |
| `base_url` | API 接口地址 |
| `api_key` | 密钥。openai/doubao/modelscope/mengyuai/zai 裸密钥自动补 `Bearer`；gemini 走 `x-goog-api-key` 无需 Bearer；shatangyun 的 token 放 URL 无需 Bearer；comfyui 无需密钥 |
| `model` | 模型标识（comfyui 为工作流文件名） |
| `format` | 接口格式：`openai`（OpenAI 兼容，含硅基流动/火山方舟/NewAPI）/ `openai-chat` / `doubao` / `gemini` / `modelscope`（魔搭异步）/ `shatangyun`（砂糖云 NovelAI）/ `mengyuai` / `zai` / `comfyui`（本地工作流） |
| `default_size` | 默认图片尺寸（如 `1024x1024`、`2K`）；参数错误自动回退最高可用 |
| `support_img2img` | 是否支持图生图（不支持时强制降级文生图） |
| `strength` | 图生图强度 0.1~1.0，越低越偏离原图，默认 0.5 |
| `watermark` | 仅火山方舟，布尔，`true` 加 "AI 生成" 水印 / `false` 不加，默认不加 |
| `prompt_optimize_mode` | 仅火山方舟，`standard` 标准质量优先 / `fast` 极速，默认 `standard` |
| `background` | 仅火山方舟，`opaque` 不透明 / `transparent` 透明背景（透明仅 PNG 图生图生效），默认 `opaque` |
| `custom_prompt_add` | 附加正面词，提升画质（如 best quality、masterpiece） |
| `negative_prompt_add` | 附加负面词，去除瑕疵（如 lowres、bad hands、watermark） |

## 使用方式

用户在群里描述画面，Planner 调用 `generate_picture` 生图后自动把图片发回群聊。触发场景示例：

- "画一张黄昏海滩的图"
- "画你自己，穿着裙子站在花田里"（`draw_self=true`）
- "参考我刚发的这张图片，画一张赛博朋克风格的"（`image_refs="1"`）
- "把我发的两张图融合成一张"（`image_refs="[1,2]"`）
- "把这张图拆成人物、背景、文字"（`layer_split`，需即梦后端）

## 技术架构

```
用户消息 → Planner 识别意图 → 调用 generate_picture(prompt, draw_self?, image_refs?, style?, model_id?, size?)
  → 自我形象/参考图读取（自我形象经主程序字节级校验转 base64，参考图优先主程序缓存）
  → 提示词优化（LLM，常规公式 或 自我形象场景公式，保留身份/换装语义）
  → 按 format 分发到对应后端 → 图片生成
  → 发送回群聊（每日计数自增，达上限拒绝）
```

## 权限声明

- `tool.register` — 注册 LLM 工具
- `http.request` — 调用绘图后端 API
- `plugin.config.read/write` — 读写配置
- `plugin.file.read/write` — 读写自我形象图、下载参考图、持久化每日计数
- `model.access` — 提示词优化
- `send.image` / `send.hybrid` — 发送图片/多段消息
- `encoding.use` / `time.use` — base64 编解码与每日计数时间

## 许可证

GPL-3.0