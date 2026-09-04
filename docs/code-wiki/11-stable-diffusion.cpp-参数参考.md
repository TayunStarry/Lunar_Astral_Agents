# 11 stable-diffusion.cpp 参数参考

> [🏠 文档地图](README.md) | [◀ 上一章](10-llama.cpp-参数参考.md)
> 覆盖：`local_data/models/document/` 下原 stable-diffusion.cpp 参数文档（中/英文两版已合并为本节，原文件已删除）

本文为 stable-diffusion.cpp `sd-cli` 命令行参数的中文速查表（commit 3633072），由上游 `--help` 输出整理翻译。与项目相关的图像生成管线见 [02-核心系统-钛宇-月华](02-核心系统-钛宇-月华.md) 与 [04-公共子系统](04-公共子系统.md) §4.4。

## CLI 选项 (CLI Options)

| 参数 | 说明 |
|------|------|
| `-o`, `--output <string>` | 结果图像写入路径。可以使用 printf 风格的 %d 格式说明符用于图像序列（默认：./output.png）（例如 output_%03d.png）。单文件视频输出支持 .avi、.webm 和动画 .webp |
| `--image <string>` | 要检查的图像路径（用于元数据模式） |
| `--metadata-format <string>` | 元数据输出格式，之一 [text, json]（默认：text） |
| `--preview-path <string>` | 预览图像写入路径（默认：./preview.png）。多帧预览支持 .avi、.webm 和动画 .webp |
| `--preview-interval <int>` | 图像预览文件连续更新之间的去噪步数间隔（默认为 1，意味着每步更新） |
| `--output-begin-idx <int>` | 输出图像序列的起始索引，必须为非负数（如果在输出路径中指定了 %d 则默认为 0，否则为 1） |
| `--canny` | 应用 canny 预处理器（边缘检测） |
| `--convert-name` | 转换张量名称（用于转换模式） |
| `-v`, `--verbose` | 打印额外信息 |
| `--color` | 根据级别为日志标签着色 |
| `--taesd-preview-only` | 阻止使用 taesd 解码最终图像（与 --preview tae 配合使用） |
| `--preview-noisy` | 启用预览模型的嘈杂输入而不是去噪输出 |
| `--metadata-raw` | 包含未解析元数据负载的原始十六进制预览 |
| `--metadata-brief` | 在文本输出中截断长元数据文本值 |
| `--metadata-all` | 包含结构/容器条目，如 IHDR、IDAT 和非元数据 JPEG 段 |
| `-M`, `--mode` | 运行模式，之一 [img_gen, vid_gen, upscale, convert, metadata]，默认：img_gen |
| `--preview` | 预览方法。必须是以下之一 [none, proj, tae, vae]（默认为 none） |
| `-h`, `--help` | 显示此帮助消息并退出 |

---

## 上下文选项 (Context Options)

