# 11 stable-diffusion.cpp 图像生成参数手册

> [🏠 手册门户](README.md) ｜ [◀ 上一模块](10-llama.cpp-参数参考.md) ｜ [下一模块 ▶](12-图像生成参数策略.md)

## 一、功能介绍

stable-diffusion.cpp 是平台内置的图像生成引擎，其命令行工具 sd-cli 与服务端 sd-server 负责承载文生图、图生图、图像超分辨率、视频生成与图像元数据读取等任务：它以本地 safetensors/GGUF 模型与配套文本编码器、VAE、超分模型完成扩散采样并输出图像，月华的绘图链路都经由它完成出图。本手册是 sd-cli 与 sd-server 全部运行参数的速查手册，同时涵盖 RealESRGAN 超分相关参数，用于在你需要调整出图尺寸、步数、采样器、显存策略或超分行为时快速查清参数含义、默认值与推荐取值。管线层面的调参策略见 [12-图像生成参数策略](12-图像生成参数策略.md)，在平台中的角色见 [02-核心系统-钛宇-月华](02-核心系统-钛宇-月华.md)，运行与部署方式见 [08-构建运行与配置](08-构建运行与配置.md)。

## 二、使用方法

本手册不是「必读」，多数情况下使用平台默认参数即可正常出图。只在出现下表场景时，才需要针对性地查阅并调整对应参数。

| 你想解决的问题 | 建议先看的参数分组 |
|----------------|--------------------|
| 调整出图尺寸与比例 | 生成选项（`--width`、`--height`、`--hires-*`） |
| 调整画质、步数与采样器 | 生成选项（`--steps`、`--sampling-method`、`--scheduler`、`--cfg-scale`、`--guidance`） |
| 显存不足或爆显存 | 上下文选项（`--max-vram`、`--offload-to-cpu`、`--vae-tiling`、`--split-mode`、`--type`） |
| 出图全黑、噪点或伪影 | 生成选项（`--cfg-scale`、`--eta`、`--strength`、`--vae-*` 分块） |
| 生成过慢 | 上下文选项（`--type` 量化、`--fa`）与生成选项（缓存 `--cache-mode`、`--steps`） |
| 需要局部重绘或参考图控制 | 生成选项（`--init-img`、`--mask`、`--control-image`、`--ref-image`、ControlNet/IP-Adapter 相关） |
| 需要放大图像或超分 | 生成选项（`--mode upscale`、`--upscale-model`、`--upscale-repeats`、`--upscale-tile-size`） |
| 作为服务被多次调用 | 服务端选项（`--listen-ip`、`--listen-port`、`--serve-html-path`） |

## 三、功能特性

| 特性 | 说明 |
|------|------|
| 文生图 | 依据提示词从噪声直接生成图像，支持宽高、步数、CFG 等控制 |
| 图生图与重绘 | 以初始图像加噪去噪，支持 `--strength` 强度控制与掩码局部重绘 |
| 参考图与风格控制 | 支持参考图、ControlNet 控制图像/视频、IP-Adapter、PuLID、PHOTOMAKER 等条件引导 |
| 视频生成 | 通过多帧输出与 motion 模块生成视频，支持帧率与帧数控制 |
| VAE 与解码 | 支持独立 VAE、taesd 快速解码、VAE 分块与圆形填充、卷积直算路径 |
| 采样器与调度器 | 内置多种采样方法与 sigma 调度器，可按模型与场景选择 |
| 显存分片与卸载 | 支持图切割执行、按模块后端分配、权重卸载到内存、多 GPU 拆分与 RPC 卸载 |
| 超分辨率 | 通过 RealESRGAN 模型对图像进行多轮放大，支持图块大小控制 |
| 缓存加速 | 支持 easycache、ucache、dbcache、spectrum 等多种缓存策略 |
| HTTP 服务 | sd-server 常驻模型并对外暴露 OpenAI 兼容、WebUI 兼容与原生三套接口族 |
| 元数据读写 | 可读取图像内嵌生成元数据，也可选择是否写入元数据 |

## 四、运行参数

