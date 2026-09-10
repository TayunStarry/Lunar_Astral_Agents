# LTP9 绘图插件

根据提示词调用绘图后端生成图片。补齐了自我形象(selfie)、参考图、多图融合（火山方舟）、
LLM 提示词优化、魔搭/Gemini/ComfyUI 后端，并可经 `engine.send` 发送到会话。

## 插件能力

- **同步模型**：网络走 `engine.http.get/post`（同步阻塞），图片 `engine.image`，LLM `engine.llm`，
  发送 `engine.send`，等待用 `engine.sleep`，不产生 Promise。
- **后端格式**：`openai`（含火山方舟 `watermark`/`prompt_optimize_mode`/透明背景 与多图融合）、
  `modelscope`（异步任务+结果轮询）、`gemini`（inlineData base64）、`comfyui`（工作流占位符+history 轮询）
- **图像输入**：`draw_self`（`engine.image.loadValid` 读自我形象）、`image_refs`（`engine.image.download` 下载参考图）、多图融合
- **提示词优化**：`engine.llm.chat`（读取 `lunar_config.json` agent 字段）
- **图层拆分**：`layerSplit`（`engine.export("layerSplit")` 或 `art.layer_split` 事件），仅火山方舟 Seedream 5.0 pro；`layer_split.enabled` 手动开关
- **发送**：`context.groupId` 存在时 `engine.send.image`/`engine.send.hybrid` 发送，否则返回数据并广播
- **每日上限**、**2K 重试**

## 权限申请（用 ltp9_keygen 生成 permissions.key 时勾选）

- `allow-network`：`engine.http` 调用绘图后端、`engine.image.download` 下载参考图/结果
- `allow-file`：读写 `data/daily_count.json`、`data/selfie.png`、工作流文件、读/校验图片
- `allow-agent`：`engine.llm.chat` 提示词优化
- `allow-send`：`engine.send.image` 发送图片到会话
- `allow-signal`：广播回执

## 调用方式

1. **导出函数**：
   `engine.call("com.yaraflow.art-ltp9").run("generatePicture", [{ prompt, draw_self?, image_refs?, style?, model_id?, size? }])`（同步阻塞）
2. **事件**：客户端 `Emit("art.generate", { prompt, context }, requestID)`；结果经 `engine.signal.all` 广播。
   `context` 可含 `groupId` 与 `image_urls`（参考图直链数组）。

## 配置文件说明（config.yaml）

- `models.default_model_id` / `models.backends.*`：后端（`format`/`base_url`/`api_key`/`model`/`default_size`/`support_img2img`/`strength`/`watermark`/`prompt_optimize_mode`/`background`/`custom_prompt_add`/`negative_prompt_add`）
- `selfie.*`：自我形象开关/路径/角色描述
- `limits.daily_max`：每日最多绘图次数
- `layer_split.enabled`：拆图工具开关

> 生成权限密钥的步骤见 `subsystem/ltp9_keygen`。