| 参数 | 说明 |
|------|------|
| `-m`, `--model <string>` | 完整模型路径 |
| `--clip_l <string>` | clip-l 文本编码器路径 |
| `--clip_g <string>` | clip-g 文本编码器路径 |
| `--clip_vision <string>` | clip-vision 编码器路径 |
| `--t5xxl <string>` | t5xxl 文本编码器路径 |
| `--llm <string>` | llm 文本编码器路径。例如：（qwenvl2.5 用于 qwen-image，mistral-small3.2 用于 flux2，...） |
| `--llm_vision <string>` | llm 视觉编码器路径 |
| `--qwen2vl <string>` | --llm 的别名。已弃用。 |
| `--qwen2vl_vision <string>` | --llm_vision 的别名。已弃用。 |
| `--diffusion-model <string>` | 独立扩散模型路径 |
| `--high-noise-diffusion-model <string>` | 独立高噪声扩散模型路径 |
| `--vae <string>` | 独立 vae 模型路径 |
| `--taesd <string>` | taesd 路径。使用 Tiny AutoEncoder 进行快速解码（低质量） |
| `--tae <string>` | --taesd 的别名 |
| `--control-net <string>` | 控制网模型路径 |
| `--embd-dir <string>` | embeddings 目录 |
| `--lora-model-dir <string>` | lora 模型目录 |
| `--hires-upscalers-dir <string>` | highres fix 上采样器模型目录 |
| `--tensor-type-rules <string>` | 每个张量模式的权重类型（示例："^vae\.=f16,model\.=q8_0"） |
| `--photo-maker <string>` | PHOTOMAKER 模型路径 |
| `--upscale-model <string>` | esrgan 模型路径 |
| `--backend <string>` | 运行时后端分配，例如 cpu 或 clip=cpu,vae=cuda0,diffusion=vulkan0 |
| `--params-backend <string>` | 参数后端分配，例如 cpu 或 diffusion=cpu,clip=cpu |
| `-t`, `--threads <int>` | 计算时使用的线程数（默认：-1）。如果 threads <= 0，则线程数将设置为 CPU 物理核心数 |
| `--chroma-t5-mask-pad <int>` | chroma 的 t5 掩码填充大小 |
| `--max-vram <float>` | 图切割分割执行的最大 VRAM 预算（GiB）。0 禁用图分割；-1 自动检测可用 VRAM 减去 1 GiB |
| `--force-sdxl-vae-conv-scale` | 强制在 sdxl vae 上使用 conv scale |
| `--offload-to-cpu` | 将权重放置在 RAM 中以节省 VRAM，并在需要时自动加载到 VRAM |
| `--mmap` | 是否内存映射模型 |
| `--control-net-cpu` | 将 controlnet 保留在 cpu 中（用于低 vram） |
| `--clip-on-cpu` | 将 clip 保留在 cpu 中（用于低 vram） |
| `--vae-on-cpu` | 将 vae 保留在 cpu 中（用于低 vram） |
| `--fa` | 使用 flash attention |
| `--diffusion-fa` | 仅在扩散模型中使用 flash attention |
| `--diffusion-conv-direct` | 在扩散模型中使用 ggml_conv2d_direct |
| `--vae-conv-direct` | 在 vae 模型中使用 ggml_conv2d_direct |
| `--circular` | 启用卷积的圆形填充 |
| `--circularx` | 仅在 x 轴（宽度）上启用圆形 RoPE 包装 |
| `--circulary` | 仅在 y 轴（高度）上启用圆形 RoPE 包装 |
| `--chroma-disable-dit-mask` | 禁用 chroma 的 dit 掩码 |
| `--qwen-image-zero-cond-t` | 为 qwen image 启用 zero_cond_t |
| `--chroma-enable-t5-mask` | 为 chroma 启用 t5 掩码 |
| `--type` | 权重类型（示例：f32, f16, q4_0, q4_1, q5_0, q5_1, q8_0, q2_K, q3_K, q4_K）。如果未指定，默认为权重文件的类型 |
| `--rng` | RNG，之一 [std_default, cuda, cpu]，默认：cuda(sd-webui), cpu(comfyui) |
| `--sampler-rng` | 采样器 RNG，之一 [std_default, cuda, cpu]。如果未指定，使用 --rng |
| `--prediction` | 预测类型覆盖，之一 [eps, v, edm_v, sd3_flow, flux_flow, flux2_flow] |
| `--lora-apply-mode` | 应用 LoRA 的方式，之一 [auto, immediately, at_runtime]，默认为 auto。在 auto 模式下，如果模型权重包含任何量化参数，将使用 at_runtime 模式；否则使用 immediately 模式。immediately 模式可能在量化参数方面存在精度和兼容性问题，但通常提供更快的推理速度，在某些情况下内存使用更低。at_runtime 模式则正好相反。 |

---

## 生成选项 (Generation Options)