### sd-server 服务端选项

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-l`, `--listen-ip <string>` | 服务端监听 IP | 127.0.0.1 | 需要外部访问时改为对外地址，注意安全 |
| `--serve-html-path <string>` | 在根路径提供的 HTML 文件路径 | 可选（空） | 需要用自定义前端覆盖内嵌页面时设置 |
| `--listen-port <int>` | 服务端监听端口 | 1234 | 端口冲突时更换 |
| `-v`, `--verbose` | 打印额外信息 | — | 排障时开启 |
| `--color` | 根据级别为日志标签着色 | — | 仅便于阅读日志 |
| `-h`, `--help` | 显示帮助并退出 | — | 只用于自查参数 |

### 上下文选项（服务端/新版本新增）

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--uncond-diffusion-model <string>` | 独立无条件扩散模型路径，当前用于 Ideogram4 CFG | — | 特定模型需要时设置 |
| `--embeddings-connectors <string>` | LTXAV embeddings 连接器路径 | — | 视频生成场景使用 |
| `--vae-format <string>` | VAE 潜在空间格式覆盖，可取 auto、flux、sd3、flux2 或 wan | auto | 一般保持 auto |
| `--audio-vae <string>` | 独立 LTX audio vae 模型路径 | — | 含音频的视频生成场景 |
| `--ip-adapter <string>` | IP-Adapter 模型路径，需要 `--clip_vision` | — | 风格迁移场景使用 |
| `--motion-module <string>` | AnimateDiff motion 模块路径（SD 1.5），`--video-frames` 大于 1 时启用视频生成 | — | 需要动画效果时设置 |
| `--model-args <string>` | 额外模型参数，键值对列表，支持 chroma_use_dit_mask 等开关 | — | 特定模型微调时使用 |
| `--pulid-weights <string>` | PuLID Flux 权重路径 | — | 人脸身份保持场景使用 |
| `--split-mode <string>` | 分配到多设备模块的权重分布方式，layer 为每设备整块 transformer，row 为跨设备拆分 matmul 行（仅 CUDA） | layer | 多卡优先 layer |
| `--rpc-servers <string>` | 用于卸载的 RPC 服务器列表，逗号分隔，格式为 host:port | — | 跨机卸载算力时使用 |
| `--max-vram <string>` | 图切割分段执行的最大 VRAM 预算（GiB），可按后端/设备分别指定 | 0（禁用图分割） | 显存不足时设为可用显存附近 |
| `--stream-layers` | 在 `--max-vram` 之上启用驻留加预取流式，无 `--max-vram` 时无效 | false | 图切割基础上进一步省显存 |
| `--eager-load` | 模型加载时即将所有参数载入 params 后端，而非首次使用时惰性加载 | false | 追求首图速度时开启 |
| `--auto-fit` | 根据模型大小与各设备内存预算自动挑选 diffusion/te/vae 的设备放置，可能跨 GPU 拆分模块 | 空闲内存减去少量余量 | 不知道如何分配时交由自动决策 |
| `--list-devices` | 列出可用的后端设备并退出，名称即后端分配参数接受的设备名 | — | 配置后端前先执行它确认设备名 |
| `--control-net-cpu` | 已弃用，改用按模块后端分配方式将 controlnet 指向 CPU | — | 请勿使用，改用新方式 |
| `--clip-on-cpu` | 已弃用，改用按模块后端分配方式将文本编码器指向 CPU | — | 请勿使用，改用新方式 |
| `--vae-on-cpu` | 已弃用，改用按模块后端分配方式将 VAE 指向 CPU | — | 请勿使用，改用新方式 |

### 生成选项（服务端/新版本新增）

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `--ad-model <string>` | ADetailer 用 YOLOv8 检测模型（转换后）路径 | — | 需要面部/细节自动重绘时设置 |
| `--ad-prompt <string>` | ADetailer 提示词，空则继承主提示词，支持占位标签 | — | 一般留空继承主提示词 |
| `--ad-negative-prompt <string>` | ADetailer 负面提示词，空则继承主负面提示词 | — | 一般留空继承 |
| `--extra-ad-args <string>` | ADetailer 额外参数，键值对列表，支持检测与重绘相关开关 | — | 高级调优时使用 |
| `--ip-adapter-image <string>` | IP-Adapter 参考图像路径 | — | 风格迁移场景必填 |
| `--pulid-id-embedding <string>` | PuLID id embedding 路径 | — | 身份保持场景使用 |
| `--pulid-id-weight <float>` | PuLID 身份注入强度 | — | 越高越像参考身份 |
| `--ip-adapter-strength <float>` | 应用 IP-Adapter 的强度 | 1.0 | 风格过强时可下调 |
| `--extra-tiling-args <string>` | VAE 分块额外参数，键值对列表，LTX 视频 VAE 支持时间分块帧数与重叠 | — | 视频解码分块时使用 |
| `--ref-image-args <string>` | 设置参考图像处理方式的键值对列表，空则从模型权重自动检测 | 空（自动检测） | 一般为空 |
| `--qwen-image-layers <int>` | Qwen Image Layered 层数，latent/output 数量为层数加 1 | 3 | 一般保持默认 |
| `--temporal-tiling` | 为 LTX 视频 VAE 解码启用时间分块 | — | 长视频解码省显存时开启 |
| `--ref-video <string>` | MiniMax-H3 Ref2VA 参考视频帧目录（24 fps），可多次使用 | — | 参考视频生成场景使用 |
| `--ref-video-audio <string>` | 与 `--ref-video` 按索引配对的 WAV 音轨，可多次使用 | — | 配套参考音频时使用 |
| `--ref-audio <string>` | MiniMax-H3 Ref2VA 的独立 WAV 参考，可多次使用 | — | 独立音频参考时使用 |
| `--prompt-file <string>` | 包含要渲染提示词的文件路径 | — | 提示词很长时用文件 |
| `--negative-prompt-file <string>` | 包含负面提示词的文件路径 | — | 负面提示词很长时用文件 |
| `--hires-sigmas <string>` | highres fix 第二遍的自定义 sigma 值，逗号分隔 | — | 需要精细控制二次采样时使用 |

> 说明：相比早期版本，采样方法与调度器选项有所扩充，采样方法新增 `dpm++2m_sde`、`dpm++2m_sde_bt`、`lms`；调度器新增 `ltx2`、`logit_normal`、`flux2`、`flux`、`beta`；预测类型新增 `sefi_flow`；噪声乘数默认取值列表也相应扩充。具体可用取值以实际运行时为准。

### HTTP 接口与请求字段

sd-server 常驻模型并对外提供三套接口族，启动后可用浏览器访问 `http://127.0.0.1:1234/` 使用内嵌 Web UI，监听地址与端口可调整，也可用自定义 HTML 覆盖内嵌前端。

| 接口族 | 前缀 | 形态 | 用途 |
|--------|------|------|------|
| OpenAI 兼容 | `/v1/...` | 同步 JSON | 兼容 OpenAI 图片接口，最通用 |
| WebUI 兼容 | `/sdapi/v1/...` | 同步 JSON | 兼容 A1111/WebUI 客户端工具 |
| 原生 sdcpp | `/sdcpp/v1/...` | 异步任务 | 提供完整原生参数控制与能力发现 |

> 通用约束：三种兼容接口均不解析提示词内嵌的 LoRA 标签，LoRA 必须通过结构化字段传递。

文生图接口请求字段：

| 字段 | 类型 | 说明 | 建议 |
|------|------|------|------|
| `prompt` | 字符串 | 推理提示词，必填 | 描述越具体越可控 |
| `n` | 整数 | 生成图片数量 | 按需设置 |
| `size` | 字符串 | 尺寸，格式为「宽x高」 | 宽高建议为 32 的倍数 |
| `output_format` | 字符串 | 输出格式，可取 png、jpeg 或 webp | 需要无损时用 png |
| `output_compression` | 整数 | 输出压缩率，取值会被截断到 0 到 100 | 有损格式时按需调整 |

原生参数扩展：OpenAI 与 WebUI 兼容接口只暴露有限字段，若需使用原生控制（步数、CFG、采样器、调度器、随机种子、VAE 分块、引导等），可在提示词中内嵌一个名为 `sd_cpp_extra_args` 的 JSON 扩展块，其结构与原生接口请求一致。服务端会抽取该块解析，并将其从最终提示词中移除后再参与生成。该机制不适用于原生接口，因为原生接口请求体已直接使用原生结构。