| 参数 | 说明 |
|------|------|
| `-p`, `--prompt <string>` | 要渲染的提示词 |
| `-n`, `--negative-prompt <string>` | 负面提示词（默认：""） |
| `-i`, `--init-img <string>` | 初始图像路径 |
| `--end-img <string>` | 结束图像路径，flf2v 必需 |
| `--mask <string>` | 掩码图像路径 |
| `--control-image <string>` | 控制图像路径，控制网 |
| `--control-video <string>` | 控制视频帧路径，必须是目录路径。内部视频帧应以图像形式按字典序（字符）顺序存储。例如，如果控制视频路径是 `frames`，目录包含如 00.png, 01.png, ... 等图像。 |
| `--pm-id-images-dir <string>` | PHOTOMAKER 输入 id 图像目录路径 |
| `--pm-id-embed-path <string>` | PHOTOMAKER v2 id 嵌入路径 |
| `--hires-upscaler <string>` | highres fix 上采样器，Lanczos, Nearest, Latent, Latent (nearest), Latent (nearest-exact), Latent (antialiased), Latent (bicubic), Latent (bicubic antialiased)，或 --hires-upscalers-dir 下的模型名称（默认：Latent） |
| `--extra-sample-args <string>` | 额外采样器参数，key=value 列表。目前 lcm 支持 noise_clip_std, noise_scale_start, noise_scale_end |
| `-H`, `--height <int>` | 图像高度，像素空间（默认：512） |
| `-W`, `--width <int>` | 图像宽度，像素空间（默认：512） |
| `--steps <int>` | 采样步数（默认：20） |
| `--high-noise-steps <int>` | （高噪声）采样步数（默认：-1 = 自动） |
| `--clip-skip <int>` | 忽略 CLIP 网络的最后层；1 不忽略任何层，2 忽略一层（默认：-1）。<= 0 表示未指定，SD1.x 将为 1，SD2.x 将为 2 |
| `-b`, `--batch-count <int>` | 批量计数 |
| `--video-frames <int>` | 视频帧数（默认：1） |
| `--fps <int>` | 帧率（默认：24） |
| `--timestep-shift <int>` | NitroFusion 模型的时间步偏移（默认：0）。NitroSD-Realism 推荐 N 约为 250，NitroSD-Vibrant 约为 500 |
| `--upscale-repeats <int>` | 运行 ESRGAN 上采样器的次数（默认：1） |
| `--upscale-tile-size <int>` | ESRGAN 上采样的图块大小（默认：128） |
| `--hires-width <int>` | highres fix 目标宽度，0 使用 --hires-scale（默认：0） |
| `--hires-height <int>` | highres fix 目标高度，0 使用 --hires-scale（默认：0） |
| `--hires-steps <int>` | highres fix 第二遍采样步数，0 重用 --steps（默认：0） |
| `--hires-upscale-tile-size <int>` | highres fix 上采样器图块大小，保留给模型支持的上采样器（默认：128） |
| `--cfg-scale <float>` | 无条件引导比例：（默认：7.0） |
| `--img-cfg-scale <float>` | inpaint 或 instruct-pix2pix 模型的图像引导比例：（默认：与 --cfg-scale 相同） |
| `--guidance <float>` | 具有引导输入的模型的蒸馏引导比例（默认：3.5） |
| `--slg-scale <float>` | 跳层引导（SLG）比例，仅适用于 DiT 模型：（默认：0）。0 表示禁用，sd3.5 medium 的 2.5 值不错 |
| `--skip-layer-start <float>` | SLG 启用点（默认：0.01） |
| `--skip-layer-end <float>` | SLG 禁用点（默认：0.2） |
| `--eta <float>` | 噪声乘数（ddim_trailing, tcd, res_multistep 和 res_2s 默认为 0；euler_a, er_sde 和 dpm++2s_a 默认为 1） |
| `--flow-shift <float>` | SD3.x 或 WAN 等 Flow 模型的偏移值（默认：自动） |
| `--high-noise-cfg-scale <float>` | （高噪声）无条件引导比例：（默认：7.0） |
| `--high-noise-img-cfg-scale <float>` | （高噪声）inpaint 或 instruct-pix2pix 模型的图像引导比例（默认：与 --cfg-scale 相同） |
| `--high-noise-guidance <float>` | （高噪声）具有引导输入的模型的蒸馏引导比例（默认：3.5） |
| `--high-noise-slg-scale <float>` | （高噪声）跳层引导（SLG）比例，仅适用于 DiT 模型：（默认：0） |
| `--high-noise-skip-layer-start <float>` | （高噪声）SLG 启用点（默认：0.01） |
| `--high-noise-skip-layer-end <float>` | （高噪声）SLG 禁用点（默认：0.2） |
| `--high-noise-eta <float>` | （高噪声）噪声乘数（ddim_trailing, tcd, res_multistep 和 res_2s 默认为 0；euler_a, er_sde 和 dpm++2s_a 默认为 1） |
| `--strength <float>` | 加噪/去噪强度（默认：0.75） |
| `--pm-style-strength <float>` | PHOTOMAKER 风格强度 |
| `--control-strength <float>` | 应用 Control Net 的强度（默认：0.9）。1.0 对应完全破坏初始图像中的信息 |
| `--moe-boundary <float>` | Wan2.2 MoE 模型的时间步边界。（默认：0.875）。仅在 `--high-noise-steps` 设置为 -1 时启用 |
| `--vace-strength <float>` | wan vace 强度 |
| `--vae-tile-overlap <float>` | vae 分块的图块重叠，以图块大小的分数表示（默认：0.5） |
| `--hires-scale <float>` | 未设置目标大小时 highres fix 的比例（默认：2.0） |
| `--hires-denoising-strength <float>` | highres fix 第二遍去噪强度（默认：0.7） |
| `--increase-ref-index` | 根据参考图像列出的顺序自动增加其索引（从 1 开始） |
| `--disable-auto-resize-ref-image` | 禁用参考图像的自动调整大小 |
| `--disable-image-metadata` | 不在图像文件中嵌入生成元数据 |
| `--vae-tiling` | 以分块方式处理 vae 以减少内存使用 |
| `--hires` | 启用 highres fix |
| `-s`, `--seed` | RNG 种子（默认：42，< 0 使用随机种子） |
| `--sampling-method` | 采样方法，之一 [euler, euler_a, heun, dpm2, dpm++2s_a, dpm++2m, dpm++2mv2, ipndm, ipndm_v, lcm, ddim_trailing, tcd, res_multistep, res_2s, er_sde, euler_cfg_pp, euler_a_cfg_pp]（Flux/SD3/Wan 默认：euler，其他默认：euler_a） |
| `--high-noise-sampling-method` | （高噪声）采样方法，之一 [euler, euler_a, heun, dpm2, dpm++2s_a, dpm++2m, dpm++2mv2, ipndm, ipndm_v, lcm, ddim_trailing, tcd, res_multistep, res_2s, er_sde, euler_cfg_pp, euler_a_cfg_pp]（Flux/SD3/Wan 默认：euler，其他默认：euler_a） |
| `--scheduler` | 去噪器 sigma 调度器，之一 [discrete, karras, exponential, ays, gits, smoothstep, sgm_uniform, simple, kl_optimal, lcm, bong_tangent]，默认：discrete |
| `--sigmas` | 采样器的自定义 sigma 值，逗号分隔（例如："14.61,7.8,3.5,0.0"） |
| `--skip-layers` | SLG 步骤要跳过的层（默认：[7,8,9]） |
| `--high-noise-skip-layers` | （高噪声）SLG 步骤要跳过的层（默认：[7,8,9]） |
| `-r`, `--ref-image` | Flux Kontext 模型的参考图像（可多次使用） |
| `--cache-mode` | 缓存方法：'easycache' (DiT), 'ucache' (UNET), 'dbcache'/'taylorseer'/'cache-dit' (DiT 块级), 'spectrum' (UNET/DiT Chebyshev+Taylor 预测) |
| `--cache-option` | 命名缓存参数（key=value 格式，逗号分隔）。easycache/ucache：threshold=,start=,end=,decay=,relative=,reset=；dbcache/taylorseer/cache-dit：Fn=,Bn=,threshold=,warmup=；spectrum：w=,m=,lam=,window=,flex=,warmup=,stop=。示例："threshold=0.25" 或 "threshold=1.5,reset=0" |
| `--scm-mask` | cache-dit 的 SCM 步骤掩码：逗号分隔的 0/1（例如："1,1,1,0,0,1,0,0,1,0"）- 1=计算，0=可缓存 |
| `--scm-policy` | SCM 策略：'dynamic'（默认）或 'static' |
| `--vae-tile-size` | vae 分块的图块大小，格式 [X]x[Y]（默认：32x32） |
| `--vae-relative-tile-size` | vae 分块的相对图块大小，格式 [X]x[Y]，如果 < 1 则为图像大小的分数，如果 >=1 则为每维度的图块数量（覆盖 --vae-tile-size） |