### CLI 选项

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-o`, `--output <string>` | 结果图像写入路径，可用序列格式生成图像序列，单文件视频输出支持 .avi、.webm 与动画 .webp | ./output.png | 需要批量输出时用序列格式 |
| `--image <string>` | 要检查的图像路径（元数据模式） | — | 读取元数据时指定 |
| `--metadata-format <string>` | 元数据输出格式，可取 text 或 json | text | 程序化处理时用 json |
| `--preview-path <string>` | 预览图像写入路径，多帧预览支持 .avi、.webm 与动画 .webp | ./preview.png | 需要预览时设置 |
| `--preview-interval <int>` | 图像预览文件连续更新之间的去噪步数间隔 | 1（每步更新） | 降低更新频率可省开销 |
| `--output-begin-idx <int>` | 输出图像序列的起始索引，必须为非负数 | 视输出路径是否含序列格式而定 | 一般保持默认 |
| `--canny` | 应用 canny 预处理器（边缘检测） | — | 需要边缘控制图时开启 |
| `--convert-name` | 转换张量名称（用于转换模式） | — | 转换模型时使用 |
| `-v`, `--verbose` | 打印额外信息 | — | 排障时开启 |
| `--color` | 根据级别为日志标签着色 | — | 仅便于阅读日志 |
| `--taesd-preview-only` | 阻止使用 taesd 解码最终图像（与预览 tae 配合使用） | — | 仅需快速预览时开启 |
| `--preview-noisy` | 启用预览模型的嘈杂输入而不是去噪输出 | — | 观察噪声过程时使用 |
| `--metadata-raw` | 包含未解析元数据负载的原始十六进制预览 | — | 深挖元数据时开启 |
| `--metadata-brief` | 在文本输出中截断长元数据文本值 | — | 保持输出简洁 |
| `--metadata-all` | 包含结构/容器条目，如 IHDR、IDAT 与非元数据 JPEG 段 | — | 需要完整结构信息时开启 |
| `-M`, `--mode` | 运行模式，可取 img_gen、vid_gen、upscale、convert、metadata | img_gen | 超分时用 upscale |
| `--preview` | 预览方法，可取 none、proj、tae、vae | none | 需要预览时选择对应方法 |
| `-h`, `--help` | 显示帮助并退出 | — | 只用于自查参数 |

### 上下文选项

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-m`, `--model <string>` | 完整模型路径 | — | 必填，指向主模型 |
| `--clip_l <string>` | clip-l 文本编码器路径 | — | 按模型配套设置 |
| `--clip_g <string>` | clip-g 文本编码器路径 | — | 按模型配套设置 |
| `--clip_vision <string>` | clip-vision 编码器路径 | — | 使用 IP-Adapter 等时必需 |
| `--t5xxl <string>` | t5xxl 文本编码器路径 | — | 按模型配套设置 |
| `--llm <string>` | llm 文本编码器路径，按模型选择对应模型 | — | 新型模型按需设置 |
| `--llm_vision <string>` | llm 视觉编码器路径 | — | 需要视觉输入时设置 |
| `--qwen2vl <string>` | `--llm` 的别名，已弃用 | — | 请勿使用，改用 `--llm` |
| `--qwen2vl_vision <string>` | `--llm_vision` 的别名，已弃用 | — | 请勿使用，改用 `--llm_vision` |
| `--diffusion-model <string>` | 独立扩散模型路径 | — | 分离式模型时设置 |
| `--high-noise-diffusion-model <string>` | 独立高噪声扩散模型路径 | — | 高噪声阶段单独模型时设置 |
| `--vae <string>` | 独立 vae 模型路径 | — | 模型与 VAE 分离时设置 |
| `--taesd <string>` | taesd 路径，使用 Tiny AutoEncoder 快速解码（低质量） | — | 追求速度的预览场景 |
| `--tae <string>` | `--taesd` 的别名 | — | 与 `--taesd` 同义 |
| `--control-net <string>` | 控制网模型路径 | — | 需要 ControlNet 引导时设置 |
| `--embd-dir <string>` | embeddings 目录 | — | 使用文本反演时设置 |
| `--lora-model-dir <string>` | lora 模型目录 | — | 使用 LoRA 时设置 |
| `--hires-upscalers-dir <string>` | highres fix 上采样器模型目录 | — | 使用模型型上采样器时设置 |
| `--tensor-type-rules <string>` | 每个张量模式的权重类型，按张量名模式映射到类型 | — | 精细控制各张量量化时使用 |
| `--photo-maker <string>` | PHOTOMAKER 模型路径 | — | 使用 PHOTOMAKER 时设置 |
| `--upscale-model <string>` | esrgan 模型路径 | — | 超分时必填 |
| `--backend <string>` | 运行时后端分配，可整体指定或按模块指定 | — | 多设备时按模块分配 |
| `--params-backend <string>` | 参数后端分配，可整体或按模块指定 | — | 控制权重放置位置 |
| `-t`, `--threads <int>` | 计算时使用的线程数，小于等于 0 时设为 CPU 物理核心数 | -1 | 纯 CPU 时按物理核心数 |
| `--chroma-t5-mask-pad <int>` | chroma 的 t5 掩码填充大小 | — | 特定模型微调时使用 |
| `--max-vram <float>` | 图切割分割执行的最大 VRAM 预算（GiB），0 禁用图分割，-1 自动检测可用 VRAM 减去 1 GiB | 未设置 | 爆显存时设为此卡可用值 |
| `--force-sdxl-vae-conv-scale` | 强制在 sdxl vae 上使用卷积缩放 | — | 特定 VAE 异常时尝试 |
| `--offload-to-cpu` | 将权重放置于内存以节省显存，并在需要时自动载入显存 | — | 显存紧张时开启，以速度换显存 |
| `--mmap` | 是否内存映射模型 | — | 一般保持默认 |
| `--control-net-cpu` | 将 controlnet 保留在 cpu（用于低显存） | — | 低显存场景开启 |
| `--clip-on-cpu` | 将文本编码器保留在 cpu（用于低显存） | — | 低显存场景开启 |
| `--vae-on-cpu` | 将 vae 保留在 cpu（用于低显存） | — | 低显存场景开启 |
| `--fa` | 使用 flash attention | — | 加速并省显存 |
| `--diffusion-fa` | 仅在扩散模型中使用 flash attention | — | 只对扩散模型启用 |
| `--diffusion-conv-direct` | 在扩散模型中使用卷积直算路径 | — | 兼容性或性能排障时使用 |
| `--vae-conv-direct` | 在 vae 模型中使用卷积直算路径 | — | 同上 |
| `--circular` | 启用卷积的圆形填充 | — | 生成无缝纹理时使用 |
| `--circularx` | 仅在 x 轴（宽度）上启用圆形 RoPE 包装 | — | 横向无缝时使用 |
| `--circulary` | 仅在 y 轴（高度）上启用圆形 RoPE 包装 | — | 纵向无缝时使用 |
| `--chroma-disable-dit-mask` | 禁用 chroma 的 dit 掩码 | — | 特定模型微调 |
| `--qwen-image-zero-cond-t` | 为 qwen image 启用 zero_cond_t | — | 特定模型微调 |
| `--chroma-enable-t5-mask` | 为 chroma 启用 t5 掩码 | — | 特定模型微调 |
| `--type` | 权重类型，如 f32、f16、q4_0、q4_1、q5_0、q5_1、q8_0 及各类 K 量化 | 未指定（用权重文件类型） | 显存紧张时用量化类型 |
| `--rng` | RNG，可取 std_default、cuda、cpu | cuda（sd-webui）或 cpu（comfyui） | 一般保持默认 |
| `--sampler-rng` | 采样器 RNG，可取 std_default、cuda、cpu | 未指定时使用 `--rng` | 一般保持默认 |
| `--prediction` | 预测类型覆盖，可取 eps、v、edm_v、sd3_flow、flux_flow、flux2_flow、sefi_flow | 未指定（按模型） | 一般保持默认 |
| `--lora-apply-mode` | 应用 LoRA 的方式，可取 auto、immediately、at_runtime | auto | 量化权重下建议保持 auto |

### 生成选项

| 参数 | 说明 | 默认值 | 建议 |
|------|------|--------|------|
| `-p`, `--prompt <string>` | 要渲染的提示词 | — | 必填 |
| `-n`, `--negative-prompt <string>` | 负面提示词 | 空 | 需要规避内容时填写 |
| `-i`, `--init-img <string>` | 初始图像路径 | — | 图生图时必填 |
| `--end-img <string>` | 结束图像路径，flf2v 必需 | — | 首尾帧视频场景使用 |
| `--mask <string>` | 掩码图像路径 | — | 局部重绘时设置 |
| `--control-image <string>` | 控制图像路径，用于控制网 | — | ControlNet 场景必填 |
| `--control-video <string>` | 控制视频帧路径，必须是目录且帧按字典序以图像存储 | — | 视频控制场景设置 |
| `--pm-id-images-dir <string>` | PHOTOMAKER 输入 id 图像目录路径 | — | PHOTOMAKER 场景使用 |
| `--pm-id-embed-path <string>` | PHOTOMAKER v2 id 嵌入路径 | — | PHOTOMAKER 场景使用 |
| `--hires-upscaler <string>` | highres fix 上采样器，可选 Lanczos、Nearest、Latent 及其变体，或上采样器目录下的模型名 | Latent | 一般保持默认 |
| `--extra-sample-args <string>` | 额外采样器参数，键值对列表，目前 lcm 支持噪声裁剪与缩放相关项 | — | 高级调优时使用 |
| `-H`, `--height <int>` | 图像高度，像素空间 | 512 | 建议为 32 的倍数 |
| `-W`, `--width <int>` | 图像宽度，像素空间 | 512 | 建议为 32 的倍数 |
| `--steps <int>` | 采样步数 | 20 | 画质优先可提高，过高收益递减 |
| `--high-noise-steps <int>` | 高噪声采样步数 | -1（自动） | 一般保持自动 |
| `--clip-skip <int>` | 忽略 CLIP 网络的最后层，1 不忽略、2 忽略一层，小于等于 0 表示未指定 | -1 | 一般保持默认 |
| `-b`, `--batch-count <int>` | 批量计数 | — | 一次出多张时设置 |
| `--video-frames <int>` | 视频帧数 | 1 | 视频生成时调大 |
| `--fps <int>` | 帧率 | 24 | 视频生成时按需调整 |
| `--timestep-shift <int>` | NitroFusion 模型的时间步偏移，NitroSD-Realism 推荐约 250，NitroSD-Vibrant 约 500 | 0 | 按模型说明设置 |
| `--upscale-repeats <int>` | 运行 ESRGAN 上采样器的次数 | 1 | 需要更大倍数时增加 |
| `--upscale-tile-size <int>` | ESRGAN 上采样的图块大小 | 128 | 显存不足时调小 |
| `--hires-width <int>` | highres fix 目标宽度，0 表示使用 `--hires-scale` | 0 | 需要指定尺寸时设置 |
| `--hires-height <int>` | highres fix 目标高度，0 表示使用 `--hires-scale` | 0 | 需要指定尺寸时设置 |
| `--hires-steps <int>` | highres fix 第二遍采样步数，0 表示重用 `--steps` | 0 | 第二遍要更多细节时设置 |
| `--hires-upscale-tile-size <int>` | highres fix 上采样器图块大小 | 128 | 显存不足时调小 |
| `--cfg-scale <float>` | 无条件引导比例 | 7.0 | 过高易过曝或伪影 |
| `--img-cfg-scale <float>` | inpaint 或 instruct-pix2pix 模型的图像引导比例 | 与 `--cfg-scale` 相同 | 需要强约束初始图时调整 |
| `--guidance <float>` | 具有引导输入的模型的蒸馏引导比例 | 3.5 | 按模型推荐值设置 |
| `--slg-scale <float>` | 跳层引导比例，仅适用于 DiT 模型，0 表示禁用 | 0 | sd3.5 medium 可试 2.5 |
| `--skip-layer-start <float>` | SLG 启用点 | 0.01 | 一般保持默认 |
| `--skip-layer-end <float>` | SLG 禁用点 | 0.2 | 一般保持默认 |
| `--eta <float>` | 噪声乘数，取值默认依采样器而定 | 视采样器而定 | 一般保持默认 |
| `--flow-shift <float>` | Flow 模型（如 SD3.x、WAN）的偏移值 | 自动 | 一般保持自动 |
| `--high-noise-cfg-scale <float>` | 高噪声无条件引导比例 | 7.0 | 一般保持默认 |
| `--high-noise-img-cfg-scale <float>` | 高噪声 inpaint 或 instruct-pix2pix 的图像引导比例 | 与 `--cfg-scale` 相同 | 一般保持默认 |
| `--high-noise-guidance <float>` | 高噪声蒸馏引导比例 | 3.5 | 一般保持默认 |
| `--high-noise-slg-scale <float>` | 高噪声跳层引导比例，仅适用于 DiT 模型 | 0 | 一般保持默认 |
| `--high-noise-skip-layer-start <float>` | 高噪声 SLG 启用点 | 0.01 | 一般保持默认 |
| `--high-noise-skip-layer-end <float>` | 高噪声 SLG 禁用点 | 0.2 | 一般保持默认 |
| `--high-noise-eta <float>` | 高噪声噪声乘数，取值默认依采样器而定 | 视采样器而定 | 一般保持默认 |
| `--strength <float>` | 加噪/去噪强度 | 0.75 | 图生图改动幅度大时调高 |
| `--pm-style-strength <float>` | PHOTOMAKER 风格强度 | — | PHOTOMAKER 场景使用 |
| `--control-strength <float>` | 应用 ControlNet 的强度，1.0 对应完全破坏初始图像信息 | 0.9 | 约束不足时调高 |
| `--moe-boundary <float>` | Wan2.2 MoE 模型的时间步边界，仅在 `--high-noise-steps` 为 -1 时启用 | 0.875 | 一般保持默认 |
| `--vace-strength <float>` | wan vace 强度 | — | vace 场景使用 |
| `--vae-tile-overlap <float>` | vae 分块的图块重叠，以图块大小的分数表示 | 0.5 | 有接缝时适当调低 |
| `--hires-scale <float>` | 未设置目标大小时 highres fix 的比例 | 2.0 | 需要更大倍率时调高 |
| `--hires-denoising-strength <float>` | highres fix 第二遍去噪强度 | 0.7 | 细节不足时调高 |
| `--increase-ref-index` | 根据参考图像列出的顺序自动增加其索引（从 1 开始） | — | 多参考图场景使用 |
| `--disable-auto-resize-ref-image` | 禁用参考图像的自动调整大小 | — | 需要保持原始尺寸时开启 |
| `--disable-image-metadata` | 不在图像文件中嵌入生成元数据 | — | 需要更小文件时开启 |
| `--vae-tiling` | 以分块方式处理 vae 以减少内存使用 | — | 显存紧张时开启，注意接缝 |
| `--hires` | 启用 highres fix | — | 需要二次放大修复时开启 |
| `-s`, `--seed` | RNG 种子，小于 0 表示随机 | 42 | 需要复现时设固定值 |
| `--sampling-method` | 采样方法，可取 euler、euler_a、heun、dpm2、dpm++2s_a、dpm++2m、dpm++2mv2、dpm++2m_sde、dpm++2m_sde_bt、ipndm、ipndm_v、lcm、ddim_trailing、tcd、res_multistep、res_2s、er_sde、lms、euler_cfg_pp、euler_a_cfg_pp | euler（Flux/SD3/Wan），euler_a（其他） | 按模型与题材选择 |
| `--high-noise-sampling-method` | 高噪声采样方法，可选值与 `--sampling-method` 相同 | euler（Flux/SD3/Wan），euler_a（其他） | 一般与主采样方法一致 |
| `--scheduler` | 去噪器 sigma 调度器，可取 discrete、karras、exponential、ays、gits、smoothstep、sgm_uniform、simple、kl_optimal、lcm、bong_tangent、ltx2、logit_normal、flux2、flux、beta | discrete（具体视模型而定） | 一般保持默认 |
| `--sigmas` | 采样器的自定义 sigma 值，逗号分隔 | — | 需要精细控制采样时使用 |
| `--skip-layers` | SLG 步骤要跳过的层 | 7,8,9 | 使用 SLG 时按需调整 |
| `--high-noise-skip-layers` | 高噪声 SLG 步骤要跳过的层 | 7,8,9 | 使用 SLG 时按需调整 |
| `-r`, `--ref-image` | Flux Kontext 模型的参考图像，可多次使用 | — | 参考图引导场景使用 |
| `--cache-mode` | 缓存方法，可取 easycache（DiT）、ucache（UNET）、dbcache/taylorseer/cache-dit（DiT 块级）、spectrum（UNET/DiT 预测） | — | 追求速度时可尝试 |
| `--cache-option` | 命名缓存参数，键值对格式、逗号分隔 | — | 与缓存方法配套调整 |
| `--scm-mask` | cache-dit 的 SCM 步骤掩码，逗号分隔的 0/1，1 表示计算、0 表示可缓存 | — | 高级缓存调优 |
| `--scm-policy` | SCM 策略，可取 dynamic 或 static | dynamic | 一般保持默认 |
| `--vae-tile-size` | vae 分块的图块大小，格式为 宽x高 | 32x32 | 有接缝时调大图块 |
| `--vae-relative-tile-size` | vae 分块的相对图块大小，小于 1 为图像大小分数，大于等于 1 为每维度图块数量，覆盖 `--vae-tile-size` | — | 需要相对分块时使用 |

## 五、常见排故

| 现象 | 可能原因 | 处理办法 |
|------|----------|----------|
| 显存不足或爆显存 | 分辨率与批量过大、权重与 VAE 未优化 | 降低宽高与批量，设置 `--max-vram`，开启 `--offload-to-cpu`、`--vae-tiling`、`--diffusion-fa`，将 `--type` 改为量化类型，用后端分配把文本编码器或 VAE 放到 CPU |
| 出图全黑 | 提示词或负面提示词失衡、CFG 或引导异常、VAE 不匹配 | 检查 `--cfg-scale`、`--guidance`，确认 VAE 与模型配套，必要时用 `--vae-format` 覆盖 |
| 出图全是噪点或糊 | 步数过低、强度或噪声乘数异常、模型与预测类型不匹配 | 提高 `--steps`，检查 `--eta`、`--strength` 与 `--prediction` |
| 画面出现规则接缝或网格 | VAE 分块几何与重叠不当 | 调整 `--vae-tile-size`、`--vae-relative-tile-size`、`--vae-tile-overlap`，或关闭 `--vae-tiling` |
| 生成过慢 | 未量化权重、未启用加速、缓存未开 | 使用量化 `--type`，开启 `--fa`，尝试 `--cache-mode`，降低 `--steps` 或分辨率 |
| 模型或 VAE 不匹配报错 | 文本编码器、VAE 与主模型不配套 | 核对 `--clip_l`、`--clip_g`、`--t5xxl`、`--llm`、`--vae` 是否与模型对应，必要时指定 `--vae-format` |
| 超分失败或结果异常 | 未指定超分模型、图块过大或倍率不当 | 设置 `--upscale-model`，将 `--mode` 设为 upscale，调小 `--upscale-tile-size`，合理设置 `--upscale-repeats` |
| 服务启动后无法访问 | 端口被占用或监听地址受限 | 更换 `--listen-port`，或将 `--listen-ip` 改为对外监听地址 |
| 通过兼容接口传 LoRA 不生效 | 兼容接口不解析提示词内嵌 LoRA 标签 | 改用结构化字段传递 LoRA，或使用原生接口族 |
| 参考图或控制图无效果 | 强度过低、参考处理参数不当、编码器缺失 | 调高 `--control-strength`、`--ip-adapter-strength`，确认 `--clip_vision` 等编码器已提供，必要时调整参考图处理参数 |

## 六、引用跳转

- [02-核心系统-钛宇-月华](02-核心系统-钛宇-月华.md)
- [08-构建运行与配置](08-构建运行与配置.md)
- [12-图像生成参数策略](12-图像生成参数策略.md)
